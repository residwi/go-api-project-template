# Makefile

-include .env

APP_NAME ?= ecommerce-api
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GOBIN := $(or $(shell go env GOBIN),$(shell go env GOPATH)/bin)
GOOSE := $(GOBIN)/goose
RIVER := $(GOBIN)/river
GOOSE_VERSION = $(shell go list -m -f '{{.Version}}' github.com/pressly/goose/v3)
RIVER_VERSION = $(shell go list -m -f '{{.Version}}' github.com/riverqueue/river)

DB_HOST ?= localhost
DB_PORT ?= 5432
DB_USER ?= postgres
DB_PASSWORD ?= postgres
DB_NAME ?= ecommerce
DB_SSLMODE ?= disable
DATABASE_URL = postgres://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=$(DB_SSLMODE)

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

.PHONY: build-mockgateway
build-mockgateway: ## Build the dev-only mock payment gateway
	go build -o bin/mockgateway ./cmd/mockgateway

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
		go install github.com/air-verse/air@latest; \
		air; \
	fi

.PHONY: test
test: ## Run tests (requires Docker)
	@echo "Running tests..."
	go test -v -race -count=1 -timeout 5m -cover ./...

.PHONY: test-one
test-one: ## Run tests matching NAME, with .env loaded (e.g. make test-one NAME=TestPlaceOrder)
	@echo "Running tests matching: '$(NAME)'"
	env $$(grep -v '^\(#\|$$\)' .env) go test -count=1 -race -shuffle=on -failfast -v ./... -run "$(NAME)"

.PHONY: test-coverage
test-coverage: ## Run tests with coverage report (requires Docker)
	@echo "Running tests with coverage..."
	go test -v -race -count=1 -timeout 5m ./internal/... ./test/... \
		-coverpkg=$$(go list ./internal/... | grep -v /testutil | paste -sd, -) \
		-coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"
	go tool cover -func=coverage.out | grep total


.PHONY: lint
lint: ## Run linter
	golangci-lint run ./...

.PHONY: check-arch
check-arch: ## Check layer dependencies (domain -> core -> adapter) via go-arch-lint
	@if ! command -v go-arch-lint > /dev/null; then \
		echo "Installing go-arch-lint..."; \
		go install github.com/fe3dback/go-arch-lint@latest; \
	fi
	@go-arch-lint check

.PHONY: fmt
fmt: ## Format code
	@echo "Formatting..."
	golangci-lint fmt ./...

.PHONY: vet
vet: ## Run go vet
	@echo "Vetting..."
	go vet ./...

.PHONY: vuln
vuln: ## Run govulncheck
	@if command -v govulncheck > /dev/null; then \
		govulncheck ./...; \
	else \
		echo "Installing govulncheck..."; \
		go install golang.org/x/vuln/cmd/govulncheck@latest; \
		govulncheck ./...; \
	fi

.PHONY: tidy
tidy: ## Tidy go modules
	@echo "Tidying..."
	go mod tidy

.PHONY: clean
clean: ## Clean build artifacts
	@echo "Cleaning..."
	rm -rf bin/
	rm -f coverage.out coverage.html
	rm -rf tmp/

# Database commands
.PHONY: db-create
db-create: ## Create the database in the compose postgres
	@echo "Creating database..."
	docker compose exec -T postgres createdb -U $(DB_USER) $(DB_NAME) || true

.PHONY: db-drop
db-drop: ## Drop the database in the compose postgres, closing open connections
	@echo "Dropping database..."
	docker compose exec -T postgres dropdb -U $(DB_USER) --if-exists --force $(DB_NAME)

.PHONY: migrate-install
migrate-install: ## Install the goose and river CLIs at the versions go.mod pins
	@echo "Installing goose..."
	go install github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VERSION)
	@echo "Installing river..."
	go install github.com/riverqueue/river/cmd/river@$(RIVER_VERSION)

.PHONY: migrate-up
migrate-up: ## Run all pending migrations
	@echo "Running migrations..."
	$(GOOSE) -dir ./db/migrations postgres "$(DATABASE_URL)" up

.PHONY: migrate-jobs
migrate-jobs: ## Run River's own migrations
	$(RIVER) migrate-up --database-url "$(DATABASE_URL)"

.PHONY: migrate-down
migrate-down: ## Rollback the last migration
	@echo "Rolling back migration..."
	$(GOOSE) -dir ./db/migrations postgres "$(DATABASE_URL)" down

.PHONY: migrate-down-all
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
	go install github.com/air-verse/air@latest
	$(MAKE) migrate-install
	cp -n .env.example .env || true
	@command -v golangci-lint > /dev/null || echo "golangci-lint not found: brew install golangci-lint (https://golangci-lint.run/docs/welcome/install/local/)"
	@echo "Setup complete!"

.PHONY: deps
deps: ## Download dependencies
	@echo "Downloading dependencies..."
	go mod download
	go mod verify

.PHONY: mocks
mocks: ## Generate mocks with the vektra/mockery:3 image
	@echo "Generating mocks..."
	docker run --rm -q --pull always -e GOTOOLCHAIN=auto -v "$(CURDIR)":/src -w /src \
		-v go-api-mockery-mod:/go/pkg/mod -v go-api-mockery-build:/tmp/.cache \
		vektra/mockery:3

.PHONY: all
all: fmt vet check-arch lint test build ## Run all checks and build

.PHONY: test-clean
test-clean: ## Remove shared test containers (postgres + redis)
	@echo "Removing test containers..."
	@docker rm -f go-api-test-postgres go-api-test-redis 2>/dev/null || true

.PHONY: seed
seed: ## Apply seed data to the compose postgres
	@echo "Applying seed data..."
	docker compose exec -T postgres psql -U $(DB_USER) -d $(DB_NAME) -v ON_ERROR_STOP=1 < db/seeds/data.sql
