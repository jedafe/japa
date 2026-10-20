package nextactions

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/clientip"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/visitor"
)

// TierResolver is the subset of tiers.Service the resolver needs.
type TierResolver interface {
	ResolveUserTier(ctx context.Context, userID pgtype.UUID) (string, error)
}

// UserState is the per-request signal bundle predicates evaluate against.
type UserState struct {
	UserClass      string      // guest | regular | pro | business
	UserID         pgtype.UUID // invalid for guests
	SessionKey     string      // JWT JTI for users; client IP for guests
	AccountAgeDays int
	OnboardingDone bool
	EmailVerified  bool
	ProfilePct     int // 0-100 — display_name + avatar + bio completeness proxy
	PageViews      int // session counter (Valkey); populated by the service
	DwellSeconds   int // session counter (Valkey); populated by the service
	VCtx           visitor.VisitorContext
	Surface        string
}

// StateResolver builds UserState once per request — one batched user-row
// read plus tier/onboarding lookups, not per-predicate queries.
type StateResolver struct {
	read  *gen.Queries
	tiers TierResolver // nil → all users resolve as regular
}

// NewStateResolver creates a resolver backed by the read pool.
func NewStateResolver(read *gen.Queries, tiers TierResolver) *StateResolver {
	return &StateResolver{read: read, tiers: tiers}
}

// Resolve builds the state for the request. Never fails hard — a degraded
// signal (missing profile row, tier lookup error) resolves to safe defaults
// rather than erroring, per §122 graceful-degradation.
func (r *StateResolver) Resolve(ctx context.Context, req *http.Request, surface string) *UserState {
	s := &UserState{
		UserClass:  ClassGuest,
		SessionKey: clientip.FromRequest(req),
		VCtx:       visitor.FromContext(ctx),
		Surface:    surface,
	}

	u := auth.FromContext(ctx)
	if u == nil {
		return s
	}

	s.UserID = u.UserID
	s.UserClass = ClassRegular
	if u.SessionID != "" {
		s.SessionKey = u.SessionID
	}

	if r.read != nil {
		if row, err := r.read.GetUserForNextActionState(ctx, u.UserID); err == nil {
			if row.CreatedAt.Valid {
				s.AccountAgeDays = int(time.Since(row.CreatedAt.Time).Hours() / 24)
			}
			s.EmailVerified = row.EmailVerifiedAt.Valid
			s.ProfilePct = profilePct(row)
		}
		if ob, err := r.read.GetOnboardingState(ctx, u.UserID); err == nil {
			s.OnboardingDone = ob.Completed
		}
	}

	if r.tiers != nil {
		if tier, err := r.tiers.ResolveUserTier(ctx, u.UserID); err == nil && tier != "" {
			s.UserClass = tier
		}
	}

	return s
}

// profilePct scores profile completeness from the three fields that make a
// member look "set up": display name, avatar, bio. Each contributes a third.
// Ref: §122 profile_completeness predicate — intentionally simple v1 signal.
func profilePct(row gen.GetUserForNextActionStateRow) int {
	filled := 0
	for _, f := range []*string{row.DisplayName, row.AvatarUrl, row.Bio} {
		if f != nil && strings.TrimSpace(*f) != "" {
			filled++
		}
	}
	return filled * 100 / 3
}
