# Common tasks. On Windows these also work from Git Bash or WSL.

BINARY := bin/server
DATABASE_URL ?= postgres://claims:claims@localhost:5432/claims?sslmode=disable
PRODUCT_ID ?= cfe6aa75-5da8-44f5-b587-56857841ad9f

.PHONY: help run run-postgres build test test-verbose fmt vet media db-up db-down db-psql demo tidy

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-14s %s\n", $$1, $$2}'

run: ## Run the service with the in-memory store
	go run ./cmd/server

run-postgres: ## Run the service against PostgreSQL
	DATABASE_URL="$(DATABASE_URL)" go run ./cmd/server

build: ## Build the server binary
	go build -o $(BINARY) ./cmd/server

test: ## Run all tests
	go test ./...

test-verbose: ## Run all tests with per-test output
	go test -v ./...

fmt: ## Format the code
	gofmt -w .

vet: ## Run go vet
	go vet ./...

tidy: ## Tidy module dependencies
	go mod tidy

media: ## Regenerate local placeholder product images
	go run ./cmd/genmedia

db-up: ## Start PostgreSQL in Docker
	docker compose up -d

db-down: ## Stop PostgreSQL and remove its volume
	docker compose down -v

db-psql: ## Open a psql shell against the local database
	docker compose exec postgres psql -U claims -d claims

demo: ## Trigger a run against a locally running service and print the claims
	@curl -s -X POST http://localhost:8080/claims/identify \
		-H 'Content-Type: application/json' \
		-d '{"productId":"$(PRODUCT_ID)"}'
	@echo
