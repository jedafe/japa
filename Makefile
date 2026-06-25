.PHONY: build run test migrate docker-up docker-down sqlc-gen lint clean

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
	golangci-lint run ./...

# Remove build artifacts
clean:
	rm -rf bin/
