package sources

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestResolveCategoryMap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		settings     string
		categories   []string
		wantEntities []string
		wantMethods  []string
		wantPipeline Pipeline
		wantTopics   []string
		wantWarnings []string // substrings; each must appear in some warning
	}{
		{
			name:         "single extract category",
			categories:   []string{"job"},
			wantEntities: []string{"job"},
			wantMethods:  []string{"jsonld", "api", "tabular", "render", "static"},
			wantPipeline: PipelineExtract,
			wantTopics:   []string{"job", "hiring", "vacancy", "career", "remote work", "recruitment"},
		},
		{
			name:         "single curate category",
			categories:   []string{"news"},
			wantEntities: []string{"news"},
			wantMethods:  []string{"rss", "jsonld", "render", "static"},
			wantPipeline: PipelineCurate,
			wantTopics:   []string{"migration", "immigration", "japa", "diaspora", "visa", "relocation", "travel"},
		},
		{
			name:       "any extract forces extract pipeline",
			categories: []string{"news", "job"},
			// news first: entity_types and methods union in first-seen order.
			wantEntities: []string{"news", "job"},
			wantMethods:  []string{"rss", "jsonld", "render", "static", "api", "tabular"},
			wantPipeline: PipelineExtract,
			wantTopics: []string{"migration", "immigration", "japa", "diaspora", "visa", "relocation", "travel",
				"job", "hiring", "vacancy", "career", "remote work", "recruitment"},
		},
		{
			name:         "extract first preserves its ordering",
			categories:   []string{"job", "news"},
			wantEntities: []string{"job", "news"},
			wantMethods:  []string{"jsonld", "api", "tabular", "render", "static", "rss"},
			wantPipeline: PipelineExtract,
			wantTopics: []string{"job", "hiring", "vacancy", "career", "remote work", "recruitment",
				"migration", "immigration", "japa", "diaspora", "visa", "relocation", "travel"},
		},
		{
			name:       "topic union dedupes shared vocabulary",
			categories: []string{"news", "blog"},
			// news and blog share japa + diaspora — appear once, first-seen.
			wantEntities: []string{"news", "article"},
			wantMethods:  []string{"rss", "jsonld", "render", "static"},
			wantPipeline: PipelineCurate,
			wantTopics: []string{"migration", "immigration", "japa", "diaspora", "visa", "relocation", "travel",
				"study abroad", "settlement", "expat"},
		},
		{
			name:         "all curate keeps first-seen pipeline",
			categories:   []string{"event", "dataset"},
			wantEntities: []string{"event", "dataset"},
			wantMethods:  []string{"ical", "jsonld", "tabular", "git", "api"},
			wantPipeline: PipelineCurate,
			wantTopics: []string{"webinar", "fair", "embassy", "orientation", "deadline",
				"exchange rate", "processing time", "visa fee", "tuition"},
		},
		{
			name:       "video resolves faithfully for caller routing",
			categories: []string{"video"},
			// channel_sync is not an intel crawl method; resolver returns
			// the entry verbatim and callers route it to the §106 flow.
			wantEntities: nil,
			wantMethods:  []string{"channel_sync"},
			wantPipeline: PipelineCurate,
			wantTopics:   []string{"japa", "relocation", "study abroad", "visa"},
		},
		{
			name:         "unknown category skipped with warning",
			categories:   []string{"news", "bogus"},
			wantEntities: []string{"news"},
			wantMethods:  []string{"rss", "jsonld", "render", "static"},
			wantPipeline: PipelineCurate,
			wantTopics:   []string{"migration", "immigration", "japa", "diaspora", "visa", "relocation", "travel"},
			wantWarnings: []string{`unknown category "bogus"`},
		},
		{
			name:         "all unknown resolves empty",
			categories:   []string{"bogus", "also-bogus"},
			wantEntities: nil,
			wantMethods:  nil,
			wantPipeline: "",
			wantTopics:   nil,
			wantWarnings: []string{`unknown category "bogus"`, `unknown category "also-bogus"`},
		},
		{
			name:         "empty selection resolves empty",
			categories:   nil,
			wantEntities: nil,
			wantMethods:  nil,
			wantPipeline: "",
			wantTopics:   nil,
		},
		{
			name:         "blank and duplicate categories skipped",
			categories:   []string{"", "  ", "news", "news"},
			wantEntities: []string{"news"},
			wantMethods:  []string{"rss", "jsonld", "render", "static"},
			wantPipeline: PipelineCurate,
			wantTopics:   []string{"migration", "immigration", "japa", "diaspora", "visa", "relocation", "travel"},
		},
		{
			name:         "nil settings uses embedded default",
			settings:     "",
			categories:   []string{"visa"},
			wantEntities: []string{"visa"},
			wantMethods:  []string{"static", "pdf", "render"},
			wantPipeline: PipelineExtract,
			wantTopics:   []string{"visa", "immigration", "embassy", "residence permit", "asylum", "border"},
		},
		{
			name:     "custom settings override default",
			settings: `{"news":{"entity_types":["article"],"methods":["search"],"pipeline":"hybrid","topics":["custom"]}}`,
			categories: []string{"news", "job"},
			// Only news exists in the custom map — job warns; hybrid is the
			// first-seen pipeline with no extract present.
			wantEntities: []string{"article"},
			wantMethods:  []string{"search"},
			wantPipeline: PipelineHybrid,
			wantTopics:   []string{"custom"},
			wantWarnings: []string{`unknown category "job"`},
		},
		{
			name:         "malformed settings falls back to default with warning",
			settings:     `[broken`,
			categories:   []string{"event"},
			wantEntities: []string{"event"},
			wantMethods:  []string{"ical", "jsonld"},
			wantPipeline: PipelineCurate,
			wantTopics:   []string{"webinar", "fair", "embassy", "orientation", "deadline"},
			wantWarnings: []string{"unparseable"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ResolveCategoryMap([]byte(tt.settings), tt.categories)

			if !slices.Equal(got.EntityTypes, tt.wantEntities) {
				t.Errorf("EntityTypes = %v, want %v", got.EntityTypes, tt.wantEntities)
			}
			if !slices.Equal(got.Methods, tt.wantMethods) {
				t.Errorf("Methods = %v, want %v", got.Methods, tt.wantMethods)
			}
			if got.Pipeline != tt.wantPipeline {
				t.Errorf("Pipeline = %q, want %q", got.Pipeline, tt.wantPipeline)
			}
			if !slices.Equal(got.Topics, tt.wantTopics) {
				t.Errorf("Topics = %v, want %v", got.Topics, tt.wantTopics)
			}
			for _, want := range tt.wantWarnings {
				found := false
				for _, w := range got.Warnings {
					if strings.Contains(w, want) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Warnings = %v, want one containing %q", got.Warnings, want)
				}
			}
			if len(tt.wantWarnings) == 0 && len(got.Warnings) != 0 {
				t.Errorf("Warnings = %v, want none", got.Warnings)
			}
		})
	}
}

func TestDefaultCategoryMap(t *testing.T) {
	t.Parallel()

	raw := DefaultCategoryMap()
	if len(raw) == 0 {
		t.Fatal("DefaultCategoryMap returned empty")
	}

	catMap, err := ParseCategoryMap(raw)
	if err != nil {
		t.Fatalf("embedded map does not parse: %v", err)
	}

	// Spec §2 categories — the embedded map is the canonical copy.
	want := []string{"job", "news", "blog", "event", "scholarship", "visa", "dataset", "video"}
	for _, cat := range want {
		if _, ok := catMap[cat]; !ok {
			t.Errorf("embedded map missing category %q", cat)
		}
	}
	if len(catMap) != len(want) {
		t.Errorf("embedded map has %d categories, want %d", len(catMap), len(want))
	}

	// Spot-check invariants the resolver relies on.
	if catMap["job"].Pipeline != PipelineExtract {
		t.Errorf("job pipeline = %q, want extract", catMap["job"].Pipeline)
	}
	if len(catMap["video"].EntityTypes) != 0 {
		t.Errorf("video entity_types = %v, want empty", catMap["video"].EntityTypes)
	}
	for cat, m := range catMap {
		if !m.Pipeline.IsValid() {
			t.Errorf("category %q pipeline %q fails Pipeline.IsValid", cat, m.Pipeline)
		}
	}
}

func TestParseCategoryMap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		wantLen int
		wantErr bool
	}{
		{"empty", ``, 0, false},
		{"empty object", `{}`, 0, false},
		{"one entry", `{"job":{"entity_types":["job"],"methods":["static"],"pipeline":"extract","topics":[]}}`, 1, false},
		{"malformed", `[broken`, 0, true},
		{"wrong shape", `["job"]`, 0, true},
		{"entry wrong shape", `{"job":"extract"}`, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseCategoryMap([]byte(tt.raw))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseCategoryMap(%q) expected error", tt.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseCategoryMap(%q) unexpected error: %v", tt.raw, err)
			}
			if len(got) != tt.wantLen {
				t.Fatalf("ParseCategoryMap(%q) len = %d, want %d", tt.raw, len(got), tt.wantLen)
			}
		})
	}
}

func TestCategoryMapJSONIsObject(t *testing.T) {
	t.Parallel()

	// Guard: the embedded file must stay a JSON object — a stray edit that
	// turns it into e.g. a scalar would silently resolve every category as
	// unknown.
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(defaultCategoryMapJSON, &doc); err != nil {
		t.Fatalf("category_map.json is not a JSON object: %v", err)
	}
}
