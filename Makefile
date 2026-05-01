# Makefile

APP_NAME := ecommerce-api
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GOPATH := $(shell go env GOPATH)
GOOSE := $(GOPATH)/bin/goose

DB_HOST ?= localhost
DB_PORT ?= 5432
DB_USER ?= postgres
DB_PASSWORD ?= postgres
DB_NAME ?= ecommerce
DB_SSLMODE ?= disable
DATABASE_URL := postgres://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=$(DB_SSLMODE)

.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

.PHONY: build
build: build-api build-worker ## Build all binaries

.PHONY: build-api
build-api: ## Build the API server
	@echo "Building API..."
	go build -o bin/api ./cmd/api

.PHONY: build-worker
build-worker: ## Build the worker
	@echo "Building worker..."
	go build -o bin/worker ./cmd/worker

.PHONY: run
run: ## Run the API server
	@echo "Running API..."
	go run ./cmd/api

.PHONY: run-worker
run-worker: ## Run the worker
	@echo "Running worker..."
	go run ./cmd/worker

.PHONY: dev
dev: ## Run with hot reload using air
	@if command -v air > /dev/null; then \
		air; \
	else \
		echo "Installing air..."; \
		go install github.com/cosmtrek/air@latest; \
		air; \
	fi

.PHONY: test
test: ## Run tests (requires Docker)
	@echo "Running tests..."
	go test -v -race -count=1 -timeout 5m -cover ./...

.PHONY: test-coverage
test-coverage: ## Run tests with coverage report (requires Docker)
	@echo "Running tests with coverage..."
	go test -v -race -count=1 -timeout 5m ./internal/... ./mocks/... \
		-coverpkg=$$(go list ./internal/... | grep -v /testhelper | paste -sd, -) \
		-coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"
	go tool cover -func=coverage.out | grep total


.PHONY: lint
lint: ## Run linter
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run ./...; \
	else \
		echo "Installing golangci-lint..."; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
		golangci-lint run ./...; \
	fi

.PHONY: fmt
fmt: ## Format code
	@echo "Formatting..."
	go fmt ./...
	gofmt -s -w .

.PHONY: vet
vet: ## Run go vet
	@echo "Vetting..."
	go vet ./...

.PHONY: tidy
tidy: ## Tidy go modules
	@echo "Tidying..."
	go mod tidy

.PHONY: clean
clean: ## Clean build artifacts
	@echo "Cleaning..."
	rm -rf bin/
	rm -f coverage.out coverage.html

# Database commands
.PHONY: db-create
db-create: ## Create the database
	@echo "Creating database..."
	PGPASSWORD=$(DB_PASSWORD) createdb -h $(DB_HOST) -p $(DB_PORT) -U $(DB_USER) $(DB_NAME) || true

.PHONY: db-drop
db-drop: ## Drop the database
	@echo "Dropping database..."
	PGPASSWORD=$(DB_PASSWORD) dropdb -h $(DB_HOST) -p $(DB_PORT) -U $(DB_USER) $(DB_NAME) || true

.PHONY: migrate-install
migrate-install: ## Install goose CLI
	@echo "Installing goose..."
	go install github.com/pressly/goose/v3/cmd/goose@latest

.PHONY: migrate-up
migrate-up: ## Run all pending migrations
	@echo "Running migrations..."
	$(GOOSE) -dir ./db/migrations postgres "$(DATABASE_URL)" up

.PHONY: migrate-rollback
migrate-down: ## Rollback the last migration
	@echo "Rolling back migration..."
	$(GOOSE) -dir ./db/migrations postgres "$(DATABASE_URL)" down

.PHONY: migrate-rollback-all
migrate-down-all: ## Rollback all migrations
	@echo "Rolling back all migrations..."
	$(GOOSE) -dir ./db/migrations postgres "$(DATABASE_URL)" reset

.PHONY: migrate-create
migrate-create: ## Create a new migration (usage: make migrate-create name=migration_name)
	@if [ -z "$(name)" ]; then echo "Usage: make migrate-create name=migration_name"; exit 1; fi
	@echo "Creating migration: $(name)"
	$(GOOSE) -dir ./db/migrations create $(name) sql

.PHONY: migrate-status
migrate-status: ## Show migration status
	@$(GOOSE) -dir ./db/migrations postgres "$(DATABASE_URL)" status

.PHONY: migrate-version
migrate-version: ## Show current migration version
	@$(GOOSE) -dir ./db/migrations postgres "$(DATABASE_URL)" version

# Docker commands
.PHONY: docker-build
docker-build: ## Build Docker image
	@echo "Building Docker image..."
	docker build -t $(APP_NAME):$(VERSION) .

.PHONY: docker-up
docker-up: ## Start postgres and redis
	@echo "Starting services..."
	docker compose up -d postgres redis

.PHONY: docker-dev
docker-dev: ## Start all services with hot reload (API + worker)
	@echo "Starting dev environment..."
	docker compose up --build

.PHONY: docker-down
docker-down: ## Stop all services
	@echo "Stopping services..."
	docker compose down

.PHONY: docker-logs
docker-logs: ## View logs
	docker compose logs -f

.PHONY: docker-clean
docker-clean: ## Clean up Docker resources
	docker compose down -v --rmi local

# Development setup
.PHONY: setup
setup: ## Setup development environment
	@echo "Setting up development environment..."
	go mod download
	go install github.com/cosmtrek/air@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/pressly/goose/v3/cmd/goose@latest
	cp -n .env.example .env || true
	@echo "Setup complete!"

.PHONY: deps
deps: ## Download dependencies
	@echo "Downloading dependencies..."
	go mod download
	go mod verify

.PHONY: mocks
mocks: ## Generate mocks
	@echo "Generating mocks..."
	mockery

.PHONY: all
all: fmt vet lint test build ## Run all checks and build

.PHONY: ci
ci: deps fmt vet lint test ## Run CI pipeline

.PHONY: test-clean
test-clean: ## Remove shared test containers (postgres + redis)
	@echo "Removing test containers..."
	@docker rm -f go-api-test-postgres go-api-test-redis 2>/dev/null || true

.PHONY: seed
seed: ## Apply seed data to the database (DATABASE_URL must be set)
	@if [ -z "$(DATABASE_URL)" ]; then echo "DATABASE_URL is not set"; exit 1; fi
	@echo "Applying seed data..."
	psql "$(DATABASE_URL)" -f db/seeds/data.sql
