// Package comparison implements the Generic Comparison System for JapaConnect
// (Section 30).
//
// Two-layer architecture: admin-defined Comparison Types with their own custom
// field definitions, and content-linked comparison items, dimensions, and values.
// Comparisons are registered as a content_type='comparison' under the Section 14
// content type system. The comparison_type_definitions table allows admins to
// define named Comparison Types (e.g. "University Comparison", "Bank Comparison")
// with associated custom field groups and per-UserGroup access control.
//
// The Service struct wraps two sqlc-generated Queries instances: one reading
// from the reader pool and one writing to the writer pool.
package comparison

import (
	"context"
	"math"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/comparison/resolver"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/customfields"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Dimension types supported by the comparison system.
const (
	DimensionTypeText     = "text"
	DimensionTypeRating   = "rating"
	DimensionTypeBoolean  = "boolean"
	DimensionTypeNumber   = "number"
	DimensionTypeCurrency = "currency"
)

// Pagination defaults.
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// validDimensionTypes is the set of all supported dimension types.
var validDimensionTypes = map[string]bool{
	DimensionTypeText:     true,
	DimensionTypeRating:   true,
	DimensionTypeBoolean:  true,
	DimensionTypeNumber:   true,
	DimensionTypeCurrency: true,
}

// IsValidDimensionType returns true if the given type is a supported dimension type.
func IsValidDimensionType(dt string) bool {
	return validDimensionTypes[dt]
}

// toInt32 converts a non-negative int to int32, clamping it to the
// maximum int32 value. Callers use it for loop indices and slice lengths
// so gosec G115 does not flag the conversion.
func toInt32(n int) int32 {
	if n > math.MaxInt32 {
		return math.MaxInt32
	}

	if n < math.MinInt32 {
		return math.MinInt32
	}

	return int32(n) // #nosec G115 -- value is clamped to the int32 range above
}

// DefaultComparisonTypes are the pre-seeded comparison types for the platform.
var DefaultComparisonTypes = []struct {
	Name        string
	Slug        string
	Description string
	ItemLabel   string
}{
	{Name: "Universities", Slug: "universities", Description: "Compare universities and study programs", ItemLabel: "University"},
	{Name: "Visa Types", Slug: "visa-types", Description: "Compare visa categories and requirements", ItemLabel: "Visa"},
	{Name: "Banks", Slug: "banks", Description: "Compare banking services and fees", ItemLabel: "Bank"},
	{Name: "Courses", Slug: "courses", Description: "Compare courses and training programs", ItemLabel: "Course"},
}

// TierResolver resolves a user's effective unified tier (regular/pro/business)
// for per-tier quota lookups. Implemented by the tiers package.
type TierResolver interface {
	ResolveUserTier(ctx context.Context, userID pgtype.UUID) (string, error)
}

// Service provides comparison operations with reader/writer pool separation.
type Service struct {
	store        *Store
	db           *db.DB
	contentSvc   *content.Service
	cfSvc        *customfields.Service
	settingsSvc  *config.Settings
	resolver     *resolver.Registry
	vk           valkeycompat.Cmdable
	tierResolver TierResolver
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{
		store: &Store{
			read:  gen.New(database.Reader),
			write: gen.New(database.Writer),
		},
		db:         database,
		contentSvc: content.NewService(database),
		cfSvc:      customfields.NewService(database),
	}
}

// SetSettings injects the hot-reloadable system settings service.
func (s *Service) SetSettings(ss *config.Settings) {
	s.settingsSvc = ss
}

// SetContentService injects the shared content service so comparison and JCE
// operations use the same searcher/pool as the rest of the backend.
func (s *Service) SetContentService(cs *content.Service) {
	s.contentSvc = cs
}

// SetResolver injects the item_ref resolver registry.
func (s *Service) SetResolver(r *resolver.Registry) {
	s.resolver = r
}

// SetValkey injects the Valkey client for selection-token and cache operations.
func (s *Service) SetValkey(vk valkeycompat.Cmdable) {
	s.vk = vk
}

// SetTierResolver injects the unified-tier resolver used for per-tier quota
// lookups (e.g. jce.comparisons.max_per_day.{tier}).
func (s *Service) SetTierResolver(tr TierResolver) {
	s.tierResolver = tr
}
