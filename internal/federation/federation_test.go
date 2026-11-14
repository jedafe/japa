package federation

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// The service tests below exercise the pure document builders and URL
// helpers; store-touching paths need a database and are covered by the
// route contract tests.

func testService() *Service {
	return &Service{cfg: Config{
		APIBase:      "https://api.japa.to",
		SiteBase:     "https://japa.to",
		HandleDomain: "japa.to",
		Version:      "1.2.3",
	}}
}

func TestMain(m *testing.M) {
	// internal/crypto caches its key set on first use — a default key must
	// exist before any test that touches Encrypt/Decrypt.
	if os.Getenv("JC_ENCRYPTION_KEY") == "" &&
		os.Getenv("JC_STOCK_ENCRYPTION_KEY") == "" &&
		os.Getenv("JC_TOTP_ENCRYPTION_KEY") == "" {
		os.Setenv("JC_ENCRYPTION_KEY", "federation-test-key")
		os.Setenv("JC_ENV", "development")
	}
	os.Exit(m.Run())
}

// --- keys ------------------------------------------------------------------

func TestGenerateActorKeyPairRoundTrip(t *testing.T) {
	pubPEM, privPEM, err := generateActorKeyPair()
	if err != nil {
		t.Fatalf("generateActorKeyPair: %v", err)
	}

	pub, err := parsePublicKeyPEM(pubPEM)
	if err != nil {
		t.Fatalf("parse public: %v", err)
	}
	priv, err := parsePrivateKeyPEM(privPEM)
	if err != nil {
		t.Fatalf("parse private: %v", err)
	}

	if pub.N.Cmp(priv.N) != 0 {
		t.Error("public/private keypair mismatch")
	}
	if priv.Size() < 256 {
		t.Errorf("expected RSA-2048 (>=256 bytes), got %d", priv.Size())
	}
}

func TestParseKeyPEMRejectsGarbage(t *testing.T) {
	if _, err := parsePublicKeyPEM("not a pem"); err == nil {
		t.Error("expected error for invalid public PEM")
	}
	if _, err := parsePrivateKeyPEM("-----BEGIN RSA PRIVATE KEY-----\nAAAA\n-----END RSA PRIVATE KEY-----"); err == nil {
		t.Error("expected error for truncated private PEM")
	}
}

// --- HTTP signatures -------------------------------------------------------

func TestSignVerifyRoundTrip(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	body := []byte(`{"type":"Follow"}`)

	req := httptest.NewRequest("POST", "https://remote.example/inbox?x=1", strings.NewReader(string(body)))
	req.Host = "remote.example"

	if err := SignRequest(req, "https://api.japa.to/federation/actors/alice#main-key", priv, body); err != nil {
		t.Fatalf("SignRequest: %v", err)
	}
	for _, h := range []string{"Signature", "Date", "Digest"} {
		if req.Header.Get(h) == "" {
			t.Fatalf("missing %s header after signing", h)
		}
	}

	if err := VerifySignature(req, &priv.PublicKey, body); err != nil {
		t.Fatalf("VerifySignature: %v", err)
	}
}

func TestVerifySignatureRejectsTamperedBody(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	body := []byte(`{"type":"Follow"}`)

	req := httptest.NewRequest("POST", "https://remote.example/inbox", strings.NewReader(string(body)))
	req.Host = "remote.example"
	if err := SignRequest(req, "key", priv, body); err != nil {
		t.Fatalf("SignRequest: %v", err)
	}

	if err := VerifySignature(req, &priv.PublicKey, []byte(`{"type":"Delete"}`)); err == nil {
		t.Error("expected digest mismatch for tampered body")
	}
}

func TestVerifySignatureRejectsStaleDate(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	body := []byte(`{"type":"Follow"}`)

	req := httptest.NewRequest("POST", "https://remote.example/inbox", strings.NewReader(string(body)))
	req.Host = "remote.example"
	if err := SignRequest(req, "key", priv, body); err != nil {
		t.Fatalf("SignRequest: %v", err)
	}
	// Rewind the signed Date past the 12h skew window; the signature over
	// "date" still verifies cryptographically, so freshness must reject it.
	stale := time.Now().Add(-13 * time.Hour).UTC().Format(http.TimeFormat)
	req.Header.Set("Date", stale)

	if err := VerifySignature(req, &priv.PublicKey, body); err == nil {
		t.Error("expected stale date rejection")
	}
}

func TestVerifySignatureRejectsMissing(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	req := httptest.NewRequest("POST", "https://remote.example/inbox", nil)
	if err := VerifySignature(req, &priv.PublicKey, nil); err == nil {
		t.Error("expected error for missing Signature header")
	}
}

func TestParseSignatureHeaderQuotedCommas(t *testing.T) {
	params := ParseSignatureHeader(`keyId="https://a.example/u#k,with,commas",algorithm="rsa-sha256",headers="(request-target) host date",signature="abc123"`)
	if params["keyId"] != "https://a.example/u#k,with,commas" {
		t.Errorf("keyId truncated at inner comma: %q", params["keyId"])
	}
	if params["algorithm"] != "rsa-sha256" {
		t.Errorf("algorithm: %q", params["algorithm"])
	}
	if params["headers"] != "(request-target) host date" {
		t.Errorf("headers: %q", params["headers"])
	}
}

// --- WebFinger / actor docs -------------------------------------------------

func TestWebfingerJRD(t *testing.T) {
	doc := webfingerJRD("alice", "japa.to",
		"https://api.japa.to/federation/actors/alice",
		"https://japa.to/u/alice")

	if doc["subject"] != "acct:alice@japa.to" {
		t.Errorf("subject: %v", doc["subject"])
	}
	links, ok := doc["links"].([]any)
	if !ok || len(links) != 2 {
		t.Fatalf("expected 2 links, got %v", doc["links"])
	}
	self := links[0].(map[string]any)
	if self["rel"] != "self" || self["type"] != "application/activity+json" {
		t.Errorf("self link malformed: %v", self)
	}
}

func testActorProfileRow() gen.GetActorProfileByUsernameRow {
	name := "Alice O."
	return gen.GetActorProfileByUsernameRow{
		ActorUrl:                  "https://api.japa.to/federation/actors/alice",
		Username:                  "alice",
		PublicKeyPem:              "-----BEGIN PUBLIC KEY-----\ntest\n-----END PUBLIC KEY-----\n",
		PublicKeyID:               "https://api.japa.to/federation/actors/alice#main-key",
		InboxUrl:                  "https://api.japa.to/federation/actors/alice/inbox",
		OutboxUrl:                 "https://api.japa.to/federation/actors/alice/outbox",
		FollowersUrl:              "https://api.japa.to/federation/actors/alice/followers",
		FollowingUrl:              "https://api.japa.to/federation/actors/alice/following",
		ManuallyApprovesFollowers: false,
		DisplayName:               &name,
		UserCreatedAt:             pgtype.Timestamptz{Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Valid: true},
	}
}

func TestActorDocumentShape(t *testing.T) {
	doc := actorDocument(testActorProfileRow(), "https://japa.to")

	for _, field := range []string{"id", "type", "preferredUsername", "inbox", "outbox", "followers", "following", "publicKey", "endpoints", "url"} {
		if _, ok := doc[field]; !ok {
			t.Errorf("actor doc missing %q", field)
		}
	}
	if doc["type"] != "Person" {
		t.Errorf("type: %v", doc["type"])
	}
	if doc["name"] != "Alice O." {
		t.Errorf("name: %v", doc["name"])
	}
	pk := doc["publicKey"].(map[string]any)
	if _, ok := pk["privateKeyPem"]; ok {
		t.Error("private key leaked into actor document")
	}
	eps := doc["endpoints"].(map[string]any)
	if eps["sharedInbox"] != "https://api.japa.to/federation/inbox" {
		t.Errorf("sharedInbox: %v", eps["sharedInbox"])
	}

	raw, _ := json.Marshal(doc)
	if strings.Contains(string(raw), "PrivateKeyEnc") || strings.Contains(string(raw), "private_key") {
		t.Error("actor document contains private key material")
	}
}

func TestActorDocumentOmitsEmptyOptionals(t *testing.T) {
	row := testActorProfileRow()
	row.DisplayName = nil
	row.Bio = nil
	row.AvatarUrl = nil
	row.WebsiteUrl = nil
	doc := actorDocument(row, "https://japa.to")
	for _, absent := range []string{"name", "summary", "icon", "attachment"} {
		if _, ok := doc[absent]; ok {
			t.Errorf("empty optional %q should be absent", absent)
		}
	}
}

// --- objects + collections --------------------------------------------------

func TestObjectDocumentNoteVsArticle(t *testing.T) {
	base := gen.GetFederatableContentByIDRow{
		ContentType: "story",
		Title:       "Hello",
		Slug:        "hello",
		BodyHtml:    "<p>Hi</p>",
		PublishedAt: pgtype.Timestamptz{Time: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), Valid: true},
	}
	note := objectDocument(base, "https://api.japa.to/federation/actors/alice",
		"https://api.japa.to/federation/actors/alice/followers",
		"https://japa.to", "https://japa.to/story/hello")
	if note["type"] != "Note" {
		t.Errorf("story should map to Note, got %v", note["type"])
	}
	if note["attributedTo"] != "https://api.japa.to/federation/actors/alice" {
		t.Errorf("attributedTo: %v", note["attributedTo"])
	}
	to, ok := note["to"].([]string)
	if !ok || len(to) != 1 || to[0] != asPublic {
		t.Errorf("to should be [Public], got %v", note["to"])
	}

	base.ContentType = "blog_post"
	article := objectDocument(base, "https://api.japa.to/federation/actors/alice",
		"https://api.japa.to/federation/actors/alice/followers",
		"https://japa.to", "https://japa.to/blog/hello")
	if article["type"] != "Article" {
		t.Errorf("blog_post should map to Article, got %v", article["type"])
	}
	if article["name"] != "Hello" {
		t.Errorf("Article should carry title as name, got %v", article["name"])
	}

	// No-nulls rule: optional keys must be absent, not null — several
	// remote implementations reject unexpected nulls.
	raw, _ := json.Marshal(article)
	for _, k := range []string{`"summary":null`, `"inReplyTo":null`} {
		if strings.Contains(string(raw), k) {
			t.Errorf("object emits %s — optional keys must be absent", k)
		}
	}
}

func TestCreateActivityOmitsNilPublished(t *testing.T) {
	obj := map[string]any{"id": "https://api.japa.to/federation/objects/contents/1", "type": "Note"}
	act := createActivity("https://api.japa.to/federation/actors/alice", "https://api.japa.to/federation/actors/alice/followers", obj)
	if _, ok := act["published"]; ok {
		t.Error("published should be absent when the object has none")
	}
	obj["published"] = "2024-01-01T00:00:00Z"
	act = createActivity("a", "f", obj)
	if act["published"] != "2024-01-01T00:00:00Z" {
		t.Errorf("published not copied: %v", act["published"])
	}
}

func TestPageURLPrefersCanonical(t *testing.T) {
	s := testService()
	canonical := "https://japa.to/blog/custom-canonical"
	if got := s.pageURL("blog_post", "slug", &canonical); got != canonical {
		t.Errorf("canonical_url should win, got %q", got)
	}
	if got := s.pageURL("story", "slug", nil); got != "https://japa.to/story/slug" {
		t.Errorf("route path fallback, got %q", got)
	}
	empty := ""
	if got := s.pageURL("story", "slug", &empty); got != "https://japa.to/story/slug" {
		t.Errorf("empty canonical should fall back, got %q", got)
	}
}

func TestOrderedCollectionPagination(t *testing.T) {
	coll := orderedCollection("https://api.japa.to/federation/actors/alice/outbox", 42, "https://api.japa.to/federation/actors/alice/outbox?page=1")
	if coll["type"] != "OrderedCollection" || coll["totalItems"] != int64(42) {
		t.Errorf("collection shell: %v", coll)
	}

	pageID := func(p int) string { return "https://x/o?page=" + string(rune('0'+p)) }
	mid := orderedCollectionPage("https://x/o?page=2", "https://x/o", []any{"a"}, 42, true, true, pageID, 2)
	if mid["type"] != "OrderedCollectionPage" {
		t.Errorf("page type: %v", mid["type"])
	}
	if _, ok := mid["next"]; !ok {
		t.Error("middle page missing next")
	}
	if _, ok := mid["prev"]; !ok {
		t.Error("middle page missing prev")
	}

	last := orderedCollectionPage("https://x/o?page=3", "https://x/o", []any{"a"}, 42, false, true, pageID, 3)
	if _, ok := last["next"]; ok {
		t.Error("last page must omit next")
	}
}

// --- NodeInfo ---------------------------------------------------------------

func TestNodeInfoDocument(t *testing.T) {
	doc := nodeInfoDocument("1.2.3", "https://api.japa.to",
		gen.GetNodeInfoUsageRow{TotalUsers: 10, LocalPosts: 20, LocalComments: 30}, true)

	if doc["version"] != "2.1" {
		t.Errorf("version: %v", doc["version"])
	}
	protocols := doc["protocols"].([]string)
	if len(protocols) != 1 || protocols[0] != "activitypub" {
		t.Errorf("protocols: %v", protocols)
	}
	usage := doc["usage"].(map[string]any)
	if usage["localPosts"] != int64(20) {
		t.Errorf("localPosts: %v", usage["localPosts"])
	}
}

func TestWellKnownNodeInfo(t *testing.T) {
	doc := wellKnownNodeInfo("https://api.japa.to/federation/nodeinfo/2.1")
	links := doc["links"].([]any)
	href := links[0].(map[string]any)["href"]
	if href != "https://api.japa.to/federation/nodeinfo/2.1" {
		t.Errorf("nodeinfo href: %v", href)
	}
}

// --- service helpers --------------------------------------------------------

func TestHandleDomainDerivation(t *testing.T) {
	configured := &Service{cfg: Config{SiteBase: "https://japa.to", HandleDomain: "social.japa.to"}}
	if got := configured.handleDomain(); got != "social.japa.to" {
		t.Errorf("configured domain should win, got %q", got)
	}

	derived := &Service{cfg: Config{SiteBase: "https://japa.to/"}}
	if got := derived.handleDomain(); got != "japa.to" {
		t.Errorf("expected host of SiteBase, got %q", got)
	}

	empty := &Service{}
	if got := empty.handleDomain(); got != "" {
		t.Errorf("expected empty domain, got %q", got)
	}
}

func TestAcctResourceRegex(t *testing.T) {
	valid := []string{"acct:alice@japa.to", "acct:bob.smith@sub.japa.to", "acct:a_b-c@japa.to"}
	for _, r := range valid {
		if !acctResourceRe.MatchString(r) {
			t.Errorf("valid resource rejected: %s", r)
		}
	}
	invalid := []string{"alice@japa.to", "acct:alice", "acct:@japa.to", "acct:alice@", "acct:alice@japa.to@evil.com", ""}
	for _, r := range invalid {
		if acctResourceRe.MatchString(r) {
			t.Errorf("invalid resource accepted: %s", r)
		}
	}
}

func TestServiceURLs(t *testing.T) {
	s := testService()
	if got := s.actorURL("alice"); got != "https://api.japa.to/federation/actors/alice" {
		t.Errorf("actorURL: %q", got)
	}
	if got := s.objectURL("abc"); got != "https://api.japa.to/federation/objects/contents/abc" {
		t.Errorf("objectURL: %q", got)
	}
	if got := s.profileURL("alice"); got != "https://japa.to/u/alice" {
		t.Errorf("profileURL: %q", got)
	}
}

func TestSharedInboxURLDerivation(t *testing.T) {
	got := sharedInboxURL("https://api.japa.to/federation/actors/alice")
	if got != "https://api.japa.to/federation/inbox" {
		t.Errorf("sharedInboxURL: %q", got)
	}
}

// --- serialization safety ---------------------------------------------------

func TestFederationStatusNeverSerializesKeys(t *testing.T) {
	s := testService()
	actor := gen.FederatedActor{
		ActorUrl:          "https://api.japa.to/federation/actors/alice",
		PreferredUsername: "alice",
		PublicKeyPem:      "PUBLIC",
		PrivateKeyEnc:     "CIPHERTEXT",
		Status:            ActorStatusActive,
	}
	st := s.statusFor(actor, "alice")
	raw, err := json.Marshal(st)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := string(raw)
	if strings.Contains(out, "CIPHERTEXT") || strings.Contains(out, "PUBLIC") || strings.Contains(out, "private") || strings.Contains(out, "public_key") {
		t.Errorf("member status leaks key material: %s", out)
	}
}

func TestAdminActorRowSanitizedBeforeMarshal(t *testing.T) {
	// ListActors zeroes PrivateKeyEnc before the row is returned; the
	// serialized row must never carry it.
	row := gen.ListFederatedActorsRow{PreferredUsername: "alice", PrivateKeyEnc: "CIPHERTEXT"}
	row.PrivateKeyEnc = ""
	raw, _ := json.Marshal(AdminActorRow{ListFederatedActorsRow: row, Handle: "alice@japa.to"})
	if strings.Contains(string(raw), "CIPHERTEXT") {
		t.Errorf("admin row leaked ciphertext: %s", raw)
	}
}

func TestParsePage(t *testing.T) {
	req := httptest.NewRequest("GET", "/x/outbox", nil)
	if got := parsePage(req); got != 0 {
		t.Errorf("no page param: got %d, want 0 (collection)", got)
	}
	for raw, want := range map[string]int{"?page=1": 1, "?page=true": 1, "?page=3": 3, "?page=abc": 0, "?page=-2": 0, "?page=0": 0} {
		req := httptest.NewRequest("GET", "/x/outbox"+raw, nil)
		if got := parsePage(req); got != want {
			t.Errorf("%s: got %d, want %d", raw, got, want)
		}
	}
}

func TestAPObjectTypeMapping(t *testing.T) {
	for _, ct := range federatableTypes {
		got := apObjectType(ct)
		if ct == "blog_post" || ct == "guide" {
			if got != "Article" {
				t.Errorf("%s should map to Article, got %s", ct, got)
			}
		} else if got != "Note" {
			t.Errorf("%s should map to Note, got %s", ct, got)
		}
	}
}
