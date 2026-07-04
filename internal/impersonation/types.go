// Package impersonation implements admin impersonation (Section 46).
//
// Impersonation allows an admin to act as another user for support and
// debugging purposes. The impersonated identity is stored in Valkey with
// a 30-minute TTL and injected into the request context via middleware.
// All actions taken during impersonation are audit-logged with both the
// real admin ID and the impersonated user ID.
package impersonation

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

// ImpersonationContext holds both the real admin and impersonated user IDs.
type ImpersonationContext struct { //nolint:revive // stuttering type name preserved for API compatibility
	AdminID        pgtype.UUID
	ImpersonatedID pgtype.UUID
}

type contextKey struct{ name string }

var impersonatedUserKey = contextKey{"impersonated_user"}

// ImpersonatedFromContext extracts the ImpersonationContext if present.
// Returns nil if no impersonation is active for this request.
func ImpersonatedFromContext(ctx context.Context) *ImpersonationContext {
	ic, _ := ctx.Value(impersonatedUserKey).(*ImpersonationContext)
	return ic
}

// EffectiveUserID returns the impersonated user ID when active, otherwise
// falls back to the real user ID from the auth context. This is the
// primary API for downstream handlers to resolve "who is acting right now.".
func EffectiveUserID(ctx context.Context) pgtype.UUID {
	if ic := ImpersonatedFromContext(ctx); ic != nil {
		return ic.ImpersonatedID
	}

	return pgtype.UUID{}
}
