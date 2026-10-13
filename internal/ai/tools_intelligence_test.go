package ai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
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
	source  gen.IntelSource
	record  gen.IntelRecord
	records []gen.ListIntelRecordsForAgentRow
	err     error
}

func (s *stubIntelQuerier) GetIntelSource(ctx context.Context, id pgtype.UUID) (gen.IntelSource, error) {
	if s.err != nil {
		return gen.IntelSource{}, s.err
	}

	return s.source, nil
}

func (s *stubIntelQuerier) GetIntelRecord(ctx context.Context, id pgtype.UUID) (gen.IntelRecord, error) {
	if s.err != nil {
		return gen.IntelRecord{}, s.err
	}

	return s.record, nil
}

func (s *stubIntelQuerier) GetIntelSourceByURL(ctx context.Context, sourceUrl string) (gen.IntelSource, error) {
	if s.err != nil {
		return gen.IntelSource{}, s.err
	}

	return s.source, nil
}

func (s *stubIntelQuerier) ListIntelRecordsForAgent(ctx context.Context, arg gen.ListIntelRecordsForAgentParams) ([]gen.ListIntelRecordsForAgentRow, error) {
	if s.err != nil {
		return nil, s.err
	}

	return s.records, nil
}

func TestSourceCrawlNowHandler(t *testing.T) {
	t.Parallel()

	enq := &stubTaskEnqueuer{}
	q := &stubIntelQuerier{source: gen.IntelSource{Enabled: true}}
	deps := &ToolDeps{TaskEnqueuer: enq, IntelQuerier: q}

	handler := SourceCrawlNowHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(`{"source_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{})
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

	tr, _ := handler(t.Context(), json.RawMessage(`{"source_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{})
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

	tr, err := handler(t.Context(), json.RawMessage(`{"record_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{})
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

	tr, _ := handler(t.Context(), json.RawMessage(`{"record_id":""}`), pgtype.UUID{})
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

	tr, err := handler(t.Context(), json.RawMessage(`{"content_id":"00000000-0000-0000-0000-000000000001","intel_record_id":"00000000-0000-0000-0000-000000000002"}`), pgtype.UUID{})
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

	tr, _ := handler(t.Context(), json.RawMessage(`{"content_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{})
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

	tr, _ := handler(t.Context(), json.RawMessage(`{"content_id":"00000000-0000-0000-0000-000000000001","intel_record_id":"00000000-0000-0000-0000-000000000002"}`), pgtype.UUID{})
	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
}

func TestIntelRecordGetHandler(t *testing.T) {
	t.Parallel()

	q := &stubIntelQuerier{record: gen.IntelRecord{
		Status:            "extracted",
		EntityType:        "scholarship",
		Extracted:         []byte(`{"title":"Chevening"}`),
		ConfidenceOverall: 0.9,
	}}
	deps := &ToolDeps{IntelQuerier: q}

	tr, err := IntelRecordGetHandler(deps)(t.Context(), json.RawMessage(`{"record_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}
	if !strings.Contains(tr.Content, `"extracted"`) || !strings.Contains(tr.Content, "Chevening") {
		t.Fatalf("expected extracted payload in result, got %s", tr.Content)
	}
}

func TestIntelRecordGetHandler_MissingID(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{IntelQuerier: &stubIntelQuerier{}}

	tr, _ := IntelRecordGetHandler(deps)(t.Context(), json.RawMessage(`{}`), pgtype.UUID{})
	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
}

func TestIntelRecordListHandler(t *testing.T) {
	t.Parallel()

	q := &stubIntelQuerier{records: []gen.ListIntelRecordsForAgentRow{
		{EntityType: "job", Status: "published"},
		{EntityType: "job", Status: "review_required"},
	}}
	deps := &ToolDeps{IntelQuerier: q}

	tr, err := IntelRecordListHandler(deps)(t.Context(), json.RawMessage(`{"status":"published","limit":10}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}
	if !strings.Contains(tr.Content, `"count":2`) {
		t.Fatalf("expected 2 records in result, got %s", tr.Content)
	}
	// The list surface must omit the heavy extraction payloads.
	if strings.Contains(tr.Content, `"extracted"`) {
		t.Fatalf("list result must not contain extracted payloads: %s", tr.Content)
	}
}

func TestIntelSourceGetHandler(t *testing.T) {
	t.Parallel()

	q := &stubIntelQuerier{source: gen.IntelSource{
		Name:       "Example Feed",
		Status:     "active",
		Enabled:    true,
		TrustLevel: 2,
	}}
	deps := &ToolDeps{IntelQuerier: q}

	tr, err := IntelSourceGetHandler(deps)(t.Context(), json.RawMessage(`{"source_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}
	if !strings.Contains(tr.Content, "Example Feed") {
		t.Fatalf("expected source name in result, got %s", tr.Content)
	}
}
