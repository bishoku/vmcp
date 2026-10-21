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
	"github.com/bishoku/vmcp/core/internal/secrets"
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
	Box                *secrets.Box
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
	_, err := e.Pool.Exec(ctx, `INSERT INTO audit_export_state(consumer,include_payload) VALUES($1,COALESCE((SELECT (settings->'exportPayloads'->>$1)::boolean FROM audit_settings WHERE singleton),false)) ON CONFLICT(consumer) DO NOTHING`, e.Consumer)
	return err
}
func (e *Exporter) Fill(ctx context.Context) error {
	// Discover by absence, not a sequence high-water mark: transaction commits can
	// arrive out of ID order. A late commit must never be permanently skipped.
	_, err := e.Pool.Exec(ctx, `INSERT INTO audit_deliveries(consumer,event_id)
 SELECT $1,a.event_id FROM audit_events a WHERE EXISTS(SELECT 1 FROM audit_export_state WHERE consumer=$1 AND active)
 AND NOT EXISTS(SELECT 1 FROM audit_deliveries d WHERE d.consumer=$1 AND d.event_id=a.event_id)
 ORDER BY a.id LIMIT 500 ON CONFLICT DO NOTHING`, e.Consumer)
	return err
}

type exportClaim struct {
	id         string
	attempts   int
	raw, saved []byte
	include    bool
}

func (e *Exporter) claimOne(ctx context.Context, lease string) (*exportClaim, error) {
	claim := &exportClaim{}
	err := e.Pool.QueryRow(ctx, `WITH candidate AS (
 SELECT d.consumer,d.event_id FROM audit_deliveries d JOIN audit_export_state s USING(consumer)
 WHERE d.consumer=$1 AND s.active AND d.next_attempt<=now() AND (d.state='pending' OR (d.state='leased' AND d.lease_until<now()))
 ORDER BY d.next_attempt,d.event_id LIMIT 1 FOR UPDATE OF d SKIP LOCKED), claimed AS (
 UPDATE audit_deliveries d SET state='leased',lease_token=$2,lease_until=now()+interval '30 seconds',attempts=attempts+1
 FROM candidate c WHERE d.consumer=c.consumer AND d.event_id=c.event_id RETURNING d.event_id,d.attempts,d.envelope)
 SELECT c.event_id::text,c.attempts,to_jsonb(a),c.envelope,s.include_payload FROM claimed c JOIN audit_events a ON a.event_id=c.event_id JOIN audit_export_state s ON s.consumer=$1`, e.Consumer, lease).Scan(&claim.id, &claim.attempts, &claim.raw, &claim.saved, &claim.include)
	return claim, err
}
func (e *Exporter) Step(ctx context.Context) (bool, error) {
	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		return false, err
	}
	lease := fmt.Sprintf("%x-%x-%x-%x-%x", token[:4], token[4:6], token[6:8], token[8:10], token[10:])
	claim, err := e.claimOne(ctx, lease)
	if errors.Is(err, pgx.ErrNoRows) {
		if fillErr := e.Fill(ctx); fillErr != nil {
			return false, fillErr
		}
		claim, err = e.claimOne(ctx, lease)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	id, attempts := claim.id, claim.attempts
	raw, freezeErr := e.freeze(ctx, claim, lease)
	publisher := e.Publisher
	if publisher == nil {
		publisher = &HTTPPublisher{e.Endpoint, e.Client}
	}
	accepted := false
	if freezeErr == nil {
		sendCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		accepted = publisher.Publish(sendCtx, id, raw) == nil
		cancel()
	}
	complete, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if accepted {
		_, err = e.Pool.Exec(complete, `UPDATE audit_deliveries SET state='delivered',delivered_at=now(),lease_until=NULL,envelope=NULL WHERE consumer=$1 AND event_id=$2 AND lease_token=$3`, e.Consumer, id, lease)
	} else {
		state := "pending"
		if attempts >= 20 {
			state = "dead"
		}
		delay := min(3600, 1<<min(attempts, 12))
		_, err = e.Pool.Exec(complete, `UPDATE audit_deliveries SET state=$4,next_attempt=now()+make_interval(secs=>$5),lease_until=NULL WHERE consumer=$1 AND event_id=$2 AND lease_token=$3`, e.Consumer, id, lease, state, delay)
	}
	if err == nil && freezeErr != nil {
		return true, errors.New("audit export snapshot unavailable")
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

// freeze pins the delivery bytes before network I/O. Only the fenced lease owner
// may create a snapshot; retries never rebuild it from changing retention state.
func (e *Exporter) freeze(ctx context.Context, claim *exportClaim, lease string) ([]byte, error) {
	id, saved, include, metadata := claim.id, claim.saved, claim.include, claim.raw
	var err error

	if len(saved) > 0 {
		if !include {
			return nil, errors.New("payload export authorization revoked")
		}
		if json.Valid(saved) {
			return saved, nil
		}
		if e.Box == nil {
			return nil, errors.New("export snapshot key unavailable")
		}
		return e.Box.OpenBound(saved, e.Consumer+":"+id, "audit.export")
	}
	// Metadata is immutable and protected by its delivery receipt. Avoid extra
	// reads/writes for the existing metadata-only exporter path.
	raw := metadata
	if !include {
		return raw, nil
	}
	if include {
		if e.Box == nil {
			return nil, errors.New("payload export requires an encryption key")
		}
		var event map[string]any
		if json.Unmarshal(metadata, &event) != nil {
			return nil, errors.New("invalid export metadata")
		}
		state, _ := event["payload_state"].(string)
		var cipher []byte
		err = e.Pool.QueryRow(ctx, `SELECT ciphertext FROM audit_payloads WHERE event_id=$1::uuid`, id).Scan(&cipher)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		var content json.RawMessage
		if len(cipher) > 0 {
			content, err = e.Box.OpenBound(cipher, id, "audit.payload")
			if err != nil {
				return nil, errors.New("audit payload decryption failed")
			}
		} else if state == "captured" || state == "redacted" {
			state = "expired"
		}

		event["payload"] = map[string]any{"state": state, "content": content}
		raw, err = json.Marshal(event)
		if err != nil {
			return nil, err
		}
	}
	snapshot := raw
	if e.Box != nil {
		snapshot, err = e.Box.SealBound(raw, e.Consumer+":"+id, "audit.export")
		if err != nil {
			return nil, err
		}
	}
	tag, err := e.Pool.Exec(ctx, `UPDATE audit_deliveries SET envelope=$4 WHERE consumer=$1 AND event_id=$2::uuid AND lease_token=$3::uuid AND state='leased' AND envelope IS NULL`, e.Consumer, id, lease, snapshot)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() != 1 {
		return nil, errors.New("audit export lease lost")
	}
	return raw, nil
}
