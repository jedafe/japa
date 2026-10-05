// Package roadmap — service layer for the public product roadmap (§113).
//
// Board reads are cached in Valkey (jc:roadmap:board); every item,
// category, or vote mutation deletes the key. Vote counts self-heal via a
// recount query after each mutation (ADR-127). Each published item owns a
// hidden contents row (content_type='roadmap') so §49 comments work
// unchanged.
package roadmap

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	jcvalkey "github.com/jedafe/japa/internal/valkey"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// Enqueuer is the interface for enqueuing background tasks.
type Enqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// Service provides roadmap operations with reader/writer pool separation.
type Service struct {
	store    *Store
	db       *db.DB
	enqueuer Enqueuer
	settings *config.Settings
	vk       valkeycompat.Cmdable
}

// boardCacheKey is the Valkey key for the JSON-serialized public board.
// Ref: conventions/valkey-keys-catalog.md (jc:roadmap:board).
const boardCacheKey = "jc:roadmap:board"

// taskTypeShipNotify is the Asynq task enqueued fire-and-forget when an
// item transitions to shipped (Rule 8). Registered in the §11 task
// catalog; the worker handler is wired in internal/worker.
// Ref: sections/11-job-queue-asynq.md (roadmap:ship_notify).
const taskTypeShipNotify = "roadmap:ship_notify"

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{
		store: &Store{
			read:  gen.New(database.Reader),
			write: gen.New(database.Writer),
		},
		db:       database,
		enqueuer: &noopEnqueuer{},
	}
}

// NewServiceWithEnqueuer creates a Service with a custom enqueuer for background tasks.
func NewServiceWithEnqueuer(database *db.DB, enqueuer Enqueuer) *Service {
	svc := NewService(database)
	svc.enqueuer = enqueuer

	return svc
}

// SetSettings injects the config.Settings provider for hot-reloadable
// roadmap settings (roadmap.submissions_enabled,
// roadmap.board_cache_ttl_seconds, roadmap.shipped_window_days).
func (s *Service) SetSettings(settings *config.Settings) { s.settings = settings }

// SetValkey injects the Valkey client used for the board cache. Nil
// disables caching (degraded mode — every read hits the database).
func (s *Service) SetValkey(vk valkeycompat.Cmdable) { s.vk = vk }

// noopEnqueuer is a no-op implementation for development.
type noopEnqueuer struct{}

func (n *noopEnqueuer) Enqueue(_ string, _ []byte, _ ...interface{}) error {
	return nil
}

// ---------------------------------------------------------------------------
// Settings helpers — all values are hot-reloadable via system_settings (Rule 3)
// ---------------------------------------------------------------------------

func (s *Service) settingsInt(ctx context.Context, key string, fallback int) int {
	if s.settings == nil {
		return fallback
	}

	return s.settings.GetIntOr(ctx, key, fallback)
}

func (s *Service) settingsBool(ctx context.Context, key string, fallback bool) bool {
	if s.settings == nil {
		return fallback
	}

	return s.settings.GetBoolOr(ctx, key, fallback)
}

// ---------------------------------------------------------------------------
// Public board
// ---------------------------------------------------------------------------

// GetBoard returns the public board payload: items grouped into status
// columns plus the category list. The unfiltered, anonymous payload is
// cached under jc:roadmap:board with TTL roadmap.board_cache_ttl_seconds
// (default 60s). Category-filtered views and authenticated viewers bypass
// the cache — has_voted is viewer-specific, so caching it under the shared
// key would leak one user's vote state to another.
func (s *Service) GetBoard(ctx context.Context, viewerID pgtype.UUID, categorySlug string) (*Board, error) {
	ttl := time.Duration(s.settingsInt(ctx, "roadmap.board_cache_ttl_seconds", 60)) * time.Second
	window := int32(s.settingsInt(ctx, "roadmap.shipped_window_days", 90)) // #nosec

	fetch := func(ctx context.Context) (*Board, error) {
		var catFilter *string
		if categorySlug != "" {
			catFilter = &categorySlug
		}

		rows, err := s.store.listPublicItems(ctx, viewerID, catFilter, window)
		if err != nil {
			return nil, err
		}

		cats, err := s.store.listCategories(ctx)
		if err != nil {
			return nil, err
		}

		return buildBoard(rows, cats), nil
	}

	// Only the anonymous, unfiltered board is cached — see the doc comment.
	if categorySlug == "" && !viewerID.Valid && s.vk != nil {
		return jcvalkey.GetOrComputeCompat(ctx, s.vk, boardCacheKey, ttl, fetch)
	}

	return fetch(ctx)
}

// buildBoard groups public rows into the ordered status columns and maps
// categories to their DTO shape.
func buildBoard(rows []gen.ListPublicRoadmapItemsRow, cats []gen.RoadmapCategory) *Board {
	byStatus := make(map[string][]BoardItem, len(boardColumnOrder))
	for _, r := range rows {
		byStatus[r.Status] = append(byStatus[r.Status], boardItemFromRow(r))
	}

	columns := make([]BoardColumn, 0, len(boardColumnOrder))
	for _, def := range boardColumnOrder {
		items := byStatus[string(def.Status)]
		if items == nil {
			items = make([]BoardItem, 0)
		}

		columns = append(columns, BoardColumn{
			Status: string(def.Status),
			Label:  def.Label,
			Items:  items,
		})
	}

	categories := make([]CategoryDTO, 0, len(cats))
	for _, c := range cats {
		categories = append(categories, CategoryDTO{
			ID:          c.ID,
			Slug:        c.Slug,
			Name:        c.Name,
			Description: c.Description,
			Color:       c.Color,
			SortOrder:   c.SortOrder,
		})
	}

	return &Board{Columns: columns, Categories: categories}
}

func boardItemFromRow(r gen.ListPublicRoadmapItemsRow) BoardItem {
	var contentID *pgtype.UUID
	if r.ContentID.Valid {
		c := r.ContentID
		contentID = &c
	}

	var shippedAt *time.Time
	if r.ShippedAt.Valid {
		t := r.ShippedAt.Time
		shippedAt = &t
	}

	return BoardItem{
		ID:             r.ID,
		Slug:           r.Slug,
		Title:          r.Title,
		Summary:        r.Summary,
		Status:         r.Status,
		CategoryName:   r.CategoryName,
		CategorySlug:   r.CategorySlug,
		CategoryColor:  r.CategoryColor,
		VoteCount:      r.VoteCount,
		HasVoted:       r.HasVoted,
		IsFlagship:     r.IsFlagship,
		TargetWindow:   r.TargetWindow,
		FeatureFlagKey: r.FeatureFlagKey,
		ContentID:      contentID,
		ShippedAt:      shippedAt,
		CreatedAt:      r.CreatedAt.Time,
	}
}

// GetItem returns the public detail for one item by slug. Items whose
// status is not publicly visible (pending, declined, archived) are
// indistinguishable from missing items — the service returns not_found so
// pending submissions cannot be discovered by slug guessing (§113).
func (s *Service) GetItem(ctx context.Context, slug string, viewerID pgtype.UUID) (*ItemDetail, error) {
	row, err := s.store.getItemBySlug(ctx, slug, viewerID)
	if err != nil {
		return nil, err // pgx.ErrNoRows → 404 via HandleServiceError
	}

	if !IsPublicStatus(ItemStatus(row.Status)) {
		return nil, fmt.Errorf("%w: roadmap item %q is not public", ErrNotFound, slug)
	}

	item := boardItemFromRow(gen.ListPublicRoadmapItemsRow{
		ID:             row.ID,
		Title:          row.Title,
		Slug:           row.Slug,
		Summary:        row.Summary,
		CategoryID:     row.CategoryID,
		Status:         row.Status,
		IsFlagship:     row.IsFlagship,
		TargetWindow:   row.TargetWindow,
		FeatureFlagKey: row.FeatureFlagKey,
		ContentID:      row.ContentID,
		VoteCount:      row.VoteCount,
		ShippedAt:      row.ShippedAt,
		CreatedAt:      row.CreatedAt,
		CategoryName:   row.CategoryName,
		CategorySlug:   row.CategorySlug,
		CategoryColor:  row.CategoryColor,
		HasVoted:       row.HasVoted,
	})

	return &ItemDetail{BoardItem: item, Body: row.Body}, nil
}

// GetChangelog returns all shipped items ordered by shipped_at DESC —
// the release-notes archive is unbounded; the roadmap.shipped_window_days
// window only governs how long a shipped item stays on the board (§113).
func (s *Service) GetChangelog(ctx context.Context) ([]gen.ListRoadmapChangelogRow, error) {
	rows, err := s.store.listChangelog(ctx)
	if err != nil {
		return nil, err
	}

	if rows == nil {
		rows = make([]gen.ListRoadmapChangelogRow, 0)
	}

	return rows, nil
}

// ---------------------------------------------------------------------------
// Votes
// ---------------------------------------------------------------------------

// CastVote adds the user's vote to an item and returns the refreshed
// vote_count. Only votable-status items accept votes (§113); a repeat
// vote returns ErrAlreadyVoted → 409.
func (s *Service) CastVote(ctx context.Context, itemID, userID pgtype.UUID) (int32, error) {
	item, err := s.store.getItemByID(ctx, itemID)
	if err != nil {
		return 0, err
	}

	if !IsVotableStatus(ItemStatus(item.Status)) {
		return 0, fmt.Errorf("%w: items in status %q are not votable", errpkg.ErrInvalid, item.Status)
	}

	if err := s.store.tryCreateVote(ctx, itemID, userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// ON CONFLICT DO NOTHING returned no row → vote already exists.
			return 0, ErrAlreadyVoted
		}

		return 0, err
	}

	if err := s.store.syncVoteCount(ctx, itemID); err != nil {
		return 0, err
	}

	s.invalidateBoard(ctx)

	return s.voteCount(ctx, itemID)
}

// RemoveVote deletes the user's vote and returns the refreshed vote_count.
// Removing a vote that does not exist returns ErrNotVoted → 409.
func (s *Service) RemoveVote(ctx context.Context, itemID, userID pgtype.UUID) (int32, error) {
	rows, err := s.store.deleteVote(ctx, itemID, userID)
	if err != nil {
		return 0, err
	}

	if rows == 0 {
		return 0, ErrNotVoted
	}

	if err := s.store.syncVoteCount(ctx, itemID); err != nil {
		return 0, err
	}

	s.invalidateBoard(ctx)

	return s.voteCount(ctx, itemID)
}

func (s *Service) voteCount(ctx context.Context, itemID pgtype.UUID) (int32, error) {
	item, err := s.store.getItemByID(ctx, itemID)
	if err != nil {
		return 0, err
	}

	return item.VoteCount, nil
}

// ---------------------------------------------------------------------------
// Community submissions
// ---------------------------------------------------------------------------

// SubmissionInput holds the POST /roadmap/submissions payload.
type SubmissionInput struct {
	Title      string
	Summary    string
	Body       string
	CategoryID pgtype.UUID
	UserID     pgtype.UUID
}

// CreateSubmission records a community feature request in status pending.
// Gated by the roadmap.submissions_enabled setting (§113); no shadow
// contents row is created until a moderator approves the item to
// under_review — pending items are never public.
func (s *Service) CreateSubmission(ctx context.Context, in SubmissionInput) (gen.RoadmapItem, error) {
	if !s.settingsBool(ctx, "roadmap.submissions_enabled", true) {
		return gen.RoadmapItem{}, fmt.Errorf("%w: roadmap submissions are closed", ErrSubmissionsDisabled)
	}

	if strings.TrimSpace(in.Title) == "" {
		return gen.RoadmapItem{}, fmt.Errorf("%w: title is required", errpkg.ErrRequired)
	}

	slug, err := s.uniqueItemSlug(ctx, content.Slugify(in.Title))
	if err != nil {
		return gen.RoadmapItem{}, err
	}

	item, err := s.store.createItem(ctx, gen.CreateRoadmapItemParams{
		Title:       strings.TrimSpace(in.Title),
		Slug:        slug,
		Summary:     strPtrOrNil(in.Summary),
		Body:        strPtrOrNil(in.Body),
		CategoryID:  in.CategoryID,
		Status:      string(StatusPending),
		SubmittedBy: in.UserID,
	})
	if err != nil {
		return gen.RoadmapItem{}, err
	}

	// Submissions don't mutate the public board, but a pending→under_review
	// approval could land while this response is in flight — invalidate so a
	// stale cached board never masks a newly approved item.
	s.invalidateBoard(ctx)

	return item, nil
}

// ---------------------------------------------------------------------------
// Admin items
// ---------------------------------------------------------------------------

// AdminItemInput carries the writable fields for create/update. Pointer
// fields distinguish "absent" (keep current / default) from "set".
type AdminItemInput struct {
	Title          string
	Slug           string
	Summary        *string
	Body           *string
	CategoryID     *pgtype.UUID
	Status         string
	SortOrder      *int32
	IsFlagship     *bool
	TargetWindow   *string
	FeatureFlagKey *string
	ActorID        pgtype.UUID
}

// AdminListItems returns the paginated admin DataTable payload.
func (s *Service) AdminListItems(ctx context.Context, f AdminListFilter) (AdminListResponse, error) {
	return s.store.listAdminItems(ctx, f)
}

// AdminListSubmissions is the pending triage queue — AdminListItems
// hardcoded to status=pending.
func (s *Service) AdminListSubmissions(ctx context.Context, f AdminListFilter) (AdminListResponse, error) {
	f.Status = string(StatusPending)
	f.IncludeDeleted = false

	return s.store.listAdminItems(ctx, f)
}

// AdminGetItem returns a single item including soft-deleted rows.
func (s *Service) AdminGetItem(ctx context.Context, id pgtype.UUID) (gen.RoadmapItem, error) {
	return s.store.getItemByID(ctx, id)
}

// AdminCreateItem creates a curated board item. When the initial status is
// publicly visible the shadow contents row is created immediately so §49
// comments work from first publish (§113).
func (s *Service) AdminCreateItem(ctx context.Context, in AdminItemInput) (gen.RoadmapItem, error) {
	if strings.TrimSpace(in.Title) == "" {
		return gen.RoadmapItem{}, fmt.Errorf("%w: title is required", errpkg.ErrRequired)
	}

	status := in.Status
	if status == "" {
		status = string(StatusUnderReview)
	}

	if !ValidStatus(status) {
		return gen.RoadmapItem{}, fmt.Errorf("%w: unknown status %q", errpkg.ErrInvalid, status)
	}

	base := in.Slug
	if base == "" {
		base = in.Title
	}

	slug, err := s.uniqueItemSlug(ctx, content.Slugify(base))
	if err != nil {
		return gen.RoadmapItem{}, err
	}

	var categoryID pgtype.UUID
	if in.CategoryID != nil {
		categoryID = *in.CategoryID
	}

	var sortOrder int32
	if in.SortOrder != nil {
		sortOrder = *in.SortOrder
	}

	var isFlagship bool
	if in.IsFlagship != nil {
		isFlagship = *in.IsFlagship
	}

	item, err := s.store.createItem(ctx, gen.CreateRoadmapItemParams{
		Title:          strings.TrimSpace(in.Title),
		Slug:           slug,
		Summary:        in.Summary,
		Body:           in.Body,
		CategoryID:     categoryID,
		Status:         status,
		SortOrder:      sortOrder,
		IsFlagship:     isFlagship,
		TargetWindow:   in.TargetWindow,
		FeatureFlagKey: in.FeatureFlagKey,
	})
	if err != nil {
		return gen.RoadmapItem{}, err
	}

	// Lazily create the shadow contents row on first publishable save (§113).
	// Failure is logged, not fatal — a later PATCH retries via
	// ensureShadowContent and the item itself is already durable.
	if IsPublicStatus(ItemStatus(item.Status)) {
		if _, err := s.ensureShadowContent(ctx, item, in.ActorID); err != nil {
			slog.Error("roadmap shadow content create failed", "item_id", item.ID, "error", err)
		}
	}

	s.invalidateBoard(ctx)

	return item, nil
}

// AdminUpdateItem applies a PATCH to an item and keeps the shadow contents
// row in sync (title, body, refreshed content_hash per Rule 12).
func (s *Service) AdminUpdateItem(ctx context.Context, id pgtype.UUID, in AdminItemInput) (gen.RoadmapItem, error) {
	current, err := s.store.getItemByID(ctx, id)
	if err != nil {
		return gen.RoadmapItem{}, err
	}

	title := current.Title
	if strings.TrimSpace(in.Title) != "" {
		title = strings.TrimSpace(in.Title)
	}

	slug := current.Slug
	if in.Slug != "" && in.Slug != current.Slug {
		slug, err = s.uniqueItemSlug(ctx, content.Slugify(in.Slug))
		if err != nil {
			return gen.RoadmapItem{}, err
		}
	}

	summary := current.Summary
	if in.Summary != nil {
		summary = in.Summary
	}

	body := current.Body
	if in.Body != nil {
		body = in.Body
	}

	categoryID := current.CategoryID
	if in.CategoryID != nil {
		categoryID = *in.CategoryID
	}

	sortOrder := current.SortOrder
	if in.SortOrder != nil {
		sortOrder = *in.SortOrder
	}

	isFlagship := current.IsFlagship
	if in.IsFlagship != nil {
		isFlagship = *in.IsFlagship
	}

	targetWindow := current.TargetWindow
	if in.TargetWindow != nil {
		targetWindow = in.TargetWindow
	}

	featureFlagKey := current.FeatureFlagKey
	if in.FeatureFlagKey != nil {
		featureFlagKey = in.FeatureFlagKey
	}

	item, err := s.store.updateItem(ctx, gen.UpdateRoadmapItemParams{
		ID:             id,
		Title:          title,
		Slug:           slug,
		Summary:        summary,
		Body:           body,
		CategoryID:     categoryID,
		SortOrder:      sortOrder,
		IsFlagship:     isFlagship,
		TargetWindow:   targetWindow,
		FeatureFlagKey: featureFlagKey,
		ApprovedBy:     current.ApprovedBy, // unchanged by edit; set on approval
	})
	if err != nil {
		return gen.RoadmapItem{}, err
	}

	s.syncShadowContent(ctx, item, in.ActorID)
	s.invalidateBoard(ctx)

	return item, nil
}

// AdminSetStatus drives the lifecycle state machine (§113). Approving a
// pending submission (pending → under_review) creates the shadow contents
// row and stamps approved_by; → shipped stamps shipped_at (SQL-side) and
// enqueues roadmap:ship_notify fire-and-forget (Rule 8).
func (s *Service) AdminSetStatus(ctx context.Context, id pgtype.UUID, to ItemStatus, actorID pgtype.UUID) (gen.RoadmapItem, error) {
	if !ValidStatus(string(to)) {
		return gen.RoadmapItem{}, fmt.Errorf("%w: unknown status %q", errpkg.ErrInvalid, to)
	}

	current, err := s.store.getItemByID(ctx, id)
	if err != nil {
		return gen.RoadmapItem{}, err
	}

	from := ItemStatus(current.Status)
	if !IsValidTransition(from, to) {
		return gen.RoadmapItem{}, fmt.Errorf("%w: %s → %s", ErrInvalidTransition, from, to)
	}

	// Approving a community submission: create the shadow contents row and
	// record the approving moderator. The full-row update is required to set
	// approved_by — UpdateRoadmapItemStatus only touches status/shipped_at.
	if from == StatusPending && to == StatusUnderReview {
		if _, err := s.ensureShadowContent(ctx, current, actorID); err != nil {
			return gen.RoadmapItem{}, fmt.Errorf("approving roadmap item: %w", err)
		}

		if _, err := s.store.updateItem(ctx, gen.UpdateRoadmapItemParams{
			ID:             id,
			Title:          current.Title,
			Slug:           current.Slug,
			Summary:        current.Summary,
			Body:           current.Body,
			CategoryID:     current.CategoryID,
			SortOrder:      current.SortOrder,
			IsFlagship:     current.IsFlagship,
			TargetWindow:   current.TargetWindow,
			FeatureFlagKey: current.FeatureFlagKey,
			ApprovedBy:     actorID,
		}); err != nil {
			return gen.RoadmapItem{}, err
		}
	}

	item, err := s.store.updateItemStatus(ctx, id, string(to))
	if err != nil {
		return gen.RoadmapItem{}, err
	}

	if to == StatusShipped {
		s.enqueueShipNotify(item)
	}

	s.invalidateBoard(ctx)

	return item, nil
}

// AdminDeleteItem soft-deletes an item. force=true permanently removes the
// row and is only valid on an already-soft-deleted item (Rule 11) — the
// admin.hard_delete permission gate lives in rbac.RequireForceDelete
// middleware at the route level.
func (s *Service) AdminDeleteItem(ctx context.Context, id pgtype.UUID, force bool) error {
	item, err := s.store.getItemByID(ctx, id)
	if err != nil {
		return err
	}

	if force {
		if !item.DeletedAt.Valid {
			return fmt.Errorf("%w: item must be soft-deleted before force delete", errpkg.ErrNotDeleted)
		}

		if err := s.store.hardDeleteItem(ctx, id); err != nil {
			return err
		}

		s.invalidateBoard(ctx)

		return nil
	}

	if err := s.store.softDeleteItem(ctx, id); err != nil {
		return err
	}

	s.invalidateBoard(ctx)

	return nil
}

// AdminRestoreItem clears deleted_at on a soft-deleted item and returns
// the restored row.
func (s *Service) AdminRestoreItem(ctx context.Context, id pgtype.UUID) (gen.RoadmapItem, error) {
	item, err := s.store.getItemByID(ctx, id)
	if err != nil {
		return gen.RoadmapItem{}, err
	}

	if !item.DeletedAt.Valid {
		return gen.RoadmapItem{}, fmt.Errorf("%w: item is not deleted", ErrNotFound)
	}

	if err := s.store.restoreItem(ctx, id); err != nil {
		return gen.RoadmapItem{}, err
	}

	s.invalidateBoard(ctx)

	return s.store.getItemByID(ctx, id)
}

// AdminReorderItems applies explicit sort_order values to a set of items.
func (s *Service) AdminReorderItems(ctx context.Context, ids []pgtype.UUID, orders []int32) error {
	if len(ids) == 0 {
		return nil
	}

	if len(ids) != len(orders) {
		return fmt.Errorf("%w: ids and sort_orders length mismatch", errpkg.ErrInvalid)
	}

	if err := s.store.reorderItems(ctx, ids, orders); err != nil {
		return err
	}

	s.invalidateBoard(ctx)

	return nil
}

// ---------------------------------------------------------------------------
// Categories
// ---------------------------------------------------------------------------

// CategoryInput carries the writable fields for category create/update.
type CategoryInput struct {
	Slug        string
	Name        string
	Description *string
	Color       *string
	SortOrder   *int32
}

// ListCategories returns the public (non-deleted) categories.
func (s *Service) ListCategories(ctx context.Context) ([]gen.RoadmapCategory, error) {
	return s.store.listCategories(ctx)
}

// AdminListCategories returns all categories including soft-deleted rows.
func (s *Service) AdminListCategories(ctx context.Context) ([]gen.RoadmapCategory, error) {
	return s.store.listAdminCategories(ctx)
}

// AdminCreateCategory creates a category; an empty slug is derived from the name.
func (s *Service) AdminCreateCategory(ctx context.Context, in CategoryInput) (gen.RoadmapCategory, error) {
	if strings.TrimSpace(in.Name) == "" {
		return gen.RoadmapCategory{}, fmt.Errorf("%w: name is required", errpkg.ErrRequired)
	}

	slug := in.Slug
	if slug == "" {
		slug = content.Slugify(in.Name)
	}

	var sortOrder int32
	if in.SortOrder != nil {
		sortOrder = *in.SortOrder
	}

	cat, err := s.store.createCategory(ctx, gen.CreateRoadmapCategoryParams{
		Slug:        slug,
		Name:        strings.TrimSpace(in.Name),
		Description: in.Description,
		Color:       in.Color,
		SortOrder:   sortOrder,
	})
	if err != nil {
		return gen.RoadmapCategory{}, err
	}

	s.invalidateBoard(ctx)

	return cat, nil
}

// AdminUpdateCategory applies a PATCH to a category.
func (s *Service) AdminUpdateCategory(ctx context.Context, id pgtype.UUID, in CategoryInput) (gen.RoadmapCategory, error) {
	current, err := s.store.getCategoryByID(ctx, id)
	if err != nil {
		return gen.RoadmapCategory{}, err
	}

	name := current.Name
	if strings.TrimSpace(in.Name) != "" {
		name = strings.TrimSpace(in.Name)
	}

	slug := current.Slug
	if in.Slug != "" {
		slug = in.Slug
	}

	description := current.Description
	if in.Description != nil {
		description = in.Description
	}

	color := current.Color
	if in.Color != nil {
		color = in.Color
	}

	sortOrder := current.SortOrder
	if in.SortOrder != nil {
		sortOrder = *in.SortOrder
	}

	cat, err := s.store.updateCategory(ctx, gen.UpdateRoadmapCategoryParams{
		ID:          id,
		Slug:        slug,
		Name:        name,
		Description: description,
		Color:       color,
		SortOrder:   sortOrder,
	})
	if err != nil {
		return gen.RoadmapCategory{}, err
	}

	s.invalidateBoard(ctx)

	return cat, nil
}

// AdminDeleteCategory soft-deletes a category. Items keep their
// category_id — the FK only fires on hard delete (ON DELETE SET NULL).
func (s *Service) AdminDeleteCategory(ctx context.Context, id pgtype.UUID) error {
	if _, err := s.store.getCategoryByID(ctx, id); err != nil {
		return err
	}

	if err := s.store.softDeleteCategory(ctx, id); err != nil {
		return err
	}

	s.invalidateBoard(ctx)

	return nil
}

// ---------------------------------------------------------------------------
// Shadow contents row (§49 comments backing)
// ---------------------------------------------------------------------------

// ensureShadowContent returns the item's shadow contents ID, creating and
// linking the row on first publishable save. The author is the submitter
// when present, else the acting admin.
func (s *Service) ensureShadowContent(ctx context.Context, item gen.RoadmapItem, actorID pgtype.UUID) (pgtype.UUID, error) {
	if item.ContentID.Valid {
		return item.ContentID, nil
	}

	author := item.SubmittedBy
	if !author.Valid {
		author = actorID
	}

	contentID, err := s.createShadowContent(ctx, item, author)
	if err != nil {
		return pgtype.UUID{}, err
	}

	if err := s.store.setItemContent(ctx, item.ID, contentID); err != nil {
		return pgtype.UUID{}, err
	}

	return contentID, nil
}

// createShadowContent inserts the hidden contents row that backs the §49
// comment thread. content_hash is computed per Rule 12 from the decoded
// body text via the shared content.ComputeContentHash helper. On a slug
// collision the shadow slug retries with a numeric suffix.
func (s *Service) createShadowContent(ctx context.Context, item gen.RoadmapItem, authorID pgtype.UUID) (pgtype.UUID, error) {
	bodyText := shadowBodyText(deref(item.Body))
	hash := content.ComputeContentHash(bodyText)

	baseSlug := "roadmap-" + item.Slug

	for i := 0; i < content.MaxSlugAttempts; i++ {
		slug := baseSlug
		if i > 0 {
			slug = fmt.Sprintf("%s-%d", baseSlug, i+1)
		}

		id, err := s.store.createShadowContent(ctx, gen.CreateRoadmapShadowContentParams{
			AuthorID:    authorID,
			Title:       item.Title,
			Slug:        slug,
			Body:        bodyText,
			ContentHash: &hash,
		})
		if err == nil {
			return id, nil
		}

		if !isUniqueViolation(err) {
			return pgtype.UUID{}, err
		}
	}

	return pgtype.UUID{}, fmt.Errorf("creating roadmap shadow content: no unique slug after %d attempts", content.MaxSlugAttempts)
}

// syncShadowContent keeps the shadow contents row in step with the item —
// creating it on first publishable save, updating title/body/content_hash
// when it already exists (Rule 12). Failures are logged, not fatal.
func (s *Service) syncShadowContent(ctx context.Context, item gen.RoadmapItem, actorID pgtype.UUID) {
	if !item.ContentID.Valid {
		if IsPublicStatus(ItemStatus(item.Status)) {
			if _, err := s.ensureShadowContent(ctx, item, actorID); err != nil {
				slog.Error("roadmap shadow content create failed", "item_id", item.ID, "error", err)
			}
		}

		return
	}

	bodyText := shadowBodyText(deref(item.Body))
	hash := content.ComputeContentHash(bodyText)

	if err := s.store.updateShadowContent(ctx, gen.UpdateRoadmapShadowContentParams{
		ContentID:   item.ContentID,
		Title:       item.Title,
		Body:        bodyText,
		ContentHash: &hash,
	}); err != nil {
		slog.Error("roadmap shadow content sync failed", "item_id", item.ID, "content_id", item.ContentID, "error", err)
	}
}

// shadowBodyText returns the plain source text used for the shadow
// contents row's body and content_hash. roadmap_items.body stores
// base64-encoded TipTap JSON (§49 comment convention); plain-text and
// markdown bodies pass through unchanged.
func shadowBodyText(body string) string {
	if body == "" {
		return ""
	}

	if decoded, err := base64.StdEncoding.DecodeString(body); err == nil {
		if trimmed := strings.TrimSpace(string(decoded)); strings.HasPrefix(trimmed, "{") {
			return trimmed
		}
	}

	return body
}

// isUniqueViolation reports whether err wraps a Postgres unique-violation
// (SQLSTATE 23505) — used for shadow-content slug retry.
func isUniqueViolation(err error) bool {
	return strings.Contains(err.Error(), "23505") || strings.Contains(err.Error(), "duplicate key")
}

// ---------------------------------------------------------------------------
// Board cache invalidation
// ---------------------------------------------------------------------------

// invalidateBoard drops the cached public board after any item, category,
// or vote mutation. Best-effort — a Valkey failure only delays visibility
// by one TTL.
func (s *Service) invalidateBoard(ctx context.Context) {
	if s.vk == nil {
		return
	}

	if err := s.vk.Del(ctx, boardCacheKey).Err(); err != nil {
		slog.Warn("roadmap board cache invalidation failed", "error", err)
	}
}

// ---------------------------------------------------------------------------
// Background task helpers
// ---------------------------------------------------------------------------

// enqueueShipNotify fires the roadmap:ship_notify task — fire-and-forget
// per Rule 8; the request never blocks on the notification fan-out.
func (s *Service) enqueueShipNotify(item gen.RoadmapItem) {
	payload, err := json.Marshal(map[string]any{
		"item_id": item.ID.String(),
		"title":   item.Title,
		"slug":    item.Slug,
	})
	if err != nil {
		return
	}

	if err := s.enqueuer.Enqueue(taskTypeShipNotify, payload); err != nil {
		slog.Error("enqueue roadmap ship_notify", "item_id", item.ID, "error", err)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// uniqueItemSlug finds an unused roadmap_items slug by appending -2, -3,
// … to the base until a gap appears. getItemBySlug wraps the gen query
// error with %w, so errors.Is still sees pgx.ErrNoRows.
func (s *Service) uniqueItemSlug(ctx context.Context, base string) (string, error) {
	candidate := base

	for i := 2; ; i++ {
		_, err := s.store.getItemBySlug(ctx, candidate, pgtype.UUID{})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return candidate, nil
			}

			return "", err
		}

		if i > content.MaxSlugAttempts {
			return "", fmt.Errorf("%w: could not find unique slug for %q", errpkg.ErrInvalid, base)
		}

		candidate = fmt.Sprintf("%s-%d", base, i)
	}
}

func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}
