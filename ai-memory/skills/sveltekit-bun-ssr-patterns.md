# Skill: SvelteKit + Bun SSR Patterns

## Three separate apps, one shared session
Main site (SSR), admin SPA, control panel SPA are three distinct SvelteKit apps, sharing session/auth via `domain=.japaconnect.com` scoped cookies where they live on subdomains.

## Metadata resolution
Every SSR page load that renders a publicly shareable entity should call the shared metadata resolver (Section 26) to populate OG tags — don't write per-template metadata logic; it drifts out of sync with the canonical resolver over time.

## i18n at request time
Translation dictionaries load per-request based on the URL locale prefix (`/fr/...` vs. unprefixed default), pulled from Redis cache, not bundled at build time — see ADR-013 for why this is runtime-resolved rather than compile-time.

## Library compatibility
Spot-check Bun compatibility before adding a new Node-ecosystem dependency to any of the three frontend apps — not every package guarantees Bun runtime support.
