// Package search provides Meilisearch-backed search for JapaConnect.
//
// The client file owns Meilisearch connection setup and index bootstrapping.
// Every index uses "id" (a UUID string) as its primary key and exposes
// tenant_id as a filterable attribute so searches can be scoped to the
// requesting tenant — multi-tenancy from day one.
package search

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/meilisearch/meilisearch-go"
)

// Meilisearch index UIDs — one per searchable entity type.
const (
	IndexPosts             = "posts"              // blog posts, discussions, pages (contents table)
	IndexForumThreads      = "forum_threads"      // Q&A threads
	IndexSpaces            = "spaces"             // geo-first community spaces
	IndexUsers             = "users"              // user profiles
	IndexTags              = "tags"               // forum tags
	IndexKnowledgeArticles = "knowledge_articles" // learning platform articles
	IndexLearningTracks    = "learning_tracks"    // Japa University marketplace courses (ADR-063)
	IndexDocPages          = "doc_pages"          // documentation pages (SEARCH-001)
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
	IndexLearningTracks,
	IndexDocPages,
}

// filterableAttributesByIndex declares which attributes each index exposes
// for filtering. tenant_id is filterable on every index so searches scope to
// the requesting tenant.
var filterableAttributesByIndex = map[string][]string{
	IndexPosts:             {"tenant_id", "content_type", "status", "relevant_countries"},
	IndexForumThreads:      {"tenant_id", "space_id", "solved"},
	IndexSpaces:            {"tenant_id", "level", "status"},
	IndexUsers:             {"tenant_id", "status"},
	IndexTags:              {"tenant_id"},
	IndexKnowledgeArticles: {"tenant_id", "status"},
	IndexLearningTracks:    {"tenant_id", "difficulty", "category", "language", "relevant_countries", "is_free"},
	IndexDocPages:          {"tenant_id", "audience", "review_status"},
}

// NewClient creates a Meilisearch client and ensures every search index
// exists with the correct filterable attributes.
//
// The client is returned even when Meilisearch is unreachable: index setup
// is best-effort and failures are logged as warnings. This lets the server
// boot without a running Meilisearch instance; the search handler degrades
// gracefully to empty results and the index sync task skips when the client
// cannot reach the engine.
//
// Note: the meilisearch-go v0.36 API exposes a ServiceManager interface
// rather than a concrete *Client, so this returns meilisearch.ServiceManager.
func NewClient(url, key string) (meilisearch.ServiceManager, error) {
	if url == "" {
		return nil, errors.New("meilisearch URL is required")
	}

	client := meilisearch.New(url, meilisearch.WithAPIKey(key))

	if err := ensureIndexes(context.Background(), client); err != nil {
		slog.Warn("meilisearch index setup incomplete — search will degrade until the engine is reachable", "error", err)
	}

	slog.Info("meilisearch client created", "url", url)

	return client, nil
}

// ensureIndexes creates every index (idempotent) and sets filterable
// attributes so tenant-scoped filtering works. Both operations enqueue
// asynchronous Meilisearch tasks; a nil error means every task was accepted,
// not that indexing finished. A single failure short-circuits so the caller
// logs one clear warning instead of one per index.
func ensureIndexes(ctx context.Context, client meilisearch.ServiceManager) error {
	setupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	for _, uid := range allIndexes {
		if _, err := client.CreateIndexWithContext(setupCtx, &meilisearch.IndexConfig{
			Uid:        uid,
			PrimaryKey: PrimaryKey,
		}); err != nil {
			return fmt.Errorf("creating index %q: %w", uid, err)
		}

		filterable := stringsToInterface(filterableAttributesByIndex[uid])
		if _, err := client.Index(uid).UpdateFilterableAttributesWithContext(setupCtx, &filterable); err != nil {
			return fmt.Errorf("setting filterable attributes for index %q: %w", uid, err)
		}
	}

	return nil
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
