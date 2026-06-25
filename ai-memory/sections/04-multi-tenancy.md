# Section 04 — Multi-Tenancy

**Status:** ✅ Locked

## Decided
`tenant_id` present on every scoped table from the first migration. Single tenant seeded at launch. This is baseline schema convention going forward — every new table needs `tenant_id` unless there's a documented reason it's intentionally global.

## See Also
ADR-004.
