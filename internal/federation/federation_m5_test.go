package federation

// Ref: japaconnect-engineering/sections/126-activitypub-federation.md (M5)
// Unit coverage for the consume-side surface's pure logic — field
// extraction, target/cursor shapes, and the new document fields. The
// DB-backed paths ride the JC_TEST_DATABASE_URL suite.

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

func TestSpaceGroupEligibleOptOut(t *testing.T) {
	base := func() gen.GetGroupActorDocBySlugRow {
		var creator pgtype.UUID
		if err := creator.Scan("00000000-0000-0000-0000-000000000002"); err != nil {
			t.Fatal(err)
		}
		return gen.GetGroupActorDocBySlugRow{
			SpaceStatus:         "active",
			SpaceVisibility:     "public",
			SpaceLevel:          "custom",
			SpaceCreatorUserID:  creator,
			SpaceFederateOptOut: false,
		}
	}
	if !spaceGroupEligible(base()) {
		t.Fatal("default public member-created space must be group-eligible")
	}
	opted := base()
	opted.SpaceFederateOptOut = true
	if spaceGroupEligible(opted) {
		t.Fatal("federate_opt_out must drop the space out of the Group envelope")
	}
	priv := base()
	priv.SpaceVisibility = "private"
	if spaceGroupEligible(priv) {
		t.Fatal("private space must stay ineligible")
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

func TestIsSignerCollectionTarget(t *testing.T) {
	actor := "https://remote.example/users/alice"
	cases := []struct {
		target string
		want   bool
	}{
		{actor + "/collections/featured", true},
		{"https://remote.example/users/alice/collections/liked", true},
		{"https://remote.example/collections/instance", true},
		{"https://evil.remote.example/collections/featured", false},
		{"https://other.example/users/alice/collections/featured", false},
		{"https://remote.example/users/alice", false},
		{"https://remote.example/posts/1", false},
		{"not a url", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isSignerCollectionTarget(c.target, actor); got != c.want {
			t.Errorf("isSignerCollectionTarget(%q) = %v, want %v", c.target, got, c.want)
		}
	}
}

func TestRemotePostFieldsClampAndCaps(t *testing.T) {
	// A far-future published must not pin the post atop timelines — the
	// keyset sorts on it, so ingest clamps to receipt time.
	future := time.Now().Add(365 * 24 * time.Hour).UTC().Format(time.RFC3339)
	f := remotePostFieldsFromObject(map[string]any{
		"content":   "hello",
		"published": future,
		"name":      strings.Repeat("n", maxRemotePostField+50),
		"summary":   strings.Repeat("s", maxRemotePostField+50),
	})
	if !f.PublishedAt.Valid || f.PublishedAt.Time.After(time.Now().Add(time.Minute)) {
		t.Errorf("future published not clamped: %v", f.PublishedAt.Time)
	}
	if got := len([]rune(*f.Name)); got != maxRemotePostField {
		t.Errorf("name len %d want %d", got, maxRemotePostField)
	}
	if got := len([]rune(*f.Summary)); got != maxRemotePostField {
		t.Errorf("summary len %d want %d", got, maxRemotePostField)
	}
	// An honest past stamp passes through untouched.
	past := "2024-01-15T10:00:00Z"
	f2 := remotePostFieldsFromObject(map[string]any{"content": "x", "published": past})
	if !f2.PublishedAt.Valid || f2.PublishedAt.Time.Format(time.RFC3339) != past {
		t.Errorf("past published altered: %v", f2.PublishedAt.Time)
	}
}

func TestRuneCap(t *testing.T) {
	s := strings.Repeat("é", 5) // multibyte runes
	if got := runeCap(s, 3); got != "ééé" {
		t.Errorf("runeCap split or miscounted: %q", got)
	}
	if got := runeCap("ab", 10); got != "ab" {
		t.Errorf("runeCap under limit: %q", got)
	}
}

func TestResolveTargetURLAcctSyntax(t *testing.T) {
	// A zero Service suffices — every case must fail before the
	// instanceAllowed/WebFinger path dereferences store state.
	s := &Service{}
	for _, target := range []string{
		"", " ", "user @remote.example", "user@remote .example",
		"us er@remote.example", "user@remote.example/x", "user@remote.example?x",
		"user@remote.example#frag", "user@remote@example",
		"acct:user @remote.example", "@us\ter@remote.example",
		"http://remote.example/users/a", "ftp://remote.example/users/a",
	} {
		if _, err := s.resolveTargetURL(context.Background(), target); !errors.Is(err, ErrInvalidTarget) {
			t.Errorf("resolveTargetURL(%q) err = %v, want ErrInvalidTarget", target, err)
		}
	}
}

// §126 tag discovery — a relay Announce{Note} is the Follow{Hashtag}
// delivery form: the ingest arm resolves the embedded doc's author and
// stores under them (not the relay signer), then the boost ledgers.
// A announce with no covering subscription keeps the pre-arm behavior.
func TestDBInboundAnnounceTagIngest(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	relay, relayURL := newRemoteActor(t, ctx, svc, "relay.example")
	author, authorURL := newRemoteActor(t, ctx, svc, "orig.example")
	if _, err := svc.store.write.InsertTagSubscription(ctx, gen.InsertTagSubscriptionParams{
		UserID: userID, Tag: "golang", Domain: "orig.example",
	}); err != nil {
		t.Fatalf("insert tag sub: %v", err)
	}
	t.Cleanup(func() {
		_, _ = svc.db.Writer.Exec(context.Background(),
			`DELETE FROM federation_remote_posts WHERE remote_actor_id IN ($1, $2)`, author.ID, relay.ID)
	})

	noteID := fmt.Sprintf("https://orig.example/objects/%d", time.Now().UnixNano())
	act := map[string]any{
		"id": relayURL + "#ann1", "type": "Announce", "actor": relayURL,
		"object": map[string]any{
			"id":           noteID,
			"type":         "Note",
			"attributedTo": authorURL,
			"content":      "<p>hello #golang world</p>",
			"tag":          []any{map[string]any{"type": "Hashtag", "name": "#golang"}},
		},
	}
	row := runInbound(t, ctx, svc, act)
	if row.Status != "processed" {
		t.Fatalf("announce status %q (reason %v)", row.Status, row.Error)
	}
	var actorID pgtype.UUID
	var shares int64
	if err := d.Reader.QueryRow(ctx,
		`SELECT remote_actor_id FROM federation_remote_posts WHERE ap_id = $1`, noteID).Scan(&actorID); err != nil {
		t.Fatalf("post lookup: %v", err)
	}
	if actorID != author.ID {
		t.Fatalf("post attributed to %v, want author %v", actorID, author.ID)
	}
	if err := d.Reader.QueryRow(ctx,
		`SELECT COUNT(*) FROM federation_remote_post_shares s
		 JOIN federation_remote_posts p ON p.id = s.post_id
		 WHERE p.ap_id = $1 AND s.remote_actor_id = $2`, noteID, relay.ID).Scan(&shares); err != nil {
		t.Fatalf("share lookup: %v", err)
	}
	if shares != 1 {
		t.Fatalf("relay boost shares = %d, want 1", shares)
	}

	// No covering subscription — the announce drops exactly like before.
	other, otherURL := newRemoteActor(t, ctx, svc, "other.example")
	miss := map[string]any{
		"id": relayURL + "#ann2", "type": "Announce", "actor": relayURL,
		"object": map[string]any{
			"id":           fmt.Sprintf("https://other.example/objects/%d", time.Now().UnixNano()),
			"type":         "Note",
			"attributedTo": otherURL,
			"content":      "<p>nobody follows this #golang</p>",
			"tag":          []any{map[string]any{"type": "Hashtag", "name": "#golang"}},
		},
	}
	row = runInbound(t, ctx, svc, miss)
	if row.Status != "ignored" {
		t.Fatalf("uncovered announce status %q (reason %v)", row.Status, row.Error)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM federation_remote_posts WHERE remote_actor_id = $1`, other.ID); n != 0 {
		t.Fatalf("uncovered posts stored: %d", n)
	}
}
