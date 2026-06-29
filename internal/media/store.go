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
	write      *gen.Queries
	read       *gen.Queries
	writePool  *pgxpool.Pool
	readPool   *pgxpool.Pool
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
	m, err := s.read.GetMediaByID(ctx, gen.GetMediaByIDParams{
		ID:       id,
		TenantID: tenantID,
	})
	if err != nil {
		return gen.Medium{}, fmt.Errorf("getting media by ID: %w", err)
	}
	return m, nil
}

// GetMediaByStoragePath returns a media item by its storage path.
func (s *Store) GetMediaByStoragePath(ctx context.Context, storagePath string, tenantID pgtype.UUID) (gen.Medium, error) {
	m, err := s.read.GetMediaByStoragePath(ctx, gen.GetMediaByStoragePathParams{
		StoragePath: storagePath,
		TenantID:    tenantID,
	})
	if err != nil {
		return gen.Medium{}, fmt.Errorf("getting media by storage path: %w", err)
	}
	return m, nil
}

// ConfirmMedia marks a media item as confirmed.
func (s *Store) ConfirmMedia(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.ConfirmMedia(ctx, gen.ConfirmMediaParams{
		ID:       id,
		TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("confirming media: %w", err)
	}
	return nil
}

// UpdateMediaMetadata updates metadata fields on a media item.
func (s *Store) UpdateMediaMetadata(ctx context.Context, params gen.UpdateMediaMetadataParams) error {
	if err := s.write.UpdateMediaMetadata(ctx, params); err != nil {
		return fmt.Errorf("updating media metadata: %w", err)
	}
	return nil
}

// SoftDeleteMedia soft-deletes a media item.
func (s *Store) SoftDeleteMedia(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.SoftDeleteMedia(ctx, gen.SoftDeleteMediaParams{
		ID:       id,
		TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("soft-deleting media: %w", err)
	}
	return nil
}

// ListMediaByUploader returns paginated media for a specific uploader.
func (s *Store) ListMediaByUploader(ctx context.Context, tenantID, uploaderID pgtype.UUID, limit, offset int32) ([]gen.Medium, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}
	items, err := s.read.ListMediaByUploader(ctx, gen.ListMediaByUploaderParams{
		TenantID:       tenantID,
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
		TenantID:   tenantID,
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
		TenantID:    tenantID,
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
		TenantID:   tenantID,
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
	count, err := s.read.CountMediaByUploader(ctx, gen.CountMediaByUploaderParams{
		TenantID:       tenantID,
		UploaderUserID: uploaderID,
	})
	if err != nil {
		return 0, fmt.Errorf("counting media by uploader: %w", err)
	}
	return count, nil
}

// BulkSoftDeleteMedia soft-deletes multiple media items.
func (s *Store) BulkSoftDeleteMedia(ctx context.Context, tenantID pgtype.UUID, mediaIDs []pgtype.UUID) error {
	if len(mediaIDs) == 0 {
		return nil
	}
	if err := s.write.BulkSoftDeleteMedia(ctx, gen.BulkSoftDeleteMediaParams{
		TenantID: tenantID,
		MediaIds: mediaIDs,
	}); err != nil {
		return fmt.Errorf("bulk soft-deleting media: %w", err)
	}
	return nil
}

// scanMediaRow holds the minimal fields needed for virus scanning.
type scanMediaRow struct {
	StorageBucket    string
	StoragePath      string
	OriginalFilename string
	UploaderUserID   string
}

// UpdateMediaStoragePath updates the storage path for a media item.
// Used after video processing to point to the new file location.
func (s *Store) UpdateMediaStoragePath(ctx context.Context, id, tenantID pgtype.UUID, storagePath string) error {
	_, err := s.writePool.Exec(ctx, `
		UPDATE media SET storage_path = $1 WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL
	`, storagePath, id, tenantID)
	if err != nil {
		return fmt.Errorf("updating media storage path: %w", err)
	}
	return nil
}

// GetMediaForScan retrieves a media item by ID without tenant filtering.
// Used by background workers that operate across all tenants.
func (s *Store) GetMediaForScan(ctx context.Context, mediaID []byte) (scanMediaRow, error) {
	var row scanMediaRow
	err := s.readPool.QueryRow(ctx, `
		SELECT storage_bucket, storage_path, original_filename, uploader_user_id::text
		FROM media
		WHERE id = $1 AND deleted_at IS NULL
	`, mediaID).Scan(&row.StorageBucket, &row.StoragePath, &row.OriginalFilename, &row.UploaderUserID)
	if err != nil {
		return scanMediaRow{}, fmt.Errorf("getting media for scan: %w", err)
	}
	return row, nil
}

// ConfirmMediaByID marks a media item as confirmed by ID alone (no tenant).
// Used by the background scan processor after a clean virus scan.
func (s *Store) ConfirmMediaByID(ctx context.Context, mediaID []byte) error {
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
func (s *Store) RejectMediaByID(ctx context.Context, mediaID []byte, virusName string) error {
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
