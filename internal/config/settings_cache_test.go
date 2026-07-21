package config

import (
	"encoding/json"
	"testing"
	"time"
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
}
