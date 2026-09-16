package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleDraft_MissingPermission(t *testing.T) {
	t.Parallel()

	h := &Handlers{}
	req := httptest.NewRequest(http.MethodPost, "/ai/assist/draft", bytes.NewReader([]byte(`{"content_type":"blog_post","title":"x"}`)))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()

	h.HandleDraft(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleDraft_ValidBlogPost(t *testing.T) {
	t.Parallel()

	disp := NewDispatcher()
	disp.RegisterProvider("content_generation", &mockProvider{})
	h := &Handlers{Dispatcher: disp}
	user, platform := testUserAndPlatform()
	body, _ := json.Marshal(map[string]any{
		"content_type": "blog_post",
		"title":        "My journey",
	})
	req := httptest.NewRequest(http.MethodPost, "/ai/assist/draft", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withAuthContext(req, user, platform)
	w := httptest.NewRecorder()

	h.HandleDraft(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleDraft_ReturnsBodyAndTitle(t *testing.T) {
	t.Parallel()

	provider := &mockProvider{
		name: "mock",
		sendFn: func(_ context.Context, _ Request) (Response, error) {
			return Response{
				ProviderRef: "mock-ref",
				Status:      "success",
				Raw: map[string]any{
					"model": "mock-model",
					"usage": map[string]any{"total_tokens": 42.0},
					"choices": []any{
						map[string]any{
							"message": map[string]any{
								"content": `{"title":"Final Title","body_html":"<p>Hello</p>","field_values":{}}`,
							},
						},
					},
				},
			}, nil
		},
	}
	disp := NewDispatcher()
	disp.RegisterProvider("content_generation", provider)
	h := &Handlers{Dispatcher: disp}

	user, platform := testUserAndPlatform()
	body, _ := json.Marshal(map[string]any{
		"content_type": "blog_post",
		"title":        "Draft Title",
		"tone":         "friendly",
	})
	req := httptest.NewRequest(http.MethodPost, "/ai/assist/draft", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withAuthContext(req, user, platform)
	w := httptest.NewRecorder()

	h.HandleDraft(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Data DraftResult `json:"data"`
	}

	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Data.Title != "Final Title" {
		t.Errorf("title = %q, want %q", resp.Data.Title, "Final Title")
	}

	if resp.Data.BodyHTML != "<p>Hello</p>" {
		t.Errorf("body_html = %q, want %q", resp.Data.BodyHTML, "<p>Hello</p>")
	}

	if resp.Data.TokensUsed != 42 {
		t.Errorf("tokens_used = %d, want 42", resp.Data.TokensUsed)
	}
}

func TestHandleDraft_MissingContentType(t *testing.T) {
	t.Parallel()

	h := &Handlers{}
	user, platform := testUserAndPlatform()
	req := httptest.NewRequest(http.MethodPost, "/ai/assist/draft", bytes.NewReader([]byte(`{"title":"x"}`)))
	req.Header.Set("Content-Type", "application/json")
	req = withAuthContext(req, user, platform)
	w := httptest.NewRecorder()

	h.HandleDraft(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleDraft_HumanizeOverride(t *testing.T) {
	t.Parallel()

	var got map[string]any

	provider := &mockProvider{
		name: "mock",
		sendFn: func(_ context.Context, req Request) (Response, error) {
			got = req.Payload
			return Response{Raw: map[string]any{
				"choices": []any{map[string]any{"message": map[string]any{"content": `{"body_html":"<p>x</p>"}`}}},
			}}, nil
		},
	}
	disp := NewDispatcher()
	disp.RegisterProvider("content_generation", provider)
	h := &Handlers{Dispatcher: disp}
	user, platform := testUserAndPlatform()
	body, _ := json.Marshal(map[string]any{
		"content_type":   "blog_post",
		"title":          "t",
		"humanize":       true,
		"humanize_level": "heavy",
	})
	req := httptest.NewRequest(http.MethodPost, "/ai/assist/draft", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withAuthContext(req, user, platform)
	w := httptest.NewRecorder()

	h.HandleDraft(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	if got["humanize"] != true || got["humanize_level"] != "heavy" {
		t.Errorf("humanize payload = %v", got)
	}
}
