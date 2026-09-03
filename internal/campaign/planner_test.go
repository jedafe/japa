// Package campaign — planner_test.go validates the AI planning helpers and
// DefaultPlanner dispatch/parsing behaviour.
package campaign

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jedafe/japa/internal/ai"
)

// fakeAIDispatcher implements the AIDispatcher interface for tests.
type fakeAIDispatcher struct {
	resp   ai.Response
	err    error
	last   ai.Request
	lastCategory string
}

func (f *fakeAIDispatcher) Dispatch(ctx context.Context, category string, req ai.Request) (ai.Response, error) {
	f.lastCategory = category
	f.last = req
	return f.resp, f.err
}

func TestExtractAIText(t *testing.T) {
	tests := []struct {
		name    string
		raw     map[string]any
		want    string
		wantOK  bool
	}{
		{
			name:    "text field",
			raw:     map[string]any{"text": "hello"},
			want:    "hello",
			wantOK:  true,
		},
		{
			name:    "OpenAI choices",
			raw:     map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": "json here"}}}},
			want:    "json here",
			wantOK:  true,
		},
		{
			name:    "empty",
			raw:     map[string]any{},
			want:    "",
			wantOK:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := extractAIText(tt.raw)
			if ok != tt.wantOK || got != tt.want {
				t.Fatalf("extractAIText(%+v) = %q, %v; want %q, %v", tt.raw, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestUnmarshalJSONFromText(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		wantErr bool
	}{
		{
			name: "direct JSON",
			text: `{"personas":[{"name":"A","slug":"a","voice_prompt":"v"}]}`,
		},
		{
			name: "wrapped in markdown",
			text: "Some text\n```json\n{\"personas\":[{\"name\":\"A\",\"slug\":\"a\",\"voice_prompt\":\"v\"}]}\n```",
		},
		{
			name:    "no JSON",
			text:    "just plain text",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var wrapper struct {
				Personas []PersonaSpec `json:"personas"`
			}
			err := unmarshalJSONFromText(tt.text, &wrapper)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(wrapper.Personas) == 0 || wrapper.Personas[0].Name != "A" {
				t.Fatalf("unexpected parsed result: %+v", wrapper.Personas)
			}
		})
	}
}

func TestPlanPersonas_PromptRequiresJSON(t *testing.T) {
	fake := &fakeAIDispatcher{
		resp: ai.Response{Raw: map[string]any{"text": `{"personas":[{"name":"Writer A","slug":"writer-a","voice_prompt":"friendly"}]}`}},
	}
	planner := NewDefaultPlanner(fake)

	specs, err := planner.PlanPersonas(context.Background(), "investment brief", 1, []string{"tone"})
	if err != nil {
		t.Fatalf("PlanPersonas failed: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected 1 persona, got %d", len(specs))
	}
	if specs[0].Name != "Writer A" {
		t.Fatalf("unexpected persona name: %s", specs[0].Name)
	}
}

func TestPlanPersonas_RequiresMinPersonas(t *testing.T) {
	fake := &fakeAIDispatcher{
		resp: ai.Response{Raw: map[string]any{"text": `{"personas":[{"name":"One","slug":"one","voice_prompt":"v"}]}`}},
	}
	planner := NewDefaultPlanner(fake)

	_, err := planner.PlanPersonas(context.Background(), "brief", 3, nil)
	if err == nil || !strings.Contains(err.Error(), "returned 1 personas, expected at least 3") {
		t.Fatalf("expected under-supply error, got: %v", err)
	}
}

func TestPlanContent_PromptForbidsPersonaReuse(t *testing.T) {
	fake := &fakeAIDispatcher{
		resp: ai.Response{Raw: map[string]any{"text": `{"items":[{"persona_id":"p1","content_type":"blog","title":"T1","topic_prompt":"Write about X"},{"persona_id":"p2","content_type":"story","title":"T2","topic_prompt":"Write about Y"}]}`}},
	}
	planner := NewDefaultPlanner(fake)

	pool := []personaSummary{
		{ID: "p1", Name: "A", VoicePrompt: "v1"},
		{ID: "p2", Name: "B", VoicePrompt: "v2"},
	}
	plan, err := planner.PlanContent(context.Background(), "investment brief", []string{"blog", "story"}, pool, 2, nil, "", "")
	if err != nil {
		t.Fatalf("PlanContent failed: %v", err)
	}
	if len(plan.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(plan.Items))
	}

	// The prompt must explicitly instruct the model not to reuse personas and
	// should mention the exact requested count.
	prompt := fake.last.Payload["prompt"].(string)
	if !strings.Contains(prompt, "assign a different persona to every item") {
		t.Fatalf("prompt missing persona uniqueness instruction: %s", prompt)
	}
	if !strings.Contains(prompt, "produce exactly 2 unique content items") {
		t.Fatalf("prompt missing requested count: %s", prompt)
	}
}

func TestPlanContent_DispatchError(t *testing.T) {
	fake := &fakeAIDispatcher{err: errors.New("gateway unavailable")}
	planner := NewDefaultPlanner(fake)

	_, err := planner.PlanContent(context.Background(), "brief", []string{"blog"}, []personaSummary{{ID: "p1"}}, 1, nil, "", "")
	if err == nil || !strings.Contains(err.Error(), "dispatch failed") {
		t.Fatalf("expected dispatch error, got: %v", err)
	}
}

func TestPlanContent_RequestedCountUsed(t *testing.T) {
	fake := &fakeAIDispatcher{
		resp: ai.Response{Raw: map[string]any{"text": `{"items":[{"persona_id":"p1","content_type":"blog","title":"T1","topic_prompt":"X"}]}`}},
	}
	planner := NewDefaultPlanner(fake)

	pool := []personaSummary{
		{ID: "p1", Name: "A", VoicePrompt: "v1"},
		{ID: "p2", Name: "B", VoicePrompt: "v2"},
		{ID: "p3", Name: "C", VoicePrompt: "v3"},
	}
	plan, err := planner.PlanContent(context.Background(), "investment brief", []string{"blog"}, pool, 1, nil, "", "")
	if err != nil {
		t.Fatalf("PlanContent failed: %v", err)
	}
	if len(plan.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(plan.Items))
	}

	prompt := fake.last.Payload["prompt"].(string)
	if !strings.Contains(prompt, "produce exactly 1 unique content items") {
		t.Fatalf("prompt should use requested count 1, got: %s", prompt)
	}
	if !strings.Contains(prompt, "choose the best-fitting 1 distinct personas") {
		t.Fatalf("prompt should tell AI to choose 1 persona from pool, got: %s", prompt)
	}
}

func TestPlanContent_RequestedCountExceedsPool(t *testing.T) {
	fake := &fakeAIDispatcher{}
	planner := NewDefaultPlanner(fake)

	pool := []personaSummary{{ID: "p1"}}
	_, err := planner.PlanContent(context.Background(), "brief", []string{"blog"}, pool, 3, nil, "", "")
	if err == nil || !strings.Contains(err.Error(), "exceeds persona pool size") {
		t.Fatalf("expected pool size error, got: %v", err)
	}
}

func TestPlanContent_RequestedCountZero(t *testing.T) {
	fake := &fakeAIDispatcher{}
	planner := NewDefaultPlanner(fake)

	pool := []personaSummary{{ID: "p1"}}
	_, err := planner.PlanContent(context.Background(), "brief", []string{"blog"}, pool, 0, nil, "", "")
	if err == nil || !strings.Contains(err.Error(), "requested count must be greater than 0") {
		t.Fatalf("expected positive count error, got: %v", err)
	}
}
