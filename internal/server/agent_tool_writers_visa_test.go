package server

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/visaroutes"
)

func TestToolWriterVisaRouteCreate(t *testing.T) {
	dsn := os.Getenv("JC_DATABASE_URL")
	if dsn == "" {
		dsn = "postgresql://japa:japa@localhost:5432/japa?sslmode=disable"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to database: %v", err)
	}
	defer pool.Close()

	database := &db.DB{Writer: pool, Reader: pool}

	var tenantID pgtype.UUID
	if err := pool.QueryRow(ctx, "SELECT id FROM tenants LIMIT 1").Scan(&tenantID); err != nil {
		t.Fatalf("get tenant: %v", err)
	}

	var userID pgtype.UUID
	if err := pool.QueryRow(ctx, "SELECT id FROM users WHERE email = 'admin@japaconnect.com' LIMIT 1").Scan(&userID); err != nil {
		t.Fatalf("get user: %v", err)
	}

	var destSpaceID string
	if err := pool.QueryRow(ctx, "SELECT id FROM spaces WHERE level = 'country' LIMIT 1").Scan(&destSpaceID); err != nil {
		t.Fatalf("get space: %v", err)
	}

	store := visaroutes.NewStore(database)
	visaSvc := visaroutes.NewService(store, nil)

	reg := newAgentToolsRegistry(pool, buildToolWriters(toolServiceSet{visaRoute: visaSvc}))

	payload := map[string]any{
		"destination_space_id": destSpaceID,
		"visa_category":        "work",
		"name":                 fmt.Sprintf("Test work visa %s", destSpaceID),
		"slug":                 fmt.Sprintf("test-work-visa-%s", destSpaceID),
		"gov_source_url":       "https://example.com/visa",
		"requirements":         []map[string]any{},
	}

	record, err := reg.CreateTool(ctx, tenantID, userID, "visa_route", payload)
	if err != nil {
		t.Fatalf("create visa_route: %v", err)
	}

	if record["name"] != payload["name"] {
		t.Fatalf("expected name %v, got %v", payload["name"], record["name"])
	}

	// Clean up.
	if id, ok := record["id"].(string); ok && id != "" {
		if _, err := pool.Exec(ctx, "DELETE FROM visa_routes WHERE id = $1", id); err != nil {
			t.Logf("cleanup failed: %v", err)
		}
	}
}
