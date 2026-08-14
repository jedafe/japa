// Package visatimeline — service.go orchestrates timeline creation, milestone
// editing, ownership-gated updates/deletes, public statistics, and admin
// moderation. Feature-flagged via visa_timeline.enabled system_setting (§83).
//
// Ref: japaconnect-engineering/sections/83-visa-timeline-tracker.md
//
//	japaconnect-engineering/adr/ADR-091-visa-timeline-tracker.md
package visatimeline

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
)

// TimelineDTO is the API response shape for a single timeline.
type TimelineDTO struct {
	ID                 string         `json:"id"`
	UserID             string         `json:"user_id"`
	OriginSpaceID      string         `json:"origin_space_id,omitempty"`
	DestinationSpaceID string         `json:"destination_space_id"`
	VisaCategory       string         `json:"visa_category"`
	Consulate          string         `json:"consulate,omitempty"`
	ApplicationMethod  string         `json:"application_method,omitempty"`
	Outcome            string         `json:"outcome,omitempty"`
	IsShared           bool           `json:"is_shared"`
	AnonymisedAt       string         `json:"anonymised_at,omitempty"`
	CreatedAt          string         `json:"created_at"`
	UpdatedAt          string         `json:"updated_at"`
	Milestones         []MilestoneDTO `json:"milestones,omitempty"`
}

// MilestoneDTO is the API response shape for a single milestone.
type MilestoneDTO struct {
	ID            string `json:"id"`
	TimelineID    string `json:"timeline_id"`
	MilestoneType string `json:"milestone_type"`
	OccurredAt    string `json:"occurred_at"`
	Notes         string `json:"notes,omitempty"`
	CreatedAt     string `json:"created_at"`
}

// ProcessingStatDTO is the API response shape for an aggregated stat row.
type ProcessingStatDTO struct {
	OriginSpaceID      string  `json:"origin_space_id,omitempty"`
	DestinationSpaceID string  `json:"destination_space_id"`
	VisaCategory       string  `json:"visa_category"`
	Consulate          string  `json:"consulate,omitempty"`
	PeriodStart        string  `json:"period_start"`
	SampleCount        int32   `json:"sample_count"`
	AvgDays            float64 `json:"avg_days,omitempty"`
	MedianDays         float64 `json:"median_days,omitempty"`
	P10Days            float64 `json:"p10_days,omitempty"`
	P90Days            float64 `json:"p90_days,omitempty"`
	RefreshedAt        string  `json:"refreshed_at"`
}

// TrendPointDTO is one month in the monthly trend series.
type TrendPointDTO struct {
	PeriodStart string  `json:"period_start"`
	SampleCount int32   `json:"sample_count"`
	AvgDays     float64 `json:"avg_days,omitempty"`
	MedianDays  float64 `json:"median_days,omitempty"`
}

// CreateTimelineInput is the request body for POST /me/visa-timelines.
type CreateTimelineInput struct {
	OriginSpaceID      string `json:"origin_space_id"`
	DestinationSpaceID string `json:"destination_space_id"`
	VisaCategory       string `json:"visa_category"`
	Consulate          string `json:"consulate"`
	ApplicationMethod  string `json:"application_method"`
	Outcome            string `json:"outcome"`
	IsShared           bool   `json:"is_shared"`
}

// UpdateTimelineInput is the request body for PATCH /me/visa-timelines/:id.
type UpdateTimelineInput struct {
	Outcome           *string `json:"outcome"`
	IsShared          *bool   `json:"is_shared"`
	Consulate         *string `json:"consulate"`
	ApplicationMethod *string `json:"application_method"`
}

// AddMilestoneInput is the request body for POST /me/visa-timelines/:id/milestones.
type AddMilestoneInput struct {
	MilestoneType string `json:"milestone_type"`
	OccurredAt    string `json:"occurred_at"` // RFC 3339
	Notes         string `json:"notes"`
}

// UpdateMilestoneInput is the request body for PATCH /me/visa-timelines/:id/milestones/:mid.
type UpdateMilestoneInput struct {
	MilestoneType *string `json:"milestone_type"`
	OccurredAt    *string `json:"occurred_at"` // RFC 3339
	Notes         *string `json:"notes"`
}

// Service holds the business logic for the Visa Timeline Tracker.
type Service struct {
	store    *Store
	settings *config.Settings
}

// NewService creates a visa-timeline service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{store: store, settings: settings}
}

// Enabled reads the visa_timeline.enabled system_setting toggle (§83 feature
// flag).
func (s *Service) Enabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return false
	}
	return s.settings.GetBoolOr(ctx, tenantID, "visa_timeline.enabled", false)
}

// Create records a new visa timeline for the authenticated user.
func (s *Service) Create(ctx context.Context, tenantID, userID pgtype.UUID, input CreateTimelineInput) (TimelineDTO, error) {
	if input.DestinationSpaceID == "" || input.VisaCategory == "" {
		return TimelineDTO{}, fmt.Errorf("destination_space_id and visa_category are required")
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
		return TimelineDTO{}, fmt.Errorf("invalid destination_space_id: %w", err)
	}

	var originSpaceID pgtype.UUID
	if input.OriginSpaceID != "" {
		if err := originSpaceID.Scan(input.OriginSpaceID); err != nil {
			return TimelineDTO{}, fmt.Errorf("invalid origin_space_id: %w", err)
		}
	}

	t := Timeline{
		TenantID:           tenantID,
		UserID:             userID,
		OriginSpaceID:      originSpaceID,
		DestinationSpaceID: destSpaceID,
		VisaCategory:       input.VisaCategory,
		IsShared:           input.IsShared,
	}
	if input.Consulate != "" {
		t.Consulate = &input.Consulate
	}
	if input.ApplicationMethod != "" {
		t.ApplicationMethod = &input.ApplicationMethod
	}
	if input.Outcome != "" {
		t.Outcome = &input.Outcome
	}

	created, err := s.store.CreateTimeline(ctx, t)
	if err != nil {
		return TimelineDTO{}, fmt.Errorf("visatimeline: create: %w", err)
	}
	return timelineToDTO(created), nil
}

// Get returns a single timeline by id. The caller is responsible for any
// share-visibility enforcement (public stats use the aggregated table, not
// individual timelines).
func (s *Service) Get(ctx context.Context, id, tenantID pgtype.UUID) (TimelineDTO, error) {
	t, err := s.store.GetTimeline(ctx, id, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return TimelineDTO{}, pgx.ErrNoRows
		}
		return TimelineDTO{}, fmt.Errorf("visatimeline: get: %w", err)
	}
	return timelineToDTO(t), nil
}

// ListOwn returns the authenticated user's own timelines.
func (s *Service) ListOwn(ctx context.Context, userID, tenantID pgtype.UUID) ([]TimelineDTO, error) {
	timelines, err := s.store.ListUserTimelines(ctx, userID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("visatimeline: list own: %w", err)
	}
	dtos := make([]TimelineDTO, 0, len(timelines))
	for _, t := range timelines {
		dtos = append(dtos, timelineToDTO(t))
	}
	return dtos, nil
}

// UpdateOwn updates the authenticated user's own timeline (outcome / share
// toggle). Ownership is verified against the stored user_id.
func (s *Service) UpdateOwn(ctx context.Context, id, tenantID, userID pgtype.UUID, input UpdateTimelineInput) (TimelineDTO, error) {
	existing, err := s.store.GetTimeline(ctx, id, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return TimelineDTO{}, pgx.ErrNoRows
		}
		return TimelineDTO{}, fmt.Errorf("visatimeline: update: get: %w", err)
	}
	if existing.UserID != userID {
		return TimelineDTO{}, ErrNotOwner
	}

	storeInput := StoreUpdateTimelineInput{
		Outcome:           input.Outcome,
		IsShared:          input.IsShared,
		Consulate:         input.Consulate,
		ApplicationMethod: input.ApplicationMethod,
	}
	if err := s.store.UpdateTimeline(ctx, id, tenantID, storeInput); err != nil {
		if err == pgx.ErrNoRows {
			return TimelineDTO{}, pgx.ErrNoRows
		}
		return TimelineDTO{}, fmt.Errorf("visatimeline: update: %w", err)
	}

	return s.Get(ctx, id, tenantID)
}

// DeleteOwn soft-deletes the authenticated user's own timeline. Ownership is
// verified against the stored user_id.
func (s *Service) DeleteOwn(ctx context.Context, id, tenantID, userID pgtype.UUID) error {
	existing, err := s.store.GetTimeline(ctx, id, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return pgx.ErrNoRows
		}
		return fmt.Errorf("visatimeline: delete: get: %w", err)
	}
	if existing.UserID != userID {
		return ErrNotOwner
	}

	if err := s.store.DeleteTimeline(ctx, id, tenantID); err != nil {
		if err == pgx.ErrNoRows {
			return pgx.ErrNoRows
		}
		return fmt.Errorf("visatimeline: delete: %w", err)
	}
	return nil
}

// milestoneOrder defines the canonical chronological order of visa
// application milestones (§83). A milestone that violates this order
// relative to existing milestones is flagged as implausible.
var milestoneOrder = map[string]int{
	"applied":        1,
	"biometrics":     2,
	"medical":        3,
	"docs_requested": 4,
	"decision":       5,
	"visa_issued":    6,
}

// ErrImplausibleSequence is returned when a new milestone's type or date
// violates the canonical milestone order relative to existing milestones
// (§83 Privacy & Anti-Gaming: implausible sequences are flagged).
var ErrImplausibleSequence = fmt.Errorf("implausible milestone sequence")

// AddMilestone appends a milestone to a timeline owned by the authenticated
// user. Ownership is verified against the stored user_id. Implausible
// milestone sequences (e.g. decision before applied, or a date that precedes
// an earlier-stage milestone) are rejected per §83 anti-gaming rules.
func (s *Service) AddMilestone(ctx context.Context, timelineID, tenantID, userID pgtype.UUID, input AddMilestoneInput) (MilestoneDTO, error) {
	if input.MilestoneType == "" {
		return MilestoneDTO{}, fmt.Errorf("milestone_type is required")
	}

	existing, err := s.store.GetTimeline(ctx, timelineID, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return MilestoneDTO{}, pgx.ErrNoRows
		}
		return MilestoneDTO{}, fmt.Errorf("visatimeline: add milestone: get: %w", err)
	}
	if existing.UserID != userID {
		return MilestoneDTO{}, ErrNotOwner
	}

	var occurredAt pgtype.Timestamptz
	if input.OccurredAt != "" {
		if err := occurredAt.Scan(input.OccurredAt); err != nil {
			return MilestoneDTO{}, fmt.Errorf("invalid occurred_at: %w", err)
		}
	}

	// Implausible-sequence check (§83 Privacy & Anti-Gaming). Fetch
	// existing milestones and verify the new milestone's type-order and
	// date are consistent with the canonical order. A milestone is
	// implausible if:
	//   - a later-stage milestone already exists with an earlier date, OR
	//   - an earlier-stage milestone already exists with a later date.
	existingMilestones, err := s.store.ListMilestones(ctx, timelineID, tenantID)
	if err != nil {
		return MilestoneDTO{}, fmt.Errorf("visatimeline: add milestone: list existing: %w", err)
	}
	newOrder, newOrderOk := milestoneOrder[input.MilestoneType]
	if newOrderOk && occurredAt.Valid {
		for _, em := range existingMilestones {
			if !em.OccurredAt.Valid {
				continue
			}
			emOrder, emOrderOk := milestoneOrder[em.MilestoneType]
			if !emOrderOk {
				continue
			}
			// New milestone is later-stage but dated before an
			// earlier-stage existing milestone → implausible.
			if newOrder > emOrder && occurredAt.Time.Before(em.OccurredAt.Time) {
				return MilestoneDTO{}, ErrImplausibleSequence
			}
			// New milestone is earlier-stage but dated after a
			// later-stage existing milestone → implausible.
			if newOrder < emOrder && occurredAt.Time.After(em.OccurredAt.Time) {
				return MilestoneDTO{}, ErrImplausibleSequence
			}
		}
	}

	var notesPtr *string
	if input.Notes != "" {
		notesPtr = &input.Notes
	}

	m, err := s.store.AddMilestone(ctx, timelineID, tenantID, input.MilestoneType, occurredAt, notesPtr)
	if err != nil {
		return MilestoneDTO{}, fmt.Errorf("visatimeline: add milestone: %w", err)
	}
	return milestoneToDTO(m), nil
}

// UpdateMilestone edits a milestone belonging to a timeline owned by the
// authenticated user. Ownership is verified via the parent timeline.
func (s *Service) UpdateMilestone(ctx context.Context, milestoneID, timelineID, tenantID, userID pgtype.UUID, input UpdateMilestoneInput) error {
	existing, err := s.store.GetTimeline(ctx, timelineID, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return pgx.ErrNoRows
		}
		return fmt.Errorf("visatimeline: update milestone: get: %w", err)
	}
	if existing.UserID != userID {
		return ErrNotOwner
	}

	storeInput := StoreUpdateMilestoneInput{
		MilestoneType: input.MilestoneType,
		Notes:         input.Notes,
	}
	if input.OccurredAt != nil {
		var ts pgtype.Timestamptz
		if *input.OccurredAt != "" {
			if err := ts.Scan(*input.OccurredAt); err != nil {
				return fmt.Errorf("invalid occurred_at: %w", err)
			}
		}
		storeInput.OccurredAt = &ts
	}

	if err := s.store.UpdateMilestone(ctx, milestoneID, tenantID, storeInput); err != nil {
		if err == pgx.ErrNoRows {
			return pgx.ErrNoRows
		}
		return fmt.Errorf("visatimeline: update milestone: %w", err)
	}
	return nil
}

// ListMilestones returns the milestones for a timeline. Ownership is not
// required (the caller may list milestones for their own timeline).
func (s *Service) ListMilestones(ctx context.Context, timelineID, tenantID pgtype.UUID) ([]MilestoneDTO, error) {
	milestones, err := s.store.ListMilestones(ctx, timelineID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("visatimeline: list milestones: %w", err)
	}
	dtos := make([]MilestoneDTO, 0, len(milestones))
	for _, m := range milestones {
		dtos = append(dtos, milestoneToDTO(m))
	}
	return dtos, nil
}

// GetStats returns the public aggregated processing-time statistics for an
// origin → destination → visa category combination. Consulate and months
// are optional filters (§83 route signature: ?dest=&origin=&visa=&consulate=&months=).
func (s *Service) GetStats(ctx context.Context, tenantID, originSpaceID, destSpaceID pgtype.UUID, visaCategory, consulate string, months int) ([]ProcessingStatDTO, error) {
	stats, err := s.store.GetStats(ctx, tenantID, originSpaceID, destSpaceID, visaCategory, consulate, months)
	if err != nil {
		return nil, fmt.Errorf("visatimeline: get stats: %w", err)
	}
	dtos := make([]ProcessingStatDTO, 0, len(stats))
	for _, st := range stats {
		dtos = append(dtos, statToDTO(st))
	}
	return dtos, nil
}

// GetTrend returns the monthly trend series for a destination space + visa
// category over the last `months` months. Consulate is an optional filter.
func (s *Service) GetTrend(ctx context.Context, tenantID, destSpaceID pgtype.UUID, visaCategory, consulate string, months int) ([]TrendPointDTO, error) {
	points, err := s.store.GetTrend(ctx, tenantID, destSpaceID, visaCategory, consulate, months)
	if err != nil {
		return nil, fmt.Errorf("visatimeline: get trend: %w", err)
	}
	dtos := make([]TrendPointDTO, 0, len(points))
	for _, p := range points {
		dtos = append(dtos, trendToDTO(p))
	}
	return dtos, nil
}

// AdminList returns all timelines (any share state) for the moderation queue.
func (s *Service) AdminList(ctx context.Context, tenantID pgtype.UUID, filter AdminListFilter) ([]TimelineDTO, int64, error) {
	filter.TenantID = tenantID
	timelines, total, err := s.store.AdminList(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("visatimeline: admin list: %w", err)
	}
	dtos := make([]TimelineDTO, 0, len(timelines))
	for _, t := range timelines {
		dtos = append(dtos, timelineToDTO(t))
	}
	return dtos, total, nil
}

// AdminDelete soft-deletes any timeline (admin override, visa_timeline.manage_any
// permission checked at route level).
func (s *Service) AdminDelete(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.store.DeleteTimeline(ctx, id, tenantID); err != nil {
		if err == pgx.ErrNoRows {
			return pgx.ErrNoRows
		}
		return fmt.Errorf("visatimeline: admin delete: %w", err)
	}
	return nil
}

// ErrNotOwner is returned when a user tries to modify a timeline they did not
// create.
var ErrNotOwner = fmt.Errorf("not the timeline owner")

// timelineToDTO converts a Timeline row to a TimelineDTO. When the Timeline
// has joined Milestones (from ListUserTimelines / AdminList), they are
// mapped to MilestoneDTO so list endpoints return timelines with their
// milestones in a single round-trip (§83 — no separate list-milestones
// route).
func timelineToDTO(t Timeline) TimelineDTO {
	dto := TimelineDTO{
		ID:                 t.ID.String(),
		UserID:             t.UserID.String(),
		DestinationSpaceID: t.DestinationSpaceID.String(),
		VisaCategory:       t.VisaCategory,
		IsShared:           t.IsShared,
		AnonymisedAt:       formatTs(t.AnonymisedAt),
		CreatedAt:          formatTs(t.CreatedAt),
		UpdatedAt:          formatTs(t.UpdatedAt),
	}
	if t.OriginSpaceID.Valid {
		dto.OriginSpaceID = t.OriginSpaceID.String()
	}
	if t.Consulate != nil {
		dto.Consulate = *t.Consulate
	}
	if t.ApplicationMethod != nil {
		dto.ApplicationMethod = *t.ApplicationMethod
	}
	if t.Outcome != nil {
		dto.Outcome = *t.Outcome
	}
	if len(t.Milestones) > 0 {
		dto.Milestones = make([]MilestoneDTO, 0, len(t.Milestones))
		for _, m := range t.Milestones {
			dto.Milestones = append(dto.Milestones, milestoneToDTO(m))
		}
	}
	return dto
}

// milestoneToDTO converts a Milestone row to a MilestoneDTO.
func milestoneToDTO(m Milestone) MilestoneDTO {
	dto := MilestoneDTO{
		ID:            m.ID.String(),
		TimelineID:    m.TimelineID.String(),
		MilestoneType: m.MilestoneType,
		OccurredAt:    formatTs(m.OccurredAt),
		CreatedAt:     formatTs(m.CreatedAt),
	}
	if m.Notes != nil {
		dto.Notes = *m.Notes
	}
	return dto
}

// statToDTO converts a ProcessingStat row to a ProcessingStatDTO.
func statToDTO(st ProcessingStat) ProcessingStatDTO {
	dto := ProcessingStatDTO{
		DestinationSpaceID: st.DestinationSpaceID.String(),
		VisaCategory:       st.VisaCategory,
		SampleCount:        st.SampleCount,
		RefreshedAt:        formatTs(st.RefreshedAt),
	}
	if st.OriginSpaceID.Valid {
		dto.OriginSpaceID = st.OriginSpaceID.String()
	}
	if st.Consulate != nil {
		dto.Consulate = *st.Consulate
	}
	dto.PeriodStart = formatTs(st.PeriodStart)
	if st.AvgDays != nil {
		dto.AvgDays = *st.AvgDays
	}
	if st.MedianDays != nil {
		dto.MedianDays = *st.MedianDays
	}
	if st.P10Days != nil {
		dto.P10Days = *st.P10Days
	}
	if st.P90Days != nil {
		dto.P90Days = *st.P90Days
	}
	return dto
}

// trendToDTO converts a TrendPoint to a TrendPointDTO.
func trendToDTO(p TrendPoint) TrendPointDTO {
	dto := TrendPointDTO{
		PeriodStart: formatTs(p.PeriodStart),
		SampleCount: p.SampleCount,
	}
	if p.AvgDays != nil {
		dto.AvgDays = *p.AvgDays
	}
	if p.MedianDays != nil {
		dto.MedianDays = *p.MedianDays
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

// RefreshStats recomputes the visa_processing_stats aggregate table from
// shared, non-deleted visa_timelines and their milestones. Called nightly
// by the visa_timeline:refresh_stats Asynq task (§83, ADR-091).
// Implements worker.VisaTimelineProcessor.
func (s *Service) RefreshStats(ctx context.Context) (int64, error) {
	n, err := s.store.RefreshStats(ctx)
	if err != nil {
		return 0, fmt.Errorf("visatimeline: refresh stats: %w", err)
	}
	return n, nil
}

// PromptUpdate sends a weekly nudge to members with in-progress visa
// timelines (no decision/visa_issued milestone). Called weekly by the
// visa_timeline:prompt_update Asynq task (§83, ADR-091).
// Implements worker.VisaTimelineProcessor.
//
// The actual notification dispatch is delegated to the notification service
// when wired; until then the count is logged so the task is not a silent
// no-op (mirroring the planning_group:checkin_reminder pattern).
func (s *Service) PromptUpdate(ctx context.Context) (int64, error) {
	owners, err := s.store.InProgressTimelineOwners(ctx)
	if err != nil {
		return 0, fmt.Errorf("visatimeline: prompt update: %w", err)
	}
	if len(owners) > 0 {
		slog.InfoContext(ctx, "visatimeline: prompt update queued",
			"count", len(owners))
	}
	return int64(len(owners)), nil
}
