// Package cvformatter — handlers.go provides the HTTP handlers for the
// International CV / Resume Formatter routes (§95, ADR-103).
//
// Routes:
//
//	GET  /api/v1/cv-formatter/standards           — list destination standards (public)
//	GET  /api/v1/cv-formatter/standards/:dest     — single destination standard (public)
//	POST /api/v1/cv-formatter/format              — submit CV for formatting (auth, cv_formatter.use)
//	GET  /api/v1/cv-formatter/sessions/:id        — poll session status + output (auth, owner)
//	GET  /api/v1/cv-formatter/sessions/:id/download — download as PDF or DOCX (auth, owner)
//	GET  /api/v1/admin/cv-formatter/standards     — list standards (admin, cv_formatter.manage)
//	POST /api/v1/admin/cv-formatter/standards     — create/update standard (admin, cv_formatter.manage)
//
// Ref: japaconnect-engineering/sections/95-cv-formatter.md
//
//	japaconnect-engineering/adr/ADR-103-cv-formatter.md
package cvformatter

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/config"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Handlers exposes the CV formatter endpoints.
type Handlers struct {
	service  *Service
	settings *config.Settings
}

// NewHandlers creates CV formatter handlers bound to the provided
// service.
func NewHandlers(service *Service, settings *config.Settings) *Handlers {
	return &Handlers{service: service, settings: settings}
}

// ---------------------------------------------------------------------------
// Shared helpers (same pattern as servicedir/handlers.go)
// ---------------------------------------------------------------------------

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

	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = int32(n) // #nosec
		}
	}

	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = int32(n) // #nosec
		}
	}

	return page, limit
}

// ---------------------------------------------------------------------------
// Public handlers (standards — anonymous browse per §95)
// ---------------------------------------------------------------------------

// HandleListStandards handles GET /cv-formatter/standards — list all
// destination CV standards. Public; returns 404 with cv_formatter_disabled
// when the feature flag is off. Accepts ?dest= to filter by destination
// space ID (geo-aware tool pattern: ?dest= is the primary filter).
func (h *Handlers) HandleListStandards(w http.ResponseWriter, r *http.Request) {
	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "cv_formatter_disabled", "CV formatter is not available")
		return
	}

	standards, err := h.service.ListStandards(r.Context())
	if err != nil {
		slog.Error("cvformatter: list standards failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list standards")

		return
	}

	// Filter by ?dest= destination space ID (geo-aware tool pattern).
	if destParam := r.URL.Query().Get("dest"); destParam != "" {
		var destSpaceID pgtype.UUID
		if err := destSpaceID.Scan(destParam); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid destination space ID")
			return
		}

		filtered := make([]StandardDTO, 0, 1)

		for _, s := range standards {
			if s.DestinationSpaceID == destSpaceID.String() {
				filtered = append(filtered, s)
			}
		}

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": filtered})

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": standards})
}

// HandleGetStandard handles GET /cv-formatter/standards/:dest — single
// destination CV standard. Public; :dest is the destination_space_id.
func (h *Handlers) HandleGetStandard(w http.ResponseWriter, r *http.Request) {
	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "cv_formatter_disabled", "CV formatter is not available")
		return
	}

	destSpaceID, ok := scanUUIDParam(w, r, "dest")
	if !ok {
		return
	}

	standard, err := h.service.GetStandardByDest(r.Context(), destSpaceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "standard_not_found", "CV standard not found for this destination")
			return
		}

		slog.Error("cvformatter: get standard failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to get standard")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": standard})
}

// ---------------------------------------------------------------------------
// Authenticated handler (format — auth + quota required)
// ---------------------------------------------------------------------------

// HandleFormat handles POST /cv-formatter/format — submit a CV for
// formatting. Auth required (cv_formatter.use permission checked at route
// registration). Quota-checked via CheckQuota. Creates a session with
// status='pending' and returns 202 Accepted. The actual AI processing is
// an Asynq task (not implemented here).
//
// Accepts two content types:
//   - application/json: body with input_text (pasted text)
//   - multipart/form-data: file upload (PDF or DOCX) with form fields
//     destination_space_id, origin_space_id (optional), output_format (optional)
func (h *Handlers) HandleFormat(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "cv_formatter_disabled", "CV formatter is not available")
		return
	}

	var input FormatInput

	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		// File upload path — parse multipart form and extract text.
		r.Body = http.MaxBytesReader(w, r.Body, MaxUploadBytes)
		// #nosec G120 // total body size is bounded by MaxBytesReader above.
		if err := r.ParseMultipartForm(32 << 10); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_form", "Failed to parse multipart form (max 10 MB)")
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "missing_file", "file is required for multipart uploads")
			return
		}

		defer file.Close()

		// Validate file size.
		if header.Size > MaxUploadBytes {
			errpkg.WriteError(w, http.StatusBadRequest, "file_too_large", "File exceeds 10 MB limit")
			return
		}

		// Determine MIME type from the file header (not just the extension).
		mimeType := header.Header.Get("Content-Type")
		if mimeType == "" {
			mimeType = detectMimeType(header.Filename)
		}

		if !AllowedMimeTypes[mimeType] {
			errpkg.WriteError(w, http.StatusBadRequest, "unsupported_type",
				"Only PDF and DOCX files are accepted")
			return
		}

		// Read the file body.
		body, err := io.ReadAll(io.LimitReader(file, MaxUploadBytes+1))
		if err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "read_failed", "Failed to read uploaded file")
			return
		}

		if int64(len(body)) > MaxUploadBytes {
			errpkg.WriteError(w, http.StatusBadRequest, "file_too_large", "File exceeds 10 MB limit")
			return
		}

		// Extract text from the file.
		extracted, err := ExtractTextFromFile(body, mimeType)
		if err != nil {
			slog.Error("cvformatter: text extraction failed", "filename", header.Filename, "error", err)
			errpkg.WriteError(w, http.StatusBadRequest, "extraction_failed",
				"Failed to extract text from file. Ensure it is a valid PDF or DOCX.")

			return
		}

		if strings.TrimSpace(extracted) == "" {
			errpkg.WriteError(w, http.StatusBadRequest, "empty_text",
				"No text could be extracted from the file. If it is a scanned PDF, paste the text manually.")
			return
		}

		// Populate input from form fields + extracted text.
		input.DestinationSpaceID = r.FormValue("destination_space_id")
		input.OriginSpaceID = r.FormValue("origin_space_id")
		input.OutputFormat = r.FormValue("output_format")
		input.AIModel = r.FormValue("ai_model")
		input.InputText = extracted
		filename := header.Filename
		size := header.Size
		input.OriginalFilename = &filename
		input.FileMimeType = &mimeType
		input.FileSizeBytes = &size
		// FileStoragePath is nil — we don't persist the raw file to S3
		// in this iteration; the extracted text is sufficient for AI
		// processing. A future iteration can upload to SeaweedFS.
	} else {
		// JSON path — text pasted directly.
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return
		}
	}

	if input.DestinationSpaceID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_destination", "destination_space_id is required")
		return
	}

	if input.InputText == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_input", "input_text or a file is required")
		return
	}

	if err := h.service.CheckQuota(r.Context(), user.UserID); err != nil {
		if errors.Is(err, ErrQuotaExceeded) {
			errpkg.WriteError(w, http.StatusTooManyRequests, "quota_exceeded", "Monthly CV formatting quota reached")
			return
		}

		slog.Error("cvformatter: quota check failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "quota_check_failed", "Failed to check quota")

		return
	}

	session, err := h.service.CreateSession(r.Context(), user.UserID, input)
	if err != nil {
		slog.Error("cvformatter: create session failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "create_failed", "Failed to create format session")

		return
	}

	errpkg.WriteJSON(w, http.StatusAccepted, map[string]any{"data": session})
}

// ---------------------------------------------------------------------------
// Authenticated handlers (session poll + download — auth + owner only)
// ---------------------------------------------------------------------------

// HandleGetSession handles GET /cv-formatter/sessions/:id — poll session
// status and retrieve output. Auth required; owner only.
func (h *Handlers) HandleGetSession(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	sessionID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "cv_formatter_disabled", "CV formatter is not available")
		return
	}

	row, err := h.service.GetSession(r.Context(), sessionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "session_not_found", "Format session not found")
			return
		}

		slog.Error("cvformatter: get session failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to get session")

		return
	}

	if row.UserID != user.UserID {
		errpkg.WriteError(w, http.StatusForbidden, "not_owner", "You can only access your own format sessions")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": sessionRowToDTO(row)})
}

// HandleDownload handles GET /cv-formatter/sessions/:id/download —
// download the formatted CV as a file. Auth required; owner only. The
// response body is the output_text with a Content-Type header matching
// the requested format (docx or pdf). Returns 409 if the session is not
// yet complete or has failed.
func (h *Handlers) HandleDownload(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	sessionID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if h.service == nil || !h.service.Enabled(r.Context()) {
		errpkg.WriteError(w, http.StatusNotFound, "cv_formatter_disabled", "CV formatter is not available")
		return
	}

	row, err := h.service.GetSession(r.Context(), sessionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "session_not_found", "Format session not found")
			return
		}

		slog.Error("cvformatter: download get session failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to get session")

		return
	}

	if row.UserID != user.UserID {
		errpkg.WriteError(w, http.StatusForbidden, "not_owner", "You can only access your own format sessions")
		return
	}

	if row.Status != SessionStatusComplete {
		errpkg.WriteError(w, http.StatusConflict, "not_ready", "Session output is not ready (status: "+row.Status+")")
		return
	}

	if row.OutputText == nil {
		errpkg.WriteError(w, http.StatusConflict, "no_output", "Session has no output text")
		return
	}

	// Generate the actual document file (DOCX or PDF) from the AI output
	// text. The output_text is plain text from the AI Gateway; we convert
	// it to a properly formatted document for download.
	var fileBytes []byte

	var contentType string

	var ext string

	if row.OutputFormat == OutputFormatPDF {
		contentType = "application/pdf"
		ext = "pdf"

		var err error

		fileBytes, err = GeneratePDF(*row.OutputText)
		if err != nil {
			slog.Error("cvformatter: generate PDF failed", "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "generate_failed", "Failed to generate PDF")

			return
		}
	} else {
		contentType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
		ext = "docx"

		var err error

		fileBytes, err = GenerateDOCX(*row.OutputText)
		if err != nil {
			slog.Error("cvformatter: generate DOCX failed", "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "generate_failed", "Failed to generate DOCX")

			return
		}
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", "attachment; filename=\"cv-formatted."+ext+"\"")
	w.Header().Set("Content-Length", strconv.Itoa(len(fileBytes)))
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write(fileBytes); err != nil {
		slog.Error("cvformatter: write download body", "error", err)
	}
}

// ---------------------------------------------------------------------------
// Admin handlers (standards management — cv_formatter.manage permission)
// ---------------------------------------------------------------------------

// HandleAdminListStandards handles GET /admin/cv-formatter/standards —
// list all CV standards for management. Admin-only (cv_formatter.manage
// permission checked at route registration).
func (h *Handlers) HandleAdminListStandards(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "cv_formatter_unavailable", "CV formatter not configured")
		return
	}

	// Admin list works even if the public feature flag is off, so
	// administrators can configure standards before enabling the feature.

	standards, err := h.service.ListStandards(r.Context())
	if err != nil {
		slog.Error("cvformatter: admin list standards failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list standards")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": standards})
}

// HandleAdminUpsertStandard handles POST /admin/cv-formatter/standards —
// create or update a CV standard for a destination space. Admin-only
// (cv_formatter.manage permission checked at route registration).
func (h *Handlers) HandleAdminUpsertStandard(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	if h.service == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "cv_formatter_unavailable", "CV formatter not configured")
		return
	}

	var input StandardInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if input.DestinationSpaceID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_destination", "destination_space_id is required")
		return
	}

	standard, err := h.service.UpsertStandard(r.Context(), input)
	if err != nil {
		slog.Error("cvformatter: admin upsert standard failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "upsert_failed", "Failed to create or update standard")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": standard})
}

// detectMimeType infers the MIME type from the filename extension when
// the Content-Type header is missing or generic.
func detectMimeType(filename string) string {
	lower := strings.ToLower(filename)

	switch {
	case strings.HasSuffix(lower, ".pdf"):
		return "application/pdf"
	case strings.HasSuffix(lower, ".docx"):
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	default:
		return ""
	}
}
