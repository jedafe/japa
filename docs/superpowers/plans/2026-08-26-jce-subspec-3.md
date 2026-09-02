# JCE Sub-spec 3 — Implementation Plan

**Status:** Implementation and audit fixes completed; awaiting final end-to-end smoke test  
**Governing spec:** `docs/superpowers/specs/2026-08-25-jce-subspec-2-design.md` (Sub-spec 2/3 design)  
**Source of truth:** `.jeos/sections/30-generic-comparison-system.md`

Sub-spec 3 delivers JapaScore, eligibility, ranking, explainability, and recommendations on top of the Sub-spec 1/2 foundation. Most of the schema, backend, and UI scaffolding already exists; this plan tracks the remaining hardening, wiring, and gap-filling work needed to ship.

---

## Phase 0 — Read the spec (done)

- `docs/superpowers/specs/2026-08-25-jce-subspec-2-design.md` already contains detailed Sub-spec 3 design (sections 6, 7, 8).
- `.jeos/sections/30-generic-comparison-system.md` defines the base schema and forward-compatibility notes.

---

## Phase 1 — Schema / sqlc verification

**Goal:** ensure the database and generated code match the design.

- [ ] Run `make sqlc-gen` and confirm `internal/db/gen/*.go` is current.
- [ ] Confirm `migrations/000540` created `comparison_content_metadata.smart_mode` and `score_version`.
- [ ] Confirm `migrations/000553` seeded `config` keys `profile_id`, `weights_snapshot`, `recommendation_summary`, `last_scored_at`.
- [ ] Confirm `migrations/000556` added `UNIQUE (tenant_id, content_id, item_id, factor_id, profile_id)` and that `internal/db/queries/jce.sql` `UpsertComparisonScore` uses the same `ON CONFLICT` target.
- [ ] If any drift is found, add a follow-up migration (`000557` or later) to reconcile.

---

## Phase 2 — Scoring engine hardening

**Goal:** make `internal/jce/scoring/engine.go` spec-compliant and production-robust.

- [ ] Review `Score()` transaction ordering: avoid unnecessary recomputation when `force=false` by reading the cache first.
- [ ] Confirm weight-map math: keep the current `Σ(score×weight) / Σ(weight)`; explicitly normalize `weights_snapshot` to sum to 1 before persisting.
- [ ] Missing inputs: every `data_source` must use `factor.config.missing_score` and produce a user-facing explanation (`No data` warning).
- [ ] Eligibility: `penalty_ineligible` defaults to `0`, `penalty_limited` to `50`; ensure admin editor exposes these.
- [ ] Cache invalidation: when a profile is updated/deleted/set-default, also clear `jc:jce:score:*` keys.
- [ ] Backfill `engine_test.go` for missing-input handling, eligibility, smart-mode multipliers, and recommendation template triggering.

---

## Phase 3 — API routes, feature flags & permissions

**Goal:** confirm the backend contract is fully wired and gated.

- [ ] Verify `internal/server/server.go` applies `featureMiddleware` to the `/api/v1/jce` group.
- [ ] Verify `feature.jce.profiles`, `feature.jce.smart_compare`, `feature.jce.scoring` are registered.
- [ ] Confirm `migrations/000555` RBAC nodes are seeded and grants applied.
- [ ] Decide and document guest/anonymous scoring behavior (current code requires `userID.Valid`).
- [ ] Verify `PreviewScoreForItems` permission/feature gates match the product spec.

---

## Phase 4 — Admin UI hardening

**Goal:** finish the admin score-factor and eligibility-rule editors.

- [ ] In `apps/admin/src/routes/comparison-types/+page.svelte`, replace any remaining raw JSON fields for `score_factors.config`, `eligibility_rules`, and `recommendation_templates` with structured form controls.
- [ ] Ensure `buildTypeConfig()` persists `default_weights`, `allowed_goals`, `eligibility_rules`, `recommendation_templates`, and `smart_modes`.
- [ ] Confirm `ProfileFormFields.svelte` exists and is usable.
- [ ] Verify `ScoreFactorAdmin` interface matches the backend model.

---

## Phase 5 — JCE UI completion

**Goal:** complete the user-facing smart comparison and score experience.

- [ ] `ScoreExplanation.svelte`: add delta display (`+` / `−`) and a `No data` warning when `score_value == missing_score`.
- [ ] `RecommendationPanel.svelte`: render `action_type` as chips (`apply`, `book`, `review`, `check`) with `action_url` links.
- [ ] `ProfileDrawer.svelte`: add a per-factor `preferences` editor so users can tune weights (or explicitly decide this is admin-only and document).
- [ ] Audit `apps/main/src/lib/i18n/{en,fr}.json` and add all missing `jce.*` keys.

---

## Phase 6 — Resolver `score_inputs` coverage

**Goal:** ensure every resolver emits the `score_inputs` keys the engine expects.

- [ ] Verify resolvers for `space`, `city`, `job_listing`, `visa`, `mentor`, `course`, `languagetest`, `scholarship`, and `content` populate the right keys.
- [ ] Add `organization` / `bank` resolver if it does not exist, or confirm the generic `db.go` resolver covers it.
- [ ] Update resolver tests.

---

## Phase 7 — Verification & launch

**Goal:** prove the end-to-end flow works.

- [ ] `go test ./internal/jce/... ./internal/comparison/...`
- [ ] `make vet`
- [ ] `bun run check` and `bun run build` in `apps/main` and `apps/admin`
- [ ] `bun run test` in `apps/main` and `apps/admin`
- [ ] End-to-end smoke test:
  1. Enable JCE feature flags.
  2. Create a profile and a quick comparison.
  3. Toggle a smart mode and score.
  4. Confirm `comparison_scores` and `comparison_recommendations` rows are written.
  5. Confirm score badge, explanation, and recommendations render in `apps/main`.
- [ ] Update catalog docs (`api-routes-catalog.md`, `valkey-keys-catalog.md`, `system-settings-catalog.md`) only if new routes or keys are introduced.

---

## Out of scope

- Data-freshness/ingestion tables and AI decision-agent tools (Sub-spec 4).
- New content creation / promotion flows (already in Sub-spec 1).

---

## Notes

- All JEOS rules (`AGENTS.md`, `jc-backend-develop` skill) apply: full RBAC CRUD for any new entity, catalog updates for new routes/keys, and no hand-editing of generated `internal/db/gen` files.
- This plan will be updated as implementation progresses.
