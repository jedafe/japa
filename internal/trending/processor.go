// Package trending — background processor for computing trending snapshots.
//
// The Processor computes velocity-based trending scores for content, tags,
// spaces, and users using raw SQL. It runs as an Asynq periodic task.
package trending

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jedafe/japa/internal/db"
)

// trendingWindows defines the time windows for trending computation.
var trendingWindows = []struct {
	Name    string
	Expires time.Duration
}{
	{"24h", 2 * time.Hour},
	{"7d", 8 * time.Hour},
	{"30d", 32 * time.Hour},
}

// Processor computes trending snapshots for all tenants.
type Processor struct {
	db     *db.DB
	logger *slog.Logger
}

// NewProcessor creates a Processor backed by the given database.
func NewProcessor(database *db.DB, logger *slog.Logger) *Processor {
	return &Processor{db: database, logger: logger}
}

// ComputeTrending recomputes trending snapshots for all tenants.
// For each tenant it deletes expired snapshots, then computes velocity-based
// scores for content, tags, spaces, and users across three time windows.
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
			p.logger.Error("trending: failed for tenant", "tenant_id", fmt.Sprintf("%x", tid), "error", err)
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
func (p *Processor) computeForTenant(ctx context.Context, tenantID [16]byte) (int64, error) {
	// Delete expired snapshots for this tenant.
	_, err := p.db.Writer.Exec(ctx, `
		DELETE FROM trending_snapshots
		WHERE tenant_id = $1 AND expires_at < now()
	`, tenantID)
	if err != nil {
		return 0, fmt.Errorf("deleting expired snapshots: %w", err)
	}

	var total int64

	for _, w := range trendingWindows {
		expiresAt := time.Now().Add(w.Expires)

		contentN, err := p.computeContentTrending(ctx, tenantID, w.Name, expiresAt)
		if err != nil {
			return total, fmt.Errorf("content trending (%s): %w", w.Name, err)
		}
		total += contentN

		tagN, err := p.computeTagTrending(ctx, tenantID, w.Name, expiresAt)
		if err != nil {
			return total, fmt.Errorf("tag trending (%s): %w", w.Name, err)
		}
		total += tagN

		spaceN, err := p.computeSpaceTrending(ctx, tenantID, w.Name, expiresAt)
		if err != nil {
			return total, fmt.Errorf("space trending (%s): %w", w.Name, err)
		}
		total += spaceN

		userN, err := p.computeUserTrending(ctx, tenantID, w.Name, expiresAt)
		if err != nil {
			return total, fmt.Errorf("user trending (%s): %w", w.Name, err)
		}
		total += userN
	}

	return total, nil
}

// computeContentTrending computes velocity scores for published content.
// Formula: (views*1 + comments*5 + votes*10 + bookmarks*3 + shares*8) / hours_since_publish
func (p *Processor) computeContentTrending(ctx context.Context, tenantID [16]byte, window string, expiresAt time.Time) (int64, error) {
	// Determine the time boundary for the window.
	interval := windowInterval(window)

	result, err := p.db.Writer.Exec(ctx, `
		INSERT INTO trending_snapshots (tenant_id, entity_type, entity_id, score, "window", geo_code, expires_at)
		SELECT
			c.tenant_id,
			'content',
			c.id,
			CASE
				WHEN EXTRACT(EPOCH FROM (now() - c.published_at)) > 0
				THEN GREATEST(0,
					(COALESCE(ce.views, 0) * 1.0 +
					 COALESCE(ce.comments_count, 0) * 5.0 +
					 COALESCE(ce.votes_score, 0) * 10.0 +
					 COALESCE(ce.bookmarks_count, 0) * 3.0 +
					 COALESCE(ce.shares_count, 0) * 8.0) /
					(EXTRACT(EPOCH FROM (now() - c.published_at)) / 3600.0))
				ELSE 0
			END AS score,
			$2,
			NULL,
			$3
		FROM contents c
		LEFT JOIN content_engagement ce ON ce.content_id = c.id
		WHERE c.tenant_id = $1
		  AND c.status = 'published'
		  AND c.visibility = 'public'
		  AND c.published_at IS NOT NULL
		  AND c.published_at >= now() - $4::interval
		ON CONFLICT (tenant_id, entity_type, entity_id, "window", geo_code)
		DO UPDATE SET
			score = EXCLUDED.score,
			computed_at = now(),
			expires_at = EXCLUDED.expires_at
	`, tenantID, window, expiresAt, interval)
	if err != nil {
		return 0, fmt.Errorf("upserting content trending: %w", err)
	}

	return result.RowsAffected(), nil
}

// computeTagTrending computes trending tags by follower count and recent usage.
func (p *Processor) computeTagTrending(ctx context.Context, tenantID [16]byte, window string, expiresAt time.Time) (int64, error) {
	interval := windowInterval(window)

	result, err := p.db.Writer.Exec(ctx, `
		INSERT INTO trending_snapshots (tenant_id, entity_type, entity_id, score, "window", geo_code, expires_at)
		SELECT
			t.tenant_id,
			'tag',
			t.id,
			COALESCE(t.follower_count, 0)::float * 1.0 +
				COUNT(DISTINCT ct.content_id)::float * 3.0 AS score,
			$2,
			NULL,
			$3
		FROM tags t
		LEFT JOIN content_tags ct ON ct.tag_id = t.id
		LEFT JOIN contents c ON c.id = ct.content_id
			AND c.status = 'published'
			AND c.published_at >= now() - $4::interval
		WHERE t.tenant_id = $1
		GROUP BY t.id, t.tenant_id, t.follower_count
		ON CONFLICT (tenant_id, entity_type, entity_id, "window", geo_code)
		DO UPDATE SET
			score = EXCLUDED.score,
			computed_at = now(),
			expires_at = EXCLUDED.expires_at
	`, tenantID, window, expiresAt, interval)
	if err != nil {
		return 0, fmt.Errorf("upserting tag trending: %w", err)
	}

	return result.RowsAffected(), nil
}

// computeSpaceTrending computes trending spaces by member count and recent post activity.
func (p *Processor) computeSpaceTrending(ctx context.Context, tenantID [16]byte, window string, expiresAt time.Time) (int64, error) {
	interval := windowInterval(window)

	result, err := p.db.Writer.Exec(ctx, `
		INSERT INTO trending_snapshots (tenant_id, entity_type, entity_id, score, "window", geo_code, expires_at)
		SELECT
			s.tenant_id,
			'space',
			s.id,
			COALESCE(s.member_count, 0)::float * 1.0 +
				COALESCE(s.post_count, 0)::float * 2.0 +
				recent.count::float * 5.0 AS score,
			$2,
			NULL,
			$3
		FROM spaces s
		LEFT JOIN LATERAL (
			SELECT COUNT(*) AS count
			FROM contents c
			WHERE c.space_id = s.id
			  AND c.tenant_id = s.tenant_id
			  AND c.status = 'published'
			  AND c.published_at >= now() - $4::interval
		) recent ON true
		WHERE s.tenant_id = $1
		ON CONFLICT (tenant_id, entity_type, entity_id, "window", geo_code)
		DO UPDATE SET
			score = EXCLUDED.score,
			computed_at = now(),
			expires_at = EXCLUDED.expires_at
	`, tenantID, window, expiresAt, interval)
	if err != nil {
		return 0, fmt.Errorf("upserting space trending: %w", err)
	}

	return result.RowsAffected(), nil
}

// computeUserTrending computes trending users by content engagement in the window.
func (p *Processor) computeUserTrending(ctx context.Context, tenantID [16]byte, window string, expiresAt time.Time) (int64, error) {
	interval := windowInterval(window)

	result, err := p.db.Writer.Exec(ctx, `
		INSERT INTO trending_snapshots (tenant_id, entity_type, entity_id, score, "window", geo_code, expires_at)
		SELECT
			c.tenant_id,
			'user',
			c.author_id,
			GREATEST(0,
				SUM(
					COALESCE(ce.views, 0) * 1.0 +
					COALESCE(ce.comments_count, 0) * 5.0 +
					COALESCE(ce.votes_score, 0) * 10.0 +
					COALESCE(ce.bookmarks_count, 0) * 3.0 +
					COALESCE(ce.shares_count, 0) * 8.0
				) / GREATEST(1, EXTRACT(EPOCH FROM (now() - MIN(c.published_at))) / 3600.0)
			) AS score,
			$2,
			NULL,
			$3
		FROM contents c
		LEFT JOIN content_engagement ce ON ce.content_id = c.id
		WHERE c.tenant_id = $1
		  AND c.status = 'published'
		  AND c.author_id IS NOT NULL
		  AND c.published_at >= now() - $4::interval
		GROUP BY c.tenant_id, c.author_id
		ON CONFLICT (tenant_id, entity_type, entity_id, "window", geo_code)
		DO UPDATE SET
			score = EXCLUDED.score,
			computed_at = now(),
			expires_at = EXCLUDED.expires_at
	`, tenantID, window, expiresAt, interval)
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
