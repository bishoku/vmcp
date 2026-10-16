package e2e

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/secrets"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/testdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type acceptanceProfile struct {
	Servers, Tools, Resources, Sessions, SteadyRPS, BurstRPS, Workers int
	Smoke                                                             bool
}

func acceptanceNumber(name string, fallback, minValue, maxValue int) int {
	n, err := strconv.Atoi(os.Getenv(name))
	if err != nil {
		return fallback
	}
	return min(max(n, minValue), maxValue)
}
func acceptanceSeed(t *testing.T, p acceptanceProfile, fixtures []*httptest.Server) (*pgxpool.Pool, string, string, []string) {
	t.Helper()
	database := testdb.New(t)
	ctx := context.Background()
	if err := store.Migrate(ctx, database); err != nil {
		t.Fatal(err)
	}
	pool, err := store.Connect(ctx, database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	master := base64.StdEncoding.EncodeToString(key)
	box, err := secrets.NewBox(master)
	if err != nil {
		t.Fatal(err)
	}
	var tenant string
	if err = pool.QueryRow(ctx, `SELECT id::text FROM tenants WHERE slug='default'`).Scan(&tenant); err != nil {
		t.Fatal(err)
	}
	rows := make([][]any, p.Servers)
	for i := range rows {
		id, _ := audit.NewID()
		headers, err := box.SealHeadersBound(map[string]string{"Authorization": "Bearer synthetic-fixture"}, id)
		if err != nil {
			t.Fatal(err)
		}
		rows[i] = []any{id, tenant, fmt.Sprintf("accept-up-%04d", i), fmt.Sprintf("Synthetic upstream %d", i), fixtures[i%len(fixtures)].URL + fmt.Sprintf("/up-%04d", i), "streamable_http", headers}
	}
	if _, err = pool.CopyFrom(ctx, pgx.Identifier{"upstreams"}, []string{"id", "tenant_id", "slug", "name", "url", "transport", "encrypted_headers"}, pgx.CopyFromRows(rows)); err != nil {
		t.Fatal(err)
	}
	tools := make([]any, p.Tools)
	for i := range tools {
		tools[i] = acceptanceTool(i)
	}
	definitions, _ := json.Marshal(tools)
	execSQL := func(q string, args ...any) {
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	execSQL(`INSERT INTO upstream_tools(upstream_id,name,description,input_schema,annotations,definition) SELECT u.id,d->>'name',d->>'description',d->'inputSchema',d->'annotations',d FROM upstreams u CROSS JOIN jsonb_array_elements($1::jsonb) d`, string(definitions))
	execSQL(`UPDATE upstreams SET mcp_capabilities='{"tools":{},"resources":{}}'::jsonb`)
	execSQL(`INSERT INTO upstream_catalog(upstream_id,kind,name,definition) SELECT u.id,'resource','test://resource/'||n,jsonb_build_object('uri','test://resource/'||n,'name','Synthetic resource '||n,'description','Contact support@example.com') FROM upstreams u CROSS JOIN generate_series(0,$1-1) n`, p.Resources)
	execSQL(`INSERT INTO virtual_servers(tenant_id,slug,name,pii_config) SELECT $1::uuid,'accept-vs-'||lpad(n::text,4,'0'),'Synthetic virtual server '||n,CASE WHEN n%10=0 THEN '{"enabled":true,"detectors":["email"],"maskArguments":false}'::jsonb ELSE '{}'::jsonb END FROM generate_series(0,$2-1) n`, tenant, p.Servers)
	execSQL(`INSERT INTO virtual_server_tools(virtual_server_id,upstream_id,tool_name,alias,position) SELECT v.id,u.id,t.name,t.name,substring(t.name from 6)::int FROM virtual_servers v JOIN upstreams u ON right(v.slug,4)=right(u.slug,4) JOIN upstream_tools t ON t.upstream_id=u.id`)
	execSQL(`INSERT INTO virtual_catalog(virtual_server_id,upstream_id,kind,name,position) SELECT v.id,u.id,c.kind,c.name,substring(c.name from 17)::int FROM virtual_servers v JOIN upstreams u ON right(v.slug,4)=right(u.slug,4) JOIN upstream_catalog c ON c.upstream_id=u.id`)
	tokens := make([]string, 64)
	for i := range tokens {
		nonce, _ := audit.NewID()
		tokens[i] = "acceptance-" + nonce
		execSQL(`INSERT INTO agent_keys(tenant_id,slug,key_hash,key_prefix) VALUES($1::uuid,$2,$3,'synthetic')`, tenant, fmt.Sprintf("accept-key-%02d", i), store.HashKey(tokens[i]))
	}
	execSQL(`INSERT INTO agent_key_servers(agent_key_id,virtual_server_id) SELECT k.id,v.id FROM agent_keys k CROSS JOIN virtual_servers v`)
	return pool, database, master, tokens
}

type acceptanceNode struct {
	mu                                      sync.Mutex
	cmd                                     *exec.Cmd
	done                                    chan error
	url, name                               string
	pid                                     int
	alive                                   bool
	binary, database, master, redis, output string
}

func (n *acceptanceNode) start(t *testing.T) {
	t.Helper()
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.url == "" {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		n.url = "http://" + listener.Addr().String()
		listener.Close()
	}
	parsed, _ := url.Parse(n.url)
	cmd := exec.Command(n.binary)
	// Deliberately do not inherit VMCP application settings or production credentials.
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "GOMAXPROCS=2", "GOMEMLIMIT=768MiB", "VMCP_DATABASE_URL=" + n.database, "VMCP_MASTER_KEY=" + n.master, "VMCP_TENANT_ID=default", "VMCP_DP_ADDR=" + parsed.Host, "VMCP_NODE_ID=" + n.name, "VMCP_AUDIT_MODE=strict", "VMCP_REDIS_URL=" + n.redis, "VMCP_REDIS_FAILURE_POLICY=strict", "VMCP_EGRESS_ALLOW_CIDRS=127.0.0.0/8,::1/128", "VMCP_HEALTH_INTERVAL=30s"}
	file, err := os.OpenFile(filepath.Join(n.output, n.name+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdout = file
	cmd.Stderr = file
	started := time.Now()
	if err = cmd.Start(); err != nil {
		file.Close()
		t.Fatal(err)
	}
	n.cmd = cmd
	n.pid = cmd.Process.Pid
	if err := os.WriteFile(filepath.Join(n.output, n.name+".pid"), []byte(strconv.Itoa(n.pid)), 0600); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		file.Close()
		t.Fatal(err)
	}
	n.alive = true
	n.done = make(chan error, 1)
	done := n.done
	go func() { done <- cmd.Wait(); file.Close() }()
	client := &http.Client{Timeout: time.Second}
	for time.Since(started) < 60*time.Second {
		resp, err := client.Get(n.url + "/readyz")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				t.Logf("%s ready in %s (PID %d)", n.name, time.Since(started).Round(time.Millisecond), n.pid)
				return
			}
		}
		select {
		case err := <-done:
			n.alive = false
			t.Fatalf("%s exited during startup: %v; see node log", n.name, err)
		default:
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("%s did not become ready", n.name)
}
func (n *acceptanceNode) kill() {
	n.mu.Lock()
	if !n.alive {
		n.mu.Unlock()
		return
	}
	cmd, done := n.cmd, n.done
	n.alive = false
	n.mu.Unlock()
	_ = cmd.Process.Kill()
	<-done
}
func (n *acceptanceNode) process() (int, bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.pid, n.alive
}

// A passive-health round-robin proxy, matching the demo's no-POST-retry policy.
// Failures open a 10s cooldown. It never replays a request on another node.
func acceptanceBalancer(nodes []*acceptanceNode) *httptest.Server {
	proxies := make([]*httputil.ReverseProxy, len(nodes))
	var next atomic.Uint64
	down := make([]atomic.Int64, len(nodes))
	transport := &http.Transport{MaxIdleConns: 512, MaxIdleConnsPerHost: 256, IdleConnTimeout: 90 * time.Second, DialContext: (&net.Dialer{Timeout: time.Second}).DialContext}
	for i, n := range nodes {
		u, _ := url.Parse(n.url)
		proxy := httputil.NewSingleHostReverseProxy(u)
		proxy.Transport = transport
		proxy.ErrorLog = log.New(io.Discard, "", 0)
		proxy.ModifyResponse = func(resp *http.Response) error { resp.Header.Set("X-Acceptance-Node", n.name); return nil }
		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			if r.Context().Err() == nil {
				down[i].Store(time.Now().Add(10 * time.Second).UnixNano())
			}
			http.Error(w, "acceptance proxy: node unavailable", 502)
		}
		proxies[i] = proxy
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for range proxies {
			i := int(next.Add(1)-1) % len(proxies)
			if time.Now().UnixNano() >= down[i].Load() {
				proxies[i].ServeHTTP(w, r)
				return
			}
		}
		http.Error(w, "acceptance proxy: no healthy node", 503)
	}))
}
