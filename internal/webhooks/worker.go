package webhooks

import "context"

// WorkerAdapter adapts the webhooks Service to the worker.WebhookProcessor
// interface so the Asynq worker can call the service without importing the
// webhooks package directly.
type WorkerAdapter struct {
	Service *Service
}

// NewWorkerAdapter creates a WorkerAdapter wrapping the given Service.
func NewWorkerAdapter(service *Service) *WorkerAdapter {
	return &WorkerAdapter{Service: service}
}

// DeliverWebhook processes a webhook delivery by ID.
func (a *WorkerAdapter) DeliverWebhook(ctx context.Context, deliveryID [16]byte) error {
	return a.Service.DeliverWebhook(ctx, deliveryID)
}
