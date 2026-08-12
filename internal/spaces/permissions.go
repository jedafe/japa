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
	space gen.GetSpaceByIDRow,
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

// CanReplyInSpace reports whether the given user may reply to content in the
// space, based on the space's replying_perm axis (Section 05 lines 145-154,
// BUG-002). The axis values mirror those of CanPostInSpace.
func (s *Service) CanReplyInSpace(
	ctx context.Context,
	space gen.GetSpaceByIDRow,
	userID, tenantID pgtype.UUID,
	permChecker PermissionChecker,
	verifiedChecker UserVerifiedChecker,
) bool {
	return canActInSpace(ctx, s, space.ReplyingPerm, space, userID, tenantID, permChecker, verifiedChecker)
}

// CanCommentInSpace reports whether the given user may comment on content in
// the space, based on the space's commenting_perm axis (Section 05 lines
// 145-154, BUG-002). The axis values mirror those of CanPostInSpace.
func (s *Service) CanCommentInSpace(
	ctx context.Context,
	space gen.GetSpaceByIDRow,
	userID, tenantID pgtype.UUID,
	permChecker PermissionChecker,
	verifiedChecker UserVerifiedChecker,
) bool {
	return canActInSpace(ctx, s, space.CommentingPerm, space, userID, tenantID, permChecker, verifiedChecker)
}

// canActInSpace is the shared enforcement logic for the posting, replying, and
// commenting permission axes (Section 05 lines 145-154). All three axes share
// the same value set and resolution rules; only the column differs.
func canActInSpace(
	ctx context.Context,
	s *Service,
	perm string,
	space gen.GetSpaceByIDRow,
	userID, tenantID pgtype.UUID,
	permChecker PermissionChecker,
	verifiedChecker UserVerifiedChecker,
) bool {
	switch perm {
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
