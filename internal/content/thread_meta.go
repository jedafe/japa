package content

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	rssmodel "github.com/jedafe/japa/internal/rss/model"
)

// Thread-context lookups backing the /thread/{slug} embed card (thread-
// rename spec). Two thread kinds resolve through a contents row:
//
//   - anchor thread — a lazily-created `discussion` row bound to a
//     non-content entity (tool page, wiki page) via thread_anchors;
//   - §49 linked discussion — a `discussion` row created about another
//     content via content_thread_links (StartConversation).
//
// Both enrich GET /content/{slug} with optional `thread_anchor` /
// `thread_parent` envelope fields so the thread page can render the
// embedded entity without extra round-trips. Only `discussion` rows can
// carry either link — the handler skips the lookups for other types.

// ThreadAnchorMeta is the entity descriptor stored on thread_anchors.
type ThreadAnchorMeta struct {
	EntityType    string  `json:"entity_type"`
	EntityKey     string  `json:"entity_key"`
	EntityURL     string  `json:"entity_url"`
	EntityTitle   string  `json:"entity_title"`
	EntitySummary *string `json:"entity_summary,omitempty"`
}

// ThreadParentMeta is the parent content a linked-discussion thread was
// started about — enough for the thread page's embed card to render title,
// excerpt, and cover without a second fetch. URL is the parent's canonical
// public detail URL — resolved server-side because some routes (roadmap
// shadow rows) can't be reconstructed from the content slug alone.
type ThreadParentMeta struct {
	Slug                  string  `json:"slug"`
	Title                 string  `json:"title"`
	ContentType           string  `json:"content_type"`
	Excerpt               string  `json:"excerpt"`
	URL                   string  `json:"url"`
	FeaturedStorageBucket *string `json:"-"`
	FeaturedStoragePath   *string `json:"-"`
}

// ThreadAnchorFor returns the anchor descriptor for a contents row, or nil
// when the row carries no anchor.
func (s *Service) ThreadAnchorFor(ctx context.Context, contentID pgtype.UUID) (*ThreadAnchorMeta, error) {
	row, err := s.read.GetThreadAnchorByContentID(ctx, contentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, err
	}

	return &ThreadAnchorMeta{
		EntityType:    row.EntityType,
		EntityKey:     row.EntityKey,
		EntityURL:     row.EntityUrl,
		EntityTitle:   row.EntityTitle,
		EntitySummary: row.EntitySummary,
	}, nil
}

// ThreadParentFor returns the content a linked-discussion thread was
// started about, or nil when the row is not a link target.
func (s *Service) ThreadParentFor(ctx context.Context, contentID pgtype.UUID) (*ThreadParentMeta, error) {
	row, err := s.read.GetThreadParentByThreadID(ctx, contentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, err
	}

	excerpt := ""
	if row.MetaDescription != nil {
		excerpt = *row.MetaDescription
	}
	if excerpt == "" {
		excerpt = ExcerptFromBody(row.Body, "", 160)
	}

	// A roadmap shadow row's public page is /roadmap/{item.slug} — its own
	// slug is "roadmap-{item.slug}", so LinkPathFor's /content fallback
	// would point the embed card (and shell canonicals) at a stub page.
	url := rssmodel.LinkPathFor(row.ContentType) + row.Slug
	if row.RoadmapItemSlug != nil && *row.RoadmapItemSlug != "" {
		url = "/roadmap/" + *row.RoadmapItemSlug
	}

	return &ThreadParentMeta{
		Slug:                  row.Slug,
		Title:                 row.Title,
		ContentType:           row.ContentType,
		Excerpt:               excerpt,
		URL:                   url,
		FeaturedStorageBucket: row.FeaturedStorageBucket,
		FeaturedStoragePath:   row.FeaturedStoragePath,
	}, nil
}
