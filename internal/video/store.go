package video

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Store wraps SQLC queries for video_sources, video_channel_claims, and
// video_import_jobs.
type Store struct {
	read  *gen.Queries
	write *gen.Queries
	db    *db.DB
}

// NewStore creates a Store using the writer and reader pools.
func NewStore(writer, reader *pgxpool.Pool) *Store {
	return &Store{
		read:  gen.New(reader),
		write: gen.New(writer),
		db:    &db.DB{Writer: writer, Reader: reader},
	}
}

// CreateOrUpdateSource upserts a video source keyed by (tenant_id, source_url).
func (s *Store) CreateOrUpdateSource(ctx context.Context, arg gen.CreateOrUpdateVideoSourceParams) (gen.VideoSource, error) {
	row, err := s.write.CreateOrUpdateVideoSource(ctx, arg)
	if err != nil {
		return gen.VideoSource{}, fmt.Errorf("upserting video source: %w", err)
	}
	return row, nil
}

// GetSourceByURL returns a source by tenant and canonical URL.
func (s *Store) GetSourceByURL(ctx context.Context, tenantID pgtype.UUID, sourceURL string) (gen.VideoSource, error) {
	row, err := s.read.GetVideoSourceByURL(ctx, gen.GetVideoSourceByURLParams{
		TenantID:  tenantID,
		SourceUrl: sourceURL,
	})
	if err != nil {
		return gen.VideoSource{}, fmt.Errorf("getting video source by URL: %w", err)
	}
	return row, nil
}

// GetSourceByID returns a single video source.
func (s *Store) GetSourceByID(ctx context.Context, tenantID, id pgtype.UUID) (gen.VideoSource, error) {
	row, err := s.read.GetVideoSourceByID(ctx, gen.GetVideoSourceByIDParams{
		ID:       id,
		TenantID: tenantID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.VideoSource{}, errpkg.ErrNotFound
		}
		return gen.VideoSource{}, fmt.Errorf("getting video source: %w", err)
	}
	return row, nil
}

// GetSourceByExternalID returns a source by (source_type, external_id).
func (s *Store) GetSourceByExternalID(ctx context.Context, tenantID pgtype.UUID, sourceType, externalID string) (gen.VideoSource, error) {
	row, err := s.read.GetVideoSourceByExternalID(ctx, gen.GetVideoSourceByExternalIDParams{
		TenantID:   tenantID,
		SourceType: sourceType,
		ExternalID: &externalID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.VideoSource{}, errpkg.ErrNotFound
		}
		return gen.VideoSource{}, fmt.Errorf("getting video source by external id: %w", err)
	}
	return row, nil
}

// ListSourcesByUsername lists active sources for a channel identified by
// username.
func (s *Store) ListSourcesByUsername(ctx context.Context, tenantID pgtype.UUID, username string, limit, offset int32) ([]gen.VideoSource, error) {
	rows, err := s.read.ListVideoSourcesByAuthor(ctx, gen.ListVideoSourcesByAuthorParams{
		TenantID:   tenantID,
		Username:   username,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing video sources by username: %w", err)
	}
	return rows, nil
}

// ListSourcesByAuthorUserID lists sources for a specific user ID.
func (s *Store) ListSourcesByAuthorUserID(ctx context.Context, tenantID, authorUserID pgtype.UUID, limit, offset int32) ([]gen.VideoSource, error) {
	rows, err := s.read.ListVideoSourcesByAuthorUserID(ctx, gen.ListVideoSourcesByAuthorUserIDParams{
		TenantID:     tenantID,
		AuthorUserID: authorUserID,
		PageLimit:    limit,
		PageOffset:   offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing video sources by author id: %w", err)
	}
	return rows, nil
}

// CreateImportJob creates a bulk import job for the user.
func (s *Store) CreateImportJob(ctx context.Context, arg gen.CreateVideoImportJobParams) (gen.VideoImportJob, error) {
	row, err := s.write.CreateVideoImportJob(ctx, arg)
	if err != nil {
		return gen.VideoImportJob{}, fmt.Errorf("creating video import job: %w", err)
	}
	return row, nil
}

// ListImportJobsByUser lists import jobs for a user.
func (s *Store) ListImportJobsByUser(ctx context.Context, tenantID, userID pgtype.UUID, limit, offset int32) ([]gen.VideoImportJob, error) {
	rows, err := s.read.ListVideoImportJobsByUser(ctx, gen.ListVideoImportJobsByUserParams{
		TenantID:   tenantID,
		UserID:     userID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing video import jobs: %w", err)
	}
	return rows, nil
}

// CreateChannelClaim creates a video channel claim.
func (s *Store) CreateChannelClaim(ctx context.Context, arg gen.CreateVideoChannelClaimParams) (gen.VideoChannelClaim, error) {
	row, err := s.write.CreateVideoChannelClaim(ctx, arg)
	if err != nil {
		return gen.VideoChannelClaim{}, fmt.Errorf("creating video channel claim: %w", err)
	}
	return row, nil
}

// ListChannelClaimsByAuthor lists claims against a channel.
func (s *Store) ListChannelClaimsByAuthor(ctx context.Context, tenantID, videoAuthorUserID pgtype.UUID, limit, offset int32) ([]gen.VideoChannelClaim, error) {
	rows, err := s.read.ListVideoChannelClaimsByAuthor(ctx, gen.ListVideoChannelClaimsByAuthorParams{
		TenantID:          tenantID,
		VideoAuthorUserID: videoAuthorUserID,
		PageLimit:         limit,
		PageOffset:        offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing video channel claims: %w", err)
	}
	return rows, nil
}
