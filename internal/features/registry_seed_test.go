package features

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// seedDefaultOverrides documents every flag where the 000735 baseline seed
// enables a flag that the Registry defaults to disabled. The seed file is a
// snapshot of a live production database — its values reflect operator
// choices at dump time, not fresh-install intent. The precedence contract is:
//
//   - Operator installs (seeded DB): the feature_flags row exists and wins —
//     Registry.Default never applies, so these features stay enabled.
//   - Fresh installs (no row): Registry.Default applies — these V2/opt-in
//     features start off and are enabled from /control/extensions.
//
// This is deliberate divergence, not drift. Migration 000811 additionally
// backfills explicit rows from legacy system_settings so operator state is
// always preserved in feature_flags regardless of the baseline seed.
//
// Adding an entry here requires a code comment explaining why the fresh-
// install default diverges from the seeded operator state. An entry for a
// key that no longer diverges (seed updated or default changed) fails the
// test so the list cannot go stale.
var seedDefaultOverrides = map[string]string{
	// V2 community features — enabled on the seeded production DB, but
	// opt-in (Default: false) for fresh installs.
	"achievement.enabled":        "V2 community feature — opt-in for fresh installs",
	"calendar.enabled":           "V2 community feature — opt-in for fresh installs",
	"digest.enabled":             "V2 community feature — opt-in for fresh installs",
	"forum.ama.enabled":          "V2 community feature — opt-in for fresh installs",
	"japa_stage.enabled":         "V2 community feature — opt-in for fresh installs",
	"mutual_aid.enabled":         "V2 community feature — opt-in for fresh installs",
	"planning_group.enabled":     "V2 community feature — opt-in for fresh installs",
	"scam_alert.enabled":         "V2 community feature — opt-in for fresh installs",
	"story.enabled":              "V2 community feature — opt-in for fresh installs",
	"wiki.enabled":               "V2 community feature — opt-in for fresh installs",
	"wiki.structured_edits":      "depends on wiki.enabled — opt-in for fresh installs",
	"service_directory.enabled":  "V2 community feature — opt-in for fresh installs",
	"mentorship.enabled":         "V2 community feature — opt-in for fresh installs",
	"mentorship.comparison":      "V2 community feature — opt-in for fresh installs",
	"visa_timeline.enabled":      "V2 tool — opt-in for fresh installs",
	"visa_routes.enabled":        "V2 tool — opt-in for fresh installs",
	"appointment_alert.enabled":  "V2 tool — opt-in for fresh installs",
	"arrival_checklist.enabled":  "V2 tool — opt-in for fresh installs",
	"credential_checker.enabled": "V2 tool — opt-in for fresh installs",
	"cv_formatter.enabled":       "V2 tool — opt-in for fresh installs",
	"housing_guide.enabled":      "V2 tool — opt-in for fresh installs",
	"attestation_guide.enabled":  "V2 tool — opt-in for fresh installs",
	"language_test_hub.enabled":  "V2 tool — opt-in for fresh installs",
	"tax_calculator.enabled":     "V2 tool — opt-in for fresh installs",
	"community_locator.enabled":  "V2 tool — opt-in for fresh installs",
	// Commerce/AI/integrations enabled in production but off for fresh
	// installs — they require credentials or operator setup to be useful.
	"feature.affiliate_program": "requires affiliate configuration",
	"feature.ai_content_gen":    "requires AI provider credentials",
	"feature.japabot":           "requires AI provider credentials",
	"feature.newsletter":        "requires mail delivery configuration",
	"feature.referral":          "requires referral program configuration",
	"feature.wallet":            "requires payment/credit configuration",
	"rag.enabled":               "requires AI provider credentials",
	"studio.enabled":            "requires AI provider credentials",
	"participation.enabled":     "requires AI provider credentials",
	"webhooks.enabled":          "requires webhook endpoint configuration",
}

// seedFlagRowRe matches the baseline seed's INSERT shape:
//
//	INSERT INTO public.feature_flags VALUES ('key', true, ...)
var seedFlagRowRe = regexp.MustCompile(`INSERT INTO public\.feature_flags VALUES \('([^']+)', (true|false)`)

// TestRegistryMatchesBaselineSeed pins the divergence contract between the
// 000735 baseline seed (operator DB snapshot) and Registry.Default
// (fresh-install intent). For every key present in BOTH, the seeded value
// must equal the registry default unless the key is allowlisted in
// seedDefaultOverrides. Undocumented divergence and stale allowlist entries
// both fail, so seed-vs-registry drift can only grow deliberately.
func TestRegistryMatchesBaselineSeed(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../../migrations/000735_baseline_seed.up.sql")
	if err != nil {
		t.Fatalf("read baseline seed: %v", err)
	}

	seeded := map[string]bool{}
	for _, m := range seedFlagRowRe.FindAllStringSubmatch(string(raw), -1) {
		seeded[m[1]] = m[2] == "true"
	}

	if len(seeded) == 0 {
		t.Fatal("no feature_flags rows parsed from baseline seed — test may be broken")
	}

	var undocumented []string

	for _, f := range Registry {
		seedEnabled, ok := seeded[f.Key]
		if !ok {
			continue // not seeded — registry default governs fresh installs
		}

		if seedEnabled == f.Default {
			continue // in agreement
		}

		if _, allowed := seedDefaultOverrides[f.Key]; !allowed {
			undocumented = append(undocumented, f.Key)
		}
	}

	if len(undocumented) > 0 {
		sort.Strings(undocumented)
		t.Errorf("registry defaults diverge from baseline seed without a documented override: %s\n"+
			"either align the default or add the key to seedDefaultOverrides with a rationale",
			strings.Join(undocumented, ", "))
	}

	// Stale overrides: every entry must actually describe a divergence.
	for key := range seedDefaultOverrides {
		f, ok := RegistryLookup(key)
		if !ok {
			t.Errorf("seedDefaultOverrides[%q]: key not in Registry", key)
			continue
		}

		if seedEnabled, seeded := seeded[key]; seeded && seedEnabled == f.Default {
			t.Errorf("seedDefaultOverrides[%q]: no longer diverges (seed=%v, default=%v) — remove the entry",
				key, seedEnabled, f.Default)
		}
	}
}

// TestRegistryDefaultSurfacesValid guards the eval path: DefaultSurfaces is
// now applied by surfaceRuleFor for flags without a stored surface rule, so
// a typo'd surface name or state would silently resolve as visible. Every
// entry must use a known surface and a valid state.
func TestRegistryDefaultSurfacesValid(t *testing.T) {
	t.Parallel()

	for _, f := range Registry {
		for surface, state := range f.DefaultSurfaces {
			if !ValidSurface(surface) {
				t.Errorf("%s: DefaultSurfaces surface %q is not a known client surface", f.Key, surface)
			}

			switch state {
			case "visible", "hidden", "disabled":
			default:
				t.Errorf("%s: DefaultSurfaces[%q] state %q is invalid (visible|hidden|disabled)", f.Key, surface, state)
			}
		}
	}
}
