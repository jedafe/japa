package roadmap

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store provides low-level database access for roadmap operations.
// The Service layer wraps Store methods with business logic.
// Ref: japaconnect-engineering/sections/113-public-product-roadmap.md
type Store struct {
	write *gen.Queries
	read  *gen.Queries
}

// NewStore creates a Store backed by the given gen.Queries instances.
func NewStore(read, write *gen.Queries) *Store {
	return &Store{read: read, write: write}
}

// ---------------------------------------------------------------------------
// Roadmap items
// ---------------------------------------------------------------------------

func (s *Store) createItem(ctx context.Context, p gen.CreateRoadmapItemParams) (gen.RoadmapItem, error) {
	row, err := s.write.CreateRoadmapItem(ctx, p)
	if err != nil {
		return gen.RoadmapItem{}, fmt.Errorf("creating roadmap item: %w", err)
	}

	return row, nil
}

func (s *Store) getItemByID(ctx context.Context, id pgtype.UUID) (gen.RoadmapItem, error) {
	row, err := s.read.GetRoadmapItemByID(ctx, id)
	if err != nil {
		return gen.RoadmapItem{}, fmt.Errorf("getting roadmap item by ID: %w", err)
	}

	return row, nil
}

func (s *Store) getItemBySlug(ctx context.Context, slug string, viewerID pgtype.UUID) (gen.GetRoadmapItemBySlugRow, error) {
	row, err := s.read.GetRoadmapItemBySlug(ctx, gen.GetRoadmapItemBySlugParams{
		ViewerID: viewerID, // zero-value UUID → NULL → anonymous viewer
		Slug:     slug,
	})
	if err != nil {
		return gen.GetRoadmapItemBySlugRow{}, fmt.Errorf("getting roadmap item by slug: %w", err)
	}

	return row, nil
}

func (s *Store) listPublicItems(ctx context.Context, viewerID pgtype.UUID, categorySlug *string, shippedWindowDays int32) ([]gen.ListPublicRoadmapItemsRow, error) {
	// The query's no-filter sentinel is an empty string
	// (sqlc.narg(category_slug)::text = ''); a NULL would make the whole
	// predicate NULL and silently drop every row.
	slug := ""
	if categorySlug != nil {
		slug = *categorySlug
	}

	rows, err := s.read.ListPublicRoadmapItems(ctx, gen.ListPublicRoadmapItemsParams{
		ViewerID:          viewerID,
		Statuses:          PublicStatuses,
		CategorySlug:      &slug,
		ShippedWindowDays: shippedWindowDays,
	})
	if err != nil {
		return nil, fmt.Errorf("listing public roadmap items: %w", err)
	}

	return rows, nil
}

func (s *Store) listChangelog(ctx context.Context) ([]gen.ListRoadmapChangelogRow, error) {
	rows, err := s.read.ListRoadmapChangelog(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing roadmap changelog: %w", err)
	}

	return rows, nil
}

func (s *Store) listAdminItems(ctx context.Context, f AdminListFilter) (AdminListResponse, error) {
	limit := f.PageSize
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	// Empty string is the no-filter sentinel for these params (the queries
	// compare sqlc.narg(...)::text = ''); NULLs would make the predicates
	// NULL and silently drop every row.
	status, search, sort := f.Status, f.Search, f.Sort

	rows, err := s.read.ListAdminRoadmapItems(ctx, gen.ListAdminRoadmapItemsParams{
		Status:         &status,
		CategoryID:     f.CategoryID,
		Search:         &search,
		IncludeDeleted: f.IncludeDeleted,
		Sort:           &sort,
		PageOffset:     f.PageOffset,
		PageLimit:      limit,
	})
	if err != nil {
		return AdminListResponse{}, fmt.Errorf("listing admin roadmap items: %w", err)
	}

	total, err := s.read.CountAdminRoadmapItems(ctx, gen.CountAdminRoadmapItemsParams{
		Status:         &status,
		CategoryID:     f.CategoryID,
		Search:         &search,
		IncludeDeleted: f.IncludeDeleted,
	})
	if err != nil {
		return AdminListResponse{}, fmt.Errorf("counting admin roadmap items: %w", err)
	}

	totalPages := int32(0)
	if total > 0 && limit > 0 {
		totalPages = int32((total + int64(limit) - 1) / int64(limit)) // #nosec
	}

	items := rows
	if items == nil {
		items = make([]gen.ListAdminRoadmapItemsRow, 0)
	}

	return AdminListResponse{
		Items:      items,
		Total:      total,
		Page:       f.PageOffset / limit,
		TotalPages: totalPages,
	}, nil
}

func (s *Store) updateItem(ctx context.Context, p gen.UpdateRoadmapItemParams) (gen.RoadmapItem, error) {
	row, err := s.write.UpdateRoadmapItem(ctx, p)
	if err != nil {
		return gen.RoadmapItem{}, fmt.Errorf("updating roadmap item: %w", err)
	}

	return row, nil
}

func (s *Store) updateItemStatus(ctx context.Context, id pgtype.UUID, status string) (gen.RoadmapItem, error) {
	row, err := s.write.UpdateRoadmapItemStatus(ctx, gen.UpdateRoadmapItemStatusParams{
		ID:     id,
		Status: status,
	})
	if err != nil {
		return gen.RoadmapItem{}, fmt.Errorf("updating roadmap item status: %w", err)
	}

	return row, nil
}

func (s *Store) setItemContent(ctx context.Context, itemID, contentID pgtype.UUID) error {
	if err := s.write.SetRoadmapItemContent(ctx, gen.SetRoadmapItemContentParams{
		ID:        itemID,
		ContentID: contentID,
	}); err != nil {
		return fmt.Errorf("linking roadmap item content: %w", err)
	}

	return nil
}

func (s *Store) softDeleteItem(ctx context.Context, id pgtype.UUID) error {
	if err := s.write.SoftDeleteRoadmapItem(ctx, id); err != nil {
		return fmt.Errorf("soft-deleting roadmap item: %w", err)
	}

	return nil
}

func (s *Store) restoreItem(ctx context.Context, id pgtype.UUID) error {
	if err := s.write.RestoreRoadmapItem(ctx, id); err != nil {
		return fmt.Errorf("restoring roadmap item: %w", err)
	}

	return nil
}

func (s *Store) hardDeleteItem(ctx context.Context, id pgtype.UUID) error {
	if err := s.write.HardDeleteRoadmapItem(ctx, id); err != nil {
		return fmt.Errorf("hard-deleting roadmap item: %w", err)
	}

	return nil
}

func (s *Store) updateShadowContent(ctx context.Context, p gen.UpdateRoadmapShadowContentParams) error {
	if err := s.write.UpdateRoadmapShadowContent(ctx, p); err != nil {
		return fmt.Errorf("updating roadmap shadow content: %w", err)
	}

	return nil
}

func (s *Store) reorderItems(ctx context.Context, ids []pgtype.UUID, orders []int32) error {
	if err := s.write.ReorderRoadmapItems(ctx, gen.ReorderRoadmapItemsParams{
		Ids:    ids,
		Orders: orders,
	}); err != nil {
		return fmt.Errorf("reordering roadmap items: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Roadmap categories
// ---------------------------------------------------------------------------

func (s *Store) listCategories(ctx context.Context) ([]gen.RoadmapCategory, error) {
	rows, err := s.read.ListRoadmapCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing roadmap categories: %w", err)
	}

	return rows, nil
}

func (s *Store) listAdminCategories(ctx context.Context) ([]gen.RoadmapCategory, error) {
	rows, err := s.read.ListAdminRoadmapCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing admin roadmap categories: %w", err)
	}

	return rows, nil
}

func (s *Store) getCategoryByID(ctx context.Context, id pgtype.UUID) (gen.RoadmapCategory, error) {
	row, err := s.read.GetRoadmapCategoryByID(ctx, id)
	if err != nil {
		return gen.RoadmapCategory{}, fmt.Errorf("getting roadmap category by ID: %w", err)
	}

	return row, nil
}

func (s *Store) createCategory(ctx context.Context, p gen.CreateRoadmapCategoryParams) (gen.RoadmapCategory, error) {
	row, err := s.write.CreateRoadmapCategory(ctx, p)
	if err != nil {
		return gen.RoadmapCategory{}, fmt.Errorf("creating roadmap category: %w", err)
	}

	return row, nil
}

func (s *Store) updateCategory(ctx context.Context, p gen.UpdateRoadmapCategoryParams) (gen.RoadmapCategory, error) {
	row, err := s.write.UpdateRoadmapCategory(ctx, p)
	if err != nil {
		return gen.RoadmapCategory{}, fmt.Errorf("updating roadmap category: %w", err)
	}

	return row, nil
}

func (s *Store) softDeleteCategory(ctx context.Context, id pgtype.UUID) error {
	if err := s.write.SoftDeleteRoadmapCategory(ctx, id); err != nil {
		return fmt.Errorf("soft-deleting roadmap category: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Roadmap votes
// ---------------------------------------------------------------------------

// tryCreateVote inserts a vote row, returning pgx.ErrNoRows when the
// (item_id, user_id) pair already exists (ON CONFLICT DO NOTHING).
func (s *Store) tryCreateVote(ctx context.Context, itemID, userID pgtype.UUID) error {
	_, err := s.write.TryCreateRoadmapVote(ctx, gen.TryCreateRoadmapVoteParams{
		ItemID: itemID,
		UserID: userID,
	})
	if err != nil {
		return fmt.Errorf("creating roadmap vote: %w", err)
	}

	return nil
}

// deleteVote removes a vote row and returns the number of rows affected —
// 0 means the user had not voted.
func (s *Store) deleteVote(ctx context.Context, itemID, userID pgtype.UUID) (int64, error) {
	rows, err := s.write.DeleteRoadmapVote(ctx, gen.DeleteRoadmapVoteParams{
		ItemID: itemID,
		UserID: userID,
	})
	if err != nil {
		return 0, fmt.Errorf("deleting roadmap vote: %w", err)
	}

	return rows, nil
}

// syncVoteCount rewrites vote_count from a live recount — self-healing
// rather than increment/decrement drift (ADR-127).
func (s *Store) syncVoteCount(ctx context.Context, itemID pgtype.UUID) error {
	if err := s.write.SyncRoadmapVoteCount(ctx, itemID); err != nil {
		return fmt.Errorf("syncing roadmap vote count: %w", err)
	}

	return nil
}

//nolint:unused // consumed by the roadmap:ship_notify worker task (§11 catalog)
func (s *Store) listVoterIDs(ctx context.Context, itemID pgtype.UUID) ([]pgtype.UUID, error) {
	rows, err := s.read.ListRoadmapVoterIDs(ctx, itemID)
	if err != nil {
		return nil, fmt.Errorf("listing roadmap voter IDs: %w", err)
	}

	return rows, nil
}

// ---------------------------------------------------------------------------
// Shadow contents row (§49 comments backing)
// ---------------------------------------------------------------------------

func (s *Store) createShadowContent(ctx context.Context, p gen.CreateRoadmapShadowContentParams) (pgtype.UUID, error) {
	id, err := s.write.CreateRoadmapShadowContent(ctx, p)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("creating roadmap shadow content: %w", err)
	}

	return id, nil
}
