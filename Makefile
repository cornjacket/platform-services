.PHONY: build run test clean docker-up docker-down migrate help

# Go parameters
BINARY_NAME=platform
MAIN_PATH=./cmd/platform

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Build the binary
	go build -o bin/$(BINARY_NAME) $(MAIN_PATH)

run: ## Run the application (requires docker-up first)
	go run $(MAIN_PATH)

test: ## Run tests
	go test -v ./...

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

migrate: ## Run database migrations
	@echo "Running migrations..."
	@for f in migrations/*.sql; do \
		echo "Applying $$f..."; \
		docker compose exec -T postgres psql -U cornjacket -d cornjacket -f /dev/stdin < $$f; \
	done
	@echo "Migrations complete."

migrate-local: ## Run migrations using local psql
	@echo "Running migrations..."
	@for f in migrations/*.sql; do \
		echo "Applying $$f..."; \
		psql "postgres://cornjacket:cornjacket@localhost:5432/cornjacket?sslmode=disable" -f $$f; \
	done
	@echo "Migrations complete."

lint: ## Run linter
	golangci-lint run

fmt: ## Format code
	go fmt ./...
	gofmt -s -w .

# Development workflow
dev: docker-up migrate run ## Full dev setup: start docker, migrate, run app
