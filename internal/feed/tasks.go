package feed

// Task names for Asynq queue.
const (
	TaskFanOutToFeeds        = "content:fan_out_to_feeds"
	TaskUpdateVelocityScores = "maintenance:update_velocity_scores"
)

// FanOutPayload is the payload for the fan_out_to_feeds task.
type FanOutPayload struct {
	ContentID   string `json:"content_id"`
	SpaceID     string `json:"space_id"`
	ContentType string `json:"content_type"`
}

// UpdateVelocityPayload is the payload for the update_velocity_scores task.
type UpdateVelocityPayload struct {
	TenantID string `json:"tenant_id"`
}
