# Scholarship Intelligence Sources — Design Spec

**Date:** 2026-09-03
**Status:** Draft — awaiting review
**Scope:** Add Open Scholarships and Grants.gov as `crawl_method = "api"` sources, plus DAAD, Erasmus Mundus, and Chevening as `crawl_method = "static"` sources, for the scholarship intelligence pipeline. High-confidence records are published immediately; lower-confidence records go to HITL review.
**Approach:** Hybrid — AI extraction + human editorial override/queue, but high-confidence official/trusted records are published immediately so users can find active scholarships in the finder.

## References

- ADR-073 (Scholarship Finder)
- ADR-112 (Dedicated `scholarships` projection table)
- `docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md`
- `.jeos/sections/73-scholarship-finder.md`
- `.jeos/AGENTS.md` Rules 3, 4, 7, 16

## Context

The `scholarships` projection table and the ingestion spine exist. The pipeline currently produces `scholarships` rows with `guide_id = NULL` and relies on the AI Content Studio to generate the `contents` guide later. Because `FindScholarships` joins `contents` and requires `c.status = 'published'`, those rows are invisible in the finder. This design resolves that by having the `ScholarshipPromoter` create the `contents` guide immediately for high-confidence, auto-published records while still routing lower-confidence records to the moderation queue for HITL review.

## Decisions

1. **Fast publish for high-confidence records.** For records that pass the publish gate (`confidence_overall >= intel.extraction_confidence_auto` and `trust_level <= 2`), the `ScholarshipPromoter` creates a `contents` row (guide/scholarship, status `published`) and sets `scholarships.guide_id`. For lower-confidence or untrusted records, the promoter leaves `guide_id = NULL` and the record is routed to `review_required` for the moderation queue / Studio.
2. **Source-specific `provider_config` for API sources.** Extend `APIProviderConfig` with `method` (default `GET`), `body` (optional JSON template), and `path` (JSONPath-like pointer to the record list within the response). This lets the generic `APIFetcher` handle Open Scholarships and Grants.gov without a dedicated provider package.
3. **Five initial sources: two APIs + three static HTML pages.**
   - **Open Scholarships** (`https://scholarships.grudged.io`): `GET /api/scholarships?availability=open&limit=...`. License `CC-BY-4.0`, attribution required.
   - **Grants.gov** (`https://api.grants.gov/v1/api/search2`): `POST` with a JSON body filtering for posted opportunities and education/scholarship keywords. Response path: `data.opportunityData.opportunityDetails`.
   - **DAAD Scholarship Database** (`https://www2.daad.de/deutschland/stipendium/datenbank/en/21148-scholarship-database/`): `static` crawl of Germany’s official scholarship database.
   - **Erasmus Mundus Joint Masters** (`https://erasmus-plus.ec.europa.eu/opportunities/individuals/students/erasmus-mundus-joint-masters`): `static` crawl of the EU Erasmus Mundus catalogue.
   - **Chevening Scholarships** (`https://www.chevening.org/scholarships/`): `static` crawl of the UK government’s global scholarship programme.
4. **Moderation is still in the loop.** Auto-published records still get `ai_generated = true` and `hitl_reviewed_by = NULL` (auto-published, not human-reviewed). Admin edits create `editorial_overrides` and the next re-crawl respects them. Low-confidence records require manual approval before a `contents` row is created.

## Source configuration

### API sources

| Field | Open Scholarships | Grants.gov |
|---|---|---|
| `name` | Open Scholarships | Grants.gov Search |
| `source_url` | `https://scholarships.grudged.io/api/scholarships?availability=open` | `https://api.grants.gov/v1/api/search2` |
| `category` | `scholarship` | `scholarship` |
| `source_type` | `scholarship_database` | `government` |
| `trust_level` | `2` | `1` |
| `crawl_method` | `api` | `api` |
| `crawl_frequency` | `daily` | `daily` |
| `source_license` | `CC-BY-4.0` | `public-domain` |
| `attribution_required` | `true` | `true` |
| `provider_config.method` | `GET` | `POST` |
| `provider_config.body` | none | `{"oppStatuses":"posted","rows":100,"keyword":"scholarship education"}` |
| `provider_config.path` | (empty: body is an array) | `data.opportunityData.opportunityDetails` |

### Static sources

| Field | DAAD | Erasmus Mundus | Chevening |
|---|---|---|---|
| `name` | DAAD Scholarship Database | Erasmus Mundus Joint Masters | Chevening Scholarships |
| `source_url` | `https://www2.daad.de/deutschland/stipendium/datenbank/en/21148-scholarship-database/` | `https://erasmus-plus.ec.europa.eu/opportunities/individuals/students/erasmus-mundus-joint-masters` | `https://www.chevening.org/scholarships/` |
| `category` | `scholarship` | `scholarship` | `scholarship` |
| `source_type` | `government` | `international_org` | `government` |
| `trust_level` | `1` | `1` | `1` |
| `crawl_method` | `static` | `static` | `static` |
| `crawl_frequency` | `weekly` | `weekly` | `weekly` |
| `source_license` | `public-domain` | `public-domain` | `public-domain` |
| `attribution_required` | `true` | `true` | `true` |
| `provider_config` | `{}` | `{}` | `{}` |

## Fetcher changes

`internal/intelligence/fetcher/api.go`:
- Add `Method`, `Body`, and `Path` to `APIProviderConfig`.
- If `Method` is `POST`, create the request with the JSON body.
- If `Path` is set, parse the JSON body and return only the array at that path as the `Body` (so the content hash and downstream extractor see a stable list). Keep `ContentType = application/json`.
- Continue supporting `Headers` for auth.

`internal/intelligence/fetcher/static.go`:
- No structural changes. The existing `StaticFetcher` downloads the page and normalizes HTML for hashing. The extractor prompt must be able to parse list/cards from an HTML page.

## Extraction mapping

The `scholarship` extraction schema already defines the canonical fields. The prompt should instruct the LLM to map from each source's shape. When a document contains multiple scholarship entries, the extractor should return a JSON array of records; the worker/promoter loop must then create one `intel_record` + one `scholarships` row per record.

- **Open Scholarships**: `name` → `title`, `sponsor` → `university` or ignore, `award` → `funding`, `deadline` → `deadline`, `eligibility` → `eligible_countries`/`degree_level`/`fields_of_study`, `geo` → `country`, `links` → `application_url`, `provenance.source_url` → evidence source, `status` → filter out `closed`.
- **Grants.gov**: `opportunityTitle` → `title`, `closeDate` → `deadline`, `opportunityNumber`/`agencyCode` → `university` (or ignore), `fundingInstrument`/`category` → `funding`/`fields_of_study`, `agency` → `country` (default to `USA` when not specified), `opportunityId` → part of `application_url`.
- **DAAD**: parse scholarship cards; `title` from programme name, `university` from host institution, `country` = `DEU`, `degree_level` from status (graduate/postdoc), `deadline` from application dates, `eligible_countries` from target group, `funding` from benefits, `application_url` from DAAD portal link.
- **Erasmus Mundus**: parse catalogue cards; `title` from programme name, `country` from consortium countries, `degree_level` = `masters`, `deadline` from application period, `eligible_countries` from worldwide, `funding` from scholarship contribution, `application_url` from programme website.
- **Chevening**: parse scholarship listing pages; `title` = Chevening Scholarships, `country` = `GBR`, `degree_level` = `masters`, `eligible_countries` from eligible countries page, `deadline` from application cycle, `funding` from award value, `application_url` from apply page.

## Multiple records per source document

The current extractor returns one `Extraction` map per call. For sources whose document contains multiple scholarships (Open Scholarships API, Grants.gov search results, static catalogue list pages), the extractor should be instructed to return a top-level array. The worker's `HandleIntelExtract` must then create one `intel_record` per array element and enqueue `intel:dedup` for each. If the existing extractor cannot return a list, an adapter pre-processor will split the document into per-record text chunks before extraction.

For v1, the simplest path is: the `APIFetcher` and `StaticFetcher` return the full document; the LLM prompt asks for a JSON array of records; a lightweight `extractor.SplitRecords` helper splits the returned `Extraction` into a list of `Extraction` maps, and the worker enqueues the rest of the pipeline once per record.

## Promoter changes

`internal/intelligence/promoter/scholarship.go`:
- Extend `Store` / `TxStore` with `CreateContent` and `CreateContentParams` (or accept a `ContentCreator` interface) so the promoter can create the `contents` row.
- On the **create path** when `verification_status = 'verified'`, `override_source = 'ingestion'`, and the record came from the auto-publish gate:
  1. Build a body from the extraction (title + summary + key fields as TipTap JSON or plain text; `body_html` escaped).
  2. Compute `content_hash` per Rule 12.
  3. Insert `contents` row with `content_type='guide'`, `guide_category='scholarship'`, `status='published'`, `ai_generated=true`, `ai_model` from `provider/extractor`, `canonical_url = source_url`.
  4. Pass the returned `content_id` into `CreateScholarshipParams.GuideID`.
- On the **review-required path**: leave `guide_id` NULL and do not create `contents`. The moderation queue/Studio can later create it.
- On the **update path**: if the existing row has a `guide_id`, re-derive the body if title/description changed and neither is in `editorial_overrides`; update the `contents` row and recompute `content_hash`.

## Publish gate

The existing publish gate in `internal/intelligence/services/gate.go` (or `worker/intelligence.go` `HandleIntelPublishGate`) already routes by confidence and trust level. No new gate logic is required, but ensure the threshold system settings are seeded:
- `intel.extraction_confidence_auto` = `0.85`
- `intel.extraction_confidence_review` = `0.50`

## Data flow

```text
intel:schedule_crawls → intel:fetch_source (api or static adapter) → intel:detect_changes → intel:extract
  → record splitter (if multi-record document)
  → intel:dedup (per record) → intel:publish_gate
    → AUTO_PUBLISH → intel:promote (creates scholarships + contents) → intel:post_publish
    → REVIEW_REQUIRED → moderation queue / Studio (contents created later)
```

## Catalogs and migrations

1. **Migrations**:
   - Seed `intel_sources` rows for the five sources above (or provide as insertable seed data; no schema change needed).
   - No table changes required unless `APIProviderConfig` additions require persistence changes (JSONB, no schema change).
2. **Catalogs to update** (per `jc-backend-develop` contract):
   - `.jeos/conventions/asynq-tasks.md` (already has `intel:*` tasks; confirm `intel:promote` description is accurate).
   - `.jeos/conventions/system-settings-catalog.md` (confirm `intel.*` settings exist).
   - `.jeos/conventions/api-routes-catalog.md` (no new routes; sources managed via existing `/api/v1/intel/sources`).

## Testing strategy

- Unit tests for `APIFetcher` with POST body and `Path` extraction.
- `ScholarshipPromoter` tests for both auto-publish (creates content) and review-required (does not create content) paths.
- Static extraction test using the DAAD or Erasmus HTML test fixture.
- End-to-end: enqueue `intel:fetch_source` for a test source, verify pipeline creates `scholarships` + `contents` rows.

## Risks and mitigations

- **Grants.gov is a grants API, not scholarships.** Mitigation: filter by `fundingInstrument=Grant` + education keywords, accept that many results may not be student aid. Monitor precision and disable if noise is high.
- **Open Scholarships coverage is US/Nevada-first.** Mitigation: treat as a proof-of-concept source; the three static sources add EU/UK/Germany coverage.
- **Static HTML sources are fragile.** Mitigation: use low-frequency (`weekly`) crawls, low trust threshold, and expect `consecutive_failures` auto-disable. Add CSS selector hints to `provider_config` if site structure is stable.
- **Multiple records per document are not yet wired.** Mitigation: implement `extractor.SplitRecords` / worker per-record enqueue as part of this work; if too large, limit v1 to one record per source document and expand in a follow-up.
- **Auto-publish without HITL can surface incorrect data.** Mitigation: only auto-publish Tier 1-2 sources with confidence >= 0.85; admin can edit and set `editorial_overrides`; re-crawl respects overrides.
