package tasks

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
)

// ScheduledPublisher finds draft content whose publish_at has passed and
// transitions it to published. It operates across all tenants — the Asynq
// scheduler enqueues this task periodically (every 60 seconds).
type ScheduledPublisher struct {
	database *db.DB
}

// NewScheduledPublisher creates a ScheduledPublisher backed by the given database.
func NewScheduledPublisher(database *db.DB) *ScheduledPublisher {
	return &ScheduledPublisher{database: database}
}

// PublishReadyContent finds all draft content where publish_at <= now() and
// transitions each to 'published', setting published_at. Returns the number
// of content items published.
func (sp *ScheduledPublisher) PublishReadyContent(ctx context.Context) (int, error) {
	// Find draft content ready to publish.
	rows, err := sp.database.Reader.Query(ctx, `
		SELECT id, tenant_id
		FROM contents
		WHERE status = 'draft'
		  AND publish_at IS NOT NULL
		  AND publish_at <= now()
		ORDER BY publish_at ASC
		LIMIT 100
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type pendingContent struct {
		id       pgtype.UUID
		tenantID pgtype.UUID
	}

	var pending []pendingContent

	for rows.Next() {
		var pc pendingContent
		if err := rows.Scan(&pc.id, &pc.tenantID); err != nil {
			slog.Error("scheduled_publish: scan error", "error", err)
			continue
		}

		pending = append(pending, pc)
	}

	if err := rows.Err(); err != nil {
		return 0, err
	}

	if len(pending) == 0 {
		return 0, nil
	}

	published := 0

	for _, pc := range pending {
		_, err := sp.database.Writer.Exec(ctx, `
			UPDATE contents
			SET status = 'published',
			    published_at = COALESCE(published_at, now()),
			    updated_at = now()
			WHERE id = $1
			  AND tenant_id = $2
			  AND status = 'draft'
		`, pc.id, pc.tenantID)
		if err != nil {
			slog.Error("scheduled_publish: failed to publish content",
				"content_id", pc.id.String(), "error", err)
			continue
		}

		published++

		slog.Info("scheduled_publish: content published",
			"content_id", pc.id.String(),
			"tenant_id", pc.tenantID.String())
	}

	if published > 0 {
		slog.Info("scheduled_publish: batch complete", "published", published, "total", len(pending))
	}

	return published, nil
}
