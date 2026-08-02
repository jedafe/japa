package middleware

import "testing"

func TestIsLoopbackIdentifier(t *testing.T) {
	tests := []struct {
		name       string
		identifier string
		want       bool
	}{
		{"ipv4 loopback", "ip:127.0.0.1", true},
		{"ipv4 loopback alt", "ip:127.0.1.5", true},
		{"ipv6 loopback", "ip:::1", true},
		{"public ipv4", "ip:203.0.113.10", false},
		{"public ipv6", "ip:2606:4700::1", false},
		{"authenticated user", "user:123e4567-e89b-12d3-a456-426614174000", false},
		{"empty", "", false},
		{"garbage", "ip:not-an-ip", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isLoopbackIdentifier(tt.identifier); got != tt.want {
				t.Fatalf("isLoopbackIdentifier(%q) = %v, want %v", tt.identifier, got, tt.want)
			}
		})
	}
}
