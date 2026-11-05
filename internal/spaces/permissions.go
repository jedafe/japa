package spaces

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// PermissionChecker checks whether a user holds a given permission node.
// Implemented by *rbac.RBAC; kept as an interface so the spaces package does
// not import rbac directly (avoids an import cycle).
type PermissionChecker interface {
	HasPermission(ctx context.Context, userID pgtype.UUID, permKey string) bool
}

// UserVerifiedChecker returns true if the user's email is verified. The
// "verified" posting axis (Section 05 line 149) requires a verified account.
// Implemented by a closure or store method.
type UserVerifiedChecker func(ctx context.Context, userID pgtype.UUID) (bool, error)

// SpaceDenial classifies why a write action (post/comment/reply) is denied in
// a space. It lets handlers render an actionable 403 message — the user sees
// the actual requirement (join, verify email, moderators only) instead of a
// bare "no permission" string.
type SpaceDenial string

const (
	DenialNone           SpaceDenial = ""
	DenialWikiOnly       SpaceDenial = "wiki_only"
	DenialSpaceInactive  SpaceDenial = "space_inactive"
	DenialJoinRequired   SpaceDenial = "join_required"
	DenialVerifiedOnly   SpaceDenial = "verified_only"
	DenialModeratorsOnly SpaceDenial = "moderators_only"
	DenialAdminsOnly     SpaceDenial = "admins_only"
	DenialRestricted     SpaceDenial = "restricted"
)

// denialNoun maps a write action to the plural noun used in denial messages.
var denialNoun = map[string]string{
	"post":    "posts",
	"comment": "comments",
	"reply":   "replies",
}

// denialVerb maps a write action to the verb used in denial messages. Any
// unrecognized action falls back to "comment".
func denialVerb(action string) string {
	switch action {
	case "post":
		return "post"
	case "reply":
		return "reply"
	default:
		return "comment"
	}
}

// DenialMessage renders the user-facing explanation for a denied space write
// action. action is "post", "comment", or "reply". It describes the concrete
// requirement the user is missing without exposing internal state (e.g. why
// a space is inactive). Returns "" for DenialNone.
func DenialMessage(d SpaceDenial, action string) string {
	verb := denialVerb(action)
	noun := denialNoun[verb]

	switch d {
	case DenialWikiOnly:
		return "This space is wiki-only"
	case DenialSpaceInactive:
		return fmt.Sprintf("This space is not accepting new %s.", noun)
	case DenialJoinRequired:
		return fmt.Sprintf("You must join this space to %s.", verb)
	case DenialVerifiedOnly:
		return fmt.Sprintf("You must verify your email address to %s in this space.", verb)
	case DenialModeratorsOnly:
		return fmt.Sprintf("Only this space's moderators can %s.", verb)
	case DenialAdminsOnly:
		return fmt.Sprintf("Only administrators can %s in this space.", verb)
	case DenialRestricted:
		return fmt.Sprintf("You do not have permission to %s in this space.", verb)
	default:
		return ""
	}
}

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
	userID pgtype.UUID,
	permChecker PermissionChecker,
	verifiedChecker UserVerifiedChecker,
) bool {
	return s.spaceWriteDenial(ctx, space.PostingPerm, space, userID, permChecker, verifiedChecker) == DenialNone
}

// CanReplyInSpace reports whether the given user may reply to content in the
// space, based on the space's replying_perm axis (Section 05 lines 145-154,
// BUG-002). The axis values mirror those of CanPostInSpace.
func (s *Service) CanReplyInSpace(
	ctx context.Context,
	space gen.GetSpaceByIDRow,
	userID pgtype.UUID,
	permChecker PermissionChecker,
	verifiedChecker UserVerifiedChecker,
) bool {
	return s.spaceWriteDenial(ctx, space.ReplyingPerm, space, userID, permChecker, verifiedChecker) == DenialNone
}

// CanCommentInSpace reports whether the given user may comment on content in
// the space, based on the space's commenting_perm axis (Section 05 lines
// 145-154, BUG-002). The axis values mirror those of CanPostInSpace.
func (s *Service) CanCommentInSpace(
	ctx context.Context,
	space gen.GetSpaceByIDRow,
	userID pgtype.UUID,
	permChecker PermissionChecker,
	verifiedChecker UserVerifiedChecker,
) bool {
	return s.spaceWriteDenial(ctx, space.CommentingPerm, space, userID, permChecker, verifiedChecker) == DenialNone
}

// SpaceWriteDenial evaluates the given perm axis for the user in the space
// and returns the denial reason — DenialNone when the action is allowed.
// Exported for the PostCheckerAdapter's ExplainSpaceDenial; callers that only
// need a verdict should use the Can*InSpace methods.
func (s *Service) SpaceWriteDenial(
	ctx context.Context,
	perm string,
	space gen.GetSpaceByIDRow,
	userID pgtype.UUID,
	permChecker PermissionChecker,
	verifiedChecker UserVerifiedChecker,
) SpaceDenial {
	return s.spaceWriteDenial(ctx, perm, space, userID, permChecker, verifiedChecker)
}

// spaceWriteDenial is the shared enforcement logic for the posting, replying,
// and commenting permission axes (Section 05 lines 145-154). All three axes
// share the same value set and resolution rules; only the column differs.
func (s *Service) spaceWriteDenial(
	ctx context.Context,
	perm string,
	space gen.GetSpaceByIDRow,
	userID pgtype.UUID,
	permChecker PermissionChecker,
	verifiedChecker UserVerifiedChecker,
) SpaceDenial {
	// §80 wiki-only space types accept no community writes — structural rule,
	// applied before the axis so even admin-granted writes can't land.
	if s.IsWikiOnlyType(ctx, space.SpaceTypeID) {
		return DenialWikiOnly
	}

	// Non-active spaces are frozen — no posts/replies/comments regardless of
	// the axis (BUG-040).
	if space.Status != StatusActive {
		return DenialSpaceInactive
	}

	switch perm {
	case PermOpen:
		return DenialNone
	case PermMembers:
		isMember, err := s.IsMember(ctx, space.ID, userID)
		if err != nil || !isMember {
			return DenialJoinRequired
		}

		return DenialNone
	case PermVerified:
		isMember, err := s.IsMember(ctx, space.ID, userID)
		if err != nil || !isMember {
			return DenialJoinRequired
		}

		if verifiedChecker == nil {
			return DenialRestricted
		}

		verified, err := verifiedChecker(ctx, userID)
		if err != nil || !verified {
			return DenialVerifiedOnly
		}

		return DenialNone
	case PermModerators:
		isMod, err := s.IsModerator(ctx, space.ID, userID)
		if err != nil || !isMod {
			return DenialModeratorsOnly
		}

		return DenialNone
	case PermAdmins:
		if permChecker == nil {
			return DenialRestricted
		}

		if permChecker.HasPermission(ctx, userID, "spaces.moderate.any") {
			return DenialNone
		}

		return DenialAdminsOnly
	default:
		return DenialRestricted
	}
}
