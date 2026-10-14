package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
)

func unifiedHTTPHandlers(t *testing.T, provider *recordingProvider, perms *stubPerms) *AgentHTTPHandlers {
	t.Helper()
	agent, _, _ := newUnifiedHandler(t, provider, perms)
	return NewAgentHTTPHandlers(agent, newStubAgentSessions())
}

func authedRequest(t *testing.T, method, path, body string, uid byte) *http.Request {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	user := &auth.AuthenticatedUser{UserID: pgtype.UUID{Bytes: [16]byte{uid}, Valid: true}}
	return r.WithContext(context.WithValue(r.Context(), auth.UserContextKey, user))
}

func TestHandleUnifiedChat_JSONContract(t *testing.T) {
	provider := &recordingProvider{name: "mock", reply: "hello back"}
	hh := unifiedHTTPHandlers(t, provider, &stubPerms{granted: map[string]bool{"ai.context.use": true}})

	r := authedRequest(t, http.MethodPost, "/api/v1/ai/chat", `{"message":"hi"}`, 1)
	w := httptest.NewRecorder()
	hh.HandleUnifiedChat(w, r)

	res := w.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d: %s", res.StatusCode, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{`"reply":"hello back"`, `"conversation_id"`, `"message_id"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("JSON contract missing %s: %s", want, body)
		}
	}
}

func TestHandleUnifiedChat_SSEStream(t *testing.T) {
	provider := &recordingProvider{name: "mock", reply: "streamed"}
	hh := unifiedHTTPHandlers(t, provider, &stubPerms{granted: map[string]bool{"ai.context.use": true}})

	r := authedRequest(t, http.MethodPost, "/api/v1/ai/chat", `{"message":"hi"}`, 1)
	r.Header.Set("Accept", "text/event-stream")
	w := httptest.NewRecorder()
	hh.HandleUnifiedChat(w, r)

	res := w.Result()
	if ct := res.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("expected SSE content type, got %s", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "data: ") || !strings.Contains(body, `"type":"session_start"`) || !strings.Contains(body, `"type":"final_response"`) || !strings.Contains(body, `"type":"conversation"`) {
		t.Fatalf("bad SSE body: %s", body)
	}
}

func TestHandleUnifiedChat_EmptyMessage(t *testing.T) {
	hh := unifiedHTTPHandlers(t, &recordingProvider{name: "mock"}, &stubPerms{granted: map[string]bool{"ai.context.use": true}})
	r := authedRequest(t, http.MethodPost, "/api/v1/ai/chat", `{}`, 1)
	w := httptest.NewRecorder()
	hh.HandleUnifiedChat(w, r)
	if w.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Result().StatusCode)
	}
}

func TestHandleUnifiedChat_NoPermission(t *testing.T) {
	hh := unifiedHTTPHandlers(t, &recordingProvider{name: "mock"}, &stubPerms{granted: map[string]bool{}})
	r := authedRequest(t, http.MethodPost, "/api/v1/ai/chat", `{"message":"hi"}`, 1)
	w := httptest.NewRecorder()
	hh.HandleUnifiedChat(w, r)
	res := w.Result()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", res.StatusCode)
	}
	if code := decodeError(t, w.Body)["code"]; code != "ai_permission_required" {
		t.Fatalf("expected ai_permission_required, got %s", code)
	}
}

func TestAIPrefsHandlers(t *testing.T) {
	h := &Handlers{Convos: NewConversationService(newStubConvoStore(), nil)}

	// GET default → empty object.
	r := authedRequest(t, http.MethodGet, "/api/v1/me/ai-preferences", "", 1)
	w := httptest.NewRecorder()
	h.HandleGetAIPrefs(w, r)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("GET status %d", w.Result().StatusCode)
	}

	// PUT valid.
	r = authedRequest(t, http.MethodPut, "/api/v1/me/ai-preferences",
		`{"default_mode":"agent","disabled_categories":["forum"]}`, 1)
	w = httptest.NewRecorder()
	h.HandlePutAIPrefs(w, r)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("PUT status %d: %s", w.Result().StatusCode, w.Body.String())
	}

	// PUT invalid default_mode → 400.
	r = authedRequest(t, http.MethodPut, "/api/v1/me/ai-preferences", `{"default_mode":"turbo"}`, 1)
	w = httptest.NewRecorder()
	h.HandlePutAIPrefs(w, r)
	if w.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("bad mode should 400, got %d", w.Result().StatusCode)
	}

	// GET returns what was saved.
	r = authedRequest(t, http.MethodGet, "/api/v1/me/ai-preferences", "", 1)
	w = httptest.NewRecorder()
	h.HandleGetAIPrefs(w, r)
	if !strings.Contains(w.Body.String(), `"default_mode":"agent"`) {
		t.Fatalf("prefs not persisted: %s", w.Body.String())
	}
}
