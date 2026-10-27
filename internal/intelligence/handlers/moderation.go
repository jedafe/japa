package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/intelligence/extractor/schemas"
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

// HandleSourcesDirectory serves the control-panel source registry and,
// without include_disabled, the enabled+active subset. It is registered on
// the authenticated /intel/sources route behind intel.sources.view|manage —
// the public anonymous directory is HandlePublicSourcesDirectory on
// /sources. Internal crawl state (errors, robots policy, provider config)
// is omitted from the response shape.
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

	q := r.URL.Query()
	category := q.Get("category")
	includeDisabled := q.Get("include_disabled") == "true" || q.Get("include_disabled") == "1"
	page, limit := h.parsePagination(r)
	offset := clampInt32((page - 1) * limit)

	// Server-side filtering/pagination for both surfaces: the public shape
	// filters enabled=true; include_disabled (control panel registry) sees
	// everything. ?q= searches name/domain/source_url/keywords, ?status= and
	// ?sort=/dir= (whitelisted) apply to the registry view.
	listParams := gen.ListIntelSourcesFilteredParams{
		Search:    q.Get("q"),
		Category:  category,
		SortKey:   q.Get("sort"),
		SortDir:   q.Get("dir"),
		RowLimit:  clampInt32(limit),
		RowOffset: offset,
	}

	if !includeDisabled {
		enabledOnly := true
		listParams.Enabled = &enabledOnly
		listParams.Status = "active"
	} else {
		listParams.Status = q.Get("status")
	}

	if enabledParam := q.Get("enabled"); includeDisabled && (enabledParam == "true" || enabledParam == "false") {
		v := enabledParam == "true"
		listParams.Enabled = &v
	}

	if _, ok := intelSourceSortKeys[listParams.SortKey]; !ok {
		listParams.SortKey = ""
	}

	if listParams.SortDir != "desc" {
		listParams.SortDir = "asc"
	}

	rows, err := h.intelSources.ListFiltered(r.Context(), listParams)
	if err != nil {
		slog.Error("intel: sources directory list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "sources_list_failed", "Failed to list sources")

		return
	}

	total, err := h.intelSources.CountFiltered(r.Context(), gen.CountIntelSourcesFilteredParams{
		Search:   listParams.Search,
		Category: listParams.Category,
		Status:   listParams.Status,
		Enabled:  listParams.Enabled,
	})
	if err != nil {
		slog.Error("intel: sources directory count failed", "error", err)
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
				"total":       total,
			},
		})

		return
	}

	entries := make([]sourceDirectoryEntry, 0, len(rows))

	for _, s := range rows {
		entries = append(entries, sourceToDirectoryEntry(s))
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": entries,
		"meta": map[string]interface{}{
			"page_size":   limit,
			"page_offset": (page - 1) * limit,
			"count":       len(entries),
			"total":       total,
		},
	})
}

// intelSourceSortKeys is the whitelist of ?sort= column names accepted by
// the filtered sources registry list. Anything else falls back to the
// default created_at DESC ordering.
var intelSourceSortKeys = map[string]bool{
	"name":            true,
	"domain":          true,
	"category":        true,
	"status":          true,
	"trust_level":     true,
	"crawl_method":    true,
	"enabled":         true,
	"last_crawled_at": true,
	"created_at":      true,
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

	entry["rejected_at"] = s.RejectedAt
	if s.RejectedBy.Valid {
		entry["rejected_by"] = s.RejectedBy.String()
	} else {
		entry["rejected_by"] = nil
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

	// note is optional, but a non-empty body that isn't valid JSON is a
	// client bug worth a 400 rather than silently treating it as absent.
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	// Refuse to approve a record whose extraction carries no field values —
	// promoting it would publish a domain row built entirely from fallbacks
	// (source name as title, nulls elsewhere). The reviewer must edit the
	// extraction first (PATCH .../edit) or reject the record.
	record, err := h.intelStores.GetIntelRecord(r.Context(), recordID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "record_not_found", "Intel record not found")
			return
		}

		slog.Error("intel: approve load record failed", "record_id", recordID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "approve_failed", "Failed to load record")

		return
	}

	// State guard: only a record awaiting review may be approved. Approving
	// an already-approved/rejected/published record — or one still moving
	// through the pipeline — would silently overwrite another reviewer's
	// decision or skip the publication gate. 409 lets the client refresh
	// instead of believing its write won.
	if record.Status != "review_required" {
		errpkg.WriteError(w, http.StatusConflict, "invalid_record_state",
			"Record is not awaiting review (current status: "+record.Status+")")

		return
	}

	if !schemas.HasExtractedValues(record.Extracted) {
		errpkg.WriteError(w, http.StatusUnprocessableEntity, "record_empty", "Record has no extracted field values — edit the extraction or reject it instead")

		return
	}

	// Atomic transition: the UPDATE only applies while the record is still
	// review_required, so a racing reviewer cannot both pass the state check
	// and overwrite each other's decision. 0 rows → the other write won.
	moved, err := h.intelStores.TransitionIntelRecordStatus(r.Context(), recordID, "review_required", "approved")
	if err != nil {
		slog.Error("intel: approve status update failed", "record_id", recordID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "approve_failed", "Failed to approve record")

		return
	}

	if !moved {
		errpkg.WriteError(w, http.StatusConflict, "invalid_record_state", "Record is no longer awaiting review — refresh to see its current state")

		return
	}

	// Write the audit entry to the review log.
	note := body.Note
	if note == "" {
		note = "approved by reviewer"
	}

	// The status transition already committed — a log write failure must not
	// 500 (a retry would then 409 on a successful action). Surface the gap as
	// log_failed so the client can flag a missing audit entry instead.
	logFailed := false

	if err := h.intelStores.CreateIntelReviewLog(r.Context(), gen.CreateIntelReviewLogParams{
		IntelRecordID: recordID,
		ActorUserID:   userID,
		Action:        "approve",
		FieldChanges:  []byte(`[]`),
		Note:          &note,
	}); err != nil {
		slog.Error("intel: approve review log write failed", "record_id", recordID.String(), "error", err)
		logFailed = true
	}

	// BUG-003 fix: enqueue intel:promote so the approved record is actually
	// published to scholarships + contents (spec §647, route catalog §2237).
	// Without this the approval is silently lost — re-crawl creates a NEW
	// record rather than promoting the approved one. Fire-and-forget per
	// Rule 8; an enqueue failure is logged but does not roll back the
	// approval. promote_queued surfaces the failure so the reviewer knows
	// to use the re-promote action instead of assuming it will publish.
	promoteQueued := true

	if err := h.enqueueIntelPromote(r.Context(), recordID, userID); err != nil {
		slog.Error("intel: approve enqueue promote failed", "record_id", recordID.String(), "error", err)

		promoteQueued = false
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"record_id":      recordID.String(),
			"status":         "approved",
			"promote_queued": promoteQueued,
			"log_failed":     logFailed,
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

	// note is optional, but a non-empty body that isn't valid JSON is a
	// client bug worth a 400 rather than silently treating it as absent.
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	// State guard mirrors approve: only a record awaiting review may be
	// rejected — a raced/duplicate reject must not overwrite another
	// reviewer's decision.
	record, err := h.intelStores.GetIntelRecord(r.Context(), recordID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "record_not_found", "Intel record not found")
			return
		}

		slog.Error("intel: reject load record failed", "record_id", recordID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "reject_failed", "Failed to load record")

		return
	}

	if record.Status != "review_required" {
		errpkg.WriteError(w, http.StatusConflict, "invalid_record_state",
			"Record is not awaiting review (current status: "+record.Status+")")

		return
	}

	// Atomic transition (same rationale as approve): a racing decision can
	// no longer slip between the check and the write.
	moved, err := h.intelStores.TransitionIntelRecordStatus(r.Context(), recordID, "review_required", "rejected")
	if err != nil {
		slog.Error("intel: reject status update failed", "record_id", recordID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "reject_failed", "Failed to reject record")

		return
	}

	if !moved {
		errpkg.WriteError(w, http.StatusConflict, "invalid_record_state", "Record is no longer awaiting review — refresh to see its current state")

		return
	}

	// Write the audit entry to the review log.
	note := body.Note
	if note == "" {
		note = "rejected by reviewer"
	}

	// Same ordering rationale as approve — the transition already committed,
	// so a log failure surfaces as a flag, not a 500 on a successful action.
	logFailed := false

	if err := h.intelStores.CreateIntelReviewLog(r.Context(), gen.CreateIntelReviewLogParams{
		IntelRecordID: recordID,
		ActorUserID:   userID,
		Action:        "reject",
		FieldChanges:  []byte(`[]`),
		Note:          &note,
	}); err != nil {
		slog.Error("intel: reject review log write failed", "record_id", recordID.String(), "error", err)
		logFailed = true
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"record_id":  recordID.String(),
			"status":     "rejected",
			"log_failed": logFailed,
		},
	})
}

// HandleRequeuePromote re-enqueues intel:promote for a record stuck in
// 'approved'. Approve enqueues promotion fire-and-forget, so a queue outage
// leaves the record approved-but-never-published with no built-in recovery —
// this endpoint is the operator-visible retry path. Records already
// promoted (promoted_record_id set) or in any other status get 409.
// // POST /api/v1/intel/records/{id}/promote
// // Requires: intel.review.approve.
func (h *Handlers) HandleRequeuePromote(w http.ResponseWriter, r *http.Request) {
	if h.intelStores == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	userID, ok := requireUser(w, r)
	if !ok {
		return
	}

	recordID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	record, err := h.intelStores.GetIntelRecord(r.Context(), recordID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "record_not_found", "Intel record not found")
			return
		}

		slog.Error("intel: re-promote load record failed", "record_id", recordID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "repromote_failed", "Failed to load record")

		return
	}

	if record.PromotedRecordID.Valid {
		errpkg.WriteError(w, http.StatusConflict, "already_promoted", "Record has already been promoted")

		return
	}

	if record.Status != "approved" {
		errpkg.WriteError(w, http.StatusConflict, "invalid_record_state",
			"Only approved records can be promoted (current status: "+record.Status+")")

		return
	}

	if err := h.enqueueIntelPromote(r.Context(), recordID, userID); err != nil {
		slog.Error("intel: re-promote enqueue failed", "record_id", recordID.String(), "error", err)
		errpkg.WriteError(w, http.StatusServiceUnavailable, "promote_enqueue_failed", "Failed to queue promotion — try again later")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"record_id": recordID.String(),
			"queued":    true,
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

// intelRecordSortKeys is the whitelist of ?sort= column names accepted by the
// review queue and records list endpoints. Anything else falls back to the
// default extracted_at DESC ordering. The keys match the intel_records column
// names (and the admin table's column keys) exactly.
var intelRecordSortKeys = map[string]bool{
	"entity_type":        true,
	"external_key":       true,
	"status":             true,
	"confidence_overall": true,
	"freshness_score":    true,
	"extracted_at":       true,
}

// parseIntelRecordListParams extracts the shared list params used by both
// HandleReviewQueue and HandleListIntelRecords: entity_type filter, free-text
// search (?q=), and whitelisted sort (?sort=&dir=).
func parseIntelRecordListParams(r *http.Request) gen.ListIntelRecordsFilteredParams {
	q := r.URL.Query()

	params := gen.ListIntelRecordsFilteredParams{
		EntityType: q.Get("entity_type"),
		Search:     q.Get("q"),
	}

	if sortKey := q.Get("sort"); intelRecordSortKeys[sortKey] {
		params.SortKey = sortKey
		if q.Get("dir") == "desc" {
			params.SortDir = "desc"
		} else {
			params.SortDir = "asc"
		}
	}

	return params
}

// listIntelRecordsFiltered runs the filtered list + count queries and writes
// the standard list response. status is supplied by the caller (the review
// queue pins review_required; the records endpoint accepts ?status=).
func (h *Handlers) listIntelRecordsFiltered(w http.ResponseWriter, r *http.Request, status, errCode, errMsg string) {
	page, limit := h.parsePagination(r)
	offset := clampInt32((page - 1) * limit)

	params := parseIntelRecordListParams(r)
	params.Status = status
	params.RowLimit = clampInt32(limit)
	params.RowOffset = offset

	rows, err := h.intelStores.ListIntelRecordsFiltered(r.Context(), params)
	if err != nil {
		slog.Error("intel: records list failed", "status", status, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, errCode, errMsg)

		return
	}

	total, err := h.intelStores.CountIntelRecordsFiltered(r.Context(), gen.CountIntelRecordsFilteredParams{
		Status:     status,
		EntityType: params.EntityType,
		Search:     params.Search,
	})
	if err != nil {
		slog.Error("intel: records count failed", "status", status, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, errCode, errMsg)

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

// HandleReviewQueue lists intel records awaiting human review.
// // BUG-002 fix: the intel_records.status CHECK constraint (migration 000428)
// only allows ('extracted','deduped','auto_published','review_required',
// 'approved','rejected','published','stale','off_topic'). The prior 'review'
// value matched nothing, so the moderation queue was always empty.
// // BUG-001 fix: the prior handler hardcoded EntityType='scholarship', so
// job/visa/grant records awaiting review were invisible. Now an optional
// ?entity_type= query param filters by entity type; when omitted, ALL
// entity types with review_required status are returned.
// // Supports ?q= free-text search and whitelisted ?sort=&dir= so the admin
// table's search box and sortable headers actually filter/order the queue
// (previously the params were silently ignored).
// GET /api/v1/intel/review/queue.
func (h *Handlers) HandleReviewQueue(w http.ResponseWriter, r *http.Request) {
	if h.intelStores == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	h.listIntelRecordsFiltered(w, r, "review_required", "review_queue_failed", "Failed to load review queue")
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
	"off_topic":       true,
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_status", "status must be one of: extracted, deduped, auto_published, review_required, approved, rejected, published, stale, off_topic")
		return
	}

	// Also honors ?entity_type=, ?q= free-text search, and whitelisted
	// ?sort=&dir= — shared with the review queue handler.
	h.listIntelRecordsFiltered(w, r, status, "records_list_failed", "Failed to list intel records")
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
