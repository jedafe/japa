package events

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
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
// Two event classes are not bridged:
//   - *.discovered events are the output of the notify_discoveries handler
//     itself — enqueuing them would loop the task on itself indefinitely.
//   - Events with no entity id resolve to nothing the handler can fan out on.
func (p *AsynqEventPublisher) Publish(ctx context.Context, evt DomainEvent) error {
	if strings.HasSuffix(evt.Type, ".discovered") || evt.EntityID == uuid.Nil {
		return nil
	}

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
