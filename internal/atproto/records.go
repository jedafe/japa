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
	"strings"
	"time"
	"unicode"

	"github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/indigo/atproto/syntax"
	indmst "github.com/bluesky-social/indigo/mst"
	atrepo "github.com/bluesky-social/indigo/repo"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	gen "github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/features"
	"github.com/jedafe/japa/internal/rss/model"
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

// publishProfile writes the member's app.bsky.actor.profile — rkey
// "self", so repeated calls are upserts, not duplicates.
func (s *Service) publishProfile(ctx context.Context, account *gen.AtprotoAccount) error {
	user, err := s.store.userByID(ctx, account.UserID)
	if err != nil {
		return fmt.Errorf("atproto: profile user: %w", err)
	}
	profile, err := s.store.q.GetAtprotoUserProfile(ctx, account.UserID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("atproto: user profile: %w", err)
	}
	rec := profileRecord(user.Username, profile, account.CreatedAt.Time)
	if _, err := s.PutRecord(ctx, account.ID, CollectionProfile, ProfileRkey, rec, pgtype.UUID{}); err != nil {
		return err
	}
	return nil
}

// profileRecord projects user_profiles → app.bsky.actor.profile.
// displayName falls back to the username; avatar/banner are blob refs
// and arrive with A4's media pipeline.
func profileRecord(username string, p gen.GetAtprotoUserProfileRow, accountCreated time.Time) *bsky.ActorProfile {
	rec := &bsky.ActorProfile{LexiconTypeID: CollectionProfile}
	name := username
	if p.DisplayName != nil && strings.TrimSpace(*p.DisplayName) != "" {
		name = strings.TrimSpace(*p.DisplayName)
	}
	rec.DisplayName = &name
	if p.Bio != nil && strings.TrimSpace(*p.Bio) != "" {
		bio := strings.TrimSpace(*p.Bio)
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
// markdown-stripped excerpt, and the canonical page URL, bounded by the
// lexicon's grapheme limit. Facets/embeds are A4 (UTF-8 byte-offset
// correctness is its own risk surface per §127).
func (s *Service) postRecord(row gen.GetAtprotoProjectableContentRow) *bsky.FeedPost {
	var sb strings.Builder
	if t := strings.TrimSpace(row.Title); t != "" {
		sb.WriteString(truncateRunes(t, 120))
		sb.WriteString("\n\n")
	}
	sb.WriteString(truncateRunes(stripMarkdown(row.Body), 140))
	if sb.Len() > 0 {
		sb.WriteString("\n\n")
	}
	sb.WriteString(s.pageURL(row.ContentType, row.Slug, row.CanonicalUrl))

	created := row.CreatedAt.Time
	if row.PublishedAt.Valid {
		created = row.PublishedAt.Time
	}
	return &bsky.FeedPost{
		LexiconTypeID: CollectionPost,
		Text:          truncateRunes(strings.TrimSpace(sb.String()), postTextRunes),
		CreatedAt:     created.UTC().Format(time.RFC3339),
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
// are upserts, never duplicates. Content that has left the federatable
// set (unpublished, visibility tightened, deleted, flag-gated) resolves
// permanent(ErrNotFound) — retrying can never make it eligible. The
// feature.atproto kill-switch freezes new projections but never blocks
// deletes, so tombstones still drain the repo while the feature is off.
// Returns the record's at:// URI.
func (s *Service) PublishContent(ctx context.Context, accountID, contentID pgtype.UUID) (string, error) {
	if !s.featureOn(ctx) {
		return "", permanent(fmt.Errorf("atproto: feature.atproto disabled"))
	}
	row, err := s.store.q.GetAtprotoProjectableContent(ctx, gen.GetAtprotoProjectableContentParams{
		ID:           contentID,
		ContentTypes: s.federatableTypesFor(ctx),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", permanent(ErrNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("atproto: projectable content: %w", err)
	}
	uri, err := s.PutRecord(ctx, accountID, CollectionPost, contentRkey(contentID), s.postRecord(row), contentID)
	if errors.Is(err, ErrNotFound) {
		return "", permanent(err)
	}
	return uri, err
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
	if rkey == "" {
		return permanent(fmt.Errorf("atproto: delete uri %q: missing record key", uri))
	}
	err = s.withRepoWrite(ctx, accountID, func(rep *atrepo.Repo) ([]indexWrite, error) {
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
