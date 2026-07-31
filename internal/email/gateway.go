package email

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// EmailDispatcher holds a priority-ordered list of providers per category and
// implements circuit-breaker fallback: on failure, try the next provider
// in order rather than failing the whole operation.
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

// Dispatch sends the email to the first available provider for the given
// category. On failure it logs the error and tries the next provider, returning
// on the first success. If all providers fail, the last error is returned.
func (d *EmailDispatcher) Dispatch(ctx context.Context, req EmailRequest) (EmailResponse, error) {
	d.mu.RLock()
	providers := d.providersByCategory[req.Category]
	d.mu.RUnlock()

	if len(providers) == 0 {
		return EmailResponse{}, fmt.Errorf("email: no providers registered for category %q", req.Category)
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
