package jce

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/comparison"
	"github.com/jedafe/japa/internal/comparison/resolver"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/features"
	"github.com/jedafe/japa/internal/rbac"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// IngestionService is the public interface for JCE ingestion/refresh
// operations. It is implemented by *Service.
type IngestionService interface {
	RefreshComparison(ctx context.Context, contentID, userID pgtype.UUID, sourceKind *string) (IngestionJob, error)
	GetIngestionJob(ctx context.Context, id pgtype.UUID) (IngestionJob, error)
	ListIngestionJobs(ctx context.Context, contentID pgtype.UUID, limit, offset int) ([]IngestionJob, error)
	ProcessRefreshValues(ctx context.Context, contentID, jobID pgtype.UUID, sourceKind *string, userID pgtype.UUID) error
	ScheduleStaleRefresh(ctx context.Context, userID pgtype.UUID) (int, error)
}

// ScoringEngine is the contract the service needs from the JapaScore engine.
// It is kept in the jce package so the service can call the engine without
// creating an import cycle with the scoring subpackage.
type ScoringEngine interface {
	Score(ctx context.Context, input ScoreInput) (ScoreResult, error)
	GetScore(ctx context.Context, input GetScoreInput) (ScoreResult, error)
	PreviewScoreForItems(ctx context.Context, input ScorePreviewInput) (ScoreResult, error)
}

// scoringEngineFactory constructs a ScoringEngine. It is populated by the
// scoring subpackage init so the service package can remain decoupled and
// avoid an import cycle.
var scoringEngineFactory func(
	database *db.DB,
	compSvc *comparison.Service,
	r *resolver.Registry,
	settings *config.Settings,
	vk valkeycompat.Cmdable,
) ScoringEngine

// RegisterScoringEngineFactory registers the engine factory. The scoring
// subpackage calls this from its init function.
func RegisterScoringEngineFactory(factory func(
	database *db.DB,
	compSvc *comparison.Service,
	r *resolver.Registry,
	settings *config.Settings,
	vk valkeycompat.Cmdable,
) ScoringEngine,
) {
	scoringEngineFactory = factory
}

// validDataSources is the set of data_source values accepted by score factors.
var validDataSources = map[string]bool{
	"dimension":         true,
	"resolver_metadata": true,
	"profile":           true,
	"eligibility":       true,
	"aggregate":         true,
}

// safeInt32 converts an int to int32, clamping to [0, math.MaxInt32] to
// prevent integer overflow (gosec G115).
func safeInt32(n int) int32 {
	if n < 0 {
		return 0
	}

	if n > math.MaxInt32 {
		return math.MaxInt32
	}

	return int32(n)
}

// taskEnqueuer is the subset of the worker enqueuer used by the JCE service.
// This avoids an import cycle with internal/worker.
type taskEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// aiDispatcher is the narrow interface the JCE service needs for ai_estimate
// fills and the AI advisor. It is implemented by an adapter in the server
// package so the jce package does not import the full ai subsystem and create
// an import cycle.
// // Ref: docs/superpowers/specs/2026-08-30-jce-subspec-4-design.md §10.
type aiDispatcher interface {
	// Generate returns a plain text completion for the given category and prompt.
	// systemContext is an optional system prompt; model and maxTokens are passed
	// through to the underlying AI gateway when non-empty / positive; otherwise
	// the gateway's configured defaults are used.
	Generate(ctx context.Context, category, prompt, systemContext, model string, maxTokens int) (string, error)
}

// Service orchestrates JCE profiles, score factors, scoring, recommendations,
// and data-source-backed value refresh.
type Service struct {
	db          *db.DB
	compSvc     *comparison.Service
	r           *resolver.Registry
	settings    *config.Settings
	vk          valkeycompat.Cmdable
	features    features.FeatureChecker
	enqueuer    taskEnqueuer
	dispatcher  aiDispatcher
	profiles    *profileStore
	factors     *scoreFactorStore
	recs        *recommendationStore
	dataSources *dataSourceStore
	ingestion   *ingestionStore
	engine      ScoringEngine
	sources     dataSourceRegistry
}

// NewService creates a JCE service backed by the given dependencies. The
// scoring engine is constructed via a factory registered by the scoring
// subpackage when it is linked, which avoids an import cycle.
// // The service does not create its own feature checker; callers must inject
// one with SetFeatureChecker (usually the shared checker from the server).
func NewService(
	database *db.DB,
	compSvc *comparison.Service,
	r *resolver.Registry,
	settings *config.Settings,
	vk valkeycompat.Cmdable,
) *Service {
	svc := &Service{
		db:          database,
		compSvc:     compSvc,
		r:           r,
		settings:    settings,
		vk:          vk,
		profiles:    newProfileStore(database),
		factors:     newScoreFactorStore(database),
		recs:        newRecommendationStore(database),
		dataSources: newDataSourceStore(database),
		ingestion:   newIngestionStore(database),
	}
	if scoringEngineFactory != nil {
		svc.engine = scoringEngineFactory(database, compSvc, r, settings, vk)
	}

	return svc
}

// SetFeatureChecker injects the shared feature checker. The service does not
// own the checker and must not close it.
func (s *Service) SetFeatureChecker(c features.FeatureChecker) {
	s.features = c
}

// SetEnqueuer injects the Asynq task enqueuer. The service does not own it.
func (s *Service) SetEnqueuer(e taskEnqueuer) {
	s.enqueuer = e
}

// SetAIDispatcher injects the AI dispatcher used for ai_estimate fills.
// The service does not own it.
func (s *Service) SetAIDispatcher(d aiDispatcher) {
	s.dispatcher = d
}

// SetSourceRegistry links JCE data sources to the canonical intel_sources
// registry (v2 source directory).
func (s *Service) SetSourceRegistry(r dataSourceRegistry) {
	s.sources = r
	s.dataSources.setSourceRegistry(r)
}

// Close stops background goroutines owned by the service.
// // The service does not close the injected feature checker; the caller that
// created the checker is responsible for stopping it.
func (s *Service) Close() {}

// deleteFreshnessCache removes the cached freshness summary for a comparison.
func (s *Service) deleteFreshnessCache(ctx context.Context, contentID pgtype.UUID) {
	if s.vk == nil {
		return
	}

	_ = s.vk.Del(ctx, freshnessCacheKey(contentID)).Err()
}

// deleteAdvisorCacheForNoProfile removes advisor results where the caller did
// not supply a profile_id, i.e. the key segment is "none".
func (s *Service) deleteAdvisorCacheForNoProfile(ctx context.Context) {
	if s.vk == nil {
		return
	}

	pattern := fmt.Sprintf("jc:jce:advisor::*:none:*")

	var cursor uint64

	for {
		keys, nextCursor, err := s.vk.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			slog.Warn("jce: failed to scan advisor no-profile cache for invalidation", "error", err)
			return
		}

		if len(keys) > 0 {
			_ = s.vk.Del(ctx, keys...).Err()
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
}

// deleteAdvisorCache removes all cached advisor results for a comparison.
func (s *Service) deleteAdvisorCache(ctx context.Context, contentID pgtype.UUID) {
	if s.vk == nil {
		return
	}

	pattern := fmt.Sprintf("jc:jce:advisor::%s:*", contentID.String())

	var cursor uint64

	for {
		keys, nextCursor, err := s.vk.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			slog.Warn("jce: failed to scan advisor cache for invalidation", "error", err)
			return
		}

		if len(keys) > 0 {
			_ = s.vk.Del(ctx, keys...).Err()
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
}

// deleteAdvisorCacheForProfile removes all cached advisor results tied to a
// comparison profile across every comparison.
func (s *Service) deleteAdvisorCacheForProfile(ctx context.Context, profileID pgtype.UUID) {
	if s.vk == nil {
		return
	}

	pattern := fmt.Sprintf("jc:jce:advisor::*:%s:*", profileID.String())

	var cursor uint64

	for {
		keys, nextCursor, err := s.vk.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			slog.Warn("jce: failed to scan advisor profile cache for invalidation", "error", err)
			return
		}

		if len(keys) > 0 {
			_ = s.vk.Del(ctx, keys...).Err()
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
}

// deleteJceCacheByPattern deletes all Valkey keys matching a pattern.
// It is used for tenant-wide invalidations (e.g. data source changes).
func (s *Service) deleteJceCacheByPattern(ctx context.Context, pattern string) {
	if s.vk == nil {
		return
	}

	var cursor uint64

	for {
		keys, nextCursor, err := s.vk.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			slog.Warn("jce: failed to scan cache for invalidation", "pattern", pattern, "error", err)
			return
		}

		if len(keys) > 0 {
			_ = s.vk.Del(ctx, keys...).Err()
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
}

// invalidateProfileCache removes the Valkey cache entry for a single profile
// and every cached ScoreResult that was computed for that profile.
// It uses the profile owner's user id, not the caller's, so that admin edits
// clear the correct owner's cache.
func (s *Service) invalidateProfileCache(ctx context.Context, ownerUserID *string, profileID pgtype.UUID) {
	if s.vk == nil {
		return
	}

	uid := pgtype.UUID{}
	if ownerUserID != nil {
		uid = stringToUUID(*ownerUserID)
	}

	if uid.Valid {
		_ = s.vk.Del(ctx, ProfileCacheKey(uid, profileID)).Err()
	}

	s.invalidateScoreCacheForProfile(ctx, profileID)
}

// invalidateScoreCacheForProfile removes every cached ScoreResultKey for a
// profile by reading the tracking set written by the scoring engine.
func (s *Service) invalidateScoreCacheForProfile(ctx context.Context, profileID pgtype.UUID) {
	if s.vk == nil || !profileID.Valid {
		return
	}

	setKey := ScoreResultSetKey(profileID)

	keys, err := s.vk.SMembers(ctx, setKey).Result()
	if err != nil {
		return
	}

	if len(keys) > 0 {
		_ = s.vk.Del(ctx, keys...).Err()
	}

	_ = s.vk.Del(ctx, setKey).Err()
}

// userIDFromContext extracts the user id from the request context, or returns
// an invalid UUID when the caller is unauthenticated.
func (s *Service) userIDFromContext(ctx context.Context) pgtype.UUID {
	if u := auth.FromContext(ctx); u != nil {
		return u.UserID
	}

	return pgtype.UUID{}
}

// permissionChecker returns a closure that checks a permission for the
// currently authenticated user. A nil RBAC resolver in the context is treated
// as a deny-all fallback.
func (s *Service) permissionChecker(ctx context.Context, userID pgtype.UUID) func(string) bool {
	r := rbac.FromContext(ctx)
	if r == nil {
		return func(string) bool { return false }
	}

	return func(perm string) bool {
		return r.HasPermission(ctx, userID, perm)
	}
}

// featureEnabled is a nil-safe feature flag check.
func (s *Service) featureEnabled(ctx context.Context, key string) bool {
	if s.features == nil {
		return false
	}

	return s.features.IsEnabled(ctx, key)
}

// ownsProfile reports whether the caller owns a non-template profile.
func (s *Service) ownsProfile(p ComparisonProfile, userID pgtype.UUID) bool {
	if !userID.Valid || p.UserID == nil {
		return false
	}

	return *p.UserID == userID.String()
}

// canAccessComparison checks comparison visibility for the caller. It matches
// the scoring engine's canScoreContent logic and is reused by the service for
// recommendations.
func (s *Service) canAccessComparison(cmp comparison.JCEComparison, userID pgtype.UUID, check func(string) bool, publicPerm string) bool {
	if !userID.Valid {
		return false
	}

	if cmp.Content.AuthorID.Valid && cmp.Content.AuthorID == userID {
		return true
	}

	if check == nil {
		return false
	}

	if check("comparison.edit_any") {
		return true
	}

	if cmp.Content.Visibility == content.VisibilityPublic && check(publicPerm) {
		return true
	}

	return false
}

// canViewJCEComparison checks whether the caller may view a comparison.
// It mirrors the logic in internal/comparison/jce_handlers.go.
func (s *Service) canViewJCEComparison(ctx context.Context, cmp comparison.JCEComparison, userID pgtype.UUID) bool {
	if !userID.Valid {
		return false
	}
	// Public published detailed comparisons are always visible.
	if cmp.Content.Status == content.StatusPublished &&
		cmp.Content.Visibility == content.VisibilityPublic &&
		cmp.Metadata.Depth == "detailed" {
		return true
	}
	// Owner.
	if cmp.Content.AuthorID.Valid && cmp.Content.AuthorID == userID {
		return true
	}
	// Admin with edit_any.
	check := s.permissionChecker(ctx, userID)
	if check("comparison.edit_any") {
		return true
	}

	return false
}

// allowedGoal parses an allowed goal entry from type or settings JSON.
type allowedGoal struct {
	Key string `json:"key"`
}

// comparisonTypeGoalConfig is the subset of comparison_type_definitions.config
// that contributes valid goal keys.
type comparisonTypeGoalConfig struct {
	AllowedGoals  []allowedGoal       `json:"allowed_goals"`
	GoalFactorMap map[string][]string `json:"goal_factor_map"`
}

// defaultAllowedGoalKeys returns the built-in goal keys used when no tenant
// configuration is available.
func defaultAllowedGoalKeys() []string {
	return []string{
		"pr_fast",
		"low_cost",
		"high_income",
		"family_friendly",
		"scholarships",
		"language_ease",
	}
}

// allowedGoalKeys returns the set of goal keys that may be used in a profile.
// It starts with built-in defaults, then overlays jce.default_goals from
// settings and the allowed_goals/goal_factor_map from each comparison type.
func (s *Service) allowedGoalKeys(ctx context.Context) (map[string]bool, error) {
	keys := make(map[string]bool, len(defaultAllowedGoalKeys()))
	for _, k := range defaultAllowedGoalKeys() {
		keys[k] = true
	}

	if s.settings != nil {
		raw := s.settings.GetStringOr(ctx, "jce.default_goals", "")
		if raw != "" {
			var goals []allowedGoal
			if err := json.Unmarshal([]byte(raw), &goals); err == nil {
				for _, g := range goals {
					if g.Key == "" {
						continue
					}

					keys[g.Key] = true
				}
			}
		}
	}

	if s.compSvc != nil {
		types, err := s.compSvc.ListComparisonTypes(ctx, 1000, 0)
		if err != nil {
			return nil, fmt.Errorf("loading comparison types for goal validation: %w", err)
		}

		for _, t := range types {
			if len(t.Config) == 0 || string(t.Config) == "null" || string(t.Config) == "{}" {
				continue
			}

			var cfg comparisonTypeGoalConfig
			if err := json.Unmarshal(t.Config, &cfg); err != nil {
				continue
			}

			for _, g := range cfg.AllowedGoals {
				if g.Key == "" {
					continue
				}

				keys[g.Key] = true
			}

			for k := range cfg.GoalFactorMap {
				if k == "" {
					continue
				}

				keys[k] = true
			}
		}
	}

	return keys, nil
}

// factorKeysForTenant returns all factor keys visible to the tenant.
func (s *Service) factorKeysForTenant(ctx context.Context) (map[string]bool, error) {
	factors, err := s.factors.listByType(ctx, pgtype.UUID{}, true)
	if err != nil {
		return nil, fmt.Errorf("loading tenant factor keys: %w", err)
	}

	keys := make(map[string]bool, len(factors))
	for _, f := range factors {
		keys[f.FactorKey] = true
	}

	return keys, nil
}

// validateProfileInput validates the profile input. It is used for creates and
// full updates.
func (s *Service) validateProfileInput(ctx context.Context, in ComparisonProfileInput, isTemplate bool) error {
	if in.Title == "" {
		return fmt.Errorf("%w: title is required", errpkg.ErrInvalid)
	}

	if !isTemplate && len(in.TargetSpaceIDs) == 0 {
		return fmt.Errorf("%w: at least one target space is required", errpkg.ErrInvalid)
	}

	allowed, err := s.allowedGoalKeys(ctx)
	if err != nil {
		return fmt.Errorf("loading allowed goals: %w", err)
	}

	for _, g := range in.Goals {
		if !allowed[g.Key] {
			return fmt.Errorf("%w: goal %q is not allowed", errpkg.ErrInvalid, g.Key)
		}
	}

	factorKeys, err := s.factorKeysForTenant(ctx)
	if err != nil {
		return fmt.Errorf("loading factor keys: %w", err)
	}

	for k := range in.Preferences {
		if !factorKeys[k] {
			return fmt.Errorf("%w: preference %q is not a known factor", errpkg.ErrInvalid, k)
		}
	}

	return nil
}

// mergeProfileInput merges an existing profile with a partial update input.
func (s *Service) mergeProfileInput(existing ComparisonProfile, in ComparisonProfileInput) ComparisonProfileInput {
	out := in
	if out.Title == "" {
		out.Title = existing.Title
	}

	if out.SourceSpaceID == nil {
		out.SourceSpaceID = existing.SourceSpaceID
	}

	if out.TargetSpaceIDs == nil {
		out.TargetSpaceIDs = existing.TargetSpaceIDs
	}

	if out.Goals == nil {
		out.Goals = existing.Goals
	}

	if out.Preferences == nil {
		out.Preferences = existing.Preferences
	}

	if out.Demographics == (ProfileDemographics{}) {
		out.Demographics = existing.Demographics
	}
	// Preserve immutable flags on a normal update. The default flag can only be
	// changed through SetDefaultComparisonProfile, and the template flag is
	// controlled by creation/admin logic.
	out.IsDefault = existing.IsDefault
	out.IsTemplate = existing.IsTemplate

	return out
}

// CreateComparisonProfile creates a new comparison profile.
func (s *Service) CreateComparisonProfile(ctx context.Context, userID pgtype.UUID, in ComparisonProfileInput) (ComparisonProfile, error) {
	if !s.featureEnabled(ctx, "feature.jce.profiles") {
		return ComparisonProfile{}, fmt.Errorf("%w: feature.jce.profiles disabled", errpkg.ErrFeatureDisabled)
	}

	if in.IsTemplate {
		if !s.permissionChecker(ctx, userID)("comparison.edit_any") {
			return ComparisonProfile{}, fmt.Errorf("%w: only admins can create profile templates", errpkg.ErrForbidden)
		}

		userID = pgtype.UUID{}
	} else {
		if !userID.Valid {
			return ComparisonProfile{}, fmt.Errorf("%w: authentication required", errpkg.ErrForbidden)
		}

		if !s.permissionChecker(ctx, userID)("jce.profile.create") {
			return ComparisonProfile{}, fmt.Errorf("%w: missing jce.profile.create", errpkg.ErrForbidden)
		}
	}

	if err := s.validateProfileInput(ctx, in, in.IsTemplate); err != nil {
		return ComparisonProfile{}, err
	}

	createIn := in
	if in.IsDefault && !in.IsTemplate {
		createIn.IsDefault = false
	}

	// Create (and optionally set as default) inside a single transaction so the
	// default flag is never persisted without the profile row.
	p, err := db.WithTxResult(ctx, s.db.Writer, func(q *gen.Queries) (ComparisonProfile, error) {
		p, err := s.profiles.createWithTx(ctx, q, userID, createIn)
		if err != nil {
			return ComparisonProfile{}, err
		}

		if in.IsDefault && !in.IsTemplate {
			if err := s.profiles.setDefaultWithTx(ctx, q, stringToUUID(p.ID), userID); err != nil {
				return ComparisonProfile{}, err
			}

			p.IsDefault = true
		}

		return p, nil
	})
	if err != nil {
		return ComparisonProfile{}, fmt.Errorf("creating comparison profile: %w", err)
	}

	s.invalidateProfileCache(ctx, p.UserID, stringToUUID(p.ID))

	if in.IsDefault && !in.IsTemplate {
		s.deleteAdvisorCacheForNoProfile(ctx)
	}

	return p, nil
}

// ListComparisonProfiles returns the caller's profiles plus tenant templates.
// When all is true and the caller has comparison.edit_any, it returns all
// non-deleted tenant profiles (excluding templates) plus templates.
func (s *Service) ListComparisonProfiles(ctx context.Context, userID pgtype.UUID, includeTemplates, all bool) ([]ComparisonProfile, error) {
	if !s.featureEnabled(ctx, "feature.jce.profiles") {
		return nil, fmt.Errorf("%w: feature.jce.profiles disabled", errpkg.ErrFeatureDisabled)
	}

	if !s.permissionChecker(ctx, userID)("jce.profile.list") {
		return nil, fmt.Errorf("%w: missing jce.profile.list", errpkg.ErrForbidden)
	}

	var owned []ComparisonProfile

	var err error
	if all && s.permissionChecker(ctx, userID)("comparison.edit_any") {
		owned, err = s.profiles.listByTenant(ctx)
	} else {
		owned, err = s.profiles.listByUser(ctx, userID)
	}

	if err != nil {
		return nil, fmt.Errorf("listing comparison profiles: %w", err)
	}

	if includeTemplates {
		templates, err := s.profiles.listTemplates(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing profile templates: %w", err)
		}

		owned = append(owned, templates...)
	}

	return owned, nil
}

// GetComparisonProfile returns a profile if the caller owns it, it is a
// template, or the caller has comparison.edit_any.
func (s *Service) GetComparisonProfile(ctx context.Context, id, userID pgtype.UUID) (ComparisonProfile, error) {
	if !s.featureEnabled(ctx, "feature.jce.profiles") {
		return ComparisonProfile{}, fmt.Errorf("%w: feature.jce.profiles disabled", errpkg.ErrFeatureDisabled)
	}

	if !s.permissionChecker(ctx, userID)("jce.profile.view") {
		return ComparisonProfile{}, fmt.Errorf("%w: missing jce.profile.view", errpkg.ErrForbidden)
	}

	p, err := s.profiles.get(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ComparisonProfile{}, fmt.Errorf("%w: profile not found", errpkg.ErrNotFound)
		}

		return ComparisonProfile{}, fmt.Errorf("getting comparison profile: %w", err)
	}

	if !s.ownsProfile(p, userID) && !p.IsTemplate && !s.permissionChecker(ctx, userID)("comparison.edit_any") {
		return ComparisonProfile{}, fmt.Errorf("%w: not profile owner", errpkg.ErrForbidden)
	}

	return p, nil
}

// UpdateComparisonProfile updates a profile and returns the updated model.
func (s *Service) UpdateComparisonProfile(ctx context.Context, id, userID pgtype.UUID, in ComparisonProfileInput) (ComparisonProfile, error) {
	if !s.featureEnabled(ctx, "feature.jce.profiles") {
		return ComparisonProfile{}, fmt.Errorf("%w: feature.jce.profiles disabled", errpkg.ErrFeatureDisabled)
	}

	if !s.permissionChecker(ctx, userID)("jce.profile.update") {
		return ComparisonProfile{}, fmt.Errorf("%w: missing jce.profile.update", errpkg.ErrForbidden)
	}

	existing, err := s.profiles.get(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ComparisonProfile{}, fmt.Errorf("%w: profile not found", errpkg.ErrNotFound)
		}

		return ComparisonProfile{}, fmt.Errorf("getting comparison profile: %w", err)
	}

	if existing.IsTemplate {
		if !s.permissionChecker(ctx, userID)("comparison.edit_any") {
			return ComparisonProfile{}, fmt.Errorf("%w: only admins can update templates", errpkg.ErrForbidden)
		}
	} else if !s.ownsProfile(existing, userID) && !s.permissionChecker(ctx, userID)("comparison.edit_any") {
		return ComparisonProfile{}, fmt.Errorf("%w: not profile owner", errpkg.ErrForbidden)
	}

	merged := s.mergeProfileInput(existing, in)
	if err := s.validateProfileInput(ctx, merged, existing.IsTemplate); err != nil {
		return ComparisonProfile{}, err
	}

	// Update and optionally set as default inside one transaction so the two
	// mutations cannot diverge.
	p, err := db.WithTxResult(ctx, s.db.Writer, func(q *gen.Queries) (ComparisonProfile, error) {
		p, err := s.profiles.updateWithTx(ctx, q, id, userID, merged)
		if err != nil {
			return ComparisonProfile{}, err
		}

		if !existing.IsTemplate && in.IsDefault && !p.IsDefault {
			if err := s.profiles.setDefaultWithTx(ctx, q, id, userID); err != nil {
				return ComparisonProfile{}, err
			}

			p.IsDefault = true
		}

		return p, nil
	})
	if err != nil {
		return ComparisonProfile{}, fmt.Errorf("updating comparison profile: %w", err)
	}

	s.invalidateProfileCache(ctx, p.UserID, id)
	s.deleteAdvisorCacheForProfile(ctx, id)

	if !p.IsTemplate && in.IsDefault {
		s.deleteAdvisorCacheForNoProfile(ctx)
	}

	return p, nil
}

// DeleteComparisonProfile soft-deletes a profile by id.
func (s *Service) DeleteComparisonProfile(ctx context.Context, id, userID pgtype.UUID) error {
	if !s.featureEnabled(ctx, "feature.jce.profiles") {
		return fmt.Errorf("%w: feature.jce.profiles disabled", errpkg.ErrFeatureDisabled)
	}

	if !s.permissionChecker(ctx, userID)("jce.profile.delete") {
		return fmt.Errorf("%w: missing jce.profile.delete", errpkg.ErrForbidden)
	}

	existing, err := s.profiles.get(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: profile not found", errpkg.ErrNotFound)
		}

		return fmt.Errorf("getting comparison profile: %w", err)
	}

	if existing.IsTemplate {
		if !s.permissionChecker(ctx, userID)("comparison.edit_any") {
			return fmt.Errorf("%w: only admins can delete templates", errpkg.ErrForbidden)
		}
	} else if !s.ownsProfile(existing, userID) && !s.permissionChecker(ctx, userID)("comparison.edit_any") {
		return fmt.Errorf("%w: not profile owner", errpkg.ErrForbidden)
	}

	s.invalidateProfileCache(ctx, existing.UserID, id)
	s.deleteAdvisorCacheForProfile(ctx, id)

	if !existing.IsTemplate && existing.IsDefault {
		s.deleteAdvisorCacheForNoProfile(ctx)
	}

	if err := s.profiles.delete(ctx, id); err != nil {
		return fmt.Errorf("deleting comparison profile: %w", err)
	}

	return nil
}

// SetDefaultComparisonProfile marks a profile as the user's default.
func (s *Service) SetDefaultComparisonProfile(ctx context.Context, id, userID pgtype.UUID) error {
	if !s.featureEnabled(ctx, "feature.jce.profiles") {
		return fmt.Errorf("%w: feature.jce.profiles disabled", errpkg.ErrFeatureDisabled)
	}

	if !s.permissionChecker(ctx, userID)("jce.profile.update") {
		return fmt.Errorf("%w: missing jce.profile.update", errpkg.ErrForbidden)
	}

	if !userID.Valid {
		return fmt.Errorf("%w: authentication required", errpkg.ErrForbidden)
	}

	existing, err := s.profiles.get(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: profile not found", errpkg.ErrNotFound)
		}

		return fmt.Errorf("getting comparison profile: %w", err)
	}

	if existing.IsTemplate {
		return fmt.Errorf("%w: templates cannot be set as default", errpkg.ErrInvalid)
	}

	if !s.ownsProfile(existing, userID) && !s.permissionChecker(ctx, userID)("comparison.edit_any") {
		return fmt.Errorf("%w: not profile owner", errpkg.ErrForbidden)
	}

	if err := s.profiles.setDefault(ctx, id, userID); err != nil {
		return fmt.Errorf("setting default profile: %w", err)
	}

	s.invalidateProfileCache(ctx, existing.UserID, id)
	s.deleteAdvisorCacheForNoProfile(ctx)

	return nil
}

// GetProfileStats returns per-source-space profile counts. When all is true
// and the caller has comparison.edit_any, it returns aggregate anonymized
// counts across the tenant.
func (s *Service) GetProfileStats(ctx context.Context, userID pgtype.UUID, all bool) ([]ProfileStat, error) {
	if !s.featureEnabled(ctx, "feature.jce.profiles") {
		return nil, fmt.Errorf("%w: feature.jce.profiles disabled", errpkg.ErrFeatureDisabled)
	}

	if !s.permissionChecker(ctx, userID)("jce.profile.list") {
		return nil, fmt.Errorf("%w: missing jce.profile.list", errpkg.ErrForbidden)
	}

	filterUserID := userID
	if all && s.permissionChecker(ctx, userID)("comparison.edit_any") {
		filterUserID = pgtype.UUID{}
	}

	stats, err := s.profiles.countBySourceSpace(ctx, filterUserID)
	if err != nil {
		return nil, fmt.Errorf("counting profiles by source space: %w", err)
	}

	return stats, nil
}

// ListScoreFactors lists score factors for a comparison type, optionally
// including global defaults. A nil/invalid comparisonTypeID means global.
func (s *Service) ListScoreFactors(ctx context.Context, comparisonTypeID pgtype.UUID, includeGlobal bool) ([]ScoreFactor, error) {
	if !s.featureEnabled(ctx, "feature.jce.scoring") {
		return nil, fmt.Errorf("%w: feature.jce.scoring disabled", errpkg.ErrFeatureDisabled)
	}

	userID := s.userIDFromContext(ctx)
	if !s.permissionChecker(ctx, userID)("jce.score.view") {
		return nil, fmt.Errorf("%w: missing jce.score.view", errpkg.ErrForbidden)
	}

	if !comparisonTypeID.Valid {
		includeGlobal = true
	}

	factors, err := s.factors.listByType(ctx, comparisonTypeID, includeGlobal)
	if err != nil {
		return nil, fmt.Errorf("listing score factors: %w", err)
	}

	return factors, nil
}

// CreateScoreFactor creates a new score factor for the tenant.
func (s *Service) CreateScoreFactor(ctx context.Context, userID pgtype.UUID, in ScoreFactorInput) (ScoreFactor, error) {
	_ = userID

	if !s.featureEnabled(ctx, "feature.jce.scoring") {
		return ScoreFactor{}, fmt.Errorf("%w: feature.jce.scoring disabled", errpkg.ErrFeatureDisabled)
	}

	if !s.permissionChecker(ctx, userID)("jce.factor.manage") {
		return ScoreFactor{}, fmt.Errorf("%w: missing jce.factor.manage", errpkg.ErrForbidden)
	}

	// Only admins may create system score factors.
	if in.IsSystem && !s.permissionChecker(ctx, userID)("comparison.edit_any") {
		return ScoreFactor{}, fmt.Errorf("%w: only admins can create system score factors", errpkg.ErrForbidden)
	}

	if err := s.validateScoreFactor(in); err != nil {
		return ScoreFactor{}, err
	}

	// Ensure factor_key is unique for ( comparison_type_id).
	typeID := stringToUUID(ptrString(in.ComparisonTypeID))

	_, err := s.factors.getByKey(ctx, typeID, in.FactorKey)
	if err == nil {
		return ScoreFactor{}, fmt.Errorf("%w: factor key %q already exists for this type", errpkg.ErrDuplicate, in.FactorKey)
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return ScoreFactor{}, fmt.Errorf("checking factor key uniqueness: %w", err)
	}

	f, err := s.factors.create(ctx, in)
	if err != nil {
		return ScoreFactor{}, fmt.Errorf("creating score factor: %w", err)
	}

	return f, nil
}

// validateScoreFactor validates the score factor input.
func (s *Service) validateScoreFactor(in ScoreFactorInput) error {
	if in.FactorKey == "" {
		return fmt.Errorf("%w: factor_key is required", errpkg.ErrInvalid)
	}

	if !validDataSources[in.DataSource] {
		return fmt.Errorf("%w: invalid data_source %q", errpkg.ErrInvalid, in.DataSource)
	}

	if in.DefaultWeight < 0 || in.DefaultWeight > 1 {
		return fmt.Errorf("%w: default_weight must be between 0 and 1", errpkg.ErrInvalid)
	}

	return nil
}

// GetScoreFactor returns a score factor by id and tenant.
func (s *Service) GetScoreFactor(ctx context.Context, id pgtype.UUID) (ScoreFactor, error) {
	if !s.featureEnabled(ctx, "feature.jce.scoring") {
		return ScoreFactor{}, fmt.Errorf("%w: feature.jce.scoring disabled", errpkg.ErrFeatureDisabled)
	}

	userID := s.userIDFromContext(ctx)
	if !s.permissionChecker(ctx, userID)("jce.factor.manage") {
		return ScoreFactor{}, fmt.Errorf("%w: missing jce.factor.manage", errpkg.ErrForbidden)
	}

	f, err := s.factors.get(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ScoreFactor{}, fmt.Errorf("%w: score factor not found", errpkg.ErrNotFound)
		}

		return ScoreFactor{}, fmt.Errorf("getting score factor: %w", err)
	}

	return f, nil
}

// UpdateScoreFactor updates a score factor.
func (s *Service) UpdateScoreFactor(ctx context.Context, id, userID pgtype.UUID, in ScoreFactorInput) (ScoreFactor, error) {
	_ = userID

	if !s.featureEnabled(ctx, "feature.jce.scoring") {
		return ScoreFactor{}, fmt.Errorf("%w: feature.jce.scoring disabled", errpkg.ErrFeatureDisabled)
	}

	if !s.permissionChecker(ctx, userID)("jce.factor.manage") {
		return ScoreFactor{}, fmt.Errorf("%w: missing jce.factor.manage", errpkg.ErrForbidden)
	}

	existing, err := s.factors.get(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ScoreFactor{}, fmt.Errorf("%w: score factor not found", errpkg.ErrNotFound)
		}

		return ScoreFactor{}, fmt.Errorf("getting score factor: %w", err)
	}

	if existing.IsSystem && !s.permissionChecker(ctx, userID)("comparison.edit_any") {
		return ScoreFactor{}, fmt.Errorf("%w: system factors can only be updated by admins", errpkg.ErrForbidden)
	}

	if err := s.validateScoreFactor(in); err != nil {
		return ScoreFactor{}, err
	}

	// Preserve the is_system flag and comparison type on normal updates.
	in.IsSystem = existing.IsSystem
	if in.ComparisonTypeID == nil {
		in.ComparisonTypeID = existing.ComparisonTypeID
	}

	// If the key or type changed, ensure the new (tenant, type, key) is free.
	newTypeID := stringToUUID(ptrString(in.ComparisonTypeID))
	existingTypeID := stringToUUID(ptrString(existing.ComparisonTypeID))

	if in.FactorKey != existing.FactorKey || newTypeID != existingTypeID {
		dup, err := s.factors.getByKey(ctx, newTypeID, in.FactorKey)
		if err == nil && dup.ID != existing.ID {
			return ScoreFactor{}, fmt.Errorf("%w: factor key %q already exists for this type", errpkg.ErrDuplicate, in.FactorKey)
		}

		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return ScoreFactor{}, fmt.Errorf("checking factor key uniqueness: %w", err)
		}
	}

	f, err := s.factors.update(ctx, id, in)
	if err != nil {
		return ScoreFactor{}, fmt.Errorf("updating score factor: %w", err)
	}

	return f, nil
}

// DeleteScoreFactor deletes a score factor. System factors cannot be deleted.
func (s *Service) DeleteScoreFactor(ctx context.Context, id, userID pgtype.UUID) error {
	_ = userID

	if !s.featureEnabled(ctx, "feature.jce.scoring") {
		return fmt.Errorf("%w: feature.jce.scoring disabled", errpkg.ErrFeatureDisabled)
	}

	if !s.permissionChecker(ctx, userID)("jce.factor.manage") {
		return fmt.Errorf("%w: missing jce.factor.manage", errpkg.ErrForbidden)
	}

	existing, err := s.factors.get(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: score factor not found", errpkg.ErrNotFound)
		}

		return fmt.Errorf("getting score factor: %w", err)
	}

	if existing.IsSystem {
		return fmt.Errorf("%w: cannot delete system score factors", errpkg.ErrForbidden)
	}

	if err := s.factors.delete(ctx, id); err != nil {
		return fmt.Errorf("deleting score factor: %w", err)
	}

	return nil
}

// ScoreComparison recalculates the JapaScore for a comparison.
func (s *Service) ScoreComparison(ctx context.Context, contentID, userID pgtype.UUID, profileID *pgtype.UUID, smartMode string, force bool) (ScoreResult, error) {
	if !s.featureEnabled(ctx, "feature.jce.scoring") {
		return ScoreResult{}, fmt.Errorf("%w: feature.jce.scoring disabled", errpkg.ErrFeatureDisabled)
	}

	if !userID.Valid {
		return ScoreResult{}, fmt.Errorf("%w: authentication required", errpkg.ErrForbidden)
	}

	if s.engine == nil {
		return ScoreResult{}, fmt.Errorf("%w: scoring engine not configured", errpkg.ErrNotImplemented)
	}

	uid := userID.String()
	cid := contentID.String()

	var pid *string

	if profileID != nil && profileID.Valid {
		p := profileID.String()
		pid = &p
	}

	result, err := s.engine.Score(ctx, ScoreInput{
		ContentID: cid,

		UserID:            &uid,
		ProfileID:         pid,
		SmartMode:         smartMode,
		Force:             force,
		PermissionChecker: s.permissionChecker(ctx, userID),
	})
	if err != nil {
		return ScoreResult{}, err
	}

	s.deleteAdvisorCache(ctx, contentID)

	return result, nil
}

// GetComparisonScore returns a previously calculated score for a comparison.
func (s *Service) GetComparisonScore(ctx context.Context, contentID, userID pgtype.UUID, profileID *pgtype.UUID) (ScoreResult, error) {
	if !s.featureEnabled(ctx, "feature.jce.scoring") {
		return ScoreResult{}, fmt.Errorf("%w: feature.jce.scoring disabled", errpkg.ErrFeatureDisabled)
	}

	if !userID.Valid {
		return ScoreResult{}, fmt.Errorf("%w: authentication required", errpkg.ErrForbidden)
	}

	if s.engine == nil {
		return ScoreResult{}, fmt.Errorf("%w: scoring engine not configured", errpkg.ErrNotImplemented)
	}

	uid := userID.String()
	cid := contentID.String()

	var pid *string

	if profileID != nil && profileID.Valid {
		p := profileID.String()
		pid = &p
	}

	return s.engine.GetScore(ctx, GetScoreInput{
		ContentID: cid,

		UserID:            &uid,
		ProfileID:         pid,
		PermissionChecker: s.permissionChecker(ctx, userID),
	})
}

// PreviewScoreForItems scores a list of item refs without creating content.
func (s *Service) PreviewScoreForItems(ctx context.Context, userID pgtype.UUID, in ScorePreviewInput) (ScoreResult, error) {
	if !s.featureEnabled(ctx, "feature.jce.smart_compare") {
		return ScoreResult{}, fmt.Errorf("%w: feature.jce.smart_compare disabled", errpkg.ErrFeatureDisabled)
	}

	if !s.featureEnabled(ctx, "feature.jce.scoring") {
		return ScoreResult{}, fmt.Errorf("%w: feature.jce.scoring disabled", errpkg.ErrFeatureDisabled)
	}

	if !userID.Valid {
		return ScoreResult{}, fmt.Errorf("%w: authentication required", errpkg.ErrForbidden)
	}

	if !s.permissionChecker(ctx, userID)("jce.score.view") {
		return ScoreResult{}, fmt.Errorf("%w: missing jce.score.view", errpkg.ErrForbidden)
	}

	if s.engine == nil {
		return ScoreResult{}, fmt.Errorf("%w: scoring engine not configured", errpkg.ErrNotImplemented)
	}

	uid := userID.String()

	in.UserID = &uid
	in.PermissionChecker = s.permissionChecker(ctx, userID)

	if in.SmartMode == "" {
		in.SmartMode = comparison.SmartModeNone
	}

	return s.engine.PreviewScoreForItems(ctx, in)
}

// ListRecommendations returns recommendations for a comparison, falling back
// to a score run when none are cached.
func (s *Service) ListRecommendations(ctx context.Context, contentID, userID pgtype.UUID, profileID *pgtype.UUID, limit int) ([]ComparisonRecommendation, error) {
	if !s.featureEnabled(ctx, "feature.jce.scoring") {
		return nil, fmt.Errorf("%w: feature.jce.scoring disabled", errpkg.ErrFeatureDisabled)
	}

	if !s.permissionChecker(ctx, userID)("jce.recommendation.view") {
		return nil, fmt.Errorf("%w: missing jce.recommendation.view", errpkg.ErrForbidden)
	}

	if !userID.Valid {
		return nil, fmt.Errorf("%w: authentication required", errpkg.ErrForbidden)
	}

	if s.engine == nil {
		return nil, fmt.Errorf("%w: scoring engine not configured", errpkg.ErrNotImplemented)
	}

	cmp, err := s.compSvc.GetJCEComparison(ctx, contentID.String())
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: comparison not found", errpkg.ErrNotFound)
		}

		return nil, fmt.Errorf("loading comparison: %w", err)
	}

	check := s.permissionChecker(ctx, userID)
	if !s.canAccessComparison(cmp, userID, check, "jce.recommendation.view") {
		return nil, fmt.Errorf("%w: cannot view comparison recommendations", errpkg.ErrForbidden)
	}

	var pid pgtype.UUID
	if profileID != nil {
		pid = *profileID
	}

	recs, err := s.recs.listByContent(ctx, contentID, pid, safeInt32(limit))
	if err != nil {
		return nil, fmt.Errorf("listing recommendations: %w", err)
	}

	if len(recs) > 0 {
		return recs, nil
	}

	uid := userID.String()
	cid := contentID.String()

	var pidStr *string

	if profileID != nil && profileID.Valid {
		p := profileID.String()
		pidStr = &p
	}

	result, err := s.engine.Score(ctx, ScoreInput{
		ContentID: cid,

		UserID:            &uid,
		ProfileID:         pidStr,
		PublicPerm:        "jce.recommendation.view",
		PermissionChecker: check,
	})
	if err != nil {
		return nil, fmt.Errorf("scoring for recommendations: %w", err)
	}

	recs = result.Recommendations
	if limit > 0 && len(recs) > limit {
		recs = recs[:limit]
	}

	return recs, nil
}
