// Package campaign — planner.go implements the AI planning layer.
//
// The Planner produces structured JSON for persona provisioning and content
// planning via the AI Gateway. The service validates the planner output before
// persisting it.
package campaign

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/jedafe/japa/internal/ai"
)

// AIDispatcher is the subset of the AI Gateway the planner needs.
// *ai.Dispatcher satisfies this.
type AIDispatcher interface {
	Dispatch(ctx context.Context, category string, req ai.Request) (ai.Response, error)
}

// Planner produces the persona and content plans for a campaign.
// The concrete DefaultPlanner calls the AI Gateway; the service accepts any
// Planner implementation so tests can use a mock.
type Planner interface {
	PlanPersonas(ctx context.Context, userBrief string, deficit int, profileTypeDefaults []string, voiceDiversityHints []string) ([]PersonaSpec, error)
	PlanContent(ctx context.Context, userBrief string, allowedContentTypes []string, finalPersonaPool []personaSummary, requestedCount int, voiceSummaries map[string]string, targetSpace, targetTags string) (CampaignPlanResult, error)
}

// DefaultPlanner calls the AI Gateway with deterministic, structured prompts.
type DefaultPlanner struct {
	dispatcher AIDispatcher
}

// NewDefaultPlanner creates a planner backed by the given AI dispatcher.
func NewDefaultPlanner(d AIDispatcher) *DefaultPlanner {
	return &DefaultPlanner{dispatcher: d}
}

// personaSummary is the minimal persona information sent to the content planner.
type personaSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	VoicePrompt string `json:"voice_prompt"`
}

// Ensure DefaultPlanner implements the Planner interface.
var _ Planner = (*DefaultPlanner)(nil)

// PlanPersonas asks the AI Gateway to generate `deficit` distinct new personas.
// The prompt is deterministic and explicitly requests a JSON object with a
// "personas" array. Each returned spec must have name, slug, and voice_prompt.
func (p *DefaultPlanner) PlanPersonas(ctx context.Context, userBrief string, deficit int, profileTypeDefaults []string, voiceDiversityHints []string) ([]PersonaSpec, error) {
	if p.dispatcher == nil {
		return nil, errors.New("campaign planner: no AI dispatcher configured")
	}
	if deficit <= 0 {
		return nil, nil
	}

	profileDefaults := ""
	if len(profileTypeDefaults) > 0 {
		profileDefaults = strings.Join(profileTypeDefaults, ", ")
	}
	diversity := ""
	if len(voiceDiversityHints) > 0 {
		diversity = strings.Join(voiceDiversityHints, ", ")
	}

	prompt := fmt.Sprintf(`You are an AI persona architect for a content campaign.

User brief: %q

The campaign needs %d new distinct AI personas. These personas will each author one unique piece of content for the same campaign.

The tenant's default professional profile types are: %s
Use these as fallback profile type slugs if you do not specify one explicitly.

Desired voice/role diversity: %s

Each persona must have a unique name, a URL-safe slug, a short tagline, and a voice prompt that defines how they write and what perspective they bring. You may optionally include an activity_policy object.

Return ONLY a JSON object in this exact format:
{
  "personas": [
    {
      "name": "...",
      "slug": "...",
      "tagline": "...",
      "voice_prompt": "...",
      "activity_policy": {...}
    }
  ]
}

Do not include markdown, explanations, or any text outside the JSON.`, userBrief, deficit, profileDefaults, diversity)

	resp, err := p.dispatcher.Dispatch(ctx, "content_generation", ai.Request{
		Category: "content_generation",
		Payload: map[string]any{
			"prompt": prompt,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("campaign: PlanPersonas dispatch failed: %w", err)
	}

	text, ok := extractAIText(resp.Raw)
	if !ok || text == "" {
		return nil, errors.New("campaign: PlanPersonas returned no text")
	}

	var wrapper struct {
		Personas []PersonaSpec `json:"personas"`
	}
	if err := unmarshalJSONFromText(text, &wrapper); err != nil {
		return nil, fmt.Errorf("campaign: PlanPersonas response is not valid JSON: %w", err)
	}

	if len(wrapper.Personas) < deficit {
		return nil, fmt.Errorf("campaign: PlanPersonas returned %d personas, expected at least %d", len(wrapper.Personas), deficit)
	}

	seen := make(map[string]struct{}, len(wrapper.Personas))
	for i, ps := range wrapper.Personas {
		if ps.Name == "" || ps.Slug == "" || ps.VoicePrompt == "" {
			return nil, fmt.Errorf("campaign: persona %d is missing name, slug, or voice_prompt", i)
		}
		if _, dup := seen[ps.Slug]; dup {
			return nil, fmt.Errorf("campaign: duplicate persona slug %q in plan", ps.Slug)
		}
		seen[ps.Slug] = struct{}{}
	}

	return wrapper.Personas, nil
}

// PlanContent asks the AI Gateway to assign one distinct persona to every
// campaign item and produce a content plan. The prompt explicitly instructs
// "assign a different persona to every item" and requests a JSON object with
// an "items" array.
func (p *DefaultPlanner) PlanContent(ctx context.Context, userBrief string, allowedContentTypes []string, finalPersonaPool []personaSummary, requestedCount int, voiceSummaries map[string]string, targetSpace, targetTags string) (CampaignPlanResult, error) {
	if p.dispatcher == nil {
		return CampaignPlanResult{}, errors.New("campaign planner: no AI dispatcher configured")
	}

	allowed := ""
	if len(allowedContentTypes) > 0 {
		allowed = strings.Join(allowedContentTypes, ", ")
	}

	poolJSON, err := json.Marshal(finalPersonaPool)
	if err != nil {
		return CampaignPlanResult{}, fmt.Errorf("campaign: marshal persona pool: %w", err)
	}

	targetInfo := ""
	if targetSpace != "" {
		targetInfo += fmt.Sprintf("\nTarget geo/audience space: %s", targetSpace)
	}
	if targetTags != "" {
		targetInfo += fmt.Sprintf("\nTarget tags: %s", targetTags)
	}

	if requestedCount <= 0 {
		return CampaignPlanResult{}, errors.New("campaign: requested count must be greater than 0")
	}
	if requestedCount > len(finalPersonaPool) {
		return CampaignPlanResult{}, fmt.Errorf("campaign: requested count %d exceeds persona pool size %d", requestedCount, len(finalPersonaPool))
	}
	count := requestedCount
	prompt := fmt.Sprintf(`You are a content campaign planner for a multi-persona AI content studio.

User brief: %q

The campaign must produce exactly %d unique content items.

Allowed content types: %s
If the brief does not specify a content type, choose from this list for each item.

You must assign a different persona to every item. Never reuse a persona within the same campaign.
If the available pool contains more personas than the requested count, choose the best-fitting %d distinct personas from the pool.

Available persona pool (each row is one eligible persona):
%s

%s

Each item must include:
- persona_id: the exact UUID from the pool
- content_type: one of the allowed content types
- title: a short, specific title
- topic_prompt: a detailed prompt the AI writer will use to generate the draft
- target_space_id (optional): a UUID if the item should target a specific space
- target_tags (optional): an array of tag strings

Return ONLY a JSON object in this exact format:
{
  "items": [
    {
      "persona_id": "...",
      "content_type": "...",
      "title": "...",
      "topic_prompt": "...",
      "target_space_id": "...",
      "target_tags": ["..."]
    }
  ]
}

Do not include markdown, explanations, or any text outside the JSON.`, userBrief, count, allowed, count, string(poolJSON), targetInfo)

	resp, err := p.dispatcher.Dispatch(ctx, "content_generation", ai.Request{
		Category: "content_generation",
		Payload: map[string]any{
			"prompt": prompt,
		},
	})
	if err != nil {
		return CampaignPlanResult{}, fmt.Errorf("campaign: PlanContent dispatch failed: %w", err)
	}

	text, ok := extractAIText(resp.Raw)
	if !ok || text == "" {
		return CampaignPlanResult{}, errors.New("campaign: PlanContent returned no text")
	}

	var result CampaignPlanResult
	if err := unmarshalJSONFromText(text, &result); err != nil {
		return CampaignPlanResult{}, fmt.Errorf("campaign: PlanContent response is not valid JSON: %w", err)
	}

	return result, nil
}

// extractAIText pulls generated text from an AI gateway response. Different
// providers return different field names.
func extractAIText(raw map[string]any) (string, bool) {
	for _, key := range []string{"text", "content", "output", "generated_text", "response"} {
		if v, ok := raw[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s, true
			}
		}
	}

	// OpenAI-compatible providers return choices[0].message.content.
	if choices, ok := raw["choices"].([]any); ok && len(choices) > 0 {
		if first, ok := choices[0].(map[string]any); ok {
			if msg, ok := first["message"].(map[string]any); ok {
				if content, ok := msg["content"].(string); ok && content != "" {
					return content, true
				}
			}
		}
	}
	return "", false
}

// unmarshalJSONFromText extracts a JSON object/array from a possibly noisy
// text response and unmarshals it into v.
func unmarshalJSONFromText(text string, v any) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("empty text")
	}

	// If the text is already a JSON object/array, try it directly.
	if (strings.HasPrefix(text, "{") && strings.HasSuffix(text, "}")) ||
		(strings.HasPrefix(text, "[") && strings.HasSuffix(text, "]")) {
		if err := json.Unmarshal([]byte(text), v); err == nil {
			return nil
		}
	}

	// Otherwise, find the first JSON object/array embedded in the text.
	start := strings.Index(text, "{")
	if start == -1 {
		start = strings.Index(text, "[")
	}
	end := strings.LastIndex(text, "}")
	if end == -1 {
		end = strings.LastIndex(text, "]")
	}
	if start >= 0 && end > start {
		return json.Unmarshal([]byte(text[start:end+1]), v)
	}
	return errors.New("no JSON object or array found")
}

// sanitizeSlug returns a URL-safe slug from a name or slug candidate.
// It is used when the planner returns slugs that need normalisation.
func sanitizeSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

// sortedPersonaIDs returns the IDs of a persona pool in deterministic order.
func sortedPersonaIDs(pool []personaSummary) []string {
	ids := make([]string, 0, len(pool))
	for _, p := range pool {
		ids = append(ids, p.ID)
	}
	sort.Strings(ids)
	return ids
}
