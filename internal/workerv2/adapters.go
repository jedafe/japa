// Package workerv2 bridges the v2 processor interfaces (defined in
// internal/worker/v2_processors.go) to the v2 services. The processor
// interfaces are called by Asynq task handlers which receive a single entity
// ID (or no ID for periodic tasks). These adapters delegate to the
// appropriate service method.
//
// This package lives separate from internal/worker to avoid an import cycle:
// worker → assessment → auth → worker. By placing the adapters here, worker
// never imports the service packages, and the services never import workerv2.
// Ref: japaconnect-engineering/sections/60-78 (v2 features), ADRs 061-086.
package workerv2

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/adsselfserve"
	"github.com/jedafe/japa/internal/assessment"
	"github.com/jedafe/japa/internal/contentimport"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/devices"
	"github.com/jedafe/japa/internal/fx"
	"github.com/jedafe/japa/internal/gamification"
	"github.com/jedafe/japa/internal/jobs"
	"github.com/jedafe/japa/internal/notifications"
	"github.com/jedafe/japa/internal/referral"
	"github.com/jedafe/japa/internal/remittance"
	"github.com/jedafe/japa/internal/scholarships"
	"github.com/jedafe/japa/internal/worker"
)

// ---------------------------------------------------------------------------
// JobsAdapter — wraps jobs.Service to satisfy JobsProcessor (§63, ADR-081).
// ---------------------------------------------------------------------------

// JobsAdapter adapts the jobs.Service to the JobsProcessor interface
// expected by the Asynq task handlers.
type JobsAdapter struct {
	svc   *jobs.Service
	db    *db.DB
	notif *notifications.Service
}

// NewJobsAdapter creates a JobsAdapter wrapping the given service and database.
// The notifications service is used to dispatch the application_status_update
// notification to the applicant after a status change (§63, ADR-081).
func NewJobsAdapter(svc *jobs.Service, database *db.DB, notifSvc *notifications.Service) *JobsAdapter {
	return &JobsAdapter{svc: svc, db: database, notif: notifSvc}
}

// ExpireListingsPastDeadline expires job listings past their deadline.
func (a *JobsAdapter) ExpireListingsPastDeadline(ctx context.Context) (int, error) {
	n, err := a.svc.ExpireListingsPastDeadline(ctx)
	if err != nil {
		return 0, err
	}

	return int(n), nil
}

// NotifyApplicationStatus looks up the application's platform and notifies the
// applicant of a status change. The status change already happened in the
// HTTP handler before the task was enqueued; this method only sends the
// notification (§63, ADR-081).
func (a *JobsAdapter) NotifyApplicationStatus(ctx context.Context, applicationID pgtype.UUID) error {
	app, err := a.svc.GetApplicationForNotification(ctx, applicationID)
	if err != nil {
		return fmt.Errorf("jobs: notify application status: fetch application: %w", err)
	}

	if a.notif != nil {
		if err := a.notif.CreateNotification(ctx, app.ApplicantID,
			"job_application_status_update", "system", pgtype.UUID{},
			"job_application", applicationID,
			"Job Application Update",
			"Your application status is now: "+app.Status,
			""); err != nil {
			slog.Warn("jobs: notify application status: create notification failed", "application_id", applicationID.String(), "error", err)
		}
	}

	slog.Info("jobs: notify application status", "application_id", applicationID.String(), "applicant_id", app.ApplicantID.String(), "status", app.Status)

	return nil
}

// ---------------------------------------------------------------------------
// GamificationAdapter — wraps gamification.Service (§64, ADR-082).
// ---------------------------------------------------------------------------

// GamificationAdapter adapts the gamification.Service to the
// GamificationProcessor interface.
type GamificationAdapter struct {
	svc *gamification.Service
	db  *db.DB
}

// NewGamificationAdapter creates a GamificationAdapter.
func NewGamificationAdapter(svc *gamification.Service, database *db.DB) *GamificationAdapter {
	return &GamificationAdapter{svc: svc, db: database}
}

// ExpireChallenges expires ended challenges.
func (a *GamificationAdapter) ExpireChallenges(ctx context.Context) (int, error) {
	n, err := a.svc.ExpireChallenges(ctx)
	if err != nil {
		return 0, err
	}

	return int(n), nil
}

// ---------------------------------------------------------------------------
// ImportAdapter — wraps contentimport.Service (§65, ADR-083).
// ---------------------------------------------------------------------------

// ImportAdapter adapts the contentimport.Service to the
// ImportProcessor interface. The Asynq payload carries only import_id; the
// adapter resolves , source_type, and source_url from the database.
type ImportAdapter struct {
	svc *contentimport.Service
	db  *db.DB
}

// NewImportAdapter creates an ImportAdapter.
func NewImportAdapter(svc *contentimport.Service, database *db.DB) *ImportAdapter {
	return &ImportAdapter{svc: svc, db: database}
}

// importRow holds the fields needed to call ProcessSource.
type importRow struct {
	SourceType string
	SourceURL  string
}

func (a *ImportAdapter) lookupImport(ctx context.Context, id pgtype.UUID) (importRow, error) {
	var r importRow

	err := a.db.Reader.QueryRow(
		ctx,
		"SELECT source_type, source_url FROM content_imports WHERE id = $1", id,
	).Scan(&r.SourceType, &r.SourceURL)
	if err != nil {
		return importRow{}, fmt.Errorf("looking up content_import: %w", err)
	}

	return r, nil
}

// ProcessSource resolves the import's platform and delegates to the service.
func (a *ImportAdapter) ProcessSource(ctx context.Context, importID pgtype.UUID) error {
	r, err := a.lookupImport(ctx, importID)
	if err != nil {
		return err
	}

	return a.svc.ProcessSource(ctx, importID, r.SourceType, r.SourceURL)
}

// FetchMedia delegates to the contentimport service's FetchMedia method,
// which downloads external images from the imported content's body and
// replaces their URLs with stored media URLs (Section 34).
func (a *ImportAdapter) FetchMedia(ctx context.Context, userID, contentID pgtype.UUID) error {
	return a.svc.FetchMedia(ctx, userID, contentID)
}

// ---------------------------------------------------------------------------
// AssessmentAdapter — wraps assessment.Service (§71, ADR-071).
// ---------------------------------------------------------------------------

// AssessmentAdapter adapts the assessment.Service to the
// AssessmentProcessor interface.
type AssessmentAdapter struct {
	svc   *assessment.Service
	db    *db.DB
	notif *notifications.Service
}

// NewAssessmentAdapter creates an AssessmentAdapter. The notifications
// service is used to dispatch the assessment_complete notification after
// scoring (ADR-071 §2 step 7).
func NewAssessmentAdapter(svc *assessment.Service, database *db.DB, notifSvc *notifications.Service) *AssessmentAdapter {
	return &AssessmentAdapter{svc: svc, db: database, notif: notifSvc}
}

// ScoreSubmission resolves the submission's platform, user, and field_values
// from form_submissions, then runs the scoring engine which writes the
// assessment_results row. The result does not exist yet at enqueue time —
// scoring creates it (§71). After scoring, an assessment_complete
// notification is dispatched to the authenticated user (ADR-071 §2 step 7).
func (a *AssessmentAdapter) ScoreSubmission(ctx context.Context, submissionID pgtype.UUID) error {
	var userID pgtype.UUID

	var fieldValues []byte

	err := a.db.Reader.QueryRow(
		ctx,
		"SELECT user_id, field_values FROM form_submissions WHERE id = $1",
		submissionID,
	).Scan(&userID, &fieldValues)
	if err != nil {
		return fmt.Errorf("looking up form_submission: %w", err)
	}

	resultID, created, err := a.svc.ScoreSubmission(ctx, assessment.ScoreSubmissionInput{
		SubmissionID: submissionID,
		UserID:       userID,
		FieldValues:  fieldValues,
	})
	if err != nil {
		return err
	}

	// ADR-071 §2 step 7: notify the user that their assessment result is
	// ready. Anonymous users (userID invalid) are skipped — they see the
	// result in-session via polling. Retried tasks that find the result
	// already written (created=false) skip the notification so a retry
	// never double-notifies.
	if created && userID.Valid && a.notif != nil {
		actionURL := "/tools/readiness?result=" + resultID.String()
		if err := a.notif.CreateNotification(
			ctx,

			userID,
			"assessment_complete",
			"system",
			pgtype.UUID{},
			"assessment_result",
			resultID,
			"Your readiness assessment is complete",
			"Your Japa Readiness Assessment has been scored. View your results and personalized roadmap.",
			actionURL,
		); err != nil {
			slog.Warn("task:assessment:score_submission: notification dispatch failed", "error", err)
		}
	}

	return nil
}

// CleanupAnonymousResults deletes anonymous assessment_results older than the
// given retention window. Called by the assessment:cleanup_anonymous Asynq
// task (ADR-071 §5).
func (a *AssessmentAdapter) CleanupAnonymousResults(ctx context.Context, retentionHours int32) (int64, error) {
	return a.svc.CleanupAnonymousResults(ctx, retentionHours)
}

// RescoreStaleSubmissions re-enqueues scoring for submissions that never got
// a result (e.g. the original enqueue failed after the row was written).
func (a *AssessmentAdapter) RescoreStaleSubmissions(ctx context.Context, staleMinutes int32) (int, error) {
	return a.svc.RescoreStaleSubmissions(ctx, staleMinutes)
}

// ---------------------------------------------------------------------------
// ScholarshipAdapter — wraps scholarships.Service (§73, ADR-073).
// ---------------------------------------------------------------------------

// ScholarshipAdapter adapts the scholarships.Service to the
// ScholarshipProcessor interface.
type ScholarshipAdapter struct {
	svc   *scholarships.Service
	db    *db.DB
	notif *notifications.Service
}

// NewScholarshipAdapter creates a ScholarshipAdapter. The notifications
// service is used to dispatch deadline-reminder notifications per
// ADR-073 §3 step 3.
func NewScholarshipAdapter(svc *scholarships.Service, database *db.DB, notif *notifications.Service) *ScholarshipAdapter {
	return &ScholarshipAdapter{svc: svc, db: database, notif: notif}
}

// CheckDeadlines finds saved scholarships whose application deadline is
// within the user's remind_before_days window, dispatches a notification
// per match (ADR-073 §3 step 3), and marks the reminder as sent.
func (a *ScholarshipAdapter) CheckDeadlines(ctx context.Context) (int, error) {
	var total int

	reminders, err := a.svc.FindNeedingReminders(ctx, 100)
	if err != nil {
		slog.Warn("task:scholarship:check_deadlines: find reminders failed", "error", err)
		return 0, err
	}

	for _, r := range reminders {
		if err := a.dispatchReminder(ctx, r); err != nil {
			slog.Warn("task:scholarship:check_deadlines: dispatch reminder failed", "reminder_id", r.ID, "error", err)
			continue
		}

		if err := a.svc.MarkReminderSent(ctx, r.ID); err != nil {
			slog.Warn("task:scholarship:check_deadlines: mark reminder sent failed", "reminder_id", r.ID, "error", err)
			continue
		}

		total++
	}

	return total, nil
}

// dispatchReminder creates an in-app notification for the saved-scholarship
// owner announcing the approaching application deadline. The notification
// type `scholarship.deadline_reminder` is created via the notifications
// service's non-batch path (§35, ADR-073 §3).
func (a *ScholarshipAdapter) dispatchReminder(ctx context.Context, r gen.FindScholarshipsNeedingRemindersRow) error {
	if a.notif == nil {
		slog.Warn("task:scholarship:check_deadlines: notifications service not configured, skipping dispatch")
		return nil
	}

	deadline := r.DeadlineValue

	title := "Scholarship deadline approaching"
	if r.GuideTitle != "" {
		title = "Deadline approaching: " + r.GuideTitle
	}

	body := fmt.Sprintf("The application deadline for %q is %s. Apply soon.", r.GuideTitle, deadline)

	// The main app's finder has no detail page for a single guide; link to
	// the finder search for the scholarship so the notification resolves to a
	// real page (the previous /scholarships/{id} link routed to a 404).
	actionURL := "/scholarships"
	if r.GuideTitle != "" {
		actionURL = "/scholarships?q=" + url.QueryEscape(r.GuideTitle)
	}

	return a.notif.CreateNotification(
		ctx,

		r.UserID,
		"scholarship.deadline_reminder",
		"system",
		pgtype.UUID{},
		"scholarship",
		r.GuideID,
		title,
		body,
		actionURL,
	)
}

// ---------------------------------------------------------------------------
// RemittanceAdapter — wraps remittance.Service (§74, ADR-075).
// ---------------------------------------------------------------------------

// RemittanceAdapter adapts the remittance.Service to the
// RemittanceProcessor interface.
type RemittanceAdapter struct {
	svc *remittance.Service
	db  *db.DB
}

// NewRemittanceAdapter creates a RemittanceAdapter.
func NewRemittanceAdapter(svc *remittance.Service, database *db.DB) *RemittanceAdapter {
	return &RemittanceAdapter{svc: svc, db: database}
}

// FetchRates resolves the provider's platform and performs the actual rate
// fetch by calling the provider's API adapter, inserting rate rows, and
// invalidating the corridor cache (ADR-075 §2). This is the task-handler
// entry point — it does NOT re-enqueue the fetch task (which would create
// an infinite loop). The admin manual trigger calls TriggerFetch (which
// enqueues this task), and the scheduler enqueues it on the configured
// interval.
func (a *RemittanceAdapter) FetchRates(ctx context.Context, providerID pgtype.UUID) error {
	return a.svc.FetchProviderRates(ctx, providerID)
}

// FetchAllRates fetches rates for all active API-sourced providers. Called
// by the scheduler sweep (empty payload). Manual providers are skipped
// (rates are admin-entered). Returns the number of providers processed.
// Ref: ADR-075 §2, §8.
func (a *RemittanceAdapter) FetchAllRates(ctx context.Context) (int, error) {
	n, err := a.svc.FetchAllProviderRates(ctx)
	if err != nil {
		slog.Warn("task:remittance:fetch_rates: sweep failed", "error", err)
		return 0, err
	}

	return n, nil
}

// CheckAlerts evaluates all active rate alerts against the current best
// rate for each corridor, dispatches notifications for alerts whose
// thresholds are met, and marks them triggered (ADR-075 §4). The 24-hour
// debounce is enforced by the SQL query (last_triggered_at is null or
// > 24h ago).
func (a *RemittanceAdapter) CheckAlerts(ctx context.Context) (int, error) {
	n, err := a.svc.CheckAndDispatchAlerts(ctx)
	if err != nil {
		slog.Warn("task:remittance:check_alerts: check failed", "error", err)
		return 0, err
	}

	return n, nil
}

// PruneOldRates prunes rates beyond the configured retention period
// (remittance.history_retention_days, default 365). The SQL keeps one
// snapshot per provider per corridor per day for older data (daily
// downsampling). Ref: ADR-075 §7, Rule 3 (no hardcoded config).
func (a *RemittanceAdapter) PruneOldRates(ctx context.Context) (int, error) {
	retentionDays := a.svc.HistoryRetentionDays(ctx)

	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	if err := a.svc.PruneOldRates(ctx, cutoff); err != nil {
		slog.Warn("task:remittance:prune_old_rates: sweep failed", "error", err)
		return 0, err
	}

	return 1, nil
}

// ---------------------------------------------------------------------------
// FxAdapter — wraps fx.Service (§112, ADR-125).
// ---------------------------------------------------------------------------

// FxAdapter adapts fx.Service to the worker.FxProcessor interface.
type FxAdapter struct {
	svc *fx.Service
}

// NewFxAdapter creates an FxAdapter.
func NewFxAdapter(svc *fx.Service) *FxAdapter {
	return &FxAdapter{svc: svc}
}

// PruneRates deletes fx_rates rows past fx.history_retention_days (§112).
func (a *FxAdapter) PruneRates(ctx context.Context) (int, error) {
	return a.svc.PruneRates(ctx)
}

// ---------------------------------------------------------------------------
// ReferralAdapter — wraps referral.Service (§60, ADR-078).
// ---------------------------------------------------------------------------

// ReferralAdapter adapts the referral.Service to the ReferralProcessor
// interface. The service's RecordClick uses a sessionID; the processor
// interface expects IP + userAgent. The adapter constructs a synthetic
// sessionID hash from IP + userAgent so the click is recorded.
type ReferralAdapter struct {
	svc *referral.Service
	db  *db.DB
}

// NewReferralAdapter creates a ReferralAdapter.
func NewReferralAdapter(svc *referral.Service, database *db.DB) *ReferralAdapter {
	return &ReferralAdapter{svc: svc, db: database}
}

// RecordClick records a referral click. The sessionID is now passed through
// from the redirect handler (BUG-005/BUG-010 fix) instead of being synthesized
// from IP+userAgent, which was lossy and collision-prone.
func (a *ReferralAdapter) RecordClick(ctx context.Context, code, sessionID, ip, userAgent string) error {
	if sessionID == "" {
		// Backward-compat fallback for older payloads without session_id.
		sessionID = ip + "|" + userAgent
	}

	return a.svc.RecordClick(ctx, code, sessionID)
}

// CheckConversion runs the referral conversion check for a successful payment.
// Delegates to referral.Service.CheckConversion which looks up the payer's
// referral row, runs fraud checks, and transitions pending→qualified→credited.
func (a *ReferralAdapter) CheckConversion(ctx context.Context, payerID, paymentID pgtype.UUID, sessionID string) error {
	return a.svc.CheckConversion(ctx, payerID, paymentID, sessionID)
}

// ReviewFlagged is the daily sweep hook (§60 referral:review_flagged).
// BUG-007 fix: the previous implementation auto-credited flagged referrals,
// defeating the fraud-control purpose. The daily sweep should only SURFACE
// flagged referrals for admin review — it must not credit them. The admin
// resolves flagged referrals manually via PATCH /admin/referrals/:id/resolve.
// This hook is a no-op placeholder for future notification wiring; the
// flagged queue is already exposed via GET /admin/referrals/flagged.
func (a *ReferralAdapter) ReviewFlagged(ctx context.Context, referralID pgtype.UUID) error {
	slog.Info("task:referral:review_flagged: surfaced for admin review", "referral_id", referralID)
	return nil
}

// ReviewFlaggedSweep is the daily cron path — reports the size of the
// flagged queue so ops sees it in logs/alerts. The queue itself is served by
// GET /admin/referrals/flagged; resolution stays manual (BUG-007).
func (a *ReferralAdapter) ReviewFlaggedSweep(ctx context.Context) (int, error) {
	_, total, err := a.svc.AdminListFlagged(ctx, 1, 1)
	if err != nil {
		return 0, err
	}

	return int(total), nil // #nosec G115 — flagged counts are small
}

// ---------------------------------------------------------------------------
// AdsSelfServeAdapter — wraps adsselfserve.Service (§61, ADR-079).
// ---------------------------------------------------------------------------

// AdsSelfServeAdapter adapts the adsselfserve.Service to the
// AdsSelfServeProcessor interface.
type AdsSelfServeAdapter struct {
	svc *adsselfserve.Service
	db  *db.DB
}

// NewAdsSelfServeAdapter creates an AdsSelfServeAdapter.
func NewAdsSelfServeAdapter(svc *adsselfserve.Service, database *db.DB) *AdsSelfServeAdapter {
	return &AdsSelfServeAdapter{svc: svc, db: database}
}

// RefundCampaignCredits resolves the campaign's platform and refunds credits.
func (a *AdsSelfServeAdapter) RefundCampaignCredits(ctx context.Context, campaignID pgtype.UUID) error {
	return a.svc.RefundCampaignCredits(ctx, campaignID)
}

// NotifyCampaignStatus resolves the campaign's platform and notifies the
// advertiser of the review decision (approved/rejected).
// Ref: japaconnect-engineering/sections/61-self-serve-ads.md (Asynq Tasks).
func (a *AdsSelfServeAdapter) NotifyCampaignStatus(ctx context.Context, campaignID pgtype.UUID) error {
	return a.svc.NotifyCampaignStatus(ctx, campaignID)
}

// ---------------------------------------------------------------------------
// PushAdapter — wraps devices.Service (§77, ADR-074).
// ---------------------------------------------------------------------------

// PushAdapter adapts the devices.Service to the PushProcessor
// interface. The Asynq payload carries notification_id + device_token_id;
// the adapter resolves the user and platform from the device token, then
// delegates to the service's SendPush.
type PushAdapter struct {
	svc *devices.Service
	db  *db.DB
}

// NewPushAdapter creates a PushAdapter.
func NewPushAdapter(svc *devices.Service, database *db.DB) *PushAdapter {
	return &PushAdapter{svc: svc, db: database}
}

// SendPush resolves the device token's platform and user, then sends the push
// notification. The notification payload is loaded from the notifications table.
func (a *PushAdapter) SendPush(ctx context.Context, notificationID, deviceTokenID pgtype.UUID) error {
	var userID pgtype.UUID

	err := a.db.Reader.QueryRow(
		ctx,
		"SELECT user_id FROM device_tokens WHERE id = $1", deviceTokenID,
	).Scan(&userID)
	if err != nil {
		return fmt.Errorf("looking up device token: %w", err)
	}

	// Load the notification to build the push payload.
	var title, body string

	err = a.db.Reader.QueryRow(
		ctx,
		"SELECT title, body FROM notifications WHERE id = $1",
		notificationID,
	).Scan(&title, &body)
	if err != nil {
		return fmt.Errorf("looking up notification: %w", err)
	}

	return a.svc.SendPush(ctx, userID, devices.PushPayload{
		Title: title,
		Body:  body,
		Data: map[string]interface{}{
			"notification_id": notificationID.String(),
		},
	})
}

// ---------------------------------------------------------------------------
// RegisterV2Processors wires all implemented v2 service adapters to the
// worker's processor slots. Call once at startup before the worker serves.
// PersonaProcessor and StudioProcessor are NOT registered here because their
// implementation packages (§76 Participation Orchestration, §75 AI Content
// Studio) are not yet implemented — their task handlers will log "no processor
// registered, skipping" and return nil until those packages are built.
// ---------------------------------------------------------------------------

// RegisterV2Processors wires all available v2 adapters to the worker.
// database is the *db.DB instance shared with the HTTP server.
// The service pointers are the same instances created in server.NewServer.
func RegisterV2Processors(
	database *db.DB,
	jobsSvc *jobs.Service,
	gamSvc *gamification.Service,
	impSvc *contentimport.Service,
	assSvc *assessment.Service,
	schSvc *scholarships.Service,
	remSvc *remittance.Service,
	fxSvc *fx.Service,
	refSvc *referral.Service,
	adsSvc *adsselfserve.Service,
	devSvc *devices.Service,
	notifSvc *notifications.Service,
) {
	worker.RegisterJobsProcessor(NewJobsAdapter(jobsSvc, database, notifSvc))
	worker.RegisterGamificationProcessor(NewGamificationAdapter(gamSvc, database))
	worker.RegisterImportProcessor(NewImportAdapter(impSvc, database))
	worker.RegisterAssessmentProcessor(NewAssessmentAdapter(assSvc, database, notifSvc))
	worker.RegisterScholarshipProcessor(NewScholarshipAdapter(schSvc, database, notifSvc))
	worker.RegisterRemittanceProcessor(NewRemittanceAdapter(remSvc, database))
	worker.RegisterFxProcessor(NewFxAdapter(fxSvc))
	worker.RegisterReferralProcessor(NewReferralAdapter(refSvc, database))
	worker.RegisterAdsSelfServeProcessor(NewAdsSelfServeAdapter(adsSvc, database))
	worker.RegisterPushProcessor(NewPushAdapter(devSvc, database))

	slog.Info("v2 processors registered", "jobs", true, "gamification", true, "import", true, "assessment", true, "scholarship", true, "remittance", true, "fx", true, "referral", true, "ads_self_serve", true, "push", true, "persona", false, "studio", false)
}
