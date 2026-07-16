// Package newsletter — service.go provides business logic for newsletter management.
package newsletter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/worker"
)

// Subscriber represents a newsletter subscriber.
type Subscriber struct {
	ID        pgtype.UUID        `json:"id"`
	TenantID  pgtype.UUID        `json:"tenant_id"`
	Email     string             `json:"email"`
	UserID    *pgtype.UUID       `json:"user_id,omitempty"`
	Status    string             `json:"status"`
	CreatedAt pgtype.Timestamptz `json:"created_at"`
}

// Campaign represents a newsletter campaign.
type Campaign struct {
	ID        pgtype.UUID         `json:"id"`
	TenantID  pgtype.UUID         `json:"tenant_id"`
	Name      string              `json:"name"`
	Subject   string              `json:"subject"`
	HTML      string              `json:"html"`
	Status    string              `json:"status"`
	SentAt    *pgtype.Timestamptz `json:"sent_at,omitempty"`
	CreatedAt pgtype.Timestamptz  `json:"created_at"`
	UpdatedAt pgtype.Timestamptz  `json:"updated_at"`
}

// Service coordinates newsletter subscriber and campaign management.
type Service struct {
	db     *db.DB
	client *asynq.Client
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{db: database}
}

// SetAsynqClient injects the Asynq client for enqueuing newsletter send tasks.
func (s *Service) SetAsynqClient(client *asynq.Client) {
	s.client = client
}

// GetAllSubscriberEmails returns all active subscriber emails for a tenant.
func (s *Service) GetAllSubscriberEmails(ctx context.Context, tenantID pgtype.UUID) ([]string, error) {
	rows, err := s.db.Reader.Query(ctx, `
		SELECT email FROM newsletter_subscribers
		WHERE tenant_id = $1 AND status = 'active'
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

// EnqueueNewsletterSend enqueues a newsletter_send Asynq task.
func (s *Service) EnqueueNewsletterSend(_ context.Context, campaignID, tenantID pgtype.UUID, subject, htmlBody string, recipients []string) error {
	if s.client == nil {
		return errors.New("asynq client not configured")
	}

	payload, err := json.Marshal(map[string]any{
		"newsletter_id": campaignID.String(),
		"tenant_id":     tenantID.String(),
		"subject":       subject,
		"html_body":     htmlBody,
		"recipients":    recipients,
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
		"task_id", info.ID,
		"recipients", len(recipients))

	return nil
}

// ---------------------------------------------------------------------------
// Subscriber Operations
// ---------------------------------------------------------------------------

// ListSubscribers returns all newsletter subscribers for the tenant.
func (s *Service) ListSubscribers(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]Subscriber, error) {
	rows, err := s.db.Reader.Query(ctx, `
		SELECT id, tenant_id, email, user_id, status, created_at
		FROM newsletter_subscribers
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing newsletter subscribers: %w", err)
	}
	defer rows.Close()

	var subscribers []Subscriber

	for rows.Next() {
		var sub Subscriber
		if err := rows.Scan(&sub.ID, &sub.TenantID, &sub.Email, &sub.UserID, &sub.Status, &sub.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning newsletter subscriber: %w", err)
		}

		subscribers = append(subscribers, sub)
	}

	return subscribers, rows.Err()
}

// GetSubscriber returns a single newsletter subscriber by ID.
func (s *Service) GetSubscriber(ctx context.Context, tenantID, id pgtype.UUID) (Subscriber, error) {
	var sub Subscriber

	err := s.db.Reader.QueryRow(ctx, `
		SELECT id, tenant_id, email, user_id, status, created_at
		FROM newsletter_subscribers
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id).Scan(&sub.ID, &sub.TenantID, &sub.Email, &sub.UserID, &sub.Status, &sub.CreatedAt)
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

// CreateSubscriber adds a new newsletter subscriber.
func (s *Service) CreateSubscriber(ctx context.Context, tenantID pgtype.UUID, email string) (Subscriber, error) {
	var sub Subscriber

	err := s.db.Writer.QueryRow(ctx, `
		INSERT INTO newsletter_subscribers (tenant_id, email, status)
		VALUES ($1, $2, 'active')
		RETURNING id, tenant_id, email, user_id, status, created_at
	`, tenantID, email).Scan(&sub.ID, &sub.TenantID, &sub.Email, &sub.UserID, &sub.Status, &sub.CreatedAt)
	if err != nil {
		return Subscriber{}, fmt.Errorf("creating newsletter subscriber: %w", err)
	}

	return sub, nil
}

// ---------------------------------------------------------------------------
// Campaign Operations
// ---------------------------------------------------------------------------

// ListCampaigns returns all newsletter campaigns for the tenant.
func (s *Service) ListCampaigns(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]Campaign, error) {
	rows, err := s.db.Reader.Query(ctx, `
		SELECT id, tenant_id, name, subject, html, status, sent_at, created_at, updated_at
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
		if err := rows.Scan(&c.ID, &c.TenantID, &c.Name, &c.Subject, &c.HTML, &c.Status, &c.SentAt, &c.CreatedAt, &c.UpdatedAt); err != nil {
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
		SELECT id, tenant_id, name, subject, html, status, sent_at, created_at, updated_at
		FROM newsletter_campaigns
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id).Scan(&c.ID, &c.TenantID, &c.Name, &c.Subject, &c.HTML, &c.Status, &c.SentAt, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return Campaign{}, fmt.Errorf("fetching newsletter campaign %s: %w", id, err)
	}

	return c, nil
}

// CreateCampaign creates a new newsletter campaign.
func (s *Service) CreateCampaign(ctx context.Context, tenantID pgtype.UUID, name, subject, html string) (Campaign, error) {
	var c Campaign

	err := s.db.Writer.QueryRow(ctx, `
		INSERT INTO newsletter_campaigns (tenant_id, name, subject, html, status)
		VALUES ($1, $2, $3, $4, 'draft')
		RETURNING id, tenant_id, name, subject, html, status, sent_at, created_at, updated_at
	`, tenantID, name, subject, html).Scan(&c.ID, &c.TenantID, &c.Name, &c.Subject, &c.HTML, &c.Status, &c.SentAt, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return Campaign{}, fmt.Errorf("creating newsletter campaign: %w", err)
	}

	return c, nil
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

// UpdateCampaign updates an existing newsletter campaign.
func (s *Service) UpdateCampaign(ctx context.Context, tenantID, id pgtype.UUID, name, subject, html, status *string) (Campaign, error) {
	// Fetch current values for merge.
	current, err := s.GetCampaign(ctx, tenantID, id)
	if err != nil {
		return Campaign{}, err
	}

	n := current.Name
	if name != nil {
		n = *name
	}

	su := current.Subject
	if subject != nil {
		su = *subject
	}

	h := current.HTML
	if html != nil {
		h = *html
	}

	st := current.Status
	if status != nil {
		st = *status
	}

	var updated Campaign

	err = s.db.Writer.QueryRow(ctx, `
		UPDATE newsletter_campaigns
		SET name = $3, subject = $4, html = $5, status = $6, updated_at = now()
		WHERE tenant_id = $1 AND id = $2
		RETURNING id, tenant_id, name, subject, html, status, sent_at, created_at, updated_at
	`, tenantID, id, n, su, h, st).Scan(&updated.ID, &updated.TenantID, &updated.Name, &updated.Subject, &updated.HTML, &updated.Status, &updated.SentAt, &updated.CreatedAt, &updated.UpdatedAt)
	if err != nil {
		return Campaign{}, fmt.Errorf("updating newsletter campaign %s: %w", id, err)
	}

	return updated, nil
}
