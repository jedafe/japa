// Package feed implements the Activity Feed / Timeline system (Section 32).
// Four timelines: explore (guests), primary (logged-in), space, profile.
// Four sort modes: latest, top, rising, relevant.
// Hybrid fan-out: push for spaces ≤ 5000 members, pull for larger.
package feed

import (
	"context"

	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Sort mode constants for GetFeed.
const (
	SortLatest   = "latest"
	SortTop      = "top"
	SortRising   = "rising"
	SortRelevant = "relevant"
)

// FanOutThreshold is the member count at which fan-out switches from
// push (write feeds rows) to pull (followers pull on next request).
const FanOutThreshold = 5000

// DefaultPageSize is the default number of feed items returned per page.
const DefaultPageSize = 20

// MaxPageSize is the maximum number of feed items a client can request.
const MaxPageSize = 100

// Service provides feed operations with reader/writer pool separation.
type Service struct {
	store    *Store
	db       *db.DB
	enqueuer feedEnqueuer
	vk       valkeycompat.Cmdable
	settings feedSettingsReader
	checker  FeatureChecker
}

// feedSettingsReader is the subset of config.Settings the feed service uses
// for hot-reloadable operational limits. Defined as a local interface so
// the feed package does not import config.
type feedSettingsReader interface {
	GetIntOr(ctx context.Context, key string, fallback int) int
	GetFloatOr(ctx context.Context, key string, fallback float64) float64
	GetBoolOr(ctx context.Context, key string, fallback bool) bool
}

// FeatureChecker resolves feature flag state from the feature_flags table —
// the single source of truth for feature reachability (e.g.
// geo_personalization.enabled). Defined locally to keep the dependency
// graph clean; *features.Checker satisfies it structurally.
type FeatureChecker interface {
	IsEnabled(ctx context.Context, key string) bool
}

// feedEnqueuer is the subset of worker.Enqueuer that feed needs.
type feedEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// NewService creates a Service backed by the given database pool and Valkey
// client. The Valkey client caches the ranked For-You feed (may be nil, in
// which case ranking runs on every request).
func NewService(database *db.DB, vk valkeycompat.Cmdable) *Service {
	return &Service{
		store: &Store{
			read:  gen.New(database.Reader),
			write: gen.New(database.Writer),
		},
		db: database,
		vk: vk,
	}
}

// SetEnqueuer injects the Asynq enqueuer for async fan-out tasks.
func (s *Service) SetEnqueuer(e feedEnqueuer) {
	s.enqueuer = e
}

// SetSettings injects the hot-reloadable settings reader for feed limits.
func (s *Service) SetSettings(sr feedSettingsReader) {
	s.settings = sr
}

// SetFeatureChecker injects the feature flag checker so geo-personalization
// reachability reads the feature_flags table (the single source of truth)
// instead of system_settings. Falls back to system_settings when nil.
func (s *Service) SetFeatureChecker(c FeatureChecker) {
	s.checker = c
}

// geoPersonalizationEnabled resolves the geo_personalization.enabled feature
// flag. When disabled, feed ranking falls back to global order with no geo
// boost (BUG-007). system_settings is consulted only when no checker is
// injected.
func (s *Service) geoPersonalizationEnabled(ctx context.Context) bool {
	if s.checker != nil {
		return s.checker.IsEnabled(ctx, "geo_personalization.enabled")
	}

	if s.settings == nil {
		return true
	}

	return s.settings.GetBoolOr(ctx, "geo_personalization.enabled", true)
}
