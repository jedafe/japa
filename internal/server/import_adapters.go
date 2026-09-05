// Package server — import_adapters.go provides the adapters that bridge
// the contentimport package's interfaces (ContentCreator, MediaFetcher) to
// the concrete content.Service and media.Service implementations. They live
// here to avoid a circular import: contentimport defines narrow interfaces
// so it never imports content or media directly.
//
// Ref: japaconnect-engineering/sections/65-content-import.md
//
//	japaconnect-engineering/adr/ADR-083-content-import.md
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/contentimport"
	"github.com/jedafe/japa/internal/media"
)

// ---------------------------------------------------------------------------
// contentCreatorAdapter — adapts content.Service to contentimport.ContentCreator
// ---------------------------------------------------------------------------

// contentCreatorAdapter wraps content.Service so it satisfies the
// contentimport.ContentCreator interface. The import package defines its own
// CreateContentInput/ContentRow types to stay testable in isolation; this
// adapter translates between the two.
type contentCreatorAdapter struct {
	svc *content.Service
}

// newContentCreatorAdapter creates a contentCreatorAdapter.
func newContentCreatorAdapter(svc *content.Service) *contentCreatorAdapter {
	return &contentCreatorAdapter{svc: svc}
}

// CreateContent maps the import package's input to content.CreateContentInput,
// delegates to content.Service, and returns the minimal ContentRow (just the
// ID — the import pipeline only needs it to enqueue import:fetch_media).
func (a *contentCreatorAdapter) CreateContent(ctx context.Context, tenantID pgtype.UUID, input contentimport.CreateContentInput) (contentimport.ContentRow, error) {
	row, err := a.svc.CreateContent(ctx, tenantID, content.CreateContentInput{
		AuthorID:     input.AuthorID,
		ContentType:  input.ContentType,
		Title:        input.Title,
		Body:         input.Body,
		CanonicalUrl: input.CanonicalURL,
		Status:       input.Status,
		Visibility:   input.Visibility,
	})
	if err != nil {
		return contentimport.ContentRow{}, fmt.Errorf("import content creator: %w", err)
	}

	return contentimport.ContentRow{ID: row.ID}, nil
}

// ---------------------------------------------------------------------------
// mediaFetcherAdapter — adapts content.Service + media.Service to
// contentimport.MediaFetcher
// ---------------------------------------------------------------------------

// mediaFetcherAdapter implements contentimport.MediaFetcher by reading the
// imported content's TipTap body, finding external image URLs, downloading
// each via media.Service.StoreFromURL, replacing the src with the stored
// media's imgproxy URL, and updating the content body.
type mediaFetcherAdapter struct {
	content *content.Service
	media   *media.Service
}

// newMediaFetcherAdapter creates a mediaFetcherAdapter.
func newMediaFetcherAdapter(contentSvc *content.Service, mediaSvc *media.Service) *mediaFetcherAdapter {
	return &mediaFetcherAdapter{content: contentSvc, media: mediaSvc}
}

// tiptapNode is a minimal representation of a TipTap JSON node for tree
// walking. Only the fields we need to inspect/modify are typed; the rest
// pass through as raw JSON.
type tiptapNode struct {
	Type    string          `json:"type"`
	Attrs   json.RawMessage `json:"attrs,omitempty"`
	Content []tiptapNode    `json:"content,omitempty"`
	Marks   []tiptapNode    `json:"marks,omitempty"`
	Text    string          `json:"text,omitempty"`
}

// imageAttrs holds the attrs of a TipTap image node.
type imageAttrs struct {
	Src   string `json:"src"`
	Alt   string `json:"alt,omitempty"`
	Title string `json:"title,omitempty"`
}

// FetchAndReplaceMedia reads the content body, finds external image URLs,
// downloads each via the media pipeline, replaces the src with the stored
// media URL, and saves the updated body.
func (a *mediaFetcherAdapter) FetchAndReplaceMedia(ctx context.Context, tenantID, userID, contentID pgtype.UUID) error {
	// 1. Read the current content body.
	item, err := a.content.GetContent(ctx, contentID, tenantID)
	if err != nil {
		return fmt.Errorf("reading content for media fetch: %w", err)
	}

	body := item.Body
	if strings.TrimSpace(body) == "" || body == "null" {
		return nil // nothing to fetch
	}

	// 2. Parse the TipTap JSON and collect external image URLs.
	var doc tiptapNode
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		return fmt.Errorf("parsing tiptap body: %w", err)
	}

	changed, newBody, err := a.replaceExternalImages(ctx, &doc, tenantID, userID)
	if err != nil {
		return err
	}

	if !changed {
		return nil // no external images found
	}

	// 3. Update the content body with the replaced URLs.
	_, err = a.content.UpdateContent(ctx, contentID, tenantID, userID, content.UpdateContentInput{
		Body: newBody,
	}, true)
	if err != nil {
		return fmt.Errorf("updating content body after media fetch: %w", err)
	}

	return nil
}

// replaceExternalImages walks the TipTap tree, downloads external image srcs
// via the media pipeline, and replaces them with stored media URLs. Returns
// (changed, newBody, error). When no external images are found, changed is
// false and newBody is empty — the caller skips the update.
func (a *mediaFetcherAdapter) replaceExternalImages(ctx context.Context, doc *tiptapNode, tenantID, userID pgtype.UUID) (bool, string, error) {
	changed := a.walkAndReplace(ctx, doc, tenantID, userID)
	if !changed {
		return false, "", nil
	}

	out, err := json.Marshal(doc)
	if err != nil {
		return false, "", fmt.Errorf("re-marshaling tiptap body: %w", err)
	}

	return true, string(out), nil
}

// walkAndReplace recursively walks the TipTap node tree, replacing external
// image src URLs with stored media URLs. Returns true if any replacement
// was made.
func (a *mediaFetcherAdapter) walkAndReplace(ctx context.Context, node *tiptapNode, tenantID, userID pgtype.UUID) bool {
	changed := false

	if node.Type == "image" && len(node.Attrs) > 0 {
		var attrs imageAttrs
		if err := json.Unmarshal(node.Attrs, &attrs); err == nil {
			if isExternalImageURL(attrs.Src) {
				if newURL, ok := a.fetchAndStore(ctx, tenantID, userID, attrs.Src); ok {
					attrs.Src = newURL
					if updated, err := json.Marshal(attrs); err == nil {
						node.Attrs = updated
						changed = true
					}
				}
			}
		}
	}

	for i := range node.Content {
		if a.walkAndReplace(ctx, &node.Content[i], tenantID, userID) {
			changed = true
		}
	}

	// Also check link marks on text nodes — some imported content wraps
	// images in links pointing to external CDNs.
	for i := range node.Marks {
		if a.walkAndReplace(ctx, &node.Marks[i], tenantID, userID) {
			changed = true
		}
	}

	return changed
}

// fetchAndStore downloads an external image via media.Service.StoreFromURL
// and returns the imgproxy URL for the stored media. Returns (url, false)
// if the download fails — the original URL is kept.
func (a *mediaFetcherAdapter) fetchAndStore(ctx context.Context, tenantID, userID pgtype.UUID, sourceURL string) (string, bool) {
	filename := filenameFromURL(sourceURL)

	mediaID, err := a.media.StoreFromURL(ctx, tenantID, userID, sourceURL, filename)
	if err != nil {
		return "", false
	}

	m, err := a.media.GetMedia(ctx, tenantID, mediaID)
	if err != nil {
		return "", false
	}

	return a.media.GetMediaURL(&m, media.ImageProxyOpts{}), true
}

// isExternalImageURL returns true for http(s) URLs that are not already
// pointing to the platform's own media/imgproxy path.
func isExternalImageURL(src string) bool {
	if src == "" {
		return false
	}

	if !strings.HasPrefix(src, "http://") && !strings.HasPrefix(src, "https://") {
		return false
	}
	// Skip already-stored media (imgproxy URLs, relative /media/ paths).
	if strings.Contains(src, "/imgproxy/") || strings.HasPrefix(src, "/media/") {
		return false
	}

	return true
}

// filenameFromURL extracts a filename from a URL for the media record.
func filenameFromURL(rawURL string) string {
	// Strip query string and fragment.
	if idx := strings.IndexAny(rawURL, "?#"); idx >= 0 {
		rawURL = rawURL[:idx]
	}
	// Take the last path segment.
	if idx := strings.LastIndex(rawURL, "/"); idx >= 0 && idx < len(rawURL)-1 {
		return rawURL[idx+1:]
	}

	return "imported-image"
}
