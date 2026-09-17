// Package scamalert implements the Scam Alert Board (§81, ADR-089).
// // Community-driven scam reports filed against a destination space. Reports
// enter a pending moderation queue; moderators confirm or reject them.
// Confirmed reports are publicly browsable and can be pinned to a space.
// Users can subscribe to a destination space to receive notifications when
// new confirmed scam alerts are posted.
// // Ref: japaconnect-engineering/sections/81-scam-alert-board.md
// //	japaconnect-engineering/adr/ADR-089-scam-alert-board.md
package scamalert

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
)

// Store is the data-access layer for scam_reports and
// scam_alert_subscriptions. It uses direct pgx queries because these are
// new tables without sqlc-generated queries yet. No business logic here.
type Store struct {
	pool *db.DB
}

// NewStore creates a scam-alert store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{pool: database}
}

// ScamReport holds a single row from the scam_reports table.
type ScamReport struct {
	ID pgtype.UUID

	ReporterID            pgtype.UUID
	ScamType              string
	DestinationSpaceID    pgtype.UUID
	OriginSpaceID         pgtype.UUID
	OriginCommunity       *string
	PlatformUsed          *string
	ApproximateDate       pgtype.Date
	AmountRange           *string
	Description           string
	ReportedToAuthorities bool
	Status                string
	ModeratorNote         *string
	IsPinned              bool
	PinnedSpaceID         pgtype.UUID
	ReviewedBy            pgtype.UUID
	ReviewedAt            pgtype.Timestamptz
	// ADR-088 universal AI-generation columns.
	AiGenerated    bool
	AiModel        *string
	AiGeneratedAt  pgtype.Timestamptz
	HitlReviewedBy pgtype.UUID
	HitlReviewedAt pgtype.Timestamptz
	CreatedAt      pgtype.Timestamptz
	UpdatedAt      pgtype.Timestamptz
	DeletedAt      pgtype.Timestamptz
}

// ScamReportRow is the joined result of scam_reports + spaces (destination +
// origin) + users (reporter name), returned by list and get queries.
type ScamReportRow struct {
	ScamReport
	DestSpaceName   *string
	DestSpaceCode   *string
	OriginSpaceName *string
	OriginSpaceCode *string
	ReporterName    *string
}

// CreateReport inserts a new scam report in 'pending' status.
func (s *Store) CreateReport(ctx context.Context, report ScamReport) (pgtype.UUID, error) {
	var id pgtype.UUID

	err := s.pool.Writer.QueryRow(ctx, `
INSERT INTO scam_reports (
    reporter_id, scam_type, destination_space_id,
    origin_space_id, origin_community, platform_used,
    approximate_date, amount_range, description, reported_to_authorities,
    status
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'under_review')
RETURNING id
`, report.ReporterID, report.ScamType, report.DestinationSpaceID, report.OriginSpaceID, report.OriginCommunity, report.PlatformUsed, report.ApproximateDate, report.AmountRange, report.Description, report.ReportedToAuthorities).Scan(&id)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("scamalert: create report: %w", err)
	}

	return id, nil
}

// GetReport returns a single confirmed report by id (public view).
func (s *Store) GetReport(ctx context.Context, id pgtype.UUID) (ScamReportRow, error) {
	const q = `
SELECT sr.id, sr.reporter_id, sr.scam_type,
       sr.destination_space_id, sr.origin_space_id, sr.origin_community,
       sr.platform_used, sr.approximate_date, sr.amount_range, sr.description,
       sr.reported_to_authorities, sr.status, sr.moderator_note,
       sr.is_pinned, sr.pinned_space_id, sr.reviewed_by, sr.reviewed_at,
       sr.ai_generated, sr.ai_model, sr.ai_generated_at,
       sr.hitl_reviewed_by, sr.hitl_reviewed_at,
       sr.created_at, sr.updated_at, sr.deleted_at,
       ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       os.name AS origin_space_name, os.geo_code AS origin_space_code,
       u.username AS reporter_name
FROM scam_reports sr
LEFT JOIN spaces ds ON ds.id = sr.destination_space_id
LEFT JOIN spaces os ON os.id = sr.origin_space_id
LEFT JOIN users u ON u.id = sr.reporter_id
WHERE sr.id = $1
  AND sr.status = 'confirmed' AND sr.deleted_at IS NULL
LIMIT 1
`

	var r ScamReportRow

	err := s.pool.Reader.QueryRow(ctx, q, id).Scan(
		&r.ID, &r.ReporterID, &r.ScamType,
		&r.DestinationSpaceID, &r.OriginSpaceID, &r.OriginCommunity,
		&r.PlatformUsed, &r.ApproximateDate, &r.AmountRange, &r.Description,
		&r.ReportedToAuthorities, &r.Status, &r.ModeratorNote,
		&r.IsPinned, &r.PinnedSpaceID, &r.ReviewedBy, &r.ReviewedAt,
		&r.AiGenerated, &r.AiModel, &r.AiGeneratedAt,
		&r.HitlReviewedBy, &r.HitlReviewedAt,
		&r.CreatedAt, &r.UpdatedAt, &r.DeletedAt,
		&r.DestSpaceName, &r.DestSpaceCode,
		&r.OriginSpaceName, &r.OriginSpaceCode,
		&r.ReporterName,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ScamReportRow{}, pgx.ErrNoRows
		}

		return ScamReportRow{}, fmt.Errorf("scamalert: get report: %w", err)
	}

	return r, nil
}

// GetReportForUpdate returns a single non-deleted report by id, regardless of
// status. Used by the public edit path before an ownership/permission check.
func (s *Store) GetReportForUpdate(ctx context.Context, id pgtype.UUID) (ScamReportRow, error) {
	const q = `
SELECT sr.id, sr.reporter_id, sr.scam_type,
       sr.destination_space_id, sr.origin_space_id, sr.origin_community,
       sr.platform_used, sr.approximate_date, sr.amount_range, sr.description,
       sr.reported_to_authorities, sr.status, sr.moderator_note,
       sr.is_pinned, sr.pinned_space_id, sr.reviewed_by, sr.reviewed_at,
       sr.ai_generated, sr.ai_model, sr.ai_generated_at,
       sr.hitl_reviewed_by, sr.hitl_reviewed_at,
       sr.created_at, sr.updated_at, sr.deleted_at,
       ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       os.name AS origin_space_name, os.geo_code AS origin_space_code,
       u.username AS reporter_name
FROM scam_reports sr
LEFT JOIN spaces ds ON ds.id = sr.destination_space_id
LEFT JOIN spaces os ON os.id = sr.origin_space_id
LEFT JOIN users u ON u.id = sr.reporter_id
WHERE sr.id = $1 AND sr.deleted_at IS NULL
LIMIT 1
`

	var r ScamReportRow

	err := s.pool.Reader.QueryRow(ctx, q, id).Scan(
		&r.ID, &r.ReporterID, &r.ScamType,
		&r.DestinationSpaceID, &r.OriginSpaceID, &r.OriginCommunity,
		&r.PlatformUsed, &r.ApproximateDate, &r.AmountRange, &r.Description,
		&r.ReportedToAuthorities, &r.Status, &r.ModeratorNote,
		&r.IsPinned, &r.PinnedSpaceID, &r.ReviewedBy, &r.ReviewedAt,
		&r.AiGenerated, &r.AiModel, &r.AiGeneratedAt,
		&r.HitlReviewedBy, &r.HitlReviewedAt,
		&r.CreatedAt, &r.UpdatedAt, &r.DeletedAt,
		&r.DestSpaceName, &r.DestSpaceCode,
		&r.OriginSpaceName, &r.OriginSpaceCode,
		&r.ReporterName,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ScamReportRow{}, pgx.ErrNoRows
		}

		return ScamReportRow{}, fmt.Errorf("scamalert: get report for update: %w", err)
	}

	return r, nil
}

// UpdateReport updates the editable content fields of a scam report and
// returns the refreshed row. Status, moderation metadata and ownership fields
// are not changed.
func (s *Store) UpdateReport(ctx context.Context, id pgtype.UUID, report ScamReport) (ScamReportRow, error) {
	cmd, err := s.pool.Writer.Exec(ctx, `
UPDATE scam_reports
SET scam_type = $2,
    destination_space_id = $3,
    origin_space_id = $4,
    origin_community = $5,
    platform_used = $6,
    approximate_date = $7,
    amount_range = $8,
    description = $9,
    reported_to_authorities = $10,
    updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
`, id, report.ScamType, report.DestinationSpaceID, report.OriginSpaceID, report.OriginCommunity, report.PlatformUsed, report.ApproximateDate, report.AmountRange, report.Description, report.ReportedToAuthorities)
	if err != nil {
		return ScamReportRow{}, fmt.Errorf("scamalert: update report: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return ScamReportRow{}, pgx.ErrNoRows
	}

	// Re-fetch with the same joins used by GetReportForUpdate.
	return s.GetReportForUpdate(ctx, id)
}

// ListReportsFilter holds the filter parameters for listing confirmed reports.
type ListReportsFilter struct {
	DestinationSpaceID pgtype.UUID // optional
	ScamType           string      // optional
	Limit              int32
	Offset             int32
}

// ListReports returns a paginated, filtered list of confirmed reports.
// Only rows with status='confirmed' AND deleted_at IS NULL are returned.
func (s *Store) ListReports(ctx context.Context, f ListReportsFilter) ([]ScamReportRow, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	conditions = append(conditions, "sr.status = 'confirmed'")
	conditions = append(conditions, "sr.deleted_at IS NULL")

	if f.DestinationSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("sr.destination_space_id = $%d", argIdx))
		args = append(args, f.DestinationSpaceID)
		argIdx++
	}

	if f.ScamType != "" {
		conditions = append(conditions, fmt.Sprintf("sr.scam_type = $%d", argIdx))
		args = append(args, f.ScamType)
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")

	limit := f.Limit
	if limit <= 0 {
		limit = 20
	}

	if limit > 100 {
		limit = 100
	}

	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	listQuery := fmt.Sprintf(`
SELECT sr.id, sr.reporter_id, sr.scam_type,
       sr.destination_space_id, sr.origin_space_id, sr.origin_community,
       sr.platform_used, sr.approximate_date, sr.amount_range, sr.description,
       sr.reported_to_authorities, sr.status, sr.moderator_note,
       sr.is_pinned, sr.pinned_space_id, sr.reviewed_by, sr.reviewed_at,
       sr.ai_generated, sr.ai_model, sr.ai_generated_at,
       sr.hitl_reviewed_by, sr.hitl_reviewed_at,
       sr.created_at, sr.updated_at, sr.deleted_at,
       ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       os.name AS origin_space_name, os.geo_code AS origin_space_code,
       u.username AS reporter_name
FROM scam_reports sr
LEFT JOIN spaces ds ON ds.id = sr.destination_space_id
LEFT JOIN spaces os ON os.id = sr.origin_space_id
LEFT JOIN users u ON u.id = sr.reporter_id
WHERE %s
ORDER BY sr.is_pinned DESC, sr.created_at DESC
LIMIT $%d OFFSET $%d
`, whereClause, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("scamalert: list reports: %w", err)
	}
	defer rows.Close()

	var out []ScamReportRow

	for rows.Next() {
		var r ScamReportRow
		if err := rows.Scan(
			&r.ID, &r.ReporterID, &r.ScamType,
			&r.DestinationSpaceID, &r.OriginSpaceID, &r.OriginCommunity,
			&r.PlatformUsed, &r.ApproximateDate, &r.AmountRange, &r.Description,
			&r.ReportedToAuthorities, &r.Status, &r.ModeratorNote,
			&r.IsPinned, &r.PinnedSpaceID, &r.ReviewedBy, &r.ReviewedAt,
			&r.AiGenerated, &r.AiModel, &r.AiGeneratedAt,
			&r.HitlReviewedBy, &r.HitlReviewedAt,
			&r.CreatedAt, &r.UpdatedAt, &r.DeletedAt,
			&r.DestSpaceName, &r.DestSpaceCode,
			&r.OriginSpaceName, &r.OriginSpaceCode,
			&r.ReporterName,
		); err != nil {
			return nil, 0, fmt.Errorf("scamalert: scan report row: %w", err)
		}

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("scamalert: iterate report rows: %w", err)
	}

	countQuery := fmt.Sprintf(`
SELECT COUNT(*) FROM scam_reports sr WHERE %s
`, whereClause)
	countArgs := args[:len(args)-2]

	var total int64

	if err := s.pool.Reader.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("scamalert: count reports: %w", err)
	}

	return out, total, nil
}

// Subscribe inserts a scam-alert subscription for a user + destination space.
func (s *Store) Subscribe(ctx context.Context, userID, destSpaceID pgtype.UUID) error {
	_, err := s.pool.Writer.Exec(ctx, `
INSERT INTO scam_alert_subscriptions (user_id, destination_space_id) VALUES ($1, $2, $3)
ON CONFLICT (user_id, destination_space_id) DO NOTHING
`, userID, destSpaceID)
	if err != nil {
		return fmt.Errorf("scamalert: subscribe: %w", err)
	}

	return nil
}

// IsSubscribed returns true if the user is subscribed to scam alerts for the
// given destination space.
func (s *Store) IsSubscribed(ctx context.Context, userID, destSpaceID pgtype.UUID) (bool, error) {
	var exists bool
	err := s.pool.Reader.QueryRow(ctx, `
SELECT EXISTS (
    SELECT 1 FROM scam_alert_subscriptions
    WHERE user_id = $1 AND destination_space_id = $2
)
`, userID, destSpaceID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("scamalert: is subscribed: %w", err)
	}
	return exists, nil
}

// Unsubscribe deletes a scam-alert subscription for a user + destination space.
func (s *Store) Unsubscribe(ctx context.Context, userID, destSpaceID pgtype.UUID) error {
	_, err := s.pool.Writer.Exec(ctx, `
DELETE FROM scam_alert_subscriptions
WHERE user_id = $1 AND destination_space_id = $2
`, userID, destSpaceID)
	if err != nil {
		return fmt.Errorf("scamalert: unsubscribe: %w", err)
	}

	return nil
}

// ListSubscribersByDestination returns all user IDs subscribed to scam
// alerts for the given destination space. Used by the
// scam_alert:notify_subscribers task to fan out notifications on confirm.
func (s *Store) ListSubscribersByDestination(ctx context.Context, destSpaceID pgtype.UUID) ([]pgtype.UUID, error) {
	rows, err := s.pool.Reader.Query(ctx, `
SELECT user_id FROM scam_alert_subscriptions
WHERE destination_space_id = $1
`, destSpaceID)
	if err != nil {
		return nil, fmt.Errorf("scamalert: list subscribers: %w", err)
	}
	defer rows.Close()

	var userIDs []pgtype.UUID

	for rows.Next() {
		var uid pgtype.UUID
		if err := rows.Scan(&uid); err != nil {
			return nil, fmt.Errorf("scamalert: list subscribers: scan: %w", err)
		}

		userIDs = append(userIDs, uid)
	}

	return userIDs, rows.Err()
}

// GetReportForNotification returns the report's destination_space_id and
// reporter_id for the notify_subscribers task. Returns pgx.ErrNoRows if
// the report does not exist or does not belong to the platform.
func (s *Store) GetReportForNotification(ctx context.Context, id pgtype.UUID) (destSpaceID, reporterID pgtype.UUID, scamType, description string, err error) {
	err = s.pool.Reader.QueryRow(ctx, `
SELECT destination_space_id, reporter_id, scam_type, description
FROM scam_reports
WHERE id = $1 AND status = 'confirmed'
`, id).Scan(&destSpaceID, &reporterID, &scamType, &description)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, "", "", err
	}

	return destSpaceID, reporterID, scamType, description, nil
}

// AdminListReportsFilter holds filter parameters for the admin moderation list.
type AdminListReportsFilter struct {
	Status             string      // optional: under_review|confirmed|debunked|duplicate
	DestinationSpaceID pgtype.UUID // optional
	ScamType           string      // optional
	Query              string      // optional: ?q= free-text search (ILIKE)
	Sort               string      // optional: allowlisted sort key
	Order              string      // optional: asc|desc (default desc)
	Limit              int32
	Offset             int32
}

// AdminListReports returns all reports (any status) for the moderation queue.
func (s *Store) AdminListReports(ctx context.Context, f AdminListReportsFilter) ([]ScamReportRow, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	conditions = append(conditions, "sr.deleted_at IS NULL")

	if f.Status != "" {
		conditions = append(conditions, fmt.Sprintf("sr.status = $%d", argIdx))
		args = append(args, f.Status)
		argIdx++
	}

	if f.DestinationSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("sr.destination_space_id = $%d", argIdx))
		args = append(args, f.DestinationSpaceID)
		argIdx++
	}

	if f.ScamType != "" {
		conditions = append(conditions, fmt.Sprintf("sr.scam_type = $%d", argIdx))
		args = append(args, f.ScamType)
		argIdx++
	}

	// ?q= free-text search across report content columns. The destination
	// space name is intentionally excluded: the count query has no joins.
	if q := strings.TrimSpace(f.Query); q != "" {
		conditions = append(conditions, fmt.Sprintf(
			"(sr.description ILIKE $%d OR sr.origin_community ILIKE $%d OR sr.scam_type ILIKE $%d)",
			argIdx, argIdx, argIdx))
		args = append(args, "%"+q+"%")
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")

	limit := f.Limit
	if limit <= 0 {
		limit = 20
	}

	if limit > 200 {
		limit = 200
	}

	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	// Allowlisted sort keys → column expressions. Anything unrecognised falls
	// back to the default created_at ordering so user input can never inject
	// into ORDER BY.
	sortColumn := "sr.created_at"
	switch f.Sort {
	case "scam_type":
		sortColumn = "sr.scam_type"
	case "status":
		sortColumn = "sr.status"
	case "destination_space_name":
		sortColumn = "ds.name"
	case "updated_at":
		sortColumn = "sr.updated_at"
	}

	sortOrder := "DESC"
	if f.Order == "asc" {
		sortOrder = "ASC"
	}

	listQuery := fmt.Sprintf(`
SELECT sr.id, sr.reporter_id, sr.scam_type,
       sr.destination_space_id, sr.origin_space_id, sr.origin_community,
       sr.platform_used, sr.approximate_date, sr.amount_range, sr.description,
       sr.reported_to_authorities, sr.status, sr.moderator_note,
       sr.is_pinned, sr.pinned_space_id, sr.reviewed_by, sr.reviewed_at,
       sr.ai_generated, sr.ai_model, sr.ai_generated_at,
       sr.hitl_reviewed_by, sr.hitl_reviewed_at,
       sr.created_at, sr.updated_at, sr.deleted_at,
       ds.name AS dest_space_name, ds.geo_code AS dest_space_code,
       os.name AS origin_space_name, os.geo_code AS origin_space_code,
       u.username AS reporter_name
FROM scam_reports sr
LEFT JOIN spaces ds ON ds.id = sr.destination_space_id
LEFT JOIN spaces os ON os.id = sr.origin_space_id
LEFT JOIN users u ON u.id = sr.reporter_id
WHERE %s
ORDER BY %s %s, sr.id DESC
LIMIT $%d OFFSET $%d
`, whereClause, sortColumn, sortOrder, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("scamalert: admin list reports: %w", err)
	}
	defer rows.Close()

	var out []ScamReportRow

	for rows.Next() {
		var r ScamReportRow
		if err := rows.Scan(
			&r.ID, &r.ReporterID, &r.ScamType,
			&r.DestinationSpaceID, &r.OriginSpaceID, &r.OriginCommunity,
			&r.PlatformUsed, &r.ApproximateDate, &r.AmountRange, &r.Description,
			&r.ReportedToAuthorities, &r.Status, &r.ModeratorNote,
			&r.IsPinned, &r.PinnedSpaceID, &r.ReviewedBy, &r.ReviewedAt,
			&r.AiGenerated, &r.AiModel, &r.AiGeneratedAt,
			&r.HitlReviewedBy, &r.HitlReviewedAt,
			&r.CreatedAt, &r.UpdatedAt, &r.DeletedAt,
			&r.DestSpaceName, &r.DestSpaceCode,
			&r.OriginSpaceName, &r.OriginSpaceCode,
			&r.ReporterName,
		); err != nil {
			return nil, 0, fmt.Errorf("scamalert: admin scan report row: %w", err)
		}

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("scamalert: admin iterate report rows: %w", err)
	}

	countQuery := fmt.Sprintf(`
SELECT COUNT(*) FROM scam_reports sr WHERE %s
`, whereClause)
	countArgs := args[:len(args)-2]

	var total int64

	if err := s.pool.Reader.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("scamalert: admin count reports: %w", err)
	}

	return out, total, nil
}

// UpdateReportStatus updates the status of a report and stamps the reviewer.
// status must be 'confirmed', 'debunked' or 'duplicate'.
// ADR-088: the review action itself is the human-in-the-loop approval — when
// an AI-generated report is reviewed for the first time, hitl_reviewed_by /
// hitl_reviewed_at are stamped with the reviewing moderator.
func (s *Store) UpdateReportStatus(ctx context.Context, id pgtype.UUID, status, moderatorNote string, reviewerID pgtype.UUID) error {
	var notePtr *string
	if moderatorNote != "" {
		notePtr = &moderatorNote
	}

	cmd, err := s.pool.Writer.Exec(ctx, `
UPDATE scam_reports
SET status = $2, moderator_note = $3, reviewed_by = $4, reviewed_at = now(),
    hitl_reviewed_by = CASE
        WHEN ai_generated AND hitl_reviewed_by IS NULL THEN $4
        ELSE hitl_reviewed_by
    END,
    hitl_reviewed_at = CASE
        WHEN ai_generated AND hitl_reviewed_by IS NULL THEN now()
        ELSE hitl_reviewed_at
    END,
    updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
`, id, status, notePtr, reviewerID)
	if err != nil {
		return fmt.Errorf("scamalert: update report status: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// PinReport pins a confirmed report to a space.
func (s *Store) PinReport(ctx context.Context, id, pinnedSpaceID pgtype.UUID) error {
	cmd, err := s.pool.Writer.Exec(ctx, `
UPDATE scam_reports
SET is_pinned = true, pinned_space_id = $2, updated_at = now()
WHERE id = $1 AND status = 'confirmed' AND deleted_at IS NULL
`, id, pinnedSpaceID)
	if err != nil {
		return fmt.Errorf("scamalert: pin report: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// UnpinReport removes the pin from a report.
func (s *Store) UnpinReport(ctx context.Context, id pgtype.UUID) error {
	cmd, err := s.pool.Writer.Exec(ctx, `
UPDATE scam_reports
SET is_pinned = false, pinned_space_id = NULL, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
`, id)
	if err != nil {
		return fmt.Errorf("scamalert: unpin report: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}
