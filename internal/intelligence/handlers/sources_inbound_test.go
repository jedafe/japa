package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// mapSettings is a SettingsReader stub backed by a string map — unlike
// probeTestSettings it returns configured values for any key.
type mapSettings map[string]string

func (m mapSettings) GetIntOr(_ context.Context, key string, fallback int) int {
	if v, ok := m[key]; ok {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return fallback
}

func (m mapSettings) GetStringOr(_ context.Context, key, fallback string) string {
	if v, ok := m[key]; ok {
		return v
	}
	return fallback
}

func signEmail(key, ts, token string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(ts))
	mac.Write([]byte(token))
	return hex.EncodeToString(mac.Sum(nil))
}

// TestVerifyInboundSignature covers the Mailgun-compatible triple: valid
// signature, wrong key/token, stale and future timestamps, missing parts.
func TestVerifyInboundSignature(t *testing.T) {
	t.Parallel()

	key := "whsec-test"
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	token := "tok-123"

	if !verifyInboundSignature(key, ts, token, signEmail(key, ts, token)) {
		t.Error("valid signature rejected")
	}
	if verifyInboundSignature("other-key", ts, token, signEmail(key, ts, token)) {
		t.Error("signature accepted under wrong key")
	}
	if verifyInboundSignature(key, ts, "other-token", signEmail(key, ts, token)) {
		t.Error("signature accepted under wrong token")
	}

	stale := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
	if verifyInboundSignature(key, stale, token, signEmail(key, stale, token)) {
		t.Error("stale timestamp accepted")
	}
	future := strconv.FormatInt(time.Now().Add(10*time.Minute).Unix(), 10)
	if verifyInboundSignature(key, future, token, signEmail(key, future, token)) {
		t.Error("future timestamp beyond skew accepted")
	}

	if verifyInboundSignature(key, "", token, signEmail(key, ts, token)) ||
		verifyInboundSignature(key, ts, "", signEmail(key, ts, token)) ||
		verifyInboundSignature(key, ts, token, "") ||
		verifyInboundSignature("", ts, token, signEmail(key, ts, token)) {
		t.Error("empty component accepted")
	}
}

// TestVerifyWebSubSignature covers X-Hub-Signature sha256=<hex> checks.
func TestVerifyWebSubSignature(t *testing.T) {
	t.Parallel()

	secret := "hub-secret"
	body := []byte("<feed>content</feed>")

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	good := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !verifyWebSubSignature(secret, good, body) {
		t.Error("valid sha256= signature rejected")
	}
	if verifyWebSubSignature(secret, "sha256=deadbeef", body) {
		t.Error("wrong hex accepted")
	}
	if verifyWebSubSignature(secret, "sha1="+strings.TrimPrefix(good, "sha256="), body) {
		t.Error("sha1= scheme accepted — sha256 required")
	}
	if verifyWebSubSignature(secret, "", body) {
		t.Error("empty signature accepted")
	}
	if verifyWebSubSignature("other", good, body) {
		t.Error("signature accepted under wrong secret")
	}
}

// TestInboundSenderAllowed covers the sender gate: channel allowlist wins,
// platform fallback applies when the channel is silent, *@domain globs,
// and the empty-everywhere allow-all.
func TestInboundSenderAllowed(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		cfg       string
		settings  mapSettings
		sender    string
		wantAllow bool
	}{
		{"channel_exact", `{"allowed_senders":["a@ok.example"]}`, nil, "a@ok.example", true},
		{"channel_exact_miss", `{"allowed_senders":["a@ok.example"]}`, nil, "b@ok.example", false},
		{"channel_domain_glob", `{"allowed_senders":["*@embassy.example"]}`, nil, "alerts@embassy.example", true},
		{"channel_glob_miss", `{"allowed_senders":["*@embassy.example"]}`, nil, "alerts@other.example", false},
		{"channel_empty_platform", `{"allowed_senders":[]}`, mapSettings{"intel.email.allowed_senders": `["x@p.example"]`}, "x@p.example", true},
		{"platform_fallback", `{}`, mapSettings{"intel.email.allowed_senders": `["x@p.example","*@gov.example"]`}, "noreply@gov.example", true},
		{"platform_fallback_miss", `{}`, mapSettings{"intel.email.allowed_senders": `["x@p.example"]`}, "y@p.example", false},
		{"empty_everywhere_allows", `{}`, nil, "anyone@anywhere.example", true},
		{"display_name_sender", `{"allowed_senders":["a@ok.example"]}`, nil, "Alice <a@ok.example>", true},
		{"empty_sender_rejected", `{}`, nil, "", false},
		{"case_insensitive", `{"allowed_senders":["A@OK.example"]}`, nil, "a@ok.example", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := inboundSenderAllowed(context.Background(), tc.settings, []byte(tc.cfg), tc.sender)
			if got != tc.wantAllow {
				t.Errorf("inboundSenderAllowed(%q, cfg=%s) = %v, want %v", tc.sender, tc.cfg, got, tc.wantAllow)
			}
		})
	}
}

// TestFirstAddress covers display-name extraction + lowercasing for
// channel recipient matching.
func TestFirstAddress(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ in, want string }{
		{"a@b.example", "a@b.example"},
		{"Alice <a@b.example>", "a@b.example"},
		{"  A@B.EXAMPLE  ", "a@b.example"},
		{"\"Quoted Name\" <q@c.example>", "q@c.example"},
		{"Intel <intel@inbound.japa>, Alerts <alerts@other.example>", "intel@inbound.japa"},
		{"", ""},
	} {
		if got := firstAddress(tc.in); got != tc.want {
			t.Errorf("firstAddress(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestClaimInbound exercises the SET-NX idempotency claim over miniredis:
// first delivery claims, second is a duplicate, release frees the slot.
func TestClaimInbound(t *testing.T) {
	t.Parallel()

	h := &Handlers{rdb: startProbeTestValkey(t)}
	r := httptest.NewRequest(http.MethodPost, "/x", nil)

	key, dup := h.claimInbound(r, "email:ch-1:msg-1")
	if dup || key == "" {
		t.Error("first delivery flagged duplicate")
	}
	if _, dup := h.claimInbound(r, "email:ch-1:msg-1"); !dup {
		t.Error("redelivery not flagged duplicate")
	}
	if _, dup := h.claimInbound(r, "email:ch-1:msg-2"); dup {
		t.Error("distinct delivery flagged duplicate")
	}

	// Post-claim failure releases the slot so a provider retry reclaims it.
	h.releaseInbound(r, key)
	if _, dup := h.claimInbound(r, "email:ch-1:msg-1"); dup {
		t.Error("released claim still flagged duplicate")
	}
}

// TestClaimInbound_NilRDB fails open — a deployment without Valkey still
// accepts (content-hash dedup applies downstream).
func TestClaimInbound_NilRDB(t *testing.T) {
	t.Parallel()

	h := &Handlers{}
	r := httptest.NewRequest(http.MethodPost, "/x", nil)
	if _, dup := h.claimInbound(r, "anything"); dup {
		t.Error("nil rdb flagged duplicate")
	}
}

// TestInboundMIMEHeaders parses routing headers out of a raw RFC822
// message.
func TestInboundMIMEHeaders(t *testing.T) {
	t.Parallel()

	raw := []byte("From: Alice <a@b.example>\r\nTo: Intel <intel@inbound.japa>\r\nSubject: Hi\r\nMessage-Id: <m1@b>\r\nContent-Type: text/plain\r\n\r\nBody text\r\n")

	h, err := inboundMIMEHeaders(raw)
	if err != nil {
		t.Fatalf("inboundMIMEHeaders: %v", err)
	}
	if h.Get("From") != "Alice <a@b.example>" || h.Get("Message-Id") != "<m1@b>" {
		t.Errorf("headers = %v", h)
	}
	if _, err := inboundMIMEHeaders([]byte("\x00\x01\x02")); err == nil {
		t.Error("garbage accepted as MIME")
	}
}

// TestHandleInboundEmail_NotConfigured verifies the endpoint 503s without
// its dependencies — before any signature work.
func TestHandleInboundEmail_NotConfigured(t *testing.T) {
	t.Parallel()

	h := &Handlers{}
	r := httptest.NewRequest(http.MethodPost, "/api/v1/intel/inbound/email", strings.NewReader("x"))
	rr := httptest.NewRecorder()
	h.HandleInboundEmail(rr, r)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 (body=%s)", rr.Code, rr.Body.String())
	}
}
