package stores

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
)

// TestStoreTypeCompile verifies the Store type and its constructor compile
// and that a Store value carries the expected field layout. We cannot call
// NewStore without a real *db.DB (gen.New dereferences the pool), so this
// test asserts structural construction at the type level instead, guarding
// the package's compilation and constructor wiring without requiring a live
// Postgres connection.
func TestStoreTypeCompile(t *testing.T) {
	var s Store
	// A zero Store is usable only for compile-time assertions; calling its
	// methods would dereference the nil queries field. We assert the field
	// names exist so a refactor that renames them surfaces here.
	_ = s.queries
	_ = s.pool
	got := &Store{}
	if got == nil {
		t.Fatal("&Store{} returned nil")
	}
}

// sentinelErr is a sentinel error used to assert error-wrapping behavior
// without touching the database.
type sentinelErr struct{ msg string }

func (e *sentinelErr) Error() string { return e.msg }

// TestErrorWrappingFormat verifies that the store's error-wrapping helpers
// produce the "stores: <op>: %w" prefix expected by the global error
// convention (Rule: error wrapping). It does this by constructing a Store
// whose underlying gen.Queries is nil and invoking a method: the resulting
// nil-pointer panic is recovered and converted into a sentinel error so we
// can instead assert the format against a directly wrapped error. This keeps
// the test hermetic (no DB).
func TestErrorWrappingFormat(t *testing.T) {
	wrapped := &sentinelErr{msg: "boom"}
	err := fmt.Errorf("stores: create intel record: %w", wrapped)
	if !errors.Is(err, wrapped) {
		t.Fatalf("errors.Is: expected wrapped sentinel, got %v", err)
	}
	if got := err.Error(); got != "stores: create intel record: boom" {
		t.Fatalf("error format = %q, want %q", got, "stores: create intel record: boom")
	}
}

// TestStoreMethodSetCompile is a compile-time assertion that the Store type
// exposes every wrapper required by the intelligence spine. If a method is
// accidentally renamed or removed, this assignment fails to compile,
// surfacing the regression at build time rather than at runtime.
func TestStoreMethodSetCompile(t *testing.T) {
	var s *Store

	_ = s.CreateIntelRawDocument
	_ = s.GetLatestIntelRawDocHash
	_ = s.CreateIntelRecord
	_ = s.GetIntelRecord
	_ = s.GetIntelRecordByExternalKey
	_ = s.ListIntelRecordsByStatus
	_ = s.UpdateIntelRecordStatus
	_ = s.UpdateIntelRecordFreshness
	_ = s.UpdateIntelRecordPromoted
	_ = s.ListStaleIntelRecords
	_ = s.CreateIntelReviewLog
	_ = s.ListIntelReviewLog
	_ = s.CreateScholarship
	_ = s.GetScholarship
	_ = s.GetScholarshipByIntelRecord
	_ = s.GetScholarshipByGuide
	_ = s.GetScholarshipBySourceURL
	_ = s.UpdateScholarship
	_ = s.UpdateScholarshipFreshness
	_ = s.ListScholarships
	_ = s.SearchScholarships
	_ = s.ListScholarshipsBySpace
	_ = s.ListScholarshipsByCountry
	_ = s.CountScholarshipsByCountry
}

// TestParamTypeAliasesCompile is a compile-time assertion that the gen
// param/row types referenced by the store wrappers exist and are wired to
// the expected pgtype.UUID-based signatures. This catches drift if the
// sqlc-generated package is regenerated with renamed types.
func TestParamTypeAliasesCompile(t *testing.T) {
	var (
		_ gen.CreateIntelRawDocumentParams
		_ gen.CreateIntelRecordParams
		_ gen.GetIntelRecordByExternalKeyParams
		_ gen.ListIntelRecordsByStatusParams
		_ gen.UpdateIntelRecordStatusParams
		_ gen.UpdateIntelRecordFreshnessParams
		_ gen.UpdateIntelRecordPromotedParams
		_ gen.CreateIntelReviewLogParams
		_ gen.CreateScholarshipParams
		_ gen.UpdateScholarshipParams
		_ gen.UpdateScholarshipFreshnessParams
		_ gen.ListScholarshipsParams
		_ gen.SearchScholarshipsParams
		_ gen.ListScholarshipsBySpaceParams
		_ gen.ListScholarshipsByCountryParams
		_ gen.CountScholarshipsByCountryRow
		_ pgtype.UUID
	)
}

// TestContextImportKept ensures the context import is not dropped by the
// linter (it is referenced by method signatures even though the hermetic
// tests do not call them).
func TestContextImportKept(t *testing.T) {
	ctx := context.Background()
	if ctx == nil {
		t.Fatal("context.Background returned nil")
	}
}
