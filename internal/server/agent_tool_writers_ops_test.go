package server

import (
	"testing"

	"github.com/jedafe/japa/internal/appointmentalert"
	"github.com/jedafe/japa/internal/arrivalchecklist"
	"github.com/jedafe/japa/internal/assessment"
	"github.com/jedafe/japa/internal/attestationguide"
	"github.com/jedafe/japa/internal/calendar"
	"github.com/jedafe/japa/internal/communitylocator"
	"github.com/jedafe/japa/internal/credentialchecker"
	"github.com/jedafe/japa/internal/crscalculator"
	"github.com/jedafe/japa/internal/cvformatter"
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

// buildTestToolServiceSet mirrors the production toolServiceSet wiring in
// server.go. Empty struct pointers are fine — op resolution only inspects
// the method set, never calls the methods.
func buildTestToolServiceSet() toolServiceSet {
	return toolServiceSet{
		scholarship:             &scholarships.Store{},
		visaRoute:               &visaroutes.Service{},
		housingGuide:            &housingguide.Service{},
		attestationGuide:        &attestationguide.Service{},
		arrivalChecklist:        &arrivalchecklist.Service{},
		credentialPathway:       &credentialchecker.Service{},
		credentialProfession:    &credentialchecker.Service{},
		languageTestReq:         &languagetesthub.Service{},
		taxObligation:           &taxcalculator.Service{},
		communityOrg:            &communitylocator.Store{},
		wikiPage:                &wiki.Store{},
		calendarEntry:           &calendar.Store{},
		scamReport:              &scamalert.Store{},
		planningGroup:           &planninggroups.Service{},
		visaTimeline:            &visatimeline.Service{},
		digestEdition:           &digest.Store{},
		mutualAid:               &mutualaid.Service{},
		embassy:                 &appointmentalert.Service{},
		appointmentAlert:        &appointmentalert.Service{},
		cvFormatter:             &cvformatter.Store{},
		languageTest:            &languagetesthub.Service{},
		mentorship:              &mentorship.Service{},
		processingStat:          &visatimeline.Service{},
		remittanceRate:          &remittance.Service{},
		remittanceProvider:      &remittance.Service{},
		readinessAssessment:     &assessment.Service{},
		taxTreatyPair:           &taxcalculator.Store{},
		travelProvider:          &travel.Service{},
		remittanceProviderStore: &remittance.Store{},
		travelProviderStore:     &travel.Store{},
		crsCalculator:           &crscalculator.Service{},
		pofCalculator:           &pofcalculator.Service{},
		sopGenerator:            &sopgenerator.Service{},
		ecaGuide:                &ecaguide.Service{},
	}
}

// TestToolWriterOpsMatrix locks the per-op resolution matrix: every catalog
// type must resolve exactly the seeded <tool_type>.<op> ai_tools rows. A
// renamed or removed service method silently disables an op, which would
// leave a catalog-advertised dead tool — this test catches that drift.
func TestToolWriterOpsMatrix(t *testing.T) {
	reg := newAgentToolsRegistry(nil, buildToolWriters(buildTestToolServiceSet()))
	ops := reg.WritableToolOps()

	cud := map[string]bool{"create": true, "update": true, "delete": true}
	cuda := map[string]bool{"create": true, "update": true, "delete": true, "assign_geo": true}

	want := map[string]map[string]bool{
		// Partial writers — seeded rows for the missing ops are disabled by
		// migration 000788. Re-enable only when a backing method lands.
		"attestation_guide":     {"create": true},
		"embassy":               {"create": true, "update": true},
		"housing_guide":         {"create": true, "update": true},
		"mentorship_connection": {"create": true},
		"scam_report":           {"create": true, "update": true, "assign_geo": true},
		"wiki_page":             cud,

		// assign_geo unresolvable: the update input has no geo-scope field,
		// so the op would silently no-op. Seeded assign_geo rows disabled.
		"appointment_alert":     cud,
		"credential_profession": cud,
		"crs_draw":              cud,
		"crs_ruleset":           cud,
		"eca_provider":          cud,
		"language_test":         cud,
		"mutual_aid":            cud,
		"remittance_provider":   cud,
		"remittance_rate":       cud,
		"sop_field":             cud,
		"tax_obligation_guide":  cud,
		"tax_treaty_pair":       cud,
		"travel_provider":       cud,
		"visa_timeline":         cud,

		// Read-only catalog types — no write ops at all.
		"fx_rate": {},
	}

	for _, toolType := range reg.ToolTypes() {
		expected, ok := want[toolType]
		if !ok {
			expected = cuda
		}

		got := ops[toolType]
		if got == nil {
			got = map[string]bool{}
		}

		for _, op := range []string{"create", "update", "delete", "assign_geo"} {
			if expected[op] && !got[op] {
				t.Errorf("%s.%s: expected a resolved writer op, got none", toolType, op)
			}

			if !expected[op] && got[op] {
				t.Errorf("%s.%s: writer op resolved but seeded row is disabled — re-enable the seed", toolType, op)
			}
		}
	}
}
