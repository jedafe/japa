package tiers

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

// QuotaTierResolver is the minimal tier resolver needed for quota lookups —
// satisfied by *Service and by every package-local TierResolver interface.
type QuotaTierResolver interface {
	ResolveUserTier(ctx context.Context, userID pgtype.UUID) (string, error)
}

// QuotaSettingsReader is the subset of config.Settings needed for quota
// lookups — the hot-reloadable int reader.
type QuotaSettingsReader interface {
	GetIntOr(ctx context.Context, key string, fallback int) int
}

// QuotaLimit resolves the per-user limit for "{prefix}.{tier}": the caller's
// effective tier is resolved via tr; the tier-scoped key wins; a missing key
// falls back to staffDefaults[tier] for staff tiers, otherwise to flat (the
// flat "{prefix}" key's value, or flatFallback when settings is nil).
// Negative values mean unlimited (project convention). Fail-open: a resolver
// error keeps tier="regular" — never blocks the caller.
func QuotaLimit(ctx context.Context, tr QuotaTierResolver, sr QuotaSettingsReader, userID pgtype.UUID, prefix string, flatFallback int, staffDefaults map[string]int) int {
	flat := flatFallback
	if sr != nil {
		flat = sr.GetIntOr(ctx, prefix, flatFallback)
	}

	tier := "regular"
	if tr != nil && userID.Valid {
		if resolved, err := tr.ResolveUserTier(ctx, userID); err == nil && resolved != "" {
			tier = resolved
		}
	}

	fallback := flat
	if d, ok := staffDefaults[tier]; ok {
		fallback = d
	}

	if sr == nil {
		return fallback
	}

	return sr.GetIntOr(ctx, prefix+"."+tier, fallback)
}
