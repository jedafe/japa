//go:build integration

// Package db_test contains integration tests for the database pool.
//
// These tests require a running PostgreSQL instance and are excluded from the
// default `go test ./...` run. Execute them with:
//
//	go test -tags integration ./internal/db/...
//
// The integration build tag ensures CI can run unit tests without a database.
package db_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jedafe/japa/internal/db"
)

// TestNew_PoolCreation verifies that New creates working writer and reader
// pools and that Ping succeeds against a live database.
func TestNew_PoolCreation(t *testing.T) {
	dsn := os.Getenv("JC_DATABASE_URL")
	if dsn == "" {
		dsn = "postgresql://japa:japa@localhost:5432/japa?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	database, err := db.New(ctx, dsn)
	if err != nil {
		t.Fatalf("db.New returned unexpected error: %v", err)
	}
	defer database.Close()

	if database.Writer == nil {
		t.Fatal("Writer pool is nil")
	}
	if database.Reader == nil {
		t.Fatal("Reader pool is nil")
	}

	if err := database.Ping(ctx); err != nil {
		t.Fatalf("Ping returned unexpected error: %v", err)
	}
}

// TestClose_IsIdempotent verifies that calling Close multiple times does not
// panic or return an error.
func TestClose_IsIdempotent(t *testing.T) {
	dsn := os.Getenv("JC_DATABASE_URL")
	if dsn == "" {
		dsn = "postgresql://japa:japa@localhost:5432/japa?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	database, err := db.New(ctx, dsn)
	if err != nil {
		t.Fatalf("db.New returned unexpected error: %v", err)
	}

	database.Close()
	database.Close() // should not panic
}
