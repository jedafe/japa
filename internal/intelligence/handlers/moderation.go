package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/intelligence/sources"
)

// sourceDirectoryEntry is the public-facing shape for a source in the
// directory. Internal fields (provider_config, robots_policy, error state)
// are omitted; only the fields useful to a public directory listing are
// exposed.
type sourceDirectoryEntry struct {
	ID          string  `json:"id"`
	Slug        string  `json:"slug"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	Domain      string  `json:"domain"`
	SourceURL   string  `json:"source_url"`
	Category    string  `json:"category"`
	// Categories is the multi-category array (intel-channels §6); always
	// serialized (empty → []) so admin create/update responses echo the
	// normalized set that was just written.
	Categories          []string `json:"categories"`
	SourceType          string   `json:"source_type"`
	SourceKind          string   `json:"source_kind"`
	TrustLevel          int16    `json:"trust_level"`
	AttributionRequired bool     `json:"attribution_required"`
	SourceLicense       *string  `json:"source_license,omitempty"`
	UsageRestriction    *string  `json:"usage_restriction,omitempty"`
	LogoURL             *string  `json:"logo_url,omitempty"`
	FollowerCount       int32    `json:"follower_count"`
	PostCount           int32    `json:"post_count"`
	Enabled             bool     `json:"enabled"`
	// EntityTypes is the provider_config.entity_types extraction-domain
	// override — surfaced so the control panel can manage mixed-domain
	// sources without exposing the raw config document.
	EntityTypes []string `json:"entity_types,omitempty"`
}

func sourceToDirectoryEntry(s sources.Source) sourceDirectoryEntry {
	entry := sourceDirectoryEntry{
		Domain:              s.Domain,
		SourceURL:           s.SourceUrl,
		Category:            s.Category,
		SourceType:          s.SourceType,
		SourceKind:          s.SourceKind,
		TrustLevel:          s.TrustLevel,
		AttributionRequired: s.AttributionRequired,
		SourceLicense:       s.SourceLicense,
		UsageRestriction:    s.UsageRestriction,
		LogoURL:             s.LogoUrl,
		FollowerCount:       s.FollowerCount,
		PostCount:           s.PostCount,
		Enabled:             s.Enabled,
		EntityTypes:         sources.EntityTypesFromProviderConfig(s.ProviderConfig),
	}
	if s.ID.Valid {
		entry.ID = s.ID.String()
	}

	entry.Name = s.Name
	if s.Slug != nil {
		entry.Slug = *s.Slug
	}
	entry.Description = s.Description

	entry.Categories = s.Categories
	if entry.Categories == nil {
		entry.Categories = []string{}
	}

	return entry
}

// HandleSourcesDirectory returns the public source directory. Only enabled
// sources are listed; internal crawl state (errors, robots policy, provider
// config) is omitted. The route uses OptionalAuth so anonymous users can
// browse the directory.
// // When the ?include_disabled=true query param is set, ALL sources (including
// disabled) are returned with the full crawl-state fields the control panel
// needs (crawl_method, crawl_frequency, last_crawled_at, etc.). This is used
// by the control panel sources management page so disabled sources are
// visible and re-enableable (BUG-004 fix — the prior endpoint only returned
// enabled sources via the public directory shape, so the control panel could
// never see or manage disabled sources).
// // GET /api/v1/intel/sources.
func (h *Handlers) HandleSourcesDirectory(w http.ResponseWriter, r *http.Request) {
	if h.intelSources == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	// OptionalAuth: platform may or may not be present. When absent, fall
	// back to the default platform so the public directory still renders.

	category := r.URL.Query().Get("category")
	includeDisabled := r.URL.Query().Get("include_disabled") == "true" || r.URL.Query().Get("include_disabled") == "1"
	page, limit := h.parsePagination(r)
	offset := clampInt32((page - 1) * limit)

	var rows []sources.Source

	var err error

	if category != "" {
		rows, err = h.intelSources.ListByCategory(r.Context(), sources.Category(category))
	} else {
		rows, err = h.intelSources.List(r.Context(), gen.ListIntelSourcesParams{
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
		"status":               s.Status,
		"consecutive_failures": s.ConsecutiveFailures,
	}
	if s.ID.Valid {
		entry["id"] = s.ID.String()
	}

	// Editable metadata the control-panel form needs for edit seeding
	// (description/logo/slug/kind are nullable in the DB).
	entry["description"] = nil
	if s.Description != nil {
		entry["description"] = *s.Description
	}

	entry["logo_url"] = nil
	if s.LogoUrl != nil {
		entry["logo_url"] = *s.LogoUrl
	}

	entry["slug"] = nil
	if s.Slug != nil {
		entry["slug"] = *s.Slug
	}

	entry["source_kind"] = s.SourceKind

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

	// The extraction-domain override stored inside provider_config — surfaced
	// as a first-class field so the control panel form can prefill it without
	// exposing the rest of the config document.
	entry["entity_types"] = sources.EntityTypesFromProviderConfig(s.ProviderConfig)

	// intel-channels Phase 1: multi-category + probe state. The proposal
	// review surface shows probe_report + detected_methods so admins can see
	// what the probe discovered before approving/editing channels.
	categories := s.Categories
	if categories == nil {
		categories = []string{}
	}
	entry["categories"] = categories

	keywords := s.Keywords
	if keywords == nil {
		keywords = []string{}
	}
	entry["keywords"] = keywords

	methods := s.DetectedMethods
	if methods == nil {
		methods = []string{}
	}
	entry["detected_methods"] = methods
	entry["probe_report"] = jsonBlobToRawMessage(s.ProbeReport)
	entry["last_probed_at"] = s.LastProbedAt

	entry["proposed_at"] = s.ProposedAt
	if s.ProposedBy.Valid {
		entry["proposed_by"] = s.ProposedBy.String()
	} else {
		entry["proposed_by"] = nil
	}

	entry["approved_at"] = s.ApprovedAt
	if s.ApprovedBy.Valid {
		entry["approved_by"] = s.ApprovedBy.String()
	} else {
		entry["approved_by"] = nil
	}

	return entry
}

// HandleReviewApprove approves an intel record pending review. It sets the
// record status to 'approved' and writes an audit entry to the review log.
// The record is then ready for promotion via the intel:promote task, which
// the control panel can trigger manually or which runs automatically on the
// next crawl cycle (BUG-23 fix).
// // POST /api/v1/intel/review/{record_id}/approve.
func (h *Handlers) HandleReviewApprove(w http.ResponseWriter, r *http.Request) {
	if h.intelStores == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	userID, ok := requireUser(w, r)
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
	if err := h.intelStores.UpdateIntelRecordStatus(r.Context(), gen.UpdateIntelRecordStatusParams{
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

	if err := h.intelStores.CreateIntelReviewLog(r.Context(), gen.CreateIntelReviewLogParams{
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
	if err := h.enqueueIntelPromote(r.Context(), recordID, userID); err != nil {
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
// // POST /api/v1/intel/review/{record_id}/reject.
func (h *Handlers) HandleReviewReject(w http.ResponseWriter, r *http.Request) {
	if h.intelStores == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	userID, ok := requireUser(w, r)
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
	if err := h.intelStores.UpdateIntelRecordStatus(r.Context(), gen.UpdateIntelRecordStatusParams{
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

	if err := h.intelStores.CreateIntelReviewLog(r.Context(), gen.CreateIntelReviewLogParams{
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

// intelRecordResponse is the public-facing shape for an intel record. The
// extracted/confidence/evidence columns are JSON blobs stored as []byte in
// the generated sqlc model; encoding/json base64-encodes []byte fields, which
// would surface as opaque strings (e.g. "e30=" for "{}") to clients. The
// control panel ExtractionPane and Structured Record pane expect parsed JSON
// objects, so these fields are emitted as json.RawMessage to preserve the
// stored JSON shape. All other fields mirror gen.IntelRecord exactly.
type intelRecordResponse struct {
	ID                pgtype.UUID        `json:"id"`
	SourceID          pgtype.UUID        `json:"source_id"`
	RawDocumentID     pgtype.UUID        `json:"raw_document_id"`
	EntityType        string             `json:"entity_type"`
	ExternalKey       string             `json:"external_key"`
	Extracted         json.RawMessage    `json:"extracted"`
	Confidence        json.RawMessage    `json:"confidence"`
	Evidence          json.RawMessage    `json:"evidence"`
	ConfidenceOverall float32            `json:"confidence_overall"`
	Status            string             `json:"status"`
	FreshnessScore    int32              `json:"freshness_score"`
	GeoSpaceIDs       []pgtype.UUID      `json:"geo_space_ids"`
	ExtractedAt       pgtype.Timestamptz `json:"extracted_at"`
	LastVerifiedAt    pgtype.Timestamptz `json:"last_verified_at"`
	PromotedRecordID  pgtype.UUID        `json:"promoted_record_id"`
}

// intelRecordToResponse converts a generated intel record row to the
// public-facing response shape. The three JSON-blob columns are converted
// from []byte to json.RawMessage so they serialize as JSON objects (or null
// when empty) instead of base64 strings. A nil/empty byte slice maps to a
// nil RawMessage, which encoding/json emits as null.
func intelRecordToResponse(r gen.IntelRecord) intelRecordResponse {
	return intelRecordResponse{
		ID:                r.ID,
		SourceID:          r.SourceID,
		RawDocumentID:     r.RawDocumentID,
		EntityType:        r.EntityType,
		ExternalKey:       r.ExternalKey,
		Extracted:         jsonBlobToRawMessage(r.Extracted),
		Confidence:        jsonBlobToRawMessage(r.Confidence),
		Evidence:          jsonBlobToRawMessage(r.Evidence),
		ConfidenceOverall: r.ConfidenceOverall,
		Status:            r.Status,
		FreshnessScore:    r.FreshnessScore,
		GeoSpaceIDs:       r.GeoSpaceIds,
		ExtractedAt:       r.ExtractedAt,
		LastVerifiedAt:    r.LastVerifiedAt,
		PromotedRecordID:  r.PromotedRecordID,
	}
}

// jsonBlobToRawMessage converts a JSON-blob byte slice (as returned by sqlc
// for jsonb columns) into a json.RawMessage. nil or empty slices yield a nil
// RawMessage (serialized as null). Non-empty slices are returned as-is; the
// caller is responsible for ensuring the bytes are valid JSON (the columns
// are jsonb, so the database guarantees validity).
func jsonBlobToRawMessage(b []byte) json.RawMessage {
	if len(b) == 0 {
		return nil
	}

	return json.RawMessage(b)
}

// intelRecordsToResponse converts a slice of generated intel record rows to
// the public-facing response shape (see intelRecordToResponse).
func intelRecordsToResponse(rows []gen.IntelRecord) []intelRecordResponse {
	out := make([]intelRecordResponse, 0, len(rows))
	for i := range rows {
		out = append(out, intelRecordToResponse(rows[i]))
	}

	return out
}

// HandleReviewQueue lists intel records awaiting human review.
// GET /api/v1/intel/review/queue.
func (h *Handlers) HandleReviewQueue(w http.ResponseWriter, r *http.Request) {
	if h.intelStores == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	page, limit := h.parsePagination(r)
	offset := clampInt32((page - 1) * limit)

	// BUG-002 fix: the intel_records.status CHECK constraint (migration 000428)
	// only allows ('extracted','deduped','auto_published','review_required',
	// 'approved','rejected','published','stale'). The prior 'review' value
	// matched nothing, so the moderation queue was always empty.
	// // BUG-001 fix: the prior handler hardcoded EntityType='scholarship', so
	// job/visa/grant records awaiting review were invisible. Now an optional
	// ?entity_type= query param filters by entity type; when omitted, ALL
	// entity types with review_required status are returned.
	entityType := r.URL.Query().Get("entity_type")

	var rows []gen.IntelRecord

	var err error
	if entityType != "" {
		rows, err = h.intelStores.ListIntelRecordsByStatus(r.Context(), gen.ListIntelRecordsByStatusParams{
			EntityType: entityType,
			Status:     "review_required",
			Limit:      clampInt32(limit),
			Offset:     offset,
		})
	} else {
		rows, err = h.intelStores.ListIntelRecordsByStatusAll(r.Context(), gen.ListIntelRecordsByStatusAllParams{
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

	// Total matching rows for pagination — the prior meta only carried the
	// current page length, so the admin table computed totalPages=1 and the
	// queue could never be paged past the first page.
	var total int64
	if entityType != "" {
		total, err = h.intelStores.CountIntelRecordsByStatus(r.Context(), gen.CountIntelRecordsByStatusParams{
			EntityType: entityType,
			Status:     "review_required",
		})
	} else {
		total, err = h.intelStores.CountIntelRecordsByStatusAll(r.Context(), "review_required")
	}

	if err != nil {
		slog.Error("intel: review queue count failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "review_queue_failed", "Failed to load review queue")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": intelRecordsToResponse(rows),
		"meta": map[string]interface{}{
			"page_size":   limit,
			"page_offset": (page - 1) * limit,
			"count":       len(rows),
			"total":       total,
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
// // GET /api/v1/intel/records?status={status}&entity_type={type}
// // Requires: intel.review.view
// Ref: route catalog §2248 (intel records list), BUG-A fix.
func (h *Handlers) HandleListIntelRecords(w http.ResponseWriter, r *http.Request) {
	if h.intelStores == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
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
		rows, err = h.intelStores.ListIntelRecordsByStatus(r.Context(), gen.ListIntelRecordsByStatusParams{
			EntityType: entityType,
			Status:     status,
			Limit:      clampInt32(limit),
			Offset:     offset,
		})
	} else {
		rows, err = h.intelStores.ListIntelRecordsByStatusAll(r.Context(), gen.ListIntelRecordsByStatusAllParams{
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

	var total int64
	if entityType != "" {
		total, err = h.intelStores.CountIntelRecordsByStatus(r.Context(), gen.CountIntelRecordsByStatusParams{
			EntityType: entityType,
			Status:     status,
		})
	} else {
		total, err = h.intelStores.CountIntelRecordsByStatusAll(r.Context(), status)
	}

	if err != nil {
		slog.Error("intel: records count failed", "status", status, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "records_list_failed", "Failed to list intel records")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": intelRecordsToResponse(rows),
		"meta": map[string]interface{}{
			"page_size":   limit,
			"page_offset": (page - 1) * limit,
			"count":       len(rows),
			"total":       total,
			"status":      status,
		},
	})
}

// HandleGetIntelRecord returns a single intel record by id, regardless of
// its current status. The control panel four-pane moderation detail page
// uses this to render a record after it has been approved/rejected/published
// — the prior approach scanned the review_required queue, so any record no
// longer in that queue was unreachable (BUG-B fix).
// // GET /api/v1/intel/records/{id}
// // Requires: intel.review.view
// Ref: route catalog §2249 (intel record detail), BUG-B fix.
func (h *Handlers) HandleGetIntelRecord(w http.ResponseWriter, r *http.Request) {
	handleIntelGetByID(h, w, r, "Intel record not found", "intel record",
		func(ctx context.Context, id pgtype.UUID) (gen.IntelRecord, error) {
			return h.intelStores.GetIntelRecord(ctx, id)
		},
		func(rec gen.IntelRecord) interface{} { return intelRecordToResponse(rec) })
}

// handleIntelGetByID is the shared skeleton for the GET-by-id intel handlers:
// check intelStores, resolve platform, scan the {id} path param, load the
// domain row, and write the response. getByID loads the row and toResp maps
// it to the JSON response shape.
func handleIntelGetByID[E any](h *Handlers, w http.ResponseWriter, r *http.Request, notFoundMsg, entityName string, getByID func(ctx context.Context, id pgtype.UUID) (E, error), toResp func(E) interface{}) {
	if h.intelStores == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	row, err := getByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", notFoundMsg)
			return
		}

		slog.Error("intel: "+entityName+" lookup failed", "id", id.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "lookup_failed", "Failed to load "+entityName)

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": toResp(row),
	})
}
