// Package ai — gateway.go implements the AI Gateway with circuit-breaker
// fallback across providers per category. On failure, the Dispatcher tries
// the next provider in priority order rather than failing the whole operation.
package ai

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

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
				models = append(models, Model{ID: mi.ID, Label: mi.Label})
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
			models = append(models, Model{ID: mi.ID, Label: mi.Label})
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

// Dispatch sends the request to the first available provider for the given
// category. On failure it logs the error and tries the next provider, returning
// on the first success. If all providers fail, the last error is returned.
func (d *Dispatcher) Dispatch(ctx context.Context, category string, req Request) (Response, error) {
	d.mu.RLock()
	providers := d.providersByCategory[category]
	d.mu.RUnlock()

	if len(providers) == 0 {
		return Response{}, fmt.Errorf("ai: no providers registered for category %q", category)
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
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
func (d *Dispatcher) ChatWithTools(ctx context.Context, category string, req ChatWithToolsRequest) (ChatWithToolsResponse, error) {
	d.mu.RLock()
	providers := d.providersByCategory[category]
	d.mu.RUnlock()

	if len(providers) == 0 {
		return ChatWithToolsResponse{}, fmt.Errorf("ai: no providers registered for category %q", category)
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
