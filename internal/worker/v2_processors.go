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
	"net/http"
	"time"

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
	FetchMedia(ctx context.Context, tenantID, userID, contentID pgtype.UUID) error
}

// AssessmentProcessor handles assessment background tasks (§71, ADR-071).
type AssessmentProcessor interface {
	ScoreSubmission(ctx context.Context, submissionID, tenantID pgtype.UUID) error
	CleanupAnonymousResults(ctx context.Context, retentionHours int32) (int64, error)
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

// AgentDraftProcessor handles the agent:create_draft_request task
// (Section 103, Phase 8). It creates a draft content row with the AI
// persona as author + a HITL review with source='agent_draft_request'.
// The interface is defined here (in the worker package) so the handler in
// handlers.go can call it without importing internal/ai or internal/content
// (which would create a cycle). The wiring layer (internal/server) provides
// a concrete implementation that closes over the content.Service + ai.Store.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Content Request Flow, Asynq path)
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 8, task 8.03)
type AgentDraftProcessor interface {
	CreateAgentDraft(ctx context.Context, payload AgentDraftPayload) error
}

// AgentDraftPayload is the worker-package mirror of agentDraftRequestPayload
// (handlers.go). It is exported so the AgentDraftProcessor implementation
// in internal/server can receive it without re-decoding the JSON.
type AgentDraftPayload struct {
	AgentSessionID  string `json:"agent_session_id"`
	TenantID        string `json:"tenant_id"`
	UserID          string `json:"user_id"`
	PersonaID       string `json:"persona_id"`
	ContentType     string `json:"content_type"`
	TopicPrompt     string `json:"topic_prompt"`
	ExternalResults string `json:"external_results,omitempty"`
	GeoSpaceID      string `json:"geo_space_id,omitempty"`
}

// MentorProcessor handles mentorship matching background tasks (§102, ADR-110).
type MentorProcessor interface {
	GenerateEmbedding(ctx context.Context, tenantID, userID pgtype.UUID) error
	UpdateMeilisearch(ctx context.Context, tenantID, userID pgtype.UUID) error
}

// EmbassyAlerterProcessor handles Embassy Appointment Alerter background
// tasks (§91, ADR-099). PollEmbassy scrapes a monitored embassy's booking
// page for open slots and, if availability is detected, enqueues
// embassy:dispatch_alerts. DispatchAlerts notifies all users with a
// matching active alert.
// Ref: japaconnect-engineering/sections/91-embassy-appointment-alerter.md
type EmbassyAlerterProcessor interface {
	PollEmbassy(ctx context.Context, embassyID, tenantID pgtype.UUID) error
	DispatchAlerts(ctx context.Context, embassySlug string, tenantID pgtype.UUID) error
	ExpireAlerts(ctx context.Context) (int, error)
}

// CalendarProcessor handles Community Calendar background tasks (§86, ADR-094).
// FlagStaleEntries marks recurring entries as 'unverified' when their
// staleness_check_due date has passed (nightly). SendReminders dispatches
// notifications to subscribers at 90/30/7-day marks before each entry (daily).
// Ref: japaconnect-engineering/sections/86-community-calendar.md
type CalendarProcessor interface {
	FlagStaleEntries(ctx context.Context) (int64, error)
	SendReminders(ctx context.Context) (int, error)
}

// MutualAidProcessor handles Mutual Aid Board background tasks (§85, ADR-093).
// ExpirePosts marks active posts past their 30-day expires_at as 'expired'
// and sends a 3-day-prior notification to owners whose posts are about to
// expire. Nightly sweep on the low queue.
// Ref: japaconnect-engineering/sections/85-mutual-aid-board.md
type MutualAidProcessor interface {
	ExpirePosts(ctx context.Context) (int64, error)
}

// CvFormatterProcessor handles CV Formatter background tasks (§95, ADR-103).
// ProcessSession calls the AI Gateway with the destination's cv_standards
// system prompt + the user's input_text, stores the output, and marks the
// session complete. PurgeExpiredSessions deletes cv_format_sessions rows
// past their expires_at (nightly, low queue).
// Ref: japaconnect-engineering/sections/95-cv-formatter.md
type CvFormatterProcessor interface {
	ProcessSession(ctx context.Context, sessionID pgtype.UUID) error
	PurgeExpiredSessions(ctx context.Context) (int64, error)
}

// DigestProcessor handles Community Digest Widget background tasks (§84,
// ADR-092). GenerateEdition assembles a new digest edition from platform
// analytics (top threads, new members, events, featured story, poll) and
// stores it in draft/review status for the 1-hour admin review window.
// SendEditionEmail sends the published digest to members with the digest
// notification preference enabled, reusing §62 Newsletter infrastructure.
// Ref: japaconnect-engineering/sections/84-community-digest-widget.md
type DigestProcessor interface {
	GenerateEdition(ctx context.Context, tenantID, spaceID pgtype.UUID) error
	SendEditionEmail(ctx context.Context, editionID, tenantID pgtype.UUID) error
}

// JapaStageProcessor handles the japa_stage:recommend fan-out (§87, ADR-095).
// On a successful PATCH /me/japa-stage the handler enqueues the task; the
// processor dispatches tool recommendations, updates the JapaBot peer card
// via Honcho, and prompts a Readiness Assessment when stage=planning and the
// member has no completed assessment.
type JapaStageProcessor interface {
	RecommendForStage(ctx context.Context, userID, tenantID pgtype.UUID, stage, destSpaceID string) error
}

// ---------------------------------------------------------------------------
// Global processor slots — nil-safe (graceful degradation when unregistered).
// ---------------------------------------------------------------------------

var (
	jobsProcessor           JobsProcessor
	gamificationProcessor   GamificationProcessor
	importProcessor         ImportProcessor
	assessmentProcessor     AssessmentProcessor
	scholarshipProcessor    ScholarshipProcessor
	remittanceProcessor     RemittanceProcessor
	referralProcessor       ReferralProcessor
	adsSelfServeProcessor   AdsSelfServeProcessor
	pushProcessor           PushProcessor
	personaProcessor        PersonaProcessor
	studioProcessor         StudioProcessor
	mentorProcessor         MentorProcessor
	embassyAlerterProcessor EmbassyAlerterProcessor
	calendarProcessor       CalendarProcessor
	mutualAidProcessor      MutualAidProcessor
	cvFormatterProcessor    CvFormatterProcessor
	digestProcessor         DigestProcessor
	agentDraftProcessor     AgentDraftProcessor
	japaStageProcessor      JapaStageProcessor
)

// Register functions — call once at startup before the worker serves.

func RegisterJobsProcessor(p JobsProcessor)                     { jobsProcessor = p }
func RegisterGamificationProcessor(p GamificationProcessor)     { gamificationProcessor = p }
func RegisterImportProcessor(p ImportProcessor)                 { importProcessor = p }
func RegisterAssessmentProcessor(p AssessmentProcessor)         { assessmentProcessor = p }
func RegisterScholarshipProcessor(p ScholarshipProcessor)       { scholarshipProcessor = p }
func RegisterRemittanceProcessor(p RemittanceProcessor)         { remittanceProcessor = p }
func RegisterReferralProcessor(p ReferralProcessor)             { referralProcessor = p }
func RegisterAdsSelfServeProcessor(p AdsSelfServeProcessor)     { adsSelfServeProcessor = p }
func RegisterPushProcessor(p PushProcessor)                     { pushProcessor = p }
func RegisterPersonaProcessor(p PersonaProcessor)               { personaProcessor = p }
func RegisterStudioProcessor(p StudioProcessor)                 { studioProcessor = p }
func RegisterMentorProcessor(p MentorProcessor)                 { mentorProcessor = p }
func RegisterEmbassyAlerterProcessor(p EmbassyAlerterProcessor) { embassyAlerterProcessor = p }
func RegisterCalendarProcessor(p CalendarProcessor)             { calendarProcessor = p }
func RegisterMutualAidProcessor(p MutualAidProcessor)           { mutualAidProcessor = p }
func RegisterCvFormatterProcessor(p CvFormatterProcessor)       { cvFormatterProcessor = p }
func RegisterDigestProcessor(p DigestProcessor)                 { digestProcessor = p }
func RegisterAgentDraftProcessor(p AgentDraftProcessor)         { agentDraftProcessor = p }
func RegisterJapaStageProcessor(p JapaStageProcessor)           { japaStageProcessor = p }

// ---------------------------------------------------------------------------
// Task type constants (V2 — Sections 60–77).
// ---------------------------------------------------------------------------

const (
	TypeJobsExpireListings           = "jobs:expire_listings"
	TypeJobsNotifyApplicationStatus  = "jobs:notify_application_status"
	TypeGamificationExpireChallenges = "gamification:expire_challenges"
	TypeImportProcessSource          = "import:process_source"
	TypeImportFetchMedia             = "import:fetch_media"
	TypeAssessmentScoreSubmission    = "assessment:score_submission"
	TypeAssessmentCleanupAnonymous   = "assessment:cleanup_anonymous"
	TypeScholarshipCheckDeadlines    = "scholarship:check_deadlines"
	TypeRemittanceFetchRates         = "remittance:fetch_rates"
	TypeRemittanceCheckAlerts        = "remittance:check_alerts"
	TypeRemittancePruneOldRates      = "remittance:prune_old_rates"
	TypeReferralRecordClick          = "referral:record_click"
	TypeReferralCheckConversion      = "referral:check_conversion"
	TypeReferralReviewFlagged        = "referral:review_flagged"
	TypeAdsRefundCampaignCredits     = "ads:refund_campaign_credits"
	TypeAdsNotifyCampaignStatus      = "ads:notify_campaign_status"
	TypeNotificationSendPush         = "notification:send_push"
	TypePersonaRunActivity           = "persona:run_activity"
	TypePersonaDispatchAction        = "persona:dispatch_action"
	TypeStudioRunBatch               = "studio:run_batch"
	TypeStudioRetryFailed            = "studio:retry_failed"

	// §102 Mentorship Matching (ADR-110).
	TypeMentorGenerateEmbedding = "mentor:generate_embedding"
	TypeMentorUpdateMeilisearch = "mentor:update_meilisearch"

	// §91 Embassy Appointment Alerter (ADR-099).
	// The asynq-tasks catalog lists `embassy:poll_{slug}` as the logical
	// per-embassy task; the registered handler type is `embassy:poll` with
	// the embassy slug carried in the JSON payload (asynq HandleFunc
	// matches exact task-type strings, so the slug is not embedded in the
	// type name). `embassy:dispatch_alerts` fires notifications when
	// availability is detected.
	TypeEmbassyPoll           = "embassy:poll"
	TypeEmbassyDispatchAlerts = "embassy:dispatch_alerts"
	TypeEmbassyExpireChecks   = "embassy:expire_checks"

	// §86 Community Calendar (ADR-094).
	// calendar:staleness_check runs nightly on the low queue, marking
	// recurring entries as 'unverified' when their staleness_check_due date
	// has passed. calendar:send_reminders runs daily on the default queue,
	// dispatching notifications to subscribers at 90/30/7-day marks.
	TypeCalendarStalenessCheck = "calendar:staleness_check"
	TypeCalendarSendReminders  = "calendar:send_reminders"

	// §85 Mutual Aid Board (ADR-093).
	// mutual_aid:expire runs nightly on the low queue, expiring posts
	// past their 30-day expires_at and notifying owners 3 days before.
	TypeMutualAidExpire = "mutual_aid:expire"

	// §94 Professional Credential Checker (ADR-102).
	// credentials:check_links runs weekly on the low queue, HEAD-checking
	// regulatory_body_url on active pathways and logging pathways whose
	// URLs return 404 so admins can review and update them.
	TypeCredentialsCheckLinks = "credentials:check_links"

	// §95 CV Formatter (ADR-103).
	// cv_format:process runs on the default queue, processing a single
	// pending session via the AI Gateway. cv_format_sessions:purge runs
	// nightly on the low queue, deleting sessions past their expires_at.
	TypeCvFormatProcess       = "cv_format:process"
	TypeCvFormatSessionsPurge = "cv_format_sessions:purge"

	// §84 Community Digest Widget (ADR-092).
	// digest:generate runs weekly on Sunday 22:00 UTC on the low queue,
	// assembling a new digest edition from platform analytics (top threads,
	// new members, events, featured story, poll). digest:send_email runs
	// on the default queue when an edition is published, sending the digest
	// to members with the digest notification preference enabled (reuses
	// §62 Newsletter infrastructure).
	TypeDigestGenerate  = "digest:generate"
	TypeDigestSendEmail = "digest:send_email"

	// §87 Japa Stage Progress Profiles (ADR-095).
	// japa_stage:recommend runs on the default queue after a successful
	// PATCH /me/japa-stage, dispatching tool recommendations for the new
	// stage, updating the JapaBot peer card via Honcho, and prompting a
	// Readiness Assessment when stage=planning and the member has none.
	// Idempotent on (user_id, stage, set_at) — enqueued with a 1-minute
	// Unique window so a duplicate PATCH does not double-dispatch.
	TypeJapaStageRecommend = "japa_stage:recommend"
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
	var p struct {
		ApplicationID string `json:"application_id"`
	}
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
	var p struct {
		ImportID string `json:"import_id"`
	}
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
	var p struct {
		ContentID string `json:"content_id"`
		TenantID  string `json:"tenant_id"`
		UserID    string `json:"user_id"`
	}
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("task:import:fetch_media: invalid payload: %w", err)
	}
	var contentID, tenantID, userID pgtype.UUID
	if err := contentID.Scan(p.ContentID); err != nil {
		return fmt.Errorf("task:import:fetch_media: bad content_id: %w", err)
	}
	if err := tenantID.Scan(p.TenantID); err != nil {
		return fmt.Errorf("task:import:fetch_media: bad tenant_id: %w", err)
	}
	if err := userID.Scan(p.UserID); err != nil {
		return fmt.Errorf("task:import:fetch_media: bad user_id: %w", err)
	}
	if err := importProcessor.FetchMedia(ctx, tenantID, userID, contentID); err != nil {
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

// HandleAssessmentCleanupAnonymous deletes anonymous assessment_results
// older than the retention window (§71, ADR-071 §5). Scheduled hourly.
func HandleAssessmentCleanupAnonymous(ctx context.Context, t *asynq.Task) error {
	if assessmentProcessor == nil {
		slog.Warn("task:assessment:cleanup_anonymous: no processor registered, skipping")
		return nil
	}
	retentionHours := int32(24)
	if len(t.Payload()) > 0 {
		var p struct {
			RetentionHours int32 `json:"retention_hours"`
		}
		if err := json.Unmarshal(t.Payload(), &p); err != nil {
			return fmt.Errorf("task:assessment:cleanup_anonymous: invalid payload: %w", err)
		}
		if p.RetentionHours > 0 {
			retentionHours = p.RetentionHours
		}
	}
	n, err := assessmentProcessor.CleanupAnonymousResults(ctx, retentionHours)
	if err != nil {
		return fmt.Errorf("task:assessment:cleanup_anonymous: %w", err)
	}
	slog.Info("task:assessment:cleanup_anonymous: rows deleted", "count", n, "retention_hours", retentionHours)
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
	var p struct {
		ProviderID string `json:"provider_id"`
	}
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
	var p struct {
		ReferralID string `json:"referral_id"`
	}
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
	var p struct {
		ReferralID string `json:"referral_id"`
	}
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
	var p struct {
		CampaignID string `json:"campaign_id"`
	}
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
	var p struct {
		CampaignID string `json:"campaign_id"`
	}
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
	var p struct {
		PersonaID string `json:"persona_id"`
	}
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
	var p struct {
		ActionID string `json:"action_id"`
	}
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
	var p struct {
		BatchID string `json:"batch_id"`
	}
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
	var p struct {
		JobID string `json:"job_id"`
	}
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

// HandleMentorGenerateEmbedding generates a pgvector embedding for a
// mentor's bio (§102, ADR-110). Called after a mentor updates their bio.
func HandleMentorGenerateEmbedding(ctx context.Context, t *asynq.Task) error {
	if mentorProcessor == nil {
		slog.Warn("task:mentor:generate_embedding: no processor registered, skipping")
		return nil
	}
	var p struct {
		TenantID string `json:"tenant_id"`
		UserID   string `json:"user_id"`
	}
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("task:mentor:generate_embedding: invalid payload: %w", err)
	}
	var tenantID, userID pgtype.UUID
	if err := tenantID.Scan(p.TenantID); err != nil {
		return fmt.Errorf("task:mentor:generate_embedding: bad tenant_id: %w", err)
	}
	if err := userID.Scan(p.UserID); err != nil {
		return fmt.Errorf("task:mentor:generate_embedding: bad user_id: %w", err)
	}
	if err := mentorProcessor.GenerateEmbedding(ctx, tenantID, userID); err != nil {
		return fmt.Errorf("task:mentor:generate_embedding: %w", err)
	}
	return nil
}

// HandleMentorUpdateMeilisearch syncs a mentor's profile to the
// Meilisearch mentors index (§102, ADR-110).
func HandleMentorUpdateMeilisearch(ctx context.Context, t *asynq.Task) error {
	if mentorProcessor == nil {
		slog.Warn("task:mentor:update_meilisearch: no processor registered, skipping")
		return nil
	}
	var p struct {
		TenantID string `json:"tenant_id"`
		UserID   string `json:"user_id"`
	}
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("task:mentor:update_meilisearch: invalid payload: %w", err)
	}
	var tenantID, userID pgtype.UUID
	if err := tenantID.Scan(p.TenantID); err != nil {
		return fmt.Errorf("task:mentor:update_meilisearch: bad tenant_id: %w", err)
	}
	if err := userID.Scan(p.UserID); err != nil {
		return fmt.Errorf("task:mentor:update_meilisearch: bad user_id: %w", err)
	}
	if err := mentorProcessor.UpdateMeilisearch(ctx, tenantID, userID); err != nil {
		return fmt.Errorf("task:mentor:update_meilisearch: %w", err)
	}
	return nil
}

// HandleEmbassyPoll polls a monitored embassy's booking page for open
// appointment slots (§91, ADR-099). When availability is detected the
// processor enqueues embassy:dispatch_alerts for the matching slug.
func HandleEmbassyPoll(ctx context.Context, t *asynq.Task) error {
	if embassyAlerterProcessor == nil {
		slog.Warn("task:embassy:poll: no processor registered, skipping")
		return nil
	}
	var p struct {
		EmbassyID string `json:"embassy_id"`
		TenantID  string `json:"tenant_id"`
	}
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("task:embassy:poll: invalid payload: %w", err)
	}
	var embassyID, tenantID pgtype.UUID
	if err := embassyID.Scan(p.EmbassyID); err != nil {
		return fmt.Errorf("task:embassy:poll: bad embassy_id: %w", err)
	}
	if err := tenantID.Scan(p.TenantID); err != nil {
		return fmt.Errorf("task:embassy:poll: bad tenant_id: %w", err)
	}
	if err := embassyAlerterProcessor.PollEmbassy(ctx, embassyID, tenantID); err != nil {
		return fmt.Errorf("task:embassy:poll: %w", err)
	}
	return nil
}

// HandleEmbassyDispatchAlerts dispatches notifications to all users with an
// active alert matching the embassy whose availability was detected (§91).
func HandleEmbassyDispatchAlerts(ctx context.Context, t *asynq.Task) error {
	if embassyAlerterProcessor == nil {
		slog.Warn("task:embassy:dispatch_alerts: no processor registered, skipping")
		return nil
	}
	var p struct {
		EmbassySlug string `json:"embassy_slug"`
		TenantID    string `json:"tenant_id"`
	}
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("task:embassy:dispatch_alerts: invalid payload: %w", err)
	}
	var tenantID pgtype.UUID
	if err := tenantID.Scan(p.TenantID); err != nil {
		return fmt.Errorf("task:embassy:dispatch_alerts: bad tenant_id: %w", err)
	}
	if err := embassyAlerterProcessor.DispatchAlerts(ctx, p.EmbassySlug, tenantID); err != nil {
		return fmt.Errorf("task:embassy:dispatch_alerts: %w", err)
	}
	return nil
}

// HandleEmbassyExpireChecks transitions appointment_alerts past their
// expires_at from 'active' to 'expired' (§91, ADR-099). Nightly sweep.
func HandleEmbassyExpireChecks(ctx context.Context, _ *asynq.Task) error {
	if embassyAlerterProcessor == nil {
		slog.Warn("task:embassy:expire_checks: no processor registered, skipping")
		return nil
	}
	n, err := embassyAlerterProcessor.ExpireAlerts(ctx)
	if err != nil {
		return fmt.Errorf("task:embassy:expire_checks: %w", err)
	}
	slog.Info("task:embassy:expire_checks: expired alerts", "count", n)
	return nil
}

// HandleCalendarStalenessCheck marks recurring calendar entries as
// 'unverified' when their staleness_check_due date has passed (§86, ADR-094).
// Nightly sweep on the low queue.
func HandleCalendarStalenessCheck(ctx context.Context, _ *asynq.Task) error {
	if calendarProcessor == nil {
		slog.Warn("task:calendar:staleness_check: no processor registered, skipping")
		return nil
	}
	n, err := calendarProcessor.FlagStaleEntries(ctx)
	if err != nil {
		return fmt.Errorf("task:calendar:staleness_check: %w", err)
	}
	slog.Info("task:calendar:staleness_check: flagged stale entries", "count", n)
	return nil
}

// HandleCalendarSendReminders dispatches notifications to subscribers at
// 90/30/7-day marks before each published calendar entry (§86, ADR-094).
// Daily sweep on the default queue.
func HandleCalendarSendReminders(ctx context.Context, _ *asynq.Task) error {
	if calendarProcessor == nil {
		slog.Warn("task:calendar:send_reminders: no processor registered, skipping")
		return nil
	}
	n, err := calendarProcessor.SendReminders(ctx)
	if err != nil {
		return fmt.Errorf("task:calendar:send_reminders: %w", err)
	}
	slog.Info("task:calendar:send_reminders: dispatched reminders", "count", n)
	return nil
}

// HandleMutualAidExpire expires mutual aid posts past their 30-day
// expires_at and sends a 3-day-prior notification to owners whose posts
// are about to expire (§85, ADR-093). Nightly sweep on the low queue.
func HandleMutualAidExpire(ctx context.Context, _ *asynq.Task) error {
	if mutualAidProcessor == nil {
		slog.Warn("task:mutual_aid:expire: no processor registered, skipping")
		return nil
	}
	n, err := mutualAidProcessor.ExpirePosts(ctx)
	if err != nil {
		return fmt.Errorf("task:mutual_aid:expire: %w", err)
	}
	slog.Info("task:mutual_aid:expire: expired posts", "count", n)
	return nil
}

// HandleCredentialsCheckLinks HEAD-checks regulatory_body_url on active
// credential pathways and logs pathways whose URLs return 404 so admins
// can review and update them (§94, ADR-102). Weekly sweep on the low
// queue. Also updates source_url_last_checked to today for each checked
// pathway.
func HandleCredentialsCheckLinks(ctx context.Context, _ *asynq.Task) error {
	if dbPool == nil {
		slog.Warn("task:credentials:check_links: no db pool registered, skipping")
		return nil
	}

	const q = `
SELECT id, regulatory_body_url
FROM credential_pathways
WHERE is_active = true
  AND regulatory_body_url IS NOT NULL
  AND regulatory_body_url != ''
`
	rows, err := dbPool.Reader.Query(ctx, q)
	if err != nil {
		return fmt.Errorf("task:credentials:check_links: query pathways: %w", err)
	}
	defer rows.Close()

	type pathway struct {
		id  pgtype.UUID
		url string
	}
	var pathways []pathway
	for rows.Next() {
		var p pathway
		var urlPtr *string
		if err := rows.Scan(&p.id, &urlPtr); err != nil {
			return fmt.Errorf("task:credentials:check_links: scan row: %w", err)
		}
		if urlPtr != nil {
			p.url = *urlPtr
			pathways = append(pathways, p)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("task:credentials:check_links: iterate rows: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	broken := 0
	for _, p := range pathways {
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, p.url, nil)
		if err != nil {
			slog.Warn("task:credentials:check_links: invalid URL",
				"pathway_id", p.id.String(), "url", p.url, "error", err)
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			slog.Warn("task:credentials:check_links: request failed",
				"pathway_id", p.id.String(), "url", p.url, "error", err)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			broken++
			slog.Warn("task:credentials:check_links: regulatory_body_url returns 404",
				"pathway_id", p.id.String(), "url", p.url)
		}
	}

	// Update source_url_last_checked for all checked pathways.
	if len(pathways) > 0 {
		const updateQ = `
UPDATE credential_pathways
SET source_url_last_checked = now()::date, updated_at = now()
WHERE is_active = true
  AND regulatory_body_url IS NOT NULL
  AND regulatory_body_url != ''
`
		if _, err := dbPool.Writer.Exec(ctx, updateQ); err != nil {
			slog.Error("task:credentials:check_links: update last_checked failed", "error", err)
		}
	}

	slog.Info("task:credentials:check_links: checked pathways",
		"total", len(pathways), "broken", broken)
	return nil
}

// HandleCvFormatProcess processes a single CV format session via the AI
// Gateway (§95, ADR-103). The session_id is carried in the JSON payload.
// Default queue. Idempotent on session_id — re-processing a completed
// session is a no-op.
func HandleCvFormatProcess(ctx context.Context, t *asynq.Task) error {
	if cvFormatterProcessor == nil {
		slog.Warn("task:cv_format:process: no processor registered, skipping")
		return nil
	}
	var p struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("task:cv_format:process: invalid payload: %w", err)
	}
	var id pgtype.UUID
	if err := id.Scan(p.SessionID); err != nil {
		return fmt.Errorf("task:cv_format:process: bad session_id: %w", err)
	}
	if err := cvFormatterProcessor.ProcessSession(ctx, id); err != nil {
		return fmt.Errorf("task:cv_format:process: %w", err)
	}
	return nil
}

// HandleCvFormatSessionsPurge deletes cv_format_sessions rows past their
// expires_at (§95, ADR-103). Nightly sweep on the low queue.
func HandleCvFormatSessionsPurge(ctx context.Context, _ *asynq.Task) error {
	if cvFormatterProcessor == nil {
		slog.Warn("task:cv_format_sessions:purge: no processor registered, skipping")
		return nil
	}
	n, err := cvFormatterProcessor.PurgeExpiredSessions(ctx)
	if err != nil {
		return fmt.Errorf("task:cv_format_sessions:purge: %w", err)
	}
	slog.Info("task:cv_format_sessions:purge: purged expired sessions", "count", n)
	return nil
}

// HandleDigestGenerate assembles a new community digest edition from platform
// analytics (§84, ADR-092). Runs weekly on Sunday 22:00 UTC on the low queue.
// When triggered by the admin "Generate" button, the payload carries
// tenant_id and optionally space_id for a per-Space digest.
func HandleDigestGenerate(ctx context.Context, t *asynq.Task) error {
	if digestProcessor == nil {
		slog.Warn("task:digest:generate: no processor registered, skipping")
		return nil
	}
	var p struct {
		TenantID string `json:"tenant_id"`
		SpaceID  string `json:"space_id"`
	}
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("task:digest:generate: invalid payload: %w", err)
	}
	if p.TenantID == "" {
		// Cron mode: iterate all tenants. The processor handles tenant
		// iteration when tenantID is the zero UUID.
		if err := digestProcessor.GenerateEdition(ctx, pgtype.UUID{}, pgtype.UUID{}); err != nil {
			return fmt.Errorf("task:digest:generate: %w", err)
		}
		return nil
	}
	var tenantID pgtype.UUID
	if err := tenantID.Scan(p.TenantID); err != nil {
		return fmt.Errorf("task:digest:generate: bad tenant_id: %w", err)
	}
	var spaceID pgtype.UUID
	if p.SpaceID != "" {
		if err := spaceID.Scan(p.SpaceID); err != nil {
			return fmt.Errorf("task:digest:generate: bad space_id: %w", err)
		}
	}
	if err := digestProcessor.GenerateEdition(ctx, tenantID, spaceID); err != nil {
		return fmt.Errorf("task:digest:generate: %w", err)
	}
	slog.Info("task:digest:generate: edition assembled",
		"tenant_id", p.TenantID, "space_id", p.SpaceID)
	return nil
}

// HandleDigestSendEmail sends a published digest edition to members with the
// digest notification preference enabled (§84, ADR-092). Runs on the default
// queue when an edition is published. Reuses §62 Newsletter infrastructure.
func HandleDigestSendEmail(ctx context.Context, t *asynq.Task) error {
	if digestProcessor == nil {
		slog.Warn("task:digest:send_email: no processor registered, skipping")
		return nil
	}
	var p struct {
		EditionID string `json:"edition_id"`
		TenantID  string `json:"tenant_id"`
	}
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("task:digest:send_email: invalid payload: %w", err)
	}
	var editionID pgtype.UUID
	if err := editionID.Scan(p.EditionID); err != nil {
		return fmt.Errorf("task:digest:send_email: bad edition_id: %w", err)
	}
	var tenantID pgtype.UUID
	if err := tenantID.Scan(p.TenantID); err != nil {
		return fmt.Errorf("task:digest:send_email: bad tenant_id: %w", err)
	}
	if err := digestProcessor.SendEditionEmail(ctx, editionID, tenantID); err != nil {
		return fmt.Errorf("task:digest:send_email: %w", err)
	}
	slog.Info("task:digest:send_email: sent",
		"edition_id", p.EditionID, "tenant_id", p.TenantID)
	return nil
}

// HandleJapaStageRecommend dispatches the §87 stage-change fan-out: tool
// recommendation notification, JapaBot peer card update via Honcho, and a
// Readiness Assessment prompt when stage=planning and the member has no
// completed assessment. Enqueued fire-and-forget by PATCH /me/japa-stage.
func HandleJapaStageRecommend(ctx context.Context, t *asynq.Task) error {
	if japaStageProcessor == nil {
		slog.Warn("task:japa_stage:recommend: no processor registered, skipping")
		return nil
	}
	var p struct {
		UserID             string `json:"user_id"`
		TenantID           string `json:"tenant_id"`
		Stage              string `json:"stage"`
		DestinationSpaceID string `json:"destination_space_id"`
	}
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("task:japa_stage:recommend: invalid payload: %w", err)
	}
	var userID pgtype.UUID
	if err := userID.Scan(p.UserID); err != nil {
		return fmt.Errorf("task:japa_stage:recommend: bad user_id: %w", err)
	}
	var tenantID pgtype.UUID
	if err := tenantID.Scan(p.TenantID); err != nil {
		return fmt.Errorf("task:japa_stage:recommend: bad tenant_id: %w", err)
	}
	if err := japaStageProcessor.RecommendForStage(ctx, userID, tenantID, p.Stage, p.DestinationSpaceID); err != nil {
		return fmt.Errorf("task:japa_stage:recommend: %w", err)
	}
	slog.Info("task:japa_stage:recommend: dispatched",
		"user_id", p.UserID, "stage", p.Stage)
	return nil
}
