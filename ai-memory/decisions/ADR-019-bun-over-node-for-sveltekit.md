# ADR-019: Bun Over Node for the SvelteKit Runtime

**Status:** Accepted (carried over from earlier planning)

**Context:** SvelteKit needs a JS runtime for SSR across three frontend apps (main site, admin SPA, control panel SPA).

**Decision:** Bun runtime.

**Rationale:** Performance and startup-time characteristics favorable for SSR workloads, and a faster local development loop, weighed against Node's larger ecosystem maturity.

**Consequences:** Library compatibility should be spot-checked against Bun specifically when adding new frontend dependencies, since not every Node-targeting package is guaranteed compatible. Pinned at 1.3.x (see `AGENTS.md` for the exact patch version in use) — Bun ships fast; bump deliberately, not by drifting to whatever's latest in CI.
