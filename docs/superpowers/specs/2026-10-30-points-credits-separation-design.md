# Points ⇄ Credits Mutual Exclusivity — Design

**Date:** 2026-10-30
**Status:** Approved direction, pending implementation plan
**JEOS impact:** §28 (Wallet & Credits), §58 (Credit Currency System), §125 (Donations & Tips), ADR-024, ADR-028 — plus a new ADR recording the separation decision.

## 1. Goal

Make Points and Credits two fully independent currencies:

| | Points | Credits |
|---|---|---|
| Earned by | On-site activity & engagement via `activity_point_rules` (admin-configurable, signed values) | Voluntary tips received (credits/fiat-settled), credit bundle purchases, admin grants |
| Spent on | **Nothing** — pure reputation: leaderboard rank, profile boost, badge/challenge evaluation | On-site transactions (tips, purchases) and withdrawals per §58 |
| Direction | Up on engagement, down on negative signals (spam, downvotes, upheld reports) | Monotonic per ledger semantics |

**No conversion exists in either direction.** The legacy one-way Points→Credits conversion (ADR-024) is retired entirely.

## 2. Current State Audit

- `internal/wallet` — `ConvertPointsToCredits` service method, `ConvertPoints` handler, `POST /me/wallet/convert` route (perm `wallet.points.convert`), `wallet_settings.points_to_credits_rate` column, `wallet.points_to_credit_rate` + `wallet.min_convert_points` settings keys, `DefaultPointsToCreditsRate`/`DefaultMinConvertPoints` consts. `OperationConvert` ledger op exists for historical entries.
- `internal/tips` — `method='points'` is a first-class settlement method: `tips.points_amount` column, `tip_settings.accept_points` column, `tips.points_daily_limit` + `tips.points_daily_recipient_limit` settings, points branches in `createWalletTip`/`settleWalletTip`, points formatting in notifications, 429 error mappings.
- Points economy — `activity_point_rules` (signed `point_value`, `cap_per_day`, `cap_per_week`, `one_time`, `is_enabled`) drives `ProcessActivityEvent` → `wallet_ledger` entries (`operation='earn'`, signed amount). Negative values already flow through correctly.
- Existing emitters: `content.created`, `comment.created`, `follow.created`, `space.joined`, `assessment.completed`, `scam_alert.confirmed`, `achievement.created`.
- Reserved-but-unemitted keys in `internal/wallet/wallet.go`: `reaction.added`, `content.flagged_removed`, `spam.detected`, `referral.completed`, `daily.login`, `profile.completed`.
- Gamification: badges/challenges already award `reward_points` via `Service.AwardPoints` → leaderboard reads the points ledger. Negative earn entries lower rank automatically.
- Referral service currently rewards **credits** (`referral.reward_credits`) — left intact (admin can set 0); new `referral.completed` activity event adds the points path.

## 3. Retire Conversion (route removal — 404)

Backend removals:

- `internal/server/server.go`: delete `POST /me/wallet/convert` route + its `wallet.points.convert` permission + idempotency middleware wiring; update the `SetSettings` comment.
- `internal/wallet/handlers.go`: delete `ConvertPoints` handler; strip `points_to_credits_rate`, `min_convert_points` from `GetMemberSettings`/`GetSettings`/`UpdateSettings` (and their input structs). `GET /me/wallet/settings` is kept, returning earn-cap fields only.
- `internal/wallet/service.go`/`store.go`: delete `ConvertPointsToCredits`, `ConvertPointsToCreditsInput`, conversion-rate resolution in `getWalletSettings`, `DefaultPointsToCreditsRate`, `DefaultMinConvertPoints`.
- Keep `OperationConvert` constant for rendering historical ledger rows.
- Catalogs: remove `wallet.points_to_credit_rate`, `wallet.min_convert_points` from `internal/control/settings_catalog.go` and `internal/admin/settings.go`.
- `api-routes-catalog.md` / `system-settings-catalog.md` (JEOS conventions): remove the route + keys.

## 4. Remove Points From Tips

- `internal/tips/tips.go`: remove `MethodPoints`, `PointsAmount` fields from input/DTO/stats types.
- `internal/tips/create.go`: `createWalletTip` becomes credits-only; delete the points branch, `AcceptPoints` gate, and daily-points-limit params.
- `internal/tips/settle.go`/`settle_svc.go`: remove `pointsDailyLimit`/`pointsDailyRecipientLimit` logic, `pointsFor`, and the points notification formatting. Refund/clawback paths reject `method='points'` tips (`tip_method_retired`) — legacy rows are kept for audit but cannot be refunded.
- `internal/tips/settings.go`: remove `AcceptPoints` from the settings struct/DTOs.
- `internal/tips/handlers.go`: remove `daily_points_limit`/`daily_recipient_points_limit` 429 mappings.
- Settings catalog: remove `tips.points_daily_limit`, `tips.points_daily_recipient_limit`.
- The `tips_method_check` CHECK constraint keeps `'points'` allowed so historical rows remain valid; no new ones can be created in code.

## 5. Points Emission Coverage — All Site Activities

New activity keys (constants in `wallet.go`) + emitter wiring. Emitters use the existing `ActivityEmitter` interface (`EmitActivityEvent` → `EnqueueProcessActivityEvent`, fire-and-forget per Rule 8).

### Positive events

| Activity key | Emitter site | Proposed default |
|---|---|---|
| `user.registered` | `auth.Register` + OAuth first-signup path | +10, `one_time` |
| `daily.login` | All `last_login_at` update sites (`auth/handlers.go` ×3, `oauth_handlers.go` ×1). Reference = UTC date → once/day via ledger idempotency-key dedupe | +1, `cap_per_day` 1 |
| `profile.completed` | onboarding/profile-completion checkpoint | +15, `one_time` |
| `referral.completed` | referral reward settlement (`referral/service.go`) | +25 |
| `reaction.added` | positive reaction/upvote received on content or comment → **author** | +1, `cap_per_day` 50 |
| `academy.lesson_completed` | `academy` `UpdateProgress`/`SubmitQuiz` → `status='completed'` | +5 |
| `academy.track_completed` | track-completion task path | +50 |
| `forum.solution_marked` | `forum.SolveThread` → accepted-answer author | +10 |
| `tip.sent` | tips settle path → **sender** (credits & fiat) | +5 |

Badge unlocks and challenge completions already award points via `reward_points`/`AwardPoints` — no change. Daily-login *streaks* are modeled as gamification challenges on `daily.login` (no new infrastructure).

### Points → badge unlocks (already wired)

`badge_definitions.trigger_type='lifetime_points'` + `trigger_value` already exists and is evaluated on every points ledger write (`EvaluateForWallet` → `EvaluatePointsBadges`, `gamification/service.go`). The admin badge UI already exposes the "Lifetime Points" trigger. No new work — but the implementation must not regress it when removing conversion code paths (it lives on the same `EvaluateForWallet` hook).

**Leaderboard ranking fix:** the leaderboard currently orders by `lifetime_points` (sum of *positive* earns only — `gamification.sql` query). Under the new model penalties must actually demote users, so the leaderboard switches to ordering by **current points balance** (sum of *all* signed entries). `lifetime_points` is retained for badge thresholds and the `/me/wallet` payload — the two metrics now mean different things on purpose: lifetime = total ever earned (badges), balance = current standing (rank).

### Negative events (penalties bypass the earn killswitch and daily earn cap)

| Activity key | Emitter site | Proposed default |
|---|---|---|
| `reaction.negative` | comment downvote + forum thread downvote → **author** | −1 |
| `spam.detected` | spam pipeline flag/block → content author | −20 |
| `content.flagged_removed` | moderation content-removal action → author | −10 |
| `report.upheld` | report resolved as upheld (`reports/moderation.go` resolve path) → reported user | −10 |

### Semantics for negative rules in `ProcessActivityEvent`

- Negative `point_value` skips the `EnableEarning` killswitch and the daily-earn-cap check (both are *earning* gates; penalties always apply).
- Ledger row keeps `operation='earn'` with signed (negative) `amount` — leaderboard sums and balance math need no change.
- `one_time`, `cap_per_day`, `cap_per_week` semantics apply symmetrically.
- Emitters are injected as optional deps (nil-safe) following the existing `activity`/`emitter` field pattern in each service.

## 6. Migration `000968_points_credits_separation.up/down.sql`

- `ALTER TABLE wallet_settings DROP COLUMN points_to_credits_rate`
- `ALTER TABLE tip_settings DROP COLUMN accept_points`
- `ALTER TABLE tips DROP COLUMN points_amount`
- Delete `system_settings` rows: `wallet.points_to_credit_rate`, `wallet.min_convert_points`, `tips.points_daily_limit`, `tips.points_daily_recipient_limit`
- Delete permission `wallet.points.convert` (+ `role_permissions` grants)
- Seed new `activity_point_rules` rows per §5 defaults
- Down migration restores columns (empty) and removed settings/permission rows
- Update `internal/db/queries/*.sql` (wallet + tips queries referencing dropped columns), regenerate via `make sqlc-gen` (Rule 2 — never hand-edit `internal/db/gen/`)

## 7. Frontend

- `apps/main`: delete `WalletConvertTab.svelte` + its tab registration in wallet settings; remove `points_amount`/`accept_points` from `lib/api/tips.ts` types; remove the points method from `TipDialog.svelte`; remove `accept_points` toggles in `TipsSection.svelte` + `SpaceSettings.svelte`; update `settingsLoaders.server.ts` and wallet `types.ts`; ledger UI keeps rendering `convert`/`points` history.
- `apps/admin`: remove conversion-rate/min-convert fields from the wallet page (`apps/admin/src/routes/(app)/wallet/+page.svelte`) and `api.ts`/`api/types.ts`.
- Tooling per Rule 18: `bun`/`bunx` only.

## 8. JEOS Updates (before/with code — JEOS first)

- Amend §28: remove conversion route/settings; document separation.
- Amend §58: note credits have no points provenance.
- Amend §125: tips settle credits/fiat only; sender earns `tip.sent` points.
- New ADR (e.g. ADR-136) superseding ADR-024's one-way conversion clause and amending ADR-028's "points serve double duty" rationale — points are now reputation-only.
- Update `conventions/api-routes-catalog.md`, `conventions/system-settings-catalog.md`, permission catalog.

## 9. Verification

- `go build ./...`, `go test ./internal/wallet ./internal/tips ./internal/gamification`
- `make sqlc-gen` clean regen; `migrate` up/down on a dev DB
- `bun run check` in `apps/main` and `apps/admin`
- Manual: downvote a comment → author loses points; settle credit tip → sender gains `tip.sent` points; `POST /me/wallet/convert` → 404.
