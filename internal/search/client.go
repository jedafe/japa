// Package search provides pluggable-engine search for JapaConnect.
// // The client file owns the index catalog and engine bootstrapping. The
// engine itself sits behind the Engine port (engine.go); Meilisearch is the
// only driver today (meili.go). Every index uses "id" (a UUID string) as its
// primary key and exposes as a filterable attribute so searches
// can be scoped to the requesting platform. JapaConnect operates as a single platform.
// // Ref: japaconnect-engineering/sections/13-search.md (Search Engine Port)
package search

import (
	"context"
	"fmt"
	"log/slog"
)

// Meilisearch index UIDs — one per searchable entity type.
const (
	IndexPosts             = "posts"                      // blog posts, discussions, pages (contents table)
	IndexForumThreads      = "forum_threads"              // Q&A threads
	IndexSpaces            = "spaces"                     // geo-first community spaces
	IndexUsers             = "users"                      // user profiles
	IndexTags              = "tags"                       // forum tags
	IndexKnowledgeArticles = "knowledge_articles"         // learning platform articles (§13, ADR-069)
	IndexAcademyTracks    = "academy_tracks"            // Japa Academy marketplace courses (ADR-063)
	IndexDocPages          = "doc_pages"                  // documentation pages (SEARCH-001)
	IndexOrganizations     = "organizations"              // verified orgs/businesses (Section 54)
	IndexLanguageTestReqs  = "language_test_requirements" // §99 language test score requirements (JapaBot RAG)
	IndexPofRequirements   = "pof_requirements"           // §108 POF requirement rows (JapaBot RAG)
	IndexEcaGuides         = "eca_guides"                 // §109 ECA guides (JapaBot RAG)
	IndexSopSamples        = "sop_samples"                // §110 published SOP samples (JapaBot RAG)
)

// PrimaryKey is the primary key used by every Meilisearch index. All
// JapaConnect entities use UUID strings as document IDs.
const PrimaryKey = "id"

// allIndexes is the complete list of indexes ensured at startup.
var allIndexes = []string{
	IndexPosts,
	IndexForumThreads,
	IndexSpaces,
	IndexUsers,
	IndexTags,
	IndexKnowledgeArticles,
	IndexAcademyTracks,
	IndexDocPages,
	IndexOrganizations,
	IndexLanguageTestReqs,
	IndexPofRequirements,
	IndexEcaGuides,
	IndexSopSamples,
}

// filterableAttributesByIndex declares which attributes each index exposes
// for filtering.
var filterableAttributesByIndex = map[string][]string{
	IndexPosts:             {"content_type", "status", "relevant_countries", "space_id", "tag_slugs"},
	IndexForumThreads:      {"space_id", "solved"},
	IndexSpaces:            {"level", "status"},
	IndexUsers:             {"status"},
	IndexTags:              {},
	IndexKnowledgeArticles: {"status"},
	IndexAcademyTracks:    {"difficulty", "category", "language", "relevant_countries", "is_free"},
	IndexDocPages:          {"audience", "review_status"},
	IndexOrganizations:     {"org_type", "verified", "status", "country_code"},
	IndexLanguageTestReqs:  {"test_slug", "destination_space_id", "visa_category", "is_japabot_indexed"},
	IndexPofRequirements:   {"destination_space_id", "visa_type", "is_japabot_indexed"},
	IndexEcaGuides:         {"destination_space_id", "provider_slug", "purpose", "is_japabot_indexed"},
	IndexSopSamples:        {"destination_space_id", "program_level", "field_of_study", "is_japabot_indexed"},
}

// sortableAttributesByIndex declares which attributes each index exposes for
// the §54 ?sort= param. created_at is sortable on every index so "latest"
// works universally. engagement_score is sortable on the posts index so
// "top" works for content.
var sortableAttributesByIndex = map[string][]string{
	IndexPosts:             {"created_at", "engagement_score"},
	IndexForumThreads:      {"created_at"},
	IndexSpaces:            {"created_at"},
	IndexUsers:             {"created_at"},
	IndexTags:              {"created_at"},
	IndexKnowledgeArticles: {"created_at"},
	IndexAcademyTracks:    {"created_at"},
	IndexDocPages:          {"created_at"},
	IndexOrganizations:     {"created_at"},
	IndexLanguageTestReqs:  {"created_at"},
	IndexPofRequirements:   {"created_at"},
	IndexEcaGuides:         {"created_at"},
	IndexSopSamples:        {"created_at"},
}

// NewEngine creates the configured search engine and ensures every search
// index exists with the correct schema. Meilisearch is the only driver
// today — the Engine port is the seam for adding another backend later
// without touching callers.
// // The engine is returned even when it is unreachable: index setup is
// best-effort and failures are logged as warnings. This lets the server
// boot without a running search engine; the search handler degrades
// gracefully to empty results and the index sync task skips when the
// engine cannot be reached.
func NewEngine(url, key string) (Engine, error) {
	engine, err := newMeiliEngine(url, key)
	if err != nil {
		return nil, err
	}

	if err := ensureIndexes(context.Background(), engine); err != nil {
		slog.Warn("search engine index setup incomplete — search will degrade until the engine is reachable", "error", err)
	}

	slog.Info("search engine created", "engine", engine.Name(), "url", url)

	return engine, nil
}

// ensureIndexes applies the index catalog through the Engine port: each
// index is created (idempotent) with its filterable and sortable attributes
// so platform-scoped filtering works. A nil error means the engine accepted
// the schema — some engines apply it asynchronously. A single failure
// short-circuits so the caller logs one clear warning instead of one per
// index.
func ensureIndexes(ctx context.Context, engine Engine) error {
	for _, uid := range allIndexes {
		if err := engine.EnsureIndex(ctx, IndexSchema{
			UID:                  uid,
			PrimaryKey:           PrimaryKey,
			FilterableAttributes: filterableAttributesByIndex[uid],
			SortableAttributes:   sortableAttributesByIndex[uid],
		}); err != nil {
			return fmt.Errorf("index %q: %w", uid, err)
		}
	}

	return nil
}
