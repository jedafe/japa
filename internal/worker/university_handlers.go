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
// Japa University creator economy task handlers (ADR-063)
// ---------------------------------------------------------------------------

// HandleUniversityProcessEnrollment processes a successful course_access
// payment: it credits the creator revenue ledger, upserts the enrollment,
// and increments the track's cached enrollment counter. Idempotent on
// payment_id (AUDIT-BE-005).
//
// Payload: { tenant_id, user_id, track_id, payment_id, source, gross_kobo }
func HandleUniversityProcessEnrollment(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:university:process_enrollment")

	if universityProcessor == nil {
		slog.Warn("task:university:process_enrollment: no processor registered, skipping")
		return nil
	}

	var payload struct {
		TenantID  string `json:"tenant_id"`
		UserID    string `json:"user_id"`
		TrackID   string `json:"track_id"`
		PaymentID string `json:"payment_id"`
		Source    string `json:"source"`
		GrossKobo int64  `json:"gross_kobo"`
	}
	if err := parsePayloadOrSkip(t, &payload); err != nil {
		slog.Error("task:university:process_enrollment: invalid payload", "error", err)
		return err
	}

	if err := universityProcessor.ProcessEnrollment(ctx, payload.TenantID, payload.UserID,
		payload.TrackID, payload.PaymentID, payload.Source, payload.GrossKobo); err != nil {
		slog.Error("task:university:process_enrollment: processor failed", "error", err)
		return err
	}
	return nil
}

// HandleUniversityCalculatePeriodPayouts walks every approved instructor
// with open revenue_splits in the period and rolls them into instructor_payouts
// rows when the accrued net meets the configured minimum.
//
// Payload: { tenant_id, period_start, period_end }
// When period_start/period_end are empty, the previous calendar month is used.
func HandleUniversityCalculatePeriodPayouts(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:university:calculate_period_payouts")

	if universityProcessor == nil {
		slog.Warn("task:university:calculate_period_payouts: no processor registered, skipping")
		return nil
	}

	var payload struct {
		TenantID    string `json:"tenant_id"`
		PeriodStart string `json:"period_start"`
		PeriodEnd   string `json:"period_end"`
	}
	if err := parsePayloadOrSkip(t, &payload); err != nil {
		slog.Error("task:university:calculate_period_payouts: invalid payload", "error", err)
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

	if err := universityProcessor.CalculatePeriodPayouts(ctx, payload.TenantID,
		payload.PeriodStart, payload.PeriodEnd); err != nil {
		slog.Error("task:university:calculate_period_payouts: processor failed", "error", err)
		return err
	}
	return nil
}

// HandleUniversitySendInstructorPayout executes a single pending/failed
// instructor payout: marks it processing, performs the transfer, and stamps
// the final status with a provider reference.
//
// Payload: { tenant_id, payout_id }
func HandleUniversitySendInstructorPayout(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:university:send_instructor_payout")

	if universityProcessor == nil {
		slog.Warn("task:university:send_instructor_payout: no processor registered, skipping")
		return nil
	}

	var payload struct {
		TenantID string `json:"tenant_id"`
		PayoutID string `json:"payout_id"`
	}
	if err := parsePayloadOrSkip(t, &payload); err != nil {
		slog.Error("task:university:send_instructor_payout: invalid payload", "error", err)
		return err
	}

	if err := universityProcessor.SendInstructorPayout(ctx, payload.TenantID, payload.PayoutID); err != nil {
		slog.Error("task:university:send_instructor_payout: processor failed", "error", err)
		return err
	}
	return nil
}

// HandleUniversityNotifyInstructorApproved notifies a creator that their
// instructor application was approved. V1: log only; the notification
// dispatch is wired once the template catalog includes university templates.
//
// Payload: { tenant_id, instructor_id }
func HandleUniversityNotifyInstructorApproved(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:university:notify_instructor_approved")

	if universityProcessor == nil {
		return nil
	}

	var payload struct {
		TenantID     string `json:"tenant_id"`
		InstructorID string `json:"instructor_id"`
	}
	if err := parsePayloadOrSkip(t, &payload); err != nil {
		return err
	}
	return universityProcessor.NotifyInstructorApproved(ctx, payload.TenantID, payload.InstructorID)
}

// HandleUniversityNotifyCourseApproved notifies a creator that their
// submitted track was approved to the marketplace.
//
// Payload: { tenant_id, track_id }
func HandleUniversityNotifyCourseApproved(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:university:notify_course_approved")

	if universityProcessor == nil {
		return nil
	}

	var payload struct {
		TenantID string `json:"tenant_id"`
		TrackID  string `json:"track_id"`
	}
	if err := parsePayloadOrSkip(t, &payload); err != nil {
		return err
	}
	return universityProcessor.NotifyCourseApproved(ctx, payload.TenantID, payload.TrackID)
}

// ensure encoding/json + fmt imports are referenced (used by parsePayload).
var (
	_ = json.Marshal
	_ = fmt.Sprintf
)
