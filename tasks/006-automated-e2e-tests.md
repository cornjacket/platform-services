# Task 006: Automated End-to-End Tests

**Status:** Complete
**Created:** 2026-02-06
**Updated:** 2026-02-06

## Context

Phase 1 requires automated end-to-end testing to verify the complete event flow:
```
HTTP POST → Ingestion → Outbox → Event Store + Redpanda → Event Handler → Projections → Query API
```

Manual testing is documented in DEVELOPMENT.md but is error-prone and time-consuming. Automated tests ensure the system works correctly and can be run as part of CI.

## Functionality

- Run e2e tests against a running platform instance
- Support multiple environments (local, dev, staging)
- Run individual tests or all tests
- Self-registering test pattern (tests register via `init()`)
- Clear pass/fail output with details on failure

## Design

### Directory Structure

```
platform-services/
├── e2e/
│   ├── main.go              # Test runner program
│   ├── run.sh               # Shell script to invoke runner with env setup
│   ├── README.md            # E2E test documentation
│   ├── runner/
│   │   └── runner.go        # Test registration and execution framework
│   ├── client/
│   │   └── client.go        # HTTP client helpers (reusable)
│   └── tests/
│       ├── ingest_event.go      # Test: ingest → projection
│       ├── query_projection.go  # Test: query existing projection
│       └── full_flow.go         # Test: complete write → read flow
```

### Test Runner Framework

**e2e/runner/runner.go:**

```go
package runner

import (
    "context"
    "fmt"
)

// Test represents a single e2e test.
type Test struct {
    Name        string
    Description string
    Run         func(ctx context.Context, cfg *Config) error
}

// Config holds test runner configuration.
type Config struct {
    IngestionURL string
    QueryURL     string
    Env          string
}

var registry = make(map[string]*Test)

// Register adds a test to the registry (called from test init()).
func Register(t *Test) {
    registry[t.Name] = t
}

// GetTest returns a test by name.
func GetTest(name string) (*Test, bool) {
    t, ok := registry[name]
    return t, ok
}

// GetAllTests returns all registered tests.
func GetAllTests() []*Test {
    tests := make([]*Test, 0, len(registry))
    for _, t := range registry {
        tests = append(tests, t)
    }
    return tests
}
```

### Test Registration Pattern

Each test file registers itself in `init()`:

**e2e/tests/ingest_event_test.go:**

```go
package tests

import (
    "context"
    "github.com/cornjacket/platform-services/e2e/runner"
)

func init() {
    runner.Register(&runner.Test{
        Name:        "ingest-event",
        Description: "Ingest an event and verify it appears in projections",
        Run:         runIngestEventTest,
    })
}

func runIngestEventTest(ctx context.Context, cfg *runner.Config) error {
    // 1. POST event to ingestion
    // 2. Wait for processing
    // 3. GET projection from query service
    // 4. Verify projection matches expected state
    return nil
}
```

### Main Program

**e2e/main.go:**

```go
package main

import (
    "context"
    "flag"
    "fmt"
    "os"

    "github.com/cornjacket/platform-services/e2e/runner"
    _ "github.com/cornjacket/platform-services/e2e/tests" // Register tests
)

func main() {
    env := flag.String("env", "local", "Environment (local, dev, staging)")
    testName := flag.String("test", "", "Specific test to run (runs all if empty)")
    flag.Parse()

    cfg := loadConfig(*env)

    if *testName != "" {
        // Run single test
        test, ok := runner.GetTest(*testName)
        if !ok {
            fmt.Fprintf(os.Stderr, "unknown test: %s\n", *testName)
            os.Exit(1)
        }
        runTest(test, cfg)
    } else {
        // Run all tests
        for _, test := range runner.GetAllTests() {
            runTest(test, cfg)
        }
    }
}
```

### Shell Script

**e2e/run.sh:**

```bash
#!/bin/bash
set -e

# Default environment
ENV="${ENV:-local}"

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -env=*)
            ENV="${1#*=}"
            shift
            ;;
        -test=*)
            TEST="${1#*=}"
            shift
            ;;
        *)
            shift
            ;;
    esac
done

# Set environment-specific variables
case $ENV in
    local)
        export E2E_INGESTION_URL="http://localhost:8080"
        export E2E_QUERY_URL="http://localhost:8081"
        ;;
    dev)
        export E2E_INGESTION_URL="https://api-dev.cornjacket.com"
        export E2E_QUERY_URL="https://query-dev.cornjacket.com"
        ;;
    staging)
        export E2E_INGESTION_URL="https://api-staging.cornjacket.com"
        export E2E_QUERY_URL="https://query-staging.cornjacket.com"
        ;;
    *)
        echo "Unknown environment: $ENV"
        exit 1
        ;;
esac

# Build and run
cd "$(dirname "$0")"

ARGS="-env=$ENV"
if [ -n "$TEST" ]; then
    ARGS="$ARGS -test=$TEST"
fi

go run . $ARGS
```

### Environment Configuration

| Environment | Ingestion URL | Query URL |
|-------------|---------------|-----------|
| local | `http://localhost:8080` | `http://localhost:8081` |
| dev | TBD | TBD |
| staging | TBD | TBD |

### Initial Test Cases

| Test Name | Description |
|-----------|-------------|
| `ingest-event` | POST event, verify projection created |
| `query-projection` | Query existing projection by type/id |
| `full-flow` | Ingest event, query projection, verify state |
| `list-projections` | Ingest multiple events, list projections |
| `projection-update` | Ingest event, then newer event, verify update |

## Files to Create

- `e2e/main.go` — Test runner entry point
- `e2e/run.sh` — Shell script for running tests
- `e2e/runner/runner.go` — Test framework (registration, config, execution)
- `e2e/tests/ingest_event_test.go` — Ingest and verify test
- `e2e/tests/query_projection_test.go` — Query projection test
- `e2e/tests/full_flow_test.go` — Complete flow test
- `e2e/README.md` — E2E test documentation

## Acceptance Criteria

- [x] `e2e/run.sh` runs all tests against local environment by default
- [x] `e2e/run.sh -env=local` explicitly targets local environment
- [x] `e2e/run.sh -test=ingest-event` runs single test
- [x] Tests self-register via `init()` pattern
- [x] At least 3 tests implemented: ingest, query, full-flow
- [x] Clear pass/fail output with error details
- [ ] Tests pass against running local platform (`docker compose up` + `go run ./cmd/platform`)

## Usage

```bash
# Start platform
docker compose up -d
make migrate-all
go run ./cmd/platform &

# Run all e2e tests
./e2e/run.sh

# Run specific test
./e2e/run.sh -test=ingest-event

# Run against different environment
./e2e/run.sh -env=dev
```

## Notes

- Tests assume platform is already running (they don't start/stop services)
- Each test should use unique aggregate IDs to avoid conflicts
- Consider adding `-cleanup` flag to delete test data after runs
- Future: integrate with GitHub Actions for CI
