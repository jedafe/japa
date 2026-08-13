package events

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// newTestClient starts an in-process Redis (miniredis) and returns an Asynq
// client plus an inspector for inspecting the resulting queue.
func newTestClient(t *testing.T) (*asynq.Client, *asynq.Inspector) {
	t.Helper()
	mr := miniredis.RunT(t)
	opt := asynq.RedisClientOpt{Addr: mr.Addr()}
	client := asynq.NewClient(opt)
	inspector := asynq.NewInspector(opt)
	t.Cleanup(func() {
		_ = client.Close()
		_ = inspector.Close()
	})
	return client, inspector
}

func TestAsynqEventPublisher_Publish(t *testing.T) {
	client, inspector := newTestClient(t)
	pub := NewAsynqEventPublisher(client)

	occurredAt := time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC)
	evt := DomainEvent{
		Type:       "scholarship.published",
		EntityType: "scholarship",
		EntityID:   uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		Payload:    json.RawMessage(`{"scholarship_id":"11111111-1111-1111-1111-111111111111"}`),
		OccurredAt: occurredAt,
	}

	if err := pub.Publish(context.Background(), evt); err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}

	tasks, err := inspector.ListPendingTasks("default", asynq.PageSize(10))
	if err != nil {
		t.Fatalf("GetPendingTasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 pending task, got %d", len(tasks))
	}
	if tasks[0].Type != "intel:notify_discoveries" {
		t.Fatalf("task type = %q, want %q", tasks[0].Type, "intel:notify_discoveries")
	}

	var got DomainEvent
	if err := json.Unmarshal(tasks[0].Payload, &got); err != nil {
		t.Fatalf("unmarshal DomainEvent from task payload: %v", err)
	}
	if got.Type != evt.Type {
		t.Fatalf("Type = %q, want %q", got.Type, evt.Type)
	}
	if got.EntityType != evt.EntityType {
		t.Fatalf("EntityType = %q, want %q", got.EntityType, evt.EntityType)
	}
	if got.EntityID != evt.EntityID {
		t.Fatalf("EntityID = %v, want %v", got.EntityID, evt.EntityID)
	}
	if !got.OccurredAt.Equal(evt.OccurredAt) {
		t.Fatalf("OccurredAt = %v, want %v", got.OccurredAt, evt.OccurredAt)
	}
	if string(got.Payload) != string(evt.Payload) {
		t.Fatalf("Payload = %s, want %s", got.Payload, evt.Payload)
	}
}

func TestAsynqEventPublisher_PublishDeadlineChanged(t *testing.T) {
	client, inspector := newTestClient(t)
	pub := NewAsynqEventPublisher(client)

	occurredAt := time.Date(2026, 8, 12, 10, 30, 0, 0, time.UTC)
	evt := DomainEvent{
		Type:       "scholarship.deadline_changed",
		EntityType: "scholarship",
		EntityID:   uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		Payload:    json.RawMessage(`{"old":"2026-09-01","new":"2026-08-20"}`),
		OccurredAt: occurredAt,
	}

	if err := pub.Publish(context.Background(), evt); err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}

	tasks, err := inspector.ListPendingTasks("default", asynq.PageSize(10))
	if err != nil {
		t.Fatalf("GetPendingTasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 pending task, got %d", len(tasks))
	}
	if tasks[0].Type != "intel:notify_discoveries" {
		t.Fatalf("task type = %q, want %q", tasks[0].Type, "intel:notify_discoveries")
	}

	var got DomainEvent
	if err := json.Unmarshal(tasks[0].Payload, &got); err != nil {
		t.Fatalf("unmarshal DomainEvent: %v", err)
	}
	if got.Type != "scholarship.deadline_changed" {
		t.Fatalf("Type = %q, want %q", got.Type, "scholarship.deadline_changed")
	}
}

// Compile-time check that AsynqEventPublisher satisfies the interface.
var _ DomainEventPublisher = (*AsynqEventPublisher)(nil)
