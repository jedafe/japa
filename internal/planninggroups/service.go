// Package planninggroups — service.go orchestrates planning group creation,
// membership, milestones, weekly check-ins, and admin expiry extension.
// Feature-flagged via planning_group.enabled system_setting (§82).
//
// Ref: japaconnect-engineering/sections/82-planning-groups.md
//
//	japaconnect-engineering/adr/ADR-090-planning-groups.md
package planninggroups

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	MemberCount           int64  `json:"member_count"`
	Status                string `json:"status"`
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
	DestinationSpaceID    *string `json:"destination_space_id"`
	VisaCategory          *string `json:"visa_category"`
	TargetDepartureWindow *string `json:"target_departure_window"`
	TargetDepartureAt     *string `json:"target_departure_at"`
	MaxMembers            *int32  `json:"max_members"`
	IsOpenJoin            *bool   `json:"is_open_join"`
	ExpiresAt             *string `json:"expires_at"`
}

// ErrNotOwner is returned when a user tries to update or delete a planning
// group they did not create.
var ErrNotOwner = errors.New("not the planning group owner")

// ErrNotMember is returned when a user tries to access group content without
// being a member.
var ErrNotMember = errors.New("not a member of this planning group")

// ErrGroupNotOpen is returned when a user tries to join an invite-only group.
var ErrGroupNotOpen = errors.New("planning group is not open for joining")

// ErrGroupFull is returned when a group is at max_members capacity.
var ErrGroupFull = errors.New("planning group is full")

// ErrAlreadyMember is returned when a user tries to join a group they're
// already in.
var ErrAlreadyMember = errors.New("already a member of this planning group")

// Notifier is the interface used to dispatch planning-group notifications.
// Satisfied by *notifications.Service; declared here to avoid an import
// cycle. Ref: §82 Notifications & Gamification, §35 Notification System.
type Notifier interface {
	CreateNotification(ctx context.Context, recipientID pgtype.UUID, nType, actorType string, actorID pgtype.UUID, subjectType string, subjectID pgtype.UUID, title, body, actionURL string) error
}

// FeatureChecker resolves feature flag state from the feature_flags table —
// the single source of truth for feature reachability.
type FeatureChecker interface {
	IsEnabled(ctx context.Context, key string) bool
}

// Service holds the business logic for Japa Planning Groups.
type Service struct {
	store    *Store
	settings *config.Settings
	checker  FeatureChecker
	notifier Notifier
}

// NewService creates a planning-groups service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{store: store, settings: settings}
}

// SetFeatureChecker injects the feature flag checker so Enabled reads the
// feature_flags table (the single source of truth for feature reachability)
// instead of system_settings. Falls back to system_settings when nil.
func (s *Service) SetFeatureChecker(c FeatureChecker) { s.checker = c }

// WithNotifier attaches the notification dispatcher used by the
// mission-complete celebration (§82 Integration with §89 Achievement Feed).
func (s *Service) WithNotifier(n Notifier) *Service { s.notifier = n; return s }

// Enabled reads the planning_group.enabled feature flag (§82);
// system_settings is consulted only as a fallback when no checker is wired.
func (s *Service) Enabled(ctx context.Context) bool {
	if s.checker != nil {
		return s.checker.IsEnabled(ctx, "planning_group.enabled")
	}

	if s.settings == nil {
		return false
	}

	return s.settings.GetBoolOr(ctx, "planning_group.enabled", false)
}

// achievementFeedEnabled resolves the achievement.enabled feature flag —
// the §82 milestone-complete celebration suggests posting to the §89
// Achievement Feed only when that feature is reachable.
func (s *Service) achievementFeedEnabled(ctx context.Context) bool {
	if s.checker != nil {
		return s.checker.IsEnabled(ctx, "achievement.enabled")
	}

	if s.settings == nil {
		return false
	}

	return s.settings.GetBoolOr(ctx, "achievement.enabled", false)
}

// Create creates a new planning group owned by the requesting user. The
// caller must have the planning_group.create permission (checked at route
// registration).
func (s *Service) Create(ctx context.Context, ownerID pgtype.UUID, input CreateGroupInput) (PlanningGroupDTO, error) {
	if input.Name == "" || input.DestinationSpaceID == "" {
		return PlanningGroupDTO{}, errors.New("name and destination_space_id are required")
	}
	// §82/ADR-090: groups are 5–20 members (max_members DEFAULT 15). Enforce
	// a 2–50 range server-side (frontend validates the same).
	if input.MaxMembers > 0 && (input.MaxMembers < 2 || input.MaxMembers > 50) {
		return PlanningGroupDTO{}, errors.New("max_members must be between 2 and 50")
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
		t, err := parseFlexibleDate(input.TargetDepartureAt)
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
		t, err := parseFlexibleDate(input.ExpiresAt)
		if err != nil {
			return PlanningGroupDTO{}, fmt.Errorf("invalid expires_at: %w", err)
		}

		expiresAt = pgtype.Timestamptz{Time: t, Valid: true}
	}

	group, err := s.store.CreateGroup(ctx, PlanningGroup{

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
	if err := s.store.JoinGroup(ctx, group.ID, ownerID, pgtype.Date{}); err != nil {
		return PlanningGroupDTO{}, fmt.Errorf("planninggroups: create: auto-join owner: %w", err)
	}

	return groupToDTO(group), nil
}

// Get returns a single planning group by id.
func (s *Service) Get(ctx context.Context, id pgtype.UUID) (PlanningGroupDTO, error) {
	group, err := s.store.GetGroup(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PlanningGroupDTO{}, pgx.ErrNoRows
		}

		return PlanningGroupDTO{}, fmt.Errorf("planninggroups: get: %w", err)
	}

	return groupToDTO(group), nil
}

// List returns a paginated list of planning groups, optionally filtered by
// destination space and visa category.
func (s *Service) List(ctx context.Context, destSpaceID pgtype.UUID, visaCategory string, page, limit int32) ([]PlanningGroupDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	groups, total, err := s.store.ListGroups(ctx, ListGroupsFilter{

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
func (s *Service) UpdateOwn(ctx context.Context, id, userID pgtype.UUID, input UpdateGroupInput) (PlanningGroupDTO, error) {
	existing, err := s.store.GetGroup(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
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

	if input.DestinationSpaceID != nil && *input.DestinationSpaceID != "" {
		var destID pgtype.UUID
		if err := destID.Scan(*input.DestinationSpaceID); err != nil {
			return PlanningGroupDTO{}, fmt.Errorf("invalid destination_space_id: %w", err)
		}

		storeInput.DestinationSpaceID = &destID
	}

	if input.TargetDepartureAt != nil && *input.TargetDepartureAt != "" {
		t, err := parseFlexibleDate(*input.TargetDepartureAt)
		if err != nil {
			return PlanningGroupDTO{}, fmt.Errorf("invalid target_departure_at: %w", err)
		}

		ts := pgtype.Timestamptz{Time: t, Valid: true}
		storeInput.TargetDepartureAt = &ts
	}

	if input.ExpiresAt != nil && *input.ExpiresAt != "" {
		t, err := parseFlexibleDate(*input.ExpiresAt)
		if err != nil {
			return PlanningGroupDTO{}, fmt.Errorf("invalid expires_at: %w", err)
		}

		ts := pgtype.Timestamptz{Time: t, Valid: true}
		storeInput.ExpiresAt = &ts
	}

	if err := s.store.UpdateGroup(ctx, id, storeInput); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PlanningGroupDTO{}, pgx.ErrNoRows
		}

		return PlanningGroupDTO{}, fmt.Errorf("planninggroups: update: %w", err)
	}

	return s.Get(ctx, id)
}

// DeleteOwn soft-deletes a planning group owned by the requesting user.
// The caller must verify ownership (planning_group.manage_own permission +
// owner_id match).
func (s *Service) DeleteOwn(ctx context.Context, id, userID pgtype.UUID) error {
	existing, err := s.store.GetGroup(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("planninggroups: delete: get: %w", err)
	}

	if existing.OwnerID != userID {
		return ErrNotOwner
	}

	if err := s.store.DeleteGroup(ctx, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("planninggroups: delete: %w", err)
	}

	return nil
}

// Join adds the requesting user as a member of a planning group. The
// caller must have the planning_group.create permission (checked at route
// registration). departureDate is optional (RFC 3339 or YYYY-MM-DD).
// Checks is_open_join and max_members capacity per §82.
func (s *Service) Join(ctx context.Context, groupID, userID pgtype.UUID, departureDate string) error {
	group, err := s.store.GetGroup(ctx, groupID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("planninggroups: join: get: %w", err)
	}

	// Check if already a member.
	isMember, err := s.store.IsMember(ctx, groupID, userID)
	if err != nil {
		return fmt.Errorf("planninggroups: join: is_member: %w", err)
	}

	if isMember {
		return ErrAlreadyMember
	}

	// §82: invite-only groups cannot be joined directly.
	if !group.IsOpenJoin {
		return ErrGroupNotOpen
	}

	// §82: enforce max_members capacity.
	if group.MaxMembers.Valid && group.MemberCount >= int64(group.MaxMembers.Int32) {
		return ErrGroupFull
	}

	var dep pgtype.Date

	if departureDate != "" {
		t, err := time.Parse("2006-01-02", departureDate)
		if err != nil {
			return fmt.Errorf("invalid departure_date (use YYYY-MM-DD): %w", err)
		}

		dep = pgtype.Date{Time: t, Valid: true}
	}

	if err := s.store.JoinGroup(ctx, groupID, userID, dep); err != nil {
		return fmt.Errorf("planninggroups: join: %w", err)
	}

	return nil
}

// Leave removes the requesting user from a planning group.
func (s *Service) Leave(ctx context.Context, groupID, userID pgtype.UUID) error {
	if err := s.store.LeaveGroup(ctx, groupID, userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("planninggroups: leave: %w", err)
	}

	return nil
}

// GetMembers returns all members of a planning group.
func (s *Service) GetMembers(ctx context.Context, groupID pgtype.UUID) ([]MemberDTO, error) {
	members, err := s.store.GetMembers(ctx, groupID)
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
func (s *Service) CreateMilestone(ctx context.Context, groupID, userID pgtype.UUID, title string) (MilestoneDTO, error) {
	if title == "" {
		return MilestoneDTO{}, errors.New("title is required")
	}

	m, err := s.store.CreateMilestone(ctx, groupID, userID, title)
	if err != nil {
		return MilestoneDTO{}, fmt.Errorf("planninggroups: create milestone: %w", err)
	}

	return milestoneToDTO(m), nil
}

// ListMilestones returns all milestones for the requesting user in a group.
func (s *Service) ListMilestones(ctx context.Context, groupID, userID pgtype.UUID) ([]MilestoneDTO, error) {
	milestones, err := s.store.ListMilestones(ctx, groupID, userID)
	if err != nil {
		return nil, fmt.Errorf("planninggroups: list milestones: %w", err)
	}

	dtos := make([]MilestoneDTO, 0, len(milestones))
	for _, m := range milestones {
		dtos = append(dtos, milestoneToDTO(m))
	}

	return dtos, nil
}

// UpdateMilestone updates the completed state of a milestone. The milestone
// must belong to the given group and the requesting user (ownership check).
// When marking a milestone as completed, the service checks whether all of
// the user's milestones in the group are now complete; if so, it dispatches
// a "Mission Complete" celebration notification with a prompt to share in
// the Achievement Feed (§82 Integration with §89).
func (s *Service) UpdateMilestone(ctx context.Context, milestoneID, groupID, userID pgtype.UUID, completed bool) error {
	if err := s.store.UpdateMilestone(ctx, milestoneID, groupID, userID, completed); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("planninggroups: update milestone: %w", err)
	}

	// §82: on all milestones complete → celebration notification + prompt
	// to share in Achievement Feed (§89). Only fires when the milestone was
	// just marked complete and the achievement feature is enabled.
	if completed && s.notifier != nil &&
		s.achievementFeedEnabled(ctx) {
		remaining, err := s.store.CountIncompleteMilestones(ctx, groupID, userID)
		if err != nil {
			slog.Warn("planninggroups: count incomplete milestones failed", "error", err)
		} else if remaining == 0 {
			zero := pgtype.UUID{}
			title := "Mission Complete! Share your win in the Achievement Feed"
			body := "You've completed all your milestones in this planning group. Celebrate by sharing your achievement with the community."

			if err := s.notifier.CreateNotification(ctx, userID, "planning_group.mission_complete", "system", zero, "planning_group", groupID, title, body, "/achievements/new"); err != nil {
				slog.Warn("planninggroups: mission complete notification failed", "error", err)
			}
		}
	}

	return nil
}

// IsMember returns true if the user is a member of the planning group.
func (s *Service) IsMember(ctx context.Context, groupID, userID pgtype.UUID) (bool, error) {
	return s.store.IsMember(ctx, groupID, userID)
}

// InviteMember adds a user to a planning group by invitation. Only the
// group owner may invite (checked at the handler level via
// planning_group.manage_own). The invited user is added directly as a
// member (§82: "invite user (owner only)").
func (s *Service) InviteMember(ctx context.Context, groupID, inviteeUserID pgtype.UUID) error {
	group, err := s.store.GetGroup(ctx, groupID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("planninggroups: invite: get: %w", err)
	}
	// Enforce max_members capacity even for invitations.
	if group.MaxMembers.Valid && group.MemberCount >= int64(group.MaxMembers.Int32) {
		return ErrGroupFull
	}

	if err := s.store.InviteMember(ctx, groupID, inviteeUserID); err != nil {
		return fmt.Errorf("planninggroups: invite: %w", err)
	}

	return nil
}

// AdminListGroups returns a paginated list of all planning groups for the
// admin review queue, optionally filtered by destination and search term.
func (s *Service) AdminListGroups(ctx context.Context, f AdminListGroupsFilter) ([]PlanningGroupDTO, int64, error) {
	groups, total, err := s.store.AdminListGroups(ctx, f)
	if err != nil {
		return nil, 0, fmt.Errorf("planninggroups: admin list: %w", err)
	}

	dtos := make([]PlanningGroupDTO, 0, len(groups))
	for _, g := range groups {
		dtos = append(dtos, groupToDTO(g))
	}

	return dtos, total, nil
}

// ExpireGroups soft-deletes planning groups whose expires_at has passed.
// Called nightly by the planning_group:expire Asynq task (§82, ADR-090).
// Implements worker.PlanningGroupProcessor.
func (s *Service) ExpireGroups(ctx context.Context) (int64, error) {
	n, err := s.store.ExpireGroups(ctx)
	if err != nil {
		return 0, fmt.Errorf("planninggroups: expire groups: %w", err)
	}

	return n, nil
}

// SendCheckinReminders identifies members who haven't checked in this week
// and dispatches reminder notifications. Called weekly by the
// planning_group:checkin_reminder Asynq task (§82, ADR-090).
// Implements worker.PlanningGroupProcessor.
func (s *Service) SendCheckinReminders(ctx context.Context) (int64, error) {
	members, err := s.store.MembersWithoutWeeklyCheckin(ctx)
	if err != nil {
		return 0, fmt.Errorf("planninggroups: send checkin reminders: %w", err)
	}

	if len(members) == 0 {
		return 0, nil
	}

	var sent int64

	zero := pgtype.UUID{}
	title := "Weekly check-in reminder"
	body := "You haven't checked in with your planning group this week. Share your progress and keep your accountability on track."

	for _, m := range members {
		if s.notifier == nil {
			// No notifier wired — log and stop so we don't silently
			// claim success.
			slog.WarnContext(ctx, "planninggroups: checkin reminder notifier not wired",
				"group_id", m.GroupID, "user_id", m.UserID)
			break
		}

		if err := s.notifier.CreateNotification(
			ctx, m.UserID,
			"planning_group.checkin_reminder", "system", zero,
			"planning_group", m.GroupID,
			title, body,
			"/planning-groups/"+m.GroupID.String(),
		); err != nil {
			slog.WarnContext(ctx, "planninggroups: checkin reminder notification failed",
				"error", err, "user_id", m.UserID, "group_id", m.GroupID)
			continue
		}

		sent++
	}

	slog.InfoContext(ctx, "planninggroups: checkin reminders dispatched",
		"sent", sent, "eligible", len(members))

	return sent, nil
}

// ErrCheckinTooLong is returned when a check-in note exceeds the 200-char
// limit defined in §82 (note VARCHAR(200)).
var ErrCheckinTooLong = errors.New("check-in note exceeds 200 characters")

// CreateCheckin adds a weekly check-in for the requesting user in a group.
// Returns ErrAlreadyCheckedIn if the user has already checked in this week.
func (s *Service) CreateCheckin(ctx context.Context, groupID, userID pgtype.UUID, note string) (CheckinDTO, error) {
	if note == "" {
		return CheckinDTO{}, errors.New("note is required")
	}
	// §82: note VARCHAR(200).
	if len(note) > 200 {
		return CheckinDTO{}, ErrCheckinTooLong
	}

	c, err := s.store.CreateCheckin(ctx, groupID, userID, note)
	if err != nil {
		if errors.Is(err, ErrAlreadyCheckedIn) {
			return CheckinDTO{}, ErrAlreadyCheckedIn
		}

		return CheckinDTO{}, fmt.Errorf("planninggroups: create checkin: %w", err)
	}

	return checkinToDTO(c), nil
}

// ListCheckins returns this week's check-ins for a group.
func (s *Service) ListCheckins(ctx context.Context, groupID pgtype.UUID) ([]CheckinDTO, error) {
	checkins, err := s.store.ListCheckins(ctx, groupID)
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
// registration). newExpiry is RFC 3339 or YYYY-MM-DD.
func (s *Service) AdminExtendExpiry(ctx context.Context, id pgtype.UUID, newExpiry string) error {
	if newExpiry == "" {
		return errors.New("expires_at is required")
	}

	t, err := parseFlexibleDate(newExpiry)
	if err != nil {
		return fmt.Errorf("invalid expires_at: %w", err)
	}

	exp := pgtype.Timestamptz{Time: t, Valid: true}

	if err := s.store.ExtendExpiry(ctx, id, exp); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("planninggroups: admin extend expiry: %w", err)
	}

	return nil
}

// AdminExtendExpiryByDays extends the expires_at of a planning group by
// the given number of days, computed from the later of the current
// expires_at or now(). If the group has no expires_at, the new expiry is
// now + days. Admin-only.
func (s *Service) AdminExtendExpiryByDays(ctx context.Context, id pgtype.UUID, days int) error {
	if days <= 0 {
		return errors.New("days must be a positive integer")
	}

	group, err := s.store.GetGroup(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("planninggroups: admin extend by days: get: %w", err)
	}

	base := time.Now().UTC()
	if group.ExpiresAt.Valid && group.ExpiresAt.Time.After(base) {
		base = group.ExpiresAt.Time
	}

	newExpiry := base.AddDate(0, 0, days)

	exp := pgtype.Timestamptz{Time: newExpiry, Valid: true}
	if err := s.store.ExtendExpiry(ctx, id, exp); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("planninggroups: admin extend by days: %w", err)
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
		MemberCount:        g.MemberCount,
		Status:             groupStatus(g),
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

// groupStatus derives a display status for a planning group: "deleted",
// "expired", or "active".
func groupStatus(g PlanningGroup) string {
	if g.DeletedAt.Valid {
		return "deleted"
	}

	if g.ExpiresAt.Valid && g.ExpiresAt.Time.Before(time.Now()) {
		return "expired"
	}

	return "active"
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

// parseFlexibleDate parses a date/datetime string accepting both RFC 3339
// (e.g. "2026-08-14T00:00:00Z") and YYYY-MM-DD (e.g. "2026-08-14"). The
// latter is what HTML <input type="date"> submits. A bare date is parsed
// at midnight UTC.
func parseFlexibleDate(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err == nil {
		return t, nil
	}

	t, err = time.Parse("2006-01-02", s)
	if err == nil {
		return t, nil
	}

	return time.Time{}, errors.New("unrecognized date format (use RFC 3339 or YYYY-MM-DD)")
}
