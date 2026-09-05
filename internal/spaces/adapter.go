package spaces

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// PostCheckerAdapter wraps the spaces Service so it satisfies the
// content.SpacePostChecker interface. It loads the space by ID, then delegates
// to Service.CanPostInSpace with the injected permission and verified
// checkers (BUG-003).
type PostCheckerAdapter struct {
	Service         *Service
	PermChecker     PermissionChecker
	VerifiedChecker UserVerifiedChecker
}

// NewPostCheckerAdapter creates an adapter for use as a content.SpacePostChecker.
func NewPostCheckerAdapter(svc *Service) *PostCheckerAdapter {
	return &PostCheckerAdapter{Service: svc}
}

// CanPostInSpace loads the space and checks the posting_perm axis. Super
// admins bypass the membership/permission check — they can post to any space
// without joining it first (Section 03 — admin override).
func (a *PostCheckerAdapter) CanPostInSpace(ctx context.Context, spaceID, userID, tenantID pgtype.UUID) (bool, error) {
	// Super admins bypass all posting-perm checks.
	isSuperAdmin, err := a.Service.IsSuperAdmin(ctx, tenantID, userID)
	if err == nil && isSuperAdmin {
		return true, nil
	}

	space, err := a.Service.GetSpace(ctx, spaceID, tenantID)
	if err != nil {
		return false, fmt.Errorf("loading space for post check: %w", err)
	}

	return a.Service.CanPostInSpace(ctx, space, userID, tenantID, a.PermChecker, a.VerifiedChecker), nil
}

// CanCommentInSpace loads the space and checks the commenting_perm axis
// (Section 05 lines 145-154, BUG-002). Super admins bypass the check.
func (a *PostCheckerAdapter) CanCommentInSpace(ctx context.Context, spaceID, userID, tenantID pgtype.UUID) (bool, error) {
	isSuperAdmin, err := a.Service.IsSuperAdmin(ctx, tenantID, userID)
	if err == nil && isSuperAdmin {
		return true, nil
	}

	space, err := a.Service.GetSpace(ctx, spaceID, tenantID)
	if err != nil {
		return false, fmt.Errorf("loading space for comment check: %w", err)
	}

	return a.Service.CanCommentInSpace(ctx, space, userID, tenantID, a.PermChecker, a.VerifiedChecker), nil
}

// CanReplyInSpace loads the space and checks the replying_perm axis
// (Section 05 lines 145-154, BUG-002). Super admins bypass the check.
func (a *PostCheckerAdapter) CanReplyInSpace(ctx context.Context, spaceID, userID, tenantID pgtype.UUID) (bool, error) {
	isSuperAdmin, err := a.Service.IsSuperAdmin(ctx, tenantID, userID)
	if err == nil && isSuperAdmin {
		return true, nil
	}

	space, err := a.Service.GetSpace(ctx, spaceID, tenantID)
	if err != nil {
		return false, fmt.Errorf("loading space for reply check: %w", err)
	}

	return a.Service.CanReplyInSpace(ctx, space, userID, tenantID, a.PermChecker, a.VerifiedChecker), nil
}
