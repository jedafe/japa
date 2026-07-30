package spaces

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// PermissionChecker checks whether a user holds a given permission node.
// Implemented by *rbac.RBAC; kept as an interface so the spaces package does
// not import rbac directly (avoids an import cycle).
type PermissionChecker interface {
	HasPermission(ctx context.Context, userID, tenantID pgtype.UUID, permKey string) bool
}

// UserVerifiedChecker returns true if the user's email is verified. The
// "verified" posting axis (Section 05 line 149) requires a verified account.
// Implemented by a closure or store method.
type UserVerifiedChecker func(ctx context.Context, userID, tenantID pgtype.UUID) (bool, error)

// CanPostInSpace reports whether the given user may post content into the
// space, based on the space's posting_perm axis (Section 05 lines 145-154,
// BUG-003).
//
//   - "open":       anyone may post.
//   - "members":    the user must be an approved member of the space.
//   - "verified":   the user must be a verified member of the space.
//   - "moderators": the user must be a moderator of the space.
//   - "admins":     the user must hold the spaces.moderate.any permission.
func (s *Service) CanPostInSpace(
	ctx context.Context,
	space gen.Space,
	userID, tenantID pgtype.UUID,
	permChecker PermissionChecker,
	verifiedChecker UserVerifiedChecker,
) bool {
	switch space.PostingPerm {
	case PermOpen:
		return true
	case PermMembers:
		isMember, err := s.IsMember(ctx, space.ID, userID, tenantID)
		if err != nil || !isMember {
			return false
		}
		return true
	case PermVerified:
		isMember, err := s.IsMember(ctx, space.ID, userID, tenantID)
		if err != nil || !isMember {
			return false
		}
		if verifiedChecker == nil {
			return false
		}
		verified, err := verifiedChecker(ctx, userID, tenantID)
		if err != nil || !verified {
			return false
		}
		return true
	case PermModerators:
		isMod, err := s.IsModerator(ctx, space.ID, userID)
		if err != nil || !isMod {
			return false
		}
		return true
	case PermAdmins:
		if permChecker == nil {
			return false
		}
		return permChecker.HasPermission(ctx, userID, tenantID, "spaces.moderate.any")
	default:
		return false
	}
}
