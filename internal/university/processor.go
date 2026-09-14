package university

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// Processor implements the worker.UniversityProcessor interface so the five
// university Asynq tasks can delegate to this package. The worker package
// owns the task-type constants and the thin asynq handlers; this struct owns
// the business logic each task runs.
type Processor struct {
	service *Service
}

// NewProcessor creates a university processor bound to the provided service.
func NewProcessor(service *Service) *Processor {
	return &Processor{service: service}
}

// ProcessEnrollment handles university:process_enrollment.
// Payload: { , user_id, track_id, payment_id, source, gross_kobo }.
func (p *Processor) ProcessEnrollment(ctx context.Context, userID, trackID, paymentID, source string, grossKobo int64) error {
	uID, err := scanUUID(userID)
	if err != nil {
		return fmt.Errorf("university: process enrollment: %w", err)
	}

	trID, err := scanUUID(trackID)
	if err != nil {
		return fmt.Errorf("university: process enrollment: %w", err)
	}

	var payID pgtype.UUID
	if paymentID != "" {
		if err := payID.Scan(paymentID); err != nil {
			return fmt.Errorf("university: process enrollment: invalid payment_id: %w", err)
		}
	}

	return p.service.ProcessEnrollment(ctx, uID, trID, payID, source, grossKobo)
}

// CalculatePeriodPayouts handles university:calculate_period_payouts.
// Payload: { , period_start, period_end }.
func (p *Processor) CalculatePeriodPayouts(ctx context.Context, periodStart, periodEnd string) error {
	start, err := time.Parse(time.DateOnly, periodStart)
	if err != nil {
		return fmt.Errorf("university: calculate payouts: invalid period_start: %w", err)
	}

	end, err := time.Parse(time.DateOnly, periodEnd)
	if err != nil {
		return fmt.Errorf("university: calculate payouts: invalid period_end: %w", err)
	}

	return p.service.CalculatePeriodPayouts(ctx, start, end)
}

// SendInstructorPayout handles university:send_instructor_payout.
// Payload: { , payout_id }.
func (p *Processor) SendInstructorPayout(ctx context.Context, payoutID string) error {
	pID, err := scanUUID(payoutID)
	if err != nil {
		return fmt.Errorf("university: send payout: %w", err)
	}

	return p.service.SendInstructorPayout(ctx, pID)
}

// NotifyInstructorApproved handles university:notify_instructor_approved.
// Payload: { , instructor_id }
// Dispatches an in-app notification to the creator via the notifications
// service (wired via Service.SetNotifier). If no notifier is wired, logs only.
// Ref: japaconnect-engineering/sections/35-notification-system.md.
func (p *Processor) NotifyInstructorApproved(ctx context.Context, instructorID string) error {
	iID, err := scanUUID(instructorID)
	if err != nil {
		return fmt.Errorf("university: notify instructor approved: bad instructor_id: %w", err)
	}

	return p.service.NotifyInstructorApprovedDispatch(ctx, iID)
}

// NotifyCourseApproved handles university:notify_course_approved.
// Payload: { , track_id }
// Dispatches an in-app notification to the creator via the notifications
// service (wired via Service.SetNotifier). If no notifier is wired, logs only.
// Ref: japaconnect-engineering/sections/35-notification-system.md.
func (p *Processor) NotifyCourseApproved(ctx context.Context, trackID string) error {
	trID, err := scanUUID(trackID)
	if err != nil {
		return fmt.Errorf("university: notify course approved: bad track_id: %w", err)
	}

	return p.service.NotifyCourseApprovedDispatch(ctx, trID)
}

// FetchLogo handles university:fetch_logo (Section 39, low tier).
// Downloads a university logo from a remote URL, stores it to SeaweedFS,
// and updates the space's logo_media_id.
// Payload: { , space_id, logo_url, uploaded_by }. uploaded_by (the
// acting admin's user ID) is attributed as the media uploader; there is no
// generic "system user" row so an invalid/missing uploaded_by is passed
// through as an invalid pgtype.UUID and Service.FetchLogo skips gracefully.
func (p *Processor) FetchLogo(ctx context.Context, spaceID, logoURL, uploadedBy string) error {
	sID, err := scanUUID(spaceID)
	if err != nil {
		return fmt.Errorf("university: fetch logo: invalid space_id: %w", err)
	}
	// uploadedBy is best-effort: an invalid/empty value is passed through as
	// a zero-value (invalid) pgtype.UUID rather than failing the task.
	uID, _ := scanUUID(uploadedBy)

	return p.service.FetchLogo(ctx, sID, logoURL, uID)
}

// scanUUID parses a UUID string into a pgtype.UUID.
func scanUUID(s string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(s); err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid uuid %q: %w", s, err)
	}

	return id, nil
}

// ensure encoding/json import is used (for future payload parsing).
var _ = json.Marshal
