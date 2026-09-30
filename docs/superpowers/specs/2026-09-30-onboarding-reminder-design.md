# Onboarding Completion Reminder — Design

**Date:** 2026-09-30
**Section:** JEOS §09 (Onboarding Flow), §11 (Job Queue / Asynq)
**Status:** Approved

## Goal

Newly registered members who have not finished the onboarding wizard
(`/onboarding`, 5 steps) get nudged to continue via two channels:

1. An **in-app notification** delivered by a daily Asynq cron sweep — up to 5
   reminders, only within the first 14 days of the account.
2. A **snoozable feed card** at the top of the main app home feed — "Finish
   setting up your account — step X of 5" with a continue CTA and an X button
   that hides the card for the session (reappears on the next visit/tab until
   onboarding completes).

Guests never see either channel. Users with `onboarding_state.completed = true`
are excluded permanently.

## Backend

### Schema — migration `000758_onboarding_reminders`

`onboarding_state` gains two columns (same upsert-friendly pattern as
`tour_dismissals` in 000756):

- `reminder_count int NOT NULL DEFAULT 0`
- `last_reminder_at timestamptz` (NULL until first send)

Users who never started the wizard have no `onboarding_state` row; the
mark-reminded statement is an `INSERT … ON CONFLICT (user_id) DO UPDATE`
upsert that creates the row (all other columns have defaults), identical to
the `DismissTour` precedent.

### Notification type — seeded in the same migration

```sql
INSERT INTO notification_types
VALUES ('onboarding_reminder', 'Onboarding Reminder',
        'Reminder to finish setting up your account',
        true, false, false, false, 'system');
```

(in_app default on; email/push off — user preferences can still override via
`notification_preferences`, per Section 35.)

### Settings — seeded + cataloged

| Key | Default | Meaning |
|---|---|---|
| `onboarding.reminder_enabled` | `true` | Kill switch; sweep no-ops when false. |
| `onboarding.reminder_max_count` | `5` | Max reminders per user. |
| `onboarding.reminder_window_days` | `14` | Only accounts younger than this are reminded. |
| `onboarding.reminder_min_interval_hours` | `20` | Minimum gap between sends; protects against double cron runs. |

All four registered in `internal/control/settings_catalog.go` (panel
`control`, json/int/bool types as appropriate) and documented in
`conventions/system-settings-catalog.md`.

### sqlc — `internal/db/queries/onboarding.sql`

```sql
-- name: ListOnboardingReminderCandidates :many
SELECT u.id
FROM users u
LEFT JOIN onboarding_state o ON o.user_id = u.id
WHERE (o.completed IS NOT TRUE)
  AND u.created_at > now() - make_interval(days => $1::int)
  AND (o.reminder_count IS NULL OR o.reminder_count < $2::int)
  AND (o.last_reminder_at IS NULL
       OR o.last_reminder_at < now() - make_interval(hours => $3::int))
  AND u.deleted_at IS NULL
  AND u.status = 'active'          -- exact predicate per users table
LIMIT $4::int;

-- name: MarkOnboardingReminded :exec
INSERT INTO onboarding_state (user_id, reminder_count, last_reminder_at)
VALUES ($1, 1, now())
ON CONFLICT (user_id) DO UPDATE
SET reminder_count = onboarding_state.reminder_count + 1,
    last_reminder_at = now(),
    updated_at = now();
```

Candidate query parameters are read from the settings above; a hard batch
`LIMIT` (e.g. 2000) bounds each sweep. sqlc output is regenerated via
`make sqlc-gen` — generated files are never hand-edited.

### Asynq task — `onboarding:reminder_sweep`

- Constant `TypeOnboardingReminderSweep = "onboarding:reminder_sweep"` in
  `internal/worker/tasks.go`, registered in `handlerRegistry` and in the
  §11 task catalog first (Rule 7).
- Scheduler entry in `internal/worker/server.go` — `@every 24h` (or cronspec
  equivalent matching existing entries), queue per `QueueForTask`.
- Handler (`internal/worker/handlers.go`, "Periodic sweep handlers" group):
  1. Return early when `onboarding.reminder_enabled` is false.
  2. `ListOnboardingReminderCandidates` with the settings-derived window,
     max count, and min interval.
  3. For each candidate: `MarkOnboardingReminded` then
     `DispatchNotification(ctx, userID, "onboarding_reminder", …)` with
     subject_type `"onboarding"`, subject_id = user ID, zero actor, and
     title/body resolved through the `literalResolver` catalog (§14) so the
     text is localized. Action URL `/onboarding`.

The existing batch-window machinery dedupes correctly for daily sends: a
repeat dispatch on the same (recipient, type, subject) extends the open
window only until it is dispatched; after dispatch the next sweep creates a
fresh window → one notification per day.

Mark-then-dispatch ordering is intentional: if dispatch fails the user is
still counted (prevents spam on repeated failures; max 5 caps total risk).

## Frontend (apps/main only)

### Data — extend `(app)/+layout.server.ts`

The layout already calls `GET /onboarding/status` for the product tour.
Extend its result payload with `completed` and `current_step` so the card
needs no extra request. Guests skip the fetch (unchanged).

### `OnboardingSetupCard.svelte` — `src/lib/components/onboarding/`

- Rendered at the top of the home feed (`(app)/+page.svelte`), inside the
  authed shell only.
- Visible when `data.user && !data.onboarding.completed` and sessionStorage
  key `jc:onboarding_banner_snoozed` is unset.
- Contents: `t('onboarding.reminder_title')`, progress bar + `Step X of 5`
  via `t('onboarding.reminder_progress', …, { step, total })`, CTA button
  linking to `/onboarding`, X button that sets the sessionStorage flag and
  hides the card.
- Styling on platform CSS vars (`--color-surface`, `--color-primary`,
  `--radius-card`), consistent with `ContinueLearningCard`-style cards.
- i18n keys added to `en.json` + `fr.json` (`onboarding.reminder_title`,
  `onboarding.reminder_body`, `onboarding.reminder_cta`,
  `onboarding.reminder_progress`, `onboarding.reminder_dismiss` aria label).

## JEOS updates (before/with code — Rule 1)

- `sections/09-onboarding-flow.md` — document the reminder sweep, cadence,
  settings keys, notification type, and feed card.
- `sections/11-job-queue-asynq.md` — add `onboarding:reminder_sweep` to the
  task catalog (queue, schedule, payload).
- `conventions/system-settings-catalog.md` — four new rows.
- `conventions/api-routes-catalog.md` — no route changes (sweep is internal).

## Testing

- Go: unit tests for the candidate-filter SQL shape are impractical without
  a DB; cover any pure helpers (e.g. payload parsing, setting clamping) in
  `internal/onboarding/`/`internal/worker/` tests.
- Frontend: `svelte-check` clean on apps/main; manual check that the card
  hides on snooze, hides when completed, and links to `/onboarding`.
- Verification: `gofmt`, `go build ./...`, `go test ./internal/onboarding
  ./internal/worker`, `bun run check` in apps/main.

## Out of scope

- Email/push channels for the reminder (in-app only; type defaults set
  accordingly).
- Reminders on Docs/Learn surfaces.
- Admin-facing analytics on reminder conversion.
