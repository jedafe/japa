# Phase 3 Notification Patterns

**Date:** 2026-06-26
**Phase:** 3
**Purpose:** Implementation patterns learned during Section 35 (Notification System), reusable in future sections.

## Pattern: Batch Deduplication Window (5-Minute Aggregation)

When multiple users react to the same post within 5 minutes, the recipient gets ONE notification — not 10. The `notification_batch_windows` table tracks an open window per `(recipient, notification_type, subject_type, subject_id)`. The window lifecycle is: create → extend (add actors) → dispatch (create final notification). The window closes after 5 minutes, dispatched by an Asynq scheduled task.

The `DispatchNotification` service method implements a check-then-create/extend logic:

```go
func (s *Service) DispatchNotification(ctx context.Context, tenantID, recipientID pgtype.UUID, nType, actorType string, actorID pgtype.UUID, subjectType string, subjectID pgtype.UUID, title, body, actionURL string) (DispatchResult, error) {
    // Check user preferences — if in-app is disabled, skip entirely.
    pref, err := s.store.getNotificationPreferences(ctx, recipientID)
    if err == nil {
        for _, p := range pref {
            if p.NotificationType == nType && !p.InApp {
                return DispatchResult{Action: "skipped"}, nil
            }
        }
    }

    // Try to find an existing open batch window.
    window, err := s.store.getOpenBatchWindow(ctx, gen.GetOpenBatchWindowParams{
        RecipientID:      recipientID,
        NotificationType: nType,
        SubjectType:      subjectType,
        SubjectID:        subjectID,
    })
    if err == nil && !window.Dispatched {
        // Extend existing window.
        if err := s.store.extendBatchWindow(ctx, gen.ExtendBatchWindowParams{
            ActorID: actorID,
            ID:      window.ID,
        }); err != nil {
            return DispatchResult{}, fmt.Errorf("extending batch window: %w", err)
        }
        return DispatchResult{Action: "extended", WindowID: window.ID}, nil
    }

    // Create new window.
    if err := s.store.createBatchWindow(ctx, gen.CreateBatchWindowParams{
        RecipientID:      recipientID,
        NotificationType: nType,
        SubjectType:      subjectType,
        SubjectID:        subjectID,
        ActorID:          actorID,
    }); err != nil {
        return DispatchResult{}, fmt.Errorf("creating batch window: %w", err)
    }

    // Retrieve the newly created window to get its ID.
    window, err = s.store.getOpenBatchWindow(ctx, gen.GetOpenBatchWindowParams{
        RecipientID:      recipientID,
        NotificationType: nType,
        SubjectType:      subjectType,
        SubjectID:        subjectID,
    })
    if err != nil {
        return DispatchResult{}, fmt.Errorf("retrieving new batch window: %w", err)
    }

    return DispatchResult{Action: "created", WindowID: window.ID}, nil
}
```

The batch window SQL uses `ARRAY[@actor_id::uuid]` to initialize the actor array and `array_append(actor_ids, @actor_id)` to extend it:

```sql
-- name: CreateBatchWindow :exec
INSERT INTO notification_batch_windows (recipient_id, notification_type, subject_type, subject_id, actor_ids, actor_count, last_actor_id, window_close_at)
VALUES (@recipient_id, @notification_type, @subject_type, @subject_id, ARRAY[@actor_id::uuid], 1, @actor_id, now() + interval '5 minutes');

-- name: ExtendBatchWindow :exec
UPDATE notification_batch_windows
SET actor_ids = array_append(actor_ids, @actor_id),
    actor_count = actor_count + 1,
    last_actor_id = @actor_id
WHERE id = @id;
```

The `UNIQUE(recipient_id, notification_type, subject_type, subject_id)` constraint prevents race conditions where two goroutines try to create a window simultaneously — the second INSERT fails and the service retries the extend path.

## Pattern: Polymorphic Actor + Subject (Two-Level Reference)

Every notification has both an **actor** (who triggered it) and a **subject** (what it's about). Both use a `text` discriminator + UUID pair, but they represent different relationship levels:

- `actor_type`/`actor_id`: `'user'` or `'system'` — who performed the action
- `subject_type`/`subject_id`: `'content'`, `'space'`, `'comment'`, `'dm_conversation'`, `'review'` — what entity the action targets

This two-level reference allows notification titles like "Alice commented on your post in Space X" — the actor is Alice (user), the subject is the post (content), and the space can be resolved from the subject's parent context.

```sql
CREATE TABLE notifications (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    recipient_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    notification_type  text NOT NULL REFERENCES notification_types(key),
    actor_type         text,        -- 'user' or 'system'
    actor_id           uuid,
    subject_type       text,        -- 'content', 'space', 'comment', 'dm_conversation', 'review'
    subject_id         uuid,
    title              text NOT NULL,
    body               text,
    action_url         text,
    read_at            timestamptz,
    created_at         timestamptz NOT NULL DEFAULT now()
);
```

The `actor_id` and `subject_id` columns are nullable because system notifications (e.g., `system_announcement`) have no actor, and some notification types (e.g., a broadcast) may have no specific subject. The service layer handles nullable fields via pointer conversion:

```go
func (s *Service) CreateNotification(ctx context.Context, tenantID, recipientID pgtype.UUID, nType, actorType string, actorID pgtype.UUID, subjectType string, subjectID pgtype.UUID, title, body, actionURL string) error {
    var actorTypePtr *string
    if actorType != "" {
        actorTypePtr = &actorType
    }
    var subjectTypePtr *string
    if subjectType != "" {
        subjectTypePtr = &subjectType
    }
    var bodyPtr *string
    if body != "" {
        bodyPtr = &body
    }
    var actionURLPtr *string
    if actionURL != "" {
        actionURLPtr = &actionURL
    }

    return s.store.insertNotification(ctx, gen.InsertNotificationParams{
        TenantID:         tenantID,
        RecipientID:      recipientID,
        NotificationType: nType,
        ActorType:        actorTypePtr,
        ActorID:          actorID,
        SubjectType:      subjectTypePtr,
        SubjectID:        subjectID,
        Title:            title,
        Body:             bodyPtr,
        ActionUrl:        actionURLPtr,
    })
}
```

The nullable pointer pattern (`*string`) is the sqlc convention for optional text columns — use `nil` for absent values, not empty strings.

## Pattern: Admin-Configurable Notification Type Registry

The `notification_types` table is a seedable registry, not hardcoded constants. Each type has default delivery preferences (`default_in_app`, `default_email`, `default_push`) and an `enabled` flag. Users can override defaults per type in `notification_preferences`. Admins can add/remove types without a code deploy:

```sql
CREATE TABLE notification_types (
    key              text PRIMARY KEY,
    label            text NOT NULL,
    description      text,
    enabled          boolean NOT NULL DEFAULT true,
    default_in_app   boolean NOT NULL DEFAULT true,
    default_email    boolean NOT NULL DEFAULT false,
    default_push     boolean NOT NULL DEFAULT false
);

-- Seed default notification types
INSERT INTO notification_types (key, label, description, default_in_app, default_email, default_push) VALUES
    ('new_follower', 'New Follower', 'Someone followed you', true, false, false),
    ('post_reply', 'Post Reply', 'Someone replied to your post', true, true, false),
    ('comment', 'Comment', 'Someone commented on your post', true, true, false),
    ('mention', 'Mention', 'Someone mentioned you', true, true, false),
    ('reaction', 'Reaction', 'Someone reacted to your post', true, false, false),
    ('dm_message', 'Direct Message', 'You received a new message', true, true, true),
    ('dm_request', 'Message Request', 'Someone you don''t mutually follow wants to message you', true, false, false),
    ('system_announcement', 'System Announcement', 'Platform-wide announcement', true, true, false)
ON CONFLICT (key) DO NOTHING;
```

The Go constants mirror the seeded keys for type-safe references in application code:

```go
const (
    TypeNewFollower        = "new_follower"
    TypePostReply          = "post_reply"
    TypeComment            = "comment"
    TypeMention            = "mention"
    TypeReaction           = "reaction"
    TypeDMMessage          = "dm_message"
    TypeDMRequest          = "dm_request"
    TypeSystemAnnouncement = "system_announcement"
)
```

The `ON CONFLICT (key) DO NOTHING` clause makes the seed idempotent — re-running the migration is safe. The `enabled` flag allows admins to disable a notification type without removing it, and the `label`/`description` fields provide frontend display text without hardcoded strings.

## Pattern: Aggregated Actor Title Building

Instead of "Alice reacted", "Bob reacted", "Charlie reacted" (10 separate notifications), the batch system produces "Alice and 9 others reacted". The `BuildBatchTitle` function handles three cases: single actor, two actors, and many actors:

```go
func BuildBatchTitle(singleActorName string, actorCount int32, action string) string {
    if actorCount <= 1 {
        return fmt.Sprintf("%s %s", singleActorName, action)
    }
    others := actorCount - 1
    return fmt.Sprintf("%s and %d other%s %s", singleActorName, others, pluralize(others), action)
}

func pluralize(n int32) string {
    if n == 1 {
        return ""
    }
    return "s"
}
```

Test cases confirm the expected output:

```go
func TestBuildBatchTitle(t *testing.T) {
    tests := []struct {
        name      string
        actorName string
        count     int32
        action    string
        want      string
    }{
        {"single actor", "Alice", 1, "reacted", "Alice reacted"},
        {"two actors", "Alice", 2, "reacted", "Alice and 1 other reacted"},
        {"three actors", "Alice", 3, "reacted", "Alice and 2 others reacted"},
        {"ten actors", "Alice", 10, "reacted", "Alice and 9 others reacted"},
        {"hundred actors", "Bob", 100, "followed you", "Bob and 99 others followed you"},
    }
    // ...
}
```

The `SplitNames` helper extracts the first actor name from a comma-separated list stored in the batch window's actor metadata:

```go
func SplitNames(names string) string {
    parts := strings.Split(names, ",")
    if len(parts) == 0 {
        return ""
    }
    return strings.TrimSpace(parts[0])
}
```

This pattern is extensible — when the batch window is dispatched, the Asynq task processor calls `BuildBatchTitle` with the aggregated actor names from the window's `actor_ids` array, resolved against the users table.

## Pattern: Read/Write Pool Separation in Store Layer

The Store struct holds two `gen.Queries` instances — one for the reader connection pool, one for the writer pool. Write operations (INSERT, UPDATE, DELETE) use `s.write`, read operations (SELECT) use `s.read`. Both pools connect to the same sqlc-generated queries but route to different PostgreSQL connection pools:

```go
type Store struct {
    write *gen.Queries
    read  *gen.Queries
}

func NewStore(read, write *gen.Queries) *Store {
    return &Store{read: read, write: write}
}
```

The Service layer constructs the Store from the database pool:

```go
func NewService(database *db.DB) *Service {
    return &Service{
        store: &Store{
            read:  gen.New(database.Reader),
            write: gen.New(database.Writer),
        },
        db: database,
    }
}
```

This separation is the standard pattern for all sections in JapaConnect. The reader pool can be pointed at a read replica in production without changing any application code. The `db.Writer` pool is also used directly for raw SQL when the table name is dynamic (see Section 31's `updateFollowerCount` pattern).

## Pattern: Notification Preferences with Type Joins

The `GetNotificationPreferences` query joins `notification_preferences` with `notification_types` to return the label and description alongside the user's preference settings. This allows the frontend to display a full preference UI without a separate query for type metadata:

```sql
-- name: GetNotificationPreferences :many
SELECT np.user_id, np.notification_type, np.in_app, np.email, np.push,
       nt.label AS type_label
FROM notification_preferences np
JOIN notification_types nt ON nt.key = np.notification_type
WHERE np.user_id = @user_id;
```

The service layer wraps the result in a `PreferenceInfo` struct for JSON serialization:

```go
type PreferenceInfo struct {
    gen.GetNotificationPreferencesRow
}

func (s *Service) GetPreferences(ctx context.Context, userID pgtype.UUID) ([]PreferenceInfo, error) {
    rows, err := s.store.getNotificationPreferences(ctx, userID)
    if err != nil {
        return nil, fmt.Errorf("getting preferences: %w", err)
    }
    result := make([]PreferenceInfo, len(rows))
    for i, r := range rows {
        result[i] = PreferenceInfo{GetNotificationPreferencesRow: r}
    }
    return result, nil
}
```

The upsert query uses `ON CONFLICT (user_id, notification_type) DO UPDATE` so the preference endpoint is idempotent — calling PUT with the same values is a no-op:

```sql
-- name: UpsertNotificationPreference :exec
INSERT INTO notification_preferences (user_id, notification_type, in_app, email, push)
VALUES (@user_id, @notification_type, @in_app, @email, @push)
ON CONFLICT (user_id, notification_type) DO UPDATE
SET in_app = EXCLUDED.in_app, email = EXCLUDED.email, push = EXCLUDED.push;
```

## Pattern: Mark All As Read (Bulk Update)

A single `UPDATE` query sets `read_at = now()` for all unread notifications — more efficient than iterating individual notifications. The query uses `tenant_id + recipient_id + read_at IS NULL` as the filter:

```sql
-- name: MarkAllNotificationsRead :exec
UPDATE notifications SET read_at = now()
WHERE tenant_id = @tenant_id AND recipient_id = @recipient_id AND read_at IS NULL;
```

The single-mark query is equally important — it scopes by notification ID and recipient ID to prevent one user from marking another user's notifications as read:

```sql
-- name: MarkNotificationRead :exec
UPDATE notifications SET read_at = now() WHERE id = @id AND recipient_id = @recipient_id;
```

Both queries use the indexed columns (`idx_notifications_recipient` covers `tenant_id, recipient_id, read_at, created_at DESC`) so the UPDATE is an index scan, not a sequential scan. The `MarkAllNotificationsRead` handler returns a simple success boolean — the client can optimistically update its local state without re-fetching the full notification list:

```go
func (h *Handlers) MarkAllAsRead(w http.ResponseWriter, r *http.Request) {
    user, ok := h.requireUser(w, r)
    if !ok {
        return
    }
    tenantID, ok := h.requireTenant(w, r)
    if !ok {
        return
    }

    if err := h.service.MarkAllAsRead(r.Context(), tenantID, user.UserID); err != nil {
        writeError(w, http.StatusInternalServerError, "internal_error", "Failed to mark all notifications as read")
        return
    }
    writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"success": true}})
}
```

## Lesson: Preference Check Before Batch Window Creation

The `DispatchNotification` method checks the recipient's notification preferences **before** creating or extending a batch window. If the recipient has disabled in-app delivery for the notification type, the method returns `Action: "skipped"` without touching the batch window table. This prevents wasted batch window rows for users who opted out:

```go
pref, err := s.store.getNotificationPreferences(ctx, recipientID)
if err == nil {
    for _, p := range pref {
        if p.NotificationType == nType && !p.InApp {
            return DispatchResult{Action: "skipped"}, nil
        }
    }
}
```

The preference check is best-effort — if the preferences query fails (e.g., table doesn't exist yet), the dispatch proceeds. This is intentional: a missing preferences table should not block notification delivery. The `ON CONFLICT (key) DO NOTHING` seed pattern ensures the `notification_types` table is always populated, but `notification_preferences` is user-created and may be sparse.

## Lesson: Asynq Task Placeholder for Batch Dispatch

The batch window dispatch (converting a window into a final notification) is wired to an Asynq task but the processor is a placeholder:

```go
// Task names for Asynq queue.
const (
    TaskDispatchBatch = "notification:dispatch_batch"
)

// NotificationDispatchPayload is the payload for the dispatch_batch task.
type NotificationDispatchPayload struct {
    WindowID string `json:"window_id"`
}
```

The `DispatchBatch` service method currently only marks the window as dispatched:

```go
func (s *Service) DispatchBatch(ctx context.Context, windowID pgtype.UUID, tenantID pgtype.UUID) error {
    return s.store.markBatchWindowDispatched(ctx, windowID)
}
```

The full implementation — resolving actor IDs from the `actor_ids` array, calling `BuildBatchTitle`, creating the final notification row, and cleaning up expired windows — is deferred to Phase 6 when Asynq workers are wired. The placeholder ensures the schema, queries, and service layer are testable now without requiring the full Asynq infrastructure. The `TaskDispatchBatch` constant and `NotificationDispatchPayload` struct define the contract that the Phase 6 worker will implement.
