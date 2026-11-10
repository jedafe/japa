package comments

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/cdn"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/features"
	rssmodel "github.com/jedafe/japa/internal/rss/model"
	"github.com/jedafe/japa/internal/tools"
)

// ---------------------------------------------------------------------------
// Thread Anchors (/thread/{slug} entity threads)
//
// A thread anchor binds a lazily-created `discussion` contents row to a
// non-content entity — a tool page (/tools/{tool}) or a wiki page
// (/wiki/{space}/{page}) — so the §49 comment engine can anchor a thread to
// it. The anchor row is the /thread/{slug} page for that entity.
//
// GET  /api/v1/threads/anchor/{entityType}/{entityKey} — resolve an anchor
// POST /api/v1/threads/anchor — get-or-create an anchor (auth)
//
// Ref: docs/superpowers/specs/2026-11-08-thread-rename-unified-discussions-design.md
// ---------------------------------------------------------------------------

// anchorPlatformSpaceSlug is the platform-run space all anchor contents rows
// are filed under. Seeded by migration 001091; creator_user_id IS NULL so the
// space reports is_primary. Wiki spaces can't be reused — wiki-only space
// types reject comments (ErrWikiOnlySpace), and tool pages have no space of
// their own.
const anchorPlatformSpaceSlug = "community-threads"

// anchorSpaceID is resolved lazily from anchorPlatformSpaceSlug — the seeded
// id is environment-dependent (migrations run against different datasets).
var anchorSpaceID pgtype.UUID

// anchorBotUserID is resolved lazily from the 'community-threads' system
// user seeded by migration 001093 — environment-dependent like the space id.
var anchorBotUserID pgtype.UUID

// toolAnchorEnabled reports whether a tool may hold a live anchor: known to
// the static page registry AND not disabled by its feature flag. A missing
// flag mapping or nil checker means the tool is always on.
func (s *Service) toolAnchorEnabled(ctx context.Context, entityKey string) bool {
	page, ok := tools.Lookup(entityKey)
	if !ok {
		return false
	}
	if page.Flag == "" || s.flags == nil {
		return true
	}
	return s.flags.IsEnabled(ctx, page.Flag)
}

// AnchorSummary is the public shape returned by the anchor endpoints — the
// backing contents row identifiers plus the entity descriptor the
// /thread/{slug} embed card renders.
type AnchorSummary struct {
	ContentID     pgtype.UUID `json:"content_id"`
	Slug          string      `json:"slug"`
	Title         string      `json:"title"`
	CommentCount  int32       `json:"comment_count"`
	EntityType    string      `json:"entity_type"`
	EntityKey     string      `json:"entity_key"`
	EntityURL     string      `json:"entity_url"`
	EntityTitle   string      `json:"entity_title"`
	EntitySummary *string     `json:"entity_summary,omitempty"`
	// Created is true only on the fresh-mint path — false for an existing
	// anchor resolution and for a soft-deleted thread's restore. Lets
	// callers (StartConversation) distinguish a genuinely new conversation
	// from get-or-create "get" hits without a second resolve.
	Created bool `json:"created"`
}

// anchorSpace resolves the seeded platform space id once per process.
func (s *Service) anchorSpace(ctx context.Context) (pgtype.UUID, error) {
	if anchorSpaceID.Valid {
		return anchorSpaceID, nil
	}

	space, err := s.read.GetSpaceBySlug(ctx, anchorPlatformSpaceSlug)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("resolving thread-anchor platform space: %w", err)
	}

	anchorSpaceID = space.ID

	return anchorSpaceID, nil
}

// anchorBotUser resolves the 'community-threads' system author (seeded by
// migration 001093) once per process. Communal entities — tools and wiki
// pages — have no single owner, so their minted threads are attributed to
// the bot, never to whichever visitor or approver triggered a lazy mint:
// a user-owned anchor thread could be retitled (defeating the generated-
// title heal) or deleted by a drive-by minter. Falls back to the caller
// when the bot row can't be resolved — a failed lookup must not block a
// legitimate mint.
func (s *Service) anchorBotUser(ctx context.Context, fallback pgtype.UUID) pgtype.UUID {
	if anchorBotUserID.Valid {
		return anchorBotUserID
	}
	bot, err := s.read.GetUserByUsername(ctx, "community-threads")
	if err != nil {
		slog.Warn("comments: anchor bot user lookup failed", "error", err)
		return fallback
	}
	anchorBotUserID = bot.UserID
	return anchorBotUserID
}

// anchorSpaceForContent resolves the space a content anchor files under —
// the content_spaces primary is authoritative, contents.space_id the
// legacy fallback. Three outcomes:
//   - public+active primary space → the thread files under it, inheriting
//     the article's audience
//   - a bound but non-public/inactive/missing space → ok=false (a public
//     thread would outlive its parent's space gate — leak protection)
//   - no space binding at all (roadmap shadow rows, imports) → the
//     platform anchor space, same as tool/wiki threads
//
// Shared by the mint gate (describeEntity) and the resolve gate so a
// re-filed parent stops resolving on the same rule that blocked minting.
func (s *Service) anchorSpaceForContent(ctx context.Context, row gen.GetContentByIDRow) (pgtype.UUID, bool) {
	spaceID := row.SpaceID
	bound := spaceID.Valid
	spaces, err := s.read.GetContentSpaces(ctx, row.ID)
	if err != nil && !spaceID.Valid {
		// The authoritative binding set is unreadable and the legacy
		// space_id can't vouch for it either — the row might be
		// private-bound, so it must not mint under the public platform
		// space. Fail closed; the caller can retry.
		return pgtype.UUID{}, false
	}
	for _, sp := range spaces {
		bound = true
		if sp.IsPrimary {
			spaceID = sp.SpaceID
			break
		}
	}
	if spaceID.Valid {
		space, err := s.read.GetSpaceByID(ctx, spaceID)
		if err != nil {
			return pgtype.UUID{}, false
		}
		if space.Visibility != "public" || space.Status != "active" || space.DeletedAt.Valid {
			return pgtype.UUID{}, false
		}
		return spaceID, true
	}
	if bound {
		// content_spaces rows exist but none resolves to a usable primary.
		return pgtype.UUID{}, false
	}
	platform, err := s.anchorSpace(ctx)
	return platform, err == nil
}

// contentEntityURL resolves the article's canonical detail route via the
// shared rss/model map (LinkPathFor — single Go source, mirrored by
// contentLink.ts) so thread_anchors.entity_url lands on the real content
// page, not a discussion stub or the /content redirect shim.
func contentEntityURL(contentType, slug string) string {
	return rssmodel.LinkPathFor(contentType) + slug
}

// ResolveAnchor returns the thread anchor bound to an entity, or ErrNotFound.
// entity_type='content': a `discussion` row IS a thread already and resolves
// to itself; every other content type resolves through its anchored
// discussion row (minted on publish or on first "Start a conversation").
func (s *Service) ResolveAnchor(ctx context.Context, entityType, entityKey string) (AnchorSummary, error) {
	if entityType == "content" {
		var contentID pgtype.UUID
		if err := contentID.Scan(entityKey); err != nil {
			return AnchorSummary{}, fmt.Errorf("content key must be a content UUID: %w", errpkg.ErrInvalid)
		}

		row, err := s.read.GetContentByID(ctx, contentID)
		if err != nil {
			return AnchorSummary{}, err
		}
		// The resolve route is public — only surface rows an anonymous
		// viewer could load on an entity page. Drafts, pending-moderation,
		// non-public and auto-hidden rows must not leak title/slug/
		// comment_count — and a content type gated off by §42 flags hides
		// its pages, so its conversation hides too.
		if row.Status != content.StatusPublished || row.Visibility != content.VisibilityPublic || row.AutoHidden {
			return AnchorSummary{}, pgx.ErrNoRows
		}
		if res, ok := features.EvalContentType(ctx, s.flags, row.ContentType); ok && !res.Enabled {
			return AnchorSummary{}, pgx.ErrNoRows
		}

		// A discussion row IS its own thread — no separate anchor needed.
		if row.ContentType == "discussion" {
			return AnchorSummary{
				ContentID:    row.ID,
				Slug:         row.Slug,
				Title:        row.Title,
				CommentCount: row.CommentCount,
				EntityType:   "content",
				EntityKey:    entityKey,
			}, nil
		}

		// The minted thread files under the parent's primary space — a post
		// re-filed into a private/inactive space (UpdateContentSpaces)
		// stops resolving on the same gate that blocks minting, even
		// before the sweeper drops the backing row.
		if _, ok := s.anchorSpaceForContent(ctx, row); !ok {
			return AnchorSummary{}, pgx.ErrNoRows
		}

		anchor, err := s.read.GetThreadAnchorByEntity(ctx, gen.GetThreadAnchorByEntityParams{
			EntityType: entityType,
			EntityKey:  entityKey,
		})
		if err != nil {
			return AnchorSummary{}, err
		}

		return anchorSummaryFromRow(anchor), nil
	}

	row, err := s.read.GetThreadAnchorByEntity(ctx, gen.GetThreadAnchorByEntityParams{
		EntityType: entityType,
		EntityKey:  entityKey,
	})
	if err != nil {
		return AnchorSummary{}, err
	}

	// Live-gate the entity behind the anchor — the same conditions that
	// block minting. Flag writes fire the drop hook, but direct flag edits,
	// rollout scoping and lazy states never reach it, so resolve re-checks.
	switch entityType {
	case "tool":
		if !s.toolAnchorEnabled(ctx, entityKey) {
			return AnchorSummary{}, pgx.ErrNoRows
		}
	case "wiki_page":
		if s.flags != nil && !s.flags.IsEnabled(ctx, "wiki.enabled") {
			return AnchorSummary{}, pgx.ErrNoRows
		}
		// Same gate the mint query applies — a page deleted, or filed under
		// a space that went private/inactive/deleted, stops resolving even
		// if the space-close drop hook missed it.
		var pageID pgtype.UUID
		if err := pageID.Scan(entityKey); err != nil {
			return AnchorSummary{}, pgx.ErrNoRows
		}
		if _, err := s.read.GetWikiPageAnchorInfo(ctx, pageID); err != nil {
			return AnchorSummary{}, pgx.ErrNoRows
		}
	}

	return anchorSummaryFromRow(row), nil
}

func anchorSummaryFromRow(row gen.GetThreadAnchorByEntityRow) AnchorSummary {
	out := AnchorSummary{
		ContentID:   row.ContentID,
		Slug:        row.ContentSlug,
		Title:       row.ContentTitle,
		EntityType:  row.EntityType,
		EntityKey:   row.EntityKey,
		EntityURL:   row.EntityUrl,
		EntityTitle: row.EntityTitle,
	}
	out.CommentCount = row.CommentCount
	if row.EntitySummary != nil {
		out.EntitySummary = row.EntitySummary
	}

	return out
}

// entityDescriptor is the validated display record for an anchor target.
type entityDescriptor struct {
	URL     string
	Title   string
	Summary *string
	// SpaceID the anchor contents row files under — the platform anchor
	// space for tool/wiki entities; the parent's own primary space for
	// content entities so the thread inherits the article's audience.
	SpaceID    pgtype.UUID
	GeoSpaceID pgtype.UUID
	// Visibility of the minted thread row — mirrors the parent content's
	// audience for content entities; empty defaults to public.
	Visibility string
	// AuthorID the minted thread is attributed to when the entity has an
	// owner (content entities: the article author). Zero for communal
	// entities (wiki/tool) — those mint under the community-threads bot.
	AuthorID pgtype.UUID
}

// describeEntity validates the requested anchor target and resolves its
// display fields server-side (never trusting client-supplied titles).
func (s *Service) describeEntity(ctx context.Context, entityType, entityKey string) (entityDescriptor, error) {
	switch entityType {
	case "content":
		{
			var contentID pgtype.UUID
			if err := contentID.Scan(entityKey); err != nil {
				return entityDescriptor{}, fmt.Errorf("content key must be a content UUID: %w", errpkg.ErrInvalid)
			}

			row, err := s.read.GetContentByID(ctx, contentID)
			if err != nil {
				return entityDescriptor{}, err
			}

			// Only live, public rows mint a thread: drafts/review/private/
			// auto-hidden rows have no public thread page, and a minted
			// thread would leak the unpublished title via /thread/{slug}.
			// Discussion rows resolve to themselves before reaching here;
			// forum_thread rows already have a thread surface.
			if row.Status != content.StatusPublished || row.Visibility != content.VisibilityPublic || row.AutoHidden {
				return entityDescriptor{}, fmt.Errorf("content is not publicly live: %w", errpkg.ErrInvalid)
			}
			// A type flagged off by §42 (jobs.enabled, video.enabled, …)
			// hides the page, so it must not mint a public thread either.
			if res, ok := features.EvalContentType(ctx, s.flags, row.ContentType); ok && !res.Enabled {
				return entityDescriptor{}, fmt.Errorf("content_type %q is flag-disabled: %w", row.ContentType, errpkg.ErrInvalid)
			}
			if row.ContentType == "discussion" || row.ContentType == "forum_thread" {
				return entityDescriptor{}, fmt.Errorf("content_type %q is already a thread: %w", row.ContentType, errpkg.ErrInvalid)
			}

			// Same leak guard as wiki pages: the thread files under the
			// parent's primary space (content_spaces is authoritative;
			// contents.space_id is the legacy fallback), so only
			// public+active spaces may bind a public anchor. Rows with no
			// space binding at all (roadmap shadows, imports) fall back to
			// the platform anchor space.
			spaceID, ok := s.anchorSpaceForContent(ctx, row)
			if !ok {
				return entityDescriptor{}, fmt.Errorf("content has no public primary space: %w", errpkg.ErrInvalid)
			}

			summary := ""
			if row.MetaDescription != nil {
				summary = strings.TrimSpace(*row.MetaDescription)
			}
			if summary == "" {
				summary = content.ExcerptFromBody(row.Body, "", 160)
			}

			entityURL := contentEntityURL(row.ContentType, row.Slug)
			if row.ContentType == "roadmap" {
				// Shadow-row slugs are "roadmap-{item.slug}" — the public
				// entity page is the item route, which the content slug
				// can't reconstruct. Unresolved items keep the generic
				// /content fallback rather than fail the describe.
				if itemSlug, err := s.read.GetRoadmapItemSlugByContentID(ctx, row.ID); err == nil {
					entityURL = "/roadmap/" + itemSlug
				}
			}

			return entityDescriptor{
				URL:        entityURL,
				Title:      row.Title,
				Summary:    &summary,
				SpaceID:    spaceID,
				GeoSpaceID: row.GeoSpaceID,
				Visibility: row.Visibility,
				AuthorID:   row.AuthorID,
			}, nil
		}
	}

	spaceID, err := s.anchorSpace(ctx)
	if err != nil {
		return entityDescriptor{}, err
	}

	switch entityType {
	case "tool":
		page, ok := tools.Lookup(entityKey)
		if !ok {
			return entityDescriptor{}, fmt.Errorf("unknown tool %q: %w", entityKey, errpkg.ErrInvalid)
		}
		// A disabled tool page must not mint a thread — same leak gate as
		// content status/visibility (the flag write path drops live anchors).
		if !s.toolAnchorEnabled(ctx, entityKey) {
			return entityDescriptor{}, fmt.Errorf("tool %q is disabled: %w", entityKey, errpkg.ErrInvalid)
		}

		return entityDescriptor{
			URL:     "/tools/" + entityKey,
			Title:   page.Title,
			Summary: &page.Summary,
			SpaceID: spaceID,
		}, nil
	case "wiki_page":
		var pageID pgtype.UUID
		if err := pageID.Scan(entityKey); err != nil {
			return entityDescriptor{}, fmt.Errorf("wiki_page key must be a page UUID: %w", errpkg.ErrInvalid)
		}

		// wiki.enabled off means no /wiki pages — no public threads either.
		if s.flags != nil && !s.flags.IsEnabled(ctx, "wiki.enabled") {
			return entityDescriptor{}, fmt.Errorf("wiki is disabled: %w", errpkg.ErrInvalid)
		}

		page, err := s.read.GetWikiPageAnchorInfo(ctx, pageID)
		if err != nil {
			return entityDescriptor{}, err
		}

		summary := strings.TrimSpace(page.Excerpt)
		if summary == "" {
			summary = "Discussing a page in the " + page.SpaceName + " wiki."
		}

		// The page's display label is "{Space Name} {Page Title}" — the same
		// expansion GetWikiPage applies to dto.Title. {name} tokens in the
		// stored title expand to the space name first.
		title := strings.ReplaceAll(strings.TrimSpace(page.Title), "{name}", page.SpaceName)
		title = pageDisplayTitle(page.SpaceName, title)

		return entityDescriptor{
			URL:     "/wiki/" + page.SpaceSlug + "/" + page.PageSlug,
			Title:   title,
			Summary: &summary,
			SpaceID: spaceID,
		}, nil
	default:
		return entityDescriptor{}, fmt.Errorf("unsupported entity_type %q: %w", entityType, errpkg.ErrInvalid)
	}
}

// anchorThreadTitle composes a minted thread's title — "{Title} (A
// Thread)" for content and wiki pages (the wiki title is the page's
// "{Space} {Page}" display label), "{Name} Tool (A Thread)" for tools.
// The 001100 migration mirrors this for pre-existing minted wiki threads.
func anchorThreadTitle(entityType, title string) string {
	if entityType == "tool" {
		return title + " Tool (A Thread)"
	}
	return title + " (A Thread)"
}

// pageDisplayTitle is a wiki page's display label — "{Space Name} {Page
// Title}" by default ("Argentina Cost of Living"). A title already
// carrying the space name at a word boundary ("United Kingdom Visa
// Routes" from a {name} expansion, or an editor's own naming) isn't
// doubled. Mirrors wiki.pageDisplayTitle — the packages can't share the
// helper without a new dependency edge.
func pageDisplayTitle(spaceName, title string) string {
	t := strings.TrimSpace(title)
	if spaceName == "" || t == "" {
		return t
	}
	if strings.Contains(
		" "+strings.ToLower(t)+" ",
		" "+strings.ToLower(spaceName)+" ",
	) {
		return t
	}
	return spaceName + " " + t
}

// isGeneratedAnchorTitle reports whether a thread title still carries a
// minted form — the current "… (A Thread)" spec or a legacy template
// ("Discussing …", "Thread about …", "… — community thread"). An
// author-retitled thread doesn't match and is left alone by the
// drift heal.
func isGeneratedAnchorTitle(title string) bool {
	if strings.HasSuffix(title, " (A Thread)") ||
		strings.HasSuffix(title, " — community thread") ||
		strings.HasSuffix(title, " — Q&A thread") ||
		strings.HasSuffix(title, " — local thread") {
		return true
	}
	return strings.HasPrefix(title, "Discussing ") ||
		strings.HasPrefix(title, "Discussing: ") ||
		strings.HasPrefix(title, "Thread about ")
}

// CreateAnchor get-or-creates a thread anchor for an entity. The backing
// contents row is a published `discussion` authored by the caller, filed
// under the platform anchor space so comments aren't gated by a wiki-only
// or member-restricted space.
func (s *Service) CreateAnchor(ctx context.Context, userID pgtype.UUID, entityType, entityKey string) (AnchorSummary, error) {
	existing, err := s.ResolveAnchor(ctx, entityType, entityKey)
	// Any successful resolve is get-or-create "get": existing anchors return
	// their thread, and content `discussion` rows resolve to themselves —
	// they are their own thread, so "start a conversation" is a no-op link.
	if err == nil {
		// A `discussion` row resolving to itself has no backing anchor —
		// nothing below applies to it: no entity fields to refresh (the
		// upsert would insert a junk entity_key=content_id self-anchor),
		// no minted title to heal (it's user-authored, and would otherwise
		// append " (A Thread)" on every resolve).
		selfResolving := false
		if entityType == "content" {
			var pid pgtype.UUID
			if scanErr := pid.Scan(entityKey); scanErr == nil {
				selfResolving = pid == existing.ContentID
			}
		}
		// Heal drifted entity fields — page/article renames and revised
		// excerpts land here because the mint hooks (wiki approval, content
		// publish, lazy mint) re-enter CreateAnchor on every change. The
		// upsert refreshes display fields in place; the live-binding CASE
		// keeps content_id. Best-effort: a describe failure never blocks
		// the get.
		if desc, dErr := s.describeEntity(ctx, entityType, entityKey); dErr == nil {
			if !selfResolving && (existing.EntityURL != desc.URL || existing.EntityTitle != desc.Title ||
				!summaryEqual(existing.EntitySummary, desc.Summary)) {
				if _, uErr := s.write.CreateThreadAnchor(ctx, gen.CreateThreadAnchorParams{
					ContentID:     existing.ContentID,
					EntityType:    entityType,
					EntityKey:     entityKey,
					EntityUrl:     desc.URL,
					EntityTitle:   desc.Title,
					EntitySummary: desc.Summary,
				}); uErr != nil {
					slog.Warn("comments: anchor entity refresh failed", "entity_key", entityKey, "error", uErr)
				} else {
					existing.EntityURL = desc.URL
					existing.EntityTitle = desc.Title
					existing.EntitySummary = desc.Summary
				}
			}
			// Title + space heal — one read of the thread row covers both:
			//   - title: a minted title is "{Entity} (A Thread)" (or a
			//     legacy "Discussing …" template); still carrying a
			//     generated form means it was never author-retitled, so an
			//     entity rename retitles the thread here.
			//   - space: the thread files under the parent's primary
			//     space — a public→public re-file (UpdateContentSpaces)
			//     leaves it under the stale space until this re-homes it.
			if tRow, tErr := s.read.GetContentByID(ctx, existing.ContentID); !selfResolving && tErr == nil {
				healed := false
				expectedTitle := anchorThreadTitle(entityType, desc.Title)
				if tRow.Title != expectedTitle && isGeneratedAnchorTitle(tRow.Title) {
					if uErr := s.write.SetThreadRowTitle(ctx, gen.SetThreadRowTitleParams{
						ID:       existing.ContentID,
						Title:    expectedTitle,
						OldTitle: tRow.Title,
					}); uErr != nil {
						slog.Warn("comments: anchor thread retitle failed",
							"entity_key", entityKey, "error", uErr)
					} else {
						healed = true
					}
				}
				if entityType == "content" && desc.SpaceID.Valid && tRow.SpaceID != desc.SpaceID {
					if uErr := s.write.SetThreadRowSpaceID(ctx, gen.SetThreadRowSpaceIDParams{
						ID:      existing.ContentID,
						SpaceID: desc.SpaceID,
					}); uErr != nil {
						slog.Warn("comments: anchor thread space re-home failed",
							"entity_key", entityKey, "error", uErr)
					} else {
						healed = true
					}
				}
				// The raw heal writes skip UpdateContent's edge purge —
				// a stale /thread/{slug} page would otherwise serve the
				// old title/space badge until TTL (ADR-134). Fire-and-
				// forget, no-op when no CDN is configured.
				if healed {
					cdn.PurgeContentDetail(ctx, tRow.ID.String(), tRow.ContentType, tRow.Slug)
				}
			}
		}
		// Heal stray comments on the entity row — a mint interrupted between
		// link and reparent, or a comment written straight to the article —
		// so one entity never shows two conversations. Idempotent no-op once
		// the entity row's comments are all on the thread.
		if entityType == "content" {
			var pid pgtype.UUID
			if scanErr := pid.Scan(entityKey); scanErr == nil && pid != existing.ContentID {
				if repErr := s.reparentComments(ctx, pid, existing.ContentID); repErr != nil {
					slog.Warn("comments: reparent comments on resolve failed", "entity_key", entityKey, "error", repErr)
				}
			}
		}
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return AnchorSummary{}, fmt.Errorf("resolving thread anchor: %w", err)
	}

	desc, err := s.describeEntity(ctx, entityType, entityKey)
	if err != nil {
		return AnchorSummary{}, err
	}

	// Restore over re-mint: a surviving anchor whose thread is soft-deleted
	// means the entity was unpublished/hidden, not destroyed — restoring
	// the row brings back its comments, votes and canonical /thread/{slug}
	// URL instead of starting an empty thread on a new slug. Re-entering
	// CreateAnchor then runs the full "get" path: entity-field upsert,
	// title/space heals and the comment reparent.
	if raw, aErr := s.read.GetThreadAnchorRow(ctx, gen.GetThreadAnchorRowParams{
		EntityType: entityType,
		EntityKey:  entityKey,
	}); aErr == nil {
		if restored, rErr := s.write.RestoreThreadRow(ctx, raw.ContentID); rErr == nil {
			if scErr := s.write.EnsureContentScore(ctx, restored); scErr != nil {
				slog.Warn("comments: ensure content_scores on thread restore failed",
					"content_id", restored.String(), "error", scErr)
			}
			if rcErr := s.write.RecountContentCommentCount(ctx, restored); rcErr != nil {
				slog.Warn("comments: recount on thread restore failed",
					"content_id", restored.String(), "error", rcErr)
			}
			if vsErr := s.write.IncrementScoreVote(ctx, restored); vsErr != nil {
				slog.Warn("comments: vote rescore on thread restore failed",
					"content_id", restored.String(), "error", vsErr)
			}
			// The restored page was 404ing at the edge while deleted —
			// drop any cached negative response (ADR-134).
			cdn.PurgeContentDetail(ctx, restored.String(), "discussion")
			return s.CreateAnchor(ctx, userID, entityType, entityKey)
		} else if !errors.Is(rErr, pgx.ErrNoRows) {
			slog.Warn("comments: thread restore failed",
				"entity_key", entityKey, "error", rErr)
		}
	}

	if s.creator == nil {
		return AnchorSummary{}, fmt.Errorf("content creator not wired")
	}

	title := anchorThreadTitle(entityType, desc.Title)

	// The thread body starts empty — /thread/{slug} renders the entity's
	// excerpt (embed card / parent-article preview) as the default context,
	// and the author may inline-edit to add their own text. No generated
	// prose is written into the post body.

	// Same uniqueness trick as StartConversation: the hash input carries the
	// entity coordinates + a fresh UUID so repeat anchors can't collide on
	// the contents content_hash index (dedup is meaningless for generated
	// rows).
	hashInput := strings.ToLower(strings.Join(strings.Fields(title), " ")) +
		"|" + entityType + ":" + entityKey + "|" + uuid.NewString()
	hashSum := sha256.Sum256([]byte(hashInput))
	contentHash := hex.EncodeToString(hashSum[:])

	visibility := desc.Visibility
	if visibility == "" {
		visibility = content.VisibilityPublic
	}

	// The minted thread is authored by the entity's owner when it has one —
	// a visitor who triggers a lazy mint on someone else's article must not
	// end up owning (and moderating) its thread. Communal entities (tools,
	// wiki pages) attribute to the platform bot author instead.
	authorID := desc.AuthorID
	if !authorID.Valid {
		authorID = s.anchorBotUser(ctx, userID)
	}

	row, err := s.creator.CreateContent(ctx, content.CreateContentInput{
		AuthorID:    authorID,
		ContentType: "discussion",
		Title:       title,
		ContentHash: &contentHash,
		SpaceID:     desc.SpaceID,
		GeoSpaceID:  desc.GeoSpaceID,
		Status:      content.StatusPublished,
		Visibility:  visibility,
		// System-generated rows bypass the moderation clamp — the anchor is
		// a container for comments, not authored prose (same rationale as
		// StartConversation's derived threads).
		SkipStoryModeration: true,
	})
	if err != nil {
		return AnchorSummary{}, fmt.Errorf("creating anchor thread: %w", err)
	}

	// Feeds INNER JOIN content_scores — a row without one never lists.
	// Minted rows bypass the publish path that usually ensures this.
	if err := s.write.EnsureContentScore(ctx, row.ID); err != nil {
		slog.Warn("comments: ensure content_scores on anchor mint failed",
			"content_id", row.ID.String(), "error", err)
	}

	// Content threads also record the §49 parent link — GetThreadParentByThreadID
	// resolves it into the thread page's canonical-content embed card, and
	// comment counts propagate up to the article via the link. Ensure, not
	// the share upsert — a structural binding must not inflate link_count.
	if entityType == "content" {
		var parentID pgtype.UUID
		if err := parentID.Scan(entityKey); err == nil {
			if err := s.write.EnsureContentThreadLink(ctx, gen.EnsureContentThreadLinkParams{
				ContentID: parentID,
				ThreadID:  row.ID,
			}); err != nil {
				return AnchorSummary{}, fmt.Errorf("linking anchor thread to parent content: %w", err)
			}
		}
	}

	anchor, err := s.write.CreateThreadAnchor(ctx, gen.CreateThreadAnchorParams{
		ContentID:     row.ID,
		EntityType:    entityType,
		EntityKey:     entityKey,
		EntityUrl:     desc.URL,
		EntityTitle:   desc.Title,
		EntitySummary: desc.Summary,
	})
	if err != nil {
		return AnchorSummary{}, fmt.Errorf("creating thread anchor: %w", err)
	}

	// Lost the get-or-create race: ON CONFLICT kept the earlier row's
	// content_id, so our minted contents row is unanchored. Soft-delete it —
	// a live duplicate thread would list as a standalone discussion and its
	// link would double-count on the parent — then re-resolve to return
	// the winning anchor's slug. (The reparent below runs only on the
	// winning path — comments must never move onto an orphan.)
	if anchor.ContentID != row.ID {
		if s.creator != nil {
			if delErr := s.creator.DeleteContent(ctx, row.ID); delErr != nil {
				slog.Warn("comments: delete losing anchor-mint row failed",
					"content_id", row.ID.String(), "error", delErr)
			}
		}
		return s.ResolveAnchor(ctx, entityType, entityKey)
	}

	// Comments posted on the article before its thread existed — guest
	// kind='self' fallback, pre-migration rows, or direct API writes —
	// move onto the canonical thread so the conversation never splits
	// across the two rows. Best-effort after the anchor is durable: the
	// existing-anchor path above re-runs the move on every resolve.
	if entityType == "content" {
		var parentID pgtype.UUID
		if err := parentID.Scan(entityKey); err == nil {
			if repErr := s.reparentComments(ctx, parentID, row.ID); repErr != nil {
				slog.Warn("comments: reparent comments on mint failed", "entity_key", entityKey, "error", repErr)
			}
		}
	}

	return AnchorSummary{
		ContentID:     anchor.ContentID,
		Slug:          row.Slug,
		Title:         row.Title,
		EntityType:    anchor.EntityType,
		EntityKey:     anchor.EntityKey,
		EntityURL:     anchor.EntityUrl,
		EntityTitle:   anchor.EntityTitle,
		EntitySummary: anchor.EntitySummary,
		Created:       true,
	}, nil
}

// summaryEqual compares two nullable entity summaries.
func summaryEqual(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// reparentComments moves comments left on an entity row (posted before its
// thread was minted, or written straight to the article via the API) onto
// its canonical thread, then resyncs both rows' comment_count. Idempotent —
// safe to run on every anchor resolution. The cheap existence probe short-
// circuits the common steady state (nothing to move) so an anchor resolve
// doesn't write five rows on every call.
func (s *Service) reparentComments(ctx context.Context, from, to pgtype.UUID) error {
	if n, err := s.read.CountCommentsByContent(ctx, from); err == nil && n == 0 {
		return nil
	}
	if err := s.write.ReparentCommentsToThread(ctx, gen.ReparentCommentsToThreadParams{
		ThreadID:  to,
		ContentID: from,
	}); err != nil {
		return fmt.Errorf("reparenting comments to thread: %w", err)
	}
	for _, id := range []pgtype.UUID{from, to} {
		if err := s.write.RecountContentCommentCount(ctx, id); err != nil {
			return fmt.Errorf("recounting comment_count: %w", err)
		}
		// comment_votes follow their comment's content_id — the moved votes
		// leave the entity row's union tally and join the thread's, so both
		// rows' vote_score need the drift-proof recompute.
		if err := s.write.IncrementScoreVote(ctx, id); err != nil {
			return fmt.Errorf("recomputing vote_score: %w", err)
		}
	}
	return nil
}

// DropAnchorsForSpace drops every live thread anchored to a space's wiki
// pages AND every content thread filed under it — called when the space
// leaves public/active so public thread pages stop leaking titles and
// excerpts (the mint gates only admit public, active spaces). Re-entry is
// a no-op; threads re-mint lazily once the space reopens.
func (s *Service) DropAnchorsForSpace(ctx context.Context, spaceID pgtype.UUID) error {
	wikiKeys, err := s.read.ListWikiAnchorsBySpace(ctx, spaceID)
	if err != nil {
		return fmt.Errorf("listing wiki anchors for space: %w", err)
	}
	for _, key := range wikiKeys {
		if err := s.DeleteAnchor(ctx, "wiki_page", key); err != nil {
			slog.Warn("comments: drop wiki anchor failed",
				"entity_key", key, "error", err)
		}
	}
	contentKeys, err := s.read.ListContentAnchorsBySpace(ctx, spaceID)
	if err != nil {
		return fmt.Errorf("listing content anchors for space: %w", err)
	}
	for _, key := range contentKeys {
		if err := s.DeleteAnchor(ctx, "content", key); err != nil {
			slog.Warn("comments: drop content anchor failed",
				"entity_key", key, "error", err)
		}
	}
	return nil
}

// ReconcileAnchorsWithFlags drops live threads whose entity's gating flag
// is off — tool pages (per-slug flag), the whole wiki surface
// (wiki.enabled), and content whose parent's type flag is off
// (jobs.enabled, video.enabled, … via §42 GatedContentTypes). Called from
// the flag-write hook in server.go; idempotent and safe to run at startup.
func (s *Service) ReconcileAnchorsWithFlags(ctx context.Context) {
	if s.flags == nil {
		return
	}
	for _, page := range tools.Pages {
		if page.Flag != "" && !s.flags.IsEnabled(ctx, page.Flag) {
			if err := s.DeleteAnchor(ctx, "tool", page.Slug); err != nil {
				slog.Warn("comments: drop disabled-tool anchor failed",
					"entity_key", page.Slug, "error", err)
			}
		}
	}
	if !s.flags.IsEnabled(ctx, "wiki.enabled") {
		s.dropAnchorKeys(ctx, "wiki_page", s.listAnchorKeys(ctx, "wiki_page"))
	}
	// Global IsEnabled, deliberately not EvalFlagScoped: the config-change
	// callback's context.WithoutCancel propagates the writing admin's
	// CallerScope, and a partially-rolled-out flag would scoped-evaluate
	// "disabled" for that one caller — bulk deletion must follow the
	// flag's global live state, not one scope's projection.
	for ct, flag := range rssmodel.ContentTypeFeatureFlag {
		if !s.flags.IsEnabled(ctx, flag) {
			s.dropAnchorKeys(ctx, "content", s.listContentAnchorKeys(ctx, ct))
		}
	}
}

// listAnchorKeys / listContentAnchorKeys fetch live anchor keys for a
// reconcile sweep — errors collapse to an empty list (a failed list drops
// nothing; the next reconcile retries).
func (s *Service) listAnchorKeys(ctx context.Context, entityType string) []string {
	keys, err := s.read.ListAnchorKeysByType(ctx, entityType)
	if err != nil {
		slog.Warn("comments: list anchors by type failed",
			"entity_type", entityType, "error", err)
		return nil
	}
	return keys
}

func (s *Service) listContentAnchorKeys(ctx context.Context, contentType string) []string {
	keys, err := s.read.ListContentAnchorKeysByParentType(ctx, contentType)
	if err != nil {
		slog.Warn("comments: list anchors by parent type failed",
			"content_type", contentType, "error", err)
		return nil
	}
	return keys
}

// dropAnchorKeys runs DeleteAnchor per key — the loop keeps the full
// delete side-effects (content.deleted webhook, edge purge) a bulk
// UPDATE would skip.
func (s *Service) dropAnchorKeys(ctx context.Context, entityType string, keys []string) {
	for _, key := range keys {
		if err := s.DeleteAnchor(ctx, entityType, key); err != nil {
			slog.Warn("comments: reconcile anchor drop failed",
				"entity_type", entityType, "entity_key", key, "error", err)
		}
	}
}

// DeleteAnchor drops the entity's backing thread — soft-deletes the
// contents row so /thread/{slug} drops out of resolve (the anchor query
// filters deleted contents) and listings, without destroying the data.
// Called when the anchored entity itself is deleted. No-op when no
// anchor exists.
func (s *Service) DeleteAnchor(ctx context.Context, entityType, entityKey string) error {
	row, err := s.read.GetThreadAnchorByEntity(ctx, gen.GetThreadAnchorByEntityParams{
		EntityType: entityType,
		EntityKey:  entityKey,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("resolving thread anchor for delete: %w", err)
	}
	if s.creator == nil {
		return fmt.Errorf("content deleter not wired")
	}
	return s.creator.DeleteContent(ctx, row.ContentID)
}

// ---------------------------------------------------------------------------
// HTTP Handlers
// ---------------------------------------------------------------------------

// ResolveAnchor handles GET /api/v1/threads/anchor/{entityType}/{entityKey}.
func (h *Handlers) ResolveAnchor(w http.ResponseWriter, r *http.Request) {
	entityType := chi.URLParam(r, "entityType")
	entityKey := chi.URLParam(r, "entityKey")
	if entityType == "" || entityKey == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_anchor", "entity_type and entity_key are required")
		return
	}

	anchor, err := h.Service.ResolveAnchor(r.Context(), entityType, entityKey)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "No thread for this entity yet")
			return
		}
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": anchor})
}

// CreateAnchor handles POST /api/v1/threads/anchor.
func (h *Handlers) CreateAnchor(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	var input struct {
		EntityType string `json:"entity_type"`
		EntityKey  string `json:"entity_key"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
			return
		}
	}

	input.EntityType = strings.TrimSpace(input.EntityType)
	input.EntityKey = strings.TrimSpace(input.EntityKey)
	if input.EntityType == "" || input.EntityKey == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "entity_type and entity_key are required")
		return
	}

	anchor, err := h.Service.CreateAnchor(r.Context(), user.UserID, input.EntityType, input.EntityKey)
	if err != nil {
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Entity not found")
		case errors.Is(err, errpkg.ErrInvalid):
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_entity", err.Error())
		case errors.Is(err, content.ErrMissingPrimarySpace):
			errpkg.WriteError(w, http.StatusUnprocessableEntity, "missing_primary_space",
				"Content requires a primary space before publishing or scheduling")
		default:
			handleServiceError(w, err)
		}
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": anchor})
}
