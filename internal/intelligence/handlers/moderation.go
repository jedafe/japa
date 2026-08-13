package handlers

import (
	"net/http"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/sources"
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

	// OptionalAuth: tenant may or may not be present. When absent, return
	// the directory anyway (it is public).
	_ = r // tenant not required for the public directory

	category := r.URL.Query().Get("category")
	page, limit := parsePagination(r)
	offset := int32((page - 1) * limit)

	var rows []sources.Source
	var err error

	if category != "" {
		rows, err = h.intelSources.ListByCategory(r.Context(), sources.Category(category))
	} else {
		rows, err = h.intelSources.List(r.Context(), gen.ListIntelSourcesParams{
			Limit:  int32(limit),
			Offset: offset,
		})
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "sources_list_failed", err.Error())
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

// --- Review queue handlers (RBAC-gated stubs) ---
//
// The review queue endpoints are admin/control-panel routes gated by
// intel.review.* permissions. The full review-queue UI (approve, reject,
// edit, override) lands in a follow-up task once the review-queue service
// is built. For now these return 501 so the routes are wired and RBAC is
// enforced, but the business logic is deferred.

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

	page, limit := parsePagination(r)
	offset := int32((page - 1) * limit)

	rows, err := h.intelStores.ListIntelRecordsByStatus(r.Context(), gen.ListIntelRecordsByStatusParams{
		EntityType: "scholarship",
		Status:     "review",
		Limit:      int32(limit),
		Offset:     offset,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "review_queue_failed", err.Error())
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

// HandleReviewApprove approves an intel record pending review.
// POST /api/v1/intel/review/{record_id}/approve
func (h *Handlers) HandleReviewApprove(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "not_implemented", "Review approve is not yet implemented")
}

// HandleReviewReject rejects an intel record pending review.
// POST /api/v1/intel/review/{record_id}/reject
func (h *Handlers) HandleReviewReject(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "not_implemented", "Review reject is not yet implemented")
}
