# ADR-005: Geo-First Space System

**Status:** Accepted

**Context:** The platform's core organizing principle is geography (origin country, destination country) intersected with topic.

**Decision:** Primary Space (exactly one per hierarchy level, changeable on relocation) plus unbounded Followed Spaces, reusing the existing universal Follow infrastructure rather than building bespoke space-following logic.

**Rationale:** Reusing Follow infrastructure for Followed Spaces avoids a parallel membership system. Treating Primary Space as singular-and-changeable models the real behavior of migration — a user's primary geo context changes when they relocate, and the system should reflect that directly rather than accumulating stale primary affiliations.

**Consequences:** Country and state/province Spaces are seeded from an external dataset (see ADR-020); city-level Spaces are deliberately not auto-seeded.
