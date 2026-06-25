# ADR-013: Runtime-Resolved i18n Over Compile-Time Bundling

**Status:** Accepted (supersedes an earlier recommendation in the same planning session)

**Context:** Paraglide JS (compile-time message extraction) was the initial recommendation for SvelteKit i18n. Mid-session, the requirement changed: admins need to create new language packs at runtime, auto-populate them via AI, and have assigned user groups edit strings — none of which can require a code deploy.

**Decision:** Drop Paraglide. Use a runtime-loadable i18n library (`svelte-i18n` or equivalent) that resolves translation values from a database-backed dictionary, cached in Redis, rather than bundling translations into the JS build.

**Rationale:** Compile-time tools are fundamentally incompatible with "create and edit a language without redeploying." This was a direct consequence of a scope change, not a reversal of judgment about Paraglide's quality for a static-locale use case.

**Consequences:** Slightly more runtime overhead than compile-time resolution would have given, accepted as the necessary tradeoff for admin-managed dynamic language packs.
