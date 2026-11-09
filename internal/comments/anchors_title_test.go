package comments

import "testing"

func TestAnchorThreadTitle(t *testing.T) {
	t.Parallel()
	cases := []struct {
		entityType, title, want string
	}{
		{"content", "My Post", "My Post (A Thread)"},
		{"wiki_page", "Argentina Cost of Living", "Argentina Cost of Living (A Thread)"},
		{"tool", "Visa Routes", "Visa Routes Tool (A Thread)"},
	}
	for _, tc := range cases {
		if got := anchorThreadTitle(tc.entityType, tc.title); got != tc.want {
			t.Errorf("anchorThreadTitle(%q, %q) = %q, want %q", tc.entityType, tc.title, got, tc.want)
		}
	}
}

func TestPageDisplayTitle(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, space, title, want string
	}{
		{"prepends space", "Argentina", "Cost of Living", "Argentina Cost of Living"},
		{"already prefixed", "Finland", "Finland Overview", "Finland Overview"},
		{"name mid-title", "United Kingdom", "United Kingdom Visa Routes", "United Kingdom Visa Routes"},
		{"name mid-title only", "United Kingdom", "Visa Routes for United Kingdom", "Visa Routes for United Kingdom"},
		{"word boundary — substring is not a match", "Lima", "Climate", "Lima Climate"},
		{"exact-name title kept", "Argentina", "Argentina", "Argentina"},
		{"case-insensitive contains", "Argentina", "argentina overview", "argentina overview"},
		{"empty space name", "", "Cost of Living", "Cost of Living"},
		{"empty title", "Argentina", "", ""},
		{"trims whitespace", "Argentina", "  Cost of Living  ", "Argentina Cost of Living"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pageDisplayTitle(tc.space, tc.title); got != tc.want {
				t.Errorf("pageDisplayTitle(%q, %q) = %q, want %q", tc.space, tc.title, got, tc.want)
			}
		})
	}
}

func TestIsGeneratedAnchorTitle(t *testing.T) {
	t.Parallel()
	generated := []string{
		"My Post (A Thread)",
		"Visa Routes Tool (A Thread)",
		"Discussing Finland Overview",
		"Discussing: My Post",
		"Thread about Cost of Living",
		"Argentina Overview — community thread",
		"Visa Routes — Q&A thread",
		"Boston — local thread",
	}
	for _, title := range generated {
		if !isGeneratedAnchorTitle(title) {
			t.Errorf("isGeneratedAnchorTitle(%q) = false, want true", title)
		}
	}
	authored := []string{
		"My visa journey to Spain",
		"What nobody tells you about moving",
		"",
	}
	for _, title := range authored {
		if isGeneratedAnchorTitle(title) {
			t.Errorf("isGeneratedAnchorTitle(%q) = true, want false", title)
		}
	}
}
