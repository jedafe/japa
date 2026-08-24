package server

import (
	"context"
	"fmt"
	"os"
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

// TestToolWriterNewBackendsCreate verifies the five previously unwritable
// dedicated tool types can now be created through the agent tool registry.
func TestToolWriterNewBackendsCreate(t *testing.T) {
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
	settings := config.NewSettings(database, nil)

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

	svcSet := toolServiceSet{
		credentialProfession: credentialchecker.NewService(credentialchecker.NewStore(database), settings),
		languageTest:         languagetesthub.NewService(languagetesthub.NewStore(database), settings),
		processingStat:       visatimeline.NewService(visatimeline.NewStore(database), settings),
		readinessAssessment:  assessment.NewService(assessment.NewStore(database), settings),
		remittanceRate:       remittance.NewService(remittance.NewStore(database), settings),
		remittanceProvider:   remittance.NewService(remittance.NewStore(database), settings),
	}

	reg := newAgentToolsRegistry(pool, buildToolWriters(svcSet))

	suffix := fmt.Sprintf("%d", time.Now().Unix())

	// Create a remittance provider so we can create a rate under it.
	providerRecord, err := reg.CreateTool(ctx, tenantID, userID, "remittance_provider", map[string]any{
		"name":          fmt.Sprintf("Test Provider %s", suffix),
		"slug":          fmt.Sprintf("test-provider-%s", suffix),
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
		name     string
		toolType string
		payload  map[string]any
		table    string
	}{
		{
			name:     "language_test",
			toolType: "language_test",
			payload: map[string]any{
				"slug":            fmt.Sprintf("test-language-%s", suffix),
				"name":            fmt.Sprintf("Test Language %s", suffix),
				"official_url":    "https://example.com/test",
				"validity_months": 24,
			},
			table: "language_tests",
		},
		{
			name:     "credential_profession",
			toolType: "credential_profession",
			payload: map[string]any{
				"slug":     fmt.Sprintf("test-profession-%s", suffix),
				"name":     fmt.Sprintf("Test Profession %s", suffix),
				"category": "healthcare",
			},
			table: "credential_professions",
		},
		{
			name:     "processing_time",
			toolType: "processing_time",
			payload: map[string]any{
				"origin_space_id":      destSpaceID,
				"destination_space_id": destSpaceID,
				"visa_category":        "work",
				"consulate":            fmt.Sprintf("Lagos %s", suffix),
				"period_start":         time.Now().AddDate(0, 0, -2).Format("2006-01-02"),
				"sample_count":         10,
				"avg_days":             45.0,
			},
			table: "visa_processing_stats",
		},
		{
			name:     "readiness_assessment",
			toolType: "readiness_assessment",
			payload: map[string]any{
				"name":            fmt.Sprintf("Test Assessment %s", suffix),
				"slug":            fmt.Sprintf("test-assessment-%s", suffix),
				"space_id":        destSpaceID,
				"submit_label":    "Submit",
				"success_message": "Done",
			},
			table: "forms",
		},
		{
			name:     "remittance_rate",
			toolType: "remittance_rate",
			payload: map[string]any{
				"provider_id":      providerID,
				"send_currency":    "USD",
				"receive_currency": "NGN",
				"rate":             1500.0,
				"fee_flat":         5.0,
			},
			table: "remittance_rates",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record, err := reg.CreateTool(ctx, tenantID, userID, tt.toolType, tt.payload)
			if err != nil {
				t.Fatalf("create %s: %v", tt.toolType, err)
			}
			id, _ := record["id"].(string)
			if id == "" {
				t.Fatalf("%s create did not return an id", tt.toolType)
			}

			switch tt.toolType {
			case "language_test", "credential_profession", "processing_time":
				if err := reg.DeleteTool(ctx, tenantID, userID, tt.toolType, id); err != nil {
					t.Logf("cleanup delete %s: %v", tt.toolType, err)
				}
			default:
				if _, err := pool.Exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE id = $1", tt.table), id); err != nil {
					t.Logf("cleanup failed: %v", err)
				}
			}
		})
	}
}
