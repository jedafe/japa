package linkpreview

import (
	"net"
	"testing"
)

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", true},
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"192.168.1.1", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"::1", true},
		{"2001:4860:4860::8888", false},
	}
	for _, tt := range tests {
		ip := net.ParseIP(tt.ip)
		if ip == nil {
			t.Errorf("failed to parse IP: %s", tt.ip)
			continue
		}
		got := IsPrivateIP(ip)
		if got != tt.want {
			t.Errorf("IsPrivateIP(%s) = %v, want %v", tt.ip, got, tt.want)
		}
	}
}

func TestValidateURL(t *testing.T) {
	tests := []struct {
		url     string
		wantErr bool
	}{
		{"https://example.com", false},
		{"http://example.com", false},
		{"ftp://example.com", true},
		{"file:///etc/passwd", true},
		{"http://127.0.0.1", true},
		{"http://10.0.0.1", true},
		{"http://192.168.1.1", true},
		{"", true},
	}
	for _, tt := range tests {
		err := ValidateURL(tt.url)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
		}
	}
}

func TestIsBlockedDomain(t *testing.T) {
	blacklist := []string{"evil.com", "bad.org"}

	tests := []struct {
		host string
		want bool
	}{
		{"evil.com", true},
		{"sub.evil.com", true},
		{"good.com", false},
		{"example.org", false},
		{"bad.org", true},
	}
	for _, tt := range tests {
		got := IsBlockedDomain(tt.host, blacklist)
		if got != tt.want {
			t.Errorf("IsBlockedDomain(%q) = %v, want %v", tt.host, got, tt.want)
		}
	}
}
