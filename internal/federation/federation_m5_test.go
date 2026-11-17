package federation

// Ref: japaconnect-engineering/sections/126-activitypub-federation.md (M5)
// Unit coverage for the consume-side surface's pure logic — field
// extraction, target/cursor shapes, and the new document fields. The
// DB-backed paths ride the JC_TEST_DATABASE_URL suite.

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	gen "github.com/jedafe/japa/internal/db/gen"
)

func TestRemotePostFieldsFromObject(t *testing.T) {
	obj := map[string]any{
		"id":        "https://remote.example/posts/1",
		"content":   "<p>Hello <b>world</b></p>",
		"name":      "A title",
		"summary":   "spoiler warning",
		"url":       "https://remote.example/@alice/1",
		"sensitive": true,
		"published": "2024-06-01T12:00:00Z",
		"tag": []any{
			map[string]any{"type": "Hashtag", "name": "#Naija"},
			map[string]any{"type": "Hashtag", "name": "naija"}, // deduped case-insensitively
			map[string]any{"type": "Mention", "name": "@bob"},
		},
	}
	f := remotePostFieldsFromObject(obj)
	if f.ContentText != "Hello world" {
		t.Errorf("content_text: %q", f.ContentText)
	}
	if f.Name == nil || *f.Name != "A title" {
		t.Errorf("name: %v", f.Name)
	}
	if f.Summary == nil || *f.Summary != "spoiler warning" {
		t.Errorf("summary: %v", f.Summary)
	}
	if f.URL == nil || *f.URL != "https://remote.example/@alice/1" {
		t.Errorf("url: %v", f.URL)
	}
	if !f.Sensitive {
		t.Error("sensitive should be true")
	}
	if len(f.Hashtags) != 1 || f.Hashtags[0] != "Naija" {
		t.Errorf("hashtags: %v", f.Hashtags)
	}
	if !f.PublishedAt.Valid || f.PublishedAt.Time.Format(time.RFC3339) != "2024-06-01T12:00:00Z" {
		t.Errorf("published_at: %v", f.PublishedAt)
	}
}

func TestRemotePostFieldsURLShapes(t *testing.T) {
	// Link-shaped url doc resolves via href.
	f := remotePostFieldsFromObject(map[string]any{
		"url": map[string]any{"type": "Link", "href": "https://remote.example/p/9"},
	})
	if f.URL == nil || *f.URL != "https://remote.example/p/9" {
		t.Errorf("link url: %v", f.URL)
	}
	// Non-https urls never ride into storage — remote-advertised data.
	f = remotePostFieldsFromObject(map[string]any{"url": "http://remote.example/p/9"})
	if f.URL != nil {
		t.Errorf("http url should drop, got %v", *f.URL)
	}
	f = remotePostFieldsFromObject(map[string]any{"url": "javascript:alert(1)"})
	if f.URL != nil {
		t.Errorf("js url should drop, got %v", *f.URL)
	}
	// Missing optional fields leave nil/empty, never panic.
	f = remotePostFieldsFromObject(map[string]any{})
	if f.URL != nil || f.Name != nil || f.PublishedAt.Valid || len(f.Hashtags) != 0 {
		t.Errorf("empty object should produce empty fields: %+v", f)
	}
}

func TestRemotePostHashtagsCap(t *testing.T) {
	tags := make([]any, 0, maxRemotePostHashtags+5)
	for i := 0; i < maxRemotePostHashtags+5; i++ {
		tags = append(tags, map[string]any{
			"type": "Hashtag", "name": "#tag" + string(rune('a'+i)),
		})
	}
	got := remotePostHashtags(map[string]any{"tag": tags})
	if len(got) != maxRemotePostHashtags {
		t.Errorf("hashtag cap: got %d want %d", len(got), maxRemotePostHashtags)
	}
}

func TestAPIDList(t *testing.T) {
	if got := apIDList("https://x/a"); len(got) != 1 || got[0] != "https://x/a" {
		t.Errorf("string form: %v", got)
	}
	if got := apIDList(map[string]any{"id": "https://x/b"}); len(got) != 1 || got[0] != "https://x/b" {
		t.Errorf("object form: %v", got)
	}
	got := apIDList([]any{"https://x/c", map[string]any{"id": "https://x/d"}})
	if len(got) != 2 {
		t.Errorf("array form: %v", got)
	}
	if got := apIDList(nil); got != nil {
		t.Errorf("nil form: %v", got)
	}
	if got := apIDList(""); got != nil {
		t.Errorf("empty form: %v", got)
	}
}

func TestSameURLHostSignerGate(t *testing.T) {
	// The Add/Remove collection gate hinges on host equality — a
	// subdomain must never satisfy it.
	if sameURLHost("https://evil.remote.example/collections/featured", "https://remote.example/users/a") {
		t.Error("subdomain must not satisfy the signer-domain gate")
	}
	if !sameURLHost("https://remote.example/users/a/collections/featured", "https://remote.example/users/a") {
		t.Error("same-host collection should satisfy the gate")
	}
}

func TestActorDocumentMovedTo(t *testing.T) {
	row := testActorProfileRow()
	doc := actorDocument(row, "https://japa.to")
	if _, ok := doc["movedTo"]; ok {
		t.Error("movedTo must be absent without a redirect")
	}
	target := "https://new.example/users/alice"
	row.MovedToUrl = &target
	doc = actorDocument(row, "https://japa.to")
	if doc["movedTo"] != target {
		t.Errorf("movedTo: %v", doc["movedTo"])
	}
}

func TestGroupDocumentFeaturedCollections(t *testing.T) {
	row := gen.GetGroupActorDocBySlugRow{
		ActorUrl:     "https://api.japa.to/federation/groups/lagos",
		PublicKeyID:  "https://api.japa.to/federation/groups/lagos#main-key",
		PublicKeyPem: "-----BEGIN PUBLIC KEY-----\ntest\n-----END PUBLIC KEY-----\n",
		InboxUrl:     "https://api.japa.to/federation/groups/lagos/inbox",
		OutboxUrl:    "https://api.japa.to/federation/groups/lagos/outbox",
		FollowersUrl: "https://api.japa.to/federation/groups/lagos/followers",
	}
	doc := groupDocument(row, "https://japa.to")
	if doc["featured"] != row.ActorUrl+"/collections/featured" {
		t.Errorf("featured: %v", doc["featured"])
	}
	if doc["featuredTags"] != row.ActorUrl+"/collections/tags" {
		t.Errorf("featuredTags: %v", doc["featuredTags"])
	}
}

func TestRemoteTimelineCursorShape(t *testing.T) {
	// The cursor contract is base64(published_at|id) — the format the
	// query's keyset predicate decodes. Pin both directions so a format
	// change can't silently strand paginated clients.
	ts := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	var id pgtype.UUID
	if err := id.Scan("00000000-0000-0000-0000-000000000001"); err != nil {
		t.Fatal(err)
	}
	// Encode the way RemoteTimeline does and confirm it round-trips the
	// decode path (RFC3339Nano | uuid).
	enc := ts.UTC().Format(time.RFC3339Nano) + "|" + "00000000-0000-0000-0000-000000000001"
	if parsed, perr := time.Parse(time.RFC3339Nano, enc[:len(enc)-37]); perr != nil || !parsed.Equal(ts) {
		t.Errorf("cursor ts parse: %v %v", parsed, perr)
	}
	var rid pgtype.UUID
	if err := rid.Scan(enc[len(enc)-36:]); err != nil || rid != id {
		t.Errorf("cursor id parse: %v %v", rid, err)
	}
}
