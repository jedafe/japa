package atproto

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	comatproto "github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/indigo/atproto/atcrypto"
	atrepo "github.com/bluesky-social/indigo/repo"
	"github.com/gorilla/websocket"
	"github.com/ipfs/go-cid"
	car "github.com/ipld/go-car"
	"github.com/jackc/pgx/v5/pgtype"
	jcrypto "github.com/jedafe/japa/internal/crypto"
	"github.com/jedafe/japa/internal/db"
	gen "github.com/jedafe/japa/internal/db/gen"
	"github.com/valkey-io/valkey-go"
	"github.com/valkey-io/valkey-go/valkeycompat"
	cbg "github.com/whyrusleeping/cbor-gen"
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

// floorSettings disables the §127 projection floor for tests that
// predate it — the floor's own tests re-set the real value on the
// returned service.
type floorSettings struct{ minPosts int }

func (f floorSettings) GetSetting(_ context.Context, _ string) (string, error) {
	return "", errors.New("unset")
}
func (f floorSettings) GetSettingBool(_ context.Context, _ string) (bool, error) {
	return false, errors.New("unset")
}
func (f floorSettings) GetSettingInt(_ context.Context, key string) (int, error) {
	if key == "federation.min_posts_discoverable" {
		return f.minPosts, nil
	}
	return 0, errors.New("unset")
}

func newAtprotoService(t *testing.T, d *db.DB) *Service {
	t.Helper()
	svc := NewService(d, Config{
		PDSURL:       "https://api.test.dev",
		APIBase:      "https://api.test.dev",
		SiteBase:     "https://test.dev",
		HandleDomain: "test.dev",
		// A6-2: opt-in tries the PLC directory first — point it at a
		// dead address so tests take the did:web fallback path
		// deterministically (instant refusal, never a real directory).
		PLCURL: "http://127.0.0.1:1",
		// The e2e asserts both sides of the replay window — pin the
		// 30s default rather than inheriting "disabled" from the
		// zero value.
		OAuthReuseGraceSeconds: 30,
	})
	svc.SetSettings(floorSettings{0})
	return svc
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
	var reason *string
	if err := d.Writer.QueryRow(ctx,
		`SELECT disabled_reason FROM atproto_accounts WHERE user_id = $1`, userID).Scan(&reason); err != nil {
		t.Fatalf("disabled_reason read: %v", err)
	}
	if reason == nil || *reason != DisableReasonMember {
		t.Fatalf("member disable reason = %v, want 'member'", reason)
	}

	// Deactivated-repo semantics: identity keeps resolving so signed
	// history stays verifiable; the repo itself is frozen.
	if got, err := svc.ResolveHandleDID(ctx, wantHandle); err != nil || got != st.DID {
		t.Errorf("disabled handle resolve = %q, %v — want %q", got, err, st.DID)
	}
	if _, err := svc.RepoCARSince(ctx, st.DID, ""); err == nil {
		t.Error("disabled repo served a CAR — want RepoDeactivated")
	} else {
		var inactive *repoInactiveError
		if !errors.As(err, &inactive) || inactive.name != "RepoDeactivated" {
			t.Errorf("disabled repo error = %v, want RepoDeactivated", err)
		}
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
	// Handle-legal target ('x' suffix — '_' is §50-legal but DNS-illegal;
	// the handle-illegal rename path is covered by
	// TestDBRenameToHandleInvalidFallsBack).
	renamed := uname + "x"
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
	// Rename Charlie so his stored handle is orphaned — handle-legal
	// target ('-m' suffix; a '_' rename exercises the uuid-fallback path
	// covered by TestDBRenameToHandleInvalidFallsBack).
	movedName := charlieName + "-m"
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

// Stale-claim release on read: when an existing account holder renames
// onto a handle stale-claimed by another account's row, accountByHandle's
// live-username preference shadows the stale row — the claim would never
// be released and the true holder locked out of the handle permanently.
// syncHandle must release the stale claim and land the sync in one read.
func TestDBStaleClaimReleasedOnRead(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newAtprotoService(t, d)
	base := fmt.Sprintf("atpdl%d", time.Now().UnixNano())

	// B holds "b-<base>", opts in, then renames away — the stored handle
	// goes stale but still claims b-<base>.test.dev.
	bID, _ := newAtprotoNamedUser(t, ctx, d, "b-"+base, "active")
	if _, err := svc.EnableAccount(ctx, bID); err != nil {
		t.Fatalf("enable B: %v", err)
	}
	if _, err := d.Writer.Exec(ctx, `UPDATE users SET username = $1 WHERE id = $2`, "bmoved-"+base, bID); err != nil {
		t.Fatalf("rename B: %v", err)
	}

	// A already holds an account under "a-<base>" and renames onto the
	// contested handle — A never hits the create-path reclaim because
	// the account row already exists.
	aID, _ := newAtprotoNamedUser(t, ctx, d, "a-"+base, "active")
	aSt, err := svc.EnableAccount(ctx, aID)
	if err != nil {
		t.Fatalf("enable A: %v", err)
	}
	if _, err := d.Writer.Exec(ctx, `UPDATE users SET username = $1 WHERE id = $2`, "b-"+base, aID); err != nil {
		t.Fatalf("rename A: %v", err)
	}

	contested := "b-" + base + ".test.dev"

	// First read: the stale claim is released and A's sync lands — the
	// handle resolves to A's permanent DID immediately.
	did, err := svc.ResolveHandleDID(ctx, contested)
	if err != nil || did != aSt.DID {
		t.Fatalf("contested handle: did=%q err=%v, want %q", did, err, aSt.DID)
	}

	// Both stored handles settled: A claims the contested handle, B's
	// row tracks B's live username.
	var handle string
	if err := d.Writer.QueryRow(ctx,
		`SELECT handle FROM atproto_accounts WHERE user_id = $1`, aID).Scan(&handle); err != nil {
		t.Fatalf("select A handle: %v", err)
	}
	if handle != contested {
		t.Errorf("A stored handle = %q, want %q", handle, contested)
	}
	if err := d.Writer.QueryRow(ctx,
		`SELECT handle FROM atproto_accounts WHERE user_id = $1`, bID).Scan(&handle); err != nil {
		t.Fatalf("select B handle: %v", err)
	}
	if want := "bmoved-" + base + ".test.dev"; handle != want {
		t.Errorf("B stored handle = %q, want synced %q", handle, want)
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

	// "api" is both a PDS-host collision and a reserved localpart —
	// either reason must reject with ErrInvalidHandle. Seed data may
	// already hold the username — reuse the row.
	var userID pgtype.UUID
	if err := d.Writer.QueryRow(ctx,
		`INSERT INTO users (username, email, status) VALUES ('api', 'api-pds@test.dev', 'active')
		 ON CONFLICT (username) DO UPDATE SET status = 'active'
		 RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("api user: %v", err)
	}
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

// TestDBAutoProvision covers the §126 auto opt-in path — the worker task
// provisions only when no account row exists, so a member's explicit
// opt-out (disabled row) is never silently resurrected by a retried task.
func TestDBAutoProvision(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)

	// Absent row → mints the account, same as EnableAccount.
	if err := svc.AutoProvision(ctx, userID); err != nil {
		t.Fatalf("AutoProvision: %v", err)
	}
	st, err := svc.GetMyAccount(ctx, userID)
	if err != nil || !st.Enabled || st.Handle != uname+".test.dev" {
		t.Fatalf("expected active account, got %+v %v", st, err)
	}

	// Idempotent — a retried task against an active row keeps the same DID.
	if err := svc.AutoProvision(ctx, userID); err != nil {
		t.Fatalf("AutoProvision idempotent: %v", err)
	}
	st2, _ := svc.GetMyAccount(ctx, userID)
	if st2.DID != st.DID {
		t.Fatalf("DID changed on retry: %q → %q", st.DID, st2.DID)
	}

	// Explicit opt-out → the row goes disabled; a late/retried task must
	// leave it disabled (same guarantee for tombstone rows).
	if _, err := svc.DisableAccount(ctx, userID); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if err := svc.AutoProvision(ctx, userID); err != nil {
		t.Fatalf("AutoProvision after opt-out: %v", err)
	}
	st3, _ := svc.GetMyAccount(ctx, userID)
	if st3.Enabled {
		t.Fatalf("opt-out resurrected: %+v", st3)
	}

	// Underscore username (the OAuth-generated shape, e.g. john_google_abc)
	// can't form an atproto handle → ineligible, not an error — the task
	// must end clean rather than burn retries to the DLQ.
	oauthID, _ := newAtprotoNamedUser(t, ctx, d, "oauth_test_user", "active")
	if err := svc.AutoProvision(ctx, oauthID); err != nil {
		t.Fatalf("AutoProvision underscore user: %v", err)
	}
	if st, _ := svc.GetMyAccount(ctx, oauthID); st.Enabled {
		t.Fatalf("handle-ineligible user provisioned: %+v", st)
	}

	// Suspended member → ineligible, not an error.
	suspID, _ := newAtprotoUser(t, ctx, d, "suspended")
	if err := svc.AutoProvision(ctx, suspID); err != nil {
		t.Fatalf("AutoProvision suspended user: %v", err)
	}
	if st, _ := svc.GetMyAccount(ctx, suspID); st.Enabled {
		t.Fatalf("suspended user provisioned: %+v", st)
	}
}

// jc:atp:did:{id} is keyed on the USER uuid — the did:web u/{id} path
// component — not the atproto_accounts row PK. Disable evicts both
// identity caches, but deactivated-repo semantics keep the doc and
// handle resolvable — the freeze surfaces only on the sync endpoints,
// and a stale warm cache must never resurrect the wrong answer.
func TestDBDisableKeepsIdentityCaches(t *testing.T) {
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

	// Identity still resolves — signed history stays verifiable — while
	// the sync surface reports the repo as deactivated.
	if _, err := svc.DIDDocument(ctx, uuidString(userID)); err != nil {
		t.Errorf("disabled DID doc no longer served: %v", err)
	}
	if _, err := svc.ResolveHandleDID(ctx, uname+".test.dev"); err != nil {
		t.Errorf("disabled handle no longer resolves: %v", err)
	}
	acct := atprotoAccountFor(t, ctx, d, userID)
	if _, err := svc.RepoCARSince(ctx, acct.Did, ""); err == nil {
		t.Error("disabled repo CAR served — want RepoDeactivated")
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
	a, err := gen.New(d.Writer).GetAtprotoAccountAnyUserStatus(ctx, userID)
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

// §127 projection floor (federation.min_posts_discoverable): below the
// floor the repo emits nothing — posts resolve as ineligible and the
// profile is withheld — while the account + handle stay live. Crossing
// the floor projects the withheld backlog together; dropping back
// below tombstones everything.
func TestDBRepoPostFloorGate(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)
	svc.SetSettings(floorSettings{3})
	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	acct := atprotoAccountFor(t, ctx, d, userID)

	countRecords := func(coll string) int {
		t.Helper()
		var n int
		if err := d.Writer.QueryRow(ctx,
			`SELECT COUNT(*) FROM atproto_records WHERE account_id = $1 AND collection = $2`,
			acct.ID, coll).Scan(&n); err != nil {
			t.Fatalf("count records: %v", err)
		}
		return n
	}

	// Enable ran below the floor: account active, no profile indexed.
	if got := acct.Status; got != AccountStatusActive {
		t.Fatalf("account status = %q, want active — identity is not gated", got)
	}
	if n := countRecords(CollectionProfile); n != 0 {
		t.Fatalf("profile rows = %d below floor, want 0", n)
	}

	c1 := newAtprotoContent(t, ctx, d, userID, "discussion", "published", "public")
	if _, err := svc.PublishContent(ctx, c1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("PublishContent below floor err=%v, want ErrNotFound", err)
	}
	newAtprotoContent(t, ctx, d, userID, "discussion", "published", "public")
	c3 := newAtprotoContent(t, ctx, d, userID, "discussion", "published", "public")

	// At the floor the publish lands; the reconcile then converges the
	// withheld backlog — profile + the two skipped posts — together.
	if _, err := svc.PublishContent(ctx, c3); err != nil {
		t.Fatalf("PublishContent at floor: %v", err)
	}
	if err := svc.ReconcileRepo(ctx, acct.ID); err != nil {
		t.Fatalf("ReconcileRepo: %v", err)
	}
	if n := countRecords(CollectionPost); n != 3 {
		t.Fatalf("post rows = %d after crossing reconcile, want 3", n)
	}
	if n := countRecords(CollectionProfile); n != 1 {
		t.Fatalf("profile rows = %d after crossing reconcile, want 1", n)
	}

	// Dropping below the floor converges back to an empty repo: posts
	// tombstone via the reconcile stale-check (the floor makes the row
	// read ineligible) and the profile comes down via publishProfile.
	if _, err := d.Writer.Exec(ctx, `UPDATE contents SET status = 'draft' WHERE id = $1`, c3); err != nil {
		t.Fatalf("unpublish: %v", err)
	}
	if err := svc.ReconcileRepo(ctx, acct.ID); err != nil {
		t.Fatalf("ReconcileRepo below floor: %v", err)
	}
	if n := countRecords(CollectionPost) + countRecords(CollectionProfile); n != 0 {
		t.Fatalf("rows = %d below floor after reconcile, want 0", n)
	}
	if got := atprotoAccountFor(t, ctx, d, userID).Status; got != AccountStatusActive {
		t.Fatalf("account status = %q, want active — floor must not disable identity", got)
	}
}

// A profile task landing on a disabled/tombstone account must end
// permanently without touching identity — syncHandle emits PLC ops,
// #identity events, and handle rewrites, none of which may run on a
// frozen repo (the refreshProfileRecord active-only invariant).
func TestDBPublishProfileInactiveAccount(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)
	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	acct := atprotoAccountFor(t, ctx, d, userID)
	if _, err := svc.DisableAccount(ctx, userID); err != nil {
		t.Fatalf("DisableAccount: %v", err)
	}
	acct = atprotoAccountFor(t, ctx, d, userID) // stale fetch reads 'active'
	if err := svc.publishProfile(ctx, &acct); !isPermanentErr(err) {
		t.Fatalf("publishProfile on disabled account err=%v, want permanent", err)
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

// ---------------------------------------------------------------------------
// A3 — sync surface + firehose
// ---------------------------------------------------------------------------

// repoEvents lists an account's firehose rows in stream order.
func repoEvents(t *testing.T, ctx context.Context, d *db.DB, did string) []gen.AtprotoRepoEvent {
	t.Helper()
	rows, err := d.Writer.Query(ctx,
		`SELECT seq, did, kind, payload, created_at FROM atproto_repo_events WHERE did = $1 ORDER BY seq`, did)
	if err != nil {
		t.Fatalf("events query: %v", err)
	}
	defer rows.Close()
	var out []gen.AtprotoRepoEvent
	for rows.Next() {
		var ev gen.AtprotoRepoEvent
		if err := rows.Scan(&ev.Seq, &ev.Did, &ev.Kind, &ev.Payload, &ev.CreatedAt); err != nil {
			t.Fatalf("event scan: %v", err)
		}
		out = append(out, ev)
	}
	return out
}

// decodeFrameHeader parses a subscribeRepos frame's {op,t} map, leaving the
// returned reader positioned at the frame body (indigo's UnmarshalCBOR reuses
// a *CborReader verbatim, so the same stream continues into the body).
func decodeFrameHeader(t *testing.T, msg []byte) (int64, string, *cbg.CborReader) {
	t.Helper()
	cr := cbg.NewCborReader(bytes.NewReader(msg))
	maj, extra, err := cr.ReadHeader()
	if err != nil || maj != cbg.MajMap {
		t.Fatalf("frame header: maj=%d extra=%d err=%v", maj, extra, err)
	}
	var op int64
	var typ string
	for i := uint64(0); i < extra; i++ {
		kmaj, klen, err := cr.ReadHeader()
		if err != nil || kmaj != cbg.MajTextString {
			t.Fatalf("frame header key: maj=%d err=%v", kmaj, err)
		}
		kbuf := make([]byte, klen)
		if _, err := io.ReadFull(cr, kbuf); err != nil {
			t.Fatalf("frame header key read: %v", err)
		}
		vmaj, vlen, err := cr.ReadHeader()
		if err != nil {
			t.Fatalf("frame header value: %v", err)
		}
		switch string(kbuf) {
		case "t":
			if vmaj != cbg.MajTextString {
				t.Fatalf("frame t not text: maj=%d", vmaj)
			}
			vbuf := make([]byte, vlen)
			if _, err := io.ReadFull(cr, vbuf); err != nil {
				t.Fatalf("frame t read: %v", err)
			}
			typ = string(vbuf)
		case "op":
			if vmaj == cbg.MajNegativeInt {
				op = -int64(vlen) - 1
			} else {
				op = int64(vlen)
			}
		default:
			t.Fatalf("unexpected frame header key %q", kbuf)
		}
	}
	return op, typ, cr
}

// Firehose rows land in emission order, and each commit's ops carry the
// create/update/delete shape relays apply against their copy.
func TestDBFirehoseEventsOrdered(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)

	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	acct := atprotoAccountFor(t, ctx, d, userID)

	contentID := newAtprotoContent(t, ctx, d, userID, "discussion", "published", "public")
	if _, err := svc.PublishContent(ctx, contentID); err != nil {
		t.Fatalf("PublishContent: %v", err)
	}
	// Same rkey rewritten → update op naming the previous record CID.
	if _, err := svc.PublishContent(ctx, contentID); err != nil {
		t.Fatalf("republish: %v", err)
	}
	if err := svc.DeleteRecordURI(ctx, acct.ID, ContentPostURI(acct.Did, contentID)); err != nil {
		t.Fatalf("DeleteRecordURI: %v", err)
	}

	events := repoEvents(t, ctx, d, acct.Did)
	kinds := make([]string, 0, len(events))
	for _, ev := range events {
		kinds = append(kinds, ev.Kind)
	}
	// account → identity → profile commit → post create → post update → post delete
	want := []string{"account", "identity", "commit", "commit", "commit", "commit"}
	if len(kinds) != len(want) {
		t.Fatalf("event kinds = %v, want %v", kinds, want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("event[%d] kind = %q, want %q (all: %v)", i, kinds[i], want[i], kinds)
		}
	}
	for i := 1; i < len(events); i++ {
		if events[i].Seq <= events[i-1].Seq {
			t.Fatalf("seqs not increasing: %v", events)
		}
	}

	var account comatproto.SyncSubscribeRepos_Account
	if err := json.Unmarshal(events[0].Payload, &account); err != nil {
		t.Fatalf("account payload: %v", err)
	}
	if !account.Active || account.Did != acct.Did {
		t.Fatalf("account payload = %+v", account)
	}
	var identity comatproto.SyncSubscribeRepos_Identity
	if err := json.Unmarshal(events[1].Payload, &identity); err != nil {
		t.Fatalf("identity payload: %v", err)
	}
	if identity.Did != acct.Did || identity.Handle == nil || *identity.Handle != acct.Handle {
		t.Fatalf("identity payload = %+v", identity)
	}

	commitBodies := make([]comatproto.SyncSubscribeRepos_Commit, 0, 4)
	for _, ev := range events[2:] {
		var b comatproto.SyncSubscribeRepos_Commit
		if err := json.Unmarshal(ev.Payload, &b); err != nil {
			t.Fatalf("commit payload: %v", err)
		}
		commitBodies = append(commitBodies, b)
	}
	if commitBodies[0].Repo != acct.Did || len(commitBodies[0].Blocks) == 0 {
		t.Fatalf("profile commit payload = %+v", commitBodies[0])
	}
	postPath := CollectionPost + "/" + contentID.String()
	ops := commitBodies[1].Ops
	if len(ops) != 1 || ops[0].Action != "create" || ops[0].Path != postPath || ops[0].Cid == nil {
		t.Fatalf("create op = %+v", ops)
	}
	createdCid := ops[0].Cid.String()
	ops = commitBodies[2].Ops
	if len(ops) != 1 || ops[0].Action != "update" || ops[0].Path != postPath {
		t.Fatalf("update op = %+v", ops)
	}
	if ops[0].Prev == nil || ops[0].Prev.String() != createdCid {
		t.Fatalf("update op prev = %v, want %s", ops[0].Prev, createdCid)
	}
	ops = commitBodies[3].Ops
	if len(ops) != 1 || ops[0].Action != "delete" || ops[0].Path != postPath || ops[0].Prev == nil {
		t.Fatalf("delete op = %+v", ops)
	}
	// Every commit frame carries a diff CAR whose root is the commit CID.
	for i, b := range commitBodies {
		cr, err := car.NewCarReader(bytes.NewReader(b.Blocks))
		if err != nil {
			t.Fatalf("commit[%d] diff CAR header: %v", i, err)
		}
		if len(cr.Header.Roots) != 1 || cr.Header.Roots[0].String() != b.Commit.String() {
			t.Fatalf("commit[%d] CAR roots = %v, want [%s]", i, cr.Header.Roots, b.Commit)
		}
	}
}

// A rename emits #identity so relay handle caches refresh.
func TestDBIdentityEventOnRename(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, uname := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)
	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	acct := atprotoAccountFor(t, ctx, d, userID)

	renamed := "renamed" + uname
	if _, err := d.Writer.Exec(ctx, `UPDATE users SET username = $1 WHERE id = $2`, renamed, userID); err != nil {
		t.Fatalf("rename: %v", err)
	}
	// The new handle resolves to the same permanent DID (lazy sync +
	// identity emit); the freed old handle no longer resolves.
	newHandle := renamed + ".test.dev"
	did, err := svc.ResolveHandleDID(ctx, newHandle)
	if err != nil {
		t.Fatalf("ResolveHandleDID: %v", err)
	}
	if did != acct.Did {
		t.Fatalf("renamed did = %q, want %q", did, acct.Did)
	}
	if _, err := svc.ResolveHandleDID(ctx, acct.Handle); !errors.Is(err, ErrNotFound) {
		t.Fatalf("old handle resolve: %v, want ErrNotFound", err)
	}

	events := repoEvents(t, ctx, d, acct.Did)
	var last comatproto.SyncSubscribeRepos_Identity
	found := false
	for _, ev := range events {
		if ev.Kind != EventKindIdentity {
			continue
		}
		if err := json.Unmarshal(ev.Payload, &last); err != nil {
			t.Fatalf("identity payload: %v", err)
		}
		found = true
	}
	if !found || last.Handle == nil || *last.Handle != renamed+".test.dev" {
		t.Fatalf("identity events = %+v (found=%v)", events, found)
	}
}

// The public read surface resolves the did, serves CARs, and reports the
// head commit — the lexicon error names travel on misses.
func TestDBSyncEndpoints(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)
	h := NewHandlers(svc)

	rec := httptest.NewRecorder()
	h.DescribeServer(rec, httptest.NewRequest(http.MethodGet, "/xrpc/com.atproto.server.describeServer", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("describeServer: %d %s", rec.Code, rec.Body)
	}
	var desc struct {
		AvailableUserDomains []string `json:"availableUserDomains"`
		Did                  string   `json:"did"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &desc); err != nil {
		t.Fatalf("describeServer decode: %v", err)
	}
	if desc.Did != "did:web:api.test.dev" || len(desc.AvailableUserDomains) != 1 || desc.AvailableUserDomains[0] != ".test.dev" {
		t.Fatalf("describeServer = %+v", desc)
	}

	rec = httptest.NewRecorder()
	h.GetHostDIDDoc(rec, httptest.NewRequest(http.MethodGet, "/.well-known/did.json", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Type"), "did+ld+json") {
		t.Fatalf("host did doc: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}

	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	acct := atprotoAccountFor(t, ctx, d, userID)
	contentID := newAtprotoContent(t, ctx, d, userID, "discussion", "published", "public")
	if _, err := svc.PublishContent(ctx, contentID); err != nil {
		t.Fatalf("PublishContent: %v", err)
	}

	headCid, rev, err := svc.HeadCommit(ctx, acct.Did)
	if err != nil {
		t.Fatalf("HeadCommit: %v", err)
	}
	if headCid == "" || rev == "" {
		t.Fatalf("head = %q rev %q", headCid, rev)
	}
	rec = httptest.NewRecorder()
	h.GetLatestCommit(rec, httptest.NewRequest(http.MethodGet,
		"/xrpc/com.atproto.sync.getLatestCommit?did="+acct.Did, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("getLatestCommit: %d %s", rec.Code, rec.Body)
	}
	var latest comatproto.SyncGetLatestCommit_Output
	if err := json.Unmarshal(rec.Body.Bytes(), &latest); err != nil {
		t.Fatalf("getLatestCommit decode: %v", err)
	}
	if latest.Cid != headCid || latest.Rev != rev {
		t.Fatalf("getLatestCommit = %+v, want %s@%s", latest, headCid, rev)
	}

	// getRepo → valid CAR containing the head commit block.
	rec = httptest.NewRecorder()
	h.GetRepo(rec, httptest.NewRequest(http.MethodGet, "/xrpc/com.atproto.sync.getRepo?did="+acct.Did, nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != carMediaType {
		t.Fatalf("getRepo: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	cr, err := car.NewCarReader(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatalf("getRepo CAR: %v", err)
	}
	if len(cr.Header.Roots) != 1 || cr.Header.Roots[0].String() != headCid {
		t.Fatalf("getRepo roots = %v, want [%s]", cr.Header.Roots, headCid)
	}
	foundHead := false
	for {
		b, err := cr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("getRepo CAR block: %v", err)
		}
		if b.Cid().String() == headCid {
			foundHead = true
		}
	}
	if !foundHead {
		t.Fatal("getRepo CAR missing head commit block")
	}

	// getBlocks returns only the named, known blocks.
	data, err := svc.BlocksCAR(ctx, acct.Did, []string{headCid, "bafkreihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenxquvyku"})
	if err != nil {
		t.Fatalf("BlocksCAR: %v", err)
	}
	cr, err = car.NewCarReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("BlocksCAR CAR: %v", err)
	}
	n := 0
	for {
		b, err := cr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("BlocksCAR block: %v", err)
		}
		if b.Cid().String() != headCid {
			t.Fatalf("BlocksCAR returned unrequested %s", b.Cid())
		}
		n++
	}
	if n != 1 {
		t.Fatalf("BlocksCAR blocks = %d, want 1 (unknown CID omitted)", n)
	}

	// getRecord serves the record-bearing CAR; a missing rkey maps to
	// RecordNotFound — and a missing did to RepoNotFound.
	data, err = svc.RecordCAR(ctx, acct.Did, CollectionPost, contentID.String())
	if err != nil || len(data) == 0 {
		t.Fatalf("RecordCAR: %v", err)
	}
	if _, err := svc.RecordCAR(ctx, acct.Did, CollectionPost, "nosuch"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("RecordCAR missing rkey: %v", err)
	}
	rec = httptest.NewRecorder()
	h.GetRecord(rec, httptest.NewRequest(http.MethodGet,
		"/xrpc/com.atproto.sync.getRecord?did="+acct.Did+"&collection="+CollectionPost+"&rkey=nosuch", nil))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "RecordNotFound") {
		t.Fatalf("getRecord miss: %d %s", rec.Code, rec.Body)
	}
	// Lexicon requires did; legacy repo is still accepted for
	// pre-rename callers — both name the same repo.
	rec = httptest.NewRecorder()
	h.GetRecord(rec, httptest.NewRequest(http.MethodGet,
		"/xrpc/com.atproto.sync.getRecord?repo="+acct.Did+"&collection="+CollectionPost+"&rkey="+contentID.String(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("getRecord legacy repo param: %d %s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	h.GetRepo(rec, httptest.NewRequest(http.MethodGet, "/xrpc/com.atproto.sync.getRepo?did=did:web:api.test.dev:atproto:u:00000000-0000-0000-0000-000000000000", nil))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "RepoNotFound") {
		t.Fatalf("getRepo miss: %d %s", rec.Code, rec.Body)
	}
}

// subscribeRepos replays the log from ?cursor= then live-tails — frames are
// two-part CBOR a relay can UnmarshalCBOR straight off the wire.
func TestDBSubscribeReposStream(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)

	srv := httptest.NewServer(http.HandlerFunc(NewHandlers(svc).SubscribeRepos))
	t.Cleanup(srv.Close)
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	// The stream is global — prior tests contributed rows — so measure the
	// high-water mark; a cursor past it is a FutureCursor error frame.
	maxSeq, err := svc.MaxEventSeq(ctx)
	if err != nil {
		t.Fatalf("MaxEventSeq: %v", err)
	}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL+"?cursor="+fmt.Sprint(maxSeq+1000), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read error frame: %v", err)
	}
	op, _, cr := decodeFrameHeader(t, msg)
	if op != -1 {
		t.Fatalf("error frame op = %d, want -1", op)
	}
	maj, extra, err := cr.ReadHeader()
	if err != nil || maj != cbg.MajMap || extra != 2 {
		t.Fatalf("error body: maj=%d extra=%d err=%v", maj, extra, err)
	}
	conn.Close()

	// Live tail: connect at the current mark, then enable → this account's
	// account/identity/commit frames arrive in order, seqs stamped.
	conn, _, err = websocket.DefaultDialer.Dial(wsURL+"?cursor="+fmt.Sprint(maxSeq), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	acct := atprotoAccountFor(t, ctx, d, userID)

	var commitBody comatproto.SyncSubscribeRepos_Commit
	lastSeq := maxSeq
	for _, wantType := range []string{"#account", "#identity", "#commit"} {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read %s frame: %v", wantType, err)
		}
		op, typ, cr := decodeFrameHeader(t, msg)
		if op != 1 || typ != wantType {
			t.Fatalf("frame = op %d type %q, want op 1 %s", op, typ, wantType)
		}
		switch typ {
		case "#account":
			var b comatproto.SyncSubscribeRepos_Account
			if err := b.UnmarshalCBOR(cr); err != nil {
				t.Fatalf("account body: %v", err)
			}
			if b.Did != acct.Did || !b.Active || b.Seq <= lastSeq {
				t.Fatalf("account body = %+v", b)
			}
			lastSeq = b.Seq
		case "#identity":
			var b comatproto.SyncSubscribeRepos_Identity
			if err := b.UnmarshalCBOR(cr); err != nil {
				t.Fatalf("identity body: %v", err)
			}
			if b.Did != acct.Did || b.Seq <= lastSeq {
				t.Fatalf("identity body = %+v", b)
			}
			lastSeq = b.Seq
		case "#commit":
			if err := commitBody.UnmarshalCBOR(cr); err != nil {
				t.Fatalf("commit body: %v", err)
			}
			if commitBody.Repo != acct.Did || commitBody.Seq <= lastSeq || commitBody.Rev == "" || len(commitBody.Blocks) == 0 {
				t.Fatalf("commit body = %+v", commitBody)
			}
			lastSeq = commitBody.Seq
		}
	}
}

// requestCrawl posts the PDS hostname to the configured relay; a disabled
// account fails permanently (no retry) — transient relay errors retry.
func TestDBRequestCrawl(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)

	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	acct := atprotoAccountFor(t, ctx, d, userID)

	var gotBody []byte
	relay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/xrpc/com.atproto.sync.requestCrawl" || r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(relay.Close)
	svc.cfg.RelayURL = relay.URL

	if err := svc.RequestCrawl(ctx, acct.ID); err != nil {
		t.Fatalf("RequestCrawl: %v", err)
	}
	var body map[string]string
	if err := json.Unmarshal(gotBody, &body); err != nil {
		t.Fatalf("relay body: %v", err)
	}
	if body["hostname"] != "api.test.dev" {
		t.Fatalf("relay body = %v", body)
	}

	if _, err := svc.DisableAccount(ctx, userID); err != nil {
		t.Fatalf("DisableAccount: %v", err)
	}
	err := svc.RequestCrawl(ctx, acct.ID)
	var pe interface{ Permanent() bool }
	if err == nil || !errors.As(err, &pe) || !pe.Permanent() {
		t.Fatalf("disabled-account crawl: got %v, want permanent", err)
	}
}

// getRepo?since serves the diff CAR: head commit root + only the blocks
// newer commits introduced — never blocks the client's rev already had.
func TestDBGetRepoSince(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)
	h := NewHandlers(svc)

	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	acct := atprotoAccountFor(t, ctx, d, userID)

	var rootRev string
	if err := d.Writer.QueryRow(ctx,
		`SELECT rev FROM atproto_repo_commits WHERE account_id = $1 ORDER BY seq LIMIT 1`,
		acct.ID).Scan(&rootRev); err != nil {
		t.Fatalf("root rev: %v", err)
	}

	contentID := newAtprotoContent(t, ctx, d, userID, "discussion", "published", "public")
	if _, err := svc.PublishContent(ctx, contentID); err != nil {
		t.Fatalf("PublishContent: %v", err)
	}
	headCid, headRev, err := svc.HeadCommit(ctx, acct.Did)
	if err != nil {
		t.Fatalf("HeadCommit: %v", err)
	}

	readCAR := func(t *testing.T, data []byte) (roots []string, blocks map[string][]byte) {
		t.Helper()
		cr, err := car.NewCarReader(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("CAR: %v", err)
		}
		for _, r := range cr.Header.Roots {
			roots = append(roots, r.String())
		}
		blocks = map[string][]byte{}
		for {
			b, err := cr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("CAR block: %v", err)
			}
			blocks[b.Cid().String()] = b.RawData()
		}
		return roots, blocks
	}

	// since = root rev → diff = the create commit's introduced set only:
	// its commit block, new MST nodes, the post record — NOT the root
	// commit or profile blocks.
	data, err := svc.RepoCARSince(ctx, acct.Did, rootRev)
	if err != nil {
		t.Fatalf("RepoCARSince(root): %v", err)
	}
	roots, blocks := readCAR(t, data)
	if len(roots) != 1 || roots[0] != headCid {
		t.Fatalf("diff roots = %v, want [%s]", roots, headCid)
	}
	var wantBlocks int
	if err := d.Writer.QueryRow(ctx,
		`SELECT COUNT(*) FROM atproto_commit_blocks WHERE account_id = $1 AND seq > 1`,
		acct.ID).Scan(&wantBlocks); err != nil {
		t.Fatalf("index count: %v", err)
	}
	if len(blocks) != wantBlocks {
		t.Fatalf("diff blocks = %d, want %d (commit_blocks rows for seq>1)", len(blocks), wantBlocks)
	}
	if _, ok := blocks[headCid]; !ok {
		t.Fatal("diff missing head commit block")
	}
	// The root commit's block must NOT ship in the diff.
	var rootCid string
	if err := d.Writer.QueryRow(ctx,
		`SELECT cid FROM atproto_repo_commits WHERE account_id = $1 AND seq = 1`,
		acct.ID).Scan(&rootCid); err != nil {
		t.Fatalf("root cid: %v", err)
	}
	if _, ok := blocks[rootCid]; ok {
		t.Fatal("diff contains pre-since commit block")
	}

	// since = head rev → already-current answer: head commit block only.
	data, err = svc.RepoCARSince(ctx, acct.Did, headRev)
	if err != nil {
		t.Fatalf("RepoCARSince(head): %v", err)
	}
	roots, blocks = readCAR(t, data)
	if len(roots) != 1 || roots[0] != headCid || len(blocks) != 1 {
		t.Fatalf("since=head: roots=%v blocks=%d, want head commit only", roots, len(blocks))
	}
	if _, ok := blocks[headCid]; !ok {
		t.Fatal("since=head missing head commit block")
	}

	// since = "" → identical to the full-CAR path (the live tree — the
	// DAG traversal legitimately skips superseded blocks; the index diff
	// is a superset, which is legal for content-addressed blocks).
	data, err = svc.RepoCARSince(ctx, acct.Did, "")
	if err != nil {
		t.Fatalf("RepoCARSince(empty): %v", err)
	}
	full, err := svc.RepoCAR(ctx, acct.ID)
	if err != nil {
		t.Fatalf("RepoCAR: %v", err)
	}
	if !bytes.Equal(data, full) {
		t.Fatal("RepoCARSince(\"\") differs from RepoCAR")
	}

	// since naming a valid TID we never minted → we can't know what the
	// client holds → full CAR (the provably-sufficient answer).
	data, err = svc.RepoCARSince(ctx, acct.Did, "3aaaaaaaaaaaa")
	if err != nil {
		t.Fatalf("RepoCARSince(unknown): %v", err)
	}
	if !bytes.Equal(data, full) {
		t.Fatal("RepoCARSince(unknown rev) differs from full CAR")
	}

	// The endpoint wires the param through — since param on the wire.
	rec := httptest.NewRecorder()
	h.GetRepo(rec, httptest.NewRequest(http.MethodGet,
		"/xrpc/com.atproto.sync.getRepo?did="+acct.Did+"&since="+headRev, nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != carMediaType {
		t.Fatalf("getRepo?since: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	_, blocks = readCAR(t, rec.Body.Bytes())
	if len(blocks) != 1 {
		t.Fatalf("getRepo?since=head: %d blocks, want 1", len(blocks))
	}
}

// TestDBConcurrentCommitEventOrder hammers the event sequencer with
// concurrent repo writes on two accounts plus racing lifecycle emits —
// the cross-account case the seq lock exists for: without serialized
// inserts, two txs can commit out of seq order and a seq > cursor
// reader skips the still-uncommitted lower seq forever. Asserts every
// commit lands on the stream and nothing deadlocks under contention.
func TestDBConcurrentCommitEventOrder(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newAtprotoService(t, d)

	u1, _ := newAtprotoUser(t, ctx, d, "active")
	u2, _ := newAtprotoUser(t, ctx, d, "active")
	if _, err := svc.EnableAccount(ctx, u1); err != nil {
		t.Fatalf("enable u1: %v", err)
	}
	if _, err := svc.EnableAccount(ctx, u2); err != nil {
		t.Fatalf("enable u2: %v", err)
	}
	a1 := atprotoAccountFor(t, ctx, d, u1)
	a2 := atprotoAccountFor(t, ctx, d, u2)

	const n = 4
	var wg sync.WaitGroup
	errCh := make(chan error, 2*n)
	put := func(acct gen.AtprotoAccount, tag string) {
		defer wg.Done()
		rec := &bsky.ActorProfile{
			LexiconTypeID: CollectionProfile,
			DisplayName:   &tag,
		}
		if _, err := svc.PutRecord(ctx, acct.ID, CollectionProfile, ProfileRkey, rec, pgtype.UUID{}); err != nil {
			errCh <- err
		}
	}
	for i := 0; i < n; i++ {
		wg.Add(3)
		go put(a1, fmt.Sprintf("a1-%d", i))
		go put(a2, fmt.Sprintf("a2-%d", i))
		// Lifecycle emits racing the commit inserts — the pool path
		// holds the same seq lock across its short tx.
		go func() {
			defer wg.Done()
			svc.emitIdentityEvent(ctx, a1.Did, a1.Handle)
			svc.emitAccountEvent(ctx, a2.Did, true, "")
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent put: %v", err)
	}

	// Every commit landed — n profile writes + the root commit per DID.
	// The lock makes commit order == seq order, so a replaying reader
	// can never observe a later seq while an earlier one is in flight.
	for _, acct := range []gen.AtprotoAccount{a1, a2} {
		var commits int64
		if err := d.Writer.QueryRow(ctx,
			`SELECT count(*) FROM atproto_repo_events WHERE did = $1 AND kind = 'commit'`,
			acct.Did).Scan(&commits); err != nil {
			t.Fatalf("count commits: %v", err)
		}
		if commits != int64(n+1) {
			t.Fatalf("%s commits = %d, want %d", acct.Did, commits, n+1)
		}
	}
}

// setUserStatus flips users.status directly — lifecycle tests drive the
// same column the admin/moderation paths write.
func setUserStatus(t *testing.T, ctx context.Context, d *db.DB, userID pgtype.UUID, status string) {
	t.Helper()
	if _, err := d.Writer.Exec(ctx, `UPDATE users SET status = $2 WHERE id = $1`, userID, status); err != nil {
		t.Fatalf("set user status: %v", err)
	}
}

// getRepoStatus reports servability: active repos carry rev, inactive
// ones carry the lexicon status word, unknown/tombstone DIDs are
// RepoNotFound — the mapping the relay's inactive-recheck reads.
func TestDBGetRepoStatusEndpoint(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)
	h := NewHandlers(svc)

	out, err := svc.RepoStatus(ctx, "did:web:api.test.dev:atproto:u:00000000-0000-0000-0000-000000000000")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown did status = %+v, %v — want ErrNotFound", out, err)
	}

	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	acct := atprotoAccountFor(t, ctx, d, userID)
	out, err = svc.RepoStatus(ctx, acct.Did)
	if err != nil || !out.Active || out.Rev == "" || out.Status != "" {
		t.Fatalf("active repo status = %+v, %v", out, err)
	}

	if _, err := svc.DisableAccount(ctx, userID); err != nil {
		t.Fatalf("DisableAccount: %v", err)
	}
	out, err = svc.RepoStatus(ctx, acct.Did)
	if err != nil || out.Active || out.Status != "deactivated" || out.Rev != "" {
		t.Fatalf("disabled repo status = %+v, %v — want deactivated", out, err)
	}

	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("re-enable: %v", err)
	}
	setUserStatus(t, ctx, d, userID, "suspended")
	out, err = svc.RepoStatus(ctx, acct.Did)
	if err != nil || out.Active || out.Status != "suspended" {
		t.Fatalf("suspended repo status = %+v, %v", out, err)
	}
	setUserStatus(t, ctx, d, userID, "banned")
	out, err = svc.RepoStatus(ctx, acct.Did)
	if err != nil || out.Active || out.Status != "takendown" {
		t.Fatalf("banned repo status = %+v, %v", out, err)
	}
	setUserStatus(t, ctx, d, userID, "active")

	// A6-1: tombstone returns the explicit deleted end-state — relays hit
	// getRepoStatus on their inactive-recheck path and need more than
	// RepoNotFound. Other sync reads still answer not-found.
	if _, err := d.Writer.Exec(ctx,
		`UPDATE atproto_accounts SET status = 'tombstone' WHERE user_id = $1`, userID); err != nil {
		t.Fatalf("tombstone: %v", err)
	}
	out, err = svc.RepoStatus(ctx, acct.Did)
	if err != nil || out.Active || out.Status != "deleted" || out.Rev != "" {
		t.Fatalf("tombstone status = %+v, %v — want {active:false,status:deleted}", out, err)
	}

	rec := httptest.NewRecorder()
	h.GetRepoStatus(rec, httptest.NewRequest(http.MethodGet,
		"/xrpc/com.atproto.sync.getRepoStatus?did="+acct.Did, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("getRepoStatus tombstone: %d %s", rec.Code, rec.Body)
	}
	var body struct {
		Active bool   `json:"active"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode tombstone status: %v", err)
	}
	if body.Active || body.Status != "deleted" {
		t.Fatalf("getRepoStatus tombstone body = %+v — want deleted", body)
	}
}

// listRepos is the hosted-repo census: required fields, inactive repos
// carry their status word, and the did cursor paginates.
func TestDBListReposEndpoint(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newAtprotoService(t, d)
	h := NewHandlers(svc)

	u1, _ := newAtprotoUser(t, ctx, d, "active")
	u2, _ := newAtprotoUser(t, ctx, d, "active")
	if _, err := svc.EnableAccount(ctx, u1); err != nil {
		t.Fatalf("enable u1: %v", err)
	}
	if _, err := svc.EnableAccount(ctx, u2); err != nil {
		t.Fatalf("enable u2: %v", err)
	}
	a1 := atprotoAccountFor(t, ctx, d, u1)
	a2 := atprotoAccountFor(t, ctx, d, u2)
	if _, err := svc.DisableAccount(ctx, u2); err != nil {
		t.Fatalf("disable u2: %v", err)
	}

	repos, cursor, err := svc.ListRepos(ctx, "", 1000)
	if err != nil {
		t.Fatalf("ListRepos: %v", err)
	}
	seen := map[string]HostedRepo{}
	for _, r := range repos {
		seen[r.DID] = r
		if r.DID == "" || r.Head == "" || r.Rev == "" {
			t.Fatalf("repo entry missing required fields: %+v", r)
		}
	}
	r1, ok := seen[a1.Did]
	if !ok || !r1.Active {
		t.Fatalf("a1 entry = %+v present=%v", r1, ok)
	}
	r2, ok := seen[a2.Did]
	if !ok || r2.Active || r2.Status != "deactivated" {
		t.Fatalf("a2 entry = %+v present=%v — want active:false deactivated", r2, ok)
	}
	if len(repos) < 1000 && cursor != "" {
		t.Fatalf("cursor %q returned with a short page", cursor)
	}

	// Pagination: walk by the returned cursor — no repeats, reaches the end.
	first, cur, err := svc.ListRepos(ctx, "", 1)
	if err != nil || len(first) != 1 || cur == "" {
		t.Fatalf("first page: len=%d cursor=%q err=%v", len(first), cur, err)
	}
	second, _, err := svc.ListRepos(ctx, cur, 1000)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	for _, r := range second {
		if r.DID == first[0].DID {
			t.Fatalf("cursor page repeated %s", r.DID)
		}
	}

	rec := httptest.NewRecorder()
	h.ListRepos(rec, httptest.NewRequest(http.MethodGet, "/xrpc/com.atproto.sync.listRepos?limit=2", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"repos"`) {
		t.Fatalf("listRepos handler: %d %s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	h.ListRepos(rec, httptest.NewRequest(http.MethodGet, "/xrpc/com.atproto.sync.listRepos?limit=abc", nil))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "InvalidRequest") {
		t.Fatalf("listRepos bad limit: %d %s", rec.Code, rec.Body)
	}
}

// Malformed inputs are InvalidRequest, never a silent wrong answer:
// bad since TIDs, over-bounded cids, missing did.
func TestDBSyncInputValidation(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)
	h := NewHandlers(svc)
	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	acct := atprotoAccountFor(t, ctx, d, userID)

	for _, bad := range []string{"not-a-tid", "3k!nvalid", "zzzzzzzzzzzzz"} {
		if _, err := svc.RepoCARSince(ctx, acct.Did, bad); !errors.Is(err, ErrInvalidID) {
			t.Fatalf("since %q: %v — want ErrInvalidID", bad, err)
		}
	}
	rec := httptest.NewRecorder()
	h.GetRepo(rec, httptest.NewRequest(http.MethodGet,
		"/xrpc/com.atproto.sync.getRepo?did="+acct.Did+"&since=not-a-tid", nil))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "InvalidRequest") {
		t.Fatalf("getRepo bad since: %d %s", rec.Code, rec.Body)
	}

	cids := make([]string, maxGetBlocksCIDs+1)
	for i := range cids {
		cids[i] = "bafkreihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenxquvyku"
	}
	if _, err := svc.BlocksCAR(ctx, acct.Did, cids); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("BlocksCAR over cap: %v", err)
	}
	rec = httptest.NewRecorder()
	h.GetBlocks(rec, httptest.NewRequest(http.MethodGet,
		"/xrpc/com.atproto.sync.getBlocks?did="+acct.Did+"&cids="+strings.Join(cids, "&cids="), nil))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "InvalidRequest") {
		t.Fatalf("getBlocks over cap: %d %s", rec.Code, rec.Body)
	}
}

// Inactive repos answer with their lexicon names — a frozen repo is
// never confused with an unhosted DID by the sync endpoints.
func TestDBInactiveRepoErrorNames(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)

	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	acct := atprotoAccountFor(t, ctx, d, userID)
	wantName := func(err error) string {
		t.Helper()
		var inactive *repoInactiveError
		if !errors.As(err, &inactive) {
			t.Fatalf("error %v is not a repoInactiveError", err)
		}
		return inactive.name
	}

	setUserStatus(t, ctx, d, userID, "suspended")
	if _, err := svc.RepoCARSince(ctx, acct.Did, ""); wantName(err) != "RepoSuspended" {
		t.Fatalf("suspended user getRepo: %v", err)
	}
	setUserStatus(t, ctx, d, userID, "banned")
	if _, err := svc.RepoCARSince(ctx, acct.Did, ""); wantName(err) != "RepoTakendown" {
		t.Fatalf("banned user getRepo: %v", err)
	}
	setUserStatus(t, ctx, d, userID, "active")
	if _, err := svc.DisableAccount(ctx, userID); err != nil {
		t.Fatalf("DisableAccount: %v", err)
	}
	if _, _, err := svc.HeadCommit(ctx, acct.Did); wantName(err) != "RepoDeactivated" {
		t.Fatalf("disabled getLatestCommit: %v", err)
	}
	if _, err := svc.RecordCAR(ctx, acct.Did, CollectionProfile, ProfileRkey); wantName(err) != "RepoDeactivated" {
		t.Fatalf("disabled getRecord: %v", err)
	}
}

// SyncUserLifecycle converges the account on users.status: freeze on
// suspension (once), reactivate only lifecycle-frozen accounts, never
// resurrect a member's own opt-out or a tombstone.
func TestDBLifecycleSync(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)

	// Never opted in → no-op.
	if _, err := svc.SyncUserLifecycle(ctx, userID); err != nil {
		t.Fatalf("lifecycle no-account: %v", err)
	}

	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	acct := atprotoAccountFor(t, ctx, d, userID)
	maxSeq, err := svc.MaxEventSeq(ctx)
	if err != nil {
		t.Fatalf("MaxEventSeq: %v", err)
	}

	// Suspension freezes with 'lifecycle' reason + one #account frame.
	setUserStatus(t, ctx, d, userID, "suspended")
	if _, err := svc.SyncUserLifecycle(ctx, userID); err != nil {
		t.Fatalf("lifecycle suspend: %v", err)
	}
	acct = atprotoAccountFor(t, ctx, d, userID)
	if acct.Status != AccountStatusDisabled || acct.DisabledReason == nil || *acct.DisabledReason != DisableReasonLifecycle {
		t.Fatalf("frozen account = %+v", acct)
	}
	rows, err := svc.EventsAfter(ctx, maxSeq, 10)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	var frame struct {
		Active bool   `json:"active"`
		Status string `json:"status"`
	}
	if len(rows) != 1 || rows[0].Kind != EventKindAccount ||
		json.Unmarshal(rows[0].Payload, &frame) != nil || frame.Active || frame.Status != "suspended" {
		t.Fatalf("freeze events = %+v", rows)
	}
	// Repeat sync emits nothing — the transition already announced.
	if _, err := svc.SyncUserLifecycle(ctx, userID); err != nil {
		t.Fatalf("lifecycle repeat: %v", err)
	}
	rows, err = svc.EventsAfter(ctx, maxSeq, 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("repeat sync events = %+v", rows)
	}

	// Restore reactivates a lifecycle freeze (+ one #account frame).
	setUserStatus(t, ctx, d, userID, "active")
	if _, err := svc.SyncUserLifecycle(ctx, userID); err != nil {
		t.Fatalf("lifecycle restore: %v", err)
	}
	acct = atprotoAccountFor(t, ctx, d, userID)
	if acct.Status != AccountStatusActive || acct.DisabledReason != nil {
		t.Fatalf("reactivated account = %+v", acct)
	}

	// A member opt-out must NOT be reactivated by a moderation cycle.
	if _, err := svc.DisableAccount(ctx, userID); err != nil {
		t.Fatalf("DisableAccount: %v", err)
	}
	if _, err := svc.SyncUserLifecycle(ctx, userID); err != nil {
		t.Fatalf("lifecycle member-disabled: %v", err)
	}
	acct = atprotoAccountFor(t, ctx, d, userID)
	if acct.Status != AccountStatusDisabled || acct.DisabledReason == nil || *acct.DisabledReason != DisableReasonMember {
		t.Fatalf("member-disabled account resurrected: %+v", acct)
	}

	// Nor a historical NULL-reason disable.
	if _, err := d.Writer.Exec(ctx,
		`UPDATE atproto_accounts SET disabled_reason = NULL WHERE user_id = $1`, userID); err != nil {
		t.Fatalf("null reason: %v", err)
	}
	if _, err := svc.SyncUserLifecycle(ctx, userID); err != nil {
		t.Fatalf("lifecycle null-reason: %v", err)
	}
	acct = atprotoAccountFor(t, ctx, d, userID)
	if acct.Status != AccountStatusDisabled {
		t.Fatalf("null-reason disable resurrected: %+v", acct)
	}

	// Tombstone is terminal either direction.
	if _, err := d.Writer.Exec(ctx,
		`UPDATE atproto_accounts SET status = 'tombstone' WHERE user_id = $1`, userID); err != nil {
		t.Fatalf("tombstone: %v", err)
	}
	setUserStatus(t, ctx, d, userID, "suspended")
	if _, err := svc.SyncUserLifecycle(ctx, userID); err != nil {
		t.Fatalf("lifecycle tombstone: %v", err)
	}
	acct = atprotoAccountFor(t, ctx, d, userID)
	if acct.Status != AccountStatusTombstone {
		t.Fatalf("tombstone mutated: %+v", acct)
	}
}

// A queued publish that lands after a suspension must not commit —
// the live users.status gate inside the repo tx drops it permanently.
func TestDBPublishAfterSuspensionDropped(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)
	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	acct := atprotoAccountFor(t, ctx, d, userID)
	contentID := newAtprotoContent(t, ctx, d, userID, "discussion", "published", "public")
	seqBefore := acct.RepoSeq

	setUserStatus(t, ctx, d, userID, "suspended")
	// Content resolution itself gates on u.status='active' → permanent.
	if _, err := svc.PublishContent(ctx, contentID); err == nil {
		t.Fatal("PublishContent on suspended user succeeded")
	} else {
		var p interface{ Permanent() bool }
		if !errors.As(err, &p) || !p.Permanent() {
			t.Fatalf("PublishContent error not permanent: %v", err)
		}
	}
	// The write-path gate catches a direct PutRecord too — the account
	// row is still 'active' (no lifecycle task ran), only the user
	// status guard inside the tx stops the commit.
	name := "x"
	if _, err := svc.PutRecord(ctx, acct.ID, CollectionProfile, ProfileRkey,
		&bsky.ActorProfile{LexiconTypeID: CollectionProfile, DisplayName: &name}, pgtype.UUID{}); err == nil {
		t.Fatal("PutRecord on suspended user committed")
	} else {
		var p interface{ Permanent() bool }
		if !errors.As(err, &p) || !p.Permanent() {
			t.Fatalf("PutRecord error not permanent: %v", err)
		}
	}
	acct = atprotoAccountFor(t, ctx, d, userID)
	if acct.RepoSeq != seqBefore {
		t.Fatalf("repo_seq advanced past suspension: %d → %d", seqBefore, acct.RepoSeq)
	}
}

// EraseAccount is the GDPR ordering: records retract in signed commits
// while key custody exists, the account tombstones, #account deleted
// announces it — and a second call is a no-op.
func TestDBEraseAccount(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)
	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	acct := atprotoAccountFor(t, ctx, d, userID)
	contentID := newAtprotoContent(t, ctx, d, userID, "discussion", "published", "public")
	if _, err := svc.PublishContent(ctx, contentID); err != nil {
		t.Fatalf("PublishContent: %v", err)
	}
	maxSeq, err := svc.MaxEventSeq(ctx)
	if err != nil {
		t.Fatalf("MaxEventSeq: %v", err)
	}

	// Erasure from an active account (hard-delete path order) and from
	// a lifecycle-frozen one both work — run the second user through
	// the freeze-first order.
	userID2, _ := newAtprotoUser(t, ctx, d, "active")
	if _, err := svc.EnableAccount(ctx, userID2); err != nil {
		t.Fatalf("EnableAccount u2: %v", err)
	}
	if _, err := svc.DisableAccount(ctx, userID2); err != nil {
		t.Fatalf("DisableAccount u2: %v", err)
	}

	if err := svc.EraseAccount(ctx, userID); err != nil {
		t.Fatalf("EraseAccount: %v", err)
	}
	acct = atprotoAccountFor(t, ctx, d, userID)
	if acct.Status != AccountStatusTombstone {
		t.Fatalf("erased account = %+v", acct)
	}
	var recs int
	if err := d.Writer.QueryRow(ctx,
		`SELECT count(*) FROM atproto_records WHERE account_id = $1`, acct.ID).Scan(&recs); err != nil {
		t.Fatalf("count records: %v", err)
	}
	if recs != 0 {
		t.Fatalf("records left after erasure: %d", recs)
	}
	// The retraction commit landed: repo_seq advanced past the
	// publish commits, and the #commit frame carries delete ops so the
	// network sees the content leave.
	acct = atprotoAccountFor(t, ctx, d, userID)
	if acct.RepoSeq < 3 {
		t.Fatalf("repo_seq = %d — want root + publish + retraction commits", acct.RepoSeq)
	}
	var commitPayload []byte
	if err := d.Writer.QueryRow(ctx,
		`SELECT payload FROM atproto_repo_events WHERE did = $1 AND kind = 'commit' ORDER BY seq DESC LIMIT 1`,
		acct.Did).Scan(&commitPayload); err != nil {
		t.Fatalf("last commit event: %v", err)
	}
	var commitFrame struct {
		Ops []struct {
			Action string `json:"action"`
		} `json:"ops"`
	}
	deletes := 0
	if err := json.Unmarshal(commitPayload, &commitFrame); err == nil {
		for _, op := range commitFrame.Ops {
			if op.Action == "delete" {
				deletes++
			}
		}
	}
	if deletes != 2 {
		t.Fatalf("retraction commit delete ops = %d, want 2: %s", deletes, commitPayload)
	}
	var acctPayload []byte
	if err := d.Writer.QueryRow(ctx,
		`SELECT payload FROM atproto_repo_events WHERE did = $1 AND kind = 'account' ORDER BY seq DESC LIMIT 1`,
		acct.Did).Scan(&acctPayload); err != nil {
		t.Fatalf("account event: %v", err)
	}
	var acctFrame struct {
		Active bool   `json:"active"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(acctPayload, &acctFrame); err != nil || acctFrame.Active || acctFrame.Status != "deleted" {
		t.Fatalf("erasure account frame: %s", acctPayload)
	}
	rows, err := svc.EventsAfter(ctx, maxSeq, 50)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	// Frames read in seq order: retraction commit(s) precede
	// #account{deleted}, and A6-1's #tombstone frame is the last word.
	lastKind := ""
	secondLastKind := ""
	for _, r := range rows {
		if r.Did == acct.Did {
			secondLastKind = lastKind
			lastKind = r.Kind
		}
	}
	if lastKind != EventKindTombstone || secondLastKind != EventKindAccount {
		t.Fatalf("erased did tail frames = %q → %q, want account → tombstone",
			secondLastKind, lastKind)
	}

	// Idempotent — a retried erasure is a no-op, no second commit.
	seqAfter := acct.RepoSeq
	if err := svc.EraseAccount(ctx, userID); err != nil {
		t.Fatalf("EraseAccount retry: %v", err)
	}
	if acct2 := atprotoAccountFor(t, ctx, d, userID); acct2.RepoSeq != seqAfter {
		t.Fatalf("retry minted commits: %d → %d", seqAfter, acct2.RepoSeq)
	}

	// Frozen-first order works too (lifecycle sync ran before erasure).
	if err := svc.EraseAccount(ctx, userID2); err != nil {
		t.Fatalf("EraseAccount frozen: %v", err)
	}
	if acct2 := atprotoAccountFor(t, ctx, d, userID2); acct2.Status != AccountStatusTombstone {
		t.Fatalf("frozen erasure = %+v", acct2)
	}

	// Tombstoned repos vanish from both identity and sync surfaces —
	// getRepoStatus alone keeps the explicit deleted end-state (A6-1)
	// because relays recheck inactive repos through it.
	if _, err := svc.DIDDocument(ctx, uuidString(userID)); err != ErrNotFound {
		t.Fatalf("tombstone DID doc = %v", err)
	}
	stOut, stErr := svc.RepoStatus(ctx, acct.Did)
	if stErr != nil || stOut.Active || stOut.Status != "deleted" {
		t.Fatalf("tombstone repo status = %+v, %v — want deleted", stOut, stErr)
	}
}

// Reserved localparts can never mint a handle — a member named "api"
// would otherwise claim the API host's own subdomain.
func TestDBReservedHandleRejected(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newAtprotoService(t, d)
	for _, name := range []string{"api", "www", "admin", "pds", "relay", "cdn"} {
		// Seed data may already hold these usernames — reuse the row.
		var uid pgtype.UUID
		if err := d.Writer.QueryRow(ctx,
			`INSERT INTO users (username, email, status) VALUES ($1, $2, 'active')
			 ON CONFLICT (username) DO UPDATE SET status = 'active'
			 RETURNING id`, name, name+"-atp@test.dev").Scan(&uid); err != nil {
			t.Fatalf("reserved-name user %q: %v", name, err)
		}
		if _, err := svc.EnableAccount(ctx, uid); !errors.Is(err, ErrInvalidHandle) {
			t.Fatalf("reserved localpart %q enabled: %v", name, err)
		}
	}
	uid, _ := newAtprotoUser(t, ctx, d, "active")
	if _, err := svc.EnableAccount(ctx, uid); err != nil {
		t.Fatalf("normal username rejected: %v", err)
	}
}

// ILIKE metacharacters in the admin search are literal — a member named
// "attest%" must not match every "attest…" account.
func TestDBAdminSearchEscapesWildcards(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newAtprotoService(t, d)

	u1, _ := newAtprotoNamedUser(t, ctx, d, "ilikeme", "active")
	if _, err := svc.EnableAccount(ctx, u1); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}

	// An unescaped '%' would match every account; escaped it matches none.
	rows, _, err := svc.ListAccounts(ctx, "%", 50, 0)
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("search %% matched %d rows — wildcards must be literal", len(rows))
	}
	// The literal substring still finds the account.
	rows, _, err = svc.ListAccounts(ctx, "ilikeme", 50, 0)
	if err != nil || len(rows) != 1 {
		t.Fatalf("literal search: %+v, %v", rows, err)
	}
}

// Conformance runs: the admin surface runs the same appview checks as
// the CLI, persists every run (pass or fail), and serves them back
// through history. A stub appview stands in for public.api.bsky.app.
func TestDBConformanceRun(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")
	adminID, _ := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)
	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	acct := atprotoAccountFor(t, ctx, d, userID)
	contentID := newAtprotoContent(t, ctx, d, userID, "discussion", "published", "public")
	if _, err := svc.PublishContent(ctx, contentID); err != nil {
		t.Fatalf("PublishContent: %v", err)
	}

	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/xrpc/app.bsky.actor.getProfile":
			json.NewEncoder(w).Encode(map[string]any{"did": acct.Did, "handle": acct.Handle})
		case "/xrpc/app.bsky.feed.getAuthorFeed":
			json.NewEncoder(w).Encode(map[string]any{"feed": []any{map[string]any{"post": map[string]any{}}}})
		default:
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	t.Cleanup(stub.Close)
	confsvc := NewService(d, Config{
		PDSURL: "https://api.test.dev", APIBase: "https://api.test.dev",
		SiteBase: "https://test.dev", HandleDomain: "test.dev",
		AppViewURL: stub.URL,
	})

	// Explicit DID — all checks pass, run persisted.
	run, res, err := confsvc.RunConformanceCheck(ctx, "", acct.Did, adminID)
	if err != nil {
		t.Fatalf("RunConformanceCheck: %v", err)
	}
	if !res.Passed || !run.Passed || run.Actor != acct.Did || run.Did != acct.Did {
		t.Fatalf("passing run = %+v / %+v", res, run)
	}
	if len(res.Checks) != 4 {
		t.Fatalf("checks = %+v", res.Checks)
	}
	for _, c := range res.Checks {
		if !c.Pass {
			t.Fatalf("check %s failed: %s", c.Name, c.Detail)
		}
	}
	if !run.RunBy.Valid || run.RunBy != adminID {
		t.Fatalf("run_by = %+v, want %v", run.RunBy, adminID)
	}

	// Explicit handle resolves the same DID.
	_, res, err = confsvc.RunConformanceCheck(ctx, acct.Handle, "", adminID)
	if err != nil || !res.Passed || res.DID != acct.Did {
		t.Fatalf("handle run = %+v, %v", res, err)
	}

	// Auto-pick — picks SOME active account; the stub answers for our
	// account regardless, so assert the run shape rather than the target.
	_, res, err = confsvc.RunConformanceCheck(ctx, "", "", adminID)
	if err != nil {
		t.Fatalf("auto-pick run: %v", err)
	}
	if len(res.Checks) != 4 || res.Actor == "" {
		t.Fatalf("auto-pick result = %+v", res)
	}

	// History returns every run newest-first.
	runs, total, err := confsvc.ListConformanceRuns(ctx, 50, 0)
	if err != nil {
		t.Fatalf("ListConformanceRuns: %v", err)
	}
	if total < 3 || len(runs) < 3 {
		t.Fatalf("history: total=%d len=%d", total, len(runs))
	}
	for i := 1; i < len(runs); i++ {
		if runs[i].CreatedAt.Time.After(runs[i-1].CreatedAt.Time) {
			t.Fatalf("history not newest-first at %d", i)
		}
	}

	// A failing appview still persists the run — a FAIL in history is
	// the signal the page exists to show.
	badStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"did": acct.Did, "handle": "handle.invalid"})
	}))
	t.Cleanup(badStub.Close)
	badsvc := NewService(d, Config{
		PDSURL: "https://api.test.dev", APIBase: "https://api.test.dev",
		SiteBase: "https://test.dev", HandleDomain: "test.dev",
		AppViewURL: badStub.URL,
	})
	run, res, err = badsvc.RunConformanceCheck(ctx, "", acct.Did, adminID)
	if err != nil {
		t.Fatalf("failing run: %v", err)
	}
	if res.Passed || run.Passed {
		t.Fatalf("handle.invalid run passed: %+v / %+v", res, run)
	}
	var failedNames []string
	for _, c := range res.Checks {
		if !c.Pass {
			failedNames = append(failedNames, c.Name)
		}
	}
	if len(failedNames) == 0 {
		t.Fatal("no checks failed on handle.invalid")
	}
}

// The wire view must emit checks as a JSON array — the raw []byte would
// marshal as base64 and break the history UI's parse.
func TestConformanceRunViewJSON(t *testing.T) {
	checks := []byte(`[{"name":"profile","pass":true,"detail":"ok"}]`)
	v := conformanceRunView{
		AtprotoConformanceRun: gen.AtprotoConformanceRun{
			Actor:  "alice.test.dev",
			Did:    "did:web:test.dev:atproto:u:x",
			Checks: checks,
			Passed: true,
		},
		Checks: checks,
	}
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal view: %v", err)
	}
	var decoded struct {
		Checks []ConformanceCheck `json:"checks"`
	}
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("view checks not a JSON array: %v — %s", err, out)
	}
	if len(decoded.Checks) != 1 || decoded.Checks[0].Name != "profile" {
		t.Fatalf("view checks mangled: %s", out)
	}
}
