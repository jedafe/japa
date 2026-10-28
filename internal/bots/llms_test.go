package bots

import (
	"strings"
	"testing"
)

func TestGenerateLLMsTxt_Basic(t *testing.T) {
	t.Parallel()

	txt := GenerateLLMsTxt("JapaConnect", "A community platform for the Nigerian diaspora", LLMsBriefData{})

	if !strings.Contains(txt, "# JapaConnect") {
		t.Error("expected site name as header")
	}

	if !strings.Contains(txt, "> A community platform for the Nigerian diaspora") {
		t.Error("expected site description")
	}
}

func TestGenerateLLMsTxt_WithLinks(t *testing.T) {
	t.Parallel()

	data := LLMsBriefData{
		TopLinks: []Link{
			{Title: "Getting Started", URL: "https://japa.to/docs/getting-started"},
			{Title: "API Reference", URL: "https://japa.to/docs/api"},
		},
		RecentPosts: []Link{
			{Title: "Moving to Canada", URL: "https://japa.to/blog/moving-to-canada"},
		},
		Optional: []Link{
			{Title: "Extended content index", URL: "https://japa.to/llms-full.txt"},
		},
	}

	txt := GenerateLLMsTxt("JapaConnect", "Test description", data)

	if !strings.Contains(txt, "## Top Links") {
		t.Error("expected Top Links section")
	}

	if !strings.Contains(txt, "- [Getting Started](https://japa.to/docs/getting-started)") {
		t.Error("expected first link")
	}

	if !strings.Contains(txt, "- [API Reference](https://japa.to/docs/api)") {
		t.Error("expected second link")
	}

	if !strings.Contains(txt, "## Recent Posts") {
		t.Error("expected Recent Posts section")
	}

	if !strings.Contains(txt, "- [Moving to Canada](https://japa.to/blog/moving-to-canada)") {
		t.Error("expected recent post link")
	}

	if !strings.Contains(txt, "## Optional") {
		t.Error("expected Optional section")
	}

	if !strings.Contains(txt, "- [Extended content index](https://japa.to/llms-full.txt)") {
		t.Error("expected llms-full.txt cross-link")
	}
}

func TestGenerateLLMsTxt_NoLinks(t *testing.T) {
	t.Parallel()

	txt := GenerateLLMsTxt("Test", "Desc", LLMsBriefData{})

	if strings.Contains(txt, "## Top Links") {
		t.Error("should not have Top Links section when no links provided")
	}

	if strings.Contains(txt, "## Recent Posts") {
		t.Error("should not have Recent Posts section when no posts provided")
	}
}

func TestGenerateLLMsTxt_SanitizesTitles(t *testing.T) {
	t.Parallel()

	data := LLMsBriefData{
		TopLinks: []Link{{Title: "Evil]\nDisallow: /", URL: "https://japa.to/x"}},
	}

	txt := GenerateLLMsTxt("Test", "Desc", data)

	if !strings.Contains(txt, "- [Evil) Disallow: /](https://japa.to/x)") {
		t.Errorf("expected sanitized inline title, got:\n%s", txt)
	}
}

func TestGenerateLLMsFullTxt_Basic(t *testing.T) {
	t.Parallel()

	txt := GenerateLLMsFullTxt("JapaConnect", "Test site", LLMsData{})

	if !strings.Contains(txt, "# JapaConnect") {
		t.Error("expected site name as header")
	}

	if !strings.Contains(txt, "> Test site") {
		t.Error("expected site description")
	}
}

func TestGenerateLLMsFullTxt_WithRecentPosts(t *testing.T) {
	t.Parallel()

	data := LLMsData{
		RecentPosts: []Link{
			{Title: "Moving to Canada", URL: "https://japa.to/blog/moving-to-canada"},
			{Title: "UK Visa Guide", URL: "https://japa.to/blog/uk-visa-guide"},
		},
	}

	txt := GenerateLLMsFullTxt("Test", "Desc", data)

	if !strings.Contains(txt, "## Recent Posts") {
		t.Error("expected Recent Posts section")
	}

	if !strings.Contains(txt, "- [Moving to Canada](https://japa.to/blog/moving-to-canada)") {
		t.Error("expected first post")
	}

	if !strings.Contains(txt, "- [UK Visa Guide](https://japa.to/blog/uk-visa-guide)") {
		t.Error("expected second post")
	}
}

func TestGenerateLLMsFullTxt_WithTags(t *testing.T) {
	t.Parallel()

	data := LLMsData{
		Tags: []Link{
			{Title: "Canada Immigration", URL: "https://japa.to/tags/canada-immigration"},
		},
	}

	txt := GenerateLLMsFullTxt("Test", "Desc", data)

	if !strings.Contains(txt, "## Topics") {
		t.Error("expected Topics section")
	}

	if !strings.Contains(txt, "- [Canada Immigration](https://japa.to/tags/canada-immigration)") {
		t.Error("expected tag link")
	}
}

func TestGenerateLLMsFullTxt_WithSpaces(t *testing.T) {
	t.Parallel()

	data := LLMsData{
		Spaces: []Link{
			{Title: "Toronto Newcomers", URL: "https://japa.to/spaces/toronto-newcomers"},
		},
	}

	txt := GenerateLLMsFullTxt("Test", "Desc", data)

	if !strings.Contains(txt, "## Spaces") {
		t.Error("expected Spaces section")
	}

	if !strings.Contains(txt, "- [Toronto Newcomers](https://japa.to/spaces/toronto-newcomers)") {
		t.Error("expected space link")
	}
}

func TestGenerateLLMsFullTxt_AllSections(t *testing.T) {
	t.Parallel()

	data := LLMsData{
		RecentPosts: []Link{{Title: "Post 1", URL: "/post1"}},
		Tags:        []Link{{Title: "Tag 1", URL: "/tag1"}},
		Spaces:      []Link{{Title: "Space 1", URL: "/space1"}},
		Optional:    []Link{{Title: "RSS feed", URL: "https://japa.to/feed.xml"}},
	}

	txt := GenerateLLMsFullTxt("Test", "Desc", data)

	for _, want := range []string{"## Recent Posts", "## Topics", "## Spaces", "## Optional"} {
		if !strings.Contains(txt, want) {
			t.Errorf("expected %s section", want)
		}
	}
}

func TestGenerateLLMsFullTxt_EmptyData(t *testing.T) {
	t.Parallel()

	txt := GenerateLLMsFullTxt("Test", "Desc", LLMsData{})

	for _, unwanted := range []string{"## Recent Posts", "## Topics", "## Spaces", "## Optional"} {
		if strings.Contains(txt, unwanted) {
			t.Errorf("should not have %s section when empty", unwanted)
		}
	}
}

func TestAbsolutizeLinks(t *testing.T) {
	t.Parallel()

	links := []Link{
		{Title: "Rel", URL: "/blog/x"},
		{Title: "Abs", URL: "https://academy.japa.to/tracks/y"},
	}

	got := absolutizeLinks(links, "https://japa.to")

	if got[0].URL != "https://japa.to/blog/x" {
		t.Errorf("relative URL not absolutized: %s", got[0].URL)
	}

	if got[1].URL != "https://academy.japa.to/tracks/y" {
		t.Errorf("absolute URL should be untouched: %s", got[1].URL)
	}
}

func TestContentURLPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		contentType string
		slug        string
		want        string
	}{
		{"blog_post", "my-post", "/blog/my-post"},
		{"forum_thread", "my-thread", "/forum/my-thread"},
		{"guide", "my-guide", "/guide/my-guide"},
		{"discussion", "my-discussion", "/forum/my-discussion"},
		{"comparison", "my-comparison", "/comparisons/my-comparison"},
		{"story", "my-story", "/story/my-story"},
		{"job_listing", "my-job", "/jobs/my-job"},
		{"service_listing", "my-svc", "/services/my-svc"},
		{"video", "my-video", "/video/my-video"},
		{"unknown_type", "my-content", "/content/my-content"},
		{"", "no-type", "/content/no-type"},
	}

	for _, tt := range tests {
		got := ContentURLPath(tt.contentType, tt.slug)
		if got != tt.want {
			t.Errorf("contentURLPath(%q, %q) = %q, want %q", tt.contentType, tt.slug, got, tt.want)
		}
	}
}
