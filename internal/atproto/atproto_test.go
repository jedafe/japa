package atproto

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/multiformats/go-multibase"
)

func testService() *Service {
	return &Service{cfg: Config{
		PDSURL:       "https://api.japa.to",
		APIBase:      "https://api.japa.to",
		SiteBase:     "https://japa.to",
		HandleDomain: "japa.to",
	}}
}

func TestMain(m *testing.M) {
	// internal/crypto caches its key set on first use — a default key must
	// exist before any test that touches Encrypt/Decrypt.
	if os.Getenv("JC_ENCRYPTION_KEY") == "" &&
		os.Getenv("JC_STOCK_ENCRYPTION_KEY") == "" &&
		os.Getenv("JC_TOTP_ENCRYPTION_KEY") == "" {
		os.Setenv("JC_ENCRYPTION_KEY", "atproto-test-key")
		os.Setenv("JC_ENV", "development")
	}
	os.Exit(m.Run())
}

// --- keys ------------------------------------------------------------------

// The publicKeyMultibase encoding is the conformance-critical primitive:
// multibase(base58btc) of uvarint(secp256k1-pub=0xE7) || compressed-33.
// Decoding must round-trip to exactly [0xE7 0x01 || 33-byte pubkey].
func TestGenerateSigningKeyRoundTrip(t *testing.T) {
	privHex, pubMB, err := generateSigningKey()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}

	priv, err := parsePrivateKeyHex(privHex)
	if err != nil {
		t.Fatalf("parse priv: %v", err)
	}
	reDerived, err := publicKeyMultibase(priv.PubKey())
	if err != nil {
		t.Fatalf("publicKeyMultibase: %v", err)
	}
	if reDerived != pubMB {
		t.Error("re-derived publicKeyMultibase differs from stored value")
	}

	// Decode the multibase and verify the multicodec prefix + length.
	_, decoded, err := multibase.Decode(pubMB)
	if err != nil {
		t.Fatalf("multibase decode: %v", err)
	}
	if len(decoded) != 35 {
		t.Fatalf("decoded key length = %d, want 35 (2-byte varint + 33-byte compressed)", len(decoded))
	}
	if decoded[0] != 0xe7 || decoded[1] != 0x01 {
		t.Errorf("multicodec prefix = %x %x, want e7 01 (secp256k1-pub)", decoded[0], decoded[1])
	}
	if pubMB[0] != 'z' {
		t.Errorf("multibase must be base58btc ('z' prefix), got %q", pubMB[:4])
	}
}

func TestParsePrivateKeyRejectsGarbage(t *testing.T) {
	for _, bad := range []string{"zz", "", "deadbeef"} {
		if _, err := parsePrivateKeyHex(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

// --- DID + handle derivation ------------------------------------------------

func TestDIDAndHandleDerivation(t *testing.T) {
	cfg := testService().cfg

	if got := cfg.didFor("550e8400-e29b-41d4-a716-446655440000"); got != "did:web:api.japa.to:atproto:u:550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("didFor = %q", got)
	}
	if got := cfg.handleFor("alice"); got != "alice.japa.to" {
		t.Errorf("handleFor = %q", got)
	}
	if got := cfg.pdsHost(); got != "api.japa.to" {
		t.Errorf("pdsHost = %q", got)
	}
}

func TestHandleDomainFallbacks(t *testing.T) {
	// SiteBase host derivation (HandleDomain unset)
	cfg := Config{SiteBase: "https://japa.to"}
	if got := cfg.handleDomain(); got != "japa.to" {
		t.Errorf("handleDomain fallback = %q", got)
	}
	// PDSURL falls back to APIBase
	cfg = Config{APIBase: "https://api.japa.to"}
	if got := cfg.pdsURL(); got != "https://api.japa.to" {
		t.Errorf("pdsURL fallback = %q", got)
	}
}

func TestHandleDomainStripsPort(t *testing.T) {
	// Dev JC_SITE_URL values carry ports — handles are DNS names and
	// can't; "alice.localhost:3000" would never resolve.
	cfg := Config{SiteBase: "http://localhost:3000"}
	if got := cfg.handleDomain(); got != "localhost" {
		t.Errorf("handleDomain(SiteBase w/ port) = %q, want localhost", got)
	}
	cfg = Config{HandleDomain: "JAPA.TO:443"}
	if got := cfg.handleDomain(); got != "japa.to" {
		t.Errorf("handleDomain(config w/ port) = %q, want japa.to", got)
	}
	// pdsHost keeps its port deliberately — did:web encodes it (%3A).
	cfg = Config{PDSURL: "http://localhost:8080"}
	if got := cfg.pdsHost(); got != "localhost:8080" {
		t.Errorf("pdsHost = %q, want port preserved", got)
	}
}

// validHandle is the mint-time gate: the username-derived label must be
// DNS-legal (the §50 charset allows '_', atproto labels don't) and the
// derived handle must not collide with the PDS host.
func TestValidHandle(t *testing.T) {
	svc := testService() // pdsHost api.japa.to, domain japa.to
	cases := []struct {
		username, handle string
		want             bool
	}{
		{"alice", "alice.japa.to", true},
		{"alice-w", "alice-w.japa.to", true},
		{"Alice9", "alice9.japa.to", true},    // casefolds into a legal label
		{"9lives", "9lives.japa.to", true},    // leading digit is legal in labels
		{"alice_w", "alice_w.japa.to", false}, // underscore: legal username, illegal handle
		{"-alice", "-alice.japa.to", false},   // leading hyphen
		{"alice-", "alice-.japa.to", false},   // trailing hyphen
		{"api", "api.japa.to", false},         // collides with the PDS host itself
	}
	for _, c := range cases {
		if got := svc.validHandle(c.username, c.handle); got != c.want {
			t.Errorf("validHandle(%q, %q) = %v, want %v", c.username, c.handle, got, c.want)
		}
	}
}

// invalidateAccount must drop the real cache keys the service writes:
// jc:atp:did:{user_uuid} (keyed by the did:web u/{id} path component —
// the USER uuid, not the account row PK) and jc:atp:account:{handle}.
func TestInvalidateAccountDropsRealKeys(t *testing.T) {
	vk := startTestCompat(t)
	svc := testService()
	svc.vk = vk
	ctx := context.Background()

	if err := vk.Set(ctx, "jc:atp:did:some-user-uuid", []byte(`{}`), 0).Err(); err != nil {
		t.Fatalf("seed did key: %v", err)
	}
	if err := vk.Set(ctx, "jc:atp:account:alice.japa.to", []byte("did:web:x"), 0).Err(); err != nil {
		t.Fatalf("seed account key: %v", err)
	}

	// Mixed-case args must still hit the lowercase-normalized keys.
	svc.invalidateAccount(ctx, "Alice.JAPA.to", "SOME-USER-UUID")

	if _, err := vk.Get(ctx, "jc:atp:did:some-user-uuid").Result(); err == nil {
		t.Error("jc:atp:did:* still cached after invalidation")
	}
	if _, err := vk.Get(ctx, "jc:atp:account:alice.japa.to").Result(); err == nil {
		t.Error("jc:atp:account:* still cached after invalidation")
	}
}

// --- HandleFromHost ----------------------------------------------------------

func TestHandleFromHost(t *testing.T) {
	svc := testService()

	cases := []struct {
		host string
		want string
	}{
		{"alice.japa.to", "alice.japa.to"},
		{"ALICE.japa.to", "alice.japa.to"},     // case-insensitive
		{"alice.japa.to:443", "alice.japa.to"}, // port stripped
		{"japa.to", ""},                        // bare domain isn't a handle
		{"api.japa.to", ""},                    // the PDS host itself isn't a handle
		{"example.com", ""},                    // foreign domain
		{"", ""},
	}
	for _, c := range cases {
		if got := svc.HandleFromHost(c.host); got != c.want {
			t.Errorf("HandleFromHost(%q) = %q, want %q", c.host, got, c.want)
		}
	}
}

// --- DID document ------------------------------------------------------------

func TestDIDDocumentShape(t *testing.T) {
	doc := didDocument(
		"did:web:api.japa.to:atproto:u:alice",
		"alice.japa.to",
		"zTestKey",
		"https://api.japa.to",
	)

	if doc["id"] != "did:web:api.japa.to:atproto:u:alice" {
		t.Errorf("id = %v", doc["id"])
	}
	aka := doc["alsoKnownAs"].([]string)
	if len(aka) != 1 || aka[0] != "at://alice.japa.to" {
		t.Errorf("alsoKnownAs = %v", aka)
	}
	vms := doc["verificationMethod"].([]map[string]any)
	vm := vms[0]
	if vm["type"] != "Multikey" || vm["id"] != "did:web:api.japa.to:atproto:u:alice#atproto" {
		t.Errorf("verificationMethod = %+v", vm)
	}
	if vm["publicKeyMultibase"] != "zTestKey" {
		t.Errorf("publicKeyMultibase = %v", vm["publicKeyMultibase"])
	}
	svcs := doc["service"].([]map[string]any)
	svc := svcs[0]
	if svc["type"] != "AtprotoPersonalDataServer" || svc["serviceEndpoint"] != "https://api.japa.to" {
		t.Errorf("service = %+v", svc)
	}
}

// --- private key never serializes ----------------------------------------------

func TestAccountStatusNeverSerializesKey(t *testing.T) {
	st := AccountStatus{}
	raw, err := json.Marshal(st)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for k := range m {
		if strings.Contains(strings.ToLower(k), "key") {
			t.Errorf("key-like field %q present on member status view", k)
		}
	}
}

func TestAdminAccountNeverSerializesKey(t *testing.T) {
	a := AdminAccount{SigningKeyEnc: "CIPHERTEXT"}
	raw, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if contains := string(raw); contains != "" {
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		if _, ok := m["signing_key_enc"]; ok {
			t.Error("signing_key_enc leaked into admin JSON")
		}
		if _, ok := m["SigningKeyEnc"]; ok {
			t.Error("SigningKeyEnc leaked into admin JSON")
		}
	}
}

// --- hex encoding used for storage ------------------------------------------------

func TestPrivateKeyHexIs32Bytes(t *testing.T) {
	privHex, _, err := generateSigningKey()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	raw, err := hex.DecodeString(privHex)
	if err != nil || len(raw) != 32 {
		t.Fatalf("privHex decodes to %d bytes (err=%v), want 32", len(raw), err)
	}
}
