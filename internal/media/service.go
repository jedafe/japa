package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// MaxFileSizeBytes is the fallback maximum allowed upload size when
// media.max_upload_mb is not configured in system_settings (50 MB).
const MaxFileSizeBytes int64 = 50 * 1024 * 1024

// MaxUploadBytes returns the effective maximum upload size in bytes,
// consulting the media.max_upload_mb system_setting (Rule 3) and falling
// back to MaxFileSizeBytes (50 MB) when unset or the settings provider is
// not wired. Shared by InitiateUpload, DirectUpload, StoreFromBytes, and
// AdminUpload so every upload path honours the same configurable limit.
func (s *Service) MaxUploadBytes(ctx context.Context) int64 {
	maxBytes := MaxFileSizeBytes

	if s.settings != nil {
		if maxMB := s.settings.GetIntOr(ctx, "media.max_upload_mb", 50); maxMB > 0 {
			maxBytes = int64(maxMB) * 1024 * 1024
		}
	}

	return maxBytes
}

// ErrMediaNotFound is returned when a media item does not exist or does not
// belong to the requesting user. The handler maps this to 404 to avoid
// leaking existence information to unauthorized users.
var ErrMediaNotFound = errors.New("media not found")

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
// uploadCtx selects which MIME allowlist applies (§34); "document" permits
// CV-style attachments used by job applications (§63).
func (s *Service) InitiateUpload(ctx context.Context, uploaderID pgtype.UUID, filename, mimeType string, fileSize int64, uploadCtx UploadContext) (UploadResult, error) {
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

	// Max upload size from system_settings 'media.max_upload_mb' (Rule 3).
	// Falls back to MaxFileSizeBytes (50 MB) when settings are not configured.
	maxBytes := s.MaxUploadBytes(ctx)

	if fileSize > maxBytes {
		return UploadResult{}, fmt.Errorf("file too large: %d bytes exceeds limit of %d bytes", fileSize, maxBytes)
	}

	if !s.isAllowedMIME(ctx, mimeType, uploadCtx) {
		return UploadResult{}, fmt.Errorf("unsupported MIME type: %s", mimeType)
	}

	// Generate unique storage path under temp/ prefix.
	ext := filepath.Ext(filename)
	storagePath := fmt.Sprintf("%s/%s%s", StoragePathTemp,

		uuid.New().String(),
		ext)

	bucket := s.storageBucket(ctx)

	m, err := s.store.InsertMedia(ctx, gen.InsertMediaParams{

		UploaderUserID:   uploaderID,
		OriginalFilename: filename,
		MimeType:         mimeType,
		FileSizeBytes:    fileSize,
		StoragePath:      storagePath,
		StorageBucket:    bucket,
	})
	if err != nil {
		return UploadResult{}, fmt.Errorf("initiating upload: %w", err)
	}

	// Generate presigned URL for client upload.
	// Uses S3 client if configured, otherwise falls back to local path.
	expiresAt := time.Now().Add(15 * time.Minute)
	presignedURL := "/seaweedfs/upload/" + bucket + "/" + storagePath // fallback

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
//
// BUG-004: the uploaderID parameter enforces that only the user who
// initiated the upload can confirm it. Without this, any authenticated
// user in the same platform could confirm another user's media ID by
// guessing it. The presigned URL is single-use, but the confirm endpoint
// itself must be uploader-scoped per §34 two-step upload.
func (s *Service) ConfirmUpload(ctx context.Context, mediaID, uploaderID pgtype.UUID) error {
	// Fetch the media record to get storage_path and claimed MIME type.
	m, err := s.store.GetMediaByID(ctx, mediaID)
	if err != nil {
		return fmt.Errorf("media not found: %w", err)
	}

	// BUG-004: verify the confirming user is the uploader. Return a generic
	// "not found" error so existence is not leaked to unauthorized users.
	if m.UploaderUserID != uploaderID {
		return ErrMediaNotFound
	}

	// Verify the stored object exists and its real size BEFORE confirming —
	// the presigned PUT is unauthenticated, so a client can confirm without
	// ever uploading, or declare a small file at initiate and write a much
	// larger object. A definitively-missing object is a failed upload, not
	// a degradation: reject it. Oversize objects are deleted rather than
	// confirmed. The row's file_size_bytes recorded the claimed size at
	// initiate — persist the real byte count so listings stay accurate.
	size, serr := s.objectSizeBytes(ctx, m.StorageBucket, m.StoragePath)
	switch {
	case errors.Is(serr, errObjectMissing):
		return fmt.Errorf("%w: upload did not complete — object not found in storage", errObjectMissing)
	case serr != nil:
		// Storage unreachable — the size was never verified, so confirm
		// must not proceed (an unreachable backend could mask a missing
		// or oversized object). Reject as retryable; the row stays
		// unconfirmed and the client can re-confirm after recovery, or
		// the nightly scan sweep finalizes it.
		return fmt.Errorf("%w: %v", errStorageUnavailable, serr)
	case size <= 0:
		return fmt.Errorf("%w: uploaded object is empty", errUploadInvalid)
	case size > s.MaxUploadBytes(ctx):
		s.deleteObject(ctx, m.StorageBucket, m.StoragePath)
		return fmt.Errorf("%w: uploaded object is %d bytes, exceeding the %d byte upload limit",
			errUploadInvalid, size, s.MaxUploadBytes(ctx))
	case size != m.FileSizeBytes:
		_ = s.store.UpdateMediaStorage(ctx, gen.UpdateMediaStorageParams{
			ID:            mediaID,
			StoragePath:   m.StoragePath,
			MimeType:      m.MimeType,
			FileSizeBytes: size,
		})
	}

	// Fetch the first 512 bytes from object storage for content inspection.
	// The size check above already established the object exists, so a
	// missing-file error here means it raced away; anything else is an
	// indeterminate storage state — reject as retryable rather than
	// confirming unverified content.
	header, err := s.fetchFileHeader(ctx, m.StorageBucket, m.StoragePath)
	if err != nil {
		if errors.Is(err, errObjectMissing) || os.IsNotExist(err) {
			return fmt.Errorf("%w: upload did not complete — object not found in storage", errObjectMissing)
		}
		return fmt.Errorf("%w: %v", errStorageUnavailable, err)
	}
	if len(header) == 0 {
		return fmt.Errorf("%w: could not read stored object bytes", errStorageUnavailable)
	}

	detected, matches, verifyErr := VerifyMIMEType(header, m.MimeType)
	if verifyErr != nil {
		return fmt.Errorf("mime verification failed: %w", verifyErr)
	}
	if !matches {
		return fmt.Errorf("%w: MIME mismatch — client claimed %s but content is %s",
			errUploadInvalid, CanonicalMIMEType(m.MimeType), detected)
	}

	return s.store.ConfirmMedia(ctx, mediaID)
}

// errObjectMissing marks a definitively-missing storage object — S3 404 or
// absent local file — as opposed to storage being unreachable. ConfirmUpload
// treats missing objects as a failed upload (reject) and unreachable
// storage as retryable (errStorageUnavailable) — neither confirms.
var errObjectMissing = errors.New("media: stored object not found")

// errUploadInvalid marks a stored upload object that failed confirmation
// validation (empty, or over the configured size limit) — a 400-class
// client error, distinct from storage failures.
var errUploadInvalid = errors.New("media: uploaded object failed validation")

// errStorageUnavailable marks an indeterminate storage state (unreachable
// backend, transient failures) during confirm — a 503-class retryable
// error. The row stays unconfirmed so the client can retry and the
// nightly scan sweep remains a backstop.
var errStorageUnavailable = errors.New("media: storage temporarily unavailable")

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

// objectSizeBytes returns the stored object's real size — S3 HEAD when the
// client is configured, local stat otherwise. Returns errObjectMissing when
// the object definitively does not exist; other errors mean the size could
// not be determined (storage unreachable — ConfirmUpload rejects those as
// retryable rather than confirming unverified objects).
func (s *Service) objectSizeBytes(ctx context.Context, bucket, path string) (int64, error) {
	var upstreamErr error
	notFound := s.s3 == nil // local-only storage: stat decides missing

	if s.s3 != nil {
		objectURL := fmt.Sprintf("%s/%s/%s", s.s3.endpoint, bucket, path)
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, objectURL, nil)
		if err == nil {
			resp, err := s.s3.httpClient.Do(req)
			if err == nil {
				_ = resp.Body.Close()
				switch {
				case resp.StatusCode == http.StatusOK && resp.ContentLength >= 0:
					return resp.ContentLength, nil
				case resp.StatusCode == http.StatusNotFound:
					notFound = true // may still exist via local write fallback
				default:
					upstreamErr = fmt.Errorf("s3 head returned %d", resp.StatusCode)
				}
			} else {
				upstreamErr = err
			}
		} else {
			upstreamErr = err
		}
	}

	// Always check local too — objects may have been written through the
	// local fallback even when S3 is configured.
	localPath, err := resolveMediaPath(bucket, path)
	if err != nil {
		if upstreamErr != nil {
			return 0, upstreamErr
		}
		return 0, err
	}

	fi, statErr := os.Stat(localPath)
	switch {
	case statErr == nil:
		return fi.Size(), nil
	case os.IsNotExist(statErr) && notFound:
		return 0, errObjectMissing
	case os.IsNotExist(statErr) && upstreamErr != nil:
		return 0, upstreamErr // indeterminate — storage unreachable
	default:
		return 0, statErr
	}
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
// Resolves through resolveMediaPath so the lookup matches the
// data/media/{bucket}/{storagePath} layout used by SeaweedFSUpload,
// storeLocalMediaFile and ServeMedia.
func openLocalFile(bucket, path string) (io.ReadCloser, error) {
	localPath, err := resolveMediaPath(bucket, path)
	if err != nil {
		return nil, err
	}

	return openFile(localPath)
}

// openFileFunc is a package-level function variable so tests can override it.
var openFile = defaultOpenFile

func defaultOpenFile(path string) (io.ReadCloser, error) {
	if err := validateMediaPath(path); err != nil {
		return nil, err
	}
	return os.Open(path) // #nosec G304 // path validated by validateMediaPath
}

// getMediaForScan retrieves a media item by ID for the background
// virus-scan worker.
func (s *Service) getMediaForScan(ctx context.Context, mediaID []byte) (ScanMediaRow, error) {
	return s.store.GetMediaForScan(ctx, mediaID)
}

// confirmMediaByID marks a media item as confirmed.
// Used by the background scan processor.
func (s *Service) confirmMediaByID(ctx context.Context, mediaID []byte) error {
	return s.store.ConfirmMediaByID(ctx, mediaID)
}

// rejectMediaByID quarantines a media item.
// This gives admins a chance to review false positives before removal.
func (s *Service) rejectMediaByID(ctx context.Context, mediaID []byte, virusName string) error {
	return s.store.QuarantineMedia(ctx, mediaID, virusName)
}

// ApproveMedia clears quarantine and confirms the media item.
func (s *Service) ApproveMedia(ctx context.Context, mediaID, reviewerID pgtype.UUID) error {
	return s.store.ApproveQuarantined(ctx, mediaID, reviewerID)
}

// RejectMedia soft-deletes a quarantined media item after admin review.
func (s *Service) RejectMedia(ctx context.Context, mediaID, reviewerID pgtype.UUID) error {
	return s.store.RejectQuarantined(ctx, mediaID, reviewerID)
}

// AdminUpdateStatus allows admin to change media status (approve/reject/quarantine) for any item.
func (s *Service) AdminUpdateStatus(ctx context.Context, mediaID, reviewerID pgtype.UUID, confirmed bool, quarantineReason *string) error {
	var reason string
	if quarantineReason != nil {
		reason = *quarantineReason
	}

	return s.store.AdminUpdateStatus(ctx, gen.AdminUpdateMediaStatusParams{
		ID: mediaID,

		Confirmed:        confirmed,
		QuarantineReason: reason,
		ReviewedBy:       reviewerID,
	})
}

// ListQuarantined returns all quarantined media items for a platform.
func (s *Service) ListQuarantined(ctx context.Context, limit, offset int32) ([]QuarantinedMedia, error) {
	return s.store.ListQuarantined(ctx, limit, offset)
}

// GetQuarantinedMedia returns a single quarantined media item by ID.
func (s *Service) GetQuarantinedMedia(ctx context.Context, mediaID pgtype.UUID) (QuarantinedMedia, error) {
	return s.store.GetQuarantinedMedia(ctx, mediaID)
}

// GetMedia returns a single media item by ID.
func (s *Service) GetMedia(ctx context.Context, mediaID pgtype.UUID) (gen.Medium, error) {
	return s.store.GetMediaByID(ctx, mediaID)
}

// UpdateMetadata updates alt_text, caption, dimensions, and user_group_id on a media item.
// Admin path — no ownership check (gated by media.edit_any).
func (s *Service) UpdateMetadata(ctx context.Context, mediaID pgtype.UUID, altText, caption *string, width, height *int32, duration *float64, userGroupID pgtype.UUID) error {
	var durationNum pgtype.Numeric
	if duration != nil {
		if err := durationNum.Scan(fmt.Sprintf("%f", *duration)); err != nil {
			return fmt.Errorf("invalid duration %v: %w", *duration, err)
		}
	}

	return s.store.UpdateMediaMetadata(ctx, gen.UpdateMediaMetadataParams{
		ID: mediaID,

		AltText:      altText,
		Caption:      caption,
		WidthPx:      width,
		HeightPx:     height,
		DurationSecs: durationNum,
		UserGroupID:  userGroupID,
	})
}

// UpdateMetadataOwn updates metadata scoped to the uploader (gated by media.edit_own).
// Returns ErrMediaNotFound if the media belongs to a different user.
func (s *Service) UpdateMetadataOwn(ctx context.Context, uploaderID, mediaID pgtype.UUID, altText, caption *string, width, height *int32, duration *float64, userGroupID pgtype.UUID) error {
	var durationNum pgtype.Numeric
	if duration != nil {
		if err := durationNum.Scan(fmt.Sprintf("%f", *duration)); err != nil {
			return fmt.Errorf("invalid duration %v: %w", *duration, err)
		}
	}

	return s.store.UpdateMediaMetadataOwn(ctx, gen.UpdateMediaMetadataOwnParams{
		ID: mediaID,

		UploaderUserID: uploaderID,
		AltText:        altText,
		Caption:        caption,
		WidthPx:        width,
		HeightPx:       height,
		DurationSecs:   durationNum,
		UserGroupID:    userGroupID,
	})
}

// DeleteMedia soft-deletes a media item (admin path — no ownership check).
func (s *Service) DeleteMedia(ctx context.Context, mediaID pgtype.UUID) error {
	return s.store.SoftDeleteMedia(ctx, mediaID)
}

// HardDeleteMedia permanently removes a media item that is already
// soft-deleted (admin force-delete path — gated by admin.hard_delete).
// The storage object is removed alongside the row — best-effort, so a
// storage failure still allows the row removal (the media:cleanup_deleted
// sweep would have orphaned objects otherwise).
// Returns errpkg.ErrNotFound when the media does not exist and
// errpkg.ErrNotDeleted when it is not soft-deleted.
func (s *Service) HardDeleteMedia(ctx context.Context, mediaID pgtype.UUID) error {
	if m, err := s.store.GetMediaByIDAny(ctx, mediaID); err == nil {
		_ = s.deleteObject(ctx, m.StorageBucket, m.StoragePath)
	}
	return s.store.HardDeleteMedia(ctx, mediaID)
}

// PurgeDeletedMedia hard-deletes soft-deleted media rows (and their storage
// objects) older than retentionDays — the media:cleanup_deleted worker
// sweep. The object is removed before the row so a storage failure leaves
// the row in place for the next sweep instead of orphaning the object.
// Returns the number of rows purged.
func (s *Service) PurgeDeletedMedia(ctx context.Context, retentionDays, limit int) (int, error) {
	rows, err := s.store.ListMediaDeletedForCleanup(ctx, gen.ListMediaDeletedForCleanupParams{
		RetentionDays: int32(retentionDays),
		RowLimit:      int32(limit),
	})
	if err != nil {
		return 0, err
	}

	purged := 0
	for _, row := range rows {
		if err := s.deleteObject(ctx, row.StorageBucket, row.StoragePath); err != nil {
			slog.Warn("media gc: object delete failed, keeping row for next sweep",
				"media_id", row.ID.String(), "error", err)
			continue
		}
		if err := s.store.HardDeleteMedia(ctx, row.ID); err != nil {
			slog.Warn("media gc: row delete failed", "media_id", row.ID.String(), "error", err)
			continue
		}
		purged++
	}

	return purged, nil
}

// DeleteMediaOwn soft-deletes a media item scoped to the uploader (gated by media.delete_own).
// Returns ErrMediaNotFound if the media belongs to a different user.
func (s *Service) DeleteMediaOwn(ctx context.Context, uploaderID, mediaID pgtype.UUID) error {
	return s.store.SoftDeleteMediaOwn(ctx, mediaID, uploaderID)
}

// ListMedia returns paginated media, optionally filtered by uploader, type,
// search query, status, and flagged state, along with the total count matching
// the filter. The total is used by callers to compute total_pages. Empty/zero
// filter values mean "no filter on this field".
func (s *Service) ListMedia(ctx context.Context, uploaderID *pgtype.UUID, mimeType, query, status, flagged *string, deleted string, limit, offset int32) ([]gen.Medium, int32, error) {
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

	items, err := s.store.ListMediaFiltered(ctx, uploader, mimePattern, searchQuery, statusFilter, flaggedFilter, deleted, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	total, err := s.store.CountMediaFiltered(ctx, uploader, mimePattern, searchQuery, statusFilter, flaggedFilter, deleted)
	if err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

// ListByUser returns the media on the user's published public posts (featured/
// header images), plus the total count. Public-safe: excludes unconfirmed,
// group-restricted, and draft/private-post media. Powers the profile Media tab.
func (s *Service) ListByUser(ctx context.Context, userID pgtype.UUID, limit, offset int32) ([]gen.Medium, int32, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	if offset < 0 {
		offset = 0
	}

	items, err := s.store.ListMediaFromPublicPostsByUser(ctx, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing media from public posts: %w", err)
	}

	total, err := s.store.CountMediaFromPublicPostsByUser(ctx, userID)
	if err != nil {
		return nil, 0, fmt.Errorf("counting media from public posts: %w", err)
	}

	return items, total, nil
}

// ListByOrganization returns media from the org's public posts, with a total
// count. Powers the org profile Media tab.
func (s *Service) ListByOrganization(ctx context.Context, orgID pgtype.UUID, limit, offset int32) ([]gen.Medium, int32, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	if offset < 0 {
		offset = 0
	}

	items, err := s.store.ListMediaFromOrgPublicPosts(ctx, orgID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing media from org public posts: %w", err)
	}

	total, err := s.store.CountMediaFromOrgPublicPosts(ctx, orgID)
	if err != nil {
		return nil, 0, fmt.Errorf("counting media from org public posts: %w", err)
	}

	return items, total, nil
}

// GetMediaURL generates a displayable URL for the media item.
// Images get an imgproxy signed URL (NEVER cached in DB — generated on
// every serialization); binaries imgproxy cannot transform — videos,
// audio, PDFs — get the same-origin ServeURL instead so the returned URL
// always resolves.
func (s *Service) GetMediaURL(m *gen.Medium, opts ImageProxyOpts) string {
	if m == nil {
		return ""
	}

	if !isProxyableImageMIME(m.MimeType) {
		return ServeURL(m.StorageBucket, m.StoragePath)
	}

	return MediaURL(m.StorageBucket, m.StoragePath, opts)
}

// BulkDelete soft-deletes multiple media items (admin — no ownership check).
func (s *Service) BulkDelete(ctx context.Context, mediaIDs []pgtype.UUID) error {
	if len(mediaIDs) == 0 {
		return nil
	}

	return s.store.BulkSoftDeleteMedia(ctx, mediaIDs)
}

// BulkDeleteOwn soft-deletes multiple media items scoped to the uploader.
func (s *Service) BulkDeleteOwn(ctx context.Context, uploaderID pgtype.UUID, mediaIDs []pgtype.UUID) error {
	if len(mediaIDs) == 0 {
		return nil
	}

	return s.store.BulkSoftDeleteMediaOwn(ctx, uploaderID, mediaIDs)
}

// IsAllowedMIMEType checks if a MIME type is in the allowed set.
func IsAllowedMIMEType(mimeType string) bool {
	return AllowedMIMETypes[mimeType]
}

// isAllowedMIME checks if a MIME type is allowed for upload under the given
// upload context, considering system_settings 'media.allowed_video_types'
// for video types (Rule 3). Unknown contexts reject everything.
func (s *Service) isAllowedMIME(ctx context.Context, mimeType string, uploadCtx UploadContext) bool {
	canonical := CanonicalMIMEType(mimeType)

	allowed := AllowedMIMETypesByContext[uploadCtx]
	if allowed == nil {
		return false
	}

	if !ValidateFileType(canonical, allowed) {
		return false
	}

	// Video types additionally check media.allowed_video_types from
	// system_settings. Default is "mp4,webm,mov" per the
	// system-settings-catalog.
	if strings.HasPrefix(canonical, "video/") && s.settings != nil {
		allowedCSV := s.settings.GetStringOr(ctx, "media.allowed_video_types", "mp4,webm,mov")
		if !isVideoTypeAllowed(canonical, allowedCSV) {
			return false
		}
	}

	// Per-context extension allowlist (Rule 3): media uploads consult
	// content.allowed_upload_types, avatar uploads consult
	// users.allowed_avatar_types. Unset/empty means no admin restriction —
	// the base context allowlist alone applies, so behaviour is unchanged
	// until the setting is configured.
	if s.settings != nil {
		if key := contextUploadTypeSetting[uploadCtx]; key != "" {
			if csv := s.settings.GetStringOr(ctx, key, ""); strings.TrimSpace(csv) != "" && !isExtAllowed(canonical, csv) {
				return false
			}
		}
	}

	return true
}

// isVideoTypeAllowed checks if a canonical video MIME type is in the
// comma-separated list of allowed video extensions from system_settings.
// The setting stores extensions (mp4,webm,mov); this maps them to MIME types.
func isVideoTypeAllowed(canonicalMIME, allowedCSV string) bool {
	extToMIME := map[string]string{
		"mp4":  "video/mp4",
		"webm": "video/webm",
		"mov":  "video/quicktime",
		"avi":  "video/avi",
		"mkv":  "video/x-matroska",
	}

	for _, ext := range strings.Split(allowedCSV, ",") {
		ext = strings.TrimSpace(strings.ToLower(ext))
		if mime, ok := extToMIME[ext]; ok && mime == canonicalMIME {
			return true
		}
	}

	return false
}

// NormalizeMIMEType normalizes a MIME type to a canonical form for comparison.
func NormalizeMIMEType(mimeType string) string {
	return strings.ToLower(strings.TrimSpace(mimeType))
}

// DirectUpload stores a file locally and creates the media record.
func (s *Service) DirectUpload(ctx context.Context, uploaderID pgtype.UUID, filename, mimeType string, fileSize int64, localPath, storagePath string) (pgtype.UUID, error) {
	m, err := s.store.InsertMedia(ctx, gen.InsertMediaParams{

		UploaderUserID:   uploaderID,
		OriginalFilename: filename,
		MimeType:         mimeType,
		FileSizeBytes:    fileSize,
		StoragePath:      storagePath,
		StorageBucket:    s.storageBucket(ctx),
	})
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("inserting media record: %w", err)
	}

	if err := s.store.ConfirmMedia(ctx, m.ID); err != nil {
		return pgtype.UUID{}, fmt.Errorf("confirming media: %w", err)
	}

	return m.ID, nil
}

// StoreFromBytes stores raw bytes as a confirmed media item. When attribution
// is non-nil, the source_* columns are populated for stock photo attribution.
func (s *Service) StoreFromBytes(ctx context.Context, uploaderID pgtype.UUID, data []byte, filename string, attribution *MediaAttribution) (pgtype.UUID, error) {
	if len(data) == 0 {
		return pgtype.UUID{}, errors.New("data is empty")
	}
	// Max upload size from system_settings 'media.max_upload_mb' (Rule 3).
	maxBytes := s.MaxUploadBytes(ctx)
	if int64(len(data)) > maxBytes {
		return pgtype.UUID{}, fmt.Errorf("data too large: %d bytes exceeds limit of %d bytes", len(data), maxBytes)
	}

	// Sniff MIME type from content (never the client-supplied header).
	mimeType := DetectStorageMIME(data, filename)

	// Ingestion gate: only media-family types may become media rows. Remote
	// fetches that return HTML error pages, archives, or executables are
	// rejected rather than stored as undetectable .bin blobs.
	if !isStorableMediaMIME(mimeType) {
		return pgtype.UUID{}, fmt.Errorf("unsupported content type %q for media storage", mimeType)
	}

	// Compress oversized raster images before storage — curated/ingested
	// originals (Wikidata logos, stock photos, video thumbnails) routinely
	// arrive at multi-megabyte sizes. GIF animation and SVG vectors pass
	// through byte-identical.
	data, mimeType = optimizeImage(data, mimeType)

	// Generate unique storage path.
	ext := extensionFromMIME(mimeType)
	if ext == ".bin" {
		// Keep the original extension when it is a known-safe media
		// container — a .bin path makes ServeMedia return
		// application/octet-stream and browsers download instead of
		// playing.
		if fe := strings.ToLower(filepath.Ext(filename)); isSafeMediaExt(fe) {
			ext = fe
		}
	}
	storagePath := fmt.Sprintf("%s/%s%s", StoragePathMedia,

		uuid.New().String(),
		ext,
	)

	bucket := s.storageBucket(ctx)

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
		if err := storeLocalMediaFile(bucket, storagePath, data); err != nil {
			return pgtype.UUID{}, fmt.Errorf("storing to local fallback: %w", err)
		}
	}

	// Build insert params with optional attribution.
	params := gen.InsertMediaParams{

		UploaderUserID:   uploaderID,
		OriginalFilename: filename,
		MimeType:         mimeType,
		FileSizeBytes:    int64(len(data)),
		StoragePath:      storagePath,
		StorageBucket:    bucket,
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

	if err := s.store.ConfirmMedia(ctx, m.ID); err != nil {
		return pgtype.UUID{}, fmt.Errorf("confirming media: %w", err)
	}

	// Enqueue a virus scan — the same content:scan_media task user uploads
	// get via enqueueMediaTasks. Confirmation is not deferred on it: the
	// scan runs post-confirmation exactly like user uploads, and can still
	// quarantine the row (QuarantineMediaByID has no confirmed filter).
	s.enqueueScanTask(m.ID)

	return m.ID, nil
}

// StoreFromURL downloads content from a remote URL, sniffs the MIME type
// from the content (not the extension), uploads it to SeaweedFS, and creates
// a media record. Returns the media ID. Used by server-side ingestion tasks
// like university:fetch_logo (Section 39).
//
// The uploaderID is the system/admin user ID to attribute the media to.
func (s *Service) StoreFromURL(ctx context.Context, uploaderID pgtype.UUID, sourceURL, filename string) (pgtype.UUID, error) {
	// SSRF guard — same rules as ProxyRemote: http(s) only, no userinfo,
	// and every dial validates resolved IPs as public. Ingestion callers
	// pass URLs from external metadata (Wikidata, stock APIs), so a
	// malicious or compromised source must not reach internal hosts or
	// cloud metadata endpoints. Redirect hops are validated at connect
	// time by the guarded DialContext.
	u, err := url.Parse(sourceURL)
	if err != nil || u == nil || u.Host == "" || u.User != nil ||
		(u.Scheme != "https" && u.Scheme != "http") {
		return pgtype.UUID{}, fmt.Errorf("invalid source url")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("creating download request: %w", err)
	}

	req.Header.Set("User-Agent", "JapaConnect/1.0 (university:fetch_logo)")

	// Share the proxy's SSRF-safe transport; keep the larger download
	// timeout this path needs.
	client := &http.Client{Timeout: 30 * time.Second, Transport: proxyHTTPClient.Transport}

	resp, err := client.Do(req)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("downloading media from %s: %w", sourceURL, err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return pgtype.UUID{}, fmt.Errorf("download returned %d for %s", resp.StatusCode, sourceURL)
	}

	body, err := readLimited(resp.Body, MaxFileSizeBytes)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("reading body from %s: %w", sourceURL, err)
	}

	if len(body) == 0 {
		return pgtype.UUID{}, fmt.Errorf("body is empty from %s", sourceURL)
	}

	return s.StoreFromBytes(ctx, uploaderID, body, filename, nil)
}

// DetectStorageMIME determines the content type of raw bytes by content
// inspection (http.DetectContentType — never a client-supplied header),
// falling back to the filename extension when the sniffer returns
// application/octet-stream for container formats it cannot fingerprint
// (mkv, avi, docx…). Shared by StoreFromBytes and callers that need to
// validate a payload before storing it.
func DetectStorageMIME(data []byte, filename string) string {
	mimeType := http.DetectContentType(data)
	if mimeType == "application/octet-stream" {
		// ISO-BMFF container: the ftyp major brand identifies the real
		// type — isom MP4s, m4a audio, HEIC/AVIF stills — that the sniffer
		// cannot fingerprint.
		if brand := isoBMFFBrandMIME(data); brand != "" {
			return brand
		}
	}
	if mimeType == "application/octet-stream" ||
		strings.HasPrefix(mimeType, "text/plain") ||
		strings.HasPrefix(mimeType, "text/xml") {
		// octet-stream: container formats (mkv, avi, docx…) the sniffer
		// cannot fingerprint. text/plain|text/xml: magic-byte-free formats
		// — notably SVG, which sniffs as generic text but is a valid
		// image type when the filename says .svg. Callers re-validate the
		// resolved type, so a misleading extension cannot widen what is
		// accepted. ExtensionToMIME (project-controlled) is consulted
		// before mime.TypeByExtension, which depends on the host's
		// /etc/mime.types and may be empty on minimal images.
		ext := strings.ToLower(filepath.Ext(filename))
		if fm := ExtensionToMIME[ext]; fm != "" {
			mimeType = fm
		} else if fm := mime.TypeByExtension(ext); fm != "" {
			mimeType = fm
		}
	}

	return mimeType
}

// isStorableMediaMIME reports whether a sniffed MIME type belongs to the
// media family (image/*, video/*, audio/*, or application/pdf). Server-side
// ingestion (StoreFromBytes/StoreFromURL — curated logos, thumbnails, stock
// photos, imported body images) uses this to reject remote content that is
// not media at all, such as HTML error pages or executables. Deliberately
// broader than the upload-context allowlists: admin upload-type restrictions
// govern user uploads, not system ingestion.
func isStorableMediaMIME(mimeType string) bool {
	c := CanonicalMIMEType(mimeType)
	return strings.HasPrefix(c, "image/") ||
		strings.HasPrefix(c, "video/") ||
		strings.HasPrefix(c, "audio/") ||
		c == "application/pdf"
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

	// #nosec G301 // media directories are created inside the configured storage root.
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return fmt.Errorf("creating media directory: %w", err)
	}

	// #nosec G306 // media files are meant to be world-readable via ServeMedia.
	if err := os.WriteFile(localPath, body, 0o644); err != nil {
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
	case strings.HasPrefix(mimeType, "image/avif"):
		return ".avif"
	case strings.HasPrefix(mimeType, "image/bmp"):
		return ".bmp"
	case strings.HasPrefix(mimeType, "image/tiff"):
		return ".tiff"
	// Video/audio — without these every upload lands as ".bin" and
	// ServeMedia returns application/octet-stream, so browsers download
	// the file instead of playing it.
	case strings.HasPrefix(mimeType, "video/mp4"):
		return ".mp4"
	case strings.HasPrefix(mimeType, "video/webm"):
		return ".webm"
	case strings.HasPrefix(mimeType, "video/quicktime"):
		return ".mov"
	case strings.HasPrefix(mimeType, "video/x-msvideo"):
		return ".avi"
	case strings.HasPrefix(mimeType, "video/x-matroska"):
		return ".mkv"
	case strings.HasPrefix(mimeType, "video/3gpp"):
		return ".3gp"
	case strings.HasPrefix(mimeType, "audio/mpeg"):
		return ".mp3"
	case strings.HasPrefix(mimeType, "audio/mp4"), strings.HasPrefix(mimeType, "audio/x-m4a"):
		return ".m4a"
	case strings.HasPrefix(mimeType, "audio/ogg"):
		return ".ogg"
	case strings.HasPrefix(mimeType, "audio/wav"), strings.HasPrefix(mimeType, "audio/x-wav"):
		return ".wav"
	case strings.HasPrefix(mimeType, "application/pdf"):
		return ".pdf"
	default:
		return ".bin"
	}
}

// safeMediaExts are filename extensions allowed to override a ".bin" result
// from extensionFromMIME. Only inert media/document containers are listed —
// extensions that map to active content (html, svg, js…) are intentionally
// absent so they can never be served under a runnable type.
var safeMediaExts = map[string]bool{
	".mp4": true, ".m4v": true, ".mov": true, ".webm": true,
	".mkv": true, ".avi": true, ".wmv": true, ".flv": true,
	".ts": true, ".3gp": true, ".mpg": true, ".mpeg": true,
	".mp3": true, ".m4a": true, ".wav": true, ".ogg": true,
	".oga": true, ".opus": true, ".flac": true, ".aac": true,
	".pdf": true, ".txt": true, ".md": true, ".csv": true,
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true,
	".gif": true, ".avif": true,
}

func isSafeMediaExt(ext string) bool {
	return safeMediaExts[ext]
}
