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
	"github.com/jedafe/japa/internal/auth"
)

// ImpersonationContext holds both the real admin and impersonated user IDs.
type ImpersonationContext struct { //nolint:revive // stuttering type name preserved for API compatibility
	AdminID        pgtype.UUID
	ImpersonatedID pgtype.UUID
}

type contextKey struct{ name string }

var impersonatedUserKey = contextKey{"impersonated_user"}

// EffectiveUserKey carries the resolved effective user ID (impersonated
// target when active, else the real authenticated user) for downstream
// enforcement such as ban checks. The auth.UserContextKey always keeps the
// REAL admin identity for audit logging — never overwrite it.
var EffectiveUserKey = contextKey{"effective_user"}

// ImpersonatedFromContext extracts the ImpersonationContext if present.
// Returns nil if no impersonation is active for this request.
func ImpersonatedFromContext(ctx context.Context) *ImpersonationContext {
	ic, _ := ctx.Value(impersonatedUserKey).(*ImpersonationContext)
	return ic
}

// EffectiveUserFromContext returns the effective user ID placed in context
// by the impersonation middleware. Returns the zero UUID when the middleware
// has not run (e.g. no auth at all).
func EffectiveUserFromContext(ctx context.Context) pgtype.UUID {
	id, _ := ctx.Value(EffectiveUserKey).(pgtype.UUID)
	return id
}

// ResolveEffectiveUserID resolves who is acting right now: the impersonated
// target when impersonation is active, otherwise the real authenticated user.
// Returns the effective ID, the admin ID (equal to the effective ID when not
// impersonating, zero when unauthenticated), and whether impersonation is
// active.
func ResolveEffectiveUserID(ctx context.Context) (effectiveID, adminID pgtype.UUID, isImpersonating bool) {
	if ic := ImpersonatedFromContext(ctx); ic != nil {
		return ic.ImpersonatedID, ic.AdminID, true
	}
	if user := auth.FromContext(ctx); user != nil && user.UserID.Valid {
		return user.UserID, user.UserID, false
	}
	return pgtype.UUID{}, pgtype.UUID{}, false
}

// EffectiveUserID returns the impersonated user ID when active, otherwise
// falls back to the real user ID from the auth context. This is the
// primary API for downstream handlers to resolve "who is acting right now.".
// Kept for backward compatibility: returns the zero UUID only when no auth
// is present at all.
func EffectiveUserID(ctx context.Context) pgtype.UUID {
	effective, _, _ := ResolveEffectiveUserID(ctx)
	return effective
}
