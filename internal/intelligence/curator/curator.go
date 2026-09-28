// Package curator implements the deterministic, no-LLM record normalizers
// for the intel-channels Phase 1 "curate" pipeline. Where the extract
// pipeline sends a raw page body to the LLM extractor, the curate pipeline
// parses already-structured channel bodies — RSS feed items, ld+json nodes,
// iCal VEVENTs, tabular rows — straight into validated extractor.Extraction
// records with confidence 1.0 and a stable external_key.
//
// The raw-doc body contracts are the fetcher's output formats (Task 8):
//   - rss:     the feed XML document
//   - jsonld:  the page's <script type="application/ld+json"> blocks,
//     newline-joined (each block is a JSON object or array)
//   - ical:    the raw iCalendar bytes
//   - tabular: the raw CSV/TSV bytes
//   - sitemap: a "url<TAB>lastmod" line manifest — no curator is registered
//     for it, so sitemap channels fall back to the extract pipeline.
//
// The worker package cannot import this package (import cycle through
// internal/ai → internal/auth → worker); it consumes the registry through
// the intelCuratorPort interface and a server.go adapter.
package curator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/jedafe/japa/internal/intelligence/extractor"
	"github.com/jedafe/japa/internal/intelligence/extractor/schemas"
	"github.com/jedafe/japa/internal/intelligence/sources"
)

// Record is one curated record: a deterministic extraction plus the stable
// external key the dedup/upsert chain relies on. EntityType is the
// per-curator override (e.g. a ld+json node's @type mapping); empty means
// the worker falls back to the channel's entity_types[0].
type Record struct {
	Extraction  extractor.Extraction
	ExternalKey string
	EntityType  string
}

// Curator normalizes one crawl-method body format into curated records.
// Handles returns the intel_source_channels.crawl_method value the curator
// serves (rss, jsonld, ical, tabular).
type Curator interface {
	Handles() string
	Curate(ctx context.Context, body []byte, contentType string, ch sources.Channel) ([]Record, error)
}

// ErrNoCurator is returned by Registry.Curate when no curator handles the
// channel's crawl method. The worker treats it as a signal to fall back to
// the intel:extract pipeline rather than a failure.
var ErrNoCurator = errors.New("curator: no curator for crawl method")

// Registry dispatches a channel body to the curator registered for its
// crawl method.
type Registry struct {
	byMethod map[string]Curator
}

// NewRegistry builds a Registry over the given curators, keyed by their
// Handles() value. A duplicate Handles() value panics — registration is
// static configuration, not user input.
func NewRegistry(curators ...Curator) *Registry {
	r := &Registry{byMethod: make(map[string]Curator, len(curators))}

	for _, c := range curators {
		if _, dup := r.byMethod[c.Handles()]; dup {
			panic(fmt.Sprintf("curator: duplicate registration for %q", c.Handles()))
		}

		r.byMethod[c.Handles()] = c
	}

	return r
}

// Default returns the curator set: rss, jsonld, ical, tabular, api,
// static, oneshot (deterministic extraction), plus email, websub, and
// search (Phase 3). Crawl methods with no curator (sitemap, git, social,
// …) resolve to ErrNoCurator and the worker falls back to intel:extract —
// the same fallback a curator signals when a body is below its quality bar.
func Default() *Registry {
	return NewRegistry(
		RSSNews{},
		JSONLD{},
		ICal{},
		Tabular{},
		API{},
		StaticHTML{method: string(sources.CrawlMethodStatic)},
		StaticHTML{method: string(sources.CrawlMethodOneshot)},
		Email{},
		WebSub{},
		Search{},
	)
}

// Curate dispatches to the curator registered for ch.CrawlMethod, or
// returns ErrNoCurator when none is registered.
func (r *Registry) Curate(ctx context.Context, body []byte, contentType string, ch sources.Channel) ([]Record, error) {
	c, ok := r.byMethod[ch.CrawlMethod]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrNoCurator, ch.CrawlMethod)
	}

	return c.Curate(ctx, body, contentType, ch)
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// field adds a value/confidence/evidence triple to the extraction.
// Deterministic curators emit confidence 1.0 — the value came verbatim from
// a structured document, not a model guess. evidence is the raw source
// snippet, satisfying the hallucination guard in extractor.Validate.
//
// []string values are converted to []any: Validate's array coercers
// type-assert []any (the JSON-decoded shape) and would otherwise reset the
// field to null.
func field(ext extractor.Extraction, name string, value any, evidence string) {
	if value == nil {
		return
	}

	switch v := value.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return
		}
	case []string:
		arr := make([]any, 0, len(v))
		for _, s := range v {
			if strings.TrimSpace(s) != "" {
				arr = append(arr, s)
			}
		}
		if len(arr) == 0 {
			return
		}
		value = arr
	}

	ev := evidence
	ext[name] = extractor.FieldExtraction{Value: value, Confidence: 1.0, Evidence: &ev}
}

// hashKey returns a stable external key component: the SHA-256 hex of the
// joined parts. Used by every curator's last-resort key derivation so a
// re-crawl of unchanged content reproduces the same key.
func hashKey(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(h[:])
}

// resolveEntityType returns the record's entity type: the curator's own
// override first (ld+json @type mapping), then the channel's declared
// entity_types[0]. Empty means the worker decides (or routes to review).
func resolveEntityType(override string, ch sources.Channel) string {
	if override != "" {
		return override
	}

	for _, et := range ch.EntityTypes {
		if et != "" {
			return et
		}
	}

	return ""
}

// validate runs the extraction through the schema's hallucination guard and
// type coercers when a schema is registered for the entity type, so curated
// records carry exactly the schema's field set — the same cleaned shape the
// LLM extract path persists. Validation errors are informational only here
// (a missing required field is a routing signal for the hybrid pipeline,
// checked by the worker against the stored extraction).
func validate(ext extractor.Extraction, entityType string) extractor.Extraction {
	schema, ok := schemas.ForEntityType(entityType)
	if !ok {
		return ext
	}

	cleaned, _ := extractor.Validate(ext, schema)

	return cleaned
}
