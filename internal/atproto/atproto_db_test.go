package atproto

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/bluesky-social/indigo/atproto/atcrypto"
	atrepo "github.com/bluesky-social/indigo/repo"
	"github.com/ipfs/go-cid"
	"github.com/jackc/pgx/v5/pgtype"
	jcrypto "github.com/jedafe/japa/internal/crypto"
	"github.com/jedafe/japa/internal/db"
	gen "github.com/jedafe/japa/internal/db/gen"
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

// ---------------------------------------------------------------------------
// A2 — repo engine
// ---------------------------------------------------------------------------

// newAtprotoContent inserts an eligible contents row for the author —
// title/slug/body derived from the test clock so unique-index collisions
// can't flake parallel runs.
func newAtprotoContent(t *testing.T, ctx context.Context, d *db.DB, authorID pgtype.UUID, contentType, status, visibility string) pgtype.UUID {
	t.Helper()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var id pgtype.UUID
	err := d.Writer.QueryRow(ctx,
		`INSERT INTO contents (author_id, content_type, title, slug, body, status, visibility)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		authorID, contentType, "post "+suffix, "slug-"+suffix, "hello **world** from "+suffix, status, visibility).Scan(&id)
	if err != nil {
		t.Fatalf("insert content: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.Writer.Exec(context.Background(), `DELETE FROM contents WHERE id = $1`, id)
	})
	return id
}

// atprotoAccountFor reads the member's account row.
func atprotoAccountFor(t *testing.T, ctx context.Context, d *db.DB, userID pgtype.UUID) gen.AtprotoAccount {
	t.Helper()
	var a gen.AtprotoAccount
	err := d.Writer.QueryRow(ctx, `SELECT * FROM atproto_accounts WHERE user_id = $1`, userID).Scan(
		&a.ID, &a.UserID, &a.Handle, &a.Did, &a.SigningKeyEnc, &a.PublicKeyMultibase,
		&a.Status, &a.RepoSeq, &a.CreatedAt, &a.UpdatedAt, &a.DisabledAt)
	if err != nil {
		t.Fatalf("read account: %v", err)
	}
	return a
}

// Opt-in must mint the repo root commit — the signed commit chain, MST
// blocks, and the app.bsky.actor.profile/self record all land atomically.
func TestDBRepoInitializedOnOptIn(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)

	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	acct := atprotoAccountFor(t, ctx, d, userID)
	if acct.RepoSeq != 1 {
		t.Fatalf("repo_seq = %d, want 1 (root commit)", acct.RepoSeq)
	}

	var commits, blocks int
	if err := d.Writer.QueryRow(ctx, `SELECT COUNT(*) FROM atproto_repo_commits WHERE account_id = $1`, acct.ID).Scan(&commits); err != nil {
		t.Fatal(err)
	}
	if err := d.Writer.QueryRow(ctx, `SELECT COUNT(*) FROM atproto_repo_blocks WHERE account_id = $1`, acct.ID).Scan(&blocks); err != nil {
		t.Fatal(err)
	}
	if commits != 1 || blocks < 2 {
		t.Fatalf("commits=%d blocks=%d — root commit needs a commit row + MST/record/commit blocks", commits, blocks)
	}

	// The profile record indexes under at://{did}/app.bsky.actor.profile/self.
	var uri, coll, rkey string
	err := d.Writer.QueryRow(ctx,
		`SELECT uri, collection, rkey FROM atproto_records WHERE account_id = $1`, acct.ID).
		Scan(&uri, &coll, &rkey)
	if err != nil {
		t.Fatalf("profile record row: %v", err)
	}
	wantURI := "at://" + acct.Did + "/app.bsky.actor.profile/self"
	if uri != wantURI || coll != "app.bsky.actor.profile" || rkey != "self" {
		t.Fatalf("record index = %q/%q/%q, want %q", uri, coll, rkey, wantURI)
	}

	// Re-opt-in is a no-op for the repo — the root commit already exists.
	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("re-EnableAccount: %v", err)
	}
	if got := atprotoAccountFor(t, ctx, d, userID).RepoSeq; got != 1 {
		t.Fatalf("re-enable bumped repo_seq to %d — extra commits on identical profile", got)
	}
	_ = uname
}

// Feed.post projection: eligible contents → record + commit; delete →
// tombstone commit + index drop. Reopening the repo proves the MST
// actually reflects both writes (not just the index table).
func TestDBRepoPublishAndDeletePost(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)
	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	acct := atprotoAccountFor(t, ctx, d, userID)
	contentID := newAtprotoContent(t, ctx, d, userID, "discussion", "published", "public")

	uri, err := svc.PublishContent(ctx, contentID)
	if err != nil {
		t.Fatalf("PublishContent: %v", err)
	}
	if !strings.HasPrefix(uri, "at://"+acct.Did+"/app.bsky.feed.post/") {
		t.Fatalf("uri = %q", uri)
	}
	if got := atprotoAccountFor(t, ctx, d, userID).RepoSeq; got != 2 {
		t.Fatalf("repo_seq = %d, want 2", got)
	}

	// Index row links the at:// uri back to the contents row.
	var cidStr, contentLink string
	var rkey string
	err = d.Writer.QueryRow(ctx,
		`SELECT cid, rkey, content_id::text FROM atproto_records WHERE uri = $1`, uri).
		Scan(&cidStr, &rkey, &contentLink)
	if err != nil || cidStr == "" || contentLink != uuidString(contentID) {
		t.Fatalf("record index: cid=%q content=%q err=%v", cidStr, contentLink, err)
	}

	// Reopen the repo and walk the MST — both records present.
	var dataCid string
	err = d.Writer.QueryRow(ctx,
		`SELECT cid FROM atproto_repo_commits WHERE account_id = $1 ORDER BY seq DESC LIMIT 1`, acct.ID).Scan(&dataCid)
	if err != nil {
		t.Fatalf("head commit: %v", err)
	}

	if err := svc.DeleteRecord(ctx, acct.ID, "app.bsky.feed.post", rkey); err != nil {
		t.Fatalf("DeleteRecord: %v", err)
	}
	if got := atprotoAccountFor(t, ctx, d, userID).RepoSeq; got != 3 {
		t.Fatalf("repo_seq after delete = %d, want 3", got)
	}
	var n int
	if err := d.Writer.QueryRow(ctx, `SELECT COUNT(*) FROM atproto_records WHERE uri = $1`, uri).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("index row survived delete")
	}
	// Profile record still indexed.
	if err := d.Writer.QueryRow(ctx,
		`SELECT COUNT(*) FROM atproto_records WHERE account_id = $1 AND collection = 'app.bsky.actor.profile'`, acct.ID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("profile record missing after post delete (n=%d, err=%v)", n, err)
	}
}

// The CAR export must re-ingest into a valid repo: every record readable,
// commit signature verifiable against the account's K-256 key. This is
// the conformance property getRepo (A3) depends on.
func TestDBRepoCARRoundTrip(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)
	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	acct := atprotoAccountFor(t, ctx, d, userID)
	contentID := newAtprotoContent(t, ctx, d, userID, "blog_post", "published", "public")
	if _, err := svc.PublishContent(ctx, contentID); err != nil {
		t.Fatalf("PublishContent: %v", err)
	}

	carBytes, err := svc.RepoCAR(ctx, acct.ID)
	if err != nil {
		t.Fatalf("RepoCAR: %v", err)
	}
	if len(carBytes) == 0 {
		t.Fatal("empty CAR")
	}

	rep, err := atrepo.ReadRepoFromCar(ctx, bytes.NewReader(carBytes))
	if err != nil {
		t.Fatalf("ReadRepoFromCar: %v", err)
	}
	if rep.RepoDid() != acct.Did {
		t.Fatalf("car repo did = %q, want %q", rep.RepoDid(), acct.Did)
	}

	paths := map[string]bool{}
	if err := rep.ForEach(ctx, "", func(k string, _ cid.Cid) error {
		paths[k] = true
		return nil
	}); err != nil {
		t.Fatalf("ForEach: %v", err)
	}
	if !paths["app.bsky.actor.profile/self"] {
		t.Fatal("profile record absent from CAR repo")
	}
	var postPath string
	for k := range paths {
		if strings.HasPrefix(k, "app.bsky.feed.post/") {
			postPath = k
		}
	}
	if postPath == "" {
		t.Fatal("post record absent from CAR repo")
	}

	// Verify the head commit's K-256 signature against the account's key.
	sc := rep.SignedCommit()
	privHex, err := jcrypto.Decrypt(acct.SigningKeyEnc)
	if err != nil {
		t.Fatalf("decrypt key: %v", err)
	}
	priv, err := parsePrivateKeyHex(privHex)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := atcrypto.ParsePublicBytesK256(priv.PubKey().SerializeCompressed())
	if err != nil {
		t.Fatalf("parse pubkey: %v", err)
	}
	sb, err := sc.Unsigned().BytesForSigning()
	if err != nil {
		t.Fatalf("unsigned commit bytes: %v", err)
	}
	if err := pub.HashAndVerify(sb, sc.Sig); err != nil {
		t.Fatalf("commit signature does not verify: %v", err)
	}
	if sc.Version != 3 {
		t.Fatalf("commit version = %d, want 3", sc.Version)
	}
	_ = uname
}

// Ineligible content (draft, members-only) and disabled accounts must not
// write records — the eligibility rule is the projection's only guard.
func TestDBRepoPublishGuards(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)
	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	draft := newAtprotoContent(t, ctx, d, userID, "discussion", "draft", "public")
	if _, err := svc.PublishContent(ctx, draft); !errors.Is(err, ErrNotFound) {
		t.Errorf("draft publish: got %v, want ErrNotFound", err)
	}
	membersOnly := newAtprotoContent(t, ctx, d, userID, "discussion", "published", "members")
	if _, err := svc.PublishContent(ctx, membersOnly); !errors.Is(err, ErrNotFound) {
		t.Errorf("members-only publish: got %v, want ErrNotFound", err)
	}
	nonFederatable := newAtprotoContent(t, ctx, d, userID, "page", "published", "public")
	if _, err := svc.PublishContent(ctx, nonFederatable); !errors.Is(err, ErrNotFound) {
		t.Errorf("non-federatable type publish: got %v, want ErrNotFound", err)
	}
	if got := atprotoAccountFor(t, ctx, d, userID).RepoSeq; got != 1 {
		t.Errorf("guarded publishes still committed (repo_seq=%d)", got)
	}

	// Content whose author holds no active account resolves no repo —
	// the service derives the repo from the author, never the caller.
	otherID, _ := newAtprotoUser(t, ctx, d, "active")
	foreign := newAtprotoContent(t, ctx, d, otherID, "discussion", "published", "public")
	if _, err := svc.PublishContent(ctx, foreign); !errors.Is(err, ErrNotFound) {
		t.Errorf("author-without-account publish: got %v, want ErrNotFound", err)
	}

	if _, err := svc.DisableAccount(ctx, userID); err != nil {
		t.Fatalf("DisableAccount: %v", err)
	}
	pub := newAtprotoContent(t, ctx, d, userID, "discussion", "published", "public")
	if _, err := svc.PublishContent(ctx, pub); !errors.Is(err, ErrNotFound) {
		t.Errorf("disabled-account publish: got %v, want ErrNotFound", err)
	}
}

// Blobs round-trip under raw-codec sha256 CIDs — the ref format records
// embed via LexBlob.
func TestDBBlobStore(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)
	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	acct := atprotoAccountFor(t, ctx, d, userID)

	payload := []byte("fake-png-bytes")
	cidStr, err := svc.PutBlob(ctx, acct.ID, "image/png", payload)
	if err != nil {
		t.Fatalf("PutBlob: %v", err)
	}
	if _, err := cid.Parse(cidStr); err != nil {
		t.Fatalf("blob cid %q does not parse: %v", cidStr, err)
	}
	mime, data, err := svc.GetBlob(ctx, acct.ID, cidStr)
	if err != nil {
		t.Fatalf("GetBlob: %v", err)
	}
	if mime != "image/png" || !bytes.Equal(data, payload) {
		t.Fatalf("blob round-trip: mime=%q bytes=%q", mime, data)
	}
	// Re-put is idempotent — same bytes, same CID.
	again, err := svc.PutBlob(ctx, acct.ID, "image/png", payload)
	if err != nil || again != cidStr {
		t.Fatalf("idempotent put: %q vs %q err=%v", again, cidStr, err)
	}
	if _, _, err := svc.GetBlob(ctx, acct.ID, "bafkreiaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); err != ErrNotFound {
		t.Errorf("missing blob: got %v, want ErrNotFound", err)
	}
}

// The task entry point: PublishRecordJSON decodes the payload doc into
// its lexicon type and runs the same put+commit path.
func TestDBPublishRecordJSONTaskPath(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)
	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	acct := atprotoAccountFor(t, ctx, d, userID)

	bio := "relocation notes"
	rec, err := ProfileRecordJSON(nil, &bio, uname, acct.CreatedAt.Time)
	if err != nil {
		t.Fatalf("ProfileRecordJSON: %v", err)
	}
	uri, err := svc.PublishRecordJSON(ctx, acct.ID, CollectionProfile, ProfileRkey, rec, pgtype.UUID{})
	if err != nil {
		t.Fatalf("PublishRecordJSON: %v", err)
	}
	if uri != "at://"+acct.Did+"/app.bsky.actor.profile/self" {
		t.Fatalf("uri = %q", uri)
	}
	if got := atprotoAccountFor(t, ctx, d, userID).RepoSeq; got != 2 {
		t.Fatalf("repo_seq = %d, want 2", got)
	}
	// Unknown collection is rejected before any write.
	if _, err := svc.PublishRecordJSON(ctx, acct.ID, "com.example.bogus", "x", rec, pgtype.UUID{}); err == nil {
		t.Error("bogus collection accepted")
	}
}

// Republishing the same content must upsert the one record — the
// content-keyed rkey exists precisely so retries/edits never mint
// duplicate feed.post entries.
func TestDBRepublishUpsertsPost(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)
	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	acct := atprotoAccountFor(t, ctx, d, userID)
	contentID := newAtprotoContent(t, ctx, d, userID, "discussion", "published", "public")

	uri, err := svc.PublishContent(ctx, contentID)
	if err != nil {
		t.Fatalf("PublishContent: %v", err)
	}
	wantURI := ContentPostURI(acct.Did, contentID)
	if uri != wantURI {
		t.Fatalf("uri = %q, want content-keyed %q", uri, wantURI)
	}

	// Second publish of the same content: one new commit, no new row.
	uri2, err := svc.PublishContent(ctx, contentID)
	if err != nil {
		t.Fatalf("republish: %v", err)
	}
	if uri2 != uri {
		t.Fatalf("republish changed URI: %q vs %q", uri2, uri)
	}
	var n int
	if err := d.Writer.QueryRow(ctx,
		`SELECT COUNT(*) FROM atproto_records WHERE account_id = $1 AND collection = 'app.bsky.feed.post'`,
		acct.ID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("post records = %d, want 1 (republish must upsert)", n)
	}
	if got := atprotoAccountFor(t, ctx, d, userID).RepoSeq; got != 3 {
		t.Fatalf("repo_seq = %d, want 3 (init + publish + republish)", got)
	}
}

// The atproto:delete_record path: DeleteRecordURI parses the URI,
// checks the authority matches the repo, deletes, and commits —
// while bad URIs and foreign authorities are permanent failures.
func TestDBDeleteRecordURIPath(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)
	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	acct := atprotoAccountFor(t, ctx, d, userID)
	contentID := newAtprotoContent(t, ctx, d, userID, "blog_post", "published", "public")
	if _, err := svc.PublishContent(ctx, contentID); err != nil {
		t.Fatalf("PublishContent: %v", err)
	}

	uri := ContentPostURI(acct.Did, contentID)
	if err := svc.DeleteRecordURI(ctx, acct.ID, uri); err != nil {
		t.Fatalf("DeleteRecordURI: %v", err)
	}
	var n int
	if err := d.Writer.QueryRow(ctx, `SELECT COUNT(*) FROM atproto_records WHERE uri = $1`, uri).Scan(&n); err != nil || n != 0 {
		t.Fatalf("record index row survived delete (n=%d)", n)
	}
	if got := atprotoAccountFor(t, ctx, d, userID).RepoSeq; got != 3 {
		t.Fatalf("repo_seq = %d, want 3 (tombstone commit)", got)
	}

	// Retried delete of the same URI is a no-op (idempotent unpublish).
	if err := svc.DeleteRecordURI(ctx, acct.ID, uri); err != nil {
		t.Fatalf("idempotent re-delete: %v", err)
	}

	// Unparseable URI and foreign-authority URI are permanent failures.
	var pe interface{ Permanent() bool }
	if err := svc.DeleteRecordURI(ctx, acct.ID, "not-an-at-uri"); !errors.As(err, &pe) || !pe.Permanent() {
		t.Errorf("bad uri: got %v, want permanent", err)
	}
	foreign := strings.Replace(uri, acct.Did, "did:web:example.com", 1)
	if err := svc.DeleteRecordURI(ctx, acct.ID, foreign); !errors.As(err, &pe) || !pe.Permanent() {
		t.Errorf("foreign-authority uri: got %v, want permanent", err)
	}
}

// Task-facing permanence: ineligible content, bogus collections, and
// inactive accounts must report Permanent() so the worker maps them to
// SkipRetry instead of burning retries into the dead-letter queue.
func TestDBPublishPermanentClassification(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)
	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	acct := atprotoAccountFor(t, ctx, d, userID)
	var pe interface{ Permanent() bool }

	draft := newAtprotoContent(t, ctx, d, userID, "discussion", "draft", "public")
	if _, err := svc.PublishContent(ctx, draft); !errors.As(err, &pe) || !pe.Permanent() {
		t.Errorf("ineligible content: got %v, want permanent", err)
	}

	if _, err := svc.PublishRecordJSON(ctx, acct.ID, "com.example.bogus", "x", json.RawMessage(`{}`), pgtype.UUID{}); !errors.As(err, &pe) || !pe.Permanent() {
		t.Errorf("bogus collection: got %v, want permanent", err)
	}

	if _, err := svc.DisableAccount(ctx, userID); err != nil {
		t.Fatalf("DisableAccount: %v", err)
	}
	pub := newAtprotoContent(t, ctx, d, userID, "discussion", "published", "public")
	if _, err := svc.PublishContent(ctx, pub); !errors.As(err, &pe) || !pe.Permanent() {
		t.Errorf("disabled account: got %v, want permanent", err)
	}
}

// A suspended member's account stays status='active' but must not
// project — every atproto resolution query joins u.status='active',
// and the content-keyed account lookup must gate the same way
// (§127: same eligibility as AP).
func TestDBSuspendedUserNotProjected(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)
	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	contentID := newAtprotoContent(t, ctx, d, userID, "discussion", "published", "public")

	if _, err := d.Writer.Exec(ctx, `UPDATE users SET status = 'suspended' WHERE id = $1`, userID); err != nil {
		t.Fatalf("suspend user: %v", err)
	}
	var pe interface{ Permanent() bool }
	if _, err := svc.PublishContent(ctx, contentID); !errors.As(err, &pe) || !pe.Permanent() {
		t.Fatalf("suspended user publish: got %v, want permanent", err)
	}
}

// indigo's TIDClock resets to zero on every OpenRepo — a wall-clock
// regression would mint rev <= head's and corrupt the chain ordering.
// The guard must refuse to persist and stay transient (retryable once
// the clock recovers), rolling back with no commit, no blocks, no seq.
func TestDBRepoRevRegressionRefused(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)
	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	acct := atprotoAccountFor(t, ctx, d, userID)
	contentID := newAtprotoContent(t, ctx, d, userID, "discussion", "published", "public")

	// Forge a head rev far in the future — 'j'+12×'z' is a valid TID
	// well beyond any wall-clock timestamp for decades.
	if _, err := d.Writer.Exec(ctx,
		`UPDATE atproto_repo_commits SET rev = 'jzzzzzzzzzzzz' WHERE account_id = $1`, acct.ID); err != nil {
		t.Fatalf("forge head rev: %v", err)
	}

	_, err := svc.PublishContent(ctx, contentID)
	if err == nil {
		t.Fatal("publish succeeded against a future head rev — chain ordering would break")
	}
	var pe interface{ Permanent() bool }
	if errors.As(err, &pe) && pe.Permanent() {
		t.Fatalf("rev regression classified permanent — must retry once the clock recovers: %v", err)
	}

	if got := atprotoAccountFor(t, ctx, d, userID).RepoSeq; got != 1 {
		t.Fatalf("repo_seq advanced to %d despite refused commit", got)
	}
	var commits int
	if err := d.Writer.QueryRow(ctx,
		`SELECT COUNT(*) FROM atproto_repo_commits WHERE account_id = $1`, acct.ID).Scan(&commits); err != nil || commits != 1 {
		t.Fatalf("commits = %d — refused write persisted a commit row", commits)
	}
	var recs int
	if err := d.Writer.QueryRow(ctx,
		`SELECT COUNT(*) FROM atproto_records WHERE account_id = $1 AND collection = 'app.bsky.feed.post'`, acct.ID).Scan(&recs); err != nil || recs != 0 {
		t.Fatalf("post index rows = %d — refused write persisted an index row", recs)
	}
}

// A record task carrying content_id must target the owning repo — the
// index row it writes becomes the delete-resolution link, so a
// mispaired (account, content) would let a content delete tombstone
// another member's record. Mismatch is permanent.
func TestDBPublishRecordContentPairing(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	ownerID, _ := newAtprotoUser(t, ctx, d, "active")
	otherID, _ := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)
	if _, err := svc.EnableAccount(ctx, ownerID); err != nil {
		t.Fatalf("EnableAccount owner: %v", err)
	}
	if _, err := svc.EnableAccount(ctx, otherID); err != nil {
		t.Fatalf("EnableAccount other: %v", err)
	}
	ownerAcct := atprotoAccountFor(t, ctx, d, ownerID)
	otherAcct := atprotoAccountFor(t, ctx, d, otherID)
	contentID := newAtprotoContent(t, ctx, d, ownerID, "discussion", "published", "public")

	raw, err := json.Marshal(map[string]any{
		"$type":     "app.bsky.feed.post",
		"text":      "x",
		"createdAt": "2025-01-01T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	var pe interface{ Permanent() bool }
	if _, err := svc.PublishRecordJSON(ctx, otherAcct.ID, CollectionPost, contentID.String(), raw, contentID); !errors.As(err, &pe) || !pe.Permanent() {
		t.Fatalf("mispaired record task: got %v, want permanent", err)
	}

	// The owning account proceeds — explicit rkey upsert indexes the
	// content link so delete-resolution can tombstone it later.
	uri, err := svc.PublishRecordJSON(ctx, ownerAcct.ID, CollectionPost, contentID.String(), raw, contentID)
	if err != nil {
		t.Fatalf("paired record task: %v", err)
	}
	want := "at://" + ownerAcct.Did + "/" + CollectionPost + "/" + contentID.String()
	if uri != want {
		t.Fatalf("uri = %q, want %q", uri, want)
	}
}
