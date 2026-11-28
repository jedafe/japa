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
		// decision. The expected-status guard binds to the row we read;
		// a raced transition (decline landing mid-submit, or a second
		// submit winning first) fails the UPDATE — re-read once and
		// retry against the fresh status rather than silently
		// overwriting a new admin decision.
		row, err = s.resubmitApplication(ctx, userID, existing.Status, motivation, areas)
	}
	if err != nil {
		return nil, fmt.Errorf("mentorship: submit application: %w", err)
	}

	s.trackEvent(ctx, userID, "mentorship_application_submitted", "mentor_application", row.ID, map[string]any{})
	return applicationDTO(row), nil
}

// resubmitApplication runs the resubmit UPDATE guarded on the status
// the caller read, retrying once against the fresh status when a raced
// transition invalidates the guard. A row that lands on pending
// mid-call is reported as the pending conflict (a racing submit won —
// same outcome); a row that keeps losing the race is not actionable.
func (s *Service) resubmitApplication(ctx context.Context, userID pgtype.UUID, expected, motivation string, areas []string) (gen.MentorApplication, error) {
	var row gen.MentorApplication
	for attempt := 0; attempt < 2; attempt++ {
		var err error
		row, err = s.write.ResubmitMentorApplication(ctx, gen.ResubmitMentorApplicationParams{
			UserID:         userID,
			Motivation:     motivation,
			MentorAreas:    areas,
			ExpectedStatus: expected,
		})
		switch {
		case err == nil:
			return row, nil
		case !errors.Is(err, pgx.ErrNoRows):
			return row, fmt.Errorf("mentorship: resubmit application: %w", err)
		}
		// The guard lost the race — see where the row landed.
		cur, rerr := s.read.GetMentorApplicationByUser(ctx, userID)
		switch {
		case errors.Is(rerr, pgx.ErrNoRows):
			return row, ErrApplicationNotFound
		case rerr != nil:
			return row, fmt.Errorf("mentorship: load application: %w", rerr)
		case cur.Status == "pending":
			return row, ErrApplicationPending
		default:
			expected = cur.Status
		}
	}
	return row, ErrApplicationNotActionable
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
// dead-ending on a 409. If the status write fails after the grant, the
// membership is rolled back — but only when the pre-call probe proved
// this call created it — so the row returns to exactly pending+no-grant.
// POST /admin/mentorship/applications/{id}/approve.
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
	var groupID pgtype.UUID
	wasMember := true // probe failure fails safe: never strip a grant
	if s.groups != nil {
		var gerr error
		groupID, gerr = s.mentorsGroupID(ctx)
		if gerr != nil {
			return nil, gerr
		}
		if in, perr := s.read.IsUserInGroupID(ctx, gen.IsUserInGroupIDParams{
			UserID:  prior.UserID,
			GroupID: groupID,
		}); perr == nil {
			wasMember = in
		} else {
			slog.Warn("mentorship: membership probe failed — grant treated as pre-existing", "application_id", appID, "error", perr)
		}
		if err := s.groups.AddUserToGroup(ctx, prior.UserID, groupID); err != nil {
			return nil, fmt.Errorf("mentorship: grant mentors group: %w", err)
		}
	} else {
		slog.Warn("mentorship: approve without group seam — grant skipped", "application_id", appID)
	}

	if prior.Status == "approved" {
		// Ensure-grant retry — the add is status-neutral, so a raced
		// decline or resubmit can't be caught by an UPDATE guard. Re-read
		// the row: anything but approved must not keep a grant this call
		// created.
		cur, rerr := s.read.GetMentorApplicationByID(ctx, appID)
		switch {
		case errors.Is(rerr, pgx.ErrNoRows):
			s.compensateGrant(ctx, prior.UserID, groupID, wasMember)
			return nil, ErrApplicationNotFound
		case rerr != nil:
			return nil, fmt.Errorf("mentorship: re-check application: %w", rerr)
		case cur.Status != "approved":
			s.compensateGrant(ctx, prior.UserID, groupID, wasMember)
			return nil, ErrApplicationNotActionable
		}
		return applicationDTO(cur), nil
	}

	row, err := s.write.ApproveMentorApplication(ctx, gen.ApproveMentorApplicationParams{
		ID:             appID,
		ReviewerUserID: reviewerID,
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Raced transition — find where the row landed. A concurrent
		// approve converges (the grant is wanted anyway); a concurrent
		// decline or delete means this call's grant now lingers on a
		// row it shouldn't, so strip it.
		cur, rerr := s.read.GetMentorApplicationByID(ctx, appID)
		if rerr == nil && cur.Status == "approved" {
			return applicationDTO(cur), nil
		}
		s.compensateGrant(ctx, prior.UserID, groupID, wasMember)
		if errors.Is(rerr, pgx.ErrNoRows) {
			return nil, ErrApplicationNotFound
		}
		return nil, ErrApplicationNotActionable
	case err != nil:
		s.compensateGrant(ctx, prior.UserID, groupID, wasMember)
		return nil, fmt.Errorf("mentorship: approve application: %w", err)
	}

	s.enqueueApplicationDecision(ctx, row.ID)
	s.trackEvent(ctx, reviewerID, "mentorship_application_approved", "mentor_application", row.ID,
		map[string]any{"applicant_id": row.UserID.String()})
	return applicationDTO(row), nil
}

// DeclineApplication transitions pending → declined (reject) or
// approved → declined (revoke). On revoke the mentors membership is
// removed BEFORE the status write; if the write then fails, a
// membership this call removed is re-added so the still-approved row
// isn't left grantless. RemoveUserFromGroup fires
// RetireMentorWithoutBecomeMentorPerm, retiring is_mentor + the shadow
// listing in the same call. POST /admin/mentorship/applications/{id}/decline.
func (s *Service) DeclineApplication(ctx context.Context, appID, reviewerID pgtype.UUID, reason string) (*MentorApplication, error) {
	prior, err := s.loadActionableApplication(ctx, appID, "pending", "approved")
	if err != nil {
		return nil, err
	}

	// Resolve the mentors group upfront when the seam is wired — the
	// revoke arm needs it to remove, and both arms need it for the
	// post-write sweep. On the pending arm a resolution failure only
	// degrades the sweep (warn); on the approved arm the revoke can't
	// run without it, so the call fails.
	var groupID pgtype.UUID
	wasMember := false // probe failure fails safe: don't fabricate a grant
	if s.groups != nil {
		var gerr error
		groupID, gerr = s.mentorsGroupID(ctx)
		switch {
		case gerr == nil:
		case prior.Status == "approved":
			return nil, gerr
		default:
			slog.Warn("mentorship: mentors group unresolved — post-decline sweep skipped", "application_id", appID, "error", gerr)
		}
	}

	// Revoke arm: an approved application carried a mentors membership.
	// Removal runs BEFORE the status write — a failed decline then leaves
	// an approved row whose grant is already gone (safely retryable,
	// the DELETE is a no-op on rerun) rather than a declined row whose
	// grant lingers. The pre-call probe lets compensation restore only a
	// membership this call actually removed.
	if prior.Status == "approved" && groupID.Valid {
		if in, perr := s.read.IsUserInGroupID(ctx, gen.IsUserInGroupIDParams{
			UserID:  prior.UserID,
			GroupID: groupID,
		}); perr == nil {
			wasMember = in
		} else {
			slog.Warn("mentorship: membership probe failed — revoke proceeds without compensation baseline", "application_id", appID, "error", perr)
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
		ExpectedStatus: prior.Status,
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Raced transition — every reachable post-state is already
		// consistent without compensation: a concurrent decline removed
		// the membership too; a member resubmit (approved → pending)
		// correctly leaves the row grantless pending re-review; a
		// delete voids the whole question.
		return nil, ErrApplicationNotActionable
	case err != nil:
		s.restoreMembership(ctx, prior.UserID, groupID, wasMember)
		return nil, fmt.Errorf("mentorship: decline application: %w", err)
	}

	// Post-decline sweep: an ensure-grant (approve on an approved row)
	// re-adds membership without changing status, which no UPDATE guard
	// can see — and a pending-row application can hold an out-of-band
	// grant the pre-write arm never touched. A declined row must hold
	// no mentors membership; remove whatever survived the write.
	s.sweepMembership(ctx, prior.UserID, groupID)

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

// compensateGrant rolls back the mentors membership after a failed
// status write — the saga-compensation arm of ApproveApplication. It
// only removes a grant this call provably created (wasMember=false);
// a pre-existing or unprobed membership is left alone so the rollback
// never strips capability the applicant held before the call.
func (s *Service) compensateGrant(ctx context.Context, userID, groupID pgtype.UUID, wasMember bool) {
	if s.groups == nil || !groupID.Valid || wasMember {
		return
	}
	if err := s.groups.RemoveUserFromGroup(ctx, userID, groupID); err != nil {
		slog.Warn("mentorship: approve compensation failed — pending row may hold a stale grant",
			"user_id", userID, "error", err)
	}
}

// restoreMembership re-adds a mentors membership that a failed revoke
// arm removed — the saga-compensation arm of DeclineApplication. It
// only re-adds a membership the probe proved existed (wasMember=true),
// so an approved row isn't left grantless and no grant is fabricated
// where none was.
func (s *Service) restoreMembership(ctx context.Context, userID, groupID pgtype.UUID, wasMember bool) {
	if s.groups == nil || !groupID.Valid || !wasMember {
		return
	}
	if err := s.groups.AddUserToGroup(ctx, userID, groupID); err != nil {
		slog.Warn("mentorship: decline compensation failed — approved row left grantless (ensure-grant retry heals)",
			"user_id", userID, "error", err)
	}
}

// sweepMembership enforces the post-decline invariant — a declined
// application holds no mentors grant. It re-probes after the status
// write and removes anything present: a raced ensure-grant add (which
// is status-neutral and escapes the UPDATE guard) or an out-of-band
// grant on a pending-row application. RemoveUserFromGroup is idempotent
// and its retire sweep self-skips while another group grants the node.
func (s *Service) sweepMembership(ctx context.Context, userID, groupID pgtype.UUID) {
	if s.groups == nil || !groupID.Valid {
		return
	}
	in, err := s.read.IsUserInGroupID(ctx, gen.IsUserInGroupIDParams{UserID: userID, GroupID: groupID})
	switch {
	case err != nil:
		slog.Warn("mentorship: post-decline membership probe failed", "user_id", userID, "error", err)
		return
	case !in:
		return
	}
	if err := s.groups.RemoveUserFromGroup(ctx, userID, groupID); err != nil {
		slog.Warn("mentorship: post-decline sweep failed — declined row left with a grant",
			"user_id", userID, "error", err)
	}
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
