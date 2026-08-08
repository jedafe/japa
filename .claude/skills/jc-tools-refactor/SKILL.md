---
name: jc-tools-refactor
description: Use when refactoring or modernizing existing JapaConnect Tools Web Surface code (apps/tools, port 3005, tools.japaconnect.com). Covers Svelte 5 Runes migration, shell consolidation, LayerChart upgrades, and removing hardcoded data per Rule 16.
---

# JapaConnect Tools Surface — Refactor Skill

Refactoring guide for `apps/tools/` (§78, ADR-086). Use when modernizing existing tool pages, consolidating shell components, or migrating patterns.

## Surface Overview

SvelteKit SSR · Port 3005 · `tools.japaconnect.com`. Sixth SvelteKit app. Svelte 5 Runes, Bits UI, Tailwind v4, LayerChart 2.0.1, `lucide-svelte@^1.0.1`.

## Refactor Scenarios

### 1. Migrate Svelte 4 Patterns to Svelte 5 Runes

The Tools app was built with Svelte 5 Runes from the start, but if you encounter legacy patterns:

| Svelte 4 | Svelte 5 Runes |
|---|---|
| `let count = 0` | `let count = $state(0)` |
| `$: doubled = count * 2` | `const doubled = $derived(count * 2)` |
| `$: { effect() }` | `$effect(() => { ... })` |
| `export let prop` | `let { prop } = $props()` |
| `createEventDispatcher` | callback props (`let { onclick } = $props()`) |
| `<slot />` | `{@render children()}` (snippets) |
| `on:click` | `onclick` |
| `use:action` | `{@attach ...}` |

### 2. Consolidate Shell Components

If `HubHeader` and `SingleToolHeader` drift in structure, consolidate shared logic:
- Both render: logo/brand, theme toggle, auth links
- Difference: Hub shows "JapaConnect Tools" brand; Single-tool shows tool name + accent icon
- Extract a `ShellHeader` base component with a `variant` prop if divergence is minimal
- Keep them separate if the layouts diverge significantly (single-tool may have tool-specific nav)

### 3. Remove Hardcoded Data (Rule 16)

If a tool page contains hardcoded option lists, migrate to API-driven:

```typescript
// ❌ Before — hardcoded visa types
const visaTypes = ['student', 'work', 'tourist', 'business'];

// ✅ After — from API selector
const res = await apiGet<ChecklistSelector>(ctx, '/checklist/selector');
const visaTypes = res.destinations.find(d => d.id === selectedDest)?.visa_types ?? [];
```

Check for: country lists, currency lists, visa type lists, city lists, scholarship category lists, provider lists. All must come from the API.

### 4. LayerChart Upgrades

When upgrading LayerChart (currently pinned at 2.0.1):
1. Check the changelog for breaking changes
2. The remittance trend chart uses `LineChart` — verify the component API hasn't changed
3. Colors must stay `var(--chart-*)` (dark-mode safe)
4. Run the build after upgrade — LayerChart exports can change between minor versions
5. If `LineChart` API changes, update the remittance page's chart section

### 5. Migrate `$env/static/public` to `$env/dynamic/public`

Optional env vars (Umami site ID, API base URL) should use `$env/dynamic/public` to avoid build failures when vars aren't set:

```typescript
// ❌ Before — build fails if var not set
import { PUBLIC_JC_UMAMI_URL } from '$env/static/public';

// ✅ After — runtime check, build always succeeds
import { env } from '$env/dynamic/public';
const umamiEnabled = $derived(!!env.PUBLIC_JC_UMAMI_URL);
```

### 6. Extract Shared Registration Prompt Modal

The registration prompt modal is duplicated across visa-checklist, readiness, scholarships, and remittance. If it drifts, extract to a shared component:

```svelte
<!-- $lib/components/ui/RegisterPrompt.svelte -->
<script lang="ts">
  import { mainAppUrl } from '$lib/seo';
  let { open, onClose } = $props();
</script>
{#if open}
  <!-- modal markup -->
{/if}
```

### 7. Consolidate API Client Patterns

If tool pages duplicate API call patterns, extract helpers:
- `apiGet`/`apiPost`/`apiPatch`/`apiDelete` already exist in `$lib/api/client`
- Tool-specific helpers (e.g. `toggleChecklistItem`, `saveScholarship`) can be extracted to `$lib/tools/{tool}.ts`

### 8. Type Consolidation

If types are scattered or duplicated, consolidate in `src/lib/types/index.ts`:
- All tool-specific types (ChecklistSelector, AssessmentForm, ColCity, Scholarship, RemittanceRate, ChatConversation) live here
- Keep types aligned with the Go backend structs (check `api-routes-catalog.md` for response shapes)

### 9. Apply Geo-Aware Tool Pattern to Existing Tools (§§90–97, §§99–101)

When refactoring v1 tools or building v2 functional tools, apply the geo-aware tool pattern (`.jeos/skills/geo-aware-tool-pattern.md`):

- **Add `?dest=` filter** to all list/lookup API calls — destination country is the primary data axis
- **Remove hardcoded nationality** — replace any hardcoded country/nationality references with destination selector-driven API calls
- **Add destination selector as first UI input** — render a country selector before any tool-specific content; wire it to reactive `?dest=` param on all downstream API calls
- **Add `destination_country_id` to types** — ensure `src/lib/types/index.ts` includes `destination_country_id` in v2 tool content types
- **Handle feature-flag 404 (not 403)** — ADR-042: disabled features return 404; the Tools app should hide the tool card and show "coming soon" state on 404

```typescript
// ❌ Before — hardcoded nationality, no dest filter
const visaTypes = ['student', 'work', 'tourist'];
const res = await apiGet(ctx, '/visa-routes/compare');

// ✅ After — geo-aware: dest selector drives API calls
let selectedDest = $state<string | null>(null);
const res = $derived(await apiGet(ctx, `/visa-routes/compare?dest=${selectedDest}`));
```

### 10. Add ADR-088 AI Generation Columns to Tool Content Tables

When refactoring tool content types or API response handling, ensure ADR-088 compliance:

- Add `ai_generated` (boolean), `ai_model` (string), `hitl_reviewed_by` (string | null), `ai_generated_at` (string | null) to tool content TypeScript types
- Add `is_japabot_indexed` (boolean) computed column to types where applicable
- Filter API responses to only show `hitl_reviewed_by IS NOT NULL` content (HITL review mandatory before publish)
- AI content generation goes through the §20 AI Gateway — no direct LLM calls from the Tools app

### 11. Migrate V2 Tool Pages to Svelte 5 Runes

V2 functional tool pages should use Svelte 5 Runes patterns from the start. If any v2 tool page uses legacy patterns, apply the migration table from §1. Key patterns for v2 tools:

- Destination selector state: `let selectedDest = $state<string | null>(null)`
- Derived API response: `let toolData = $derived.by(async () => { ... })` or `$effect` for async fetch
- Reactive `?dest=` param: `const apiUrl = $derived(`/api/v1/visa-routes/compare?dest=${selectedDest}`)`

## Refactor Order

1. **Read the section file** (§70-§74, §78, §§90–97, §§99–101) — understand the spec before refactoring
2. **Run the audit skill** (`jc-tools-audit`) — identify what needs refactoring
3. **Make changes incrementally** — one tool page at a time
4. **Build after each change** — `bun run build` catches type and template errors
5. **Verify both shell modes** — hub and single-tool — still render correctly
6. **Check Rule 16 and Rule 17** — no hardcoded data, no AI attribution

## See Also

ADR-086, §78, `ROADMAP-TOOLS.md`, ADR-088 (AI content generation), ADR-042 (feature-flag 404), `.jeos/skills/geo-aware-tool-pattern.md`, §§90–97 §§99–101 (V2 functional tools), `jc-tools-develop`, `jc-tools-audit`, `jc-tools-debug`, `jc-main-refactor` (Svelte 4→5 migration patterns).
