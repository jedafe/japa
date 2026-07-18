// Package reports — Content Reports & Flagging (JEOS Section 51).
//
// Users report content, comments, other users, spaces, and DM messages;
// reports feed the admin moderation queue (Section 22).
package reports

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/rbac"
	valkeycompat "github.com/valkey-io/valkey-go/valkeycompat"
)

// Valid entity_type and reason values, matching the migration's CHECK constraints.
const (
	EntityTypeContent = "content"
	EntityTypeComment = "comment"
	EntityTypeUser    = "user"
	EntityTypeSpace   = "space"
	EntityTypeMessage = "message"
)

var validEntityTypes = map[string]bool{
	EntityTypeContent: true,
	EntityTypeComment: true,
	EntityTypeUser:    true,
	EntityTypeSpace:   true,
	EntityTypeMessage: true,
}

var validReasons = map[string]bool{
	"spam":            true,
	"misinformation":  true,
	"harassment":      true,
	"hate_speech":     true,
	"illegal_content": true,
	"self_harm":       true,
	"copyright":       true,
	"other":           true,
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
	ErrNotFound          = errors.New("report not found")
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
}

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

// CreateReport submits a report. Returns ErrAlreadyReported if this reporter
// already reported the same entity (enforced by a partial unique index).
// For content entities, checks the auto-hide threshold and hides the content
// when the pending report count reaches it.
func (s *Service) CreateReport(ctx context.Context, tenantID, reporterUserID pgtype.UUID, input CreateReportInput) (gen.ContentReport, error) {
	if !validEntityTypes[input.EntityType] {
		return gen.ContentReport{}, fmt.Errorf("%w: %s", ErrInvalidEntityType, input.EntityType)
	}

	if !validReasons[input.Reason] {
		return gen.ContentReport{}, fmt.Errorf("%w: %s", ErrInvalidReason, input.Reason)
	}

	report, err := s.write.CreateReport(ctx, gen.CreateReportParams{
		TenantID:       tenantID,
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

	if input.EntityType == EntityTypeContent {
		s.checkAutoHide(ctx, tenantID, input.EntityType, input.EntityID)
	}

	return report, nil
}

// checkAutoHide counts pending reports for the entity and auto-hides content
// when the threshold is reached. Best-effort — errors are logged but never
// propagate to the caller.
func (s *Service) checkAutoHide(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID pgtype.UUID) {
	threshold := 3
	if s.settings != nil {
		threshold = s.settings.GetIntOr(ctx, tenantID, "moderation.auto_hide_report_count", 3)
	}

	count, err := s.write.CountPendingReportsForEntity(ctx, gen.CountPendingReportsForEntityParams{
		TenantID:   tenantID,
		EntityType: entityType,
		EntityID:   entityID,
	})
	if err != nil {
		slog.Warn("reports: failed to count pending reports for auto-hide",
			"entity_type", entityType, "entity_id", entityID.String(), "error", err)
		return
	}

	if count < int32(threshold) {
		return
	}

	_, err = s.write.AutoHideContent(ctx, gen.AutoHideContentParams{
		ID:       entityID,
		TenantID: tenantID,
	})
	if err != nil {
		slog.Warn("reports: failed to auto-hide content",
			"content_id", entityID.String(), "report_count", count, "error", err)
		return
	}

	slog.Info("reports: content auto-hidden",
		"content_id", entityID.String(), "report_count", count, "threshold", threshold)
}

// ListMyReports returns the caller's own submitted reports, newest first.
func (s *Service) ListMyReports(ctx context.Context, tenantID, reporterUserID pgtype.UUID, limit, offset int32) ([]gen.ContentReport, error) {
	reports, err := s.read.ListReportsByReporter(ctx, gen.ListReportsByReporterParams{
		TenantID:       tenantID,
		ReporterUserID: reporterUserID,
		PageLimit:      limit,
		PageOffset:     offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing my reports: %w", err)
	}

	return reports, nil
}

// CountReports returns the total number of reports for the tenant (respecting
// an optional status filter) so callers can paginate.
func (s *Service) CountReports(ctx context.Context, tenantID pgtype.UUID, status, q *string) (int32, error) {
	var total int32
	query := `SELECT COUNT(*)::int FROM content_reports WHERE tenant_id = $1`
	args := []any{tenantID}
	if status != nil {
		args = append(args, *status)
		query += fmt.Sprintf(` AND status = $%d`, len(args))
	}
	if q != nil && *q != "" {
		args = append(args, *q)
		p := fmt.Sprintf("$%d", len(args))
		query += ` AND (entity_id::text ILIKE '%' || ` + p + ` || '%'` +
			` OR entity_type ILIKE '%' || ` + p + ` || '%'` +
			` OR reporter_user_id::text ILIKE '%' || ` + p + ` || '%')`
	}
	if err := s.db.Reader.QueryRow(ctx, query, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("counting reports: %w", err)
	}
	return total, nil
}

// ListReports returns all reports for the tenant (admin), optionally
// filtered by status and/or entity_type.
func (s *Service) ListReports(ctx context.Context, tenantID pgtype.UUID, status, entityType, q *string, limit, offset int32) ([]gen.ContentReport, error) {
	reports, err := s.read.ListReports(ctx, gen.ListReportsParams{
		TenantID:   tenantID,
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
func (s *Service) GetReport(ctx context.Context, id, tenantID pgtype.UUID) (gen.ContentReport, error) {
	report, err := s.read.GetReport(ctx, gen.GetReportParams{ID: id, TenantID: tenantID})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.ContentReport{}, ErrNotFound
	}

	if err != nil {
		return gen.ContentReport{}, fmt.Errorf("getting report: %w", err)
	}

	return report, nil
}

// UpdateReportStatus actions or dismisses a report (admin).
func (s *Service) UpdateReportStatus(ctx context.Context, id, tenantID, reviewedBy pgtype.UUID, status string, resolutionNote *string) (gen.ContentReport, error) {
	if !validStatuses[status] {
		return gen.ContentReport{}, fmt.Errorf("%w: %s", ErrInvalidStatus, status)
	}

	report, err := s.write.UpdateReportStatus(ctx, gen.UpdateReportStatusParams{
		Status:         status,
		ReviewedBy:     reviewedBy,
		ResolutionNote: resolutionNote,
		ID:             id,
		TenantID:       tenantID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.ContentReport{}, ErrNotFound
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
		query = `SELECT author_id FROM comments WHERE id = $1`
	case EntityTypeMessage:
		query = `SELECT sender_id FROM dm_messages WHERE id = $1`
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

// WarnUser records a moderation warning against the given user (reporter side-effect).
func (s *Service) WarnUser(ctx context.Context, userID pgtype.UUID, reason string) error {
	return s.write.CreateUserWarning(ctx, gen.CreateUserWarningParams{
		UserID: userID,
		Reason:  &reason,
	})
}

// BanUser bans the given user and invalidates caches so the ban takes effect
// immediately (Rule 6), mirroring the admin user-moderation handler.
func (s *Service) BanUser(ctx context.Context, userID pgtype.UUID, reason string) error {
	if err := s.write.CreateUserBan(ctx, gen.CreateUserBanParams{
		UserID: userID,
		Reason:  &reason,
		Column3: 0,
	}); err != nil {
		return err
	}
	if s.valkey != nil {
		_ = rbac.InvalidatePermCache(ctx, s.valkey, userID)
		_ = s.valkey.Del(ctx, "jc:ban:"+userID.String()).Err()
	}
	return nil
}
