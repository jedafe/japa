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
| `internal/assessment/handlers_test.go` or `service_test.go` | Tests for `dimensions` output and `resolveRoadmapContent` global resolution. |
| `apps/main/src/routes/(app)/create/+page.server.ts` | Load `GET /assessment` and pass `assessmentDimensions` to the page. |
| `apps/main/src/lib/components/create/panel/CreateJourneyPanel.svelte` | Render `Roadmap dimension` and `Purpose` selects. |
| `apps/main/src/lib/components/create/CreateShell.svelte` | Manage `roadmapDimension`/`roadmapPurpose` state and merge them into `field_values`. |
| `apps/main/src/lib/types.ts` (if needed) | Add `Assessment`/`AssessmentDimensions` types. |
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
- Test: `internal/assessment/handlers_test.go`

**Interfaces:**
- Consumes: `AssessmentFormDTO.Fields[].Config.Dimension`, `ScoringConfig.CuratedRoadmap`, fallback `curatedRoadmapLibrary()`.
- Produces: `data.dimensions []string` in `GET /assessment` response.

- [ ] **Step 1: Write the failing test**

In `internal/assessment/handlers_test.go`:

```go
func TestHandleGetAssessment_IncludesDimensions(t *testing.T) {
	// Arrange: build a minimal form with two field dimensions and a default library.
	form := buildActiveForm(t, []string{"financial", "logistical"})

	// Act: call the handler and parse response.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/assessment", nil)
	// tenant context and service wiring omitted; follow existing handler-test pattern.

	h.HandleGetAssessment(rec, req)

	// Assert: response contains dimensions from fields and default curated library.
	var body struct {
		Data struct {
			Dimensions []string `json:"dimensions"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Contains(t, body.Data.Dimensions, "financial")
	assert.Contains(t, body.Data.Dimensions, "logistical")
	assert.Contains(t, body.Data.Dimensions, "language") // from default library
	assert.NotContains(t, body.Data.Dimensions, "")      // no empty dimensions
}
```

- [ ] **Step 2: Run the test and confirm it fails**

```bash
go test ./internal/assessment/... -run TestHandleGetAssessment_IncludesDimensions -v
```

Expected: FAIL with `dimensions` field missing or empty.

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
git add internal/assessment/service.go internal/assessment/handlers.go internal/assessment/handlers_test.go
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

In `internal/assessment/service_test.go`:

```go
func TestResolveRoadmapContent_PrefersNonFamilyGuideWithBetterMatch(t *testing.T) {
	// Setup: one generic guide at slug "guide-generic-finance" with no context,
	// and one specific guide at slug "canada-study-finance" with target_country=CA, visa_route=student.
	// Visitor context: destination=CA, visa=student.
	// The specific guide is not a variant of the generic slug family.
	steps := []RoadmapStep{
		{Title: "Build savings", ContentSlug: "guide-generic-finance", Dimension: "financial"},
	}

	svc := newTestService(t)
	ctx := context.Background()

	journey := JourneyContext{
		DestinationCountry: "CA",
		VisaRoute:          "student",
		JourneyStage:       "planning",
	}

	resolved := svc.resolveRoadmapContent(ctx, testTenantID, steps, journey)

	require.Len(t, resolved, 1)
	assert.Equal(t, "canada-study-finance", resolved[0].ContentSlug,
		"a guide with the same dimension and a stronger journey match should win even if not a slug-family variant")
}

func TestResolveRoadmapContent_KeepsOriginalOnTie(t *testing.T) {
	// Setup: generic guide and another guide both have the same context score
	// because both only match the journey_stage. The original curated slug must win.
	steps := []RoadmapStep{
		{Title: "Build savings", ContentSlug: "guide-generic-finance", Dimension: "financial"},
	}

	svc := newTestService(t)
	ctx := context.Background()
	journey := JourneyContext{JourneyStage: "planning"}

	resolved := svc.resolveRoadmapContent(ctx, testTenantID, steps, journey)
	require.Len(t, resolved, 1)
	assert.Equal(t, "guide-generic-finance", resolved[0].ContentSlug,
		"when scores tie, the original curated slug is kept")
}
```

- [ ] **Step 2: Run the test and confirm it fails**

```bash
go test ./internal/assessment/... -run TestResolveRoadmapContent -v
```

Expected: FAIL because the slug-family guard rejects the non-family guide.

- [ ] **Step 3: Modify `resolveRoadmapContent` in `internal/assessment/service.go`**

Inside the candidate loop, replace:

```go
if row.Slug != base && !strings.HasPrefix(row.Slug, base+"-") {
    continue
}
```

with the tie-breaker logic:

```go
if score == bestScore && bestScore > 0 && row.Slug == base && best.Slug != base {
    // Prefer the original curated slug when the match is equally specific.
    best = row
    bestScore = score
}

if score > bestScore {
    best = row
    bestScore = score
}
```

Leave the existing score calculation and the `bestScore > 0` replacement gate unchanged.

- [ ] **Step 4: Run the test and confirm it passes**

```bash
go test ./internal/assessment/... -run TestResolveRoadmapContent -v
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
- Modify: `apps/main/src/routes/(app)/create/+page.server.ts`
- Modify: `apps/main/src/lib/types.ts` (if needed)

**Interfaces:**
- Consumes: `GET /assessment` response `dimensions`.
- Produces: `page.data.assessmentDimensions: string[]`.

- [ ] **Step 1: Add `assessment` type and load call**

In `apps/main/src/lib/types.ts` (or a new `assessment.ts` type file if preferred), add:

```ts
export interface AssessmentForm {
	id: string;
	name: string;
	slug: string;
	description: string;
	is_multi_step: boolean;
	submit_label: string;
	status: string;
	max_score: number;
	thresholds: Array<{ min: number; max: number; label: string; color: string }>;
	fields: Array<{
		id: string;
		label: string;
		key: string;
		field_type: string;
		is_required: boolean;
		position: number;
		placeholder: string;
		options: unknown;
		config: { weight: number; dimension: string };
	}>;
	dimensions: string[];
}
```

In `apps/main/src/routes/(app)/create/+page.server.ts`, add to the parallel fetch:

```ts
const [assessmentRes] = await Promise.allSettled([
	// ...existing calls...
	apiGet<AssessmentForm>(ctx, '/assessment')
]);
```

Then include in the returned data:

```ts
const assessment = assessmentRes.status === 'fulfilled' ? assessmentRes.value : null;

return {
	// ...existing fields...
	assessmentDimensions: assessment?.dimensions ?? []
};
```

- [ ] **Step 2: Run typecheck and verify no errors**

```bash
bun run check --cwd apps/main
```

Expected: PASS (may have pre-existing issues, but no new ones).

- [ ] **Step 3: Commit**

```bash
git add apps/main/src/routes/(app)/create/+page.server.ts apps/main/src/lib/types.ts
git commit -m "feat(create): load assessment dimensions into create page data"
```

---

### Task 4: Frontend — add `Roadmap dimension` and `Purpose` to the Journey panel

**Files:**
- Modify: `apps/main/src/lib/components/create/panel/CreateJourneyPanel.svelte`
- Modify: `apps/main/src/lib/components/create/CreateShell.svelte`

**Interfaces:**
- Consumes: `assessmentDimensions: string[]`, `purposeOptions: JourneyOption[]`, `journeyOptions` already fetched.
- Produces: `roadmapDimension` and `roadmapPurpose` values sent through `onroadmapdimension` / `onroadmappurpose` callbacks.

- [ ] **Step 1: Update `CreateJourneyPanel.svelte` props and render**

Add to the `Props` interface:

```ts
interface Props {
	journeyStage: string | null;
	visaRoute: string | null;
	sourceCountry: string | null;
	targetCountry: string | null;
	dimension: string | null;
	purpose: string | null;
	assessmentDimensions: string[];
	purposeOptions?: { value: string; label: string }[];
	onjourney: (v: string | null) => void;
	onvisa: (v: string | null) => void;
	onsource: (v: string | null) => void;
	ontarget: (v: string | null) => void;
	ondimension: (v: string | null) => void;
	onpurpose: (v: string | null) => void;
	onedit: () => void;
}
```

Add two field rows after the existing four. For `Roadmap dimension` use a select with the `assessmentDimensions` array. For `Purpose` use the existing `optionFetcher` pattern with `purposeOptions`. Add a small nudge label when `dimension` is empty:

```svelte
{#if dimension === null || dimension === ''}
	<p class="journey-hint">{t('create.journey.dimension_nudge', 'Set a roadmap dimension so this guide can appear in readiness roadmaps.')}</p>
{/if}
```

- [ ] **Step 2: Update `CreateShell.svelte`**

Add runes after the existing journey runes:

```ts
let roadmapDimension = $state<string | null>(untrack(() => (draft?.field_values?.dimension as string) ?? null));
let roadmapPurpose = $state<string | null>(untrack(() => (draft?.field_values?.purpose as string) ?? null));
```

Update the `snapshot` `field_values` for guide content:

```ts
field_values: {
	...activeFieldValues,
	dimension: isGuide ? roadmapDimension : null,
	purpose: isGuide ? roadmapPurpose : null
}
```

Pass the new props through `CreateRightPanel` and then into `CreateJourneyPanel`:

```svelte
<CreateJourneyPanel
	{journeyStage}
	{visaRoute}
	{sourceCountry}
	{targetCountry}
	dimension={roadmapDimension}
	purpose={roadmapPurpose}
	assessmentDimensions={data.assessmentDimensions}
	purposeOptions={journeyOptions?.purposes ?? []}
	onjourney={(v) => { journeyStage = v; markDirty(); }}
	onvisa={(v) => { visaRoute = v; markDirty(); }}
	onsource={(v) => { sourceCountry = v; markDirty(); }}
	ontarget={(v) => { targetCountry = v; markDirty(); }}
	ondimension={(v) => { roadmapDimension = v; markDirty(); }}
	onpurpose={(v) => { roadmapPurpose = v; markDirty(); }}
	onedit={markDirty}
/>
```

Also wire through `CreateRightPanel` props.

- [ ] **Step 3: Run typecheck and build**

```bash
bun run check --cwd apps/main
bun run build --cwd apps/main
```

Expected: typecheck and build pass (or only pre-existing issues).

- [ ] **Step 4: Commit**

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
rm apps/admin/src/routes/tools/readiness/variants/+page.svelte
rmdir apps/admin/src/routes/tools/readiness/variants
```

- [ ] **Step 2: Remove the "Manage Variants" button**

In `apps/admin/src/routes/tools/readiness/+page.svelte`, delete the button/link that opens the variants page (around the "Manage Variants" text). Ensure the remaining page still works.

- [ ] **Step 3: Remove sidebar entry if present**

Check `apps/admin/src/lib/components/layout/sidebar.svelte` for any `/tools/readiness/variants` route. Remove it if it exists.

- [ ] **Step 4: Run typecheck and build**

```bash
bun run check --cwd apps/admin
bun run build --cwd apps/admin
```

Expected: no new errors.

- [ ] **Step 5: Commit**

```bash
git add -A apps/admin/src/routes/tools/readiness/variants apps/admin/src/routes/tools/readiness/+page.svelte
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

- [ ] **Step 1: Go tests**

```bash
go test ./internal/assessment/...
```

Expected: PASS.

- [ ] **Step 2: Frontend typecheck and build**

```bash
bun run check --cwd apps/main
bun run check --cwd apps/admin
bun run build --cwd apps/main
bun run build --cwd apps/admin
```

Expected: no new errors.

- [ ] **Step 3: Manual / Playwright smoke test**

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

2. **Placeholder scan:** No `TBD`, `TODO`, or vague steps; code snippets and commands are provided.

3. **Type consistency:** `AssessmentForm`, `roadmapDimension`/`roadmapPurpose` state, and `ondimension`/`onpurpose` callbacks match the component interfaces defined in Task 4.

## Execution handoff

Plan complete and saved to `docs/superpowers/plans/2026-09-09-readiness-roadmap-content-journey-plan.md`.

Two execution options:

1. **Subagent-Driven (recommended)** — dispatch a fresh subagent per task, review between tasks, fast iteration. Required sub-skill: `superpowers:subagent-driven-development`.
2. **Inline Execution** — execute tasks in this session using `superpowers:executing-plans`, batch execution with checkpoints.

Which approach do you want?
