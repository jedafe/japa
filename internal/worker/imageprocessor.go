// Package worker — imageprocessor.go defines the ImageProcessor interface
// that the image_processing task delegates to. The media package (or a
// noop implementation) implements this interface and registers itself at
// startup via RegisterImageProcessor.
package worker

import "context"

// ImageProcessor defines the contract for processing uploaded images —
// resizing, optimizing, and generating thumbnails via imgproxy or
// similar tooling.
type ImageProcessor interface {
	ProcessImage(ctx context.Context, mediaID, operation string, width, height int) error
}

var imageProcessor ImageProcessor

// RegisterImageProcessor registers the image processor for the
// image_processing background task. Call once at startup before the worker
// begins processing tasks. Pass nil to disable image processing (graceful
// degradation).
func RegisterImageProcessor(p ImageProcessor) {
	imageProcessor = p
}
