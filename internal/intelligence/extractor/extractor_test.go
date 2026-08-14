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

func (m *mockGateway) ChatWithTools(ctx context.Context, category string, req ai.ChatWithToolsRequest) (ai.ChatWithToolsResponse, error) {
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

func (m *failingThenSuccessGateway) ChatWithTools(ctx context.Context, category string, req ai.ChatWithToolsRequest) (ai.ChatWithToolsResponse, error) {
	m.calls++
	if m.calls <= m.failures {
		return ai.ChatWithToolsResponse{}, errors.New("transient failure")
	}
	return m.resp, nil
}

func TestExtract_Success(t *testing.T) {
	body := `{"title":{"value":"DAAD EPOS","confidence":0.95,"evidence":"DAAD EPOS programme"},"country":{"value":["DEU"],"confidence":0.9,"evidence":"Germany"}}`
	gw := &mockGateway{resp: ai.ChatWithToolsResponse{Content: body}}
	ex := NewExtractor(gw)
	res, err := ex.Extract(context.Background(), schemas.Scholarship, "https://example.com", "html", "source text here")
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
	good := `{"title":{"value":"X","confidence":0.9,"evidence":"e"},"country":{"value":["USA"],"confidence":0.9,"evidence":"e"}}`
	gw := &failingThenSuccessGateway{failures: 1, resp: ai.ChatWithToolsResponse{Content: good}}
	ex := NewExtractor(gw, WithMaxRetries(2))
	_, err := ex.Extract(context.Background(), schemas.Scholarship, "https://example.com", "html", "text")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}
}

func TestExtract_RetriesOnBadJSON(t *testing.T) {
	// Gateway always returns invalid JSON -> should exhaust retries and error.
	gw := &mockGateway{resp: ai.ChatWithToolsResponse{Content: `{not json`}}
	ex := NewExtractor(gw, WithMaxRetries(1))
	_, err := ex.Extract(context.Background(), schemas.Scholarship, "https://example.com", "html", "text")
	if err == nil {
		t.Error("expected error after exhausting retries on bad JSON")
	}
}

func TestExtract_GatewayError(t *testing.T) {
	gw := &mockGateway{err: errors.New("all providers failed")}
	ex := NewExtractor(gw, WithMaxRetries(1))
	_, err := ex.Extract(context.Background(), schemas.Scholarship, "https://example.com", "html", "text")
	if err == nil {
		t.Error("expected error when gateway fails")
	}
}

func TestExtract_StripsCodeFences(t *testing.T) {
	body := "```json\n{\"title\":{\"value\":\"X\",\"confidence\":0.9,\"evidence\":\"e\"},\"country\":{\"value\":[\"USA\"],\"confidence\":0.9,\"evidence\":\"e\"}}\n```"
	gw := &mockGateway{resp: ai.ChatWithToolsResponse{Content: body}}
	ex := NewExtractor(gw)
	res, err := ex.Extract(context.Background(), schemas.Scholarship, "https://example.com", "html", "text")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}
	if res.Extraction["title"].Value != "X" {
		t.Errorf("title not extracted after fence strip: %v", res.Extraction["title"].Value)
	}
}

func TestExtract_PromptContainsSchemaAndSource(t *testing.T) {
	body := `{"title":{"value":"X","confidence":0.9,"evidence":"e"},"country":{"value":["USA"],"confidence":0.9,"evidence":"e"}}`
	gw := &mockGateway{resp: ai.ChatWithToolsResponse{Content: body}}
	ex := NewExtractor(gw)
	_, err := ex.Extract(context.Background(), schemas.Scholarship, "https://example.com", "html", "UNIQUE_SOURCE_TEXT")
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
