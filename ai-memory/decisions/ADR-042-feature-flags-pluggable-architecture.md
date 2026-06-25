# ADR-042: Feature Flags & Pluggable Architecture

**Status:** Accepted

**Context:** Admin needs to turn platform features on/off without causing site breakage. Features like Blog, Forum, DMs, Wallet, Learning Platform, and Advertising should be independently toggleable. When disabled, no null-pointer errors, no broken navigation, no error pages — just a clean 404.

**Decision:** `feature_flags (key, tenant_id, enabled, config)` table seeded with all non-core features. A `FeatureGate` middleware wraps all feature-gated API routes — disabled features return HTTP 404 with `{"error":"feature_disabled"}` (not 403, to not reveal the feature exists). SvelteKit route guards read features from page data (loaded once per session) and `throw redirect(302, '/')` for disabled features. Navigation items filtered by feature state in `getNavItems()`. Admin Settings organised into 13 named categories with dedicated pages — never a flat wall of settings.

**Rationale:** 404 over 403 for disabled features is a deliberate privacy choice — third parties (scrapers, competitors) should not be able to discover which features a tenant has disabled. The seeded feature registry as a DB table (not an env var) means features can be toggled without a server restart.

**Consequences:** Every new feature added to the platform must be added to the `FeatureRegistry` seeder. A missing feature flag entry means a feature is always-on and cannot be toggled. This is enforced by the "new feature checklist" in `ai-memory/skills/rbac-permission-node-pattern.md`.
