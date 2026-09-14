package config

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TestCachedSettingJSONRoundTrip guards the Valkey settings cache: the entry
// must survive a JSON marshal/unmarshal with its value intact. A struct with
// unexported fields marshals to "{}", which silently turned every cross-process
// (post-restart) cache read into an empty string.
func TestCachedSettingJSONRoundTrip(t *testing.T) {
	t.Parallel()

	orig := cachedSetting{
		Value:     `{"default":"reddit","by_type":{"blog_post":"grid"}}`,
		CreatedAt: time.Now().Truncate(time.Second),
		Missing:   true,
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if string(data) == "{}" {
		t.Fatalf("cachedSetting serialized to %q — fields are not being marshaled", data)
	}

	var back cachedSetting
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if back.Value != orig.Value {
		t.Errorf("Value not preserved through JSON: got %q, want %q", back.Value, orig.Value)
	}

	if !back.CreatedAt.Equal(orig.CreatedAt) {
		t.Errorf("CreatedAt not preserved: got %v, want %v", back.CreatedAt, orig.CreatedAt)
	}

	if back.Missing != orig.Missing {
		t.Errorf("Missing not preserved through JSON: got %v, want %v", back.Missing, orig.Missing)
	}
}

// TestGetSettingCachesMissing verifies that a missing-setting sentinel is
// returned from the in-memory cache without hitting the database again.
func TestGetSettingCachesMissing(t *testing.T) {
	t.Parallel()

	s := &Settings{cache: make(map[string]cachedSetting)}

	var tid pgtype.UUID
	_ = tid.Scan("11111111-1111-1111-1111-111111111111")

	cacheKey := "missing_key"
	s.cache[cacheKey] = cachedSetting{Value: "", CreatedAt: time.Now(), Missing: true}

	_, err := s.GetSetting(t.Context(), tid, "missing_key")
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected pgx.ErrNoRows from cached missing setting, got %v", err)
	}

	if got := s.GetIntOr(t.Context(), tid, "missing_key", 42); got != 42 {
		t.Errorf("GetIntOr fallback not returned for missing setting: got %d, want 42", got)
	}

	if got := s.GetBoolOr(t.Context(), "missing_key", true); !got {
		t.Errorf("GetBoolOr fallback not returned for missing setting: got %v, want true", got)
	}

	if got := s.GetStringOr(t.Context(), tid, "missing_key", "fallback"); got != "fallback" {
		t.Errorf("GetStringOr fallback not returned for missing setting: got %q, want %q", got, "fallback")
	}
}
