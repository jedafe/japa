// Package events defines the domain event emission seam for the
// Intelligence System. v1 ships AsynqEventPublisher; a future KafkaEventPublisher
// can swap in at this one interface without touching business logic.
//
// Ref: docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md (Section 6)
package events

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// DomainEventPublisher emits intelligence domain events.
type DomainEventPublisher interface {
	Publish(ctx context.Context, evt DomainEvent) error
}

// DomainEvent is the canonical event shape emitted on scholarship publish/update.
type DomainEvent struct {
	Type       string          `json:"type"`        // "scholarship.published" | "scholarship.deadline_changed"
	EntityType string          `json:"entity_type"` // "scholarship"
	EntityID   uuid.UUID       `json:"entity_id"`
	Payload    json.RawMessage `json:"payload"`
	OccurredAt time.Time       `json:"occurred_at"`
}
