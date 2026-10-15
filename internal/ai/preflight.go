// Package ai — preflight.go implements the shared AI admission pipeline
// (§20 amendment — unified AI preflight). Every user-facing AI surface —
// JapaBot chat (/ai/chat), in-editor assist (/ai/assist/*), and the AI
// agent (/ai/agent) — runs the same checks before a prompt leaves the
// platform for an external provider:
//
//  1. Feature gate — feature_flags reachability (the legacy
//     system_settings key is the nil-checker fallback).
//  2. ai_processing consent (§19/ADR-014). Previously only /ai/chat
//     enforced this; assist and agent prompts contain user data too.
//  3. Unified daily budget — jc:ai:queries:{user_id}:{date} counted
//     against the per-tier ai.assistant.daily_queries.{tier} limits, and
//     jc:ai:cost:{user_id}:{date} capped by ai.daily_cost_limit_usd
//     (0 = disabled). Per-surface limits (agent.rate_limit_per_day,
//     agent.daily_budget_usd) still apply on top for the agent.
//
// RBAC stays at the route layer (rbac.RequirePermission) and in the
// executor's per-tool catalog filter — it is not part of the preflight.
//
// Ref: japaconnect-engineering/sections/20-ai-core.md (Unified AI preflight amendment)
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (API Routes)
package ai

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/valkey"
)

// SurfaceGate describes one user-facing AI surface's reachability config:
// the authoritative feature_flags key plus the deprecated system_settings
// fallback used when no feature checker is wired.
type SurfaceGate struct {
	FeatureKey     string
	SettingKey     string
	SettingDefault bool
	DisabledCode   string // standard error code (conventions/api-response-format.md)
	DisabledMsg    string
}

// The user-facing AI surfaces covered by the unified preflight.
var (
	// GateJapaBot gates POST /ai/chat (Section 20 JapaBot).
	GateJapaBot = SurfaceGate{
		FeatureKey:     "feature.japabot",
		SettingKey:     "ai.japabot_enabled",
		SettingDefault: true,
		DisabledCode:   "japabot_disabled",
		DisabledMsg:    "JapaBot is currently disabled",
	}
	// GateAssist gates the /ai/assist/* endpoints. Shares the
	// feature.ai_content_gen flag with /ai/generate; the legacy
	// ai.content_assist_enabled setting is the deprecated fallback.
	GateAssist = SurfaceGate{
		FeatureKey:     "feature.ai_content_gen",
		SettingKey:     "ai.content_assist_enabled",
		SettingDefault: true,
		DisabledCode:   "ai_assist_disabled",
		DisabledMsg:    "AI content assist is currently disabled",
	}
	// The §103 executor's feature.ai_agent kill-switch is resolved directly
	// by AgentHandler.agentEnabled (same flag + agent.enabled fallback);
	// agent requests run through GateJapaBot admission on /ai/chat.
)

// TierResolver resolves a user's effective unified tier
// (regular/pro/business) for per-tier daily query limits.
// *tiers.Service satisfies this.
type TierResolver interface {
	ResolveUserTier(ctx context.Context, userID pgtype.UUID) (string, error)
}

// PreflightError is a typed admission failure; Status and Code map onto the
// standard API error envelope so HTTP callers can write it verbatim.
type PreflightError struct {
	Status  int
	Code    string
	Message string
}

func (e *PreflightError) Error() string { return e.Message }

// Preflight bundles the cross-cutting admission checks shared by the
// user-facing AI surfaces. It is constructed once at wiring time and held
// by both *Handlers (chat + assist) and *AgentHandler (agent).
type Preflight struct {
	checker  FeatureChecker
	settings SettingsReader
	consents ConsentChecker
	budget   RateLimiter
	tiers    TierResolver
}

// NewPreflight creates the shared admission pipeline. Any nil dependency
// degrades only its own step (checker nil → settings fallback; settings
// nil → gate open; consents nil → consent skipped; budget nil → no daily
// budget; tiers nil → regular-tier limits).
func NewPreflight(checker FeatureChecker, settings SettingsReader, consents ConsentChecker, budget RateLimiter, tiers TierResolver) *Preflight {
	return &Preflight{
		checker:  checker,
		settings: settings,
		consents: consents,
		budget:   budget,
		tiers:    tiers,
	}
}

// aiBudgetDailyTTL is the safety TTL for the unified daily keys. The
// {date} component scopes each key to the UTC day; the 24h TTL is the
// upper bound (mirrors agentRateLimitTTL).
const aiBudgetDailyTTL = 24 * time.Hour

// aiQueriesKey is the unified per-user daily AI request counter.
// Ref: conventions/valkey-keys-catalog.md — jc:ai:queries:{user_id}:{date}
func aiQueriesKey(userID pgtype.UUID) string {
	return valkey.MustKey("ai:queries:%s:%s", userID.String(), time.Now().UTC().Format("2006-01-02"))
}

// aiCostKey is the unified per-user daily AI spend accumulator (USD).
// Ref: conventions/valkey-keys-catalog.md — jc:ai:cost:{user_id}:{date}
func aiCostKey(userID pgtype.UUID) string {
	return valkey.MustKey("ai:cost:%s:%s", userID.String(), time.Now().UTC().Format("2006-01-02"))
}

// defaultDailyQueryLimits mirrors the ai.assistant.daily_queries.{tier}
// catalog defaults. -1 = unlimited.
var defaultDailyQueryLimits = map[string]int{
	"regular":  10,
	"pro":      100,
	"business": -1,
}

// gateEnabled resolves surface reachability: feature_flags is the source
// of truth when a checker is wired; the deprecated system_settings key is
// the fallback. Nil settings with no checker defaults to open.
func (p *Preflight) gateEnabled(ctx context.Context, gate SurfaceGate) bool {
	if p.checker != nil {
		return p.checker.IsEnabled(ctx, gate.FeatureKey)
	}

	if p.settings == nil {
		return true
	}

	return p.settings.GetBoolOr(ctx, gate.SettingKey, gate.SettingDefault)
}

// CheckAdmission runs the deterministic admission gates — surface
// reachability then ai_processing consent. Call CheckBudget separately
// after input validation so malformed requests do not burn quota.
func (p *Preflight) CheckAdmission(ctx context.Context, userID pgtype.UUID, gate SurfaceGate) *PreflightError {
	if p == nil {
		return nil
	}

	if !p.gateEnabled(ctx, gate) {
		return &PreflightError{Status: http.StatusServiceUnavailable, Code: gate.DisabledCode, Message: gate.DisabledMsg}
	}

	return p.CheckConsent(ctx, userID)
}

// CheckConsent enforces §19/ADR-014: user prompts may not leave the
// platform for external AI providers without ai_processing consent. A nil
// checker or an invalid (guest) userID skips the check; a checker error
// fails open so a consent-store outage cannot lock users out.
func (p *Preflight) CheckConsent(ctx context.Context, userID pgtype.UUID) *PreflightError {
	if p == nil || p.consents == nil || !userID.Valid {
		return nil
	}

	consented, err := p.consents.HasConsent(ctx, userID, "ai_processing")
	if err != nil {
		slog.Warn("ai: consent check failed, allowing request", "error", err)
		return nil
	}

	if !consented {
		return &PreflightError{Status: http.StatusForbidden, Code: "ai_consent_required", Message: "You must consent to AI processing before using AI features"}
	}

	return nil
}

// CheckBudget enforces the unified daily AI budget. The query counter is
// incremented on every admitted request (increment-then-check, matching
// the agent's jc:agent:rate pattern) and compared against the caller's
// per-tier ai.assistant.daily_queries.{tier} limit (-1 = unlimited). The
// jc:ai:cost accumulator is checked against ai.daily_cost_limit_usd
// (0 = disabled). A nil budget client or invalid userID skips; Valkey
// failures fail open.
func (p *Preflight) CheckBudget(ctx context.Context, userID pgtype.UUID) *PreflightError {
	if p == nil || p.budget == nil || !userID.Valid {
		return nil
	}

	if limit := p.dailyQueryLimit(ctx, userID); limit >= 0 {
		key := aiQueriesKey(userID)

		count, err := p.budget.Incr(ctx, key)
		if err != nil {
			slog.Warn("ai: unified daily query check failed, allowing request", "error", err)
		} else {
			if count == 1 {
				if err := p.budget.Expire(ctx, key, int(aiBudgetDailyTTL.Seconds())); err != nil {
					slog.Warn("ai: failed to set unified query key TTL", "error", err)
				}
			}

			if count > int64(limit) {
				return &PreflightError{
					Status:  http.StatusTooManyRequests,
					Code:    "ai_daily_limit",
					Message: fmt.Sprintf("Daily AI query limit reached (%d)", limit),
				}
			}
		}
	}

	if costCap := p.dailyCostLimit(ctx); costCap > 0 {
		raw, err := p.budget.Get(ctx, aiCostKey(userID))
		if err == nil && raw != "" {
			if total, perr := strconv.ParseFloat(raw, 64); perr == nil && total >= costCap {
				return &PreflightError{
					Status:  http.StatusPaymentRequired,
					Code:    "ai_daily_budget",
					Message: "Daily AI spend budget exceeded",
				}
			}
		}
	}

	return nil
}

// dailyQueryLimit resolves the caller's per-tier daily query limit.
// Unknown or unresolvable tiers fall back to regular. -1 = unlimited.
func (p *Preflight) dailyQueryLimit(ctx context.Context, userID pgtype.UUID) int {
	tier := "regular"

	if p.tiers != nil {
		if resolved, err := p.tiers.ResolveUserTier(ctx, userID); err == nil && resolved != "" {
			tier = resolved
		} else if err != nil {
			slog.Warn("ai: failed to resolve user tier for daily query limit", "error", err)
		}
	}

	fallback := defaultDailyQueryLimits["regular"]
	if d, ok := defaultDailyQueryLimits[tier]; ok {
		fallback = d
	}

	if p.settings == nil {
		return fallback
	}

	return p.settings.GetIntOr(ctx, "ai.assistant.daily_queries."+tier, fallback)
}

// dailyCostLimit reads the unified daily AI spend cap. 0 = disabled.
func (p *Preflight) dailyCostLimit(ctx context.Context) float64 {
	if p.settings == nil {
		return 0
	}

	return p.settings.GetFloatOr(ctx, "ai.daily_cost_limit_usd", 0)
}

// RecordCost accumulates a completed request's estimated USD cost into
// jc:ai:cost:{user_id}:{date}. Called after dispatch:
// dispatchAndRecordUsage covers generate/chat/assist; the agent
// accumulates alongside its own jc:agent:cost key. Best-effort — Valkey
// failures are logged, never propagated.
func (p *Preflight) RecordCost(ctx context.Context, userID pgtype.UUID, cost float64) {
	if p == nil || p.budget == nil || cost <= 0 || !userID.Valid {
		return
	}

	key := aiCostKey(userID)

	total, err := p.budget.IncrByFloat(ctx, key, cost)
	if err != nil {
		slog.Warn("ai: failed to track unified AI cost", "error", err)
		return
	}

	if total == cost {
		if err := p.budget.Expire(ctx, key, int(aiBudgetDailyTTL.Seconds())); err != nil {
			slog.Warn("ai: failed to set unified cost key TTL", "error", err)
		}
	}
}
