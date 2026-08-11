// Package digest — service.go orchestrates digest edition browsing, draft
// editing, publication, and generation triggers. Feature-flagged via
// feature.digest (feature_flags gate) and digest.email system_setting (§84).
//
// Ref: japaconnect-engineering/sections/84-community-digest.md
//      japaconnect-engineering/adr/ADR-092-community-digest.md
package digest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
	valkeycompat "github.com/valkey-io/valkey-go/valkeycompat"
)

// digestDraftKeyTTL is the Valkey TTL for jc:digest_draft keys (§84 spec: 2h
// review window).
const digestDraftKeyTTL = 2 * time.Hour

// TaskEnqueuer is the interface for enqueuing Asynq background tasks
// (structural typing — *worker.Enqueuer satisfies this).
type TaskEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// noopEnqueuer silently drops tasks when no real enqueuer is wired (tests,
// development).
type noopEnqueuer struct{}

func (noopEnqueuer) Enqueue(string, []byte, ...interface{}) error { return nil }

// DigestEditionDTO is the API response shape for a digest edition (listing).
type DigestEditionDTO struct {
	ID          string `json:"id"`
	SpaceID     string `json:"space_id,omitempty"`
	Cadence     string `json:"cadence"`
	Status      string `json:"status"`
	ScheduledFor string `json:"scheduled_for,omitempty"`
	PublishedAt string `json:"published_at,omitempty"`
	EmailSent   bool   `json:"email_sent"`
	GeneratedBy string `json:"generated_by,omitempty"`
	ReviewedBy  string `json:"reviewed_by,omitempty"`
	CreatedAt   string `json:"created_at"`
}

// DigestEditionDetailDTO is the full digest edition with content.
type DigestEditionDetailDTO struct {
	DigestEditionDTO
	Content json.RawMessage `json:"content"`
}

// UpdateEditionInput is the request body for PATCH /admin/digest/:id.
type UpdateEditionInput struct {
	Content json.RawMessage `json:"content"`
}

// Service holds the business logic for the Community Digest.
type Service struct {
	store    *Store
	settings *config.Settings
	enqueuer TaskEnqueuer
	vk       valkeycompat.Cmdable
}

// NewService creates a digest service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{store: store, settings: settings, enqueuer: noopEnqueuer{}}
}

// SetEnqueuer injects the Asynq task enqueuer used to dispatch
// digest:generate and digest:send_email tasks (§84, Rule 7/8).
func (s *Service) SetEnqueuer(e TaskEnqueuer) {
	if e == nil {
		s.enqueuer = noopEnqueuer{}
		return
	}
	s.enqueuer = e
}

// SetValkey injects the Valkey client used for the jc:digest_draft lock
// (§84, valkey-keys-catalog.md, Rule 4).
func (s *Service) SetValkey(vk valkeycompat.Cmdable) { s.vk = vk }

// Enabled reads the digest.enabled system_setting toggle.
func (s *Service) Enabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return false
	}
	return s.settings.GetBoolOr(ctx, tenantID, "digest.enabled", false)
}

// EmailEnabled reads the digest.email system_setting toggle.
func (s *Service) EmailEnabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return false
	}
	return s.settings.GetBoolOr(ctx, tenantID, "digest.email", false)
}

// GetLatest returns the most recently published edition for a tenant
// (and optionally a space).
func (s *Service) GetLatest(ctx context.Context, tenantID, spaceID pgtype.UUID) (DigestEditionDetailDTO, error) {
	edition, err := s.store.GetLatestPublished(ctx, tenantID, spaceID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return DigestEditionDetailDTO{}, pgx.ErrNoRows
		}
		return DigestEditionDetailDTO{}, fmt.Errorf("digest: get latest: %w", err)
	}
	return editionToDetailDTO(edition), nil
}

// Get returns a single published edition by id (public browse).
func (s *Service) Get(ctx context.Context, tenantID, id pgtype.UUID) (DigestEditionDetailDTO, error) {
	edition, err := s.store.GetEdition(ctx, id, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return DigestEditionDetailDTO{}, pgx.ErrNoRows
		}
		return DigestEditionDetailDTO{}, fmt.Errorf("digest: get: %w", err)
	}
	if edition.Status != "published" {
		return DigestEditionDetailDTO{}, pgx.ErrNoRows
	}
	return editionToDetailDTO(edition), nil
}

// AdminListFilter holds admin list filter parameters (§84 admin pattern:
// filter + sort + paginate, no infinite scroll).
type AdminListFilter struct {
	TenantID pgtype.UUID
	SpaceID  pgtype.UUID
	Status   string // draft|review|published|cancelled
	Sort     string // created_at|published_at|scheduled_for (default: created_at)
	Order    string // asc|desc (default: desc)
}

// AdminList returns all digest editions for admin management (paginated,
// filterable by space/status, sortable).
func (s *Service) AdminList(ctx context.Context, f AdminListFilter, page, limit int32) ([]DigestEditionDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	editions, total, err := s.store.ListEditions(ctx, ListEditionsFilter{
		TenantID: f.TenantID,
		SpaceID:  f.SpaceID,
		Status:   f.Status,
		Sort:     f.Sort,
		Order:    f.Order,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("digest: admin list: %w", err)
	}

	dtos := make([]DigestEditionDTO, 0, len(editions))
	for _, e := range editions {
		dtos = append(dtos, editionToDTO(e))
	}
	return dtos, total, nil
}

// AdminGet returns a single edition (any status) for admin management.
func (s *Service) AdminGet(ctx context.Context, tenantID, id pgtype.UUID) (DigestEditionDetailDTO, error) {
	edition, err := s.store.GetEdition(ctx, id, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return DigestEditionDetailDTO{}, pgx.ErrNoRows
		}
		return DigestEditionDetailDTO{}, fmt.Errorf("digest: admin get: %w", err)
	}
	return editionToDetailDTO(edition), nil
}

// AdminUpdate edits the content of a draft edition before publication.
func (s *Service) AdminUpdate(ctx context.Context, tenantID, id pgtype.UUID, content json.RawMessage) error {
	if len(content) == 0 {
		return fmt.Errorf("content is required")
	}
	if err := s.store.UpdateEdition(ctx, id, tenantID, content); err != nil {
		if err == pgx.ErrNoRows {
			return pgx.ErrNoRows
		}
		return fmt.Errorf("digest: admin update: %w", err)
	}
	return nil
}

// AdminPublish publishes a draft or review edition, enqueues the
// digest:send_email task (fire-and-forget, Rule 8), and clears the
// jc:digest_draft Valkey key (§84, ADR-092).
func (s *Service) AdminPublish(ctx context.Context, tenantID, reviewerID, id pgtype.UUID) error {
	if err := s.store.PublishEdition(ctx, id, tenantID, reviewerID); err != nil {
		if err == pgx.ErrNoRows {
			return pgx.ErrNoRows
		}
		return fmt.Errorf("digest: admin publish: %w", err)
	}

	// Clear the draft review lock (§84, valkey-keys-catalog.md).
	s.clearDraftLock(ctx, tenantID, id)

	// Fire-and-forget: enqueue digest:send_email so members with the digest
	// notification preference receive the published edition (§84, Rule 8).
	// Reuses §62 Newsletter infrastructure in the task handler.
	payload, _ := json.Marshal(map[string]string{
		"edition_id": id.String(),
		"tenant_id":  tenantID.String(),
	})
	if err := s.enqueuer.Enqueue("digest:send_email", payload); err != nil {
		// Log but do not fail the publish — email delivery is async and
		// retryable. The edition is already published.
		slog.Error("digest: enqueue send_email failed", "error", err, "edition_id", id.String())
	}
	return nil
}

// AdminGenerate triggers generation of a new digest edition by enqueuing the
// digest:generate Asynq task (§84, ADR-092). The task assembles the edition
// from platform analytics — no AI generation (purely metrics-driven).
// Returns "accepted" to indicate the task was enqueued (202 response).
func (s *Service) AdminGenerate(ctx context.Context, tenantID pgtype.UUID) (string, error) {
	payload, _ := json.Marshal(map[string]string{
		"tenant_id": tenantID.String(),
	})
	if err := s.enqueuer.Enqueue("digest:generate", payload); err != nil {
		return "", fmt.Errorf("digest: enqueue generate: %w", err)
	}
	return "accepted", nil
}

// setDraftLock sets jc:digest_draft:{tenant_id}:{edition_id} with a 2h TTL
// marking the edition as pending admin review (§84, valkey-keys-catalog.md).
func (s *Service) setDraftLock(ctx context.Context, tenantID, editionID pgtype.UUID) {
	if s.vk == nil || !tenantID.Valid || !editionID.Valid {
		return
	}
	key := "jc:digest_draft:" + tenantID.String() + ":" + editionID.String()
	if err := s.vk.Set(ctx, key, "1", digestDraftKeyTTL).Err(); err != nil {
		slog.Error("digest: set draft lock", "error", err, "key", key)
	}
}

// clearDraftLock removes the jc:digest_draft key on publish or cancel.
func (s *Service) clearDraftLock(ctx context.Context, tenantID, editionID pgtype.UUID) {
	if s.vk == nil || !tenantID.Valid || !editionID.Valid {
		return
	}
	key := "jc:digest_draft:" + tenantID.String() + ":" + editionID.String()
	if err := s.vk.Del(ctx, key).Err(); err != nil {
		slog.Error("digest: clear draft lock", "error", err, "key", key)
	}
}

// editionToDTO converts a DigestEdition to a DigestEditionDTO.
func editionToDTO(e DigestEdition) DigestEditionDTO {
	dto := DigestEditionDTO{
		ID:        e.ID.String(),
		Cadence:   e.Cadence,
		Status:    e.Status,
		EmailSent: e.EmailSent,
		CreatedAt: formatTs(e.CreatedAt),
	}
	if e.SpaceID.Valid {
		dto.SpaceID = e.SpaceID.String()
	}
	dto.ScheduledFor = formatTs(e.ScheduledFor)
	dto.PublishedAt = formatTs(e.PublishedAt)
	if e.GeneratedBy != "" {
		dto.GeneratedBy = e.GeneratedBy
	}
	if e.ReviewedBy.Valid {
		dto.ReviewedBy = e.ReviewedBy.String()
	}
	return dto
}

// editionToDetailDTO converts a DigestEdition to a DigestEditionDetailDTO.
func editionToDetailDTO(e DigestEdition) DigestEditionDetailDTO {
	dto := DigestEditionDetailDTO{
		DigestEditionDTO: editionToDTO(e),
	}
	if len(e.Content) > 0 {
		dto.Content = json.RawMessage(e.Content)
	}
	return dto
}

// formatTs renders a pgtype.Timestamptz as RFC 3339 UTC, or "" if NULL.
func formatTs(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339)
}
