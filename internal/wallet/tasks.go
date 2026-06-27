package wallet

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// Task names for Asynq.
const (
	TaskProcessActivityEvent = "wallet:process_activity_event"
)

// ProcessActivityEventPayload holds the payload for the process_activity_event task.
type ProcessActivityEventPayload struct {
	EventID  string `json:"event_id"`
	TenantID string `json:"tenant_id"`
}

// EnqueueProcessActivityEvent enqueues a task to process an activity event.
func (s *Service) EnqueueProcessActivityEvent(ctx context.Context, eventID, tenantID pgtype.UUID) error {
	// This is a placeholder for Asynq integration.
	// In production, this would use an Asynq client to enqueue the task.
	// For now, we'll process synchronously.
	return s.ProcessActivityEvent(ctx, eventID, tenantID)
}

// ProcessActivityEventTask is the Asynq task handler for processing activity events.
// This would be registered with an Asynq worker in production.
func (s *Service) ProcessActivityEventTask(ctx context.Context, payload []byte) error {
	var p ProcessActivityEventPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("unmarshaling payload: %w", err)
	}

	var eventID, tenantID pgtype.UUID
	if err := eventID.Scan(p.EventID); err != nil {
		return fmt.Errorf("invalid event_id: %w", err)
	}
	if err := tenantID.Scan(p.TenantID); err != nil {
		return fmt.Errorf("invalid tenant_id: %w", err)
	}

	return s.ProcessActivityEvent(ctx, eventID, tenantID)
}