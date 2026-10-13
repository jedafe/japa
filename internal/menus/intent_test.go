package menus

import (
	"testing"

	"github.com/jedafe/japa/internal/visitor"
)

func item(url string, children ...MenuItem) MenuItem {
	return MenuItem{URL: url, Children: children}
}

func TestFilterIntentItemsHidesAndPins(t *testing.T) {
	items := []MenuItem{
		item("/feed"),
		item("/universities"),
		item("/tools", item("/tools/travel"), item("/tools/readiness")),
		item("/scholarships"),
		item("/wiki"),
	}

	rule := visitor.IntentRule{
		HidePaths: []string{"/universities", "/scholarships"},
		PinPaths:  []string{"/wiki", "/tools"},
	}

	out := filterIntentItems(items, rule)

	var urls []string
	for _, it := range out {
		urls = append(urls, it.URL)
	}

	// Pinned entries lead, preserving relative order.
	if len(urls) < 2 || urls[0] != "/tools" || urls[1] != "/wiki" {
		t.Fatalf("expected pinned [/tools /wiki] first, got %v", urls)
	}

	// Hidden entries are gone.
	for _, u := range urls {
		if u == "/universities" || u == "/scholarships" {
			t.Fatalf("hidden path %s survived: %v", u, urls)
		}
	}

	// Unpinned survivors keep their relative order.
	if urls[len(urls)-1] != "/feed" {
		t.Fatalf("expected /feed last, got %v", urls)
	}
}

func TestFilterIntentItemsHidesSubtrees(t *testing.T) {
	items := []MenuItem{
		item("/tools", item("/tools/travel")),
	}

	out := filterIntentItems(items, visitor.IntentRule{HidePaths: []string{"/tools"}})
	if len(out) != 0 {
		t.Fatalf("expected parent hide to drop subtree, got %v", out)
	}
}

func TestFilterIntentItemsSegmentAware(t *testing.T) {
	items := []MenuItem{item("/toolsmith"), item("/tools")}

	out := filterIntentItems(items, visitor.IntentRule{HidePaths: []string{"/tools"}})
	if len(out) != 1 || out[0].URL != "/toolsmith" {
		t.Fatalf("expected /toolsmith to survive /tools prefix, got %v", out)
	}
}
