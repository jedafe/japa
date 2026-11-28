// Package mentorship — applications.go implements the §102 2026-11-28
// amendment: member mentor applications. Members without
// mentorship.become_mentor submit a mentor_applications row; admin
// approval grants the capability by adding the applicant to the
// `mentors` system group (backing role mentor-role carrying exactly
// become_mentor + respond). Revocation removes the membership so the
// existing RetireMentorWithoutBecomeMentorPerm sweep retires the mentor
// profile + shadow listing automatically.
//
// Ref: japaconnect-engineering/sections/102-mentorship-matching.md
//      (Amendment — Member Mentor Applications + Discovery CTA)
package mentorship

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// mentorsGroupSlug is the seeded system group approved applicants join.
// Resolved by slug (001049 convention) — the row id differs per deploy.
const mentorsGroupSlug = "mentors"

// Application input caps. Motivation is free text an admin reads — a hard
// ceiling keeps the review queue usable; areas are matching keywords so
// they share the mentor_areas shape (short labels).
const (
	applicationMotivationMax = 2000
	applicationAreasMax      = 10
	applicationAreaMaxLen    = 60
)

// Application lifecycle errors — handlers map them onto the standard
// error envelope (Rule 5).
var (
	// ErrAlreadyEligible — caller already holds mentorship.become_mentor;
	// they opt in via PATCH /me/mentor-profile, not the application.
	ErrAlreadyEligible = errors.New("mentorship: already mentor-eligible")
	// ErrApplicationPending — a live application is already awaiting review.
	ErrApplicationPending = errors.New("mentorship: application already pending")
	// ErrApplicationNotFound — no row for the id/user.
	ErrApplicationNotFound = errors.New("mentorship: application not found")
	// ErrApplicationNotActionable — approve on a non-pending row, or
	// decline on a row that is neither pending nor approved.
	ErrApplicationNotActionable = errors.New("mentorship: application not actionable")
	// ErrInvalidMotivation — empty or over-long motivation text.
	ErrInvalidMotivation = errors.New("mentorship: invalid motivation")
	// ErrInvalidAreas — too many areas or an over-long entry.
	ErrInvalidAreas = errors.New("mentorship: invalid mentor areas")
)

// PermChecker is the narrow RBAC seam the application flow needs —
// satisfied by *rbac.RBAC. Only become_mentor is probed; apply gating is
// middleware-level like every other node.
type PermChecker interface {
	HasPermission(ctx context.Context, userID pgtype.UUID, permKey string) bool
}

// GroupMembership is the narrow rbac.Store seam for the mentors group
// grant/revoke. The store methods already invalidate the permission
// cache (Rule 6) and removal fires the mentor-retire sweep.
type GroupMembership interface {
	AddUserToGroup(ctx context.Context, userID, groupID pgtype.UUID) error
	RemoveUserFromGroup(ctx context.Context, userID, groupID pgtype.UUID) error
}

// ApplicationNotifier is the notifications seam for the decision dispatch —
// satisfied by *notifications.Service.
type ApplicationNotifier interface {
	CreateNotification(ctx context.Context, recipientID pgtype.UUID, nType, actorType string, actorID pgtype.UUID, subjectType string, subjectID pgtype.UUID, title, body, actionURL string) error
}

// SetPermChecker wires the RBAC resolver used for eligibility probes.
func (s *Service) SetPermChecker(p PermChecker) { s.permChecker = p }

// SetGroupMembership wires the rbac.Store used for mentors grants.
func (s *Service) SetGroupMembership(g GroupMembership) { s.groups = g }

// SetApplicationNotifier wires the notification dispatcher used by the
// mentorship:notify_application_decision task handler.
func (s *Service) SetApplicationNotifier(n ApplicationNotifier) { s.appNotifier = n }

// mentorsGroupID resolves (and memoizes) the mentors system group id.
func (s *Service) mentorsGroupID(ctx context.Context) (pgtype.UUID, error) {
	s.groupMu.Lock()
	defer s.groupMu.Unlock()
	if s.mentorsGroup.Valid {
		return s.mentorsGroup, nil
	}
	row, err := s.read.GetUserGroupBySlug(ctx, mentorsGroupSlug)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("mentorship: resolve mentors group: %w", err)
	}
	s.mentorsGroup = row.ID
	return row.ID, nil
}

// MentorApplication is the API DTO for one application row.
type MentorApplication struct {
	ID             string     `json:"id"`
	UserID         string     `json:"user_id"`
	Status         string     `json:"status"`
	Motivation     string     `json:"motivation"`
	MentorAreas    []string   `json:"mentor_areas"`
	ReviewerUserID string     `json:"reviewer_user_id,omitempty"`
	ReviewedAt     *time.Time `json:"reviewed_at,omitempty"`
	DeclineReason  string     `json:"decline_reason,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`

	// Admin list enrichment (join in ListMentorApplications).
	Username    string `json:"username,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
}

// MyApplicationStatus is the GET /me/mentor-application payload — one
// call feeds every become-mentor surface: `eligible` (holds
// become_mentor now), `is_mentor` (already opted in), and the latest
// application row when one exists.
type MyApplicationStatus struct {
	Eligible    bool               `json:"eligible"`
	IsMentor    bool               `json:"is_mentor"`
	Application *MentorApplication `json:"application"`
}

func applicationDTO(row gen.MentorApplication) *MentorApplication {
	dto := &MentorApplication{
		ID:          row.ID.String(),
		UserID:      row.UserID.String(),
		Status:      row.Status,
		Motivation:  row.Motivation,
		MentorAreas: row.MentorAreas,
	}
	if row.MentorAreas == nil {
		dto.MentorAreas = []string{}
	}
	if row.ReviewerUserID.Valid {
		dto.ReviewerUserID = row.ReviewerUserID.String()
	}
	if row.ReviewedAt.Valid {
		t := row.ReviewedAt.Time
		dto.ReviewedAt = &t
	}
	if row.DeclineReason != nil {
		dto.DeclineReason = *row.DeclineReason
	}
	if row.CreatedAt.Valid {
		dto.CreatedAt = row.CreatedAt.Time
	}
	if row.UpdatedAt.Valid {
		dto.UpdatedAt = row.UpdatedAt.Time
	}
	return dto
}

// eligible reports whether the caller can opt in directly — no
// application needed. Unwired checker (tests, misconfig) fails closed.
func (s *Service) eligible(ctx context.Context, userID pgtype.UUID) bool {
	return s.permChecker != nil && s.permChecker.HasPermission(ctx, userID, "mentorship.become_mentor")
}

// GetMyApplication assembles the eligibility probe + application row for
// GET /me/mentor-application.
func (s *Service) GetMyApplication(ctx context.Context, userID pgtype.UUID) (MyApplicationStatus, error) {
	out := MyApplicationStatus{Eligible: s.eligible(ctx, userID)}

	// is_mentor — read the profile row directly (GetMentorProfile resolves
	// the users-fallback for profile-less accounts). Best-effort: a read
	// failure must not blank the eligibility probe.
	if profile, err := s.GetMentorProfile(ctx, userID); err == nil {
		out.IsMentor = profile.IsMentor != nil && *profile.IsMentor
	}

	row, err := s.read.GetMentorApplicationByUser(ctx, userID)
	switch {
	case err == nil:
		out.Application = applicationDTO(row)
	case errors.Is(err, pgx.ErrNoRows):
		// no application yet — null field.
	default:
		return out, fmt.Errorf("mentorship: load application: %w", err)
	}
	return out, nil
}

// validateApplicationInput enforces the motivation/areas shape — the
// handler trims first; the service re-checks so the contract holds for
// every caller.
func validateApplicationInput(motivation string, areas []string) (string, []string, error) {
	motivation = strings.TrimSpace(motivation)
	// RuneCount — the client's maxlength counts characters; counting
	// bytes here would reject multibyte text well under the limit.
	if motivation == "" || utf8.RuneCountInString(motivation) > applicationMotivationMax {
		return "", nil, ErrInvalidMotivation
	}
	clean := make([]string, 0, len(areas))
	seen := map[string]bool{}
	for _, a := range areas {
		a = strings.TrimSpace(a)
		if a == "" || seen[strings.ToLower(a)] {
			continue
		}
		if utf8.RuneCountInString(a) > applicationAreaMaxLen {
			return "", nil, ErrInvalidAreas
		}
		seen[strings.ToLower(a)] = true
		clean = append(clean, a)
	}
	if len(clean) > applicationAreasMax {
		return "", nil, ErrInvalidAreas
	}
	return motivation, clean, nil
}

// ApplyForMentor submits (or resubmits after a decline) a mentor
// application. POST /me/mentor-application.
func (s *Service) ApplyForMentor(ctx context.Context, userID pgtype.UUID, motivation string, areas []string) (*MentorApplication, error) {
	if s.eligible(ctx, userID) {
		return nil, ErrAlreadyEligible
	}

	motivation, areas, err := validateApplicationInput(motivation, areas)
	if err != nil {
		return nil, err
	}

	var row gen.MentorApplication
	existing, err := s.read.GetMentorApplicationByUser(ctx, userID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		row, err = s.write.CreateMentorApplication(ctx, gen.CreateMentorApplicationParams{
			UserID:      userID,
			Motivation:  motivation,
			MentorAreas: areas,
		})
		// Two tabs racing a first submit: the UNIQUE(user_id) loser maps
		// to the pending conflict instead of a 500.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrApplicationPending
		}
	case err != nil:
		return nil, fmt.Errorf("mentorship: load application: %w", err)
	case existing.Status == "pending":
		return nil, ErrApplicationPending
	default:
		// declined → resubmit; approved-but-no-permission (grant removed
		// out-of-band) → re-review rather than silently trusting a stale
		// decision.
		row, err = s.write.ResubmitMentorApplication(ctx, gen.ResubmitMentorApplicationParams{
			UserID:      userID,
			Motivation:  motivation,
			MentorAreas: areas,
		})
		// A racing submit flipped the row to pending first — same
		// outcome the member asked for; report the pending conflict.
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrApplicationPending
		}
	}
	if err != nil {
		return nil, fmt.Errorf("mentorship: submit application: %w", err)
	}

	s.trackEvent(ctx, userID, "mentorship_application_submitted", "mentor_application", row.ID, map[string]any{})
	return applicationDTO(row), nil
}

// ListApplications is the admin review queue — pending-first ordering
// comes from the query; q filters username/display name.
func (s *Service) ListApplications(ctx context.Context, status, q string, limit, offset int32) ([]MentorApplication, int64, error) {
	switch status {
	case "", "pending", "approved", "declined":
	default:
		return nil, 0, ErrApplicationNotActionable
	}

	rows, err := s.read.ListMentorApplications(ctx, gen.ListMentorApplicationsParams{
		Status:     status,
		Q:          strings.TrimSpace(q),
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("mentorship: list applications: %w", err)
	}
	total, err := s.read.CountMentorApplications(ctx, gen.CountMentorApplicationsParams{
		Status: status,
		Q:      strings.TrimSpace(q),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("mentorship: count applications: %w", err)
	}

	out := make([]MentorApplication, 0, len(rows))
	for _, r := range rows {
		dto := applicationDTO(gen.MentorApplication{
			ID:             r.ID,
			UserID:         r.UserID,
			Status:         r.Status,
			Motivation:     r.Motivation,
			MentorAreas:    r.MentorAreas,
			ReviewerUserID: r.ReviewerUserID,
			ReviewedAt:     r.ReviewedAt,
			DeclineReason:  r.DeclineReason,
			CreatedAt:      r.CreatedAt,
			UpdatedAt:      r.UpdatedAt,
		})
		dto.Username = r.Username
		dto.DisplayName = r.DisplayName
		out = append(out, *dto)
	}
	return out, total, nil
}

// loadActionableApplication fetches the row and enforces the caller's
// allowed source statuses — gives the admin a precise 404 vs 409 split.
func (s *Service) loadActionableApplication(ctx context.Context, appID pgtype.UUID, allowed ...string) (gen.MentorApplication, error) {
	row, err := s.read.GetMentorApplicationByID(ctx, appID)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrApplicationNotFound
	}
	if err != nil {
		return row, fmt.Errorf("mentorship: load application: %w", err)
	}
	for _, st := range allowed {
		if row.Status == st {
			return row, nil
		}
	}
	return row, ErrApplicationNotActionable
}

// ApproveApplication transitions pending → approved and grants the
// mentors group membership (the capability). The grant runs BEFORE the
// status write and the call is idempotent on approved rows: a retry
// after a partial failure (approved row whose grant never landed, or a
// membership removed out-of-band) re-heals the grant instead of
// dead-ending on a 409. POST
// /admin/mentorship/applications/{id}/approve.
func (s *Service) ApproveApplication(ctx context.Context, appID, reviewerID pgtype.UUID) (*MentorApplication, error) {
	prior, err := s.loadActionableApplication(ctx, appID, "pending", "approved")
	if err != nil {
		return nil, err
	}

	// The grant: mentors membership carries become_mentor + respond.
	// Running it before the transition keeps failures retryable — a
	// failed grant leaves the row pending; AddUserToGroup is idempotent
	// (ON CONFLICT DO NOTHING) so re-approval re-heals cleanly.
	// rbac.Store invalidates the permission cache itself (Rule 6).
	if s.groups != nil {
		groupID, gerr := s.mentorsGroupID(ctx)
		if gerr != nil {
			return nil, gerr
		}
		if err := s.groups.AddUserToGroup(ctx, prior.UserID, groupID); err != nil {
			return nil, fmt.Errorf("mentorship: grant mentors group: %w", err)
		}
	} else {
		slog.Warn("mentorship: approve without group seam — grant skipped", "application_id", appID)
	}

	if prior.Status == "approved" {
		// Ensure-grant retry — nothing to transition.
		return applicationDTO(prior), nil
	}

	row, err := s.write.ApproveMentorApplication(ctx, gen.ApproveMentorApplicationParams{
		ID:             appID,
		ReviewerUserID: reviewerID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrApplicationNotActionable
		}
		return nil, fmt.Errorf("mentorship: approve application: %w", err)
	}

	s.enqueueApplicationDecision(ctx, row.ID)
	s.trackEvent(ctx, reviewerID, "mentorship_application_approved", "mentor_application", row.ID,
		map[string]any{"applicant_id": row.UserID.String()})
	return applicationDTO(row), nil
}

// DeclineApplication transitions pending → declined (reject) or
// approved → declined (revoke). On revoke the mentors membership is
// removed — RemoveUserFromGroup fires RetireMentorWithoutBecomeMentorPerm,
// retiring is_mentor + the shadow listing in the same call. POST
// /admin/mentorship/applications/{id}/decline.
func (s *Service) DeclineApplication(ctx context.Context, appID, reviewerID pgtype.UUID, reason string) (*MentorApplication, error) {
	prior, err := s.loadActionableApplication(ctx, appID, "pending", "approved")
	if err != nil {
		return nil, err
	}

	// Revoke arm: an approved application carried a mentors membership.
	// Removal runs BEFORE the status write — a failed decline then leaves
	// an approved row whose grant is already gone (safely retryable,
	// the DELETE is a no-op on rerun) rather than a declined row whose
	// grant lingers. RemoveUserFromGroup fires the mentor-retire sweep
	// itself, retiring is_mentor + the shadow listing.
	if prior.Status == "approved" && s.groups != nil {
		groupID, gerr := s.mentorsGroupID(ctx)
		if gerr != nil {
			return nil, gerr
		}
		if err := s.groups.RemoveUserFromGroup(ctx, prior.UserID, groupID); err != nil {
			return nil, fmt.Errorf("mentorship: revoke mentors group: %w", err)
		}
	}

	reason = strings.TrimSpace(reason)
	row, err := s.write.DeclineMentorApplication(ctx, gen.DeclineMentorApplicationParams{
		ID:             appID,
		ReviewerUserID: reviewerID,
		DeclineReason:  strNilIfEmpty(reason),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrApplicationNotActionable
		}
		return nil, fmt.Errorf("mentorship: decline application: %w", err)
	}

	s.enqueueApplicationDecision(ctx, row.ID)
	s.trackEvent(ctx, reviewerID, "mentorship_application_declined", "mentor_application", row.ID,
		map[string]any{"applicant_id": row.UserID.String(), "revoked": prior.Status == "approved"})
	return applicationDTO(row), nil
}

func strNilIfEmpty(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

// enqueueApplicationDecision fires the notify task — fire-and-forget
// (Rule 8), nil enqueuer degrades to skip.
func (s *Service) enqueueApplicationDecision(ctx context.Context, appID pgtype.UUID) {
	if s.enqueuer == nil {
		return
	}
	payload, err := json.Marshal(map[string]string{"application_id": appID.String()})
	if err != nil {
		slog.Warn("mentorship: marshal application decision payload", "application_id", appID, "error", err)
		return
	}
	if err := s.enqueuer.Enqueue("mentorship:notify_application_decision", payload); err != nil {
		slog.Warn("mentorship: enqueue application decision", "application_id", appID, "error", err)
	}
}

// NotifyApplicationDecisionDispatch is the mentorship:notify_application_decision
// task handler — in-app notification on approve/decline. Called by the
// worker MentorProcessor.
func (s *Service) NotifyApplicationDecisionDispatch(ctx context.Context, appID pgtype.UUID) error {
	row, err := s.read.GetMentorApplicationByID(ctx, appID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // row deleted between enqueue and dispatch — nothing to do
	}
	if err != nil {
		return fmt.Errorf("mentorship: load application for decision notify: %w", err)
	}
	if s.appNotifier == nil {
		slog.Info("mentorship:notify_application_decision: no notifier wired, logging only", "application_id", appID, "status", row.Status)
		return nil
	}

	var nType, title, body, actionURL string
	switch row.Status {
	case "approved":
		nType = "mentorship_application_approved"
		title = "Mentor Application Approved"
		body = "Your mentor application has been approved. Set up your mentor profile to start receiving requests."
		actionURL = "/settings/mentorship"
	case "declined":
		nType = "mentorship_application_declined"
		title = "Mentor Application Declined"
		body = "Your mentor application was declined."
		if row.DeclineReason != nil && *row.DeclineReason != "" {
			body = "Your mentor application was declined: " + *row.DeclineReason
		}
		actionURL = "/become-mentor"
	default:
		return nil // pending rows have no decision yet
	}

	return s.appNotifier.CreateNotification(
		ctx,
		row.UserID,
		nType,
		"system",
		pgtype.UUID{},
		"mentor_application",
		row.ID,
		title,
		body,
		actionURL,
	)
}
