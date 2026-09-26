package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jedafe/japa/internal/ai"
	"github.com/jedafe/japa/internal/intelligence/extractor"
	"github.com/jedafe/japa/internal/intelligence/promoter"
	"github.com/jedafe/japa/internal/worker"
)

// staticGateway is a test double for extractor.AIGateway that always returns
// the configured response content.
type staticGateway struct {
	content string
	calls   int
}

func (g *staticGateway) ChatWithTools(ctx context.Context, category string, req ai.ChatWithToolsRequest) (ai.ChatWithToolsResponse, error) {
	g.calls++
	return ai.ChatWithToolsResponse{Content: g.content}, nil
}

func TestIntelExtractorPortAdapter_Extract_SingleRecord(t *testing.T) {
	body := `{"title":{"value":"DAAD EPOS","confidence":0.95,"evidence":"e"},"country":{"value":["DEU"],"confidence":0.9,"evidence":"e"}}`
	gw := &staticGateway{content: body}
	ext := extractor.NewExtractor(gw)
	adapter := &intelExtractorPortAdapter{ext: ext}

	results, err := adapter.Extract(t.Context(), "scholarship", "https://example.org/daad", "html", "source text")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if !strings.Contains(results[0].ExternalKey, "DAAD EPOS") && !strings.HasPrefix(results[0].ExternalKey, "https://example.org/daad|") {
		t.Errorf("unexpected external key: %q", results[0].ExternalKey)
	}

	if results[0].ConfidenceOverall == 0 {
		t.Errorf("expected non-zero confidence overall")
	}
}

func TestIntelExtractorPortAdapter_Extract_MultiRecord(t *testing.T) {
	body := `{"records":[{"title":{"value":"A","confidence":0.9,"evidence":"e"},"country":{"value":["USA"],"confidence":0.9,"evidence":"e"}},{"title":{"value":"B","confidence":0.8,"evidence":"e"},"country":{"value":["CAN"],"confidence":0.8,"evidence":"e"}}]}`
	gw := &staticGateway{content: body}
	ext := extractor.NewExtractor(gw)
	adapter := &intelExtractorPortAdapter{ext: ext}

	results, err := adapter.Extract(t.Context(), "scholarship", "https://example.org/list", "html", "source text")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	for i, res := range results {
		if res.ExternalKey == "" {
			t.Errorf("result %d: external key empty", i)
		}

		if res.ConfidenceOverall == 0 {
			t.Errorf("result %d: confidence overall zero", i)
		}

		if len(res.RawExtraction) == 0 {
			t.Errorf("result %d: raw extraction empty", i)
		}
	}

	if results[0].ExternalKey == results[1].ExternalKey {
		t.Errorf("expected distinct external keys; got %q and %q", results[0].ExternalKey, results[1].ExternalKey)
	}
}

func TestIntelRecordsToResults_DerivesExternalKeyAndConfidence(t *testing.T) {
	adapter := &intelExtractorPortAdapter{}
	records := []extractor.Extraction{
		{
			"title":   {Value: "MEXT", Confidence: 0.95, Evidence: strPtr("e")},
			"country": {Value: []any{"JP"}, Confidence: 0.9, Evidence: strPtr("e")},
		},
	}

	results, err := adapter.recordsToResults("https://example.org/mext", records)
	if err != nil {
		t.Fatalf("recordsToResults failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if !strings.HasPrefix(results[0].ExternalKey, "https://example.org/mext|") {
		t.Errorf("expected external key to start with source URL, got %q", results[0].ExternalKey)
	}

	want := float32((0.95 + 0.9) / 2)
	if results[0].ConfidenceOverall != want {
		t.Errorf("confidence overall = %v, want %v", results[0].ConfidenceOverall, want)
	}
}

func TestIntelDeriveExternalKey_ApplicationURL(t *testing.T) {
	ext := extractor.Extraction{
		"application_url": {Value: "https://apply.example.org/s1", Confidence: 0.9, Evidence: strPtr("e")},
		"title":           {Value: "Title", Confidence: 0.9, Evidence: strPtr("e")},
	}
	key := deriveExternalKey("https://source.example.org", ext, 0)
	want := "https://source.example.org|https://apply.example.org/s1"

	if key != want {
		t.Errorf("deriveExternalKey = %q, want %q", key, want)
	}
}

func TestIntelDeriveExternalKey_TitleDeadlineHash(t *testing.T) {
	ext := extractor.Extraction{
		"title":    {Value: "Title", Confidence: 0.9, Evidence: strPtr("e")},
		"deadline": {Value: "2027-03-15", Confidence: 0.8, Evidence: strPtr("e")},
	}

	key := deriveExternalKey("https://source.example.org", ext, 0)
	if !strings.HasPrefix(key, "https://source.example.org|") {
		t.Errorf("expected source URL prefix, got %q", key)
	}
	// Same inputs must produce the same key.
	key2 := deriveExternalKey("https://source.example.org", ext, 0)
	if key != key2 {
		t.Errorf("external key not stable: %q vs %q", key, key2)
	}
}

func TestIntelDeriveExternalKey_StableFallback(t *testing.T) {
	ext := extractor.Extraction{
		"country": {Value: []any{"USA"}, Confidence: 0.9, Evidence: strPtr("e")},
		"amount":  {Value: 1000.0, Confidence: 0.8, Evidence: strPtr("e")},
	}
	key1 := deriveExternalKey("https://source.example.org", ext, 0)
	key2 := deriveExternalKey("https://source.example.org", ext, 1)

	if key1 != key2 {
		t.Errorf("fallback key depends on index: %q vs %q", key1, key2)
	}

	if !strings.HasPrefix(key1, "https://source.example.org|") {
		t.Errorf("expected source URL prefix, got %q", key1)
	}
}

func TestIntelRecordsToResults_RawExtractionIsValidJSON(t *testing.T) {
	adapter := &intelExtractorPortAdapter{}
	records := []extractor.Extraction{
		{
			"title": {Value: "T", Confidence: 0.9, Evidence: strPtr("e")},
		},
	}

	results, err := adapter.recordsToResults("https://example.org", records)
	if err != nil {
		t.Fatalf("recordsToResults failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	var parsed map[string]any
	if err := json.Unmarshal(results[0].RawExtraction, &parsed); err != nil {
		t.Errorf("RawExtraction is not valid JSON: %v", err)
	}
}

func strPtr(s string) *string { return &s }

// stubPromoter returns a fixed error so the adapter's error mapping can be
// exercised without a store.
type stubPromoter struct{ err error }

func (s stubPromoter) Promote(_ context.Context, _ uuid.UUID) (promoter.PromoteResult, error) {
	return promoter.PromoteResult{}, s.err
}

func (s stubPromoter) Entity() string { return "wiki_page" }

func TestIntelPromoterPortAdapter_MapsRouteToReview(t *testing.T) {
	inner := fmt.Errorf("promoter: wiki page requires a resolvable geo space but none matched: %w", promoter.ErrRouteToReview)
	a := &intelPromoterPortAdapter{prom: stubPromoter{err: inner}}

	_, err := a.Promote(t.Context(), uuid.New())

	var rte *worker.RouteToReviewError
	if !errors.As(err, &rte) {
		t.Fatalf("err = %T %v, want *worker.RouteToReviewError", err, err)
	}
	if !errors.Is(err, promoter.ErrRouteToReview) {
		t.Error("wrapped reason should still match promoter.ErrRouteToReview")
	}
	if !strings.Contains(err.Error(), "resolvable geo space") {
		t.Errorf("detail lost in mapping: %v", err)
	}
}

func TestIntelPromoterPortAdapter_PassesThroughOtherErrors(t *testing.T) {
	a := &intelPromoterPortAdapter{prom: stubPromoter{err: errors.New("db down")}}

	_, err := a.Promote(t.Context(), uuid.New())

	var rte *worker.RouteToReviewError
	if errors.As(err, &rte) {
		t.Fatalf("infra error must not map to RouteToReviewError: %v", err)
	}
	if err == nil || err.Error() != "db down" {
		t.Errorf("err = %v, want passthrough 'db down'", err)
	}
}
