---
name: jc-docs-debug
description: Use when reviewing or debugging JapaConnect Docs app code (apps/docs, SSR). Diagnoses IntersectionObserver and keydown listener leaks, Cmd+K search issues, code block highlighting, and responsive three-column collapse.
---

# JapaConnect Docs — Review & Debugging Skill

Conventions and gates: `jc-docs-develop`. Run in order.

## 0. API-Readiness First
Data not loading → verify route before debugging (⚠️ alert if missing, pause). Sidebar empty? Check `/api/v1/docs/sections` responds — do NOT hardcode a fallback tree.

## 1. Observer/Listener Leaks (Docs' #1 Bug)
- IntersectionObserver in `$effect` MUST return `() => observer.disconnect()` — accumulation breaks anchor highlighting and leaks memory
- Cmd+K `window.addEventListener('keydown', ...)` MUST return removeEventListener cleanup — stacked listeners open multiple dialogs
- Any `ResizeObserver`/`setInterval` similarly cleaned up

## 2. Cmd+K Search
`<Combobox.Input>` auto-focused on dialog open · first focusable inside `<Dialog.Content>` · Escape closes without Combobox conflict · debounce cleanup (`return () => clearTimeout(id)`) · results from `/docs/search` not local filtering.

## 3. Code Blocks
Highlighting in `$effect` + browser guard + dynamic import (not in SSR bundle) · re-runs on route change (new page content) · copy reads `textContent` not innerHTML · "Copied!" tooltip resets after ~2s.

## 4. Sidebar
From `/api/v1/docs/sections` (Rule 16) · active page tracks `$page.url.pathname` · mobile drawer is `<Dialog.Root>` with focus trap, focus returns to hamburger on close · nav data cached (no re-fetch per internal navigation).

## 5. Responsive Collapse
Three→one column at breakpoints · right panel hidden ≤ tablet · no horizontal overflow at any width.

## 6. `{@html}` Safety
Only for backend TipTap-rendered docs content — flag any other instance.

## 7. Standard Checks
Apply `jc-main-debug` §§1–3, 5–7 (SSR/hydration, Runes, Bits UI, Tailwind, SEO, a11y). LayerChart checks (`jc-main-debug` §4) if usage-stat charts present.

## Output Protocol
1. Root cause · 2. Broken code · 3. Fixed code · 4. Secondary findings
