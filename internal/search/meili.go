// Package search — meili.go is the Meilisearch driver for the Engine port.
//
// It translates the engine-neutral Query/FilterExpr types into
// meilisearch-go calls and converts responses back into the neutral
// IndexResult/Hit types at the boundary.
//
// Ref: japaconnect-engineering/sections/13-search.md (Search Engine Port)
package search

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/meilisearch/meilisearch-go"
)

// meiliEngine implements Engine over a Meilisearch client.
type meiliEngine struct {
	client meilisearch.ServiceManager
	// indexPrefix namespaces physical index UIDs (JC_MEILI_INDEX_PREFIX) so
	// a non-production environment can share one instance without colliding
	// with another environment's indexes. Callers always see logical names.
	indexPrefix string
}

// indexPrefixPattern is the safe charset for a Meilisearch index UID prefix
// (index UIDs allow alphanumerics, hyphens, and underscores).
var indexPrefixPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// newMeiliEngine creates the Meilisearch-backed Engine. The client is
// returned even when Meilisearch is unreachable: index setup is best-effort
// and failures are logged as warnings. This lets the server boot without a
// running search engine — handlers degrade to empty results and the index
// sync task skips until the engine is reachable.
//
// Note: the meilisearch-go v0.36 API exposes a ServiceManager interface
// rather than a concrete *Client.
func newMeiliEngine(url, key, indexPrefix string) (*meiliEngine, error) {
	if url == "" {
		return nil, errors.New("meilisearch URL is required")
	}
	if indexPrefix != "" && !indexPrefixPattern.MatchString(indexPrefix) {
		return nil, fmt.Errorf("invalid meilisearch index prefix %q (letters, digits, '-' and '_' only)", indexPrefix)
	}

	return &meiliEngine{client: meilisearch.New(url, meilisearch.WithAPIKey(key)), indexPrefix: indexPrefix}, nil
}

// ix maps a logical index name to the physical Meilisearch UID.
func (e *meiliEngine) ix(logical string) string { return e.indexPrefix + logical }

// unix maps a physical Meilisearch UID back to the logical index name so
// consumers see catalog names regardless of the configured prefix.
func (e *meiliEngine) unix(uid string) string { return strings.TrimPrefix(uid, e.indexPrefix) }

// Name implements Engine.
func (e *meiliEngine) Name() string { return "Meilisearch" }

// Health implements Engine.
func (e *meiliEngine) Health(ctx context.Context) error {
	_, err := e.client.HealthWithContext(ctx)

	return err
}

// EnsureIndex implements Engine. Index creation and filterable attributes
// are fatal; sortable attributes are best-effort (older Meilisearch versions
// may not support them — sorting then degrades to native relevance).
func (e *meiliEngine) EnsureIndex(ctx context.Context, schema IndexSchema) error {
	setupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if _, err := e.client.CreateIndexWithContext(setupCtx, &meilisearch.IndexConfig{
		Uid:        e.ix(schema.UID),
		PrimaryKey: schema.PrimaryKey,
	}); err != nil {
		return fmt.Errorf("creating index %q: %w", schema.UID, err)
	}

	filterable := stringsToInterface(schema.FilterableAttributes)
	if _, err := e.client.Index(e.ix(schema.UID)).UpdateFilterableAttributesWithContext(setupCtx, &filterable); err != nil {
		return fmt.Errorf("setting filterable attributes for index %q: %w", schema.UID, err)
	}

	if _, err := e.client.Index(e.ix(schema.UID)).UpdateSortableAttributesWithContext(setupCtx, &schema.SortableAttributes); err != nil {
		slog.Warn("meilisearch: failed to set sortable attributes (non-fatal)",
			"index", schema.UID, "error", err)
	}

	return nil
}

// Search implements Engine.
func (e *meiliEngine) Search(ctx context.Context, index string, q Query) (*IndexResult, error) {
	resp, err := e.client.Index(e.ix(index)).SearchWithContext(ctx, q.Text, e.toSearchRequest(q))
	if err != nil {
		return nil, err
	}

	return e.toIndexResult(resp), nil
}

// MultiSearch implements Engine. Meilisearch runs all queries in one HTTP
// call; results come back in request order with per-query IndexUID.
func (e *meiliEngine) MultiSearch(ctx context.Context, queries []Query) ([]IndexResult, error) {
	meiliQueries := make([]*meilisearch.SearchRequest, len(queries))
	for i, q := range queries {
		meiliQueries[i] = e.toSearchRequest(q)
	}

	resp, err := e.client.MultiSearchWithContext(ctx, &meilisearch.MultiSearchRequest{
		Queries: meiliQueries,
	})
	if err != nil {
		return nil, err
	}

	results := make([]IndexResult, len(resp.Results))
	for i, sr := range resp.Results {
		results[i] = *e.toIndexResult(&sr)
	}

	return results, nil
}

// UpsertDocuments implements Engine. docs is marshaled to JSON as-is; pass a
// slice (or pointer to a slice) of structs or maps.
func (e *meiliEngine) UpsertDocuments(ctx context.Context, index string, docs any) error {
	_, err := e.client.Index(e.ix(index)).AddDocumentsWithContext(ctx, docs, nil)

	return err
}

// DeleteDocument implements Engine.
func (e *meiliEngine) DeleteDocument(ctx context.Context, index, docID string) error {
	_, err := e.client.Index(e.ix(index)).DeleteDocumentWithContext(ctx, docID, nil)

	return err
}

// toSearchRequest translates the neutral Query into a Meilisearch
// SearchRequest. The Filter AST is rendered to Meilisearch filter syntax;
// Sort passes through in the shared "field:dir" form.
func (e *meiliEngine) toSearchRequest(q Query) *meilisearch.SearchRequest {
	req := &meilisearch.SearchRequest{
		IndexUID:             e.ix(q.IndexUID),
		Query:                q.Text,
		Limit:                int64(q.Limit),
		Offset:               int64(q.Offset),
		Sort:                 q.Sort,
		AttributesToRetrieve: q.AttributesToRetrieve,
	}

	if f := renderMeiliFilter(q.Filter); f != "" {
		req.Filter = f
	}

	return req
}

// toIndexResult converts a Meilisearch SearchResponse to the neutral
// IndexResult. meilisearch.Hit and Hit share the same underlying map type,
// so conversion is a cast, not a copy. The physical index UID is mapped back
// to the logical name so consumers match the index catalog unprefixed.
func (e *meiliEngine) toIndexResult(resp *meilisearch.SearchResponse) *IndexResult {
	hits := make([]Hit, len(resp.Hits))
	for i, h := range resp.Hits {
		hits[i] = Hit(h)
	}

	return &IndexResult{
		IndexUID:           e.unix(resp.IndexUID),
		Hits:               hits,
		EstimatedTotalHits: resp.EstimatedTotalHits,
	}
}

// ---------------------------------------------------------------------------
// FilterExpr → Meilisearch filter syntax
// ---------------------------------------------------------------------------

// renderMeiliFilter translates the filter AST into a Meilisearch filter
// expression. Returns "" for nil or empty filters so callers can omit the
// Filter field entirely.
//
// Rendering rules:
//   - Eq on an array attribute is membership: tag_slugs = "x" matches
//     documents whose tag_slugs array contains "x".
//   - Composite children are always parenthesized so precedence never
//     depends on operand order.
//   - In with no values renders "" — callers must not emit empty sets.
func renderMeiliFilter(f FilterExpr) string {
	switch v := f.(type) {
	case nil:
		return ""
	case Eq:
		return fmt.Sprintf(`%s = "%s"`, v.Field, escapeMeiliString(v.Value))
	case In:
		if len(v.Values) == 0 {
			return ""
		}

		quoted := make([]string, len(v.Values))
		for i, s := range v.Values {
			quoted[i] = `"` + escapeMeiliString(s) + `"`
		}

		return fmt.Sprintf(`%s IN [%s]`, v.Field, strings.Join(quoted, ", "))
	case IsEmpty:
		return v.Field + " IS EMPTY"
	case And:
		parts := make([]string, 0, len(v.All))

		for _, e := range v.All {
			if s := renderMeiliFilter(e); s != "" {
				parts = append(parts, "("+s+")")
			}
		}

		return strings.Join(parts, " AND ")
	case Or:
		parts := make([]string, 0, len(v.Any))

		for _, e := range v.Any {
			if s := renderMeiliFilter(e); s != "" {
				parts = append(parts, "("+s+")")
			}
		}

		return strings.Join(parts, " OR ")
	default:
		return ""
	}
}

// escapeMeiliString escapes a value for embedding in a double-quoted
// Meilisearch filter literal.
func escapeMeiliString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)

	return strings.ReplaceAll(s, `"`, `\"`)
}

// stringsToInterface converts a string slice to an interface slice, the
// shape meilisearch-go expects for filterable attribute updates.
func stringsToInterface(ss []string) []interface{} {
	out := make([]interface{}, len(ss))
	for i, s := range ss {
		out[i] = s
	}

	return out
}
