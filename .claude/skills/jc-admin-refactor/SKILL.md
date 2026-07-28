---
name: jc-admin-refactor
description: Use when updating or refactoring existing JapaConnect Admin Panel code (apps/admin). Upgrades tables to the full DataTable spec, migrates charts to LayerChart, modernizes to Svelte 5 Runes and Bits UI.
---

# JapaConnect Admin Panel — Updating & Refactoring Skill

Conventions and gates: `jc-admin-develop`. Behavior-preserving modernization only; one-line comment per change.

## 0. Gates
Refactor needs a new endpoint (e.g. adding a missing Delete action)? Verify it exists first. Missing → ⚠️/⛔ alert, pause. **When refactoring an entity page, run Gate 2 (RBAC CRUD) on that entity** — refactoring is the moment to catch pages shipped without full CRUD.

## 1. Table → Full DataTable Spec Upgrade
Any table missing part of the mandatory spec gets upgraded to ALL of it:
- Add missing: sort (URL params) · search (`?q=` debounced) · filter panel (`<Accordion.Root>`) · pagination + page-size selector
- Add missing action-column items: View (modal) / Edit / Delete — plus state actions (Suspend/Unsuspend, Ban/Unban, Approve/Reject) conditional on row state
- Add bulk selection (`Set<string>`) + bulk bar
- Add Create button (non-readonly entities)
- Readonly entities: add Delete/Remove/Clear to the action column + "Clear all/older-than" bulk option
- Replace `window.confirm` → `<Dialog.Root>` confirmation · replace navigation-on-view → `<Dialog.Root>` preview

## 2. Chart Migration → LayerChart
Rewrite all Chart.js / canvas / raw-SVG / D3-DOM charts as LayerChart (pattern in `jc-main-refactor` §3). Dashboard stat cards get sparklines (`<Chart>` + `<Spline>`, no axes). Analytics pages get a date-range picker driving `?from=&to=`. Remove replaced chart libraries from package.json.

## 3. Data Fetching Modernization
`onMount` fetch → reactive `$effect` with loading/error/`finally`; params inside effect scope so sort/filter/page changes re-fetch.

## 4. Form Modernization
Single `$state` form object · `submitting` + `formError` states · CSRF header · dialog closes + table refreshes on success · unsaved-changes `beforeNavigate` guard on edit forms.

## 5. RBAC Hardening
Role-string checks → permission-node checks: `$derived(session?.permissions.includes('users.ban') ?? false)`. **Japa University**: each university action has its own node (`university.instructor.approve`, `university.course.approve`, `university.payout.process`) — gate each individually, not by a blanket `university.*` role.

## 6. Standard Migrations
Svelte 4→5, Bits UI adoption, Tailwind v4, i18n sweep, a11y — apply `jc-main-refactor` §§1–2, 5. Consolidate duplicate fetches (user groups, spaces, content types) into `setContext`/`getContext` at admin root layout.

## 7. Japa University Admin Pages (ADR-063 + Section 39)
- **Add university entity pages** if missing: Universities (Section 39 space type) with full DataTable + CRUD + Verify state action + CSV bulk import. Instructor profiles with review dialog + both-direction state transitions. University course approval with Approve/Reject. Payout management with process/mark-failed state machine.
- **Mask payout_method:** any raw display of `instructor_profiles.payout_method` → masked display (show last 4 digits or type only).
- **University logo:** if stored as a raw URL string → migrate to Section 34 media upload (SeaweedFS + imgproxy).
- **Run Gate 2 on every university entity** during refactor — refactoring is when missing CRUD/state routes get caught.
