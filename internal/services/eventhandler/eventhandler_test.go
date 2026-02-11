//go:build component

package eventhandler

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

// projectionCall captures a single call to WriteProjection.
type projectionCall struct {
	ProjType    string
	AggregateID string
	State       json.RawMessage
	Event       *events.Envelope
}

// channelProjectionWriter captures projection writes via a channel.
type channelProjectionWriter struct {
	calls chan projectionCall
}

func (m *channelProjectionWriter) WriteProjection(_ context.Context, projType, aggregateID string, state []byte, event *events.Envelope) error {
	m.calls <- projectionCall{
		ProjType:    projType,
		AggregateID: aggregateID,
		State:       state,
		Event:       event,
	}
	return nil
}

// Compile-time check: channelProjectionWriter implements ProjectionWriter.
var _ ProjectionWriter = (*channelProjectionWriter)(nil)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func produceEvent(t *testing.T, topic string, env *events.Envelope) {
	t.Helper()
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
}

func newComponentEnvelope(eventType, aggregateID string, payload map[string]any) *events.Envelope {
	payloadBytes, _ := json.Marshal(payload)
	return &events.Envelope{
		EventID:     uuid.Must(uuid.NewV7()),
		EventType:   eventType,
		AggregateID: aggregateID,
		EventTime:   time.Now().UTC().Truncate(time.Microsecond),
		IngestedAt:  time.Now().UTC().Truncate(time.Microsecond),
		Payload:     payloadBytes,
		Metadata:    events.Metadata{Source: "component-test", SchemaVersion: 1},
	}
}

func startEventHandler(t *testing.T, mock *channelProjectionWriter) *RunningService {
	t.Helper()
	topic := testutil.TestTopicName(t)
	ctx := context.Background()

	svc, err := Start(ctx, Config{
		Brokers:       testutil.TestBrokers(),
		ConsumerGroup: "test-group-" + topic,
		Topics:        []string{topic},
		PollTimeout:   time.Second,
	}, mock, testLogger())
	require.NoError(t, err)

	// Store topic on test for event producers
	t.Cleanup(func() {
		svc.Shutdown(context.Background())
	})

	return svc
}

func TestEventHandler_SensorEvent(t *testing.T) {
	topic := testutil.TestTopicName(t)
	mock := &channelProjectionWriter{calls: make(chan projectionCall, 10)}

	svc, err := Start(context.Background(), Config{
		Brokers:       testutil.TestBrokers(),
		ConsumerGroup: "test-group-" + topic,
		Topics:        []string{topic},
		PollTimeout:   time.Second,
	}, mock, testLogger())
	require.NoError(t, err)
	defer svc.Shutdown(context.Background())

	env := newComponentEnvelope("sensor.reading", "device-001", map[string]any{"temperature": 23.5})
	produceEvent(t, topic, env)

	select {
	case call := <-mock.calls:
		assert.Equal(t, "sensor_state", call.ProjType)
		assert.Equal(t, "device-001", call.AggregateID)
		assert.Equal(t, env.EventID, call.Event.EventID)
		assert.Contains(t, string(call.State), "temperature")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for projection write")
	}
}

func TestEventHandler_UserEvent(t *testing.T) {
	topic := testutil.TestTopicName(t)
	mock := &channelProjectionWriter{calls: make(chan projectionCall, 10)}

	svc, err := Start(context.Background(), Config{
		Brokers:       testutil.TestBrokers(),
		ConsumerGroup: "test-group-" + topic,
		Topics:        []string{topic},
		PollTimeout:   time.Second,
	}, mock, testLogger())
	require.NoError(t, err)
	defer svc.Shutdown(context.Background())

	env := newComponentEnvelope("user.login", "session-abc", map[string]any{"user": "alice"})
	produceEvent(t, topic, env)

	select {
	case call := <-mock.calls:
		assert.Equal(t, "user_session", call.ProjType)
		assert.Equal(t, "session-abc", call.AggregateID)
		assert.Equal(t, env.EventID, call.Event.EventID)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for projection write")
	}
	assert.Empty(t, mock.calls, "unexpected extra projection writes")
}

func TestEventHandler_UnknownEventType(t *testing.T) {
	topic := testutil.TestTopicName(t)
	mock := &channelProjectionWriter{calls: make(chan projectionCall, 10)}

	svc, err := Start(context.Background(), Config{
		Brokers:       testutil.TestBrokers(),
		ConsumerGroup: "test-group-" + topic,
		Topics:        []string{topic},
		PollTimeout:   time.Second,
	}, mock, testLogger())
	require.NoError(t, err)
	defer svc.Shutdown(context.Background())

	// Strategy to prove that unknown event does not induce projection write
	// is to first send "billing.charge" followed by sending "sensor.reading".
	// Due to ordered nature of RedPanda message bus we know that "billing.charge"
	// will be processed before "sensor.reading" but only the "sensor.reading"
	// (i.e. second event) will be receeived by the ProjectionWriter mock thereby
	// showing the unknown event was dropped

	// unknown event produced.
	// the mock will not receive a projection write for "billing.charge",
	// we must confirm the consumer actually processed it (didn't just lag).
	unknownEnv := newComponentEnvelope("billing.charge", "invoice-99", map[string]any{"amount": 100})
	produceEvent(t, topic, unknownEnv)
	env := newComponentEnvelope("sensor.reading", "device-001", map[string]any{"temperature": 23.5})
	produceEvent(t, topic, env)

	select {
	case call := <-mock.calls:
		assert.Equal(t, "sensor_state", call.ProjType)
		assert.Equal(t, "device-001", call.AggregateID)
		assert.Equal(t, env.EventID, call.Event.EventID)
		assert.Contains(t, string(call.State), "temperature")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for projection write")
	}

}
