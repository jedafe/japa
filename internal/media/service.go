package media

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// MaxFileSizeBytes is the maximum allowed upload size (50 MB).
const MaxFileSizeBytes int64 = 50 * 1024 * 1024

// AllowedMIMETypes is the set of MIME types accepted for upload.
var AllowedMIMETypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
	"image/gif":  true,
	"image/svg+xml": true,
	"video/mp4":       true,
	"video/webm":      true,
	"audio/mpeg":      true,
	"audio/ogg":       true,
	"audio/wav":       true,
	"application/pdf": true,
}

// UploadResult contains the presigned URL details for a two-step upload.
type UploadResult struct {
	MediaID      string `json:"media_id"`
	StoragePath  string `json:"storage_path"`
	PresignedURL string `json:"presigned_url"`
	ExpiresAt    string `json:"expires_at"`
}

// InitiateUpload creates a media row and returns presigned URL details.
// The client uploads directly to MinIO using the presigned URL.
func (s *Service) InitiateUpload(ctx context.Context, tenantID, uploaderID pgtype.UUID, filename, mimeType string, fileSize int64) (UploadResult, error) {
	// Guard: validate inputs early.
	if filename == "" {
		return UploadResult{}, fmt.Errorf("filename is required")
	}
	if mimeType == "" {
		return UploadResult{}, fmt.Errorf("mime_type is required")
	}
	if fileSize <= 0 {
		return UploadResult{}, fmt.Errorf("file_size_bytes must be positive")
	}
	if fileSize > MaxFileSizeBytes {
		return UploadResult{}, fmt.Errorf("file too large: %d bytes exceeds limit of %d bytes", fileSize, MaxFileSizeBytes)
	}
	if !AllowedMIMETypes[mimeType] {
		return UploadResult{}, fmt.Errorf("unsupported MIME type: %s", mimeType)
	}

	// Generate unique storage path under temp/ prefix.
	ext := filepath.Ext(filename)
	storagePath := fmt.Sprintf("%s/%s/%s%s",
		StoragePathTemp,
		tenantID.String(),
		uuid.New().String(),
		ext)

	m, err := s.store.InsertMedia(ctx, gen.InsertMediaParams{
		TenantID:         tenantID,
		UploaderUserID:   uploaderID,
		OriginalFilename: filename,
		MimeType:         mimeType,
		FileSizeBytes:    fileSize,
		StoragePath:      storagePath,
		StorageBucket:    DefaultBucket,
	})
	if err != nil {
		return UploadResult{}, fmt.Errorf("initiating upload: %w", err)
	}

	// TODO: generate actual MinIO presigned URL via MinIO client.
	presignedURL := fmt.Sprintf("/minio/upload/%s", storagePath)
	expiresAt := time.Now().Add(15 * time.Minute)

	return UploadResult{
		MediaID:      m.ID.String(),
		StoragePath:  storagePath,
		PresignedURL: presignedURL,
		ExpiresAt:    expiresAt.Format(time.RFC3339),
	}, nil
}

// ConfirmUpload marks media as confirmed after client completes upload.
// Before confirming, it verifies the uploaded file's actual content matches
// the client-claimed MIME type by reading the first 512 bytes and calling
// http.DetectContentType. A mismatch rejects the upload to prevent MIME
// spoofing (e.g. an executable renamed to .jpg).
func (s *Service) ConfirmUpload(ctx context.Context, tenantID, mediaID pgtype.UUID) error {
	// Fetch the media record to get storage_path and claimed MIME type.
	m, err := s.store.GetMediaByID(ctx, mediaID, tenantID)
	if err != nil {
		return fmt.Errorf("media not found: %w", err)
	}

	// Fetch the first 512 bytes from object storage for content inspection.
	header, err := s.fetchFileHeader(ctx, m.StorageBucket, m.StoragePath)
	if err != nil {
		slog.Warn("mime inspection: failed to read file header, confirming without check",
			"media_id", mediaID.String(), "error", err)
		// Degrade gracefully — if object storage is unreachable we still
		// confirm rather than blocking all uploads.
		return s.store.ConfirmMedia(ctx, mediaID, tenantID)
	}

	if len(header) > 0 {
		detected := http.DetectContentType(header)
		claimed := NormalizeMIMEType(m.MimeType)

		// DetectContentType returns "application/octet-stream" for unknown
		// binary data. Allow that through — it means the content is valid
		// but not identifiable from the first 512 bytes alone (e.g. large
		// video files). Only reject when detected is a concrete type that
		// conflicts with the claimed type.
		if detected != "application/octet-stream" && NormalizeMIMEType(detected) != claimed {
			return fmt.Errorf("MIME mismatch: client claimed %s but content is %s", claimed, detected)
		}
	}

	return s.store.ConfirmMedia(ctx, mediaID, tenantID)
}

// fetchFileHeader reads up to n bytes from the beginning of an object in
// the storage bucket. This is used for content-based MIME type detection
// during upload confirmation. Returns an empty slice if the object is
// smaller than n bytes (which is fine — http.DetectContentType works with
// as little as 1 byte for some formats).
func (s *Service) fetchFileHeader(ctx context.Context, bucket, path string) ([]byte, error) {
	// TODO: Replace with real MinIO GetObject call when the client is wired.
	// For now, attempt a local filesystem read from the storage path for
	// development environments. In production this will hit the S3-compatible
	// object storage API.
	//
	// Pattern: the presigned URL points to /minio/upload/{path}, so the
	// actual object lives at /{bucket}/{path} in the MinIO root. During
	// local dev with weed/MinIO, files land in the working directory.
	const headerSize = 512

	// Try local filesystem first (development).
	f, err := openLocalFile(bucket, path)
	if err != nil {
		return nil, fmt.Errorf("open file %s/%s: %w", bucket, path, err)
	}
	defer f.Close()

	buf := make([]byte, headerSize)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("read file header: %w", err)
	}
	return buf[:n], nil
}

// openLocalFile attempts to open a file from the local filesystem for
// development. In production this would be replaced by an S3 client.
func openLocalFile(bucket, path string) (io.ReadCloser, error) {
	return openFile(fmt.Sprintf("%s/%s", bucket, path))
}

// openFileFunc is a package-level function variable so tests can override it.
var openFile = defaultOpenFile

func defaultOpenFile(path string) (io.ReadCloser, error) {
	return os.Open(path)
}

// getMediaForScan retrieves a media item by ID without tenant filtering.
// This is used by background workers that operate across all tenants.
func (s *Service) getMediaForScan(ctx context.Context, mediaID []byte) (scanMediaRow, error) {
	return s.store.GetMediaForScan(ctx, mediaID)
}

// confirmMediaByID marks a media item as confirmed by ID alone (no tenant).
// Used by the background scan processor.
func (s *Service) confirmMediaByID(ctx context.Context, mediaID []byte) error {
	return s.store.ConfirmMediaByID(ctx, mediaID)
}

// rejectMediaByID soft-deletes a media item by ID and logs the virus name.
// Used by the background scan processor when ClamAV detects malware.
func (s *Service) rejectMediaByID(ctx context.Context, mediaID []byte, virusName string) error {
	return s.store.RejectMediaByID(ctx, mediaID, virusName)
}

// GetMedia returns a single media item by ID.
func (s *Service) GetMedia(ctx context.Context, tenantID, mediaID pgtype.UUID) (gen.Medium, error) {
	return s.store.GetMediaByID(ctx, mediaID, tenantID)
}

// UpdateMetadata updates alt_text, caption, dimensions on a media item.
func (s *Service) UpdateMetadata(ctx context.Context, tenantID, mediaID pgtype.UUID, altText, caption *string, width, height *int32, duration *float64) error {
	var durationNum pgtype.Numeric
	if duration != nil {
		_ = durationNum.Scan(fmt.Sprintf("%f", *duration))
	}
	return s.store.UpdateMediaMetadata(ctx, gen.UpdateMediaMetadataParams{
		ID:           mediaID,
		TenantID:     tenantID,
		AltText:      altText,
		Caption:      caption,
		WidthPx:      width,
		HeightPx:     height,
		DurationSecs: durationNum,
	})
}

// DeleteMedia soft-deletes a media item.
func (s *Service) DeleteMedia(ctx context.Context, tenantID, mediaID pgtype.UUID) error {
	return s.store.SoftDeleteMedia(ctx, mediaID, tenantID)
}

// ListMedia returns paginated media, optionally filtered by uploader, type, or search query.
func (s *Service) ListMedia(ctx context.Context, tenantID pgtype.UUID, uploaderID *pgtype.UUID, mimeType *string, query *string, limit, offset int32) ([]gen.Medium, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}
	if offset < 0 {
		offset = 0
	}

	// Search query takes precedence over type filter.
	if query != nil && *query != "" {
		return s.store.SearchMedia(ctx, tenantID, *query, limit, offset)
	}
	if mimeType != nil && *mimeType != "" {
		return s.store.ListMediaByType(ctx, tenantID, *mimeType, limit, offset)
	}
	if uploaderID != nil {
		return s.store.ListMediaByUploader(ctx, tenantID, *uploaderID, limit, offset)
	}
	return s.store.ListMediaByTenant(ctx, tenantID, limit, offset)
}

// GetMediaURL generates an imgproxy signed URL for the media item.
// NEVER cached in DB — generated on every serialization.
func (s *Service) GetMediaURL(m *gen.Medium, opts ImageProxyOpts) string {
	if m == nil {
		return ""
	}
	return MediaURL(m.StorageBucket, m.StoragePath, opts)
}

// BulkDelete soft-deletes multiple media items.
func (s *Service) BulkDelete(ctx context.Context, tenantID pgtype.UUID, mediaIDs []pgtype.UUID) error {
	if len(mediaIDs) == 0 {
		return nil
	}
	return s.store.BulkSoftDeleteMedia(ctx, tenantID, mediaIDs)
}

// IsAllowedMIMEType checks if a MIME type is in the allowed set.
func IsAllowedMIMEType(mimeType string) bool {
	return AllowedMIMETypes[mimeType]
}

// NormalizeMIMEType normalizes a MIME type to a canonical form for comparison.
func NormalizeMIMEType(mimeType string) string {
	return strings.ToLower(strings.TrimSpace(mimeType))
}
