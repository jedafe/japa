package federation

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store wraps the sqlc queries used by the federation service. Read and
// write pools stay separate per the repo's db.DB split.
type Store struct {
	read  *gen.Queries
	write *gen.Queries
}

// NewStore creates a Store backed by the given query handles.
func NewStore(read, write *gen.Queries) *Store {
	return &Store{read: read, write: write}
}

func (s *Store) actorByUsername(ctx context.Context, username string) (gen.FederatedActor, error) {
	return s.read.GetFederatedActorByUsername(ctx, username)
}

func (s *Store) actorProfile(ctx context.Context, username string) (gen.GetActorProfileByUsernameRow, error) {
	return s.read.GetActorProfileByUsername(ctx, username)
}

func (s *Store) actorByUserID(ctx context.Context, userID pgtype.UUID) (gen.FederatedActor, error) {
	return s.read.GetFederatedActorByUserID(ctx, userID)
}

func (s *Store) userByID(ctx context.Context, userID pgtype.UUID) (gen.GetUserByIDRow, error) {
	return s.read.GetUserByID(ctx, userID)
}

func (s *Store) userPrivacy(ctx context.Context, userID pgtype.UUID) (gen.GetUserPrivacySettingsRow, error) {
	return s.read.GetUserPrivacySettings(ctx, userID)
}

func (s *Store) createActor(ctx context.Context, arg gen.CreateFederatedActorParams) (gen.FederatedActor, error) {
	return s.write.CreateFederatedActor(ctx, arg)
}

func (s *Store) setActorStatus(ctx context.Context, id pgtype.UUID, status string) error {
	return s.write.UpdateFederatedActorStatus(ctx, gen.UpdateFederatedActorStatusParams{ID: id, Status: status})
}

func (s *Store) listActors(ctx context.Context, q string, limit, offset int32) ([]gen.ListFederatedActorsRow, error) {
	return s.read.ListFederatedActors(ctx, gen.ListFederatedActorsParams{Q: q, PageLimit: limit, PageOffset: offset})
}

func (s *Store) countActors(ctx context.Context, q string) (int64, error) {
	return s.read.CountFederatedActors(ctx, q)
}

func (s *Store) stats(ctx context.Context) (gen.GetFederationStatsRow, error) {
	return s.read.GetFederationStats(ctx)
}

func (s *Store) nodeInfoUsage(ctx context.Context) (gen.GetNodeInfoUsageRow, error) {
	return s.read.GetNodeInfoUsage(ctx)
}

func (s *Store) federatableContent(ctx context.Context, authorID pgtype.UUID, types []string, limit, offset int32) ([]gen.ListFederatableContentByAuthorRow, error) {
	return s.read.ListFederatableContentByAuthor(ctx, gen.ListFederatableContentByAuthorParams{
		AuthorID:     authorID,
		ContentTypes: types,
		PageLimit:    limit,
		PageOffset:   offset,
	})
}

func (s *Store) countFederatableContent(ctx context.Context, authorID pgtype.UUID, types []string) (int64, error) {
	return s.read.CountFederatableContentByAuthor(ctx, gen.CountFederatableContentByAuthorParams{
		AuthorID:     authorID,
		ContentTypes: types,
	})
}

func (s *Store) federatableContentByID(ctx context.Context, id pgtype.UUID, types []string) (gen.GetFederatableContentByIDRow, error) {
	return s.read.GetFederatableContentByID(ctx, gen.GetFederatableContentByIDParams{
		ID:           id,
		ContentTypes: types,
	})
}

func (s *Store) remoteFollowerURLs(ctx context.Context, localActorID pgtype.UUID, limit, offset int32) ([]string, error) {
	return s.read.ListRemoteFollowerURLsByActor(ctx, gen.ListRemoteFollowerURLsByActorParams{
		LocalActorID: localActorID,
		PageLimit:    limit,
		PageOffset:   offset,
	})
}

func (s *Store) countRemoteFollowers(ctx context.Context, localActorID pgtype.UUID) (int64, error) {
	return s.read.CountRemoteFollowersByActor(ctx, localActorID)
}

func (s *Store) localFollowerURLs(ctx context.Context, userID pgtype.UUID, limit, offset int32) ([]string, error) {
	return s.read.ListLocalFederatedFollowerURLs(ctx, gen.ListLocalFederatedFollowerURLsParams{
		UserID:     userID,
		PageLimit:  limit,
		PageOffset: offset,
	})
}

func (s *Store) countLocalFollowers(ctx context.Context, userID pgtype.UUID) (int64, error) {
	return s.read.CountLocalFederatedFollowers(ctx, userID)
}

func (s *Store) localFollowingURLs(ctx context.Context, userID pgtype.UUID, limit, offset int32) ([]string, error) {
	return s.read.ListLocalFederatedFollowingURLs(ctx, gen.ListLocalFederatedFollowingURLsParams{
		UserID:     userID,
		PageLimit:  limit,
		PageOffset: offset,
	})
}

func (s *Store) countLocalFollowing(ctx context.Context, userID pgtype.UUID) (int64, error) {
	return s.read.CountLocalFederatedFollowing(ctx, userID)
}

func (s *Store) createBlock(ctx context.Context, domain, reason string, blockedBy pgtype.UUID) (gen.FederationInstanceBlock, error) {
	return s.write.CreateInstanceBlock(ctx, gen.CreateInstanceBlockParams{
		Domain:    domain,
		Reason:    stringPtrOrNil(reason),
		BlockedBy: blockedBy,
	})
}

func (s *Store) deleteBlock(ctx context.Context, id pgtype.UUID) error {
	return s.write.DeleteInstanceBlock(ctx, id)
}

func (s *Store) listBlocks(ctx context.Context, q string, limit, offset int32) ([]gen.FederationInstanceBlock, error) {
	return s.read.ListInstanceBlocks(ctx, gen.ListInstanceBlocksParams{Q: q, PageLimit: limit, PageOffset: offset})
}

func (s *Store) countBlocks(ctx context.Context, q string) (int64, error) {
	return s.read.CountInstanceBlocks(ctx, q)
}

func stringPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
