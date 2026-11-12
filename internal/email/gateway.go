package email

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
var ErrNoProviders = errors.New("email: no providers registered")

// EmailDispatcher holds a priority-ordered list of providers per category and
// implements circuit-breaker fallback: on failure, try the next provider
// in order rather than failing the whole operation.
//
//nolint:revive // exported: stutter naming is intentional
type EmailDispatcher struct {
	mu                  sync.RWMutex
	providersByCategory map[string][]EmailProvider
}

// NewEmailDispatcher creates an empty EmailDispatcher. Providers are registered
// via RegisterProvider before the dispatcher is used.
func NewEmailDispatcher() *EmailDispatcher {
	return &EmailDispatcher{
		providersByCategory: make(map[string][]EmailProvider),
	}
}

// RegisterProvider adds a provider to the priority list for the given
// category. Providers registered first are tried first during dispatch.
func (d *EmailDispatcher) RegisterProvider(category string, p EmailProvider) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.providersByCategory[category] = append(d.providersByCategory[category], p)
}

// ClearCategory removes all providers registered for the given category.
// Used by LoadAndRegisterProviders to hot-swap the provider list at runtime.
func (d *EmailDispatcher) ClearCategory(category string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.providersByCategory, category)
}

// ClearAll removes all providers from all categories. Used when reloading
// the entire provider configuration from the database.
func (d *EmailDispatcher) ClearAll() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.providersByCategory = make(map[string][]EmailProvider)
}

// ReplaceProviders atomically swaps the whole category→chain map under a
// single lock. LoadAndRegisterProviders builds the new chains first and
// calls this once, so an in-flight Dispatch never observes a partially
// rebuilt (empty) category — which would surface as ErrNoProviders and
// permanently drop a queued send.
func (d *EmailDispatcher) ReplaceProviders(chains map[string][]EmailProvider) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.providersByCategory = chains
}

// HasCategory reports whether any provider is registered for the category.
func (d *EmailDispatcher) HasCategory(category string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return len(d.providersByCategory[category]) > 0
}

// Dispatch sends the email to the first available provider for the given
// category. On failure it logs the error and tries the next provider, returning
// on the first success. If all providers fail, the last error is returned.
func (d *EmailDispatcher) Dispatch(ctx context.Context, req EmailRequest) (EmailResponse, error) {
	d.mu.RLock()
	providers := d.providersByCategory[req.Category]
	d.mu.RUnlock()

	if len(providers) == 0 {
		return EmailResponse{}, fmt.Errorf("%w for category %q", ErrNoProviders, req.Category)
	}

	var lastErr error

	for _, p := range providers {
		resp, err := p.Send(ctx, req)
		if err == nil {
			return resp, nil
		}

		lastErr = err
		slog.Error(
			"email provider failed, trying next",
			"provider", p.Name(),
			"category", req.Category,
			"error", err,
		)
	}

	return EmailResponse{}, fmt.Errorf("email: all providers for category %q failed: %w", req.Category, lastErr)
}
