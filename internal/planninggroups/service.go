// Package planninggroups — service.go orchestrates planning group creation,
// membership, milestones, weekly check-ins, and admin expiry extension.
// Feature-flagged via planning_group.enabled system_setting (§82).
//
// Ref: japaconnect-engineering/sections/82-planning-groups.md
//      japaconnect-engineering/adr/ADR-090-planning-groups.md
package planninggroups

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
)

// PlanningGroupDTO is the API response shape for a single planning group.
type PlanningGroupDTO struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	Description           string `json:"description,omitempty"`
	DestinationSpaceID    string `json:"destination_space_id"`
	VisaCategory          string `json:"visa_category,omitempty"`
	TargetDepartureWindow string `json:"target_departure_window,omitempty"`
	TargetDepartureAt     string `json:"target_departure_at,omitempty"`
	MaxMembers            int32  `json:"max_members,omitempty"`
	IsOpenJoin            bool   `json:"is_open_join"`
	OwnerID               string `json:"owner_id"`
	ExpiresAt             string `json:"expires_at,omitempty"`
	CreatedAt             string `json:"created_at"`
	UpdatedAt             string `json:"updated_at,omitempty"`
}

// MemberDTO is the API response shape for a planning group member.
type MemberDTO struct {
	UserID        string `json:"user_id"`
	DepartureDate string `json:"departure_date,omitempty"`
	JoinedAt      string `json:"joined_at"`
	Username      string `json:"username,omitempty"`
	AvatarURL     string `json:"avatar_url,omitempty"`
}

// MilestoneDTO is the API response shape for a planning group milestone.
type MilestoneDTO struct {
	ID          string `json:"id"`
	GroupID     string `json:"group_id"`
	UserID      string `json:"user_id"`
	Title       string `json:"title"`
	Completed   bool   `json:"completed"`
	CompletedAt string `json:"completed_at,omitempty"`
	CreatedAt   string `json:"created_at"`
}

// CheckinDTO is the API response shape for a planning group check-in.
type CheckinDTO struct {
	ID        string `json:"id"`
	GroupID   string `json:"group_id"`
	UserID    string `json:"user_id"`
	Note      string `json:"note"`
	CreatedAt string `json:"created_at"`
	Username  string `json:"username,omitempty"`
}

// CreateGroupInput is the request body for POST /planning-groups.
type CreateGroupInput struct {
	Name                  string `json:"name"`
	Description           string `json:"description"`
	DestinationSpaceID    string `json:"destination_space_id"`
	VisaCategory          string `json:"visa_category"`
	TargetDepartureWindow string `json:"target_departure_window"`
	TargetDepartureAt     string `json:"target_departure_at"`
	MaxMembers            int32  `json:"max_members"`
	IsOpenJoin            bool   `json:"is_open_join"`
	ExpiresAt             string `json:"expires_at"`
}

// UpdateGroupInput is the request body for PATCH /planning-groups/:id.
type UpdateGroupInput struct {
	Name                  *string `json:"name"`
	Description           *string `json:"description"`
	VisaCategory          *string `json:"visa_category"`
	TargetDepartureWindow *string `json:"target_departure_window"`
	TargetDepartureAt     *string `json:"target_departure_at"`
	MaxMembers            *int32  `json:"max_members"`
	IsOpenJoin            *bool   `json:"is_open_join"`
	ExpiresAt             *string `json:"expires_at"`
}

// ErrNotOwner is returned when a user tries to update or delete a planning
// group they did not create.
var ErrNotOwner = fmt.Errorf("not the planning group owner")

// Service holds the business logic for Japa Planning Groups.
type Service struct {
	store    *Store
	settings *config.Settings
}

// NewService creates a planning-groups service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{store: store, settings: settings}
}

// Enabled reads the planning_group.enabled system_setting toggle (§82).
func (s *Service) Enabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return false
	}
	return s.settings.GetBoolOr(ctx, tenantID, "planning_group.enabled", false)
}

// Create creates a new planning group owned by the requesting user. The
// caller must have the planning_group.create permission (checked at route
// registration).
func (s *Service) Create(ctx context.Context, tenantID, ownerID pgtype.UUID, input CreateGroupInput) (PlanningGroupDTO, error) {
	if input.Name == "" || input.DestinationSpaceID == "" {
		return PlanningGroupDTO{}, fmt.Errorf("name and destination_space_id are required")
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
		return PlanningGroupDTO{}, fmt.Errorf("invalid destination_space_id: %w", err)
	}

	var descPtr *string
	if input.Description != "" {
		descPtr = &input.Description
	}
	var visaPtr *string
	if input.VisaCategory != "" {
		visaPtr = &input.VisaCategory
	}
	var windowPtr *string
	if input.TargetDepartureWindow != "" {
		windowPtr = &input.TargetDepartureWindow
	}

	var targetDepartureAt pgtype.Timestamptz
	if input.TargetDepartureAt != "" {
		t, err := time.Parse(time.RFC3339, input.TargetDepartureAt)
		if err != nil {
			return PlanningGroupDTO{}, fmt.Errorf("invalid target_departure_at: %w", err)
		}
		targetDepartureAt = pgtype.Timestamptz{Time: t, Valid: true}
	}

	var maxMembers pgtype.Int4
	if input.MaxMembers > 0 {
		maxMembers = pgtype.Int4{Int32: input.MaxMembers, Valid: true}
	}

	var expiresAt pgtype.Timestamptz
	if input.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, input.ExpiresAt)
		if err != nil {
			return PlanningGroupDTO{}, fmt.Errorf("invalid expires_at: %w", err)
		}
		expiresAt = pgtype.Timestamptz{Time: t, Valid: true}
	}

	group, err := s.store.CreateGroup(ctx, PlanningGroup{
		TenantID:              tenantID,
		Name:                  input.Name,
		Description:           descPtr,
		DestinationSpaceID:    destSpaceID,
		VisaCategory:          visaPtr,
		TargetDepartureWindow: windowPtr,
		TargetDepartureAt:     targetDepartureAt,
		MaxMembers:            maxMembers,
		IsOpenJoin:            input.IsOpenJoin,
		OwnerID:               ownerID,
		ExpiresAt:             expiresAt,
	})
	if err != nil {
		return PlanningGroupDTO{}, fmt.Errorf("planninggroups: create: %w", err)
	}

	// The owner is automatically a member of their own group.
	if err := s.store.JoinGroup(ctx, group.ID, ownerID, tenantID, pgtype.Date{}); err != nil {
		return PlanningGroupDTO{}, fmt.Errorf("planninggroups: create: auto-join owner: %w", err)
	}

	return groupToDTO(group), nil
}

// Get returns a single planning group by id.
func (s *Service) Get(ctx context.Context, id, tenantID pgtype.UUID) (PlanningGroupDTO, error) {
	group, err := s.store.GetGroup(ctx, id, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return PlanningGroupDTO{}, pgx.ErrNoRows
		}
		return PlanningGroupDTO{}, fmt.Errorf("planninggroups: get: %w", err)
	}
	return groupToDTO(group), nil
}

// List returns a paginated list of planning groups, optionally filtered by
// destination space and visa category.
func (s *Service) List(ctx context.Context, tenantID, destSpaceID pgtype.UUID, visaCategory string, page, limit int32) ([]PlanningGroupDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	groups, total, err := s.store.ListGroups(ctx, ListGroupsFilter{
		TenantID:           tenantID,
		DestinationSpaceID: destSpaceID,
		VisaCategory:       visaCategory,
		Limit:              limit,
		Offset:             offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("planninggroups: list: %w", err)
	}

	dtos := make([]PlanningGroupDTO, 0, len(groups))
	for _, g := range groups {
		dtos = append(dtos, groupToDTO(g))
	}
	return dtos, total, nil
}

// UpdateOwn updates a planning group owned by the requesting user. The
// caller must verify ownership (planning_group.manage_own permission +
// owner_id match).
func (s *Service) UpdateOwn(ctx context.Context, id, tenantID, userID pgtype.UUID, input UpdateGroupInput) (PlanningGroupDTO, error) {
	existing, err := s.store.GetGroup(ctx, id, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return PlanningGroupDTO{}, pgx.ErrNoRows
		}
		return PlanningGroupDTO{}, fmt.Errorf("planninggroups: update: get: %w", err)
	}
	if existing.OwnerID != userID {
		return PlanningGroupDTO{}, ErrNotOwner
	}

	var storeInput StoreUpdateInput
	storeInput.Name = input.Name
	storeInput.Description = input.Description
	storeInput.VisaCategory = input.VisaCategory
	storeInput.TargetDepartureWindow = input.TargetDepartureWindow
	storeInput.MaxMembers = input.MaxMembers
	storeInput.IsOpenJoin = input.IsOpenJoin

	if input.TargetDepartureAt != nil && *input.TargetDepartureAt != "" {
		t, err := time.Parse(time.RFC3339, *input.TargetDepartureAt)
		if err != nil {
			return PlanningGroupDTO{}, fmt.Errorf("invalid target_departure_at: %w", err)
		}
		ts := pgtype.Timestamptz{Time: t, Valid: true}
		storeInput.TargetDepartureAt = &ts
	}
	if input.ExpiresAt != nil && *input.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, *input.ExpiresAt)
		if err != nil {
			return PlanningGroupDTO{}, fmt.Errorf("invalid expires_at: %w", err)
		}
		ts := pgtype.Timestamptz{Time: t, Valid: true}
		storeInput.ExpiresAt = &ts
	}

	if err := s.store.UpdateGroup(ctx, id, tenantID, storeInput); err != nil {
		if err == pgx.ErrNoRows {
			return PlanningGroupDTO{}, pgx.ErrNoRows
		}
		return PlanningGroupDTO{}, fmt.Errorf("planninggroups: update: %w", err)
	}

	return s.Get(ctx, id, tenantID)
}

// DeleteOwn soft-deletes a planning group owned by the requesting user.
// The caller must verify ownership (planning_group.manage_own permission +
// owner_id match).
func (s *Service) DeleteOwn(ctx context.Context, id, tenantID, userID pgtype.UUID) error {
	existing, err := s.store.GetGroup(ctx, id, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return pgx.ErrNoRows
		}
		return fmt.Errorf("planninggroups: delete: get: %w", err)
	}
	if existing.OwnerID != userID {
		return ErrNotOwner
	}

	if err := s.store.DeleteGroup(ctx, id, tenantID); err != nil {
		if err == pgx.ErrNoRows {
			return pgx.ErrNoRows
		}
		return fmt.Errorf("planninggroups: delete: %w", err)
	}
	return nil
}

// Join adds the requesting user as a member of a planning group. The
// caller must have the planning_group.create permission (checked at route
// registration). departureDate is optional (RFC 3339 or YYYY-MM-DD).
func (s *Service) Join(ctx context.Context, groupID, userID, tenantID pgtype.UUID, departureDate string) error {
	var dep pgtype.Date
	if departureDate != "" {
		t, err := time.Parse("2006-01-02", departureDate)
		if err != nil {
			return fmt.Errorf("invalid departure_date (use YYYY-MM-DD): %w", err)
		}
		dep = pgtype.Date{Time: t, Valid: true}
	}

	if err := s.store.JoinGroup(ctx, groupID, userID, tenantID, dep); err != nil {
		return fmt.Errorf("planninggroups: join: %w", err)
	}
	return nil
}

// Leave removes the requesting user from a planning group.
func (s *Service) Leave(ctx context.Context, groupID, userID, tenantID pgtype.UUID) error {
	if err := s.store.LeaveGroup(ctx, groupID, userID, tenantID); err != nil {
		if err == pgx.ErrNoRows {
			return pgx.ErrNoRows
		}
		return fmt.Errorf("planninggroups: leave: %w", err)
	}
	return nil
}

// GetMembers returns all members of a planning group.
func (s *Service) GetMembers(ctx context.Context, groupID, tenantID pgtype.UUID) ([]MemberDTO, error) {
	members, err := s.store.GetMembers(ctx, groupID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("planninggroups: get members: %w", err)
	}
	dtos := make([]MemberDTO, 0, len(members))
	for _, m := range members {
		dtos = append(dtos, memberToDTO(m))
	}
	return dtos, nil
}

// CreateMilestone adds a milestone for the requesting user in a group.
func (s *Service) CreateMilestone(ctx context.Context, groupID, userID, tenantID pgtype.UUID, title string) (MilestoneDTO, error) {
	if title == "" {
		return MilestoneDTO{}, fmt.Errorf("title is required")
	}
	m, err := s.store.CreateMilestone(ctx, groupID, userID, tenantID, title)
	if err != nil {
		return MilestoneDTO{}, fmt.Errorf("planninggroups: create milestone: %w", err)
	}
	return milestoneToDTO(m), nil
}

// ListMilestones returns all milestones for the requesting user in a group.
func (s *Service) ListMilestones(ctx context.Context, groupID, userID, tenantID pgtype.UUID) ([]MilestoneDTO, error) {
	milestones, err := s.store.ListMilestones(ctx, groupID, userID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("planninggroups: list milestones: %w", err)
	}
	dtos := make([]MilestoneDTO, 0, len(milestones))
	for _, m := range milestones {
		dtos = append(dtos, milestoneToDTO(m))
	}
	return dtos, nil
}

// UpdateMilestone updates the completed state of a milestone.
func (s *Service) UpdateMilestone(ctx context.Context, milestoneID, tenantID pgtype.UUID, completed bool) error {
	if err := s.store.UpdateMilestone(ctx, milestoneID, tenantID, completed); err != nil {
		if err == pgx.ErrNoRows {
			return pgx.ErrNoRows
		}
		return fmt.Errorf("planninggroups: update milestone: %w", err)
	}
	return nil
}

// CreateCheckin adds a weekly check-in for the requesting user in a group.
func (s *Service) CreateCheckin(ctx context.Context, groupID, userID, tenantID pgtype.UUID, note string) (CheckinDTO, error) {
	if note == "" {
		return CheckinDTO{}, fmt.Errorf("note is required")
	}
	c, err := s.store.CreateCheckin(ctx, groupID, userID, tenantID, note)
	if err != nil {
		return CheckinDTO{}, fmt.Errorf("planninggroups: create checkin: %w", err)
	}
	return checkinToDTO(c), nil
}

// ListCheckins returns this week's check-ins for a group.
func (s *Service) ListCheckins(ctx context.Context, groupID, tenantID pgtype.UUID) ([]CheckinDTO, error) {
	checkins, err := s.store.ListCheckins(ctx, groupID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("planninggroups: list checkins: %w", err)
	}
	dtos := make([]CheckinDTO, 0, len(checkins))
	for _, c := range checkins {
		dtos = append(dtos, checkinToDTO(c))
	}
	return dtos, nil
}

// AdminExtendExpiry extends the expires_at timestamp on a planning group.
// Admin-only (planning_group.manage_any permission checked at route
// registration). newExpiry is RFC 3339.
func (s *Service) AdminExtendExpiry(ctx context.Context, id, tenantID pgtype.UUID, newExpiry string) error {
	if newExpiry == "" {
		return fmt.Errorf("expires_at is required")
	}
	t, err := time.Parse(time.RFC3339, newExpiry)
	if err != nil {
		return fmt.Errorf("invalid expires_at: %w", err)
	}
	exp := pgtype.Timestamptz{Time: t, Valid: true}

	if err := s.store.ExtendExpiry(ctx, id, tenantID, exp); err != nil {
		if err == pgx.ErrNoRows {
			return pgx.ErrNoRows
		}
		return fmt.Errorf("planninggroups: admin extend expiry: %w", err)
	}
	return nil
}

// groupToDTO converts a PlanningGroup to a PlanningGroupDTO.
func groupToDTO(g PlanningGroup) PlanningGroupDTO {
	dto := PlanningGroupDTO{
		ID:                 g.ID.String(),
		Name:               g.Name,
		DestinationSpaceID: g.DestinationSpaceID.String(),
		IsOpenJoin:         g.IsOpenJoin,
		OwnerID:            g.OwnerID.String(),
		CreatedAt:          formatTs(g.CreatedAt),
		UpdatedAt:          formatTs(g.UpdatedAt),
		ExpiresAt:          formatTs(g.ExpiresAt),
		TargetDepartureAt:  formatTs(g.TargetDepartureAt),
	}
	if g.Description != nil {
		dto.Description = *g.Description
	}
	if g.VisaCategory != nil {
		dto.VisaCategory = *g.VisaCategory
	}
	if g.TargetDepartureWindow != nil {
		dto.TargetDepartureWindow = *g.TargetDepartureWindow
	}
	if g.MaxMembers.Valid {
		dto.MaxMembers = g.MaxMembers.Int32
	}
	return dto
}

// memberToDTO converts a PlanningGroupMember to a MemberDTO.
func memberToDTO(m PlanningGroupMember) MemberDTO {
	dto := MemberDTO{
		UserID:   m.UserID.String(),
		JoinedAt: formatTs(m.JoinedAt),
	}
	if m.DepartureDate.Valid {
		dto.DepartureDate = m.DepartureDate.Time.Format("2006-01-02")
	}
	if m.Username != nil {
		dto.Username = *m.Username
	}
	if m.AvatarURL != nil {
		dto.AvatarURL = *m.AvatarURL
	}
	return dto
}

// milestoneToDTO converts a PlanningGroupMilestone to a MilestoneDTO.
func milestoneToDTO(m PlanningGroupMilestone) MilestoneDTO {
	return MilestoneDTO{
		ID:          m.ID.String(),
		GroupID:     m.GroupID.String(),
		UserID:      m.UserID.String(),
		Title:       m.Title,
		Completed:   m.Completed,
		CompletedAt: formatTs(m.CompletedAt),
		CreatedAt:   formatTs(m.CreatedAt),
	}
}

// checkinToDTO converts a PlanningGroupCheckin to a CheckinDTO.
func checkinToDTO(c PlanningGroupCheckin) CheckinDTO {
	dto := CheckinDTO{
		ID:        c.ID.String(),
		GroupID:   c.GroupID.String(),
		UserID:    c.UserID.String(),
		Note:      c.Note,
		CreatedAt: formatTs(c.CreatedAt),
	}
	if c.Username != nil {
		dto.Username = *c.Username
	}
	return dto
}

// formatTs renders a pgtype.Timestamptz as RFC 3339 UTC, or "" if NULL.
func formatTs(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339)
}
