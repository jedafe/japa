package nextactions

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

// EvalDeps abstracts the signal sources predicates read — one implementation
// backed by sqlc queries + Valkey lives in the service; tests stub it.
type EvalDeps interface {
	// CountEvents returns platform_events count within `days`. session=true
	// keys on session_id (guest IP / member JTI); session=false on user_id
	// (guests still fall back to session_id — they have no user rows).
	CountEvents(ctx context.Context, s *UserState, eventType string, days int, session bool) (int64, error)
	// ToolUsed reports whether the state used tool `slug` ("*" = any tool)
	// within `days` — platform_events `tool.used` meta.tool.
	ToolUsed(ctx context.Context, s *UserState, slug string, days int) (bool, error)
	// FeatureUsed reports a `feature.<slug>` style event within `days`.
	FeatureUsed(ctx context.Context, s *UserState, slug string, days int) (bool, error)
	// GateHit reports a `feature.gate_hit` event within `days` (feature
	// filter optional — empty matches any).
	GateHit(ctx context.Context, s *UserState, feature string, days int) (bool, error)
	// AssessmentDone returns completion + latest total_score (§71).
	AssessmentDone(ctx context.Context, userID pgtype.UUID) (bool, int, error)
	// HasPushSub reports an active §35 push subscription.
	HasPushSub(ctx context.Context, userID pgtype.UUID) (bool, error)
}

// EvalConditions evaluates a predicate group: every `all` clause must pass,
// and when `any` clauses exist at least one must pass. An empty group is true.
// Evaluation failures (deps error, guest state for user-only signals) resolve
// to false — a broken signal suppresses a suggestion, never errors the request.
func EvalConditions(ctx context.Context, g ConditionGroup, s *UserState, deps EvalDeps) bool {
	for _, p := range g.All {
		if !evalPredicate(ctx, p, s, deps) {
			return false
		}
	}
	if len(g.Any) > 0 {
		for _, p := range g.Any {
			if evalPredicate(ctx, p, s, deps) {
				return true
			}
		}
		return false
	}
	return true
}

func evalPredicate(ctx context.Context, p Predicate, s *UserState, deps EvalDeps) bool {
	switch p.Predicate {
	case "user_class", "account_tier":
		return slices.Contains(p.In, s.UserClass)
	case "account_age_days":
		return cmpInt(s.AccountAgeDays, p)
	case "onboarding_completed":
		return eqBool(s.OnboardingDone, p.Eq)
	case "email_verified":
		return eqBool(s.EmailVerified, p.Eq)
	case "profile_completeness":
		return cmpInt(s.ProfilePct, p)
	case "japa_stage":
		return slices.Contains(p.In, s.VCtx.JapaStage)
	case "archetype":
		return slices.Contains(p.In, s.VCtx.Archetype)
	case "destination_country":
		return slices.Contains(p.In, s.VCtx.DestinationCountry)
	case "origin_country":
		return slices.Contains(p.In, s.VCtx.OriginCountry)
	case "current_country":
		return slices.Contains(p.In, s.VCtx.CurrentCountry)
	case "surface":
		return slices.Contains(p.In, s.Surface)
	case "assessment_completed", "assessment_score":
		return evalAssessment(ctx, p, s, deps)
	case "tool_used":
		return evalDepsBool(ctx, s, deps, func() (bool, error) { return deps.ToolUsed(ctx, s, p.Slug, daysOr(p.WithinDays, 30)) })
	case "tool_not_used":
		return !evalDepsBool(ctx, s, deps, func() (bool, error) { return deps.ToolUsed(ctx, s, p.Slug, daysOr(p.WithinDays, 30)) })
	case "feature_used":
		return evalDepsBool(ctx, s, deps, func() (bool, error) { return deps.FeatureUsed(ctx, s, p.Slug, daysOr(p.WithinDays, 30)) })
	case "feature_not_used":
		return !evalDepsBool(ctx, s, deps, func() (bool, error) { return deps.FeatureUsed(ctx, s, p.Slug, daysOr(p.WithinDays, 30)) })
	case "feature_gate_hit":
		return evalDepsBool(ctx, s, deps, func() (bool, error) { return deps.GateHit(ctx, s, p.Feature, daysOr(p.WithinDays, 30)) })
	case "event_count":
		return evalEventCount(ctx, p, s, deps)
	case "session_page_views":
		return cmpInt(s.PageViews, p)
	case "dwell_seconds":
		return cmpInt(s.DwellSeconds, p)
	case "has_push_subscription":
		if !s.UserID.Valid {
			return eqBool(false, p.Eq)
		}
		return evalDepsBool(ctx, s, deps, func() (bool, error) { return deps.HasPushSub(ctx, s.UserID) }) == eqBool(true, p.Eq)
	default:
		return false
	}
}

func evalAssessment(ctx context.Context, p Predicate, s *UserState, deps EvalDeps) bool {
	if deps == nil || !s.UserID.Valid {
		// Guests have no user-scoped assessment result.
		if p.Predicate == "assessment_completed" && p.Eq != nil && !*p.Eq {
			return true // "not completed" is true for a guest
		}
		return false
	}
	done, score, err := deps.AssessmentDone(ctx, s.UserID)
	if err != nil {
		return false
	}
	if p.Predicate == "assessment_completed" {
		return eqBool(done, p.Eq)
	}
	if !done {
		return false
	}
	return cmpInt(score, p)
}

func evalEventCount(ctx context.Context, p Predicate, s *UserState, deps EvalDeps) bool {
	if deps == nil || p.Gte == nil || p.Type == "" {
		return false
	}
	n, err := deps.CountEvents(ctx, s, p.Type, windowDays(p.Window), p.Window == "session")
	if err != nil {
		return false
	}
	return n >= int64(*p.Gte)
}

// evalDepsBool calls a deps probe and folds errors to false.
func evalDepsBool(ctx context.Context, _ *UserState, deps EvalDeps, fn func() (bool, error)) bool {
	if deps == nil {
		return false
	}
	v, err := fn()
	return err == nil && v
}

// cmpInt applies the lt/gt/gte comparators on a predicate; a predicate with
// none of them never matches.
func cmpInt(actual int, p Predicate) bool {
	matched := false
	if p.Lt != nil {
		if actual >= *p.Lt {
			return false
		}
		matched = true
	}
	if p.Gt != nil {
		if actual <= *p.Gt {
			return false
		}
		matched = true
	}
	if p.Gte != nil {
		if actual < *p.Gte {
			return false
		}
		matched = true
	}
	return matched
}

// eqBool compares a bool against the predicate's eq arg; a nil eq means
// "must be true" — {predicate:"x"} reads as x == true.
func eqBool(actual bool, eq *bool) bool {
	if eq == nil {
		return actual
	}
	return actual == *eq
}

// windowDays parses a predicate window: "session" → 1 (session-scoped rows
// are short-lived; a day bounds them generously), "Nd" → N, empty/invalid → 30.
func windowDays(w string) int {
	if w == "session" {
		return 1
	}
	if d, err := strconv.Atoi(strings.TrimSuffix(w, "d")); err == nil && d > 0 {
		return d
	}
	return 30
}

func daysOr(v, fallback int) int {
	if v > 0 {
		return v
	}
	return fallback
}
