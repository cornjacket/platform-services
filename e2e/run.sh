#!/bin/bash
set -e

# Default environment
ENV="local"
TEST=""
LIST=""

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
        -list)
            LIST="true"
            shift
            ;;
        -h|--help)
            echo "Usage: $0 [options]"
            echo ""
            echo "Options:"
            echo "  -env=ENV    Environment to run against (local, dev, staging)"
            echo "              Default: local"
            echo "  -test=NAME  Run a specific test by name"
            echo "              Default: run all tests"
            echo "  -list       List available tests"
            echo "  -h, --help  Show this help message"
            echo ""
            echo "Examples:"
            echo "  $0                      # Run all tests against local"
            echo "  $0 -env=dev             # Run all tests against dev"
            echo "  $0 -test=ingest-event   # Run single test"
            echo "  $0 -list                # List available tests"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            echo "Use -h or --help for usage information"
            exit 1
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
        export E2E_INGESTION_URL="${E2E_INGESTION_URL:-https://api-dev.cornjacket.com}"
        export E2E_QUERY_URL="${E2E_QUERY_URL:-https://query-dev.cornjacket.com}"
        ;;
    staging)
        export E2E_INGESTION_URL="${E2E_INGESTION_URL:-https://api-staging.cornjacket.com}"
        export E2E_QUERY_URL="${E2E_QUERY_URL:-https://query-staging.cornjacket.com}"
        ;;
    *)
        echo "Unknown environment: $ENV"
        echo "Valid environments: local, dev, staging"
        exit 1
        ;;
esac

# Change to e2e directory
cd "$(dirname "$0")"

# Build arguments
ARGS="-env=$ENV"
if [ -n "$TEST" ]; then
    ARGS="$ARGS -test=$TEST"
fi
if [ -n "$LIST" ]; then
    ARGS="-list"
fi

# Run the test runner
exec go run . $ARGS
