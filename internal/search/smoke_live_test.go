package search

// Temporary live smoke test against a real Meilisearch instance.
// Not part of CI — run manually: SEARCH_LIVE_URL=http://localhost:7700 go test ./internal/search/ -run TestLiveEngine -v

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestLiveEngine(t *testing.T) {
	url := os.Getenv("SEARCH_LIVE_URL")
	if url == "" {
		t.Skip("set SEARCH_LIVE_URL to run against a live engine")
	}

	ctx := context.Background()

	engine, err := NewEngine(url, "")
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	if engine == nil {
		t.Fatal("engine is nil")
	}
	if engine.Name() != "Meilisearch" {
		t.Errorf("Name() = %q, want Meilisearch", engine.Name())
	}
	if err := engine.Health(ctx); err != nil {
		t.Fatalf("Health: %v", err)
	}

	// Dedicated throwaway index.
	const idx = "__engine_smoke"

	if err := engine.EnsureIndex(ctx, IndexSchema{
		UID:                  idx,
		PrimaryKey:           "id",
		FilterableAttributes: []string{"relevant_countries", "review_status", "audience"},
		SortableAttributes:   []string{"created_at"},
	}); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}

	// EnsureIndex enqueues async engine tasks — poll until the index exists.
	waitForIndex := func() error {
		for range 60 {
			_, err := engine.Search(ctx, idx, Query{IndexUID: idx, Text: "", Limit: 1})
			if err == nil {
				return nil
			}
			time.Sleep(200 * time.Millisecond)
		}
		return err
	}
	if err := waitForIndex(); err != nil {
		t.Fatalf("index never became ready: %v", err)
	}

	doc := map[string]any{
		"id":                 "smoke-1",
		"title":              "smoke test document",
		"excerpt":            "hello world",
		"url":                "/smoke",
		"relevant_countries": []string{"NG"},
		"review_status":      "published",
		"audience":           "member",
		"created_at":         int64(1700000000),
	}

	if err := engine.UpsertDocuments(ctx, idx, []map[string]any{doc}); err != nil {
		t.Fatalf("UpsertDocuments: %v", err)
	}

	// Indexing is async — poll until the document is searchable.
	for range 50 {
		r, err := engine.Search(ctx, idx, Query{IndexUID: idx, Text: "smoke", Limit: 1})
		if err == nil && len(r.Hits) > 0 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Single-index search with the geo filter shape.
	res, err := engine.Search(ctx, idx, Query{
		IndexUID: idx,
		Text:     "smoke",
		Limit:    10,
		Filter: andAll(
			orAny(
				IsEmpty{Field: "relevant_countries"},
				Eq{Field: "relevant_countries", Value: "NG"},
			),
		),
		Sort:                 []string{"created_at:desc"},
		AttributesToRetrieve: []string{"id", "title"},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Hits) != 1 {
		t.Fatalf("Search hits = %d, want 1", len(res.Hits))
	}
	if got := stringField(res.Hits[0], "id"); got != "smoke-1" {
		t.Errorf("hit id = %q", got)
	}

	// Geo mismatch should return zero hits.
	res2, err := engine.Search(ctx, idx, Query{
		IndexUID: idx,
		Text:     "smoke",
		Filter: andAll(
			orAny(
				IsEmpty{Field: "relevant_countries"},
				Eq{Field: "relevant_countries", Value: "JP"},
			),
		),
	})
	if err != nil {
		t.Fatalf("Search(geo mismatch): %v", err)
	}
	if len(res2.Hits) != 0 {
		t.Errorf("geo-mismatched search returned %d hits, want 0", len(res2.Hits))
	}

	// MultiSearch with the docs-style In filter.
	multi, err := engine.MultiSearch(ctx, []Query{
		{
			IndexUID: idx,
			Text:     "smoke",
			Filter: andAll(
				Eq{Field: "review_status", Value: "published"},
				In{Field: "audience", Values: []string{"member", "public"}},
			),
		},
		{
			IndexUID: idx,
			Text:     "smoke",
			Filter:   Eq{Field: "review_status", Value: "draft"},
		},
	})
	if err != nil {
		t.Fatalf("MultiSearch: %v", err)
	}
	if len(multi) != 2 {
		t.Fatalf("MultiSearch results = %d, want 2", len(multi))
	}
	if multi[0].IndexUID != idx || len(multi[0].Hits) != 1 {
		t.Errorf("first result: IndexUID=%q hits=%d, want %q/1", multi[0].IndexUID, len(multi[0].Hits), idx)
	}
	if len(multi[1].Hits) != 0 {
		t.Errorf("second result hits = %d, want 0 (mismatched filter)", len(multi[1].Hits))
	}

	// Hit.DecodeInto on a real engine hit.
	var decoded docPageHit
	if err := multi[0].Hits[0].DecodeInto(&decoded); err != nil {
		t.Fatalf("DecodeInto: %v", err)
	}

	// Delete the doc and confirm it's gone.
	if err := engine.DeleteDocument(ctx, idx, "smoke-1"); err != nil {
		t.Fatalf("DeleteDocument: %v", err)
	}
	// Deletion is async — poll until the document is gone.
	for range 50 {
		r, err := engine.Search(ctx, idx, Query{IndexUID: idx, Text: "smoke"})
		if err == nil && len(r.Hits) == 0 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	res3, err := engine.Search(ctx, idx, Query{IndexUID: idx, Text: "smoke"})
	if err != nil {
		t.Fatalf("post-delete Search: %v", err)
	}
	if len(res3.Hits) != 0 {
		t.Errorf("post-delete hits = %d, want 0", len(res3.Hits))
	}
}
