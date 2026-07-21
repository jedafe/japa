# User Hub — Plan 3: Dashboard Family

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development (recommended) or superpowers:executing-plans. Steps use `- [ ]`. Load **jc-main-develop**, **svelte**, and **svelte-layerchart** (for the blocked analytics chart wrappers). **Depends on Plan 1** (registry pattern, `optimisticAction`, shell integration). Independent of Plan 2.

**Goal:** Ship the `/dashboard` activity family — overview, content manager, relationships, and the ADR-060 learning/certificates/knowledge-base views — reusing Plan 1's two-registry + rail machinery. `analytics` stays `backendReady:false` (charts specified, not wired).

**Architecture:** Mirror Plan 1's settings machinery for a second family: a `dashboardViews.ts` client-meta registry + `dashboardLoaders.server.ts`, a dynamic `/dashboard/[view]` route, and a shared `HubRail` component (extracted from Plan 1's settings rail so both families share one grouped, active-aware, disabled-gated nav). Views are read surfaces using `DashboardPanel` + `MetricCard`; discrete actions (unfollow, publish, delete) use `optimisticAction`. The content manager's Edit deep-links to `/create?draft={id}`.

**Tech Stack:** As Plan 1, plus LayerChart 2.0.1 (analytics wrappers only).

**Spec:** `docs/superpowers/specs/2026-07-21-user-hub-design.md` §3, §4, §7; §5 (analytics contract).

## Global Constraints

(Identical to Plan 1.) Plus:
- **LayerChart** (analytics wrappers): colors via `var(--chart-1..5)`; fixed-height containers; per-chart skeletons; rounded numbers; NO ad slots on dashboards; data only from API (Rule 16).
- **Cross-app links** (ADR-060): absolute subdomain URLs to `learn.`/`docs.`; do not re-implement those surfaces.
- **`analytics` is `backendReady:false`** — its route 404s and the rail entry is disabled until the §5 backend endpoints exist.

## File Structure

```
src/lib/components/hub/
  HubRail.svelte ........................ (CREATE) shared grouped rail (extracted from settings layout)
  DashboardPanel.svelte ................. (CREATE) read chrome (listed in Plan 1 file map; built here)
  MetricCard.svelte ..................... (CREATE) metric tile (built here)
  charts/
    AreaChartCard.svelte ............... (CREATE) built, wired only when analytics unblocks
    LineChartCard.svelte ............... (CREATE)
    BarChartCard.svelte ................ (CREATE)
  dashboard/
    dashboardViews.ts ................... (CREATE) client meta registry
    dashboardViews.test.ts ............. (CREATE)
    dashboardLoaders.server.ts ......... (CREATE) server loader registry
    contentQuery.ts .................... (CREATE) pure: buildContentQuery
    contentQuery.test.ts ............... (CREATE)
    views/
      OverviewView.svelte .............. (CREATE)
      ContentManagerView.svelte ....... (CREATE)
      RelationshipsView.svelte ........ (CREATE)
      LearningView.svelte ............. (CREATE)
      CertificatesView.svelte ......... (CREATE)
      KnowledgeBaseView.svelte ........ (CREATE)
src/routes/(app)/dashboard/
  +page.server.ts ....................... (CREATE) redirect → /dashboard/overview
  +layout.svelte ........................ (CREATE) HubRail + {@render children()}
  [view]/+page.server.ts ................ (CREATE) dispatch loader; 404 unknown/!backendReady
  [view]/+page.svelte ................... (CREATE) render registry component
src/routes/(app)/settings/+layout.svelte  (MODIFY) refactor to use HubRail (behavior-preserving)
src/lib/api/hub.ts ...................... (MODIFY) add content-manager + relationship action wrappers
src/lib/i18n/en.json, fr.json ........... (MODIFY)
```

---

## Phase 0 — Primitives + pure logic

### Task 1: `MetricCard` + `DashboardPanel`

**Files:** Create `src/lib/components/hub/MetricCard.svelte`, `DashboardPanel.svelte`.

- [ ] **Step 1: `MetricCard`** props `{ labelKey: string; value: number | string; delta?: number }` — renders label + `Math.round`-ed number (+ optional signed delta). Tailwind card tokens.
- [ ] **Step 2: `DashboardPanel`** props `{ titleKey: string; actions?: Snippet; children: Snippet }` — header + optional actions + slot; no footer.
- [ ] **Step 3:** `cd apps/main && bun run check` → PASS. **Step 4:** Commit `feat(main): DashboardPanel + MetricCard primitives`.

### Task 2: `buildContentQuery` (TDD)

**Files:** Create `src/lib/components/hub/dashboard/contentQuery.ts`, `contentQuery.test.ts`.

**Interfaces:** Produces `buildContentQuery(opts: { status?: string; type?: string; cursor?: string; pageSize?: number }): string` (returns the `/me/content` path with an encoded query; drafts/scheduled use dedicated paths).

- [ ] **Step 1: Write the failing test**

```ts
import { describe, it, expect } from 'vitest';
import { buildContentQuery } from './contentQuery';

describe('buildContentQuery', () => {
	it('defaults to published content with a page size', () => {
		expect(buildContentQuery({})).toBe('/me/content?page_size=20');
	});
	it('routes drafts and scheduled to their own endpoints', () => {
		expect(buildContentQuery({ status: 'draft' })).toBe('/me/drafts?page_size=20');
		expect(buildContentQuery({ status: 'scheduled' })).toBe('/me/scheduled?page_size=20');
	});
	it('adds a content-type filter and cursor', () => {
		expect(buildContentQuery({ type: 'blog_post', cursor: 'abc' })).toBe(
			'/me/content?page_size=20&type=blog_post&cursor=abc'
		);
	});
});
```

- [ ] **Step 2:** Run `cd apps/main && bun run test contentQuery` → FAIL.
- [ ] **Step 3: Implement**

```ts
export function buildContentQuery(opts: {
	status?: string;
	type?: string;
	cursor?: string;
	pageSize?: number;
}): string {
	const size = opts.pageSize ?? 20;
	if (opts.status === 'draft') return `/me/drafts?page_size=${size}`;
	if (opts.status === 'scheduled') return `/me/scheduled?page_size=${size}`;
	const params = new URLSearchParams({ page_size: String(size) });
	if (opts.type) params.set('type', opts.type);
	if (opts.cursor) params.set('cursor', opts.cursor);
	return `/me/content?${params.toString()}`;
}
```

- [ ] **Step 4:** Run → PASS. **Step 5:** Commit `feat(main): content-manager query builder`.

---

## Phase 1 — Shared rail + dashboard machinery

### Task 3: Extract `HubRail`, dashboard registry, route, refactor settings layout

**Files:** Create `HubRail.svelte`, `dashboardViews.ts`, `dashboardViews.test.ts`, `dashboardLoaders.server.ts`, `dashboard/+page.server.ts`, `[view]/+page.server.ts`, `[view]/+page.svelte`, `dashboard/+layout.svelte`; Modify `settings/+layout.svelte`.

**Interfaces:**
- `HubRail` props `{ groups: [HubGroup, { slug; labelKey; backendReady; component }[]][]; basePath: string; active: string; labelKey: string }`.
- `dashboardViews.ts`: `DASHBOARD_VIEWS: ViewMeta[]`, `resolveView`, `viewsByGroup`, `DEFAULT_VIEW='overview'`.

- [ ] **Step 1: Write the failing test** `dashboardViews.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { resolveView, DEFAULT_VIEW } from './dashboardViews';

describe('dashboard registry', () => {
	it('defaults to overview and resolves known views', () => {
		expect(DEFAULT_VIEW).toBe('overview');
		expect(resolveView('content')?.slug).toBe('content');
		expect(resolveView('nope')).toBeNull();
	});
	it('flags analytics as not backend-ready', () => {
		expect(resolveView('analytics')?.backendReady).toBe(false);
	});
});
```

- [ ] **Step 2:** Run `cd apps/main && bun run test dashboardViews` → FAIL.
- [ ] **Step 3: Implement `HubRail.svelte`** — the grouped, active-aware, disabled-gated nav lifted verbatim from Plan 1's `settings/+layout.svelte`, parameterized by `groups`/`basePath`/`active`/`labelKey` (an item is disabled when `!backendReady || !component`; href = `${basePath}/${slug}`).
- [ ] **Step 4: Implement `dashboardViews.ts`** (group `'activity'`; `overview, analytics(backendReady:false, component:null), content, relationships, learning, certificates, knowledge-base`), `dashboardLoaders.server.ts` (overview → `/me` + `/me/analytics` + recent `/me/content?page_size=5`; content → first page via `buildContentQuery`; relationships → `/me/following` etc.; learning → `/me/enrollments`; certificates → `/me/certificates`; knowledge-base → `/me/bookmarks`).
- [ ] **Step 5: Implement the route trio** (`dashboard/+page.server.ts` redirect to `/dashboard/overview`; `[view]/+page.server.ts` dispatch + 404 gating — identical shape to Plan 1 Task 5; `[view]/+page.svelte` render). `dashboard/+layout.svelte` uses `HubRail` with `viewsByGroup()`.
- [ ] **Step 6: Refactor `settings/+layout.svelte`** to render `HubRail` with `sectionsByGroup()`, `basePath="/settings"`, `active={page.params.section}` — behavior-preserving (Plan 1's settings round-trip must still pass).
- [ ] **Step 7:** Run `cd apps/main && bun run test && bun run check` → PASS (settings tests still green after refactor).
- [ ] **Step 8:** Commit `feat(main): shared HubRail + dashboard registry/route + settings refactor`.

---

## Phase 2 — Views

### Task 4: Overview view

**Files:** Create `views/OverviewView.svelte`; wire registry.

**Behavior:** `MetricCard` row from `data.analytics` (the 4 real counts: content/drafts/published/bookmarks); a "recent content" list from `data.recent`; quick links to the other views. `DashboardPanel` wrappers; `LoadingSkeleton` per panel.

- [ ] Step 1: Implement. Step 2: wire `dashboardViews.ts` overview → component. Step 3: `bun run check` PASS. Step 4: Commit `feat(main): dashboard overview view`.

### Task 5: Content manager view

**Files:** Create `views/ContentManagerView.svelte`; wire registry; add wrappers to `hub.ts`.

**Behavior:**
- Status tabs **Published / Drafts / Scheduled** + a content-type `Select` filter; changing either updates the URL (`?status=&type=`) so the loader re-fetches via `buildContentQuery` (Task 2).
- Rows: title · type pill · status · date · views/comments · actions:
  - **Edit** → `href="/create?draft={id}"` (reuses composer).
  - **Publish**/**Archive** → `optimisticAction` on `publishContent(id)` / `archiveContent(id)`.
  - **Delete** → confirm `Dialog` → `optimisticAction(deleteContent(id))`.
- Cursor pagination ("Load more"). `EmptyState` when a tab is empty.
- Add to `hub.ts`: `export const publishContent = (id) => api.post(\`/content/${id}/publish\`); export const archiveContent = (id) => api.post(\`/content/${id}/archive\`); export const deleteContent = (id) => api.delete(\`/content/${id}\`);`

- [ ] Step 1: Add wrappers. Step 2: Implement the view (tabs + filter + rows + actions + pagination). Step 3: wire registry. Step 4: `bun run check` PASS. Step 5: Commit `feat(main): content manager view`.

### Task 6: Relationships view

**Files:** Create `views/RelationshipsView.svelte`; wire registry; add wrappers.

**Behavior:** tabs **Following / Followers / Spaces / Tags** (loader provides all four lists or fetches per active tab via URL `?tab=`); count `MetricCard`s on top; each following/space/tag row has an **Unfollow/Leave** action via `optimisticAction`. Add `unfollow`/`leaveSpace`/`unfollowTag` wrappers to `hub.ts` (`DELETE`/`POST` per the follow-system routes). `EmptyState` per empty tab.

- [ ] Step 1: Add wrappers. Step 2: Implement. Step 3: wire registry. Step 4: `bun run check` PASS. Step 5: Commit `feat(main): relationships view`.

### Task 7: Learning + Certificates + Knowledge-base (ADR-060 cross-app)

**Files:** Create `views/LearningView.svelte`, `CertificatesView.svelte`, `KnowledgeBaseView.svelte`; wire registry.

**Behavior:**
- **Learning**: continue-learning cards from `data` (`/me/enrollments`); **progress bars** (scalar %, no chart); **Resume** links to `https://learn.japaconnect.com/...` (absolute).
- **Certificates**: grid from `data` (`/me/certificates`); each links to `/certificates/:id`.
- **Knowledge-base**: bookmarked docs from `data` (`/me/bookmarks`) + a prominent link to `https://docs.japaconnect.com/docs/users`.
- All three use `DashboardPanel`; `EmptyState` when empty. No re-implementation of Learn/Docs surfaces.

- [ ] Step 1: Implement the three views. Step 2: wire registry (all three). Step 3: `bun run check` PASS. Step 4: Commit `feat(main): learning/certificates/knowledge-base views`.

### Task 8: Analytics chart wrappers (built, not wired) + blocked gating

**Files:** Create `charts/AreaChartCard.svelte`, `LineChartCard.svelte`, `BarChartCard.svelte`. (No `AnalyticsView` component — `analytics` stays `component: null, backendReady:false`.)

**Behavior:** the three LayerChart wrappers per spec §4 (fixed-height container, `var(--chart-*)` colors, per-chart skeleton, rounded numbers), each taking typed series props matching the §5 contract shapes. They compile and are unit-testable for number formatting, but are NOT mounted anywhere until the backend endpoints land and an `AnalyticsView` is added (future work). The registry keeps `analytics` disabled/404.

- [ ] **Step 1: Write a failing test** for one wrapper's number formatting (e.g., a `formatMetric(n)` util rounding + thousands-separating) in `charts/format.ts` + `format.test.ts`.
- [ ] Step 2: Implement `format.ts` + the three chart wrappers (per svelte-layerchart skill). Step 3: `bun run test charts && bun run check` → PASS. Step 4: Commit `feat(main): analytics LayerChart wrappers (built, awaiting backend)`.

> **Do NOT** create an `AnalyticsView` or flip `analytics.backendReady` to true. Per spec §5/§12, that happens only after `GET /me/analytics/{impressions,followers,content,geo}` exist. A registry test (Task 3) already asserts `analytics.backendReady === false`.

---

## Phase 3 — i18n + verification

### Task 9: i18n keys + full gate + live round-trip

**Files:** Modify `en.json`, `fr.json`.

- [ ] Step 1: `cd apps/main && grep -rohE "dashboard\.[a-z_.]+" src/lib/components/hub/dashboard src/routes/\(app\)/dashboard | sort -u` → add every key to en+fr.
- [ ] Step 2: `cd apps/main && bun run test && bun run check` → all PASS (incl. Plan 1 settings tests after the HubRail refactor).
- [ ] Step 3: Live round-trip (backend :8080): `/dashboard` → redirects to overview; content manager lists by status+type, Edit deep-links `/create?draft={id}`, Publish/Delete work with rollback; relationships tabs + unfollow; learning progress bars + Resume opens learn subdomain; `/dashboard/analytics` → 404 (blocked); rail highlights the active view; settings rail still works (refactor regression check).
- [ ] Step 4: Final commit (if fixes) `fix(main): dashboard family round-trip corrections`.

---

## Self-Review (plan author)

- **Spec coverage:** overview (T4) · content manager incl. `/create` edit reuse (T5) · relationships (T6) · learning/certificates/KB (T7) · analytics blocked with built-but-unwired charts (T8) · shared HubRail + dashboard registry/route (T3) · DashboardPanel/MetricCard primitives (T1) · content query builder (T2).
- **Type/registry consistency:** `HubRail` (T3) reused by both families; `ViewMeta` mirrors Plan 1's `SectionMeta`; `buildContentQuery` (T2) consumed by the content loader (T3) + view (T5); `backendReady:false` gating for analytics matches Plan 1's devices pattern and the spec §5/§12.
- **Cross-plan integrity:** the T3 settings-layout refactor is behavior-preserving and re-runs Plan 1's settings tests (T3 Step 7, T9 Step 2) to catch regressions.
- **Rule 16 discipline:** no `AnalyticsView` and no fabricated series — charts exist as unwired primitives only; progress uses scalar bars, not invented time-series.
