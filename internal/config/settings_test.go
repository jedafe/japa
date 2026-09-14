package config

import (
	"testing"
)

// TestParseBoolRoundTrip verifies parseBool handles all JSONB storage formats
// that system_settings values can take: JSON bool, JSON string, bare token.
func TestParseBoolRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"json true literal", `true`, true},
		{"json false literal", `false`, false},
		{"json string true", `"true"`, true},
		{"json string 1", `"1"`, true},
		{"json string false", `"false"`, false},
		{"json string 0", `"0"`, false},
		{"bare word true", `true`, true},
		{"bare word false", `false`, false},
		{"bare digit 1", `1`, true},
		{"bare digit 0", `0`, false},
		{"whitespace", ` "true" `, true},
		{"empty string", `""`, false},
		{"garbage", `not-a-bool`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := parseBool(tt.input)
			if got != tt.want {
				t.Errorf("parseBool(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestParseIntRoundTrip verifies parseInt handles JSON numbers, JSON strings,
// and bare integer tokens.
func TestParseIntRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{"json number", `5000`, 5000, false},
		{"json zero", `0`, 0, false},
		{"json negative", `-1`, -1, false},
		{"json large", `86400`, 86400, false},
		{"json string number", `"5000"`, 5000, false},
		{"json string padded", `" 120 "`, 120, false},
		{"bare number", `5000`, 5000, false},
		{"bare padded", ` 120 `, 120, false},
		{"empty string", `""`, 0, true},
		{"garbage", `not-a-number`, 0, true},
		{"float", `3.14`, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseInt(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseInt(%q) should return error", tt.input)
				}

				return
			}

			if err != nil {
				t.Errorf("parseInt(%q) unexpected error: %v", tt.input, err)
				return
			}

			if got != tt.want {
				t.Errorf("parseInt(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestSettingsCacheInvalidation verifies InvalidateSettings clears the
// in-memory cache for a specific tenant.
func TestSettingsCacheInvalidation(t *testing.T) {
	t.Parallel()

	s := &Settings{cache: make(map[string]cachedSetting)}

	s.cache["test_key"] = cachedSetting{Value: `true`}
	s.InvalidateSettings(t.Context())

	if _, ok := s.cache["test_key"]; ok {
		t.Error("invalidate should have removed the cached entry")
	}
}

// TestSettingsCloseStopsCleanupGoroutine verifies that Close stops the
// background cleanup goroutine started by NewSettings. Without Close, the
// goroutine leaks on every NewSettings call (BUG-001).
func TestSettingsCloseStopsCleanupGoroutine(t *testing.T) {
	t.Parallel()

	s := &Settings{
		cache:  make(map[string]cachedSetting),
		stopCh: make(chan struct{}),
	}
	// Simulate the cleanup loop exiting when stopCh is closed.
	go func() {
		<-s.stopCh
	}()
	s.Close()

	// Calling Close again should be a no-op (not panic).
	s.Close()

	// Verify stopCh is closed (non-blocking read succeeds).
	select {
	case <-s.stopCh:
		// expected — channel is closed
	default:
		t.Error("Close should have closed stopCh")
	}
}

// TestSettingsCloseNilStopCh verifies Close is safe on a Settings created
// without NewSettings (stopCh == nil), as done in unit tests.
func TestSettingsCloseNilStopCh(t *testing.T) {
	t.Parallel()

	s := &Settings{cache: make(map[string]cachedSetting)}
	s.Close() // must not panic
}
