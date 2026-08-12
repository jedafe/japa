package publicapi

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestSwaggerUIAssetsEmbedded verifies the go:embed directive actually
// embedded the Swagger UI dist files so the handler can serve them without
// a CDN dependency (PUB-016, Section 47).
func TestSwaggerUIAssetsEmbedded(t *testing.T) {
	want := []string{"swagger-ui.css", "swagger-ui-bundle.js", "swagger-ui-standalone-preset.js", "favicon-32x32.png"}
	for _, name := range want {
		b, err := fs.ReadFile(swaggerSubFS, name)
		if err != nil {
			t.Fatalf("embedded asset %s missing: %v", name, err)
		}
		if len(b) == 0 {
			t.Fatalf("embedded asset %s is empty", name)
		}
	}
}

// TestHandleSwaggerUIAssetsServesCSS confirms the assets handler serves an
// embedded file with the locked content-type and no CDN reference.
func TestHandleSwaggerUIAssetsServesCSS(t *testing.T) {
	srv := httptest.NewServer(http.StripPrefix("/developer/swagger/assets", HandleSwaggerUIAssets()))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/developer/swagger/assets/swagger-ui.css")
	if err != nil {
		t.Fatalf("get css: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/css") {
		t.Fatalf("content-type = %q, want text/css", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		t.Fatal("css body empty")
	}
}

// TestHandleSwaggerUIHTMLNoCDN confirms the Swagger UI HTML page loads
// assets from the same origin and contains no unpkg/CDN reference (PUB-016).
func TestHandleSwaggerUIHTMLNoCDN(t *testing.T) {
	srv := httptest.NewServer(HandleSwaggerUI("JapaConnect"))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("get html: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options = %q, want DENY", got)
	}
	csp := resp.Header.Get("Content-Security-Policy")
	if strings.Contains(csp, "unpkg") {
		t.Fatalf("CSP still allows unpkg: %q", csp)
	}
	body, _ := io.ReadAll(resp.Body)
	html := string(body)
	if strings.Contains(html, "unpkg.com") {
		t.Fatal("HTML still references unpkg.com CDN")
	}
	if !strings.Contains(html, "/developer/swagger/assets/swagger-ui.css") {
		t.Fatal("HTML does not reference local embedded swagger-ui.css")
	}
	if !strings.Contains(html, "/developer/swagger/assets/swagger-ui-bundle.js") {
		t.Fatal("HTML does not reference local embedded swagger-ui-bundle.js")
	}
}
