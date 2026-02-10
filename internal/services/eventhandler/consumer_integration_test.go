//go:build integration

package eventhandler

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/cornjacket/platform-services/internal/shared/domain/events"
	"github.com/cornjacket/platform-services/internal/testutil"
)

func TestConsumerRoundTrip(t *testing.T) {
	topic := testutil.TestTopicName(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Track what the handler receives
	var mu sync.Mutex
	var received *events.Envelope

	registry := NewHandlerRegistry(logger)
	registry.Register("sensor.", &mockEventHandler{
		HandleFn: func(ctx context.Context, event *events.Envelope) error {
			mu.Lock()
			defer mu.Unlock()
			received = event
			return nil
		},
	})

	// Create consumer
	consumer, err := NewConsumer(registry, ConsumerConfig{
		Brokers:     testutil.TestBrokers(),
		GroupID:     "test-group-" + topic, // unique group per test
		Topics:      []string{topic},
		PollTimeout: time.Second,
	}, logger)
	require.NoError(t, err)

	// Start consumer in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	consumerDone := make(chan error, 1)
	go func() {
		consumerDone <- consumer.Start(ctx)
	}()

	// Produce a message to the topic
	env := &events.Envelope{
		EventID:     uuid.Must(uuid.NewV7()),
		EventType:   "sensor.reading",
		AggregateID: "device-roundtrip",
		EventTime:   time.Now().UTC().Truncate(time.Microsecond),
		IngestedAt:  time.Now().UTC().Truncate(time.Microsecond),
		Payload:     json.RawMessage(`{"value": 42}`),
		Metadata:    events.Metadata{Source: "integration-test", SchemaVersion: 1},
	}

	value, err := json.Marshal(env)
	require.NoError(t, err)

	producer, err := kgo.NewClient(
		kgo.SeedBrokers(testutil.TestBrokers()...),
		kgo.AllowAutoTopicCreation(),
	)
	require.NoError(t, err)
	defer producer.Close()

	results := producer.ProduceSync(context.Background(), &kgo.Record{
		Topic: topic,
		Key:   []byte(env.AggregateID),
		Value: value,
	})
	require.NoError(t, results.FirstErr())

	// Wait for the handler to receive the event
	deadline := time.After(5 * time.Second)
	for {
		mu.Lock()
		got := received
		mu.Unlock()
		if got != nil {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for consumer to receive event")
		case <-time.After(50 * time.Millisecond):
			// poll again
		}
	}

	// Verify the deserialized envelope
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, env.EventID, received.EventID)
	assert.Equal(t, env.EventType, received.EventType)
	assert.Equal(t, env.AggregateID, received.AggregateID)
	assert.JSONEq(t, string(env.Payload), string(received.Payload))

	// Stop consumer
	cancel()
	select {
	case err := <-consumerDone:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("consumer did not stop within timeout")
	}
}
