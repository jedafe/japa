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
Deep-equality `$derived` dirty tracking · `beforeNavigate` guard · JSON validation `$effect` · batch save.

## 6. Toggle Hardening
Add `toggling` guards, in-flight disable, optimistic + rollback to all extension/feature-flag toggles.

## 7. Standard Migrations
Svelte 4→5, Bits UI adoption, Tailwind v4, i18n, a11y — apply `jc-main-refactor` §§1–2, 5.
