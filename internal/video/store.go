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

// UpdateVideoSourceStatus sets a video source status by id.
func (s *Store) UpdateVideoSourceStatus(ctx context.Context, id pgtype.UUID, status string) (gen.VideoSource, error) {
	row, err := s.write.UpdateVideoSourceStatus(ctx, gen.UpdateVideoSourceStatusParams{
		ID: id,

		Status: status,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.VideoSource{}, errpkg.ErrNotFound
		}

		return gen.VideoSource{}, fmt.Errorf("updating video source status: %w", err)
	}

	return gen.VideoSource{
		ID: row.ID,

		AuthorUserID:     row.AuthorUserID,
		SourceType:       row.SourceType,
		SourceUrl:        row.SourceUrl,
		ExternalID:       row.ExternalID,
		Title:            row.Title,
		Description:      row.Description,
		Category:         row.Category,
		ThumbnailUrl:     row.ThumbnailUrl,
		ThumbnailMediaID: row.ThumbnailMediaID,
		DurationSecs:     row.DurationSecs,
		Status:           row.Status,
		FieldValues:      row.FieldValues,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
		SourceID:         row.SourceID,
		Slug:             row.Slug,
	}, nil
}

// CreateOrUpdateSource upserts a video source keyed by (, source_url).
func (s *Store) CreateOrUpdateSource(ctx context.Context, arg gen.CreateOrUpdateVideoSourceParams) (gen.VideoSource, error) {
	row, err := s.write.CreateOrUpdateVideoSource(ctx, arg)
	if err != nil {
		return gen.VideoSource{}, fmt.Errorf("upserting video source: %w", err)
	}

	return gen.VideoSource{
		ID: row.ID,

		AuthorUserID:     row.AuthorUserID,
		SourceType:       row.SourceType,
		SourceUrl:        row.SourceUrl,
		ExternalID:       row.ExternalID,
		Title:            row.Title,
		Description:      row.Description,
		Category:         row.Category,
		ThumbnailUrl:     row.ThumbnailUrl,
		ThumbnailMediaID: row.ThumbnailMediaID,
		DurationSecs:     row.DurationSecs,
		Status:           row.Status,
		FieldValues:      row.FieldValues,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
		SourceID:         row.SourceID,
		Slug:             row.Slug,
	}, nil
}

// GetSourceByURL returns a source by platform and canonical URL.
func (s *Store) GetSourceByURL(ctx context.Context, sourceURL string) (gen.VideoSource, error) {
	row, err := s.read.GetVideoSourceByURL(ctx,

		sourceURL,
	)
	if err != nil {
		return gen.VideoSource{}, fmt.Errorf("getting video source by URL: %w", err)
	}

	return gen.VideoSource{
		ID: row.ID,

		AuthorUserID:     row.AuthorUserID,
		SourceType:       row.SourceType,
		SourceUrl:        row.SourceUrl,
		ExternalID:       row.ExternalID,
		Title:            row.Title,
		Description:      row.Description,
		Category:         row.Category,
		ThumbnailUrl:     row.ThumbnailUrl,
		ThumbnailMediaID: row.ThumbnailMediaID,
		DurationSecs:     row.DurationSecs,
		Status:           row.Status,
		FieldValues:      row.FieldValues,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
		SourceID:         row.SourceID,
		Slug:             row.Slug,
	}, nil
}

// GetSourceByID returns a single video source.
func (s *Store) GetSourceByID(ctx context.Context, id pgtype.UUID) (gen.VideoSource, error) {
	row, err := s.read.GetVideoSourceByID(ctx,
		id,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.VideoSource{}, errpkg.ErrNotFound
		}

		return gen.VideoSource{}, fmt.Errorf("getting video source: %w", err)
	}

	return row, nil
}

// ListSourcesForAdmin lists video sources for the admin queue.
func (s *Store) ListSourcesForAdmin(ctx context.Context, status, sourceType, q, sort, dir string, limit, offset int32) ([]gen.ListVideoSourcesForAdminRow, error) {
	rows, err := s.read.ListVideoSourcesForAdmin(ctx, gen.ListVideoSourcesForAdminParams{

		Status:     status,
		SourceType: sourceType,
		Q:          q,
		Sort:       sort,
		Dir:        dir,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing admin video sources: %w", err)
	}

	return rows, nil
}

// CountSourcesForAdmin returns the total filtered video source count for the admin queue.
func (s *Store) CountSourcesForAdmin(ctx context.Context, status, sourceType, q string) (int32, error) {
	total, err := s.read.CountVideoSourcesForAdmin(ctx, gen.CountVideoSourcesForAdminParams{

		Status:     status,
		SourceType: sourceType,
		Q:          q,
	})
	if err != nil {
		return 0, fmt.Errorf("counting admin video sources: %w", err)
	}

	return total, nil
}

// GetSourceByExternalID returns a source by (source_type, external_id).
func (s *Store) GetSourceByExternalID(ctx context.Context, sourceType, externalID string) (gen.VideoSource, error) {
	row, err := s.read.GetVideoSourceByExternalID(ctx, gen.GetVideoSourceByExternalIDParams{

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
func (s *Store) ListSourcesByUsername(ctx context.Context, username string, limit, offset int32) ([]gen.VideoSource, error) {
	rows, err := s.read.ListVideoSourcesByAuthor(ctx, gen.ListVideoSourcesByAuthorParams{

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
func (s *Store) ListSourcesByAuthorUserID(ctx context.Context, authorUserID pgtype.UUID, limit, offset int32) ([]gen.VideoSource, error) {
	rows, err := s.read.ListVideoSourcesByAuthorUserID(ctx, gen.ListVideoSourcesByAuthorUserIDParams{

		AuthorUserID: authorUserID,
		PageLimit:    limit,
		PageOffset:   offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing video sources by author id: %w", err)
	}

	return rows, nil
}

// ListBySourceID returns active video sources linked to a canonical
// intel_sources row, plus the total count.
func (s *Store) ListBySourceID(ctx context.Context, sourceID pgtype.UUID, limit, offset int32) ([]gen.VideoSource, int64, error) {
	rows, err := s.read.ListVideoSourcesBySourceID(ctx, gen.ListVideoSourcesBySourceIDParams{

		SourceID:   sourceID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("listing video sources by source id: %w", err)
	}

	total, err := s.read.CountVideoSourcesBySourceID(ctx,

		sourceID,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("counting video sources by source id: %w", err)
	}

	return rows, int64(total), nil
}

// SetVideoSourceIntelSource force-stamps the canonical intel_sources link on
// a video source (§106 channel-sync amendment).
func (s *Store) SetVideoSourceIntelSource(ctx context.Context, videoSourceID, intelSourceID pgtype.UUID) error {
	return s.write.SetVideoSourceIntelSource(ctx, gen.SetVideoSourceIntelSourceParams{
		SourceID: intelSourceID,
		ID:       videoSourceID,
	})
}

// LinkVideoSourcesToChannel backfills source_id on a channel's previously
// imported videos once the channel's intel source is registered (§106).
func (s *Store) LinkVideoSourcesToChannel(ctx context.Context, sourceID, authorID pgtype.UUID) error {
	return s.write.LinkVideoSourcesToChannel(ctx, gen.LinkVideoSourcesToChannelParams{
		SourceID:     sourceID,
		AuthorUserID: authorID,
	})
}

// GetSourceBySlug returns a video source by its public slug (§106
// /video/{slug} canonical route).
func (s *Store) GetSourceBySlug(ctx context.Context, slug string) (gen.VideoSource, error) {
	row, err := s.read.GetVideoSourceBySlug(ctx, slug)
	if err != nil {
		return gen.VideoSource{}, fmt.Errorf("getting video source by slug: %w", err)
	}

	return row, nil
}

// GetChannelUsernameBySourceID returns the channel account username behind a
// video_channel intel source (§106).
func (s *Store) GetChannelUsernameBySourceID(ctx context.Context, sourceID pgtype.UUID) (string, error) {
	return s.read.GetVideoChannelUsernameBySourceID(ctx, sourceID)
}

// ListItemsBySourceID returns the video sources for a canonical intel source
// enriched with the author username and published content slug so /sources
// item cards can link to the in-app video page (§106).
func (s *Store) ListItemsBySourceID(ctx context.Context, sourceID pgtype.UUID, limit, offset int32) ([]gen.ListVideoSourceItemsBySourceIDRow, int64, error) {
	rows, err := s.read.ListVideoSourceItemsBySourceID(ctx, gen.ListVideoSourceItemsBySourceIDParams{

		SourceID:   sourceID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("listing video source items by source id: %w", err)
	}

	total, err := s.read.CountVideoSourcesBySourceID(ctx,

		sourceID,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("counting video sources by source id: %w", err)
	}

	return rows, int64(total), nil
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
func (s *Store) ListImportJobsByUser(ctx context.Context, userID pgtype.UUID, limit, offset int32) ([]gen.VideoImportJob, error) {
	rows, err := s.read.ListVideoImportJobsByUser(ctx, gen.ListVideoImportJobsByUserParams{

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
func (s *Store) ListChannelClaimsByAuthor(ctx context.Context, videoAuthorUserID pgtype.UUID, limit, offset int32) ([]gen.VideoChannelClaim, error) {
	rows, err := s.read.ListVideoChannelClaimsByAuthor(ctx, gen.ListVideoChannelClaimsByAuthorParams{

		VideoAuthorUserID: videoAuthorUserID,
		PageLimit:         limit,
		PageOffset:        offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing video channel claims: %w", err)
	}

	return rows, nil
}

// GetUserForClaimByUsername resolves a username to the user record needed for
// the claim flow (id, is_synthetic, synthetic_source).
func (s *Store) GetUserForClaimByUsername(ctx context.Context, username string) (gen.GetUserForVideoClaimRow, error) {
	row, err := s.read.GetUserForVideoClaim(ctx,

		username,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.GetUserForVideoClaimRow{}, errpkg.ErrNotFound
		}

		return gen.GetUserForVideoClaimRow{}, fmt.Errorf("looking up user for claim: %w", err)
	}

	return row, nil
}

// GetUserSyntheticStatusByID returns whether the given user is a synthetic channel account.
func (s *Store) GetUserSyntheticStatusByID(ctx context.Context, userID pgtype.UUID) (bool, error) {
	isSynthetic, err := s.read.GetUserSyntheticStatusByID(ctx,

		userID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, errpkg.ErrNotFound
		}

		return false, fmt.Errorf("looking up user synthetic status: %w", err)
	}

	return isSynthetic, nil
}

// CountOpenClaimsByAuthor counts pending or approved ownership claims for an author.
func (s *Store) CountOpenClaimsByAuthor(ctx context.Context, videoAuthorUserID pgtype.UUID) (int32, error) {
	count, err := s.read.CountOpenVideoChannelClaimsByAuthor(ctx,

		videoAuthorUserID,
	)
	if err != nil {
		return 0, fmt.Errorf("counting open channel claims: %w", err)
	}

	return count, nil
}

// ListClaimsByClaimant lists claims filed by a user.
func (s *Store) ListClaimsByClaimant(ctx context.Context, claimantUserID pgtype.UUID, limit, offset int32) ([]gen.VideoChannelClaim, error) {
	rows, err := s.read.ListVideoChannelClaimsByClaimant(ctx, gen.ListVideoChannelClaimsByClaimantParams{

		ClaimantUserID: claimantUserID,
		PageLimit:      limit,
		PageOffset:     offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing claimant video channel claims: %w", err)
	}

	return rows, nil
}

// ListClaimsForAdmin returns the admin review queue with optional status/claim_type filters, search, and sorting.
func (s *Store) ListClaimsForAdmin(ctx context.Context, status, claimType, q, sort, dir string, limit, offset int32) ([]gen.ListVideoChannelClaimsRow, error) {
	rows, err := s.read.ListVideoChannelClaims(ctx, gen.ListVideoChannelClaimsParams{

		Status:     status,
		ClaimType:  claimType,
		Q:          q,
		Sort:       sort,
		Dir:        dir,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing admin video channel claims: %w", err)
	}

	return rows, nil
}

// CountClaimsForAdmin returns the total number of claims for the admin queue filter.
func (s *Store) CountClaimsForAdmin(ctx context.Context, status, claimType, q string) (int32, error) {
	total, err := s.read.CountVideoChannelClaims(ctx, gen.CountVideoChannelClaimsParams{

		Status:    status,
		ClaimType: claimType,
		Q:         q,
	})
	if err != nil {
		return 0, fmt.Errorf("counting admin video channel claims: %w", err)
	}

	return total, nil
}

// ListImportJobsForAdmin lists video import jobs for the admin queue.
func (s *Store) ListImportJobsForAdmin(ctx context.Context, status, sourceType, q, sort, dir string, limit, offset int32) ([]gen.ListVideoImportJobsForAdminRow, error) {
	rows, err := s.read.ListVideoImportJobsForAdmin(ctx, gen.ListVideoImportJobsForAdminParams{

		Status:     status,
		SourceType: sourceType,
		Q:          q,
		Sort:       sort,
		Dir:        dir,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing admin video import jobs: %w", err)
	}

	return rows, nil
}

// CountImportJobsForAdmin returns the total filtered video import job count for the admin queue.
func (s *Store) CountImportJobsForAdmin(ctx context.Context, status, sourceType, q string) (int32, error) {
	total, err := s.read.CountVideoImportJobsForAdmin(ctx, gen.CountVideoImportJobsForAdminParams{

		Status:     status,
		SourceType: sourceType,
		Q:          q,
	})
	if err != nil {
		return 0, fmt.Errorf("counting admin video import jobs: %w", err)
	}

	return total, nil
}

// GetChannelClaimByID returns a single claim row.
func (s *Store) GetChannelClaimByID(ctx context.Context, id pgtype.UUID) (gen.VideoChannelClaim, error) {
	row, err := s.read.GetVideoChannelClaimByID(ctx,

		id,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.VideoChannelClaim{}, errpkg.ErrNotFound
		}

		return gen.VideoChannelClaim{}, fmt.Errorf("getting video channel claim: %w", err)
	}

	return row, nil
}

// GetImportJobByID returns a single import job.
func (s *Store) GetImportJobByID(ctx context.Context, id pgtype.UUID) (gen.VideoImportJob, error) {
	row, err := s.read.GetVideoImportJobByID(ctx,

		id,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.VideoImportJob{}, errpkg.ErrNotFound
		}

		return gen.VideoImportJob{}, fmt.Errorf("getting video import job: %w", err)
	}

	return row, nil
}

// UpdateImportJob updates an import job's progress and status.
func (s *Store) UpdateImportJob(ctx context.Context, arg gen.UpdateVideoImportJobParams) (gen.VideoImportJob, error) {
	row, err := s.write.UpdateVideoImportJob(ctx, arg)
	if err != nil {
		return gen.VideoImportJob{}, fmt.Errorf("updating video import job: %w", err)
	}

	return row, nil
}

// GetUserByUsername looks up a user by platform and username.
func (s *Store) GetUserByUsername(ctx context.Context, username string) (gen.GetUserByUsernameRow, error) {
	row, err := s.read.GetUserByUsername(ctx,

		username,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.GetUserByUsernameRow{}, errpkg.ErrNotFound
		}

		return gen.GetUserByUsernameRow{}, fmt.Errorf("getting user by username: %w", err)
	}

	return row, nil
}

// GetDefaultSpaceForUser returns the first public or member space the user
// can access, for attributing auto-published video content.
func (s *Store) GetDefaultSpaceForUser(ctx context.Context, userID pgtype.UUID) (pgtype.UUID, error) {
	rows, err := s.read.ListSpaces(ctx,

		userID,
	)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("listing spaces for user: %w", err)
	}

	if len(rows) == 0 {
		return pgtype.UUID{}, errpkg.ErrNotFound
	}

	return rows[0].ID, nil
}
