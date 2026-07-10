// Package errors provides shared error types for the JapaConnect backend.
//
// Sentinel errors defined here allow handlers to use errors.Is() for
// error matching instead of fragile string containment checks.
package errors

import "errors"

// Domain errors — returned by service layers to indicate specific failure modes.
var (
	// ErrDuplicate indicates a unique constraint violation (e.g. duplicate email, slug).
	ErrDuplicate = errors.New("duplicate")

	// ErrAlreadyExists indicates the resource already exists.
	ErrAlreadyExists = errors.New("already_exists")

	// ErrNotFound indicates the requested resource was not found.
	ErrNotFound = errors.New("not_found")

	// ErrRequired indicates a required field is missing.
	ErrRequired = errors.New("required")

	// ErrInvalid indicates invalid input or format.
	ErrInvalid = errors.New("invalid")

	// ErrExpired indicates a token, code, or session has expired.
	ErrExpired = errors.New("expired")

	// ErrForbidden indicates the user lacks permission for the operation.
	ErrForbidden = errors.New("forbidden")

	// ErrBelongsToAnother indicates the resource belongs to another user/entity.
	ErrBelongsToAnother = errors.New("does not belong")

	// ErrMaxDepth indicates a maximum depth limit was reached (e.g. comment nesting).
	ErrMaxDepth = errors.New("maximum depth reached")

	// ErrInvalidStatusTransition indicates an invalid state transition.
	ErrInvalidStatusTransition = errors.New("invalid status transition")
)

// WrapDuplicate wraps an error with the duplicate sentinel if it appears to be
// a duplicate constraint violation. This is a transitional helper while services
// migrate to returning ErrDuplicate directly.
func WrapDuplicate(err error) error {
	if err == nil {
		return nil
	}
	// Check if already wrapped
	if errors.Is(err, ErrDuplicate) {
		return err
	}
	// Wrap with duplicate sentinel for backward compatibility
	return errors.Join(ErrDuplicate, err)
}
