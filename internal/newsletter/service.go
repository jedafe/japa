// Package newsletter — service.go provides business logic for the Newsletter
// & Digest System (Section 62, ADR-080). Covers subscriber management,
// campaign CRUD, one-click unsubscribe tokens, subscriber preferences,
// and weekly digest support.
// // Ref: japaconnect-engineering/sections/62-newsletter-digest.md
// //	japaconnect-engineering/adr/ADR-080-newsletter-digest.md
package newsletter

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/worker"
	valkeycompat "github.com/valkey-io/valkey-go/valkeycompat"
)

// Subscriber status constants.
const (
	StatusPending      = "pending" // double opt-in, awaiting confirm (migration 001170)
	StatusSubscribed   = "subscribed"
	StatusUnsubscribed = "unsubscribed"
	StatusBounced      = "bounced"
	StatusComplained   = "complained"
)

// Campaign status constants.
const (
	CampaignDraft     = "draft"
	CampaignScheduled = "scheduled"
	CampaignSending   = "sending"
	CampaignSent      = "sent"
	CampaignCancelled = "cancelled"
)

// Segment constants.
const (
	SegmentAll            = "all"
	SegmentSpace          = "space"
	SegmentTag            = "tag"
	SegmentProSubscribers = "pro_subscribers"
)

// Source constants.
const (
	SourceSignup     = "signup"
	SourceOnboarding = "onboarding"
	SourceFooter     = "footer"
	SourceImport     = "import"
	SourceAPI        = "api"
)

// Task name constants (Section 11 catalog).
const (
	TaskNewsletterSend   = "newsletter:send"
	TaskNewsletterDigest = "newsletter:digest"
)

// Subscriber represents a newsletter subscriber.
type Subscriber struct {
	ID pgtype.UUID `json:"id"`

	Email           string             `json:"email"`
	UserID          *pgtype.UUID       `json:"user_id,omitempty"`
	Status          string             `json:"status"`
	DigestEnabled   bool               `json:"digest_enabled"`
	CampaignEnabled bool               `json:"campaign_enabled"`
	SubscribedAt    pgtype.Timestamptz `json:"subscribed_at"`
	UnsubscribedAt  pgtype.Timestamptz `json:"unsubscribed_at,omitempty"`
	Source          string             `json:"source"`
	CreatedAt       pgtype.Timestamptz `json:"created_at"`
}

// Campaign represents a newsletter campaign.
type Campaign struct {
	ID pgtype.UUID `json:"id"`

	Name           string              `json:"name"`
	Subject        string              `json:"subject"`
	HTML           string              `json:"html"`
	PreviewText    *string             `json:"preview_text,omitempty"`
	Body           json.RawMessage     `json:"body"`
	Segment        string              `json:"segment"`
	SegmentID      pgtype.UUID         `json:"segment_id,omitempty"`
	Status         string              `json:"status"`
	ScheduledAt    pgtype.Timestamptz  `json:"scheduled_at,omitempty"`
	SentAt         *pgtype.Timestamptz `json:"sent_at,omitempty"`
	RecipientCount int32               `json:"recipient_count"`
	OpenCount      int32               `json:"open_count"`
	ClickCount     int32               `json:"click_count"`
	CreatedBy      pgtype.UUID         `json:"created_by,omitempty"`
	CreatedAt      pgtype.Timestamptz  `json:"created_at"`
	UpdatedAt      pgtype.Timestamptz  `json:"updated_at"`
}

// FeatureChecker resolves feature flag state from the feature_flags table —
// the single source of truth for feature reachability.
type FeatureChecker interface {
	IsEnabled(ctx context.Context, key string) bool
}

// Service coordinates newsletter subscriber and campaign management,
// unsubscribe tokens, subscriber preferences, and weekly digest support.
type Service struct {
	db           *db.DB
	queries      *gen.Queries
	writeQueries *gen.Queries
	client       *asynq.Client
	settings     *config.Settings
	checker      FeatureChecker
	vk           valkeycompat.Cmdable
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{
		db:           database,
		queries:      gen.New(database.Reader),
		writeQueries: gen.New(database.Writer),
	}
}

// SetAsynqClient injects the Asynq client for enqueuing newsletter send tasks.
func (s *Service) SetAsynqClient(client *asynq.Client) {
	s.client = client
}

// SetSettings injects the config.Settings provider.
func (s *Service) SetSettings(settings *config.Settings) { s.settings = settings }

// SetFeatureChecker injects the feature flag checker so Enabled reads the
// feature_flags table (the single source of truth for feature reachability,
// matching the feature.newsletter route gate) instead of system_settings.
// Falls back to the legacy newsletter.enabled system_setting when nil.
func (s *Service) SetFeatureChecker(c FeatureChecker) { s.checker = c }

// Enabled reads the feature.newsletter flag (ADR-080, §62) — the same key the
// route gate enforces. Falls back to the legacy newsletter.enabled
// system_setting when no checker is injected; the fallback defaults to
// false — an unwired checker must fail closed (audit NL-I2), matching the
// feature_flags registry default.
func (s *Service) Enabled(ctx context.Context) bool {
	if s.checker != nil {
		return s.checker.IsEnabled(ctx, "feature.newsletter")
	}

	if s.settings == nil {
		return false
	}

	return s.settings.GetBoolOr(ctx, "newsletter.enabled", false)
}

// SetValkey injects the Valkey client used for webhook idempotency keys
// (jc:newsletter_webhook_evt:*, valkey-keys-catalog.md, Rule 4).
func (s *Service) SetValkey(vk valkeycompat.Cmdable) { s.vk = vk }

// WebhookDedupe returns the Valkey client used for webhook dedupe, or nil.
func (s *Service) WebhookDedupe() valkeycompat.Cmdable { return s.vk }

// WebhookSecret returns the shared secret used to authenticate provider
// bounce/complaint webhooks (§62 Compliance, ADR-080 §4). Read from the
// JC_NEWSLETTER_WEBHOOK_SECRET environment variable. An empty string means
// webhooks are rejected (fail-closed).
func (s *Service) WebhookSecret(_ context.Context) string {
	return os.Getenv("JC_NEWSLETTER_WEBHOOK_SECRET")
}

// ClaimSend atomically transitions a draft|scheduled campaign to 'sending'
// and stamps a fresh send_id lease (§62). The send endpoint calls this
// BEFORE enqueueing the task: a winning claim owns the send and guarantees
// the queued task finds 'sending'; a losing claim means another send is in
// flight or the campaign is in a terminal state, so the caller returns 409
// instead of queueing a duplicate task that could double-send. Returns
// (true, nil) when the claim won.
// Returns (claimed, prevStatus, error) — prevStatus is the exact status the
// claim displaced (draft or scheduled), for RevertSendClaim on enqueue
// failure. Using the claim's own pre-image — not the handler's earlier
// fetch — means a concurrent PATCH in the fetch→claim gap is restored
// faithfully rather than silently undone.
func (s *Service) ClaimSend(ctx context.Context, campaignID pgtype.UUID, sendID string) (bool, string, error) {
	row, err := s.writeQueries.ClaimNewsletterSendNow(ctx, gen.ClaimNewsletterSendNowParams{
		ID:     campaignID,
		SendID: &sendID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, "", nil
		}

		return false, "", fmt.Errorf("claiming newsletter send: %w", err)
	}

	return true, row.PrevStatus, nil
}

// RevertSendClaim undoes a send claim after a failed enqueue — restores the
// pre-claim status (draft or scheduled), guarded by status='sending' so it
// can never clobber a state that arrived after the claim.
func (s *Service) RevertSendClaim(ctx context.Context, campaignID pgtype.UUID, sendID, status string) {
	if err := s.writeQueries.RevertNewsletterSendClaim(ctx, gen.RevertNewsletterSendClaimParams{
		ID: campaignID, Status: status, SendID: &sendID,
	}); err != nil {
		slog.Warn("newsletter: revert send claim failed", "campaign_id", campaignID.String(), "error", err)
	}
}

// EnqueueNewsletterSend enqueues a newsletter_send Asynq task. The worker
// re-fetches the campaign and resolves segment-aware recipients from the
// DB, so only the IDs are carried in the payload (§62, ADR-080 §3).
func (s *Service) EnqueueNewsletterSend(_ context.Context, campaignID pgtype.UUID, sendID string) error {
	if s.client == nil {
		return errors.New("asynq client not configured")
	}

	payload, err := json.Marshal(map[string]any{
		"newsletter_id": campaignID.String(),
		// The lease this send was claimed under — the worker proceeds only
		// while the campaign's send_id still matches, so a stale task
		// can't mail for a send that was reset/re-claimed (§62).
		"send_id": sendID,
	})
	if err != nil {
		return fmt.Errorf("marshalling newsletter payload: %w", err)
	}

	task := asynq.NewTask(worker.TypeNewsletterSend, payload)

	info, err := s.client.Enqueue(task, asynq.Queue(worker.QueueForTask(worker.TypeNewsletterSend)), asynq.Unique(24*time.Hour))
	if err != nil {
		return fmt.Errorf("enqueuing newsletter send: %w", err)
	}

	slog.Info("newsletter: send task enqueued", "campaign_id", campaignID.String(), "task_id", info.ID)

	return nil
}

// EnqueueNewsletterSendAt schedules a newsletter_send task for a future time
// (§62 campaign scheduling). The worker re-fetches the campaign at execution
// time and resolves segment-aware recipients — only the ID travels in the
// payload. Dedupes on (campaign_id, scheduled_at) so re-scheduling to a new
// time enqueues a fresh task.
func (s *Service) EnqueueNewsletterSendAt(_ context.Context, campaignID pgtype.UUID, at time.Time) error {
	if s.client == nil {
		return errors.New("asynq client not configured")
	}

	payload, err := json.Marshal(map[string]any{
		"newsletter_id": campaignID.String(),
		// Carried in the payload so (a) Unique() doesn't collapse a
		// reschedule onto the old task's fingerprint and (b) the worker's
		// claim can reject stale tasks whose campaign was rescheduled/
		// cancelled. Truncated to seconds — RFC3339 has no sub-second
		// precision and the claim compares date_trunc('second', ...).
		"scheduled_at": at.UTC().Truncate(time.Second).Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("marshalling newsletter payload: %w", err)
	}

	task := asynq.NewTask(worker.TypeNewsletterSend, payload)

	info, err := s.client.Enqueue(task,
		asynq.Queue(worker.QueueForTask(worker.TypeNewsletterSend)),
		asynq.ProcessAt(at),
		asynq.Unique(24*time.Hour),
	)
	if err != nil {
		return fmt.Errorf("enqueuing scheduled newsletter send: %w", err)
	}

	slog.Info("newsletter: scheduled send enqueued", "campaign_id", campaignID.String(), "at", at, "task_id", info.ID)

	return nil
}

// ---------------------------------------------------------------------------
// Subscriber Operations
// ---------------------------------------------------------------------------

// ListSubscribersFiltered returns newsletter subscribers for the platform,
// optionally filtered by status and email search (§62 admin route
// ?status=&search=). Uses the AdminListSubscribers sqlc query.
func (s *Service) ListSubscribersFiltered(ctx context.Context, status, search string, limit, offset int32) ([]gen.NewsletterSubscriber, error) {
	return s.queries.AdminListSubscribers(ctx, gen.AdminListSubscribersParams{
		Status:     status,
		Search:     search,
		PageLimit:  limit,
		PageOffset: offset,
	})
}

// GetSubscriber returns a single newsletter subscriber by ID.
func (s *Service) GetSubscriber(ctx context.Context, id pgtype.UUID) (Subscriber, error) {
	var sub Subscriber

	err := s.db.Reader.QueryRow(ctx, `
		SELECT id, email, user_id, status, digest_enabled, campaign_enabled,
		       subscribed_at, unsubscribed_at, source, created_at
		FROM newsletter_subscribers
		WHERE id = $1
	`, id).Scan(&sub.ID, &sub.Email, &sub.UserID, &sub.Status,
		&sub.DigestEnabled, &sub.CampaignEnabled, &sub.SubscribedAt, &sub.UnsubscribedAt,
		&sub.Source, &sub.CreatedAt)
	if err != nil {
		return Subscriber{}, fmt.Errorf("fetching newsletter subscriber %s: %w", id, err)
	}

	return sub, nil
}

// DeleteSubscriber removes a newsletter subscriber.
func (s *Service) DeleteSubscriber(ctx context.Context, id pgtype.UUID) error {
	tag, err := s.db.Writer.Exec(ctx, `
		DELETE FROM newsletter_subscribers
		WHERE id = $1
	`, id)
	if err != nil {
		return fmt.Errorf("deleting newsletter subscriber %s: %w", id, err)
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("newsletter subscriber %s not found", id)
	}

	return nil
}

// UpdateSubscriberStatus updates a subscriber's status (admin PATCH endpoint).
func (s *Service) UpdateSubscriberStatus(ctx context.Context, id pgtype.UUID, status string) (gen.NewsletterSubscriber, error) {
	return s.writeQueries.UpdateSubscriberStatus(ctx, gen.UpdateSubscriberStatusParams{
		ID:     id,
		Status: status,
	})
}

// CreateSubscriber adds a new newsletter subscriber (or re-subscribes a
// previously unsubscribed email). The source parameter tracks where the
// subscription originated ('signup', 'onboarding', 'footer', 'import', 'api').
func (s *Service) CreateSubscriber(ctx context.Context, email string, userID *pgtype.UUID, source string) (gen.NewsletterSubscriber, error) {
	// Canonical form: the column is case-sensitive text, so 'Alice@X.com'
	// and 'alice@x.com' would be two rows (double-mail) without a single
	// normalization point. All write paths funnel through here or
	// CreateSubscriberDirect; read paths use lower(email) = lower(@email)
	// so legacy mixed-case rows still match.
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return gen.NewsletterSubscriber{}, errors.New("newsletter: email is required")
	}

	if source == "" {
		source = SourceSignup
	}

	var userIDArg pgtype.UUID
	if userID != nil {
		userIDArg = *userID
	}

	if s.DoubleOptIn(ctx) {
		// Double opt-in (migration 001170): the row lands in 'pending' until
		// the emailed confirm link is used. Pending rows are invisible to
		// every recipient query (all filter status='subscribed'). A row that
		// comes back 'subscribed' here was already active — the upsert kept
		// its status, so no confirm email is needed.
		subscriber, err := s.writeQueries.SubscribePending(ctx, gen.SubscribePendingParams{
			Email:  email,
			UserID: userIDArg,
			Source: source,
		})
		if err != nil {
			return gen.NewsletterSubscriber{}, fmt.Errorf("creating pending newsletter subscriber: %w", err)
		}

		return subscriber, nil
	}

	subscriber, err := s.writeQueries.Subscribe(ctx, gen.SubscribeParams{
		Email:  email,
		UserID: userIDArg,
		Source: source,
	})
	if err != nil {
		return gen.NewsletterSubscriber{}, fmt.Errorf("creating newsletter subscriber: %w", err)
	}

	return subscriber, nil
}

// CreateSubscriberDirect inserts/updates a subscriber as 'subscribed'
// unconditionally — the admin create/import path. Double opt-in applies to
// public signups only: admin-added rows send no confirmation email, so a
// pending insert there would be stuck with no way to confirm.
func (s *Service) CreateSubscriberDirect(ctx context.Context, email string, userID *pgtype.UUID, source string) (gen.NewsletterSubscriber, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return gen.NewsletterSubscriber{}, errors.New("newsletter: email is required")
	}

	if source == "" {
		source = SourceSignup
	}

	var userIDArg pgtype.UUID
	if userID != nil {
		userIDArg = *userID
	}

	subscriber, err := s.writeQueries.Subscribe(ctx, gen.SubscribeParams{
		Email:  email,
		UserID: userIDArg,
		Source: source,
	})
	if err != nil {
		return gen.NewsletterSubscriber{}, fmt.Errorf("creating newsletter subscriber (admin): %w", err)
	}

	return subscriber, nil
}

// DoubleOptIn reports whether the newsletter.double_opt_in system setting is
// on (§62 Compliance hardening, audit NL-I1). Off by default — the §62
// contract is single opt-in; when enabled, signups land in 'pending' until
// the emailed confirmation link is used.
func (s *Service) DoubleOptIn(ctx context.Context) bool {
	if s.settings == nil {
		return false
	}

	return s.settings.GetBoolOr(ctx, "newsletter.double_opt_in", false)
}

// EnqueueConfirmEmail dispatches the double opt-in confirmation email to a
// pending subscriber via the email:send task (fire-and-forget, Rule 8). The
// link mirrors the unsubscribe flow: a frontend page at /newsletter/confirm
// that calls GET /newsletter/confirm?token=.
func (s *Service) EnqueueConfirmEmail(ctx context.Context, emailAddr, token string) error {
	if s.client == nil {
		return errors.New("asynq client not configured")
	}

	base := s.siteBaseURL(ctx)
	if base == "" {
		return errors.New("site base URL unset — cannot build confirm link")
	}
	confirmURL := strings.TrimRight(base, "/") + "/newsletter/confirm?token=" + token

	payload, err := json.Marshal(map[string]any{
		"to":            emailAddr,
		"subject":       "Confirm your newsletter subscription",
		"body":          fmt.Sprintf(`<p>Please confirm your subscription by clicking the link below:</p><p><a href="%s">Confirm subscription</a></p><p>This link expires in 7 days. If you did not sign up, you can ignore this email — you will not receive any further messages.</p>`, confirmURL),
		"html":          true,
		"template_slug": "newsletter_confirm",
		"template_data": map[string]any{
			"action_url":  confirmURL,
			"site_url":    base,
			"support_url": strings.TrimRight(base, "/") + "/support",
			"expiry":      "7 days",
		},
	})
	if err != nil {
		return fmt.Errorf("marshalling confirm email payload: %w", err)
	}

	task := asynq.NewTask(worker.TypeEmailSend, payload)

	if _, err := s.client.Enqueue(task, asynq.Queue(worker.QueueForTask(worker.TypeEmailSend))); err != nil {
		return fmt.Errorf("enqueuing confirm email: %w", err)
	}

	return nil
}

// ReserveConfirmSend records a confirm-email send for the address under a
// 1-hour cooldown key (jc:newsletter_confirm_sent:{email} — catalog). Returns
// true when the caller may send (first send in the window); false when a
// confirmation was already sent recently. Without it, resubmitting a
// victim's address emails them a fresh confirm link every time — the same
// mail-bomb vector double opt-in exists to close. Fails open when valkey is
// unwired (the row is pending — silent suppression would be worse).
func (s *Service) ReserveConfirmSend(ctx context.Context, email string) bool {
	if s.vk == nil {
		return true
	}

	key := "jc:newsletter_confirm_sent:" + strings.ToLower(strings.TrimSpace(email))
	set, err := s.vk.SetNX(ctx, key, 1, time.Hour).Result()
	if err != nil {
		slog.Warn("newsletter: confirm cooldown check failed (sending anyway)", "error", err)
		return true
	}

	return set
}

// ReleaseConfirmSend drops the cooldown reservation after a failed send —
// otherwise a pending subscriber whose enqueue died would be locked out of
// a working confirmation email for the rest of the window.
func (s *Service) ReleaseConfirmSend(ctx context.Context, email string) {
	if s.vk == nil {
		return
	}

	key := "jc:newsletter_confirm_sent:" + strings.ToLower(strings.TrimSpace(email))
	if err := s.vk.Del(ctx, key).Err(); err != nil {
		slog.Warn("newsletter: confirm cooldown release failed", "error", err)
	}
}

// siteBaseURL resolves the platform site base URL (site.base_url setting,
// then JC_SITE_URL env) — mirrors digest/worker helpers.
func (s *Service) siteBaseURL(ctx context.Context) string {
	if s.settings != nil {
		if v := s.settings.GetStringOr(ctx, "site.base_url", ""); v != "" {
			return v
		}
	}

	return os.Getenv("JC_SITE_URL")
}

// ---------------------------------------------------------------------------
// Campaign Operations
// ---------------------------------------------------------------------------

// GetCampaign returns a single newsletter campaign by ID.
func (s *Service) GetCampaign(ctx context.Context, id pgtype.UUID) (Campaign, error) {
	var c Campaign

	err := s.db.Reader.QueryRow(ctx, `
		SELECT id, name, subject, html, preview_text, body, segment, segment_id,
		       status, scheduled_at, sent_at, recipient_count, open_count, click_count,
		       created_by, created_at, updated_at
		FROM newsletter_campaigns
		WHERE id = $1
	`, id).Scan(&c.ID, &c.Name, &c.Subject, &c.HTML, &c.PreviewText,
		&c.Body, &c.Segment, &c.SegmentID, &c.Status, &c.ScheduledAt, &c.SentAt,
		&c.RecipientCount, &c.OpenCount, &c.ClickCount, &c.CreatedBy, &c.CreatedAt,
		&c.UpdatedAt)
	if err != nil {
		return Campaign{}, fmt.Errorf("fetching newsletter campaign %s: %w", id, err)
	}

	return c, nil
}

// CreateCampaign creates a new newsletter campaign. The createdBy parameter
// is the authenticated admin's user ID (required by the schema). The
// params struct carries the fields: body (jsonb), segment, preview_text,
// and optional scheduled_at (which sets status to 'scheduled' instead of 'draft').
func (s *Service) CreateCampaign(ctx context.Context, createdBy pgtype.UUID, params CreateCampaignInput) (gen.NewsletterCampaign, error) {
	status := CampaignDraft
	if params.ScheduledAt != nil {
		status = CampaignScheduled
	}

	var scheduledAt pgtype.Timestamptz
	if params.ScheduledAt != nil {
		scheduledAt = pgtype.Timestamptz{Time: *params.ScheduledAt, Valid: true}
	}

	var segmentID pgtype.UUID
	if params.SegmentID != "" {
		if err := segmentID.Scan(params.SegmentID); err != nil {
			return gen.NewsletterCampaign{}, fmt.Errorf("newsletter: invalid segment_id: %w", err)
		}
	}

	campaign, err := s.writeQueries.CreateCampaign(ctx, gen.CreateCampaignParams{
		Name:        params.Name,
		Subject:     params.Subject,
		Html:        params.HTML,
		PreviewText: &params.PreviewText,
		Body:        params.Body,
		Segment:     params.Segment,
		SegmentID:   segmentID,
		Status:      status,
		ScheduledAt: scheduledAt,
		CreatedBy:   createdBy,
	})
	if err != nil {
		return gen.NewsletterCampaign{}, fmt.Errorf("creating newsletter campaign: %w", err)
	}

	// §62 scheduling: a scheduled_at puts the campaign on the asynq delayed
	// queue — without this, 'scheduled' was a dead-end status. A failed
	// enqueue is logged, not fatal: the admin Send button still works and
	// the campaign row reflects the intended time.
	if params.ScheduledAt != nil {
		if err := s.EnqueueNewsletterSendAt(ctx, campaign.ID, *params.ScheduledAt); err != nil {
			slog.Error("newsletter: schedule enqueue failed", "campaign_id", campaign.ID.String(), "error", err)
		}
	}

	return campaign, nil
}

// DeleteCampaign removes a newsletter campaign.
func (s *Service) DeleteCampaign(ctx context.Context, id pgtype.UUID) error {
	// Refuse to delete an in-flight send — a 'sending' campaign must be
	// cancelled or reset to draft first. Deleting mid-send would orphan
	// the running task (its lease checks start failing) and drop the
	// send_state audit trail (ON DELETE CASCADE) while mail is still
	// going out.
	tag, err := s.db.Writer.Exec(ctx, `
		DELETE FROM newsletter_campaigns
		WHERE id = $1 AND status != 'sending'
	`, id)
	if err != nil {
		return fmt.Errorf("deleting newsletter campaign %s: %w", id, err)
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("newsletter campaign %s not found or currently sending", id)
	}

	return nil
}

// UpdateCampaignInput carries optional fields for updating a campaign.
type UpdateCampaignInput struct {
	Name        *string
	Subject     *string
	HTML        *string
	PreviewText *string
	Body        json.RawMessage
	Segment     *string
	SegmentID   *string
	Status      *string
	ScheduledAt *time.Time
}

// UpdateCampaign updates an existing newsletter campaign with V2 fields.
func (s *Service) UpdateCampaign(ctx context.Context, id pgtype.UUID, input UpdateCampaignInput) (gen.NewsletterCampaign, error) {
	var segmentID pgtype.UUID
	if input.SegmentID != nil && *input.SegmentID != "" {
		if err := segmentID.Scan(*input.SegmentID); err != nil {
			return gen.NewsletterCampaign{}, fmt.Errorf("newsletter: invalid segment_id: %w", err)
		}
	}

	var scheduledAt pgtype.Timestamptz
	if input.ScheduledAt != nil {
		scheduledAt = pgtype.Timestamptz{Time: *input.ScheduledAt, Valid: true}
	}

	var body []byte
	if input.Body != nil {
		body = input.Body
	}

	campaign, err := s.writeQueries.UpdateCampaign(ctx, gen.UpdateCampaignParams{
		ID:          id,
		Name:        input.Name,
		Subject:     input.Subject,
		Html:        input.HTML,
		PreviewText: input.PreviewText,
		Body:        body,
		Segment:     input.Segment,
		SegmentID:   segmentID,
		Status:      input.Status,
		ScheduledAt: scheduledAt,
	})
	if err != nil {
		return gen.NewsletterCampaign{}, fmt.Errorf("updating newsletter campaign %s: %w", id, err)
	}

	// Re-scheduling a scheduled campaign enqueues the task at the new time
	// (payload carries scheduled_at so Unique() doesn't collapse it).
	if campaign.Status == CampaignScheduled && campaign.ScheduledAt.Valid {
		if err := s.EnqueueNewsletterSendAt(ctx, campaign.ID, campaign.ScheduledAt.Time); err != nil {
			slog.Error("newsletter: schedule enqueue failed", "campaign_id", campaign.ID.String(), "error", err)
		}
	}

	return campaign, nil
}

// ---------------------------------------------------------------------------
// Subscriber preferences, unsubscribe tokens, digest support
// ---------------------------------------------------------------------------

// Unsubscribe unsubscribes an email from the newsletter.
func (s *Service) Unsubscribe(ctx context.Context, email string) error {
	return s.writeQueries.Unsubscribe(ctx,

		email,
	)
}

// UnsubscribeByToken unsubscribes a subscriber via a one-click unsubscribe
// token. The token is single-use: it is deleted after a successful
// unsubscribe. The UnsubscribeByID query is a :one returning the updated
// row, so a platform mismatch (token belongs to a different platform's
// subscriber) yields a "no rows" error rather than a silent no-op (§62).
func (s *Service) UnsubscribeByToken(ctx context.Context, token string) error {
	// Writer-pool read — a token minted seconds ago (send → immediate
	// click) must not 404 under replica lag.
	tokenRow, err := s.writeQueries.GetUnsubscribeToken(ctx, token)
	if err != nil {
		return fmt.Errorf("newsletter: get unsubscribe token: %w", err)
	}

	if _, err := s.writeQueries.UnsubscribeByID(ctx,

		tokenRow.SubscriberID,
	); err != nil {
		return fmt.Errorf("newsletter: unsubscribe by id (platform mismatch or not found): %w", err)
	}

	// Delete the token to enforce single-use semantics. A failure here means
	// the token could be reused — return the error so the caller can retry
	// or alert, rather than silently leaving the token valid.
	if err := s.writeQueries.DeleteUnsubscribeToken(ctx, token); err != nil {
		return fmt.Errorf("newsletter: delete unsubscribe token (single-use): %w", err)
	}

	return nil
}

// GetPreferences returns a subscriber's preferences.
func (s *Service) GetPreferences(ctx context.Context, subscriberID pgtype.UUID) (gen.NewsletterSubscriber, error) {
	return s.queries.GetSubscriberByID(ctx,

		subscriberID,
	)
}

// GetPreferencesByUserID returns a subscriber's preferences by user ID.
func (s *Service) GetPreferencesByUserID(ctx context.Context, userID pgtype.UUID) (gen.NewsletterSubscriber, error) {
	return s.queries.GetSubscriberByUserID(ctx,

		userID,
	)
}

// UpdatePreferences updates a subscriber's digest/campaign toggles.
func (s *Service) UpdatePreferences(ctx context.Context, subscriberID pgtype.UUID, digestEnabled, campaignEnabled *bool) (gen.NewsletterSubscriber, error) {
	return s.writeQueries.UpdatePreferences(ctx, gen.UpdatePreferencesParams{
		ID:              subscriberID,
		DigestEnabled:   digestEnabled,
		CampaignEnabled: campaignEnabled,
	})
}

// GenerateUnsubscribeToken generates a random hex token for one-click unsubscribe.
func GenerateUnsubscribeToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("newsletter: generate token: %w", err)
	}

	return hex.EncodeToString(b), nil
}

// CreateUnsubscribeToken creates a one-click unsubscribe token for a subscriber.
func (s *Service) CreateUnsubscribeToken(ctx context.Context, subscriberID pgtype.UUID) (string, error) {
	token, err := GenerateUnsubscribeToken()
	if err != nil {
		return "", err
	}

	_, err = s.writeQueries.CreateUnsubscribeToken(ctx, gen.CreateUnsubscribeTokenParams{
		Token:        token,
		SubscriberID: subscriberID,
	})
	if err != nil {
		return "", fmt.Errorf("newsletter: create unsubscribe token: %w", err)
	}

	return token, nil
}

// CreateConfirmToken mints a purpose='confirm' token for a pending
// subscriber (double opt-in, migration 001170). Confirm tokens cannot be
// replayed as unsubscribe tokens — GetUnsubscribeToken filters by purpose.
func (s *Service) CreateConfirmToken(ctx context.Context, subscriberID pgtype.UUID) (string, error) {
	token, err := GenerateUnsubscribeToken()
	if err != nil {
		return "", err
	}

	if _, err := s.writeQueries.CreateConfirmToken(ctx, gen.CreateConfirmTokenParams{
		Token:        token,
		SubscriberID: subscriberID,
	}); err != nil {
		return "", fmt.Errorf("newsletter: create confirm token: %w", err)
	}

	return token, nil
}

// ConfirmByToken flips a pending subscriber to 'subscribed' via a
// purpose='confirm' token. Single-use: the token is deleted after a
// successful confirm. pgx.ErrNoRows propagates for the handler's 404
// mapping (unknown token, wrong purpose, or subscriber no longer pending).
func (s *Service) ConfirmByToken(ctx context.Context, token string) (gen.NewsletterSubscriber, error) {
	// Writer-pool read — same read-your-writes reason as the unsubscribe
	// token lookup (a just-emailed confirm link must never 404 on lag).
	tokenRow, err := s.writeQueries.GetConfirmToken(ctx, token)
	if err != nil {
		return gen.NewsletterSubscriber{}, fmt.Errorf("newsletter: get confirm token: %w", err)
	}

	subscriber, err := s.writeQueries.ConfirmSubscriber(ctx, tokenRow.SubscriberID)
	if err != nil {
		return gen.NewsletterSubscriber{}, fmt.Errorf("newsletter: confirm subscriber (not pending or not found): %w", err)
	}

	// The confirm already committed — a failed single-use delete must not
	// surface as an error to the subscriber. The leftover token can't
	// confirm again anyway (the pending check above rejects replays) and
	// the weekly GC sweeps it.
	if err := s.writeQueries.DeleteUnsubscribeToken(ctx, token); err != nil {
		slog.Warn("newsletter: delete confirm token failed (token left for GC)", "error", err)
	}

	return subscriber, nil
}

// CreateCampaignInput is the request body for creating a campaign.
type CreateCampaignInput struct {
	Name        string          `json:"name"`
	Subject     string          `json:"subject"`
	HTML        string          `json:"html"`
	PreviewText string          `json:"preview_text"`
	Body        json.RawMessage `json:"body"`
	Segment     string          `json:"segment"`
	SegmentID   string          `json:"segment_id"`
	ScheduledAt *time.Time      `json:"scheduled_at"`
}

// AdminListSubscribers returns paginated subscribers for admin oversight,
// optionally filtered by status and email search (§62 admin route).
func (s *Service) AdminListSubscribers(ctx context.Context, status, search string, page, limit int) ([]gen.NewsletterSubscriber, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	// #nosec G115 // value bounded by caller
	offset := int32((page - 1) * limit)

	rows, err := s.queries.AdminListSubscribers(ctx, gen.AdminListSubscribersParams{
		Status:     status,
		Search:     search,
		PageLimit:  int32(limit), //nolint:gosec // G115: bounded
		PageOffset: offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("newsletter: admin list subscribers: %w", err)
	}

	total, err := s.queries.AdminCountSubscribers(ctx, gen.AdminCountSubscribersParams{
		Status: status,
		Search: search,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("newsletter: admin count subscribers: %w", err)
	}

	return rows, total, nil
}

// AdminSubscriberStats returns subscriber counts by status.
func (s *Service) AdminSubscriberStats(ctx context.Context) ([]gen.AdminSubscriberStatsRow, error) {
	return s.queries.AdminSubscriberStats(ctx)
}

// AdminListCampaigns returns paginated campaigns for admin oversight.
func (s *Service) AdminListCampaigns(ctx context.Context, status string, page, limit int) ([]gen.NewsletterCampaign, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	// #nosec G115 // value bounded by caller
	offset := int32((page - 1) * limit)

	rows, err := s.queries.AdminListCampaigns(ctx, gen.AdminListCampaignsParams{
		Status:     status,
		PageLimit:  int32(limit), //nolint:gosec // G115: bounded
		PageOffset: offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("newsletter: admin list campaigns: %w", err)
	}

	total, err := s.queries.AdminCountCampaigns(ctx,

		status,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("newsletter: admin count campaigns: %w", err)
	}

	return rows, total, nil
}

// MarkBounced marks a subscriber as bounced (from email provider webhook).
func (s *Service) MarkBounced(ctx context.Context, email string) error {
	return s.writeQueries.MarkBounced(ctx,

		email,
	)
}

// MarkComplained marks a subscriber as complained (from email provider webhook).
func (s *Service) MarkComplained(ctx context.Context, email string) error {
	return s.writeQueries.MarkComplained(ctx,

		email,
	)
}

// IncrementCampaignOpens increments the open_count for a campaign (from
// email provider webhook engagement events). The campaignID is a UUID string
// scanned into pgtype.UUID (§62 schema: open_count).
func (s *Service) IncrementCampaignOpens(ctx context.Context, campaignID string) error {
	var id pgtype.UUID
	if err := id.Scan(campaignID); err != nil {
		return fmt.Errorf("newsletter: invalid campaign_id: %w", err)
	}

	return s.writeQueries.IncrementCampaignOpens(ctx,

		id,
	)
}

// IncrementCampaignClicks increments the click_count for a campaign (from
// email provider webhook engagement events). The campaignID is a UUID string
// scanned into pgtype.UUID (§62 schema: click_count).
func (s *Service) IncrementCampaignClicks(ctx context.Context, campaignID string) error {
	var id pgtype.UUID
	if err := id.Scan(campaignID); err != nil {
		return fmt.Errorf("newsletter: invalid campaign_id: %w", err)
	}

	return s.writeQueries.IncrementCampaignClicks(ctx,

		id,
	)
}

// GetDigestSubscribers returns subscribers with digest enabled (for the digest task).
func (s *Service) GetDigestSubscribers(ctx context.Context, page, limit int) ([]gen.NewsletterSubscriber, int64, error) {
	if limit <= 0 || limit > 500 {
		limit = 500
	}

	if page <= 0 {
		page = 1
	}

	// #nosec G115 // value bounded by caller
	offset := int32((page - 1) * limit)

	rows, err := s.queries.GetDigestSubscribers(ctx, gen.GetDigestSubscribersParams{
		PageLimit:  int32(limit), //nolint:gosec // G115: bounded
		PageOffset: offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("newsletter: get digest subscribers: %w", err)
	}

	total, err := s.queries.CountDigestSubscribers(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("newsletter: count digest subscribers: %w", err)
	}

	return rows, total, nil
}
