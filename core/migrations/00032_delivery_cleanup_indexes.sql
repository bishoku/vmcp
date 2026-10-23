-- +goose Up
-- Child-side FK indexes avoid full-table scans for each operation cascade.
CREATE INDEX mcp_stream_operation ON mcp_streams(operation_id) WHERE operation_id IS NOT NULL;
CREATE INDEX mcp_pending_operation ON mcp_pending_requests(operation_id);
-- Keep bounded retention batches independent of the size of retained history.
CREATE INDEX mcp_operation_cleanup ON mcp_operations(created_at) WHERE state <> 'running';

-- +goose Down
DROP INDEX mcp_operation_cleanup;
DROP INDEX mcp_pending_operation;
DROP INDEX mcp_stream_operation;
