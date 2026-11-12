# Compare Hub Upgrade Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restructure `/compare` + `/comparisons` into a discovery-first hub (category cards → family browse pages → existing picker), per the approved mockup.

**Architecture:** Additive backend changes to `GET /jce/comparisons` (CSV type filter, `preview_items` hydration, opt-in type facets) plus a new `/compare/[category]` family browse route (SvelteKit param matcher). All picker/workspace logic is preserved — the category page is a browse layer that links into the existing `?type=` picker.

**Tech Stack:** Go + sqlc + pgx/v5 (`internal/comparison`), SvelteKit 2 + Svelte 5 runes + Tailwind v4 (`apps/main`), vitest, `go test`.

**Spec:** `docs/superpowers/specs/2026-11-11-compare-hub-upgrade-design.md`

## Global Constraints

- **Bun only, at repo root.** `bun install` in repo root only; never inside `apps/*`. Root `bun.lock` is the only lockfile.
- **Preserve picker logic:** `NarrowPanel`, `CompareTray`, `FieldPoolPicker`, `StepHeader` callbacks, scope chains, `preselect`/`compare_token`, guest-selection login flow, promote/delete, AI quick compare, `mine` tabs, `feed.xml`, SEO/JSON-LD — unchanged.
- **All copy via `t()`** with `jce.compare.*`/`jce.comparisons.*` keys; no hardcoded strings (Rule 16). Keys added to `apps/main/src/lib/i18n/en.json`.
- **Tokens only:** `--color-*` CSS vars; family tints live in `COMPARE_FAMILIES`. RTL icons keep `rtl:-scale-x-100`.
- **Svelte 5 runes** (`$state`, `$derived`, `$props`, `$effect`), Bits UI primitives where applicable, lucide-svelte icons (legacy typings — cast via `as unknown as Component` at the seam, existing convention).
- **After sqlc changes:** run `sqlc generate` (config: `sqlc.yaml`, queries in `internal/db/queries/`).
- **Commit to `main`** after every task, matching repo message style (`feat:`, `fix:`, `docs:` etc.).

---

### Task 1: Backend — `filter_type` accepts CSV → `TypeIDs`

**Files:**
- Modify: `internal/db/queries/comparison.sql` (type predicate in `ListComparisonsWithFilters` ~line 295 and `CountComparisonsWithFilters` ~line 333)
- Modify: `internal/comparison/jce_service.go` (`JCEComparisonFilter` ~line 94, `ListJCEComparisons` ~line 687)
- Modify: `internal/comparison/jce_handlers.go` (`ListJCEComparisons` ~line 68-92)
- Modify: `internal/comparison/service.go` (`ListComparisons`/`ListComparisonsAdmin` filter mapping ~line 2122)
- Test: `internal/comparison/jce_test.go` (append)

**Interfaces:**
- Consumes: nothing new.
- Produces: `JCEComparisonFilter.TypeIDs []pgtype.UUID` (nil = unfiltered), `parseTypeSlugCSV(raw string) []string`, query params `type_ids uuid[]` (NULL = unfiltered). Used by Tasks 2-3 and the family page.

- [ ] **Step 1: Write the failing test**

Append to `internal/comparison/jce_test.go`:

```go
func TestParseTypeSlugCSV(t *testing.T) {
	cases := []struct {
		raw  string
		want []string
	}{
		{"countries", []string{"countries"}},
		{"countries,states,cities", []string{"countries", "states", "cities"}},
		{" countries , states ", []string{"countries", "states"}},
		{"", nil},
		{"a,,b,", []string{"a", "b"}},
	}
	for _, tc := range cases {
		got := parseTypeSlugCSV(tc.raw)
		if len(got) != len(tc.want) {
			t.Fatalf("parseTypeSlugCSV(%q) = %v, want %v", tc.raw, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("parseTypeSlugCSV(%q)[%d] = %q, want %q", tc.raw, i, got[i], tc.want[i])
			}
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/comparison/ -run TestParseTypeSlugCSV -v`
Expected: FAIL — `undefined: parseTypeSlugCSV`

- [ ] **Step 3: Implement**

In `internal/comparison/jce_handlers.go`, add near the top of `ListJCEComparisons` (with the other small helpers):

```go
// parseTypeSlugCSV splits a comma-separated filter_type/facet_scope value into
// trimmed, non-empty slugs. Single-slug callers pass one element.
func parseTypeSlugCSV(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	for _, s := range strings.Split(raw, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}
```

Replace the single-slug block (~lines 68-92) with CSV resolution:

```go
var typeIDs []pgtype.UUID
if csv := parseTypeSlugCSV(firstNonEmpty(q.Get("filter_type"), q.Get("type"))); len(csv) > 0 {
	for _, slug := range csv {
		td, err := h.Service.GetComparisonTypeBySlug(r.Context(), slug)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				errpkg.WriteError(w, http.StatusNotFound, "type_not_found", "Comparison type not found")
				return
			}
			handleServiceError(w, err)
			return
		}
		typeIDs = append(typeIDs, td.ID)
	}
} else if v := q.Get("type_id"); v != "" {
	var tid pgtype.UUID
	if err := tid.Scan(v); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_type_id", "Invalid type_id")
		return
	}
	typeIDs = []pgtype.UUID{tid}
}
```

Add the `firstNonEmpty` helper if the file doesn't already have one (check before adding — several handler files define it):

```go
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
```

Set `filter := JCEComparisonFilter{ ..., TypeIDs: typeIDs, ... }` (replacing `TypeID: typeID`).

In `internal/comparison/jce_service.go`:

```go
type JCEComparisonFilter struct {
	SpaceID    pgtype.UUID
	TypeIDs    []pgtype.UUID // nil = no type filter; one element = legacy single-type filter
	// ...rest unchanged
}
```

Pass into the store calls:

```go
items, err := s.store.listComparisonsWithFilters(ctx, gen.ListComparisonsWithFiltersParams{
	// ...
	TypeIDs: f.TypeIDs,
	// ...
})
total, err := s.store.countComparisonsWithFilters(ctx, gen.CountComparisonsWithFiltersParams{
	// ...
	TypeIDs: f.TypeIDs,
	// ...
})
```

In `internal/comparison/service.go` find `ListComparisons`/`ListComparisonsAdmin` (~line 2122) where `ListComparisonsFilter` maps to `TypeID` — update the same predicate param to `TypeIDs`.

In `internal/db/queries/comparison.sql`, replace the type predicate in **both** `ListComparisonsWithFilters` and `CountComparisonsWithFilters` — the existing block:

```sql
AND (
 sqlc.narg(type_id)::uuid IS NULL
 OR EXISTS (
 SELECT 1 FROM comparison_items ci
 WHERE ci.content_id = c.id
 AND ci.type_id = sqlc.narg(type_id)
 AND ci.deleted_at IS NULL
 )
)
```

becomes:

```sql
AND (
 sqlc.narg(type_ids)::uuid[] IS NULL
 OR EXISTS (
 SELECT 1 FROM comparison_items ci
 WHERE ci.content_id = c.id
 AND ci.type_id = ANY(sqlc.narg(type_ids)::uuid[])
 AND ci.deleted_at IS NULL
 )
)
```

Run `sqlc generate` and fix remaining compile errors (rename `TypeID` field accesses to `TypeIDs` where the param is now a slice — `params.TypeID` → `params.TypeIds`; pass `nil` for no filter).

- [ ] **Step 4: Run tests**

Run: `go test ./internal/comparison/ -run TestParseTypeSlugCSV -v && go build ./internal/comparison/`
Expected: PASS + clean build.

- [ ] **Step 5: Commit**

```bash
git add internal/db/queries/comparison.sql internal/db/gen/comparison.sql.go internal/comparison/jce_service.go internal/comparison/jce_handlers.go internal/comparison/service.go internal/comparison/jce_test.go
git commit -m "feat(jce): filter_type accepts CSV — multi-type comparison lists"
```

---

### Task 2: Backend — `facet=types` + `facet_scope` counts

**Files:**
- Modify: `internal/db/queries/comparison.sql` (append `CountComparisonsByType`)
- Modify: `internal/comparison/jce_service.go` (`JCEComparisonListResult`, facet computation in `ListJCEComparisons`)
- Modify: `internal/comparison/jce_handlers.go` (parse `facet`/`facet_scope`, emit `meta.facets`)
- Test: `internal/comparison/jce_test.go` (append)

**Interfaces:**
- Consumes: `parseTypeSlugCSV`, `JCEComparisonFilter.TypeIDs` (Task 1).
- Produces: `meta.facets.types: [{type_id, slug, count}]`; `JCEComparisonFilter.FacetTypes bool` + `FacetScope []pgtype.UUID`. Consumed by Tasks 9-11.

- [ ] **Step 1: Write the failing test**

```go
func TestJCETypeFacetShape(t *testing.T) {
	f := JCETypeFacet{TypeID: "uuid-1", Slug: "countries", Count: 16}
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["slug"] != "countries" || m["count"].(float64) != 16 {
		t.Fatalf("unexpected facet JSON: %s", b)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/comparison/ -run TestJCETypeFacetShape -v`
Expected: FAIL — `undefined: JCETypeFacet`

- [ ] **Step 3: Implement**

Append to `internal/db/queries/comparison.sql` (uses the same WHERE predicates as `CountComparisonsWithFilters` **minus** the type predicate, plus a `scope_ids` post-filter on the primary type):

```sql
-- name: CountComparisonsByType :many
-- Per-primary-type comparison counts for facet rails. Primary type = the
-- first live item's type_id (same rule as the list's type_id projection).
-- scope_ids (NULL) restricts the counted type set for family pages.
SELECT first_type.type_id, count(*)::int AS count
FROM (
 SELECT c.id,
   (SELECT ci.type_id FROM comparison_items ci
    WHERE ci.content_id = c.id AND ci.deleted_at IS NULL
    ORDER BY ci.position LIMIT 1) AS type_id
 FROM contents c
 JOIN comparison_content_metadata m ON m.content_id = c.id
 WHERE c.content_type = 'comparison'
  AND (
   CASE sqlc.narg(deleted)::text
   WHEN 'only' THEN c.deleted_at IS NOT NULL
   WHEN 'all' THEN TRUE
   ELSE (c.deleted_at IS NULL AND c.auto_hidden = false)
   END
  )
  AND (sqlc.narg(space_id)::uuid IS NULL OR c.space_id = sqlc.narg(space_id))
  AND (sqlc.narg(depth)::text IS NULL OR m.depth = sqlc.narg(depth))
  AND (sqlc.narg(status)::text IS NULL OR c.status = sqlc.narg(status))
  AND (sqlc.narg(visibility)::text IS NULL OR c.visibility = sqlc.narg(visibility))
  AND (sqlc.narg(mine)::boolean IS NOT TRUE OR c.author_id = @user_id)
  AND (
   sqlc.narg(q)::text IS NULL OR sqlc.narg(q) = ''
   OR c.title ILIKE '%' || sqlc.narg(q) || '%'
   OR c.slug ILIKE '%' || sqlc.narg(q) || '%'
  )
) first_type
WHERE first_type.type_id IS NOT NULL
 AND (sqlc.narg(scope_ids)::uuid[] IS NULL OR first_type.type_id = ANY(sqlc.narg(scope_ids)::uuid[]))
GROUP BY first_type.type_id
ORDER BY count DESC;
```

In `internal/comparison/jce_service.go` add:

```go
// JCETypeFacet is one row of meta.facets.types on GET /jce/comparisons.
type JCETypeFacet struct {
	TypeID string `json:"type_id"`
	Slug   string `json:"slug,omitempty"`
	Count  int32  `json:"count"`
}
```

Add to `JCEComparisonFilter`: `FacetTypes bool` and `FacetScope []pgtype.UUID`.
Add to `JCEComparisonListResult`: `TypeFacets []JCETypeFacet`.

At the end of `ListJCEComparisons` (before the return), compute facets only when requested:

```go
var typeFacets []JCETypeFacet
if f.FacetTypes {
	// countComparisonsByType is a new thin wrapper added in jce_store.go —
	// calls s.read.CountComparisonsByType and wraps errors with fmt.Errorf
	// (same convention as listComparisonsWithFilters at line 52).
	rows, err := s.store.countComparisonsByType(ctx, gen.CountComparisonsByTypeParams{
		Deleted:    deleted,
		SpaceID:    f.SpaceID,
		Depth:      depth,
		Status:     status,
		Visibility: visibility,
		Mine:       &mine,
		UserID:     userID,
		Q:          query,
		ScopeIds:   f.FacetScope,
	})
	if err == nil {
		// Enrich type_id → slug using the type catalog (small, already cached
		// by callers of /jce/comparison-types).
		types, terr := s.ListComparisonTypes(ctx, 200, 0)
		slugByID := map[string]string{}
		if terr == nil {
			for _, td := range types {
				slugByID[td.ID.String()] = td.Slug
			}
		}
		typeFacets = make([]JCETypeFacet, 0, len(rows))
		for _, r := range rows {
			typeFacets = append(typeFacets, JCETypeFacet{
				TypeID: r.TypeID.String(),
				Slug:   slugByID[r.TypeID.String()],
				Count:  r.Count,
			})
		}
	}
}
```

Check `r.TypeID` type after regen — the subquery column is `type_id uuid` nullable → `pgtype.UUID`. Guard `r.TypeID.Valid`. Set `TypeFacets: typeFacets` on the result.

In `internal/comparison/jce_handlers.go`, after building `filter`:

```go
filter.FacetTypes = q.Get("facet") == "types"
if filter.FacetTypes {
	if csv := parseTypeSlugCSV(q.Get("facet_scope")); len(csv) > 0 {
		for _, slug := range csv {
			td, err := h.Service.GetComparisonTypeBySlug(r.Context(), slug)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					errpkg.WriteError(w, http.StatusNotFound, "type_not_found", "Comparison type not found")
					return
				}
				handleServiceError(w, err)
				return
			}
			filter.FacetScope = append(filter.FacetScope, td.ID)
		}
	}
}
```

Emit in the response meta:

```go
meta := map[string]any{
	"total":             res.Total,
	"page":              res.Page,
	"limit":             res.Size,
	"sort_options":      res.SortOptions,
	"page_size_options": res.PageSizeOptions,
}
if filter.FacetTypes {
	meta["facets"] = map[string]any{"types": res.TypeFacets}
}
errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": res.Items, "meta": meta})
```

Run `sqlc generate`.

- [ ] **Step 4: Run tests + build**

Run: `go test ./internal/comparison/ -v && go build ./...`
Expected: PASS + clean build (the facet count runs against a real DB in integration; the unit test covers the JSON shape).

- [ ] **Step 5: Commit**

```bash
git add internal/db/queries/comparison.sql internal/db/gen/comparison.sql.go internal/comparison/jce_service.go internal/comparison/jce_handlers.go internal/comparison/jce_test.go
git commit -m "feat(jce): facet=types + facet_scope counts on comparison list"
```

---

### Task 3: Backend — `preview_items` on list summaries

**Files:**
- Modify: `internal/db/queries/comparison.sql` (append `ListComparisonItemPreviews`, `ListSpaceGeoByIDs`)
- Modify: `internal/db/queries/spaces.sql` — no, put `ListSpaceGeoByIDs` in `comparison.sql` to keep the feature's queries together.
- Modify: `internal/comparison/jce_service.go` (DTO types, `loadPreviewItems`, attach in `ListJCEComparisons`)
- Test: `internal/comparison/jce_test.go` (append)

**Interfaces:**
- Consumes: `resolver.CoerceMap`, `s.resolver.Resolve` (pattern at `internal/comparison/service.go:1755-1786`), `media.MediaURL` (usage at `jce_service.go:1476`), `GetMediaByIDs` (exists in `internal/db/gen/media.sql.go:377`), `golang.org/x/sync/errgroup` (in go.mod).
- Produces: `JCEComparisonListItem` (embeds `gen.ListComparisonsWithFiltersRow` + `PreviewItems`), `JCEPreviewItem{Title, ImageURL, GeoCode, CountryCode, ItemType}`. `JCEComparisonListResult.Items` becomes `[]JCEComparisonListItem` — fix the two list handlers that serialize `res.Items` (they write it opaquely, so only the type changes).

- [ ] **Step 1: Write the failing test**

```go
func TestMergePreviewItemFields(t *testing.T) {
	p := JCEPreviewItem{Title: "Canada"}
	mergePreviewItem(&p, previewItemParts{
		imageURL:    "https://cdn/x.png",
		geoCode:     "ca",
		countryCode: "ca",
		itemType:    "country",
	})
	if p.ImageURL != "https://cdn/x.png" || p.CountryCode != "ca" || p.ItemType != "country" {
		t.Fatalf("merge failed: %+v", p)
	}
	// Explicit fields win — a second merge must not overwrite.
	mergePreviewItem(&p, previewItemParts{imageURL: "https://cdn/y.png", countryCode: "us"})
	if p.ImageURL != "https://cdn/x.png" || p.CountryCode != "ca" {
		t.Fatalf("merge overwrote explicit fields: %+v", p)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/comparison/ -run TestMergePreviewItemFields -v`
Expected: FAIL — `undefined: JCEPreviewItem / mergePreviewItem / previewItemParts`

- [ ] **Step 3: Implement**

Append to `internal/db/queries/comparison.sql`:

```sql
-- name: ListComparisonItemPreviews :many
-- First ≤3 live items per content row for directory card previews.
SELECT ranked.content_id, ranked.id, ranked.title, ranked.image_id,
 ranked.geo_space_id, ranked.item_ref, ranked.position
FROM (
 SELECT ci.content_id, ci.id, ci.title, ci.image_id, ci.geo_space_id,
   ci.item_ref, ci.position,
   ROW_NUMBER() OVER (PARTITION BY ci.content_id ORDER BY ci.position, ci.created_at) AS rn
 FROM comparison_items ci
 WHERE ci.content_id = ANY(@content_ids::uuid[])
  AND ci.deleted_at IS NULL
) ranked
WHERE ranked.rn <= 3
ORDER BY ranked.content_id, ranked.position;

-- name: ListSpaceGeoByIDs :many
-- id → geo code + parent (country) geo code for flag medallions.
SELECT s.id, s.geo_code, s.level, p.geo_code AS parent_geo_code
FROM spaces s
LEFT JOIN spaces p ON p.id = s.parent_id
WHERE s.id = ANY(@ids::uuid[]);
```

Run `sqlc generate`.

In `internal/comparison/jce_service.go` add the DTO types near `JCEComparisonListResult`:

```go
// JCEPreviewItem is a ≤3-item card preview embedded on list summaries.
type JCEPreviewItem struct {
	Title       string `json:"title"`
	ImageURL    string `json:"image_url,omitempty"`
	GeoCode     string `json:"geo_code,omitempty"`
	CountryCode string `json:"country_code,omitempty"`
	ItemType    string `json:"item_type,omitempty"`
}

// JCEComparisonListItem wraps the list row with card previews. The embedded
// sqlc row's JSON fields stay flat — the payload is additive only.
type JCEComparisonListItem struct {
	gen.ListComparisonsWithFiltersRow
	PreviewItems []JCEPreviewItem `json:"preview_items,omitempty"`
}

// previewItemParts carries one resolved facet of a preview item; empty fields
// never overwrite values already set (explicit image/geo beats resolver data).
type previewItemParts struct {
	imageURL    string
	geoCode     string
	countryCode string
	itemType    string
}

func mergePreviewItem(p *JCEPreviewItem, parts previewItemParts) {
	if p.ImageURL == "" {
		p.ImageURL = parts.imageURL
	}
	if p.GeoCode == "" {
		p.GeoCode = parts.geoCode
	}
	if p.CountryCode == "" {
		p.CountryCode = parts.countryCode
	}
	if p.ItemType == "" {
		p.ItemType = parts.itemType
	}
}
```

Add `loadPreviewItems` — one batched items query + one media query + one geo query + bounded resolver calls. `previews[i]` stays positionally aligned with `rows[i]` throughout:

```go
// loadPreviewItems hydrates ≤3 items per content id for directory cards.
// Best-effort: any failed lookup drops that field, never the list row.
func (s *Service) loadPreviewItems(ctx context.Context, contentIDs []pgtype.UUID) map[pgtype.UUID][]JCEPreviewItem {
	out := map[pgtype.UUID][]JCEPreviewItem{}
	if len(contentIDs) == 0 {
		return out
	}

	rows, err := s.store.listComparisonItemPreviews(ctx, contentIDs)
	if err != nil {
		return out
	}

	previews := make([]*JCEPreviewItem, len(rows))
	for i, r := range rows {
		previews[i] = &JCEPreviewItem{Title: r.Title}
	}

	// Media batch.
	{
		var imageIDs []pgtype.UUID
		for _, r := range rows {
			if r.ImageID.Valid {
				imageIDs = append(imageIDs, r.ImageID)
			}
		}
		if len(imageIDs) > 0 {
			if mediaRows, merr := s.store.getMediaByIDs(ctx, imageIDs); merr == nil {
				urlByID := map[string]string{}
				for _, m := range mediaRows {
					// Medium.StorageBucket/StoragePath are plain strings.
					urlByID[m.ID.String()] = media.MediaURL(m.StorageBucket, m.StoragePath, media.ImageProxyOpts{})
				}
				for i, r := range rows {
					if r.ImageID.Valid {
						mergePreviewItem(previews[i], previewItemParts{imageURL: urlByID[r.ImageID.String()]})
					}
				}
			}
		}
	}

	// Geo batch: geo_space_id → geo_code + parent (country) code.
	{
		var spaceIDs []pgtype.UUID
		for _, r := range rows {
			if r.GeoSpaceID.Valid {
				spaceIDs = append(spaceIDs, r.GeoSpaceID)
			}
		}
		if len(spaceIDs) > 0 {
			if geoRows, gerr := s.store.listSpaceGeoByIDs(ctx, spaceIDs); gerr == nil {
				type geoParts struct{ code, country string }
				geoByID := map[string]geoParts{}
				for _, g := range geoRows {
					code, country := "", ""
					if g.GeoCode != nil {
						code = *g.GeoCode
					}
					if g.Level != nil && *g.Level == "country" {
						country = code
					} else if g.ParentGeoCode != nil {
						country = *g.ParentGeoCode
					}
					geoByID[g.ID.String()] = geoParts{code: code, country: country}
				}
				for i, r := range rows {
					if r.GeoSpaceID.Valid {
						gp := geoByID[r.GeoSpaceID.String()]
						mergePreviewItem(previews[i], previewItemParts{geoCode: gp.code, countryCode: gp.country})
					}
				}
			}
		}
	}

	// item_ref resolution fills remaining gaps (university logos, wiki geo).
	// Deduped per ref, bounded concurrency — resolution is service-level, not
	// a SQL loop; failures leave fields empty.
	if s.resolver != nil {
		type refJob struct {
			idx int
			key string
			ref map[string]any
		}
		seen := map[string]*resolver.ResolvedItem{}
		var jobs []refJob
		for i, r := range rows {
			if len(r.ItemRef) == 0 {
				continue
			}
			if previews[i].ImageURL != "" && previews[i].CountryCode != "" {
				continue // explicit image + geo already cover the card
			}
			ref, err := resolver.CoerceMap(r.ItemRef)
			if err != nil {
				continue
			}
			key := itemRefDedupKey(r.ItemRef)
			jobs = append(jobs, refJob{idx: i, key: key, ref: ref})
		}
		g, gctx := errgroup.WithContext(ctx)
		g.SetLimit(8)
		var mu sync.Mutex
		for _, j := range jobs {
			j := j
			g.Go(func() error {
				mu.Lock()
				cached, ok := seen[j.key]
				mu.Unlock()
				if !ok {
					resolved, rerr := s.resolver.Resolve(gctx, j.ref)
					mu.Lock()
					if rerr == nil {
						seen[j.key] = &resolved
						cached = &resolved
					}
					mu.Unlock()
					if rerr != nil {
						return nil // best-effort
					}
				}
				parts := previewItemParts{imageURL: cached.ImageURL}
				if t, _ := j.ref["type"].(string); t != "" {
					parts.itemType = t
				}
				if c, _ := cached.Metadata["geo_code"].(string); c != "" {
					parts.geoCode = c
				}
				if c, _ := cached.Metadata["country"].(string); c != "" {
					parts.countryCode = c
				} else if c, _ := cached.Metadata["country_code"].(string); c != "" {
					parts.countryCode = c
				}
				mu.Lock()
				mergePreviewItem(previews[j.idx], parts)
				mu.Unlock()
				return nil
			})
		}
		_ = g.Wait()
	}

	for i, r := range rows {
		out[r.ContentID] = append(out[r.ContentID], *previews[i])
	}
	return out
}
```

Check the generated row field names after `sqlc generate` — `itemRefDedupKey` exists in `internal/comparison` (used at service.go:1760); `media.ImageProxyOpts` usage at jce_service.go:1476. Add thin store wrappers in `internal/comparison/jce_store.go` next to `listComparisonsWithFilters` (line 52): `listComparisonItemPreviews`, `listSpaceGeoByIDs`, `getMediaByIDs`, `countComparisonsByType` — each calls `s.read.<GenMethod>(ctx, arg)` and wraps errors with `fmt.Errorf("<doing>: %w", err)` (the file's existing convention).

Then in `ListJCEComparisons`, after `items` is fetched:

```go
contentIDs := make([]pgtype.UUID, 0, len(items))
for _, it := range items {
	contentIDs = append(contentIDs, it.ID)
}
previewByContent := s.loadPreviewItems(ctx, contentIDs)
listItems := make([]JCEComparisonListItem, len(items))
for i, it := range items {
	listItems[i] = JCEComparisonListItem{
		ListComparisonsWithFiltersRow: it,
		PreviewItems:                 previewByContent[it.ID],
	}
}
```

Return `Items: listItems` and change `JCEComparisonListResult.Items` to `[]JCEComparisonListItem`. Fix the admin `ListComparisonsAdmin` caller if it shares the type (it uses `ListComparisonsWithFiltersRow` directly — keep that one on the row type; only `ListJCEComparisons`' result changes).

Imports to add in `jce_service.go`: `sync`, `golang.org/x/sync/errgroup`, `internal/comparison/resolver` (already imported for `resolver.ResolvedItem`? check), `media` package (same import used at line 1476).

- [ ] **Step 4: Run tests + build**

Run: `go test ./internal/comparison/ -v && go build ./...`
Expected: PASS + clean build.

- [ ] **Step 5: Commit**

```bash
git add internal/db/queries/comparison.sql internal/db/gen/comparison.sql.go internal/comparison/jce_service.go internal/comparison/jce_test.go
git commit -m "feat(jce): preview_items on comparison list summaries"
```

---

### Task 4: Frontend foundation — types, families, i18n, param matcher

**Files:**
- Modify: `apps/main/src/lib/types/jce.ts` (~line 110 `JCEComparisonSummary`)
- Modify: `apps/main/src/lib/types/index.ts` (~line 59 `ListMeta`)
- Create: `apps/main/src/lib/components/compare/compareFamilies.ts`
- Create: `apps/main/src/lib/components/compare/compareFamilies.test.ts`
- Create: `apps/main/src/params/compareCategory.ts`
- Modify: `apps/main/src/lib/i18n/en.json` (insert near existing `jce.compare.*` keys ~line 2588)

**Interfaces:**
- Produces: `JCEPreviewItem`, `JCEComparisonSummary.preview_items`, `ListMeta.facets`, `COMPARE_FAMILIES`, `familyForKey/familyForSlug/familySlug/familyKeyFromSlug/groupTypesByFamily`, the `compareCategory` param matcher. Consumed by Tasks 6-11.

- [ ] **Step 1: Write the failing test**

`apps/main/src/lib/components/compare/compareFamilies.test.ts`:

```ts
import { describe, expect, it } from 'vitest';
import {
	COMPARE_FAMILIES,
	familyForSlug,
	familyKeyFromSlug,
	familySlug,
	groupTypesByFamily
} from './compareFamilies.js';
import type { ComparisonType } from '#lib/types/jce.js';

const t = (id: string, slug: string, family?: string): ComparisonType =>
	({ id, slug, name: slug, config: family ? { family } : null }) as unknown as ComparisonType;

describe('compareFamilies', () => {
	it('maps family keys to hyphenated URL slugs', () => {
		expect(familySlug('visa_routes')).toBe('visa-routes');
		expect(familyKeyFromSlug('visa-routes')).toBe('visa_routes');
		expect(familySlug('location')).toBe('location');
	});

	it('matcher set covers every family slug', () => {
		for (const f of COMPARE_FAMILIES) {
			expect(familyForSlug(f.slug)?.key).toBe(f.key);
		}
		expect(familyForSlug('not-a-family')).toBeUndefined();
	});

	it('groups types under their family, unknown families last into ungrouped', () => {
		const { groups, ungrouped } = groupTypesByFamily([
			t('1', 'countries', 'location'),
			t('2', 'states', 'location'),
			t('3', 'universities', 'education'),
			t('4', 'mentors') // no family
		]);
		expect(groups.map((g) => g.family.key)).toEqual(['location', 'education']);
		expect(groups[0].types.map((x) => x.slug)).toEqual(['countries', 'states']);
		expect(ungrouped.map((x) => x.slug)).toEqual(['mentors']);
	});

	it('keeps COMPARE_FAMILIES order regardless of input order', () => {
		const { groups } = groupTypesByFamily([
			t('1', 'universities', 'education'),
			t('2', 'countries', 'location')
		]);
		expect(groups.map((g) => g.family.key)).toEqual(['location', 'education']);
	});
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/main && bun run test -- compareFamilies`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement**

`apps/main/src/lib/components/compare/compareFamilies.ts`:

```ts
/**
 * Comparison-type families — the hub's category model. `key` is the admin-set
 * `config.family` value on a comparison type; `slug` is its URL form
 * (underscores → hyphens, e.g. visa_routes → /compare/visa-routes).
 *
 * Mirrors FAMILY_ORDER in TypeCardGrid.svelte: a new admin family needs a
 * one-line entry here to get a /compare/<slug> browse page.
 */
import type { Component } from 'svelte';
import {
	Globe,
	GraduationCap,
	Stamp,
	Landmark,
	Wallet,
	Building2,
	Plane,
	Briefcase,
	GitCompare
} from 'lucide-svelte';
import type { ComparisonType } from '#lib/types/jce.js';

// lucide-svelte ships legacy class-component typings; the family cards take
// Svelte 5 `Component` props, so cast at the seam (same convention as
// /compare's asComponent helper).
const asComponent = (icon: unknown): Component => icon as Component;

export interface CompareFamily {
	key: string;
	slug: string;
	icon: Component;
	tint: string;
}

export const COMPARE_FAMILIES: CompareFamily[] = [
	{ key: 'location', slug: 'location', icon: asComponent(Globe), tint: '#4f7cff' },
	{ key: 'education', slug: 'education', icon: asComponent(GraduationCap), tint: '#22a06b' },
	{ key: 'visa_routes', slug: 'visa-routes', icon: asComponent(Stamp), tint: '#8b5cf6' },
	{ key: 'banking', slug: 'banking', icon: asComponent(Landmark), tint: '#e8a13a' },
	{ key: 'cost-of-living', slug: 'cost-of-living', icon: asComponent(Wallet), tint: '#f97316' },
	{ key: 'embassies', slug: 'embassies', icon: asComponent(Building2), tint: '#ec4899' },
	{ key: 'travel', slug: 'travel', icon: asComponent(Plane), tint: '#06b6d4' },
	{ key: 'jobs', slug: 'jobs', icon: asComponent(Briefcase), tint: '#64748b' }
];

export const FALLBACK_FAMILY_ICON: Component = asComponent(GitCompare);

export function familySlug(key: string): string {
	return key.replace(/_/g, '-');
}

export function familyKeyFromSlug(slug: string): string {
	return slug.replace(/-/g, '_');
}

export function familyForKey(key: string): CompareFamily | undefined {
	return COMPARE_FAMILIES.find((f) => f.key === key);
}

export function familyForSlug(slug: string): CompareFamily | undefined {
	return COMPARE_FAMILIES.find((f) => f.slug === slug);
}

export interface FamilyGroup {
	family: CompareFamily;
	types: ComparisonType[];
}

/** Bucket types by config.family — groups in COMPARE_FAMILIES order, then
 *  ungrouped types (no family, or a family with no catalog entry). */
export function groupTypesByFamily(types: ComparisonType[]): {
	groups: FamilyGroup[];
	ungrouped: ComparisonType[];
} {
	const byKey = new Map<string, ComparisonType[]>();
	const ungrouped: ComparisonType[] = [];
	for (const cmpType of types) {
		const key = typeof cmpType.config?.family === 'string' ? cmpType.config.family : '';
		const family = key ? familyForKey(key) : undefined;
		if (!family) {
			ungrouped.push(cmpType);
			continue;
		}
		byKey.set(family.key, [...(byKey.get(family.key) ?? []), cmpType]);
	}
	const groups = COMPARE_FAMILIES.flatMap((family) => {
		const list = byKey.get(family.key);
		return list?.length ? [{ family, types: list }] : [];
	});
	return { groups, ungrouped };
}
```

`apps/main/src/params/compareCategory.ts`:

```ts
import { COMPARE_FAMILIES } from '#lib/components/compare/compareFamilies.js';

const slugs = new Set(COMPARE_FAMILIES.map((f) => f.slug));

// /compare/[category=compareCategory] sorts before /compare/[slug], so
// /compare/location renders the family browse page while comparison slugs
// keep hitting the quick-result route. A comparison literally slugged
// "location" would be shadowed — accepted (slugs are generated).
export function match(param: string): boolean {
	return slugs.has(param);
}
```

Type additions — `apps/main/src/lib/types/jce.ts`, after `JCEComparisonSummary` (~line 110):

```ts
export interface JCEPreviewItem {
	title: string;
	image_url?: string;
	geo_code?: string;
	country_code?: string;
	item_type?: string;
}
```

And add to `JCEComparisonSummary`: `preview_items?: JCEPreviewItem[];`

`apps/main/src/lib/types/index.ts`, extend `ListMeta`:

```ts
	sort_options?: string[];
	page_size_options?: number[];
	/** §JCE facets — present only when the request asks for facet=types. */
	facets?: { types?: { type_id: string; slug?: string; count: number }[] };
}
```

i18n — insert into `en.json` near the `jce.compare.*` block (~line 2588):

```json
	"jce.compare.family.location": "Location",
	"jce.compare.family.education": "Education",
	"jce.compare.family.visa_routes": "Visa Routes",
	"jce.compare.family.banking": "Banking & Finance",
	"jce.compare.family.cost-of-living": "Cost of Living",
	"jce.compare.family.embassies": "Embassies",
	"jce.compare.family.travel": "Travel",
	"jce.compare.family.jobs": "Jobs & Business",
	"jce.compare.family_desc.location": "Compare countries, states and cities to find the best places to live, study, work and do business.",
	"jce.compare.family_desc.education": "Compare universities, scholarships, courses and language tests.",
	"jce.compare.family_desc.visa_routes": "Compare work, student, tourist, family and PR visa routes.",
	"jce.compare.family_desc.banking": "Compare banks, remittance services and account setup.",
	"jce.compare.family_desc.cost-of-living": "Compare everyday costs — housing, transport, groceries and utilities.",
	"jce.compare.family_desc.embassies": "Compare embassies and consulates.",
	"jce.compare.family_desc.travel": "Compare travel routes.",
	"jce.compare.family_desc.jobs": "Compare job listings and business registration options.",
	"jce.compare.landing.search_placeholder": "Search countries, universities, visa routes, jobs...",
	"jce.compare.landing.all_categories": "All categories",
	"jce.compare.landing.sort_popular": "Popular",
	"jce.compare.landing.sort_recent": "Recently added",
	"jce.compare.landing.trending": "Trending comparisons",
	"jce.compare.landing.view_all": "View all comparisons",
	"jce.compare.landing.ai_help_title": "Need help comparing?",
	"jce.compare.landing.ai_help_body": "I can help you find the right options and answer your questions.",
	"jce.compare.landing.ai_help_cta": "Chat with Japa AI",
	"jce.compare.badge.popular": "Popular",
	"jce.compare.badge.trending": "Trending",
	"jce.compare.badge.hot": "Hot",
	"jce.compare.vs": "vs",
	"jce.compare.compare_now": "Compare now",
	"jce.compare.bookmark": "Bookmark",
	"jce.compare.category.comparisons_count": "{n} comparisons",
	"jce.compare.category.updated_weekly": "Updated weekly",
	"jce.compare.category.cant_decide": "Can't decide?",
	"jce.compare.category.cant_decide_body": "Compare up to {max} entries side by side.",
	"jce.compare.category.start_custom": "Start custom comparison",
	"jce.compare.category.refine": "Refine results",
	"jce.compare.category.categories": "Categories",
	"jce.compare.category.all": "All comparisons",
	"jce.compare.category.search_placeholder": "Search comparisons...",
	"jce.compare.category.create_own_title": "Create your own comparison",
	"jce.compare.category.create_own_body": "Choose your entries and compare the metrics that matter most to you.",
	"jce.compare.category.sort_placeholder": "Sort by",
	"jce.compare.category.sort.popularity": "Most popular",
```

- [ ] **Step 4: Run tests + typecheck**

Run: `cd apps/main && bun run test -- compareFamilies && bunx svelte-kit sync && bunx svelte-check --tsconfig ./tsconfig.json | tail -20`
Expected: tests PASS; svelte-check no new errors in touched files.

- [ ] **Step 5: Commit**

```bash
git add apps/main/src/lib/types/jce.ts apps/main/src/lib/types/index.ts apps/main/src/lib/components/compare/compareFamilies.ts apps/main/src/lib/components/compare/compareFamilies.test.ts apps/main/src/params/compareCategory.ts apps/main/src/lib/i18n/en.json
git commit -m "feat(compare): family catalog, preview types, category matcher"
```

---

### Task 5: Badge derivation + matrix best-cell helpers

**Files:**
- Create: `apps/main/src/lib/components/compare/compareBadge.ts`
- Create: `apps/main/src/lib/components/compare/compareBadge.test.ts`
- Modify: `apps/main/src/lib/utils/comparison.ts` (append `matrixBestCells`)
- Create: `apps/main/src/lib/utils/comparison.test.ts`

**Interfaces:**
- Produces: `badgeFor(item, {popularityIndex?, hotMinViews?, now?}) → 'popular'|'hot'|'trending'|''`, `matrixBestCells(matrix, dims) → Set<string>` (key `item.id:dim.id`). Consumed by Tasks 6, 9-12.

- [ ] **Step 1: Write the failing tests**

`compareBadge.test.ts`:

```ts
import { describe, expect, it } from 'vitest';
import { badgeFor } from './compareBadge.js';
import type { JCEComparisonSummary } from '#lib/types/jce.js';

const NOW = Date.parse('2026-11-11T00:00:00Z');
const item = (o: Partial<JCEComparisonSummary>): JCEComparisonSummary =>
	({ id: 'x', title: 't', slug: 's', status: 'published', visibility: 'public', depth: 'detailed', ...o }) as JCEComparisonSummary;

describe('badgeFor', () => {
	it('top-3 popularity position wins popular', () => {
		expect(badgeFor(item({ view_count: 10 }), { popularityIndex: 0, now: NOW })).toBe('popular');
		expect(badgeFor(item({ view_count: 999999 }), { popularityIndex: 2, now: NOW })).toBe('popular');
	});
	it('high views without a top slot is hot', () => {
		expect(badgeFor(item({ view_count: 6000 }), { popularityIndex: 5, now: NOW })).toBe('hot');
		expect(badgeFor(item({ view_count: 6000 }), { now: NOW })).toBe('hot');
	});
	it('recent comparisons are trending', () => {
		expect(badgeFor(item({ created_at: '2026-11-05T00:00:00Z', view_count: 3 }), { now: NOW })).toBe('trending');
	});
	it('old low-view items get no badge', () => {
		expect(badgeFor(item({ created_at: '2026-01-01T00:00:00Z', view_count: 2 }), { now: NOW })).toBe('');
	});
});
```

`comparison.test.ts`:

```ts
import { describe, expect, it } from 'vitest';
import { matrixBestCells } from './comparison.js';
import type { JCEDimension, MatrixItem } from '#lib/types/jce.js';

const dims: JCEDimension[] = [
	{ id: 'd-rating', label: 'Job Opportunities', dimension_type: 'rating' },
	{ id: 'd-cost', label: 'Cost of Living', dimension_type: 'cost' }
];
const matrix: MatrixItem[] = [
	{ id: 'a', title: 'A', dimensions: [
		{ dimension_id: 'd-rating', label: '', type: 'rating', value_rating: 4 },
		{ dimension_id: 'd-cost', label: '', type: 'cost', value_rating: 2000, currency_code: 'USD' }
	]},
	{ id: 'b', title: 'B', dimensions: [
		{ dimension_id: 'd-rating', label: '', type: 'rating', value_rating: 5 },
		{ dimension_id: 'd-cost', label: '', type: 'cost', value_rating: 2400, currency_code: 'USD' }
	]}
];

describe('matrixBestCells', () => {
	it('higher wins for ratings, lower wins for cost-like dims', () => {
		const best = matrixBestCells(matrix, dims);
		expect(best.has('b:d-rating')).toBe(true);
		expect(best.has('a:d-cost')).toBe(true);
		expect(best.size).toBe(2);
	});
	it('skips dimensions with <2 numeric cells', () => {
		const sparse: MatrixItem[] = [{ id: 'a', title: 'A', dimensions: [
			{ dimension_id: 'd-rating', label: '', type: 'rating', value_rating: 4 }
		]}];
		expect(matrixBestCells(sparse, dims).size).toBe(0);
	});
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd apps/main && bun run test -- compareBadge comparison`
Expected: FAIL — modules not found / function missing.

- [ ] **Step 3: Implement**

`apps/main/src/lib/components/compare/compareBadge.ts`:

```ts
/**
 * Badge derivation for comparison cards (spec §4, v1 client-side rules):
 * position ≤2 in a popularity sort → popular; view_count over threshold →
 * hot; created within 14 days → trending. Priority: popular > hot > trending.
 */
import type { JCEComparisonSummary } from '#lib/types/jce.js';

export type CompareBadgeKind = 'popular' | 'hot' | 'trending' | '';

export const HOT_MIN_VIEWS = 5000;
const TRENDING_WINDOW_MS = 14 * 24 * 60 * 60 * 1000;

export function badgeFor(
	item: JCEComparisonSummary,
	opts: { popularityIndex?: number; hotMinViews?: number; now?: number } = {}
): CompareBadgeKind {
	if ((opts.popularityIndex ?? Infinity) <= 2) return 'popular';
	if ((item.view_count ?? 0) >= (opts.hotMinViews ?? HOT_MIN_VIEWS)) return 'hot';
	const created = Date.parse(item.created_at ?? '');
	const now = opts.now ?? Date.now();
	if (!Number.isNaN(created) && now - created < TRENDING_WINDOW_MS) return 'trending';
	return '';
}
```

Append to `apps/main/src/lib/utils/comparison.ts` (keep the file's existing exports untouched):

```ts
/**
 * Best-value cell per dimension row. Numeric comparisons only — value_rating
 * first, then a numeric parse of value (strings like "$1,200" are skipped).
 * Cost-like dimensions (dimension_type 'cost' or a cost/price/fee/tuition
 * label) invert to lower-is-better. Keys are `${item.id}:${dim.id}` for the
 * matrix <td> lookup. Rows with fewer than two numeric cells are skipped.
 */
export function matrixBestCells(matrix: MatrixItem[], dims: JCEDimension[]): Set<string> {
	const best = new Set<string>();
	for (const dim of dims) {
		const lowerIsBetter =
			dim.dimension_type === 'cost' || /cost|price|fee|tuition/i.test(dim.label);
		let bestVal: number | null = null;
		let bestKey = '';
		for (const item of matrix) {
			const cell = item.dimensions.find((d) => d.dimension_id === dim.id);
			const v =
				cell?.value_rating ??
				(cell?.value != null && cell.value !== '' ? Number(cell.value) : null);
			if (v == null || Number.isNaN(v)) continue;
			if (bestVal === null || (lowerIsBetter ? v < bestVal : v > bestVal)) {
				bestVal = v;
				bestKey = `${item.id}:${dim.id}`;
			}
		}
		if (bestKey) best.add(bestKey);
	}
	return best;
}
```

Verify `MatrixItem`/`JCEDimension` are already imported in `comparison.ts` (the file already exports `deduplicateMatrix`, `uniqueDimensions`, `uniqueResolvedItems` — they are).

- [ ] **Step 4: Run tests**

Run: `cd apps/main && bun run test -- compareBadge comparison`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/main/src/lib/components/compare/compareBadge.ts apps/main/src/lib/components/compare/compareBadge.test.ts apps/main/src/lib/utils/comparison.ts apps/main/src/lib/utils/comparison.test.ts
git commit -m "feat(compare): badge derivation + matrix best-cell helpers"
```

---

### Task 6: `ComparisonVsCard` component

**Files:**
- Create: `apps/main/src/lib/components/compare/ComparisonVsCard.svelte`

**Interfaces:**
- Consumes: `JCEComparisonSummary` (+`preview_items`), `flagUrl`/`flagEmoji` (`#lib/utils/flags.js`), `badgeFor`/`CompareBadgeKind`, `getViewerState`/`updateViewerState` (`#lib/api/viewerState.js`), `api` client, `t()`/`i18n`.
- Produces: `<ComparisonVsCard summary={...} badge="popular" authed={bool} />` — the signature card reused by Tasks 9-11.

- [ ] **Step 1: Create the component**

```svelte
<script lang="ts">
	/**
	 * ComparisonVsCard — the hub's signature card: diagonal split header from
	 * the first two preview_items images (tinted fallback), a flag-pair "vs"
	 * medallion when the items carry geo codes, badge chip, title, excerpt,
	 * "Compare now →" and a viewer-state bookmark + compact view count.
	 */
	import { Bookmark, Eye } from 'lucide-svelte';
	import { api } from '#lib/api/client.js';
	import { getViewerState, updateViewerState } from '#lib/api/viewerState.js';
	import { t } from '#lib/i18n/index.js';
	import { flagUrl } from '#lib/utils/flags.js';
	import type { JCEComparisonSummary, JCEPreviewItem } from '#lib/types/jce.js';
	import type { CompareBadgeKind } from './compareBadge.js';

	let {
		summary,
		badge = '',
		authed = false
	}: { summary: JCEComparisonSummary; badge?: CompareBadgeKind; authed?: boolean } = $props();

	const previews = $derived((summary.preview_items ?? []).slice(0, 3));
	const href = $derived(
		summary.depth === 'quick' ? `/compare/${summary.slug}` : `/comparisons/${summary.slug}`
	);

	function codeOf(item: JCEPreviewItem): string {
		return (item.country_code ?? item.geo_code ?? '').toLowerCase();
	}
	// The medallion shows when ≥2 previews resolve a flag; fewer → initials.
	const flagPair = $derived.by(() => {
		const codes = previews.map(codeOf).filter(Boolean);
		return codes.length >= 2 ? [flagUrl(codes[0]), flagUrl(codes[1])] : null;
	});

	const views = $derived(
		new Intl.NumberFormat(undefined, { notation: 'compact', maximumFractionDigits: 1 }).format(
			summary.view_count ?? 0
		)
	);

	let bookmarked = $state(false);
	let bookmarkInit = $state(false);
	$effect(() => {
		if (bookmarkInit || !authed) return;
		bookmarkInit = true;
		// Batched: one GET /me/content-states serves every card on the page.
		getViewerState(summary.id).then((s) => {
			bookmarked = s.bookmarked;
		});
	});

	async function toggleBookmark(e: MouseEvent) {
		e.preventDefault();
		e.stopPropagation();
		if (!authed) return;
		const next = !bookmarked;
		bookmarked = next;
		updateViewerState(summary.id, { bookmarked: next });
		try {
			if (next) await api.post(`/content/${summary.id}/bookmark`, {});
			else await api.delete(`/content/${summary.id}/bookmark`);
		} catch {
			bookmarked = !next;
			updateViewerState(summary.id, { bookmarked: !next });
		}
	}
</script>

<a
	{href}
	class="group flex flex-col overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] transition-colors hover:bg-[var(--color-surface-subtle)]"
>
	<!-- Split header: first two preview images, diagonal seam; falls back to
	     a tinted placeholder when previews/images are absent. -->
	<div class="relative h-36 overflow-hidden bg-[var(--color-surface-subtle)]">
		{#if previews[0]?.image_url && previews[1]?.image_url}
			<img src={previews[0].image_url} alt="" class="absolute inset-0 h-full w-full object-cover" />
			<img
				src={previews[1].image_url}
				alt=""
				class="absolute inset-0 h-full w-full object-cover"
				style="clip-path: polygon(58% 0, 100% 0, 100% 100%, 42% 100%)"
			/>
			<div
				class="absolute inset-y-0 w-[3px] bg-[var(--color-surface)]"
				style="left: 50%; transform: translateX(-50%) skewX(-14deg)"
				aria-hidden="true"
			></div>
		{:else if previews[0]?.image_url}
			<img src={previews[0].image_url} alt="" class="absolute inset-0 h-full w-full object-cover" />
		{/if}

		{#if flagPair}
			<div class="absolute inset-x-0 bottom-2 flex items-center justify-center gap-1" aria-hidden="true">
				{#if flagPair[0]}
					<img src={flagPair[0]} alt="" class="h-7 w-7 rounded-full border-2 border-[var(--color-surface)] object-cover shadow-sm" />
				{/if}
				<span class="rounded-full bg-[var(--color-surface)] px-1.5 py-0.5 text-[10px] font-bold text-[var(--color-muted)] shadow-sm">{t('jce.compare.vs')}</span>
				{#if flagPair[1]}
					<img src={flagPair[1]} alt="" class="h-7 w-7 rounded-full border-2 border-[var(--color-surface)] object-cover shadow-sm" />
				{/if}
			</div>
		{/if}

		{#if badge}
			<span
				class="absolute start-2 top-2 rounded-full px-2 py-0.5 text-xs font-medium
					{badge === 'popular' ? 'bg-emerald-500/90 text-white' : ''}
					{badge === 'hot' ? 'bg-red-500/90 text-white' : ''}
					{badge === 'trending' ? 'bg-[var(--color-primary)]/90 text-[var(--color-primary-contrast)]' : ''}"
			>{t(`jce.compare.badge.${badge}`)}</span>
		{/if}
	</div>

	<div class="flex flex-1 flex-col gap-2 p-4">
		<h3 class="font-semibold leading-snug text-[var(--color-text)]">{summary.title}</h3>
		{#if summary.excerpt}
			<p class="line-clamp-2 text-sm text-[var(--color-muted)]">{summary.excerpt}</p>
		{/if}
		<div class="mt-auto flex items-center justify-between gap-2 pt-1">
			<span class="inline-flex items-center gap-1 text-sm font-medium text-[var(--color-primary-text)]">
				{t('jce.compare.compare_now')}
				<span aria-hidden="true" class="rtl:-scale-x-100">→</span>
			</span>
			<span class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
				{#if authed}
					<button
						type="button"
						onclick={toggleBookmark}
						aria-pressed={bookmarked}
						aria-label={t('jce.compare.bookmark')}
						class="rounded p-0.5 transition-colors hover:text-[var(--color-text)] {bookmarked ? 'text-[var(--color-primary-text)]' : ''}"
					>
						<Bookmark size={14} fill={bookmarked ? 'currentColor' : 'none'} />
					</button>
				{/if}
				<span class="inline-flex items-center gap-1"><Eye size={12} /> {views}</span>
			</span>
		</div>
	</div>
</a>
```

Check `--color-primary-text` exists (used across the app — it appears in +page.svelte and comparisons page); if the primary-contrast token name differs, use the same var the app's primary buttons use. Check `excerpt` field exists on `JCEComparisonSummary` — it does (line 116).

- [ ] **Step 2: Typecheck**

Run: `cd apps/main && bunx svelte-check --tsconfig ./tsconfig.json | tail -20`
Expected: no errors in `ComparisonVsCard.svelte`.

- [ ] **Step 3: Commit**

```bash
git add apps/main/src/lib/components/compare/ComparisonVsCard.svelte
git commit -m "feat(compare): ComparisonVsCard — split-image vs card with flags + bookmark"
```

---

### Task 7: `CompareCategoryCard` + `CompareRefineRail`

**Files:**
- Create: `apps/main/src/lib/components/compare/CompareCategoryCard.svelte`
- Create: `apps/main/src/lib/components/compare/CompareRefineRail.svelte`

**Interfaces:**
- Consumes: `CompareFamily`/`ComparisonType`, `t()`, `JceSelect` not needed (rail uses links).
- Produces:
  - `<CompareCategoryCard {icon} {tint} {name} {subtitle} {count}? {href} />`
  - `<CompareRefineRail {title} bind:query {categories} {onQuery} />` where `categories: {label: string; count?: number; href: string; active: boolean}[]`.

- [ ] **Step 1: Create `CompareCategoryCard.svelte`**

```svelte
<script lang="ts">
	/** Category tile for the /compare landing: tinted icon chip, name,
	 *  subcategory subtitle, optional comparison count, trailing chevron. */
	import { ChevronRight } from 'lucide-svelte';
	import type { Component } from 'svelte';

	let {
		icon: Icon,
		tint,
		name,
		subtitle = '',
		count,
		href
	}: {
		icon: Component;
		tint: string;
		name: string;
		subtitle?: string;
		count?: number;
		href: string;
	} = $props();
</script>

<a
	{href}
	class="group flex items-center gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:bg-[var(--color-surface-subtle)]"
>
	<span
		class="grid h-11 w-11 shrink-0 place-items-center rounded-[var(--radius)]"
		style="background: color-mix(in srgb, {tint} 14%, transparent); color: {tint}"
	>
		<Icon size={22} />
	</span>
	<span class="min-w-0 flex-1">
		<span class="block font-semibold text-[var(--color-text)]">
			{name}
			{#if count !== undefined}
				<span class="ms-1 text-xs font-normal text-[var(--color-muted)]">{count}</span>
			{/if}
		</span>
		{#if subtitle}
			<span class="mt-0.5 line-clamp-2 block text-sm text-[var(--color-muted)]">{subtitle}</span>
		{/if}
	</span>
	<ChevronRight size={16} class="shrink-0 text-[var(--color-muted)] transition-transform group-hover:translate-x-0.5 rtl:-scale-x-100" />
</a>
```

- [ ] **Step 2: Create `CompareRefineRail.svelte`**

```svelte
<script lang="ts">
	/** Refine rail — search + counted category links (mockup 2 left column).
	 *  The themes block only renders when the caller supplies real facets. */
	import { Search } from 'lucide-svelte';
	import { t } from '#lib/i18n/index.js';

	export interface RailCategory {
		label: string;
		count?: number;
		href: string;
		active: boolean;
	}

	let {
		query = $bindable(''),
		categories,
		onQuery
	}: {
		query?: string;
		categories: RailCategory[];
		onQuery: (q: string) => void;
	} = $props();

	let timeout: ReturnType<typeof setTimeout> | null = null;
	function input(e: Event) {
		query = (e.currentTarget as HTMLInputElement).value;
		if (timeout) clearTimeout(timeout);
		timeout = setTimeout(() => onQuery(query), 300);
	}
</script>

<aside class="w-full shrink-0 space-y-5 lg:w-60">
	<h2 class="text-sm font-semibold text-[var(--color-text)]">{t('jce.compare.category.refine')}</h2>
	<div class="relative">
		<Search size={15} class="absolute start-3 top-1/2 -translate-y-1/2 text-[var(--color-muted)]" />
		<input
			type="text"
			value={query}
			oninput={input}
			placeholder={t('jce.compare.category.search_placeholder')}
			class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] py-2 ps-9 pe-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
		/>
	</div>

	<nav class="space-y-1" aria-label={t('jce.compare.category.categories')}>
		<p class="pb-1 text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)]">
			{t('jce.compare.category.categories')}
		</p>
		{#each categories as cat (cat.href)}
			<a
				href={cat.href}
				aria-current={cat.active ? 'page' : undefined}
				class="flex items-center justify-between rounded-[var(--radius)] px-3 py-2 text-sm transition-colors
					{cat.active
						? 'bg-[var(--color-primary)]/10 font-medium text-[var(--color-primary-text)]'
						: 'text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]'}"
			>
				<span>{cat.label}</span>
				{#if cat.count !== undefined}
					<span class="text-xs text-[var(--color-muted)]">{cat.count}</span>
				{/if}
			</a>
		{/each}
	</nav>
</aside>
```

- [ ] **Step 3: Typecheck**

Run: `cd apps/main && bunx svelte-check --tsconfig ./tsconfig.json | tail -20`
Expected: no errors in the two new files.

- [ ] **Step 4: Commit**

```bash
git add apps/main/src/lib/components/compare/CompareCategoryCard.svelte apps/main/src/lib/components/compare/CompareRefineRail.svelte
git commit -m "feat(compare): category card + refine rail components"
```

---

### Task 8: `HeroArtCompare` — stacked entity-card scene

**Files:**
- Modify: `apps/main/src/lib/components/heroes/HeroArtCompare.svelte`

**Interfaces:**
- Consumes: same `accent` prop — no signature change (both call sites keep working).
- Produces: three stacked mini entity cards with header bands + dot-metric rows, evoking the mockup's Canada/Australia/Germany cards.

- [ ] **Step 1: Replace the SVG body**

Keep the `accent` prop and the orbit/ellipse + "VS" node; replace the two blank cards with three stacked cards (offset depths like the mockup fan). Full file:

```svelte
<script lang="ts">
	/**
	 * /compare + /comparisons hero scene — a fanned stack of entity cards
	 * (header band + two dot-metric rows each) over the orbit + "VS" node.
	 */
	let { accent = '#3b49df' }: { accent?: string } = $props();

	// Three stacked cards — depth order back→front.
	const cards = [
		{ x: 58, y: 62, rot: -7, tint: 0.05, flags: '#e5453c' },
		{ x: 140, y: 48, rot: 0, tint: 0.1, flags: '#3b49df' },
		{ x: 226, y: 62, rot: 7, tint: 0.05, flags: '#e8a13a' }
	];
</script>

<svg viewBox="0 0 380 224" class="h-full w-full" role="presentation">
	<circle cx="190" cy="112" r="104" fill={accent} opacity="0.05" />
	<ellipse
		cx="190" cy="112" rx="160" ry="82" fill="none"
		stroke={accent} stroke-width="1" opacity="0.22"
		stroke-dasharray="2 6" transform="rotate(-6 190 112)"
	/>

	{#each cards as card, i (i)}
		<g transform="rotate({card.rot} {card.x + 52} {card.y + 60})">
			<rect x={card.x} y={card.y} width="104" height="122" rx="12"
				fill="var(--color-surface)" stroke="var(--color-border)" />
			<!-- header band (skyline stand-in) -->
			<rect x={card.x} y={card.y} width="104" height="42" rx="12" fill={accent} opacity="0.16" />
			<rect x={card.x} y={card.y + 30} width="104" height="12" fill={accent} opacity="0.16" />
			<!-- flag chip -->
			<circle cx={card.x + 16} cy={card.y + 14} r="8" fill={card.flags} opacity="0.85" />
			<rect x={card.x + 28} y={card.y + 11} width="40" height="6" rx="3" fill="var(--color-text)" opacity="0.55" />
			<!-- metric rows: label bar + dot rating -->
			{#each [0, 1] as row (row)}
				{@const y = card.y + 62 + row * 26}
				<rect x={card.x + 12} {y} width="52" height="6" rx="3" fill={accent} opacity="0.28" />
				{#each [0, 1, 2, 3] as dot (dot)}
					<circle cx={card.x + 70 + dot * 9} cy={y + 3} r="3"
						fill={accent} opacity={dot < 3 - row ? 0.7 : 0.18} />
				{/each}
			{/each}
			<!-- footer line -->
			<rect x={card.x + 12} y={card.y + 108} width="80" height="5" rx="2.5" fill={accent} opacity="0.12" />
		</g>
	{/each}

	<!-- center "vs" node stays the signature -->
	<circle cx="190" cy="150" r="17" fill="var(--color-surface)" stroke={accent} stroke-width="1.5" />
	<text x="190" y="154" text-anchor="middle" font-size="11" font-weight="700"
		fill={accent} font-family="system-ui, sans-serif">VS</text>
</svg>
```

- [ ] **Step 2: Typecheck + eyeball**

Run: `cd apps/main && bunx svelte-check --tsconfig ./tsconfig.json | tail -10`
Expected: no errors. (Visual check happens in Task 9's smoke test.)

- [ ] **Step 3: Commit**

```bash
git add apps/main/src/lib/components/heroes/HeroArtCompare.svelte
git commit -m "feat(compare): hero art — fanned entity-card scene"
```

---

### Task 9: `/compare` landing — search panel, category grid, trending, AI card

**Files:**
- Modify: `apps/main/src/routes/(app)/compare/+page.server.ts` (~line 439 `popularPromise`)
- Modify: `apps/main/src/routes/(app)/compare/+page.svelte` (landing branch ~line 871-906 + imports)

**Interfaces:**
- Consumes: `groupTypesByFamily`, `COMPARE_FAMILIES`, `CompareCategoryCard`, `ComparisonVsCard`, `badgeFor`, `JceSelect`, existing `data.types`/`popularComparisons`.
- Produces: `data.popularComparisons` (server field name unchanged — only `page_size` bumps to 9); landing helpers `familySubtitle(types)`, `detailURL(item)`, `setTrendingSort`.

- [ ] **Step 1: Server — keep the popular fetch, bump page_size to 9**

In `+page.server.ts` the existing `popularPromise` (~line 439) already fetches published/detailed/popularity — change `page_size=6` → `page_size=9` so the trending row has enough cards. No other server change needed (category cards derive from `types`, already loaded).

- [ ] **Step 2: Rewrite the landing branch**

In `+page.svelte`, the `{#if mine}` / `{:else if !selectedType}` / `{:else}` block — only the middle branch changes. Add imports:

```ts
import { COMPARE_FAMILIES, FALLBACK_FAMILY_ICON, groupTypesByFamily } from '#lib/components/compare/compareFamilies.js';
import CompareCategoryCard from '#lib/components/compare/CompareCategoryCard.svelte';
import ComparisonVsCard from '#lib/components/compare/ComparisonVsCard.svelte';
import { badgeFor } from '#lib/components/compare/compareBadge.js';
import { MessageCircle } from 'lucide-svelte';
```

Add state + derived near the other landing vars (~line 224 `popularComparisons`):

```ts
let familyGroups = $derived(groupTypesByFamily(types));
let trendingSort = $state<'popularity' | 'created_at'>('popularity');
let trendingItems = $state<JCEComparisonSummary[] | null>(null); // null = use server seed
let trendingLoading = $state(false);

async function setTrendingSort(next: 'popularity' | 'created_at') {
	if (next === trendingSort) return;
	trendingSort = next;
	if (next === 'popularity') { trendingItems = null; return; }
	trendingLoading = true;
	const res = await api.get<JCEComparisonSummary[]>(
		'/jce/comparisons?filter_depth=detailed&filter_status=published&filter_visibility=public&page_size=9&sort=created_at'
	);
	trendingLoading = false;
	trendingItems = res.data ?? [];
}
let trending = $derived(trendingItems ?? popularComparisons);

// Card subtitle: member type names, else item_ref_types labels.
function familySubtitle(members: ComparisonType[]): string {
	if (members.length > 1) return members.map((m) => m.name).join(', ');
	const refs = (members[0]?.config?.item_ref_types as string[] | undefined) ?? [];
	return refs.map(sourcePlural).join(', ');
}
function detailURL(item: JCEComparisonSummary): string {
	return item.depth === 'quick' ? `/compare/${item.slug}` : `/comparisons/${item.slug}`;
}
let landingSearch = $state('');
function submitLandingSearch() {
	if (!landingSearch.trim()) return;
	goto(`/comparisons?q=${encodeURIComponent(landingSearch.trim())}`);
}
```

Replace the `{:else if !selectedType}` branch (~lines 871-906) with:

```svelte
{:else if !selectedType}
	<!-- Landing: search panel → category grid → trending → AI help -->
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 sm:p-5">
		<h2 class="mb-3 text-base font-semibold text-[var(--color-text)]">
			{t('jce.compare.choose_type')}
		</h2>
		<form class="flex flex-col gap-3 sm:flex-row" onsubmit={(e) => { e.preventDefault(); submitLandingSearch(); }}>
			<div class="relative flex-1">
				<Search size={16} class="absolute start-3 top-1/2 -translate-y-1/2 text-[var(--color-muted)]" />
				<input
					type="text"
					bind:value={landingSearch}
					placeholder={t('jce.compare.landing.search_placeholder')}
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] py-2.5 ps-9 pe-4 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
				/>
			</div>
			<div class="flex items-center gap-2">
				{#each ([
					{ v: 'popularity', label: t('jce.compare.landing.sort_popular') },
					{ v: 'created_at', label: t('jce.compare.landing.sort_recent') }
				] as const) as pill (pill.v)}
					<button
						type="button"
						onclick={() => setTrendingSort(pill.v)}
						aria-pressed={trendingSort === pill.v}
						class="rounded-[var(--radius)] border px-3 py-2 text-sm font-medium transition-colors
							{trendingSort === pill.v ? 'border-[var(--color-primary)] bg-[var(--color-primary)]/10 text-[var(--color-primary-text)]' : 'border-[var(--color-border)] text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
					>{pill.label}</button>
				{/each}
			</div>
		</form>

		<div class="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4">
			{#each familyGroups.groups as g (g.family.key)}
				<CompareCategoryCard
					icon={g.family.icon}
					tint={g.family.tint}
					name={t(`jce.compare.family.${g.family.key}`, g.family.key)}
					subtitle={familySubtitle(g.types)}
					href="/compare/{g.family.slug}"
				/>
			{/each}
			{#each familyGroups.ungrouped as cmpType (cmpType.id)}
				<CompareCategoryCard
					icon={typeIcon(cmpType)}
					tint="var(--color-primary)"
					name={cmpType.name}
					subtitle={cmpType.description ?? ''}
					count={cmpType.item_count}
					href="/comparisons?filter_type={cmpType.slug}"
				/>
			{/each}
		</div>
	</section>

	{#if trending.length > 0}
		<section class="space-y-3">
			<div class="flex items-center justify-between">
				<h2 class="text-sm font-semibold text-[var(--color-text)]">{t('jce.compare.landing.trending')}</h2>
				<a href="/comparisons" class="text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]">
					{t('jce.compare.landing.view_all')} →
				</a>
			</div>
			<div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
				{#each trending as item, i (item.id)}
					<ComparisonVsCard
						summary={item}
						badge={trendingSort === 'popularity' ? badgeFor(item, { popularityIndex: i }) : badgeFor(item)}
						authed={Boolean(user)}
					/>
				{/each}
			</div>
		</section>
	{/if}

	{#if aiQuickEnabled}
		<section class="flex items-center gap-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<span class="grid h-12 w-12 shrink-0 place-items-center rounded-full bg-[var(--color-primary)]/10 text-[var(--color-primary-text)]">
				<MessageCircle size={22} />
			</span>
			<div class="min-w-0 flex-1">
				<p class="font-semibold text-[var(--color-text)]">{t('jce.compare.landing.ai_help_title')}</p>
				<p class="text-sm text-[var(--color-muted)]">{t('jce.compare.landing.ai_help_body')}</p>
			</div>
			<button
				type="button"
				onclick={() => (aiQuickOpen = true)}
				class="shrink-0 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)]"
			>{t('jce.compare.landing.ai_help_cta')}</button>
		</section>
	{/if}
```

Note: `aiQuickOpen`/`aiQuickEnabled`/`aiQuickOpen` state already exist on the page; the `AiQuickPanel` mount stays wherever it currently is. If `aiQuickEnabled` is false the section is skipped (spec: feature-gated as today).

- [ ] **Step 3: Typecheck + smoke**

Run: `cd apps/main && bunx svelte-check --tsconfig ./tsconfig.json | tail -20`
Expected: no errors in `+page.svelte`.

Smoke: `cd apps/main && bunx vite dev --port 3000` (background), visit `/compare`, verify the search panel, category grid, trending row, and that `?type=<slug>` + `?mine=true` still work.

- [ ] **Step 4: Commit**

```bash
git add "apps/main/src/routes/(app)/compare/+page.svelte" "apps/main/src/routes/(app)/compare/+page.server.ts"
git commit -m "feat(compare): discovery landing — search, category grid, trending vs-cards"
```

---

### Task 10: `/compare/[category]` family browse page

**Files:**
- Create: `apps/main/src/routes/(app)/compare/[category=compareCategory]/+page.server.ts`
- Create: `apps/main/src/routes/(app)/compare/[category=compareCategory]/+page.svelte`

**Interfaces:**
- Consumes: `familyForSlug`/`COMPARE_FAMILIES`, `groupTypesByFamily` (member types by `config.family === family.key`), `apiList`, `CompareRefineRail`, `ComparisonVsCard`, `badgeFor`, `PaginationControls`, `JceSelect`, `SeoHead`.
- Produces: `/compare/<family>` route; URL params `sub` (subcategory type slug), `q`, `sort`, `page`, `page_size` — same conventions as `/comparisons`.

- [ ] **Step 1: Create `+page.server.ts`**

```ts
import { error } from '@sveltejs/kit';
import { apiContext, apiGet, apiList } from '#lib/server/api.js';
import { familyForSlug } from '#lib/components/compare/compareFamilies.js';
import type { ComparisonType, ComparisonTypeConfig, JCEComparisonSummary } from '#lib/types/jce.js';
import type { PageServerLoad } from './$types';

function parseConfig(raw: unknown): ComparisonTypeConfig | null {
	if (!raw) return null;
	if (typeof raw === 'object') return raw as ComparisonTypeConfig;
	try {
		return JSON.parse(Buffer.from(String(raw), 'base64').toString('utf-8')) as ComparisonTypeConfig;
	} catch {
		return null;
	}
}

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const family = familyForSlug(event.params.category);
	if (!family) throw error(404);

	let types: ComparisonType[] = [];
	try {
		const res = await apiGet<ComparisonType[]>(ctx, '/jce/comparison-types?page_size=100');
		types = (res ?? []).map((t) => ({ ...t, config: parseConfig(t.config as unknown) ?? undefined }));
	} catch {
		types = [];
	}
	const memberTypes = types.filter((t) => t.config?.family === family.key);
	if (memberTypes.length === 0) throw error(404);

	const sp = event.url.searchParams;
	const sub = sp.get('sub') ?? '';
	const q = sp.get('q') ?? '';
	const sort = sp.get('sort') ?? 'popularity';
	const page = Math.max(1, parseInt(sp.get('page') ?? '1', 10) || 1);
	const pageSize = Math.min(96, Math.max(1, parseInt(sp.get('page_size') ?? '12', 10) || 12));

	const activeSlugs = sub && memberTypes.some((t) => t.slug === sub) ? [sub] : memberTypes.map((t) => t.slug);
	const params = new URLSearchParams({
		filter_type: activeSlugs.join(','),
		filter_depth: 'detailed',
		filter_status: 'published',
		filter_visibility: 'public',
		sort,
		page: String(page),
		page_size: String(pageSize),
		facet: 'types',
		facet_scope: memberTypes.map((t) => t.slug).join(',')
	});
	if (q) params.set('q', q);

	let items: JCEComparisonSummary[] = [];
	let meta = { total: 0, page: 1, limit: pageSize } as Record<string, unknown>;
	let loadError = '';
	try {
		const res = await apiList<JCEComparisonSummary>(ctx, `/jce/comparisons?${params}`);
		items = res.items;
		meta = res.meta ?? meta;
	} catch {
		loadError = 'jce.comparisons.errors.load';
	}

	return {
		// Serializable only — the page resolves the full CompareFamily
		// (icon component isn't serializable across the load boundary).
		familySlug: family.slug,
		memberTypes,
		items,
		meta,
		sub,
		filters: { q, sort, page, page_size: pageSize },
		loadError,
		user: event.locals.user
	};
};
```

- [ ] **Step 2: Create `+page.svelte`**

```svelte
<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { onDestroy } from 'svelte';
	import { ChevronRight, GitCompare, Scale } from 'lucide-svelte';
	import SeoHead from '#lib/seo/SeoHead.svelte';
	import { t, siteName } from '#lib/i18n/index.js';
	import JceSelect from '#lib/components/ui/JceSelect.svelte';
	import { EmptyState, PaginationControls } from '#lib/components/ui/index.js';
	import CompareRefineRail from '#lib/components/compare/CompareRefineRail.svelte';
	import ComparisonVsCard from '#lib/components/compare/ComparisonVsCard.svelte';
	import { badgeFor } from '#lib/components/compare/compareBadge.js';
	import { familyForSlug } from '#lib/components/compare/compareFamilies.js';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let family = $derived(familyForSlug(data.familySlug)!);
	let FamIcon = $derived(family.icon);
	let memberTypes = $derived(data.memberTypes);
	let meta = $derived(data.meta as { total?: number; page?: number; limit?: number; sort_options?: string[]; facets?: { types?: { type_id: string; slug?: string; count: number }[] } });
	let query = $state(data.filters.q);
	let timeout: ReturnType<typeof setTimeout> | null = null;
	onDestroy(() => { if (timeout) clearTimeout(timeout); });

	const facetCountBySlug = $derived.by(() => {
		const map: Record<string, number> = {};
		for (const f of meta.facets?.types ?? []) if (f.slug) map[f.slug] = f.count;
		return map;
	});

	function url(updates: Record<string, string | null>): string {
		const u = new URL(page.url.href);
		for (const [k, v] of Object.entries(updates)) {
			if (v === null || v === '') u.searchParams.delete(k);
			else u.searchParams.set(k, v);
		}
		return u.pathname + u.search;
	}

	const railCategories = $derived([
		{
			label: t('jce.compare.category.all'),
			count: meta.total ?? 0,
			href: url({ sub: null, page: null }),
			active: !data.sub
		},
		...memberTypes.map((mt) => ({
			label: mt.name,
			count: facetCountBySlug[mt.slug] ?? 0,
			href: url({ sub: mt.slug, page: null }),
			active: data.sub === mt.slug
		}))
	]);

	function onQuery(q: string) {
		goto(url({ q: q || null, page: null }), { replace: true, reset: false });
	}

	const sortOptions = $derived(
		(meta.sort_options ?? ['popularity']).map((v) => ({
			value: v,
			label: t(`jce.comparisons.sort.${v}`, v)
		}))
	);

	const totalPages = $derived(Math.max(1, Math.ceil((meta.total ?? 0) / (meta.limit ?? 12))));
</script>

<SeoHead
	title={`${t(`jce.compare.family.${family.key}`, family.key)} — ${t('jce.compare.heading')} — ${siteName()}`}
	description={t(`jce.compare.family_desc.${family.key}`, '')}
/>

<div class="mx-auto w-full max-w-6xl space-y-6 p-4">
	<!-- Breadcrumb + category header -->
	<nav class="flex items-center gap-1.5 text-sm text-[var(--color-muted)]" aria-label="breadcrumb">
		<a href="/" class="hover:text-[var(--color-text)]">{t('nav.home', 'Home')}</a>
		<ChevronRight size={13} class="rtl:-scale-x-100" />
		<a href="/compare" class="hover:text-[var(--color-text)]">{t('jce.compare.heading')}</a>
		<ChevronRight size={13} class="rtl:-scale-x-100" />
		<span class="text-[var(--color-text)]">{t(`jce.compare.family.${family.key}`, family.key)}</span>
	</nav>

	<header class="flex flex-wrap items-start justify-between gap-4">
		<div class="flex items-start gap-4">
			<span
				class="grid h-14 w-14 place-items-center rounded-[var(--radius-card)]"
				style="background: color-mix(in srgb, {family.tint} 14%, transparent); color: {family.tint}"
			>
				<FamIcon size={28} />
			</span>
			<div>
				<h1 class="text-2xl font-bold text-[var(--color-text)] sm:text-3xl">
					{t(`jce.compare.family.${family.key}`, family.key)}
				</h1>
				<p class="mt-1 max-w-md text-sm text-[var(--color-muted)]">
					{t(`jce.compare.family_desc.${family.key}`, '')}
				</p>
				<p class="mt-2 flex flex-wrap items-center gap-3 text-xs text-[var(--color-muted)]">
					<span class="inline-flex items-center gap-1">
						<GitCompare size={12} />
						{t('jce.compare.category.comparisons_count', '{n} comparisons', { n: String(meta.total ?? 0) })}
					</span>
					<span>{t('jce.compare.category.updated_weekly')}</span>
				</p>
			</div>
		</div>

		<!-- Can't decide? → picker handoff -->
		<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<p class="text-sm font-semibold text-[var(--color-text)]">{t('jce.compare.category.cant_decide')}</p>
			<p class="mt-0.5 text-xs text-[var(--color-muted)]">{t('jce.compare.category.cant_decide_body', undefined, { max: '3' })}</p>
			{#if memberTypes.length === 1}
				<a href="/compare?type={memberTypes[0].slug}" class="mt-3 inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)]">
					<Scale size={14} /> {t('jce.compare.category.start_custom')}
				</a>
			{:else}
				<div class="mt-3 flex flex-wrap gap-2">
					{#each memberTypes as mt (mt.id)}
						<a href="/compare?type={mt.slug}" class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]">{mt.name}</a>
					{/each}
				</div>
			{/if}
		</div>
	</header>

	<div class="flex flex-col gap-6 lg:flex-row">
		<CompareRefineRail bind:query categories={railCategories} {onQuery} />

		<main class="min-w-0 flex-1 space-y-4">
			<div class="flex items-center justify-between gap-3">
				<p class="text-sm font-medium text-[var(--color-text)]">
					{t('jce.compare.category.comparisons_count', '{n} comparisons', { n: String(meta.total ?? 0) })}
				</p>
				<JceSelect
					options={sortOptions}
					value={data.filters.sort}
					onchange={(v) => goto(url({ sort: v, page: null }), { replace: true })}
					class="w-44"
				/>
			</div>

			{#if data.loadError}
				<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">{t(data.loadError)}</p>
			{:else if data.items.length === 0}
				<EmptyState icon={GitCompare} title={t('jce.comparisons.empty_public')} />
			{:else}
				<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
					{#each data.items as item, i (item.id)}
						<ComparisonVsCard
							summary={item}
							badge={data.filters.sort === 'popularity' ? badgeFor(item, { popularityIndex: i }) : badgeFor(item)}
							authed={Boolean(data.user)}
						/>
					{/each}
				</div>
				<PaginationControls
					page={meta.page ?? 1}
					{totalPages}
					onPageChange={(p) => goto(url({ page: String(p) }), { replace: true })}
				/>
			{/if}
		</main>
	</div>

	<!-- Bottom CTA strip -->
	<section class="flex flex-wrap items-center justify-between gap-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-primary)]/5 p-5">
		<div class="flex items-center gap-3">
			<span class="grid h-10 w-10 place-items-center rounded-[var(--radius)] bg-[var(--color-primary)]/10 text-[var(--color-primary-text)]">
				<Scale size={18} />
			</span>
			<div>
				<p class="font-semibold text-[var(--color-text)]">{t('jce.compare.category.create_own_title')}</p>
				<p class="text-sm text-[var(--color-muted)]">{t('jce.compare.category.create_own_body')}</p>
			</div>
		</div>
		<a href="/compare?type={memberTypes[0]?.slug}" class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2.5 text-sm font-medium text-[var(--color-primary-contrast)]">
			{t('jce.compare.category.start_custom')}
		</a>
	</section>
</div>
```

Check `<family.icon />` — dynamic component on a derived object: use `{@const FamIcon = family.icon}` then `<FamIcon />` (Svelte 5 requires a capitalized variable for dynamic components).

- [ ] **Step 3: Typecheck + route smoke**

Run: `cd apps/main && bunx svelte-check --tsconfig ./tsconfig.json | tail -20`
Expected: no errors.

Smoke: `/compare/location` renders the browse page (when `location` family exists); `/compare/<quick-slug>` still renders quick results; `/compare/not-a-category` hits the quick-result route (404 as before).

- [ ] **Step 4: Commit**

```bash
git add "apps/main/src/routes/(app)/compare/[category=compareCategory]"
git commit -m "feat(compare): family browse page /compare/[category]"
```

---

### Task 11: `/comparisons` index — trending row, facet rail, vs-cards

**Files:**
- Modify: `apps/main/src/routes/(app)/comparisons/+page.server.ts` (add `facet=types` to the list call, expose `unfiltered` flag)
- Modify: `apps/main/src/routes/(app)/comparisons/+page.svelte`

**Interfaces:**
- Consumes: `meta.facets.types`, `ComparisonVsCard`, `CompareRefineRail`, `badgeFor`.
- Produces: index page keeps `filter_type` param semantics; rail "All types" = no `filter_type`.

- [ ] **Step 1: Server — request facets**

In `+page.server.ts`, append `params.set('facet', 'types')` before the `apiList` call (facets are cheap and always useful here — the rail shows the all-types distribution since the facet query ignores `filter_type`). Also return `unfiltered: !q && !filterType && !mine` so the page knows when to show the trending row.

- [ ] **Step 2: Page — rail + trending row + vs-cards**

In `+page.svelte`:

- Import `CompareRefineRail`, `ComparisonVsCard`, `badgeFor`, `COMPARE_FAMILIES`-adjacent type map (types already loaded as `data.types` with `type_name` mapped per item — facet slug → type name via `types` list).
- Derive `railCategories`: `[{label: 'All types', count: meta.total, href: buildURL({filter_type:null,page:1}), active: !filters.filter_type}]` + `data.types.map(tp => ({label: tp.name, count: facetBySlug[tp.slug] ?? 0, href: buildURL({filter_type: tp.slug, page:1}), active: filters.filter_type===tp.slug}))`.
- Layout: wrap the existing `max-w-3xl` list column in a `flex` row: `<CompareRefineRail>` (hidden below `lg`: keep the existing Filters `Modal` for mobile) + `<main>` containing trending row + grid.
- Trending row: add a second server fetch `trendingPromise` (`sort=popularity&page_size=3`, same public filters) returned as `trendingItems`; render `ComparisonVsCard`s with `badgeFor(item, {popularityIndex: i})` — only when `data.unfiltered` (no q/filter_type/mine active).
- Replace the `<article>` card markup (~lines 306-341) with `<ComparisonVsCard summary={item} badge={badgeFor(item)} authed={Boolean(user)} />` inside the same `sm:grid-cols-2` grid — **but only when `!isMine`**. The `mine` tab keeps the existing article cards: they carry status/visibility chips and management actions the vs-card doesn't model.

- [ ] **Step 3: Typecheck + smoke**

Run: `cd apps/main && bunx svelte-check --tsconfig ./tsconfig.json | tail -20`; smoke `/comparisons` + `?filter_type=` + `?mine=true`.
Expected: no errors; facets counts show beside rail categories.

- [ ] **Step 4: Commit**

```bash
git add "apps/main/src/routes/(app)/comparisons/+page.server.ts" "apps/main/src/routes/(app)/comparisons/+page.svelte"
git commit -m "feat(comparisons): trending row, type-facet rail, vs-cards"
```

---

### Task 12: Detail pages — item images + matrix highlight

**Files:**
- Modify: `apps/main/src/routes/(app)/compare/[slug]/+page.svelte`
- Modify: `apps/main/src/routes/(app)/comparisons/[slug]/+page.svelte`

**Interfaces:**
- Consumes: `matrixBestCells` (Task 5), `flagUrl`, `ResolvedItem.image_url`, `MatrixItem.image_url`.
- Produces: none new.

- [ ] **Step 1: Item cards render image/flag**

In both detail pages' `resolvedItems` grids (`compare/[slug]` ~line 239-259, `comparisons/[slug]` ~line 258-279), add above the title:

```svelte
{#if item.image_url || flagUrl(item.metadata?.geo_code as string ?? item.metadata?.country_code as string)}
	<div class="mb-1 flex items-center gap-2">
		{#if item.image_url}
			<img src={item.image_url} alt="" class="h-8 w-8 rounded-full border border-[var(--color-border)] object-cover" />
		{:else}
			{@const flag = flagUrl(item.metadata?.geo_code as string ?? item.metadata?.country_code as string)}
			{#if flag}<img src={flag} alt="" class="h-6 w-8 rounded-sm border border-[var(--color-border)] object-cover" />{/if}
		{/if}
	</div>
{/if}
```

(`ResolvedItem.metadata.geo_code`/`country_code` are emitted by the resolvers — `score_inputs.go` puts `geo_code`/`country` into metadata; the flag util handles either.)

- [ ] **Step 2: Matrix best-cell highlight + sticky first column**

In both pages, add to script:

```ts
import { matrixBestCells } from '#lib/utils/comparison.js';
let bestCells = $derived(matrixBestCells(matrix, uniqueDims));
```

In the matrix `<td>` render, apply the winner tint:

```svelte
<td class="text-[var(--color-muted)] {bestCells.has(`${item.id}:${dim.id}`) ? 'bg-emerald-500/10 font-semibold text-emerald-600 dark:text-emerald-400' : ''}">
	{formatValue(cellFor(item, dim))}
	{#if bestCells.has(`${item.id}:${dim.id}`)}<span aria-label="best">✓</span>{/if}
</td>
```

Sticky first column: on the matrix wrapper `<div class="prose-table-wrapper">` add a style for `td:first-child, th:first-child { position: sticky; inset-inline-start: 0; background: var(--color-surface); z-index: 1; }` — implement in a `<style>` block or a small scoped class to avoid breaking the shared `prose-table-wrapper` used elsewhere:

```svelte
<style>
	.compare-matrix :is(td, th):first-child {
		position: sticky;
		inset-inline-start: 0;
		background: var(--color-surface);
		z-index: 1;
	}
</style>
```

and add `compare-matrix` class to the wrapper div on both pages.

- [ ] **Step 3: Typecheck + smoke**

Run: `cd apps/main && bunx svelte-check --tsconfig ./tsconfig.json | tail -20`; open a published comparison with images + a multi-item matrix.
Expected: no errors; winning cells tinted.

- [ ] **Step 4: Commit**

```bash
git add "apps/main/src/routes/(app)/compare/[slug]/+page.svelte" "apps/main/src/routes/(app)/comparisons/[slug]/+page.svelte"
git commit -m "feat(compare): item images + best-cell matrix highlight on detail pages"
```

---

### Task 13: Verification sweep

- [ ] **Step 1: Full test + check suite**

```bash
cd apps/main && bun run test && bunx svelte-check --tsconfig ./tsconfig.json
cd ../.. && go build ./... && go test ./internal/comparison/ ./internal/jce/
```

Expected: all green (or documented pre-existing failures only).

- [ ] **Step 2: Manual smoke matrix**

Dev server (`bunx vite dev --port 3000` + backend on :8080):

| Surface | Check |
|---|---|
| `/compare` | search panel, category cards → family pages, trending sort pills, AI card (feature-gated) |
| `/compare/location` | rail counts, sub filter, sort, pagination, custom-comparison CTAs |
| `/compare?type=x` | picker untouched: narrow panel, tray, field pool, create flow |
| `/compare?mine=true` | mine tab + promote/delete |
| `/compare/<quick-slug>` | quick result: images, matrix highlight, owner actions |
| `/comparisons` | trending row, rail, vs-cards, modal on mobile width |
| `/comparisons/<slug>` | article layout intact, matrix highlight |
| `/compare/visa-routes` (or an absent family) | graceful — matcher pass-through or family 404 |
| Dark mode + RTL spot-check | tokens only; `rtl:-scale-x-100` on directional icons |

- [ ] **Step 3: Final commit**

```bash
git add -A && git status  # should be clean or only intentional leftovers
```

---

## Notes for implementers

- **`JceSelect` signature** (used everywhere already): `options: {value, label}[]`, `value`, `onchange: (v: string) => void`, `placeholder?`, `class?`.
- **`PaginationControls`**: `page`, `totalPages`, `onPageChange`.
- **`EmptyState`**: `icon`, `title`, optional `action: {label, href}`.
- **Dynamic lucide components** need a capitalized local (`{@const I = family.icon}` → `<I />`).
- **lucide-svelte legacy typings**: cast `as unknown as Component` at seams (existing convention in `/compare/+page.svelte:570`).
- The admin JCE screens (`apps/admin`) are untouched — `filter_type` CSV is additive and `preview_items` is omitted there.
