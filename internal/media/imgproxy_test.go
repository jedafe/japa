package media

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
)

// testImgproxyConfig returns a deterministic config so tests do not depend
// on env vars or the system_settings resolver.
func testImgproxyConfig(t *testing.T) mediaURLConfig {
	t.Helper()
	kb, err := hex.DecodeString("736563726574") // "secret"
	if err != nil {
		t.Fatalf("bad test key hex: %v", err)
	}
	sb, err := hex.DecodeString("68656c6c6f") // "hello"
	if err != nil {
		t.Fatalf("bad test salt hex: %v", err)
	}
	return mediaURLConfig{
		keyBytes:  kb,
		saltBytes: sb,
		baseURL:   "https://img.example.com",
		valid:     true,
	}
}

// expectedSig recomputes the imgproxy signature the same way imgproxy
// verifies it: base64url(HMAC-SHA256(key, salt || path)).
func expectedSig(t *testing.T, cfg mediaURLConfig, path string) string {
	t.Helper()
	mac := hmac.New(sha256.New, cfg.keyBytes)
	mac.Write(append(cfg.saltBytes, []byte(path)...))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func TestMediaURLWithConfig_SignedURL(t *testing.T) {
	cfg := testImgproxyConfig(t)
	got := mediaURLWithConfig("avatars", "u/42.jpg", ImageProxyOpts{
		ResizingType: "fill",
		Width:        300,
		Height:       300,
		Format:       "webp",
	}, cfg)

	srcB64 := base64.RawURLEncoding.EncodeToString([]byte("s3://avatars/u/42.jpg"))
	wantPath := "/rs:fill:300:300/" + srcB64 + ".webp"
	want := cfg.baseURL + "/" + expectedSig(t, cfg, wantPath) + wantPath

	if got != want {
		t.Fatalf("signed URL mismatch:\n got: %s\nwant: %s", got, want)
	}
	if !strings.HasPrefix(got, cfg.baseURL+"/") {
		t.Fatalf("URL missing base prefix: %s", got)
	}
}

func TestMediaURLWithConfig_OptionDefaults(t *testing.T) {
	cfg := testImgproxyConfig(t)
	cases := []struct {
		name     string
		opts     ImageProxyOpts
		wantOpts string
	}{
		{"all empty", ImageProxyOpts{}, "/rs:auto:640:0/"},
		{"size only", ImageProxyOpts{Width: 200, Height: 100}, "/rs:fill:200:100/"},
		{"format passthrough", ImageProxyOpts{Width: 64, Height: 64, Format: "png"}, "/rs:fill:64:64/"},
	}
	for _, tc := range cases {
		got := mediaURLWithConfig("b", "p.png", tc.opts, cfg)
		if !strings.Contains(got, tc.wantOpts) {
			t.Fatalf("%s: URL %q missing %q", tc.name, got, tc.wantOpts)
		}
	}
	// Height 0 flips resizing type to auto; explicit format applies.
	got := mediaURLWithConfig("b", "p.png", ImageProxyOpts{Width: 200, Height: 0, Format: "jpg"}, cfg)
	if !strings.Contains(got, "/rs:auto:200:0/") || !strings.HasSuffix(got, ".jpg") {
		t.Fatalf("auto/jpg URL wrong: %s", got)
	}
}

func TestMediaURLWithConfig_Fallback(t *testing.T) {
	// Unsigned/invalid config or empty base URL must fall back to the local
	// serve endpoint — never sign with an empty key (Section 34).
	for _, cfg := range []mediaURLConfig{
		{valid: false, baseURL: "https://img.example.com", keyBytes: []byte("k"), saltBytes: []byte("s")},
		{valid: true, baseURL: "", keyBytes: []byte("k"), saltBytes: []byte("s")},
	} {
		got := mediaURLWithConfig("avatars", "u/42.jpg", ImageProxyOpts{}, cfg)
		if got != "/api/v1/media/serve/avatars/u/42.jpg" {
			t.Fatalf("expected local serve fallback, got %s", got)
		}
	}
}

func TestMediaURLWithConfig_SignatureSensitivity(t *testing.T) {
	cfg := testImgproxyConfig(t)
	other := cfg
	other.keyBytes = []byte("different-key")

	a := mediaURLWithConfig("b", "p.jpg", ImageProxyOpts{Width: 100, Height: 100}, cfg)
	b := mediaURLWithConfig("b", "p.jpg", ImageProxyOpts{Width: 100, Height: 100}, other)
	if a == b {
		t.Fatal("different keys produced identical signatures")
	}
}
