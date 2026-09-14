// Package scamalert — service.go orchestrates scam report submission,
// browsing, subscription, and moderator review. Feature-flagged via
// scam_alert.enabled system_setting (§81). Anonymous reports are allowed
// when scam_alert.anonymous_reports is enabled.
// // Ref: japaconnect-engineering/sections/81-scam-alert-board.md
// //	japaconnect-engineering/adr/ADR-089-scam-alert-board.md
package scamalert

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/wallet"
)

// ScamReportDTO is the API response shape for a single scam report.
type ScamReportDTO struct {
	ID                    string `json:"id"`
	ReporterID            string `json:"reporter_id,omitempty"`
	ScamType              string `json:"scam_type"`
	DestinationSpaceID    string `json:"destination_space_id,omitempty"`
	DestinationSpaceName  string `json:"destination_space_name,omitempty"`
	DestinationSpaceCode  string `json:"destination_space_code,omitempty"`
	OriginSpaceID         string `json:"origin_space_id,omitempty"`
	OriginSpaceName       string `json:"origin_space_name,omitempty"`
	OriginSpaceCode       string `json:"origin_space_code,omitempty"`
	OriginCommunity       string `json:"origin_community,omitempty"`
	PlatformUsed          string `json:"platform_used,omitempty"`
	ApproximateDate       string `json:"approximate_date,omitempty"`
	AmountRange           string `json:"amount_range,omitempty"`
	Description           string `json:"description"`
	ReportedToAuthorities bool   `json:"reported_to_authorities"`
	Status                string `json:"status"`
	ModeratorNote         string `json:"moderator_note,omitempty"`
	IsPinned              bool   `json:"is_pinned"`
	IsSubscribed          bool   `json:"is_subscribed"`
	PinnedSpaceID         string `json:"pinned_space_id,omitempty"`
	ReviewedBy            string `json:"reviewed_by,omitempty"`
	ReviewedAt            string `json:"reviewed_at,omitempty"`
	// ADR-088 universal AI-generation fields (admin review surfaces).
	AiGenerated    bool   `json:"ai_generated"`
	AiModel        string `json:"ai_model,omitempty"`
	AiGeneratedAt  string `json:"ai_generated_at,omitempty"`
	HitlReviewedBy string `json:"hitl_reviewed_by,omitempty"`
	HitlReviewedAt string `json:"hitl_reviewed_at,omitempty"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at,omitempty"`
	ReporterName   string `json:"reporter_name,omitempty"`
}

// SubmitReportInput is the request body for POST /scam-alerts.
type SubmitReportInput struct {
	ScamType              string `json:"scam_type"`
	DestinationSpaceID    string `json:"destination_space_id"`
	OriginSpaceID         string `json:"origin_space_id"`
	OriginCommunity       string `json:"origin_community"`
	PlatformUsed          string `json:"platform_used"`
	ApproximateDate       string `json:"approximate_date"` // YYYY-MM-DD
	AmountRange           string `json:"amount_range"`
	Description           string `json:"description"`
	ReportedToAuthorities bool   `json:"reported_to_authorities"`
}

// UpdateReportInput is the request body for PATCH /scam-alerts/:id.
// It mirrors SubmitReportInput but omits immutable ownership fields
// (reporter_id, status, moderator_note, reviewed_by, etc.).
type UpdateReportInput SubmitReportInput

// ReviewInput is the request body for PATCH /admin/scam-alerts/:id.
type ReviewInput struct {
	Status        string `json:"status"` // confirmed|debunked|duplicate
	ModeratorNote string `json:"moderator_note"`
}

// Notifier is the interface used by the scam-alert service to dispatch
// notifications to subscribers. It is satisfied by *notifications.Service.
// Defined here (rather than importing notifications directly) to avoid an
// import cycle: notifications → worker → scamalert.
// Ref: japaconnect-engineering/sections/35-notification-system.md.
type Notifier interface {
	CreateNotification(ctx context.Context, recipientID pgtype.UUID, nType, actorType string, actorID pgtype.UUID, subjectType string, subjectID pgtype.UUID, title, body, actionURL string) error
}

// activityEmitter records wallet activity events and enqueues processing.
// Satisfied by *wallet.Service (wired via SetActivityEmitter). The emitted
// event is processed by the wallet:process_activity_event task, which runs
// badge/challenge evaluation for the scam_alert.confirmed key — including
// the community_guardian badge (§64).
type activityEmitter = wallet.ActivityEmitter

// TaskEnqueuer enqueues Asynq tasks fire-and-forget (Rule 8).
type TaskEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// FeatureChecker checks whether a feature flag is enabled for a tenant.
// Satisfied by *features.Checker via structural typing — defined locally to
// avoid importing the features package (keeps the dependency graph clean).
type FeatureChecker interface {
	IsEnabled(ctx context.Context, tenantID pgtype.UUID, key string) bool
}

// Task name constants for Asynq tasks enqueued by this service.
const (
	TaskNotifySubscribers = "scam_alert:notify_subscribers"
	TaskAwardPoints       = "wallet:award_points"
)

// Service holds the business logic for the Scam Alert Board.
type Service struct {
	store    *Store
	settings *config.Settings
	notifier Notifier
	activity activityEmitter
	enqueuer TaskEnqueuer
	checker  FeatureChecker
	rbac     permissionChecker
}

// NewService creates a scam-alert service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{store: store, settings: settings}
}

// SetNotifier injects the notification service used to dispatch subscriber
// notifications on report confirmation (§81, §35). Called once at startup.
func (s *Service) SetNotifier(n Notifier) { s.notifier = n }

// SetActivityEmitter injects the wallet activity emitter used to record
// scam_alert.confirmed events for the reporter — the process task evaluates
// badge/challenge triggers (community_guardian, §64). Optional — nil-safe.
func (s *Service) SetActivityEmitter(e activityEmitter) { s.activity = e }

// SetEnqueuer injects the Asynq task enqueuer for fire-and-forget fan-out
// (Rule 8). Called once at startup.
func (s *Service) SetEnqueuer(e TaskEnqueuer) { s.enqueuer = e }

// SetFeatureChecker injects the feature flag checker so Enabled can read the
// feature_flags table (the single source of truth for feature reachability)
// instead of system_settings. Falls back to system_settings when nil.
func (s *Service) SetFeatureChecker(c FeatureChecker) { s.checker = c }

// SetRBAC injects the permission resolver used for edit authorization
// (scam_alert.edit_own / scam_alert.edit_any).
func (s *Service) SetRBAC(r permissionChecker) { s.rbac = r }

// Enabled checks whether the scam_alert feature is enabled for the given
// tenant. Delegates to the features.Checker (feature_flags table) when
// available, falling back to system_settings for backward compatibility.
func (s *Service) Enabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.checker != nil {
		return s.checker.IsEnabled(ctx, tenantID, "scam_alert.enabled")
	}

	if s.settings == nil {
		return false
	}

	return s.settings.GetBoolOr(ctx, "scam_alert.enabled", false)
}

// AnonymousReportsAllowed reads the scam_alert.anonymous_reports setting.
// §81 default is true.
func (s *Service) AnonymousReportsAllowed(ctx context.Context) bool {
	if s.settings == nil {
		return true
	}

	return s.settings.GetBoolOr(ctx, "scam_alert.anonymous_reports", true)
}

// Option is a single selectable value exposed by the schema endpoint.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// SchemaOptions holds the canonical scam report vocabulary.
type SchemaOptions struct {
	ScamTypes []Option `json:"scam_types"`
	Platforms []Option `json:"platforms"`
}

// validScamTypes and validPlatforms are the §81 enum values. The DB CHECK
// constraints (migration 000417) enforce these at the store layer; these
// constants let the service reject invalid input with a structured error
// before hitting the DB.
var validScamTypes = map[string]bool{
	"fraudulent_agent":  true,
	"fake_job":          true,
	"housing_fraud":     true,
	"fake_scholarship":  true,
	"whatsapp_telegram": true,
	"other":             true,
}

var validPlatforms = map[string]bool{
	"whatsapp":  true,
	"instagram": true,
	"linkedin":  true,
	"email":     true,
	"website":   true,
	"other":     true,
}

// human labels for schema endpoint (Rule 16: backend owns canonical values).
var scamTypeLabels = map[string]string{
	"fraudulent_agent":  "Fraudulent agent",
	"fake_job":          "Fake job offer",
	"housing_fraud":     "Housing fraud",
	"fake_scholarship":  "Fake scholarship",
	"whatsapp_telegram": "WhatsApp / Telegram",
	"other":             "Other",
}

var platformLabels = map[string]string{
	"whatsapp":  "WhatsApp",
	"instagram": "Instagram",
	"linkedin":  "LinkedIn",
	"email":     "Email",
	"website":   "Website",
	"other":     "Other",
}

func sortOptions(opts []Option) {
	sort.Slice(opts, func(i, j int) bool { return opts[i].Value < opts[j].Value })
}

// Schema returns the canonical scam type and platform options.
func (s *Service) Schema() SchemaOptions {
	scamTypes := make([]Option, 0, len(validScamTypes))
	for v := range validScamTypes {
		scamTypes = append(scamTypes, Option{Value: v, Label: scamTypeLabels[v]})
	}

	sortOptions(scamTypes)

	platforms := make([]Option, 0, len(validPlatforms))
	for v := range validPlatforms {
		platforms = append(platforms, Option{Value: v, Label: platformLabels[v]})
	}

	sortOptions(platforms)

	return SchemaOptions{ScamTypes: scamTypes, Platforms: platforms}
}

// validReviewStatuses are the status values a moderator may set via Review
// (§81 status machine: under_review → confirmed | debunked | duplicate).
var validReviewStatuses = map[string]bool{
	"confirmed": true,
	"debunked":  true,
	"duplicate": true,
}

// ErrInvalidScamType is returned when the submitted scam_type is not in the
// §81 enum.
var ErrInvalidScamType = errors.New("invalid scam_type")

// ErrInvalidPlatform is returned when the submitted platform_used is not in
// the §81 enum.
var ErrInvalidPlatform = errors.New("invalid platform_used")

// ErrInvalidStatus is returned when the review status is not in the §81 enum.
var ErrInvalidStatus = errors.New("invalid status")

// ErrEditForbidden is returned when a user attempts to edit a scam report they
// do not own and they do not hold scam_alert.edit_any.
var ErrEditForbidden = errors.New("edit forbidden")

// ErrModeratorNoteRequired is returned when a report is confirmed without a
// moderator note. §81 legal boundary: a confirmed report must always carry a
// moderator note — it is also the HITL record for AI-generated reports
// (ADR-088).
var ErrModeratorNoteRequired = errors.New("moderator note required")

// permissionChecker is the subset of rbac.RBAC used for edit authorization.
type permissionChecker interface {
	HasPermission(ctx context.Context, userID pgtype.UUID, permKey string) bool
}

// Submit creates a new scam report in 'under_review' status. reporterID may
// be zero-valued (invalid) for anonymous reports when anonymous reports are
// allowed.
func (s *Service) Submit(ctx context.Context, reporterID pgtype.UUID, input SubmitReportInput) (ScamReportDTO, error) {
	report, err := buildScamReport(UpdateReportInput(input))
	if err != nil {
		return ScamReportDTO{}, err
	}

	// reporterID is only set when valid (authenticated submitter).
	if reporterID.Valid {
		report.ReporterID = reporterID
	}

	report.Status = "under_review"

	id, err := s.store.CreateReport(ctx, report)
	if err != nil {
		slog.Error("scamalert: create report failed", "error", err)
		return ScamReportDTO{}, fmt.Errorf("scamalert: submit: %w", err)
	}

	// Return the pending report as submitted. The public Get/List endpoints
	// only surface confirmed reports, so we build the DTO from the input
	// rather than re-querying (the report is not yet publicly visible).
	dto := ScamReportDTO{
		ID:                    id.String(),
		ScamType:              report.ScamType,
		DestinationSpaceID:    report.DestinationSpaceID.String(),
		Description:           report.Description,
		ReportedToAuthorities: report.ReportedToAuthorities,
		Status:                report.Status,
	}
	if report.OriginSpaceID.Valid {
		dto.OriginSpaceID = report.OriginSpaceID.String()
	}

	if report.OriginCommunity != nil {
		dto.OriginCommunity = *report.OriginCommunity
	}

	if report.PlatformUsed != nil {
		dto.PlatformUsed = *report.PlatformUsed
	}

	if report.ApproximateDate.Valid {
		dto.ApproximateDate = report.ApproximateDate.Time.Format("2006-01-02")
	}

	if report.AmountRange != nil {
		dto.AmountRange = *report.AmountRange
	}

	return dto, nil
}

// buildScamReport validates a submit/update input and converts it into a
// ScamReport value. The caller is responsible for setting TenantID, ReporterID
// and Status for create, and for supplying the report id/tenant for update.
func buildScamReport(input UpdateReportInput) (ScamReport, error) {
	if input.ScamType == "" || input.DestinationSpaceID == "" || input.Description == "" {
		return ScamReport{}, errors.New("scam_type, destination_space_id, and description are required")
	}

	if !validScamTypes[input.ScamType] {
		return ScamReport{}, ErrInvalidScamType
	}

	if input.PlatformUsed != "" && !validPlatforms[input.PlatformUsed] {
		return ScamReport{}, ErrInvalidPlatform
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
		return ScamReport{}, fmt.Errorf("invalid destination_space_id: %w", err)
	}

	var originSpaceID pgtype.UUID
	if input.OriginSpaceID != "" {
		if err := originSpaceID.Scan(input.OriginSpaceID); err != nil {
			return ScamReport{}, fmt.Errorf("invalid origin_space_id: %w", err)
		}
	}

	var originCommunity *string
	if input.OriginCommunity != "" {
		originCommunity = &input.OriginCommunity
	}

	var platformUsed *string
	if input.PlatformUsed != "" {
		platformUsed = &input.PlatformUsed
	}

	var amountRange *string
	if input.AmountRange != "" {
		amountRange = &input.AmountRange
	}

	var approximateDate pgtype.Date
	if input.ApproximateDate != "" {
		if err := approximateDate.Scan(input.ApproximateDate); err != nil {
			return ScamReport{}, fmt.Errorf("invalid approximate_date: %w", err)
		}
	}

	return ScamReport{
		ScamType:              input.ScamType,
		DestinationSpaceID:    destSpaceID,
		OriginSpaceID:         originSpaceID,
		OriginCommunity:       originCommunity,
		PlatformUsed:          platformUsed,
		ApproximateDate:       approximateDate,
		AmountRange:           amountRange,
		Description:           input.Description,
		ReportedToAuthorities: input.ReportedToAuthorities,
	}, nil
}

// Update edits a scam report's content fields. Ownership is enforced unless
// the caller holds scam_alert.edit_any. Review status, pins and moderation
// metadata are immutable through this endpoint.
func (s *Service) Update(ctx context.Context, id, tenantID, callerID pgtype.UUID, input UpdateReportInput) (ScamReportDTO, error) {
	existing, err := s.store.GetReportForUpdate(ctx, id, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ScamReportDTO{}, pgx.ErrNoRows
		}

		return ScamReportDTO{}, fmt.Errorf("scamalert: update: %w", err)
	}

	editAny := s.rbac != nil && s.rbac.HasPermission(ctx, callerID, "scam_alert.edit_any")
	if !editAny && (!existing.ReporterID.Valid || existing.ReporterID != callerID) {
		return ScamReportDTO{}, ErrEditForbidden
	}

	report, err := buildScamReport(input)
	if err != nil {
		return ScamReportDTO{}, err
	}

	row, err := s.store.UpdateReport(ctx, id, tenantID, report)
	if err != nil {
		return ScamReportDTO{}, fmt.Errorf("scamalert: update: %w", err)
	}

	return rowToDTO(row), nil
}

// IsSubscribed reports whether a user is subscribed to scam alerts for a
// destination space.
func (s *Service) IsSubscribed(ctx context.Context, userID, destSpaceID pgtype.UUID) (bool, error) {
	return s.store.IsSubscribed(ctx, userID, destSpaceID)
}

// Get returns a single confirmed scam report (public view).
func (s *Service) Get(ctx context.Context, tenantID, id pgtype.UUID) (ScamReportDTO, error) {
	row, err := s.store.GetReport(ctx, id, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ScamReportDTO{}, pgx.ErrNoRows
		}

		return ScamReportDTO{}, fmt.Errorf("scamalert: get: %w", err)
	}

	return rowToDTO(row), nil
}

// List returns a paginated, filtered list of confirmed scam reports.
func (s *Service) List(ctx context.Context, destSpaceID pgtype.UUID, scamType string, page, limit int32) ([]ScamReportDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	rows, total, err := s.store.ListReports(ctx, ListReportsFilter{

		DestinationSpaceID: destSpaceID,
		ScamType:           scamType,
		Limit:              limit,
		Offset:             offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("scamalert: list: %w", err)
	}

	dtos := make([]ScamReportDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, rowToDTO(r))
	}

	return dtos, total, nil
}

// Subscribe adds a scam-alert subscription for a user + destination space.
func (s *Service) Subscribe(ctx context.Context, userID, tenantID, destSpaceID pgtype.UUID) error {
	if err := s.store.Subscribe(ctx, userID, tenantID, destSpaceID); err != nil {
		return fmt.Errorf("scamalert: subscribe: %w", err)
	}

	return nil
}

// Unsubscribe removes a scam-alert subscription for a user + destination space.
func (s *Service) Unsubscribe(ctx context.Context, userID, destSpaceID pgtype.UUID) error {
	if err := s.store.Unsubscribe(ctx, userID, destSpaceID); err != nil {
		return fmt.Errorf("scamalert: unsubscribe: %w", err)
	}

	return nil
}

// AdminList returns all reports (any status) for the moderation queue.
// Supports ?status=&dest=&scam_type=&q=&sort=&order= per the DataTable
// contract (api-routes-catalog.md).
func (s *Service) AdminList(ctx context.Context, status string, destSpaceID pgtype.UUID, scamType, query, sortKey, order string, page, limit int32) ([]ScamReportDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	rows, total, err := s.store.AdminListReports(ctx, AdminListReportsFilter{

		Status:             status,
		DestinationSpaceID: destSpaceID,
		ScamType:           scamType,
		Query:              query,
		Sort:               sortKey,
		Order:              order,
		Limit:              limit,
		Offset:             offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("scamalert: admin list: %w", err)
	}

	dtos := make([]ScamReportDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, rowToDTO(r))
	}

	return dtos, total, nil
}

// Review updates the status of a report (confirm, debunk, or mark duplicate).
// The reviewerID is stamped as reviewed_by. §81 status machine:
// under_review → confirmed | debunked | duplicate.
// // When the status is "confirmed", a scam_alert:notify_subscribers Asynq task
// is enqueued fire-and-forget (Rule 8) to fan out notifications to
// subscribers of the report's destination space, award the Community
// Guardian badge to the reporter (§64), and enqueue wallet:award_points
// for the reporter's contribution.
func (s *Service) Review(ctx context.Context, id pgtype.UUID, status, note string, reviewerID pgtype.UUID) error {
	if !validReviewStatuses[status] {
		return ErrInvalidStatus
	}

	// §81 legal boundary + ADR-088: a confirmed report must always carry a
	// moderator note. For AI-generated reports the note doubles as the HITL
	// record — the store stamps hitl_reviewed_by on first review.
	if status == "confirmed" && strings.TrimSpace(note) == "" {
		return ErrModeratorNoteRequired
	}

	if err := s.store.UpdateReportStatus(ctx, id, status, note, reviewerID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("scamalert: review: %w", err)
	}

	// §81: on confirm, enqueue the notification + badge fan-out task.
	// Fire-and-forget — never block the review response (Rule 8).
	if status == "confirmed" && s.enqueuer != nil {
		payload, err := json.Marshal(struct {
			ReportID string `json:"report_id"`
		}{
			ReportID: id.String(),
		})
		if err != nil {
			slog.Error("scamalert: review: marshal notify payload failed", "error", err)
		} else if err := s.enqueuer.Enqueue(TaskNotifySubscribers, payload); err != nil {
			slog.Error("scamalert: review: enqueue notify task failed", "error", err)
		}
	}

	return nil
}

// NotifySubscribers is the Asynq task processor for
// scam_alert:notify_subscribers. It is called fire-and-forget after a
// moderator confirms a report. It:
// 1. Loads the confirmed report's destination_space_id + reporter_id.
// 2. Queries all subscribers for that destination.
// 3. Dispatches a scam_alert.confirmed notification to each subscriber
// via the notification service (§35).
// 4. Awards the Community Guardian badge to the reporter via the
// gamification service (§64) — EvaluateForWallet with activity key
// "scam_alert.confirmed".
// 5. Enqueues wallet:award_points for the reporter's contribution.
// // All steps are nil-safe: if the notifier/awarder/enqueuer is not wired,
// the method logs a warning and continues. This makes the feature
// gracefully degrade in tests and development.
func (s *Service) NotifySubscribers(ctx context.Context, reportID, tenantID pgtype.UUID) error {
	destSpaceID, reporterID, scamType, description, err := s.store.GetReportForNotification(ctx, reportID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Warn("scamalert: notify: report not found or not confirmed", "report_id", reportID.String())
			return nil
		}

		return fmt.Errorf("scamalert: notify: get report: %w", err)
	}

	// 1. Fan out notifications to subscribers.
	if s.notifier != nil {
		subscribers, err := s.store.ListSubscribersByDestination(ctx, destSpaceID)
		if err != nil {
			slog.Error("scamalert: notify: list subscribers failed", "error", err)
		} else {
			title := "Scam alert confirmed: " + scamType
			body := excerpt(description, 200)
			actionURL := "/scam-alerts/" + reportID.String()

			for _, subID := range subscribers {
				if err := s.notifier.CreateNotification(
					ctx, subID,
					"scam_alert.confirmed", "system", pgtype.UUID{},
					"scam_report", reportID,
					title, body, actionURL,
				); err != nil {
					slog.Warn("scamalert: notify: create notification failed", "subscriber_id", subID.String(), "error", err)
				}
			}

			slog.Info("scamalert: notify: dispatched notifications", "report_id", reportID.String(), "subscriber_count", len(subscribers))
		}
	} else {
		slog.Warn("scamalert: notify: notifier not wired, skipping subscriber fan-out")
	}

	// 2. Record the scam_alert.confirmed activity for the reporter (§64).
	// The wallet:process_activity_event task evaluates badge/challenge
	// triggers for the key — community_guardian (threshold 1) is awarded
	// on the reporter's first confirmed report. Emitting the activity
	// event (rather than calling the evaluator directly) is what makes
	// the activity_count trigger see this report. The reporter_id may be
	// invalid for anonymous reports — skip in that case.
	if s.activity != nil && reporterID.Valid {
		event, err := s.activity.EmitActivityEvent(ctx, reporterID, wallet.EmitActivityEventInput{
			ActivityKey: wallet.ActivityKeyScamAlertConfirmed,
			Reference:   "scam_alert:" + reportID.String(),
			Payload:     map[string]interface{}{"scam_type": scamType},
		})
		if err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				slog.Warn("scamalert: notify: emit activity event failed", "reporter_id", reporterID.String(), "error", err)
			}
		} else if err := s.activity.EnqueueProcessActivityEvent(ctx, event.ID, tenantID); err != nil {
			slog.Warn("scamalert: notify: enqueue activity event failed", "reporter_id", reporterID.String(), "error", err)
		}
	}

	// 3. Enqueue wallet:award_points for the reporter's contribution.
	// The points amount is configured via the wallet activity_rules
	// table. Here we enqueue a direct award with a fixed amount as a
	// baseline; the wallet service deduplicates via idempotency key.
	if s.enqueuer != nil && reporterID.Valid {
		payload, err := json.Marshal(struct {
			UserID      string `json:"user_id"`
			Points      int64  `json:"points"`
			Reason      string `json:"reason"`
			ReferenceID string `json:"reference_id,omitempty"`
		}{

			UserID:      reporterID.String(),
			Points:      50, // baseline points for a confirmed scam report
			Reason:      "scam_alert.confirmed",
			ReferenceID: reportID.String(),
		})
		if err != nil {
			slog.Error("scamalert: notify: marshal award payload failed", "error", err)
		} else if err := s.enqueuer.Enqueue(TaskAwardPoints, payload); err != nil {
			slog.Error("scamalert: notify: enqueue award points failed", "error", err)
		}
	}

	return nil
}

// excerpt returns the first n characters of s, suffixed with "…" if truncated.
func excerpt(s string, n int) string {
	if len(s) <= n {
		return s
	}

	return s[:n] + "…"
}

// Pin pins a confirmed report to a space.
func (s *Service) Pin(ctx context.Context, id, spaceID pgtype.UUID) error {
	if err := s.store.PinReport(ctx, id, spaceID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("scamalert: pin: %w", err)
	}

	return nil
}

// Unpin removes the pin from a report.
func (s *Service) Unpin(ctx context.Context, id pgtype.UUID) error {
	if err := s.store.UnpinReport(ctx, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("scamalert: unpin: %w", err)
	}

	return nil
}

// rowToDTO converts a ScamReportRow (joined query result) to a ScamReportDTO.
func rowToDTO(r ScamReportRow) ScamReportDTO {
	dto := ScamReportDTO{
		ID:                    r.ID.String(),
		ScamType:              r.ScamType,
		Description:           r.Description,
		ReportedToAuthorities: r.ReportedToAuthorities,
		Status:                r.Status,
		IsPinned:              r.IsPinned,
		CreatedAt:             formatTs(r.CreatedAt),
		UpdatedAt:             formatTs(r.UpdatedAt),
		ReviewedAt:            formatTs(r.ReviewedAt),
	}
	if r.ReporterID.Valid {
		dto.ReporterID = r.ReporterID.String()
	}

	if r.DestinationSpaceID.Valid {
		dto.DestinationSpaceID = r.DestinationSpaceID.String()
	}

	if r.DestSpaceName != nil {
		dto.DestinationSpaceName = *r.DestSpaceName
	}

	if r.DestSpaceCode != nil {
		dto.DestinationSpaceCode = *r.DestSpaceCode
	}

	if r.OriginSpaceID.Valid {
		dto.OriginSpaceID = r.OriginSpaceID.String()
	}

	if r.OriginSpaceName != nil {
		dto.OriginSpaceName = *r.OriginSpaceName
	}

	if r.OriginSpaceCode != nil {
		dto.OriginSpaceCode = *r.OriginSpaceCode
	}

	if r.OriginCommunity != nil {
		dto.OriginCommunity = *r.OriginCommunity
	}

	if r.PlatformUsed != nil {
		dto.PlatformUsed = *r.PlatformUsed
	}

	if r.ApproximateDate.Valid {
		dto.ApproximateDate = r.ApproximateDate.Time.Format("2006-01-02")
	}

	if r.AmountRange != nil {
		dto.AmountRange = *r.AmountRange
	}

	if r.ModeratorNote != nil {
		dto.ModeratorNote = *r.ModeratorNote
	}

	if r.PinnedSpaceID.Valid {
		dto.PinnedSpaceID = r.PinnedSpaceID.String()
	}

	if r.ReviewedBy.Valid {
		dto.ReviewedBy = r.ReviewedBy.String()
	}

	dto.AiGenerated = r.AiGenerated
	dto.AiGeneratedAt = formatTs(r.AiGeneratedAt)
	dto.HitlReviewedAt = formatTs(r.HitlReviewedAt)

	if r.AiModel != nil {
		dto.AiModel = *r.AiModel
	}

	if r.HitlReviewedBy.Valid {
		dto.HitlReviewedBy = r.HitlReviewedBy.String()
	}

	if r.ReporterName != nil {
		dto.ReporterName = *r.ReporterName
	}

	return dto
}

// formatTs renders a pgtype.Timestamptz as RFC 3339 UTC, or "" if NULL.
func formatTs(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}

	return t.Time.UTC().Format(time.RFC3339)
}
