package federation

import (
	"context"
	"time"

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

func (s *Store) actorHandlesByUserID(ctx context.Context, userID pgtype.UUID) (gen.GetFederatedActorHandlesByUserIDRow, error) {
	return s.read.GetFederatedActorHandlesByUserID(ctx, userID)
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

// setActorStatusFrom performs a conditional transition — it only writes
// while the row still holds expected, and reports rows affected. 0 means
// a concurrent transition won; callers re-read and dispatch on that state.
func (s *Store) setActorStatusFrom(ctx context.Context, id pgtype.UUID, status, expected string) (int64, error) {
	return s.write.UpdateFederatedActorStatusFrom(ctx, gen.UpdateFederatedActorStatusFromParams{ID: id, Status: status, Expected: expected})
}

func (s *Store) listActors(ctx context.Context, q, sortKey, sortDir string, limit, offset int32) ([]gen.ListFederatedActorsRow, error) {
	return s.read.ListFederatedActors(ctx, gen.ListFederatedActorsParams{
		Q: q, SortKey: sortKey, SortDir: sortDir, PageLimit: limit, PageOffset: offset,
	})
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

// ---------------------------------------------------------------------------
// M2 — remote actors, follows, deliveries, inbound, provenance
// ---------------------------------------------------------------------------

func (s *Store) remoteActorByURL(ctx context.Context, actorURL string) (gen.RemoteActor, error) {
	return s.read.GetRemoteActorByURL(ctx, actorURL)
}

func (s *Store) remoteActorByID(ctx context.Context, id pgtype.UUID) (gen.RemoteActor, error) {
	return s.read.GetRemoteActorByID(ctx, id)
}

func (s *Store) remoteActorByKeyID(ctx context.Context, keyID string) (gen.RemoteActor, error) {
	return s.read.GetRemoteActorByKeyID(ctx, &keyID)
}

func (s *Store) upsertRemoteActor(ctx context.Context, arg gen.UpsertRemoteActorParams) (gen.RemoteActor, error) {
	return s.write.UpsertRemoteActor(ctx, arg)
}

func (s *Store) setRemoteActorStatus(ctx context.Context, id pgtype.UUID, status string) error {
	return s.write.SetRemoteActorStatus(ctx, gen.SetRemoteActorStatusParams{ID: id, Status: status})
}

func (s *Store) upsertFollow(ctx context.Context, arg gen.UpsertFederationFollowParams) (gen.FederationFollow, error) {
	return s.write.UpsertFederationFollow(ctx, arg)
}

func (s *Store) followByActivityID(ctx context.Context, apID string) (gen.FederationFollow, error) {
	return s.read.GetFederationFollowByActivityID(ctx, &apID)
}

func (s *Store) followByPair(ctx context.Context, localActorID, remoteActorID pgtype.UUID) (gen.FederationFollow, error) {
	return s.read.GetFederationFollowByPair(ctx, gen.GetFederationFollowByPairParams{
		LocalActorID:  localActorID,
		RemoteActorID: remoteActorID,
	})
}

func (s *Store) setFollowStatus(ctx context.Context, localActorID, remoteActorID pgtype.UUID, status string) error {
	return s.write.UpdateFederationFollowStatus(ctx, gen.UpdateFederationFollowStatusParams{
		LocalActorID:  localActorID,
		RemoteActorID: remoteActorID,
		Status:        status,
	})
}

func (s *Store) undoFollowsByRemoteActor(ctx context.Context, remoteActorID pgtype.UUID) error {
	return s.write.UndoFederationFollowsByRemoteActor(ctx, remoteActorID)
}

func (s *Store) remoteFollowerInboxes(ctx context.Context, localActorID pgtype.UUID) ([]gen.ListRemoteFollowerInboxesByActorRow, error) {
	return s.read.ListRemoteFollowerInboxesByActor(ctx, localActorID)
}

func (s *Store) insertDelivery(ctx context.Context, arg gen.InsertFederationDeliveryParams) (gen.FederationDelivery, error) {
	return s.write.InsertFederationDelivery(ctx, arg)
}

func (s *Store) deliveryByID(ctx context.Context, id pgtype.UUID) (gen.FederationDelivery, error) {
	return s.read.GetFederationDeliveryByID(ctx, id)
}

func (s *Store) dueDeliveries(ctx context.Context, limit int32) ([]gen.FederationDelivery, error) {
	return s.read.ListDueFederationDeliveries(ctx, limit)
}

func (s *Store) updateDeliveryStatus(ctx context.Context, arg gen.UpdateFederationDeliveryStatusParams) error {
	return s.write.UpdateFederationDeliveryStatus(ctx, arg)
}

func (s *Store) pruneDeliveries(ctx context.Context, retentionDays int32) error {
	return s.write.PruneFederationDeliveries(ctx, retentionDays)
}

func (s *Store) pruneInbound(ctx context.Context, retentionDays int32) error {
	return s.write.PruneFederationInbound(ctx, retentionDays)
}

func (s *Store) insertInbound(ctx context.Context, arg gen.InsertFederationInboundParams) (gen.FederationInbound, error) {
	return s.write.InsertFederationInbound(ctx, arg)
}

func (s *Store) inboundByID(ctx context.Context, id pgtype.UUID) (gen.FederationInbound, error) {
	return s.read.GetFederationInboundByID(ctx, id)
}

func (s *Store) updateInboundStatus(ctx context.Context, id pgtype.UUID, status string, errMsg *string) error {
	return s.write.UpdateFederationInboundStatus(ctx, gen.UpdateFederationInboundStatusParams{
		ID: id, Status: status, Error: errMsg,
	})
}

func (s *Store) instanceBlock(ctx context.Context, domain string) (gen.FederationInstanceBlock, error) {
	return s.read.GetInstanceBlockByDomain(ctx, domain)
}

func (s *Store) insertRemoteFollow(ctx context.Context, remoteActorID, userID pgtype.UUID) (int64, error) {
	return s.write.InsertRemoteFollow(ctx, gen.InsertRemoteFollowParams{
		RemoteActorID: remoteActorID,
		UserID:        userID,
	})
}

func (s *Store) deleteRemoteFollow(ctx context.Context, remoteActorID, userID pgtype.UUID) (int64, error) {
	return s.write.DeleteRemoteFollow(ctx, gen.DeleteRemoteFollowParams{
		RemoteActorID: remoteActorID,
		UserID:        userID,
	})
}

func (s *Store) deleteRemoteFollowsByActor(ctx context.Context, remoteActorID pgtype.UUID) error {
	return s.write.DeleteRemoteFollowsByActor(ctx, remoteActorID)
}

func (s *Store) addRemoteReaction(ctx context.Context, entityType string, entityID, remoteActorID pgtype.UUID, reactionKey string) (int64, error) {
	return s.write.AddRemoteReaction(ctx, gen.AddRemoteReactionParams{
		EntityType:    entityType,
		EntityID:      entityID,
		RemoteActorID: remoteActorID,
		ReactionKey:   reactionKey,
	})
}

func (s *Store) removeRemoteReaction(ctx context.Context, entityType string, entityID, remoteActorID pgtype.UUID, reactionKey string) (int64, error) {
	return s.write.RemoveRemoteReaction(ctx, gen.RemoveRemoteReactionParams{
		EntityType:    entityType,
		EntityID:      entityID,
		RemoteActorID: remoteActorID,
		ReactionKey:   reactionKey,
	})
}

func (s *Store) deleteRemoteReactionsByActor(ctx context.Context, remoteActorID pgtype.UUID) ([]gen.DeleteRemoteReactionsByActorRow, error) {
	return s.write.DeleteRemoteReactionsByActor(ctx, remoteActorID)
}

func (s *Store) insertAPRemoteComment(ctx context.Context, arg gen.InsertAPRemoteCommentParams) (gen.Comment, error) {
	return s.write.InsertAPRemoteComment(ctx, arg)
}

func (s *Store) softDeleteAPCommentsByActor(ctx context.Context, remoteActorID pgtype.UUID) ([]gen.SoftDeleteAPCommentsByActorRow, error) {
	return s.write.SoftDeleteAPCommentsByActor(ctx, remoteActorID)
}

func (s *Store) incrementShareCount(ctx context.Context, contentID pgtype.UUID) error {
	return s.write.IncrementContentShareCount(ctx, contentID)
}

func (s *Store) decrementShareCount(ctx context.Context, contentID pgtype.UUID) error {
	return s.write.DecrementContentShareCount(ctx, contentID)
}

func (s *Store) actorByID(ctx context.Context, id pgtype.UUID) (gen.FederatedActor, error) {
	return s.read.GetFederatedActorByID(ctx, id)
}

func (s *Store) hasDeliveredObject(ctx context.Context, localActorID pgtype.UUID, objectID string) bool {
	v, err := s.read.HasDeliveredObject(ctx, gen.HasDeliveredObjectParams{LocalActorID: localActorID, ObjectID: objectID})
	return err == nil && v
}

func (s *Store) deliveredObjectActor(ctx context.Context, objectID string) (pgtype.UUID, bool) {
	id, err := s.read.GetDeliveredObjectActor(ctx, objectID)
	if err != nil {
		return pgtype.UUID{}, false
	}
	return id, true
}

func (s *Store) pendingFollowRequests(ctx context.Context, userID pgtype.UUID) ([]gen.ListPendingFederationFollowsByUserRow, error) {
	return s.read.ListPendingFederationFollowsByUser(ctx, userID)
}

func (s *Store) resolveFederationFollow(ctx context.Context, localActorID, remoteActorID pgtype.UUID, status string) (int64, error) {
	return s.write.ResolveFederationFollow(ctx, gen.ResolveFederationFollowParams{
		LocalActorID:  localActorID,
		RemoteActorID: remoteActorID,
		Status:        status,
	})
}

func (s *Store) inboundByActivityID(ctx context.Context, apID string) (gen.FederationInbound, error) {
	return s.read.GetFederationInboundByActivityID(ctx, &apID)
}

// contentLiteByID is the ungated contents read the delete fanout and
// inbound lock checks need — soft-deleted/unpublished rows still resolve
// (the question is what the row proves, not whether the object is live).
func (s *Store) contentLiteByID(ctx context.Context, id pgtype.UUID) (gen.GetContentLiteByIDRow, error) {
	return s.read.GetContentLiteByID(ctx, id)
}

// wasPublished is the durable "remote copy plausible" signal —
// published_at survives unpublish, private flips, and soft-delete.
func wasPublished(row gen.GetContentLiteByIDRow) bool {
	return row.PublishedAt.Valid && row.PublishedAt.Time.Before(time.Now())
}

func (s *Store) staleInbound(ctx context.Context, limit int32) ([]pgtype.UUID, error) {
	return s.read.ListStaleFederationInbound(ctx, limit)
}
