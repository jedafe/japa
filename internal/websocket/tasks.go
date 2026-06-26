package websocket

// Task names for Asynq queue.
const (
	TaskSendDMNotification = "dm:send_notification"
)

// DMSendNotificationPayload is the payload for the dm:send_notification task.
type DMSendNotificationPayload struct {
	ConversationID string `json:"conversation_id"`
	MessageID      string `json:"message_id"`
	SenderID       string `json:"sender_id"`
}
