package curator

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/jedafe/japa/internal/intelligence/extractor"
	"github.com/jedafe/japa/internal/intelligence/sources"
)

// Search curates search crawl-method channels: the raw doc body is the
// SearchFetcher's application/x-ndjson result page — one
// {"title","url","snippet","published_at?"} object per line. Each line
// becomes one record keyed on the result URL, so re-polls of an unchanged
// SERP dedupe through the (source_id, external_key) index.
type Search struct{}

// Handles returns the crawl method this curator serves.
func (Search) Handles() string { return string(sources.CrawlMethodSearch) }

// searchHit mirrors the fetcher's normalized result object.
type searchHit struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Snippet     string `json:"snippet"`
	PublishedAt string `json:"published_at"`
}

// Curate scans the NDJSON body and emits one record per well-formed hit;
// malformed lines are skipped rather than failing the whole page.
func (Search) Curate(_ context.Context, body []byte, _ string, ch sources.Channel) ([]Record, error) {
	entityType := resolveEntityType("", ch)
	var out []Record

	sc := bufio.NewScanner(bytes.NewReader(body))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var hit searchHit
		if err := json.Unmarshal(line, &hit); err != nil || hit.URL == "" {
			continue
		}

		ext := make(extractor.Extraction)
		field(ext, "title", hit.Title, hit.Title)
		field(ext, "summary", hit.Snippet, hit.Snippet)
		field(ext, "article_url", hit.URL, hit.URL)
		if iso := parseFeedDate(hit.PublishedAt); iso != "" {
			field(ext, "published_date", iso, hit.PublishedAt)
		}

		out = append(out, Record{
			Extraction:  validate(ext, entityType),
			ExternalKey: hit.URL,
			EntityType:  entityType,
		})
	}
	if err := sc.Err(); err != nil {
		return nil, errors.New("curator: search: scan body: " + err.Error())
	}
	if len(out) == 0 {
		return nil, errors.New("curator: search: no results in body")
	}

	return out, nil
}
