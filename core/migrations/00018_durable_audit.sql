-- +goose Up
ALTER TABLE audit_events
 ADD COLUMN event_id uuid NOT NULL DEFAULT gen_random_uuid(),
 ADD COLUMN operation_id uuid,
 ADD COLUMN request_id uuid,
 ADD COLUMN trace_id text NOT NULL DEFAULT '',
 ADD COLUMN schema_version integer NOT NULL DEFAULT 1,
 ADD COLUMN actor_type text NOT NULL DEFAULT '',
 ADD COLUMN actor_id text NOT NULL DEFAULT '',
 ADD COLUMN issuer text NOT NULL DEFAULT '',
 ADD COLUMN action text NOT NULL DEFAULT '',
 ADD COLUMN target text NOT NULL DEFAULT '',
 ADD COLUMN config_revision bigint NOT NULL DEFAULT 0,
 ADD COLUMN stage text NOT NULL DEFAULT 'event';
ALTER TABLE audit_events DROP CONSTRAINT audit_events_status_check;
ALTER TABLE audit_events ADD CONSTRAINT audit_events_status_check CHECK (status IN ('started','ok','tool_error','error','denied'));
ALTER TABLE audit_events ADD CONSTRAINT audit_events_stage_check CHECK (stage IN ('event','admission','outcome','mutation'));
ALTER TABLE audit_events ADD CONSTRAINT audit_events_lifecycle_check CHECK ((stage NOT IN ('admission','outcome') OR (operation_id IS NOT NULL AND request_id IS NOT NULL)) AND (stage <> 'admission' OR status='started') AND (stage <> 'outcome' OR status<>'started') AND schema_version > 0);
CREATE UNIQUE INDEX audit_events_event_id_idx ON audit_events(event_id);
CREATE UNIQUE INDEX audit_events_operation_stage_idx ON audit_events(operation_id,stage) WHERE stage IN ('admission','outcome');
CREATE INDEX audit_events_pending_idx ON audit_events(ts,operation_id) WHERE stage='admission';

CREATE INDEX audit_events_request_id_idx ON audit_events(request_id);
-- Reserved durable consumer checkpoint; request handling never advances exporters.
CREATE TABLE audit_export_state (
 consumer text PRIMARY KEY,
 last_event_id bigint NOT NULL DEFAULT 0,
 updated_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE audit_export_state;
DROP INDEX audit_events_request_id_idx;
DROP INDEX audit_events_pending_idx;
DROP INDEX audit_events_operation_stage_idx;
DROP INDEX audit_events_event_id_idx;
DELETE FROM audit_events WHERE status='started';
ALTER TABLE audit_events DROP CONSTRAINT audit_events_stage_check;
ALTER TABLE audit_events DROP CONSTRAINT audit_events_lifecycle_check;
ALTER TABLE audit_events DROP CONSTRAINT audit_events_status_check;
ALTER TABLE audit_events ADD CONSTRAINT audit_events_status_check CHECK (status IN ('ok','tool_error','error','denied'));
ALTER TABLE audit_events DROP COLUMN event_id, DROP COLUMN operation_id, DROP COLUMN request_id,
 DROP COLUMN trace_id, DROP COLUMN schema_version, DROP COLUMN actor_type, DROP COLUMN actor_id,
 DROP COLUMN issuer, DROP COLUMN action, DROP COLUMN target, DROP COLUMN config_revision, DROP COLUMN stage;
