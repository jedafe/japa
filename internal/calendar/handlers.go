// Package calendar — handlers.go provides the HTTP handlers for the
// Community Calendar routes (§86, ADR-094).
//
// Routes:
//
//	GET    /api/v1/calendar               — list approved entries (public)
//	GET    /api/v1/calendar/:id           — single approved entry (public)
//	POST   /api/v1/calendar               — submit entry (auth, calendar.submit)
//	PATCH  /api/v1/calendar/:id           — edit own entry (auth, calendar.edit_own/edit_any)
//	POST   /api/v1/calendar/:id/save      — save entry (auth)
//	DELETE /api/v1/calendar/:id/save      — unsave entry (auth)
//	POST   /api/v1/calendar/subscribe     — subscribe (auth)
//	DELETE /api/v1/calendar/subscribe     — unsubscribe (auth)
//	GET    /api/v1/admin/calendar         — admin list (auth, calendar.review)
//	PATCH  /api/v1/admin/calendar/:id     — approve/archive/flag (auth, calendar.review)
//
// Ref: japaconnect-engineering/sections/86-community-calendar.md
//
//	japaconnect-engineering/adr/ADR-094-community-calendar.md
package calendar

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/literals"
)

// updater is the subset of Service methods used by the public edit handler.
type updater interface {
	Enabled(ctx context.Context) bool
	Update(ctx context.Context, id, callerID pgtype.UUID, input UpdateEntryInput) (CalendarEntryDTO, error)
}

// literalResolver resolves {{...}} template literals in user-facing text
// fields (§14). Implemented by *literals.Catalog; nil disables resolution.
type literalResolver interface {
	ResolveFields(ctx context.Context, v any, rc literals.RenderContext)
}

// Handlers exposes the calendar endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
	updater  updater
	lit      literalResolver // nil disables literal resolution (§14)
}

// NewHandlers creates calendar handlers bound to the provided service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings, updater: service}
}

// SetLiteralResolver injects the template-literal catalog so entry titles and
// descriptions render {{...}} placeholders (§14).
func (h *Handlers) SetLiteralResolver(l literalResolver) { h.lit = l }

// resolveLiterals resolves {{...}} placeholders on an entry DTO or slice.
// Best-effort — failures are logged inside the resolver.
func (h *Handlers) resolveLiterals(r *http.Request, v any) {
	if literals.IsRawRequest(r) {
		return
	}

	if h.lit == nil {
		return
	}

	h.lit.ResolveFields(r.Context(), v, literals.NewRequestContext(r, ""))
}

func requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	u := auth.FromContext(r.Context())
	if u == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return u, true
}

func scanUUIDParam(w http.ResponseWriter, r *http.Request, name string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, name)); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid "+name)
		return pgtype.UUID{}, false
	}

	return id, true
}

func parsePageParams(r *http.Request) (page, limit int32) {
	page = 1
	limit = 20

	if v := r.URL.Query().Get("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = int32(n) // #nosec
		}
	} else if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = int32(n) // #nosec
		}
	}

	// Rule 5: page_offset is a record OFFSET (not a page number) —
	// ?page_size=N&page_offset=N. Derive the page from it. Keep `page` as a
	// legacy alias.
	if v := r.URL.Query().Get("page_offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			page = int32(n/int(limit)) + 1 // #nosec
		}
	} else if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = int32(n) // #nosec
		}
	}

	return page, limit
}

// ---------------------------------------------------------------------------
// Public handlers (browse — anonymous)
// ---------------------------------------------------------------------------

// HandleCategories handles GET /calendar/categories — returns the allowed
// calendar categories from the calendar.categories system_setting.
// BUG-009: categories are served via the API (Rule 16), not hardcoded in
// the frontend.
func (h *Handlers) HandleCategories(w http.ResponseWriter, r *http.Request) {
	cats := h.service.Categories(r.Context())
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": cats})
}

// HandleList handles GET /calendar — list published calendar entries with
// optional filters (dest, origin, category, from, to, view). Also accepts
// ?destination_space_id= as legacy alias.
func (h *Handlers) HandleList(w http.ResponseWriter, r *http.Request) {
	var destSpaceID pgtype.UUID

	destParam := r.URL.Query().Get("dest")
	if destParam == "" {
		destParam = r.URL.Query().Get("destination_space_id")
	}

	if destParam != "" {
		if err := destSpaceID.Scan(destParam); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	}

	var originSpaceID pgtype.UUID

	originParam := r.URL.Query().Get("origin")
	if originParam == "" {
		originParam = r.URL.Query().Get("origin_space_id")
	}

	if originParam != "" {
		if err := originSpaceID.Scan(originParam); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid origin space ID")
			return
		}
	}

	category := r.URL.Query().Get("category")

	var from, to pgtype.Timestamptz

	if v := r.URL.Query().Get("from"); v != "" {
		ts, err := parseTimestamp(v)
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_date", "Invalid from date")
			return
		}

		from = ts
	}

	if v := r.URL.Query().Get("to"); v != "" {
		ts, err := parseTimestamp(v)
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_date", "Invalid to date")
			return
		}

		// A date-only "to" filter means "up to and including that day" —
		// extend to end-of-day so entries on the date itself are not
		// silently excluded.
		if isDateOnly(v) {
			ts.Time = ts.Time.Add(24*time.Hour - time.Nanosecond)
		}

		to = ts
	}

	page, limit := parsePageParams(r)

	var userID pgtype.UUID
	if u := auth.FromContext(r.Context()); u != nil {
		userID = u.UserID
	}

	entries, total, err := h.service.List(r.Context(), destSpaceID, originSpaceID, category, from, to, page, limit, userID)
	if err != nil {
		slog.Error("calendar: list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list calendar entries")

		return
	}

	h.resolveLiterals(r, &entries)

	// Report whether the caller is subscribed to the applied
	// destination+category filter so the frontend can render the Subscribe
	// button's persisted state.
	subscribed := false
	if userID.Valid && destSpaceID.Valid {
		subscribed = h.service.IsSubscribed(r.Context(), userID, destSpaceID, category)
	}

	offset := (page - 1) * limit

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": entries,
		"meta": map[string]any{
			"page":        page,
			"page_size":   limit,
			"page_offset": offset,
			"total":       total,
			"subscribed":  subscribed,
		},
		"pagination": map[string]any{
			"page":        page,
			"page_size":   limit,
			"page_offset": offset,
			"total":       total,
		},
	})
}

// HandleListSaved handles GET /me/calendar/saved — returns the calendar
// entries saved by the authenticated user. §86 Integration: "Saved entries
// appear on the user dashboard alongside visa checklists.".
func (h *Handlers) HandleListSaved(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	page, limit := parsePageParams(r)
	if limit > 100 {
		limit = 100
	}

	entries, total, err := h.service.ListSaved(r.Context(), user.UserID, int(page), int(limit))
	if err != nil {
		slog.Error("calendar: list saved failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_saved_failed", "Failed to list saved calendar entries")

		return
	}

	totalPages := int64(0)
	if limit > 0 {
		totalPages = (total + int64(limit) - 1) / int64(limit)
	}

	h.resolveLiterals(r, &entries)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": entries,
		"meta": map[string]any{
			"page":        page,
			"per_page":    limit,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}

// HandleGet handles GET /calendar/:id — single approved calendar entry.
func (h *Handlers) HandleGet(w http.ResponseWriter, r *http.Request) {
	entryID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var userID pgtype.UUID
	if u := auth.FromContext(r.Context()); u != nil {
		userID = u.UserID
	}

	entry, err := h.service.Get(r.Context(), entryID, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "entry_not_found", "Calendar entry not found")
			return
		}

		slog.Error("calendar: get failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to get calendar entry")

		return
	}

	h.resolveLiterals(r, &entry)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": entry})
}

// ---------------------------------------------------------------------------
// Authenticated handlers (submit, save, subscribe)
// ---------------------------------------------------------------------------

// HandleSubmit handles POST /calendar — submit a new calendar entry for
// review. Auth required (calendar.submit permission checked at route
// registration).
func (h *Handlers) HandleSubmit(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	var input SubmitEntryInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.Title == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_title", "Title is required")
		return
	}

	if input.Category == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_category", "Category is required")
		return
	}

	if input.OccursAt == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_occurs_at", "occurs_at is required")
		return
	}
	// §86: source_url is required ("verifiable official source link").
	if input.SourceURL == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_source_url", "source_url is required")
		return
	}

	entryID, err := h.service.Submit(r.Context(), user.UserID, input)
	if err != nil {
		var ve *ValidationError
		if errors.As(err, &ve) {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", ve.Error())
			return
		}

		slog.Error("calendar: submit failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "submit_failed", "Failed to submit calendar entry")

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]any{
			"id":     entryID,
			"status": "pending",
		},
	})
}

// HandleUpdate handles PATCH /calendar/:id — edit an existing calendar entry.
// Requires authentication and either calendar.edit_own (submitter) or
// calendar.edit_any (moderator/admin).
func (h *Handlers) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	entryID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.updater == nil || !h.updater.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "calendar_disabled", "Calendar is not available")
		return
	}

	var input UpdateEntryInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.Title == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_title", "Title is required")
		return
	}

	if input.Category == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_category", "Category is required")
		return
	}

	if input.OccursAt == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_occurs_at", "occurs_at is required")
		return
	}

	if input.SourceURL == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_source_url", "source_url is required")
		return
	}

	entry, err := h.updater.Update(r.Context(), entryID, user.UserID, input)
	if err != nil {
		if errors.Is(err, ErrEditForbidden) {
			errpkg.WriteError(w, http.StatusForbidden, "edit_forbidden", "You do not have permission to edit this calendar entry")
			return
		}

		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "entry_not_found", "Calendar entry not found")
			return
		}

		var ve *ValidationError
		if errors.As(err, &ve) {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", ve.Error())
			return
		}

		slog.Error("calendar: update failed", "id", entryID, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to update calendar entry")

		return
	}

	h.resolveLiterals(r, &entry)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": entry})
}

// HandleSave handles POST /calendar/:id/save — bookmark a calendar entry.
func (h *Handlers) HandleSave(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	entryID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.service.Save(r.Context(), user.UserID, entryID); err != nil {
		slog.Error("calendar: save failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "save_failed", "Failed to save calendar entry")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"saved": true}})
}

// HandleUnsave handles DELETE /calendar/:id/save — remove a bookmark.
func (h *Handlers) HandleUnsave(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	entryID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.service.Unsave(r.Context(), user.UserID, entryID); err != nil {
		slog.Error("calendar: unsave failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "unsave_failed", "Failed to unsave calendar entry")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"saved": false}})
}

// HandleSubscribe handles POST /calendar/subscribe — subscribe to a
// category/destination combination.
func (h *Handlers) HandleSubscribe(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	var input SubscribeInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	// Accept both destination_space_id (canonical) and destination_country
	// (frontend alias).
	destStr := input.DestinationSpaceID
	if destStr == "" {
		destStr = input.DestinationCountry
	}

	if destStr == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_destination", "destination_space_id is required")
		return
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(destStr); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination_space_id")
		return
	}

	if err := h.service.Subscribe(r.Context(), user.UserID, destSpaceID, input.Category); err != nil {
		slog.Error("calendar: subscribe failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "subscribe_failed", "Failed to subscribe")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"subscribed": true}})
}

// HandleUnsubscribe handles DELETE /calendar/subscribe — remove a
// category/destination subscription. Accepts dest/destination_space_id from
// either query params or JSON body (frontend sends a body).
func (h *Handlers) HandleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	destParam := r.URL.Query().Get("dest")
	if destParam == "" {
		destParam = r.URL.Query().Get("destination_space_id")
	}

	category := r.URL.Query().Get("category")

	// If not in query params, try JSON body (frontend sends a body).
	if destParam == "" {
		var body struct {
			DestinationSpaceID string `json:"destination_space_id"`
			DestinationCountry string `json:"destination_country"`
			Category           string `json:"category"`
		}

		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			if body.DestinationSpaceID != "" {
				destParam = body.DestinationSpaceID
			} else if body.DestinationCountry != "" {
				destParam = body.DestinationCountry
			}

			if body.Category != "" {
				category = body.Category
			}
		}
	}

	var destSpaceID pgtype.UUID
	if destParam != "" {
		if err := destSpaceID.Scan(destParam); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	} else {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_destination", "dest or destination_space_id is required")
		return
	}

	if err := h.service.Unsubscribe(r.Context(), user.UserID, destSpaceID, category); err != nil {
		slog.Error("calendar: unsubscribe failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "unsubscribe_failed", "Failed to unsubscribe")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"subscribed": false}})
}

// ---------------------------------------------------------------------------
// Admin handlers (review — calendar.review)
// ---------------------------------------------------------------------------

// HandleAdminList handles GET /admin/calendar — admin list of all calendar
// entries (any status). Supports dest, origin, category, status, q, from, to,
// sort, order, page, page_size params per geo-aware tool pattern.
func (h *Handlers) HandleAdminList(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	var destSpaceID pgtype.UUID

	destParam := r.URL.Query().Get("dest")
	if destParam == "" {
		destParam = r.URL.Query().Get("destination_space_id")
	}

	if destParam != "" {
		if err := destSpaceID.Scan(destParam); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}
	}

	var originSpaceID pgtype.UUID

	originParam := r.URL.Query().Get("origin")
	if originParam == "" {
		originParam = r.URL.Query().Get("origin_space_id")
	}

	if originParam != "" {
		if err := originSpaceID.Scan(originParam); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid origin space ID")
			return
		}
	}

	category := r.URL.Query().Get("category")
	status := r.URL.Query().Get("status")
	q := r.URL.Query().Get("q")
	sort := r.URL.Query().Get("sort")

	deleted := r.URL.Query().Get("deleted")
	if deleted != "" && deleted != "only" && deleted != "all" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_filter", "deleted must be 'only' or 'all'")
		return
	}

	order := r.URL.Query().Get("order")
	if order == "" {
		order = r.URL.Query().Get("dir")
	}

	var from, to pgtype.Timestamptz

	if v := r.URL.Query().Get("from"); v != "" {
		ts, err := parseTimestamp(v)
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_date", "Invalid from date")
			return
		}

		from = ts
	}

	if v := r.URL.Query().Get("to"); v != "" {
		ts, err := parseTimestamp(v)
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_date", "Invalid to date")
			return
		}

		if isDateOnly(v) {
			ts.Time = ts.Time.Add(24*time.Hour - time.Nanosecond)
		}

		to = ts
	}

	page, limit := parsePageParams(r)
	if limit > 200 {
		limit = 200
	}

	entries, total, err := h.service.AdminList(r.Context(), destSpaceID, originSpaceID, category, status, q, deleted, from, to, sort, order, page, limit)
	if err != nil {
		slog.Error("calendar: admin list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "admin_list_failed", "Failed to list calendar entries")

		return
	}

	totalPages := int64(0)
	if limit > 0 {
		totalPages = (total + int64(limit) - 1) / int64(limit)
	}

	// Geo-aware tool pattern response envelope: { data, meta: { page, per_page, total, total_pages } }
	h.resolveLiterals(r, &entries)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": entries,
		"meta": map[string]any{
			"page":        page,
			"per_page":    limit,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}

// HandleAdminUpdate handles PATCH /admin/calendar/:id — publish, archive,
// or mark unverified a calendar entry.
func (h *Handlers) HandleAdminUpdate(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	entryID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var input AdminUpdateEntryInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.Status == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_status", "status is required")
		return
	}

	if err := h.service.AdminUpdate(r.Context(), entryID, user.UserID, input.Status, input.Reason); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "entry_not_found", "Calendar entry not found")
			return
		}
		// BUG-003: ADR-088 HITL gate — AI-generated content requires HITL
		// review before publication. Return 422 so the frontend can show
		// a meaningful error.
		if errors.Is(err, ErrHITLReviewRequired) {
			errpkg.WriteError(w, http.StatusUnprocessableEntity, "hitl_review_required", "AI-generated content requires HITL review before publication")
			return
		}
		// BUG-005: §86 requires a verifiable official source_url.
		if errors.Is(err, ErrSourceURLRequired) {
			errpkg.WriteError(w, http.StatusUnprocessableEntity, "source_url_required", "A verifiable official source URL is required before publication")
			return
		}

		slog.Error("calendar: admin update failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to update calendar entry")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"updated": true}})
}

// HandleAdminDelete handles DELETE /admin/calendar/:id — soft-deletes a
// calendar entry (Rule 11). Gated by calendar.manage (admin+).
// ?force=true permanently removes an already-soft-deleted row, gated by
// admin.hard_delete via rbac.RequireForceDelete.
// Ref: japaconnect-engineering/sections/86-community-calendar.md
// Ref: japaconnect-engineering/AGENTS.md Rule 11.
func (h *Handlers) HandleAdminDelete(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	entryID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if r.URL.Query().Get("force") == "true" {
		err := h.service.AdminHardDelete(r.Context(), entryID)
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "entry_not_found", "Calendar entry not found")
			return
		}

		if err != nil {
			errpkg.HandleServiceError(w, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)

		return
	}

	if err := h.service.AdminSoftDelete(r.Context(), entryID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "entry_not_found", "Calendar entry not found")
			return
		}

		slog.Error("calendar: admin delete failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "delete_failed", "Failed to delete calendar entry")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"deleted": true}})
}
