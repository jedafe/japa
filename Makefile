.PHONY: build run test test-integration migrate docker-up docker-down sqlc-gen lint lint-fix lint-changed fmt vet check clean

# Go binary path (for tools installed via go install)
GOBIN := $(shell go env GOPATH)/bin

# Build the API binary into bin/japa-api
build:
	go build -o bin/japa-api ./cmd/api/

# Run the API locally (requires JC_ env vars or defaults)
run:
	go run ./cmd/api/

# Run all unit tests (excludes integration tests)
test:
	go test ./...

# Run integration tests (requires a running PostgreSQL)
test-integration:
	go test -tags integration ./...

# Run database migrations
migrate:
	go run ./cmd/migrate/

# Start all Docker services
docker-up:
	docker compose up -d

# Stop all Docker services
docker-down:
	docker compose down

# Generate Go code from SQL queries using sqlc
sqlc-gen:
	sqlc generate

# Run the linter
lint:
	$(GOBIN)/golangci-lint run ./...

# Auto-fix lint issues where possible
lint-fix:
	$(GOBIN)/golangci-lint run --fix ./...

# Run linter on changed files only (for pre-commit)
lint-changed:
	$(GOBIN)/golangci-lint run --new-from-rev=HEAD~1 ./...

# Format code
fmt:
	$(GOBIN)/golangci-lint fmt ./...

# Vet the code
vet:
	go vet ./...

# Run all checks (build, vet, lint, test)
check: build vet lint test

# Remove build artifacts
clean:
	rm -rf bin/
