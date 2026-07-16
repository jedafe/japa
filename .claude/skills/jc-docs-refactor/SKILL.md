---
name: jc-docs-refactor
description: Use when updating or refactoring existing JapaConnect Docs app code (apps/docs). Converts scroll tracking to IntersectionObserver, modernizes search to Dialog+Combobox, fixes listener cleanup, migrates any charts to LayerChart.
---

# JapaConnect Docs — Updating & Refactoring Skill

Conventions and gates: `jc-docs-develop`. Behavior-preserving; one-line comment per change.

## 0. API-Readiness Gate
Refactor needs an endpoint → verify first (⚠️ alert if missing, pause). Hardcoded sidebar tree found → replace with `/api/v1/docs/sections` fetch (verify route first).

## 1. Scroll Tracking → IntersectionObserver
Replace scroll-event heading tracking with IntersectionObserver in `$effect` + `disconnect()` cleanup (`rootMargin: '-10% 0% -80% 0%'`).

## 2. Global Listeners → $effect with Cleanup
onMount/onDestroy listener pairs (Cmd+K, resize) → single `$effect` returning the remove call.

## 3. Search Debounce Modernization
`$:` + manual clearTimeout → `$effect` with `return () => clearTimeout(id)`; fetch `/docs/search` on settled query.

## 4. Search Modal → Dialog + Combobox
Custom overlays → `<Dialog.Root bind:open>` + `<Combobox.Root>` composition (grouped results, keyboard nav).

## 5. Mobile Sidebar → Dialog.Root
Custom drawers → `<Dialog.Root>` for focus trapping.

## 6. Chart Migration → LayerChart
Any usage-stat/analytics chart rewritten in LayerChart (`jc-main-refactor` §3 pattern).

## 7. Standard Migrations
Apply `jc-main-refactor` §§1–2, 4–5 (Svelte 4→5, Bits UI — incl. `<Tabs.Root>` for API reference, Rule 16, skeletons, Tailwind v4, i18n, a11y, types).
