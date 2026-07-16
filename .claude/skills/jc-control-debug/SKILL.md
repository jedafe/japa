---
name: jc-control-debug
description: Use when reviewing or debugging JapaConnect Control Panel code (apps/control, SPA). Diagnoses polling memory leaks, sensitive key handling, extension toggle races, DataTable spec gaps, LayerChart issues, and RBAC gaps.
---

# JapaConnect Control Panel — Review & Debugging Skill

Conventions and gates: `jc-control-develop`. Run in order.

## 0. Gates First
Data not loading → verify route exists before debugging (⚠️ alert if missing). Entity page missing a CRUD/state action → check backend route (⛔ alert if missing; hiding the action in the UI is not a fix).

## 1. Polling Memory Leaks (#1 Control Panel Bug)
Every `setInterval` inside `$effect` MUST return `() => clearInterval(id)`. Stacked intervals from navigation = exponential API load. Also check: initial fetch before interval starts · fetch errors caught silently inside the tick (a thrown error kills the loop) · optional `visibilitychange` pause.

## 2. Sensitive Data
Raw API key never in `$state` after one-time reveal (dialog `onOpenChange` clears it, with close-animation delay) · never in URL params/history/`console.log` · secret settings masked · JSON textareas validated before PATCH.

## 3. Extension Toggle Races
Toggle `disabled` during in-flight PATCH · `toggling` guard flag · optimistic + rollback on failure · disable-with-active-content confirmation dialog.

## 4. Settings Dirty State
Deep equality (`JSON.stringify`) not reference compare · `original` snapshot captured AFTER data load · `beforeNavigate` guard fires when dirty · batch save includes ALL changed keys.

## 5. DataTable Spec
Apply the full checklist from `jc-admin-debug` §1 — including readonly-entity Delete/Clear actions on audit/impersonation/job logs, and Revoke/Enable/Disable state actions conditional on row state.

## 6. LayerChart
Fixed-height containers · `var(--color-*)` colors (dark-default theme — verify contrast) · live charts appending points correctly on poll ticks without unbounded array growth (cap series length, e.g. last 60 points) · `d3-scale` imports · skeletons · LayerChart vs Bits UI `Tooltip` import alias.

## 7. Permission Gate
Root layout verifies `control_panel.access` node (not just session) · 403 → error page not blank screen · super_admin-only actions doubly gated.

## 8. SPA / Svelte 5 / Bits UI / A11y
Apply `jc-admin-debug` §§2–3, 6–7.

## Output Protocol
1. Root cause (one sentence) · 2. Broken code · 3. Fixed code · 4. Secondary findings
