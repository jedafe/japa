package comments

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
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

// toolAnchorMeta mirrors the indexable /tools/{tool} pages
// (seo.mainAppToolPaths + the names surfaced on the /tools hub). Only these
// keys may bind a 'tool' anchor — arbitrary slugs are rejected so the
// endpoint cannot mint threads for non-existent pages.
var toolAnchorMeta = map[string]struct {
	Title   string
	Summary string
}{
	"arrival-checklist":  {"Arrival Checklist", "A personalized checklist for your first weeks after landing — documents to carry, registrations to complete, bank, phone, transport, and the tasks people commonly forget. Built per destination so nothing slips through."},
	"attestation-guide":  {"Attestation Guide", "Step-by-step guidance for getting your documents attested — which offices to visit, what order to follow, and the fees and timelines at each stage. Covers Nigerian and destination-country requirements."},
	"community-locator":  {"Community Locator", "Find diaspora communities near you — associations, religious centres, cultural groups, and informal networks organised by city and country of origin."},
	"cost-of-living":     {"Cost of Living Calculator", "Compare real living costs across destination cities — rent, food, transport, childcare, and utilities — side by side with your origin city so you can budget before you move."},
	"credential-checker": {"Credential Checker", "Check how your degrees and professional credentials transfer abroad — recognition status, evaluation bodies, and the extra steps each destination requires."},
	"crs-calculator":     {"CRS Calculator", "Estimate your Comprehensive Ranking System score for Canadian Express Entry — age, education, language, and work experience broken down per factor so you can see where to gain points."},
	"currency-converter": {"Currency Converter", "Convert between your home and destination currencies with live rates — useful for budgeting, remittances, and comparing salaries across countries."},
	"cv-formatter":       {"CV Formatter", "Format your CV for your destination market — the structure, length, and conventions recruiters in that country actually expect."},
	"eca-guide":          {"ECA Guide", "Educational Credential Assessment guidance — which ECA bodies evaluate your qualifications, how long each takes, and which one your destination accepts."},
	"embassy-alerter":    {"Embassy Alerter", "Alerts when embassy and visa application centre appointment slots open — interview calendars fill fast and checking manually is a losing game."},
	"housing-guide":      {"Housing Guide", "Housing guides for destination cities — where newcomers typically live, what deposits and paperwork landlords expect, and realistic rent ranges by area."},
	"language-test":      {"Language Test Hub", "Prepare for IELTS, TOEFL, PTE and other language tests — target scores per visa route, prep resources, and booking links."},
	"pof-calculator":     {"Proof of Funds Calculator", "Compute proof-of-funds requirements for your destination — minimum balances, how many months of statements, and acceptable account types."},
	"processing-tracker": {"Processing Time Tracker", "Track current visa and permit processing times — real submission-to-decision timelines by route, so you can plan flights and resignations sensibly."},
	"readiness":          {"Readiness Assessment", "Assess how ready you are to relocate — documents, finances, credentials, and timeline scored so you know exactly what's left before you can move."},
	"remittance":         {"Remittance Rate Tracker", "Track live remittance rates and fees across corridors — compare providers before you send money home."},
	"sop-generator":      {"SOP Generator", "Draft a statement of purpose for admissions or visa applications — structured around your background, goals, and the programme you're targeting."},
	"tax-calculator":     {"Tax Calculator", "Estimate income tax and take-home pay in your destination country — so salary offers can be compared on net, not gross."},
	"travel":             {"Travel Search", "Search and compare flights and hotels for your route — prices across airlines and dates so you can book the legwork smartly."},
	"visa-checklist":     {"Visa Checklist", "Generate your visa document checklist by destination — every required document, form, and supporting evidence for your route, in submission order."},
	"visa-routes":        {"Visa Routes", "Compare visa routes for your destination — eligibility, cost, timeline, and pathway-to-residency for each option."},
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

// contentEntityURL mirrors the frontend CONTENT_ROUTE_PREFIX map
// (contentLink.ts) so thread_anchors.entity_url points at the article's
// canonical detail route — the embed card and any URL consumers land on
// the real content page, not a discussion stub.
func contentEntityURL(contentType, slug string) string {
	prefix := map[string]string{
		"blog_post":       "/blog",
		"guide":           "/guide",
		"discussion":      "/thread",
		"forum_thread":    "/thread",
		"comparison":      "/comparisons",
		"story":           "/story",
		"achievement":     "/achievements",
		"page":            "/pages",
		"job_listing":     "/jobs",
		"service_listing": "/services",
		"video":           "/video",
		"list":            "/list",
	}[contentType]
	if prefix == "" {
		prefix = "/content"
	}
	return prefix + "/" + slug
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
		// viewer could load on an entity page. Drafts, pending-moderation
		// and non-public rows must not leak title/slug/comment_count.
		if row.Status != content.StatusPublished || row.Visibility != content.VisibilityPublic {
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

			// Only live, public rows mint a thread: drafts/review/private rows
			// have no public thread page, and a minted thread would leak the
			// unpublished title via /thread/{slug}. Discussion rows resolve to
			// themselves before reaching here; forum_thread rows already have
			// a thread surface.
			if row.Status != content.StatusPublished || row.Visibility != content.VisibilityPublic {
				return entityDescriptor{}, fmt.Errorf("content is not publicly live: %w", errpkg.ErrInvalid)
			}
			if row.ContentType == "discussion" || row.ContentType == "forum_thread" {
				return entityDescriptor{}, fmt.Errorf("content_type %q is already a thread: %w", row.ContentType, errpkg.ErrInvalid)
			}

			// Same leak guard as wiki pages: the thread files under the
			// parent's primary space (content_spaces is authoritative;
			// contents.space_id is the legacy fallback), so only
			// public+active spaces may bind a public anchor.
			spaceID := row.SpaceID
			if spaces, err := s.read.GetContentSpaces(ctx, row.ID); err == nil {
				for _, sp := range spaces {
					if sp.IsPrimary {
						spaceID = sp.SpaceID
						break
					}
				}
			}
			if !spaceID.Valid {
				return entityDescriptor{}, fmt.Errorf("content has no primary space: %w", errpkg.ErrInvalid)
			}
			space, err := s.read.GetSpaceByID(ctx, spaceID)
			if err != nil {
				return entityDescriptor{}, err
			}
			if space.Visibility != "public" || space.Status != "active" || space.DeletedAt.Valid {
				return entityDescriptor{}, fmt.Errorf("content space is not public: %w", errpkg.ErrInvalid)
			}

			summary := ""
			if row.MetaDescription != nil {
				summary = strings.TrimSpace(*row.MetaDescription)
			}
			if summary == "" {
				summary = content.ExcerptFromBody(row.Body, "", 160)
			}

			return entityDescriptor{
				URL:        contentEntityURL(row.ContentType, row.Slug),
				Title:      row.Title,
				Summary:    &summary,
				SpaceID:    spaceID,
				GeoSpaceID: row.GeoSpaceID,
				Visibility: row.Visibility,
			}, nil
		}
	}

	spaceID, err := s.anchorSpace(ctx)
	if err != nil {
		return entityDescriptor{}, err
	}

	switch entityType {
	case "tool":
		meta, ok := toolAnchorMeta[entityKey]
		if !ok {
			return entityDescriptor{}, fmt.Errorf("unknown tool %q: %w", entityKey, errpkg.ErrInvalid)
		}

		return entityDescriptor{
			URL:     "/tools/" + entityKey,
			Title:   meta.Title,
			Summary: &meta.Summary,
			SpaceID: spaceID,
		}, nil
	case "wiki_page":
		var pageID pgtype.UUID
		if err := pageID.Scan(entityKey); err != nil {
			return entityDescriptor{}, fmt.Errorf("wiki_page key must be a page UUID: %w", errpkg.ErrInvalid)
		}

		page, err := s.read.GetWikiPageAnchorInfo(ctx, pageID)
		if err != nil {
			return entityDescriptor{}, err
		}

		summary := "Discussing a page in the " + page.SpaceName + " wiki."

		return entityDescriptor{
			URL:     "/wiki/" + page.SpaceSlug + "/" + page.PageSlug,
			Title:   page.Title,
			Summary: &summary,
			SpaceID: spaceID,
		}, nil
	default:
		return entityDescriptor{}, fmt.Errorf("unsupported entity_type %q: %w", entityType, errpkg.ErrInvalid)
	}
}

// anchorBodyDoc renders an anchor thread's body as a TipTap document: the
// entity description paragraph, then a prompt line that invites
// conversation about the entity.
func anchorBodyDoc(description, title string) string {
	text := func(s string) map[string]any {
		return map[string]any{
			"type":    "paragraph",
			"content": []any{map[string]any{"type": "text", "text": s}},
		}
	}
	doc := map[string]any{
		"type":    "doc",
		"content": []any{text(description), text("Questions, experiences, and tips about " + title + " — join the discussion below.")},
	}
	b, err := json.Marshal(doc)
	if err != nil {
		return ""
	}
	return string(b)
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
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return AnchorSummary{}, fmt.Errorf("resolving thread anchor: %w", err)
	}

	desc, err := s.describeEntity(ctx, entityType, entityKey)
	if err != nil {
		return AnchorSummary{}, err
	}

	if s.creator == nil {
		return AnchorSummary{}, fmt.Errorf("content creator not wired")
	}

	title := "Discussing: " + desc.Title

	// The thread body carries the entity's real description plus a
	// conversation prompt — /thread/{slug} renders it as the body section,
	// so anchor threads aren't empty shells (unified-discussion spec).
	// body_html is supplied explicitly: createContent would otherwise run
	// the TipTap JSON through the markdown renderer and store the doc as
	// one literal-JSON paragraph.
	body, bodyHTML := "", ""
	if desc.Summary != nil && *desc.Summary != "" {
		body = anchorBodyDoc(*desc.Summary, desc.Title)
		bodyHTML = "<p>" + html.EscapeString(*desc.Summary) + "</p>" +
			"<p>" + html.EscapeString("Questions, experiences, and tips about "+desc.Title+" — join the discussion below.") + "</p>"
	}

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

	row, err := s.creator.CreateContent(ctx, content.CreateContentInput{
		AuthorID:    userID,
		ContentType: "discussion",
		Title:       title,
		Body:        body,
		BodyHtml:    bodyHTML,
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

	// Content threads also record the §49 parent link — GetThreadParentByThreadID
	// resolves it into the thread page's canonical-content embed card, and
	// comment counts propagate up to the article via the link.
	if entityType == "content" {
		var parentID pgtype.UUID
		if err := parentID.Scan(entityKey); err == nil {
			if _, err := s.write.CreateContentThreadLink(ctx, gen.CreateContentThreadLinkParams{
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
	// content_id, so our minted contents row is unanchored. Re-resolve to
	// return the winning anchor's slug instead of the orphan's.
	if anchor.ContentID != row.ID {
		return s.ResolveAnchor(ctx, entityType, entityKey)
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
	}, nil
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
