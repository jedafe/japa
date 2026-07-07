package worker

import (
	"github.com/hibiken/asynq"
)

// Enqueuer adapts *asynq.Client to the string/[]byte enqueue interface used by
// the feature services (gdpr, affiliate, coupons, tickets, analytics), whose
// Enqueuer interfaces all have the shape:
//
//	Enqueue(taskType string, payload []byte, opts ...interface{}) error
//
// In development those services fall back to a per-package noopEnqueuer that
// silently drops every task. Wiring this adapter in server setup makes their
// background jobs actually run (e.g. GDPR account-deletion cleanup + Honcho
// erasure, affiliate commission crediting, coupon expiry, ticket status
// emails, analytics event flushing).
type Enqueuer struct {
	client *asynq.Client
}

// NewEnqueuer wraps an *asynq.Client so it satisfies the feature-service
// Enqueuer interfaces via structural typing.
func NewEnqueuer(client *asynq.Client) *Enqueuer {
	return &Enqueuer{client: client}
}

// Enqueue builds an asynq.Task from the task type and payload and enqueues it
// on the shared client. Any variadic opts that are asynq.Option values are
// forwarded to the client; any other values are ignored so callers that pass
// nothing (the common case) work unchanged.
func (e *Enqueuer) Enqueue(taskType string, payload []byte, opts ...interface{}) error {
	if e == nil || e.client == nil {
		return nil
	}
	asynqOpts := make([]asynq.Option, 0, len(opts))
	for _, o := range opts {
		if opt, ok := o.(asynq.Option); ok {
			asynqOpts = append(asynqOpts, opt)
		}
	}
	_, err := e.client.Enqueue(asynq.NewTask(taskType, payload), asynqOpts...)
	return err
}
