// Package scamalert — service.go orchestrates scam report submission,
// browsing, subscription, and moderator review. Feature-flagged via
// scam_alert.enabled system_setting (§81). Anonymous reports are allowed
// when scam_alert.anonymous_reports is enabled.
//
// Ref: japaconnect-engineering/sections/81-scam-alert-board.md
//      japaconnect-engineering/adr/ADR-089-scam-alert-board.md
package scamalert

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
)

// ScamReportDTO is the API response shape for a single scam report.
type ScamReportDTO struct {
	ID                    string `json:"id"`
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
	PinnedSpaceID         string `json:"pinned_space_id,omitempty"`
	ReviewedBy            string `json:"reviewed_by,omitempty"`
	ReviewedAt            string `json:"reviewed_at,omitempty"`
	CreatedAt             string `json:"created_at"`
	UpdatedAt             string `json:"updated_at,omitempty"`
	ReporterName          string `json:"reporter_name,omitempty"`
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

// ReviewInput is the request body for PATCH /admin/scam-alerts/:id.
type ReviewInput struct {
	Status        string `json:"status"` // confirmed|rejected
	ModeratorNote string `json:"moderator_note"`
}

// Service holds the business logic for the Scam Alert Board.
type Service struct {
	store    *Store
	settings *config.Settings
}

// NewService creates a scam-alert service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{store: store, settings: settings}
}

// Enabled reads the scam_alert.enabled system_setting toggle (§81 feature flag).
func (s *Service) Enabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return false
	}
	return s.settings.GetBoolOr(ctx, tenantID, "scam_alert.enabled", false)
}

// AnonymousReportsAllowed reads the scam_alert.anonymous_reports setting.
func (s *Service) AnonymousReportsAllowed(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return false
	}
	return s.settings.GetBoolOr(ctx, tenantID, "scam_alert.anonymous_reports", false)
}

// Submit creates a new scam report in 'pending' status. reporterID may be
// zero-valued (invalid) for anonymous reports when anonymous reports are
// allowed.
func (s *Service) Submit(ctx context.Context, tenantID, reporterID pgtype.UUID, input SubmitReportInput) (ScamReportDTO, error) {
	if input.ScamType == "" || input.DestinationSpaceID == "" || input.Description == "" {
		return ScamReportDTO{}, fmt.Errorf("scam_type, destination_space_id, and description are required")
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
		return ScamReportDTO{}, fmt.Errorf("invalid destination_space_id: %w", err)
	}

	var originSpaceID pgtype.UUID
	if input.OriginSpaceID != "" {
		if err := originSpaceID.Scan(input.OriginSpaceID); err != nil {
			return ScamReportDTO{}, fmt.Errorf("invalid origin_space_id: %w", err)
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
			return ScamReportDTO{}, fmt.Errorf("invalid approximate_date: %w", err)
		}
	}

	// reporterID is only set when valid (authenticated submitter).
	var reporterPtr pgtype.UUID
	if reporterID.Valid {
		reporterPtr = reporterID
	}

	report := ScamReport{
		TenantID:              tenantID,
		ReporterID:            reporterPtr,
		ScamType:              input.ScamType,
		DestinationSpaceID:    destSpaceID,
		OriginSpaceID:         originSpaceID,
		OriginCommunity:       originCommunity,
		PlatformUsed:          platformUsed,
		ApproximateDate:       approximateDate,
		AmountRange:           amountRange,
		Description:           input.Description,
		ReportedToAuthorities: input.ReportedToAuthorities,
	}

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
		ScamType:              input.ScamType,
		DestinationSpaceID:    destSpaceID.String(),
		Description:           input.Description,
		ReportedToAuthorities: input.ReportedToAuthorities,
		Status:                "pending",
	}
	if originSpaceID.Valid {
		dto.OriginSpaceID = originSpaceID.String()
	}
	if originCommunity != nil {
		dto.OriginCommunity = *originCommunity
	}
	if platformUsed != nil {
		dto.PlatformUsed = *platformUsed
	}
	if approximateDate.Valid {
		dto.ApproximateDate = approximateDate.Time.Format("2006-01-02")
	}
	if amountRange != nil {
		dto.AmountRange = *amountRange
	}
	return dto, nil
}

// Get returns a single confirmed scam report (public view).
func (s *Service) Get(ctx context.Context, tenantID pgtype.UUID, id pgtype.UUID) (ScamReportDTO, error) {
	row, err := s.store.GetReport(ctx, id, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ScamReportDTO{}, pgx.ErrNoRows
		}
		return ScamReportDTO{}, fmt.Errorf("scamalert: get: %w", err)
	}
	return rowToDTO(row), nil
}

// List returns a paginated, filtered list of confirmed scam reports.
func (s *Service) List(ctx context.Context, tenantID pgtype.UUID, destSpaceID pgtype.UUID, scamType string, page, limit int32) ([]ScamReportDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	rows, total, err := s.store.ListReports(ctx, ListReportsFilter{
		TenantID:           tenantID,
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
func (s *Service) Unsubscribe(ctx context.Context, userID, tenantID, destSpaceID pgtype.UUID) error {
	if err := s.store.Unsubscribe(ctx, userID, tenantID, destSpaceID); err != nil {
		return fmt.Errorf("scamalert: unsubscribe: %w", err)
	}
	return nil
}

// AdminList returns all reports (any status) for the moderation queue.
func (s *Service) AdminList(ctx context.Context, tenantID pgtype.UUID, status string, destSpaceID pgtype.UUID, scamType string, page, limit int32) ([]ScamReportDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	rows, total, err := s.store.AdminListReports(ctx, AdminListReportsFilter{
		TenantID:           tenantID,
		Status:             status,
		DestinationSpaceID: destSpaceID,
		ScamType:           scamType,
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

// Review updates the status of a report (confirm or reject). The reviewerID
// is stamped as reviewed_by.
func (s *Service) Review(ctx context.Context, id, tenantID pgtype.UUID, status, note string, reviewerID pgtype.UUID) error {
	if status != "confirmed" && status != "rejected" {
		return fmt.Errorf("status must be 'confirmed' or 'rejected'")
	}
	if err := s.store.UpdateReportStatus(ctx, id, tenantID, status, note, reviewerID); err != nil {
		if err == pgx.ErrNoRows {
			return pgx.ErrNoRows
		}
		return fmt.Errorf("scamalert: review: %w", err)
	}
	return nil
}

// Pin pins a confirmed report to a space.
func (s *Service) Pin(ctx context.Context, id, tenantID, spaceID pgtype.UUID) error {
	if err := s.store.PinReport(ctx, id, tenantID, spaceID); err != nil {
		if err == pgx.ErrNoRows {
			return pgx.ErrNoRows
		}
		return fmt.Errorf("scamalert: pin: %w", err)
	}
	return nil
}

// Unpin removes the pin from a report.
func (s *Service) Unpin(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.store.UnpinReport(ctx, id, tenantID); err != nil {
		if err == pgx.ErrNoRows {
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
