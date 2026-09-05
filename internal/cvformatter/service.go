// Package cvformatter — service.go orchestrates CV standard management,
// session creation, and quota enforcement for the International CV /
// Resume Formatter (§95, ADR-103). Feature-flagged via the
// cv_formatter.enabled system_setting. The actual AI formatting is
// dispatched as the cv_format:process Asynq task; this service creates the
// session row, enqueues the task, and returns the session for polling.
//
// Ref: japaconnect-engineering/sections/95-cv-formatter.md
//
//	japaconnect-engineering/adr/ADR-103-cv-formatter.md
package cvformatter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/worker"
)

// permissionChecker is the subset of rbac.RBAC the cvformatter service
// needs to check the cv_formatter.unlimited permission (premium bypass).
// Local interface so cvformatter does not import rbac directly.
type permissionChecker interface {
	HasPermission(ctx context.Context, userID, tenantID pgtype.UUID, permKey string) bool
}

// TaskEnqueuer is the minimal interface for enqueueing Asynq tasks.
// Implemented by *worker.Enqueuer; a nil enqueuer makes task dispatch a
// graceful no-op (development/tests).
type TaskEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// Cache is the minimal Valkey interface for the quota counter.
// INCR atomically increments a key and returns the new value.
// Expire sets a TTL on a key. Implemented by *redis.Cmdable wrappers.
type Cache interface {
	INCR(ctx context.Context, key string) (int64, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error
}

// StandardDTO is the API response shape for a single CV standard.
type StandardDTO struct {
	ID                    string   `json:"id"`
	DestinationSpaceID    string   `json:"destination_space_id"`
	DestinationSpaceName  string   `json:"destination_space_name,omitempty"`
	DestinationSpaceCode  string   `json:"destination_space_code,omitempty"`
	RecommendedLengthPage string   `json:"recommended_length_pages,omitempty"`
	PhotoPolicy           string   `json:"photo_policy,omitempty"`
	DateFormat            string   `json:"date_format,omitempty"`
	RecommendedSections   []string `json:"recommended_sections,omitempty"`
	ToneGuidance          string   `json:"tone_guidance,omitempty"`
	CommonPitfalls        string   `json:"common_pitfalls,omitempty"`
	CreatedAt             string   `json:"created_at"`
	UpdatedAt             string   `json:"updated_at"`
}

// SessionDTO is the API response shape for a single CV format session.
type SessionDTO struct {
	ID                 string `json:"id"`
	TenantID           string `json:"tenant_id"`
	UserID             string `json:"user_id"`
	OriginSpaceID      string `json:"origin_space_id,omitempty"`
	DestinationSpaceID string `json:"destination_space_id"`
	InputText          string `json:"input_text"`
	OutputText         string `json:"output_text,omitempty"`
	OutputFormat       string `json:"output_format"`
	Status             string `json:"status"`
	AIModel            string `json:"ai_model,omitempty"`
	TokensUsed         int32  `json:"tokens_used,omitempty"`
	CreatedAt          string `json:"created_at"`
	ExpiresAt          string `json:"expires_at"`
}

// FormatInput is the request body for POST /cv-formatter/format.
// When a file is uploaded, InputText contains the extracted text and
// the File* fields preserve the original file metadata.
type FormatInput struct {
	DestinationSpaceID string `json:"destination_space_id"`
	OriginSpaceID      string `json:"origin_space_id"`
	InputText          string `json:"input_text"`
	OutputFormat       string `json:"output_format"`
	AIModel            string `json:"ai_model"`
	// File upload metadata (populated by the multipart handler).
	OriginalFilename *string `json:"-"`
	FileStoragePath  *string `json:"-"`
	FileMimeType     *string `json:"-"`
	FileSizeBytes    *int64  `json:"-"`
}

// StandardInput is the request body for POST /admin/cv-formatter/standards.
type StandardInput struct {
	DestinationSpaceID    string   `json:"destination_space_id"`
	RecommendedLengthPage string   `json:"recommended_length_pages"`
	PhotoPolicy           string   `json:"photo_policy"`
	DateFormat            string   `json:"date_format"`
	RecommendedSections   []string `json:"recommended_sections"`
	ToneGuidance          string   `json:"tone_guidance"`
	CommonPitfalls        string   `json:"common_pitfalls"`
}

// ErrQuotaExceeded is returned when a standard member has reached the
// monthly format-request quota.
var ErrQuotaExceeded = errors.New("cv_formatter quota exceeded for this month")

// Service holds the business logic for the CV Formatter.
type Service struct {
	store        *Store
	settings     *config.Settings
	rbac         permissionChecker
	enqueuer     TaskEnqueuer
	cache        Cache
	aiDispatcher AIDispatcher
}

// NewService creates a CV formatter service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{
		store:    store,
		settings: settings,
	}
}

// SetRBAC injects the RBAC resolver used for the cv_formatter.unlimited
// permission check (premium quota bypass). Nil-safe.
func (s *Service) SetRBAC(r permissionChecker) { s.rbac = r }

// SetEnqueuer injects the Asynq task enqueuer for cv_format:process.
// Nil-safe: a nil enqueuer (development/tests) means the session is
// created but the AI task is not dispatched.
func (s *Service) SetEnqueuer(e TaskEnqueuer) { s.enqueuer = e }

// SetCache injects the Valkey cache used for the monthly quota counter
// (jc:cv_quota:{user_id}:{YYYY-MM}). Nil-safe: falls back to DB count.
func (s *Service) SetCache(c Cache) { s.cache = c }

// Enabled reads the cv_formatter.enabled system_setting toggle
// (§95 feature flag).
func (s *Service) Enabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return false
	}

	return s.settings.GetBoolOr(ctx, tenantID, "cv_formatter.enabled", false)
}

// CheckQuota returns nil if the user may submit a new format request,
// or ErrQuotaExceeded if the standard-member monthly quota has been
// reached. Premium users (cv_formatter.unlimited permission) bypass the
// quota. The quota limit is read from the cv_formatter.quota_standard
// system_setting (default 3). The counter is tracked in Valkey via
// jc:cv_quota:{user_id}:{YYYY-MM} with TTL=end-of-month (§95, ADR-103).
// Falls back to DB count when no cache is wired (development/tests).
func (s *Service) CheckQuota(ctx context.Context, userID, tenantID pgtype.UUID) error {
	if s.rbac != nil && s.rbac.HasPermission(ctx, userID, tenantID, "cv_formatter.unlimited") {
		return nil
	}

	limit := 3
	if s.settings != nil {
		limit = s.settings.GetIntOr(ctx, tenantID, "cv_formatter.quota_standard", 3)
	}

	if limit <= 0 {
		return nil
	}

	if s.cache != nil {
		key := cvQuotaKey(userID)

		count, err := s.cache.INCR(ctx, key)
		if err != nil {
			// Best-effort: fall back to DB count on cache error.
			count, err = s.store.CountUserSessionsThisMonth(ctx, userID)
			if err != nil {
				return fmt.Errorf("cvformatter: check quota: %w", err)
			}
		} else if count == 1 {
			// First increment of the month — set TTL to end of month.
			_ = s.cache.Expire(ctx, key, timeUntilEndOfMonth())
		}

		if count > int64(limit) {
			return ErrQuotaExceeded
		}

		return nil
	}
	// Fallback: DB count (no cache wired).
	count, err := s.store.CountUserSessionsThisMonth(ctx, userID)
	if err != nil {
		return fmt.Errorf("cvformatter: check quota: %w", err)
	}

	if count >= int64(limit) {
		return ErrQuotaExceeded
	}

	return nil
}

// cvQuotaKey builds the Valkey quota counter key per §95:
// jc:cv_quota:{user_id}:{YYYY-MM}
func cvQuotaKey(userID pgtype.UUID) string {
	return "jc:cv_quota:" + userID.String() + ":" + time.Now().UTC().Format("2006-01")
}

// timeUntilEndOfMonth returns the duration until the end of the current
// calendar month (UTC), used as the TTL for the quota counter key.
func timeUntilEndOfMonth() time.Duration {
	now := time.Now().UTC()
	nextMonth := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC)

	return nextMonth.Sub(now)
}

// ListStandards returns all CV standards as DTOs.
func (s *Service) ListStandards(ctx context.Context) ([]StandardDTO, error) {
	rows, err := s.store.ListStandards(ctx)
	if err != nil {
		return nil, fmt.Errorf("cvformatter: list standards: %w", err)
	}

	dtos := make([]StandardDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, standardRowToDTO(r))
	}

	return dtos, nil
}

// GetStandardByDest returns the CV standard for the given destination
// space ID.
func (s *Service) GetStandardByDest(ctx context.Context, destSpaceID pgtype.UUID) (StandardDTO, error) {
	row, err := s.store.GetStandardByDest(ctx, destSpaceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return StandardDTO{}, pgx.ErrNoRows
		}

		return StandardDTO{}, fmt.Errorf("cvformatter: get standard by dest: %w", err)
	}

	return standardRowToDTO(row), nil
}

// UpsertStandard creates or updates a CV standard for the given
// destination space. If a standard already exists for the destination,
// it is updated; otherwise a new row is inserted.
func (s *Service) UpsertStandard(ctx context.Context, input StandardInput) (StandardDTO, error) {
	if input.DestinationSpaceID == "" {
		return StandardDTO{}, errors.New("destination_space_id is required")
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
		return StandardDTO{}, fmt.Errorf("cvformatter: invalid destination_space_id: %w", err)
	}

	storeInput := StandardUpsert{
		DestinationSpaceID: destSpaceID,
	}
	if input.RecommendedLengthPage != "" {
		storeInput.RecommendedLengthPages = &input.RecommendedLengthPage
	}

	if input.PhotoPolicy != "" {
		storeInput.PhotoPolicy = &input.PhotoPolicy
	}

	if input.DateFormat != "" {
		storeInput.DateFormat = &input.DateFormat
	}

	if len(input.RecommendedSections) > 0 {
		storeInput.RecommendedSections = input.RecommendedSections
	}

	if input.ToneGuidance != "" {
		storeInput.ToneGuidance = &input.ToneGuidance
	}

	if input.CommonPitfalls != "" {
		storeInput.CommonPitfalls = &input.CommonPitfalls
	}

	// Try create first; fall back to update on conflict.
	row, err := s.store.CreateStandard(ctx, storeInput)
	if err != nil {
		// If the standard already exists for this destination, update it.
		if updateErr := s.store.UpdateStandard(ctx, storeInput); updateErr != nil {
			if errors.Is(updateErr, pgx.ErrNoRows) {
				return StandardDTO{}, fmt.Errorf("cvformatter: upsert standard: create failed (%w) and update found no row", err)
			}

			return StandardDTO{}, fmt.Errorf("cvformatter: upsert standard: update: %w", updateErr)
		}

		row, err = s.store.GetStandardByDest(ctx, destSpaceID)
		if err != nil {
			return StandardDTO{}, fmt.Errorf("cvformatter: upsert standard: refetch: %w", err)
		}
	}

	return standardRowToDTO(row), nil
}

// CreateSession creates a new CV format session with status='pending',
// then enqueues the cv_format:process Asynq task (fire-and-forget, Rule 8).
// The caller must have already verified the feature flag and quota.
func (s *Service) CreateSession(ctx context.Context, tenantID, userID pgtype.UUID, input FormatInput) (SessionDTO, error) {
	if input.DestinationSpaceID == "" {
		return SessionDTO{}, errors.New("destination_space_id is required")
	}

	if input.InputText == "" {
		return SessionDTO{}, errors.New("input_text is required")
	}

	// Validate output format (§95: docx or pdf only).
	outputFormat := input.OutputFormat
	if outputFormat == "" {
		outputFormat = OutputFormatDOCX
	}

	if outputFormat != OutputFormatDOCX && outputFormat != OutputFormatPDF {
		return SessionDTO{}, fmt.Errorf("cvformatter: invalid output_format %q (must be docx or pdf)", outputFormat)
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
		return SessionDTO{}, fmt.Errorf("cvformatter: invalid destination_space_id: %w", err)
	}

	var originSpaceID pgtype.UUID
	if input.OriginSpaceID != "" {
		if err := originSpaceID.Scan(input.OriginSpaceID); err != nil {
			return SessionDTO{}, fmt.Errorf("cvformatter: invalid origin_space_id: %w", err)
		}
	}

	var aiModel *string
	if input.AIModel != "" {
		aiModel = &input.AIModel
	}

	// ADR-088: compute SHA-256 of the input text for provenance tracking.
	hash := sha256.Sum256([]byte(input.InputText))
	promptHash := hex.EncodeToString(hash[:])

	row, err := s.store.CreateSession(ctx, SessionInput{
		TenantID:           tenantID,
		UserID:             userID,
		OriginSpaceID:      originSpaceID,
		DestinationSpaceID: destSpaceID,
		InputText:          input.InputText,
		OutputFormat:       outputFormat,
		AIModel:            aiModel,
		AIPromptHash:       &promptHash,
		OriginalFilename:   input.OriginalFilename,
		FileStoragePath:    input.FileStoragePath,
		FileMimeType:       input.FileMimeType,
		FileSizeBytes:      input.FileSizeBytes,
	})
	if err != nil {
		return SessionDTO{}, fmt.Errorf("cvformatter: create session: %w", err)
	}

	// Enqueue the cv_format:process task (fire-and-forget, Rule 8). A nil
	// enqueuer (development/tests) makes this a graceful no-op.
	if s.enqueuer != nil {
		payload, err := json.Marshal(map[string]string{
			"session_id": row.ID.String(),
		})
		if err != nil {
			slog.Error("cvformatter: marshal process payload", "error", err)
		} else if err := s.enqueuer.Enqueue(worker.TypeCvFormatProcess, payload); err != nil {
			slog.Error("cvformatter: enqueue cv_format:process", "session_id", row.ID.String(), "error", err)
		}
	}

	return sessionRowToDTO(row), nil
}

// GetSession returns a single CV format session by ID. The caller must
// verify ownership (user_id match) before returning the DTO to the
// client.
func (s *Service) GetSession(ctx context.Context, id pgtype.UUID) (SessionRow, error) {
	row, err := s.store.GetSessionByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SessionRow{}, pgx.ErrNoRows
		}

		return SessionRow{}, fmt.Errorf("cvformatter: get session: %w", err)
	}

	return row, nil
}

// ProcessSession implements worker.CvFormatterProcessor. It loads the
// pending session, fetches the destination's cv_standards row, assembles
// a system prompt, calls the AI Gateway, stores the output, and marks
// the session complete. Re-processing a completed/failed session is a
// no-op. Called by the cv_format:process Asynq task handler.
//
// The AI dispatcher is optional — when nil (development/tests), the
// session is marked failed with a descriptive error.
func (s *Service) ProcessSession(ctx context.Context, sessionID pgtype.UUID) error {
	row, err := s.store.GetSessionByID(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("cvformatter: process: load session: %w", err)
	}
	// Idempotent: skip if already complete or failed.
	if row.Status == SessionStatusComplete || row.Status == SessionStatusFailed {
		return nil
	}

	// Mark failed if no AI dispatcher is wired.
	if s.aiDispatcher == nil {
		_ = s.store.UpdateSessionStatus(ctx, sessionID, SessionStatusFailed, nil, nil, nil)
		return errors.New("cvformatter: process: no AI dispatcher configured")
	}

	// Fetch the destination standard for the system prompt.
	standard, err := s.store.GetStandardByDest(ctx, row.DestinationSpaceID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("cvformatter: process: load standard: %w", err)
	}

	// Assemble the system prompt from the standard fields.
	systemPrompt := buildCvSystemPrompt(standard)

	// Call the AI Gateway.
	resp, err := s.aiDispatcher.Dispatch(ctx, "content_generation", AIRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   row.InputText,
	})
	if err != nil {
		_ = s.store.UpdateSessionStatus(ctx, sessionID, SessionStatusFailed, nil, nil, nil)
		return fmt.Errorf("cvformatter: process: AI dispatch: %w", err)
	}

	// Store the output and mark complete.
	outputText := resp.Text

	var tokensUsed *int32

	if resp.Tokens > 0 {
		t := int32(resp.Tokens) // #nosec
		tokensUsed = &t
	}

	var model *string

	if resp.Model != "" {
		m := resp.Model
		model = &m
	}

	if err := s.store.UpdateSessionStatus(ctx, sessionID, SessionStatusComplete, &outputText, tokensUsed, model); err != nil {
		return fmt.Errorf("cvformatter: process: update session: %w", err)
	}

	return nil
}

// PurgeExpiredSessions implements worker.CvFormatterProcessor. It deletes
// cv_format_sessions rows past their expires_at. Called by the
// cv_format_sessions:purge nightly Asynq task (§95, ADR-103).
func (s *Service) PurgeExpiredSessions(ctx context.Context) (int64, error) {
	n, err := s.store.DeleteExpiredSessions(ctx)
	if err != nil {
		return 0, fmt.Errorf("cvformatter: purge expired sessions: %w", err)
	}

	return n, nil
}

// SetAIDispatcher injects the AI Gateway dispatcher used by ProcessSession
// to call the AI for CV formatting. Nil-safe: ProcessSession marks the
// session failed when no dispatcher is wired.
func (s *Service) SetAIDispatcher(d AIDispatcher) { s.aiDispatcher = d }

// AIDispatcher is the subset of *ai.Dispatcher the cvformatter service
// calls to format a CV. Local interface to avoid importing internal/ai
// (which would create a cycle).
type AIDispatcher interface {
	Dispatch(ctx context.Context, category string, req AIRequest) (AIResponse, error)
}

// AIRequest is the provider-agnostic request envelope passed to the AI
// dispatcher. The adapter in internal/server converts this to ai.Request.
type AIRequest struct {
	SystemPrompt string
	UserPrompt   string
}

// AIResponse is the provider-agnostic response envelope returned by the
// AI dispatcher.
type AIResponse struct {
	Text   string
	Tokens int
	Model  string
}

// buildCvSystemPrompt assembles the system prompt for the AI from the
// destination's cv_standards row. The prompt instructs the AI to
// reformat the user's CV according to the destination's conventions.
func buildCvSystemPrompt(r StandardRow) string {
	prompt := "You are a professional CV/resume formatter. Reformat the user's CV text to conform to the conventions of the destination country's job market."
	if r.DestinationSpaceName != nil {
		prompt += " Destination: " + *r.DestinationSpaceName + "."
	}

	if r.RecommendedLengthPages != nil {
		prompt += " Recommended length: " + *r.RecommendedLengthPages + " pages."
	}

	if r.PhotoPolicy != nil {
		prompt += " Photo policy: " + *r.PhotoPolicy + "."
	}

	if r.DateFormat != nil {
		prompt += " Date format: " + *r.DateFormat + "."
	}

	if len(r.RecommendedSections) > 0 {
		prompt += " Recommended sections: " + joinStrings(r.RecommendedSections, ", ") + "."
	}

	if r.ToneGuidance != nil {
		prompt += " Tone: " + *r.ToneGuidance + "."
	}

	if r.CommonPitfalls != nil {
		prompt += " Common pitfalls to avoid: " + *r.CommonPitfalls + "."
	}

	prompt += " Preserve all factual information from the user's CV. Do not invent experience, skills, or dates. Output the reformatted CV text only."

	return prompt
}

// joinStrings joins a slice of strings with the given separator.
func joinStrings(ss []string, sep string) string {
	if len(ss) == 0 {
		return ""
	}

	result := ss[0]
	for _, s := range ss[1:] {
		result += sep + s
	}

	return result
}

// standardRowToDTO converts a StandardRow to a StandardDTO.
func standardRowToDTO(r StandardRow) StandardDTO {
	dto := StandardDTO{
		ID:                  r.ID.String(),
		DestinationSpaceID:  r.DestinationSpaceID.String(),
		RecommendedSections: r.RecommendedSections,
		CreatedAt:           formatTs(r.CreatedAt),
		UpdatedAt:           formatTs(r.UpdatedAt),
	}
	if r.DestinationSpaceName != nil {
		dto.DestinationSpaceName = *r.DestinationSpaceName
	}

	if r.DestinationSpaceCode != nil {
		dto.DestinationSpaceCode = *r.DestinationSpaceCode
	}

	if r.RecommendedLengthPages != nil {
		dto.RecommendedLengthPage = *r.RecommendedLengthPages
	}

	if r.PhotoPolicy != nil {
		dto.PhotoPolicy = *r.PhotoPolicy
	}

	if r.DateFormat != nil {
		dto.DateFormat = *r.DateFormat
	}

	if r.ToneGuidance != nil {
		dto.ToneGuidance = *r.ToneGuidance
	}

	if r.CommonPitfalls != nil {
		dto.CommonPitfalls = *r.CommonPitfalls
	}

	return dto
}

// sessionRowToDTO converts a SessionRow to a SessionDTO.
func sessionRowToDTO(r SessionRow) SessionDTO {
	dto := SessionDTO{
		ID:                 r.ID.String(),
		TenantID:           r.TenantID.String(),
		UserID:             r.UserID.String(),
		DestinationSpaceID: r.DestinationSpaceID.String(),
		InputText:          r.InputText,
		OutputFormat:       r.OutputFormat,
		Status:             r.Status,
		CreatedAt:          formatTs(r.CreatedAt),
		ExpiresAt:          formatTs(r.ExpiresAt),
	}
	if r.OriginSpaceID.Valid {
		dto.OriginSpaceID = r.OriginSpaceID.String()
	}

	if r.OutputText != nil {
		dto.OutputText = *r.OutputText
	}

	if r.AIModel != nil {
		dto.AIModel = *r.AIModel
	}

	if r.TokensUsed != nil {
		dto.TokensUsed = *r.TokensUsed
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
