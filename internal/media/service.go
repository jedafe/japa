package media

import (
	"context"
	"errors"
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
	"image/jpeg":      true,
	"image/png":       true,
	"image/webp":      true,
	"image/gif":       true,
	"image/svg+xml":   true,
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
// The client uploads directly to SeaweedFS using the presigned URL.
func (s *Service) InitiateUpload(ctx context.Context, tenantID, uploaderID pgtype.UUID, filename, mimeType string, fileSize int64) (UploadResult, error) {
	// Guard: validate inputs early.
	if filename == "" {
		return UploadResult{}, errors.New("filename is required")
	}

	if mimeType == "" {
		return UploadResult{}, errors.New("mime_type is required")
	}

	if fileSize <= 0 {
		return UploadResult{}, errors.New("file_size_bytes must be positive")
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

	// Generate presigned URL for client upload.
	// Uses S3 client if configured, otherwise falls back to local path.
	expiresAt := time.Now().Add(15 * time.Minute)
	presignedURL := "/seaweedfs/upload/" + storagePath // fallback

	if s.s3 != nil {
		url, err := s.s3.PresignPut(ctx, storagePath, 15*time.Minute)
		if err != nil {
			slog.Warn("s3 presign failed, using fallback path",
				"storage_path", storagePath, "error", err)
		} else {
			presignedURL = url
		}
	}

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
		detected, matches, verifyErr := VerifyMIMEType(header, m.MimeType)
		if verifyErr != nil {
			slog.Warn("mime inspection: verification error, confirming without check",
				"media_id", mediaID.String(), "error", verifyErr)
			return s.store.ConfirmMedia(ctx, mediaID, tenantID)
		}

		if !matches {
			return fmt.Errorf("MIME mismatch: client claimed %s but content is %s",
				CanonicalMIMEType(m.MimeType), detected)
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
	const headerSize = 512

	// Try S3 client first if configured.
	if s.s3 != nil {
		return s.fetchFromS3(ctx, bucket, path, headerSize)
	}

	// Fallback to local filesystem (development).
	return s.fetchFromLocal(bucket, path, headerSize)
}

// fetchFromS3 reads the first n bytes from S3-compatible storage.
func (s *Service) fetchFromS3(ctx context.Context, bucket, path string, n int64) ([]byte, error) {
	// Construct the S3 GET URL
	objectURL := fmt.Sprintf("%s/%s/%s", s.s3.endpoint, bucket, path)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, objectURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating s3 request: %w", err)
	}

	req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", n-1))

	resp, err := s.s3.httpClient.Do(req)
	if err != nil {
		// Fall back to local filesystem if S3 is unreachable
		return s.fetchFromLocal(bucket, path, n)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return s.fetchFromLocal(bucket, path, n)
	}

	buf := make([]byte, n)

	read, err := io.ReadAtLeast(resp.Body, buf, 1)
	if err != nil {
		return nil, fmt.Errorf("reading s3 object header: %w", err)
	}

	return buf[:read], nil
}

// fetchFromLocal reads the first n bytes from local filesystem (development).
func (s *Service) fetchFromLocal(bucket, path string, n int64) ([]byte, error) {
	f, err := openLocalFile(bucket, path)
	if err != nil {
		return nil, fmt.Errorf("open file %s/%s: %w", bucket, path, err)
	}

	defer func() { _ = f.Close() }()

	buf := make([]byte, n)

	read, err := f.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("read file header: %w", err)
	}

	return buf[:read], nil
}

// openLocalFile attempts to open a file from the local filesystem for
// development. In production this would be replaced by an S3 client.
func openLocalFile(bucket, path string) (io.ReadCloser, error) {
	return openFile(fmt.Sprintf("%s/%s", bucket, path))
}

// openFileFunc is a package-level function variable so tests can override it.
var openFile = defaultOpenFile

func defaultOpenFile(path string) (io.ReadCloser, error) {
	return os.Open(path) //nolint:gosec // G304: path derived from validated media storage config
}

// getMediaForScan retrieves a media item by ID without tenant filtering.
// This is used by background workers that operate across all tenants.
func (s *Service) getMediaForScan(ctx context.Context, mediaID []byte) (ScanMediaRow, error) {
	return s.store.GetMediaForScan(ctx, mediaID)
}

// confirmMediaByID marks a media item as confirmed, scoped to tenant.
// Used by the background scan processor.
func (s *Service) confirmMediaByID(ctx context.Context, mediaID []byte, tenantID string) error {
	return s.store.ConfirmMediaByID(ctx, mediaID, tenantID)
}

// rejectMediaByID quarantines a media item, scoped to tenant.
// This gives admins a chance to review false positives before removal.
func (s *Service) rejectMediaByID(ctx context.Context, mediaID []byte, tenantID string, virusName string) error {
	return s.store.QuarantineMedia(ctx, mediaID, tenantID, virusName)
}

// ApproveMedia clears quarantine and confirms the media item.
func (s *Service) ApproveMedia(ctx context.Context, mediaID, tenantID, reviewerID pgtype.UUID) error {
	return s.store.ApproveQuarantined(ctx, mediaID, tenantID, reviewerID)
}

// RejectMedia soft-deletes a quarantined media item after admin review.
func (s *Service) RejectMedia(ctx context.Context, mediaID, tenantID, reviewerID pgtype.UUID) error {
	return s.store.RejectQuarantined(ctx, mediaID, tenantID, reviewerID)
}

// AdminUpdateStatus allows admin to change media status (approve/reject/quarantine) for any item.
func (s *Service) AdminUpdateStatus(ctx context.Context, tenantID, mediaID, reviewerID pgtype.UUID, confirmed bool, quarantineReason *string) error {
	var reason string
	if quarantineReason != nil {
		reason = *quarantineReason
	}
	return s.store.AdminUpdateStatus(ctx, gen.AdminUpdateMediaStatusParams{
		ID:               mediaID,
		TenantID:         tenantID,
		Confirmed:        confirmed,
		QuarantineReason: reason,
		ReviewedBy:       reviewerID,
	})
}

// ListQuarantined returns all quarantined media items for a tenant.
func (s *Service) ListQuarantined(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]QuarantinedMedia, error) {
	return s.store.ListQuarantined(ctx, tenantID, limit, offset)
}

// GetQuarantinedMedia returns a single quarantined media item by ID.
func (s *Service) GetQuarantinedMedia(ctx context.Context, mediaID, tenantID pgtype.UUID) (QuarantinedMedia, error) {
	return s.store.GetQuarantinedMedia(ctx, mediaID, tenantID)
}

// GetMedia returns a single media item by ID.
func (s *Service) GetMedia(ctx context.Context, tenantID, mediaID pgtype.UUID) (gen.Medium, error) {
	return s.store.GetMediaByID(ctx, mediaID, tenantID)
}

// UpdateMetadata updates alt_text, caption, dimensions, and user_group_id on a media item.
func (s *Service) UpdateMetadata(ctx context.Context, tenantID, mediaID pgtype.UUID, altText, caption *string, width, height *int32, duration *float64, userGroupID pgtype.UUID) error {
	var durationNum pgtype.Numeric
	if duration != nil {
		if err := durationNum.Scan(fmt.Sprintf("%f", *duration)); err != nil {
			return fmt.Errorf("invalid duration %v: %w", *duration, err)
		}
	}

	return s.store.UpdateMediaMetadata(ctx, gen.UpdateMediaMetadataParams{
		ID:           mediaID,
		TenantID:     tenantID,
		AltText:      altText,
		Caption:      caption,
		WidthPx:      width,
		HeightPx:     height,
		DurationSecs: durationNum,
		UserGroupID:  userGroupID,
	})
}

// DeleteMedia soft-deletes a media item.
func (s *Service) DeleteMedia(ctx context.Context, tenantID, mediaID pgtype.UUID) error {
	return s.store.SoftDeleteMedia(ctx, mediaID, tenantID)
}

// ListMedia returns paginated media, optionally filtered by uploader, type,
// search query, status, and flagged state, along with the total count matching
// the filter. The total is used by callers to compute total_pages. Empty/zero
// filter values mean "no filter on this field".
func (s *Service) ListMedia(ctx context.Context, tenantID pgtype.UUID, uploaderID *pgtype.UUID, mimeType, query, status, flagged *string, limit, offset int32) ([]gen.Medium, int32, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}
	if offset < 0 {
		offset = 0
	}

	var uploader pgtype.UUID
	if uploaderID != nil {
		uploader = *uploaderID
	}

	mimePattern := ""
	if mimeType != nil {
		mimePattern = *mimeType
	}

	searchQuery := ""
	if query != nil {
		searchQuery = *query
	}

	statusFilter := ""
	if status != nil {
		statusFilter = *status
	}

	flaggedFilter := ""
	if flagged != nil {
		flaggedFilter = *flagged
	}

	items, err := s.store.ListMediaFiltered(ctx, tenantID, uploader, mimePattern, searchQuery, statusFilter, flaggedFilter, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.store.CountMediaFiltered(ctx, tenantID, uploader, mimePattern, searchQuery, statusFilter, flaggedFilter)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ListByUser returns the media on the user's published public posts (featured/
// header images), plus the total count. Public-safe: excludes unconfirmed,
// group-restricted, and draft/private-post media. Powers the profile Media tab.
func (s *Service) ListByUser(ctx context.Context, tenantID, userID pgtype.UUID, limit, offset int32) ([]gen.Medium, int32, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}
	if offset < 0 {
		offset = 0
	}
	items, err := s.store.ListMediaFromPublicPostsByUser(ctx, tenantID, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing media from public posts: %w", err)
	}
	total, err := s.store.CountMediaFromPublicPostsByUser(ctx, tenantID, userID)
	if err != nil {
		return nil, 0, fmt.Errorf("counting media from public posts: %w", err)
	}
	return items, total, nil
}

// ListByOrganization returns media from the org's public posts, with a total
// count. Powers the org profile Media tab.
func (s *Service) ListByOrganization(ctx context.Context, tenantID, orgID pgtype.UUID, limit, offset int32) ([]gen.Medium, int32, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}
	if offset < 0 {
		offset = 0
	}
	items, err := s.store.ListMediaFromOrgPublicPosts(ctx, tenantID, orgID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing media from org public posts: %w", err)
	}
	total, err := s.store.CountMediaFromOrgPublicPosts(ctx, tenantID, orgID)
	if err != nil {
		return nil, 0, fmt.Errorf("counting media from org public posts: %w", err)
	}
	return items, total, nil
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

// DirectUpload stores a file locally and creates the media record.
func (s *Service) DirectUpload(ctx context.Context, tenantID, uploaderID pgtype.UUID, filename, mimeType string, fileSize int64, localPath, storagePath string) (pgtype.UUID, error) {
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
		return pgtype.UUID{}, fmt.Errorf("inserting media record: %w", err)
	}

	if err := s.store.ConfirmMedia(ctx, m.ID, tenantID); err != nil {
		return pgtype.UUID{}, fmt.Errorf("confirming media: %w", err)
	}

	return m.ID, nil
}

// StoreFromBytes stores raw bytes as a confirmed media item. When attribution
// is non-nil, the source_* columns are populated for stock photo attribution.
func (s *Service) StoreFromBytes(ctx context.Context, tenantID, uploaderID pgtype.UUID, data []byte, filename string, attribution *MediaAttribution) (pgtype.UUID, error) {
	if len(data) == 0 {
		return pgtype.UUID{}, errors.New("data is empty")
	}
	if int64(len(data)) > MaxFileSizeBytes {
		return pgtype.UUID{}, fmt.Errorf("data too large: %d bytes exceeds limit of %d bytes", len(data), MaxFileSizeBytes)
	}

	// Sniff MIME type from content.
	mimeType := http.DetectContentType(data)

	// Generate unique storage path.
	ext := extensionFromMIME(mimeType)
	storagePath := fmt.Sprintf("%s/%s/%s%s",
		StoragePathMedia,
		tenantID.String(),
		uuid.New().String(),
		ext,
	)

	// Upload to S3/SeaweedFS with local fallback.
	stored := false
	if s.s3 != nil {
		if err := s.s3.PutObject(ctx, storagePath, mimeType, data); err != nil {
			slog.Warn("media: s3 put failed, trying local fallback",
				"storage_path", storagePath, "error", err)
		} else {
			stored = true
		}
	}
	if !stored {
		if err := storeLocalMediaFile(DefaultBucket, storagePath, data); err != nil {
			return pgtype.UUID{}, fmt.Errorf("storing to local fallback: %w", err)
		}
	}

	// Build insert params with optional attribution.
	params := gen.InsertMediaParams{
		TenantID:         tenantID,
		UploaderUserID:   uploaderID,
		OriginalFilename: filename,
		MimeType:         mimeType,
		FileSizeBytes:    int64(len(data)),
		StoragePath:      storagePath,
		StorageBucket:    DefaultBucket,
	}
	if attribution != nil && attribution.SourcePlatform != "" {
		params.SourcePlatform = &attribution.SourcePlatform
		params.SourcePhotographer = &attribution.SourcePhotographer
		params.SourcePhotographerUrl = &attribution.SourcePhotographerURL
		params.SourceLicense = &attribution.SourceLicense
		params.SourceLicenseUrl = &attribution.SourceLicenseURL
		params.SourcePhotoID = &attribution.SourcePhotoID
	}

	m, err := s.store.InsertMedia(ctx, params)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("inserting media record: %w", err)
	}

	if err := s.store.ConfirmMedia(ctx, m.ID, tenantID); err != nil {
		return pgtype.UUID{}, fmt.Errorf("confirming media: %w", err)
	}

	return m.ID, nil
}

// StoreFromURL downloads content from a remote URL, sniffs the MIME type
// from the content (not the extension), uploads it to SeaweedFS, and creates
// a media record. Returns the media ID. Used by server-side ingestion tasks
// like university:fetch_logo (Section 39).
//
// The uploaderID is the system/admin user ID to attribute the media to.
func (s *Service) StoreFromURL(ctx context.Context, tenantID, uploaderID pgtype.UUID, sourceURL, filename string) (pgtype.UUID, error) {
	// 1. Download the image.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("creating download request: %w", err)
	}
	req.Header.Set("User-Agent", "JapaConnect/1.0 (university:fetch_logo)")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("downloading logo from %s: %w", sourceURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return pgtype.UUID{}, fmt.Errorf("logo download returned %d for %s", resp.StatusCode, sourceURL)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxFileSizeBytes))
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("reading logo body: %w", err)
	}

	if len(body) == 0 {
		return pgtype.UUID{}, fmt.Errorf("logo body is empty from %s", sourceURL)
	}

	return s.StoreFromBytes(ctx, tenantID, uploaderID, body, filename, nil)
}

// storeLocalMediaFile writes body to disk at data/media/{bucket}/{storagePath}
// (mediaRoot, defined in handlers.go). This is the local-storage fallback
// used when S3/SeaweedFS is not configured or unreachable, so ServeMedia
// can still retrieve the file.
func storeLocalMediaFile(bucket, storagePath string, body []byte) error {
	localPath, err := resolveMediaPath(bucket, storagePath)
	if err != nil {
		return fmt.Errorf("resolving local media path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return fmt.Errorf("creating media directory: %w", err)
	}
	if err := os.WriteFile(localPath, body, 0o644); err != nil { //nolint:gosec // G306: media files are meant to be world-readable via ServeMedia
		return fmt.Errorf("writing media file: %w", err)
	}
	return nil
}

// extensionFromMIME returns a file extension for common image MIME types.
func extensionFromMIME(mimeType string) string {
	switch {
	case strings.HasPrefix(mimeType, "image/png"):
		return ".png"
	case strings.HasPrefix(mimeType, "image/jpeg"):
		return ".jpg"
	case strings.HasPrefix(mimeType, "image/webp"):
		return ".webp"
	case strings.HasPrefix(mimeType, "image/gif"):
		return ".gif"
	case strings.HasPrefix(mimeType, "image/svg"):
		return ".svg"
	default:
		return ".bin"
	}
}
