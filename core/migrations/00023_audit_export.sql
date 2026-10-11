-- +goose Up
ALTER TABLE audit_export_state ADD COLUMN active boolean NOT NULL DEFAULT true;
CREATE TABLE audit_deliveries (
 consumer text NOT NULL REFERENCES audit_export_state(consumer),
 event_id uuid NOT NULL REFERENCES audit_events(event_id) ON DELETE CASCADE,
 state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','leased','delivered','dead')),
 attempts integer NOT NULL DEFAULT 0,
 next_attempt timestamptz NOT NULL DEFAULT now(),
 lease_until timestamptz,
 lease_token uuid,
 delivered_at timestamptz,
 PRIMARY KEY(consumer,event_id)
);
CREATE INDEX audit_deliveries_due ON audit_deliveries(consumer,next_attempt) WHERE state IN ('pending','leased');
CREATE TABLE audit_legal_holds (
 event_id uuid PRIMARY KEY REFERENCES audit_events(event_id),
 reason text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_events_retention ON audit_events(ts,id);
-- +goose Down
DROP INDEX audit_events_retention;
DROP TABLE audit_legal_holds;
DROP TABLE audit_deliveries;
ALTER TABLE audit_export_state DROP COLUMN active;
