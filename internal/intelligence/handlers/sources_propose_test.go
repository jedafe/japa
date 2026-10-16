package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/intelligence/sources"
)

// doProposeRequest invokes HandleProposeSource with a JSON body; authed
// injects an AuthenticatedUser into the request context as RequireAuth
// would.
func doProposeRequest(t *testing.T, h *Handlers, body string, authed bool) *httptest.ResponseRecorder {
	t.Helper()

	r := httptest.NewRequest(http.MethodPost, "/api/v1/sources/propose", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	if authed {
		u := &auth.AuthenticatedUser{UserID: mustUUID(t, "550e8400-e29b-41d4-a716-446655440000")}
		r = r.WithContext(context.WithValue(r.Context(), auth.UserContextKey, u))
	}

	rr := httptest.NewRecorder()
	h.HandleProposeSource(rr, r)

	return rr
}

// TestHandleProposeSource_NotConfigured covers the nil-dependency contract:
// no sources service wired → 503 intel_unavailable before auth or body.
func TestHandleProposeSource_NotConfigured(t *testing.T) {
	t.Parallel()

	h := &Handlers{}
	rr := doProposeRequest(t, h, `{"name":"X"}`, true)

	assertIntelUnavailable(t, rr)
}

// TestHandleProposeSource_Unauthenticated verifies the endpoint requires
// auth — a proposal creates a proposed source row attributed to the user.
func TestHandleProposeSource_Unauthenticated(t *testing.T) {
	t.Parallel()

	h := &Handlers{intelSources: sources.NewService(nil)}
	rr := doProposeRequest(t, h, `{"name":"X","source_url":"https://example.com","categories":["news"]}`, false)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (body=%s)", rr.Code, http.StatusUnauthorized, rr.Body.String())
	}
}

// TestHandleProposeSource_Validation covers the intel-channels payload rules
// the handler surfaces as 400s. The service is constructed with a nil store:
// every case here fails validation before the uniqueness lookup, so no
// database is needed.
func TestHandleProposeSource_Validation(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		body       string
		wantStatus int
		wantCode   string
	}{
		{
			"missing categories",
			`{"name":"Example","source_url":"https://example.com","source_kind":"publisher"}`,
			http.StatusBadRequest, "missing_required",
		},
		{
			"empty categories array",
			`{"name":"Example","source_url":"https://example.com","categories":[]}`,
			http.StatusBadRequest, "missing_required",
		},
		{
			"unknown source_kind",
			`{"name":"Example","source_url":"https://example.com","categories":["news"],"source_kind":"robot"}`,
			http.StatusBadRequest, "invalid_source_kind",
		},
		{
			"non-object probe_snapshot",
			`{"name":"Example","source_url":"https://example.com","categories":["news"],"probe_snapshot":["rss"]}`,
			http.StatusBadRequest, "invalid_probe_snapshot",
		},
		{
			"invalid source_url",
			`{"name":"Example","source_url":"ftp://example.com","categories":["news"]}`,
			http.StatusBadRequest, "invalid_source_url",
		},
		{
			"legacy video on non-platform host",
			`{"name":"Example","source_url":"https://example.com/videos","categories":["news"],"source_type":"video"}`,
			http.StatusBadRequest, "invalid_source_url",
		},
		{
			"missing name",
			`{"source_url":"https://example.com","categories":["news"]}`,
			http.StatusBadRequest, "missing_required",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := &Handlers{intelSources: sources.NewService(nil)}
			rr := doProposeRequest(t, h, tc.body, true)

			if rr.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body=%s)", rr.Code, tc.wantStatus, rr.Body.String())
			}

			var env errorEnvelope
			if err := json.NewDecoder(rr.Body).Decode(&env); err != nil {
				t.Fatalf("decode error envelope: %v", err)
			}

			if env.Error.Code != tc.wantCode {
				t.Errorf("error.code = %q, want %q", env.Error.Code, tc.wantCode)
			}
		})
	}
}

// TestProposeSourceRequest_NewFields verifies the request DTO binds the
// intel-channels payload fields: categories array, source_kind, and the raw
// probe_snapshot document.
func TestProposeSourceRequest_NewFields(t *testing.T) {
	t.Parallel()

	var req proposeSourceRequest

	body := `{
		"name": "Example",
		"source_url": "https://example.com",
		"categories": ["news", "job"],
		"source_kind": "aggregator",
		"probe_snapshot": {"final_url": "https://example.com", "channels": [{"method": "rss"}]}
	}`
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&req); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(req.Categories) != 2 || req.Categories[0] != "news" || req.Categories[1] != "job" {
		t.Errorf("categories = %v, want [news job]", req.Categories)
	}

	if req.SourceKind != "aggregator" {
		t.Errorf("source_kind = %q, want aggregator", req.SourceKind)
	}

	if len(req.ProbeSnapshot) == 0 {
		t.Error("probe_snapshot was not bound")
	}

	var snapshot map[string]json.RawMessage
	if err := json.Unmarshal(req.ProbeSnapshot, &snapshot); err != nil {
		t.Fatalf("probe_snapshot is not a JSON object: %v", err)
	}

	if _, ok := snapshot["final_url"]; !ok {
		t.Errorf("probe_snapshot missing final_url: %s", req.ProbeSnapshot)
	}
}
