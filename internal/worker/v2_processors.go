// Package worker — v2_processors.go defines the processor interfaces and
// Asynq task handlers for the V2 feature program (Sections 60–76, ADRs
// 068, 070–085). Each V2 service implements the relevant interface and
// registers itself at startup via the Register* functions, mirroring the
// AIProcessor / RAGIndexer / ContentEmbedder patterns used by V1 tasks.
//
// Registered handlers:
//   - jobs:expire_listings, jobs:notify_application_status  (§63, ADR-081)
//   - gamification:expire_challenges                         (§64, ADR-082)
//   - import:process_source, import:fetch_media              (§65, ADR-083)
//   - assessment:score_submission                            (§71, ADR-071)
//   - scholarship:check_deadlines                            (§73, ADR-073)
//   - remittance:fetch_rates, remittance:check_alerts,
//     remittance:prune_old_rates                             (§74, ADR-075)
//   - referral:record_click, referral:check_conversion,
//     referral:review_flagged                                (§60, ADR-078)
//   - ads:refund_campaign_credits, ads:notify_campaign_status(§61, ADR-079)
//   - notification:send_push                                 (§77, ADR-074)
//   - persona:run_activity, persona:dispatch_action          (§76, ADR-077)
//   - studio:run_batch, studio:retry_failed                  (§75, ADR-076)
//
// Ref: japaconnect-engineering/sections/11-job-queue-asynq.md (catalog)
package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// Processor interfaces — each V2 service implements the subset it needs.
// ---------------------------------------------------------------------------

// JobsProcessor handles jobs-board background tasks (Section 63, ADR-081).
type JobsProcessor interface {
	ExpireListingsPastDeadline(ctx context.Context) (int, error)
	NotifyApplicationStatus(ctx context.Context, applicationID pgtype.UUID) error
}

// GamificationProcessor handles gamification background tasks (§64, ADR-082).
type GamificationProcessor interface {
	ExpireChallenges(ctx context.Context) (int, error)
}

// ImportProcessor handles content-import background tasks (§65, ADR-083).
type ImportProcessor interface {
	ProcessSource(ctx context.Context, importID pgtype.UUID) error
	FetchMedia(ctx context.Context, importID pgtype.UUID) error
}

// AssessmentProcessor handles assessment background tasks (§71, ADR-071).
type AssessmentProcessor interface {
	ScoreSubmission(ctx context.Context, submissionID, tenantID pgtype.UUID) error
}

// ScholarshipProcessor handles scholarship background tasks (§73, ADR-073).
type ScholarshipProcessor interface {
	CheckDeadlines(ctx context.Context) (int, error)
}

// RemittanceProcessor handles remittance background tasks (§74, ADR-075).
type RemittanceProcessor interface {
	FetchRates(ctx context.Context, providerID pgtype.UUID) error
	CheckAlerts(ctx context.Context) (int, error)
	PruneOldRates(ctx context.Context) (int, error)
}

// ReferralProcessor handles referral background tasks (§60, ADR-078).
type ReferralProcessor interface {
	RecordClick(ctx context.Context, code, ip, userAgent string) error
	CheckConversion(ctx context.Context, referralID pgtype.UUID) error
	ReviewFlagged(ctx context.Context, referralID pgtype.UUID) error
}

// AdsSelfServeProcessor handles self-serve ads background tasks (§61, ADR-079).
type AdsSelfServeProcessor interface {
	RefundCampaignCredits(ctx context.Context, campaignID pgtype.UUID) error
	NotifyCampaignStatus(ctx context.Context, campaignID pgtype.UUID) error
}

// PushProcessor handles mobile push delivery (§77, ADR-074).
type PushProcessor interface {
	SendPush(ctx context.Context, notificationID, deviceTokenID pgtype.UUID) error
}

// PersonaProcessor handles participation orchestration tasks (§76, ADR-077).
type PersonaProcessor interface {
	RunActivity(ctx context.Context, personaID pgtype.UUID) error
	DispatchAction(ctx context.Context, actionID pgtype.UUID) error
	SweepActivePersonas(ctx context.Context) error
}

// StudioProcessor handles AI Content Studio tasks (§75, ADR-076).
type StudioProcessor interface {
	RunBatch(ctx context.Context, batchID pgtype.UUID) error
	RetryFailed(ctx context.Context, jobID pgtype.UUID) error
}

// ---------------------------------------------------------------------------
// Global processor slots — nil-safe (graceful degradation when unregistered).
// ---------------------------------------------------------------------------

var (
	jobsProcessor          JobsProcessor
	gamificationProcessor  GamificationProcessor
	importProcessor        ImportProcessor
	assessmentProcessor    AssessmentProcessor
	scholarshipProcessor   ScholarshipProcessor
	remittanceProcessor    RemittanceProcessor
	referralProcessor      ReferralProcessor
	adsSelfServeProcessor  AdsSelfServeProcessor
	pushProcessor          PushProcessor
	personaProcessor       PersonaProcessor
	studioProcessor        StudioProcessor
)

// Register functions — call once at startup before the worker serves.

func RegisterJobsProcessor(p JobsProcessor)             { jobsProcessor = p }
func RegisterGamificationProcessor(p GamificationProcessor) { gamificationProcessor = p }
func RegisterImportProcessor(p ImportProcessor)         { importProcessor = p }
func RegisterAssessmentProcessor(p AssessmentProcessor) { assessmentProcessor = p }
func RegisterScholarshipProcessor(p ScholarshipProcessor) { scholarshipProcessor = p }
func RegisterRemittanceProcessor(p RemittanceProcessor) { remittanceProcessor = p }
func RegisterReferralProcessor(p ReferralProcessor)     { referralProcessor = p }
func RegisterAdsSelfServeProcessor(p AdsSelfServeProcessor) { adsSelfServeProcessor = p }
func RegisterPushProcessor(p PushProcessor)             { pushProcessor = p }
func RegisterPersonaProcessor(p PersonaProcessor)       { personaProcessor = p }
func RegisterStudioProcessor(p StudioProcessor)         { studioProcessor = p }

// ---------------------------------------------------------------------------
// Task type constants (V2 — Sections 60–77).
// ---------------------------------------------------------------------------

const (
	TypeJobsExpireListings          = "jobs:expire_listings"
	TypeJobsNotifyApplicationStatus = "jobs:notify_application_status"
	TypeGamificationExpireChallenges = "gamification:expire_challenges"
	TypeImportProcessSource         = "import:process_source"
	TypeImportFetchMedia            = "import:fetch_media"
	TypeAssessmentScoreSubmission   = "assessment:score_submission"
	TypeScholarshipCheckDeadlines   = "scholarship:check_deadlines"
	TypeRemittanceFetchRates        = "remittance:fetch_rates"
	TypeRemittanceCheckAlerts       = "remittance:check_alerts"
	TypeRemittancePruneOldRates     = "remittance:prune_old_rates"
	TypeReferralRecordClick         = "referral:record_click"
	TypeReferralCheckConversion     = "referral:check_conversion"
	TypeReferralReviewFlagged       = "referral:review_flagged"
	TypeAdsRefundCampaignCredits    = "ads:refund_campaign_credits"
	TypeAdsNotifyCampaignStatus     = "ads:notify_campaign_status"
	TypeNotificationSendPush        = "notification:send_push"
	TypePersonaRunActivity          = "persona:run_activity"
	TypePersonaDispatchAction       = "persona:dispatch_action"
	TypeStudioRunBatch              = "studio:run_batch"
	TypeStudioRetryFailed           = "studio:retry_failed"
)

// ---------------------------------------------------------------------------
// Asynq handler functions.
// ---------------------------------------------------------------------------

// HandleJobsExpireListings expires job listings past their deadline (§63).
func HandleJobsExpireListings(ctx context.Context, _ *asynq.Task) error {
	if jobsProcessor == nil {
		slog.Warn("task:jobs:expire_listings: no processor registered, skipping")
		return nil
	}
	n, err := jobsProcessor.ExpireListingsPastDeadline(ctx)
	if err != nil {
		return fmt.Errorf("task:jobs:expire_listings: %w", err)
	}
	slog.Info("task:jobs:expire_listings: expired listings", "count", n)
	return nil
}

// HandleJobsNotifyApplicationStatus notifies an applicant of a status change (§63).
func HandleJobsNotifyApplicationStatus(ctx context.Context, t *asynq.Task) error {
	if jobsProcessor == nil {
		slog.Warn("task:jobs:notify_application_status: no processor registered, skipping")
		return nil
	}
	var p struct{ ApplicationID string `json:"application_id"` }
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("task:jobs:notify_application_status: invalid payload: %w", err)
	}
	var id pgtype.UUID
	if err := id.Scan(p.ApplicationID); err != nil {
		return fmt.Errorf("task:jobs:notify_application_status: bad application_id: %w", err)
	}
	if err := jobsProcessor.NotifyApplicationStatus(ctx, id); err != nil {
		return fmt.Errorf("task:jobs:notify_application_status: %w", err)
	}
	return nil
}

// HandleGamificationExpireChallenges expires ended challenges (§64).
func HandleGamificationExpireChallenges(ctx context.Context, _ *asynq.Task) error {
	if gamificationProcessor == nil {
		slog.Warn("task:gamification:expire_challenges: no processor registered, skipping")
		return nil
	}
	n, err := gamificationProcessor.ExpireChallenges(ctx)
	if err != nil {
		return fmt.Errorf("task:gamification:expire_challenges: %w", err)
	}
	slog.Info("task:gamification:expire_challenges: expired challenges", "count", n)
	return nil
}

// HandleImportProcessSource processes a content import source (§65).
func HandleImportProcessSource(ctx context.Context, t *asynq.Task) error {
	if importProcessor == nil {
		slog.Warn("task:import:process_source: no processor registered, skipping")
		return nil
	}
	var p struct{ ImportID string `json:"import_id"` }
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("task:import:process_source: invalid payload: %w", err)
	}
	var id pgtype.UUID
	if err := id.Scan(p.ImportID); err != nil {
		return fmt.Errorf("task:import:process_source: bad import_id: %w", err)
	}
	if err := importProcessor.ProcessSource(ctx, id); err != nil {
		return fmt.Errorf("task:import:process_source: %w", err)
	}
	return nil
}

// HandleImportFetchMedia fetches media for an import (§65).
func HandleImportFetchMedia(ctx context.Context, t *asynq.Task) error {
	if importProcessor == nil {
		slog.Warn("task:import:fetch_media: no processor registered, skipping")
		return nil
	}
	var p struct{ ImportID string `json:"import_id"` }
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("task:import:fetch_media: invalid payload: %w", err)
	}
	var id pgtype.UUID
	if err := id.Scan(p.ImportID); err != nil {
		return fmt.Errorf("task:import:fetch_media: bad import_id: %w", err)
	}
	if err := importProcessor.FetchMedia(ctx, id); err != nil {
		return fmt.Errorf("task:import:fetch_media: %w", err)
	}
	return nil
}

// HandleAssessmentScoreSubmission scores an assessment submission (§71).
// The payload is enqueued by assessment.Service.SubmitAssessment and carries
// the submission_id + tenant_id (the result row does not exist yet — scoring
// creates it).
func HandleAssessmentScoreSubmission(ctx context.Context, t *asynq.Task) error {
	if assessmentProcessor == nil {
		slog.Warn("task:assessment:score_submission: no processor registered, skipping")
		return nil
	}
	var p struct {
		SubmissionID string `json:"submission_id"`
		TenantID     string `json:"tenant_id"`
	}
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("task:assessment:score_submission: invalid payload: %w", err)
	}
	var submissionID, tenantID pgtype.UUID
	if err := submissionID.Scan(p.SubmissionID); err != nil {
		return fmt.Errorf("task:assessment:score_submission: bad submission_id: %w", err)
	}
	if err := tenantID.Scan(p.TenantID); err != nil {
		return fmt.Errorf("task:assessment:score_submission: bad tenant_id: %w", err)
	}
	if err := assessmentProcessor.ScoreSubmission(ctx, submissionID, tenantID); err != nil {
		return fmt.Errorf("task:assessment:score_submission: %w", err)
	}
	return nil
}

// HandleScholarshipCheckDeadlines sends deadline reminders (§73).
func HandleScholarshipCheckDeadlines(ctx context.Context, _ *asynq.Task) error {
	if scholarshipProcessor == nil {
		slog.Warn("task:scholarship:check_deadlines: no processor registered, skipping")
		return nil
	}
	n, err := scholarshipProcessor.CheckDeadlines(ctx)
	if err != nil {
		return fmt.Errorf("task:scholarship:check_deadlines: %w", err)
	}
	slog.Info("task:scholarship:check_deadlines: reminders sent", "count", n)
	return nil
}

// HandleRemittanceFetchRates fetches live rates for a provider (§74).
func HandleRemittanceFetchRates(ctx context.Context, t *asynq.Task) error {
	if remittanceProcessor == nil {
		slog.Warn("task:remittance:fetch_rates: no processor registered, skipping")
		return nil
	}
	var p struct{ ProviderID string `json:"provider_id"` }
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("task:remittance:fetch_rates: invalid payload: %w", err)
	}
	var id pgtype.UUID
	if err := id.Scan(p.ProviderID); err != nil {
		return fmt.Errorf("task:remittance:fetch_rates: bad provider_id: %w", err)
	}
	if err := remittanceProcessor.FetchRates(ctx, id); err != nil {
		return fmt.Errorf("task:remittance:fetch_rates: %w", err)
	}
	return nil
}

// HandleRemittanceCheckAlerts checks rate alerts against latest rates (§74).
func HandleRemittanceCheckAlerts(ctx context.Context, _ *asynq.Task) error {
	if remittanceProcessor == nil {
		slog.Warn("task:remittance:check_alerts: no processor registered, skipping")
		return nil
	}
	n, err := remittanceProcessor.CheckAlerts(ctx)
	if err != nil {
		return fmt.Errorf("task:remittance:check_alerts: %w", err)
	}
	slog.Info("task:remittance:check_alerts: alerts triggered", "count", n)
	return nil
}

// HandleRemittancePruneOldRates removes rates beyond retention (§74).
func HandleRemittancePruneOldRates(ctx context.Context, _ *asynq.Task) error {
	if remittanceProcessor == nil {
		slog.Warn("task:remittance:prune_old_rates: no processor registered, skipping")
		return nil
	}
	n, err := remittanceProcessor.PruneOldRates(ctx)
	if err != nil {
		return fmt.Errorf("task:remittance:prune_old_rates: %w", err)
	}
	slog.Info("task:remittance:prune_old_rates: pruned rates", "count", n)
	return nil
}

// HandleReferralRecordClick records a referral click (§60).
func HandleReferralRecordClick(ctx context.Context, t *asynq.Task) error {
	if referralProcessor == nil {
		slog.Warn("task:referral:record_click: no processor registered, skipping")
		return nil
	}
	var p struct {
		Code      string `json:"code"`
		IP        string `json:"ip"`
		UserAgent string `json:"user_agent"`
	}
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("task:referral:record_click: invalid payload: %w", err)
	}
	if err := referralProcessor.RecordClick(ctx, p.Code, p.IP, p.UserAgent); err != nil {
		return fmt.Errorf("task:referral:record_click: %w", err)
	}
	return nil
}

// HandleReferralCheckConversion checks if a referral has converted (§60).
func HandleReferralCheckConversion(ctx context.Context, t *asynq.Task) error {
	if referralProcessor == nil {
		slog.Warn("task:referral:check_conversion: no processor registered, skipping")
		return nil
	}
	var p struct{ ReferralID string `json:"referral_id"` }
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("task:referral:check_conversion: invalid payload: %w", err)
	}
	var id pgtype.UUID
	if err := id.Scan(p.ReferralID); err != nil {
		return fmt.Errorf("task:referral:check_conversion: bad referral_id: %w", err)
	}
	if err := referralProcessor.CheckConversion(ctx, id); err != nil {
		return fmt.Errorf("task:referral:check_conversion: %w", err)
	}
	return nil
}

// HandleReferralReviewFlagged reviews a fraud-flagged referral (§60).
func HandleReferralReviewFlagged(ctx context.Context, t *asynq.Task) error {
	if referralProcessor == nil {
		slog.Warn("task:referral:review_flagged: no processor registered, skipping")
		return nil
	}
	var p struct{ ReferralID string `json:"referral_id"` }
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("task:referral:review_flagged: invalid payload: %w", err)
	}
	var id pgtype.UUID
	if err := id.Scan(p.ReferralID); err != nil {
		return fmt.Errorf("task:referral:review_flagged: bad referral_id: %w", err)
	}
	if err := referralProcessor.ReviewFlagged(ctx, id); err != nil {
		return fmt.Errorf("task:referral:review_flagged: %w", err)
	}
	return nil
}

// HandleAdsRefundCampaignCredits refunds credits for a rejected campaign (§61).
func HandleAdsRefundCampaignCredits(ctx context.Context, t *asynq.Task) error {
	if adsSelfServeProcessor == nil {
		slog.Warn("task:ads:refund_campaign_credits: no processor registered, skipping")
		return nil
	}
	var p struct{ CampaignID string `json:"campaign_id"` }
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("task:ads:refund_campaign_credits: invalid payload: %w", err)
	}
	var id pgtype.UUID
	if err := id.Scan(p.CampaignID); err != nil {
		return fmt.Errorf("task:ads:refund_campaign_credits: bad campaign_id: %w", err)
	}
	if err := adsSelfServeProcessor.RefundCampaignCredits(ctx, id); err != nil {
		return fmt.Errorf("task:ads:refund_campaign_credits: %w", err)
	}
	return nil
}

// HandleAdsNotifyCampaignStatus notifies an advertiser of a status change (§61).
func HandleAdsNotifyCampaignStatus(ctx context.Context, t *asynq.Task) error {
	if adsSelfServeProcessor == nil {
		slog.Warn("task:ads:notify_campaign_status: no processor registered, skipping")
		return nil
	}
	var p struct{ CampaignID string `json:"campaign_id"` }
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("task:ads:notify_campaign_status: invalid payload: %w", err)
	}
	var id pgtype.UUID
	if err := id.Scan(p.CampaignID); err != nil {
		return fmt.Errorf("task:ads:notify_campaign_status: bad campaign_id: %w", err)
	}
	if err := adsSelfServeProcessor.NotifyCampaignStatus(ctx, id); err != nil {
		return fmt.Errorf("task:ads:notify_campaign_status: %w", err)
	}
	return nil
}

// HandleNotificationSendPush dispatches a push notification to FCM/APNs (§77).
func HandleNotificationSendPush(ctx context.Context, t *asynq.Task) error {
	if pushProcessor == nil {
		slog.Warn("task:notification:send_push: no processor registered, skipping")
		return nil
	}
	var p struct {
		NotificationID string `json:"notification_id"`
		DeviceTokenID  string `json:"device_token_id"`
	}
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("task:notification:send_push: invalid payload: %w", err)
	}
	var notifID, deviceID pgtype.UUID
	if err := notifID.Scan(p.NotificationID); err != nil {
		return fmt.Errorf("task:notification:send_push: bad notification_id: %w", err)
	}
	if err := deviceID.Scan(p.DeviceTokenID); err != nil {
		return fmt.Errorf("task:notification:send_push: bad device_token_id: %w", err)
	}
	if err := pushProcessor.SendPush(ctx, notifID, deviceID); err != nil {
		return fmt.Errorf("task:notification:send_push: %w", err)
	}
	return nil
}

// HandlePersonaRunActivity runs a scheduled persona activity cycle (§76).
// When the payload has no persona_id, it runs in sweep mode: enqueuing
// run_activity tasks for all active personas. The sweep interval is
// controlled by participation.run_interval_minutes (system_settings).
func HandlePersonaRunActivity(ctx context.Context, t *asynq.Task) error {
	if personaProcessor == nil {
		slog.Warn("task:persona:run_activity: no processor registered, skipping")
		return nil
	}
	var p struct{ PersonaID string `json:"persona_id"` }
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("task:persona:run_activity: invalid payload: %w", err)
	}

	// Sweep mode: no persona_id — enqueue run_activity for all active personas.
	if p.PersonaID == "" {
		if err := personaProcessor.SweepActivePersonas(ctx); err != nil {
			return fmt.Errorf("task:persona:run_activity: sweep: %w", err)
		}
		return nil
	}

	var id pgtype.UUID
	if err := id.Scan(p.PersonaID); err != nil {
		return fmt.Errorf("task:persona:run_activity: bad persona_id: %w", err)
	}
	if err := personaProcessor.RunActivity(ctx, id); err != nil {
		return fmt.Errorf("task:persona:run_activity: %w", err)
	}
	return nil
}

// HandlePersonaDispatchAction dispatches a queued persona action (§76).
func HandlePersonaDispatchAction(ctx context.Context, t *asynq.Task) error {
	if personaProcessor == nil {
		slog.Warn("task:persona:dispatch_action: no processor registered, skipping")
		return nil
	}
	var p struct{ ActionID string `json:"action_id"` }
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("task:persona:dispatch_action: invalid payload: %w", err)
	}
	var id pgtype.UUID
	if err := id.Scan(p.ActionID); err != nil {
		return fmt.Errorf("task:persona:dispatch_action: bad action_id: %w", err)
	}
	if err := personaProcessor.DispatchAction(ctx, id); err != nil {
		return fmt.Errorf("task:persona:dispatch_action: %w", err)
	}
	return nil
}

// HandleStudioRunBatch runs an AI Content Studio batch (§75).
func HandleStudioRunBatch(ctx context.Context, t *asynq.Task) error {
	if studioProcessor == nil {
		slog.Warn("task:studio:run_batch: no processor registered, skipping")
		return nil
	}
	var p struct{ BatchID string `json:"batch_id"` }
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("task:studio:run_batch: invalid payload: %w", err)
	}
	var id pgtype.UUID
	if err := id.Scan(p.BatchID); err != nil {
		return fmt.Errorf("task:studio:run_batch: bad batch_id: %w", err)
	}
	if err := studioProcessor.RunBatch(ctx, id); err != nil {
		return fmt.Errorf("task:studio:run_batch: %w", err)
	}
	return nil
}

// HandleStudioRetryFailed retries a failed studio job (§75).
func HandleStudioRetryFailed(ctx context.Context, t *asynq.Task) error {
	if studioProcessor == nil {
		slog.Warn("task:studio:retry_failed: no processor registered, skipping")
		return nil
	}
	var p struct{ JobID string `json:"job_id"` }
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("task:studio:retry_failed: invalid payload: %w", err)
	}
	var id pgtype.UUID
	if err := id.Scan(p.JobID); err != nil {
		return fmt.Errorf("task:studio:retry_failed: bad job_id: %w", err)
	}
	if err := studioProcessor.RetryFailed(ctx, id); err != nil {
		return fmt.Errorf("task:studio:retry_failed: %w", err)
	}
	return nil
}
