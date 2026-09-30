# Onboarding Completion Reminder — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Newly registered members who have not finished the 5-step onboarding wizard get a daily in-app notification reminder (max 5 sends, within 14 days of signup) plus a snoozable "finish setup" card at the top of the main-app home feed.

**Architecture:** A daily Asynq cron task (`onboarding:reminder_sweep`) scans `users ⋈ onboarding_state` for incomplete accounts inside the reminder window, upserts `reminder_count`/`last_reminder_at` on `onboarding_state`, and inserts an in-app `onboarding_reminder` notification directly (same pattern as `HandleReportNotifyAdmin` — the batch-window `DispatchNotification` path is designed for actor-driven social notifications, not system reminders). The frontend card reads `completed`/`current_step` from the existing `/onboarding/status` layout fetch — no new endpoint.

**Tech Stack:** Go, PostgreSQL, sqlc (named `@param` style), Asynq, SvelteKit/Svelte 5 runes, Bun.

## Global Constraints

- **JEOS first (Rule 1):** update `.jeos/sections/09-onboarding-flow.md`, `.jeos/sections/11-job-queue-asynq.md`, and `.jeos/conventions/system-settings-catalog.md` before/with the code they describe. `.jeos` is a submodule — changes stay in its working tree.
- **Rule 2:** never hand-edit `internal/db/gen/*` — edit `internal/db/queries/onboarding.sql` and run `make sqlc-gen`.
- **Rule 7:** `onboarding:reminder_sweep` must be added to the §11 task catalog before use in code.
- **Rule 17:** no AI/agent attribution in code, comments, or commit messages.
- **Rule 18:** Bun only for JS tooling (`bun run check`, never npm/npx).
- sqlc queries use named `@param` style (see existing `internal/db/queries/onboarding.sql`).
- `users.status` values: `'active'` is the only sendable state (`'suspended'`, `'deleted'` exist). There is no `deleted_at` column on `users` — filter on `u.status = 'active'`.
- `notification_types` column order for seed INSERT: `(key, label, description, enabled, default_in_app, default_email, default_push, category)`.
- Migration number **000759** (000758 is `wiki_page_feedback`).
- Onboarding wizard: `/onboarding`, `totalSteps = 5` (`internal/onboarding/service.go:156`, `apps/main/src/routes/onboarding/+page.svelte:123`).
- Do not commit unrelated pre-existing worktree changes; stage only files listed in each task's commit step.

## File Map

| File | Change |
|---|---|
| `migrations/000759_onboarding_reminders.up.sql` / `.down.sql` | New columns + notification_type seed |
| `internal/db/queries/onboarding.sql` | 2 new queries |
| `internal/db/gen/onboarding.sql.go` | Regenerated (never hand-edited) |
| `internal/worker/tasks.go` | `TypeOnboardingReminderSweep` + registry entry |
| `internal/worker/handlers.go` | `HandleOnboardingReminderSweep` + setting helpers |
| `internal/worker/server.go` | Scheduler `schedule(...)` line |
| `internal/control/settings_catalog.go` | 4 `onboarding.reminder_*` entries |
| `internal/onboarding/onboarding_test.go` | Unit test for the settings-parse helper |
| `.jeos/sections/09-onboarding-flow.md` | Feature doc |
| `.jeos/sections/11-job-queue-asynq.md` | Task catalog row |
| `.jeos/conventions/system-settings-catalog.md` | 4 settings rows |
| `apps/main/src/routes/+layout.server.ts` | Extend onboarding payload |
| `apps/main/src/lib/components/onboarding/OnboardingSetupCard.svelte` | New card component |
| `apps/main/src/routes/(app)/+page.svelte` | Mount card above `FeedList` |
| `apps/main/src/lib/i18n/en.json` / `fr.json` | `onboarding.reminder_*` keys |

---

### Task 1: JEOS + migration + sqlc queries + regeneration

**Files:**
- Modify: `.jeos/sections/11-job-queue-asynq.md` (task catalog — add row)
- Modify: `.jeos/sections/09-onboarding-flow.md` (append reminder paragraph)
- Modify: `.jeos/conventions/system-settings-catalog.md` (4 rows)
- Create: `migrations/000759_onboarding_reminders.up.sql`
- Create: `migrations/000759_onboarding_reminders.down.sql`
- Modify: `internal/db/queries/onboarding.sql` (append)
- Modify: `internal/control/settings_catalog.go` (append after `onboarding.tour_steps`)
- Regenerate: `internal/db/gen/onboarding.sql.go` via `make sqlc-gen`

**Interfaces:**
- Produces (consumed by Task 2):
  - `gen.ListOnboardingReminderCandidates(ctx, gen.ListOnboardingReminderCandidatesParams{WindowDays int32, MaxCount int32, MinIntervalHours int32, BatchLimit int32}) ([]pgtype.UUID, error)`
  - `gen.MarkOnboardingReminded(ctx, userID pgtype.UUID) error`
  - `gen.GetSettingValue(ctx, key string) (json.RawMessage, error)` — already exists (`internal/db/queries/settings.sql:3`), returns jsonb.

- [ ] **Step 1: JEOS §11 task catalog entry**

In `.jeos/sections/11-job-queue-asynq.md`, the catalog is a flat `task_name — description` list (e.g. `coupon:expire_check — deactivate coupons past expires_at (periodic sweep) (Section 55)` near line 133). Add a line in that block:

```text
onboarding:reminder_sweep         — daily sweep (0 9 * * *): in-app notification reminder to active users with incomplete onboarding within onboarding.reminder_window_days, under onboarding.reminder_max_count, past onboarding.reminder_min_interval_hours (Section 09)
```

- [ ] **Step 2: JEOS §09 update**

Append to `.jeos/sections/09-onboarding-flow.md` (after the product-tour paragraph):

```markdown
**Incomplete-onboarding reminders:** a daily Asynq sweep (`onboarding:reminder_sweep`, §11) notifies newly registered users who haven't finished the wizard. `onboarding_state` tracks `reminder_count`/`last_reminder_at`; reminders stop after `onboarding.reminder_max_count` sends (default 5), outside `onboarding.reminder_window_days` of registration (default 14), or when `onboarding.reminder_enabled` is false. The sweep inserts an in-app `onboarding_reminder` notification (action_url `/onboarding`) and is also surfaced in the main app as a snoozable "finish setup" feed card driven by `GET /onboarding/status`.
```

- [ ] **Step 3: Settings catalog docs**

In `.jeos/conventions/system-settings-catalog.md`, add rows to the onboarding block (match existing column layout):

```markdown
| `onboarding.reminder_enabled` | bool | `true` | Master switch for the incomplete-onboarding reminder sweep |
| `onboarding.reminder_max_count` | int | `5` | Max reminder notifications sent per user |
| `onboarding.reminder_window_days` | int | `14` | Only accounts younger than this many days are reminded |
| `onboarding.reminder_min_interval_hours` | int | `20` | Minimum hours between reminder sends (cron double-run guard) |
```

- [ ] **Step 4: Migration**

`migrations/000759_onboarding_reminders.up.sql`:

```sql
-- 000759_onboarding_reminders
-- Reminder bookkeeping for incomplete onboarding (Section 09): the daily
-- onboarding:reminder_sweep task (§11) nudges newly registered users who
-- haven't finished the wizard. reminder_count bounds sends; the
-- onboarding_reminder notification type gates user preference overrides.

BEGIN;

ALTER TABLE onboarding_state
    ADD COLUMN reminder_count int NOT NULL DEFAULT 0,
    ADD COLUMN last_reminder_at timestamptz;

INSERT INTO public.notification_types
    (key, label, description, enabled, default_in_app, default_email, default_push, category)
SELECT 'onboarding_reminder', 'Onboarding Reminder',
       'Reminder to finish setting up your account',
       true, true, false, false, 'system'
WHERE NOT EXISTS (
    SELECT 1 FROM public.notification_types WHERE key = 'onboarding_reminder'
);

COMMIT;
```

`migrations/000759_onboarding_reminders.down.sql`:

```sql
-- 000759_onboarding_reminders (down)

BEGIN;

DELETE FROM public.notification_types WHERE key = 'onboarding_reminder';

ALTER TABLE onboarding_state
    DROP COLUMN IF EXISTS reminder_count,
    DROP COLUMN IF EXISTS last_reminder_at;

COMMIT;
```

- [ ] **Step 5: sqlc queries — append to `internal/db/queries/onboarding.sql`**

```sql
-- name: ListOnboardingReminderCandidates :many
-- Active, non-completed onboarding users inside the reminder window who
-- haven't hit the send cap or the minimum interval, and who haven't opted
-- out of onboarding_reminder in-app notifications (§35 preferences).
SELECT u.id
FROM users u
LEFT JOIN onboarding_state o ON o.user_id = u.id
WHERE u.status = 'active'
  AND COALESCE(o.completed, false) = false
  AND u.created_at > now() - make_interval(days => @window_days::int)
  AND COALESCE(o.reminder_count, 0) < @max_count::int
  AND (o.last_reminder_at IS NULL
       OR o.last_reminder_at < now() - make_interval(hours => @min_interval_hours::int))
  AND NOT EXISTS (
      SELECT 1 FROM notification_preferences np
      WHERE np.user_id = u.id
        AND np.notification_type = 'onboarding_reminder'
        AND np.in_app = false
  )
ORDER BY u.created_at DESC
LIMIT @batch_limit::int;

-- name: MarkOnboardingReminded :exec
-- Upserts so users with no onboarding_state row (never started the wizard)
-- are tracked too — same pattern as DismissTour.
INSERT INTO onboarding_state (user_id, reminder_count, last_reminder_at)
VALUES (@user_id, 1, now())
ON CONFLICT (user_id) DO UPDATE SET
    reminder_count = onboarding_state.reminder_count + 1,
    last_reminder_at = now(),
    updated_at = now();
```

- [ ] **Step 6: Settings catalog — append in `internal/control/settings_catalog.go`** after the `onboarding.tour_steps` entry:

```go
	"onboarding.reminder_enabled":            {Type: "bool", DefaultValue: "true", Description: "Daily in-app reminder for users who haven't finished onboarding.", Panel: "control"},
	"onboarding.reminder_max_count":          {Type: "int", DefaultValue: "5", Description: "Max onboarding reminder notifications sent per user.", Panel: "control"},
	"onboarding.reminder_window_days":        {Type: "int", DefaultValue: "14", Description: "Only accounts younger than this many days receive onboarding reminders.", Panel: "control"},
	"onboarding.reminder_min_interval_hours": {Type: "int", DefaultValue: "20", Description: "Minimum hours between onboarding reminder sends.", Panel: "control"},
```

- [ ] **Step 7: Regenerate + build**

```bash
make sqlc-gen
gofmt -w internal/control/settings_catalog.go
go build ./internal/...
```

Expected: `gen.ListOnboardingReminderCandidates` and `gen.MarkOnboardingReminded` exist in `internal/db/gen/onboarding.sql.go`; build clean.

- [ ] **Step 8: Commit**

```bash
git add migrations/000759_onboarding_reminders.up.sql migrations/000759_onboarding_reminders.down.sql internal/db/queries/onboarding.sql internal/db/gen/onboarding.sql.go internal/control/settings_catalog.go .jeos
git commit -m "feat(onboarding): reminder sweep schema, queries, settings (§09/§11)"
```

(If `.jeos` submodule staging is undesirable, split: commit it separately inside `.jeos` per its own repo rules.)

---

### Task 2: Asynq sweep task + handler + scheduler

**Files:**
- Modify: `internal/worker/tasks.go` (const + `handlerRegistry` entry)
- Modify: `internal/worker/handlers.go` (new handler + helpers, in "Periodic sweep handlers" region ~line 4278)
- Modify: `internal/worker/server.go` (schedule line, after the other sweep registrations)

**Interfaces:**
- Consumes: `gen.ListOnboardingReminderCandidates`, `gen.MarkOnboardingReminded`, `gen.GetSettingValue`, `gen.InsertNotification` (`gen.InsertNotificationParams{RecipientID, NotificationType, ActorType *string, ActorID pgtype.UUID, SubjectType *string, SubjectID pgtype.UUID, Title string, Body *string, ActionUrl *string}` — verify exact field names from the generated file before writing).
- Package vars available in `internal/worker/handlers.go`: `dbPool` (`*db.DB`, `.Reader`/`.Writer`), `parsePayload(t, &v)`, `scanUUID(string)`.
- Produces: `HandleOnboardingReminderSweep(ctx, *asynq.Task) error`, registered as `TypeOnboardingReminderSweep = "onboarding:reminder_sweep"`.

- [ ] **Step 1: Task type — `internal/worker/tasks.go`**

In the "Periodic sweep tasks" const block, append:

```go
	TypeOnboardingReminderSweep  = "onboarding:reminder_sweep" // §09: nudge users who haven't finished onboarding
```

Add to `handlerRegistry` in the same sweep section:

```go
	{TypeOnboardingReminderSweep, HandleOnboardingReminderSweep},
```

- [ ] **Step 2: Handler — `internal/worker/handlers.go`**, in the "Periodic sweep handlers" region (next to `HandleScheduledPublishSweep`):

```go
// HandleOnboardingReminderSweep nudges newly registered users who have not
// finished the onboarding wizard (§09). Runs daily via the Asynq scheduler.
// Sends are bounded by onboarding.reminder_max_count (default 5) and only
// apply within onboarding.reminder_window_days of registration (default 14);
// onboarding.reminder_min_interval_hours (default 20) prevents double-sends
// if the cron fires twice in a day.
// Ref: japaconnect-engineering/sections/09-onboarding-flow.md
func HandleOnboardingReminderSweep(ctx context.Context, _ *asynq.Task) error {
	slog.Info("task:onboarding:reminder_sweep: starting sweep")

	if dbPool == nil {
		slog.Warn("task:onboarding:reminder_sweep: no database, skipping")
		return nil
	}

	if !onboardingReminderEnabled(ctx) {
		slog.Info("task:onboarding:reminder_sweep: disabled via onboarding.reminder_enabled")
		return nil
	}

	q := gen.New(dbPool.Reader)
	candidates, err := q.ListOnboardingReminderCandidates(ctx, gen.ListOnboardingReminderCandidatesParams{
		WindowDays:       onboardingReminderIntSetting(ctx, "onboarding.reminder_window_days", 14),
		MaxCount:         onboardingReminderIntSetting(ctx, "onboarding.reminder_max_count", 5),
		MinIntervalHours: onboardingReminderIntSetting(ctx, "onboarding.reminder_min_interval_hours", 20),
		BatchLimit:       2000,
	})
	if err != nil {
		slog.Error("task:onboarding:reminder_sweep: candidate query failed", "error", err)
		return err
	}
	if len(candidates) == 0 {
		return nil
	}

	wq := gen.New(dbPool.Writer)
	actorType := "system"
	subjectType := "onboarding"
	actionURL := "/onboarding"
	title := "Finish setting up your account"
	body := "You're almost there — finish the setup steps to get the most out of JapaConnect."
	sent := 0

	for _, userID := range candidates {
		// Mark first: a failed insert must not allow the next sweep to
		// re-send unboundedly — reminder_count is the global cap.
		if err := wq.MarkOnboardingReminded(ctx, userID); err != nil {
			slog.Error("task:onboarding:reminder_sweep: mark failed", "user_id", userID.String(), "error", err)
			continue
		}

		if _, err := wq.InsertNotification(ctx, gen.InsertNotificationParams{
			RecipientID:      userID,
			NotificationType: "onboarding_reminder",
			ActorType:        &actorType,
			ActorID:          pgtype.UUID{},
			SubjectType:      &subjectType,
			SubjectID:        userID,
			Title:            title,
			Body:             &body,
			ActionUrl:        &actionURL,
		}); err != nil {
			slog.Error("task:onboarding:reminder_sweep: insert failed", "user_id", userID.String(), "error", err)
			continue
		}
		sent++
	}

	slog.Info("task:onboarding:reminder_sweep: complete", "sent", sent, "candidates", len(candidates))
	return nil
}
```

IMPORTANT — before writing this, read `gen.InsertNotificationParams` in `internal/db/gen/notifications.sql.go` and `HandleReportNotifyAdmin` (handlers.go ~4735) and copy the exact param shapes: `ActorType` should be a `*string` pointing to `"system"` (create `actorType := "system"`), `SubjectID` is `pgtype.UUID`. Adjust the skeleton above to the generated types.

Setting helpers (same `newsletterBatchSize` pattern — `GetSettingValue` returns jsonb bytes, parse `"true"`/ints; put next to the handler):

```go
// onboardingReminderEnabled reads the onboarding.reminder_enabled setting
// (§09, default true). Missing/malformed values fall back to true.
func onboardingReminderEnabled(ctx context.Context) bool {
	if dbPool == nil {
		return true
	}
	row, err := gen.New(dbPool.Reader).GetSettingValue(ctx, "onboarding.reminder_enabled")
	if err != nil || len(row) == 0 {
		return true
	}
	return strings.Trim(string(row), `"`) != "false"
}

// onboardingReminderIntSetting reads an integer onboarding.reminder_*
// setting with clamping — invalid or non-positive values fall back.
func onboardingReminderIntSetting(ctx context.Context, key string, fallback int32) int32 {
	if dbPool == nil {
		return fallback
	}
	row, err := gen.New(dbPool.Reader).GetSettingValue(ctx, key)
	if err != nil || len(row) == 0 {
		return fallback
	}
	n, err := strconv.Atoi(strings.Trim(string(row), `"`))
	if err != nil || n <= 0 {
		return fallback
	}
	if n > 10000 {
		n = 10000
	}
	return int32(n) //nolint:gosec // bounded above
}
```

- [ ] **Step 3: Scheduler — `internal/worker/server.go`**

In `NewScheduler`, in the "Internal sweep tasks" block (after the ban/boost sweeps), add:

```go
	// §09: incomplete-onboarding reminders — daily at 09:00.
	schedule("0 9 * * *", TypeOnboardingReminderSweep, 24*time.Hour)
```

- [ ] **Step 4: Unit test — `internal/onboarding/onboarding_test.go`** is the wrong package (helpers live in worker). Add `internal/worker/reminder_settings_test.go`:

```go
package worker

import (
	"context"
	"testing"
)

// The helpers depend on dbPool which is nil in unit tests — the fallback
// path is the testable unit here.
func TestOnboardingReminderIntSetting_NilPoolFallback(t *testing.T) {
	if got := onboardingReminderIntSetting(context.Background(), "onboarding.reminder_max_count", 5); got != 5 {
		t.Fatalf("got %d, want 5", got)
	}
}

func TestOnboardingReminderEnabled_NilPoolFallback(t *testing.T) {
	if !onboardingReminderEnabled(context.Background()) {
		t.Fatal("expected enabled fallback with nil pool")
	}
}
```

- [ ] **Step 5: Verify**

```bash
gofmt -w internal/worker/
go build ./internal/...
go test ./internal/worker/ -run TestOnboardingReminder -v
```

Expected: build clean, both tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/worker/tasks.go internal/worker/handlers.go internal/worker/server.go internal/worker/reminder_settings_test.go
git commit -m "feat(onboarding): daily reminder sweep task for incomplete onboarding (§09/§11)"
```

---

### Task 3: Frontend — layout payload + feed card + i18n

**Files:**
- Modify: `apps/main/src/routes/+layout.server.ts` (~line 152-168, the onboarding `apiGet` block)
- Create: `apps/main/src/lib/components/onboarding/OnboardingSetupCard.svelte`
- Modify: `apps/main/src/routes/(app)/+page.svelte` (mount above `<FeedList`, ~line 128)
- Modify: `apps/main/src/lib/i18n/en.json`, `apps/main/src/lib/i18n/fr.json`

**Interfaces:**
- Consumes: `OnboardingStatus` (`apps/main/src/lib/types/index.ts:609` — `current_step`, `completed` already exist), layout data fields `data.tourDismissed`/`data.tourSteps`.
- Produces (layout data, readable via `page.data` in any `(app)` page): `onboardingCompleted: boolean`, `onboardingStep: number`.
- New component `OnboardingSetupCard` — no props; reads `page.data` and `sessionStorage['jc:onboarding_banner_snoozed']`.

- [ ] **Step 1: Extend the layout loader — `apps/main/src/routes/+layout.server.ts`**

Change the onboarding `apiGet` result mapping to include completion fields (keep the existing `dismissed`/`steps` fields untouched):

```ts
		user
			? apiGet<OnboardingStatus>(ctx, '/onboarding/status')
					.then((v) => ({
						ok: true as const,
						value: {
							dismissed: !!v?.tour_dismissed,
							steps: v?.tour_steps?.['main'] ?? null,
							completed: !!v?.completed,
							currentStep: v?.current_step ?? 1
						}
					}))
					.catch(() => ({
						ok: false as const,
						value: { dismissed: false, steps: null as string[] | null, completed: true, currentStep: 1 }
					}))
			: Promise.resolve({
					ok: true as const,
					value: { dismissed: false, steps: null as string[] | null, completed: true, currentStep: 1 }
				})
```

(`completed: true` on failure/no-user is the conservative choice — the card hides rather than nagging on a fetch error.)

Then, where `tourDismissed`/`tourSteps` are extracted (~line 206) and returned (~line 233), add:

```ts
	const onboardingCompleted = tourResult.value.completed;
	const onboardingStep = tourResult.value.currentStep;
```

and add both to the returned object.

- [ ] **Step 2: New component — `apps/main/src/lib/components/onboarding/OnboardingSetupCard.svelte`**

```svelte
<script lang="ts">
	// §09: incomplete-onboarding reminder card. Shown at the top of the home
	// feed until the wizard is finished; the X button snoozes it for the
	// browser session (it reappears on the next visit while incomplete).
	import { page } from '$app/state';
	import { browser } from '$app/environment';
	import { X, ArrowRight } from 'lucide-svelte';
	import { t } from '$lib/i18n';

	const SNOOZE_KEY = 'jc:onboarding_banner_snoozed';
	const TOTAL_STEPS = 5; // mirrors internal/onboarding/service.go totalSteps

	let snoozed = $state(browser && sessionStorage.getItem(SNOOZE_KEY) === '1');

	const visible = $derived(
		!!page.data.user && page.data.onboardingCompleted === false && !snoozed
	);
	const step = $derived((page.data.onboardingStep as number) ?? 1);

	function snooze() {
		sessionStorage.setItem(SNOOZE_KEY, '1');
		snoozed = true;
	}
</script>

{#if visible}
	<div
		class="mb-3 flex items-center gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3"
		role="status"
	>
		<div class="min-w-0 flex-1">
			<p class="text-sm font-medium text-[var(--color-text)]">
				{t('onboarding.reminder_title', 'Finish setting up your account')}
			</p>
			<p class="mt-0.5 text-xs text-[var(--color-muted)]">
				{t('onboarding.reminder_progress', 'Step {step} of {total}', { step, total: TOTAL_STEPS })}
			</p>
			<div class="mt-2 h-1 overflow-hidden rounded-full bg-[var(--color-surface-subtle)]">
				<div
					class="h-full rounded-full bg-[var(--color-primary)] transition-all"
					style="width: {Math.min(100, Math.max(0, ((step - 1) / TOTAL_STEPS) * 100))}%"
				></div>
			</div>
		</div>
		<a
			href="/onboarding"
			class="flex shrink-0 items-center gap-1 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] hover:opacity-90"
		>
			{t('onboarding.reminder_cta', 'Continue setup')}
			<ArrowRight size={14} />
		</a>
		<button
			type="button"
			onclick={snooze}
			class="shrink-0 rounded-full p-1 text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
			aria-label={t('onboarding.reminder_dismiss', 'Dismiss reminder')}
		>
			<X size={14} />
		</button>
	</div>
{/if}
```

Verify the exact icon export names in `lucide-svelte` usage elsewhere (e.g. `X`, `ArrowRight` are used in `LeftNav.svelte`/`AppShell.svelte` — grep before writing).

- [ ] **Step 3: Mount in the feed — `apps/main/src/routes/(app)/+page.svelte`**

In the authed `{:else}` branch, inside `<div class="p-3">` immediately above `<FeedList` (~line 128):

```svelte
		<div class="p-3">
			<!-- §09: finish-setup reminder — self-gating on page.data.onboardingCompleted -->
			<OnboardingSetupCard />
			<FeedList
```

Add the import at the top with the other component imports:

```ts
	import OnboardingSetupCard from '$lib/components/onboarding/OnboardingSetupCard.svelte';
```

- [ ] **Step 4: i18n — `apps/main/src/lib/i18n/en.json`**

In the existing `onboarding.*` key block (find `"onboarding.` keys and append alphabetically inside that group):

```json
	"onboarding.reminder_title": "Finish setting up your account",
	"onboarding.reminder_progress": "Step {step} of {total}",
	"onboarding.reminder_cta": "Continue setup",
	"onboarding.reminder_dismiss": "Dismiss reminder",
```

**`fr.json`** — same positions:

```json
	"onboarding.reminder_title": "Terminez la configuration de votre compte",
	"onboarding.reminder_progress": "Étape {step} sur {total}",
	"onboarding.reminder_cta": "Continuer",
	"onboarding.reminder_dismiss": "Ignorer le rappel",
```

Check the file's key format first — if keys are nested objects rather than flat `"onboarding.reminder_*"`, match the existing structure. (`t()` params replace `{step}`/`{total}` — confirmed in `store.svelte.ts`.)

- [ ] **Step 5: Verify**

```bash
cd apps/main && bun run check
```

Expected: `svelte-check found 0 errors and 0 warnings`.

- [ ] **Step 6: Commit**

```bash
git add apps/main/src/routes/+layout.server.ts apps/main/src/lib/components/onboarding/OnboardingSetupCard.svelte 'apps/main/src/routes/(app)/+page.svelte' apps/main/src/lib/i18n/en.json apps/main/src/lib/i18n/fr.json
git commit -m "feat(main): finish-setup feed card for incomplete onboarding (§09)"
```

---

### Task 4: End-to-end sanity + full verification

**Files:** none new — verification pass only.

- [ ] **Step 1: Backend green**

```bash
gofmt -l internal/ migrations/ 2>/dev/null | head   # should print nothing
go build ./...
go test ./internal/onboarding ./internal/control ./internal/worker
```

- [ ] **Step 2: Frontend green**

```bash
cd apps/main && bun run check
```

- [ ] **Step 3: Migration smoke test** (if a local DB is running)

```bash
# whatever the repo's migrate command is (check Makefile: `make migrate` or similar)
grep -n 'migrate' Makefile | head -5
```

Apply 000759 up, confirm `\d onboarding_state` shows `reminder_count`/`last_reminder_at` and `notification_types` contains `onboarding_reminder`; then run the down file and confirm clean rollback. Skip if no local DB.

- [ ] **Step 4: Behavioral trace (mental/walkthrough, no infra)**

- New user registers → no `onboarding_state` row → sweep next day: `COALESCE(o.completed,false)=false` ✓, age < 14d ✓, count NULL→0 < 5 ✓ → upsert creates row, `InsertNotification` fires → notification links `/onboarding`.
- User finishes wizard → `completed=true` → excluded everywhere; card hides because `onboardingCompleted === true`.
- User opts out of `onboarding_reminder` in-app in notification preferences → `NOT EXISTS` clause excludes them.
- Sweep runs twice same day → `last_reminder_at < now() - 20h` blocks the second run.
- Card: X click → `sessionStorage` set → hidden until tab/session ends → reappears next session while incomplete.

- [ ] **Step 5: Final status**

Report: files changed, test results, and note the `.jeos` submodule changes are uncommitted in its working tree.
