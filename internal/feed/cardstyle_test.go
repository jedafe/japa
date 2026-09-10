package feed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/jedafe/japa/internal/tenant"
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
			want: CardStyleConfig{Default: DefaultCardStyle, ByType: map[string]string{}},
		},
		{
			name: "legacy scalar",
			raw:  `"magazine"`,
			want: CardStyleConfig{Default: "magazine", ByType: map[string]string{}},
		},
		{
			name: "legacy invalid scalar coerced to default",
			raw:  `"devto"`,
			want: CardStyleConfig{Default: DefaultCardStyle, ByType: map[string]string{}},
		},
		{
			name: "object with per-type overrides",
			raw:  `{"default":"reddit","by_type":{"blog_post":"grid","discussion":"reddit"}}`,
			want: CardStyleConfig{Default: "reddit", ByType: map[string]string{"blog_post": "grid", "discussion": "reddit"}},
		},
		{
			name: "object invalid default coerced",
			raw:  `{"default":"bogus","by_type":{}}`,
			want: CardStyleConfig{Default: DefaultCardStyle, ByType: map[string]string{}},
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
			want: CardStyleConfig{Default: DefaultCardStyle, ByType: map[string]string{}},
		},
		{
			// The control UI + settings PATCH store every value as a JSONB
			// string, so an object saved from the editor is double-encoded:
			// the stored value is a JSON string whose content is object JSON.
			name: "double-encoded object (control UI save path)",
			raw:  `"{\"default\":\"reddit\",\"by_type\":{\"blog_post\":\"grid\"}}"`,
			want: CardStyleConfig{Default: "reddit", ByType: map[string]string{"blog_post": "grid"}},
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

func TestGetCardStyleDefaultShape(t *testing.T) {
	t.Parallel()

	// settings is nil, so the handler reads no stored value and must return
	// the normalized default config. This locks the response field names.
	h := &Handlers{}

	req := httptest.NewRequest(http.MethodGet, "/feed/card-style", nil)
	ctx := context.WithValue(req.Context(), tenant.TenantContextKey, &tenant.Tenant{})
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	h.GetCardStyle(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	var body struct {
		Data CardStyleConfig `json:"data"`
	}

	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if body.Data.Default != DefaultCardStyle {
		t.Errorf("data.default = %q, want %q", body.Data.Default, DefaultCardStyle)
	}

	if body.Data.ByType == nil || len(body.Data.ByType) != 0 {
		t.Errorf("data.by_type = %v, want empty non-nil map", body.Data.ByType)
	}
}
