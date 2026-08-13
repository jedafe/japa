// Package scholarships implements the Scholarship Finder surface
// (Section 73, ADR-073).
//
// Ref: japaconnect-engineering/sections/73-scholarship-finder.md
//      japaconnect-engineering/adr/ADR-073-scholarship-finder.md
package scholarships

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db/gen"
)

// ErrGuideNotFound indicates the target guide is not a published scholarship
// (or does not exist), so it cannot be saved to a user's tracker.
var ErrGuideNotFound = errors.New("scholarships: guide not found or not a published scholarship")

// Status constants for saved scholarship tracker.
const (
	StatusSaved        = "saved"
	StatusResearching  = "researching"
	StatusApplied      = "applied"
	StatusAccepted     = "accepted"
	StatusRejected     = "rejected"
	StatusWithdrawn    = "withdrawn"
)

// Task name constants. The deadline-reminder worker dispatches
// notifications in-process via notifications.Service.CreateNotification
// (§35, ADR-073 §3) rather than enqueuing a separate task — there is no
// `notification:send` task in the asynq catalog (Rule 7).
const (
	TaskCheckDeadlines = "scholarship:check_deadlines"
	TaskAwardPoints    = "wallet:award_points"
)

// TaskEnqueuer enqueues Asynq tasks fire-and-forget (Rule 8).
type TaskEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
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

// ScholarshipResult is a single scholarship in the finder results. The
// flat top-level fields (Institution, Country, LevelOfStudy, AwardAmountUSD,
// ApplicationDeadline, ApplicationURL, IsRenewable, Eligibility,
// HowToApply, ContactEmail, Description) are projected from the
// custom_field_values map so the tools/main app contracts receive a stable
// flat shape. The raw FieldValues map is also returned for the main app
// which renders arbitrary fields.
//
// Ref: japaconnect-engineering/sections/73-scholarship-finder.md
type ScholarshipResult struct {
	GuideID            string            `json:"guide_id"`
	ID                 string            `json:"id"`
	Title              string            `json:"title"`
	Slug               string            `json:"slug"`
	Summary            string            `json:"summary"`
	Description        string            `json:"description"`
	Institution        string            `json:"institution"`
	Country            string            `json:"country"`
	LevelOfStudy       string            `json:"level_of_study"`
	AwardAmountUSD     *float64          `json:"award_amount_usd"`
	IsRenewable        *bool             `json:"is_renewable"`
	ApplicationDeadline string           `json:"application_deadline"`
	ApplicationURL     string            `json:"application_url"`
	Eligibility        string            `json:"eligibility"`
	HowToApply         string            `json:"how_to_apply"`
	ContactEmail       string            `json:"contact_email"`
	FieldValues        map[string]string `json:"field_values"`
}

// SavedScholarshipResponse is the saved-tracker row shape returned by
// GET /scholarships/saved. The nested `Scholarship` object matches the
// flat ScholarshipResult shape so the tools app can read
// `item.scholarship.id`, `item.scholarship.title`, etc.
type SavedScholarshipResponse struct {
	ID               string             `json:"id"`
	Scholarship      ScholarshipResult  `json:"scholarship"`
	Status           string             `json:"status"`
	Notes            string             `json:"notes"`
	RemindBeforeDays int32              `json:"remind_before_days"`
	LastRemindedAt   string             `json:"last_reminded_at"`
	CreatedAt        string             `json:"created_at"`
	UpdatedAt        string             `json:"updated_at"`
}

// AdminDeadlineItem is a single row in the admin upcoming-deadlines report,
// shaped for the admin panel's DeadlineItem contract.
type AdminDeadlineItem struct {
	ID           string  `json:"id"`
	Title        string  `json:"title"`
	Deadline     string  `json:"deadline"`
	DaysUntil    *int    `json:"days_until"`
	Status       string  `json:"status"`
	GuideID      string  `json:"guide_id"`
	UserID       string  `json:"user_id"`
	Username     string  `json:"username"`
}

// AdminStats is the aggregate stats payload for the admin panel.
type AdminStats struct {
	TotalScholarships  int64                       `json:"total_scholarships"`
	UpcomingDeadlines  int64                       `json:"upcoming_deadlines"`
	RemindersSent      int64                       `json:"reminders_sent"`
	UniqueUsers        int64                       `json:"unique_users"`
	Distribution       []gen.AdminScholarshipStatusDistributionRow `json:"distribution"`
}

// projectScholarshipFields populates the flat top-level fields on a
// ScholarshipResult from its FieldValues map. The keys match the
// scholarship custom-field group seeded per §06/ADR-073.
func projectScholarshipFields(r *ScholarshipResult) {
	if r.FieldValues == nil {
		return
	}
	get := func(key string) string {
		if v, ok := r.FieldValues[key]; ok {
			return v
		}
		return ""
	}
	r.Institution = get("provider_institution")
	r.Country = get("countries_eligible")
	r.LevelOfStudy = get("level_of_study")
	r.ApplicationDeadline = get("application_deadline")
	r.ApplicationURL = get("application_link")
	r.Eligibility = get("eligibility_criteria")
	r.HowToApply = get("how_to_apply")
	r.ContactEmail = get("contact_email")
	r.Description = get("description")
	if v := get("award_amount_usd"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			r.AwardAmountUSD = &f
		}
	}
	if v := get("is_renewable"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			r.IsRenewable = &b
		}
	}
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

// Enabled reads the scholarship_finder.enabled system_settings toggle
// (ADR-073, §73). Defaults to true when unset.
func (s *Service) Enabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return true
	}
	return s.settings.GetBoolOr(ctx, tenantID, "scholarship_finder.enabled", true)
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
				JOIN custom_fields cf ON cf.id = cfv.field_id
				WHERE cfv.entity_id = c.id
				  AND cf.key = 'countries_eligible'
				  AND cfv.value_text ILIKE '%%' || $%d || '%%'
			)`, argIdx))
		args = append(args, filters.Country)
		argIdx++
	}

	if filters.Level != "" {
		conditions = append(conditions, fmt.Sprintf(`
			EXISTS (
				SELECT 1 FROM custom_field_values cfv
				JOIN custom_fields cf ON cf.id = cfv.field_id
				WHERE cfv.entity_id = c.id
				  AND cf.key = 'level_of_study'
				  AND cfv.value_text = $%d
			)`, argIdx))
		args = append(args, filters.Level)
		argIdx++
	}

	if filters.Renewable != nil {
		conditions = append(conditions, fmt.Sprintf(`
			EXISTS (
				SELECT 1 FROM custom_field_values cfv
				JOIN custom_fields cf ON cf.id = cfv.field_id
				WHERE cfv.entity_id = c.id
				  AND cf.key = 'is_renewable'
				  AND cfv.value_bool = $%d
			)`, argIdx))
		args = append(args, *filters.Renewable)
		argIdx++
	}

	if filters.Q != "" {
		conditions = append(conditions, fmt.Sprintf("c.title ILIKE '%%' || $%d || '%%'", argIdx))
		args = append(args, filters.Q)
		argIdx++
	}

	// Amount + deadline range filters on custom_field_values. These were
	// previously accepted by the API but never applied to the query.
	if filters.MinAmount > 0 {
		conditions = append(conditions, fmt.Sprintf(`
			EXISTS (
				SELECT 1 FROM custom_field_values cfv
				JOIN custom_fields cf ON cf.id = cfv.field_id
				WHERE cfv.entity_id = c.id
				  AND cf.key = 'award_amount_usd'
				  AND cfv.value_number >= $%d
			)`, argIdx))
		args = append(args, filters.MinAmount)
		argIdx++
	}
	if filters.MaxAmount > 0 {
		conditions = append(conditions, fmt.Sprintf(`
			EXISTS (
				SELECT 1 FROM custom_field_values cfv
				JOIN custom_fields cf ON cf.id = cfv.field_id
				WHERE cfv.entity_id = c.id
				  AND cf.key = 'award_amount_usd'
				  AND cfv.value_number <= $%d
			)`, argIdx))
		args = append(args, filters.MaxAmount)
		argIdx++
	}
	if filters.DeadlineFrom != "" {
		conditions = append(conditions, fmt.Sprintf(`
			EXISTS (
				SELECT 1 FROM custom_field_values cfv
				JOIN custom_fields cf ON cf.id = cfv.field_id
				WHERE cfv.entity_id = c.id
				  AND cf.key = 'application_deadline'
				  AND cfv.value_text >= $%d
			)`, argIdx))
		args = append(args, filters.DeadlineFrom)
		argIdx++
	}
	if filters.DeadlineTo != "" {
		conditions = append(conditions, fmt.Sprintf(`
			EXISTS (
				SELECT 1 FROM custom_field_values cfv
				JOIN custom_fields cf ON cf.id = cfv.field_id
				WHERE cfv.entity_id = c.id
				  AND cf.key = 'application_deadline'
				  AND cfv.value_text <= $%d
			)`, argIdx))
		args = append(args, filters.DeadlineTo)
		argIdx++
	}

	// Sort order
	var orderBy string
	switch filters.Sort {
	case "deadline_asc":
		orderBy = `(SELECT cfv.value_text FROM custom_field_values cfv JOIN custom_fields cf ON cf.id = cfv.field_id WHERE cfv.entity_id = c.id AND cf.key = 'application_deadline') ASC NULLS LAST`
	case "amount_desc":
		orderBy = `(SELECT cfv.value_number FROM custom_field_values cfv JOIN custom_fields cf ON cf.id = cfv.field_id WHERE cfv.entity_id = c.id AND cf.key = 'award_amount_usd') DESC NULLS LAST`
	default:
		orderBy = "c.created_at DESC"
	}

	whereClause := strings.Join(conditions, " AND ")

	// Single query with COUNT(*) OVER() to get the total count and the
	// page rows in one pass — avoids running the filter subqueries twice
	// (BUG-016: previously a separate count query re-executed all EXISTS
	// subqueries). The window function appends the full-result-set count
	// to every row; we read it from the first row (it's identical across
	// rows in the same result set).
	dataQuery := fmt.Sprintf(`
		SELECT c.id, c.title, c.slug, c.meta_description,
			COALESCE(
				(SELECT json_object_agg(cf.key,
					CASE
						WHEN cfv.value_text IS NOT NULL THEN cfv.value_text
						WHEN cfv.value_number IS NOT NULL THEN cfv.value_number::text
						WHEN cfv.value_bool IS NOT NULL THEN cfv.value_bool::text
						ELSE ''
					END)
				 FROM custom_field_values cfv
				 JOIN custom_fields cf ON cf.id = cfv.field_id
				 WHERE cfv.entity_id = c.id
				   AND COALESCE(cfv.value_text, cfv.value_number::text, cfv.value_bool::text) != ''
				), '{}'::json
			) AS field_values,
			COUNT(*) OVER() AS total_count
		FROM contents c
		WHERE %s
		ORDER BY %s
		LIMIT $%d OFFSET $%d`,
		whereClause, orderBy, argIdx, argIdx+1)
	args = append(args, filters.Limit, offset)

	// Execute the combined query.
	pool := s.store.pool.Reader
	rows, err := pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("scholarships: data query: %w", err)
	}
	defer rows.Close()

	var results []ScholarshipResult
	var total int64
	for rows.Next() {
		var r ScholarshipResult
		var fieldValuesJSON []byte
		var summary *string
		var rowCount int64
		if err := rows.Scan(&r.GuideID, &r.Title, &r.Slug, &summary, &fieldValuesJSON, &rowCount); err != nil {
			return nil, 0, fmt.Errorf("scholarships: scan row: %w", err)
		}
		// COUNT(*) OVER() is the same on every row; capture it from the
		// first row. When the result set is empty the loop body never
		// runs and total stays 0.
		if rowCount > 0 {
			total = rowCount
		}
		if summary != nil {
			r.Summary = *summary
		}
		if err := json.Unmarshal(fieldValuesJSON, &r.FieldValues); err != nil {
			r.FieldValues = map[string]string{}
		}
		r.ID = r.GuideID
		projectScholarshipFields(&r)
		results = append(results, r)
	}

	return results, total, nil
}

// GetScholarshipDetail returns a single scholarship guide with all field values.
func (s *Service) GetScholarshipDetail(ctx context.Context, tenantID pgtype.UUID, guideID pgtype.UUID) (ScholarshipResult, error) {
	query := `
		SELECT c.id, c.title, c.slug, c.meta_description,
			COALESCE(
				(SELECT json_object_agg(cf.key,
					CASE
						WHEN cfv.value_text IS NOT NULL THEN cfv.value_text
						WHEN cfv.value_number IS NOT NULL THEN cfv.value_number::text
						WHEN cfv.value_bool IS NOT NULL THEN cfv.value_bool::text
						ELSE ''
					END)
				 FROM custom_field_values cfv
				 JOIN custom_fields cf ON cf.id = cfv.field_id
				 WHERE cfv.entity_id = c.id
				   AND COALESCE(cfv.value_text, cfv.value_number::text, cfv.value_bool::text) != ''
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
	r.ID = r.GuideID
	projectScholarshipFields(&r)
	return r, nil
}

// SaveScholarship saves a scholarship to the user's tracker. The guide is
// validated to be a published scholarship guide first, and a duplicate save
// (ON CONFLICT DO NOTHING) returns the existing tracker row instead of an
// error so repeated saves are idempotent.
func (s *Service) SaveScholarship(ctx context.Context, tenantID, userID, guideID pgtype.UUID) (gen.SavedScholarship, error) {
	// Validate the target is a published scholarship guide before saving.
	// Without this, any content ID (draft, unpublished, or non-scholarship)
	// could be saved to the tracker.
	if _, err := s.GetScholarshipDetail(ctx, tenantID, guideID); err != nil {
		return gen.SavedScholarship{}, ErrGuideNotFound
	}

	defaultReminderDays := int32(s.settings.GetIntOr(ctx, tenantID, "scholarship_finder.default_reminder_days", 14))

	saved, err := s.store.SaveScholarship(ctx, gen.SaveScholarshipParams{
		TenantID:         tenantID,
		UserID:           userID,
		GuideID:          guideID,
		RemindBeforeDays: defaultReminderDays,
	})
	if err != nil {
		// ON CONFLICT (tenant_id, user_id, guide_id) DO NOTHING returns no
		// row when the tracker entry already exists; surface the existing
		// row so a repeat save is idempotent rather than a 400.
		if errors.Is(err, pgx.ErrNoRows) {
			existing, gerr := s.store.GetSavedScholarship(ctx, gen.GetSavedScholarshipParams{
				TenantID: tenantID,
				UserID:   userID,
				GuideID:  guideID,
			})
			if gerr != nil {
				return gen.SavedScholarship{}, gerr
			}
			return existing, nil
		}
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
// `notes` is a pointer so callers can clear notes with an explicit empty
// string (nil means "leave unchanged", matching the SQL COALESCE update).
func (s *Service) UpdateSavedScholarship(ctx context.Context, tenantID, userID, guideID pgtype.UUID, status string, notes *string, remindBeforeDays *int32) (gen.SavedScholarship, error) {
	if status != "" && !IsValidStatus(status) {
		return gen.SavedScholarship{}, fmt.Errorf("scholarships: invalid status: %s", status)
	}

	var statusArg *string
	if status != "" {
		statusArg = &status
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
		Notes:            notes,
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

// ListSavedScholarships returns a user's saved scholarships as
// SavedScholarshipResponse rows (nested scholarship object). Each row
// fetches the scholarship detail to populate the flat fields.
func (s *Service) ListSavedScholarships(ctx context.Context, tenantID, userID pgtype.UUID, status string, page, limit int) ([]SavedScholarshipResponse, int64, error) {
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

	out := make([]SavedScholarshipResponse, 0, len(rows))
	for _, row := range rows {
		sch, err := s.GetScholarshipDetail(ctx, tenantID, row.GuideID)
		if err != nil {
			// Fallback: build a minimal scholarship from the row's joined
			// guide_title/guide_slug so a missing detail doesn't drop the
			// saved entry from the tracker list.
			sch = ScholarshipResult{
				GuideID: pgtypeUUIDString(row.GuideID),
				ID:      pgtypeUUIDString(row.GuideID),
				Title:   row.GuideTitle,
				Slug:    row.GuideSlug,
			}
		}
		out = append(out, SavedScholarshipResponse{
			ID:               pgtypeUUIDString(row.ID),
			Scholarship:      sch,
			Status:           row.Status,
			Notes:            pgtypeStringRef(row.Notes),
			RemindBeforeDays: row.RemindBeforeDays,
			LastRemindedAt:   pgtypeTimestampString(row.LastRemindedAt),
			CreatedAt:        pgtypeTimestampString(row.CreatedAt),
			UpdatedAt:        pgtypeTimestampString(row.UpdatedAt),
		})
	}
	return out, total, nil
}

// AdminStats returns aggregate save counts, upcoming-deadline count,
// reminders-sent count, unique-users count, and the per-status
// distribution for the admin panel.
func (s *Service) AdminStats(ctx context.Context, tenantID pgtype.UUID) (AdminStats, error) {
	dist, err := s.store.AdminStatusDistribution(ctx, tenantID)
	if err != nil {
		return AdminStats{}, err
	}
	var total, remindersSent int64
	for _, d := range dist {
		total += d.Count
	}
	// Upcoming deadlines + reminders sent come from the deadlines +
	// reminder-log stores. We reuse the existing paginated stores with a
	// large page to get aggregate counts (acceptable for v2 scale per
	// ADR-073; a dedicated count query is the future migration path).
	dl, err := s.store.AdminUpcomingDeadlines(ctx, gen.AdminScholarshipUpcomingDeadlinesParams{
		TenantID:   tenantID,
		PageLimit:  1000,
		PageOffset: 0,
	})
	if err != nil {
		return AdminStats{}, err
	}
	upcoming := int64(len(dl))
	rl, err := s.store.AdminReminderLog(ctx, gen.AdminScholarshipReminderLogParams{
		TenantID:   tenantID,
		PageLimit:  1000,
		PageOffset: 0,
	})
	if err != nil {
		return AdminStats{}, err
	}
	remindersSent = int64(len(rl))
	uniqueUsers := int64(len(uniqueUserIDsFromDeadlines(dl)))

	return AdminStats{
		TotalScholarships: total,
		UpcomingDeadlines: upcoming,
		RemindersSent:     remindersSent,
		UniqueUsers:       uniqueUsers,
		Distribution:      dist,
	}, nil
}

// AdminDeadlines returns the upcoming-deadline report mapped to the
// AdminDeadlineItem shape (id, title, deadline, days_until, status) that
// the admin panel's DeadlineItem contract expects.
func (s *Service) AdminDeadlines(ctx context.Context, tenantID pgtype.UUID, page, limit int) ([]AdminDeadlineItem, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := int32((page - 1) * limit)

	rows, err := s.store.AdminUpcomingDeadlines(ctx, gen.AdminScholarshipUpcomingDeadlinesParams{
		TenantID:   tenantID,
		PageLimit:  int32(limit),
		PageOffset: offset,
	})
	if err != nil {
		return nil, err
	}

	out := make([]AdminDeadlineItem, 0, len(rows))
	now := time.Now().UTC()
	for _, row := range rows {
		item := AdminDeadlineItem{
			ID:       pgtypeUUIDString(row.ID),
			Title:    row.GuideTitle,
			Deadline: pgtypeStringRef(row.DeadlineValue),
			Status:   row.Status,
			GuideID:  pgtypeUUIDString(row.GuideID),
			UserID:   pgtypeUUIDString(row.UserID),
			Username: row.Username,
		}
		if row.DeadlineValue != nil && *row.DeadlineValue != "" {
			if t, err := time.Parse("2006-01-02", *row.DeadlineValue); err == nil {
				days := int(t.Sub(now).Hours() / 24)
				item.DaysUntil = &days
			} else if t, err := time.Parse(time.RFC3339, *row.DeadlineValue); err == nil {
				days := int(t.Sub(now).Hours() / 24)
				item.DaysUntil = &days
			}
		}
		out = append(out, item)
	}
	return out, nil
}

// uniqueUserIDsFromDeadlines returns the distinct set of user IDs from a
// deadlines row slice (used for the admin unique-users stat).
func uniqueUserIDsFromDeadlines(rows []gen.AdminScholarshipUpcomingDeadlinesRow) []pgtype.UUID {
	seen := make(map[[16]byte]struct{}, len(rows))
	out := make([]pgtype.UUID, 0, len(rows))
	for _, r := range rows {
		if !r.UserID.Valid {
			continue
		}
		var key [16]byte
		copy(key[:], r.UserID.Bytes[:])
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, r.UserID)
	}
	return out
}

// pgtypeUUIDString returns the canonical string form of a pgtype.UUID,
// or "" when invalid.
func pgtypeUUIDString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return id.String()
}

// pgtypeStringRef dereferences a *string, returning "" for nil.
func pgtypeStringRef(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// pgtypeTimestampString returns the ISO8601 UTC string for a pgtype
// timestamptz, or "" when invalid.
func pgtypeTimestampString(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339)
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
//
// The store query returns every saved scholarship with a non-null deadline
// and no recent reminder; this method additionally enforces the
// remind_before_days window so reminders are only dispatched for deadlines
// that are still ahead and within the user's configured lead time. Without
// this filter the daily sweep would notify users about far-future deadlines
// (and re-notify every day), which is notification spam (ADR-073 §3).
func (s *Service) FindNeedingReminders(ctx context.Context, tenantID pgtype.UUID, batchLimit int32) ([]gen.FindScholarshipsNeedingRemindersRow, error) {
	rows, err := s.store.FindNeedingReminders(ctx, gen.FindScholarshipsNeedingRemindersParams{
		TenantID:   tenantID,
		BatchLimit: batchLimit,
	})
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	out := make([]gen.FindScholarshipsNeedingRemindersRow, 0, len(rows))

	for _, r := range rows {
		if !withinReminderWindow(r, now) {
			continue
		}

		out = append(out, r)
	}

	return out, nil
}

// withinReminderWindow reports whether a saved scholarship should receive a
// deadline reminder right now: the deadline must be parseable, still ahead,
// and within the user's configured remind_before_days lead time.
func withinReminderWindow(r gen.FindScholarshipsNeedingRemindersRow, now time.Time) bool {
	if r.DeadlineValue == "" {
		return false
	}

	deadline, ok := parseDeadlineValue(r.DeadlineValue)
	if !ok {
		// Unparseable deadline — skip rather than emit a malformed reminder.
		return false
	}

	// Compare by calendar day (UTC) so a deadline later today still counts as
	// "ahead of" now rather than being treated as already passed.
	deadlineDay := time.Date(deadline.Year(), deadline.Month(), deadline.Day(), 0, 0, 0, 0, time.UTC)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	if deadlineDay.Before(today) {
		return false
	}

	windowEnd := today.AddDate(0, 0, int(r.RemindBeforeDays))

	return !deadlineDay.After(windowEnd)
}

// parseDeadlineValue parses an application_deadline custom-field value, which
// is stored as text in either YYYY-MM-DD or RFC3339 form (matching the admin
// deadline report parsing in AdminDeadlines).
func parseDeadlineValue(v string) (time.Time, bool) {
	if t, err := time.Parse("2006-01-02", v); err == nil {
		return t, true
	}

	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, true
	}

	return time.Time{}, false
}

// MarkReminderSent updates last_reminded_at for a saved scholarship.
func (s *Service) MarkReminderSent(ctx context.Context, id pgtype.UUID) error {
	return s.store.MarkReminderSent(ctx, id)
}
