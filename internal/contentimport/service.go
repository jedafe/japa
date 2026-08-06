// Package contentimport — service.go orchestrates the content import
// pipeline: StartImport creates a tracking record and enqueues the
// import:process_source task (fire-and-forget per Rule 8). The task handler
// parses RSS feeds or ZIP exports, converts posts to TipTap JSON (Section 15),
// creates draft contents rows with canonical_url set to the original URL
// (Section 26), and enqueues import:fetch_media for each imported item.
//
// Ref: japaconnect-engineering/sections/65-content-import.md
//      japaconnect-engineering/adr/ADR-083-content-import.md
package contentimport

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db/gen"
	"golang.org/x/net/html"
)

// Source type constants — the set of supported import sources (Section 65).
const (
	SourceTypeMedium   = "medium"
	SourceTypeSubstack = "substack"
	SourceTypeRSS      = "rss"
	SourceTypeZIP      = "zip"
)

// Status constants — the lifecycle states of a content import.
const (
	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
	StatusPartial    = "partial"
)

// Task type constants — Asynq task names (Section 11 catalog).
const (
	TaskProcessSource = "import:process_source"
	TaskFetchMedia    = "import:fetch_media"
)

// DefaultMaxItemsPerRun is the fallback for import.max_items_per_run when
// the system_settings key is unset or no settings reader is wired.
const DefaultMaxItemsPerRun = 50

// RSSItem represents a single item parsed from an RSS feed.
type RSSItem struct {
	Title       string
	Link        string
	Description string
	Content     string
	PublishedAt time.Time
}

// ImportError records a single item that failed to import.
type ImportError struct {
	ItemTitle string `json:"item_title"`
	URL      string `json:"url"`
	Error    string `json:"error"`
}

// TaskEnqueuer is the minimal interface for enqueueing Asynq tasks.
// The concrete implementation is the Asynq client adapter.
type TaskEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// ContentCreator is the interface for creating draft contents rows from
// imported posts. The content.Service satisfies this via its CreateContent
// method.
type ContentCreator interface {
	CreateContent(ctx context.Context, tenantID pgtype.UUID, input CreateContentInput) (ContentRow, error)
}

// CreateContentInput holds the parameters for creating a draft content row
// from an imported post. This mirrors content.CreateContentInput but is
// defined here to avoid a circular import and to keep the import package
// testable in isolation.
type CreateContentInput struct {
	AuthorID     pgtype.UUID
	ContentType  string
	Title        string
	Body         string
	CanonicalURL *string
	Status       string
	Visibility   string
}

// ContentRow is the minimal subset of the created content row needed by
// the import pipeline (the content ID for media fetch enqueueing).
type ContentRow struct {
	ID pgtype.UUID
}

// MediaFetcher downloads external images referenced in an imported content's
// TipTap body, stores them via the media pipeline (Section 34), and replaces
// the external URLs with stored media URLs. The concrete implementation
// adapts media.Service + content.Service and is wired in server.go.
type MediaFetcher interface {
	FetchAndReplaceMedia(ctx context.Context, tenantID, userID, contentID pgtype.UUID) error
}

// Service holds the business logic for the content import pipeline.
type Service struct {
	store        *Store
	settings     *config.Settings
	enqueuer     TaskEnqueuer
	creator      ContentCreator
	mediaFetcher MediaFetcher
	httpClient   *http.Client
}

// NewService creates a content-import service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{
		store:    store,
		settings: settings,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SetEnqueuer injects the Asynq task enqueuer for import:process_source
// and import:fetch_media tasks.
func (s *Service) SetEnqueuer(e TaskEnqueuer) { s.enqueuer = e }

// SetContentCreator injects the content creation adapter for creating
// draft contents rows from imported posts.
func (s *Service) SetContentCreator(c ContentCreator) { s.creator = c }

// SetMediaFetcher injects the media fetcher that downloads external images
// from imported content and replaces their URLs with stored media URLs.
func (s *Service) SetMediaFetcher(mf MediaFetcher) { s.mediaFetcher = mf }

// SetHTTPClient injects the HTTP client used for fetching RSS feeds.
// Primarily for testing; the default client has a 30-second timeout.
func (s *Service) SetHTTPClient(c *http.Client) { s.httpClient = c }

// maxItemsPerRun reads import.max_items_per_run from system_settings,
// falling back to DefaultMaxItemsPerRun when unset or no settings reader
// is wired.
func (s *Service) maxItemsPerRun(ctx context.Context, tenantID pgtype.UUID) int {
	if s.settings == nil {
		return DefaultMaxItemsPerRun
	}
	return s.settings.GetIntOr(ctx, tenantID, "import.max_items_per_run", DefaultMaxItemsPerRun)
}

// StartImport creates a content_imports record with status='pending' and
// enqueues the import:process_source task fire-and-forget (Rule 8). The
// handler returns the import record immediately — the task runs
// asynchronously.
func (s *Service) StartImport(ctx context.Context, tenantID, userID pgtype.UUID, sourceType, sourceURL string) (gen.ContentImport, error) {
	if !isValidSourceType(sourceType) {
		return gen.ContentImport{}, fmt.Errorf("contentimport: invalid source type %q", sourceType)
	}

	var urlPtr *string
	if sourceURL != "" {
		urlPtr = &sourceURL
	}

	imp, err := s.store.Create(ctx, tenantID, userID, sourceType, urlPtr)
	if err != nil {
		return gen.ContentImport{}, fmt.Errorf("contentimport: start: create record: %w", err)
	}

	// Enqueue import:process_source fire-and-forget (Rule 8).
	if s.enqueuer != nil {
		payload, err := json.Marshal(map[string]string{
			"import_id":   imp.ID.String(),
			"tenant_id":   tenantID.String(),
			"user_id":     userID.String(),
			"source_type": sourceType,
			"source_url":  sourceURL,
		})
		if err != nil {
			slog.Error("contentimport: start: marshal payload", "error", err)
		} else if err := s.enqueuer.Enqueue(TaskProcessSource, payload); err != nil {
			slog.Error("contentimport: start: enqueue process_source",
				"import_id", imp.ID.String(), "error", err)
		}
	}

	return imp, nil
}

// GetImport returns a single import with its details, scoped to the tenant.
func (s *Service) GetImport(ctx context.Context, tenantID, importID pgtype.UUID) (gen.ContentImport, error) {
	return s.store.GetByID(ctx, tenantID, importID)
}

// ListUserImports returns a user's import history with pagination.
// Page is 1-based; limit is clamped to a reasonable maximum.
func (s *Service) ListUserImports(ctx context.Context, tenantID, userID pgtype.UUID, page, limit int) ([]gen.ContentImport, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset := int32((page - 1) * limit)

	rows, err := s.store.ListUserImports(ctx, tenantID, userID, offset, int32(limit))
	if err != nil {
		return nil, 0, err
	}

	total, err := s.store.CountUserImports(ctx, tenantID, userID)
	if err != nil {
		return nil, 0, err
	}

	return rows, total, nil
}

// AdminListImports returns all imports for a tenant, optionally filtered
// by status, with pagination. An empty status returns all statuses.
func (s *Service) AdminListImports(ctx context.Context, tenantID pgtype.UUID, status string, page, limit int) ([]gen.AdminListContentImportsRow, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset := int32((page - 1) * limit)

	rows, err := s.store.AdminListImports(ctx, tenantID, status, offset, int32(limit))
	if err != nil {
		return nil, 0, err
	}

	total, err := s.store.AdminCountImports(ctx, tenantID, status)
	if err != nil {
		return nil, 0, err
	}

	return rows, total, nil
}

// RetryImport resets a failed or partial import to pending status and
// re-enqueues the import:process_source task. Only imports with status
// 'failed' or 'partial' can be retried — the SQL query enforces this.
func (s *Service) RetryImport(ctx context.Context, tenantID, importID pgtype.UUID) (gen.ContentImport, error) {
	imp, err := s.store.ResetForRetry(ctx, tenantID, importID)
	if err != nil {
		return gen.ContentImport{}, fmt.Errorf("contentimport: retry: reset: %w", err)
	}

	// Re-enqueue import:process_source fire-and-forget (Rule 8).
	if s.enqueuer != nil {
		sourceURL := ""
		if imp.SourceUrl != nil {
			sourceURL = *imp.SourceUrl
		}
		payload, err := json.Marshal(map[string]string{
			"import_id":   imp.ID.String(),
			"tenant_id":   tenantID.String(),
			"user_id":     imp.UserID.String(),
			"source_type": imp.SourceType,
			"source_url":  sourceURL,
		})
		if err != nil {
			slog.Error("contentimport: retry: marshal payload", "error", err)
		} else if err := s.enqueuer.Enqueue(TaskProcessSource, payload); err != nil {
			slog.Error("contentimport: retry: enqueue process_source",
				"import_id", imp.ID.String(), "error", err)
		}
	}

	return imp, nil
}

// UpdateImportStatus updates the status, counts, error log, and completed_at
// timestamp for an import. Called by the import:process_source task handler
// after processing completes (or fails).
func (s *Service) UpdateImportStatus(ctx context.Context, tenantID, importID pgtype.UUID, status string, totalItems, importedCount, failedCount *int32, errorLog []byte, completedAt pgtype.Timestamptz) (gen.ContentImport, error) {
	return s.store.UpdateStatus(ctx, tenantID, importID, status, totalItems, importedCount, failedCount, errorLog, completedAt)
}

// ProcessSource is the import:process_source task handler. It parses the
// source (RSS feed or ZIP export), converts posts to draft contents rows
// with content_type='blog_post', status='draft', and canonical_url set to
// the original URL, enqueues import:fetch_media for each imported item,
// and updates the content_imports record with counts.
func (s *Service) ProcessSource(ctx context.Context, tenantID, importID pgtype.UUID, sourceType, sourceURL string) error {
	// Mark as processing.
	if _, err := s.store.UpdateStatus(ctx, tenantID, importID, StatusProcessing, nil, nil, nil, []byte("[]"), pgtype.Timestamptz{}); err != nil {
		return fmt.Errorf("contentimport: process: mark processing: %w", err)
	}

	// Resolve the import's user_id so created content is attributed correctly.
	imp, err := s.store.GetByID(ctx, tenantID, importID)
	if err != nil {
		return fmt.Errorf("contentimport: process: lookup import: %w", err)
	}

	maxItems := s.maxItemsPerRun(ctx, tenantID)

	var items []RSSItem
	var parseErr error

	switch sourceType {
	case SourceTypeMedium, SourceTypeSubstack, SourceTypeRSS:
		items, parseErr = s.ParseRSSFeed(sourceURL)
	case SourceTypeZIP:
		// ZIP export processing is deferred to v2.1 per ADR-083.
		parseErr = fmt.Errorf("zip import not yet implemented")
	default:
		parseErr = fmt.Errorf("unsupported source type %q", sourceType)
	}

	if parseErr != nil {
		errLog := marshalErrorLog([]ImportError{{
			ItemTitle: "",
			URL:       sourceURL,
			Error:     parseErr.Error(),
		}})
		now := nowTimestamptz()
		_, _ = s.store.UpdateStatus(ctx, tenantID, importID, StatusFailed, nil, nil, nil, errLog, now)
		return fmt.Errorf("contentimport: process: parse source: %w", parseErr)
	}

	// Clamp to max_items_per_run.
	if len(items) > maxItems {
		items = items[:maxItems]
	}

	totalItems := int32(len(items))
	var importedCount, failedCount int32
	var errors []ImportError

	for _, item := range items {
		if err := s.importSingleItem(ctx, tenantID, importID, imp.UserID, item); err != nil {
			failedCount++
			errors = append(errors, ImportError{
				ItemTitle: item.Title,
				URL:       item.Link,
				Error:     err.Error(),
			})
			slog.Warn("contentimport: process: item failed",
				"import_id", importID.String(), "title", item.Title, "error", err)
			continue
		}
		importedCount++
	}

	// Determine final status.
	finalStatus := StatusCompleted
	if failedCount > 0 && importedCount > 0 {
		finalStatus = StatusPartial
	} else if failedCount > 0 && importedCount == 0 {
		finalStatus = StatusFailed
	}

	now := nowTimestamptz()
	_, err = s.store.UpdateStatus(ctx, tenantID, importID, finalStatus,
		&totalItems, &importedCount, &failedCount,
		marshalErrorLog(errors), now)
	if err != nil {
		return fmt.Errorf("contentimport: process: update final status: %w", err)
	}

	slog.Info("contentimport: process: complete",
		"import_id", importID.String(),
		"total", totalItems, "imported", importedCount, "failed", failedCount,
		"status", finalStatus)

	return nil
}

// FetchMedia is the import:fetch_media task handler. It downloads external
// images referenced in an imported content's TipTap body, stores them via
// the media pipeline (Section 34), and replaces the external URLs with
// stored media URLs. The heavy lifting is delegated to the MediaFetcher
// adapter (wired in server.go) to avoid a circular import on media/content.
//
// If no MediaFetcher is wired, the method logs a warning and returns nil —
// the imported content is still usable, just with external image URLs.
func (s *Service) FetchMedia(ctx context.Context, tenantID, userID, contentID pgtype.UUID) error {
	if s.mediaFetcher == nil {
		slog.Warn("contentimport: fetch_media: no media fetcher configured, skipping",
			"content_id", contentID.String())
		return nil
	}
	if err := s.mediaFetcher.FetchAndReplaceMedia(ctx, tenantID, userID, contentID); err != nil {
		return fmt.Errorf("contentimport: fetch_media: %w", err)
	}
	slog.Info("contentimport: fetch_media: complete", "content_id", contentID.String())
	return nil
}

// importSingleItem converts a single RSS item to a draft contents row and
// enqueues import:fetch_media for the imported content.
func (s *Service) importSingleItem(ctx context.Context, tenantID, importID, userID pgtype.UUID, item RSSItem) error {
	if s.creator == nil {
		return fmt.Errorf("no content creator configured")
	}

	// Convert HTML content to TipTap JSON.
	body, err := ConvertHTMLToTipTap(item.Content)
	if err != nil {
		return fmt.Errorf("convert html to tiptap: %w", err)
	}

	// Fallback to description if content is empty.
	bodyStr := string(body)
	if strings.TrimSpace(bodyStr) == "" || bodyStr == "null" {
		body, err = ConvertHTMLToTipTap(item.Description)
		if err != nil {
			return fmt.Errorf("convert description to tiptap: %w", err)
		}
		bodyStr = string(body)
	}

	canonicalURL := item.Link
	if canonicalURL == "" {
		return fmt.Errorf("item has no canonical URL")
	}

	row, err := s.creator.CreateContent(ctx, tenantID, CreateContentInput{
		AuthorID:     userID,
		ContentType:  "blog_post",
		Title:        item.Title,
		Body:         bodyStr,
		CanonicalURL: &canonicalURL,
		Status:       "draft",
		Visibility:   "public",
	})
	if err != nil {
		return fmt.Errorf("create content: %w", err)
	}

	// Enqueue import:fetch_media for the imported content (low queue).
	if s.enqueuer != nil {
		payload, err := json.Marshal(map[string]string{
			"content_id": row.ID.String(),
			"tenant_id":  tenantID.String(),
			"user_id":    userID.String(),
			"import_id":  importID.String(),
		})
		if err != nil {
			slog.Warn("contentimport: fetch_media: marshal payload", "error", err)
		} else if err := s.enqueuer.Enqueue(TaskFetchMedia, payload); err != nil {
			slog.Warn("contentimport: fetch_media: enqueue failed",
				"content_id", row.ID.String(), "error", err)
		}
	}

	return nil
}

// ParseRSSFeed fetches and parses an RSS feed, returning items with title,
// link, content, and published date. Works with Medium, Substack, and
// generic RSS feeds — all use the standard RSS 2.0 format.
func (s *Service) ParseRSSFeed(url string) ([]RSSItem, error) {
	if url == "" {
		return nil, fmt.Errorf("empty feed URL")
	}

	client := s.httpClient
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch feed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("feed returned status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read feed body: %w", err)
	}

	return parseRSSXML(data)
}

// parseRSSXML parses raw RSS XML bytes into RSSItem structs.
// Uses encoding/xml from the standard library.
func parseRSSXML(data []byte) ([]RSSItem, error) {
	type rssChannel struct {
		Title string    `xml:"title"`
		Items []rssXMLItem `xml:"item"`
	}
	type rssDoc struct {
		XMLName xml.Name   `xml:"rss"`
		Channel rssChannel `xml:"channel"`
	}

	// Also handle Atom feeds by trying RSS first, then Atom.
	var doc rssDoc
	if err := xml.Unmarshal(data, &doc); err != nil {
		// Try Atom format.
		return parseAtomXML(data)
	}

	items := make([]RSSItem, 0, len(doc.Channel.Items))
	for _, xitem := range doc.Channel.Items {
		items = append(items, RSSItem{
			Title:       xitem.Title,
			Link:        xitem.Link,
			Description: xitem.Description,
			Content:     xitem.Content,
			PublishedAt: parseRSSDate(xitem.PubDate),
		})
	}

	if len(items) == 0 {
		// Maybe it's an Atom feed.
		return parseAtomXML(data)
	}

	return items, nil
}

// rssXMLItem maps an RSS 2.0 <item> element.
type rssXMLItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	Content     string `xml:"content:encoded"`
	PubDate     string `xml:"pubDate"`
}

// parseAtomXML parses an Atom feed as a fallback.
func parseAtomXML(data []byte) ([]RSSItem, error) {
	type atomEntry struct {
		Title   string `xml:"title"`
		Link    string `xml:"link"`
		Summary string `xml:"summary"`
		Content string `xml:"content"`
		Published string `xml:"published"`
	}
	type atomFeed struct {
		XMLName xml.Name    `xml:"feed"`
		Entries []atomEntry `xml:"entry"`
	}

	var feed atomFeed
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("parse feed (tried RSS and Atom): %w", err)
	}

	items := make([]RSSItem, 0, len(feed.Entries))
	for _, entry := range feed.Entries {
		content := entry.Content
		if content == "" {
			content = entry.Summary
		}
		items = append(items, RSSItem{
			Title:       entry.Title,
			Link:        entry.Link,
			Description: entry.Summary,
			Content:     content,
			PublishedAt: parseRSSDate(entry.Published),
		})
	}

	return items, nil
}

// parseRSSDate parses common RSS date formats, returning the zero time on
// failure (the import proceeds without a published date).
func parseRSSDate(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	// RFC 1123Z — the standard RSS pubDate format.
	layouts := []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC3339,
		"Mon, 2 Jan 2006 15:04:05 MST",
		"Mon, 2 Jan 2006 15:04:05 -0700",
		"2006-01-02T15:04:05Z",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// ConvertHTMLToTipTap converts HTML content to TipTap JSON (Section 15).
// This is a basic conversion that handles paragraphs, headings (h1-h6),
// lists (ul/ol with li), images, and blockquotes. Inline formatting is
// preserved as text within paragraphs.
func ConvertHTMLToTipTap(htmlStr string) (json.RawMessage, error) {
	if strings.TrimSpace(htmlStr) == "" {
		return json.RawMessage(`{"type":"doc","content":[]}`), nil
	}

	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}

	var content []map[string]interface{}
	walkHTML(doc, &content)

	if content == nil {
		content = []map[string]interface{}{}
	}

	result := map[string]interface{}{
		"type":    "doc",
		"content": content,
	}

	return json.Marshal(result)
}

// walkHTML traverses the parsed HTML tree and appends TipTap block nodes
// to content. It processes the body element's children, converting block-
// level elements to TipTap node types.
func walkHTML(n *html.Node, content *[]map[string]interface{}) {
	if n == nil {
		return
	}

	// Find the body element.
	if n.Type == html.ElementNode && n.Data == "body" {
		// collectBlocks iterates siblings starting from the first child,
		// so we only call it once on the first child (not in a loop).
		if n.FirstChild != nil {
			collectBlocks(n.FirstChild, content)
		}
		return
	}

	// Recurse to find the body.
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		walkHTML(child, content)
	}
}

// collectBlocks processes a node and its siblings, appending TipTap block
// nodes to content.
func collectBlocks(n *html.Node, content *[]map[string]interface{}) {
	for c := n; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode {
			switch c.Data {
			case "h1", "h2", "h3", "h4", "h5", "h6":
				level := headingLevel(c.Data)
				*content = append(*content, map[string]interface{}{
					"type": "heading",
					"attrs": map[string]interface{}{
						"level": level,
					},
					"content": collectInline(c),
				})
			case "p":
				inline := collectInline(c)
				if len(inline) > 0 {
					*content = append(*content, map[string]interface{}{
						"type":    "paragraph",
						"content": inline,
					})
				}
			case "ul":
				*content = append(*content, map[string]interface{}{
					"type":    "bulletList",
					"content": collectListItems(c),
				})
			case "ol":
				*content = append(*content, map[string]interface{}{
					"type":    "orderedList",
					"content": collectListItems(c),
				})
			case "blockquote":
				*content = append(*content, map[string]interface{}{
					"type":    "blockquote",
					"content": collectBlockContent(c),
				})
			case "img":
				src := getAttr(c, "src")
				alt := getAttr(c, "alt")
				if src != "" {
					*content = append(*content, map[string]interface{}{
						"type": "image",
						"attrs": map[string]interface{}{
							"src": src,
							"alt": alt,
						},
					})
				}
			case "br":
				*content = append(*content, map[string]interface{}{
					"type": "paragraph",
					"content": []map[string]interface{}{
						{"type": "hardBreak"},
					},
				})
			case "pre":
				text := textContent(c)
				if text != "" {
					*content = append(*content, map[string]interface{}{
						"type": "codeBlock",
						"content": []map[string]interface{}{
							{"type": "text", "text": text},
						},
					})
				}
			default:
				// Unknown block element — recurse into children.
				for child := c.FirstChild; child != nil; child = child.NextSibling {
					collectBlocks(child, content)
				}
			}
		} else if c.Type == html.TextNode {
			text := strings.TrimSpace(c.Data)
			if text != "" {
				// Bare text outside a block element — wrap in a paragraph.
				*content = append(*content, map[string]interface{}{
					"type": "paragraph",
					"content": []map[string]interface{}{
						{"type": "text", "text": text},
					},
				})
			}
		}
	}
}

// collectBlockContent collects child blocks for container elements like
// blockquote.
func collectBlockContent(n *html.Node) []map[string]interface{} {
	var content []map[string]interface{}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		collectBlocks(child, &content)
	}
	if content == nil {
		content = []map[string]interface{}{}
	}
	return content
}

// collectListItems converts <li> children of a <ul>/<ol> into TipTap
// listItem nodes.
func collectListItems(n *html.Node) []map[string]interface{} {
	var items []map[string]interface{}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && child.Data == "li" {
			items = append(items, map[string]interface{}{
				"type":    "listItem",
				"content": collectParagraphs(child),
			})
		}
	}
	if items == nil {
		items = []map[string]interface{}{}
	}
	return items
}

// collectParagraphs wraps inline content in <li> into paragraph nodes.
func collectParagraphs(n *html.Node) []map[string]interface{} {
	inline := collectInline(n)
	if len(inline) == 0 {
		return []map[string]interface{}{}
	}
	return []map[string]interface{}{
		{
			"type":    "paragraph",
			"content": inline,
		},
	}
}

// collectInline gathers inline content (text, bold, italic, links) from a
// node's children, returning TipTap inline node structs.
func collectInline(n *html.Node) []map[string]interface{} {
	var nodes []map[string]interface{}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		collectInlineNode(child, &nodes, nil)
	}
	return nodes
}

// collectInlineNode processes a single inline node, appending TipTap inline
// nodes. The marks parameter carries inherited marks (bold, italic) for
// nested formatting.
func collectInlineNode(n *html.Node, nodes *[]map[string]interface{}, marks []map[string]interface{}) {
	switch n.Type {
	case html.TextNode:
		text := n.Data
		if strings.TrimSpace(text) != "" {
			node := map[string]interface{}{
				"type": "text",
				"text": text,
			}
			if len(marks) > 0 {
				node["marks"] = marks
			}
			*nodes = append(*nodes, node)
		}
	case html.ElementNode:
		switch n.Data {
		case "strong", "b":
			newMarks := appendMarks(marks, map[string]interface{}{"type": "bold"})
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				collectInlineNode(child, nodes, newMarks)
			}
		case "em", "i":
			newMarks := appendMarks(marks, map[string]interface{}{"type": "italic"})
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				collectInlineNode(child, nodes, newMarks)
			}
		case "a":
			href := getAttr(n, "href")
			newMarks := appendMarks(marks, map[string]interface{}{
				"type": "link",
				"attrs": map[string]interface{}{
					"href": href,
				},
			})
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				collectInlineNode(child, nodes, newMarks)
			}
		case "code":
			node := map[string]interface{}{
				"type": "text",
				"text": textContent(n),
			}
			newMarks := appendMarks(marks, map[string]interface{}{"type": "code"})
			node["marks"] = newMarks
			*nodes = append(*nodes, node)
		case "br":
			*nodes = append(*nodes, map[string]interface{}{"type": "hardBreak"})
		case "img":
			src := getAttr(n, "src")
			alt := getAttr(n, "alt")
			if src != "" {
				*nodes = append(*nodes, map[string]interface{}{
					"type": "image",
					"attrs": map[string]interface{}{
						"src": src,
						"alt": alt,
					},
				})
			}
		default:
			// Unknown inline element — recurse into children.
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				collectInlineNode(child, nodes, marks)
			}
		}
	}
}

// appendMarks appends a new mark to the existing marks slice, returning a
// new slice (does not mutate the original).
func appendMarks(marks []map[string]interface{}, mark map[string]interface{}) []map[string]interface{} {
	result := make([]map[string]interface{}, len(marks), len(marks)+1)
	copy(result, marks)
	result = append(result, mark)
	return result
}

// headingLevel extracts the numeric level from an h1-h6 tag name.
func headingLevel(tag string) int {
	if len(tag) < 2 || tag[0] != 'h' {
		return 1
	}
	switch tag[1] {
	case '1':
		return 1
	case '2':
		return 2
	case '3':
		return 3
	case '4':
		return 4
	case '5':
		return 5
	case '6':
		return 6
	default:
		return 1
	}
}

// getAttr returns the value of an attribute on an element node, or empty
// string if not present.
func getAttr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

// textContent returns the concatenated text content of a node and its
// descendants.
func textContent(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var sb strings.Builder
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		sb.WriteString(textContent(child))
	}
	return sb.String()
}

// isValidSourceType checks whether a source type is one of the allowed
// values from the content_imports CHECK constraint.
func isValidSourceType(s string) bool {
	switch s {
	case SourceTypeMedium, SourceTypeSubstack, SourceTypeRSS, SourceTypeZIP:
		return true
	default:
		return false
	}
}

// marshalErrorLog serializes a slice of ImportError into the JSONB format
// stored in content_imports.error_log. Returns "[]" for empty or nil input.
func marshalErrorLog(errors []ImportError) []byte {
	if len(errors) == 0 {
		return []byte("[]")
	}
	data, err := json.Marshal(errors)
	if err != nil {
		slog.Error("contentimport: marshal error log", "error", err)
		return []byte("[]")
	}
	return data
}

// nowTimestamptz returns the current time as a valid pgtype.Timestamptz
// in UTC, for setting completed_at on import records.
func nowTimestamptz() pgtype.Timestamptz {
	return pgtype.Timestamptz{
		Time:  time.Now().UTC(),
		Valid: true,
	}
}
