// Package ecaguide — service.go orchestrates ECA guide lookup, provider
// comparison, community updates, and admin HITL workflow for §109.
// // Ref: japaconnect-engineering/sections/109-eca-guide.md
// //	japaconnect-engineering/adr/ADR-120-eca-guide.md
package ecaguide

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
)

// ProviderDTO is the API shape for an ECA provider.
type ProviderDTO struct {
	ID      string `json:"id"`
	Slug    string `json:"slug"`
	Name    string `json:"name"`
	Website string `json:"website,omitempty"`
}

// GuideDTO is the API shape for an ECA guide.
type GuideDTO struct {
	ID                   string          `json:"id"`
	DestinationSpaceID   string          `json:"destination_space_id"`
	DestinationSpaceName string          `json:"destination_space_name,omitempty"`
	ProviderID           string          `json:"provider_id"`
	ProviderSlug         string          `json:"provider_slug,omitempty"`
	ProviderName         string          `json:"provider_name,omitempty"`
	Purpose              string          `json:"purpose"`
	PathwaySteps         json.RawMessage `json:"pathway_steps"`
	FeeEstimate          string          `json:"fee_estimate,omitempty"`
	Currency             string          `json:"currency,omitempty"`
	TimelineEstimate     string          `json:"timeline_estimate,omitempty"`
	DocumentsRequired    json.RawMessage `json:"documents_required,omitempty"`
	AcceptedFor          json.RawMessage `json:"accepted_for,omitempty"`
	OfficialURL          string          `json:"official_url,omitempty"`
	SourceURLLastChecked string          `json:"source_url_last_checked,omitempty"`
	Status               string          `json:"status"`
	IsActive             bool            `json:"is_active"`
	AIGenerated          bool            `json:"ai_generated"`
	IsJapabotIndexed     bool            `json:"is_japabot_indexed"`
}

// UpdateDTO is the API shape for a community update.
type UpdateDTO struct {
	ID             string `json:"id"`
	GuideID        string `json:"guide_id"`
	UserName       string `json:"user_name,omitempty"`
	StepIndex      *int32 `json:"step_index,omitempty"`
	FeeEstimate    string `json:"fee_estimate,omitempty"`
	TurnaroundDays *int32 `json:"turnaround_days,omitempty"`
	Note           string `json:"note,omitempty"`
	Status         string `json:"status"`
	CreatedAt      string `json:"created_at"`
}

// GuideDetailDTO is GET /eca/guides/:id — guide + approved updates.
type GuideDetailDTO struct {
	Guide   GuideDTO    `json:"guide"`
	Updates []UpdateDTO `json:"updates"`
}

// GuideRequest is the admin create/update body.
type GuideRequest struct {
	DestinationSpaceID string           `json:"destination_space_id"`
	ProviderID         string           `json:"provider_id"`
	Purpose            string           `json:"purpose"`
	PathwaySteps       *json.RawMessage `json:"pathway_steps"`
	FeeEstimate        *string          `json:"fee_estimate"`
	Currency           *string          `json:"currency"`
	TimelineEstimate   *string          `json:"timeline_estimate"`
	DocumentsRequired  *json.RawMessage `json:"documents_required"`
	AcceptedFor        *json.RawMessage `json:"accepted_for"`
	OfficialURL        *string          `json:"official_url"`
	Status             string           `json:"status"`
	IsActive           *bool            `json:"is_active"`
}

// UpdateRequest is the community-update submission body.
type UpdateRequest struct {
	StepIndex      *int32 `json:"step_index"`
	FeeEstimate    string `json:"fee_estimate"`
	TurnaroundDays *int32 `json:"turnaround_days"`
	Note           string `json:"note"`
}

// ModerateRequest is the admin moderation body.
type ModerateRequest struct {
	Status string `json:"status"` // approved|rejected
}

// Service orchestrates the ECA guide.
type Service struct {
	store    *Store
	settings *config.Settings
}

// NewService creates the service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{store: store, settings: settings}
}

// ListProviders returns active providers, optionally destination-scoped.
func (s *Service) ListProviders(ctx context.Context, destSpaceID pgtype.UUID) ([]ProviderDTO, error) {
	rows, err := s.store.ListProviders(ctx, destSpaceID)
	if err != nil {
		return nil, err
	}
	out := make([]ProviderDTO, 0, len(rows))
	for _, p := range rows {
		d := ProviderDTO{ID: uuidStr(p.ID), Slug: p.Slug, Name: p.Name}
		if p.Website != nil {
			d.Website = *p.Website
		}
		out = append(out, d)
	}
	return out, nil
}

// ListGuides returns published guides.
func (s *Service) ListGuides(ctx context.Context, destSpaceID pgtype.UUID, purpose, provider string) ([]GuideDTO, error) {
	rows, err := s.store.ListGuides(ctx, destSpaceID, purpose, provider)
	if err != nil {
		return nil, err
	}
	return guideDTOs(rows), nil
}

// GetGuide returns a guide detail plus approved community updates.
func (s *Service) GetGuide(ctx context.Context, id pgtype.UUID) (*GuideDetailDTO, error) {
	g, err := s.store.GetGuide(ctx, id)
	if err != nil {
		return nil, err
	}
	updates, err := s.store.ListApprovedUpdates(ctx, id)
	if err != nil {
		return nil, err
	}
	return &GuideDetailDTO{Guide: guideDTO(g), Updates: updateDTOs(updates)}, nil
}

// CompareGuides returns published guides side-by-side per provider.
func (s *Service) CompareGuides(ctx context.Context, destSpaceID pgtype.UUID, providerIDs []pgtype.UUID) ([]GuideDTO, error) {
	if len(providerIDs) == 0 {
		return nil, errors.New("provider_ids is required")
	}
	if len(providerIDs) > 8 {
		return nil, errors.New("provider_ids: max 8 providers")
	}
	rows, err := s.store.CompareGuides(ctx, destSpaceID, providerIDs)
	if err != nil {
		return nil, err
	}
	return guideDTOs(rows), nil
}

// CreateCommunityUpdate queues a member-submitted fee/timeline report.
func (s *Service) CreateCommunityUpdate(ctx context.Context, guideID, userID pgtype.UUID, in UpdateRequest) (string, error) {
	if in.FeeEstimate == "" && in.TurnaroundDays == nil && in.Note == "" {
		return "", errors.New("at least one of fee_estimate, turnaround_days or note is required")
	}
	id, err := s.store.CreateCommunityUpdate(ctx, guideID, userID, in.StepIndex, in.FeeEstimate, in.TurnaroundDays, in.Note)
	if err != nil {
		return "", err
	}
	return uuidStr(id), nil
}

// AdminListGuides returns guides in any status (admin).
func (s *Service) AdminListGuides(ctx context.Context, destSpaceID pgtype.UUID, status string, page, limit int32) ([]GuideDTO, int64, error) {
	rows, total, err := s.store.AdminListGuides(ctx, destSpaceID, status, page, limit)
	if err != nil {
		return nil, 0, err
	}
	return guideDTOs(rows), total, nil
}

// CreateGuide inserts a guide (admin).
func (s *Service) CreateGuide(ctx context.Context, in GuideRequest) (*GuideDTO, error) {
	if in.DestinationSpaceID == "" || in.ProviderID == "" || in.Purpose == "" || in.PathwaySteps == nil {
		return nil, errors.New("destination_space_id, provider_id, purpose and pathway_steps are required")
	}
	row := GuideRow{
		Purpose: in.Purpose,
		Status:  orDefault(in.Status, StatusDraft),
	}
	if err := row.DestinationSpaceID.Scan(in.DestinationSpaceID); err != nil {
		return nil, fmt.Errorf("invalid destination_space_id")
	}
	if err := row.ProviderID.Scan(in.ProviderID); err != nil {
		return nil, fmt.Errorf("invalid provider_id")
	}
	row.PathwaySteps = *in.PathwaySteps
	if in.DocumentsRequired != nil {
		row.DocumentsRequired = *in.DocumentsRequired
	}
	if in.AcceptedFor != nil {
		row.AcceptedFor = *in.AcceptedFor
	}
	row.FeeEstimate = in.FeeEstimate
	row.Currency = in.Currency
	row.TimelineEstimate = in.TimelineEstimate
	row.OfficialURL = in.OfficialURL

	id, err := s.store.CreateGuide(ctx, row)
	if err != nil {
		return nil, err
	}
	dto := guideDTO(row)
	dto.ID = uuidStr(id)
	return &dto, nil
}

// UpdateGuide applies a partial update (admin).
func (s *Service) UpdateGuide(ctx context.Context, id pgtype.UUID, in GuideRequest) error {
	row := GuideRow{
		Purpose:          in.Purpose,
		FeeEstimate:      in.FeeEstimate,
		Currency:         in.Currency,
		TimelineEstimate: in.TimelineEstimate,
		OfficialURL:      in.OfficialURL,
		Status:           in.Status,
	}
	if in.PathwaySteps != nil {
		row.PathwaySteps = *in.PathwaySteps
	}
	if in.DocumentsRequired != nil {
		row.DocumentsRequired = *in.DocumentsRequired
	}
	if in.AcceptedFor != nil {
		row.AcceptedFor = *in.AcceptedFor
	}
	if in.IsActive != nil {
		row.IsActive = pgtype.Bool{Bool: *in.IsActive, Valid: true}
	}
	return s.store.UpdateGuide(ctx, id, row)
}

// ApproveGuide publishes a guide after HITL review.
func (s *Service) ApproveGuide(ctx context.Context, id, reviewerID pgtype.UUID, notes string) error {
	return s.store.ApproveGuide(ctx, id, reviewerID, notes)
}

// AdminListUpdates returns community updates for moderation.
func (s *Service) AdminListUpdates(ctx context.Context, status string, page, limit int32) ([]UpdateDTO, int64, error) {
	rows, total, err := s.store.AdminListUpdates(ctx, status, page, limit)
	if err != nil {
		return nil, 0, err
	}
	return updateDTOs(rows), total, nil
}

// ModerateUpdate approves or rejects a community update.
func (s *Service) ModerateUpdate(ctx context.Context, id, reviewerID pgtype.UUID, status string) error {
	if status != StatusApproved && status != StatusRejected {
		return errors.New("status must be approved or rejected")
	}
	return s.store.ModerateUpdate(ctx, id, reviewerID, status)
}

// ---------------------------------------------------------------------
// DTO helpers
// ---------------------------------------------------------------------

func guideDTOs(rows []GuideRow) []GuideDTO {
	out := make([]GuideDTO, 0, len(rows))
	for _, g := range rows {
		out = append(out, guideDTO(g))
	}
	return out
}

func guideDTO(g GuideRow) GuideDTO {
	d := GuideDTO{
		ID:                 uuidStr(g.ID),
		DestinationSpaceID: uuidStr(g.DestinationSpaceID),
		ProviderID:         uuidStr(g.ProviderID),
		Purpose:            g.Purpose,
		PathwaySteps:       json.RawMessage(g.PathwaySteps),
		Status:             g.Status,
		IsActive:           g.IsActive.Bool,
		AIGenerated:        g.AIGenerated,
		IsJapabotIndexed:   g.IsJapabotIndexed,
	}
	if g.DestinationSpaceName != nil {
		d.DestinationSpaceName = *g.DestinationSpaceName
	}
	if g.ProviderSlug != nil {
		d.ProviderSlug = *g.ProviderSlug
	}
	if g.ProviderName != nil {
		d.ProviderName = *g.ProviderName
	}
	if g.FeeEstimate != nil {
		d.FeeEstimate = *g.FeeEstimate
	}
	if g.Currency != nil {
		d.Currency = *g.Currency
	}
	if g.TimelineEstimate != nil {
		d.TimelineEstimate = *g.TimelineEstimate
	}
	if len(g.DocumentsRequired) > 0 {
		d.DocumentsRequired = json.RawMessage(g.DocumentsRequired)
	}
	if len(g.AcceptedFor) > 0 {
		d.AcceptedFor = json.RawMessage(g.AcceptedFor)
	}
	if g.OfficialURL != nil {
		d.OfficialURL = *g.OfficialURL
	}
	if g.SourceURLLastChecked.Valid {
		d.SourceURLLastChecked = g.SourceURLLastChecked.Time.Format("2006-01-02")
	}
	return d
}

func updateDTOs(rows []UpdateRow) []UpdateDTO {
	out := make([]UpdateDTO, 0, len(rows))
	for _, u := range rows {
		d := UpdateDTO{
			ID:      uuidStr(u.ID),
			GuideID: uuidStr(u.GuideID),
			Status:  u.Status,
		}
		if u.UserName != nil {
			d.UserName = *u.UserName
		}
		if u.StepIndex.Valid {
			v := u.StepIndex.Int32
			d.StepIndex = &v
		}
		if u.FeeEstimate != nil {
			d.FeeEstimate = *u.FeeEstimate
		}
		if u.TurnaroundDays.Valid {
			v := u.TurnaroundDays.Int32
			d.TurnaroundDays = &v
		}
		if u.Note != nil {
			d.Note = *u.Note
		}
		if u.CreatedAt.Valid {
			d.CreatedAt = u.CreatedAt.Time.Format(time.RFC3339)
		}
		out = append(out, d)
	}
	return out
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func uuidStr(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// RejectGuide marks a pending/draft guide rejected (HITL reject from the
// unified moderation queue).
func (s *Service) RejectGuide(ctx context.Context, id, reviewerID pgtype.UUID, notes string) error {
	return s.store.RejectGuide(ctx, id, reviewerID, notes)
}
