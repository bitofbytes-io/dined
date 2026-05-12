.DEFAULT_GOAL := help
.PHONY: help dev run run-postgres build test migrate migrate-down migrate-status tail-prod docker-build docker-buildx clean

BIN_DIR ?= bin
PORT ?= 4600
APP_ENV ?= development
DATA_STORE ?= memory
API_TOKEN ?= dined
GOOGLE_PLACES_API_KEY ?=
export DATABASE_URL

REGISTRY ?= registry.bitofbytes.io
IMAGE_REPO ?= dined
PLATFORMS ?= linux/arm64/v8
TAG ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)

-include local.mk

dev: tail-prod ## Run local visual preview with memory storage and no database
	APP_ENV=development DATA_STORE=memory PORT=$(PORT) API_TOKEN=$(API_TOKEN) GOOGLE_PLACES_API_KEY="$(GOOGLE_PLACES_API_KEY)" SECURE_COOKIES=false go run ./cmd/dined

run: dev ## Alias for local preview

run-postgres: tail-prod ## Run locally against Postgres
	@test -n '$(DATABASE_URL)' || (echo "DATABASE_URL must be set in local.mk or the environment" >&2; exit 1)
	APP_ENV=$(APP_ENV) DATA_STORE=postgres PORT=$(PORT) DATABASE_URL='$(DATABASE_URL)' API_TOKEN=$(API_TOKEN) GOOGLE_PLACES_API_KEY="$(GOOGLE_PLACES_API_KEY)" SECURE_COOKIES=false go run ./cmd/dined

build: tail-prod ## Build the production binary
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/dined ./cmd/dined

tail-prod: ## Build static CSS
	cp tailwind/styles.css static/styles.css

migrate: ## Apply database migrations
	@test -n '$(DATABASE_URL)' || (echo "DATABASE_URL must be set in local.mk or the environment" >&2; exit 1)
	goose -dir migrations postgres '$(DATABASE_URL)' up

migrate-down: ## Roll back the last migration
	@test -n '$(DATABASE_URL)' || (echo "DATABASE_URL must be set in local.mk or the environment" >&2; exit 1)
	goose -dir migrations postgres '$(DATABASE_URL)' down

migrate-status: ## Show migration status
	@test -n '$(DATABASE_URL)' || (echo "DATABASE_URL must be set in local.mk or the environment" >&2; exit 1)
	goose -dir migrations postgres '$(DATABASE_URL)' status

test: ## Run Go tests
	go test -v ./...

docker-build: tail-prod ## Build the Docker image locally
	docker build -t $(REGISTRY)/$(IMAGE_REPO):$(TAG) .

docker-buildx: tail-prod ## Build and push multi-arch Docker image
	docker buildx build \
		--platform $(PLATFORMS) \
		--tag $(REGISTRY)/$(IMAGE_REPO):$(TAG) \
		--tag $(REGISTRY)/$(IMAGE_REPO):latest \
		--push \
		.

clean: ## Remove local build outputs
	rm -rf $(BIN_DIR)

help: ## Show this help menu
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make <target>\n\nTargets:\n"} /^[a-zA-Z0-9_.-]+:.*##/ {printf "  %-20s %s\n", $$1, $$2} END {printf "\n"}' $(MAKEFILE_LIST)
