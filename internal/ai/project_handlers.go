// Package ai — project_handlers.go implements the AI Projects endpoints
// (§103 amendment): CRUD over ai_projects plus membership assignment for
// conversations and agent sessions. Projects group threads in the /ai
// sidebar and carry per-project instructions (injected server-side into
// the thread's AI context) and pinned picker defaults.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
// (Amendment — AI Projects)
package ai

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// SetProjectService injects the AI projects store.
func (h *Handlers) SetProjectService(s *ProjectService) {
	h.Projects = s
}

// projectInput is the POST/PATCH body. Pointer fields preserve "not
// provided" for PATCH merges; DefaultTools is a raw JSON object (same
// shape as the `tools` selection on POST /ai/chat) stored verbatim.
type projectInput struct {
	Name           *string          `json:"name"`
	Instructions   *string          `json:"instructions"`
	DefaultModel   *string          `json:"default_model"`
	DefaultTone    *string          `json:"default_tone"`
	DefaultVoiceID *string          `json:"default_voice_id"`
	DefaultTools   *json.RawMessage `json:"default_tools"`
}

const (
	maxProjectNameLen         = 200
	maxProjectInstructionsLen = 4000
	maxProjectPinLen          = 128
	maxProjectToolsLen        = 8192
)

// validateProjectInput bounds the text fields and requires name on create.
func validateProjectInput(w http.ResponseWriter, in projectInput, create bool) bool {
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" || len(name) > maxProjectNameLen {
			errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "name is required and must be at most 200 characters")
			return false
		}
	} else if create {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "name is required")
		return false
	}
	if in.Instructions != nil && len(*in.Instructions) > maxProjectInstructionsLen {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "instructions must be at most 4000 characters")
		return false
	}
	for field, v := range map[string]*string{
		"default_model":    in.DefaultModel,
		"default_tone":     in.DefaultTone,
		"default_voice_id": in.DefaultVoiceID,
	} {
		if v != nil && len(*v) > maxProjectPinLen {
			errpkg.WriteError(w, http.StatusBadRequest, "validation_error", field+" must be at most 128 characters")
			return false
		}
	}
	if in.DefaultTools != nil {
		raw := *in.DefaultTools
		var obj map[string]any
		if len(raw) > maxProjectToolsLen || !json.Valid(raw) || json.Unmarshal(raw, &obj) != nil || obj == nil {
			errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "default_tools must be a JSON object")
			return false
		}
	}
	return true
}

// projectToJSON renders an ai_projects row. DefaultTools is decoded so
// clients get the object, not a stringified blob; pgtype timestamps go
// through the shared tsToISO helper.
func projectToJSON(p gen.AiProject) map[string]any {
	var tools any
	if len(p.DefaultTools) > 0 {
		_ = json.Unmarshal(p.DefaultTools, &tools)
	}
	if tools == nil {
		tools = map[string]any{}
	}
	return map[string]any{
		"id":               p.ID.String(),
		"name":             p.Name,
		"instructions":     p.Instructions,
		"default_model":    p.DefaultModel,
		"default_tone":     p.DefaultTone,
		"default_voice_id": p.DefaultVoiceID,
		"default_tools":    tools,
		"created_at":       tsToISO(p.CreatedAt),
		"updated_at":       tsToISO(p.UpdatedAt),
	}
}

func projectInputToService(in projectInput) ProjectInput {
	out := ProjectInput{}
	if in.Name != nil {
		out.Name = strings.TrimSpace(*in.Name)
	}
	if in.Instructions != nil {
		out.Instructions = *in.Instructions
	}
	if in.DefaultModel != nil {
		out.DefaultModel = strings.TrimSpace(*in.DefaultModel)
	}
	if in.DefaultTone != nil {
		out.DefaultTone = strings.TrimSpace(*in.DefaultTone)
	}
	if in.DefaultVoiceID != nil {
		out.DefaultVoiceID = strings.TrimSpace(*in.DefaultVoiceID)
	}
	if in.DefaultTools != nil {
		out.DefaultTools = *in.DefaultTools
	}
	return out
}

// requireProjectsReady is the shared auth + service-wired gate.
func (h *Handlers) requireProjectsReady(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}
	if h.Projects == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "not_configured", "AI projects are not configured")
		return nil, false
	}
	return user, true
}

// requireProject resolves the {id} path param to an owned live project —
// shared by the get/update handlers.
func (h *Handlers) requireProject(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, gen.AiProject, bool) {
	user, ok := h.requireProjectsReady(w, r)
	if !ok {
		return nil, gen.AiProject{}, false
	}
	projectID, parseErr := scanUUID(chi.URLParam(r, "id"))
	if parseErr != nil || !projectID.Valid {
		errpkg.WriteError(w, http.StatusNotFound, "project_not_found", "Project not found")
		return nil, gen.AiProject{}, false
	}
	p, err := h.Projects.Get(r.Context(), user.UserID, projectID)
	if err != nil {
		writeProjectError(w, err, "Failed to load project")
		return nil, gen.AiProject{}, false
	}
	return user, p, true
}

func writeProjectError(w http.ResponseWriter, err error, fallback string) {
	if errors.Is(err, ErrProjectNotFound) {
		errpkg.WriteError(w, http.StatusNotFound, "project_not_found", "Project not found")
		return
	}
	if errors.Is(err, ErrProjectNameTaken) {
		errpkg.WriteError(w, http.StatusConflict, "project_name_taken", "A project with this name already exists")
		return
	}
	if errors.Is(err, ErrInvalidVoicePin) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_voice_pin", "default_voice_id is not a known voice")
		return
	}
	errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", fallback)
}

// validateModelPin checks a non-empty default_model against the registered
// chat-category models — the same catalog the composer picker is drawn
// from. Skipped when no dispatcher is wired (dev without providers).
func (h *Handlers) validateModelPin(model string) bool {
	if model == "" || h.Dispatcher == nil {
		return true
	}
	return h.isValidModelForCategory(model, "chat")
}

// HandleListProjects handles GET /ai/projects — rows carry member counts
// for the sidebar group headers.
func (h *Handlers) HandleListProjects(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireProjectsReady(w, r)
	if !ok {
		return
	}

	pageSize := parsePageSize(r)
	pageOffset := parsePageOffset(r)

	rows, total, err := h.Projects.List(r.Context(), user.UserID, pageSize, pageOffset)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list projects")
		return
	}

	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		p := projectToJSON(gen.AiProject{
			ID:             row.ID,
			UserID:         row.UserID,
			Name:           row.Name,
			Instructions:   row.Instructions,
			DefaultModel:   row.DefaultModel,
			DefaultTone:    row.DefaultTone,
			DefaultVoiceID: row.DefaultVoiceID,
			DefaultTools:   row.DefaultTools,
			CreatedAt:      row.CreatedAt,
			UpdatedAt:      row.UpdatedAt,
		})
		p["conversation_count"] = row.ConversationCount
		p["session_count"] = row.SessionCount
		out = append(out, p)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": out,
		"meta": map[string]any{
			"total":       total,
			"page_size":   pageSize,
			"page_offset": pageOffset,
		},
	})
}

// HandleCreateProject handles POST /ai/projects.
func (h *Handlers) HandleCreateProject(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireProjectsReady(w, r)
	if !ok {
		return
	}

	var in projectInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}
	if !validateProjectInput(w, in, true) {
		return
	}
	if in.DefaultModel != nil && !h.validateModelPin(strings.TrimSpace(*in.DefaultModel)) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_model_pin", "default_model is not available for the chat category")
		return
	}

	p, err := h.Projects.Create(r.Context(), user.UserID, projectInputToService(in))
	if err != nil {
		writeProjectError(w, err, "Failed to create project")
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": projectToJSON(p)})
}

// HandleGetProject handles GET /ai/projects/{id}.
func (h *Handlers) HandleGetProject(w http.ResponseWriter, r *http.Request) {
	_, p, ok := h.requireProject(w, r)
	if !ok {
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": projectToJSON(p)})
}

// HandleUpdateProject handles PATCH /ai/projects/{id} — provided fields
// overwrite stored ones; omitted fields keep their current values.
func (h *Handlers) HandleUpdateProject(w http.ResponseWriter, r *http.Request) {
	user, existing, ok := h.requireProject(w, r)
	if !ok {
		return
	}

	var in projectInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}
	if !validateProjectInput(w, in, false) {
		return
	}
	if in.DefaultModel != nil && !h.validateModelPin(strings.TrimSpace(*in.DefaultModel)) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_model_pin", "default_model is not available for the chat category")
		return
	}

	merged := ProjectInput{
		Name:           existing.Name,
		Instructions:   existing.Instructions,
		DefaultModel:   existing.DefaultModel,
		DefaultTone:    existing.DefaultTone,
		DefaultVoiceID: existing.DefaultVoiceID,
		DefaultTools:   existing.DefaultTools,
	}
	if in.Name != nil {
		merged.Name = strings.TrimSpace(*in.Name)
	}
	if in.Instructions != nil {
		merged.Instructions = *in.Instructions
	}
	if in.DefaultModel != nil {
		merged.DefaultModel = strings.TrimSpace(*in.DefaultModel)
	}
	if in.DefaultTone != nil {
		merged.DefaultTone = strings.TrimSpace(*in.DefaultTone)
	}
	if in.DefaultVoiceID != nil {
		merged.DefaultVoiceID = strings.TrimSpace(*in.DefaultVoiceID)
	}
	if in.DefaultTools != nil {
		merged.DefaultTools = *in.DefaultTools
	}

	p, err := h.Projects.Update(r.Context(), user.UserID, existing.ID, merged)
	if err != nil {
		writeProjectError(w, err, "Failed to update project")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": projectToJSON(p)})
}

// HandleDeleteProject handles DELETE /ai/projects/{id}. Soft-deletes the
// project and un-groups its members — threads themselves are untouched.
func (h *Handlers) HandleDeleteProject(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireProjectsReady(w, r)
	if !ok {
		return
	}

	projectID, parseErr := scanUUID(chi.URLParam(r, "id"))
	if parseErr != nil || !projectID.Valid {
		errpkg.WriteError(w, http.StatusNotFound, "project_not_found", "Project not found")
		return
	}

	if err := h.Projects.Delete(r.Context(), user.UserID, projectID); err != nil {
		writeProjectError(w, err, "Failed to delete project")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// assignProjectInput is the PUT body for membership routes —
// {project_id: "uuid"|null|""}; null/empty unassigns.
type assignProjectInput struct {
	ProjectID *string `json:"project_id"`
}

// assignProject shares auth/param/body parsing between the conversation
// and agent-session membership routes.
func (h *Handlers) assignProject(w http.ResponseWriter, r *http.Request, conversation bool) {
	user, ok := h.requireProjectsReady(w, r)
	if !ok {
		return
	}
	notFoundCode := "conversation_not_found"
	if !conversation {
		notFoundCode = "session_not_found"
	}

	targetID, parseErr := scanUUID(chi.URLParam(r, "id"))
	if parseErr != nil || !targetID.Valid {
		errpkg.WriteError(w, http.StatusNotFound, notFoundCode, "Not found")
		return
	}

	var in assignProjectInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	var projectID *pgtype.UUID
	if in.ProjectID != nil && strings.TrimSpace(*in.ProjectID) != "" {
		pid, err := scanUUID(strings.TrimSpace(*in.ProjectID))
		if err != nil || !pid.Valid {
			errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "project_id must be a UUID or null")
			return
		}
		projectID = &pid
	}

	var err error
	if conversation {
		err = h.Projects.SetConversationProject(r.Context(), user.UserID, targetID, projectID)
	} else {
		err = h.Projects.SetSessionProject(r.Context(), user.UserID, targetID, projectID)
	}
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			errpkg.WriteError(w, http.StatusNotFound, "project_not_found", "Project not found")
			return
		}
		if errors.Is(err, ErrConversationNotFound) || errors.Is(err, ErrSessionNotFound) {
			errpkg.WriteError(w, http.StatusNotFound, notFoundCode, "Not found")
			return
		}
		if errors.Is(err, ErrSessionLinked) {
			// §103 — linked sessions mirror their conversation's project;
			// assigning them directly is a no-op that would only confuse.
			errpkg.WriteError(w, http.StatusConflict, "session_linked_to_conversation", "Session belongs to a conversation — move the conversation instead")
			return
		}
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to assign project")
		return
	}

	var resp any
	if projectID != nil {
		resp = projectID.String()
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"project_id": resp}})
}

// HandleAssignConversationProject handles PUT /ai/conversations/{id}/project.
func (h *Handlers) HandleAssignConversationProject(w http.ResponseWriter, r *http.Request) {
	h.assignProject(w, r, true)
}

// HandleAssignSessionProject handles PUT /ai/agent/sessions/{id}/project.
func (h *Handlers) HandleAssignSessionProject(w http.ResponseWriter, r *http.Request) {
	h.assignProject(w, r, false)
}
