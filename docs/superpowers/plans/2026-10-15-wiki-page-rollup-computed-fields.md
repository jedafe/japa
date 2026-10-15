# Wiki Page-Scope Rollup & Computed Fields Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a wiki category page aggregate its own sub-pages (`rollup.mode: "pages"`) and let fields declare `options.computed` so their values derive from source fields instead of manual entry — starting with the city `cost_of_living` category, whose topic pages are flattened to direct children.

**Architecture:** Read-time aggregation only — nothing is persisted. The page rollup reuses `aggregateRollupValues` with the row axis swapped from descendant spaces to page templates; computed fields are a per-field `options.computed` spec resolved against the entity's already-loaded cfv rows in `GetPageFields`/`GetEntityFields`. The ops validator and applier both reject `set_value` on computed fields.

**Tech Stack:** Go (internal/wiki, internal/customfields, sqlc gen types), SvelteKit 2 + Svelte 5 runes (apps/main), Svelte SPA admin (apps/admin), PostgreSQL golang-migrate.

**Spec:** `docs/superpowers/specs/2026-10-15-wiki-page-rollup-computed-fields-design.md`

## Global Constraints

- Follow existing conventions in each file — the wiki package uses `// §80`-style spec references in comments.
- Fail closed on field-level RBAC: a caller without `can_view` on a source field contributes nothing to aggregates or computed values.
- Never emit NaN/Inf into DTOs — every folded scalar passes the existing `finite()` guard.
- Do not modify the shared `aggregateRollupValues` signature — the page rollup reuses it as-is.
- `customfields` parsers are nil-on-malformed, matching `CurrencyCodeFromOptions`/`PrimaryPartFromOptions`.
- Commit style: `feat(wiki): …` / `fix(wiki): …` with the Devin trailer:
  ```
  Generated with [Devin](https://devin.ai)

  Co-Authored-By: Devin <158243242+devin-ai-integration[bot]@users.noreply.github.com>
  ```
- Verify with `go build ./internal/wiki/ ./internal/customfields/` and `go test ./internal/wiki/ ./internal/customfields/`; frontend with `cd apps/main && bun run check` (or `npx svelte-check --tsconfig ./tsconfig.json`), admin `cd apps/admin && bun run check`.

---

### Task 1: `customfields` computed-field spec parser

**Files:**
- Modify: `internal/customfields/customfields.go` (append near `CurrencyCodeFromOptions`, ~line 176)
- Test: `internal/customfields/computed_test.go` (create)

**Interfaces:**
- Produces:
  - `type ComputedFieldSource struct { Group string; Key string }`
  - `type ComputedFieldSpec struct { Metric string; Sources []ComputedFieldSource }`
  - `func ComputedSpecFromOptions(options []byte) *ComputedFieldSpec` — nil on absent/malformed/invalid; empty `metric` defaults to `"avg"`; valid metrics are `min`, `avg`, `max`, `sum`.

- [ ] **Step 1: Write the failing test**

```go
package customfields

import "testing"

func TestComputedSpecFromOptions(t *testing.T) {
	if ComputedSpecFromOptions(nil) != nil {
		t.Fatal("nil options should parse to nil")
	}
	if ComputedSpecFromOptions([]byte(`{"currency_code":"USD"}`)) != nil {
		t.Fatal("options without computed should parse to nil")
	}
	if ComputedSpecFromOptions([]byte(`{"computed":{"metric":"bogus","sources":[{"group":"g","key":"k"}]}}`)) != nil {
		t.Fatal("invalid metric should parse to nil")
	}
	if ComputedSpecFromOptions([]byte(`{"computed":{"sources":[]}}`)) != nil {
		t.Fatal("empty sources should parse to nil")
	}
	if ComputedSpecFromOptions([]byte(`{"computed":{"sources":[{"group":"","key":"k"}]}}`)) != nil {
		t.Fatal("source missing group should parse to nil")
	}

	spec := ComputedSpecFromOptions([]byte(
		`{"computed":{"sources":[{"group":"wiki_city_prices_food_groceries","key":"monthly_total"},{"group":"wiki_city_housing","key":"rent_1br"}]}}`))
	if spec == nil {
		t.Fatal("valid spec misparsed")
	}
	if spec.Metric != "avg" {
		t.Fatalf("empty metric should default to avg, got %q", spec.Metric)
	}
	if len(spec.Sources) != 2 || spec.Sources[0].Group != "wiki_city_prices_food_groceries" || spec.Sources[0].Key != "monthly_total" {
		t.Fatalf("sources misparsed: %+v", spec.Sources)
	}

	spec = ComputedSpecFromOptions([]byte(`{"computed":{"metric":"sum","sources":[{"group":"g","key":"k"}]}}`))
	if spec == nil || spec.Metric != "sum" {
		t.Fatalf("sum metric misparsed: %+v", spec)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/customfields/ -run TestComputedSpecFromOptions -v`
Expected: FAIL — `ComputedSpecFromOptions` undefined.

- [ ] **Step 3: Implement the spec + parser**

Append to `internal/customfields/customfields.go` (after `CurrencyCodeFromOptions`):

```go
// ComputedFieldSource is one source field in a computed spec — a group
// slug + field key pair whose stored cfv contributes one scalar to the
// aggregate (§80 page rollup amendment).
type ComputedFieldSource struct {
	Group string `json:"group"`
	Key   string `json:"key"`
}

// ComputedFieldSpec is the parsed shape of a field's options.computed:
// the field's displayed value is the metric fold of its sources' stored
// values, never a manual cfv write. Read-time only — the stored cfv row
// (if any) is ignored for display while the spec is set.
type ComputedFieldSpec struct {
	Metric  string                `json:"metric"`
	Sources []ComputedFieldSource `json:"sources"`
}

// validComputedMetrics is the fold vocabulary an options.computed spec
// may request; empty metric defaults to "avg".
var validComputedMetrics = map[string]bool{"min": true, "avg": true, "max": true, "sum": true}

// ComputedSpecFromOptions decodes a field's options.computed — nil on
// absent, malformed, metric-invalid, or sourceless specs (computed is
// strictly opt-in).
func ComputedSpecFromOptions(options []byte) *ComputedFieldSpec {
	if len(options) == 0 {
		return nil
	}
	var o struct {
		Computed *ComputedFieldSpec `json:"computed"`
	}
	if err := json.Unmarshal(options, &o); err != nil || o.Computed == nil || len(o.Computed.Sources) == 0 {
		return nil
	}
	spec := o.Computed
	if spec.Metric == "" {
		spec.Metric = "avg"
	}
	if !validComputedMetrics[spec.Metric] {
		return nil
	}
	for _, s := range spec.Sources {
		if s.Group == "" || s.Key == "" {
			return nil
		}
	}
	return spec
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/customfields/ -run TestComputedSpecFromOptions -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/customfields/customfields.go internal/customfields/computed_test.go
git commit -m "feat(customfields): options.computed spec parser (§80 page rollup)"
```

---

### Task 2: Page-mode rollup — config, DTOs, `computePageRollup`, wiring

**Files:**
- Modify: `internal/wiki/service_hierarchy.go` (config struct ~line 62, mode const ~line 74, DTOs ~lines 117-129, new function after `computeRollup` ~line 1098)
- Modify: `internal/wiki/service_types.go` (`validateRollupInput` ~line 791)
- Modify: `internal/wiki/service.go` (rollup block ~lines 903-911, `lazyCategoryPage` ~lines 1109-1118)
- Modify: `internal/wiki/service_ops_test.go` (`fakeFieldReader` ~line 113 — `ListFieldValuesWithDefsForEntities` must return fixture values)
- Test: `internal/wiki/service_hierarchy_test.go` (append)

**Interfaces:**
- Consumes: `WikiCategory` (store_types.go), `WikiPageTemplate` (store_types.go:609), `rollupFieldDef`, `rollupFieldTypes`, `aggregateRollupValues`, `SectionQuickFacts`, `substituteTemplateLabel`, `customfields.FieldValueFromDefRow(v.ValueText, v.ValueNumber, v.ValueBool, v.ValueDate, v.ValueJson, v.FieldType)`.
- Produces:
  - `const rollupModePages = "pages"`, `const rollupScopePages = "pages"`
  - `WikiRollupConfig.Pages []string` (json `pages,omitempty`)
  - `type WikiRollupPageRefDTO struct { Slug, Title string }`
  - `WikiRollupRowDTO.Page *WikiRollupPageRefDTO` (json `page,omitempty`)
  - `WikiRollupDTO.Scope string` (json `scope,omitempty`)
  - `func (s *Service) computePageRollup(ctx context.Context, userID, spaceID pgtype.UUID, spaceSlug, spaceName string, cfg *WikiRollupConfig, templates []WikiPageTemplate) *WikiRollupDTO`

- [ ] **Step 1: Extend `fakeFieldReader` so page-rollup tests can feed values**

In `internal/wiki/service_ops_test.go`, add a field to the struct (after `values`):

```go
	valuesForEntities  []gen.ListCustomFieldValuesWithDefsForEntitiesRow
```

and change `ListFieldValuesWithDefsForEntities` to:

```go
func (f *fakeFieldReader) ListFieldValuesWithDefsForEntities(_ context.Context, _ string, _ []pgtype.UUID) ([]gen.ListCustomFieldValuesWithDefsForEntitiesRow, error) {
	return f.valuesForEntities, nil
}
```

- [ ] **Step 2: Write the failing tests**

Append to `internal/wiki/service_hierarchy_test.go`:

```go
// ---------------------------------------------------------------------------
// computePageRollup — page-scope aggregation (§80 page rollup amendment).
// Rows are the category's page templates; values are the host space's cfv
// in each page's bound group.
// ---------------------------------------------------------------------------

func pgid(b byte) pgtype.UUID { return pgtype.UUID{Valid: true, Bytes: [16]byte{b}} }

func groupField(id, groupID pgtype.UUID, key, label, typ string) gen.ListCustomFieldsByGroupRow {
	return gen.ListCustomFieldsByGroupRow{
		ID: id, GroupID: groupID, Key: key, Label: label,
		FieldType: typ, ShowInFrontend: true,
	}
}

func TestComputePageRollup_Basic(t *testing.T) {
	spaceID := pgid(0xEE)
	groupA, groupB := pgid(0xA0), pgid(0xB0)
	fieldA, fieldB := pgid(0xA1), pgid(0xB1)

	ff := &fakeFieldReader{
		groups: map[string]gen.CustomFieldGroup{
			groupA.String(): {ID: groupA, Slug: "wiki_city_prices_food_groceries"},
			groupB.String(): {ID: groupB, Slug: "wiki_city_prices_transportation"},
		},
		groupFields: map[string][]gen.ListCustomFieldsByGroupRow{
			groupA.String(): {groupField(fieldA, groupA, "monthly_total", "Monthly total", customfields.FieldTypeCurrency)},
			groupB.String(): {groupField(fieldB, groupB, "monthly_total", "Monthly total", customfields.FieldTypeCurrency)},
		},
		valuesForEntities: []gen.ListCustomFieldValuesWithDefsForEntitiesRow{
			{FieldID: fieldA, EntityID: spaceID, FieldType: customfields.FieldTypeCurrency, ValueNumber: numericVal(t, "100"), FieldGroupID: groupA},
			{FieldID: fieldB, EntityID: spaceID, FieldType: customfields.FieldTypeCurrency, ValueNumber: numericVal(t, "200"), FieldGroupID: groupB},
		},
		perms: map[string][2]bool{fieldA.String(): {true, true}, fieldB.String(): {true, true}},
	}
	s := &Service{fields: ff, perms: fakePermChecker{granted: map[string]bool{}}}

	templates := []WikiPageTemplate{
		{ID: pgid(0x01), Slug: "food-groceries-prices", Label: "Food & Groceries Prices in {name}", FieldGroupID: groupA, Position: 1},
		{ID: pgid(0x02), Slug: "transportation-prices", Label: "Transportation Prices in {name}", FieldGroupID: groupB, Position: 2},
	}

	out := s.computePageRollup(context.Background(), pgid(0x01), spaceID, "toronto", "Toronto",
		&WikiRollupConfig{Enabled: true, Mode: rollupModePages, Metrics: []string{"avg"}}, templates)
	if out == nil {
		t.Fatal("rollup should resolve")
	}
	if out.Scope != rollupScopePages {
		t.Fatalf("scope: got %q", out.Scope)
	}
	f := fieldByKey(t, out.Fields, "monthly_total")
	if f.Samples != 2 || f.Metrics.Avg == nil || *f.Metrics.Avg != 150 {
		t.Fatalf("avg across pages wrong: %+v", f)
	}
	if len(out.Rows) != 2 {
		t.Fatalf("rows: got %d want 2", len(out.Rows))
	}
	if out.Rows[0].Page == nil || out.Rows[0].Page.Slug != "food-groceries-prices" {
		t.Fatalf("row 0 page ref wrong: %+v", out.Rows[0])
	}
	if out.Rows[0].Values["monthly_total"] == nil || out.Rows[1].Values["monthly_total"] == nil {
		t.Fatalf("row values missing: %+v", out.Rows)
	}
}

func TestComputePageRollup_ConfiguredSubsetAndEmptyRow(t *testing.T) {
	spaceID := pgid(0xEE)
	groupA, groupB := pgid(0xA0), pgid(0xB0)
	fieldA := pgid(0xA1)

	ff := &fakeFieldReader{
		groupFields: map[string][]gen.ListCustomFieldsByGroupRow{
			groupA.String(): {groupField(fieldA, groupA, "monthly_total", "Monthly total", customfields.FieldTypeCurrency)},
		},
		valuesForEntities: []gen.ListCustomFieldValuesWithDefsForEntitiesRow{
			{FieldID: fieldA, EntityID: spaceID, FieldType: customfields.FieldTypeCurrency, ValueNumber: numericVal(t, "100"), FieldGroupID: groupA},
		},
		perms: map[string][2]bool{fieldA.String(): {true, true}},
	}
	s := &Service{fields: ff, perms: fakePermChecker{granted: map[string]bool{}}}

	templates := []WikiPageTemplate{
		{ID: pgid(0x01), Slug: "food", Label: "Food in {name}", FieldGroupID: groupA, Position: 1},
		{ID: pgid(0x02), Slug: "transport", Label: "Transport in {name}", FieldGroupID: groupB, Position: 2},
	}

	// Unknown slug skipped; value-less configured page still emits a row.
	out := s.computePageRollup(context.Background(), pgid(0x01), spaceID, "toronto", "Toronto",
		&WikiRollupConfig{Enabled: true, Mode: rollupModePages, Pages: []string{"food", "missing", "transport"}}, templates)
	if out == nil {
		t.Fatal("rollup should resolve")
	}
	if len(out.Rows) != 2 {
		t.Fatalf("rows: got %d want 2 (unknown slug skipped)", len(out.Rows))
	}
	if out.Rows[0].Page.Slug != "food" || out.Rows[1].Page.Slug != "transport" {
		t.Fatalf("config order not preserved: %+v", out.Rows)
	}
	if len(out.Rows[1].Values) != 0 {
		t.Fatalf("empty page row should carry no values: %+v", out.Rows[1])
	}
}

func TestComputePageRollup_QuickFactsExcluded(t *testing.T) {
	spaceID := pgid(0xEE)
	groupA := pgid(0xA0)
	fieldA, fieldQF := pgid(0xA1), pgid(0xA2)

	qf := "quick_facts"
	row := groupField(fieldA, groupA, "monthly_total", "Monthly total", customfields.FieldTypeCurrency)
	qfRow := groupField(fieldQF, groupA, "pop", "Population", customfields.FieldTypeNumber)
	qfRow.Section = &qf

	ff := &fakeFieldReader{
		groupFields: map[string][]gen.ListCustomFieldsByGroupRow{groupA.String(): {row, qfRow}},
		valuesForEntities: []gen.ListCustomFieldValuesWithDefsForEntitiesRow{
			{FieldID: fieldA, EntityID: spaceID, FieldType: customfields.FieldTypeCurrency, ValueNumber: numericVal(t, "100")},
			{FieldID: fieldQF, EntityID: spaceID, FieldType: customfields.FieldTypeNumber, ValueNumber: numericVal(t, "999")},
		},
		perms: map[string][2]bool{fieldA.String(): {true, true}, fieldQF.String(): {true, true}},
	}
	s := &Service{fields: ff, perms: fakePermChecker{granted: map[string]bool{}}}

	out := s.computePageRollup(context.Background(), pgid(0x01), spaceID, "x", "X",
		&WikiRollupConfig{Enabled: true, Mode: rollupModePages},
		[]WikiPageTemplate{{Slug: "food", Label: "Food", FieldGroupID: groupA}})

	f := fieldByKey(t, out.Fields, "monthly_total")
	if *f.Metrics.Avg != 100 {
		t.Fatalf("avg wrong: %+v", f)
	}
	for _, fo := range out.Fields {
		if fo.Key == "pop" {
			t.Fatal("quick_facts field must not aggregate")
		}
	}
}

func TestValidateRollupInput_PagesMode(t *testing.T) {
	raw, err := validateRollupInput(json.RawMessage(
		`{"enabled":true,"mode":"pages","metrics":["avg"],"pages":["food-groceries-prices"]}`))
	if err != nil || raw == nil {
		t.Fatalf("pages mode should validate: %v", err)
	}
	if _, err := validateRollupInput(json.RawMessage(`{"enabled":true,"mode":"bogus"}`)); err == nil {
		t.Fatal("unknown mode must be rejected")
	}
	if _, err := validateRollupInput(json.RawMessage(`{"enabled":true,"mode":"pages","pages":["BAD SLUG"]}`)); err == nil {
		t.Fatal("invalid page slug must be rejected")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/wiki/ -run 'TestComputePageRollup|TestValidateRollupInput_PagesMode' -v`
Expected: FAIL — `computePageRollup`/`rollupModePages` undefined.

- [ ] **Step 4: Implement**

In `service_hierarchy.go`:

1. After `rollupModeDirectory` const, add:

```go
// rollupModePages switches a category rollup from descendant-space
// aggregation to same-space sub-page aggregation: rows are the
// category's configured page templates, values the space's cfv in each
// page's bound group (§80 page rollup amendment).
const rollupModePages = "pages"

// rollupScopePages marks WikiRollupDTO.Scope for page-mode payloads —
// the frontend picks row labels/links off it.
const rollupScopePages = "pages"
```

2. `WikiRollupConfig` gains:

```go
	Pages        []string `json:"pages,omitempty"`
```

3. `WikiRollupDTO` gains `Scope string `json:"scope,omitempty"``; `WikiRollupRowDTO` gains `Page *WikiRollupPageRefDTO `json:"page,omitempty"``; new DTO:

```go
// WikiRollupPageRefDTO identifies a sub-page rollup row — the
// /wiki/{space}/{slug} link target plus its display title
// (section_label when set, else the {name}-substituted label).
// //nolint:revive // exported: DTO naming follows package convention
type WikiRollupPageRefDTO struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
}
```

4. Append `computePageRollup` after `computeRollup`:

```go
// computePageRollup aggregates a category's own sub-pages (mode
// "pages"): rows are the configured page templates — or every template
// under the category when cfg.Pages is empty — and values are the host
// space's cfv in each page's bound group. Field defs, metric defaults,
// the can_view gate, and the aggregation fold are identical to
// computeRollup; only the row axis changes.
func (s *Service) computePageRollup(ctx context.Context, userID, spaceID pgtype.UUID, spaceSlug, spaceName string, cfg *WikiRollupConfig, templates []WikiPageTemplate) *WikiRollupDTO {
	if s.fields == nil {
		return nil
	}

	sources := make([]WikiPageTemplate, 0, len(templates))
	if len(cfg.Pages) > 0 {
		bySlug := make(map[string]WikiPageTemplate, len(templates))
		for _, t := range templates {
			bySlug[t.Slug] = t
		}
		for _, slug := range cfg.Pages {
			if t, ok := bySlug[slug]; ok {
				sources = append(sources, t)
			} else {
				slog.Warn("wiki: page rollup: unknown template slug", "slug", slug)
			}
		}
	} else {
		sources = templates
	}
	if len(sources) == 0 {
		return nil
	}

	canManage := s.perms != nil && s.perms.HasPermission(ctx, userID, PermFieldsManage)
	visibleSet := s.visibleFieldIDs(ctx, userID, canManage)
	fieldVisible := func(id pgtype.UUID) bool {
		if canManage {
			return true
		}
		return visibleSet != nil && visibleSet[id.String()]
	}

	defsByID := map[pgtype.UUID]rollupFieldDef{}
	defsByKey := map[string]rollupFieldDef{}
	fieldOrder := []string{}
	fieldSource := map[pgtype.UUID]int{} // field id → index into sources
	groupSeen := map[pgtype.UUID]bool{}

	for i, t := range sources {
		if !t.FieldGroupID.Valid || groupSeen[t.FieldGroupID] {
			continue // two templates sharing a group → fields attach to the first
		}
		groupSeen[t.FieldGroupID] = true

		fields, err := s.fields.ListFieldsByGroup(ctx, t.FieldGroupID)
		if err != nil {
			slog.Warn("wiki: page rollup: fields failed", "group_id", t.FieldGroupID.String(), "error", err)
			continue
		}
		for _, f := range fields {
			if !rollupFieldTypes[f.FieldType] {
				continue
			}
			if f.Section != nil && *f.Section == SectionQuickFacts {
				continue
			}
			if !fieldVisible(f.ID) {
				continue
			}

			def := rollupFieldDef{key: f.Key, label: f.Label, typ: f.FieldType, pos: f.Position}
			if f.Section != nil {
				def.section = *f.Section
			}
			def.currency = customfields.CurrencyCodeFromOptions(f.Options)
			if def.typ == customfields.FieldTypePriceParts {
				def.primaryPart = customfields.PrimaryPartFromOptions(f.Options)
				if pp := customfields.PricePartsOptionsFromJSON(f.Options); pp != nil {
					def.primaryKind = pp.PricePartKind(def.primaryPart)
					if pp.CurrencyCode != "" {
						def.currency = pp.CurrencyCode
					}
				}
			}
			defsByID[f.ID] = def
			fieldSource[f.ID] = i
			if _, seen := defsByKey[f.Key]; !seen {
				defsByKey[f.Key] = def
				fieldOrder = append(fieldOrder, f.Key)
			}
		}
	}
	if len(defsByID) == 0 {
		return nil
	}

	vals, err := s.fields.ListFieldValuesWithDefsForEntities(ctx, "space", []pgtype.UUID{spaceID})
	if err != nil {
		slog.Warn("wiki: page rollup: values failed", "space_id", spaceID.String(), "error", err)
		return nil
	}

	metrics := map[string]bool{}
	for _, m := range cfg.Metrics {
		metrics[m] = true
	}
	if len(metrics) == 0 {
		metrics = map[string]bool{"min": true, "avg": true, "max": true}
	}

	fieldsOut, _ := aggregateRollupValues(vals, defsByID, defsByKey, fieldOrder, metrics)

	// Row values bucket by the field's source template — the entity axis
	// is the single host space, so the fold's rowsByEntity map cannot
	// split pages; fieldSource carries the per-field ownership.
	pageValues := make([]map[string]any, len(sources))
	for _, v := range vals {
		def, ok := defsByID[v.FieldID]
		if !ok {
			continue
		}
		i := fieldSource[v.FieldID]
		if pageValues[i] == nil {
			pageValues[i] = map[string]any{}
		}
		pageValues[i][def.key] = customfields.FieldValueFromDefRow(
			v.ValueText, v.ValueNumber, v.ValueBool, v.ValueDate, v.ValueJson, v.FieldType)
	}

	// Every resolved template emits a row — a page with no values still
	// renders its drill-down link with '—' cells. Spaces counts the pages
	// that contributed at least one value.
	contrib := 0
	rows := make([]WikiRollupRowDTO, 0, len(sources))
	for i, t := range sources {
		title := substituteTemplateLabel(t.Label, spaceName)
		if t.SectionLabel != nil && strings.TrimSpace(*t.SectionLabel) != "" {
			title = *t.SectionLabel
		}
		pv := pageValues[i]
		if pv == nil {
			pv = map[string]any{}
		} else {
			contrib++
		}
		rows = append(rows, WikiRollupRowDTO{
			Space:  WikiSpaceRefDTO{Slug: spaceSlug, Name: spaceName},
			Page:   &WikiRollupPageRefDTO{Slug: t.Slug, Title: title},
			Values: pv,
		})
	}

	return &WikiRollupDTO{Scope: rollupScopePages, Fields: fieldsOut, Rows: rows, Spaces: contrib}
}
```

5. In `service_types.go` `validateRollupInput`, replace the mode check with:

```go
	// mode selects the renderer: empty = fold-up aggregation over
	// descendant spaces (default), "directory" = child-space list,
	// "pages" = same-space sub-page aggregation. Anything else is a typo
	// the frontend would silently ignore — reject it.
	if cfg.Mode != "" && cfg.Mode != rollupModeDirectory && cfg.Mode != rollupModePages {
		return nil, fmt.Errorf("%w: unknown rollup mode %q", ErrInvalidInput, cfg.Mode)
	}
```

and after the `Levels` loop add:

```go
	for _, p := range cfg.Pages {
		if !wikiSlugPattern.MatchString(p) {
			return nil, fmt.Errorf("%w: invalid rollup page %q", ErrInvalidInput, p)
		}
	}
```

6. In `service.go` `GetWikiPage`, replace the rollup block (currently `if rcfg := parseRollupConfig(cat.Rollup); rcfg != nil && page.Slug == cat.Slug { if spaceTypeID, ... }`) with:

```go
		// Rollup/directory: only on the category's own page (sibling
		// template pages render their bound group's items, not the
		// aggregate or child list). mode "pages" aggregates the
		// category's sub-pages in this space — no descendant lookup.
		if rcfg := parseRollupConfig(cat.Rollup); rcfg != nil && page.Slug == cat.Slug {
			if rcfg.Mode == rollupModePages {
				dto.Rollup = s.computePageRollup(ctx, userID, spaceID, page.SpaceSlug, page.SpaceName, rcfg, templates)
			} else if spaceTypeID, _, terr := s.store.GetSpaceTypeRef(ctx, spaceID); terr == nil {
				if rcfg.Mode == rollupModeDirectory {
					dto.Directory = s.computeDirectory(ctx, spaceID, spaceTypeID, rcfg)
				} else {
					dto.Rollup = s.computeRollup(ctx, userID, spaceID, spaceTypeID, rcfg, cat.Category)
				}
			}
		}
```

(Verify the `WikiPageWithSpace`/`page` struct field names for slug/name — `page.SpaceSlug`/`page.SpaceName` are used elsewhere in `GetWikiPage`.)

7. In `lazyCategoryPage`, the rollup block currently reads `if spaceTypeID, _, terr := s.store.GetSpaceTypeRef(ctx, spaceID); terr == nil { dto.Ancestors, ...; if rcfg := ... }`. Restructure so pages mode doesn't need the type ref:

```go
	if rcfg := parseRollupConfig(cat.Rollup); rcfg != nil && rcfg.Mode == rollupModePages {
		dto.Rollup = s.computePageRollup(ctx, userID, spaceID, spaceSlug, spaceName, rcfg, templates)
	}
	if spaceTypeID, _, terr := s.store.GetSpaceTypeRef(ctx, spaceID); terr == nil {
		dto.Ancestors, dto.AncestorsAlt = s.getAncestors(ctx, spaceID, spaceTypeID)
		if rcfg := parseRollupConfig(cat.Rollup); rcfg != nil {
			if rcfg.Mode == rollupModeDirectory {
				dto.Directory = s.computeDirectory(ctx, spaceID, spaceTypeID, rcfg)
			} else if rcfg.Mode != rollupModePages {
				dto.Rollup = s.computeRollup(ctx, userID, spaceID, spaceTypeID, rcfg, cat.Category)
			}
		}
	}
```

- [ ] **Step 5: Run tests**

Run: `go build ./internal/wiki/ && go test ./internal/wiki/ -run 'TestComputePageRollup|TestValidateRollupInput|TestParseRollupConfig' -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/wiki/service_hierarchy.go internal/wiki/service_types.go internal/wiki/service.go internal/wiki/service_ops_test.go internal/wiki/service_hierarchy_test.go
git commit -m "feat(wiki): page-scope rollup — category pages aggregate their sub-pages (§80)"
```

---

### Task 3: Computed fields — read path

**Files:**
- Create: `internal/wiki/service_computed.go`
- Modify: `internal/wiki/service_fields.go` (`EntityFieldDTO` ~line 171; call sites ~line 583 in `GetEntityFields` and ~line 772 in `GetPageFields`)
- Test: `internal/wiki/service_computed_test.go` (create)

**Interfaces:**
- Consumes: `customfields.ComputedSpecFromOptions` (Task 1), `cfvNumeric`, `finite`, `avgOf`, `minOf`, `maxOf` (service_hierarchy.go), `customfields.PrimaryPartFromOptions`, `customfields.FieldTypePriceRange`/`FieldTypePriceParts`.
- Produces:
  - `EntityFieldDTO.Computed bool` (json `computed,omitempty`)
  - `func (s *Service) applyComputedFields(ctx context.Context, dtos []EntityFieldDTO, values []gen.ListCustomFieldValuesWithDefsByEntityRow, visibleSet map[string]bool, canManage bool)`
  - `func cfvSourceScalar(v gen.ListCustomFieldValuesWithDefsByEntityRow) (float64, bool)`
  - `func foldComputed(metric string, scalars []float64) float64`
  - `func computedValueJSON(fieldType string, options []byte, v float64) json.RawMessage`

- [ ] **Step 1: Write the failing test**

Create `internal/wiki/service_computed_test.go`:

```go
package wiki

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/customfields"
	"github.com/jedafe/japa/internal/db/gen"
)

func entityVal(fieldID, groupID pgtype.UUID, key, fieldType string, num pgtype.Numeric, jsn []byte) gen.ListCustomFieldValuesWithDefsByEntityRow {
	return gen.ListCustomFieldValuesWithDefsByEntityRow{
		FieldID: fieldID, FieldGroupID: groupID, FieldKey: key, FieldType: fieldType,
		ValueNumber: num, ValueJson: jsn,
	}
}

func computedDTO(t *testing.T, key, fieldType string, sources string) EntityFieldDTO {
	t.Helper()
	return EntityFieldDTO{
		ID: pgid(0xF0).String(), Key: key, Label: key, FieldType: fieldType,
		Options:  json.RawMessage(`{"computed":{"metric":"avg","sources":` + sources + `}}`),
		Visible:  true,
		CanEdit:  true,
	}
}

func TestApplyComputedFields_Avg(t *testing.T) {
	groupA, groupB := pgid(0xA0), pgid(0xB0)
	srcField := pgid(0xA1)

	ff := &fakeFieldReader{
		groups: map[string]gen.CustomFieldGroup{
			groupA.String(): {ID: groupA, Slug: "g_food"},
			groupB.String(): {ID: groupB, Slug: "g_transport"},
		},
	}
	s := &Service{fields: ff}

	dtos := []EntityFieldDTO{
		computedDTO(t, "exp_food", customfields.FieldTypeCurrency,
			`[{"group":"g_food","key":"monthly_total"},{"group":"g_transport","key":"monthly_total"}]`),
	}
	values := []gen.ListCustomFieldValuesWithDefsByEntityRow{
		entityVal(srcField, groupA, "monthly_total", customfields.FieldTypeCurrency, numericVal(t, "300"), nil),
		entityVal(pgid(0xB1), groupB, "monthly_total", customfields.FieldTypeCurrency, numericVal(t, "500"), nil),
	}
	visible := map[string]bool{srcField.String(): true, pgid(0xB1).String(): true}

	s.applyComputedFields(context.Background(), dtos, values, visible, false)

	var got float64
	if err := json.Unmarshal(dtos[0].Value, &got); err != nil || got != 400 {
		t.Fatalf("value: got %s err %v", dtos[0].Value, err)
	}
	if !dtos[0].Computed || dtos[0].CanEdit {
		t.Fatalf("computed/can_edit flags wrong: %+v", dtos[0])
	}
}

func TestApplyComputedFields_NoSourcesEmpty(t *testing.T) {
	s := &Service{fields: &fakeFieldReader{}}
	dtos := []EntityFieldDTO{
		computedDTO(t, "exp_food", customfields.FieldTypeCurrency, `[{"group":"nope","key":"k"}]`),
	}
	dtos[0].Value = json.RawMessage(`999`) // stored manual value — must not leak through
	s.applyComputedFields(context.Background(), dtos, nil, map[string]bool{}, false)
	if dtos[0].Value != nil {
		t.Fatalf("stored value must not survive a computed field: %s", dtos[0].Value)
	}
	if !dtos[0].Computed {
		t.Fatal("still computed even with zero sources")
	}
}

func TestApplyComputedFields_SourceVisibility(t *testing.T) {
	groupA := pgid(0xA0)
	hidden, shown := pgid(0xA1), pgid(0xA2)
	ff := &fakeFieldReader{
		groups: map[string]gen.CustomFieldGroup{groupA.String(): {ID: groupA, Slug: "g"}},
	}
	s := &Service{fields: ff}

	dtos := []EntityFieldDTO{
		computedDTO(t, "f", customfields.FieldTypeNumber, `[{"group":"g","key":"a"},{"group":"g","key":"b"}]`),
	}
	values := []gen.ListCustomFieldValuesWithDefsByEntityRow{
		entityVal(hidden, groupA, "a", customfields.FieldTypeNumber, numericVal(t, "10"), nil),
		entityVal(shown, groupA, "b", customfields.FieldTypeNumber, numericVal(t, "30"), nil),
	}
	// 'hidden' not in the caller's can_view set — contributes nothing.
	s.applyComputedFields(context.Background(), dtos, values, map[string]bool{shown.String(): true}, false)

	var got float64
	if err := json.Unmarshal(dtos[0].Value, &got); err != nil || got != 30 {
		t.Fatalf("invisible source must not aggregate, got %s", dtos[0].Value)
	}
}

func TestApplyComputedFields_MetricSumAndPriceRangeSource(t *testing.T) {
	groupA := pgid(0xA0)
	srcField := pgid(0xA1)
	ff := &fakeFieldReader{
		groups: map[string]gen.CustomFieldGroup{groupA.String(): {ID: groupA, Slug: "g"}},
	}
	s := &Service{fields: ff}

	dtos := []EntityFieldDTO{{
		ID: pgid(0xF0).String(), Key: "total", Label: "Total", FieldType: customfields.FieldTypeCurrency,
		Options: json.RawMessage(`{"computed":{"metric":"sum","sources":[{"group":"g","key":"rent"}]}}`),
		Visible: true, CanEdit: true,
	}}
	values := []gen.ListCustomFieldValuesWithDefsByEntityRow{
		entityVal(srcField, groupA, "rent", customfields.FieldTypePriceRange, pgtype.Numeric{}, []byte(`{"min":100,"avg":140,"max":200}`)),
	}
	s.applyComputedFields(context.Background(), dtos, values, map[string]bool{srcField.String(): true}, false)

	var got float64
	if err := json.Unmarshal(dtos[0].Value, &got); err != nil || got != 140 {
		t.Fatalf("price_range source should project avg, got %s", dtos[0].Value)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/wiki/ -run TestApplyComputedFields -v`
Expected: FAIL — `applyComputedFields` undefined.

- [ ] **Step 3: Implement `service_computed.go`**

```go
// Package wiki — service_computed.go implements options.computed: a
// field whose displayed value is an aggregate of source fields' stored
// cfv on the same entity (§80 page rollup amendment). Read-time only —
// computed values are never persisted, and a declared-computed field
// rejects manual set_value (see checkSetValueFields/applySetValue).
package wiki

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/customfields"
	"github.com/jedafe/japa/internal/db/gen"
)

// applyComputedFields post-processes a rendered field list: each field
// whose options declare a computed spec gets its Value folded from the
// sources' stored values, Computed flagged, and CanEdit cleared (the
// field is derived — there is nothing to edit). Sources the caller
// cannot can_view contribute nothing (fail-closed, same contract as the
// rollups). Mutates dtos in place; call after Visible is populated and
// before fieldSections so the computed values land in both views.
func (s *Service) applyComputedFields(ctx context.Context, dtos []EntityFieldDTO, values []gen.ListCustomFieldValuesWithDefsByEntityRow, visibleSet map[string]bool, canManage bool) {
	if s.fields == nil || len(dtos) == 0 {
		return
	}

	// Index the entity's stored values by group+key — the spec's lookup
	// coordinate after group-slug resolution.
	bySource := make(map[string]gen.ListCustomFieldValuesWithDefsByEntityRow, len(values))
	for _, v := range values {
		bySource[v.FieldGroupID.String()+"/"+v.FieldKey] = v
	}

	groupIDs := map[string]pgtype.UUID{} // slug → id, memoized per call
	sourceVisible := func(id pgtype.UUID) bool {
		if canManage {
			return true
		}
		return visibleSet != nil && visibleSet[id.String()]
	}

	for i := range dtos {
		if !dtos[i].Visible {
			continue
		}
		spec := customfields.ComputedSpecFromOptions(dtos[i].Options)
		if spec == nil {
			continue
		}

		scalars := []float64{}
		for _, src := range spec.Sources {
			gid, ok := groupIDs[src.Group]
			if !ok {
				g, err := s.fields.GetFieldGroupBySlug(ctx, src.Group)
				if err != nil {
					slog.Warn("wiki: computed field: source group unresolved",
						"group", src.Group, "error", err)
					groupIDs[src.Group] = pgtype.UUID{}
					continue
				}
				gid = g.ID
				groupIDs[src.Group] = gid
			}
			if !gid.Valid {
				continue
			}
			v, ok := bySource[gid.String()+"/"+src.Key]
			if !ok || !sourceVisible(v.FieldID) {
				continue
			}
			if f, ok := cfvSourceScalar(v); ok {
				scalars = append(scalars, f)
			}
		}

		dtos[i].Computed = true
		dtos[i].CanEdit = false
		if len(scalars) == 0 {
			dtos[i].Value = nil
			continue
		}
		dtos[i].Value = computedValueJSON(dtos[i].FieldType, dtos[i].Options, foldComputed(spec.Metric, scalars))
	}
}

// cfvSourceScalar projects one stored source value to the scalar it
// contributes: scalar numerics read value_number; price_range reads avg
// (min then max fallback when avg is undeclared); price_parts reads the
// part named by the source field's own options.primary_part.
func cfvSourceScalar(v gen.ListCustomFieldValuesWithDefsByEntityRow) (float64, bool) {
	switch v.FieldType {
	case customfields.FieldTypePriceRange:
		var pr struct {
			Avg *float64 `json:"avg"`
			Min *float64 `json:"min"`
			Max *float64 `json:"max"`
		}
		if json.Unmarshal(v.ValueJson, &pr) != nil {
			return 0, false
		}
		for _, p := range []*float64{pr.Avg, pr.Min, pr.Max} {
			if p != nil && finite(*p) {
				return *p, true
			}
		}
		return 0, false
	case customfields.FieldTypePriceParts:
		pp := customfields.PrimaryPartFromOptions(v.FieldOptions)
		if pp == "" {
			return 0, false
		}
		var parts struct {
			Parts map[string]json.RawMessage `json:"parts"`
		}
		if json.Unmarshal(v.ValueJson, &parts) != nil {
			return 0, false
		}
		raw, ok := parts.Parts[pp]
		if !ok {
			return 0, false
		}
		var f float64
		if json.Unmarshal(raw, &f) != nil || !finite(f) {
			return 0, false
		}
		return f, true
	default:
		f, ok := cfvNumeric(v.ValueNumber)
		return f, ok && finite(f)
	}
}

// foldComputed applies the spec's metric to the collected source scalars.
// scalars is non-empty by contract (callers guard before folding).
func foldComputed(metric string, scalars []float64) float64 {
	switch metric {
	case "min":
		return minOf(scalars)
	case "max":
		return maxOf(scalars)
	case "sum":
		var s float64
		for _, x := range scalars {
			s += x
		}
		return s
	default: // "avg"
		return avgOf(scalars)
	}
}

// computedValueJSON renders the aggregate in the field type's storage
// shape so every existing value renderer works unchanged: scalar types
// get a bare number, price_range gets {"avg":v}, price_parts gets
// {"parts":{primary:v}} using the field's own options.primary_part.
func computedValueJSON(fieldType string, options []byte, v float64) json.RawMessage {
	switch fieldType {
	case customfields.FieldTypePriceRange:
		b, _ := json.Marshal(map[string]float64{"avg": v})
		return b
	case customfields.FieldTypePriceParts:
		pp := customfields.PrimaryPartFromOptions(options)
		if pp == "" {
			return nil
		}
		b, _ := json.Marshal(map[string]any{"parts": map[string]float64{pp: v}})
		return b
	default:
		b, _ := json.Marshal(v)
		return b
	}
}
```

- [ ] **Step 4: Add `Computed` to `EntityFieldDTO` and wire both read paths**

In `service_fields.go` `EntityFieldDTO` (after `Section`):

```go
	// Computed marks a field whose Value is derived from options.computed
	// sources — never manually editable (set_value ops are rejected).
	Computed bool `json:"computed,omitempty"`
```

In `GetPageFields`, after the `for _, f := range fields` loop (after line ~773, before `if len(dtos) == 0`):

```go
	s.applyComputedFields(ctx, dtos, values, visibleSet, canManage)
```

In `GetEntityFields`, inside the `for _, a := range assignments` loop, after the `for _, f := range fields` loop ends (before `if len(dtos) == 0 { continue }`):

```go
		s.applyComputedFields(ctx, dtos, values, visibleSet, canManage)
```

Note: `values` in `GetEntityFields` is `[]gen.ListCustomFieldValuesWithDefsByEntityRow` — the same variable already loaded at line ~496.

- [ ] **Step 5: Run tests**

Run: `go build ./internal/wiki/ && go test ./internal/wiki/ -run TestApplyComputedFields -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/wiki/service_computed.go internal/wiki/service_computed_test.go internal/wiki/service_fields.go
git commit -m "feat(wiki): computed fields — options.computed values derived from source fields (§80)"
```

---

### Task 4: Write-path rejection of `set_value` on computed fields

**Files:**
- Modify: `internal/wiki/service_fields.go` (`checkSetValueFields`, ~line 1252)
- Modify: `internal/wiki/fieldops_apply.go` (`applySetValue`, ~line 105)
- Test: `internal/wiki/service_computed_test.go` (append)

**Interfaces:**
- Consumes: `customfields.ComputedSpecFromOptions` (Task 1), `schemaField.options` (already populated by `entitySchemaFields`), `ErrOpNotPermitted`.

- [ ] **Step 1: Write the failing test**

Append to `service_computed_test.go`:

```go
func TestCheckSetValueFields_ComputedRejected(t *testing.T) {
	s := &Service{fields: &fakeFieldReader{
		perms: map[string][2]bool{},
	}}
	computedOpts := []byte(`{"computed":{"sources":[{"group":"g","key":"k"}]}}`)
	schema := map[string]schemaField{
		"derived": {id: pgid(0x01), key: "derived", fieldType: customfields.FieldTypeCurrency, options: computedOpts},
		"manual":  {id: pgid(0x02), key: "manual", fieldType: customfields.FieldTypeCurrency},
	}
	op := FieldOp{Op: OpSetValue, FieldKey: "derived", EntityType: "space", EntityID: pgid(0xEE).String(), Value: json.RawMessage(`100`)}

	err := s.checkSetValueFields(context.Background(), pgid(0x01), []FieldOp{op}, true, schema)
	if err == nil {
		t.Fatal("set_value on a computed field must be rejected")
	}
}

func TestApplySetValue_ComputedRejected(t *testing.T) {
	// The computed check precedes any query — nil q/tx prove it never
	// reaches the store.
	schema := map[string]schemaField{
		"derived": {id: pgid(0x01), key: "derived", fieldType: customfields.FieldTypeCurrency,
			options: []byte(`{"computed":{"sources":[{"group":"g","key":"k"}]}}`)},
	}
	op := FieldOp{Op: OpSetValue, FieldKey: "derived", EntityType: "space", EntityID: pgid(0xEE).String(), Value: json.RawMessage(`100`)}
	if err := applySetValue(context.Background(), nil, nil, op, schema); err == nil {
		t.Fatal("applySetValue must reject computed fields")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/wiki/ -run 'ComputedRejected' -v`
Expected: FAIL — both currently allow the write.

- [ ] **Step 3: Implement**

In `checkSetValueFields`, immediately after the `field, exists := schema[op.FieldKey]` block (after the not-exists error return, before the `eid.Scan` block):

```go
		// Computed fields derive their value from options.computed
		// sources — manual set_value is rejected for every tier,
		// including wiki.fields.manage.
		if customfields.ComputedSpecFromOptions(field.options) != nil {
			return fmt.Errorf("%w: ops[%d] field %q is computed and cannot be set manually", ErrOpNotPermitted, i, op.FieldKey)
		}
```

In `applySetValue` (fieldops_apply.go), immediately after the schema lookup error return:

```go
	// The apply-path twin of the submit-time computed check — a revision
	// queued before a spec was added (or an earlier upsert_field that
	// introduced one) cannot slip a manual value through approval.
	if customfields.ComputedSpecFromOptions(field.options) != nil {
		return fmt.Errorf("field %q is computed and cannot be set manually", op.FieldKey)
	}
```

- [ ] **Step 4: Run tests**

Run: `go build ./internal/wiki/ && go test ./internal/wiki/ -run 'Computed|TestSubmitFieldOps|TestApply' -v`
Expected: PASS (existing ops tests unaffected)

- [ ] **Step 5: Commit**

```bash
git add internal/wiki/service_fields.go internal/wiki/fieldops_apply.go internal/wiki/service_computed_test.go
git commit -m "feat(wiki): reject manual set_value on computed fields (§80)"
```

---

### Task 5: Composite preview sections on the category stub page

**Files:**
- Modify: `internal/wiki/service.go` (`GetWikiPage` ~line 916, `lazyCategoryPage` ~line 1111-1121)

**Interfaces:**
- Consumes: `childTemplates(templates, parentID)` — passing `pgtype.UUID{}` returns the top-level set (it filters `t.ParentTemplateID == parentID`); `attachCompositeSections` (service_hierarchy.go:841).

- [ ] **Step 1: Write the failing test**

Append to `service_hierarchy_test.go`:

```go
func TestChildTemplates_TopLevelForCategoryPage(t *testing.T) {
	parent := pgid(0x50)
	templates := []WikiPageTemplate{
		{Slug: "a", ParentTemplateID: pgtype.UUID{}},
		{Slug: "b", ParentTemplateID: parent},
		{Slug: "c", ParentTemplateID: pgtype.UUID{}},
	}
	got := childTemplates(templates, pgtype.UUID{})
	if len(got) != 2 || got[0].Slug != "a" || got[1].Slug != "c" {
		t.Fatalf("category-page composite set must be top-level templates: %+v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/wiki/ -run TestChildTemplates_TopLevelForCategoryPage -v`
Expected: likely PASS already (documents existing behavior) — if it passes, keep it as a regression guard and continue.

- [ ] **Step 3: Wire the category stub page**

In `GetWikiPage`, change the composite block (currently `if pageTmpl != nil { s.attachCompositeSections(...) }`) to:

```go
		// §80 page rollup amendment: a template page composes previews of
		// its child templates; the category stub page composes previews of
		// the TOP-LEVEL templates — the flattened topic tree's item tables
		// surface directly on the category page (replacing the removed
		// 'prices' intermediate).
		if pageTmpl != nil {
			s.attachCompositeSections(ctx, userID, spaceID, page.SpaceName, groupID, &dto, childTemplates(templates, pageTmpl.ID))
		} else if page.Slug == cat.Slug {
			s.attachCompositeSections(ctx, userID, spaceID, page.SpaceName, groupID, &dto, childTemplates(templates, pgtype.UUID{}))
		}
```

In `lazyCategoryPage`, after the rollup/ancestors block (before `dto.Siblings`):

```go
	// Category stub composes one preview section per top-level template —
	// same as the materialized path.
	s.attachCompositeSections(ctx, userID, spaceID, spaceName, cat.FieldGroupID, &dto, childTemplates(templates, pgtype.UUID{}))
```

- [ ] **Step 4: Build + run wiki tests**

Run: `go build ./internal/wiki/ && go test ./internal/wiki/ -v 2>&1 | tail -20`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/wiki/service.go internal/wiki/service_hierarchy_test.go
git commit -m "feat(wiki): composite preview sections on category stub pages (§80)"
```

---

### Task 6: Migration 000857 — flatten topic pages + enable pages mode on city cost_of_living

**Files:**
- Create: `migrations/000857_wiki_city_cost_of_living_page_rollup.up.sql`
- Create: `migrations/000857_wiki_city_cost_of_living_page_rollup.down.sql`

Check first: `ls migrations | tail -5` — if a later migration number already exists, use the next free number.

- [ ] **Step 1: Write the up migration**

```sql
-- 000857_wiki_city_cost_of_living_page_rollup.up.sql
-- §80 page rollup amendment: the city's cost_of_living sub-pages feed
-- the category page's aggregates directly, so the 'prices' intermediate
-- level is redundant. Flatten the topic templates to direct children of
-- the category, drop the 'prices' template, and switch the city
-- cost_of_living rollup to mode "pages".
--
-- The wiki_city_prices group and the 'city-prices' comparison type are
-- untouched — the flat city↔city comparison keeps working. Materialized
-- /wiki/{city}/prices rows survive as ad-hoc pages bound to that group
-- (page_template_id SET NULL); unmaterialized lazy /prices URLs 404.
--
-- Ref: docs/superpowers/specs/2026-10-15-wiki-page-rollup-computed-fields-design.md

BEGIN;

UPDATE wiki_type_page_templates t
SET parent_template_id = NULL
FROM wiki_type_categories c
JOIN space_types st ON st.id = c.space_type_id
WHERE t.category_id = c.id
  AND st.slug = 'city'
  AND c.category = 'cost_of_living'
  AND t.slug IN ('food-groceries-prices', 'restaurant-prices',
                 'transportation-prices', 'utilities-prices',
                 'lifestyle-prices');

DELETE FROM wiki_type_page_templates t
USING wiki_type_categories c
JOIN space_types st ON st.id = c.space_type_id
WHERE t.category_id = c.id
  AND st.slug = 'city'
  AND c.category = 'cost_of_living'
  AND t.slug = 'prices'
  AND t.is_system;

UPDATE wiki_type_categories c
SET rollup = '{"enabled": true, "mode": "pages",
               "metrics": ["min", "avg", "max"],
               "pages": ["food-groceries-prices", "restaurant-prices",
                          "transportation-prices", "utilities-prices",
                          "lifestyle-prices"]}'::jsonb
FROM space_types st
WHERE c.space_type_id = st.id
  AND st.slug = 'city'
  AND c.category = 'cost_of_living';

COMMIT;
```

- [ ] **Step 2: Write the down migration**

```sql
-- 000857_wiki_city_cost_of_living_page_rollup.down.sql
BEGIN;

-- Restore the 'prices' template (000750 seed shape).
INSERT INTO wiki_type_page_templates (category_id, slug, label, field_group_id, position, is_system)
SELECT c.id, 'prices', 'Prices in {name}', g.id, 1, true
FROM wiki_type_categories c
JOIN space_types st ON st.id = c.space_type_id AND st.slug = 'city'
JOIN custom_field_groups g ON g.slug = 'wiki_city_prices'
WHERE c.category = 'cost_of_living'
ON CONFLICT (category_id, slug) DO NOTHING;

-- Re-nest the topic templates under 'prices'.
UPDATE wiki_type_page_templates t
SET parent_template_id = p.id
FROM wiki_type_categories c
JOIN space_types st ON st.id = c.space_type_id
JOIN wiki_type_page_templates p
  ON p.category_id = c.id AND p.slug = 'prices'
WHERE t.category_id = c.id
  AND st.slug = 'city'
  AND c.category = 'cost_of_living'
  AND t.slug IN ('food-groceries-prices', 'restaurant-prices',
                 'transportation-prices', 'utilities-prices',
                 'lifestyle-prices');

UPDATE wiki_type_categories c
SET rollup = NULL
FROM space_types st
WHERE c.space_type_id = st.id
  AND st.slug = 'city'
  AND c.category = 'cost_of_living';

COMMIT;
```

- [ ] **Step 3: Verify the migration applies**

If a local DB is running (`docker compose ps` / project Makefile target), run migrations up/down once. Otherwise: `psql --dry-run` isn't available — validate syntax by eyeballing against the 000750/000775 seeds and confirm identifiers (`wiki_type_page_templates`, `wiki_type_categories`, `custom_field_groups`, `space_types`) and column names match the schema. Record which was done.

- [ ] **Step 4: Commit**

```bash
git add migrations/000857_wiki_city_cost_of_living_page_rollup.*.sql
git commit -m "feat(wiki): city cost-of-living page rollup — flatten topics, drop prices page (§80)"
```

---

### Task 7: Frontend — types, page-mode rollup panel, computed badge, i18n

**Files:**
- Modify: `apps/main/src/lib/types/index.ts` (`WikiRollupRow` ~line 3443, `WikiRollup` ~line 3450)
- Modify: `apps/main/src/lib/api/wiki.ts` (`WikiEntityField` ~line 10)
- Modify: `apps/main/src/lib/components/wiki/WikiRollupPanel.svelte`
- Modify: `apps/main/src/routes/(app)/wiki/[space_slug]/[page_slug]/+page.svelte` (line ~627 — pass `spaceSlug`)
- Modify: `apps/main/src/lib/components/wiki/WikiItemTables.svelte` (computed badge on the three label cells)
- Modify: `apps/main/src/lib/i18n/en.json`, `apps/main/src/lib/i18n/fr.json`

**Interfaces:**
- Consumes: `WikiRollupDTO.scope`/`row.page` (Task 2), `EntityFieldDTO.computed` (Task 3).

- [ ] **Step 1: Types**

In `types/index.ts`:

```ts
/** §80 page rollup — a sub-page row reference on a pages-mode rollup. */
export interface WikiRollupPageRef {
	slug: string;
	title: string;
}

export interface WikiRollupRow {
	space: WikiSpaceRef;
	/** Present on mode "pages" rows — the contributing sub-page. */
	page?: WikiRollupPageRef;
	values: Record<string, unknown>;
}

export interface WikiRollup {
	/** "pages" = same-space sub-page aggregation; absent = descendant spaces. */
	scope?: string;
	fields: WikiRollupField[];
	rows: WikiRollupRow[];
	spaces: number;
}
```

In `api/wiki.ts` `WikiEntityField`, add:

```ts
	/** §80 page rollup — value is derived from options.computed sources;
	 *  never manually editable. */
	computed?: boolean;
```

- [ ] **Step 2: `WikiRollupPanel` page mode**

Add a `spaceSlug` prop:

```ts
	let {
		rollup,
		spaceName,
		spaceSlug = '',
		contextCurrency = ''
	}: { rollup: WikiRollup; spaceName: string; spaceSlug?: string; contextCurrency?: string } = $props();
```

Add near `spaceColLabel`:

```ts
	const pageScope = $derived(rollup.scope === 'pages');
```

Change the first-column header cell:

```svelte
<th class="px-4 py-2 font-medium">
	{pageScope ? t('wiki.rollup_col_page', 'Page') : spaceColLabel}
</th>
```

Change the row label cell (the `<a href="/wiki/{row.space.slug}">` block):

```svelte
<td class="px-4 py-2">
	{#if row.page}
		<a
			href="/wiki/{encodeURIComponent(spaceSlug)}/{encodeURIComponent(row.page.slug)}"
			class="font-medium text-[var(--color-primary)] hover:underline"
		>
			{row.page.title}
		</a>
	{:else}
		<a
			href="/wiki/{encodeURIComponent(row.space.slug)}"
			class="font-medium text-[var(--color-primary)] hover:underline"
		>
			{row.space.name}
		</a>
	{/if}
</td>
```

Change the footer line:

```svelte
<p class="text-xs text-[var(--color-muted)]">
	{pageScope
		? t('wiki.rollup_scope_pages', 'Aggregated from {count} cost pages', { count: rollup.spaces })
		: t('wiki.rollup_scope', 'Aggregated from {count} places under {name}', { count: rollup.spaces, name: spaceName })}
</p>
```

(The compare rail needs no change — page rows carry no `space.id`/`type_slug`, so `compareTarget` is null and the checkboxes/footer never render.)

- [ ] **Step 3: Pass `spaceSlug` on the page**

In `+page.svelte` (~line 627):

```svelte
<WikiRollupPanel
	rollup={data.page.rollup}
	spaceName={spaceName}
	spaceSlug={data.spaceSlug}
	contextCurrency={data.page.space_currency_code ?? ''}
/>
```

- [ ] **Step 4: Computed badge in `WikiItemTables`**

Next to each `{f.label}` in the three label cells (price-table `<td>` ~line 171, parts-table `<td>` ~line 193, dl `<dt>` ~line 232), append:

```svelte
{#if f.computed}
	<span
		class="ml-1.5 rounded-full border border-[var(--color-border)] px-1.5 py-px align-middle text-[10px] font-medium uppercase tracking-wide text-[var(--color-muted)]"
		title={t('wiki.field_computed_hint', 'Computed from this page’s sub-pages')}
	>
		{t('wiki.field_computed', 'computed')}
	</span>
{/if}
```

- [ ] **Step 5: i18n keys**

`en.json` (near the existing `wiki.rollup_*` keys ~line 5822):

```json
"wiki.rollup_col_page": "Page",
"wiki.rollup_scope_pages": "Aggregated from {count} cost pages",
"wiki.field_computed": "computed",
"wiki.field_computed_hint": "Computed from this page's sub-pages",
```

`fr.json` (near ~line 5789):

```json
"wiki.rollup_col_page": "Page",
"wiki.rollup_scope_pages": "Agrégé à partir de {count} pages de coûts",
"wiki.field_computed": "calculé",
"wiki.field_computed_hint": "Calculé à partir des sous-pages de cette page",
```

- [ ] **Step 6: Typecheck + build**

Run: `cd apps/main && bun run check`
Expected: no new errors (pre-existing baseline noise is acceptable — diff against `git stash` output if unsure).

- [ ] **Step 7: Commit**

```bash
git add apps/main/src/lib/types/index.ts apps/main/src/lib/api/wiki.ts apps/main/src/lib/components/wiki/WikiRollupPanel.svelte apps/main/src/lib/components/wiki/WikiItemTables.svelte "apps/main/src/routes/(app)/wiki/[space_slug]/[page_slug]/+page.svelte" apps/main/src/lib/i18n/en.json apps/main/src/lib/i18n/fr.json
git commit -m "feat(main): pages-mode rollup rendering + computed field badge (§80)"
```

---

### Task 8: Admin — pages mode + sub-page picker in the category rollup editor

**Files:**
- Modify: `apps/admin/src/lib/components/wiki/WikiTypesAdmin.svelte` (fields ~line 474, `rollupFromForm` ~498, `flattenRollup` ~517, rollup column ~590, form snippet ~1250)
- Modify: `apps/admin/src/lib/locales/en.json`, `apps/admin/src/lib/locales/fr.json`

**Interfaces:**
- Consumes: mode `"pages"` + `pages[]` in the rollup JSONB (Task 2 validation accepts them), `wikiAdminApi.listPageTemplates(categoryID)` (already used at ~line 835), `WikiPageTemplateItem`.

- [ ] **Step 1: Extend the flatten/assemble pair**

In `flattenRollup`, add `pages` to the config type and seed key:

```ts
	const cfg = (r.rollup ?? null) as {
		enabled?: boolean;
		mode?: string;
		metrics?: string[];
		levels?: string[];
		pages?: string[];
		section_limit?: number;
	} | null;
	return {
		...r,
		icon: r.icon ?? '',
		rollup_enabled: !!cfg?.enabled,
		rollup_mode: cfg?.mode === 'directory' ? 'directory' : cfg?.mode === 'pages' ? 'pages' : 'aggregate',
		rollup_metrics: cfg?.metrics ?? [],
		rollup_levels: cfg?.levels ?? [],
		rollup_pages: cfg?.pages ?? [],
		rollup_section_limit: cfg?.section_limit ?? 0
	};
```

In `rollupFromForm`, handle pages mode:

```ts
	function rollupFromForm(body: Record<string, unknown>): Record<string, unknown> | null {
		if (!body.rollup_enabled) return null;
		const mode = String(body.rollup_mode ?? 'aggregate');
		const cfg: Record<string, unknown> = { enabled: true, metrics: [] };
		if (mode === 'directory') {
			cfg.mode = 'directory';
		} else {
			cfg.metrics = ((body.rollup_metrics as string[]) ?? []).filter((m) =>
				(ROLLUP_METRICS as readonly string[]).includes(m)
			);
			const lim = Number(body.rollup_section_limit ?? 0);
			if (lim > 0) cfg.section_limit = lim;
			if (mode === 'pages') {
				cfg.mode = 'pages';
				const pages = (body.rollup_pages as string[]) ?? [];
				if (pages.length) cfg.pages = pages;
			} else {
				const levels = (body.rollup_levels as string[]) ?? [];
				if (levels.length) cfg.levels = levels;
			}
		}
		return cfg;
	}
```

Add `rollup_pages` to `catFields` (after `rollup_levels`):

```ts
		{ key: 'rollup_pages', label: t('app.wiki.rollup_pages', 'Pages'), type: 'text' },
```

Update the rollup column renderer (~line 597) to label pages mode:

```ts
							const label = cfg.mode === 'directory'
								? t('app.wiki.rollup_directory', 'directory')
								: cfg.mode === 'pages'
									? t('app.wiki.rollup_pages_mode', 'pages')
									: t('app.wiki.rollup_on', 'on');
```

- [ ] **Step 2: Mode picker + pages multi-select in the form snippet**

Add the option to the mode `AutocompleteSelect` options array:

```ts
{ value: 'pages', label: t('app.wiki.rollup_mode_pages', 'Sub-pages — aggregate this category’s pages') }
```

Update the help line to cover all three modes:

```svelte
<p class="mt-1 text-xs" style="color: var(--color-muted)">
	{value.rollup_mode === 'directory'
		? t('app.wiki.rollup_mode_directory_help', 'The category page lists child wiki spaces (e.g. a country’s states) with scoped search.')
		: value.rollup_mode === 'pages'
			? t('app.wiki.rollup_mode_pages_help', 'The category page aggregates its own sub-pages’ field values (e.g. a city’s cost-of-living topic pages).')
			: t('app.wiki.rollup_mode_aggregate_help', 'Descendant spaces’ numeric/price-range field values fold up onto this category’s page.')}
</p>
```

Replace the `{#if value.rollup_mode !== 'directory'}` block's structure: metrics + section limit show for `aggregate` and `pages`; the levels picker shows only for `aggregate`; pages mode shows a template checklist. Insert after the metrics/section-limit block, replacing the whole levels `<div>`:

```svelte
					{#if value.rollup_mode === 'pages'}
						<div>
							<p class="mb-1 text-sm font-medium" style="color: var(--color-foreground)">
								{t('app.wiki.rollup_pages', 'Source pages')}
							</p>
							{#if value.id}
								{#await catTemplates(String(value.id)) then tpls}
									{#if tpls.length === 0}
										<p class="text-xs" style="color: var(--color-muted)">
											{t('app.wiki.rollup_no_pages', 'This category has no page templates yet — all templates feed the rollup once created.')}
										</p>
									{:else}
										<div class="space-y-1.5">
											{#each tpls as tpl (tpl.id)}
												<label class="flex items-center gap-2 text-sm" style="color: var(--color-foreground)">
													<input
														type="checkbox" class="h-4 w-4 rounded"
														checked={((value.rollup_pages as string[]) ?? []).includes(tpl.slug)}
														onchange={(e) => toggleRollupList(value, 'rollup_pages', tpl.slug, e.currentTarget.checked)}
													/>
													{tpl.label}
													<span class="font-mono text-xs" style="color: var(--color-muted)">{tpl.slug}</span>
												</label>
											{/each}
										</div>
										<p class="mt-1 text-xs" style="color: var(--color-muted)">
											{t('app.wiki.rollup_pages_help', 'Restrict which sub-pages feed the rollup — none checked = every page template under this category.')}
										</p>
									{/if}
								{/await}
							{:else}
								<p class="text-xs" style="color: var(--color-muted)">
									{t('app.wiki.rollup_pages_create_first', 'Save the category first — its page templates become selectable once they exist.')}
								</p>
							{/if}
						</div>
					{:else if value.rollup_mode !== 'directory'}
						<div>
							<!-- existing 'Child levels' block, unchanged -->
						</div>
					{/if}
```

Add the memoized fetch helper in the script (near `childTypesOf`):

```ts
	// Page-rollup source picker: each category's templates, fetched once
	// and cached — {#await} re-invokes the function every render, so the
	// Map keeps the promise stable.
	const tplCache = new Map<string, Promise<WikiPageTemplateItem[]>>();
	function catTemplates(categoryID: string): Promise<WikiPageTemplateItem[]> {
		if (!tplCache.has(categoryID)) {
			tplCache.set(
				categoryID,
				wikiAdminApi
					.listPageTemplates(categoryID)
					.then((res) => ((res.data as WikiPageTemplateItem[]) ?? []))
					.catch(() => [] as WikiPageTemplateItem[])
			);
		}
		return tplCache.get(categoryID)!;
	}
```

- [ ] **Step 3: i18n keys**

`en.json` (near `app.wiki.rollup_*` ~line 4341):

```json
"app.wiki.rollup_mode_pages": "Sub-pages — aggregate this category's pages",
"app.wiki.rollup_mode_pages_help": "The category page aggregates its own sub-pages' field values (e.g. a city's cost-of-living topic pages).",
"app.wiki.rollup_pages": "Source pages",
"app.wiki.rollup_pages_mode": "pages",
"app.wiki.rollup_pages_help": "Restrict which sub-pages feed the rollup — none checked = every page template under this category.",
"app.wiki.rollup_no_pages": "This category has no page templates yet — all templates feed the rollup once created.",
"app.wiki.rollup_pages_create_first": "Save the category first — its page templates become selectable once they exist.",
```

`fr.json` (matching block ~line 4333):

```json
"app.wiki.rollup_mode_pages": "Sous-pages — agréger les pages de cette catégorie",
"app.wiki.rollup_mode_pages_help": "La page de catégorie agrège les valeurs de ses propres sous-pages (ex. les pages thématiques du coût de la vie d'une ville).",
"app.wiki.rollup_pages": "Pages sources",
"app.wiki.rollup_pages_mode": "pages",
"app.wiki.rollup_pages_help": "Restreint les sous-pages agrégées — aucune case cochée = tous les modèles de page de la catégorie.",
"app.wiki.rollup_no_pages": "Cette catégorie n'a pas encore de modèles de page — tous les modèles alimenteront l'agrégat une fois créés.",
"app.wiki.rollup_pages_create_first": "Enregistrez d'abord la catégorie — ses modèles de page deviennent sélectionnables une fois créés.",
```

- [ ] **Step 4: Typecheck**

Run: `cd apps/admin && bun run check`
Expected: no new errors.

- [ ] **Step 5: Commit**

```bash
git add apps/admin/src/lib/components/wiki/WikiTypesAdmin.svelte apps/admin/src/lib/locales/en.json apps/admin/src/lib/locales/fr.json
git commit -m "feat(admin): pages mode + sub-page picker in wiki category rollup editor (§80)"
```

---

### Task 9: End-to-end verification

- [ ] **Step 1: Full backend test pass**

Run: `go build ./... && go test ./internal/wiki/ ./internal/customfields/`
Expected: PASS

- [ ] **Step 2: Manual smoke (if a dev DB/server is running)**

- Apply migration 000857; open `/wiki/{some-city}/cost-of-living` — rollup section renders with one row per topic page; topic pages render as direct siblings under Cost of Living; `/wiki/{city}/prices` lazy URL 404s (materialized rows still render).
- Add a `computed` spec to a `wiki_city_cost_of_living` field via the admin field editor (options JSON), e.g. `{"computed":{"metric":"avg","sources":[{"group":"<topic group slug>","key":"<field key>"}]}}` — the category page shows the averaged value with the "computed" badge; a `set_value` op on it is rejected.

- [ ] **Step 3: Commit docs updates if `.jeos` catalogs require them**

The `.jeos` conventions catalog document this feature family — add a one-line amendment note where §80 rollup modes are cataloged (`docs` or `.jeos` file that already lists `"directory"` mode) if the file exists; otherwise skip.
