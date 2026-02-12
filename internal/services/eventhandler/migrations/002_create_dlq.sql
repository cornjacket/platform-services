-- +goose Up
-- Dead Letter Queue table - stores failed events for debugging and replay
-- Per-consumer DLQ in Postgres (not Redpanda topic)

-- Note: Using native uuidv7() from PostgreSQL 18 (no extension needed)

CREATE TABLE IF NOT EXISTS dlq (
    dlq_id UUID PRIMARY KEY DEFAULT uuidv7(),
    consumer VARCHAR(255) NOT NULL,
    event_id UUID NOT NULL,
    event_payload JSONB NOT NULL,
    error_message TEXT NOT NULL,
    failed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    retry_count INTEGER NOT NULL DEFAULT 0,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',

    -- status: 'pending', 'replayed', 'discarded'
    CONSTRAINT dlq_status_check CHECK (status IN ('pending', 'replayed', 'discarded'))
);

-- Index for querying by consumer
CREATE INDEX IF NOT EXISTS idx_dlq_consumer ON dlq (consumer);

-- Index for querying by status
CREATE INDEX IF NOT EXISTS idx_dlq_status ON dlq (status);

-- Index for querying by failed_at
CREATE INDEX IF NOT EXISTS idx_dlq_failed_at ON dlq (failed_at);
