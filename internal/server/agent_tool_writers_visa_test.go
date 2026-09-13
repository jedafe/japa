package server

import (
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/housingguide"
	"github.com/jedafe/japa/internal/visaroutes"
)

func TestToolWriterCreateSmoke(t *testing.T) {
	dsn := os.Getenv("JC_DATABASE_URL")
	if dsn == "" {
		dsn = "postgresql://japa:japa@localhost:5432/japa?sslmode=disable" // #nosec
	}

	ctx := t.Context()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to database: %v", err)
	}

	defer pool.Close()

	database := &db.DB{Writer: pool, Reader: pool}

	var userID pgtype.UUID
	if err := pool.QueryRow(ctx, "SELECT id FROM users WHERE email = 'admin@japaconnect.com' LIMIT 1").Scan(&userID); err != nil {
		t.Fatalf("get user: %v", err)
	}

	var destSpaceID string
	if err := pool.QueryRow(ctx, "SELECT id FROM spaces WHERE level = 'country' LIMIT 1").Scan(&destSpaceID); err != nil {
		t.Fatalf("get space: %v", err)
	}

	visaSvc := visaroutes.NewService(visaroutes.NewStore(database), nil)
	housingSvc := housingguide.NewService(housingguide.NewStore(database), nil)

	reg := newAgentToolsRegistry(pool, buildToolWriters(toolServiceSet{
		visaRoute:    visaSvc,
		housingGuide: housingSvc,
	}))

	tests := []struct {
		name      string
		toolType  string
		payload   map[string]any
		wantField string
		wantValue any
		table     string
	}{
		{
			name:     "visa_route",
			toolType: "visa_route",
			payload: map[string]any{
				"destination_space_id": destSpaceID,
				"visa_category":        "work",
				"name":                 "Test work visa " + destSpaceID,
				"slug":                 "test-work-visa-" + destSpaceID,
				"gov_source_url":       "https://example.com/visa",
				"requirements":         []map[string]any{},
			},
			wantField: "name",
			wantValue: "Test work visa " + destSpaceID,
			table:     "visa_routes",
		},
		{
			name:     "housing_guide",
			toolType: "housing_guide",
			payload: map[string]any{
				"destination_space_id": destSpaceID,
				"accommodation_type":   "apartment",
				"search_platforms":     []map[string]any{},
				"rent_ranges":          map[string]any{},
				"scam_categories":      []string{},
			},
			wantField: "accommodation_type",
			wantValue: "apartment",
			table:     "housing_guides",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record, err := reg.CreateTool(ctx, pgtype.UUID{}, userID, tt.toolType, tt.payload)
			if err != nil {
				t.Fatalf("create %s: %v", tt.toolType, err)
			}

			if record[tt.wantField] != tt.wantValue {
				t.Fatalf("expected %s %v, got %v", tt.wantField, tt.wantValue, record[tt.wantField])
			}

			if id, ok := record["id"].(string); ok && id != "" && tt.table != "" {
				if _, err := pool.Exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE id = $1", tt.table), id); err != nil {
					t.Logf("cleanup failed: %v", err)
				}
			}
		})
	}
}
