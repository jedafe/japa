package atproto

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/valkey-io/valkey-go"
	"github.com/valkey-io/valkey-go/valkeycompat"
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
	return newAtprotoNamedUser(t, ctx, d, fmt.Sprintf("atptest%d", time.Now().UnixNano()), status)
}

// newAtprotoNamedUser inserts a user with an explicit username — tests
// that need specific names (caseful, underscore-bearing) use it directly.
// Generated names must be handle-legal: atproto labels reject '_', so the
// default generator produces "atptest<n>" (no underscore) so generic
// tests exercise the happy path.
func newAtprotoNamedUser(t *testing.T, ctx context.Context, d *db.DB, uname, status string) (pgtype.UUID, string) {
	t.Helper()
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

// startTestCompat wires an in-memory Valkey (miniredis) behind the
// go-redis-compatible Cmdable the service caches through — mirrors
// internal/valkey/cachehelper_test.go's startTestCompat.
func startTestCompat(t *testing.T) valkeycompat.Cmdable {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	vk, err := valkey.NewClient(valkey.ClientOption{
		InitAddress:  []string{mr.Addr()},
		DisableCache: true,
	})
	if err != nil {
		t.Fatalf("valkey.NewClient: %v", err)
	}
	t.Cleanup(vk.Close)
	return valkeycompat.NewAdapter(vk)
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
	if st.DID != "did:web:api.test.dev:atproto:u:"+uuidString(userID) {
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

	// did:web doc resolves at the u/{user_uuid} path and carries the
	// right fields.
	doc, err := svc.DIDDocument(ctx, uuidString(userID))
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
	if _, err := svc.DIDDocument(ctx, uuidString(userID)); err != ErrNotFound {
		t.Errorf("suspended user's DID doc served: %v", err)
	}
	// Suspended user can't re-enable through the upsert path either.
	if _, err := svc.EnableAccount(ctx, userID); err != ErrNotFound {
		t.Errorf("suspended enable: %v", err)
	}
}

// Case-insensitive handle claims: users 'Alice' and 'alice' are distinct
// (users.username is case-sensitively unique) but the normalized handle
// alice.test.dev can only belong to one account — first come, first
// served; the loser gets ErrHandleTaken.
func TestDBHandleClaimCaseInsensitive(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newAtprotoService(t, d)

	// Two users whose usernames differ only by case — legal under
	// users.username's case-sensitive uniqueness.
	base := fmt.Sprintf("atpTwin%d", time.Now().UnixNano())
	var upperID, lowerID pgtype.UUID
	if err := d.Writer.QueryRow(ctx,
		`INSERT INTO users (username, email, status) VALUES ($1, $2, 'active') RETURNING id`,
		strings.ToUpper(base), strings.ToUpper(base)+"@test.dev").Scan(&upperID); err != nil {
		t.Fatalf("insert upper user: %v", err)
	}
	if err := d.Writer.QueryRow(ctx,
		`INSERT INTO users (username, email, status) VALUES ($1, $2, 'active') RETURNING id`,
		strings.ToLower(base), strings.ToLower(base)+"@test.dev").Scan(&lowerID); err != nil {
		t.Fatalf("insert lower user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.Writer.Exec(context.Background(), `DELETE FROM users WHERE id IN ($1,$2)`, upperID, lowerID)
	})

	st, err := svc.EnableAccount(ctx, upperID)
	if err != nil {
		t.Fatalf("EnableAccount (upper): %v", err)
	}
	wantHandle := strings.ToLower(base) + ".test.dev" // normalized lowercase
	if st.Handle != wantHandle {
		t.Fatalf("handle = %q, want lowercase %q", st.Handle, wantHandle)
	}

	if _, err := svc.EnableAccount(ctx, lowerID); err != ErrHandleTaken {
		t.Errorf("lowercase twin enable: got %v, want ErrHandleTaken", err)
	}
}

// Rename coherence: after a username change the new subdomain resolves
// to the same permanent DID and the stored handle lazily syncs — the
// did:web path (immutable u:{user_uuid}) keeps serving the doc.
func TestDBRenameResolution(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)

	st, err := svc.EnableAccount(ctx, userID)
	if err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	renamed := uname + "_renamed"
	if _, err := d.Writer.Exec(ctx, `UPDATE users SET username = $1 WHERE id = $2`, renamed, userID); err != nil {
		t.Fatalf("rename user: %v", err)
	}

	// New subdomain resolves to the permanent DID.
	did, err := svc.ResolveHandleDID(ctx, renamed+".test.dev")
	if err != nil || did != st.DID {
		t.Fatalf("renamed handle resolution: %v / %q", err, did)
	}

	// Stored handle lazily synced to the live username.
	stored, err := svc.ResolveHandleDID(ctx, renamed+".test.dev")
	if err != nil || stored != st.DID {
		t.Fatalf("post-sync resolve: %v / %q", err, stored)
	}
	var handle string
	if err := d.Writer.QueryRow(ctx,
		`SELECT handle FROM atproto_accounts WHERE user_id = $1`, userID).Scan(&handle); err != nil {
		t.Fatalf("select handle: %v", err)
	}
	if handle != renamed+".test.dev" {
		t.Errorf("stored handle = %q, want lazy sync to %q", handle, renamed+".test.dev")
	}

	// The abandoned old subdomain no longer resolves post-sync.
	if _, err := svc.ResolveHandleDID(ctx, uname+".test.dev"); err != ErrNotFound {
		t.Errorf("old handle still resolves: %v", err)
	}

	// The immutable did:web path (user UUID — rename-independent) still
	// serves the doc.
	doc, err := svc.DIDDocument(ctx, uuidString(userID))
	if err != nil {
		t.Fatalf("DIDDocument via uuid path: %v", err)
	}
	if doc["id"] != st.DID {
		t.Errorf("doc id = %v, want %q", doc["id"], st.DID)
	}
	aka, _ := doc["alsoKnownAs"].([]string)
	if len(aka) != 1 || aka[0] != "at://"+renamed+".test.dev" {
		t.Errorf("alsoKnownAs = %v, want live handle", aka)
	}
}

// Stale-handle reads must not poison resolution: when a request hits a
// handle whose claim just migrated away, the row's DID must NOT be
// returned or cached — the handle is now unclaimed.
func TestDBStaleHandleResolvesNotFound(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)

	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	// Rename without touching any read path — the stored handle is stale.
	if _, err := d.Writer.Exec(ctx, `UPDATE users SET username = $1 WHERE id = $2`, uname+"_x", userID); err != nil {
		t.Fatalf("rename: %v", err)
	}

	// The abandoned subdomain resolves nothing — before the post-sync
	// check it returned (and cached) the owner's DID for a freed handle.
	did, err := svc.ResolveHandleDID(ctx, uname+".test.dev")
	if err != ErrNotFound {
		t.Fatalf("stale handle resolved: did=%q err=%v", did, err)
	}
}

// Stale-claim reclaim: an account row whose holder renamed away still
// occupies the old normalized handle — a new member legitimately named
// after it must win the claim (the stale row lazy-syncs, freeing it).
func TestDBStaleClaimReclaimed(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newAtprotoService(t, d)
	base := fmt.Sprintf("atpstale%d", time.Now().UnixNano())

	// Charlie mints 'charlie.japa.to' then renames away — stored handle
	// goes stale (no read path has synced it yet).
	charlieID, charlieName := newAtprotoUser(t, ctx, d, "active")
	charlieName = charlieName + "-" + base // unique-ish (hyphen: handle-legal)
	if _, err := d.Writer.Exec(ctx, `UPDATE users SET username = $1 WHERE id = $2`, charlieName, charlieID); err != nil {
		t.Fatalf("name charlie: %v", err)
	}
	charlieSt, err := svc.EnableAccount(ctx, charlieID)
	if err != nil {
		t.Fatalf("EnableAccount (charlie): %v", err)
	}
	// Rename Charlie so his stored handle is orphaned.
	movedName := charlieName + "_moved"
	if _, err := d.Writer.Exec(ctx, `UPDATE users SET username = $1 WHERE id = $2`, movedName, charlieID); err != nil {
		t.Fatalf("rename charlie: %v", err)
	}

	// New member Bob legitimately claims the abandoned username.
	bobID, _ := newAtprotoUser(t, ctx, d, "active")
	if _, err := d.Writer.Exec(ctx, `UPDATE users SET username = $1 WHERE id = $2`, charlieName, bobID); err != nil {
		t.Fatalf("name bob: %v", err)
	}
	bobSt, err := svc.EnableAccount(ctx, bobID)
	if err != nil {
		t.Fatalf("EnableAccount (bob): %v — stale claim should have been reclaimed", err)
	}
	if bobSt.Handle != strings.ToLower(charlieName)+".test.dev" {
		t.Errorf("bob handle = %q", bobSt.Handle)
	}
	if bobSt.DID == charlieSt.DID {
		t.Error("bob minted charlie's DID")
	}

	// Charlie's row now carries his live handle.
	var handle string
	if err := d.Writer.QueryRow(ctx,
		`SELECT handle FROM atproto_accounts WHERE user_id = $1`, charlieID).Scan(&handle); err != nil {
		t.Fatalf("select charlie handle: %v", err)
	}
	if handle != strings.ToLower(movedName)+".test.dev" {
		t.Errorf("charlie stored handle = %q, want synced %q", handle, strings.ToLower(movedName)+".test.dev")
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

// Caseful usernames: users.username preserves case but handles are
// normalized lowercase. A rename to a caseful name must still resolve
// publicly — the live-username precedence compares lower(u.username)
// (pre-fix: u.username = @username never matched a caseful live name,
// so the new subdomain could only self-heal via a member-side read).
func TestDBCasefulRenameResolution(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newAtprotoService(t, d)

	userID, uname := newAtprotoNamedUser(t, ctx, d,
		fmt.Sprintf("CaseUser%d", time.Now().UnixNano()), "active")
	st, err := svc.EnableAccount(ctx, userID)
	if err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	if st.Handle != strings.ToLower(uname)+".test.dev" {
		t.Fatalf("handle = %q, want normalized %q", st.Handle, strings.ToLower(uname)+".test.dev")
	}

	renamed := uname + "New"
	if _, err := d.Writer.Exec(ctx, `UPDATE users SET username = $1 WHERE id = $2`, renamed, userID); err != nil {
		t.Fatalf("rename: %v", err)
	}

	did, err := svc.ResolveHandleDID(ctx, strings.ToLower(renamed)+".test.dev")
	if err != nil || did != st.DID {
		t.Fatalf("caseful rename resolution: %v / %q", err, did)
	}
}

// Underscore usernames are legal under §50 but can't form an atproto
// handle label — opt-in must fail with ErrInvalidHandle rather than mint
// an account that verifies as handle.invalid on the network.
func TestDBUnderscoreUsernameRejected(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newAtprotoService(t, d)

	userID, _ := newAtprotoNamedUser(t, ctx, d,
		fmt.Sprintf("atp_bad%d", time.Now().UnixNano()), "active")
	if _, err := svc.EnableAccount(ctx, userID); err != ErrInvalidHandle {
		t.Errorf("underscore enable: got %v, want ErrInvalidHandle", err)
	}
}

// A handle colliding with the PDS host itself can never resolve —
// HandleFromHost excludes it by design. Opt-in must reject, not mint a
// permanently dead account.
func TestDBPDSHostCollisionRejected(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newAtprotoService(t, d) // pdsHost api.test.dev, domain test.dev

	userID, _ := newAtprotoNamedUser(t, ctx, d, "api", "active")
	if _, err := svc.EnableAccount(ctx, userID); err != ErrInvalidHandle {
		t.Errorf("pds-host collision enable: got %v, want ErrInvalidHandle", err)
	}
}

// Tombstone is terminal (A6's erasure end-state): the member endpoints
// must neither resurrect it via enable nor downgrade it via disable.
func TestDBTombstoneTerminal(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)

	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	if _, err := d.Writer.Exec(ctx,
		`UPDATE atproto_accounts SET status = 'tombstone' WHERE user_id = $1`, userID); err != nil {
		t.Fatalf("tombstone: %v", err)
	}

	if _, err := svc.EnableAccount(ctx, userID); err != ErrNotFound {
		t.Errorf("enable tombstoned: got %v, want ErrNotFound", err)
	}
	st, err := svc.DisableAccount(ctx, userID)
	if err != nil {
		t.Fatalf("DisableAccount: %v", err)
	}
	if st.Status != AccountStatusTombstone {
		t.Errorf("disable downgraded tombstone to %q", st.Status)
	}
	var status string
	if err := d.Writer.QueryRow(ctx,
		`SELECT status FROM atproto_accounts WHERE user_id = $1`, userID).Scan(&status); err != nil {
		t.Fatalf("select status: %v", err)
	}
	if status != AccountStatusTombstone {
		t.Errorf("stored status = %q, want tombstone", status)
	}
}

// jc:atp:did:{id} is keyed on the USER uuid — the did:web u/{id} path
// component — not the atproto_accounts row PK. Disable must evict both
// the DID-doc cache and the well-known account cache so a deactivated
// member stops resolving immediately, not after the 10m TTL.
func TestDBDisableInvalidatesCaches(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)
	svc.SetValkey(startTestCompat(t))

	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	// Warm both caches.
	if _, err := svc.DIDDocument(ctx, uuidString(userID)); err != nil {
		t.Fatalf("DIDDocument warm: %v", err)
	}
	if _, err := svc.ResolveHandleDID(ctx, uname+".test.dev"); err != nil {
		t.Fatalf("ResolveHandleDID warm: %v", err)
	}

	if _, err := svc.DisableAccount(ctx, userID); err != nil {
		t.Fatalf("DisableAccount: %v", err)
	}

	// Both lookups must now miss — a lingering DID doc cache would keep
	// serving the doc for a deactivated member until TTL expiry.
	if _, err := svc.DIDDocument(ctx, uuidString(userID)); err != ErrNotFound {
		t.Errorf("disabled DID doc still served (cache not invalidated): %v", err)
	}
	if _, err := svc.ResolveHandleDID(ctx, uname+".test.dev"); err != ErrNotFound {
		t.Errorf("disabled handle still resolves (cache not invalidated): %v", err)
	}
}

// An unconfigured PDS must fail closed: opt-in returns ErrMisconfigured
// instead of persisting "did:web::atproto:u:…" ids / "{username}."
// handles that can never resolve.
func TestDBEnableMisconfiguredFailsClosed(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")
	svc := NewService(d, Config{})

	if _, err := svc.EnableAccount(ctx, userID); err != ErrMisconfigured {
		t.Errorf("unconfigured enable: got %v, want ErrMisconfigured", err)
	}
}
