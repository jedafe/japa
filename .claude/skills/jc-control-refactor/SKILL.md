---
name: jc-control-refactor
description: Use when updating or refactoring existing JapaConnect Control Panel code (apps/control). Fixes polling patterns, upgrades tables to full DataTable spec, migrates ops charts to LayerChart, hardens key handling.
---

# JapaConnect Control Panel — Updating & Refactoring Skill

Conventions and gates: `jc-control-develop`. Behavior-preserving only; one-line comment per change.

## 0. Gates
New endpoint needed by the refactor → verify first (⚠️/⛔ alerts, pause). Refactoring an entity page → run Gate 2 (RBAC CRUD) on that entity and flag gaps.

## 1. Polling → Safe $effect Pattern
```svelte
// BEFORE: onMount/onDestroy interval pair (or worse, no cleanup)
// AFTER:
$effect(() => {
  const tick = async () => { try { const r = await fetch(url); if (r.ok) data = await r.json(); } catch {} };
  tick();
  const id = setInterval(tick, 30_000);
  return () => clearInterval(id);
});
```

## 2. Table → Full DataTable Spec
Apply `jc-admin-refactor` §1 in full. Control-specific: audit log / impersonation log / webhook deliveries / job history are readonly — ensure Delete/Remove/Clear per row + "Clear all / older-than" bulk. API keys get Revoke state action; extensions list-view gets Enable/Disable.

## 3. Chart Migration → LayerChart
All health/queue/cost/traffic visualizations rewritten in LayerChart (patterns in `jc-control-develop`). Live-updating charts: cap series arrays (last 60 points) to prevent unbounded growth. Remove old chart libraries from package.json.

## 4. Key Handling Hardening
One-time reveal dialog clears value on close · remove any `$state` retention, URL exposure, console logging of raw keys.

## 5. Settings Modernization
Deep-equality `$derived` dirty tracking · `beforeNavigate` guard · JSON validation `$effect` · batch save. **Geo/University settings (ADR-063/064):** add `geo_personalization.enabled` killswitch with disable-warning dialog · add `university.*` settings (commission tiers, pro stream rate, payout threshold/period, surface_domain) with range validation and surface_domain change warning.

## 6. Toggle Hardening
Add `toggling` guards, in-flight disable, optimistic + rollback to all extension/feature-flag toggles.

## 7. Standard Migrations
Svelte 4→5, Bits UI adoption, Tailwind v4, i18n, a11y — apply `jc-main-refactor` §§1–2, 5.
**V2 control panel:** No new control panel pages for v2 features — v2 features are managed in the Admin panel. Control panel remains super-admin infrastructure (tenants, system settings, feature flags). Update system_settings pages to expose the 62 new v2 keys (settings are already stored; control panel just needs form fields for them).

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
