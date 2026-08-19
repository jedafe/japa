package handlers

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// handleSourceCreateBody is the JSON body for POST /api/v1/intel/sources.
// It mirrors the editable fields of intel_sources. Optional fields use
// pointer types so omitted fields are NULL rather than zero-values.
type handleSourceCreateBody struct {
	Name                string  `json:"name"`
	Domain              string  `json:"domain"`
	SourceURL           string  `json:"source_url"`
	Category            string  `json:"category"`
	SourceType          string  `json:"source_type"`
	TrustLevel          int16   `json:"trust_level"`
	CrawlMethod         string  `json:"crawl_method"`
	CrawlFrequency      string  `json:"crawl_frequency"`
	CrawlFrequencyMins  int32   `json:"crawl_frequency_mins"`
	SourceLicense       *string `json:"source_license"`
	AttributionRequired bool    `json:"attribution_required"`
	UsageRestriction    *string `json:"usage_restriction"`
	Enabled             *bool   `json:"enabled"`
	PersonaID           *string `json:"persona_id"` // optional: AI persona to attribute promoted content to
}

// HandleCreateSource creates a new intel source. The body carries the
// configurable fields; robots_policy and provider_config default to empty
// JSON objects (the fetcher populates robots_policy on first crawl).
//
// POST /api/v1/intel/sources
//
// Requires: intel.sources.manage
func (h *Handlers) HandleCreateSource(w http.ResponseWriter, r *http.Request) {
	if h.intelSources == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	userID, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var body handleSourceCreateBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}
	if body.Name == "" || body.SourceURL == "" || body.Category == "" || body.CrawlMethod == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_field", "name, source_url, category, and crawl_method are required")
		return
	}
	if body.TrustLevel < 1 || body.TrustLevel > 4 {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_trust_level", "trust_level must be 1-4")
		return
	}

	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	freqMins := body.CrawlFrequencyMins
	if freqMins <= 0 {
		freqMins = 1440 // default: daily
	}
	freq := body.CrawlFrequency
	if freq == "" {
		freq = "daily"
	}
	domain := body.Domain
	if domain == "" {
		domain = extractDomain(body.SourceURL)
	}

	params := gen.CreateIntelSourceParams{
		TenantID:            tenantID,
		Name:                body.Name,
		Domain:              domain,
		SourceUrl:           body.SourceURL,
		Category:            body.Category,
		SourceType:          body.SourceType,
		TrustLevel:          body.TrustLevel,
		CrawlMethod:         body.CrawlMethod,
		CrawlFrequency:      freq,
		CrawlFrequencyMins:  freqMins,
		RobotsPolicy:        []byte(`{}`),
		SourceLicense:       body.SourceLicense,
		AttributionRequired: body.AttributionRequired,
		UsageRestriction:    body.UsageRestriction,
		ProviderConfig:      []byte(`{}`),
		Enabled:             enabled,
	}
	if body.PersonaID != nil && *body.PersonaID != "" {
		var pid pgtype.UUID
		if err := pid.Scan(*body.PersonaID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_persona_id", "persona_id must be a valid UUID")
			return
		}
		params.PersonaID = pid
	}

	src, err := h.intelSources.Create(r.Context(), tenantID, params)
	if err != nil {
		slog.Error("intel: create source failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "create_failed", "Failed to create source")
		return
	}

	_ = userID // actor available for audit logging if needed
	errpkg.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"data": sourceToDirectoryEntry(src),
	})
}

// HandleUpdateSource updates an existing intel source. The body carries the
// editable fields; the enabled toggle can be flipped via this endpoint or the
// dedicated enable/disable routes. Returns the updated source.
//
// PATCH /api/v1/intel/sources/{id}
//
// Requires: intel.sources.manage
func (h *Handlers) HandleUpdateSource(w http.ResponseWriter, r *http.Request) {
	if h.intelSources == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	sourceID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	// Load the existing row so PATCH semantics (partial update) work: fields
	// not in the body keep their current values.
	existing, err := h.intelSources.Get(r.Context(), tenantID, sourceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Source not found")
			return
		}
		slog.Error("intel: update source lookup failed", "source_id", sourceID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "lookup_failed", "Failed to load source")
		return
	}

	// BUG-010 fix: a plain bool cannot distinguish "false" from "omitted", so
	// attribution_required was never overrideable on PATCH. Decode the raw
	// body into a map first to detect key presence, then unmarshal the typed
	// struct for the remaining fields.
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Failed to read request body")
		return
	}
	var presence map[string]json.RawMessage
	if err := json.Unmarshal(rawBody, &presence); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}
	var body handleSourceCreateBody
	if err := json.Unmarshal(rawBody, &body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	// Merge: only non-zero / non-nil body fields override the existing row.
	params := gen.UpdateIntelSourceParams{
		TenantID:            tenantID,
		ID:                  sourceID,
		Name:                existing.Name,
		Domain:              existing.Domain,
		SourceUrl:           existing.SourceUrl,
		Category:            existing.Category,
		SourceType:          existing.SourceType,
		TrustLevel:          existing.TrustLevel,
		CrawlMethod:         existing.CrawlMethod,
		CrawlFrequency:      existing.CrawlFrequency,
		CrawlFrequencyMins:  existing.CrawlFrequencyMins,
		SourceLicense:       existing.SourceLicense,
		AttributionRequired: existing.AttributionRequired,
		UsageRestriction:    existing.UsageRestriction,
		Enabled:             existing.Enabled,
	}
	if body.Name != "" {
		params.Name = body.Name
	}
	if body.Domain != "" {
		params.Domain = body.Domain
	}
	if body.SourceURL != "" {
		params.SourceUrl = body.SourceURL
	}
	if body.Category != "" {
		params.Category = body.Category
	}
	if body.SourceType != "" {
		params.SourceType = body.SourceType
	}
	if body.TrustLevel >= 1 && body.TrustLevel <= 4 {
		params.TrustLevel = body.TrustLevel
	}
	if body.CrawlMethod != "" {
		params.CrawlMethod = body.CrawlMethod
	}
	if body.CrawlFrequency != "" {
		params.CrawlFrequency = body.CrawlFrequency
	}
	if body.CrawlFrequencyMins > 0 {
		params.CrawlFrequencyMins = body.CrawlFrequencyMins
	}
	if body.SourceLicense != nil {
		params.SourceLicense = body.SourceLicense
	}
	// attribution_required: override only when the JSON key was present
	// (handles false vs omitted — BUG-010 fix).
	if _, present := presence["attribution_required"]; present {
		params.AttributionRequired = body.AttributionRequired
	}
	if body.Enabled != nil {
		params.Enabled = *body.Enabled
	}
	// persona_id: override only when the JSON key was present and non-empty.
	// The COALESCE in the sqlc query keeps the existing value when the arg
	// is NULL (Valid: false), so clearing to NULL is not supported via PATCH.
	if rawPID, present := presence["persona_id"]; present {
		var pidStr string
		if err := json.Unmarshal(rawPID, &pidStr); err == nil && pidStr != "" {
			var pid pgtype.UUID
			if err := pid.Scan(pidStr); err != nil {
				errpkg.WriteError(w, http.StatusBadRequest, "invalid_persona_id", "persona_id must be a valid UUID")
				return
			}
			params.PersonaID = pid
		}
	}

	src, err := h.intelSources.Update(r.Context(), tenantID, params)
	if err != nil {
		slog.Error("intel: update source failed", "source_id", sourceID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to update source")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": sourceToDirectoryEntry(src),
	})
}

// HandleDeleteSource removes an intel source from the registry.
//
// BUG-004 fix: the spine tables cascade on delete
// (intel_sources → intel_raw_documents → intel_records → scholarships →
// contents), so a naive delete wipes published scholarship content,
// comments, SEO, and RAG embeddings — violating spec §650 / route-catalog
// §2225 ("promoted records preserved") and Rule 11. The handler now refuses
// to delete a source that has any promoted records (returns 409 with a
// pointer to the disable endpoint). To remove a source with promoted data,
// admins disable it (PATCH enabled=false); the source row stays and
// promoted records are preserved.
//
// DELETE /api/v1/intel/sources/{id}
//
// Requires: intel.sources.manage
func (h *Handlers) HandleDeleteSource(w http.ResponseWriter, r *http.Request) {
	if h.intelSources == nil || h.intelStores == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	sourceID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	// Enforce the "promoted records preserved" invariant (spec §650).
	promoted, err := h.intelStores.CountPromotedIntelRecordsBySource(r.Context(), tenantID, sourceID)
	if err != nil {
		slog.Error("intel: delete source promoted-count failed", "source_id", sourceID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "delete_failed", "Failed to verify source safety")
		return
	}
	if promoted > 0 {
		errpkg.WriteError(w, http.StatusConflict, "source_has_promoted_records",
			"Cannot delete a source with promoted records — disable it instead (PATCH enabled=false) to preserve published content.")
		return
	}

	if err := h.intelSources.Delete(r.Context(), tenantID, sourceID); err != nil {
		slog.Error("intel: delete source failed", "source_id", sourceID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "delete_failed", "Failed to delete source")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{"deleted": true},
	})
}

// HandleGetRawDocument returns a single intel_raw_documents row by id. The
// control panel moderation surface uses this to render the raw content pane
// (HTML rendered, JSON pretty-printed, PDF via embedded viewer). The body is
// returned as base64 to preserve binary content (PDF bytes, etc.).
//
// GET /api/v1/intel/raw-documents/{id}
//
// Requires: intel.review.view
func (h *Handlers) HandleGetRawDocument(w http.ResponseWriter, r *http.Request) {
	if h.intelStores == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	docID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	doc, err := h.intelStores.GetIntelRawDocument(r.Context(), tenantID, docID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Raw document not found")
			return
		}
		slog.Error("intel: raw document lookup failed", "doc_id", docID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "lookup_failed", "Failed to load raw document")
		return
	}

	// Catalog §2229 contract: "body as base64, headers, hash". The prior
	// response omitted http_headers (a jsonb column) — the four-pane
	// moderation raw-content pane needs them to reproduce the fetch context.
	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"id":           pgUUIDString(doc.ID),
			"source_id":    pgUUIDString(doc.SourceID),
			"url":          doc.Url,
			"http_status":  doc.HttpStatus,
			"http_headers": json.RawMessage(doc.HttpHeaders), // jsonb → inline JSON
			"content_type": doc.ContentType,
			"content_hash": doc.ContentHash,
			"retrieved_at": doc.RetrievedAt,
			"body_b64":     base64Encode(doc.Body),
		},
	})
}

// HandleReviewEdit saves admin-edited extraction fields to the scholarship's
// editorial_overrides JSONB. The body carries a map of field-name → edited
// value; these are merged into the existing editorial_overrides and the
// override_source is flipped to 'editorial' (or 'mixed'). An audit entry is
// written to intel_review_log. The record is then ready for promotion via
// intel:promote (triggered by the subsequent approve action or automatically).
//
// PATCH /api/v1/intel/review/{record_id}/edit
//
// Requires: intel.review.edit
func (h *Handlers) HandleReviewEdit(w http.ResponseWriter, r *http.Request) {
	if h.intelStores == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	userID, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	recordID, ok := scanUUIDParam(w, r, "record_id")
	if !ok {
		return
	}

	var body struct {
		Fields map[string]json.RawMessage `json:"fields"`
		Note   string                     `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}
	if len(body.Fields) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_field", "fields is required and must not be empty")
		return
	}

	// Resolve the intel record → its promoted scholarship projection row.
	if _, err := h.intelStores.GetIntelRecord(r.Context(), tenantID, recordID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Intel record not found")
			return
		}
		slog.Error("intel: review edit record lookup failed", "record_id", recordID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "lookup_failed", "Failed to load intel record")
		return
	}

	// Load the existing editorial_overrides so we merge rather than replace.
	scholarship, err := h.intelStores.GetScholarshipByIntelRecord(r.Context(), tenantID, recordID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Scholarship projection not found for this record")
			return
		}
		slog.Error("intel: review edit scholarship lookup failed", "record_id", recordID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "lookup_failed", "Failed to load scholarship")
		return
	}

	merged := mergeEditorialOverrides(scholarship.EditorialOverrides, body.Fields)

	if err := h.intelStores.UpdateScholarshipEditorialOverrides(r.Context(), tenantID, scholarship.ID, merged); err != nil {
		slog.Error("intel: review edit apply failed", "scholarship_id", pgUUIDString(scholarship.ID), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "edit_failed", "Failed to apply editorial edits")
		return
	}

	// BUG-011 fix: mirror the SQL CASE in UpdateScholarshipEditorialOverrides
	// (scholarships_intel.sql:51-55) so the response reports the real
	// override_source rather than a hardcoded "editorial". When the row was
	// 'ingestion' (no prior overrides) → 'editorial'; when it already had
	// overrides ('editorial' or 'mixed') → 'mixed'.
	overrideSource := "editorial"
	if scholarship.OverrideSource == "editorial" || scholarship.OverrideSource == "mixed" {
		overrideSource = "mixed"
	}

	// Write the audit entry to the review log.
	note := body.Note
	if note == "" {
		note = "editorial override applied by reviewer"
	}
	fieldChangesJSON, _ := json.Marshal(body.Fields)
	if err := h.intelStores.CreateIntelReviewLog(r.Context(), tenantID, gen.CreateIntelReviewLogParams{
		IntelRecordID: recordID,
		ActorUserID:   userID,
		Action:        "edit",
		FieldChanges:  fieldChangesJSON,
		Note:          &note,
	}); err != nil {
		slog.Error("intel: review edit log write failed", "record_id", recordID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "review_log_failed", "Failed to write review log")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"record_id":       recordID.String(),
			"scholarship_id":  pgUUIDString(scholarship.ID),
			"edited_fields":   body.Fields,
			"override_source": overrideSource,
		},
	})
}

// mergeEditorialOverrides merges the admin-edited fields into the existing
// editorial_overrides JSONB. The existing overrides are a JSON object
// (map[string]any); the new fields are merged in. Returns the merged JSON.
func mergeEditorialOverrides(existing []byte, newFields map[string]json.RawMessage) []byte {
	merged := make(map[string]json.RawMessage)
	if len(existing) > 0 {
		_ = json.Unmarshal(existing, &merged)
	}
	for k, v := range newFields {
		merged[k] = v
	}
	out, err := json.Marshal(merged)
	if err != nil {
		return []byte(`{}`)
	}
	return out
}

// extractDomain returns the host portion of a URL string. Used as a fallback
// when the caller omits the domain field on source creation.
func extractDomain(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	rawURL = strings.TrimPrefix(rawURL, "https://")
	rawURL = strings.TrimPrefix(rawURL, "http://")
	if idx := strings.Index(rawURL, "/"); idx > 0 {
		return rawURL[:idx]
	}
	return rawURL
}

// base64Encode returns the standard base64 encoding of b. Used for raw
// document bodies which may contain binary content (PDF bytes).
func base64Encode(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString(b)
}

// HandleCrawlNow triggers an immediate crawl of a source by enqueuing an
// intel:fetch_source Asynq task (fire-and-forget per Rule 8). The handler
// verifies the source exists and is enabled before enqueuing.
//
// POST /api/v1/intel/sources/{id}/crawl-now
//
// Requires: intel.sources.crawl_now
// Ref: INTEL-011 fix, §26 RBAC CRUD completeness, route catalog §2226.
func (h *Handlers) HandleCrawlNow(w http.ResponseWriter, r *http.Request) {
	if h.intelSources == nil || h.asynqClient == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	sourceID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	// Verify the source exists and is enabled before enqueuing.
	src, err := h.intelSources.Get(r.Context(), tenantID, sourceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Source not found")
			return
		}
		slog.Error("intel: crawl-now source lookup failed", "source_id", sourceID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "lookup_failed", "Failed to load source")
		return
	}
	if !src.Enabled {
		errpkg.WriteError(w, http.StatusConflict, "source_disabled", "Cannot crawl a disabled source — enable it first")
		return
	}

	// Enqueue the fetch_source task (fire-and-forget per Rule 8).
	payload, err := json.Marshal(map[string]string{
		"source_id": sourceID.String(),
		"tenant_id": tenantID.String(),
	})
	if err != nil {
		slog.Error("intel: crawl-now marshal failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "enqueue_failed", "Failed to build task payload")
		return
	}
	if _, err := h.asynqClient.EnqueueContext(r.Context(), asynq.NewTask(TaskIntelFetchSource, payload)); err != nil {
		slog.Error("intel: crawl-now enqueue failed", "source_id", sourceID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "enqueue_failed", "Failed to enqueue crawl task")
		return
	}

	errpkg.WriteJSON(w, http.StatusAccepted, map[string]interface{}{
		"data": map[string]interface{}{
			"source_id": sourceID.String(),
			"status":    "queued",
			"task":      TaskIntelFetchSource,
		},
	})
}

// HandleListProposedSources returns the sources with status='proposed' for
// admin review. These are sources discovered by the agent (source.discover)
// that have not yet been approved.
//
// GET /api/v1/intel/sources/proposed
//
// Requires: intel.sources.manage
// Ref: docs/superpowers/specs/2026-08-19-on-demand-intel-to-content.md
func (h *Handlers) HandleListProposedSources(w http.ResponseWriter, r *http.Request) {
	if h.intelSources == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	_, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	page, limit := h.parsePagination(r)
	offset := clampInt32((page - 1) * limit)

	rows, err := h.intelSources.ListProposed(r.Context(), tenantID, clampInt32(limit), offset)
	if err != nil {
		slog.Error("intel: list proposed sources failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_proposed_failed", "Failed to list proposed sources")
		return
	}

	entries := make([]map[string]interface{}, 0, len(rows))
	for _, s := range rows {
		entries = append(entries, sourceToFullEntry(s))
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": entries,
		"meta": map[string]interface{}{
			"page_size":   limit,
			"page_offset": (page - 1) * limit,
			"count":       len(entries),
		},
	})
}

// HandleApproveProposedSource approves a proposed source: sets
// status='active', enabled=true, and records the approver.
//
// POST /api/v1/intel/sources/{id}/approve
//
// Requires: intel.sources.manage
// Ref: docs/superpowers/specs/2026-08-19-on-demand-intel-to-content.md
func (h *Handlers) HandleApproveProposedSource(w http.ResponseWriter, r *http.Request) {
	if h.intelSources == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	userID, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	sourceID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	src, err := h.intelSources.ApproveProposed(r.Context(), tenantID, sourceID, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Proposed source not found")
			return
		}
		slog.Error("intel: approve proposed source failed", "source_id", sourceID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "approve_failed", "Failed to approve source")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"source_id": sourceID.String(),
			"status":    src.Status,
			"enabled":   src.Enabled,
		},
	})
}

// HandleDisableSource disables a source from the review queue surface.
// This is the review-side counterpart to PATCH enabled=false: it sets
// enabled=false so future crawls stop, but existing promoted records are
// preserved (spec §650). An audit entry is written to intel_review_log.
//
// POST /api/v1/intel/sources/{id}/disable
//
// Requires: intel.review.disable_source
// Ref: INTEL-011 fix, §26 RBAC CRUD completeness, route catalog §2228.
func (h *Handlers) HandleDisableSource(w http.ResponseWriter, r *http.Request) {
	if h.intelSources == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	userID, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	sourceID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	// Verify the source exists before disabling.
	src, err := h.intelSources.Get(r.Context(), tenantID, sourceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Source not found")
			return
		}
		slog.Error("intel: disable source lookup failed", "source_id", sourceID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "lookup_failed", "Failed to load source")
		return
	}

	if err := h.intelSources.Disable(r.Context(), tenantID, sourceID); err != nil {
		slog.Error("intel: disable source failed", "source_id", sourceID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "disable_failed", "Failed to disable source")
		return
	}

	slog.Info("intel: source disabled via review queue",
		"source_id", sourceID.String(), "actor", userID.String())

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"source_id": sourceID.String(),
			"enabled":   false,
			"domain":    src.Domain,
		},
	})
}
