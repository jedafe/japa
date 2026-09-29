# Wiki Directory Pages Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A wiki category may be declared a "directory" — its page lists the space's descendant wiki entities (e.g. `/wiki/nigeria/states`, `/wiki/lagos/universities`) with optional search — and `/wiki` becomes a countries-only landing.

**Architecture:** Directory mode rides the existing `wiki_type_categories.rollup` JSONB (new `"mode": "directory"` key — no schema migration). The backend resolves descendants with the same machinery as rollups (`wiki_type_parents` edges scope eligible types; `space_parent_spaces ∪ parent_id` supplies actual spaces) but lists *all* children regardless of stored field values. Category stub pages gain lazy resolution (matching how page templates already resolve lazily) so a directory needs no `wiki_pages` row and no `seed_stubs`. The frontend renders a new `WikiDirectoryPanel` (search + card grid), the tab strip falls back to the category stub slug, and `/wiki` drops the drill-down pickers for a single search + country grid.

**Tech Stack:** Go backend (`internal/wiki`, sqlc-gen'd queries), SvelteKit 2 + Svelte 5 Runes (`apps/main`), admin SPA (`apps/admin`), vitest.

**Key product decision — flat URLs:** space slugs are globally unique, so the URL scheme stays `/wiki/{space_slug}/{page_slug}`. The user's example `/wiki/nigeria/lagos/cities` is realized as `/wiki/lagos/cities`. Nested paths are a non-goal.

## Global Constraints

- No new DB tables/columns — the `rollup` JSONB column carries `mode`.
- `GET /wiki/{space}/{page}` gains `directory` (response addition only, no new routes).
- URL grammar unchanged: `/wiki/{space_slug}` and `/wiki/{space_slug}/{page_slug}`.
- `parseRollupConfig` stays fail-soft on the read path (bad config → nil); `validateRollupInput` stays strict on the admin write path (`ErrInvalidInput`).
- sqlc regeneration command is `make sqlc-gen` (runs `sqlc generate`).
- Backend tests: `go test ./internal/wiki/`. Frontend tests: `cd apps/main && bun run test` (vitest).
- Follow existing code conventions: DTO suffix on exported API shapes (`//nolint:revive` where used), `§80` spec references in comments, i18n via `t()` in main/admin UIs.
- Do not push or open PRs; commit per task.

## Design reference

- Category template model: `internal/wiki/store_types.go` (`WikiCategory.Rollup []byte`), `internal/db/queries/wiki_types.sql`.
- Rollup read path: `internal/wiki/service_hierarchy.go` (`parseRollupConfig`, `computeRollup`, `rollupTypeIDs`), attached in `internal/wiki/service.go` `GetWikiPage` (~line 600: `page.Slug == cat.Slug` gate).
- Lazy page resolution: `internal/wiki/service.go` `getLazyTemplatePage` (~line 632) — resolves `wiki_type_page_templates` only; category stubs are **not** lazily resolvable today.
- Descendant listing: `ListWikiDescendantSpaces` (sqlc) wrapped by `Store.ListDescendantSpaces` (`internal/wiki/store.go` ~line 1264).
- Search endpoint: `GET /wiki/destinations?within={slug}&level={type}&q={q}&has_wiki=false` (`internal/wiki/handlers.go` ~line 461; query `ListWikiDestinations` in `internal/db/queries/wiki.sql`). The `within` clause currently restricts ancestors to `level IN ('country','state','city')` — Task 5 relaxes it.
- Config validation: `validateRollupInput` in `internal/wiki/service_types.go` ~line 659.

---

### Task 1: `mode` on the rollup config + validation

**Files:**
- Modify: `internal/wiki/service_hierarchy.go` (~lines 49-60 `WikiRollupConfig`, ~line 108 `parseRollupConfig`)
- Modify: `internal/wiki/service_types.go` (~lines 651-684 `validRollupMetrics`, `validateRollupInput`)
- Test: `internal/wiki/service_hierarchy_test.go` (extend `TestParseRollupConfig`, `TestValidateRollupInput`)

**Interfaces:**
- Produces: `WikiRollupConfig.Mode string` (`json:"mode,omitempty"`); const `rollupModeDirectory = "directory"`; `validRollupModes` map. Later tasks dispatch on `cfg.Mode == rollupModeDirectory`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/wiki/service_hierarchy_test.go`:

```go
func TestParseRollupConfig_DirectoryMode(t *testing.T) {
	cfg := parseRollupConfig([]byte(`{"enabled":true,"mode":"directory","levels":["state"]}`))
	if cfg == nil || cfg.Mode != "directory" || len(cfg.Levels) != 1 {
		t.Fatalf("directory config misparsed: %+v", cfg)
	}
	// Aggregate stays the default mode when absent.
	cfg = parseRollupConfig([]byte(`{"enabled":true,"metrics":["min"]}`))
	if cfg == nil || cfg.Mode != "" {
		t.Fatalf("absent mode should parse as empty, got %+v", cfg)
	}
}
```

And extend `TestValidateRollupInput`:

```go
	if _, err := validateRollupInput(json.RawMessage(`{"enabled":true,"mode":"sidebar"}`)); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unknown mode: want ErrInvalidInput, got %v", err)
	}
	if _, err := validateRollupInput(json.RawMessage(`{"enabled":true,"mode":"directory","levels":["state","university"]}`)); err != nil {
		t.Fatalf("directory config rejected: %v", err)
	}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/wiki/ -run 'TestParseRollupConfig|TestValidateRollupInput' -v`
Expected: FAIL — `cfg.Mode` field does not exist (compile error).

- [ ] **Step 3: Implement**

In `internal/wiki/service_hierarchy.go`, extend `WikiRollupConfig`:

```go
type WikiRollupConfig struct {
	Enabled      bool     `json:"enabled"`
	Metrics      []string `json:"metrics"`
	Levels       []string `json:"levels"`
	SectionLimit int      `json:"section_limit,omitempty"`
	// Mode selects what the category page renders: "" or "aggregate" is the
	// §80 field-rollup; "directory" renders the descendant-space list with
	// no field requirement (§80 directory amendment).
	Mode string `json:"mode,omitempty"`
}

// rollupModeDirectory switches a category page from field aggregation to a
// plain descendant list — the "States in Nigeria" directory page.
const rollupModeDirectory = "directory"
```

In `internal/wiki/service_types.go`, next to `validRollupMetrics`:

```go
// validRollupModes is the render-mode vocabulary — "" / "aggregate" keep
// §80 field aggregation; "directory" renders the child list instead.
var validRollupModes = map[string]bool{"": true, "aggregate": true, "directory": true}
```

And inside `validateRollupInput`, after the metrics loop:

```go
	if !validRollupModes[cfg.Mode] {
		return nil, fmt.Errorf("%w: unknown rollup mode %q", ErrInvalidInput, cfg.Mode)
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/wiki/ -run 'TestParseRollupConfig|TestValidateRollupInput' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/wiki/service_hierarchy.go internal/wiki/service_types.go internal/wiki/service_hierarchy_test.go
git commit -m "wiki: add directory mode to category rollup config"
```

---

### Task 2: `ListWikiDirectoryChildren` query + store wrapper

The directory list needs `geo_code` and `page_count` per child (card subtitle + "N guides" label) — `ListWikiDescendantSpaces` returns neither, so add a sibling query rather than widening the rollup one.

**Files:**
- Modify: `internal/db/queries/wiki_types.sql` (append at end)
- Regenerate: `internal/db/gen/wiki_types.sql.go` (via `make sqlc-gen`)
- Modify: `internal/wiki/store.go` (~line 1250, near `WikiDescendant`/`ListDescendantSpaces`)

**Interfaces:**
- Produces: `Store.ListDirectoryChildren(ctx, spaceID pgtype.UUID, typeIDs []pgtype.UUID) ([]WikiDirectoryChild, error)`; `WikiDirectoryChild{ID, Slug, Name, Level, SpaceTypeID, SpaceTypeSlug, GeoCode *string, PageCount int64, Depth int32}`.

- [ ] **Step 1: Add the sqlc query**

Append to `internal/db/queries/wiki_types.sql`:

```sql
-- name: ListWikiDirectoryChildren :many
-- Directory-mode category pages: transitive descendants of a space (depth
-- ≤3) restricted to the given space types, each carrying its active wiki
-- page count. Same link semantics as ListWikiDescendantSpaces
-- (space_parent_spaces ∪ parent_id); children whose type has no enabled
-- wiki_type_configs row are excluded — the directory only lists spaces
-- that can carry a wiki.
WITH RECURSIVE descendants AS (
    SELECT kid.id, 1 AS depth
    FROM (
        SELECT sps.space_id AS id FROM space_parent_spaces sps WHERE sps.parent_space_id = @space_id
        UNION
        SELECT s.id FROM spaces s WHERE s.parent_id = @space_id
    ) kid
    UNION
    SELECT kid.id, d.depth + 1
    FROM descendants d
    JOIN LATERAL (
        SELECT sps.space_id AS id FROM space_parent_spaces sps WHERE sps.parent_space_id = d.id
        UNION
        SELECT s2.id FROM spaces s2 WHERE s2.parent_id = d.id
    ) kid ON true
    WHERE d.depth < 3
)
SELECT DISTINCT ON (s.id) s.id, s.slug, s.name, s.level, s.space_type_id,
       COALESCE(st.slug, '') AS space_type_slug, d.depth, s.geo_code,
       (SELECT COUNT(*) FROM wiki_pages wp
         WHERE wp.space_id = s.id AND wp.is_active = true
           AND wp.deleted_at IS NULL)::bigint AS page_count
FROM descendants d
JOIN spaces s ON s.id = d.id
LEFT JOIN space_types st ON st.id = s.space_type_id
JOIN wiki_type_configs wtc ON wtc.space_type_id = s.space_type_id AND wtc.enabled = true
WHERE s.deleted_at IS NULL AND s.status = 'active'
  AND (cardinality(@type_ids::uuid[]) = 0 OR s.space_type_id = ANY(@type_ids::uuid[]))
ORDER BY s.id, d.depth;
```

- [ ] **Step 2: Regenerate sqlc**

Run: `make sqlc-gen`
Expected: `internal/db/gen/wiki_types.sql.go` gains `ListWikiDirectoryChildren` with params `{SpaceID, TypeIds}` and row fields `ID, Slug, Name, Level, SpaceTypeID, SpaceTypeSlug, Depth, GeoCode, PageCount`.

- [ ] **Step 3: Add the store wrapper**

In `internal/wiki/store.go`, after `ListDescendantSpaces`. NOTE — the query deliberately joins `wiki_type_configs.enabled` (unlike `ListWikiDescendantSpaces`, which trusts its caller-supplied type IDs): a directory lists only children that can actually carry a wiki, matching `ListWikiDestinations`' base predicate. An explicit `levels` slug pointing at a non-wiki type therefore yields an empty directory rather than dead links.

```go
// WikiDirectoryChild is one row of a directory-mode category's child list —
// a descendant space with its geo code and live wiki page count.
type WikiDirectoryChild struct {
	ID            pgtype.UUID
	Slug          string
	Name          string
	Level         string
	SpaceTypeID   pgtype.UUID
	SpaceTypeSlug string
	GeoCode       *string
	PageCount     int64
	Depth         int32
}

// ListDirectoryChildren returns the space's transitive descendants (depth
// ≤3) restricted to wiki-enabled space types — the directory-mode row set.
// An empty typeIDs list returns every wiki-enabled descendant.
func (s *Store) ListDirectoryChildren(ctx context.Context, spaceID pgtype.UUID, typeIDs []pgtype.UUID) ([]WikiDirectoryChild, error) {
	if typeIDs == nil {
		typeIDs = []pgtype.UUID{}
	}

	rows, err := s.read.ListWikiDirectoryChildren(ctx, gen.ListWikiDirectoryChildrenParams{
		SpaceID: spaceID,
		TypeIds: typeIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("wiki: list directory children: %w", err)
	}

	out := make([]WikiDirectoryChild, 0, len(rows))
	for _, r := range rows {
		out = append(out, WikiDirectoryChild{
			ID: r.ID, Slug: r.Slug, Name: r.Name, Level: r.Level,
			SpaceTypeID: r.SpaceTypeID, SpaceTypeSlug: r.SpaceTypeSlug,
			GeoCode: r.GeoCode, PageCount: r.PageCount, Depth: r.Depth,
		})
	}

	return out, nil
}
```

(Confirm the generated row's `GeoCode` type — `ListWikiDestinationsRow.GeoCode` is `*string`; if sqlc emits `pgtype.Text` instead, map `r.GeoCode.String`/`Valid` accordingly.)

- [ ] **Step 4: Verify build**

Run: `go build ./internal/wiki/`
Expected: compiles clean.

- [ ] **Step 5: Commit**

```bash
git add internal/db/queries/wiki_types.sql internal/db/gen/wiki_types.sql.go internal/wiki/store.go
git commit -m "wiki: add ListWikiDirectoryChildren query for directory-mode pages"
```

---

### Task 3: `computeDirectory` + `directory` on the page DTO (materialized path)

**Files:**
- Modify: `internal/wiki/service_hierarchy.go` (after the rollup DTOs ~line 104; new fn after `computeRollup`)
- Modify: `internal/wiki/service.go` (`WikiPageDetailDTO` ~line 105; rollup attach ~lines 598-604)
- Test: `internal/wiki/service_hierarchy_test.go`

**Interfaces:**
- Consumes: `rollupModeDirectory` (Task 1), `Store.ListDirectoryChildren`, `WikiDirectoryChild` (Task 2), `s.rollupTypeIDs` (existing).
- Produces: `WikiDirectoryDTO{Levels []string, Children []WikiDirectoryChildDTO, Total int}` on `WikiPageDetailDTO.Directory *WikiDirectoryDTO`; `directoryChildRows(descendants []WikiDirectoryChild, limit int) []WikiDirectoryChildDTO` (pure); `Service.computeDirectory(ctx, spaceID, spaceTypeID pgtype.UUID, cfg *WikiRollupConfig) *WikiDirectoryDTO`; const `defaultDirectoryLimit = 200`.

- [ ] **Step 1: Write the failing tests**

Two tests — a pure one for the sort/cap mapper and a DB-backed one for the full service path. The package's integration harness (`opsTestStore`/`seedHierarchyFixture`/`seedSpaceOfType`, skips without `JC_DATABASE_URL`) already covers `computeRollup` the same way (`TestComputeRollup_IncludesTemplateGroupFields`, ~line 822).

Append to `internal/wiki/service_hierarchy_test.go`:

```go
func TestDirectoryChildRows_SortAndCap(t *testing.T) {
	mk := func(name, slug string, depth int32) WikiDirectoryChild {
		return WikiDirectoryChild{Slug: slug, Name: name, Depth: depth, Level: "state", PageCount: 2}
	}
	desc := []WikiDirectoryChild{mk("Zamfara", "zamfara", 1), mk("Ikeja", "ikeja", 2), mk("Abia", "abia", 1)}

	rows := directoryChildRows(desc, 10)
	if len(rows) != 3 || rows[0].Slug != "abia" || rows[1].Slug != "zamfara" || rows[2].Slug != "ikeja" {
		t.Fatalf("expected depth-then-name order [abia zamfara ikeja], got %+v", rows)
	}

	capped := directoryChildRows(desc, 2)
	if len(capped) != 2 || capped[1].Slug != "zamfara" {
		t.Fatalf("cap should keep the head, got %+v", capped)
	}
}

// TestComputeDirectory_ListsValuelessChildren — the core contract a rollup
// cannot satisfy: every linked descendant of the scoped type appears,
// field values or not; unlinked/unscoped spaces are excluded.
func TestComputeDirectory_ListsValuelessChildren(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	hf := seedHierarchyFixture(t, ctx, database)
	uniq := strings.ReplaceAll(genUUID(t, ctx, database), "-", "")[:8]

	mk := func(q string, args ...any) pgtype.UUID { return mustIDF(t, ctx, database, q, args...) }

	// Child wiki type (state-equivalent) and one wiki-less child space.
	childTypeID := mk(`INSERT INTO space_types (id,name,slug) VALUES (gen_random_uuid(),$1,$1) RETURNING id`, "wop-child-"+uniq)
	mk(`INSERT INTO wiki_type_configs (space_type_id,enabled,label) VALUES ($1,true,$2) RETURNING space_type_id`,
		childTypeID, "Wop Child "+uniq)

	parentSpace, _ := seedSpaceOfType(t, ctx, database, hf.typeID, "dirparent")
	childSpace, _ := seedSpaceOfType(t, ctx, database, childTypeID, "dirchild")
	if _, err := database.Writer.Exec(ctx,
		`UPDATE spaces SET parent_id = $1 WHERE id = $2`, parentSpace, childSpace); err != nil {
		t.Fatalf("link child space: %v", err)
	}
	// Second child of the fixture's own type — not in the scoped levels.
	straySpace, _ := seedSpaceOfType(t, ctx, database, hf.typeID, "dirstray")
	if _, err := database.Writer.Exec(ctx,
		`UPDATE spaces SET parent_id = $1 WHERE id = $2`, parentSpace, straySpace); err != nil {
		t.Fatalf("link stray space: %v", err)
	}

	t.Cleanup(func() {
		cctx := context.Background()
		database.Writer.Exec(cctx, `DELETE FROM wiki_type_configs WHERE space_type_id = $1`, childTypeID)
		database.Writer.Exec(cctx, `DELETE FROM space_types WHERE id = $1`, childTypeID)
	})

	svc := NewService(store, nil)
	dir := svc.computeDirectory(ctx, parentSpace, hf.typeID,
		&WikiRollupConfig{Enabled: true, Mode: rollupModeDirectory, Levels: []string{"wop-child-" + uniq}})
	if dir == nil {
		t.Fatal("directory is nil — descendants unresolved")
	}
	if dir.Total != 1 || len(dir.Children) != 1 {
		t.Fatalf("want exactly the scoped child, got %+v", dir)
	}
	c := dir.Children[0]
	if c.Slug == "" || c.TypeSlug != "wop-child-"+uniq || c.PageCount != 0 {
		t.Fatalf("child row malformed: %+v", c)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/wiki/ -run 'TestDirectoryChildRows|TestComputeDirectory' -v`
Expected: FAIL — `directoryChildRows`/`computeDirectory`/`rollupModeDirectory` don't compile yet (the DB test also skips without `JC_DATABASE_URL`).

- [ ] **Step 3: Implement**

In `internal/wiki/service_hierarchy.go`, after `WikiRollupDTO`:

```go
// WikiDirectoryChildDTO is one descendant space row on a directory-mode
// category page — the "States" tab card on /wiki/{country}.
// //nolint:revive // exported: DTO naming follows package convention
type WikiDirectoryChildDTO struct {
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	Level     string `json:"level"`
	TypeSlug  string `json:"type_slug,omitempty"`
	GeoCode   string `json:"geo_code,omitempty"`
	PageCount int64  `json:"page_count"`
}

// WikiDirectoryDTO is the read-time descendant listing attached to a
// directory-mode category page. Never persisted — recomputed per request.
// Levels echoes the config's explicit type slugs (empty = every declared
// descendant type); Total is the untruncated child count so the frontend
// can show "Showing N of M" and gate the search hint.
// //nolint:revive // exported: DTO naming follows package convention
type WikiDirectoryDTO struct {
	Levels   []string               `json:"levels"`
	Children []WikiDirectoryChildDTO `json:"children"`
	Total    int                    `json:"total"`
}

// defaultDirectoryLimit bounds the embedded children list — the full set
// stays reachable through GET /wiki/destinations?within={slug}&q={q}.
const defaultDirectoryLimit = 200

// directoryChildRows sorts descendants depth-first then by name and caps
// the emitted rows at limit — pure (no I/O) like aggregateRollupValues so
// ordering/truncation is unit-testable.
func directoryChildRows(descendants []WikiDirectoryChild, limit int) []WikiDirectoryChildDTO {
	sort.SliceStable(descendants, func(i, j int) bool {
		if descendants[i].Depth != descendants[j].Depth {
			return descendants[i].Depth < descendants[j].Depth
		}
		return descendants[i].Name < descendants[j].Name
	})

	out := make([]WikiDirectoryChildDTO, 0, min(limit, len(descendants)))
	for _, d := range descendants {
		if len(out) >= limit {
			break
		}
		row := WikiDirectoryChildDTO{
			Slug: d.Slug, Name: d.Name, Level: d.Level,
			TypeSlug: d.SpaceTypeSlug, PageCount: d.PageCount,
		}
		if d.GeoCode != nil {
			row.GeoCode = *d.GeoCode
		}
		out = append(out, row)
	}

	return out
}

// computeDirectory lists a directory-mode category's descendant spaces —
// every descendant of the scoped types, with or without stored field
// values (unlike computeRollup, which filters to value-bearing children).
// Type scope comes from cfg.Levels or the declared-edge closure via
// rollupTypeIDs; membership comes from the actual space links.
func (s *Service) computeDirectory(ctx context.Context, spaceID, spaceTypeID pgtype.UUID, cfg *WikiRollupConfig) *WikiDirectoryDTO {
	typeIDs, err := s.rollupTypeIDs(ctx, spaceTypeID, cfg.Levels)
	if err != nil {
		slog.Warn("wiki: directory: type scope failed", "error", err)
		return nil
	}
	if len(typeIDs) == 0 {
		return nil
	}

	descendants, err := s.store.ListDirectoryChildren(ctx, spaceID, typeIDs)
	if err != nil {
		slog.Warn("wiki: directory: descendants failed",
			"space_id", spaceID.String(), "error", err)
		return nil
	}
	if len(descendants) == 0 {
		return nil
	}

	limit := cfg.SectionLimit
	if limit <= 0 {
		limit = defaultDirectoryLimit
		if s.settings != nil {
			limit = s.settings.GetIntOr(ctx, "wiki.directory_default_limit", defaultDirectoryLimit)
		}
	}
	if limit <= 0 {
		limit = defaultDirectoryLimit
	}

	return &WikiDirectoryDTO{
		Levels:   cfg.Levels,
		Children: directoryChildRows(descendants, limit),
		Total:    len(descendants),
	}
}
```

In `internal/wiki/service.go`, on `WikiPageDetailDTO` next to `Rollup` (~line 105):

```go
	// Directory is the read-time descendant list when the category is in
	// directory mode (rollup.mode="directory") — nil otherwise. Mutually
	// exclusive with Rollup: the mode dispatches which one attaches.
	Directory    *WikiDirectoryDTO `json:"directory,omitempty"`
```

Then replace the rollup attach block (~line 600):

```go
		// Rollup/directory: only on the category's own page (sibling
		// template pages render their bound group's items, not the
		// aggregate/list). The config mode picks which payload attaches.
		if rcfg := parseRollupConfig(cat.Rollup); rcfg != nil && page.Slug == cat.Slug {
			if spaceTypeID, _, terr := s.store.GetSpaceTypeRef(ctx, spaceID); terr == nil {
				if rcfg.Mode == rollupModeDirectory {
					dto.Directory = s.computeDirectory(ctx, spaceID, spaceTypeID, rcfg)
				} else {
					dto.Rollup = s.computeRollup(ctx, userID, spaceID, spaceTypeID, rcfg, cat.Category)
				}
			}
		}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/wiki/ -run 'TestDirectoryChildRows|TestComputeDirectory' -v && go build ./internal/wiki/`
Expected: PASS + clean build (with a DB via `JC_DATABASE_URL`; otherwise the DB test skips and the pure test still runs).

- [ ] **Step 5: Commit**

```bash
git add internal/wiki/service_hierarchy.go internal/wiki/service_hierarchy_test.go internal/wiki/service.go
git commit -m "wiki: computeDirectory + directory payload on category pages"
```

---

### Task 4: Lazy category pages (resolution + materialization)

Today `getLazyTemplatePage` only resolves `wiki_type_page_templates` — a category stub slug (`cat.slug`) 404s unless a `wiki_pages` row was seeded (`seed_stubs` or first submission). Directory pages need the category stub itself to resolve lazily, and first-submission materialization must create the category-bound row.

Review-verified: `getLazyTemplatePage` already returns `pgx.ErrNoRows` on a template miss (service.go:635-639), so the dispatcher is a thin chain — **no signature changes to the template path**.

**Files:**
- Modify: `internal/wiki/service.go` (`GetWikiPage` NoRows branch ~line 530; new `getLazyPage`/`lazyCategoryPage` after `getLazyTemplatePage` ~line 694; `materializeTemplatePage` ~line 838)
- Modify: `internal/wiki/service_fields.go` (`GetPageEntityTarget` ~line 765 — the second `materializeTemplatePage` caller)
- Test: `internal/wiki/service_hierarchy_test.go`

**Interfaces:**
- Consumes: `Store.GetCategoryForSpace(ctx, spaceID, slugOrCategory) (WikiCategory, error)` — matches `wtc.slug = @slug OR wtc.category = @slug` (verified at `wiki_types.sql` `GetWikiCategoryForSpace`), `Store.createPageBound(ctx, q, spaceID, createdBy, slug, title, category, pageTemplateID, fieldGroupID, position)` (`pgtype.UUID{}` = no template binding), Task 3's `computeDirectory`.
- Produces: `Service.getLazyPage` (dispatcher), `Service.lazyCategoryPage` (new), `Service.materializeLazyPage` (renamed `materializeTemplatePage`, template-then-category).
- Callers of `materializeTemplatePage` to rename: `service.go` `SubmitRevision` (~line 789) and `service_fields.go` `GetPageEntityTarget` (~line 765) — exactly two, verified by grep.
- Precedence note: page templates resolve **before** category stubs (existing rule — `GetPageTemplateBySlug` runs first). A template slug colliding with a category stub slug shadows the category page; document, don't special-case.

- [ ] **Step 1: Write the failing tests**

Append to `internal/wiki/service_hierarchy_test.go`:

```go
// TestGetWikiPage_LazyCategoryStubPage — a category's stub slug resolves
// without a wiki_pages row (§80: category pages are lazy like template
// pages); a directory-mode category carries the descendant list.
func TestGetWikiPage_LazyCategoryStubPage(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	hf := seedHierarchyFixture(t, ctx, database)
	uniq := strings.ReplaceAll(genUUID(t, ctx, database), "-", "")[:8]

	mk := func(q string, args ...any) pgtype.UUID { return mustIDF(t, ctx, database, q, args...) }
	childTypeID := mk(`INSERT INTO space_types (id,name,slug) VALUES (gen_random_uuid(),$1,$1) RETURNING id`, "wop-child-"+uniq)
	mk(`INSERT INTO wiki_type_configs (space_type_id,enabled,label) VALUES ($1,true,$2) RETURNING space_type_id`,
		childTypeID, "Wop Child "+uniq)
	t.Cleanup(func() {
		cctx := context.Background()
		database.Writer.Exec(cctx, `DELETE FROM wiki_type_configs WHERE space_type_id = $1`, childTypeID)
		database.Writer.Exec(cctx, `DELETE FROM space_types WHERE id = $1`, childTypeID)
	})

	parentSpace, spaceSlug := seedSpaceOfType(t, ctx, database, hf.typeID, "dirlazy")
	childSpace, _ := seedSpaceOfType(t, ctx, database, childTypeID, "dirchild")
	if _, err := database.Writer.Exec(ctx,
		`UPDATE spaces SET parent_id = $1 WHERE id = $2`, parentSpace, childSpace); err != nil {
		t.Fatalf("link child: %v", err)
	}

	// catA ('cost-of-living') becomes a directory over the child type.
	if _, err := database.Writer.Exec(ctx,
		`UPDATE wiki_type_categories SET rollup = $1 WHERE id = $2`,
		[]byte(`{"enabled":true,"mode":"directory","levels":["wop-child-`+uniq+`"]}`), hf.catAID); err != nil {
		t.Fatalf("set directory rollup: %v", err)
	}

	svc := NewService(store, nil)
	svc.SetPermissionChecker(allowAllPerms{})
	svc.SetCustomFields(customfields.NewService(database))

	page, err := svc.GetWikiPage(ctx, pgtype.UUID{}, spaceSlug, "cost-of-living")
	if err != nil {
		t.Fatalf("lazy category page: %v", err)
	}
	if !page.Lazy || page.Directory == nil {
		t.Fatalf("want lazy directory page, got lazy=%v dir=%v", page.Lazy, page.Directory)
	}
	if page.Category != "cost_of_living" || page.CategoryLabel != "Cost of Living" {
		t.Fatalf("category identity: %+v", page.WikiPageDTO)
	}
	if len(page.Directory.Children) != 1 || page.Directory.Total != 1 {
		t.Fatalf("directory children: %+v", page.Directory)
	}
	var slug string
	if err := database.Reader.QueryRow(ctx, `SELECT slug FROM spaces WHERE id = $1`, childSpace).Scan(&slug); err != nil {
		t.Fatalf("child slug: %v", err)
	}
	if page.Directory.Children[0].Slug != slug {
		t.Fatalf("wrong child: %+v", page.Directory.Children[0])
	}

	// A plain (non-directory) category stub still resolves lazily — with
	// no directory payload — and unknown slugs still 404.
	plain, err := svc.GetWikiPage(ctx, pgtype.UUID{}, spaceSlug, "other")
	if err != nil || !plain.Lazy || plain.Directory != nil {
		t.Fatalf("plain lazy category: page=%+v err=%v", plain, err)
	}
	if _, err := svc.GetWikiPage(ctx, pgtype.UUID{}, spaceSlug, "no-such-slug"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("unknown slug: want pgx.ErrNoRows, got %v", err)
	}
}

// TestSubmitRevision_MaterializesLazyCategoryPage — first prose submission
// on a lazy category stub persists a wiki_pages row bound to the
// category's token/group with NULL page_template_id.
func TestSubmitRevision_MaterializesLazyCategoryPage(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	hf := seedHierarchyFixture(t, ctx, database)
	fx := seedOpsFixture(t, ctx, database)

	spaceID, spaceSlug := seedSpaceOfType(t, ctx, database, hf.typeID, "lazycat")
	t.Cleanup(func() {
		cctx := context.Background()
		database.Writer.Exec(cctx, `DELETE FROM wiki_revisions WHERE wiki_page_id IN (SELECT id FROM wiki_pages WHERE space_id = $1 AND slug = 'other')`, spaceID)
		database.Writer.Exec(cctx, `DELETE FROM wiki_pages WHERE space_id = $1 AND slug = 'other'`, spaceID)
	})

	svc := NewService(store, nil)
	revID, err := svc.SubmitRevision(ctx, fx.userID, spaceSlug, "other", SubmitRevisionInput{
		Content: []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Notes."}]}]}`),
	})
	if err != nil {
		t.Fatalf("submit on lazy category: %v", err)
	}
	if revID == "" {
		t.Fatal("empty revision id")
	}

	var tmplID, groupID pgtype.UUID
	var category string
	if err := database.Reader.QueryRow(ctx,
		`SELECT page_template_id, field_group_id, category FROM wiki_pages WHERE space_id = $1 AND slug = 'other'`, spaceID).
		Scan(&tmplID, &groupID, &category); err != nil {
		t.Fatalf("materialized row: %v", err)
	}
	if tmplID.Valid {
		t.Fatalf("category-stub page must not bind a template, got %v", tmplID)
	}
	if groupID != hf.groupIDs[1] || category != "other" {
		t.Fatalf("binding: group=%v want %v, category=%q want other", groupID, hf.groupIDs[1], category)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/wiki/ -run 'TestGetWikiPage_LazyCategoryStubPage|TestSubmitRevision_MaterializesLazyCategoryPage' -v`
Expected: compile OK, but the lazy-category cases fail — `getLazyTemplatePage` returns `pgx.ErrNoRows` for `cost-of-living`/`other` today (or compile error if `lazyCategoryPage` is referenced before it exists — write tests first, then implement).

- [ ] **Step 3: Implement the dispatcher + lazy category page**

In `GetWikiPage` (~line 530), replace `return s.getLazyTemplatePage(...)` with:

```go
			return s.getLazyPage(ctx, userID, spaceID, spaceSlug, spaceName, pageSlug)
```

After `getLazyTemplatePage` (keep it unchanged — it already converts a template miss to `pgx.ErrNoRows`), add:

```go
// getLazyPage resolves a slug with no wiki_pages row: first the space
// type's page templates (existing rule — a template slug colliding with
// a category stub shadows it), then the type's category stubs (§80 —
// category pages are lazy like template pages; directory-mode categories
// need no materialized row). pgx.ErrNoRows → real 404.
func (s *Service) getLazyPage(ctx context.Context, userID, spaceID pgtype.UUID, spaceSlug, spaceName, pageSlug string) (WikiPageDetailDTO, error) {
	dto, err := s.getLazyTemplatePage(ctx, userID, spaceID, spaceSlug, spaceName, pageSlug)
	if !errors.Is(err, pgx.ErrNoRows) {
		return dto, err // rendered, or a real lookup error
	}

	cat, cerr := s.store.GetCategoryForSpace(ctx, spaceID, pageSlug)
	if cerr != nil {
		if errors.Is(cerr, ErrWikiCategoryNotFound) {
			return WikiPageDetailDTO{}, pgx.ErrNoRows
		}
		return WikiPageDetailDTO{}, fmt.Errorf("wiki: lazy page: resolve category: %w", cerr)
	}

	return s.lazyCategoryPage(ctx, userID, spaceID, spaceSlug, spaceName, cat)
}

// lazyCategoryPage renders a category's stub page with no wiki_pages row —
// the directory-mode list page needs no materialization to work. Carries
// the category's label, bound group, template links, ancestors/siblings,
// and rollup/directory payload like the materialized path.
func (s *Service) lazyCategoryPage(ctx context.Context, userID, spaceID pgtype.UUID, spaceSlug, spaceName string, cat WikiCategory) (WikiPageDetailDTO, error) {
	dto := WikiPageDetailDTO{
		WikiPageDTO: WikiPageDTO{
			SpaceID:   spaceID.String(),
			Slug:      cat.Slug,
			Title:     substituteTemplateLabel(cat.Label, spaceName),
			Category:  cat.Category,
			IsActive:  true,
			SpaceSlug: spaceSlug,
			SpaceName: spaceName,
			Position:  cat.Position,
		},
		SpaceSlug: spaceSlug,
		SpaceName: spaceName,
		Lazy:      true,
	}
	dto.CategoryLabel = cat.Label

	if cat.FieldGroupID.Valid {
		dto.FieldGroupID = cat.FieldGroupID.String()
		if cat.FieldGroupSlug != nil {
			dto.FieldGroupSlug = *cat.FieldGroupSlug
		}
		if s.fields != nil {
			if grp, ferr := s.GetPageFields(ctx, userID, spaceID, cat.FieldGroupID); ferr == nil && grp != nil {
				dto.Fields = grp
			} else if ferr != nil {
				slog.Warn("wiki: lazy category fields load failed",
					"space_id", spaceID.String(), "group_id", cat.FieldGroupID.String(), "error", ferr)
			}
		}
	}

	if tmpls, terr := s.store.ListPageTemplates(ctx, cat.ID); terr == nil {
		dto.PageTemplates = pageTemplateItems(tmpls, spaceName)
	}

	if spaceTypeID, _, terr := s.store.GetSpaceTypeRef(ctx, spaceID); terr == nil {
		dto.Ancestors, dto.AncestorsAlt = s.getAncestors(ctx, spaceID, spaceTypeID)
		if rcfg := parseRollupConfig(cat.Rollup); rcfg != nil {
			if rcfg.Mode == rollupModeDirectory {
				dto.Directory = s.computeDirectory(ctx, spaceID, spaceTypeID, rcfg)
			} else {
				dto.Rollup = s.computeRollup(ctx, userID, spaceID, spaceTypeID, rcfg, cat.Category)
			}
		}
	}
	dto.Siblings = s.getSiblings(ctx, spaceID, cat.Category, cat.Slug)

	return dto, nil
}
```

- [ ] **Step 4: Extend materialization to category stubs**

Rename `materializeTemplatePage` → `materializeLazyPage` (it now covers both lazy kinds) and add the category fallback after the template miss:

```go
func (s *Service) materializeLazyPage(ctx context.Context, spaceID pgtype.UUID, spaceSlug, spaceName string, createdBy pgtype.UUID, pageSlug string) (WikiPageWithSpace, error) {
	// Deliberately no WikiPageRowExists check here (unchanged — see the
	// template branch below for the deactivation-vs-slug-reuse asymmetry).
	tmpl, err := s.store.GetPageTemplateBySlug(ctx, spaceID, pageSlug)
	if err == nil {
		title := substituteTemplateLabel(tmpl.Label, spaceName)
		return s.createLazyBoundPage(ctx, spaceID, spaceSlug, spaceName, createdBy,
			pageSlug, title, tmpl.CategoryToken, tmpl.ID, tmpl.FieldGroupID, tmpl.Position)
	}
	if !errors.Is(err, ErrWikiPageTemplateNotFound) {
		return WikiPageWithSpace{}, fmt.Errorf("wiki: materialize page: resolve template: %w", err)
	}

	// Category stub: binds the category token/group, no page_template_id.
	cat, cerr := s.store.GetCategoryForSpace(ctx, spaceID, pageSlug)
	if cerr != nil {
		if errors.Is(cerr, ErrWikiCategoryNotFound) {
			return WikiPageWithSpace{}, pgx.ErrNoRows
		}
		return WikiPageWithSpace{}, fmt.Errorf("wiki: materialize page: resolve category: %w", cerr)
	}

	title := substituteTemplateLabel(cat.Label, spaceName)
	return s.createLazyBoundPage(ctx, spaceID, spaceSlug, spaceName, createdBy,
		cat.Slug, title, cat.Category, pgtype.UUID{}, cat.FieldGroupID, cat.Position)
}

// createLazyBoundPage is the shared create-race tail of materializeLazyPage
// — ErrPageExists means a concurrent first submission won the unique index;
// re-resolve the row and return it.
func (s *Service) createLazyBoundPage(ctx context.Context, spaceID pgtype.UUID, spaceSlug, spaceName string, createdBy pgtype.UUID, slug, title, category string, tmplID, groupID pgtype.UUID, position int32) (WikiPageWithSpace, error) {
	pageID, err := s.store.createPageBound(ctx, s.store.write, spaceID, createdBy, slug, title, category, tmplID, groupID, position)
	if err != nil {
		if errors.Is(err, ErrPageExists) {
			page, rerr := s.store.GetWikiPageBySlug(ctx, spaceSlug, slug)
			if rerr != nil {
				return WikiPageWithSpace{}, pgx.ErrNoRows
			}
			return page, nil
		}
		return WikiPageWithSpace{}, fmt.Errorf("wiki: materialize page: %w", err)
	}

	return WikiPageWithSpace{
		WikiPage: WikiPage{
			ID: pageID, SpaceID: spaceID, Slug: slug, Title: title,
			Category: category, IsActive: true,
			PageTemplateID: tmplID, FieldGroupID: groupID, Position: position,
		},
		SpaceSlug: spaceSlug,
		SpaceName: spaceName,
	}, nil
}
```

Update the two callers: `SubmitRevision` (`service.go`, ~line 789) and `GetPageEntityTarget` (`service_fields.go`, ~line 765) — rename `s.materializeTemplatePage` → `s.materializeLazyPage` at both. Update any test references likewise (`grep -rn materializeTemplatePage internal/wiki/`).

- [ ] **Step 5: Run tests + build**

Run: `go build ./... && go test ./internal/wiki/ -run 'Lazy|Materializes' -v`
Expected: new tests pass; `TestGetWikiPage_LazyTemplatePage`, `TestSubmitRevision_MaterializesLazyTemplatePage`, `TestSubmitPageFieldOps_ValidatesBeforeMaterializing`, `TestGetWikiPage_LazyRespectsTombstones` still pass.

- [ ] **Step 6: Commit**

```bash
git add internal/wiki/service.go internal/wiki/service_fields.go internal/wiki/service_hierarchy_test.go
git commit -m "wiki: lazy-resolve category stub pages and materialize them on first submission"
```

---

### Task 5: `directory` marker on `/wiki/categories` + relax the `within` geo guard

The tab strip/index need to know which categories are directories; the directory page's search hits `/wiki/destinations?within=` whose ancestor predicate is currently hardcoded to geo levels — a directory under a non-geo space (e.g. a university's "faculties" page) must also resolve.

**Files:**
- Modify: `internal/wiki/handlers.go` (`wikiCategoryItem` ~line 337, `categoryItem` ~line 352)
- Modify: `internal/db/queries/wiki.sql` (`ListWikiDestinations` within clause ~lines 93-95)
- Regenerate: `internal/db/gen/wiki.sql.go` (`make sqlc-gen`)

**Interfaces:**
- Produces: `wikiCategoryItem.Directory bool json:"directory,omitempty"` — `true` iff `parseRollupConfig(c.Rollup).Mode == "directory"`. (`page_slug` already exists.) `ListWikiDestinations` unchanged signature; `within` now matches any linked ancestor, geo or not.

- [ ] **Step 1: Add the DTO field**

In `internal/wiki/handlers.go`:

```go
	// PageTemplates lists the category's declared sibling pages — lazy
	// template pages the frontend links into (§80); empty when the
	// category declares none or the request has no space context.
	PageTemplates []wikiPageTemplateItem `json:"page_templates,omitempty"`
	// Directory marks directory-mode categories — the tab strip and space
	// index link them even with no materialized pages (§80 directory
	// amendment).
	Directory bool `json:"directory,omitempty"`
```

In `categoryItem`, before `return item`:

```go
	if cfg := parseRollupConfig(c.Rollup); cfg != nil && cfg.Mode == rollupModeDirectory {
		item.Directory = true
	}
```

- [ ] **Step 2: Relax the `within` ancestor predicate**

In `internal/db/queries/wiki.sql`, `ListWikiDestinations`, replace:

```sql
        WHERE (g.slug = @within AND g.level IN ('country', 'state', 'city'))
           OR (gp.slug = @within AND gp.level IN ('country', 'state', 'city'))
           OR (ggp.slug = @within AND ggp.level IN ('country', 'state', 'city'))
```

with:

```sql
        -- Any linked ancestor qualifies (§80 directory amendment — the
        -- geo-level whitelist predates directory pages on non-geo spaces;
        -- the result side already requires an enabled wiki type).
        WHERE g.slug = @within OR gp.slug = @within OR ggp.slug = @within
```

- [ ] **Step 3: Regenerate + build**

Run: `make sqlc-gen && go build ./...`
Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add internal/wiki/handlers.go internal/db/queries/wiki.sql internal/db/gen/wiki.sql.go
git commit -m "wiki: expose directory flag on categories; unscope within= ancestor levels"
```

---

### Task 6: Frontend types + `WikiDirectoryPanel`

**Files:**
- Modify: `apps/main/src/lib/types/index.ts` (after `WikiRollup` ~line 3135; `WikiPage` ~line 3167)
- Create: `apps/main/src/lib/utils/wikiDirectory.ts`
- Test: `apps/main/src/lib/utils/wikiDirectory.test.ts`
- Create: `apps/main/src/lib/components/wiki/WikiDirectoryPanel.svelte`
- Modify: `apps/main/src/lib/i18n/en.json`, `apps/main/src/lib/i18n/fr.json`

**Interfaces:**
- Produces: `WikiDirectoryChild`, `WikiDirectory`, `WikiPage.directory?: WikiDirectory`; `groupDirectoryChildren(children: WikiDirectoryChild[]): Map<string, WikiDirectoryChild[]>`; `<WikiDirectoryPanel directory={WikiDirectory} spaceSlug={string} />`.

- [ ] **Step 1: Types**

In `apps/main/src/lib/types/index.ts`, after `WikiRollup`:

```ts
/** One child space row on a directory-mode wiki category page. */
export interface WikiDirectoryChild {
	slug: string;
	name: string;
	level: string;
	type_slug?: string;
	geo_code?: string;
	page_count: number;
}

/** §80 directory mode: the descendant-space listing attached to a
 *  directory-mode category page (rollup.mode === 'directory'). `levels`
 *  echoes the config's explicit type slugs (empty = every declared
 *  descendant type); `total` is the untruncated count. */
export interface WikiDirectory {
	levels: string[];
	children: WikiDirectoryChild[];
	total: number;
}
```

And on `WikiPage`, after `rollup?: WikiRollup;`:

```ts
	/** §80 directory mode: the descendant-space list — present only on a
	 *  directory-mode category's page (mutually exclusive with `rollup`). */
	directory?: WikiDirectory;
```

- [ ] **Step 2: Write the failing util test**

Create `apps/main/src/lib/utils/wikiDirectory.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { groupDirectoryChildren } from './wikiDirectory';
import type { WikiDirectoryChild } from '$lib/types';

const c = (slug: string, type = 'state'): WikiDirectoryChild => ({
	slug,
	name: slug,
	level: type,
	type_slug: type,
	page_count: 0
});

describe('groupDirectoryChildren', () => {
	it('groups by type_slug preserving first-seen order', () => {
		const g = groupDirectoryChildren([c('lagos'), c('harvard', 'university'), c('kano')]);
		expect([...g.keys()]).toEqual(['state', 'university']);
		expect(g.get('state')?.map((x) => x.slug)).toEqual(['lagos', 'kano']);
	});

	it('falls back to level when type_slug is absent', () => {
		const noType: WikiDirectoryChild = { slug: 'x', name: 'x', level: 'city', page_count: 0 };
		expect(groupDirectoryChildren([noType]).has('city')).toBe(true);
	});

	it('returns an empty map for no children', () => {
		expect(groupDirectoryChildren([]).size).toBe(0);
	});
});
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd apps/main && bunx vitest run src/lib/utils/wikiDirectory.test.ts`
Expected: FAIL — module does not exist.

- [ ] **Step 4: Implement the util**

Create `apps/main/src/lib/utils/wikiDirectory.ts`:

```ts
import type { WikiDirectoryChild } from '$lib/types';

/** Group directory children by type_slug (falling back to level),
 *  preserving first-seen order — a multi-level directory ("states and
 *  universities") renders one section per group. */
export function groupDirectoryChildren(children: WikiDirectoryChild[]): Map<string, WikiDirectoryChild[]> {
	const groups = new Map<string, WikiDirectoryChild[]>();
	for (const c of children) {
		const key = c.type_slug || c.level;
		const arr = groups.get(key);
		if (arr) arr.push(c);
		else groups.set(key, [c]);
	}
	return groups;
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd apps/main && bunx vitest run src/lib/utils/wikiDirectory.test.ts`
Expected: PASS.

- [ ] **Step 6: The panel component**

Create `apps/main/src/lib/components/wiki/WikiDirectoryPanel.svelte`:

```svelte
<script lang="ts">
	// §80 directory mode — the child-space list on a directory-mode
	// category page (e.g. "States" under /wiki/nigeria). Search narrows via
	// GET /wiki/destinations?within={space}&level={type}&q=; results jump
	// straight to the child's wiki.
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { MapPin } from 'lucide-svelte';
	import type { WikiDirectory, WikiDestination } from '$lib/types';
	import { groupDirectoryChildren } from '$lib/utils/wikiDirectory';

	let { directory, spaceSlug }: { directory: WikiDirectory; spaceSlug: string } = $props();

	const groups = $derived(groupDirectoryChildren(directory.children));
	const truncated = $derived(directory.total > directory.children.length);
	const levels = $derived(directory.levels ?? []);

	function pageCountLabel(count: number): string {
		return t(
			count === 1 ? 'wiki.destination_page_count_one' : 'wiki.destination_page_count',
			count === 1 ? '{count} guide' : '{count} guides'
		).replace('{count}', String(count));
	}

	// Search across each declared level (or unscoped when levels is empty —
	// within= already limits to this space's descendants).
	async function searchChildren(q: string): Promise<AutocompleteItem[]> {
		const query = q.trim();
		if (!query) return [];
		const scopes = levels.length > 0 ? levels : [''];
		const lists = await Promise.all(
			scopes.map(async (lvl) => {
				const params = new URLSearchParams({ within: spaceSlug, has_wiki: 'false', q: query });
				if (lvl) params.set('level', lvl);
				try {
					const res = await api.get<WikiDestination[]>(`/wiki/destinations?${params.toString()}`);
					return (res.data ?? []).map(
						(d): AutocompleteItem => ({ id: d.slug, label: d.name, sublabel: d.geo_code ?? '' })
					);
				} catch {
					return [] as AutocompleteItem[];
				}
			})
		);
		return lists.flat();
	}

	function onSelect(item: AutocompleteItem) {
		if (item.id) goto(`/wiki/${encodeURIComponent(item.id)}`);
	}
</script>

<section class="mt-6 space-y-4">
	<div class="w-full max-w-sm">
		<Autocomplete
			id="wiki-directory-search"
			placeholder={t('wiki.directory_search', 'Search…')}
			emptyMessage={t('common.no_results', 'No results found')}
			fetcher={searchChildren}
			selectedId={null}
			selectedLabel={null}
			{onSelect}
		/>
	</div>

	{#if directory.children.length === 0}
		<p class="text-sm text-[var(--color-muted)]">
			{t('wiki.directory_empty', 'Nothing listed here yet.')}
		</p>
	{:else}
		{#each [...groups] as [typeKey, children] (typeKey)}
			{#if groups.size > 1}
				<h3 class="text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)]">
					{t(`wiki.type_${typeKey}`, typeKey.replace(/_/g, ' '))}
				</h3>
			{/if}
			<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
				{#each children as child (child.slug)}
					<a
						href="/wiki/{encodeURIComponent(child.slug)}"
						class="flex flex-col gap-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3 transition-colors hover:border-[var(--color-primary)] hover:bg-[var(--color-surface-subtle)]"
					>
						<span class="flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
							{child.name}
							{#if child.geo_code}
								<span class="rounded bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-[10px] uppercase text-[var(--color-muted)]">{child.geo_code}</span>
							{/if}
						</span>
						<span class="flex items-center gap-1 text-xs text-[var(--color-muted)]">
							<MapPin size={11} aria-hidden="true" />
							{pageCountLabel(child.page_count)}
						</span>
					</a>
				{/each}
			</div>
		{/each}
		{#if truncated}
			<p class="text-xs text-[var(--color-muted)]">
				{t('wiki.directory_truncated', 'Showing {shown} of {total} — search to find more.')
					.replace('{shown}', String(directory.children.length))
					.replace('{total}', String(directory.total))}
			</p>
		{/if}
	{/if}
</section>
```

- [ ] **Step 7: i18n keys**

Add to `apps/main/src/lib/i18n/en.json` (and French equivalents in `fr.json`):

```json
"wiki.directory_search": "Search…",
"wiki.directory_empty": "Nothing listed here yet.",
"wiki.directory_truncated": "Showing {shown} of {total} — search to find more."
```

- [ ] **Step 8: Verify + commit**

Run: `cd apps/main && bunx vitest run && bunx svelte-check --tsconfig ./tsconfig.json` (or `bun run check` if defined)
Expected: tests pass, no new type errors.

```bash
git add apps/main/src/lib/types/index.ts apps/main/src/lib/utils/wikiDirectory.ts apps/main/src/lib/utils/wikiDirectory.test.ts apps/main/src/lib/components/wiki/WikiDirectoryPanel.svelte apps/main/src/lib/i18n/en.json apps/main/src/lib/i18n/fr.json
git commit -m "main: WikiDirectoryPanel — searchable child-space list on directory pages"
```

---

### Task 7: Mount the panel + carry `page_slug`/`directory` through the loads

**Files:**
- Modify: `apps/main/src/routes/(app)/wiki/[space_slug]/[page_slug]/+page.svelte` (~line 435, next to the rollup mount; the `ApiCategory`/`WikiApiCategory` types live in the two `+page.server.ts` files)
- Modify: `apps/main/src/routes/(app)/wiki/[space_slug]/+page.server.ts` (`WikiApiCategory` ~line 7)
- Modify: `apps/main/src/routes/(app)/wiki/[space_slug]/[page_slug]/+page.server.ts` (~line 7)
- Modify: `apps/main/src/routes/(app)/wiki/[space_slug]/[page_slug]/history/+page.server.ts` (~line 7 — its `WikiApiCategory` is the minimal `{slug, label_key, label}` shape; extend or leave — WikiTabs only needs the new fields when present, so extension is optional but consistent)
- Modify: `apps/main/src/routes/(app)/wiki/[space_slug]/+page.svelte` (`ApiCategory` ~line 39)

**Interfaces:**
- Consumes: `WikiPage.directory` (Task 6), `<WikiDirectoryPanel>` (Task 6); backend `wikiCategoryItem.directory` (Task 5). NOTE — `page_slug` is **already emitted** by `GET /wiki/categories` today (`wikiCategoryItem.PageSlug` → `json:"page_slug"`, handlers.go:339) — the frontend just doesn't declare it yet. Also confirmed: the API item's `slug` field carries the category *token* (e.g. `cost_of_living`) — `WikiTabs`'s `c.slug`/`firstPage` matching keys on it; `page_slug` is the URL slug (dash form) and is only for hrefs.

- [ ] **Step 1: Extend the local category types**

In all three `+page.server.ts` files and the space index `+page.svelte` `ApiCategory` type, extend the shape:

```ts
type WikiApiCategory = {
	slug: string;
	/** URL slug of the category's stub page — the lazy-resolvable link
	 *  target for categories with no materialized page. */
	page_slug?: string;
	label_key: string;
	label: string;
	/** §80 directory mode — the category page lists descendant spaces. */
	directory?: boolean;
	page_templates?: { slug: string; label: string; position: number }[];
};
```

(In `+page.svelte` it's the `ApiCategory` type; same additions.)

- [ ] **Step 2: Mount the panel**

In `apps/main/src/routes/(app)/wiki/[space_slug]/[page_slug]/+page.svelte`, import it next to `WikiRollupPanel`:

```ts
import WikiDirectoryPanel from '$lib/components/wiki/WikiDirectoryPanel.svelte';
```

And after the `{#if data.page.rollup}…{/if}` block (~line 436):

```svelte
					{#if data.page.directory}
						<WikiDirectoryPanel directory={data.page.directory} spaceSlug={data.spaceSlug} />
					{/if}
```

- [ ] **Step 3: Verify**

Run: `cd apps/main && bunx svelte-check --tsconfig ./tsconfig.json`
Expected: no new errors.

- [ ] **Step 4: Commit**

```bash
git add "apps/main/src/routes/(app)/wiki"
git commit -m "main: render directory panel on wiki pages; carry page_slug/directory on categories"
```

---

### Task 8: Tab strip + space-index cards for lazy categories

`WikiTabs` currently needs a materialized `firstPage` per category — a pure directory category (no `wiki_pages` row) would get no tab. Fall back to the category stub URL when the category is a directory or declares page templates; the space index's card groups get the same rule.

**Files:**
- Modify: `apps/main/src/lib/components/wiki/WikiTabs.svelte` (`WikiTabCategory` ~line 16, `tabs` derived ~line 57)
- Modify: `apps/main/src/routes/(app)/wiki/[space_slug]/+page.svelte` (`CardGroup` ~line 117, `cardGroups` ~line 122, `firstSlug` ~line 371)

**Interfaces:**
- Consumes: `category.page_slug`, `category.directory`, `category.page_templates` (Task 7 types).

- [ ] **Step 1: WikiTabs**

Extend `WikiTabCategory`:

```ts
export interface WikiTabCategory {
	slug: string;
	/** URL slug of the category's stub page — the lazy-resolvable tab
	 *  target when no materialized page exists in the category. */
	page_slug?: string;
	label_key?: string;
	label: string;
	/** §80 directory mode — always earns a tab (the stub page is the
	 *  directory listing). */
	directory?: boolean;
	page_templates?: { slug: string; label: string; position: number }[];
}
```

In the `tabs` derived, replace the per-category loop body. **Precedence fix (review):** a directory category's tab must link `page_slug` *first* — `firstPage` could return a sibling materialized page that doesn't carry the directory payload.

```ts
	for (const c of categories) {
		if (c.slug === 'overview') continue;
		const p = firstPage(c.slug);
		// §80 lazy categories: directory-mode categories and categories
		// with declared page templates resolve their stub page without a
		// wiki_pages row — link it instead of hiding the tab. Directory
		// categories link the stub page unconditionally: the stub IS the
		// listing — a sibling page would lack the directory payload.
		const href = c.directory && c.page_slug
			? `${base}/${encodeURIComponent(c.page_slug)}`
			: p
				? `${base}/${encodeURIComponent(p.slug)}`
				: (c.page_templates?.length ?? 0) > 0 && c.page_slug
					? `${base}/${encodeURIComponent(c.page_slug)}`
					: null;
		if (!href) continue;
		list.push({
			slug: c.slug,
			label: catLabel(c),
			href,
			icon: categoryIcon(c.slug)
		});
	}
```

- [ ] **Step 2: Space-index cards**

In `[space_slug]/+page.svelte` `cardGroups`, extend `CardGroup` with `page_slug` and include directory categories:

```ts
		// Lazy template/directory categories: a category with only
		// unmaterialized targets still gets a card so the links are
		// reachable.
		const cats = new Set(groups.keys());
		for (const c of apiCategories) {
			if ((c.page_templates?.length ?? 0) > 0 || c.directory) cats.add(c.slug);
		}
```

and in the group map:

```ts
			return {
				category,
				label: categoryLabel(category),
				pages,
				templates,
				page_slug: apiCategories.find((c) => c.slug === category)?.page_slug ?? ''
			};
```

Then the card link (~line 371):

```ts
					{@const firstSlug = first?.slug ?? group.templates[0]?.slug ?? group.page_slug ?? ''}
```

- [ ] **Step 3: Verify**

Run: `cd apps/main && bunx svelte-check --tsconfig ./tsconfig.json`
Expected: no new errors.

- [ ] **Step 4: Commit**

```bash
git add apps/main/src/lib/components/wiki/WikiTabs.svelte "apps/main/src/routes/(app)/wiki/[space_slug]/+page.svelte"
git commit -m "main: link lazy directory/template categories in tabs and index cards"
```

---

### Task 9: `/wiki` landing — countries-only grid + single search

Per the product call: the landing lists the root chain type (countries) only — deeper levels are reached through each space's directory tabs; a single search covers every enabled type so nothing becomes unreachable.

**Files:**
- Modify: `apps/main/src/routes/(app)/wiki/+page.svelte`
- Modify: `apps/main/src/routes/(app)/wiki/+page.server.ts`

**Interfaces:**
- Consumes: existing `data.destinations` (root-type list), `data.chain`, `data.features`. Server stops fetching `extraTypes`/`typeWikis`.

- [ ] **Step 1: Trim the loader**

In `+page.server.ts`, drop `extraTypes`/`typeWikis` (and the `wikiDrillChain` import if unused):

```ts
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);

	let destinations: WikiDestination[] = [];
	let types: WikiTypeItem[] = [];
	let chain: WikiTypeItem[] = [];
	try {
		const typesRes = await apiList<WikiTypeItem>(ctx, '/wiki/types');
		types = (typesRes.items ?? [])
			.filter((t) => t.enabled !== false)
			.sort((a, b) => a.position - b.position);
		chain = wikiDrillChain(types);

		const rootSlug = chain[0]?.space_type_slug ?? '';
		if (rootSlug) {
			const rootDest = await apiList<WikiDestination>(
				ctx,
				`/wiki/destinations?level=${encodeURIComponent(rootSlug)}&has_wiki=false`
			);
			destinations = rootDest.items ?? [];
		}
	} catch {
		/* degrade to empty list */
	}

	return { destinations, types, chain };
};
```

- [ ] **Step 2: Simplify the page**

In `+page.svelte`, **remove** every drill-down identifier: `showContinent`, `pickable`, `selectedContinent`, `picks`, `deepestIdx`, `gridType`, `sidelinedTypes`, `deepestSpace`, `browseOptions`, `browseLoading`, `extraTypes`, `typeWikis`, `allTypes`, `levelFetcher`, `onSelectLevel`, `onClearLevel`, `onSelectContinent`, `onClearContinent`, `loadBrowse`, `searchDestinations`, `allContinents`, `allContinentsInitial`, `searchTypeEntities`, `onSelectTypeEntity`, `typeSectionLabel`, `typeSearchHint`, and the `wikiPrimaryChild`/`wikiSidelinedChildren` imports + the `GraduationCap` icon import.

**Keep:** `initialDestinations`, `wikiEnabled`, `structuredEditsOn`, `chain`/`rootType`, `toAutocompleteItem`, `typeLabel`, `pageCountLabel`, `page` (`$app/state`), and imports `t`, `api`, `goto`, `Autocomplete`, `AutocompleteItem`, `EmptyState`, `BookOpen`/`Globe`/`MapPin`/`Table2`, `ListPageBar`, types `WikiDestination`/`WikiTypeItem`.

Replace the removed functions with a single all-type search:

```ts
	async function searchAll(q: string): Promise<AutocompleteItem[]> {
		const query = q.trim();
		if (!query) return [];
		try {
			const usp = new URLSearchParams({ has_wiki: 'false', q: query });
			const res = await api.get<WikiDestination[]>(`/wiki/destinations?${usp.toString()}`);
			return (res.data ?? []).map(toAutocompleteItem);
		} catch {
			return [];
		}
	}

	function onSelectDestination(item: AutocompleteItem) {
		if (item.id) goto(`/wiki/${encodeURIComponent(item.id)}`);
	}
```

Template: keep `ListPageBar`, the description, the `structuredEditsOn` note, the `!wikiEnabled` EmptyState. Replace the chooser card + `extraTypes` sections + conditional grid with:

```svelte
		<div class="mt-8 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<div class="mb-4 flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
				<Globe size={16} />
				{t('wiki.destination_label', 'Destination')}
			</div>
			<div class="w-full max-w-sm">
				<Autocomplete
					id="wiki-search"
					placeholder={t('wiki.search_hint', 'Search a destination…')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={searchAll}
					selectedId={null}
					selectedLabel={null}
					onSelect={onSelectDestination}
				/>
			</div>
		</div>

		<div class="mt-6">
			{#if initialDestinations.length === 0}
				<EmptyState
					icon={MapPin}
					title={t('wiki.no_destinations', 'No wikis yet')}
					body={t('wiki.no_destinations_body', 'No destination wikis are available right now.')}
				/>
			{:else}
				<h2 class="mb-3 text-sm font-semibold uppercase tracking-wide text-[var(--color-muted)]">
					{rootType ? typeLabel(rootType) : t('wiki.browse_destinations', 'Browse destinations')}
				</h2>
				<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
					{#each initialDestinations as dest (dest.slug)}
						<a
							href="/wiki/{encodeURIComponent(dest.slug)}"
							class="flex flex-col gap-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3 transition-colors hover:border-[var(--color-primary)] hover:bg-[var(--color-surface-subtle)]"
							data-sveltekit-preload-data="hover"
						>
							<span class="flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
								{dest.name}
								{#if dest.geo_code}
									<span class="rounded bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-[10px] uppercase text-[var(--color-muted)]">{dest.geo_code}</span>
								{/if}
							</span>
							<span class="text-xs text-[var(--color-muted)]">{pageCountLabel(dest.page_count)}</span>
						</a>
					{/each}
				</div>
			{/if}
		</div>
```

Add `wiki.search_hint` to `en.json`/`fr.json`. Drop now-unused icon imports (`GraduationCap`, `Table2` only if unused after the edit — `Table2` is used by the `structuredEditsOn` note, keep it; `BookOpen`/`MapPin` still used).

- [ ] **Step 3: Verify**

Run: `cd apps/main && bunx svelte-check --tsconfig ./tsconfig.json && bunx vitest run`
Expected: no new errors; `wikiTypeChain` tests (if any) still pass — `wikiDrillChain` stays exported for the server loader.

- [ ] **Step 4: Commit**

```bash
git add "apps/main/src/routes/(app)/wiki/+page.svelte" "apps/main/src/routes/(app)/wiki/+page.server.ts" apps/main/src/lib/i18n/en.json apps/main/src/lib/i18n/fr.json
git commit -m "main: /wiki landing shows the root type (countries) only, with a global destination search"
```

---

### Task 10: Admin UI — document the mode in the category editor

**Files:**
- Modify: `apps/admin/src/lib/components/wiki/WikiTypesAdmin.svelte` (`catFields` rollup field ~line 319; Rollup column ~line 395)
- Modify: `apps/admin/src/lib/locales/en.json`, `apps/admin/src/lib/locales/fr.json`

**Interfaces:**
- Consumes: `WikiCategoryItem.rollup` (already present as `Record<string, unknown>`).

- [ ] **Step 1: Update the help copy + column badge**

Change the `rollup_json` field's `help` to document the mode:

```ts
		help: t('app.wiki.cat_rollup_help', 'Aggregates: {"enabled":true,"metrics":["min","avg","max"],"levels":["city"]}. Directory: {"enabled":true,"mode":"directory","levels":["state"]} — the category page lists child spaces of those types (levels omitted = every declared child type).')
```

Update the Rollup column render to surface the mode:

```ts
					{
						key: 'rollup',
						label: t('app.wiki.cat_rollup_col', 'Rollup'),
						render: (v: unknown) => {
							const r = v as { enabled?: boolean; mode?: string } | null | undefined;
							if (!r?.enabled)
								return `<span class="text-xs" style="color: var(--color-muted)">—</span>`;
							if (r.mode === 'directory')
								return `<span class="text-xs font-medium" style="color: var(--color-primary)">${t('app.wiki.rollup_dir', 'directory')}</span>`;
							return `<span class="text-xs font-medium" style="color: var(--color-primary)">${t('app.wiki.rollup_on', 'on')}</span>`;
						}
					},
```

Add `app.wiki.rollup_dir` ("directory") + the new `app.wiki.cat_rollup_help` copy to both admin locale files.

- [ ] **Step 2: Verify + commit**

Run: `cd apps/admin && bunx svelte-check --tsconfig ./tsconfig.json`
Expected: clean.

```bash
git add apps/admin/src/lib/components/wiki/WikiTypesAdmin.svelte apps/admin/src/lib/locales/en.json apps/admin/src/lib/locales/fr.json
git commit -m "admin: document directory mode in the wiki category editor"
```

---

### Task 11: Spec note + end-to-end verification

**Files:**
- Modify: `.jeos/sections/80-destination-wiki.md` (append a short subsection)

- [ ] **Step 1: Document the mode**

Append to `.jeos/sections/80-destination-wiki.md`:

```markdown
### Directory-mode categories

A category's `rollup` config accepts `"mode": "directory"`. The category
page (its stub slug, e.g. `/wiki/{country}/states`) then renders the
space's descendant wiki entities instead of a field aggregate:

```json
{ "enabled": true, "mode": "directory", "levels": ["state"], "section_limit": 200 }
```

- `levels` restricts the descendant space types (omitted = the transitive
  closure of `wiki_type_parents`). Membership still comes from actual
  `space_parent_spaces`/`parent_id` links — edges scope, links supply rows.
- Unlike aggregate rollups, every descendant is listed regardless of
  stored field values; children carry `page_count` + `geo_code`.
- The page payload gains `directory: { levels, children[], total }`;
  `GET /wiki/categories?space=` marks directory categories with
  `directory: true` so the tab strip/index link them without a
  materialized `wiki_pages` row.
- Category stub pages resolve lazily (no `seed_stubs` required) and
  materialize on first prose submission — same rule as page templates.
- Search on the directory page reuses
  `GET /wiki/destinations?within={space}&level={type}&q=`; `within` now
  matches any linked ancestor (the geo-level whitelist was removed).
```

- [ ] **Step 2: Configure directories (manual, via admin UI or API)**

In the admin UI (`/community/wiki?tab=types` → a type → Categories → Add category), or via the API (admin session):

```bash
# On the "country" wiki type:
POST /api/v1/admin/wiki/types/{country_type_id}/categories
{ "slug": "states",       "label": "States",       "position": 90,
  "rollup": {"enabled": true, "mode": "directory", "levels": ["state"]} }
{ "slug": "universities", "label": "Universities", "position": 91,
  "rollup": {"enabled": true, "mode": "directory", "levels": ["university"]} }

# On the "state" wiki type: levels ["city"], plus universities.
# On the "city" wiki type: e.g. levels ["neighborhood","university"].
```

(Field group is auto-provisioned — leave `field_group_id` blank. Slugs become the URL segment: `/wiki/nigeria/states`.)

- [ ] **Step 3: End-to-end smoke**

With `wiki.enabled` on and seed spaces (nigeria + lagos + universities linked via `space_parent_spaces`/`parent_id`):

1. `GET /api/v1/wiki/nigeria/states` → 200 with `data.directory.children[]` (states, `page_count` ≥ 0, `lazy: true`).
2. `GET /api/v1/wiki/categories?space=nigeria` → the `states`/`universities` categories carry `directory: true` and `page_slug`.
3. `/wiki` → country grid + single search only.
4. `/wiki/nigeria` → "States" tab + card visible without any `wiki_pages` row; click → `/wiki/nigeria/states` renders the grid; search box filters via `within=nigeria&level=state&q=`.
5. `/wiki/lagos/universities` (a `city`-type directory) → universities linked under Lagos.
6. `POST /api/v1/wiki/nigeria/states/revisions` with a prose intro → materializes the page (lazy → real row); directory still renders.

- [ ] **Step 4: Full test sweep + commit**

Run: `go test ./internal/wiki/ && cd apps/main && bun run test && cd ../admin && bun run test`

```bash
git add .jeos/sections/80-destination-wiki.md
git commit -m "docs: document directory-mode wiki categories in section 80"
```

---

## Self-review notes

Review pass against the actual code (post-draft) — all plan code verified against `service.go`, `service_hierarchy.go`, `service_types.go`, `store_types.go`, `handlers.go`, `wiki.sql`/`wiki_types.sql`, `WikiTabs.svelte`, both space/page `+page.svelte`+`.server.ts`, the landing pair, `Autocomplete.svelte`, `types/index.ts`, `WikiTypesAdmin.svelte`, and the test harness in `service_hierarchy_test.go`/`store_ops_test.go`.

- **Coverage:** config model (T1), list query (T2), compute + attach (T3), lazy resolution + materialization (T4), categories marker + `within` fix (T5), panel (T6), page wiring (T7), tabs/cards (T8), landing (T9), admin (T10), docs + verify (T11).
- **Backward compat:** `rollup` configs without `mode` behave exactly as today (aggregate). Non-directory categories gain lazy resolution — a behavior change: a declared category stub URL now renders an empty lazy page instead of 404 (matches the §80 amendment's stated "category pages are lazy" intent; `seed_stubs` remains for index/seeded-page behavior).
- **Slug precedence:** `getLazyPage`/`materializeLazyPage` try page templates before category stubs. A template slug colliding with a category stub shadows the stub — existing deterministic rule, admins should not reuse slugs.
- **`within` broadening** (T5): `?within=` now matches spaces whose linked ancestor is any space, not only geo levels. Result rows still require an enabled wiki type, so the only new matches are children under non-geo ancestors — the intended generalization for directory search. **Known limitation (inherited):** `within`'s hops 2–3 (`gp`/`ggp`) follow `parent_id` only — a junction-only (`space_parent_spaces`, non-primary) grandchild appears in the embedded `children[]` list (full recursion over both link kinds) but can be missed by directory search. Acceptable; note if it ever matters.
- **`has_wiki` default:** `GET /wiki/destinations` defaults to `has_wiki=true` — every directory-side call must pass `has_wiki=false` (the panel and landing code do).
- **Fields on directory pages:** the bound group still attaches (`fields`, `field_group_id`, `field_group_slug`) — a directory page is a normal category page otherwise and may carry prose + structured data; the auto-provisioned group is empty unless populated.
- **Not in scope:** nested URL paths (`/wiki/nigeria/lagos/…`), server-side pagination on `GET /wiki/destinations` (client search + 200-row cap suffice for now), per-directory admin UI beyond the JSON textarea.
