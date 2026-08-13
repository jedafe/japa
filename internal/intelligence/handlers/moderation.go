package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/sources"
	"github.com/jedafe/japa/internal/tenant"
)

// sourceDirectoryEntry is the public-facing shape for a source in the
// directory. Internal fields (provider_config, robots_policy, error state)
// are omitted; only the fields useful to a public directory listing are
// exposed.
type sourceDirectoryEntry struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	Domain            string  `json:"domain"`
	SourceURL         string  `json:"source_url"`
	Category          string  `json:"category"`
	SourceType        string  `json:"source_type"`
	TrustLevel        int16   `json:"trust_level"`
	AttributionRequired bool   `json:"attribution_required"`
	SourceLicense     *string `json:"source_license,omitempty"`
	UsageRestriction  *string `json:"usage_restriction,omitempty"`
	Enabled           bool    `json:"enabled"`
}

func sourceToDirectoryEntry(s sources.Source) sourceDirectoryEntry {
	entry := sourceDirectoryEntry{
		Domain:             s.Domain,
		SourceURL:          s.SourceUrl,
		Category:           s.Category,
		SourceType:         s.SourceType,
		TrustLevel:         s.TrustLevel,
		AttributionRequired: s.AttributionRequired,
		SourceLicense:      s.SourceLicense,
		UsageRestriction:   s.UsageRestriction,
		Enabled:            s.Enabled,
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
// GET /api/v1/intel/sources
func (h *Handlers) HandleSourcesDirectory(w http.ResponseWriter, r *http.Request) {
	if h.intelSources == nil {
		writeError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	// OptionalAuth: tenant may or may not be present. When absent, fall
	// back to the default tenant so the public directory still renders.
	tenantID := tenant.IDFromContext(r.Context())
	if !tenantID.Valid {
		tenantID = defaultIntelTenantID
	}

	category := r.URL.Query().Get("category")
	page, limit := parsePagination(r)
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
		writeError(w, http.StatusInternalServerError, "sources_list_failed", "Failed to list sources")
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

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": entries,
		"meta": map[string]interface{}{
			"page_size":   limit,
			"page_offset": (page - 1) * limit,
			"count":       len(entries),
		},
	})
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
		writeError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
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
		writeError(w, http.StatusInternalServerError, "approve_failed", "Failed to approve record")
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
		writeError(w, http.StatusInternalServerError, "review_log_failed", "Failed to write review log")
		return
	}

	// BUG-003 fix: enqueue intel:promote so the approved record is actually
	// published to scholarships + contents (spec §647, route catalog §2237).
	// Without this the approval is silently lost — re-crawl creates a NEW
	// record rather than promoting the approved one. Fire-and-forget per
	// Rule 8; an enqueue failure is logged but does not roll back the
	// approval (the control panel can re-trigger promotion manually).
	if err := h.enqueueIntelPromote(r.Context(), tenantID, recordID); err != nil {
		slog.Error("intel: approve enqueue promote failed", "record_id", recordID.String(), "error", err)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
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
		writeError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
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
		writeError(w, http.StatusInternalServerError, "reject_failed", "Failed to reject record")
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
		writeError(w, http.StatusInternalServerError, "review_log_failed", "Failed to write review log")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
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
		writeError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	if _, ok := requireTenant(w, r); !ok {
		return
	}
	tenantID := tenant.IDFromContext(r.Context())
	if !tenantID.Valid {
		tenantID = defaultIntelTenantID
	}

	page, limit := parsePagination(r)
	offset := clampInt32((page - 1) * limit)

	// BUG-002 fix: the intel_records.status CHECK constraint (migration 000428)
	// only allows ('extracted','deduped','auto_published','review_required',
	// 'approved','rejected','published','stale'). The prior 'review' value
	// matched nothing, so the moderation queue was always empty.
	rows, err := h.intelStores.ListIntelRecordsByStatus(r.Context(), tenantID, gen.ListIntelRecordsByStatusParams{
		EntityType: "scholarship",
		Status:     "review_required",
		Limit:      clampInt32(limit),
		Offset:     offset,
	})
	if err != nil {
		slog.Error("intel: review queue list failed", "error", err)
		writeError(w, http.StatusInternalServerError, "review_queue_failed", "Failed to load review queue")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": rows,
		"meta": map[string]interface{}{
			"page_size":   limit,
			"page_offset": (page - 1) * limit,
			"count":       len(rows),
		},
	})
}
