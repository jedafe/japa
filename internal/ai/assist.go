// Package ai — assist.go is the shared in-editor assist implementation
// (§20 V1 tool catalog) behind both POST /ai/assist/* (HTTP adapters in
// handlers.go) and the agent executor tools re-enabled by migration
// 000847 (tools_assist.go). One code path, two adapters — the tool
// catalog names (content.improve et al.) and the REST endpoints dispatch
// identical gateway requests.
//
// Ref: japaconnect-engineering/sections/20-ai-core.md (V1 Tool Catalog)
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Assist tools)
package ai

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// AssistRunner is the shared assist surface. *Handlers satisfies it; the
// agent's ToolDeps closes over the interface (ToolDeps.Assist) so the
// executor tools and REST handlers run the same dispatch path.
type AssistRunner interface {
	AssistImprove(ctx context.Context, userID pgtype.UUID, in AssistImproveInput) (Response, error)
	AssistSummarize(ctx context.Context, userID pgtype.UUID, in AssistSummarizeInput) (Response, error)
	AssistTranslate(ctx context.Context, userID pgtype.UUID, in AssistTranslateInput) (Response, error)
	AssistGenerateTags(ctx context.Context, userID pgtype.UUID, in AssistTagsInput) (Response, error)
	AssistGenerateOG(ctx context.Context, userID pgtype.UUID, in AssistOGInput) (Response, error)
	AssistDraft(ctx context.Context, userID pgtype.UUID, in DraftInput) (DraftResult, error)
}

// AssistError is a typed validation failure from an assist op. Status and
// Code map onto the standard API error envelope; tool handlers map it to
// an error ToolResult.
type AssistError struct {
	Status  int
	Code    string
	Message string
}

func (e *AssistError) Error() string { return e.Message }

func assistValidation(message string) *AssistError {
	return &AssistError{Status: http.StatusBadRequest, Code: "validation_error", Message: message}
}

// assistCall is the normalized dispatch description every assist op
// reduces to: a prompt, the usage-log task type, model/voice overrides,
// and the humanize merge inputs.
type assistCall struct {
	prompt         string
	usageTask      string         // ai_usage_log task_type
	payloadExtra   map[string]any // provider payload metadata (task, tone, ...)
	model          string         // per-request model override
	voiceID        string         // per-request persona voice
	humanize       *bool          // per-request humanize override
	humanizeLevel  string         // per-request humanize level override
	responseFormat map[string]any // e.g. {"type":"json_object"} for draft
	maxTokens      int            // e.g. 4096 for draft
}

// The shared "content_generation" gateway category + default-model setting
// key for all assist ops — a single platform default powers every content
// assistant surface.
const assistCategory = "content_generation"
const assistDefaultModelKey = "ai.model.default.content_generation"

// AssistImproveInput — POST /ai/assist/improve and the content.improve tool.
type AssistImproveInput struct {
	Text          string `json:"text"`
	Tone          string `json:"tone,omitempty"`
	Humanize      *bool  `json:"humanize,omitempty"`
	HumanizeLevel string `json:"humanize_level,omitempty"`
	VoiceID       string `json:"voice_id,omitempty"`
	Model         string `json:"model,omitempty"`
}

// AssistSummarizeInput — POST /ai/assist/summarize and the content.summarize tool.
type AssistSummarizeInput struct {
	Text          string `json:"text"`
	MaxWords      int    `json:"max_words,omitempty"`
	Humanize      *bool  `json:"humanize,omitempty"`
	HumanizeLevel string `json:"humanize_level,omitempty"`
	VoiceID       string `json:"voice_id,omitempty"`
	Model         string `json:"model,omitempty"`
}

// AssistTranslateInput — POST /ai/assist/translate and the platform.translate_string tool.
type AssistTranslateInput struct {
	Text          string `json:"text"`
	TargetLocale  string `json:"target_locale"`
	Humanize      *bool  `json:"humanize,omitempty"`
	HumanizeLevel string `json:"humanize_level,omitempty"`
	VoiceID       string `json:"voice_id,omitempty"`
	Model         string `json:"model,omitempty"`
}

// AssistTagsInput — POST /ai/assist/generate-tags and the content.suggest_tags tool.
type AssistTagsInput struct {
	Title   string `json:"title,omitempty"`
	Text    string `json:"text,omitempty"`
	VoiceID string `json:"voice_id,omitempty"`
	Model   string `json:"model,omitempty"`
}

// AssistOGInput — POST /ai/assist/generate-og and the content.generate_og tool.
type AssistOGInput struct {
	Title       string `json:"title"`
	Text        string `json:"text,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	VoiceID     string `json:"voice_id,omitempty"`
	Model       string `json:"model,omitempty"`
}

// runAssist dispatches one assist op through the gateway: payload build →
// model/voice resolution → humanize merge → dispatchAndRecordUsage.
// userID is both the usage-log owner and the voice-visibility caller.
func (h *Handlers) runAssist(ctx context.Context, userID pgtype.UUID, c assistCall) (Response, error) {
	payload := map[string]any{
		"prompt":  c.prompt,
		"user_id": userID.String(),
	}

	for k, v := range c.payloadExtra {
		payload[k] = v
	}

	if c.responseFormat != nil {
		payload["response_format"] = c.responseFormat
	}

	if c.maxTokens > 0 {
		payload["max_tokens"] = c.maxTokens
	}

	if err := h.resolveModelAndVoice(ctx, userID, payload, c.model, c.voiceID, assistCategory, assistDefaultModelKey); err != nil {
		return Response{}, err
	}

	// Humanization: merge global ai.humanize.* setting with per-request override.
	hEnabled, hLevel := h.resolveHandlerHumanize(ctx, c.humanize, c.humanizeLevel)
	writeHumanizePayload(payload, hEnabled, hLevel)

	return h.dispatchAndRecordUsage(ctx, userID, c.usageTask, assistCategory, Request{
		Category: assistCategory,
		Payload:  payload,
	})
}

// resolveModelAndVoice resolves and validates a per-request model/voice
// override and mutates payload. Returns a typed *AssistError on validation
// failure so both HTTP and tool callers can map it. callerID is the user
// whose voice visibility applies (BUG-009: invisible voices no-op instead
// of erroring).
func (h *Handlers) resolveModelAndVoice(ctx context.Context, callerID pgtype.UUID, payload map[string]any, model, voiceID, category, defaultKey string) error {
	effectiveModel := model
	if effectiveModel == "" && defaultKey != "" && h.Settings != nil {
		effectiveModel = h.Settings.GetStringOr(ctx, defaultKey, "")
	}

	if effectiveModel != "" {
		if !h.isValidModelForCategory(effectiveModel, category) {
			return &AssistError{Status: http.StatusBadRequest, Code: "invalid_model", Message: "The requested model is not available for this category"}
		}

		payload["model"] = effectiveModel
	}

	if voiceID != "" {
		voicePrompt, err := h.resolveVoicePrompt(ctx, callerID, voiceID)
		if err != nil {
			return &AssistError{Status: http.StatusBadRequest, Code: "invalid_voice", Message: "The requested voice could not be found"}
		}

		if voicePrompt != "" {
			payload["voice_prompt"] = voicePrompt
		}
	}

	return nil
}

// AssistImprove — improve writing quality. Tone defaults to professional.
func (h *Handlers) AssistImprove(ctx context.Context, userID pgtype.UUID, in AssistImproveInput) (Response, error) {
	if in.Text == "" {
		return Response{}, assistValidation("text is required")
	}

	tone := in.Tone
	if tone == "" {
		tone = "professional"
	}

	return h.runAssist(ctx, userID, assistCall{
		usageTask:     "assist_improve",
		prompt:        "Improve the following text for clarity, grammar, and style. Tone: " + tone + ".\n\n" + in.Text,
		payloadExtra:  map[string]any{"task": "improve", "tone": tone},
		model:         in.Model,
		voiceID:       in.VoiceID,
		humanize:      in.Humanize,
		humanizeLevel: in.HumanizeLevel,
	})
}

// AssistSummarize — summarize text. MaxWords defaults to 100.
func (h *Handlers) AssistSummarize(ctx context.Context, userID pgtype.UUID, in AssistSummarizeInput) (Response, error) {
	if in.Text == "" {
		return Response{}, assistValidation("text is required")
	}

	maxWords := in.MaxWords
	if maxWords <= 0 {
		maxWords = 100
	}

	return h.runAssist(ctx, userID, assistCall{
		usageTask:     "assist_summarize",
		prompt:        "Summarize the following text in at most " + strconv.Itoa(maxWords) + " words.\n\n" + in.Text,
		payloadExtra:  map[string]any{"task": "summarize", "max_words": maxWords},
		model:         in.Model,
		voiceID:       in.VoiceID,
		humanize:      in.Humanize,
		humanizeLevel: in.HumanizeLevel,
	})
}

// AssistTranslate — translate text to a target locale.
func (h *Handlers) AssistTranslate(ctx context.Context, userID pgtype.UUID, in AssistTranslateInput) (Response, error) {
	if in.Text == "" {
		return Response{}, assistValidation("text is required")
	}

	if in.TargetLocale == "" {
		return Response{}, assistValidation("target_locale is required")
	}

	return h.runAssist(ctx, userID, assistCall{
		usageTask:     "assist_translate",
		prompt:        "Translate the following text to locale " + in.TargetLocale + ".\n\n" + in.Text,
		payloadExtra:  map[string]any{"task": "translate", "target_locale": in.TargetLocale},
		model:         in.Model,
		voiceID:       in.VoiceID,
		humanize:      in.Humanize,
		humanizeLevel: in.HumanizeLevel,
	})
}

// AssistGenerateTags — generate a JSON tag array from title + body.
func (h *Handlers) AssistGenerateTags(ctx context.Context, userID pgtype.UUID, in AssistTagsInput) (Response, error) {
	if in.Title == "" && in.Text == "" {
		return Response{}, assistValidation("title or text is required")
	}

	return h.runAssist(ctx, userID, assistCall{
		usageTask:    "assist_generate_tags",
		prompt:       "Generate relevant content tags for the following. Return as a JSON array of strings.\n\nTitle: " + in.Title + "\n\nText: " + in.Text,
		payloadExtra: map[string]any{"task": "generate_tags"},
		model:        in.Model,
		voiceID:      in.VoiceID,
	})
}

// AssistGenerateOG — generate og:title/description/image-alt JSON.
// ContentType defaults to post.
func (h *Handlers) AssistGenerateOG(ctx context.Context, userID pgtype.UUID, in AssistOGInput) (Response, error) {
	if in.Title == "" {
		return Response{}, assistValidation("title is required")
	}

	contentType := in.ContentType
	if contentType == "" {
		contentType = "post"
	}

	return h.runAssist(ctx, userID, assistCall{
		usageTask:    "assist_generate_og",
		prompt:       "Generate Open Graph metadata (og:title, og:description, og:image alt text) for the following " + contentType + ". Return as JSON.\n\nTitle: " + in.Title + "\n\nText: " + in.Text,
		payloadExtra: map[string]any{"task": "generate_og", "content_type": contentType},
		model:        in.Model,
		voiceID:      in.VoiceID,
	})
}

// AssistDraft — generate or improve a body and optionally fill custom
// fields. The draft task uses the JSON response format and a 4096-token
// cap, then parses + normalizes the structured result.
func (h *Handlers) AssistDraft(ctx context.Context, userID pgtype.UUID, in DraftInput) (DraftResult, error) {
	if in.ContentType == "" {
		return DraftResult{}, assistValidation("content_type is required")
	}

	if in.Tone == "" {
		in.Tone = "professional"
	}

	resp, err := h.runAssist(ctx, userID, assistCall{
		usageTask:      "assist_draft",
		prompt:         buildDraftPrompt(in),
		payloadExtra:   map[string]any{"task": "draft", "content_type": in.ContentType, "tone": in.Tone},
		model:          in.Model,
		voiceID:        in.VoiceID,
		humanize:       in.Humanize,
		humanizeLevel:  in.HumanizeLevel,
		responseFormat: map[string]any{"type": "json_object"},
		maxTokens:      4096,
	})
	if err != nil {
		return DraftResult{}, err
	}

	result, err := parseDraftResponse(resp)
	if err != nil {
		return DraftResult{}, &AssistError{Status: http.StatusBadGateway, Code: "ai_error", Message: "Failed to parse AI draft"}
	}

	result.FieldValues = normalizeFieldValues(result.FieldValues, in.FieldSchema)

	return result, nil
}

// writeAssistError maps an assist-op error onto the HTTP envelope: typed
// *AssistError values carry their own status/code; anything else is a
// provider failure (502 ai_error).
func writeAssistError(w http.ResponseWriter, msg string, err error) {
	var ae *AssistError
	if errors.As(err, &ae) {
		errpkg.WriteError(w, ae.Status, ae.Code, ae.Message)
		return
	}

	slog.Error("ai assist failed", "error", err)
	errpkg.WriteError(w, http.StatusBadGateway, "ai_error", msg)
}
