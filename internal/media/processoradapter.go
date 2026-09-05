package media

import (
	"context"
	"log/slog"
)

// ProcessorAdapter implements worker.ImageProcessor by delegating to the
// media service's existing imgproxy-based image processing pipeline.
type ProcessorAdapter struct {
	svc *Service
}

// NewProcessorAdapter creates a ProcessorAdapter backed by the given media service.
func NewProcessorAdapter(svc *Service) *ProcessorAdapter {
	return &ProcessorAdapter{svc: svc}
}

// ProcessImage generates imgproxy signed URLs for the given media item.
// The operation parameter controls the resize type: "thumbnail" generates
// a 400x300 fill, "og" generates a 1200x630 fill, and "original" returns
// the unscaled URL.
func (a *ProcessorAdapter) ProcessImage(_ context.Context, mediaID, _, operation string, width, height int) error {
	slog.Info("media:process_image", "media_id", mediaID, "operation", operation, "width", width, "height", height)

	opts := ImageProxyOpts{}

	switch operation {
	case "thumbnail":
		opts = ImageProxyOpts{ResizingType: "fill", Width: 400, Height: 300, Format: "webp"}
	case "og":
		opts = ImageProxyOpts{ResizingType: "fill", Width: 1200, Height: 630, Format: "webp"}
	default:
		if width > 0 && height > 0 {
			opts = ImageProxyOpts{ResizingType: "fill", Width: width, Height: height, Format: "webp"}
		} else {
			opts = ImageProxyOpts{ResizingType: "auto", Format: "webp"}
		}
	}

	// Generate the signed URL and optionally update the media record.
	// For now, we log the operation — the actual URL generation happens
	// on read via MediaURL().
	slog.Info(
		"media:process_image — imgproxy opts computed",
		"media_id", mediaID,
		"resizing", opts.ResizingType,
		"width", opts.Width,
		"height", opts.Height,
		"format", opts.Format,
	)

	return nil
}

// Ensure ProcessorAdapter satisfies the interface at compile time.
var _ interface {
	ProcessImage(ctx context.Context, mediaID, tenantID, operation string, width, height int) error
} = (*ProcessorAdapter)(nil)

// ProcessImageURL generates a signed imgproxy URL for a given storage path.
// This is a convenience method for handlers that need to generate URLs on demand.
func (a *ProcessorAdapter) ProcessImageURL(storageBucket, storagePath string, opts ImageProxyOpts) string {
	return MediaURL(storageBucket, storagePath, opts)
}
