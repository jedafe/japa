package atproto

// A6-3/A6-4 DB tests — external-client sessions (app passwords, JWT
// pair, single-use refresh rotation, deleteSession) + the authed
// portability endpoints (getServiceAuth, checkAccountStatus,
// activate/deactivate) + tombstone credential revocation.
//
// Ref: japaconnect-engineering/sections/127-atproto-native.md (A6-3/A6-4)

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/atproto/atcrypto"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgtype"
	jdb "github.com/jedafe/japa/internal/db"
	gen "github.com/jedafe/japa/internal/db/gen"
)

// enableWithAppPassword opts a fresh member in and mints an app
// password — the bootstrap external clients need (members hold no
// main-PDS password).
func enableWithAppPassword(t *testing.T, ctx context.Context, d *jdb.DB, svc *Service) (pgtype.UUID, *SessionAccount, string) {
	t.Helper()
	userID, _ := newAtprotoUser(t, ctx, d, "active")
	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	acct, err := svc.store.accountByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("account: %v", err)
	}
	_, plain, err := svc.CreateAppPassword(ctx, acct.ID, "bsky-app")
	if err != nil {
		t.Fatalf("CreateAppPassword: %v", err)
	}
	sess := &SessionAccount{AccountID: acct.ID, UserID: userID, DID: acct.Did, Handle: acct.Handle, Status: acct.Status}
	return userID, sess, plain
}

func TestAppPasswordMintFormat(t *testing.T) {
	for i := 0; i < 50; i++ {
		plain, err := mintAppPassword()
		if err != nil {
			t.Fatal(err)
		}
		parts := strings.Split(plain, "-")
		if len(parts) != 4 {
			t.Fatalf("password %q — want 4 groups", plain)
		}
		for _, p := range parts {
			if len(p) != 4 {
				t.Fatalf("password %q — group %q not 4 chars", plain, p)
			}
			for _, c := range p {
				if !strings.ContainsRune(appPasswordAlphabet, c) {
					t.Fatalf("password %q — %q outside alphabet", plain, c)
				}
			}
		}
	}
}

func TestDBAppPasswordCRUD(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")
	svc := newAtprotoService(t, d)
	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	acct, _ := svc.store.accountByUserID(ctx, userID)

	view, plain, err := svc.CreateAppPassword(ctx, acct.ID, "cli")
	if err != nil {
		t.Fatalf("CreateAppPassword: %v", err)
	}
	if plain == "" || view.Name != "cli" {
		t.Fatalf("bad view: %+v", view)
	}
	// Duplicate live name → conflict.
	if _, _, err := svc.CreateAppPassword(ctx, acct.ID, "cli"); !errors.Is(err, ErrHandleTaken) {
		t.Fatalf("dupe name err = %v, want ErrHandleTaken", err)
	}
	list, err := svc.ListAppPasswords(ctx, acct.ID)
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %v / %d", err, len(list))
	}
	// Revoke by name (XRPC path) frees the name for reuse.
	if err := svc.RevokeAppPasswordByName(ctx, acct.ID, "cli"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, _, err := svc.CreateAppPassword(ctx, acct.ID, "cli"); err != nil {
		t.Fatalf("re-mint after revoke: %v", err)
	}
	list, _ = svc.ListAppPasswords(ctx, acct.ID)
	if len(list) != 1 {
		t.Fatalf("post-revoke list = %d", len(list))
	}
	// Revoke by id (member-API path).
	if err := svc.RevokeAppPassword(ctx, acct.ID, list[0].ID); err != nil {
		t.Fatalf("revoke by id: %v", err)
	}
	if err := svc.RevokeAppPassword(ctx, acct.ID, list[0].ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("double revoke err = %v, want ErrNotFound", err)
	}
}

func TestDBCreateSessionAndGetSession(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newAtprotoService(t, d)
	userID, sess, plain := enableWithAppPassword(t, ctx, d, svc)

	// Login by DID.
	acct, access, refresh, err := svc.CreateSession(ctx, sess.DID, plain, "test-agent", "127.0.0.1")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if acct.DID != sess.DID || acct.Handle != sess.Handle || access == "" || refresh == "" {
		t.Fatalf("session = %+v", acct)
	}
	// Login by handle also works (identifier flexibility).
	if _, _, _, err := svc.CreateSession(ctx, sess.Handle, plain, "", ""); err != nil {
		t.Fatalf("CreateSession by handle: %v", err)
	}
	// And bare username.
	uname, _ := newAtprotoNamedUser(t, ctx, d, "sessuser", "active")
	if _, err := svc.EnableAccount(ctx, uname); err != nil {
		t.Fatalf("EnableAccount 2: %v", err)
	}
	acct2, _ := svc.store.accountByUserID(ctx, uname)
	if _, p2, err := svc.CreateAppPassword(ctx, acct2.ID, "x"); err != nil {
		t.Fatal(err)
	} else if _, _, _, err := svc.CreateSession(ctx, "sessuser", p2, "", ""); err != nil {
		t.Fatalf("CreateSession by bare username: %v", err)
	}

	// Access JWT authenticates.
	authed, err := svc.AuthenticateAccess(ctx, "Bearer "+access)
	if err != nil {
		t.Fatalf("AuthenticateAccess: %v", err)
	}
	if authed.DID != sess.DID {
		t.Fatalf("authed did = %q", authed.DID)
	}
	// Claims sanity: scope+aud+sub are the contract.
	claims := jwt.MapClaims{}
	if _, err := jwt.ParseWithClaims(access, claims, func(_ *jwt.Token) (any, error) { return svc.jwtSecret(), nil }); err != nil {
		t.Fatalf("access parse: %v", err)
	}
	if claims["scope"] != sessionScopeAccess || claims["sub"] != sess.DID {
		t.Fatalf("access claims = %v", claims)
	}
	// Refresh token must NOT serve as access.
	if _, err := svc.AuthenticateAccess(ctx, "Bearer "+refresh); !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("refresh-as-access err = %v, want ErrAuthRequired", err)
	}
	_ = userID
}

func TestDBSessionWrongCredentials(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newAtprotoService(t, d)
	_, sess, plain := enableWithAppPassword(t, ctx, d, svc)

	if _, _, _, err := svc.CreateSession(ctx, sess.DID, "wrong-pass-word-1", "", ""); !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("bad password err = %v, want ErrAuthRequired", err)
	}
	if _, _, _, err := svc.CreateSession(ctx, "did:plc:nobodyhome00000000", plain, "", ""); !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("unknown identifier err = %v, want ErrAuthRequired", err)
	}
	if _, _, _, err := svc.CreateSession(ctx, "", plain, "", ""); !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("empty identifier err = %v, want ErrAuthRequired", err)
	}
	// Revoked password stops admitting.
	if err := svc.RevokeAppPasswordByName(ctx, sess.AccountID, "bsky-app"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := svc.CreateSession(ctx, sess.DID, plain, "", ""); !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("revoked password err = %v, want ErrAuthRequired", err)
	}
}

// Revoking an app password kills every session it minted — the
// "log that client out" lever.
func TestDBRevokeAppPasswordKillsSessions(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newAtprotoService(t, d)
	_, sess, plain := enableWithAppPassword(t, ctx, d, svc)

	_, _, refresh, err := svc.CreateSession(ctx, sess.DID, plain, "", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	view, err := svc.ListAppPasswords(ctx, sess.AccountID)
	if err != nil || len(view) == 0 {
		t.Fatalf("list: %v n=%d", err, len(view))
	}
	if err := svc.RevokeAppPassword(ctx, sess.AccountID, view[0].ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, _, _, err := svc.RefreshSession(ctx, "Bearer "+refresh); !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("refresh on revoked app-password session err = %v, want ErrAuthRequired", err)
	}
}

// A valid JWT whose sub doesn't name the session's account is
// malformed, never a refresh — and crucially it must NOT trip the
// theft-revoke arm (the real session's hash was never presented).
func TestDBRefreshSubMismatch(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newAtprotoService(t, d)
	_, sess, plain := enableWithAppPassword(t, ctx, d, svc)

	_, _, refresh1, err := svc.CreateSession(ctx, sess.DID, plain, "", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Pull session 1's id out of its own refresh JWT, then forge a
	// properly-signed refresh naming that session but a foreign did.
	claims := jwt.MapClaims{}
	if _, err := jwt.ParseWithClaims(refresh1, claims, func(t *jwt.Token) (any, error) {
		return svc.jwtSecret(), nil
	}); err != nil {
		t.Fatalf("parse refresh: %v", err)
	}
	jti, _ := claims["jti"].(string)
	forged, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"scope": sessionScopeRefresh,
		"sub":   "did:plc:forg3dsubmismatchxxxxxxxxxxxxxx",
		"aud":   svc.hostDID(),
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(refreshTokenTTL).Unix(),
		"jti":   jti,
		"nonce": "forged",
	}).SignedString(svc.jwtSecret())
	if err != nil {
		t.Fatalf("forge: %v", err)
	}
	if _, _, _, err := svc.RefreshSession(ctx, "Bearer "+forged); !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("forged sub err = %v, want ErrAuthRequired", err)
	}
	// The mismatch arm ran BEFORE the hash check — the real refresh
	// token is unburned and the session survives.
	if _, _, _, err := svc.RefreshSession(ctx, "Bearer "+refresh1); err != nil {
		t.Fatalf("legit refresh after forged attempt: %v", err)
	}
}

func TestDBRefreshRotation(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newAtprotoService(t, d)
	_, sess, plain := enableWithAppPassword(t, ctx, d, svc)

	_, access1, refresh1, err := svc.CreateSession(ctx, sess.DID, plain, "", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	acct, access2, refresh2, err := svc.RefreshSession(ctx, "Bearer "+refresh1)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if refresh2 == refresh1 || access2 == access1 {
		t.Fatal("rotation returned identical tokens")
	}
	if acct.DID != sess.DID {
		t.Fatalf("rotated session did = %q", acct.DID)
	}
	// The burned refresh must not re-rotate — theft semantics revoke
	// the session entirely, killing even the NEW refresh.
	if _, _, _, err := svc.RefreshSession(ctx, "Bearer "+refresh1); !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("stale refresh err = %v, want ErrAuthRequired", err)
	}
	if _, _, _, err := svc.RefreshSession(ctx, "Bearer "+refresh2); !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("post-theft new refresh err = %v, want ErrAuthRequired", err)
	}
}

func TestDBDeleteSession(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newAtprotoService(t, d)
	_, sess, plain := enableWithAppPassword(t, ctx, d, svc)

	_, _, refresh, err := svc.CreateSession(ctx, sess.DID, plain, "", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.DeleteSession(ctx, "Bearer "+refresh); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, _, _, err := svc.RefreshSession(ctx, "Bearer "+refresh); !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("refresh after delete err = %v, want ErrAuthRequired", err)
	}
	// Idempotent — second delete still succeeds.
	if err := svc.DeleteSession(ctx, "Bearer "+refresh); err != nil {
		t.Fatalf("second delete: %v", err)
	}
}

// A suspended member's credentials die at every gate — login refuses
// (no oracle that the password was valid), a live session can't
// refresh, and outstanding access tokens stop resolving.
func TestDBSuspendedMemberCredentialsDie(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newAtprotoService(t, d)
	userID, sess, plain := enableWithAppPassword(t, ctx, d, svc)

	_, access, refresh, err := svc.CreateSession(ctx, sess.DID, plain, "", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	setUserStatus(t, ctx, d, userID, "suspended")

	if _, _, _, err := svc.CreateSession(ctx, sess.DID, plain, "", ""); !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("suspended createSession err = %v, want ErrAuthRequired", err)
	}
	if _, _, _, err := svc.RefreshSession(ctx, "Bearer "+refresh); !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("suspended refresh err = %v, want ErrAuthRequired", err)
	}
	if _, err := svc.AuthenticateAccess(ctx, "Bearer "+access); !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("suspended access err = %v, want ErrAuthRequired", err)
	}
}

func TestDBDisabledAccountSessionLifecycle(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newAtprotoService(t, d)
	userID, sess, plain := enableWithAppPassword(t, ctx, d, svc)

	// Deactivate via XRPC semantics — the member-disable path.
	acct := &SessionAccount{AccountID: sess.AccountID, UserID: userID, Status: AccountStatusActive}
	if err := svc.DeactivateAccount(ctx, acct); err != nil {
		t.Fatalf("DeactivateAccount: %v", err)
	}
	row := atprotoAccountFor(t, ctx, d, userID)
	if row.Status != AccountStatusDisabled || row.DisabledReason == nil || *row.DisabledReason != DisableReasonMember {
		t.Fatalf("deactivated row = %+v", row)
	}
	// A disabled account STILL mints sessions — the client needs them
	// to activateAccount.
	acct2, access, _, err := svc.CreateSession(ctx, sess.DID, plain, "", "")
	if err != nil {
		t.Fatalf("disabled createSession: %v", err)
	}
	if active, st := acct2.activeStatus(); active || st != "deactivated" {
		t.Fatalf("disabled session active/status = %v/%q", active, st)
	}
	// Activate through the session.
	authed, err := svc.AuthenticateAccess(ctx, "Bearer "+access)
	if err != nil {
		t.Fatalf("auth: %v", err)
	}
	if err := svc.ActivateAccount(ctx, authed); err != nil {
		t.Fatalf("ActivateAccount: %v", err)
	}
	row = atprotoAccountFor(t, ctx, d, userID)
	if row.Status != AccountStatusActive {
		t.Fatalf("reactivated row = %+v", row)
	}
	// Admin-disabled refuses through the same gate.
	if _, err := svc.AdminDisableAccount(ctx, sess.AccountID); err != nil {
		t.Fatalf("AdminDisable: %v", err)
	}
	authed2, _, _, err := svc.CreateSession(ctx, sess.DID, plain, "", "")
	if err != nil {
		t.Fatalf("admin-disabled createSession: %v", err)
	}
	if err := svc.ActivateAccount(ctx, authed2); !errors.Is(err, ErrAdminDisabled) {
		t.Fatalf("activate admin-disabled err = %v, want ErrAdminDisabled", err)
	}
}

func TestDBServiceAuth(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newAtprotoService(t, d)
	_, sess, _ := enableWithAppPassword(t, ctx, d, svc)

	token, err := svc.ServiceAuth(ctx, sess, "did:web:destination.example", "com.atproto.repo.importRepo", 0)
	if err != nil {
		t.Fatalf("ServiceAuth: %v", err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("jwt parts = %d", len(parts))
	}
	// Verify ES256 signature against the account's repo pubkey.
	hdr, _ := base64.RawURLEncoding.DecodeString(parts[0])
	var hdrMap map[string]string
	_ = json.Unmarshal(hdr, &hdrMap)
	if hdrMap["alg"] != "ES256" {
		t.Fatalf("alg = %v", hdrMap)
	}
	claimsRaw, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims map[string]any
	_ = json.Unmarshal(claimsRaw, &claims)
	if claims["iss"] != sess.DID || claims["aud"] != "did:web:destination.example" || claims["lxm"] != "com.atproto.repo.importRepo" {
		t.Fatalf("claims = %v", claims)
	}
	if exp, _ := claims["exp"].(float64); exp <= float64(time.Now().Unix()) || exp > float64(time.Now().Add(61*time.Second).Unix()) {
		t.Fatalf("exp = %v out of 60s window", exp)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("sig decode: %v", err)
	}
	// Verify with indigo atcrypto against the stored pubkey multibase —
	// the same verificationMethod material the DID doc publishes.
	row, err := gen.New(d.Writer).GetAtprotoAccountByID(ctx, sess.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := atcrypto.ParsePublicMultibase(row.PublicKeyMultibase)
	if err != nil {
		t.Fatalf("pubkey parse: %v", err)
	}
	// HashAndVerify applies sha256 itself — pass the signing input raw,
	// matching HashAndSign on the minting side.
	if err := pub.HashAndVerify([]byte(parts[0]+"."+parts[1]), sig); err != nil {
		t.Fatalf("service auth verify: %v", err)
	}
}

func TestDBCheckAccountStatus(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newAtprotoService(t, d)
	userID, sess, _ := enableWithAppPassword(t, ctx, d, svc)

	stats, valid, err := svc.AccountStatusStats(ctx, sess)
	if err != nil {
		t.Fatalf("AccountStatusStats: %v", err)
	}
	if !valid {
		t.Fatal("validDid = false on a hosted did:web")
	}
	if stats.AccountStatus != AccountStatusActive || stats.RepoRev == "" || stats.RepoCommit == "" {
		t.Fatalf("stats = %+v", stats)
	}
	if stats.RepoBlocks <= 0 || stats.IndexedRecords <= 0 {
		t.Fatalf("empty repo stats after opt-in: %+v", stats)
	}
	_ = userID
}

func TestDBExportRepo(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newAtprotoService(t, d)
	userID, sess, _ := enableWithAppPassword(t, ctx, d, svc)

	car, did, err := svc.ExportRepo(ctx, userID)
	if err != nil {
		t.Fatalf("ExportRepo: %v", err)
	}
	if did != sess.DID || len(car) == 0 {
		t.Fatalf("export did=%q len=%d", did, len(car))
	}
	// A CAR stream is CBOR, not JSON — the export isn't an error body.
	if car[0] == '{' {
		t.Fatal("export returned JSON — expected CAR bytes")
	}
}

func TestDBTombstoneRevokesCredentials(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	svc := newAtprotoService(t, d)
	userID, sess, plain := enableWithAppPassword(t, ctx, d, svc)

	_, access, refresh, err := svc.CreateSession(ctx, sess.DID, plain, "", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	acct, err := svc.store.accountByUserID(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.TombstoneAccount(ctx, acct); err != nil {
		t.Fatalf("TombstoneAccount: %v", err)
	}
	// Refresh dies immediately.
	if _, _, _, err := svc.RefreshSession(ctx, "Bearer "+refresh); !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("refresh after tombstone err = %v, want ErrAuthRequired", err)
	}
	// Access token is still self-contained valid crypto-wise, but the
	// account no longer resolves — AuthenticateAccess refuses.
	if _, err := svc.AuthenticateAccess(ctx, "Bearer "+access); !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("access after tombstone err = %v, want ErrAuthRequired", err)
	}
	// New logins impossible.
	if _, _, _, err := svc.CreateSession(ctx, sess.DID, plain, "", ""); !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("createSession after tombstone err = %v, want ErrAuthRequired", err)
	}
}
