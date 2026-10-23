package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/intelligence/extractor/schemas"
	"github.com/jedafe/japa/internal/intelligence/sources"
	"github.com/jedafe/japa/internal/worker"
)

// handleSourceCreateBody is the JSON body for POST /api/v1/intel/sources.
// It mirrors the editable fields of intel_sources. Optional fields use
// pointer types so omitted fields are NULL rather than zero-values.
type handleSourceCreateBody struct {
	Name        string  `json:"name"`
	Slug        *string `json:"slug"`
	Domain      string  `json:"domain"`
	SourceURL   string  `json:"source_url"`
	Description *string `json:"description"`
	Category    string  `json:"category"`
	// Categories is the multi-category array (intel-channels §6). On create
	// it is normalized like Propose and dual-writes the legacy scalar
	// category when category is empty; on PATCH the key's presence decides
	// whether the column is replaced (empty array clears to '{}').
	Categories          []string `json:"categories"`
	SourceType          string   `json:"source_type"`
	SourceKind          *string  `json:"source_kind"`
	TrustLevel          int16    `json:"trust_level"`
	CrawlMethod         string   `json:"crawl_method"`
	CrawlFrequency      string   `json:"crawl_frequency"`
	CrawlFrequencyMins  int32    `json:"crawl_frequency_mins"`
	SourceLicense       *string  `json:"source_license"`
	AttributionRequired bool     `json:"attribution_required"`
	UsageRestriction    *string  `json:"usage_restriction"`
	LogoURL             *string  `json:"logo_url"`
	Enabled             *bool    `json:"enabled"`
	PersonaID           *string  `json:"persona_id"` // optional: AI persona to attribute promoted content to
	// EntityTypes declares the extraction domains a mixed source publishes
	// (e.g. ["scholarship","job"]). Stored inside provider_config; when
	// omitted or empty the source extracts under its single category-derived
	// type as before. Each value must map to a registered extraction schema.
	EntityTypes []string `json:"entity_types"`
}

// validateEntityTypes returns the first value that does not map to a
// registered extraction schema, or "" when all values are valid.
func validateEntityTypes(types []string) string {
	for _, et := range types {
		if et == "" {
			return "(empty)"
		}

		if _, ok := schemas.ForEntityType(et); !ok {
			return et
		}
	}

	return ""
}

// providerConfigWithEntityTypes builds a provider_config document carrying
// the entity_types override, preserving any other keys already present.
// Failure to merge (corrupt existing document) yields "{}" — the entity
// types were already validated, so a degraded config beats a failed write.
func providerConfigWithEntityTypes(existing []byte, types []string) []byte {
	out, err := sources.SetEntityTypesInProviderConfig(existing, types)
	if err != nil {
		slog.Warn("intel: provider_config merge failed; resetting to entity_types only", "error", err)

		out, _ = sources.SetEntityTypesInProviderConfig(nil, types)
	}

	if len(out) == 0 {
		out = []byte(`{}`)
	}

	return out
}

// HandleCreateSource creates a new intel source. The body carries the
// configurable fields; robots_policy and provider_config default to empty
// JSON objects (the fetcher populates robots_policy on first crawl).
// // POST /api/v1/intel/sources
// // Requires: intel.sources.manage.
func (h *Handlers) HandleCreateSource(w http.ResponseWriter, r *http.Request) {
	if h.intelSources == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	userID, ok := requireUser(w, r)
	if !ok {
		return
	}

	var body handleSourceCreateBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	params, ok := buildCreateSourceParams(w, body)
	if !ok {
		return
	}

	src, err := h.intelSources.Create(r.Context(), params)
	if err != nil {
		if errors.Is(err, errpkg.ErrAlreadyExists) {
			errpkg.WriteError(w, http.StatusConflict, "already_exists", "A source with this URL already exists")
			return
		}

		slog.Error("intel: create source failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "create_failed", "Failed to create source")

		return
	}

	_ = userID // actor available for audit logging if needed

	errpkg.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"data": sourceToDirectoryEntry(src),
	})
}

// buildCreateSourceParams validates the create-source request body and
// normalises it into CreateIntelSourceParams, applying defaults for enabled,
// crawl frequency, and domain. On a validation failure it writes the error
// response and returns ok=false.
func buildCreateSourceParams(w http.ResponseWriter, body handleSourceCreateBody) (gen.CreateIntelSourceParams, bool) {
	body.Name = strings.TrimSpace(body.Name)
	body.Domain = strings.TrimSpace(body.Domain)
	body.SourceURL = strings.TrimSpace(body.SourceURL)
	body.Category = strings.TrimSpace(body.Category)
	body.SourceType = strings.TrimSpace(body.SourceType)
	body.CrawlMethod = strings.TrimSpace(body.CrawlMethod)
	body.CrawlFrequency = strings.TrimSpace(body.CrawlFrequency)

	// Multi-category (intel-channels §6): normalize like Propose, then
	// dual-write the legacy scalar from categories[0] when the caller sent
	// the array without a category — so a categories-only submission still
	// satisfies the required-field check below.
	categories := sources.NormalizeCategories(body.Categories)
	if body.Category == "" && len(categories) > 0 {
		body.Category = categories[0]
	}

	if body.Name == "" || body.SourceURL == "" || body.Category == "" || body.CrawlMethod == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_field", "name, source_url, category, and crawl_method are required")
		return gen.CreateIntelSourceParams{}, false
	}

	if u, err := url.Parse(body.SourceURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_source_url", "source_url must be a valid http(s) URL")
		return gen.CreateIntelSourceParams{}, false
	}

	if body.TrustLevel < 1 || body.TrustLevel > 4 {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_trust_level", "trust_level must be 1-4")
		return gen.CreateIntelSourceParams{}, false
	}

	if bad := validateEntityTypes(body.EntityTypes); bad != "" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_entity_type", "entity_types contains an unregistered type: "+bad)
		return gen.CreateIntelSourceParams{}, false
	}

	enabled, freqMins, freq, domain := resolveSourceDefaults(body)

	slug := body.Name
	if body.Slug != nil && *body.Slug != "" {
		slug = *body.Slug
	}

	slug = slugifySourceName(slug)

	description := body.Description
	logoURL := body.LogoURL

	sourceKind := body.SourceKind
	if sourceKind == nil || *sourceKind == "" {
		k := "publisher"
		sourceKind = &k
	}

	if !sources.SourceKind(*sourceKind).IsValid() {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_source_kind", "source_kind is not a recognized value")
		return gen.CreateIntelSourceParams{}, false
	}

	params := gen.CreateIntelSourceParams{
		Name:                body.Name,
		Slug:                &slug,
		Domain:              domain,
		SourceUrl:           body.SourceURL,
		Description:         description,
		Category:            body.Category,
		SourceType:          body.SourceType,
		SourceKind:          sourceKind,
		TrustLevel:          body.TrustLevel,
		CrawlMethod:         body.CrawlMethod,
		CrawlFrequency:      freq,
		CrawlFrequencyMins:  freqMins,
		RobotsPolicy:        []byte(`{}`),
		SourceLicense:       body.SourceLicense,
		AttributionRequired: body.AttributionRequired,
		UsageRestriction:    body.UsageRestriction,
		ProviderConfig:      providerConfigWithEntityTypes(nil, body.EntityTypes),
		LogoUrl:             logoURL,
		Enabled:             enabled,
	}

	// Only a non-empty normalized array is bound: nil keeps the INSERT's
	// COALESCE fallback (ARRAY[category]) for back-compat callers, and an
	// explicit empty array would otherwise mask the dual-write.
	if len(categories) > 0 {
		params.Categories = categories
	}

	if body.PersonaID != nil && *body.PersonaID != "" {
		var pid pgtype.UUID
		if err := pid.Scan(*body.PersonaID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_persona_id", "persona_id must be a valid UUID")
			return gen.CreateIntelSourceParams{}, false
		}

		params.PersonaID = pid
	}

	return params, true
}

// resolveSourceDefaults applies the create-source defaults: enabled defaults
// true, crawl frequency defaults to daily (1440 minutes), and domain falls
// back to the source_url's host.
func resolveSourceDefaults(body handleSourceCreateBody) (enabled bool, freqMins int32, freq, domain string) {
	enabled = true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}

	freqMins = body.CrawlFrequencyMins
	if freqMins <= 0 {
		freqMins = 1440 // default: daily
	}

	freq = body.CrawlFrequency
	if freq == "" {
		freq = "daily"
	}

	domain = body.Domain
	if domain == "" {
		domain = extractDomain(body.SourceURL)
	}

	return enabled, freqMins, freq, domain
}

// HandleUpdateSource updates an existing intel source. The body carries the
// editable fields; the enabled toggle can be flipped via this endpoint or the
// dedicated enable/disable routes. Returns the updated source.
// // PATCH /api/v1/intel/sources/{id}
// // Requires: intel.sources.manage.
func (h *Handlers) HandleUpdateSource(w http.ResponseWriter, r *http.Request) {
	if h.intelSources == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	if _, ok := requireUser(w, r); !ok {
		return
	}

	sourceID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	// Load the existing row so PATCH semantics (partial update) work: fields
	// not in the body keep their current values.
	existing, err := h.intelSources.Get(r.Context(), sourceID)
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

	if body.SourceURL != "" {
		body.SourceURL = strings.TrimSpace(body.SourceURL)

		if u, err := url.Parse(body.SourceURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_source_url", "source_url must be a valid http(s) URL")
			return
		}
	}

	// entity_types is a presence-detected field: the key absent leaves the
	// override untouched, an empty array removes it (source falls back to
	// its category-derived type), and a non-empty array replaces it. Every
	// value must map to a registered extraction schema.
	if _, present := presence["entity_types"]; present {
		if bad := validateEntityTypes(body.EntityTypes); bad != "" {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_entity_type", "entity_types contains an unregistered type: "+bad)
			return
		}
	}

	// Enabling is reserved for approved (status='active') sources. Flipping
	// enabled on a proposed/rejected row would mark it live without the
	// verify probe + channel materialization that ApproveProposed runs — a
	// silently dead source. Point the caller at the approve flow instead.
	if body.Enabled != nil && *body.Enabled && existing.Status != "active" {
		errpkg.WriteError(w, http.StatusConflict, "source_not_active",
			"Cannot enable a "+existing.Status+" source — approve it first (POST /intel/sources/{id}/approve)")
		return
	}

	// source_kind is patchable (the merge below applies it); validate the
	// 10-kind enum here so an unknown value is a 400, not a CHECK violation.
	if body.SourceKind != nil && *body.SourceKind != "" && !sources.SourceKind(*body.SourceKind).IsValid() {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_source_kind", "source_kind is not a recognized value")
		return
	}

	// Merge: only non-zero / non-nil body fields override the existing row.
	params := gen.UpdateIntelSourceParams{
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

	if err := mergeSourcePatch(&params, &body, presence); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_persona_id", err.Error())
		return
	}

	// categories is a presence-detected array field (same semantics as the
	// channel PATCH): absent leaves the column untouched, present replaces
	// it — an empty array or null clears to '{}'. It cannot ride the main
	// update (the query's COALESCE can't express "set empty"), so it goes
	// through the dedicated UpdateCategories after the scalar write.
	categoriesPresent := false

	var newCategories []string

	if _, present := presence["categories"]; present {
		categoriesPresent = true
		newCategories = sources.NormalizeCategories(body.Categories)
		// Dual-write: the legacy primary follows categories[0] unless the
		// patch set category explicitly (applied by mergeSourcePatch).
		if _, catPresent := presence["category"]; !catPresent && len(newCategories) > 0 {
			params.Category = newCategories[0]
		}
	}

	src, err := h.intelSources.Update(r.Context(), params)
	if err != nil {
		slog.Error("intel: update source failed", "source_id", sourceID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to update source")

		return
	}

	if categoriesPresent {
		updated, cerr := h.intelSources.UpdateCategories(r.Context(), gen.UpdateIntelSourceCategoriesParams{
			ID:         sourceID,
			Categories: newCategories,
		})
		if cerr != nil {
			slog.Error("intel: update source categories failed", "source_id", sourceID.String(), "error", cerr)
			errpkg.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to update source categories")

			return
		}
		// updated reflects both writes — the response echoes the fresh row.
		src = updated
	}

	// persona_id: an explicit JSON null clears the attribution — the update
	// query COALESCEs the column so it cannot express NULL itself. An absent
	// key or empty string keeps the existing value.
	if raw, present := presence["persona_id"]; present && string(raw) == "null" {
		if cerr := h.intelSources.ClearPersona(r.Context(), sourceID); cerr != nil {
			slog.Error("intel: clear source persona failed", "source_id", sourceID.String(), "error", cerr)
			errpkg.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to clear source persona")

			return
		}

		src.PersonaID = pgtype.UUID{}
	}

	// Apply the entity_types override when the key was present: merge into
	// the existing provider_config so sibling keys (API endpoint config,
	// crawl hints) survive — never a wholesale replace.
	if _, present := presence["entity_types"]; present {
		merged := providerConfigWithEntityTypes(existing.ProviderConfig, body.EntityTypes)
		if uerr := h.intelSources.UpdateProviderConfig(r.Context(), gen.UpdateIntelSourceProviderConfigParams{
			ID:             sourceID,
			ProviderConfig: merged,
		}); uerr != nil {
			slog.Error("intel: provider_config update failed", "source_id", sourceID.String(), "error", uerr)
			errpkg.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to update source entity types")

			return
		}
		// src predates the provider_config write — stamp the merged document
		// so the response echoes the override that was just applied.
		src.ProviderConfig = merged
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": sourceToDirectoryEntry(src),
	})
}

// mergeSourcePatch applies the PATCH merge rules: non-zero / non-nil body
// fields override the seeded params; presence-detected keys (BUG-010) are
// applied only when the JSON key was present. It returns an error when
// persona_id is present but not a valid UUID.
func mergeSourcePatch(params *gen.UpdateIntelSourceParams, body *handleSourceCreateBody, presence map[string]json.RawMessage) error {
	patchStrField(&params.Name, body.Name)
	patchStrField(&params.Domain, body.Domain)
	patchStrField(&params.SourceUrl, body.SourceURL)
	patchStrField(&params.Category, body.Category)
	patchStrField(&params.SourceType, body.SourceType)
	patchStrField(&params.CrawlMethod, body.CrawlMethod)
	patchStrField(&params.CrawlFrequency, body.CrawlFrequency)

	if body.Slug != nil && *body.Slug != "" {
		slug := slugifySourceName(*body.Slug)
		params.Slug = &slug
	}

	if body.Description != nil {
		params.Description = body.Description
	}

	if body.SourceKind != nil && *body.SourceKind != "" {
		params.SourceKind = body.SourceKind
	}

	if body.LogoURL != nil {
		params.LogoUrl = body.LogoURL
	}

	if body.TrustLevel >= 1 && body.TrustLevel <= 4 {
		params.TrustLevel = body.TrustLevel
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
	rawPID, present := presence["persona_id"]
	if !present {
		return nil
	}

	var pidStr string
	// A non-string persona_id is ignored — the existing value is kept.
	_ = json.Unmarshal(rawPID, &pidStr)

	if pidStr == "" {
		return nil
	}

	var pid pgtype.UUID
	if err := pid.Scan(pidStr); err != nil {
		return errors.New("persona_id must be a valid UUID")
	}

	params.PersonaID = pid

	return nil
}

// patchStrField assigns v to dst when v is non-empty (PATCH merge helper).
func patchStrField(dst *string, v string) {
	if v != "" {
		*dst = v
	}
}

// HandleDeleteSource removes an intel source from the registry.
// // BUG-004 fix: the spine tables cascade on delete
// (intel_sources → intel_raw_documents → intel_records → scholarships →
// contents), so a naive delete wipes published scholarship content,
// comments, SEO, and RAG embeddings — violating spec §650 / route-catalog
// §2225 ("promoted records preserved") and Rule 11. The handler now refuses
// to delete a source that has any promoted records (returns 409 with a
// pointer to the disable endpoint). To remove a source with promoted data,
// admins disable it (PATCH enabled=false); the source row stays and
// promoted records are preserved.
// // DELETE /api/v1/intel/sources/{id}
// // Requires: intel.sources.manage.
func (h *Handlers) HandleDeleteSource(w http.ResponseWriter, r *http.Request) {
	if h.intelSources == nil || h.intelStores == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	if _, ok := requireUser(w, r); !ok {
		return
	}

	sourceID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	// Enforce the "promoted records preserved" invariant (spec §650).
	promoted, err := h.intelStores.CountPromotedIntelRecordsBySource(r.Context(), sourceID)
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

	if err := h.intelSources.Delete(r.Context(), sourceID); err != nil {
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
// // GET /api/v1/intel/raw-documents/{id}
// // Requires: intel.review.view.
func (h *Handlers) HandleGetRawDocument(w http.ResponseWriter, r *http.Request) {
	if h.intelStores == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	if _, ok := requireUser(w, r); !ok {
		return
	}

	docID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	doc, err := h.intelStores.GetIntelRawDocument(r.Context(), docID)
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

// HandleReviewEdit saves admin-edited extraction fields on an intel record.
// The body carries a map of field-name → edited value.
//
// Two write targets:
//  1. intel_records.extracted — always merged. Review-queue records have no
//     domain projection yet (projections are created by intel:promote after
//     approval), so the extracted payload is the only place an edit can
//     persist such that promotion respects it. This also makes the endpoint
//     work for every entity type, not just scholarships (INTEL-009 fix: the
//     prior implementation required a scholarships projection row and 404'd
//     for every unprojected record).
//  2. scholarships.editorial_overrides — additionally merged when a
//     scholarship projection already exists for the record, preserving the
//     BUG-011 override semantics (override_source flips to
//     'editorial'/'mixed') so re-crawl does not overwrite the edited fields.
//
// An audit entry is written to intel_review_log either way.
// // PATCH /api/v1/intel/review/{record_id}/edit
// // Requires: intel.review.edit.
func (h *Handlers) HandleReviewEdit(w http.ResponseWriter, r *http.Request) {
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

	record, err := h.intelStores.GetIntelRecord(r.Context(), recordID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Intel record not found")
			return
		}

		slog.Error("intel: review edit record lookup failed", "record_id", recordID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "lookup_failed", "Failed to load intel record")

		return
	}

	// Merge the edited fields into the record's extracted payload so the
	// change persists for records without a projection row and promotion
	// (intel:promote) picks up the reviewer edits.
	mergedExtracted := mergeEditorialOverrides(record.Extracted, body.Fields)

	if err := h.intelStores.UpdateIntelRecordExtracted(r.Context(), gen.UpdateIntelRecordExtractedParams{
		ID:        recordID,
		Extracted: mergedExtracted,
	}); err != nil {
		slog.Error("intel: review edit apply failed", "record_id", recordID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "edit_failed", "Failed to apply editorial edits")

		return
	}

	// When a scholarship projection already exists for this record, mirror
	// the edit into its editorial_overrides so the published projection is
	// corrected too and re-crawl does not overwrite the edited fields.
	var scholarshipID string

	var overrideSource string

	if record.EntityType == "scholarship" {
		scholarship, err := h.intelStores.GetScholarshipByIntelRecord(r.Context(), recordID)
		if err == nil {
			merged := mergeEditorialOverrides(scholarship.EditorialOverrides, body.Fields)
			if err := h.intelStores.UpdateScholarshipEditorialOverrides(r.Context(), scholarship.ID, merged); err != nil {
				slog.Error("intel: review edit scholarship override apply failed", "scholarship_id", pgUUIDString(scholarship.ID), "error", err)
				errpkg.WriteError(w, http.StatusInternalServerError, "edit_failed", "Failed to apply editorial edits")

				return
			}

			scholarshipID = pgUUIDString(scholarship.ID)
			// BUG-011: mirror the SQL CASE in UpdateScholarshipEditorialOverrides
			// (scholarships_intel.sql) so the response reports the real
			// override_source rather than a hardcoded "editorial".
			overrideSource = "editorial"
			if scholarship.OverrideSource == "editorial" || scholarship.OverrideSource == "mixed" {
				overrideSource = "mixed"
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			slog.Error("intel: review edit scholarship lookup failed", "record_id", recordID.String(), "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "lookup_failed", "Failed to load scholarship")

			return
		}
		// pgx.ErrNoRows → no projection yet; the extracted write above is
		// sufficient and promotion will carry the edits into the projection.
	}

	// Write the audit entry to the review log.
	note := body.Note
	if note == "" {
		note = "editorial override applied by reviewer"
	}

	fieldChangesJSON, _ := json.Marshal(body.Fields)
	if err := h.intelStores.CreateIntelReviewLog(r.Context(), gen.CreateIntelReviewLogParams{
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
			"scholarship_id":  scholarshipID,
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

// slugifySourceName converts a source name into a URL-safe slug.
// It lowercases, replaces non-alphanumeric characters with hyphens, and
// collapses multiple hyphens. It does not guarantee uniqueness; callers
// should rely on the database unique index and retry on conflict.
func slugifySourceName(name string) string {
	re := regexp.MustCompile(`[^a-zA-Z0-9]+`)
	slug := re.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-")
	slug = strings.Trim(slug, "-")

	return slug
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
// // POST /api/v1/intel/sources/{id}/crawl-now
// // Requires: intel.sources.crawl_now
// Ref: INTEL-011 fix, §26 RBAC CRUD completeness, route catalog §2226.
func (h *Handlers) HandleCrawlNow(w http.ResponseWriter, r *http.Request) {
	if h.intelSources == nil || h.asynqClient == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	if _, ok := requireUser(w, r); !ok {
		return
	}

	sourceID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	// Verify the source exists and is enabled before enqueuing.
	src, err := h.intelSources.Get(r.Context(), sourceID)
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
// // GET /api/v1/intel/sources/proposed
// // Requires: intel.sources.manage
// Ref: docs/superpowers/specs/2026-08-19-on-demand-intel-to-content.md.
func (h *Handlers) HandleListProposedSources(w http.ResponseWriter, r *http.Request) {
	if h.intelSources == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	_, ok := requireUser(w, r)
	if !ok {
		return
	}

	page, limit := h.parsePagination(r)
	offset := clampInt32((page - 1) * limit)
	search := r.URL.Query().Get("q")

	rows, err := h.intelSources.ListProposed(r.Context(), clampInt32(limit), offset, search)
	if err != nil {
		slog.Error("intel: list proposed sources failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_proposed_failed", "Failed to list proposed sources")

		return
	}

	total, err := h.intelSources.CountProposed(r.Context(), search)
	if err != nil {
		slog.Error("intel: count proposed sources failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_proposed_failed", "Failed to list proposed sources")

		return
	}

	entries := make([]map[string]interface{}, 0, len(rows))

	for _, s := range rows {
		entry := sourceToFullEntry(s.Source)
		entry["proposer_name"] = s.ProposerName
		entries = append(entries, entry)
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

// HandleApproveProposedSource approves a proposed source: sets
// status='active', enabled=true, and records the approver.
// // POST /api/v1/intel/sources/{id}/approve
// // Requires: intel.sources.manage
// Ref: docs/superpowers/specs/2026-08-19-on-demand-intel-to-content.md.
func (h *Handlers) HandleApproveProposedSource(w http.ResponseWriter, r *http.Request) {
	if h.intelSources == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	userID, ok := requireUser(w, r)
	if !ok {
		return
	}

	sourceID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	src, err := h.intelSources.ApproveProposed(r.Context(), sourceID, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Proposed source not found")
			return
		}

		slog.Error("intel: approve proposed source failed", "source_id", sourceID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "approve_failed", "Failed to approve source")

		return
	}

	// Materialize directory tags for the approved source's keywords on the
	// write path so the public GET stays read-only.
	h.createSourceTags(r.Context(), src.Keywords)

	// §106 channel-sync amendment: a newly approved video_channel is due
	// immediately — kick the poller so its first import doesn't wait for the
	// hourly cron. Fire-and-forget (Rule 8).
	if src.SourceType == "video_channel" && h.asynqClient != nil {
		if _, err := h.asynqClient.EnqueueContext(r.Context(), asynq.NewTask(TaskVideoSyncChannels, []byte("{}"))); err != nil {
			slog.Warn("intel: approve proposed: video sync enqueue failed", "source_id", sourceID.String(), "error", err)
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"source_id": sourceID.String(),
			"status":    src.Status,
			"enabled":   src.Enabled,
		},
	})
}

// HandleRejectProposedSource rejects a proposed source: status='rejected',
// enabled=false. The row is kept (with the reviewer recorded) so repeat
// spam proposals remain auditable — the alternative, DELETE, would erase
// the provenance and let the same URL be re-proposed silently. A row no
// longer in 'proposed' returns 404, mirroring the approve precondition.
// // POST /api/v1/intel/sources/{id}/reject
// // Requires: intel.sources.manage.
func (h *Handlers) HandleRejectProposedSource(w http.ResponseWriter, r *http.Request) {
	if h.intelSources == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	userID, ok := requireUser(w, r)
	if !ok {
		return
	}

	sourceID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	src, err := h.intelSources.RejectProposed(r.Context(), sourceID, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Proposed source not found")
			return
		}

		slog.Error("intel: reject proposed source failed", "source_id", sourceID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "reject_failed", "Failed to reject source")

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

// HandleGetSourceByID returns a single intel source by id with the full
// crawl-state fields — the control-panel equivalent of the public
// /sources/{slug} detail. The review detail page uses it to resolve a
// record's source without scanning the whole registry.
// // GET /api/v1/intel/sources/{id}
// // Requires: intel.sources.view or intel.sources.manage.
func (h *Handlers) HandleGetSourceByID(w http.ResponseWriter, r *http.Request) {
	if h.intelSources == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	if _, ok := requireUser(w, r); !ok {
		return
	}

	sourceID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	src, err := h.intelSources.Get(r.Context(), sourceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Source not found")
			return
		}

		slog.Error("intel: get source failed", "source_id", sourceID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "lookup_failed", "Failed to load source")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": sourceToFullEntry(src),
	})
}

// HandleDisableSource disables a source from the review queue surface.
// This is the review-side counterpart to PATCH enabled=false: it sets
// enabled=false so future crawls stop, but existing promoted records are
// preserved (spec §650). An audit entry is written to intel_review_log.
// // POST /api/v1/intel/sources/{id}/disable
// // Requires: intel.review.disable_source
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

	sourceID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	// Verify the source exists before disabling.
	src, err := h.intelSources.Get(r.Context(), sourceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Source not found")
			return
		}

		slog.Error("intel: disable source lookup failed", "source_id", sourceID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "lookup_failed", "Failed to load source")

		return
	}

	if err := h.intelSources.Disable(r.Context(), sourceID); err != nil {
		slog.Error("intel: disable source failed", "source_id", sourceID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "disable_failed", "Failed to disable source")

		return
	}

	slog.Info("intel: source disabled via review queue", "source_id", sourceID.String(), "actor", userID.String())

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"source_id": sourceID.String(),
			"enabled":   false,
			"domain":    src.Domain,
		},
	})
}

// ---------------------------------------------------------------------------
// Channel admin endpoints (intel-channels Phase 1, spec §5).
//
// A channel is one entry point (entry_url + crawl_method + pipeline) under an
// intel_sources row. These handlers back the control panel's per-source
// channels editor: list/add/edit/disable/crawl-now per channel plus the
// "Re-probe" action that re-runs the deep verify probe.
//
// Routes live under /api/v1/admin/intel/* and are RBAC-gated per action by
// the intel.channels.* nodes (migration 000684); RBAC is enforced by route
// middleware, so the handlers only authenticate + validate.
// ---------------------------------------------------------------------------

// channelAdminEntry is the admin-facing shape for one intel_source_channels
// row. provider_config is emitted as inline JSON (jsonb) rather than a
// base64 string, matching the raw-document response convention.
func channelToAdminEntry(ch sources.Channel) map[string]interface{} {
	// text[] columns marshal nil as null — the control panel form expects
	// arrays, so normalize empty slices to [] for a stable shape.
	entityTypes := ch.EntityTypes
	if entityTypes == nil {
		entityTypes = []string{}
	}

	topics := ch.RelevanceTopics
	if topics == nil {
		topics = []string{}
	}

	patterns := ch.RelevanceUrlPatterns
	if patterns == nil {
		patterns = []string{}
	}

	entry := map[string]interface{}{
		"id":                     pgUUIDString(ch.ID),
		"source_id":              pgUUIDString(ch.SourceID),
		"label":                  ch.Label,
		"entry_url":              ch.EntryUrl,
		"crawl_method":           ch.CrawlMethod,
		"entity_types":           entityTypes,
		"pipeline":               ch.Pipeline,
		"provider_config":        jsonBlobToRawMessage(ch.ProviderConfig),
		"crawl_frequency_mins":   ch.CrawlFrequencyMins,
		"enabled":                ch.Enabled,
		"origin":                 ch.Origin,
		"relevance_mode":         ch.RelevanceMode,
		"relevance_topics":       topics,
		"relevance_url_patterns": patterns,
		"consecutive_failures":   ch.ConsecutiveFailures,
		"last_crawled_at":        ch.LastCrawledAt,
		"last_success_at":        ch.LastSuccessAt,
		"last_error_at":          ch.LastErrorAt,
		"last_changed_at":        ch.LastChangedAt,
		"created_at":             ch.CreatedAt,
		"updated_at":             ch.UpdatedAt,
	}
	if ch.LastErrorMessage != nil {
		entry["last_error_message"] = *ch.LastErrorMessage
	} else {
		entry["last_error_message"] = nil
	}

	return entry
}

// writeChannelServiceErr maps sources-service errors onto HTTP responses for
// the channel endpoints: validation sentinels → 400, uniqueness → 409,
// missing rows → 404, everything else → 500 (logged).
func writeChannelServiceErr(w http.ResponseWriter, err error, logMsg, failCode, failMsg string) {
	switch {
	case errors.Is(err, errpkg.ErrInvalid), errors.Is(err, errpkg.ErrRequired):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", err.Error())
	case errors.Is(err, errpkg.ErrAlreadyExists):
		errpkg.WriteError(w, http.StatusConflict, "already_exists",
			"A channel with this entry URL and entity types already exists for the source")
	case errors.Is(err, pgx.ErrNoRows):
		errpkg.WriteError(w, http.StatusNotFound, "not_found", failMsg)
	default:
		slog.Error(logMsg, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, failCode, failMsg)
	}
}

// parseProviderConfigRaw validates a provider_config JSON value from a
// request body: it must decode to a JSON object (null is normalized to {}).
// Returns ok=false after writing the error response on invalid input.
func parseProviderConfigRaw(w http.ResponseWriter, raw json.RawMessage) ([]byte, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return []byte(`{}`), true
	}

	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_provider_config", "provider_config must be a JSON object")
		return nil, false
	}

	return []byte(raw), true
}

// HandleAdminListSourceChannels returns every channel owned by a source,
// ordered by created_at. Used by the control panel channels editor on the
// source detail page and by the proposal-review surface (probe-materialized
// channels appear here with origin='probe').
// // GET /api/v1/admin/intel/sources/{id}/channels
// // Requires: intel.channels.list.
func (h *Handlers) HandleAdminListSourceChannels(w http.ResponseWriter, r *http.Request) {
	if h.intelSources == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	if _, ok := requireUser(w, r); !ok {
		return
	}

	sourceID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	// Verify the source exists so a typo'd id reports 404 instead of an
	// empty channel list that looks like "no channels yet".
	if _, err := h.intelSources.Get(r.Context(), sourceID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Source not found")
			return
		}

		slog.Error("intel: list channels source lookup failed", "source_id", sourceID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "lookup_failed", "Failed to load source")

		return
	}

	channels, err := h.intelSources.ListChannels(r.Context(), sourceID)
	if err != nil {
		slog.Error("intel: list channels failed", "source_id", sourceID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list channels")

		return
	}

	entries := make([]map[string]interface{}, 0, len(channels))
	for _, ch := range channels {
		entries = append(entries, channelToAdminEntry(ch))
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": entries,
		"meta": map[string]interface{}{"count": len(entries)},
	})
}

// handleChannelCreateBody is the JSON body for
// POST /api/v1/admin/intel/sources/{id}/channels. The service fills the
// schema defaults (pipeline='extract', relevance_mode='filtered',
// origin='admin', provider_config='{}', crawl_frequency_mins=1440) for
// fields the caller omits.
type handleChannelCreateBody struct {
	Label                string          `json:"label"`
	EntryURL             string          `json:"entry_url"`
	CrawlMethod          string          `json:"crawl_method"`
	EntityTypes          []string        `json:"entity_types"`
	Pipeline             string          `json:"pipeline"`
	ProviderConfig       json.RawMessage `json:"provider_config"`
	CrawlFrequencyMins   int32           `json:"crawl_frequency_mins"`
	Enabled              *bool           `json:"enabled"`
	RelevanceMode        string          `json:"relevance_mode"`
	RelevanceTopics      []string        `json:"relevance_topics"`
	RelevanceURLPatterns []string        `json:"relevance_url_patterns"`
}

// HandleAdminCreateChannel registers a new channel under a source
// (origin='admin' via the service default). The body carries the
// channel-editable fields from spec §5; domain validation (known
// crawl_method/pipeline/relevance_mode, http(s) entry URL for pull methods,
// entity_types required for extract|curate pipelines) lives in
// sources.Service.CreateChannel.
// // POST /api/v1/admin/intel/sources/{id}/channels
// // Requires: intel.channels.create.
func (h *Handlers) HandleAdminCreateChannel(w http.ResponseWriter, r *http.Request) {
	if h.intelSources == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	if _, ok := requireUser(w, r); !ok {
		return
	}

	sourceID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var body handleChannelCreateBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	// Verify the source exists before validating the body — a 404 on a
	// typo'd source id beats a channel-validation error on a dead parent.
	if _, err := h.intelSources.Get(r.Context(), sourceID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Source not found")
			return
		}

		slog.Error("intel: create channel source lookup failed", "source_id", sourceID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "lookup_failed", "Failed to load source")

		return
	}

	if bad := validateEntityTypes(body.EntityTypes); bad != "" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_entity_type", "entity_types contains an unregistered type: "+bad)
		return
	}

	params := gen.CreateIntelSourceChannelParams{
		SourceID:             sourceID,
		Label:                body.Label,
		EntryUrl:             body.EntryURL,
		CrawlMethod:          body.CrawlMethod,
		EntityTypes:          body.EntityTypes,
		Pipeline:             body.Pipeline,
		CrawlFrequencyMins:   body.CrawlFrequencyMins,
		RelevanceMode:        body.RelevanceMode,
		RelevanceTopics:      body.RelevanceTopics,
		RelevanceUrlPatterns: body.RelevanceURLPatterns,
		Enabled:              true, // channels are created enabled unless explicitly disabled
	}
	if body.Enabled != nil {
		params.Enabled = *body.Enabled
	}

	if len(body.ProviderConfig) > 0 {
		pc, ok := parseProviderConfigRaw(w, body.ProviderConfig)
		if !ok {
			return
		}

		params.ProviderConfig = pc
	}

	ch, err := h.intelSources.CreateChannel(r.Context(), params)
	if err != nil {
		writeChannelServiceErr(w, err, "intel: create channel failed", "create_failed", "Failed to create channel")
		return
	}

	// WebSub channels self-manage their hub subscription — kick off the
	// subscribe task (fire-and-forget; the hub POST + renewal chain live
	// in the worker, intel-channels Phase 3).
	if ch.CrawlMethod == "webhook" && ch.Enabled && inboundCfgString(channelProviderConfig(ch.ProviderConfig), "hub") != "" {
		h.enqueueWebSubSubscribe(r.Context(), ch.ID.String())
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"data": channelToAdminEntry(ch),
	})
}

// enqueueWebSubSubscribe fires intel:websub_subscribe for a channel —
// best-effort: a failed enqueue only delays the subscription until the
// next channel edit or manual crawl.
func (h *Handlers) enqueueWebSubSubscribe(ctx context.Context, channelID string) {
	if h.asynqClient == nil {
		return
	}

	payload, _ := json.Marshal(map[string]string{
		"version":    "v1",
		"channel_id": channelID,
	})
	if _, err := h.asynqClient.EnqueueContext(ctx, asynq.NewTask(worker.TypeIntelWebSubSubscribe, payload),
		worker.TaskOpts(worker.TypeIntelWebSubSubscribe)...); err != nil {
		slog.Warn("intel: websub_subscribe enqueue failed", "channel_id", channelID, "error", err)
	}
}

// HandleAdminGetChannel returns a single channel with its full config and
// crawl bookkeeping. Used by the channels editor to prefill the edit form.
// // GET /api/v1/admin/intel/channels/{id}
// // Requires: intel.channels.view.
func (h *Handlers) HandleAdminGetChannel(w http.ResponseWriter, r *http.Request) {
	if h.intelSources == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	if _, ok := requireUser(w, r); !ok {
		return
	}

	channelID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	ch, err := h.intelSources.GetChannel(r.Context(), channelID)
	if err != nil {
		writeChannelServiceErr(w, err, "intel: get channel failed", "lookup_failed", "Channel not found")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": channelToAdminEntry(ch),
	})
}

// handleChannelPatchBody is the JSON body for
// PATCH /api/v1/admin/intel/channels/{id}. All fields are optional; JSON key
// presence (detected on the raw body) decides which update path runs:
//   - entity_types present (even [] or null → explicit clear) →
//     UpdateChannelEntityTypes
//   - any relevance_mode|relevance_topics|relevance_url_patterns key →
//     UpdateChannelRelevance (absent keys keep their current values)
//   - any scalar key (label/entry_url/crawl_method/pipeline/provider_config/
//     crawl_frequency_mins/enabled) → UpdateChannel
type handleChannelPatchBody struct {
	Label                *string         `json:"label"`
	EntryURL             *string         `json:"entry_url"`
	CrawlMethod          *string         `json:"crawl_method"`
	Pipeline             *string         `json:"pipeline"`
	ProviderConfig       json.RawMessage `json:"provider_config"`
	CrawlFrequencyMins   *int32          `json:"crawl_frequency_mins"`
	Enabled              *bool           `json:"enabled"`
	EntityTypes          []string        `json:"entity_types"`
	RelevanceMode        *string         `json:"relevance_mode"`
	RelevanceTopics      []string        `json:"relevance_topics"`
	RelevanceURLPatterns []string        `json:"relevance_url_patterns"`
}

// channelRelevanceKeys are the JSON keys that route through
// UpdateChannelRelevance (direct assignment, empty arrays allowed).
var channelRelevanceKeys = []string{"relevance_mode", "relevance_topics", "relevance_url_patterns"}

// HandleAdminUpdateChannel applies a partial update to a channel. Presence
// semantics (spec §5): an array key that is present — even empty — is an
// explicit set/clear, while absent keys leave the column untouched. The
// patch composes up to three service calls: entity_types first (so a
// simultaneous pipeline switch sees the new types), then relevance, then
// scalar fields.
// // PATCH /api/v1/admin/intel/channels/{id}
// // Requires: intel.channels.update.
func (h *Handlers) HandleAdminUpdateChannel(w http.ResponseWriter, r *http.Request) {
	if h.intelSources == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	if _, ok := requireUser(w, r); !ok {
		return
	}

	channelID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

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

	var body handleChannelPatchBody
	if err := json.Unmarshal(rawBody, &body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	entityTypesPresent := false
	if _, ok := presence["entity_types"]; ok {
		entityTypesPresent = true

		if bad := validateEntityTypes(body.EntityTypes); bad != "" {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_entity_type", "entity_types contains an unregistered type: "+bad)
			return
		}
	}

	relevancePresent := false

	for _, k := range channelRelevanceKeys {
		if _, ok := presence[k]; ok {
			relevancePresent = true
			break
		}
	}

	scalar := gen.UpdateIntelSourceChannelParams{ID: channelID}
	scalarPresent := false

	if body.Label != nil {
		scalar.Label = body.Label
		scalarPresent = true
	}

	if body.EntryURL != nil {
		scalar.EntryUrl = body.EntryURL
		scalarPresent = true
	}

	if body.CrawlMethod != nil {
		scalar.CrawlMethod = body.CrawlMethod
		scalarPresent = true
	}

	if body.Pipeline != nil {
		scalar.Pipeline = body.Pipeline
		scalarPresent = true
	}

	if raw, ok := presence["provider_config"]; ok {
		pc, ok := parseProviderConfigRaw(w, raw)
		if !ok {
			return
		}

		scalar.ProviderConfig = pc
		scalarPresent = true
	}

	if body.CrawlFrequencyMins != nil {
		if *body.CrawlFrequencyMins <= 0 {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", "crawl_frequency_mins must be positive")
			return
		}

		scalar.CrawlFrequencyMins = body.CrawlFrequencyMins
		scalarPresent = true
	}

	if body.Enabled != nil {
		scalar.Enabled = body.Enabled
		scalarPresent = true
	}

	if !entityTypesPresent && !relevancePresent && !scalarPresent {
		errpkg.WriteError(w, http.StatusBadRequest, "empty_patch", "No editable channel fields in request body")
		return
	}

	// Load the existing row only once the patch is known to carry work:
	// needed for the 404 contract and for merging absent relevance keys
	// into the direct-assignment update.
	existing, err := h.intelSources.GetChannel(r.Context(), channelID)
	if err != nil {
		writeChannelServiceErr(w, err, "intel: update channel lookup failed", "lookup_failed", "Channel not found")
		return
	}

	updated := existing

	// entity_types first: a same-patch pipeline switch to extract|curate
	// must see the new types when UpdateChannel runs its cross-field check.
	if entityTypesPresent {
		types := body.EntityTypes
		if types == nil { // explicit null → clear
			types = []string{}
		}

		updated, err = h.intelSources.UpdateChannelEntityTypes(r.Context(), gen.UpdateIntelChannelEntityTypesParams{
			ID:          channelID,
			EntityTypes: types,
		})
		if err != nil {
			writeChannelServiceErr(w, err, "intel: update channel entity types failed", "update_failed", "Failed to update channel")
			return
		}
	}

	if relevancePresent {
		relParams := gen.UpdateIntelChannelRelevanceParams{
			ID:                   channelID,
			RelevanceMode:        updated.RelevanceMode,
			RelevanceTopics:      updated.RelevanceTopics,
			RelevanceUrlPatterns: updated.RelevanceUrlPatterns,
		}
		if body.RelevanceMode != nil {
			relParams.RelevanceMode = *body.RelevanceMode
		}

		if _, ok := presence["relevance_topics"]; ok {
			relParams.RelevanceTopics = body.RelevanceTopics
			if relParams.RelevanceTopics == nil { // explicit null → clear
				relParams.RelevanceTopics = []string{}
			}
		}

		if _, ok := presence["relevance_url_patterns"]; ok {
			relParams.RelevanceUrlPatterns = body.RelevanceURLPatterns
			if relParams.RelevanceUrlPatterns == nil {
				relParams.RelevanceUrlPatterns = []string{}
			}
		}

		updated, err = h.intelSources.UpdateChannelRelevance(r.Context(), relParams)
		if err != nil {
			writeChannelServiceErr(w, err, "intel: update channel relevance failed", "update_failed", "Failed to update channel")
			return
		}
	}

	if scalarPresent {
		updated, err = h.intelSources.UpdateChannel(r.Context(), scalar)
		if err != nil {
			writeChannelServiceErr(w, err, "intel: update channel failed", "update_failed", "Failed to update channel")
			return
		}
	}

	// Mirror the create hook: an edit that leaves the channel as an enabled
	// webhook with a hub and touched provider_config/crawl_method/enabled
	// needs a (re)subscription kick — e.g. adding a hub to a previously
	// hubless webhook channel.
	_, pcTouched := presence["provider_config"]
	if (pcTouched || body.CrawlMethod != nil || body.Enabled != nil) &&
		updated.CrawlMethod == "webhook" && updated.Enabled &&
		inboundCfgString(channelProviderConfig(updated.ProviderConfig), "hub") != "" {
		h.enqueueWebSubSubscribe(r.Context(), updated.ID.String())
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": channelToAdminEntry(updated),
	})
}

// HandleAdminDeleteChannel removes a channel from a source. Raw documents
// reference the channel with ON DELETE SET NULL and inbound events cascade,
// so deleting a channel never orphans pipeline state.
// // DELETE /api/v1/admin/intel/channels/{id}
// // Requires: intel.channels.delete.
func (h *Handlers) HandleAdminDeleteChannel(w http.ResponseWriter, r *http.Request) {
	if h.intelSources == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	if _, ok := requireUser(w, r); !ok {
		return
	}

	channelID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	// Verify the channel exists so a typo'd id reports 404 (the DELETE
	// statement itself is a no-op on missing rows).
	if _, err := h.intelSources.GetChannel(r.Context(), channelID); err != nil {
		writeChannelServiceErr(w, err, "intel: delete channel lookup failed", "lookup_failed", "Channel not found")
		return
	}

	if err := h.intelSources.DeleteChannel(r.Context(), channelID); err != nil {
		slog.Error("intel: delete channel failed", "channel_id", channelID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "delete_failed", "Failed to delete channel")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{"deleted": true},
	})
}

// HandleAdminCrawlChannel enqueues an intel:fetch_channel task for an
// immediate crawl of a single channel — the channel-granular counterpart of
// HandleCrawlNow (intel-channels Phase 1 moved scheduling to channels).
// Disabled channels and methods the worker cannot fetch (manual, email,
// webhook — the same set ListDueChannels excludes and HandleIntelFetchChannel
// skips) are rejected with 409 so a queued-but-no-op enqueue never misleads.
// // POST /api/v1/admin/intel/channels/{id}/crawl
// // Requires: intel.channels.crawl.
func (h *Handlers) HandleAdminCrawlChannel(w http.ResponseWriter, r *http.Request) {
	if h.intelSources == nil || h.asynqClient == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	if _, ok := requireUser(w, r); !ok {
		return
	}

	channelID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	ch, err := h.intelSources.GetChannel(r.Context(), channelID)
	if err != nil {
		writeChannelServiceErr(w, err, "intel: crawl channel lookup failed", "lookup_failed", "Channel not found")
		return
	}

	if !ch.Enabled {
		errpkg.WriteError(w, http.StatusConflict, "channel_disabled", "Cannot crawl a disabled channel — enable it first")
		return
	}

	method := sources.CrawlMethod(ch.CrawlMethod)
	if sources.InboundMethod(method) || method == sources.CrawlMethodManual {
		errpkg.WriteError(w, http.StatusConflict, "channel_not_crawlable",
			"Channels with crawl_method manual, email, or webhook are never fetched — inbound endpoints own ingestion")
		return
	}

	// Payload mirrors worker.intelChannelPayload: source_id is advisory (the
	// channel row's source_id is authoritative at handling time).
	payload, err := json.Marshal(map[string]string{
		"version":    "v1",
		"channel_id": channelID.String(),
		"source_id":  ch.SourceID.String(),
	})
	if err != nil {
		slog.Error("intel: crawl channel marshal failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "enqueue_failed", "Failed to build task payload")

		return
	}

	if _, err := h.asynqClient.EnqueueContext(r.Context(), asynq.NewTask(TaskIntelFetchChannel, payload),
		worker.TaskOpts(worker.TypeIntelFetchChannel)...); err != nil {
		slog.Error("intel: crawl channel enqueue failed", "channel_id", channelID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "enqueue_failed", "Failed to enqueue crawl task")

		return
	}

	errpkg.WriteJSON(w, http.StatusAccepted, map[string]interface{}{
		"data": map[string]interface{}{
			"channel_id": channelID.String(),
			"status":     "queued",
			"task":       TaskIntelFetchChannel,
		},
	})
}

// HandleAdminReprobeSource enqueues an intel:probe_verify task to re-run the
// deep verify probe for a source — the "Re-probe" action in the control
// panel channels editor (spec §5). The probe rewrites probe_report /
// detected_methods and materializes channels idempotently (existing rows
// are deduped on source_id+entry_url+entity_types). The worker still applies
// its own gates (disabled source, verify-exempt source_type).
// // POST /api/v1/admin/intel/sources/{id}/reprobe
// // Requires: intel.sources.probe.
func (h *Handlers) HandleAdminReprobeSource(w http.ResponseWriter, r *http.Request) {
	if h.intelSources == nil || h.asynqClient == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	if _, ok := requireUser(w, r); !ok {
		return
	}

	sourceID, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	// Verify the source exists before enqueuing (a stray id would enqueue a
	// task the worker immediately discards).
	src, err := h.intelSources.Get(r.Context(), sourceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Source not found")
			return
		}

		slog.Error("intel: reprobe source lookup failed", "source_id", sourceID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "lookup_failed", "Failed to load source")

		return
	}

	if !src.Enabled {
		errpkg.WriteError(w, http.StatusConflict, "source_disabled", "Cannot re-probe a disabled source — enable it first")
		return
	}

	// Payload mirrors worker.intelSourcePayload.
	payload, err := json.Marshal(map[string]string{
		"version":   "v1",
		"source_id": sourceID.String(),
	})
	if err != nil {
		slog.Error("intel: reprobe marshal failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "enqueue_failed", "Failed to build task payload")

		return
	}

	if _, err := h.asynqClient.EnqueueContext(r.Context(), asynq.NewTask(TaskIntelProbeVerify, payload),
		worker.TaskOpts(worker.TypeIntelProbeVerify)...); err != nil {
		slog.Error("intel: reprobe enqueue failed", "source_id", sourceID.String(), "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "enqueue_failed", "Failed to enqueue probe task")

		return
	}

	errpkg.WriteJSON(w, http.StatusAccepted, map[string]interface{}{
		"data": map[string]interface{}{
			"source_id": sourceID.String(),
			"status":    "queued",
			"task":      TaskIntelProbeVerify,
		},
	})
}
