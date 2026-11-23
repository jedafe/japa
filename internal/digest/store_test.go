package digest

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/valkey-io/valkey-go"
	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db"
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
		query      string
		wantPage   int32
		wantLimit  int32
		wantOffset int32
	}{
		{"defaults", "", 1, 20, 0},
		{"page and limit", "?page=3&limit=50", 3, 50, 100},
		{"zero page rejected", "?page=0", 1, 20, 0},
		{"negative page rejected", "?page=-2", 1, 20, 0},
		{"non-numeric ignored", "?page=abc&limit=xyz", 1, 20, 0},
		{"limit over 100 rejected to default", "?limit=500", 1, 20, 0},
		{"canonical page_size+page_offset", "?page_size=25&page_offset=50", 3, 25, 50},
		{"unaligned page_offset kept verbatim", "?page_size=20&page_offset=50", 3, 20, 50},
		{"page_offset wins over page", "?page=5&page_size=10&page_offset=30", 4, 10, 30},
		{"negative page_offset rejected to default", "?page_offset=-4", 1, 20, 0},
		{"page_size wins over limit alias", "?page_size=15&limit=40", 1, 15, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequest(http.MethodGet, "/admin/digest"+tc.query, nil)

			page, limit, offset := parsePageParams(r)
			if page != tc.wantPage || limit != tc.wantLimit || offset != tc.wantOffset {
				t.Errorf("parsePageParams(%q) = (%d, %d, %d), want (%d, %d, %d)",
					tc.query, page, limit, offset, tc.wantPage, tc.wantLimit, tc.wantOffset)
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

// ---------------------------------------------------------------------------
// AdminGenerate — per-admin cooldown lock (audit BUG-013)
// ---------------------------------------------------------------------------

func newCooldownTestService(t *testing.T) (*Service, *miniredis.Miniredis) {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	t.Cleanup(mr.Close)

	vk, err := valkey.NewClient(valkey.ClientOption{
		InitAddress:  []string{mr.Addr()},
		DisableCache: true,
	})
	if err != nil {
		t.Fatalf("valkey connect: %v", err)
	}
	t.Cleanup(func() { vk.Close() })

	compat := valkeycompat.NewAdapter(vk)

	// Settings read path resolves via the L2 Valkey cache — seeding it keeps
	// the nil-pool DB out of the test.
	settings := config.NewSettings(&db.DB{}, compat)
	t.Cleanup(settings.Close)

	seed := fmt.Sprintf(`{"value":%q,"created_at":%q}`, "300", time.Now().UTC().Format(time.RFC3339Nano))
	if err := compat.Set(t.Context(), "jc:settings:digest.admin_generate_cooldown_seconds", seed, 0).Err(); err != nil {
		t.Fatalf("seed settings cache: %v", err)
	}

	svc := NewService(nil, settings)
	svc.SetValkey(compat)
	return svc, mr
}

func TestAdminGenerateCooldown(t *testing.T) {
	t.Parallel()

	svc, mr := newCooldownTestService(t)
	ctx := t.Context()

	var adminA, adminB pgtype.UUID
	if err := adminA.Scan("11111111-1111-1111-1111-111111111111"); err != nil {
		t.Fatal(err)
	}
	if err := adminB.Scan("22222222-2222-2222-2222-222222222222"); err != nil {
		t.Fatal(err)
	}

	st, err := svc.AdminGenerate(ctx, adminA, pgtype.UUID{})
	if err != nil || st != "accepted" {
		t.Fatalf("first generate = (%q, %v), want (accepted, nil)", st, err)
	}
	if !mr.Exists("jc:digest_admin_gen_lock:" + adminA.String()) {
		t.Fatal("cooldown lock key was not written")
	}

	if _, err := svc.AdminGenerate(ctx, adminA, pgtype.UUID{}); !errors.Is(err, ErrGenerateCooldown) {
		t.Fatalf("second generate err = %v, want ErrGenerateCooldown", err)
	}

	// A different admin is not blocked by A's lock.
	if _, err := svc.AdminGenerate(ctx, adminB, pgtype.UUID{}); err != nil {
		t.Fatalf("admin B generate err = %v, want nil", err)
	}
}

// ---------------------------------------------------------------------------
// AdminUpdate — PATCH validation (audit BUG-023)
// ---------------------------------------------------------------------------

func TestAdminUpdateValidation(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil)
	ctx := t.Context()

	var id pgtype.UUID
	if err := id.Scan("33333333-3333-3333-3333-333333333333"); err != nil {
		t.Fatal(err)
	}

	if err := svc.AdminUpdate(ctx, id, AdminUpdateInput{}); !errors.Is(err, ErrNoUpdateFields) {
		t.Errorf("empty input err = %v, want ErrNoUpdateFields", err)
	}

	badCadence := "daily"
	if err := svc.AdminUpdate(ctx, id, AdminUpdateInput{Cadence: &badCadence}); !errors.Is(err, ErrInvalidCadence) {
		t.Errorf("bad cadence err = %v, want ErrInvalidCadence", err)
	}

	past := time.Now().Add(-time.Hour)
	if err := svc.AdminUpdate(ctx, id, AdminUpdateInput{ScheduledFor: &past}); !errors.Is(err, ErrInvalidSchedule) {
		t.Errorf("past schedule err = %v, want ErrInvalidSchedule", err)
	}

	array := json.RawMessage(`[1,2]`)
	if err := svc.AdminUpdate(ctx, id, AdminUpdateInput{Content: &array}); !errors.Is(err, ErrInvalidContent) {
		t.Errorf("array content err = %v, want ErrInvalidContent", err)
	}
}
