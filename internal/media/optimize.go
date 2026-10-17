package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	_ "golang.org/x/image/webp" // register webp decoder for image.Decode
	_ "image/gif"               // register gif decoder for image.Decode
	_ "image/jpeg"              // register jpeg decoder for image.Decode
	_ "image/png"               // register png decoder for image.Decode

	webpenc "github.com/gen2brain/webp"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"golang.org/x/image/draw"
)

// Image storage optimization constants. Raster images larger than
// OptimizeSkipBytes are downscaled to OptimizeMaxDimension and re-encoded
// as WebP so multi-megabyte originals from uploads and curated ingestion
// (Wikidata logos, stock photos, video thumbnails) don't accumulate in
// storage. Lossy WebP keeps alpha channels, so transparent PNGs are safe.
const (
	// OptimizeMaxDimension caps stored raster width/height in pixels.
	OptimizeMaxDimension = 2048
	// OptimizeSkipBytes keeps already-small images byte-identical.
	OptimizeSkipBytes = 256 * 1024
	// optimizeWebPQuality is the WebP encode quality for re-encoded images.
	optimizeWebPQuality = 80
	// optimizeMaxPixels bounds decode memory — images reporting more than
	// ~67MP are left byte-identical rather than decoding into an unbounded
	// bitmap (a small crafted PNG can decompress to gigabytes).
	optimizeMaxPixels = 64 << 20
)

// OptimizeMIME is the MIME type optimizeImage emits for re-encoded rasters.
const OptimizeMIME = "image/webp"

// optimizeImage downscales and re-encodes a raster image when its byte size
// exceeds OptimizeSkipBytes. Returns the optimized WebP bytes plus
// OptimizeMIME. Animated GIFs, SVGs, non-images, un-decodable payloads, and
// re-encodes that would inflate the file all return the original bytes and
// MIME unchanged.
func optimizeImage(data []byte, mimeType string) ([]byte, string) {
	if len(data) < OptimizeSkipBytes {
		return data, mimeType
	}

	canonical := CanonicalMIMEType(mimeType)
	if !strings.HasPrefix(canonical, "image/") ||
		canonical == "image/gif" ||
		canonical == "image/svg+xml" {
		return data, mimeType
	}

	// Probe dimensions before decoding: pixel-bomb payloads (tiny file,
	// huge canvas) are skipped rather than allocating the bitmap.
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || int64(cfg.Width)*int64(cfg.Height) > optimizeMaxPixels {
		return data, mimeType
	}

	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return data, mimeType
	}

	img := src
	b := src.Bounds()

	w, h := b.Dx(), b.Dy()
	if w > OptimizeMaxDimension || h > OptimizeMaxDimension {
		scale := float64(OptimizeMaxDimension) / float64(max(w, h))
		nw := max(1, int(float64(w)*scale+0.5))
		nh := max(1, int(float64(h)*scale+0.5))
		dst := image.NewNRGBA(image.Rect(0, 0, nw, nh))
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
		img = dst
	}

	var buf bytes.Buffer
	if err := webpenc.Encode(&buf, img, webpenc.Options{Quality: optimizeWebPQuality}); err != nil {
		return data, mimeType
	}

	// Never inflate: keep the original when re-encoding produced a
	// larger file (already well-optimized inputs).
	if buf.Len() >= len(data) {
		return data, mimeType
	}

	return buf.Bytes(), OptimizeMIME
}

// OptimizeMedia re-encodes a stored media object in place: fetches the
// original, runs optimizeImage, rewrites storage (under a .webp path when
// the format changed), and updates the media row's mime_type,
// file_size_bytes, and storage_path. Idempotent — rows that are already
// optimal return nil without touching storage.
func (s *Service) OptimizeMedia(ctx context.Context, mediaID pgtype.UUID) error {
	m, err := s.store.GetMediaByID(ctx, mediaID)
	if err != nil {
		return fmt.Errorf("loading media %s: %w", mediaID.String(), err)
	}

	canonical := CanonicalMIMEType(m.MimeType)
	if !strings.HasPrefix(canonical, "image/") ||
		canonical == "image/gif" ||
		canonical == "image/svg+xml" {
		return nil // nothing optimizable — skip the object fetch entirely
	}

	// The row may have entered quarantine or been rejected between enqueue
	// and task run — leave review-state objects untouched.
	if m.QuarantinedAt.Valid || m.RejectedAt.Valid {
		return nil
	}

	data, err := s.fetchObjectBytes(ctx, m.StorageBucket, m.StoragePath, MaxMediaUploadBody)
	if err != nil {
		if errors.Is(err, errObjectMissing) {
			// Row references an object that does not exist — nothing to
			// optimize; returning nil stops pointless retries.
			slog.Warn("media: optimize skipped — stored object missing",
				"media_id", mediaID.String(),
				"storage_path", m.StoragePath)

			return nil
		}

		return fmt.Errorf("reading stored object %s/%s: %w", m.StorageBucket, m.StoragePath, err)
	}

	optimized, outMIME := optimizeImage(data, m.MimeType)
	if outMIME == m.MimeType && bytes.Equal(optimized, data) {
		return nil // already optimal
	}

	// When the format changes the storage path extension must follow, so
	// ServeMedia and imgproxy URLs advertise the right type.
	newPath := m.StoragePath
	if outMIME != m.MimeType {
		newPath = strings.TrimSuffix(m.StoragePath, filepath.Ext(m.StoragePath)) + extensionFromMIME(outMIME)
	}

	stored := false

	if s.s3 != nil {
		if err := s.s3.putObject(ctx, m.StorageBucket, newPath, outMIME, optimized); err != nil {
			slog.Warn("media: optimize s3 put failed, trying local fallback",
				"media_id", mediaID.String(), "storage_path", newPath, "error", err)
		} else {
			stored = true
		}
	}

	if !stored {
		if err := storeLocalMediaFile(m.StorageBucket, newPath, optimized); err != nil {
			return fmt.Errorf("storing optimized object: %w", err)
		}
	}

	if err := s.store.UpdateMediaStorage(ctx, gen.UpdateMediaStorageParams{
		ID:            mediaID,
		StoragePath:   newPath,
		MimeType:      outMIME,
		FileSizeBytes: int64(len(optimized)),
	}); err != nil {
		return fmt.Errorf("updating media row: %w", err)
	}

	slog.Info("media: optimized stored object",
		"media_id", mediaID.String(),
		"old_bytes", len(data),
		"new_bytes", len(optimized),
		"old_mime", m.MimeType,
		"new_mime", outMIME,
	)

	// Best-effort cleanup of the old object when the path changed.
	if newPath != m.StoragePath {
		_ = s.deleteObject(ctx, m.StorageBucket, m.StoragePath)
	}

	return nil
}

// fetchObjectBytes reads a full stored object — S3 when configured, local
// disk otherwise — capped at limit+1 bytes. Exceeding the limit is an
// error so oversized objects fail loudly rather than truncating.
func (s *Service) fetchObjectBytes(ctx context.Context, bucket, path string, limit int64) ([]byte, error) {
	if s.s3 != nil {
		objectURL := fmt.Sprintf("%s/%s/%s", s.s3.endpoint, bucket, path)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, objectURL, nil)
		if err == nil {
			resp, err := s.s3.httpClient.Do(req)
			if err == nil {
				defer resp.Body.Close()

				if resp.StatusCode == http.StatusOK {
					return readLimited(resp.Body, limit)
				}
			}
		}
		// Fall through to local on any S3 failure (matches fetchFromS3).
	}

	f, err := openLocalFile(bucket, path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errObjectMissing
		}

		return nil, fmt.Errorf("open file %s/%s: %w", bucket, path, err)
	}

	defer func() { _ = f.Close() }()

	return readLimited(f, limit)
}

func readLimited(r io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}

	if int64(len(data)) > limit {
		return nil, fmt.Errorf("object exceeds read limit of %d bytes", limit)
	}

	return data, nil
}

// deleteObject removes a stored object from every location it may live —
// S3 and the local fallback — since the original may have been written
// through either path (local fallback on S3 write failure). Returns a
// joined error so GC callers can retry later; callers that only need
// best-effort cleanup ignore the return.
func (s *Service) deleteObject(ctx context.Context, bucket, path string) error {
	var errs []error

	if s.s3 != nil {
		if err := s.s3.DeleteObject(ctx, bucket, path); err != nil {
			slog.Warn("media: failed to delete old object", "bucket", bucket, "path", path, "error", err)
			errs = append(errs, err)
		}
	}

	localPath, err := resolveMediaPath(bucket, path)
	if err != nil {
		slog.Warn("media: could not resolve old object path", "bucket", bucket, "path", path, "error", err)
		errs = append(errs, err)
	} else if err := os.Remove(localPath); err != nil && !os.IsNotExist(err) {
		slog.Warn("media: failed to delete old object", "path", localPath, "error", err)
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}
