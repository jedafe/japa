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
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/worker"
)

// Subscriber status constants.
const (
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
	ID              pgtype.UUID `json:"id"`
	TenantID        pgtype.UUID
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
	ID             pgtype.UUID `json:"id"`
	TenantID       pgtype.UUID
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

// Service coordinates newsletter subscriber and campaign management,
// unsubscribe tokens, subscriber preferences, and weekly digest support.
type Service struct {
	db       *db.DB
	queries  *gen.Queries
	client   *asynq.Client
	settings *config.Settings
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{
		db:      database,
		queries: gen.New(database.Reader),
	}
}

// SetAsynqClient injects the Asynq client for enqueuing newsletter send tasks.
func (s *Service) SetAsynqClient(client *asynq.Client) {
	s.client = client
}

// SetSettings injects the config.Settings provider.
func (s *Service) SetSettings(settings *config.Settings) { s.settings = settings }

// Enabled reads the newsletter.enabled system_settings toggle (ADR-080, §62).
// Defaults to true when unset or when settings are not injected.
func (s *Service) Enabled(ctx context.Context) bool {
	if s.settings == nil {
		return true
	}

	return s.settings.GetBoolOr(ctx, "newsletter.enabled", true)
}

// WebhookSecret returns the shared secret used to authenticate provider
// bounce/complaint webhooks (§62 Compliance, ADR-080 §4). Read from the
// JC_NEWSLETTER_WEBHOOK_SECRET environment variable. An empty string means
// webhooks are rejected (fail-closed).
func (s *Service) WebhookSecret(_ context.Context) string {
	return os.Getenv("JC_NEWSLETTER_WEBHOOK_SECRET")
}

// EnqueueNewsletterSend enqueues a newsletter_send Asynq task. The worker
// re-fetches the campaign and resolves segment-aware recipients from the
// DB, so only the IDs are carried in the payload (§62, ADR-080 §3).
func (s *Service) EnqueueNewsletterSend(_ context.Context, campaignID pgtype.UUID) error {
	if s.client == nil {
		return errors.New("asynq client not configured")
	}

	payload, err := json.Marshal(map[string]any{
		"newsletter_id": campaignID.String(),
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

// ---------------------------------------------------------------------------
// Subscriber Operations
// ---------------------------------------------------------------------------

// ListSubscribersFiltered returns newsletter subscribers for the tenant,
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
	return s.queries.UpdateSubscriberStatus(ctx, gen.UpdateSubscriberStatusParams{

		ID:     id,
		Status: status,
	})
}

// CreateSubscriber adds a new newsletter subscriber (or re-subscribes a
// previously unsubscribed email). The source parameter tracks where the
// subscription originated ('signup', 'onboarding', 'footer', 'import', 'api').
func (s *Service) CreateSubscriber(ctx context.Context, email string, userID *pgtype.UUID, source string) (gen.NewsletterSubscriber, error) {
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

	subscriber, err := s.queries.Subscribe(ctx, gen.SubscribeParams{

		Email:  email,
		UserID: userIDArg,
		Source: source,
	})
	if err != nil {
		return gen.NewsletterSubscriber{}, fmt.Errorf("creating newsletter subscriber: %w", err)
	}

	return subscriber, nil
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

	campaign, err := s.queries.CreateCampaign(ctx, gen.CreateCampaignParams{

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

	return campaign, nil
}

// DeleteCampaign removes a newsletter campaign.
func (s *Service) DeleteCampaign(ctx context.Context, id pgtype.UUID) error {
	tag, err := s.db.Writer.Exec(ctx, `
		DELETE FROM newsletter_campaigns
		WHERE id = $1
	`, id)
	if err != nil {
		return fmt.Errorf("deleting newsletter campaign %s: %w", id, err)
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("newsletter campaign %s not found", id)
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

	campaign, err := s.queries.UpdateCampaign(ctx, gen.UpdateCampaignParams{

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

	return campaign, nil
}

// ---------------------------------------------------------------------------
// Subscriber preferences, unsubscribe tokens, digest support
// ---------------------------------------------------------------------------

// Unsubscribe unsubscribes an email from the newsletter.
func (s *Service) Unsubscribe(ctx context.Context, email string) error {
	return s.queries.Unsubscribe(ctx,

		email,
	)
}

// UnsubscribeByToken unsubscribes a subscriber via a one-click unsubscribe
// token. The token is single-use: it is deleted after a successful
// unsubscribe. The UnsubscribeByID query is a :one returning the updated
// row, so a tenant mismatch (token belongs to a different tenant's
// subscriber) yields a "no rows" error rather than a silent no-op (§62).
func (s *Service) UnsubscribeByToken(ctx context.Context, token string) error {
	tokenRow, err := s.queries.GetUnsubscribeToken(ctx, token)
	if err != nil {
		return fmt.Errorf("newsletter: get unsubscribe token: %w", err)
	}

	if _, err := s.queries.UnsubscribeByID(ctx,

		tokenRow.SubscriberID,
	); err != nil {
		return fmt.Errorf("newsletter: unsubscribe by id (tenant mismatch or not found): %w", err)
	}

	// Delete the token to enforce single-use semantics. A failure here means
	// the token could be reused — return the error so the caller can retry
	// or alert, rather than silently leaving the token valid.
	if err := s.queries.DeleteUnsubscribeToken(ctx, token); err != nil {
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
	return s.queries.UpdatePreferences(ctx, gen.UpdatePreferencesParams{

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

	_, err = s.queries.CreateUnsubscribeToken(ctx, gen.CreateUnsubscribeTokenParams{
		Token:        token,
		SubscriberID: subscriberID,
	})
	if err != nil {
		return "", fmt.Errorf("newsletter: create unsubscribe token: %w", err)
	}

	return token, nil
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
	return s.queries.MarkBounced(ctx,

		email,
	)
}

// MarkComplained marks a subscriber as complained (from email provider webhook).
func (s *Service) MarkComplained(ctx context.Context, email string) error {
	return s.queries.MarkComplained(ctx,

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

	return s.queries.IncrementCampaignOpens(ctx,

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

	return s.queries.IncrementCampaignClicks(ctx,

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
