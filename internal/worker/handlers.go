package worker

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
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
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// ---------------------------------------------------------------------------
// Package-level dependencies for database and cache access.
// Call RegisterDBPool and RegisterRedis at startup before processing tasks.
// ---------------------------------------------------------------------------

var (
	dbPool  *db.DB
	siteName string
	//nolint:unused // set by RegisterRedis
	rdb interface {
		Ping(ctx context.Context) error
	}
	taskEnqueuer *Enqueuer
)

// RegisterDBPool sets the database connection pool for worker handlers.
func RegisterDBPool(d *db.DB) {
	dbPool = d
}

// RegisterSiteName sets the site name for worker handlers.
func RegisterSiteName(name string) {
	siteName = name
}

// RegisterTaskEnqueuer sets the task enqueuer for worker handlers that need
// to enqueue follow-up tasks (e.g. payment webhook → affiliate commission).
func RegisterTaskEnqueuer(e *Enqueuer) {
	taskEnqueuer = e
}

// RegisterRedis sets the Valkey/Redis client for worker handlers.
func RegisterRedis(client interface {
	Ping(ctx context.Context) error
},
) {
	rdb = client
}

// settingsReader is the subset of config.Settings the worker needs to read
// hot-reloadable operational limits. Declared as a local interface so the
// worker package does not import config (avoids an import cycle); *config.Settings
// satisfies it structurally.
type settingsReader interface {
	GetIntOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback int) int
}

// appSettings holds the injected settings reader. Nil until RegisterSettings is
// called, in which case handlers fall back to their compiled-in defaults.
var appSettings settingsReader

// RegisterSettings injects the hot-reloadable settings reader for worker handlers.
func RegisterSettings(s settingsReader) {
	appSettings = s
}

// videoProcessor holds the video processor for background video processing.
var videoProcessor interface {
	Transcode(ctx context.Context, inputPath, outputPath string) error
	GenerateThumbnail(ctx context.Context, inputPath, outputPath, timestamp string) error
	Compress(ctx context.Context, inputPath, outputPath string) error
}

// RegisterVideoProcessor sets the video processor for the video_processing handler.
func RegisterVideoProcessor(p interface {
	Transcode(ctx context.Context, inputPath, outputPath string) error
	GenerateThumbnail(ctx context.Context, inputPath, outputPath, timestamp string) error
	Compress(ctx context.Context, inputPath, outputPath string) error
},
) {
	videoProcessor = p
}

// geoipUpdater holds the GeoIP database updater for the maintenance task.
var geoipUpdater interface{ DownloadAndUpdate() error }

// RegisterGeoIPUpdater sets the GeoIP database updater for the maintenance task.
func RegisterGeoIPUpdater(u interface{ DownloadAndUpdate() error }) {
	geoipUpdater = u
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
		return nil, errors.New("database pool not registered")
	}

	return gen.New(dbPool.Writer), nil
}

// readDB returns a read-only query interface.

// ---------------------------------------------------------------------------
// Critical tier handlers
// ---------------------------------------------------------------------------

// HandleNotificationDispatchBatch processes a batch notification window from the queue.
// The notification service's DispatchBatch reads the window, builds the aggregated
// notification row, and marks the window dispatched.
func HandleNotificationDispatchBatch(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:notification:dispatch_batch", "payload", t.Payload())

	if notificationBatchDispatcher == nil {
		slog.Warn("task:notification:dispatch_batch: no dispatcher registered, skipping")
		return nil
	}

	var payload struct {
		WindowID string `json:"window_id"`
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:notification:dispatch_batch: invalid payload", "error", err)
		return err
	}

	windowID, err := scanUUID(payload.WindowID)
	if err != nil {
		slog.Error("task:notification:dispatch_batch: invalid window_id", "value", payload.WindowID, "error", err)
		return err
	}

	return notificationBatchDispatcher.DispatchBatch(ctx, windowID)
}


// HandleWebhookDelivery delivers a queued webhook to the registered endpoint.
func HandleWebhookDelivery(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:webhook:deliver", "payload", t.Payload())

	if webhookProcessor == nil {
		slog.Warn("task:webhook:deliver: no webhook processor registered, skipping")
		return nil
	}

	var payload struct {
		DeliveryID string `json:"delivery_id"`
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:webhook:deliver: invalid payload", "error", err)
		return err
	}

	var deliveryID pgtype.UUID
	if err := deliveryID.Scan(payload.DeliveryID); err != nil {
		slog.Error("task:webhook:deliver: invalid delivery_id", "error", err)
		return err
	}

	return webhookProcessor.DeliverWebhook(ctx, deliveryID.Bytes)
}

// HandlePaymentWebhookProcess processes an incoming payment webhook event and updates payment status.
func HandlePaymentWebhookProcess(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:payment:process_webhook", "payload", t.Payload())

	queries, err := requireDB()
	if err != nil {
		slog.Warn("task:payment:process_webhook: no database, skipping", "error", err)
		return nil
	}

	var payload struct {
		EventID  string `json:"event_id"`
		TenantID string `json:"tenant_id"`
		Status   string `json:"status"` // "succeeded", "failed", "refunded"
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:payment:process_webhook: invalid payload", "error", err)
		return err
	}

	tenantID, err := scanUUID(payload.TenantID)
	if err != nil {
		return fmt.Errorf("task:payment:process_webhook: %w", err)
	}

	// Look up pending payments and match by provider_ref.
	rows, err := queries.ListPaymentsByStatus(ctx, gen.ListPaymentsByStatusParams{
		TenantID: tenantID,
		Status:   "pending",
	})
	if err != nil {
		slog.Error("task:payment:process_webhook: failed to list pending payments", "error", err)
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
				slog.Error("task:payment:process_webhook: failed to update payment",
					"payment_id", p.ID, "error", err)
				continue
			}

			updated++

			slog.Info("task:payment:process_webhook: payment updated",
				"payment_id", p.ID, "status", payload.Status)

			// Enqueue affiliate commission crediting on successful payment.
			if payload.Status == "succeeded" && taskEnqueuer != nil {
				commissionPayload, _ := json.Marshal(map[string]string{
					"payment_id": p.ID.String(),
					"user_id":    p.UserID.String(),
					"amount":     fmt.Sprintf("%d", p.Amount),
				})
				if err := taskEnqueuer.Enqueue(TypeAffiliateCreditCommission, commissionPayload); err != nil {
					slog.Error("task:payment:process_webhook: failed to enqueue affiliate commission",
						"payment_id", p.ID, "error", err)
				}
			}
		}
	}

	if updated == 0 {
		slog.Warn("task:payment:process_webhook: no matching payment found",
			"event_id", payload.EventID)
	}

	return nil
}

// HandleGdprRetentionSweep sweeps for GDPR deletion requests in retention_wait
// whose retention period has elapsed, and triggers hard delete for each.
// 
func HandleGdprRetentionSweep(ctx context.Context, _ *asynq.Task) error {
	slog.Info("task:gdpr:retention_sweep: starting sweep")

	if dbPool == nil {
		slog.Warn("task:gdpr:retention_sweep: no database, skipping")
		return nil
	}

	if gdprProcessor == nil {
		slog.Warn("task:gdpr:retention_sweep: no GDPR processor, skipping")
		return nil
	}

	rows, err := dbPool.Reader.Query(ctx, `
		SELECT r.id, r.tenant_id
		FROM account_deletion_requests r
		JOIN system_settings s ON s.tenant_id = r.tenant_id AND s.key = 'gdpr.retention_days'
		WHERE r.status = 'retention_wait'
		  AND r.created_at + ((s.value->>0)::int || ' days')::interval <= now()
		ORDER BY r.created_at ASC
		LIMIT 100
	`)
	if err != nil {
		slog.Error("task:gdpr:retention_sweep: query failed", "error", err)
		return err
	}
	defer rows.Close()

	var requestIDs [][16]byte
	for rows.Next() {
		var id, tenantID pgtype.UUID
		if err := rows.Scan(&id, &tenantID); err != nil {
			slog.Error("task:gdpr:retention_sweep: scan error", "error", err)
			continue
		}
		requestIDs = append(requestIDs, id.Bytes)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if len(requestIDs) == 0 {
		return nil
	}

	completed := 0
	for _, reqID := range requestIDs {
		if err := gdprProcessor.ProcessAccountDeletion(ctx, reqID); err != nil {
			errpkg.CaptureException(fmt.Errorf("gdpr retention sweep deletion failed: %w", err))
			continue
		}
		completed++
	}

	slog.Info("task:gdpr:retention_sweep: sweep complete",
		"completed", completed, "total", len(requestIDs))
	return nil
}

// HandleTicketSendConfirmation sends a confirmation email when a new ticket is created.
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

	if err := sendEmail(payload.Email, "Ticket Confirmation - "+ticket.Subject, body, false); err != nil {
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

// HandleEmailSend sends a single transactional email via SMTP.
func HandleEmailSend(_ context.Context, t *asynq.Task) error {
	slog.Info("task:email:send", "payload", t.Payload())

	var payload struct {
		To      string `json:"to"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
		HTML    bool   `json:"html"`
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:email:send: invalid payload", "error", err)
		return err
	}

	if payload.To == "" || payload.Subject == "" {
		return errors.New("task:email:send: to and subject are required")
	}

	if err := sendEmail(payload.To, payload.Subject, payload.Body, payload.HTML); err != nil {
		slog.Error("task:email:send: failed", "to", payload.To, "error", err)
		return err
	}

	slog.Info("task:email:send: sent", "to", payload.To)

	return nil
}

// HandleSearchIndexSync synchronizes an entity to the Meilisearch index.
func HandleSearchIndexSync(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:search:index_entity", "payload", t.Payload())

	if searchIndexer == nil {
		slog.Warn("task:search:index_entity: no search indexer registered, skipping")
		return nil
	}

	var payload struct {
		ContentID  string `json:"content_id"`
		EntityID   string `json:"entity_id"`
		EntityType string `json:"entity_type"`
		TenantID   string `json:"tenant_id"`
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:search:index_entity: invalid payload", "error", err)
		return err
	}

	// Support both legacy content_id and new entity_id fields.
	entityIDStr := payload.EntityID
	if entityIDStr == "" {
		entityIDStr = payload.ContentID
	}

	if entityIDStr == "" {
		slog.Error("task:search:index_entity: missing entity_id")
		return errors.New("missing entity_id")
	}

	var entityID pgtype.UUID
	if err := entityID.Scan(entityIDStr); err != nil {
		slog.Error("task:search:index_entity: invalid entity_id", "value", entityIDStr, "error", err)
		return err
	}

	var tenantID pgtype.UUID
	if err := tenantID.Scan(payload.TenantID); err != nil {
		slog.Error("task:search:index_entity: invalid tenant_id", "value", payload.TenantID, "error", err)
		return err
	}

	entityType := payload.EntityType
	if entityType == "" {
		entityType = "content"
	}

	switch entityType {
	case "content":
		return searchIndexer.IndexContent(ctx, entityID.Bytes, tenantID.Bytes)
	case "forum_thread":
		return searchIndexer.IndexForumThread(ctx, entityID.Bytes, tenantID.Bytes)
	case "space":
		return searchIndexer.IndexSpace(ctx, entityID.Bytes, tenantID.Bytes)
	case "user":
		return searchIndexer.IndexUser(ctx, entityID.Bytes, tenantID.Bytes)
	case "tag":
		return searchIndexer.IndexTag(ctx, entityID.Bytes, tenantID.Bytes)
	default:
		slog.Warn("task:search:index_entity: unknown entity type, skipping", "entity_type", entityType)
		return nil
	}
}

// HandleImageProcessing processes uploaded images through imgproxy for resizing and optimization.
func HandleImageProcessing(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:content:process_image", "payload", t.Payload())

	if imageProcessor == nil {
		slog.Warn("task:content:process_image: no image processor registered, skipping")
		return nil
	}

	var payload struct {
		MediaID   string `json:"media_id"`
		TenantID  string `json:"tenant_id"`
		Operation string `json:"operation"` // "resize", "optimize", "thumbnail"
		Width     int    `json:"width"`
		Height    int    `json:"height"`
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:content:process_image: invalid payload", "error", err)
		return err
	}

	if payload.MediaID == "" {
		slog.Error("task:content:process_image: missing media_id in payload")
		return errors.New("missing media_id")
	}

	if err := imageProcessor.ProcessImage(ctx, payload.MediaID, payload.TenantID, payload.Operation, payload.Width, payload.Height); err != nil {
		slog.Error("task:content:process_image: failed",
			"media_id", payload.MediaID, "operation", payload.Operation, "error", err)
		return err
	}

	slog.Info("task:content:process_image: completed",
		"media_id", payload.MediaID,
		"operation", payload.Operation,
		"width", payload.Width,
		"height", payload.Height)

	return nil
}

// HandleAITaskProcessing executes an AI task through the configured provider.
func HandleAITaskProcessing(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:ai:process_task", "payload", t.Payload())

	if aiProcessor == nil {
		slog.Warn("task:ai:process_task: no ai processor registered, skipping")
		return nil
	}

	var payload struct {
		TaskType  string `json:"task_type"`
		Prompt    string `json:"prompt"`
		ModelID   string `json:"model_id"`
		RequestID string `json:"request_id"`
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:ai:process_task: invalid payload", "error", err)
		return err
	}

	if err := aiProcessor.ProcessAITask(ctx, payload.TaskType, payload.Prompt, payload.ModelID, payload.RequestID); err != nil {
		slog.Error("task:ai:process_task: failed",
			"task_type", payload.TaskType, "request_id", payload.RequestID, "error", err)
		return err
	}

	slog.Info("task:ai:process_task: processed",
		"task_type", payload.TaskType,
		"model_id", payload.ModelID,
		"request_id", payload.RequestID)

	return nil
}

// HandleEngagementCountUpdate recalculates engagement counts for a content entity.
func HandleEngagementCountUpdate(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:content:update_engagement", "payload", t.Payload())

	queries, err := requireDB()
	if err != nil {
		slog.Warn("task:content:update_engagement: no database, skipping", "error", err)
		return nil
	}

	var payload struct {
		EntityType     string `json:"entity_type"`
		EntityID       string `json:"entity_id"`
		EngagementType string `json:"engagement_type"` // "view", "comment", "share", "bookmark"
		Delta          int    `json:"delta"`
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:content:update_engagement: invalid payload", "error", err)
		return err
	}

	if payload.EntityType != "content" {
		slog.Info("task:content:update_engagement: non-content entity, skipping",
			"entity_type", payload.EntityType)
		return nil
	}

	entityID, err := scanUUID(payload.EntityID)
	if err != nil {
		return fmt.Errorf("task:content:update_engagement: %w", err)
	}

	// Fetch current scores.
	score, err := queries.GetContentScore(ctx, entityID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Warn("task:content:update_engagement: no content_score found",
				"content_id", payload.EntityID)
			return nil
		}

		slog.Error("task:content:update_engagement: failed to get content score",
			"content_id", payload.EntityID, "error", err)

		return err
	}

	voteScore := score.VoteScore
	commentCount := score.CommentCount
	viewCount := score.ViewCount
	bookmarkCount := score.BookmarkCount

	switch payload.EngagementType {
	case "view":
		viewCount += int32(payload.Delta) //nolint:gosec // G115: delta is typically ±1 for engagement events
	case "comment":
		commentCount += int32(payload.Delta) //nolint:gosec // G115: delta is typically ±1 for engagement events
	case "bookmark":
		bookmarkCount += int32(payload.Delta) //nolint:gosec // G115: delta is typically ±1 for engagement events
	}

	if err := queries.UpdateContentScore(ctx, gen.UpdateContentScoreParams{
		VoteScore:     voteScore,
		CommentCount:  commentCount,
		ViewCount:     viewCount,
		BookmarkCount: bookmarkCount,
		ContentID:     entityID,
	}); err != nil {
		slog.Error("task:content:update_engagement: failed to update score",
			"content_id", payload.EntityID, "error", err)
		return err
	}

	slog.Info("task:content:update_engagement: score updated",
		"content_id", payload.EntityID,
		"engagement_type", payload.EngagementType,
		"delta", payload.Delta)

	return nil
}

// HandleModerationScan runs content through the spam and moderation pipeline.
func HandleModerationScan(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:content:scan_moderation", "payload", t.Payload())

	if moderationProcessor == nil {
		slog.Warn("task:content:scan_moderation: no moderation processor registered, skipping")
		return nil
	}

	var payload struct {
		EntityType string `json:"entity_type"`
		EntityID   string `json:"entity_id"`
		TenantID   string `json:"tenant_id"`
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:content:scan_moderation: invalid payload", "error", err)
		return err
	}

	if err := moderationProcessor.ScanContent(ctx, payload.EntityType, payload.EntityID, payload.TenantID); err != nil {
		slog.Error("task:content:scan_moderation: scan failed",
			"entity_type", payload.EntityType, "entity_id", payload.EntityID, "error", err)
		return err
	}

	slog.Info("task:content:scan_moderation: scanned",
		"entity_type", payload.EntityType,
		"entity_id", payload.EntityID)

	return nil
}

// HandleScheduledPublish publishes content that has reached its scheduled publish time.
func HandleScheduledPublish(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:content:publish_scheduled", "payload", t.Payload())

	queries, err := requireDB()
	if err != nil {
		slog.Warn("task:content:publish_scheduled: no database, skipping", "error", err)
		return nil
	}

	var payload struct {
		ContentID string `json:"content_id"`
		TenantID  string `json:"tenant_id"`
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:content:publish_scheduled: invalid payload", "error", err)
		return err
	}

	contentID, err := scanUUID(payload.ContentID)
	if err != nil {
		return fmt.Errorf("task:content:publish_scheduled: %w", err)
	}

	tenantID, err := scanUUID(payload.TenantID)
	if err != nil {
		return fmt.Errorf("task:content:publish_scheduled: %w", err)
	}

	// Update the content status to published.
	if _, err := queries.UpdateContentStatus(ctx, gen.UpdateContentStatusParams{
		Status:   "published",
		ID:       contentID,
		TenantID: tenantID,
	}); err != nil {
		slog.Error("task:content:publish_scheduled: failed to publish",
			"content_id", payload.ContentID, "error", err)
		return err
	}

	slog.Info("task:content:publish_scheduled: content published",
		"content_id", payload.ContentID)

	return nil
}

// HandleLinkPreviewFetch fetches and stores OpenGraph metadata for a URL.
func HandleLinkPreviewFetch(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:content:fetch_link_preview", "payload", t.Payload())

	if linkPreviewProcessor == nil {
		slog.Warn("task:content:fetch_link_preview: no link preview processor registered, skipping")
		return nil
	}

	var payload struct {
		URL      string `json:"url"`
		BlockID  string `json:"block_id"`
		TenantID string `json:"tenant_id"`
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:content:fetch_link_preview: invalid payload", "error", err)
		return err
	}

	if payload.URL == "" {
		slog.Error("task:content:fetch_link_preview: missing url in payload")
		return errors.New("missing url")
	}

	if err := linkPreviewProcessor.FetchAndStore(ctx, payload.URL, payload.BlockID, payload.TenantID); err != nil {
		slog.Error("task:content:fetch_link_preview: failed",
			"url", payload.URL, "error", err)
		return err
	}

	slog.Info("task:content:fetch_link_preview: fetched",
		"url", payload.URL, "block_id", payload.BlockID)

	return nil
}

// HandleLanguagePackAITranslate translates a language pack using AI.
func HandleLanguagePackAITranslate(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:i18n:translate_language_pack", "payload", t.Payload())

	if aiTranslateProcessor == nil {
		slog.Warn("task:i18n:translate_language_pack: no ai translate processor registered, skipping")
		return nil
	}

	var payload struct {
		PackID     string `json:"pack_id"`
		SourceLang string `json:"source_lang"`
		TargetLang string `json:"target_lang"`
		TenantID   string `json:"tenant_id"`
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:i18n:translate_language_pack: invalid payload", "error", err)
		return err
	}

	if err := aiTranslateProcessor.TranslatePack(ctx, payload.PackID, payload.SourceLang, payload.TargetLang, payload.TenantID); err != nil {
		slog.Error("task:i18n:translate_language_pack: translation failed",
			"pack_id", payload.PackID, "error", err)
		return err
	}

	slog.Info("task:i18n:translate_language_pack: translation queued",
		"pack_id", payload.PackID,
		"source", payload.SourceLang,
		"target", payload.TargetLang)

	return nil
}

// HandleMediaScan scans uploaded media for viruses and validates file integrity.
func HandleMediaScan(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:content:scan_media", "payload", t.Payload())

	if mediaScanProcessor == nil {
		slog.Warn("task:content:scan_media: no media scan processor registered, skipping")
		return nil
	}

	var payload struct {
		MediaID string `json:"media_id"`
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:content:scan_media: invalid payload", "error", err)
		return err
	}

	if payload.MediaID == "" {
		slog.Error("task:content:scan_media: missing media_id in payload")
		return errors.New("missing media_id")
	}

	return mediaScanProcessor.ProcessMediaScan(ctx, payload.MediaID)
}

// HandleContentFanOutToFeeds fans out a published content item to all relevant user feeds.
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

	// Hot-reloadable fan-out threshold (feed.fanout_threshold). Falls back to
	// 5000 when unset or when settings are not injected. See AUDIT-BE-009.
	maxFanOut := 5000
	if appSettings != nil {
		maxFanOut = appSettings.GetIntOr(ctx, tenantID, "feed.fanout_threshold", 5000)
	}

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

			if err := rows.Err(); err != nil {
				slog.Error("task:content:fan_out_to_feeds: bubble rows iteration error",
					"content_id", payload.ContentID, "parent_id", parentID, "error", err)
			}

			rows.Close()

			depth++
		}

		slog.Info("task:content:fan_out_to_feeds: bubble fan-out complete",
			"content_id", payload.ContentID, "fed_count", fedCount, "depth", depth)
	}

	return nil
}

// HandleAIGenerateContentEmbedding generates a vector embedding for content search.
func HandleAIGenerateContentEmbedding(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:content:generate_embedding", "payload", t.Payload())

	if contentEmbedder == nil {
		slog.Warn("task:content:generate_embedding: no content embedder registered, skipping")
		return nil
	}

	var payload struct {
		ContentID string `json:"content_id"`
		TenantID  string `json:"tenant_id"`
		Text      string `json:"text"`
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:content:generate_embedding: invalid payload", "error", err)
		return err
	}

	contentID, err := scanUUID(payload.ContentID)
	if err != nil {
		return fmt.Errorf("task:content:generate_embedding: %w", err)
	}

	tenantID, err := scanUUID(payload.TenantID)
	if err != nil {
		return fmt.Errorf("task:content:generate_embedding: %w", err)
	}

	if payload.Text == "" {
		slog.Warn("task:content:generate_embedding: empty text, skipping",
			"content_id", payload.ContentID)
		return nil
	}

	return contentEmbedder.EmbedContent(ctx, contentID.Bytes, tenantID.Bytes, payload.Text)
}

// HandleReviewAggregateRecalc recalculates the aggregate rating for a reviewable entity.
func HandleReviewAggregateRecalc(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:content:recalc_review_aggregate", "payload", t.Payload())

	queries, err := requireDB()
	if err != nil {
		slog.Warn("task:content:recalc_review_aggregate: no database, skipping", "error", err)
		return nil
	}

	var payload struct {
		TenantID   string `json:"tenant_id"`
		EntityType string `json:"entity_type"`
		EntityID   string `json:"entity_id"`
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:content:recalc_review_aggregate: invalid payload", "error", err)
		return err
	}

	tenantID, err := scanUUID(payload.TenantID)
	if err != nil {
		return fmt.Errorf("task:content:recalc_review_aggregate: %w", err)
	}

	entityID, err := scanUUID(payload.EntityID)
	if err != nil {
		return fmt.Errorf("task:content:recalc_review_aggregate: %w", err)
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
		slog.Error("task:content:recalc_review_aggregate: failed to list reviews",
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
		slog.Error("task:content:recalc_review_aggregate: failed to upsert aggregate",
			"entity_type", payload.EntityType, "entity_id", payload.EntityID, "error", err)
		return err
	}

	slog.Info("task:content:recalc_review_aggregate: aggregate updated",
		"entity_type", payload.EntityType, "entity_id", payload.EntityID,
		"review_count", publishedCount, "avg_rating", fmt.Sprintf("%.1f", avgRating))

	return nil
}

// HandleTicketSendStatusUpdate sends a status update email when a ticket's status changes.
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

	if err := sendEmail(payload.Email, "Ticket Status Update - "+ticket.Subject, body, false); err != nil {
		slog.Error("task:ticket:send_status_update: email failed",
			"ticket_id", payload.TicketID, "error", err)
		return err
	}

	slog.Info("task:ticket:send_status_update: notification sent",
		"ticket_id", payload.TicketID, "new_status", payload.NewStatus)

	return nil
}

// HandleLearningUpdateTrackCompletion recalculates a user's progress in a learning track.
func HandleLearningUpdateTrackCompletion(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:learning:update_track_completion", "payload", t.Payload())

	if learningProcessor == nil {
		slog.Warn("task:learning:update_track_completion: no learning processor registered, skipping")
		return nil
	}

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

	if err := learningProcessor.UpdateTrackCompletion(ctx, payload.UserID, payload.TrackID, payload.TenantID); err != nil {
		slog.Error("task:learning:update_track_completion: failed",
			"user_id", payload.UserID, "track_id", payload.TrackID, "error", err)
		return err
	}

	slog.Info("task:learning:update_track_completion: progress updated",
		"user_id", payload.UserID,
		"track_id", payload.TrackID,
		"progress", payload.Progress)

	return nil
}

// HandleVideoProcessing processes uploaded video files for transcoding and thumbnail generation.
func HandleVideoProcessing(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:content:process_video", "payload", t.Payload())

	var payload struct {
		MediaID   string `json:"media_id"`
		TenantID  string `json:"tenant_id"`
		Operation string `json:"operation"` // "transcode", "thumbnail", "compress"
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:content:process_video: invalid payload", "error", err)
		return err
	}

	if videoProcessor == nil {
		slog.Warn("task:content:process_video: no video processor registered, skipping")
		return nil
	}

	queries, err := requireDB()
	if err != nil {
		slog.Warn("task:content:process_video: no database, skipping", "error", err)
		return nil
	}

	mediaID, err := scanUUID(payload.MediaID)
	if err != nil {
		return fmt.Errorf("task:content:process_video: %w", err)
	}

	tenantID, err := scanUUID(payload.TenantID)
	if err != nil {
		return fmt.Errorf("task:content:process_video: %w", err)
	}

	// Look up the media record to get the current storage path.
	media, err := queries.GetMediaByID(ctx, gen.GetMediaByIDParams{
		ID:       mediaID,
		TenantID: tenantID,
	})
	if err != nil {
		slog.Error("task:content:process_video: media not found",
			"media_id", payload.MediaID, "error", err)
		return fmt.Errorf("task:content:process_video: media not found: %w", err)
	}

	inputPath := media.StoragePath

	// Route to the appropriate processing operation.
	var outputPath string

	switch payload.Operation {
	case "transcode":
		outputPath = inputPath + ".transcoded.mp4"
		if err := videoProcessor.Transcode(ctx, inputPath, outputPath); err != nil {
			slog.Error("task:content:process_video: transcode failed",
				"media_id", payload.MediaID, "error", err)
			return fmt.Errorf("task:content:process_video: transcode: %w", err)
		}
	case "thumbnail":
		outputPath = inputPath + ".thumb.jpg"
		if err := videoProcessor.GenerateThumbnail(ctx, inputPath, outputPath, "00:00:01"); err != nil {
			slog.Error("task:content:process_video: thumbnail failed",
				"media_id", payload.MediaID, "error", err)
			return fmt.Errorf("task:content:process_video: thumbnail: %w", err)
		}
	case "compress":
		outputPath = inputPath + ".compressed.mp4"
		if err := videoProcessor.Compress(ctx, inputPath, outputPath); err != nil {
			slog.Error("task:content:process_video: compress failed",
				"media_id", payload.MediaID, "error", err)
			return fmt.Errorf("task:content:process_video: compress: %w", err)
		}
	default:
		return fmt.Errorf("task:content:process_video: unknown operation %q", payload.Operation)
	}

	// Update the media record to point to the processed file.
	_, err = dbPool.Writer.Exec(ctx,
		`UPDATE media SET storage_path = $1 WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL`,
		outputPath, mediaID, tenantID)
	if err != nil {
		slog.Error("task:content:process_video: failed to update storage path",
			"media_id", payload.MediaID, "new_path", outputPath, "error", err)
		return fmt.Errorf("task:content:process_video: update storage path: %w", err)
	}

	slog.Info("task:content:process_video: completed",
		"media_id", payload.MediaID,
		"operation", payload.Operation,
		"output_path", outputPath)

	return nil
}

// HandleWalletProcessActivityEvent processes a wallet activity event and updates the ledger.
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
	reason := "Activity: " + payload.ActivityKey

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

// HandleNewsletterSend sends a newsletter campaign to all subscribers.
func HandleNewsletterSend(_ context.Context, t *asynq.Task) error {
	slog.Info("task:newsletter:send", "payload", t.Payload())

	var payload struct {
		NewsletterID string   `json:"newsletter_id"`
		TenantID     string   `json:"tenant_id"`
		Subject      string   `json:"subject"`
		HTMLBody     string   `json:"html_body"`
		Recipients   []string `json:"recipients"`
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:newsletter:send: invalid payload", "error", err)
		return err
	}

	sent := 0
	failed := 0

	for _, recipient := range payload.Recipients {
		if err := sendEmail(recipient, payload.Subject, payload.HTMLBody, true); err != nil {
			slog.Error("task:newsletter:send: email failed",
				"recipient", recipient, "error", err)

			failed++

			continue
		}

		sent++
	}

	slog.Info("task:newsletter:send: batch complete",
		"newsletter_id", payload.NewsletterID,
		"sent", sent, "failed", failed,
		"total", len(payload.Recipients))

	return nil
}

// ---------------------------------------------------------------------------
// Low tier handlers
// ---------------------------------------------------------------------------

// HandleDigestEmail sends a digest email summarizing recent activity to a user.
func HandleDigestEmail(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:email:send_digest", "payload", t.Payload())

	if dbPool == nil {
		slog.Warn("task:email:send_digest: no database, skipping")
		return nil
	}

	var payload struct {
		UserID   string `json:"user_id"`
		TenantID string `json:"tenant_id"`
		Email    string `json:"email"`
		Period   string `json:"period"` // "daily", "weekly"
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:email:send_digest: invalid payload", "error", err)
		return err
	}

	userID, err := scanUUID(payload.UserID)
	if err != nil {
		return fmt.Errorf("task:email:send_digest: %w", err)
	}

	tenantID, err := scanUUID(payload.TenantID)
	if err != nil {
		return fmt.Errorf("task:email:send_digest: %w", err)
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
		slog.Error("task:email:send_digest: failed to fetch feed",
			"user_id", payload.UserID, "error", err)
		return err
	}
	defer feedItems.Close()

	var body strings.Builder

	body.WriteString(fmt.Sprintf("<h2>%s Digest</h2>\n", cases.Title(language.English).String(payload.Period)))
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
		slog.Error("task:email:send_digest: row iteration error", "error", err)
	}

	if itemCount == 0 {
		slog.Info("task:email:send_digest: no items, skipping",
			"user_id", payload.UserID)
		return nil
	}

	if err := sendEmail(payload.Email,
		fmt.Sprintf("Your %s Digest", cases.Title(language.English).String(payload.Period)),
		body.String(), true); err != nil {
		slog.Error("task:email:send_digest: email failed",
			"user_id", payload.UserID, "error", err)
		return err
	}

	slog.Info("task:email:send_digest: sent",
		"user_id", payload.UserID, "items", itemCount)

	return nil
}

// HandleSitemapGenerate regenerates the XML sitemap for the tenant.
func HandleSitemapGenerate(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:sitemap:generate", "payload", t.Payload())

	if dbPool == nil {
		slog.Warn("task:sitemap:generate: no database, skipping")
		return nil
	}

	var payload struct {
		TenantID string `json:"tenant_id"`
		BaseURL  string `json:"base_url"`
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:sitemap:generate: invalid payload", "error", err)
		return err
	}

	tenantID, err := scanUUID(payload.TenantID)
	if err != nil {
		return fmt.Errorf("task:sitemap:generate: %w", err)
	}

	// Generate sitemap XML from published content.
	rows, err := dbPool.Reader.Query(ctx,
		`SELECT slug, updated_at FROM contents
		 WHERE tenant_id = $1 AND status = 'published'
		 ORDER BY updated_at DESC
		 LIMIT 10000`,
		tenantID)
	if err != nil {
		slog.Error("task:sitemap:generate: failed to list content", "error", err)
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

	if err := os.WriteFile("sitemap.xml", []byte(sitemap.String()), 0o600); err != nil {
		slog.Error("task:sitemap:generate: failed to write file", "error", err)
		return err
	}

	slog.Info("task:sitemap:generate: sitemap written", "url_count", urlCount)

	return nil
}

// HandleRSSRegenerate regenerates the RSS feed for published content.
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
	rss.WriteString(fmt.Sprintf("  <title>%s</title>\n", siteName))
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

	if err := os.WriteFile("feed.xml", []byte(rss.String()), 0o600); err != nil {
		slog.Error("task:rss_regenerate: failed to write file", "error", err)
		return err
	}

	slog.Info("task:rss_regenerate: feed written", "item_count", itemCount)

	return nil
}

// HandleDataExport generates a GDPR data export archive for a user.
func HandleDataExport(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:data:export_user_data", "payload", t.Payload())

	if gdprProcessor == nil {
		slog.Warn("task:data:export_user_data: no GDPR processor registered, skipping")
		return nil
	}

	var payload struct {
		RequestID string `json:"request_id"`
		TenantID  string `json:"tenant_id"`
		UserID    string `json:"user_id"`
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:data:export_user_data: invalid payload", "error", err)
		return err
	}

	var requestID pgtype.UUID
	if err := requestID.Scan(payload.RequestID); err != nil {
		slog.Error("task:data:export_user_data: invalid request_id", "error", err)
		return err
	}

	if err := gdprProcessor.ProcessDataExport(ctx, requestID.Bytes); err != nil {
		errpkg.CaptureException(fmt.Errorf("gdpr data export failed (request=%s): %w", payload.RequestID, err))
		return err
	}
	return nil
}

// HandleAccountDeletionCleanup performs the final cleanup after an account deletion request.
func HandleAccountDeletionCleanup(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:account:deletion_cleanup", "payload", t.Payload())

	if gdprProcessor == nil {
		slog.Warn("task:account:deletion_cleanup: no GDPR processor registered, skipping")
		return nil
	}

	var payload struct {
		RequestID string `json:"request_id"`
		TenantID  string `json:"tenant_id"`
		UserID    string `json:"user_id"`
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:account:deletion_cleanup: invalid payload", "error", err)
		return err
	}

	var requestID pgtype.UUID
	if err := requestID.Scan(payload.RequestID); err != nil {
		slog.Error("task:account:deletion_cleanup: invalid request_id", "error", err)
		return err
	}

	if err := gdprProcessor.ProcessAccountDeletion(ctx, requestID.Bytes); err != nil {
		errpkg.CaptureException(fmt.Errorf("gdpr account deletion failed (request=%s): %w", payload.RequestID, err))
		return err
	}
	return nil
}

// HandleAdEventIngest ingests an ad impression or click event for analytics.
func HandleAdEventIngest(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:ad:ingest_event_batch", "payload", t.Payload())

	queries, err := requireDB()
	if err != nil {
		slog.Warn("task:ad:ingest_event_batch: no database, skipping", "error", err)
		return nil
	}

	var payload struct {
		TenantID   string `json:"tenant_id"`
		CampaignID string `json:"campaign_id"`
		UserID     string `json:"user_id"`
		EventType  string `json:"event_type"` // "impression", "click"
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:ad:ingest_event_batch: invalid payload", "error", err)
		return err
	}

	tenantID, err := scanUUID(payload.TenantID)
	if err != nil {
		return fmt.Errorf("task:ad:ingest_event_batch: %w", err)
	}

	campaignID, err := scanUUID(payload.CampaignID)
	if err != nil {
		return fmt.Errorf("task:ad:ingest_event_batch: %w", err)
	}

	userID, err := scanUUID(payload.UserID)
	if err != nil {
		return fmt.Errorf("task:ad:ingest_event_batch: %w", err)
	}

	if _, err := queries.InsertAdEvent(ctx, gen.InsertAdEventParams{
		TenantID:   tenantID,
		CampaignID: campaignID,
		UserID:     userID,
		EventType:  payload.EventType,
	}); err != nil {
		slog.Error("task:ad:ingest_event_batch: failed to insert event",
			"campaign_id", payload.CampaignID, "error", err)
		return err
	}

	slog.Info("task:ad:ingest_event_batch: event recorded",
		"campaign_id", payload.CampaignID,
		"event_type", payload.EventType)

	return nil
}

// HandleMaintenanceUpdateVelocityScores recalculates spam velocity scores for recent content.
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

// HandleMaintenanceCheckSLABreaches checks for tickets that have breached their SLA deadlines.
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

// HandleMaintenanceUpdateGeoIPDB downloads and updates the GeoIP database.
func HandleMaintenanceUpdateGeoIPDB(_ context.Context, t *asynq.Task) error {
	slog.Info("task:maintenance:update_geoip_db", "payload", t.Payload())

	if geoipUpdater == nil {
		slog.Warn("task:maintenance:update_geoip_db: updater not registered, skipping")
		return nil
	}

	if err := geoipUpdater.DownloadAndUpdate(); err != nil {
		slog.Error("task:maintenance:update_geoip_db: update failed", "error", err)
		return fmt.Errorf("geoip database update: %w", err)
	}

	slog.Info("task:maintenance:update_geoip_db: update complete")

	return nil
}

// HandleSpaceAutoPopulate auto-populates a space with recommended content based on its tags.
func HandleSpaceAutoPopulate(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:space:auto_populate", "payload", t.Payload())

	if spaceAutoPopulateProcessor == nil {
		slog.Warn("task:space:auto_populate: no space auto-populate processor registered, skipping")
		return nil
	}

	var payload struct {
		SpaceID  string `json:"space_id"`
		TenantID string `json:"tenant_id"`
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:space:auto_populate: invalid payload", "error", err)
		return err
	}

	if err := spaceAutoPopulateProcessor.AutoPopulateSpace(ctx, payload.SpaceID, payload.TenantID); err != nil {
		slog.Error("task:space:auto_populate: failed",
			"space_id", payload.SpaceID, "error", err)
		return err
	}

	slog.Info("task:space:auto_populate: completed",
		"space_id", payload.SpaceID)

	return nil
}

// HandleAnalyticsRefreshViews refreshes the materialized analytics view counts.
func HandleAnalyticsRefreshViews(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:analytics:refresh_views", "payload", t.Payload())

	if analyticsProcessor == nil {
		slog.Warn("task:analytics:refresh_views: no analytics processor registered, skipping")
		return nil
	}

	return analyticsProcessor.RefreshViews(ctx)
}

// HandleSecurityScan runs a periodic security scan on uploaded media.
func HandleSecurityScan(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:security:scan", "payload", t.Payload())

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

	if dbPool == nil {
		slog.Warn("task:security:scan: no database, skipping")
		return nil
	}

	// Find media with status 'uploaded' that need scanning.
	rows, err := dbPool.Reader.Query(ctx, `
		SELECT id, file_path FROM media
		WHERE tenant_id = $1 AND status = 'uploaded'
		ORDER BY created_at ASC
		LIMIT 50
	`, tenantID)
	if err != nil {
		slog.Error("task:security:scan: query failed", "error", err)
		return err
	}
	defer rows.Close()

	scanCount := 0
	for rows.Next() {
		var id pgtype.UUID
		var filePath string
		if err := rows.Scan(&id, &filePath); err != nil {
			continue
		}
		// Enqueue each media for ClamAV scan via the moderation processor.
		if moderationProcessor != nil {
			if err := moderationProcessor.ScanContent(ctx, "media", id.String(), payload.TenantID); err != nil {
				slog.Error("task:security:scan: scan enqueue failed", "media_id", id.String(), "error", err)
			} else {
				scanCount++
			}
		}
	}

	if err := rows.Err(); err != nil {
		slog.Error("task:security:scan: rows iteration error", "tenant_id", payload.TenantID, "error", err)
		return err
	}

	slog.Info("task:security:scan: complete", "scanned_count", scanCount)
	return nil
}

// HandleMaintenanceRegenerateLLMsTxt regenerates the llms.txt file for the tenant.
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

	llms.WriteString(fmt.Sprintf("# %s\n\n", siteName))
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

	if err := os.WriteFile("llms.txt", []byte(llms.String()), 0o600); err != nil {
		slog.Error("task:maintenance:regenerate_llms_txt: failed to write file", "error", err)
		return err
	}

	slog.Info("task:maintenance:regenerate_llms_txt: file written",
		"page_count", pageCount)

	return nil
}

// HandleLearningIssueCertificate issues a completion certificate for a learning track.
func HandleLearningIssueCertificate(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:learning:issue_certificate", "payload", t.Payload())

	if learningProcessor == nil {
		slog.Warn("task:learning:issue_certificate: no learning processor registered, skipping")
		return nil
	}

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

	if err := learningProcessor.IssueCertificate(ctx, payload.UserID, payload.TrackID, payload.TenantID, payload.CourseName); err != nil {
		slog.Error("task:learning:issue_certificate: failed",
			"user_id", payload.UserID, "track_id", payload.TrackID, "error", err)
		return err
	}

	slog.Info("task:learning:issue_certificate: certificate issued",
		"user_id", payload.UserID,
		"track_id", payload.TrackID,
		"course_name", payload.CourseName)

	return nil
}

// HandleDBManualVacuum runs a manual VACUUM on high-churn database tables.
func HandleDBManualVacuum(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:db:manual_vacuum", "payload", t.Payload())

	if dbPool == nil {
		slog.Warn("task:db:manual_vacuum: no database, skipping")
		return nil
	}

	var payload struct {
		TableName string `json:"table_name"` // optional: specific table to vacuum
		Full      bool   `json:"full"`       // VACUUM FULL vs regular
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:db:manual_vacuum: invalid payload", "error", err)
		return err
	}

	vacuumSQL := "VACUUM"
	if payload.Full {
		vacuumSQL = "VACUUM FULL"
	}

	if payload.TableName != "" {
		if !isValidTableName(payload.TableName) {
			return fmt.Errorf("task:db:manual_vacuum: invalid table name %q", payload.TableName)
		}

		vacuumSQL += " " + payload.TableName
	}

	if _, err := dbPool.Writer.Exec(ctx, vacuumSQL); err != nil {
		slog.Error("task:db:manual_vacuum: vacuum failed", "error", err)
		return err
	}

	slog.Info("task:db:manual_vacuum: vacuum complete", "sql", vacuumSQL)

	return nil
}

// HandleNewsletterDigest compiles and sends a weekly newsletter digest.
func HandleNewsletterDigest(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:newsletter:digest", "payload", t.Payload())

	if dbPool == nil {
		slog.Warn("task:newsletter:digest: no database, skipping")
		return nil
	}

	var payload struct {
		TenantID string `json:"tenant_id"`
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:newsletter:digest: invalid payload", "error", err)
		return err
	}

	tenantID, err := scanUUID(payload.TenantID)
	if err != nil {
		return fmt.Errorf("task:newsletter:digest: %w", err)
	}

	// Fetch recent published content for the digest.
	rows, err := dbPool.Reader.Query(ctx,
		`SELECT title, slug, created_at FROM contents
		 WHERE tenant_id = $1 AND status = 'published'
		 ORDER BY created_at DESC
		 LIMIT 10`,
		tenantID)
	if err != nil {
		slog.Error("task:newsletter:digest: failed to list content", "error", err)
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

	slog.Info("task:newsletter:digest: digest prepared",
		"tenant_id", payload.TenantID,
		"item_count", itemCount)

	return nil
}

// ---------------------------------------------------------------------------
// Trending & Discovery handlers
// ---------------------------------------------------------------------------

// HandleComputeTrending recomputes trending snapshots for all tenants.
func HandleComputeTrending(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:maintenance:compute_trending", "payload", t.Payload())

	if trendingProcessor == nil {
		slog.Warn("task:maintenance:compute_trending: no trending processor registered, skipping")
		return nil
	}

	return trendingProcessor.ComputeTrending(ctx)
}

// HandleComputeSuggestions recomputes suggestion data freshness.
func HandleComputeSuggestions(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:maintenance:compute_suggestions", "payload", t.Payload())

	if trendingProcessor == nil {
		slog.Warn("task:maintenance:compute_suggestions: no trending processor registered, skipping")
		return nil
	}

	return trendingProcessor.ComputeSuggestions(ctx)
}

// ---------------------------------------------------------------------------
// Push notification handler
// ---------------------------------------------------------------------------

// HandlePushDelivery delivers a push notification to a user's registered device.
func HandlePushDelivery(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:push:deliver", "payload", t.Payload())

	if pushDelivery == nil {
		slog.Warn("task:push:deliver: no push delivery registered, skipping")
		return nil
	}

	if !pushDelivery.IsEnabled() {
		slog.Warn("task:push:deliver: VAPID keys not configured, skipping")
		return nil
	}

	var payload struct {
		UserID    string `json:"user_id"`
		Title     string `json:"title"`
		Body      string `json:"body"`
		ActionURL string `json:"action_url"`
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:push:deliver: invalid payload", "error", err)
		return err
	}

	userID, err := scanUUID(payload.UserID)
	if err != nil {
		return fmt.Errorf("task:push:deliver: %w", err)
	}

	if dbPool == nil {
		slog.Warn("task:push:deliver: no database, skipping")
		return nil
	}

	// Fetch user's push subscriptions.
	rows, err := dbPool.Reader.Query(ctx,
		`SELECT id, endpoint, p256dh, auth_key FROM push_subscriptions WHERE user_id = $1`,
		userID)
	if err != nil {
		slog.Error("task:push:deliver: failed to query subscriptions",
			"user_id", payload.UserID, "error", err)
		return nil // non-fatal: subscriptions may not exist yet
	}
	defer rows.Close()

	// Build the push payload.
	type pushPayload struct {
		Title     string `json:"title"`
		Body      string `json:"body"`
		ActionURL string `json:"action_url,omitempty"`
	}

	pushJSON, err := json.Marshal(pushPayload{
		Title:     payload.Title,
		Body:      payload.Body,
		ActionURL: payload.ActionURL,
	})
	if err != nil {
		return fmt.Errorf("task:push:delivery: marshal payload: %w", err)
	}

	sent := 0
	failed := 0

	for rows.Next() {
		var id pgtype.UUID

		var endpoint, p256dh, authKey string
		if err := rows.Scan(&id, &endpoint, &p256dh, &authKey); err != nil {
			slog.Error("task:push:deliver: scan error", "error", err)

			failed++

			continue
		}

		if err := pushDelivery.Send(ctx, endpoint, p256dh, authKey, pushJSON); err != nil {
			slog.Error("task:push:deliver: send failed",
				"user_id", payload.UserID, "endpoint", endpoint[:min(30, len(endpoint))], "error", err)

			failed++
			// If subscription expired (410), remove it.
			if isSubscriptionExpired(err) {
				slog.Info("task:push:deliver: removing expired subscription",
					"user_id", payload.UserID)

				_, _ = dbPool.Writer.Exec(ctx,
					`DELETE FROM push_subscriptions WHERE id = $1`, id)
			}

			continue
		}

		sent++
	}

	if err := rows.Err(); err != nil {
		slog.Error("task:push:deliver: row iteration error", "error", err)
	}

	slog.Info("task:push:deliver: complete",
		"user_id", payload.UserID, "sent", sent, "failed", failed)

	return nil
}

// isSubscriptionExpired checks if an error indicates a 410 Gone response.
func isSubscriptionExpired(err error) bool {
	return err != nil && errors.Is(err, errpkg.ErrExpired)
}

// pushDelivery holds the push notification delivery dependency.
// Set via RegisterPushDelivery before task processing starts.
var pushDelivery interface {
	IsEnabled() bool
	Send(ctx context.Context, endpoint, p256dh, auth string, payload []byte) error
}

// RegisterPushDelivery sets the push delivery implementation for the worker.
func RegisterPushDelivery(d interface {
	IsEnabled() bool
	Send(ctx context.Context, endpoint, p256dh, auth string, payload []byte) error
},
) {
	pushDelivery = d
}

// ---------------------------------------------------------------------------
// DM notification handlers (critical tier — push notifications for DM events)
// ---------------------------------------------------------------------------

// HandleDMNotifyRequest enqueues a push notification for a new DM request.
// Payload: { recipient_id, tenant_id, sender_username, conversation_id }
func HandleDMNotifyRequest(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:dm:notify_request", "payload", t.Payload())

	var payload struct {
		RecipientID    string `json:"recipient_id"`
		TenantID       string `json:"tenant_id"`
		SenderUsername string `json:"sender_username"`
		ConversationID string `json:"conversation_id"`
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:dm:notify_request: invalid payload", "error", err)
		return err
	}

	if taskEnqueuer == nil {
		slog.Warn("task:dm:notify_request: no task enqueuer, skipping")
		return nil
	}

	title := "New message request"
	body := payload.SenderUsername + " wants to send you a message"
	actionURL := "/dm/conversations/" + payload.ConversationID

	pushPayload, _ := json.Marshal(map[string]string{
		"user_id":    payload.RecipientID,
		"title":      title,
		"body":       body,
		"action_url": actionURL,
	})

	if err := taskEnqueuer.Enqueue(TypePushDelivery, pushPayload); err != nil {
		slog.Error("task:dm:notify_request: failed to enqueue push", "error", err)
		return err
	}

	slog.Info("task:dm:notify_request: push enqueued", "recipient_id", payload.RecipientID)
	return nil
}

// HandleDMNotifyMessage enqueues a push notification for a new DM message
// when the conversation is not currently open. Payload:
// { recipient_id, tenant_id, sender_username, conversation_id, body_preview }
func HandleDMNotifyMessage(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:dm:notify_message", "payload", t.Payload())

	var payload struct {
		RecipientID    string `json:"recipient_id"`
		TenantID       string `json:"tenant_id"`
		SenderUsername string `json:"sender_username"`
		ConversationID string `json:"conversation_id"`
		BodyPreview    string `json:"body_preview"`
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:dm:notify_message: invalid payload", "error", err)
		return err
	}

	if taskEnqueuer == nil {
		slog.Warn("task:dm:notify_message: no task enqueuer, skipping")
		return nil
	}

	title := payload.SenderUsername
	body := payload.BodyPreview
	if body == "" {
		body = "Sent you a message"
	}
	actionURL := "/dm/conversations/" + payload.ConversationID

	pushPayload, _ := json.Marshal(map[string]string{
		"user_id":    payload.RecipientID,
		"title":      title,
		"body":       body,
		"action_url": actionURL,
	})

	if err := taskEnqueuer.Enqueue(TypePushDelivery, pushPayload); err != nil {
		slog.Error("task:dm:notify_message: failed to enqueue push", "error", err)
		return err
	}

	slog.Info("task:dm:notify_message: push enqueued", "recipient_id", payload.RecipientID)
	return nil
}

// ---------------------------------------------------------------------------
// Periodic sweep handlers (enqueued by Asynq scheduler)
// ---------------------------------------------------------------------------

// HandleScheduledPublishSweep sweeps for content past its scheduled publish time and publishes it.
func HandleScheduledPublishSweep(ctx context.Context, _ *asynq.Task) error {
	slog.Info("task:content:publish_sweep: starting sweep")

	if dbPool == nil {
		slog.Warn("task:content:publish_sweep: no database, skipping")
		return nil
	}

	// Find draft content ready to publish.
	rows, err := dbPool.Reader.Query(ctx, `
		SELECT id, tenant_id
		FROM contents
		WHERE status = 'draft'
		  AND publish_at IS NOT NULL
		  AND publish_at <= now()
		ORDER BY publish_at ASC
		LIMIT 100
	`)
	if err != nil {
		slog.Error("task:content:publish_sweep: query failed", "error", err)
		return err
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
			slog.Error("task:content:publish_sweep: scan error", "error", err)
			continue
		}

		pending = append(pending, pc)
	}

	if err := rows.Err(); err != nil {
		return err
	}

	if len(pending) == 0 {
		slog.Info("task:content:publish_sweep: no content ready to publish")
		return nil
	}

	published := 0

	for _, pc := range pending {
		_, err := dbPool.Writer.Exec(ctx, `
			UPDATE contents
			SET status = 'published',
			    published_at = COALESCE(published_at, now()),
			    updated_at = now()
			WHERE id = $1
			  AND tenant_id = $2
			  AND status = 'draft'
		`, pc.id, pc.tenantID)
		if err != nil {
			slog.Error("task:content:publish_sweep: failed to publish",
				"content_id", pc.id.String(), "error", err)
			continue
		}

		published++

		slog.Info("task:content:publish_sweep: content published",
			"content_id", pc.id.String())
	}

	slog.Info("task:content:publish_sweep: sweep complete",
		"published", published, "total", len(pending))

	return nil
}

// HandleVelocityScoreRecalc recalculates velocity scores for recently
// published content. The formula is:
//
// HandleVelocityScoreRecalc recalculates velocity scores for spam detection.
func HandleVelocityScoreRecalc(ctx context.Context, _ *asynq.Task) error {
	slog.Info("task:maintenance:recalc_velocity_scores: starting recalculation")

	if dbPool == nil {
		slog.Warn("task:maintenance:recalc_velocity_scores: no database, skipping")
		return nil
	}

	// Ensure every published content from the last 30 days has an engagement row.
	_, err := dbPool.Writer.Exec(ctx, `
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
		slog.Error("task:maintenance:recalc_velocity_scores: failed to ensure engagement rows", "error", err)
		return err
	}

	// Recalculate velocity scores.
	result, err := dbPool.Writer.Exec(ctx, `
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
		slog.Error("task:maintenance:recalc_velocity_scores: recalculation failed", "error", err)
		return err
	}

	rowsAffected := result.RowsAffected()
	slog.Info("task:maintenance:recalc_velocity_scores: complete", "updated", rowsAffected)

	return nil
}

// ---------------------------------------------------------------------------
// Email helper
// ---------------------------------------------------------------------------

// wrapHTMLEmail wraps an HTML body in a minimal responsive email template.
func wrapHTMLEmail(body string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title></title>
<style>
  body { margin: 0; padding: 0; background-color: #f4f4f7; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; }
  .wrapper { width: 100%%; background-color: #f4f4f7; padding: 40px 0; }
  .container { max-width: 600px; margin: 0 auto; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.06); }
  .content { padding: 32px; color: #333333; font-size: 15px; line-height: 1.6; }
  .footer { padding: 24px 32px; text-align: center; font-size: 12px; color: #999999; border-top: 1px solid #eeeeee; }
  a { color: #2563eb; text-decoration: none; }
  @media only screen and (max-width: 600px) {
    .container { margin: 0 16px; }
    .content { padding: 24px 20px; }
  }
</style>
</head>
<body>
<div class="wrapper">
  <div class="container">
    <div class="content">%s</div>
    <div class="footer">%s &mdash; Your journey, connected.</div>
  </div>
</div>
</body>
</html>`, body, siteName)
}

// smtpConfig holds parsed SMTP configuration from environment variables.
type smtpConfig struct {
	Host string
	Port string
	User string
	Pass string
	From string
	Addr string
}

// readSMTPConfig reads SMTP settings from environment variables.
// Returns nil if JC_SMTP_HOST is not set (SMTP not configured).
func readSMTPConfig() *smtpConfig {
	host := os.Getenv("JC_SMTP_HOST")
	if host == "" {
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

	return &smtpConfig{
		Host: host,
		Port: port,
		User: user,
		Pass: pass,
		From: from,
		Addr: host + ":" + port,
	}
}

// sendEmail sends an email via SMTP. If SMTP is not configured, it logs
// a warning and returns nil (graceful degradation).
func sendEmail(to, subject, body string, isHTML bool) error {
	cfg := readSMTPConfig()
	if cfg == nil {
		slog.Warn("email_send: SMTP not configured (JC_SMTP_HOST is empty), skipping")
		return nil
	}

	contentType := "text/plain; charset=UTF-8"
	contentBody := body

	if isHTML {
		contentType = "text/html; charset=UTF-8"
		contentBody = wrapHTMLEmail(body)
	}

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: %s\r\n\r\n%s",
		cfg.From, to, subject, contentType, contentBody)

	var auth smtp.Auth
	if cfg.User != "" && cfg.Pass != "" {
		auth = smtp.PlainAuth("", cfg.User, cfg.Pass, cfg.Host)
	}

	// Try TLS first for port 465, otherwise use STARTTLS or plain.
	if cfg.Port == "465" {
		tlsConfig := &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}

		conn, err := tls.Dial("tcp", cfg.Addr, tlsConfig)
		if err != nil {
			slog.Error("email_send: TLS dial failed", "addr", cfg.Addr, "error", err)
			return fmt.Errorf("TLS dial %s: %w", cfg.Addr, err)
		}

		defer func() { _ = conn.Close() }()

		client, err := smtp.NewClient(conn, cfg.Host)
		if err != nil {
			return fmt.Errorf("smtp client: %w", err)
		}

		defer func() { _ = client.Close() }()

		if auth != nil {
			if err := client.Auth(auth); err != nil {
				return fmt.Errorf("smtp auth: %w", err)
			}
		}

		if err := client.Mail(cfg.From); err != nil {
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
	if err := smtp.SendMail(cfg.Addr, auth, cfg.From, []string{to}, []byte(msg)); err != nil {
		slog.Error("email_send: SMTP send failed", "to", to, "addr", cfg.Addr, "error", err)
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
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '_' {
			return false
		}
	}

	return true
}

// ── Missing Phase 31 Task Handlers (stub implementations) ───────────

// HandleMaintenanceCheckExpiredBans lifts expired user_moderation_actions with action='ban'.
func HandleMaintenanceCheckExpiredBans(ctx context.Context, _ *asynq.Task) error {
	slog.Info("maintenance:check_expired_bans — lifting expired bans")
	_, err := requireDB()
	if err != nil {
		return err
	}
	// Mark expired bans as 'unban' actions.
	result, err := dbPool.Writer.Exec(ctx, `
		UPDATE user_moderation_actions
		SET action = 'unban'
		WHERE action = 'ban'
		AND expires_at IS NOT NULL
		AND expires_at < now()
	`)
	if err != nil {
		slog.Error("maintenance:check_expired_bans failed", "error", err)
		return err
	}
	rows := result.RowsAffected()
	if rows > 0 {
		slog.Info("maintenance:check_expired_bans — lifted expired bans", "count", rows)
	}
	return nil
}

// HandleMaintenanceCheckExpiredBoosts expires organization_boosts past ends_at.
func HandleMaintenanceCheckExpiredBoosts(ctx context.Context, _ *asynq.Task) error {
	slog.Info("maintenance:check_expired_boosts — expiring boosts")
	_, err := requireDB()
	if err != nil {
		return err
	}
	result, err := dbPool.Writer.Exec(ctx, `
		UPDATE organization_boosts
		SET status = 'expired'
		WHERE status = 'active'
		AND ends_at < now()
	`)
	if err != nil {
		slog.Error("maintenance:check_expired_boosts failed", "error", err)
		return err
	}
	rows := result.RowsAffected()
	if rows > 0 {
		slog.Info("maintenance:check_expired_boosts — expired boosts", "count", rows)
	}
	return nil
}

// HandleMaintenancePruneOldFeeds deletes feeds rows older than 30 days.
func HandleMaintenancePruneOldFeeds(ctx context.Context, _ *asynq.Task) error {
	slog.Info("maintenance:prune_old_feeds — pruning old feed entries")
	_, err := requireDB()
	if err != nil {
		return err
	}
	result, err := dbPool.Writer.Exec(ctx, `
		DELETE FROM feeds
		WHERE created_at < now() - interval '30 days'
	`)
	if err != nil {
		slog.Error("maintenance:prune_old_feeds failed", "error", err)
		return err
	}
	rows := result.RowsAffected()
	if rows > 0 {
		slog.Info("maintenance:prune_old_feeds — pruned old feeds", "count", rows)
	}
	return nil
}

// HandleMaintenancePruneAuditLogs deletes audit_logs rows older than 90 days.
func HandleMaintenancePruneAuditLogs(ctx context.Context, _ *asynq.Task) error {
	slog.Info("maintenance:prune_audit_logs — pruning old audit logs")
	_, err := requireDB()
	if err != nil {
		return err
	}
	result, err := dbPool.Writer.Exec(ctx, `
		DELETE FROM audit_logs
		WHERE created_at < now() - interval '90 days'
	`)
	if err != nil {
		slog.Error("maintenance:prune_audit_logs failed", "error", err)
		return err
	}
	rows := result.RowsAffected()
	if rows > 0 {
		slog.Info("maintenance:prune_audit_logs — pruned old audit logs", "count", rows)
	}
	return nil
}

// HandleMaintenancePrunePushTokens deletes stale/unresponsive push tokens older than 90 days.
func HandleMaintenancePrunePushTokens(ctx context.Context, _ *asynq.Task) error {
	slog.Info("maintenance:prune_push_tokens — pruning stale push tokens")
	_, err := requireDB()
	if err != nil {
		return err
	}
	result, err := dbPool.Writer.Exec(ctx, `
		DELETE FROM push_tokens
		WHERE last_used_at < now() - interval '90 days'
		   OR (last_used_at IS NULL AND created_at < now() - interval '30 days')
	`)
	if err != nil {
		slog.Error("maintenance:prune_push_tokens failed", "error", err)
		return err
	}
	rows := result.RowsAffected()
	if rows > 0 {
		slog.Info("maintenance:prune_push_tokens — pruned stale tokens", "count", rows)
	}
	return nil
}

// HandleModerationAutoHide checks if a reported entity has reached the
// auto-hide threshold and archives it. Enqueued after each report is created.
func HandleModerationAutoHide(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:moderation:auto_hide", "payload", t.Payload())

	if dbPool == nil {
		slog.Warn("task:moderation:auto_hide: no database, skipping")
		return nil
	}

	var payload struct {
		EntityType string `json:"entity_type"`
		EntityID   string `json:"entity_id"`
		TenantID   string `json:"tenant_id"`
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:moderation:auto_hide: invalid payload", "error", err)
		return err
	}

	tenantID, err := scanUUID(payload.TenantID)
	if err != nil {
		return fmt.Errorf("task:moderation:auto_hide: %w", err)
	}

	entityID, err := scanUUID(payload.EntityID)
	if err != nil {
		return fmt.Errorf("task:moderation:auto_hide: %w", err)
	}

	// Read the auto-hide threshold from system settings (default: 3 reports).
	threshold := 3
	if appSettings != nil {
		threshold = appSettings.GetIntOr(ctx, tenantID, "moderation.auto_hide_report_count", 3)
	}

	// Count pending reports for this entity.
	var reportCount int
	err = dbPool.Reader.QueryRow(ctx,
		`SELECT COUNT(*) FROM content_reports
		 WHERE tenant_id = $1 AND entity_type = $2 AND entity_id = $3
		   AND status = 'pending'`,
		tenantID, payload.EntityType, entityID).Scan(&reportCount)
	if err != nil {
		slog.Error("task:moderation:auto_hide: failed to count reports",
			"entity_type", payload.EntityType, "entity_id", payload.EntityID, "error", err)
		return err
	}

	if reportCount < threshold {
		slog.Info("task:moderation:auto_hide: below threshold, skipping",
			"entity_type", payload.EntityType, "entity_id", payload.EntityID,
			"report_count", reportCount, "threshold", threshold)
		return nil
	}

	// Archive the entity based on its type.
	switch payload.EntityType {
	case "content":
		result, err := dbPool.Writer.Exec(ctx,
			`UPDATE contents SET status = 'archived', updated_at = now()
			 WHERE id = $1 AND tenant_id = $2 AND status != 'archived'`,
			entityID, tenantID)
		if err != nil {
			slog.Error("task:moderation:auto_hide: failed to archive content",
				"content_id", payload.EntityID, "error", err)
			return err
		}
		if n := result.RowsAffected(); n > 0 {
			slog.Info("task:moderation:auto_hide: content archived",
				"content_id", payload.EntityID, "report_count", reportCount)
		}

	case "comment":
		result, err := dbPool.Writer.Exec(ctx,
			`UPDATE comments SET deleted_at = now()
			 WHERE id = $1 AND deleted_at IS NULL`,
			entityID)
		if err != nil {
			slog.Error("task:moderation:auto_hide: failed to hide comment",
				"comment_id", payload.EntityID, "error", err)
			return err
		}
		if n := result.RowsAffected(); n > 0 {
			slog.Info("task:moderation:auto_hide: comment hidden",
				"comment_id", payload.EntityID, "report_count", reportCount)
		}

	case "message":
		result, err := dbPool.Writer.Exec(ctx,
			`UPDATE dm_messages SET deleted_at = now()
			 WHERE id = $1 AND deleted_at IS NULL`,
			entityID)
		if err != nil {
			slog.Error("task:moderation:auto_hide: failed to hide message",
				"message_id", payload.EntityID, "error", err)
			return err
		}
		if n := result.RowsAffected(); n > 0 {
			slog.Info("task:moderation:auto_hide: message hidden",
				"message_id", payload.EntityID, "report_count", reportCount)
		}

	default:
		// user/space reports require human review — log but don't auto-hide.
		slog.Info("task:moderation:auto_hide: entity type requires human review",
			"entity_type", payload.EntityType, "entity_id", payload.EntityID,
			"report_count", reportCount)
	}

	return nil
}

// HandleDMCleanupDeclinedRequests purges dm_requests declined > 90 days ago.
func HandleDMCleanupDeclinedRequests(ctx context.Context, _ *asynq.Task) error {
	slog.Info("dm:cleanup_declined_requests — purging old declined requests")
	_, err := requireDB()
	if err != nil {
		return err
	}
	result, err := dbPool.Writer.Exec(ctx, `
		DELETE FROM dm_requests
		WHERE status = 'declined'
		AND updated_at < now() - interval '90 days'
	`)
	if err != nil {
		slog.Error("dm:cleanup_declined_requests failed", "error", err)
		return err
	}
	rows := result.RowsAffected()
	if rows > 0 {
		slog.Info("dm:cleanup_declined_requests — purged old declined requests", "count", rows)
	}
	return nil
}

// HandleCouponTrackEvents flushes coupon_events to aggregate counters.
func HandleCouponTrackEvents(ctx context.Context, t *asynq.Task) error {
	slog.Info("coupon:track_events — flushing coupon event counters")
	_, err := requireDB()
	if err != nil {
		return err
	}
	// Aggregate coupon usage counts from events table
	result, err := dbPool.Writer.Exec(ctx, `
		UPDATE discount_codes dc
		SET uses_count = (
			SELECT COUNT(*)::int
			FROM coupon_events ce
			WHERE ce.coupon_code = dc.code
			  AND ce.created_at > dc.updated_at
		)
		WHERE EXISTS (
			SELECT 1 FROM coupon_events ce
			WHERE ce.coupon_code = dc.code
			  AND ce.created_at > dc.updated_at
		)
	`)
	if err != nil {
		slog.Error("coupon:track_events failed", "error", err)
		return err
	}
	rows := result.RowsAffected()
	if rows > 0 {
		slog.Info("coupon:track_events — flushed coupon event counters", "count", rows)
	}
	return nil
}

// HandleCouponExpireCheck marks past-expiry coupons as expired.
func HandleCouponExpireCheck(ctx context.Context, _ *asynq.Task) error {
	slog.Info("coupon:expire_check — checking for expired coupons")
	_, err := requireDB()
	if err != nil {
		return err
	}
	result, err := dbPool.Writer.Exec(ctx, `
		UPDATE coupons
		SET status = 'expired'
		WHERE status = 'active'
		AND expires_at IS NOT NULL
		AND expires_at < now()
	`)
	if err != nil {
		slog.Error("coupon:expire_check failed", "error", err)
		return err
	}
	rows := result.RowsAffected()
	if rows > 0 {
		slog.Info("coupon:expire_check — expired coupons", "count", rows)
	}
	return nil
}

// HandleAffiliateRecordClick persists click event to affiliate_link_events.
func HandleAffiliateRecordClick(ctx context.Context, t *asynq.Task) error {
	slog.Info("affiliate:record_click — persisting click event")

	var payload struct {
		LinkID   string `json:"link_id"`
		UserID   string `json:"user_id"`
		IPAddr   string `json:"ip_addr"`
		UserAgent string `json:"user_agent"`
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("affiliate:record_click: invalid payload", "error", err)
		return err
	}

	_, err := requireDB()
	if err != nil {
		return err
	}

	linkID, err := scanUUID(payload.LinkID)
	if err != nil {
		return fmt.Errorf("affiliate:record_click: %w", err)
	}

	var userID pgtype.UUID
	if payload.UserID != "" {
		if uidErr := userID.Scan(payload.UserID); uidErr != nil {
			slog.Warn("affiliate:record_click: invalid user_id", "error", uidErr)
		}
	}

	_, err = dbPool.Writer.Exec(ctx, `
		INSERT INTO affiliate_link_events (link_id, user_id, event_type, ip_addr, user_agent, created_at)
		VALUES ($1, $2, 'click', $3, $4, now())
	`, linkID, userID, payload.IPAddr, payload.UserAgent)
	if err != nil {
		slog.Error("affiliate:record_click failed", "error", err)
		return err
	}

	slog.Info("affiliate:record_click — recorded", "link_id", payload.LinkID)
	return nil
}

// HandleRedirectRecordHit increments hit_count/last_hit_at for a redirect rule.
// Moved off the request path (was a per-request goroutine in redirects.Service).
func HandleRedirectRecordHit(ctx context.Context, t *asynq.Task) error {
	var payload struct {
		RedirectID string `json:"redirect_id"`
		TenantID   string `json:"tenant_id"`
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("redirect:record_hit: invalid payload", "error", err)
		return err
	}

	q, err := requireDB()
	if err != nil {
		return err
	}

	redirectID, err := scanUUID(payload.RedirectID)
	if err != nil {
		return fmt.Errorf("redirect:record_hit: %w", err)
	}

	tenantID, err := scanUUID(payload.TenantID)
	if err != nil {
		return fmt.Errorf("redirect:record_hit: %w", err)
	}

	if err := q.RecordRedirectHit(ctx, gen.RecordRedirectHitParams{
		ID: redirectID, TenantID: tenantID,
	}); err != nil {
		slog.Error("redirect:record_hit failed", "redirect_id", payload.RedirectID, "error", err)
		return err
	}

	return nil
}

// HandleAffiliateCreditCommission credits commission after payment succeeds.
func HandleAffiliateCreditCommission(ctx context.Context, t *asynq.Task) error {
	slog.Info("affiliate:credit_commission — crediting commission")

	var payload struct {
		PaymentID string  `json:"payment_id"`
		UserID    string  `json:"user_id"`
		Amount    float64 `json:"amount"`
		LinkCode  string  `json:"link_code"`
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("affiliate:credit_commission: invalid payload", "error", err)
		return err
	}

	_, err := requireDB()
	if err != nil {
		return err
	}

	// Look up the affiliate link and member to determine commission rate
	var commissionRate int
	var memberUserID pgtype.UUID
	err = dbPool.Reader.QueryRow(ctx, `
		SELECT apm.user_id, COALESCE(aps.commission_rates->>'pro_monthly', '0')::int
		FROM affiliate_program_members apm
		JOIN user_affiliate_links ual ON ual.user_id = apm.user_id
		JOIN affiliate_program_settings aps ON aps.tenant_id = apm.tenant_id
		WHERE ual.link_code = $1 AND apm.status = 'active' AND ual.is_active = true
	`, payload.LinkCode).Scan(&memberUserID, &commissionRate)
	if err != nil {
		slog.Warn("affiliate:credit_commission: link not found or inactive", "link_code", payload.LinkCode, "error", err)
		return nil // not an error — link may be invalid
	}

	if commissionRate <= 0 {
		slog.Info("affiliate:credit_commission: no commission rate configured")
		return nil
	}

	// Calculate commission (percentage of payment amount)
	commission := int64(float64(payload.Amount) * float64(commissionRate) / 100.0)
	if commission <= 0 {
		return nil
	}

	paymentID, err := scanUUID(payload.PaymentID)
	if err != nil {
		return fmt.Errorf("affiliate:credit_commission: %w", err)
	}

	// Credit the affiliate member's wallet
	_, err = dbPool.Writer.Exec(ctx, `
		INSERT INTO wallet_transactions (tenant_id, user_id, amount, transaction_type, reference_type, reference_id, description, created_at)
		SELECT apm.tenant_id, apm.user_id, $1, 'credit', 'affiliate_commission', $2, 'Affiliate commission', now()
		FROM affiliate_program_members apm
		WHERE apm.user_id = $3 AND apm.status = 'active'
	`, commission, paymentID, memberUserID)
	if err != nil {
		slog.Error("affiliate:credit_commission: failed to credit wallet", "error", err)
		return err
	}

	slog.Info("affiliate:credit_commission — credited", "amount", commission, "link_code", payload.LinkCode)
	return nil
}

// HandleAffiliateExpireCheck deactivates links for suspended/banned members.
func HandleAffiliateExpireCheck(ctx context.Context, _ *asynq.Task) error {
	slog.Info("affiliate:expire_check — deactivating links for suspended members")
	_, err := requireDB()
	if err != nil {
		return err
	}
	result, err := dbPool.Writer.Exec(ctx, `
		UPDATE user_affiliate_links
		SET is_active = false
		WHERE is_active = true
		AND user_id IN (
			SELECT user_id FROM affiliate_program_members
			WHERE status IN ('suspended', 'banned')
		)
	`)
	if err != nil {
		slog.Error("affiliate:expire_check failed", "error", err)
		return err
	}
	rows := result.RowsAffected()
	if rows > 0 {
		slog.Info("affiliate:expire_check — deactivated links", "count", rows)
	}
	return nil
}
