package mentions

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

// NoopBlockChecker is a stub implementation that always returns false.
// Replace with real implementation when Section 19 (user blocking) is complete.
type NoopBlockChecker struct{}

// IsBlocked always returns false for the noop implementation.
func (n *NoopBlockChecker) IsBlocked(_ context.Context, _, _ pgtype.UUID) (bool, error) {
	return false, nil
}

// NoopSpaceMemberChecker is a stub implementation that always returns true.
// Replace with real implementation when space membership queries are added.
type NoopSpaceMemberChecker struct{}

// IsMember always returns true for the noop implementation.
func (n *NoopSpaceMemberChecker) IsMember(_ context.Context, _, _ pgtype.UUID) (bool, error) {
	return true, nil
}

// NoopUserStatusChecker is a stub implementation that always returns false.
// Replace with real implementation when Section 19 (account erasure) is complete.
type NoopUserStatusChecker struct{}

// IsErased always returns false for the noop implementation.
func (n *NoopUserStatusChecker) IsErased(_ context.Context, _ pgtype.UUID) (bool, error) {
	return false, nil
}
