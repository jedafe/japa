package ai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// --- fake stores ---------------------------------------------------------

type fakeByokProviders struct {
	providers []ByokProvider
	clicks    []pgtype.UUID
	err       error
}

func (f *fakeByokProviders) ListEnabled(context.Context) ([]ByokProvider, error) {
	return f.providers, f.err
}

func (f *fakeByokProviders) Get(_ context.Context, id pgtype.UUID) (ByokProvider, error) {
	for _, p := range f.providers {
		if p.ID == id {
			return p, nil
		}
	}
	return ByokProvider{}, ErrByokProviderNotFound
}

func (f *fakeByokProviders) IncrementReferralClicks(_ context.Context, id pgtype.UUID) error {
	f.clicks = append(f.clicks, id)
	return nil
}

type fakeUserKeys struct {
	keys       []UserAIKey
	resolved   Provider
	resolveErr error
	marked     [][2]string // [status, lastError]
	createErr  error
}

func (f *fakeUserKeys) ListForUser(_ context.Context, _ pgtype.UUID) ([]UserAIKey, error) {
	return f.keys, nil
}

func (f *fakeUserKeys) GetForUser(_ context.Context, _, keyID pgtype.UUID) (UserAIKey, error) {
	for _, k := range f.keys {
		if k.ID == keyID {
			return k, nil
		}
	}
	return UserAIKey{}, ErrAIKeyNotFound
}

func (f *fakeUserKeys) Create(_ context.Context, _ pgtype.UUID, providerID pgtype.UUID, _, label string) (UserAIKey, error) {
	if f.createErr != nil {
		return UserAIKey{}, f.createErr
	}
	var id pgtype.UUID
	_ = id.Scan("99999999-9999-9999-9999-999999999999")

	return UserAIKey{
		ID:             id,
		ByokProviderID: providerID,
		Label:          label,
		Status:         "active",
	}, nil
}

func (f *fakeUserKeys) Update(_ context.Context, _, keyID pgtype.UUID, _, _ *string) (UserAIKey, error) {
	for _, k := range f.keys {
		if k.ID == keyID {
			return k, nil
		}
	}
	return UserAIKey{}, ErrAIKeyNotFound
}

func (f *fakeUserKeys) Delete(_ context.Context, _, keyID pgtype.UUID) error {
	for _, k := range f.keys {
		if k.ID == keyID {
			return nil
		}
	}
	return ErrAIKeyNotFound
}

func (f *fakeUserKeys) MarkStatus(_ context.Context, _ pgtype.UUID, status, lastError string) error {
	f.marked = append(f.marked, [2]string{status, lastError})
	return nil
}

func (f *fakeUserKeys) ResolveProvider(_ context.Context, _, _ pgtype.UUID, _ string) (Provider, error) {
	if f.resolveErr != nil {
		return nil, f.resolveErr
	}
	return f.resolved, nil
}

// fetchModelsProvider is a minimal Provider whose FetchModels outcome the
// test controls.
type fetchModelsProvider struct{ err error }

func (p *fetchModelsProvider) Name() string        { return "fake" }
func (p *fetchModelsProvider) Models() []ModelInfo { return nil }
func (p *fetchModelsProvider) Send(context.Context, Request) (Response, error) {
	return Response{}, errors.New("unused")
}
func (p *fetchModelsProvider) ChatWithTools(context.Context, ChatWithToolsRequest) (ChatWithToolsResponse, error) {
	return ChatWithToolsResponse{}, errors.New("unused")
}
func (p *fetchModelsProvider) VerifyWebhookSignature([]byte, string) error { return nil }
func (p *fetchModelsProvider) FetchModels(context.Context) ([]ModelInfo, error) {
	return nil, p.err
}

// withIDParam injects chi's {id} route param into the request context.
func withIDParam(r *http.Request, id string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// --- tests ---------------------------------------------------------------

func TestHandleListByokProvidersMergesKeys(t *testing.T) {
	t.Parallel()

	pid := mustUUID(t, "11111111-1111-1111-1111-111111111111")
	kid := mustUUID(t, "22222222-2222-2222-2222-222222222222")

	h := &Handlers{}
	h.SetByokStores(
		&fakeByokProviders{providers: []ByokProvider{
			{ID: pid, ProviderType: "openai", DisplayName: "OpenAI", Enabled: true},
			{ID: mustUUID(t, "33333333-3333-3333-3333-333333333333"), ProviderType: "groq", DisplayName: "Groq", Enabled: true},
		}},
		&fakeUserKeys{keys: []UserAIKey{
			{ID: kid, ByokProviderID: pid, Label: "main", APIKeyMasked: "sk-****1234", Status: "active"},
		}},
	)

	r := authedRequest(t, http.MethodGet, "/api/v1/ai/byok/providers", "", 1)
	w := httptest.NewRecorder()
	h.HandleListByokProviders(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{`"has_key":true`, `"api_key_masked":"sk-****1234"`, "OpenAI", "Groq", `"has_key":false`} {
		if !strings.Contains(body, want) {
			t.Errorf("merged payload missing %q: %s", want, body)
		}
	}
}

func TestHandleCreateMyAIKeyValidation(t *testing.T) {
	t.Parallel()

	h := &Handlers{}
	h.SetByokStores(&fakeByokProviders{}, &fakeUserKeys{})

	// Missing api_key → 400.
	r := authedRequest(t, http.MethodPost, "/api/v1/me/ai-keys",
		`{"byok_provider_id":"11111111-1111-1111-1111-111111111111"}`, 1)
	w := httptest.NewRecorder()
	h.HandleCreateMyAIKey(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("empty api_key: status %d, want 400", w.Code)
	}

	// Unknown provider → 404 via the sentinel.
	h.SetByokStores(&fakeByokProviders{}, &fakeUserKeys{createErr: ErrByokProviderNotFound})
	r = authedRequest(t, http.MethodPost, "/api/v1/me/ai-keys",
		`{"byok_provider_id":"11111111-1111-1111-1111-111111111111","api_key":"sk-test"}`, 1)
	w = httptest.NewRecorder()
	h.HandleCreateMyAIKey(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("unknown provider: status %d, want 404", w.Code)
	}
}

func TestHandleDeleteMyAIKeyNotFound(t *testing.T) {
	t.Parallel()

	h := &Handlers{}
	h.SetByokStores(&fakeByokProviders{}, &fakeUserKeys{})

	r := authedRequest(t, http.MethodDelete, "/api/v1/me/ai-keys/x", "", 1)
	r = withIDParam(r, "33333333-3333-3333-3333-333333333333")
	w := httptest.NewRecorder()
	h.HandleDeleteMyAIKey(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("foreign/unknown id: status %d, want 404", w.Code)
	}
}

func TestHandleVerifyMyAIKeyAuthErrorMarksInvalid(t *testing.T) {
	t.Parallel()

	kid := mustUUID(t, "22222222-2222-2222-2222-222222222222")
	uk := &fakeUserKeys{
		keys:     []UserAIKey{{ID: kid, Status: "active"}},
		resolved: &fetchModelsProvider{err: &HTTPStatusError{Provider: "openai", Status: 401}},
	}
	h := &Handlers{}
	h.SetByokStores(&fakeByokProviders{}, uk)

	r := authedRequest(t, http.MethodPost, "/api/v1/me/ai-keys/x/verify", "", 1)
	r = withIDParam(r, kid.String())
	w := httptest.NewRecorder()
	h.HandleVerifyMyAIKey(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if len(uk.marked) != 1 || uk.marked[0][0] != "invalid" {
		t.Errorf("expected key marked invalid, got %+v", uk.marked)
	}
}

func TestHandleVerifyMyAIKeyTransientError(t *testing.T) {
	t.Parallel()

	kid := mustUUID(t, "22222222-2222-2222-2222-222222222222")
	uk := &fakeUserKeys{
		keys:     []UserAIKey{{ID: kid, Status: "active"}},
		resolved: &fetchModelsProvider{err: &HTTPStatusError{Provider: "openai", Status: 429}},
	}
	h := &Handlers{}
	h.SetByokStores(&fakeByokProviders{}, uk)

	r := authedRequest(t, http.MethodPost, "/api/v1/me/ai-keys/x/verify", "", 1)
	r = withIDParam(r, kid.String())
	w := httptest.NewRecorder()
	h.HandleVerifyMyAIKey(w, r)

	if w.Code != http.StatusBadGateway {
		t.Errorf("transient failure: status %d, want 502", w.Code)
	}
	if len(uk.marked) != 0 {
		t.Errorf("transient failure must not touch status, got %+v", uk.marked)
	}
}
