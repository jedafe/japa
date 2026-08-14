package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/intelligence/sources"
	"github.com/jedafe/japa/internal/tenant"
)

// sourceDirectoryEntry is the public-facing shape for a source in the
// directory. Internal fields (provider_config, robots_policy, error state)
// are omitted; only the fields useful to a public directory listing are
// exposed.
type sourceDirectoryEntry struct {
	ID                  string  `json:"id"`
	Name                string  `json:"name"`
	Domain              string  `json:"domain"`
	SourceURL           string  `json:"source_url"`
	Category            string  `json:"category"`
	SourceType          string  `json:"source_type"`
	TrustLevel          int16   `json:"trust_level"`
	AttributionRequired bool    `json:"attribution_required"`
	SourceLicense       *string `json:"source_license,omitempty"`
	UsageRestriction    *string `json:"usage_restriction,omitempty"`
	Enabled             bool    `json:"enabled"`
}

func sourceToDirectoryEntry(s sources.Source) sourceDirectoryEntry {
	entry := sourceDirectoryEntry{
		Domain:              s.Domain,
		SourceURL:           s.SourceUrl,
		Category:            s.Category,
		SourceType:          s.SourceType,
		TrustLevel:          s.TrustLevel,
		AttributionRequired: s.AttributionRequired,
		SourceLicense:       s.SourceLicense,
		UsageRestriction:    s.UsageRestriction,
		Enabled:             s.Enabled,
	}
	if s.ID.Valid {
		entry.ID = s.ID.String()
	}
	entry.Name = s.Name
	return entry
}

// HandleSourcesDirectory returns the public source directory. Only enabled
// sources are listed; internal crawl state (errors, robots policy, provider
// config) is omitted. The route uses OptionalAuth so anonymous users can
// browse the directory.
//
// When the ?include_disabled=true query param is set, ALL sources (including
// disabled) are returned with the full crawl-state fields the control panel
// needs (crawl_method, crawl_frequency, last_crawled_at, etc.). This is used
// by the control panel sources management page so disabled sources are
// visible and re-enableable (BUG-004 fix — the prior endpoint only returned
// enabled sources via the public directory shape, so the control panel could
// never see or manage disabled sources).
//
// GET /api/v1/intel/sources
func (h *Handlers) HandleSourcesDirectory(w http.ResponseWriter, r *http.Request) {
	if h.intelSources == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	// OptionalAuth: tenant may or may not be present. When absent, fall
	// back to the default tenant so the public directory still renders.
	tenantID := tenant.IDFromContext(r.Context())
	if !tenantID.Valid {
		tenantID = defaultIntelTenantID
	}

	category := r.URL.Query().Get("category")
	includeDisabled := r.URL.Query().Get("include_disabled") == "true" || r.URL.Query().Get("include_disabled") == "1"
	page, limit := h.parsePagination(r)
	offset := clampInt32((page - 1) * limit)

	var rows []sources.Source
	var err error

	if category != "" {
		rows, err = h.intelSources.ListByCategory(r.Context(), tenantID, sources.Category(category))
	} else {
		rows, err = h.intelSources.List(r.Context(), tenantID, gen.ListIntelSourcesParams{
			Limit:  clampInt32(limit),
			Offset: offset,
		})
	}
	if err != nil {
		slog.Error("intel: sources directory list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "sources_list_failed", "Failed to list sources")
		return
	}

	// When the control panel requests all sources (include_disabled), return
	// the full source row shape so crawl-state fields are available. Otherwise
	// return the trimmed public directory entry (enabled sources only).
	if includeDisabled {
		fullEntries := make([]map[string]interface{}, 0, len(rows))
		for _, s := range rows {
			fullEntries = append(fullEntries, sourceToFullEntry(s))
		}
		errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"data": fullEntries,
			"meta": map[string]interface{}{
				"page_size":   limit,
				"page_offset": (page - 1) * limit,
				"count":       len(fullEntries),
			},
		})
		return
	}

	entries := make([]sourceDirectoryEntry, 0, len(rows))
	for _, s := range rows {
		// Only expose enabled sources in the public directory.
		if !s.Enabled {
			continue
		}
		entries = append(entries, sourceToDirectoryEntry(s))
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

// sourceToFullEntry returns the full source row as a map for the control
// panel's sources management page. Unlike sourceToDirectoryEntry, this
// includes crawl-state fields (crawl_method, crawl_frequency,
// crawl_frequency_mins, last_crawled_at, last_success_at, last_error_at,
// last_error_message, consecutive_failures, created_at) that the control
// panel table and freshness-queue computation require (BUG-004 fix).
func sourceToFullEntry(s sources.Source) map[string]interface{} {
	entry := map[string]interface{}{
		"name":                 s.Name,
		"domain":               s.Domain,
		"source_url":           s.SourceUrl,
		"category":             s.Category,
		"source_type":          s.SourceType,
		"trust_level":          s.TrustLevel,
		"crawl_method":         s.CrawlMethod,
		"crawl_frequency":      s.CrawlFrequency,
		"crawl_frequency_mins": s.CrawlFrequencyMins,
		"attribution_required": s.AttributionRequired,
		"enabled":              s.Enabled,
		"consecutive_failures": s.ConsecutiveFailures,
	}
	if s.ID.Valid {
		entry["id"] = s.ID.String()
	}
	if s.SourceLicense != nil {
		entry["source_license"] = *s.SourceLicense
	} else {
		entry["source_license"] = nil
	}
	if s.UsageRestriction != nil {
		entry["usage_restriction"] = *s.UsageRestriction
	} else {
		entry["usage_restriction"] = nil
	}
	if s.LastErrorMessage != nil {
		entry["last_error_message"] = *s.LastErrorMessage
	} else {
		entry["last_error_message"] = nil
	}
	entry["last_crawled_at"] = s.LastCrawledAt
	entry["last_success_at"] = s.LastSuccessAt
	entry["last_error_at"] = s.LastErrorAt
	entry["created_at"] = s.CreatedAt
	if s.PersonaID.Valid {
		entry["persona_id"] = s.PersonaID.String()
	} else {
		entry["persona_id"] = nil
	}
	return entry
}

// HandleReviewApprove approves an intel record pending review. It sets the
// record status to 'approved' and writes an audit entry to the review log.
// The record is then ready for promotion via the intel:promote task, which
// the control panel can trigger manually or which runs automatically on the
// next crawl cycle (BUG-23 fix).
//
// POST /api/v1/intel/review/{record_id}/approve
func (h *Handlers) HandleReviewApprove(w http.ResponseWriter, r *http.Request) {
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
		Note string `json:"note"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body) // note is optional
	}

	// Set the record status to 'approved'.
	if err := h.intelStores.UpdateIntelRecordStatus(r.Context(), tenantID, gen.UpdateIntelRecordStatusParams{
		ID:     recordID,
		Status: "approved",
	}); err != nil {
		slog.Error("intel: approve status update failed", "record_id", recordID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "approve_failed", "Failed to approve record")
		return
	}

	// Write the audit entry to the review log.
	note := body.Note
	if note == "" {
		note = "approved by reviewer"
	}
	if err := h.intelStores.CreateIntelReviewLog(r.Context(), tenantID, gen.CreateIntelReviewLogParams{
		IntelRecordID: recordID,
		ActorUserID:   userID,
		Action:        "approve",
		FieldChanges:  []byte(`[]`),
		Note:          &note,
	}); err != nil {
		slog.Error("intel: approve review log write failed", "record_id", recordID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "review_log_failed", "Failed to write review log")
		return
	}

	// BUG-003 fix: enqueue intel:promote so the approved record is actually
	// published to scholarships + contents (spec §647, route catalog §2237).
	// Without this the approval is silently lost — re-crawl creates a NEW
	// record rather than promoting the approved one. Fire-and-forget per
	// Rule 8; an enqueue failure is logged but does not roll back the
	// approval (the control panel can re-trigger promotion manually).
	if err := h.enqueueIntelPromote(r.Context(), tenantID, recordID, userID); err != nil {
		slog.Error("intel: approve enqueue promote failed", "record_id", recordID.String(), "error", err)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"record_id": recordID.String(),
			"status":    "approved",
		},
	})
}

// HandleReviewReject rejects an intel record pending review. It sets the
// record status to 'rejected' and writes an audit entry to the review log.
// The record is not promoted (BUG-23 fix).
//
// POST /api/v1/intel/review/{record_id}/reject
func (h *Handlers) HandleReviewReject(w http.ResponseWriter, r *http.Request) {
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
		Note string `json:"note"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body) // note is optional
	}

	// Set the record status to 'rejected'.
	if err := h.intelStores.UpdateIntelRecordStatus(r.Context(), tenantID, gen.UpdateIntelRecordStatusParams{
		ID:     recordID,
		Status: "rejected",
	}); err != nil {
		slog.Error("intel: reject status update failed", "record_id", recordID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "reject_failed", "Failed to reject record")
		return
	}

	// Write the audit entry to the review log.
	note := body.Note
	if note == "" {
		note = "rejected by reviewer"
	}
	if err := h.intelStores.CreateIntelReviewLog(r.Context(), tenantID, gen.CreateIntelReviewLogParams{
		IntelRecordID: recordID,
		ActorUserID:   userID,
		Action:        "reject",
		FieldChanges:  []byte(`[]`),
		Note:          &note,
	}); err != nil {
		slog.Error("intel: reject review log write failed", "record_id", recordID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "review_log_failed", "Failed to write review log")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"record_id": recordID.String(),
			"status":    "rejected",
		},
	})
}

// HandleReviewQueue lists intel records awaiting human review.
// GET /api/v1/intel/review/queue
func (h *Handlers) HandleReviewQueue(w http.ResponseWriter, r *http.Request) {
	if h.intelStores == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	if _, ok := requireTenant(w, r); !ok {
		return
	}
	tenantID := tenant.IDFromContext(r.Context())
	if !tenantID.Valid {
		tenantID = defaultIntelTenantID
	}

	page, limit := h.parsePagination(r)
	offset := clampInt32((page - 1) * limit)

	// BUG-002 fix: the intel_records.status CHECK constraint (migration 000428)
	// only allows ('extracted','deduped','auto_published','review_required',
	// 'approved','rejected','published','stale'). The prior 'review' value
	// matched nothing, so the moderation queue was always empty.
	//
	// BUG-001 fix: the prior handler hardcoded EntityType='scholarship', so
	// job/visa/grant records awaiting review were invisible. Now an optional
	// ?entity_type= query param filters by entity type; when omitted, ALL
	// entity types with review_required status are returned.
	entityType := r.URL.Query().Get("entity_type")

	var rows []gen.IntelRecord
	var err error
	if entityType != "" {
		rows, err = h.intelStores.ListIntelRecordsByStatus(r.Context(), tenantID, gen.ListIntelRecordsByStatusParams{
			EntityType: entityType,
			Status:     "review_required",
			Limit:      clampInt32(limit),
			Offset:     offset,
		})
	} else {
		rows, err = h.intelStores.ListIntelRecordsByStatusAll(r.Context(), tenantID, gen.ListIntelRecordsByStatusAllParams{
			Status: "review_required",
			Limit:  clampInt32(limit),
			Offset: offset,
		})
	}
	if err != nil {
		slog.Error("intel: review queue list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "review_queue_failed", "Failed to load review queue")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": rows,
		"meta": map[string]interface{}{
			"page_size":   limit,
			"page_offset": (page - 1) * limit,
			"count":       len(rows),
		},
	})
}

// validIntelRecordStatuses mirrors the intel_records.status CHECK
// constraint (migration 000428). Used to validate the ?status= query param
// on the records list endpoint so an unknown value returns 400 rather than
// a silently-empty result.
var validIntelRecordStatuses = map[string]bool{
	"extracted":       true,
	"deduped":         true,
	"auto_published":  true,
	"review_required": true,
	"approved":        true,
	"rejected":        true,
	"published":       true,
	"stale":           true,
}

// HandleListIntelRecords lists intel records filtered by status (and
// optionally entity_type), newest extraction first. Unlike
// HandleReviewQueue (which is hardcoded to review_required), this endpoint
// accepts an arbitrary ?status= value so the control panel can browse
// records in any pipeline stage (approved, published, rejected, stale,
// etc.). The control panel review list page and the intelligence overview
// page both call this endpoint for non-review_required statuses.
//
// GET /api/v1/intel/records?status={status}&entity_type={type}
//
// Requires: intel.review.view
// Ref: route catalog §2248 (intel records list), BUG-A fix.
func (h *Handlers) HandleListIntelRecords(w http.ResponseWriter, r *http.Request) {
	if h.intelStores == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	if _, ok := requireTenant(w, r); !ok {
		return
	}
	tenantID := tenant.IDFromContext(r.Context())
	if !tenantID.Valid {
		tenantID = defaultIntelTenantID
	}

	status := r.URL.Query().Get("status")
	if status == "" {
		status = "review_required"
	}
	if !validIntelRecordStatuses[status] {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_status", "status must be one of: extracted, deduped, auto_published, review_required, approved, rejected, published, stale")
		return
	}

	entityType := r.URL.Query().Get("entity_type")

	page, limit := h.parsePagination(r)
	offset := clampInt32((page - 1) * limit)

	var rows []gen.IntelRecord
	var err error
	if entityType != "" {
		rows, err = h.intelStores.ListIntelRecordsByStatus(r.Context(), tenantID, gen.ListIntelRecordsByStatusParams{
			EntityType: entityType,
			Status:     status,
			Limit:      clampInt32(limit),
			Offset:     offset,
		})
	} else {
		rows, err = h.intelStores.ListIntelRecordsByStatusAll(r.Context(), tenantID, gen.ListIntelRecordsByStatusAllParams{
			Status: status,
			Limit:  clampInt32(limit),
			Offset: offset,
		})
	}
	if err != nil {
		slog.Error("intel: records list failed", "status", status, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "records_list_failed", "Failed to list intel records")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": rows,
		"meta": map[string]interface{}{
			"page_size":   limit,
			"page_offset": (page - 1) * limit,
			"count":       len(rows),
			"status":      status,
		},
	})
}

// HandleGetIntelRecord returns a single intel record by id, regardless of
// its current status. The control panel four-pane moderation detail page
// uses this to render a record after it has been approved/rejected/published
// — the prior approach scanned the review_required queue, so any record no
// longer in that queue was unreachable (BUG-B fix).
//
// GET /api/v1/intel/records/{id}
//
// Requires: intel.review.view
// Ref: route catalog §2249 (intel record detail), BUG-B fix.
func (h *Handlers) HandleGetIntelRecord(w http.ResponseWriter, r *http.Request) {
	if h.intelStores == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	if _, ok := requireTenant(w, r); !ok {
		return
	}
	tenantID := tenant.IDFromContext(r.Context())
	if !tenantID.Valid {
		tenantID = defaultIntelTenantID
	}

	recordID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	rec, err := h.intelStores.GetIntelRecord(r.Context(), tenantID, recordID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Intel record not found")
			return
		}
		slog.Error("intel: record lookup failed", "record_id", recordID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "lookup_failed", "Failed to load intel record")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": rec,
	})
}
