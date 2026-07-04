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
	"github.com/jedafe/japa/internal/content"
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

// Service provides comparison operations with reader/writer pool separation.
type Service struct {
	store      *Store
	db         *db.DB
	contentSvc *content.Service
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
	}
}
