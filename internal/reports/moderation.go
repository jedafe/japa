// Moderator workbench (spec .jeos/specs/2026-09-23-studio-moderation-tab.md).
// The /me/moderation/* surface gives space moderators/owners, tag moderators
// (universal tags — including forum tags since migration 000719), assignees,
// and platform moderators a scoped view of the content_reports queue plus the
// actions they are allowed to take.
package reports

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/rbac"
)

// ErrActionNotAllowed is returned when the caller can see the report but the
// requested action exceeds their scope (tag-scoped moderators cannot warn/ban).
var ErrActionNotAllowed = errors.New("action not allowed for this moderation scope")

// ErrAlreadyResolved is returned when an action targets a report that is no
// longer open (already actioned or dismissed) — including the race where a
// second moderator resolves it between the queue read and the PATCH.
var ErrAlreadyResolved = errors.New("report already resolved")

// ScopedEntity is a moderated space or tag with display fields for the
// frontend filter dropdowns.
type ScopedEntity struct {
	ID   pgtype.UUID `json:"id"`
	Name string      `json:"name"`
	Slug string      `json:"slug"`
}

// ModerationScope is the caller's moderation envelope — the union of every
// scope that makes queue items visible to them.
type ModerationScope struct {
	PlatformModerator bool           `json:"platform_moderator"`
	ModeratedSpaces   []ScopedEntity `json:"moderated_spaces"`
	ModeratedTags     []ScopedEntity `json:"moderated_tags"`
	AssignedCount     int32          `json:"assigned_count"`
	OpenCount         int32          `json:"open_count"`
}

// Active reports whether the caller has any moderation scope at all — drives
// the /studio Moderation tab.
func (s *ModerationScope) Active() bool {
	return s.PlatformModerator ||
		len(s.ModeratedSpaces) > 0 ||
		len(s.ModeratedTags) > 0 ||
		s.AssignedCount > 0
}

func scopedIDs(entities []ScopedEntity) []pgtype.UUID {
	ids := make([]pgtype.UUID, len(entities))
	for i, e := range entities {
		ids[i] = e.ID
	}
	return ids
}

// SetRBACResolver injects the permission resolver used to detect platform
// moderators (admin.moderation / reports.review / wildcard).
func (s *Service) SetRBACResolver(r *rbac.RBAC) {
	s.rbac = r
}

// resolveScopeEntities loads the caller's platform flag and moderated
// space/tag sets — the inputs the queue queries authorize against. Kept
// separate from ResolveModerationScope so the mutation path can re-resolve
// fresh at action time without paying for the count queries.
func (s *Service) resolveScopeEntities(ctx context.Context, userID pgtype.UUID) (*ModerationScope, error) {
	scope := &ModerationScope{
		ModeratedSpaces: []ScopedEntity{},
		ModeratedTags:   []ScopedEntity{},
	}

	if s.rbac != nil {
		scope.PlatformModerator = s.rbac.HasPermission(ctx, userID, "admin.moderation") ||
			s.rbac.HasPermission(ctx, userID, "reports.review")
	}

	spaces, err := s.read.ListModeratedSpaces(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("listing moderated spaces: %w", err)
	}
	for _, sp := range spaces {
		scope.ModeratedSpaces = append(scope.ModeratedSpaces, ScopedEntity{ID: sp.ID, Name: sp.Name, Slug: sp.Slug})
	}

	tags, err := s.read.ListModeratedTags(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("listing moderated tags: %w", err)
	}
	for _, tg := range tags {
		scope.ModeratedTags = append(scope.ModeratedTags, ScopedEntity{ID: tg.ID, Name: tg.Name, Slug: tg.Slug})
	}

	return scope, nil
}

// ResolveModerationScope builds the caller's moderation envelope.
func (s *Service) ResolveModerationScope(ctx context.Context, userID pgtype.UUID) (*ModerationScope, error) {
	scope, err := s.resolveScopeEntities(ctx, userID)
	if err != nil {
		return nil, err
	}

	assigned, err := s.read.CountAssignedReports(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("counting assigned reports: %w", err)
	}
	scope.AssignedCount = assigned

	// Short-circuit: callers with no scope see an empty queue — skip the
	// full-table scope CTE (this runs on every /studio layout load).
	if !scope.Active() {
		return scope, nil
	}

	open, err := s.read.CountModerationQueue(ctx, gen.CountModerationQueueParams{
		Platform: scope.PlatformModerator,
		OpenOnly: true,
		Caller:   userID,
		SpaceIds: scopedIDs(scope.ModeratedSpaces),
		TagIds:   scopedIDs(scope.ModeratedTags),
	})
	if err != nil {
		return nil, fmt.Errorf("counting open moderation items: %w", err)
	}
	scope.OpenCount = open

	return scope, nil
}

// ModerationQueueFilter holds the optional list filters for the scoped queue.
type ModerationQueueFilter struct {
	Status       *string
	EntityType   *string
	SpaceID      pgtype.UUID
	TagID        pgtype.UUID
	AssignedOnly bool
	Q            *string
}

func (s *Service) queueParams(scope *ModerationScope, caller pgtype.UUID, f ModerationQueueFilter) (gen.ListModerationQueueParams, gen.CountModerationQueueParams) {
	list := gen.ListModerationQueueParams{
		Platform:      scope.PlatformModerator,
		Status:        f.Status,
		EntityType:    f.EntityType,
		FilterSpaceID: f.SpaceID,
		FilterTagID:   f.TagID,
		AssignedOnly:  f.AssignedOnly,
		Q:             f.Q,
		Caller:        caller,
		SpaceIds:      scopedIDs(scope.ModeratedSpaces),
		TagIds:        scopedIDs(scope.ModeratedTags),
	}
	count := gen.CountModerationQueueParams{
		Platform:      list.Platform,
		Status:        list.Status,
		EntityType:    list.EntityType,
		FilterSpaceID: list.FilterSpaceID,
		FilterTagID:   list.FilterTagID,
		AssignedOnly:  list.AssignedOnly,
		Q:             list.Q,
		Caller:        list.Caller,
		SpaceIds:      list.SpaceIds,
		TagIds:        list.TagIds,
	}
	return list, count
}

// ListModerationQueue returns the caller-scoped queue page.
func (s *Service) ListModerationQueue(ctx context.Context, scope *ModerationScope, caller pgtype.UUID, f ModerationQueueFilter, limit, offset int32) ([]gen.ListModerationQueueRow, error) {
	list, _ := s.queueParams(scope, caller, f)
	list.PageLimit = limit
	list.PageOffset = offset

	rows, err := s.read.ListModerationQueue(ctx, list)
	if err != nil {
		return nil, fmt.Errorf("listing moderation queue: %w", err)
	}
	// JSON contract: an empty page must serialize as [] — a nil slice
	// marshals as null and crashes consumers that read items.length.
	if rows == nil {
		rows = []gen.ListModerationQueueRow{}
	}
	return rows, nil
}

// CountModerationQueue returns the scoped total for pagination.
func (s *Service) CountModerationQueue(ctx context.Context, scope *ModerationScope, caller pgtype.UUID, f ModerationQueueFilter) (int32, error) {
	_, count := s.queueParams(scope, caller, f)
	total, err := s.read.CountModerationQueue(ctx, count)
	if err != nil {
		return 0, fmt.Errorf("counting moderation queue: %w", err)
	}
	return total, nil
}

// GetModerationQueueItem returns a single report with its scope flags for the
// caller. ErrNotFound when the row does not exist or is out of scope.
func (s *Service) GetModerationQueueItem(ctx context.Context, scope *ModerationScope, caller, id pgtype.UUID) (gen.GetModerationQueueItemRow, error) {
	row, err := s.read.GetModerationQueueItem(ctx, gen.GetModerationQueueItemParams{
		Caller:   caller,
		SpaceIds: scopedIDs(scope.ModeratedSpaces),
		TagIds:   scopedIDs(scope.ModeratedTags),
		ID:       id,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return row, errpkg.ErrNotFound
	}
	if err != nil {
		return row, fmt.Errorf("getting moderation item: %w", err)
	}
	if !scope.PlatformModerator && !row.ScopeAssigned && !row.ScopeSpace && !row.ScopeTag {
		return row, errpkg.ErrNotFound
	}
	return row, nil
}

// ActionModerationItem applies an action to a scoped report.
// Actions: dismiss (status='dismissed'), remove/warn/ban (status='actioned').
// warn/ban additionally fire the user-moderation side effects and require
// platform, assigned, or space scope — tag scope alone grants content-level
// actions only (spec §4).
//
// Scope is re-resolved fresh here (not taken from the handler) so a moderator
// whose grants were revoked between page load and PATCH is denied at action
// time. The status transition is atomic: ResolveReportIfOpen only updates
// rows still pending/reviewing, so concurrent actions can't double-fire.
func (s *Service) ActionModerationItem(ctx context.Context, caller, id pgtype.UUID, action, reason string) (gen.ContentReport, error) {
	scope, err := s.resolveScopeEntities(ctx, caller)
	if err != nil {
		return gen.ContentReport{}, err
	}

	item, err := s.GetModerationQueueItem(ctx, scope, caller, id)
	if err != nil {
		return gen.ContentReport{}, err
	}

	if item.Status != "pending" && item.Status != "reviewing" {
		return gen.ContentReport{}, ErrAlreadyResolved
	}

	switch action {
	case "dismiss", "remove":
		// Any scope may resolve a visible report.
	case "warn", "ban":
		if !scope.PlatformModerator && !item.ScopeAssigned && !item.ScopeSpace {
			return gen.ContentReport{}, ErrActionNotAllowed
		}
	default:
		return gen.ContentReport{}, fmt.Errorf("%w: %s", ErrInvalidStatus, action)
	}

	status := "actioned"
	if action == "dismiss" {
		status = "dismissed"
	}

	var note *string
	if reason != "" {
		note = &reason
	}

	updated, err := s.write.ResolveReportIfOpen(ctx, gen.ResolveReportIfOpenParams{
		Status:         status,
		ReviewedBy:     caller,
		ResolutionNote: note,
		ID:             id,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.ContentReport{}, ErrAlreadyResolved
	}
	if err != nil {
		return gen.ContentReport{}, fmt.Errorf("resolving report: %w", err)
	}

	// Side effects are best-effort: the status transition is already committed,
	// so a failed warn/ban logs and still returns the actioned report — same
	// semantics as the admin UpdateReport handler.
	if err := s.applyActionSideEffects(ctx, action, item.EntityType, item.EntityID, reason); err != nil {
		slog.Warn("moderation: side-effect failed", "id", id.String(), "action", action, "error", err)
	}

	return updated, nil
}

// applyActionSideEffects fires the warn/ban user-moderation effects shared with
// the admin report handler. The target is the reported user (resolved via
// ResolveTargetUser), never the reporter. Missing targets are skipped.
func (s *Service) applyActionSideEffects(ctx context.Context, action, entityType string, entityID pgtype.UUID, reason string) error {
	if action != "warn" && action != "ban" {
		return nil
	}

	target, found, err := s.ResolveTargetUser(ctx, entityType, entityID)
	if err != nil {
		return fmt.Errorf("resolve %s target: %w", action, err)
	}
	if !found {
		return nil
	}

	if action == "warn" {
		if err := s.WarnUser(ctx, target, reason); err != nil {
			return fmt.Errorf("warn user: %w", err)
		}
		return nil
	}
	if err := s.BanUser(ctx, target, reason); err != nil {
		return fmt.Errorf("ban user: %w", err)
	}
	return nil
}
