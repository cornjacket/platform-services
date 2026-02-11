//go:build integration || component

package testutil

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

const defaultBrokers = "localhost:9092"

// TestBrokers returns the Redpanda broker addresses for integration tests.
// Override with INTEGRATION_REDPANDA_BROKERS environment variable.
func TestBrokers() []string {
	brokers := os.Getenv("INTEGRATION_REDPANDA_BROKERS")
	if brokers == "" {
		brokers = defaultBrokers
	}
	return strings.Split(brokers, ",")
}

// TestTopicName generates a unique topic name from the test name and current timestamp.
func TestTopicName(t *testing.T) string {
	t.Helper()
	// Sanitize test name: replace / and spaces with dashes
	name := strings.ReplaceAll(t.Name(), "/", "-")
	name = strings.ReplaceAll(name, " ", "-")
	return fmt.Sprintf("test-%s-%d", name, time.Now().UnixNano())
}
