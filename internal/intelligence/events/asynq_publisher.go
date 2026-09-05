package events

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

// AsynqEventPublisher is the v1 implementation. It enqueues the
// intel:notify_discoveries task, which fans out to §35 notifications.
type AsynqEventPublisher struct {
	client *asynq.Client
}

// NewAsynqEventPublisher returns an AsynqEventPublisher backed by the given client.
func NewAsynqEventPublisher(client *asynq.Client) *AsynqEventPublisher {
	return &AsynqEventPublisher{client: client}
}

// Publish marshals the DomainEvent and enqueues it as an intel:notify_discoveries task.
func (p *AsynqEventPublisher) Publish(ctx context.Context, evt DomainEvent) error {
	payload, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("events: marshal: %w", err)
	}

	task := asynq.NewTask("intel:notify_discoveries", payload)

	_, err = p.client.EnqueueContext(ctx, task)
	if err != nil {
		return fmt.Errorf("events: enqueue: %w", err)
	}

	return nil
}
