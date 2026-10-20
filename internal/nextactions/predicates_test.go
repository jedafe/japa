package nextactions

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/visitor"
)

// stubDeps is a fake EvalDeps for unit tests.
type stubDeps struct {
	events      int64
	toolUsed    bool
	featureUsed bool
	gateHit     bool
	assessDone  bool
	assessScore int
	pushSub     bool
}

func (s stubDeps) CountEvents(_ context.Context, _ *UserState, _ string, _ int, _ bool) (int64, error) {
	return s.events, nil
}
func (s stubDeps) ToolUsed(_ context.Context, _ *UserState, _ string, _ int) (bool, error) {
	return s.toolUsed, nil
}
func (s stubDeps) FeatureUsed(_ context.Context, _ *UserState, _ string, _ int) (bool, error) {
	return s.featureUsed, nil
}
func (s stubDeps) GateHit(_ context.Context, _ *UserState, _ string, _ int) (bool, error) {
	return s.gateHit, nil
}
func (s stubDeps) AssessmentDone(_ context.Context, _ pgtype.UUID) (bool, int, error) {
	return s.assessDone, s.assessScore, nil
}
func (s stubDeps) HasPushSub(_ context.Context, _ pgtype.UUID) (bool, error) {
	return s.pushSub, nil
}

func userState() *UserState {
	return &UserState{
		UserClass:      ClassRegular,
		UserID:         pgtype.UUID{Bytes: [16]byte{1, 2, 3}, Valid: true},
		AccountAgeDays: 10,
		EmailVerified:  true,
		VCtx: visitor.VisitorContext{
			DestinationCountry: "CA",
			JapaStage:          "planning",
		},
		Surface: "main",
	}
}

func TestEvalConditions(t *testing.T) {
	t.Parallel()

	yes := boolPtr(true)
	no := boolPtr(false)

	cases := []struct {
		name string
		g    ConditionGroup
		s    *UserState
		d    EvalDeps
		want bool
	}{
		{"empty group true", ConditionGroup{}, userState(), stubDeps{}, true},
		{"account_age lt pass", ConditionGroup{All: []Predicate{{Predicate: "account_age_days", Lt: intPtr(30)}}}, userState(), stubDeps{}, true},
		{"account_age lt fail", ConditionGroup{All: []Predicate{{Predicate: "account_age_days", Lt: intPtr(5)}}}, userState(), stubDeps{}, false},
		{"account_age gt pass", ConditionGroup{All: []Predicate{{Predicate: "account_age_days", Gt: intPtr(5)}}}, userState(), stubDeps{}, true},
		{"dest in pass", ConditionGroup{All: []Predicate{{Predicate: "destination_country", In: []string{"CA"}}}}, userState(), stubDeps{}, true},
		{"dest in fail", ConditionGroup{All: []Predicate{{Predicate: "destination_country", In: []string{"GB"}}}}, userState(), stubDeps{}, false},
		{"stage in", ConditionGroup{All: []Predicate{{Predicate: "japa_stage", In: []string{"planning", "preparing"}}}}, userState(), stubDeps{}, true},
		{"tier in", ConditionGroup{All: []Predicate{{Predicate: "account_tier", In: []string{"pro", "business"}}}}, userState(), stubDeps{}, false},
		{"email verified", ConditionGroup{All: []Predicate{{Predicate: "email_verified", Eq: yes}}}, userState(), stubDeps{}, true},
		{"assessment not done", ConditionGroup{All: []Predicate{{Predicate: "assessment_completed", Eq: no}}}, userState(), stubDeps{assessDone: false}, true},
		{"assessment done fails not-done", ConditionGroup{All: []Predicate{{Predicate: "assessment_completed", Eq: no}}}, userState(), stubDeps{assessDone: true}, false},
		{"assessment score lt", ConditionGroup{All: []Predicate{{Predicate: "assessment_score", Lt: intPtr(50)}}}, userState(), stubDeps{assessDone: true, assessScore: 40}, true},
		{"event_count gte", ConditionGroup{All: []Predicate{{Predicate: "event_count", Type: "content.viewed", Gte: intPtr(3), Window: "7d"}}}, userState(), stubDeps{events: 5}, true},
		{"event_count short", ConditionGroup{All: []Predicate{{Predicate: "event_count", Type: "content.viewed", Gte: intPtr(3), Window: "7d"}}}, userState(), stubDeps{events: 2}, false},
		{"tool_used", ConditionGroup{All: []Predicate{{Predicate: "tool_used", Slug: "crs-calculator"}}}, userState(), stubDeps{toolUsed: true}, true},
		{"tool_not_used", ConditionGroup{All: []Predicate{{Predicate: "tool_not_used", Slug: "crs-calculator"}}}, userState(), stubDeps{toolUsed: false}, true},
		{"gate_hit", ConditionGroup{All: []Predicate{{Predicate: "feature_gate_hit", WithinDays: 7}}}, userState(), stubDeps{gateHit: true}, true},
		{"surface", ConditionGroup{All: []Predicate{{Predicate: "surface", In: []string{"tools"}}}}, userState(), stubDeps{}, false},
		{"all AND fails on one", ConditionGroup{All: []Predicate{
			{Predicate: "destination_country", In: []string{"CA"}},
			{Predicate: "destination_country", In: []string{"GB"}},
		}}, userState(), stubDeps{}, false},
		{"any OR passes on one", ConditionGroup{Any: []Predicate{
			{Predicate: "destination_country", In: []string{"GB"}},
			{Predicate: "account_age_days", Lt: intPtr(30)},
		}}, userState(), stubDeps{}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := EvalConditions(context.Background(), tc.g, tc.s, tc.d); got != tc.want {
				t.Errorf("EvalConditions = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestWindowDays(t *testing.T) {
	t.Parallel()
	cases := map[string]int{"7d": 7, "30d": 30, "session": 1, "": 30, "bogus": 30}
	for w, want := range cases {
		if got := windowDays(w); got != want {
			t.Errorf("windowDays(%q) = %d, want %d", w, got, want)
		}
	}
}
