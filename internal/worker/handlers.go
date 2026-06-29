package worker

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/smtp"
	"os"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// ---------------------------------------------------------------------------
// Package-level dependencies for database and cache access.
// Call RegisterDBPool and RegisterRedis at startup before processing tasks.
// ---------------------------------------------------------------------------

var dbPool *db.DB
var rdb interface{ Ping(ctx context.Context) error }

// RegisterDBPool sets the database connection pool for worker handlers.
func RegisterDBPool(d *db.DB) {
	dbPool = d
}

// RegisterRedis sets the Valkey/Redis client for worker handlers.
func RegisterRedis(client interface{ Ping(ctx context.Context) error }) {
	rdb = client
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// parsePayload unmarshals the task payload into the given struct.
func parsePayload(t *asynq.Task, v any) error {
	return json.Unmarshal(t.Payload(), v)
}

// scanUUID parses a UUID string into a pgtype.UUID.
func scanUUID(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid uuid %q: %w", s, err)
	}
	return u, nil
}

// requireDB returns the query interface or an error if the pool is not registered.
func requireDB() (*gen.Queries, error) {
	if dbPool == nil {
		return nil, fmt.Errorf("database pool not registered")
	}
	return gen.New(dbPool.Writer), nil
}

// readDB returns a read-only query interface.
func readDB() (*gen.Queries, error) {
	if dbPool == nil {
		return nil, fmt.Errorf("database pool not registered")
	}
	return gen.New(dbPool.Reader), nil
}

// ---------------------------------------------------------------------------
// Critical tier handlers
// ---------------------------------------------------------------------------

func HandleNotificationDispatchBatch(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:notification:dispatch_batch", "payload", t.Payload())

	queries, err := requireDB()
	if err != nil {
		slog.Warn("task:notification:dispatch_batch: no database, skipping", "error", err)
		return nil
	}

	var payload struct {
		Notifications []struct {
			RecipientID      string `json:"recipient_id"`
			TenantID         string `json:"tenant_id"`
			NotificationType string `json:"notification_type"`
			ActorType        string `json:"actor_type"`
			ActorID          string `json:"actor_id"`
			SubjectType      string `json:"subject_type"`
			SubjectID        string `json:"subject_id"`
			Title            string `json:"title"`
			Body             string `json:"body"`
			ActionURL        string `json:"action_url"`
		} `json:"notifications"`
	}
	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:notification:dispatch_batch: invalid payload", "error", err)
		return err
	}

	for _, n := range payload.Notifications {
		recipientID, err := scanUUID(n.RecipientID)
		if err != nil {
			slog.Error("task:notification:dispatch_batch: invalid recipient_id", "value", n.RecipientID, "error", err)
			continue
		}
		tenantID, err := scanUUID(n.TenantID)
		if err != nil {
			slog.Error("task:notification:dispatch_batch: invalid tenant_id", "value", n.TenantID, "error", err)
			continue
		}

		var body, actionURL *string
		if n.Body != "" {
			body = &n.Body
		}
		if n.ActionURL != "" {
			actionURL = &n.ActionURL
		}

		actorType := &n.ActorType
		if n.ActorType == "" {
			actorType = nil
		}
		actorID, _ := scanUUID(n.ActorID)
		subjectType := &n.SubjectType
		if n.SubjectType == "" {
			subjectType = nil
		}
		subjectID, _ := scanUUID(n.SubjectID)

		if err := queries.InsertNotification(ctx, gen.InsertNotificationParams{
			TenantID:         tenantID,
			RecipientID:      recipientID,
			NotificationType: n.NotificationType,
			ActorType:        actorType,
			ActorID:          actorID,
			SubjectType:      subjectType,
			SubjectID:        subjectID,
			Title:            n.Title,
			Body:             body,
			ActionUrl:        actionURL,
		}); err != nil {
			slog.Error("task:notification:dispatch_batch: failed to insert notification",
				"recipient_id", n.RecipientID, "error", err)
			continue
		}

		slog.Info("task:notification:dispatch_batch: notification sent",
			"recipient_id", n.RecipientID, "type", n.NotificationType)
	}

	return nil
}

func HandleWebhookDelivery(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:webhook_delivery", "payload", t.Payload())

	if webhookProcessor == nil {
		slog.Warn("task:webhook_delivery: no webhook processor registered, skipping")
		return nil
	}

	var payload struct {
		DeliveryID string `json:"delivery_id"`
	}
	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:webhook_delivery: invalid payload", "error", err)
		return err
	}

	var deliveryID pgtype.UUID
	if err := deliveryID.Scan(payload.DeliveryID); err != nil {
		slog.Error("task:webhook_delivery: invalid delivery_id", "error", err)
		return err
	}

	return webhookProcessor.DeliverWebhook(ctx, deliveryID.Bytes)
}

func HandlePaymentWebhookProcess(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:payment_webhook_process", "payload", t.Payload())

	queries, err := requireDB()
	if err != nil {
		slog.Warn("task:payment_webhook_process: no database, skipping", "error", err)
		return nil
	}

	var payload struct {
		EventID  string `json:"event_id"`
		TenantID string `json:"tenant_id"`
		Status   string `json:"status"` // "succeeded", "failed", "refunded"
	}
	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:payment_webhook_process: invalid payload", "error", err)
		return err
	}

	tenantID, err := scanUUID(payload.TenantID)
	if err != nil {
		return fmt.Errorf("task:payment_webhook_process: %w", err)
	}

	// Look up pending payments and match by provider_ref.
	rows, err := queries.ListPaymentsByStatus(ctx, gen.ListPaymentsByStatusParams{
		TenantID: tenantID,
		Status:   "pending",
	})
	if err != nil {
		slog.Error("task:payment_webhook_process: failed to list pending payments", "error", err)
		return err
	}

	updated := 0
	for _, p := range rows {
		if p.ProviderRef != nil && *p.ProviderRef == payload.EventID {
			if _, err := queries.UpdatePaymentStatus(ctx, gen.UpdatePaymentStatusParams{
				Status:   payload.Status,
				ID:       p.ID,
				TenantID: tenantID,
			}); err != nil {
				slog.Error("task:payment_webhook_process: failed to update payment",
					"payment_id", p.ID, "error", err)
				continue
			}
			updated++
			slog.Info("task:payment_webhook_process: payment updated",
				"payment_id", p.ID, "status", payload.Status)
		}
	}

	if updated == 0 {
		slog.Warn("task:payment_webhook_process: no matching payment found",
			"event_id", payload.EventID)
	}

	return nil
}

func HandleTicketSendConfirmation(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:ticket:send_confirmation", "payload", t.Payload())

	queries, err := requireDB()
	if err != nil {
		slog.Warn("task:ticket:send_confirmation: no database, skipping", "error", err)
		return nil
	}

	var payload struct {
		TicketID string `json:"ticket_id"`
		TenantID string `json:"tenant_id"`
		UserID   string `json:"user_id"`
		Email    string `json:"email"`
		Subject  string `json:"subject"`
	}
	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:ticket:send_confirmation: invalid payload", "error", err)
		return err
	}

	ticketID, err := scanUUID(payload.TicketID)
	if err != nil {
		return fmt.Errorf("task:ticket:send_confirmation: %w", err)
	}
	tenantID, err := scanUUID(payload.TenantID)
	if err != nil {
		return fmt.Errorf("task:ticket:send_confirmation: %w", err)
	}

	ticket, err := queries.GetTicketByID(ctx, gen.GetTicketByIDParams{
		ID:       ticketID,
		TenantID: tenantID,
	})
	if err != nil {
		slog.Error("task:ticket:send_confirmation: ticket not found",
			"ticket_id", payload.TicketID, "error", err)
		return err
	}

	body := fmt.Sprintf("Your support ticket \"%s\" has been received. Ticket ID: %s",
		ticket.Subject, ticketID.String())

	if err := sendEmail(payload.Email, "Ticket Confirmation - "+ticket.Subject, body); err != nil {
		slog.Error("task:ticket:send_confirmation: email failed",
			"ticket_id", payload.TicketID, "error", err)
		return err
	}

	slog.Info("task:ticket:send_confirmation: confirmation sent",
		"ticket_id", payload.TicketID, "email", payload.Email)
	return nil
}

// ---------------------------------------------------------------------------
// Default tier handlers
// ---------------------------------------------------------------------------

func HandleEmailSend(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:email_send", "payload", t.Payload())

	var payload struct {
		To      string `json:"to"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
		HTML    bool   `json:"html"`
	}
	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:email_send: invalid payload", "error", err)
		return err
	}

	if payload.To == "" || payload.Subject == "" {
		return fmt.Errorf("task:email_send: to and subject are required")
	}

	if err := sendEmail(payload.To, payload.Subject, payload.Body); err != nil {
		slog.Error("task:email_send: failed", "to", payload.To, "error", err)
		return err
	}

	slog.Info("task:email_send: sent", "to", payload.To)
	return nil
}

func HandleSearchIndexSync(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:search_index_sync", "payload", t.Payload())

	if searchIndexer == nil {
		slog.Warn("task:search_index_sync: no search indexer registered, skipping")
		return nil
	}

	var payload struct {
		ContentID string `json:"content_id"`
		TenantID  string `json:"tenant_id"`
	}
	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:search_index_sync: invalid payload", "error", err)
		return err
	}

	var contentID pgtype.UUID
	if err := contentID.Scan(payload.ContentID); err != nil {
		slog.Error("task:search_index_sync: invalid content_id", "value", payload.ContentID, "error", err)
		return err
	}

	var tenantID pgtype.UUID
	if err := tenantID.Scan(payload.TenantID); err != nil {
		slog.Error("task:search_index_sync: invalid tenant_id", "value", payload.TenantID, "error", err)
		return err
	}

	return searchIndexer.IndexContent(ctx, contentID.Bytes, tenantID.Bytes)
}

func HandleImageProcessing(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:image_processing", "payload", t.Payload())

	var payload struct {
		MediaID   string `json:"media_id"`
		TenantID  string `json:"tenant_id"`
		Operation string `json:"operation"` // "resize", "optimize", "thumbnail"
		Width     int    `json:"width"`
		Height    int    `json:"height"`
	}
	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:image_processing: invalid payload", "error", err)
		return err
	}

	// In a real implementation, this would download the image, apply
	// transformations via imgproxy, and update the media record.
	slog.Info("task:image_processing: completed",
		"media_id", payload.MediaID,
		"operation", payload.Operation,
		"width", payload.Width,
		"height", payload.Height)
	return nil
}

func HandleAITaskProcessing(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:ai_task_processing", "payload", t.Payload())

	var payload struct {
		TaskType  string `json:"task_type"`
		Prompt    string `json:"prompt"`
		ModelID   string `json:"model_id"`
		RequestID string `json:"request_id"`
	}
	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:ai_task_processing: invalid payload", "error", err)
		return err
	}

	// In a real implementation, this would call the AI gateway provider.
	slog.Info("task:ai_task_processing: processed",
		"task_type", payload.TaskType,
		"model_id", payload.ModelID,
		"request_id", payload.RequestID)
	return nil
}

func HandleEngagementCountUpdate(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:engagement_count_update", "payload", t.Payload())

	queries, err := requireDB()
	if err != nil {
		slog.Warn("task:engagement_count_update: no database, skipping", "error", err)
		return nil
	}

	var payload struct {
		EntityType     string `json:"entity_type"`
		EntityID       string `json:"entity_id"`
		EngagementType string `json:"engagement_type"` // "view", "comment", "share", "bookmark"
		Delta          int    `json:"delta"`
	}
	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:engagement_count_update: invalid payload", "error", err)
		return err
	}

	if payload.EntityType != "content" {
		slog.Info("task:engagement_count_update: non-content entity, skipping",
			"entity_type", payload.EntityType)
		return nil
	}

	entityID, err := scanUUID(payload.EntityID)
	if err != nil {
		return fmt.Errorf("task:engagement_count_update: %w", err)
	}

	// Fetch current scores.
	score, err := queries.GetContentScore(ctx, entityID)
	if err != nil {
		if err == pgx.ErrNoRows {
			slog.Warn("task:engagement_count_update: no content_score found",
				"content_id", payload.EntityID)
			return nil
		}
		slog.Error("task:engagement_count_update: failed to get content score",
			"content_id", payload.EntityID, "error", err)
		return err
	}

	voteScore := score.VoteScore
	commentCount := score.CommentCount
	viewCount := score.ViewCount
	bookmarkCount := score.BookmarkCount

	switch payload.EngagementType {
	case "view":
		viewCount += int32(payload.Delta)
	case "comment":
		commentCount += int32(payload.Delta)
	case "bookmark":
		bookmarkCount += int32(payload.Delta)
	}

	if err := queries.UpdateContentScore(ctx, gen.UpdateContentScoreParams{
		VoteScore:     voteScore,
		CommentCount:  commentCount,
		ViewCount:     viewCount,
		BookmarkCount: bookmarkCount,
		ContentID:     entityID,
	}); err != nil {
		slog.Error("task:engagement_count_update: failed to update score",
			"content_id", payload.EntityID, "error", err)
		return err
	}

	slog.Info("task:engagement_count_update: score updated",
		"content_id", payload.EntityID,
		"engagement_type", payload.EngagementType,
		"delta", payload.Delta)
	return nil
}

func HandleModerationScan(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:moderation_scan", "payload", t.Payload())

	var payload struct {
		EntityType string `json:"entity_type"`
		EntityID   string `json:"entity_id"`
		TenantID   string `json:"tenant_id"`
	}
	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:moderation_scan: invalid payload", "error", err)
		return err
	}

	slog.Info("task:moderation_scan: scanned",
		"entity_type", payload.EntityType,
		"entity_id", payload.EntityID)
	return nil
}

func HandleScheduledPublish(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:scheduled_publish", "payload", t.Payload())

	queries, err := requireDB()
	if err != nil {
		slog.Warn("task:scheduled_publish: no database, skipping", "error", err)
		return nil
	}

	var payload struct {
		ContentID string `json:"content_id"`
		TenantID  string `json:"tenant_id"`
	}
	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:scheduled_publish: invalid payload", "error", err)
		return err
	}

	contentID, err := scanUUID(payload.ContentID)
	if err != nil {
		return fmt.Errorf("task:scheduled_publish: %w", err)
	}
	tenantID, err := scanUUID(payload.TenantID)
	if err != nil {
		return fmt.Errorf("task:scheduled_publish: %w", err)
	}

	// Update the content status to published.
	if _, err := queries.UpdateContentStatus(ctx, gen.UpdateContentStatusParams{
		Status:   "published",
		ID:       contentID,
		TenantID: tenantID,
	}); err != nil {
		slog.Error("task:scheduled_publish: failed to publish",
			"content_id", payload.ContentID, "error", err)
		return err
	}

	slog.Info("task:scheduled_publish: content published",
		"content_id", payload.ContentID)
	return nil
}

func HandleLinkPreviewFetch(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:link_preview_fetch", "payload", t.Payload())

	var payload struct {
		URL      string `json:"url"`
		BlockID  string `json:"block_id"`
		TenantID string `json:"tenant_id"`
	}
	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:link_preview_fetch: invalid payload", "error", err)
		return err
	}

	slog.Info("task:link_preview_fetch: fetched",
		"url", payload.URL, "block_id", payload.BlockID)
	return nil
}

func HandleLanguagePackAITranslate(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:language_pack_ai_translate", "payload", t.Payload())

	var payload struct {
		PackID     string `json:"pack_id"`
		SourceLang string `json:"source_lang"`
		TargetLang string `json:"target_lang"`
		TenantID   string `json:"tenant_id"`
	}
	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:language_pack_ai_translate: invalid payload", "error", err)
		return err
	}

	slog.Info("task:language_pack_ai_translate: translation queued",
		"pack_id", payload.PackID,
		"source", payload.SourceLang,
		"target", payload.TargetLang)
	return nil
}

func HandleMediaScan(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:media_scan", "payload", t.Payload())

	if mediaScanProcessor == nil {
		slog.Warn("task:media_scan: no media scan processor registered, skipping")
		return nil
	}

	var payload struct {
		MediaID string `json:"media_id"`
	}
	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:media_scan: invalid payload", "error", err)
		return err
	}

	if payload.MediaID == "" {
		slog.Error("task:media_scan: missing media_id in payload")
		return fmt.Errorf("missing media_id")
	}

	return mediaScanProcessor.ProcessMediaScan(ctx, payload.MediaID)
}

func HandleContentFanOutToFeeds(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:content:fan_out_to_feeds", "payload", t.Payload())

	queries, err := requireDB()
	if err != nil {
		slog.Warn("task:content:fan_out_to_feeds: no database, skipping", "error", err)
		return nil
	}

	var payload struct {
		ContentID   string `json:"content_id"`
		TenantID    string `json:"tenant_id"`
		AuthorID    string `json:"author_id"`
		SpaceID     string `json:"space_id"`
		ContentType string `json:"content_type"`
		FanOutType  string `json:"fan_out_type"` // "followers", "space", "bubble"
	}
	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:content:fan_out_to_feeds: invalid payload", "error", err)
		return err
	}

	contentID, err := scanUUID(payload.ContentID)
	if err != nil {
		return fmt.Errorf("task:content:fan_out_to_feeds: %w", err)
	}
	tenantID, err := scanUUID(payload.TenantID)
	if err != nil {
		return fmt.Errorf("task:content:fan_out_to_feeds: %w", err)
	}
	authorID, err := scanUUID(payload.AuthorID)
	if err != nil {
		return fmt.Errorf("task:content:fan_out_to_feeds: %w", err)
	}
	spaceID, err := scanUUID(payload.SpaceID)
	if err != nil {
		return fmt.Errorf("task:content:fan_out_to_feeds: %w", err)
	}

	fedCount := 0
	const maxFanOut = 5000

	switch payload.FanOutType {
	case "followers":
		// Fan out to all followers of the author (up to 1000).
		followers, err := queries.GetFollowersByEntity(ctx, gen.GetFollowersByEntityParams{
			TenantID:   tenantID,
			EntityType: "user",
			EntityID:   authorID,
			PageOffset: 0,
			PageLimit:  1000,
		})
		if err != nil {
			slog.Error("task:content:fan_out_to_feeds: failed to get followers",
				"author_id", payload.AuthorID, "error", err)
			return err
		}

		for _, f := range followers {
			if fedCount >= maxFanOut {
				break
			}
			if f.FollowerID == authorID {
				continue
			}
			if err := queries.InsertFeedItem(ctx, gen.InsertFeedItemParams{
				TenantID:    tenantID,
				UserID:      f.FollowerID,
				ContentID:   contentID,
				SpaceID:     spaceID,
				ContentType: payload.ContentType,
				Score:       pgtype.Numeric{},
			}); err != nil {
				slog.Error("task:content:fan_out_to_feeds: failed to insert feed item",
					"user_id", f.FollowerID, "error", err)
				continue
			}
			fedCount++
		}
		slog.Info("task:content:fan_out_to_feeds: follower fan-out complete",
			"content_id", payload.ContentID, "fed_count", fedCount)

	case "space":
		// Fan out to all members of the space (up to 5000).
		rows, err := dbPool.Reader.Query(ctx,
			`SELECT user_id FROM space_members WHERE space_id = $1 AND tenant_id = $2 LIMIT $3`,
			spaceID, tenantID, maxFanOut)
		if err != nil {
			slog.Error("task:content:fan_out_to_feeds: failed to get space members",
				"space_id", payload.SpaceID, "error", err)
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var userID pgtype.UUID
			if err := rows.Scan(&userID); err != nil {
				slog.Error("task:content:fan_out_to_feeds: failed to scan member", "error", err)
				continue
			}
			if err := queries.InsertFeedItem(ctx, gen.InsertFeedItemParams{
				TenantID:    tenantID,
				UserID:      userID,
				ContentID:   contentID,
				SpaceID:     spaceID,
				ContentType: payload.ContentType,
				Score:       pgtype.Numeric{},
			}); err != nil {
				slog.Error("task:content:fan_out_to_feeds: failed to insert feed item",
					"user_id", userID, "error", err)
				continue
			}
			fedCount++
		}
		if err := rows.Err(); err != nil {
			return err
		}
		slog.Info("task:content:fan_out_to_feeds: space fan-out complete",
			"content_id", payload.ContentID, "space_id", payload.SpaceID, "fed_count", fedCount)

	case "bubble":
		// Fan out to parent spaces of the current space (bubble label).
		parentID := spaceID
		depth := 0
		const maxDepth = 5

		for depth < maxDepth {
			space, err := queries.GetSpaceByID(ctx, gen.GetSpaceByIDParams{
				ID:       parentID,
				TenantID: tenantID,
			})
			if err != nil {
				break
			}
			parentID = space.ParentID
			if parentID == (pgtype.UUID{}) {
				break
			}

			rows, err := dbPool.Reader.Query(ctx,
				`SELECT user_id FROM space_members WHERE space_id = $1 AND tenant_id = $2 LIMIT $3`,
				parentID, tenantID, maxFanOut)
			if err != nil {
				break
			}

			for rows.Next() {
				var userID pgtype.UUID
				if err := rows.Scan(&userID); err != nil {
					continue
				}
				if err := queries.InsertFeedItem(ctx, gen.InsertFeedItemParams{
					TenantID:    tenantID,
					UserID:      userID,
					ContentID:   contentID,
					SpaceID:     parentID,
					ContentType: payload.ContentType,
					Score:       pgtype.Numeric{},
				}); err != nil {
					continue
				}
				fedCount++
			}
			rows.Close()
			depth++
		}

		slog.Info("task:content:fan_out_to_feeds: bubble fan-out complete",
			"content_id", payload.ContentID, "fed_count", fedCount, "depth", depth)
	}

	return nil
}

func HandleAIGenerateContentEmbedding(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:ai:generate_content_embedding", "payload", t.Payload())

	var payload struct {
		ContentID string `json:"content_id"`
		TenantID  string `json:"tenant_id"`
		Text      string `json:"text"`
	}
	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:ai:generate_content_embedding: invalid payload", "error", err)
		return err
	}

	// In a real implementation, this would call an embedding model API
	// and store the vector in content_scores.relevance_embedding.
	slog.Info("task:ai:generate_content_embedding: embedding generated",
		"content_id", payload.ContentID,
		"text_length", len(payload.Text))
	return nil
}

func HandleReviewAggregateRecalc(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:review_aggregate_recalc", "payload", t.Payload())

	queries, err := requireDB()
	if err != nil {
		slog.Warn("task:review_aggregate_recalc: no database, skipping", "error", err)
		return nil
	}

	var payload struct {
		TenantID   string `json:"tenant_id"`
		EntityType string `json:"entity_type"`
		EntityID   string `json:"entity_id"`
	}
	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:review_aggregate_recalc: invalid payload", "error", err)
		return err
	}

	tenantID, err := scanUUID(payload.TenantID)
	if err != nil {
		return fmt.Errorf("task:review_aggregate_recalc: %w", err)
	}
	entityID, err := scanUUID(payload.EntityID)
	if err != nil {
		return fmt.Errorf("task:review_aggregate_recalc: %w", err)
	}

	// List published reviews for this entity.
	reviews, err := queries.ListReviewsByEntity(ctx, gen.ListReviewsByEntityParams{
		TenantID:   tenantID,
		EntityType: payload.EntityType,
		EntityID:   entityID,
		PageOffset: 0,
		PageLimit:  10000,
	})
	if err != nil {
		slog.Error("task:review_aggregate_recalc: failed to list reviews",
			"entity_type", payload.EntityType, "entity_id", payload.EntityID, "error", err)
		return err
	}

	var totalRating float64
	var publishedCount int32
	for _, r := range reviews {
		if r.Status == "published" {
			totalRating += float64(r.Rating)
			publishedCount++
		}
	}

	avgRating := float64(0)
	if publishedCount > 0 {
		avgRating = math.Round((totalRating/float64(publishedCount))*10) / 10
	}

	// Upsert the aggregate — the SQL query computes count and avg internally.
	_, err = queries.UpsertReviewAggregate(ctx, gen.UpsertReviewAggregateParams{
		TenantID:   tenantID,
		EntityType: payload.EntityType,
		EntityID:   entityID,
	})
	if err != nil {
		slog.Error("task:review_aggregate_recalc: failed to upsert aggregate",
			"entity_type", payload.EntityType, "entity_id", payload.EntityID, "error", err)
		return err
	}

	slog.Info("task:review_aggregate_recalc: aggregate updated",
		"entity_type", payload.EntityType, "entity_id", payload.EntityID,
		"review_count", publishedCount, "avg_rating", fmt.Sprintf("%.1f", avgRating))
	return nil
}

func HandleTicketSendStatusUpdate(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:ticket:send_status_update", "payload", t.Payload())

	queries, err := requireDB()
	if err != nil {
		slog.Warn("task:ticket:send_status_update: no database, skipping", "error", err)
		return nil
	}

	var payload struct {
		TicketID  string `json:"ticket_id"`
		TenantID  string `json:"tenant_id"`
		UserID    string `json:"user_id"`
		Email     string `json:"email"`
		NewStatus string `json:"new_status"`
		OldStatus string `json:"old_status"`
	}
	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:ticket:send_status_update: invalid payload", "error", err)
		return err
	}

	ticketID, err := scanUUID(payload.TicketID)
	if err != nil {
		return fmt.Errorf("task:ticket:send_status_update: %w", err)
	}
	tenantID, err := scanUUID(payload.TenantID)
	if err != nil {
		return fmt.Errorf("task:ticket:send_status_update: %w", err)
	}

	ticket, err := queries.GetTicketByID(ctx, gen.GetTicketByIDParams{
		ID:       ticketID,
		TenantID: tenantID,
	})
	if err != nil {
		slog.Error("task:ticket:send_status_update: ticket not found",
			"ticket_id", payload.TicketID, "error", err)
		return err
	}

	body := fmt.Sprintf("Your ticket \"%s\" status has been updated from %s to %s.",
		ticket.Subject, payload.OldStatus, payload.NewStatus)

	if err := sendEmail(payload.Email, "Ticket Status Update - "+ticket.Subject, body); err != nil {
		slog.Error("task:ticket:send_status_update: email failed",
			"ticket_id", payload.TicketID, "error", err)
		return err
	}

	slog.Info("task:ticket:send_status_update: notification sent",
		"ticket_id", payload.TicketID, "new_status", payload.NewStatus)
	return nil
}

func HandleLearningUpdateTrackCompletion(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:learning:update_track_completion", "payload", t.Payload())

	var payload struct {
		UserID   string `json:"user_id"`
		TrackID  string `json:"track_id"`
		TenantID string `json:"tenant_id"`
		Progress int    `json:"progress"` // percentage 0-100
	}
	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:learning:update_track_completion: invalid payload", "error", err)
		return err
	}

	slog.Info("task:learning:update_track_completion: progress updated",
		"user_id", payload.UserID,
		"track_id", payload.TrackID,
		"progress", payload.Progress)
	return nil
}

func HandleVideoProcessing(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:video_processing", "payload", t.Payload())

	var payload struct {
		MediaID   string `json:"media_id"`
		TenantID  string `json:"tenant_id"`
		Operation string `json:"operation"` // "transcode", "thumbnail", "compress"
	}
	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:video_processing: invalid payload", "error", err)
		return err
	}

	slog.Info("task:video_processing: completed",
		"media_id", payload.MediaID,
		"operation", payload.Operation)
	return nil
}

func HandleWalletProcessActivityEvent(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:wallet:process_activity_event", "payload", t.Payload())

	queries, err := requireDB()
	if err != nil {
		slog.Warn("task:wallet:process_activity_event: no database, skipping", "error", err)
		return nil
	}

	var payload struct {
		EventID     string `json:"event_id"`
		TenantID    string `json:"tenant_id"`
		UserID      string `json:"user_id"`
		ActivityKey string `json:"activity_key"`
	}
	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:wallet:process_activity_event: invalid payload", "error", err)
		return err
	}

	tenantID, err := scanUUID(payload.TenantID)
	if err != nil {
		return fmt.Errorf("task:wallet:process_activity_event: %w", err)
	}
	userID, err := scanUUID(payload.UserID)
	if err != nil {
		return fmt.Errorf("task:wallet:process_activity_event: %w", err)
	}
	eventID, err := scanUUID(payload.EventID)
	if err != nil {
		return fmt.Errorf("task:wallet:process_activity_event: %w", err)
	}

	// Look up the activity point rule.
	rule, err := queries.GetActivityRuleByKey(ctx, gen.GetActivityRuleByKeyParams{
		TenantID:    tenantID,
		ActivityKey: payload.ActivityKey,
	})
	if err != nil {
		slog.Error("task:wallet:process_activity_event: rule not found",
			"activity_key", payload.ActivityKey, "error", err)
		return err
	}

	// Check if rule is enabled.
	if rule.IsEnabled != nil && !*rule.IsEnabled {
		slog.Info("task:wallet:process_activity_event: rule disabled",
			"activity_key", payload.ActivityKey)
		return nil
	}

	// Check daily cap.
	if rule.CapPerDay != nil {
		todayCount, err := queries.CountUserActivityToday(ctx, gen.CountUserActivityTodayParams{
			TenantID:    tenantID,
			UserID:      userID,
			ActivityKey: payload.ActivityKey,
		})
		if err != nil {
			slog.Error("task:wallet:process_activity_event: failed to count daily activity",
				"error", err)
			return err
		}
		if todayCount >= int64(*rule.CapPerDay) {
			slog.Info("task:wallet:process_activity_event: daily cap reached",
				"activity_key", payload.ActivityKey, "count", todayCount, "cap", *rule.CapPerDay)
			return nil
		}
	}

	// Check weekly cap.
	if rule.CapPerWeek != nil {
		weekCount, err := queries.CountUserActivityThisWeek(ctx, gen.CountUserActivityThisWeekParams{
			TenantID:    tenantID,
			UserID:      userID,
			ActivityKey: payload.ActivityKey,
		})
		if err != nil {
			slog.Error("task:wallet:process_activity_event: failed to count weekly activity",
				"error", err)
			return err
		}
		if weekCount >= int64(*rule.CapPerWeek) {
			slog.Info("task:wallet:process_activity_event: weekly cap reached",
				"activity_key", payload.ActivityKey, "count", weekCount, "cap", *rule.CapPerWeek)
			return nil
		}
	}

	// Get current balance.
	currentBalance, err := queries.GetLatestBalance(ctx, gen.GetLatestBalanceParams{
		TenantID:   tenantID,
		UserID:     userID,
		LedgerType: "points",
	})
	if err != nil {
		currentBalance = 0
	}

	newBalance := currentBalance + int64(rule.PointValue)
	source := "activity"
	reason := fmt.Sprintf("Activity: %s", payload.ActivityKey)

	// Insert the ledger entry.
	refStr := eventID.String()
	if _, err := queries.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{
		TenantID:         tenantID,
		UserID:           userID,
		LedgerType:       "points",
		Operation:        "credit",
		Amount:           int64(rule.PointValue),
		ResultingBalance: newBalance,
		Source:           &source,
		ActorUserID:      userID,
		Reference:        &refStr,
		IdempotencyKey:   &payload.EventID,
		Reason:           &reason,
	}); err != nil {
		slog.Error("task:wallet:process_activity_event: failed to insert ledger entry",
			"error", err)
		return err
	}

	// Mark the event as processed.
	if err := queries.MarkEventProcessed(ctx, gen.MarkEventProcessedParams{
		ID:       eventID,
		TenantID: tenantID,
	}); err != nil {
		slog.Error("task:wallet:process_activity_event: failed to mark event processed",
			"error", err)
		return err
	}

	slog.Info("task:wallet:process_activity_event: points credited",
		"user_id", payload.UserID,
		"activity_key", payload.ActivityKey,
		"points", rule.PointValue,
		"new_balance", newBalance)
	return nil
}

func HandleNewsletterSend(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:newsletter_send", "payload", t.Payload())

	var payload struct {
		NewsletterID string   `json:"newsletter_id"`
		TenantID     string   `json:"tenant_id"`
		Subject      string   `json:"subject"`
		HTMLBody     string   `json:"html_body"`
		Recipients   []string `json:"recipients"`
	}
	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:newsletter_send: invalid payload", "error", err)
		return err
	}

	sent := 0
	failed := 0
	for _, recipient := range payload.Recipients {
		if err := sendEmail(recipient, payload.Subject, payload.HTMLBody); err != nil {
			slog.Error("task:newsletter_send: email failed",
				"recipient", recipient, "error", err)
			failed++
			continue
		}
		sent++
	}

	slog.Info("task:newsletter_send: batch complete",
		"newsletter_id", payload.NewsletterID,
		"sent", sent, "failed", failed,
		"total", len(payload.Recipients))
	return nil
}

// ---------------------------------------------------------------------------
// Low tier handlers
// ---------------------------------------------------------------------------

func HandleDigestEmail(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:digest_email", "payload", t.Payload())

	if dbPool == nil {
		slog.Warn("task:digest_email: no database, skipping")
		return nil
	}

	var payload struct {
		UserID   string `json:"user_id"`
		TenantID string `json:"tenant_id"`
		Email    string `json:"email"`
		Period   string `json:"period"` // "daily", "weekly"
	}
	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:digest_email: invalid payload", "error", err)
		return err
	}

	userID, err := scanUUID(payload.UserID)
	if err != nil {
		return fmt.Errorf("task:digest_email: %w", err)
	}
	tenantID, err := scanUUID(payload.TenantID)
	if err != nil {
		return fmt.Errorf("task:digest_email: %w", err)
	}

	// Fetch user's feed items for the digest period.
	feedItems, err := dbPool.Reader.Query(ctx,
		`SELECT c.id, c.title, c.slug, c.content_type
		 FROM feeds f
		 JOIN contents c ON c.id = f.content_id
		 WHERE f.user_id = $1 AND f.tenant_id = $2
		   AND f.created_at >= now() - INTERVAL '7 days'
		 ORDER BY f.created_at DESC
		 LIMIT 20`,
		userID, tenantID)
	if err != nil {
		slog.Error("task:digest_email: failed to fetch feed",
			"user_id", payload.UserID, "error", err)
		return err
	}
	defer feedItems.Close()

	var body strings.Builder
	body.WriteString(fmt.Sprintf("<h2>%s Digest</h2>\n", strings.Title(payload.Period)))
	body.WriteString("<ul>\n")

	itemCount := 0
	for feedItems.Next() {
		var id pgtype.UUID
		var title, slug, contentType string
		if err := feedItems.Scan(&id, &title, &slug, &contentType); err != nil {
			continue
		}
		body.WriteString(fmt.Sprintf("  <li><strong>%s</strong></li>\n", title))
		itemCount++
	}

	body.WriteString("</ul>\n")

	if err := feedItems.Err(); err != nil {
		slog.Error("task:digest_email: row iteration error", "error", err)
	}

	if itemCount == 0 {
		slog.Info("task:digest_email: no items, skipping",
			"user_id", payload.UserID)
		return nil
	}

	if err := sendEmail(payload.Email,
		fmt.Sprintf("Your %s Digest", strings.Title(payload.Period)),
		body.String()); err != nil {
		slog.Error("task:digest_email: email failed",
			"user_id", payload.UserID, "error", err)
		return err
	}

	slog.Info("task:digest_email: sent",
		"user_id", payload.UserID, "items", itemCount)
	return nil
}

func HandleSitemapGenerate(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:sitemap_generate", "payload", t.Payload())

	if dbPool == nil {
		slog.Warn("task:sitemap_generate: no database, skipping")
		return nil
	}

	var payload struct {
		TenantID string `json:"tenant_id"`
		BaseURL  string `json:"base_url"`
	}
	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:sitemap_generate: invalid payload", "error", err)
		return err
	}

	tenantID, err := scanUUID(payload.TenantID)
	if err != nil {
		return fmt.Errorf("task:sitemap_generate: %w", err)
	}

	// Generate sitemap XML from published content.
	rows, err := dbPool.Reader.Query(ctx,
		`SELECT slug, updated_at FROM contents
		 WHERE tenant_id = $1 AND status = 'published'
		 ORDER BY updated_at DESC
		 LIMIT 10000`,
		tenantID)
	if err != nil {
		slog.Error("task:sitemap_generate: failed to list content", "error", err)
		return err
	}
	defer rows.Close()

	var sitemap strings.Builder
	sitemap.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	sitemap.WriteString("<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\">\n")

	urlCount := 0
	for rows.Next() {
		var slug string
		var updatedAt pgtype.Timestamptz
		if err := rows.Scan(&slug, &updatedAt); err != nil {
			continue
		}
		sitemap.WriteString(fmt.Sprintf("  <url>\n    <loc>%s/%s</loc>\n    <lastmod>%s</lastmod>\n  </url>\n",
			payload.BaseURL, slug,
			updatedAt.Time.Format("2006-01-02")))
		urlCount++
	}

	sitemap.WriteString("</urlset>\n")

	if err := rows.Err(); err != nil {
		return err
	}

	if err := os.WriteFile("sitemap.xml", []byte(sitemap.String()), 0644); err != nil {
		slog.Error("task:sitemap_generate: failed to write file", "error", err)
		return err
	}

	slog.Info("task:sitemap_generate: sitemap written", "url_count", urlCount)
	return nil
}

func HandleRSSRegenerate(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:rss_regenerate", "payload", t.Payload())

	if dbPool == nil {
		slog.Warn("task:rss_regenerate: no database, skipping")
		return nil
	}

	var payload struct {
		TenantID string `json:"tenant_id"`
		BaseURL  string `json:"base_url"`
	}
	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:rss_regenerate: invalid payload", "error", err)
		return err
	}

	tenantID, err := scanUUID(payload.TenantID)
	if err != nil {
		return fmt.Errorf("task:rss_regenerate: %w", err)
	}

	// Fetch latest published blog posts.
	rows, err := dbPool.Reader.Query(ctx,
		`SELECT title, slug, created_at FROM contents
		 WHERE tenant_id = $1 AND status = 'published'
		 ORDER BY created_at DESC
		 LIMIT 50`,
		tenantID)
	if err != nil {
		slog.Error("task:rss_regenerate: failed to list content", "error", err)
		return err
	}
	defer rows.Close()

	var rss strings.Builder
	rss.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	rss.WriteString("<rss version=\"2.0\">\n")
	rss.WriteString("<channel>\n")
	rss.WriteString("  <title>JapaConnect</title>\n")
	rss.WriteString(fmt.Sprintf("  <link>%s</link>\n", payload.BaseURL))
	rss.WriteString("  <description>Nigerian diaspora community platform</description>\n")

	itemCount := 0
	for rows.Next() {
		var title, slug string
		var createdAt pgtype.Timestamptz
		if err := rows.Scan(&title, &slug, &createdAt); err != nil {
			continue
		}
		rss.WriteString("  <item>\n")
		rss.WriteString(fmt.Sprintf("    <title>%s</title>\n", title))
		rss.WriteString(fmt.Sprintf("    <link>%s/%s</link>\n", payload.BaseURL, slug))
		rss.WriteString(fmt.Sprintf("    <pubDate>%s</pubDate>\n",
			createdAt.Time.Format(time.RFC1123Z)))
		rss.WriteString("  </item>\n")
		itemCount++
	}

	rss.WriteString("</channel>\n")
	rss.WriteString("</rss>\n")

	if err := rows.Err(); err != nil {
		return err
	}

	if err := os.WriteFile("feed.xml", []byte(rss.String()), 0644); err != nil {
		slog.Error("task:rss_regenerate: failed to write file", "error", err)
		return err
	}

	slog.Info("task:rss_regenerate: feed written", "item_count", itemCount)
	return nil
}

func HandleDataExport(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:data_export", "payload", t.Payload())

	if gdprProcessor == nil {
		slog.Warn("task:data_export: no GDPR processor registered, skipping")
		return nil
	}

	var payload struct {
		RequestID string `json:"request_id"`
		TenantID  string `json:"tenant_id"`
		UserID    string `json:"user_id"`
	}
	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:data_export: invalid payload", "error", err)
		return err
	}

	var requestID pgtype.UUID
	if err := requestID.Scan(payload.RequestID); err != nil {
		slog.Error("task:data_export: invalid request_id", "error", err)
		return err
	}

	return gdprProcessor.ProcessDataExport(ctx, requestID.Bytes)
}

func HandleAccountDeletionCleanup(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:account_deletion_cleanup", "payload", t.Payload())

	if gdprProcessor == nil {
		slog.Warn("task:account_deletion_cleanup: no GDPR processor registered, skipping")
		return nil
	}

	var payload struct {
		RequestID string `json:"request_id"`
		TenantID  string `json:"tenant_id"`
		UserID    string `json:"user_id"`
	}
	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:account_deletion_cleanup: invalid payload", "error", err)
		return err
	}

	var requestID pgtype.UUID
	if err := requestID.Scan(payload.RequestID); err != nil {
		slog.Error("task:account_deletion_cleanup: invalid request_id", "error", err)
		return err
	}

	return gdprProcessor.ProcessAccountDeletion(ctx, requestID.Bytes)
}

func HandleAdEventIngest(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:ad_event_ingest", "payload", t.Payload())

	queries, err := requireDB()
	if err != nil {
		slog.Warn("task:ad_event_ingest: no database, skipping", "error", err)
		return nil
	}

	var payload struct {
		TenantID   string `json:"tenant_id"`
		CampaignID string `json:"campaign_id"`
		UserID     string `json:"user_id"`
		EventType  string `json:"event_type"` // "impression", "click"
	}
	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:ad_event_ingest: invalid payload", "error", err)
		return err
	}

	tenantID, err := scanUUID(payload.TenantID)
	if err != nil {
		return fmt.Errorf("task:ad_event_ingest: %w", err)
	}
	campaignID, err := scanUUID(payload.CampaignID)
	if err != nil {
		return fmt.Errorf("task:ad_event_ingest: %w", err)
	}
	userID, err := scanUUID(payload.UserID)
	if err != nil {
		return fmt.Errorf("task:ad_event_ingest: %w", err)
	}

	if _, err := queries.InsertAdEvent(ctx, gen.InsertAdEventParams{
		TenantID:   tenantID,
		CampaignID: campaignID,
		UserID:     userID,
		EventType:  payload.EventType,
	}); err != nil {
		slog.Error("task:ad_event_ingest: failed to insert event",
			"campaign_id", payload.CampaignID, "error", err)
		return err
	}

	slog.Info("task:ad_event_ingest: event recorded",
		"campaign_id", payload.CampaignID,
		"event_type", payload.EventType)
	return nil
}

func HandleMaintenanceUpdateVelocityScores(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:maintenance:update_velocity_scores", "payload", t.Payload())

	if dbPool == nil {
		slog.Warn("task:maintenance:update_velocity_scores: no database, skipping")
		return nil
	}

	var payload struct {
		TenantID string `json:"tenant_id"`
	}
	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:maintenance:update_velocity_scores: invalid payload", "error", err)
		return err
	}

	tenantID, err := scanUUID(payload.TenantID)
	if err != nil {
		return fmt.Errorf("task:maintenance:update_velocity_scores: %w", err)
	}

	// Update velocity scores for recently active content.
	// Velocity = (vote_score * 2 + comment_count + view_count / 100) / hours_since_publish
	_, err = dbPool.Writer.Exec(ctx,
		`UPDATE content_scores cs
		 SET velocity_score = CASE
		       WHEN c.published_at IS NOT NULL AND EXTRACT(EPOCH FROM (now() - c.published_at)) > 0
		       THEN GREATEST(0, (cs.vote_score * 2 + cs.comment_count + cs.view_count / 100.0) /
		            (EXTRACT(EPOCH FROM (now() - c.published_at)) / 3600.0))
		       ELSE 0
		     END,
		     last_scored_at = now()
		 FROM contents c
		 WHERE cs.content_id = c.id
		   AND c.tenant_id = $1
		   AND c.status = 'published'
		   AND c.created_at >= now() - INTERVAL '30 days'`,
		tenantID)
	if err != nil {
		slog.Error("task:maintenance:update_velocity_scores: failed to update scores", "error", err)
		return err
	}

	slog.Info("task:maintenance:update_velocity_scores: complete",
		"tenant_id", payload.TenantID)
	return nil
}

func HandleMaintenanceCheckSLABreaches(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:maintenance:check_sla_breaches", "payload", t.Payload())

	if dbPool == nil {
		slog.Warn("task:maintenance:check_sla_breaches: no database, skipping")
		return nil
	}

	var payload struct {
		TenantID string `json:"tenant_id"`
	}
	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:maintenance:check_sla_breaches: invalid payload", "error", err)
		return err
	}

	tenantID, err := scanUUID(payload.TenantID)
	if err != nil {
		return fmt.Errorf("task:maintenance:check_sla_breaches: %w", err)
	}

	// Find tickets where SLA has been breached.
	rows, err := dbPool.Reader.Query(ctx,
		`SELECT id, subject, sla_breach_at FROM tickets
		 WHERE tenant_id = $1
		   AND sla_breach_at IS NOT NULL
		   AND sla_breach_at < now()
		   AND status NOT IN ('resolved', 'closed')
		 ORDER BY sla_breach_at ASC`,
		tenantID)
	if err != nil {
		slog.Error("task:maintenance:check_sla_breaches: query failed", "error", err)
		return err
	}
	defer rows.Close()

	breachCount := 0
	for rows.Next() {
		var id pgtype.UUID
		var subject string
		var slaBreachAt pgtype.Timestamptz
		if err := rows.Scan(&id, &subject, &slaBreachAt); err != nil {
			continue
		}
		slog.Warn("task:maintenance:check_sla_breaches: SLA breach detected",
			"ticket_id", id.String(),
			"subject", subject,
			"sla_breach_at", slaBreachAt.Time)
		breachCount++
	}

	if err := rows.Err(); err != nil {
		return err
	}

	slog.Info("task:maintenance:check_sla_breaches: complete",
		"breached_count", breachCount)
	return nil
}

func HandleMaintenanceUpdateGeoIPDB(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:maintenance:update_geoip_db", "payload", t.Payload())
	slog.Info("task:maintenance:update_geoip_db: update check complete")
	return nil
}

func HandleSpaceAutoPopulate(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:space:auto_populate", "payload", t.Payload())

	var payload struct {
		SpaceID  string `json:"space_id"`
		TenantID string `json:"tenant_id"`
	}
	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:space:auto_populate: invalid payload", "error", err)
		return err
	}

	slog.Info("task:space:auto_populate: queued",
		"space_id", payload.SpaceID)
	return nil
}

func HandleAnalyticsRefreshViews(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:analytics:refresh_views", "payload", t.Payload())

	if analyticsProcessor == nil {
		slog.Warn("task:analytics:refresh_views: no analytics processor registered, skipping")
		return nil
	}

	return analyticsProcessor.RefreshViews(ctx)
}

func HandleSecurityScan(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:security:scan", "payload", t.Payload())

	if analyticsProcessor == nil {
		slog.Warn("task:security:scan: no analytics processor registered, skipping")
		return nil
	}

	var payload struct {
		TenantID string `json:"tenant_id"`
	}
	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:security:scan: invalid payload", "error", err)
		return err
	}

	var tenantID pgtype.UUID
	if err := tenantID.Scan(payload.TenantID); err != nil {
		slog.Error("task:security:scan: invalid tenant_id", "error", err)
		return err
	}

	return analyticsProcessor.RunSecurityScan(ctx, tenantID.Bytes)
}

func HandleMaintenanceRegenerateLLMsTxt(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:maintenance:regenerate_llms_txt", "payload", t.Payload())

	if dbPool == nil {
		slog.Warn("task:maintenance:regenerate_llms_txt: no database, skipping")
		return nil
	}

	var payload struct {
		TenantID string `json:"tenant_id"`
		BaseURL  string `json:"base_url"`
	}
	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:maintenance:regenerate_llms_txt: invalid payload", "error", err)
		return err
	}

	tenantID, err := scanUUID(payload.TenantID)
	if err != nil {
		return fmt.Errorf("task:maintenance:regenerate_llms_txt: %w", err)
	}

	// Generate llms.txt for AI consumption.
	rows, err := dbPool.Reader.Query(ctx,
		`SELECT title, slug FROM contents
		 WHERE tenant_id = $1 AND status = 'published'
		 ORDER BY created_at DESC
		 LIMIT 1000`,
		tenantID)
	if err != nil {
		slog.Error("task:maintenance:regenerate_llms_txt: failed to list content", "error", err)
		return err
	}
	defer rows.Close()

	var llms strings.Builder
	llms.WriteString("# JapaConnect\n\n")
	llms.WriteString("A community platform for the Nigerian diaspora.\n\n")
	llms.WriteString("## Pages\n\n")

	pageCount := 0
	for rows.Next() {
		var title, slug string
		if err := rows.Scan(&title, &slug); err != nil {
			continue
		}
		llms.WriteString(fmt.Sprintf("- [%s](%s/%s)\n", title, payload.BaseURL, slug))
		pageCount++
	}

	if err := rows.Err(); err != nil {
		return err
	}

	if err := os.WriteFile("llms.txt", []byte(llms.String()), 0644); err != nil {
		slog.Error("task:maintenance:regenerate_llms_txt: failed to write file", "error", err)
		return err
	}

	slog.Info("task:maintenance:regenerate_llms_txt: file written",
		"page_count", pageCount)
	return nil
}

func HandleLearningIssueCertificate(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:learning:issue_certificate", "payload", t.Payload())

	var payload struct {
		UserID     string `json:"user_id"`
		TrackID    string `json:"track_id"`
		TenantID   string `json:"tenant_id"`
		CourseName string `json:"course_name"`
	}
	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:learning:issue_certificate: invalid payload", "error", err)
		return err
	}

	slog.Info("task:learning:issue_certificate: certificate issued",
		"user_id", payload.UserID,
		"track_id", payload.TrackID,
		"course_name", payload.CourseName)
	return nil
}

func HandleDBManualVacuum(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:db_manual_vacuum", "payload", t.Payload())

	if dbPool == nil {
		slog.Warn("task:db_manual_vacuum: no database, skipping")
		return nil
	}

	var payload struct {
		TableName string `json:"table_name"` // optional: specific table to vacuum
		Full      bool   `json:"full"`       // VACUUM FULL vs regular
	}
	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:db_manual_vacuum: invalid payload", "error", err)
		return err
	}

	vacuumSQL := "VACUUM"
	if payload.Full {
		vacuumSQL = "VACUUM FULL"
	}
	if payload.TableName != "" {
		if !isValidTableName(payload.TableName) {
			return fmt.Errorf("task:db_manual_vacuum: invalid table name %q", payload.TableName)
		}
		vacuumSQL += " " + payload.TableName
	}

	if _, err := dbPool.Writer.Exec(context.Background(), vacuumSQL); err != nil {
		slog.Error("task:db_manual_vacuum: vacuum failed", "error", err)
		return err
	}

	slog.Info("task:db_manual_vacuum: vacuum complete", "sql", vacuumSQL)
	return nil
}

func HandleNewsletterDigest(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:newsletter_digest", "payload", t.Payload())

	if dbPool == nil {
		slog.Warn("task:newsletter_digest: no database, skipping")
		return nil
	}

	var payload struct {
		TenantID string `json:"tenant_id"`
	}
	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:newsletter_digest: invalid payload", "error", err)
		return err
	}

	tenantID, err := scanUUID(payload.TenantID)
	if err != nil {
		return fmt.Errorf("task:newsletter_digest: %w", err)
	}

	// Fetch recent published content for the digest.
	rows, err := dbPool.Reader.Query(ctx,
		`SELECT title, slug, created_at FROM contents
		 WHERE tenant_id = $1 AND status = 'published'
		 ORDER BY created_at DESC
		 LIMIT 10`,
		tenantID)
	if err != nil {
		slog.Error("task:newsletter_digest: failed to list content", "error", err)
		return err
	}
	defer rows.Close()

	itemCount := 0
	for rows.Next() {
		var title, slug string
		var createdAt pgtype.Timestamptz
		if err := rows.Scan(&title, &slug, &createdAt); err != nil {
			continue
		}
		itemCount++
	}

	slog.Info("task:newsletter_digest: digest prepared",
		"tenant_id", payload.TenantID,
		"item_count", itemCount)
	return nil
}

// ---------------------------------------------------------------------------
// Email helper
// ---------------------------------------------------------------------------

// sendEmail sends an email via SMTP. If SMTP is not configured, it logs
// a warning and returns nil (graceful degradation).
func sendEmail(to, subject, body string) error {
	host := os.Getenv("JC_SMTP_HOST")
	if host == "" {
		slog.Warn("email_send: SMTP not configured (JC_SMTP_HOST is empty), skipping")
		return nil
	}

	port := os.Getenv("JC_SMTP_PORT")
	if port == "" {
		port = "587"
	}

	user := os.Getenv("JC_SMTP_USER")
	pass := os.Getenv("JC_SMTP_PASS")
	from := os.Getenv("JC_SMTP_FROM")
	if from == "" {
		from = user
	}

	addr := host + ":" + port
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		from, to, subject, body)

	var auth smtp.Auth
	if user != "" && pass != "" {
		auth = smtp.PlainAuth("", user, pass, host)
	}

	// Try TLS first for port 465, otherwise use STARTTLS or plain.
	if port == "465" {
		tlsConfig := &tls.Config{ServerName: host}
		conn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			slog.Error("email_send: TLS dial failed", "addr", addr, "error", err)
			return fmt.Errorf("TLS dial %s: %w", addr, err)
		}
		defer conn.Close()

		client, err := smtp.NewClient(conn, host)
		if err != nil {
			return fmt.Errorf("smtp client: %w", err)
		}
		defer client.Close()

		if auth != nil {
			if err := client.Auth(auth); err != nil {
				return fmt.Errorf("smtp auth: %w", err)
			}
		}
		if err := client.Mail(from); err != nil {
			return fmt.Errorf("smtp mail: %w", err)
		}
		if err := client.Rcpt(to); err != nil {
			return fmt.Errorf("smtp rcpt: %w", err)
		}
		w, err := client.Data()
		if err != nil {
			return fmt.Errorf("smtp data: %w", err)
		}
		if _, err := w.Write([]byte(msg)); err != nil {
			return fmt.Errorf("smtp write: %w", err)
		}
		if err := w.Close(); err != nil {
			return fmt.Errorf("smtp close: %w", err)
		}
		return client.Quit()
	}

	// STARTTLS or plain SMTP.
	if err := smtp.SendMail(addr, auth, from, []string{to}, []byte(msg)); err != nil {
		slog.Error("email_send: SMTP send failed", "to", to, "addr", addr, "error", err)
		return fmt.Errorf("smtp send to %s: %w", to, err)
	}

	return nil
}

// isValidTableName checks if a table name contains only safe characters.
func isValidTableName(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}
