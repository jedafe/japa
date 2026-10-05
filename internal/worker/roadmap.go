// roadmap.go — §113 worker handler for the public product roadmap:
//
//	roadmap:ship_notify — fan out a roadmap_shipped in-app notification to
//	                      every voter plus the item's submitter when a
//	                      roadmap item transitions to shipped
//
// Ref: japaconnect-engineering/sections/113-public-product-roadmap.md
//
//	japaconnect-engineering/sections/11-job-queue-asynq.md

package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
)

// HandleRoadmapShipNotify notifies every voter and the submitter that a
// roadmap item shipped (§113). The payload carries {item_id, title, slug}
// per the §11 task catalog; the row is re-read for submitted_by and as a
// fallback for title/slug so a stale payload cannot misreport. Enqueued
// fire-and-forget by AdminSetStatus — fan-out failures on individual
// recipients are logged, never fatal, so one bad row can't starve the rest.
func HandleRoadmapShipNotify(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:roadmap:ship_notify")

	queries, err := requireDB()
	if err != nil {
		slog.Warn("task:roadmap:ship_notify: no database, skipping", "error", err)
		return nil
	}

	var payload struct {
		ItemID string `json:"item_id"`
		Title  string `json:"title"`
		Slug   string `json:"slug"`
	}

	if err := parsePayloadOrSkip(t, &payload); err != nil {
		slog.Error("task:roadmap:ship_notify: invalid payload", "error", err)
		return err
	}

	itemID, err := scanUUID(payload.ItemID)
	if err != nil {
		return fmt.Errorf("task:roadmap:ship_notify: %w", err)
	}

	item, err := queries.GetRoadmapItemByID(ctx, itemID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Hard-deleted between enqueue and dispatch — nothing to notify.
			slog.Warn("task:roadmap:ship_notify: item gone, skipping", "item_id", payload.ItemID)
			return nil
		}

		return fmt.Errorf("task:roadmap:ship_notify: get item: %w", err)
	}

	voters, err := queries.ListRoadmapVoterIDs(ctx, itemID)
	if err != nil {
		return fmt.Errorf("task:roadmap:ship_notify: list voters: %w", err)
	}

	// Recipients = voters + submitter, deduped — the submitter may have
	// voted on their own request.
	recipients := make(map[pgtype.UUID]struct{}, len(voters)+1)
	for _, v := range voters {
		recipients[v] = struct{}{}
	}

	if item.SubmittedBy.Valid {
		recipients[item.SubmittedBy] = struct{}{}
	}

	if len(recipients) == 0 {
		slog.Info("task:roadmap:ship_notify: no recipients", "item_id", payload.ItemID)
		return nil
	}

	title := payload.Title
	if title == "" {
		title = item.Title
	}

	slug := payload.Slug
	if slug == "" {
		slug = item.Slug
	}

	notified := insertRoadmapShipNotifications(ctx, recipients, itemID, title, slug)

	slog.Info("task:roadmap:ship_notify: complete",
		"item_id", payload.ItemID, "recipient_count", len(recipients), "notified", notified)

	return nil
}

// insertRoadmapShipNotifications writes one roadmap_shipped in-app
// notification per recipient on the writer pool. Per-recipient failures
// are logged and counted, never fatal, so one bad row can't starve the
// rest of the fan-out (§35 notification insert path, same as the
// service-booking and SLA-breach fan-outs).
func insertRoadmapShipNotifications(ctx context.Context, recipients map[pgtype.UUID]struct{}, itemID pgtype.UUID, title, slug string) int {
	actorType, subjectType := "system", "roadmap_item"
	notifTitle := "Shipped: " + title
	bodyStr := fmt.Sprintf("A roadmap item you follow has shipped: %s.", title)
	actionURL := "/roadmap/" + slug

	notified := 0
	writer := gen.New(dbPool.Writer)

	for rid := range recipients {
		if _, nErr := writer.InsertNotification(ctx, gen.InsertNotificationParams{
			RecipientID:      rid,
			NotificationType: "roadmap_shipped", // seeded by migration 000796 (§113)
			ActorType:        &actorType,
			SubjectType:      &subjectType,
			SubjectID:        itemID,
			Title:            notifTitle,
			Body:             &bodyStr,
			ActionUrl:        &actionURL,
		}); nErr != nil {
			slog.Error("task:roadmap:ship_notify: notification failed",
				"item_id", itemID.String(), "user_id", rid.String(), "error", nErr)
			continue
		}

		notified++
	}

	return notified
}
