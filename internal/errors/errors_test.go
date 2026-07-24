package errors

import (
	stderrors "errors"
	"fmt"
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
