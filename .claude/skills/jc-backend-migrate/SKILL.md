---
name: jc-backend-migrate
description: Use when creating, modifying, or reviewing database migrations for the JapaConnect backend (PostgreSQL 18, golang-migrate). Covers up/down pairing, zero-downtime rules, the circular FK stub pattern, and post-migration sqlc regeneration.
---

# JapaConnect Backend — Migrations Skill

High-risk activity with its own defect history (BE-003: runner applied `.down.sql` forward). Follow exactly.

## Rules of the Road
1. **Always a pair**: `NNNN_name.up.sql` + `NNNN_name.down.sql`. The down must genuinely reverse the up — write it, don't stub it with a comment.
2. **Never edit an applied migration.** Fix-forward with a new pair. The only exception is a migration that has never left your machine.
3. **Numbering**: next contiguous number; check the directory before naming — collisions break the runner.
4. **Runner sanity** (BE-003 anchor): forward runs glob `.up.sql` only. If touching the runner, add/keep the test asserting `.down.sql` files are never applied forward.
5. **After every schema change**: update the affected `.sql` query files → `sqlc generate` → commit generated code in the same PR. Schema and sqlc structs must never drift.
6. **Catalog duty**: new tables/columns that introduce settings keys or Valkey patterns update those catalogs; table-count changes noted for JEOS stat accuracy.

## Zero-Downtime Rules (Prod Is Running While You Migrate)
- Add columns as NULLable or with DEFAULT → backfill in batches (separate migration or Asynq task for big tables) → then add NOT NULL/constraints in a later migration
- Never `ALTER TYPE` / rewrite a big table in one step — new column, dual-write window, swap
- Index creation on hot tables: `CREATE INDEX CONCURRENTLY` (must be outside a transaction — mark the migration `-- +migrate NoTransaction` per runner convention)
- Dropping columns: stop reading (deploy) → stop writing (deploy) → drop (migration) — three phases, never one

## Established Patterns (Locked)
- **Circular FK users ↔ spaces**: stub table migration first, `ALTER TABLE ... ADD CONSTRAINT` after both exist. Reuse this pattern for any future cycle.
- **Wallet ledger**: append-only — migrations must never add UPDATE-enabling constructs to the transactions table; balance remains derived.
- **Seed migrations**: RBAC permission nodes seed completely (BE-005). New entity = its full node set in the same PR as its routes.
- **Honcho**: separate logical database on the same PG instance — its schema is Honcho's own; JC migrations never touch it.

## Review Checklist (When Reviewing a Migration PR)
- [ ] Pair present, down actually reverses up
- [ ] Idempotent-safe where sensible (`IF NOT EXISTS` on CREATE INDEX/TABLE in seeds)
- [ ] No data loss hidden in down (dropping a column in down is fine; dropping in UP needs the three-phase note)
- [ ] Foreign keys have ON DELETE behavior chosen deliberately (CASCADE vs RESTRICT vs SET NULL — justify in a comment)
- [ ] Big-table operations follow zero-downtime rules
- [ ] sqlc regenerated, build green
