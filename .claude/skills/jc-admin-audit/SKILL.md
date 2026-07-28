---
name: jc-admin-audit
description: Use when performing a comprehensive quality audit of the JapaConnect Admin Panel (apps/admin) before merge or release. Verifies RBAC CRUD completeness per entity, full DataTable spec compliance, LayerChart usage, SPA architecture, and security.
---

# JapaConnect Admin Panel — Audit Skill

Work file-by-file. Conventions: `jc-admin-develop`.

## A. RBAC CRUD Completeness Matrix (Run First — Highest Priority)
Build a matrix of EVERY entity managed in the panel × required backend routes:

| Entity | List | Get | Create | Update | Delete | State routes | Frontend exposes all? |
|---|---|---|---|---|---|---|---|

- Verify each route against JEOS `api-routes-catalog.md` or live probe
- **Any entity where super-admin cannot perform full CRUD from the UI is a Critical finding** — whether the gap is a missing backend route (⛔ MISSING RBAC CRUD ENDPOINTS block) or a missing frontend control
- State-capable entities (users, orgs, content, spaces, claims): verify BOTH directions exist and are surfaced (Ban AND Unban, Suspend AND Unsuspend, Approve AND Reject)
- **Japa University entities (ADR-063):**
  - `universities` (Section 39 space type): List/Get/Create/Update/Delete + Verify/Unverify + status transitions (publish/draft/archive) + CSV bulk import
  - `instructor_profiles`: List/Get + Approve/Reject + Suspend/Unsuspend + tier/commission update. **Both directions of every state transition required.**
  - `university/tracks` (course approval): List/Get + Approve/Reject (sets `is_published_to_university`)
  - `university/payouts`: List/Get + Process (trigger payout) + Mark-failed. State: pending→processing→paid/failed
- **Sensitive data check:** `instructor_profiles.payout_method` (jsonb) must be masked in the admin UI — never displayed raw (Critical security finding if exposed)

## B. DataTable Spec Compliance (Per Entity List Page)
- [ ] Sortable (URL-param) · [ ] Searchable (`?q=` debounced) · [ ] Filterable (`?filter=` panel) · [ ] Paginated (page-size selector, API meta)
- [ ] Action column: View (modal) / Edit / Delete on every row
- [ ] Conditional state actions per row state
- [ ] Bulk selection + contextual bulk bar
- [ ] Create button on non-readonly entities
- [ ] Readonly entities have Delete/Remove/Clear in action column
- [ ] Destructive actions behind `<Dialog.Root>` (no `window.confirm`) · [ ] View never navigates
Flag any list page using a custom table instead of the shared `<DataTable>`.

## C. API Consumption Verification
Every `fetch()` route confirmed against the catalog. Uncatalogued routes → Critical (⚠️ block).

## D. LayerChart Compliance
Dashboard + all 5 analytics pages use LayerChart exclusively · no Chart.js/canvas/raw-SVG charts · colors `var(--color-*)` (dark-mode verified) · fixed-height containers · skeletons · data from analytics endpoints with `?from=&to=` support.

## E. SPA Architecture
`ssr=false`/`prerender=false` · zero `+page.server.ts` · auth guard on every protected route · `goto()` navigation · token refresh present.

## F. Security
Permission-node gating on every destructive/privileged action (UI-side, not just backend) · CSRF on all mutations · `{@html}` audit · no PII/secrets in console.log · session validated per mount.

## G. Data Fetching Quality
try/catch + error state + retry · loading skeletons per table/chart · pagination state in URL (survives back/forward).

## H. Bits UI, Svelte 5, Accessibility, i18n
Apply `jc-main-audit` sections D, B(runes), H, I. Admin-specific a11y: `aria-sort`, disabled-not-hidden bulk buttons, focus return on dialog close, `aria-live` on tables.

## Report Format
```
## Audit Report — Admin Panel (apps/admin)
**Files Audited:** [n] · **Status:** PASS / NEEDS FIXES
### Critical / High / Medium / Low
[lists — RBAC CRUD gaps always Critical]
### RBAC CRUD Matrix
[the full entity × routes table]
### Compliance Summary
| RBAC CRUD completeness | ✅/❌ |
| DataTable spec | ✅/❌ |
| API routes verified | ✅/❌ |
| LayerChart | ✅/❌ |
| SPA architecture | ✅/❌ |
| Security | ✅/❌ |
| Bits UI · Svelte 5 · A11y · i18n | ✅/❌ |
```
