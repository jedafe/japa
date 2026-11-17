package federation

// Ref: japaconnect-engineering/sections/126-activitypub-federation.md (M5)
// Remote-post ingest + the previously-unscoped verbs. federation_remote_posts
// only stores objects from remote actors a member actively follows — the
// outbound edge is the sole ingest warrant, so the timeline stays bounded
// by construction. Content lands as plain text (the remote-comment
// posture): remote markup never reaches our renderers.

import (
	"context"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	gen "github.com/jedafe/japa/internal/db/gen"
)

// maxRemotePostHashtags bounds the tag[] extraction on ingest.
const maxRemotePostHashtags = 20

// maxRemotePostField bounds a stored remote post's name/summary (runes) —
// remote-advertised data gets a ceiling like every other bounded field.
const maxRemotePostField = 2000

// remotePostFields flattens an AP object doc into the columns
// federation_remote_posts stores — shared by Create ingest and the
// Update refresh so both paths map fields identically.
type remotePostFields struct {
	ContentText string
	Name        *string
	Summary     *string
	URL         *string
	Sensitive   bool
	Hashtags    []string
	PublishedAt pgtype.Timestamptz
}

func remotePostFieldsFromObject(obj map[string]any) remotePostFields {
	f := remotePostFields{
		ContentText: htmlToText(apStr(obj["content"])),
		Name:        stringPtrOrNil(runeCap(apStr(obj["name"]), maxRemotePostField)),
		Summary:     stringPtrOrNil(runeCap(apStr(obj["summary"]), maxRemotePostField)),
		Hashtags:    remotePostHashtags(obj),
	}
	if s, _ := obj["sensitive"].(bool); s {
		f.Sensitive = true
	}
	// url is a bare IRI or a {type:Link, href} doc — remote-advertised
	// data, so only https may ride into the stored row.
	u := apStr(obj["url"])
	if u == "" {
		if o := apObj(obj["url"]); o != nil {
			u = apStr(o["href"])
		}
	}
	if pu, err := url.Parse(u); err == nil && strings.EqualFold(pu.Scheme, "https") && pu.Hostname() != "" {
		f.URL = &u
	}
	if t, err := time.Parse(time.RFC3339, apStr(obj["published"])); err == nil {
		// A remote can claim any published stamp — a far-future date
		// would pin the post atop every follower's timeline (the
		// keyset sorts on it). Clamp to receipt time.
		if now := time.Now().UTC(); t.After(now) {
			t = now
		}
		f.PublishedAt = pgtype.Timestamptz{Time: t.UTC(), Valid: true}
	}
	return f
}

// runeCap truncates s at n runes — byte-slicing could split a multibyte
// rune and make Postgres reject the insert.
func runeCap(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// remotePostHashtags extracts "#tag" names from the object's tag[] —
// deduped, '#'-stripped, bounded.
func remotePostHashtags(obj map[string]any) []string {
	raw, _ := obj["tag"].([]any)
	seen := map[string]bool{}
	var tags []string
	for _, t := range raw {
		m := apObj(t)
		if m == nil || !strings.EqualFold(apStr(m["type"]), "Hashtag") {
			continue
		}
		name := strings.TrimPrefix(apStr(m["name"]), "#")
		if name == "" || seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		tags = append(tags, name)
		if len(tags) >= maxRemotePostHashtags {
			break
		}
	}
	if tags == nil {
		return []string{}
	}
	return tags
}

// storeRemotePost is the dispatchCreate arm for objects that anchor no
// local thread — top-level posts and replies into remote threads. The
// ingest warrant is twofold (§126 tag discovery): the signer has ≥1
// accepted outbound follower, OR the object's hashtags meet a member's
// live tag subscription on the signer's domain. Everything else stays
// ignored.
func (s *Service) storeRemotePost(ctx context.Context, q *gen.Queries, obj map[string]any, remoteURI, replyTo, body string, signer *gen.RemoteActor) dispatchResult {
	f := remotePostFieldsFromObject(obj)
	warrant, err := q.ExistsAcceptedOutboundFollowerOfRemote(ctx, signer.ID)
	if err != nil {
		return dispatchResult{status: "error", reason: "ingest warrant: " + err.Error()}
	}
	if !warrant {
		warrant, err = q.ExistsTagSubscriptionForPost(ctx, gen.ExistsTagSubscriptionForPostParams{
			Domain:   signer.Domain,
			Hashtags: f.Hashtags,
		})
		if err != nil {
			return dispatchResult{status: "error", reason: "tag ingest warrant: " + err.Error()}
		}
		if warrant {
			slog.Info("federation: remote post ingested via tag subscription",
				"actor", signer.ActorUrl, "tags", f.Hashtags)
		}
	}
	if !warrant {
		return dispatchResult{status: "ignored", reason: "author not followed by any member"}
	}
	if f.ContentText == "" && body != "" {
		f.ContentText = body
	}
	if f.ContentText == "" {
		return dispatchResult{status: "rejected", reason: "empty remote post"}
	}
	if len(f.ContentText) > maxRemoteCommentText {
		return dispatchResult{status: "rejected", reason: "remote post text too large"}
	}
	params := gen.InsertFederationRemotePostParams{
		RemoteActorID: signer.ID,
		ApID:          remoteURI,
		Url:           f.URL,
		Name:          f.Name,
		Summary:       f.Summary,
		ContentText:   f.ContentText,
		Sensitive:     f.Sensitive,
		Hashtags:      f.Hashtags,
		PublishedAt:   f.PublishedAt,
	}
	// inReplyTo is only stored for remote parents — local anchors went
	// the comment path instead.
	if replyTo != "" && !strings.HasPrefix(replyTo, s.apiBase()) {
		params.InReplyToUrl = &replyTo
	}
	n, err := q.InsertFederationRemotePost(ctx, params)
	if err != nil {
		return dispatchResult{status: "error", reason: "remote post insert: " + err.Error()}
	}
	if n == 0 {
		return dispatchResult{status: "ignored", reason: "remote post replayed"}
	}
	return dispatchResult{status: "processed"}
}

// updateRemotePost is the dispatchUpdate arm for stored timeline posts —
// provenance-bound: only the author can rewrite their own row.
func (s *Service) updateRemotePost(ctx context.Context, q *gen.Queries, objectID string, obj map[string]any, signer *gen.RemoteActor) dispatchResult {
	post, err := q.GetFederationRemotePostByApID(ctx, objectID)
	if err != nil {
		return dispatchResult{status: "ignored", reason: "updated object unknown"}
	}
	if post.RemoteActorID != signer.ID {
		return dispatchResult{status: "rejected", reason: "update of another actor's object"}
	}
	f := remotePostFieldsFromObject(obj)
	if f.ContentText == "" || len(f.ContentText) > maxRemoteCommentText {
		return dispatchResult{status: "rejected", reason: "bad updated text"}
	}
	edited := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	if t, terr := time.Parse(time.RFC3339, apStr(obj["updated"])); terr == nil {
		edited = pgtype.Timestamptz{Time: t.UTC(), Valid: true}
	}
	n, err := q.UpdateFederationRemotePost(ctx, gen.UpdateFederationRemotePostParams{
		ApID:          objectID,
		RemoteActorID: signer.ID,
		Name:          f.Name,
		Summary:       f.Summary,
		ContentText:   f.ContentText,
		Sensitive:     f.Sensitive,
		Hashtags:      f.Hashtags,
		Url:           f.URL,
		EditedAt:      edited,
	})
	if err != nil {
		return dispatchResult{status: "error", reason: "remote post update: " + err.Error()}
	}
	if n == 0 {
		return dispatchResult{status: "ignored", reason: "updated object tombstoned"}
	}
	return dispatchResult{status: "processed"}
}

// tombstoneRemotePost is the shared Delete{object}/Undo{Create} arm —
// provenance-checked exactly like the comment tombstone.
func (s *Service) tombstoneRemotePost(ctx context.Context, q *gen.Queries, objectID string, signer *gen.RemoteActor) dispatchResult {
	post, err := q.GetFederationRemotePostByApID(ctx, objectID)
	if err != nil {
		return dispatchResult{status: "ignored", reason: "delete target unknown"}
	}
	if post.RemoteActorID != signer.ID {
		return dispatchResult{status: "rejected", reason: "delete of another actor's object"}
	}
	n, err := q.TombstoneFederationRemotePost(ctx, gen.TombstoneFederationRemotePostParams{
		ApID: objectID, RemoteActorID: signer.ID,
	})
	if err != nil {
		return dispatchResult{status: "error", reason: "remote post tombstone: " + err.Error()}
	}
	if n == 0 {
		return dispatchResult{status: "ignored", reason: "remote post already tombstoned"}
	}
	return dispatchResult{status: "processed"}
}

// dispatchFlag lands a remote moderation report (§126 M5): each object
// IRI resolves to a local entity — content (our object URL, ungated:
// reports land regardless of eligibility), user (our actor URL), or
// comment (an ingested remote reply). The reporter carries
// remote_actor_id provenance; the partial unique dedupes replays.
func (s *Service) dispatchFlag(ctx context.Context, q *gen.Queries, act map[string]any, signer *gen.RemoteActor) dispatchResult {
	if signer == nil {
		return dispatchResult{status: "rejected", reason: "unsigned flag"}
	}
	notes := runeCap(apStr(act["content"]), 2000)
	inserted := int64(0)
	for _, targetID := range apIDList(act["object"]) {
		entityType, entityID, ok := s.flagTargetEntity(ctx, q, targetID)
		if !ok {
			continue
		}
		n, err := q.InsertRemoteContentReport(ctx, gen.InsertRemoteContentReportParams{
			ReporterRemoteActorID: signer.ID,
			EntityType:            entityType,
			EntityID:              entityID,
			Notes:                 stringPtrOrNil(notes),
		})
		if err != nil {
			return dispatchResult{status: "error", reason: "report insert: " + err.Error()}
		}
		inserted += n
	}
	if inserted == 0 {
		return dispatchResult{status: "ignored", reason: "no resolvable flag target"}
	}
	return dispatchResult{status: "processed"}
}

// apIDList normalizes object references — Flag carries an IRI, an array
// of IRIs, or embedded objects.
func apIDList(v any) []string {
	switch o := v.(type) {
	case string:
		if o == "" {
			return nil
		}
		return []string{o}
	case map[string]any:
		if id := apStr(o["id"]); id != "" {
			return []string{id}
		}
	case []any:
		var out []string
		for _, item := range o {
			if id := apID(item); id != "" {
				out = append(out, id)
			}
		}
		return out
	}
	return nil
}

// flagTargetEntity maps one Flag object IRI onto a content_reports
// entity — content (our object URL), user (our actor URL), comment (an
// ingested remote reply's remote_uri). Anything else isn't ours to
// report on.
func (s *Service) flagTargetEntity(ctx context.Context, q *gen.Queries, targetID string) (string, pgtype.UUID, bool) {
	if contentID, ok := s.contentIDForObject(targetID); ok {
		return "content", contentID, true
	}
	if username, ok := strings.CutPrefix(targetID, s.apiBase()+"/federation/actors/"); ok &&
		username != "" && !strings.Contains(username, "/") {
		user, err := q.GetUserByUsername(ctx, username)
		if err != nil {
			return "", pgtype.UUID{}, false
		}
		return "user", user.ID, true
	}
	if c, err := q.GetAtprotoCommentByRemoteURI(ctx, &targetID); err == nil && c.ID.Valid && !c.DeletedAt.Valid {
		return "comment", c.ID, true
	}
	// A Group actor URL flags the space itself.
	if slug, ok := strings.CutPrefix(targetID, s.apiBase()+"/federation/groups/"); ok &&
		slug != "" && !strings.Contains(slug, "/") {
		row, err := s.store.read.GetGroupActorDocBySlug(ctx, slug)
		if err != nil || !row.SpaceID.Valid {
			return "", pgtype.UUID{}, false
		}
		return "space", row.SpaceID, true
	}
	return "", pgtype.UUID{}, false
}

// dispatchBlock severs the relationship between the signing remote
// actor and the local actor they name — Mastodon parity: their inbound
// edges to us die (edge + mirror + follower_count) AND our member's
// outbound edges to them die (you can't keep following someone who
// blocked you — their instance will reject the next re-request anyway).
func (s *Service) dispatchBlock(ctx context.Context, q *gen.Queries, act map[string]any, signer *gen.RemoteActor) dispatchResult {
	if signer == nil {
		return dispatchResult{status: "rejected", reason: "unsigned block"}
	}
	actor, _, err := s.localActorForObject(ctx, q, apObjectID(act))
	if err != nil {
		return dispatchResult{status: "ignored", reason: "block target is not a local actor"}
	}
	// Inbound half — their follow edges to the blocked actor die with
	// the follows mirror and its follower_count share.
	if err := q.UpdateFederationFollowStatus(ctx, gen.UpdateFederationFollowStatusParams{
		LocalActorID: actor.ID, RemoteActorID: signer.ID,
		Direction: "inbound", Status: "undone",
	}); err != nil {
		return dispatchResult{status: "error", reason: "block inbound unwind: " + err.Error()}
	}
	userID := followUserID(ctx, q, actor.ID)
	if userID.Valid {
		if n, derr := q.DeleteRemoteFollow(ctx, gen.DeleteRemoteFollowParams{
			RemoteActorID: signer.ID, UserID: userID,
		}); derr != nil {
			slog.Warn("federation: block follows unwind failed", "error", derr)
		} else if n > 0 {
			s.bumpCounter(q.AdjustUserFollowerCount(ctx, gen.AdjustUserFollowerCountParams{
				UserID: userID, Delta: -1,
			}))
		}
	}
	// Outbound half — the member's follow of the blocker dies with its
	// remote_actor mirror.
	if err := q.UpdateFederationFollowStatus(ctx, gen.UpdateFederationFollowStatusParams{
		LocalActorID: actor.ID, RemoteActorID: signer.ID,
		Direction: "outbound", Status: "undone",
	}); err != nil {
		return dispatchResult{status: "error", reason: "block outbound unwind: " + err.Error()}
	}
	if userID.Valid {
		if _, derr := q.DeleteOutboundFollowMirror(ctx, gen.DeleteOutboundFollowMirrorParams{
			FollowerID: userID, RemoteActorID: signer.ID,
		}); derr != nil {
			slog.Warn("federation: block outbound mirror delete failed", "error", derr)
		}
	}
	return dispatchResult{status: "processed"}
}

// shareRemotePost is the dispatchAnnounce arm for stored remote posts —
// one boost row per (post, remote actor), share_count riding rows-affected.
func (s *Service) shareRemotePost(ctx context.Context, q *gen.Queries, act map[string]any, signer *gen.RemoteActor) dispatchResult {
	post, err := q.GetFederationRemotePostByApID(ctx, apObjectID(act))
	if err != nil || post.DeletedAt.Valid {
		return dispatchResult{status: "ignored", reason: "announce target is not a local object"}
	}
	n, err := q.InsertRemotePostShare(ctx, gen.InsertRemotePostShareParams{
		PostID:        post.ID,
		RemoteActorID: signer.ID,
		ApID:          stringPtrOrNil(apStr(act["id"])),
	})
	if err != nil {
		return dispatchResult{status: "error", reason: "remote post share: " + err.Error()}
	}
	if n == 0 {
		return dispatchResult{status: "ignored", reason: "announce replayed"}
	}
	if err := q.AdjustRemotePostShareCount(ctx, gen.AdjustRemotePostShareCountParams{
		ID: post.ID, Delta: 1,
	}); err != nil {
		slog.Warn("federation: remote post share count failed", "error", err)
	}
	return dispatchResult{status: "processed"}
}

// unshareRemotePost is the Undo{Announce} arm — the delete's
// rows-affected gates the decrement so replays can't underflow.
func (s *Service) unshareRemotePost(ctx context.Context, q *gen.Queries, objectID string, signer *gen.RemoteActor) dispatchResult {
	post, err := q.GetFederationRemotePostByApID(ctx, objectID)
	if err != nil {
		return dispatchResult{status: "ignored", reason: "unannounce target unknown"}
	}
	n, err := q.DeleteRemotePostShare(ctx, gen.DeleteRemotePostShareParams{
		PostID: post.ID, RemoteActorID: signer.ID,
	})
	if err != nil {
		return dispatchResult{status: "error", reason: "remote post unshare: " + err.Error()}
	}
	if n > 0 {
		if err := q.AdjustRemotePostShareCount(ctx, gen.AdjustRemotePostShareCountParams{
			ID: post.ID, Delta: -1,
		}); err != nil {
			slog.Warn("federation: remote post unshare count failed", "error", err)
		}
	}
	return dispatchResult{status: "processed"}
}

// dispatchAddRemove is remote featured-pin bookkeeping (§126 M5): an
// Add/Remove whose object is a stored remote post authored by the signer
// and whose target collection lives on the signer's domain toggles
// is_pinned. Everything else — foreign collections, our own collections
// (remote actors cannot curate ours), unknown objects — ignores.
func (s *Service) dispatchAddRemove(ctx context.Context, q *gen.Queries, act map[string]any, signer *gen.RemoteActor) dispatchResult {
	if signer == nil {
		return dispatchResult{status: "rejected", reason: "unsigned add/remove"}
	}
	objectID := apObjectID(act)
	targetID := apID(act["target"])
	if objectID == "" || targetID == "" {
		return dispatchResult{status: "ignored", reason: "add/remove without object or target"}
	}
	if !isSignerCollectionTarget(targetID, signer.ActorUrl) {
		return dispatchResult{status: "ignored", reason: "target is not a signer-domain collection"}
	}
	post, err := q.GetFederationRemotePostByApID(ctx, objectID)
	if err != nil {
		return dispatchResult{status: "ignored", reason: "add/remove object unknown"}
	}
	if post.RemoteActorID != signer.ID {
		return dispatchResult{status: "rejected", reason: "pin of another actor's post"}
	}
	n, err := q.SetFederationRemotePostPinned(ctx, gen.SetFederationRemotePostPinnedParams{
		ApID: objectID, RemoteActorID: signer.ID,
		IsPinned: apStr(act["type"]) == "Add",
	})
	if err != nil {
		return dispatchResult{status: "error", reason: "remote post pin: " + err.Error()}
	}
	if n == 0 {
		return dispatchResult{status: "ignored", reason: "remote post tombstoned"}
	}
	return dispatchResult{status: "processed"}
}

// isSignerCollectionTarget gates Add/Remove curation: the target must be
// a collection IRI on the signer's own domain — either the canonical
// {actor}/collections/featured the fediverse pins into (Mastodon,
// Misskey, Firefish convention) or any explicit /collections/* path
// there. A bare host match would let a remote pin by pointing at any
// URL it controls.
func isSignerCollectionTarget(targetID, actorURL string) bool {
	tu, err := url.Parse(targetID)
	if err != nil || tu.Scheme == "" {
		return false
	}
	if targetID == strings.TrimSuffix(actorURL, "/")+"/collections/featured" {
		return true
	}
	return sameURLHost(targetID, actorURL) && strings.Contains(tu.Path, "/collections/")
}
