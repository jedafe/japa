# ADR-035: Notification System — Batching and Deduplication

**Status:** Accepted

**Context:** Without batching, 10 users reacting to the same post within 5 minutes would produce 10 separate push notifications to the post author, which is poor UX. A batching system was needed that groups rapid-fire events of the same type on the same subject into a single summarised notification.

**Decision:** `notification_batch_windows` table: one row per `(recipient_id, notification_type, subject_type, subject_id)` with a 5-minute window. When a new notification event fires, the system checks for an open window. If one exists, it extends the window and adds the actor to `actor_ids[]`. If none exists, a new window is created and a `notification:dispatch_batch` Asynq task is enqueued to fire at `window_close_at`. The dispatched notification reads actor_count and formats accordingly: "Alice reacted to your post" (1 actor), "Alice and 9 others reacted to your post" (10 actors). Delivery channels (in-app WS event, email, push) are driven by per-user `notification_preferences`, not hardcoded per notification type. `notification_types` is an admin-configurable table — new types can be added without a code deploy.

**Rationale:** 5-minute window is short enough to feel responsive but long enough to batch typical reaction bursts. Actor array is bounded (practical maximum ~50 actors before the window closes). Admin-configurable `notification_types` means new notification types (e.g. from new V2 features) don't require a code change to become manageable from the Admin Panel.

**Consequences:** There is a 5-minute delay between a first notification event and delivery when a window is opened. This is acceptable — real-time feel is preserved by the WS `notification` event for the first actor; the batch catches up.
