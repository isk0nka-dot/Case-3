# =============================================================================
#  Event Collector Service — Makefile
#  High-throughput proctoring event ingestion (gRPC -> Kafka + ClickHouse)
# =============================================================================

# ---------------------------------------------------------------------------
#  Variables
# ---------------------------------------------------------------------------

APP_NAME := event-collector
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
GO := go
GOFLAGS := -trimpath
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)

# Docker
DOCKER_IMAGE := argus/$(APP_NAME)
DOCKER_TAG := $(VERSION)
COMPOSE_FILE := ../../../deploy/docker-compose.yaml

# Directories
BIN_DIR := bin
CMD_DIR := cmd/server
PROTO_DIR := api/proto/v1
MIGRATION_DIR := ../../../deploy/clickhouse

# Tools
GOLANGCI_LINT := golangci-lint
PROTOC := protoc

# ClickHouse connection defaults (override via environment)
CLICKHOUSE_HOST ?= localhost
CLICKHOUSE_PORT ?= 9000
CLICKHOUSE_USER ?= default
CLICKHOUSE_PASSWORD ?=
CLICKHOUSE_DATABASE ?= argus_analytics

# ---------------------------------------------------------------------------
#  Phony Targets
# ---------------------------------------------------------------------------

.PHONY: all build run test test-cover lint proto docker-build docker-up \
        docker-down clean migrate fmt vet mod-tidy help

# ---------------------------------------------------------------------------
#  Default Target
# ---------------------------------------------------------------------------

all: fmt vet lint test build ## Run fmt, vet, lint, test, then build

# ---------------------------------------------------------------------------
#  Build
# ---------------------------------------------------------------------------

build: ## Build the binary into bin/event-collector
	@echo "==> Building $(APP_NAME) $(VERSION)..."
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) -ldflags '$(LDFLAGS)' \
		-o $(BIN_DIR)/$(APP_NAME) ./$(CMD_DIR)
	@echo "==> Binary: $(BIN_DIR)/$(APP_NAME)"

# ---------------------------------------------------------------------------
#  Run
# ---------------------------------------------------------------------------

run: build ## Build and run the service with default config
	@echo "==> Running $(APP_NAME)..."
	set -a && source .env && set +a && ./$(BIN_DIR)/$(APP_NAME)

# ---------------------------------------------------------------------------
#  Testing
# ---------------------------------------------------------------------------

test: ## Run all tests with race detector
	@echo "==> Running tests..."
	$(GO) test -race -count=1 -timeout 120s ./...

test-cover: ## Run tests with coverage report
	@echo "==> Running tests with coverage..."
	@mkdir -p $(BIN_DIR)
	$(GO) test -race -count=1 -timeout 120s -coverprofile=$(BIN_DIR)/coverage.out ./...
	$(GO) tool cover -html=$(BIN_DIR)/coverage.out -o $(BIN_DIR)/coverage.html
	@echo "==> Coverage report: $(BIN_DIR)/coverage.html"
	@$(GO) tool cover -func=$(BIN_DIR)/coverage.out | tail -1

# ---------------------------------------------------------------------------
#  Code Quality
# ---------------------------------------------------------------------------

lint: ## Run golangci-lint
	@echo "==> Linting..."
	$(GOLANGCI_LINT) run ./...

fmt: ## Format all Go source files
	@echo "==> Formatting..."
	$(GO) fmt ./...

vet: ## Run go vet on all packages
	@echo "==> Vetting..."
	$(GO) vet ./...

# ---------------------------------------------------------------------------
#  Protobuf Code Generation
# ---------------------------------------------------------------------------

proto: ## Generate Go code from proto files (requires protoc, protoc-gen-go, protoc-gen-go-grpc)
	@echo "==> Generating protobuf code..."
	$(PROTOC) --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		$(PROTO_DIR)/event_collector.proto
	$(PROTOC) --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		$(PROTO_DIR)/inferencepb/inference.proto
	@echo "==> Proto generation complete"

sync-types: ## Verify TypeScript types are in sync with proto (CI-safe)
	@echo "==> Checking proto ↔ TypeScript contract alignment..."
	@bash ../argus-infra/scripts/proto-sync.sh --check
	@echo "==> Contract sync verified"

sync-types-generate: ## Generate TypeScript types from proto (after proto changes)
	@echo "==> Syncing proto → TypeScript types..."
	@bash ../argus-infra/scripts/proto-sync.sh
	@echo "==> Sync complete"

# ---------------------------------------------------------------------------
#  Docker
# ---------------------------------------------------------------------------

docker-build: ## Build Docker image
	@echo "==> Building Docker image $(DOCKER_IMAGE):$(DOCKER_TAG)..."
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) -t $(DOCKER_IMAGE):latest .

docker-up: ## Start docker-compose (Kafka + ClickHouse + service)
	@echo "==> Starting services..."
	docker compose -f $(COMPOSE_FILE) up -d
	@echo "==> Services started. Use 'make docker-down' to stop."

docker-down: ## Stop docker-compose
	@echo "==> Stopping services..."
	docker compose -f $(COMPOSE_FILE) down

# ---------------------------------------------------------------------------
#  Database Migrations
# ---------------------------------------------------------------------------

migrate: ## Run ClickHouse schema migrations
	@echo "==> Running ClickHouse migrations..."
	@if [ -z "$(CLICKHOUSE_PASSWORD)" ]; then \
		clickhouse-client \
			--host $(CLICKHOUSE_HOST) \
			--port $(CLICKHOUSE_PORT) \
			--user $(CLICKHOUSE_USER) \
			--database $(CLICKHOUSE_DATABASE) \
			--multiquery < $(MIGRATION_DIR)/init.sql; \
	else \
		clickhouse-client \
			--host $(CLICKHOUSE_HOST) \
			--port $(CLICKHOUSE_PORT) \
			--user $(CLICKHOUSE_USER) \
			--password $(CLICKHOUSE_PASSWORD) \
			--database $(CLICKHOUSE_DATABASE) \
			--multiquery < $(MIGRATION_DIR)/init.sql; \
	fi
	@echo "==> Migrations complete"

# ---------------------------------------------------------------------------
#  Dependency Management
# ---------------------------------------------------------------------------

mod-tidy: ## Run go mod tidy
	@echo "==> Tidying modules..."
	$(GO) mod tidy

# ---------------------------------------------------------------------------
#  Cleanup
# ---------------------------------------------------------------------------

clean: ## Remove build artifacts
	@echo "==> Cleaning..."
	rm -rf $(BIN_DIR)
	$(GO) clean -cache -testcache
	@echo "==> Clean complete"

# ---------------------------------------------------------------------------
#  Help
# ---------------------------------------------------------------------------

help: ## Show this help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'
