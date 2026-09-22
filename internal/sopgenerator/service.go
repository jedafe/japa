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
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
)

// ErrQuotaExceeded is returned when the monthly generation quota is hit.
var ErrQuotaExceeded = errors.New("sop_generator quota exceeded for this month")

// ErrNotOwner is returned when a draft is accessed by a non-owner.
var ErrNotOwner = errors.New("draft does not belong to this user")

// TaskEnqueuer is the minimal interface for enqueueing Asynq tasks.
// Implemented by *worker.Enqueuer; a nil enqueuer makes dispatch a
// graceful no-op (development/tests).
type TaskEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// Cache is the minimal Valkey interface for the quota counter.
type Cache interface {
	INCR(ctx context.Context, key string) (int64, error)
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
}

// SampleDTO is the API shape for a published sample.
type SampleDTO struct {
	ID              string `json:"id"`
	DestinationName string `json:"destination_space_name,omitempty"`
	ProgramLevel    string `json:"program_level"`
	FieldOfStudy    string `json:"field_of_study,omitempty"`
	Title           string `json:"title"`
	Body            string `json:"body"`
	WordCount       *int32 `json:"word_count,omitempty"`
	AIGenerated     bool   `json:"ai_generated"`
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
		out = append(out, TemplateDTO{
			ID:                 uuidStr(t.ID),
			DestinationSpaceID: uuidStr(t.DestinationSpaceID),
			Purpose:            t.Purpose,
			Title:              t.Title,
			Structure:          json.RawMessage(t.Structure),
		})
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
	if in.Purpose == "" || len(in.Inputs) == 0 {
		return "", errors.New("purpose and inputs are required")
	}

	if err := s.checkQuota(ctx, userID); err != nil {
		return "", err
	}

	row := DraftRow{
		UserID:  userID,
		Purpose: in.Purpose,
		Inputs:  in.Inputs,
	}
	if in.DestinationSpaceID != "" {
		if err := row.DestinationSpaceID.Scan(in.DestinationSpaceID); err != nil {
			return "", fmt.Errorf("invalid destination_space_id")
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
		out = append(out, TemplateDTO{
			ID:                 uuidStr(t.ID),
			DestinationSpaceID: uuidStr(t.DestinationSpaceID),
			Purpose:            t.Purpose,
			Title:              t.Title,
			Structure:          json.RawMessage(t.Structure),
		})
	}
	return out, total, nil
}

// ApproveSample publishes a sample after HITL review.
func (s *Service) ApproveSample(ctx context.Context, id, reviewerID pgtype.UUID, notes string) error {
	return s.store.ApproveSample(ctx, id, reviewerID, notes)
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

// checkQuota increments the monthly counter and rejects when over the
// limit. Valkey INCR with end-of-month TTL; DB count fallback.
func (s *Service) checkQuota(ctx context.Context, userID pgtype.UUID) error {
	limit := s.quotaLimit(ctx)
	if limit <= 0 {
		return nil
	}

	if s.cache != nil {
		key := sopQuotaKey(userID)
		count, err := s.cache.INCR(ctx, key)
		if err != nil {
			count, err = s.store.CountDraftsThisMonth(ctx, userID)
			if err != nil {
				return fmt.Errorf("sopgenerator: check quota: %w", err)
			}
			if count >= int64(limit) {
				return ErrQuotaExceeded
			}
			return nil
		}
		if count == 1 {
			_ = s.cache.Expire(ctx, key, timeUntilEndOfMonth())
		}
		if count > int64(limit) {
			return ErrQuotaExceeded
		}
		return nil
	}

	count, err := s.store.CountDraftsThisMonth(ctx, userID)
	if err != nil {
		return fmt.Errorf("sopgenerator: check quota: %w", err)
	}
	if count >= int64(limit) {
		return ErrQuotaExceeded
	}
	return nil
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
