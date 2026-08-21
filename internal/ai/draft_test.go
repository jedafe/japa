package ai

import (
	"bytes"
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
	user, tenant := testUserAndTenant()
	body, _ := json.Marshal(map[string]any{
		"content_type": "blog_post",
		"title":        "My journey",
	})
	req := httptest.NewRequest(http.MethodPost, "/ai/assist/draft", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withAuthContext(req, user, tenant)
	w := httptest.NewRecorder()

	h.HandleDraft(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}
