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

func (s *stubIntelSourceManager) Create(ctx context.Context, arg gen.CreateIntelSourceParams) (gen.IntelSource, error) {
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

func (s *stubIntelSourceManager) GetByURL(ctx context.Context, sourceURL string) (gen.IntelSource, error) {
	if src, ok := s.byURL[sourceURL]; ok {
		return src, nil
	}

	return gen.IntelSource{}, nil
}

type stubWebSearcher func(ctx context.Context, client *http.Client, settings SettingsReader, query string, limit int) (string, []WebSearchResult, error)

func (s stubWebSearcher) Search(ctx context.Context, client *http.Client, settings SettingsReader, query string, limit int) (string, []WebSearchResult, error) {
	return s(ctx, client, settings, query, limit)
}

type stubSettingsForDiscover struct {
	bools map[string]bool
}

func (stubSettingsForDiscover) GetStringOr(ctx context.Context, key, defaultValue string) string {
	return defaultValue
}

func (s stubSettingsForDiscover) GetBoolOr(ctx context.Context, key string, defaultValue bool) bool {
	if v, ok := s.bools[key]; ok {
		return v
	}

	return defaultValue
}

func (stubSettingsForDiscover) GetIntOr(ctx context.Context, key string, defaultValue int) int {
	return defaultValue
}

func (stubSettingsForDiscover) GetFloatOr(ctx context.Context, key string, defaultValue float64) float64 {
	return defaultValue
}

func (stubSettingsForDiscover) GetJSON(ctx context.Context, key string, defaultValue []byte) []byte {
	return defaultValue
}

func TestSourceDiscoverHandler(t *testing.T) {
	t.Parallel()

	manager := &stubIntelSourceManager{byURL: map[string]gen.IntelSource{}}
	searcher := func(ctx context.Context, client *http.Client, settings SettingsReader, query string, limit int) (string, []WebSearchResult, error) {
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

	tr, err := handler(t.Context(), json.RawMessage(`{"query":"scholarships Nigeria","category":"scholarship","limit":10}`), pgtype.UUID{})
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
	searcher := func(ctx context.Context, client *http.Client, settings SettingsReader, query string, limit int) (string, []WebSearchResult, error) {
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

	tr, _ := handler(t.Context(), json.RawMessage(`{"query":"scholarships"}`), pgtype.UUID{})
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

	tr, _ := handler(t.Context(), json.RawMessage(`{"query":"test"}`), pgtype.UUID{})
	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
}

func TestSourceDiscoverHandler_AutoEnableGate(t *testing.T) {
	t.Parallel()

	settings := stubSettingsForDiscover{
		bools: map[string]bool{"agent.auto_enable_discovered_sources": true},
	}
	searcher := func(ctx context.Context, client *http.Client, s SettingsReader, query string, limit int) (string, []WebSearchResult, error) {
		return "test", []WebSearchResult{{Title: "Source", URL: "https://example.com/feed"}}, nil
	}

	newDeps := func(perms *stubPerms) (*ToolDeps, *stubIntelSourceManager) {
		manager := &stubIntelSourceManager{byURL: map[string]gen.IntelSource{}}

		return &ToolDeps{
			IntelSourceManager: manager,
			Settings:           settings,
			HTTPClient:         http.DefaultClient,
			SearchWebFunc:      searcher,
			Permission:         perms,
		}, manager
	}

	args := json.RawMessage(`{"query":"jobs Lagos","category":"job"}`)

	// Setting on but no intel.sources.manage — member-level callers still
	// get proposed rows; the setting is not a privilege escalation.
	deps, manager := newDeps(&stubPerms{granted: map[string]bool{}})

	tr, err := SourceDiscoverHandler(deps)(t.Context(), args, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q: %s", tr.Status, tr.Content)
	}

	if len(manager.created) != 1 || manager.created[0].Status != "proposed" || manager.created[0].Enabled {
		t.Fatalf("without intel.sources.manage expected proposed+disabled, got %+v", manager.created)
	}

	// Setting on AND intel.sources.manage — the caller could approve the
	// proposal anyway, so direct activation is allowed.
	deps, manager = newDeps(&stubPerms{granted: map[string]bool{"intel.sources.manage": true}})

	tr, err = SourceDiscoverHandler(deps)(t.Context(), args, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q: %s", tr.Status, tr.Content)
	}

	if len(manager.created) != 1 || manager.created[0].Status != "active" || !manager.created[0].Enabled {
		t.Fatalf("with intel.sources.manage expected active+enabled, got %+v", manager.created)
	}
}
