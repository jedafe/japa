package nextactions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/analytics"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// defaults — fallbacks when system_settings keys are unset (Rule 3).
const (
	defaultCacheTTLSeconds      = 300
	defaultMaxActive            = 5
	defaultMaxUpsellPerDay      = 1
	defaultGuestStateTTL        = 2592000 // 30 days
	defaultSnoozeHours          = 168
	defaultSweepWindowDays      = 7
	defaultSweepIntervalMinutes = 60
	defaultSweepBatch           = 500
	rulesCacheTTL               = 5 * time.Minute
	sessionCounterFieldPrefix   = "ctr:"
)

// EventTracker is the analytics slice the service needs (§40 writes).
type EventTracker interface {
	TrackEvent(ctx context.Context, input analytics.TrackEventInput) (gen.CreatePlatformEventRow, error)
}

// Notifier dispatches a materialized suggestion via §35 (wired in Task 10).
type Notifier interface {
	NotifySuggestion(ctx context.Context, userID pgtype.UUID, rule Rule) error
}

// Service is the next-action engine: pull-path evaluation, lifecycle state,
// guest state, and sweep materialization.
type Service struct {
	read     *gen.Queries
	write    *gen.Queries
	cache    valkeycompat.Cmdable // nil disables caching (tests)
	settings *config.Settings
	resolver *StateResolver
	deps     EvalDeps
	tracker  EventTracker // nil disables platform_events writes (tests)
	notifier Notifier     // nil disables sweep dispatch (tests)
}

// Deps bundles the service's constructor arguments.
type Deps struct {
	Read     *gen.Queries
	Write    *gen.Queries
	Cache    valkeycompat.Cmdable
	Settings *config.Settings
	Resolver *StateResolver
	Tracker  EventTracker
	Notifier Notifier
}

// New creates the engine.
func New(d Deps) *Service {
	return &Service{
		read: d.Read, write: d.Write, cache: d.Cache,
		settings: d.Settings, resolver: d.Resolver,
		deps: newQueryDeps(d.Read), tracker: d.Tracker, notifier: d.Notifier,
	}
}

// SetNotifier wires the §35 notifier after construction (breaker for the
// notifications→worker→server init order).
func (s *Service) SetNotifier(n Notifier) { s.notifier = n }

func (s *Service) settingInt(ctx context.Context, key string, fallback int) int {
	if s.settings == nil {
		return fallback
	}
	return s.settings.GetIntOr(ctx, key, fallback)
}

// ---------------------------------------------------------------------------
// Pull path
// ---------------------------------------------------------------------------

// Suggestions evaluates rules for the request and returns ranked, render-ready
// suggestions. Guests and members share the path; member impressions write
// user_next_actions rows, guest impressions accumulate in the Valkey hash.
func (s *Service) Suggestions(ctx context.Context, r *http.Request, surface string, limit int) ([]Suggestion, error) {
	state := s.resolver.Resolve(ctx, r, surface)

	// One-shot guest→user state merge (see ensureMerged).
	s.ensureMerged(ctx, state, r)

	cacheKey := s.cacheKey(state)
	if s.cache != nil {
		if state.UserID.Valid {
			if b, err := s.cache.Get(ctx, cacheKey).Bytes(); err == nil {
				var cached []Suggestion
				if json.Unmarshal(b, &cached) == nil {
					shown := sliceSuggestions(cached, limit)
					s.markDelivered(ctx, state, shown)
					return shown, nil
				}
			}
		} else if cached, ok := s.guestCacheRead(ctx, state.SessionKey, state.Surface); ok {
			shown := sliceSuggestions(cached, limit)
			s.markDelivered(ctx, state, shown)
			return shown, nil
		}
	}

	rules, err := s.loadRules(ctx)
	if err != nil {
		return nil, fmt.Errorf("nextactions: load rules: %w", err)
	}

	states := s.loadStates(ctx, state)
	out := make([]Suggestion, 0, len(rules))
	upsellShownToday := s.upsellShownToday(ctx, state)
	maxActive := s.settingInt(ctx, "nextactions.max_active_per_user", defaultMaxActive)
	maxUpsell := s.settingInt(ctx, "nextactions.max_upsell_per_day", defaultMaxUpsellPerDay)

	for i := range rules {
		rule := rules[i]
		if !s.audienceMatch(rule, state) {
			continue
		}
		// Saved state is checked before conditions: EvalConditions runs a
		// platform_events COUNT per event predicate — skipping suppressed
		// rules first avoids burning queries on rules that can never render.
		st := states[rule.ID.Bytes]
		if s.stateFiltered(rule, st) {
			continue
		}
		if !EvalConditions(ctx, rule.Conditions, state, s.deps) {
			continue
		}
		if rule.CompletionSignal != nil && EvalConditions(ctx, *rule.CompletionSignal, state, s.deps) {
			s.markCompleted(ctx, state, rule.ID)
			continue
		}
		if rule.Upsell && upsellShownToday >= int64(maxUpsell) {
			continue
		}
		out = append(out, s.toSuggestion(rule))
		if rule.Upsell {
			upsellShownToday++
		}
		if len(out) >= maxActive {
			break
		}
	}

	// Rules are already priority-ordered (ListEnabledNextActionRules ORDER BY
	// priority). Impressions fire on DELIVERY, not eval: only the `shown`
	// slice reaches the wire — items beyond `limit` were never sent to a
	// client and must not count as served (nor earn a serve marker that would
	// legitimise forged track calls). On later cache hits, markDelivered
	// counts items newly entering the response via the served-marker check.
	shown := sliceSuggestions(out, limit)
	s.markDelivered(ctx, state, shown)

	if s.cache != nil {
		if b, err := json.Marshal(out); err == nil {
			ttl := time.Duration(s.settingInt(ctx, "nextactions.cache_ttl_seconds", defaultCacheTTLSeconds)) * time.Second
			if state.UserID.Valid {
				_ = s.cache.Set(ctx, cacheKey, b, ttl).Err()
			} else {
				s.guestCacheWrite(ctx, state.SessionKey, state.Surface, b, ttl)
			}
		}
	}

	return shown, nil
}

// cacheKey — user and guest keys per the §122 catalog. The surface is part of
// the key because evaluation depends on it (audience.surfaces filter and the
// surface predicate): without it a `?surface=tools` eval would poison the
// `?surface=main` cache for the TTL. The guest result cache shares the state
// hash key via per-surface `_cache:{surface}` fields to keep one key.
func (s *Service) cacheKey(state *UserState) string {
	if state.UserID.Valid {
		return valkey.MustKey("nextactions:user:%s:%s", state.UserID.String(), state.Surface)
	}
	return valkey.MustKey("nextactions:guest:%s", state.SessionKey)
}

// guestCacheFieldName is the per-surface result-cache field inside the guest
// state hash.
func guestCacheFieldName(surface string) string { return "_cache:" + surface }

// servedMarkerTTL bounds how long after serving a rule the client may report
// lifecycle events for it — long enough that a CTA click hours after a
// session-cached render still lands, short enough that forged track calls for
// unserved rules get rejected.
const servedMarkerTTL = 24 * time.Hour

// ErrRuleNotServed — a /track call referenced a rule this identity was never
// served within the marker TTL (forged or expired). Maps to 404.
var ErrRuleNotServed = errors.New("nextactions: rule not served to this caller")

// servedKey — jc:nextactions:served:{user|guest}:{id}:{rule_id} evidence
// marker written on serve; /track requires it.
func (s *Service) servedKey(state *UserState, ruleID pgtype.UUID) string {
	id := state.SessionKey
	kind := "guest"
	if state.UserID.Valid {
		kind = "user"
		id = state.UserID.String()
	}
	return valkey.MustKey("nextactions:served:%s:%s:%s", kind, id, ruleID.String())
}

func (s *Service) markServed(ctx context.Context, state *UserState, ruleID pgtype.UUID) {
	if s.cache != nil {
		_ = s.cache.Set(ctx, s.servedKey(state, ruleID), "1", servedMarkerTTL).Err()
	}
}

// wasServed gates /track — the marker means this identity actually received
// the rule recently. Cache failure degrades open (analytics over suppression);
// a missing marker is a forged or stale call and is rejected.
func (s *Service) wasServed(ctx context.Context, state *UserState, ruleID pgtype.UUID) bool {
	if s.cache == nil {
		return true
	}
	n, err := s.cache.Exists(ctx, s.servedKey(state, ruleID)).Result()
	return err != nil || n > 0
}

// loadRules returns enabled in-window rules, cached 5 min in Valkey.
func (s *Service) loadRules(ctx context.Context) ([]Rule, error) {
	if s.cache != nil {
		if b, err := s.cache.Get(ctx, valkey.MustKey("nextactions:rules")).Bytes(); err == nil {
			var rules []Rule
			if json.Unmarshal(b, &rules) == nil {
				return rules, nil
			}
		}
	}
	rows, err := s.read.ListEnabledNextActionRules(ctx)
	if err != nil {
		return nil, err
	}
	rules := make([]Rule, 0, len(rows))
	for i := range rows {
		r, err := decodeRule(rows[i])
		if err != nil {
			slog.Warn("nextactions: skipping undecodable rule", "slug", rows[i].Slug, "error", err)
			continue
		}
		rules = append(rules, r)
	}
	if s.cache != nil {
		if b, err := json.Marshal(rules); err == nil {
			_ = s.cache.Set(ctx, valkey.MustKey("nextactions:rules"), b, rulesCacheTTL).Err()
		}
	}
	return rules, nil
}

// InvalidateRulesCache busts the rule list after an admin mutation.
func (s *Service) InvalidateRulesCache(ctx context.Context) {
	if s.cache != nil {
		_ = s.cache.Del(ctx, valkey.MustKey("nextactions:rules")).Err()
	}
}

// loadStates builds the rule_id → state map: user_next_actions rows for
// members, the guest Valkey hash otherwise.
func (s *Service) loadStates(ctx context.Context, state *UserState) map[[16]byte]userActionState {
	out := map[[16]byte]userActionState{}
	if s.read == nil {
		return out
	}
	if state.UserID.Valid {
		rows, err := s.read.GetUserNextActions(ctx, state.UserID)
		if err != nil {
			slog.Warn("nextactions: load user states failed", "error", err)
			return out
		}
		for _, r := range rows {
			out[r.RuleID.Bytes] = userActionState{
				RuleID: r.RuleID, Status: r.Status,
				Impressions: r.Impressions, LastShownAt: r.LastShownAt, SnoozedUntil: r.SnoozedUntil,
			}
		}
		return out
	}
	for rid, gs := range s.guestStates(ctx, state.SessionKey) {
		out[rid] = gs
	}
	return out
}

// stateFiltered reports whether a rule's saved state suppresses it.
func (s *Service) stateFiltered(rule Rule, st userActionState) bool {
	now := time.Now()
	switch st.Status {
	case StatusDismissed, StatusCompleted, StatusExpired:
		return true
	case StatusSnoozed:
		if st.SnoozedUntil.Valid && st.SnoozedUntil.Time.After(now) {
			return true
		}
	}
	if rule.Frequency.Mode == "once" && st.Impressions >= 1 {
		return true
	}
	if rule.Frequency.MaxImpressions > 0 && st.Impressions >= int32(rule.Frequency.MaxImpressions) {
		return true
	}
	if rule.Frequency.Mode == "recurring" && rule.Frequency.CooldownHours > 0 &&
		st.LastShownAt.Valid && st.LastShownAt.Time.Add(time.Duration(rule.Frequency.CooldownHours)*time.Hour).After(now) {
		return true
	}
	return false
}

// audienceMatch applies the rule's audience filters against the state.
func (s *Service) audienceMatch(rule Rule, state *UserState) bool {
	a := rule.Audience
	if len(a.UserClass) > 0 && !slices.Contains(a.UserClass, state.UserClass) {
		return false
	}
	if len(a.Surfaces) > 0 && !slices.Contains(a.Surfaces, state.Surface) {
		return false
	}
	if len(a.Archetype) > 0 && !slices.Contains(a.Archetype, state.VCtx.Archetype) {
		return false
	}
	if len(a.JapaStage) > 0 && !slices.Contains(a.JapaStage, state.VCtx.JapaStage) {
		return false
	}
	if len(a.DestinationCountry) > 0 && !slices.Contains(a.DestinationCountry, state.VCtx.DestinationCountry) {
		return false
	}
	if len(a.OriginCountry) > 0 && !slices.Contains(a.OriginCountry, state.VCtx.OriginCountry) {
		return false
	}
	return true
}

// audienceMatchSweep applies the same audience filters minus surfaces — the
// sweep has no surface; a surface restriction only constrains where a rule
// renders, not whether it may push.
func (s *Service) audienceMatchSweep(rule Rule, state *UserState) bool {
	a := rule.Audience
	if len(a.UserClass) > 0 && !slices.Contains(a.UserClass, state.UserClass) {
		return false
	}

	if len(a.Archetype) > 0 && !slices.Contains(a.Archetype, state.VCtx.Archetype) {
		return false
	}

	if len(a.JapaStage) > 0 && !slices.Contains(a.JapaStage, state.VCtx.JapaStage) {
		return false
	}

	if len(a.DestinationCountry) > 0 && !slices.Contains(a.DestinationCountry, state.VCtx.DestinationCountry) {
		return false
	}

	if len(a.OriginCountry) > 0 && !slices.Contains(a.OriginCountry, state.VCtx.OriginCountry) {
		return false
	}

	return true
}

func (s *Service) toSuggestion(rule Rule) Suggestion {
	sug := Suggestion{
		upsell: rule.Upsell,
		ID:     rule.ID.String(), RuleSlug: rule.Slug,
		Title: rule.Payload.Title, Body: rule.Payload.Body,
		CTALabel: rule.Payload.CTALabel, CTAURL: rule.Payload.CTAURL,
		Icon: rule.Payload.Icon, Channels: rule.Channels, Priority: rule.Priority,
		SnoozeDefaultHours: rule.Frequency.SnoozeDefaultHours,
	}
	return sug
}

func sliceSuggestions(in []Suggestion, limit int) []Suggestion {
	if limit > 0 && len(in) > limit {
		return in[:limit]
	}
	return in
}

// upsellShownToday counts upsell impressions in the last 24h — platform_events
// for members, the guest hash's upsell_day counter for guests.
func (s *Service) upsellShownToday(ctx context.Context, state *UserState) int64 {
	if state.UserID.Valid && s.read != nil {
		n, err := s.read.CountUpsellShownToday(ctx, state.UserID)
		if err == nil {
			return n
		}
	}
	return int64(s.guestUpsellToday(ctx, state.SessionKey))
}

// ---------------------------------------------------------------------------
// Impressions + lifecycle
// ---------------------------------------------------------------------------

// markDelivered records the impression for each suggestion actually being
// returned. The served marker doubles as "already counted" evidence, making
// the call idempotent across cold evals and cache-hit deliveries.
func (s *Service) markDelivered(ctx context.Context, state *UserState, shown []Suggestion) {
	for _, sug := range shown {
		ruleID := parseUUID(sug.ID)
		if !ruleID.Valid || s.wasServed(ctx, state, ruleID) {
			continue
		}
		s.recordImpression(ctx, state, sug)
	}
}

// ensureMerged runs the one-shot guest→user merge: the first authed request
// carrying the jc_guest cookie (or IP fallback) copies guest dismissals/
// completions into user_next_actions so pre-auth nudge fatigue doesn't
// resurface (§122). Called from Suggestions AND the lifecycle mutations —
// a signup-success complete() can arrive before the first authed pull.
func (s *Service) ensureMerged(ctx context.Context, state *UserState, r *http.Request) {
	if state.UserID.Valid && s.cache != nil {
		mergedKey := valkey.MustKey("nextactions:merged:%s", state.UserID.String())
		if ok, err := s.cache.SetNX(ctx, mergedKey, "1", 90*24*time.Hour).Result(); err == nil && ok {
			// Drop the marker when the merge fails so the next authed
			// request retries — otherwise a mid-merge DB blip loses the
			// guest's state for the marker's whole 90-day TTL.
			if err := s.MergeGuestState(ctx, state.UserID, guestSessionKey(r)); err != nil {
				slog.Warn("nextactions: guest merge failed, will retry", "user_id", state.UserID, "error", err)
				_ = s.cache.Del(ctx, mergedKey).Err()
			}
		}
	}
}

// recordImpression marks a served suggestion shown: DB row for members,
// guest hash + shown platform_event for guests.
func (s *Service) recordImpression(ctx context.Context, state *UserState, sug Suggestion) {
	ruleID := parseUUID(sug.ID)
	if !ruleID.Valid {
		return
	}
	if state.UserID.Valid {
		if s.write != nil {
			_ = s.write.UpsertUserNextAction(ctx, gen.UpsertUserNextActionParams{UserID: state.UserID, RuleID: ruleID})
			_ = s.write.MarkNextActionShown(ctx, gen.MarkNextActionShownParams{UserID: state.UserID, RuleID: ruleID})
		}
	} else {
		s.guestMarkShown(ctx, state.SessionKey, ruleID)
		if sug.upsell {
			s.guestUpsellBump(ctx, state.SessionKey)
		}
	}
	s.markServed(ctx, state, ruleID)
	// Server-authoritative shown event — the §40 funnel no longer depends on
	// clients calling track('shown'). Shipped renderers don't emit their own
	// shown events; the /track endpoint still accepts 'shown' for compat.
	s.trackSlug(ctx, state, ruleID, sug.RuleSlug, analytics.EventNextActionShown)
}

// lifecycleMutationAllowed gates dismiss/complete the same way wasServed
// gates /track, plus the sweep path: a member row materialized by the push
// sweep has a user_next_actions row but may never have been pull-served, so
// row existence counts as evidence too.
func (s *Service) lifecycleMutationAllowed(ctx context.Context, state *UserState, ruleID pgtype.UUID) bool {
	if s.wasServed(ctx, state, ruleID) {
		return true
	}
	if state.UserID.Valid && s.read != nil {
		has, err := s.read.HasUserNextAction(ctx, gen.HasUserNextActionParams{UserID: state.UserID, RuleID: ruleID})
		return err != nil || has // read failure degrades open
	}
	// Guests: the per-rule field in the state hash is itself serve evidence
	// (written by guestMarkShown; outlives the 24h marker via the 30d TTL).
	if !state.UserID.Valid && s.cache != nil && state.SessionKey != "" {
		has, err := s.cache.HExists(ctx, valkey.MustKey("nextactions:guest:%s", state.SessionKey), ruleID.String()).Result()
		return err != nil || has
	}
	return false
}

// Dismiss marks a suggestion dismissed (optional snooze_hours — a snooze is
// a temporary dismiss per §122 semantics).
func (s *Service) Dismiss(ctx context.Context, r *http.Request, ruleID pgtype.UUID, snoozeHours int) error {
	state := s.resolver.Resolve(ctx, r, "")
	s.ensureMerged(ctx, state, r) // a post-signup dismiss may arrive before the first authed pull
	if !s.lifecycleMutationAllowed(ctx, state, ruleID) {
		return fmt.Errorf("%w: rule %s", ErrRuleNotServed, ruleID.String())
	}
	if state.UserID.Valid {
		if s.write == nil {
			return errors.New("nextactions: no write pool")
		}
		// Upsert first — the row may not exist yet (e.g. served from the shared
		// rules cache without a recorded impression), and a silent 0-row UPDATE
		// would drop the dismissal.
		_ = s.write.UpsertUserNextAction(ctx, gen.UpsertUserNextActionParams{UserID: state.UserID, RuleID: ruleID})
		var err error
		if snoozeHours > 0 {
			err = s.write.SnoozeNextAction(ctx, gen.SnoozeNextActionParams{
				UserID: state.UserID, RuleID: ruleID, Hours: int32(snoozeHours), // #nosec G115 — bounded by handler clamp
			})
		} else {
			err = s.write.DismissNextAction(ctx, gen.DismissNextActionParams{UserID: state.UserID, RuleID: ruleID})
		}
		if err != nil {
			return fmt.Errorf("nextactions: dismiss: %w", err)
		}
	} else {
		s.guestSetStatus(ctx, state.SessionKey, ruleID, map[string]any{"status": statusFor(snoozeHours), "snoozed_until": snoozeUntil(snoozeHours)})
	}
	s.bustCache(ctx, state)
	s.track(ctx, state, ruleID, analytics.EventNextActionDismissed)
	return nil
}

// Complete marks a suggestion's action taken.
func (s *Service) Complete(ctx context.Context, r *http.Request, ruleID pgtype.UUID) error {
	state := s.resolver.Resolve(ctx, r, "")
	s.ensureMerged(ctx, state, r) // signup-success pending completion arrives before the first pull
	if !s.lifecycleMutationAllowed(ctx, state, ruleID) {
		return fmt.Errorf("%w: rule %s", ErrRuleNotServed, ruleID.String())
	}
	if state.UserID.Valid {
		if s.write == nil {
			return errors.New("nextactions: no write pool")
		}
		_ = s.write.UpsertUserNextAction(ctx, gen.UpsertUserNextActionParams{UserID: state.UserID, RuleID: ruleID})
		if err := s.write.CompleteNextAction(ctx, gen.CompleteNextActionParams{UserID: state.UserID, RuleID: ruleID}); err != nil {
			return fmt.Errorf("nextactions: complete: %w", err)
		}
	} else {
		s.guestSetStatus(ctx, state.SessionKey, ruleID, map[string]any{"status": StatusCompleted})
	}
	s.bustCache(ctx, state)
	s.track(ctx, state, ruleID, analytics.EventNextActionCompleted)
	return nil
}

// Track records a shown|clicked lifecycle event for the suggestion.
func (s *Service) Track(ctx context.Context, r *http.Request, ruleID pgtype.UUID, event string) error {
	state := s.resolver.Resolve(ctx, r, "")
	s.ensureMerged(ctx, state, r)
	var et analytics.EventType
	switch event {
	case "shown":
		et = analytics.EventNextActionShown
	case "clicked":
		et = analytics.EventNextActionClicked
	default:
		return fmt.Errorf("nextactions: unknown track event %q", event)
	}
	if !s.wasServed(ctx, state, ruleID) {
		return fmt.Errorf("%w: rule %s", ErrRuleNotServed, ruleID.String())
	}
	if et == analytics.EventNextActionShown && s.shownAlreadyEmitted(ctx, state, ruleID) {
		return nil // server-side recordImpression already counted this impression
	}
	s.track(ctx, state, ruleID, et)
	return nil
}

// shownAlreadyEmitted dedupes a client track('shown') against the server-side
// emission in recordImpression — external renderers can keep tracking without
// inflating the funnel.
func (s *Service) shownAlreadyEmitted(ctx context.Context, state *UserState, ruleID pgtype.UUID) bool {
	if s.read == nil {
		return false
	}
	slug := s.ruleSlug(ctx, ruleID)
	if slug == "" {
		return false
	}
	has, err := s.read.HasRecentNextActionShown(ctx, gen.HasRecentNextActionShownParams{
		UserID:    state.UserID,
		SessionID: state.SessionKey,
		RuleSlug:  slug,
		Seconds:   float64(s.settingInt(ctx, "nextactions.cache_ttl_seconds", defaultCacheTTLSeconds)),
	})
	return err == nil && has
}

// markCompleted is the completion_signal path — same as Complete minus the
// request dependency (used inside evaluation loops).
func (s *Service) markCompleted(ctx context.Context, state *UserState, ruleID pgtype.UUID) {
	if state.UserID.Valid && s.write != nil {
		// Upsert first so the terminal row persists even when the suggestion
		// was never materialized — otherwise the completion_signal re-evaluates
		// on every pass forever.
		_ = s.write.UpsertUserNextAction(ctx, gen.UpsertUserNextActionParams{UserID: state.UserID, RuleID: ruleID})
		_ = s.write.CompleteNextAction(ctx, gen.CompleteNextActionParams{UserID: state.UserID, RuleID: ruleID})
	} else if !state.UserID.Valid {
		s.guestSetStatus(ctx, state.SessionKey, ruleID, map[string]any{"status": StatusCompleted})
	}
}

// track writes a lifecycle platform_event; rule_slug joins to NextActionStats.
func (s *Service) track(ctx context.Context, state *UserState, ruleID pgtype.UUID, et analytics.EventType) {
	s.trackSlug(ctx, state, ruleID, s.ruleSlug(ctx, ruleID), et)
}

// trackSlug is track with the slug already in hand — used by recordImpression
// to emit the server-authoritative shown event without a rule lookup.
func (s *Service) trackSlug(ctx context.Context, state *UserState, ruleID pgtype.UUID, slug string, et analytics.EventType) {
	if s.tracker == nil {
		return
	}
	meta := map[string]any{}
	if slug != "" {
		meta["rule_slug"] = slug
	}
	_, _ = s.tracker.TrackEvent(ctx, analytics.TrackEventInput{
		EventType:  et,
		UserID:     state.UserID,
		EntityType: "next_action_rule",
		EntityID:   ruleID,
		Meta:       meta,
		SessionID:  state.SessionKey,
	})
}

func (s *Service) ruleSlug(ctx context.Context, ruleID pgtype.UUID) string {
	if s.read == nil {
		return ""
	}
	row, err := s.read.GetNextActionRule(ctx, ruleID)
	if err != nil {
		return ""
	}
	return row.Slug
}

func (s *Service) bustCache(ctx context.Context, state *UserState) {
	if s.cache == nil {
		return
	}
	if state.UserID.Valid {
		// User keys hold only the cached result lists — safe to delete. The
		// keys are surface-scoped (cacheKey), so clear every variant via SCAN:
		// the mutation must invalidate the suggestion on all surfaces.
		pattern := valkey.MustKey("nextactions:user:%s:*", state.UserID.String())
		var cursor uint64
		for {
			keys, next, err := s.cache.Scan(ctx, cursor, pattern, 50).Result()
			if err != nil {
				break
			}
			for _, k := range keys {
				_ = s.cache.Del(ctx, k).Err()
			}
			if next == 0 {
				break
			}
			cursor = next
		}
		return
	}
	// The guest key is the STATE hash — deleting it would wipe the dismissal/
	// completion/snooze just written. Only the per-surface _cache:* fields
	// are dropped.
	hk := valkey.MustKey("nextactions:guest:%s", state.SessionKey)
	if m, err := s.cache.HGetAll(ctx, hk).Result(); err == nil {
		fields := make([]string, 0, len(m))
		for f := range m {
			if strings.HasPrefix(f, "_cache") {
				fields = append(fields, f)
			}
		}
		if len(fields) > 0 {
			_ = s.cache.HDel(ctx, hk, fields...).Err()
		}
	}
}

// ---------------------------------------------------------------------------
// Guest state (Valkey hash — no Postgres rows for anonymous visitors)
// ---------------------------------------------------------------------------

// guestCacheFieldPrefix marks hash fields that hold per-surface result caches
// ("_cache:{surface}") — guestStates must skip them when decoding rule state.
const guestCacheFieldPrefix = "_cache"

// guestStates decodes the per-rule state map from the guest hash.
func (s *Service) guestStates(ctx context.Context, key string) map[[16]byte]userActionState {
	out := map[[16]byte]userActionState{}
	if s.cache == nil || key == "" {
		return out
	}
	m, err := s.cache.HGetAll(ctx, valkey.MustKey("nextactions:guest:%s", key)).Result()
	if err != nil {
		return out
	}
	for field, raw := range m {
		if strings.HasPrefix(field, guestCacheFieldPrefix) || strings.HasPrefix(field, sessionCounterFieldPrefix) {
			continue
		}
		var g struct {
			Status       string `json:"status"`
			Impressions  int32  `json:"impressions"`
			LastShownAt  int64  `json:"last_shown_at"`
			SnoozedUntil int64  `json:"snoozed_until"`
			UpsellDay    string `json:"uds"` // date for the upsell-day counter
			UpsellCount  int    `json:"uc"`
		}
		if err := json.Unmarshal([]byte(raw), &g); err != nil {
			continue
		}
		st := userActionState{Status: g.Status, Impressions: g.Impressions}
		if g.LastShownAt > 0 {
			st.LastShownAt = pgtype.Timestamptz{Time: time.Unix(g.LastShownAt, 0).UTC(), Valid: true}
		}
		if g.SnoozedUntil > 0 {
			st.SnoozedUntil = pgtype.Timestamptz{Time: time.Unix(g.SnoozedUntil, 0).UTC(), Valid: true}
		}
		var rid pgtype.UUID
		if err := rid.Scan(field); err == nil {
			out[rid.Bytes] = st
		}
	}
	return out
}

// guestMarkShown bumps impressions + status for a guest-served rule.
// Mirrors MarkNextActionShown's SQL guard — terminal states (dismissed,
// completed, expired) are never flipped back to shown.
func (s *Service) guestMarkShown(ctx context.Context, key string, ruleID pgtype.UUID) {
	if s.cache != nil && key != "" && ruleID.Valid {
		if raw, err := s.cache.HGet(ctx, valkey.MustKey("nextactions:guest:%s", key), ruleID.String()).Result(); err == nil {
			var cur struct {
				Status string `json:"status"`
			}
			if json.Unmarshal([]byte(raw), &cur) == nil &&
				(cur.Status == StatusDismissed || cur.Status == StatusCompleted || cur.Status == StatusExpired) {
				return
			}
		}
	}
	s.guestSetStatus(ctx, key, ruleID, map[string]any{
		"status":        StatusShown,
		"impressions+":  1,
		"last_shown_at": time.Now().Unix(),
	})
}

// guestSetStatus merges fields into a rule's guest-state entry.
func (s *Service) guestSetStatus(ctx context.Context, key string, ruleID pgtype.UUID, fields map[string]any) {
	if s.cache == nil || key == "" || !ruleID.Valid {
		return
	}
	hk := valkey.MustKey("nextactions:guest:%s", key)
	fk := ruleID.String()
	var cur map[string]any
	if raw, err := s.cache.HGet(ctx, hk, fk).Result(); err == nil {
		_ = json.Unmarshal([]byte(raw), &cur)
	}
	if cur == nil {
		cur = map[string]any{}
	}
	for k, v := range fields {
		if k == "impressions+" {
			cur["impressions"] = toFloat(cur["impressions"]) + toFloat(v)
			continue
		}
		cur[k] = v
	}
	if b, err := json.Marshal(cur); err == nil {
		_ = s.cache.HSet(ctx, hk, fk, string(b)).Err()
		_ = s.cache.Expire(ctx, hk, time.Duration(s.settingInt(ctx, "nextactions.guest_state_ttl_seconds", defaultGuestStateTTL))*time.Second).Err()
	}
}

// guestUpsellToday reads the guest hash's daily upsell counter.
func (s *Service) guestUpsellToday(ctx context.Context, key string) int {
	return s.guestUpsellRead(ctx, key).Count
}

type guestUpsellDay struct {
	Date  string `json:"d"`
	Count int    `json:"c"`
}

func (s *Service) guestUpsellRead(ctx context.Context, key string) guestUpsellDay {
	if s.cache == nil || key == "" {
		return guestUpsellDay{}
	}
	raw, err := s.cache.HGet(ctx, valkey.MustKey("nextactions:guest:%s", key), "upsell_day").Result()
	if err != nil {
		return guestUpsellDay{}
	}
	var v guestUpsellDay
	if json.Unmarshal([]byte(raw), &v) != nil || v.Date != time.Now().UTC().Format("2006-01-02") {
		return guestUpsellDay{}
	}
	return v
}

// guestUpsellBump increments the guest's daily upsell counter — the guest-side
// enforcement of nextactions.max_upsell_per_day.
func (s *Service) guestUpsellBump(ctx context.Context, key string) {
	if s.cache == nil || key == "" {
		return
	}
	v := s.guestUpsellRead(ctx, key)
	v.Date = time.Now().UTC().Format("2006-01-02")
	v.Count++
	if b, err := json.Marshal(v); err == nil {
		hk := valkey.MustKey("nextactions:guest:%s", key)
		_ = s.cache.HSet(ctx, hk, "upsell_day", string(b)).Err()
		_ = s.cache.Expire(ctx, hk, time.Duration(s.settingInt(ctx, "nextactions.guest_state_ttl_seconds", defaultGuestStateTTL))*time.Second).Err()
	}
}

// guestCacheRead returns the cached suggestion list from the guest hash's
// per-surface _cache:{surface} field ({exp, data}) — the hash key can't hold
// a GET-able string value, so the list lives in a field with its own
// embedded expiry.
func (s *Service) guestCacheRead(ctx context.Context, key, surface string) ([]Suggestion, bool) {
	if s.cache == nil || key == "" {
		return nil, false
	}
	raw, err := s.cache.HGet(ctx, valkey.MustKey("nextactions:guest:%s", key), guestCacheFieldName(surface)).Result()
	if err != nil {
		return nil, false
	}
	var v struct {
		Exp  int64        `json:"exp"`
		Data []Suggestion `json:"data"`
	}
	if json.Unmarshal([]byte(raw), &v) != nil || v.Exp <= time.Now().Unix() {
		return nil, false
	}
	return v.Data, true
}

func (s *Service) guestCacheWrite(ctx context.Context, key, surface string, data []byte, ttl time.Duration) {
	if key == "" {
		return
	}
	payload, err := json.Marshal(struct {
		Exp  int64           `json:"exp"`
		Data json.RawMessage `json:"data"`
	}{Exp: time.Now().Add(ttl).Unix(), Data: data})
	if err != nil {
		return
	}
	hk := valkey.MustKey("nextactions:guest:%s", key)
	_ = s.cache.HSet(ctx, hk, guestCacheFieldName(surface), string(payload)).Err()
	_ = s.cache.Expire(ctx, hk, time.Duration(s.settingInt(ctx, "nextactions.guest_state_ttl_seconds", defaultGuestStateTTL))*time.Second).Err()
}

// MergeGuestState copies guest states into user_next_actions at register/
// login — terminal statuses so nudges don't resurface, plus impressions,
// last_shown_at, and snoozed_until so frequency caps carry pre-auth history
// (MergeGuestNextAction handles the status precedence). Returns the first
// write error so ensureMerged can retry rather than burn the one-shot marker.
func (s *Service) MergeGuestState(ctx context.Context, userID pgtype.UUID, guestKey string) error {
	if s.write == nil || !userID.Valid || guestKey == "" {
		return nil
	}
	var firstErr error
	for rid, st := range s.guestStates(ctx, guestKey) {
		status := st.Status
		if status == "" {
			status = StatusPending
		}
		if err := s.write.MergeGuestNextAction(ctx, gen.MergeGuestNextActionParams{
			UserID:       userID,
			RuleID:       pgtype.UUID{Bytes: rid, Valid: true},
			Status:       status,
			Impressions:  st.Impressions,
			LastShownAt:  st.LastShownAt,
			SnoozedUntil: st.SnoozedUntil,
		}); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	// Attribute pre-auth platform_events to the new user — guest tool/usage
	// history must count for member-scoped predicates (e.g. a guest who ran
	// the CRS tool and registered must not be re-nudged to "try it").
	if n, err := s.write.BackfillGuestSessionEvents(ctx, gen.BackfillGuestSessionEventsParams{
		UserID:    userID,
		SessionID: &guestKey,
	}); err != nil {
		if firstErr == nil {
			firstErr = err
		}
	} else if n > 0 {
		slog.Info("nextactions: backfilled guest events", "user_id", userID, "events", n)
	}
	return firstErr
}

// ---------------------------------------------------------------------------
// Sweep (Task 10 wires the worker; the engine lives here)
// ---------------------------------------------------------------------------

// sweepLockTTL maps nextactions.sweep_interval_minutes to the run-lock TTL —
// 5 minutes shorter than the interval so the next hourly scheduler fire lands
// after expiry (a TTL equal to the interval could skip a fire by milliseconds
// and silently halve the cadence). Floored at 1 minute.
func sweepLockTTL(intervalMin int) time.Duration {
	ttl := time.Duration(intervalMin)*time.Minute - 5*time.Minute
	if ttl < time.Minute {
		return time.Minute
	}
	return ttl
}

// Sweep materializes user_next_actions rows for push-channel rules over
// recently-active users and dispatches them via the notifier. Auth-only —
// guests have no push channels. Ref: §122 sweep path.
func (s *Service) Sweep(ctx context.Context) error {
	if s.read == nil || s.write == nil {
		return errors.New("nextactions: no db pools")
	}
	if s.cache != nil {
		lock := valkey.MustKey("nextactions:sweep:lock")
		// The run-lock doubles as the cadence check: the scheduler fires
		// hourly and the lock lives for the configured
		// nextactions.sweep_interval_minutes — see sweepLockTTL.
		intervalMin := s.settingInt(ctx, "nextactions.sweep_interval_minutes", defaultSweepIntervalMinutes)
		if ok, err := s.cache.SetNX(ctx, lock, "1", sweepLockTTL(intervalMin)).Result(); err == nil && !ok {
			slog.Info("nextactions: sweep already running, skipping")
			return nil
		}
	}

	rules, err := s.loadRules(ctx)
	if err != nil {
		return fmt.Errorf("nextactions: sweep load rules: %w", err)
	}
	pushRules := make([]Rule, 0, len(rules))
	for _, r := range rules {
		if r.CompletionSignal != nil || slices.ContainsFunc(r.Channels, func(c string) bool { return slices.Contains(PushChannels, c) }) {
			pushRules = append(pushRules, r)
		}
	}
	if len(pushRules) == 0 {
		return nil
	}

	days := s.settingInt(ctx, "nextactions.sweep_activity_window_days", defaultSweepWindowDays)
	batch := defaultSweepBatch
	var after pgtype.UUID // keyset cursor — stable under concurrent writes
	for {
		users, err := s.read.ListActiveUsersForSweep(ctx, gen.ListActiveUsersForSweepParams{
			AfterID: after, Days: int32(days), BatchSize: int32(batch), // #nosec G115 — bounded
		})
		if err != nil {
			return fmt.Errorf("nextactions: sweep candidates: %w", err)
		}
		for _, uid := range users {
			s.sweepUser(ctx, uid, pushRules)
		}
		if len(users) < batch {
			break
		}
		after = users[len(users)-1]
	}
	// Terminal-state housekeeping — write 'expired' for rows whose rule is
	// disabled or past valid_until so they stop counting as live.
	if n, err := s.write.ExpireStaleUserNextActions(ctx); err != nil {
		slog.Warn("nextactions: expire stale rows failed", "error", err)
	} else if n > 0 {
		slog.Info("nextactions: expired stale suggestion rows", "rows", n)
	}
	return nil
}

// sweepUser evaluates push rules for one user. The sweep lacks a request —
// it synthesizes a state from DB signals (tier, account age, onboarding,
// event counts); geo/stage predicates resolve empty (sweep rules that need
// them simply won't fire on this path — fine for v1 notifications).
func (s *Service) sweepUser(ctx context.Context, userID pgtype.UUID, rules []Rule) {
	states := map[[16]byte]userActionState{}
	rows, err := s.read.GetUserNextActions(ctx, userID)
	if err == nil {
		for _, r := range rows {
			states[r.RuleID.Bytes] = userActionState{
				RuleID: r.RuleID, Status: r.Status, Impressions: r.Impressions,
				LastShownAt: r.LastShownAt, SnoozedUntil: r.SnoozedUntil,
			}
		}
	}

	// Full sweep state — tier, account signals, profile, and the member
	// VisitorContext (japa stage, archetype, countries) resolved off-request
	// via ResolveForUser, so all predicates evaluate on the sweep path.
	sweepState := s.resolver.ResolveSweep(ctx, userID)

	// The global caps apply on the sweep path too — without them a user
	// could receive every matching push rule each pass, and upsell pushes
	// could outnumber nextactions.max_upsell_per_day.
	maxActive := s.settingInt(ctx, "nextactions.max_active_per_user", defaultMaxActive)
	maxUpsell := s.settingInt(ctx, "nextactions.max_upsell_per_day", defaultMaxUpsellPerDay)
	upsellShownToday := s.upsellShownToday(ctx, sweepState)
	notified := 0

	for i := range rules {
		rule := rules[i]
		if !s.audienceMatchSweep(rule, sweepState) {
			continue
		}
		st := states[rule.ID.Bytes]
		if s.stateFiltered(rule, st) {
			continue
		}
		if !EvalConditions(ctx, rule.Conditions, sweepState, s.deps) {
			continue
		}
		if rule.CompletionSignal != nil && EvalConditions(ctx, *rule.CompletionSignal, sweepState, s.deps) {
			s.markCompleted(ctx, sweepState, rule.ID)
			continue
		}
		// Materialize + dispatch only for push-channel rules. Rules with a
		// completion signal but pull-only channels reach this point solely for
		// the auto-complete re-eval above — they must not notify.
		if !slices.ContainsFunc(rule.Channels, func(c string) bool { return slices.Contains(PushChannels, c) }) {
			continue
		}
		if notified >= maxActive {
			break
		}
		if rule.Upsell && upsellShownToday >= int64(maxUpsell) {
			continue
		}
		_ = s.write.UpsertUserNextAction(ctx, gen.UpsertUserNextActionParams{UserID: userID, RuleID: rule.ID})
		if s.notifier != nil {
			if err := s.notifier.NotifySuggestion(ctx, userID, rule); err != nil {
				slog.Warn("nextactions: sweep notify failed", "rule", rule.Slug, "error", err)
				continue // failed delivery must not consume impression budget
			}
		}
		notified++
		if rule.Upsell {
			upsellShownToday++
		}
		_ = s.write.MarkNextActionShown(ctx, gen.MarkNextActionShownParams{UserID: userID, RuleID: rule.ID})
		// A push/notification delivery IS a shown impression — emit so the
		// §40 funnel covers sweep-delivered suggestions, not just pull-rendered.
		s.trackSlug(ctx, sweepState, rule.ID, rule.Slug, analytics.EventNextActionShown)
	}
}

// ---------------------------------------------------------------------------
// decode + helpers
// ---------------------------------------------------------------------------

// decodeRule unmarshals a gen row's JSONB columns into a typed Rule.
func decodeRule(row gen.NextActionRule) (Rule, error) {
	r := Rule{
		ID: row.ID, Slug: row.Slug, Name: row.Name, Description: row.Description,
		Enabled: row.Enabled, Priority: row.Priority, Upsell: row.Upsell,
		ValidFrom: row.ValidFrom, ValidUntil: row.ValidUntil,
		CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	if err := json.Unmarshal(row.Audience, &r.Audience); err != nil {
		return r, fmt.Errorf("audience: %w", err)
	}
	if err := json.Unmarshal(row.Conditions, &r.Conditions); err != nil {
		return r, fmt.Errorf("conditions: %w", err)
	}
	if err := json.Unmarshal(row.Payload, &r.Payload); err != nil {
		return r, fmt.Errorf("payload: %w", err)
	}
	if err := json.Unmarshal(row.Channels, &r.Channels); err != nil {
		return r, fmt.Errorf("channels: %w", err)
	}
	if err := json.Unmarshal(row.Frequency, &r.Frequency); err != nil {
		return r, fmt.Errorf("frequency: %w", err)
	}
	if len(row.CompletionSignal) > 0 {
		var g ConditionGroup
		if err := json.Unmarshal(row.CompletionSignal, &g); err != nil {
			return r, fmt.Errorf("completion_signal: %w", err)
		}
		r.CompletionSignal = &g
	}
	return r, nil
}

func parseUUID(s string) pgtype.UUID {
	var id pgtype.UUID
	if err := id.Scan(s); err != nil {
		return pgtype.UUID{}
	}
	return id
}

func statusFor(snoozeHours int) string {
	if snoozeHours > 0 {
		return StatusSnoozed
	}
	return StatusDismissed
}

func snoozeUntil(hours int) int64 {
	if hours <= 0 {
		return 0
	}
	return time.Now().Add(time.Duration(hours) * time.Hour).Unix()
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int32:
		return float64(n)
	default:
		return 0
	}
}
