package sources

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
)

// Service applies the business logic for the intel_sources registry. It
// wraps a Store and is the entry point used by HTTP/admin handlers and the
// scheduler. Methods here are intentionally thin: validation that belongs
// at the domain layer lives here, while persistence is delegated to the
// Store.
type Service struct {
	store *Store
}

// NewService creates a sources service backed by the given store.
func NewService(store *Store) *Service {
	return &Service{store: store}
}

// Create registers a new source in the registry. Defaults status to 'active'
// when the caller did not supply a status (e.g. admin creation flow).
func (s *Service) Create(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateIntelSourceParams) (Source, error) {
	if arg.Status == nil || arg.Status == "" {
		arg.Status = "active"
	}
	return s.store.Create(ctx, tenantID, arg)
}

// Get retrieves a source by ID.
func (s *Service) Get(ctx context.Context, tenantID pgtype.UUID, id SourceID) (Source, error) {
	return s.store.Get(ctx, tenantID, id)
}

// GetByURL retrieves a source by its canonical source_url.
func (s *Service) GetByURL(ctx context.Context, tenantID pgtype.UUID, sourceURL string) (Source, error) {
	return s.store.GetByURL(ctx, tenantID, sourceURL)
}

// List returns a page of sources.
func (s *Service) List(ctx context.Context, tenantID pgtype.UUID, arg gen.ListIntelSourcesParams) ([]Source, error) {
	return s.store.List(ctx, tenantID, arg)
}

// ListByCategory returns enabled sources for a given category.
func (s *Service) ListByCategory(ctx context.Context, tenantID pgtype.UUID, category Category) ([]Source, error) {
	return s.store.ListByCategory(ctx, tenantID, string(category))
}

// ListDue returns up to limit sources whose crawl interval has elapsed.
func (s *Service) ListDue(ctx context.Context, tenantID pgtype.UUID, limit int32) ([]Source, error) {
	return s.store.ListDue(ctx, tenantID, limit)
}

// Enable re-enables a previously disabled source so future crawls resume.
func (s *Service) Enable(ctx context.Context, tenantID pgtype.UUID, id SourceID) error {
	return s.store.UpdateEnabled(ctx, tenantID, id, true)
}

// Disable disables a source: future crawls stop, but existing promoted
// records are preserved.
func (s *Service) Disable(ctx context.Context, tenantID pgtype.UUID, id SourceID) error {
	return s.store.UpdateEnabled(ctx, tenantID, id, false)
}

// Delete removes a source from the registry.
func (s *Service) Delete(ctx context.Context, tenantID pgtype.UUID, id SourceID) error {
	return s.store.Delete(ctx, tenantID, id)
}

// Update edits the configurable fields of a source (name, domain, url,
// category, trust level, crawl settings, license, usage restriction, enabled
// toggle). Returns the updated source row.
func (s *Service) Update(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelSourceParams) (Source, error) {
	return s.store.Update(ctx, tenantID, arg)
}

// CrawlNow is a no-op service method retained for API surface compatibility.
// The actual crawl-now trigger is handled by the HTTP handler which enqueues
// the intel:fetch_source Asynq task directly (INTEL-011 fix). This method
// verifies the source exists and is enabled.
func (s *Service) CrawlNow(ctx context.Context, tenantID pgtype.UUID, id SourceID) error {
	src, err := s.store.Get(ctx, tenantID, id)
	if err != nil {
		return fmt.Errorf("sources: crawl-now: load source: %w", err)
	}
	if !src.Enabled {
		return fmt.Errorf("sources: crawl-now: source is disabled")
	}
	return nil
}
