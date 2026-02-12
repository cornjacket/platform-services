.PHONY: build run test test-integration test-component test-all clean help
.PHONY: skeleton-up skeleton-down skeleton-logs fullstack-up fullstack-down fullstack-logs
.PHONY: docker-build migrate-all migrate-ingestion migrate-eventhandler migrate
.PHONY: e2e-skeleton e2e-fullstack lint fmt dev

# Go parameters
BINARY_NAME=platform
MAIN_PATH=./cmd/platform

# Docker Compose layering
COMPOSE_DIR  = docker-compose
COMPOSE_BASE = -f $(COMPOSE_DIR)/docker-compose.yaml
COMPOSE_FULL = $(COMPOSE_BASE) -f $(COMPOSE_DIR)/docker-compose.fullstack.yaml

# Database defaults (dev — skeleton mode, host binary connects to localhost)
INGESTION_DB_URL ?= postgres://cornjacket:cornjacket@localhost:5432/cornjacket?sslmode=disable
EVENTHANDLER_DB_URL ?= postgres://cornjacket:cornjacket@localhost:5432/cornjacket?sslmode=disable

help: ## Show this help
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ── Build & Run ──────────────────────────────────────────────

build: ## Build the binary
	go build -o bin/$(BINARY_NAME) $(MAIN_PATH)

run: ## Run the application (requires skeleton-up first)
	go run $(MAIN_PATH)

# ── Skeleton Mode (infrastructure only — platform runs on host) ──

skeleton-up: ## Start infrastructure containers (Postgres, Redpanda)
	docker compose $(COMPOSE_BASE) up -d

skeleton-down: ## Stop infrastructure containers
	docker compose $(COMPOSE_BASE) down

skeleton-logs: ## Show infrastructure logs
	docker compose $(COMPOSE_BASE) logs -f

# ── Fullstack Mode (everything containerized) ────────────────

fullstack-up: ## Start full stack (infrastructure + platform + Traefik + EMQX)
	docker compose $(COMPOSE_FULL) up -d --build

fullstack-down: ## Stop full stack
	docker compose $(COMPOSE_FULL) down

fullstack-logs: ## Show full stack logs
	docker compose $(COMPOSE_FULL) logs -f

# ── Container Build ──────────────────────────────────────────

docker-build: ## Build the platform container image
	docker build -t cornjacket-platform .

# ── Migrations (per-service, ADR-0010) ───────────────────────

migrate-ingestion: ## Run ingestion service migrations
	@echo "Running ingestion migrations..."
	@for f in internal/services/ingestion/migrations/*.sql; do \
		echo "Applying $$f..."; \
		docker compose $(COMPOSE_BASE) exec -T postgres psql -U cornjacket -d cornjacket -f /dev/stdin < $$f; \
	done
	@echo "Ingestion migrations complete."

migrate-eventhandler: ## Run event handler migrations
	@echo "Running event handler migrations..."
	@for f in internal/services/eventhandler/migrations/*.sql; do \
		echo "Applying $$f..."; \
		docker compose $(COMPOSE_BASE) exec -T postgres psql -U cornjacket -d cornjacket -f /dev/stdin < $$f; \
	done
	@echo "Event handler migrations complete."

migrate-all: migrate-ingestion migrate-eventhandler ## Run all migrations

migrate: migrate-all ## Alias for migrate-all

# ── Tests ────────────────────────────────────────────────────

test: ## Run unit tests
	go test -v ./...

test-integration: ## Run integration tests (requires skeleton-up)
	go test -tags=integration -v ./...

test-component: ## Run component tests (requires skeleton-up)
	go test -tags=component -v ./internal/services/...

test-all: test test-integration test-component ## Run unit, integration, and component tests

test-coverage: ## Run tests with coverage
	go test -cover ./...

# ── E2E Tests ────────────────────────────────────────────────

e2e-skeleton: ## Run e2e tests against skeleton (binary on host)
	E2E_INGESTION_URL=http://localhost:8080 \
	E2E_QUERY_URL=http://localhost:8081 \
	go run ./e2e -env local

e2e-fullstack: ## Run e2e tests against fullstack (containerized behind Traefik)
	E2E_INGESTION_URL=http://localhost \
	E2E_QUERY_URL=http://localhost \
	go run ./e2e -env local

# ── Code Quality ─────────────────────────────────────────────

lint: ## Run linter
	golangci-lint run

fmt: ## Format code
	go fmt ./...
	gofmt -s -w .

clean: ## Clean build artifacts
	rm -rf bin/
	go clean

# ── Development Workflow ─────────────────────────────────────

dev: skeleton-up migrate-all run ## Full dev setup: start infrastructure, migrate, run app
