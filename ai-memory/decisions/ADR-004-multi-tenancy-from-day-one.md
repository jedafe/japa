# ADR-004: Multi-Tenancy Baked In From Day One

**Status:** Accepted

**Context:** v1 launches with a single tenant, but the platform's longer-term trajectory includes potential multi-community expansion.

**Decision:** Every scoped table carries `tenant_id` from the first migration, even though only one tenant is seeded at launch.

**Rationale:** Retrofitting tenant scoping onto a schema after the fact, once dozens of tables and queries assume single-tenancy, is materially more expensive than building it in from the start. The marginal cost of the column and the scoping clause now is small.

**Consequences:** Every new table going forward must include `tenant_id` as a matter of course — this is now baseline schema convention, not a special case.
