---
name: jc-docs-audit
description: Use when performing a comprehensive quality audit of the JapaConnect Docs app (apps/docs) before merge or release. Verifies observer/listener cleanup, API-driven sidebar, search modal composition, code blocks, SEO structured data, and standard compliance.
---

# JapaConnect Docs — Audit Skill

Work file-by-file. Conventions: `jc-docs-develop`.

## A. Memory Leak Audit — Critical (Run First)
| Subscription | Required cleanup |
|---|---|
| IntersectionObserver | `$effect` returns `disconnect()` |
| window keydown (Cmd+K) | `$effect` returns `removeEventListener` |
| setTimeout debounce | `$effect` returns `clearTimeout` |
| setInterval / ResizeObserver | cleared/disconnected |
Verify by navigating between 10 docs pages — single observer/listener set in DevTools. Any missing cleanup → Critical.

## B. API Consumption Verification
Every route confirmed against catalog/probe (⚠️ block if missing). Sidebar MUST come from `/api/v1/docs/sections` — hardcoded tree = Critical Rule 16 violation.

## C. Search Modal
`<Dialog.Root>` + `<Combobox.Root>` composition · autofocus · 200ms debounce · grouped results · full keyboard path (Cmd+K → arrows → Enter → Escape).

## D. Code Blocks
Language badge · copy button (textContent, tooltip confirmation, screen-reader announce) · post-render dynamic-import highlighting · dark-mode contrast of syntax colors.

## E. On-This-Page
Live-DOM heading extraction · IntersectionObserver tracking · smooth anchor scroll · hidden on mobile · `role="navigation"` + aria-label.

## F. Responsive Layout
Column collapse at breakpoints · `<Dialog.Root>` mobile drawer with focus trap · no horizontal overflow.

## G. SEO & Structured Data
Per page: title, description, canonical → `docs.japaconnect.com`, TechArticle JSON-LD · landing: WebSite + SearchAction JSON-LD · API reference: appropriate WebPage schema.

## H. LayerChart (If Usage Stats Present)
Apply `jc-main-audit` §E.

## I. Standard Compliance
Apply `jc-main-audit` §§B (Rule 16), D (Bits UI), B(runes), G (security/{@html}), H (a11y — plus code-region roles, copy-button labels, nav landmarks, external-link rel), I (i18n).

## Report Format
Same structure as `jc-main-audit` with categories: Memory leaks · API routes/sidebar · Search modal · Code blocks · On-This-Page · Responsive · SEO · Bits UI · Svelte 5 · A11y.
