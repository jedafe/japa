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
	// Each registered predicate with minimally valid args — the test proves
	// both the name registry and the arg contract stay consistent.
	preds := map[string]Predicate{
		"user_class":            {Predicate: "user_class", In: []string{"regular"}},
		"account_tier":          {Predicate: "account_tier", In: []string{"regular"}},
		"account_age_days":      {Predicate: "account_age_days", Lt: intPtr(30)},
		"onboarding_completed":  {Predicate: "onboarding_completed"},
		"email_verified":        {Predicate: "email_verified"},
		"profile_completeness":  {Predicate: "profile_completeness", Gte: intPtr(50)},
		"japa_stage":            {Predicate: "japa_stage", In: []string{"planning"}},
		"archetype":             {Predicate: "archetype", In: []string{"student"}},
		"destination_country":   {Predicate: "destination_country", In: []string{"CA"}},
		"origin_country":        {Predicate: "origin_country", In: []string{"NG"}},
		"current_country":       {Predicate: "current_country", In: []string{"NG"}},
		"assessment_completed":  {Predicate: "assessment_completed"},
		"assessment_score":      {Predicate: "assessment_score", Gte: intPtr(60)},
		"tool_used":             {Predicate: "tool_used", Slug: "crs-calculator"},
		"tool_not_used":         {Predicate: "tool_not_used", Slug: "crs-calculator"},
		"feature_used":          {Predicate: "feature_used", Slug: "checklist"},
		"feature_not_used":      {Predicate: "feature_not_used", Slug: "checklist"},
		"event_count":           {Predicate: "event_count", Type: "content.viewed", Gte: intPtr(3)},
		"feature_gate_hit":      {Predicate: "feature_gate_hit"},
		"session_page_views":    {Predicate: "session_page_views", Gte: intPtr(3)},
		"dwell_seconds":         {Predicate: "dwell_seconds", Gte: intPtr(60)},
		"has_push_subscription": {Predicate: "has_push_subscription"},
		"surface":               {Predicate: "surface", In: []string{"main"}},
	}
	for name, pred := range preds {
		r := validRule()
		r.Conditions = ConditionGroup{All: []Predicate{pred}}
		if err := ValidateRule(r, nil); err != nil {
			t.Errorf("predicate %q should validate, got %v", name, err)
		}
	}
	for name := range predicateRegistry {
		if _, ok := preds[name]; !ok {
			t.Errorf("registered predicate %q missing from test matrix", name)
		}
	}
}
