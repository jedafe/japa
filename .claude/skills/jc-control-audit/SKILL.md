---
name: jc-control-audit
description: Use when performing a comprehensive quality audit of the JapaConnect Control Panel (apps/control) before merge or release. Verifies security-critical key handling, polling reliability, RBAC CRUD completeness, DataTable spec, and LayerChart compliance.
---

# JapaConnect Control Panel — Audit Skill

Work file-by-file. Conventions: `jc-control-develop`. Security findings carry extra weight here.

## A. RBAC CRUD Completeness Matrix (Run First)
Same matrix method as `jc-admin-audit` §A, over Control entities: API keys, OAuth clients, extensions, tenant sites, system settings, permission nodes, webhook endpoints, backup configs. Verify List/Get/Create/Update/Delete + state routes (revoke, enable/disable, activate/deactivate) exist in backend AND are surfaced in UI. **Any super-admin CRUD gap = Critical** (⛔ block).

## B. Security — Critical
- [ ] Raw API keys cleared from `$state` after one-time reveal · never in URLs/history/console
- [ ] Reveal dialog clears on `onOpenChange`
- [ ] Secret settings values masked in UI
- [ ] `control_panel.access` node verified on mount (not just session) · 403 → error page
- [ ] CSRF on all mutations · JSON settings validated pre-save

## C. Polling Reliability
Every `setInterval`: `$effect` cleanup return present · initial fetch before interval · errors caught inside tick · no stacked intervals (navigate away/back 5×, confirm single interval in DevTools) · live chart series capped (≤60 points).

## D. DataTable Spec Compliance
Full checklist from `jc-admin-audit` §B per entity list. Control-specific verifications: readonly logs (audit, impersonation, webhook deliveries, job history) have per-row Delete/Clear + bulk clear · API keys have Revoke · extensions have Enable/Disable conditional on state · failed jobs have Retry.

## E. LayerChart Compliance
Health/queue/cost dashboards use LayerChart exclusively · `var(--color-*)` only (dark-default verified) · fixed heights · skeletons · poll-driven charts append correctly.

## F. Extension & Settings Integrity
Toggle: in-flight disable, rollback, active-content warning. Settings: deep-equality dirty, `beforeNavigate` guard, snapshot-after-load, batch save all changed keys. **Geo/Japa University settings (ADR-063/064):** `geo_personalization.enabled` killswitch present with disable-warning dialog · `university.*` settings (commission tiers, pro stream rate, payout threshold/period, surface_domain) all present and editable · JSON-typed settings validated before save · `university.surface_domain` change shows a warning (affects Learn app host detection).

## G. API Consumption, SPA, Bits UI, Svelte 5, A11y, i18n
Apply `jc-admin-audit` §§C, E, and `jc-main-audit` §§D, B(runes), H, I. Plus: env badge correct · monospace on IDs/keys/args · health cards show last-updated timestamp.

## Report Format
```
## Audit Report — Control Panel (apps/control)
**Files Audited:** [n] · **Status:** PASS / NEEDS FIXES
### Critical (security + RBAC CRUD gaps block deployment) / High / Medium / Low
### RBAC CRUD Matrix
[entity × routes table]
### Compliance Summary
| Security (keys, permissions) | ✅/❌ |
| Polling reliability | ✅/❌ |
| RBAC CRUD completeness | ✅/❌ |
| DataTable spec | ✅/❌ |
| LayerChart | ✅/❌ |
| Settings/toggle integrity | ✅/❌ |
| SPA · Bits UI · Svelte 5 · A11y | ✅/❌ |
```
