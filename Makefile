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
	secrets redis-secret redis-up redis-down up up-admin down logs ps dev dev-down dev-logs dev-ps dev-restart dev-check dev-test dev-build library-install-host library-label-disk

help: ## List available commands
	@awk 'BEGIN {FS = ":.*## "; printf "Usage: make <target>\n\n"} /^[a-zA-Z0-9_-]+:.*## / {printf "  %-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

secrets: ## Generate account encryption keys into .env (kept if already set)
	@touch .env
	@for k in ACCOUNTS_ENC_KEY ACCOUNTS_INDEX_KEY ACCOUNTS_PEPPER ALTCHA_HMAC_KEY; do \
		grep -q "^$$k=" .env || echo "$$k=$$(openssl rand -base64 32)" >> .env; \
	done
	@$(MAKE) --no-print-directory redis-secret
	@echo ".env now holds the account keys. Back it up: losing them makes stored emails unreadable."

redis-secret: ## Generate REDIS_PASSWORD into .env (kept if already set)
	@touch .env
	@grep -q "^REDIS_PASSWORD=" .env || echo "REDIS_PASSWORD=$$(openssl rand -hex 24)" >> .env

redis-up: redis-secret ## Start the Redis shared by every stack (creates the gazes-shared network)
	@docker network inspect gazes-shared >/dev/null 2>&1 || docker network create gazes-shared >/dev/null
	$(DOCKER_COMPOSE) -f compose.redis.yaml up -d --wait

redis-down: ## Stop the shared Redis (its data volume is kept)
	$(DOCKER_COMPOSE) -f compose.redis.yaml down

deps: ## Download Go modules and install locked frontend dependencies
	$(GO) mod download
	$(PNPM) --dir web install --frozen-lockfile

build: build-backend build-web ## Build the backend binaries and frontend

build-backend: ## Build server and CLIs into bin/
	@mkdir -p bin
	$(GO) build -tags=$(GO_TAGS) -trimpath -o bin/gazes-server ./cmd/server
	$(GO) build -tags=$(GO_TAGS) -trimpath -o bin/gazes-logs ./cmd/logs
	$(GO) build -tags=$(GO_TAGS) -trimpath -o bin/gazes-library ./cmd/library

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

up: secrets redis-up ## Build and start the Docker stack, waiting for health checks
	$(DOCKER_COMPOSE) up -d --build --wait

up-admin: secrets redis-up ## Start Docker with loopback-only Prowlarr administration
	$(DOCKER_COMPOSE) -f compose.yaml -f compose.admin.yaml up -d --build --wait

down: ## Stop the Docker stack while preserving its volumes
	$(DOCKER_COMPOSE) down

logs: ## Follow Docker service logs
	$(DOCKER_COMPOSE) logs -f

ps: ## Show Docker service status
	$(DOCKER_COMPOSE) ps

dev: redis-up ## Build and start the Docker development environment on port 8080
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

library-install-host: ## Install the AV1 library host setup: /mnt/gazes, udev rule, mount units (sudo)
	sudo deploy/library/install-host.sh

library-label-disk: ## Label a disk for the library: make library-label-disk DEV=/dev/sdX1 LABEL=GAZES-1 (sudo)
	@test -n "$(DEV)" -a -n "$(LABEL)" || { echo "usage: make library-label-disk DEV=/dev/sdX1 LABEL=GAZES-1"; exit 2; }
	sudo deploy/library/label-disk.sh "$(DEV)" "$(LABEL)"
