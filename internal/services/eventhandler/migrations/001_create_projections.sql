-- +goose Up
-- Projections table - materialized views for queries (CQRS read side)
-- Updated by the Event Handler when it consumes events from Redpanda

-- Note: Using native uuidv7() from PostgreSQL 18 (no extension needed)

-- Example projection: aggregate state
-- This is a generic key-value projection; specific projections can be added later
CREATE TABLE IF NOT EXISTS projections (
    projection_id UUID PRIMARY KEY DEFAULT uuidv7(),
    projection_type VARCHAR(255) NOT NULL,
    aggregate_id VARCHAR(255) NOT NULL,
    state JSONB NOT NULL,
    last_event_id UUID,
    last_event_timestamp TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Ensure one projection per type per aggregate
    UNIQUE (projection_type, aggregate_id)
);

-- Index for querying projections by type
CREATE INDEX IF NOT EXISTS idx_projections_type ON projections (projection_type);

-- Index for querying projections by aggregate
CREATE INDEX IF NOT EXISTS idx_projections_aggregate_id ON projections (aggregate_id);
