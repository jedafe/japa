package media

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// VideoProcessorAdapter implements worker.MediaVideoProcessor by delegating
// to the ffmpeg VideoProcessor with storage plumbing around it: the media
// row's storage_path is a storage key, not a filesystem path, so the
// adapter fetches the object (S3 or local fallback) into a temp file,
// runs ffmpeg, writes the result back through the normal storage path,
// and updates the media row.
type VideoProcessorAdapter struct {
	svc *Service
	vp  *VideoProcessor
}

// NewVideoProcessorAdapter creates the adapter registered with the worker
// for content:process_video tasks.
func NewVideoProcessorAdapter(svc *Service) *VideoProcessorAdapter {
	return &VideoProcessorAdapter{svc: svc, vp: NewVideoProcessor()}
}

// ProcessMediaVideo runs one ffmpeg operation against a stored video:
// "transcode" (H.264/AAC MP4, fast/CRF 23) or "compress" (slow/CRF 28).
// The output is written back under the same key with a .mp4 extension and
// the media row's storage_path/mime_type/file_size_bytes are updated; the
// original object is deleted afterwards. Unknown operations are skipped —
// the media table has no field to attach a generated thumbnail to, so a
// "thumbnail" op would have nowhere to record its output (video-source
// thumbnails are handled by internal/video, not this task).
func (a *VideoProcessorAdapter) ProcessMediaVideo(ctx context.Context, mediaID, operation string) error {
	if operation != "transcode" && operation != "compress" {
		slog.Info("media:process_video: unsupported operation, skipping",
			"media_id", mediaID, "operation", operation)
		return nil
	}

	var id pgtype.UUID
	if err := id.Scan(mediaID); err != nil {
		return fmt.Errorf("invalid media_id %q: %w", mediaID, err)
	}

	m, err := a.svc.store.GetMediaByID(ctx, id)
	if err != nil {
		// Row deleted between enqueue and run — nothing to process.
		slog.Info("media:process_video: media row not found, skipping", "media_id", mediaID)
		return nil
	}

	// Review-state rows are left untouched, and the row must still be a
	// video (it cannot have been reclassified, but guard anyway).
	if m.QuarantinedAt.Valid || m.RejectedAt.Valid ||
		!strings.HasPrefix(CanonicalMIMEType(m.MimeType), "video/") {
		return nil
	}

	data, err := a.svc.fetchObjectBytes(ctx, m.StorageBucket, m.StoragePath, MaxMediaUploadBody)
	if err != nil {
		if errors.Is(err, errObjectMissing) {
			slog.Warn("media:process_video skipped — stored object missing",
				"media_id", mediaID, "storage_path", m.StoragePath)
			return nil
		}

		return fmt.Errorf("reading stored object %s/%s: %w", m.StorageBucket, m.StoragePath, err)
	}

	// ffmpeg works on filesystem paths — stage the object in temp. The
	// temp dir is inside validateMediaPath's allowlist.
	ext := filepath.Ext(m.StoragePath)

	in, err := os.CreateTemp("", "jc-video-in-*"+ext)
	if err != nil {
		return fmt.Errorf("staging input: %w", err)
	}

	defer func() { _ = os.Remove(in.Name()) }()

	if _, err := in.Write(data); err != nil {
		_ = in.Close()
		return fmt.Errorf("staging input: %w", err)
	}

	if err := in.Close(); err != nil {
		return fmt.Errorf("staging input: %w", err)
	}

	out, err := os.CreateTemp("", "jc-video-out-*.mp4")
	if err != nil {
		return fmt.Errorf("staging output: %w", err)
	}

	outPath := out.Name()

	defer func() { _ = os.Remove(outPath) }()
	// ffmpeg needs to open the path itself — close the placeholder.
	if err := out.Close(); err != nil {
		return fmt.Errorf("staging output: %w", err)
	}

	switch operation {
	case "transcode":
		err = a.vp.Transcode(ctx, in.Name(), outPath)
	case "compress":
		err = a.vp.Compress(ctx, in.Name(), outPath)
	}

	if err != nil {
		return err
	}

	processed, err := os.ReadFile(outPath) // #nosec G304 -- outPath is a same-function os.CreateTemp staging file, never caller-controlled
	if err != nil {
		return fmt.Errorf("reading processed output: %w", err)
	}

	// The output is always MP4 — the storage path extension must follow
	// so ServeMedia advertises the right type.
	newPath := strings.TrimSuffix(m.StoragePath, ext) + ".mp4"

	stored := false

	if a.svc.s3 != nil {
		if err := a.svc.s3.putObject(ctx, m.StorageBucket, newPath, "video/mp4", processed); err != nil {
			slog.Warn("media:process_video s3 put failed, trying local fallback",
				"media_id", mediaID, "storage_path", newPath, "error", err)
		} else {
			stored = true
		}
	}

	if !stored {
		if err := storeLocalMediaFile(m.StorageBucket, newPath, processed); err != nil {
			return fmt.Errorf("storing processed object: %w", err)
		}
	}

	if err := a.svc.store.UpdateMediaStorage(ctx, gen.UpdateMediaStorageParams{
		ID:            id,
		StoragePath:   newPath,
		MimeType:      "video/mp4",
		FileSizeBytes: int64(len(processed)),
	}); err != nil {
		return fmt.Errorf("updating media row: %w", err)
	}

	// Best-effort cleanup of the original object when the path changed.
	if newPath != m.StoragePath {
		_ = a.svc.deleteObject(ctx, m.StorageBucket, m.StoragePath)
	}

	slog.Info("media:process_video: completed",
		"media_id", mediaID,
		"operation", operation,
		"old_bytes", len(data),
		"new_bytes", len(processed))

	return nil
}

// Ensure VideoProcessorAdapter satisfies the interface at compile time.
var _ interface {
	ProcessMediaVideo(ctx context.Context, mediaID, operation string) error
} = (*VideoProcessorAdapter)(nil)
