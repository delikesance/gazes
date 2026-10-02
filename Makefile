.DEFAULT_GOAL := help

GO ?= go
PNPM ?= pnpm
DOCKER_COMPOSE ?= docker compose
DEV_PROJECT ?= gazes-dev
DEV_COMPOSE = $(DOCKER_COMPOSE) -p $(DEV_PROJECT) -f compose.yaml -f compose.dev.yaml
DEV_WAIT_TIMEOUT ?= 600
# Keep the torrent engine's SQLite runtime out of the diagnostic SQLite binary.
GO_TAGS ?= nosqlite

.PHONY: help deps build build-backend build-web dev-backend dev-web start-web \
	test test-backend test-race test-web lint lint-backend lint-web typecheck check \
	up up-admin down logs ps dev dev-down dev-logs dev-ps dev-restart dev-check dev-test dev-build

help: ## List available commands
	@awk 'BEGIN {FS = ":.*## "; printf "Usage: make <target>\n\n"} /^[a-zA-Z0-9_-]+:.*## / {printf "  %-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

deps: ## Download Go modules and install locked frontend dependencies
	$(GO) mod download
	$(PNPM) --dir web install --frozen-lockfile

build: build-backend build-web ## Build the backend binaries and frontend

build-backend: ## Build server and diagnostic CLI into bin/
	@mkdir -p bin
	$(GO) build -tags=$(GO_TAGS) -trimpath -o bin/gazes-server ./cmd/server
	$(GO) build -tags=$(GO_TAGS) -trimpath -o bin/gazes-logs ./cmd/logs

build-web: ## Build the frontend with webpack and subtitle assets
	$(PNPM) --dir web run build --webpack

dev-backend: ## Run the native backend (requires FFmpeg and ffprobe)
	$(GO) run -tags=$(GO_TAGS) ./cmd/server

dev-web: ## Run the frontend development server on port 4389
	$(PNPM) --dir web run dev

start-web: ## Serve the built frontend on port 4389
	$(PNPM) --dir web run start

test: test-backend test-web ## Run Go tests and frontend file-selection tests

test-backend: ## Run all Go tests
	$(GO) test -tags=$(GO_TAGS) ./...

test-race: ## Run backend tests with the race detector
	$(GO) test -tags=$(GO_TAGS) -race ./internal/... ./cmd/...

test-web: ## Run frontend file-selection tests without a browser
	$(PNPM) --dir web run test:files

lint: lint-backend lint-web ## Run Go vet and frontend ESLint

lint-backend: ## Run Go vet
	$(GO) vet -tags=$(GO_TAGS) ./...

lint-web: ## Run frontend ESLint
	$(PNPM) --dir web run lint

typecheck: ## Check frontend TypeScript types
	$(PNPM) --dir web exec tsc --noEmit

check: lint typecheck test ## Run lint, type checks, and tests

up: ## Build and start the Docker stack, waiting for health checks
	$(DOCKER_COMPOSE) up -d --build --wait

up-admin: ## Start Docker with loopback-only Prowlarr administration
	$(DOCKER_COMPOSE) -f compose.yaml -f compose.admin.yaml up -d --build --wait

down: ## Stop the Docker stack while preserving its volumes
	$(DOCKER_COMPOSE) down

logs: ## Follow Docker service logs
	$(DOCKER_COMPOSE) logs -f

ps: ## Show Docker service status
	$(DOCKER_COMPOSE) ps

dev: ## Build and start the Docker development environment on port 8080
	$(DEV_COMPOSE) up -d --build --wait --wait-timeout $(DEV_WAIT_TIMEOUT)

dev-down: ## Stop development containers and preserve dependency/data volumes
	$(DEV_COMPOSE) down

dev-logs: ## Follow development service logs
	$(DEV_COMPOSE) logs -f

dev-ps: ## Show development service status
	$(DEV_COMPOSE) ps

dev-restart: ## Rebuild and restart the backend after Go source changes
	$(DEV_COMPOSE) restart backend

dev-test: ## Run Go and frontend file-selection tests in development containers
	$(DEV_COMPOSE) exec backend go test ./...
	$(DEV_COMPOSE) exec web pnpm run test:files

dev-check: ## Run lint, type checks, and tests in development containers
	$(DEV_COMPOSE) exec backend go vet ./...
	$(DEV_COMPOSE) exec web pnpm run lint
	$(DEV_COMPOSE) exec web pnpm exec tsc --noEmit
	$(MAKE) dev-test

dev-build: ## Build production images using Docker
	$(DOCKER_COMPOSE) build backend web
