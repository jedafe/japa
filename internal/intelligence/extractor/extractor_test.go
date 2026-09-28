package extractor

import (
	"context"
	"errors"
	"testing"

	"github.com/jedafe/japa/internal/ai"
	"github.com/jedafe/japa/internal/intelligence/extractor/schemas"
)

// mockGateway is a test double for the AIGateway interface.
type mockGateway struct {
	resp    ai.ChatWithToolsResponse
	err     error
	calls   int
	lastReq ai.ChatWithToolsRequest
}

func (m *mockGateway) ChatWithTools(_ context.Context, _ string, req ai.ChatWithToolsRequest) (ai.ChatWithToolsResponse, error) {
	m.calls++

	m.lastReq = req
	if m.err != nil {
		return ai.ChatWithToolsResponse{}, m.err
	}

	return m.resp, nil
}

// failingThenSuccessGateway fails the first N calls then succeeds.
type failingThenSuccessGateway struct {
	failures int
	resp     ai.ChatWithToolsResponse
	calls    int
}

func (m *failingThenSuccessGateway) ChatWithTools(_ context.Context, _ string, _ ai.ChatWithToolsRequest) (ai.ChatWithToolsResponse, error) {
	m.calls++
	if m.calls <= m.failures {
		return ai.ChatWithToolsResponse{}, errors.New("transient failure")
	}

	return m.resp, nil
}

func TestExtract_Success(t *testing.T) {
	t.Parallel()

	body := `{"title":{"value":"DAAD EPOS","confidence":0.95,"evidence":"DAAD EPOS programme"},"country":{"value":["DEU"],"confidence":0.9,"evidence":"Germany"}}`
	gw := &mockGateway{resp: ai.ChatWithToolsResponse{Content: body}}
	ex := NewExtractor(gw)

	res, err := ex.Extract(t.Context(), schemas.Scholarship, "https://example.com", "html", "source text here")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	if res.Extraction["title"].Value != "DAAD EPOS" {
		t.Errorf("title not extracted: %v", res.Extraction["title"].Value)
	}

	if gw.calls != 1 {
		t.Errorf("expected 1 gateway call, got %d", gw.calls)
	}
}

func TestExtract_RetriesOnMalformedJSON(t *testing.T) {
	t.Parallel()

	good := `{"title":{"value":"X","confidence":0.9,"evidence":"e"},"country":{"value":["USA"],"confidence":0.9,"evidence":"e"}}`
	gw := &failingThenSuccessGateway{failures: 1, resp: ai.ChatWithToolsResponse{Content: good}}
	ex := NewExtractor(gw, WithMaxRetries(2))

	_, err := ex.Extract(t.Context(), schemas.Scholarship, "https://example.com", "html", "text")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}
}

func TestExtract_RetriesOnBadJSON(t *testing.T) {
	t.Parallel()
	// Gateway always returns invalid JSON -> should exhaust retries and error.
	gw := &mockGateway{resp: ai.ChatWithToolsResponse{Content: `{not json`}}
	ex := NewExtractor(gw, WithMaxRetries(1))

	_, err := ex.Extract(t.Context(), schemas.Scholarship, "https://example.com", "html", "text")
	if err == nil {
		t.Error("expected error after exhausting retries on bad JSON")
	}
}

func TestExtract_GatewayError(t *testing.T) {
	t.Parallel()

	gw := &mockGateway{err: errors.New("all providers failed")}
	ex := NewExtractor(gw, WithMaxRetries(1))

	_, err := ex.Extract(t.Context(), schemas.Scholarship, "https://example.com", "html", "text")
	if err == nil {
		t.Error("expected error when gateway fails")
	}
}

func TestExtract_StripsCodeFences(t *testing.T) {
	t.Parallel()

	body := "```json\n{\"title\":{\"value\":\"X\",\"confidence\":0.9,\"evidence\":\"e\"},\"country\":{\"value\":[\"USA\"],\"confidence\":0.9,\"evidence\":\"e\"}}\n```"
	gw := &mockGateway{resp: ai.ChatWithToolsResponse{Content: body}}
	ex := NewExtractor(gw)

	res, err := ex.Extract(t.Context(), schemas.Scholarship, "https://example.com", "html", "text")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	if res.Extraction["title"].Value != "X" {
		t.Errorf("title not extracted after fence strip: %v", res.Extraction["title"].Value)
	}
}

func TestExtract_RecordsArray(t *testing.T) {
	t.Parallel()
	// Multi-record output from the LLM.
	body := `{"records":[{"title":{"value":"A","confidence":0.9,"evidence":"e"},"country":{"value":["USA"],"confidence":0.9,"evidence":"e"}},{"title":{"value":"B","confidence":0.8,"evidence":"e"},"country":{"value":["CAN"],"confidence":0.8,"evidence":"e"}}]}`
	gw := &mockGateway{resp: ai.ChatWithToolsResponse{Content: body}}
	ex := NewExtractor(gw)

	res, err := ex.Extract(t.Context(), schemas.Scholarship, "https://example.com", "html", "two scholarships")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	if len(res.Records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(res.Records))
	}

	if res.Records[0]["title"].Value != "A" {
		t.Errorf("record 0 title = %v, want A", res.Records[0]["title"].Value)
	}

	if res.Records[1]["title"].Value != "B" {
		t.Errorf("record 1 title = %v, want B", res.Records[1]["title"].Value)
	}

	if res.Extraction["title"].Value != "A" {
		t.Errorf("fallback Extraction title = %v, want A", res.Extraction["title"].Value)
	}
}

func TestConfidenceOverall(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		ext  Extraction
		want float32
	}{
		{"empty", Extraction{}, 0},
		{"all null", Extraction{"title": {Value: nil, Confidence: 0}, "country": {Value: nil, Confidence: 0}}, 0},
		{"all null with conf 1 must score 0", Extraction{
			"title":   {Value: nil, Confidence: 1},
			"country": {Value: nil, Confidence: 1},
		}, 0},
		{"null conf 1 does not inflate mean", Extraction{
			"title":    {Value: "A", Confidence: 0.6},
			"country":  {Value: nil, Confidence: 1},
			"deadline": {Value: nil, Confidence: 0},
		}, 0.6},
		{"mixed", Extraction{
			"title":    {Value: "A", Confidence: 0.9},
			"country":  {Value: nil, Confidence: 0},
			"deadline": {Value: "2027-01-01", Confidence: 0.7},
		}, (0.9 + 0.7) / 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := ConfidenceOverall(tc.ext)
			if got != tc.want {
				t.Errorf("ConfidenceOverall = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseExtraction_RecordsArray(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"records":[{"title":{"value":"X","confidence":0.9,"evidence":"e"}}]}`)

	ext, err := ParseExtraction(raw)
	if err != nil {
		t.Fatalf("ParseExtraction failed: %v", err)
	}

	records := splitRecords(ext)
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	if records[0]["title"].Value != "X" {
		t.Errorf("record title = %v, want X", records[0]["title"].Value)
	}
}

func TestSplitRecords_PreservesNullField(t *testing.T) {
	t.Parallel()

	ext := Extraction{
		"records": {Value: []any{
			map[string]any{"title": map[string]any{"value": "A", "confidence": 0.9, "evidence": "e"}, "country": nil},
		}},
	}

	records := splitRecords(ext)
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	if records[0]["title"].Value != "A" {
		t.Errorf("record title = %v, want A", records[0]["title"].Value)
	}

	if _, ok := records[0]["country"]; !ok {
		t.Error("expected null country field to be preserved")
	}
}

func TestExtract_PromptContainsSchemaAndSource(t *testing.T) {
	t.Parallel()

	body := `{"title":{"value":"X","confidence":0.9,"evidence":"e"},"country":{"value":["USA"],"confidence":0.9,"evidence":"e"}}`
	gw := &mockGateway{resp: ai.ChatWithToolsResponse{Content: body}}
	ex := NewExtractor(gw)

	_, err := ex.Extract(t.Context(), schemas.Scholarship, "https://example.com", "html", "UNIQUE_SOURCE_TEXT")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	if gw.lastReq.Messages[0].Content == "" {
		t.Error("prompt was empty")
	}

	if !contains(gw.lastReq.Messages[0].Content, "UNIQUE_SOURCE_TEXT") {
		t.Error("prompt did not contain source text")
	}

	if !contains(gw.lastReq.Messages[0].Content, "scholarship") {
		t.Error("prompt did not contain schema entity type")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}

	return -1
}
