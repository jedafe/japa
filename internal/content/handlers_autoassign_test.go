package content

import (
	"testing"
)

func keywordSet(keywords []string) map[string]struct{} {
	m := make(map[string]struct{}, len(keywords))
	for _, k := range keywords {
		m[k] = struct{}{}
	}
	return m
}

func TestExtractTitleKeywords(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		title          string
		wantPresent    []string
		wantNotPresent []string
	}{
		{
			name:           "multi_word_university_name",
			title:          "University of Lagos",
			wantPresent:    []string{"lagos", "university of", "of lagos", "university of lagos"},
			wantNotPresent: []string{"university", "of"},
		},
		{
			name:           "university_initials_in_title",
			title:          "Study at UNILAG in Lagos",
			wantPresent:    []string{"unilag", "lagos", "unilag in lagos"},
			wantNotPresent: []string{"at", "in"},
		},
		{
			name:        "multi_word_geo_keyword",
			title:       "New York",
			wantPresent: []string{"new", "york", "new york"},
		},
		{
			name:           "punctuation_normalised",
			title:          "Moving to Lagos, Nigeria",
			wantPresent:    []string{"lagos", "nigeria", "lagos nigeria"},
			wantNotPresent: []string{"to"},
		},
		{
			name:           "hyphenated_name",
			title:          "New-York City",
			wantPresent:    []string{"new", "york", "new york", "york city", "new york city"},
			wantNotPresent: []string{"city"},
		},
		{
			name:           "single_letter_skipped",
			title:          "I am in Lagos",
			wantPresent:    []string{"lagos"},
			wantNotPresent: []string{"i", "am"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTitleKeywords(tt.title)
			gotSet := keywordSet(got)

			for _, want := range tt.wantPresent {
				if _, ok := gotSet[want]; !ok {
					t.Errorf("expected n-gram %q to be present in %v", want, got)
				}
			}

			for _, notWant := range tt.wantNotPresent {
				if _, ok := gotSet[notWant]; ok {
					t.Errorf("expected n-gram %q to be excluded, got %v", notWant, got)
				}
			}
		})
	}
}

func TestExtractTitleKeywordsDedupes(t *testing.T) {
	t.Parallel()

	got := extractTitleKeywords("Lagos")
	want := []string{"lagos"}

	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}
