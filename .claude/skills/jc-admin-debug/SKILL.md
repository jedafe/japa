---
name: jc-admin-debug
description: Use when reviewing or debugging JapaConnect Admin Panel code (apps/admin, SPA). Diagnoses DataTable spec violations, RBAC gaps, SPA auth issues, LayerChart problems, and Svelte 5/Bits UI errors.
---

# JapaConnect Admin Panel — Review & Debugging Skill

Conventions and gates: `jc-admin-develop`. Run checks in order.

## 0. Gates First
- Data not loading? Verify the route exists before debugging frontend. Missing → ⚠️ MISSING BACKEND API ROUTES alert, pause.
- Entity page missing an action (edit, delete, unban)? Check if the backend route exists. Missing → ⛔ MISSING RBAC CRUD ENDPOINTS alert, pause. A frontend "fix" that hides the action is NOT acceptable — super-admin CRUD is non-negotiable.

## 1. DataTable Spec Violations
Check the page against the mandatory spec — flag every miss:
- [ ] Sortable columns (URL-param driven, survives navigation)
- [ ] Search input wired to `?q=`, debounced
- [ ] Filter panel wired to `?filter=`
- [ ] Pagination with page-size selector from API `meta`
- [ ] Action column with View + Edit + Delete on every row
- [ ] State actions (Suspend/Unsuspend, Ban/Unban, Approve/Reject) present AND conditional on current state — a banned user's row must show Unban, not Ban
- [ ] Bulk checkboxes + select-all + contextual bulk bar
- [ ] Create button (top) on non-readonly entities
- [ ] Readonly entities still have Delete/Remove/Clear in the action column
- Bulk selection reset on page change · destructive actions behind `<Dialog.Root>` confirmation (never `window.confirm`) · View opens a modal, never navigates

## 2. SPA Auth
Root layout `ssr=false`/`prerender=false` · auth guard `$effect` + `goto('/login')` on every protected route · no `+page.server.ts` files · `goto()` not `window.location.href` · 403 → permission error UI, not blank screen.

## 3. Data Fetching
`$effect` (not `onMount`) so param changes re-fetch · reactive params inside effect scope · try/catch + error state + `finally { loading = false }` · `$derived` for computed counts.

## 4. LayerChart
Fixed-height container present · colors `var(--color-*)` only (test dark mode) · API date strings → `Date` objects · data from `/api/v1/admin/analytics/*` not hardcoded · skeleton per chart · LayerChart `Tooltip` vs Bits UI `Tooltip` import collision — alias one (`import { Tooltip as ChartTooltip } from 'layerchart'`).

## 5. RBAC UI
Permission-node checks (`session.permissions.includes(...)`) not role-string checks · destructive controls hidden/disabled without permission · state-action buttons permission-gated individually (ban ≠ suspend permission).

## 6. Bits UI / Svelte 5
Apply the full violation tables from `jc-main-debug` sections 2–3.

## 7. Accessibility
`<th scope="col">` + `aria-sort` on sortable headers · bulk buttons `disabled` (not hidden) when nothing selected · focus returns to trigger on dialog close · `aria-live="polite"` on table container during load.

## Output Protocol
1. Root cause (one sentence) · 2. Broken code · 3. Fixed code · 4. Secondary findings
