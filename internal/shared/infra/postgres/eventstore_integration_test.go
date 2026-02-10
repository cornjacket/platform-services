//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cornjacket/platform-services/internal/shared/domain/events"
	"github.com/cornjacket/platform-services/internal/testutil"
)

// TestMain is shared with outbox_integration_test.go (same package).
// Migrations for both outbox and event_store are in ingestion/migrations.

func TestEventStoreInsert(t *testing.T) {
	testutil.TruncateTables(t, testPool, "event_store")
	repo := NewEventStoreRepo(testPool, testLogger())

	env := testEnvelope(t)
	err := repo.Insert(context.Background(), env)
	require.NoError(t, err)

	// Verify all 7 columns
	var (
		eventID, aggregateID, eventType string
		eventTime, ingestedAt           time.Time
		payload, metadata               []byte
	)
	err = testPool.QueryRow(context.Background(),
		`SELECT event_id, event_type, aggregate_id, event_time, ingested_at, payload, metadata
		 FROM event_store WHERE event_id = $1`, env.EventID,
	).Scan(&eventID, &eventType, &aggregateID, &eventTime, &ingestedAt, &payload, &metadata)
	require.NoError(t, err)

	assert.Equal(t, env.EventID.String(), eventID)
	assert.Equal(t, env.EventType, eventType)
	assert.Equal(t, env.AggregateID, aggregateID)
	assert.True(t, env.EventTime.Equal(eventTime), "EventTime mismatch")
	assert.True(t, env.IngestedAt.Equal(ingestedAt), "IngestedAt mismatch")

	// Verify JSONB content
	assert.JSONEq(t, string(env.Payload), string(payload))

	var storedMeta events.Metadata
	require.NoError(t, json.Unmarshal(metadata, &storedMeta))
	assert.Equal(t, env.Metadata.Source, storedMeta.Source)
	assert.Equal(t, env.Metadata.SchemaVersion, storedMeta.SchemaVersion)
}

func TestEventStoreInsert_Duplicate(t *testing.T) {
	testutil.TruncateTables(t, testPool, "event_store")
	repo := NewEventStoreRepo(testPool, testLogger())

	env := testEnvelope(t)
	require.NoError(t, repo.Insert(context.Background(), env))

	// Second insert with same event_id should fail
	err := repo.Insert(context.Background(), env)
	require.Error(t, err)

	// Verify it's a Postgres unique violation (code 23505)
	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr), "expected pgconn.PgError, got %T", err)
	assert.Equal(t, "23505", pgErr.Code)
}

func TestEventStoreInsertRoundTrip(t *testing.T) {
	testutil.TruncateTables(t, testPool, "event_store")
	repo := NewEventStoreRepo(testPool, testLogger())

	env := testEnvelope(t)
	require.NoError(t, repo.Insert(context.Background(), env))

	// Query back and verify JSONB + TIMESTAMPTZ fidelity
	var (
		eventTime, ingestedAt time.Time
		payload               []byte
	)
	err := testPool.QueryRow(context.Background(),
		`SELECT event_time, ingested_at, payload FROM event_store WHERE event_id = $1`,
		env.EventID,
	).Scan(&eventTime, &ingestedAt, &payload)
	require.NoError(t, err)

	// Timestamps should survive round-trip without precision loss
	// (Postgres TIMESTAMPTZ has microsecond precision, matching our Truncate(time.Microsecond))
	assert.True(t, env.EventTime.Equal(eventTime),
		"EventTime precision loss: sent %v, got %v", env.EventTime, eventTime)
	assert.True(t, env.IngestedAt.Equal(ingestedAt),
		"IngestedAt precision loss: sent %v, got %v", env.IngestedAt, ingestedAt)

	// JSONB round-trip
	assert.JSONEq(t, string(env.Payload), string(payload))
}
