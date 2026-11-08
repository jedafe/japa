package comments

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
	"arrival-checklist":  {"Arrival Checklist", "A personalized arrival checklist for your destination."},
	"attestation-guide":  {"Attestation Guide", "Step-by-step document attestation guidance."},
	"community-locator":  {"Community Locator", "Find diaspora communities near you."},
	"cost-of-living":     {"Cost of Living Calculator", "Compare living costs across destination cities."},
	"credential-checker": {"Credential Checker", "Check how your credentials transfer abroad."},
	"crs-calculator":     {"CRS Calculator", "Estimate your Comprehensive Ranking System score."},
	"currency-converter": {"Currency Converter", "Convert between origin and destination currencies."},
	"cv-formatter":       {"CV Formatter", "Format your CV for your destination market."},
	"eca-guide":          {"ECA Guide", "Educational Credential Assessment guidance."},
	"embassy-alerter":    {"Embassy Alerter", "Alerts for embassy appointment availability."},
	"housing-guide":      {"Housing Guide", "Housing guides for your destination city."},
	"language-test":      {"Language Test Hub", "Prepare for IELTS, TOEFL and other language tests."},
	"pof-calculator":     {"Proof of Funds Calculator", "Compute proof-of-funds requirements."},
	"processing-tracker": {"Processing Time Tracker", "Track visa processing times."},
	"readiness":          {"Readiness Assessment", "Assess your readiness to relocate abroad."},
	"remittance":         {"Remittance Rate Tracker", "Track live remittance rates across corridors."},
	"sop-generator":      {"SOP Generator", "Generate a statement of purpose draft."},
	"tax-calculator":     {"Tax Calculator", "Estimate taxes in your destination country."},
	"travel":             {"Travel Search", "Search and compare flights and hotels."},
	"visa-checklist":     {"Visa Checklist", "Generate your visa document checklist by destination."},
	"visa-routes":        {"Visa Routes", "Compare visa routes for your destination."},
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

// ResolveAnchor returns the thread anchor bound to an entity, or ErrNotFound.
// entity_type='content' is special-cased: the contents row itself IS the
// thread (/thread/{slug} renders its embed + comments), so no anchor row is
// needed — resolve returns the row's own identity.
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

		return AnchorSummary{
			ContentID:    row.ID,
			Slug:         row.Slug,
			Title:        row.Title,
			CommentCount: row.CommentCount,
			EntityType:   "content",
			EntityKey:    entityKey,
		}, nil
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
	// SpaceID the anchor contents row files under — always the platform
	// anchor space today (see anchorPlatformSpaceSlug).
	SpaceID pgtype.UUID
}

// describeEntity validates the requested anchor target and resolves its
// display fields server-side (never trusting client-supplied titles).
func (s *Service) describeEntity(ctx context.Context, entityType, entityKey string) (entityDescriptor, error) {
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

// CreateAnchor get-or-creates a thread anchor for an entity. The backing
// contents row is a published `discussion` authored by the caller, filed
// under the platform anchor space so comments aren't gated by a wiki-only
// or member-restricted space.
func (s *Service) CreateAnchor(ctx context.Context, userID pgtype.UUID, entityType, entityKey string) (AnchorSummary, error) {
	existing, err := s.ResolveAnchor(ctx, entityType, entityKey)
	// A 'content' entity resolves to the row itself — the thread already
	// exists, so "start a conversation" is a no-op deep link.
	if err == nil && entityType == "content" {
		return existing, nil
	}
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

	// Same uniqueness trick as StartConversation: the hash input carries the
	// entity coordinates + a fresh UUID so repeat anchors can't collide on
	// the contents content_hash index (dedup is meaningless for generated
	// rows).
	hashInput := strings.ToLower(strings.Join(strings.Fields(title), " ")) +
		"|" + entityType + ":" + entityKey + "|" + uuid.NewString()
	hashSum := sha256.Sum256([]byte(hashInput))
	contentHash := hex.EncodeToString(hashSum[:])

	row, err := s.creator.CreateContent(ctx, content.CreateContentInput{
		AuthorID:    userID,
		ContentType: "discussion",
		Title:       title,
		Body:        "",
		ContentHash: &contentHash,
		SpaceID:     desc.SpaceID,
		Status:      content.StatusPublished,
		Visibility:  content.VisibilityPublic,
		// System-generated rows bypass the moderation clamp — the anchor is
		// a container for comments, not authored prose (same rationale as
		// StartConversation's derived threads).
		SkipStoryModeration: true,
	})
	if err != nil {
		return AnchorSummary{}, fmt.Errorf("creating anchor thread: %w", err)
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
