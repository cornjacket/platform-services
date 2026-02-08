-- Migration: Dual timestamps for event-driven semantics
-- EventTime: when the event occurred (from producer)
-- IngestedAt: when the platform received it (from platform clock)

-- Rename timestamp to event_time
ALTER TABLE event_store RENAME COLUMN timestamp TO event_time;

-- Add ingested_at column (backfill with event_time for existing rows)
ALTER TABLE event_store ADD COLUMN ingested_at TIMESTAMPTZ;
UPDATE event_store SET ingested_at = event_time WHERE ingested_at IS NULL;
ALTER TABLE event_store ALTER COLUMN ingested_at SET NOT NULL;

-- Update index name to match new column
DROP INDEX IF EXISTS idx_event_store_timestamp;
CREATE INDEX IF NOT EXISTS idx_event_store_event_time ON event_store (event_time);

-- Add index for ingested_at (useful for auditing, SLA tracking)
CREATE INDEX IF NOT EXISTS idx_event_store_ingested_at ON event_store (ingested_at);
