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
			return resp, nil
		}
		lastErr = err
		slog.Error("ai provider failed, trying next",
			"provider", p.Name(),
			"category", category,
			"error", err,
		)
	}

	return Response{}, fmt.Errorf("ai: all providers for category %q failed: %w", category, lastErr)
}
