package media

// Asynq task type constants for background media processing.
// ClamAV scan and video/image processing are deferred to Phase 6.
const (
	TaskMediaScan       = "media_scan"
	TaskImageProcessing = "image_processing"
	TaskVideoProcessing = "video_processing"
)

// MediaScanPayload is the payload for the media_scan task (ClamAV scan).
type MediaScanPayload struct {
	MediaID string `json:"media_id"`
}

// ImageProcessingPayload is the payload for the image_processing task.
type ImageProcessingPayload struct {
	MediaID string `json:"media_id"`
}

// VideoProcessingPayload is the payload for the video_processing task.
type VideoProcessingPayload struct {
	MediaID   string `json:"media_id"`
	Transcode bool   `json:"transcode"`
}
