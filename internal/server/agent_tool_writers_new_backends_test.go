package server

import (
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jedafe/japa/internal/assessment"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/credentialchecker"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/languagetesthub"
	"github.com/jedafe/japa/internal/remittance"
	"github.com/jedafe/japa/internal/visatimeline"
)

// TestToolWriterNewBackendsFullCRUD verifies create, update and delete for the
// five dedicated tool types that were previously missing writable backends.
func TestToolWriterNewBackendsFullCRUD(t *testing.T) {
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
	settings := config.NewSettings(database, nil)

	var userID pgtype.UUID
	if err := pool.QueryRow(ctx, "SELECT id FROM users WHERE email = 'admin@japaconnect.com' LIMIT 1").Scan(&userID); err != nil {
		t.Fatalf("get user: %v", err)
	}

	var destSpaceID string
	if err := pool.QueryRow(ctx, "SELECT id FROM spaces WHERE level = 'country' LIMIT 1").Scan(&destSpaceID); err != nil {
		t.Fatalf("get space: %v", err)
	}

	svcSet := toolServiceSet{
		credentialProfession: credentialchecker.NewService(credentialchecker.NewStore(database), settings),
		languageTest:         languagetesthub.NewService(languagetesthub.NewStore(database), settings),
		processingStat:       visatimeline.NewService(visatimeline.NewStore(database), settings),
		readinessAssessment:  assessment.NewService(assessment.NewStore(database), settings),
		remittanceRate:       remittance.NewService(remittance.NewStore(database), settings),
		remittanceProvider:   remittance.NewService(remittance.NewStore(database), settings),
	}

	reg := newAgentToolsRegistry(pool, buildToolWriters(svcSet))

	suffix := strconv.FormatInt(time.Now().Unix(), 10)

	// Create a remittance provider so we can create a rate under it.
	providerRecord, err := reg.CreateTool(ctx, pgtype.UUID{}, userID, "remittance_provider", map[string]any{
		"name":          "Test Provider " + suffix,
		"slug":          "test-provider-" + suffix,
		"data_source":   "manual",
		"is_active":     true,
		"display_order": 0,
	})
	if err != nil {
		t.Fatalf("create remittance_provider: %v", err)
	}

	providerID, _ := providerRecord["id"].(string)
	if providerID == "" {
		t.Fatalf("remittance_provider create did not return an id")
	}

	defer pool.Exec(ctx, "DELETE FROM remittance_providers WHERE id = $1", providerID)

	tests := []struct {
		name         string
		toolType     string
		create       map[string]any
		update       map[string]any
		expectUpdate map[string]any
	}{
		{
			name:     "language_test",
			toolType: "language_test",
			create: map[string]any{
				"slug":            "test-language-" + suffix,
				"name":            "Test Language " + suffix,
				"official_url":    "https://example.com/test",
				"validity_months": 24,
			},
			update: map[string]any{
				"validity_months": 36,
			},
			expectUpdate: map[string]any{
				"validity_months": float64(36),
			},
		},
		{
			name:     "credential_profession",
			toolType: "credential_profession",
			create: map[string]any{
				"slug":     "test-profession-" + suffix,
				"name":     "Test Profession " + suffix,
				"category": "healthcare",
			},
			update: map[string]any{
				"category": "engineering",
			},
			expectUpdate: map[string]any{
				"category": "engineering",
			},
		},
		{
			name:     "processing_time",
			toolType: "processing_time",
			create: map[string]any{
				"origin_space_id":      destSpaceID,
				"destination_space_id": destSpaceID,
				"visa_category":        "work",
				"consulate":            "Lagos " + suffix,
				"period_start":         time.Now().AddDate(0, 0, -2).Format("2006-01-02"),
				"sample_count":         10,
				"avg_days":             45.0,
			},
			update: map[string]any{
				"avg_days": 50.0,
			},
			expectUpdate: map[string]any{
				"avg_days": 50.0,
			},
		},
		{
			name:     "readiness_assessment",
			toolType: "readiness_assessment",
			create: map[string]any{
				"name":            "Test Assessment " + suffix,
				"slug":            "test-assessment-" + suffix,
				"space_id":        destSpaceID,
				"submit_label":    "Submit",
				"success_message": "Done",
			},
			update: map[string]any{
				"name": "Updated Assessment " + suffix,
			},
			expectUpdate: map[string]any{
				"name": "Updated Assessment " + suffix,
			},
		},
		{
			name:     "remittance_rate",
			toolType: "remittance_rate",
			create: map[string]any{
				"provider_id":      providerID,
				"send_currency":    "USD",
				"receive_currency": "NGN",
				"rate":             1500.0,
				"fee_flat":         5.0,
			},
			update: map[string]any{
				"rate": 1550.0,
			},
			expectUpdate: map[string]any{
				"rate": 1550.0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record, err := reg.CreateTool(ctx, pgtype.UUID{}, userID, tt.toolType, tt.create)
			if err != nil {
				t.Fatalf("create %s: %v", tt.toolType, err)
			}

			id, _ := record["id"].(string)
			if id == "" {
				t.Fatalf("%s create did not return an id", tt.toolType)
			}

			updated, err := reg.UpdateTool(ctx, pgtype.UUID{}, userID, tt.toolType, id, tt.update)
			if err != nil {
				t.Fatalf("update %s: %v", tt.toolType, err)
			}

			for k, v := range tt.expectUpdate {
				if updated[k] != v {
					t.Fatalf("%s update field %s: expected %v, got %v", tt.toolType, k, v, updated[k])
				}
			}

			if err := reg.DeleteTool(ctx, pgtype.UUID{}, userID, tt.toolType, id); err != nil {
				t.Fatalf("delete %s: %v", tt.toolType, err)
			}
		})
	}
}
