.PHONY: build run test test-integration test-all clean docker-up docker-down migrate-all migrate-ingestion migrate-eventhandler help

# Go parameters
BINARY_NAME=platform
MAIN_PATH=./cmd/platform

# Database defaults (dev)
INGESTION_DB_URL ?= postgres://cornjacket:cornjacket@localhost:5432/cornjacket?sslmode=disable
EVENTHANDLER_DB_URL ?= postgres://cornjacket:cornjacket@localhost:5432/cornjacket?sslmode=disable

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Build the binary
	go build -o bin/$(BINARY_NAME) $(MAIN_PATH)

run: ## Run the application (requires docker-up first)
	go run $(MAIN_PATH)

test: ## Run tests
	go test -v ./...

test-integration: ## Run integration tests (requires docker-up)
	go test -tags=integration -v ./...

test-all: test test-integration ## Run unit and integration tests

test-coverage: ## Run tests with coverage
	go test -cover ./...

clean: ## Clean build artifacts
	rm -rf bin/
	go clean

docker-up: ## Start infrastructure containers
	docker compose up -d

docker-down: ## Stop infrastructure containers
	docker compose down

docker-logs: ## Show infrastructure logs
	docker compose logs -f

# Per-service migrations (ADR-0010)
migrate-ingestion: ## Run ingestion service migrations
	@echo "Running ingestion migrations..."
	@for f in internal/services/ingestion/migrations/*.sql; do \
		echo "Applying $$f..."; \
		docker compose exec -T postgres psql -U cornjacket -d cornjacket -f /dev/stdin < $$f; \
	done
	@echo "Ingestion migrations complete."

migrate-eventhandler: ## Run event handler migrations
	@echo "Running event handler migrations..."
	@for f in internal/services/eventhandler/migrations/*.sql; do \
		echo "Applying $$f..."; \
		docker compose exec -T postgres psql -U cornjacket -d cornjacket -f /dev/stdin < $$f; \
	done
	@echo "Event handler migrations complete."

migrate-all: migrate-ingestion migrate-eventhandler ## Run all migrations

# Aliases for convenience
migrate: migrate-all ## Alias for migrate-all

lint: ## Run linter
	golangci-lint run

fmt: ## Format code
	go fmt ./...
	gofmt -s -w .

# Development workflow
dev: docker-up migrate-all run ## Full dev setup: start docker, migrate, run app
