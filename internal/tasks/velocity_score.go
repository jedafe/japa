package tasks

import (
	"context"
	"log/slog"
	"time"

	"github.com/jedafe/japa/internal/db"
)

// VelocityScorer recalculates velocity scores for recently published content.
// Velocity measures engagement speed: how quickly content accumulates views,
// comments, votes, bookmarks, and shares relative to its age.
type VelocityScorer struct {
	database *db.DB
}

// NewVelocityScorer creates a VelocityScorer backed by the given database.
func NewVelocityScorer(database *db.DB) *VelocityScorer {
	return &VelocityScorer{database: database}
}

// RecalculateScores updates velocity scores for all published content
// created in the last 30 days. The formula is:
//
//	velocity = (views*1 + comments*5 + votes*10 + bookmarks*3 + shares*8) / hours_since_publish
//
// Content with zero hours since publish gets a velocity of 0 to avoid
// division by zero. Returns the number of content items updated.
func (vs *VelocityScorer) RecalculateScores(ctx context.Context) (int, error) {
	// Upsert engagement rows for any published content that doesn't have one yet,
	// then recalculate velocity scores for all active content.
	_, err := vs.database.Writer.Exec(ctx, `
		-- Ensure every published content from the last 30 days has an engagement row.
		INSERT INTO content_engagement (content_id, views, comments_count, votes_score, bookmarks_count, shares_count)
		SELECT c.id, 0, 0, 0, 0, 0
		FROM contents c
		LEFT JOIN content_engagement ce ON ce.content_id = c.id
		WHERE c.status = 'published'
		  AND c.created_at >= now() - INTERVAL '30 days'
		  AND ce.content_id IS NULL
		ON CONFLICT (content_id) DO NOTHING
	`)
	if err != nil {
		slog.Error("velocity_score: failed to ensure engagement rows", "error", err)
		return 0, err
	}

	// Recalculate velocity scores.
	// Formula: (views*1 + comments*5 + votes*10 + bookmarks*3 + shares*8) / hours_since_publish
	result, err := vs.database.Writer.Exec(ctx, `
		UPDATE content_engagement ce
		SET velocity_score = CASE
		      WHEN c.published_at IS NOT NULL
		           AND EXTRACT(EPOCH FROM (now() - c.published_at)) > 0
		      THEN GREATEST(0,
		           (ce.views * 1.0 +
		            ce.comments_count * 5.0 +
		            ce.votes_score * 10.0 +
		            ce.bookmarks_count * 3.0 +
		            ce.shares_count * 8.0) /
		           (EXTRACT(EPOCH FROM (now() - c.published_at)) / 3600.0))
		      ELSE 0
		    END,
		    last_calculated_at = now(),
		    updated_at = now()
		FROM contents c
		WHERE ce.content_id = c.id
		  AND c.status = 'published'
		  AND c.created_at >= now() - INTERVAL '30 days'
	`)
	if err != nil {
		slog.Error("velocity_score: failed to recalculate scores", "error", err)
		return 0, err
	}

	rowsAffected := result.RowsAffected()
	slog.Info("velocity_score: recalculation complete",
		"updated", rowsAffected,
		"timestamp", time.Now().UTC().Format(time.RFC3339))

	return int(rowsAffected), nil
}
