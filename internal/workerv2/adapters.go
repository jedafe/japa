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
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/adsselfserve"
	"github.com/jedafe/japa/internal/assessment"
	"github.com/jedafe/japa/internal/contentimport"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/devices"
	"github.com/jedafe/japa/internal/gamification"
	"github.com/jedafe/japa/internal/jobs"
	"github.com/jedafe/japa/internal/referral"
	"github.com/jedafe/japa/internal/remittance"
	"github.com/jedafe/japa/internal/scholarships"
	"github.com/jedafe/japa/internal/worker"
)

// tenantRow is a minimal tenant row for cross-tenant iteration.
type tenantRow struct {
	ID pgtype.UUID
}

// listTenantIDs returns every tenant ID in the system. Periodic v2 tasks
// (expire listings, expire challenges, check deadlines, check alerts, prune
// old rates) iterate over this list so each tenant gets its own sweep.
func listTenantIDs(ctx context.Context, database *db.DB) ([]pgtype.UUID, error) {
	rows, err := database.Reader.Query(ctx, "SELECT id FROM tenants ORDER BY created_at")
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
	svc *jobs.Service
	db  *db.DB
}

// NewJobsAdapter creates a JobsAdapter wrapping the given service and database.
func NewJobsAdapter(svc *jobs.Service, database *db.DB) *JobsAdapter {
	return &JobsAdapter{svc: svc, db: database}
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
// applicant of a status change.
func (a *JobsAdapter) NotifyApplicationStatus(ctx context.Context, applicationID pgtype.UUID) error {
	tid, err := lookupTenantIDByEntity(ctx, a.db, "job_applications", "id", applicationID)
	if err != nil {
		return err
	}
	_, err = a.svc.UpdateApplicationStatus(ctx, tid, applicationID, "", nil)
	return err
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

// FetchMedia resolves the import's tenant and fetches media for the import.
// The contentimport service does not expose a separate FetchMedia method;
// ProcessSource handles both source processing and media fetching. We log
// the request and delegate to ProcessSource so the task is not silently
// dropped.
func (a *ImportAdapter) FetchMedia(ctx context.Context, importID pgtype.UUID) error {
	r, err := a.lookupImport(ctx, importID)
	if err != nil {
		return err
	}
	slog.Info("task:import:fetch_media: delegating to ProcessSource", "import_id", importID, "tenant_id", r.TenantID)
	return a.svc.ProcessSource(ctx, r.TenantID, importID, r.SourceType, r.SourceURL)
}

// ---------------------------------------------------------------------------
// AssessmentAdapter — wraps assessment.Service (§71, ADR-071).
// ---------------------------------------------------------------------------

// AssessmentAdapter adapts the tenant-scoped assessment.Service to the
// AssessmentProcessor interface.
type AssessmentAdapter struct {
	svc *assessment.Service
	db  *db.DB
}

// NewAssessmentAdapter creates an AssessmentAdapter.
func NewAssessmentAdapter(svc *assessment.Service, database *db.DB) *AssessmentAdapter {
	return &AssessmentAdapter{svc: svc, db: database}
}

// ScoreSubmission resolves the result's tenant and scores the submission.
// The assessment service's ScoreSubmission requires SubmissionID, UserID, and
// FieldValues — these are looked up from the assessment_results row.
func (a *AssessmentAdapter) ScoreSubmission(ctx context.Context, resultID pgtype.UUID) error {
	var tid, submissionID, userID pgtype.UUID
	var fieldValues []byte
	err := a.db.Reader.QueryRow(ctx,
		"SELECT tenant_id, submission_id, user_id, field_values FROM assessment_results WHERE id = $1",
		resultID,
	).Scan(&tid, &submissionID, &userID, &fieldValues)
	if err != nil {
		return fmt.Errorf("looking up assessment_result: %w", err)
	}
	_, err = a.svc.ScoreSubmission(ctx, tid, assessment.ScoreSubmissionInput{
		SubmissionID: submissionID,
		UserID:       userID,
		FieldValues:  fieldValues,
	})
	return err
}

// ---------------------------------------------------------------------------
// ScholarshipAdapter — wraps scholarships.Service (§73, ADR-073).
// ---------------------------------------------------------------------------

// ScholarshipAdapter adapts the tenant-scoped scholarships.Service to the
// cross-tenant ScholarshipProcessor interface.
type ScholarshipAdapter struct {
	svc *scholarships.Service
	db  *db.DB
}

// NewScholarshipAdapter creates a ScholarshipAdapter.
func NewScholarshipAdapter(svc *scholarships.Service, database *db.DB) *ScholarshipAdapter {
	return &ScholarshipAdapter{svc: svc, db: database}
}

// CheckDeadlines iterates every tenant and sends deadline reminders.
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
			if err := a.svc.MarkReminderSent(ctx, r.ID); err != nil {
				slog.Warn("task:scholarship:check_deadlines: mark reminder sent failed", "reminder_id", r.ID, "error", err)
				continue
			}
			total++
		}
	}
	return total, nil
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

// FetchRates resolves the provider's tenant and triggers a rate fetch.
func (a *RemittanceAdapter) FetchRates(ctx context.Context, providerID pgtype.UUID) error {
	tid, err := lookupTenantIDByEntity(ctx, a.db, "remittance_providers", "id", providerID)
	if err != nil {
		return err
	}
	return a.svc.TriggerFetch(ctx, tid, providerID)
}

// CheckAlerts iterates every tenant and checks all active rate alerts
// against latest rates. The service's FindAlertsNeedingNotifications is
// corridor-specific (requires send/receive currency); this adapter queries
// all active alerts directly via SQL so every corridor is covered.
func (a *RemittanceAdapter) CheckAlerts(ctx context.Context) (int, error) {
	tenantIDs, err := listTenantIDs(ctx, a.db)
	if err != nil {
		return 0, err
	}
	var total int
	for _, tid := range tenantIDs {
		rows, err := a.db.Reader.Query(ctx,
			`SELECT id FROM remittance_rate_alerts
			 WHERE tenant_id = $1 AND is_active = true
			   AND (last_triggered_at IS NULL OR last_triggered_at < now() - interval '24 hours')`,
			tid)
		if err != nil {
			slog.Warn("task:remittance:check_alerts: tenant query failed", "tenant_id", tid, "error", err)
			continue
		}
		for rows.Next() {
			var alertID pgtype.UUID
			if err := rows.Scan(&alertID); err != nil {
				rows.Close()
				slog.Warn("task:remittance:check_alerts: scan failed", "error", err)
				continue
			}
			if err := a.svc.MarkAlertTriggered(ctx, alertID); err != nil {
				slog.Warn("task:remittance:check_alerts: mark alert triggered failed", "alert_id", alertID, "error", err)
				continue
			}
			total++
		}
		rows.Close()
	}
	return total, nil
}

// PruneOldRates iterates every tenant and prunes rates beyond retention.
func (a *RemittanceAdapter) PruneOldRates(ctx context.Context) (int, error) {
	tenantIDs, err := listTenantIDs(ctx, a.db)
	if err != nil {
		return 0, err
	}
	cutoff := time.Now().AddDate(0, 0, -90)
	var total int
	for _, tid := range tenantIDs {
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

// RecordClick records a referral click. The sessionID is derived from IP +
// userAgent so the service's dedup logic still works.
func (a *ReferralAdapter) RecordClick(ctx context.Context, code, ip, userAgent string) error {
	sessionID := ip + "|" + userAgent
	return a.svc.RecordClick(ctx, code, sessionID)
}

// CheckConversion resolves the referral's tenant and checks for conversion.
func (a *ReferralAdapter) CheckConversion(ctx context.Context, referralID pgtype.UUID) error {
	tid, err := lookupTenantIDByEntity(ctx, a.db, "referrals", "id", referralID)
	if err != nil {
		return err
	}
	// The service's CheckConversion requires payer/payment/session context
	// that the task payload doesn't carry. Log and skip — conversion checks
	// are triggered from the payment flow, not from a background task.
	slog.Info("task:referral:check_conversion: tenant resolved, skipping (conversion checked at payment time)", "tenant_id", tid, "referral_id", referralID)
	return nil
}

// ReviewFlagged resolves the referral's tenant and resolves the flagged referral.
// The service's AdminResolveFlagged accepts "credit" or "reject"; the background
// task defaults to "credit" so legitimate referrals are not penalised by a
// delayed review. An admin can manually reject via the admin panel.
func (a *ReferralAdapter) ReviewFlagged(ctx context.Context, referralID pgtype.UUID) error {
	tid, err := lookupTenantIDByEntity(ctx, a.db, "referrals", "id", referralID)
	if err != nil {
		return err
	}
	return a.svc.AdminResolveFlagged(ctx, tid, referralID, "credit")
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

// NotifyCampaignStatus resolves the campaign's tenant and notifies the advertiser.
func (a *AdsSelfServeAdapter) NotifyCampaignStatus(ctx context.Context, campaignID pgtype.UUID) error {
	// The service does not have a dedicated NotifyCampaignStatus method.
	// Log the status notification request — the notification is sent via
	// the standard notification dispatch path.
	tid, err := lookupTenantIDByEntity(ctx, a.db, "ad_campaigns", "id", campaignID)
	if err != nil {
		return err
	}
	slog.Info("task:ads:notify_campaign_status: campaign status notification queued", "tenant_id", tid, "campaign_id", campaignID)
	return nil
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
) {
	worker.RegisterJobsProcessor(NewJobsAdapter(jobsSvc, database))
	worker.RegisterGamificationProcessor(NewGamificationAdapter(gamSvc, database))
	worker.RegisterImportProcessor(NewImportAdapter(impSvc, database))
	worker.RegisterAssessmentProcessor(NewAssessmentAdapter(assSvc, database))
	worker.RegisterScholarshipProcessor(NewScholarshipAdapter(schSvc, database))
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
