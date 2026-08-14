package spaces

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// ---------------------------------------------------------------------------
// Space Rule Handlers
// ---------------------------------------------------------------------------

// ListSpaceRules handles GET /api/v1/spaces/{slug}/rules. The space is
// resolved by the SpaceResolver middleware and read from the request context.
func (h *Handlers) ListSpaceRules(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	rules, err := h.Service.ListSpaceRules(r.Context(), space.ID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	if rules == nil {
		rules = []gen.SpaceRule{}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": rules})
}

// CreateSpaceRule handles POST /api/v1/spaces/{slug}/rules.
func (h *Handlers) CreateSpaceRule(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	var req struct {
		Title    string `json:"title"`
		Body     string `json:"body"`
		Position int32  `json:"position"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.Body) == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "Title and body are required")
		return
	}

	rule, err := h.Service.CreateSpaceRule(r.Context(), gen.CreateSpaceRuleParams{
		SpaceID:  space.ID,
		Title:    req.Title,
		Body:     req.Body,
		Position: req.Position,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": rule})
}

// UpdateSpaceRule handles PATCH /api/v1/spaces/{slug}/rules/{ruleId}.
func (h *Handlers) UpdateSpaceRule(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	ruleID, err := parseUUIDParam(r, "ruleId")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid rule ID format")
		return
	}

	var req struct {
		Title    string `json:"title"`
		Body     string `json:"body"`
		Position int32  `json:"position"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.Body) == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "Title and body are required")
		return
	}

	rule, err := h.Service.UpdateSpaceRule(r.Context(), gen.UpdateSpaceRuleParams{
		Title:    req.Title,
		Body:     req.Body,
		Position: req.Position,
		ID:       ruleID,
		SpaceID:  space.ID,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": rule})
}

// DeleteSpaceRule handles DELETE /api/v1/spaces/{slug}/rules/{ruleId}.
func (h *Handlers) DeleteSpaceRule(w http.ResponseWriter, r *http.Request) {
	space, ok := spaceFromContext(w, r)
	if !ok {
		return
	}

	ruleID, err := parseUUIDParam(r, "ruleId")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid rule ID format")
		return
	}

	if err := h.Service.DeleteSpaceRule(r.Context(), ruleID, space.ID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}
