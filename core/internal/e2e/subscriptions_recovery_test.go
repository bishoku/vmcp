package e2e

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/delivery"
	"github.com/bishoku/vmcp/core/internal/fixture"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/upstream"
	"github.com/bishoku/vmcp/core/internal/virtual"
)

// The downstream GET stays on node A throughout every handoff. A changed lease
// owner alone is insufficient: each replacement must deliver a new resource hint.
func TestSubscriptionNotificationsSurviveWorkerFailover(t *testing.T) {
	h := newSubscriptionRecoveryHarness(t)
	stop := h.startWorkers(t, 1)
	h.subscribe(t)
	owner := h.waitReady(t, "")
	notifications := h.notifications(t)
	h.expectUpdate(t, notifications)
	for round := 0; round < 3; round++ {
		stop()
		stop = h.startWorkers(t, round%2)
		owner = h.waitReady(t, owner)
		h.expectUpdate(t, notifications)
	}
}

// Closing only the physical SSE response leaves both gateway HTTP servers and
// the client's subscription intact. The worker must reconnect automatically.
func TestSubscriptionNotificationsRecoverAfterUpstreamDisconnect(t *testing.T) {
	h := newSubscriptionRecoveryHarness(t)
	h.startWorkers(t, 1)
	h.subscribe(t)
	owner := h.waitReady(t, "")
	notifications := h.notifications(t)
	h.expectUpdate(t, notifications)
	for round := 0; round < 3; round++ {
		if !h.physical.disconnectStream() {
			t.Fatal("no active physical SSE stream to disconnect")
		}
		owner = h.waitReady(t, owner)
		h.expectUpdate(t, notifications)
	}
}

func TestSubscriptionCancellationDuringInitializeRemainsRetryable(t *testing.T) {
	h := newSubscriptionRecoveryHarness(t)
	barrier := h.physical.blockNextInitialize()
	stop := h.startWorkers(t, 1)
	subscribed := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(h.ctx, 15*time.Second)
		defer cancel()
		_, err := h.client.Call(ctx, "resources/subscribe", map[string]string{"uri": h.uri})
		subscribed <- err
	}()
	awaitSubscriptionSignal(t, h.ctx, barrier.entered, "physical initialize request")
	initial, err := h.state(h.ctx)
	if err != nil || initial.owner == "" || initial.ready || initial.failed {
		t.Fatalf("initializing subscription state: %+v, %v", initial, err)
	}
	// Wait for a real heartbeat, rather than cancelling before the initialization
	// heartbeat has had an opportunity to touch the lease.
	waitSubscriptionCondition(t, h.ctx, "initialization lease renewal", func(ctx context.Context) (bool, string) {
		state, err := h.state(ctx)
		return err == nil && state.owner == initial.owner && state.lease.After(initial.lease), fmt.Sprintf("state=%+v error=%v", state, err)
	})
	stop()
	awaitSubscriptionSignal(t, h.ctx, barrier.cancelled, "cancelled initialize request")
	waitSubscriptionCondition(t, h.ctx, "retryable lease release after cancellation", func(ctx context.Context) (bool, string) {
		state, err := h.state(ctx)
		return err == nil && state.owner == "" && !state.ready && !state.failed, fmt.Sprintf("state=%+v error=%v", state, err)
	})
	h.startWorkers(t, 0)
	h.waitReady(t, initial.owner)
	select {
	case err := <-subscribed:
		if err != nil {
			t.Fatalf("original subscribe request did not recover: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("original subscribe request did not finish after failover")
	}
	h.expectUpdate(t, h.notifications(t))
}

type subscriptionRecoveryHarness struct {
	*env
	ctx      context.Context
	physical *subscriptionRecoveryCatalog
	nodes    [2]*virtual.Handler
	client   *upstream.Client
	endpoint string
	uri      string
}

func newSubscriptionRecoveryHarness(t *testing.T) *subscriptionRecoveryHarness {
	t.Helper()
	e := setup(t)
	ctx, cancel := context.WithTimeout(e.ctx, 45*time.Second)
	h := &subscriptionRecoveryHarness{env: e, ctx: ctx, physical: &subscriptionRecoveryCatalog{CatalogServer: fixture.NewCatalog(), streams: make(map[string]*subscriptionRecoveryStream)}}
	physical := httptest.NewServer(h.physical)
	t.Cleanup(physical.Close)
	// Cancel requests before the physical server is closed, including on setup
	// failures before the gateway nodes have been constructed.
	t.Cleanup(cancel)
	probe := upstream.NewClient(physical.URL, nil, nil)
	init, err := probe.Initialize(ctx)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := upstream.DiscoverCatalog(ctx, probe, init.Capabilities)
	if err != nil {
		t.Fatal(err)
	}
	if err := probe.Close(ctx); err != nil {
		t.Fatal(err)
	}
	database := &store.Store{Pool: e.pool, Box: e.syncer.Box}
	if err := database.Do(ctx, func(tx *store.Tx) error {
		if _, _, err := tx.PutUpstream(ctx, "recovery-source", store.UpstreamInput{Name: "Recovery source", URL: physical.URL, Transport: "streamable_http", Enabled: true}, nil, false); err != nil {
			return err
		}
		if err := tx.ReplaceCatalog(ctx, "recovery-source", catalog); err != nil {
			return err
		}
		_, _, err := tx.PutVirtualServer(ctx, "recovery-agent", store.VirtualServerInput{Name: "Recovery agent", Enabled: true, Catalog: []store.CatalogSelection{{Upstream: "recovery-source", Kind: "resource", Name: "doc://public"}}}, nil, false)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	waitSubscriptionCondition(t, ctx, "subscription catalog snapshot", func(context.Context) (bool, string) {
		objects := e.syncer.Snapshot().VirtualServers["recovery-agent"].Catalog
		return len(objects) == 1, fmt.Sprintf("catalog objects=%d", len(objects))
	})
	broker := &delivery.Store{Pool: e.pool, Box: e.syncer.Box}
	for i := range h.nodes {
		manager := virtual.NewManager()
		manager.Store = virtual.PGSessionStore{Pool: e.pool}
		node := &virtual.Handler{Snap: e.syncer, Sessions: manager, Delivery: broker, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
		mux := http.NewServeMux()
		node.Register(mux)
		server := httptest.NewServer(mux)
		h.nodes[i] = node
		if i == 0 {
			h.endpoint = server.URL + "/mcp/v1/virtual/recovery-agent"
		}
		t.Cleanup(func() {
			node.StopWorkers()
			bounded, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cleanupCancel()
			if err := node.WaitWorkers(bounded); err != nil {
				t.Errorf("wait for gateway workers: %v", err)
			}
			manager.Close()
			server.Close()
		})
	}
	h.client = upstream.NewClient(h.endpoint, nil, nil)
	if _, err := h.client.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	h.uri = config.CatalogName("recovery-source", "resource", "doc://public", "")
	t.Cleanup(func() {
		bounded, cleanupCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cleanupCancel()
		_ = h.client.Close(bounded)
	})
	t.Cleanup(cancel)
	return h
}

func (h *subscriptionRecoveryHarness) startWorkers(t *testing.T, node int) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithCancel(h.ctx)
	h.nodes[node].RunSubscriptions(ctx)
	t.Cleanup(cancel)
	return cancel
}

func (h *subscriptionRecoveryHarness) subscribe(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(h.ctx, 15*time.Second)
	defer cancel()
	if _, err := h.client.Call(ctx, "resources/subscribe", map[string]string{"uri": h.uri}); err != nil {
		t.Fatal(err)
	}
}

type subscriptionRecoveryState struct {
	owner         string
	lease         time.Time
	ready, failed bool
}

func (h *subscriptionRecoveryHarness) state(ctx context.Context) (subscriptionRecoveryState, error) {
	var state subscriptionRecoveryState
	err := h.pool.QueryRow(ctx, `SELECT COALESCE(owner::text,''),COALESCE(lease_until,'epoch'::timestamptz),ready,failed FROM mcp_resource_subscriptions WHERE session_id=$1 AND uri=$2`, h.client.SessionID(), h.uri).Scan(&state.owner, &state.lease, &state.ready, &state.failed)
	return state, err
}

func (h *subscriptionRecoveryHarness) waitReady(t *testing.T, previousOwner string) string {
	t.Helper()
	var owner string
	waitSubscriptionCondition(t, h.ctx, "replacement subscription ready", func(ctx context.Context) (bool, string) {
		state, err := h.state(ctx)
		streams := h.physical.readyStreams()
		owner = state.owner
		return err == nil && owner != "" && owner != previousOwner && state.ready && !state.failed && streams == 1, fmt.Sprintf("state=%+v streams=%d error=%v", state, streams, err)
	})
	return owner
}

type subscriptionRecoveryNotifications struct {
	events <-chan string
	done   <-chan error
}

func (h *subscriptionRecoveryHarness) notifications(t *testing.T) subscriptionRecoveryNotifications {
	t.Helper()
	ctx, cancel := context.WithCancel(h.ctx)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, h.endpoint, nil)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("Mcp-Session-Id", h.client.SessionID())
	request.Header.Set("MCP-Protocol-Version", mcp.ProtocolVersion20250618)
	transport := &http.Transport{ResponseHeaderTimeout: 5 * time.Second}
	response, err := (&http.Client{Transport: transport}).Do(request)
	if err != nil {
		cancel()
		transport.CloseIdleConnections()
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		cancel()
		response.Body.Close()
		transport.CloseIdleConnections()
		t.Fatalf("downstream GET status: %d", response.StatusCode)
	}
	events := make(chan string, 16)
	done := make(chan error, 1)
	go func() {
		scan := bufio.NewScanner(response.Body)
		for scan.Scan() {
			line := scan.Text()
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			var message struct {
				Method string `json:"method"`
				Params struct {
					URI string `json:"uri"`
				} `json:"params"`
			}
			if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &message); err != nil {
				done <- err
				return
			}
			if message.Method == "notifications/resources/updated" {
				select {
				case events <- message.Params.URI:
				case <-ctx.Done():
					done <- ctx.Err()
					return
				}
			}
		}
		err := scan.Err()
		if err == nil {
			err = io.EOF
		}
		done <- err
	}()
	t.Cleanup(func() {
		cancel()
		response.Body.Close()
		transport.CloseIdleConnections()
	})
	return subscriptionRecoveryNotifications{events: events, done: done}
}

func (h *subscriptionRecoveryHarness) expectUpdate(t *testing.T, notifications subscriptionRecoveryNotifications) {
	t.Helper()
	select {
	case uri := <-notifications.events:
		t.Fatalf("unexpected extra notification before update: %s", uri)
	default:
	}
	h.physical.Update("doc://public")
	select {
	case uri := <-notifications.events:
		if uri != h.uri {
			t.Fatalf("resource notification URI: got %q, want %q", uri, h.uri)
		}
	case err := <-notifications.done:
		t.Fatalf("downstream notification stream stopped: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("fresh resource notification not delivered after recovery")
	}
}

func waitSubscriptionCondition(t *testing.T, parent context.Context, description string, check func(context.Context) (bool, string)) {
	t.Helper()
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		ok, detail := check(ctx)
		if ok {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for %s: %s (%v)", description, detail, ctx.Err())
		case <-tick.C:
		}
	}
}

func awaitSubscriptionSignal(t *testing.T, ctx context.Context, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-ctx.Done():
		t.Fatalf("waiting for %s: %v", description, ctx.Err())
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}

// These controls are test-local: the shared catalog fixture and production
// transports keep their ordinary behavior outside explicitly armed barriers.
type subscriptionRecoveryCatalog struct {
	*fixture.CatalogServer
	mu       sync.Mutex
	streams  map[string]*subscriptionRecoveryStream
	nextInit *subscriptionRecoveryInitBarrier
}

type subscriptionRecoveryStream struct {
	cancel context.CancelFunc
	ready  bool
}

type subscriptionRecoveryInitBarrier struct {
	entered, cancelled chan struct{}
}

func (c *subscriptionRecoveryCatalog) blockNextInitialize() *subscriptionRecoveryInitBarrier {
	barrier := &subscriptionRecoveryInitBarrier{entered: make(chan struct{}), cancelled: make(chan struct{})}
	c.mu.Lock()
	c.nextInit = barrier
	c.mu.Unlock()
	return barrier
}

func (c *subscriptionRecoveryCatalog) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "read fixture request", http.StatusBadRequest)
			return
		}
		r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(raw))
		var message struct{ Method string }
		if json.Unmarshal(raw, &message) == nil && message.Method == "initialize" {
			c.mu.Lock()
			barrier := c.nextInit
			c.nextInit = nil
			c.mu.Unlock()
			if barrier != nil {
				close(barrier.entered)
				<-r.Context().Done()
				close(barrier.cancelled)
				return
			}
		}
	}
	if r.Method != http.MethodGet {
		c.CatalogServer.ServeHTTP(w, r)
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	stream := &subscriptionRecoveryStream{cancel: cancel}
	sid := r.Header.Get("Mcp-Session-Id")
	c.mu.Lock()
	c.streams[sid] = stream
	c.mu.Unlock()
	defer func() {
		cancel()
		c.mu.Lock()
		delete(c.streams, sid)
		c.mu.Unlock()
	}()
	c.CatalogServer.ServeHTTP(&subscriptionRecoveryFlusher{ResponseWriter: w, flushed: func() {
		c.mu.Lock()
		stream.ready = true
		c.mu.Unlock()
	}}, r.WithContext(ctx))
}

func (c *subscriptionRecoveryCatalog) readyStreams() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	count := 0
	for _, stream := range c.streams {
		if stream.ready {
			count++
		}
	}
	return count
}

func (c *subscriptionRecoveryCatalog) disconnectStream() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, stream := range c.streams {
		if stream.ready {
			stream.cancel()
			return true
		}
	}
	return false
}

type subscriptionRecoveryFlusher struct {
	http.ResponseWriter
	flushed func()
}

func (w *subscriptionRecoveryFlusher) Flush() {
	w.ResponseWriter.(http.Flusher).Flush()
	w.flushed()
}
