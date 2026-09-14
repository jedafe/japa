package ai

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

type stubScholarshipSearcher struct {
	called  bool
	filters ScholarshipSearchFilters
	results []ScholarshipSearchResult
	total   int64
	err     error
}

func (s *stubScholarshipSearcher) FindScholarships(ctx context.Context, filters ScholarshipSearchFilters) ([]ScholarshipSearchResult, int64, error) {
	s.called = true
	s.filters = filters

	if s.err != nil {
		return nil, 0, s.err
	}

	return s.results, s.total, nil
}

func TestScholarshipSearchHandler(t *testing.T) {
	t.Parallel()

	stub := &stubScholarshipSearcher{
		results: []ScholarshipSearchResult{
			{
				ID:                  "00000000-0000-0000-0000-000000000001",
				GuideID:             "00000000-0000-0000-0000-000000000002",
				IntelRecordID:       "00000000-0000-0000-0000-000000000003",
				Title:               "Nigerian Scholarship",
				Country:             "NGA",
				LevelOfStudy:        "Masters",
				ApplicationURL:      "https://example.com/apply",
				ApplicationDeadline: "2026-12-01",
			},
		},
	}
	deps := &ToolDeps{ScholarshipSearcher: stub}

	handler := ScholarshipSearchHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(`{"country":"NGA","level":"Masters","limit":5}`), pgtype.UUID{}, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}

	if !stub.called {
		t.Fatal("expected scholarship search to be called")
	}

	if stub.filters.Country != "NGA" {
		t.Fatalf("country = %q, want NGA", stub.filters.Country)
	}

	if stub.filters.Limit != 5 {
		t.Fatalf("limit = %d, want 5", stub.filters.Limit)
	}
}

func TestScholarshipSearchHandler_NoDeps(t *testing.T) {
	t.Parallel()

	handler := ScholarshipSearchHandler(&ToolDeps{})

	tr, _ := handler(t.Context(), json.RawMessage(`{"country":"NGA"}`), pgtype.UUID{}, pgtype.UUID{})
	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
}
