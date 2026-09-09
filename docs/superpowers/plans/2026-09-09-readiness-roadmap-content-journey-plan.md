# Readiness Roadmap Content Journey Resolution Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the `/admin/tools/readiness/variants` page and make the readiness assessment resolve its roadmap from guide content tagged with `dimension`, `purpose`, and journey context at creation time.

**Architecture:** Extend the existing public `GET /assessment` response with a derived `dimensions` array, relax `internal/assessment/service.go:resolveRoadmapContent` to search the whole published corpus by `dimension` + journey context, add `Roadmap dimension` and `Purpose` fields to the main content composer’s Journey panel, and delete the admin variants page.

**Tech Stack:** Go, SvelteKit/Svelte 5, Bun, sqlc, PostgreSQL.

## Global Constraints

- All new behavior comes from the backend API — no hardcoded option lists in the frontend (Rule 16).
- No AI authorship attribution in commits, comments, or docs (Rule 17).
- Use `bun` for JS/TS tooling, never `npm`/`npx`/`pnpm`/`yarn` (Rule 18).
- Do not hand-edit `internal/db/gen` files; change `.sql` and run `make sqlc-gen` if needed.
- All commits to `main`; do not push unless explicitly asked.
- Update JEOS before diverging from the spec.
- TDD for backend: failing test first, then implementation.

## File map

| File | Responsibility |
|---|---|
| `internal/assessment/service.go` | Compute dimensions for `GET /assessment`; relax `resolveRoadmapContent`. |
| `internal/assessment/handlers.go` | Include `dimensions` in `HandleGetAssessment` JSON response. |
| `internal/assessment/service_test.go` | Tests for `dimensions` output and `pickBestRoadmapContent` global resolution. |
| `apps/main/src/routes/(app)/create/+page.server.ts` | Load `GET /assessment` and return `assessmentDimensions`. |
| `apps/main/src/routes/(app)/create/+page.svelte` | Pass `data.assessmentDimensions` into `CreateShell`. |
| `apps/main/src/lib/components/create/CreateShell.svelte` | Accept `assessmentDimensions`; manage `roadmapDimension`/`roadmapPurpose`; pass through to `CreateRightPanel`. |
| `apps/main/src/lib/components/create/panel/CreateRightPanel.svelte` | Forward new props to `CreateJourneyPanel`. |
| `apps/main/src/lib/components/create/panel/CreateJourneyPanel.svelte` | Render `Roadmap dimension` and `Purpose` selects. |
| `apps/main/src/lib/types/index.ts` | Add `dimensions: string[]` to the existing `AssessmentForm` type. |
| `apps/admin/src/routes/tools/readiness/variants/+page.svelte` | Delete. |
| `apps/admin/src/routes/tools/readiness/+page.svelte` | Remove the "Manage Variants" button. |
| `apps/admin/src/lib/components/layout/sidebar.svelte` | Remove variants entry if it exists. |
| `.jeos/sections/71-readiness-assessment.md` | Update to remove variants workflow. |
| `.jeos/adr/ADR-071-japa-readiness-assessment.md` | Update to describe content-journey resolution. |

---

### Task 1: Backend — add dimension list to the active assessment

**Files:**
- Modify: `internal/assessment/service.go` (add `assessmentDimensions` helper)
- Modify: `internal/assessment/handlers.go` (`HandleGetAssessment` response)
- Create: `internal/assessment/service_test.go`

**Interfaces:**
- Consumes: `AssessmentFormDTO.Fields[].Config.Dimension`, `ScoringConfig.CuratedRoadmap`, fallback `curatedRoadmapLibrary()`.
- Produces: `data.dimensions []string` in `GET /assessment` response.

- [ ] **Step 1: Write the failing test**

Create `internal/assessment/service_test.go`:

```go
package assessment

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAssessmentDimensions(t *testing.T) {
	form := &AssessmentFormDTO{
		Fields: []AssessmentField{
			{Config: FieldScoringConfig{Dimension: "financial"}},
			{Config: FieldScoringConfig{Dimension: "logistical"}},
			{Config: FieldScoringConfig{Dimension: ""}},
		},
		// Empty CuratedRoadmap falls back to the default library.
		ScoringConfig: ScoringConfig{},
	}

	got := assessmentDimensions(form)
	sort.Strings(got)

	assert.Contains(t, got, "financial")
	assert.Contains(t, got, "logistical")
	assert.Contains(t, got, "language") // from default library
	assert.NotContains(t, got, "")      // empty dimensions are skipped
}
```

- [ ] **Step 2: Run the test and confirm it fails**

```bash
go test ./internal/assessment/... -run TestAssessmentDimensions -v
```

Expected: FAIL because `assessmentDimensions` is not yet defined.

- [ ] **Step 3: Add the helper in `internal/assessment/service.go`**

Add after the `AssessmentFormDTO` type:

```go
// assessmentDimensions returns the distinct readiness dimensions that may be
// recommended by the active assessment. It is the union of dimensions declared
// on form fields and the keys of the active/default curated roadmap library.
// Ref: docs/superpowers/specs/2026-09-09-readiness-roadmap-content-journey-design.md
func assessmentDimensions(form *AssessmentFormDTO) []string {
	seen := make(map[string]bool)

	for _, f := range form.Fields {
		if d := strings.TrimSpace(f.Config.Dimension); d != "" {
			seen[d] = true
		}
	}

	library := form.ScoringConfig.CuratedRoadmap
	if len(library) == 0 {
		library = curatedRoadmapLibrary()
	}
	for d := range library {
		seen[d] = true
	}

	out := make([]string, 0, len(seen))
	for d := range seen {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}
```

- [ ] **Step 4: Include `dimensions` in `HandleGetAssessment`**

In `internal/assessment/handlers.go` inside `HandleGetAssessment`, update the response map:

```go
errpkg.WriteJSON(w, http.StatusOK, map[string]any{
	"data": map[string]any{
		"id":            form.ID,
		"name":          form.Name,
		"slug":          form.Slug,
		"description":   form.Description,
		"is_multi_step": form.IsMultiStep,
		"submit_label":  form.SubmitLabel,
		"status":        form.Status,
		"max_score":     form.ScoringConfig.MaxScore,
		"thresholds":    thresholds,
		"fields":        publicFieldDTOs(form.Fields),
		"dimensions":    assessmentDimensions(form),
		"scoring_config": map[string]any{
			"max_score":  form.ScoringConfig.MaxScore,
			"thresholds": thresholds,
		},
	},
})
```

- [ ] **Step 5: Run the test and confirm it passes**

```bash
go test ./internal/assessment/... -run TestHandleGetAssessment_IncludesDimensions -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/assessment/service.go internal/assessment/service_test.go internal/assessment/handlers.go
git commit -m "feat(assessment): include derived dimensions in GET /assessment response"
```

---

### Task 2: Backend — relax `resolveRoadmapContent` to global search

**Files:**
- Modify: `internal/assessment/service.go` (`resolveRoadmapContent`)
- Test: `internal/assessment/service_test.go`

**Interfaces:**
- Consumes: `FindRoadmapContentByDimension` rows, `RoadmapStep.ContentSlug` (base), `RoadmapStep.Dimension`.
- Produces: mutated `RoadmapStep` with `ContentSlug` possibly swapped to the best matching published guide.

- [ ] **Step 1: Write the failing test**

Add to `internal/assessment/service_test.go`:

```go
func ptr(s string) *string { return &s }

func TestPickBestRoadmapContent_PrefersStrongestContextMatch(t *testing.T) {
	base := "guide-generic-finance"
	rows := []gen.FindRoadmapContentByDimensionRow{
		{Slug: base},
		{Slug: "canada-study-finance", TargetCountry: ptr("CA"), VisaRoute: ptr("student")},
	}
	journey := JourneyContext{
		DestinationCountry: "CA",
		VisaRoute:          "student",
		JourneyStage:       "planning",
	}

	best, score := pickBestRoadmapContent(rows, base, map[string]bool{}, journey)

	assert.Equal(t, "canada-study-finance", best.Slug)
	assert.Equal(t, 6, score, "target match (4) + visa match (2)")
}

func TestPickBestRoadmapContent_KeepsOriginalOnTie(t *testing.T) {
	base := "guide-generic-finance"
	rows := []gen.FindRoadmapContentByDimensionRow{
		{Slug: "another-planning-finance", JourneyStage: ptr("planning")},
		{Slug: base, JourneyStage: ptr("planning")},
	}
	journey := JourneyContext{JourneyStage: "planning"}

	best, _ := pickBestRoadmapContent(rows, base, map[string]bool{}, journey)
	assert.Equal(t, base, best.Slug, "when scores tie, the original curated slug wins")
}

func TestPickBestRoadmapContent_SkipsUsedSlugs(t *testing.T) {
	base := "guide-generic-finance"
	rows := []gen.FindRoadmapContentByDimensionRow{
		{Slug: "used-already", TargetCountry: ptr("CA")},
		{Slug: base, TargetCountry: ptr("CA")},
	}
	journey := JourneyContext{DestinationCountry: "CA"}

	best, _ := pickBestRoadmapContent(rows, base, map[string]bool{"used-already": true}, journey)
	assert.Equal(t, base, best.Slug, "already-used slugs are skipped")
}

func TestPickBestRoadmapContent_BaseWinsTieEvenWhenUsed(t *testing.T) {
	base := "guide-generic-finance"
	rows := []gen.FindRoadmapContentByDimensionRow{
		{Slug: "another-planning-finance", JourneyStage: ptr("planning")},
		{Slug: base, JourneyStage: ptr("planning")},
	}
	journey := JourneyContext{JourneyStage: "planning"}

	// base is in used (as it would be at runtime from the seed map), but it is
	// still allowed to compete and win on a tie.
	best, _ := pickBestRoadmapContent(rows, base, map[string]bool{base: true}, journey)
	assert.Equal(t, base, best.Slug, "the original base slug wins ties even if it is in the used set")
}
```

- [ ] **Step 2: Run the test and confirm it fails**

```bash
go test ./internal/assessment/... -run TestPickBestRoadmapContent -v
```

Expected: FAIL because `pickBestRoadmapContent` is not yet defined.

- [ ] **Step 3: Extract `pickBestRoadmapContent` and remove the slug-family guard**

In `internal/assessment/service.go`, add a new pure helper above `resolveRoadmapContent`:

```go
// pickBestRoadmapContent selects the highest-scoring, unused published guide for a
// roadmap step. It does not restrict candidates to the original step's slug family,
// so any guide with the same dimension and a better journey match can win.
//
// Ref: docs/superpowers/specs/2026-09-09-readiness-roadmap-content-journey-design.md
func pickBestRoadmapContent(rows []gen.FindRoadmapContentByDimensionRow, base string, used map[string]bool, journeyCtx JourneyContext) (gen.FindRoadmapContentByDimensionRow, int) {
	best := gen.FindRoadmapContentByDimensionRow{}
	bestScore := 0

	for _, row := range rows {
		// The current step's own curated slug is allowed as a candidate even if it
		// is already in `used`, so the tie-breaker can keep the original on equal
		// context scores. Other steps' slugs remain blocked to avoid duplicates.
		if row.Slug == "" || (used[row.Slug] && row.Slug != base) {
			continue
		}

		score := 0
		if journeyCtx.DestinationCountry != "" && row.TargetCountry != nil && *row.TargetCountry == journeyCtx.DestinationCountry {
			score += 4
		}
		if journeyCtx.OriginCountry != "" && row.SourceCountry != nil && *row.SourceCountry == journeyCtx.OriginCountry {
			score += 1
		}
		if journeyCtx.JourneyStage != "" && row.JourneyStage != nil && *row.JourneyStage == journeyCtx.JourneyStage {
			score += 2
		}
		if journeyCtx.VisaRoute != "" && row.VisaRoute != nil && *row.VisaRoute == journeyCtx.VisaRoute {
			score += 2
		}
		if p := stringFromAny(row.Purpose); journeyCtx.Purpose != "" && p != "" && p == journeyCtx.Purpose {
			score += 1
		}

		if score <= 0 {
			continue
		}

		// Tie-breaker: when two guides are equally specific, keep the original.
		if score == bestScore && bestScore > 0 && row.Slug == base && best.Slug != base {
			best = row
		}

		if score > bestScore {
			best = row
			bestScore = score
		}
	}

	return best, bestScore
}
```

Then simplify `resolveRoadmapContent` to call the helper (keep the existing step metadata update block):

```go
for i := range steps {
    if steps[i].ContentSlug == "" {
        continue
    }

    rows, err := s.store.FindRoadmapContentByDimension(
        ctx, tenantID,
        steps[i].Dimension,
        journeyCtx.DestinationCountry,
        journeyCtx.JourneyStage,
        journeyCtx.VisaRoute,
        journeyCtx.OriginCountry,
        journeyCtx.Purpose,
    )
    if err != nil {
        slog.Warn("assessment: resolve roadmap content failed",
            "dimension", steps[i].Dimension,
            "error", err)
        continue
    }

    best, bestScore := pickBestRoadmapContent(rows, steps[i].ContentSlug, used, journeyCtx)
    if bestScore == 0 {
        continue
    }

    steps[i].ContentSlug = best.Slug
    if best.Title != "" {
        steps[i].Title = best.Title
    }
    if best.Description != "" {
        steps[i].Description = best.Description
    }
    if best.TargetCountry != nil && *best.TargetCountry != "" {
        steps[i].TargetCountry = *best.TargetCountry
    }
    if best.JourneyStage != nil && *best.JourneyStage != "" {
        steps[i].JourneyStage = *best.JourneyStage
    }
    if best.VisaRoute != nil && *best.VisaRoute != "" {
        steps[i].VisaRoute = *best.VisaRoute
    }
    if best.SourceCountry != nil && *best.SourceCountry != "" {
        steps[i].SourceCountry = *best.SourceCountry
    }
    if p := stringFromAny(best.Purpose); p != "" {
        steps[i].Purpose = p
    }

    used[best.Slug] = true
}
```

Remove the old inline scoring loop and the slug-family guard.

**Note:** `FindRoadmapContentByDimension` only returns content whose `field_values->>'dimension'` matches. If the original base guide is not tagged with that dimension, it will not be in the candidate set and cannot win ties. Either tag base curated guides with their dimensions or explicitly include the base content as a fallback candidate.

- [ ] **Step 4: Run the test and confirm it passes**

```bash
go test ./internal/assessment/... -run TestPickBestRoadmapContent -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/assessment/service.go internal/assessment/service_test.go
git commit -m "feat(assessment): resolve roadmap from any matching guide by dimension + context"
```

---

### Task 3: Frontend — load dimensions into the create page

**Files:**
- Modify: `apps/main/src/lib/types/index.ts`
- Modify: `apps/main/src/routes/(app)/create/+page.server.ts`
- Modify: `apps/main/src/routes/(app)/create/+page.svelte`

**Interfaces:**
- Consumes: `GET /assessment` response `dimensions`.
- Produces: `page.data.assessmentDimensions: string[]`.

- [ ] **Step 1: Extend the existing `AssessmentForm` type**

In `apps/main/src/lib/types/index.ts`, the `AssessmentForm` interface already exists. Add one field:

```ts
export interface AssessmentForm {
	// ...existing fields...
	dimensions: string[];
}
```

- [ ] **Step 2: Load the assessment in `+page.server.ts`**

Add `assessmentRes` to the `Promise.allSettled` destructuring and to the fetch list:

```ts
const [
	typesRes,
	flagsRes,
	spacesRes,
	groupsRes,
	seriesRes,
	orgsRes,
	compTypesRes,
	dimTypesRes,
	visibilityRes,
	assessmentRes
] = await Promise.allSettled([
	apiList<ContentTypeMeta>(ctx, '/content-types'),
	apiGet<Record<string, boolean>>(ctx, '/feature-flags'),
	apiList<MeSpace>(ctx, '/me/spaces'),
	apiList<CustomFieldGroup>(ctx, '/custom-field-groups'),
	apiList<BlogSeries>(ctx, '/blog/series'),
	apiList<ComposerOrganization>(ctx, '/me/organizations'),
	apiList<ComparisonType>(ctx, '/jce/comparison-types?page_size=50'),
	apiList<{ value: string; label: string }>(ctx, '/jce/comparisons/dimension-types'),
	apiGet<{ value: Visibility; label: string }[]>(ctx, '/content/visibility-options'),
	apiGet<AssessmentForm>(ctx, '/assessment')
]);
```

Then add to the returned data:

```ts
const assessment = assessmentRes.status === 'fulfilled' ? assessmentRes.value : null;

return {
	// ...existing fields...
	assessmentDimensions: assessment?.dimensions ?? []
};
```

- [ ] **Step 3: Pass `assessmentDimensions` into `CreateShell`**

In `apps/main/src/routes/(app)/create/+page.svelte`, add `assessmentDimensions={data.assessmentDimensions}` to the existing `<CreateShell ... />` call.

- [ ] **Step 4: Run typecheck and verify no errors**

```bash
bun --cwd apps/main run check
```

Expected: PASS (may have pre-existing issues, but no new ones).

- [ ] **Step 5: Commit**

```bash
git add apps/main/src/lib/types/index.ts apps/main/src/routes/(app)/create/+page.server.ts apps/main/src/routes/(app)/create/+page.svelte
git commit -m "feat(create): load assessment dimensions into create page data"
```

---

### Task 4: Frontend — add `Roadmap dimension` and `Purpose` to the Journey panel

**Files:**
- Modify: `apps/main/src/lib/components/create/panel/CreateJourneyPanel.svelte`
- Modify: `apps/main/src/lib/components/create/panel/CreateRightPanel.svelte`
- Modify: `apps/main/src/lib/components/create/CreateShell.svelte`

**Interfaces:**
- Consumes: `assessmentDimensions: string[]`, `journeyOptions` already fetched inside `CreateJourneyPanel`.
- Produces: `roadmapDimension` and `roadmapPurpose` values sent through `ondimension` / `onpurpose` callbacks.

- [ ] **Step 1: Update `CreateJourneyPanel.svelte`**

Update the `Props` interface and destructure:

```ts
interface Props {
	journeyStage: string | null;
	visaRoute: string | null;
	sourceCountry: string | null;
	targetCountry: string | null;
	dimension: string | null;
	purpose: string | null;
	assessmentDimensions: string[];
	onjourney: (v: string | null) => void;
	onvisa: (v: string | null) => void;
	onsource: (v: string | null) => void;
	ontarget: (v: string | null) => void;
	ondimension: (v: string | null) => void;
	onpurpose: (v: string | null) => void;
	onedit: () => void;
}

let {
	journeyStage,
	visaRoute,
	sourceCountry,
	targetCountry,
	dimension,
	purpose,
	assessmentDimensions,
	onjourney,
	onvisa,
	onsource,
	ontarget,
	ondimension,
	onpurpose,
	onedit
}: Props = $props();
```

Derive option lists locally (purpose comes from the already-fetched `journeyOptions`):

```ts
let dimensionOptions = $derived(assessmentDimensions.map((d) => ({ value: d, label: d })));
let purposeOptions = $derived(options?.purposes ?? []);
```

Add handler helpers after the existing setters:

```ts
function setDimension(item: AutocompleteItem) {
	ondimension(item.id);
	onedit();
}

function setPurpose(item: AutocompleteItem) {
	onpurpose(item.id);
	onedit();
}

function setDimensionText(v: string) {
	const trimmed = v.trim() || null;
	ondimension(trimmed);
	onedit();
}
```

Add two field rows inside the `.journey-panel` div, after the existing four. Use the existing `Autocomplete` component and `optionFetcher`/`optionLabel`/`optionInitial` helpers:

```svelte
<div class="field-row">
	<label class="field-label" for="journey-dimension">{t('create.journey.dimension', 'Roadmap dimension')}</label>
	{#if dimensionOptions.length > 0}
		<Autocomplete
			id="journey-dimension"
			placeholder={t('create.journey.dimension_placeholder', 'Select a readiness dimension')}
			emptyMessage={t('journey.banner.no_options', 'No matching option')}
			fetcher={optionFetcher(dimensionOptions)}
			initialFetcher={() => optionInitial(dimensionOptions, dimension)}
			selectedId={dimension}
			selectedLabel={optionLabel(dimensionOptions, dimension ?? '')}
			onSelect={setDimension}
			onClear={() => { ondimension(null); onedit(); }}
			allowFreeText
		/>
	{:else}
		<input
			id="journey-dimension"
			type="text"
			class="field-input"
			placeholder={t('create.journey.dimension_placeholder', 'Select a readiness dimension')}
			value={dimension ?? ''}
			oninput={(e) => setDimensionText(e.currentTarget.value)}
		/>
	{/if}
</div>

{#if dimension === null || dimension === ''}
	<p class="journey-hint">{t('create.journey.dimension_nudge', 'Set a roadmap dimension so this guide can appear in readiness roadmaps.')}</p>
{/if}

<div class="field-row">
	<label class="field-label" for="journey-purpose">{t('create.journey.purpose', 'Purpose')}</label>
	{#if purposeOptions.length > 0}
		<Autocomplete
			id="journey-purpose"
			placeholder={t('create.journey.purpose_placeholder', 'Select a purpose')}
			emptyMessage={t('journey.banner.no_options', 'No matching option')}
			fetcher={optionFetcher(purposeOptions)}
			initialFetcher={() => optionInitial(purposeOptions, purpose)}
			selectedId={purpose}
			selectedLabel={optionLabel(purposeOptions, purpose ?? '')}
			onSelect={setPurpose}
			onClear={() => { onpurpose(null); onedit(); }}
		/>
	{:else}
		<input
			id="journey-purpose"
			type="text"
			class="field-input"
			placeholder={t('create.journey.purpose_placeholder', 'Select a purpose')}
			value={purpose ?? ''}
			oninput={(e) => { onpurpose(e.currentTarget.value.trim() || null); onedit(); }}
		/>
	{/if}
</div>
```

- [ ] **Step 2: Update `CreateRightPanel.svelte`**

Add to the `Props` interface and destructure:

```ts
interface Props {
	// ...existing props...
	dimension: string | null;
	purpose: string | null;
	assessmentDimensions: string[];
	ondimension: (v: string | null) => void;
	onpurpose: (v: string | null) => void;
}
```

Pass the new props into `CreateJourneyPanel`:

```svelte
<CreateJourneyPanel
	{journeyStage}
	{visaRoute}
	{sourceCountry}
	{targetCountry}
	{dimension}
	{purpose}
	{assessmentDimensions}
	onjourney
	onvisa
	onsource
	ontarget
	ondimension
	onpurpose
	onedit
/>
```

- [ ] **Step 3: Update `CreateShell.svelte`**

Add to the `Props` interface and destructure:

```ts
interface Props {
	// ...existing props...
	assessmentDimensions: string[];
}

let {
	// ...existing destructured props...
	assessmentDimensions
}: Props = $props();
```

Add runes after the existing journey runes:

```ts
let roadmapDimension = $state<string | null>(untrack(() => (draft?.field_values?.dimension as string) ?? null));
let roadmapPurpose = $state<string | null>(untrack(() => (draft?.field_values?.purpose as string) ?? null));
```

Update the `snapshot` `field_values` for guide content so `dimension`/`purpose` are only present when non-null:

```ts
field_values: {
	...activeFieldValues,
	...(isGuide && roadmapDimension ? { dimension: roadmapDimension } : {}),
	...(isGuide && roadmapPurpose ? { purpose: roadmapPurpose } : {})
}
```

Pass the new props to `CreateRightPanel`:

```svelte
<CreateRightPanel
	// ...existing props...
	dimension={roadmapDimension}
	purpose={roadmapPurpose}
	assessmentDimensions={assessmentDimensions}
	ondimension={(v) => { roadmapDimension = v; markDirty(); }}
	onpurpose={(v) => { roadmapPurpose = v; markDirty(); }}
/>
```

- [ ] **Step 4: Run typecheck and build**

```bash
bun --cwd apps/main run check
bun --cwd apps/main run build
```

Expected: typecheck and build pass (or only pre-existing issues).

- [ ] **Step 5: Commit**

```bash
git add apps/main/src/lib/components/create/CreateShell.svelte \
  apps/main/src/lib/components/create/panel/CreateJourneyPanel.svelte \
  apps/main/src/lib/components/create/panel/CreateRightPanel.svelte
git commit -m "feat(create): add roadmap dimension and purpose to journey panel"
```

---

### Task 5: Admin — remove the variants page

**Files:**
- Delete: `apps/admin/src/routes/tools/readiness/variants/+page.svelte`
- Modify: `apps/admin/src/routes/tools/readiness/+page.svelte`
- Modify: `apps/admin/src/lib/components/layout/sidebar.svelte` (if variants link exists)

**Interfaces:**
- Consumes: none.
- Produces: UI no longer surfaces the variants page.

- [ ] **Step 1: Delete the variants page**

```bash
git rm -r apps/admin/src/routes/tools/readiness/variants
```

- [ ] **Step 2: Remove the "Manage Variants" button and unused `canCreate` derived**

In `apps/admin/src/routes/tools/readiness/+page.svelte`, delete the `<a href="/tools/readiness/variants">` block (around the "Manage Variants" text). Then remove the `canCreate` derived, which is only used for that button:

```ts
// Remove this line:
const canCreate = $derived(hasPermission('content.create'));
```

Also remove the `{#if canCreate}` wrapper around the deleted link.

- [ ] **Step 3: Remove sidebar entry if present**

Check `apps/admin/src/lib/components/layout/sidebar.svelte` for any `/tools/readiness/variants` route. It does not currently exist, but remove it if it appears.

- [ ] **Step 4: Run typecheck and build**

```bash
bun --cwd apps/admin run check
bun --cwd apps/admin run build
```

Expected: no new errors.

- [ ] **Step 5: Commit**

```bash
git add apps/admin/src/routes/tools/readiness/+page.svelte
git commit -m "chore(admin): remove readiness roadmap variants management page"
```

---

### Task 6: Docs — update JEOS for the new workflow

**Files:**
- Modify: `.jeos/sections/71-readiness-assessment.md`
- Modify: `.jeos/adr/ADR-071-japa-readiness-assessment.md`

**Interfaces:**
- Consumes: the spec and current code behavior.
- Produces: JEOS/ADR text matching the implemented behavior.

**Important:** Per AGENTS.md, JEOS is always updated before code. Draft the section and ADR changes **before** merging the corresponding implementation tasks (Tasks 1–5). Do not let the docs and code diverge.

- [ ] **Step 1: Update Section 71**

Replace the "Placeholder variants and admin UI" section with a section describing the content-journey tagging workflow. State that:

- Authors tag guides with `dimension` in `field_values`, `purpose` in `field_values`, and journey columns at creation time.
- The scoring engine resolves each step to the highest-matching published guide in the corpus.
- The `/admin/tools/readiness/variants` page has been removed.

- [ ] **Step 2: Update ADR-071**

Update the section on roadmap content resolution to describe the new global match and tie-breaker. Remove references to the slug-family variant workflow as the required path. Keep migration `000587` as historical context (it seeded placeholder draft content, but the engine no longer requires it).

- [ ] **Step 3: Commit**

```bash
git add .jeos/sections/71-readiness-assessment.md .jeos/adr/ADR-071-japa-readiness-assessment.md
git commit -m "docs: update JEOS for content-journey readiness roadmap resolution"
```

---

### Task 7: Verification

- [ ] **Step 1: Go tests, build, and vet**

```bash
go test ./internal/assessment/...
go build ./...
go vet ./...
```

Expected: PASS and no build/vet errors.

- [ ] **Step 2: Frontend typecheck and build**

```bash
bun --cwd apps/main run check
bun --cwd apps/admin run check
bun --cwd apps/main run build
bun --cwd apps/admin run build
```

Expected: no new errors.

- [ ] **Step 3: Manual / Playwright smoke test**

Run Playwright if the project has an e2e suite (otherwise perform the steps manually in the running dev servers):

```bash
bun --cwd apps/main run test:e2e
```

Manual steps:
1. Create a guide with `dimension=financial`, `target_country=CA`, `source_country=NG`, `journey_stage=planning`, `visa_route=work`.
2. Submit a readiness assessment with matching context and a weak `financial` score.
3. Verify `/tools/readiness?result=<id>` resolves to the new guide.
4. Visit `/guides?tab=roadmap` with the same journey context; verify the guide appears and is ranked appropriately.

- [ ] **Step 4: Final commit and final `git status` check**

```bash
git status
```

Ensure only pre-existing, unrelated working-tree changes remain.

---

## Plan self-review

1. **Spec coverage:**
   - Backend global resolution — Task 2.
   - `GET /assessment` `dimensions` — Task 1.
   - Frontend `dimension`/`purpose` fields — Tasks 3 and 4.
   - Admin variants removal — Task 5.
   - JEOS/ADR updates — Task 6.
   - Verification — Task 7.

2. **Placeholder scan:** No `TBD`, `TODO`, or vague steps; code snippets and commands are provided. The frontend data-flow chain, the `CreateRightPanel` wiring, and the `CreateJourneyPanel` markup are now explicit.

3. **Type consistency:** `AssessmentForm`, `roadmapDimension`/`roadmapPurpose` state, `ondimension`/`onpurpose` callbacks, and `assessmentDimensions` prop passing match across `+page.server.ts`, `+page.svelte`, `CreateShell`, `CreateRightPanel`, and `CreateJourneyPanel`.

## Execution handoff

Plan complete and saved to `docs/superpowers/plans/2026-09-09-readiness-roadmap-content-journey-plan.md`.

Two execution options:

1. **Subagent-Driven (recommended)** — dispatch a fresh subagent per task, review between tasks, fast iteration. Required sub-skill: `superpowers:subagent-driven-development`.
2. **Inline Execution** — execute tasks in this session using `superpowers:executing-plans`, batch execution with checkpoints.

Which approach do you want?
