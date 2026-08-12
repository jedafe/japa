package sources

import (
	"context"
	"fmt"

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

// Create registers a new source in the registry.
func (s *Service) Create(ctx context.Context, arg gen.CreateIntelSourceParams) (Source, error) {
	return s.store.Create(ctx, arg)
}

// Get retrieves a source by ID.
func (s *Service) Get(ctx context.Context, id SourceID) (Source, error) {
	return s.store.Get(ctx, id)
}

// List returns a page of sources.
func (s *Service) List(ctx context.Context, arg gen.ListIntelSourcesParams) ([]Source, error) {
	return s.store.List(ctx, arg)
}

// ListByCategory returns enabled sources for a given category.
func (s *Service) ListByCategory(ctx context.Context, category Category) ([]Source, error) {
	return s.store.ListByCategory(ctx, string(category))
}

// ListDue returns up to limit sources whose crawl interval has elapsed.
func (s *Service) ListDue(ctx context.Context, limit int32) ([]Source, error) {
	return s.store.ListDue(ctx, limit)
}

// Enable re-enables a previously disabled source so future crawls resume.
func (s *Service) Enable(ctx context.Context, id SourceID) error {
	return s.store.UpdateEnabled(ctx, id, true)
}

// Disable disables a source: future crawls stop, but existing promoted
// records are preserved.
func (s *Service) Disable(ctx context.Context, id SourceID) error {
	return s.store.UpdateEnabled(ctx, id, false)
}

// Delete removes a source from the registry.
func (s *Service) Delete(ctx context.Context, id SourceID) error {
	return s.store.Delete(ctx, id)
}

// CrawlNow would enqueue an intel:fetch_source task for the source. The
// Asynq wiring lands in Task 22; until then this is a stub so the service
// surface is complete and callers can be written against it.
//
// TODO(task-22): wire Asynq enqueue of intel:fetch_source here.
func (s *Service) CrawlNow(ctx context.Context, id SourceID) error {
	_ = ctx
	_ = id
	return fmt.Errorf("sources: crawl-now not wired yet (Task 22)")
}
