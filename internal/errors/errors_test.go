package errors

import (
	stderrors "errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSentinelsAreDistinct(t *testing.T) {
	t.Parallel()

	sentinels := []error{
		ErrDuplicate, ErrAlreadyExists, ErrNotFound, ErrRequired, ErrInvalid,
		ErrExpired, ErrForbidden, ErrBelongsToAnother, ErrMaxDepth,
		ErrInvalidStatusTransition,
	}

	for i, a := range sentinels {
		for j, b := range sentinels {
			if i != j && stderrors.Is(a, b) {
				t.Errorf("sentinels %d (%v) and %d (%v) match each other", i, a, j, b)
			}
		}
	}
}

func TestWrapDuplicateNil(t *testing.T) {
	t.Parallel()

	if err := WrapDuplicate(nil); err != nil {
		t.Errorf("WrapDuplicate(nil) = %v, want nil", err)
	}
}

func TestWrapDuplicateWrapsPlainError(t *testing.T) {
	t.Parallel()

	cause := stderrors.New("unique violation on users.email")

	err := WrapDuplicate(cause)
	if !stderrors.Is(err, ErrDuplicate) {
		t.Error("wrapped error does not match ErrDuplicate")
	}

	if !stderrors.Is(err, cause) {
		t.Error("wrapped error lost the original cause")
	}
}

func TestWrapDuplicateIsIdempotent(t *testing.T) {
	t.Parallel()

	once := WrapDuplicate(stderrors.New("boom"))

	twice := WrapDuplicate(once)
	if twice != once { //nolint:errorlint // identity check: already-wrapped errors are returned as-is
		t.Errorf("WrapDuplicate re-wrapped an already-duplicate error: %v", twice)
	}
}

func TestWrapDuplicatePreservesExistingDuplicateChain(t *testing.T) {
	t.Parallel()

	cause := fmt.Errorf("insert failed: %w", ErrDuplicate)

	err := WrapDuplicate(cause)
	if err.Error() != cause.Error() {
		t.Errorf("WrapDuplicate(%v) = %v, want the input unchanged", cause, err)
	}
}

func TestTrackingNoopsWithoutDSN(t *testing.T) {
	t.Parallel()

	// With no DSN configured every tracking call must be a safe no-op.
	InitGlitchTip("", "test-release")
	CaptureException(nil)
	CaptureException(stderrors.New("boom"))
	CaptureMessage("hello")
	Flush(0)
}

func TestStripSentinelPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		sentinel error
		want     string
	}{
		{"wrapped with colon", fmt.Errorf("%w: points to convert must be positive", ErrInvalid), ErrInvalid, "points to convert must be positive"},
		{"wrapped no colon", fmt.Errorf("%w details here", ErrForbidden), ErrForbidden, "details here"},
		{"bare sentinel", ErrNotFound, ErrNotFound, "not_found"},
		{"no match", stderrors.New("some other error"), ErrInvalid, "some other error"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := stripSentinelPrefix(tc.err, tc.sentinel)
			if got != tc.want {
				t.Errorf("stripSentinelPrefix(%v, %v) = %q, want %q", tc.err, tc.sentinel, got, tc.want)
			}
		})
	}
}

func TestHandleServiceErrorMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"ErrNotFound", ErrNotFound, http.StatusNotFound, "not_found"},
		{"ErrDuplicate", ErrDuplicate, http.StatusConflict, "already_exists"},
		{"ErrAlreadyExists", ErrAlreadyExists, http.StatusConflict, "already_exists"},
		{"ErrRequired", ErrRequired, http.StatusBadRequest, "validation_error"},
		{"ErrInvalid", ErrInvalid, http.StatusBadRequest, "validation_error"},
		{"ErrForbidden", ErrForbidden, http.StatusForbidden, "forbidden"},
		{"ErrBelongsToAnother", ErrBelongsToAnother, http.StatusForbidden, "forbidden"},
		{"ErrExpired", ErrExpired, http.StatusUnauthorized, "expired"},
		{"ErrMaxDepth", ErrMaxDepth, http.StatusBadRequest, "depth_exceeded"},
		{"ErrInvalidStatusTransition", ErrInvalidStatusTransition, http.StatusConflict, "invalid_status_transition"},
		{"ErrStatusDerived", ErrStatusDerived, http.StatusConflict, "status_derived"},
		{"ErrInsufficientBalance", ErrInsufficientBalance, http.StatusUnprocessableEntity, "insufficient_balance"},
		{"unknown error", stderrors.New("boom"), http.StatusInternalServerError, "internal_error"},
		{"wrapped ErrInvalid", fmt.Errorf("%w: bad input", ErrInvalid), http.StatusBadRequest, "validation_error"},
		{"wrapped ErrForbidden", fmt.Errorf("%w: insufficient permissions", ErrForbidden), http.StatusForbidden, "forbidden"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rr := httptest.NewRecorder()
			HandleServiceError(rr, tc.err)

			if rr.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rr.Code, tc.wantStatus)
			}

			body := rr.Body.String()
			if !strings.Contains(body, tc.wantCode) {
				t.Errorf("body %q does not contain code %q", body, tc.wantCode)
			}
		})
	}
}

func TestHandleServiceErrorStripsSentinelPrefix(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	HandleServiceError(rr, fmt.Errorf("%w: points to convert must be positive", ErrInvalid))

	body := rr.Body.String()
	if strings.Contains(body, "invalid: points") {
		t.Errorf("sentinel prefix leaked into response: %s", body)
	}

	if !strings.Contains(body, "points to convert must be positive") {
		t.Errorf("detail message lost from response: %s", body)
	}
}
