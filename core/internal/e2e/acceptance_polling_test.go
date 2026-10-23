//go:build acceptance

package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bishoku/vmcp/core/internal/delivery"
	"github.com/bishoku/vmcp/core/internal/secrets"
	"github.com/jackc/pgx/v5/pgxpool"
)

type acceptancePollResult struct {
	values map[string]any
	err    error
}

// The host writes durable events while a different process (a real DP pod)
// serves GET SSE. No local wake channel can cross this process boundary.
func acceptancePollingProbe(parent context.Context, node *acceptanceNode, client *acceptanceClient, pool *pgxpool.Pool, master string) func() (map[string]any, error) {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan acceptancePollResult, 1)
	go func() {
		values := map[string]any{"writer": "native host", "reader": node.name, "interval_ms": 250, "budget_ms": 500, "samples": 0}
		finish := func(err error) {
			observation := map[string]any{"values": values}
			if err != nil {
				observation["error"] = err.Error()
			}
			raw, _ := json.MarshalIndent(observation, "", "  ")
			_ = os.WriteFile(filepath.Join(os.Getenv("VMCP_ACCEPTANCE_OUTPUT"), "poll-probe.json"), raw, 0600)
			done <- acceptancePollResult{values, err}
		}
		box, err := secrets.NewBox(master)
		if err != nil {
			finish(err)
			return
		}
		req, _ := http.NewRequestWithContext(ctx, "GET", node.url+fmt.Sprintf("/mcp/v1/virtual/accept-vs-%04d", client.server), nil)
		req.Header.Set("Authorization", "Bearer "+client.token)
		req.Header.Set("Mcp-Session-Id", client.session)
		req.Header.Set("MCP-Protocol-Version", "2025-06-18")
		req.Header.Set("Accept", "text/event-stream")
		transport := &http.Transport{ResponseHeaderTimeout: 5 * time.Second}
		defer transport.CloseIdleConnections()
		// Opening GET is idempotent and does not execute a tool. A failed first
		// connection is recorded separately from per-event delivery latency.
		// Never retry writes or hide failures in the actual load generator.
		connectStarted := time.Now()
		connectDeadline := connectStarted.Add(10 * time.Second)
		httpClient := &http.Client{Transport: transport}
		var resp *http.Response
		attempts := 0
		for {
			attempts++
			resp, err = httpClient.Do(req)
			values["connection_attempts"] = attempts
			if err == nil && resp.StatusCode == http.StatusOK {
				break
			}
			if err == nil {
				status := resp.StatusCode
				resp.Body.Close()
				if status < 500 {
					finish(fmt.Errorf("polling probe GET status %d", status))
					return
				}
				err = fmt.Errorf("polling probe GET status %d", status)
			}
			if time.Now().After(connectDeadline) {
				finish(fmt.Errorf("polling probe connection failed: %w", err))
				return
			}
			timer := time.NewTimer(200 * time.Millisecond)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				finish(ctx.Err())
				return
			}
		}
		values["connection_setup_ms"] = float64(time.Since(connectStarted).Microseconds()) / 1000
		defer resp.Body.Close()
		var stream string
		if err = pool.QueryRow(ctx, `SELECT id::text FROM mcp_streams WHERE session_id=$1 AND operation_id IS NULL AND NOT closed`, client.session).Scan(&stream); err != nil {
			finish(err)
			return
		}
		incoming := make(chan string, 1)
		go func() {
			scanner := bufio.NewScanner(io.LimitReader(resp.Body, 4<<20))
			for scanner.Scan() {
				line := scanner.Text()
				if !strings.HasPrefix(line, "data: ") {
					continue
				}
				var msg struct {
					Method string `json:"method"`
					Params struct {
						Data string `json:"data"`
					} `json:"params"`
				}
				if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &msg) == nil && msg.Method == "notifications/message" && strings.HasPrefix(msg.Params.Data, "poll-probe-") {
					select {
					case incoming <- msg.Params.Data:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
		latencies := make([]float64, 0, 32)
		// A GET stream lasts 65s: use a bounded probe inside this lease window.
		for i := 0; i < 24; i++ {
			if i > 0 {
				timer := time.NewTimer(time.Second)
				select {
				case <-timer.C:
				case <-ctx.Done():
					timer.Stop()
					finish(ctx.Err())
					return
				}
			}
			nonce := fmt.Sprintf("poll-probe-%d", i)
			payload, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "notifications/message", "params": map[string]any{"level": "info", "logger": "poll-probe", "data": nonce}})
			id := delivery.UUID()
			sealed, sealErr := box.SealBound(payload, id, "mcp.event")
			if sealErr != nil {
				finish(sealErr)
				return
			}
			began := time.Now()
			bounded, stop := context.WithTimeout(ctx, 2*time.Second)
			_, err = pool.Exec(bounded, `INSERT INTO mcp_events(id,stream_id,payload) VALUES($1,$2,$3)`, id, stream, sealed)
			stop()
			if err != nil {
				finish(err)
				return
			}
			timer := time.NewTimer(2 * time.Second)
			select {
			case got := <-incoming:
				timer.Stop()
				if got != nonce {
					finish(fmt.Errorf("poll probe wrong event %q", got))
					return
				}
				latencies = append(latencies, float64(time.Since(began).Microseconds())/1000)
				values["samples"] = len(latencies)
				if previous, ok := values["max_ms"].(float64); !ok || latencies[len(latencies)-1] > previous {
					values["max_ms"] = latencies[len(latencies)-1]
				}
			case <-timer.C:
				finish(fmt.Errorf("cross-replica polling exceeded 2s"))
				return
			case <-ctx.Done():
				timer.Stop()
				finish(ctx.Err())
				return
			}
		}
		sort.Float64s(latencies)
		values["samples"] = len(latencies)
		values["p50_ms"] = latencies[len(latencies)/2]
		values["p95_ms"] = latencies[int(float64(len(latencies)-1)*.95)]
		values["max_ms"] = latencies[len(latencies)-1]
		if latencies[len(latencies)-1] > 500 {
			finish(fmt.Errorf("cross-replica polling exceeded 500ms budget"))
			return
		}
		finish(nil)
	}()
	return func() (map[string]any, error) {
		defer cancel()
		select {
		case result := <-done:
			return result.values, result.err
		case <-time.After(5 * time.Second):
			return nil, fmt.Errorf("polling probe did not finish")
		}
	}
}
