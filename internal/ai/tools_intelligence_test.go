package ai

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/worker"
)

type stubTaskEnqueuer struct {
	called    bool
	taskType  string
	payload   []byte
	returnErr error
}

func (s *stubTaskEnqueuer) Enqueue(taskType string, payload []byte, opts ...any) error {
	s.called = true
	s.taskType = taskType
	s.payload = payload
	return s.returnErr
}

type stubIntelQuerier struct {
	source gen.IntelSource
	record gen.IntelRecord
	err    error
}

func (s *stubIntelQuerier) GetIntelSource(ctx context.Context, arg gen.GetIntelSourceParams) (gen.IntelSource, error) {
	if s.err != nil {
		return gen.IntelSource{}, s.err
	}
	return s.source, nil
}

func (s *stubIntelQuerier) GetIntelRecord(ctx context.Context, arg gen.GetIntelRecordParams) (gen.IntelRecord, error) {
	if s.err != nil {
		return gen.IntelRecord{}, s.err
	}
	return s.record, nil
}

func (s *stubIntelQuerier) GetIntelSourceByURL(ctx context.Context, arg gen.GetIntelSourceByURLParams) (gen.IntelSource, error) {
	if s.err != nil {
		return gen.IntelSource{}, s.err
	}
	return s.source, nil
}

func TestSourceCrawlNowHandler(t *testing.T) {
	t.Parallel()

	enq := &stubTaskEnqueuer{}
	q := &stubIntelQuerier{source: gen.IntelSource{Enabled: true}}
	deps := &ToolDeps{TaskEnqueuer: enq, IntelQuerier: q}

	handler := SourceCrawlNowHandler(deps)
	tr, err := handler(context.Background(), json.RawMessage(`{"source_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{}, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}
	if !enq.called {
		t.Fatal("expected task to be enqueued")
	}
	if enq.taskType != worker.TypeIntelFetchSource {
		t.Fatalf("task type = %q, want %q", enq.taskType, worker.TypeIntelFetchSource)
	}
}

func TestSourceCrawlNowHandler_DisabledSource(t *testing.T) {
	t.Parallel()

	enq := &stubTaskEnqueuer{}
	q := &stubIntelQuerier{source: gen.IntelSource{Enabled: false}}
	deps := &ToolDeps{TaskEnqueuer: enq, IntelQuerier: q}

	handler := SourceCrawlNowHandler(deps)
	tr, _ := handler(context.Background(), json.RawMessage(`{"source_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{}, pgtype.UUID{})
	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
	if enq.called {
		t.Error("expected no task to be enqueued for disabled source")
	}
}

func TestRecordUpdateNowHandler_ByRecord(t *testing.T) {
	t.Parallel()

	sourceID := pgtype.UUID{}
	sourceID.Scan("00000000-0000-0000-0000-000000000002")

	enq := &stubTaskEnqueuer{}
	q := &stubIntelQuerier{
		record: gen.IntelRecord{SourceID: sourceID},
		source: gen.IntelSource{Enabled: true},
	}
	deps := &ToolDeps{TaskEnqueuer: enq, IntelQuerier: q}

	handler := RecordUpdateNowHandler(deps)
	tr, err := handler(context.Background(), json.RawMessage(`{"record_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{}, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}
	if !enq.called {
		t.Fatal("expected task to be enqueued")
	}
	if enq.taskType != worker.TypeIntelFetchSource {
		t.Fatalf("task type = %q, want %q", enq.taskType, worker.TypeIntelFetchSource)
	}
}

func TestRecordUpdateNowHandler_MissingRecordAndSource(t *testing.T) {
	t.Parallel()

	enq := &stubTaskEnqueuer{}
	deps := &ToolDeps{TaskEnqueuer: enq}

	handler := RecordUpdateNowHandler(deps)
	tr, _ := handler(context.Background(), json.RawMessage(`{"record_id":""}`), pgtype.UUID{}, pgtype.UUID{})
	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
	if enq.called {
		t.Error("expected no task to be enqueued")
	}
}

func TestContentRegenerateFromSourceHandler(t *testing.T) {
	t.Parallel()

	enq := &stubTaskEnqueuer{}
	q := &stubIntelQuerier{record: gen.IntelRecord{}}
	deps := &ToolDeps{TaskEnqueuer: enq, IntelQuerier: q}

	handler := ContentRegenerateFromSourceHandler(deps)
	tr, err := handler(context.Background(), json.RawMessage(`{"content_id":"00000000-0000-0000-0000-000000000001","intel_record_id":"00000000-0000-0000-0000-000000000002"}`), pgtype.UUID{}, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}
	if !enq.called {
		t.Fatal("expected task to be enqueued")
	}
	if enq.taskType != worker.TypeContentRegenerateFromSource {
		t.Fatalf("task type = %q, want %q", enq.taskType, worker.TypeContentRegenerateFromSource)
	}
}

func TestContentRegenerateFromSourceHandler_MissingArgs(t *testing.T) {
	t.Parallel()

	enq := &stubTaskEnqueuer{}
	deps := &ToolDeps{TaskEnqueuer: enq}

	handler := ContentRegenerateFromSourceHandler(deps)
	tr, _ := handler(context.Background(), json.RawMessage(`{"content_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{}, pgtype.UUID{})
	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
	if enq.called {
		t.Error("expected no task to be enqueued")
	}
}

func TestContentRegenerateFromSourceHandler_EnqueueFailure(t *testing.T) {
	t.Parallel()

	enq := &stubTaskEnqueuer{returnErr: errors.New("queue full")}
	q := &stubIntelQuerier{record: gen.IntelRecord{}}
	deps := &ToolDeps{TaskEnqueuer: enq, IntelQuerier: q}

	handler := ContentRegenerateFromSourceHandler(deps)
	tr, _ := handler(context.Background(), json.RawMessage(`{"content_id":"00000000-0000-0000-0000-000000000001","intel_record_id":"00000000-0000-0000-0000-000000000002"}`), pgtype.UUID{}, pgtype.UUID{})
	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
}
