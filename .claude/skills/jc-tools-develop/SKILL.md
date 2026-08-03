---
name: jc-tools-develop
description: Use when developing the JapaConnect Tools Web Surface (apps/tools, port 3005, tools.japaconnect.com). Covers host-detection shell rendering, identity tool pages, and the standalone single-tool domain mode.
---

# JapaConnect Tools Surface — Develop Skill

> ⚠️ **V2 PLANNED SURFACE — NOT YET BUILT**
> This surface (apps/tools, §78, ADR-086) is created during Backend Phases 43–44 and Tools Phases 01–04.
> **Generate this full skill set from the existing surface skill template before implementing.**
> See `ROADMAP-TOOLS.md` Phase 01.01 — scaffold step includes skill generation.

## Surface Overview

SvelteKit SSR · Port 3005 · `tools.japaconnect.com` (configurable via `system_settings`)
Sixth SvelteKit app in the JapaConnect monorepo (`apps/tools/`).
Same Go backend, same `/api/v1`, no community navigation.

## Host Detection (Core Mechanic — §78, ADR-086)

`hooks.server.ts` reads `system_settings` per-tool domain keys and sets `event.locals.toolSurface`:
- `'hub'` → full tools listing (default when host = `tools.japaconnect.com`)
- `'remittance'` / `'scholarships'` / `'cost-of-living'` / `'visa-checklist'` / `'readiness'` / `'japabot'`
  → single-tool shell (tool branding, no cross-tool nav)

Root layout reads `toolSurface` and renders the appropriate shell. Every tool route works
in both shell modes — the shell wrapping changes, the tool content does not.

## ⛔ GATE — API-READINESS (Run Before Any Implementation)

Every tool page must verify its endpoints exist in `api-routes-catalog.md` before implementation.
All v2 tool routes are marked "V2 planned" — they become available as backend phases land:
- Visa Checklist → Backend Phase 43
- Readiness Assessment → Backend Phase 43
- Cost of Living → Backend Phase 43
- Scholarship Finder → Backend Phase 43
- Remittance Rate Tracker → Backend Phase 44
- JapaBot (RAG-grounded) → Backend Phase 41

## Stack

SvelteKit 2.57.x (SSR, adapter-auto) · Bun 1.3.x · Tailwind CSS v4 · Bits UI · Lucide Svelte ·
LayerChart 2.0.1 (pinned exactly — for remittance trend charts, assessment score gauge) ·
env vars `JC_` prefix (public: `PUBLIC_JC_*`) · Umami analytics (`PUBLIC_JC_UMAMI_SITE_ID_TOOLS`)

## Conventions

Same as the main app: SSR load functions fetch via the Go API, Rule 16 (no hardcoded data),
Rule 17 (no AI attribution). Public env var `PUBLIC_JC_API_BASE` for API calls.
Cookie auth (`domain=.japaconnect.com`) works for subdomains; custom domains use OAuth flow
back to `api.japaconnect.com` (see §78 auth section).

## Tool Pages

| Route | Tool | Backend phase | Refs |
|---|---|---|---|
| `/` | Tools hub (all tools listed) | Any | §78 |
| `/visa-checklist` | Visa Document Checklist | 43 | §70, ADR-070 |
| `/readiness` | Japa Readiness Assessment | 43 | §71, ADR-071 |
| `/cost-of-living` | Cost of Living Calculator | 43 | §72, ADR-072 |
| `/scholarships` | Scholarship Finder | 43 | §73, ADR-073 |
| `/remittance` | Remittance Rate Tracker | 44 | §74, ADR-075 |
| `/japabot` | JapaBot standalone | 41 | §69, §20 |

## See Also

ADR-086 (multi-surface architecture), §78 (Tools Web Surface section), `ROADMAP-TOOLS.md`,
ADR-007 (subdomain principle), ADR-063 (Japa University — host-detection precedent).
