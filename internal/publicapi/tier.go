package publicapi

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// GateHitTracker receives tier-gate denies (§122 feature.gate_hit feed).
type GateHitTracker func(ctx context.Context, userID pgtype.UUID, feature, requiredTier string)

var gateTracker GateHitTracker

// SetGateTracker wires the deny-path tracker (fire-and-forget by contract —
// it must never block or alter the 403).
func SetGateTracker(fn GateHitTracker) { gateTracker = fn }

// tierRanks orders unified user tiers lowest to highest. Unknown tiers
// resolve to rank 0 — denied by any positive minimum (fail-closed).
var tierRanks = map[string]int{
	"regular":  1,
	"pro":      2,
	"business": 3,
}

// RequireTier enforces that the API key owner's unified user tier is at
// least minTier. The tier is resolved per request via tr so a lapsed
// subscription revokes access immediately without key revocation.
// Ref: docs/superpowers/specs/2026-09-23-agent-data-api-design.md (Tier gating).
func RequireTier(tr TierResolver, minTier string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			keyCtx := APIKeyFromContext(r.Context())
			if keyCtx == nil {
				errpkg.WriteError(w, http.StatusUnauthorized, "api_key_required", "X-API-Key header is required")
				return
			}

			tier := "regular"

			if tr != nil {
				if resolved, err := tr.ResolveUserTier(r.Context(), keyCtx.UserID); err == nil && resolved != "" {
					tier = resolved
				}
			}

			if !TierAtLeast(tier, minTier) {
				if gateTracker != nil {
					gateTracker(r.Context(), keyCtx.UserID, r.URL.Path, minTier)
				}
				errpkg.WriteError(w, http.StatusForbidden, "tier_required",
					"This endpoint requires a "+minTier+" subscription")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// TierAtLeast reports whether tier meets minTier by rank. Unknown tiers
// rank 0 — fail-closed for any positive minimum. Exported so the MCP
// tool gate (Task 7) enforces the same premium-tier policy as REST;
// without it, MCP tools would bypass RequireTier entirely.
func TierAtLeast(tier, minTier string) bool {
	return tierRanks[tier] >= tierRanks[minTier]
}
