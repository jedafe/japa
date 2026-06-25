# Section 35 — Notification System

**Status:** ✅ Locked

## Schema

```sql
CREATE TABLE notification_types (
  key          text PRIMARY KEY,
  -- 'new_follower'|'post_reply'|'comment'|'mention'|'reaction'
  -- |'dm_message'|'dm_request'|'course_enrolled'|'review_approved'
  -- |'referral_qualified'|'moderation_action'|'system_announcement'
  label        text NOT NULL,
  description  text,
  enabled      boolean NOT NULL DEFAULT true,
  -- Default delivery preferences for this type (user can override)
  default_in_app  boolean NOT NULL DEFAULT true,
  default_email   boolean NOT NULL DEFAULT false,
  default_push    boolean NOT NULL DEFAULT false
);

CREATE TABLE notifications (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id       uuid NOT NULL REFERENCES tenants(id),
  recipient_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  notification_type text NOT NULL REFERENCES notification_types(key),
  -- Polymorphic actor (who triggered it)
  actor_type      text,        -- 'user'|'system'
  actor_id        uuid,
  -- Polymorphic subject (what it's about)
  subject_type    text,        -- 'content'|'space'|'comment'|'dm_conversation'|'review'
  subject_id      uuid,
  title           text NOT NULL,
  body            text,
  action_url      text,
  read_at         timestamptz,
  created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_notifications_recipient ON notifications (tenant_id, recipient_id, read_at, created_at DESC);

-- Per-user per-type delivery preferences
CREATE TABLE notification_preferences (
  user_id           uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  notification_type text NOT NULL REFERENCES notification_types(key),
  in_app            boolean NOT NULL DEFAULT true,
  email             boolean NOT NULL DEFAULT false,
  push              boolean NOT NULL DEFAULT false,
  PRIMARY KEY (user_id, notification_type)
);

-- Deduplication + batching
CREATE TABLE notification_batch_windows (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  recipient_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  notification_type text NOT NULL,
  subject_type    text NOT NULL,
  subject_id      uuid NOT NULL,
  actor_ids       uuid[] NOT NULL DEFAULT '{}',
  actor_count     int NOT NULL DEFAULT 1,
  last_actor_id   uuid,
  window_open_at  timestamptz NOT NULL DEFAULT now(),
  window_close_at timestamptz NOT NULL,  -- e.g. now() + interval '5 minutes'
  dispatched      boolean NOT NULL DEFAULT false,
  UNIQUE(recipient_id, notification_type, subject_type, subject_id)
);
```

## Deduplication Logic

When 10 users react to the same post within 5 minutes, the recipient gets one notification "Alice and 9 others reacted to your post" — not 10 separate ones:

```go
func DispatchNotification(ctx context.Context, n NotificationInput) error {
  // Check for an open batch window for this (recipient, type, subject)
  window, err := queries.GetOpenBatchWindow(ctx, n.RecipientID, n.Type, n.SubjectType, n.SubjectID)
  if err == nil && !window.Dispatched {
    // Extend window and add actor
    return queries.ExtendBatchWindow(ctx, window.ID, n.ActorID)
  }
  // No window → open a new one, schedule dispatch task at window_close_at
  windowID := queries.CreateBatchWindow(ctx, n)
  return enqueueAt(ctx, "notification:dispatch_batch", windowID, time.Now().Add(5*time.Minute))
}
```

## Real-Time Delivery

The `notification:dispatch_batch` task (critical tier) writes the final `notifications` row and sends via WebSocket `notification` event (Section 10). Email and push are dispatched in the same task if the user's preferences indicate them.

## Notification Centre UI

- Dropdown (desktop) / full-screen drawer (mobile) accessible from header bell icon
- Real-time unread badge via WS `notification` event
- Mark all as read / mark individual as read
- Filter by type (All / Mentions / Reactions / Follows / DMs / System)
- Infinite scroll within the dropdown (not paginated — notifications are conversational)
- Preference settings page at `/settings/notifications` with per-type toggles for in-app / email / push

## See Also

Section 10 (WS `notification` event type), Section 11 (notification_dispatch task → dispatch_batch). ADR-035.
