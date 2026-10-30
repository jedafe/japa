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
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/rbac"
	"github.com/jedafe/japa/internal/wallet"
)

// ErrActionNotAllowed is returned when the caller can see the report but the
// requested action exceeds their scope (tag-scoped moderators cannot warn/ban).
var ErrActionNotAllowed = errors.New("action not allowed for this moderation scope")

// ErrAlreadyResolved is returned when an action targets a report that is no
// longer open (already actioned or dismissed) — including the race where a
// second moderator resolves it between the queue read and the PATCH.
var ErrAlreadyResolved = errors.New("report already resolved")

// ErrInvalidAction is returned when the PATCH body's action is not one of
// dismiss/remove/warn/ban.
var ErrInvalidAction = errors.New("invalid moderation action")

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
// ContentType filters on the resolved entity's contents.content_type (reports
// on a story, or a comment under one) — same axis as /studio/contents' type
// filter, distinct from EntityType (the report target kind).
type ModerationQueueFilter struct {
	Status       *string
	EntityType   *string
	ContentType  *string
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
		ContentType:   f.ContentType,
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
		ContentType:   list.ContentType,
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

// removableEntityTypes lists entity types the "remove" action can take down:
// user/space/source reports resolve with warn/ban or dismissal instead.
var removableEntityTypes = map[string]bool{
	EntityTypeContent:    true,
	EntityTypeComment:    true,
	EntityTypeMessage:    true,
	EntityTypeForumReply: true,
}

// resolveWithAction is the shared terminal-action path for both the scoped
// moderator queue and the admin reports handler. It validates the action
// against the entity (removable types for remove; resolvable user target for
// warn/ban), applies entity-level side effects for remove *before* the status
// transition so a failed removal leaves the report open, then atomically
// resolves via ResolveReportIfOpen and fires warn/ban user side effects.
func (s *Service) resolveWithAction(ctx context.Context, id, reviewerID pgtype.UUID, entityType string, entityID pgtype.UUID, action, reason string) (gen.ContentReport, error) {
	var target pgtype.UUID
	var hasTarget bool

	switch action {
	case "dismiss":
		// Always applicable.
	case "remove":
		if !removableEntityTypes[entityType] {
			return gen.ContentReport{}, fmt.Errorf("%w: %s", ErrActionNotApplicable, entityType)
		}
	case "warn", "ban":
		t, found, err := s.ResolveTargetUser(ctx, entityType, entityID)
		if err != nil {
			return gen.ContentReport{}, fmt.Errorf("resolve %s target: %w", action, err)
		}
		if !found {
			return gen.ContentReport{}, ErrNoTarget
		}
		target, hasTarget = t, true
	default:
		return gen.ContentReport{}, fmt.Errorf("%w: %s", ErrInvalidAction, action)
	}

	// Removal and the status transition run in one transaction: if the
	// transition fails (another moderator resolved concurrently) the removal
	// rolls back — a dismissed report never leaves deleted content behind.
	status := "actioned"
	if action == "dismiss" {
		status = "dismissed"
	}

	var note *string
	if reason != "" {
		note = &reason
	}

	updated, err := db.WithTxResult(ctx, s.db.Writer, func(q *gen.Queries) (gen.ContentReport, error) {
		if action == "remove" {
			if err := s.removeReportedEntity(ctx, q, entityType, entityID); err != nil {
				return gen.ContentReport{}, err
			}
		}
		row, err := q.ResolveReportIfOpen(ctx, gen.ResolveReportIfOpenParams{
			Status:         status,
			ReviewedBy:     reviewerID,
			ResolutionNote: note,
			ID:             id,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.ContentReport{}, ErrAlreadyResolved
		}
		return row, err
	})
	if err != nil {
		if errors.Is(err, ErrAlreadyResolved) {
			return gen.ContentReport{}, err
		}
		return gen.ContentReport{}, fmt.Errorf("resolving report: %w", err)
	}

	// warn/ban side effects are best-effort: the status transition is
	// committed and the target was validated, so a downstream failure logs
	// and still returns the actioned report.
	if hasTarget {
		var sideErr error
		if action == "warn" {
			sideErr = s.WarnUser(ctx, target, reason)
		} else {
			sideErr = s.BanUser(ctx, target, reason)
		}
		if sideErr != nil {
			slog.Warn("moderation: side-effect failed", "id", id.String(), "action", action, "error", sideErr)
		}
	}

	// ADR-136: upheld moderation deducts the offender's points —
	// report.upheld for warn/ban; content.flagged_removed for removals
	// (deduped per report id).
	s.emitModerationPenalty(ctx, action, entityType, entityID, target, hasTarget, id)

	return updated, nil
}

// emitModerationPenalty deducts points after an upheld moderation action —
// fire-and-forget (Rule 8). warn/ban hit the resolved target user
// (report.upheld); removals hit the reported entity's author
// (content.flagged_removed).
func (s *Service) emitModerationPenalty(ctx context.Context, action, entityType string, entityID, target pgtype.UUID, hasTarget bool, reportID pgtype.UUID) {
	if s.activity == nil {
		return
	}

	var penalized pgtype.UUID
	var key string
	switch action {
	case "warn", "ban":
		if !hasTarget {
			return
		}
		penalized, key = target, wallet.ActivityKeyReportUpheld
	case "remove":
		t, found, err := s.ResolveTargetUser(ctx, entityType, entityID)
		if err != nil || !found {
			return
		}
		penalized, key = t, wallet.ActivityKeyContentFlagged
	default:
		return
	}
	if !penalized.Valid {
		return
	}

	event, err := s.activity.EmitActivityEvent(ctx, penalized, wallet.EmitActivityEventInput{
		ActivityKey: key,
		Reference:   "report:" + reportID.String(),
	})
	if err != nil {
		return
	}
	_ = s.activity.EnqueueProcessActivityEvent(ctx, event.ID)
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
		return gen.ContentReport{}, fmt.Errorf("%w: %s", ErrInvalidAction, action)
	}

	return s.resolveWithAction(ctx, id, caller, item.EntityType, item.EntityID, action, reason)
}

// ApplyAdminAction is the admin-path analogue of ActionModerationItem: it
// applies a terminal action (dismiss/remove/warn/ban) via the shared
// resolveWithAction flow, or marks the report 'reviewing' (non-terminal)
// when the caller starts work on it.
func (s *Service) ApplyAdminAction(ctx context.Context, id, reviewerID pgtype.UUID, action, reason string) (gen.ContentReport, error) {
	report, err := s.GetReport(ctx, id)
	if err != nil {
		return gen.ContentReport{}, err
	}

	if action == "review" {
		var note *string
		if reason != "" {
			note = &reason
		}
		// ResolveReportIfOpen's guard makes the transition atomic — a
		// concurrently-actioned report can't be flipped back to reviewing.
		row, err := s.write.ResolveReportIfOpen(ctx, gen.ResolveReportIfOpenParams{
			Status:         "reviewing",
			ReviewedBy:     reviewerID,
			ResolutionNote: note,
			ID:             id,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.ContentReport{}, ErrAlreadyResolved
		}
		return row, err
	}

	return s.resolveWithAction(ctx, id, reviewerID, report.EntityType, report.EntityID, action, reason)
}
