package nextactions

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/visitor"
)

func testRule(slug string, prio int32, audience []string, chans []string) Rule {
	return Rule{
		Slug: slug, Name: slug, Enabled: true, Priority: prio,
		Audience:  Audience{UserClass: audience},
		Payload:   Payload{Title: "T", CTAURL: "/x"},
		Channels:  chans,
		Frequency: Frequency{Mode: "recurring", MaxImpressions: 5, CooldownHours: 0},
	}
}

func TestStateFiltered(t *testing.T) {
	t.Parallel()
	svc := &Service{}
	rule := testRule("r", 1, []string{"regular"}, []string{"popup"})
	now := time.Now()

	cases := []struct {
		name string
		st   userActionState
		want bool
	}{
		{"pending passes", userActionState{Status: StatusPending}, false},
		{"dismissed filtered", userActionState{Status: StatusDismissed}, true},
		{"completed filtered", userActionState{Status: StatusCompleted}, true},
		{"expired filtered", userActionState{Status: StatusExpired}, true},
		{"snoozed future filtered", userActionState{Status: StatusSnoozed,
			SnoozedUntil: pgtype.Timestamptz{Time: now.Add(time.Hour), Valid: true}}, true},
		{"snoozed past passes", userActionState{Status: StatusSnoozed,
			SnoozedUntil: pgtype.Timestamptz{Time: now.Add(-time.Hour), Valid: true}}, false},
		{"impressions capped", userActionState{Status: StatusShown, Impressions: 5}, true},
		{"under cap passes", userActionState{Status: StatusShown, Impressions: 2}, false},
	}

	for _, tc := range cases {
		if got := svc.stateFiltered(rule, tc.st); got != tc.want {
			t.Errorf("%s: stateFiltered = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestStateFilteredOnceMode(t *testing.T) {
	t.Parallel()
	svc := &Service{}
	rule := testRule("r", 1, []string{"regular"}, []string{"popup"})
	rule.Frequency.Mode = "once"
	rule.Frequency.MaxImpressions = 0
	if !svc.stateFiltered(rule, userActionState{Status: StatusShown, Impressions: 1}) {
		t.Error("once-mode rule should filter after 1 impression")
	}
	if svc.stateFiltered(rule, userActionState{Status: StatusPending}) {
		t.Error("once-mode rule should pass with 0 impressions")
	}
}

func TestStateFilteredCooldown(t *testing.T) {
	t.Parallel()
	svc := &Service{}
	rule := testRule("r", 1, []string{"regular"}, []string{"popup"})
	rule.Frequency.CooldownHours = 24
	recent := userActionState{Status: StatusShown, Impressions: 1,
		LastShownAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true}}
	if !svc.stateFiltered(rule, recent) {
		t.Error("cooldown should filter recently-shown rule")
	}
	stale := userActionState{Status: StatusShown, Impressions: 1,
		LastShownAt: pgtype.Timestamptz{Time: time.Now().Add(-48 * time.Hour), Valid: true}}
	if svc.stateFiltered(rule, stale) {
		t.Error("stale impression should pass cooldown")
	}
}

func TestAudienceMatch(t *testing.T) {
	t.Parallel()
	svc := &Service{}
	rule := testRule("r", 1, []string{"regular", "pro"}, []string{"popup"})
	rule.Audience.DestinationCountry = []string{"CA"}
	rule.Audience.Surfaces = []string{"main"}

	member := &UserState{UserClass: ClassRegular, Surface: "main",
		VCtx: visitor.VisitorContext{DestinationCountry: "CA"}}
	if !svc.audienceMatch(rule, member) {
		t.Error("expected audience match")
	}
	guest := &UserState{UserClass: ClassGuest, Surface: "main",
		VCtx: visitor.VisitorContext{DestinationCountry: "CA"}}
	if svc.audienceMatch(rule, guest) {
		t.Error("guest should not match member-only audience")
	}
	wrongSurface := &UserState{UserClass: ClassRegular, Surface: "docs",
		VCtx: visitor.VisitorContext{DestinationCountry: "CA"}}
	if svc.audienceMatch(rule, wrongSurface) {
		t.Error("docs surface should not match")
	}
	wrongDest := &UserState{UserClass: ClassRegular, Surface: "main",
		VCtx: visitor.VisitorContext{DestinationCountry: "GB"}}
	if svc.audienceMatch(rule, wrongDest) {
		t.Error("GB dest should not match CA audience")
	}
}

func TestUpsellFlagFlowsToSuggestion(t *testing.T) {
	t.Parallel()
	svc := &Service{}
	rule := testRule("u", 1, []string{"regular"}, []string{"popup"})
	rule.Upsell = true
	if !svc.toSuggestion(rule).upsell {
		t.Error("upsell flag should flow to the suggestion")
	}
	if svc.toSuggestion(testRule("p", 1, nil, nil)).upsell {
		t.Error("non-upsell rule should not flag the suggestion")
	}
}

func TestParseUUID(t *testing.T) {
	t.Parallel()
	id := pgtype.UUID{Bytes: [16]byte{9}, Valid: true}
	if got := parseUUID(id.String()); !got.Valid || got != id {
		t.Errorf("parseUUID round-trip failed: %v", got)
	}
	if parseUUID("not-a-uuid").Valid {
		t.Error("bad uuid should be invalid")
	}
}
