// Package ai — gateway.go implements the AI Gateway with circuit-breaker
// fallback across providers per category. On failure, the Dispatcher tries
// the next provider in priority order rather than failing the whole operation.
package ai

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
)

// ErrNoProviders marks dispatch failures caused by no provider being
// registered for a category. It is a permanent configuration error —
// retrying the same dispatch cannot conjure a provider — so background
// task handlers should treat errors matching errors.Is(err,
// ErrNoProviders) as non-retryable.
var ErrNoProviders = errors.New("ai: no providers registered")

// Dispatcher holds a priority-ordered list of providers per category and
// implements circuit-breaker fallback: on failure, try the next provider
// in order rather than failing the whole operation.
type Dispatcher struct {
	mu                  sync.RWMutex
	providersByCategory map[string][]Provider
}

// NewDispatcher creates an empty Dispatcher. Providers are registered
// via RegisterProvider before the dispatcher is used.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		providersByCategory: make(map[string][]Provider),
	}
}

// RegisterProvider adds a provider to the priority list for the given
// category. Providers registered first are tried first during dispatch.
func (d *Dispatcher) RegisterProvider(category string, p Provider) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.providersByCategory[category] = append(d.providersByCategory[category], p)
}

// ClearCategory removes all providers registered for the given category.
// Used by ReloadProviders to hot-swap the provider list at runtime.
func (d *Dispatcher) ClearCategory(category string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.providersByCategory, category)
}

// ClearAll removes all providers from all categories. Used when reloading
// the entire provider configuration from the database.
func (d *Dispatcher) ClearAll() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.providersByCategory = make(map[string][]Provider)
}

// HasCategory reports whether any provider is registered for the category.
func (d *Dispatcher) HasCategory(category string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	_, ok := d.providersByCategory[category]

	return ok && len(d.providersByCategory[category]) > 0
}

// RegisteredModels returns the union of models exposed by every registered
// provider across all categories. Used by the admin model picker so the list
// reflects the providers actually wired at startup (Rule 16).
func (d *Dispatcher) RegisteredModels() []Model {
	d.mu.RLock()
	defer d.mu.RUnlock()

	seen := make(map[string]struct{})

	var models []Model

	for _, providers := range d.providersByCategory {
		for _, p := range providers {
			for _, mi := range p.Models() {
				if mi.ID == "" {
					continue
				}

				if _, ok := seen[mi.ID]; ok {
					continue
				}

				seen[mi.ID] = struct{}{}

				models = append(models, Model(mi))
			}
		}
	}

	return models
}

// RegisteredModelsByCategory returns the union of models exposed by every
// provider registered for the given category (e.g. "embedding"). Used by the
// RAG embedding-model picker so only embedding-capable models are offered
// (Rule 16: derived from registered providers, not hardcoded).
func (d *Dispatcher) RegisteredModelsByCategory(category string) []Model {
	d.mu.RLock()
	defer d.mu.RUnlock()

	providers, ok := d.providersByCategory[category]
	if !ok {
		return nil
	}

	seen := make(map[string]struct{})

	var models []Model

	for _, p := range providers {
		for _, mi := range p.Models() {
			if mi.ID == "" {
				continue
			}

			if _, ok := seen[mi.ID]; ok {
				continue
			}

			seen[mi.ID] = struct{}{}

			models = append(models, Model(mi))
		}
	}

	return models
}

// RegisteredCategories returns the categories that have at least one provider
// wired at startup. Used by HandleListCategories so the list reflects the
// actual gateway configuration (Rule 16: no hardcoded data).
func (d *Dispatcher) RegisteredCategories() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	categories := make([]string, 0, len(d.providersByCategory))
	for cat := range d.providersByCategory {
		categories = append(categories, cat)
	}

	return categories
}

// providersForModel returns the providers for a category, reordered so providers
// that advertise the requested model are tried first. If no model is requested
// or no provider advertises it, the configured priority order is preserved.
func (d *Dispatcher) providersForModel(category, model string) []Provider {
	d.mu.RLock()
	providers := d.providersByCategory[category]
	d.mu.RUnlock()

	if model == "" || len(providers) == 0 {
		return providers
	}

	var matched, others []Provider

	for _, p := range providers {
		if providerHasModel(p, model) {
			matched = append(matched, p)
		} else {
			others = append(others, p)
		}
	}

	if len(matched) == 0 {
		return providers
	}

	return append(matched, others...)
}

func providerHasModel(p Provider, model string) bool {
	for _, m := range p.Models() {
		if m.ID == model {
			return true
		}
	}

	return false
}

// Dispatch sends the request to the first available provider for the given
// category. When the request payload specifies a model, providers that
// advertise that model in their catalog are tried first (Rule 16 / ADR-114:
// model-based routing with category fallback). On failure it logs the error
// and tries the next provider, returning on the first success. If all
// providers fail, the last error is returned.
func (d *Dispatcher) Dispatch(ctx context.Context, category string, req Request) (Response, error) {
	model := ""
	if m, ok := req.Payload["model"].(string); ok {
		model = m
	}

	providers := d.providersForModel(category, model)

	if len(providers) == 0 {
		return Response{}, fmt.Errorf("%w for category %q", ErrNoProviders, category)
	}

	var lastErr error

	for _, p := range providers {
		resp, err := p.Send(ctx, req)
		if err == nil {
			resp.ProviderName = p.Name()
			return resp, nil
		}

		lastErr = err
		slog.Error(
			"ai provider failed, trying next",
			"provider", p.Name(),
			"category", category,
			"error", err,
		)
	}

	return Response{}, fmt.Errorf("ai: all providers for category %q failed: %w", category, lastErr)
}

// ChatWithTools dispatches a function-calling chat completion to the first
// available provider for the given category, with circuit-breaker fallback
// identical to Dispatch. On failure it logs the error and tries the next
// provider, returning on the first success. If all providers fail, the last
// error is returned.
//
// Providers that do not support function calling return
// ErrFunctionCallingUnsupported, which is treated as a failure so the
// dispatcher falls through to the next provider.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md.
func (d *Dispatcher) ChatWithTools(ctx context.Context, category string, req ChatWithToolsRequest) (ChatWithToolsResponse, error) {
	providers := d.providersForModel(category, req.Model)

	if len(providers) == 0 {
		return ChatWithToolsResponse{}, fmt.Errorf("%w for category %q", ErrNoProviders, category)
	}

	var lastErr error

	for _, p := range providers {
		resp, err := p.ChatWithTools(ctx, req)
		if err == nil {
			resp.ProviderName = p.Name()
			return resp, nil
		}

		lastErr = err
		slog.Error(
			"ai provider ChatWithTools failed, trying next",
			"provider", p.Name(),
			"category", category,
			"error", err,
		)
	}

	return ChatWithToolsResponse{}, fmt.Errorf("ai: all providers for category %q failed: %w", category, lastErr)
}

// ExtractText normalizes a provider Response's Raw payload into the
// generated-text string every consumer adapter needs. Providers write
// different shapes into Raw — plain "text"/"content"/"output" strings
// (simple providers, Cursor, Devin) or the OpenAI-compatible
// choices[0].message.content structure (OpenAIProvider, XiaomiProvider).
// Use this instead of probing Raw keys directly so adapters work across
// provider types.
func ExtractText(resp Response) string {
	if resp.Raw == nil {
		return ""
	}

	for _, k := range []string{"text", "content", "output"} {
		if v, ok := resp.Raw[k].(string); ok && v != "" {
			return v
		}
	}

	if choices, ok := resp.Raw["choices"].([]openAIChoice); ok && len(choices) > 0 {
		msg := choices[0].Message
		if msg.Content != "" {
			return msg.Content
		}

		return msg.Reasoning
	}

	if choices, ok := resp.Raw["choices"].([]mimoChoice); ok && len(choices) > 0 {
		return choices[0].Message.Content
	}

	if choicesRaw, ok := resp.Raw["choices"].([]any); ok && len(choicesRaw) > 0 {
		if choice, ok := choicesRaw[0].(map[string]any); ok {
			if msg, ok := choice["message"].(map[string]any); ok {
				if c, ok := msg["content"].(string); ok && c != "" {
					return c
				}

				if r, ok := msg["reasoning"].(string); ok {
					return r
				}
			}
		}
	}

	return ""
}

// ExtractModel returns the model identifier a provider reported in
// Response.Raw, or "" when absent.
func ExtractModel(resp Response) string {
	if v, ok := resp.Raw["model"].(string); ok {
		return v
	}

	return ""
}

// ExtractTokens returns total token usage from Response.Raw across the
// provider-specific usage shapes, or 0 when absent.
func ExtractTokens(resp Response) int {
	if v, ok := resp.Raw["total_tokens"].(float64); ok {
		return int(v)
	}

	if v, ok := resp.Raw["tokens_used"].(float64); ok {
		return int(v)
	}

	switch usage := resp.Raw["usage"].(type) {
	case openAIUsage:
		return usage.TotalTokens
	case mimoUsage:
		return usage.TotalTokens
	case map[string]any:
		if t, ok := usage["total_tokens"].(float64); ok {
			return int(t)
		}
	}

	return 0
}
