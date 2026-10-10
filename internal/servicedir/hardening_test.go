// hardening_test.go — unit coverage for the §111 booking-surface
// hardening round: uniform-404 /me/ booking object routes, the
// preferred_dates rejection on mentorship-kind create, the privileged
// free-text caps, and the composite reviews/categories feature gate.
// These paths run entirely on sentinels + feature flags — no database.
package servicedir

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// errCode extracts the machine-readable code from the standard
// {"error":{"code":...}} envelope.
func errCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()

	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}

	return body.Error.Code
}

// TestWriteMyBookingError_Uniform404 — a foreign-but-existing booking must
// be indistinguishable from a missing one on the user-facing
// /me/service-bookings object routes (ErrNotOwner and pgx.ErrNoRows both
// answer 404 booking_not_found). Other errors still delegate to
// writeServiceError.
func TestWriteMyBookingError_Uniform404(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode int
		wantBody string
	}{
		{"foreign booking", ErrNotOwner, http.StatusNotFound, "booking_not_found"},
		{"wrapped foreign booking", fmt.Errorf("respond: %w", ErrNotOwner), http.StatusNotFound, "booking_not_found"},
		{"missing booking", pgx.ErrNoRows, http.StatusNotFound, "booking_not_found"},
		{"sentinel passthrough", ErrInvalidBookingTransition, http.StatusConflict, "invalid_booking_transition"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeMyBookingError(rec, tc.err, "failed_code", "failed message")

			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tc.wantCode, rec.Body.String())
			}
			if got := errCode(t, rec); got != tc.wantBody {
				t.Fatalf("code = %q, want %q", got, tc.wantBody)
			}
		})
	}
}

// TestMentorshipBookingPreferredDatesRejected — a mentorship request has
// no date surface; preferred_dates is rejected outright with
// ErrInvalidInput (400 invalid_input) instead of being silently dropped.
func TestMentorshipBookingPreferredDatesRejected(t *testing.T) {
	svc := &Service{checker: fakeFlags{"mentorship.enabled": true}}

	var bookerID, providerID, contentID pgtype.UUID
	bookerID.Scan("11111111-1111-1111-1111-111111111111")
	providerID.Scan("22222222-2222-2222-2222-222222222222")
	contentID.Scan("33333333-3333-3333-3333-333333333333")

	listing := ServiceRow{AuthorID: providerID}
	input := CreateBookingInput{
		Goals:          "g",
		PreferredDates: &PreferredDatesInput{StartOn: "2026-01-01", EndOn: "2026-01-10"},
	}

	_, err := svc.createMentorshipBooking(t.Context(), bookerID, contentID, listing, input)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

// TestPrivilegedBookingTextCaps — the privileged free-text fields carry
// the same 4000-rune cap as the booker-facing inputs (400 input_too_long).
func TestPrivilegedBookingTextCaps(t *testing.T) {
	svc := &Service{checker: fakeFlags{"service_packages.enabled": true}}

	var orgID, bookingID, userID pgtype.UUID
	orgID.Scan("11111111-1111-1111-1111-111111111111")
	bookingID.Scan("22222222-2222-2222-2222-222222222222")
	userID.Scan("33333333-3333-3333-3333-333333333333")

	overlong := strings.Repeat("x", maxBookingTextLen+1)

	t.Run("org response note", func(t *testing.T) {
		_, err := svc.OrgTransition(t.Context(), orgID, bookingID, userID,
			OrgBookingActionInput{Status: BookingStatusConfirmed, OrgResponseNote: overlong})
		if !errors.Is(err, ErrInputTooLong) {
			t.Fatalf("expected ErrInputTooLong, got %v", err)
		}
	})

	t.Run("org cancel reason", func(t *testing.T) {
		_, err := svc.OrgTransition(t.Context(), orgID, bookingID, userID,
			OrgBookingActionInput{Status: BookingStatusCancelled, CancelReason: overlong})
		if !errors.Is(err, ErrInputTooLong) {
			t.Fatalf("expected ErrInputTooLong, got %v", err)
		}
	})

	t.Run("admin resolution note", func(t *testing.T) {
		_, err := svc.AdminSetBookingStatus(t.Context(), bookingID,
			AdminBookingActionInput{Status: BookingStatusCancelled, ResolutionNote: overlong})
		if !errors.Is(err, ErrInputTooLong) {
			t.Fatalf("expected ErrInputTooLong, got %v", err)
		}
	})
}

// TestCategoriesCompositeGate — /services/categories answers 404 when both
// flags are off; in a mentorship-only deployment the response is scoped to
// applies_to='mentorship' rows only.
func TestCategoriesCompositeGate(t *testing.T) {
	newReq := func() (*httptest.ResponseRecorder, *http.Request) {
		return httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/services/categories", nil)
	}

	t.Run("both flags off → 404", func(t *testing.T) {
		h := &Handlers{service: &Service{checker: fakeFlags{}}}
		rec, req := newReq()
		h.HandleCategories(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
		if got := errCode(t, rec); got != "service_directory_disabled" {
			t.Fatalf("code = %q, want service_directory_disabled", got)
		}
	})

	t.Run("mentorship-only → only applies_to=mentorship|both rows", func(t *testing.T) {
		// store=nil falls back to the hardcoded §98 enum, which carries no
		// applies_to — the mentorship-only scope must filter every row out
		// rather than leak service/package categories. 'both' stays visible
		// (it means "applies to every kind" per the applies_to convention).
		h := &Handlers{service: &Service{checker: fakeFlags{"mentorship.enabled": true}}}
		rec, req := newReq()
		h.HandleCategories(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}

		var body struct {
			Data []Option `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		for _, o := range body.Data {
			if o.AppliesTo != "mentorship" && o.AppliesTo != "both" {
				t.Fatalf("category %q has applies_to %q, want mentorship|both only", o.Value, o.AppliesTo)
			}
		}
	})
}

// TestReviewsCompositeGate — /services/{id}/reviews answers 404 when both
// flags are off (the composite gate replaces the directory-only one).
func TestReviewsCompositeGate(t *testing.T) {
	h := &Handlers{service: &Service{checker: fakeFlags{}}}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/services/11111111-1111-1111-1111-111111111111/reviews", nil)
	h.HandleListReviews(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if got := errCode(t, rec); got != "service_directory_disabled" {
		t.Fatalf("code = %q, want service_directory_disabled", got)
	}
}
