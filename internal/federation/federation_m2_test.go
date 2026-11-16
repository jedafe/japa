package federation

// §126 M2 DB integration tests — the inbound dispatch pipeline end-to-end
// against real Postgres: signature-trusted rows from federation_inbound
// through tx dispatch into domain rows, ledger statuses, and counter
// convergence. Remote actors seed via direct SQL (status='active',
// fresh fetched_at) so resolveRemoteActor takes the store fast path —
// no network in tests.

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	gen "github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/spam"
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
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
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
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
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

// Undo targets must not be gated on the object still being federatable —
// a Like lands while published, the post then goes private, and the
// remote's unlike still has to settle the count.
func TestDBUndoLikeAfterUnpublish(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	cleanDeliveries(t, ctx, svc)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	contentID, objURL := newFederatedContent(t, ctx, svc, userID)
	remote, actorURL := newRemoteActor(t, ctx, svc, "undo.example")

	row := runInbound(t, ctx, svc, map[string]any{
		"id": actorURL + "#like-1", "type": "Like", "actor": actorURL, "object": objURL,
	})
	if row.Status != "processed" {
		t.Fatalf("like status %q", row.Status)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM reactions WHERE remote_actor_id = $1 AND entity_id = $2`, remote.ID, contentID); n != 1 {
		t.Fatalf("reaction rows: %d", n)
	}

	// Post leaves the federatable set — the unlike must still apply.
	if _, err := d.Writer.Exec(ctx,
		`UPDATE contents SET visibility = 'private' WHERE id = $1`, contentID); err != nil {
		t.Fatalf("unpublish: %v", err)
	}
	row = runInbound(t, ctx, svc, map[string]any{
		"id": actorURL + "#undo-like-1", "type": "Undo", "actor": actorURL,
		"object": map[string]any{"type": "Like", "actor": actorURL, "object": objURL},
	})
	if row.Status != "processed" {
		t.Fatalf("undo status %q (reason %v)", row.Status, row.Error)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM reactions WHERE remote_actor_id = $1 AND entity_id = $2`, remote.ID, contentID); n != 0 {
		t.Fatalf("reaction rows after undo on private content: %d", n)
	}
}

// A privacy flip after EnableFederation must gate the NEXT inbound
// Follow — the stored flag only refreshes at enable/reactivate.
func TestDBFollowUsesLiveApprovalFlag(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	cleanDeliveries(t, ctx, svc)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	r1, u1 := newRemoteActor(t, ctx, svc, "flip1.example")
	row := runInbound(t, ctx, svc, map[string]any{
		"id": u1 + "#f1", "type": "Follow", "actor": u1,
		"object": svc.apiBase() + "/federation/actors/" + uname,
	})
	if row.Status != "processed" {
		t.Fatalf("first follow status %q", row.Status)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM federation_follows WHERE remote_actor_id = $1 AND status = 'accepted'`, r1.ID); n != 1 {
		t.Fatalf("auto-accept before flip: %d", n)
	}

	// Member switches to followers-only — the next Follow must park.
	approveFollowers(t, ctx, d, userID)
	r2, u2 := newRemoteActor(t, ctx, svc, "flip2.example")
	row = runInbound(t, ctx, svc, map[string]any{
		"id": u2 + "#f2", "type": "Follow", "actor": u2,
		"object": svc.apiBase() + "/federation/actors/" + uname,
	})
	if row.Status != "processed" {
		t.Fatalf("second follow status %q", row.Status)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM federation_follows WHERE remote_actor_id = $1 AND status = 'pending'`, r2.ID); n != 1 {
		t.Fatalf("pending edge after live privacy flip: %d", n)
	}

	// Re-sent Follows against the same pending edge must not re-ping.
	row = runInbound(t, ctx, svc, map[string]any{
		"id": u2 + "#f2-retry", "type": "Follow", "actor": u2,
		"object": svc.apiBase() + "/federation/actors/" + uname,
	})
	if row.Status != "processed" {
		t.Fatalf("resent follow status %q", row.Status)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM federation_follows WHERE remote_actor_id = $1`, r2.ID); n != 1 {
		t.Fatalf("edge count after resend: %d", n)
	}
}

// §49 lock semantics apply to remote replies too.
func TestDBRemoteReplyToLockedContent(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	contentID, objURL := newFederatedContent(t, ctx, svc, userID)
	remote, actorURL := newRemoteActor(t, ctx, svc, "lock.example")
	if _, err := d.Writer.Exec(ctx,
		`UPDATE contents SET is_locked = true WHERE id = $1`, contentID); err != nil {
		t.Fatalf("lock: %v", err)
	}
	row := runInbound(t, ctx, svc, map[string]any{
		"id": actorURL + "#note-1", "type": "Create", "actor": actorURL,
		"object": map[string]any{
			"id": actorURL + "/notes/1", "type": "Note",
			"inReplyTo": objURL, "content": "<p>hi</p>",
		},
	})
	if row.Status != "ignored" {
		t.Fatalf("locked reply status %q (want ignored)", row.Status)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM comments WHERE remote_ap_actor_id = $1`, remote.ID); n != 0 {
		t.Fatalf("comments on locked content: %d", n)
	}
}

// Stale 'received' inbound rows get re-armed by the daily task.
func TestDBStaleInboundSweep(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	cleanDeliveries(t, ctx, svc)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	q := gen.New(d.Writer)
	stale, err := q.InsertFederationInbound(ctx, gen.InsertFederationInboundParams{
		ActivityApID: strPtr("https://remote.example/act/stale"), ActivityType: "Like",
		ActorUrl: strPtr("https://remote.example/actors/1"), SignatureValid: true,
		Payload: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("insert inbound: %v", err)
	}
	if _, err := d.Writer.Exec(ctx,
		`UPDATE federation_inbound SET created_at = now() - interval '10 minutes' WHERE id = $1`, stale.ID); err != nil {
		t.Fatalf("backdate inbound: %v", err)
	}
	fresh, err := q.InsertFederationInbound(ctx, gen.InsertFederationInboundParams{
		ActivityApID: strPtr("https://remote.example/act/fresh"), ActivityType: "Like",
		ActorUrl: strPtr("https://remote.example/actors/1"), SignatureValid: true,
		Payload: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("insert fresh inbound: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.Writer.Exec(context.Background(),
			`DELETE FROM federation_inbound WHERE id IN ($1,$2)`, stale.ID, fresh.ID)
	})

	enq := &recordingEnqueuer{}
	svc.SetEnqueuer(enq)
	if err := svc.PruneDeliveries(ctx, 30); err != nil {
		t.Fatalf("prune: %v", err)
	}
	got := map[pgtype.UUID]bool{}
	for _, id := range enq.inbounds {
		got[id] = true
	}
	if !got[stale.ID] {
		t.Error("stale inbound row not re-enqueued")
	}
	if got[fresh.ID] {
		t.Error("fresh inbound row double-armed")
	}
}

// An instance blocked after an actor row was cached must stop verifying —
// the keyId hit path re-checks the domain gate.
func TestDBKeyActorBlockedAfterCache(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	remote, _ := newRemoteActor(t, ctx, svc, "lateblock.example")
	if _, err := gen.New(d.Writer).CreateInstanceBlock(ctx, gen.CreateInstanceBlockParams{
		Domain: "lateblock.example", BlockedBy: userID,
	}); err != nil {
		t.Fatalf("block: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.Writer.Exec(context.Background(),
			`DELETE FROM federation_instance_blocks WHERE domain = 'lateblock.example'`)
	})
	if _, err := svc.resolveKeyActor(ctx, *remote.PublicKeyID); !errors.Is(err, ErrRemoteBlocked) {
		t.Fatalf("cached-but-blocked key resolved: %v", err)
	}
}

type recordingEnqueuer struct {
	deliveries []pgtype.UUID
	inbounds   []pgtype.UUID
}

func (e *recordingEnqueuer) EnqueueDelivery(_ context.Context, id pgtype.UUID) error {
	e.deliveries = append(e.deliveries, id)
	return nil
}
func (e *recordingEnqueuer) EnqueueDeliveryAfter(context.Context, pgtype.UUID, time.Duration) error {
	return nil
}
func (e *recordingEnqueuer) EnqueueInbound(_ context.Context, id pgtype.UUID) error {
	e.inbounds = append(e.inbounds, id)
	return nil
}

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

// fakeSpamChecker records invocations and returns a fixed verdict.
type fakeSpamChecker struct {
	blocked bool
	checked int
}

func (f *fakeSpamChecker) Check(_ context.Context, _ spam.SpamCheckRequest) (spam.SpamCheckResult, error) {
	f.checked++
	return spam.SpamCheckResult{Blocked: f.blocked, Reasons: []string{"test verdict"}}, nil
}

func TestDBInboundReplySpamCheck(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	contentID, objIRI := newFederatedContent(t, ctx, svc, userID)
	_, actorURL := newRemoteActor(t, ctx, svc, "remote.example")

	fake := &fakeSpamChecker{blocked: true}
	svc.SetSpamChecker(fake)

	create := map[string]any{
		"id": actorURL + "#c1", "type": "Create", "actor": actorURL,
		"object": map[string]any{
			"id":        actorURL + "/notes/1",
			"type":      "Note",
			"inReplyTo": objIRI,
			"content":   "<p>spammy</p>",
		},
	}
	row := runInbound(t, ctx, svc, create)
	if row.Status != "rejected" {
		t.Fatalf("blocked reply status %q (reason %v)", row.Status, row.Error)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM comments WHERE content_id = $1`, contentID); n != 0 {
		t.Fatalf("blocked reply landed: %d comments", n)
	}

	// A clean verdict lets the same reply through.
	fake.blocked = false
	create["id"] = actorURL + "#c2"
	create["object"].(map[string]any)["id"] = actorURL + "/notes/2"
	row = runInbound(t, ctx, svc, create)
	if row.Status != "processed" {
		t.Fatalf("clean reply status %q (reason %v)", row.Status, row.Error)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM comments WHERE content_id = $1`, contentID); n != 1 {
		t.Fatalf("clean reply missing: %d comments", n)
	}
	if fake.checked != 2 {
		t.Fatalf("spam checks: %d want 2", fake.checked)
	}
}

func TestDBRefollowAfterUndoResurrects(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	remote, actorURL := newRemoteActor(t, ctx, svc, "remote.example")

	follow := func(id string) map[string]any {
		return map[string]any{
			"type":   "Follow",
			"actor":  actorURL,
			"object": svc.apiBase() + "/federation/actors/" + uname,
			"id":     id,
		}
	}
	if row := runInbound(t, ctx, svc, follow(actorURL+"#f1")); row.Status != "processed" {
		t.Fatalf("follow: %q", row.Status)
	}
	undo := map[string]any{
		"id": actorURL + "#u1", "type": "Undo", "actor": actorURL,
		"object": follow(actorURL + "#f1"),
	}
	if row := runInbound(t, ctx, svc, undo); row.Status != "processed" {
		t.Fatalf("undo: %q", row.Status)
	}
	// A NEW Follow activity id after Undo is a genuine re-request: the
	// edge resurrects and a second Accept is ledgered — the per-event
	// discriminator keeps it distinct under the (activity_ap_id, inbox)
	// dedup.
	if row := runInbound(t, ctx, svc, follow(actorURL+"#f2")); row.Status != "processed" {
		t.Fatalf("refollow status %q (reason %v)", row.Status, row.Error)
	}
	var accepts int64
	if err := d.Reader.QueryRow(ctx,
		`SELECT COUNT(*) FROM federation_deliveries WHERE remote_actor_id = $1 AND activity_type = 'Accept'`,
		remote.ID).Scan(&accepts); err != nil {
		t.Fatal(err)
	}
	if accepts != 2 {
		t.Fatalf("accept deliveries: %d want 2 — re-Follow must re-answer", accepts)
	}
	// An id-less re-Follow can't be distinguished from a replay of the
	// undone Follow — the edge stays undone (conservative).
	idless := map[string]any{
		"type":   "Follow",
		"actor":  actorURL,
		"object": svc.apiBase() + "/federation/actors/" + uname,
	}
	undo2 := map[string]any{
		"id": actorURL + "#u2", "type": "Undo", "actor": actorURL,
		"object": follow(actorURL + "#f2"),
	}
	if row := runInbound(t, ctx, svc, undo2); row.Status != "processed" {
		t.Fatalf("second undo: %q", row.Status)
	}
	if row := runInbound(t, ctx, svc, idless); row.Status != "ignored" {
		t.Fatalf("id-less refollow status %q want ignored", row.Status)
	}
	// Update path: spam-checked edits reject.
	fake := &fakeSpamChecker{blocked: true}
	svc.SetSpamChecker(fake)
	upd := map[string]any{
		"id": actorURL + "#u9", "type": "Update", "actor": actorURL,
		"object": map[string]any{
			"id": actorURL + "/notes/9", "type": "Note", "content": "<p>spam</p>",
		},
	}
	if row := runInbound(t, ctx, svc, upd); row.Status != "rejected" {
		t.Fatalf("spammy update status %q", row.Status)
	}
	svc.SetSpamChecker(nil)
}

func TestDBRefollowAcceptDistinctDelivery(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	remote, actorURL := newRemoteActor(t, ctx, svc, "remote.example")
	objURL := svc.apiBase() + "/federation/actors/" + uname

	// Two distinct Follow events (fresh activity ids) on the same edge —
	// the remote re-sent because it never saw the first Accept. Each must
	// produce its own delivery row.
	for _, id := range []string{actorURL + "#fa", actorURL + "#fb"} {
		row := runInbound(t, ctx, svc, map[string]any{
			"id": id, "type": "Follow", "actor": actorURL, "object": objURL,
		})
		if row.Status != "processed" {
			t.Fatalf("follow %s: %q (%v)", id, row.Status, row.Error)
		}
	}
	var n int64
	if err := d.Reader.QueryRow(ctx,
		`SELECT COUNT(*) FROM federation_deliveries WHERE remote_actor_id = $1 AND activity_type = 'Accept'`,
		remote.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("accept deliveries: %d want 2 (re-sent Follows must re-answer)", n)
	}
}

func TestDBRemoteObjectIDDomainGate(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	contentID, objIRI := newFederatedContent(t, ctx, svc, userID)
	_, actorURL := newRemoteActor(t, ctx, svc, "remote.example")

	// Object id minted on OUR domain — squatting on a local object URL.
	create := map[string]any{
		"id": actorURL + "#c1", "type": "Create", "actor": actorURL,
		"object": map[string]any{
			"id":        svc.apiBase() + "/federation/objects/contents/" + contentID.String(),
			"type":      "Note",
			"inReplyTo": objIRI,
			"content":   "<p>squat</p>",
		},
	}
	if row := runInbound(t, ctx, svc, create); row.Status != "rejected" {
		t.Fatalf("foreign-domain id status %q", row.Status)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM comments WHERE content_id = $1`, contentID); n != 0 {
		t.Fatalf("squatted comment landed: %d", n)
	}
}

func TestDBInboundPrune(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newFederationService(t, d)

	old, err := d.Writer.Exec(ctx,
		`UPDATE federation_inbound SET created_at = now() - interval '40 days'`)
	if err != nil {
		t.Fatalf("age inbound: %v", err)
	}
	if err := svc.PruneDeliveries(ctx, 30); err != nil {
		t.Fatalf("prune: %v", err)
	}
	var remaining int64
	if err := d.Reader.QueryRow(ctx, `SELECT COUNT(*) FROM federation_inbound`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if old.RowsAffected() > 0 && remaining != 0 {
		t.Fatalf("inbound rows past retention survived prune: %d", remaining)
	}
}

func TestDBObjectTombstone(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	contentID, _ := newFederatedContent(t, ctx, svc, userID)

	// Live → document.
	if _, err := svc.ObjectDocument(ctx, contentID); err != nil {
		t.Fatalf("live object: %v", err)
	}
	// Soft-deleted → 410 Tombstone (was publicly addressable).
	if _, err := d.Writer.Exec(ctx,
		`UPDATE contents SET deleted_at = now() WHERE id = $1`, contentID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ObjectDocument(ctx, contentID); err != ErrGone {
		t.Fatalf("deleted object: got %v, want ErrGone", err)
	}
	// Never-published → still 404 (existence stays hidden).
	var neverPub pgtype.UUID
	if err := d.Writer.QueryRow(ctx,
		`INSERT INTO contents (author_id, content_type, title, slug, body, content_hash, status, visibility)
		 VALUES ($1, 'story', 'draft', $2, 'x', $3, 'draft', 'public') RETURNING id`,
		userID, fmt.Sprintf("draft-%d", time.Now().UnixNano()),
		fmt.Sprintf("draft-hash-%d", time.Now().UnixNano())).Scan(&neverPub); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = d.Writer.Exec(context.Background(), `DELETE FROM contents WHERE id = $1`, neverPub)
	})
	if _, err := svc.ObjectDocument(ctx, neverPub); err != ErrNotFound {
		t.Fatalf("draft object: got %v, want ErrNotFound", err)
	}
}

// The remote follower edge is a real follows row — the member's
// denormalized users.follower_count must count it like a local follow,
// and unwind on Undo.
func TestDBRemoteFollowAdjustsFollowerCount(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	_, actorURL := newRemoteActor(t, ctx, svc, "remote.example")
	actorIRI := svc.apiBase() + "/federation/actors/" + uname
	followerCount := func() int64 {
		var n int64
		if err := svc.db.Reader.QueryRow(ctx,
			`SELECT follower_count FROM users WHERE id = $1`, userID).Scan(&n); err != nil {
			t.Fatalf("follower_count: %v", err)
		}
		return n
	}
	before := followerCount()

	follow := map[string]any{
		"id": actorURL + "#fc1", "type": "Follow", "actor": actorURL, "object": actorIRI,
	}
	if row := runInbound(t, ctx, svc, follow); row.Status != "processed" {
		t.Fatalf("follow: %q (%v)", row.Status, row.Error)
	}
	if got := followerCount(); got != before+1 {
		t.Fatalf("follower_count after remote follow: got %d, want %d", got, before+1)
	}

	undo := map[string]any{
		"id": actorURL + "#fc2", "type": "Undo", "actor": actorURL,
		"object": map[string]any{
			"id": actorURL + "#fc1", "type": "Follow", "actor": actorURL, "object": actorIRI,
		},
	}
	if row := runInbound(t, ctx, svc, undo); row.Status != "processed" {
		t.Fatalf("undo: %q", row.Status)
	}
	if got := followerCount(); got != before {
		t.Fatalf("follower_count after undo: got %d, want %d", got, before)
	}
}

// Two successive unpublish events on the same object must ledger two
// DISTINCT Delete ap_ids — (activity_ap_id, inbox) dedup plus remote-side
// activity-id dedup would otherwise collapse the second Delete and leave
// the remote serving a resurrected copy.
func TestDBDeleteFanoutDistinctApIDs(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	_, actorURL := newRemoteActor(t, ctx, svc, "remote.example")
	if row := runInbound(t, ctx, svc, map[string]any{
		"type": "Follow", "actor": actorURL,
		"object": svc.apiBase() + "/federation/actors/" + uname,
	}); row.Status != "processed" {
		t.Fatalf("follow: %q", row.Status)
	}
	contentID, objIRI := newFederatedContent(t, ctx, svc, userID)

	// Unpublish event #1 → Delete#1.
	if err := svc.fanoutDeleteIfDelivered(ctx, contentID); err != nil {
		t.Fatalf("first delete fanout: %v", err)
	}
	// A later lifecycle event carries a fresh updated_at — its Delete
	// ap_id must differ from the first.
	if _, err := d.Writer.Exec(ctx,
		`UPDATE contents SET updated_at = updated_at + interval '1 second' WHERE id = $1`, contentID); err != nil {
		t.Fatal(err)
	}
	if err := svc.fanoutDeleteIfDelivered(ctx, contentID); err != nil {
		t.Fatalf("second delete fanout: %v", err)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM federation_deliveries WHERE activity_type = 'Delete' AND activity_ap_id LIKE $1`,
		objIRI+"#delete/%"); n != 2 {
		t.Fatalf("distinct delete deliveries: %d (want 2)", n)
	}
}

// swapFetchTransport routes fake public hostnames to TLS test servers —
// the documents use .test hostnames so checkFetchURL's literal-IP rule
// doesn't fire, the dial maps each host to a server's real address, and
// skip-verify absorbs the cert mismatch. The SSRF transport is restored
// on cleanup.
func swapFetchTransport(t *testing.T, routes map[string]string) {
	t.Helper()
	old := fetchClient.Transport
	fetchClient.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if host, _, err := net.SplitHostPort(addr); err == nil {
				if real, ok := routes[host]; ok {
					addr = real
				}
			}
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
	}
	t.Cleanup(func() { fetchClient.Transport = old })
}

func actorDocHandler(doc map[string]any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/activity+json")
		_ = json.NewEncoder(w).Encode(doc)
	}
}

func tlsAddr(srv *httptest.Server) string { return srv.Listener.Addr().String() }

// A document fetched at keyId's base may CLAIM any actor id — without
// confirming that claim at the actor's own address, an attacker mints
// remote_actors rows under a victim URL keyed to the attacker's pem
// (full signing impersonation). The canonical re-fetch must reject it.
func TestResolveKeyActorRejectsForgedActorID(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newFederationService(t, d)

	victimActorURL := "https://victim.test/users/alice"
	victim := httptest.NewTLSServer(actorDocHandler(map[string]any{
		"id":                victimActorURL,
		"type":              "Person",
		"preferredUsername": "alice",
		"inbox":             "https://victim.test/inbox",
		"publicKey": map[string]any{
			"id":           victimActorURL + "#main-key",
			"owner":        victimActorURL,
			"publicKeyPem": "-----BEGIN PUBLIC KEY-----\nVICTIM\n-----END PUBLIC KEY-----",
		},
	}))
	defer victim.Close()

	keyID := "https://evil.test/keys/k1#main"
	evil := httptest.NewTLSServer(actorDocHandler(map[string]any{
		"id":                victimActorURL, // the lie — claims to BE the victim
		"type":              "Person",
		"preferredUsername": "alice",
		"inbox":             "https://evil.test/inbox",
		"publicKey": map[string]any{
			"id":           keyID,
			"publicKeyPem": "-----BEGIN PUBLIC KEY-----\nEVIL\n-----END PUBLIC KEY-----",
		},
	}))
	defer evil.Close()
	swapFetchTransport(t, map[string]string{
		"victim.test": tlsAddr(victim), "evil.test": tlsAddr(evil),
	})

	if _, err := svc.resolveKeyActor(ctx, keyID); err == nil {
		t.Fatal("forged actor id accepted — attacker row cached under victim URL")
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM remote_actors WHERE actor_url = $1`, victimActorURL); n != 0 {
		t.Fatalf("impersonated actor row persisted: %d", n)
	}
}

// The common case — keyId base IS the actor URL: self-claim is canonical,
// no second fetch, resolve succeeds.
func TestResolveKeyActorAcceptsSelfClaimedActor(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newFederationService(t, d)

	actorURL := "https://real.test/users/alice"
	srv := httptest.NewTLSServer(actorDocHandler(map[string]any{
		"id":                actorURL,
		"type":              "Person",
		"preferredUsername": "alice",
		"inbox":             "https://real.test/inbox",
		"publicKey": map[string]any{
			"id":           actorURL + "#main-key",
			"owner":        actorURL,
			"publicKeyPem": "-----BEGIN PUBLIC KEY-----\nREAL\n-----END PUBLIC KEY-----",
		},
	}))
	defer srv.Close()
	swapFetchTransport(t, map[string]string{"real.test": tlsAddr(srv)})

	row, err := svc.resolveKeyActor(ctx, actorURL+"#main-key")
	if err != nil {
		t.Fatalf("self-claimed actor rejected: %v", err)
	}
	if row.ActorUrl != actorURL {
		t.Fatalf("actor_url: %q", row.ActorUrl)
	}
}

// A stale signer row + an unreachable remote must not stall dispatch —
// Delete{actor} in particular arrives precisely when the remote can never
// refresh again. The cached row's identity is what dispatch needs.
func TestDBInboundStaleSignerFallsBackToCacheRow(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	remote, actorURL := newRemoteActor(t, ctx, svc, "remote.example")
	if _, err := d.Writer.Exec(ctx,
		`UPDATE remote_actors SET fetched_at = now() - interval '7 hours' WHERE id = $1`, remote.ID); err != nil {
		t.Fatal(err)
	}

	row := runInbound(t, ctx, svc, map[string]any{
		"id":     actorURL + "#stale-f1",
		"type":   "Follow",
		"actor":  actorURL,
		"object": svc.apiBase() + "/federation/actors/" + uname,
	})
	if row.Status != "processed" {
		t.Fatalf("stale-signer follow status %q (reason %v)", row.Status, row.Error)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM follows WHERE follower_remote_actor_id = $1`, remote.ID); n != 1 {
		t.Fatalf("mirror row missing after stale-signer dispatch: %d", n)
	}
}

// rejectInbound must not ledger a foreign-domain activity id — storing it
// would squat the global ap_id dedup the provenance rule protects.
func TestDBRejectInboundStripsForeignActivityID(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newFederationService(t, d)

	act := map[string]any{
		"id":     "https://victim.example/statuses/1#like",
		"type":   "Like",
		"actor":  "https://evil.example/actors/bob",
		"object": "https://api.test.dev/federation/objects/contents/x",
	}
	svc.rejectInbound(ctx, act, "https://evil.example/actors/bob", "test rejection", []byte(`{}`))
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM federation_inbound WHERE activity_ap_id = 'https://victim.example/statuses/1#like'`); n != 0 {
		t.Fatal("foreign-domain activity id ledgered — dedup key squatted")
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM federation_inbound WHERE activity_ap_id IS NULL AND status = 'rejected' AND error = 'test rejection'`); n != 1 {
		t.Fatalf("rejected audit row missing: %d", n)
	}
}

// A follower advertising no inbox can never receive the Accept — the
// edge still lands; the activity must not error-retry forever.
func TestDBFollowNoInboxProcessed(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	actorURL := fmt.Sprintf("https://noinbox.example/actors/%d", time.Now().UnixNano())
	remote, err := svc.store.upsertRemoteActor(ctx, gen.UpsertRemoteActorParams{
		ActorUrl:          actorURL,
		Domain:            "noinbox.example",
		PublicKeyPem:      strPtr("-----BEGIN PUBLIC KEY-----\nfake\n-----END PUBLIC KEY-----"),
		PublicKeyID:       strPtr(actorURL + "#main-key"),
		PreferredUsername: strPtr("ghost"),
	})
	if err != nil {
		t.Fatalf("seed inboxless remote: %v", err)
	}
	t.Cleanup(func() {
		_, _ = svc.db.Writer.Exec(context.Background(), `DELETE FROM remote_actors WHERE id = $1`, remote.ID)
	})

	row := runInbound(t, ctx, svc, map[string]any{
		"id":     actorURL + "#f-noinbox",
		"type":   "Follow",
		"actor":  actorURL,
		"object": svc.apiBase() + "/federation/actors/" + uname,
	})
	if row.Status != "processed" {
		t.Fatalf("inboxless follow status %q (reason %v)", row.Status, row.Error)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM federation_follows WHERE remote_actor_id = $1 AND status = 'accepted'`, remote.ID); n != 1 {
		t.Fatalf("edge not honored: %d", n)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM federation_deliveries WHERE remote_actor_id = $1`, remote.ID); n != 0 {
		t.Fatalf("phantom delivery rows: %d", n)
	}
}

// A 429 is a throttle, not a refusal — the delivery retries on the
// ledger's backoff instead of dead-lettering on one attempt.
func TestDBDelivery429Retries(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	actor, err := svc.store.actorByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("actor: %v", err)
	}

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	old := deliverClient.Transport
	deliverClient.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	t.Cleanup(func() { deliverClient.Transport = old })

	row, err := gen.New(d.Writer).InsertFederationDelivery(ctx, gen.InsertFederationDeliveryParams{
		ActivityApID: fmt.Sprintf("https://api.test.dev/x#c-%d", time.Now().UnixNano()),
		ActivityType: "Create",
		ActivityJson: []byte(`{"type":"Create"}`),
		LocalActorID: actor.ID,
		InboxUrl:     srv.URL + "/inbox",
	})
	if err != nil {
		t.Fatalf("insert delivery: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.Writer.Exec(context.Background(), `DELETE FROM federation_deliveries WHERE id = $1`, row.ID)
	})

	if err := svc.attemptDelivery(ctx, row); err != nil {
		t.Fatalf("attemptDelivery: %v", err)
	}
	var status string
	var attempts int32
	if err := d.Reader.QueryRow(ctx,
		`SELECT status, attempts FROM federation_deliveries WHERE id = $1`, row.ID).Scan(&status, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != "failed" || attempts != 1 {
		t.Fatalf("429 outcome: status=%q attempts=%d, want failed/1", status, attempts)
	}
}

// A disabled author's objects 410 publicly — remote engagement landing
// (and notifying) on them after the Delete{actor} fanout is incoherent.
func TestDBInboundLikeDisabledAuthorIgnored(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	contentID, objIRI := newFederatedContent(t, ctx, svc, userID)
	if _, err := svc.DisableFederation(ctx, userID); err != nil {
		t.Fatalf("disable: %v", err)
	}
	remote, actorURL := newRemoteActor(t, ctx, svc, "remote.example")

	row := runInbound(t, ctx, svc, map[string]any{
		"id": actorURL + "#l-disabled", "type": "Like", "actor": actorURL, "object": objIRI,
	})
	if row.Status != "ignored" {
		t.Fatalf("like on disabled author's object: status %q", row.Status)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM reactions WHERE entity_id = $1 AND remote_actor_id = $2`,
		contentID, remote.ID); n != 0 {
		t.Fatalf("reaction landed on disabled author's object: %d", n)
	}
}

// HasDeliveredObject → GetLatestDeliveredObjectType: a Create that never
// arrived (dead delivery) proves nothing to the remote — an Update after
// it would be ignored there, so re-fanout must emit another Create with
// a fresh discriminated ap_id.
func TestDBDeadCreateRefanoutStillCreates(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	cleanDeliveries(t, ctx, svc)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	_, actorURL := newRemoteActor(t, ctx, svc, "remote.example")
	if row := runInbound(t, ctx, svc, map[string]any{
		"type": "Follow", "actor": actorURL,
		"object": svc.apiBase() + "/federation/actors/" + uname,
	}); row.Status != "processed" {
		t.Fatalf("follow: %q", row.Status)
	}
	contentID, objIRI := newFederatedContent(t, ctx, svc, userID)

	if err := svc.Fanout(ctx, contentID); err != nil {
		t.Fatalf("fanout: %v", err)
	}
	// Every inbox dead-lettered — nothing remote ever saw the object.
	if _, err := d.Writer.Exec(ctx,
		`UPDATE federation_deliveries SET status = 'dead'`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Writer.Exec(ctx,
		`UPDATE contents SET updated_at = updated_at + interval '1 second' WHERE id = $1`, contentID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Fanout(ctx, contentID); err != nil {
		t.Fatalf("re-fanout: %v", err)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(DISTINCT activity_ap_id) FROM federation_deliveries
		 WHERE activity_type = 'Create' AND activity_ap_id LIKE $1`, objIRI+"#create/%"); n != 2 {
		t.Fatalf("create deliveries after dead-letter re-fanout: %d (want 2)", n)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM federation_deliveries
		 WHERE activity_type = 'Update' AND activity_json->'object'->>'id' = $1`, objIRI); n != 0 {
		t.Fatalf("update deliveries after dead-letter re-fanout: %d (want 0)", n)
	}
}

// Republish after a delivered Delete: the remote holds a tombstone, so
// the lifecycle must restart at Create — and the second Create's ap_id
// must differ from the first's or (activity_ap_id, inbox) dedup and
// remote-side activity-id dedup would swallow it.
func TestDBRepublishAfterDeleteSendsCreate(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	cleanDeliveries(t, ctx, svc)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	_, actorURL := newRemoteActor(t, ctx, svc, "remote.example")
	if row := runInbound(t, ctx, svc, map[string]any{
		"type": "Follow", "actor": actorURL,
		"object": svc.apiBase() + "/federation/actors/" + uname,
	}); row.Status != "processed" {
		t.Fatalf("follow: %q", row.Status)
	}
	contentID, objIRI := newFederatedContent(t, ctx, svc, userID)

	if err := svc.Fanout(ctx, contentID); err != nil {
		t.Fatalf("fanout: %v", err)
	}
	if _, err := d.Writer.Exec(ctx,
		`UPDATE federation_deliveries SET status = 'delivered'`); err != nil {
		t.Fatal(err)
	}
	if err := svc.fanoutDeleteIfDelivered(ctx, contentID); err != nil {
		t.Fatalf("delete fanout: %v", err)
	}
	if _, err := d.Writer.Exec(ctx,
		`UPDATE federation_deliveries SET status = 'delivered'`); err != nil {
		t.Fatal(err)
	}
	// Republish carries a fresh updated_at.
	if _, err := d.Writer.Exec(ctx,
		`UPDATE contents SET updated_at = updated_at + interval '1 second' WHERE id = $1`, contentID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Fanout(ctx, contentID); err != nil {
		t.Fatalf("republish fanout: %v", err)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(DISTINCT activity_ap_id) FROM federation_deliveries
		 WHERE activity_type = 'Create' AND activity_ap_id LIKE $1`, objIRI+"#create/%"); n != 2 {
		t.Fatalf("create deliveries after republish: %d (want 2)", n)
	}
	if n := countRows(t, ctx, svc,
		`SELECT COUNT(*) FROM federation_deliveries
		 WHERE activity_type = 'Update' AND activity_json->'object'->>'id' = $1`, objIRI); n != 0 {
		t.Fatalf("update deliveries on tombstone republish: %d (want 0)", n)
	}
}

// Accept/Reject answer outbound Follows — M2 sends none, so an inbound
// one can only desynchronize the ledgers (a Reject flipping the accepted
// edge would strand the native mirror and follower_count; an Accept
// could resurrect an undone edge). It must be ignored, edge untouched.
func TestDBInboundAcceptRejectIgnored(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	remote, actorURL := newRemoteActor(t, ctx, svc, "remote.example")
	localActorURL := svc.apiBase() + "/federation/actors/" + uname

	if row := runInbound(t, ctx, svc, map[string]any{
		"id": actorURL + "#follow-1", "type": "Follow",
		"actor": actorURL, "object": localActorURL,
	}); row.Status != "processed" {
		t.Fatalf("follow: %q", row.Status)
	}
	edgeStatus := func() string {
		var st string
		if err := svc.db.Reader.QueryRow(ctx,
			`SELECT status FROM federation_follows WHERE remote_actor_id = $1`, remote.ID).Scan(&st); err != nil {
			t.Fatalf("read edge: %v", err)
		}
		return st
	}
	if st := edgeStatus(); st != "accepted" {
		t.Fatalf("edge after follow: %q", st)
	}

	for i, typ := range []string{"Reject", "Accept"} {
		row := runInbound(t, ctx, svc, map[string]any{
			"id": fmt.Sprintf("%s#resp-%d", actorURL, i), "type": typ, "actor": actorURL,
			"object": map[string]any{
				"id": actorURL + "#follow-1", "type": "Follow",
				"actor": actorURL, "object": localActorURL,
			},
		})
		if row.Status != "ignored" {
			t.Fatalf("inbound %s of own follow: status %q", typ, row.Status)
		}
	}
	if st := edgeStatus(); st != "accepted" {
		t.Fatalf("edge after accept/reject: %q (want accepted)", st)
	}
}
