package media

// Asynq task type constants for background media processing.
// ClamAV scan and video/image processing are deferred to Phase 6.
const (
	TaskMediaScan       = "content:scan_media"
	TaskImageProcessing = "content:process_image"
	TaskVideoProcessing = "content:process_video"
)

// MediaScanPayload is the payload for the content:scan_media task (ClamAV scan).
type MediaScanPayload struct { //nolint:revive // stuttering type name preserved for API compatibility
	MediaID string `json:"media_id"`
}

// ImageProcessingPayload is the payload for the content:process_image task.
type ImageProcessingPayload struct {
	MediaID string `json:"media_id"`
}

// VideoProcessingPayload is the payload for the content:process_video task.
type VideoProcessingPayload struct {
	MediaID   string `json:"media_id"`
	Transcode bool   `json:"transcode"`
}
