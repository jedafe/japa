// Package worker — mediavideo.go defines the MediaVideoProcessor interface that the
// content:process_video task delegates to. The media package implements this
// interface and registers itself at startup via RegisterMediaVideoProcessor,
// mirroring the MediaScanProcessor/MediaCleaner pattern.
package worker

import "context"

// MediaVideoProcessor defines the contract for processing a single stored
// video. The worker calls ProcessMediaVideo from the process_video task
// handler; the media package owns the DB fetch, object fetch (S3 or local
// fallback), ffmpeg invocation on temp files, storage rewrite, and row
// update — the storage_path column holds a storage key, not a filesystem
// path, so the handler cannot shell out to ffmpeg directly.
type MediaVideoProcessor interface {
	// ProcessMediaVideo handles one media row. operation is "transcode"
	// or "compress"; unknown operations are skipped without error.
	ProcessMediaVideo(ctx context.Context, mediaID, operation string) error
}

var mediaVideoProcessor MediaVideoProcessor

// RegisterMediaVideoProcessor registers the media video processor for the
// content:process_video background task. Call once at startup before the
// worker begins processing tasks. Pass nil to disable video processing
// (graceful degradation).
func RegisterMediaVideoProcessor(p MediaVideoProcessor) {
	mediaVideoProcessor = p
}
