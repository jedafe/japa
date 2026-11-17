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
		return s.repairActorURLs(ctx, a)
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
			return s.repairActorURLs(ctx, a2)
		}
		return gen.FederatedActor{}, fmt.Errorf("federation: instance actor create: %w", err)
	}
	return a, nil
}

// repairActorURLs self-heals rows minted while the API base URL was
// unconfigured (legacy dev data, base-url changes): a relative @id makes
// every derived id — activity ap_ids, the key id — unresolvable to
// remotes. Every relative URL column re-bases onto the configured API
// base (path preserved, so application/person/group URL families all
// heal — shared inboxes stay shared); a relative public_key_id re-bases
// the same way keeping its fragment, so the RSA pair is untouched.
// Rows pass through untouched when the actor_url is already absolute
// or when no absolute base is configured to repair to (a rewrite that
// changes nothing is a write per read). Cached docs drop so the repair
// serves immediately.
func (s *Service) repairActorURLs(ctx context.Context, a gen.FederatedActor) (gen.FederatedActor, error) {
	base := s.apiBase()
	absolute := func(u string) bool {
		return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
	}
	if absolute(a.ActorUrl) || !absolute(base) {
		return a, nil
	}
	rebase := func(u string) string {
		if u == "" || absolute(u) {
			return u
		}
		if !strings.HasPrefix(u, "/") {
			u = "/" + u
		}
		return base + u
	}
	row, err := s.store.write.UpdateFederatedActorURLs(ctx, gen.UpdateFederatedActorURLsParams{
		ID:           a.ID,
		ActorUrl:     rebase(a.ActorUrl),
		PublicKeyID:  rebase(a.PublicKeyID),
		InboxUrl:     rebase(a.InboxUrl),
		OutboxUrl:    rebase(a.OutboxUrl),
		FollowersUrl: rebase(a.FollowersUrl),
		FollowingUrl: rebase(a.FollowingUrl),
	})
	if err != nil {
		return gen.FederatedActor{}, fmt.Errorf("federation: actor url repair: %w", err)
	}
	if a.Kind == ActorKindApplication && s.vk != nil {
		_ = s.vk.Del(ctx, "jc:fed:instance:doc").Err()
	}
	s.invalidateActorCache(ctx, a.PreferredUsername)
	slog.Info("federation: actor urls repaired", "kind", a.Kind, "actor_url", row.ActorUrl)
	return row, nil
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
// public federatable object surfaced as Announce{object} items by the
// instance actor (relay/crawl backfill — the member's own Create rides
// their personal outbox; here the instance boosts).
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
		"cc":     []string{s.instanceURL() + "/followers"},
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
			return gen.FederatedActor{}, ErrConflict // space missing, not member-created, or not public/active
		}
		return gen.FederatedActor{}, err
	}
	if existing, err := s.store.actorBySpaceID(ctx, space.ID); err == nil {
		switch existing.Status {
		case ActorStatusActive:
			return s.repairActorURLs(ctx, existing) // already provisioned — idempotent (lazy URL heal)
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
	// Handle collision: another space's group actor already owns the
	// !{slug} handle — e.g. the slug was renamed away from an earlier
	// federated space and reused (the handle stays bound to its original
	// actor). Minting would only fail on the actor_url unique index.
	if other, err := s.store.read.GetGroupActorDocBySlug(ctx, space.Slug); err == nil && other.SpaceID != space.ID {
		return gen.FederatedActor{}, ErrConflict
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

// ListGroupActors is the admin DataTable source for provisioned groups —
// `?q=` narrows on space slug/name or actor URL (ILIKE).
func (s *Service) ListGroupActors(ctx context.Context, q string, limit, offset int32) ([]gen.ListGroupActorsRow, int64, error) {
	rows, err := s.store.read.ListGroupActors(ctx, gen.ListGroupActorsParams{Q: q, PageLimit: limit, PageOffset: offset})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.store.read.CountGroupActors(ctx, q)
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
		CreatedAt:                 r.CreatedAt, UpdatedAt: r.UpdatedAt,
		KeyRotatedAt: r.KeyRotatedAt, Kind: r.Kind, SpaceID: r.SpaceID,
	}
}

// groupDocRow resolves slug → the actor+space join row gated by the
// member-created/public/active envelope the public surface requires.
// spaceGroupEligible is the §126 member-created/public/active envelope a
// space must hold for its Group actor to resolve — doc-serving, collection
// reads, and inbound Follow all share it. Mirrors the provisioning gate in
// GetSpaceForGroupActor: an erased creator drops creator_user_id to NULL
// and the space leaves the envelope, stopping resolution like a
// flipped-private space.
func spaceGroupEligible(row gen.GetGroupActorDocBySlugRow) bool {
	return !row.SpaceDeletedAt.Valid &&
		row.SpaceStatus == "active" &&
		row.SpaceVisibility == "public" &&
		row.SpaceLevel == "custom" &&
		row.SpaceCreatorUserID.Valid
}

func (s *Service) groupDocRow(ctx context.Context, slug string) (gen.GetGroupActorDocBySlugRow, error) {
	row, err := s.store.read.GetGroupActorDocBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return row, ErrNotFound
		}
		return row, err
	}
	if !spaceGroupEligible(row) {
		return row, ErrNotFound // space left the member-created/public/active envelope
	}
	return row, nil
}

// GroupDocument serves GET /federation/groups/{slug}. The space
// envelope gate runs live on EVERY request — the cache may only elide
// the doc build, never the eligibility join: a flipped-private (or
// deactivated, deleted, creator-erased) space must stop resolving
// immediately, not at the 5-minute TTL.
func (s *Service) GroupDocument(ctx context.Context, slug string) (map[string]any, error) {
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
	cacheKey := "jc:fed:groupdoc:" + strings.ToLower(slug)
	if doc, ok := s.cacheGet(ctx, cacheKey); ok {
		return doc, nil
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

// invalidateGroupDoc drops the cached Group doc AND its WebFinger JRD —
// a tombstoned/eligibility-lost group must stop resolving on both
// surfaces immediately, not after the 5-minute TTL (the person path
// likewise drops actor + webfinger keys together).
func (s *Service) invalidateGroupDoc(ctx context.Context, slug string) {
	if s.vk == nil || slug == "" {
		return
	}
	lower := strings.ToLower(slug)
	// Two single-key DELs — the compat adapter rejects multi-key DEL
	// across hash slots.
	_ = s.vk.Del(ctx, "jc:fed:groupdoc:"+lower).Err()
	_ = s.vk.Del(ctx, "jc:fed:webfinger:!"+lower).Err()
}

// groupDocument builds the Group actor document from the join row.
func groupDocument(row gen.GetGroupActorDocBySlugRow, siteBase string) map[string]any {
	doc := map[string]any{
		"@context": asContext,
		"id":       row.ActorUrl,
		"type":     "Group",
		// The handle is the actor's provision-time preferred_username —
		// immutable like actor_url — so a space slug rename never
		// advertises an unresolvable acct:!{slug}. `url` deliberately
		// tracks the LIVE space slug (the space's current page).
		"preferredUsername":         row.PreferredUsername,
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
		// §126 M5: pinned-content + hashtag collections, same shape the
		// Person doc advertises.
		"featured":     row.ActorUrl + "/collections/featured",
		"featuredTags": row.ActorUrl + "/collections/tags",
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
// routes (outbox/followers share the space + status gates). Tombstone
// answers 410 like the doc route — "gone", not "never existed".
func (s *Service) groupActorForCollection(ctx context.Context, slug string) (gen.GetGroupActorDocBySlugRow, error) {
	row, err := s.groupDocRow(ctx, slug)
	if err != nil {
		return row, err
	}
	if row.Status == ActorStatusTombstone {
		return row, ErrGone
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
			"cc":     []string{row.FollowersUrl},
		})
	}
	return orderedCollectionPage(pageID(page), row.OutboxUrl, items, total,
		int64(page)*int64(size) < total, page > 1, pageID, page), nil
}

// GroupFeatured serves the space's pinned-content collection (§126 M5):
// the group outbox eligibility predicate plus is_pinned, rendered as
// embedded object documents under the author's Person actor.
func (s *Service) GroupFeatured(ctx context.Context, slug string) (map[string]any, error) {
	row, err := s.groupActorForCollection(ctx, slug)
	if err != nil {
		return nil, err
	}
	rows, err := s.store.read.ListFeaturedFederatableContentBySpace(ctx,
		gen.ListFeaturedFederatableContentBySpaceParams{
			SpaceID: row.SpaceID, ContentTypes: s.federatableTypesFor(ctx), PageLimit: s.outboxPageSize(ctx),
		})
	if err != nil {
		return nil, fmt.Errorf("federation: group featured items: %w", err)
	}
	items := make([]any, 0, len(rows))
	for _, r := range rows {
		items = append(items, s.objectFromGetRow(ctx, gen.GetFederatableContentByIDRow{
			ID:             r.ID,
			ContentType:    r.ContentType,
			Title:          r.Title,
			Slug:           r.Slug,
			BodyHtml:       r.BodyHtml,
			CanonicalUrl:   r.CanonicalUrl,
			PublishedAt:    r.PublishedAt,
			UpdatedAt:      r.UpdatedAt,
			FeaturedBucket: r.FeaturedBucket,
			FeaturedPath:   r.FeaturedPath,
			FeaturedMime:   r.FeaturedMime,
			HeaderBucket:   r.HeaderBucket,
			HeaderPath:     r.HeaderPath,
			HeaderMime:     r.HeaderMime,
		}, gen.FederatedActor{ActorUrl: r.AuthorActorUrl, FollowersUrl: r.AuthorFollowersUrl}))
	}
	return map[string]any{
		"@context":     asContext[0],
		"id":           row.ActorUrl + "/collections/featured",
		"type":         "OrderedCollection",
		"totalItems":   len(items),
		"orderedItems": items,
	}, nil
}

// GroupFeaturedTags serves the space's hashtag collection (§126 M5) —
// tags used across the space's eligible content set.
func (s *Service) GroupFeaturedTags(ctx context.Context, slug string) (map[string]any, error) {
	row, err := s.groupActorForCollection(ctx, slug)
	if err != nil {
		return nil, err
	}
	tags, err := s.store.read.ListSpaceFederatedTags(ctx, gen.ListSpaceFederatedTagsParams{
		SpaceID: row.SpaceID, ContentTypes: s.federatableTypesFor(ctx), PageLimit: s.outboxPageSize(ctx),
	})
	if err != nil {
		return nil, fmt.Errorf("federation: group featuredTags items: %w", err)
	}
	items := make([]any, 0, len(tags))
	for _, t := range tags {
		items = append(items, map[string]any{
			"type": "Hashtag",
			"name": "#" + t.Name,
			"href": s.siteBase() + "/tags/" + t.Slug,
		})
	}
	return map[string]any{
		"@context":     asContext[0],
		"id":           row.ActorUrl + "/collections/tags",
		"type":         "OrderedCollection",
		"totalItems":   len(items),
		"orderedItems": items,
	}, nil
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
// The announce must not fire when the space has left the
// member-created/public/active envelope — the actor URL embedded in it
// would 404 on the remote's actor fetch (the gate runs live; nothing
// transitions federated_actors.status on a space mutation).
func (s *Service) fanoutGroupAnnounce(ctx context.Context, spaceID pgtype.UUID, memberActivity map[string]any, contentID pgtype.UUID) {
	if !spaceID.Valid {
		return
	}
	group, err := s.store.actorBySpaceID(ctx, spaceID)
	if err != nil {
		return
	}
	// groupDocRow is the shared slug-keyed gate — spaceGroupEligible +
	// the actor row in one join.
	row, err := s.groupDocRow(ctx, group.PreferredUsername)
	if err != nil || row.Status != ActorStatusActive {
		return
	}
	inboxes, err := s.store.remoteFollowerInboxes(ctx, row.ID)
	if err != nil || len(inboxes) == 0 {
		return
	}
	stamp := time.Now().Unix()
	apID := fmt.Sprintf("%s#announce/%s/%d", row.ActorUrl, contentID.String(), stamp)
	activity := map[string]any{
		"id":     apID,
		"type":   "Announce",
		"actor":  row.ActorUrl,
		"object": memberActivity,
		"to":     []string{asPublic},
		"cc":     []string{row.FollowersUrl},
	}
	if _, err := s.insertDeliveries(ctx, row.ID, activity, apID, inboxes, true); err != nil {
		slog.Warn("federation: group announce fanout failed", "group_actor_id", row.ID, "error", err)
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
	// Deliveries target sharedInbox when advertised — the relay's
	// coalesced delivery point (same preference ListRemoteFollowerInboxes
	// applies to followers).
	inbox := doc.Endpoints.SharedInbox
	if inbox == "" {
		inbox = doc.Inbox
	}
	if inbox == "" {
		return gen.FederationRelay{}, fmt.Errorf("%w: relay actor advertises no inbox", ErrRemoteFetch)
	}
	// The inbox is remote-advertised data — an http/garbage URL can never
	// take a signed delivery (deliverAP is https-only), so fail at
	// subscribe rather than ledger a pending row that must dead-letter
	// before an admin learns it can't work.
	iu, ierr := url.Parse(inbox)
	if ierr != nil || !strings.EqualFold(iu.Scheme, "https") || iu.Hostname() == "" {
		return gen.FederationRelay{}, fmt.Errorf("%w: relay actor advertises unusable inbox", ErrRemoteFetch)
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
		ActorUrl: actorURL, InboxUrl: inbox, Domain: domain,
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
	// The activity id must be the LEDGERED follow_ap_id — a re-subscribe
	// while pending keeps the original id in the row, so minting a fresh
	// one here would let the remote answer a Follow we can't match.
	// (Rejected/failed rows get a NEW id from the upsert's CASE arm.)
	apID := followID
	if relay.FollowApID != nil && *relay.FollowApID != "" {
		apID = *relay.FollowApID
	}
	follow := map[string]any{
		"@context": asContext,
		"id":       apID,
		"type":     "Follow",
		"actor":    inst.ActorUrl,
		"object":   asPublic,
		"to":       []string{actorURL},
	}
	row := gen.ListRemoteFollowerInboxesByActorRow{InboxUrl: &inbox, RemoteActorID: remote.ID}
	if _, err := s.insertDeliveries(ctx, inst.ID, follow, apID, []gen.ListRemoteFollowerInboxesByActorRow{row}, true); err != nil {
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

// ListRelays is the admin DataTable source for relay subscriptions —
// `?status=` narrows to one subscription state (empty = all), `?q=`
// ILIKE-filters actor_url/domain.
func (s *Service) ListRelays(ctx context.Context, q, status string, limit, offset int32) ([]gen.FederationRelay, int64, error) {
	rows, err := s.store.read.ListFederationRelays(ctx, gen.ListFederationRelaysParams{Q: q, Status: status, PageLimit: limit, PageOffset: offset})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.store.read.CountFederationRelays(ctx, gen.CountFederationRelaysParams{Q: q, Status: status})
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

// markRelayUnreachable unwinds a live relay whose inbox can no longer
// be delivered to — the §126 "becomes unreachable" arm. Relay
// deliveries carry no remote_actor_id (reach expansion adds bare inbox
// URLs), so the match is on the subscribed inbox_url — a sharedInbox
// coalesces every subscription on a host anyway, so a dead shared
// endpoint marks all of the host's relays (all are genuinely
// undeliverable there). endpointLevel distinguishes causes: only
// endpoint-level dead letters (inbox gone, unusable URL, exhausted
// retries, blocked destination) unwind an accepted row — a plain 4xx
// is the remote refusing THAT activity, not going away. Pending rows
// unwind whenever their own Follow dead-letters (activityApID ==
// follow_ap_id — a refused handshake is a failed handshake) but never
// on unrelated inbox traffic. Best-effort: a missed mark only means
// future fanout burns a few more retry budgets until an admin
// resubscribes or a Delete{actor} lands.
func (s *Service) markRelayUnreachable(ctx context.Context, row gen.FederationDelivery, reason string, endpointLevel bool) {
	if s.store == nil || row.InboxUrl == "" {
		return
	}
	if err := s.store.markRelayFailedByInbox(ctx, row.InboxUrl, row.ActivityApID, reason, endpointLevel); err != nil {
		slog.Warn("federation: relay unreachable mark failed", "inbox", row.InboxUrl, "error", err)
	}
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
