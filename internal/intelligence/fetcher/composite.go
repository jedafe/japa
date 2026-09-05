package fetcher

import (
	"context"
	"fmt"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

// Composite is the fetcher adapter that dispatches to the correct per-method
// fetcher (static, rss, api) based on source.CrawlMethod. PDF and OCR methods
// use the StaticFetcher to download raw bytes; text extraction happens in the
// extract step. It is the single Fetcher implementation wired into the worker
// via RegisterIntelDependencies.
//
// Ref: docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md (Section 4).
type Composite struct {
	static *StaticFetcher
	rss    *RSSFetcher
	api    *APIFetcher
}

// NewComposite returns a Composite fetcher with all adapters wired.
func NewComposite() *Composite {
	return &Composite{
		static: NewStaticFetcher(),
		rss:    NewRSSFetcher(),
		api:    NewAPIFetcher(),
	}
}

// Fetch selects the adapter by source.CrawlMethod and delegates. Returns
// ErrUnknownCrawlMethod when the source's crawl_method is not one of the
// supported values.
//
// For PDF and OCR crawl methods, the StaticFetcher downloads the raw bytes
// (the PDF file). Text extraction from those bytes happens in the extract
// step via PDFFetcher.ExtractText / OCRFetcher, not in the fetch step — the
// fetcher's job is to retrieve raw bytes + compute a content hash for change
// detection.
func (c *Composite) Fetch(ctx context.Context, source sources.Source) (FetchResult, error) {
	switch sources.CrawlMethod(source.CrawlMethod) {
	case sources.CrawlMethodStatic, sources.CrawlMethodPDF, sources.CrawlMethodOCR:
		return c.static.Fetch(ctx, source)
	case sources.CrawlMethodRSS:
		return c.rss.Fetch(ctx, source)
	case sources.CrawlMethodAPI:
		return c.api.Fetch(ctx, source)
	default:
		return FetchResult{}, fmt.Errorf("fetcher: unknown crawl_method %q", source.CrawlMethod)
	}
}
