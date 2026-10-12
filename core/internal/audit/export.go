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

// Publisher returns nil only after the destination has acknowledged acceptance.
// Implementations must bound buffers, respect context cancellation and tolerate duplicates.
type Publisher interface {
	Publish(context.Context, string, []byte) error
	Close()
}

// Exporter delivers at least once using fenced PostgreSQL receipts.
// HTTP fields remain supported for existing callers; broker publishers use Publisher.
type Exporter struct {
	Publisher          Publisher
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

// NewPublishingExporter shares the HTTP exporter's durable delivery ledger.
func NewPublishingExporter(pool *pgxpool.Pool, consumer string, publisher Publisher) (*Exporter, error) {
	if !regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`).MatchString(consumer) || publisher == nil {
		return nil, errors.New("invalid audit consumer or publisher")
	}
	return &Exporter{Pool: pool, Consumer: consumer, Publisher: publisher}, nil
}

// HTTPPublisher preserves the existing HTTPS and egress-policy contract.
type HTTPPublisher struct {
	Endpoint string
	Client   *http.Client
}

func NewHTTPPublisher(endpoint string) (*HTTPPublisher, error) {
	e, err := NewExporter(nil, "validation", endpoint)
	if err != nil {
		return nil, err
	}
	return &HTTPPublisher{e.Endpoint, e.Client}, nil
}
func (p *HTTPPublisher) Close() { p.Client.CloseIdleConnections() }
func (p *HTTPPublisher) Publish(ctx context.Context, id string, raw []byte) error {
	request, err := http.NewRequestWithContext(ctx, "POST", p.Endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", id)
	response, err := p.Client.Do(request)
	if response != nil {
		defer response.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	}
	if err != nil {
		return errors.New("HTTP audit delivery failed")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return errors.New("HTTP audit destination rejected event")
	}
	return nil
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
	publisher := e.Publisher
	if publisher == nil {
		publisher = &HTTPPublisher{e.Endpoint, e.Client}
	}
	sendCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	accepted := publisher.Publish(sendCtx, id, raw) == nil
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
