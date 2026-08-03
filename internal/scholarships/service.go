// Package scholarships implements the Scholarship Finder surface
// (Section 73, ADR-073).
//
// Ref: japaconnect-engineering/sections/73-scholarship-finder.md
//      japaconnect-engineering/adr/ADR-073-scholarship-finder.md
package scholarships

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db/gen"
)

// Status constants for saved scholarship tracker.
const (
	StatusSaved        = "saved"
	StatusResearching  = "researching"
	StatusApplied      = "applied"
	StatusAccepted     = "accepted"
	StatusRejected     = "rejected"
	StatusWithdrawn    = "withdrawn"
)

// Task name constants.
const (
	TaskCheckDeadlines  = "scholarship:check_deadlines"
	TaskAwardPoints     = "wallet:award_points"
	TaskSendNotification = "notification:send"
)

// TaskEnqueuer enqueues Asynq tasks fire-and-forget (Rule 8).
type TaskEnqueuer interface {
	Enqueue(taskType string, payload []byte) error
}

// ScholarshipFilters holds the finder query parameters.
type ScholarshipFilters struct {
	Country      string
	Level        string
	MinAmount    float64
	MaxAmount    float64
	Renewable    *bool
	DeadlineFrom string
	DeadlineTo   string
	Q            string
	Sort         string
	Page         int
	Limit        int
}

// ScholarshipResult is a single scholarship in the finder results.
type ScholarshipResult struct {
	GuideID     string            `json:"guide_id"`
	Title       string            `json:"title"`
	Slug        string            `json:"slug"`
	Summary     string            `json:"summary"`
	FieldValues map[string]string `json:"field_values"`
}

// IsValidStatus checks if a status value is valid.
func IsValidStatus(status string) bool {
	switch status {
	case StatusSaved, StatusResearching, StatusApplied, StatusAccepted, StatusRejected, StatusWithdrawn:
		return true
	}
	return false
}

// Service implements the scholarship finder business logic.
type Service struct {
	store    *Store
	settings *config.Settings
	enqueuer TaskEnqueuer
}

// NewService creates a scholarships service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{store: store, settings: settings}
}

// SetEnqueuer injects the Asynq task enqueuer.
func (s *Service) SetEnqueuer(e TaskEnqueuer) { s.enqueuer = e }

// FindScholarships queries published scholarship guides with server-side
// filtering on custom_field_values. Uses a direct pgx query for dynamic
// filtering.
func (s *Service) FindScholarships(ctx context.Context, tenantID pgtype.UUID, filters ScholarshipFilters) ([]ScholarshipResult, int64, error) {
	if filters.Limit <= 0 || filters.Limit > 100 {
		filters.Limit = 20
	}
	if filters.Page <= 0 {
		filters.Page = 1
	}
	offset := (filters.Page - 1) * filters.Limit

	// Build dynamic WHERE clauses for custom_field_values filtering.
	// Each filter adds an EXISTS subquery on custom_field_values.
	var conditions []string
	var args []interface{}
	argIdx := 1

	args = append(args, tenantID)
	tenantIdx := argIdx
	argIdx++

	conditions = append(conditions, fmt.Sprintf("c.tenant_id = $%d", tenantIdx))
	conditions = append(conditions, "c.content_type = 'guide'")
	conditions = append(conditions, "c.guide_category = 'scholarship'")
	conditions = append(conditions, "c.status = 'published'")
	conditions = append(conditions, "c.deleted_at IS NULL")

	if filters.Country != "" {
		conditions = append(conditions, fmt.Sprintf(`
			EXISTS (
				SELECT 1 FROM custom_field_values cfv
				WHERE cfv.content_id = c.id
				  AND cfv.field_key = 'countries_eligible'
				  AND cfv.value_text ILIKE '%%' || $%d || '%%'
			)`, argIdx))
		args = append(args, filters.Country)
		argIdx++
	}

	if filters.Level != "" {
		conditions = append(conditions, fmt.Sprintf(`
			EXISTS (
				SELECT 1 FROM custom_field_values cfv
				WHERE cfv.content_id = c.id
				  AND cfv.field_key = 'level_of_study'
				  AND cfv.value_text = $%d
			)`, argIdx))
		args = append(args, filters.Level)
		argIdx++
	}

	if filters.Renewable != nil {
		conditions = append(conditions, fmt.Sprintf(`
			EXISTS (
				SELECT 1 FROM custom_field_values cfv
				WHERE cfv.content_id = c.id
				  AND cfv.field_key = 'is_renewable'
				  AND cfv.value_boolean = $%d
			)`, argIdx))
		args = append(args, *filters.Renewable)
		argIdx++
	}

	if filters.Q != "" {
		conditions = append(conditions, fmt.Sprintf("c.title ILIKE '%%' || $%d || '%%'", argIdx))
		args = append(args, filters.Q)
		argIdx++
	}

	// Sort order
	var orderBy string
	switch filters.Sort {
	case "deadline_asc":
		orderBy = `(SELECT cfv.value_text FROM custom_field_values cfv WHERE cfv.content_id = c.id AND cfv.field_key = 'application_deadline') ASC NULLS LAST`
	case "amount_desc":
		orderBy = `(SELECT cfv.value_number FROM custom_field_values cfv WHERE cfv.content_id = c.id AND cfv.field_key = 'award_amount_usd') DESC NULLS LAST`
	default:
		orderBy = "c.created_at DESC"
	}

	whereClause := strings.Join(conditions, " AND ")

	// Count query
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM contents c WHERE %s`, whereClause)
	// Data query with field values extraction
	dataQuery := fmt.Sprintf(`
		SELECT c.id, c.title, c.slug, c.summary,
			COALESCE(
				(SELECT json_object_agg(cfv.field_key, cfv.value_text)
				 FROM custom_field_values cfv
				 WHERE cfv.content_id = c.id
				   AND cfv.value_text IS NOT NULL AND cfv.value_text != ''
				), '{}'::json
			) AS field_values
		FROM contents c
		WHERE %s
		ORDER BY %s
		LIMIT $%d OFFSET $%d`,
		whereClause, orderBy, argIdx, argIdx+1)
	args = append(args, filters.Limit, offset)

	// Execute count
	var total int64
	pool := s.store.pool.Reader
	err := pool.QueryRow(ctx, countQuery, args[:len(args)-2]...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("scholarships: count query: %w", err)
	}

	// Execute data query
	rows, err := pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("scholarships: data query: %w", err)
	}
	defer rows.Close()

	var results []ScholarshipResult
	for rows.Next() {
		var r ScholarshipResult
		var fieldValuesJSON []byte
		var summary *string
		if err := rows.Scan(&r.GuideID, &r.Title, &r.Slug, &summary, &fieldValuesJSON); err != nil {
			return nil, 0, fmt.Errorf("scholarships: scan row: %w", err)
		}
		if summary != nil {
			r.Summary = *summary
		}
		if err := json.Unmarshal(fieldValuesJSON, &r.FieldValues); err != nil {
			r.FieldValues = map[string]string{}
		}
		results = append(results, r)
	}

	return results, total, nil
}

// GetScholarshipDetail returns a single scholarship guide with all field values.
func (s *Service) GetScholarshipDetail(ctx context.Context, tenantID pgtype.UUID, guideID pgtype.UUID) (ScholarshipResult, error) {
	query := `
		SELECT c.id, c.title, c.slug, c.summary,
			COALESCE(
				(SELECT json_object_agg(cfv.field_key, cfv.value_text)
				 FROM custom_field_values cfv
				 WHERE cfv.content_id = c.id
				   AND cfv.value_text IS NOT NULL AND cfv.value_text != ''
				), '{}'::json
			) AS field_values
		FROM contents c
		WHERE c.tenant_id = $1 AND c.id = $2
		  AND c.content_type = 'guide' AND c.guide_category = 'scholarship'
		  AND c.status = 'published' AND c.deleted_at IS NULL`

	var r ScholarshipResult
	var fieldValuesJSON []byte
	var summary *string
	err := s.store.pool.Reader.QueryRow(ctx, query, tenantID, guideID).Scan(
		&r.GuideID, &r.Title, &r.Slug, &summary, &fieldValuesJSON,
	)
	if err != nil {
		return ScholarshipResult{}, fmt.Errorf("scholarships: get detail: %w", err)
	}
	if summary != nil {
		r.Summary = *summary
	}
	if err := json.Unmarshal(fieldValuesJSON, &r.FieldValues); err != nil {
		r.FieldValues = map[string]string{}
	}
	return r, nil
}

// SaveScholarship saves a scholarship to the user's tracker.
func (s *Service) SaveScholarship(ctx context.Context, tenantID, userID, guideID pgtype.UUID) (gen.SavedScholarship, error) {
	defaultReminderDays := int32(s.settings.GetIntOr(ctx, tenantID, "scholarship_finder.default_reminder_days", 14))

	saved, err := s.store.SaveScholarship(ctx, gen.SaveScholarshipParams{
		TenantID:         tenantID,
		UserID:           userID,
		GuideID:          guideID,
		RemindBeforeDays: defaultReminderDays,
	})
	if err != nil {
		return gen.SavedScholarship{}, err
	}

	// Award completion points if this is the user's first save (fire-and-forget).
	if s.enqueuer != nil {
		completionPoints := s.settings.GetIntOr(ctx, tenantID, "scholarship_finder.completion_points", 10)
		if completionPoints > 0 {
			payload, _ := json.Marshal(map[string]interface{}{
				"tenant_id": tenantID.String(),
				"user_id":   userID.String(),
				"amount":    completionPoints,
				"reason":    "scholarship_saved",
			})
			if err := s.enqueuer.Enqueue(TaskAwardPoints, payload); err != nil {
				slog.Warn("scholarships: enqueue award points failed", "error", err)
			}
		}
	}

	return saved, nil
}

// UpdateSavedScholarship updates the status/notes/reminder_days.
func (s *Service) UpdateSavedScholarship(ctx context.Context, tenantID, userID, guideID pgtype.UUID, status, notes string, remindBeforeDays *int32) (gen.SavedScholarship, error) {
	if status != "" && !IsValidStatus(status) {
		return gen.SavedScholarship{}, fmt.Errorf("scholarships: invalid status: %s", status)
	}

	var statusArg, notesArg *string
	if status != "" {
		statusArg = &status
	}
	if notes != "" {
		notesArg = &notes
	}

	var remindArg *int32
	if remindBeforeDays != nil {
		remindArg = remindBeforeDays
	}

	return s.store.UpdateSavedScholarship(ctx, gen.UpdateSavedScholarshipParams{
		TenantID:         tenantID,
		UserID:           userID,
		GuideID:          guideID,
		Status:           statusArg,
		Notes:            notesArg,
		RemindBeforeDays: remindArg,
	})
}

// DeleteSavedScholarship removes a scholarship from the tracker.
func (s *Service) DeleteSavedScholarship(ctx context.Context, tenantID, userID, guideID pgtype.UUID) error {
	return s.store.DeleteSavedScholarship(ctx, gen.DeleteSavedScholarshipParams{
		TenantID: tenantID,
		UserID:   userID,
		GuideID:  guideID,
	})
}

// ListSavedScholarships returns a user's saved scholarships.
func (s *Service) ListSavedScholarships(ctx context.Context, tenantID, userID pgtype.UUID, status string, page, limit int) ([]gen.ListSavedScholarshipsRow, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := int32((page - 1) * limit)

	rows, err := s.store.ListSavedScholarships(ctx, gen.ListSavedScholarshipsParams{
		TenantID:  tenantID,
		UserID:    userID,
		Status:    status,
		PageLimit: int32(limit),
		PageOffset: offset,
	})
	if err != nil {
		return nil, 0, err
	}

	total, err := s.store.CountSavedScholarships(ctx, gen.CountSavedScholarshipsParams{
		TenantID: tenantID,
		UserID:   userID,
		Status:   status,
	})
	if err != nil {
		return nil, 0, err
	}

	return rows, total, nil
}

// AdminStatusDistribution returns the status distribution of saved scholarships.
func (s *Service) AdminStatusDistribution(ctx context.Context, tenantID pgtype.UUID) ([]gen.AdminScholarshipStatusDistributionRow, error) {
	return s.store.AdminStatusDistribution(ctx, tenantID)
}

// AdminTopSaved returns the most-saved scholarships.
func (s *Service) AdminTopSaved(ctx context.Context, tenantID pgtype.UUID, page, limit int) ([]gen.AdminScholarshipTopSavedRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := int32((page - 1) * limit)
	return s.store.AdminTopSaved(ctx, gen.AdminScholarshipTopSavedParams{
		TenantID:   tenantID,
		PageLimit:  int32(limit),
		PageOffset: offset,
	})
}

// AdminUpcomingDeadlines returns upcoming deadline report.
func (s *Service) AdminUpcomingDeadlines(ctx context.Context, tenantID pgtype.UUID, page, limit int) ([]gen.AdminScholarshipUpcomingDeadlinesRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := int32((page - 1) * limit)
	return s.store.AdminUpcomingDeadlines(ctx, gen.AdminScholarshipUpcomingDeadlinesParams{
		TenantID:   tenantID,
		PageLimit:  int32(limit),
		PageOffset: offset,
	})
}

// AdminReminderLog returns the reminder delivery history.
func (s *Service) AdminReminderLog(ctx context.Context, tenantID pgtype.UUID, page, limit int) ([]gen.AdminScholarshipReminderLogRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := int32((page - 1) * limit)
	return s.store.AdminReminderLog(ctx, gen.AdminScholarshipReminderLogParams{
		TenantID:   tenantID,
		PageLimit:  int32(limit),
		PageOffset: offset,
	})
}

// FindNeedingReminders returns saved scholarships needing deadline reminders.
// Called by the scholarship:check_deadlines worker task.
func (s *Service) FindNeedingReminders(ctx context.Context, tenantID pgtype.UUID, batchLimit int32) ([]gen.FindScholarshipsNeedingRemindersRow, error) {
	return s.store.FindNeedingReminders(ctx, gen.FindScholarshipsNeedingRemindersParams{
		TenantID:   tenantID,
		BatchLimit: batchLimit,
	})
}

// MarkReminderSent updates last_reminded_at for a saved scholarship.
func (s *Service) MarkReminderSent(ctx context.Context, id pgtype.UUID) error {
	return s.store.MarkReminderSent(ctx, id)
}
