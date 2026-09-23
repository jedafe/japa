package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/valkey-io/valkey-go"
	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/intelligence/probe"
)

// --- probe test helpers ----------------------------------------------------

// stubProber satisfies previewProber — records the call and returns the
// canned report/error so handler tests never touch the network.
type stubProber struct {
	report   probe.Report
	err      error
	calls    int
	lastURL  string
	lastKind string
	lastCats []string
}

func (s *stubProber) Preview(_ context.Context, rawURL, kind string, categories []string) (probe.Report, error) {
	s.calls++
	s.lastURL = rawURL
	s.lastKind = kind
	s.lastCats = categories

	return s.report, s.err
}

// probeTestSettings is a SettingsReader stub returning the configured probe
// rate limit for intel.probe.rate_limit_per_hr and the fallback otherwise.
type probeTestSettings struct{ limit int }

func (s probeTestSettings) GetIntOr(_ context.Context, key string, fallback int) int {
	if key == "intel.probe.rate_limit_per_hr" {
		return s.limit
	}

	return fallback
}

func (s probeTestSettings) GetStringOr(_ context.Context, _, fallback string) string {
	return fallback
}

// startProbeTestValkey spins up an in-memory Valkey (miniredis) so the probe
// quota counter can be exercised without a real server. Mirrors the
// startTestValkey helper used by the fetcher package tests.
func startProbeTestValkey(t *testing.T) valkeycompat.Cmdable {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)

	vk, err := valkey.NewClient(valkey.ClientOption{
		InitAddress:  []string{mr.Addr()},
		DisableCache: true,
	})
	if err != nil {
		t.Fatalf("valkey.NewClient: %v", err)
	}
	t.Cleanup(vk.Close)

	return valkeycompat.NewAdapter(vk)
}

// doProbeRequest invokes HandleProbeSource with a JSON body; authed injects
// an AuthenticatedUser into the request context as RequireAuth would.
func doProbeRequest(t *testing.T, h *Handlers, body string, authed bool) *httptest.ResponseRecorder {
	t.Helper()

	r := httptest.NewRequest(http.MethodPost, "/api/v1/sources/probe", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	if authed {
		u := &auth.AuthenticatedUser{UserID: mustUUID(t, "550e8400-e29b-41d4-a716-446655440000")}
		r = r.WithContext(context.WithValue(r.Context(), auth.UserContextKey, u))
	}

	rr := httptest.NewRecorder()
	h.HandleProbeSource(rr, r)

	return rr
}

func decodeDataEnvelope(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&env); err != nil {
		t.Fatalf("decode data envelope: %v (body=%s)", err, rr.Body.String())
	}

	return env.Data
}

// --- tests -----------------------------------------------------------------

// TestHandleProbeSource_NotConfigured covers the nil-dependency contract:
// no prober wired → 503 intel_unavailable before touching auth or the body.
func TestHandleProbeSource_NotConfigured(t *testing.T) {
	t.Parallel()

	h := &Handlers{}
	rr := doProbeRequest(t, h, `{"url":"https://example.com"}`, true)

	assertIntelUnavailable(t, rr)
}

// TestHandleProbeSource_Unauthenticated verifies the endpoint requires auth —
// the probe spends outbound requests so it must not be anonymous.
func TestHandleProbeSource_Unauthenticated(t *testing.T) {
	t.Parallel()

	h := &Handlers{prober: &stubProber{}}
	rr := doProbeRequest(t, h, `{"url":"https://example.com"}`, false)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (body=%s)", rr.Code, http.StatusUnauthorized, rr.Body.String())
	}

	var env errorEnvelope
	if err := json.NewDecoder(rr.Body).Decode(&env); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}

	if env.Error.Code != "unauthorized" {
		t.Errorf("error.code = %q, want unauthorized", env.Error.Code)
	}
}

// TestHandleProbeSource_RejectsPrivateTargets covers the entry-URL SSRF
// guard: literal private IPs, link-local metadata endpoints, and non-http(s)
// schemes are rejected before the prober runs.
func TestHandleProbeSource_RejectsPrivateTargets(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		body       string
		wantStatus int
		wantCode   string
	}{
		{"loopback_ip", `{"url":"http://127.0.0.1:8080/admin"}`, http.StatusUnprocessableEntity, "url_not_allowed"},
		{"link_local_metadata", `{"url":"http://169.254.169.254/latest/meta-data"}`, http.StatusUnprocessableEntity, "url_not_allowed"},
		{"private_class_a", `{"url":"http://10.0.0.5/internal"}`, http.StatusUnprocessableEntity, "url_not_allowed"},
		{"private_class_c", `{"url":"https://192.168.1.1/"}`, http.StatusUnprocessableEntity, "url_not_allowed"},
		{"file_scheme", `{"url":"file:///etc/passwd"}`, http.StatusBadRequest, "invalid_url"},
		{"missing_url", `{"source_kind":"publisher"}`, http.StatusBadRequest, "missing_url"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stub := &stubProber{}
			h := &Handlers{prober: stub}
			rr := doProbeRequest(t, h, tc.body, true)

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

			if stub.calls != 0 {
				t.Errorf("prober called %d times for a rejected target", stub.calls)
			}
		})
	}
}

// TestHandleProbeSource_HappyPath verifies a successful probe: the request
// URL/kind/categories reach the prober and the report is returned under
// `data`. `source_url` is accepted as an alias for `url` (propose-form
// payload field name).
func TestHandleProbeSource_HappyPath(t *testing.T) {
	t.Parallel()

	stub := &stubProber{report: probe.Report{
		FinalURL: "https://example.com/scholarships",
		Methods: []probe.DetectedMethod{
			{Method: "rss", Confidence: 0.95, Evidence: `<link rel="alternate">`},
		},
		Channels: []probe.ChannelProposal{
			{Label: "RSS feed", EntryURL: "https://example.com/feed", CrawlMethod: "rss", Pipeline: "curate", Confidence: 0.95},
		},
		ProbedAt: time.Now().UTC(),
	}}

	h := &Handlers{prober: stub}
	rr := doProbeRequest(t, h,
		`{"source_url":"https://example.com/scholarships","source_kind":"publisher","categories":["scholarship","news"]}`,
		true)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", rr.Code, http.StatusOK, rr.Body.String())
	}

	if stub.calls != 1 {
		t.Fatalf("prober calls = %d, want 1", stub.calls)
	}

	if stub.lastURL != "https://example.com/scholarships" {
		t.Errorf("prober url = %q, want https://example.com/scholarships", stub.lastURL)
	}

	if stub.lastKind != "publisher" {
		t.Errorf("prober kind = %q, want publisher", stub.lastKind)
	}

	if len(stub.lastCats) != 2 || stub.lastCats[0] != "scholarship" {
		t.Errorf("prober categories = %v, want [scholarship news]", stub.lastCats)
	}

	data := decodeDataEnvelope(t, rr)
	if data["final_url"] != "https://example.com/scholarships" {
		t.Errorf("data.final_url = %v", data["final_url"])
	}

	channels, ok := data["channels"].([]any)
	if !ok || len(channels) != 1 {
		t.Fatalf("data.channels = %v, want 1 proposal", data["channels"])
	}
}

// TestHandleProbeSource_QuotaEnforced exercises the Valkey fixed-window
// quota: with a limit of 2 the first two probes pass and the third is 429.
func TestHandleProbeSource_QuotaEnforced(t *testing.T) {
	t.Parallel()

	h := &Handlers{
		prober:   &stubProber{report: probe.Report{FinalURL: "https://example.com"}},
		rdb:      startProbeTestValkey(t),
		settings: probeTestSettings{limit: 2},
	}

	for i := 1; i <= 2; i++ {
		rr := doProbeRequest(t, h, `{"url":"https://example.com"}`, true)
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200 (body=%s)", i, rr.Code, rr.Body.String())
		}
	}

	rr := doProbeRequest(t, h, `{"url":"https://example.com"}`, true)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("request 3: status = %d, want 429 (body=%s)", rr.Code, rr.Body.String())
	}

	var env errorEnvelope
	if err := json.NewDecoder(rr.Body).Decode(&env); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}

	if env.Error.Code != "probe_quota_exceeded" {
		t.Errorf("error.code = %q, want probe_quota_exceeded", env.Error.Code)
	}
}

// TestHandleProbeSource_ProberErrorMapping covers the Preview error → HTTP
// mapping: validation sentinels → 400, SSRF rejections → 422, transport
// failures → 502.
func TestHandleProbeSource_ProberErrorMapping(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"invalid_url", errpkg.ErrInvalid, http.StatusBadRequest, "invalid_url"},
		{"ssrf_redirect", errors.New("fetcher: redirect to private IP 169.254.169.254 rejected (SSRF)"), http.StatusUnprocessableEntity, "url_not_allowed"},
		{"transport_failure", errors.New("dial tcp: connection refused"), http.StatusBadGateway, "probe_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := &Handlers{prober: &stubProber{err: tc.err}}
			rr := doProbeRequest(t, h, `{"url":"https://example.com"}`, true)

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

// TestHandleGetSourcesMeta_IntelChannels verifies the intel-channels meta
// extension: source_kinds carries the 10-kind {value,label} set,
// channel_pipelines lists extract|curate|hybrid, and the public categories
// list includes the new proposal categories.
func TestHandleGetSourcesMeta_IntelChannels(t *testing.T) {
	t.Parallel()

	h := &Handlers{}
	r := httptest.NewRequest(http.MethodGet, "/api/v1/sources/meta", nil)
	rr := httptest.NewRecorder()
	h.HandleGetSourcesMeta(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rr.Code, rr.Body.String())
	}

	data := decodeDataEnvelope(t, rr)

	// channel_pipelines must be exactly the CHECK set.
	pipelines, ok := data["channel_pipelines"].([]any)
	if !ok {
		t.Fatalf("channel_pipelines missing or wrong type: %v", data["channel_pipelines"])
	}

	got := make([]string, 0, len(pipelines))
	for _, p := range pipelines {
		got = append(got, p.(string))
	}

	want := []string{"extract", "curate", "hybrid"}
	if len(got) != len(want) {
		t.Fatalf("channel_pipelines = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("channel_pipelines[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	// source_kinds must carry the 10-kind set as {value,label} options.
	kinds, ok := data["source_kinds"].([]any)
	if !ok || len(kinds) != 10 {
		t.Fatalf("source_kinds = %v, want 10 options", data["source_kinds"])
	}

	kindValues := map[string]bool{}
	for _, k := range kinds {
		opt, ok := k.(map[string]any)
		if !ok {
			t.Fatalf("source_kinds option not an object: %v", k)
		}
		if opt["value"] == "" || opt["label"] == "" {
			t.Errorf("source_kinds option missing value/label: %v", opt)
		}
		kindValues[opt["value"].(string)] = true
	}

	for _, v := range []string{
		"publisher", "institution", "individual", "aggregator",
		"channel_profile", "institution_site", "personal_blog",
		"job_board", "gov_portal", "community",
	} {
		if !kindValues[v] {
			t.Errorf("source_kinds missing %q", v)
		}
	}

	// submittable_source_kinds is the propose-form subset: the 8-kind set
	// minus the entity-stub kinds (institution, individual).
	subKinds, ok := data["submittable_source_kinds"].([]any)
	if !ok || len(subKinds) != 8 {
		t.Fatalf("submittable_source_kinds = %v, want 8 options", data["submittable_source_kinds"])
	}
	for _, k := range subKinds {
		opt := k.(map[string]any)
		if v := opt["value"]; v == "institution" || v == "individual" {
			t.Errorf("submittable_source_kinds must not contain %q", v)
		}
	}

	// categories must include the intel-channels additions.
	cats, ok := data["categories"].([]any)
	if !ok {
		t.Fatalf("categories missing or wrong type: %v", data["categories"])
	}

	catSet := map[string]bool{}
	for _, c := range cats {
		catSet[c.(string)] = true
	}
	for _, v := range []string{"blog", "event", "dataset", "announcement", "scam_alert"} {
		if !catSet[v] {
			t.Errorf("categories missing %q", v)
		}
	}
}
