package auth

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

// IsOwnerEmail reports whether the given account email belongs to the
// platform owner designated by JC_OWNER_EMAIL. The comparison is
// case-insensitive; an empty ownerEmail disables ownership entirely.
// Ref: .jeos/sections/03-rbac.md (Amendment — Platform Owner & Super-Admin
// Account Lifecycle).
func IsOwnerEmail(email, ownerEmail string) bool {
	return ownerEmail != "" && strings.EqualFold(strings.TrimSpace(email), ownerEmail)
}

// SuperAdminPromoter adds a user to the super_admin group. Implemented by an
// adapter over the rbac store (wired in server.go) so the auth package does
// not import rbac — rbac already imports auth for request context.
type SuperAdminPromoter interface {
	PromoteToSuperAdmin(ctx context.Context, userID pgtype.UUID) error
}
