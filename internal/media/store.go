package media

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store provides low-level database access for media.
type Store struct {
	write     *gen.Queries
	read      *gen.Queries
	writePool *pgxpool.Pool
	readPool  *pgxpool.Pool
}

// NewStore creates a Store backed by the given gen.Queries and pool instances.
func NewStore(read, write *gen.Queries, readPool, writePool *pgxpool.Pool) *Store {
	return &Store{read: read, write: write, readPool: readPool, writePool: writePool}
}

// InsertMedia creates a new media row and returns it.
func (s *Store) InsertMedia(ctx context.Context, params gen.InsertMediaParams) (gen.Medium, error) {
	m, err := s.write.InsertMedia(ctx, params)
	if err != nil {
		return gen.Medium{}, fmt.Errorf("inserting media: %w", err)
	}

	return m, nil
}

// GetMediaByID returns a media item by ID (excludes soft-deleted).
func (s *Store) GetMediaByID(ctx context.Context, id, tenantID pgtype.UUID) (gen.Medium, error) {
	m, err := s.read.GetMediaByID(ctx,
		id,
	)
	if err != nil {
		return gen.Medium{}, fmt.Errorf("getting media by ID: %w", err)
	}

	return m, nil
}

// GetMediaByStoragePath returns a media item by its storage path.
func (s *Store) GetMediaByStoragePath(ctx context.Context, storagePath string, tenantID pgtype.UUID) (gen.Medium, error) {
	m, err := s.read.GetMediaByStoragePath(ctx,
		storagePath,
	)
	if err != nil {
		return gen.Medium{}, fmt.Errorf("getting media by storage path: %w", err)
	}

	return m, nil
}

// ConfirmMedia marks a media item as confirmed.
func (s *Store) ConfirmMedia(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.ConfirmMedia(ctx,
		id,
	); err != nil {
		return fmt.Errorf("confirming media: %w", err)
	}

	return nil
}

// UpdateMediaMetadata updates metadata fields on a media item (admin — no ownership check).
func (s *Store) UpdateMediaMetadata(ctx context.Context, params gen.UpdateMediaMetadataParams) error {
	if err := s.write.UpdateMediaMetadata(ctx, params); err != nil {
		return fmt.Errorf("updating media metadata: %w", err)
	}

	return nil
}

// UpdateMediaMetadataOwn updates metadata fields scoped to the uploader.
func (s *Store) UpdateMediaMetadataOwn(ctx context.Context, params gen.UpdateMediaMetadataOwnParams) error {
	if err := s.write.UpdateMediaMetadataOwn(ctx, params); err != nil {
		return fmt.Errorf("updating own media metadata: %w", err)
	}

	return nil
}

// SoftDeleteMedia soft-deletes a media item (admin — no ownership check).
func (s *Store) SoftDeleteMedia(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.SoftDeleteMedia(ctx,
		id,
	); err != nil {
		return fmt.Errorf("soft-deleting media: %w", err)
	}

	return nil
}

// SoftDeleteMediaOwn soft-deletes a media item scoped to the uploader.
// Returns ErrMediaNotFound if the media belongs to a different user or
// does not exist, so the caller can return a 404 without leaking existence.
func (s *Store) SoftDeleteMediaOwn(ctx context.Context, id, tenantID, uploaderID pgtype.UUID) error {
	if err := s.write.SoftDeleteMediaOwn(ctx, gen.SoftDeleteMediaOwnParams{
		ID: id,

		UploaderUserID: uploaderID,
	}); err != nil {
		return fmt.Errorf("soft-deleting own media: %w", err)
	}

	return nil
}

// ListMediaByUploader returns paginated media for a specific uploader.
func (s *Store) ListMediaByUploader(ctx context.Context, tenantID, uploaderID pgtype.UUID, limit, offset int32) ([]gen.Medium, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	items, err := s.read.ListMediaByUploader(ctx, gen.ListMediaByUploaderParams{

		UploaderUserID: uploaderID,
		PageOffset:     offset,
		PageLimit:      limit,
	})
	if err != nil {
		return nil, fmt.Errorf("listing media by uploader: %w", err)
	}

	return items, nil
}

// ListMediaByTenant returns all non-deleted media for a tenant.
func (s *Store) ListMediaByTenant(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]gen.Medium, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	items, err := s.read.ListMediaByTenant(ctx, gen.ListMediaByTenantParams{

		PageOffset: offset,
		PageLimit:  limit,
	})
	if err != nil {
		return nil, fmt.Errorf("listing media by tenant: %w", err)
	}

	return items, nil
}

// ListMediaByType returns paginated media matching a MIME type prefix.
func (s *Store) ListMediaByType(ctx context.Context, tenantID pgtype.UUID, mimePattern string, limit, offset int32) ([]gen.Medium, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	items, err := s.read.ListMediaByType(ctx, gen.ListMediaByTypeParams{

		MimePattern: &mimePattern,
		PageOffset:  offset,
		PageLimit:   limit,
	})
	if err != nil {
		return nil, fmt.Errorf("listing media by type: %w", err)
	}

	return items, nil
}

// SearchMedia searches media by filename, alt_text, and caption.
func (s *Store) SearchMedia(ctx context.Context, tenantID pgtype.UUID, query string, limit, offset int32) ([]gen.Medium, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	items, err := s.read.SearchMedia(ctx, gen.SearchMediaParams{

		Query:      &query,
		PageOffset: offset,
		PageLimit:  limit,
	})
	if err != nil {
		return nil, fmt.Errorf("searching media: %w", err)
	}

	return items, nil
}

// CountMediaByUploader returns the count of non-deleted media for an uploader.
func (s *Store) CountMediaByUploader(ctx context.Context, tenantID, uploaderID pgtype.UUID) (int32, error) {
	count, err := s.read.CountMediaByUploader(ctx,

		uploaderID,
	)
	if err != nil {
		return 0, fmt.Errorf("counting media by uploader: %w", err)
	}

	return count, nil
}

// CountMediaByTenant returns the count of all non-deleted media for a tenant.
func (s *Store) CountMediaByTenant(ctx context.Context, tenantID pgtype.UUID) (int32, error) {
	count, err := s.read.CountMediaByTenant(ctx)
	if err != nil {
		return 0, fmt.Errorf("counting media by tenant: %w", err)
	}

	return count, nil
}

// CountMediaByType returns the count of non-deleted media matching a MIME type prefix.
func (s *Store) CountMediaByType(ctx context.Context, tenantID pgtype.UUID, mimePattern string) (int32, error) {
	count, err := s.read.CountMediaByType(ctx,

		&mimePattern,
	)
	if err != nil {
		return 0, fmt.Errorf("counting media by type: %w", err)
	}

	return count, nil
}

// CountSearchMedia returns the count of non-deleted media matching a search query.
func (s *Store) CountSearchMedia(ctx context.Context, tenantID pgtype.UUID, query string) (int32, error) {
	count, err := s.read.CountSearchMedia(ctx,

		&query,
	)
	if err != nil {
		return 0, fmt.Errorf("counting search media: %w", err)
	}

	return count, nil
}

// ListMediaFiltered returns paginated media matching the given filters
// (uploader, mime type, search query, status, flagged). Empty/zero values mean
// "no filter on this field". Used by the admin media list endpoint.
func (s *Store) ListMediaFiltered(ctx context.Context, tenantID, uploaderID pgtype.UUID, mimePattern, query, status, flagged string, limit, offset int32) ([]gen.Medium, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	items, err := s.read.ListMediaFiltered(ctx, gen.ListMediaFilteredParams{

		UploaderUserID: uploaderID,
		MimePattern:    mimePattern,
		Query:          query,
		Status:         status,
		Flagged:        flagged,
		PageOffset:     offset,
		PageLimit:      limit,
	})
	if err != nil {
		return nil, fmt.Errorf("listing filtered media: %w", err)
	}

	return items, nil
}

// CountMediaFiltered returns the count of media matching the given filters,
// ignoring limit/offset. Used to compute total_pages for the admin list.
func (s *Store) CountMediaFiltered(ctx context.Context, tenantID, uploaderID pgtype.UUID, mimePattern, query, status, flagged string) (int32, error) {
	count, err := s.read.CountMediaFiltered(ctx, gen.CountMediaFilteredParams{

		UploaderUserID: uploaderID,
		MimePattern:    mimePattern,
		Query:          query,
		Status:         status,
		Flagged:        flagged,
	})
	if err != nil {
		return 0, fmt.Errorf("counting filtered media: %w", err)
	}

	return count, nil
}

// ListMediaFromPublicPostsByUser returns paginated media that is the
// featured/header image of the user's published, public posts. Public-safe:
// excludes unconfirmed media, group-restricted media, and draft/private-post
// media. Powers the public profile Media tab.
func (s *Store) ListMediaFromPublicPostsByUser(ctx context.Context, tenantID, authorID pgtype.UUID, limit, offset int32) ([]gen.Medium, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	items, err := s.read.ListMediaFromPublicPostsByUser(ctx, gen.ListMediaFromPublicPostsByUserParams{

		AuthorID:   authorID,
		PageOffset: offset,
		PageLimit:  limit,
	})
	if err != nil {
		return nil, fmt.Errorf("listing media from public posts by user: %w", err)
	}

	return items, nil
}

// CountMediaFromPublicPostsByUser returns the count of media that is the
// featured/header image of the user's published, public posts.
func (s *Store) CountMediaFromPublicPostsByUser(ctx context.Context, tenantID, authorID pgtype.UUID) (int32, error) {
	count, err := s.read.CountMediaFromPublicPostsByUser(ctx,

		authorID,
	)
	if err != nil {
		return 0, fmt.Errorf("counting media from public posts by user: %w", err)
	}

	return count, nil
}

// ListMediaFromOrgPublicPosts returns media used as featured/header images by
// the org's published-public posts, paginated. Powers the org Media tab.
func (s *Store) ListMediaFromOrgPublicPosts(ctx context.Context, tenantID, orgID pgtype.UUID, limit, offset int32) ([]gen.Medium, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	items, err := s.read.ListMediaFromOrgPublicPosts(ctx, gen.ListMediaFromOrgPublicPostsParams{

		OrganizationID: orgID,
		PageOffset:     offset,
		PageLimit:      limit,
	})
	if err != nil {
		return nil, fmt.Errorf("listing media from org public posts: %w", err)
	}

	return items, nil
}

// CountMediaFromOrgPublicPosts returns the count of media that is the
// featured/header image of the org's published, public posts.
func (s *Store) CountMediaFromOrgPublicPosts(ctx context.Context, tenantID, orgID pgtype.UUID) (int32, error) {
	count, err := s.read.CountMediaFromOrgPublicPosts(ctx,

		orgID,
	)
	if err != nil {
		return 0, fmt.Errorf("counting media from org public posts: %w", err)
	}

	return count, nil
}

// BulkSoftDeleteMedia soft-deletes multiple media items (admin — no ownership check).
func (s *Store) BulkSoftDeleteMedia(ctx context.Context, tenantID pgtype.UUID, mediaIDs []pgtype.UUID) error {
	if len(mediaIDs) == 0 {
		return nil
	}

	if err := s.write.BulkSoftDeleteMedia(ctx,

		mediaIDs,
	); err != nil {
		return fmt.Errorf("bulk soft-deleting media: %w", err)
	}

	return nil
}

// BulkSoftDeleteMediaOwn soft-deletes multiple media items scoped to the uploader.
func (s *Store) BulkSoftDeleteMediaOwn(ctx context.Context, tenantID, uploaderID pgtype.UUID, mediaIDs []pgtype.UUID) error {
	if len(mediaIDs) == 0 {
		return nil
	}

	if err := s.write.BulkSoftDeleteMediaOwn(ctx, gen.BulkSoftDeleteMediaOwnParams{

		UploaderUserID: uploaderID,
		MediaIds:       mediaIDs,
	}); err != nil {
		return fmt.Errorf("bulk soft-deleting own media: %w", err)
	}

	return nil
}

// QuarantinedMedia holds the full media row including quarantine fields.
type QuarantinedMedia struct {
	ID               pgtype.UUID `json:"id"`
	TenantID         pgtype.UUID
	UploaderUserID   pgtype.UUID        `json:"uploader_user_id"`
	OriginalFilename string             `json:"original_filename"`
	MimeType         string             `json:"mime_type"`
	FileSizeBytes    int64              `json:"file_size_bytes"`
	WidthPx          *int32             `json:"width_px,omitempty"`
	HeightPx         *int32             `json:"height_px,omitempty"`
	DurationSecs     pgtype.Numeric     `json:"duration_secs,omitempty"`
	StoragePath      string             `json:"storage_path"`
	StorageBucket    string             `json:"storage_bucket"`
	AltText          *string            `json:"alt_text,omitempty"`
	Caption          *string            `json:"caption,omitempty"`
	Confirmed        bool               `json:"confirmed"`
	DeletedAt        pgtype.Timestamptz `json:"deleted_at,omitempty"`
	QuarantineReason *string            `json:"quarantine_reason,omitempty"`
	QuarantinedAt    pgtype.Timestamptz `json:"quarantined_at,omitempty"`
	ReviewedBy       pgtype.UUID        `json:"reviewed_by,omitempty"`
	ReviewedAt       pgtype.Timestamptz `json:"reviewed_at,omitempty"`
	CreatedAt        pgtype.Timestamptz `json:"created_at"`
	UserGroupID      pgtype.UUID        `json:"user_group_id,omitempty"`
}

// ScanMediaRow holds the minimal fields needed for virus scanning.
type ScanMediaRow struct {
	StorageBucket    string
	StoragePath      string
	OriginalFilename string
	UploaderUserID   string
	TenantID         string
}

// UpdateMediaStoragePath updates the storage path for a media item.
// Used after video processing to point to the new file location.
func (s *Store) UpdateMediaStoragePath(ctx context.Context, id, tenantID pgtype.UUID, storagePath string) error {
	_, err := s.writePool.Exec(ctx, `
		UPDATE media SET storage_path = $1 WHERE id = $2 AND deleted_at IS NULL
	`, storagePath, id)
	if err != nil {
		return fmt.Errorf("updating media storage path: %w", err)
	}

	return nil
}

// GetMediaForScan retrieves a media item by ID with its .
// Used by background workers that operate across all tenants — the
// returned from this read is threaded into subsequent write operations
// (ConfirmMediaByID, QuarantineMedia) to prevent cross-tenant modification.
func (s *Store) GetMediaForScan(ctx context.Context, mediaID []byte) (ScanMediaRow, error) {
	var row ScanMediaRow

	err := s.readPool.QueryRow(ctx, `
		SELECT storage_bucket, storage_path, original_filename, uploader_user_id::text, tenant_id::text
		FROM media
		WHERE id = $1 AND deleted_at IS NULL
	`, mediaID).Scan(&row.StorageBucket, &row.StoragePath, &row.OriginalFilename, &row.UploaderUserID)
	if err != nil {
		return ScanMediaRow{}, fmt.Errorf("getting media for scan: %w", err)
	}

	return row, nil
}

// ConfirmMediaByID marks a media item as confirmed, scoped to tenant.
// Used by the background scan processor after a clean virus scan.
func (s *Store) ConfirmMediaByID(ctx context.Context, mediaID []byte, tenantID string) error {
	_, err := s.writePool.Exec(ctx, `
		UPDATE media SET confirmed = true WHERE id = $1 AND deleted_at IS NULL
	`, mediaID)
	if err != nil {
		return fmt.Errorf("confirming media by ID: %w", err)
	}

	return nil
}

// RejectMediaByID soft-deletes a media item and logs the virus name.
// Used by the background scan processor when ClamAV detects malware.
func (s *Store) RejectMediaByID(ctx context.Context, mediaID []byte, tenantID, virusName string) error {
	_, err := s.writePool.Exec(ctx, `
		UPDATE media
		SET deleted_at = now(),
		    alt_text = coalesce(alt_text, '') || ' [REJECTED: ' || $2 || ']'
		WHERE id = $1 AND deleted_at IS NULL
	`, mediaID, virusName)
	if err != nil {
		return fmt.Errorf("rejecting media by ID: %w", err)
	}

	return nil
}

// QuarantineMedia marks a media item as quarantined, scoped to tenant.
// This allows admins to review false positives before permanent removal.
func (s *Store) QuarantineMedia(ctx context.Context, mediaID []byte, tenantID, reason string) error {
	_, err := s.writePool.Exec(ctx, `
		UPDATE media
		SET quarantine_reason = $2,
		    quarantined_at = now()
		WHERE id = $1 AND deleted_at IS NULL
	`, mediaID, reason)
	if err != nil {
		return fmt.Errorf("quarantining media by ID: %w", err)
	}

	return nil
}

// ApproveQuarantined clears quarantine fields and confirms the media item.
func (s *Store) ApproveQuarantined(ctx context.Context, mediaID, tenantID, reviewerID pgtype.UUID) error {
	if err := s.write.ApproveQuarantinedMedia(ctx, gen.ApproveQuarantinedMediaParams{
		ReviewedBy: reviewerID,
		ID:         mediaID,
	}); err != nil {
		return fmt.Errorf("approving quarantined media: %w", err)
	}

	return nil
}

// RejectQuarantined soft-deletes a quarantined media item after admin review.
func (s *Store) RejectQuarantined(ctx context.Context, mediaID, tenantID, reviewerID pgtype.UUID) error {
	if err := s.write.RejectQuarantinedMedia(ctx, gen.RejectQuarantinedMediaParams{
		ReviewedBy: reviewerID,
		ID:         mediaID,
	}); err != nil {
		return fmt.Errorf("rejecting quarantined media: %w", err)
	}

	return nil
}

// AdminUpdateStatus allows admin to change media status for any item.
func (s *Store) AdminUpdateStatus(ctx context.Context, params gen.AdminUpdateMediaStatusParams) error {
	if err := s.write.AdminUpdateMediaStatus(ctx, params); err != nil {
		return fmt.Errorf("updating media status: %w", err)
	}

	return nil
}

// ListQuarantined returns all quarantined media items for a tenant.
func (s *Store) ListQuarantined(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]QuarantinedMedia, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListQuarantinedMedia(ctx, gen.ListQuarantinedMediaParams{

		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing quarantined media: %w", err)
	}

	items := make([]QuarantinedMedia, len(rows))
	for i, r := range rows {
		items[i] = QuarantinedMedia{
			ID: r.ID,

			UploaderUserID:   r.UploaderUserID,
			OriginalFilename: r.OriginalFilename,
			MimeType:         r.MimeType,
			FileSizeBytes:    r.FileSizeBytes,
			WidthPx:          r.WidthPx,
			HeightPx:         r.HeightPx,
			DurationSecs:     r.DurationSecs,
			StoragePath:      r.StoragePath,
			StorageBucket:    r.StorageBucket,
			AltText:          r.AltText,
			Caption:          r.Caption,
			Confirmed:        r.Confirmed,
			DeletedAt:        r.DeletedAt,
			QuarantineReason: r.QuarantineReason,
			QuarantinedAt:    r.QuarantinedAt,
			ReviewedBy:       r.ReviewedBy,
			ReviewedAt:       r.ReviewedAt,
			CreatedAt:        r.CreatedAt,
		}
	}

	return items, nil
}

// GetQuarantinedMedia returns a single quarantined media item by ID.
func (s *Store) GetQuarantinedMedia(ctx context.Context, mediaID, tenantID pgtype.UUID) (QuarantinedMedia, error) {
	var m QuarantinedMedia

	err := s.readPool.QueryRow(ctx, `
		SELECT id, uploader_user_id, original_filename, mime_type,
		       file_size_bytes, width_px, height_px, duration_secs, storage_path,
		       storage_bucket, alt_text, caption, confirmed, deleted_at,
		       quarantine_reason, quarantined_at, reviewed_by, reviewed_at,
		       created_at
		FROM media
		WHERE id = $1
		  AND quarantine_reason IS NOT NULL
		  AND deleted_at IS NULL
	`, mediaID).Scan(
		&m.ID, &m.UploaderUserID, &m.OriginalFilename, &m.MimeType,
		&m.FileSizeBytes, &m.WidthPx, &m.HeightPx, &m.DurationSecs, &m.StoragePath,
		&m.StorageBucket, &m.AltText, &m.Caption, &m.Confirmed, &m.DeletedAt,
		&m.QuarantineReason, &m.QuarantinedAt, &m.ReviewedBy, &m.ReviewedAt,
		&m.CreatedAt,
	)
	if err != nil {
		return QuarantinedMedia{}, fmt.Errorf("getting quarantined media: %w", err)
	}

	return m, nil
}
