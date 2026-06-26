package media

import (
	"context"
	"fmt"
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
func (s *Service) ConfirmUpload(ctx context.Context, tenantID, mediaID pgtype.UUID) error {
	return s.store.ConfirmMedia(ctx, mediaID, tenantID)
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
