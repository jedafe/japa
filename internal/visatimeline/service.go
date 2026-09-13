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
	"errors"
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
	ID                 string  `json:"id"`
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

// Notifier is the interface used to dispatch visa-timeline notifications
// (the weekly visa_timeline:prompt_update nudge, §83). Satisfied by
// *notifications.Service; declared here to avoid an import cycle. Mirrors
// the planninggroups.Notifier pattern (§82).
// Ref: japaconnect-engineering/sections/83-visa-timeline-tracker.md.
type Notifier interface {
	CreateNotification(ctx context.Context, tenantID, recipientID pgtype.UUID, nType, actorType string, actorID pgtype.UUID, subjectType string, subjectID pgtype.UUID, title, body, actionURL string) error
}

// Service holds the business logic for the Visa Timeline Tracker.
type Service struct {
	store    *Store
	settings *config.Settings
	notifier Notifier
}

// NewService creates a visa-timeline service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{store: store, settings: settings}
}

// WithNotifier attaches the notification dispatcher used by the weekly
// visa_timeline:prompt_update nudge (§83). Mirrors the
// planninggroups.WithNotifier pattern (§82).
func (s *Service) WithNotifier(n Notifier) *Service { s.notifier = n; return s }

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
		return TimelineDTO{}, errors.New("destination_space_id and visa_category are required")
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
		if errors.Is(err, pgx.ErrNoRows) {
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
		if errors.Is(err, pgx.ErrNoRows) {
			return TimelineDTO{}, pgx.ErrNoRows
		}

		return TimelineDTO{}, fmt.Errorf("visatimeline: update: get: %w", err)
	}

	if existing.UserID != userID {
		return TimelineDTO{}, ErrNotOwner
	}

	storeInput := StoreUpdateTimelineInput(input)
	if err := s.store.UpdateTimeline(ctx, id, tenantID, storeInput); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
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
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("visatimeline: delete: get: %w", err)
	}

	if existing.UserID != userID {
		return ErrNotOwner
	}

	if err := s.store.DeleteTimeline(ctx, id, tenantID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
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
var ErrImplausibleSequence = errors.New("implausible milestone sequence")

// AddMilestone appends a milestone to a timeline owned by the authenticated
// user. Ownership is verified against the stored user_id. Implausible
// milestone sequences (e.g. decision before applied, or a date that precedes
// an earlier-stage milestone) are rejected per §83 anti-gaming rules.
func (s *Service) AddMilestone(ctx context.Context, timelineID, tenantID, userID pgtype.UUID, input AddMilestoneInput) (MilestoneDTO, error) {
	if input.MilestoneType == "" {
		return MilestoneDTO{}, errors.New("milestone_type is required")
	}

	existing, err := s.store.GetTimeline(ctx, timelineID, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
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
				s.flagImplausible(ctx, tenantID, userID, timelineID, input.MilestoneType, occurredAt)
				return MilestoneDTO{}, ErrImplausibleSequence
			}
			// New milestone is earlier-stage but dated after a
			// later-stage existing milestone → implausible.
			if newOrder < emOrder && occurredAt.Time.After(em.OccurredAt.Time) {
				s.flagImplausible(ctx, tenantID, userID, timelineID, input.MilestoneType, occurredAt)
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

// flagImplausible records an audit_logs entry for an implausible milestone
// sequence attempt (§83 Privacy & Anti-Gaming) so moderators can review
// flagged attempts. Failures are logged but do not change the rejection
// outcome — the milestone is rejected regardless.
func (s *Service) flagImplausible(ctx context.Context, tenantID, userID, timelineID pgtype.UUID, milestoneType string, occurredAt pgtype.Timestamptz) {
	if err := s.store.FlagImplausibleSequence(ctx, tenantID, userID, timelineID, milestoneType, occurredAt); err != nil {
		slog.WarnContext(ctx, "visatimeline: failed to record implausible-sequence flag",
			"error", err, "timeline_id", timelineID, "user_id", userID)
	}
}

// UpdateMilestone edits a milestone belonging to a timeline owned by the
// authenticated user. Ownership is verified via the parent timeline.
func (s *Service) UpdateMilestone(ctx context.Context, milestoneID, timelineID, tenantID, userID pgtype.UUID, input UpdateMilestoneInput) error {
	existing, err := s.store.GetTimeline(ctx, timelineID, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
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
		if errors.Is(err, pgx.ErrNoRows) {
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
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("visatimeline: admin delete: %w", err)
	}

	return nil
}

// ErrNotOwner is returned when a user tries to modify a timeline they did not
// create.
var ErrNotOwner = errors.New("not the timeline owner")

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
		ID:                 st.ID.String(),
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
// Each eligible owner receives a visa_timeline.prompt_update notification
// with an action URL back to /visa-timelines/my. Mirrors the
// planning_group:checkin_reminder dispatch pattern (§82). If no notifier is
// wired, the method logs a warning and returns 0 so the task does not
// silently claim success.
func (s *Service) PromptUpdate(ctx context.Context) (int64, error) {
	owners, err := s.store.InProgressTimelineOwners(ctx)
	if err != nil {
		return 0, fmt.Errorf("visatimeline: prompt update: %w", err)
	}

	if len(owners) == 0 {
		return 0, nil
	}

	if s.notifier == nil {
		slog.WarnContext(ctx, "visatimeline: prompt update notifier not wired",
			"eligible", len(owners))
		return 0, nil
	}

	var sent int64

	zero := pgtype.UUID{}
	title := "Update your visa timeline"
	body := "Your visa application timeline is still in progress. Add the latest milestone to help the community and keep your record up to date."

	for _, o := range owners {
		if err := s.notifier.CreateNotification(
			ctx, o.TenantID, o.UserID,
			"visa_timeline.prompt_update", "system", zero,
			"visa_timeline", zero,
			title, body,
			"/visa-timelines/my",
		); err != nil {
			slog.WarnContext(ctx, "visatimeline: prompt update notification failed",
				"error", err, "user_id", o.UserID)
			continue
		}

		sent++
	}

	slog.InfoContext(ctx, "visatimeline: prompt updates dispatched",
		"sent", sent, "eligible", len(owners))

	return sent, nil
}

// CreateProcessingTimeRequest is the request body for creating a
// visa_processing_stats row via the tool writer.
type CreateProcessingTimeRequest struct {
	OriginSpaceID      string   `json:"origin_space_id"`
	DestinationSpaceID string   `json:"destination_space_id"`
	VisaCategory       string   `json:"visa_category"`
	Consulate          string   `json:"consulate"`
	PeriodStart        string   `json:"period_start"` // YYYY-MM-DD
	SampleCount        int32    `json:"sample_count"`
	AvgDays            *float64 `json:"avg_days"`
	MedianDays         *float64 `json:"median_days"`
	P10Days            *float64 `json:"p10_days"`
	P90Days            *float64 `json:"p90_days"`
}

// parseSpaceID converts a string UUID into a pgtype.UUID, returning a zero
// value for an empty string.
func parseSpaceID(v string) (pgtype.UUID, error) {
	if v == "" {
		return pgtype.UUID{}, nil
	}

	var id pgtype.UUID
	if err := id.Scan(v); err != nil {
		return pgtype.UUID{}, err
	}

	return id, nil
}

// CreateProcessingTime creates a new visa_processing_stats row.
func (s *Service) CreateProcessingTime(ctx context.Context, tenantID pgtype.UUID, input CreateProcessingTimeRequest) (ProcessingStatDTO, error) {
	if input.DestinationSpaceID == "" {
		return ProcessingStatDTO{}, errors.New("destination_space_id is required")
	}

	if input.VisaCategory == "" {
		return ProcessingStatDTO{}, errors.New("visa_category is required")
	}

	if input.PeriodStart == "" {
		return ProcessingStatDTO{}, errors.New("period_start is required")
	}

	destSpaceID, err := parseSpaceID(input.DestinationSpaceID)
	if err != nil {
		return ProcessingStatDTO{}, fmt.Errorf("invalid destination_space_id: %w", err)
	}

	originSpaceID, err := parseSpaceID(input.OriginSpaceID)
	if err != nil {
		return ProcessingStatDTO{}, fmt.Errorf("invalid origin_space_id: %w", err)
	}

	t, err := time.Parse("2006-01-02", input.PeriodStart)
	if err != nil {
		return ProcessingStatDTO{}, fmt.Errorf("invalid period_start: %w", err)
	}

	var consulate *string
	if input.Consulate != "" {
		consulate = &input.Consulate
	}

	row, err := s.store.CreateProcessingTime(ctx, CreateProcessingTimeInput{
		TenantID:           tenantID,
		OriginSpaceID:      originSpaceID,
		DestinationSpaceID: destSpaceID,
		VisaCategory:       input.VisaCategory,
		Consulate:          consulate,
		PeriodStart:        pgtype.Timestamptz{Time: t.UTC(), Valid: true},
		SampleCount:        input.SampleCount,
		AvgDays:            input.AvgDays,
		MedianDays:         input.MedianDays,
		P10Days:            input.P10Days,
		P90Days:            input.P90Days,
	})
	if err != nil {
		return ProcessingStatDTO{}, fmt.Errorf("visatimeline: create processing time: %w", err)
	}

	return statToDTO(row), nil
}

// UpdateProcessingTimeRequest is the request body for updating a
// visa_processing_stats row via the tool writer.
type UpdateProcessingTimeRequest struct {
	OriginSpaceID      *pgtype.UUID `json:"origin_space_id,omitempty"`
	DestinationSpaceID *pgtype.UUID `json:"destination_space_id,omitempty"`
	VisaCategory       *string      `json:"visa_category,omitempty"`
	Consulate          *string      `json:"consulate,omitempty"`
	PeriodStart        *string      `json:"period_start,omitempty"`
	SampleCount        *int32       `json:"sample_count,omitempty"`
	AvgDays            *float64     `json:"avg_days,omitempty"`
	MedianDays         *float64     `json:"median_days,omitempty"`
	P10Days            *float64     `json:"p10_days,omitempty"`
	P90Days            *float64     `json:"p90_days,omitempty"`
}

// UpdateProcessingTime updates a visa_processing_stats row.
func (s *Service) UpdateProcessingTime(ctx context.Context, id, tenantID pgtype.UUID, input UpdateProcessingTimeRequest) (ProcessingStatDTO, error) {
	var in UpdateProcessingTimeInput
	in.OriginSpaceID = input.OriginSpaceID
	in.DestinationSpaceID = input.DestinationSpaceID
	in.VisaCategory = input.VisaCategory
	in.Consulate = input.Consulate
	in.SampleCount = input.SampleCount
	in.AvgDays = input.AvgDays
	in.MedianDays = input.MedianDays
	in.P10Days = input.P10Days
	in.P90Days = input.P90Days

	if input.PeriodStart != nil {
		t, err := time.Parse("2006-01-02", *input.PeriodStart)
		if err != nil {
			return ProcessingStatDTO{}, fmt.Errorf("invalid period_start: %w", err)
		}

		ts := pgtype.Timestamptz{Time: t.UTC(), Valid: true}
		in.PeriodStart = &ts
	}

	row, err := s.store.UpdateProcessingTime(ctx, id, tenantID, in)
	if err != nil {
		return ProcessingStatDTO{}, fmt.Errorf("visatimeline: update processing time: %w", err)
	}

	return statToDTO(row), nil
}

// DeleteProcessingTime deletes a visa_processing_stats row.
func (s *Service) DeleteProcessingTime(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.store.DeleteProcessingTime(ctx, id, tenantID); err != nil {
		return fmt.Errorf("visatimeline: delete processing time: %w", err)
	}

	return nil
}
