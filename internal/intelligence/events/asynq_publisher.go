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

// Publish marshals the DomainEvent and enqueues it as a task on the bridge.
// Routing is by event suffix:
//   - *.discovered events → intel:discovery_fanout — they are the output of
//     notify_discoveries, so routing them back there would loop the task on
//     itself indefinitely; the fan-out task is their terminal consumer.
//   - everything else → intel:notify_discoveries.
// Events with no entity id resolve to nothing the handlers can fan out on.
func (p *AsynqEventPublisher) Publish(ctx context.Context, evt DomainEvent) error {
	if evt.EntityID == uuid.Nil {
		return nil
	}

	taskType := "intel:notify_discoveries"
	if strings.HasSuffix(evt.Type, ".discovered") {
		taskType = "intel:discovery_fanout"
	}

	payload, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("events: marshal: %w", err)
	}

	task := asynq.NewTask(taskType, payload)

	_, err = p.client.EnqueueContext(ctx, task)
	if err != nil {
		return fmt.Errorf("events: enqueue: %w", err)
	}

	return nil
}
