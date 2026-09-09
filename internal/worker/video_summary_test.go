package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
)

type fakeVideoSummaryProcessor struct {
	called bool
	err    error
}

func (f *fakeVideoSummaryProcessor) GenerateDraft(ctx context.Context, tenantID, userID, videoSourceID pgtype.UUID) (gen.CreateContentRow, error) {
	f.called = true
	return gen.CreateContentRow{}, f.err
}

func TestHandleVideoGenerateSummary(t *testing.T) {
	proc := &fakeVideoSummaryProcessor{}
	RegisterVideoSummaryProcessor(proc)

	payload := GenerateSummaryPayload{
		TenantID:      pgtype.UUID{Bytes: [16]byte{1}, Valid: true}.String(),
		UserID:        pgtype.UUID{Bytes: [16]byte{2}, Valid: true}.String(),
		VideoSourceID: pgtype.UUID{Bytes: [16]byte{3}, Valid: true}.String(),
	}
	b, _ := json.Marshal(payload)

	task := asynq.NewTask(TypeVideoGenerateSummary, b)
	err := HandleVideoGenerateSummary(context.Background(), task)
	if err != nil {
		t.Fatalf("HandleVideoGenerateSummary: %v", err)
	}
	if !proc.called {
		t.Error("processor not called")
	}
}

func TestHandleVideoGenerateSummary_InvalidPayload(t *testing.T) {
	task := asynq.NewTask(TypeVideoGenerateSummary, []byte("not json"))
	err := HandleVideoGenerateSummary(context.Background(), task)
	if err == nil {
		t.Fatal("expected error for invalid payload")
	}
	if !errors.Is(err, asynq.SkipRetry) {
		t.Errorf("expected SkipRetry, got %v", err)
	}
}
