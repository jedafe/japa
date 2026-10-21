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

// PredicateArg describes one argument slot of a predicate for the Control
// rule builder — drives which input fields the editor renders.
type PredicateArg struct {
	Key      string `json:"key"`  // in|eq|lt|gt|gte|slug|type|feature|within_days|window
	Type     string `json:"type"` // string_list|bool|number|string|select
	Required bool   `json:"required"`
	Options  string `json:"options,omitempty"` // for select: hint shown in placeholder
}

// PredicateSpec is the builder-facing contract for one predicate.
type PredicateSpec struct {
	Name string         `json:"name"`
	Args []PredicateArg `json:"args"`
}

var inArg = PredicateArg{Key: "in", Type: "string_list", Required: true}
var numArgs = []PredicateArg{{Key: "lt", Type: "number"}, {Key: "gt", Type: "number"}, {Key: "gte", Type: "number"}}

// PredicateSpecs returns the arg schema per registered predicate — served to
// the Control rule editor so the structured builder stays in sync with the
// code-registered vocabulary without hardcoding names client-side.
func PredicateSpecs() []PredicateSpec {
	return []PredicateSpec{
		{Name: "user_class", Args: []PredicateArg{inArg}},
		{Name: "account_tier", Args: []PredicateArg{inArg}},
		{Name: "account_age_days", Args: numArgs},
		{Name: "onboarding_completed", Args: []PredicateArg{{Key: "eq", Type: "bool"}}},
		{Name: "email_verified", Args: []PredicateArg{{Key: "eq", Type: "bool"}}},
		{Name: "profile_completeness", Args: numArgs},
		{Name: "japa_stage", Args: []PredicateArg{inArg}},
		{Name: "archetype", Args: []PredicateArg{inArg}},
		{Name: "destination_country", Args: []PredicateArg{inArg}},
		{Name: "origin_country", Args: []PredicateArg{inArg}},
		{Name: "current_country", Args: []PredicateArg{inArg}},
		{Name: "surface", Args: []PredicateArg{inArg}},
		{Name: "assessment_completed", Args: []PredicateArg{{Key: "eq", Type: "bool"}}},
		{Name: "assessment_score", Args: numArgs},
		{Name: "tool_used", Args: []PredicateArg{{Key: "slug", Type: "string", Required: true}, {Key: "within_days", Type: "number"}}},
		{Name: "tool_not_used", Args: []PredicateArg{{Key: "slug", Type: "string", Required: true}, {Key: "within_days", Type: "number"}}},
		{Name: "feature_used", Args: []PredicateArg{{Key: "slug", Type: "string", Required: true}, {Key: "within_days", Type: "number"}}},
		{Name: "feature_not_used", Args: []PredicateArg{{Key: "slug", Type: "string", Required: true}, {Key: "within_days", Type: "number"}}},
		{Name: "feature_gate_hit", Args: []PredicateArg{{Key: "feature", Type: "string"}, {Key: "within_days", Type: "number"}}},
		{Name: "event_count", Args: []PredicateArg{{Key: "type", Type: "string", Required: true}, {Key: "lt", Type: "number"}, {Key: "gt", Type: "number"}, {Key: "gte", Type: "number"}, {Key: "window", Type: "string", Options: "session or Nd (e.g. 7d)"}}},
		{Name: "has_push_subscription", Args: []PredicateArg{{Key: "eq", Type: "bool"}}},
	}
}

// userClassSet and surfaceSet bound the audience vocab — a typo'd class or
// surface makes a rule silently never-match, so validate at write time.
var userClassSet = map[string]bool{
	ClassGuest: true, ClassRegular: true, ClassPro: true, ClassBusiness: true,
}

var surfaceSet = map[string]bool{
	"main": true, "tools": true, "academy": true, "docs": true, "mobile": true,
}

var frequencyModeSet = map[string]bool{"once": true, "recurring": true}

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
	if !frequencyModeSet[r.Frequency.Mode] {
		return fmt.Errorf("frequency.mode must be once or recurring")
	}
	// A recurring push rule with no cooldown would re-notify on every hourly
	// sweep — require a cooldown so nudge fatigue stays bounded.
	if r.Frequency.Mode == "recurring" && r.Frequency.CooldownHours <= 0 {
		for _, ch := range r.Channels {
			if slices.Contains(PushChannels, ch) {
				return fmt.Errorf("recurring rules with push channel %q require frequency.cooldown_hours > 0", ch)
			}
		}
	}
	for _, uc := range r.Audience.UserClass {
		if !userClassSet[uc] {
			return fmt.Errorf("unknown audience user_class %q", uc)
		}
	}
	for _, sf := range r.Audience.Surfaces {
		if !surfaceSet[sf] {
			return fmt.Errorf("unknown audience surface %q", sf)
		}
	}
	if slices.Contains(r.Audience.UserClass, ClassGuest) {
		for _, ch := range r.Channels {
			if slices.Contains(PushChannels, ch) {
				return fmt.Errorf("guest-audience rules may not use push channel %q", ch)
			}
		}
	}
	// An inverted schedule window can never match — catch it at write time
	// instead of leaving an always-dead rule.
	if r.ValidFrom.Valid && r.ValidUntil.Valid && !r.ValidUntil.Time.After(r.ValidFrom.Time) {
		return fmt.Errorf("valid_until must be after valid_from")
	}
	if err := validateGroup(r.Conditions); err != nil {
		return fmt.Errorf("conditions: %w", err)
	}
	if r.CompletionSignal != nil {
		// An empty group evaluates true — an enabled-but-empty signal would
		// auto-complete the rule on every eval and never render it.
		if len(r.CompletionSignal.All)+len(r.CompletionSignal.Any) == 0 {
			return fmt.Errorf("completion_signal requires at least one predicate")
		}
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
		if err := validatePredicateArgs(p); err != nil {
			return err
		}
	}
	return nil
}

// validatePredicateArgs rejects predicates that can never match — required
// args missing, empty `in` lists, or comparators with no bound.
func validatePredicateArgs(p Predicate) error {
	switch p.Predicate {
	case "user_class", "account_tier", "japa_stage", "archetype",
		"destination_country", "origin_country", "current_country", "surface":
		if len(p.In) == 0 {
			return fmt.Errorf("%s requires a non-empty in list", p.Predicate)
		}
		// Bounded vocabularies — a typo'd value makes the rule silently
		// never-match, same as the audience.user_class check above.
		switch p.Predicate {
		case "user_class":
			for _, v := range p.In {
				if !userClassSet[v] {
					return fmt.Errorf("user_class: unknown value %q", v)
				}
			}
		case "surface":
			for _, v := range p.In {
				if !surfaceSet[v] {
					return fmt.Errorf("surface: unknown value %q", v)
				}
			}
		}
	case "tool_used", "tool_not_used", "feature_used", "feature_not_used":
		if p.Slug == "" {
			return fmt.Errorf("%s requires slug", p.Predicate)
		}
	case "event_count":
		if p.Type == "" || (p.Lt == nil && p.Gt == nil && p.Gte == nil) {
			return fmt.Errorf("event_count requires type and at least one of lt, gt, gte")
		}
	case "account_age_days", "profile_completeness", "assessment_score":
		if p.Lt == nil && p.Gt == nil && p.Gte == nil {
			return fmt.Errorf("%s requires lt, gt, or gte", p.Predicate)
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
