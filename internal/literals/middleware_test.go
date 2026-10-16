package literals

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/valkey-io/valkey-go"
	"github.com/valkey-io/valkey-go/valkeycompat"

	jcvalkey "github.com/jedafe/japa/internal/valkey"
)

// testCatalog builds a Catalog backed by miniredis seeded with testTokens at
// the catalog cache key, so ResolveJSON has literals to work with.
func testCatalog(t *testing.T) *Catalog {
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

	t.Cleanup(func() { vk.Close() })

	compat := valkeycompat.NewAdapter(vk)

	encoded, err := json.Marshal(testTokens)
	if err != nil {
		t.Fatal(err)
	}

	if err := compat.Set(t.Context(), jcvalkey.MustKey(catalogKeyTemplate, ""), string(encoded), 0).Err(); err != nil {
		t.Fatalf("seed catalog: %v", err)
	}

	return NewCatalog(nil, compat)
}

func serve(t *testing.T, cat *Catalog, handler http.HandlerFunc, target string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	Middleware(cat)(handler).ServeHTTP(rec, req)

	return rec
}

func TestMiddlewareResolvesJSONBody(t *testing.T) {
	t.Parallel()

	rec := serve(t, testCatalog(t), func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"title":"{{Platform_Name}} {{Current_Year}}","id":"{{keep}}"}`))
	}, "/api/v1/x")

	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q", got)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON %q: %v", rec.Body.String(), err)
	}

	if body["title"] != "JapaConnect" {
		// Current_Year resolves to the real year — just check {{ is gone.
		if !strings.Contains(body["title"].(string), "JapaConnect") || strings.Contains(body["title"].(string), "{{") {
			t.Fatalf("title = %v", body["title"])
		}
	}

	if body["id"] != "{{keep}}" {
		t.Fatalf("id = %v — unknown tokens must pass through", body["id"])
	}

	if got := rec.Header().Get("Content-Length"); got == "" {
		t.Fatal("Content-Length not set on buffered response")
	}
}

func TestMiddlewareRawRequestPassthrough(t *testing.T) {
	t.Parallel()

	rec := serve(t, testCatalog(t), func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"title":"{{Platform_Name}}"}`))
	}, "/api/v1/x?raw=1")

	if !strings.Contains(rec.Body.String(), "{{Platform_Name}}") {
		t.Fatalf("raw=1 body was resolved: %q", rec.Body.String())
	}
}

func TestMiddlewareNonJSONPassthrough(t *testing.T) {
	t.Parallel()

	xml := `<feed><title>{{Platform_Name}}</title></feed>`
	rec := serve(t, testCatalog(t), func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(xml))
	}, "/feed.xml")

	if rec.Body.String() != xml {
		t.Fatalf("non-JSON body altered: %q", rec.Body.String())
	}
}

func TestMiddlewareFlushPassthrough(t *testing.T) {
	t.Parallel()

	rec := serve(t, testCatalog(t), func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"a":"{{Platform_Name}}"}`))
		w.(http.Flusher).Flush()
		_, _ = w.Write([]byte(`{"b":"{{Platform_Name}}"}`))
	}, "/api/v1/x")

	// A mid-handler flush streams the buffered head unmodified.
	if !strings.Contains(rec.Body.String(), "{{Platform_Name}}") {
		t.Fatalf("flushed body altered: %q", rec.Body.String())
	}
}

func TestMiddlewareUpgradePassthrough(t *testing.T) {
	t.Parallel()

	cat := testCatalog(t)
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "upgrade")

	rec := httptest.NewRecorder()
	Middleware(cat)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"x":"{{Platform_Name}}"}`))
	})).ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), "{{Platform_Name}}") {
		t.Fatalf("upgrade body altered: %q", rec.Body.String())
	}
}

func TestMiddlewareNoContent(t *testing.T) {
	t.Parallel()

	rec := serve(t, testCatalog(t), func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNoContent)
	}, "/api/v1/x")

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}

	if rec.Body.Len() != 0 {
		t.Fatalf("204 body = %q", rec.Body.String())
	}
}
