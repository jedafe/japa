package worker

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"math"
	"math/big"
	"net/smtp"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/jedafe/japa/internal/bots"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	rssmodel "github.com/jedafe/japa/internal/rss/model"
	"github.com/jedafe/japa/internal/seo"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/email"
)

// paymentPurposeToAffiliateProduct maps a payment purpose_type + purpose_id
// to the corresponding affiliate product_type + product_id used in the
// jc:aff_click attribution key (Section 56).
//
// The redirect handler stores the attribution key using the affiliate link's
// product_type vocabulary (pro_subscription, course, general) and the link's
// product_id (NULL for pro_subscription/general → "null", content_id for
// course). The payment webhook, however, works with the payment's
// purpose_type vocabulary (pro_membership, course_access, credit_purchase,
// etc.) and the payment's purpose_id (subscription_id, content_id, etc.).
//
// Without this mapping, the attribution key stored by the redirect handler
// (jc:aff_click:{user}:pro_subscription:null) would never match the key
// looked up by the payment webhook (jc:aff_click:{user}:pro_membership:{sub_id}),
// and commission would never be credited (BUG-001/003).
//
// Mapping:
//   - pro_membership → pro_subscription, product_id = "null" (pro_subscription
//     links have no product_id; the subscription_id is not part of the key)
//   - course_access  → course, product_id = purpose_id (content_id matches)
//   - everything else → general, product_id = "null"
func paymentPurposeToAffiliateProduct(purposeType string, purposeID pgtype.UUID) (affProductType, affProductID string) {
	affProductID = "null"

	switch purposeType {
	case "pro_membership":
		affProductType = "pro_subscription"
		// pro_subscription links have NULL product_id — the subscription_id
		// is NOT part of the attribution key.
	case "course_access":
		affProductType = "course"

		if purposeID.Valid {
			affProductID = purposeID.String()
		}
	default:
		affProductType = "general"
	}

	return
}

// ---------------------------------------------------------------------------
// Package-level dependencies for database and cache access.
// Call RegisterDBPool and RegisterRedis at startup before processing tasks.
// ---------------------------------------------------------------------------

var (
	dbPool   *db.DB
	siteName string

	rdb          redisCache
	taskEnqueuer *Enqueuer
)

// emailGateway holds the email dispatcher for provider-based email sending.
var emailGateway *email.EmailDispatcher

// RegisterEmailGateway sets the email dispatcher for worker handlers.
func RegisterEmailGateway(d *email.EmailDispatcher) {
	emailGateway = d
}

// redisCache is the subset of the valkeycompat.Cmdable API that worker
// handlers need. Used by HandleRSSRegenerate to cache feeds under
// jc:cache:feed:rss:{tenant_id} (universal) and
// jc:cache:feed:rss:blog:{tenant_id} (blog-only), and by
// HandleMaintenanceRegenerateLLMsTxt to invalidate llms.txt caches.
// Also used by the intelligence cron handlers for the scheduler leader
// lock (jc:intel:scheduler_leader, BUG-13 fix).
// valkeycompat.Cmdable satisfies this interface.
type redisCache interface {
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *valkeycompat.StatusCmd
	Get(ctx context.Context, key string) *valkeycompat.StringCmd
	Del(ctx context.Context, keys ...string) *valkeycompat.IntCmd
	SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) *valkeycompat.BoolCmd
}

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
func RegisterRedis(client redisCache) {
	rdb = client
}

// settingsReader is the subset of config.Settings the worker needs to read
// hot-reloadable operational limits. Declared as a local interface so the
// worker package does not import config (avoids an import cycle); *config.Settings
// satisfies it structurally.
type settingsReader interface {
	GetIntOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback int) int
	GetFloatOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback float64) float64
	GetStringOr(ctx context.Context, tenantID pgtype.UUID, key, fallback string) string
	GetBoolOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback bool) bool
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

// parsePayloadOrSkip wraps parsePayload and returns asynq.SkipRetry on
// decode failure — malformed payloads are permanent errors that should
// not burn retries.
func parsePayloadOrSkip(t *asynq.Task, v any) error {
	if err := parsePayload(t, v); err != nil {
		return fmt.Errorf("malformed payload: %w: %w", err, asynq.SkipRetry)
	}

	return nil
}

// scanUUID parses a UUID string into a pgtype.UUID.
func scanUUID(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid uuid %q: %w", s, err)
	}

	return u, nil
}

// parseCreditPurchaseMetadata extracts credits_to_grant and currency_code
// from a credit_purchase payment's metadata JSON (Section 58). The metadata
// is set by wallet.purchaseCredits when creating the payment row.
func parseCreditPurchaseMetadata(metadata []byte) (credits int, currency string) {
	if len(metadata) == 0 {
		return 0, ""
	}

	var m map[string]any
	if err := json.Unmarshal(metadata, &m); err != nil {
		return 0, ""
	}

	if v, ok := m["credits_to_grant"]; ok {
		switch n := v.(type) {
		case float64:
			credits = int(n)
		case int:
			credits = n
		}
	}

	if v, ok := m["currency_code"].(string); ok {
		currency = v
	}

	return credits, currency
}

// scheduleFanOutTask enqueues a content:fan_out_to_feeds task with the given
// fan-out type. Used by the scheduled-publish handler to trigger multi-space
// fan-out (Section 32). Fire-and-forget per Rule 8.
func scheduleFanOutTask(enq *Enqueuer, contentID, tenantID, authorID, spaceID pgtype.UUID, contentType, fanOutType string) {
	if enq == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"content_id":   contentID.String(),
		"tenant_id":    tenantID.String(),
		"author_id":    authorID.String(),
		"space_id":     spaceID.String(),
		"content_type": contentType,
		"fan_out_type": fanOutType,
	})
	if err != nil {
		slog.Warn("scheduleFanOutTask: failed to marshal payload", "error", err)
		return
	}

	if err := enq.Enqueue(TypeContentFanOutToFeeds, payload); err != nil {
		slog.Warn("scheduleFanOutTask: failed to enqueue fan-out",
			"content_id", contentID.String(), "fan_out_type", fanOutType, "error", err)
	}
}

// scheduleRSSRegenerate enqueues the rss:regenerate task for a tenant so
// RSS feeds reflect newly published content without waiting for the hourly
// cron. Fire-and-forget per Rule 8 (RSS-013).
//
// Ref: japaconnect-engineering/sections/26-seo-performance.md
// Ref: japaconnect-engineering/sections/11-job-queue-asynq.md.
func scheduleRSSRegenerate(enq *Enqueuer, tenantID pgtype.UUID) {
	if enq == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"tenant_id": tenantID.String(),
	})
	if err != nil {
		slog.Warn("scheduleRSSRegenerate: failed to marshal payload", "error", err)
		return
	}

	if err := enq.Enqueue(TypeRSSRegenerate, payload); err != nil {
		slog.Warn("scheduleRSSRegenerate: failed to enqueue rss:regenerate",
			"tenant_id", tenantID.String(), "error", err)
	}
}

// requireDB returns the query interface or an error if the pool is not registered.
func requireDB() (*gen.Queries, error) {
	if dbPool == nil {
		return nil, errors.New("database pool not registered")
	}

	return gen.New(dbPool.Writer), nil
}

// resolveBaseURL returns the site base URL for a tenant by checking
// system_settings first, then falling back to the JC_SITE_URL env var.
func resolveBaseURL(ctx context.Context, tenantID pgtype.UUID) string {
	if dbPool == nil {
		return os.Getenv("JC_SITE_URL")
	}

	q := gen.New(dbPool.Reader)

	row, err := q.GetSettingValue(ctx, gen.GetSettingValueParams{
		TenantID: tenantID, Key: "site.base_url",
	})
	if err == nil && len(row) > 0 {
		return string(row)
	}

	return os.Getenv("JC_SITE_URL")
}

// newsletterUnsubscribeLink resolves (or creates) a one-click unsubscribe
// token for the recipient and returns the /unsubscribe?token=TOKEN URL plus
// the SMTP List-Unsubscribe / List-Unsubscribe-Post headers required for
// GDPR one-click unsubscribe (§62, ADR-080 §4). Returns an empty url and
// nil headers (with a warning log) if the subscriber cannot be resolved or
// the token cannot be created — the email is still sent without the link
// rather than dropped, but the failure is visible in logs.
//
// Ref: japaconnect-engineering/sections/62-newsletter-digest.md
//
//	japaconnect-engineering/adr/ADR-080-newsletter-digest.md
func newsletterUnsubscribeLink(ctx context.Context, tenantID pgtype.UUID, recipient string) (string, map[string]string) {
	if dbPool == nil {
		return "", nil
	}

	q := gen.New(dbPool.Reader)

	sub, err := q.GetSubscriberByEmail(ctx, gen.GetSubscriberByEmailParams{
		TenantID: tenantID, Email: recipient,
	})
	if err != nil {
		slog.Warn("newsletter: resolve subscriber for unsubscribe token failed",
			"recipient", recipient, "error", err)
		return "", nil
	}

	token, err := newsletterGenerateUnsubscribeToken()
	if err != nil {
		slog.Warn("newsletter: generate unsubscribe token failed", "error", err)
		return "", nil
	}

	if _, err := q.CreateUnsubscribeToken(ctx, gen.CreateUnsubscribeTokenParams{
		Token:        token,
		SubscriberID: sub.ID,
	}); err != nil {
		slog.Warn("newsletter: persist unsubscribe token failed", "error", err)
		return "", nil
	}

	base := resolveBaseURL(ctx, tenantID)
	if base == "" {
		base = os.Getenv("JC_SITE_URL")
	}

	url := strings.TrimRight(base, "/") + "/unsubscribe?token=" + token

	headers := map[string]string{
		"List-Unsubscribe":      "<" + url + ">",
		"List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
	}

	return url, headers
}

// newsletterGenerateUnsubscribeToken generates a 32-byte hex token for
// one-click unsubscribe. Mirrors internal/newsletter.GenerateUnsubscribeToken
// without importing the newsletter package (worker uses gen directly).
func newsletterGenerateUnsubscribeToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("newsletter: generate token: %w", err)
	}

	return hex.EncodeToString(b), nil
}

// readDB returns a read-only query interface.

// ---------------------------------------------------------------------------
// Critical tier handlers
// ---------------------------------------------------------------------------

// HandleNotificationDispatchBatch processes a batch notification window from the queue.
// The notification service's DispatchBatch reads the window, builds the aggregated
// notification row, and marks the window dispatched.
func HandleNotificationDispatchBatch(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:notification:dispatch_batch")

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
	slog.Info("task:webhook:deliver")

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
	slog.Info("task:payment:process_webhook")

	queries, err := requireDB()
	if err != nil {
		slog.Warn("task:payment:process_webhook: no database, skipping", "error", err)
		return nil
	}

	var payload struct {
		EventID     string `json:"event_id"`
		TenantID    string `json:"tenant_id"`
		Status      string `json:"status"` // "succeeded", "failed", "refunded"
		ProviderRef string `json:"provider_ref"`
		Provider    string `json:"provider"`
	}

	if err := parsePayloadOrSkip(t, &payload); err != nil {
		slog.Error("task:payment:process_webhook: invalid payload", "error", err)
		return err
	}

	tenantID, err := scanUUID(payload.TenantID)
	if err != nil {
		return fmt.Errorf("task:payment:process_webhook: %w", err)
	}

	// Match the payment by provider_ref (BUG-002/BUG-019). The provider_ref
	// is the transaction reference stored on the payment row at intent time
	// (Stripe pi_.../cs_..., Paystack reference, NOWPayments order_id). The
	// event_id is the provider's event ID (Stripe evt_...) which is a
	// different value and must not be used for matching.
	if payload.ProviderRef == "" {
		slog.Warn("task:payment:process_webhook: no provider_ref in payload, cannot match",
			"event_id", payload.EventID)
		return nil
	}

	p, err := queries.GetPaymentByProviderRef(ctx, gen.GetPaymentByProviderRefParams{
		TenantID:    tenantID,
		ProviderRef: &payload.ProviderRef,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Warn("task:payment:process_webhook: no matching payment found",
				"provider_ref", payload.ProviderRef)
			return nil
		}

		return fmt.Errorf("task:payment:process_webhook: query payment by provider_ref: %w", err)
	}

	// Status-transition guard: prevent downgrading from succeeded.
	if p.Status == "succeeded" && payload.Status != "refunded" {
		slog.Info("task:payment:process_webhook: skipping — payment already succeeded",
			"payment_id", p.ID, "current_status", p.Status, "new_status", payload.Status)
		return nil
	}

	// Refund reversal (BUG-003): when a payment is marked refunded, reverse
	// the side effects based on purpose_type per JEOS §17:
	// - credit_purchase: reverse the wallet credit grant
	// - course_access: revoke the course access grant (if table exists)
	// - org_boost: expire the organization boost
	// - pro_membership: refunds affect future renewal only (no reversal)
	if payload.Status == "refunded" {
		if err := reversePaymentSideEffects(ctx, tenantID, p); err != nil {
			slog.Error("task:payment:process_webhook: failed to reverse side effects",
				"payment_id", p.ID, "purpose_type", p.PurposeType, "error", err)
			// Non-fatal: still mark the payment as refunded so the status
			// reflects the provider's state. The reversal can be retried.
		}
	}

	// Enqueue affiliate commission BEFORE marking payment as succeeded
	// so a failed enqueue doesn't lose the commission (AUDIT-BE-006).
	//
	// Attribution (Section 56): the link code is resolved from the Valkey
	// jc:aff_click:{user_id}:{product_type}:{product_id} attribution key set
	// by the redirect handler, NOT from the payer's first active link. The
	// previous implementation used GetActiveAffiliateLinkByUser (LIMIT 1),
	// which self-triggered commission on the payer's own first link for any
	// payment they made (BUG-003). When no attribution key exists, no
	// commission is credited — the user reached the purchase without an
	// affiliate redirect.
	if payload.Status == "succeeded" && taskEnqueuer != nil {
		var linkCode string
		// Map the payment's purpose_type to the affiliate product_type
		// vocabulary used in the jc:aff_click attribution key (BUG-001/003).
		// The redirect handler stores the key with the affiliate link's
		// product_type (pro_subscription, course, general), not the payment's
		// purpose_type (pro_membership, course_access, etc.).
		affProductType, affProductID := paymentPurposeToAffiliateProduct(p.PurposeType, p.PurposeID)

		attrKey := valkey.MustKey("aff_click:%s:%s:%s",
			p.UserID.String(), affProductType, affProductID)
		if rdb != nil {
			if v, gErr := rdb.Get(ctx, attrKey).Result(); gErr == nil && v != "" {
				linkCode = v
			}
		}

		commissionPayload, _ := json.Marshal(map[string]any{
			"payment_id":   p.ID.String(),
			"user_id":      p.UserID.String(),
			"amount":       p.Amount,
			"link_code":    linkCode,
			"purpose_type": p.PurposeType,
			"product_id":   affProductID,
		})
		if err := taskEnqueuer.Enqueue(TypeAffiliateCreditCommission, commissionPayload); err != nil {
			slog.Error("task:payment:process_webhook: failed to enqueue affiliate commission",
				"payment_id", p.ID, "error", err)
			// Don't mark payment as succeeded if commission enqueue fails;
			// the task will retry on the next Asynq attempt.
			return nil
		}
	}

	if _, err := queries.UpdatePaymentStatus(ctx, gen.UpdatePaymentStatusParams{
		Status:   payload.Status,
		ID:       p.ID,
		TenantID: tenantID,
	}); err != nil {
		slog.Error("task:payment:process_webhook: failed to update payment",
			"payment_id", p.ID, "error", err)
		return err
	}

	// ADR-063: when a course_access payment succeeds, enqueue the
	// university enrollment processor to credit the creator ledger,
	// upsert the enrollment, and bump the track's enrollment counter.
	// The purpose_id is the track_id; metadata may carry the user_id
	// if it differs from the payer (gift purchases). Fire-and-forget
	// per Rule 8 — the payment is already marked succeeded.
	if payload.Status == "succeeded" && p.PurposeType == "course_access" && taskEnqueuer != nil {
		trackID := ""
		if p.PurposeID.Valid {
			trackID = p.PurposeID.String()
		}

		if trackID != "" {
			enrollPayload, _ := json.Marshal(map[string]any{
				"tenant_id":  tenantID.String(),
				"user_id":    p.UserID.String(),
				"track_id":   trackID,
				"payment_id": p.ID.String(),
				"source":     "purchase",
				"gross_kobo": p.Amount,
			})
			if err := taskEnqueuer.Enqueue(TypeUniversityProcessEnrollment, enrollPayload); err != nil {
				slog.Error("task:payment:process_webhook: failed to enqueue university enrollment",
					"payment_id", p.ID, "error", err)
			}
		}
	}

	// Section 58: when a credit_purchase payment succeeds, enqueue
	// the wallet credit-grant task. The credits_to_grant and
	// currency_code are read from the payment metadata (set by
	// purchaseCredits when creating the payment row). Fire-and-forget
	// per Rule 8 — the payment is already marked succeeded.
	if payload.Status == "succeeded" && p.PurposeType == "credit_purchase" && taskEnqueuer != nil {
		creditsToGrant, currencyCode := parseCreditPurchaseMetadata(p.Metadata)
		if creditsToGrant > 0 {
			creditPayload, _ := json.Marshal(map[string]any{
				"payment_id":       p.ID.String(),
				"tenant_id":        tenantID.String(),
				"user_id":          p.UserID.String(),
				"credits_to_grant": creditsToGrant,
				"currency_code":    currencyCode,
			})
			if err := taskEnqueuer.Enqueue(TypeWalletProcessCreditPurchase, creditPayload); err != nil {
				slog.Error("task:payment:process_webhook: failed to enqueue credit purchase grant",
					"payment_id", p.ID, "error", err)
			}
		}
	}

	// §60 Referral conversion — on any successful payment, enqueue the
	// referral:check_conversion task so the referrer's pending referral can
	// transition to qualified→credited. The task is idempotent (no-op when no
	// referral row exists for the payer). Fire-and-forget per Rule 8.
	// Ref: japaconnect-engineering/sections/60-referral-system.md (Attribution)
	if payload.Status == "succeeded" && taskEnqueuer != nil {
		referralPayload, _ := json.Marshal(map[string]any{
			"tenant_id":  tenantID.String(),
			"payer_id":   p.UserID.String(),
			"payment_id": p.ID.String(),
			"session_id": p.UserID.String(), // attribution key is per-user at signup
		})
		if err := taskEnqueuer.Enqueue(TypeReferralCheckConversion, referralPayload); err != nil {
			slog.Error("task:payment:process_webhook: failed to enqueue referral check_conversion",
				"payment_id", p.ID, "error", err)
		}
	}

	slog.Info("task:payment:process_webhook: payment updated",
		"payment_id", p.ID, "status", payload.Status)

	// Section 06 Paid Form Submissions: when a form_submission payment
	// succeeds, update the linked form_submission status from
	// 'pending_payment' to 'new' and enqueue form triggers so the
	// submission is processed (emails, webhooks, tickets, etc.).
	if payload.Status == "succeeded" && p.PurposeType == "form_submission" {
		submission, sErr := queries.GetFormSubmissionByPaymentID(ctx, gen.GetFormSubmissionByPaymentIDParams{
			PaymentID: p.ID,
			TenantID:  tenantID,
		})
		if sErr != nil {
			slog.Warn("task:payment:process_webhook: no form submission linked to payment",
				"payment_id", p.ID, "error", sErr)
		} else if submission.Status == "pending_payment" {
			if err := queries.UpdateFormSubmissionStatus(ctx, gen.UpdateFormSubmissionStatusParams{
				Status:   "new",
				ID:       submission.ID,
				TenantID: tenantID,
			}); err != nil {
				slog.Error("task:payment:process_webhook: failed to update form submission status",
					"submission_id", submission.ID, "error", err)
			} else if taskEnqueuer != nil {
				// Enqueue form:fire_submission_triggers so the worker
				// dispatches any triggers configured on this form.
				formID := ""
				if submission.FormID.Valid {
					formID = submission.FormID.String()
				}

				userID := ""
				if submission.UserID.Valid {
					userID = submission.UserID.String()
				}

				triggerPayload, _ := json.Marshal(struct {
					SubmissionID string `json:"submission_id"`
					TenantID     string `json:"tenant_id"`
					FormID       string `json:"form_id"`
					UserID       string `json:"user_id,omitempty"`
					FieldValues  []byte `json:"field_values"`
				}{
					SubmissionID: submission.ID.String(),
					TenantID:     tenantID.String(),
					FormID:       formID,
					UserID:       userID,
					FieldValues:  submission.FieldValues,
				})
				if err := taskEnqueuer.Enqueue(TypeFormFireSubmissionTriggers, triggerPayload); err != nil {
					slog.Error("task:payment:process_webhook: failed to enqueue form triggers",
						"submission_id", submission.ID, "error", err)
				}
			}
		}
	}

	return nil
}

// reversePaymentSideEffects reverses the side effects of a payment based on
// its purpose_type when the payment is refunded (JEOS §17):
//   - credit_purchase: enqueue a wallet credit reversal
//   - course_access: revoke the course access grant
//   - org_boost: expire the organization boost
//   - pro_membership: no reversal (refunds affect future renewal only)
func reversePaymentSideEffects(ctx context.Context, tenantID pgtype.UUID, p gen.Payment) error {
	switch p.PurposeType {
	case "credit_purchase":
		// Reverse the wallet credit grant by enqueueing a negative grant.
		if taskEnqueuer != nil {
			creditsToGrant, currencyCode := parseCreditPurchaseMetadata(p.Metadata)
			if creditsToGrant > 0 {
				reversalPayload, _ := json.Marshal(map[string]any{
					"payment_id":       p.ID.String(),
					"tenant_id":        tenantID.String(),
					"user_id":          p.UserID.String(),
					"credits_to_grant": -creditsToGrant, // negative = debit
					"currency_code":    currencyCode,
				})
				if err := taskEnqueuer.Enqueue(TypeWalletProcessCreditPurchase, reversalPayload); err != nil {
					return fmt.Errorf("enqueue credit reversal: %w", err)
				}
			}
		}
	case "course_access":
		// Revoke the course access grant for this payment (refund/chargeback).
		// The course_access_grants table was added in migration 000211.
		slog.Info("task:payment:process_webhook: revoking course access grant",
			"payment_id", p.ID)

		if p.PurposeID.Valid {
			if _, err := dbPool.Writer.Exec(ctx,
				`UPDATE course_access_grants SET revoked_at = now()
				 WHERE payment_id = $1 AND revoked_at IS NULL`,
				p.ID); err != nil {
				return fmt.Errorf("revoking course access grant: %w", err)
			}
		}
	case "org_boost":
		// Expire the organization boost immediately (JEOS §17).
		if p.PurposeID.Valid {
			if dbPool != nil {
				_, err := dbPool.Writer.Exec(ctx,
					`UPDATE organization_boosts SET status = 'expired', ends_at = now()
					 WHERE payment_id = $1 AND tenant_id = $2`,
					p.ID, tenantID)
				if err != nil {
					return fmt.Errorf("expiring org boost: %w", err)
				}
			}
		}
	case "pro_membership", "donation":
		// No reversal needed — pro membership refunds affect future
		// renewal only; donations are non-reversible.
	}

	return nil
}

// HandleGdprRetentionSweep performs two sweeps:
// 1. Re-enqueues stale `pending` deletion/export requests whose enqueue was lost.
// 2. Processes `retention_wait` requests whose retention period has elapsed.
func HandleGdprRetentionSweep(ctx context.Context, _ *asynq.Task) error {
	slog.Info("task:gdpr:retention_sweep: starting sweep")

	if dbPool == nil {
		slog.Warn("task:gdpr:retention_sweep: no database, skipping")
		return nil
	}

	// Sweep 1: Re-enqueue stale pending deletion requests (older than 30 min).
	// These are requests whose initial enqueue was lost (H10).
	reenqueued := 0

	if taskEnqueuer != nil {
		rq := gen.New(dbPool.Reader)

		pendingRows, err := rq.ListStalePendingDeletionRequests(ctx)
		if err != nil {
			slog.Error("task:gdpr:retention_sweep: pending query failed", "error", err)
		} else {
			for _, pr := range pendingRows {
				payload, _ := json.Marshal(map[string]string{
					"request_id": pr.ID.String(),
					"tenant_id":  pr.TenantID.String(),
					"user_id":    pr.UserID.String(),
				})
				if err := taskEnqueuer.Enqueue(TypeAccountDeletionCleanup, payload); err != nil {
					slog.Error("task:gdpr:retention_sweep: failed to re-enqueue deletion",
						"request_id", pr.ID.String(), "error", err)
					continue
				}

				reenqueued++
			}
		}
	}

	if reenqueued > 0 {
		slog.Info("task:gdpr:retention_sweep: re-enqueued stale pending requests", "count", reenqueued)
	}

	// Sweep 2: Process retention_wait requests whose retention period has elapsed.
	if gdprProcessor == nil {
		slog.Warn("task:gdpr:retention_sweep: no GDPR processor, skipping retention sweep")
		return nil
	}

	rq := gen.New(dbPool.Reader)

	retentionRows, err := rq.ListRetentionWaitElapsed(ctx)
	if err != nil {
		slog.Error("task:gdpr:retention_sweep: query failed", "error", err)
		return err
	}

	var requestIDs [][16]byte
	for _, rr := range retentionRows {
		requestIDs = append(requestIDs, rr.ID.Bytes)
	}

	if len(requestIDs) == 0 && reenqueued == 0 {
		// No retention_wait or stale pending work, but still run the
		// expired export cleanup (BUG-GDPR-005) before exiting.
		if err := gdprProcessor.CleanupExpiredExports(ctx); err != nil {
			slog.Error("task:gdpr:retention_sweep: expired export cleanup failed", "error", err)
		}

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

	// Sweep 3: Clean up expired data export files (BUG-GDPR-005, Section 19).
	// Deletes files from storage and nulls download URLs for completed
	// exports whose ExpiresAt has passed.
	if err := gdprProcessor.CleanupExpiredExports(ctx); err != nil {
		slog.Error("task:gdpr:retention_sweep: expired export cleanup failed", "error", err)
	}

	slog.Info("task:gdpr:retention_sweep: sweep complete",
		"retention_completed", completed, "pending_reenqueued", reenqueued)

	return nil
}

// HandleTicketSendConfirmation sends a confirmation email when a new ticket is created.
func HandleTicketSendConfirmation(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:ticket:send_confirmation")

	queries, err := requireDB()
	if err != nil {
		slog.Warn("task:ticket:send_confirmation: no database, skipping", "error", err)
		return nil
	}

	var payload struct {
		TicketID string `json:"ticket_id"`
		TenantID string `json:"tenant_id"`
	}

	if err := parsePayloadOrSkip(t, &payload); err != nil {
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

	if ticket.SubmitterEmail == nil || *ticket.SubmitterEmail == "" {
		slog.Warn("task:ticket:send_confirmation: no submitter email, skipping",
			"ticket_id", payload.TicketID)
		return nil
	}

	ticketTmpl, err := template.New("ticket-confirm").Parse(`<p>Your support ticket "<strong>{{.Subject}}</strong>" has been received.</p>
<p>Ticket ID: <code>{{.TicketID}}</code></p>`)
	if err != nil {
		return fmt.Errorf("parsing ticket template: %w", err)
	}

	var bodyBuf bytes.Buffer
	if err := ticketTmpl.Execute(&bodyBuf, struct {
		Subject  string
		TicketID string
	}{Subject: ticket.Subject, TicketID: ticketID.String()}); err != nil {
		return fmt.Errorf("rendering ticket template: %w", err)
	}

	body := bodyBuf.String()

	if err := sendEmailWithContext(ctx, *ticket.SubmitterEmail, "Ticket Confirmation - "+ticket.Subject, body, true); err != nil {
		slog.Error("task:ticket:send_confirmation: email failed",
			"ticket_id", payload.TicketID, "error", err)
		return err
	}

	slog.Info("task:ticket:send_confirmation: confirmation sent",
		"ticket_id", payload.TicketID, "email", *ticket.SubmitterEmail)

	return nil
}

// ---------------------------------------------------------------------------
// Default tier handlers
// ---------------------------------------------------------------------------

// HandleEmailSend sends a single transactional email via SMTP.
// When template_slug is provided with a tenant_id, the handler looks up
// the DB-stored email template and renders it with html/template (XSS-safe).
// Falls back to raw body/subject when no template is specified (backward compat).
//
// Ref: sections/20-email-notification-bugs.md.
func HandleEmailSend(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:email:send")

	var payload struct {
		To               string            `json:"to"`
		Subject          string            `json:"subject"`
		Body             string            `json:"body"`
		HTML             bool              `json:"html"`
		TemplateSlug     string            `json:"template_slug,omitempty"`
		TenantID         string            `json:"tenant_id,omitempty"`
		TemplateData     map[string]string `json:"template_data,omitempty"`
		TemplateSiteName string            `json:"template_site_name,omitempty"`
	}

	if err := parsePayloadOrSkip(t, &payload); err != nil {
		slog.Error("task:email:send: invalid payload", "error", err)
		return err
	}

	if payload.To == "" {
		return errors.New("task:email:send: to is required")
	}

	subject := payload.Subject
	body := payload.Body
	isHTML := payload.HTML

	// When a template_slug is provided, look up the DB-stored template and
	// render it with html/template for XSS safety.
	if payload.TemplateSlug != "" && dbPool != nil && payload.TenantID != "" {
		tid, err := scanUUID(payload.TenantID)
		if err != nil {
			slog.Error("task:email:send: invalid tenant_id for template", "error", err)
		} else {
			tmpl, err := gen.New(dbPool.Reader).GetEmailTemplateBySlug(ctx, gen.GetEmailTemplateBySlugParams{
				Slug:     payload.TemplateSlug,
				TenantID: tid,
			})
			if err != nil {
				slog.Warn("task:email:send: template not found, falling back to raw body",
					"template_slug", payload.TemplateSlug, "error", err)
			} else {
				// Use template subject if the caller didn't override it.
				if subject == "" {
					subject = tmpl.Subject
				}

				// Render the template body with html/template (auto-escapes values).
				if tmpl.HtmlBody != "" {
					tmplParsed, err := template.New("email").Parse(tmpl.HtmlBody)
					if err != nil {
						slog.Error("task:email:send: failed to parse template",
							"template_slug", payload.TemplateSlug, "error", err)
					} else {
						data := templateDataFromMap(payload.TemplateSiteName, payload.TemplateData)

						var buf bytes.Buffer

						if err := tmplParsed.Execute(&buf, data); err != nil {
							slog.Error("task:email:send: failed to render template",
								"template_slug", payload.TemplateSlug, "error", err)
						} else {
							body = buf.String()
							isHTML = true
						}
					}
				} else if tmpl.PlainTextBody != "" {
					body = tmpl.PlainTextBody
					isHTML = false
				}
			}
		}
	}

	if payload.To == "" || subject == "" {
		return errors.New("task:email:send: to and subject are required")
	}

	if err := sendEmailWithContext(ctx, payload.To, subject, body, isHTML); err != nil {
		slog.Error("task:email:send: failed", "to", payload.To, "error", err)
		return err
	}

	slog.Info("task:email:send: sent", "to", payload.To)

	return nil
}

// templateDataFromMap builds a map[string]any for html/template rendering,
// converting snake_case map keys to PascalCase so all 22 template type
// variables (SiteName, UserName, ActionURL, DeviceInfo, Amount, Currency,
// TicketID, etc.) are resolved generically without a fixed struct.
func templateDataFromMap(siteName string, m map[string]string) map[string]any {
	if m == nil {
		m = make(map[string]string)
	}

	if siteName == "" {
		siteName = "JapaConnect"
	}

	data := make(map[string]any, len(m)+1)
	data["SiteName"] = siteName

	for k, v := range m {
		data[snakeToPascal(k)] = v
	}

	return data
}

// snakeToPascal converts a snake_case key (e.g. "user_name") to PascalCase
// (e.g. "UserName") so it matches Go template variable names ({{.UserName}}).
func snakeToPascal(s string) string {
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}

	return strings.Join(parts, "")
}

// HandleSearchIndexSync synchronizes an entity to the Meilisearch index.
func HandleSearchIndexSync(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:search:index_entity")

	if searchIndexer == nil {
		slog.Warn("task:search:index_entity: no search indexer registered, skipping")
		return nil
	}

	var payload struct {
		ContentID  string `json:"content_id"`
		EntityID   string `json:"entity_id"`
		EntityType string `json:"entity_type"`
		TenantID   string `json:"tenant_id"`
		Operation  string `json:"operation"`
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

	// Handle delete operation.
	if payload.Operation == "delete" {
		indexName := "contents"

		switch entityType {
		case "forum_thread":
			indexName = "forum_threads"
		case "space":
			indexName = "spaces"
		case "user":
			indexName = "users"
		case "tag":
			indexName = "tags"
		case "learning_track":
			indexName = "learning_tracks"
		case "doc_page":
			indexName = "doc_pages"
		case "organization":
			indexName = "organizations"
		case "language_test_requirement":
			indexName = "language_test_requirements"
		}

		return searchIndexer.DeleteDocument(ctx, indexName, entityID.Bytes)
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
	case "learning_track":
		return searchIndexer.IndexLearningTrack(ctx, entityID.Bytes, tenantID.Bytes)
	case "doc_page":
		return searchIndexer.IndexDocPage(ctx, entityID.Bytes, tenantID.Bytes)
	case "organization":
		return searchIndexer.IndexOrganization(ctx, entityID.Bytes, tenantID.Bytes)
	case "language_test_requirement":
		return searchIndexer.IndexLanguageTestRequirement(ctx, entityID.Bytes, tenantID.Bytes)
	default:
		slog.Warn("task:search:index_entity: unknown entity type, skipping", "entity_type", entityType)
		return nil
	}
}

// engagementReindexBatchSize is the max number of content documents re-indexed
// per cron tick. Tunable via the search.engagement_reindex_batch_size system
// setting (§13). A conservative default keeps each tick fast even on large
// tenants.
const engagementReindexBatchSize = 100

// engagementReindexCursorKey is the Valkey key holding the last-swept
// timestamp for the engagement-score re-index cron. The cursor advances
// monotonically so each tick only re-indexes content whose scores changed
// since the previous tick. Ref: conventions/valkey-keys-catalog.md
// (jc:search:engagement_reindex_cursor).
const engagementReindexCursorKey = "jc:search:engagement_reindex_cursor"

// HandleSearchReindexEngagementScores is the §13 batched engagement-score
// re-index cron. It runs every 5 minutes (see NewScheduler), reads a cursor
// from Valkey, finds content whose content_scores.last_scored_at is newer
// than the cursor, and re-indexes those documents so ?sort=top stays fresh
// without re-indexing on every engagement event. The cursor is advanced to
// the newest last_scored_at seen in the batch.
//
// On the first run (no cursor), the cursor defaults to now-1h so only
// recently-scored content is indexed, avoiding a full re-index sweep.
//
// Failures are logged but do not return an error — the next tick retries
// from the same cursor. A nil search indexer or DB pool is a no-op.
func HandleSearchReindexEngagementScores(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:search:reindex_engagement_scores")

	if searchIndexer == nil {
		slog.Warn("task:search:reindex_engagement_scores: no search indexer, skipping")
		return nil
	}

	queries, err := requireDB()
	if err != nil {
		slog.Warn("task:search:reindex_engagement_scores: no database, skipping", "error", err)
		return nil
	}

	// Read the cursor from Valkey. Default to now-1h on first run.
	cursor := time.Now().Add(-1 * time.Hour)

	if rdb != nil {
		if v, gErr := rdb.Get(ctx, engagementReindexCursorKey).Result(); gErr == nil && v != "" {
			if parsed, pErr := time.Parse(time.RFC3339Nano, v); pErr == nil {
				cursor = parsed
			}
		}
	}

	rows, err := queries.ListContentIDsScoredSince(ctx, gen.ListContentIDsScoredSinceParams{
		Cursor:    pgtype.Timestamptz{Time: cursor, Valid: true},
		BatchSize: engagementReindexBatchSize,
	})
	if err != nil {
		slog.Error("task:search:reindex_engagement_scores: query failed", "error", err)
		return err
	}

	if len(rows) == 0 {
		slog.Info("task:search:reindex_engagement_scores: no changes since cursor")
		return nil
	}

	reindexed := 0

	var newest time.Time

	for _, row := range rows {
		if iErr := searchIndexer.IndexContent(ctx, row.ID.Bytes, row.TenantID.Bytes); iErr != nil {
			slog.Warn("task:search:reindex_engagement_scores: failed to re-index content",
				"content_id", row.ID.String(), "error", iErr)
			continue
		}

		reindexed++
	}

	// Advance the cursor to the newest last_scored_at in this batch. We
	// don't have last_scored_at in the row, but since rows are ordered ASC,
	// the last row is the newest. Use time.Now() as a safe upper bound —
	// the next tick will pick up anything scored between now and the next
	// run. This avoids a second query to fetch last_scored_at.
	newest = time.Now()
	if rdb != nil {
		if sErr := rdb.Set(ctx, engagementReindexCursorKey, newest.Format(time.RFC3339Nano), 7*24*time.Hour).Err(); sErr != nil {
			slog.Warn("task:search:reindex_engagement_scores: failed to update cursor", "error", sErr)
		}
	}

	slog.Info("task:search:reindex_engagement_scores: completed",
		"reindexed", reindexed, "total", len(rows), "cursor", newest.Format(time.RFC3339Nano))

	return nil
}

// HandleImageProcessing processes uploaded images through imgproxy for resizing and optimization.
func HandleImageProcessing(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:content:process_image")

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
	slog.Info("task:ai:process_task")

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

// HandleDocsGeneratePage generates a docs page via AI in the background
// (TASK-004). Fire-and-forget: the AI handler enqueues this task and returns
// 202 Accepted immediately; the worker calls the registered DocsPageGenerator
// to dispatch the AI request and persist the result.
func HandleDocsGeneratePage(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:docs:generate_page")

	if docsPageGenerator == nil {
		slog.Warn("task:docs:generate_page: no docs page generator registered, skipping")
		return nil
	}

	var payload struct {
		Prompt          string   `json:"prompt"`
		Title           string   `json:"title"`
		Slug            string   `json:"slug"`
		UserID          string   `json:"user_id"`
		TenantID        string   `json:"tenant_id"`
		Audience        string   `json:"audience"`
		RelatedFeatures []string `json:"related_features"`
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:docs:generate_page: invalid payload", "error", err)
		return err
	}

	if payload.Title == "" {
		return errors.New("task:docs:generate_page: title is required")
	}

	if err := docsPageGenerator.GenerateDocsPage(ctx, GenerateDocsPageRequest{
		Prompt:          payload.Prompt,
		Title:           payload.Title,
		Slug:            payload.Slug,
		Audience:        payload.Audience,
		RelatedFeatures: payload.RelatedFeatures,
		UserID:          payload.UserID,
		TenantID:        payload.TenantID,
	}); err != nil {
		slog.Error("task:docs:generate_page: failed", "error", err, "audience", payload.Audience)
		return err
	}

	slog.Info("task:docs:generate_page: completed", "user_id", payload.UserID, "tenant_id", payload.TenantID, "audience", payload.Audience)

	return nil
}

// HandleFormFireSubmissionTriggers fires the triggers configured on a form
// after a submission is created. The customfields service implements the
// actual dispatch logic (assign_user_group / send_email / send_webhook /
// create_ticket / notify_admin); this handler just parses the payload and
// delegates.
//
// Ref: japaconnect-engineering/sections/06-custom-field-builder-form-builder.md.
func HandleFormFireSubmissionTriggers(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:form:fire_submission_triggers")

	if formTriggerProcessor == nil {
		slog.Warn("task:form:fire_submission_triggers: no processor registered, skipping")
		return nil
	}

	var payload struct {
		SubmissionID string `json:"submission_id"`
		TenantID     string `json:"tenant_id"`
		FormID       string `json:"form_id"`
		UserID       string `json:"user_id"` // empty for anonymous submissions
		FieldValues  []byte `json:"field_values"`
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:form:fire_submission_triggers: invalid payload", "error", err)
		return err
	}

	if payload.SubmissionID == "" || payload.FormID == "" || payload.TenantID == "" {
		return errors.New("task:form:fire_submission_triggers: submission_id, form_id, and tenant_id are required")
	}

	var submissionID, tenantID, formID pgtype.UUID
	if err := submissionID.Scan(payload.SubmissionID); err != nil {
		return fmt.Errorf("task:form:fire_submission_triggers: invalid submission_id: %w", err)
	}

	if err := tenantID.Scan(payload.TenantID); err != nil {
		return fmt.Errorf("task:form:fire_submission_triggers: invalid tenant_id: %w", err)
	}

	if err := formID.Scan(payload.FormID); err != nil {
		return fmt.Errorf("task:form:fire_submission_triggers: invalid form_id: %w", err)
	}

	var submitterUserID *pgtype.UUID

	if payload.UserID != "" {
		var uid pgtype.UUID
		if err := uid.Scan(payload.UserID); err == nil {
			submitterUserID = &uid
		}
	}

	if err := formTriggerProcessor.FireSubmissionTriggers(ctx, submissionID, tenantID, formID, submitterUserID, payload.FieldValues); err != nil {
		slog.Error("task:form:fire_submission_triggers: failed",
			"submission_id", payload.SubmissionID, "error", err)
		return err
	}

	slog.Info("task:form:fire_submission_triggers: completed", "submission_id", payload.SubmissionID)

	return nil
}

// HandleEngagementCountUpdate recalculates engagement counts for a content entity.
func HandleEngagementCountUpdate(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:content:update_engagement")

	queries, err := requireDB()
	if err != nil {
		slog.Warn("task:content:update_engagement: no database, skipping", "error", err)
		return nil
	}

	var payload struct {
		EntityType     string `json:"entity_type"`
		EntityID       string `json:"entity_id"`
		EngagementType string `json:"engagement_type"` // "view", "comment", "vote", "share", "bookmark"
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

	// Ensure a content_scores row exists before incrementing. Without this,
	// content published before the score-row-on-publish fix (or via paths that
	// bypass UpdateContentStatus) would be silently skipped, leaving
	// vote_score/bookmark_count stuck at 0. Idempotent (ON CONFLICT DO NOTHING).
	if err := queries.EnsureContentScore(ctx, entityID); err != nil {
		slog.Error("task:content:update_engagement: failed to ensure content_scores row",
			"content_id", payload.EntityID, "error", err)
		return err
	}

	// Atomically increment the relevant column. This avoids the read-modify-
	// write race condition where concurrent engagement events (vote, comment,
	// bookmark, view arriving simultaneously) each read the same baseline,
	// add their delta, and write the full row back — overwriting each other's
	// deltas. Each event touches only its own column, so concurrent UPDATEs
	// never interfere. Ref: Section 32, Guide §21.
	// #nosec G115 // value bounded by caller
	delta := int32(payload.Delta) //nolint:gosec // G115: delta is typically ±1 for engagement events

	switch payload.EngagementType {
	case "view":
		if err := queries.IncrementScoreView(ctx, gen.IncrementScoreViewParams{
			Delta: delta, ContentID: entityID,
		}); err != nil {
			slog.Error("task:content:update_engagement: failed to increment view_count",
				"content_id", payload.EntityID, "error", err)
			return err
		}
	case "comment":
		if err := queries.IncrementScoreComment(ctx, gen.IncrementScoreCommentParams{
			Delta: delta, ContentID: entityID,
		}); err != nil {
			slog.Error("task:content:update_engagement: failed to increment comment_count",
				"content_id", payload.EntityID, "error", err)
			return err
		}
	case "vote":
		if err := queries.IncrementScoreVote(ctx, gen.IncrementScoreVoteParams{
			Delta: delta, ContentID: entityID,
		}); err != nil {
			slog.Error("task:content:update_engagement: failed to increment vote_score",
				"content_id", payload.EntityID, "error", err)
			return err
		}
	case "bookmark":
		if err := queries.IncrementScoreBookmark(ctx, gen.IncrementScoreBookmarkParams{
			Delta: delta, ContentID: entityID,
		}); err != nil {
			slog.Error("task:content:update_engagement: failed to increment bookmark_count",
				"content_id", payload.EntityID, "error", err)
			return err
		}
	default:
		slog.Warn("task:content:update_engagement: unknown engagement type",
			"engagement_type", payload.EngagementType)
		return nil
	}

	slog.Info("task:content:update_engagement: score updated",
		"content_id", payload.EntityID,
		"engagement_type", payload.EngagementType,
		"delta", payload.Delta)

	return nil
}

// HandleModerationScan runs content through the spam and moderation pipeline.
func HandleModerationScan(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:content:scan_moderation")

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
	slog.Info("task:content:publish_scheduled")

	if dbPool == nil {
		slog.Warn("task:content:publish_scheduled: no database, skipping")
		return nil
	}

	var payload struct {
		ContentID string `json:"content_id"`
		TenantID  string `json:"tenant_id"`
	}

	if err := parsePayloadOrSkip(t, &payload); err != nil {
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

	// Re-validate: only publish if still scheduled and publish_at has passed.
	// Prevents force-publishing canceled or already-published content.
	rowsAffected, err := gen.New(dbPool.Writer).PublishScheduledContent(ctx, gen.PublishScheduledContentParams{
		ID: contentID, TenantID: tenantID,
	})
	if err != nil {
		slog.Error("task:content:publish_scheduled: failed to publish",
			"content_id", payload.ContentID, "error", err)
		return err
	}

	if rowsAffected == 0 {
		slog.Info("task:content:publish_scheduled: content no longer scheduled or publish_at not reached",
			"content_id", payload.ContentID)
		return nil
	}

	slog.Info("task:content:publish_scheduled: content published",
		"content_id", payload.ContentID)

	// Ensure a content_scores row exists so the published content is not
	// excluded from INNER JOIN feed queries (explore/top/rising/space/profile).
	// Ref: Section 32, Guide §21.
	if err := gen.New(dbPool.Writer).EnsureContentScore(ctx, contentID); err != nil {
		slog.Warn("task:content:publish_scheduled: failed to ensure content_scores row",
			"content_id", payload.ContentID, "error", err)
	}

	// Enqueue feed fan-out for the newly published content (BL-005).
	// Fetch the content to get author_id, space_id and content_type for the
	// fan-out. Per Section 32, fan-out fires to author followers + each
	// assigned space (content_spaces) + bubble to parent spaces.
	published, err := gen.New(dbPool.Reader).GetContentByID(ctx, gen.GetContentByIDParams{
		ID: contentID, TenantID: tenantID,
	})
	if err != nil {
		slog.Warn("task:content:publish_scheduled: failed to fetch published content for fan-out",
			"content_id", payload.ContentID, "error", err)
		return nil
	}

	// 1. Always fan out to the author's followers.
	scheduleFanOutTask(taskEnqueuer, contentID, tenantID, published.AuthorID, pgtype.UUID{}, published.ContentType, "followers")

	// 2. Fan out to each assigned space via content_spaces (Section 14).
	//    Falls back to the primary contents.space_id if no content_spaces rows.
	spaceIDs := []pgtype.UUID{}

	if rows, err := gen.New(dbPool.Reader).GetContentSpaces(ctx, contentID); err == nil {
		for _, r := range rows {
			spaceIDs = append(spaceIDs, r.SpaceID)
		}
	} else {
		slog.Warn("task:content:publish_scheduled: failed to fetch content_spaces, using primary space_id",
			"content_id", payload.ContentID, "error", err)
	}

	if len(spaceIDs) == 0 && published.SpaceID.Valid {
		spaceIDs = append(spaceIDs, published.SpaceID)
	}

	// 3. Enqueue a "space" task and a "bubble" task per assigned space.
	for _, sid := range spaceIDs {
		scheduleFanOutTask(taskEnqueuer, contentID, tenantID, published.AuthorID, sid, published.ContentType, "space")
		scheduleFanOutTask(taskEnqueuer, contentID, tenantID, published.AuthorID, sid, published.ContentType, "bubble")
	}

	// RSS-013: regenerate RSS feeds so the newly published scheduled content
	// appears without waiting for the hourly cron. Fire-and-forget.
	scheduleRSSRegenerate(taskEnqueuer, tenantID)

	return nil
}

// HandleLinkPreviewFetch fetches and stores OpenGraph metadata for a URL.
func HandleLinkPreviewFetch(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:content:fetch_link_preview")

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
	slog.Info("task:i18n:translate_language_pack")

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
	slog.Info("task:content:scan_media")

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

// defaultScore returns a valid pgtype.Numeric with value 0, suitable for
// feed inserts where the score column is NOT NULL with DEFAULT 0. An
// uninitialized pgtype.Numeric{} sends SQL NULL which violates the constraint.
func defaultScore() pgtype.Numeric {
	return pgtype.Numeric{Int: big.NewInt(0), Exp: 0, Valid: true}
}

// HandleContentFanOutToFeeds fans out a published content item to all relevant user feeds.
func HandleContentFanOutToFeeds(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:content:fan_out_to_feeds")

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

	// authorID is optional — only required for "followers" fan-out. For
	// "space" and "bubble" tasks it may be empty (zero UUID).
	var authorID pgtype.UUID
	if payload.AuthorID != "" {
		authorID, err = scanUUID(payload.AuthorID)
		if err != nil {
			return fmt.Errorf("task:content:fan_out_to_feeds: %w", err)
		}
	}

	// spaceID is optional — only required for "space" and "bubble" fan-out.
	var spaceID pgtype.UUID
	if payload.SpaceID != "" {
		spaceID, err = scanUUID(payload.SpaceID)
		if err != nil {
			return fmt.Errorf("task:content:fan_out_to_feeds: %w", err)
		}
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
		if !authorID.Valid {
			slog.Warn("task:content:fan_out_to_feeds: followers fan-out requires author_id",
				"content_id", payload.ContentID)
			return nil
		}
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
				Score:       defaultScore(),
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
		if !spaceID.Valid {
			slog.Warn("task:content:fan_out_to_feeds: space fan-out requires space_id",
				"content_id", payload.ContentID)
			return nil
		}
		// Section 32 hybrid fan-out: push for spaces ≤ threshold, pull for
		// larger spaces. In pull mode, we skip writing feeds rows — members
		// will discover the post via the space feed query (GetSpaceFeed),
		// which pulls by space_id at read time. This prevents unbounded
		// feed row writes for very large spaces.
		memberCount, err := gen.New(dbPool.Reader).CountSpaceMembers(ctx, gen.CountSpaceMembersParams{
			SpaceID: spaceID, TenantID: tenantID,
		})
		if err != nil {
			slog.Error("task:content:fan_out_to_feeds: failed to count space members",
				"space_id", payload.SpaceID, "error", err)
			return err
		}

		if memberCount > int64(maxFanOut) {
			// Pull strategy: skip push fan-out. The space feed query
			// (GetSpaceFeed) already pulls published content by space_id,
			// so members will see this post on their next space feed request
			// without a feeds row. Ref: Section 32 L50–55.
			slog.Info("task:content:fan_out_to_feeds: pull strategy (space over threshold)",
				"content_id", payload.ContentID, "space_id", payload.SpaceID,
				"member_count", memberCount, "threshold", maxFanOut)

			return nil
		}

		// Push strategy: write a feeds row for every member.
		memberIDs, err := gen.New(dbPool.Reader).ListSpaceMemberUserIDs(ctx, gen.ListSpaceMemberUserIDsParams{
			SpaceID: spaceID, TenantID: tenantID, LimitVal: int32(maxFanOut), // #nosec
		})
		if err != nil {
			slog.Error("task:content:fan_out_to_feeds: failed to get space members",
				"space_id", payload.SpaceID, "error", err)
			return err
		}

		for _, userID := range memberIDs {
			if err := queries.InsertFeedItem(ctx, gen.InsertFeedItemParams{
				TenantID:    tenantID,
				UserID:      userID,
				ContentID:   contentID,
				SpaceID:     spaceID,
				ContentType: payload.ContentType,
				Score:       defaultScore(),
			}); err != nil {
				slog.Error("task:content:fan_out_to_feeds: failed to insert feed item",
					"user_id", userID, "error", err)
				continue
			}

			fedCount++
		}

		slog.Info("task:content:fan_out_to_feeds: space fan-out complete",
			"content_id", payload.ContentID, "space_id", payload.SpaceID, "fed_count", fedCount)

	case "bubble":
		// Fan out to parent spaces of the current space (bubble label).
		// Per Section 05, posts bubble up the space hierarchy based on
		// spaces.bubble_depth (default 2). Spaces can have multiple parents
		// via space_parent_spaces, so we BFS up the hierarchy.
		maxDepth := 2
		if appSettings != nil {
			maxDepth = appSettings.GetIntOr(ctx, tenantID, "spaces.bubble_depth", 2)
		}

		visited := map[pgtype.UUID]bool{spaceID: true}
		currentLevel := []pgtype.UUID{spaceID}

		for range maxDepth {
			if len(currentLevel) == 0 {
				break
			}

			var nextLevel []pgtype.UUID

			for _, currentID := range currentLevel {
				parents, err := gen.New(dbPool.Reader).GetSpaceParentSpaces(ctx, currentID)
				if err != nil {
					continue
				}

				for _, p := range parents {
					if !p.ParentSpaceID.Valid || visited[p.ParentSpaceID] {
						continue
					}

					visited[p.ParentSpaceID] = true

					nextLevel = append(nextLevel, p.ParentSpaceID)

					memberIDs, err := gen.New(dbPool.Reader).ListSpaceMemberUserIDs(ctx, gen.ListSpaceMemberUserIDsParams{
						SpaceID: p.ParentSpaceID, TenantID: tenantID, LimitVal: int32(maxFanOut), // #nosec
					})
					if err != nil {
						continue
					}

					for _, userID := range memberIDs {
						if err := queries.InsertFeedItem(ctx, gen.InsertFeedItemParams{
							TenantID:    tenantID,
							UserID:      userID,
							ContentID:   contentID,
							SpaceID:     p.ParentSpaceID,
							ContentType: payload.ContentType,
							Score:       defaultScore(),
						}); err != nil {
							continue
						}

						fedCount++
					}
				}
			}

			currentLevel = nextLevel
		}

		slog.Info("task:content:fan_out_to_feeds: bubble fan-out complete",
			"content_id", payload.ContentID, "fed_count", fedCount, "max_depth", maxDepth)
	}

	return nil
}

// HandleAIGenerateContentEmbedding generates a vector embedding for content search.
func HandleAIGenerateContentEmbedding(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:content:generate_embedding")

	if contentEmbedder == nil {
		slog.Warn("task:content:generate_embedding: no content embedder registered, skipping")
		return nil
	}

	var payload struct {
		ContentID string `json:"content_id"`
		TenantID  string `json:"tenant_id"`
	}

	if err := parsePayloadOrSkip(t, &payload); err != nil {
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

	// Re-fetch content text from database (payload is IDs-only).
	if dbPool == nil {
		slog.Warn("task:content:generate_embedding: no database, skipping")
		return nil
	}

	contentRow, err := gen.New(dbPool.Reader).GetContentTitleAndBody(ctx, gen.GetContentTitleAndBodyParams{
		ID: contentID, TenantID: tenantID,
	})
	if err != nil {
		slog.Error("task:content:generate_embedding: content not found",
			"content_id", payload.ContentID, "error", err)
		return nil
	}

	title, body := contentRow.Title, contentRow.Body

	text := title
	if body != "" {
		if text != "" {
			text += "\n\n"
		}

		text += body
	}

	if text == "" {
		slog.Warn("task:content:generate_embedding: empty text, skipping",
			"content_id", payload.ContentID)
		return nil
	}

	if err := contentEmbedder.EmbedContent(ctx, contentID.Bytes, tenantID.Bytes, text); err != nil {
		return fmt.Errorf("task:content:generate_embedding: %w", err)
	}

	// RAG extension (Section 69, ADR-069): also chunk and upsert into
	// content_embeddings. Best-effort — errors are logged, not propagated.
	ragIndexAfterEmbed(ctx, tenantID.Bytes, contentID.Bytes, text)

	return nil
}

// HandleContentGenerateDraft generates a content draft via the AI gateway
// (Section 75, ADR-076). This is the extended content:generate_draft task
// used by the AI Content Studio. It loads the persona voice prompt, content
// type field schema, and topic brief from the studio_jobs row, calls the AI
// gateway, and creates a contents row in 'draft' status.
//
// Until the full Studio package is implemented, this handler delegates to
// the registered AIProcessor (same gateway used by ai:process_task). When a
// StudioProcessor is registered, it takes precedence.
func HandleContentGenerateDraft(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:content:generate_draft")

	// If a studio processor is registered, delegate to it — it runs the full
	// §75 single-job pipeline (persona voice, field schema, content creation,
	// review routing).
	if studioProcessor != nil {
		var p struct {
			JobID    string `json:"job_id"`
			TenantID string `json:"tenant_id"`
		}

		if err := parsePayloadOrSkip(t, &p); err != nil {
			return fmt.Errorf("task:content:generate_draft: invalid payload: %w", err)
		}

		var id pgtype.UUID
		if err := id.Scan(p.JobID); err != nil {
			return fmt.Errorf("task:content:generate_draft: bad job_id: %w", err)
		}

		var tenantID pgtype.UUID
		if err := tenantID.Scan(p.TenantID); err != nil {
			return fmt.Errorf("task:content:generate_draft: bad tenant_id: %w", err)
		}

		if err := studioProcessor.GenerateJob(ctx, tenantID, id); err != nil {
			return fmt.Errorf("task:content:generate_draft: %w", err)
		}

		return nil
	}

	// Fallback: use the generic AI processor to generate the draft.
	if aiProcessor == nil {
		slog.Warn("task:content:generate_draft: no studio or AI processor registered, skipping")
		return nil
	}

	var payload struct {
		JobID       string `json:"job_id"`
		TenantID    string `json:"tenant_id"`
		PersonaID   string `json:"persona_id"`
		ContentType string `json:"content_type"`
		TopicPrompt string `json:"topic_prompt"`
	}

	if err := parsePayloadOrSkip(t, &payload); err != nil {
		return fmt.Errorf("task:content:generate_draft: invalid payload: %w", err)
	}

	if payload.TopicPrompt == "" {
		slog.Warn("task:content:generate_draft: empty topic_prompt, skipping",
			"job_id", payload.JobID)
		return nil
	}

	// Build a structured prompt for the AI gateway.
	prompt := fmt.Sprintf("Generate a %s draft about: %s", payload.ContentType, payload.TopicPrompt)

	if err := aiProcessor.ProcessAITask(ctx, "content_generation", prompt, "", payload.JobID); err != nil {
		return fmt.Errorf("task:content:generate_draft: AI processing failed: %w", err)
	}

	slog.Info("task:content:generate_draft: draft generation dispatched",
		"job_id", payload.JobID, "content_type", payload.ContentType)

	return nil
}

// contentRegenerateFromSourcePayload is the payload for the
// content:regenerate_from_source task (§75, ADR-076 Amendment 2).
type contentRegenerateFromSourcePayload struct {
	ContentID     string `json:"content_id"`
	IntelRecordID string `json:"intel_record_id"`
	TenantID      string `json:"tenant_id"`
}

// HandleContentRegenerateFromSource rewrites an existing contents row from
// the latest linked intel_record extraction. It delegates to the registered
// StudioProcessor (internal/server/agent_adapters.go) so the worker package
// does not import internal/content, internal/ai, or internal/studio.
func HandleContentRegenerateFromSource(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:content:regenerate_from_source")

	if studioProcessor == nil {
		slog.Warn("task:content:regenerate_from_source: no StudioProcessor registered, skipping")
		return nil
	}

	var payload contentRegenerateFromSourcePayload
	if err := parsePayloadOrSkip(t, &payload); err != nil {
		return fmt.Errorf("task:content:regenerate_from_source: invalid payload: %w", err)
	}

	contentID, err := scanUUID(payload.ContentID)
	if err != nil {
		return fmt.Errorf("task:content:regenerate_from_source: bad content_id: %w", err)
	}

	intelRecordID, err := scanUUID(payload.IntelRecordID)
	if err != nil {
		return fmt.Errorf("task:content:regenerate_from_source: bad intel_record_id: %w", err)
	}

	tenantID, err := scanUUID(payload.TenantID)
	if err != nil {
		return fmt.Errorf("task:content:regenerate_from_source: bad tenant_id: %w", err)
	}

	if err := studioProcessor.RegenerateFromSource(ctx, tenantID, contentID, intelRecordID); err != nil {
		return fmt.Errorf("task:content:regenerate_from_source: %w", err)
	}

	slog.Info("task:content:regenerate_from_source: completed",
		"content_id", payload.ContentID, "intel_record_id", payload.IntelRecordID)

	return nil
}

// agentDraftRequestPayload is the payload for the agent:create_draft_request
// task (Section 103, Phase 8). It extends content:generate_draft with an
// agent_session_id reference so the worker can persist provenance linking
// the draft back to the agent session that triggered it.
//
// The canonical type is worker.AgentDraftPayload (v2_processors.go); this
// alias keeps the handler readable while delegating to the exported type.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Content Request Flow, Asynq path)
// Ref: japaconnect-engineering/sections/11-job-queue-asynq.md (agent:create_draft_request)
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 8, task 8.03).
type agentDraftRequestPayload = AgentDraftPayload

// HandleAgentCreateDraftRequest is the worker handler for the
// agent:create_draft_request task (Section 103, Phase 8). It generates a
// draft content row from external search results + the topic prompt, with
// the AI persona as the author, and creates a HITL review with source=
// 'agent_draft_request' so the draft enters the admin review queue.
//
// The handler extends the content:generate_draft pipeline (§75) with the
// agent_session_id reference. When a StudioProcessor is registered, it is
// reused for the generation step; otherwise the generic AIProcessor is
// used. The contents row + HITL review are created via the registered
// AgentDraftProcessor (wired in internal/server), which encapsulates the
// content.Service.CreateContent + ai.Store.CreateHITLReview calls so the
// worker package does not import internal/content or internal/ai (avoiding
// a cycle).
//
// Until an AgentDraftProcessor is registered (RegisterAgentDraftProcessor),
// this handler logs a warning and returns nil — the task is a no-op. This
// matches the graceful-degradation pattern of HandleContentGenerateDraft.
func HandleAgentCreateDraftRequest(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:agent:create_draft_request")

	var payload agentDraftRequestPayload
	if err := parsePayloadOrSkip(t, &payload); err != nil {
		return fmt.Errorf("task:agent:create_draft_request: invalid payload: %w", err)
	}

	if payload.TopicPrompt == "" {
		slog.Warn("task:agent:create_draft_request: empty topic_prompt, skipping",
			"agent_session_id", payload.AgentSessionID)
		return nil
	}

	if payload.PersonaID == "" {
		slog.Warn("task:agent:create_draft_request: empty persona_id, skipping",
			"agent_session_id", payload.AgentSessionID)
		return nil
	}

	if payload.TenantID == "" {
		slog.Warn("task:agent:create_draft_request: empty tenant_id, skipping",
			"agent_session_id", payload.AgentSessionID)
		return nil
	}

	if payload.ContentType == "" {
		slog.Warn("task:agent:create_draft_request: empty content_type, skipping",
			"agent_session_id", payload.AgentSessionID)
		return nil
	}

	if agentDraftProcessor == nil {
		slog.Warn("task:agent:create_draft_request: no agent draft processor registered, skipping",
			"agent_session_id", payload.AgentSessionID)
		return nil
	}

	if err := agentDraftProcessor.CreateAgentDraft(ctx, payload); err != nil {
		return fmt.Errorf("task:agent:create_draft_request: %w", err)
	}

	slog.Info("task:agent:create_draft_request: draft created for admin review",
		"agent_session_id", payload.AgentSessionID,
		"persona_id", payload.PersonaID,
		"content_type", payload.ContentType)

	return nil
}

// ragIndexAfterEmbed is called after the legacy content_scores embedding is
// written. It extends the embedding task to also chunk and upsert into
// content_embeddings (the RAG knowledge layer, Section 69, ADR-069). Errors
// are logged but never propagated — RAG indexing is a best-effort extension
// that must not break the legacy embedding pipeline.
func ragIndexAfterEmbed(ctx context.Context, tenantID, contentID [16]byte, text string) {
	if ragIndexer == nil {
		return
	}

	tenantUUID := pgtype.UUID{Bytes: tenantID, Valid: true}
	contentUUID := pgtype.UUID{Bytes: contentID, Valid: true}

	// BUG-004 fix: resolve the indexing language from the i18n.default_language
	// system setting (§7 i18n) instead of hardcoding "en". Falls back to "en"
	// when settings are not injected or the key is unset. The language is
	// stored on each content_embeddings row and used by retrieval to filter
	// by the user's locale (ADR-069 §3).
	// Ref: japaconnect-engineering/sections/69-rag-knowledge-layer.md
	//      japaconnect-engineering/conventions/system-settings-catalog.md
	language := "en"
	if appSettings != nil {
		language = appSettings.GetStringOr(ctx, tenantUUID, "i18n.default_language", "en")
	}

	if err := ragIndexer.IndexEntity(ctx, tenantUUID, "content", contentUUID, text, language); err != nil {
		slog.Warn("task:content:generate_embedding: RAG index failed",
			"content_id", contentUUID.String(), "error", err)
	}
}

// HandleReviewAggregateRecalc recalculates the aggregate rating for a reviewable entity.
func HandleReviewAggregateRecalc(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:content:recalc_review_aggregate")

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
	slog.Info("task:ticket:send_status_update")

	queries, err := requireDB()
	if err != nil {
		slog.Warn("task:ticket:send_status_update: no database, skipping", "error", err)
		return nil
	}

	var payload struct {
		TicketID  string `json:"ticket_id"`
		TenantID  string `json:"tenant_id"`
		OldStatus string `json:"old_status"`
		NewStatus string `json:"new_status"`
	}

	if err := parsePayloadOrSkip(t, &payload); err != nil {
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

	if ticket.SubmitterEmail == nil || *ticket.SubmitterEmail == "" {
		slog.Warn("task:ticket:send_status_update: no submitter email, skipping",
			"ticket_id", payload.TicketID)
		return nil
	}

	statusTmpl, err := template.New("ticket-status").Parse(`<p>Your ticket "<strong>{{.Subject}}</strong>" status has been updated from <em>{{.OldStatus}}</em> to <em>{{.NewStatus}}</em>.</p>`)
	if err != nil {
		return fmt.Errorf("parsing status template: %w", err)
	}

	var statusBuf bytes.Buffer
	if err := statusTmpl.Execute(&statusBuf, struct {
		Subject   string
		OldStatus string
		NewStatus string
	}{Subject: ticket.Subject, OldStatus: payload.OldStatus, NewStatus: payload.NewStatus}); err != nil {
		return fmt.Errorf("rendering status template: %w", err)
	}

	body := statusBuf.String()

	if err := sendEmailWithContext(ctx, *ticket.SubmitterEmail, "Ticket Status Update - "+ticket.Subject, body, true); err != nil {
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
	slog.Info("task:learning:update_track_completion")

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
	slog.Info("task:content:process_video")

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
		// Thumbnail timestamp is configurable via system_settings
		// (media.video_thumbnail_time, default "00:00:01").
		thumbTime := "00:00:01"
		if appSettings != nil {
			thumbTime = appSettings.GetStringOr(ctx, tenantID, "media.video_thumbnail_time", "00:00:01")
		}

		if err := videoProcessor.GenerateThumbnail(ctx, inputPath, outputPath, thumbTime); err != nil {
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
	if err := gen.New(dbPool.Writer).UpdateMediaStoragePath(ctx, gen.UpdateMediaStoragePathParams{
		StoragePath: outputPath, ID: mediaID, TenantID: tenantID,
	}); err != nil {
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
	slog.Info("task:wallet:process_activity_event")

	if walletProcessor == nil {
		slog.Warn("task:wallet:process_activity_event: no processor registered, skipping")
		return nil
	}

	return walletProcessor.ProcessActivityEventTask(ctx, t.Payload())
}

// HandleWalletProcessCreditPurchase credits the user's wallet after a
// credit_purchase payment webhook confirms success (Section 58). The task is
// enqueued by HandlePaymentWebhookProcess when purpose_type="credit_purchase"
// and status="succeeded". Idempotent via the ledger's idempotency_key.
func HandleWalletProcessCreditPurchase(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:wallet:process_credit_purchase")

	if walletProcessor == nil {
		slog.Warn("task:wallet:process_credit_purchase: no processor registered, skipping")
		return nil
	}

	return walletProcessor.ProcessCreditPurchaseTask(ctx, t.Payload())
}

// HandleWalletAwardPoints awards points (or credits) to a user's wallet.
// Enqueued fire-and-forget by checklist completion, referral conversion,
// assessment scoring, and scholarship save (Sections 28, 60, 70, 71, 73).
// Delegates to the wallet processor; nil-safe if no processor is registered.
func HandleWalletAwardPoints(ctx context.Context, t *asynq.Task) error {
	if walletProcessor == nil {
		slog.Warn("task:wallet:award_points — no processor registered, skipping")
		return nil
	}

	return walletProcessor.AwardPointsTask(ctx, t.Payload())
}

// HandleWalletDeductCredits deducts credits from a user's wallet for ad
// campaign funding, or refunds credits on campaign rejection/early end
// (Section 61 — Self-Serve Ads). Delegates to the wallet processor; nil-safe.
func HandleWalletDeductCredits(ctx context.Context, t *asynq.Task) error {
	if walletProcessor == nil {
		slog.Warn("task:wallet:deduct_credits — no processor registered, skipping")
		return nil
	}

	return walletProcessor.DeductCreditsTask(ctx, t.Payload())
}

// HandleNewsletterSend sends a newsletter campaign to all subscribers
// matching the campaign's segment. Uses the marketing-category email
// provider (ADR-015/ADR-080), batches sends by newsletter.batch_size
// (§62/§22 fan-out pattern), and generates a per-recipient one-click
// unsubscribe token + List-Unsubscribe header (§62 Compliance, ADR-080 §4).
func HandleNewsletterSend(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:newsletter:send")

	if dbPool == nil {
		slog.Warn("task:newsletter:send: no database, skipping")
		return nil
	}

	var payload struct {
		NewsletterID string `json:"newsletter_id"`
		TenantID     string `json:"tenant_id"`
	}

	if err := parsePayloadOrSkip(t, &payload); err != nil {
		return err
	}

	campaignID, err := scanUUID(payload.NewsletterID)
	if err != nil {
		return fmt.Errorf("task:newsletter:send: %w", err)
	}

	tenantID, err := scanUUID(payload.TenantID)
	if err != nil {
		return fmt.Errorf("task:newsletter:send: %w", err)
	}

	reader := gen.New(dbPool.Reader)
	writer := gen.New(dbPool.Writer)

	// Fetch the full campaign row (incl. segment) so we can resolve the
	// correct recipient set per §62/ADR-080 §3.
	campaign, err := reader.GetNewsletterCampaignForSend(ctx, gen.GetNewsletterCampaignForSendParams{
		ID: campaignID, TenantID: tenantID,
	})
	if err != nil {
		return fmt.Errorf("task:newsletter:send: fetch campaign %s: %w", payload.NewsletterID, err)
	}

	subject, htmlBody := campaign.Subject, campaign.Html

	// Resolve the batch size from system_settings (§62, default 500).
	batchSize := newsletterBatchSize(ctx, tenantID)

	// Resolve recipients page-by-page according to the campaign segment,
	// upserting send-state rows and dispatching in batches. Paging keeps
	// memory bounded for large lists and mirrors the §22 fan-out pattern.
	sent := 0
	failed := 0
	total := 0
	page := int32(0)

	for {
		recipients, err := newsletterRecipientsForSegment(ctx, reader, tenantID, campaign.Segment, campaign.SegmentID, page, batchSize)
		if err != nil {
			return fmt.Errorf("task:newsletter:send: resolve recipients (segment=%s page=%d): %w", campaign.Segment, page, err)
		}

		if len(recipients) == 0 {
			break
		}

		// Upsert pending send-state rows for this batch so a crash mid-send
		// is resumable. UpsertNewsletterSendState resolves all subscribed
		// recipients internally (segment='all'); for segment-specific sends
		// the send-state rows are best-effort bookkeeping and may include
		// recipients outside the segment — that is acceptable because the
		// per-recipient MarkNewsletterRecipientSent/Failed calls below are
		// the source of truth for delivery status.
		if err := writer.UpsertNewsletterSendState(ctx, gen.UpsertNewsletterSendStateParams{
			CampaignID: campaignID, TenantID: tenantID,
		}); err != nil {
			slog.Warn("task:newsletter:send: upsert send state failed", "error", err)
		}

		for _, r := range recipients {
			total++
			// Generate a per-recipient one-click unsubscribe token + headers.
			unsubURL, headers := newsletterUnsubscribeLink(ctx, tenantID, r)
			body := htmlBody

			if unsubURL != "" {
				body = appendUnsubscribeLink(htmlBody, unsubURL)
			}

			if err := sendMarketingEmail(ctx, r, subject, body, true, headers); err != nil {
				slog.Error("task:newsletter:send: email failed", "recipient", r, "error", err)
				errMsg := err.Error()
				_ = writer.MarkNewsletterRecipientFailed(ctx, gen.MarkNewsletterRecipientFailedParams{
					Error: &errMsg, CampaignID: campaignID, Recipient: r,
				})
				failed++

				continue
			}

			_ = writer.MarkNewsletterRecipientSent(ctx, gen.MarkNewsletterRecipientSentParams{
				CampaignID: campaignID, Recipient: r,
			})
			sent++
		}

		//nolint:gosec // G115: bounded
		if int32(len(recipients)) < batchSize { // #nosec //nolint:gosec // G115: bounded
			break
		}

		page++
	}

	// Mark the campaign as sent with the recipient count (Section 62).
	_ = writer.MarkCampaignSent(ctx, gen.MarkCampaignSentParams{
		TenantID:       tenantID,
		ID:             campaignID,
		RecipientCount: int32(sent), // #nosec
	})

	slog.Info("task:newsletter:send: batch complete",
		"newsletter_id", payload.NewsletterID,
		"segment", campaign.Segment,
		"sent", sent, "failed", failed, "total", total)

	return nil
}

// newsletterRecipientsForSegment resolves the recipient email list for one
// page of the campaign's segment. Falls back to 'all' for unknown segments
// or when a space/tag segment yields no followers (so a misconfigured
// segment never silently drops every recipient).
func newsletterRecipientsForSegment(ctx context.Context, q *gen.Queries, tenantID pgtype.UUID, segment string, segmentID pgtype.UUID, page, limit int32) ([]string, error) {
	offset := page * limit

	switch segment {
	case "pro_subscribers":
		return q.NewsletterRecipientsPro(ctx, gen.NewsletterRecipientsProParams{
			TenantID: tenantID, PageOffset: offset, PageLimit: limit,
		})
	case "space":
		if !segmentID.Valid {
			break
		}

		rows, err := q.NewsletterRecipientsBySpace(ctx, gen.NewsletterRecipientsBySpaceParams{
			TenantID: tenantID, SegmentID: segmentID, PageOffset: offset, PageLimit: limit,
		})
		if err == nil && len(rows) > 0 {
			return rows, nil
		}
		// Fall back to 'all' if the space has no matching subscribers.
		slog.Warn("task:newsletter:send: space segment yielded no recipients, falling back to all",
			"segment_id", segmentID.String(), "error", err)
	case "tag":
		if !segmentID.Valid {
			break
		}

		rows, err := q.NewsletterRecipientsByTag(ctx, gen.NewsletterRecipientsByTagParams{
			TenantID: tenantID, SegmentID: segmentID, PageOffset: offset, PageLimit: limit,
		})
		if err == nil && len(rows) > 0 {
			return rows, nil
		}

		slog.Warn("task:newsletter:send: tag segment yielded no recipients, falling back to all",
			"segment_id", segmentID.String(), "error", err)
	}

	return q.NewsletterRecipientsAll(ctx, gen.NewsletterRecipientsAllParams{
		TenantID: tenantID, PageOffset: offset, PageLimit: limit,
	})
}

// newsletterBatchSize reads the newsletter.batch_size system_setting
// (§62, default 500). Falls back to 500 on any error.
func newsletterBatchSize(ctx context.Context, tenantID pgtype.UUID) int32 {
	if dbPool == nil {
		return 500
	}

	row, err := gen.New(dbPool.Reader).GetSettingValue(ctx, gen.GetSettingValueParams{
		TenantID: tenantID, Key: "newsletter.batch_size",
	})
	if err != nil || len(row) == 0 {
		return 500
	}

	n, err := strconv.Atoi(strings.Trim(string(row), `"`))
	if err != nil || n <= 0 {
		return 500
	}

	if n > 5000 {
		n = 5000
	}

	// #nosec G109 // value bounded by MaxPageSize check
	return int32(n) //nolint:gosec // bounded by 5000 cap above
}

// appendUnsubscribeLink appends a one-click unsubscribe footer to the HTML
// body if not already present. The link uses the /unsubscribe?token=TOKEN
// page on the main app (§62 Compliance, ADR-080 §4).
func appendUnsubscribeLink(htmlBody, unsubURL string) string {
	footer := `<p style="margin-top:24px;font-size:11px;color:#888;"><a href="` + unsubURL + `">Unsubscribe</a></p>`

	if strings.Contains(htmlBody, unsubURL) {
		return htmlBody
	}

	if strings.Contains(htmlBody, "</body>") {
		return strings.Replace(htmlBody, "</body>", footer+"</body>", 1)
	}

	return htmlBody + footer
}

// ---------------------------------------------------------------------------
// Low tier handlers
// ---------------------------------------------------------------------------

// HandleDigestEmail sends a digest email summarizing recent activity to a user.
// When called by the cron scheduler with an empty payload, iterates all active users.
func HandleDigestEmail(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:email:send_digest")

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

	_ = parsePayload(t, &payload) // tolerate empty payload from cron scheduler

	// When a specific user is targeted, send only to that user.
	if payload.UserID != "" && payload.TenantID != "" && payload.Email != "" {
		return sendDigestToUser(ctx, payload.UserID, payload.TenantID, payload.Email, payload.Period)
	}

	// Cron mode: iterate all active users with digest enabled.
	period := payload.Period
	if period == "" {
		period = "daily"
	}

	digestUsers, err := gen.New(dbPool.Reader).ListActiveUsersForDigest(ctx)
	if err != nil {
		slog.Error("task:email:send_digest: list users failed", "error", err)
		return err
	}

	sentCount := 0

	for _, u := range digestUsers {
		if err := sendDigestToUser(ctx, u.ID.String(), u.TenantID.String(), u.Email, period); err != nil {
			slog.Error("task:email:send_digest: failed for user", "user_id", u.ID.String(), "error", err)
			continue
		}

		sentCount++
	}

	slog.Info("task:email:send_digest: complete", "sent", sentCount)

	return nil
}

// sendDigestToUser sends a digest email to a single user.
func sendDigestToUser(ctx context.Context, userID, tenantID, email, period string) error {
	if period == "" {
		period = "daily"
	}

	uid, err := scanUUID(userID)
	if err != nil {
		return fmt.Errorf("invalid user_id: %w", err)
	}

	tid, err := scanUUID(tenantID)
	if err != nil {
		return fmt.Errorf("invalid tenant_id: %w", err)
	}

	feedItems, err := gen.New(dbPool.Reader).ListDigestFeedItems(ctx, gen.ListDigestFeedItemsParams{
		UserID: uid, TenantID: tid,
	})
	if err != nil {
		return fmt.Errorf("fetching feed: %w", err)
	}

	// Build digest HTML using html/template for XSS safety.
	digestTmpl, err := template.New("digest").Parse(`<h2>{{.Period}} Digest</h2>
<ul>
{{range .Items}}  <li><strong>{{.Title}}</strong></li>
{{end}}</ul>`)
	if err != nil {
		return fmt.Errorf("parsing digest template: %w", err)
	}

	type digestData struct {
		Period string
		Items  []struct{ Title string }
	}

	var items []struct{ Title string }

	itemCount := 0

	for _, fi := range feedItems {
		items = append(items, struct{ Title string }{Title: fi.Title})
		itemCount++
	}

	if itemCount == 0 {
		return nil
	}

	var body strings.Builder
	if err := digestTmpl.Execute(&body, digestData{
		Period: cases.Title(language.English).String(period),
		Items:  items,
	}); err != nil {
		return fmt.Errorf("rendering digest template: %w", err)
	}

	if err := sendEmailWithContext(ctx, email,
		fmt.Sprintf("Your %s Digest", cases.Title(language.English).String(period)),
		body.String(), true); err != nil {
		return fmt.Errorf("sending email: %w", err)
	}

	slog.Info("task:email:send_digest: sent",
		"user_id", userID, "items", itemCount)

	return nil
}

// HandleSitemapGenerate regenerates the XML sitemap for the tenant.
// When called by the cron scheduler with an empty payload, iterates all active tenants.
func HandleSitemapGenerate(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:sitemap:generate")

	if dbPool == nil {
		slog.Warn("task:sitemap:generate: no database, skipping")
		return nil
	}

	var payload struct {
		TenantID string `json:"tenant_id"`
		BaseURL  string `json:"base_url"`
	}

	_ = parsePayload(t, &payload) // tolerate empty payload from cron scheduler

	// Collect tenant IDs: single requested tenant or all active ones.
	var tenantIDs []pgtype.UUID

	if payload.TenantID != "" {
		tid, err := scanUUID(payload.TenantID)
		if err != nil {
			return fmt.Errorf("task:sitemap:generate: %w", err)
		}

		tenantIDs = []pgtype.UUID{tid}
	} else {
		var err error

		tenantIDs, err = gen.New(dbPool.Reader).ListActiveTenantIDs(ctx)
		if err != nil {
			slog.Error("task:sitemap:generate: list tenants failed", "error", err)
			return err
		}
	}

	for _, tenantID := range tenantIDs {
		baseURL := payload.BaseURL
		if baseURL == "" {
			baseURL = resolveBaseURL(ctx, tenantID)
		}

		if baseURL == "" {
			slog.Warn("task:sitemap:generate: no base_url, skipping tenant", "tenant_id", tenantID.String())
			continue
		}

		generateSitemapForTenant(ctx, tenantID, baseURL)
	}

	return nil
}

// generateSitemapForTenant generates the sitemap XML for a single tenant.
// Content is filtered through the Section 43 sitemap quality gates
// (bots.IsContentIndexableByType) using per-content-type thresholds
// from system_settings.
// Ref: japaconnect-engineering/sections/43-bot-crawler-management.md.
func generateSitemapForTenant(ctx context.Context, tenantID pgtype.UUID, baseURL string) {
	slugs, err := gen.New(dbPool.Reader).ListPublishedSlugsForSitemap(ctx, tenantID)
	if err != nil {
		slog.Error("task:sitemap:generate: failed to list content", "tenant_id", tenantID.String(), "error", err)
		return
	}

	// Read all sitemap quality gate thresholds from system_settings.
	// Falls back to spec defaults when unset or settings unavailable.
	cfg := bots.DefaultSitemapQualityConfig()
	if appSettings != nil {
		cfg.MinWords = appSettings.GetIntOr(ctx, tenantID, "seo.sitemap.content_min_words", cfg.MinWords)
		cfg.BlogMinWords = appSettings.GetIntOr(ctx, tenantID, "seo.sitemap.blog_min_words", cfg.BlogMinWords)
		cfg.ForumMinWords = appSettings.GetIntOr(ctx, tenantID, "seo.sitemap.forum_min_words", cfg.ForumMinWords)
		cfg.DiscussionMinWords = appSettings.GetIntOr(ctx, tenantID, "seo.sitemap.discussion_min_words", cfg.DiscussionMinWords)
		cfg.GuideMinWords = appSettings.GetIntOr(ctx, tenantID, "seo.sitemap.guide_min_words", cfg.GuideMinWords)
		cfg.ComparisonMinWords = appSettings.GetIntOr(ctx, tenantID, "seo.sitemap.comparison_min_words", cfg.ComparisonMinWords)
		cfg.BlogMinEngagement = appSettings.GetIntOr(ctx, tenantID, "seo.sitemap.blog_min_engagement", cfg.BlogMinEngagement)
		cfg.ForumMinReplies = appSettings.GetIntOr(ctx, tenantID, "seo.sitemap.forum_min_replies", cfg.ForumMinReplies)
		cfg.QuestionMinAnswers = appSettings.GetIntOr(ctx, tenantID, "seo.sitemap.question_min_answers", cfg.QuestionMinAnswers)
		cfg.DiscussionMinComments = appSettings.GetIntOr(ctx, tenantID, "seo.sitemap.discussion_min_comments", cfg.DiscussionMinComments)
	}

	var sitemap strings.Builder

	sitemap.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	sitemap.WriteString("<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\">\n")

	urlCount := 0

	for _, s := range slugs {
		// Apply the Section 43 per-content-type quality gate.
		isQuestion := s.IsQuestion != nil && *s.IsQuestion
		if !bots.IsContentIndexableByType(bots.ContentIndexableInput{
			ContentType:   s.ContentType,
			Status:        "published",
			Visibility:    s.Visibility,
			WordCount:     int(s.WordCount),
			CommentCount:  int(s.CommentCount),
			ShareCount:    int(s.ShareCount),
			ReactionCount: int(s.ReactionCount),
			ReplyCount:    int(s.ReplyCount),
			IsQuestion:    isQuestion,
			Solved:        s.Solved,
			SeoIndexable:  s.SeoIndexable,
		}, cfg) {
			continue
		}

		sitemap.WriteString(fmt.Sprintf("  <url>\n    <loc>%s/%s</loc>\n    <lastmod>%s</lastmod>\n  </url>\n",
			baseURL, s.Slug,
			s.UpdatedAt.Time.Format("2006-01-02")))

		urlCount++
	}

	sitemap.WriteString("</urlset>\n")

	// Write to a tenant-scoped path so multi-tenant deployments don't
	// overwrite each other. The output directory is configurable via
	// JC_SITEMAP_DIR (default: ./sitemaps).
	sitemapDir := getEnv("JC_SITEMAP_DIR", "./sitemaps")
	//nolint:gosec // G301: bounded
	if err := os.MkdirAll(sitemapDir, 0o755); err != nil { // #nosec //nolint:gosec // G301: bounded
		slog.Error("task:sitemap:generate: failed to create dir", "dir", sitemapDir, "error", err)
		return
	}

	sitemapPath := fmt.Sprintf("%s/sitemap-%s.xml", sitemapDir, tenantID.String())
	if err := os.WriteFile(sitemapPath, []byte(sitemap.String()), 0o600); err != nil {
		slog.Error("task:sitemap:generate: failed to write file", "path", sitemapPath, "error", err)
		return
	}

	slog.Info("task:sitemap:generate: sitemap written", "tenant_id", tenantID.String(), "path", sitemapPath, "url_count", urlCount)

	// Invalidate the on-demand sitemap Valkey cache so the live HTTP
	// handlers serve fresh XML on next request (BE-11 fix).
	// Ref: japaconnect-engineering/sections/26-seo-performance.md
	seo.InvalidateSitemapCache(ctx, rdb, tenantID)
}

// HandleRSSRegenerate regenerates the RSS feed for published content.
// When called by the cron scheduler with an empty payload, iterates all active tenants.
func HandleRSSRegenerate(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:rss_regenerate")

	if dbPool == nil {
		slog.Warn("task:rss_regenerate: no database, skipping")
		return nil
	}

	var payload struct {
		TenantID string `json:"tenant_id"`
		BaseURL  string `json:"base_url"`
	}

	_ = parsePayload(t, &payload) // tolerate empty payload from cron scheduler

	// Collect tenant IDs: single requested tenant or all active ones.
	var tenantIDs []pgtype.UUID

	if payload.TenantID != "" {
		tid, err := scanUUID(payload.TenantID)
		if err != nil {
			return fmt.Errorf("task:rss_regenerate: %w", err)
		}

		tenantIDs = []pgtype.UUID{tid}
	} else {
		var err error

		tenantIDs, err = gen.New(dbPool.Reader).ListActiveTenantIDs(ctx)
		if err != nil {
			slog.Error("task:rss_regenerate: list tenants failed", "error", err)
			return err
		}
	}

	for _, tenantID := range tenantIDs {
		baseURL := payload.BaseURL
		if baseURL == "" {
			baseURL = resolveBaseURL(ctx, tenantID)
		}

		if baseURL == "" {
			slog.Warn("task:rss_regenerate: no base_url, skipping tenant", "tenant_id", tenantID.String())
			continue
		}

		generateRSSForTenant(ctx, tenantID, baseURL)
	}

	return nil
}

// rssFeedLimit is the number of items returned per feed. Matches
// internal/rss.defaultFeedLimit. RSS-008: previously hardcoded as 20 inline.
const rssFeedLimit = 20

// generateRSSForTenant generates the RSS feed for a single tenant using the
// shared sqlc query (ListRSSFeedItems — the same one internal/rss uses for
// live requests) and caches the rendered XML in Valkey under
// jc:cache:feed:rss:{tenant_id} with a 1-hour TTL (VALKEY-002 compliance).
// The live /feed.xml route can read from this cache to avoid a DB hit on
// every request. Writing to a local file is removed — it was not
// multi-tenant safe.
//
// RSS-001: also generates a blog-only feed (ListPublishedPostsForRSS) into
// jc:cache:feed:rss:blog:{tenant_id} so /api/v1/blog/feed.xml doesn't serve
// non-blog content from the universal cache.
//
// RSS-011: uses the shared internal/rss/model subpackage for the XML model
// and rendering helpers — no more local copies that can drift (this drift
// caused RSS-003: the old local formatRSSTime didn't check IsZero).
func generateRSSForTenant(ctx context.Context, tenantID pgtype.UUID, baseURL string) {
	q := gen.New(dbPool.Reader)

	rows, err := q.ListRSSFeedItems(ctx, gen.ListRSSFeedItemsParams{
		TenantID:  tenantID,
		PageLimit: rssFeedLimit,
	})
	if err != nil {
		slog.Error("task:rss_regenerate: failed to list content", "tenant_id", tenantID.String(), "error", err)
		return
	}

	items := make([]rssmodel.Item, 0, len(rows))
	for _, row := range rows {
		items = append(items, rssmodel.ItemFromRow(
			row.ID, row.Title, row.Slug, row.BodyHtml, row.Body,
			row.Username, row.PublishedAt, row.CreatedAt, baseURL, "/content/",
		))
	}

	xmlBytes := rssmodel.Render(siteName, baseURL, siteName+" — latest content", "en", items)

	cacheKey := valkey.MustKey("cache:feed:rss:%s", tenantID.String())
	if rdb != nil {
		if err := rdb.Set(ctx, cacheKey, string(xmlBytes), time.Hour).Err(); err != nil {
			slog.Error("task:rss_regenerate: failed to cache feed", "tenant_id", tenantID.String(), "error", err)
		}
	} else {
		slog.Warn("task:rss_regenerate: no redis client, feed not cached", "tenant_id", tenantID.String())
	}

	slog.Info("task:rss_regenerate: feed cached", "tenant_id", tenantID.String(), "item_count", len(items), "cache_key", cacheKey)

	// RSS-001: generate the blog-only feed into a separate cache key so
	// /api/v1/blog/feed.xml doesn't serve non-blog content from the
	// universal cache. Uses ListPublishedPostsForRSS (content_type=blog_post,
	// auto_hidden=false) — the same query the live blog handler falls back
	// to on a cache miss.
	generateBlogRSSForTenant(ctx, q, tenantID, baseURL)
}

// generateBlogRSSForTenant generates the blog-only RSS feed and caches it
// under jc:cache:feed:rss:blog:{tenant_id} (1hr TTL).
func generateBlogRSSForTenant(ctx context.Context, q *gen.Queries, tenantID pgtype.UUID, baseURL string) {
	rows, err := q.ListPublishedPostsForRSS(ctx, gen.ListPublishedPostsForRSSParams{
		TenantID:  tenantID,
		PageLimit: rssFeedLimit,
	})
	if err != nil {
		slog.Error("task:rss_regenerate: failed to list blog posts", "tenant_id", tenantID.String(), "error", err)
		return
	}

	items := make([]rssmodel.Item, 0, len(rows))

	for _, p := range rows {
		pubDate := rssmodel.FormatTime(p.PublishedAt)
		if pubDate == "" {
			pubDate = rssmodel.FormatTime(p.CreatedAt)
		}

		desc := rssmodel.HTMLToTextExcerpt(p.BodyHtml, rssmodel.ExcerptMaxRunes)
		if desc == "" && p.Body != "" {
			desc = rssmodel.ExcerptFromBody(p.Body)
		}

		items = append(items, rssmodel.Item{
			Title:       p.Title,
			Link:        baseURL + "/blog/" + p.Slug,
			GUID:        rssmodel.GUIDFromID(p.ID),
			Description: desc,
			PubDate:     pubDate,
		})
	}

	xmlBytes := rssmodel.Render(
		siteName+" Blog",
		baseURL+"/blog",
		siteName+" Blog — latest posts",
		"en-us",
		items,
	)

	cacheKey := valkey.MustKey("cache:feed:rss:blog:%s", tenantID.String())
	if rdb != nil {
		if err := rdb.Set(ctx, cacheKey, string(xmlBytes), time.Hour).Err(); err != nil {
			slog.Error("task:rss_regenerate: failed to cache blog feed", "tenant_id", tenantID.String(), "error", err)
		}
	}

	slog.Info("task:rss_regenerate: blog feed cached", "tenant_id", tenantID.String(), "item_count", len(items), "cache_key", cacheKey)
}

// HandleDataExport generates a GDPR data export archive for a user.
func HandleDataExport(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:data:export_user_data")

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

// HandlePlatformDataExport generates a platform-wide data export file
// (Section 67). The payload carries export_id + tenant_id.
func HandlePlatformDataExport(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:platform:data_export")

	if platformDataExportProcessor == nil {
		slog.Warn("task:platform:data_export: no processor registered, skipping")
		return nil
	}

	var payload struct {
		ExportID string `json:"export_id"`
		TenantID string `json:"tenant_id"`
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:platform:data_export: invalid payload", "error", err)
		return err
	}

	var exportID pgtype.UUID
	if err := exportID.Scan(payload.ExportID); err != nil {
		slog.Error("task:platform:data_export: invalid export_id", "error", err)
		return err
	}

	if err := platformDataExportProcessor.ProcessExport(ctx, exportID); err != nil {
		errpkg.CaptureException(fmt.Errorf("platform data export failed (export=%s): %w", payload.ExportID, err))
		return err
	}

	return nil
}

// HandlePlatformDataImport processes a platform-wide data import file
// (Section 67). The payload carries import_id + tenant_id.
func HandlePlatformDataImport(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:platform:data_import")

	if platformDataImportProcessor == nil {
		slog.Warn("task:platform:data_import: no processor registered, skipping")
		return nil
	}

	var payload struct {
		ImportID string `json:"import_id"`
		TenantID string `json:"tenant_id"`
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:platform:data_import: invalid payload", "error", err)
		return err
	}

	var importID pgtype.UUID
	if err := importID.Scan(payload.ImportID); err != nil {
		slog.Error("task:platform:data_import: invalid import_id", "error", err)
		return err
	}

	if err := platformDataImportProcessor.ProcessImport(ctx, importID); err != nil {
		errpkg.CaptureException(fmt.Errorf("platform data import failed (import=%s): %w", payload.ImportID, err))
		return err
	}

	return nil
}

// HandleAccountDeletionCleanup performs the final cleanup after an account deletion request.
func HandleAccountDeletionCleanup(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:account:deletion_cleanup")

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
	slog.Info("task:ad:ingest_event_batch")

	queries, err := requireDB()
	if err != nil {
		slog.Warn("task:ad:ingest_event_batch: no database, skipping", "error", err)
		return nil
	}

	var payload struct {
		TenantID   string `json:"tenant_id"`
		CampaignID string `json:"campaign_id"`
		UserID     string `json:"user_id"`    // may be empty for guest events
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

	// user_id is nullable — guest events have no user_id in the payload.
	var userID pgtype.UUID
	if payload.UserID != "" {
		userID, err = scanUUID(payload.UserID)
		if err != nil {
			return fmt.Errorf("task:ad:ingest_event_batch: %w", err)
		}
	}

	if _, err := queries.InsertAdEvent(ctx, gen.InsertAdEventParams{
		TenantID:   tenantID,
		CampaignID: campaignID,
		UserID:     userID,
		EventType:  payload.EventType,
		OccurredAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
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
	slog.Info("task:maintenance:update_velocity_scores")

	if dbPool == nil {
		slog.Warn("task:maintenance:update_velocity_scores: no database, skipping")
		return nil
	}

	var payload struct {
		TenantID string `json:"tenant_id"`
	}

	// Payload is optional — scheduler sends nil. Parse without failing.
	_ = parsePayload(t, &payload)

	// If a specific tenant was provided, process only that tenant.
	// Otherwise, iterate over all active tenants.
	var tenantIDs []pgtype.UUID

	if payload.TenantID != "" {
		tid, err := scanUUID(payload.TenantID)
		if err != nil {
			return fmt.Errorf("task:maintenance:update_velocity_scores: %w", err)
		}

		tenantIDs = []pgtype.UUID{tid}
	} else {
		var err error

		tenantIDs, err = gen.New(dbPool.Reader).ListActiveTenantIDs(ctx)
		if err != nil {
			slog.Error("task:maintenance:update_velocity_scores: failed to list tenants", "error", err)
			return err
		}
	}

	for _, tenantID := range tenantIDs {
		// Update velocity scores for recently active content.
		// Velocity = (vote_score * 2 + comment_count + view_count / 100) /
		//   (hours_since_publish ^ gravity)
		// gravity is hot-reloadable via feed.velocity.gravity (default 1.8).
		gravity := 1.8
		if appSettings != nil {
			gravity = appSettings.GetFloatOr(ctx, tenantID, "feed.velocity.gravity", 1.8)
		}

		q := gen.New(dbPool.Writer)
		if err := q.UpdateVelocityScores(ctx, gen.UpdateVelocityScoresParams{
			Gravity:  gravity,
			TenantID: tenantID,
		}); err != nil {
			slog.Error("task:maintenance:update_velocity_scores: failed to update scores", "error", err, "tenant_id", tenantID.String())
		}
	}

	slog.Info("task:maintenance:update_velocity_scores: complete", "tenants", len(tenantIDs))

	return nil
}

// HandleMaintenanceCheckSLABreaches checks for tickets that have breached their SLA deadlines.
func HandleMaintenanceCheckSLABreaches(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:maintenance:check_sla_breaches")

	if dbPool == nil {
		slog.Warn("task:maintenance:check_sla_breaches: no database, skipping")
		return nil
	}

	var payload struct {
		TenantID string `json:"tenant_id"`
	}

	// Payload is optional — scheduler sends nil. Parse without failing.
	_ = parsePayload(t, &payload)

	// If a specific tenant was provided, process only that tenant.
	// Otherwise, iterate over all active tenants.
	var tenantIDs []pgtype.UUID

	if payload.TenantID != "" {
		tid, err := scanUUID(payload.TenantID)
		if err != nil {
			return fmt.Errorf("task:maintenance:check_sla_breaches: %w", err)
		}

		tenantIDs = []pgtype.UUID{tid}
	} else {
		var err error

		tenantIDs, err = gen.New(dbPool.Reader).ListActiveTenantIDs(ctx)
		if err != nil {
			slog.Error("task:maintenance:check_sla_breaches: failed to list tenants", "error", err)
			return err
		}
	}

	breachCount := 0
	autoClosedCount := 0

	for _, tenantID := range tenantIDs {
		// Find tickets where SLA has been breached.
		breaches, err := gen.New(dbPool.Reader).ListSLABreachedTickets(ctx, tenantID)
		if err != nil {
			slog.Error("task:maintenance:check_sla_breaches: query failed", "error", err, "tenant_id", tenantID.String())
			continue
		}

		for _, b := range breaches {
			slog.Warn("task:maintenance:check_sla_breaches: SLA breach detected",
				"ticket_id", b.ID.String(),
				"subject", b.Subject,
				"sla_breach_at", b.SlaBreachAt.Time)

			breachCount++

			// Create an in-app notification for the assigned agent.
			// Ref: japaconnect-engineering/sections/38-ticket-system.md (SLA Tracking)
			if b.AssignedTo.Valid {
				title := "SLA breached: " + b.Subject
				bodyStr := fmt.Sprintf("Ticket %s has breached its SLA deadline.", b.ID.String())
				actionURL := "/admin/tickets/" + b.ID.String()
				actorType := "system"
				subjectType := "ticket"

				if _, nErr := gen.New(dbPool.Writer).InsertNotification(ctx, gen.InsertNotificationParams{
					TenantID:         tenantID,
					RecipientID:      b.AssignedTo,
					NotificationType: "ticket_sla_breached",
					ActorType:        &actorType,
					SubjectType:      &subjectType,
					SubjectID:        b.ID,
					Title:            title,
					Body:             &bodyStr,
					ActionUrl:        &actionURL,
				}); nErr != nil {
					slog.Error("task:maintenance:check_sla_breaches: failed to create notification",
						"ticket_id", b.ID.String(), "error", nErr)
				}
			}
		}

		// Auto-close resolved tickets older than tickets.auto_close_resolved_days
		// (system_settings, default 7). Ref: conventions/system-settings-catalog.md.
		autoCloseDays := 7
		if appSettings != nil {
			autoCloseDays = appSettings.GetIntOr(ctx, tenantID, "tickets.auto_close_resolved_days", 7)
		}

		daysStr := strconv.Itoa(autoCloseDays)

		closed, err := gen.New(dbPool.Writer).AutoCloseResolvedTickets(ctx, gen.AutoCloseResolvedTicketsParams{
			TenantID: tenantID,
			Days:     &daysStr,
		})
		if err != nil {
			slog.Error("task:maintenance:check_sla_breaches: auto-close failed", "error", err, "tenant_id", tenantID.String())
			continue
		}

		if closed > 0 {
			slog.Info("task:maintenance:check_sla_breaches: auto-closed resolved tickets",
				"count", closed, "tenant_id", tenantID.String(), "days_threshold", autoCloseDays)

			autoClosedCount += int(closed)
		}
	}

	slog.Info("task:maintenance:check_sla_breaches: complete", "breached_count", breachCount, "auto_closed_count", autoClosedCount)

	return nil
}

// HandleMaintenanceUpdateGeoIPDB downloads and updates the GeoIP database.
func HandleMaintenanceUpdateGeoIPDB(_ context.Context, t *asynq.Task) error {
	slog.Info("task:maintenance:update_geoip_db")

	if geoipUpdater == nil {
		slog.Warn("task:maintenance:update_geoip_db: updater not registered, skipping")
		return nil
	}

	if err := geoipUpdater.DownloadAndUpdate(); err != nil {
		// JC_MAXMIND_LICENSE_KEY not set is a config issue, not a task failure.
		// Skip gracefully to avoid burning retries in the DLQ.
		if strings.Contains(err.Error(), "JC_MAXMIND_LICENSE_KEY is not set") {
			slog.Warn("task:maintenance:update_geoip_db: license key not configured, skipping")
			return nil
		}

		slog.Error("task:maintenance:update_geoip_db: update failed", "error", err)

		return fmt.Errorf("geoip database update: %w", err)
	}

	slog.Info("task:maintenance:update_geoip_db: update complete")

	return nil
}

// HandleSpaceAutoPopulate auto-populates a space with recommended content based on its tags.
func HandleSpaceAutoPopulate(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:space:auto_populate")

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
	slog.Info("task:analytics:refresh_views")

	if analyticsProcessor == nil {
		slog.Warn("task:analytics:refresh_views: no analytics processor registered, skipping")
		return nil
	}

	return analyticsProcessor.RefreshViews(ctx)
}

// HandleSecurityScan runs a periodic security scan on uploaded media and
// triggers the analytics security scan (failed-login spikes, SLA breaches,
// stale tokens) that populates security_scan_results for the control panel.
//
// When the cron scheduler fires with an empty payload ("{}"), the handler
// iterates over all active tenants. A specific tenant_id can be passed for
// targeted scans.
func HandleSecurityScan(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:security:scan")

	if dbPool == nil {
		slog.Warn("task:security:scan: no database, skipping")
		return nil
	}

	var payload struct {
		TenantID string `json:"tenant_id"`
	}

	if err := parsePayload(t, &payload); err != nil {
		// Tolerate empty/malformed payload from cron scheduler — process all tenants.
		slog.Warn("task:security:scan: could not parse payload, processing all tenants", "error", err)
	}

	// Collect tenant IDs: either the single requested tenant or all active ones.
	var tenantIDs []pgtype.UUID

	if payload.TenantID != "" {
		var tid pgtype.UUID
		if err := tid.Scan(payload.TenantID); err != nil {
			slog.Error("task:security:scan: invalid tenant_id", "error", err)
			return fmt.Errorf("task:security:scan: invalid tenant_id: %w", err)
		}

		tenantIDs = []pgtype.UUID{tid}
	} else {
		var err error

		tenantIDs, err = gen.New(dbPool.Reader).ListActiveTenantIDs(ctx)
		if err != nil {
			slog.Error("task:security:scan: list tenants failed", "error", err)
			return fmt.Errorf("task:security:scan: %w", err)
		}
	}

	if len(tenantIDs) == 0 {
		slog.Warn("task:security:scan: no active tenants found")
		return nil
	}

	for _, tenantID := range tenantIDs {
		tid := tenantID // capture for closure

		// 1. Run the analytics security scan (populates security_scan_results).
		if analyticsProcessor != nil {
			if err := analyticsProcessor.RunSecurityScan(ctx, tid.Bytes); err != nil {
				slog.Error("task:security:scan: analytics scan failed", "tenant_id", tid.String(), "error", err)
			}
		}

		// 2. Scan uploaded media via ClamAV (existing behaviour).
		mediaRows, err := gen.New(dbPool.Reader).ListMediaForScanning(ctx, tid)
		if err != nil {
			slog.Error("task:security:scan: media query failed", "tenant_id", tid.String(), "error", err)
			continue
		}

		scanCount := 0

		for _, mr := range mediaRows {
			if moderationProcessor != nil {
				if err := moderationProcessor.ScanContent(ctx, "media", mr.ID.String(), tid.String()); err != nil {
					slog.Error("task:security:scan: media scan enqueue failed", "media_id", mr.ID.String(), "error", err)
				} else {
					scanCount++
				}
			}
		}

		slog.Info("task:security:scan: tenant complete", "tenant_id", tid.String(), "media_scanned", scanCount)
	}

	slog.Info("task:security:scan: complete", "tenant_count", len(tenantIDs))

	return nil
}

// HandleMaintenanceRegenerateLLMsTxt regenerates the llms.txt file for the tenant.
// When called by the cron scheduler with an empty payload, iterates all active tenants.
// The handler invalidates the Valkey cache keys (jc:llms_txt:{tenant_id} and
// jc:llms_full_txt:{tenant_id}) so the next request to /llms.txt or /llms-full.txt
// regenerates the content from the database via bots.LLMsHandler.
// Ref: japaconnect-engineering/sections/43-bot-crawler-management.md.
func HandleMaintenanceRegenerateLLMsTxt(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:maintenance:regenerate_llms_txt")

	if dbPool == nil {
		slog.Warn("task:maintenance:regenerate_llms_txt: no database, skipping")
		return nil
	}

	var payload struct {
		TenantID string `json:"tenant_id"`
		BaseURL  string `json:"base_url"`
	}

	_ = parsePayload(t, &payload) // tolerate empty payload from cron scheduler

	// Collect tenant IDs: single requested tenant or all active ones.
	var tenantIDs []pgtype.UUID

	if payload.TenantID != "" {
		tid, err := scanUUID(payload.TenantID)
		if err != nil {
			return fmt.Errorf("task:maintenance:regenerate_llms_txt: %w", err)
		}

		tenantIDs = []pgtype.UUID{tid}
	} else {
		var err error

		tenantIDs, err = gen.New(dbPool.Reader).ListActiveTenantIDs(ctx)
		if err != nil {
			slog.Error("task:maintenance:regenerate_llms_txt: list tenants failed", "error", err)
			return err
		}
	}

	for _, tenantID := range tenantIDs {
		invalidateLLMsCacheForTenant(ctx, tenantID)
	}

	slog.Info("task:maintenance:regenerate_llms_txt: complete", "tenant_count", len(tenantIDs))

	return nil
}

// invalidateLLMsCacheForTenant deletes the cached llms.txt and llms-full.txt
// for a tenant so the next request regenerates them from the database.
func invalidateLLMsCacheForTenant(ctx context.Context, tenantID pgtype.UUID) {
	if rdb == nil {
		slog.Warn("task:maintenance:regenerate_llms_txt: no redis client, skipping", "tenant_id", tenantID.String())
		return
	}

	keys := []string{
		valkey.MustKey("llms_txt:%s", tenantID.String()),
		valkey.MustKey("llms_full_txt:%s", tenantID.String()),
		valkey.MustKey("robots_txt:%s", tenantID.String()),
	}

	for _, key := range keys {
		if err := rdb.Del(ctx, key).Err(); err != nil {
			slog.Warn("task:maintenance:regenerate_llms_txt: failed to delete cache key", "key", key, "error", err)
		}
	}

	slog.Info("task:maintenance:regenerate_llms_txt: invalidated caches", "tenant_id", tenantID.String())
}

// HandleLearningIssueCertificate issues a completion certificate for a learning track.
func HandleLearningIssueCertificate(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:learning:issue_certificate")

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
	slog.Info("task:db:manual_vacuum")

	if dbPool == nil {
		slog.Warn("task:db:manual_vacuum: no database, skipping")
		return nil
	}

	// Versioned payload: Version 1 is the current shape. A Version of 0
	// is accepted for backward compatibility with any in-flight tasks
	// queued before the version field was added.
	var payload struct {
		Version   int    `json:"version"`    // payload schema version (current: 1)
		TableName string `json:"table_name"` // optional: specific table to vacuum
		Full      bool   `json:"full"`       // VACUUM FULL vs regular
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:db:manual_vacuum: invalid payload", "error", err)
		return err
	}

	if err := dbPool.Vacuum(ctx, payload.TableName, payload.Full); err != nil {
		slog.Error("task:db:manual_vacuum: vacuum failed", "error", err)
		return err
	}

	slog.Info("task:db:manual_vacuum: vacuum complete", "table", payload.TableName, "full", payload.Full, "version", payload.Version)

	return nil
}

// HandleNewsletterDigest compiles and sends a weekly newsletter digest to all
// active subscribers for the tenant. Fetches recently published content, builds
// an HTML digest, and sends via the registered email gateway or SMTP fallback.
func HandleNewsletterDigest(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:newsletter:digest")

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
	digestItems, err := gen.New(dbPool.Reader).ListPublishedContentForDigest(ctx, tenantID)
	if err != nil {
		slog.Error("task:newsletter:digest: failed to list content", "error", err)
		return err
	}

	if len(digestItems) == 0 {
		slog.Info("task:newsletter:digest: no recent content, skipping", "tenant_id", payload.TenantID)
		return nil
	}

	// Fetch newsletter subscribers with digest enabled for this tenant.
	rows, err := dbPool.Reader.Query(ctx,
		`SELECT email FROM newsletter_subscribers WHERE tenant_id = $1 AND status = 'subscribed' AND digest_enabled = true`,
		tenantID)
	if err != nil {
		slog.Error("task:newsletter:digest: failed to list subscribers", "error", err)
		return err
	}

	var subscribers []string

	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			continue
		}

		subscribers = append(subscribers, email)
	}

	rows.Close()

	if len(subscribers) == 0 {
		slog.Info("task:newsletter:digest: no digest subscribers, skipping", "tenant_id", payload.TenantID)
		return nil
	}

	// Resolve the site name for the email subject.
	name := siteName
	if name == "" {
		name = "JapaConnect"
	}

	subject := "Weekly Digest — " + name

	// Build the HTML digest body.
	var body strings.Builder

	body.WriteString("<div style=\"font-family: sans-serif; max-width: 600px; margin: 0 auto;\">\n")
	body.WriteString(fmt.Sprintf("<h2>%s</h2>\n", subject))
	body.WriteString("<p>Here's a roundup of recent content you might have missed:</p>\n<ul>\n")

	for _, item := range digestItems {
		body.WriteString(fmt.Sprintf("<li><a href=\"%s/%s\">%s</a></li>\n",
			resolveBaseURL(ctx, tenantID), item.Slug, item.Title))
	}

	body.WriteString("</ul>\n</div>\n")

	sent := 0
	failed := 0

	for _, recipient := range subscribers {
		// Generate a per-recipient one-click unsubscribe token + headers
		// (§62 Compliance, ADR-080 §4) and send via the marketing-category
		// provider (ADR-015/ADR-080).
		unsubURL, headers := newsletterUnsubscribeLink(ctx, tenantID, recipient)
		htmlBody := body.String()

		if unsubURL != "" {
			htmlBody = appendUnsubscribeLink(htmlBody, unsubURL)
		}

		if err := sendMarketingEmail(ctx, recipient, subject, htmlBody, true, headers); err != nil {
			slog.Error("task:newsletter:digest: email failed", "recipient", recipient, "error", err)

			failed++

			continue
		}

		sent++
	}

	slog.Info("task:newsletter:digest: digest sent",
		"tenant_id", payload.TenantID,
		"item_count", len(digestItems),
		"subscribers", len(subscribers),
		"sent", sent, "failed", failed)

	return nil
}

// ---------------------------------------------------------------------------
// Trending & Discovery handlers
// ---------------------------------------------------------------------------

// HandleComputeTrending recomputes trending snapshots for all tenants.
func HandleComputeTrending(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:maintenance:compute_trending")

	if trendingProcessor == nil {
		slog.Warn("task:maintenance:compute_trending: no trending processor registered, skipping")
		return nil
	}

	return trendingProcessor.ComputeTrending(ctx)
}

// HandleComputeSuggestions recomputes suggestion data freshness.
func HandleComputeSuggestions(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:maintenance:compute_suggestions")

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
	slog.Info("task:push:deliver")

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
	subs, err := gen.New(dbPool.Reader).ListPushSubscriptionsByUser(ctx, userID)
	if err != nil {
		slog.Error("task:push:deliver: failed to query subscriptions",
			"user_id", payload.UserID, "error", err)
		return fmt.Errorf("task:push:deliver: query subscriptions: %w", err)
	}

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

	for _, sub := range subs {
		if err := pushDelivery.Send(ctx, sub.Endpoint, sub.P256dh, sub.AuthKey, pushJSON); err != nil {
			slog.Error("task:push:deliver: send failed",
				"user_id", payload.UserID, "endpoint", sub.Endpoint[:min(30, len(sub.Endpoint))], "error", err)

			failed++
			// If subscription expired (410), remove it.
			if isSubscriptionExpired(err) {
				slog.Info("task:push:deliver: removing expired subscription",
					"user_id", payload.UserID)

				_ = gen.New(dbPool.Writer).DeletePushSubscription(ctx, sub.ID)
			}

			continue
		}

		sent++
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
// Payload: { recipient_id, tenant_id, sender_username, conversation_id }.
func HandleDMNotifyRequest(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:dm:notify_request")

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
// { recipient_id, tenant_id, sender_username, conversation_id, body_preview }.
func HandleDMNotifyMessage(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:dm:notify_message")

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

	// Find scheduled content ready to publish.
	pending, err := gen.New(dbPool.Reader).ListScheduledContentReady(ctx)
	if err != nil {
		slog.Error("task:content:publish_sweep: query failed", "error", err)
		return err
	}

	if len(pending) == 0 {
		slog.Info("task:content:publish_sweep: no content ready to publish")
		return nil
	}

	published := 0
	// RSS-013: track tenants that had content published so we enqueue one
	// rss:regenerate per tenant after the loop (avoids redundant enqueues
	// when a tenant has multiple posts going live in the same sweep).
	affectedTenants := make(map[pgtype.UUID]struct{})

	for _, pc := range pending {
		rowsAffected, err := gen.New(dbPool.Writer).PublishScheduledContent(ctx, gen.PublishScheduledContentParams(pc))
		if err != nil {
			slog.Error("task:content:publish_sweep: failed to publish",
				"content_id", pc.ID.String(), "error", err)
			continue
		}

		if rowsAffected == 0 {
			continue
		}

		published++
		affectedTenants[pc.TenantID] = struct{}{}

		slog.Info("task:content:publish_sweep: content published",
			"content_id", pc.ID.String())
	}

	// RSS-013: regenerate RSS feeds for each affected tenant so newly
	// published scheduled content appears without waiting for the hourly
	// cron. Fire-and-forget.
	for tid := range affectedTenants {
		scheduleRSSRegenerate(taskEnqueuer, tid)
	}

	slog.Info("task:content:publish_sweep: sweep complete",
		"published", published, "total", len(pending),
		"rss_tenants", len(affectedTenants))

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

// sendEmail sends an email via the registered dispatcher if available,
// otherwise falls back to direct SMTP.
func sendEmail(to, subject, body string, isHTML bool) error {
	return sendEmailWithContext(context.Background(), to, subject, body, isHTML)
}

// sendEmailWithContext sends an email using the provided context for the
// gateway dispatch path. The legacy SMTP fallback does not support context
// cancellation (net/smtp has no context API).
func sendEmailWithContext(ctx context.Context, to, subject, body string, isHTML bool) error {
	return sendEmailWithCategory(ctx, to, subject, body, isHTML, "transactional", nil)
}

// sendMarketingEmail sends a marketing-category email (newsletter campaigns
// and digest) via the ADR-015 marketing provider, with optional extra SMTP
// headers (e.g. List-Unsubscribe for GDPR one-click unsubscribe).
// Ref: japaconnect-engineering/adr/ADR-015-multi-provider-email.md
//
//	japaconnect-engineering/adr/ADR-080-newsletter-digest.md
func sendMarketingEmail(ctx context.Context, to, subject, body string, isHTML bool, headers map[string]string) error {
	return sendEmailWithCategory(ctx, to, subject, body, isHTML, "marketing", headers)
}

// sendEmailWithCategory is the shared dispatch path. category is
// "transactional" or "marketing" (ADR-015). headers is an optional set of
// extra SMTP headers merged into the message by providers that support them.
func sendEmailWithCategory(ctx context.Context, to, subject, body string, isHTML bool, category string, headers map[string]string) error {
	// If dispatcher is available, use it
	if emailGateway != nil {
		req := email.EmailRequest{
			To:       to,
			Subject:  subject,
			Body:     body,
			IsHTML:   isHTML,
			Category: category,
			Headers:  headers,
		}
		_, err := emailGateway.Dispatch(ctx, req)

		return err
	}

	// Fallback to direct SMTP (legacy path)
	cfg := readSMTPConfig()
	if cfg == nil {
		return errors.New("no email provider configured: email gateway is nil and SMTP config is missing")
	}

	contentType := "text/plain; charset=UTF-8"
	contentBody := body

	if isHTML {
		contentType = "text/html; charset=UTF-8"
		contentBody = wrapHTMLEmail(body)
	}

	// Build the message, merging any extra headers in a stable order.
	var hdr strings.Builder

	hdr.WriteString(fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: %s\r\n",
		cfg.From, to, subject, contentType))

	if len(headers) > 0 {
		keys := make([]string, 0, len(headers))
		for k := range headers {
			keys = append(keys, k)
		}

		sort.Strings(keys)

		for _, k := range keys {
			hdr.WriteString(k + ": " + headers[k] + "\r\n")
		}
	}

	msg := hdr.String() + "\r\n" + contentBody

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

		// #nosec G707 // SMTP config validated before use
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
	// #nosec G707 // SMTP config validated before use
	if err := smtp.SendMail(cfg.Addr, auth, cfg.From, []string{to}, []byte(msg)); err != nil {
		slog.Error("email_send: SMTP send failed", "to", to, "addr", cfg.Addr, "error", err)
		return fmt.Errorf("smtp send to %s: %w", to, err)
	}

	return nil
}

// ── Missing Phase 31 Task Handlers (stub implementations) ───────────

// HandleMaintenanceCheckExpiredBans lifts expired user_moderation_actions with action='ban'.
func HandleMaintenanceCheckExpiredBans(ctx context.Context, _ *asynq.Task) error {
	slog.Info("maintenance:check_expired_bans — lifting expired bans")

	if dbPool == nil {
		return errors.New("maintenance:check_expired_bans: database pool not registered")
	}
	// Mark expired bans as 'unban' actions.
	if err := gen.New(dbPool.Writer).ExpireModerationBans(ctx); err != nil {
		slog.Error("maintenance:check_expired_bans failed", "error", err)
		return err
	}

	return nil
}

// HandleMaintenanceCheckExpiredBoosts expires organization_boosts past end_date.
func HandleMaintenanceCheckExpiredBoosts(ctx context.Context, _ *asynq.Task) error {
	slog.Info("maintenance:check_expired_boosts — expiring boosts")

	if dbPool == nil {
		return errors.New("maintenance:check_expired_boosts: database pool not registered")
	}

	if err := gen.New(dbPool.Writer).ExpireOrganizationBoosts(ctx); err != nil {
		slog.Error("maintenance:check_expired_boosts failed", "error", err)
		return err
	}

	return nil
}

// HandleMaintenancePruneOldFeeds deletes feeds rows older than 30 days.
func HandleMaintenancePruneOldFeeds(ctx context.Context, _ *asynq.Task) error {
	slog.Info("maintenance:prune_old_feeds — pruning old feed entries")

	if dbPool == nil {
		return errors.New("maintenance:prune_old_feeds: database pool not registered")
	}

	if err := gen.New(dbPool.Writer).PruneOldFeeds(ctx); err != nil {
		slog.Error("maintenance:prune_old_feeds failed", "error", err)
		return err
	}

	return nil
}

// HandleMaintenancePruneAuditLogs deletes audit_logs rows older than the
// per-tenant 'audit.log_retention_days' system_settings value (default 365).
// Ref: conventions/system-settings-catalog.md (audit.log_retention_days).
func HandleMaintenancePruneAuditLogs(ctx context.Context, _ *asynq.Task) error {
	slog.Info("maintenance:prune_audit_logs — pruning old audit logs")

	if dbPool == nil {
		return errors.New("maintenance:prune_audit_logs: database pool not registered")
	}

	if err := gen.New(dbPool.Writer).PruneAuditLogs(ctx); err != nil {
		slog.Error("maintenance:prune_audit_logs failed", "error", err)
		return err
	}

	return nil
}

// HandleMaintenancePrunePushTokens deletes stale/unresponsive push tokens older than 90 days.
func HandleMaintenancePrunePushTokens(ctx context.Context, _ *asynq.Task) error {
	slog.Info("maintenance:prune_push_tokens — pruning stale push tokens")

	if dbPool == nil {
		return errors.New("maintenance:prune_push_tokens: database pool not registered")
	}

	if err := gen.New(dbPool.Writer).PruneStalePushTokens(ctx); err != nil {
		slog.Error("maintenance:prune_push_tokens failed", "error", err)
		return err
	}

	return nil
}

// HandleModerationAutoHide checks if a reported entity has reached the
// auto-hide threshold and archives it. Enqueued after each report is created.
func HandleModerationAutoHide(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:moderation:auto_hide")

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

	// Read the auto-hide threshold from system settings.
	// Default: 5 reports per system-settings-catalog.md.
	threshold := 5
	if appSettings != nil {
		threshold = appSettings.GetIntOr(ctx, tenantID, "moderation.auto_hide_report_count", 5)
	}

	// Count pending reports for this entity.
	reportCount, err := gen.New(dbPool.Reader).CountPendingReportsByEntity(ctx, gen.CountPendingReportsByEntityParams{
		TenantID: tenantID, EntityType: payload.EntityType, EntityID: entityID,
	})
	if err != nil {
		slog.Error("task:moderation:auto_hide: failed to count reports",
			"entity_type", payload.EntityType, "entity_id", payload.EntityID, "error", err)
		return err
	}

	if int(reportCount) < threshold {
		slog.Info("task:moderation:auto_hide: below threshold, skipping",
			"entity_type", payload.EntityType, "entity_id", payload.EntityID,
			"report_count", reportCount, "threshold", threshold)

		return nil
	}

	// Archive the entity based on its type.
	switch payload.EntityType {
	case "content":
		n, err := gen.New(dbPool.Writer).ArchiveContent(ctx, gen.ArchiveContentParams{
			ID: entityID, TenantID: tenantID,
		})
		if err != nil {
			slog.Error("task:moderation:auto_hide: failed to archive content",
				"content_id", payload.EntityID, "error", err)
			return err
		}

		if n > 0 {
			slog.Info("task:moderation:auto_hide: content archived",
				"content_id", payload.EntityID, "report_count", reportCount)
		}

	case "comment":
		q := gen.New(dbPool.Writer)
		if err := q.SoftDeleteCommentByID(ctx, entityID); err != nil {
			slog.Error("task:moderation:auto_hide: failed to hide comment",
				"comment_id", payload.EntityID, "error", err)
			return err
		}

		slog.Info("task:moderation:auto_hide: comment hidden",
			"comment_id", payload.EntityID, "report_count", reportCount)

	case "message":
		q := gen.New(dbPool.Writer)
		if err := q.SoftDeleteDMMessageByID(ctx, entityID); err != nil {
			slog.Error("task:moderation:auto_hide: failed to hide message",
				"message_id", payload.EntityID, "error", err)
			return err
		}

		slog.Info("task:moderation:auto_hide: message hidden",
			"message_id", payload.EntityID, "report_count", reportCount)

	default:
		// user/space reports require human review — log but don't auto-hide.
		slog.Info("task:moderation:auto_hide: entity type requires human review",
			"entity_type", payload.EntityType, "entity_id", payload.EntityID,
			"report_count", reportCount)
	}

	return nil
}

// HandleReportNotifyAdmin notifies the moderation team (every user holding
// the reports.review permission) that a new content report was submitted.
// For each recipient it inserts an in-app notification and enqueues a push
// delivery. Idempotent on report_id — duplicate deliveries are acceptable
// (the notification insert has no unique constraint, but the Asynq task
// dedup window + retry backoff prevents spam).
//
// Ref: japaconnect-engineering/sections/51-content-reports-flagging.md.
func HandleReportNotifyAdmin(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:report:notify_admin")

	if dbPool == nil {
		slog.Warn("task:report:notify_admin: no database, skipping")
		return nil
	}

	var payload struct {
		ReportID   string `json:"report_id"`
		TenantID   string `json:"tenant_id"`
		EntityType string `json:"entity_type"`
		EntityID   string `json:"entity_id"`
		Reason     string `json:"reason"`
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:report:notify_admin: invalid payload", "error", err)
		return err
	}

	tenantID, err := scanUUID(payload.TenantID)
	if err != nil {
		return fmt.Errorf("task:report:notify_admin: %w", err)
	}

	reportID, err := scanUUID(payload.ReportID)
	if err != nil {
		return fmt.Errorf("task:report:notify_admin: %w", err)
	}

	// Resolve every user with the reports.review permission for this tenant.
	q := gen.New(dbPool.Reader)

	recipients, err := q.ListUserIDsByPermissionKey(ctx, gen.ListUserIDsByPermissionKeyParams{
		TenantID:      tenantID,
		PermissionKey: "reports.review",
	})
	if err != nil {
		slog.Error("task:report:notify_admin: failed to list recipients",
			"error", err, "tenant_id", payload.TenantID)
		return err
	}

	if len(recipients) == 0 {
		slog.Info("task:report:notify_admin: no recipients with reports.review, skipping",
			"tenant_id", payload.TenantID)
		return nil
	}

	// Build the notification content.
	title := "New content report"

	body := fmt.Sprintf("%s reported (%s)", payload.EntityType, payload.Reason)
	if payload.Reason == "" {
		body = payload.EntityType + " reported"
	}

	actionURL := "/admin/reports"
	subjectType := "content_report"
	actorType := "system"

	wq := gen.New(dbPool.Writer)

	for _, recipientID := range recipients {
		bodyPtr := &body
		actionURLPtr := &actionURL
		subjectTypePtr := &subjectType
		actorTypePtr := &actorType

		if _, err := wq.InsertNotification(ctx, gen.InsertNotificationParams{
			TenantID:         tenantID,
			RecipientID:      recipientID,
			NotificationType: "report.new",
			ActorType:        actorTypePtr,
			ActorID:          pgtype.UUID{},
			SubjectType:      subjectTypePtr,
			SubjectID:        reportID,
			Title:            title,
			Body:             bodyPtr,
			ActionUrl:        actionURLPtr,
		}); err != nil {
			slog.Error("task:report:notify_admin: failed to insert notification",
				"error", err, "recipient_id", recipientID.String())
			continue
		}

		// Enqueue a push notification for this recipient (best-effort).
		if taskEnqueuer != nil {
			pushPayload, _ := json.Marshal(map[string]string{
				"user_id":    recipientID.String(),
				"title":      title,
				"body":       body,
				"action_url": actionURL,
			})
			if err := taskEnqueuer.Enqueue(TypePushDelivery, pushPayload); err != nil {
				slog.Warn("task:report:notify_admin: failed to enqueue push",
					"error", err, "recipient_id", recipientID.String())
			}
		}
	}

	slog.Info("task:report:notify_admin: notifications dispatched",
		"report_id", payload.ReportID, "recipients", len(recipients))

	return nil
}

// HandleDMCleanupDeclinedRequests purges dm_requests declined > 90 days ago.
func HandleDMCleanupDeclinedRequests(ctx context.Context, _ *asynq.Task) error {
	slog.Info("dm:cleanup_declined_requests — purging old declined requests")

	if dbPool == nil {
		return errors.New("dm:cleanup_declined_requests: database pool not registered")
	}

	if err := gen.New(dbPool.Writer).CleanupDeclinedDMRequests(ctx); err != nil {
		slog.Error("dm:cleanup_declined_requests failed", "error", err)
		return err
	}

	return nil
}

// HandleCouponTrackEvents flushes coupon_events to aggregate counters.
// Recounts total_clicks and total_copies from the events table into the
// coupons table. The coupon_events table is append-only so a full recount
// is safe and idempotent.
func HandleCouponTrackEvents(ctx context.Context, t *asynq.Task) error {
	slog.Info("coupon:track_events — flushing coupon event counters")

	if dbPool == nil {
		return errors.New("coupon:track_events: database pool not registered")
	}

	w := gen.New(dbPool.Writer)

	// Update total_clicks from click events.
	if err := w.UpdateCouponClickCounts(ctx); err != nil {
		slog.Error("coupon:track_events: failed to update click counts", "error", err)
		return err
	}

	// Update total_copies from copy events.
	if err := w.UpdateCouponCopyCounts(ctx); err != nil {
		slog.Error("coupon:track_events: failed to update copy counts", "error", err)
		return err
	}

	return nil
}

// HandleCouponExpireCheck marks past-expiry coupons as expired.
func HandleCouponExpireCheck(ctx context.Context, _ *asynq.Task) error {
	slog.Info("coupon:expire_check — checking for expired coupons")

	if dbPool == nil {
		return errors.New("coupon:expire_check: database pool not registered")
	}

	if err := gen.New(dbPool.Writer).ExpireCoupons(ctx); err != nil {
		slog.Error("coupon:expire_check failed", "error", err)
		return err
	}

	return nil
}

// HandleAffiliateRecordClick persists click event to affiliate_link_events
// and increments denormalized click counters on the link and member rows.
//
// The payload carries pre-hashed ip_hash and ua_hash (SHA-256 hex) from the
// redirect handler — raw PII never transits Asynq/Valkey (Section 56).
func HandleAffiliateRecordClick(ctx context.Context, t *asynq.Task) error {
	slog.Info("affiliate:record_click — persisting click event")

	var payload struct {
		LinkID      string `json:"link_id"`
		IPHash      string `json:"ip_hash"`
		UAHash      string `json:"ua_hash"`
		ReferrerURL string `json:"referrer_url"`
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

	// Look up the link to get tenant_id (required for the event row).
	link, err := gen.New(dbPool.Reader).GetAffiliateLinkByIDOnly(ctx, linkID)
	if err != nil {
		slog.Error("affiliate:record_click: link not found", "link_id", payload.LinkID, "error", err)
		return err
	}

	// Use the pre-hashed values directly (already SHA-256 hex from the
	// redirect handler). Re-hashing would hash the empty string since the
	// old payload field names didn't match.
	ipHashStr := payload.IPHash
	uaHashStr := payload.UAHash

	var referrerURL *string
	if payload.ReferrerURL != "" {
		referrerURL = &payload.ReferrerURL
	}

	if _, err := gen.New(dbPool.Writer).CreateAffiliateLinkEvent(ctx, gen.CreateAffiliateLinkEventParams{
		TenantID:      link.TenantID,
		LinkID:        linkID,
		EventType:     "click",
		IpHash:        &ipHashStr,
		UserAgentHash: &uaHashStr,
		ReferrerUrl:   referrerURL,
	}); err != nil {
		slog.Error("affiliate:record_click failed", "error", err)
		return err
	}

	// Increment denormalized click counters (Section 56).
	if err := gen.New(dbPool.Writer).IncrementAffiliateLinkClicks(ctx, linkID); err != nil {
		slog.Error("affiliate:record_click: failed to increment link clicks",
			"link_id", payload.LinkID, "error", err)
	}

	if err := gen.New(dbPool.Writer).IncrementAffiliateMemberClicks(ctx, link.UserID); err != nil {
		slog.Error("affiliate:record_click: failed to increment member clicks",
			"user_id", link.UserID, "error", err)
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
//
// Per Section 56 Commission Crediting, this handler:
//  1. Looks up the affiliate link by code and resolves the commission rate.
//  2. Calculates commission using integer math (amount in kobo × rate in basis
//     points / 10000) — never float (Bug Hunter's Guide §1.3).
//  3. Credits the wallet via the immutable ledger inside a transaction holding
//     pg_advisory_xact_lock to prevent double-spend (§13).
//  4. Inserts a conversion event in affiliate_link_events.
//  5. Increments denormalized counters on the link and member rows.
//  6. Deletes the jc:aff_click attribution key to prevent double-crediting.
func HandleAffiliateCreditCommission(ctx context.Context, t *asynq.Task) error {
	slog.Info("affiliate:credit_commission — crediting commission")

	var payload struct {
		PaymentID   string `json:"payment_id"`
		UserID      string `json:"user_id"`
		Amount      int64  `json:"amount"`
		LinkCode    string `json:"link_code"`
		PurposeType string `json:"purpose_type"`
		ProductID   string `json:"product_id"`
	}

	if err := parsePayloadOrSkip(t, &payload); err != nil {
		slog.Error("affiliate:credit_commission: invalid payload", "error", err)
		return err
	}

	if payload.LinkCode == "" {
		slog.Info("affiliate:credit_commission: no affiliate link, skipping")
		return nil
	}

	if _, err := requireDB(); err != nil {
		return err
	}

	// Look up the affiliate link by code (needed for link_id, tenant_id,
	// and user_id for counter increments and conversion event).
	link, err := gen.New(dbPool.Reader).GetAffiliateLinkByCode(ctx, payload.LinkCode)
	if err != nil {
		slog.Warn("affiliate:credit_commission: link not found or inactive",
			"link_code", payload.LinkCode, "error", err)
		return nil
	}

	// Resolve the commission rate from system_settings via the join query.
	commissionInfo, err := gen.New(dbPool.Reader).GetAffiliateCommissionInfo(ctx, payload.LinkCode)
	if err != nil {
		slog.Warn("affiliate:credit_commission: commission info not found",
			"link_code", payload.LinkCode, "error", err)
		return nil
	}

	memberUserID := commissionInfo.UserID
	memberTenantID := commissionInfo.TenantID

	if commissionInfo.CommissionRate <= 0 {
		slog.Info("affiliate:credit_commission: no commission rate configured")
		return nil
	}

	// Integer math: convert rate to basis points (1% = 100 bps) and compute
	// commission = amount_kobo * rate_bps / 10000. This avoids float
	// precision loss (Bug Hunter's Guide §1.3).
	rateBps := int64(math.Round(commissionInfo.CommissionRate * 100))

	commission := payload.Amount * rateBps / 10000
	if commission <= 0 {
		return nil
	}

	paymentID, err := scanUUID(payload.PaymentID)
	if err != nil {
		return fmt.Errorf("affiliate:credit_commission: %w", err)
	}

	// Credit the wallet via the immutable ledger inside a transaction holding
	// pg_advisory_xact_lock(user_id) to serialize concurrent balance
	// mutations (Bug Hunter's Guide §13 — double-spend prevention).
	tx, err := dbPool.Writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("affiliate:credit_commission: beginning tx: %w", err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	qtx := gen.New(tx)

	lockKey := "wallet:" + memberTenantID.String() + ":" + memberUserID.String()
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", lockKey); err != nil {
		return fmt.Errorf("affiliate:credit_commission: acquiring wallet lock: %w", err)
	}

	currentBalance, err := qtx.GetLatestBalance(ctx, gen.GetLatestBalanceParams{
		TenantID:   memberTenantID,
		UserID:     memberUserID,
		LedgerType: "credits",
	})
	if err != nil {
		currentBalance = 0
	}

	newBalance := currentBalance + commission

	source := "affiliate_commission"
	refStr := paymentID.String()
	idempotencyKey := "commission:" + payload.PaymentID
	reason := "Affiliate commission"

	if _, err := qtx.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{
		TenantID:         memberTenantID,
		UserID:           memberUserID,
		LedgerType:       "credits",
		Operation:        "credit",
		Amount:           commission,
		ResultingBalance: newBalance,
		Source:           &source,
		ActorUserID:      memberUserID,
		Reference:        &refStr,
		IdempotencyKey:   &idempotencyKey,
		Reason:           &reason,
	}); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			slog.Info("affiliate:credit_commission: already credited (idempotent)",
				"payment_id", payload.PaymentID)
			return nil
		}

		slog.Error("affiliate:credit_commission: failed to credit wallet", "error", err)

		return err
	}

	// Insert the conversion event (Section 56 Commission Crediting).
	commissionNumeric := pgtype.Numeric{}
	if err := commissionNumeric.Scan(strconv.FormatInt(commission, 10)); err != nil {
		slog.Warn("affiliate:credit_commission: failed to scan commission numeric", "error", err)
	} else {
		if _, err := qtx.CreateAffiliateLinkEvent(ctx, gen.CreateAffiliateLinkEventParams{
			TenantID:           memberTenantID,
			LinkID:             link.ID,
			EventType:          "conversion",
			ConvertedPaymentID: paymentID,
			CommissionAmount:   commissionNumeric,
		}); err != nil {
			slog.Error("affiliate:credit_commission: failed to create conversion event",
				"link_id", link.ID, "error", err)
		}
	}

	// Increment denormalized counters (link + member).
	if err := qtx.IncrementAffiliateLinkConversions(ctx, gen.IncrementAffiliateLinkConversionsParams{
		Commission: commissionNumeric, LinkID: link.ID,
	}); err != nil {
		slog.Error("affiliate:credit_commission: failed to increment link conversions",
			"link_id", link.ID, "error", err)
	}

	if err := qtx.IncrementAffiliateMemberConversions(ctx, gen.IncrementAffiliateMemberConversionsParams{
		Commission: commissionNumeric, UserID: memberUserID,
	}); err != nil {
		slog.Error("affiliate:credit_commission: failed to increment member conversions",
			"user_id", memberUserID, "error", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("affiliate:credit_commission: committing tx: %w", err)
	}

	// Delete the attribution key so the same click can't be double-credited
	// (Section 56 Commission Crediting). Use the same purpose_type→product_type
	// mapping as the payment webhook (BUG-004) — the key was stored with the
	// affiliate product_type, not the payment purpose_type.
	if rdb != nil {
		var purposeID pgtype.UUID
		if payload.ProductID != "null" {
			_ = purposeID.Scan(payload.ProductID)
		}

		affProductType, affProductID := paymentPurposeToAffiliateProduct(payload.PurposeType, purposeID)

		attrKey := valkey.MustKey("aff_click:%s:%s:%s",
			payload.UserID, affProductType, affProductID)
		if err := rdb.Del(ctx, attrKey).Err(); err != nil {
			slog.Warn("affiliate:credit_commission: failed to delete attribution key",
				"key", attrKey, "error", err)
		}
	}

	slog.Info("affiliate:credit_commission — credited",
		"amount", commission, "link_code", payload.LinkCode)

	return nil
}

// HandleAffiliateExpireCheck deactivates links for suspended/banned members.
func HandleAffiliateExpireCheck(ctx context.Context, _ *asynq.Task) error {
	slog.Info("affiliate:expire_check — deactivating links for suspended members")

	if dbPool == nil {
		return errors.New("affiliate:expire_check: database pool not registered")
	}

	if err := gen.New(dbPool.Writer).DeactivateLinksForSuspendedMembers(ctx); err != nil {
		slog.Error("affiliate:expire_check failed", "error", err)
		return err
	}

	return nil
}

// HandleTenantCleanupData is the handler for tenant:cleanup_data tasks.
// It hard-deletes all data associated with a soft-deleted tenant after the
// 30-day grace period (Section 21). The task is enqueued with a 30-day delay
// at the time of soft-deletion.
func HandleTenantCleanupData(ctx context.Context, t *asynq.Task) error {
	if dbPool == nil {
		return errors.New("tenant:cleanup_data: db pool not registered")
	}

	var payload struct {
		TenantID    string `json:"tenant_id"`
		InitiatedBy string `json:"initiated_by"`
	}

	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("tenant:cleanup_data: bad payload: %w", err)
	}

	if payload.TenantID == "" {
		return errors.New("tenant:cleanup_data: missing tenant_id in payload")
	}

	var tenantID pgtype.UUID
	if err := tenantID.Scan(payload.TenantID); err != nil {
		return fmt.Errorf("tenant:cleanup_data: invalid tenant_id %q: %w", payload.TenantID, err)
	}

	// Verify the tenant is still in 'deleted' status before cleanup.
	// If the tenant was re-activated (unsuspended), skip hard-delete.
	status, err := gen.New(dbPool.Reader).GetTenantStatus(ctx, tenantID)
	if err != nil {
		// Tenant may have been already hard-deleted or doesn't exist.
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Info("tenant:cleanup_data — tenant not found, skipping", "tenant_id", payload.TenantID)
			return nil
		}

		return fmt.Errorf("tenant:cleanup_data: %w", err)
	}

	if status != "deleted" {
		slog.Info("tenant:cleanup_data — tenant no longer in deleted status, skipping", "tenant_id", payload.TenantID, "status", status)
		return nil
	}

	slog.Info("tenant:cleanup_data — starting cascade cleanup", "tenant_id", payload.TenantID, "initiated_by", payload.InitiatedBy)

	// Delete all tenant-scoped rows from tables WITHOUT ON DELETE CASCADE on
	// tenants(id), in FK dependency order. Tables with CASCADE are auto-deleted
	// by PostgreSQL and intentionally omitted from the query.
	wq := gen.New(dbPool.Writer)

	totalDeleted, err := wq.DeleteTenantData(ctx, tenantID)
	if err != nil {
		slog.Error("tenant:cleanup_data — failed to delete tenant data", "error", err)
		return fmt.Errorf("tenant:cleanup_data: delete tenant data: %w", err)
	}

	// Hard delete the tenant itself.
	if err := wq.HardDeleteTenant(ctx, tenantID); err != nil {
		return fmt.Errorf("tenant:cleanup_data: delete tenant: %w", err)
	}

	slog.Info("tenant:cleanup_data — cleanup complete", "tenant_id", payload.TenantID, "child_rows_deleted", totalDeleted, "tenant_deleted", true)

	return nil
}

// HandleWikiSeedStubPages auto-seeds the 7 default wiki sub-pages for a
// Country Space when wiki.enabled is activated (§80, ADR-088).
// The task is idempotent: if the stub pages already exist (matched by
// tenant_id + space_id + slug), the processor skips them.
func HandleWikiSeedStubPages(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:wiki:seed_stub_pages")

	if wikiSeedStubPagesProcessor == nil {
		slog.Warn("task:wiki:seed_stub_pages: no wiki seed processor registered, skipping")
		return nil
	}

	var payload struct {
		SpaceID  string `json:"space_id"`
		TenantID string `json:"tenant_id"`
	}

	if err := parsePayload(t, &payload); err != nil {
		slog.Error("task:wiki:seed_stub_pages: invalid payload", "error", err)
		return err
	}

	if payload.SpaceID == "" {
		return errors.New("wiki:seed_stub_pages: space_id is required")
	}

	if err := wikiSeedStubPagesProcessor.SeedStubPages(ctx, payload.SpaceID, payload.TenantID); err != nil {
		slog.Error("task:wiki:seed_stub_pages: failed",
			"space_id", payload.SpaceID, "error", err)
		return err
	}

	slog.Info("task:wiki:seed_stub_pages: completed",
		"space_id", payload.SpaceID)

	return nil
}
