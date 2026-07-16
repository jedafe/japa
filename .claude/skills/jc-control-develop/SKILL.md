---
name: jc-control-develop
description: Use when building new pages or components for the JapaConnect Control Panel (apps/control, port 3002, SPA). Covers infrastructure dashboards with LayerChart, the RBAC CRUD Gate, the mandatory DataTable spec, API key/extension/settings/job-queue patterns, and safe polling.
---

# JapaConnect Control Panel — Development Skill

Operator-facing SPA (adapter-static, `super_admin` + `control_panel.access` only). API base `/api/v1/control/*`. Design: dev.to admin + Vercel dashboard; dark mode default; monospace for IDs/keys/JSON.

## ⛔ GATE 1 — API-READINESS
Identical to `jc-admin-develop` Gate 1: list endpoints → verify each → any missing: STOP + ⚠️ MISSING BACKEND API ROUTES alert. Never mock.

## ⛔ GATE 2 — RBAC CRUD (Non-Negotiable)
**Super-admin full CRUD on every Control Panel entity** (API keys, OAuth clients, extensions, tenant sites, system settings, permission nodes, webhook endpoints, backup configs). Before implementing any entity page, verify List/Get/Create/Update/Delete + state routes (activate/deactivate, enable/disable, revoke/restore) all exist and are RBAC-gated. Any missing: STOP + ⛔ MISSING RBAC CRUD ENDPOINTS alert (format in `jc-admin-develop`).

## 📋 DATATABLE SPEC — Same Mandate as Admin
Every entity list (API keys, sites, extensions list-view, failed jobs, audit log, impersonation log) uses the full spec from `jc-admin-develop`: **sortable · searchable · filterable · paginated · action column (View/Edit/Delete + state actions like Revoke, Enable/Disable) · bulk actions · Create button on non-readonly entities**.
Readonly entities here (audit log, impersonation log, webhook delivery log, job history): action column MUST include **Delete/Remove/Clear** per row + a "Clear all / Clear older than…" bulk control.

## LayerChart — Infrastructure & Ops Visualization (Mandatory)
All Control Panel dashboards render metrics with LayerChart:
- **Health dashboard:** per-service time-series sparklines (PG pool usage, Valkey memory, Meilisearch index size) — `<Chart>` + `<Area>`/`<Spline>`
- **Job queue monitor:** queue depth over time per tier (`<Spline>` × 3, legend), failed-vs-processed stacked `<Bars>`
- **AI cost monitoring:** daily spend `<Bars>` + cumulative `<Spline>`
- **Traffic/platform metrics:** request volume, error rate `<Area>`
- Rules: colors `var(--color-success|warning|danger|primary)` only · fixed-height containers · data from `/api/v1/control/*` endpoints · skeletons · live charts append new points on each poll tick

## Key Page Patterns
**API Keys:** DataTable · create in `<Dialog.Root>` with scope multi-select · show key ONCE in reveal dialog (copy button + `<Tooltip.Root>` "Copied!") · never keep raw key in `$state` after close (`onOpenChange` clears) · Revoke = state action with confirmation.
**Extensions:** card grid + `<Toggle.Root>` per extension · toggle disabled while PATCH in-flight · optimistic + rollback · disabling with active content → warning dialog · Configure → manifest-driven settings dialog.
**System Settings (49 keys):** `<Tabs.Root>` per category · type-appropriate inputs · JSON values validated before save · deep-equality dirty tracking (`$derived(JSON.stringify(a)!==JSON.stringify(b))`) · unsaved banner + `beforeNavigate` guard · batch `PATCH` with `[{key,value}]`.
**Job Queue:** tabs Critical/Default/Low · stats + failed-jobs DataTable (retry action) · poll 15s.
**Health:** service cards + LayerChart sparklines · poll 30s.
**Safe polling (mandatory pattern):**
```svelte
$effect(() => {
  const tick = async () => { try { const r = await fetch(url); if (r.ok) data = await r.json(); } catch {} };
  tick();
  const id = setInterval(tick, 30_000);
  return () => clearInterval(id);
});
```

## SPA Architecture & Stack
Same as `jc-admin-develop`: `ssr=false`, auth guard verifying BOTH session AND `control_panel.access` node, `$effect` fetching, Svelte 5 Runes, Bits UI dot-notation, Tailwind v4, `$t()` i18n, no AI attribution. Environment badge (PRODUCTION/STAGING) in top bar.

## Deliverables Per Task
1. Gate 1 + Gate 2 output · 2. `+page.svelte` (full-spec DataTable / LayerChart dashboard / settings pattern) · 3. dialogs & components · 4. types · 5. every `/api/v1/control/*` route confirmed
