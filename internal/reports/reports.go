// Package reports — Content Reports & Flagging (JEOS Section 51).
//
// Users report content, comments, other users, spaces, and DM messages;
// reports feed the admin moderation queue (Section 22).
package reports

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
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
	read  *gen.Queries
	write *gen.Queries
}

// NewService creates a Service backed by the given database.
func NewService(database *db.DB) *Service {
	return &Service{read: gen.New(database.Reader), write: gen.New(database.Writer)}
}

// CreateReport submits a report. Returns ErrAlreadyReported if this reporter
// already reported the same entity (enforced by a partial unique index).
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

	return report, nil
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

// ListReports returns all reports for the tenant (admin), optionally
// filtered by status and/or entity_type.
func (s *Service) ListReports(ctx context.Context, tenantID pgtype.UUID, status, entityType *string, limit, offset int32) ([]gen.ContentReport, error) {
	reports, err := s.read.ListReports(ctx, gen.ListReportsParams{
		TenantID:   tenantID,
		Status:     status,
		EntityType: entityType,
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
