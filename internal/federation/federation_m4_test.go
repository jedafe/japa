package federation

// M4 tests — OStatus subscribe link, Update{Person} fanout on identity
// edits, featured/featuredTags collections, inbound Move{actor→target}
// with alsoKnownAs verification (§126).

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// featured / featuredTags
// ---------------------------------------------------------------------------

// seedFeaturedContent inserts one contents row with the given pin/opt-out
// profile and returns its id.
func seedContentRow(t *testing.T, ctx context.Context, svc *Service, authorID interface{ String() string }, pinned, optOut bool, visibility string) string {
	t.Helper()
	var id string
	err := svc.db.Writer.QueryRow(ctx, `
		INSERT INTO contents (author_id, content_type, title, slug, body, body_html,
		                      content_hash, status, visibility, is_pinned, federate_opt_out, published_at)
		VALUES ($1, 'story', $2, $3, 'hi', '<p>hi</p>', $4, 'published', $5, $6, $7, now())
		RETURNING id::text`,
		authorID, fmt.Sprintf("pin-%d", time.Now().UnixNano()), fmt.Sprintf("m4-%d", time.Now().UnixNano()),
		fmt.Sprintf("m4hash-%d", time.Now().UnixNano()), visibility, pinned, optOut).Scan(&id)
	if err != nil {
		t.Fatalf("insert content: %v", err)
	}
	t.Cleanup(func() {
		_, _ = svc.db.Writer.Exec(context.Background(), `DELETE FROM contents WHERE id::text = $1`, id)
	})
	return id
}

func TestDBFeaturedCollection(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}

	// Only the pinned+eligible row may appear — the same gates as the outbox.
	seedContentRow(t, ctx, svc, userID, true, false, "public")
	seedContentRow(t, ctx, svc, userID, false, false, "public") // unpinned → excluded
	seedContentRow(t, ctx, svc, userID, true, true, "public")   // opted out → excluded
	seedContentRow(t, ctx, svc, userID, true, false, "private") // non-public → excluded

	doc, err := svc.Featured(ctx, uname)
	if err != nil {
		t.Fatalf("Featured: %v", err)
	}
	items, _ := doc["orderedItems"].([]any)
	if len(items) != 1 {
		t.Fatalf("featured items: %d", len(items))
	}
	if doc["totalItems"] != len(items) {
		t.Fatalf("totalItems %v != items %d", doc["totalItems"], len(items))
	}
	obj, _ := items[0].(map[string]any)
	if obj["attributedTo"] == "" || obj["id"] == "" {
		t.Fatalf("featured item is not an object doc: %v", obj)
	}
	// featured items are raw objects (Mastodon shape), not Create wrappers.
	if obj["type"] == "Create" {
		t.Fatalf("featured item wrapped in Create: %v", obj["type"])
	}
}

func TestDBFeaturedTagsCollection(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}

	tagID := fmt.Sprintf("%d", time.Now().UnixNano())
	var tid string
	if err := svc.db.Writer.QueryRow(ctx,
		`INSERT INTO tags (name, slug) VALUES ($1, $2) RETURNING id::text`,
		"m4tag"+tagID[len(tagID)-6:], "m4tag-"+tagID).Scan(&tid); err != nil {
		t.Fatalf("insert tag: %v", err)
	}
	t.Cleanup(func() {
		_, _ = svc.db.Writer.Exec(context.Background(), `DELETE FROM tags WHERE id::text = $1`, tid)
	})
	var optOutTag string
	if err := svc.db.Writer.QueryRow(ctx,
		`INSERT INTO tags (name, slug) VALUES ($1, $2) RETURNING id::text`,
		"m4tagx"+tagID[len(tagID)-6:], "m4tagx-"+tagID).Scan(&optOutTag); err != nil {
		t.Fatalf("insert tag2: %v", err)
	}
	t.Cleanup(func() {
		_, _ = svc.db.Writer.Exec(context.Background(), `DELETE FROM tags WHERE id::text = $1`, optOutTag)
	})

	eligible := seedContentRow(t, ctx, svc, userID, false, false, "public")
	optedOut := seedContentRow(t, ctx, svc, userID, false, true, "public")
	for contentID, tag := range map[string]string{eligible: tid, optedOut: optOutTag} {
		if _, err := svc.db.Writer.Exec(ctx,
			`INSERT INTO content_tags (content_id, tag_id) VALUES ($1, $2)`, contentID, tag); err != nil {
			t.Fatalf("insert content_tag: %v", err)
		}
	}

	doc, err := svc.FeaturedTags(ctx, uname)
	if err != nil {
		t.Fatalf("FeaturedTags: %v", err)
	}
	items, _ := doc["orderedItems"].([]any)
	if len(items) != 1 {
		t.Fatalf("featuredTags items: %d", len(items))
	}
	entry, _ := items[0].(map[string]any)
	if entry["type"] != "Hashtag" || entry["name"] == "" || entry["href"] == "" {
		t.Fatalf("malformed Hashtag item: %v", entry)
	}
	if entry["name"] != "#m4tag"+tagID[len(tagID)-6:] {
		t.Fatalf("unexpected tag name: %v", entry["name"])
	}
}

func TestDBActorDocFeaturedURLs(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	doc, err := svc.ActorDocument(ctx, uname)
	if err != nil {
		t.Fatalf("ActorDocument: %v", err)
	}
	actorURL := svc.apiBase() + "/federation/actors/" + uname
	if doc["featured"] != actorURL+"/collections/featured" {
		t.Fatalf("featured url: %v", doc["featured"])
	}
	if doc["featuredTags"] != actorURL+"/collections/tags" {
		t.Fatalf("featuredTags url: %v", doc["featuredTags"])
	}
}

// ---------------------------------------------------------------------------
// Update{Person} fanout on identity edits
// ---------------------------------------------------------------------------

// FanoutActorUpdateForUser is the §126 M4 seam the users service calls
// after doc-rendered identity edits — the update must land as a delivery
// row per remote follower inbox, carrying the fresh actor document.
func TestDBFanoutActorUpdateForUser(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	remote, actorURL := newRemoteActor(t, ctx, svc, "remote.example")
	actorIRI := svc.apiBase() + "/federation/actors/" + uname

	// Land an accepted remote follower edge first — Update fanout expands
	// the same inbox set as content fanout.
	follow := map[string]any{
		"id": actorURL + "#follow-pre-update", "type": "Follow",
		"actor": actorURL, "object": actorIRI,
	}
	if row := runInbound(t, ctx, svc, follow); row.Status != "processed" {
		t.Fatalf("follow status %q", row.Status)
	}

	svc.FanoutActorUpdateForUser(ctx, userID)

	var typ, inbox string
	err := svc.db.Reader.QueryRow(ctx, `
		SELECT activity_type, inbox_url FROM federation_deliveries
		WHERE remote_actor_id = $1 AND activity_type = 'Update'`, remote.ID).Scan(&typ, &inbox)
	if err != nil {
		t.Fatalf("update delivery missing: %v", err)
	}
	if inbox != "https://remote.example/inbox" {
		t.Fatalf("update inbox: %q", inbox)
	}
	var obj map[string]any
	if err := svc.db.Reader.QueryRow(ctx, `
		SELECT activity_json->'object' FROM federation_deliveries
		WHERE remote_actor_id = $1 AND activity_type = 'Update'`, remote.ID).Scan(&obj); err != nil {
		t.Fatalf("update object: %v", err)
	}
	if obj["id"] != actorIRI || obj["type"] != "Person" {
		t.Fatalf("update object is not the actor doc: %v", obj)
	}
	// The featured URLs ride along in the pushed doc.
	if obj["featured"] == "" || obj["featuredTags"] == "" {
		t.Fatalf("pushed doc missing collections: %v", obj)
	}

	// A SECOND identity edit must produce a SECOND delivery — the ap_id
	// keys on wall-clock nanos, not the actor row's updated_at, so
	// back-to-back edits don't dedup-collapse.
	svc.FanoutActorUpdateForUser(ctx, userID)
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM federation_deliveries WHERE remote_actor_id = $1 AND activity_type = 'Update'`, remote.ID); n != 2 {
		t.Fatalf("second update fanout collapsed: %d rows", n)
	}

	// Unfederated user → no-op, zero-delta on the ledger (other suites
	// write Update rows too, so assert no NEW rows rather than a count).
	otherID, _ := newFederationUser(t, ctx, d, "active")
	before := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM federation_deliveries WHERE activity_type = 'Update'`)
	svc.FanoutActorUpdateForUser(ctx, otherID)
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM federation_deliveries WHERE activity_type = 'Update'`); n != before {
		t.Fatalf("unfederated fanout created rows: %d → %d", before, n)
	}
}

// ---------------------------------------------------------------------------
// Move{actor→target}
// ---------------------------------------------------------------------------

// moveDoc builds a target actor doc — alsoKnownAs drives the consent check.
func moveTargetDoc(targetURL string, alsoKnownAs []string) map[string]any {
	doc := map[string]any{
		"id":                targetURL,
		"type":              "Person",
		"preferredUsername": "newalice",
		"inbox":             "https://target.test/inbox",
		"publicKey": map[string]any{
			"id":           targetURL + "#main-key",
			"owner":        targetURL,
			"publicKeyPem": "-----BEGIN PUBLIC KEY-----\nTARGET\n-----END PUBLIC KEY-----",
		},
	}
	if alsoKnownAs != nil {
		doc["alsoKnownAs"] = alsoKnownAs
	}
	return doc
}

// A verified Move repoints the mover's federation_follows edges and the
// native follows mirror onto the target actor, then tombstones the old
// URL — fanout to the member must now reach the new inbox.
func TestDBInboundMoveRepointsFollows(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	old, oldURL := newRemoteActor(t, ctx, svc, "remote.example")
	actorIRI := svc.apiBase() + "/federation/actors/" + uname

	follow := map[string]any{
		"id": oldURL + "#follow-1", "type": "Follow",
		"actor": oldURL, "object": actorIRI,
	}
	if row := runInbound(t, ctx, svc, follow); row.Status != "processed" {
		t.Fatalf("follow status %q", row.Status)
	}

	targetURL := fmt.Sprintf("https://target.test/actors/new%d", time.Now().UnixNano())
	srv := httptest.NewTLSServer(actorDocHandler(moveTargetDoc(targetURL, []string{oldURL})))
	defer srv.Close()
	swapFetchTransport(t, map[string]string{"target.test": tlsAddr(srv)})

	// A queued delivery to the old inbox must deaden with the move; a
	// delivered row is history and stays.
	if _, err := svc.db.Writer.Exec(ctx, `
		INSERT INTO federation_deliveries (activity_ap_id, activity_type, activity_json, inbox_url, remote_actor_id, status)
		VALUES ($1, 'Create', '{}', $2, $3, 'pending'), ($4, 'Create', '{}', $2, $3, 'delivered')`,
		oldURL+"#dlv-p", "https://remote.example/inbox", old.ID, oldURL+"#dlv-d"); err != nil {
		t.Fatalf("seed deliveries: %v", err)
	}

	move := map[string]any{
		"id": oldURL + "#move-1", "type": "Move",
		"actor": oldURL, "object": oldURL, "target": targetURL,
	}
	row := runInbound(t, ctx, svc, move)
	if row.Status != "processed" {
		t.Fatalf("move status %q (reason %v)", row.Status, row.Error)
	}

	// Old actor tombstoned; the new row resolved + edges repointed.
	var oldStatus, newID string
	if err := svc.db.Reader.QueryRow(ctx,
		`SELECT status FROM remote_actors WHERE id = $1`, old.ID).Scan(&oldStatus); err != nil {
		t.Fatalf("old status: %v", err)
	}
	if oldStatus != "tombstone" {
		t.Fatalf("old actor status: %q", oldStatus)
	}
	if err := svc.db.Reader.QueryRow(ctx,
		`SELECT id::text FROM remote_actors WHERE actor_url = $1`, targetURL).Scan(&newID); err != nil {
		t.Fatalf("target actor row: %v", err)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM federation_follows WHERE remote_actor_id = $1::uuid AND status = 'accepted'`, newID); n != 1 {
		t.Fatalf("edge not repointed to target: %d", n)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM federation_follows WHERE remote_actor_id = $1`, old.ID); n != 0 {
		t.Fatalf("edges left on old actor: %d", n)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM follows WHERE follower_remote_actor_id = $1::uuid`, newID); n != 1 {
		t.Fatalf("follows mirror not repointed: %d", n)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM follows WHERE follower_remote_actor_id = $1`, old.ID); n != 0 {
		t.Fatalf("mirror left on old actor: %d", n)
	}
	// Fanout inbox expansion must now reach the NEW inbox.
	var inbox string
	if err := svc.db.Reader.QueryRow(ctx, `
		SELECT COALESCE(ra.shared_inbox_url, ra.inbox_url)
		FROM federation_follows ff JOIN remote_actors ra ON ra.id = ff.remote_actor_id
		WHERE ff.remote_actor_id = $1::uuid`, newID).Scan(&inbox); err != nil {
		t.Fatalf("new inbox: %v", err)
	}
	if inbox != "https://target.test/inbox" {
		t.Fatalf("fanout inbox: %q", inbox)
	}
	// Pending delivery deadened (the seeded row — plus the follow's own
	// Accept, which can never land on a tombstoned inbox either).
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM federation_deliveries WHERE remote_actor_id = $1 AND status = 'dead' AND activity_ap_id = $2`,
		old.ID, oldURL+"#dlv-p"); n != 1 {
		t.Fatalf("pending delivery not deadened: %d", n)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM federation_deliveries WHERE remote_actor_id = $1 AND status = 'delivered'`, old.ID); n != 1 {
		t.Fatalf("delivered row touched: %d", n)
	}
}

// The target's alsoKnownAs is the ONLY evidence the migration is
// endorsed — a target that does not list the old actor rejects, and the
// old actor keeps its edges.
func TestDBInboundMoveRejectsUnconfirmedTarget(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	old, oldURL := newRemoteActor(t, ctx, svc, "remote.example")
	actorIRI := svc.apiBase() + "/federation/actors/" + uname
	follow := map[string]any{
		"id": oldURL + "#follow-1", "type": "Follow",
		"actor": oldURL, "object": actorIRI,
	}
	if row := runInbound(t, ctx, svc, follow); row.Status != "processed" {
		t.Fatalf("follow status %q", row.Status)
	}

	targetURL := fmt.Sprintf("https://target.test/actors/new%d", time.Now().UnixNano())
	srv := httptest.NewTLSServer(actorDocHandler(moveTargetDoc(targetURL, nil))) // no alsoKnownAs
	defer srv.Close()
	swapFetchTransport(t, map[string]string{"target.test": tlsAddr(srv)})

	move := map[string]any{
		"id": oldURL + "#move-1", "type": "Move",
		"actor": oldURL, "object": oldURL, "target": targetURL,
	}
	row := runInbound(t, ctx, svc, move)
	if row.Status != "rejected" {
		t.Fatalf("unconfirmed move status %q", row.Status)
	}
	var oldStatus string
	if err := svc.db.Reader.QueryRow(ctx,
		`SELECT status FROM remote_actors WHERE id = $1`, old.ID).Scan(&oldStatus); err != nil {
		t.Fatalf("old status: %v", err)
	}
	if oldStatus != "active" {
		t.Fatalf("old actor mutated on rejected move: %q", oldStatus)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM federation_follows WHERE remote_actor_id = $1 AND status = 'accepted'`, old.ID); n != 1 {
		t.Fatalf("edge moved on rejected move: %d", n)
	}
}

// Signer/object binding: a Move whose object is not the signing actor,
// or pointing at a local actor, rejects without any remote fetch.
func TestDBInboundMoveBinding(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	_, oldURL := newRemoteActor(t, ctx, svc, "remote.example")
	actorIRI := svc.apiBase() + "/federation/actors/" + uname

	// object != actor — the moving actor must move itself.
	row := runInbound(t, ctx, svc, map[string]any{
		"id": oldURL + "#move-bad-1", "type": "Move",
		"actor": oldURL, "object": "https://victim.example/actors/x",
		"target": fmt.Sprintf("https://target.test/actors/new%d", time.Now().UnixNano()),
	})
	if row.Status != "rejected" {
		t.Fatalf("foreign-object move status %q", row.Status)
	}

	// A Move can't migrate INTO us — the target must be a remote actor.
	row = runInbound(t, ctx, svc, map[string]any{
		"id": oldURL + "#move-bad-2", "type": "Move",
		"actor": oldURL, "object": oldURL, "target": actorIRI,
	})
	if row.Status != "rejected" {
		t.Fatalf("local-target move status %q", row.Status)
	}
}

// When the member was followed from BOTH accounts, the new-actor edge
// wins: the old edge cannot repoint over the unique pair and goes undone.
func TestDBInboundMoveCollision(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	old, oldURL := newRemoteActor(t, ctx, svc, "remote.example")
	actorIRI := svc.apiBase() + "/federation/actors/" + uname
	targetURL := fmt.Sprintf("https://target.test/actors/new%d", time.Now().UnixNano())

	srv := httptest.NewTLSServer(actorDocHandler(moveTargetDoc(targetURL, []string{oldURL})))
	defer srv.Close()
	swapFetchTransport(t, map[string]string{"target.test": tlsAddr(srv)})

	// Both accounts already follow the member.
	for i, u := range []string{oldURL, targetURL} {
		follow := map[string]any{
			"id": u + fmt.Sprintf("#follow-%d", i), "type": "Follow",
			"actor": u, "object": actorIRI,
		}
		if row := runInbound(t, ctx, svc, follow); row.Status != "processed" {
			t.Fatalf("follow %d status %q", i, row.Status)
		}
	}
	var newID string
	if err := svc.db.Reader.QueryRow(ctx,
		`SELECT id::text FROM remote_actors WHERE actor_url = $1`, targetURL).Scan(&newID); err != nil {
		t.Fatalf("target actor row: %v", err)
	}

	row := runInbound(t, ctx, svc, map[string]any{
		"id": oldURL + "#move-1", "type": "Move",
		"actor": oldURL, "object": oldURL, "target": targetURL,
	})
	if row.Status != "processed" {
		t.Fatalf("move status %q (reason %v)", row.Status, row.Error)
	}
	// Exactly one accepted edge — on the NEW actor; the merged leftover
	// deletes (post-merge old edges are redundant history, not unfollows).
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM federation_follows WHERE remote_actor_id = $1::uuid AND status = 'accepted'`, newID); n != 1 {
		t.Fatalf("new-actor accepted edges: %d", n)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM federation_follows WHERE remote_actor_id = $1`, old.ID); n != 0 {
		t.Fatalf("old-actor edges left after merge: %d", n)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM follows WHERE follower_remote_actor_id = $1::uuid`, newID); n != 1 {
		t.Fatalf("mirror on new actor: %d", n)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM follows WHERE follower_remote_actor_id = $1`, old.ID); n != 0 {
		t.Fatalf("duplicate mirror left: %d", n)
	}
	// The two accounts merged into one follower — follower_count must
	// drop from 2 to 1 with the deleted duplicate mirror.
	var fcount int
	if err := svc.db.Reader.QueryRow(ctx,
		`SELECT follower_count FROM users WHERE id = $1`, userID).Scan(&fcount); err != nil {
		t.Fatalf("follower_count: %v", err)
	}
	if fcount != 1 {
		t.Fatalf("follower_count after merge: %d (want 1)", fcount)
	}
}

// The sharpest collision: the new account followed then UNFOLLOWED (edge
// 'undone') while the old still follows — the person's live state is a
// follower, so the Move must resurrect the new edge as 'accepted', not
// undo the old one and lose the follower entirely.
func TestDBInboundMoveUndoneEdgeMerges(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	old, oldURL := newRemoteActor(t, ctx, svc, "remote.example")
	actorIRI := svc.apiBase() + "/federation/actors/" + uname
	targetURL := fmt.Sprintf("https://target.test/actors/new%d", time.Now().UnixNano())

	srv := httptest.NewTLSServer(actorDocHandler(moveTargetDoc(targetURL, []string{oldURL})))
	defer srv.Close()
	swapFetchTransport(t, map[string]string{"target.test": tlsAddr(srv)})

	// old follows; new follows then undoes its own follow.
	for _, act := range []map[string]any{
		{"id": oldURL + "#follow-1", "type": "Follow", "actor": oldURL, "object": actorIRI},
		{"id": targetURL + "#follow-1", "type": "Follow", "actor": targetURL, "object": actorIRI},
		{"id": targetURL + "#undo-1", "type": "Undo", "actor": targetURL,
			"object": map[string]any{"type": "Follow", "actor": targetURL, "object": actorIRI}},
	} {
		if row := runInbound(t, ctx, svc, act); row.Status != "processed" {
			t.Fatalf("%s status %q (reason %v)", act["type"], row.Status, row.Error)
		}
	}
	var newID string
	if err := svc.db.Reader.QueryRow(ctx,
		`SELECT id::text FROM remote_actors WHERE actor_url = $1`, targetURL).Scan(&newID); err != nil {
		t.Fatalf("target actor row: %v", err)
	}
	var fcount int
	if err := svc.db.Reader.QueryRow(ctx,
		`SELECT follower_count FROM users WHERE id = $1`, userID).Scan(&fcount); err != nil {
		t.Fatalf("pre-move follower_count: %v", err)
	}
	if fcount != 1 {
		t.Fatalf("pre-move follower_count: %d (want 1)", fcount)
	}

	row := runInbound(t, ctx, svc, map[string]any{
		"id": oldURL + "#move-1", "type": "Move",
		"actor": oldURL, "object": oldURL, "target": targetURL,
	})
	if row.Status != "processed" {
		t.Fatalf("move status %q (reason %v)", row.Status, row.Error)
	}
	// The live edge must SURVIVE on the new actor — 'undone' + live
	// merge = accepted, not undone+undone = lost follower.
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM federation_follows WHERE remote_actor_id = $1::uuid AND status = 'accepted'`, newID); n != 1 {
		t.Fatalf("merged edge not accepted: %d", n)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM federation_follows WHERE remote_actor_id = $1`, old.ID); n != 0 {
		t.Fatalf("old edges left: %d", n)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM follows WHERE follower_remote_actor_id = $1::uuid`, newID); n != 1 {
		t.Fatalf("mirror on new actor: %d", n)
	}
	if err := svc.db.Reader.QueryRow(ctx,
		`SELECT follower_count FROM users WHERE id = $1`, userID).Scan(&fcount); err != nil {
		t.Fatalf("post-move follower_count: %v", err)
	}
	if fcount != 1 {
		t.Fatalf("post-move follower_count: %d (want 1)", fcount)
	}
}

// A replayed/new-id Move after the first can't resurrect the old actor —
// the tombstoned signer rejects at resolution, state stays repointed.
func TestDBInboundMoveSecondMoveRejected(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	_, oldURL := newRemoteActor(t, ctx, svc, "remote.example")
	actorIRI := svc.apiBase() + "/federation/actors/" + uname
	targetURL := fmt.Sprintf("https://target.test/actors/new%d", time.Now().UnixNano())

	follow := map[string]any{
		"id": oldURL + "#follow-1", "type": "Follow",
		"actor": oldURL, "object": actorIRI,
	}
	if row := runInbound(t, ctx, svc, follow); row.Status != "processed" {
		t.Fatalf("follow status %q", row.Status)
	}

	srv := httptest.NewTLSServer(actorDocHandler(moveTargetDoc(targetURL, []string{oldURL})))
	defer srv.Close()
	swapFetchTransport(t, map[string]string{"target.test": tlsAddr(srv)})

	move := map[string]any{
		"id": oldURL + "#move-1", "type": "Move",
		"actor": oldURL, "object": oldURL, "target": targetURL,
	}
	if row := runInbound(t, ctx, svc, move); row.Status != "processed" {
		t.Fatalf("first move status %q", row.Status)
	}

	// A second Move from the same old URL under a new ap_id — the old
	// actor is tombstoned, so signer resolution rejects it.
	row := runInbound(t, ctx, svc, map[string]any{
		"id": oldURL + "#move-2", "type": "Move",
		"actor": oldURL, "object": oldURL, "target": targetURL,
	})
	if row.Status != "rejected" {
		t.Fatalf("second move status %q", row.Status)
	}
	var newID string
	if err := svc.db.Reader.QueryRow(ctx,
		`SELECT id::text FROM remote_actors WHERE actor_url = $1`, targetURL).Scan(&newID); err != nil {
		t.Fatalf("target row: %v", err)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM federation_follows WHERE remote_actor_id = $1::uuid AND status = 'accepted'`, newID); n != 1 {
		t.Fatalf("edge lost on second move: %d", n)
	}
}
