// Package planninggroups implements the Japa Planning Groups feature
// (§82, ADR-090).
//
// A Planning Group is a temporary, destination-scoped cohort of users
// preparing to relocate to the same country/city around the same time
// window. Members share milestones, weekly check-ins, and departure
// countdowns. Groups auto-expire after the target departure window to
// keep the cohort focused and prevent stale groups from accumulating.
//
// Tables:
//   - planning_groups            — the group itself
//   - planning_group_members     — membership rows (group_id, user_id)
//   - planning_group_milestones  — per-user milestone tracking
//   - planning_group_checkins    — weekly progress check-ins
//
// Ref: japaconnect-engineering/sections/82-planning-groups.md
//
//	japaconnect-engineering/adr/ADR-090-planning-groups.md
package planninggroups

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
)

// Store is the data-access layer for planning groups and their companion
// tables (members, milestones, check-ins). It uses direct pgx queries
// because these are new tables without sqlc-generated queries yet. No
// business logic here.
type Store struct {
	pool *db.DB
}

// NewStore creates a planning-groups store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{pool: database}
}

// PlanningGroup represents a single planning_groups row.
type PlanningGroup struct {
	ID                    pgtype.UUID
	TenantID              pgtype.UUID
	Name                  string
	Description           *string
	DestinationSpaceID    pgtype.UUID
	VisaCategory          *string
	TargetDepartureWindow *string
	TargetDepartureAt     pgtype.Timestamptz
	MaxMembers            pgtype.Int4
	IsOpenJoin            bool
	OwnerID               pgtype.UUID
	ExpiresAt             pgtype.Timestamptz
	CreatedAt             pgtype.Timestamptz
	UpdatedAt             pgtype.Timestamptz
	DeletedAt             pgtype.Timestamptz
	MemberCount           int64
}

// PlanningGroupMember represents a single planning_group_members row.
type PlanningGroupMember struct {
	GroupID       pgtype.UUID
	UserID        pgtype.UUID
	TenantID      pgtype.UUID
	DepartureDate pgtype.Date
	JoinedAt      pgtype.Timestamptz
	Username      *string
	AvatarURL     *string
}

// PlanningGroupMilestone represents a single planning_group_milestones row.
type PlanningGroupMilestone struct {
	ID          pgtype.UUID
	TenantID    pgtype.UUID
	GroupID     pgtype.UUID
	UserID      pgtype.UUID
	Title       string
	Completed   bool
	CompletedAt pgtype.Timestamptz
	CreatedAt   pgtype.Timestamptz
}

// PlanningGroupCheckin represents a single planning_group_checkins row.
type PlanningGroupCheckin struct {
	ID        pgtype.UUID
	TenantID  pgtype.UUID
	GroupID   pgtype.UUID
	UserID    pgtype.UUID
	Note      string
	CreatedAt pgtype.Timestamptz
	Username  *string
}

// CreateGroup inserts a new planning group and returns the created row.
func (s *Store) CreateGroup(ctx context.Context, group PlanningGroup) (PlanningGroup, error) {
	const q = `
INSERT INTO planning_groups (
    tenant_id, name, description, destination_space_id, visa_category,
    target_departure_window, target_departure_at, max_members, is_open_join,
    owner_id, expires_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING id, tenant_id, name, description, destination_space_id, visa_category,
          target_departure_window, target_departure_at, max_members, is_open_join,
          owner_id, expires_at, created_at, updated_at, deleted_at
`

	var g PlanningGroup

	err := s.pool.Writer.QueryRow(
		ctx, q,
		group.TenantID, group.Name, group.Description, group.DestinationSpaceID,
		group.VisaCategory, group.TargetDepartureWindow, group.TargetDepartureAt,
		group.MaxMembers, group.IsOpenJoin, group.OwnerID, group.ExpiresAt,
	).Scan(
		&g.ID, &g.TenantID, &g.Name, &g.Description, &g.DestinationSpaceID,
		&g.VisaCategory, &g.TargetDepartureWindow, &g.TargetDepartureAt,
		&g.MaxMembers, &g.IsOpenJoin, &g.OwnerID, &g.ExpiresAt,
		&g.CreatedAt, &g.UpdatedAt, &g.DeletedAt,
	)
	if err != nil {
		return PlanningGroup{}, fmt.Errorf("planninggroups: create group: %w", err)
	}

	return g, nil
}

// GetMemberCount returns the number of members in a group.
func (s *Store) GetMemberCount(ctx context.Context, groupID, tenantID pgtype.UUID) (int64, error) {
	var count int64

	err := s.pool.Reader.QueryRow(ctx, `
SELECT COUNT(*) FROM planning_group_members
WHERE group_id = $1 AND tenant_id = $2
`, groupID, tenantID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("planninggroups: get member count: %w", err)
	}

	return count, nil
}

// IsMember returns true if the user is a member of the planning group.
func (s *Store) IsMember(ctx context.Context, groupID, userID, tenantID pgtype.UUID) (bool, error) {
	var exists bool

	err := s.pool.Reader.QueryRow(ctx, `
SELECT EXISTS(SELECT 1 FROM planning_group_members
WHERE group_id = $1 AND user_id = $2 AND tenant_id = $3)
`, groupID, userID, tenantID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("planninggroups: is member: %w", err)
	}

	return exists, nil
}

// GetGroup returns a single planning group by id (must not be soft-deleted).
// Includes a member_count subquery.
func (s *Store) GetGroup(ctx context.Context, id, tenantID pgtype.UUID) (PlanningGroup, error) {
	const q = `
SELECT g.id, g.tenant_id, g.name, g.description, g.destination_space_id, g.visa_category,
       g.target_departure_window, g.target_departure_at, g.max_members, g.is_open_join,
       g.owner_id, g.expires_at, g.created_at, g.updated_at, g.deleted_at,
       COALESCE(m.cnt, 0) AS member_count
FROM planning_groups g
LEFT JOIN (SELECT group_id, COUNT(*) AS cnt FROM planning_group_members GROUP BY group_id) m
  ON m.group_id = g.id
WHERE g.id = $1 AND g.tenant_id = $2 AND g.deleted_at IS NULL
`

	var g PlanningGroup

	err := s.pool.Reader.QueryRow(ctx, q, id, tenantID).Scan(
		&g.ID, &g.TenantID, &g.Name, &g.Description, &g.DestinationSpaceID,
		&g.VisaCategory, &g.TargetDepartureWindow, &g.TargetDepartureAt,
		&g.MaxMembers, &g.IsOpenJoin, &g.OwnerID, &g.ExpiresAt,
		&g.CreatedAt, &g.UpdatedAt, &g.DeletedAt, &g.MemberCount,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PlanningGroup{}, pgx.ErrNoRows
		}

		return PlanningGroup{}, fmt.Errorf("planninggroups: get group: %w", err)
	}

	return g, nil
}

// ListGroupsFilter holds the filter parameters for listing planning groups.
type ListGroupsFilter struct {
	TenantID           pgtype.UUID
	DestinationSpaceID pgtype.UUID // optional
	VisaCategory       string      // optional
	Limit              int32
	Offset             int32
}

// ListGroups returns a paginated list of non-deleted planning groups,
// optionally filtered by destination space and visa category.
func (s *Store) ListGroups(ctx context.Context, f ListGroupsFilter) ([]PlanningGroup, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	conditions = append(conditions, fmt.Sprintf("tenant_id = $%d", argIdx))
	args = append(args, f.TenantID)
	argIdx++

	conditions = append(conditions, "deleted_at IS NULL")

	if f.DestinationSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("destination_space_id = $%d", argIdx))
		args = append(args, f.DestinationSpaceID)
		argIdx++
	}

	if f.VisaCategory != "" {
		conditions = append(conditions, fmt.Sprintf("visa_category = $%d", argIdx))
		args = append(args, f.VisaCategory)
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")

	limit := f.Limit
	if limit <= 0 {
		limit = 20
	}

	if limit > 100 {
		limit = 100
	}

	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	listQ := fmt.Sprintf(`
SELECT g.id, g.tenant_id, g.name, g.description, g.destination_space_id, g.visa_category,
       g.target_departure_window, g.target_departure_at, g.max_members, g.is_open_join,
       g.owner_id, g.expires_at, g.created_at, g.updated_at, g.deleted_at,
       COALESCE(m.cnt, 0) AS member_count
FROM planning_groups g
LEFT JOIN (SELECT group_id, COUNT(*) AS cnt FROM planning_group_members GROUP BY group_id) m
  ON m.group_id = g.id
WHERE %s
ORDER BY g.created_at DESC
LIMIT $%d OFFSET $%d
`, whereClause, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("planninggroups: list groups: %w", err)
	}
	defer rows.Close()

	var out []PlanningGroup

	for rows.Next() {
		var g PlanningGroup
		if err := rows.Scan(
			&g.ID, &g.TenantID, &g.Name, &g.Description, &g.DestinationSpaceID,
			&g.VisaCategory, &g.TargetDepartureWindow, &g.TargetDepartureAt,
			&g.MaxMembers, &g.IsOpenJoin, &g.OwnerID, &g.ExpiresAt,
			&g.CreatedAt, &g.UpdatedAt, &g.DeletedAt, &g.MemberCount,
		); err != nil {
			return nil, 0, fmt.Errorf("planninggroups: scan group: %w", err)
		}

		out = append(out, g)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("planninggroups: iterate groups: %w", err)
	}

	countQ := "SELECT COUNT(*) FROM planning_groups WHERE " + whereClause
	countArgs := args[:len(args)-2]

	var total int64

	if err := s.pool.Reader.QueryRow(ctx, countQ, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("planninggroups: count groups: %w", err)
	}

	return out, total, nil
}

// StoreUpdateInput holds the mutable fields for updating a planning group.
// Only non-nil fields are updated. This is the store-level input using
// pgtype values; the service exposes a JSON-friendly UpdateGroupInput.
type StoreUpdateInput struct {
	Name                  *string
	Description           *string
	DestinationSpaceID    *pgtype.UUID
	VisaCategory          *string
	TargetDepartureWindow *string
	TargetDepartureAt     *pgtype.Timestamptz
	MaxMembers            *int32
	IsOpenJoin            *bool
	ExpiresAt             *pgtype.Timestamptz
}

// UpdateGroup updates the mutable fields on a planning group. Only the
// provided (non-nil) fields are written.
func (s *Store) UpdateGroup(ctx context.Context, id, tenantID pgtype.UUID, input StoreUpdateInput) error {
	var (
		sets   []string
		args   []any
		argIdx = 1
	)

	if input.Name != nil {
		sets = append(sets, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *input.Name)
		argIdx++
	}

	if input.Description != nil {
		sets = append(sets, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *input.Description)
		argIdx++
	}

	if input.DestinationSpaceID != nil {
		sets = append(sets, fmt.Sprintf("destination_space_id = $%d", argIdx))
		args = append(args, *input.DestinationSpaceID)
		argIdx++
	}

	if input.VisaCategory != nil {
		sets = append(sets, fmt.Sprintf("visa_category = $%d", argIdx))
		args = append(args, *input.VisaCategory)
		argIdx++
	}

	if input.TargetDepartureWindow != nil {
		sets = append(sets, fmt.Sprintf("target_departure_window = $%d", argIdx))
		args = append(args, *input.TargetDepartureWindow)
		argIdx++
	}

	if input.TargetDepartureAt != nil {
		sets = append(sets, fmt.Sprintf("target_departure_at = $%d", argIdx))
		args = append(args, *input.TargetDepartureAt)
		argIdx++
	}

	if input.MaxMembers != nil {
		sets = append(sets, fmt.Sprintf("max_members = $%d", argIdx))
		args = append(args, *input.MaxMembers)
		argIdx++
	}

	if input.IsOpenJoin != nil {
		sets = append(sets, fmt.Sprintf("is_open_join = $%d", argIdx))
		args = append(args, *input.IsOpenJoin)
		argIdx++
	}

	if input.ExpiresAt != nil {
		sets = append(sets, fmt.Sprintf("expires_at = $%d", argIdx))
		args = append(args, *input.ExpiresAt)
		argIdx++
	}

	if len(sets) == 0 {
		return nil // nothing to update
	}

	sets = append(sets, "updated_at = now()")
	args = append(args, id, tenantID)

	q := fmt.Sprintf(`
UPDATE planning_groups SET %s WHERE id = $%d AND tenant_id = $%d AND deleted_at IS NULL
`, strings.Join(sets, ", "), argIdx, argIdx+1)

	cmd, err := s.pool.Writer.Exec(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("planninggroups: update group: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// DeleteGroup soft-deletes a planning group by setting deleted_at.
func (s *Store) DeleteGroup(ctx context.Context, id, tenantID pgtype.UUID) error {
	cmd, err := s.pool.Writer.Exec(ctx, `
UPDATE planning_groups SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
`, id, tenantID)
	if err != nil {
		return fmt.Errorf("planninggroups: delete group: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// JoinGroup inserts a membership row for the given user in the given group.
// departureDate is optional (may be NULL).
func (s *Store) JoinGroup(ctx context.Context, groupID, userID, tenantID pgtype.UUID, departureDate pgtype.Date) error {
	_, err := s.pool.Writer.Exec(ctx, `
INSERT INTO planning_group_members (group_id, user_id, tenant_id, departure_date)
VALUES ($1, $2, $3, $4)
ON CONFLICT (group_id, user_id) DO NOTHING
`, groupID, userID, tenantID, departureDate)
	if err != nil {
		return fmt.Errorf("planninggroups: join group: %w", err)
	}

	return nil
}

// LeaveGroup removes a membership row for the given user.
func (s *Store) LeaveGroup(ctx context.Context, groupID, userID, tenantID pgtype.UUID) error {
	cmd, err := s.pool.Writer.Exec(ctx, `
DELETE FROM planning_group_members
WHERE group_id = $1 AND user_id = $2 AND tenant_id = $3
`, groupID, userID, tenantID)
	if err != nil {
		return fmt.Errorf("planninggroups: leave group: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// GetMembers returns all members of a planning group, joining users for
// display name and avatar.
func (s *Store) GetMembers(ctx context.Context, groupID, tenantID pgtype.UUID) ([]PlanningGroupMember, error) {
	const q = `
SELECT m.group_id, m.user_id, m.tenant_id, m.departure_date, m.joined_at,
       u.username, up.avatar_url
FROM planning_group_members m
LEFT JOIN users u ON u.id = m.user_id
LEFT JOIN user_profiles up ON up.user_id = m.user_id
WHERE m.group_id = $1 AND m.tenant_id = $2
ORDER BY m.joined_at ASC
`

	rows, err := s.pool.Reader.Query(ctx, q, groupID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("planninggroups: get members: %w", err)
	}

	defer rows.Close()

	var out []PlanningGroupMember

	for rows.Next() {
		var m PlanningGroupMember
		if err := rows.Scan(
			&m.GroupID, &m.UserID, &m.TenantID, &m.DepartureDate, &m.JoinedAt,
			&m.Username, &m.AvatarURL,
		); err != nil {
			return nil, fmt.Errorf("planninggroups: scan member: %w", err)
		}

		out = append(out, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("planninggroups: iterate members: %w", err)
	}

	return out, nil
}

// CreateMilestone inserts a new milestone for a user in a group.
func (s *Store) CreateMilestone(ctx context.Context, groupID, userID, tenantID pgtype.UUID, title string) (PlanningGroupMilestone, error) {
	const q = `
INSERT INTO planning_group_milestones (tenant_id, group_id, user_id, title)
VALUES ($1, $2, $3, $4)
RETURNING id, tenant_id, group_id, user_id, title, completed, completed_at, created_at
`

	var m PlanningGroupMilestone

	err := s.pool.Writer.QueryRow(ctx, q, tenantID, groupID, userID, title).Scan(
		&m.ID, &m.TenantID, &m.GroupID, &m.UserID, &m.Title,
		&m.Completed, &m.CompletedAt, &m.CreatedAt,
	)
	if err != nil {
		return PlanningGroupMilestone{}, fmt.Errorf("planninggroups: create milestone: %w", err)
	}

	return m, nil
}

// ListMilestones returns all milestones for a user in a group.
func (s *Store) ListMilestones(ctx context.Context, groupID, userID, tenantID pgtype.UUID) ([]PlanningGroupMilestone, error) {
	const q = `
SELECT id, tenant_id, group_id, user_id, title, completed, completed_at, created_at
FROM planning_group_milestones
WHERE group_id = $1 AND user_id = $2 AND tenant_id = $3
ORDER BY created_at ASC
`

	rows, err := s.pool.Reader.Query(ctx, q, groupID, userID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("planninggroups: list milestones: %w", err)
	}

	defer rows.Close()

	var out []PlanningGroupMilestone

	for rows.Next() {
		var m PlanningGroupMilestone
		if err := rows.Scan(
			&m.ID, &m.TenantID, &m.GroupID, &m.UserID, &m.Title,
			&m.Completed, &m.CompletedAt, &m.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("planninggroups: scan milestone: %w", err)
		}

		out = append(out, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("planninggroups: iterate milestones: %w", err)
	}

	return out, nil
}

// UpdateMilestone updates the completed state of a milestone. When marking
// completed, completed_at is stamped; when un-completing, it is cleared.
// The milestone must belong to the given group and user (ownership check).
func (s *Store) UpdateMilestone(ctx context.Context, milestoneID, groupID, userID, tenantID pgtype.UUID, completed bool) error {
	var q string
	if completed {
		q = `
UPDATE planning_group_milestones
SET completed = true, completed_at = now()
WHERE id = $1 AND group_id = $2 AND user_id = $3 AND tenant_id = $4
`
	} else {
		q = `
UPDATE planning_group_milestones
SET completed = false, completed_at = NULL
WHERE id = $1 AND group_id = $2 AND user_id = $3 AND tenant_id = $4
`
	}

	cmd, err := s.pool.Writer.Exec(ctx, q, milestoneID, groupID, userID, tenantID)
	if err != nil {
		return fmt.Errorf("planninggroups: update milestone: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// CountIncompleteMilestones returns the number of milestones for the given
// user in the given group that are not yet completed. Used by the service
// to detect "all milestones complete" after a milestone update (§82
// Integration with §89 Achievement Feed).
func (s *Store) CountIncompleteMilestones(ctx context.Context, groupID, userID, tenantID pgtype.UUID) (int64, error) {
	const q = `
SELECT count(*) FROM planning_group_milestones
WHERE group_id = $1 AND user_id = $2 AND tenant_id = $3 AND completed = false
`

	var count int64

	err := s.pool.Reader.QueryRow(ctx, q, groupID, userID, tenantID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("planninggroups: count incomplete milestones: %w", err)
	}

	return count, nil
}

// ErrAlreadyCheckedIn is returned when a user submits a second check-in in
// the same week. The weekly unique index idx_planning_group_checkins_weekly
// enforces one check-in per user per group per week (§82).
var ErrAlreadyCheckedIn = errors.New("already checked in this week")

// CreateCheckin inserts a new weekly check-in for a user in a group.
// Returns ErrAlreadyCheckedIn if the user has already checked in this week
// (unique violation on idx_planning_group_checkins_weekly).
func (s *Store) CreateCheckin(ctx context.Context, groupID, userID, tenantID pgtype.UUID, note string) (PlanningGroupCheckin, error) {
	const q = `
INSERT INTO planning_group_checkins (tenant_id, group_id, user_id, note)
VALUES ($1, $2, $3, $4)
RETURNING id, tenant_id, group_id, user_id, note, created_at
`

	var c PlanningGroupCheckin

	err := s.pool.Writer.QueryRow(ctx, q, tenantID, groupID, userID, note).Scan(
		&c.ID, &c.TenantID, &c.GroupID, &c.UserID, &c.Note, &c.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return PlanningGroupCheckin{}, ErrAlreadyCheckedIn
		}

		return PlanningGroupCheckin{}, fmt.Errorf("planninggroups: create checkin: %w", err)
	}

	return c, nil
}

// ListCheckins returns this week's check-ins for a group, joining users for
// display name. "This week" is the trailing 7 days from now.
func (s *Store) ListCheckins(ctx context.Context, groupID, tenantID pgtype.UUID) ([]PlanningGroupCheckin, error) {
	const q = `
SELECT c.id, c.tenant_id, c.group_id, c.user_id, c.note, c.created_at,
       u.username
FROM planning_group_checkins c
LEFT JOIN users u ON u.id = c.user_id
WHERE c.group_id = $1 AND c.tenant_id = $2
  AND c.created_at >= now() - interval '7 days'
ORDER BY c.created_at DESC
`

	rows, err := s.pool.Reader.Query(ctx, q, groupID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("planninggroups: list checkins: %w", err)
	}

	defer rows.Close()

	var out []PlanningGroupCheckin

	for rows.Next() {
		var c PlanningGroupCheckin
		if err := rows.Scan(
			&c.ID, &c.TenantID, &c.GroupID, &c.UserID, &c.Note, &c.CreatedAt,
			&c.Username,
		); err != nil {
			return nil, fmt.Errorf("planninggroups: scan checkin: %w", err)
		}

		out = append(out, c)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("planninggroups: iterate checkins: %w", err)
	}

	return out, nil
}

// ExtendExpiry updates the expires_at timestamp on a planning group. Used by
// admins to keep a group alive past its original auto-expiry.
func (s *Store) ExtendExpiry(ctx context.Context, id, tenantID pgtype.UUID, newExpiry pgtype.Timestamptz) error {
	cmd, err := s.pool.Writer.Exec(ctx, `
UPDATE planning_groups SET expires_at = $3, updated_at = now()
WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
`, id, tenantID, newExpiry)
	if err != nil {
		return fmt.Errorf("planninggroups: extend expiry: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// AdminListGroupsFilter holds the filter parameters for the admin list.
type AdminListGroupsFilter struct {
	TenantID           pgtype.UUID
	DestinationSpaceID pgtype.UUID // optional
	Search             string      // optional — case-insensitive ILIKE on name
	Limit              int32
	Offset             int32
}

// AdminListGroups returns a paginated list of all planning groups for the
// admin review queue (including soft-deleted ones), with member_count.
// Supports optional destination filter and name search.
func (s *Store) AdminListGroups(ctx context.Context, f AdminListGroupsFilter) ([]PlanningGroup, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	conditions = append(conditions, fmt.Sprintf("g.tenant_id = $%d", argIdx))
	args = append(args, f.TenantID)
	argIdx++

	if f.DestinationSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("g.destination_space_id = $%d", argIdx))
		args = append(args, f.DestinationSpaceID)
		argIdx++
	}

	if f.Search != "" {
		conditions = append(conditions, fmt.Sprintf("g.name ILIKE $%d", argIdx))
		args = append(args, "%"+f.Search+"%")
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")

	limit := f.Limit
	if limit <= 0 {
		limit = 25
	}

	if limit > 100 {
		limit = 100
	}

	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	listQ := fmt.Sprintf(`
SELECT g.id, g.tenant_id, g.name, g.description, g.destination_space_id, g.visa_category,
       g.target_departure_window, g.target_departure_at, g.max_members, g.is_open_join,
       g.owner_id, g.expires_at, g.created_at, g.updated_at, g.deleted_at,
       COALESCE(m.cnt, 0) AS member_count
FROM planning_groups g
LEFT JOIN (SELECT group_id, COUNT(*) AS cnt FROM planning_group_members GROUP BY group_id) m
  ON m.group_id = g.id
WHERE %s
ORDER BY g.created_at DESC
LIMIT $%d OFFSET $%d
`, whereClause, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("planninggroups: admin list groups: %w", err)
	}
	defer rows.Close()

	var out []PlanningGroup

	for rows.Next() {
		var g PlanningGroup
		if err := rows.Scan(
			&g.ID, &g.TenantID, &g.Name, &g.Description, &g.DestinationSpaceID,
			&g.VisaCategory, &g.TargetDepartureWindow, &g.TargetDepartureAt,
			&g.MaxMembers, &g.IsOpenJoin, &g.OwnerID, &g.ExpiresAt,
			&g.CreatedAt, &g.UpdatedAt, &g.DeletedAt, &g.MemberCount,
		); err != nil {
			return nil, 0, fmt.Errorf("planninggroups: admin scan group: %w", err)
		}

		out = append(out, g)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("planninggroups: admin iterate groups: %w", err)
	}

	countQ := fmt.Sprintf(`
SELECT COUNT(*) FROM planning_groups g WHERE %s
`, whereClause)
	countArgs := args[:len(args)-2]

	var total int64

	if err := s.pool.Reader.QueryRow(ctx, countQ, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("planninggroups: admin count groups: %w", err)
	}

	return out, total, nil
}

// ExpireGroups soft-deletes planning groups whose expires_at has passed.
// Returns the number of groups expired. Called nightly by the
// planning_group:expire Asynq task (§82, ADR-090).
func (s *Store) ExpireGroups(ctx context.Context) (int64, error) {
	cmd, err := s.pool.Writer.Exec(ctx, `
UPDATE planning_groups
SET deleted_at = now(), updated_at = now()
WHERE expires_at IS NOT NULL
  AND expires_at < now()
  AND deleted_at IS NULL
`)
	if err != nil {
		return 0, fmt.Errorf("planninggroups: expire groups: %w", err)
	}

	return cmd.RowsAffected(), nil
}

// ExpiringGroupOwner is used by SendCheckinReminders to identify members
// who haven't checked in this week.
type MemberWithoutCheckin struct {
	GroupID  pgtype.UUID
	UserID   pgtype.UUID
	TenantID pgtype.UUID
}

// MembersWithoutWeeklyCheckin returns group members who haven't submitted
// a check-in in the current week. Used by the planning_group:checkin_reminder
// Asynq task (§82, ADR-090) to send weekly reminders.
func (s *Store) MembersWithoutWeeklyCheckin(ctx context.Context) ([]MemberWithoutCheckin, error) {
	const q = `
SELECT m.group_id, m.user_id, m.tenant_id
FROM planning_group_members m
JOIN planning_groups g ON g.id = m.group_id AND g.deleted_at IS NULL
WHERE NOT EXISTS (
    SELECT 1 FROM planning_group_checkins c
    WHERE c.group_id = m.group_id
      AND c.user_id = m.user_id
      AND c.tenant_id = m.tenant_id
      AND c.created_at >= date_trunc('week', now())
)
`

	rows, err := s.pool.Reader.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("planninggroups: members without checkin: %w", err)
	}

	defer rows.Close()

	var out []MemberWithoutCheckin

	for rows.Next() {
		var m MemberWithoutCheckin
		if err := rows.Scan(&m.GroupID, &m.UserID, &m.TenantID); err != nil {
			return nil, fmt.Errorf("planninggroups: scan member without checkin: %w", err)
		}

		out = append(out, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("planninggroups: iterate members without checkin: %w", err)
	}

	return out, nil
}

// InviteMember inserts a membership row for an invited user. Used by the
// POST /planning-groups/:id/invite endpoint (owner only, §82).
func (s *Store) InviteMember(ctx context.Context, groupID, userID, tenantID pgtype.UUID) error {
	_, err := s.pool.Writer.Exec(ctx, `
INSERT INTO planning_group_members (group_id, user_id, tenant_id)
VALUES ($1, $2, $3)
ON CONFLICT (group_id, user_id) DO NOTHING
`, groupID, userID, tenantID)
	if err != nil {
		return fmt.Errorf("planninggroups: invite member: %w", err)
	}

	return nil
}
