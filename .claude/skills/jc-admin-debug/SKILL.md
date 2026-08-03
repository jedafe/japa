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
Permission-node checks (`session.permissions.includes(...)`) not role-string checks · destructive controls hidden/disabled without permission · state-action buttons permission-gated individually (ban ≠ suspend permission). **Japa University**: `university.instructor.approve` ≠ `university.instructor.suspend` ≠ `university.course.approve` ≠ `university.payout.process` — each is a separate node, gate each action individually.

## 6. Bits UI / Svelte 5
Apply the full violation tables from `jc-main-debug` sections 2–3.

## 6b. Japa University Admin (ADR-063 + Section 39)
- **Instructor approval missing both directions:** approve without reject (or vice versa) — Gate 2 requires BOTH directions of every state transition. A pending instructor must have Approve AND Reject; an approved instructor must have Suspend AND Unsuspend.
- **Payout method exposed:** `instructor_profiles.payout_method` (jsonb) shown raw in the admin UI — Critical security finding. Must be masked.
- **University course approval one-directional:** only Approve without Reject — both required. Approve sets `is_published_to_university = true`; Reject keeps it false.
- **Payout state machine broken:** `instructor_payouts` status must go pending→processing→paid or →failed. A jump from pending to paid (skipping processing) or from paid back to pending is a defect.
- **University logo not uploading:** logo upload must go through Section 34 media (SeaweedFS + imgproxy) — if it's stored as a raw URL string, that's a defect.

## 7. Accessibility
`<th scope="col">` + `aria-sort` on sortable headers · bulk buttons `disabled` (not hidden) when nothing selected · focus returns to trigger on dialog close · `aria-live="polite"` on table container during load.

## Output Protocol
1. Root cause (one sentence) · 2. Broken code · 3. Fixed code · 4. Secondary findings
**V2 admin phases (20–23):** Persona Management + Content Studio (Phase 20) → Newsletter + Jobs (Phase 21) → Gamification/Analytics/Commerce (Phase 22) → QA (Phase 23). See `ROADMAP-ADMIN.md` for task breakdown.

**New admin surfaces in v2:** AI Persona management (`/admin/personas`), Content Studio (`/admin/studio`), Participation queue (tab in moderation queue), RAG index status widget, Remittance provider management, Newsletter composer, Gamification badge/challenge CRUD, Advanced analytics dashboard.

## V2 Feature Areas (Implement Only After Corresponding Backend Phase Is Complete)

| Feature | Backend phase | Key sections | Gates |
|---|---|---|---|
| JapaBot (RAG-grounded) | Phase 41 | §69 | `/ai/chat` amended |
| AI Content Studio (admin) | Phase 42 | §75 | `studio.*` RBAC |
| Participation queue (admin) | Phase 42 | §76 | `participation.review` RBAC |
| Visa Checklist | Phase 43 | §70 | `checklist.use` RBAC |
| Readiness Assessment | Phase 43 | §71 | `assessment.take` RBAC |
| Cost of Living Calculator | Phase 43 | §72 | `col_calculator.use` RBAC |
| Scholarship Finder | Phase 43 | §73 | `scholarship.browse` RBAC |
| Remittance Rate Tracker | Phase 44 | §74 | `remittance.view` RBAC |
| Referral system | Phase 45 | §60 | `referral.use` RBAC |
| Self-serve ads | Phase 45 | §61 | `ads.self_serve.create` RBAC |
| Newsletter | Phase 45 | §62 | `newsletter.subscribe` RBAC |
| Jobs board | Phase 45 | §63 | `jobs.browse` RBAC |
| Gamification | Phase 45 | §64 | `gamification.view` RBAC |
| Content import | Phase 46 | §65 | `import.use` RBAC |
| Advanced analytics | Phase 46 | §66 | `analytics.advanced` RBAC |
| Travel search | Phase 46 | §67 | `travel.search` RBAC |

**API-Readiness Gate applies to all v2 features.** Verify against `api-routes-catalog.md` — all v2 routes are marked as "V2 planned" in the catalog. Once the corresponding backend phase lands, the planned routes become available.

**New surface in v2:** `apps/tools` (§78, ADR-086) — a sixth SvelteKit SSR app at `tools.japaconnect.com` (port 3005). Generate `jc-tools-*` skills before implementing this surface. Skills live at `.ai/skills/frontend/06-tools/`.
