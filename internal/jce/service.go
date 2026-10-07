package jce

import (
	"context"
	"log/slog"
	"math"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/comparison"
	"github.com/jedafe/japa/internal/comparison/resolver"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db"
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

// stringToUUID parses a UUID string into a pgtype.UUID; an empty or invalid string returns an invalid UUID.
func stringToUUID(s string) pgtype.UUID {
	var u pgtype.UUID
	if s == "" {
		return u
	}

	if err := u.Scan(s); err != nil {
		return u
	}

	return u
}

// taskEnqueuer is the subset of the worker enqueuer used by the JCE service.
// This avoids an import cycle with internal/worker.
type taskEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// aiDispatcher is the narrow interface the JCE service needs for ai_estimate
// fills. It is implemented by an adapter in the server package so the jce
// package does not import the full ai subsystem and create an import cycle.
// // Ref: docs/superpowers/specs/2026-08-30-jce-subspec-4-design.md §10.
type aiDispatcher interface {
	// Generate returns a plain text completion for the given category and prompt.
	// systemContext is an optional system prompt; model and maxTokens are passed
	// through to the underlying AI gateway when non-empty / positive; otherwise
	// the gateway's configured defaults are used.
	Generate(ctx context.Context, category, prompt, systemContext, model string, maxTokens int) (string, error)
}

// Service orchestrates JCE data sources, freshness reporting, and
// data-source-backed value refresh.
type Service struct {
	db          *db.DB
	compSvc     *comparison.Service
	r           *resolver.Registry
	settings    *config.Settings
	vk          valkeycompat.Cmdable
	features    features.FeatureChecker
	enqueuer    taskEnqueuer
	dispatcher  aiDispatcher
	dataSources *dataSourceStore
	ingestion   *ingestionStore
	sources     dataSourceRegistry
}

// NewService creates a JCE service backed by the given dependencies.
// // The service does not create its own feature checker; callers must inject
// one with SetFeatureChecker (usually the shared checker from the server).
func NewService(
	database *db.DB,
	compSvc *comparison.Service,
	r *resolver.Registry,
	settings *config.Settings,
	vk valkeycompat.Cmdable,
) *Service {
	return &Service{
		db:          database,
		compSvc:     compSvc,
		r:           r,
		settings:    settings,
		vk:          vk,
		dataSources: newDataSourceStore(database),
		ingestion:   newIngestionStore(database),
	}
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

// deleteJceCacheByPattern deletes all Valkey keys matching a pattern.
// It is used for platform-wide invalidations (e.g. data source changes).
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
