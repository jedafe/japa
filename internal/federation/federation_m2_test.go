package federation

// §126 M2 DB integration tests — the inbound dispatch pipeline end-to-end
// against real Postgres: signature-trusted rows from federation_inbound
// through tx dispatch into domain rows, ledger statuses, and counter
// convergence. Remote actors seed via direct SQL (status='active',
// fresh fetched_at) so resolveRemoteActor takes the store fast path —
// no network in tests.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	gen "github.com/jedafe/japa/internal/db/gen"
)

// newRemoteActor seeds an active remote_actors row on `domain`.
func newRemoteActor(t *testing.T, ctx context.Context, svc *Service, domain string) (gen.RemoteActor, string) {
	t.Helper()
	actorURL := fmt.Sprintf("https://%s/actors/%d", domain, time.Now().UnixNano())
	row, err := svc.store.upsertRemoteActor(ctx, gen.UpsertRemoteActorParams{
		ActorUrl:          actorURL,
		Acct:              strPtr("alice@" + domain),
		Domain:            domain,
		InboxUrl:          strPtr("https://" + domain + "/inbox"),
		PublicKeyPem:      strPtr("-----BEGIN PUBLIC KEY-----\nfake\n-----END PUBLIC KEY-----"),
		PublicKeyID:       strPtr(actorURL + "#main-key"),
		PreferredUsername: strPtr("alice"),
		DisplayName:       strPtr("Alice Remote"),
	})
	if err != nil {
		t.Fatalf("seed remote actor: %v", err)
	}
	t.Cleanup(func() {
		// comments.remote_ap_actor_id carries no cascade — remote comments
		// must go before the actor row.
		_, _ = svc.db.Writer.Exec(context.Background(), `DELETE FROM comments WHERE remote_ap_actor_id = $1`, row.ID)
		_, _ = svc.db.Writer.Exec(context.Background(), `DELETE FROM remote_actors WHERE id = $1`, row.ID)
	})
	return row, actorURL
}

func strPtr(s string) *string { return &s }

// newFederatedContent seeds a published, public, federatable story for the
// member and returns its id + AP object URL.
func newFederatedContent(t *testing.T, ctx context.Context, svc *Service, userID pgtype.UUID) (pgtype.UUID, string) {
	t.Helper()
	var id pgtype.UUID
	err := svc.db.Writer.QueryRow(ctx, `
		INSERT INTO contents (author_id, content_type, title, slug, body, body_html,
		                      content_hash, status, visibility, published_at)
		VALUES ($1, 'story', 'Fed target', $2, 'hi', '<p>hi</p>',
		        $3, 'published', 'public', now())
		RETURNING id`, userID, fmt.Sprintf("fed-m2-%d", time.Now().UnixNano()),
		fmt.Sprintf("hash-m2-%d", time.Now().UnixNano())).Scan(&id)
	if err != nil {
		t.Fatalf("insert content: %v", err)
	}
	t.Cleanup(func() {
		_, _ = svc.db.Writer.Exec(context.Background(), `DELETE FROM contents WHERE id = $1`, id)
	})
	return id, svc.apiBase() + "/federation/objects/contents/" + id.String()
}

// runInbound ledgers the activity and drives ProcessInbound on it. A
// replayed activity_ap_id dedups at the constraint — the helper returns
// the already-ledgered row the same way the handler returns 202.
func runInbound(t *testing.T, ctx context.Context, svc *Service, act map[string]any) gen.FederationInbound {
	t.Helper()
	payload, _ := json.Marshal(act)
	row, err := svc.store.insertInbound(ctx, gen.InsertFederationInboundParams{
		ActivityApID:   stringPtrOrNil(apStr(act["id"])),
		ActivityType:   apStr(act["type"]),
		ActorUrl:       stringPtrOrNil(apActorID(act)),
		SignatureValid: true,
		Payload:        payload,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			var existing gen.FederationInbound
			if qerr := svc.db.Reader.QueryRow(ctx,
				`SELECT id, status, COALESCE(error,'') FROM federation_inbound WHERE activity_ap_id = $1`,
				apStr(act["id"])).Scan(&existing.ID, &existing.Status, new(string)); qerr != nil {
				t.Fatalf("deduped inbound lookup: %v", qerr)
			}
			return existing
		}
		t.Fatalf("insert inbound: %v", err)
	}
	if err := svc.ProcessInbound(ctx, row.ID); err != nil {
		t.Fatalf("ProcessInbound: %v", err)
	}
	var status, reason string
	if err := svc.db.Reader.QueryRow(ctx,
		`SELECT status, COALESCE(error,'') FROM federation_inbound WHERE id = $1`, row.ID).Scan(&status, &reason); err != nil {
		t.Fatalf("read inbound status: %v", err)
	}
	row.Status = status
	if reason != "" {
		row.Error = &reason
	}
	return row
}

func countRows(t *testing.T, ctx context.Context, svc *Service, q string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := svc.db.Reader.QueryRow(ctx, q, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func TestDBInboundFollowAccepted(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	remote, actorURL := newRemoteActor(t, ctx, svc, "remote.example")

	act := map[string]any{
		"id":     actorURL + "#follow-1",
		"type":   "Follow",
		"actor":  actorURL,
		"object": svc.apiBase() + "/federation/actors/" + uname,
	}
	row := runInbound(t, ctx, svc, act)
	if row.Status != "processed" {
		t.Fatalf("inbound status %q (reason %v)", row.Status, row.Error)
	}

	// federation_follows edge accepted.
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM federation_follows WHERE remote_actor_id = $1 AND status = 'accepted'`, remote.ID); n != 1 {
		t.Fatalf("federation_follows accepted rows: %d", n)
	}
	// Native follows mirror with remote provenance.
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM follows WHERE follower_remote_actor_id = $1 AND entity_type = 'user'`, remote.ID); n != 1 {
		t.Fatalf("follows mirror rows: %d", n)
	}
	// Accept{Follow} ledgered for delivery.
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM federation_deliveries WHERE remote_actor_id = $1 AND activity_type = 'Accept'`, remote.ID); n != 1 {
		t.Fatalf("accept deliveries: %d", n)
	}

	// Replay converges: still one edge, one mirror, one delivery, no
	// second notification (rows-affected gates it).
	row2 := runInbound(t, ctx, svc, act)
	if row2.Status != "processed" {
		t.Fatalf("replay status %q", row2.Status)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM follows WHERE follower_remote_actor_id = $1`, remote.ID); n != 1 {
		t.Fatalf("mirror rows after replay: %d", n)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM federation_deliveries WHERE remote_actor_id = $1 AND activity_type = 'Accept'`, remote.ID); n != 1 {
		t.Fatalf("accept deliveries after replay: %d", n)
	}
}

func TestDBInboundUndoFollow(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	remote, actorURL := newRemoteActor(t, ctx, svc, "remote.example")
	actorIRI := svc.apiBase() + "/federation/actors/" + uname

	runInbound(t, ctx, svc, map[string]any{
		"id": actorURL + "#f1", "type": "Follow", "actor": actorURL, "object": actorIRI,
	})
	undo := map[string]any{
		"id":    actorURL + "#u1",
		"type":  "Undo",
		"actor": actorURL,
		"object": map[string]any{
			"id": actorURL + "#f1", "type": "Follow", "actor": actorURL, "object": actorIRI,
		},
	}
	row := runInbound(t, ctx, svc, undo)
	if row.Status != "processed" {
		t.Fatalf("undo status %q (reason %v)", row.Status, row.Error)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM federation_follows WHERE remote_actor_id = $1 AND status = 'undone'`, remote.ID); n != 1 {
		t.Fatalf("undone rows: %d", n)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM follows WHERE follower_remote_actor_id = $1`, remote.ID); n != 0 {
		t.Fatalf("mirror rows after undo: %d", n)
	}
}

func TestDBInboundLikeAnnounceUndo(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	contentID, objIRI := newFederatedContent(t, ctx, svc, userID)
	remote, actorURL := newRemoteActor(t, ctx, svc, "remote.example")

	like := map[string]any{
		"id": actorURL + "#l1", "type": "Like", "actor": actorURL, "object": objIRI,
	}
	row := runInbound(t, ctx, svc, like)
	if row.Status != "processed" {
		t.Fatalf("like status %q (reason %v)", row.Status, row.Error)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM reactions WHERE entity_type = 'content' AND entity_id = $1 AND remote_actor_id = $2`,
		contentID, remote.ID); n != 1 {
		t.Fatalf("remote reactions: %d", n)
	}
	// Replayed Like is ignored — no second reaction.
	like["id"] = actorURL + "#l2"
	row = runInbound(t, ctx, svc, like)
	if row.Status != "ignored" {
		t.Fatalf("replayed like status %q", row.Status)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM reactions WHERE entity_id = $1 AND remote_actor_id = $2`,
		contentID, remote.ID); n != 1 {
		t.Fatalf("reactions after replay: %d", n)
	}

	boost := map[string]any{
		"id": actorURL + "#b1", "type": "Announce", "actor": actorURL, "object": objIRI,
	}
	row = runInbound(t, ctx, svc, boost)
	if row.Status != "processed" {
		t.Fatalf("announce status %q (reason %v)", row.Status, row.Error)
	}
	var shares, shareCount int64
	if err := d.Reader.QueryRow(ctx,
		`SELECT COUNT(*) FROM federation_remote_shares WHERE content_id = $1`, contentID).Scan(&shares); err != nil {
		t.Fatalf("shares: %v", err)
	}
	if err := d.Reader.QueryRow(ctx,
		`SELECT COALESCE(share_count,0) FROM contents WHERE id = $1`, contentID).Scan(&shareCount); err != nil {
		t.Fatalf("share_count: %v", err)
	}
	if shares != 1 || shareCount != 1 {
		t.Fatalf("shares=%d share_count=%d", shares, shareCount)
	}
	// Double-announce can't double-count.
	boost["id"] = actorURL + "#b2"
	if row := runInbound(t, ctx, svc, boost); row.Status != "ignored" {
		t.Fatalf("replayed announce status %q", row.Status)
	}

	// Undo{Like} + Undo{Announce} unwind.
	runInbound(t, ctx, svc, map[string]any{
		"id": actorURL + "#ul", "type": "Undo", "actor": actorURL,
		"object": map[string]any{"id": actorURL + "#l1", "type": "Like", "actor": actorURL, "object": objIRI},
	})
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM reactions WHERE entity_id = $1 AND remote_actor_id = $2`,
		contentID, remote.ID); n != 0 {
		t.Fatalf("reactions after unlike: %d", n)
	}
	runInbound(t, ctx, svc, map[string]any{
		"id": actorURL + "#ub", "type": "Undo", "actor": actorURL,
		"object": map[string]any{"id": actorURL + "#b1", "type": "Announce", "actor": actorURL, "object": objIRI},
	})
	if err := d.Reader.QueryRow(ctx,
		`SELECT COALESCE(share_count,0) FROM contents WHERE id = $1`, contentID).Scan(&shareCount); err != nil {
		t.Fatal(err)
	}
	if shareCount != 0 {
		t.Fatalf("share_count after unannounce: %d", shareCount)
	}
}

func TestDBInboundCreateReplyAndDelete(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	contentID, objIRI := newFederatedContent(t, ctx, svc, userID)
	remote, actorURL := newRemoteActor(t, ctx, svc, "remote.example")
	other, otherURL := newRemoteActor(t, ctx, svc, "evil.example")
	_ = other

	noteIRI := actorURL + "/notes/1"
	create := map[string]any{
		"id": actorURL + "#c1", "type": "Create", "actor": actorURL,
		"object": map[string]any{
			"id":        noteIRI,
			"type":      "Note",
			"inReplyTo": objIRI,
			"content":   "<p>Hello from <b>remote</b></p>",
		},
	}
	row := runInbound(t, ctx, svc, create)
	if row.Status != "processed" {
		t.Fatalf("create status %q (reason %v)", row.Status, row.Error)
	}
	var cID, cContent, apActor pgtype.UUID
	var body []byte
	if err := d.Reader.QueryRow(ctx,
		`SELECT id, content_id, remote_ap_actor_id, body FROM comments WHERE remote_uri = $1`, noteIRI).Scan(&cID, &cContent, &apActor, &body); err != nil {
		t.Fatalf("comment: %v", err)
	}
	if cContent != contentID {
		t.Fatalf("comment content_id: %v want %v", cContent, contentID)
	}
	if !apActor.Valid || apActor != remote.ID {
		t.Fatalf("comment provenance: %v want %v", apActor, remote.ID)
	}
	if got := string(body); got == "" || !json.Valid(body) {
		t.Fatalf("comment body not ProseMirror doc: %q", got)
	}

	// A foreign actor's Delete{object} is rejected — provenance gate.
	badDelete := map[string]any{
		"id": otherURL + "#d1", "type": "Delete", "actor": otherURL, "object": noteIRI,
	}
	if row := runInbound(t, ctx, svc, badDelete); row.Status != "rejected" {
		t.Fatalf("foreign delete status %q", row.Status)
	}
	var deleted *time.Time
	if err := d.Reader.QueryRow(ctx,
		`SELECT deleted_at FROM comments WHERE remote_uri = $1`, noteIRI).Scan(&deleted); err != nil {
		t.Fatal(err)
	}
	if deleted != nil {
		t.Fatal("foreign delete tombstoned the comment")
	}

	// The author's own Delete works.
	del := map[string]any{
		"id": actorURL + "#d1", "type": "Delete", "actor": actorURL, "object": noteIRI,
	}
	if row := runInbound(t, ctx, svc, del); row.Status != "processed" {
		t.Fatalf("delete status %q (reason %v)", row.Status, row.Error)
	}
	if err := d.Reader.QueryRow(ctx,
		`SELECT deleted_at FROM comments WHERE remote_uri = $1`, noteIRI).Scan(&deleted); err != nil {
		t.Fatal(err)
	}
	if deleted == nil {
		t.Fatal("own delete did not tombstone")
	}
}

func TestDBInboundDeleteActorUnwinds(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	_, objIRI := newFederatedContent(t, ctx, svc, userID)
	remote, actorURL := newRemoteActor(t, ctx, svc, "remote.example")
	actorIRI := svc.apiBase() + "/federation/actors/" + uname

	runInbound(t, ctx, svc, map[string]any{
		"id": actorURL + "#f", "type": "Follow", "actor": actorURL, "object": actorIRI})
	runInbound(t, ctx, svc, map[string]any{
		"id": actorURL + "#l", "type": "Like", "actor": actorURL, "object": objIRI})
	runInbound(t, ctx, svc, map[string]any{
		"id": actorURL + "#b", "type": "Announce", "actor": actorURL, "object": objIRI})
	runInbound(t, ctx, svc, map[string]any{
		"id": actorURL + "#c", "type": "Create", "actor": actorURL,
		"object": map[string]any{"id": actorURL + "/n1", "type": "Note", "inReplyTo": objIRI, "content": "<p>x</p>"}})

	delActor := map[string]any{
		"id": actorURL + "#del", "type": "Delete", "actor": actorURL, "object": actorURL,
	}
	row := runInbound(t, ctx, svc, delActor)
	if row.Status != "processed" {
		t.Fatalf("delete actor status %q (reason %v)", row.Status, row.Error)
	}

	checks := []struct {
		name string
		n    int64
	}{
		{"federation_follows undone", countRows(t, ctx, svc,
			`SELECT COUNT(*) FROM federation_follows WHERE remote_actor_id = $1 AND status = 'undone'`, remote.ID)},
		{"follows mirror", countRows(t, ctx, svc,
			`SELECT COUNT(*) FROM follows WHERE follower_remote_actor_id = $1`, remote.ID)},
		{"reactions", countRows(t, ctx, svc,
			`SELECT COUNT(*) FROM reactions WHERE remote_actor_id = $1`, remote.ID)},
		{"shares", countRows(t, ctx, svc,
			`SELECT COUNT(*) FROM federation_remote_shares WHERE remote_actor_id = $1`, remote.ID)},
		{"live comments", countRows(t, ctx, svc,
			`SELECT COUNT(*) FROM comments WHERE remote_ap_actor_id = $1 AND deleted_at IS NULL`, remote.ID)},
		{"tombstoned actor", countRows(t, ctx, svc,
			`SELECT COUNT(*) FROM remote_actors WHERE id = $1 AND status = 'tombstone'`, remote.ID)},
	}
	want := []int64{1, 0, 0, 0, 0, 1}
	for i, c := range checks {
		if c.n != want[i] {
			t.Errorf("%s: got %d want %d", c.name, c.n, want[i])
		}
	}
}

func TestDBInboundRejectsActorMismatch(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	signer, signerURL := newRemoteActor(t, ctx, svc, "signer.example")
	_, victimURL := newRemoteActor(t, ctx, svc, "victim.example")
	_ = signer

	// Signer is signer.example but the activity claims victim.example —
	// dispatch must reject before any write.
	act := map[string]any{
		"id":     victimURL + "#f",
		"type":   "Follow",
		"actor":  victimURL,
		"object": svc.apiBase() + "/federation/actors/" + uname,
	}
	payload, _ := json.Marshal(act)
	row, err := svc.store.insertInbound(ctx, gen.InsertFederationInboundParams{
		ActivityApID:   stringPtrOrNil(victimURL + "#f"),
		ActivityType:   "Follow",
		ActorUrl:       stringPtrOrNil(signerURL), // ledgered signer ≠ declared actor
		SignatureValid: true,
		Payload:        payload,
	})
	if err != nil {
		t.Fatalf("insert inbound: %v", err)
	}
	if err := svc.ProcessInbound(ctx, row.ID); err != nil {
		t.Fatalf("ProcessInbound: %v", err)
	}
	var status string
	if err := d.Reader.QueryRow(ctx,
		`SELECT status FROM federation_inbound WHERE id = $1`, row.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "rejected" {
		t.Fatalf("mismatch status %q", status)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM federation_follows WHERE remote_actor_id IN ($1)`, signer.ID); n != 0 {
		t.Fatalf("forged follow wrote rows: %d", n)
	}
}

// approveFollowers seeds profile_visibility='followers' so the member's
// federated actor lands manually_approves_followers at EnableFederation.
func approveFollowers(t *testing.T, ctx context.Context, d *db.DB, userID pgtype.UUID) {
	t.Helper()
	if _, err := d.Writer.Exec(ctx,
		`INSERT INTO user_privacy_settings (user_id, profile_visibility)
		 VALUES ($1, 'followers') ON CONFLICT (user_id) DO UPDATE SET profile_visibility = 'followers'`,
		userID); err != nil {
		t.Fatalf("privacy settings: %v", err)
	}
}

// cleanDeliveries wipes ledger rows for the involved actors —
// federation_deliveries FKs are NO ACTION so rows outlive test cleanups.
func cleanDeliveries(t *testing.T, ctx context.Context, svc *Service) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = svc.db.Writer.Exec(context.Background(), `DELETE FROM federation_deliveries`)
	})
}

func TestDBFollowRequestApproveReject(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newFederationUser(t, ctx, d, "active")
	approveFollowers(t, ctx, d, userID)
	svc := newFederationService(t, d)
	cleanDeliveries(t, ctx, svc)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	remote, actorURL := newRemoteActor(t, ctx, svc, "remote.example")

	// Manual approval parks the edge pending — no mirror, no Accept.
	row := runInbound(t, ctx, svc, map[string]any{
		"id":     actorURL + "#follow-req",
		"type":   "Follow",
		"actor":  actorURL,
		"object": svc.apiBase() + "/federation/actors/" + uname,
	})
	if row.Status != "processed" {
		t.Fatalf("follow status %q (reason %v)", row.Status, row.Error)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM federation_follows WHERE remote_actor_id = $1 AND status = 'pending'`, remote.ID); n != 1 {
		t.Fatalf("pending edge: %d", n)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM follows WHERE follower_remote_actor_id = $1`, remote.ID); n != 0 {
		t.Fatalf("premature mirror rows: %d", n)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM federation_deliveries WHERE remote_actor_id = $1`, remote.ID); n != 0 {
		t.Fatalf("premature deliveries: %d", n)
	}

	// The member sees exactly one pending request.
	reqs, err := svc.ListFollowRequests(ctx, userID)
	if err != nil || len(reqs) != 1 {
		t.Fatalf("follow requests: %v %d", err, len(reqs))
	}
	if reqs[0].RemoteActorID != remote.ID || reqs[0].ActorUrl != actorURL {
		t.Fatalf("request row: %+v", reqs[0])
	}

	// Another member cannot resolve it.
	otherID, _ := newFederationUser(t, ctx, d, "active")
	if _, err := svc.EnableFederation(ctx, otherID); err != nil {
		t.Fatalf("enable other: %v", err)
	}
	if err := svc.ResolveFollowRequest(ctx, otherID, remote.ID, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign resolve: %v", err)
	}

	// Reject writes the edge and ledgers Reject{Follow} verbatim.
	if err := svc.ResolveFollowRequest(ctx, userID, remote.ID, false); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM federation_follows WHERE remote_actor_id = $1 AND status = 'rejected'`, remote.ID); n != 1 {
		t.Fatalf("rejected edge: %d", n)
	}
	var rejType string
	var rejObj []byte
	if err := svc.db.Reader.QueryRow(ctx,
		`SELECT activity_type, activity_json FROM federation_deliveries WHERE remote_actor_id = $1`,
		remote.ID).Scan(&rejType, &rejObj); err != nil {
		t.Fatalf("reject delivery: %v", err)
	}
	if rejType != "Reject" {
		t.Fatalf("delivery type %q", rejType)
	}
	var rejAct map[string]any
	_ = json.Unmarshal(rejObj, &rejAct)
	obj, _ := rejAct["object"].(map[string]any)
	if obj["id"] != actorURL+"#follow-req" {
		t.Fatalf("reject does not embed the verbatim follow: %s", rejObj)
	}

	// Resolved edges are terminal — a replayed resolve is a clean 404.
	if err := svc.ResolveFollowRequest(ctx, userID, remote.ID, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("re-resolve: %v", err)
	}
}

func TestDBFollowRequestAcceptMirrors(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newFederationUser(t, ctx, d, "active")
	approveFollowers(t, ctx, d, userID)
	svc := newFederationService(t, d)
	cleanDeliveries(t, ctx, svc)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	remote, actorURL := newRemoteActor(t, ctx, svc, "remote2.example")
	runInbound(t, ctx, svc, map[string]any{
		"id":     actorURL + "#follow-acc",
		"type":   "Follow",
		"actor":  actorURL,
		"object": svc.apiBase() + "/federation/actors/" + uname,
	})

	if err := svc.ResolveFollowRequest(ctx, userID, remote.ID, true); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM federation_follows WHERE remote_actor_id = $1 AND status = 'accepted'`, remote.ID); n != 1 {
		t.Fatalf("accepted edge: %d", n)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM follows WHERE follower_remote_actor_id = $1 AND entity_type = 'user'`, remote.ID); n != 1 {
		t.Fatalf("mirror rows: %d", n)
	}
	var actType string
	if err := svc.db.Reader.QueryRow(ctx,
		`SELECT activity_type FROM federation_deliveries WHERE remote_actor_id = $1`,
		remote.ID).Scan(&actType); err != nil {
		t.Fatalf("accept delivery: %v", err)
	}
	if actType != "Accept" {
		t.Fatalf("delivery type %q", actType)
	}
}

// Two id-less Follows from different actors on one shared inbox must not
// collapse onto a single "#accepts/" delivery key.
func TestDBIdlessFollowAcceptDistinct(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	cleanDeliveries(t, ctx, svc)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	// Same domain → same seeded inbox_url; the payloads differ only by
	// actor, which is exactly the collision the hash fallback prevents.
	r1, u1 := newRemoteActor(t, ctx, svc, "shared.example")
	r2, u2 := newRemoteActor(t, ctx, svc, "shared.example")
	for _, u := range []string{u1, u2} {
		row := runInbound(t, ctx, svc, map[string]any{
			"type":   "Follow",
			"actor":  u,
			"object": svc.apiBase() + "/federation/actors/" + uname,
		})
		if row.Status != "processed" {
			t.Fatalf("follow status %q", row.Status)
		}
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM federation_deliveries WHERE remote_actor_id IN ($1, $2) AND activity_type = 'Accept'`,
		r1.ID, r2.ID); n != 2 {
		t.Fatalf("accept deliveries: %d (want 2)", n)
	}
}

// Stranded-work sweep: PruneDeliveries re-arms failed-due and stale
// pending rows whose task enqueue was lost, while fresh pending rows are
// left to their in-flight task.
func TestDBPruneDeliveriesSweepsStranded(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	cleanDeliveries(t, ctx, svc)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	actor, err := svc.store.actorByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("actor: %v", err)
	}
	q := gen.New(d.Writer)
	mkDelivery := func(apID, inbox string) gen.FederationDelivery {
		row, err := q.InsertFederationDelivery(ctx, gen.InsertFederationDeliveryParams{
			ActivityApID: apID, ActivityType: "Create", ActivityJson: []byte(`{}`),
			LocalActorID: actor.ID, InboxUrl: inbox,
		})
		if err != nil {
			t.Fatalf("insert delivery: %v", err)
		}
		return row
	}
	stale := mkDelivery("https://x/d1", "https://remote.example/i1")
	if _, err := d.Writer.Exec(ctx,
		`UPDATE federation_deliveries SET created_at = now() - interval '10 minutes' WHERE id = $1`, stale.ID); err != nil {
		t.Fatalf("backdate: %v", err)
	}
	failedDue := mkDelivery("https://x/d2", "https://remote.example/i2")
	if _, err := d.Writer.Exec(ctx,
		`UPDATE federation_deliveries SET status='failed', next_retry_at = now() - interval '1 minute' WHERE id = $1`, failedDue.ID); err != nil {
		t.Fatalf("failed due: %v", err)
	}
	failedFuture := mkDelivery("https://x/d3", "https://remote.example/i3")
	if _, err := d.Writer.Exec(ctx,
		`UPDATE federation_deliveries SET status='failed', next_retry_at = now() + interval '1 hour' WHERE id = $1`, failedFuture.ID); err != nil {
		t.Fatalf("failed future: %v", err)
	}
	fresh := mkDelivery("https://x/d4", "https://remote.example/i4")

	enq := &recordingEnqueuer{}
	svc.SetEnqueuer(enq)
	if err := svc.PruneDeliveries(ctx, 30); err != nil {
		t.Fatalf("prune: %v", err)
	}
	got := map[pgtype.UUID]bool{}
	for _, id := range enq.deliveries {
		got[id] = true
	}
	if !got[stale.ID] {
		t.Error("stale pending delivery not re-enqueued")
	}
	if !got[failedDue.ID] {
		t.Error("due failed delivery not re-enqueued")
	}
	if got[failedFuture.ID] {
		t.Error("future-retry delivery re-enqueued early")
	}
	if got[fresh.ID] {
		t.Error("fresh pending delivery double-armed by the sweep")
	}
}

// After the 30-day delivery prune drops a delivered object's ledger rows,
// published_at is the surviving "remote copy plausible" signal — an
// unpublish must still fan out Delete.
func TestDBFanoutDeleteAfterLedgerPrune(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	cleanDeliveries(t, ctx, svc)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	contentID, _ := newFederatedContent(t, ctx, svc, userID)
	remote, _ := newRemoteActor(t, ctx, svc, "remote3.example")
	actor, err := svc.store.actorByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("actor: %v", err)
	}
	if _, err := gen.New(d.Writer).UpsertFederationFollow(ctx, gen.UpsertFederationFollowParams{
		LocalActorID: actor.ID, RemoteActorID: remote.ID, Status: "accepted",
	}); err != nil {
		t.Fatalf("follow: %v", err)
	}
	// Pruned-ledger state: the object left the federatable set with no
	// surviving delivery row to prove a remote copy.
	if _, err := d.Writer.Exec(ctx,
		`UPDATE contents SET visibility = 'private' WHERE id = $1`, contentID); err != nil {
		t.Fatalf("unpublish: %v", err)
	}
	if err := svc.Fanout(ctx, contentID); err != nil {
		t.Fatalf("fanout: %v", err)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM federation_deliveries WHERE remote_actor_id = $1 AND activity_type = 'Delete'`,
		remote.ID); n != 1 {
		t.Fatalf("delete deliveries after prune: %d (want 1)", n)
	}
}

type recordingEnqueuer struct{ deliveries []pgtype.UUID }

func (e *recordingEnqueuer) EnqueueDelivery(_ context.Context, id pgtype.UUID) error {
	e.deliveries = append(e.deliveries, id)
	return nil
}
func (e *recordingEnqueuer) EnqueueDeliveryAfter(context.Context, pgtype.UUID, time.Duration) error {
	return nil
}
func (e *recordingEnqueuer) EnqueueInbound(context.Context, pgtype.UUID) error { return nil }

func TestDBRemoteProvenanceConstraints(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	contentID, _ := newFederatedContent(t, ctx, svc, userID)
	remote, _ := newRemoteActor(t, ctx, svc, "remote.example")

	// follows XOR: both provenance sides → rejected by CHECK.
	_, err := d.Writer.Exec(ctx,
		`INSERT INTO follows (follower_id, follower_remote_actor_id, entity_type, entity_id)
		 VALUES ($1, $2, 'user', $1)`, userID, remote.ID)
	if err == nil {
		t.Error("follows XOR accepted both provenance sides")
	}
	// Neither side → rejected.
	_, err = d.Writer.Exec(ctx,
		`INSERT INTO follows (entity_type, entity_id) VALUES ('user', $1)`, userID)
	if err == nil {
		t.Error("follows XOR accepted no provenance")
	}
	// reactions XOR: both sides → rejected.
	_, err = d.Writer.Exec(ctx,
		`INSERT INTO reactions (entity_type, entity_id, user_id, remote_actor_id, reaction_key)
		 VALUES ('content', $1, $2, $3, 'like')`, contentID, userID, remote.ID)
	if err == nil {
		t.Error("reactions XOR accepted both actor sources")
	}

	// Remote reaction survives ON DELETE CASCADE of the remote actor —
	// deleting the actor row removes its reactions.
	if _, err := d.Writer.Exec(ctx,
		`INSERT INTO reactions (entity_type, entity_id, remote_actor_id, reaction_key)
		 VALUES ('content', $1, $2, 'like')`, contentID, remote.ID); err != nil {
		t.Fatalf("remote reaction insert: %v", err)
	}
	if _, err := d.Writer.Exec(ctx,
		`DELETE FROM remote_actors WHERE id = $1`, remote.ID); err != nil {
		t.Fatalf("remote actor delete: %v", err)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM reactions WHERE remote_actor_id = $1`, remote.ID); n != 0 {
		t.Errorf("reaction rows survived remote actor delete: %d", n)
	}
}
