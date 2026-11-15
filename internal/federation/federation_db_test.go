package federation

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
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

	rows, total, err := svc.ListActors(ctx, "fedtest_", 50, 0)
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

	if _, err := svc.CreateBlock(ctx, "bad/host", "", userID); err != ErrInvalidDomain {
		t.Errorf("bad domain: got %v, want ErrInvalidDomain", err)
	}

	if err := svc.DeleteBlock(ctx, block.ID); err != nil {
		t.Errorf("DeleteBlock: %v", err)
	}
}
