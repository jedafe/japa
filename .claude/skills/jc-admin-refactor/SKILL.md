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
Role-string checks → permission-node checks: `$derived(session?.permissions.includes('users.ban') ?? false)`.

## 6. Standard Migrations
Svelte 4→5, Bits UI adoption, Tailwind v4, i18n sweep, a11y — apply `jc-main-refactor` §§1–2, 5. Consolidate duplicate fetches (user groups, spaces, content types) into `setContext`/`getContext` at admin root layout.
