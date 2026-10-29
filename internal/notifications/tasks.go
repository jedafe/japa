package notifications

// NotificationDispatchPayload is the payload for the dispatch_batch task.
type NotificationDispatchPayload struct {
	WindowID string `json:"window_id"`
}

// PushDeliveryPayload is the payload for the push:delivery task.
type PushDeliveryPayload struct {
	UserID    string `json:"user_id"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	ActionURL string `json:"action_url,omitempty"`
	// NotificationID distinguishes logically distinct pushes that share
	// identical text — asynq.Unique dedupes on payload bytes (BUG-012).
	NotificationID string `json:"notification_id,omitempty"`
}
