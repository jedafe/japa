// Package search — indexer.go owns the document sync path: fetching a content
// row from PostgreSQL and upserting it into the Meilisearch posts index.
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
	ID          string `json:"id"`
	TenantID    string `json:"tenant_id"`
	Title       string `json:"title"`
	Slug        string `json:"slug"`
	Excerpt     string `json:"excerpt"`
	ContentType string `json:"content_type"`
	URL         string `json:"url"`
	Status      string `json:"status"`
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

	row, err := ix.read.GetContentByID(ctx, gen.GetContentByIDParams{
		ID:       pgtype.UUID{Bytes: contentID, Valid: true},
		TenantID: pgtype.UUID{Bytes: tenantID, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("fetching content for index: %w", err)
	}

	doc := contentDocument{
		ID:          row.ID.String(),
		TenantID:    row.TenantID.String(),
		Title:       row.Title,
		Slug:        row.Slug,
		Excerpt:     buildExcerpt(row.Body),
		ContentType: row.ContentType,
		URL:         contentURL(row.ContentType, row.Slug),
		Status:      row.Status,
	}

	docs := []contentDocument{doc}
	if _, err := ix.client.Index(IndexPosts).AddDocumentsWithContext(ctx, &docs, nil); err != nil {
		return fmt.Errorf("indexing content %s: %w", doc.ID, err)
	}

	slog.Info("search:index: content indexed", "content_id", doc.ID, "title", doc.Title)
	return nil
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
