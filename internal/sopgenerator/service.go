// Package sopgenerator — service.go orchestrates SOP generation for §110.
// Generation is async: POST /sop/generate creates a queued draft and
// enqueues the sop:generate Asynq task; the worker calls ProcessDraft
// which dispatches to the §20 AI Gateway and writes the result. Quota is
// enforced via Valkey jc:sop_quota:{user_id}:{YYYY-MM} (cataloged).
// // Ref: japaconnect-engineering/sections/110-sop-generator.md
// //	japaconnect-engineering/adr/ADR-121-sop-generator.md
package sopgenerator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
)

// ErrQuotaExceeded is returned when the monthly generation quota is hit.
var ErrQuotaExceeded = errors.New("sop_generator quota exceeded for this month")

// ErrNotOwner is returned when a draft is accessed by a non-owner.
var ErrNotOwner = errors.New("draft does not belong to this user")

// ErrInvalidPurpose is returned when a purpose outside the §110 enum is
// submitted (study_permit|student_visa|work_visa|visitor_visa).
var ErrInvalidPurpose = errors.New("purpose must be one of study_permit|student_visa|work_visa|visitor_visa")

// ErrInvalidProgramLevel is returned when a program_level outside the
// §110 enum is submitted (diploma|bachelors|masters|mba|phd).
var ErrInvalidProgramLevel = errors.New("program_level must be one of diploma|bachelors|masters|mba|phd")

// ErrInvalidInput is returned for other malformed request payloads.
var ErrInvalidInput = errors.New("invalid request input")

// ErrDuplicateField is returned when a sop_fields name violates the
// case-insensitive uniqueness index.
var ErrDuplicateField = errors.New("a field of study with that name already exists")

func validPurpose(p string) bool {
	switch p {
	case "study_permit", "student_visa", "work_visa", "visitor_visa":
		return true
	}
	return false
}

func validProgramLevel(l string) bool {
	switch l {
	case "diploma", "bachelors", "masters", "mba", "phd":
		return true
	}
	return false
}

// TaskEnqueuer is the minimal interface for enqueueing Asynq tasks.
// Implemented by *worker.Enqueuer; a nil enqueuer makes dispatch a
// graceful no-op (development/tests).
type TaskEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// Cache is the minimal Valkey interface for the quota counter.
type Cache interface {
	INCR(ctx context.Context, key string) (int64, error)
	DECR(ctx context.Context, key string) (int64, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error
}

// AIDispatcher is the subset of *ai.Dispatcher the service calls. Local
// interface to avoid an import cycle; the server package adapts.
type AIDispatcher interface {
	Dispatch(ctx context.Context, category string, req AIRequest) (AIResponse, error)
}

// AIRequest / AIResponse are the provider-agnostic envelopes.
type AIRequest struct {
	SystemPrompt string
	UserPrompt   string
}

// AIResponse carries the generated text and model provenance.
type AIResponse struct {
	Text   string
	Model  string
	Tokens int
}

// TemplateDTO is the API shape for a template.
type TemplateDTO struct {
	ID                 string          `json:"id"`
	DestinationSpaceID string          `json:"destination_space_id,omitempty"`
	Purpose            string          `json:"purpose"`
	Title              string          `json:"title"`
	Structure          json.RawMessage `json:"structure"`
	Status             string          `json:"status,omitempty"`
	IsActive           bool            `json:"is_active"`
	AIGenerated        bool            `json:"ai_generated"`
	CreatedAt          string          `json:"created_at,omitempty"`
	UpdatedAt          string          `json:"updated_at,omitempty"`
}

// SampleDTO is the API shape for a published sample.
type SampleDTO struct {
	ID                 string `json:"id"`
	DestinationName    string `json:"destination_space_name,omitempty"`
	DestinationSpaceID string `json:"destination_space_id,omitempty"`
	ProgramLevel       string `json:"program_level"`
	FieldOfStudy       string `json:"field_of_study,omitempty"`
	Title              string `json:"title"`
	Body               string `json:"body"`
	WordCount          *int32 `json:"word_count,omitempty"`
	Status             string `json:"status,omitempty"`
	AIGenerated        bool   `json:"ai_generated"`
}

// DraftDTO is the API shape for a user draft.
type DraftDTO struct {
	ID                 string          `json:"id"`
	DestinationSpaceID string          `json:"destination_space_id,omitempty"`
	Purpose            string          `json:"purpose"`
	Program            string          `json:"program,omitempty"`
	Institution        string          `json:"institution,omitempty"`
	Title              string          `json:"title,omitempty"`
	Inputs             json.RawMessage `json:"inputs"`
	GeneratedText      string          `json:"generated_text,omitempty"`
	Status             string          `json:"status"`
	AIModel            string          `json:"ai_model,omitempty"`
	Version            int32           `json:"version"`
	CreatedAt          string          `json:"created_at"`
	UpdatedAt          string          `json:"updated_at"`
}

// GenerateRequest is the POST /sop/generate body.
type GenerateRequest struct {
	DestinationSpaceID string          `json:"destination_space_id"`
	Purpose            string          `json:"purpose"` // study_permit|student_visa|work_visa|visitor_visa
	Program            string          `json:"program"`
	Institution        string          `json:"institution"`
	Title              string          `json:"title"`
	Inputs             json.RawMessage `json:"inputs"` // profile answers
}

// UpdateDraftRequest is the PATCH /sop/drafts/:id body.
type UpdateDraftRequest struct {
	Title         *string `json:"title"`
	Program       *string `json:"program"`
	Institution   *string `json:"institution"`
	GeneratedText *string `json:"generated_text"`
}

// TemplateRequest is the admin create/update body for a template.
type TemplateRequest struct {
	DestinationSpaceID string          `json:"destination_space_id"`
	Purpose            string          `json:"purpose"`
	Title              string          `json:"title"`
	Structure          json.RawMessage `json:"structure"`
	IsActive           *bool           `json:"is_active"`
}

// SampleRequest is the admin create/update body for a sample.
type SampleRequest struct {
	DestinationSpaceID string `json:"destination_space_id"`
	ProgramLevel       string `json:"program_level"`
	FieldOfStudy       string `json:"field_of_study"`
	Title              string `json:"title"`
	Body               string `json:"body"`
	WordCount          *int32 `json:"word_count"`
}

// FieldDTO is the API shape for a field-of-study option (sop_fields).
type FieldDTO struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsActive  bool   `json:"is_active"`
	SortOrder int32  `json:"sort_order"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// FieldRequest is the admin create/update body for a field option.
// Pointer fields make PATCH partial — omitted keys leave the column
// unchanged via the store's COALESCE update.
type FieldRequest struct {
	Name      string `json:"name"`
	IsActive  *bool  `json:"is_active"`
	SortOrder *int32 `json:"sort_order"`
}

// QuotaDTO is the GET /sop/quota response.
type QuotaDTO struct {
	Limit     int   `json:"limit"`
	Used      int64 `json:"used"`
	Remaining int64 `json:"remaining"`
}

// Service orchestrates the SOP generator.
type Service struct {
	store        *Store
	settings     *config.Settings
	enqueuer     TaskEnqueuer
	cache        Cache
	aiDispatcher AIDispatcher
}

// NewService creates the service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{store: store, settings: settings}
}

// SetEnqueuer injects the Asynq task enqueuer for sop:generate.
func (s *Service) SetEnqueuer(e TaskEnqueuer) { s.enqueuer = e }

// SetCache injects the Valkey cache for the quota counter
// (jc:sop_quota:{user_id}:{YYYY-MM}). Nil-safe: falls back to DB count.
func (s *Service) SetCache(c Cache) { s.cache = c }

// SetAIDispatcher injects the §20 AI Gateway dispatcher. Nil-safe:
// ProcessDraft marks the draft failed when no dispatcher is wired.
func (s *Service) SetAIDispatcher(d AIDispatcher) { s.aiDispatcher = d }

// ListTemplates returns published templates.
func (s *Service) ListTemplates(ctx context.Context, destSpaceID pgtype.UUID, purpose string) ([]TemplateDTO, error) {
	rows, err := s.store.ListTemplates(ctx, destSpaceID, purpose)
	if err != nil {
		return nil, err
	}
	out := make([]TemplateDTO, 0, len(rows))
	for _, t := range rows {
		out = append(out, templateDTO(t))
	}
	return out, nil
}

// ListSamples returns published samples.
func (s *Service) ListSamples(ctx context.Context, destSpaceID pgtype.UUID, level, field string, page, limit int32) ([]SampleDTO, int64, error) {
	rows, total, err := s.store.ListSamples(ctx, destSpaceID, level, field, page, limit)
	if err != nil {
		return nil, 0, err
	}
	return sampleDTOs(rows), total, nil
}

// GetSample returns one published sample.
func (s *Service) GetSample(ctx context.Context, id pgtype.UUID) (SampleDTO, error) {
	r, err := s.store.GetSample(ctx, id)
	if err != nil {
		return SampleDTO{}, err
	}
	return sampleDTOs([]SampleRow{r})[0], nil
}

// Quota returns the caller's remaining monthly generation quota.
func (s *Service) Quota(ctx context.Context, userID pgtype.UUID) (QuotaDTO, error) {
	limit := s.quotaLimit(ctx)
	used, err := s.quotaUsed(ctx, userID)
	if err != nil {
		return QuotaDTO{}, err
	}
	remaining := int64(limit) - used
	if remaining < 0 {
		remaining = 0
	}
	return QuotaDTO{Limit: limit, Used: used, Remaining: remaining}, nil
}

// Generate validates quota, creates a queued draft, and enqueues
// sop:generate (fire-and-forget, Rule 8). Returns the draft id.
func (s *Service) Generate(ctx context.Context, userID pgtype.UUID, in GenerateRequest) (string, error) {
	if !validPurpose(in.Purpose) {
		return "", ErrInvalidPurpose
	}
	if len(in.Inputs) == 0 {
		return "", ErrInvalidInput
	}

	release, err := s.consumeQuota(ctx, userID)
	if err != nil {
		return "", err
	}

	row := DraftRow{
		UserID:  userID,
		Purpose: in.Purpose,
		Inputs:  in.Inputs,
	}
	if in.DestinationSpaceID != "" {
		if err := row.DestinationSpaceID.Scan(in.DestinationSpaceID); err != nil {
			return "", fmt.Errorf("invalid destination_space_id: %w", ErrInvalidInput)
		}
	}
	if in.Program != "" {
		row.Program = &in.Program
	}
	if in.Institution != "" {
		row.Institution = &in.Institution
	}
	if in.Title != "" {
		row.Title = &in.Title
	}

	// ADR-088 provenance: hash the generation inputs.
	hash := sha256.Sum256(in.Inputs)
	promptHash := hex.EncodeToString(hash[:])
	row.AIPromptHash = &promptHash

	id, err := s.store.CreateDraft(ctx, row)
	if err != nil {
		// Refund the reserved quota slot — the draft never materialized.
		release()
		return "", fmt.Errorf("sopgenerator: create draft: %w", err)
	}

	if s.enqueuer != nil {
		payload, err := json.Marshal(map[string]string{"draft_id": id.String()})
		if err != nil {
			slog.Error("sopgenerator: marshal generate payload", "error", err)
		} else if err := s.enqueuer.Enqueue("sop:generate", payload); err != nil {
			slog.Error("sopgenerator: enqueue sop:generate", "draft_id", id.String(), "error", err)
		}
	}

	return uuidStr(id), nil
}

// ProcessDraft implements worker.SopProcessor. Loads a queued draft,
// builds the prompt from the configurable template + inputs, dispatches
// to the §20 AI Gateway, and marks the draft generated. Idempotent —
// re-processing a generated/failed draft is a no-op.
func (s *Service) ProcessDraft(ctx context.Context, draftID pgtype.UUID) error {
	draft, err := s.store.GetDraft(ctx, draftID)
	if err != nil {
		// Deleted or unknown draft — nothing to process. Returning nil
		// stops Asynq from retrying a permanently-gone row.
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("sopgenerator: process: load draft: %w", err)
	}
	if draft.Status != DraftQueued {
		return nil
	}

	if s.aiDispatcher == nil {
		_ = s.store.MarkDraftFailed(ctx, draftID)
		return errors.New("sopgenerator: process: no AI dispatcher configured")
	}

	systemPrompt := s.systemPrompt(ctx, draft)

	resp, err := s.aiDispatcher.Dispatch(ctx, "content_generation", AIRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   string(draft.Inputs),
	})
	if err != nil {
		_ = s.store.MarkDraftFailed(ctx, draftID)
		return fmt.Errorf("sopgenerator: process: AI dispatch: %w", err)
	}

	var model *string
	if resp.Model != "" {
		model = &resp.Model
	}

	if err := s.store.MarkDraftGenerated(ctx, draftID, resp.Text, model); err != nil {
		return fmt.Errorf("sopgenerator: process: update draft: %w", err)
	}
	return nil
}

// ListDrafts returns the caller's drafts.
func (s *Service) ListDrafts(ctx context.Context, userID pgtype.UUID, page, limit int32) ([]DraftDTO, int64, error) {
	rows, total, err := s.store.ListDrafts(ctx, userID, page, limit)
	if err != nil {
		return nil, 0, err
	}
	out := make([]DraftDTO, 0, len(rows))
	for _, d := range rows {
		out = append(out, draftDTO(d))
	}
	return out, total, nil
}

// GetDraft returns a draft if the caller owns it.
func (s *Service) GetDraft(ctx context.Context, id, userID pgtype.UUID) (DraftDTO, error) {
	row, err := s.store.GetDraft(ctx, id)
	if err != nil {
		return DraftDTO{}, err
	}
	if row.UserID != userID {
		return DraftDTO{}, ErrNotOwner
	}
	return draftDTO(row), nil
}

// UpdateDraft edits a draft the caller owns.
func (s *Service) UpdateDraft(ctx context.Context, id, userID pgtype.UUID, in UpdateDraftRequest) error {
	return s.store.UpdateDraft(ctx, id, userID, in.Title, in.Program, in.Institution, in.GeneratedText)
}

// DeleteDraft soft-deletes a draft the caller owns (Rule 11).
func (s *Service) DeleteDraft(ctx context.Context, id, userID pgtype.UUID) error {
	return s.store.SoftDeleteDraft(ctx, id, userID)
}

// AdminListTemplates returns all templates (admin).
func (s *Service) AdminListTemplates(ctx context.Context, page, limit int32) ([]TemplateDTO, int64, error) {
	rows, total, err := s.store.AdminListTemplates(ctx, page, limit)
	if err != nil {
		return nil, 0, err
	}
	out := make([]TemplateDTO, 0, len(rows))
	for _, t := range rows {
		out = append(out, templateDTO(t))
	}
	return out, total, nil
}

// ApproveSample publishes a sample after HITL review and enqueues it
// for Meilisearch indexing (fire-and-forget, Rule 8).
func (s *Service) ApproveSample(ctx context.Context, id, reviewerID pgtype.UUID, notes string) error {
	if err := s.store.ApproveSample(ctx, id, reviewerID, notes); err != nil {
		return err
	}
	if s.enqueuer != nil {
		payload, err := json.Marshal(map[string]string{
			"entity_type": "sop_sample",
			"entity_id":   id.String(),
		})
		if err != nil {
			slog.Error("sopgenerator: marshal index payload", "error", err)
		} else if err := s.enqueuer.Enqueue("search:index_entity", payload); err != nil {
			slog.Error("sopgenerator: enqueue index task", "id", id.String(), "error", err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------
// Quota (mirrors cvformatter — §95 pattern)
// ---------------------------------------------------------------------

func (s *Service) quotaLimit(ctx context.Context) int {
	if s.settings != nil {
		return s.settings.GetIntOr(ctx, "sop_generator.quota_standard", 3)
	}
	return 3
}

func (s *Service) quotaUsed(ctx context.Context, userID pgtype.UUID) (int64, error) {
	return s.store.CountDraftsThisMonth(ctx, userID)
}

// consumeQuota increments the monthly counter and rejects when over the
// limit. Valkey INCR with end-of-month TTL; DB count fallback. It returns
// a release function that refunds the reserved slot — callers invoke it
// when the follow-up write fails so a transient insert error does not
// burn a monthly generation.
func (s *Service) consumeQuota(ctx context.Context, userID pgtype.UUID) (func(), error) {
	noop := func() {}
	limit := s.quotaLimit(ctx)
	if limit <= 0 {
		return noop, nil
	}

	if s.cache != nil {
		key := sopQuotaKey(userID)
		count, err := s.cache.INCR(ctx, key)
		if err != nil {
			count, err = s.store.CountDraftsThisMonth(ctx, userID)
			if err != nil {
				return noop, fmt.Errorf("sopgenerator: check quota: %w", err)
			}
			if count >= int64(limit) {
				return noop, ErrQuotaExceeded
			}
			return noop, nil
		}
		if count == 1 {
			_ = s.cache.Expire(ctx, key, timeUntilEndOfMonth())
		}
		if count > int64(limit) {
			return noop, ErrQuotaExceeded
		}
		return func() { _, _ = s.cache.DECR(ctx, key) }, nil
	}

	count, err := s.store.CountDraftsThisMonth(ctx, userID)
	if err != nil {
		return noop, fmt.Errorf("sopgenerator: check quota: %w", err)
	}
	if count >= int64(limit) {
		return noop, ErrQuotaExceeded
	}
	return noop, nil
}

// sopQuotaKey builds the cataloged key: jc:sop_quota:{user_id}:{YYYY-MM}.
func sopQuotaKey(userID pgtype.UUID) string {
	return "jc:sop_quota:" + userID.String() + ":" + time.Now().UTC().Format("2006-01")
}

func timeUntilEndOfMonth() time.Duration {
	now := time.Now().UTC()
	next := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	return next.Sub(now)
}

// systemPrompt renders the configured prompt template with the draft's
// context fields. The template lives in system_settings
// (sop_generator.prompt_template, Rule 3) as a jsonb object; when empty a
// sensible built-in scaffold is used.
func (s *Service) systemPrompt(ctx context.Context, d DraftRow) string {
	if s.settings != nil {
		raw := s.settings.GetStringOr(ctx, "sop_generator.prompt_template", "")
		if raw != "" && raw != "{}" {
			var tpl map[string]string
			if err := json.Unmarshal([]byte(raw), &tpl); err == nil {
				if v, ok := tpl["system"]; ok && v != "" {
					return v
				}
			}
		}
	}
	return "You are an expert statement-of-purpose writer for study and " +
		"work visa applications. Write a first-person SOP that is honest, " +
		"specific, and grounded in the applicant's stated background. " +
		"Address purpose, academic/professional fit, and intent to comply " +
		"with visa conditions. Do not fabricate credentials."
}

// ---------------------------------------------------------------------
// DTO helpers
// ---------------------------------------------------------------------

func templateDTO(t TemplateRow) TemplateDTO {
	out := TemplateDTO{
		ID:                 uuidStr(t.ID),
		DestinationSpaceID: uuidStr(t.DestinationSpaceID),
		Purpose:            t.Purpose,
		Title:              t.Title,
		Status:             t.Status,
		IsActive:           t.IsActive.Bool,
		AIGenerated:        t.AIGenerated,
	}
	if len(t.Structure) > 0 {
		out.Structure = json.RawMessage(t.Structure)
	}
	if t.CreatedAt.Valid {
		out.CreatedAt = t.CreatedAt.Time.Format(time.RFC3339)
	}
	if t.UpdatedAt.Valid {
		out.UpdatedAt = t.UpdatedAt.Time.Format(time.RFC3339)
	}
	return out
}

func sampleDTOs(rows []SampleRow) []SampleDTO {
	out := make([]SampleDTO, 0, len(rows))
	for _, r := range rows {
		d := SampleDTO{
			ID:           uuidStr(r.ID),
			ProgramLevel: r.ProgramLevel,
			Title:        r.Title,
			Body:         r.Body,
			AIGenerated:  r.AIGenerated,
		}
		if r.DestinationName != nil {
			d.DestinationName = *r.DestinationName
		}
		if r.FieldOfStudy != nil {
			d.FieldOfStudy = *r.FieldOfStudy
		}
		if r.WordCount.Valid {
			v := r.WordCount.Int32
			d.WordCount = &v
		}
		out = append(out, d)
	}
	return out
}

func draftDTO(d DraftRow) DraftDTO {
	out := DraftDTO{
		ID:        uuidStr(d.ID),
		Purpose:   d.Purpose,
		Status:    d.Status,
		Version:   d.Version,
		CreatedAt: d.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt: d.UpdatedAt.Time.Format(time.RFC3339),
	}
	if d.DestinationSpaceID.Valid {
		out.DestinationSpaceID = uuidStr(d.DestinationSpaceID)
	}
	if d.Program != nil {
		out.Program = *d.Program
	}
	if d.Institution != nil {
		out.Institution = *d.Institution
	}
	if d.Title != nil {
		out.Title = *d.Title
	}
	if len(d.Inputs) > 0 {
		out.Inputs = json.RawMessage(d.Inputs)
	}
	if d.GeneratedText != nil {
		out.GeneratedText = *d.GeneratedText
	}
	if d.AIModel != nil {
		out.AIModel = *d.AIModel
	}
	return out
}

func uuidStr(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// ApproveTemplate publishes a template draft (HITL approve from the
// unified moderation queue).
func (s *Service) ApproveTemplate(ctx context.Context, id, reviewerID pgtype.UUID) error {
	return s.store.ApproveTemplate(ctx, id, reviewerID, "")
}

// RejectTemplate marks a template draft rejected (HITL reject).
func (s *Service) RejectTemplate(ctx context.Context, id pgtype.UUID) error {
	return s.store.RejectTemplate(ctx, id)
}

// RejectSample marks a pending/draft sample rejected (HITL reject).
func (s *Service) RejectSample(ctx context.Context, id, reviewerID pgtype.UUID, notes string) error {
	return s.store.RejectSample(ctx, id, reviewerID, notes)
}

// CreateTemplate inserts a draft template (admin).
func (s *Service) CreateTemplate(ctx context.Context, in TemplateRequest) (TemplateDTO, error) {
	if !validPurpose(in.Purpose) {
		return TemplateDTO{}, ErrInvalidPurpose
	}
	if in.Title == "" || len(in.Structure) == 0 {
		return TemplateDTO{}, fmt.Errorf("title and structure are required: %w", ErrInvalidInput)
	}
	row := TemplateRow{
		Purpose: in.Purpose, Title: in.Title, Structure: in.Structure,
		Status: StatusDraft,
	}
	if in.DestinationSpaceID != "" {
		var sid pgtype.UUID
		if err := sid.Scan(in.DestinationSpaceID); err != nil {
			return TemplateDTO{}, fmt.Errorf("invalid destination_space_id: %w", ErrInvalidInput)
		}
		row.DestinationSpaceID = sid
	}
	id, err := s.store.CreateTemplate(ctx, row)
	if err != nil {
		return TemplateDTO{}, err
	}
	row.ID = id
	row.IsActive = pgtype.Bool{Bool: true, Valid: true}
	return templateDTO(row), nil
}

// UpdateTemplate applies a partial update (admin).
func (s *Service) UpdateTemplate(ctx context.Context, id pgtype.UUID, in TemplateRequest) error {
	if in.Purpose != "" && !validPurpose(in.Purpose) {
		return ErrInvalidPurpose
	}
	row := TemplateRow{Purpose: in.Purpose, Title: in.Title, Structure: in.Structure}
	if in.DestinationSpaceID != "" {
		var sid pgtype.UUID
		if err := sid.Scan(in.DestinationSpaceID); err != nil {
			return fmt.Errorf("invalid destination_space_id: %w", ErrInvalidInput)
		}
		row.DestinationSpaceID = sid
	}
	if in.IsActive != nil {
		row.IsActive = pgtype.Bool{Bool: *in.IsActive, Valid: true}
	}
	return s.store.UpdateTemplate(ctx, id, row)
}

// DeleteTemplate removes a non-published template.
func (s *Service) DeleteTemplate(ctx context.Context, id pgtype.UUID) error {
	return s.store.DeleteTemplate(ctx, id)
}

// AdminListSamples returns all samples regardless of status (admin).
func (s *Service) AdminListSamples(ctx context.Context, destSpaceID pgtype.UUID, status string, page, limit int32) ([]SampleDTO, int64, error) {
	rows, total, err := s.store.AdminListSamples(ctx, destSpaceID, status, page, limit)
	if err != nil {
		return nil, 0, err
	}
	out := sampleDTOs(rows)
	for i, r := range rows {
		out[i].Status = r.Status
		if r.DestinationSpaceID.Valid {
			out[i].DestinationSpaceID = uuidStr(r.DestinationSpaceID)
		}
	}
	return out, total, nil
}

// CreateSample inserts a draft sample (admin).
func (s *Service) CreateSample(ctx context.Context, in SampleRequest) (SampleDTO, error) {
	if !validProgramLevel(in.ProgramLevel) {
		return SampleDTO{}, ErrInvalidProgramLevel
	}
	if in.Title == "" || in.Body == "" {
		return SampleDTO{}, fmt.Errorf("title and body are required: %w", ErrInvalidInput)
	}
	row := SampleRow{
		ProgramLevel: in.ProgramLevel, Title: in.Title, Body: in.Body,
		Status: StatusDraft,
	}
	if in.FieldOfStudy != "" {
		row.FieldOfStudy = &in.FieldOfStudy
	}
	if in.WordCount != nil {
		row.WordCount = pgtype.Int4{Int32: *in.WordCount, Valid: true}
	} else {
		row.WordCount = pgtype.Int4{Int32: int32(len(strings.Fields(in.Body))), Valid: true}
	}
	if in.DestinationSpaceID != "" {
		var sid pgtype.UUID
		if err := sid.Scan(in.DestinationSpaceID); err != nil {
			return SampleDTO{}, fmt.Errorf("invalid destination_space_id")
		}
		row.DestinationSpaceID = sid
	}
	id, err := s.store.CreateSample(ctx, row)
	if err != nil {
		return SampleDTO{}, err
	}
	s.ensureField(ctx, in.FieldOfStudy)
	out := sampleDTOs([]SampleRow{row})[0]
	out.ID = uuidStr(id)
	out.Status = StatusDraft
	return out, nil
}

// UpdateSample applies a partial update (admin).
func (s *Service) UpdateSample(ctx context.Context, id pgtype.UUID, in SampleRequest) error {
	if in.ProgramLevel != "" && !validProgramLevel(in.ProgramLevel) {
		return ErrInvalidProgramLevel
	}
	row := SampleRow{ProgramLevel: in.ProgramLevel, Title: in.Title, Body: in.Body}
	if in.FieldOfStudy != "" {
		row.FieldOfStudy = &in.FieldOfStudy
	}
	if in.WordCount != nil {
		row.WordCount = pgtype.Int4{Int32: *in.WordCount, Valid: true}
	}
	if in.DestinationSpaceID != "" {
		var sid pgtype.UUID
		if err := sid.Scan(in.DestinationSpaceID); err != nil {
			return fmt.Errorf("invalid destination_space_id")
		}
		row.DestinationSpaceID = sid
	}
	if err := s.store.UpdateSample(ctx, id, row); err != nil {
		return err
	}
	s.ensureField(ctx, in.FieldOfStudy)
	return nil
}

// DeleteSample removes a non-published sample.
func (s *Service) DeleteSample(ctx context.Context, id pgtype.UUID) error {
	return s.store.DeleteSample(ctx, id)
}

// ---------------------------------------------------------------------
// Fields (sop_fields) — curated field-of-study dictionary
// ---------------------------------------------------------------------

func fieldDTO(r FieldRow) FieldDTO {
	out := FieldDTO{
		ID:        uuidStr(r.ID),
		Name:      r.Name,
		IsActive:  r.IsActive,
		SortOrder: r.SortOrder,
	}
	if r.CreatedAt.Valid {
		out.CreatedAt = r.CreatedAt.Time.Format(time.RFC3339)
	}
	if r.UpdatedAt.Valid {
		out.UpdatedAt = r.UpdatedAt.Time.Format(time.RFC3339)
	}
	return out
}

// isUniqueViolation reports whether err is a Postgres unique_violation
// (SQLSTATE 23505) — surfaces as ErrDuplicateField to the caller.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// ListFields returns the active field-of-study options (public select).
func (s *Service) ListFields(ctx context.Context) ([]FieldDTO, error) {
	rows, err := s.store.ListActiveFields(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]FieldDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, fieldDTO(r))
	}
	return out, nil
}

// AdminListFields returns all field rows with an optional name search
// (admin table pagination).
func (s *Service) AdminListFields(ctx context.Context, q string, page, limit int32) ([]FieldDTO, int64, error) {
	rows, total, err := s.store.AdminListFields(ctx, q, page, limit)
	if err != nil {
		return nil, 0, err
	}
	out := make([]FieldDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, fieldDTO(r))
	}
	return out, total, nil
}

// CreateField inserts a field option (admin/MCP).
func (s *Service) CreateField(ctx context.Context, in FieldRequest) (FieldDTO, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return FieldDTO{}, fmt.Errorf("name is required: %w", ErrInvalidInput)
	}
	row := FieldRow{Name: name, IsActive: true}
	if in.IsActive != nil {
		row.IsActive = *in.IsActive
	}
	if in.SortOrder != nil {
		row.SortOrder = *in.SortOrder
	}
	id, err := s.store.CreateField(ctx, row)
	if err != nil {
		if isUniqueViolation(err) {
			return FieldDTO{}, ErrDuplicateField
		}
		return FieldDTO{}, err
	}
	row.ID = id
	return fieldDTO(row), nil
}

// UpdateField applies a partial update (admin/MCP).
func (s *Service) UpdateField(ctx context.Context, id pgtype.UUID, in FieldRequest) error {
	var name *string
	if n := strings.TrimSpace(in.Name); n != "" {
		name = &n
	}
	err := s.store.UpdateField(ctx, id, name, in.IsActive, in.SortOrder)
	if isUniqueViolation(err) {
		return ErrDuplicateField
	}
	return err
}

// DeleteField removes a field option (admin/MCP).
func (s *Service) DeleteField(ctx context.Context, id pgtype.UUID) error {
	return s.store.DeleteField(ctx, id)
}

// ensureField auto-curates a new field_of_study value into sop_fields —
// best-effort: the dictionary is a managed option list, not an FK target,
// so a failed upsert must not fail the sample write itself.
func (s *Service) ensureField(ctx context.Context, name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	if err := s.store.EnsureField(ctx, name); err != nil {
		slog.Warn("sopgenerator: ensure field failed", "name", name, "error", err)
	}
}

// ---------------------------------------------------------------------
// Content Agent draft generation (tool:generate_draft task)
// ---------------------------------------------------------------------

// EnqueueGenerate fires tool:generate_draft for a SOP template draft
// (fire-and-forget — the admin polls the moderation queue for the
// ai_draft row).
func (s *Service) EnqueueGenerate(ctx context.Context, params map[string]string) error {
	if s.enqueuer == nil {
		return errors.New("sopgenerator: task enqueuer not configured")
	}
	payload := map[string]string{"entity_type": "sop_template"}
	for k, v := range params {
		payload[k] = v
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.enqueuer.Enqueue("tool:generate_draft", raw)
}

// GenerateTemplateDraft implements the SOP branch of the
// tool:generate_draft worker task — the AI Gateway returns a template
// JSON document which is validated and inserted as an ai_draft row for
// HITL review (ADR-088).
func (s *Service) GenerateTemplateDraft(ctx context.Context, params map[string]string) error {
	if s.aiDispatcher == nil {
		return errors.New("sopgenerator: no AI dispatcher configured")
	}

	resp, err := s.aiDispatcher.Dispatch(ctx, "content_generation", AIRequest{
		SystemPrompt: "You are the JapaConnect Content Agent. Produce an SOP template " +
			"as STRICT JSON (no markdown fences): " +
			`{"purpose":"study_permit|student_visa|work_visa|visitor_visa",` +
			`"title":"...","structure":[{"section":"...","prompt_hint":"...","max_words":0}]}. ` +
			"structure must order sections intro → academic fit → career plan → " +
			"country/institution rationale → ties to home country → conclusion.",
		UserPrompt: formatParams(params),
	})
	if err != nil {
		return fmt.Errorf("sopgenerator: generate draft: AI dispatch: %w", err)
	}

	var out struct {
		Purpose   string           `json:"purpose"`
		Title     string           `json:"title"`
		Structure *json.RawMessage `json:"structure"`
	}
	if err := json.Unmarshal(extractJSON(resp.Text), &out); err != nil {
		return fmt.Errorf("sopgenerator: generate draft: unparseable AI output: %w", err)
	}
	if out.Purpose == "" || out.Title == "" || out.Structure == nil {
		return errors.New("sopgenerator: generate draft: AI output missing required fields")
	}

	row := TemplateRow{
		Purpose:     out.Purpose,
		Title:       out.Title,
		Structure:   *out.Structure,
		Status:      StatusAIDraft,
		AIGenerated: true,
	}
	if !validPurpose(out.Purpose) {
		return fmt.Errorf("sopgenerator: generate draft: AI purpose %q outside the §110 enum", out.Purpose)
	}
	if v := params["destination_space_id"]; v != "" {
		if err := row.DestinationSpaceID.Scan(v); err != nil {
			return fmt.Errorf("sopgenerator: generate draft: invalid destination_space_id")
		}
	}
	_, err = s.store.CreateTemplate(ctx, row)
	return err
}

// extractJSON strips markdown fences around a model's JSON response.
func extractJSON(s string) []byte {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "```"); i >= 0 {
		s = s[i:]
		if j := strings.Index(s, "\n"); j >= 0 {
			s = s[j+1:]
		}
		if k := strings.LastIndex(s, "```"); k >= 0 {
			s = s[:k]
		}
	}
	start := strings.IndexAny(s, "{[")
	if start < 0 {
		return []byte(s)
	}
	return []byte(s[start:])
}

// formatParams renders admin form hints for the prompt.
func formatParams(params map[string]string) string {
	if len(params) == 0 {
		return "Generate the template."
	}
	var b strings.Builder
	b.WriteString("Generate an SOP template with these hints:\n")
	for k, v := range params {
		fmt.Fprintf(&b, "- %s: %s\n", k, v)
	}
	return b.String()
}
