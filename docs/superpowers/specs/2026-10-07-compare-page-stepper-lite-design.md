# /compare Page Redesign — Stepper-Lite Flow

**Date:** 2026-10-07
**Status:** Design
**Scope:** `apps/main` only — `src/routes/(app)/compare/+page.svelte`, `src/lib/components/compare/`, `src/lib/i18n` locale files. No backend, API contract, or `+page.server.ts` changes.

## 1. Problem

`/compare` (the JCE item picker, §30) renders every control surface stacked vertically before the item grid: type button cloud (~9+ types), "source" selector, geo scope chain, selected-items panel, AI Quick panel, search input, nested-type select, sort select, page-size select. Results:

- **Cognitive overload** — up to seven distinct control surfaces before a single item is visible.
- **Internal vocabulary leaks** — "source" (item_ref_type) and "nested comparisons" are data-model terms, not user language.
- **Selection feedback is lost** — the selected-items panel scrolls away; the Compare button only appears inside it at 2+ selections.
- **Required scopes dead-end** — types like `courses` (scope `university`) render an empty grid with no guidance on what to do.
- **No discovery surface** — a guest landing on `/compare` sees a wall of type buttons, not what comparisons are possible or popular.

## 2. Goal

Reorganize `/compare` into a guided-but-flat flow — "stepper-lite": a landing mode for discovery, a workspace mode for picking, and a persistent selection tray that carries the Compare action. Every step collapses as it completes; no discrete wizard screens, no next/back buttons, no extra clicks for power users.

### Non-goals

- Backend changes — `/jce/*` endpoints, resolver registry, URL param contract all unchanged. `+page.server.ts` gains one landing-mode fetch (§3.2); everything else in it is untouched.
- `/compare/[slug]`, `/comparisons`, `/comparisons/[slug]` pages — out of scope.
- Removing functionality — nested comparisons, guest tokens, AI Quick Compare, "mine" tab, sort/page-size all remain reachable.

## 3. Design

### 3.1 Two modes driven by URL

The page renders one of two modes based on `?type=`:

| Mode | Condition | Content |
|---|---|---|
| Landing | no `type` param | type cards + popular comparisons + "mine" tab |
| Workspace | `type` param resolves to a type | step header + narrow panel + filter row + grid + tray |

Existing deep links (`?type=…&preselect=…`, `?compare_token=…`, scope params) land directly in workspace at the appropriate step state — the ~10 site features linking into `/compare` are unaffected.

### 3.2 Landing mode

```
┌─────────────────────────────────────────────┐
│ Tabs: [ New comparison | My comparisons ]   │  (authed only)
│                                             │
│ "What do you want to compare?"              │
│ ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐         │
│ │ type │ │ type │ │ type │ │ type │  …      │  type cards grid
│ │ card │ │ card │ │ card │ │ card │         │
│ └──────┘ └──────┘ └──────┘ └──────┘         │
│                                             │
│ Popular comparisons  → See all              │  strip of published
│ ┌────────┐ ┌────────┐ ┌────────┐            │  comparisons (reuse
│ │  card  │ │  card  │ │  card  │            │  /jce/comparisons
│ └────────┘ └────────┘ └────────┘            │  published+public)
└─────────────────────────────────────────────┘
```

- **Type cards** replace the button cloud: icon (from `type.icon`, fallback per-type lucide map), name, `description`, and `item_count` when present. Whole card is the click target → `?type={slug}`.
- **Popular comparisons strip** — SSR-loaded via `GET /jce/comparisons?filter_depth=detailed&filter_status=published&filter_visibility=public&page_size=6`, same call `/comparisons` uses. Hidden when empty or errored. "See all" links to `/comparisons`.
- **"Mine" tab** stays at landing level (tabs only render in landing mode).

### 3.3 Workspace step header

A slim breadcrumb-style indicator pinned at the top of workspace mode:

```
[ 1. Universities  ✕ ]  →  [ 2. Narrow: Nigeria · Lagos  ✎ ]  →  [ 3. Select 2–4 ]
```

- **Step 1 chip** — selected type name; `✕`/click returns to landing (`?type` removed).
- **Step 2 chip** — rendered only when the type has a `scope_chain`. Shows summary of selections ("Nigeria · Lagos") or "Narrow (optional)"/"Choose a university" when empty. Click re-expands the narrow panel.
- **Step 3 chip** — "Select 2–{max}"; shows live selected count.
- Chips are state indicators + affordances, not numbered chrome. Required-scope types label step 2 without "(optional)".

### 3.4 Narrow panel (B's step 2, absorbed)

For scoped types, the scope chain renders inside a distinct panel placed between the step header and the filter row:

- **Auto-expanded** on workspace entry when the type has a scope chain and no scope params are set. Titled e.g. "Narrow your options"; contains the existing `ScopeChain` selectors plus a **Skip** action when the scope is optional.
- **Collapses** to nothing (its state surfaces in the step-2 chip) after the user picks any scope value or presses Skip. Collapsed state is component state only — a fresh page load re-expands the panel if the type is scoped and no scope params are set. The step-2 chip re-opens it at any time.
- **Required scopes** (`source === 'course'` without a university — the existing special case): no Skip action; the item grid below renders a prompt state using the missing step's own label ("Choose a {step label} to see {item plural}") instead of an empty result grid.
- Non-scoped types never render the panel or the step-2 chip.

### 3.5 Unified filter row

One row replaces the scattered controls:

```
[🔍 Search universities........]        [entity ⌄]  [Sort ▾]
```

- **Search** — primary, widest element; existing 300 ms debounced server-side `q` unchanged.
- **Entity/source select** — only when `itemRefTypes.length > 1`. Relabelled: the entity source is the default option, `comparison` renders as "Existing comparisons". No standalone "Select source" section.
- **Sort select** — as today. **Page-size select moves into the same popover** as a "Per page" group. Nested-type filter (when `source === 'comparison'`) also lives here as an option group.
- Results count stays as the small caption line under the row.

### 3.6 Sticky compare tray

A fixed bottom bar in workspace mode — the persistent "finish" affordance:

```
┌──────────────────────────────────────────────────┐
│ [Lagos ✕] [Accra ✕]        2/4 selected  [✦ AI] │
│                        [Compare →]               │
└──────────────────────────────────────────────────┘
```

- Always rendered in workspace (even at 0 selections, showing "Pick 2–4 items"); chips appear as items are selected; Compare button enables at ≥ 2.
- **Guests** see "Log in to compare" in the same slot → existing `createGuestSelection` → `/login?redirect=` flow unchanged.
- **AI Quick Compare** becomes a secondary tray button (when `feature.jce.ai_quick` + `agent.use`), opening the existing form inline above the tray rather than in a mid-page panel.
- Mobile: the tray is a bottom sheet fixed above the mobile tab bar safe area.

### 3.7 Item cards

Same card grid; selection affordance clarified:

- Unselected cards show a subtle "+ Add" affordance on hover/focus instead of relying on border-color change alone.
- Selected cards keep the check state.
- When `maxItems` is reached, unselected cards render the existing disabled treatment **plus** a one-time hint in the tray ("Max {max} items — remove one to change picks"), replacing the bare `opacity-40` that reads as broken.

## 4. State matrix

| Entry | Renders as |
|---|---|
| `/compare` | Landing |
| `/compare?type=X` | Workspace, narrow panel auto-open if scoped, step 3 active |
| `/compare?type=X&country_id=…` | Workspace, narrow collapsed (summary chip), grid filtered |
| `/compare?type=X&preselect=…` | Workspace, tray pre-populated, narrow collapsed |
| `/compare?type=courses` (no university) | Workspace, narrow panel open w/o Skip, grid = prompt state |
| `/compare?type=mentors` (flag off) | 404 (unchanged) |
| `?compare_token=` + logged in | unchanged: server consumes token, redirects to `/compare/{slug}` |

## 5. Component changes

| Unit | Change |
|---|---|
| `compare/+page.svelte` | Restructure into landing/workspace branches; step-header state; tray mount. Script logic (URL sync, selection, API calls) largely preserved. |
| `compare/StepHeader.svelte` *(new)* | Three-chip step indicator; emits change-type / toggle-narrow events. |
| `compare/NarrowPanel.svelte` *(new)* | Wraps `ScopeChain`; handles expand/collapse, skip, required-mode title; renders the grid prompt state via snippet/prop flag. |
| `compare/CompareTray.svelte` *(new)* | Sticky bottom bar: chips, count, Compare/Login/AI actions. |
| `compare/TypeCardGrid.svelte` *(new)* | Landing type cards. |
| `compare/ScopeChain.svelte` | Unchanged — consumed by `NarrowPanel`. |
| `compare/+page.server.ts` | Add the popular-comparisons fetch for landing mode only; all else unchanged. |
| `src/lib/i18n` locale files | New keys under `jce.compare.*` for step labels, narrow panel, tray, prompt states. |

## 6. Edge cases & error handling

- **No types available** (feature flag off / API error) → existing error banner pattern on landing; type grid hidden.
- **Zero items after filtering** → existing empty-state card, kept verbatim.
- **Selection limit reached** → tray hint (3.7); cards disabled as today.
- **Guest flow** → tray CTA triggers existing `createGuestSelection` + login redirect; no behavior change.
- **`mine=true`** → landing "My comparisons" content (existing cards/actions unchanged).
- **Nested source selected** → grid renders the existing comparison-link cards; tray/pick UI hidden since nested comparisons link out rather than being picked. This matches current behavior where the selected panel is suppressed for `source === 'comparison'`.

## 7. Testing

- `bun run check` + `bunx svelte-check` clean on touched files.
- Manual matrix per §4: each entry state, optional vs required scope, guest vs authed, `mine` tab, nested source.
- i18n: no hardcoded strings (Rule 16 / Rule 3); all labels via `t()`.
- Verify mobile tray clears the bottom tab bar and doesn't occlude the pagination row.

## 8. Rollout notes

- No flag needed — pure frontend reorganization on the same URL contract; revert is a single-file class of change.
- Follow-on candidates (not in scope): promoting Popular comparisons into a server-ranked "trending" order; remember collapsed narrow state per type in `localStorage`.
