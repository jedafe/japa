package notifications

// Task names for Asynq queue.
const (
	TaskDispatchBatch = "notification:dispatch_batch"
	TaskPushDelivery  = "push:delivery"
)

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
}
