// Package visibility defines the shared VisibilityFilter type used by the
// content service, handlers, and dependent packages to enforce status and
// visibility gating in SQL queries.
// // It lives in its own package to avoid an import cycle: the ai package needs
// to pass filters to content service interfaces without importing the content
// package (which itself imports spam/ai).
// // Ref: japaconnect-engineering/sections/14-content-type-system.md (visibility)
package visibility

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/rbac"
)

// VisibilityFilter carries the caller's viewing rights so the SQL layer can
// enforce status and visibility gating in a single query. A zero-value filter
// represents an unauthenticated guest: only published, public content is
// returned.
// //nolint:revive // exported: stutter naming is intentional.
type VisibilityFilter struct {
	RequesterID       pgtype.UUID
	CanSeeUnpublished bool
	CanSeeMembers     bool
	CanSeePrivate     bool
}

// Params returns the individual SQL parameters derived from the filter.
func (f VisibilityFilter) Params() (pgtype.UUID, bool, bool, bool) {
	return f.RequesterID, f.CanSeeUnpublished, f.CanSeeMembers, f.CanSeePrivate
}

// InternalVisibilityFilter returns a filter that bypasses the public-only
// gating for internal/admin tools and agents. The requester ID is recorded for
// author-priority ordering, but all visibility and status levels are exposed.
func InternalVisibilityFilter(requesterID pgtype.UUID) VisibilityFilter {
	return VisibilityFilter{
		RequesterID:       requesterID,
		CanSeeUnpublished: true,
		CanSeeMembers:     true,
		CanSeePrivate:     true,
	}
}

// VisibilityFilterFromRequest derives a filter from the authenticated caller
// and the request's RBAC resolver.
// // - Authors always see their own content.
// - Admins, super-admins, and holders of content.edit_any or
// admin.moderation may see all content (unpublished, members, and private).
// - Authenticated users may see members content in addition to public.
// - Guests may only see published public content.
// //nolint:revive // exported: stutter naming is intentional.
func VisibilityFilterFromRequest(r *http.Request) VisibilityFilter {
	filter := VisibilityFilter{}

	user := auth.FromContext(r.Context())
	if user == nil {
		return filter
	}

	filter.RequesterID = user.UserID
	filter.CanSeeMembers = true

	resolver := rbac.FromContext(r.Context())
	if resolver == nil {
		return filter
	}

	if resolver.IsSuperAdmin(r.Context(), user.UserID) ||
		resolver.HasPermission(r.Context(), user.UserID, "content.edit_any") ||
		resolver.HasPermission(r.Context(), user.UserID, "admin.moderation") {
		filter.CanSeeUnpublished = true
		filter.CanSeePrivate = true
	}

	return filter
}
