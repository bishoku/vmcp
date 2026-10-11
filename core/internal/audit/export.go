package audit

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/bishoku/vmcp/core/internal/netpolicy"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Exporter delivers one JSON event per request, with a stable idempotency key.
// HTTP acknowledgements are at least once; the sink must deduplicate event IDs.
type Exporter struct {
	Pool               *pgxpool.Pool
	Consumer, Endpoint string
	Client             *http.Client
}

func NewExporter(pool *pgxpool.Pool, consumer, endpoint string) (*Exporter, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" || !regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`).MatchString(consumer) {
		return nil, errors.New("invalid SIEM consumer or HTTPS endpoint")
	}
	return &Exporter{Pool: pool, Consumer: consumer, Endpoint: endpoint, Client: netpolicy.Client(&http.Client{Timeout: 5 * time.Second})}, nil
}
func (e *Exporter) Register(ctx context.Context) error {
	_, err := e.Pool.Exec(ctx, `INSERT INTO audit_export_state(consumer) VALUES($1) ON CONFLICT(consumer) DO NOTHING`, e.Consumer)
	return err
}
func (e *Exporter) Fill(ctx context.Context) error {
	// Discover by absence, not a sequence high-water mark: transaction commits can
	// arrive out of ID order. A late commit must never be permanently skipped.
	_, err := e.Pool.Exec(ctx, `INSERT INTO audit_deliveries(consumer,event_id)
 SELECT $1,a.event_id FROM audit_events a WHERE EXISTS(SELECT 1 FROM audit_export_state WHERE consumer=$1 AND active)
 AND NOT EXISTS(SELECT 1 FROM audit_deliveries d WHERE d.consumer=$1 AND d.event_id=a.event_id)
 ORDER BY a.id LIMIT 100 ON CONFLICT DO NOTHING`, e.Consumer)
	return err
}
func (e *Exporter) Step(ctx context.Context) (bool, error) {
	if err := e.Fill(ctx); err != nil {
		return false, err
	}
	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		return false, err
	}
	lease := fmt.Sprintf("%x-%x-%x-%x-%x", token[:4], token[4:6], token[6:8], token[8:10], token[10:])
	var id string
	var raw []byte
	var attempts int
	err := e.Pool.QueryRow(ctx, `WITH candidate AS (
 SELECT d.consumer,d.event_id FROM audit_deliveries d JOIN audit_export_state s USING(consumer)
 WHERE d.consumer=$1 AND s.active AND d.next_attempt<=now() AND (d.state='pending' OR (d.state='leased' AND d.lease_until<now()))
 ORDER BY d.next_attempt,d.event_id LIMIT 1 FOR UPDATE OF d SKIP LOCKED), claimed AS (
 UPDATE audit_deliveries d SET state='leased',lease_token=$2,lease_until=now()+interval '30 seconds',attempts=attempts+1
 FROM candidate c WHERE d.consumer=c.consumer AND d.event_id=c.event_id RETURNING d.event_id,d.attempts)
 SELECT c.event_id::text,c.attempts,to_jsonb(a) FROM claimed c JOIN audit_events a ON a.event_id=c.event_id`, e.Consumer, lease).Scan(&id, &attempts, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	request, err := http.NewRequestWithContext(ctx, "POST", e.Endpoint, bytes.NewReader(raw))
	if err != nil {
		return true, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", id)
	sendCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	response, sendErr := e.Client.Do(request.WithContext(sendCtx))
	accepted := sendErr == nil && response.StatusCode >= 200 && response.StatusCode < 300
	if response != nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		response.Body.Close()
	}
	cancel()
	complete, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if accepted {
		_, err = e.Pool.Exec(complete, `UPDATE audit_deliveries SET state='delivered',delivered_at=now(),lease_until=NULL WHERE consumer=$1 AND event_id=$2 AND lease_token=$3`, e.Consumer, id, lease)
	} else {
		state := "pending"
		if attempts >= 20 {
			state = "dead"
		}
		delay := min(3600, 1<<min(attempts, 12))
		_, err = e.Pool.Exec(complete, `UPDATE audit_deliveries SET state=$4,next_attempt=now()+make_interval(secs=>$5),lease_until=NULL WHERE consumer=$1 AND event_id=$2 AND lease_token=$3`, e.Consumer, id, lease, state, delay)
	}
	return true, err
}

// Stats is safe to expose to restricted operators; no event metadata is included.
func (e *Exporter) Stats(ctx context.Context) ([]byte, error) {
	rows, err := e.Pool.Query(ctx, `SELECT state,count(*) FROM audit_deliveries WHERE consumer=$1 GROUP BY state`, e.Consumer)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[string]int64{}
	for rows.Next() {
		var state string
		var n int64
		if err := rows.Scan(&state, &n); err != nil {
			return nil, err
		}
		counts[state] = n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return json.Marshal(counts)
}
