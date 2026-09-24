package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

type fakeFieldsPropagator struct {
	calls      int
	entityType string
	entityID   string
	err        error
}

func (f *fakeFieldsPropagator) PropagateFields(_ context.Context, entityType, entityID string) error {
	f.calls++
	f.entityType = entityType
	f.entityID = entityID

	return f.err
}

func TestWikiFieldsPropagate_ValidPayload(t *testing.T) {
	fake := &fakeFieldsPropagator{}
	RegisterWikiFieldsPropagator(fake)

	defer RegisterWikiFieldsPropagator(nil)

	entityType := "organization"
	entityID := uuid.New().String()

	payload, err := json.Marshal(map[string]string{
		"entity_type": entityType,
		"entity_id":   entityID,
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	task := asynq.NewTask(TypeWikiFieldsPropagate, payload)
	if err := HandleWikiFieldsPropagate(t.Context(), task); err != nil {
		t.Fatalf("HandleWikiFieldsPropagate: %v", err)
	}

	if fake.calls != 1 {
		t.Fatalf("expected propagator called once, got %d", fake.calls)
	}

	if fake.entityType != entityType {
		t.Fatalf("entity_type mismatch: got %q, want %q", fake.entityType, entityType)
	}

	if fake.entityID != entityID {
		t.Fatalf("entity_id mismatch: got %q, want %q", fake.entityID, entityID)
	}
}

func TestWikiFieldsPropagate_MalformedPayload(t *testing.T) {
	fake := &fakeFieldsPropagator{}
	RegisterWikiFieldsPropagator(fake)

	defer RegisterWikiFieldsPropagator(nil)

	task := asynq.NewTask(TypeWikiFieldsPropagate, []byte("not json"))

	err := HandleWikiFieldsPropagate(t.Context(), task)
	if err == nil {
		t.Fatalf("expected error for malformed payload")
	}

	if fake.calls != 0 {
		t.Fatalf("propagator should not be called with malformed payload")
	}
}

func TestWikiFieldsPropagate_MissingFields(t *testing.T) {
	fake := &fakeFieldsPropagator{}
	RegisterWikiFieldsPropagator(fake)

	defer RegisterWikiFieldsPropagator(nil)

	for _, payload := range []map[string]string{
		{"entity_type": "", "entity_id": uuid.New().String()},
		{"entity_type": "organization", "entity_id": ""},
		{},
	} {
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}

		task := asynq.NewTask(TypeWikiFieldsPropagate, raw)

		if err := HandleWikiFieldsPropagate(t.Context(), task); err == nil {
			t.Fatalf("expected error for payload %v", payload)
		}
	}

	if fake.calls != 0 {
		t.Fatalf("propagator should not be called with missing fields")
	}
}

func TestWikiFieldsPropagate_PropagatorError(t *testing.T) {
	fake := &fakeFieldsPropagator{err: errors.New("transient database error")}
	RegisterWikiFieldsPropagator(fake)

	defer RegisterWikiFieldsPropagator(nil)

	payload, err := json.Marshal(map[string]string{
		"entity_type": "organization",
		"entity_id":   uuid.New().String(),
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	task := asynq.NewTask(TypeWikiFieldsPropagate, payload)

	err = HandleWikiFieldsPropagate(t.Context(), task)
	if err == nil {
		t.Fatalf("expected retryable error")
	}

	if errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("did not expect SkipRetry for propagator error")
	}

	if fake.calls != 1 {
		t.Fatalf("expected propagator called once, got %d", fake.calls)
	}
}

func TestWikiFieldsPropagate_NoPropagatorRegistered(t *testing.T) {
	RegisterWikiFieldsPropagator(nil)

	payload, err := json.Marshal(map[string]string{
		"entity_type": "organization",
		"entity_id":   uuid.New().String(),
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	task := asynq.NewTask(TypeWikiFieldsPropagate, payload)

	// Nil-safe skip: unregistered propagator must not error or retry.
	if err := HandleWikiFieldsPropagate(t.Context(), task); err != nil {
		t.Fatalf("expected nil when no propagator registered, got %v", err)
	}
}
