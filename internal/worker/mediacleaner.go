// Package worker — mediacleaner.go defines the MediaCleaner interface that
// the media:cleanup_deleted task delegates to. The media package implements
// it via Service.PurgeDeletedMedia and registers at startup via
// RegisterMediaCleaner, mirroring the MediaScanProcessor pattern.
package worker

import "context"

// MediaCleaner defines the contract for purging soft-deleted media rows
// and their storage objects past the retention window
// (media.deleted_retention_days), plus the missing_at reconcile sweep
// (m000924 — flags rows whose storage object is absent, clears rows
// whose object returned).
type MediaCleaner interface {
	PurgeDeletedMedia(ctx context.Context, retentionDays, limit int) (int, error)
	ReconcileMissingObjects(ctx context.Context, maxRows int32) (checked, marked, cleared int, exhausted bool, err error)
}

var mediaCleaner MediaCleaner

// RegisterMediaCleaner registers the media cleaner for the
// media:cleanup_deleted background task. Call once at startup before the
// worker begins processing tasks. Pass nil to disable the sweep
// (graceful degradation).
func RegisterMediaCleaner(p MediaCleaner) {
	mediaCleaner = p
}
