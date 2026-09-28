// Package errors provides shared error types for the JapaConnect backend.
//
// Sentinel errors defined here allow handlers to use errors.Is() for
// error matching instead of fragile string containment checks.
package errors

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// Domain errors — returned by service layers to indicate specific failure modes.
var (
	// ErrDuplicate indicates a unique constraint violation (e.g. duplicate email, slug).
	ErrDuplicate = errors.New("duplicate")

	// ErrAlreadyExists indicates the resource already exists.
	ErrAlreadyExists = errors.New("already_exists")

	// ErrNotFound indicates the requested resource was not found.
	ErrNotFound = errors.New("not_found")

	// ErrFeatureDisabled indicates the feature flag for the operation is off.
	ErrFeatureDisabled = errors.New("feature_disabled")

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

	// ErrInsufficientBalance indicates a wallet balance would go negative.
	ErrInsufficientBalance = errors.New("insufficient balance")

	// ErrInUse indicates a resource cannot be deleted because it is
	// referenced by other records (e.g. a field group with existing values).
	ErrInUse = errors.New("in use")

	// ErrNotDeleted indicates a hard delete was attempted on a row that is
	// not soft-deleted yet — force-delete requires a prior soft delete.
	ErrNotDeleted = errors.New("not soft-deleted")

	// ErrNotImplemented indicates the operation is recognized but not yet
	// implemented (e.g. a provider integration that is still pending).
	ErrNotImplemented = errors.New("not implemented")

	// ErrScoringDisabled indicates that scoring is disabled for the comparison type.
	ErrScoringDisabled = errors.New("scoring_disabled")

	// ErrRateLimited indicates the caller exceeded a rate or quota limit.
	ErrRateLimited = errors.New("rate_limited")
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

// IsFKViolation reports whether err is a Postgres foreign-key violation
// (SQLSTATE 23503). Force-delete paths use it to map dependent-row
// conflicts to ErrInUse.
func IsFKViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}
