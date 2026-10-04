// Package pofcalculator — service.go resolves household size → required
// amount from the JSONB amount_schedule and converts to NGN/USD via the
// shared §74 remittance FX service (no duplicate FX logic).
// // Ref: japaconnect-engineering/sections/108-pof-calculator.md
// //	japaconnect-engineering/adr/ADR-119-pof-calculator.md
package pofcalculator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
)

// FXConverter is the seam into the §74 remittance service. Implemented by
// remittance.Service.Calculate — BestReceiveAmount is the conversion via
// the best provider rate. Nil-able: conversion fields are simply omitted
// when no FX service is wired or it is disabled.
type FXConverter interface {
	Calculate(ctx context.Context, sendAmount float64, sendCurrency, receiveCurrency string) (*CalculateResult, error)
}

// CalculateResult mirrors remittance.CalculateResult (kept minimal to
// avoid importing the remittance package internals).
type CalculateResult struct {
	Results           []ProviderCalculation
	BestReceiveAmount float64
}

// ProviderCalculation is one provider's conversion output.
type ProviderCalculation struct {
	ReceiveAmount float64
}

// RequirementDTO is the API shape for a requirement row.
type RequirementDTO struct {
	ID                   string          `json:"id"`
	DestinationSpaceID   string          `json:"destination_space_id"`
	DestinationSpaceName string          `json:"destination_space_name,omitempty"`
	DestinationSpaceCode string          `json:"destination_space_code,omitempty"`
	VisaType             string          `json:"visa_type"`
	ProgramLabel         string          `json:"program_label"`
	AmountSchedule       json.RawMessage `json:"amount_schedule"`
	Currency             string          `json:"currency"`
	FundsMustBeLiquid    bool            `json:"funds_must_be_liquid"`
	AcceptedProof        json.RawMessage `json:"accepted_proof,omitempty"`
	Notes                string          `json:"notes,omitempty"`
	EffectiveDate        string          `json:"effective_date"`
	SourceURL            string          `json:"source_url,omitempty"`
	SourceURLLastChecked string          `json:"source_url_last_checked,omitempty"`
	Status               string          `json:"status"`
	IsActive             bool            `json:"is_active"`
	AIGenerated          bool            `json:"ai_generated"`
	IsJapabotIndexed     bool            `json:"is_japabot_indexed"`
	HITLReviewedAt       string          `json:"hitl_reviewed_at,omitempty"`
	CreatedAt            string          `json:"created_at,omitempty"`
	UpdatedAt            string          `json:"updated_at,omitempty"`
}

// CalculateResultDTO is one resolved requirement for a household.
type CalculateResultDTO struct {
	RequirementID     string          `json:"requirement_id"`
	ProgramLabel      string          `json:"program_label"`
	Household         int             `json:"household"`
	Amount            float64         `json:"amount"`
	Currency          string          `json:"currency"`
	NGNEquivalent     *float64        `json:"ngn_equivalent,omitempty"`
	USDEquivalent     *float64        `json:"usd_equivalent,omitempty"`
	FundsMustBeLiquid bool            `json:"funds_must_be_liquid"`
	AcceptedProof     json.RawMessage `json:"accepted_proof,omitempty"`
	Notes             string          `json:"notes,omitempty"`
	EffectiveDate     string          `json:"effective_date"`
	SourceURL         string          `json:"source_url,omitempty"`
}

// Admin request bodies.
type RequirementRequest struct {
	DestinationSpaceID string           `json:"destination_space_id"`
	VisaType           string           `json:"visa_type"`
	ProgramLabel       string           `json:"program_label"`
	AmountSchedule     *json.RawMessage `json:"amount_schedule"`
	Currency           string           `json:"currency"`
	FundsMustBeLiquid  *bool            `json:"funds_must_be_liquid"`
	AcceptedProof      *json.RawMessage `json:"accepted_proof"`
	Notes              *string          `json:"notes"`
	EffectiveDate      string           `json:"effective_date"`
	SourceURL          *string          `json:"source_url"`
	Status             string           `json:"status"`
	IsActive           *bool            `json:"is_active"`
}

// ApproveRequest is the optional approve body.
type ApproveRequest struct {
	ReviewNotes string `json:"review_notes"`
}

// errInvalidInput marks request-validation failures so the handler can
// map them to 400 instead of 500.
var errInvalidInput = errors.New("invalid input")

// Service orchestrates POF lookups and calculation.
type Service struct {
	store        *Store
	settings     *config.Settings
	fx           FXConverter
	enqueuer     TaskEnqueuer
	aiDispatcher AIDispatcher
}

// NewService creates the service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{store: store, settings: settings}
}

// SetFXConverter wires the §74 remittance service for NGN/USD conversion.
func (s *Service) SetFXConverter(fx FXConverter) { s.fx = fx }

// ListRequirements returns published requirements for ?dest=&visa_type=.
func (s *Service) ListRequirements(ctx context.Context, destSpaceID pgtype.UUID, visaType string) ([]RequirementDTO, error) {
	rows, err := s.store.ListRequirements(ctx, destSpaceID, visaType)
	if err != nil {
		return nil, err
	}
	out := make([]RequirementDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, requirementDTO(r))
	}
	return out, nil
}

// VisaTypes returns distinct visa types for a destination.
func (s *Service) VisaTypes(ctx context.Context, destSpaceID pgtype.UUID) ([]string, error) {
	return s.store.DistinctVisaTypes(ctx, destSpaceID)
}

// Calculate resolves every published requirement matching
// dest × visa_type against the household size and converts via FX.
func (s *Service) Calculate(ctx context.Context, destSpaceID pgtype.UUID, visaType string, members, dependants int) ([]CalculateResultDTO, error) {
	if members < 1 {
		return nil, errors.New("members must be >= 1")
	}
	if members+dependants > 50 {
		return nil, errors.New("household size out of range")
	}
	household := members + dependants

	rows, err := s.store.ListRequirements(ctx, destSpaceID, visaType)
	if err != nil {
		return nil, err
	}

	out := make([]CalculateResultDTO, 0, len(rows))
	for _, r := range rows {
		amount, err := resolveSchedule(r.AmountSchedule, household)
		if err != nil {
			continue // malformed schedule — skip rather than 500 the whole call
		}
		dto := CalculateResultDTO{
			RequirementID:     uuidStr(r.ID),
			ProgramLabel:      r.ProgramLabel,
			Household:         household,
			Amount:            amount,
			Currency:          r.Currency,
			FundsMustBeLiquid: r.FundsMustBeLiquid.Bool,
			EffectiveDate:     r.EffectiveDate.Time.Format("2006-01-02"),
		}
		if len(r.AcceptedProof) > 0 {
			dto.AcceptedProof = json.RawMessage(r.AcceptedProof)
		}
		if r.Notes != nil {
			dto.Notes = *r.Notes
		}
		if r.SourceURL != nil {
			dto.SourceURL = *r.SourceURL
		}
		s.convert(ctx, &dto)
		out = append(out, dto)
	}
	return out, nil
}

// convert fills NGN/USD equivalents via the remittance service. Failures
// (service disabled, no corridor rates) silently omit the fields — the
// primary amount in the destination currency is always authoritative.
func (s *Service) convert(ctx context.Context, dto *CalculateResultDTO) {
	if s.fx == nil {
		return
	}
	for _, target := range []string{"NGN", "USD"} {
		if dto.Currency == target {
			continue
		}
		res, err := s.fx.Calculate(ctx, dto.Amount, dto.Currency, target)
		if err != nil || res == nil || res.BestReceiveAmount <= 0 {
			continue
		}
		amt := res.BestReceiveAmount
		switch target {
		case "NGN":
			dto.NGNEquivalent = &amt
		case "USD":
			dto.USDEquivalent = &amt
		}
	}
}

// AdminListRequirements returns requirements in any status (admin).
func (s *Service) AdminListRequirements(ctx context.Context, destSpaceID pgtype.UUID, visaType, status, q, sort, order string, page, limit int32) ([]RequirementDTO, int64, error) {
	rows, total, err := s.store.AdminListRequirements(ctx, destSpaceID, visaType, status, q, sort, order, page, limit)
	if err != nil {
		return nil, 0, err
	}
	out := make([]RequirementDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, requirementDTO(r))
	}
	return out, total, nil
}

// CreateRequirement inserts a requirement (admin).
func (s *Service) CreateRequirement(ctx context.Context, in RequirementRequest) (*RequirementDTO, error) {
	if in.DestinationSpaceID == "" || in.VisaType == "" || in.ProgramLabel == "" ||
		in.AmountSchedule == nil || in.Currency == "" || in.EffectiveDate == "" {
		return nil, errors.New("destination_space_id, visa_type, program_label, amount_schedule, currency and effective_date are required")
	}

	var dest pgtype.UUID
	if err := dest.Scan(in.DestinationSpaceID); err != nil {
		return nil, fmt.Errorf("invalid destination_space_id")
	}
	var eff pgtype.Date
	if err := eff.Scan(in.EffectiveDate); err != nil {
		return nil, fmt.Errorf("invalid effective_date")
	}
	if _, err := resolveSchedule(*in.AmountSchedule, 1); err != nil {
		return nil, fmt.Errorf("invalid amount_schedule: %w", err)
	}

	status := in.Status
	if status == "" {
		status = StatusDraft
	}
	// HITL gate (ADR-088): only draft/pending may be set directly.
	// published comes from the approve endpoint, ai_draft from the
	// Content Agent, rejected from the reject endpoint.
	if status != StatusDraft && status != StatusPending {
		return nil, errors.New("status must be draft or pending; review transitions use the approve/reject endpoints")
	}
	row := RequirementRow{
		DestinationSpaceID: dest,
		VisaType:           in.VisaType,
		ProgramLabel:       in.ProgramLabel,
		AmountSchedule:     *in.AmountSchedule,
		Currency:           in.Currency,
		EffectiveDate:      eff,
		SourceURL:          in.SourceURL,
		Notes:              in.Notes,
		Status:             status,
	}
	if in.FundsMustBeLiquid != nil {
		row.FundsMustBeLiquid = pgtype.Bool{Bool: *in.FundsMustBeLiquid, Valid: true}
	} else {
		row.FundsMustBeLiquid = pgtype.Bool{Bool: true, Valid: true}
	}
	if in.AcceptedProof != nil {
		row.AcceptedProof = *in.AcceptedProof
	}

	id, err := s.store.CreateRequirement(ctx, row)
	if err != nil {
		return nil, err
	}
	dto := requirementDTO(row)
	dto.ID = uuidStr(id)
	return &dto, nil
}

// UpdateRequirement applies a partial update (admin).
func (s *Service) UpdateRequirement(ctx context.Context, id pgtype.UUID, in RequirementRequest) error {
	// HITL gate (ADR-088): status transitions to published/ai_draft/
	// rejected are only allowed through the approve/reject/generate
	// endpoints — direct PATCH may only set draft or pending.
	if in.Status != "" && in.Status != StatusDraft && in.Status != StatusPending {
		return fmt.Errorf("%w: status must be draft or pending; review transitions use the approve/reject endpoints", errInvalidInput)
	}
	// A published row may not be demoted through PATCH — withdraw it via
	// is_active instead so live data cannot leave the review flow silently.
	if in.Status != "" {
		cur, err := s.store.GetRequirement(ctx, id)
		if err != nil {
			return err
		}
		if cur.Status == StatusPublished {
			return fmt.Errorf("%w: published requirements cannot change status via PATCH; deactivate instead", errInvalidInput)
		}
	}
	row := RequirementRow{
		VisaType:     in.VisaType,
		ProgramLabel: in.ProgramLabel,
		Currency:     in.Currency,
		Notes:        in.Notes,
		SourceURL:    in.SourceURL,
		Status:       in.Status,
	}
	if in.DestinationSpaceID != "" {
		var dest pgtype.UUID
		if err := dest.Scan(in.DestinationSpaceID); err != nil {
			return fmt.Errorf("%w: invalid destination_space_id", errInvalidInput)
		}
		row.DestinationSpaceID = dest
	}
	if in.AmountSchedule != nil {
		if _, err := resolveSchedule(*in.AmountSchedule, 1); err != nil {
			return fmt.Errorf("%w: invalid amount_schedule: %w", errInvalidInput, err)
		}
		row.AmountSchedule = *in.AmountSchedule
	}
	if in.AcceptedProof != nil {
		row.AcceptedProof = *in.AcceptedProof
	}
	if in.FundsMustBeLiquid != nil {
		row.FundsMustBeLiquid = pgtype.Bool{Bool: *in.FundsMustBeLiquid, Valid: true}
	}
	if in.IsActive != nil {
		row.IsActive = pgtype.Bool{Bool: *in.IsActive, Valid: true}
	}
	if in.EffectiveDate != "" {
		var eff pgtype.Date
		if err := eff.Scan(in.EffectiveDate); err != nil {
			return fmt.Errorf("%w: invalid effective_date", errInvalidInput)
		}
		row.EffectiveDate = eff
	}
	return s.store.UpdateRequirement(ctx, id, row)
}

// ApproveRequirement publishes a requirement after HITL review and
// enqueues it for Meilisearch indexing.
func (s *Service) ApproveRequirement(ctx context.Context, id, reviewerID pgtype.UUID, notes string) error {
	if err := s.store.ApproveRequirement(ctx, id, reviewerID, notes); err != nil {
		return err
	}
	s.enqueueIndex(id)
	return nil
}

// ---------------------------------------------------------------------
// Amount schedule resolution
// ---------------------------------------------------------------------

// scheduleEntry is one row of the array schedule form.
type scheduleEntry struct {
	Members       int     `json:"members"`
	Amount        float64 `json:"amount"`
	PerAdditional float64 `json:"per_additional,omitempty"`
}

// scheduleFormula is the object schedule form.
type scheduleFormula struct {
	Base          float64 `json:"base"`
	BaseMembers   int     `json:"base_members"`
	PerAdditional float64 `json:"per_additional"`
}

// resolveSchedule computes the required amount for a household size.
// Array form picks the exact band, or the highest band plus
// per_additional overage; object form is a linear formula.
func resolveSchedule(raw []byte, household int) (float64, error) {
	var entries []scheduleEntry
	if err := json.Unmarshal(raw, &entries); err == nil && len(entries) > 0 {
		// exact match
		for _, e := range entries {
			if e.Members == household {
				return e.Amount, nil
			}
		}
		// largest band ≤ household; overage via that row's per_additional
		var best *scheduleEntry
		for i := range entries {
			if entries[i].Members <= household && (best == nil || entries[i].Members > best.Members) {
				best = &entries[i]
			}
		}
		if best == nil {
			return 0, errors.New("household below smallest schedule band")
		}
		amt := best.Amount
		if household > best.Members && best.PerAdditional > 0 {
			amt += float64(household-best.Members) * best.PerAdditional
		}
		return amt, nil
	}

	var f scheduleFormula
	if err := json.Unmarshal(raw, &f); err == nil && f.BaseMembers > 0 {
		if household <= f.BaseMembers {
			return f.Base, nil
		}
		return f.Base + float64(household-f.BaseMembers)*f.PerAdditional, nil
	}

	return 0, errors.New("unrecognised amount_schedule shape")
}

// ---------------------------------------------------------------------
// DTO helpers
// ---------------------------------------------------------------------

func requirementDTO(r RequirementRow) RequirementDTO {
	d := RequirementDTO{
		ID:                 uuidStr(r.ID),
		DestinationSpaceID: uuidStr(r.DestinationSpaceID),
		VisaType:           r.VisaType,
		ProgramLabel:       r.ProgramLabel,
		AmountSchedule:     json.RawMessage(r.AmountSchedule),
		Currency:           r.Currency,
		Status:             r.Status,
		AIGenerated:        r.AIGenerated,
		IsJapabotIndexed:   r.IsJapabotIndexed,
	}
	if r.FundsMustBeLiquid.Valid {
		d.FundsMustBeLiquid = r.FundsMustBeLiquid.Bool
	}
	if r.IsActive.Valid {
		d.IsActive = r.IsActive.Bool
	}
	if r.DestinationSpaceName != nil {
		d.DestinationSpaceName = *r.DestinationSpaceName
	}
	if r.DestinationSpaceCode != nil {
		d.DestinationSpaceCode = *r.DestinationSpaceCode
	}
	if len(r.AcceptedProof) > 0 {
		d.AcceptedProof = json.RawMessage(r.AcceptedProof)
	}
	if r.Notes != nil {
		d.Notes = *r.Notes
	}
	if r.EffectiveDate.Valid {
		d.EffectiveDate = r.EffectiveDate.Time.Format("2006-01-02")
	}
	if r.SourceURL != nil {
		d.SourceURL = *r.SourceURL
	}
	if r.SourceURLLastChecked.Valid {
		d.SourceURLLastChecked = r.SourceURLLastChecked.Time.Format("2006-01-02")
	}
	if r.HITLReviewedAt.Valid {
		d.HITLReviewedAt = r.HITLReviewedAt.Time.Format(time.RFC3339)
	}
	if r.CreatedAt.Valid {
		d.CreatedAt = r.CreatedAt.Time.Format(time.RFC3339)
	}
	if r.UpdatedAt.Valid {
		d.UpdatedAt = r.UpdatedAt.Time.Format(time.RFC3339)
	}
	return d
}

func uuidStr(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// RejectRequirement marks a pending/draft requirement rejected (HITL
// reject from the unified moderation queue).
func (s *Service) RejectRequirement(ctx context.Context, id, reviewerID pgtype.UUID, notes string) error {
	return s.store.RejectRequirement(ctx, id, reviewerID, notes)
}

// GetRequirement returns a requirement by ID regardless of status
// (admin detail view).
func (s *Service) GetRequirement(ctx context.Context, id pgtype.UUID) (RequirementDTO, error) {
	row, err := s.store.GetRequirement(ctx, id)
	if err != nil {
		return RequirementDTO{}, err
	}
	return requirementDTO(row), nil
}

// DeleteRequirement removes a non-published requirement.
func (s *Service) DeleteRequirement(ctx context.Context, id pgtype.UUID) error {
	return s.store.DeleteRequirement(ctx, id)
}

// TaskEnqueuer is the minimal interface for enqueueing Asynq tasks —
// used to fire search:index_entity on publish so the row reaches the
// pof_requirements Meilisearch index (ADR-088 is_japabot_indexed gate
// is applied inside the indexer).
type TaskEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// SetEnqueuer injects the Asynq task enqueuer for search indexing.
// Nil-safe: indexing becomes a graceful no-op.
func (s *Service) SetEnqueuer(e TaskEnqueuer) { s.enqueuer = e }

// AIDispatcher is the §20 AI Gateway seam for Content Agent drafts.
type AIDispatcher interface {
	Dispatch(ctx context.Context, category string, req AIRequest) (AIResponse, error)
}

// AIRequest / AIResponse are the provider-agnostic envelopes.
type AIRequest struct {
	SystemPrompt string
	UserPrompt   string
}

// AIResponse carries the generated text.
type AIResponse struct {
	Text  string
	Model string
}

// SetAIDispatcher injects the AI Gateway dispatcher for
// POST /admin/pof-requirements/generate (tool:generate_draft task).
func (s *Service) SetAIDispatcher(d AIDispatcher) { s.aiDispatcher = d }

// EnqueueGenerate fires tool:generate_draft for a POF requirement draft
// (fire-and-forget — the admin polls the moderation queue for the
// ai_draft row). destination_space_id is required.
func (s *Service) EnqueueGenerate(ctx context.Context, params map[string]string) error {
	if s.enqueuer == nil {
		return errors.New("pofcalculator: task enqueuer not configured")
	}
	if params["destination_space_id"] == "" {
		return errors.New("destination_space_id is required")
	}
	if _, err := uuidParam(params, "destination_space_id"); err != nil {
		return err
	}
	payload := map[string]string{"entity_type": "pof_requirement"}
	for k, v := range params {
		payload[k] = v
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.enqueuer.Enqueue("tool:generate_draft", raw)
}

// GenerateRequirementDraft implements the POF branch of the
// tool:generate_draft worker task — the AI Gateway returns a requirement
// JSON document which is validated and inserted as an ai_draft row for
// HITL review (ADR-088). Params carry the admin form hints
// (destination_space_id is required — the row must join a space).
func (s *Service) GenerateRequirementDraft(ctx context.Context, params map[string]string) error {
	if s.aiDispatcher == nil {
		return errors.New("pofcalculator: no AI dispatcher configured")
	}
	dest, err := uuidParam(params, "destination_space_id")
	if err != nil {
		return err
	}

	resp, err := s.aiDispatcher.Dispatch(ctx, "content_generation", AIRequest{
		SystemPrompt: "You are the JapaConnect Content Agent. Produce a proof-of-funds " +
			"requirement as STRICT JSON (no markdown fences): " +
			`{"visa_type":"...","program_label":"...","currency":"ISO-4217",` +
			`"amount_schedule":[{"members":1,"amount":15290,"per_additional":0}]` +
			` OR {"base":13347,"base_members":1,"per_additional":680},` +
			`"funds_must_be_liquid":true,"accepted_proof":["..."],"notes":"...",` +
			`"effective_date":"YYYY-MM-DD","source_url":""}. ` +
			"amount_schedule is either a band array (members → amount, optional " +
			"per_additional overage) or a formula object (base + per_additional).",
		UserPrompt: formatParams(params),
	})
	if err != nil {
		return fmt.Errorf("pofcalculator: generate draft: AI dispatch: %w", err)
	}

	var out struct {
		VisaType          string           `json:"visa_type"`
		ProgramLabel      string           `json:"program_label"`
		Currency          string           `json:"currency"`
		AmountSchedule    *json.RawMessage `json:"amount_schedule"`
		FundsMustBeLiquid *bool            `json:"funds_must_be_liquid"`
		AcceptedProof     *json.RawMessage `json:"accepted_proof"`
		Notes             *string          `json:"notes"`
		EffectiveDate     string           `json:"effective_date"`
		SourceURL         *string          `json:"source_url"`
	}
	if err := json.Unmarshal(extractJSON(resp.Text), &out); err != nil {
		return fmt.Errorf("pofcalculator: generate draft: unparseable AI output: %w", err)
	}
	if out.VisaType == "" || out.ProgramLabel == "" || out.Currency == "" ||
		out.AmountSchedule == nil || out.EffectiveDate == "" {
		return errors.New("pofcalculator: generate draft: AI output missing required fields")
	}
	if _, err := resolveSchedule(*out.AmountSchedule, 1); err != nil {
		return fmt.Errorf("pofcalculator: generate draft: invalid amount_schedule: %w", err)
	}
	var eff pgtype.Date
	if err := eff.Scan(out.EffectiveDate); err != nil {
		return fmt.Errorf("pofcalculator: generate draft: invalid effective_date")
	}

	row := RequirementRow{
		DestinationSpaceID: dest,
		VisaType:           out.VisaType,
		ProgramLabel:       out.ProgramLabel,
		AmountSchedule:     *out.AmountSchedule,
		Currency:           out.Currency,
		EffectiveDate:      eff,
		SourceURL:          out.SourceURL,
		Notes:              out.Notes,
		Status:             StatusAIDraft,
		AIGenerated:        true,
	}
	if out.FundsMustBeLiquid != nil {
		row.FundsMustBeLiquid = pgtype.Bool{Bool: *out.FundsMustBeLiquid, Valid: true}
	} else {
		row.FundsMustBeLiquid = pgtype.Bool{Bool: true, Valid: true}
	}
	if out.AcceptedProof != nil {
		row.AcceptedProof = *out.AcceptedProof
	}
	_, err = s.store.CreateRequirement(ctx, row)
	return err
}

// uuidParam parses a required UUID param.
func uuidParam(params map[string]string, key string) (pgtype.UUID, error) {
	var id pgtype.UUID
	v := params[key]
	if v == "" {
		return id, fmt.Errorf("pofcalculator: %s param is required", key)
	}
	if err := id.Scan(v); err != nil {
		return id, fmt.Errorf("pofcalculator: invalid %s", key)
	}
	return id, nil
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
		return "Generate the requirement."
	}
	var b strings.Builder
	b.WriteString("Generate a proof-of-funds requirement with these hints:\n")
	for k, v := range params {
		fmt.Fprintf(&b, "- %s: %s\n", k, v)
	}
	return b.String()
}

// enqueueIndex fires search:index_entity for a requirement (Rule 8 —
// fire-and-forget; enqueue failure is logged, not returned).
func (s *Service) enqueueIndex(id pgtype.UUID) {
	if s.enqueuer == nil {
		return
	}
	payload, err := json.Marshal(map[string]string{
		"entity_type": "pof_requirement",
		"entity_id":   id.String(),
	})
	if err != nil {
		slog.Error("pofcalculator: marshal index payload", "error", err)
		return
	}
	if err := s.enqueuer.Enqueue("search:index_entity", payload); err != nil {
		slog.Error("pofcalculator: enqueue index task", "id", id.String(), "error", err)
	}
}
