package nextactions

import "testing"

func validRule() Rule {
	return Rule{
		Slug: "test-rule", Name: "Test",
		Audience: Audience{UserClass: []string{"regular"}},
		Conditions: ConditionGroup{All: []Predicate{
			{Predicate: "account_age_days", Lt: intPtr(30)},
		}},
		Payload:   Payload{Title: "T", CTAURL: "/pricing"},
		Channels:  []string{"popup"},
		Frequency: Frequency{Mode: "recurring", MaxImpressions: 5},
	}
}

func intPtr(v int) *int    { return &v }
func boolPtr(v bool) *bool { return &v }

func TestValidateRuleRejectsGuestPushChannels(t *testing.T) {
	t.Parallel()
	r := validRule()
	r.Audience.UserClass = []string{"guest", "regular"}
	r.Channels = []string{"popup", "push"}
	if err := ValidateRule(r, nil); err == nil {
		t.Fatal("expected error for guest audience + push channel")
	}
}

func TestValidateRuleRejectsExternalCTA(t *testing.T) {
	t.Parallel()
	r := validRule()
	r.Payload.CTAURL = "https://evil.example.com/phish"
	if err := ValidateRule(r, nil); err == nil {
		t.Fatal("expected error for external cta_url")
	}
}

func TestValidateRuleRejectsUnknownChannel(t *testing.T) {
	t.Parallel()
	r := validRule()
	r.Channels = []string{"smoke_signal"}
	if err := ValidateRule(r, nil); err == nil {
		t.Fatal("expected error for unknown channel")
	}
}

func TestValidateRuleRejectsUnknownPredicate(t *testing.T) {
	t.Parallel()
	r := validRule()
	r.Conditions = ConditionGroup{All: []Predicate{{Predicate: "telepathy"}}}
	if err := ValidateRule(r, nil); err == nil {
		t.Fatal("expected error for unknown predicate")
	}
}

func TestValidateRuleRejectsMissingBasics(t *testing.T) {
	t.Parallel()
	r := validRule()
	r.Payload.Title = ""
	if err := ValidateRule(r, nil); err == nil {
		t.Fatal("expected error for empty title")
	}
	r = validRule()
	r.Slug = ""
	if err := ValidateRule(r, nil); err == nil {
		t.Fatal("expected error for empty slug")
	}
}

func TestValidateRuleAcceptsSeedShapes(t *testing.T) {
	t.Parallel()
	for _, r := range []Rule{
		validRule(),
		func() Rule {
			r := validRule()
			r.Audience.UserClass = []string{"guest"}
			r.Channels = []string{"popup", "announcement_bar", "sidebar", "in_article"}
			r.Conditions = ConditionGroup{All: []Predicate{
				{Predicate: "event_count", Type: "content.viewed", Gte: intPtr(3), Window: "7d"},
			}}
			return r
		}(),
		func() Rule {
			r := validRule()
			r.Channels = []string{"popup", "notification", "push", "email"}
			r.CompletionSignal = &ConditionGroup{All: []Predicate{
				{Predicate: "account_tier", In: []string{"pro", "business"}},
			}}
			return r
		}(),
	} {
		if err := ValidateRule(r, nil); err != nil {
			t.Fatalf("expected valid rule, got %v", err)
		}
	}
}

func TestValidateRuleAllPredicatesKnown(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"user_class", "account_tier", "account_age_days", "onboarding_completed",
		"email_verified", "profile_completeness", "japa_stage", "archetype",
		"destination_country", "origin_country", "current_country",
		"assessment_completed", "assessment_score", "tool_used", "tool_not_used",
		"feature_used", "feature_not_used", "event_count", "feature_gate_hit",
		"session_page_views", "dwell_seconds", "has_push_subscription", "surface",
	} {
		r := validRule()
		r.Conditions = ConditionGroup{All: []Predicate{{Predicate: name}}}
		if err := ValidateRule(r, nil); err != nil {
			t.Errorf("predicate %q should validate, got %v", name, err)
		}
	}
}
