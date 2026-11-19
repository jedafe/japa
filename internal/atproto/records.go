package atproto

// Record projections (§127 A2): japa.to rows → atproto lexicon records.
// The repo is a materialized replica — PostgreSQL stays canonical; these
// functions only shape what the network sees. Eligibility mirrors AP
// (published + public + federatable type + feature flags).
//
// Ref: japaconnect-engineering/sections/127-atproto-native.md (Repo engine)

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/indigo/atproto/syntax"
	indmst "github.com/bluesky-social/indigo/mst"
	atrepo "github.com/bluesky-social/indigo/repo"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	gen "github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/features"
	"github.com/jedafe/japa/internal/rss/model"
	"github.com/jedafe/japa/internal/worker"
	"github.com/rivo/uniseg"
)

// Collection NSIDs the A2 engine writes. feed.post replies carry
// additional ref material handled by A5's inbound path.
const (
	CollectionProfile = "app.bsky.actor.profile"
	CollectionPost    = "app.bsky.feed.post"
)

// ProfileRkey is the canonical rkey for the member's profile record.
const ProfileRkey = "self"

// permanentError marks failures no retry can fix — malformed payloads,
// ineligible content, inactive accounts, unparsable URIs. The worker
// maps Permanent() to asynq.SkipRetry through a structural interface so
// it never needs to import this package (auth import cycle).
type permanentError struct{ err error }

func (e permanentError) Error() string   { return e.err.Error() }
func (e permanentError) Unwrap() error   { return e.err }
func (e permanentError) Permanent() bool { return true }

// permanent wraps err so errors.Is still matches the inner sentinel
// while worker-side errors.As sees Permanent(). Already-permanent
// errors pass through unwrapped.
func permanent(err error) error {
	if err == nil {
		return nil
	}
	var p interface{ Permanent() bool }
	if errors.As(err, &p) && p.Permanent() {
		return err
	}
	return permanentError{err}
}

// isPermanentErr reports whether err carries the terminal marker —
// the same structural check worker's permanenter performs, for
// in-package callers (reconcile per-row skip vs retry-join).
func isPermanentErr(err error) bool {
	var p interface{ Permanent() bool }
	return errors.As(err, &p) && p.Permanent()
}

// postTextRunes caps feed.post text. The lexicon limit is 300 grapheme
// clusters; a rune cap is a strict upper bound since every grapheme is
// at least one rune — 280 keeps title+excerpt+link comfortably inside.
const postTextRunes = 280

// federatableTypesFor returns the shared AP allowlist minus content types
// whose feature flag is unreachable for this caller — identical pruning
// to federation.federatableTypesFor over the same list (§127: "same
// eligibility as AP" — the two projections must not drift).
func (s *Service) federatableTypesFor(ctx context.Context) []string {
	base := model.FederatableTypes()
	gated := features.GatedContentTypes(ctx, s.flags)
	if len(gated) == 0 {
		return base
	}
	hide := make(map[string]bool, len(gated))
	for _, ct := range gated {
		hide[ct] = true
	}
	out := make([]string, 0, len(base))
	for _, ct := range base {
		if !hide[ct] {
			out = append(out, ct)
		}
	}
	return out
}

// profileRecordURI is the canonical at:// address of the account's
// app.bsky.actor.profile — rkey is always "self".
func profileRecordURI(did string) string {
	return fmt.Sprintf("at://%s/%s/%s", did, CollectionProfile, ProfileRkey)
}

// publishProfile writes the member's app.bsky.actor.profile — rkey
// "self", so repeated calls are upserts, not duplicates. The §127
// projection floor applies: below federation.min_posts_discoverable
// published posts the repo emits nothing, so the profile record is
// deleted (a no-op when absent) rather than upserted — an author
// dropping under the floor converges back to an empty repo the same
// way a never-projected one stays empty. Identity (DID/handle) is not
// part of the gate and still converges via syncHandle.
func (s *Service) publishProfile(ctx context.Context, account *gen.AtprotoAccount) error {
	if !s.featureOn(ctx) {
		// Same freeze PublishContent enforces — reconcile_repo reaches
		// this path ungated via lifecycle reactivation.
		return permanent(fmt.Errorf("atproto: feature.atproto disabled"))
	}
	user, err := s.store.userByID(ctx, account.UserID)
	if err != nil {
		return fmt.Errorf("atproto: profile user: %w", err)
	}
	// Profile writes double as the identity-convergence point — a
	// rename leaves the stored handle stale until this sync. Handle
	// resolution is identity, not discovery — it runs below the floor.
	account.Handle = s.syncHandle(ctx, account.ID, account.UserID, account.Did, account.Handle, user.Username)
	if !s.authorMeetsPostFloor(ctx, account.UserID) {
		return s.DeleteRecordURI(ctx, account.ID, profileRecordURI(account.Did))
	}
	profile, err := s.store.q.GetAtprotoUserProfile(ctx, account.UserID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("atproto: user profile: %w", err)
	}
	rec := profileRecord(user.Username, profile, account.CreatedAt.Time)
	s.enrichProfileBlobs(ctx, account.ID, profile.AvatarMediaID, profile.BannerMediaID, rec)
	if _, err := s.PutRecord(ctx, account.ID, CollectionProfile, ProfileRkey, rec, pgtype.UUID{}); err != nil {
		return err
	}
	return nil
}

// profileRecord projects user_profiles → app.bsky.actor.profile.
// displayName falls back to the username; avatar/banner blob refs are
// attached at write time by enrichProfileBlobs (A4). Both strings cap
// at the
// lexicon's maxGraphemes (64/256) — a raw rune cap overshoots on
// emoji/ZWJ sequences, which the network would reject at validation.
func profileRecord(username string, p gen.GetAtprotoUserProfileRow, accountCreated time.Time) *bsky.ActorProfile {
	rec := &bsky.ActorProfile{LexiconTypeID: CollectionProfile}
	name := username
	if p.DisplayName != nil && strings.TrimSpace(*p.DisplayName) != "" {
		name = strings.TrimSpace(*p.DisplayName)
	}
	name = truncateGraphemes(name, 64)
	rec.DisplayName = &name
	if p.Bio != nil && strings.TrimSpace(*p.Bio) != "" {
		bio := truncateGraphemes(strings.TrimSpace(*p.Bio), 256)
		rec.Description = &bio
	}
	created := accountCreated.UTC().Format(time.RFC3339)
	rec.CreatedAt = &created
	return rec
}

// ProfileRecordJSON marshals a profile record for the
// atproto:publish_record task payload — callers (e.g. the users package's
// profile-update hook) embed it as `record`.
func ProfileRecordJSON(displayName, bio *string, username string, createdAt time.Time) (json.RawMessage, error) {
	p := gen.GetAtprotoUserProfileRow{DisplayName: displayName, Bio: bio}
	return json.Marshal(profileRecord(username, p, createdAt))
}

// postRecord projects a contents row → app.bsky.feed.post: title,
// markdown-stripped excerpt, and the canonical page URL — the URL always
// survives truncation whole (the excerpt shrinks first) so the link facet
// never covers a cut-off address. Facet offsets index BYTES, not runes —
// multi-byte text before the URL shifts them (§127 A4's named risk).
func (s *Service) postRecord(row gen.GetAtprotoProjectableContentRow, embed *bsky.FeedPost_Embed) *bsky.FeedPost {
	pageURL := s.pageURL(row.ContentType, row.Slug, row.CanonicalUrl)
	text := composePostText(strings.TrimSpace(row.Title), stripMarkdown(row.Body), pageURL)

	created := row.CreatedAt.Time
	if row.PublishedAt.Valid {
		created = row.PublishedAt.Time
	}
	rec := &bsky.FeedPost{
		LexiconTypeID: CollectionPost,
		Text:          text,
		CreatedAt:     created.UTC().Format(time.RFC3339),
		Embed:         embed,
	}
	if facet := linkFacet(text, pageURL); facet != nil {
		rec.Facets = []*bsky.RichtextFacet{facet}
	}
	return rec
}

// composePostText joins title/excerpt/URL within the grapheme cap —
// the excerpt gives before the URL does, so a whole link always anchors
// the post tail (the facet target must survive to be clickable).
func composePostText(title, excerpt, pageURL string) string {
	join := func(ex, url string) string {
		parts := make([]string, 0, 3)
		for _, p := range []string{truncateRunes(title, 120), ex, url} {
			if p != "" {
				parts = append(parts, p)
			}
		}
		return strings.Join(parts, "\n\n")
	}
	excerpt = truncateRunes(excerpt, 140)
	text := join(excerpt, pageURL)
	if over := utf8.RuneCountInString(text) - postTextRunes; over > 0 {
		// Shrink the excerpt by the overflow (… marks the cut).
		excerpt = truncateRunes(excerpt, max(0, utf8.RuneCountInString(excerpt)-over))
		text = join(excerpt, pageURL)
	}
	// A URL that still can't fit intact (a member-supplied canonical_url
	// has no length bound) is dropped whole, never cut — a partial link
	// would silently point nowhere. title+excerpt alone can't reach the
	// cap, and linkFacet's suffix check yields no facet automatically.
	if utf8.RuneCountInString(text) > postTextRunes {
		text = join(excerpt, "")
	}
	return text
}

// linkFacet emits the app.bsky.richtext.facet#link covering the URL at
// the text tail. Byte offsets are trivially exact — the URL is the
// suffix — and nil when truncation dropped it (facet on a partial URL
// would point nowhere).
func linkFacet(text, pageURL string) *bsky.RichtextFacet {
	if pageURL == "" || !strings.HasSuffix(text, pageURL) {
		return nil
	}
	return &bsky.RichtextFacet{
		Index: &bsky.RichtextFacet_ByteSlice{
			ByteStart: int64(len(text) - len(pageURL)),
			ByteEnd:   int64(len(text)),
		},
		Features: []*bsky.RichtextFacet_Features_Elem{{
			RichtextFacet_Link: &bsky.RichtextFacet_Link{Uri: pageURL},
		}},
	}
}

// contentRkey is the feed.post record key for a contents row — the
// content UUID itself (a legal record key), so republishing upserts the
// same record instead of duplicating posts, and the URI is derivable
// anywhere the content ID is known.
func contentRkey(contentID pgtype.UUID) string { return contentID.String() }

// ContentPostURI is the canonical at:// URI of a content's feed.post
// record — used by unpublish/delete enqueues that must name the record
// without a DB lookup.
func ContentPostURI(did string, contentID pgtype.UUID) string {
	return "at://" + did + "/" + CollectionPost + "/" + contentRkey(contentID)
}

// PublishContent projects an eligible contents row into the author's
// repo as app.bsky.feed.post, keyed by the content UUID — republishes
// are upserts, never duplicates. The account is resolved from the
// content's author, so a caller can only ever publish into the owning
// repo. Content that has left the federatable set (unpublished,
// visibility tightened, deleted, flag-gated, author without an active
// account) resolves permanent(ErrNotFound) — retrying can never make
// it eligible. The feature.atproto kill-switch freezes new projections
// but never blocks deletes, so tombstones still drain the repo while
// the feature is off. Returns the record's at:// URI.
func (s *Service) PublishContent(ctx context.Context, contentID pgtype.UUID) (string, error) {
	if !s.featureOn(ctx) {
		return "", permanent(fmt.Errorf("atproto: feature.atproto disabled"))
	}
	accountID, err := s.store.q.GetAtprotoAccountIDByContent(ctx, contentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", permanent(ErrNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("atproto: account for content: %w", err)
	}
	minPosts := s.minPostsDiscoverable(ctx)
	row, err := s.store.q.GetAtprotoProjectableContent(ctx, gen.GetAtprotoProjectableContentParams{
		ID:           contentID,
		ContentTypes: s.federatableTypesFor(ctx),
		MinPosts:     minPosts,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", permanent(ErrNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("atproto: projectable content: %w", err)
	}
	// A4: blob puts run before the repo write — blobs aren't signed repo
	// data, so they persist outside the commit tx (an aborted commit leaves
	// unreferenced blobs, which is harmless: CIDs are content-addressed and
	// a retry reuses them).
	rec := s.postRecord(row, s.imageEmbed(ctx, accountID, contentID))
	rkey := contentRkey(contentID)

	// The eligibility re-check runs inside the repo tx, on the same
	// connection, right before the commit — READ COMMITTED observes an
	// unpublish that landed after the task-time read. Without it the
	// publish-vs-unpublish interleave leaves a record live: the delete
	// emit already ran and found no index row to address.
	var uri string
	err = s.withRepoWrite(ctx, accountID, repoWriteNormal, func(rep *atrepo.Repo, q *gen.Queries) ([]indexWrite, error) {
		if _, err := q.GetAtprotoProjectableContent(ctx, gen.GetAtprotoProjectableContentParams{
			ID:           contentID,
			ContentTypes: s.federatableTypesFor(ctx),
			MinPosts:     minPosts,
		}); errors.Is(err, pgx.ErrNoRows) {
			return nil, permanent(ErrNotFound)
		} else if err != nil {
			return nil, fmt.Errorf("atproto: projectable re-check: %w", err)
		}
		rpath := CollectionPost + "/" + rkey
		recCid, err := upsertRecord(ctx, rep, rpath, rec)
		if err != nil {
			return nil, fmt.Errorf("atproto: put %s: %w", rpath, err)
		}
		uri = fmt.Sprintf("at://%s/%s", rep.RepoDid(), rpath)
		return []indexWrite{{uri: uri, collection: CollectionPost, rkey: rkey, cid: recCid.String(), contentID: contentID}}, nil
	})
	if errors.Is(err, ErrNotFound) {
		return "", permanent(err)
	}
	if err == nil {
		s.emitBacklogReconcile(ctx, accountID, row.AuthorID, minPosts)
	}
	return uri, err
}

// emitBacklogReconcile probes a just-published repo for unprojected
// work and enqueues atproto:reconcile_repo when any exists — the §127
// floor-crossing emit. Below the floor nothing projects, so a publish
// can only succeed at or above it; whatever was withheld before the
// crossing (earlier posts, the never-projected profile) is a backlog
// the reconcile's missing-walk emits together. Steady-state authors
// pay two indexed probes and enqueue nothing.
func (s *Service) emitBacklogReconcile(ctx context.Context, accountID, authorID pgtype.UUID, minPosts int32) {
	missing, err := s.store.q.ListAtprotoReconcileMissingPosts(ctx, gen.ListAtprotoReconcileMissingPostsParams{
		UserID:       authorID,
		AccountID:    accountID,
		ContentTypes: s.federatableTypesFor(ctx),
		MinPosts:     minPosts,
	})
	if err == nil && len(missing) > 0 {
		worker.EnqueueAtprotoReconcileRepo(ctx, accountID)
		return
	}
	account, aerr := s.store.q.GetAtprotoAccountByID(ctx, accountID)
	if aerr != nil {
		return
	}
	if _, perr := s.store.q.GetAtprotoRecord(ctx, profileRecordURI(account.Did)); errors.Is(perr, pgx.ErrNoRows) {
		worker.EnqueueAtprotoReconcileRepo(ctx, accountID)
	}
}

// DeleteRecordURI removes the record addressed by an at:// URI —
// the atproto:delete_record task path (§11: tombstone/unpublish). The
// URI's DID must match the account's own repo (a mismatched authority
// would delete nothing and mask a caller bug). An absent rpath is a
// no-op — no commit, no seq bump — so retried unpublishes stay
// idempotent. Permanent on bad URI, DID mismatch, or an inactive
// account (ErrNotFound).
func (s *Service) DeleteRecordURI(ctx context.Context, accountID pgtype.UUID, uri string) error {
	parsed, err := syntax.ParseATURI(uri)
	if err != nil {
		return permanent(fmt.Errorf("atproto: delete uri %q: %w", uri, err))
	}
	collection := parsed.Collection().String()
	rkey := parsed.RecordKey().String()
	if collection == "" || rkey == "" {
		return permanent(fmt.Errorf("atproto: delete uri %q: missing collection or record key", uri))
	}
	// ATURI preserves did:web percent-encoding in the authority
	// (did:web:host%3Aport:...) and didFor stores the DID in the same
	// encoded form, so the strings compare directly.
	err = s.withRepoWrite(ctx, accountID, repoWriteNormal, func(rep *atrepo.Repo, _ *gen.Queries) ([]indexWrite, error) {
		if parsed.Authority().String() != rep.RepoDid() {
			return nil, permanent(fmt.Errorf("atproto: delete uri %q: authority is not this repo", uri))
		}
		rpath := collection + "/" + rkey
		if _, _, gerr := rep.GetRecordBytes(ctx, rpath); errors.Is(gerr, indmst.ErrNotFound) {
			return nil, errRepoNoop
		} else if gerr != nil {
			return nil, fmt.Errorf("atproto: stat %s: %w", rpath, gerr)
		}
		if err := rep.DeleteRecord(ctx, rpath); err != nil {
			return nil, fmt.Errorf("atproto: delete %s/%s: %w", collection, rkey, err)
		}
		return []indexWrite{{del: true, collection: collection, rkey: rkey}}, nil
	})
	switch {
	case errors.Is(err, errRepoNoop):
		return nil
	case errors.Is(err, ErrNotFound):
		return permanent(err)
	default:
		return err
	}
}

// decodeRecord unmarshals a task-payload record doc into its lexicon
// type — the only two collections the A2 engine writes.
func decodeRecord(collection string, raw json.RawMessage) (atrepo.CborMarshaler, error) {
	switch collection {
	case CollectionProfile:
		var rec bsky.ActorProfile
		if err := json.Unmarshal(raw, &rec); err != nil {
			return nil, fmt.Errorf("atproto: bad profile record: %w", err)
		}
		rec.LexiconTypeID = CollectionProfile
		return &rec, nil
	case CollectionPost:
		var rec bsky.FeedPost
		if err := json.Unmarshal(raw, &rec); err != nil {
			return nil, fmt.Errorf("atproto: bad post record: %w", err)
		}
		rec.LexiconTypeID = CollectionPost
		return &rec, nil
	default:
		return nil, fmt.Errorf("atproto: unsupported collection %q", collection)
	}
}

// featureOn reports whether the mini-PDS surface is live — nil
// evaluator means ungated (tests, single-process dev), matching the
// features.GatedContentTypes convention.
func (s *Service) featureOn(ctx context.Context) bool {
	return s.flags == nil || s.flags.IsEnabled(ctx, "feature.atproto")
}

// FeatureOn is the exported form — server.go's unified-OAuth dispatchers
// need the gate to pick the right profile's handler/doc.
func (s *Service) FeatureOn(ctx context.Context) bool { return s.featureOn(ctx) }

// IsPDSHost reports whether hostport is the PDS host — the host-aware
// dispatch on /.well-known/oauth-protected-resource (§127 A6-5). Port
// tolerated for dev; comparison is case-insensitive on the hostname.
func (s *Service) IsPDSHost(hostport string) bool {
	u, err := url.Parse(s.cfg.PDSURL)
	if err != nil || u.Hostname() == "" {
		return false
	}
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	return strings.EqualFold(host, u.Hostname())
}

// PublishRecordJSON is the Asynq atproto:publish_record entry point:
// decode the payload's record doc, then run the standard put+commit path.
// An empty rkey creates under a fresh TID (feed.post); an explicit rkey
// upserts (app.bsky.actor.profile/self). Decode failures, a disabled
// feature flag, and inactive accounts are permanent — retrying cannot
// change the outcome.
func (s *Service) PublishRecordJSON(ctx context.Context, accountID pgtype.UUID, collection, rkey string, raw json.RawMessage, contentID pgtype.UUID) (string, error) {
	if !s.featureOn(ctx) {
		return "", permanent(fmt.Errorf("atproto: feature.atproto disabled"))
	}
	rec, err := decodeRecord(collection, raw)
	if err != nil {
		return "", permanent(err)
	}
	// A4: profile records converge through publishProfile — it rebuilds
	// the record from the live rows (payload text is advisory: blob refs
	// the pre-marshaled payload can't carry, and back-to-back edits or
	// delayed media replays leave it stale by execution time), syncs the
	// handle, and applies the §127 projection floor — below it the self
	// record deletes instead of writing.
	if _, ok := rec.(*bsky.ActorProfile); ok {
		account, err := s.store.q.GetAtprotoAccountByID(ctx, accountID)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", permanent(ErrNotFound)
		}
		if err != nil {
			return "", fmt.Errorf("atproto: profile account: %w", err)
		}
		if err := s.publishProfile(ctx, &account); err != nil {
			return "", err
		}
		return profileRecordURI(account.Did), nil
	}
	// A record task carrying content_id must target the repo that owns
	// the content — otherwise the index would link someone else's row
	// and a later content delete would tombstone the wrong repo's
	// record. Mispairing is terminal, never retryable.
	if contentID.Valid {
		owner, err := s.store.q.GetAtprotoAccountIDByContent(ctx, contentID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return "", permanent(ErrNotFound)
			}
			return "", fmt.Errorf("atproto: account for content: %w", err)
		}
		if owner != accountID {
			return "", permanent(fmt.Errorf("atproto: content %s is not owned by account %s", contentID.String(), accountID.String()))
		}
	}
	var uri string
	if rkey == "" {
		uri, _, err = s.CreateRecord(ctx, accountID, collection, rec, contentID)
	} else {
		uri, err = s.PutRecord(ctx, accountID, collection, rkey, rec, contentID)
	}
	if errors.Is(err, ErrNotFound) {
		return "", permanent(err)
	}
	return uri, err
}

// pageURL resolves the human-facing content URL — same rule as
// federation: editorial canonical_url when set, else the type's route.
func (s *Service) pageURL(contentType, slug string, canonical *string) string {
	if canonical != nil && *canonical != "" {
		c := *canonical
		if strings.HasPrefix(c, "http://") || strings.HasPrefix(c, "https://") {
			return c
		}
		return s.cfg.siteBase() + "/" + strings.TrimPrefix(c, "/")
	}
	return s.cfg.siteBase() + model.LinkPathFor(contentType) + slug
}

// truncateGraphemes caps s at n grapheme clusters — the unit a
// lexicon maxGraphemes constraint counts. Cluster boundaries are
// byte-exact, so slicing at the (n+1)th cluster's start keeps the
// result valid UTF-8.
func truncateGraphemes(s string, n int) string {
	g := uniseg.NewGraphemes(s)
	count := 0
	end := len(s)
	for g.Next() {
		if count == n {
			end, _ = g.Positions()
			break
		}
		count++
	}
	return s[:end]
}

// truncateRunes caps s at n runes, marking truncation with '…'.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

// stripMarkdown flattens markdown source to plain text for feed.post
// (same approach as internal/rag's copy — headings, emphasis, list
// markers and [text](url) syntax removed, links reduced to text).
func stripMarkdown(md string) string {
	var sb strings.Builder
	for _, line := range strings.Split(md, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		trimmed = strings.TrimLeft(trimmed, "#")
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "+ ") {
			trimmed = trimmed[2:]
		} else if len(trimmed) > 2 && unicode.IsDigit(rune(trimmed[0])) && trimmed[1] == '.' && trimmed[2] == ' ' {
			trimmed = trimmed[3:]
		}
		trimmed = strings.TrimPrefix(trimmed, "> ")
		trimmed = strings.ReplaceAll(trimmed, "**", "")
		trimmed = strings.ReplaceAll(trimmed, "__", "")
		trimmed = strings.ReplaceAll(trimmed, "`", "")
		trimmed = strings.ReplaceAll(trimmed, "*", "")
		trimmed = strings.ReplaceAll(trimmed, "_", " ")
		trimmed = stripLinks(trimmed)
		sb.WriteString(strings.TrimSpace(trimmed))
		sb.WriteString(" ")
	}
	return strings.TrimSpace(sb.String())
}

// stripLinks reduces [text](url) to text.
func stripLinks(s string) string {
	for {
		open := strings.Index(s, "](")
		if open < 0 {
			return s
		}
		close := strings.Index(s[open:], ")")
		if close < 0 {
			return s
		}
		close += open
		start := strings.LastIndex(s[:open], "[")
		if start < 0 {
			return s
		}
		s = s[:start] + s[start+1:open] + s[close+1:]
	}
}
