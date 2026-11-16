package federation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/valkey-io/valkey-go"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// DB integration tests — follow the refund_db_test.go pattern: real
// Postgres via JC_TEST_DATABASE_URL/JC_DATABASE_URL, skipped otherwise.
// Covers the paths pure unit tests can't reach: sqlc queries against the
// migrated schema, key encryption at rest, status transitions.

func testDB(t *testing.T) *db.DB {
	t.Helper()
	dsn := os.Getenv("JC_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("JC_DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("JC_TEST_DATABASE_URL not set — skipping DB integration test")
	}
	d, err := db.New(context.Background(), dsn)
	if err != nil {
		t.Skipf("cannot connect to test DB: %v", err)
	}
	t.Cleanup(func() { d.Writer.Close(); d.Reader.Close() })
	return d
}

func newFederationUser(t *testing.T, ctx context.Context, d *db.DB, status string) (pgtype.UUID, string) {
	t.Helper()
	uname := fmt.Sprintf("fedtest_%d", time.Now().UnixNano())
	var id pgtype.UUID
	err := d.Writer.QueryRow(ctx,
		`INSERT INTO users (username, email, status) VALUES ($1, $2, $3) RETURNING id`,
		uname, uname+"@test.dev", status).Scan(&id)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	// federated_actors cascade on users delete — one cleanup covers both.
	t.Cleanup(func() {
		_, _ = d.Writer.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, id)
	})
	return id, uname
}

func newFederationService(t *testing.T, d *db.DB) *Service {
	t.Helper()
	return NewService(d, Config{
		APIBase:      "https://api.test.dev",
		SiteBase:     "https://test.dev",
		HandleDomain: "test.dev",
	})
}

func TestDBEnableDisableFederation(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)

	st, err := svc.EnableFederation(ctx, userID)
	if err != nil {
		t.Fatalf("EnableFederation: %v", err)
	}
	if !st.Enabled || st.ActorURL != "https://api.test.dev/federation/actors/"+uname {
		t.Fatalf("unexpected status: %+v", st)
	}
	if st.Handle != uname+"@test.dev" {
		t.Errorf("handle: %q", st.Handle)
	}

	// Private key must be ciphertext at rest — never the PEM.
	var enc string
	if err := d.Reader.QueryRow(ctx,
		`SELECT private_key_enc FROM federated_actors WHERE user_id = $1`, userID).Scan(&enc); err != nil {
		t.Fatalf("read private_key_enc: %v", err)
	}
	if strings.Contains(enc, "PRIVATE KEY") || !strings.HasPrefix(enc, "v") {
		t.Errorf("private key not stored as internal/crypto ciphertext: %q", enc)
	}

	// Re-enable is idempotent — returns the same actor URL.
	st2, err := svc.EnableFederation(ctx, userID)
	if err != nil || st2.ActorURL != st.ActorURL {
		t.Fatalf("re-enable: %+v %v", st2, err)
	}

	st3, err := svc.DisableFederation(ctx, userID)
	if err != nil {
		t.Fatalf("DisableFederation: %v", err)
	}
	if st3.Enabled || st3.Status != ActorStatusDisabled {
		t.Errorf("expected disabled, got %+v", st3)
	}
}

func TestDBWebfingerResolution(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}

	doc, err := svc.Webfinger(ctx, "acct:"+uname+"@test.dev")
	if err != nil {
		t.Fatalf("webfinger: %v", err)
	}
	if doc["subject"] != "acct:"+uname+"@test.dev" {
		t.Errorf("subject: %v", doc["subject"])
	}

	// Uppercase localpart — acct is case-insensitive.
	if _, err := svc.Webfinger(ctx, "acct:"+strings.ToUpper(uname)+"@test.dev"); err != nil {
		t.Errorf("uppercase acct should resolve: %v", err)
	}

	// Foreign domain rejected.
	if _, err := svc.Webfinger(ctx, "acct:"+uname+"@evil.dev"); err != ErrHandleDomain {
		t.Errorf("foreign domain: got %v, want ErrHandleDomain", err)
	}

	// Disabled actor 404s.
	if _, err := svc.DisableFederation(ctx, userID); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if _, err := svc.Webfinger(ctx, "acct:"+uname+"@test.dev"); err != ErrNotFound {
		t.Errorf("disabled actor: got %v, want ErrNotFound", err)
	}
}

func TestDBSuspendedUserNotFederated(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}

	if _, err := d.Writer.Exec(ctx, `UPDATE users SET status = 'suspended' WHERE id = $1`, userID); err != nil {
		t.Fatalf("suspend: %v", err)
	}

	if _, err := svc.ActorDocument(ctx, uname); err != ErrNotFound {
		t.Errorf("suspended user's actor doc: got %v, want ErrNotFound", err)
	}
	if _, err := svc.Webfinger(ctx, "acct:"+uname+"@test.dev"); err != ErrNotFound {
		t.Errorf("suspended user's webfinger: got %v, want ErrNotFound", err)
	}
}

func TestDBActorDocumentAndOutbox(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}

	// Public story in the outbox; private + non-allowlisted excluded.
	var contentID pgtype.UUID
	err := d.Writer.QueryRow(ctx, `
		INSERT INTO contents (author_id, content_type, title, slug, body, body_html,
		                      content_hash, status, visibility, published_at)
		VALUES ($1, 'story', 'Fed hello', 'fed-hello', 'hi', '<p>hi</p>',
		        $2, 'published', 'public', now())
		RETURNING id`, userID, fmt.Sprintf("hash-%d", time.Now().UnixNano())).Scan(&contentID)
	if err != nil {
		t.Fatalf("insert content: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.Writer.Exec(context.Background(), `DELETE FROM contents WHERE id = $1`, contentID)
	})
	var privateID pgtype.UUID
	if err := d.Writer.QueryRow(ctx, `
		INSERT INTO contents (author_id, content_type, title, slug, body, body_html,
		                      content_hash, status, visibility, published_at)
		VALUES ($1, 'story', 'Private', 'fed-private', 'x', '<p>x</p>',
		        $2, 'published', 'private', now())
		RETURNING id`, userID, fmt.Sprintf("hashp-%d", time.Now().UnixNano())).Scan(&privateID); err != nil {
		t.Fatalf("insert private content: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.Writer.Exec(context.Background(), `DELETE FROM contents WHERE id = $1`, privateID)
	})

	doc, err := svc.ActorDocument(ctx, uname)
	if err != nil {
		t.Fatalf("actor doc: %v", err)
	}
	if doc["type"] != "Person" || doc["preferredUsername"] != uname {
		t.Errorf("actor doc: %v", doc)
	}

	obj, err := svc.ObjectDocument(ctx, contentID)
	if err != nil {
		t.Fatalf("object doc: %v", err)
	}
	if obj["type"] != "Note" || obj["url"] != "https://test.dev/story/fed-hello" {
		t.Errorf("object doc: %v", obj)
	}

	// Private content is not federatable.
	if _, err := svc.ObjectDocument(ctx, privateID); err != ErrNotFound {
		t.Errorf("private object: got %v, want ErrNotFound", err)
	}

	outbox, err := svc.Outbox(ctx, uname, 0)
	if err != nil {
		t.Fatalf("outbox: %v", err)
	}
	if outbox["totalItems"] != int64(1) {
		t.Errorf("totalItems: %v", outbox["totalItems"])
	}
	page, err := svc.Outbox(ctx, uname, 1)
	if err != nil {
		t.Fatalf("outbox page: %v", err)
	}
	items := page["orderedItems"].([]any)
	if len(items) != 1 {
		t.Fatalf("orderedItems: %v", items)
	}
	if items[0].(map[string]any)["type"] != "Create" {
		t.Errorf("expected Create activity, got %v", items[0])
	}
}

func TestDBAdminListHidesPrivateKey(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)
	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}

	rows, total, err := svc.ListActors(ctx, "fedtest_", "created_at", "desc", 50, 0)
	if err != nil {
		t.Fatalf("ListActors: %v", err)
	}
	if total < 1 || len(rows) < 1 {
		t.Fatalf("expected rows, total=%d", total)
	}
	for _, r := range rows {
		if r.PrivateKeyEnc != "" {
			t.Error("admin row leaked private_key_enc")
		}
	}
}

// TestDBAutoProvision covers the §126 auto opt-in path — the worker task
// provisions only when no actor row exists, so a member's explicit opt-out
// (disabled row) is never silently resurrected by a retried task.
func TestDBAutoProvision(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)

	// Absent row → mints the actor, same as EnableFederation.
	if err := svc.AutoProvision(ctx, userID); err != nil {
		t.Fatalf("AutoProvision: %v", err)
	}
	st, err := svc.GetMyFederation(ctx, userID)
	if err != nil || !st.Enabled || st.ActorURL != "https://api.test.dev/federation/actors/"+uname {
		t.Fatalf("expected active actor, got %+v %v", st, err)
	}

	// Idempotent — a retried task against an active row is a no-op and
	// keeps the same actor URL (no key rotation).
	if err := svc.AutoProvision(ctx, userID); err != nil {
		t.Fatalf("AutoProvision idempotent: %v", err)
	}
	st2, _ := svc.GetMyFederation(ctx, userID)
	if st2.ActorURL != st.ActorURL {
		t.Fatalf("actor URL changed on retry: %q → %q", st.ActorURL, st2.ActorURL)
	}

	// Explicit opt-out → the row goes disabled; a late/retried task must
	// leave it disabled.
	if _, err := svc.DisableFederation(ctx, userID); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if err := svc.AutoProvision(ctx, userID); err != nil {
		t.Fatalf("AutoProvision after opt-out: %v", err)
	}
	st3, _ := svc.GetMyFederation(ctx, userID)
	if st3.Enabled || st3.Status != ActorStatusDisabled {
		t.Fatalf("opt-out resurrected: %+v", st3)
	}

	// Suspended/deleted member → ineligible, not an error — a suspended
	// signup between enqueue and run must not burn task retries.
	suspendedID, _ := newFederationUser(t, ctx, d, "suspended")
	if err := svc.AutoProvision(ctx, suspendedID); err != nil {
		t.Fatalf("AutoProvision suspended user: %v", err)
	}
	if st, _ := svc.GetMyFederation(ctx, suspendedID); st.Enabled {
		t.Fatalf("suspended user provisioned: %+v", st)
	}
}

func TestDBInstanceBlockCRUD(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)

	block, err := svc.CreateBlock(ctx, "Spam.Example ", "test block", userID)
	if err != nil {
		t.Fatalf("CreateBlock: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.Writer.Exec(context.Background(), `DELETE FROM federation_instance_blocks WHERE id = $1`, block.ID)
	})
	if block.Domain != "spam.example" {
		t.Errorf("domain not normalized: %q", block.Domain)
	}

	for _, bad := range []string{
		"bad/host",
		"evil.com:443", // ports never match a URL-derived host
		"evil.com:443/path",
		strings.Repeat("a", 254), // over the DNS name limit
		"white space.example",
	} {
		if _, err := svc.CreateBlock(ctx, bad, "", userID); err != ErrInvalidDomain {
			t.Errorf("domain %q: got %v, want ErrInvalidDomain", bad, err)
		}
	}

	if err := svc.DeleteBlock(ctx, block.ID); err != nil {
		t.Errorf("DeleteBlock: %v", err)
	}
}

func TestDBTombstoneNeverResurrected(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)

	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	// M2's erasure path writes status='tombstone' — simulate it directly.
	if _, err := d.Writer.Exec(ctx,
		`UPDATE federated_actors SET status = 'tombstone' WHERE user_id = $1`, userID); err != nil {
		t.Fatalf("tombstone: %v", err)
	}

	// Re-opting-in must NOT revive an erased identity.
	if _, err := svc.EnableFederation(ctx, userID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("enable over tombstone: got %v, want ErrNotFound", err)
	}
	// The disable path must not flip tombstone back to disabled either —
	// that would reopen the resurrection the enable path refuses.
	if _, err := svc.DisableFederation(ctx, userID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("disable over tombstone: got %v, want ErrNotFound", err)
	}
	var status string
	if err := d.Reader.QueryRow(ctx,
		`SELECT status FROM federated_actors WHERE user_id = $1`, userID).Scan(&status); err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status != "tombstone" {
		t.Fatalf("tombstone resurrected to %q", status)
	}
	// The member-facing status reads "none" — the erased identity is
	// gone, not listed as a disabled actor.
	if st, err := svc.GetMyFederation(ctx, userID); err != nil || st.Status != "none" {
		t.Fatalf("tombstone member status: got %+v %v, want status=none", st, err)
	}

	// Disabled → re-enable still works (the only legal reactivation).
	if _, err := d.Writer.Exec(ctx,
		`UPDATE federated_actors SET status = 'disabled' WHERE user_id = $1`, userID); err != nil {
		t.Fatalf("disable: %v", err)
	}
	st, err := svc.EnableFederation(ctx, userID)
	if err != nil || !st.Enabled {
		t.Fatalf("disabled re-enable should work, got %+v %v", st, err)
	}
}

func TestDBAutoProvisionMissingUser(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newFederationService(t, d)

	// A user hard-deleted between enqueue and task execution — pgx.ErrNoRows
	// must surface as ErrNotFound so the task ends clean, not retry to DLQ.
	ghost := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	if err := svc.AutoProvision(ctx, ghost); err != nil {
		t.Fatalf("AutoProvision missing user: got %v, want nil (clean skip)", err)
	}
	if _, err := svc.EnableFederation(ctx, ghost); !errors.Is(err, ErrNotFound) {
		t.Fatalf("EnableFederation missing user: got %v, want ErrNotFound", err)
	}
}

func TestDBCollectionsAndFollowerCountExcludeSuspended(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	targetID, targetUname := newFederationUser(t, ctx, d, "active")
	followerID, followerName := newFederationUser(t, ctx, d, "active")
	svc := newFederationService(t, d)

	if _, err := svc.EnableFederation(ctx, targetID); err != nil {
		t.Fatalf("enable target: %v", err)
	}
	fst, err := svc.EnableFederation(ctx, followerID)
	if err != nil {
		t.Fatalf("enable follower: %v", err)
	}
	followerActorURL := fst.ActorURL

	// followerID follows targetID — follower shows up in the target's
	// followers collection because they're themselves federated.
	if _, err := d.Writer.Exec(ctx,
		`INSERT INTO follows (follower_id, entity_type, entity_id) VALUES ($1, 'user', $2)`,
		followerID, targetID); err != nil {
		t.Fatalf("insert follow: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.Writer.Exec(context.Background(),
			`DELETE FROM follows WHERE follower_id = $1 AND entity_type = 'user' AND entity_id = $2`,
			followerID, targetID)
	})

	page, err := svc.Followers(ctx, targetUname, 1)
	if err != nil {
		t.Fatalf("followers: %v", err)
	}
	items, _ := page["orderedItems"].([]any)
	found := false
	for _, it := range items {
		if it == followerActorURL {
			found = true
		}
	}
	if !found {
		t.Fatalf("federated follower %s missing from collection %v", followerName, items)
	}
	if page["totalItems"].(int64) != 1 {
		t.Fatalf("totalItems: %v", page["totalItems"])
	}

	// follower_count on /me/federation mirrors the collection total.
	st, err := svc.GetMyFederation(ctx, targetID)
	if err != nil || st.FollowerCount != 1 {
		t.Fatalf("follower_count: got %+v %v", st, err)
	}

	// Suspend the follower — their actor URL must leave the collection
	// (the doc 404s; listing it would be a dangling reference).
	if _, err := d.Writer.Exec(ctx, `UPDATE users SET status = 'suspended' WHERE id = $1`, followerID); err != nil {
		t.Fatalf("suspend follower: %v", err)
	}
	page, err = svc.Followers(ctx, targetUname, 1)
	if err != nil {
		t.Fatalf("followers after suspend: %v", err)
	}
	if page["totalItems"].(int64) != 0 {
		t.Fatalf("suspended follower still counted: %v", page["totalItems"])
	}
	if items, _ := page["orderedItems"].([]any); len(items) != 0 {
		t.Fatalf("suspended follower still listed: %v", items)
	}
	st, err = svc.GetMyFederation(ctx, targetID)
	if err != nil || st.FollowerCount != 0 {
		t.Fatalf("follower_count after suspend: got %+v %v", st, err)
	}

	// Same predicate on the following side: a suspended member the target
	// follows must drop out of their following collection.
	if _, err := d.Writer.Exec(ctx, `UPDATE users SET status = 'active' WHERE id = $1`, followerID); err != nil {
		t.Fatalf("restore follower: %v", err)
	}
	followingPage, err := svc.Following(ctx, followerName, 1)
	if err != nil {
		t.Fatalf("following: %v", err)
	}
	if followingPage["totalItems"].(int64) != 1 {
		t.Fatalf("following totalItems: %v", followingPage["totalItems"])
	}
	if _, err := d.Writer.Exec(ctx, `UPDATE users SET status = 'suspended' WHERE id = $1`, targetID); err != nil {
		t.Fatalf("suspend target: %v", err)
	}
	followingPage, err = svc.Following(ctx, followerName, 1)
	if err != nil {
		t.Fatalf("following after suspend: %v", err)
	}
	if followingPage["totalItems"].(int64) != 0 {
		t.Fatalf("suspended followee still counted: %v", followingPage["totalItems"])
	}
}

// TestDBLifecycleCacheInvalidation covers InvalidateActorIdentityForUser —
// fired by atproto:lifecycle_sync on transitions OUT of active, it must
// resolve the actor's handles even though the status-gated lookups can no
// longer see the suspended member, and drop both doc caches.
func TestDBLifecycleCacheInvalidation(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newFederationUser(t, ctx, d, "active")

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	vk, err := valkey.NewClient(valkey.ClientOption{InitAddress: []string{mr.Addr()}, DisableCache: true})
	if err != nil {
		t.Fatalf("valkey: %v", err)
	}
	t.Cleanup(vk.Close)

	svc := newFederationService(t, d)
	svc.SetValkey(valkeycompat.NewAdapter(vk))

	if _, err := svc.EnableFederation(ctx, userID); err != nil {
		t.Fatalf("enable: %v", err)
	}
	// Warm both doc caches.
	if _, err := svc.Webfinger(ctx, "acct:"+uname+"@test.dev"); err != nil {
		t.Fatalf("webfinger warm: %v", err)
	}
	if _, err := svc.ActorDocument(ctx, uname); err != nil {
		t.Fatalf("actor warm: %v", err)
	}
	for _, k := range []string{"jc:fed:webfinger:" + uname, "jc:fed:actor:" + uname} {
		if !mr.Exists(k) {
			t.Fatalf("%s not cached after warm", k)
		}
	}

	// Suspend, then invalidate as the lifecycle task would — the cached
	// docs must go even though actorProfile/actorByUsername now 404. The
	// extras path (a vacated handle the rename caller passes) is covered
	// by the same call.
	for _, k := range []string{"jc:fed:webfinger:oldhandle", "jc:fed:actor:oldhandle"} {
		svc.cacheSet(ctx, k, map[string]any{"u": "oldhandle"}, time.Minute)
	}
	if _, err := d.Writer.Exec(ctx, `UPDATE users SET status = 'suspended' WHERE id = $1`, userID); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	svc.InvalidateActorIdentityForUser(ctx, userID, "oldhandle")
	for _, k := range []string{"jc:fed:webfinger:" + uname, "jc:fed:actor:" + uname,
		"jc:fed:webfinger:oldhandle", "jc:fed:actor:oldhandle"} {
		if mr.Exists(k) {
			t.Errorf("%s survived lifecycle invalidation", k)
		}
	}
}
