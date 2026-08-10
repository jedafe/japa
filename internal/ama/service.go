// Package ama — service.go orchestrates AMA thread listing, retrieval,
// creation, and closing. AMA-specific rate limiting (one active session per
// host + configurable cooldown) and the jc:ama_active Valkey lock are
// enforced here. The forum.ama.opened event is emitted fire-and-forget on
// creation (§88 Notifications / §35 reuse).
//
// Ref: japaconnect-engineering/sections/88-ask-an-expat.md
//      japaconnect-engineering/adr/ADR-096-ask-an-expat.md
package ama

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
	valkeycompat "github.com/valkey-io/valkey-go/valkeycompat"
)

// AMAThreadDTO is the API response shape for an AMA thread. Body is []byte
// so encoding/json base64-encodes it (matching the forum thread contract —
// the frontend's tiptapBodyToHtml expects a base64 string).
type AMAThreadDTO struct {
	ID           string          `json:"id"`
	Title        string          `json:"title"`
	Slug         string          `json:"slug"`
	Body         []byte          `json:"body"`
	AuthorUserID string          `json:"author_user_id,omitempty"`
	AuthorName   string          `json:"author_name,omitempty"`
	AuthorAvatar string          `json:"author_avatar,omitempty"`
	AmaConfig    json.RawMessage `json:"ama_config"`
	CreatedAt    string          `json:"created_at"`
	UpdatedAt    string          `json:"updated_at"`
}

// amaConfigShape is the JSONB structure stored in forum_threads.ama_config.
// closes_at is computed by the service from open_duration_hours when absent.
type amaConfigShape struct {
	OpenDurationHours       *int    `json:"open_duration_hours,omitempty"`
	QuestionApprovalRequired *bool  `json:"question_approval_required,omitempty"`
	ClosesAt                *string `json:"closes_at,omitempty"`
	HostProfession          string  `json:"host_profession,omitempty"`
	HostDestCountryID       string  `json:"host_dest_country_id,omitempty"`
	HostYearsAbroad         *int    `json:"host_years_abroad,omitempty"`
}

// CreateAMAInput is the validated payload for creating an AMA session.
type CreateAMAInput struct {
	Title        string
	Slug         string
	Body         []byte
	SpaceID      pgtype.UUID
	IsAnonymous  bool
	HostProfession    string
	HostDestCountryID string
	HostYearsAbroad   *int
	OpenDurationHours *int
	QuestionApprovalRequired *bool
}

// permissionChecker is the subset of rbac.RBAC the AMA service needs to
// resolve host-vs-moderator close authorization. Local interface so ama does
// not import rbac.
type permissionChecker interface {
	HasPermission(ctx context.Context, userID, tenantID pgtype.UUID, permKey string) bool
}

// webhookEmitter fans an ama.* event out to subscribed webhook endpoints
// (fire-and-forget, Rule 8). Mirrors the forum webhookEmitter pattern.
type webhookEmitter interface {
	EmitEvent(ctx context.Context, tenantID pgtype.UUID, eventType string, payload any)
}

// Service holds the business logic for AMA threads.
type Service struct {
	store    *Store
	settings *config.Settings

	// Optional collaborators injected after construction (nil-safe).
	rbac     permissionChecker
	webhooks webhookEmitter
	vk       valkeycompat.Cmdable
}

// NewService creates an AMA service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{store: store, settings: settings}
}

// SetRBAC injects the permission checker used for close authorization
// (host OR forum.ama.moderate).
func (s *Service) SetRBAC(r permissionChecker) { s.rbac = r }

// SetWebhookEmitter injects the forum.ama.opened event fan-out (§88 / §35).
func (s *Service) SetWebhookEmitter(w webhookEmitter) { s.webhooks = w }

// SetValkey injects the client used for the jc:ama_active:{user_id} lock.
func (s *Service) SetValkey(vk valkeycompat.Cmdable) { s.vk = vk }

// CooldownDays returns the AMA cooldown period in days (§88 system_setting
// ama_cooldown_days, default 30).
func (s *Service) CooldownDays(ctx context.Context, tenantID pgtype.UUID) int {
	if s.settings == nil {
		return 30
	}
	return s.settings.GetIntOr(ctx, tenantID, "ama_cooldown_days", 30)
}

// ListActive returns active AMA sessions (not yet closed), filtered by
// destination country and host profession (geo-aware tool pattern) and
// cursor-paginated by created_at.
func (s *Service) ListActive(ctx context.Context, tenantID pgtype.UUID, dest, profession string, pageSize int32, cursorCreatedAt pgtype.Timestamptz) ([]AMAThreadDTO, error) {
	threads, err := s.store.ListActive(ctx, tenantID, dest, profession, pageSize, cursorCreatedAt)
	if err != nil {
		return nil, fmt.Errorf("ama: list active: %w", err)
	}
	dtos := make([]AMAThreadDTO, 0, len(threads))
	for _, t := range threads {
		dtos = append(dtos, threadToDTO(t))
	}
	return dtos, nil
}

// GetByID returns a single AMA thread by ID.
func (s *Service) GetByID(ctx context.Context, id, tenantID pgtype.UUID) (AMAThreadDTO, error) {
	thread, err := s.store.GetByID(ctx, id, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AMAThreadDTO{}, pgx.ErrNoRows
		}
		return AMAThreadDTO{}, fmt.Errorf("ama: get: %w", err)
	}
	return threadToDTO(thread), nil
}

// CreateAMA validates rate limits, persists the AMA thread, sets the
// jc:ama_active Valkey lock, and emits forum.ama.opened fire-and-forget.
// Ref: §88 (creation, rate limiting, notifications).
func (s *Service) CreateAMA(ctx context.Context, tenantID, authorUserID pgtype.UUID, in CreateAMAInput) (AMAThreadDTO, error) {
	if in.Title == "" {
		return AMAThreadDTO{}, errors.New("title is required")
	}
	if in.Slug == "" {
		return AMAThreadDTO{}, errors.New("slug is required")
	}

	// Rate limit 1: one active AMA per host (§88).
	active, err := s.store.HasActiveAMA(ctx, authorUserID, tenantID)
	if err != nil {
		return AMAThreadDTO{}, fmt.Errorf("ama: check active: %w", err)
	}
	if active {
		return AMAThreadDTO{}, ErrActiveAMAExists
	}

	// Rate limit 2: cooldown between sessions (§88, ama_cooldown_days).
	cooldown := s.CooldownDays(ctx, tenantID)
	inCooldown, err := s.store.CheckCooldown(ctx, authorUserID, tenantID, cooldown)
	if err != nil {
		return AMAThreadDTO{}, fmt.Errorf("ama: check cooldown: %w", err)
	}
	if inCooldown {
		return AMAThreadDTO{}, ErrAMACooldown
	}

	// Compute closes_at from open_duration_hours (default 48) when not set.
	cfg := amaConfigShape{
		HostProfession:    in.HostProfession,
		HostDestCountryID: in.HostDestCountryID,
		HostYearsAbroad:   in.HostYearsAbroad,
		OpenDurationHours: in.OpenDurationHours,
		QuestionApprovalRequired: in.QuestionApprovalRequired,
	}
	hours := 48
	if in.OpenDurationHours != nil && *in.OpenDurationHours > 0 {
		hours = *in.OpenDurationHours
	}
	closesAt := time.Now().UTC().Add(time.Duration(hours) * time.Hour)
	closesIso := closesAt.Format(time.RFC3339)
	cfg.ClosesAt = &closesIso

	amaConfigBytes, err := json.Marshal(cfg)
	if err != nil {
		return AMAThreadDTO{}, fmt.Errorf("ama: marshal config: %w", err)
	}

	editWindow := int32(30)
	if s.settings != nil {
		editWindow = int32(s.settings.GetIntOr(ctx, tenantID, "forum.edit_window_minutes", 30))
	}

	body := in.Body
	if body == nil {
		body = []byte("[]")
	}

	thread, err := s.store.CreateAMA(ctx, CreateAMAParams{
		TenantID:          tenantID,
		AuthorUserID:      authorUserID,
		SpaceID:           in.SpaceID,
		Title:             in.Title,
		Slug:              in.Slug,
		Body:              body,
		IsAnonymous:       in.IsAnonymous,
		EditWindowMinutes: editWindow,
		AmaConfig:         amaConfigBytes,
	})
	if err != nil {
		return AMAThreadDTO{}, fmt.Errorf("ama: create: %w", err)
	}

	// Set the jc:ama_active:{user_id} lock with TTL = closes_at (§88 / Rule 4).
	s.setActiveLock(ctx, authorUserID, closesAt)

	// Fire-and-forget forum.ama.opened event for follower notifications (§88 /
	// §35). Rule 8: never block the request on fan-out.
	if s.webhooks != nil {
		s.webhooks.EmitEvent(ctx, tenantID, "forum.ama.opened", map[string]any{
			"thread_id": thread.ID.String(), "host_id": authorUserID.String(),
			"slug": thread.Slug, "closes_at": closesIso,
		})
	}

	return threadToDTO(thread), nil
}

// CanClose reports whether the caller may close the AMA: the thread author
// (host) always may, otherwise the caller must hold forum.ama.moderate
// (§88 "host or moderator").
func (s *Service) CanClose(ctx context.Context, threadID, tenantID, callerUserID pgtype.UUID) (bool, error) {
	authorID, err := s.store.GetAuthorID(ctx, threadID, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, pgx.ErrNoRows
		}
		return false, fmt.Errorf("ama: resolve author: %w", err)
	}
	if authorID.Valid && authorID == callerUserID {
		return true, nil
	}
	if s.rbac != nil && s.rbac.HasPermission(ctx, callerUserID, tenantID, "forum.ama.moderate") {
		return true, nil
	}
	return false, nil
}

// CloseAMA closes an AMA session (host or moderator only, per CanClose).
func (s *Service) CloseAMA(ctx context.Context, threadID, tenantID, callerUserID pgtype.UUID) error {
	allowed, err := s.CanClose(ctx, threadID, tenantID, callerUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}
		return fmt.Errorf("ama: authorize close: %w", err)
	}
	if !allowed {
		return ErrCloseForbidden
	}
	if err := s.store.CloseAMA(ctx, threadID, tenantID); err != nil {
		return fmt.Errorf("ama: close: %w", err)
	}
	// Release the active-AMA lock for the host.
	s.clearActiveLock(ctx, callerUserID)
	return nil
}

// setActiveLock sets jc:ama_active:{user_id} with a TTL matching closes_at.
func (s *Service) setActiveLock(ctx context.Context, userID pgtype.UUID, closesAt time.Time) {
	if s.vk == nil || !userID.Valid {
		return
	}
	ttl := time.Until(closesAt)
	if ttl <= 0 {
		return
	}
	key := "jc:ama_active:" + userID.String()
	if err := s.vk.Set(ctx, key, "1", ttl).Err(); err != nil {
		slog.Error("ama: set active lock", "error", err, "key", key)
	}
}

// clearActiveLock removes the jc:ama_active:{user_id} lock on close.
func (s *Service) clearActiveLock(ctx context.Context, userID pgtype.UUID) {
	if s.vk == nil || !userID.Valid {
		return
	}
	key := "jc:ama_active:" + userID.String()
	if err := s.vk.Del(ctx, key).Err(); err != nil {
		slog.Error("ama: clear active lock", "error", err, "key", key)
	}
}

// threadToDTO converts an AMAThread to an AMAThreadDTO.
func threadToDTO(t AMAThread) AMAThreadDTO {
	dto := AMAThreadDTO{
		ID:           t.ID.String(),
		Title:        t.Title,
		Slug:         t.Slug,
		Body:         t.Body,
		AuthorUserID: t.AuthorUserID.String(),
		AuthorName:   t.AuthorName,
		AuthorAvatar: t.AuthorAvatar,
		AmaConfig:    json.RawMessage(t.AmaConfig),
		CreatedAt:    formatTs(t.CreatedAt),
		UpdatedAt:    formatTs(t.UpdatedAt),
	}
	return dto
}

// Sentinel errors mapped to the standard error envelope by the handlers.
var (
	ErrActiveAMAExists = errors.New("an active AMA session already exists for this host")
	ErrAMACooldown     = errors.New("AMA cooldown period has not elapsed")
	ErrCloseForbidden  = errors.New("only the host or a moderator may close this AMA")
)
