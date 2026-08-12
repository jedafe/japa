// Package newsletter — service.go provides business logic for the Newsletter
// & Digest System (Section 62, ADR-080). Covers subscriber management,
// campaign CRUD, one-click unsubscribe tokens, subscriber preferences,
// and weekly digest support.
//
// Ref: japaconnect-engineering/sections/62-newsletter-digest.md
//      japaconnect-engineering/adr/ADR-080-newsletter-digest.md
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
	ID              pgtype.UUID        `json:"id"`
	TenantID        pgtype.UUID        `json:"tenant_id"`
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
	ID             pgtype.UUID         `json:"id"`
	TenantID       pgtype.UUID         `json:"tenant_id"`
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
func (s *Service) Enabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return true
	}
	return s.settings.GetBoolOr(ctx, tenantID, "newsletter.enabled", true)
}

// WebhookSecret returns the shared secret used to authenticate provider
// bounce/complaint webhooks (§62 Compliance, ADR-080 §4). Read from the
// JC_NEWSLETTER_WEBHOOK_SECRET environment variable. An empty string means
// webhooks are rejected (fail-closed).
func (s *Service) WebhookSecret(_ context.Context) string {
	return os.Getenv("JC_NEWSLETTER_WEBHOOK_SECRET")
}

// GetAllSubscriberEmails returns all subscriber emails opted into campaigns
// for a tenant (status = 'subscribed' AND campaign_enabled = true).
func (s *Service) GetAllSubscriberEmails(ctx context.Context, tenantID pgtype.UUID) ([]string, error) {
	rows, err := s.db.Reader.Query(ctx, `
		SELECT email FROM newsletter_subscribers
		WHERE tenant_id = $1 AND status = 'subscribed' AND campaign_enabled = true
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing subscriber emails: %w", err)
	}
	defer rows.Close()

	var emails []string

	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, fmt.Errorf("scanning subscriber email: %w", err)
		}

		emails = append(emails, email)
	}

	return emails, rows.Err()
}

// EnqueueNewsletterSend enqueues a newsletter_send Asynq task. The worker
// re-fetches the campaign and resolves segment-aware recipients from the
// DB, so only the IDs are carried in the payload (§62, ADR-080 §3).
func (s *Service) EnqueueNewsletterSend(_ context.Context, campaignID, tenantID pgtype.UUID) error {
	if s.client == nil {
		return errors.New("asynq client not configured")
	}

	payload, err := json.Marshal(map[string]any{
		"newsletter_id": campaignID.String(),
		"tenant_id":     tenantID.String(),
	})
	if err != nil {
		return fmt.Errorf("marshalling newsletter payload: %w", err)
	}

	task := asynq.NewTask(worker.TypeNewsletterSend, payload)

	info, err := s.client.Enqueue(task, asynq.Queue(worker.QueueForTask(worker.TypeNewsletterSend)), asynq.Unique(24*time.Hour))
	if err != nil {
		return fmt.Errorf("enqueuing newsletter send: %w", err)
	}

	slog.Info("newsletter: send task enqueued",
		"campaign_id", campaignID.String(),
		"task_id", info.ID)

	return nil
}

// ---------------------------------------------------------------------------
// Subscriber Operations
// ---------------------------------------------------------------------------

// ListSubscribers returns all newsletter subscribers for the tenant.
func (s *Service) ListSubscribers(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]Subscriber, error) {
	rows, err := s.db.Reader.Query(ctx, `
		SELECT id, tenant_id, email, user_id, status, digest_enabled, campaign_enabled,
		       subscribed_at, unsubscribed_at, source, created_at
		FROM newsletter_subscribers
		WHERE tenant_id = $1
		ORDER BY subscribed_at DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing newsletter subscribers: %w", err)
	}
	defer rows.Close()

	var out []Subscriber
	for rows.Next() {
		var s Subscriber
		if err := rows.Scan(
			&s.ID, &s.TenantID, &s.Email, &s.UserID, &s.Status,
			&s.DigestEnabled, &s.CampaignEnabled,
			&s.SubscribedAt, &s.UnsubscribedAt, &s.Source, &s.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning newsletter subscriber: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ListSubscribersFiltered returns newsletter subscribers for the tenant,
// optionally filtered by status (§62 admin route ?status=). Uses the
// AdminListSubscribers sqlc query which supports status filtering.
func (s *Service) ListSubscribersFiltered(ctx context.Context, tenantID pgtype.UUID, status string, limit, offset int32) ([]gen.NewsletterSubscriber, error) {
	return s.queries.AdminListSubscribers(ctx, gen.AdminListSubscribersParams{
		TenantID:   tenantID,
		Status:      status,
		PageLimit:   limit,
		PageOffset:  offset,
	})
}

// GetSubscriber returns a single newsletter subscriber by ID.
func (s *Service) GetSubscriber(ctx context.Context, tenantID, id pgtype.UUID) (Subscriber, error) {
	var sub Subscriber

	err := s.db.Reader.QueryRow(ctx, `
		SELECT id, tenant_id, email, user_id, status, digest_enabled, campaign_enabled,
		       subscribed_at, unsubscribed_at, source, created_at
		FROM newsletter_subscribers
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id).Scan(&sub.ID, &sub.TenantID, &sub.Email, &sub.UserID, &sub.Status,
		&sub.DigestEnabled, &sub.CampaignEnabled, &sub.SubscribedAt, &sub.UnsubscribedAt,
		&sub.Source, &sub.CreatedAt)
	if err != nil {
		return Subscriber{}, fmt.Errorf("fetching newsletter subscriber %s: %w", id, err)
	}

	return sub, nil
}

// DeleteSubscriber removes a newsletter subscriber.
func (s *Service) DeleteSubscriber(ctx context.Context, tenantID, id pgtype.UUID) error {
	tag, err := s.db.Writer.Exec(ctx, `
		DELETE FROM newsletter_subscribers
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id)
	if err != nil {
		return fmt.Errorf("deleting newsletter subscriber %s: %w", id, err)
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("newsletter subscriber %s not found", id)
	}

	return nil
}

// UpdateSubscriberStatus updates a subscriber's status (admin PATCH endpoint).
func (s *Service) UpdateSubscriberStatus(ctx context.Context, tenantID, id pgtype.UUID, status string) (gen.NewsletterSubscriber, error) {
	return s.queries.UpdateSubscriberStatus(ctx, gen.UpdateSubscriberStatusParams{
		TenantID: tenantID,
		ID:       id,
		Status:   status,
	})
}

// CreateSubscriber adds a new newsletter subscriber (or re-subscribes a
// previously unsubscribed email). The source parameter tracks where the
// subscription originated ('signup', 'onboarding', 'footer', 'import', 'api').
func (s *Service) CreateSubscriber(ctx context.Context, tenantID pgtype.UUID, email string, userID *pgtype.UUID, source string) (gen.NewsletterSubscriber, error) {
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
		TenantID: tenantID,
		Email:    email,
		UserID:   userIDArg,
		Source:   source,
	})
	if err != nil {
		return gen.NewsletterSubscriber{}, fmt.Errorf("creating newsletter subscriber: %w", err)
	}
	return subscriber, nil
}

// ---------------------------------------------------------------------------
// Campaign Operations
// ---------------------------------------------------------------------------

// ListCampaigns returns all newsletter campaigns for the tenant.
func (s *Service) ListCampaigns(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]Campaign, error) {
	rows, err := s.db.Reader.Query(ctx, `
		SELECT id, tenant_id, name, subject, html, preview_text, body, segment, segment_id,
		       status, scheduled_at, sent_at, recipient_count, open_count, click_count,
		       created_by, created_at, updated_at
		FROM newsletter_campaigns
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing newsletter campaigns: %w", err)
	}
	defer rows.Close()

	var campaigns []Campaign

	for rows.Next() {
		var c Campaign
		if err := rows.Scan(&c.ID, &c.TenantID, &c.Name, &c.Subject, &c.HTML, &c.PreviewText,
			&c.Body, &c.Segment, &c.SegmentID, &c.Status, &c.ScheduledAt, &c.SentAt,
			&c.RecipientCount, &c.OpenCount, &c.ClickCount, &c.CreatedBy, &c.CreatedAt,
			&c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning newsletter campaign: %w", err)
		}

		campaigns = append(campaigns, c)
	}

	return campaigns, rows.Err()
}

// GetCampaign returns a single newsletter campaign by ID.
func (s *Service) GetCampaign(ctx context.Context, tenantID, id pgtype.UUID) (Campaign, error) {
	var c Campaign

	err := s.db.Reader.QueryRow(ctx, `
		SELECT id, tenant_id, name, subject, html, preview_text, body, segment, segment_id,
		       status, scheduled_at, sent_at, recipient_count, open_count, click_count,
		       created_by, created_at, updated_at
		FROM newsletter_campaigns
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id).Scan(&c.ID, &c.TenantID, &c.Name, &c.Subject, &c.HTML, &c.PreviewText,
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
func (s *Service) CreateCampaign(ctx context.Context, tenantID, createdBy pgtype.UUID, params CreateCampaignInput) (gen.NewsletterCampaign, error) {
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
		TenantID:    tenantID,
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
func (s *Service) DeleteCampaign(ctx context.Context, tenantID, id pgtype.UUID) error {
	tag, err := s.db.Writer.Exec(ctx, `
		DELETE FROM newsletter_campaigns
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id)
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
func (s *Service) UpdateCampaign(ctx context.Context, tenantID, id pgtype.UUID, input UpdateCampaignInput) (gen.NewsletterCampaign, error) {
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
		TenantID:    tenantID,
		ID:          id,
		Name:        input.Name,
		Subject:     input.Subject,
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

	// If html is provided, update it via raw SQL (the V2 sqlc query doesn't
	// include the legacy html column).
	if input.HTML != nil {
		if _, err := s.db.Writer.Exec(ctx, `
			UPDATE newsletter_campaigns SET html = $3, updated_at = now()
			WHERE tenant_id = $1 AND id = $2
		`, tenantID, id, *input.HTML); err != nil {
			return gen.NewsletterCampaign{}, fmt.Errorf("updating newsletter campaign html %s: %w", id, err)
		}
	}

	return campaign, nil
}

// ---------------------------------------------------------------------------
// Subscriber preferences, unsubscribe tokens, digest support
// ---------------------------------------------------------------------------

// Unsubscribe unsubscribes an email from the newsletter.
func (s *Service) Unsubscribe(ctx context.Context, tenantID pgtype.UUID, email string) error {
	return s.queries.Unsubscribe(ctx, gen.UnsubscribeParams{
		TenantID: tenantID,
		Email:    email,
	})
}

// UnsubscribeByToken unsubscribes a subscriber via a one-click unsubscribe token.
func (s *Service) UnsubscribeByToken(ctx context.Context, tenantID pgtype.UUID, token string) error {
	tokenRow, err := s.queries.GetUnsubscribeToken(ctx, token)
	if err != nil {
		return fmt.Errorf("newsletter: get unsubscribe token: %w", err)
	}

	if err := s.queries.UnsubscribeByID(ctx, gen.UnsubscribeByIDParams{
		TenantID: tenantID,
		ID:       tokenRow.SubscriberID,
	}); err != nil {
		return fmt.Errorf("newsletter: unsubscribe by id: %w", err)
	}

	_ = s.queries.DeleteUnsubscribeToken(ctx, token)
	return nil
}

// GetPreferences returns a subscriber's preferences.
func (s *Service) GetPreferences(ctx context.Context, tenantID, subscriberID pgtype.UUID) (gen.NewsletterSubscriber, error) {
	return s.queries.GetSubscriberByID(ctx, gen.GetSubscriberByIDParams{
		TenantID: tenantID,
		ID:       subscriberID,
	})
}

// GetPreferencesByUserID returns a subscriber's preferences by user ID.
func (s *Service) GetPreferencesByUserID(ctx context.Context, tenantID, userID pgtype.UUID) (gen.NewsletterSubscriber, error) {
	return s.queries.GetSubscriberByUserID(ctx, gen.GetSubscriberByUserIDParams{
		TenantID: tenantID,
		UserID:   userID,
	})
}

// UpdatePreferences updates a subscriber's digest/campaign toggles.
func (s *Service) UpdatePreferences(ctx context.Context, tenantID, subscriberID pgtype.UUID, digestEnabled, campaignEnabled *bool) (gen.NewsletterSubscriber, error) {
	return s.queries.UpdatePreferences(ctx, gen.UpdatePreferencesParams{
		TenantID:        tenantID,
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

// SendCampaign sends a campaign immediately via the newsletter:send task.
func (s *Service) SendCampaign(ctx context.Context, tenantID, campaignID pgtype.UUID) error {
	if err := s.queries.MarkCampaignSending(ctx, gen.MarkCampaignSendingParams{
		TenantID: tenantID,
		ID:       campaignID,
	}); err != nil {
		return fmt.Errorf("newsletter: mark campaign sending: %w", err)
	}

	if s.client != nil {
		payload, _ := json.Marshal(map[string]interface{}{
			"tenant_id":   tenantID.String(),
			"campaign_id": campaignID.String(),
		})
		task := asynq.NewTask(worker.TypeNewsletterSend, payload)
		if _, err := s.client.Enqueue(task, asynq.Queue(worker.QueueForTask(worker.TypeNewsletterSend))); err != nil {
			slog.Warn("newsletter: enqueue send failed", "error", err)
		}
	}

	return nil
}

// AdminListSubscribers returns paginated subscribers for admin oversight.
func (s *Service) AdminListSubscribers(ctx context.Context, tenantID pgtype.UUID, status string, page, limit int) ([]gen.NewsletterSubscriber, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := int32((page - 1) * limit)

	rows, err := s.queries.AdminListSubscribers(ctx, gen.AdminListSubscribersParams{
		TenantID:   tenantID,
		Status:     status,
		PageLimit:  int32(limit),
		PageOffset: offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("newsletter: admin list subscribers: %w", err)
	}

	total, err := s.queries.AdminCountSubscribers(ctx, gen.AdminCountSubscribersParams{
		TenantID: tenantID,
		Status:   status,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("newsletter: admin count subscribers: %w", err)
	}

	return rows, total, nil
}

// AdminSubscriberStats returns subscriber counts by status.
func (s *Service) AdminSubscriberStats(ctx context.Context, tenantID pgtype.UUID) ([]gen.AdminSubscriberStatsRow, error) {
	return s.queries.AdminSubscriberStats(ctx, tenantID)
}

// AdminListCampaigns returns paginated campaigns for admin oversight.
func (s *Service) AdminListCampaigns(ctx context.Context, tenantID pgtype.UUID, status string, page, limit int) ([]gen.NewsletterCampaign, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := int32((page - 1) * limit)

	rows, err := s.queries.AdminListCampaigns(ctx, gen.AdminListCampaignsParams{
		TenantID:   tenantID,
		Status:     status,
		PageLimit:  int32(limit),
		PageOffset: offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("newsletter: admin list campaigns: %w", err)
	}

	total, err := s.queries.AdminCountCampaigns(ctx, gen.AdminCountCampaignsParams{
		TenantID: tenantID,
		Status:   status,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("newsletter: admin count campaigns: %w", err)
	}

	return rows, total, nil
}

// MarkBounced marks a subscriber as bounced (from email provider webhook).
func (s *Service) MarkBounced(ctx context.Context, tenantID pgtype.UUID, email string) error {
	return s.queries.MarkBounced(ctx, gen.MarkBouncedParams{
		TenantID: tenantID,
		Email:    email,
	})
}

// MarkComplained marks a subscriber as complained (from email provider webhook).
func (s *Service) MarkComplained(ctx context.Context, tenantID pgtype.UUID, email string) error {
	return s.queries.MarkComplained(ctx, gen.MarkComplainedParams{
		TenantID: tenantID,
		Email:    email,
	})
}

// GetDigestSubscribers returns subscribers with digest enabled (for the digest task).
func (s *Service) GetDigestSubscribers(ctx context.Context, tenantID pgtype.UUID, page, limit int) ([]gen.NewsletterSubscriber, int64, error) {
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	if page <= 0 {
		page = 1
	}
	offset := int32((page - 1) * limit)

	rows, err := s.queries.GetDigestSubscribers(ctx, gen.GetDigestSubscribersParams{
		TenantID:   tenantID,
		PageLimit:  int32(limit),
		PageOffset: offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("newsletter: get digest subscribers: %w", err)
	}

	total, err := s.queries.CountDigestSubscribers(ctx, tenantID)
	if err != nil {
		return nil, 0, fmt.Errorf("newsletter: count digest subscribers: %w", err)
	}

	return rows, total, nil
}
