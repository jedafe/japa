package feed

import (
	"reflect"
	"testing"
)

func TestResolveCardStyleConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want CardStyleConfig
	}{
		{
			name: "empty -> default",
			raw:  "",
			want: CardStyleConfig{Default: "reddit", ByType: map[string]string{}},
		},
		{
			name: "legacy scalar",
			raw:  `"magazine"`,
			want: CardStyleConfig{Default: "magazine", ByType: map[string]string{}},
		},
		{
			name: "legacy invalid scalar coerced to reddit",
			raw:  `"devto"`,
			want: CardStyleConfig{Default: "reddit", ByType: map[string]string{}},
		},
		{
			name: "object with per-type overrides",
			raw:  `{"default":"reddit","by_type":{"blog_post":"grid","discussion":"reddit"}}`,
			want: CardStyleConfig{Default: "reddit", ByType: map[string]string{"blog_post": "grid", "discussion": "reddit"}},
		},
		{
			name: "object invalid default coerced",
			raw:  `{"default":"bogus","by_type":{}}`,
			want: CardStyleConfig{Default: "reddit", ByType: map[string]string{}},
		},
		{
			name: "object drops invalid per-type entry",
			raw:  `{"default":"compact","by_type":{"blog_post":"grid","discussion":"bogus"}}`,
			want: CardStyleConfig{Default: "compact", ByType: map[string]string{"blog_post": "grid"}},
		},
		{
			name: "object missing by_type",
			raw:  `{"default":"grid"}`,
			want: CardStyleConfig{Default: "grid", ByType: map[string]string{}},
		},
		{
			name: "unparseable -> default",
			raw:  `not json`,
			want: CardStyleConfig{Default: "reddit", ByType: map[string]string{}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := resolveCardStyleConfig(tc.raw)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("resolveCardStyleConfig(%q) = %+v, want %+v", tc.raw, got, tc.want)
			}
		})
	}
}
