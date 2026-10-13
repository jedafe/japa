// Package japastage — journeys.go implements ADR-129 Concurrent Journeys.
// user_journeys rows hold concurrent and past journeys beside the primary
// profile journey (user_profiles.japa_intent/japa_stage). Each journey owns
// an intent×stage pair under the ADR-122 class rule, a lifecycle status,
// an optional travel window, and a per-journey stage history. Record-only:
// personalization keys on the primary profile intent; journeys never
// promote into it.
//
// Ref: japaconnect-engineering/sections/87-japa-stage-profiles.md
//
//	japaconnect-engineering/adr/ADR-129-concurrent-journeys.md
package japastage

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// Journey lifecycle statuses (ADR-129). Transition rules live in
// validJourneyTransitions — status changes only via those edges; travel
// windows are informational and never drive transitions.
const (
	JourneyStatusPlanned   = "planned"
	JourneyStatusActive    = "active"
	JourneyStatusCompleted = "completed"
	JourneyStatusCancelled = "cancelled"
)

// validJourneyTransitions is the journey lifecycle graph:
// planned → active → completed; planned|active → cancelled;
// cancelled → planned (reopen); completed is terminal.
var validJourneyTransitions = map[string]map[string]bool{
	JourneyStatusPlanned:   {JourneyStatusActive: true, JourneyStatusCancelled: true},
	JourneyStatusActive:    {JourneyStatusCompleted: true, JourneyStatusCancelled: true, JourneyStatusPlanned: true},
	JourneyStatusCancelled: {JourneyStatusPlanned: true},
	JourneyStatusCompleted: {},
}

// ValidJourneyStatus reports whether s is a known journey lifecycle value.
func ValidJourneyStatus(s string) bool {
	_, ok := validJourneyTransitions[s]
	return ok
}

// ValidJourneyTransition reports whether a status move from→to is a legal
// edge. A no-op (from == to) is allowed so PATCH can resubmit the current
// status alongside field edits.
func ValidJourneyTransition(from, to string) bool {
	if from == to {
		return true
	}

	return validJourneyTransitions[from][to]
}

// ErrInvalidJourneyTransition is returned when a PATCH attempts a status
// move outside validJourneyTransitions (e.g. completed → active).
var ErrInvalidJourneyTransition = errors.New("japastage: invalid journey status transition")

// ErrJourneyLimitReached is returned when a member holds
// japa_stage.max_journeys non-deleted journeys (ADR-129).
var ErrJourneyLimitReached = errors.New("japastage: journey limit reached")

// ErrInvalidJourneyDate is returned when a submitted travel_start or
// travel_end is not a YYYY-MM-DD date, or end precedes start.
var ErrInvalidJourneyDate = errors.New("japastage: invalid travel window")

// maxJourneyLabelLen caps the member-supplied journey label (a hard
// input bound, not a configurable value — Rule 3 covers tunables).
const maxJourneyLabelLen = 200

// ErrInvalidJourneyInput is returned when a submitted field fails basic
// validation (unknown intent or lifecycle status). The HTTP layer maps
// the specific field error before the service is called — this is the
// service's own belt-and-braces check.
var ErrInvalidJourneyInput = errors.New("japastage: invalid journey input")

// validJourneyDate parses a YYYY-MM-DD string; empty is allowed (NULL).
// Returns ErrInvalidJourneyDate on a malformed value.
func validJourneyDate(v string) error {
	if v == "" {
		return nil
	}

	if _, err := time.Parse("2006-01-02", v); err != nil {
		return ErrInvalidJourneyDate
	}

	return nil
}

// validateWindow checks both dates and their ordering (the
// user_journeys_window_chk contract, enforced in the service so a clear
// 400 is returned instead of a constraint violation).
func validateWindow(start, end string) error {
	if err := validJourneyDate(start); err != nil {
		return err
	}

	if err := validJourneyDate(end); err != nil {
		return err
	}

	if start != "" && end != "" {
		s, _ := time.Parse("2006-01-02", start)
		e, _ := time.Parse("2006-01-02", end)

		if e.Before(s) {
			return ErrInvalidJourneyDate
		}
	}

	return nil
}

// validUUID reports whether s parses as a UUID — guards the SQL-side
// uuid cast so a malformed id yields ErrInvalidJourneyInput, not a 500.
func validUUID(s string) bool {
	var u pgtype.UUID
	return u.Scan(s) == nil
}

// JourneyDTO is the API response shape for a user_journeys row (own view).
// InWindow is computed at read time — today within the travel window — and
// is informational only; it does not imply lifecycle state.
type JourneyDTO struct {
	ID            string          `json:"id"`
	Intent        string          `json:"intent"`
	Stage         string          `json:"stage"`
	DestSpaceID   string          `json:"destination_space_id,omitempty"`
	DestSpaceName string          `json:"destination_space_name,omitempty"`
	DestSpaceCode string          `json:"destination_space_code,omitempty"`
	Label         string          `json:"label,omitempty"`
	TravelStart   string          `json:"travel_start,omitempty"`
	TravelEnd     string          `json:"travel_end,omitempty"`
	Status        string          `json:"status"`
	InWindow      bool            `json:"in_window"`
	IsPublic      bool            `json:"is_public"`
	History       json.RawMessage `json:"history,omitempty"`
	CreatedAt     string          `json:"created_at,omitempty"`
	UpdatedAt     string          `json:"updated_at,omitempty"`
}

// JourneyInput is the POST body for journey creation. Intent is required;
// Stage defaults to "planning" and must satisfy the intent class rule.
// Status on create is limited to planned|active — a member cannot mint a
// completed journey directly.
type JourneyInput struct {
	Intent      string `json:"intent"`
	Stage       string `json:"stage"`
	DestSpaceID string `json:"destination_space_id"`
	Label       string `json:"label"`
	TravelStart string `json:"travel_start"` // RFC3339 date (YYYY-MM-DD)
	TravelEnd   string `json:"travel_end"`
	Status      string `json:"status"`
	IsPublic    *bool  `json:"is_public"`
}

// JourneyPatchInput is the PATCH body. Non-empty strings set; empty means
// "leave unchanged". Status is validated against the lifecycle graph.
// A submitted value always wins over its Clear* flag — the flag only
// nulls the column when no replacement was submitted.
type JourneyPatchInput struct {
	Intent      string `json:"intent"`
	Stage       string `json:"stage"`
	DestSpaceID string `json:"destination_space_id"`
	ClearDest   bool   `json:"clear_destination"`
	Label       string `json:"label"`
	ClearLabel  bool   `json:"clear_label"`
	TravelStart string `json:"travel_start"`
	TravelEnd   string `json:"travel_end"`
	ClearWindow bool   `json:"clear_travel_window"`
	Status      string `json:"status"`
	IsPublic    *bool  `json:"is_public"`
}

// ListJourneys returns the member's non-deleted journeys (own view —
// includes cancelled) ordered most-recently-updated first.
func (s *Service) ListJourneys(ctx context.Context, userID pgtype.UUID) ([]JourneyDTO, error) {
	rows, err := s.store.ListJourneys(ctx, userID)
	if err != nil {
		return nil, err
	}

	out := make([]JourneyDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, journeyDTO(r))
	}

	return out, nil
}

// CreateJourney inserts a journey row. Stage defaults to "planning" and is
// validated against the intent class; status on create is planned|active
// only. The member cap is japa_stage.max_journeys (ADR-129).
func (s *Service) CreateJourney(ctx context.Context, userID pgtype.UUID, in JourneyInput) (JourneyDTO, error) {
	if !ValidIntent(in.Intent) {
		return JourneyDTO{}, ErrInvalidJourneyInput
	}

	if len(in.Label) > maxJourneyLabelLen {
		return JourneyDTO{}, ErrInvalidJourneyInput
	}

	stage := in.Stage
	if stage == "" {
		stage = "planning"
	}

	if !ValidStageForIntent(stage, in.Intent) {
		return JourneyDTO{}, ErrInvalidStageForIntent
	}

	status := in.Status
	if status == "" {
		status = JourneyStatusPlanned
	}

	// Only forward lifecycle starts are creatable — a member cannot mint a
	// completed or cancelled journey directly.
	if status != JourneyStatusPlanned && status != JourneyStatusActive {
		return JourneyDTO{}, ErrInvalidJourneyInput
	}

	if in.DestSpaceID != "" && !validUUID(in.DestSpaceID) {
		return JourneyDTO{}, ErrInvalidJourneyInput
	}

	if err := validateWindow(in.TravelStart, in.TravelEnd); err != nil {
		return JourneyDTO{}, err
	}

	max := s.settings.GetIntOr(ctx, "japa_stage.max_journeys", 10)

	count, err := s.store.CountLiveJourneys(ctx, userID)
	if err != nil {
		return JourneyDTO{}, err
	}

	if count >= int64(max) {
		return JourneyDTO{}, ErrJourneyLimitReached
	}

	row, err := s.store.CreateJourney(ctx, userID, in.Intent, stage, in.DestSpaceID, in.Label, in.TravelStart, in.TravelEnd, status, in.IsPublic)
	if err != nil {
		return JourneyDTO{}, err
	}

	return journeyDTO(row), nil
}

// UpdateJourney applies a partial update. Stage/intent changes are
// validated against the ADR-122 class rule — an intent switch that
// invalidates the stored stage resets it to the class default "planning"
// (journey stage is NOT NULL, unlike the profile column which Upsert-
// JourneyClass clears). Status moves must be legal edges in the lifecycle
// graph. History only appends when intent, stage, or status actually
// changed — a label- or window-only edit is not a transition.
func (s *Service) UpdateJourney(ctx context.Context, userID, journeyID pgtype.UUID, in JourneyPatchInput) (JourneyDTO, error) {
	if len(in.Label) > maxJourneyLabelLen {
		return JourneyDTO{}, ErrInvalidJourneyInput
	}

	if in.Intent != "" && !ValidIntent(in.Intent) {
		return JourneyDTO{}, ErrInvalidJourneyInput
	}

	if in.DestSpaceID != "" && !validUUID(in.DestSpaceID) {
		return JourneyDTO{}, ErrInvalidJourneyInput
	}

	if in.Status != "" && !ValidJourneyStatus(in.Status) {
		return JourneyDTO{}, ErrInvalidJourneyInput
	}

	current, err := s.store.GetJourney(ctx, userID, journeyID)
	if err != nil {
		return JourneyDTO{}, err
	}

	// Effective intent: submitted when non-empty, else stored.
	intent := in.Intent
	if intent == "" {
		intent = derefStr(current.Intent)
	}

	// Effective stage: submitted when non-empty; if an intent switch
	// invalidated the stored stage it clears to the class default
	// (planning) — journeys always carry a stage (NOT NULL).
	stage := in.Stage
	if stage == "" {
		stored := derefStr(current.Stage)
		if stored != "" && ValidStageForIntent(stored, intent) {
			stage = stored
		} else {
			stage = "planning"
		}
	}

	// Merged travel window: submitted dates win, stored dates carry over
	// unless the window is being cleared. Ordering is checked on the merged
	// pair — a PATCH that sets only travel_end can still violate the
	// window CHECK when it precedes the stored start.
	effectiveStart, effectiveEnd := in.TravelStart, in.TravelEnd
	if !in.ClearWindow {
		if effectiveStart == "" && current.TravelStart.Valid {
			effectiveStart = current.TravelStart.Time.Format("2006-01-02")
		}

		if effectiveEnd == "" && current.TravelEnd.Valid {
			effectiveEnd = current.TravelEnd.Time.Format("2006-01-02")
		}
	}

	if !ValidStageForIntent(stage, intent) {
		return JourneyDTO{}, ErrInvalidStageForIntent
	}

	if in.Status != "" && !ValidJourneyTransition(derefStr(current.Status), in.Status) {
		return JourneyDTO{}, ErrInvalidJourneyTransition
	}

	if err := validateWindow(effectiveStart, effectiveEnd); err != nil {
		return JourneyDTO{}, err
	}

	// History records transitions, not edits — append only when the
	// effective intent/stage/status changed. Label, window, and visibility
	// edits bump updated_at without polluting the stage log.
	statusChanged := in.Status != "" && in.Status != derefStr(current.Status)
	recordHistory := intent != derefStr(current.Intent) || stage != derefStr(current.Stage) || statusChanged

	// Effective destination for the history entry mirrors the store CASE.
	effectiveDest := current.DestSpaceID
	if in.DestSpaceID != "" {
		effectiveDest = uuidOrNil(in.DestSpaceID)
	} else if in.ClearDest {
		effectiveDest = pgtype.UUID{}
	}

	row, err := s.store.UpdateJourney(ctx, userID, journeyID, intent, stage, effectiveDest, recordHistory, in)
	if err != nil {
		return JourneyDTO{}, err
	}

	return journeyDTO(row), nil
}

// DeleteJourney soft-deletes a journey row owned by the member (Rule 11).
func (s *Service) DeleteJourney(ctx context.Context, userID, journeyID pgtype.UUID) error {
	return s.store.DeleteJourney(ctx, userID, journeyID)
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

// journeyDTO maps a store row to the API shape. InWindow is computed
// against today's UTC date with open bounds counting as inside.
func journeyDTO(r JourneyRow) JourneyDTO {
	d := JourneyDTO{
		ID:       uuidString(r.ID),
		Intent:   derefStr(r.Intent),
		Stage:    derefStr(r.Stage),
		Label:    derefStr(r.Label),
		Status:   derefStr(r.Status),
		IsPublic: r.IsPublic,
		History:  r.History,
	}

	if r.DestSpaceID.Valid {
		d.DestSpaceID = uuidString(r.DestSpaceID)
	}

	d.DestSpaceName = derefStr(r.DestSpaceName)
	d.DestSpaceCode = derefStr(r.DestSpaceCode)

	var start, end *time.Time

	if r.TravelStart.Valid {
		t := r.TravelStart.Time
		start = &t
		d.TravelStart = t.Format("2006-01-02")
	}

	if r.TravelEnd.Valid {
		t := r.TravelEnd.Time
		end = &t
		d.TravelEnd = t.Format("2006-01-02")
	}

	today := time.Now().UTC().Truncate(24 * time.Hour)
	d.InWindow = (start == nil || !today.Before(*start)) && (end == nil || !today.After(*end)) && (start != nil || end != nil)

	if r.CreatedAt.Valid {
		d.CreatedAt = r.CreatedAt.Time.UTC().Format(time.RFC3339)
	}

	if r.UpdatedAt.Valid {
		d.UpdatedAt = r.UpdatedAt.Time.UTC().Format(time.RFC3339)
	}

	return d
}

// uuidString renders a pgtype.UUID as the canonical hex form.
func uuidString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}

	b := u.Bytes

	const hex = "0123456789abcdef"

	buf := make([]byte, 36)

	j := 0
	for i, c := range b {
		if i == 4 || i == 6 || i == 8 || i == 10 {
			buf[j] = '-'
			j++
		}

		buf[j] = hex[c>>4]
		buf[j+1] = hex[c&0x0f]
		j += 2
	}

	return string(buf)
}
