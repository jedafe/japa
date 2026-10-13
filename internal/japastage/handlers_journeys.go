// Package japastage — handlers_journeys.go provides the HTTP handlers for
// the concurrent-journeys CRUD routes (ADR-129). Journeys are record-only
// instances beside the primary profile journey; all routes sit inside the
// japa_stage.enabled feature gate and require profile.japa_stage.edit.
//
// Routes:
//
//	GET    /api/v1/me/japa-stage/journeys      — list own journeys
//	POST   /api/v1/me/japa-stage/journeys      — create journey
//	PATCH  /api/v1/me/japa-stage/journeys/:id  — update fields + transitions
//	DELETE /api/v1/me/japa-stage/journeys/:id  — soft delete (Rule 11)
//
// Ref: japaconnect-engineering/sections/87-japa-stage-profiles.md
//
//	japaconnect-engineering/adr/ADR-129-concurrent-journeys.md
package japastage

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	errpkg "github.com/jedafe/japa/internal/errors"
)

// journeysEnabled guards the journeys routes — they share the §87
// japa_stage.enabled feature flag with the profile stage endpoints.
func (h *Handlers) journeysEnabled(w http.ResponseWriter, r *http.Request) bool {
	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "japa_stage_disabled", "Japa stage profiles are not available")
		return false
	}

	return true
}

// HandleListJourneys handles GET /me/japa-stage/journeys — the member's own
// non-deleted journeys (cancelled included) ordered by recency.
func (h *Handlers) HandleListJourneys(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	if !h.journeysEnabled(w, r) {
		return
	}

	journeys, err := h.service.ListJourneys(r.Context(), user.UserID)
	if err != nil {
		slog.Error("japastage: list journeys failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list journeys")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": journeys})
}

// HandleCreateJourney handles POST /me/japa-stage/journeys. Intent is
// required; stage defaults to planning and must satisfy the class rule;
// status on create is limited to planned|active. Returns
// journey_limit_reached at the japa_stage.max_journeys cap.
func (h *Handlers) HandleCreateJourney(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	if !h.journeysEnabled(w, r) {
		return
	}

	var input JourneyInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.Intent == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_intent", "Intent is required")
		return
	}

	if !ValidIntent(input.Intent) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_intent", "Invalid intent; must be one of: "+JoinIntents())
		return
	}

	if input.Stage != "" && !ValidStage(input.Stage) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_stage", "Invalid stage; must be one of: "+JoinStages())
		return
	}

	if input.Stage != "" && !ValidStageForIntent(input.Stage, input.Intent) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_stage_for_intent", "That stage does not apply to the selected journey type")
		return
	}

	if input.Status != "" && input.Status != JourneyStatusPlanned && input.Status != JourneyStatusActive {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_status", "Status on create must be planned or active")
		return
	}

	if err := validateWindow(input.TravelStart, input.TravelEnd); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_travel_window", "Travel dates must be YYYY-MM-DD with end on or after start")
		return
	}

	journey, err := h.service.CreateJourney(r.Context(), user.UserID, input)
	if err != nil {
		if errors.Is(err, ErrJourneyLimitReached) {
			errpkg.WriteError(w, http.StatusConflict, "journey_limit_reached", "Journey limit reached")
			return
		}

		if errors.Is(err, ErrJourneyDestinationInvalid) {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_destination", "Destination is not a valid country space")
			return
		}

		if errors.Is(err, ErrInvalidJourneyInput) || errors.Is(err, ErrInvalidStageForIntent) || errors.Is(err, ErrInvalidJourneyDate) {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_journey", "Invalid journey fields")
			return
		}

		slog.Error("japastage: create journey failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "create_failed", "Failed to create journey")

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": journey})
}

// HandleUpdateJourney handles PATCH /me/japa-stage/journeys/:id — a
// partial update of intent/stage/destination/label/window/visibility plus
// lifecycle transitions per validJourneyTransitions (ADR-129).
func (h *Handlers) HandleUpdateJourney(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	if !h.journeysEnabled(w, r) {
		return
	}

	journeyID, ok := journeyIDParam(w, r)
	if !ok {
		return
	}

	var input JourneyPatchInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.Intent != "" && !ValidIntent(input.Intent) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_intent", "Invalid intent; must be one of: "+JoinIntents())
		return
	}

	if input.Stage != "" && !ValidStage(input.Stage) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_stage", "Invalid stage; must be one of: "+JoinStages())
		return
	}

	if input.Stage != "" && input.Intent != "" && !ValidStageForIntent(input.Stage, input.Intent) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_stage_for_intent", "That stage does not apply to the selected journey type")
		return
	}

	if input.Status != "" && !ValidJourneyStatus(input.Status) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_status", "Invalid status; must be one of: planned, active, completed, cancelled")
		return
	}

	if err := validateWindow(input.TravelStart, input.TravelEnd); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_travel_window", "Travel dates must be YYYY-MM-DD with end on or after start")
		return
	}

	journey, err := h.service.UpdateJourney(r.Context(), user.UserID, journeyID, input)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "journey_not_found", "Journey not found")
			return
		}

		if errors.Is(err, ErrInvalidJourneyTransition) {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_status_transition", "That status transition is not allowed")
			return
		}

		if errors.Is(err, ErrInvalidStageForIntent) {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_stage_for_intent", "That stage does not apply to the selected journey type")
			return
		}

		if errors.Is(err, ErrInvalidJourneyDate) {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_travel_window", "Travel end must be on or after start")
			return
		}

		if errors.Is(err, ErrJourneyDestinationInvalid) {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_destination", "Destination is not a valid country space")
			return
		}

		if errors.Is(err, ErrInvalidJourneyInput) {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_journey", "Invalid journey fields")
			return
		}

		slog.Error("japastage: update journey failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to update journey")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": journey})
}

// HandleDeleteJourney handles DELETE /me/japa-stage/journeys/:id — soft
// delete (deleted_at) per Rule 11. The row stops appearing in listings;
// cancellation is a lifecycle status, deletion is record removal.
func (h *Handlers) HandleDeleteJourney(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	if !h.journeysEnabled(w, r) {
		return
	}

	journeyID, ok := journeyIDParam(w, r)
	if !ok {
		return
	}

	if err := h.service.DeleteJourney(r.Context(), user.UserID, journeyID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "journey_not_found", "Journey not found")
			return
		}

		slog.Error("japastage: delete journey failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "delete_failed", "Failed to delete journey")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"deleted": true}})
}

// journeyIDParam parses the {id} URL segment into a pgtype.UUID.
func journeyIDParam(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil || !id.Valid {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid journey id")
		return pgtype.UUID{}, false
	}

	return id, true
}
