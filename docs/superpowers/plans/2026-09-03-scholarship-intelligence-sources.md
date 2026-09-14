# Scholarship Intelligence Sources — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` to implement this plan task-by-task. The `executing-plans` skill is not available in this environment, so inline execution means the parent agent runs the tasks manually. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire Open Scholarships, Grants.gov, DAAD, Erasmus Mundus, and Chevening into the intelligence ingestion pipeline so live scholarships are fetched, extracted, and — when confidence + trust thresholds pass — published immediately as `scholarships` + `contents` rows.

**Architecture:** Extend the generic `api` fetcher to support POST/JSON body and response path extraction. The extractor returns `ExtractResult.Records`, a slice of validated `Extraction` maps, so list/catalogue sources can yield multiple scholarships. The `server` extractor adapter derives a stable `external_key` and per-record `confidence_overall`, and returns `[]worker.IntelExtractResult`. The worker's `HandleIntelExtract` loops and creates one `intel_record` + `intel:dedup` per record. The `ScholarshipPromoter` creates the `contents` guide on the create path and links it via `scholarships.guide_id`. Five `intel_sources` rows are seeded via migration.

**Tech Stack:** Go 1.23+, pgx, sqlc, Asynq/Valkey, goquery, OpenAI/gpt-4o-mini via §20 AI gateway.

## Global Constraints

- **No hardcoded values:** source URLs, crawl frequency, and confidence thresholds come from `intel_sources` rows or `system_settings`.
- **Tenant scoping:** every DB query filters by `tenant_id`; every Valkey key includes `{tenant_id}`.
- **No hand-edited `internal/db/gen` files:** edit `.sql` query files, then run `make sqlc-gen`.
- **No new Asynq task names without catalog entry:** reuse existing `intel:*` tasks; no new task types required.
- **No AI attribution in commits/PRs per `AGENTS.md` Rule 17.**
- **All contents rows must have a computed `content_hash` per Rule 12** and `canonical_url` when the source provides one.
- **Import cycles:** `internal/worker` cannot import `internal/intelligence/extractor` or `internal/intelligence/promoter`. All logic that needs those types must live in `server/intel_adapters.go` or the source package itself.

---

### Task 1: Extend `APIProviderConfig` and `APIFetcher`

**Files:**
- Modify: `internal/intelligence/fetcher/api.go`
- Test: `internal/intelligence/fetcher/api_test.go`

**Interfaces:**
- Consumes: `sources.Source.ProviderConfig` JSON.
- Produces: `FetchResult.Body` containing either the raw JSON or the sub-value at `provider_config.path`.

- [ ] **Step 1: Add `Method`, `Body`, and `Path` to `APIProviderConfig`.**

```go
type APIProviderConfig struct {
	Endpoint string            `json:"endpoint"`
	Method   string            `json:"method,omitempty"`
	Body     json.RawMessage   `json:"body,omitempty"`
	Path     string            `json:"path,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
}
```

- [ ] **Step 2: Implement `selectByPath` helper.**

Add to `api.go`:

```go
func selectByPath(data []byte, path string) ([]byte, error) {
	if path == "" {
		return data, nil
	}
	var root any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("api: unmarshal body for path selection: %w", err)
	}
	parts := strings.Split(path, ".")
	for _, p := range parts {
		switch v := root.(type) {
		case map[string]any:
			next, ok := v[p]
			if !ok {
				return nil, fmt.Errorf("api: path %q not found at segment %q", path, p)
			}
			root = next
		default:
			return nil, fmt.Errorf("api: cannot traverse path %q at segment %q", path, p)
		}
	}
	return json.Marshal(root)
}
```

- [ ] **Step 3: Update `APIFetcher.Fetch` to use `cfg.Method` and `cfg.Body`.**

```go
method := strings.ToUpper(cfg.Method)
if method == "" {
    method = http.MethodGet
}
var bodyReader io.Reader
if len(cfg.Body) > 0 {
    bodyReader = bytes.NewReader(cfg.Body)
}
req, err := http.NewRequestWithContext(ctx, method, cfg.Endpoint, bodyReader)
if len(cfg.Body) > 0 {
    req.Header.Set("Content-Type", "application/json")
}
```

After reading `body`, if `cfg.Path != ""`, call `selectByPath(body, cfg.Path)` and use the result as `FetchResult.Body`. Recompute `ContentHash` with the selected body.

- [ ] **Step 4: Add tests for POST, JSON body, and path extraction.**

Test server returns the Grants.gov fixture. Assert `FetchResult.Body` equals the extracted `opportunityDetails` array and `ContentHash` is stable.

- [ ] **Step 5: Run tests and commit.**

```bash
go test ./internal/intelligence/fetcher/... -run TestAPIFetcher -v
```

---

### Task 2: Support multi-record extraction

**Files:**
- Modify: `internal/intelligence/extractor/extractor.go`, `internal/intelligence/extractor/validate.go`, `internal/intelligence/extractor/prompts/scholarship.txt`, `internal/intelligence/extractor/rss_extract.go`, `internal/server/intel_adapters.go`, `internal/worker/intelligence.go`
- Test: `internal/intelligence/extractor/extractor_test.go`, `internal/intelligence/integration_test.go`, `internal/worker/intelligence_test.go` (if it exists)

**Interfaces:**
- Consumes: raw document text and the source row.
- Produces: `[]worker.IntelExtractResult`, one per scholarship, with stable `external_key` and per-record `confidence_overall`.

**Why this is in the server adapter, not the worker:** `internal/worker` cannot import `internal/intelligence/extractor` (import cycle: extractor → ai → auth → worker). The adapter already wraps `*extractor.Extractor` and converts to `worker.IntelExtractResult`, so the split and per-record key derivation happen there.

- [ ] **Step 1: Update `ExtractResult` to carry a `Records` slice.**

In `internal/intelligence/extractor/extractor.go`:

```go
type ExtractResult struct {
	Records    []Extraction     // one or more validated records
	Extraction Extraction       // deprecated/single-record fallback; kept for cache compatibility
	Errors     []ValidationError
}
```

- [ ] **Step 2: Update `scholarship.txt` prompt to return a top-level `records` array.**

Append:

```text
13. The source document may contain multiple scholarship records. If it does, return a JSON object with exactly one key: "records", whose value is an array of record objects matching the schema above. If it contains only one record, you may return the record object directly or an array of length 1 under "records".
```

- [ ] **Step 3: Add `extractor.ConfidenceOverall` and an unexported `splitRecords` helper.**

In `internal/intelligence/extractor/extractor.go`:

```go
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
        rec := make(Extraction)
        for k, v := range m {
            fm, ok := v.(map[string]any)
            if !ok {
                continue
            }
            conf, _ := fm["confidence"].(float64)
            var ev *string
            if e, ok := fm["evidence"].(string); ok {
                ev = &e
            }
            rec[k] = FieldExtraction{Value: fm["value"], Confidence: conf, Evidence: ev}
        }
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
```

- [ ] **Step 4: Update `extractor.Extract` to validate one or more records and set `Records`.**

After `ParseExtraction`:

```go
rawList := splitRecords(ext)
if len(rawList) == 0 {
    // single record
    cleaned, verrs := Validate(ext, schema)
    result := ExtractResult{Records: []Extraction{cleaned}, Errors: verrs}
    // cache ...
    return result, nil
}

records := make([]Extraction, 0, len(rawList))
var allErrs []ValidationError
for _, rec := range rawList {
    cleaned, verrs := Validate(rec, schema)
    records = append(records, cleaned)
    allErrs = append(allErrs, verrs...)
}
result := ExtractResult{Records: records, Errors: allErrs}
// cache ...
return result, nil
```

For caching, `ExtractResult` now serializes `Records` instead of `Extraction`. The existing cache key stays the same.

- [ ] **Step 5: Update `RuleBasedExtract` to set `Records`.**

In `rss_extract.go`:

```go
return ExtractResult{Records: []Extraction{cleaned}, Errors: verrs}, true
```

- [ ] **Step 6: Change `worker.IntelExtractResult` to carry `ExternalKey` and switch the port to return a slice.**

In `internal/worker/intelligence.go`:

```go
type intelExtractorPort interface {
    Extract(ctx context.Context, entityType, sourceURL, sourceType, sourceText string) ([]IntelExtractResult, error)
}

type IntelExtractResult struct {
    RawExtraction     []byte
    ConfidenceOverall float32
    ExternalKey       string
}
```

- [ ] **Step 7: Update `server/intel_adapters.go` `intelExtractorPortAdapter.Extract` to loop `result.Records`.**

```go
func (a *intelExtractorPortAdapter) Extract(ctx context.Context, entityType, sourceURL, sourceType, sourceText string) ([]worker.IntelExtractResult, error) {
    schema, ok := schemas.ForEntityType(entityType)
    if !ok {
        return nil, fmt.Errorf("intel: no schema registered for entity type %q", entityType)
    }

    if rbResult, rbOK := extractor.RuleBasedExtract(schema, sourceURL, sourceType, sourceText); rbOK {
        return a.recordsToResults(sourceURL, rbResult.Records), nil
    }

    result, err := a.ext.Extract(ctx, schema, sourceURL, sourceType, sourceText)
    if err != nil {
        if errors.Is(err, extractor.ErrExtractionRetriesExhausted) {
            return nil, fmt.Errorf("%w: %v", worker.ErrIntelExtractionRetriesExhausted, err)
        }
        return nil, err
    }
    return a.recordsToResults(sourceURL, result.Records), nil
}

func (a *intelExtractorPortAdapter) recordsToResults(sourceURL string, records []extractor.Extraction) []worker.IntelExtractResult {
    out := make([]worker.IntelExtractResult, len(records))
    for i, rec := range records {
        raw, err := json.Marshal(rec)
        if err != nil {
            continue
        }
        out[i] = worker.IntelExtractResult{
            RawExtraction:     raw,
            ConfidenceOverall: extractor.ConfidenceOverall(rec),
            ExternalKey:       deriveExternalKey(sourceURL, rec, i),
        }
    }
    return out
}
```

- [ ] **Step 8: Add `deriveExternalKey` and field helpers in `server/intel_adapters.go`.**

```go
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
    return fmt.Sprintf("%s|idx:%d", sourceURL, index)
}

func stringField(ext extractor.Extraction, key string) (string, bool) {
    f, ok := ext[key]
    if !ok || f.Value == nil {
        return "", false
    }
    s, ok := f.Value.(string)
    return s, ok
}
```

- [ ] **Step 9: Remove the old single-record `aggregateConfidence` in `server/intel_adapters.go`.**

Replace its use with `extractor.ConfidenceOverall`.

- [ ] **Step 10: Update `worker/intelligence.go` `HandleIntelExtract` to loop over the slice.**

```go
results, extErr := intelExtractor.Extract(...)
...
for i, result := range results {
    record, err := intelStores.CreateIntelRecord(ctx, ID, gen.CreateIntelRecordParams{
        SourceID:          rawDoc.SourceID,
        RawDocumentID:     rawDocID,
        EntityType:        entityType,
        ExternalKey:       result.ExternalKey,
        Extracted:         result.RawExtraction,
        Confidence:        []byte(`{}`),
        Evidence:          []byte(`{}`),
        ConfidenceOverall: result.ConfidenceOverall,
        GeoSpaceIds:       []pgtype.UUID{},
    })
    if err != nil {
        return fmt.Errorf("task:intel:extract: create record %d: %w", i, err)
    }
    payload := marshalPayload(intelRecordPayload{Version: intelPayloadVersion,
        RecordID: record.ID.String(),
        ID: p.ID,
    })
    if payload != nil {
        enqueueIntel(TypeIntelDedup, payload)
    }
}
```

- [ ] **Step 11: Run tests and commit.**

```bash
go test ./internal/intelligence/extractor/... ./internal/worker/... ./internal/server/... -run TestIntel -v
```

---

### Task 3: Update `ScholarshipPromoter` to create `contents` on publish

**Files:**
- Modify: `internal/intelligence/promoter/scholarship.go`, `internal/intelligence/promoter/scholarship_test.go`, `internal/intelligence/stores/stores.go`, `internal/server/intel_adapters.go`
- Test: `internal/intelligence/promoter/scholarship_test.go`, `internal/intelligence/integration_test.go`

**Interfaces:**
- Consumes: `intel_record.extracted` JSON.
- Produces: `scholarships` row with `guide_id` set to a published `contents` row, and `intel_records.promoted_record_id` + `status = 'published'`.

- [ ] **Step 1: Add content and promotion methods to `promoter.Store` and `TxStore`.**

```go
type Store interface {
    ...
    CreateContent(ctx context.Context, ID pgtype.UUID, arg gen.CreateContentParams) (gen.CreateContentRow, error)
    GetContentByID(ctx context.Context, ID, id pgtype.UUID) (gen.Content, error)
    UpdateContent(ctx context.Context, ID pgtype.UUID, arg gen.UpdateContentParams) (gen.Content, error)
    UpdateIntelRecordPromoted(ctx context.Context, ID pgtype.UUID, arg gen.UpdateIntelRecordPromotedParams) error
    ...
}

type TxStore interface {
    ...
    CreateContent(ctx context.Context, ID pgtype.UUID, arg gen.CreateContentParams) (gen.CreateContentRow, error)
    UpdateContent(ctx context.Context, ID pgtype.UUID, arg gen.UpdateContentParams) (gen.Content, error)
    UpdateIntelRecordPromoted(ctx context.Context, ID pgtype.UUID, arg gen.UpdateIntelRecordPromotedParams) error
}
```

- [ ] **Step 2: Implement wrapper methods in `internal/intelligence/stores/stores.go`.**

```go
func (s *Store) CreateContent(ctx context.Context, ID pgtype.UUID, arg gen.CreateContentParams) (gen.CreateContentRow, error) {
    arg.ID = ID
    return s.queries.CreateContent(ctx, arg)
}

func (s *Store) GetContentByID(ctx context.Context, ID, id pgtype.UUID) (gen.Content, error) {
    return s.queries.GetContentByID(ctx, gen.GetContentByIDParams{ID: ID, ID: id})
}

func (s *Store) UpdateContent(ctx context.Context, ID pgtype.UUID, arg gen.UpdateContentParams) (gen.Content, error) {
    arg.ID = ID
    return s.queries.UpdateContent(ctx, arg)
}

func (s *Store) UpdateIntelRecordPromoted(ctx context.Context, ID pgtype.UUID, arg gen.UpdateIntelRecordPromotedParams) error {
    arg.ID = ID
    return s.queries.UpdateIntelRecordPromoted(ctx, arg)
}
```

- [ ] **Step 3: Add `buildScholarshipBody` helper in `promoter/scholarship.go`.**

```go
func buildScholarshipBody(ext extractor.Extraction) string {
    var b strings.Builder
    if t := stringField(ext, "title"); t != "" {
        b.WriteString(t)
        b.WriteString("\n\n")
    }
    if d := stringPtrField(ext, "description_summary"); d != nil && *d != "" {
        b.WriteString(*d)
        b.WriteString("\n\n")
    }
    writeField(&b, ext, "university", "Host institution")
    writeField(&b, ext, "degree_level", "Level")
    writeField(&b, ext, "deadline", "Deadline")
    writeField(&b, ext, "funding", "Funding")
    writeField(&b, ext, "eligible_countries", "Eligible countries")
    if u := stringPtrField(ext, "application_url"); u != nil && *u != "" {
        b.WriteString("Apply at: ")
        b.WriteString(*u)
    }
    return strings.TrimSpace(b.String())
}

func writeField(b *strings.Builder, ext extractor.Extraction, name, label string) {
    v, ok := ext[name]
    if !ok || v.Value == nil {
        return
    }
    s := fmt.Sprint(v.Value)
    if s == "" || s == "[]" || s == "{}" {
        return
    }
    b.WriteString(label)
    b.WriteString(": ")
    b.WriteString(s)
    b.WriteString("\n")
}
```

- [ ] **Step 4: Update `createPath` to create `contents` and link it.**

```go
func (p *ScholarshipPromoter) createPath(ctx context.Context, record gen.IntelRecord, source gen.IntelSource, ext extractor.Extraction) (PromoteResult, error) {
    countryCode := firstCountry(ext, "country")
    var countrySpaceID pgtype.UUID
    if countryCode != "" {
        if id, err := p.store.GetCountrySpace(ctx, p.ID, countryCode); err == nil && id.Valid {
            countrySpaceID = id
        }
    }

    bodyText := buildScholarshipBody(ext)
    htmlBody := "<p>" + html.EscapeString(bodyText) + "</p>"
    hash := content.ComputeContentHash(bodyText)
    slug := content.Slugify(stringField(ext, "title"))
    if slug == "" || slug == "untitled" {
        slug = content.Slugify(source.Name)
    }
    canonicalURL := source.SourceUrl
    now := p.now()

    var guideID pgtype.UUID
    var result PromoteResult
    err := p.store.WithTx(ctx, func(tx TxStore) error {
        contentParams := gen.CreateContentParams{
            ID:        p.ID,
            AuthorID:        p.authorID,
            ContentType:     "guide",
            Title:           stringField(ext, "title"),
            Slug:            slug,
            Body:            bodyText,
            BodyHtml:        htmlBody,
            FieldValues:     []byte(`{}`),
            ContentHash:     &hash,
            GeoSpaceID:      countrySpaceID,
            CanonicalUrl:    &canonicalURL,
            Status:          "published",
            Visibility:      "public",
            SeoIndexable:    true,
            ReadTimeMinutes: content.ComputeReadTime(bodyText),
            GuideCategory:   ptr("scholarship"),
            PublishAt:       pgtype.Timestamptz{Time: now, Valid: true},
            PublishedAt:     pgtype.Timestamptz{Time: now, Valid: true},
            AiGenerated:     true,
            AiModel:         p.aiModel,
            AiGeneratedAt:   pgtype.Timestamptz{Time: now, Valid: true},
        }
        if reviewerID, ok := reviewerctx.ReviewerFromContext(ctx); ok {
            contentParams.HitlReviewedBy = reviewerID
            contentParams.HitlReviewedAt = pgtype.Timestamptz{Time: now, Valid: true}
        }

        contentRow, err := tx.CreateContent(ctx, p.ID, contentParams)
        if err != nil {
            return fmt.Errorf("promoter: create guide content: %w", err)
        }
        guideID = contentRow.ID

        scholarshipParams := buildCreateParams(record, source, guideID, ext)
        scholarshipParams.CountrySpaceID = countrySpaceID
        scholarship, err := tx.CreateScholarship(ctx, p.ID, scholarshipParams)
        if err != nil {
            return fmt.Errorf("promoter: create scholarship: %w", err)
        }

        if err := tx.UpdateIntelRecordPromoted(ctx, p.ID, gen.UpdateIntelRecordPromotedParams{
            ID:               record.ID,
            PromotedRecordID: scholarship.ID,
        }); err != nil {
            return fmt.Errorf("promoter: mark intel record promoted: %w", err)
        }

        result = PromoteResult{
            DomainID:  pgToUUID(scholarship.ID),
            ContentID: pgToUUID(contentRow.ID),
            Action:    ActionCreated,
        }
        return nil
    })
    return result, err
}
```

- [ ] **Step 5: Update `updatePath` to sync `contents` title/body when changed.**

If `existing.GuideID.Valid` and `title` or `description` changed and neither is in `editorial_overrides`:

```go
contentRow, err := p.store.GetContentByID(ctx, p.ID, existing.GuideID)
if err == nil {
    newBody := buildScholarshipBody(ext)
    newHash := content.ComputeContentHash(newBody)
    newSlug := content.Slugify(title)
    updateParams := gen.UpdateContentParams{
        ID:              contentRow.ID,
        ID:        p.ID,
        Title:           title,
        Slug:            newSlug,
        Body:            newBody,
        BodyHtml:        "<p>" + html.EscapeString(newBody) + "</p>",
        FieldValues:     contentRow.FieldValues,
        ContentHash:     &newHash,
        GeoSpaceID:      contentRow.GeoSpaceID,
        CanonicalUrl:    contentRow.CanonicalUrl,
        MetaTitle:       contentRow.MetaTitle,
        MetaDescription: contentRow.MetaDescription,
        OgImageUrl:      contentRow.OgImageUrl,
        SeoIndexable:    &contentRow.SeoIndexable,
        Status:          contentRow.Status,
        Visibility:      contentRow.Visibility,
        ReadTimeMinutes: content.ComputeReadTime(newBody),
        IsLocked:        contentRow.IsLocked,
        IsPinned:        contentRow.IsPinned,
        OrganizationID:  contentRow.OrganizationID,
        GuideCategory:   contentRow.GuideCategory,
        AiGeneratedAt:   contentRow.AiGeneratedAt,
        AiModel:         contentRow.AiModel,
    }
    if _, err := tx.UpdateContent(ctx, p.ID, updateParams); err != nil {
        return fmt.Errorf("promoter: update guide content: %w", err)
    }
}
```

- [ ] **Step 6: Update mock store and tests.**

Add `CreateContent`, `GetContentByID`, `UpdateContent`, `UpdateIntelRecordPromoted` to the test mock. Assert `PromoteResult.ContentID` is not `uuid.Nil` on create.

- [ ] **Step 7: Regenerate sqlc and run tests.**

```bash
make sqlc-gen
go test ./internal/intelligence/promoter/... -v
```

---

### Task 4: Add `intel_sources` seed migration

**Files:**
- Create: `migrations/000611_seed_scholarship_intel_sources.up.sql`, `migrations/000611_seed_scholarship_intel_sources.down.sql`

- [ ] **Step 1: Verify the next migration number.**

The next migration after `000610_*` is `000611`. Check `schemas/MIGRATION-ORDER.md` for ordering relative to `000610` and any pending migrations.

- [ ] **Step 2: Write up migration with the five sources.**

```sql
INSERT INTO intel_sources (
  tenant_id, name, domain, source_url, category, source_type, trust_level,
  crawl_method, crawl_frequency, crawl_frequency_mins, source_license,
  attribution_required, provider_config, enabled
) VALUES
  ('00000000-0000-0000-0000-000000000001', 'Open Scholarships', 'scholarships.grudged.io', 'https://scholarships.grudged.io/api/scholarships?availability=open&limit=500', 'scholarship', 'scholarship_database', 2, 'api', 'daily', 1440, 'CC-BY-4.0', true, '{"endpoint":"https://scholarships.grudged.io/api/scholarships?availability=open&limit=500","method":"GET"}', true),
  ('00000000-0000-0000-0000-000000000001', 'Grants.gov Search', 'api.grants.gov', 'https://api.grants.gov/v1/api/search2', 'scholarship', 'government', 1, 'api', 'daily', 1440, 'public-domain', true, '{"endpoint":"https://api.grants.gov/v1/api/search2","method":"POST","body":{"oppStatuses":"posted","rows":100,"keyword":"scholarship education"},"path":"data.opportunityData.opportunityDetails"}', true),
  ('00000000-0000-0000-0000-000000000001', 'DAAD Scholarship Database', 'www2.daad.de', 'https://www2.daad.de/deutschland/stipendium/datenbank/en/21148-scholarship-database/', 'scholarship', 'government', 1, 'static', 'weekly', 10080, 'public-domain', true, '{}', true),
  ('00000000-0000-0000-0000-000000000001', 'Erasmus Mundus Joint Masters', 'erasmus-plus.ec.europa.eu', 'https://erasmus-plus.ec.europa.eu/opportunities/individuals/students/erasmus-mundus-joint-masters', 'scholarship', 'international_org', 1, 'static', 'weekly', 10080, 'public-domain', true, '{}', true),
  ('00000000-0000-0000-0000-000000000001', 'Chevening Scholarships', 'www.chevening.org', 'https://www.chevening.org/scholarships/', 'scholarship', 'government', 1, 'static', 'weekly', 10080, 'public-domain', true, '{}', true);
```

- [ ] **Step 3: Verify the Grants.gov POST body before final commit.**

If `{"oppStatuses":"posted","rows":100,"keyword":"scholarship education"}` does not match the live `search2` schema, update `provider_config.body` in the migration and design doc.

- [ ] **Step 4: Write down migration to delete by `source_url`.**

```sql
DELETE FROM intel_sources WHERE source_url IN (
  'https://scholarships.grudged.io/api/scholarships?availability=open&limit=500',
  'https://api.grants.gov/v1/api/search2',
  'https://www2.daad.de/deutschland/stipendium/datenbank/en/21148-scholarship-database/',
  'https://erasmus-plus.ec.europa.eu/opportunities/individuals/students/erasmus-mundus-joint-masters',
  'https://www.chevening.org/scholarships/'
);
```

---

### Task 5: Update JEOS and conventions

**Files:**
- Modify: `.jeos/sections/73-scholarship-finder.md`
- Verify: `.jeos/conventions/system-settings-catalog.md`, `.jeos/conventions/asynq-tasks.md`, `.jeos/conventions/api-routes-catalog.md`, `.jeos/conventions/valkey-keys-catalog.md`

- [ ] **Step 1: Add a "Data Sources" subsection to `sections/73-scholarship-finder.md`.**

Reference the design spec and list the five sources and methods. No new routes or Asynq tasks.

- [ ] **Step 2: Verify existing `intel.*` settings cover the implementation.**

Existing keys are sufficient:
- `intel.extraction_confidence_auto` (0.85)
- `intel.extraction_confidence_review` (0.50)
- `intel.extraction_confidence_reject` (0.50)
- `intel.extraction_max_retries` (3)
- `intel.extraction_model` (gpt-4o-mini)
- `intel.schedule_batch_size` (50)
- `intel.ratelimit_default_per_min` (10)

Confirm the catalog key `intel.scholarships.max_page_size` in `system-settings-catalog.md` is actually used in code; if only `intel.scholarship.max_page_size` (singular) is used, fix the catalog typo.

---

### Task 6: Default `FindScholarships` to hide expired records

**Files:**
- Modify: `internal/scholarships/service.go`, `internal/scholarships/handlers.go`

- [ ] **Step 1: Update `ScholarshipFilters` and `HandleFindScholarships`.**

Add `IncludeExpired bool` to `ScholarshipFilters`. In `HandleFindScholarships`, parse `include_expired` query param. If false and `DeadlineFrom` is empty, set `DeadlineFrom` to `time.Now().UTC().Format("2006-01-02")`.

- [ ] **Step 2: Verify `Service.FindScholarships` applies the filter correctly.**

The existing query builder already checks `filters.DeadlineFrom` and adds `s.deadline >= $n::date`.

- [ ] **Step 3: Run tests.**

```bash
go test ./internal/scholarships/... -v
```

---

### Task 7: End-to-end verification

- [ ] **Step 1: Run the full backend test suite.**

```bash
go test ./... 2>&1 | tail -80
```

- [ ] **Step 2: Run `gosec` and `go vet`.**

```bash
gosec -no-fail -fmt json ./... 2>&1 | grep -E '"severity":\s*"HIGH"|"severity":\s*"CRITICAL"'
go vet ./...
```

- [ ] **Step 3: Run `make sqlc-gen` and `make build` (or `go build ./...`).**

```bash
make sqlc-gen
make build
```

- [ ] **Step 4: Run a manual end-to-end fetch for Open Scholarships.**

Start the worker (or use the admin control panel "Crawl now" for the Open Scholarships source). Then query the DB:

```sql
SELECT s.title, s.deadline, c.status
FROM scholarships s
JOIN contents c ON c.id = s.guide_id
WHERE s.source_url = 'https://scholarships.grudged.io/api/scholarships?availability=open&limit=500'
ORDER BY s.created_at DESC
LIMIT 10;
```

- [ ] **Step 5: Commit all changes with a plain message (no AI attribution).**

```bash
git add -A
git commit -m "feat(intel): wire Open Scholarships, Grants.gov, DAAD, Erasmus, Chevening into scholarship pipeline

- Extend APIFetcher with POST/JSON body and response path extraction.
- Support multi-record extraction for list/catalogue sources via ExtractResult.Records.
- Publish high-confidence scholarship records immediately via ScholarshipPromoter.
- Default FindScholarships to hide expired records.
- Seed five initial intel_sources for live scholarship data."
```

---

## Self-Review

1. **Spec coverage:** every design decision is mapped to a task.
2. **Placeholder scan:** no `TBD` or `TODO` remain; all file paths, SQL, and commands are concrete.
3. **Type consistency:** `APIProviderConfig` fields, `Store` interface additions, `IntelExtractResult` slice, `ExtractResult.Records`, and `FindScholarships` filter fields are consistent across tasks.
4. **Import cycles respected:** multi-record split and key derivation happen in `server/intel_adapters.go`, not `internal/worker`.
5. **Provenance and content hash:** every created `contents` row carries `canonical_url`, `content_hash`, `ai_generated`, `ai_model`, and `ai_generated_at`.
6. **HITL trace:** reviewer context stamps `hitl_reviewed_by`/`hitl_reviewed_at` on contents created from approved review records.
