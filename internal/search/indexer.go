// Package search — indexer.go owns the document sync path: fetching entity
// rows from PostgreSQL and upserting them into the Meilisearch indexes.
//
// The Indexer implements the worker.SearchIndexer interface so the Asynq
// search_index_sync task can stay thin — it parses the payload and delegates
// here. Keeping the DB fetch and document shaping in the search package means
// the index schema lives in one place next to the query code that reads it
// back.
package search

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/meilisearch/meilisearch-go"
)

// maxExcerptRunes caps the excerpt stored in the search document so the
// index stays small and result previews stay readable.
const maxExcerptRunes = 200

// Indexer syncs content documents into the Meilisearch posts index. It reads
// from the reader pool and writes to the search engine — it never touches the
// writer pool because indexing is a read of the source-of-truth row, not a
// mutation of application state.
type Indexer struct {
	client meilisearch.ServiceManager
	read   *gen.Queries
}

// NewIndexer creates an Indexer backed by the given Meilisearch client and
// database read queries. Either may be nil: a nil client makes IndexContent
// a logged no-op, and a nil read set is a programming error that surfaces on
// the first call.
func NewIndexer(client meilisearch.ServiceManager, read *gen.Queries) *Indexer {
	return &Indexer{client: client, read: read}
}

// contentDocument is the shape stored in the posts index. Field names match
// the Meilisearch searchable/filterable attributes.
type contentDocument struct {
	ID                string   `json:"id"`
	TenantID          string   `json:"tenant_id"`
	Title             string   `json:"title"`
	Slug              string   `json:"slug"`
	Excerpt           string   `json:"excerpt"`
	ContentType       string   `json:"content_type"`
	URL               string   `json:"url"`
	Status            string   `json:"status"`
	RelevantCountries []string `json:"relevant_countries"`
}

// IndexContent fetches the content row identified by contentID+tenantID and
// upserts it into the posts index. AddDocuments is an upsert in Meilisearch —
// a document with the same primary key is replaced — so this handles both
// create and update without a separate code path.
func (ix *Indexer) IndexContent(ctx context.Context, contentID, tenantID [16]byte) error {
	if ix.client == nil {
		slog.Warn("search:index: no meilisearch client, skipping")
		return nil
	}

	tID := pgtype.UUID{Bytes: tenantID, Valid: true}

	row, err := ix.read.GetContentByID(ctx, gen.GetContentByIDParams{
		ID:       pgtype.UUID{Bytes: contentID, Valid: true},
		TenantID: tID,
	})
	if err != nil {
		return fmt.Errorf("fetching content for index: %w", err)
	}

	doc := contentDocument{
		ID:                row.ID.String(),
		TenantID:          row.TenantID.String(),
		Title:             row.Title,
		Slug:              row.Slug,
		Excerpt:           buildExcerpt(row.Body),
		ContentType:       row.ContentType,
		URL:               contentURL(row.ContentType, row.Slug),
		Status:            row.Status,
		RelevantCountries: ix.resolveRelevantCountries(ctx, tID, row.GeoSpaceID),
	}

	docs := []contentDocument{doc}
	if _, err := ix.client.Index(IndexPosts).AddDocumentsWithContext(ctx, &docs, nil); err != nil {
		return fmt.Errorf("indexing content %s: %w", doc.ID, err)
	}

	slog.Info("search:index: content indexed", "content_id", doc.ID, "title", doc.Title)

	return nil
}

// resolveRelevantCountries walks up the space hierarchy from the content's
// geo_space_id to find the country-level space's geo_code (ISO 3166-1
// alpha-2). Returns an empty slice (globally relevant) when the content has
// no geo_space_id or the country cannot be resolved. ADR-064.
func (ix *Indexer) resolveRelevantCountries(ctx context.Context, tenantID pgtype.UUID, geoSpaceID pgtype.UUID) []string {
	if !geoSpaceID.Valid {
		return []string{}
	}

	const maxDepth = 5
	currentID := geoSpaceID
	for depth := 0; depth < maxDepth; depth++ {
		space, err := ix.read.GetSpaceByID(ctx, gen.GetSpaceByIDParams{
			ID: currentID, TenantID: tenantID,
		})
		if err != nil {
			return []string{}
		}
		if space.Level == "country" && space.GeoCode != nil && *space.GeoCode != "" {
			return []string{strings.ToUpper(*space.GeoCode)}
		}
		if !space.ParentID.Valid {
			return []string{}
		}
		currentID = space.ParentID
	}
	return []string{}
}

// buildExcerpt collapses whitespace in the body and truncates to a
// rune-counted preview so multi-byte text never splits a code point. The
// ellipsis marks truncation so callers can tell a full body from a cut one.
func buildExcerpt(body string) string {
	cleaned := strings.Join(strings.Fields(body), " ")
	if utf8.RuneCountInString(cleaned) <= maxExcerptRunes {
		return cleaned
	}

	runes := []rune(cleaned)

	return string(runes[:maxExcerptRunes]) + "…"
}

// contentURL maps a content type and slug to the frontend route a result link
// should open. Unknown types fall back to the generic content path so a new
// type never produces a broken link — it just routes to the content viewer.
func contentURL(contentType, slug string) string {
	switch contentType {
	case "blog_post":
		return "/blog/" + slug
	case "discussion":
		return "/forum/t/" + slug
	case "page":
		return "/" + slug
	default:
		return "/content/" + slug
	}
}

// ---------------------------------------------------------------------------
// Forum Thread Indexer
// ---------------------------------------------------------------------------

// forumThreadDocument is the shape stored in the forum_threads index.
type forumThreadDocument struct {
	ID           string `json:"id"`
	TenantID     string `json:"tenant_id"`
	Title        string `json:"title"`
	Slug         string `json:"slug"`
	BodyPreview  string `json:"excerpt"`
	SpaceID      string `json:"space_id"`
	Solved       bool   `json:"solved"`
	AuthorUserID string `json:"author_user_id"`
	URL          string `json:"url"`
}

// IndexForumThread fetches a forum thread and upserts it into the
// forum_threads index.
func (ix *Indexer) IndexForumThread(ctx context.Context, threadID, tenantID [16]byte) error {
	if ix.client == nil {
		slog.Warn("search:index: no meilisearch client, skipping")
		return nil
	}

	row, err := ix.read.GetForumThreadByID(ctx, gen.GetForumThreadByIDParams{
		ID:       pgtype.UUID{Bytes: threadID, Valid: true},
		TenantID: pgtype.UUID{Bytes: tenantID, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("fetching forum thread for index: %w", err)
	}

	bodyText := string(row.Body)
	doc := forumThreadDocument{
		ID:           row.ID.String(),
		TenantID:     row.TenantID.String(),
		Title:        row.Title,
		Slug:         row.Slug,
		BodyPreview:  buildExcerpt(bodyText),
		SpaceID:      row.SpaceID.String(),
		Solved:       row.Solved,
		AuthorUserID: row.AuthorUserID.String(),
		URL:          "/forum/t/" + row.Slug,
	}

	docs := []forumThreadDocument{doc}
	if _, err := ix.client.Index(IndexForumThreads).AddDocumentsWithContext(ctx, &docs, nil); err != nil {
		return fmt.Errorf("indexing forum thread %s: %w", doc.ID, err)
	}

	slog.Info("search:index: forum thread indexed", "thread_id", doc.ID, "title", doc.Title)

	return nil
}

// ---------------------------------------------------------------------------
// Space Indexer
// ---------------------------------------------------------------------------

// spaceDocument is the shape stored in the spaces index.
type spaceDocument struct {
	ID          string `json:"id"`
	TenantID    string `json:"tenant_id"`
	Name        string `json:"title"`
	Slug        string `json:"slug"`
	Description string `json:"excerpt"`
	Level       string `json:"level"`
	Status      string `json:"status"`
	ParentID    string `json:"parent_id"`
	URL         string `json:"url"`
}

// IndexSpace fetches a space and upserts it into the spaces index.
func (ix *Indexer) IndexSpace(ctx context.Context, spaceID, tenantID [16]byte) error {
	if ix.client == nil {
		slog.Warn("search:index: no meilisearch client, skipping")
		return nil
	}

	row, err := ix.read.GetSpaceByID(ctx, gen.GetSpaceByIDParams{
		ID:       pgtype.UUID{Bytes: spaceID, Valid: true},
		TenantID: pgtype.UUID{Bytes: tenantID, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("fetching space for index: %w", err)
	}

	desc := ""
	if row.Description != nil {
		desc = *row.Description
	}

	doc := spaceDocument{
		ID:          row.ID.String(),
		TenantID:    row.TenantID.String(),
		Name:        row.Name,
		Slug:        row.Slug,
		Description: desc,
		Level:       row.Level,
		Status:      row.Status,
		ParentID:    row.ParentID.String(),
		URL:         "/spaces/" + row.Slug,
	}

	docs := []spaceDocument{doc}
	if _, err := ix.client.Index(IndexSpaces).AddDocumentsWithContext(ctx, &docs, nil); err != nil {
		return fmt.Errorf("indexing space %s: %w", doc.ID, err)
	}

	slog.Info("search:index: space indexed", "space_id", doc.ID, "name", doc.Name)

	return nil
}

// ---------------------------------------------------------------------------
// User Indexer
// ---------------------------------------------------------------------------

// userDocument is the shape stored in the users index.
type userDocument struct {
	ID          string `json:"id"`
	TenantID    string `json:"tenant_id"`
	Username    string `json:"title"`
	DisplayName string `json:"excerpt"`
	Bio         string `json:"bio"`
	URL         string `json:"url"`
}

// IndexUser fetches a user profile and upserts it into the users index.
// Uses GetUserProfile which JOINs user_profiles for display_name and bio.
func (ix *Indexer) IndexUser(ctx context.Context, userID, tenantID [16]byte) error {
	if ix.client == nil {
		slog.Warn("search:index: no meilisearch client, skipping")
		return nil
	}

	row, err := ix.read.GetUserProfile(ctx, gen.GetUserProfileParams{
		TenantID: pgtype.UUID{Bytes: tenantID, Valid: true},
		UserID:   pgtype.UUID{Bytes: userID, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("fetching user profile for index: %w", err)
	}

	displayName := ""
	if row.DisplayName != nil {
		displayName = *row.DisplayName
	}

	bio := ""
	if row.Bio != nil {
		bio = *row.Bio
	}

	doc := userDocument{
		ID:          row.UserID.String(),
		TenantID:    row.TenantID.String(),
		Username:    row.Username,
		DisplayName: displayName,
		Bio:         bio,
		URL:         "/users/" + row.Username,
	}

	docs := []userDocument{doc}
	if _, err := ix.client.Index(IndexUsers).AddDocumentsWithContext(ctx, &docs, nil); err != nil {
		return fmt.Errorf("indexing user %s: %w", doc.ID, err)
	}

	slog.Info("search:index: user indexed", "user_id", doc.ID, "username", doc.Username)

	return nil
}

// ---------------------------------------------------------------------------
// Tag Indexer
// ---------------------------------------------------------------------------

// tagDocument is the shape stored in the tags index.
type tagDocument struct {
	ID          string `json:"id"`
	TenantID    string `json:"tenant_id"`
	Name        string `json:"title"`
	Slug        string `json:"slug"`
	Description string `json:"excerpt"`
	URL         string `json:"url"`
}

// IndexTag fetches a forum tag and upserts it into the tags index.
func (ix *Indexer) IndexTag(ctx context.Context, tagID, tenantID [16]byte) error {
	if ix.client == nil {
		slog.Warn("search:index: no meilisearch client, skipping")
		return nil
	}

	row, err := ix.read.GetForumTagByID(ctx, gen.GetForumTagByIDParams{
		ID:       pgtype.UUID{Bytes: tagID, Valid: true},
		TenantID: pgtype.UUID{Bytes: tenantID, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("fetching forum tag for index: %w", err)
	}

	desc := ""
	if row.Description != nil {
		desc = *row.Description
	}

	doc := tagDocument{
		ID:          row.ID.String(),
		TenantID:    row.TenantID.String(),
		Name:        row.Name,
		Slug:        row.Slug,
		Description: desc,
		URL:         "/forum/tags/" + row.Slug,
	}

	docs := []tagDocument{doc}
	if _, err := ix.client.Index(IndexTags).AddDocumentsWithContext(ctx, &docs, nil); err != nil {
		return fmt.Errorf("indexing tag %s: %w", doc.ID, err)
	}

	slog.Info("search:index: tag indexed", "tag_id", doc.ID, "name", doc.Name)

	return nil
}

// ---------------------------------------------------------------------------
// DeleteDocument
// ---------------------------------------------------------------------------

// DeleteDocument removes a document from the given Meilisearch index.
// Useful when an entity is soft-deleted or unpublished.
func (ix *Indexer) DeleteDocument(ctx context.Context, indexName string, id [16]byte) error {
	if ix.client == nil {
		slog.Warn("search:delete: no meilisearch client, skipping")
		return nil
	}

	docID := pgtype.UUID{Bytes: id, Valid: true}.String()
	if _, err := ix.client.Index(indexName).DeleteDocumentWithContext(ctx, docID, nil); err != nil {
		return fmt.Errorf("deleting document %s from index %s: %w", docID, indexName, err)
	}

	slog.Info("search:delete: document removed", "index", indexName, "id", docID)

	return nil
}

// ---------------------------------------------------------------------------
// Learning tracks (Japa University marketplace — ADR-063)
// ---------------------------------------------------------------------------

// learningTrackDocument is the shape stored in the learning_tracks index.
// relevant_countries is a string array of ISO 3166-1 alpha-2 codes used for
// geo-aware filtering (ADR-064): a visitor in "NG" can filter to courses
// relevant to Nigeria.
type learningTrackDocument struct {
	ID                 string   `json:"id"`
	TenantID           string   `json:"tenant_id"`
	Title              string   `json:"title"`
	Slug               string   `json:"slug"`
	Description        string   `json:"description"`
	Difficulty         string   `json:"difficulty"`
	Category           string   `json:"category"`
	Language           string   `json:"language"`
	InstructorName     string   `json:"instructor_name"`
	PriceKobo          *int64   `json:"price_kobo"`
	Currency           string   `json:"currency"`
	IsFree             bool     `json:"is_free"`
	TotalEnrolled      int32    `json:"total_enrolled"`
	AvgRating          float64  `json:"avg_rating"`
	ReviewCount        int32    `json:"review_count"`
	RelevantCountries  []string `json:"relevant_countries"`
	URL                string   `json:"url"`
}

// IndexLearningTrack fetches a published marketplace track and upserts it
// into the learning_tracks index. Only tracks with is_published_to_university
// = true are indexed; unpublished tracks are deleted from the index.
func (ix *Indexer) IndexLearningTrack(ctx context.Context, trackID, tenantID [16]byte) error {
	if ix.client == nil {
		slog.Warn("search:index: no meilisearch client, skipping")
		return nil
	}

	tID := pgtype.UUID{Bytes: tenantID, Valid: true}
	trID := pgtype.UUID{Bytes: trackID, Valid: true}

	row, err := ix.read.GetUniversityTrackByID(ctx, gen.GetUniversityTrackByIDParams{
		TenantID: tID, ID: trID,
	})
	if err != nil {
		return fmt.Errorf("fetching learning track for index: %w", err)
	}

	// If the track is not published to the marketplace, remove it from
	// the index so it doesn't appear in search results.
	if !row.IsPublishedToUniversity {
		return ix.DeleteDocument(ctx, IndexLearningTracks, trackID)
	}

	desc := ""
	if row.Description != nil {
		desc = *row.Description
	}
	category := ""
	if row.Category != nil {
		category = *row.Category
	}

	doc := learningTrackDocument{
		ID:            row.ID.String(),
		TenantID:      row.TenantID.String(),
		Title:         row.Title,
		Slug:          row.Slug,
		Description:   desc,
		Difficulty:    row.Difficulty,
		Category:      category,
		Language:      row.Language,
		TotalEnrolled: row.TotalEnrolled,
		IsFree:        row.PriceKobo == nil || *row.PriceKobo == 0,
		PriceKobo:     row.PriceKobo,
		Currency:      row.Currency,
		URL:           fmt.Sprintf("/learn/university/catalog/%s", row.Slug),
		// relevant_countries is resolved from the track's geo_space_id by
		// walking the space hierarchy to the country level (ADR-064).
		RelevantCountries: ix.resolveRelevantCountries(ctx, tID, row.GeoSpaceID),
	}

	docs := []learningTrackDocument{doc}
	if _, err := ix.client.Index(IndexLearningTracks).AddDocumentsWithContext(ctx, &docs, nil); err != nil {
		return fmt.Errorf("upserting learning track document: %w", err)
	}

	slog.Info("search:index: learning track upserted", "track_id", row.ID.String())
	return nil
}

// ---------------------------------------------------------------------------
// Doc Page Indexer (SEARCH-001)
// ---------------------------------------------------------------------------

// docPageDocument is the shape stored in the doc_pages index.
type docPageDocument struct {
	ID           string `json:"id"`
	TenantID     string `json:"tenant_id"`
	Title        string `json:"title"`
	Slug         string `json:"slug"`
	Audience     string `json:"audience"`
	BodyPreview  string `json:"excerpt"`
	ReviewStatus string `json:"review_status"`
	URL          string `json:"url"`
}

// IndexDocPage fetches a doc page and upserts it into the doc_pages index
// (SEARCH-001). AddDocuments is an upsert in Meilisearch so this handles
// both create and update.
func (ix *Indexer) IndexDocPage(ctx context.Context, pageID, tenantID [16]byte) error {
	if ix.client == nil {
		slog.Warn("search:index: no meilisearch client, skipping")
		return nil
	}

	row, err := ix.read.GetDocPageByID(ctx, gen.GetDocPageByIDParams{
		ID:       pgtype.UUID{Bytes: pageID, Valid: true},
		TenantID: pgtype.UUID{Bytes: tenantID, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("fetching doc page for index: %w", err)
	}

	doc := docPageDocument{
		ID:           row.ID.String(),
		TenantID:     row.TenantID.String(),
		Title:        row.Title,
		Slug:         row.Slug,
		Audience:     row.Audience,
		BodyPreview:  buildExcerpt(string(row.Content)),
		ReviewStatus: row.ReviewStatus,
		URL:          fmt.Sprintf("/docs/%s/%s", row.Audience, row.Slug),
	}

	docs := []docPageDocument{doc}
	if _, err := ix.client.Index(IndexDocPages).AddDocumentsWithContext(ctx, &docs, nil); err != nil {
		return fmt.Errorf("indexing doc page %s: %w", doc.ID, err)
	}

	slog.Info("search:index: doc page indexed", "page_id", doc.ID, "title", doc.Title)
	return nil
}
