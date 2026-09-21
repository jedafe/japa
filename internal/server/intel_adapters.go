package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/ai"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/curator"
	"github.com/jedafe/japa/internal/intelligence/extractor"
	"github.com/jedafe/japa/internal/intelligence/extractor/schemas"
	"github.com/jedafe/japa/internal/intelligence/promoter"
	"github.com/jedafe/japa/internal/intelligence/relevance"
	"github.com/jedafe/japa/internal/intelligence/sources"
	intel_stores "github.com/jedafe/japa/internal/intelligence/stores"
	"github.com/jedafe/japa/internal/studio"
	"github.com/jedafe/japa/internal/worker"
)

// intelSchedulerSourceAdapter wraps *sources.Store so it satisfies the
// scheduler.DueSourceLister and scheduler.DueChannelLister interfaces. The
// sources store exposes ListDue / ListDueChannels (the gen
// ListDueIntelSources / ListDueIntelChannels queries); the adapter passes
// the configured ID through. Per ADR-112 Amendment 2, all intelligence
// queries are platform-scoped.
type intelSchedulerSourceAdapter struct {
	store *sources.Store
}

func (a *intelSchedulerSourceAdapter) ListDue(ctx context.Context, limit int32) ([]gen.IntelSource, error) {
	return a.store.ListDue(ctx, limit)
}

// ListDueChannels satisfies scheduler.DueChannelLister — the intel-channels
// Phase 1 scheduling unit is the channel, not the source.
func (a *intelSchedulerSourceAdapter) ListDueChannels(ctx context.Context, limit int32) ([]gen.IntelSourceChannel, error) {
	return a.store.ListDueChannels(ctx, limit)
}

// intelExtractorPortAdapter wraps *extractor.Extractor so it satisfies the
// worker.intelExtractorPort interface without the worker package importing
// internal/intelligence/extractor (which would close the cycle
// extractor → ai → auth → worker). The adapter looks up the entity schema
// by entityType from the schemas registry; the worker derives entityType
// from the source's category via schemas.CategoryToEntityType.
type intelExtractorPortAdapter struct {
	ext *extractor.Extractor
}

// Extract calls the underlying extractor with the schema for the given
// entityType and returns a slice of worker.IntelExtractResult values, one
// per extracted record, with per-record aggregate confidence and a stable
// external_key. If the extractor returns ErrExtractionRetriesExhausted, the
// error is wrapped with the worker's port-level sentinel so HandleIntelExtract
// can detect it via errors.Is and route the record to REVIEW_REQUIRED instead
// of failing the task (BUG-16 fix).
//
// For RSS-sourced news records, a rule-based extractor is tried first —
// it parses the RSS XML directly and maps feed item fields to the news
// schema without an LLM call. This ensures news records get structured
// data even when no AI provider is configured (or the provider returns
// auth errors). When rule-based extraction succeeds, its result is used
// directly; otherwise the flow falls through to the LLM extractor.
func (a *intelExtractorPortAdapter) Extract(ctx context.Context, entityType, sourceURL, sourceType, sourceText string) ([]worker.IntelExtractResult, error) {
	schema, ok := schemas.ForEntityType(entityType)
	if !ok {
		return nil, fmt.Errorf("intel: no schema registered for entity type %q", entityType)
	}

	// Curate-only entity types (article, event, dataset) have no prompt
	// template and must never reach the LLM — a dispatch here is a
	// pipeline routing bug. The worker pre-checks via schemas.IsCurateOnly;
	// this is the defense-in-depth guard at the port boundary.
	if schemas.IsCurateOnly(entityType) {
		return nil, fmt.Errorf("%w: %q", schemas.ErrCurateOnlyEntityType, entityType)
	}

	// Try rule-based extraction first (RSS news feeds). This avoids an
	// LLM call for structured XML content that can be parsed directly.
	if rbResult, rbOK := extractor.RuleBasedExtract(schema, sourceURL, sourceType, sourceText); rbOK {
		return a.recordsToResults(sourceURL, rbResult.Records)
	}

	result, err := a.ext.Extract(ctx, schema, sourceURL, sourceType, sourceText)
	if err != nil {
		if errors.Is(err, extractor.ErrExtractionRetriesExhausted) {
			return nil, fmt.Errorf("%w: %w", worker.ErrIntelExtractionRetriesExhausted, err)
		}

		return nil, err
	}

	return a.recordsToResults(sourceURL, result.Records)
}

// intelPromoterPortAdapter wraps a promoter.Promoter so it satisfies the
// worker.intelPromoterPort interface without the worker package importing
// internal/intelligence/promoter. One adapter is registered per domain
// (scholarship, job, visa, housing, remittance, cost_of_living, and the
// 7 guide content types), keyed by Entity() in the promoter map.
type intelPromoterPortAdapter struct {
	prom promoter.Promoter
}

func (a *intelPromoterPortAdapter) Promote(ctx context.Context, recordID uuid.UUID) (worker.IntelPromoteResult, error) {
	result, err := a.prom.Promote(ctx, recordID)
	if err != nil {
		return worker.IntelPromoteResult{}, err
	}

	return worker.IntelPromoteResult{
		DomainID:      result.DomainID,
		ContentID:     result.ContentID,
		Action:        result.Action,
		ChangedFields: result.ChangedFields,
	}, nil
}

func (a *intelPromoterPortAdapter) Entity() string {
	return a.prom.Entity()
}

// intelCuratorPortAdapter wraps *curator.Registry so it satisfies the
// worker.intelCuratorPort interface without the worker package importing
// internal/intelligence/curator (which would close the cycle
// worker → curator → extractor → ai → auth → worker). The registry
// dispatches on the channel's crawl_method; a method with no curator maps
// curator.ErrNoCurator onto the worker's ErrIntelNoCurator sentinel so
// HandleIntelCurateChannel falls back to intel:extract.
type intelCuratorPortAdapter struct {
	reg *curator.Registry
}

func (a *intelCuratorPortAdapter) Curate(ctx context.Context, body []byte, contentType string, ch sources.Channel) ([]worker.IntelCuratedRecord, error) {
	recs, err := a.reg.Curate(ctx, body, contentType, ch)
	if err != nil {
		if errors.Is(err, curator.ErrNoCurator) {
			return nil, fmt.Errorf("%w: %w", worker.ErrIntelNoCurator, err)
		}

		return nil, err
	}

	out := make([]worker.IntelCuratedRecord, 0, len(recs))

	for i, rec := range recs {
		raw, err := json.Marshal(rec.Extraction)
		if err != nil {
			return nil, fmt.Errorf("intel: marshal curated record %d: %w", i, err)
		}

		conf := extractor.ConfidenceOverall(rec.Extraction)
		if conf <= 0 {
			conf = 1.0 // curated fields are deterministic — full confidence
		}

		out = append(out, worker.IntelCuratedRecord{
			RawExtraction:     raw,
			ConfidenceOverall: conf,
			ExternalKey:       rec.ExternalKey,
			EntityType:        rec.EntityType,
		})
	}

	return out, nil
}

// intelRelevanceClassifierAdapter adapts the §20 AI dispatcher to the
// worker's relevance.Classifier port for the borderline band (Task 10,
// spec §4 step 3). It dispatches a cheap yes/no prompt under
// category="classification"; the model comes from
// intel.relevance.classifier_model (empty → category-default routing).
// When no provider serves the category the call errors — the gate treats
// classifier errors as Borderline and the worker fails open (the item
// passes), so a missing provider never drops records.
type intelRelevanceClassifierAdapter struct {
	d        *ai.Dispatcher
	settings *config.Settings
}

// Classify returns true when the item is topically relevant to the
// channel's domain vocabulary.
func (a *intelRelevanceClassifierAdapter) Classify(ctx context.Context, item relevance.ItemInput, topics []string) (bool, error) {
	if a.d == nil || !a.d.HasCategory("classification") {
		return false, errors.New(`intel: no AI provider registered for category "classification"`)
	}

	model := ""
	if a.settings != nil {
		model = a.settings.GetStringOr(ctx, "intel.relevance.classifier_model", "")
	}

	var b strings.Builder
	b.WriteString("You are a topical relevance classifier for JapaConnect, a platform for ")
	b.WriteString("Nigerians/Africans migrating, relocating, studying abroad, working abroad, or settling overseas.\n")
	if len(topics) > 0 {
		b.WriteString("The channel's topic vocabulary is: ")
		b.WriteString(strings.Join(topics, ", "))
		b.WriteString(".\n")
	}
	b.WriteString("Decide whether this item is topically relevant (migration, relocation, travel, study abroad, jobs, scholarships, settlement, diaspora, visas, or the channel topics above).\n\n")
	b.WriteString("Title: ")
	b.WriteString(item.Title)
	b.WriteString("\nURL: ")
	b.WriteString(item.URL)
	if item.Summary != "" {
		summary := item.Summary
		if len(summary) > 500 {
			summary = summary[:500] // bound the prompt — the gate is a cheap pass
		}
		b.WriteString("\nSummary: ")
		b.WriteString(summary)
	}
	b.WriteString("\n\nRespond with ONLY a JSON object (no markdown, no explanation): {\"on_topic\": true} or {\"on_topic\": false}")

	resp, err := a.d.Dispatch(ctx, "classification", ai.Request{
		Category: "classification",
		Payload: map[string]any{
			"prompt":     b.String(),
			"model":      model,
			"max_tokens": 32,
		},
	})
	if err != nil {
		return false, fmt.Errorf("intel: relevance classification dispatch: %w", err)
	}

	return parseClassifierVerdict(intelClassifierText(resp.Raw))
}

// intelClassifierText pulls the assistant message text out of an
// OpenAI-compatible choices array in the dispatch Response.Raw map.
// Mirrors spam.extractChoiceText (private to that package).
func intelClassifierText(raw map[string]any) string {
	choices, ok := raw["choices"].([]any)
	if !ok || len(choices) == 0 {
		return ""
	}

	first, ok := choices[0].(map[string]any)
	if !ok {
		return ""
	}

	msg, ok := first["message"].(map[string]any)
	if !ok {
		return ""
	}

	content, _ := msg["content"].(string)

	return content
}

// parseClassifierVerdict decodes the classifier's answer: the JSON shape
// {"on_topic": bool} first, then a lenient substring fallback for
// providers that answer "on_topic"/"off_topic"/"yes"/"no" bare.
func parseClassifierVerdict(text string) (bool, error) {
	cleaned := strings.TrimSpace(text)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	var out struct {
		OnTopic  *bool `json:"on_topic"`
		Relevant *bool `json:"relevant"`
	}
	if err := json.Unmarshal([]byte(cleaned), &out); err == nil {
		if out.OnTopic != nil {
			return *out.OnTopic, nil
		}
		if out.Relevant != nil {
			return *out.Relevant, nil
		}
	}

	switch strings.ToLower(cleaned) {
	case "on_topic", "yes", "true", "relevant":
		return true, nil
	case "off_topic", "no", "false", "irrelevant", "not relevant":
		return false, nil
	}

	return false, fmt.Errorf("intel: unparseable classifier response: %q", cleaned)
}

// recordsToResults converts validated extractor records into the worker's
// IntelExtractResult slice, deriving a stable external_key and per-record
// aggregate confidence for each. An empty input yields an empty slice (not
// an error): a mixed-entity-type source legitimately extracts nothing for
// some of its declared types on a given document — the worker only fails
// the task when every type produced nothing. It returns an error if any
// record cannot be marshalled to JSON.
func (a *intelExtractorPortAdapter) recordsToResults(sourceURL string, records []extractor.Extraction) ([]worker.IntelExtractResult, error) {
	if len(records) == 0 {
		return nil, nil
	}

	out := make([]worker.IntelExtractResult, 0, len(records))

	for i, rec := range records {
		raw, err := json.Marshal(rec)
		if err != nil {
			return nil, fmt.Errorf("intel: marshal record %d: %w", i, err)
		}

		out = append(out, worker.IntelExtractResult{
			RawExtraction:     raw,
			ConfidenceOverall: extractor.ConfidenceOverall(rec),
			ExternalKey:       deriveExternalKey(sourceURL, rec, i),
		})
	}

	return out, nil
}

// deriveExternalKey returns a stable external key for one extracted record.
// It prefers the record's own application_url, then falls back to a hash of
// the source URL + title + deadline, and finally to a content-derived hash
// of all non-null field values sorted by key. The index placeholder is only
// used when the record contains no usable values at all.
func deriveExternalKey(sourceURL string, ext extractor.Extraction, index int) string {
	if u, ok := stringField(ext, "application_url"); ok && u != "" {
		return sourceURL + "|" + u
	}

	title, _ := stringField(ext, "title")
	deadline, _ := stringField(ext, "deadline")

	if title != "" {
		h := sha256.Sum256([]byte(sourceURL + "|" + title + "|" + deadline))
		return sourceURL + "|" + hex.EncodeToString(h[:])
	}

	// Worst case: build a deterministic key from all non-null field values.
	keys := make([]string, 0, len(ext))
	for k := range ext {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	var b strings.Builder

	for _, k := range keys {
		f := ext[k]
		if f.Value == nil {
			continue
		}

		v, err := json.Marshal(f.Value)
		if err != nil {
			continue
		}

		if b.Len() > 0 {
			b.WriteByte('&')
		}

		b.WriteString(k)
		b.WriteByte('=')
		b.Write(v)
	}

	if b.Len() == 0 {
		return fmt.Sprintf("%s|idx:%d", sourceURL, index)
	}

	h := sha256.Sum256([]byte(sourceURL + "|" + b.String()))

	return sourceURL + "|" + hex.EncodeToString(h[:])
}

// stringField returns a string value from an extraction record when the field
// is present, non-null, and of string type.
func stringField(ext extractor.Extraction, key string) (string, bool) {
	f, ok := ext[key]
	if !ok || f.Value == nil {
		return "", false
	}

	s, ok := f.Value.(string)

	return s, ok
}

// intelPromoterStoreAdapter wraps *stores.Store so it satisfies the
// promoter.Store interface, including the WithTx method whose callback
// type differs between the two packages. The embedded *stores.Store
// provides all CRUD methods via promotion; the adapter only overrides
// WithTx to bridge func(*stores.Store) error → func(promoter.TxStore) error
// (BUG-01/02 fix: §6.2 transaction boundary).
type intelPromoterStoreAdapter struct {
	*intel_stores.Store
}

func (a *intelPromoterStoreAdapter) WithTx(ctx context.Context, fn func(promoter.TxStore) error) error {
	return a.Store.WithTx(ctx, func(s *intel_stores.Store) error {
		return fn(s)
	})
}

// intelJobPromoterStoreAdapter wraps *stores.Store for the JobPromoter's
// JobStore interface. The WithTx bridges func(*stores.Store) error →
// func(promoter.JobTxStore) error.
type intelJobPromoterStoreAdapter struct {
	*intel_stores.Store
}

func (a *intelJobPromoterStoreAdapter) WithTx(ctx context.Context, fn func(promoter.JobTxStore) error) error {
	return a.Store.WithTx(ctx, func(s *intel_stores.Store) error {
		return fn(s)
	})
}

// intelVisaPromoterStoreAdapter wraps *stores.Store for the VisaPromoter's
// VisaStore interface.
type intelVisaPromoterStoreAdapter struct {
	*intel_stores.Store
}

func (a *intelVisaPromoterStoreAdapter) WithTx(ctx context.Context, fn func(promoter.VisaTxStore) error) error {
	return a.Store.WithTx(ctx, func(s *intel_stores.Store) error {
		return fn(s)
	})
}

// intelHousingPromoterStoreAdapter wraps *stores.Store for the
// HousingPromoter's HousingStore interface.
type intelHousingPromoterStoreAdapter struct {
	*intel_stores.Store
}

func (a *intelHousingPromoterStoreAdapter) WithTx(ctx context.Context, fn func(promoter.HousingTxStore) error) error {
	return a.Store.WithTx(ctx, func(s *intel_stores.Store) error {
		return fn(s)
	})
}

// intelRemittancePromoterStoreAdapter wraps *stores.Store for the
// RemittancePromoter's RemittanceStore interface.
type intelRemittancePromoterStoreAdapter struct {
	*intel_stores.Store
}

func (a *intelRemittancePromoterStoreAdapter) WithTx(ctx context.Context, fn func(promoter.RemittanceTxStore) error) error {
	return a.Store.WithTx(ctx, func(s *intel_stores.Store) error {
		return fn(s)
	})
}

// intelCostOfLivingPromoterStoreAdapter wraps *stores.Store for the
// CostOfLivingPromoter's CostOfLivingStore interface.
type intelCostOfLivingPromoterStoreAdapter struct {
	*intel_stores.Store
}

func (a *intelCostOfLivingPromoterStoreAdapter) WithTx(ctx context.Context, fn func(promoter.CostOfLivingTxStore) error) error {
	return a.Store.WithTx(ctx, func(s *intel_stores.Store) error {
		return fn(s)
	})
}

// intelGuidePromoterStoreAdapter wraps *stores.Store for the
// GuideContentPromoter's GuideContentStore interface (used by the 7 pure
// guide content type domains).
type intelGuidePromoterStoreAdapter struct {
	*intel_stores.Store
}

func (a *intelGuidePromoterStoreAdapter) WithTx(ctx context.Context, fn func(promoter.GuideContentTxStore) error) error {
	return a.Store.WithTx(ctx, func(s *intel_stores.Store) error {
		return fn(s)
	})
}

// intelWikiPromoterStoreAdapter wraps *stores.Store for the WikiPromoter's
// WikiStore interface.
type intelWikiPromoterStoreAdapter struct {
	*intel_stores.Store
}

func (a *intelWikiPromoterStoreAdapter) WithTx(ctx context.Context, fn func(promoter.WikiTxStore) error) error {
	return a.Store.WithTx(ctx, func(s *intel_stores.Store) error {
		return fn(s)
	})
}

// intelLanguageTestPromoterStoreAdapter wraps *stores.Store for the
// LanguageTestPromoter's LanguageTestStore interface.
type intelLanguageTestPromoterStoreAdapter struct {
	*intel_stores.Store
}

func (a *intelLanguageTestPromoterStoreAdapter) WithTx(ctx context.Context, fn func(promoter.LanguageTestTxStore) error) error {
	return a.Store.WithTx(ctx, func(s *intel_stores.Store) error {
		return fn(s)
	})
}

// intelTaxPromoterStoreAdapter wraps *stores.Store for the TaxPromoter's
// TaxStore interface.
type intelTaxPromoterStoreAdapter struct {
	*intel_stores.Store
}

func (a *intelTaxPromoterStoreAdapter) WithTx(ctx context.Context, fn func(promoter.TaxTxStore) error) error {
	return a.Store.WithTx(ctx, func(s *intel_stores.Store) error {
		return fn(s)
	})
}

// intelCommunityOrgPromoterStoreAdapter wraps *stores.Store for the
// CommunityOrgPromoter's CommunityOrgStore interface.
type intelCommunityOrgPromoterStoreAdapter struct {
	*intel_stores.Store
}

func (a *intelCommunityOrgPromoterStoreAdapter) WithTx(ctx context.Context, fn func(promoter.CommunityOrgTxStore) error) error {
	return a.Store.WithTx(ctx, func(s *intel_stores.Store) error {
		return fn(s)
	})
}

// ---------------------------------------------------------------------------
// Studio auto-promote adapter (ADR-076 Amendment 1)
// ---------------------------------------------------------------------------

// intelEntityTypeToStudioContentType maps an intel_records.entity_type to
// the Studio content_type used when auto-promoting a high-confidence
// extracted record. Mirrors the ENTITY_TO_CONTENT_TYPE map in the admin
// review page. Entity types that map to "" are skipped (no Studio content
// type — e.g. guide_content types whose promoter already wrote contents).
// #nosec G101 — entity type → content type mapping, not credentials.
var intelEntityTypeToStudioContentType = map[string]string{
	"scholarship":               "guide",
	"job":                       "job_listing",
	"visa":                      "visa_route",
	"housing":                   "housing_guide",
	"wiki_page":                 "wiki_page",
	"language_test_requirement": "language_test_requirement",
	"tax_obligation_guide":      "tax_obligation_guide",
	"community_organisation":    "community_organisation",
	"cost_of_living":            "comparison",
	"remittance":                "guide",
	"credential_pathway":        "credential_pathway",
	// guide_content entity types are intentionally omitted: their promoter
	// writes the contents row directly and returns a real ContentID, so
	// the worker's auto-promote gate (ContentID == uuid.Nil) already skips
	// them before reaching the adapter.
}

// intelCredentialPromoterStoreAdapter wraps *stores.Store for the
// CredentialPromoter's CredentialStore interface.
type intelCredentialPromoterStoreAdapter struct {
	*intel_stores.Store
}

func (a *intelCredentialPromoterStoreAdapter) WithTx(ctx context.Context, fn func(promoter.CredentialTxStore) error) error {
	return a.Store.WithTx(ctx, func(s *intel_stores.Store) error {
		return fn(s)
	})
}

// studioAutoPromoterAdapter wraps *studio.Service so it satisfies the
// worker.IntelStudioAutoPromoter port without the worker package importing
// internal/studio (which would close the cycle
// worker → studio → config → ... → worker). The adapter resolves the
// default persona from studio.default_persona_id, maps the entity_type to
// a Studio content_type, derives a topic prompt from the extracted JSON,
// and calls CreateJob with the intel_record_id set for grounding.
type studioAutoPromoterAdapter struct {
	svc      *studio.Service
	settings *config.Settings
	authorID pgtype.UUID // system user stamped as created_by
}

// AutoPromoteIntelRecord creates a Studio job grounded by the given
// intel_record_id. It is called by HandleIntelPromote when
// studio.auto_promote_intel_records is true and the record's confidence
// meets the intel.extraction_confidence_auto threshold.
func (a *studioAutoPromoterAdapter) AutoPromoteIntelRecord(ctx context.Context, recordID pgtype.UUID, entityType string, extractedJSON []byte) error {
	contentType, ok := intelEntityTypeToStudioContentType[entityType]
	if !ok || contentType == "" {
		return nil // unmapped entity type — skip silently
	}

	topic := deriveTopicFromExtraction(extractedJSON)
	if topic == "" {
		topic = fmt.Sprintf("Auto-promoted %s from intelligence", entityType)
	}

	// Resolve the default persona (studio.default_persona_id, §75).
	var personaID pgtype.UUID

	if a.settings != nil {
		if pidStr := a.settings.GetStringOr(ctx, "studio.default_persona_id", ""); pidStr != "" {
			_ = personaID.Scan(pidStr)
		}
	}

	_, err := a.svc.CreateJob(ctx, a.authorID, studio.CreateJobInput{
		PersonaID:     personaID,
		ContentType:   contentType,
		TopicPrompt:   topic,
		IntelRecordID: recordID,
	})

	return err
}

// deriveTopicFromExtraction parses the extracted JSON payload and returns
// a human-readable topic prompt. It prefers "title", then "name", then
// falls back to "" (the caller substitutes a default). Mirrors the
// topicPromptForRecord logic in the admin review page.
func deriveTopicFromExtraction(extractedJSON []byte) string {
	if len(extractedJSON) == 0 {
		return ""
	}
	// The extracted payload is a map of field_name → {value, confidence,
	// evidence}. We only need the "value" of "title" or "name".
	var fields map[string]struct {
		Value any `json:"value"`
	}

	if err := json.Unmarshal(extractedJSON, &fields); err != nil {
		return ""
	}

	if f, ok := fields["title"]; ok {
		if s, ok := f.Value.(string); ok && s != "" {
			return s
		}
	}

	if f, ok := fields["name"]; ok {
		if s, ok := f.Value.(string); ok && s != "" {
			return s
		}
	}

	return ""
}
