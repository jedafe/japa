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
func (a *PostCheckerAdapter) CanPostInSpace(ctx context.Context, spaceID, userID pgtype.UUID) (bool, error) {
	space, err := a.Service.GetSpace(ctx, spaceID)
	if err != nil {
		return false, fmt.Errorf("loading space for post check: %w", err)
	}

	// §80 wiki-only space types accept no posts — structural rule checked
	// before the super-admin bypass (a wiki-only space has no feed for the
	// content to land in, so the override doesn't apply).
	if a.Service.IsWikiOnlyType(ctx, space.SpaceTypeID) {
		return false, ErrWikiOnlySpace
	}

	// Super admins bypass all posting-perm checks.
	isSuperAdmin, err := a.Service.IsSuperAdmin(ctx, userID)
	if err == nil && isSuperAdmin {
		return true, nil
	}

	return a.Service.CanPostInSpace(ctx, space, userID, a.PermChecker, a.VerifiedChecker), nil
}

// CanCommentInSpace loads the space and checks the commenting_perm axis
// (Section 05 lines 145-154, BUG-002). Super admins bypass the check.
func (a *PostCheckerAdapter) CanCommentInSpace(ctx context.Context, spaceID, userID pgtype.UUID) (bool, error) {
	space, err := a.Service.GetSpace(ctx, spaceID)
	if err != nil {
		return false, fmt.Errorf("loading space for comment check: %w", err)
	}

	// §80 wiki-only space types accept no comments (see CanPostInSpace).
	if a.Service.IsWikiOnlyType(ctx, space.SpaceTypeID) {
		return false, ErrWikiOnlySpace
	}

	isSuperAdmin, err := a.Service.IsSuperAdmin(ctx, userID)
	if err == nil && isSuperAdmin {
		return true, nil
	}

	return a.Service.CanCommentInSpace(ctx, space, userID, a.PermChecker, a.VerifiedChecker), nil
}

// ExplainSpaceDenial returns a user-facing message describing why the write
// action ("post", "comment", or "reply") is denied for the user in the space,
// or "" when the action is allowed. Handlers use it to answer 403s with the
// concrete missing requirement (join the space, verify email, moderators
// only) instead of a bare "no permission" string.
func (a *PostCheckerAdapter) ExplainSpaceDenial(ctx context.Context, spaceID, userID pgtype.UUID, action string) (string, error) {
	space, err := a.Service.GetSpace(ctx, spaceID)
	if err != nil {
		return "", fmt.Errorf("loading space for denial explanation: %w", err)
	}

	// §80 wiki-only space types accept no community writes (see CanPostInSpace).
	if a.Service.IsWikiOnlyType(ctx, space.SpaceTypeID) {
		return "", ErrWikiOnlySpace
	}

	// Super admins bypass all write-perm checks — no denial to explain.
	isSuperAdmin, err := a.Service.IsSuperAdmin(ctx, userID)
	if err == nil && isSuperAdmin {
		return "", nil
	}

	perm := space.CommentingPerm
	switch action {
	case "post":
		perm = space.PostingPerm
	case "reply":
		perm = space.ReplyingPerm
	}

	denial := a.Service.SpaceWriteDenial(ctx, perm, space, userID, a.PermChecker, a.VerifiedChecker)

	return DenialMessage(denial, action), nil
}

// CanReplyInSpace loads the space and checks the replying_perm axis
// (Section 05 lines 145-154, BUG-002). Super admins bypass the check.
func (a *PostCheckerAdapter) CanReplyInSpace(ctx context.Context, spaceID, userID pgtype.UUID) (bool, error) {
	space, err := a.Service.GetSpace(ctx, spaceID)
	if err != nil {
		return false, fmt.Errorf("loading space for reply check: %w", err)
	}

	// §80 wiki-only space types accept no replies (see CanPostInSpace).
	if a.Service.IsWikiOnlyType(ctx, space.SpaceTypeID) {
		return false, ErrWikiOnlySpace
	}

	isSuperAdmin, err := a.Service.IsSuperAdmin(ctx, userID)
	if err == nil && isSuperAdmin {
		return true, nil
	}

	return a.Service.CanReplyInSpace(ctx, space, userID, a.PermChecker, a.VerifiedChecker), nil
}
