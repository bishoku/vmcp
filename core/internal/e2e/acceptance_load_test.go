//go:build acceptance

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bishoku/vmcp/core/internal/omni"
	"github.com/jackc/pgx/v5/pgxpool"
)

type acceptanceRecord struct {
	Session, RPC, RequestID string
	Success                 bool
}
type acceptanceTracker struct {
	mu       sync.Mutex
	records  []acceptanceRecord
	seq      atomic.Int64
	dispatch atomic.Int64
}
type acceptanceBin struct {
	Second                   int `json:"second"`
	Success, Errors, Dropped int
}
type acceptancePhase struct {
	Name                                                         string `json:"name"`
	TargetRPS                                                    int    `json:"target_rps"`
	PlannedSeconds, ElapsedSeconds                               float64
	Scheduled, Offered, Success, Errors, Dropped, InvalidResults int
	CompletedRPS, SuccessPercent, P50MS, P95MS, P99MS, MaxMS     float64
	HTTPStatuses                                                 map[string]int
	Nodes                                                        map[string]int
	ErrorExamples                                                []string
	Seconds                                                      []acceptanceBin
	FaultSecond                                                  *float64 `json:"fault_second,omitempty"`
	SLOPass                                                      bool     `json:"slo_pass"`
}

func (a *acceptanceTracker) call(ctx context.Context, c *acceptanceClient, tool int) (acceptanceResponse, bool) {
	id := a.seq.Add(1)
	nonce := fmt.Sprintf("accept-%d", id)
	var args any = map[string]string{"call_id": nonce, "message": "agent@example.com"}
	if c.rest {
		args = map[string]any{"body": args}
	}
	params := map[string]any{"name": fmt.Sprintf("tool-%03d", tool), "arguments": args}
	if c.omni {
		params = map[string]any{"name": "invoke_tool", "arguments": map[string]any{"tool_ref": omni.Ref(fmt.Sprintf("accept-vs-%04d", c.server), fmt.Sprintf("tool-%03d", tool)), "arguments": args}}
	}
	response := c.request(ctx, "tools/call", params, id)
	valid := false
	if response.err == nil {
		var result struct {
			Content []struct{ Type, Text string }
			IsError bool `json:"isError"`
		}
		if json.Unmarshal(response.result, &result) == nil && !result.IsError && len(result.Content) == 1 {
			var value map[string]string
			text := []byte(result.Content[0].Text)
			if c.rest {
				var wrapped struct {
					Result json.RawMessage `json:"result"`
				}
				_ = json.Unmarshal(text, &wrapped)
				text = wrapped.Result
			}
			if json.Unmarshal(text, &value) == nil {
				expected := "agent@example.com"
				if c.server%10 == 0 {
					expected = "[REDACTED:EMAIL]"
				}
				valid = value["call_id"] == nonce && value["tool"] == fmt.Sprintf("tool-%03d", tool) && value["route"] == fmt.Sprintf("/up-%04d", c.server) && value["message"] == expected
			}
		}
	}
	a.mu.Lock()
	a.records = append(a.records, acceptanceRecord{c.session, fmt.Sprint(id), response.requestID, response.err == nil && valid})
	a.mu.Unlock()
	return response, valid
}
func acceptanceQuantile(values []float64, q float64) float64 {
	if len(values) == 0 {
		return 0
	}
	return values[int(math.Ceil(q*float64(len(values))))-1]
}
func acceptanceRunPhase(ctx context.Context, name string, rps int, duration time.Duration, clients []*acceptanceClient, workers, tools int, tracker *acceptanceTracker, fault func(), faultAfter time.Duration) acceptancePhase {
	result := acceptancePhase{Name: name, TargetRPS: rps, PlannedSeconds: duration.Seconds(), HTTPStatuses: map[string]int{}, Nodes: map[string]int{}}
	if fault != nil {
		second := faultAfter.Seconds()
		result.FaultSecond = &second
	}
	type job struct{ number int64 }
	jobs := make(chan job)
	var wg sync.WaitGroup
	var mu sync.Mutex
	bins := map[int]*acceptanceBin{}
	latencies := make([]float64, 0, rps*int(duration.Seconds()))
	start := time.Now()
	recordBin := func(second int) *acceptanceBin {
		if bins[second] == nil {
			bins[second] = &acceptanceBin{Second: second}
		}
		return bins[second]
	}
	for range workers {
		wg.Go(func() {
			for j := range jobs {
				client := clients[int(j.number)%len(clients)]
				tool := int((uint64(j.number) * 1103515245 >> 16) % uint64(tools))
				callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				began := time.Now()
				response, valid := tracker.call(callCtx, client, tool)
				elapsed := float64(time.Since(began)) / float64(time.Millisecond)
				cancel()
				mu.Lock()
				result.HTTPStatuses[fmt.Sprint(response.status)]++
				if response.node != "" {
					result.Nodes[response.node]++
				}
				result.MaxMS = max(result.MaxMS, elapsed)
				bin := recordBin(int(time.Since(start).Seconds()))
				if response.err == nil && valid {
					result.Success++
					bin.Success++
					latencies = append(latencies, elapsed)
				} else {
					result.Errors++
					bin.Errors++
					if response.err == nil {
						result.InvalidResults++
					}
					if len(result.ErrorExamples) < 8 {
						description := "invalid result"
						if response.err != nil {
							description = response.err.Error()
						}
						result.ErrorExamples = append(result.ErrorExamples, description)
					}
				}
				mu.Unlock()
			}
		})
	}
	faultDone := make(chan struct{})
	if fault != nil {
		go func() {
			defer close(faultDone)
			timer := time.NewTimer(faultAfter)
			defer timer.Stop()
			select {
			case <-ctx.Done():
			case <-timer.C:
				fault()
			}
		}()
	} else {
		close(faultDone)
	}
	interval := time.Second / time.Duration(rps)
	scheduled := int(duration / interval)
	timer := time.NewTimer(0)
	for i := 0; i < scheduled; i++ {
		due := start.Add(time.Duration(i) * interval)
		timer.Reset(max(0, time.Until(due)))
		select {
		case <-ctx.Done():
			i = scheduled
			continue
		case <-timer.C:
		}
		number := tracker.dispatch.Add(1) - 1 // Dispatch index is independent of RPC IDs.
		mu.Lock()
		result.Scheduled++
		mu.Unlock()
		dropped := time.Since(due) > max(5*interval, 10*time.Millisecond)
		if !dropped {
			select {
			case jobs <- job{number}:
				mu.Lock()
				result.Offered++
				mu.Unlock()
			default:
				dropped = true
			}
		}
		if dropped {
			mu.Lock()
			result.Dropped++
			recordBin(int(time.Since(start).Seconds())).Dropped++
			mu.Unlock()
		}
	}
	timer.Stop()
	close(jobs)
	wg.Wait()
	<-faultDone
	result.ElapsedSeconds = time.Since(start).Seconds()
	sort.Float64s(latencies)
	result.P50MS = acceptanceQuantile(latencies, .50)
	result.P95MS = acceptanceQuantile(latencies, .95)
	result.P99MS = acceptanceQuantile(latencies, .99)
	result.CompletedRPS = float64(result.Success) / result.ElapsedSeconds
	if result.Offered > 0 {
		result.SuccessPercent = 100 * float64(result.Success) / float64(result.Offered)
	}
	for _, bin := range bins {
		result.Seconds = append(result.Seconds, *bin)
	}
	sort.Slice(result.Seconds, func(i, j int) bool { return result.Seconds[i].Second < result.Seconds[j].Second })
	return result
}
func acceptanceCheckAudit(ctx context.Context, pool *pgxpool.Pool, tracker *acceptanceTracker, fixture *acceptanceFixture) (map[string]any, error) {
	tracker.mu.Lock()
	records := append([]acceptanceRecord(nil), tracker.records...)
	tracker.mu.Unlock()
	var successIDs []string
	failed := map[string]bool{}
	for _, record := range records {
		if record.Success {
			if record.RequestID == "" {
				return nil, fmt.Errorf("successful call has no durable request ID")
			}
			successIDs = append(successIDs, record.RequestID)
		} else {
			failed[record.Session+"\x00"+record.RPC] = true
		}
	}
	var admissions, outcomes int
	err := pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE stage='admission'),count(*) FILTER(WHERE stage='outcome' AND status='ok') FROM audit_events WHERE method='tools/call' AND request_id=ANY($1::uuid[])`, successIDs).Scan(&admissions, &outcomes)
	if err != nil {
		return nil, err
	}
	rows, err := pool.Query(ctx, `SELECT a.request_id::text,COALESCE(o.session_id,''),COALESCE(o.client_request_id,'') FROM audit_events a LEFT JOIN audit_events outcome ON outcome.operation_id=a.operation_id AND outcome.stage='outcome' LEFT JOIN mcp_operations o ON o.id=a.request_id WHERE a.stage='admission' AND a.method='tools/call' AND outcome.id IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	incomplete, unexplained := 0, 0
	for rows.Next() {
		var requestID, session, rpc string
		if err = rows.Scan(&requestID, &session, &rpc); err != nil {
			return nil, err
		}
		incomplete++
		if !failed[session+"\x00"+rpc] {
			unexplained++
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	var activeServers, activeUpstreams, activeTools, activeKeys int
	if err = pool.QueryRow(ctx, `SELECT count(DISTINCT virtual_server),count(DISTINCT upstream),count(DISTINCT (virtual_server,tool)),count(DISTINCT agent_key) FROM audit_events WHERE method='tools/call' AND stage='admission'`).Scan(&activeServers, &activeUpstreams, &activeTools, &activeKeys); err != nil {
		return nil, err
	}
	report := map[string]any{"active_virtual_servers": activeServers, "active_upstreams": activeUpstreams, "active_virtual_tools": activeTools, "active_agent_keys": activeKeys, "successful_client_calls": len(successIDs), "matching_admissions": admissions, "matching_ok_outcomes": outcomes, "incomplete_operations": incomplete, "incomplete_unrelated_to_failed_calls": unexplained, "synthetic_upstream_executions": fixture.count.Load(), "duplicate_executions": fixture.duplicate.Load(), "failed_calls": len(failed)}
	if admissions != len(successIDs) || outcomes != len(successIDs) || unexplained > 0 || fixture.duplicate.Load() > 0 {
		return report, fmt.Errorf("audit or execution consistency gate failed")
	}
	return report, nil
}

func acceptanceSummarizeSLO(phase *acceptancePhase) {
	if phase.Name == "warmup" {
		phase.SLOPass = phase.InvalidResults == 0
		return
	}
	phase.SLOPass = phase.InvalidResults == 0 && phase.P95MS <= 250 && phase.P99MS <= 1000 && phase.CompletedRPS >= .95*float64(phase.TargetRPS) && phase.SuccessPercent >= 99.9
	if phase.Name == "spike" {
		phase.SLOPass = phase.InvalidResults == 0 && phase.CompletedRPS >= .8*float64(phase.TargetRPS) && phase.P99MS < 10000 && phase.SuccessPercent >= 80
	}
	if phase.Name == "node-loss" || phase.Name == "node-rejoin" {
		phase.SLOPass = phase.InvalidResults == 0 && phase.CompletedRPS >= .95*float64(phase.TargetRPS) && phase.SuccessPercent >= 99 && phase.P99MS <= 1000
		// After a 10s transition window, passive failure probes may still fail one
		// request per cooldown; no automatic replay is allowed.
		after := 10
		if phase.FaultSecond != nil {
			after = int(*phase.FaultSecond) + 10
		}
		success, errors, drops := 0, 0, 0
		for _, bin := range phase.Seconds {
			if bin.Second >= after {
				success += bin.Success
				errors += bin.Errors
				drops += bin.Dropped
			}
		}
		if success+errors > 0 && (float64(errors)/float64(success+errors) > .001 || drops > int(.05*float64(success+errors+drops))) {
			phase.SLOPass = false
		}
	}
}
func acceptanceMetricValues(raw string) map[string]float64 {
	names := map[string]bool{"vmcp_pool_acquired": true, "vmcp_pool_max": true, "vmcp_pool_acquire_seconds_total": true, "vmcp_pool_empty_acquires_total": true, "vmcp_pool_cancelled_acquires_total": true, "vmcp_snapshot_age_seconds": true, "vmcp_snapshot_refresh_seconds": true, "vmcp_snapshot_revision": true, "vmcp_sessions_active": true, "vmcp_streams_active": true, "vmcp_audit_failed_writes_total": true, "vmcp_audit_dropped_total": true, "vmcp_audit_incomplete": true, "vmcp_redis_failures_total": true, "vmcp_redis_degraded": true, "vmcp_operation_label_overflows_total": true}
	result := map[string]float64{}
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || !names[fields[0]] {
			continue
		}
		var value float64
		if _, err := fmt.Sscan(fields[1], &value); err == nil {
			result[fields[0]] = value
		}
	}
	return result
}
