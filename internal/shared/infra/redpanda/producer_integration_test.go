//go:build integration

package redpanda

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/cornjacket/platform-services/internal/shared/domain/events"
	"github.com/cornjacket/platform-services/internal/testutil"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func testEnvelope(t *testing.T) *events.Envelope {
	t.Helper()
	return &events.Envelope{
		EventID:     uuid.Must(uuid.NewV7()),
		EventType:   "sensor.reading",
		AggregateID: "device-001",
		EventTime:   time.Now().UTC().Truncate(time.Microsecond),
		IngestedAt:  time.Now().UTC().Truncate(time.Microsecond),
		Payload:     json.RawMessage(`{"temperature": 22.5}`),
		Metadata:    events.Metadata{Source: "test", SchemaVersion: 1},
	}
}

func TestProducerPublish(t *testing.T) {
	topic := testutil.TestTopicName(t)
	producer, err := NewProducer(testutil.TestBrokers(), testLogger())
	require.NoError(t, err)
	defer producer.Close()

	env := testEnvelope(t)
	err = producer.Publish(context.Background(), topic, env)
	require.NoError(t, err)

	// Consume the message to verify it was produced
	consumer, err := kgo.NewClient(
		kgo.SeedBrokers(testutil.TestBrokers()...),
		kgo.ConsumeTopics(topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	require.NoError(t, err)
	defer consumer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fetches := consumer.PollFetches(ctx)
	require.Empty(t, fetches.Errors(), "fetch errors")

	var records []*kgo.Record
	fetches.EachRecord(func(r *kgo.Record) {
		records = append(records, r)
	})
	require.Len(t, records, 1)

	// Verify the message content
	var received events.Envelope
	require.NoError(t, json.Unmarshal(records[0].Value, &received))
	assert.Equal(t, env.EventID, received.EventID)
	assert.Equal(t, env.EventType, received.EventType)
	assert.Equal(t, env.AggregateID, received.AggregateID)

	// Verify partition key is aggregate_id
	assert.Equal(t, env.AggregateID, string(records[0].Key))
}

func TestProducerPartitionKey(t *testing.T) {
	topic := testutil.TestTopicName(t)
	producer, err := NewProducer(testutil.TestBrokers(), testLogger())
	require.NoError(t, err)
	defer producer.Close()

	// Publish 3 events with the same aggregate_id
	for i := 0; i < 3; i++ {
		env := testEnvelope(t)
		env.AggregateID = "same-device"
		require.NoError(t, producer.Publish(context.Background(), topic, env))
	}

	// Consume all messages
	consumer, err := kgo.NewClient(
		kgo.SeedBrokers(testutil.TestBrokers()...),
		kgo.ConsumeTopics(topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	require.NoError(t, err)
	defer consumer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var records []*kgo.Record
	for len(records) < 3 {
		fetches := consumer.PollFetches(ctx)
		if ctx.Err() != nil {
			break
		}
		fetches.EachRecord(func(r *kgo.Record) {
			records = append(records, r)
		})
	}
	require.Len(t, records, 3)

	// All messages with the same key should land on the same partition
	partition := records[0].Partition
	for _, r := range records[1:] {
		assert.Equal(t, partition, r.Partition,
			"same aggregate_id should route to same partition")
	}
}
