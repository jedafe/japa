package server

import (
	"fmt"
	"os"
	"strings"
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
	"github.com/jedafe/japa/internal/crscalculator"
	"github.com/jedafe/japa/internal/cvformatter"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/digest"
	"github.com/jedafe/japa/internal/ecaguide"
	"github.com/jedafe/japa/internal/housingguide"
	"github.com/jedafe/japa/internal/languagetesthub"
	"github.com/jedafe/japa/internal/mentorship"
	"github.com/jedafe/japa/internal/mutualaid"
	"github.com/jedafe/japa/internal/planninggroups"
	"github.com/jedafe/japa/internal/pofcalculator"
	"github.com/jedafe/japa/internal/remittance"
	"github.com/jedafe/japa/internal/scamalert"
	"github.com/jedafe/japa/internal/scholarships"
	"github.com/jedafe/japa/internal/sopgenerator"
	"github.com/jedafe/japa/internal/taxcalculator"
	"github.com/jedafe/japa/internal/travel"
	"github.com/jedafe/japa/internal/visaroutes"
	"github.com/jedafe/japa/internal/visatimeline"
	"github.com/jedafe/japa/internal/wiki"
)

// resolutionErr matches errors that mean a writer op was registered without
// a real backing method — the drift class this test exists to catch. Service
// validation errors (missing fields, FK failures) are expected on the
// generic probe payload and are logged, not asserted.
func resolutionErr(err error) bool {
	if err == nil {
		return false
	}

	msg := err.Error()

	return strings.Contains(msg, "not found") ||
		strings.Contains(msg, "does not support") ||
		strings.Contains(msg, "has ") && strings.Contains(msg, "inputs") ||
		strings.Contains(msg, "has ") && strings.Contains(msg, "outputs")
}

// TestToolWriterLifecycle exercises every catalog type's resolved writer ops
// end-to-end against a real database: create → update → assign_geo → delete,
// as permitted by WritableToolOps. Field-validation failures on the generic
// probe payload are logged (payload coverage is per-type); resolution
// failures are hard errors — an op that cannot reach its backing method is
// catalog drift.
func TestToolWriterLifecycle(t *testing.T) {
	dsn := os.Getenv("JC_DATABASE_URL")
	if dsn == "" {
		dsn = "postgresql://japa:japa@localhost:5432/japa?sslmode=disable" // #nosec
	}

	ctx := t.Context()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("database unavailable, skipping writer lifecycle test: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Skipf("database unreachable, skipping writer lifecycle test: %v", err)
	}

	database := &db.DB{Writer: pool, Reader: pool}
	settings := config.NewSettings(database, nil)

	var userID pgtype.UUID
	if err := pool.QueryRow(ctx, "SELECT id FROM users WHERE email = 'admin@japa.to' LIMIT 1").Scan(&userID); err != nil {
		t.Skipf("seed admin user not found, skipping: %v", err)
	}

	var destSpaceID string
	if err := pool.QueryRow(ctx, "SELECT id FROM spaces WHERE level = 'country' LIMIT 1").Scan(&destSpaceID); err != nil {
		t.Skipf("seed country space not found, skipping: %v", err)
	}

	svcSet := toolServiceSet{
		scholarship:             scholarships.NewStore(database),
		visaRoute:               visaroutes.NewService(visaroutes.NewStore(database), settings),
		housingGuide:            housingguide.NewService(housingguide.NewStore(database), settings),
		attestationGuide:        attestationguide.NewService(attestationguide.NewStore(database), settings),
		arrivalChecklist:        arrivalchecklist.NewService(arrivalchecklist.NewStore(database), settings),
		credentialPathway:       credentialchecker.NewService(credentialchecker.NewStore(database), settings),
		credentialProfession:    credentialchecker.NewService(credentialchecker.NewStore(database), settings),
		languageTestReq:         languagetesthub.NewService(languagetesthub.NewStore(database), settings),
		taxObligation:           taxcalculator.NewService(taxcalculator.NewStore(database), settings),
		communityOrg:            communitylocator.NewStore(database),
		wikiPage:                wiki.NewStore(database),
		calendarEntry:           calendar.NewStore(database),
		scamReport:              scamalert.NewStore(database),
		planningGroup:           planninggroups.NewService(planninggroups.NewStore(database), settings),
		visaTimeline:            visatimeline.NewService(visatimeline.NewStore(database), settings),
		digestEdition:           digest.NewStore(database),
		mutualAid:               mutualaid.NewService(mutualaid.NewStore(database), settings),
		embassy:                 appointmentalert.NewService(appointmentalert.NewStore(database), settings),
		appointmentAlert:        appointmentalert.NewService(appointmentalert.NewStore(database), settings),
		cvFormatter:             cvformatter.NewStore(database),
		languageTest:            languagetesthub.NewService(languagetesthub.NewStore(database), settings),
		mentorship:              mentorship.NewService(database),
		processingStat:          visatimeline.NewService(visatimeline.NewStore(database), settings),
		remittanceRate:          remittance.NewService(remittance.NewStore(database), settings),
		remittanceProvider:      remittance.NewService(remittance.NewStore(database), settings),
		remittanceProviderStore: remittance.NewStore(database),
		travelProviderStore:     travel.NewStore(database),
		readinessAssessment:     assessment.NewService(assessment.NewStore(database), settings),
		taxTreatyPair:           taxcalculator.NewStore(database),
		travelProvider:          travel.NewService(travel.NewStore(database), settings),
		crsCalculator:           crscalculator.NewService(crscalculator.NewStore(database), settings),
		pofCalculator:           pofcalculator.NewService(pofcalculator.NewStore(database), settings),
		sopGenerator:            sopgenerator.NewService(sopgenerator.NewStore(database), settings),
		ecaGuide:                ecaguide.NewService(ecaguide.NewStore(database), settings),
	}

	reg := newAgentToolsRegistry(pool, buildToolWriters(svcSet))
	ops := reg.WritableToolOps()

	payload := map[string]any{
		"name":                 "writer lifecycle probe",
		"slug":                 "writer-lifecycle-probe",
		"title":                "writer lifecycle probe",
		"destination_space_id": destSpaceID,
		"mentor_id":            userID.String(),
		"mentee_id":            userID.String(),
		"accommodation_type":   "apartment",
		"visa_category":        "work",
		"gov_source_url":       "https://example.com/visa",
		"requirements":         []map[string]any{},
		"search_platforms":     []map[string]any{},
		"rent_ranges":          map[string]any{},
		"scam_categories":      []string{},
	}

	for _, toolType := range reg.ToolTypes() {
		supported := ops[toolType]
		if len(supported) == 0 {
			continue
		}

		if !supported["create"] {
			t.Logf("%s: no create op — update/delete require a created record; skipped", toolType)
			continue
		}

		record, err := reg.CreateTool(ctx, userID, toolType, payload)
		if err != nil {
			if resolutionErr(err) {
				t.Errorf("%s.create: resolution failure: %v", toolType, err)
				continue
			}

			t.Logf("%s.create: service error (payload coverage): %v", toolType, err)

			continue
		}

		id, _ := record["id"].(string)
		if id == "" {
			t.Logf("%s.create: succeeded but returned no id; update/delete skipped", toolType)
			continue
		}

		if supported["update"] {
			if _, err := reg.UpdateTool(ctx, userID, toolType, id, payload); resolutionErr(err) {
				t.Errorf("%s.update: resolution failure: %v", toolType, err)
			} else if err != nil {
				t.Logf("%s.update: service error: %v", toolType, err)
			}
		}

		if supported["assign_geo"] {
			if _, err := reg.AssignGeo(ctx, userID, toolType, id, payload); resolutionErr(err) {
				t.Errorf("%s.assign_geo: resolution failure: %v", toolType, err)
			} else if err != nil {
				t.Logf("%s.assign_geo: service error: %v", toolType, err)
			}
		}

		if supported["delete"] {
			if err := reg.DeleteTool(ctx, userID, toolType, id); resolutionErr(err) {
				t.Errorf("%s.delete: resolution failure: %v", toolType, err)
			} else if err != nil {
				t.Logf("%s.delete: service error: %v", toolType, err)
			}
		}
	}

	fmt.Println("writer lifecycle complete")
}
