package mcp

// Tests for the §119 v1.5 hardening pass: public-read narrowing for
// read-only key/OAuth callers, batch element errors, DCR scope handling,
// and PKCE format enforcement.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/ai"
)

// registerContentTools stubs content.search/content.read with one public and
// one non-public row so the visibility narrowing can be exercised.
func registerContentTools(deps *testDeps) {
	deps.registry.RegisterConfig("content.search", ai.ToolConfig{
		FunctionName: "content.search",
		Parameters:   map[string]any{"type": "object"},
	}, "", pgtype.UUID{})
	deps.registry.RegisterHandler("content.search", func(_ context.Context, _ json.RawMessage, _ pgtype.UUID) (*ai.ToolResult, error) {
		return &ai.ToolResult{
			Status: ai.ToolCallStatusSuccess,
			Content: `{"results":[
				{"id":"c1","title":"Public post","slug":"public-post","status":"published","visibility":"public"},
				{"id":"c2","title":"Draft post","slug":"draft-post","status":"draft","visibility":"public"},
				{"id":"c3","title":"Members post","slug":"members-post","status":"published","visibility":"members"},
				{"id":"c4","title":"Private post","slug":"private-post","status":"published","visibility":"private"}
			],"total":4,"geo_scoped":false}`,
		}, nil
	})

	deps.registry.RegisterConfig("content.read", ai.ToolConfig{
		FunctionName: "content.read",
		Parameters:   map[string]any{"type": "object"},
	}, "", pgtype.UUID{})
	deps.registry.RegisterHandler("content.read", func(_ context.Context, args json.RawMessage, _ pgtype.UUID) (*ai.ToolResult, error) {
		var a struct {
			ContentID string `json:"content_id"`
		}
		_ = json.Unmarshal(args, &a)

		status, visibility := "published", "public"
		if a.ContentID == "c-private" {
			visibility = "private"
		}

		return &ai.ToolResult{
			Status: ai.ToolCallStatusSuccess,
			Content: `{"content":{"id":"` + a.ContentID + `","title":"T","slug":"t","status":"` + status +
				`","visibility":"` + visibility + `","body":"body"}}`,
		}, nil
	})
}

// Read-only key callers must not receive drafts or non-public rows — the
// §103 internal visibility would otherwise leak them through the read gate.
func TestPublicReadFilter_SearchDropsNonPublic(t *testing.T) {
	srv, deps := newTestMCP(t)
	registerContentTools(deps)

	ctx := apiKeyCtx(withUser(t.Context(), testUserID), "agent.use", "api.read.content")

	_, out := postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": {"name": "content.search", "arguments": {"query": "post"}}
	}`)
	if out["error"] != nil {
		t.Fatalf("call failed: %v", out["error"])
	}

	text := out["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)

	var payload struct {
		Results []map[string]any `json:"results"`
		Total   int              `json:"total"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("unmarshal result: %v — %s", err, text)
	}

	if payload.Total != 1 || len(payload.Results) != 1 || payload.Results[0]["id"] != "c1" {
		t.Fatalf("expected only the published+public row, got %v", payload)
	}
}

// content.read on a private row must answer not-found — indistinguishable
// from a missing row.
func TestPublicReadFilter_ReadDeniesPrivate(t *testing.T) {
	srv, deps := newTestMCP(t)
	registerContentTools(deps)

	ctx := apiKeyCtx(withUser(t.Context(), testUserID), "agent.use", "api.read.content")

	_, out := postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": {"name": "content.read", "arguments": {"content_id": "c-private"}}
	}`)
	if out["error"] != nil {
		t.Fatalf("call failed: %v", out["error"])
	}

	res := out["result"].(map[string]any)
	if res["isError"] != true {
		t.Fatalf("expected isError for private row, got %v", res)
	}

	text := res["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "content_not_found") {
		t.Fatalf("expected content_not_found, got %s", text)
	}
}

// The fetch alias is narrowed through the same filter — a private row must
// surface as an error, not reshaped content.
func TestPublicReadFilter_FetchAliasDeniesPrivate(t *testing.T) {
	srv, deps := newTestMCP(t)
	registerContentTools(deps)

	ctx := apiKeyCtx(withUser(t.Context(), testUserID), "agent.use", "api.read.content")

	_, out := postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": {"name": "fetch", "arguments": {"id": "c-private"}}
	}`)

	res := out["result"].(map[string]any)
	if res["isError"] != true {
		t.Fatalf("expected isError for private fetch, got %v", res)
	}
}

// JWT callers keep the full §103 internal view — the narrowing applies to
// the key/OAuth gate population only.
func TestPublicReadFilter_JWTUnaffected(t *testing.T) {
	srv, deps := newTestMCP(t)
	registerContentTools(deps)

	ctx := withUser(t.Context(), testUserID)

	_, out := postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": {"name": "content.read", "arguments": {"content_id": "c-private"}}
	}`)

	res := out["result"].(map[string]any)
	if res["isError"] == true {
		t.Fatalf("JWT caller lost internal visibility: %v", res)
	}

	text := res["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, `"visibility":"private"`) {
		t.Fatalf("expected private row for JWT caller, got %s", text)
	}
}

// An elevated agent.write key keeps the internal view — narrowing is for
// the read-only gate only.
func TestPublicReadFilter_AgentWriteKeyUnaffected(t *testing.T) {
	srv, deps := newTestMCP(t)
	registerContentTools(deps)

	ctx := apiKeyCtx(withUser(t.Context(), testUserID), "agent.use", "agent.write")

	_, out := postJSON(t, srv, ctx, `{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": {"name": "content.read", "arguments": {"content_id": "c-private"}}
	}`)

	res := out["result"].(map[string]any)
	if res["isError"] == true {
		t.Fatalf("agent.write key narrowed unexpectedly: %v", res)
	}
}

// Malformed batch elements must get their own -32600 entry, not sink the
// batch or pass silently as notifications.
func TestMCP_BatchMalformedElements(t *testing.T) {
	srv, _ := newTestMCP(t)
	ctx := withUser(t.Context(), testUserID)

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(
		`[1, "x", {"jsonrpc":"2.0","id":9,"method":"ping"}]`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var out []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal batch: %v", err)
	}

	if len(out) != 3 {
		t.Fatalf("expected 3 responses, got %v", out)
	}

	for i := 0; i < 2; i++ {
		e := out[i]["error"].(map[string]any)
		if int(e["code"].(float64)) != -32600 {
			t.Fatalf("element %d: expected -32600, got %v", i, e)
		}
	}

	if out[2]["result"] == nil {
		t.Fatalf("expected ping result, got %v", out[2])
	}
}

// A client that requests only ungrantable scopes must be rejected — not
// silently registered with the full default set.
func TestRegisterClientAllInvalidScopes(t *testing.T) {
	h, _, store := newTestOAuth()

	body := `{"client_name":"bad","redirect_uris":["https://x.com/cb"],"scope":"bogus.scope other.bad"}`
	req := httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	h.HandleRegister(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}

	if len(store.clients) != 0 {
		t.Fatalf("client registered despite invalid scopes")
	}
}

// A client that does not send a scope field still receives the default set.
func TestRegisterClientEmptyScopeGetsDefault(t *testing.T) {
	h, _, store := newTestOAuth()

	body := `{"client_name":"ok","redirect_uris":["https://x.com/cb"]}`
	req := httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	h.HandleRegister(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	for _, c := range store.clients {
		if len(c.Scopes) != len(oauthDefaultScopeSet) {
			t.Fatalf("expected default scope set, got %v", c.Scopes)
		}
	}
}

// A malformed code_challenge must be rejected as invalid_request and
// redirected back to the (validated) client redirect URI.
func TestAuthorizeRejectsMalformedChallenge(t *testing.T) {
	h, _, store := newTestOAuth()
	store.clients["jcmc_x"] = &OAuthClient{
		ClientID:     "jcmc_x",
		ClientName:   "X",
		RedirectURIs: []string{"https://x.com/cb"},
		GrantTypes:   []string{"authorization_code"},
		Scopes:       []string{"api.read.content"},
	}

	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {"jcmc_x"},
		"redirect_uri":          {"https://x.com/cb"},
		"scope":                 {"api.read.content"},
		"state":                 {"s1"},
		"code_challenge":        {"tooshort"},
		"code_challenge_method": {"S256"},
	}
	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+q.Encode(), nil)

	rr := httptest.NewRecorder()
	h.HandleAuthorize(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("expected 302 error redirect, got %d", rr.Code)
	}

	loc, _ := url.Parse(rr.Header().Get("Location"))
	if loc.Query().Get("error") != "invalid_request" {
		t.Fatalf("expected invalid_request redirect, got %q", rr.Header().Get("Location"))
	}
}

// A verifier that fails the RFC 7636 §4.1 shape is an invalid grant —
// distinct from a well-formed verifier that mismatches the challenge.
func TestTokenExchangeRejectsMalformedVerifier(t *testing.T) {
	_, svc, store := newTestOAuth()
	store.clients["jcmc_x"] = &OAuthClient{ClientID: "jcmc_x"}

	_, challenge := pkcePair()
	code, _ := newRandomToken(oauthAuthCodePrefix, 24)
	store.codes[hashToken(code)] = &OAuthAuthorizationCode{
		ClientID:      "jcmc_x",
		UserID:        testUserID,
		RedirectURI:   "https://x.com/cb",
		CodeChallenge: challenge,
		ExpiresAt:     pgtype.Timestamptz{Time: time.Now().Add(time.Minute), Valid: true},
	}

	for _, verifier := range []string{"short", strings.Repeat("a", 129), strings.Repeat("!", 64)} {
		if _, err := svc.ExchangeAuthorizationCode(t.Context(), code, "jcmc_x", "https://x.com/cb", verifier); err == nil {
			t.Fatalf("verifier %q should be rejected", verifier)
		}
	}
}
