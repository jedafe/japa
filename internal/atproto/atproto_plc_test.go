package atproto

// A6-2 did:plc tests (§127). The known-answer vectors are real ops from
// plc.directory's public audit log for did:plc:ewvi7nxzyoun6zhxrhs64oiz
// (atprotocol.bsky.social → atproto.com) — reproducing their hash + sig
// verifies the dag-cbor encoder, the prev-link encoding, and the sig
// preimage against ground truth, not just our own round-trip.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	atcrypto "github.com/bluesky-social/indigo/atproto/atcrypto"
	"github.com/ipfs/go-cid"
	"github.com/jackc/pgx/v5"
	jcrypto "github.com/jedafe/japa/internal/crypto"
	"github.com/jedafe/japa/internal/db"
	gen "github.com/jedafe/japa/internal/db/gen"
)

// newAtprotoServiceWithPLC is newAtprotoService pointed at a directory
// stub — plc tests assert against the stub's recorded ops.
func newAtprotoServiceWithPLC(t *testing.T, d *db.DB, plcURL string) *Service {
	t.Helper()
	return NewService(d, Config{
		PDSURL:       "https://api.test.dev",
		APIBase:      "https://api.test.dev",
		SiteBase:     "https://test.dev",
		HandleDomain: "test.dev",
		PLCURL:       plcURL,
	})
}

// TestPLCGenesisDIDDerivation reproduces the genesis DID of
// did:plc:ewvi7nxzyoun6zhxrhs64oiz from the op the directory logged —
// proof our dag-cbor encoder emits byte-identical canonical form (map
// key order, null encoding, string form, nesting).
func TestPLCGenesisDIDDerivation(t *testing.T) {
	op := &plcOperation{
		rotationDIDKeys: []string{
			"did:key:zQ3shhCGUqDKjStzuDxPkTxN6ujddP4RkEKJJouJGRRkaLGbg",
			"did:key:zQ3shpKnbdPx3g3CmPf5cRVTPe1HtSwVn5ish3wSnDPQCbLJK",
		},
		signingDIDKey: "did:key:zQ3shXjHeiBuRCKmM36cuYnm7YEMzhGnCmCyW92sRJ9pribSF",
		alsoKnownAs:   []string{"at://atprotocol.bsky.social"},
		pdsEndpoint:   "https://bsky.social",
		sig:           "lza4at_jCtGo_TYgL5PC1ZNP7lhF4DV8H50LWHhvdHcB143x1wEwqZ43xvV36Pws6OOnJLJrkibEUFDFqkhIhg",
	}
	did, err := op.did()
	if err != nil {
		t.Fatalf("did derivation: %v", err)
	}
	if did != "did:plc:ewvi7nxzyoun6zhxrhs64oiz" {
		t.Fatalf("derived did = %q — dag-cbor encoding diverges from the reference op", did)
	}
}

// TestPLCUpdateOpSigVerification verifies the live second op's
// signature over OUR unsigned encoding — the update carries a prev
// CID link, so this proves the link (tag-42) serialization and the
// sig-minus-sig preimage are both byte-exact.
func TestPLCUpdateOpSigVerification(t *testing.T) {
	prev, err := cid.Decode("bafyreibfvkh3n6odvdpwj54j4xxdsgnn4zo5utbyf7z7nfbyikhtygzjcq")
	if err != nil {
		t.Fatalf("prev cid: %v", err)
	}
	op := &plcOperation{
		rotationDIDKeys: []string{
			"did:key:zQ3shhCGUqDKjStzuDxPkTxN6ujddP4RkEKJJouJGRRkaLGbg",
			"did:key:zQ3shpKnbdPx3g3CmPf5cRVTPe1HtSwVn5ish3wSnDPQCbLJK",
		},
		signingDIDKey: "did:key:zQ3shXjHeiBuRCKmM36cuYnm7YEMzhGnCmCyW92sRJ9pribSF",
		alsoKnownAs:   []string{"at://atproto.com"},
		pdsEndpoint:   "https://bsky.social",
		prev:          &prev,
	}
	payload, err := op.encode(false)
	if err != nil {
		t.Fatalf("unsigned encode: %v", err)
	}
	sig, err := base64.RawURLEncoding.DecodeString("lRDLz1RRcauzDos9LZ0Q5bi3YzJXbpgrUpZ51e__tdg89xYgHiWWtnKcrAJanBMkgW0uloD40TYWMVXyZWi4mw")
	if err != nil {
		t.Fatalf("sig decode: %v", err)
	}
	// Either rotation key could have signed — verify against both.
	for _, rk := range op.rotationDIDKeys {
		pub, perr := atcrypto.ParsePublicMultibase(strings.TrimPrefix(rk, "did:key:"))
		if perr != nil {
			t.Fatalf("rotation pubkey %s: %v", rk, perr)
		}
		if verr := pub.HashAndVerify(payload, sig); verr == nil {
			return // verified — encoding is byte-exact
		}
	}
	t.Fatal("no rotation key verifies — unsigned dag-cbor or prev-link encoding diverges")
}

// TestPLCOpRoundTrip exercises our own build→sign→derive chain: the
// derived DID is well-formed and the sig verifies against the rotation
// pubkey under the identical unsigned encoding.
func TestPLCOpRoundTrip(t *testing.T) {
	privHex, pubMB, err := generateSigningKey()
	if err != nil {
		t.Fatalf("rotation keygen: %v", err)
	}
	op := newPLCOperation(pubMB, "zSigningPub", "alice.test.dev", "https://api.test.dev", nil)
	if err := signPLCOp(op, privHex); err != nil {
		t.Fatalf("sign: %v", err)
	}
	if op.sig == "" {
		t.Fatal("sig empty after sign")
	}
	did, err := op.did()
	if err != nil {
		t.Fatalf("did: %v", err)
	}
	if !strings.HasPrefix(did, plcDIDPrefix) || len(did) != len(plcDIDPrefix)+plcDIDSuffixLen {
		t.Fatalf("malformed did %q", did)
	}
	// Sig verifies against the unsigned preimage under the rotation key.
	payload, err := op.encode(false)
	if err != nil {
		t.Fatalf("unsigned encode: %v", err)
	}
	sig, err := base64.RawURLEncoding.DecodeString(op.sig)
	if err != nil {
		t.Fatalf("sig decode: %v", err)
	}
	pub, err := atcrypto.ParsePublicMultibase(pubMB)
	if err != nil {
		t.Fatalf("pubkey: %v", err)
	}
	if err := pub.HashAndVerify(payload, sig); err != nil {
		t.Fatalf("own sig does not verify: %v", err)
	}
	// The wire form carries prev:null and the sig.
	wire, err := op.json()
	if err != nil {
		t.Fatalf("json: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(wire, &m); err != nil {
		t.Fatalf("wire decode: %v", err)
	}
	if m["type"] != plcOpType || m["prev"] != nil || m["sig"] != op.sig {
		t.Fatalf("wire form wrong: %s", wire)
	}
}

// plcStub records directory calls: POST /{did} accepts ops, GET /{did}
// answers registered/unregistered, GET /{did}/log/audit serves the op
// log the update path derives `prev` from.
type plcStub struct {
	t          *testing.T
	registered map[string]bool
	posts      []map[string]any
	headCID    string // served as the audit-log head for updates
}

func (s *plcStub) handler() http.Handler {
	s.registered = map[string]bool{}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		did := strings.TrimPrefix(r.URL.Path, "/")
		switch {
		case r.Method == http.MethodPost:
			var op map[string]any
			if err := json.NewDecoder(r.Body).Decode(&op); err != nil {
				http.Error(w, "bad op", http.StatusBadRequest)
				return
			}
			s.posts = append(s.posts, op)
			s.registered[did] = true
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/log/audit"):
			did = strings.TrimSuffix(did, "/log/audit")
			if !s.registered[did] {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"did": did, "cid": s.headCID, "nullified": false,
					"createdAt": "2025-01-01T00:00:00Z", "operation": map[string]any{}},
			})
		default:
			if s.registered[did] {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"id": did})
				return
			}
			http.Error(w, "not found", http.StatusNotFound)
		}
	})
	return mux
}

// TestDBMigratePLC runs the full swap against a stub directory:
// genesis submit → did swap → URI rewrite → #identity — and the
// idempotent second run.
func TestDBMigratePLC(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")

	stub := &plcStub{t: t}
	srv := httptest.NewServer(stub.handler())
	t.Cleanup(srv.Close)
	svc := newAtprotoServiceWithPLC(t, d, srv.URL)

	// A did:web account with two indexed records — opt-in under a dead
	// directory lands the fallback path.
	dead := newAtprotoServiceWithPLC(t, d, "http://127.0.0.1:1")
	st, err := dead.EnableAccount(ctx, userID)
	if err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	oldDID := st.DID
	if !strings.HasPrefix(oldDID, "did:web:") {
		t.Fatalf("fallback did = %q, want did:web", oldDID)
	}
	acct, err := svc.store.accountByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("accountByUserID: %v", err)
	}
	q := gen.New(d.Writer)
	for i, rkey := range []string{"r1", "r2"} {
		if _, err := q.UpsertAtprotoRecord(ctx, gen.UpsertAtprotoRecordParams{
			Uri:        "at://" + oldDID + "/app.bsky.feed.post/" + rkey,
			AccountID:  acct.ID,
			Collection: "app.bsky.feed.post",
			Rkey:       rkey,
			Cid:        "bafytest" + string(rune('a'+i)),
		}); err != nil {
			t.Fatalf("seed record: %v", err)
		}
	}

	// Migrate — the stub accepts the genesis op.
	if err := svc.MigratePLC(ctx, acct.ID); err != nil {
		t.Fatalf("MigratePLC: %v", err)
	}
	if len(stub.posts) != 1 {
		t.Fatalf("stub posts = %d, want 1", len(stub.posts))
	}
	posted := stub.posts[0]
	if posted["type"] != plcOpType || posted["prev"] != nil {
		t.Fatalf("genesis op wrong: %v", posted)
	}

	acct2, err := svc.store.accountByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if !strings.HasPrefix(acct2.Did, plcDIDPrefix) {
		t.Fatalf("did after swap = %q", acct2.Did)
	}
	if acct2.PreviousDid == nil || *acct2.PreviousDid != oldDID {
		t.Fatalf("previous_did = %v, want %q", acct2.PreviousDid, oldDID)
	}
	if acct2.PlcRotationKeyEnc == nil {
		t.Fatal("rotation key not stored")
	}
	if len(*acct2.PlcRotationKeyEnc) == 64 {
		t.Error("rotation key stored as plaintext hex — expected ciphertext")
	}
	if _, err := jcrypto.Decrypt(*acct2.PlcRotationKeyEnc); err != nil {
		t.Fatalf("rotation key undecryptable: %v", err)
	}
	// The submitted op must carry OUR keys + handle.
	rotKeys, _ := posted["rotationKeys"].([]any)
	vms, _ := posted["verificationMethods"].(map[string]any)
	if len(rotKeys) != 1 || vms["atproto"] != "did:key:"+acct2.PublicKeyMultibase {
		t.Fatalf("op keys wrong: %v", posted)
	}
	akas, _ := posted["alsoKnownAs"].([]any)
	if len(akas) != 1 || akas[0] != "at://"+acct2.Handle {
		t.Fatalf("op alsoKnownAs wrong: %v", posted)
	}

	// Both record URIs rewritten to the new authority.
	uris, err := q.ListAtprotoRecords(ctx, acct.ID)
	if err != nil {
		t.Fatalf("list records: %v", err)
	}
	for _, r := range uris {
		if !strings.HasPrefix(r.Uri, "at://"+acct2.Did+"/") {
			t.Fatalf("record uri not rewritten: %s", r.Uri)
		}
	}

	// #identity emitted for the new DID.
	var kind string
	if err := d.Writer.QueryRow(ctx,
		`SELECT kind FROM atproto_repo_events WHERE did = $1 ORDER BY seq DESC LIMIT 1`,
		acct2.Did).Scan(&kind); err != nil {
		t.Fatalf("identity event: %v", err)
	}
	if kind != EventKindIdentity {
		t.Fatalf("event kind = %q, want identity", kind)
	}

	// Idempotent: a second run no-ops (already did:plc).
	if err := svc.MigratePLC(ctx, acct.ID); err != nil {
		t.Fatalf("MigratePLC re-run: %v", err)
	}
	if len(stub.posts) != 1 {
		t.Fatalf("re-run submitted another op: %d posts", len(stub.posts))
	}
}

// TestDBMigratePLCCrashRetry covers the registered-skip arm: when the
// DID is already in the directory (an earlier attempt submitted but
// crashed pre-swap), MigratePLC goes straight to the swap.
func TestDBMigratePLCCrashRetry(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")

	stub := &plcStub{t: t}
	srv := httptest.NewServer(stub.handler())
	t.Cleanup(srv.Close)
	svc := newAtprotoServiceWithPLC(t, d, srv.URL)

	dead := newAtprotoServiceWithPLC(t, d, "http://127.0.0.1:1")
	if _, err := dead.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	acct, err := svc.store.accountByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("accountByUserID: %v", err)
	}

	// Simulate the crashed attempt: the derived DID is already
	// registered. We can't precompute it without the op — submit via a
	// first real run against a stub that drops POSTs? Simpler: register
	// the DID by running the whole migration once… but that's the
	// previous test. Here we assert the resolve-200 path directly: stub
	// pre-marks every did as registered.
	stub2 := &plcStub{t: t}
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var op map[string]any
			_ = json.NewDecoder(r.Body).Decode(&op)
			stub2.posts = append(stub2.posts, op)
			w.WriteHeader(http.StatusOK)
			return
		}
		// Every resolve answers 200 — the crash-retry path.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "did:plc:whatever"})
	}))
	t.Cleanup(srv2.Close)
	svc2 := newAtprotoServiceWithPLC(t, d, srv2.URL)

	if err := svc2.MigratePLC(ctx, acct.ID); err != nil {
		t.Fatalf("MigratePLC: %v", err)
	}
	if len(stub2.posts) != 0 {
		t.Fatalf("registered-skip still posted: %d ops", len(stub2.posts))
	}
	acct2, err := svc.store.accountByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if !strings.HasPrefix(acct2.Did, plcDIDPrefix) {
		t.Fatalf("did = %q after registered-skip swap", acct2.Did)
	}
}

// TestDBOldDIDProvenance covers the previous_did fallbacks: the did:web
// doc keeps serving under the OLD did's URL shape, and inbound
// resolution rewrites old-authority record URIs to the current DID.
func TestDBOldDIDProvenance(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	userID, _ := newAtprotoUser(t, ctx, d, "active")

	dead := newAtprotoServiceWithPLC(t, d, "http://127.0.0.1:1")
	st, err := dead.EnableAccount(ctx, userID)
	if err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	oldDID := st.DID
	acct, err := dead.store.accountByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("accountByUserID: %v", err)
	}
	// Simulate a completed migration.
	newDID := "did:plc:testmigrated"
	if _, err := d.Writer.Exec(ctx,
		`UPDATE atproto_accounts SET did = $1, previous_did = $2 WHERE id = $3`,
		newDID, oldDID, acct.ID); err != nil {
		t.Fatalf("simulate swap: %v", err)
	}
	// Seed a record under the CURRENT did (post-rewrite state).
	q := gen.New(d.Writer)
	if _, err := q.UpsertAtprotoRecord(ctx, gen.UpsertAtprotoRecordParams{
		Uri:        "at://" + newDID + "/app.bsky.feed.post/rkey1",
		AccountID:  acct.ID,
		Collection: "app.bsky.feed.post",
		Rkey:       "rkey1",
		Cid:        "bafytestz",
	}); err != nil {
		t.Fatalf("seed record: %v", err)
	}

	// The did:web doc endpoint still resolves the OLD url's did —
	// served with the OLD did as document id.
	doc, err := dead.DIDDocument(ctx, uuidString(userID))
	if err != nil {
		t.Fatalf("DIDDocument: %v", err)
	}
	if doc["id"] != oldDID {
		t.Fatalf("doc id = %v, want previous did %q", doc["id"], oldDID)
	}

	// Inbound URI resolution falls back through previous_did.
	rec, err := dead.resolveLocalRecordURI(ctx, q, "at://"+oldDID+"/app.bsky.feed.post/rkey1")
	if err != nil {
		t.Fatalf("old-did record resolution: %v", err)
	}
	if rec.Uri != "at://"+newDID+"/app.bsky.feed.post/rkey1" {
		t.Fatalf("resolved uri = %q", rec.Uri)
	}
	// A URI whose authority was never ours still misses.
	if _, err := dead.resolveLocalRecordURI(ctx, q, "at://did:plc:stranger/app.bsky.feed.post/x"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("foreign authority should miss, got %v", err)
	}
	// Follow-subject resolution matches previous_did too.
	fa, err := q.GetAtprotoAccountByDID(ctx, oldDID)
	if err != nil {
		t.Fatalf("follow subject by old did: %v", err)
	}
	if fa.Did != newDID {
		t.Fatalf("follow subject row did = %q", fa.Did)
	}
}
