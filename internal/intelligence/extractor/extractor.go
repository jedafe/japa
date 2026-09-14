package extractor

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/ai"
	"github.com/jedafe/japa/internal/intelligence/extractor/schemas"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// extractionCacheTTL is the TTL for the LLM extraction result cache (24
// hours per valkey-keys-catalog Section INTEL).
const extractionCacheTTL = 24 * time.Hour

// AIGateway is the minimal interface the extractor needs from §20. The
// concrete *ai.Dispatcher satisfies this interface; tests substitute a mock.
type AIGateway interface {
	ChatWithTools(ctx context.Context, category string, req ai.ChatWithToolsRequest) (ai.ChatWithToolsResponse, error)
}

// Extractor calls the AI gateway to extract structured entity fields from
// source text, then validates the output against the entity schema.
// // When a Valkey client is provided via WithCache, extraction results are
// cached under jc:intel:extraction_cache:{}:{content_hash} (24hr
// TTL) so identical source content skips the LLM call on subsequent crawls
// (BUG-18 fix, valkey-keys-catalog Section INTEL, tenant-scoped per
// ADR-112 Amendment 2).
type Extractor struct {
	gateway     AIGateway
	category    string
	model       string
	temperature float64
	maxTokens   int
	maxRetries  int
	timeout     time.Duration
	rdb         valkeycompat.Cmdable
}

// ExtractorOption configures an Extractor.
// //nolint:revive // exported: stutter naming is intentional
type ExtractorOption func(*Extractor)

// WithModel sets the LLM model identifier passed in the request.
func WithModel(model string) ExtractorOption {
	return func(e *Extractor) { e.model = model }
}

// WithTemperature sets the sampling temperature (default 0.0 for determinism).
func WithTemperature(t float64) ExtractorOption {
	return func(e *Extractor) { e.temperature = t }
}

// WithMaxTokens sets the max output tokens for the completion.
func WithMaxTokens(n int) ExtractorOption {
	return func(e *Extractor) { e.maxTokens = n }
}

// WithMaxRetries sets the number of retries on malformed JSON (default 2).
func WithMaxRetries(n int) ExtractorOption {
	return func(e *Extractor) { e.maxRetries = n }
}

// WithTimeout sets the per-call timeout for the AI gateway call.
func WithTimeout(d time.Duration) ExtractorOption {
	return func(e *Extractor) { e.timeout = d }
}

// WithCategory sets the AI gateway category to dispatch to
// (default "extraction").
func WithCategory(c string) ExtractorOption {
	return func(e *Extractor) { e.category = c }
}

// WithCache enables Valkey-backed extraction result caching. Identical
// source content (by SHA-256 hash) skips the LLM call and returns the
// cached extraction result. The cache key is
// jc:intel:extraction_cache:{}:{content_hash} with a 24hr TTL
// per valkey-keys-catalog Section INTEL (BUG-18 fix, tenant-scoped per
// ADR-112 Amendment 2).
func WithCache(rdb valkeycompat.Cmdable) ExtractorOption {
	return func(e *Extractor) { e.rdb = rdb }
}

// NewExtractor creates an Extractor backed by the given AI gateway.
func NewExtractor(gw AIGateway, opts ...ExtractorOption) *Extractor {
	e := &Extractor{
		gateway:     gw,
		category:    "extraction",
		temperature: 0.0,
		maxTokens:   4096,
		maxRetries:  2,
		timeout:     60 * time.Second,
	}
	for _, opt := range opts {
		opt(e)
	}

	return e
}

// ExtractResult is the output of a successful extraction: one or more
// validated records plus validation errors (which are non-fatal — the
// hallucination guard may reset fields to null while still returning data).
// Extraction is the deprecated single-record fallback kept for cache
// compatibility; new callers should use Records.
type ExtractResult struct {
	Records    []Extraction // one or more validated records
	Extraction Extraction   // deprecated/single-record fallback; kept for cache compatibility
	Errors     []ValidationError
}

// Extract calls the AI gateway with the scholarship prompt, parses the JSON
// response, and validates it against the scholarship schema. It retries up to
// maxRetries times when the LLM returns malformed JSON.
// // When a Valkey client is configured (via WithCache), the extraction result
// is cached under jc:intel:extraction_cache:{}:{content_hash}
// (24hr TTL) so identical source content skips the LLM call on subsequent
// crawls (BUG-18 fix, valkey-keys-catalog Section INTEL, tenant-scoped per
// ADR-112 Amendment 2).
func (e *Extractor) Extract(ctx context.Context, schema schemas.EntitySchema, sourceURL, sourceType, sourceText string) (ExtractResult, error) {
	// Check extraction cache (BUG-18 fix). The cache key is the SHA-256 of
	// the source text + entity type, so identical content across re-crawls
	// returns the cached extraction without an LLM call.
	if result, ok := e.cachedExtraction(ctx, schema, sourceText); ok {
		return result, nil
	}

	prompt, err := buildPrompt(schema, sourceURL, sourceType, sourceText)
	if err != nil {
		return ExtractResult{}, err
	}

	var lastErr error

	for attempt := 0; attempt <= e.maxRetries; attempt++ {
		callCtx, cancel := context.WithTimeout(ctx, e.timeout)
		resp, err := e.gateway.ChatWithTools(callCtx, e.category, ai.ChatWithToolsRequest{
			Messages: []ai.ChatMessage{
				{Role: "user", Content: prompt},
			},
			Model:       e.model, // empty = use provider's configured default
			Temperature: e.temperature,
			MaxTokens:   e.maxTokens,
		})

		cancel()

		if err != nil {
			lastErr = err
			continue
		}

		raw := stripFences(resp.Content)

		ext, perr := ParseExtraction([]byte(raw))
		if perr != nil {
			lastErr = perr
			continue
		}

		rawList := splitRecords(ext)
		if len(rawList) == 0 {
			// Single record.
			cleaned, verrs := Validate(ext, schema)
			result := ExtractResult{
				Records:    []Extraction{cleaned},
				Extraction: cleaned,
				Errors:     verrs,
			}

			// Cache the successful extraction (BUG-18 fix).
			if e.rdb != nil {
				if encoded, err := json.Marshal(result); err == nil {
					e.rdb.Set(ctx, e.extractionCacheKey(schema.EntityType, sourceText), string(encoded), extractionCacheTTL)
				}
			}

			return result, nil
		}

		records := make([]Extraction, 0, len(rawList))

		var allErrs []ValidationError

		for _, rec := range rawList {
			cleaned, verrs := Validate(rec, schema)
			records = append(records, cleaned)
			allErrs = append(allErrs, verrs...)
		}

		if len(records) == 0 {
			lastErr = errors.New("records array contained no valid records")
			continue
		}

		result := ExtractResult{
			Records:    records,
			Extraction: records[0],
			Errors:     allErrs,
		}

		// Cache the successful extraction (BUG-18 fix).
		if e.rdb != nil {
			if encoded, err := json.Marshal(result); err == nil {
				e.rdb.Set(ctx, e.extractionCacheKey(schema.EntityType, sourceText), string(encoded), extractionCacheTTL)
			}
		}

		return result, nil
	}

	return ExtractResult{}, fmt.Errorf("extractor: %w (last error: %w)", ErrExtractionRetriesExhausted, lastErr)
}

// cachedExtraction returns the cached extraction result for the source text
// when present and valid. A corrupt or legacy cache entry falls through to
// the LLM call — returning ok=false.
func (e *Extractor) cachedExtraction(ctx context.Context, schema schemas.EntitySchema, sourceText string) (ExtractResult, bool) {
	if e.rdb == nil {
		return ExtractResult{}, false
	}

	cached, err := e.rdb.Get(ctx, e.extractionCacheKey(schema.EntityType, sourceText)).Result()
	if err != nil {
		return ExtractResult{}, false
	}

	var result ExtractResult
	if err := json.Unmarshal([]byte(cached), &result); err != nil {
		return ExtractResult{}, false
	}

	// Backward compatibility: cached entries written before the multi-record
	// change may only have Extraction. Normalize to Records so downstream
	// consumers always see a slice.
	if len(result.Records) == 0 && len(result.Extraction) > 0 {
		result.Records = []Extraction{result.Extraction}
	}

	return result, true
}

// extractionCacheKey returns the Valkey key for the extraction cache.
// The key is jc:intel:extraction_cache:{}:{content_hash} where
// content_hash is the SHA-256 of the entity type + source text, ensuring
// identical content across re-crawls hits the cache. The scopes
// the cache per tenant (ADR-112 Amendment 2).
func (e *Extractor) extractionCacheKey(entityType, sourceText string) string {
	h := sha256.Sum256([]byte(entityType + ":" + sourceText))
	return "jc:intel:extraction_cache:" + pgtype.UUID{}.String() + ":" + hex.EncodeToString(h[:])
}

// stripFences removes surrounding markdown code fences (```json ... ```)
// that some LLMs emit despite instructions not to.
func stripFences(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	// Drop the opening fence line.
	if idx := strings.Index(s, "\n"); idx >= 0 {
		s = s[idx+1:]
	}

	s = strings.TrimSpace(s)
	// Drop the closing fence.
	s = strings.TrimSuffix(s, "```")

	return strings.TrimSpace(s)
}

// splitRecords normalizes an extraction that may contain a top-level "records" array.
// It returns nil if the extraction is a single record.
func splitRecords(ext Extraction) []Extraction {
	rawRecords, ok := ext["records"]
	if !ok || rawRecords.Value == nil {
		return nil
	}

	list, ok := rawRecords.Value.([]any)
	if !ok {
		return nil
	}

	out := make([]Extraction, 0, len(list))

	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}

		rec := extractionFromMap(m)
		if len(rec) > 0 {
			out = append(out, rec)
		}
	}

	return out
}

// ConfidenceOverall returns the mean of per-field confidence values.
func ConfidenceOverall(ext Extraction) float32 {
	if len(ext) == 0 {
		return 0
	}

	var sum float64

	var n int

	for _, f := range ext {
		if f.Confidence == 0 && f.Value == nil {
			continue // skip null fields from mean
		}

		sum += f.Confidence
		n++
	}

	if n == 0 {
		return 0
	}

	return float32(sum / float64(n))
}

// ErrEmptyContent is returned when the AI gateway returns an empty content
// body that cannot be parsed.
var ErrEmptyContent = errors.New("extractor: AI gateway returned empty content")

// ErrExtractionRetriesExhausted is returned when the AI gateway returns
// malformed JSON on every attempt up to maxRetries. Per the intelligence
// design spec, this should route the intel record to REVIEW_REQUIRED
// status rather than failing the Asynq task (which would retry the whole
// task and waste tokens). The worker's HandleIntelExtract checks for this
// sentinel and sets the record status to 'review_required' instead of
// returning an error (BUG-16 fix).
var ErrExtractionRetriesExhausted = errors.New("extractor: retries exhausted on malformed JSON")

//go:embed prompts/scholarship.txt
var scholarshipPrompt string

//go:embed prompts/job.txt
var jobPrompt string

//go:embed prompts/visa.txt
var visaPrompt string

//go:embed prompts/remittance.txt
var remittancePrompt string

//go:embed prompts/cost_of_living.txt
var costOfLivingPrompt string

//go:embed prompts/housing.txt
var housingPrompt string

//go:embed prompts/travel_routes.txt
var travelRoutesPrompt string

//go:embed prompts/grants.txt
var grantsPrompt string

//go:embed prompts/admission.txt
var admissionPrompt string

//go:embed prompts/banking.txt
var bankingPrompt string

//go:embed prompts/work_career.txt
var workCareerPrompt string

//go:embed prompts/health_insurance.txt
var healthInsurancePrompt string

//go:embed prompts/credential_pathway.txt
var credentialPathwayPrompt string

//go:embed prompts/culture_lifestyle.txt
var cultureLifestylePrompt string

//go:embed prompts/news.txt
var newsPrompt string

// promptTemplates maps entity_type → embedded prompt template. Adding a
// domain = add a //go:embed line + an entry here. No buildPrompt code
// change.
var promptTemplates = map[string]string{
	schemas.EntityTypeScholarship:       scholarshipPrompt,
	schemas.EntityTypeJob:               jobPrompt,
	schemas.EntityTypeVisa:              visaPrompt,
	schemas.EntityTypeRemittance:        remittancePrompt,
	schemas.EntityTypeCostOfLiving:      costOfLivingPrompt,
	schemas.EntityTypeHousing:           housingPrompt,
	schemas.EntityTypeTravelRoutes:      travelRoutesPrompt,
	schemas.EntityTypeGrants:            grantsPrompt,
	schemas.EntityTypeAdmission:         admissionPrompt,
	schemas.EntityTypeBanking:           bankingPrompt,
	schemas.EntityTypeWorkCareer:        workCareerPrompt,
	schemas.EntityTypeHealthInsurance:   healthInsurancePrompt,
	schemas.EntityTypeCultureLifestyle:  cultureLifestylePrompt,
	schemas.EntityTypeCredentialPathway: credentialPathwayPrompt,
	schemas.EntityTypeNews:              newsPrompt,
}

// buildPrompt loads the prompt template for the given entity schema and
// substitutes the placeholders. The template is selected by
// schema.EntityType from the promptTemplates registry.
func buildPrompt(schema schemas.EntitySchema, sourceURL, sourceType, sourceText string) (string, error) {
	tmpl, ok := promptTemplates[schema.EntityType]
	if !ok {
		return "", fmt.Errorf("extractor: no prompt template for entity type %q", schema.EntityType)
	}

	schemaDesc := describeSchema(schema)
	out := strings.ReplaceAll(tmpl, "{{ENTITY_SCHEMA}}", schemaDesc)
	out = strings.ReplaceAll(out, "{{SOURCE_URL}}", sourceURL)
	out = strings.ReplaceAll(out, "{{SOURCE_TYPE}}", sourceType)
	out = strings.ReplaceAll(out, "{{SOURCE_TEXT}}", sourceText)

	return out, nil
}

// describeSchema renders the schema as a human-readable field list for the
// prompt's ENTITY_SCHEMA placeholder.
func describeSchema(schema schemas.EntitySchema) string {
	var b strings.Builder

	fmt.Fprintf(&b, "entity_type: %s\nfields:", schema.EntityType)

	for _, f := range schema.Fields {
		req := "optional"
		if f.Required {
			req = "required"
		}

		enum := ""
		if len(f.EnumValues) > 0 {
			enum = fmt.Sprintf(" (enum: %s)", strings.Join(f.EnumValues, ", "))
		}

		fmt.Fprintf(&b, "\n  - %s: %s, %s%s", f.Name, f.Type, req, enum)
	}

	return b.String()
}
