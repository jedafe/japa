# Section 35 — Notification System: Implementation Report

**Date:** 2026-06-26
**Status:** ✅ COMPLETE
**Tests:** 17/17 PASS
**Build:** Clean (`go build ./...`, `go vet ./...`)

---

## Files Created

| File | Lines | Description |
|------|-------|-------------|
| `migrations/000013_notification_system.sql` | 70 | 4 tables: notification_types, notifications, notification_preferences, notification_batch_windows |
| `internal/db/queries/notifications.sql` | 89 | 14 sqlc queries (notifications CRUD, preferences, batch dedup, types) |
| `internal/db/gen/notifications.sql.go` | 398 | sqlc-generated Go code |
| `internal/notifications/notifications.go` | 54 | Package doc, constants, Service constructor |
| `internal/notifications/store.go` | 149 | 13 store methods wrapping sqlc queries |
| `internal/notifications/service.go` | 221 | Business logic: DispatchNotification, BuildBatchTitle, CRUD |
| `internal/notifications/handlers.go` | 231 | 7 HTTP handlers |
| `internal/notifications/tasks.go` | 11 | Asynq task definitions (placeholder) |
| `internal/notifications/notifications_test.go` | 332 | 17 unit tests |

**Total:** 1,555 lines across 9 files

---

## Migration: 000013_notification_system.sql

### Tables

1. **notification_types** — Admin-configurable registry of notification categories
   - `key` (text PK), `label`, `description`, `enabled`, `default_in_app`, `default_email`, `default_push`
   - 8 seeded types: new_follower, post_reply, comment, mention, reaction, dm_message, dm_request, system_announcement

2. **notifications** — Individual notification rows
   - `id` (uuid PK), `tenant_id`, `recipient_id`, `notification_type`, `actor_type`, `actor_id`, `subject_type`, `subject_id`, `title`, `body`, `action_url`, `read_at`, `created_at`
   - Indexes: recipient (tenant_id, recipient_id, read_at, created_at DESC), type (tenant_id, notification_type)

3. **notification_preferences** — Per-user notification delivery preferences
   - `user_id` + `notification_type` (composite PK), `in_app`, `email`, `push`

4. **notification_batch_windows** — Batch dedup windows for grouping rapid-fire events
   - `id` (uuid PK), `recipient_id`, `notification_type`, `subject_type`, `subject_id`, `actor_ids` (uuid[]), `actor_count`, `last_actor_id`, `window_open_at`, `window_close_at`, `dispatched`
   - UNIQUE constraint on (recipient_id, notification_type, subject_type, subject_id)

### SQLC Queries (14 total)

| Query | Type | Description |
|-------|------|-------------|
| InsertNotification | :exec | Create notification row |
| GetNotificationsByRecipient | :many | Paginated notifications with type label |
| GetUnreadNotificationCount | :one | Count unread notifications |
| MarkNotificationRead | :exec | Mark single notification read |
| MarkAllNotificationsRead | :exec | Bulk mark all as read |
| DeleteNotification | :exec | Delete single notification |
| GetNotificationPreferences | :many | User preferences with type label |
| UpsertNotificationPreference | :exec | Create/update preference |
| GetOpenBatchWindow | :one | Find open batch window |
| CreateBatchWindow | :exec | Create new batch window (5-min TTL) |
| ExtendBatchWindow | :exec | Add actor to existing window |
| MarkBatchWindowDispatched | :exec | Mark window as dispatched |
| GetNotificationTypes | :many | List all notification types |

---

## Service Layer

### DispatchNotification (Batch Dedup)

The core batch dedup logic:

1. Check user preferences — skip if in-app is disabled
2. Look for open batch window matching (recipient, type, subject)
3. If window exists and not dispatched → extend it (add actor, increment count)
4. If no window → create new window with 5-minute TTL
5. Returns `DispatchResult` with action ("created", "extended", "skipped")

### BuildBatchTitle

Aggregates actor info for batch notifications:
- Single actor: "Alice reacted"
- Multiple actors: "Alice and 9 others reacted"

### NotificationInfo / PreferenceInfo

Wrapper types that embed sqlc row types for clean JSON serialization.

---

## HTTP Handlers (7 routes)

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | /api/v1/notifications | ListNotifications | Paginated notifications |
| GET | /api/v1/notifications/unread-count | GetUnreadCount | Unread notification count |
| PATCH | /api/v1/notifications/{id}/read | MarkAsRead | Mark single as read |
| POST | /api/v1/notifications/read-all | MarkAllAsRead | Mark all as read |
| GET | /api/v1/notification-preferences | GetPreferences | User preferences |
| PUT | /api/v1/notification-preferences/{type} | UpdatePreference | Update single preference |
| GET | /api/v1/notification-types | ListNotificationTypes | All notification types |

All routes protected by `auth.RequireAuth`.

---

## Permission Keys

Added to `bootstrapPermissions` in `server.go`:

- `notifications.view` — View notifications
- `notifications.manage` — Manage notification types

---

## Key Decisions

1. **Batch dedup window = 5 minutes** — configurable via `BatchWindowMinutes` constant
2. **Actor aggregation** — "Alice and 9 others reacted" format using `BuildBatchTitle()`
3. **User preferences checked before dispatch** — if in-app disabled, notification is skipped
4. **Asynq integration deferred** — `tasks.go` contains task name constants and payload struct; actual Asynq wiring deferred to Phase 6
5. **Real-time WebSocket delivery** — referenced but not implemented; deferred to Section 10 WebSocket system
6. **Email/push delivery** — preferences store these flags but actual delivery channels deferred to Phase 6

---

## Deferrals

- Asynq task processor for `notification:dispatch_batch` — Phase 6
- WebSocket real-time notification delivery — Section 10
- Email notification delivery — Phase 6
- Push notification delivery — Phase 6
- Admin UI for managing notification types — future section

---

## Test Summary

17 unit tests covering:
- Notification type constants (8 types validated)
- Actor type constants
- BatchWindowMinutes constant
- PageSize constants (DefaultPageSize, MaxPageSize)
- writeJSON / writeError helpers
- parsePageSize (10 cases)
- parsePageOffset (6 cases)
- BuildBatchTitle (5 cases: single, 2, 3, 10, 100 actors)
- SplitNames (4 cases)
- pluralize (3 cases)
- NewService constructor
- NotificationInfo JSON tags
- DispatchResult JSON tags
- NotificationDispatchPayload JSON tags
- requireTenant (no tenant in context)
- requireUser (no user in context)
