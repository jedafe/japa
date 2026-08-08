package mentions

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// DBCheckers is a production implementation of BlockChecker, SpaceMemberChecker,
// and UserStatusChecker backed by the database.
type DBCheckers struct {
	read  *gen.Queries
}

// NewDBCheckers creates a DBCheckers backed by the given database reader pool.
func NewDBCheckers(database *db.DB) *DBCheckers {
	return &DBCheckers{read: gen.New(database.Reader)}
}

// IsBlocked checks whether a user block exists between two users in either direction.
// Tenant-scoped per JEOS §9 (BUG-006).
func (c *DBCheckers) IsBlocked(ctx context.Context, tenantID, blockerID, blockedID pgtype.UUID) (bool, error) {
	blocked, err := c.read.IsBlocked(ctx, gen.IsBlockedParams{
		BlockerID: blockerID,
		BlockedID: blockedID,
		TenantID:  tenantID,
	})
	return blocked, err
}

// IsMember checks whether a user is a member of a space.
// Tenant-scoped per Section 04 (BUG-005).
func (c *DBCheckers) IsMember(ctx context.Context, tenantID, spaceID, userID pgtype.UUID) (bool, error) {
	isMember, err := c.read.CheckSpaceMembership(ctx, gen.CheckSpaceMembershipParams{
		SpaceID:  spaceID,
		UserID:   userID,
		TenantID: tenantID,
	})
	return isMember, err
}

// IsErased checks whether a user's account has been deleted/erased.
// Tenant-scoped per Section 04 (BUG-009).
func (c *DBCheckers) IsErased(ctx context.Context, tenantID, userID pgtype.UUID) (bool, error) {
	status, err := c.read.GetUserStatus(ctx, gen.GetUserStatusParams{
		ID:       userID,
		TenantID: tenantID,
	})
	if err != nil {
		return false, err
	}
	return status == "deleted", nil
}
