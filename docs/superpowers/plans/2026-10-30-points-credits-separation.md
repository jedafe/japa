# Points ⇄ Credits Separation Implementation Plan

> **For agentic workers:** Implement task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Retire points→credits conversion, remove points as a tip method, make points a pure activity/reputation currency (earned on engagement, deducted on negative signals, feeding leaderboard rank + badge unlocks) across every japa.to surface.

**Spec:** `docs/superpowers/specs/2026-10-30-points-credits-separation-design.md`

**Architecture:** Signed `point_value` in `activity_point_rules` already flows through `wallet_ledger` (`operation='earn'`, signed amount). Work = (a) delete conversion code paths, (b) delete points tip method, (c) wire `ActivityEmitter` emitters into auth/comments/forum/content/academy/tips/referral/spam/moderation/onboarding, (d) switch leaderboard to current-balance ordering, (e) schema migration + sqlc regen, (f) frontend + JEOS updates.

## Global Constraints

- Rule 2: never hand-edit `internal/db/gen/` — edit `internal/db/queries/*.sql`, run `make sqlc-gen`.
- Rule 4: Valkey keys keep `jc:` prefix (no new keys needed).
- Rule 7: reuse existing `wallet:process_activity_event` task — no new Asynq task names.
- Rule 8: all emit calls are fire-and-forget; never block request paths.
- Rule 16: no hardcoded frontend data — all fields come from API.
- Rule 17: no AI attribution anywhere (commits, comments, docs).
- Rule 18: `bun`/`bunx` only in `apps/*`.
- Emitter pattern (mirror `internal/comments/comments.go:65-85`): `SetActivityEmitter(wallet.ActivityEmitter)` + nil-safe `emitActivity(ctx, userID, key, reference, payload)` that calls `EmitActivityEvent` then `EnqueueProcessActivityEvent`, logs-and-swallows errors.
- Idempotency: `activity::{user}:{key}:{reference}` dedupe — unique reference per logical event (e.g. `daily.login` → UTC date; `tip.sent` → tip ID; `reaction.negative` → `commentID:voterID`).

---

### Task 1: Migration `000968` + sqlc query updates + regen

**Files:**
- Create: `migrations/000968_points_credits_separation.up.sql`, `migrations/000968_points_credits_separation.down.sql`
- Modify: `internal/db/queries/wallet.sql`, `internal/db/queries/tips.sql`, `internal/db/queries/gamification.sql`

- [ ] **Step 1: Write the up migration**

```sql
-- 000968_points_credits_separation.up.sql
-- Points/Credits mutual exclusivity: drop the points→credits conversion
-- settings column and points-as-tip-method columns; retire the conversion
-- permission and settings keys; seed the new signed activity_point_rules
-- that cover the full activity surface (ADR-136).
BEGIN;

ALTER TABLE wallet_settings DROP COLUMN IF EXISTS points_to_credits_rate;
ALTER TABLE tip_settings DROP COLUMN IF EXISTS accept_points;
ALTER TABLE tips DROP COLUMN IF EXISTS points_amount;

DELETE FROM system_settings WHERE key IN (
    'wallet.points_to_credit_rate', 'wallet.min_convert_points',
    'tips.points_daily_limit', 'tips.points_daily_recipient_limit');

DELETE FROM role_permissions WHERE permission_id IN
    (SELECT id FROM permissions WHERE key = 'wallet.points.convert');
DELETE FROM permissions WHERE key = 'wallet.points.convert';

-- New activity rules (admins retune freely). Negative values = penalties.
INSERT INTO activity_point_rules (activity_key, point_value, is_enabled, cap_per_day, cap_per_week, description, one_time) VALUES
    ('user.registered',          10,  true, NULL, NULL, 'Points for creating an account', true),
    ('daily.login',               1,  true, 1,    NULL, 'Points for logging in (once per UTC day)', false),
    ('profile.completed',        15,  true, NULL, NULL, 'Points for completing profile setup', true),
    ('referral.completed',       25,  true, NULL, NULL, 'Points when a referral completes', false),
    ('reaction.added',            1,  true, 50,   NULL, 'Points when your content receives a positive reaction', false),
    ('academy.lesson_completed',  5,  true, NULL, NULL, 'Points for completing an academy lesson', false),
    ('academy.track_completed',  50,  true, NULL, NULL, 'Points for completing an academy track', false),
    ('forum.solution_marked',    10,  true, NULL, NULL, 'Points when your reply is marked the solution', false),
    ('tip.sent',                  5,  true, NULL, NULL, 'Points for tipping a creator (sender earns)', false),
    ('reaction.negative',        -1,  true, NULL, NULL, 'Penalty when your content receives a downvote', false),
    ('spam.detected',           -20,  true, NULL, NULL, 'Penalty when your content is flagged/blocked as spam', false),
    ('content.flagged_removed', -10,  true, NULL, NULL, 'Penalty when your content is removed by moderation', false),
    ('report.upheld',           -10,  true, NULL, NULL, 'Penalty when a report against you is upheld', false)
ON CONFLICT (activity_key) DO NOTHING;

COMMIT;
```

- [ ] **Step 2: Write the down migration** — re-add the three columns (nullable/backfill defaults), re-insert the permission + settings rows, delete the seeded rules.

- [ ] **Step 3: Update `internal/db/queries/tips.sql`** — remove `accept_points` from `UpsertTipSettings` (cols 5,8,12); remove `points_amount` from `InsertTip`; delete `SumTipperPointsToday` + `SumRecipientPointsToday`; drop `points_total` from `ContentTipSummary` (check callers — `handlers.go` uses `points_total` in admin stats — remove that response field).

- [ ] **Step 4: Update `internal/db/queries/wallet.sql`** — remove `points_to_credits_rate` from `UpsertWalletSettings` (lines ~142-148).

- [ ] **Step 5: Update `internal/db/queries/gamification.sql` leaderboard** — `LeaderboardAllTime`: `SUM(wl.amount) FILTER (WHERE wl.amount > 0)` → `SUM(wl.amount)`, alias `lifetime_points` → `points_balance`; same for `LeaderboardByPeriod` (`period_points` keeps name but sums all signed amounts in-window). Update the comment block. Check `GetUserLifetimePoints` stays unchanged (badge thresholds keep positive-only semantics).

- [ ] **Step 6: Run `make sqlc-gen`** — expect compile breakage downstream; Tasks 2-4 fix it.

---

### Task 2: Wallet — retire conversion + negative-rule semantics

**Files:**
- Modify: `internal/wallet/service.go`, `internal/wallet/wallet.go`, `internal/wallet/handlers.go`, `internal/wallet/store.go`, `internal/server/server.go`, `internal/control/settings_catalog.go`, `internal/admin/settings.go`

- [ ] Delete `ConvertPointsToCredits` + `ConvertPointsToCreditsInput` (service.go:378-550), `GetWalletSettingsWithMinConvert` (760-776), `DefaultPointsToCreditsRate`/`DefaultMinConvertPoints` consts, `PointsToCreditsRate` from `UpdateWalletSettingsInput` + `UpdateWalletSettings` validation + `upsertWalletSettings` param; simplify `GetWalletSettings` no-row fallback to `{DailyEarningCap:1000, EnableEarning:true}`.
- [ ] `handlers.go`: delete `ConvertPoints`; `GetSettings` returns `{data: settings}` (no min_convert); `GetMemberSettings` drops `points_to_credits_rate`/`min_convert_points`, keeps `daily_earning_cap`, `enable_earning`, `min_purchase_credits`, `min_withdrawal_credits`; `UpdateSettings` drops the rate field + validation.
- [ ] `wallet.go`: remove the two consts; update package doc comment (no more conversion); add new activity-key constants: `ActivityKeyUserRegistered="user.registered"`, `ActivityKeyDailyLogin` (exists), `ActivityKeyProfileCompleted` (exists), `ActivityKeyReferralCompleted` (exists), `ActivityKeyReactionAdded` (exists), `ActivityKeyAcademyLessonCompleted="academy.lesson_completed"`, `ActivityKeyAcademyTrackCompleted="academy.track_completed"`, `ActivityKeyForumSolutionMarked="forum.solution_marked"`, `ActivityKeyTipSent="tip.sent"`, `ActivityKeyReactionNegative="reaction.negative"`, `ActivityKeySpamDetected` (exists), `ActivityKeyContentFlagged` (exists), `ActivityKeyReportUpheld="report.upheld"`.
- [ ] `server.go`: delete the `/me/wallet/convert` route block (line ~5091) + fix `SetSettings` comment (912).
- [ ] `settings_catalog.go`: delete lines for `wallet.min_convert_points`, `wallet.points_to_credit_rate`, `tips.points_daily_limit`, `tips.points_daily_recipient_limit` (grep — also check `tips.*` entries).
- [ ] `internal/admin/settings.go` (~574-576): remove `min_convert_points`, `points_to_credit_rate` field defs.
- [ ] **Negative semantics** in `ProcessActivityEvent` (service.go:100-246): when `rule.PointValue < 0`, skip the `EnableEarning` killswitch AND the daily-cap block (wrap both checks in `if rule.PointValue > 0 { ... }`). Frequency caps and `one_time` still apply.

---

### Task 3: Tips — remove points method

**Files:** `internal/tips/{tips.go,create.go,settle.go,settle_svc.go,settings.go,handlers.go,lists.go,store.go}` + `internal/server/server.go` (stats/DTO usages)

- [ ] `tips.go`: delete `MethodPoints`, `ErrDailyPointsLimit`, `ErrDailyRecipientLimit`; `IsValidMethod` drops `MethodPoints`; remove `PointsAmount` from `Tip`, `CreateTipInput`; remove `AcceptPoints` from `Settings`, `UpdateSettingsInput`.
- [ ] `create.go`: `switch in.Method` — only `MethodCredits` → `createWalletTip`; inside it: delete points branch (`amount,ledgerType,accepted = in.CreditsAmount,"credits",st.AcceptCredits`), delete daily-limit reads + params, delete `pointsFor`/`genericAmount` points arm; after `settleWalletTip` success emit `tip.sent` (Task 4).
- [ ] `settle.go`: `settleWalletParams` drop `pointsDailyLimit`/`pointsDailyRecipientLimit`; delete the two cap blocks (lines 96-117); `InsertTipParams` drops `PointsAmount`; delete `pointsFor`; `clawbackTip`: delete `MethodPoints` branches (lines 299-302, 308, 343-346) — ledgerType for sender refund = `tip.Method` only when `MethodCredits`; legacy `method='points'` rows: clawAmount=0 → marks refunded with no ledger writes (acceptable — column is gone).
- [ ] `settle_svc.go`: `notifyClawback` — drop points `unit` branch; `tipDisplayAmount` — drop `MethodPoints` case (legacy rows render `"a tip"`).
- [ ] `settings.go`: drop `AcceptPoints` everywhere incl. `emptySettings`, `toSettings`, `acceptedMethods` (remove `MethodPoints` append).
- [ ] `store.go`: remove `sumTipperPointsToday`/`sumRecipientPointsToday` wrappers + interface entries.
- [ ] `handlers.go`: delete `ErrDailyPointsLimit`/`ErrDailyRecipientLimit` 429 mappings (~71-73); drop `points_amount` from create input parse + `points_total`/`points_amount` from stats/serialize code (~455); any admin stats field.
- [ ] `lists.go`: remove `points` handling (check `points_amount` in serialization).
- [ ] Update `create_test.go`, `settings_test.go`, `settle_db_test.go`, `oc_test.go` — remove points-path tests, keep credits/fiat coverage.

---

### Task 4: Tips — emit `tip.sent` for the sender

**Files:** `internal/tips/{tips.go,create.go,settle_svc.go}`, `internal/server/server.go`

- [ ] Add to `Service`: `activity wallet.ActivityEmitter` + `SetActivityEmitter(e)` + `emitActivity` helper mirroring comments' implementation (nil-safe, fire-and-forget). Import `internal/wallet`.
- [ ] `createWalletTip` after settle: `s.emitActivity(ctx, senderID, wallet.ActivityKeyTipSent, tip.ID.String(), nil)`.
- [ ] `SettleFiatTip` after `s.store.settleFiatTip` success: `s.emitActivity(ctx, tip.SenderUserID, wallet.ActivityKeyTipSent, tipID.String(), nil)`.
- [ ] `server.go`: `tipsService.SetActivityEmitter(walletService)` next to existing `Set*` calls.

---

### Task 5: Emitters — auth (`user.registered`, `daily.login`)

**Files:** `internal/auth/handlers.go`, `internal/auth/oauth_handlers.go`, `internal/auth/*.go` (Service/Handlers struct), `internal/server/server.go`

- [ ] Add emitter seam to `AuthHandlers` and `OAuthHandlers` (field + `SetActivityEmitter` + `emitActivity` helper — check existing struct layout; both may share a struct or need separate injection).
- [ ] `user.registered`: emit after successful user creation in `Register` (ref = `user.ID`) and in the OAuth first-signup path (oauth_handlers.go — find where new users are created).
- [ ] `daily.login`: emit at each `UpdateLastLoginAt` site (handlers.go ~595, ~1022, ~1152; oauth_handlers.go ~690) with reference `time.Now().UTC().Format("2006-01-02")` — idempotency dedupes to once/day.
- [ ] `server.go`: wire `authHandlers.SetActivityEmitter(walletService)` / OAuth equivalent.

---

### Task 6: Emitters — reactions & forum

**Files:** `internal/content/` (AddReaction service path), `internal/comments/store.go`, `internal/forum/` (Vote + SolveThread), `internal/server/server.go`

- [ ] `reaction.added`: in content `AddReaction` service — emit for the **content author** (fetch `author_user_id`; skip self-reactions). Reference: `reaction:{contentID}:{reactorID}:{reactionTypeID}` so a re-react dedupes but a different content doesn't.
- [ ] `reaction.negative`: `comments.Service.VoteComment` (store.go:898) — when `value == -1` emit for `target.AuthorUserID`, ref `comment:{commentID}:down:{userID}`. Forum `Vote` (`forum.Service.Vote`) — when `input.Value == VoteDown`, resolve entity author (thread/reply) and emit, ref `forum:{entityID}:down:{voterID}`.
- [ ] `reaction.added`: also emit on `value == 1` upvotes for comment + forum authors (refs mirror the negative ones).
- [ ] `forum.solution_marked`: in `SolveThread` (handlers.go:1459 → service) — emit for the accepted-reply author.
- [ ] Comments/forum services already have emitter seams? Comments has `activity wallet.ActivityEmitter` + `emitActivity`. Forum — check/add `SetActivityEmitter`. Content — already has `SetActivityEmitter`. Wire any missing in server.go.

---

### Task 7: Emitters — academy, referral, onboarding/profile, spam, moderation

**Files:** `internal/academy/handlers.go` (+service), `internal/referral/service.go`, `internal/onboarding` or profile-completion site, `internal/spam/processoradapter.go`, `internal/reports/moderation.go`, `internal/server/server.go`

- [ ] `academy.lesson_completed`: in `UpdateProgress` (handlers.go:396) + `SubmitQuiz` completion path (~1051, ~1249) — emit when the progress row transitions to `status='completed'` (guard re-completion: emit only when previous status ≠ completed). Ref = `lesson:{lessonID}` (one_time rule gives dedupe anyway; still use stable ref).
- [ ] `academy.track_completed`: at the track-completion update site (worker task path — `enqueueTrackCompletionUpdate` consumer or the sync fallback) — emit for the learner, ref = `track:{trackID}`.
- [ ] `referral.completed`: in `internal/referral/service.go` reward path (~line 498 where `rewardCredits` granted) — emit for the referrer, ref = referral ID.
- [ ] `profile.completed`: locate the profile-completion checkpoint (`internal/onboarding` or `users` profile update) — emit when completeness first reaches 100%/onboarding finishes, ref = userID.
- [ ] `spam.detected`: `processoradapter.go` — when `result.Blocked || result.Flagged`, resolve the content author and emit, ref = `{entityType}:{entityID}`.
- [ ] `report.upheld` + `content.flagged_removed`: `reports/moderation.go resolveWithAction` — for `warn`/`ban` emit `report.upheld` for `target`; for `remove` resolve entity author via `ResolveTargetUser` and emit `content.flagged_removed`; ref = `report:{id}`.
- [ ] Wire all new seams in `server.go`.

---

### Task 8: Frontend — apps/main

**Files:** `apps/main/src/lib/components/hub/settings/sections/wallet/*`, `apps/main/src/lib/api/tips.ts`, `apps/main/src/lib/components/tips/TipDialog.svelte`, `apps/main/src/lib/components/hub/settings/sections/TipsSection.svelte`, `apps/main/src/lib/components/spaces/SpaceSettings.svelte`, `apps/main/src/lib/components/hub/settings/settingsLoaders.server.ts`, `apps/main/src/routes/(app)/me/tips/+page.svelte`

- [ ] Delete `WalletConvertTab.svelte` + its tab registration (check `sections/wallet/` index/tabs file); strip `points_to_credits_rate`/`min_convert_points` from `types.ts`.
- [ ] `tips.ts`: remove `accept_points`, `points_amount`, `points_total` fields from types.
- [ ] `TipDialog.svelte`: remove the points method option (methods come from `PublicConfig.methods` — should disappear automatically once backend stops advertising; verify no hardcoded `'points'`).
- [ ] `TipsSection.svelte` + `SpaceSettings.svelte`: remove the `accept_points` toggle.
- [ ] `me/tips/+page.svelte`: remove points display for `method==='points'` rows (show "points (retired)" label or just the method name — check current rendering).
- [ ] `bun run check` in apps/main.

---

### Task 9: Frontend — apps/admin

**Files:** `apps/admin/src/routes/(app)/wallet/+page.svelte`, `apps/admin/src/lib/api.ts`, `apps/admin/src/lib/api/types.ts`

- [ ] Remove conversion-rate + min-convert form fields and the `points_to_credits_rate`/`min_convert_points` type fields.
- [ ] Remove `accept_points`/`points_amount`/`points_total` from tip types; remove "points" from method filter options if hardcoded.
- [ ] `bun run check` in apps/admin.

---

### Task 10: JEOS updates

**Files:** `.jeos/sections/28-wallet-credits.md`, `.jeos/sections/58-credit-currency-system.md`, `.jeos/sections/125-donations-tips.md`, new `.jeos/adr/ADR-136-points-credits-separation.md`, `.jeos/conventions/api-routes-catalog.md`, `.jeos/conventions/system-settings-catalog.md`, `.jeos/sections/03-rbac.md` (permission list)

- [ ] §28: remove the `/me/wallet/convert` route + conversion settings; document two-currency independence + negative rules + balance-based leaderboard.
- [ ] §125: methods are `credits` + fiat + `opencollective`; sender earns `tip.sent` points; drop points-limit rows from the settings table.
- [ ] §58: one line noting credits have no points provenance.
- [ ] ADR-136: decision record superseding ADR-024's one-way conversion clause and amending ADR-028's dual-duty rationale.
- [ ] Catalogs: delete `/me/wallet/convert` route row, `wallet.points.convert` permission, four retired settings keys; add the 13 new `activity_point_rules` keys if a rules catalog exists.
- [ ] JEOS is a submodule — commit inside `.jeos`, then bump the submodule pointer commit in the main repo.

---

### Task 11: Verify

- [ ] `go build ./...`
- [ ] `go test ./internal/wallet ./internal/tips ./internal/gamification ./internal/comments ./internal/forum`
- [ ] `git grep -n "points_to_credits\|PointsToCredits\|min_convert\|points_to_credit_rate\|points_amount\|accept_points\|MethodPoints\|SumTipperPoints\|SumRecipientPoints"` → only migration files + history references
- [ ] `bun run check` apps/main + apps/admin clean
- [ ] Commit each task group separately with conventional-commit messages (no AI attribution, main branch).
