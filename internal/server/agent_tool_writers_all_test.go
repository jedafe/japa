package server

import (
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jedafe/japa/internal/appointmentalert"
	"github.com/jedafe/japa/internal/arrivalchecklist"
	"github.com/jedafe/japa/internal/assessment"
	"github.com/jedafe/japa/internal/attestationguide"
	"github.com/jedafe/japa/internal/calendar"
	"github.com/jedafe/japa/internal/communitylocator"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/credentialchecker"
	"github.com/jedafe/japa/internal/cvformatter"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/digest"
	"github.com/jedafe/japa/internal/housingguide"
	"github.com/jedafe/japa/internal/languagetesthub"
	"github.com/jedafe/japa/internal/mentorship"
	"github.com/jedafe/japa/internal/mutualaid"
	"github.com/jedafe/japa/internal/planninggroups"
	"github.com/jedafe/japa/internal/remittance"
	"github.com/jedafe/japa/internal/scamalert"
	"github.com/jedafe/japa/internal/scholarships"
	"github.com/jedafe/japa/internal/taxcalculator"
	"github.com/jedafe/japa/internal/travel"
	"github.com/jedafe/japa/internal/visaroutes"
	"github.com/jedafe/japa/internal/visatimeline"
	"github.com/jedafe/japa/internal/wiki"
)

func TestToolWriterCreateAll(t *testing.T) {
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
		scholarship:          scholarships.NewStore(database),
		visaRoute:            visaroutes.NewService(visaroutes.NewStore(database), settings),
		housingGuide:         housingguide.NewService(housingguide.NewStore(database), settings),
		attestationGuide:     attestationguide.NewService(attestationguide.NewStore(database), settings),
		arrivalChecklist:     arrivalchecklist.NewService(arrivalchecklist.NewStore(database), settings),
		credentialPathway:    credentialchecker.NewService(credentialchecker.NewStore(database), settings),
		credentialProfession: credentialchecker.NewService(credentialchecker.NewStore(database), settings),
		languageTestReq:      languagetesthub.NewService(languagetesthub.NewStore(database), settings),
		taxObligation:        taxcalculator.NewService(taxcalculator.NewStore(database), settings),
		communityOrg:         communitylocator.NewStore(database),
		wikiPage:             wiki.NewStore(database),
		calendarEntry:        calendar.NewStore(database),
		scamReport:           scamalert.NewStore(database),
		planningGroup:        planninggroups.NewService(planninggroups.NewStore(database), settings),
		visaTimeline:         visatimeline.NewService(visatimeline.NewStore(database), settings),
		digestEdition:        digest.NewStore(database),
		mutualAid:            mutualaid.NewService(mutualaid.NewStore(database), settings),
		embassy:              appointmentalert.NewService(appointmentalert.NewStore(database), settings),
		appointmentAlert:     appointmentalert.NewService(appointmentalert.NewStore(database), settings),
		cvFormatter:          cvformatter.NewStore(database),
		languageTest:         languagetesthub.NewService(languagetesthub.NewStore(database), settings),
		mentorship:           mentorship.NewService(database),
		processingStat:       visatimeline.NewService(visatimeline.NewStore(database), settings),
		remittanceRate:       remittance.NewService(remittance.NewStore(database), settings),
		remittanceProvider:   remittance.NewService(remittance.NewStore(database), settings),
		readinessAssessment:  assessment.NewService(assessment.NewStore(database), settings),
		taxTreatyPair:        taxcalculator.NewStore(database),
		travelProvider:       travel.NewService(travel.NewStore(database), settings),
	}

	reg := newAgentToolsRegistry(pool, buildToolWriters(svcSet))

	results := make(map[string]error)

	for _, toolType := range reg.ToolTypes() {
		payload := map[string]any{
			"name":                 "test record",
			"destination_space_id": destSpaceID,
			"accommodation_type":   "apartment",
			"visa_category":        "work",
			"gov_source_url":       "https://example.com/visa",
			"requirements":         []map[string]any{},
			"search_platforms":     []map[string]any{},
			"rent_ranges":          map[string]any{},
			"scam_categories":      []string{},
		}
		_, err := reg.CreateTool(ctx, tenantID, userID, toolType, payload)
		results[toolType] = err
	}

	for toolType, err := range results {
		if err == nil {
			t.Logf("%s: create succeeded", toolType)
		} else {
			t.Logf("%s: create error: %v", toolType, err)
		}
	}

	// At this stage the test is informational: log all results but do not
	// fail so we can see which tools still need custom writer adapters.
	fmt.Printf("tool writer results: %v\n", results)
}
