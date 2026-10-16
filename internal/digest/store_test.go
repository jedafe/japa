package digest

import (
	"net/http"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// NormalizeContent — §84 canonical keys (events/story/new_members)
// ---------------------------------------------------------------------------

func TestNormalizeContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		in    string
		check func(t *testing.T, out map[string]json.RawMessage)
	}{
		{
			name: "legacy keys map to canonical",
			in:   `{"upcoming_events":[{"title":"E1"}],"featured_story":{"title":"S1"},"members":[{"username":"u1"}]}`,
			check: func(t *testing.T, out map[string]json.RawMessage) {
				for _, legacy := range []string{"upcoming_events", "featured_story", "members"} {
					if _, ok := out[legacy]; ok {
						t.Errorf("legacy key %q still present", legacy)
					}
				}
				for _, canonical := range []string{"events", "story", "new_members"} {
					if _, ok := out[canonical]; !ok {
						t.Errorf("canonical key %q missing", canonical)
					}
				}
			},
		},
		{
			name: "canonical wins on collision",
			in:   `{"events":[{"title":"canon"}],"upcoming_events":[{"title":"legacy"}]}`,
			check: func(t *testing.T, out map[string]json.RawMessage) {
				if !strings.Contains(string(out["events"]), "canon") {
					t.Errorf("events = %s, want canonical value", out["events"])
				}
				if _, ok := out["upcoming_events"]; ok {
					t.Error("legacy key not removed")
				}
			},
		},
		{
			name: "unknown keys preserved",
			in:   `{"events":[],"custom_block":{"foo":1}}`,
			check: func(t *testing.T, out map[string]json.RawMessage) {
				if _, ok := out["custom_block"]; !ok {
					t.Error("unknown key dropped")
				}
			},
		},
		{
			name: "already canonical passes through unchanged",
			in:   `{"events":[],"story":null}`,
			check: func(t *testing.T, out map[string]json.RawMessage) {
				if _, ok := out["events"]; !ok {
					t.Error("events missing")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out := NormalizeContent(json.RawMessage(tc.in))

			var m map[string]json.RawMessage

			if err := json.Unmarshal(out, &m); err != nil {
				t.Fatalf("normalized output is not a JSON object: %v", err)
			}

			tc.check(t, m)
		})
	}
}

func TestNormalizeContentPassthrough(t *testing.T) {
	t.Parallel()

	for _, in := range []string{
		"",
		`"not an object"`,
		`[1,2,3]`,
		`{broken`,
	} {
		got := NormalizeContent(json.RawMessage(in))
		if string(got) != in {
			t.Errorf("NormalizeContent(%q) = %q, want passthrough", in, got)
		}
	}
}

// ---------------------------------------------------------------------------
// DigestContent.UnmarshalJSON — legacy key spellings
// ---------------------------------------------------------------------------

func TestDigestContentUnmarshalLegacyKeys(t *testing.T) {
	t.Parallel()

	raw := `{
		"top_threads": [{"title": "T1"}],
		"members": [{"username": "u1"}],
		"upcoming_events": [{"title": "E1", "slug": "e1"}],
		"featured_story": {"title": "S1", "slug": "s1"}
	}`

	var c DigestContent
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(c.TopThreads) != 1 || c.TopThreads[0].Title != "T1" {
		t.Errorf("top_threads not decoded: %+v", c.TopThreads)
	}

	if len(c.NewMembers) != 1 || c.NewMembers[0].Username != "u1" {
		t.Errorf("members→new_members mapping failed: %+v", c.NewMembers)
	}

	if len(c.UpcomingEvents) != 1 || c.UpcomingEvents[0].Title != "E1" {
		t.Errorf("upcoming_events→events mapping failed: %+v", c.UpcomingEvents)
	}

	if c.FeaturedStory == nil || c.FeaturedStory.Title != "S1" {
		t.Errorf("featured_story→story mapping failed: %+v", c.FeaturedStory)
	}
}

func TestDigestContentCanonicalKeysWin(t *testing.T) {
	t.Parallel()

	raw := `{"events":[{"title":"canon"}],"upcoming_events":[{"title":"legacy"}],"story":{"title":"S"},"featured_story":{"title":"old"}}`

	var c DigestContent
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(c.UpcomingEvents) != 1 || c.UpcomingEvents[0].Title != "canon" {
		t.Errorf("canonical events lost to legacy: %+v", c.UpcomingEvents)
	}

	if c.FeaturedStory == nil || c.FeaturedStory.Title != "S" {
		t.Errorf("canonical story lost to legacy: %+v", c.FeaturedStory)
	}
}

func TestDigestContentMarshalCanonicalKeys(t *testing.T) {
	t.Parallel()

	c := DigestContent{
		UpcomingEvents: []DigestEvent{{Title: "E", Slug: "e", StartsAt: "2026-10-05T10:00:00Z"}},
		FeaturedStory:  &DigestStory{Title: "S", Slug: "s"},
	}

	out, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	s := string(out)
	for _, key := range []string{`"events"`, `"story"`, `"starts_at"`} {
		if !strings.Contains(s, key) {
			t.Errorf("marshal output missing %s: %s", key, s)
		}
	}

	for _, legacy := range []string{"upcoming_events", "featured_story"} {
		if strings.Contains(s, legacy) {
			t.Errorf("marshal output contains legacy key %s: %s", legacy, s)
		}
	}
}

// ---------------------------------------------------------------------------
// parsePageParams — 1-based page + limit guards
// ---------------------------------------------------------------------------

func TestParsePageParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		query     string
		wantPage  int32
		wantLimit int32
	}{
		{"defaults", "", 1, 20},
		{"page and limit", "?page=3&limit=50", 3, 50},
		{"zero page rejected", "?page=0", 1, 20},
		{"negative page rejected", "?page=-2", 1, 20},
		{"non-numeric ignored", "?page=abc&limit=xyz", 1, 20},
		{"limit over 100 rejected to default", "?limit=500", 1, 20},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequest(http.MethodGet, "/admin/digest"+tc.query, nil)

			page, limit := parsePageParams(r)
			if page != tc.wantPage || limit != tc.wantLimit {
				t.Errorf("parsePageParams(%q) = (%d, %d), want (%d, %d)",
					tc.query, page, limit, tc.wantPage, tc.wantLimit)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Email rendering — legacy content keys must render (pre-fix stored rows)
// ---------------------------------------------------------------------------

func TestRenderDigestEmailLegacyContent(t *testing.T) {
	t.Parallel()

	legacy := `{"upcoming_events":[{"title":"E1","slug":"e1"}],"featured_story":{"title":"S1","slug":"s1"}}`
	html := renderDigestEmail([]byte(legacy), "https://example.com")

	if !strings.Contains(html, "E1") {
		t.Error("legacy upcoming_events section not rendered")
	}

	if !strings.Contains(html, "S1") {
		t.Error("legacy featured_story section not rendered")
	}
}

func TestAppendUnsubscribeLink(t *testing.T) {
	t.Parallel()

	html := appendUnsubscribeLinkToHTML("<div>body</div>", "https://example.com/unsub?token=abc")
	if !strings.Contains(html, "https://example.com/unsub?token=abc") {
		t.Error("unsubscribe URL missing")
	}

	if !strings.Contains(html, "<div>body</div>") {
		t.Error("original body not preserved")
	}
}

// ---------------------------------------------------------------------------
// allowedSortColumns — whitelist must cover the admin sortable columns
// ---------------------------------------------------------------------------

func TestAllowedSortColumns(t *testing.T) {
	t.Parallel()

	want := []string{"cadence", "status", "email_sent", "created_at", "published_at", "scheduled_for"}
	for _, col := range want {
		if _, ok := allowedSortColumns[col]; !ok {
			t.Errorf("sort column %q missing from whitelist", col)
		}
	}

	if _, ok := allowedSortColumns["content"]; ok {
		t.Error("content must not be sortable (JSONB)")
	}
}
