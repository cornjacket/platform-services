# End-to-End Tests

Automated end-to-end tests for the Cornjacket Platform.

## Prerequisites

The platform must be running before executing tests:

```bash
# Start infrastructure
docker compose up -d

# Run migrations
make migrate-all

# Start platform services
go run ./cmd/platform
```

## Running Tests

```bash
# Run all tests sequentially against local environment
./e2e/run.sh

# Run specific test
./e2e/run.sh -test=ingest-event

# Run against different environment
./e2e/run.sh -env=dev

# List available tests
./e2e/run.sh -list
```

**Note:** When no `-test` flag is specified, all registered tests run sequentially in alphabetical order by test name. The runner exits with code 0 if all tests pass, or code 1 if any test fails.

## Available Tests

| Test | Description |
|------|-------------|
| `ingest-event` | Ingest an event and verify projection created |
| `query-projection` | Query projections by type, test pagination |
| `full-flow` | Complete flow: ingest, update, verify state changes |

## Adding New Tests

1. Create a new file in `e2e/tests/` (e.g., `my_test.go`)
2. Register the test in `init()`:

```go
package tests

import (
    "context"
    "github.com/cornjacket/platform-services/e2e/runner"
)

func init() {
    runner.Register(&runner.Test{
        Name:        "my-test",
        Description: "Description of what this test does",
        Run:         runMyTest,
    })
}

func runMyTest(ctx context.Context, cfg *runner.Config) error {
    // Test implementation
    // Return nil on success, error on failure
    return nil
}
```

3. Use helpers from `client` package:
   - `client.IngestEvent()` - POST event to ingestion API
   - `client.GetProjection()` - GET single projection
   - `client.ListProjections()` - GET list of projections
   - `client.WaitForProjection()` - Poll until projection appears
   - `client.UniqueID()` - Generate unique ID for test isolation

## Environment Configuration

| Environment | Ingestion URL | Query URL |
|-------------|---------------|-----------|
| local | `http://localhost:8080` | `http://localhost:8081` |
| dev | `https://api-dev.cornjacket.com` | `https://query-dev.cornjacket.com` |
| staging | `https://api-staging.cornjacket.com` | `https://query-staging.cornjacket.com` |

Override URLs with environment variables:
```bash
export E2E_INGESTION_URL="http://custom:8080"
export E2E_QUERY_URL="http://custom:8081"
./e2e/run.sh
```

## Test Isolation

Each test uses `uniqueID()` to generate unique aggregate IDs, preventing conflicts between test runs. Tests do not clean up after themselves, which allows inspection of test data if needed.
