# Design — Readiness Roadmap Resolution from Content Journey Metadata

**Date:** 2026-09-09  
**Status:** Draft — pending review  
**JEOS refs:** §71 (readiness assessment), §14 (content type system), §32 (activity feed / timeline)  
**ADR refs:** ADR-071 — Japa Readiness Assessment  
**Related specs:** `docs/superpowers/specs/2026-08-21-journey-aware-recommendations-design.md`

## Problem

The Japa Readiness Assessment currently uses a dedicated admin page at `/admin/tools/readiness/variants` to create origin/destination-specific roadmap guide variants. The page:

- Creates a new `contents` row for every origin→destination pair, copying the base guide body and generating a slug like `<base>-<dest>-<origin>`.
- Tags the row with `guide_category='roadmap'`, `field_values->>'dimension'`, `target_country`, and `source_country`.
- Produces many `draft` placeholder rows that the editorial team must later review and fill.

This is overkill for the actual goal. The main content composer already supports journey context (`journey_stage`, `visa_route`, `source_country`, `target_country`) through its Journey panel, and the API accepts those fields plus `field_values`. What is missing is the readiness `dimension` (and optional `purpose`) tag. As a result, the variants page is the only place where a guide can be marked as addressing a specific readiness dimension.

## Goal

Remove the `/admin/tools/readiness/variants` admin page and its workflow. Guide authors instead tag a guide with its readiness `dimension` and journey context at creation or edit time. The readiness result page (`/tools/readiness?result=[id]`) and the guides roadmap hub (`/guides?tab=roadmap`) then resolve the most relevant published guide from the whole corpus using those tags.

## Decisions from clarification

- **Resolution is global by `dimension` + journey context**, not restricted to the curated step’s slug family (Option A).
- **`dimension` is the primary key** that links a guide to a readiness weakness. A guide must have a `dimension` to be eligible for roadmap resolution, but the column itself remains optional in the data model (no DB-level constraint). The UI should warn/nudge authors to set it for guide content, not block publishing.
- **`purpose` is optional** but supported, since the existing `FindRoadmapContentByDimension` query already considers it.
- The **main content composer’s Journey tab** is the canonical place to set these fields. No dedicated admin CRUD page remains.
- Existing `draft` placeholder rows from migration `000587` can remain; they are ignored because all resolution queries require `status='published'`. A cleanup migration is out of scope unless requested.
- `sections/71-readiness-assessment.md` and `ADR-071-japa-readiness-assessment.md` must be updated to remove variant-page references and describe the new content-journey tagging workflow.
- **Dimension option source (Option B)** — the `GET /assessment` response is extended with a `dimensions` array, computed as the union of `fields[].config.dimension` and the active/default curated roadmap library keys. The create Journey panel consumes this array.
- **Tie-breaker** — when two guides have the same context score, the original curated `content_slug` wins, so the engine only swaps when there is a genuine improvement.

## Domain model

A guide that may be recommended by the readiness assessment is described by:

| Field | Meaning | Where stored |
|---|---|---|
| `dimension` | The readiness dimension this guide addresses (e.g. `financial`, `logistical`, `professional`) | `contents.field_values->>'dimension'` |
| `purpose` | The travel purpose this guide targets (e.g. `work`, `study`, `family`) | `contents.field_values->>'purpose'` |
| `journey_stage` | The canonical Japa stage this guide targets | `contents.journey_stage` |
| `visa_route` | The visa route this guide targets | `contents.visa_route` |
| `source_country` | The origin country this guide is for | `contents.source_country` |
| `target_country` | The destination country this guide is about | `contents.target_country` |

The assessment result has:

- `dimension_scores` — the user’s per-dimension score.
- `roadmap` — an ordered list of `RoadmapStep` items, each with a `dimension` and a default `content_slug`.
- `journey_context` — `origin_country`, `destination_country`, `journey_stage`, `visa_route`, `purpose`.

For each weak dimension, the scoring engine looks up the curated step, then tries to resolve that step to a published guide whose `dimension` matches and whose journey-context fields best match the user’s context.

### Match scoring

The existing `resolveRoadmapContent` uses these weights, which remain unchanged:

| Match | Weight |
|---|---|
| `target_country == destination_country` | 4 |
| `journey_stage == user's stage` | 2 |
| `visa_route == user's visa route` | 2 |
| `source_country == origin_country` | 1 |
| `purpose == user's purpose` | 1 |

The highest-scoring published guide wins. If no guide scores > 0, the original curated step is kept.

## Architecture changes

### 1. Backend — relax `resolveRoadmapContent`

File: `internal/assessment/service.go`

- Remove the slug-family guard that currently restricts candidates to the step’s base slug or a `<base>-<suffix>` variant.
- Keep the match-score logic and the “no match → keep original step” fallback.
- Add a tie-breaker: if two guides have the same score, prefer the original curated `content_slug` so the engine only swaps when there is a genuine improvement.

This lets `FindRoadmapContentByDimension` return the best matching guide from the whole corpus, not just variants of the original.

### 2. Backend — query already supports global resolution

`FindRoadmapContentByDimension` in `internal/db/queries/contents.sql` already selects any published guide by `field_values->>'dimension'` and the visitor’s journey context. No query change is required.

### 3. Frontend — add `dimension` and `purpose` to the create Journey panel

Files:

- `apps/main/src/lib/components/create/panel/CreateJourneyPanel.svelte`
- `apps/main/src/lib/components/create/CreateShell.svelte`

Add two new controls to the Journey tab when the content type is `guide`:

1. **Roadmap dimension** (necessary for roadmap inclusion, but not a hard publish requirement): a select/autocomplete populated from the `GET /assessment` response’s `dimensions` array. The backend returns the union of the active form’s `fields[].config.dimension` values and the active/default curated roadmap library keys, so the list always matches the dimensions the assessment can actually score.
2. **Purpose** (optional): a select populated from `GET /feed/journey-options`.

`CreateShell` should:

- Add `roadmapDimension` and `roadmapPurpose` runes, seeded from `draft?.field_values?.dimension` and `draft?.field_values?.purpose`.
- Merge them into the `field_values` snapshot for `guide` content:

```ts
field_values: {
  ...activeFieldValues,
  dimension: roadmapDimension,
  purpose: roadmapPurpose
}
```

They must be merged explicitly because they are not part of any custom field group and would otherwise be stripped by `activeFieldValues`.

### 4. Admin — remove the variants page

Files:

- `apps/admin/src/routes/tools/readiness/variants/+page.svelte` — delete.
- `apps/admin/src/routes/tools/readiness/+page.svelte` — remove the “Manage Variants” button.
- `apps/admin/src/lib/components/layout/sidebar.svelte` — remove the variants entry if one exists.
- `apps/admin/src/lib/locales/en.json` and `fr.json` — remove the `readiness.variants.*` keys (can be done lazily).

### 5. Docs — update JEOS

Files:

- `.jeos/sections/71-readiness-assessment.md`
- `.jeos/adr/ADR-071-japa-readiness-assessment.md`

- Remove the “Placeholder variants and the admin UI” section.
- Remove references to `/admin/tools/readiness/variants` and migration `000587` as a required workflow.
- Describe the new workflow: guide authors tag content with `dimension` and journey context, and the assessment resolves from the whole published corpus.
- Keep migration `000587` as historical context (it seeded placeholder data) but note that the scoring engine no longer depends on slug-family variants.

### 6. Migrations

No new migration is required for the resolution logic change.

An optional future migration could clean up the `draft` placeholder rows created by `000587_roadmap_geo_variants_seed.up.sql`. Those rows are currently harmless because resolution only returns `status='published'` content. This is out of scope unless the user requests it.

## API behavior

- `POST /content` and `PATCH /content/{id}` already accept `field_values` and the `journey_*` columns. No API changes are needed.
- `GET /assessment` returns a `dimensions` array in addition to the form fields and thresholds. The array is the union of `fields[].config.dimension` and the active/default curated roadmap library keys, and is used by the create Journey panel.
- `GET /assessment/result/{id}/roadmap` continues to return the ranked roadmap; internally it now uses global dimension+context resolution.
- `GET /content?type=guide&sort=roadmap` continues to work unchanged; it already ranks by journey context.

## UI changes

- Main content composer (`/create?type=guide`): Journey tab gains **Roadmap dimension** and **Purpose** fields.
- Admin readiness results page: remove the **Manage Variants** button.
- Admin variants page: removed entirely.

## Catalog updates

- `api-routes-catalog.md` — no new routes; the variants page was an admin UI surface, not an API route.
- `system-settings-catalog.md` — no new settings.
- `valkey-keys-catalog.md` — no new Valkey keys.
- `asynq-tasks.md` — no new tasks.
- `permission-key-naming.md` — no new permission nodes; the admin page used `content.create/edit_any/delete_any`.

## Testing plan

- `go test ./internal/assessment/...` — update or add tests for `resolveRoadmapContent` selecting a non-family guide when it has a better context match, and keeping the original step when no better guide exists.
- `bun run check` in `apps/main` — typecheck the create page changes.
- `bun run check` in `apps/admin` after removing the variants page.
- `go build ./...` and `go vet ./...`.
- Manual / Playwright:
  1. Create a guide with `dimension=financial`, `target_country=CA`, `source_country=NG`, `journey_stage=planning`, `visa_route=work`.
  2. Submit a readiness assessment with matching context and a weak `financial` score.
  3. Verify the result roadmap resolves to the new guide instead of the generic `guide-banking-finance-relocation`.
  4. Visit `/guides?tab=roadmap` as a user with the same journey context; verify the guide appears and is ranked appropriately.

## Open questions

All clarification questions resolved before this design was written.

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| Removing the slug-family guard can cause a step to resolve to an unintended guide if multiple guides share the same `dimension` and similar context. | Add a tie-breaker for the original slug and rely on editorial quality of `dimension` tagging. |
| Authors may not tag guides with `dimension`, causing the engine to fall back to the generic step. | Add the `dimension` selector prominently in the Journey tab and nudge/guide the author. |
| Existing `000587` draft placeholder rows may confuse content editors. | They are ignored at runtime; an optional cleanup migration can be written later. |
