package notifications

// Task names for Asynq queue.
const (
	TaskDispatchBatch = "notification:dispatch_batch"
)

// NotificationDispatchPayload is the payload for the dispatch_batch task.
type NotificationDispatchPayload struct {
	WindowID string `json:"window_id"`
}
