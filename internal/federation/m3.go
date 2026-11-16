package federation

// Ref: japaconnect-engineering/sections/126-activitypub-federation.md (M3 — Discovery & Reach)
// Instance Application actor, space Group actors, relay subscriptions
// (FEP-ae0c Mastodon-style client), authorized-fetch gate, and Hashtag
// tag emission.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	jcrypto "github.com/jedafe/japa/internal/crypto"
	gen "github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/media"
)

// Actor kinds persisted on federated_actors.kind.
const (
	ActorKindPerson      = "person"
	ActorKindApplication = "application"
	ActorKindGroup       = "group"
)

// ---------------------------------------------------------------------------
// Instance actor (Application)
// ---------------------------------------------------------------------------

// instanceURL is the singleton actor's canonical @id.
func (s *Service) instanceURL() string { return s.apiBase() + "/federation/instance" }

// ensureInstanceActor returns the singleton 'application' actor, minting
// it (with its own RSA-2048 keypair) on first use. Concurrent minters
// converge through the partial unique index — a lost race re-reads.
func (s *Service) ensureInstanceActor(ctx context.Context) (gen.FederatedActor, error) {
	if s.store == nil {
		return gen.FederatedActor{}, fmt.Errorf("federation: store not configured")
	}
	if a, err := s.store.actorByKind(ctx, ActorKindApplication); err == nil {
		return a, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return gen.FederatedActor{}, err
	}
	username := s.handleDomain()
	if username == "" {
		username = "instance"
	}
	pubPEM, privPEM, err := generateActorKeyPair()
	if err != nil {
		return gen.FederatedActor{}, fmt.Errorf("federation: instance keypair: %w", err)
	}
	privEnc, err := jcrypto.Encrypt(privPEM)
	if err != nil {
		return gen.FederatedActor{}, fmt.Errorf("federation: instance key encrypt: %w", err)
	}
	actorURL := s.instanceURL()
	base := actorURL
	a, err := s.store.createActorOfKind(ctx, gen.CreateFederatedActorOfKindParams{
		ActorUrl:          actorURL,
		PreferredUsername: username,
		PublicKeyPem:      pubPEM,
		PrivateKeyEnc:     privEnc,
		PublicKeyID:       actorURL + "#main-key-" + uuid.NewString(),
		InboxUrl:          s.apiBase() + "/federation/inbox",
		OutboxUrl:         base + "/outbox",
		FollowersUrl:      base + "/followers",
		FollowingUrl:      base + "/following",
		Kind:              ActorKindApplication,
	})
	if err != nil {
		// Lost a concurrent mint (unique on actor_url / kind) — the winner's
		// row is the answer.
		if a2, rerr := s.store.actorByKind(ctx, ActorKindApplication); rerr == nil {
			return a2, nil
		}
		return gen.FederatedActor{}, fmt.Errorf("federation: instance actor create: %w", err)
	}
	return a, nil
}

// InstanceDocument serves GET /federation/instance — the Application doc
// remotes use for authorized-fetch signatures and relay Follows.
func (s *Service) InstanceDocument(ctx context.Context) (map[string]any, error) {
	const cacheKey = "jc:fed:instance:doc"
	if doc, ok := s.cacheGet(ctx, cacheKey); ok {
		return doc, nil
	}
	actor, err := s.ensureInstanceActor(ctx)
	if err != nil {
		return nil, err
	}
	if actor.Status != ActorStatusActive {
		return nil, ErrNotFound
	}
	doc := applicationDocument(actor, s.handleDomain(), s.siteBase())
	s.cacheSet(ctx, cacheKey, doc, 5*time.Minute)
	return doc, nil
}

// applicationDocument builds the Application actor document.
func applicationDocument(actor gen.FederatedActor, domain, siteBase string) map[string]any {
	doc := map[string]any{
		"@context":                  asContext,
		"id":                        actor.ActorUrl,
		"type":                      "Application",
		"preferredUsername":         actor.PreferredUsername,
		"name":                      "JapaConnect",
		"summary":                   "JapaConnect instance actor — signs relay subscriptions and authorized fetches.",
		"url":                       siteBase,
		"inbox":                     actor.InboxUrl,
		"outbox":                    actor.OutboxUrl,
		"followers":                 actor.FollowersUrl,
		"following":                 actor.FollowingUrl,
		"manuallyApprovesFollowers": false,
		"discoverable":              true,
		"published":                 actor.CreatedAt.Time.UTC().Format(time.RFC3339),
		"publicKey": map[string]any{
			"id":           actor.PublicKeyID,
			"owner":        actor.ActorUrl,
			"publicKeyPem": actor.PublicKeyPem,
		},
		"endpoints": map[string]any{"sharedInbox": actor.InboxUrl},
	}
	if domain != "" {
		doc["name"] = domain
	}
	return doc
}

// InstanceOutbox serves the instance-level OrderedCollection — every
// public federatable object surfaced as Create items attributed to their
// real authors (relay/crawl backfill).
func (s *Service) InstanceOutbox(ctx context.Context, page int) (map[string]any, error) {
	actor, err := s.ensureInstanceActor(ctx)
	if err != nil {
		return nil, err
	}
	if actor.Status != ActorStatusActive {
		return nil, ErrNotFound
	}
	types := s.federatableTypesFor(ctx)
	total, err := s.store.read.CountAllFederatableContent(ctx, types)
	if err != nil {
		return nil, fmt.Errorf("federation: instance outbox count: %w", err)
	}
	if page <= 0 {
		return orderedCollection(actor.OutboxUrl, total, actor.OutboxUrl+"?page=1"), nil
	}
	size := s.outboxPageSize(ctx)
	offset := int32(page-1) * size
	pageID := func(p int) string { return fmt.Sprintf("%s?page=%d", actor.OutboxUrl, p) }
	if int64(offset) >= total {
		return orderedCollectionPage(pageID(page), actor.OutboxUrl, []any{}, total, false, page > 1, pageID, page), nil
	}
	rows, err := s.store.read.ListAllFederatableContent(ctx, gen.ListAllFederatableContentParams{
		ContentTypes: types, PageLimit: size, PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("federation: instance outbox page: %w", err)
	}
	items := make([]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, s.instanceAnnounceItem(ctx, row))
	}
	return orderedCollectionPage(pageID(page), actor.OutboxUrl, items, total,
		int64(page)*int64(size) < total, page > 1, pageID, page), nil
}

// instanceAnnounceItem renders an outbox item: the instance actor
// announcing a member object (the member's own Create rides their
// personal outbox; here the instance boosts).
func (s *Service) instanceAnnounceItem(ctx context.Context, row gen.ListAllFederatableContentRow) any {
	objID := s.objectURL(row.ID.String())
	return map[string]any{
		"id":     fmt.Sprintf("%s#announce/%s", s.instanceURL(), row.ID.String()),
		"type":   "Announce",
		"actor":  s.instanceURL(),
		"object": objID,
		"to":     []string{asPublic},
	}
}

// InstanceFollowers serves the OrderedCollection of remote actors that
// follow the instance actor.
func (s *Service) InstanceFollowers(ctx context.Context, page int) (map[string]any, error) {
	actor, err := s.ensureInstanceActor(ctx)
	if err != nil {
		return nil, err
	}
	if actor.Status != ActorStatusActive {
		return nil, ErrNotFound
	}
	total, err := s.store.countRemoteFollowers(ctx, actor.ID)
	if err != nil {
		return nil, fmt.Errorf("federation: instance followers count: %w", err)
	}
	if page <= 0 {
		return orderedCollection(actor.FollowersUrl, total, actor.FollowersUrl+"?page=1"), nil
	}
	size := s.outboxPageSize(ctx)
	offset := int32(page-1) * size
	pageID := func(p int) string { return fmt.Sprintf("%s?page=%d", actor.FollowersUrl, p) }
	if int64(offset) >= total {
		return orderedCollectionPage(pageID(page), actor.FollowersUrl, []any{}, total, false, page > 1, pageID, page), nil
	}
	urls, err := s.store.remoteFollowerURLs(ctx, actor.ID, size, offset)
	if err != nil {
		return nil, fmt.Errorf("federation: instance followers: %w", err)
	}
	items := make([]any, 0, len(urls))
	for _, u := range urls {
		items = append(items, u)
	}
	return orderedCollectionPage(pageID(page), actor.FollowersUrl, items, total,
		int64(page)*int64(size) < total, page > 1, pageID, page), nil
}

// ---------------------------------------------------------------------------
// Group actors (spaces)
// ---------------------------------------------------------------------------

// ErrConflict marks state-conditional admin ops that can't proceed —
// e.g. provisioning a Group actor for a non-public space.
var ErrConflict = errors.New("federation: conflicting state")

// groupURL is the Group actor's canonical @id for a space slug.
func (s *Service) groupURL(slug string) string {
	return s.apiBase() + "/federation/groups/" + slug
}

// ProvisionGroupActor mints (or reactivates) the Group actor for a public
// space — admin path POST /admin/federation/groups.
func (s *Service) ProvisionGroupActor(ctx context.Context, spaceID, adminID pgtype.UUID) (gen.FederatedActor, error) {
	space, err := s.store.read.GetSpaceForGroupActor(ctx, spaceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.FederatedActor{}, ErrConflict // space missing or not public/active
		}
		return gen.FederatedActor{}, err
	}
	if existing, err := s.store.actorBySpaceID(ctx, space.ID); err == nil {
		switch existing.Status {
		case ActorStatusActive:
			return existing, nil // already provisioned — idempotent
		case ActorStatusDisabled:
			if err := s.store.setActorStatus(ctx, existing.ID, ActorStatusActive); err != nil {
				return gen.FederatedActor{}, err
			}
			s.invalidateGroupDoc(ctx, existing.PreferredUsername)
			existing.Status = ActorStatusActive
			return existing, nil
		default: // tombstone — terminal, never resurrect
			return gen.FederatedActor{}, ErrNotFound
		}
	}
	pubPEM, privPEM, err := generateActorKeyPair()
	if err != nil {
		return gen.FederatedActor{}, fmt.Errorf("federation: group keypair: %w", err)
	}
	privEnc, err := jcrypto.Encrypt(privPEM)
	if err != nil {
		return gen.FederatedActor{}, fmt.Errorf("federation: group key encrypt: %w", err)
	}
	actorURL := s.groupURL(space.Slug)
	a, err := s.store.createActorOfKind(ctx, gen.CreateFederatedActorOfKindParams{
		ActorUrl:          actorURL,
		PreferredUsername: space.Slug,
		PublicKeyPem:      pubPEM,
		PrivateKeyEnc:     privEnc,
		PublicKeyID:       actorURL + "#main-key-" + uuid.NewString(),
		InboxUrl:          s.apiBase() + "/federation/inbox",
		OutboxUrl:         actorURL + "/outbox",
		FollowersUrl:      actorURL + "/followers",
		FollowingUrl:      actorURL + "/following",
		Kind:              ActorKindGroup,
		SpaceID:           space.ID,
	})
	if err != nil {
		// Unique race on space_id — return the winner.
		if existing, rerr := s.store.actorBySpaceID(ctx, space.ID); rerr == nil {
			return existing, nil
		}
		return gen.FederatedActor{}, fmt.Errorf("federation: group actor create: %w", err)
	}
	return a, nil
}

// TombstoneGroupActor terminally disables a Group actor and fans out
// Delete{Group} — admin path DELETE /admin/federation/groups/{id}.
func (s *Service) TombstoneGroupActor(ctx context.Context, actorID pgtype.UUID) error {
	actor, err := s.store.actorByID(ctx, actorID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if actor.Kind != ActorKindGroup {
		return ErrNotFound
	}
	if actor.Status == ActorStatusTombstone {
		return nil // already terminal — idempotent
	}
	if err := s.store.setActorStatus(ctx, actor.ID, ActorStatusTombstone); err != nil {
		return err
	}
	s.invalidateGroupDoc(ctx, actor.PreferredUsername)
	return s.fanoutActorDeleteErr(ctx, actor.ID)
}

// fanoutActorDeleteErr wraps the delete fanout for the group path.
func (s *Service) fanoutActorDeleteErr(ctx context.Context, actorID pgtype.UUID) error {
	_, err := s.fanoutActorDelete(ctx, actorID, true)
	return err
}

// ListGroupActors is the admin DataTable source for provisioned groups.
func (s *Service) ListGroupActors(ctx context.Context, limit, offset int32) ([]gen.ListGroupActorsRow, int64, error) {
	rows, err := s.store.read.ListGroupActors(ctx, gen.ListGroupActorsParams{PageLimit: limit, PageOffset: offset})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.store.read.CountGroupActors(ctx)
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// groupActorFromRow flattens the doc-join row back to the actor row.
func groupActorFromRow(r gen.GetGroupActorDocBySlugRow) gen.FederatedActor {
	return gen.FederatedActor{
		ID: r.ID, UserID: r.UserID, ActorUrl: r.ActorUrl,
		PreferredUsername: r.PreferredUsername, PublicKeyPem: r.PublicKeyPem,
		PrivateKeyEnc: r.PrivateKeyEnc, PublicKeyID: r.PublicKeyID,
		Status: r.Status, InboxUrl: r.InboxUrl, OutboxUrl: r.OutboxUrl,
		FollowersUrl: r.FollowersUrl, FollowingUrl: r.FollowingUrl,
		ManuallyApprovesFollowers: r.ManuallyApprovesFollowers,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		KeyRotatedAt: r.KeyRotatedAt, Kind: r.Kind, SpaceID: r.SpaceID,
	}
}

// groupDocRow resolves slug → the actor+space join row with the
// space-liveness gates the public surface requires.
func (s *Service) groupDocRow(ctx context.Context, slug string) (gen.GetGroupActorDocBySlugRow, error) {
	row, err := s.store.read.GetGroupActorDocBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return row, ErrNotFound
		}
		return row, err
	}
	if row.SpaceDeletedAt.Valid || row.SpaceStatus != "active" || row.SpaceVisibility != "public" {
		return row, ErrNotFound // space gone private/inactive — stop resolving
	}
	return row, nil
}

// GroupDocument serves GET /federation/groups/{slug}.
func (s *Service) GroupDocument(ctx context.Context, slug string) (map[string]any, error) {
	cacheKey := "jc:fed:groupdoc:" + strings.ToLower(slug)
	if doc, ok := s.cacheGet(ctx, cacheKey); ok {
		return doc, nil
	}
	row, err := s.groupDocRow(ctx, slug)
	if err != nil {
		return nil, err
	}
	if row.Status == ActorStatusTombstone {
		return nil, ErrGone
	}
	if row.Status != ActorStatusActive {
		return nil, ErrNotFound
	}
	doc := groupDocument(row, s.siteBase())
	s.cacheSet(ctx, cacheKey, doc, 5*time.Minute)
	return doc, nil
}

// GroupTombstone returns the 410 body for a tombstoned group actor —
// mirrors GetActor's Tombstone branch.
func (s *Service) GroupTombstone(ctx context.Context, slug string) map[string]any {
	row, err := s.store.read.GetGroupActorDocBySlug(ctx, slug)
	if err != nil || row.Status != ActorStatusTombstone {
		return nil
	}
	return tombstoneDocument(row.ActorUrl)
}

// invalidateGroupDoc drops the cached Group doc for a slug.
func (s *Service) invalidateGroupDoc(ctx context.Context, slug string) {
	if s.vk == nil || slug == "" {
		return
	}
	_ = s.vk.Del(ctx, "jc:fed:groupdoc:"+strings.ToLower(slug)).Err()
}

// groupDocument builds the Group actor document from the join row.
func groupDocument(row gen.GetGroupActorDocBySlugRow, siteBase string) map[string]any {
	doc := map[string]any{
		"@context":                  asContext,
		"id":                        row.ActorUrl,
		"type":                      "Group",
		"preferredUsername":         row.SpaceSlug,
		"name":                      row.SpaceName,
		"url":                       siteBase + "/spaces/" + row.SpaceSlug,
		"inbox":                     row.InboxUrl,
		"outbox":                    row.OutboxUrl,
		"followers":                 row.FollowersUrl,
		"following":                 row.FollowingUrl,
		"manuallyApprovesFollowers": false,
		"discoverable":              true,
		"published":                 row.CreatedAt.Time.UTC().Format(time.RFC3339),
		"publicKey": map[string]any{
			"id":           row.PublicKeyID,
			"owner":        row.ActorUrl,
			"publicKeyPem": row.PublicKeyPem,
		},
		"endpoints": map[string]any{"sharedInbox": row.InboxUrl},
	}
	if row.SpaceDescription != nil && *row.SpaceDescription != "" {
		doc["summary"] = *row.SpaceDescription
	}
	if row.LogoBucket != nil && row.LogoPath != nil {
		if u := media.URLFromParts(row.LogoBucket, row.LogoPath, media.ImageProxyOpts{}); u != nil {
			icon := map[string]any{"type": "Image", "url": *u}
			if row.LogoMime != nil {
				icon["mediaType"] = *row.LogoMime
			}
			doc["icon"] = icon
		}
	}
	return doc
}

// groupActorForCollection resolves slug → live actor for the collection
// routes (outbox/followers share the space + status gates).
func (s *Service) groupActorForCollection(ctx context.Context, slug string) (gen.GetGroupActorDocBySlugRow, error) {
	row, err := s.groupDocRow(ctx, slug)
	if err != nil {
		return row, err
	}
	if row.Status != ActorStatusActive {
		return row, ErrNotFound
	}
	return row, nil
}

// GroupOutbox serves the space's OrderedCollection — Announce items of
// the space's public content (the FEP-1b12 group relay pattern).
func (s *Service) GroupOutbox(ctx context.Context, slug string, page int) (map[string]any, error) {
	row, err := s.groupActorForCollection(ctx, slug)
	if err != nil {
		return nil, err
	}
	types := s.federatableTypesFor(ctx)
	total, err := s.store.read.CountFederatableContentBySpace(ctx, gen.CountFederatableContentBySpaceParams{
		SpaceID: row.SpaceID, ContentTypes: types,
	})
	if err != nil {
		return nil, fmt.Errorf("federation: group outbox count: %w", err)
	}
	if page <= 0 {
		return orderedCollection(row.OutboxUrl, total, row.OutboxUrl+"?page=1"), nil
	}
	size := s.outboxPageSize(ctx)
	offset := int32(page-1) * size
	pageID := func(p int) string { return fmt.Sprintf("%s?page=%d", row.OutboxUrl, p) }
	if int64(offset) >= total {
		return orderedCollectionPage(pageID(page), row.OutboxUrl, []any{}, total, false, page > 1, pageID, page), nil
	}
	rows, err := s.store.read.ListFederatableContentBySpace(ctx, gen.ListFederatableContentBySpaceParams{
		SpaceID: row.SpaceID, ContentTypes: types, PageLimit: size, PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("federation: group outbox page: %w", err)
	}
	items := make([]any, 0, len(rows))
	for _, c := range rows {
		objID := s.objectURL(c.ID.String())
		items = append(items, map[string]any{
			"id":     fmt.Sprintf("%s#announce/%s", row.ActorUrl, c.ID.String()),
			"type":   "Announce",
			"actor":  row.ActorUrl,
			"object": objID,
			"to":     []string{asPublic},
		})
	}
	return orderedCollectionPage(pageID(page), row.OutboxUrl, items, total,
		int64(page)*int64(size) < total, page > 1, pageID, page), nil
}

// GroupFollowers serves the OrderedCollection of the group's remote
// followers (federation_follows edges on the group actor).
func (s *Service) GroupFollowers(ctx context.Context, slug string, page int) (map[string]any, error) {
	row, err := s.groupActorForCollection(ctx, slug)
	if err != nil {
		return nil, err
	}
	total, err := s.store.countRemoteFollowers(ctx, row.ID)
	if err != nil {
		return nil, fmt.Errorf("federation: group followers count: %w", err)
	}
	if page <= 0 {
		return orderedCollection(row.FollowersUrl, total, row.FollowersUrl+"?page=1"), nil
	}
	size := s.outboxPageSize(ctx)
	offset := int32(page-1) * size
	pageID := func(p int) string { return fmt.Sprintf("%s?page=%d", row.FollowersUrl, p) }
	if int64(offset) >= total {
		return orderedCollectionPage(pageID(page), row.FollowersUrl, []any{}, total, false, page > 1, pageID, page), nil
	}
	urls, err := s.store.remoteFollowerURLs(ctx, row.ID, size, offset)
	if err != nil {
		return nil, fmt.Errorf("federation: group followers: %w", err)
	}
	items := make([]any, 0, len(urls))
	for _, u := range urls {
		items = append(items, u)
	}
	return orderedCollectionPage(pageID(page), row.FollowersUrl, items, total,
		int64(page)*int64(size) < total, page > 1, pageID, page), nil
}

// fanoutGroupAnnounce ledgers a group Announce of a member activity to
// the group's remote followers — the space-reach half of group
// federation. Called only for public content activities (Create/Update).
func (s *Service) fanoutGroupAnnounce(ctx context.Context, spaceID pgtype.UUID, memberActivity map[string]any, contentID pgtype.UUID) {
	if !spaceID.Valid {
		return
	}
	group, err := s.store.actorBySpaceID(ctx, spaceID)
	if err != nil || group.Status != ActorStatusActive {
		return
	}
	inboxes, err := s.store.remoteFollowerInboxes(ctx, group.ID)
	if err != nil || len(inboxes) == 0 {
		return
	}
	stamp := time.Now().Unix()
	apID := fmt.Sprintf("%s#announce/%s/%d", group.ActorUrl, contentID.String(), stamp)
	activity := map[string]any{
		"id":     apID,
		"type":   "Announce",
		"actor":  group.ActorUrl,
		"object": memberActivity,
		"to":     []string{asPublic},
		"cc":     []string{group.FollowersUrl},
	}
	if _, err := s.insertDeliveries(ctx, group.ID, activity, apID, inboxes, true); err != nil {
		slog.Warn("federation: group announce fanout failed", "group_actor_id", group.ID, "error", err)
	}
}

// ---------------------------------------------------------------------------
// Relays (FEP-ae0c, Mastodon-style client)
// ---------------------------------------------------------------------------

// SubscribeRelay upserts a relay subscription and posts Follow{Public}
// from the instance actor to the relay's inbox — admin path
// POST /admin/federation/relays.
func (s *Service) SubscribeRelay(ctx context.Context, actorURL string, adminID pgtype.UUID) (gen.FederationRelay, error) {
	u, err := url.Parse(actorURL)
	if err != nil || !strings.EqualFold(u.Scheme, "https") || u.Hostname() == "" {
		return gen.FederationRelay{}, ErrInvalidDomain
	}
	domain := strings.ToLower(u.Hostname())
	if !s.instanceAllowed(ctx, domain) {
		return gen.FederationRelay{}, ErrRemoteBlocked
	}
	doc, err := s.fetchRemoteActorDoc(ctx, actorURL)
	if err != nil {
		return gen.FederationRelay{}, err
	}
	if doc.Inbox == "" {
		return gen.FederationRelay{}, fmt.Errorf("%w: relay actor advertises no inbox", ErrRemoteFetch)
	}
	remote, err := s.upsertRemoteActor(ctx, actorURL, doc)
	if err != nil {
		return gen.FederationRelay{}, fmt.Errorf("federation: relay actor upsert: %w", err)
	}
	inst, err := s.ensureInstanceActor(ctx)
	if err != nil {
		return gen.FederationRelay{}, err
	}
	if inst.Status != ActorStatusActive {
		return gen.FederationRelay{}, ErrNotFound
	}
	followID := fmt.Sprintf("%s#relay-follow-%s", inst.ActorUrl, uuid.NewString())
	relay, err := s.store.upsertRelay(ctx, gen.UpsertFederationRelayParams{
		ActorUrl: actorURL, InboxUrl: doc.Inbox, Domain: domain,
		FollowApID: &followID, CreatedBy: adminID,
	})
	if err != nil {
		return gen.FederationRelay{}, fmt.Errorf("federation: relay upsert: %w", err)
	}
	// An already-live subscription keeps its pending Follow — a second
	// Follow for the same edge would only confuse relay implementations.
	if relay.Status != "pending" {
		return relay, nil
	}
	inbox := doc.Inbox
	follow := map[string]any{
		"@context": asContext,
		"id":       followID,
		"type":     "Follow",
		"actor":    inst.ActorUrl,
		"object":   asPublic,
		"to":       []string{actorURL},
	}
	row := gen.ListRemoteFollowerInboxesByActorRow{InboxUrl: &inbox, RemoteActorID: remote.ID}
	if _, err := s.insertDeliveries(ctx, inst.ID, follow, followID, []gen.ListRemoteFollowerInboxesByActorRow{row}, true); err != nil {
		return relay, err // relay row is ledgered; the sweeper can retry
	}
	return relay, nil
}

// UnsubscribeRelay sends Undo{Follow} from the instance actor and removes
// the ledger row — DELETE /admin/federation/relays/{id}.
func (s *Service) UnsubscribeRelay(ctx context.Context, relayID pgtype.UUID) error {
	relay, err := s.store.relayByID(ctx, relayID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	inst, err := s.ensureInstanceActor(ctx)
	if err != nil {
		return err
	}
	followID := relay.FollowApID
	if followID != nil && *followID != "" && relay.InboxUrl != "" {
		follow := map[string]any{
			"id":     *followID,
			"type":   "Follow",
			"actor":  inst.ActorUrl,
			"object": asPublic,
		}
		undoID := fmt.Sprintf("%s#relay-unfollow-%s", inst.ActorUrl, uuid.NewString())
		undo := map[string]any{
			"id":     undoID,
			"type":   "Undo",
			"actor":  inst.ActorUrl,
			"object": follow,
			"to":     []string{relay.ActorUrl},
		}
		var remoteID pgtype.UUID
		if remote, rerr := s.store.remoteActorByURL(ctx, relay.ActorUrl); rerr == nil {
			remoteID = remote.ID
		}
		inbox := relay.InboxUrl
		row := gen.ListRemoteFollowerInboxesByActorRow{InboxUrl: &inbox, RemoteActorID: remoteID}
		if _, derr := s.insertDeliveries(ctx, inst.ID, undo, undoID, []gen.ListRemoteFollowerInboxesByActorRow{row}, true); derr != nil {
			slog.Warn("federation: relay undo delivery failed — row still removed", "relay_id", relay.ID, "error", derr)
		}
	}
	return s.store.deleteRelay(ctx, relayID)
}

// ListRelays is the admin DataTable source for relay subscriptions.
func (s *Service) ListRelays(ctx context.Context, limit, offset int32) ([]gen.FederationRelay, int64, error) {
	rows, err := s.store.read.ListFederationRelays(ctx, gen.ListFederationRelaysParams{PageLimit: limit, PageOffset: offset})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.store.read.CountFederationRelays(ctx)
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// isAcceptedRelay reports whether actorURL is a relay with a live
// subscription — inbound courier traffic is only honored from relays we
// actually subscribed to.
func (s *Service) isAcceptedRelay(ctx context.Context, actorURL string) bool {
	relay, err := s.store.read.GetFederationRelayByActorURL(ctx, actorURL)
	return err == nil && relay.Status == "accepted"
}

// relayInboxes expands accepted relays into delivery targets — the
// sharedInbox-coalescing shape insertDeliveries consumes.
func (s *Service) relayInboxes(ctx context.Context) []gen.ListRemoteFollowerInboxesByActorRow {
	urls, err := s.store.read.ListActiveRelayInboxes(ctx)
	if err != nil || len(urls) == 0 {
		return nil
	}
	out := make([]gen.ListRemoteFollowerInboxesByActorRow, 0, len(urls))
	for _, u := range urls {
		u := u
		out = append(out, gen.ListRemoteFollowerInboxesByActorRow{InboxUrl: &u})
	}
	return out
}

// instanceFollowerInboxes expands remote followers of the instance
// actor — remote Follow{instance} subscribers get every public object.
func (s *Service) instanceFollowerInboxes(ctx context.Context) []gen.ListRemoteFollowerInboxesByActorRow {
	inst, err := s.store.actorByKind(ctx, ActorKindApplication)
	if err != nil || inst.Status != ActorStatusActive {
		return nil
	}
	inboxes, err := s.store.remoteFollowerInboxes(ctx, inst.ID)
	if err != nil {
		return nil
	}
	return inboxes
}

// reachInboxes returns the non-edge expansion for public object
// activities: accepted relays + instance-actor followers.
func (s *Service) reachInboxes(ctx context.Context) []gen.ListRemoteFollowerInboxesByActorRow {
	out := s.instanceFollowerInboxes(ctx)
	return append(out, s.relayInboxes(ctx)...)
}

// ---------------------------------------------------------------------------
// Authorized fetch
// ---------------------------------------------------------------------------

// ErrSignatureRequired marks unsigned/unverifiable fetches under
// authorized-fetch mode — the handler answers 401.
var ErrSignatureRequired = errors.New("federation: valid signature required")

// authorizedFetchEnabled reads the §126 gate (default false).
func (s *Service) authorizedFetchEnabled(ctx context.Context) bool {
	if s.settings == nil {
		return false
	}
	b, err := s.settings.GetSettingBool(ctx, "federation.authorized_fetch")
	return err == nil && b
}

// VerifyFetchSignature enforces authorized-fetch on a public GET: parse
// keyId → resolve signer → verify the request signature. A nil error
// means the fetch is authorized.
func (s *Service) VerifyFetchSignature(r *http.Request) error {
	ctx := r.Context()
	if !s.authorizedFetchEnabled(ctx) {
		return nil
	}
	params := ParseSignatureHeader(r.Header.Get("Signature"))
	keyID := params["keyId"]
	if keyID == "" {
		return ErrSignatureRequired
	}
	signer, err := s.resolveKeyActor(ctx, keyID)
	if err != nil {
		if errors.Is(err, ErrRemoteBlocked) {
			return ErrRemoteBlocked
		}
		return ErrSignatureRequired
	}
	if signer.PublicKeyPem == nil || *signer.PublicKeyPem == "" {
		return ErrSignatureRequired
	}
	pub, err := parsePublicKeyPEM(*signer.PublicKeyPem)
	if err != nil {
		return ErrSignatureRequired
	}
	if VerifySignature(r, pub, nil) != nil {
		return ErrSignatureRequired
	}
	return nil
}

// signOutboundFetch signs a remote AP GET with the instance actor key —
// peers running authorized fetch answer only signed document requests.
// Best-effort: any failure leaves the request unsigned rather than
// breaking the fetch path.
func (s *Service) signOutboundFetch(ctx context.Context, req *http.Request) {
	if s.store == nil {
		return
	}
	inst, err := s.ensureInstanceActor(ctx)
	if err != nil || inst.Status != ActorStatusActive {
		return
	}
	privPEM, err := jcrypto.Decrypt(inst.PrivateKeyEnc)
	if err != nil {
		return
	}
	priv, err := parsePrivateKeyPEM(privPEM)
	if err != nil {
		return
	}
	if err := SignRequest(req, inst.PublicKeyID, priv, nil); err != nil {
		slog.Warn("federation: outbound fetch signing failed", "error", err)
	}
}

// ---------------------------------------------------------------------------
// Hashtags
// ---------------------------------------------------------------------------

// decorateTags adds the tag[] Hashtag entries (§126 M3) to an object
// document — sourced from content_tags; failures are silent (a missing
// tag array never blocks the object).
func (s *Service) decorateTags(ctx context.Context, doc map[string]any, contentID pgtype.UUID) {
	if s.store == nil {
		return
	}
	rows, err := s.store.read.ListContentHashtags(ctx, contentID)
	if err != nil || len(rows) == 0 {
		return
	}
	tags := make([]any, 0, len(rows))
	for _, t := range rows {
		tags = append(tags, map[string]any{
			"type": "Hashtag",
			"name": "#" + t.Name,
			"href": s.siteBase() + "/tags/" + t.Slug,
		})
	}
	doc["tag"] = tags
}


