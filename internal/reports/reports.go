// Package reports — Content Reports & Flagging (JEOS Section 51).
//
// Users report content, comments, other users, spaces, and DM messages;
// reports feed the admin moderation queue (Section 22).
package reports

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/httpx"
	"github.com/jedafe/japa/internal/rbac"
	"github.com/jedafe/japa/internal/wallet"
	"github.com/jedafe/japa/internal/valkey"
	valkeycompat "github.com/valkey-io/valkey-go/valkeycompat"
)

// Valid entity_type values, matching the migration's CHECK constraint.
const (
	EntityTypeContent    = "content"
	EntityTypeComment    = "comment"
	EntityTypeUser       = "user"
	EntityTypeSpace      = "space"
	EntityTypeMessage    = "message"
	EntityTypeSource     = "source"
	EntityTypeForumReply = "forum_reply"
)

var validEntityTypes = map[string]bool{
	EntityTypeContent:    true,
	EntityTypeComment:    true,
	EntityTypeUser:       true,
	EntityTypeSpace:      true,
	EntityTypeMessage:    true,
	EntityTypeSource:     true,
	EntityTypeForumReply: true,
}

// defaultReasons is the fallback set used when moderation.report_reasons is
// unset or unparseable. The setting is the source of truth (Section 51 /
// Rule 16) — admins can add or remove reason codes without a deploy.
var defaultReasons = map[string]bool{
	"spam":            true,
	"misinformation":  true,
	"harassment":      true,
	"hate_speech":     true,
	"illegal_content": true,
	"self_harm":       true,
	"copyright":       true,
	"other":           true,
}

// reasonCodePattern constrains admin-configured reason codes so stored values
// stay slug-like (the frontend localizes them via report.reason.<code>).
var reasonCodePattern = regexp.MustCompile(`^[a-z0-9_]{2,48}$`)

// maxNotesLength bounds report notes (mirrored by the
// content_reports_notes_length CHECK constraint).
const maxNotesLength = 5000

// maxEvidenceURLs bounds the evidence_urls array per report.
const maxEvidenceURLs = 10

// validReasonCodes returns the admin-configured reason set, falling back to
// defaultReasons when the setting is absent or yields no usable codes.
func (s *Service) validReasonCodes(ctx context.Context) map[string]bool {
	if s.settings == nil {
		return defaultReasons
	}

	raw, err := s.settings.GetSetting(ctx, "moderation.report_reasons")
	if err != nil || strings.TrimSpace(raw) == "" {
		return defaultReasons
	}

	var codes []string
	if err := json.Unmarshal([]byte(raw), &codes); err != nil || len(codes) == 0 {
		return defaultReasons
	}

	set := make(map[string]bool, len(codes))
	for _, code := range codes {
		if reasonCodePattern.MatchString(code) {
			set[code] = true
		}
	}
	if len(set) == 0 {
		return defaultReasons
	}

	return set
}

var validStatuses = map[string]bool{
	"pending":   true,
	"reviewing": true,
	"actioned":  true,
	"dismissed": true,
}

// Sentinel errors distinguished by the handler layer.
var (
	ErrInvalidEntityType = errors.New("invalid entity_type")
	ErrInvalidReason     = errors.New("invalid reason")
	ErrInvalidStatus     = errors.New("invalid status")
	ErrAlreadyReported   = errors.New("already reported by this user")
	// ErrSelfReport blocks reporters from flagging their own account or
	// authored entities — self-reports only pollute the moderation queue.
	ErrSelfReport = errors.New("cannot report your own content")
	// ErrNotesTooLong rejects notes above maxNotesLength.
	ErrNotesTooLong = errors.New("notes exceeds maximum length")
	// ErrTooManyEvidenceURLs rejects evidence arrays above maxEvidenceURLs.
	ErrTooManyEvidenceURLs = errors.New("too many evidence URLs")
	// ErrNotParticipant blocks reporting a DM message in a conversation the
	// reporter is not part of.
	ErrNotParticipant = errors.New("not a participant in this conversation")
	// ErrNoTarget is returned when warn/ban is requested for an entity that
	// does not resolve to a user (space/source reports). The report stays
	// open so the moderator can pick an applicable action.
	ErrNoTarget = errors.New("action has no user target for this entity type")
	// ErrActionNotApplicable is returned when the action does not apply to
	// the reported entity type (e.g. remove on a user or space report).
	ErrActionNotApplicable = errors.New("action not applicable to this entity type")
	// ErrEntityNotFound rejects reports targeting entities that do not exist
	// (or were soft-deleted) — polymorphic entity_id can't carry a FK, so the
	// service probes the entity table instead of letting phantom rows into
	// the moderation queue.
	ErrEntityNotFound = errors.New("reported entity does not exist")
	// evidence_urls are rendered as <a href> in the admin/moderation UIs —
	// non-http(s) schemes are stored-XSS vectors against reviewers.
	ErrInvalidEvidenceURL = errors.New("evidence_urls must be valid http or https URLs")
)

// CreateReportInput holds parameters for submitting a report.
type CreateReportInput struct {
	EntityType   string
	EntityID     pgtype.UUID
	Reason       string
	Notes        *string
	EvidenceURLs []string
}

// Service provides report submission and moderation-queue access.
type Service struct {
	read     *gen.Queries
	write    *gen.Queries
	settings *config.Settings
	db       *db.DB
	valkey   valkeycompat.Cmdable
	rbac     *rbac.RBAC
	// activity emits wallet activity events — upheld-report points
	// penalties (ADR-136). Optional — nil-safe.
	activity wallet.ActivityEmitter
}

// SetActivityEmitter injects the wallet emitter (ADR-136).
func (s *Service) SetActivityEmitter(e wallet.ActivityEmitter) { s.activity = e }

// NewService creates a Service backed by the given database.
func NewService(database *db.DB, settings *config.Settings, vk valkeycompat.Cmdable) *Service {
	return &Service{
		read:     gen.New(database.Reader),
		write:    gen.New(database.Writer),
		settings: settings,
		db:       database,
		valkey:   vk,
	}
}

// DeleteReport removes a report (admin moderation action). It is a hard
// delete from content_reports because reports are audit-adjacent queue items,
// not user content, so Rule 11 (soft-delete user content) does not apply —
// but the deletion itself is written to audit_logs with the report snapshot
// so the action stays attributable (best-effort: an audit failure does not
// roll back the delete).
func (s *Service) DeleteReport(ctx context.Context, id, actorUserID pgtype.UUID) error {
	report, err := s.GetReport(ctx, id)
	if err != nil {
		return err
	}

	if err := s.write.DeleteReport(ctx, id); err != nil {
		return fmt.Errorf("delete report: %w", err)
	}

	targetType := "content_report"
	metadata, _ := json.Marshal(map[string]any{
		"entity_type": report.EntityType,
		"entity_id":   report.EntityID.String(),
		"reason":      report.Reason,
		"status":      report.Status,
	})
	if _, err := s.write.CreateAuditLog(ctx, gen.CreateAuditLogParams{
		ActorUserID: actorUserID,
		ActionType:  "admin.report.delete",
		TargetType:  &targetType,
		TargetID:    report.ID,
		Metadata:    metadata,
	}); err != nil {
		slog.Warn("delete report: audit log failed", "id", id.String(), "error", err)
	}

	return nil
}

// CreateReport submits a report. Returns ErrAlreadyReported if this reporter
// already reported the same entity (enforced by a partial unique index).
// For content entities, checks the auto-hide threshold and hides the content
// when the pending report count reaches it.
func (s *Service) CreateReport(ctx context.Context, reporterUserID pgtype.UUID, input CreateReportInput) (gen.ContentReport, error) {
	if !validEntityTypes[input.EntityType] {
		return gen.ContentReport{}, fmt.Errorf("%w: %s", ErrInvalidEntityType, input.EntityType)
	}

	// Polymorphic entity_id can't carry a FK — probe the entity table so
	// phantom (or already soft-deleted) targets can't pollute the queue.
	exists, err := s.entityExists(ctx, input.EntityType, input.EntityID)
	if err != nil {
		return gen.ContentReport{}, err
	}
	if !exists {
		return gen.ContentReport{}, ErrEntityNotFound
	}

	if !s.validReasonCodes(ctx)[input.Reason] {
		return gen.ContentReport{}, fmt.Errorf("%w: %s", ErrInvalidReason, input.Reason)
	}

	if input.Notes != nil && len(*input.Notes) > maxNotesLength {
		return gen.ContentReport{}, fmt.Errorf("%w: %d chars max %d", ErrNotesTooLong, len(*input.Notes), maxNotesLength)
	}

	if len(input.EvidenceURLs) > maxEvidenceURLs {
		return gen.ContentReport{}, fmt.Errorf("%w: max %d", ErrTooManyEvidenceURLs, maxEvidenceURLs)
	}

	for _, u := range input.EvidenceURLs {
		if !httpx.IsAbsoluteHTTPURL(u) {
			return gen.ContentReport{}, fmt.Errorf("%w: %q", ErrInvalidEvidenceURL, u)
		}
	}

	// Self-report guard: a reporter cannot flag their own account or an
	// entity they authored. ResolveTargetUser yields the offending user for
	// user/content/comment/message/forum_reply reports.
	if target, found, err := s.ResolveTargetUser(ctx, input.EntityType, input.EntityID); err == nil && found && target == reporterUserID {
		return gen.ContentReport{}, ErrSelfReport
	}

	report, err := s.write.CreateReport(ctx, gen.CreateReportParams{
		ReporterUserID: reporterUserID,
		EntityType:     input.EntityType,
		EntityID:       input.EntityID,
		Reason:         input.Reason,
		Notes:          input.Notes,
		EvidenceUrls:   input.EvidenceURLs,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// ON CONFLICT ... DO NOTHING suppressed the insert: duplicate report.
		return gen.ContentReport{}, ErrAlreadyReported
	}

	if err != nil {
		return gen.ContentReport{}, fmt.Errorf("creating report: %w", err)
	}

	// Auto-hide is handled by the moderation:auto_hide Asynq task enqueued by
	// the handler (fire-and-forget per Rule 8). The worker covers content,
	// comment, and message entity types and reads the same
	// moderation.auto_hide_report_count setting (default 5 per Section 51).

	return report, nil
}

// ListMyReports returns the caller's own submitted reports, newest first,
// with the resolved entity preview fields used by /dashboard/reports.
func (s *Service) ListMyReports(ctx context.Context, reporterUserID pgtype.UUID, limit, offset int32) ([]gen.ListReportsByReporterRow, error) {
	reports, err := s.read.ListReportsByReporter(ctx, gen.ListReportsByReporterParams{
		ReporterUserID: reporterUserID,
		PageLimit:      limit,
		PageOffset:     offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing my reports: %w", err)
	}

	return reports, nil
}

// CountReports returns the total number of reports for the platform (respecting
// optional status, entity_type, and q filters) so callers can paginate — the
// query mirrors ListReports's joins so totals always match page contents.
func (s *Service) CountReports(ctx context.Context, status, entityType, q *string) (int32, error) {
	total, err := s.read.CountReports(ctx, gen.CountReportsParams{
		Status:     status,
		EntityType: entityType,
		Q:          q,
	})
	if err != nil {
		return 0, fmt.Errorf("counting reports: %w", err)
	}

	return total, nil
}

// ListReports returns all reports for the platform (admin), optionally
// filtered by status and/or entity_type.
func (s *Service) ListReports(ctx context.Context, status, entityType, q *string, limit, offset int32) ([]gen.ListReportsRow, error) {
	reports, err := s.read.ListReports(ctx, gen.ListReportsParams{
		Status:     status,
		EntityType: entityType,
		Q:          q,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing reports: %w", err)
	}

	return reports, nil
}

// GetReport returns a single report by ID (admin detail view).
func (s *Service) GetReport(ctx context.Context, id pgtype.UUID) (gen.ContentReport, error) {
	report, err := s.read.GetReport(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.ContentReport{}, errpkg.ErrNotFound
	}

	if err != nil {
		return gen.ContentReport{}, fmt.Errorf("getting report: %w", err)
	}

	return report, nil
}

// UpdateReportStatus actions or dismisses a report (admin).
func (s *Service) UpdateReportStatus(ctx context.Context, id, reviewedBy pgtype.UUID, status string, resolutionNote *string) (gen.ContentReport, error) {
	if !validStatuses[status] {
		return gen.ContentReport{}, fmt.Errorf("%w: %s", ErrInvalidStatus, status)
	}

	report, err := s.write.UpdateReportStatus(ctx, gen.UpdateReportStatusParams{
		Status:         status,
		ReviewedBy:     reviewedBy,
		ResolutionNote: resolutionNote,
		ID:             id,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.ContentReport{}, errpkg.ErrNotFound
	}

	if err != nil {
		return gen.ContentReport{}, fmt.Errorf("updating report status: %w", err)
	}

	return report, nil
}

// ResolveTargetUser returns the user account that a report's action should
// target: for entity_type 'user' it is the reported user; for content/comment/
// message it is the author/sender; spaces and unknown types resolve to none.
func (s *Service) ResolveTargetUser(ctx context.Context, entityType string, entityID pgtype.UUID) (pgtype.UUID, bool, error) {
	var userID pgtype.UUID

	if entityType == EntityTypeUser {
		return entityID, true, nil
	}

	var query string

	switch entityType {
	case EntityTypeContent:
		query = `SELECT author_id FROM contents WHERE id = $1`
	case EntityTypeComment:
		query = `SELECT author_user_id FROM comments WHERE id = $1`
	case EntityTypeMessage:
		query = `SELECT sender_id FROM dm_messages WHERE id = $1`
	case EntityTypeForumReply:
		query = `SELECT author_user_id FROM forum_replies WHERE id = $1`
	default:
		return userID, false, nil
	}

	var raw [16]byte
	if err := s.db.Reader.QueryRow(ctx, query, entityID).Scan(&raw); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return userID, false, nil
		}

		return userID, false, fmt.Errorf("resolving report target user: %w", err)
	}

	userID.Bytes = raw
	userID.Valid = true

	return userID, true, nil
}

// entityExistsQueries maps entity_type to an EXISTS probe against the entity's
// table. Soft-deleted rows count as absent — a tombstoned comment or reply is
// no longer reportable (mirrors the frontend tombstone guards).
var entityExistsQueries = map[string]string{
	EntityTypeContent:    `SELECT EXISTS(SELECT 1 FROM contents WHERE id = $1 AND deleted_at IS NULL)`,
	EntityTypeComment:    `SELECT EXISTS(SELECT 1 FROM comments WHERE id = $1 AND deleted_at IS NULL)`,
	EntityTypeUser:       `SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)`,
	EntityTypeSpace:      `SELECT EXISTS(SELECT 1 FROM spaces WHERE id = $1 AND deleted_at IS NULL)`,
	EntityTypeMessage:    `SELECT EXISTS(SELECT 1 FROM dm_messages WHERE id = $1 AND deleted_at IS NULL)`,
	EntityTypeSource:     `SELECT EXISTS(SELECT 1 FROM intel_sources WHERE id = $1)`,
	EntityTypeForumReply: `SELECT EXISTS(SELECT 1 FROM forum_replies WHERE id = $1 AND deleted_at IS NULL)`,
}

func (s *Service) entityExists(ctx context.Context, entityType string, entityID pgtype.UUID) (bool, error) {
	query, ok := entityExistsQueries[entityType]
	if !ok {
		return false, nil
	}

	var exists bool
	if err := s.db.Reader.QueryRow(ctx, query, entityID).Scan(&exists); err != nil {
		return false, fmt.Errorf("checking %s existence: %w", entityType, err)
	}

	return exists, nil
}

// IsDMParticipant reports whether the given message lives in a conversation
// the user participates in — enforced on the DM report route so reports
// cannot be filed against arbitrary message UUIDs.
func (s *Service) IsDMParticipant(ctx context.Context, conversationID, messageID, userID pgtype.UUID) (bool, error) {
	var ok bool
	err := s.db.Reader.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM dm_messages m
			JOIN dm_conversations c ON c.id = m.conversation_id
			WHERE m.id = $1 AND m.conversation_id = $2
			  AND (c.participant_1_id = $3 OR c.participant_2_id = $3)
		)`, messageID, conversationID, userID).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("checking dm participation: %w", err)
	}

	return ok, nil
}

// removeReportedEntity applies the remove action: contents are archived,
// comments/DM messages/forum replies soft-deleted — mirroring the auto-hide
// worker's semantics. Only called for entity types where remove applies.
// Runs on the caller's queries so it can join a transaction.
func (s *Service) removeReportedEntity(ctx context.Context, q *gen.Queries, entityType string, entityID pgtype.UUID) error {
	switch entityType {
	case EntityTypeContent:
		if _, err := q.ArchiveContent(ctx, entityID); err != nil {
			return fmt.Errorf("archive content: %w", err)
		}
	case EntityTypeComment:
		if err := q.SoftDeleteCommentByID(ctx, entityID); err != nil {
			return fmt.Errorf("soft-delete comment: %w", err)
		}
	case EntityTypeMessage:
		if err := q.SoftDeleteDMMessageByID(ctx, entityID); err != nil {
			return fmt.Errorf("soft-delete dm message: %w", err)
		}
	case EntityTypeForumReply:
		if err := q.SoftDeleteForumReply(ctx, entityID); err != nil {
			return fmt.Errorf("soft-delete forum reply: %w", err)
		}
	default:
		return fmt.Errorf("%w: %s", ErrActionNotApplicable, entityType)
	}

	return nil
}

// WarnUser records a moderation warning against the given user (reporter side-effect).
func (s *Service) WarnUser(ctx context.Context, userID pgtype.UUID, reason string) error {
	return s.write.CreateUserWarning(ctx, gen.CreateUserWarningParams{
		UserID: userID,
		Reason: &reason,
	})
}

// BanUser bans the given user and invalidates caches so the ban takes effect
// immediately (Rule 6), mirroring the admin user-moderation handler. It also
// updates users.status to 'banned' so the ban is reflected in list queries.
func (s *Service) BanUser(ctx context.Context, userID pgtype.UUID, reason string) error {
	if err := s.write.CreateUserBan(ctx, gen.CreateUserBanParams{
		UserID: userID,
		Reason: &reason, Column3: 0,
	}); err != nil {
		return err
	}
	// Reflect the ban in users.status so list queries and status checks see
	// the banned state (mirrors internal/admin/moderation.go:BanUser).
	if err := s.write.UpdateUserStatus(ctx, gen.UpdateUserStatusParams{
		Status: "banned",
		ID:     userID,
	}); err != nil {
		slog.Error("ban user: failed to update users.status", "error", err, "user_id", userID.String())
	}

	if s.valkey != nil {
		if err := rbac.InvalidatePermCache(ctx, s.valkey, userID); err != nil {
			slog.Error("ban user: failed to invalidate permission cache", "error", err, "user_id", userID.String())
		}

		if err := s.valkey.Del(ctx, valkey.MustKey("ban:%s", userID.String())).Err(); err != nil {
			slog.Error("ban user: failed to invalidate ban cache", "error", err, "user_id", userID.String())
		}
	}

	return nil
}
