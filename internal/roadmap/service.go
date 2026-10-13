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
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/features"
	jcvalkey "github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/worker"
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

// worker.TypeRoadmapShipNotify is the Asynq task enqueued fire-and-forget
// when an item transitions to shipped (Rule 8). Registered in the §11
// task catalog; the handler is HandleRoadmapShipNotify in internal/worker.
// Ref: sections/11-job-queue-asynq.md (roadmap:ship_notify).

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
// Feature-flag sync (unified rollout §4.2–§4.4)
// ---------------------------------------------------------------------------

// flagPhaseStatus maps a rollout phase to the roadmap item status the flag
// owns for linked items (spec §4.2) — the phase vocabulary maps 1:1 onto
// item statuses. Unknown/empty phases degrade to planned — the flag has
// not declared progress.
func flagPhaseStatus(phase string) ItemStatus {
	switch ItemStatus(phase) {
	case StatusPlanned, StatusInProgress, StatusTesting, StatusShipped, StatusArchived:
		return ItemStatus(phase)
	case StatusPending, StatusUnderReview, StatusDeclined:
		return StatusPlanned // submission-only statuses are never flag phases
	default:
		return StatusPlanned
	}
}

// effectiveFlagPhase resolves an unset/invalid phase from the flag's
// enabled state the same way eval does (§4.1 defaults): enabled flags
// without a declared phase are live (shipped); disabled flags are planned.
func effectiveFlagPhase(phase string, enabled bool) string {
	if features.ValidPhase(phase) {
		return phase
	}

	if enabled {
		return string(StatusShipped)
	}

	return string(StatusPlanned)
}

// SyncFlagPhase reconciles the roadmap item linked to flagKey with the
// flag's post-write config. Wired to Checker.SetOnConfigChanged in
// server.go — fires after every flag write (SetEnabled, PatchFlag,
// SetRolloutPercentage, DeleteFlag), fire-and-forget via
// context.WithoutCancel (Rule 8).
//
// Semantics (§4.3/§4.4):
//   - flag row missing (deleted)      → unlink the item; last status kept
//   - show_on_roadmap=false           → linked item archived (link kept)
//   - show_on_roadmap=true, no item   → create item (title=name|label|key,
//     slug=slugify+dedupe, summary=description, status=phase-derived)
//   - show_on_roadmap=true, item      → write the phase-derived status
//     directly, bypassing IsValidTransition — the flag's phase graph is
//     the authority; →shipped fires roadmap:ship_notify only on a real
//     transition into shipped.
func (s *Service) SyncFlagPhase(ctx context.Context, flagKey string, cfg features.FlagConfig) error {
	if strings.TrimSpace(flagKey) == "" {
		return nil
	}

	// Read the flag row on the write pool: a missing row means the flag was
	// deleted (the only write path that leaves no row), and reading the row
	// post-write keeps the sync convergent under racing writes — the last
	// hook to run applies the newest stored config, not a stale payload.
	flagRow, err := s.store.getFlagRow(ctx, flagKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return s.unlinkFlagItems(ctx, flagKey)
	}

	if err != nil {
		return err
	}

	item, linked, err := s.linkedItem(ctx, flagKey)
	if err != nil {
		return err
	}

	eff := flagConfigFromRow(flagRow, cfg)

	if !eff.ShowOnRoadmap {
		return s.archiveLinkedItem(ctx, item, linked)
	}

	return s.applyFlagPhase(ctx, flagKey, item, linked, eff, flagRow.Enabled)
}

// flagConfigFromRow resolves the effective flag config for the sync: the
// stored config blob is the truth the triggering write persisted; the hook
// payload is the fallback for rows with no parseable config.
func flagConfigFromRow(row gen.GetFeatureFlagRowByKeyRow, fallback features.FlagConfig) features.FlagConfig {
	if len(row.Config) > 0 {
		var stored features.FlagConfig
		if err := json.Unmarshal(row.Config, &stored); err == nil {
			return stored
		}
	}

	return fallback
}

// linkedItem resolves the live roadmap item linked to a flag —
// (item, true, nil) when linked, (zero, false, nil) when not.
func (s *Service) linkedItem(ctx context.Context, flagKey string) (gen.RoadmapItem, bool, error) {
	item, err := s.store.getItemByFlagKey(ctx, flagKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.RoadmapItem{}, false, nil
	}

	if err != nil {
		return gen.RoadmapItem{}, false, err
	}

	return item, true, nil
}

// unlinkFlagItems clears feature_flag_key on every item linked to a
// deleted flag (§4.3) — items keep their last status and return to the
// manual lifecycle.
func (s *Service) unlinkFlagItems(ctx context.Context, flagKey string) error {
	if err := s.store.clearItemFlagKey(ctx, flagKey); err != nil {
		return err
	}

	s.invalidateBoard(ctx)

	return nil
}

// archiveLinkedItem archives the flag's linked item when it is opted off
// the roadmap (§4.4). The link is retained so re-enable restores it; no
// link → nothing to do.
func (s *Service) archiveLinkedItem(ctx context.Context, item gen.RoadmapItem, linked bool) error {
	if linked && item.Status != string(StatusArchived) {
		if _, err := s.store.updateItemStatus(ctx, item.ID, string(StatusArchived)); err != nil {
			return err
		}
	}

	s.invalidateBoard(ctx)

	return nil
}

// applyFlagPhase drives the linked item to the flag's phase-derived
// status — creating the item when none exists (§4.3) or writing the
// status directly when it does, bypassing IsValidTransition because the
// flag's phase graph is the authority (§4.4). →shipped fires
// roadmap:ship_notify only on a real transition into shipped.
func (s *Service) applyFlagPhase(ctx context.Context, flagKey string, item gen.RoadmapItem, linked bool, cfg features.FlagConfig, enabled bool) error {
	desired := flagPhaseStatus(effectiveFlagPhase(cfg.Phase, enabled))

	if !linked {
		return s.createItemForFlag(ctx, flagKey, cfg, desired)
	}

	if from := ItemStatus(item.Status); from != desired {
		updated, err := s.store.updateItemStatus(ctx, item.ID, string(desired))
		if err != nil {
			return err
		}

		item = updated

		if desired == StatusShipped {
			s.enqueueShipNotify(updated)
		}
	}

	// A flag flipping a linked item from archived back to a public status
	// needs the §49 shadow contents row — without it the item renders with
	// no comments thread.
	if IsPublicStatus(desired) {
		s.ensureShadowContentSynced(ctx, item)
	}

	s.invalidateBoard(ctx)

	return nil
}

// createItemForFlag inserts the roadmap row for an opted-in flag (§4.3).
// Title falls back flag name → registry label → raw key; the slug dedupes
// through uniqueItemSlug; summary carries the flag description.
func (s *Service) createItemForFlag(ctx context.Context, flagKey string, cfg features.FlagConfig, status ItemStatus) error {
	title := strings.TrimSpace(cfg.Name)
	summary := strings.TrimSpace(cfg.Description)

	if f, ok := features.RegistryLookup(flagKey); ok {
		if title == "" {
			title = f.Label
		}

		if summary == "" {
			summary = f.Description
		}
	}

	if title == "" {
		title = flagKey
	}

	slug, err := s.uniqueItemSlug(ctx, content.Slugify(title))
	if err != nil {
		return err
	}

	item, err := s.store.createItem(ctx, gen.CreateRoadmapItemParams{
		Title:          title,
		Slug:           slug,
		Summary:        strPtrOrNil(summary),
		Status:         string(status),
		FeatureFlagKey: &flagKey,
	})
	if err != nil {
		return mapWriteErr(err)
	}

	if IsPublicStatus(status) {
		s.ensureShadowContentSynced(ctx, item)
	}

	// A flag arriving on the board already shipped still needs the
	// shipped_at stamp (the board column filters on it) and the voter
	// fan-out — mirrors the create-into-shipped path in AdminCreateItem.
	if status == StatusShipped {
		if stamped, err := s.store.updateItemStatus(ctx, item.ID, string(StatusShipped)); err != nil {
			slog.Error("roadmap shipped stamp failed on flag sync create", "item_id", item.ID, "error", err)
		} else {
			item = stamped
		}

		s.enqueueShipNotify(item)
	}

	s.invalidateBoard(ctx)

	return nil
}

// ensureShadowContentSynced guarantees the §49 shadow contents row for a
// flag-synced item. There is no acting admin, so the author falls back to
// the oldest account (the 000798 backfill rule); an unresolvable author
// skips the shadow — a later publishable save recreates it.
func (s *Service) ensureShadowContentSynced(ctx context.Context, item gen.RoadmapItem) {
	if item.ContentID.Valid {
		return
	}

	author := item.SubmittedBy
	if !author.Valid {
		var err error
		if author, err = s.store.shadowAuthorID(ctx); err != nil {
			slog.Warn("roadmap shadow author lookup failed", "item_id", item.ID, "error", err)
			return
		}
	}

	if _, err := s.ensureShadowContent(ctx, item, author); err != nil {
		slog.Error("roadmap shadow content create failed on flag sync", "item_id", item.ID, "error", err)
	}
}

// flagLinkTarget validates a feature_flag_key link target and returns the
// flag's current row state for phase application. Registry keys always
// validate — a registered flag with no stored row derives its config from
// the registry default; custom keys must own a feature_flags row.
func (s *Service) flagLinkTarget(ctx context.Context, key string) (gen.GetFeatureFlagRowByKeyRow, error) {
	row, err := s.store.getFlagRow(ctx, key)
	if err == nil {
		return row, nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return gen.GetFeatureFlagRowByKeyRow{}, err
	}

	if _, ok := features.RegistryLookup(key); !ok {
		return gen.GetFeatureFlagRowByKeyRow{}, fmt.Errorf("%w: %q", ErrUnknownFlagKey, key)
	}

	return gen.GetFeatureFlagRowByKeyRow{Key: key, Enabled: features.DefaultFor(key)}, nil
}

// derivedStatusForLink returns the status a linked flag currently owns —
// applied immediately when feature_flag_key is set on an item (§4.2). It
// enforces the one-flag-one-item rule (a second item would reconcile
// wrongly under SyncFlagPhase's :one lookup while staying status-locked)
// and the flag's opt-out intent: a flag with show_on_roadmap=false links
// but keeps the item archived/hidden until it is opted in. excludeID is
// the item being edited so re-setting the same key is a no-op; pass the
// zero UUID on create.
func (s *Service) derivedStatusForLink(ctx context.Context, key string, excludeID pgtype.UUID) (ItemStatus, error) {
	row, err := s.flagLinkTarget(ctx, key)
	if err != nil {
		return "", err
	}

	taken, err := s.store.flagKeyTakenElsewhere(ctx, key, excludeID)
	if err != nil {
		return "", err
	}

	if taken {
		return "", fmt.Errorf("%w: %q", ErrFlagAlreadyLinked, key)
	}

	var cfg features.FlagConfig
	if len(row.Config) > 0 {
		// Malformed config degrades to the enabled-derived phase.
		_ = json.Unmarshal(row.Config, &cfg)
	}

	if !cfg.ShowOnRoadmap {
		// Opted off the roadmap: keep the link but stay archived — matching
		// SyncFlagPhase's show_off path — so the item is not publicly
		// visible until the next flag write opts it in (§4.4).
		return StatusArchived, nil
	}

	return flagPhaseStatus(effectiveFlagPhase(cfg.Phase, row.Enabled)), nil
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

	// Submitter sees their own item in any state (§22: a declined item must
	// be reachable so the author can appeal the decision from the detail
	// surface); everyone else gets the §113 indistinguishable-404.
	if !IsPublicStatus(ItemStatus(row.Status)) && row.SubmittedBy != viewerID {
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

	detail := &ItemDetail{BoardItem: item, Body: row.Body, IsSubmitter: row.SubmittedBy == viewerID && viewerID.Valid}

	// Related items — same category, excluding self, capped at 3 (SQL-side).
	// The category join is NULL when the item is uncategorized → no related
	// items rather than a cross-category fallback.
	if row.CategoryID.Valid {
		if rel, err := s.store.listRelatedItems(ctx, viewerID, row.ID, row.CategoryID); err == nil {
			detail.Related = make([]BoardItem, 0, len(rel))
			for _, r := range rel {
				detail.Related = append(detail.Related, boardItemFromRow(gen.ListPublicRoadmapItemsRow{
					ID:             r.ID,
					Title:          r.Title,
					Slug:           r.Slug,
					Summary:        r.Summary,
					CategoryID:     r.CategoryID,
					Status:         r.Status,
					IsFlagship:     r.IsFlagship,
					TargetWindow:   r.TargetWindow,
					FeatureFlagKey: r.FeatureFlagKey,
					ContentID:      r.ContentID,
					VoteCount:      r.VoteCount,
					ShippedAt:      r.ShippedAt,
					CreatedAt:      r.CreatedAt,
					CategoryName:   r.CategoryName,
					CategorySlug:   r.CategorySlug,
					CategoryColor:  r.CategoryColor,
					HasVoted:       r.HasVoted,
				}))
			}
		} else {
			slog.Warn("roadmap related items failed", "item_id", row.ID, "error", err)
		}
	}

	return detail, nil
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
	// Read on the write pool: a just-published (or just-shipped) item may
	// not have reached the replica yet, and the votability/deletion checks
	// below must see the authoritative state.
	item, err := s.store.getItemByIDOnWrite(ctx, itemID)
	if err != nil {
		return 0, err
	}

	// getItemByID intentionally sees soft-deleted rows (admin restore path);
	// a deleted item must not accept votes even while its status is votable.
	if item.DeletedAt.Valid {
		return 0, fmt.Errorf("%w: roadmap item is deleted", ErrNotFound)
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
	// Read-through-write: the count was just synced on the write pool, and
	// a lagging read replica would return the pre-mutation value here.
	item, err := s.store.getItemByIDOnWrite(ctx, itemID)
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

	if err := s.requireLiveCategory(ctx, in.CategoryID); err != nil {
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
		return gen.RoadmapItem{}, mapWriteErr(err)
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

	// Linking a feature flag hands the item's lifecycle to the flag (§4.2):
	// the stored status is replaced by the flag's current phase-derived
	// status. Unknown keys are rejected with unknown_feature_flag.
	var flagKey *string

	if in.FeatureFlagKey != nil {
		if key := strings.TrimSpace(*in.FeatureFlagKey); key != "" {
			// No item exists yet — the zero UUID excludes nothing in the
			// flag-key taken check.
			derived, err := s.derivedStatusForLink(ctx, key, pgtype.UUID{Valid: true})
			if err != nil {
				return gen.RoadmapItem{}, err
			}

			status = string(derived)
			flagKey = &key
		}
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

	if err := s.requireLiveCategory(ctx, categoryID); err != nil {
		return gen.RoadmapItem{}, err
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
		FeatureFlagKey: flagKey,
	})
	if err != nil {
		return gen.RoadmapItem{}, mapWriteErr(err)
	}

	// Lazily create the shadow contents row on first publishable save (§113).
	// Failure is logged, not fatal — a later PATCH retries via
	// ensureShadowContent and the item itself is already durable. On success
	// the response row is updated in place so the POST payload already
	// carries content_id instead of the pre-link snapshot.
	if IsPublicStatus(ItemStatus(item.Status)) {
		if contentID, err := s.ensureShadowContent(ctx, item, in.ActorID); err != nil {
			slog.Error("roadmap shadow content create failed", "item_id", item.ID, "error", err)
		} else {
			item.ContentID = contentID
		}
	}

	// A create that lands directly on shipped must stamp shipped_at and run
	// the voter fan-out — both otherwise only happen on the status-transition
	// path, leaving the item invisible in the shipped board column
	// (shipped_at NULL fails the window predicate) and unsent notifications.
	if ItemStatus(item.Status) == StatusShipped {
		if stamped, err := s.store.updateItemStatus(ctx, item.ID, string(StatusShipped)); err != nil {
			slog.Error("roadmap shipped stamp failed on create", "item_id", item.ID, "error", err)
		} else {
			item = stamped
			s.enqueueShipNotify(item)
		}
	}

	s.invalidateBoard(ctx)

	return item, nil
}

// AdminUpdateItem applies a PATCH to an item and keeps the shadow contents
// row in sync (title, body, refreshed content_hash per Rule 12).
func (s *Service) AdminUpdateItem(ctx context.Context, id pgtype.UUID, in AdminItemInput) (gen.RoadmapItem, error) {
	// mergeItemPatch overlays the PATCH onto the current row — read it on
	// the write pool so a replica-stale snapshot can't drop a concurrent
	// change (e.g. approved_by stamped moments ago).
	current, err := s.store.getItemByIDOnWrite(ctx, id)
	if err != nil {
		return gen.RoadmapItem{}, err
	}

	// Deleted items are immutable until restored — editing one would also
	// sync the (soft-deleted) shadow contents row.
	if current.DeletedAt.Valid {
		return gen.RoadmapItem{}, fmt.Errorf("%w: item is deleted", ErrNotFound)
	}

	params, linkStatus, err := s.mergeItemPatch(ctx, current, in)
	if err != nil {
		return gen.RoadmapItem{}, err
	}

	item, err := s.store.updateItem(ctx, params)
	if err != nil {
		return gen.RoadmapItem{}, mapWriteErr(err)
	}

	// A link set by this PATCH hands the lifecycle to the flag — apply its
	// current phase-derived status right away, bypassing the manual state
	// machine (§4.2). updateItemStatus stamps shipped_at SQL-side; the
	// voter fan-out only fires on a real transition into shipped.
	if linkStatus != "" && ItemStatus(item.Status) != linkStatus {
		prev := ItemStatus(item.Status)

		updated, err := s.store.updateItemStatus(ctx, id, string(linkStatus))
		if err != nil {
			return gen.RoadmapItem{}, err
		}

		item = updated

		if linkStatus == StatusShipped && prev != StatusShipped {
			s.enqueueShipNotify(item)
		}
	}

	s.syncShadowContent(ctx, item, in.ActorID)
	s.invalidateBoard(ctx)

	return item, nil
}

// mergeItemPatch overlays a PATCH input onto the current row — scalar
// pointers distinguish "absent" from "set to empty"; a changed slug is
// re-slugified and de-duplicated. The second return value carries the
// flag-derived status to apply when the PATCH sets a feature_flag_key
// link ("" = no status override).
func (s *Service) mergeItemPatch(ctx context.Context, current gen.RoadmapItem, in AdminItemInput) (gen.UpdateRoadmapItemParams, ItemStatus, error) {
	params := gen.UpdateRoadmapItemParams{
		ID:             current.ID,
		Title:          current.Title,
		Slug:           current.Slug,
		Summary:        current.Summary,
		Body:           current.Body,
		CategoryID:     current.CategoryID,
		SortOrder:      current.SortOrder,
		IsFlagship:     current.IsFlagship,
		TargetWindow:   current.TargetWindow,
		FeatureFlagKey: current.FeatureFlagKey,
		ApprovedBy:     current.ApprovedBy, // unchanged by edit; set on approval
	}

	if strings.TrimSpace(in.Title) != "" {
		params.Title = strings.TrimSpace(in.Title)
	}

	slug, err := s.resolveSlugPatch(ctx, current.Slug, in.Slug)
	if err != nil {
		return gen.UpdateRoadmapItemParams{}, "", err
	}

	params.Slug = slug

	if in.Summary != nil {
		params.Summary = in.Summary
	}

	if in.Body != nil {
		params.Body = in.Body
	}

	if in.CategoryID != nil {
		if err := s.requireLiveCategory(ctx, *in.CategoryID); err != nil {
			return gen.UpdateRoadmapItemParams{}, "", err
		}

		params.CategoryID = *in.CategoryID
	}

	if in.SortOrder != nil {
		params.SortOrder = *in.SortOrder
	}

	if in.IsFlagship != nil {
		params.IsFlagship = *in.IsFlagship
	}

	if in.TargetWindow != nil {
		params.TargetWindow = in.TargetWindow
	}

	var linkStatus ItemStatus

	if in.FeatureFlagKey != nil {
		key, derived, err := s.resolveFlagLink(ctx, current.ID, *in.FeatureFlagKey)
		if err != nil {
			return gen.UpdateRoadmapItemParams{}, "", err
		}

		params.FeatureFlagKey = key // nil on explicit unlink
		linkStatus = derived
	}

	return params, linkStatus, nil
}

// resolveSlugPatch picks the slug for an item update: an absent or empty
// input keeps the current slug; a new input is slugified and de-duplicated.
// The slugified candidate is compared to the current slug — a raw-input
// comparison ("My Slug!" vs "my-slug") would churn the slug to
// "my-slug-2" every save.
func (s *Service) resolveSlugPatch(ctx context.Context, current, input string) (string, error) {
	if input == "" {
		return current, nil
	}

	candidate := content.Slugify(input)
	if candidate == "" || candidate == current {
		return current, nil
	}

	return s.uniqueItemSlug(ctx, candidate)
}

// resolveFlagLink resolves a feature_flag_key PATCH value: an empty value
// unlinks (nil key, no status override — the item keeps its last status
// and returns to the manual lifecycle, §4.2); a key is validated and
// returned with the flag's current derived status for the caller to
// apply. excludeID is the item being edited — it is passed to the
// one-flag-one-item check so re-setting the same key does not self-trip.
// Unknown keys → unknown_feature_flag; keys already linked to another
// item → flag_already_linked.
func (s *Service) resolveFlagLink(ctx context.Context, excludeID pgtype.UUID, value string) (*string, ItemStatus, error) {
	key := strings.TrimSpace(value)
	if key == "" {
		return nil, "", nil
	}

	derived, err := s.derivedStatusForLink(ctx, key, excludeID)
	if err != nil {
		return nil, "", err
	}

	return &key, derived, nil
}

// AdminSetStatus drives the lifecycle state machine (§113). Approving a
// pending submission (pending → under_review) creates the shadow contents
// row and stamps approved_by; → shipped stamps shipped_at (SQL-side) and
// enqueues roadmap:ship_notify fire-and-forget (Rule 8).
func (s *Service) AdminSetStatus(ctx context.Context, id pgtype.UUID, to ItemStatus, actorID pgtype.UUID) (gen.RoadmapItem, error) {
	if !ValidStatus(string(to)) {
		return gen.RoadmapItem{}, fmt.Errorf("%w: unknown status %q", errpkg.ErrInvalid, to)
	}

	// The lifecycle check, approved_by stamp, and shadow-content ensure all
	// key off this snapshot — read on the write pool. A replica-stale
	// ContentID (nil) would make ensureShadowContent insert a second
	// shadow row and orphan the first.
	current, err := s.store.getItemByIDOnWrite(ctx, id)
	if err != nil {
		return gen.RoadmapItem{}, err
	}

	// A soft-deleted item must not move through the lifecycle — e.g. →
	// shipped would fan out roadmap_shipped notifications for a deleted item.
	if current.DeletedAt.Valid {
		return gen.RoadmapItem{}, fmt.Errorf("%w: item is deleted", ErrNotFound)
	}

	from := ItemStatus(current.Status)

	// Linked items derive status from the flag's rollout phase (§4.2) —
	// manual transitions are locked. The operator changes the flag's phase
	// instead; unlinking via PATCH returns the item to manual control.
	if current.FeatureFlagKey != nil && *current.FeatureFlagKey != "" {
		return gen.RoadmapItem{}, fmt.Errorf("%w: item status follows flag %q — change the flag's phase", ErrStatusDerived, *current.FeatureFlagKey)
	}

	if !IsValidTransition(from, to) {
		return gen.RoadmapItem{}, fmt.Errorf("%w: %s → %s", ErrInvalidTransition, from, to)
	}

	// Approving a community submission records the approving moderator. The
	// full-row update is required to set approved_by — UpdateRoadmapItemStatus
	// only touches status/shipped_at.
	if from == StatusPending && to == StatusUnderReview {
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

	// Ensure the §49 shadow contents row exists whenever the item lands on a
	// public status — not only pending→under_review approval. A declined→
	// under_review reopen reaches the public board through a different path
	// and would otherwise render with no comments thread (§113).
	if IsPublicStatus(to) {
		if _, err := s.ensureShadowContent(ctx, current, actorID); err != nil {
			return gen.RoadmapItem{}, fmt.Errorf("publishing roadmap item: %w", err)
		}
	}

	item, err := s.store.updateItemStatus(ctx, id, string(to))
	if err != nil {
		return gen.RoadmapItem{}, err
	}

	// Fire only on the real transition into shipped — an idempotent
	// shipped→shipped PATCH must not re-notify every voter.
	if to == StatusShipped && from != StatusShipped {
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
	item, err := s.store.getItemByIDOnWrite(ctx, id)
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

		// Remove the shadow contents row too — the FK lives on the item side
		// so the contents row would otherwise be orphaned forever. Comments
		// on it cascade via comments_content_id_fkey (allowed here: this is
		// the admin.hard_delete-gated path per Rule 11).
		if item.ContentID.Valid {
			if err := s.store.hardDeleteShadowContent(ctx, item.ContentID); err != nil {
				slog.Warn("roadmap shadow content hard-delete failed", "item_id", item.ID, "content_id", item.ContentID, "error", err)
			}
		}

		s.invalidateBoard(ctx)

		return nil
	}

	if err := s.store.softDeleteItem(ctx, id); err != nil {
		return err
	}

	// The shadow row tracks the item's lifecycle — soft-delete it so the
	// §49 comments thread stops being publicly readable while the item is
	// gone. Best-effort like the other shadow syncs; a failure self-heals
	// on the next delete/restore cycle.
	if item.ContentID.Valid {
		if err := s.store.softDeleteShadowContent(ctx, item.ContentID); err != nil {
			slog.Warn("roadmap shadow content soft-delete failed", "item_id", item.ID, "content_id", item.ContentID, "error", err)
		}
	}

	s.invalidateBoard(ctx)

	return nil
}

// AdminRestoreItem clears deleted_at on a soft-deleted item and returns
// the restored row.
func (s *Service) AdminRestoreItem(ctx context.Context, id pgtype.UUID) (gen.RoadmapItem, error) {
	item, err := s.store.getItemByIDOnWrite(ctx, id)
	if err != nil {
		return gen.RoadmapItem{}, err
	}

	if !item.DeletedAt.Valid {
		return gen.RoadmapItem{}, fmt.Errorf("%w: item is not deleted", ErrNotFound)
	}

	if err := s.store.restoreItem(ctx, id); err != nil {
		return gen.RoadmapItem{}, err
	}

	// Bring the shadow contents row back with the item so its comments
	// thread reappears (best-effort — a missing row is recreated on the
	// next publishable save via ensureShadowContent).
	if item.ContentID.Valid {
		if err := s.store.restoreShadowContent(ctx, item.ContentID); err != nil {
			slog.Warn("roadmap shadow content restore failed", "item_id", item.ID, "content_id", item.ContentID, "error", err)
		}
	}

	s.invalidateBoard(ctx)

	// Read-back on the write pool — the restore just committed there and a
	// replica would still report deleted_at set.
	return s.store.getItemByIDOnWrite(ctx, id)
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
		slug = in.Name
	}
	slug = content.Slugify(slug)
	if slug == "" {
		return gen.RoadmapCategory{}, fmt.Errorf("%w: slug is required (could not be slugified)", errpkg.ErrRequired)
	}

	// The slug UNIQUE constraint covers soft-deleted rows — check existence
	// across all rows so a deleted slug yields 409, not a 23505 500.
	if taken, err := s.slugTaken(ctx, slug, pgtype.UUID{}); err != nil {
		return gen.RoadmapCategory{}, err
	} else if taken {
		return gen.RoadmapCategory{}, fmt.Errorf("%w: slug %q", errpkg.ErrAlreadyExists, slug)
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
		return gen.RoadmapCategory{}, mapWriteErr(err)
	}

	s.invalidateBoard(ctx)

	return cat, nil
}

// AdminUpdateCategory applies a PATCH to a category.
func (s *Service) AdminUpdateCategory(ctx context.Context, id pgtype.UUID, in CategoryInput) (gen.RoadmapCategory, error) {
	current, err := s.store.getCategoryByIDOnWrite(ctx, id)
	if err != nil {
		return gen.RoadmapCategory{}, err
	}

	name := current.Name
	if strings.TrimSpace(in.Name) != "" {
		name = strings.TrimSpace(in.Name)
	}

	slug := current.Slug
	if in.Slug != "" {
		if candidate := content.Slugify(in.Slug); candidate != "" && candidate != current.Slug {
			slug = candidate

			if taken, err := s.slugTaken(ctx, slug, id); err != nil {
				return gen.RoadmapCategory{}, err
			} else if taken {
				return gen.RoadmapCategory{}, fmt.Errorf("%w: slug %q", errpkg.ErrAlreadyExists, slug)
			}
		}
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
		return gen.RoadmapCategory{}, mapWriteErr(err)
	}

	s.invalidateBoard(ctx)

	return cat, nil
}

// AdminDeleteCategory soft-deletes a category. Items keep their
// category_id — the FK only fires on hard delete (ON DELETE SET NULL).
func (s *Service) AdminDeleteCategory(ctx context.Context, id pgtype.UUID) error {
	if _, err := s.store.getCategoryByIDOnWrite(ctx, id); err != nil {
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
// comment thread. content_hash is computed per Rule 12 but scoped to the
// item via ComputeContentHashWithDistinguisher — the idx_contents hash
// index is global across all content types, so an undistinguished hash
// would collide with any existing row carrying the same body text (most
// commonly the empty-body hash e3b0c44… shared by drafts and seed
// content), making the publish path fail with a 23505 on every retry.
// On a slug collision the shadow slug retries with a numeric suffix.
func (s *Service) createShadowContent(ctx context.Context, item gen.RoadmapItem, authorID pgtype.UUID) (pgtype.UUID, error) {
	bodyText := shadowBodyText(deref(item.Body))
	hash := shadowContentHash(item, bodyText)

	baseSlug := "roadmap-" + item.Slug

	for i := range content.MaxSlugAttempts {
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

		// Only a slug collision is retryable — a content_hash violation is
		// unaffected by a new slug and would burn all attempts for nothing.
		if !isSlugConflict(err) {
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
	hash := shadowContentHash(item, bodyText)

	if err := s.store.updateShadowContent(ctx, gen.UpdateRoadmapShadowContentParams{
		ContentID:   item.ContentID,
		Title:       item.Title,
		Body:        bodyText,
		ContentHash: &hash,
	}); err != nil {
		slog.Error("roadmap shadow content sync failed", "item_id", item.ID, "content_id", item.ContentID, "error", err)
	}
}

// shadowContentHash computes the shadow contents row's content_hash,
// distinguished by the item id so identical bodies on different roadmap
// items — and bodies shared with ordinary contents rows — never collide
// under the global idx_contents unique index. Mirrors the video-source
// distinguisher in internal/content (§41 / Rule 12).
func shadowContentHash(item gen.RoadmapItem, bodyText string) string {
	return content.ComputeContentHashWithDistinguisher(bodyText, "roadmap:"+item.ID.String())
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
// (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}

	return strings.Contains(err.Error(), "23505") || strings.Contains(err.Error(), "duplicate key")
}

// isSlugConflict reports whether err is a unique violation on the contents
// slug — the only shadow-content conflict worth retrying with a new slug.
func isSlugConflict(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "slug")
	}

	return strings.Contains(err.Error(), "slug")
}

// mapWriteErr translates Postgres constraint violations into sentinel
// errors so handlers return a 4xx envelope instead of 500 (Rule 5):
// 23505 on the flag-key index → flag_already_linked (a link that raced
// past the pre-check), other 23505 → already_exists (slug collision),
// 23503 → invalid (a referenced row — e.g. category_id — does not exist).
func mapWriteErr(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}

	switch pgErr.Code {
	case "23505":
		if pgErr.ConstraintName == "roadmap_items_feature_flag_key_uniq" {
			return fmt.Errorf("%w: another item is linked to this flag", ErrFlagAlreadyLinked)
		}

		return fmt.Errorf("%w: slug or unique field already in use", errpkg.ErrAlreadyExists)
	case "23503":
		return fmt.Errorf("%w: a referenced record does not exist", errpkg.ErrInvalid)
	}

	return err
}

// requireLiveCategory rejects a category_id that points at a missing or
// soft-deleted category. The FK only covers existence — a deleted category
// would make the item render uncategorized on the public board (the join
// filters c.deleted_at) while still looking categorized in admin lists.
// An invalid/absent UUID is a no-op (means "no category").
func (s *Service) requireLiveCategory(ctx context.Context, id pgtype.UUID) error {
	if !id.Valid {
		return nil
	}

	cat, err := s.store.getCategoryByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: category does not exist", errpkg.ErrInvalid)
		}

		return err
	}

	if cat.DeletedAt.Valid {
		return fmt.Errorf("%w: category %q is deleted", errpkg.ErrInvalid, cat.Name)
	}

	return nil
}

// slugTaken reports whether a roadmap_categories slug is owned by a row
// other than excludeID — including soft-deleted rows, which still hold the
// UNIQUE slot. excludeID is the zero UUID on create.
func (s *Service) slugTaken(ctx context.Context, slug string, excludeID pgtype.UUID) (bool, error) {
	owner, err := s.store.getCategoryIDBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}

		return false, err
	}

	return !excludeID.Valid || owner.Bytes != excludeID.Bytes, nil
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

	if err := s.enqueuer.Enqueue(worker.TypeRoadmapShipNotify, payload); err != nil {
		slog.Error("enqueue roadmap ship_notify", "item_id", item.ID, "error", err)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// uniqueItemSlug finds an unused roadmap_items slug by appending -2, -3,
// … to the base until a gap appears. getItemIDBySlug sees soft-deleted rows
// too — the slug UNIQUE constraint covers them, so a deleted slug must not
// be handed back out (it would hit 23505 on insert).
func (s *Service) uniqueItemSlug(ctx context.Context, base string) (string, error) {
	candidate := base

	for i := 2; ; i++ {
		_, err := s.store.getItemIDBySlug(ctx, candidate)
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
