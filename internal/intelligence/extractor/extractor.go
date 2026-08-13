package extractor

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jedafe/japa/internal/ai"
	"github.com/jedafe/japa/internal/intelligence/extractor/schemas"
)

// AIGateway is the minimal interface the extractor needs from §20. The
// concrete *ai.Dispatcher satisfies this interface; tests substitute a mock.
type AIGateway interface {
	ChatWithTools(ctx context.Context, category string, req ai.ChatWithToolsRequest) (ai.ChatWithToolsResponse, error)
}

// Extractor calls the AI gateway to extract structured entity fields from
// source text, then validates the output against the entity schema.
type Extractor struct {
	gateway     AIGateway
	category    string
	model       string
	temperature float64
	maxTokens   int
	maxRetries  int
	timeout     time.Duration
}

// ExtractorOption configures an Extractor.
type ExtractorOption func(*Extractor)

// WithModel sets the LLM model identifier passed in the request.
func WithModel(model string) ExtractorOption {
	return func(e *Extractor) { e.model = model }
}

// WithTemperature sets the sampling temperature (default 0.0 for determinism).
func WithTemperature(t float64) ExtractorOption {
	return func(e *Extractor) { e.temperature = t }
}

// WithMaxTokens sets the max output tokens for the completion.
func WithMaxTokens(n int) ExtractorOption {
	return func(e *Extractor) { e.maxTokens = n }
}

// WithMaxRetries sets the number of retries on malformed JSON (default 2).
func WithMaxRetries(n int) ExtractorOption {
	return func(e *Extractor) { e.maxRetries = n }
}

// WithTimeout sets the per-call timeout for the AI gateway call.
func WithTimeout(d time.Duration) ExtractorOption {
	return func(e *Extractor) { e.timeout = d }
}

// WithCategory sets the AI gateway category to dispatch to
// (default "extraction").
func WithCategory(c string) ExtractorOption {
	return func(e *Extractor) { e.category = c }
}

// NewExtractor creates an Extractor backed by the given AI gateway.
func NewExtractor(gw AIGateway, opts ...ExtractorOption) *Extractor {
	e := &Extractor{
		gateway:     gw,
		category:    "extraction",
		temperature: 0.0,
		maxTokens:   4096,
		maxRetries:  2,
		timeout:     60 * time.Second,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// ExtractResult is the output of a successful extraction: the validated
// extraction map plus the validation errors (which are non-fatal — the
// hallucination guard may reset fields to null while still returning data).
type ExtractResult struct {
	Extraction Extraction
	Errors     []ValidationError
}

// Extract calls the AI gateway with the scholarship prompt, parses the JSON
// response, and validates it against the scholarship schema. It retries up to
// maxRetries times when the LLM returns malformed JSON.
func (e *Extractor) Extract(ctx context.Context, schema schemas.EntitySchema, sourceURL, sourceType, sourceText string) (ExtractResult, error) {
	prompt, err := buildPrompt(schema, sourceURL, sourceType, sourceText)
	if err != nil {
		return ExtractResult{}, err
	}

	var lastErr error
	for attempt := 0; attempt <= e.maxRetries; attempt++ {
		callCtx, cancel := context.WithTimeout(ctx, e.timeout)
		resp, err := e.gateway.ChatWithTools(callCtx, e.category, ai.ChatWithToolsRequest{
			Messages: []ai.ChatMessage{
				{Role: "user", Content: prompt},
			},
			Model:       e.model,
			Temperature: e.temperature,
			MaxTokens:   e.maxTokens,
		})
		cancel()
		if err != nil {
			lastErr = err
			continue
		}

		raw := stripFences(resp.Content)
		ext, perr := ParseExtraction([]byte(raw))
		if perr != nil {
			lastErr = perr
			continue
		}

		cleaned, verrs := Validate(ext, schema)
		return ExtractResult{Extraction: cleaned, Errors: verrs}, nil
	}

	return ExtractResult{}, fmt.Errorf("extractor: failed after %d attempts: %w", e.maxRetries+1, lastErr)
}

// stripFences removes surrounding markdown code fences (```json ... ```)
// that some LLMs emit despite instructions not to.
func stripFences(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	// Drop the opening fence line.
	if idx := strings.Index(s, "\n"); idx >= 0 {
		s = s[idx+1:]
	}
	s = strings.TrimSpace(s)
	// Drop the closing fence.
	if strings.HasSuffix(s, "```") {
		s = s[:len(s)-3]
	}
	return strings.TrimSpace(s)
}

// ErrEmptyContent is returned when the AI gateway returns an empty content
// body that cannot be parsed.
var ErrEmptyContent = errors.New("extractor: AI gateway returned empty content")

//go:embed prompts/scholarship.txt
var scholarshipPrompt string

// buildPrompt loads the prompt template for the given entity schema and
// substitutes the placeholders.
func buildPrompt(schema schemas.EntitySchema, sourceURL, sourceType, sourceText string) (string, error) {
	tmpl := scholarshipPrompt
	if schema.EntityType != "scholarship" {
		return "", fmt.Errorf("extractor: no prompt template for entity type %q", schema.EntityType)
	}
	schemaDesc := describeSchema(schema)
	out := strings.ReplaceAll(tmpl, "{{ENTITY_SCHEMA}}", schemaDesc)
	out = strings.ReplaceAll(out, "{{SOURCE_URL}}", sourceURL)
	out = strings.ReplaceAll(out, "{{SOURCE_TYPE}}", sourceType)
	out = strings.ReplaceAll(out, "{{SOURCE_TEXT}}", sourceText)
	return out, nil
}

// describeSchema renders the schema as a human-readable field list for the
// prompt's ENTITY_SCHEMA placeholder.
func describeSchema(schema schemas.EntitySchema) string {
	var b strings.Builder
	fmt.Fprintf(&b, "entity_type: %s\nfields:", schema.EntityType)
	for _, f := range schema.Fields {
		req := "optional"
		if f.Required {
			req = "required"
		}
		enum := ""
		if len(f.EnumValues) > 0 {
			enum = fmt.Sprintf(" (enum: %s)", strings.Join(f.EnumValues, ", "))
		}
		fmt.Fprintf(&b, "\n  - %s: %s, %s%s", f.Name, f.Type, req, enum)
	}
	return b.String()
}
