package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
)

type fakeIngestionService struct {
	called    bool
	tenants   []pgtype.UUID
	contents  []pgtype.UUID
	jobs      []pgtype.UUID
	sourceKinds []*string
	users     []pgtype.UUID
	err       error
}

func (f *fakeIngestionService) ProcessRefreshValues(ctx context.Context, tenantID, contentID, jobID pgtype.UUID, sourceKind *string, userID pgtype.UUID) error {
	f.called = true
	f.tenants = append(f.tenants, tenantID)
	f.contents = append(f.contents, contentID)
	f.jobs = append(f.jobs, jobID)
	f.sourceKinds = append(f.sourceKinds, sourceKind)
	f.users = append(f.users, userID)
	return f.err
}

func TestIngestion_JCERefresh_PayloadRoundTrip(t *testing.T) {
	t.Parallel()

	fake := &fakeIngestionService{}
	RegisterJCEService(fake)
	defer RegisterJCEService(nil)

	tenantID := uuid.New().String()
	contentID := uuid.New().String()
	jobID := uuid.New().String()
	userID := uuid.New().String()
	sk := "official"
	payload, err := json.Marshal(JceRefreshValuesPayload{
		TenantID:       tenantID,
		ContentID:      contentID,
		SourceKind:     &sk,
		IngestionJobID: jobID,
		RequestedBy:    userID,
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	task := asynq.NewTask(TypeJCERefreshValues, payload)
	if err := HandleJceRefreshValues(context.Background(), task); err != nil {
		t.Fatalf("HandleJceRefreshValues: %v", err)
	}

	if !fake.called {
		t.Fatalf("service not called")
	}
	if fake.tenants[0].String() != tenantID {
		t.Fatalf("tenant_id mismatch")
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

func TestIngestion_JCERefresh_UUIDValidation(t *testing.T) {
	t.Parallel()

	fake := &fakeIngestionService{}
	RegisterJCEService(fake)
	defer RegisterJCEService(nil)

	badPayload, err := json.Marshal(JceRefreshValuesPayload{
		TenantID:       "not-a-uuid",
		ContentID:      uuid.New().String(),
		IngestionJobID: uuid.New().String(),
		RequestedBy:    uuid.New().String(),
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	task := asynq.NewTask(TypeJCERefreshValues, badPayload)
	err = HandleJceRefreshValues(context.Background(), task)
	if err == nil {
		t.Fatalf("expected error for invalid tenant_id")
	}
	if !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("expected SkipRetry error, got %v", err)
	}
	if fake.called {
		t.Fatalf("service should not be called with invalid payload")
	}
}

func TestIngestion_JCERefresh_MalformedPayload(t *testing.T) {
	t.Parallel()

	fake := &fakeIngestionService{}
	RegisterJCEService(fake)
	defer RegisterJCEService(nil)

	task := asynq.NewTask(TypeJCERefreshValues, []byte("not json"))
	err := HandleJceRefreshValues(context.Background(), task)
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
	t.Parallel()

	RegisterJCEService(nil)

	payload, err := json.Marshal(JceRefreshValuesPayload{
		TenantID:       uuid.New().String(),
		ContentID:      uuid.New().String(),
		IngestionJobID: uuid.New().String(),
		RequestedBy:    uuid.New().String(),
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	task := asynq.NewTask(TypeJCERefreshValues, payload)
	err = HandleJceRefreshValues(context.Background(), task)
	if err == nil {
		t.Fatalf("expected error when service not registered")
	}
	if !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("expected SkipRetry, got %v", err)
	}
}

func TestIngestion_JCERefresh_RetryableServiceError(t *testing.T) {
	t.Parallel()

	fake := &fakeIngestionService{err: fmt.Errorf("transient database error")}
	RegisterJCEService(fake)
	defer RegisterJCEService(nil)

	payload, err := json.Marshal(JceRefreshValuesPayload{
		TenantID:       uuid.New().String(),
		ContentID:      uuid.New().String(),
		IngestionJobID: uuid.New().String(),
		RequestedBy:    uuid.New().String(),
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	task := asynq.NewTask(TypeJCERefreshValues, payload)
	err = HandleJceRefreshValues(context.Background(), task)
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
