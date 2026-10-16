package worker

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/hibiken/asynq"
)

// SubscriptionProcessor is the seam into payments.Service for subscription
// lifecycle work (JEOS §17 — webhook is the source of truth for activation,
// renewal, past_due, cancellation; the expiry sweep is the safety net).
type SubscriptionProcessor interface {
	ProcessSubscriptionEvent(ctx context.Context, provider, eventType, eventID string, payload []byte) error
	RunExpirySweep(ctx context.Context) (int, error)
}

var subscriptionProcessor SubscriptionProcessor

// RegisterSubscriptionProcessor wires the payments service into the worker
// for subscription:webhook_process and subscription:expiry_sweep tasks.
func RegisterSubscriptionProcessor(p SubscriptionProcessor) {
	subscriptionProcessor = p
}

// HandleSubscriptionWebhookProcess applies one provider subscription
// lifecycle event. The HTTP handler already verified the signature and
// recorded the dedupe row before enqueueing — this handler carries the raw
// payload through so the worker never re-reads the events table.
func HandleSubscriptionWebhookProcess(ctx context.Context, t *asynq.Task) error {
	var payload struct {
		Provider  string          `json:"provider"`
		EventID   string          `json:"event_id"`
		EventType string          `json:"event_type"`
		Raw       json.RawMessage `json:"payload"`
	}

	if err := parsePayloadOrSkip(t, &payload); err != nil {
		return err
	}

	if subscriptionProcessor == nil {
		slog.Warn("task:subscription:webhook_process: no processor registered, skipping")
		return nil
	}

	return subscriptionProcessor.ProcessSubscriptionEvent(ctx, payload.Provider, payload.EventType, payload.EventID, payload.Raw)
}

// HandleSubscriptionExpirySweep expires subscriptions past their entitlement
// window — the webhook-miss safety net (§17). Scheduled by NewScheduler.
func HandleSubscriptionExpirySweep(ctx context.Context, t *asynq.Task) error {
	_ = t // no payload

	if subscriptionProcessor == nil {
		slog.Warn("task:subscription:expiry_sweep: no processor registered, skipping")
		return nil
	}

	n, err := subscriptionProcessor.RunExpirySweep(ctx)
	if err != nil {
		return err
	}

	if n > 0 {
		slog.Info("task:subscription:expiry_sweep: expired subscriptions", "count", n)
	}

	return nil
}
