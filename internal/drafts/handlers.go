// Package drafts implements the generic in-progress form-draft API —
// server-side save-progress reusable by any multi-step tool surface
// (readiness, tour, sop, future wizards). Ref: §71 amendment.
package drafts

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Store is the sqlc slice the handlers need.
type Store interface {
	UpsertFormDraft(ctx context.Context, arg gen.UpsertFormDraftParams) (gen.FormDraft, error)
	GetFormDraft(ctx context.Context, arg gen.GetFormDraftParams) (gen.FormDraft, error)
	ListFormDraftsByUser(ctx context.Context, userID pgtype.UUID) ([]gen.FormDraft, error)
	DeleteFormDraft(ctx context.Context, arg gen.DeleteFormDraftParams) error
}

type Handlers struct {
	store Store
}

func NewHandlers(store Store) *Handlers { return &Handlers{store: store} }

type draftDTO struct {
	ID        string          `json:"id"`
	Kind      string          `json:"kind"`
	Key       string          `json:"key"`
	Data      json.RawMessage `json:"data"`
	UpdatedAt string          `json:"updated_at"`
}

func toDTO(d gen.FormDraft) draftDTO {
	return draftDTO{
		ID:        d.ID.String(),
		Kind:      d.Kind,
		Key:       d.DraftKey,
		Data:      d.Data,
		UpdatedAt: d.UpdatedAt.Time.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

func userID(r *http.Request) (pgtype.UUID, bool) {
	u := auth.FromContext(r.Context())
	if u == nil {
		return pgtype.UUID{}, false
	}
	return u.UserID, true
}

// ListDrafts handles GET /me/form-drafts — all the caller's drafts, newest first.
func (h *Handlers) ListDrafts(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(r)
	if !ok {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Sign in to view drafts.")
		return
	}
	rows, err := h.store.ListFormDraftsByUser(r.Context(), uid)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list drafts")
		return
	}
	out := make([]draftDTO, len(rows))
	for i, d := range rows {
		out[i] = toDTO(d)
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": out})
}

// GetDraft handles GET /me/form-drafts/{kind}/{key?} — 404 when none exists.
func (h *Handlers) GetDraft(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(r)
	if !ok {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Sign in to view drafts.")
		return
	}
	row, err := h.store.GetFormDraft(r.Context(), gen.GetFormDraftParams{
		UserID: uid, Kind: chi.URLParam(r, "kind"), DraftKey: chi.URLParam(r, "key"),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "No draft saved.")
		return
	}
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to load draft")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": toDTO(row)})
}

var kindRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
var keyRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.:-]{0,127}$`)

// maxDraftDataBytes caps the JSONB payload per draft (BUG-CE-020).
const maxDraftDataBytes = 256 * 1024

// PutDraft handles PUT /me/form-drafts/{kind}/{key?} — upsert. Body: {"data":{...}}.
func (h *Handlers) PutDraft(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(r)
	if !ok {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Sign in to save progress.")
		return
	}
	if !kindRe.MatchString(chi.URLParam(r, "kind")) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_kind", "kind must match ^[a-z][a-z0-9_-]{0,63}$")
		return
	}
	if key := chi.URLParam(r, "key"); key != "" && !keyRe.MatchString(key) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_key", "key must match ^[a-zA-Z0-9][a-zA-Z0-9_.:-]{0,127}$")
		return
	}
	var body struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil ||
		len(body.Data) == 0 || string(body.Data) == "null" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Body must be {\"data\": {...}}")
		return
	}
	if len(body.Data) > maxDraftDataBytes {
		errpkg.WriteError(w, http.StatusRequestEntityTooLarge, "draft_too_large", "Draft data exceeds the 256KB limit")
		return
	}
	row, err := h.store.UpsertFormDraft(r.Context(), gen.UpsertFormDraftParams{
		UserID: uid, Kind: chi.URLParam(r, "kind"), DraftKey: chi.URLParam(r, "key"), Data: body.Data,
	})
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to save draft")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": toDTO(row)})
}

// DeleteDraft handles DELETE /me/form-drafts/{kind}/{key?} — idempotent.
func (h *Handlers) DeleteDraft(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(r)
	if !ok {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Sign in to delete drafts.")
		return
	}
	if err := h.store.DeleteFormDraft(r.Context(), gen.DeleteFormDraftParams{
		UserID: uid, Kind: chi.URLParam(r, "kind"), DraftKey: chi.URLParam(r, "key"),
	}); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete draft")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "deleted"}})
}
