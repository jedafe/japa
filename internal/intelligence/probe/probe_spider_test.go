package probe

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// linkIndexPage renders n distinct same-host anchors plus noise that must
// not count (asset, fragment, external).
func linkIndexPage(base string, n int) string {
	var b strings.Builder

	b.WriteString("<html><body>")

	for i := range n {
		fmt.Fprintf(&b, `<a href="/item/%d">Item %d</a>`, i, i)
	}

	fmt.Fprintf(&b, `<a href="%s/logo.png">logo</a><a href="#frag">frag</a><a href="https://other.example/x">ext</a>`, base)
	b.WriteString("</body></html>")

	return b.String()
}

// TestPreview_SpiderLinkIndex verifies the spider detector proposes a
// bounded same-host crawl when the entry page is a link index.
func TestPreview_SpiderLinkIndex(t *testing.T) {
	t.Parallel()

	var srvURL string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(linkIndexPage(srvURL, 12)))
	}))
	t.Cleanup(srv.Close)
	srvURL = srv.URL

	report, err := NewProber(srv.Client(), 5).Preview(t.Context(), srv.URL+"/", "", nil)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	if _, ok := findMethod(report, "spider"); !ok {
		t.Fatalf("methods missing spider on a 12-link index page: %+v", report.Methods)
	}

	prop, ok := findChannel(report, "spider")
	if !ok {
		t.Fatalf("channels missing spider proposal: %+v", report.Channels)
	}

	if prop.EntryURL != srv.URL+"/" {
		t.Errorf("spider proposal entry_url = %q, want %q", prop.EntryURL, srv.URL+"/")
	}
}

// TestPreview_SpiderBelowFloor verifies a sparse page (fewer than the
// distinct-same-host floor) earns no spider proposal.
func TestPreview_SpiderBelowFloor(t *testing.T) {
	t.Parallel()

	var srvURL string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(linkIndexPage(srvURL, 4)))
	}))
	t.Cleanup(srv.Close)
	srvURL = srv.URL

	report, err := NewProber(srv.Client(), 5).Preview(t.Context(), srv.URL+"/", "", nil)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	if _, ok := findMethod(report, "spider"); ok {
		t.Errorf("spider method reported on a 4-link page: %+v", report.Methods)
	}
}
