package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

type stubIntelSourceManager struct {
	created []gen.IntelSource
	byURL   map[string]gen.IntelSource
}

func (s *stubIntelSourceManager) Create(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateIntelSourceParams) (gen.IntelSource, error) {
	src := gen.IntelSource{
		ID:         pgtype.UUID{Valid: true},
		Name:       arg.Name,
		Domain:     arg.Domain,
		SourceUrl:  arg.SourceUrl,
		Category:   arg.Category,
		Status:     arg.Status.(string),
		Enabled:    arg.Enabled,
		ProposedBy: arg.ProposedBy,
	}
	if src.Status == "" {
		src.Status = "proposed"
	}
	s.created = append(s.created, src)
	return src, nil
}

func (s *stubIntelSourceManager) GetByURL(ctx context.Context, tenantID pgtype.UUID, sourceURL string) (gen.IntelSource, error) {
	if src, ok := s.byURL[sourceURL]; ok {
		return src, nil
	}
	return gen.IntelSource{}, nil
}

type stubWebSearcher func(ctx context.Context, client *http.Client, settings SettingsReader, tenantID pgtype.UUID, query string, limit int) (string, []WebSearchResult, error)

func (s stubWebSearcher) Search(ctx context.Context, client *http.Client, settings SettingsReader, tenantID pgtype.UUID, query string, limit int) (string, []WebSearchResult, error) {
	return s(ctx, client, settings, tenantID, query, limit)
}

type stubSettingsForDiscover struct{}

func (stubSettingsForDiscover) GetStringOr(ctx context.Context, tenantID pgtype.UUID, key, defaultValue string) string {
	return defaultValue
}
func (stubSettingsForDiscover) GetBoolOr(ctx context.Context, tenantID pgtype.UUID, key string, defaultValue bool) bool {
	return defaultValue
}
func (stubSettingsForDiscover) GetIntOr(ctx context.Context, tenantID pgtype.UUID, key string, defaultValue int) int {
	return defaultValue
}
func (stubSettingsForDiscover) GetFloatOr(ctx context.Context, tenantID pgtype.UUID, key string, defaultValue float64) float64 {
	return defaultValue
}
func (stubSettingsForDiscover) GetJSON(ctx context.Context, tenantID pgtype.UUID, key string, defaultValue []byte) []byte {
	return defaultValue
}

func TestSourceDiscoverHandler(t *testing.T) {
	t.Parallel()

	manager := &stubIntelSourceManager{byURL: map[string]gen.IntelSource{}}
	searcher := func(ctx context.Context, client *http.Client, settings SettingsReader, tenantID pgtype.UUID, query string, limit int) (string, []WebSearchResult, error) {
		return "test", []WebSearchResult{
			{Title: "Example Scholarship", URL: "https://example.com/scholarship"},
			{Title: "Another Scholarship", URL: "https://example.org/another"},
		}, nil
	}

	deps := &ToolDeps{
		IntelSourceManager: manager,
		Settings:           stubSettingsForDiscover{},
		HTTPClient:         http.DefaultClient,
		SearchWebFunc:      searcher,
	}

	handler := SourceDiscoverHandler(deps)
	tr, err := handler(context.Background(), json.RawMessage(`{"query":"scholarships Nigeria","category":"scholarship","limit":10}`), pgtype.UUID{}, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success: %s", tr.Status, tr.Content)
	}
	if len(manager.created) != 2 {
		t.Fatalf("created %d sources, want 2", len(manager.created))
	}
	for _, s := range manager.created {
		if s.Status != "proposed" {
			t.Fatalf("status = %q, want proposed", s.Status)
		}
		if s.Enabled {
			t.Fatal("proposed source should not be enabled")
		}
	}
}

func TestSourceDiscoverHandler_Duplicate(t *testing.T) {
	t.Parallel()

	manager := &stubIntelSourceManager{
		byURL: map[string]gen.IntelSource{
			"https://example.com/existing": {
				ID:        pgtype.UUID{Valid: true},
				Name:      "Existing",
				Domain:    "example.com",
				SourceUrl: "https://example.com/existing",
				Category:  "scholarship",
				Status:    "active",
				Enabled:   true,
			},
		},
	}
	searcher := func(ctx context.Context, client *http.Client, settings SettingsReader, tenantID pgtype.UUID, query string, limit int) (string, []WebSearchResult, error) {
		return "test", []WebSearchResult{
			{Title: "Existing", URL: "https://example.com/existing"},
			{Title: "New", URL: "https://example.com/new"},
		}, nil
	}

	deps := &ToolDeps{
		IntelSourceManager: manager,
		Settings:           stubSettingsForDiscover{},
		HTTPClient:         http.DefaultClient,
		SearchWebFunc:      searcher,
	}

	handler := SourceDiscoverHandler(deps)
	tr, _ := handler(context.Background(), json.RawMessage(`{"query":"scholarships"}`), pgtype.UUID{}, pgtype.UUID{})
	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}
	if len(manager.created) != 1 {
		t.Fatalf("created %d sources, want 1", len(manager.created))
	}
}

func TestSourceDiscoverHandler_NoDeps(t *testing.T) {
	t.Parallel()

	handler := SourceDiscoverHandler(&ToolDeps{})
	tr, _ := handler(context.Background(), json.RawMessage(`{"query":"test"}`), pgtype.UUID{}, pgtype.UUID{})
	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
}
