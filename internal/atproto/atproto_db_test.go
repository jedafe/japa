package atproto

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
)

// DB integration tests — same harness as federation_db_test.go: real
// Postgres via JC_TEST_DATABASE_URL/JC_DATABASE_URL, skipped otherwise.

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

func newAtprotoUser(t *testing.T, ctx context.Context, d *db.DB, status string) (pgtype.UUID, string) {
	t.Helper()
	uname := fmt.Sprintf("atptest_%d", time.Now().UnixNano())
	var id pgtype.UUID
	err := d.Writer.QueryRow(ctx,
		`INSERT INTO users (username, email, status) VALUES ($1, $2, $3) RETURNING id`,
		uname, uname+"@test.dev", status).Scan(&id)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	// atproto_accounts cascade on users delete — one cleanup covers both.
	t.Cleanup(func() {
		_, _ = d.Writer.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, id)
	})
	return id, uname
}

func newAtprotoService(t *testing.T, d *db.DB) *Service {
	t.Helper()
	return NewService(d, Config{
		PDSURL:       "https://api.test.dev",
		APIBase:      "https://api.test.dev",
		SiteBase:     "https://test.dev",
		HandleDomain: "test.dev",
	})
}

func TestDBEnableDisableAccount(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)

	st, err := svc.EnableAccount(ctx, userID)
	if err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	wantHandle := uname + ".test.dev"
	if !st.Enabled || st.Handle != wantHandle {
		t.Fatalf("unexpected status: %+v", st)
	}
	if st.DID != "did:web:api.test.dev:atproto:u:"+uname {
		t.Fatalf("did = %q", st.DID)
	}

	// Idempotent re-enable returns the same identity.
	st2, err := svc.EnableAccount(ctx, userID)
	if err != nil || st2.DID != st.DID {
		t.Fatalf("re-enable: %v / %q vs %q", err, st2.DID, st.DID)
	}

	// Signing key is ciphertext at rest, never plaintext key material.
	var keyEnc string
	if err := d.Writer.QueryRow(ctx,
		`SELECT signing_key_enc FROM atproto_accounts WHERE user_id = $1`, userID).Scan(&keyEnc); err != nil {
		t.Fatalf("select key: %v", err)
	}
	if len(keyEnc) == 64 { // raw 32-byte hex scalar would be 64 chars
		t.Error("signing_key_enc looks like plaintext key material — expected ciphertext")
	}

	st, err = svc.DisableAccount(ctx, userID)
	if err != nil {
		t.Fatalf("DisableAccount: %v", err)
	}
	if st.Enabled || st.Status != AccountStatusDisabled {
		t.Fatalf("disabled status: %+v", st)
	}

	// Disabled accounts don't resolve.
	if _, err := svc.ResolveHandleDID(ctx, wantHandle); err != ErrNotFound {
		t.Errorf("disabled handle resolved: %v", err)
	}
}

func TestDBHandleResolution(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)

	st, err := svc.EnableAccount(ctx, userID)
	if err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}

	// Case-insensitive handle resolution.
	did, err := svc.ResolveHandleDID(ctx, uname+".TEST.DEV")
	if err != nil || did != st.DID {
		t.Errorf("ResolveHandleDID casing: %v / %q", err, did)
	}

	// did:web doc resolves and carries the right fields.
	doc, err := svc.DIDDocument(ctx, uname)
	if err != nil {
		t.Fatalf("DIDDocument: %v", err)
	}
	if doc["id"] != st.DID {
		t.Errorf("doc id = %v", doc["id"])
	}
	vms := doc["verificationMethod"].([]map[string]any)
	if vms[0]["publicKeyMultibase"] == "" {
		t.Error("missing publicKeyMultibase")
	}

	// Unknown handle → ErrNotFound.
	if _, err := svc.ResolveHandleDID(ctx, "ghost.test.dev"); err != ErrNotFound {
		t.Errorf("unknown handle: %v", err)
	}
}

func TestDBSuspendedUserNotFederated(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)

	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	if _, err := d.Writer.Exec(ctx, `UPDATE users SET status = 'suspended' WHERE id = $1`, userID); err != nil {
		t.Fatalf("suspend user: %v", err)
	}

	if _, err := svc.ResolveHandleDID(ctx, uname+".test.dev"); err != ErrNotFound {
		t.Errorf("suspended user's handle resolved: %v", err)
	}
	if _, err := svc.DIDDocument(ctx, uname); err != ErrNotFound {
		t.Errorf("suspended user's DID doc served: %v", err)
	}
	// Suspended user can't re-enable through the upsert path either.
	if _, err := svc.EnableAccount(ctx, userID); err != ErrNotFound {
		t.Errorf("suspended enable: %v", err)
	}
}

func TestDBAdminListHidesKey(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)
	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}

	accounts, total, err := svc.ListAccounts(ctx, "", 10, 0)
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	if total < 1 || len(accounts) < 1 {
		t.Fatalf("expected accounts, got total=%d len=%d", total, len(accounts))
	}
	for _, a := range accounts {
		if a.SigningKeyEnc != "" {
			t.Error("admin row carries key ciphertext — sanitization failed")
		}
	}
}
