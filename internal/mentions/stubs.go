package mentions

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

// NoopBlockChecker is a stub implementation that always returns false.
// Replace with real implementation when Section 19 (user blocking) is complete.
type NoopBlockChecker struct{}

func (n *NoopBlockChecker) IsBlocked(ctx context.Context, blockerID, blockedID pgtype.UUID) (bool, error) {
	return false, nil
}

// NoopSpaceMemberChecker is a stub implementation that always returns true.
// Replace with real implementation when space membership queries are added.
type NoopSpaceMemberChecker struct{}

func (n *NoopSpaceMemberChecker) IsMember(ctx context.Context, spaceID, userID pgtype.UUID) (bool, error) {
	return true, nil
}

// NoopUserStatusChecker is a stub implementation that always returns false.
// Replace with real implementation when Section 19 (account erasure) is complete.
type NoopUserStatusChecker struct{}

func (n *NoopUserStatusChecker) IsErased(ctx context.Context, userID pgtype.UUID) (bool, error) {
	return false, nil
}
