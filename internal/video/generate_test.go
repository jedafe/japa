package video

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
)

func TestBuildSummaryPrompt(t *testing.T) {
	t.Parallel()

	cat := "Travel Tips"
	src := gen.VideoSource{
		Title:       "Tokyo Subway Guide",
		Description: "A short guide to the Tokyo Metro.",
		Category:    &cat,
		SourceUrl:   "https://youtube.com/watch?v=abc",
		FieldValues: []byte(`{"transcript":"Take the Yamanote line. Transfer at Shibuya.","duration_secs":120}`),
	}

	svc := &Service{}
	prompt := svc.buildSummaryPrompt(src)

	for _, want := range []string{
		"Tokyo Subway Guide",
		"Tokyo Metro",
		"Travel Tips",
		"120",
		"Take the Yamanote line",
		"Return a JSON object",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

func TestParseGeneratedSummary_JSON(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	src := gen.VideoSource{Title: "Original", Description: "Original body", SourceUrl: "https://x"}

	reply := `{"title":"AI Title","body":"First paragraph.\n\nSecond paragraph."}`
	title, body := svc.parseGeneratedSummary(reply, src)

	if title != "AI Title" {
		t.Errorf("title = %q, want %q", title, "AI Title")
	}

	if !strings.Contains(body, "First paragraph") || !strings.Contains(body, "Second paragraph") {
		t.Errorf("body = %q", body)
	}
}

func TestParseGeneratedSummary_CodeBlock(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	src := gen.VideoSource{Title: "Original", Description: "Original body", SourceUrl: "https://x"}

	reply := "```json\n{\"title\":\"Block Title\",\"body\":\"Block body\"}\n```"
	title, body := svc.parseGeneratedSummary(reply, src)

	if title != "Block Title" {
		t.Errorf("title = %q, want %q", title, "Block Title")
	}

	if body != "Block body" {
		t.Errorf("body = %q, want %q", body, "Block body")
	}
}

func TestParseGeneratedSummary_Narrative(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	src := gen.VideoSource{Title: "Original", Description: "Original body", SourceUrl: "https://x"}

	reply := "Here is the draft.\n\n{\"title\":\"Narrative Title\",\"body\":\"Narrative body.\"}"
	title, body := svc.parseGeneratedSummary(reply, src)

	if title != "Narrative Title" {
		t.Errorf("title = %q", title)
	}

	if body != "Narrative body." {
		t.Errorf("body = %q", body)
	}
}

func TestBuildDraftBody(t *testing.T) {
	t.Parallel()

	svc := &Service{}

	tipTap, htmlBody, err := svc.buildDraftBody("First paragraph.\n\nSecond paragraph.")
	if err != nil {
		t.Fatalf("buildDraftBody: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal([]byte(tipTap), &doc); err != nil {
		t.Fatalf("tipTap is not valid JSON: %v", err)
	}

	if doc["type"] != "doc" {
		t.Errorf("tipTap type = %v", doc["type"])
	}

	content, ok := doc["content"].([]any)
	if !ok || len(content) != 2 {
		t.Fatalf("expected 2 content nodes, got %v", doc["content"])
	}

	if !strings.Contains(htmlBody, "<p>First paragraph.</p>") {
		t.Errorf("htmlBody missing first paragraph: %q", htmlBody)
	}

	if !strings.Contains(htmlBody, "<p>Second paragraph.</p>") {
		t.Errorf("htmlBody missing second paragraph: %q", htmlBody)
	}
}

func TestExtractTrailingJSONObject(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		want  string
	}{
		{
			input: `Here is JSON: {"title":"a","body":"b"}`,
			want:  `{"title":"a","body":"b"}`,
		},
		{
			input: `{"title":"a"}`,
			want:  `{"title":"a"}`,
		},
		{
			input: "plain text only",
			want:  "",
		},
	}

	for _, tc := range cases {
		got := extractTrailingJSONObject(tc.input)
		if got != tc.want {
			t.Errorf("extractTrailingJSONObject(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

type fakeSettingsForGenerate struct{}

func (f *fakeSettingsForGenerate) GetStringOr(_ context.Context, _ pgtype.UUID, _, fallback string) string {
	return fallback
}

func (f *fakeSettingsForGenerate) GetBoolOr(_ context.Context, _ pgtype.UUID, key string, fallback bool) bool {
	if key == "video.enabled" || key == "video.ai_summary.enabled" {
		return true
	}

	return fallback
}

func (f *fakeSettingsForGenerate) GetIntOr(_ context.Context, _ pgtype.UUID, _ string, fallback int) int {
	return fallback
}

func TestEnqueueGenerateSummary_Payload(t *testing.T) {
	t.Parallel()

	enq := &fakeEnqueuer{}
	svc := NewService(nil, nil, nil, nil)
	svc.SetEnqueuer(enq)
	svc.SetSettings(&fakeSettingsForGenerate{})
	_ = pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	userID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	sourceID := pgtype.UUID{Bytes: [16]byte{3}, Valid: true}

	err := svc.EnqueueGenerateSummary(t.Context(), userID, sourceID)
	if err != nil {
		t.Fatalf("EnqueueGenerateSummary: %v", err)
	}

	if len(enq.recorded) != 1 {
		t.Fatalf("expected 1 enqueued task, got %d", len(enq.recorded))
	}

	if enq.recorded[0].typeString != "video:generate_summary" {
		t.Errorf("task type = %q", enq.recorded[0].typeString)
	}
}
