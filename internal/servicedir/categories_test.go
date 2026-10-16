// categories_test.go — unit coverage for the dynamic service_categories
// taxonomy paths that don't need a database: with an unwired store the
// hardcoded §98 enum is the fallback for both validation (categorySet)
// and the options payload (Categories). Admin CRUD input validation runs
// before any store access, so it is exercised on a store-less Service.
//
// Ref: docs/superpowers/specs/2026-10-07-services-marketplace-upgrade-design.md §2.1
package servicedir

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestCategorySetFallsBackToStaticEnum(t *testing.T) {
	// nil store → the hardcoded §98 enum is the fallback
	s := &Service{}

	set := s.categorySet(t.Context())
	for _, slug := range []string{"immigration_legal", "relocation", "finance", "education", "health", "community", "other"} {
		if _, ok := set[slug]; !ok {
			t.Fatalf("expected %q in fallback category set", slug)
		}
	}
}

func TestCategoriesFallsBackToStaticEnum(t *testing.T) {
	// nil store → Categories serves the static enum, sorted by value.
	s := &Service{}

	opts := s.Categories(t.Context())
	if len(opts) != len(categoryLabels) {
		t.Fatalf("expected %d fallback options, got %d", len(categoryLabels), len(opts))
	}

	for _, slug := range []string{"immigration_legal", "relocation", "finance", "education", "health", "community", "other"} {
		found := false

		for _, o := range opts {
			if o.Value == slug {
				found = true

				if o.Label != categoryLabels[slug] {
					t.Fatalf("expected label %q for %q, got %q", categoryLabels[slug], slug, o.Label)
				}

				break
			}
		}

		if !found {
			t.Fatalf("expected %q in fallback Categories options", slug)
		}
	}
}

// ---------------------------------------------------------------------------
// Admin CRUD input validation (no DB — validation precedes store access)
// ---------------------------------------------------------------------------

func TestValidateCategoryInput(t *testing.T) {
	tests := []struct {
		name     string
		in       CategoryInput
		creating bool
		wantErr  bool
	}{
		{"valid_minimal", CategoryInput{Slug: "legal_help", Label: "Legal Help"}, true, false},
		{"valid_full", CategoryInput{Slug: "x9", Label: "X", AppliesTo: "package", AccentColor: "cyan"}, true, false},
		{"slug_single_char", CategoryInput{Slug: "ab", Label: "A"}, true, false},
		{"slug_too_short", CategoryInput{Slug: "a", Label: "A"}, true, true},
		{"slug_uppercase", CategoryInput{Slug: "Legal", Label: "A"}, true, true},
		{"slug_leading_underscore", CategoryInput{Slug: "_legal", Label: "A"}, true, true},
		{"slug_hyphen", CategoryInput{Slug: "legal-help", Label: "A"}, true, true},
		{"slug_too_long", CategoryInput{Slug: "a" + strings.Repeat("b", 49), Label: "A"}, true, true},
		{"slug_empty_create", CategoryInput{Slug: "", Label: "A"}, true, true},
		{"label_empty", CategoryInput{Slug: "legal", Label: "   "}, true, true},
		{"label_too_long", CategoryInput{Slug: "legal", Label: strings.Repeat("x", 81)}, true, true},
		{"label_max", CategoryInput{Slug: "legal", Label: strings.Repeat("x", 80)}, true, false},
		{"applies_to_bad", CategoryInput{Slug: "legal", Label: "A", AppliesTo: "all"}, true, true},
		{"applies_to_empty_ok", CategoryInput{Slug: "legal", Label: "A", AppliesTo: ""}, true, false},
		{"accent_bad", CategoryInput{Slug: "legal", Label: "A", AccentColor: "#ff00ff"}, true, true},
		{"accent_empty_ok", CategoryInput{Slug: "legal", Label: "A"}, true, false},
		// Update mode: slug format not re-checked (immutable), label still required.
		{"update_slug_unchecked", CategoryInput{Slug: "Anything-Goes", Label: "A"}, false, false},
		{"update_label_required", CategoryInput{Slug: "legal", Label: ""}, false, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCategoryInput(tc.in, tc.creating)
			if tc.wantErr && err == nil {
				t.Fatalf("expected validation error, got nil")
			}

			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}

			if tc.wantErr {
				var inputErr *CategoryInputError
				if !errors.As(err, &inputErr) {
					t.Fatalf("expected *CategoryInputError, got %T", err)
				}
			}
		})
	}
}

func TestAdminCreateCategoryValidationRunsBeforeStore(t *testing.T) {
	// nil store → a validation failure must return before touching the DB.
	s := &Service{}
	_, err := s.AdminCreateCategory(t.Context(), CategoryInput{Slug: "BAD SLUG", Label: "x"})

	var inputErr *CategoryInputError

	if !errors.As(err, &inputErr) {
		t.Fatalf("expected *CategoryInputError, got %v", err)
	}
}

func TestAdminUpdateCategoryRejectsSlugChange(t *testing.T) {
	// Body slug disagreeing with the path slug is rejected before the DB.
	s := &Service{}
	_, err := s.AdminUpdateCategory(t.Context(), "legal", CategoryInput{Slug: "renamed", Label: "Legal"})

	var inputErr *CategoryInputError

	if !errors.As(err, &inputErr) {
		t.Fatalf("expected *CategoryInputError, got %v", err)
	}
}

func TestAdminUpdateCategoryValidationRunsBeforeStore(t *testing.T) {
	// Same slug in body and path is fine; a missing label still fails
	// before the store lookup.
	s := &Service{}
	_, err := s.AdminUpdateCategory(t.Context(), "legal", CategoryInput{Slug: "legal", Label: ""})

	var inputErr *CategoryInputError

	if !errors.As(err, &inputErr) {
		t.Fatalf("expected *CategoryInputError, got %v", err)
	}
}

func TestWriteCategoryErrorMapping(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"validation", &CategoryInputError{Msg: "label is required"}, http.StatusBadRequest},
		{"duplicate_slug", ErrCategoryExists, http.StatusConflict},
		{"not_found", pgx.ErrNoRows, http.StatusNotFound},
		{"wrapped_not_found", fmt.Errorf("servicedir: %w", errNotFound), http.StatusNotFound},
		{"unexpected", errors.New("boom"), http.StatusInternalServerError},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeCategoryError(rec, tc.err, "failed", "Failed")

			if rec.Code != tc.wantStatus {
				t.Fatalf("expected %d, got %d", tc.wantStatus, rec.Code)
			}
		})
	}
}
