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

	"github.com/jedafe/japa/internal/cdn"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/wallet"
	"github.com/valkey-io/valkey-go/valkeycompat"
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
	// IsReporter is set on public list/get responses when the caller is the
	// report's submitter — drives reporter-only UI (edit, §22 appeal). The
	// raw reporter_id is withheld from non-reporters on public surfaces.
	IsReporter    bool   `json:"is_reporter,omitempty"`
	PinnedSpaceID string `json:"pinned_space_id,omitempty"`
	ReviewedBy    string `json:"reviewed_by,omitempty"`
	ReviewedAt    string `json:"reviewed_at,omitempty"`
	// BUG-002: accused-entity linkage — moderation context, redacted on
	// public surfaces for non-reporters like reporter identity.
	SubjectType         string `json:"subject_type,omitempty"`
	SubjectID           string `json:"subject_id,omitempty"`
	SubjectLabel        string `json:"subject_label,omitempty"`
	PlatformRef         string `json:"platform_ref,omitempty"`
	DuplicateOfReportID string `json:"duplicate_of_report_id,omitempty"`
	// ADR-088 universal AI-generation fields (admin review surfaces).
	// AiGenerated is a *bool so the public redaction path can omit the key
	// entirely — §81 strips ai_*/hitl_* provenance for every caller except
	// the reporter; a plain false would leak (and misreport) the flag.
	AiGenerated    *bool  `json:"ai_generated,omitempty"`
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
	// BUG-002: optional accused-entity linkage (prefilled by profile/org/
	// service CTAs). subject_type is user|organization|service|other;
	// subject_label is a display-name snapshot for the review queue.
	SubjectType  string `json:"subject_type"`
	SubjectID    string `json:"subject_id"`
	SubjectLabel string `json:"subject_label"`
	// PlatformRef is the external contact reference — social URL, number,
	// site URL, or email matching the external platform selected.
	PlatformRef string `json:"platform_ref"`
}

// UpdateReportInput is the request body for PATCH /scam-alerts/:id.
// It mirrors SubmitReportInput but omits immutable ownership fields
// (reporter_id, status, moderator_note, reviewed_by, etc.).
type UpdateReportInput SubmitReportInput

// ReviewInput is the request body for PATCH /admin/scam-alerts/:id.
type ReviewInput struct {
	Status              string `json:"status"` // confirmed|debunked|duplicate
	ModeratorNote       string `json:"moderator_note"`
	DuplicateOfReportID string `json:"duplicate_of_report_id"`
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

// FeatureChecker checks whether a feature flag is enabled for a platform.
// Satisfied by *features.Checker via structural typing — defined locally to
// avoid importing the features package (keeps the dependency graph clean).
type FeatureChecker interface {
	IsEnabled(ctx context.Context, key string) bool
}

// Task name constants for Asynq tasks enqueued by this service.
const (
	TaskNotifySubscribers = "scam_alert:notify_subscribers"
	TaskAwardPoints       = "wallet:award_points"
)

// deduper is the SetNX surface used for the subscriber-notification dedup
// guard (valkeycompat.Cmdable is satisfied by valkey.Client.Redis()).
type deduper interface {
	SetNX(ctx context.Context, key string, value any, expiration time.Duration) *valkeycompat.BoolCmd
}

// Service holds the business logic for the Scam Alert Board.
type Service struct {
	store    *Store
	settings *config.Settings
	notifier Notifier
	activity activityEmitter
	enqueuer TaskEnqueuer
	checker  FeatureChecker
	rbac     permissionChecker
	vk       deduper
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

// SetValkey injects the cache client used for the
// jc:scam_alert:notified:{user_id}:{report_id} dedup guard (24h TTL —
// valkey-keys-catalog). Optional — nil-safe.
func (s *Service) SetValkey(c deduper) { s.vk = c }

// Enabled checks whether the scam_alert feature is enabled for the given
// platform. Delegates to the features.Checker (feature_flags table) when
// available, falling back to system_settings for backward compatibility.
func (s *Service) Enabled(ctx context.Context) bool {
	if s.checker != nil {
		return s.checker.IsEnabled(ctx, "scam_alert.enabled")
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
	// Platform classification is binary: 'internal' = the JapaConnect
	// ecosystem itself (the accused entity is captured via
	// subject_type/subject_id/subject_label), every other value names an
	// external channel.
	"internal":  true,
	"whatsapp":  true,
	"telegram":  true,
	"facebook":  true,
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
	"internal":  "JapaConnect (internal)",
	"whatsapp":  "WhatsApp",
	"telegram":  "Telegram",
	"facebook":  "Facebook",
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

	// Explicit platform order: 'internal' (this platform) first, 'other'
	// last — per the internal/external classification flow.
	platformOrder := []string{
		"internal", "facebook", "instagram", "linkedin",
		"telegram", "whatsapp", "email", "website", "other",
	}
	platforms := make([]Option, 0, len(validPlatforms))
	for _, v := range platformOrder {
		platforms = append(platforms, Option{Value: v, Label: platformLabels[v]})
	}

	return SchemaOptions{ScamTypes: scamTypes, Platforms: platforms}
}

// validReviewStatuses are the status values a moderator may set via Review
// (§81 status machine: under_review → confirmed | debunked | duplicate).
// validSubjectTypes are the accused-entity kinds a report may reference
// (BUG-002). Optional — empty means the report is about a pattern, not an
// on-platform entity.
var validSubjectTypes = map[string]bool{
	"user":         true,
	"organization": true,
	"service":      true,
	"other":        true,
}

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

// ErrInvalidDestination is returned when destination_space_id does not
// resolve to a live country-level space (geo-aware pattern, §§79–101).
var ErrInvalidDestination = errors.New("invalid destination_space_id")

// ErrInvalidOrigin is returned when origin_space_id does not resolve to a
// live country-level space.
var ErrInvalidOrigin = errors.New("invalid origin_space_id")

// ErrInvalidDuplicateTarget is returned when duplicate_of_report_id does
// not reference an existing report.
var ErrInvalidDuplicateTarget = errors.New("duplicate_of_report_id does not reference an existing report")

// ErrSubjectRequired is returned when platform_used is 'internal' but no
// accused on-platform entity was supplied — an internal report is always
// about a JapaConnect user/org/service.
var ErrSubjectRequired = errors.New("subject required for internal reports")

// ErrInvalidSubjectType is returned when subject_type is not in the
// BUG-002 enum.
var ErrInvalidSubjectType = errors.New("invalid subject_type")

// ErrFutureDate is returned when approximate_date is in the future.
var ErrFutureDate = errors.New("approximate_date cannot be in the future")

// ErrModeratorNoteRequired is returned when a report is confirmed without a
// moderator note. §81 legal boundary: a confirmed report must always carry a
// moderator note — it is also the HITL record for AI-generated reports
// (ADR-088).
var ErrModeratorNoteRequired = errors.New("moderator note required")

// Abuse-control caps on scam report free-text fields.
const (
	MaxDescriptionRunes     = 10000
	MaxOriginCommunityRunes = 200
	MaxAmountRangeRunes     = 100
	MaxModeratorNoteRunes   = 5000
)

// validateGeoSpaces enforces the geo-aware pattern (§§79–101): destination
// is always required and must be a country-level space; origin is optional
// but must resolve to a country space when provided.
func (s *Service) validateGeoSpaces(ctx context.Context, report ScamReport) error {
	ok, err := s.store.CountrySpaceExists(ctx, report.DestinationSpaceID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidDestination
	}

	if report.OriginSpaceID.Valid {
		ok, err = s.store.CountrySpaceExists(ctx, report.OriginSpaceID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrInvalidOrigin
		}
	}

	return nil
}

// Submit creates a new scam report in 'under_review' status. reporterID may
// be zero-valued (invalid) for anonymous reports when anonymous reports are
// allowed.
func (s *Service) Submit(ctx context.Context, reporterID pgtype.UUID, input SubmitReportInput) (ScamReportDTO, error) {
	report, err := buildScamReport(UpdateReportInput(input))
	if err != nil {
		return ScamReportDTO{}, err
	}

	if err := s.validateGeoSpaces(ctx, report); err != nil {
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

// permissionChecker is the subset of rbac.RBAC used for edit authorization.
type permissionChecker interface {
	HasPermission(ctx context.Context, userID pgtype.UUID, permKey string) bool
}

// buildScamReport validates a submit/update input and converts it into a
// ScamReport value. The caller is responsible for setting ID, ReporterID
// and Status for create, and for supplying the report id/platform for update.
func buildScamReport(input UpdateReportInput) (ScamReport, error) {
	if input.ScamType == "" || input.DestinationSpaceID == "" || input.Description == "" {
		return ScamReport{}, errors.New("scam_type, destination_space_id, and description are required")
	}

	if len([]rune(input.Description)) > MaxDescriptionRunes {
		return ScamReport{}, fmt.Errorf("description must be at most %d characters", MaxDescriptionRunes)
	}

	if len([]rune(input.OriginCommunity)) > MaxOriginCommunityRunes {
		return ScamReport{}, fmt.Errorf("origin_community must be at most %d characters", MaxOriginCommunityRunes)
	}

	if len([]rune(input.AmountRange)) > MaxAmountRangeRunes {
		return ScamReport{}, fmt.Errorf("amount_range must be at most %d characters", MaxAmountRangeRunes)
	}

	if !validScamTypes[input.ScamType] {
		return ScamReport{}, ErrInvalidScamType
	}

	if input.PlatformUsed != "" && !validPlatforms[input.PlatformUsed] {
		return ScamReport{}, ErrInvalidPlatform
	}

	// Sanitize on write (mirror content/store.go:460): description is the
	// rich-text field (whitelist HTML), origin_community/amount_range are
	// plain text.
	// BUG-015: description is a plain-text field — SanitizeText strips
	// markup entirely; SanitizeHTML persists a full <html><body> document
	// wrapper that the plaintext renderers surface literally.
	input.Description = content.SanitizeText(input.Description)
	input.OriginCommunity = content.SanitizeText(input.OriginCommunity)
	input.AmountRange = content.SanitizeText(input.AmountRange)

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
		if approximateDate.Valid && approximateDate.Time.After(time.Now().AddDate(0, 0, 1)) {
			return ScamReport{}, ErrFutureDate
		}
	}

	// Optional accused-entity linkage. For 'internal' reports (the scam
	// happened on JapaConnect itself) the on-platform entity is required —
	// otherwise the report has no subject to moderate against.
	if platformUsed != nil && *platformUsed == "internal" &&
		input.SubjectType == "" && input.SubjectID == "" && input.SubjectLabel == "" {
		return ScamReport{}, ErrSubjectRequired
	}

	var subjectType *string
	var subjectLabel *string
	var subjectID pgtype.UUID
	if input.SubjectType != "" {
		if !validSubjectTypes[input.SubjectType] {
			return ScamReport{}, ErrInvalidSubjectType
		}
		subjectType = &input.SubjectType
	}
	if input.SubjectID != "" {
		if err := subjectID.Scan(input.SubjectID); err != nil {
			return ScamReport{}, fmt.Errorf("invalid subject_id: %w", err)
		}
	}
	var platformRef *string
	if input.PlatformRef != "" {
		ref := content.SanitizeText(input.PlatformRef)
		if len([]rune(ref)) > 500 {
			ref = string([]rune(ref)[:500])
		}
		platformRef = &ref
	}

	if input.SubjectLabel != "" {
		lbl := content.SanitizeText(input.SubjectLabel)
		if len([]rune(lbl)) > 200 {
			lbl = string([]rune(lbl)[:200])
		}
		subjectLabel = &lbl
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
		SubjectType:           subjectType,
		SubjectID:             subjectID,
		SubjectLabel:          subjectLabel,
		PlatformRef:           platformRef,
	}, nil
}

// Update edits a scam report's content fields. Ownership is enforced unless
// the caller holds scam_alert.edit_any. Review status, pins and moderation
// metadata are immutable through this endpoint.
func (s *Service) Update(ctx context.Context, id, callerID pgtype.UUID, input UpdateReportInput) (ScamReportDTO, error) {
	existing, err := s.store.GetReportForUpdate(ctx, id)
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

	if err := s.validateGeoSpaces(ctx, report); err != nil {
		return ScamReportDTO{}, err
	}

	row, err := s.store.UpdateReport(ctx, id, report)
	if err != nil {
		return ScamReportDTO{}, fmt.Errorf("scamalert: update: %w", err)
	}

	// BUG-001: a reporter (non-edit_any) editing an already-decided report
	// must send it back through moderation — otherwise confirmed, publicly
	// published content could be rewritten after review. Moderator edits
	// (edit_any) leave the status untouched.
	if !editAny && existing.Status != "under_review" {
		if rerr := s.store.ResetReportReview(ctx, id); rerr != nil {
			slog.Error("scamalert: reset review after reporter edit failed", "id", id, "error", rerr)
		}
		// Clear the stale review fields on the returned row too — the DTO
		// must not claim a moderator reviewed content that is pending again.
		row.ReviewedBy = pgtype.UUID{}
		row.ReviewedAt = pgtype.Timestamptz{}
		row.Status = "under_review"
		cdn.PurgePublicPaths(ctx, []string{"scam_alert:" + id.String()}, "/scam-alerts/"+id.String(), "/scam-alerts")
	}

	return rowToDTO(row), nil
}

// IsSubscribed reports whether a user is subscribed to scam alerts for a
// destination space.
func (s *Service) IsSubscribed(ctx context.Context, userID, destSpaceID pgtype.UUID) (bool, error) {
	return s.store.IsSubscribed(ctx, userID, destSpaceID)
}

// Get returns a single confirmed scam report (public view).
func (s *Service) Get(ctx context.Context, id pgtype.UUID) (ScamReportDTO, error) {
	row, err := s.store.GetReport(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ScamReportDTO{}, pgx.ErrNoRows
		}

		return ScamReportDTO{}, fmt.Errorf("scamalert: get: %w", err)
	}

	return rowToDTO(row), nil
}

// GetForViewer returns the public view of a report for the given caller.
// Non-confirmed reports (under_review/debunked/duplicate) are hidden from
// the public but remain readable by their own reporter — the §22 appeal
// surface on the detail page needs the reporter to reach debunked and
// duplicate reports.
func (s *Service) GetForViewer(ctx context.Context, id, callerID pgtype.UUID) (ScamReportDTO, error) {
	dto, err := s.Get(ctx, id)
	if err == nil || !callerID.Valid {
		return dto, err
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return dto, err
	}

	row, rerr := s.store.GetReportForUpdate(ctx, id)
	if rerr != nil {
		if errors.Is(rerr, pgx.ErrNoRows) {
			return ScamReportDTO{}, pgx.ErrNoRows
		}

		return ScamReportDTO{}, fmt.Errorf("scamalert: get for viewer: %w", rerr)
	}

	// The reporter sees their own report in any state; moderators and
	// edit_any holders see every state too — they already list pending
	// reports in the /admin/scam-alerts queue and PATCH them, so hiding the
	// single-report read would be an inconsistent denial (and it breaks the
	// main-app edit page for non-reporter moderators).
	if s.rbac != nil &&
		(s.rbac.HasPermission(ctx, callerID, "scam_alert.review") ||
			s.rbac.HasPermission(ctx, callerID, "scam_alert.edit_any")) {
		return rowToDTO(row), nil
	}

	if !row.ReporterID.Valid || row.ReporterID != callerID {
		return ScamReportDTO{}, pgx.ErrNoRows
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

// ListByOffset is the offset-paginated variant of List. The §47 public API
// contract is limit/offset — page-flooring a misaligned offset would
// silently shift the result window.
func (s *Service) ListByOffset(ctx context.Context, destSpaceID pgtype.UUID, scamType string, limit, offset int32) ([]ScamReportDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if offset < 0 {
		offset = 0
	}

	rows, total, err := s.store.ListReports(ctx, ListReportsFilter{
		DestinationSpaceID: destSpaceID,
		ScamType:           scamType,
		Limit:              limit,
		Offset:             offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("scamalert: list by offset: %w", err)
	}

	dtos := make([]ScamReportDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, rowToDTO(r))
	}

	return dtos, total, nil
}

// Subscribe adds a scam-alert subscription for a user + destination space.
func (s *Service) Subscribe(ctx context.Context, userID, destSpaceID pgtype.UUID) error {
	if err := s.store.Subscribe(ctx, userID, destSpaceID); err != nil {
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
func (s *Service) Review(ctx context.Context, id pgtype.UUID, status, note, duplicateOfReportID string, reviewerID pgtype.UUID) error {
	if !validReviewStatuses[status] {
		return ErrInvalidStatus
	}

	// §81 legal boundary + ADR-088: a confirmed report must always carry a
	// moderator note. For AI-generated reports the note doubles as the HITL
	// record — the store stamps hitl_reviewed_by on first review.
	if status == "confirmed" && strings.TrimSpace(note) == "" {
		return ErrModeratorNoteRequired
	}

	// BUG-008: capture the previous status so the fan-out task only
	// notifies subscribers on a real transition INTO 'confirmed' — a
	// re-confirm of an already-confirmed report must not re-alert.
	prevStatus := ""
	if existing, gerr := s.store.GetReportForUpdate(ctx, id); gerr == nil {
		prevStatus = existing.Status
	}

	var duplicateOf pgtype.UUID
	if duplicateOfReportID != "" {
		if err := duplicateOf.Scan(duplicateOfReportID); err != nil {
			return fmt.Errorf("scamalert: review: invalid duplicate_of_report_id: %w", err)
		}
		// The duplicate link must point at a real report — storing a dead
		// reference would make the moderation note meaningless.
		if _, derr := s.store.GetReportForUpdate(ctx, duplicateOf); derr != nil {
			return ErrInvalidDuplicateTarget
		}
	}

	if err := s.store.UpdateReportStatus(ctx, id, status, note, reviewerID, duplicateOf); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("scamalert: review: %w", err)
	}

	// ADR-134: review transitions change the public report page — purge it.
	cdn.PurgePublicPaths(ctx, []string{"scam_alert:" + id.String()}, "/scam-alerts/"+id.String(), "/scam-alerts")

	// §81: on confirm, enqueue the notification + badge fan-out task.
	// Fire-and-forget — never block the review response (Rule 8).
	if s.enqueuer != nil {
		payload, err := json.Marshal(struct {
			ReportID   string `json:"report_id"`
			PrevStatus string `json:"prev_status"`
		}{
			ReportID:   id.String(),
			PrevStatus: prevStatus,
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
func (s *Service) NotifySubscribers(ctx context.Context, reportID pgtype.UUID, prevStatus string) error {
	destSpaceID, reporterID, scamType, description, status, err := s.store.GetReportForNotification(ctx, reportID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Warn("scamalert: notify: report not found or not confirmed", "report_id", reportID.String())
			return nil
		}

		return fmt.Errorf("scamalert: notify: get report: %w", err)
	}

	// BUG-006: notify the reporter of the review outcome — previously only
	// destination subscribers heard anything, so debunked reporters never
	// learned their appeal path existed. Anonymous reports (NULL
	// reporter_id) have nobody to notify.
	if s.notifier != nil && reporterID.Valid && status != "under_review" {
		title := "Your scam report was " + status
		body := "A moderator reviewed your " + scamType + " report"
		if status == "confirmed" {
			body = "Thanks for helping protect the community — your report was confirmed and is now public."
		}
		if nerr := s.notifier.CreateNotification(
			ctx, reporterID,
			"scam_alert.reviewed", "system", pgtype.UUID{},
			"scam_report", reportID,
			title, body, "/scam-alerts/"+reportID.String(),
		); nerr != nil {
			slog.Warn("scamalert: notify: reporter notification failed", "reporter_id", reporterID.String(), "error", nerr)
		}
	}

	// Subscriber fan-out only on a real transition INTO 'confirmed'.
	if status != "confirmed" || prevStatus == "confirmed" {
		return nil
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
				// The reporter already received the 'reviewed' outcome
				// notification — skip them in the subscriber fan-out so a
				// reporter who is also subscribed isn't notified twice.
				if subID == reporterID {
					continue
				}

				// Dedup guard — jc:scam_alert:notified:{user_id}:{report_id}
				// (valkey-keys-catalog, 24h TTL). Asynq retries must not
				// re-notify a subscriber who already got this alert.
				if s.vk != nil {
					key := "jc:scam_alert:notified:" + subID.String() + ":" + reportID.String()
					set, derr := s.vk.SetNX(ctx, key, "1", 24*time.Hour).Result()
					if derr != nil {
						slog.Warn("scamalert: notify: dedup guard failed, sending anyway", "subscriber_id", subID.String(), "error", derr)
					} else if !set {
						continue
					}
				}

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
		} else if err := s.activity.EnqueueProcessActivityEvent(ctx, event.ID); err != nil {
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
	// BUG-010: validate the pin target resolves to a live space — the body
	// previously accepted any UUID and silently stored dead references.
	ok, err := s.store.SpaceExists(ctx, spaceID)
	if err != nil {
		return fmt.Errorf("scamalert: pin: %w", err)
	}
	if !ok {
		return ErrInvalidDestination
	}

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
		ID:       r.ID.String(),
		ScamType: r.ScamType,
		// BUG-015: strip any markup on read so legacy rows stored through
		// the old SanitizeHTML path (with the <html><body> wrapper) render
		// as clean plain text too.
		Description:           content.SanitizeText(r.Description),
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

	if r.SubjectType != nil {
		dto.SubjectType = *r.SubjectType
	}
	if r.SubjectID.Valid {
		dto.SubjectID = r.SubjectID.String()
	}
	if r.SubjectLabel != nil {
		dto.SubjectLabel = *r.SubjectLabel
	}
	if r.PlatformRef != nil {
		dto.PlatformRef = *r.PlatformRef
	}
	if r.DuplicateOfReportID.Valid {
		dto.DuplicateOfReportID = r.DuplicateOfReportID.String()
	}

	aiGen := r.AiGenerated
	dto.AiGenerated = &aiGen
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

// SubscriptionDTO is the API shape for a scam-alert subscription.
type SubscriptionDTO struct {
	DestinationSpaceID   string `json:"destination_space_id"`
	DestinationSpaceName string `json:"destination_space_name,omitempty"`
	DestinationSpaceCode string `json:"destination_space_code,omitempty"`
	CreatedAt            string `json:"created_at"`
}

// MyReports returns the caller's own reports in any status — powers the
// "My reports" tracking surface (BUG-016).
func (s *Service) MyReports(ctx context.Context, reporterID pgtype.UUID, page, limit int32) ([]ScamReportDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	rows, total, err := s.store.ListReportsByReporter(ctx, reporterID, limit, (page-1)*limit)
	if err != nil {
		return nil, 0, fmt.Errorf("scamalert: my reports: %w", err)
	}
	dtos := make([]ScamReportDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, rowToDTO(r))
	}
	return dtos, total, nil
}

// MySubscriptions returns the caller's scam-alert subscriptions with
// destination names (BUG-019).
func (s *Service) MySubscriptions(ctx context.Context, userID pgtype.UUID) ([]SubscriptionDTO, error) {
	rows, err := s.store.ListSubscriptionsByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("scamalert: my subscriptions: %w", err)
	}
	out := make([]SubscriptionDTO, 0, len(rows))
	for _, r := range rows {
		d := SubscriptionDTO{
			DestinationSpaceID: r.DestinationSpaceID.String(),
			CreatedAt:          formatTs(r.CreatedAt),
		}
		if r.DestinationSpaceName != nil {
			d.DestinationSpaceName = *r.DestinationSpaceName
		}
		if r.DestinationSpaceCode != nil {
			d.DestinationSpaceCode = *r.DestinationSpaceCode
		}
		out = append(out, d)
	}
	return out, nil
}

// SubscribeDest subscribes the caller to scam alerts for a destination space
// directly (without going through a report id) — the filtered-board
// subscribe CTA (BUG-019). Validates the country-level space.
func (s *Service) SubscribeDest(ctx context.Context, userID, destSpaceID pgtype.UUID) error {
	ok, err := s.store.CountrySpaceExists(ctx, destSpaceID)
	if err != nil {
		return fmt.Errorf("scamalert: subscribe dest: %w", err)
	}
	if !ok {
		return ErrInvalidDestination
	}
	return s.Subscribe(ctx, userID, destSpaceID)
}
