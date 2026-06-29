// Package worker — mediascan.go defines the MediaScanProcessor interface that the
// media_scan task delegates to. The media package implements this interface
// and registers itself at startup via RegisterMediaScanProcessor, mirroring
// the GDPR and search processor pattern.
package worker

import "context"

// MediaScanProcessor defines the contract for scanning a single media item
// for viruses. The worker calls ProcessMediaScan from the media_scan task
// handler; the media package owns the DB fetch, file read, ClamAV scan,
// and status update.
type MediaScanProcessor interface {
	ProcessMediaScan(ctx context.Context, mediaID string) error
}

var mediaScanProcessor MediaScanProcessor

// RegisterMediaScanProcessor registers the media scan processor for the
// media_scan background task. Call once at startup before the worker
// begins processing tasks. Pass nil to disable scanning (graceful
// degradation).
func RegisterMediaScanProcessor(p MediaScanProcessor) {
	mediaScanProcessor = p
}
