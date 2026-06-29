package worker

import "context"

// WebhookProcessor defines the interface for webhook delivery processing.
// The webhooks service implements this interface and registers with the
// worker so the HandleWebhookDelivery handler can delegate to it.
type WebhookProcessor interface {
	DeliverWebhook(ctx context.Context, deliveryID [16]byte) error
}

var webhookProcessor WebhookProcessor

// RegisterWebhookProcessor registers the webhooks service processor for
// background delivery tasks.
func RegisterWebhookProcessor(p WebhookProcessor) {
	webhookProcessor = p
}
