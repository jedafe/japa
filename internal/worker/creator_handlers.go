package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
)

// ---------------------------------------------------------------------------
// Japa Academy creator economy task handlers (ADR-063)
// ---------------------------------------------------------------------------

// HandleAcademyProcessEnrollment processes a successful course_access
// payment: it credits the creator revenue ledger, upserts the enrollment,
// and increments the track's cached enrollment counter. Idempotent on
// payment_id (AUDIT-BE-005).
// // Payload: { user_id, track_id, payment_id, source, gross_kobo }.
func HandleAcademyProcessEnrollment(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:academy:process_enrollment")

	if creatorProcessor == nil {
		slog.Warn("task:academy:process_enrollment: no processor registered, skipping")
		return nil
	}

	var payload struct {
		UserID    string `json:"user_id"`
		TrackID   string `json:"track_id"`
		PaymentID string `json:"payment_id"`
		Source    string `json:"source"`
		GrossKobo int64  `json:"gross_kobo"`
	}

	if err := parsePayloadOrSkip(t, &payload); err != nil {
		slog.Error("task:academy:process_enrollment: invalid payload", "error", err)
		return err
	}

	if err := creatorProcessor.ProcessEnrollment(ctx, payload.UserID,
		payload.TrackID, payload.PaymentID, payload.Source, payload.GrossKobo); err != nil {
		slog.Error("task:academy:process_enrollment: processor failed", "error", err)
		return err
	}

	return nil
}

// HandleAcademyCalculatePeriodPayouts walks every approved instructor
// with open revenue_splits in the period and rolls them into academy_instructor_payouts
// rows when the accrued net meets the configured minimum.
// // Payload: { period_start, period_end }
// When period_start/period_end are empty, the previous calendar month is used.
// When is empty (cron-mode sweep), the handler iterates all active
// platforms and runs the aggregation per platform — mirroring the pattern used by
// the sitemap, RSS, and security-scan sweep handlers (BUG-001/BUG-002).
func HandleAcademyCalculatePeriodPayouts(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:academy:calculate_period_payouts")

	if creatorProcessor == nil {
		slog.Warn("task:academy:calculate_period_payouts: no processor registered, skipping")
		return nil
	}

	var payload struct {
		PeriodStart string `json:"period_start"`
		PeriodEnd   string `json:"period_end"`
	}

	if err := parsePayloadOrSkip(t, &payload); err != nil {
		slog.Error("task:academy:calculate_period_payouts: invalid payload", "error", err)
		return err
	}

	if payload.PeriodStart == "" || payload.PeriodEnd == "" {
		// Default to the previous calendar month.
		now := time.Now().UTC()
		start := time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		payload.PeriodStart = start.Format(time.DateOnly)
		payload.PeriodEnd = end.Format(time.DateOnly)
	}

	if err := creatorProcessor.CalculatePeriodPayouts(ctx,
		payload.PeriodStart, payload.PeriodEnd); err != nil {
		slog.Error("task:academy:calculate_period_payouts: processor failed", "error", err)
	}

	return nil
}

// HandleAcademySendInstructorPayout executes a single pending/failed
// instructor payout: marks it processing, performs the transfer, and stamps
// the final status with a provider reference.
// // Payload: { payout_id }.
func HandleAcademySendInstructorPayout(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:academy:send_instructor_payout")

	if creatorProcessor == nil {
		slog.Warn("task:academy:send_instructor_payout: no processor registered, skipping")
		return nil
	}

	var payload struct {
		PayoutID string `json:"payout_id"`
	}

	if err := parsePayloadOrSkip(t, &payload); err != nil {
		slog.Error("task:academy:send_instructor_payout: invalid payload", "error", err)
		return err
	}

	if err := creatorProcessor.SendInstructorPayout(ctx, payload.PayoutID); err != nil {
		slog.Error("task:academy:send_instructor_payout: processor failed", "error", err)
		return err
	}

	return nil
}

// HandleAcademyNotifyInstructorApproved notifies a creator that their
// instructor application was approved. V1: log only; the notification
// dispatch is wired once the template catalog includes university templates.
// // Payload: { instructor_id }.
func HandleAcademyNotifyInstructorApproved(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:academy:notify_instructor_approved")

	if creatorProcessor == nil {
		return nil
	}

	var payload struct {
		InstructorID string `json:"instructor_id"`
	}

	if err := parsePayloadOrSkip(t, &payload); err != nil {
		return err
	}

	return creatorProcessor.NotifyInstructorApproved(ctx, payload.InstructorID)
}

// HandleAcademyNotifyCourseApproved notifies a creator that their
// submitted track was approved to the marketplace.
// // Payload: { track_id }.
func HandleAcademyNotifyCourseApproved(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:academy:notify_course_approved")

	if creatorProcessor == nil {
		return nil
	}

	var payload struct {
		TrackID string `json:"track_id"`
	}

	if err := parsePayloadOrSkip(t, &payload); err != nil {
		return err
	}

	return creatorProcessor.NotifyCourseApproved(ctx, payload.TrackID)
}

// HandleUniversityFetchLogo downloads a university logo from a remote URL,
// stores it to SeaweedFS via the media service, and updates the space's
// logo_media_id. Enqueued when an admin sets/changes a university's
// logo_url via POST/PATCH /admin/universities (Section 39). Low-tier —
// logo fetching is non-urgent and can be retried.
// // Payload: { space_id, logo_url, uploaded_by }. uploaded_by is
// the acting admin's user ID, attributed as the media uploader — there is
// no generic "system user" row, so a missing/invalid uploaded_by causes
// the fetch to be skipped rather than violating the media.uploader_user_id
// NOT NULL FK.
func HandleUniversityFetchLogo(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:university:fetch_logo")

	if creatorProcessor == nil {
		slog.Warn("task:university:fetch_logo: no processor registered, skipping")
		return nil
	}

	var payload struct {
		SpaceID    string `json:"space_id"`
		LogoURL    string `json:"logo_url"`
		UploadedBy string `json:"uploaded_by"`
	}

	if err := parsePayloadOrSkip(t, &payload); err != nil {
		slog.Error("task:university:fetch_logo: invalid payload", "error", err)
		return err
	}

	if payload.LogoURL == "" {
		slog.Warn("task:university:fetch_logo: empty logo_url, skipping")
		return nil
	}

	if err := creatorProcessor.FetchLogo(ctx, payload.SpaceID, payload.LogoURL, payload.UploadedBy); err != nil {
		slog.Error("task:university:fetch_logo: processor failed", "error", err)
		return err
	}

	return nil
}

// ensure encoding/json + fmt imports are referenced (used by parsePayload).
var (
	_ = json.Marshal
	_ = fmt.Sprintf
)
