# ADR-020: Country and State Auto-Seeded, City-Level Spaces Manual-Only

**Status:** Accepted

**Context:** The Geo-First Space System (ADR-005) needs canonical geographic data. Auto-seeding every city globally would create potentially tens of thousands of mostly-empty Space records with no community behind them.

**Decision:** Seed Spaces directly from the open-source Countries States Cities dataset at country and state/province level only, at launch. City-level Spaces are never auto-seeded — they're created by an admin action on demand, when there's actual community demand for that specific city.

**Rationale:** A separate `geo_regions` reference table was considered and rejected — Spaces are already the user-facing geo entity, so seeding them directly avoids a parallel data structure. Limiting auto-seed to country/state avoids the empty-records problem while still giving every user a meaningful Primary Space option immediately at launch.

**Consequences:** Admin Panel needs a "create city space under this state/country" action as part of Space management (Section 22).
