// Package search — engine.go defines the engine-neutral search port.
//
// Engine is the seam that lets the search backend be swapped (Typesense,
// a Postgres-native engine such as ParadeDB, or another provider) without
// touching handlers, indexers, or consumers. Meilisearch is the only driver
// today (meili.go); this file contains no engine-specific types.
//
// Ref: japaconnect-engineering/sections/13-search.md (Search Engine Port)
package search

import (
	"context"
	"encoding/json"
)

// Engine is the search backend port. Implementations translate the neutral
// Query/FilterExpr types into their engine's native API.
type Engine interface {
	// Name returns the engine identifier for logs and health checks
	// (e.g. "Meilisearch").
	Name() string

	// Health checks engine connectivity. Used by the control-panel
	// health view.
	Health(ctx context.Context) error

	// EnsureIndex creates the index (idempotent) and applies the schema
	// configuration (primary key, filterable and sortable attributes).
	EnsureIndex(ctx context.Context, schema IndexSchema) error

	// Search queries a single index.
	Search(ctx context.Context, index string, q Query) (*IndexResult, error)

	// MultiSearch runs one query per index in a single round-trip and
	// returns one IndexResult per query, in request order.
	MultiSearch(ctx context.Context, queries []Query) ([]IndexResult, error)

	// UpsertDocuments indexes or replaces documents. docs must be a slice
	// (or pointer to a slice) of JSON-marshalable values; documents sharing
	// the index primary key are replaced, so this is the single write path
	// for both create and update.
	UpsertDocuments(ctx context.Context, index string, docs any) error

	// DeleteDocument removes a document by primary key.
	DeleteDocument(ctx context.Context, index, docID string) error
}

// IndexSchema declares an index's shape in engine-neutral terms. Engines map
// these to their own index/collection configuration.
type IndexSchema struct {
	UID                  string   // index name
	PrimaryKey           string   // document primary key field ("id")
	FilterableAttributes []string // fields usable in FilterExpr
	SortableAttributes   []string // fields usable in Sort
}

// Query is a single-index search request in engine-neutral terms.
type Query struct {
	IndexUID string
	Text     string // full-text query string
	Limit    int
	Offset   int
	// Filter is the engine-neutral filter AST — never a raw engine string.
	Filter FilterExpr
	// Sort entries use the shared "field:asc|desc" form (Meilisearch and
	// Typesense both use this syntax). Nil means native relevance ranking.
	Sort                 []string
	AttributesToRetrieve []string
}

// IndexResult is the engine-neutral per-index search response.
type IndexResult struct {
	IndexUID           string
	Hits               []Hit
	EstimatedTotalHits int64
}

// Hit is a single search hit: the document fields returned by the engine.
// It mirrors the map shape every JSON-document engine produces, so mappers
// read fields by key without engine-specific types.
type Hit map[string]json.RawMessage

// DecodeInto decodes a Hit into the provided struct. The intermediate
// marshal keeps this engine-agnostic — field names come from the JSON tags
// on the target struct.
func (h Hit) DecodeInto(out any) error {
	raw, err := json.Marshal(map[string]json.RawMessage(h))
	if err != nil {
		return err
	}

	return json.Unmarshal(raw, out)
}

// ---------------------------------------------------------------------------
// FilterExpr — engine-neutral filter AST
// ---------------------------------------------------------------------------
//
// Handlers and adapters build FilterExpr trees; each engine translates the
// AST to its native syntax (Meilisearch filter expressions today, Typesense
// filter_by or a SQL WHERE clause in future drivers). Never pass raw
// engine-specific filter strings through the port.

// FilterExpr is a node in the filter AST.
type FilterExpr interface {
	isFilterExpr()
}

// Eq matches documents where Field equals Value. On array-typed attributes
// (tag_slugs, relevant_countries) this is membership: the document matches
// when the array contains Value — the same semantics Meilisearch and
// Typesense give to equality on array fields.
type Eq struct {
	Field string
	Value string
}

// In matches documents where Field equals any of Values — the set-membership
// form of an Or-of-Eqs.
type In struct {
	Field  string
	Values []string
}

// IsEmpty matches documents where Field is an empty array or absent — used
// for globally-relevant content (relevant_countries IS EMPTY, ADR-064).
type IsEmpty struct {
	Field string
}

// And matches documents satisfying every child expression.
type And struct {
	All []FilterExpr
}

// Or matches documents satisfying at least one child expression.
type Or struct {
	Any []FilterExpr
}

func (Eq) isFilterExpr()      {}
func (In) isFilterExpr()      {}
func (IsEmpty) isFilterExpr() {}
func (And) isFilterExpr()     {}
func (Or) isFilterExpr()      {}

// andAll builds an And from non-nil children. Returns nil when no children
// remain so callers can treat "no conditions" and "no filter" identically.
func andAll(exprs ...FilterExpr) FilterExpr {
	parts := make([]FilterExpr, 0, len(exprs))

	for _, e := range exprs {
		if e != nil {
			parts = append(parts, e)
		}
	}

	switch len(parts) {
	case 0:
		return nil
	case 1:
		return parts[0]
	default:
		return And{All: parts}
	}
}

// orAny builds an Or from non-nil children. Returns nil when empty.
func orAny(exprs ...FilterExpr) FilterExpr {
	parts := make([]FilterExpr, 0, len(exprs))

	for _, e := range exprs {
		if e != nil {
			parts = append(parts, e)
		}
	}

	switch len(parts) {
	case 0:
		return nil
	case 1:
		return parts[0]
	default:
		return Or{Any: parts}
	}
}
