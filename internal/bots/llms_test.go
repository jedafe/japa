package bots

import (
	"strings"
	"testing"
)

func TestGenerateLLMsTxt_Basic(t *testing.T) {
	txt := GenerateLLMsTxt("JapaConnect", "A community platform for the Nigerian diaspora", nil)

	if !strings.Contains(txt, "# JapaConnect") {
		t.Error("expected site name as header")
	}
	if !strings.Contains(txt, "> A community platform for the Nigerian diaspora") {
		t.Error("expected site description")
	}
}

func TestGenerateLLMsTxt_WithLinks(t *testing.T) {
	links := []Link{
		{Title: "Getting Started", URL: "/docs/getting-started"},
		{Title: "API Reference", URL: "/docs/api"},
	}

	txt := GenerateLLMsTxt("JapaConnect", "Test description", links)

	if !strings.Contains(txt, "## Top Links") {
		t.Error("expected Top Links section")
	}
	if !strings.Contains(txt, "- [Getting Started](/docs/getting-started)") {
		t.Error("expected first link")
	}
	if !strings.Contains(txt, "- [API Reference](/docs/api)") {
		t.Error("expected second link")
	}
}

func TestGenerateLLMsTxt_NoLinks(t *testing.T) {
	txt := GenerateLLMsTxt("Test", "Desc", nil)

	if strings.Contains(txt, "## Top Links") {
		t.Error("should not have Top Links section when no links provided")
	}
}

func TestGenerateLLMsFullTxt_Basic(t *testing.T) {
	txt := GenerateLLMsFullTxt("JapaConnect", "Test site", LLMsData{})

	if !strings.Contains(txt, "# JapaConnect") {
		t.Error("expected site name as header")
	}
	if !strings.Contains(txt, "> Test site") {
		t.Error("expected site description")
	}
}

func TestGenerateLLMsFullTxt_WithRecentPosts(t *testing.T) {
	data := LLMsData{
		RecentPosts: []Link{
			{Title: "Moving to Canada", URL: "/blog/moving-to-canada"},
			{Title: "UK Visa Guide", URL: "/blog/uk-visa-guide"},
		},
	}

	txt := GenerateLLMsFullTxt("Test", "Desc", data)

	if !strings.Contains(txt, "## Recent Posts") {
		t.Error("expected Recent Posts section")
	}
	if !strings.Contains(txt, "- [Moving to Canada](/blog/moving-to-canada)") {
		t.Error("expected first post")
	}
	if !strings.Contains(txt, "- [UK Visa Guide](/blog/uk-visa-guide)") {
		t.Error("expected second post")
	}
}

func TestGenerateLLMsFullTxt_WithTags(t *testing.T) {
	data := LLMsData{
		Tags: []Link{
			{Title: "Canada Immigration", URL: "/tags/canada-immigration"},
		},
	}

	txt := GenerateLLMsFullTxt("Test", "Desc", data)

	if !strings.Contains(txt, "## Topics") {
		t.Error("expected Topics section")
	}
	if !strings.Contains(txt, "- [Canada Immigration](/tags/canada-immigration)") {
		t.Error("expected tag link")
	}
}

func TestGenerateLLMsFullTxt_WithSpaces(t *testing.T) {
	data := LLMsData{
		Spaces: []Link{
			{Title: "Toronto Newcomers", URL: "/spaces/toronto-newcomers"},
		},
	}

	txt := GenerateLLMsFullTxt("Test", "Desc", data)

	if !strings.Contains(txt, "## Spaces") {
		t.Error("expected Spaces section")
	}
	if !strings.Contains(txt, "- [Toronto Newcomers](/spaces/toronto-newcomers)") {
		t.Error("expected space link")
	}
}

func TestGenerateLLMsFullTxt_AllSections(t *testing.T) {
	data := LLMsData{
		RecentPosts: []Link{{Title: "Post 1", URL: "/post1"}},
		Tags:        []Link{{Title: "Tag 1", URL: "/tag1"}},
		Spaces:      []Link{{Title: "Space 1", URL: "/space1"}},
	}

	txt := GenerateLLMsFullTxt("Test", "Desc", data)

	if !strings.Contains(txt, "## Recent Posts") {
		t.Error("expected Recent Posts section")
	}
	if !strings.Contains(txt, "## Topics") {
		t.Error("expected Topics section")
	}
	if !strings.Contains(txt, "## Spaces") {
		t.Error("expected Spaces section")
	}
}

func TestGenerateLLMsFullTxt_EmptyData(t *testing.T) {
	txt := GenerateLLMsFullTxt("Test", "Desc", LLMsData{})

	if strings.Contains(txt, "## Recent Posts") {
		t.Error("should not have Recent Posts section when empty")
	}
	if strings.Contains(txt, "## Topics") {
		t.Error("should not have Topics section when empty")
	}
	if strings.Contains(txt, "## Spaces") {
		t.Error("should not have Spaces section when empty")
	}
}
