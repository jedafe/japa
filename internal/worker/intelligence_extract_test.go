package worker

import (
	"testing"

	"github.com/jedafe/japa/internal/db/gen"
)

// TestResolveSourceEntityTypes covers the mixed-domain source contract:
// provider_config.entity_types overrides the category-derived type, invalid
// values are filtered out, and the category fallback preserves the v1
// single-type behaviour.
func TestResolveSourceEntityTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		category       string
		providerConfig string
		want           []string
	}{
		{
			name:           "no override falls back to category",
			category:       "scholarship",
			providerConfig: `{}`,
			want:           []string{"scholarship"},
		},
		{
			name:           "empty provider_config falls back to category",
			category:       "job",
			providerConfig: "",
			want:           []string{"job"},
		},
		{
			name:           "override yields all declared types",
			category:       "scholarship",
			providerConfig: `{"entity_types":["scholarship","job"]}`,
			want:           []string{"scholarship", "job"},
		},
		{
			name:           "override ignores category entirely",
			category:       "job",
			providerConfig: `{"entity_types":["news","visa"]}`,
			want:           []string{"news", "visa"},
		},
		{
			name:           "unregistered override values are dropped",
			category:       "scholarship",
			providerConfig: `{"entity_types":["scholarship","bogus_domain","job"]}`,
			want:           []string{"scholarship", "job"},
		},
		{
			name:           "all-invalid override falls back to category",
			category:       "remittance",
			providerConfig: `{"entity_types":["bogus_domain"]}`,
			want:           []string{"remittance"},
		},
		{
			name:           "empty override array falls back to category",
			category:       "news",
			providerConfig: `{"entity_types":[]}`,
			want:           []string{"news"},
		},
		{
			name:           "duplicate override values are deduped",
			category:       "news",
			providerConfig: `{"entity_types":["job","job","visa"]}`,
			want:           []string{"job", "visa"},
		},
		{
			name:           "override preserves sibling provider_config keys",
			category:       "scholarship",
			providerConfig: `{"endpoint":"https://api.example.com/x","entity_types":["job"]}`,
			want:           []string{"job"},
		},
		{
			name:           "unsupported category without override is empty",
			category:       "unmapped_category",
			providerConfig: `{}`,
			want:           nil,
		},
		{
			name:           "malformed provider_config falls back to category",
			category:       "news",
			providerConfig: `[not json`,
			want:           []string{"news"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			src := gen.IntelSource{
				Category:       tt.category,
				ProviderConfig: []byte(tt.providerConfig),
			}

			got := resolveSourceEntityTypes(src)

			if len(got) != len(tt.want) {
				t.Fatalf("resolveSourceEntityTypes = %v, want %v", got, tt.want)
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("resolveSourceEntityTypes[%d] = %q, want %q (got %v)", i, got[i], tt.want[i], got)
				}
			}
		})
	}
}
