package media

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// isPublicIP — SSRF guard
// ---------------------------------------------------------------------------

func TestIsPublicIP(t *testing.T) {
	t.Parallel()

	cases := []struct {
		ip    string
		want  bool
		label string
	}{
		{"8.8.8.8", true, "public v4"},
		{"1.1.1.1", true, "public v4"},
		{"2606:4700:4700::1111", true, "public v6"},
		{"127.0.0.1", false, "loopback v4"},
		{"::1", false, "loopback v6"},
		{"10.0.0.5", false, "RFC1918 10/8"},
		{"172.16.0.1", false, "RFC1918 172.16/12"},
		{"192.168.1.1", false, "RFC1918 192.168/16"},
		{"fd00::1", false, "IPv6 ULA"},
		{"169.254.169.254", false, "link-local cloud metadata"},
		{"169.254.1.1", false, "link-local v4"},
		{"fe80::1", false, "link-local v6"},
		{"100.64.0.1", false, "CGNAT start"},
		{"100.127.255.254", false, "CGNAT end"},
		{"100.128.0.1", true, "just above CGNAT"},
		{"198.18.0.1", false, "benchmark range"},
		{"192.0.0.9", false, "IETF protocol assignments"},
		{"192.0.2.1", false, "TEST-NET-1"},
		{"240.0.0.1", false, "reserved 240/4"},
		{"255.255.255.255", false, "broadcast"},
		{"0.0.0.0", false, "unspecified v4"},
		{"::", false, "unspecified v6"},
		{"224.0.0.1", false, "multicast v4"},
		{"ff02::1", false, "multicast v6"},
	}

	for _, tc := range cases {
		ip := net.ParseIP(tc.ip)
		if ip == nil {
			t.Fatalf("bad test ip %q", tc.ip)
		}
		if got := isPublicIP(ip); got != tc.want {
			t.Errorf("isPublicIP(%s) [%s] = %v, want %v", tc.ip, tc.label, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// StoreFromURL — SSRF + URL validation
// ---------------------------------------------------------------------------

func TestStoreFromURLRejectsBadTargets(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	var uid pgtype.UUID
	_ = uid.Scan("00000000-0000-0000-0000-000000000001")

	cases := []struct {
		url   string
		label string
	}{
		{"file:///etc/passwd", "file scheme"},
		{"gopher://example.com/x", "gopher scheme"},
		{"not-a-url", "no scheme"},
		{"http://", "missing host"},
		{"http://user:pass@example.com/x", "userinfo"},
		{"http://127.0.0.1/x", "loopback literal"},
		{"http://169.254.169.254/latest/meta-data", "cloud metadata"},
		{"http://[::1]/x", "v6 loopback"},
		{"http://10.0.0.1/internal", "private literal"},
		{"http://100.64.0.1/x", "CGNAT literal"},
		{"http://localhost/x", "localhost resolves loopback"},
	}

	for _, tc := range cases {
		// These must all fail before a successful fetch — validation or
		// the guarded dial. None require network access: literal IPs are
		// checked without resolution, and localhost resolves via hosts.
		if _, err := svc.StoreFromURL(context.Background(), uid, tc.url, "f.bin"); err == nil {
			t.Errorf("StoreFromURL(%q) [%s] succeeded, want rejection", tc.url, tc.label)
		}
	}
}

// ---------------------------------------------------------------------------
// readLimited — oversized bodies must error, not truncate
// ---------------------------------------------------------------------------

func TestReadLimited(t *testing.T) {
	t.Parallel()

	if got, err := readLimited(strings.NewReader("hello"), 5); err != nil || string(got) != "hello" {
		t.Errorf("exact-limit read = %q, %v", got, err)
	}
	if got, err := readLimited(strings.NewReader("hello"), 10); err != nil || string(got) != "hello" {
		t.Errorf("under-limit read = %q, %v", got, err)
	}
	if _, err := readLimited(strings.NewReader("hello!"), 5); err == nil {
		t.Error("over-limit read succeeded, want error (no silent truncation)")
	}
}
