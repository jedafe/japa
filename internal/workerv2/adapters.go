// Package workerv2 bridges the v2 processor interfaces (defined in
// internal/worker/v2_processors.go) to the tenant-scoped v2 services. The
// processor interfaces are called by Asynq task handlers which receive a
// single entity ID (or no ID for periodic tasks). The services, however, are
// tenant-scoped — every method requires a tenantID. These adapters resolve
// the tenant context and delegate to the appropriate service method.
//
// This package lives separate from internal/worker to avoid an import cycle:
// worker → assessment → auth → worker. By placing the adapters here, worker
// never imports the service packages, and the services never import workerv2.
//
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
	"github.com/jedafe/japa/internal/gamification"
	"github.com/jedafe/japa/internal/jobs"
	"github.com/jedafe/japa/internal/notifications"
	"github.com/jedafe/japa/internal/referral"
	"github.com/jedafe/japa/internal/remittance"
	"github.com/jedafe/japa/internal/scholarships"
	"github.com/jedafe/japa/internal/worker"
)

// tenantRow is a minimal tenant row for cross-tenant iteration.
type tenantRow struct {
	ID pgtype.UUID
}

// listTenantIDs returns every active tenant ID in the system. Periodic v2
// tasks (expire listings, expire challenges, check deadlines, check alerts,
// prune old rates) iterate over this list so each tenant gets its own sweep.
// Suspended tenants are excluded — their background processing is paused
// until they are reactivated (migration 000112_tenant_status).
func listTenantIDs(ctx context.Context, database *db.DB) ([]pgtype.UUID, error) {
	rows, err := database.Reader.Query(ctx, "SELECT id FROM tenants WHERE status = 'active' ORDER BY created_at")
	if err != nil {
		return nil, fmt.Errorf("listing tenants: %w", err)
	}
	defer rows.Close()

	var ids []pgtype.UUID
	for rows.Next() {
		var id pgtype.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning tenant id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// lookupTenantIDByEntity resolves the tenant_id for a single entity row by
// table name and entity UUID. Used by entity-specific task adapters that
// receive only the entity ID in the Asynq payload.
func lookupTenantIDByEntity(ctx context.Context, database *db.DB, table, entityCol string, id pgtype.UUID) (pgtype.UUID, error) {
	query := fmt.Sprintf("SELECT tenant_id FROM %s WHERE %s = $1", table, entityCol)
	var tenantID pgtype.UUID
	err := database.Reader.QueryRow(ctx, query, id).Scan(&tenantID)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("looking up tenant_id from %s by %s: %w", table, entityCol, err)
	}
	return tenantID, nil
}

// ---------------------------------------------------------------------------
// JobsAdapter — wraps jobs.Service to satisfy JobsProcessor (§63, ADR-081).
// ---------------------------------------------------------------------------

// JobsAdapter adapts the tenant-scoped jobs.Service to the cross-tenant
// JobsProcessor interface expected by the Asynq task handlers.
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

// ExpireListingsPastDeadline iterates every tenant and expires job listings
// past their deadline. Returns the total count across all tenants.
func (a *JobsAdapter) ExpireListingsPastDeadline(ctx context.Context) (int, error) {
	tenantIDs, err := listTenantIDs(ctx, a.db)
	if err != nil {
		return 0, err
	}
	var total int
	for _, tid := range tenantIDs {
		n, err := a.svc.ExpireListingsPastDeadline(ctx, tid)
		if err != nil {
			slog.Warn("task:jobs:expire_listings: tenant sweep failed", "tenant_id", tid, "error", err)
			continue
		}
		total += int(n)
	}
	return total, nil
}

// NotifyApplicationStatus looks up the application's tenant and notifies the
// applicant of a status change. The status change already happened in the
// HTTP handler before the task was enqueued; this method only sends the
// notification (§63, ADR-081).
func (a *JobsAdapter) NotifyApplicationStatus(ctx context.Context, applicationID pgtype.UUID) error {
	tid, err := lookupTenantIDByEntity(ctx, a.db, "job_applications", "id", applicationID)
	if err != nil {
		return err
	}
	app, err := a.svc.GetApplicationForNotification(ctx, tid, applicationID)
	if err != nil {
		return fmt.Errorf("jobs: notify application status: fetch application: %w", err)
	}
	if a.notif != nil {
		if err := a.notif.CreateNotification(ctx, tid, app.ApplicantID,
			"job_application_status_update", "system", pgtype.UUID{},
			"job_application", applicationID,
			"Job Application Update",
			fmt.Sprintf("Your application status is now: %s", app.Status),
			""); err != nil {
			slog.Warn("jobs: notify application status: create notification failed",
				"application_id", applicationID.String(), "error", err)
		}
	}
	slog.Info("jobs: notify application status",
		"application_id", applicationID.String(),
		"applicant_id", app.ApplicantID.String(),
		"status", app.Status)
	return nil
}

// ---------------------------------------------------------------------------
// GamificationAdapter — wraps gamification.Service (§64, ADR-082).
// ---------------------------------------------------------------------------

// GamificationAdapter adapts the tenant-scoped gamification.Service to the
// cross-tenant GamificationProcessor interface.
type GamificationAdapter struct {
	svc *gamification.Service
	db  *db.DB
}

// NewGamificationAdapter creates a GamificationAdapter.
func NewGamificationAdapter(svc *gamification.Service, database *db.DB) *GamificationAdapter {
	return &GamificationAdapter{svc: svc, db: database}
}

// ExpireChallenges iterates every tenant and expires ended challenges.
func (a *GamificationAdapter) ExpireChallenges(ctx context.Context) (int, error) {
	tenantIDs, err := listTenantIDs(ctx, a.db)
	if err != nil {
		return 0, err
	}
	var total int
	for _, tid := range tenantIDs {
		n, err := a.svc.ExpireChallenges(ctx, tid)
		if err != nil {
			slog.Warn("task:gamification:expire_challenges: tenant sweep failed", "tenant_id", tid, "error", err)
			continue
		}
		total += int(n)
	}
	return total, nil
}

// ---------------------------------------------------------------------------
// ImportAdapter — wraps contentimport.Service (§65, ADR-083).
// ---------------------------------------------------------------------------

// ImportAdapter adapts the tenant-scoped contentimport.Service to the
// ImportProcessor interface. The Asynq payload carries only import_id; the
// adapter resolves tenant_id, source_type, and source_url from the database.
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
	TenantID   pgtype.UUID
	SourceType string
	SourceURL  string
}

func (a *ImportAdapter) lookupImport(ctx context.Context, id pgtype.UUID) (importRow, error) {
	var r importRow
	err := a.db.Reader.QueryRow(ctx,
		"SELECT tenant_id, source_type, source_url FROM content_imports WHERE id = $1", id,
	).Scan(&r.TenantID, &r.SourceType, &r.SourceURL)
	if err != nil {
		return importRow{}, fmt.Errorf("looking up content_import: %w", err)
	}
	return r, nil
}

// ProcessSource resolves the import's tenant and delegates to the service.
func (a *ImportAdapter) ProcessSource(ctx context.Context, importID pgtype.UUID) error {
	r, err := a.lookupImport(ctx, importID)
	if err != nil {
		return err
	}
	return a.svc.ProcessSource(ctx, r.TenantID, importID, r.SourceType, r.SourceURL)
}

// FetchMedia delegates to the contentimport service's FetchMedia method,
// which downloads external images from the imported content's body and
// replaces their URLs with stored media URLs (Section 34).
func (a *ImportAdapter) FetchMedia(ctx context.Context, tenantID, userID, contentID pgtype.UUID) error {
	return a.svc.FetchMedia(ctx, tenantID, userID, contentID)
}

// ---------------------------------------------------------------------------
// AssessmentAdapter — wraps assessment.Service (§71, ADR-071).
// ---------------------------------------------------------------------------

// AssessmentAdapter adapts the tenant-scoped assessment.Service to the
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

// ScoreSubmission resolves the submission's tenant, user, and field_values
// from form_submissions, then runs the scoring engine which writes the
// assessment_results row. The result does not exist yet at enqueue time —
// scoring creates it (§71). After scoring, an assessment_complete
// notification is dispatched to the authenticated user (ADR-071 §2 step 7).
func (a *AssessmentAdapter) ScoreSubmission(ctx context.Context, submissionID, tenantID pgtype.UUID) error {
	var userID pgtype.UUID
	var fieldValues []byte
	err := a.db.Reader.QueryRow(ctx,
		"SELECT user_id, field_values FROM form_submissions WHERE id = $1 AND tenant_id = $2",
		submissionID, tenantID,
	).Scan(&userID, &fieldValues)
	if err != nil {
		return fmt.Errorf("looking up form_submission: %w", err)
	}
	resultID, err := a.svc.ScoreSubmission(ctx, tenantID, assessment.ScoreSubmissionInput{
		SubmissionID: submissionID,
		UserID:       userID,
		FieldValues:  fieldValues,
	})
	if err != nil {
		return err
	}

	// ADR-071 §2 step 7: notify the user that their assessment result is
	// ready. Anonymous users (userID invalid) are skipped — they see the
	// result in-session via polling.
	if userID.Valid && a.notif != nil {
		actionURL := fmt.Sprintf("/tools/readiness?result=%s", resultID.String())
		if err := a.notif.CreateNotification(
			ctx,
			tenantID,
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

// ---------------------------------------------------------------------------
// ScholarshipAdapter — wraps scholarships.Service (§73, ADR-073).
// ---------------------------------------------------------------------------

// ScholarshipAdapter adapts the tenant-scoped scholarships.Service to the
// cross-tenant ScholarshipProcessor interface.
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

// CheckDeadlines iterates every tenant, finds saved scholarships whose
// application deadline is within the user's remind_before_days window,
// dispatches a notification per match (ADR-073 §3 step 3), and marks
// the reminder as sent.
func (a *ScholarshipAdapter) CheckDeadlines(ctx context.Context) (int, error) {
	tenantIDs, err := listTenantIDs(ctx, a.db)
	if err != nil {
		return 0, err
	}
	var total int
	for _, tid := range tenantIDs {
		reminders, err := a.svc.FindNeedingReminders(ctx, tid, 100)
		if err != nil {
			slog.Warn("task:scholarship:check_deadlines: tenant sweep failed", "tenant_id", tid, "error", err)
			continue
		}
		for _, r := range reminders {
			if err := a.dispatchReminder(ctx, tid, r); err != nil {
				slog.Warn("task:scholarship:check_deadlines: dispatch reminder failed", "reminder_id", r.ID, "error", err)
				continue
			}
			if err := a.svc.MarkReminderSent(ctx, r.ID); err != nil {
				slog.Warn("task:scholarship:check_deadlines: mark reminder sent failed", "reminder_id", r.ID, "error", err)
				continue
			}
			total++
		}
	}
	return total, nil
}

// dispatchReminder creates an in-app notification for the saved-scholarship
// owner announcing the approaching application deadline. The notification
// type `scholarship.deadline_reminder` is created via the notifications
// service's non-batch path (§35, ADR-073 §3).
func (a *ScholarshipAdapter) dispatchReminder(ctx context.Context, tenantID pgtype.UUID, r gen.FindScholarshipsNeedingRemindersRow) error {
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
		tenantID,
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

// RemittanceAdapter adapts the tenant-scoped remittance.Service to the
// RemittanceProcessor interface.
type RemittanceAdapter struct {
	svc *remittance.Service
	db  *db.DB
}

// NewRemittanceAdapter creates a RemittanceAdapter.
func NewRemittanceAdapter(svc *remittance.Service, database *db.DB) *RemittanceAdapter {
	return &RemittanceAdapter{svc: svc, db: database}
}

// FetchRates resolves the provider's tenant and performs the actual rate
// fetch by calling the provider's API adapter, inserting rate rows, and
// invalidating the corridor cache (ADR-075 §2). This is the task-handler
// entry point — it does NOT re-enqueue the fetch task (which would create
// an infinite loop). The admin manual trigger calls TriggerFetch (which
// enqueues this task), and the scheduler enqueues it on the configured
// interval.
func (a *RemittanceAdapter) FetchRates(ctx context.Context, providerID pgtype.UUID) error {
	tid, err := lookupTenantIDByEntity(ctx, a.db, "remittance_providers", "id", providerID)
	if err != nil {
		return err
	}
	return a.svc.FetchProviderRates(ctx, tid, providerID)
}

// FetchAllRates iterates all active API-sourced providers across all tenants
// and fetches their rates. Called by the scheduler sweep (empty payload).
// Manual providers are skipped (rates are admin-entered). Returns the number
// of providers processed. Ref: ADR-075 §2, §8.
func (a *RemittanceAdapter) FetchAllRates(ctx context.Context) (int, error) {
	tenantIDs, err := listTenantIDs(ctx, a.db)
	if err != nil {
		return 0, err
	}
	var total int
	for _, tid := range tenantIDs {
		n, err := a.svc.FetchAllProviderRates(ctx, tid)
		if err != nil {
			slog.Warn("task:remittance:fetch_rates: tenant sweep failed",
				"tenant_id", tid, "error", err)
			continue
		}
		total += n
	}
	return total, nil
}

// CheckAlerts iterates every tenant, evaluates all active rate alerts
// against the current best rate for each corridor, dispatches notifications
// for alerts whose thresholds are met, and marks them triggered (ADR-075 §4).
// The 24-hour debounce is enforced by the SQL query (last_triggered_at is
// null or > 24h ago).
func (a *RemittanceAdapter) CheckAlerts(ctx context.Context) (int, error) {
	tenantIDs, err := listTenantIDs(ctx, a.db)
	if err != nil {
		return 0, err
	}
	var total int
	for _, tid := range tenantIDs {
		n, err := a.svc.CheckAndDispatchAlerts(ctx, tid)
		if err != nil {
			slog.Warn("task:remittance:check_alerts: tenant check failed", "tenant_id", tid, "error", err)
			continue
		}
		total += n
	}
	return total, nil
}

// PruneOldRates iterates every tenant and prunes rates beyond the configured
// retention period (remittance.history_retention_days, default 365). The
// SQL keeps one snapshot per provider per corridor per day for older data
// (daily downsampling). Ref: ADR-075 §7, Rule 3 (no hardcoded config).
func (a *RemittanceAdapter) PruneOldRates(ctx context.Context) (int, error) {
	tenantIDs, err := listTenantIDs(ctx, a.db)
	if err != nil {
		return 0, err
	}
	var total int
	for _, tid := range tenantIDs {
		// Read the per-tenant retention setting (Rule 3 — never hardcode).
		retentionDays := a.svc.HistoryRetentionDays(ctx, tid)
		cutoff := time.Now().AddDate(0, 0, -retentionDays)
		if err := a.svc.PruneOldRates(ctx, tid, cutoff); err != nil {
			slog.Warn("task:remittance:prune_old_rates: tenant sweep failed", "tenant_id", tid, "error", err)
			continue
		}
		total++
	}
	return total, nil
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
func (a *ReferralAdapter) CheckConversion(ctx context.Context, tenantID, payerID, paymentID pgtype.UUID, sessionID string) error {
	return a.svc.CheckConversion(ctx, tenantID, payerID, paymentID, sessionID)
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

// ---------------------------------------------------------------------------
// AdsSelfServeAdapter — wraps adsselfserve.Service (§61, ADR-079).
// ---------------------------------------------------------------------------

// AdsSelfServeAdapter adapts the tenant-scoped adsselfserve.Service to the
// AdsSelfServeProcessor interface.
type AdsSelfServeAdapter struct {
	svc *adsselfserve.Service
	db  *db.DB
}

// NewAdsSelfServeAdapter creates an AdsSelfServeAdapter.
func NewAdsSelfServeAdapter(svc *adsselfserve.Service, database *db.DB) *AdsSelfServeAdapter {
	return &AdsSelfServeAdapter{svc: svc, db: database}
}

// RefundCampaignCredits resolves the campaign's tenant and refunds credits.
func (a *AdsSelfServeAdapter) RefundCampaignCredits(ctx context.Context, campaignID pgtype.UUID) error {
	tid, err := lookupTenantIDByEntity(ctx, a.db, "ad_campaigns", "id", campaignID)
	if err != nil {
		return err
	}
	return a.svc.RefundCampaignCredits(ctx, tid, campaignID)
}

// NotifyCampaignStatus resolves the campaign's tenant and notifies the
// advertiser of the review decision (approved/rejected).
// Ref: japaconnect-engineering/sections/61-self-serve-ads.md (Asynq Tasks)
func (a *AdsSelfServeAdapter) NotifyCampaignStatus(ctx context.Context, campaignID pgtype.UUID) error {
	tid, err := lookupTenantIDByEntity(ctx, a.db, "ad_campaigns", "id", campaignID)
	if err != nil {
		return err
	}
	return a.svc.NotifyCampaignStatus(ctx, tid, campaignID)
}

// ---------------------------------------------------------------------------
// PushAdapter — wraps devices.Service (§77, ADR-074).
// ---------------------------------------------------------------------------

// PushAdapter adapts the tenant-scoped devices.Service to the PushProcessor
// interface. The Asynq payload carries notification_id + device_token_id;
// the adapter resolves the user and tenant from the device token, then
// delegates to the service's SendPush.
type PushAdapter struct {
	svc *devices.Service
	db  *db.DB
}

// NewPushAdapter creates a PushAdapter.
func NewPushAdapter(svc *devices.Service, database *db.DB) *PushAdapter {
	return &PushAdapter{svc: svc, db: database}
}

// SendPush resolves the device token's tenant and user, then sends the push
// notification. The notification payload is loaded from the notifications table.
func (a *PushAdapter) SendPush(ctx context.Context, notificationID, deviceTokenID pgtype.UUID) error {
	var tenantID, userID pgtype.UUID
	err := a.db.Reader.QueryRow(ctx,
		"SELECT tenant_id, user_id FROM device_tokens WHERE id = $1", deviceTokenID,
	).Scan(&tenantID, &userID)
	if err != nil {
		return fmt.Errorf("looking up device token: %w", err)
	}

	// Load the notification to build the push payload.
	var title, body string
	err = a.db.Reader.QueryRow(ctx,
		"SELECT title, body FROM notifications WHERE id = $1 AND tenant_id = $2",
		notificationID, tenantID,
	).Scan(&title, &body)
	if err != nil {
		return fmt.Errorf("looking up notification: %w", err)
	}

	return a.svc.SendPush(ctx, tenantID, userID, devices.PushPayload{
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
	worker.RegisterReferralProcessor(NewReferralAdapter(refSvc, database))
	worker.RegisterAdsSelfServeProcessor(NewAdsSelfServeAdapter(adsSvc, database))
	worker.RegisterPushProcessor(NewPushAdapter(devSvc, database))

	slog.Info("v2 processors registered",
		"jobs", true, "gamification", true, "import", true,
		"assessment", true, "scholarship", true, "remittance", true,
		"referral", true, "ads_self_serve", true, "push", true,
		"persona", false, "studio", false,
	)
}
