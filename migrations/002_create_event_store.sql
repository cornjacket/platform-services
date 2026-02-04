-- Event store table - append-only log of all events (CQRS write side)
-- This is the source of truth for all events in the system

CREATE TABLE IF NOT EXISTS event_store (
    event_id UUID PRIMARY KEY,
    event_type VARCHAR(255) NOT NULL,
    aggregate_id VARCHAR(255) NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL,
    payload JSONB NOT NULL,
    metadata JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for querying by aggregate
CREATE INDEX IF NOT EXISTS idx_event_store_aggregate_id ON event_store (aggregate_id);

-- Index for querying by event type
CREATE INDEX IF NOT EXISTS idx_event_store_event_type ON event_store (event_type);

-- Index for querying by timestamp (useful for replay)
CREATE INDEX IF NOT EXISTS idx_event_store_timestamp ON event_store (timestamp);
