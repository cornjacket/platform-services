-- +goose Up
-- Outbox table for the outbox-first write pattern
-- Events are written here first, then processed by the background processor
-- which writes to event_store + publishes to Redpanda, then deletes from outbox

-- Note: Using native uuidv7() from PostgreSQL 18 (no extension needed)

CREATE TABLE IF NOT EXISTS outbox (
    outbox_id UUID PRIMARY KEY DEFAULT uuidv7(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    event_payload JSONB NOT NULL,
    retry_count INTEGER NOT NULL DEFAULT 0
);

-- Index for the background processor to find unprocessed events
CREATE INDEX IF NOT EXISTS idx_outbox_created_at ON outbox (created_at);

-- Notify function for LISTEN/NOTIFY pattern
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION notify_outbox_insert()
RETURNS TRIGGER AS $$
BEGIN
    PERFORM pg_notify('outbox_insert', NEW.outbox_id::text);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- Trigger to notify on insert
DROP TRIGGER IF EXISTS outbox_insert_trigger ON outbox;
CREATE TRIGGER outbox_insert_trigger
    AFTER INSERT ON outbox
    FOR EACH ROW
    EXECUTE FUNCTION notify_outbox_insert();
