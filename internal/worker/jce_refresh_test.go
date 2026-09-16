package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
)

type fakeIngestionService struct {
	called      bool
	platforms     []pgtype.UUID
	contents    []pgtype.UUID
	jobs        []pgtype.UUID
	sourceKinds []*string
	users       []pgtype.UUID
	schEnqueued int
	err         error
}

func (f *fakeIngestionService) ProcessRefreshValues(_ context.Context, contentID, jobID pgtype.UUID, sourceKind *string, userID pgtype.UUID) error {
	f.called = true
	f.contents = append(f.contents, contentID)
	f.jobs = append(f.jobs, jobID)
	f.sourceKinds = append(f.sourceKinds, sourceKind)
	f.users = append(f.users, userID)

	return f.err
}

func (f *fakeIngestionService) ScheduleStaleRefresh(_ context.Context, _ pgtype.UUID) (int, error) {
	f.called = true
	return f.schEnqueued, f.err
}

func TestIngestion_JCERefresh_PayloadRoundTrip(t *testing.T) {
	fake := &fakeIngestionService{}
	RegisterJCEService(fake)

	defer RegisterJCEService(nil)

	contentID := uuid.New().String()
	jobID := uuid.New().String()
	userID := uuid.New().String()
	sk := "official"

	payload, err := json.Marshal(JceRefreshValuesPayload{

		ContentID:      contentID,
		SourceKind:     &sk,
		IngestionJobID: jobID,
		RequestedBy:    userID,
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	task := asynq.NewTask(TypeJCERefreshValues, payload)
	if err := HandleJceRefreshValues(t.Context(), task); err != nil {
		t.Fatalf("HandleJceRefreshValues: %v", err)
	}

	if !fake.called {
		t.Fatalf("service not called")
	}

	if fake.contents[0].String() != contentID {
		t.Fatalf("content_id mismatch")
	}

	if fake.jobs[0].String() != jobID {
		t.Fatalf("job_id mismatch")
	}

	if fake.users[0].String() != userID {
		t.Fatalf("user_id mismatch")
	}

	if fake.sourceKinds[0] == nil || *fake.sourceKinds[0] != sk {
		t.Fatalf("source_kind mismatch")
	}
}

func TestIngestion_JCERefresh_MalformedPayload(t *testing.T) {
	fake := &fakeIngestionService{}
	RegisterJCEService(fake)

	defer RegisterJCEService(nil)

	task := asynq.NewTask(TypeJCERefreshValues, []byte("not json"))

	err := HandleJceRefreshValues(t.Context(), task)
	if err == nil {
		t.Fatalf("expected error for malformed payload")
	}

	if !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("expected SkipRetry for malformed payload, got %v", err)
	}

	if fake.called {
		t.Fatalf("service should not be called with malformed payload")
	}
}

func TestIngestion_JCERefresh_ServiceNotRegistered(t *testing.T) {
	RegisterJCEService(nil)

	payload, err := json.Marshal(JceRefreshValuesPayload{

		ContentID:      uuid.New().String(),
		IngestionJobID: uuid.New().String(),
		RequestedBy:    uuid.New().String(),
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	task := asynq.NewTask(TypeJCERefreshValues, payload)

	err = HandleJceRefreshValues(t.Context(), task)
	if err == nil {
		t.Fatalf("expected error when service not registered")
	}

	if !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("expected SkipRetry, got %v", err)
	}
}

func TestIngestion_JCERefresh_RetryableServiceError(t *testing.T) {
	fake := &fakeIngestionService{err: errors.New("transient database error")}
	RegisterJCEService(fake)

	defer RegisterJCEService(nil)

	payload, err := json.Marshal(JceRefreshValuesPayload{

		ContentID:      uuid.New().String(),
		IngestionJobID: uuid.New().String(),
		RequestedBy:    uuid.New().String(),
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	task := asynq.NewTask(TypeJCERefreshValues, payload)

	err = HandleJceRefreshValues(t.Context(), task)
	if err == nil {
		t.Fatalf("expected retryable error")
	}

	if errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("did not expect SkipRetry for retryable service error")
	}

	if !fake.called {
		t.Fatalf("service not called")
	}
}

func TestIngestion_JCERefreshStale_Sweep(t *testing.T) {
	fake := &fakeIngestionService{schEnqueued: 3}
	RegisterJCEService(fake)

	defer RegisterJCEService(nil)

	task := asynq.NewTask(TypeJceRefreshStale, []byte("{}"))
	if err := HandleJceRefreshStale(t.Context(), task); err != nil {
		t.Fatalf("HandleJceRefreshStale: %v", err)
	}

	if !fake.called {
		t.Fatalf("ScheduleStaleRefresh not called")
	}
}

func TestIngestion_JCERefreshStale_ServiceNotRegistered(t *testing.T) {
	RegisterJCEService(nil)

	task := asynq.NewTask(TypeJceRefreshStale, []byte("{}"))

	err := HandleJceRefreshStale(t.Context(), task)
	if err == nil {
		t.Fatalf("expected error when service not registered")
	}

	if !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("expected SkipRetry, got %v", err)
	}
}
