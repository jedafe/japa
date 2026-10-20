package nextactions

import (
	"fmt"
	"slices"
	"strings"
)

// predicateRegistry is the code-registered predicate vocabulary (ADR-132 §2).
// Task 5 implements the evaluators; validation lives here so rule CRUD can
// reject unknown predicate names without importing the evaluator table.
var predicateRegistry = map[string]bool{
	"user_class":            true,
	"account_tier":          true,
	"account_age_days":      true,
	"onboarding_completed":  true,
	"email_verified":        true,
	"profile_completeness":  true,
	"japa_stage":            true,
	"archetype":             true,
	"destination_country":   true,
	"origin_country":        true,
	"current_country":       true,
	"assessment_completed":  true,
	"assessment_score":      true,
	"tool_used":             true,
	"tool_not_used":         true,
	"feature_used":          true,
	"feature_not_used":      true,
	"event_count":           true,
	"feature_gate_hit":      true,
	"session_page_views":    true,
	"dwell_seconds":         true,
	"has_push_subscription": true,
	"surface":               true,
}

// PredicateNames returns the registered predicate vocabulary.
func PredicateNames() []string {
	out := make([]string, 0, len(predicateRegistry))
	for k := range predicateRegistry {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

var channelSet = map[string]bool{
	ChannelPopup: true, ChannelAnnouncementBar: true, ChannelSidebar: true,
	ChannelInArticle: true, ChannelNotification: true, ChannelPush: true, ChannelEmail: true,
}

// ValidateRule checks a rule before persistence (create/update). It enforces
// the §122 contract: known channels, guest-audience rules may only use
// pull-render channels, every predicate name must be registered, and cta_url
// must be a relative path or an origin in allowedOrigins (the
// nextactions.allowed_cta_origins setting, decoded by the caller).
func ValidateRule(r Rule, allowedOrigins []string) error {
	if strings.TrimSpace(r.Slug) == "" {
		return fmt.Errorf("slug is required")
	}
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(r.Payload.Title) == "" {
		return fmt.Errorf("payload.title is required")
	}
	if err := validateCTAURL(r.Payload.CTAURL, allowedOrigins); err != nil {
		return err
	}
	if len(r.Channels) == 0 {
		return fmt.Errorf("at least one channel is required")
	}
	for _, ch := range r.Channels {
		if !channelSet[ch] {
			return fmt.Errorf("unknown channel %q", ch)
		}
	}
	if slices.Contains(r.Audience.UserClass, ClassGuest) {
		for _, ch := range r.Channels {
			if slices.Contains(PushChannels, ch) {
				return fmt.Errorf("guest-audience rules may not use push channel %q", ch)
			}
		}
	}
	if err := validateGroup(r.Conditions); err != nil {
		return fmt.Errorf("conditions: %w", err)
	}
	if r.CompletionSignal != nil {
		if err := validateGroup(*r.CompletionSignal); err != nil {
			return fmt.Errorf("completion_signal: %w", err)
		}
	}
	return nil
}

func validateGroup(g ConditionGroup) error {
	for _, p := range append(append([]Predicate{}, g.All...), g.Any...) {
		if !predicateRegistry[p.Predicate] {
			return fmt.Errorf("unknown predicate %q", p.Predicate)
		}
	}
	return nil
}

// validateCTAURL enforces the open-redirect guard: relative paths always pass;
// absolute http(s) URLs pass only when their origin is allowlisted.
func validateCTAURL(u string, allowedOrigins []string) error {
	if u == "" {
		return fmt.Errorf("payload.cta_url is required")
	}
	if strings.HasPrefix(u, "/") {
		if strings.HasPrefix(u, "//") {
			return fmt.Errorf("payload.cta_url must not be protocol-relative")
		}
		return nil
	}
	if !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "http://") {
		return fmt.Errorf("payload.cta_url must be a relative path or http(s) URL")
	}
	for _, o := range allowedOrigins {
		if o != "" && strings.HasPrefix(u, strings.TrimSuffix(o, "/")+"/") {
			return nil
		}
	}
	return fmt.Errorf("payload.cta_url origin is not in nextactions.allowed_cta_origins")
}
