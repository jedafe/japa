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
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
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
	pool   *db.DB
}

// NewIndexer creates an Indexer backed by the given Meilisearch client and
// database read queries. Either may be nil: a nil client makes IndexContent
// a logged no-op, and a nil read set is a programming error that surfaces on
// the first call.
func NewIndexer(client meilisearch.ServiceManager, read *gen.Queries) *Indexer {
	return &Indexer{client: client, read: read}
}

// SetPool injects the database pool for background reindex operations.
// Optional — when nil, reindex-from-pool operations are skipped.
func (i *Indexer) SetPool(d *db.DB) { i.pool = d }

// HasPool returns true when a database pool has been injected.
func (i *Indexer) HasPool() bool { return i.pool != nil }

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
	SpaceID           string   `json:"space_id"`
	TagSlugs          []string `json:"tag_slugs"`
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

	// Fetch tag slugs for the content so tag-scoped search works (§54
	// GET /tags/{slug}/search). Best-effort: a query error degrades to an
	// empty tag list — the document is still indexed without tag filtering.
	tagSlugs := ix.resolveContentTagSlugs(ctx, row.ID)

	spaceID := ""
	if row.SpaceID.Valid {
		spaceID = row.SpaceID.String()
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
		SpaceID:           spaceID,
		TagSlugs:          tagSlugs,
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

// resolveContentTagSlugs returns the slugs of all tags attached to the given
// content ID. Used to populate the tag_slugs array in the posts index document
// so tag-scoped search (§54 GET /tags/{slug}/search) can filter on it.
// Best-effort: on error returns an empty slice so indexing still succeeds.
func (ix *Indexer) resolveContentTagSlugs(ctx context.Context, contentID pgtype.UUID) []string {
	if ix.read == nil {
		return []string{}
	}
	tags, err := ix.read.ListContentTags(ctx, contentID)
	if err != nil {
		slog.Warn("search:index: listing content tags failed", "content_id", contentID.String(), "error", err)
		return []string{}
	}
	slugs := make([]string, 0, len(tags))
	for _, t := range tags {
		slugs = append(slugs, t.Slug)
	}
	return slugs
}

// buildExcerpt produces a plain-text preview from a content body that may be
// Tiptap/ProseMirror JSON (object or array form), HTML, or plain text. It
// collapses whitespace and truncates to a rune-counted length so multi-byte
// text never splits a code point. The ellipsis marks truncation so callers
// can tell a full body from a cut one.
func buildExcerpt(body string) string {
	plain := tiptapPlainText(body)
	cleaned := strings.Join(strings.Fields(plain), " ")
	if utf8.RuneCountInString(cleaned) <= maxExcerptRunes {
		return cleaned
	}

	runes := []rune(cleaned)

	return string(runes[:maxExcerptRunes]) + "…"
}

// tiptapPlainText extracts readable text from a content body that may be
// Tiptap/ProseMirror JSON, HTML, or plain text. It handles the two Tiptap
// shapes stored in JapaConnect:
//   - A full document object: {"type":"doc","content":[...]}
//   - A bare array of nodes:  [{"type":"paragraph","content":[...]}]
//
// HTML bodies are tag-stripped; anything else is returned as-is so a plain
// text body still works.
func tiptapPlainText(body string) string {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return ""
	}

	switch trimmed[0] {
	case '{':
		// Tiptap document object — extract text from the "content" array.
		if text, ok := extractTiptapDoc([]byte(trimmed)); ok {
			return text
		}
	case '[':
		// Bare array of Tiptap nodes (forum_threads.body shape).
		if text, ok := extractTiptapNodes([]byte(trimmed)); ok {
			return text
		}
	case '<':
		// HTML — strip tags.
		return stripHTMLTags(trimmed)
	}

	return trimmed
}

// extractTiptapDoc parses a Tiptap document object and returns the
// concatenated text of all text nodes.
func extractTiptapDoc(data []byte) (string, bool) {
	var doc struct {
		Type    string        `json:"type"`
		Content []interface{} `json:"content"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return "", false
	}
	if doc.Type != "doc" {
		return "", false
	}
	var sb strings.Builder
	for _, node := range doc.Content {
		walkTiptapNode(node, &sb)
	}
	return strings.TrimSpace(sb.String()), true
}

// extractTiptapNodes parses a bare array of Tiptap nodes and returns the
// concatenated text of all text nodes.
func extractTiptapNodes(data []byte) (string, bool) {
	var nodes []interface{}
	if err := json.Unmarshal(data, &nodes); err != nil {
		return "", false
	}
	var sb strings.Builder
	for _, node := range nodes {
		walkTiptapNode(node, &sb)
	}
	return strings.TrimSpace(sb.String()), true
}

// walkTiptapNode recursively traverses a Tiptap node, appending text from
// "text" leaf nodes and recursing into "content" children.
func walkTiptapNode(node interface{}, sb *strings.Builder) {
	m, ok := node.(map[string]interface{})
	if !ok {
		return
	}
	if text, ok := m["text"].(string); ok {
		sb.WriteString(text)
		sb.WriteByte(' ')
	}
	if content, ok := m["content"].([]interface{}); ok {
		for _, child := range content {
			walkTiptapNode(child, sb)
		}
	}
}

// stripHTMLTags removes HTML markup from s, leaving only the visible text.
// It is a deliberately simple scanner — search excerpts do not need full
// HTML parsing, just tag removal.
func stripHTMLTags(s string) string {
	var sb strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// contentURL maps a content type and slug to the frontend route a result link
// should open. Unknown types fall back to the generic content path so a new
// type never produces a broken link — it just routes to the content viewer.
func contentURL(contentType, slug string) string {
	switch contentType {
	case "blog_post":
		return "/blog/" + slug
	case "discussion":
		return "/forum/" + slug
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
		URL:          "/forum/" + row.Slug,
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
// Respects the search_indexable privacy opt-out (§13, §19): if the user has
// opted out of discoverability, their document is deleted from the index
// instead of upserted.
func (ix *Indexer) IndexUser(ctx context.Context, userID, tenantID [16]byte) error {
	if ix.client == nil {
		slog.Warn("search:index: no meilisearch client, skipping")
		return nil
	}

	tID := pgtype.UUID{Bytes: tenantID, Valid: true}
	uID := pgtype.UUID{Bytes: userID, Valid: true}

	// §13/§19: check search_indexable opt-out before indexing. If the user
	// has opted out of discoverability, remove them from the index so they
	// don't appear in search results. Best-effort: if the privacy settings
	// row is missing, default to indexable (COALESCE true in the query).
	priv, err := ix.read.GetUserPrivacySettings(ctx, gen.GetUserPrivacySettingsParams{
		TenantID: tID,
		UserID:   uID,
	})
	if err == nil && !priv.SearchIndexable {
		slog.Info("search:index: user opted out of search_indexable, removing from index", "user_id", uID.String())
		return ix.DeleteDocument(ctx, IndexUsers, userID)
	}
	// On privacy-settings query error, proceed with indexing — the
	// COALESCE default in the DB is true, so a missing row means
	// indexable. A query error is logged but does not block indexing.

	row, err := ix.read.GetUserProfile(ctx, gen.GetUserProfileParams{
		TenantID: tID,
		UserID:   uID,
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
// Organization Indexer (Section 54)
// ---------------------------------------------------------------------------

// organizationDocument is the shape stored in the organizations index.
// The title/excerpt/url field names match the shared searchAttributesToRetrieve
// list so the multi-search result mapper works uniformly across indexes.
type organizationDocument struct {
	ID          string `json:"id"`
	TenantID    string `json:"tenant_id"`
	Name        string `json:"title"`
	Slug        string `json:"slug"`
	Description string `json:"excerpt"`
	OrgType     string `json:"org_type"`
	Verified    bool   `json:"verified"`
	Status      string `json:"status"`
	CountryCode string `json:"country_code"`
	URL         string `json:"url"`
}

// IndexOrganization fetches an organization and upserts it into the
// organizations index. Soft-deleted orgs (deleted_at IS NOT NULL) are removed
// from the index so they never surface in search.
func (ix *Indexer) IndexOrganization(ctx context.Context, orgID, tenantID [16]byte) error {
	if ix.client == nil {
		slog.Warn("search:index: no meilisearch client, skipping")
		return nil
	}

	row, err := ix.read.GetOrganizationByID(ctx, gen.GetOrganizationByIDParams{
		ID:       pgtype.UUID{Bytes: orgID, Valid: true},
		TenantID: pgtype.UUID{Bytes: tenantID, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("fetching organization for index: %w", err)
	}

	// Soft-deleted orgs are removed from the index so they don't appear in
	// search results (Rule 11 — soft-delete, and GDPR erasure safety).
	if row.DeletedAt.Valid {
		return ix.DeleteDocument(ctx, IndexOrganizations, orgID)
	}

	desc := ""
	if row.Description != nil {
		desc = *row.Description
	}
	country := ""
	if row.CountryCode != nil {
		country = *row.CountryCode
	}

	doc := organizationDocument{
		ID:          row.ID.String(),
		TenantID:    row.TenantID.String(),
		Name:        row.Name,
		Slug:        row.Slug,
		Description: desc,
		OrgType:     row.OrgType,
		Verified:    row.Verified,
		Status:      row.Status,
		CountryCode: country,
		URL:         "/org/" + row.Slug,
	}

	docs := []organizationDocument{doc}
	if _, err := ix.client.Index(IndexOrganizations).AddDocumentsWithContext(ctx, &docs, nil); err != nil {
		return fmt.Errorf("indexing organization %s: %w", doc.ID, err)
	}

	slog.Info("search:index: organization indexed", "org_id", doc.ID, "name", doc.Name)
	return nil
}

// ---------------------------------------------------------------------------
// Language Test Requirement Indexer (§99, ADR-107, ADR-088 JapaBot Indexability)
// ---------------------------------------------------------------------------

// languageTestRequirementDocument is the shape stored in the
// language_test_requirements index. Only rows with is_japabot_indexed = true
// (status=published AND is_active AND hitl_reviewed_by IS NOT NULL) are
// indexed so JapaBot RAG retrieval never surfaces unreviewed content.
type languageTestRequirementDocument struct {
	ID                 string   `json:"id"`
	TenantID           string   `json:"tenant_id"`
	TestSlug           string   `json:"test_slug"`
	TestName           string   `json:"title"`
	MinimumScore       string   `json:"excerpt"`
	DestinationSpaceID string   `json:"destination_space_id"`
	VisaCategory       string   `json:"visa_category"`
	IsJapabotIndexed   bool     `json:"is_japabot_indexed"`
	Status             string   `json:"status"`
	URL                string   `json:"url"`
}

// IndexLanguageTestRequirement fetches a language_test_requirements row
// and upserts it into the language_test_requirements index. Rows that are
// not JapaBot-indexable (is_japabot_indexed = false) are deleted from the
// index so stale drafts/rejected rows do not surface in RAG retrieval.
// Ref: japaconnect-engineering/sections/99-language-test-hub.md
//      japaconnect-engineering/adr/ADR-107-language-test-hub.md
func (ix *Indexer) IndexLanguageTestRequirement(ctx context.Context, reqID, tenantID [16]byte) error {
	if ix.client == nil {
		slog.Warn("search:index: no meilisearch client, skipping")
		return nil
	}
	if ix.pool == nil {
		return fmt.Errorf("search:index: no database pool for language_test_requirements fetch")
	}

	row, err := ix.fetchLanguageTestRequirement(ctx, reqID, tenantID)
	if err != nil {
		return fmt.Errorf("fetching language_test_requirement for index: %w", err)
	}
	if row == nil {
		// Row not found or soft-deleted — remove from index.
		return ix.DeleteDocument(ctx, IndexLanguageTestReqs, reqID)
	}
	if !row.isJapabotIndexed {
		// Not indexable (draft/rejected/inactive/unreviewed) — remove.
		return ix.DeleteDocument(ctx, IndexLanguageTestReqs, reqID)
	}

	doc := languageTestRequirementDocument{
		ID:                 row.id,
		TenantID:           row.tenantID,
		TestSlug:           row.testSlug,
		TestName:           row.testName,
		MinimumScore:       row.minimumScore,
		DestinationSpaceID: row.destinationSpaceID,
		VisaCategory:       row.visaCategory,
		IsJapabotIndexed:   true,
		Status:             row.status,
		URL:                "/language-tests/" + row.testSlug,
	}

	docs := []languageTestRequirementDocument{doc}
	if _, err := ix.client.Index(IndexLanguageTestReqs).AddDocumentsWithContext(ctx, &docs, nil); err != nil {
		return fmt.Errorf("indexing language_test_requirement %s: %w", doc.ID, err)
	}

	slog.Info("search:index: language_test_requirement indexed", "id", doc.ID, "test", doc.TestSlug)
	return nil
}

// languageTestReqRow is the scanned shape for the direct pgx query.
type languageTestReqRow struct {
	id               string
	tenantID         string
	testSlug         string
	testName         string
	minimumScore     string
	destinationSpaceID string
	visaCategory     string
	status           string
	isJapabotIndexed bool
}

// fetchLanguageTestRequirement reads a single requirement row joined with
// its language_tests parent. Uses the pool directly because the
// languagetesthub feature has no sqlc-generated queries.
func (ix *Indexer) fetchLanguageTestRequirement(ctx context.Context, reqID, tenantID [16]byte) (*languageTestReqRow, error) {
	conn, err := ix.pool.Reader.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquiring connection: %w", err)
	}
	defer conn.Release()

	var r languageTestReqRow
	var visaCategory *string
	err = conn.QueryRow(ctx, `
		SELECT r.id, r.tenant_id, lt.slug, lt.name,
		       r.minimum_score, r.destination_space_id,
		       r.visa_category, r.status, r.is_japabot_indexed
		FROM language_test_requirements r
		JOIN language_tests lt ON lt.id = r.test_id
		WHERE r.id = $1 AND r.tenant_id = $2
	`, reqID, tenantID).Scan(
		&r.id, &r.tenantID, &r.testSlug, &r.testName,
		&r.minimumScore, &r.destinationSpaceID,
		&visaCategory, &r.status, &r.isJapabotIndexed,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if visaCategory != nil {
		r.visaCategory = *visaCategory
	}
	return &r, nil
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
	ID                string   `json:"id"`
	TenantID          string   `json:"tenant_id"`
	Title             string   `json:"title"`
	Slug              string   `json:"slug"`
	Description       string   `json:"description"`
	Difficulty        string   `json:"difficulty"`
	Category          string   `json:"category"`
	Language          string   `json:"language"`
	InstructorName    string   `json:"instructor_name"`
	PriceKobo         *int64   `json:"price_kobo"`
	Currency          string   `json:"currency"`
	IsFree            bool     `json:"is_free"`
	TotalEnrolled     int32    `json:"total_enrolled"`
	AvgRating         float64  `json:"avg_rating"`
	ReviewCount       int32    `json:"review_count"`
	RelevantCountries []string `json:"relevant_countries"`
	URL               string   `json:"url"`
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

// ---------------------------------------------------------------------------
// Bulk Reindex
// ---------------------------------------------------------------------------

// reindexBatchSize is the page size used when iterating tenant-wide entity
// lists during a bulk reindex. Chosen large enough that a typical dev/staging
// tenant completes in a handful of round-trips, small enough to keep each
// query result set bounded.
const reindexBatchSize = 500

// ReindexSummary records the outcome of a bulk reindex run per entity type.
type ReindexSummary struct {
	Content       int `json:"content"`
	ForumThread   int `json:"forum_thread"`
	Space         int `json:"space"`
	User          int `json:"user"`
	Tag           int `json:"tag"`
	LearningTrack int `json:"learning_track"`
	DocPage       int `json:"doc_page"`
	Organization  int `json:"organization"`
	Errors        int `json:"errors"`
}

// ReindexAll iterates every entity for the given tenant and upserts it into
// the corresponding Meilisearch index. This is the bootstrap/backfill path
// for indexes that were provisioned after data already existed — the
// per-entity IndexXxx methods only fire on create/update events, so rows
// that predate the indexer wiring are never indexed without this sweep.
//
// Each entity type is fetched in offset/cursor batches and delegated to the
// existing per-entity IndexXxx method, which reuses the document-shaping
// logic (excerpt, URL, relevant_countries, etc.).
func (ix *Indexer) ReindexAll(ctx context.Context, tenantID pgtype.UUID) (*ReindexSummary, error) {
	if ix.client == nil {
		return nil, fmt.Errorf("search:reindex: no meilisearch client")
	}
	if ix.read == nil {
		return nil, fmt.Errorf("search:reindex: no database queries")
	}

	summary := &ReindexSummary{}

	// Contents — cursor-paginated by created_at.
	contentCursor := pgtype.Timestamptz{}
	for {
		rows, err := ix.read.ListContentsByTenant(ctx, gen.ListContentsByTenantParams{
			TenantID:        tenantID,
			Q:               "",
			CursorCreatedAt: contentCursor,
			PageLimit:       reindexBatchSize,
		})
		if err != nil {
			slog.Warn("search:reindex: listing contents failed", "error", err)
			summary.Errors++
			break
		}
		for _, row := range rows {
			if err := ix.IndexContent(ctx, row.ID.Bytes, tenantID.Bytes); err != nil {
				slog.Warn("search:reindex: content failed", "id", row.ID, "error", err)
				summary.Errors++
			} else {
				summary.Content++
			}
		}
		if len(rows) < reindexBatchSize {
			break
		}
		// Advance cursor — the last row's created_at (if present) drives the
		// next page. ListContentsByTenant orders by created_at DESC.
		last := rows[len(rows)-1]
		contentCursor = last.CreatedAt
	}

	// Forum threads — cursor-paginated by created_at.
	threadCursor := pgtype.Timestamptz{}
	for {
		rows, err := ix.read.ListForumThreadsByTenant(ctx, gen.ListForumThreadsByTenantParams{
			TenantID:        tenantID,
			CursorCreatedAt: threadCursor,
			PageLimit:       reindexBatchSize,
		})
		if err != nil {
			slog.Warn("search:reindex: listing forum threads failed", "error", err)
			summary.Errors++
			break
		}
		for _, row := range rows {
			if err := ix.IndexForumThread(ctx, row.ID.Bytes, tenantID.Bytes); err != nil {
				slog.Warn("search:reindex: forum thread failed", "id", row.ID, "error", err)
				summary.Errors++
			} else {
				summary.ForumThread++
			}
		}
		if len(rows) < reindexBatchSize {
			break
		}
		last := rows[len(rows)-1]
		threadCursor = last.CreatedAt
	}

	// Forum tags — offset-paginated.
	for offset := int32(0); ; offset += reindexBatchSize {
		rows, err := ix.read.ListForumTags(ctx, gen.ListForumTagsParams{
			TenantID:   tenantID,
			PageOffset: offset,
			PageLimit:  reindexBatchSize,
		})
		if err != nil {
			slog.Warn("search:reindex: listing forum tags failed", "error", err)
			summary.Errors++
			break
		}
		for _, row := range rows {
			if err := ix.IndexTag(ctx, row.ID.Bytes, tenantID.Bytes); err != nil {
				slog.Warn("search:reindex: tag failed", "id", row.ID, "error", err)
				summary.Errors++
			} else {
				summary.Tag++
			}
		}
		if len(rows) < reindexBatchSize {
			break
		}
	}

	// Spaces — offset-paginated via the admin list query.
	for offset := int32(0); ; offset += reindexBatchSize {
		rows, err := ix.read.AdminListSpaces(ctx, gen.AdminListSpacesParams{
			TenantID:   tenantID,
			Search:     "",
			Status:     "",
			Visibility: "",
			OffsetVal:  offset,
			LimitVal:   reindexBatchSize,
		})
		if err != nil {
			slog.Warn("search:reindex: listing spaces failed", "error", err)
			summary.Errors++
			break
		}
		for _, row := range rows {
			if err := ix.IndexSpace(ctx, row.ID.Bytes, tenantID.Bytes); err != nil {
				slog.Warn("search:reindex: space failed", "id", row.ID, "error", err)
				summary.Errors++
			} else {
				summary.Space++
			}
		}
		if len(rows) < reindexBatchSize {
			break
		}
	}

	// Users — offset-paginated with empty filters (all users).
	for offset := int32(0); ; offset += reindexBatchSize {
		rows, err := ix.read.ListUsers(ctx, gen.ListUsersParams{
			TenantID:   tenantID,
			Search:     "",
			Status:     "",
			GroupSlug:  "",
			PageOffset: offset,
			PageLimit:  reindexBatchSize,
		})
		if err != nil {
			slog.Warn("search:reindex: listing users failed", "error", err)
			summary.Errors++
			break
		}
		for _, row := range rows {
			if err := ix.IndexUser(ctx, row.ID.Bytes, tenantID.Bytes); err != nil {
				slog.Warn("search:reindex: user failed", "id", row.ID, "error", err)
				summary.Errors++
			} else {
				summary.User++
			}
		}
		if len(rows) < reindexBatchSize {
			break
		}
	}

	// Learning tracks — offset-paginated via ListUniversityCatalog with
	// empty filters (all published tracks). IndexLearningTrack re-fetches
	// the full row via GetUniversityTrackByID.
	for offset := int32(0); ; offset += reindexBatchSize {
		rows, err := ix.read.ListUniversityCatalog(ctx, gen.ListUniversityCatalogParams{
			TenantID:   tenantID,
			Category:   "",
			Language:   "",
			IsFree:     false,
			PageOffset: offset,
			PageLimit:  reindexBatchSize,
		})
		if err != nil {
			slog.Warn("search:reindex: listing learning tracks failed", "error", err)
			summary.Errors++
			break
		}
		for _, row := range rows {
			if err := ix.IndexLearningTrack(ctx, row.ID.Bytes, tenantID.Bytes); err != nil {
				slog.Warn("search:reindex: learning track failed", "id", row.ID, "error", err)
				summary.Errors++
			} else {
				summary.LearningTrack++
			}
		}
		if len(rows) < reindexBatchSize {
			break
		}
	}

	// Doc pages — single query returns all for the tenant.
	docRows, err := ix.read.ListAllDocPages(ctx, tenantID)
	if err != nil {
		slog.Warn("search:reindex: listing doc pages failed", "error", err)
		summary.Errors++
	} else {
		for _, row := range docRows {
			if err := ix.IndexDocPage(ctx, row.ID.Bytes, tenantID.Bytes); err != nil {
				slog.Warn("search:reindex: doc page failed", "id", row.ID, "error", err)
				summary.Errors++
			} else {
				summary.DocPage++
			}
		}
	}

	// Organizations — offset-paginated via ListOrganizations with empty filters.
	for offset := int32(0); ; offset += reindexBatchSize {
		rows, err := ix.read.ListOrganizations(ctx, gen.ListOrganizationsParams{
			TenantID: tenantID,
			Column2:  "", // org_type filter (empty = all)
			Column3:  "", // country_code filter (empty = all)
			Column4:  "", // name search (empty = all)
			Column5:  "", // verified filter (empty = all)
			Limit:    reindexBatchSize,
			Offset:   offset,
		})
		if err != nil {
			slog.Warn("search:reindex: listing organizations failed", "error", err)
			summary.Errors++
			break
		}
		for _, row := range rows {
			if err := ix.IndexOrganization(ctx, row.ID.Bytes, tenantID.Bytes); err != nil {
				slog.Warn("search:reindex: organization failed", "id", row.ID, "error", err)
				summary.Errors++
			} else {
				summary.Organization++
			}
		}
		if len(rows) < reindexBatchSize {
			break
		}
	}

	return summary, nil
}
