// Package trending — background processor for computing trending snapshots.
// // The Processor computes velocity-based trending scores for content, tags,
// spaces, and users using raw SQL. It runs as an Asynq periodic task.
package trending

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db"
)

// trendingWindows defines the time windows for trending computation.
// Expiry durations are defaults; BUG-016 makes them configurable via
// system_settings (trending.expiry.<window>_hours).
var trendingWindows = []struct {
	Name        string
	ExpiryHours int
}{
	{"24h", 2},
	{"7d", 8},
	{"30d", 32},
}

// scoringConfig holds the configurable trending scoring parameters read from
// system_settings (BUG-016, Rule 3). Defaults match the original hardcoded
// values so behaviour is unchanged when settings are absent.
type scoringConfig struct {
	// Content/User velocity weights (BUG-002: HN gravity formula).
	ViewsWeight     float64
	CommentsWeight  float64
	VotesWeight     float64
	BookmarksWeight float64
	SharesWeight    float64
	Gravity         float64 // HN gravity exponent (default 1.8, ADR-032)

	// Tag trending weights.
	TagFollowerWeight float64
	TagContentWeight  float64

	// Space trending weights.
	SpaceMemberWeight float64
	SpacePostWeight   float64
	SpaceRecentWeight float64

	// Expiry durations (hours) per window.
	Expiry24hHours int
	Expiry7dHours  int
	Expiry30dHours int
}

// defaultScoringConfig returns the default scoring configuration matching the
// original hardcoded values.
func defaultScoringConfig() scoringConfig {
	return scoringConfig{
		ViewsWeight:       1.0,
		CommentsWeight:    5.0,
		VotesWeight:       10.0,
		BookmarksWeight:   3.0,
		SharesWeight:      8.0,
		Gravity:           1.8,
		TagFollowerWeight: 1.0,
		TagContentWeight:  3.0,
		SpaceMemberWeight: 1.0,
		SpacePostWeight:   2.0,
		SpaceRecentWeight: 5.0,
		Expiry24hHours:    2,
		Expiry7dHours:     8,
		Expiry30dHours:    32,
	}
}

// loadScoringConfig reads the scoring configuration from system_settings,
// falling back to defaults when keys are unset (BUG-016, Rule 3).
func (p *Processor) loadScoringConfig(ctx context.Context) scoringConfig {
	sc := defaultScoringConfig()
	if p.settings == nil {
		return sc
	}

	sc.ViewsWeight = p.settings.GetFloatOr(ctx, "trending.weight.views", sc.ViewsWeight)
	sc.CommentsWeight = p.settings.GetFloatOr(ctx, "trending.weight.comments", sc.CommentsWeight)
	sc.VotesWeight = p.settings.GetFloatOr(ctx, "trending.weight.votes", sc.VotesWeight)
	sc.BookmarksWeight = p.settings.GetFloatOr(ctx, "trending.weight.bookmarks", sc.BookmarksWeight)
	sc.SharesWeight = p.settings.GetFloatOr(ctx, "trending.weight.shares", sc.SharesWeight)
	sc.Gravity = p.settings.GetFloatOr(ctx, "trending.gravity", sc.Gravity)
	sc.TagFollowerWeight = p.settings.GetFloatOr(ctx, "trending.weight.tag_follower", sc.TagFollowerWeight)
	sc.TagContentWeight = p.settings.GetFloatOr(ctx, "trending.weight.tag_content", sc.TagContentWeight)
	sc.SpaceMemberWeight = p.settings.GetFloatOr(ctx, "trending.weight.space_member", sc.SpaceMemberWeight)
	sc.SpacePostWeight = p.settings.GetFloatOr(ctx, "trending.weight.space_post", sc.SpacePostWeight)
	sc.SpaceRecentWeight = p.settings.GetFloatOr(ctx, "trending.weight.space_recent", sc.SpaceRecentWeight)
	sc.Expiry24hHours = p.settings.GetIntOr(ctx, "trending.expiry.24h_hours", sc.Expiry24hHours)
	sc.Expiry7dHours = p.settings.GetIntOr(ctx, "trending.expiry.7d_hours", sc.Expiry7dHours)
	sc.Expiry30dHours = p.settings.GetIntOr(ctx, "trending.expiry.30d_hours", sc.Expiry30dHours)

	return sc
}

// expiryForWindow returns the expiry duration for the given window name.
func (sc scoringConfig) expiryForWindow(window string) time.Duration {
	hours := sc.Expiry24hHours

	switch window {
	case "7d":
		hours = sc.Expiry7dHours
	case "30d":
		hours = sc.Expiry30dHours
	}

	return time.Duration(hours) * time.Hour
}

// Processor computes trending snapshots for all tenants.
type Processor struct {
	db       *db.DB
	logger   *slog.Logger
	settings *config.Settings // nil → use defaults (BUG-016)
}

// NewProcessor creates a Processor backed by the given database.
func NewProcessor(database *db.DB, logger *slog.Logger) *Processor {
	return &Processor{db: database, logger: logger}
}

// SetSettings injects the system settings service for configurable scoring
// weights and expiry durations (BUG-016, Rule 3).
func (p *Processor) SetSettings(s *config.Settings) {
	p.settings = s
}

// ComputeTrending recomputes trending snapshots for all tenants.
// For each tenant it deletes expired snapshots, then computes velocity-based
// scores for content, tags, spaces, and users across three time windows.
// BUG-001: computes both global (NULL geo_code) and per-country snapshots so
// that geo-filtered reads (ADR-064) return results.
// // BUG-012: The per-tenant loop is sequential. For large tenant counts this
// could be parallelized by fanning out per-tenant as separate Asynq tasks
// (low tier). The current implementation is acceptable for V1 tenant counts
// because each query is time-bounded (published_at >= now() - interval) and
// uses the idx_contents_published_at index. If tenant count grows beyond
// ~100, consider dispatching computeForTenant as a separate task per tenant.
func (p *Processor) ComputeTrending(ctx context.Context) error {
	start := time.Now()

	p.logger.Info("trending: compute_trending starting")

	// Gather all active tenants.
	tenantRows, err := p.db.Writer.Query(ctx, `SELECT id FROM tenants`)
	if err != nil {
		return fmt.Errorf("listing tenants: %w", err)
	}
	defer tenantRows.Close()

	var tenantIDs [][16]byte

	for tenantRows.Next() {
		var id [16]byte
		if err := tenantRows.Scan(&id); err != nil {
			return fmt.Errorf("scanning tenant id: %w", err)
		}

		tenantIDs = append(tenantIDs, id)
	}

	if err := tenantRows.Err(); err != nil {
		return fmt.Errorf("iterating tenants: %w", err)
	}

	p.logger.Info("trending: found tenants", "count", len(tenantIDs))

	var totalSnapshots int64

	for _, tid := range tenantIDs {
		snaps, err := p.computeForTenant(ctx, tid)
		if err != nil {
			p.logger.Error("trending: failed for tenant", "tenant_id", hex.EncodeToString(tid[:]), "error", err)
			continue
		}

		totalSnapshots += snaps
	}

	p.logger.Info("trending: compute_trending complete",
		"tenants", len(tenantIDs),
		"snapshots", totalSnapshots,
		"elapsed", time.Since(start).String())

	return nil
}

// ComputeSuggestions is a no-op. Suggestions are computed on-demand via SQL
// queries (GetSuggestedUsers, GetSuggestedSpaces, GetSuggestedTags) and do
// not require pre-computation.
func (p *Processor) ComputeSuggestions(_ context.Context) error {
	p.logger.Info("trending: compute_suggestions is a no-op (computed on-demand)")
	return nil
}

// computeForTenant deletes expired snapshots and computes new ones for a tenant.
// BUG-001: computes both global (NULL geo_code) and per-country snapshots.
func (p *Processor) computeForTenant(ctx context.Context, tenantID [16]byte) (int64, error) {
	_ = // Load configurable scoring parameters (BUG-016).
		pgtype.UUID{Bytes: tenantID, Valid: true}
	sc := p.loadScoringConfig(ctx)

	// Delete expired snapshots for this tenant.
	_, err := p.db.Writer.Exec(ctx, `
		DELETE FROM trending_snapshots
		WHERE expires_at < now()
	`)
	if err != nil {
		return 0, fmt.Errorf("deleting expired snapshots: %w", err)
	}

	// Gather the tenant's active geo scopes — restricted to country codes that
	// actually have published content (resolved via the content→space chain,
	// same rollup as the feed's geo proximity). Using every spaces.geo_code
	// fans out to ~2000 seeded world geos and writes N duplicate snapshot rows
	// per entity per geo — on a populated DB that is millions of rows per run.
	geoRows, err := p.db.Writer.Query(ctx, `
		SELECT DISTINCT geo FROM (
			SELECT COALESCE(NULLIF(cp.geo_code, ''), NULLIF(sp.geo_code, ''), NULLIF(s.geo_code, '')) AS geo
			FROM contents c
			JOIN content_spaces csp ON csp.content_id = c.id
			JOIN spaces s ON s.id = csp.space_id AND s.deleted_at IS NULL
			LEFT JOIN spaces sp ON sp.id = s.parent_id AND sp.deleted_at IS NULL
			LEFT JOIN spaces cp ON cp.id = sp.parent_id AND cp.deleted_at IS NULL
			WHERE c.status = 'published' AND c.deleted_at IS NULL
		) t WHERE geo IS NOT NULL AND geo <> ''
	`, tenantID)
	if err != nil {
		return 0, fmt.Errorf("listing tenant geo scopes: %w", err)
	}

	var geoCodes []string

	for geoRows.Next() {
		var gc string
		if err := geoRows.Scan(&gc); err != nil {
			return 0, fmt.Errorf("scanning geo code: %w", err)
		}

		geoCodes = append(geoCodes, gc)
	}

	geoRows.Close()

	if err := geoRows.Err(); err != nil {
		return 0, fmt.Errorf("iterating geo codes: %w", err)
	}

	// Compute snapshots for global (NULL geo_code) + each geo scope.
	// nil → NULL in pgx, string → the country code.
	geoScopes := make([]any, 0, 1+len(geoCodes))
	geoScopes = append(geoScopes, nil) // global first

	for _, gc := range geoCodes {
		geoScopes = append(geoScopes, gc)
	}

	var total int64
	var errs []error

	// Error isolation: a failure in one entity type must not abort the
	// remaining types — otherwise a single broken query (e.g. a renamed
	// table) leaves tags/spaces/users permanently empty.
	for _, w := range trendingWindows {
		expiresAt := time.Now().Add(sc.expiryForWindow(w.Name))

		for _, geoScope := range geoScopes {
			contentN, err := p.computeContentTrending(ctx, w.Name, expiresAt, geoScope, sc)
			if err != nil {
				errs = append(errs, fmt.Errorf("content trending (%s, geo=%v): %w", w.Name, geoScope, err))
			} else {
				total += contentN
			}

			tagN, err := p.computeTagTrending(ctx, w.Name, expiresAt, geoScope, sc)
			if err != nil {
				errs = append(errs, fmt.Errorf("tag trending (%s, geo=%v): %w", w.Name, geoScope, err))
			} else {
				total += tagN
			}

			spaceN, err := p.computeSpaceTrending(ctx, w.Name, expiresAt, geoScope, sc)
			if err != nil {
				errs = append(errs, fmt.Errorf("space trending (%s, geo=%v): %w", w.Name, geoScope, err))
			} else {
				total += spaceN
			}

			userN, err := p.computeUserTrending(ctx, w.Name, expiresAt, geoScope, sc)
			if err != nil {
				errs = append(errs, fmt.Errorf("user trending (%s, geo=%v): %w", w.Name, geoScope, err))
			} else {
				total += userN
			}
		}
	}

	return total, errors.Join(errs...)
}

// computeContentTrending computes velocity scores for published content.
// BUG-002: uses the Section 32 HN gravity formula:
// //	velocity = engagement / (hours_old + 2)^gravity
// // where gravity comes from system_settings (default 1.8, ADR-032).
// BUG-001: geoScope is nil for global or a country code string for geo-scoped.
// BUG-016: weights are configurable via system_settings.
func (p *Processor) computeContentTrending(ctx context.Context, window string, expiresAt time.Time, geoScope any, sc scoringConfig) (int64, error) {
	interval := windowInterval(window)

	// The cgeo CTE resolves each content's country geo via the content_spaces →
	// spaces parent-chain rollup (same rule as the feed's geo proximity). In
	// global mode ($11 IS NULL) every published content is included once; in
	// scoped mode only contents whose resolved geo matches are written, so a
	// per-geo snapshot contains that geo's entities — not a relabelled copy of
	// the global list.
	result, err := p.db.Writer.Exec(ctx, `
		WITH cgeo AS (
			SELECT DISTINCT csp.content_id,
				COALESCE(NULLIF(cp.geo_code, ''), NULLIF(sp.geo_code, ''), NULLIF(s.geo_code, '')) AS geo_code
			FROM content_spaces csp
			JOIN spaces s ON s.id = csp.space_id AND s.deleted_at IS NULL
			LEFT JOIN spaces sp ON sp.id = s.parent_id AND sp.deleted_at IS NULL
			LEFT JOIN spaces cp ON cp.id = sp.parent_id AND cp.deleted_at IS NULL
		)
		INSERT INTO trending_snapshots (entity_type, entity_id, score, "window", geo_code, expires_at)
		SELECT
			'content',
			c.id,
			CASE
				WHEN EXTRACT(EPOCH FROM (now() - c.published_at)) > 0
				THEN GREATEST(0,
					(COALESCE(cs.view_count, 0) * $4 +
					 COALESCE(cs.comment_count, 0) * $5 +
					 COALESCE(cs.vote_score, 0) * $6 +
					 COALESCE(cs.bookmark_count, 0) * $7 +
					 (COALESCE(c.share_count, 0) + COALESCE(c.reshare_count, 0)) * $8) /
					POWER(EXTRACT(EPOCH FROM (now() - c.published_at)) / 3600.0 + 2, $9)
				)
				ELSE 0
			END AS score,
			$1,
			$10,
			$2
		FROM contents c
		LEFT JOIN content_scores cs ON cs.content_id = c.id
		LEFT JOIN cgeo g ON g.content_id = c.id AND g.geo_code = $10
		WHERE c.status = 'published'
		  AND c.visibility = 'public'
		  AND c.deleted_at IS NULL
		  AND c.published_at IS NOT NULL
		  AND c.published_at >= now() - $3::interval
		  AND ($10::text IS NULL OR g.content_id IS NOT NULL)
		ON CONFLICT (entity_type, entity_id, "window", geo_code)
		DO UPDATE SET
			score = EXCLUDED.score,
			computed_at = now(),
			expires_at = EXCLUDED.expires_at
	`, window, expiresAt, interval, sc.ViewsWeight, sc.CommentsWeight, sc.VotesWeight, sc.BookmarksWeight, sc.SharesWeight, sc.Gravity, geoScope)
	if err != nil {
		return 0, fmt.Errorf("upserting content trending: %w", err)
	}

	return result.RowsAffected(), nil
}

// computeTagTrending computes trending tags by follower count and recent usage.
// BUG-001: geoScope is nil for global or a country code string for geo-scoped.
// BUG-016: weights are configurable via system_settings.
func (p *Processor) computeTagTrending(ctx context.Context, window string, expiresAt time.Time, geoScope any, sc scoringConfig) (int64, error) {
	interval := windowInterval(window)

	// Scoped mode ($7) only includes tags used by in-window published content
	// resolved to that geo; the count likewise only counts geo-matched
	// contents. Global mode is unchanged.
	result, err := p.db.Writer.Exec(ctx, `
		WITH cgeo AS (
			SELECT DISTINCT csp.content_id,
				COALESCE(NULLIF(cp.geo_code, ''), NULLIF(sp.geo_code, ''), NULLIF(s.geo_code, '')) AS geo_code
			FROM content_spaces csp
			JOIN spaces s ON s.id = csp.space_id AND s.deleted_at IS NULL
			LEFT JOIN spaces sp ON sp.id = s.parent_id AND sp.deleted_at IS NULL
			LEFT JOIN spaces cp ON cp.id = sp.parent_id AND cp.deleted_at IS NULL
		)
		INSERT INTO trending_snapshots (entity_type, entity_id, score, "window", geo_code, expires_at)
		SELECT
			'tag',
			t.id,
			COALESCE(t.follower_count, 0)::float * $4 +
				COUNT(DISTINCT CASE WHEN $6::text IS NULL OR g.content_id IS NOT NULL THEN c.id END)::float * $5 AS score,
			$1,
			$6,
			$2
		FROM tags t
		LEFT JOIN content_tags ct ON ct.tag_id = t.id
		LEFT JOIN contents c ON c.id = ct.content_id
			AND c.status = 'published'
			AND c.deleted_at IS NULL
			AND c.published_at >= now() - $3::interval
		LEFT JOIN cgeo g ON g.content_id = c.id AND g.geo_code = $6
		WHERE ($6::text IS NULL OR EXISTS (
			SELECT 1 FROM content_tags cx
			JOIN contents cc ON cc.id = cx.content_id
				AND cc.status = 'published' AND cc.deleted_at IS NULL
				AND cc.published_at >= now() - $3::interval
			JOIN cgeo gg ON gg.content_id = cc.id AND gg.geo_code = $6
			WHERE cx.tag_id = t.id))
		GROUP BY t.id, t.follower_count
		ON CONFLICT (entity_type, entity_id, "window", geo_code)
		DO UPDATE SET
			score = EXCLUDED.score,
			computed_at = now(),
			expires_at = EXCLUDED.expires_at
	`, window, expiresAt, interval, sc.TagFollowerWeight, sc.TagContentWeight, geoScope)
	if err != nil {
		return 0, fmt.Errorf("upserting tag trending: %w", err)
	}

	return result.RowsAffected(), nil
}

// computeSpaceTrending computes trending spaces by member count and recent post activity.
// BUG-001: geoScope is nil for global or a country code string for geo-scoped.
// BUG-016: weights are configurable via system_settings.
func (p *Processor) computeSpaceTrending(ctx context.Context, window string, expiresAt time.Time, geoScope any, sc scoringConfig) (int64, error) {
	interval := windowInterval(window)

	result, err := p.db.Writer.Exec(ctx, `
		INSERT INTO trending_snapshots (entity_type, entity_id, score, "window", geo_code, expires_at)
		SELECT
			'space',
			s.id,
			COALESCE(s.member_count, 0)::float * $4 +
				COALESCE(s.post_count, 0)::float * $5 +
				recent.count::float * $6 AS score,
			$1,
			$7,
			$2
		FROM spaces s
		LEFT JOIN spaces sp ON sp.id = s.parent_id AND sp.deleted_at IS NULL
		LEFT JOIN spaces cp ON cp.id = sp.parent_id AND cp.deleted_at IS NULL
		LEFT JOIN LATERAL (
			SELECT COUNT(*) AS count
			FROM contents c
			WHERE c.space_id = s.id
			  AND c.tenant_id = s.tenant_id
			  AND c.status = 'published'
			  AND c.published_at >= now() - $3::interval
		) recent ON true
		WHERE s.deleted_at IS NULL
		  -- Scoped mode: only spaces whose resolved country geo (child →
		  -- parent rollup) matches the scope, so per-geo snapshots hold the
		  -- geo's own spaces rather than the full list relabelled.
		  AND ($7::text IS NULL OR
			COALESCE(NULLIF(cp.geo_code, ''), NULLIF(sp.geo_code, ''), NULLIF(s.geo_code, '')) = $7)
		ON CONFLICT (entity_type, entity_id, "window", geo_code)
		DO UPDATE SET
			score = EXCLUDED.score,
			computed_at = now(),
			expires_at = EXCLUDED.expires_at
	`, window, expiresAt, interval, sc.SpaceMemberWeight, sc.SpacePostWeight, sc.SpaceRecentWeight, geoScope)
	if err != nil {
		return 0, fmt.Errorf("upserting space trending: %w", err)
	}

	return result.RowsAffected(), nil
}

// computeUserTrending computes trending users by content engagement in the window.
// BUG-002: uses the Section 32 HN gravity formula.
// BUG-001: geoScope is nil for global or a country code string for geo-scoped.
// BUG-016: weights are configurable via system_settings.
func (p *Processor) computeUserTrending(ctx context.Context, window string, expiresAt time.Time, geoScope any, sc scoringConfig) (int64, error) {
	interval := windowInterval(window)

	result, err := p.db.Writer.Exec(ctx, `
		WITH cgeo AS (
			SELECT DISTINCT csp.content_id,
				COALESCE(NULLIF(cp.geo_code, ''), NULLIF(sp.geo_code, ''), NULLIF(s.geo_code, '')) AS geo_code
			FROM content_spaces csp
			JOIN spaces s ON s.id = csp.space_id AND s.deleted_at IS NULL
			LEFT JOIN spaces sp ON sp.id = s.parent_id AND sp.deleted_at IS NULL
			LEFT JOIN spaces cp ON cp.id = sp.parent_id AND cp.deleted_at IS NULL
		)
		INSERT INTO trending_snapshots (entity_type, entity_id, score, "window", geo_code, expires_at)
		SELECT
			'user',
			c.author_id,
			GREATEST(0,
				SUM(
					COALESCE(cs.view_count, 0) * $4 +
					COALESCE(cs.comment_count, 0) * $5 +
					COALESCE(cs.vote_score, 0) * $6 +
					COALESCE(cs.bookmark_count, 0) * $7 +
					(COALESCE(c.share_count, 0) + COALESCE(c.reshare_count, 0)) * $8
				) / POWER(GREATEST(1, EXTRACT(EPOCH FROM (now() - MIN(c.published_at))) / 3600.0) + 2, $9)
			) AS score,
			$1,
			$10,
			$2
		FROM contents c
		LEFT JOIN content_scores cs ON cs.content_id = c.id
		LEFT JOIN cgeo g ON g.content_id = c.id AND g.geo_code = $10
		WHERE c.status = 'published'
		  AND c.deleted_at IS NULL
		  AND c.author_id IS NOT NULL
		  AND c.published_at >= now() - $3::interval
		  AND ($10::text IS NULL OR g.content_id IS NOT NULL)
		  AND EXISTS (SELECT 1 FROM users au WHERE au.id = c.author_id AND au.status = 'active')
		GROUP BY c.author_id
		ON CONFLICT (entity_type, entity_id, "window", geo_code)
		DO UPDATE SET
			score = EXCLUDED.score,
			computed_at = now(),
			expires_at = EXCLUDED.expires_at
	`, window, expiresAt, interval, sc.ViewsWeight, sc.CommentsWeight, sc.VotesWeight, sc.BookmarksWeight, sc.SharesWeight, sc.Gravity, geoScope)
	if err != nil {
		return 0, fmt.Errorf("upserting user trending: %w", err)
	}

	return result.RowsAffected(), nil
}

// windowInterval maps a window name to a PostgreSQL interval string.
func windowInterval(window string) string {
	switch window {
	case "7d":
		return "7 days"
	case "30d":
		return "30 days"
	default:
		return "24 hours"
	}
}
