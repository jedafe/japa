// Package scholarships implements the Scholarship Finder surface
// (Section 73, ADR-073).
// // Ref: japaconnect-engineering/sections/73-scholarship-finder.md
// //	japaconnect-engineering/adr/ADR-073-scholarship-finder.md
package scholarships

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
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
	StatusSaved       = "saved"
	StatusResearching = "researching"
	StatusApplied     = "applied"
	StatusAccepted    = "accepted"
	StatusRejected    = "rejected"
	StatusWithdrawn   = "withdrawn"
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
	Country        string
	Level          string
	MinAmount      float64
	MaxAmount      float64
	Renewable      *bool
	DeadlineFrom   string
	DeadlineTo     string
	IncludeExpired bool
	Q              string
	Sort           string
	Page           int
	Limit          int
}

// ScholarshipResult is a single scholarship in the finder results. The
// flat top-level fields (Institution, Country, LevelOfStudy, AwardAmountUSD,
// ApplicationDeadline, ApplicationURL, IsRenewable, Eligibility,
// HowToApply, ContactEmail, Description) are projected from the
// custom_field_values map so the tools/main app contracts receive a stable
// flat shape. The raw FieldValues map is also returned for the main app
// which renders arbitrary fields.
// // Ref: japaconnect-engineering/sections/73-scholarship-finder.md.
// SourceAttribution is a lightweight source snapshot attached to scholarship,
// job, and remittance results so the UI can link back to /sources/[slug].
type SourceAttribution struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Slug       string `json:"slug"`
	LogoURL    string `json:"logo_url"`
	SourceURL  string `json:"source_url"`
	TrustLevel int16  `json:"trust_level"`
}

type ScholarshipResult struct {
	GuideID             string            `json:"guide_id"`
	ID                  string            `json:"id"`
	IntelRecordID       string            `json:"intel_record_id"`
	Title               string            `json:"title"`
	Slug                string            `json:"slug"`
	Summary             string            `json:"summary"`
	Description         string            `json:"description"`
	Institution         string            `json:"institution"`
	Country             string            `json:"country"`
	CountrySpaceID      string            `json:"country_space_id"`
	FieldsOfStudy       []string          `json:"fields_of_study"`
	LevelOfStudy        string            `json:"level_of_study"`
	AwardAmountUSD      *float64          `json:"award_amount_usd"`
	IsRenewable         *bool             `json:"is_renewable"`
	ApplicationDeadline string            `json:"application_deadline"`
	ApplicationURL      string            `json:"application_url"`
	Eligibility         string            `json:"eligibility"`
	HowToApply          string            `json:"how_to_apply"`
	ContactEmail        string            `json:"contact_email"`
	FieldValues         map[string]string `json:"field_values"`
	Source              SourceAttribution `json:"source"`
}

// SavedScholarshipResponse is the saved-tracker row shape returned by
// GET /scholarships/saved. The nested `Scholarship` object matches the
// flat ScholarshipResult shape so the tools app can read
// `item.scholarship.id`, `item.scholarship.title`, etc.
type SavedScholarshipResponse struct {
	ID               string            `json:"id"`
	Scholarship      ScholarshipResult `json:"scholarship"`
	Status           string            `json:"status"`
	Notes            string            `json:"notes"`
	RemindBeforeDays int32             `json:"remind_before_days"`
	LastRemindedAt   string            `json:"last_reminded_at"`
	CreatedAt        string            `json:"created_at"`
	UpdatedAt        string            `json:"updated_at"`
}

// AdminDeadlineItem is a single row in the admin upcoming-deadlines report,
// shaped for the admin panel's DeadlineItem contract.
type AdminDeadlineItem struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Deadline  string `json:"deadline"`
	DaysUntil *int   `json:"days_until"`
	Status    string `json:"status"`
	GuideID   string `json:"guide_id"`
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
}

// AdminStats is the aggregate stats payload for the admin panel.
type AdminStats struct {
	TotalScholarships int64                                       `json:"total_scholarships"`
	UpcomingDeadlines int64                                       `json:"upcoming_deadlines"`
	RemindersSent     int64                                       `json:"reminders_sent"`
	UniqueUsers       int64                                       `json:"unique_users"`
	Distribution      []gen.AdminScholarshipStatusDistributionRow `json:"distribution"`
}

// projectScholarshipFields fills in any flat top-level fields on a
// ScholarshipResult that were not already populated from the structured
// scholarships table columns. The keys match the scholarship custom-field
// group seeded per §06/ADR-073. Structured columns take precedence so a
// guide-synced projection (which writes provider_institution, level_of_study,
// country, etc. into the scholarships table) is not overwritten by the raw
// comma-separated field_values string.
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

	if r.Institution == "" {
		r.Institution = get("provider_institution")
	}
	if r.Country == "" {
		r.Country = get("countries_eligible")
	}
	if r.LevelOfStudy == "" {
		r.LevelOfStudy = get("level_of_study")
	}
	if r.ApplicationDeadline == "" {
		r.ApplicationDeadline = get("application_deadline")
	}
	if r.ApplicationURL == "" {
		r.ApplicationURL = get("application_link")
	}
	if r.Eligibility == "" {
		r.Eligibility = get("eligibility_criteria")
	}
	if r.HowToApply == "" {
		r.HowToApply = get("how_to_apply")
	}
	if r.ContactEmail == "" {
		r.ContactEmail = get("contact_email")
	}
	if r.Description == "" {
		r.Description = get("description")
	}

	if r.AwardAmountUSD == nil {
		if v := get("award_amount_usd"); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				r.AwardAmountUSD = &f
			}
		}
	}

	if r.IsRenewable == nil {
		if v := get("is_renewable"); v != "" {
			if b, err := strconv.ParseBool(v); err == nil {
				r.IsRenewable = &b
			}
		}
	}
}

// renewableFromFunding extracts the is_renewable value from the funding
// jsonb blob. It accepts string "true"/"false" or boolean values.
func renewableFromFunding(funding []byte) (bool, bool) {
	if len(funding) == 0 || string(funding) == "{}" {
		return false, false
	}

	var f map[string]interface{}
	if err := json.Unmarshal(funding, &f); err != nil {
		return false, false
	}

	v, ok := f["is_renewable"]
	if !ok {
		return false, false
	}

	switch n := v.(type) {
	case bool:
		return n, true
	case string:
		b, err := strconv.ParseBool(n)
		if err != nil {
			return false, false
		}
		return b, true
	}

	return false, false
}

// mergeContentFieldValues flattens a contents.field_values JSONB blob
// (values may be plain strings, numbers, booleans, or {"value": "..."}
// objects) into the destination map. This lets backfilled scholarship
// guides keep their original custom-field values (eligibility, how_to_apply,
// contact_email, award_amount_usd, is_renewable) alongside the structured
// scholarships table.
func mergeContentFieldValues(dst map[string]string, fieldValues []byte) {
	if len(fieldValues) == 0 || string(fieldValues) == "{}" {
		return
	}

	var m map[string]interface{}
	if err := json.Unmarshal(fieldValues, &m); err != nil {
		return
	}

	for k, v := range m {
		if v == nil {
			continue
		}
		switch t := v.(type) {
		case string:
			if s := strings.TrimSpace(t); s != "" {
				dst[k] = s
			}
		case float64:
			dst[k] = strconv.FormatFloat(t, 'f', -1, 64)
		case int:
			dst[k] = strconv.Itoa(t)
		case int64:
			dst[k] = strconv.FormatInt(t, 10)
		case bool:
			dst[k] = strconv.FormatBool(t)
		case map[string]interface{}:
			if s, ok := t["value"].(string); ok && strings.TrimSpace(s) != "" {
				dst[k] = strings.TrimSpace(s)
			}
		case []interface{}:
			if s := joinInterfaceArray(t, ", "); s != "" {
				dst[k] = s
			}
		}
	}
}

// populateScholarshipResult fills the flat top-level fields and the
// field_values map on a ScholarshipResult from the `scholarships` table
// columns (ADR-112). The field_values keys match the legacy
// custom_field_values keys (countries_eligible, level_of_study,
// application_deadline, application_link, provider_institution,
// award_amount_usd, description) so the main app's `field()` helper and
// the existing card badges continue to render without a frontend contract
// change.
func populateScholarshipResult(
	r *ScholarshipResult,
	country string,
	degreeLevel *string,
	deadline pgtype.Date,
	applicationURL, university *string,
	eligibleCountries []string,
	funding []byte,
	description *string,
	fieldValues []byte,
) {
	fv := map[string]string{}
	r.Country = country

	if country != "" {
		fv["countries_eligible"] = country
	}

	if degreeLevel != nil && *degreeLevel != "" {
		r.LevelOfStudy = *degreeLevel
		fv["level_of_study"] = *degreeLevel
	}

	if deadline.Valid {
		ds := deadline.Time.Format("2006-01-02")
		r.ApplicationDeadline = ds
		fv["application_deadline"] = ds
	}

	if applicationURL != nil && *applicationURL != "" {
		r.ApplicationURL = *applicationURL
		fv["application_link"] = *applicationURL
	}

	if university != nil && *university != "" {
		r.Institution = *university
		fv["provider_institution"] = *university
	}

	if description != nil && *description != "" {
		r.Description = *description
		fv["description"] = *description
	}

	if len(eligibleCountries) > 0 {
		fv["eligible_countries"] = strings.Join(eligibleCountries, ", ")
	}
	// Title fallback: some guides have an empty title column. Prefer the
	// contents slug as a last resort so the UI never renders a blank card.
	if r.Title == "" {
		if r.Slug != "" {
			r.Title = strings.ReplaceAll(r.Slug, "-", " ")
		} else {
			r.Title = "Untitled Scholarship"
		}
	}

	// Award amount: best-effort extraction from the funding jsonb
	// `award_amount_usd` field. Backfilled rows have empty funding.
	if amt, ok := awardAmountFromFunding(funding); ok {
		r.AwardAmountUSD = &amt
		fv["award_amount_usd"] = strconv.FormatFloat(amt, 'f', -1, 64)
	}

	// Renewable: best-effort extraction from the funding jsonb.
	if renewable, ok := renewableFromFunding(funding); ok {
		r.IsRenewable = &renewable
		fv["is_renewable"] = strconv.FormatBool(renewable)
	}

	// Merge any legacy/backfilled custom-field values stored on the
	// contents row (eligibility, how_to_apply, contact_email, etc.).
	mergeContentFieldValues(fv, fieldValues)

	r.FieldValues = fv
	projectScholarshipFields(r)
}

// awardAmountFromFunding extracts the award_amount_usd value from the funding
// jsonb blob, returning ok=false when the blob is empty or lacks the field.
func awardAmountFromFunding(funding []byte) (float64, bool) {
	if len(funding) == 0 || string(funding) == "{}" {
		return 0, false
	}

	var f map[string]interface{}
	if err := json.Unmarshal(funding, &f); err != nil {
		return 0, false
	}

	v, ok := f["award_amount_usd"]
	if !ok {
		return 0, false
	}

	return toFloat(v)
}

// toFloat coerces a json-decoded value to float64.
func toFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		if err != nil {
			return 0, false
		}

		return f, true
	}

	return 0, false
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

	return s.settings.GetBoolOr(ctx, pgtype.UUID{}, "scholarship_finder.enabled", true)
}

// Option is a single selectable value exposed by the statuses endpoint.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Statuses returns the canonical saved-scholarship status options.
func (s *Service) Statuses() []Option {
	return []Option{
		{Value: StatusSaved, Label: "Saved"},
		{Value: StatusResearching, Label: "Researching"},
		{Value: StatusApplied, Label: "Applied"},
		{Value: StatusAccepted, Label: "Accepted"},
		{Value: StatusRejected, Label: "Rejected"},
		{Value: StatusWithdrawn, Label: "Withdrawn"},
	}
}

// SetEnqueuer injects the Asynq task enqueuer.
func (s *Service) SetEnqueuer(e TaskEnqueuer) { s.enqueuer = e }

// findScholarshipsQuery builds the dynamic SQL and argument list for
// FindScholarships. It is extracted so the filter logic can be unit-tested
// without a live database. The `now` argument lets tests pin the
// "hide expired" cutoff to a fixed date.
func findScholarshipsQuery(tenantID pgtype.UUID, filters ScholarshipFilters, now time.Time) (string, []interface{}) {
	if filters.Limit <= 0 || filters.Limit > 100 {
		filters.Limit = 20
	}

	if filters.Page <= 0 {
		filters.Page = 1
	}
	// Clamp page to the largest value that keeps (page-1)*limit inside the int32 range.
	maxPage := math.MaxInt32 / filters.Limit
	if filters.Page > maxPage {
		filters.Page = maxPage
	}

	offset := (filters.Page - 1) * filters.Limit

	// Build dynamic WHERE clauses against the `scholarships` table.
	var conditions []string

	var args []interface{}

	argIdx := 1

	conditions = append(conditions, "c.status = 'published'")
	conditions = append(conditions, "c.deleted_at IS NULL")
	conditions = append(conditions, "c.guide_category = 'scholarship'")

	if filters.Country != "" {
		conditions = append(conditions, fmt.Sprintf("s.country ILIKE '%%' || $%d || '%%'", argIdx))
		args = append(args, filters.Country)
		argIdx++
	}

	if filters.Level != "" {
		conditions = append(conditions, fmt.Sprintf("s.degree_level ILIKE '%%' || $%d || '%%'", argIdx))
		args = append(args, filters.Level)
		argIdx++
	}

	if filters.DeadlineFrom != "" {
		conditions = append(conditions, fmt.Sprintf("s.deadline >= $%d::date", argIdx))
		args = append(args, filters.DeadlineFrom)
		argIdx++
	} else if !filters.IncludeExpired {
		// Default: hide expired records. Open-ended (NULL deadline) scholarships
		// remain visible so active opportunities without a fixed date surface.
		conditions = append(conditions, fmt.Sprintf("(s.deadline IS NULL OR s.deadline >= $%d::date)", argIdx))
		args = append(args, now.UTC().Format("2006-01-02"))
		argIdx++
	}

	if filters.DeadlineTo != "" {
		conditions = append(conditions, fmt.Sprintf("s.deadline <= $%d::date", argIdx))
		args = append(args, filters.DeadlineTo)
		argIdx++
	}

	if filters.Q != "" {
		conditions = append(conditions, fmt.Sprintf("c.title ILIKE '%%' || $%d || '%%'", argIdx))
		args = append(args, filters.Q)
		argIdx++
	}

	// Award amount and renewable filters look at both the scholarships.funding
	// JSONB and the legacy contents.field_values so backfilled guides still
	// match. A safe regex guard prevents non-numeric values from breaking the
	// cast.
	const numericRE = `'^[0-9]+(\.[0-9]+)?$'`
	amountValue := fmt.Sprintf("COALESCE(s.funding->>'award_amount_usd', c.field_values->>'award_amount_usd', '')")

	if filters.MinAmount > 0 {
		conditions = append(conditions, fmt.Sprintf("(%s ~ %s AND (%s)::numeric >= $%d)", amountValue, numericRE, amountValue, argIdx))
		args = append(args, filters.MinAmount)
		argIdx++
	}

	if filters.MaxAmount > 0 {
		conditions = append(conditions, fmt.Sprintf("(%s ~ %s AND (%s)::numeric <= $%d)", amountValue, numericRE, amountValue, argIdx))
		args = append(args, filters.MaxAmount)
		argIdx++
	}

	if filters.Renewable != nil {
		renewableValue := fmt.Sprintf("COALESCE(s.funding->>'is_renewable', c.field_values->>'is_renewable', '')")
		if *filters.Renewable {
			conditions = append(conditions, fmt.Sprintf("(LOWER(%s) = 'true' OR %s = '1')", renewableValue, renewableValue))
		} else {
			conditions = append(conditions, fmt.Sprintf("(LOWER(%s) = 'false' OR %s = '0')", renewableValue, renewableValue))
		}
	}

	// Sort order. `amount_desc` has no direct column on the scholarships
	// table (award amount lives in the `funding` jsonb, which is empty for
	// backfilled rows); fall back to created_at so the sort param is
	// accepted without erroring.
	var orderBy string

	switch filters.Sort {
	case "deadline_asc":
		orderBy = "s.deadline ASC NULLS LAST"
	case "amount_desc":
		// Award amount is not a column; try to order by the first non-null
		// numeric award_amount_usd from funding or field_values. Non-numeric
		// values fall to the bottom via NULLS LAST.
		orderBy = fmt.Sprintf("(CASE WHEN %s ~ %s THEN (%s)::numeric ELSE NULL END) DESC NULLS LAST", amountValue, numericRE, amountValue)
	default:
		orderBy = "s.created_at DESC"
	}

	whereClause := strings.Join(conditions, " AND ")

	fieldValueKeys := []string{
		"'eligibility_criteria'",
		"'how_to_apply'",
		"'contact_email'",
		"'award_amount_usd'",
		"'is_renewable'",
		"'provider_institution'",
		"'countries_eligible'",
		"'level_of_study'",
		"'application_deadline'",
		"'application_link'",
		"'description'",
	}
	fieldValuesSQL := fmt.Sprintf(`(
		SELECT COALESCE(jsonb_object_agg(cf.key, cfv.value_text), '{}'::jsonb) || COALESCE(c.field_values, '{}'::jsonb)
		FROM custom_field_values cfv
		JOIN custom_fields cf ON cf.id = cfv.field_id
		WHERE cfv.entity_id = c.id AND cfv.entity_type = 'content'
		  AND cf.key IN (%s)
	)`, strings.Join(fieldValueKeys, ", "))

	// Single query with COUNT(*) OVER() to get the total count and the
	// page rows in one pass. The window function appends the full-result
	// count to every row; we read it from the first row (identical across
	// rows in the same result set).
	dataQuery := fmt.Sprintf(`
		SELECT s.id, s.intel_record_id, s.guide_id, s.title, c.slug, c.meta_description,
		       s.country, s.country_space_id, s.degree_level, s.fields_of_study, s.deadline, s.application_url,
		       s.university, s.eligible_countries, s.funding, s.description, %s AS field_values,
		       COUNT(*) OVER() AS total_count
		FROM scholarships s
		JOIN contents c ON c.id = s.guide_id
		WHERE %s
		ORDER BY %s
		LIMIT $%d OFFSET $%d`,
		fieldValuesSQL, whereClause, orderBy, argIdx, argIdx+1)

	args = append(args, filters.Limit, offset)

	return dataQuery, args
}

// FindScholarships queries published scholarship guides with server-side
// filtering. Per ADR-112 the structured fields live in the dedicated
// `scholarships` table (country, degree_level, deadline, application_url,
// university, eligible_countries, funding); the editorial wrapper (slug,
// meta_description/summary) comes from the joined `contents` row. The
// previous custom_field_values projection is no longer the source of
// truth. Uses a direct pgx query for dynamic filtering.
func (s *Service) FindScholarships(ctx context.Context, tenantID pgtype.UUID, filters ScholarshipFilters) ([]ScholarshipResult, int64, error) {
	dataQuery, args := findScholarshipsQuery(tenantID, filters, time.Now())

	pool := s.store.pool.Reader

	rows, err := pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("scholarships: data query: %w", err)
	}

	defer rows.Close()

	var results []ScholarshipResult

	var total int64

	for rows.Next() {
		var (
			r                 ScholarshipResult
			id                pgtype.UUID
			intelRecordID     pgtype.UUID
			guideID           pgtype.UUID
			slug              *string
			summary           *string
			country           string
			countrySpaceID    pgtype.UUID
			degreeLevel       *string
			fieldsOfStudy     []string
			deadline          pgtype.Date
			applicationURL    *string
			university        *string
			eligibleCountries []string
			funding           []byte
			description       *string
			fieldValues       []byte
			rowCount          int64
		)

		if err := rows.Scan(
			&id, &intelRecordID, &guideID, &r.Title, &slug, &summary,
			&country, &countrySpaceID, &degreeLevel, &fieldsOfStudy, &deadline, &applicationURL,
			&university, &eligibleCountries, &funding, &description, &fieldValues,
			&rowCount,
		); err != nil {
			return nil, 0, fmt.Errorf("scholarships: scan row: %w", err)
		}

		if rowCount > 0 {
			total = rowCount
		}

		r.ID = pgtypeUUIDString(id)
		r.IntelRecordID = pgtypeUUIDString(intelRecordID)
		r.GuideID = pgtypeUUIDString(guideID)
		r.CountrySpaceID = pgtypeUUIDString(countrySpaceID)
		r.FieldsOfStudy = fieldsOfStudy

		if slug != nil {
			r.Slug = *slug
		}

		if summary != nil && *summary != "" {
			r.Summary = *summary
		} else if description != nil && *description != "" {
			r.Summary = *description
		}

		populateScholarshipResult(&r, country, degreeLevel, deadline, applicationURL, university, eligibleCountries, funding, description, fieldValues)
		results = append(results, r)
	}

	// Attach source attribution in a single batch query.
	recordIDs := make([]pgtype.UUID, 0, len(results))
	for _, r := range results {
		if r.IntelRecordID != "" {
			recordIDs = append(recordIDs, pgtypeUUID(r.IntelRecordID))
		}
	}

	if len(recordIDs) > 0 {
		sources, err := s.store.MapSourcesByRecordIDs(ctx, recordIDs)
		if err == nil {
			for i := range results {
				if src, ok := sources[results[i].IntelRecordID]; ok {
					results[i].Source = sourceAttributionFromRow(src)
				}
			}
		}
	}

	return results, total, nil
}

// GetScholarshipDetail returns a single scholarship guide with all field
// values. Per ADR-112 the structured fields are read from the `scholarships`
// table joined with `contents` (editorial wrapper). A guide that has no row
// in `scholarships` is not a finder-eligible scholarship and returns an error,
// which SaveScholarship maps to ErrGuideNotFound.
func (s *Service) GetScholarshipDetail(ctx context.Context, tenantID, guideID pgtype.UUID) (ScholarshipResult, error) {
	query := `
		SELECT s.id, s.intel_record_id, s.guide_id, s.title, c.slug, c.meta_description,
		       s.country, s.country_space_id, s.degree_level, s.fields_of_study, s.deadline, s.application_url,
		       s.university, s.eligible_countries, s.funding, s.description,
		       (
				   SELECT COALESCE(jsonb_object_agg(cf.key, cfv.value_text), '{}'::jsonb) || COALESCE(c.field_values, '{}'::jsonb)
				   FROM custom_field_values cfv
				   JOIN custom_fields cf ON cf.id = cfv.field_id
				   WHERE cfv.entity_id = c.id AND cfv.entity_type = 'content'
				     AND cf.key IN ('eligibility_criteria','how_to_apply','contact_email','award_amount_usd','is_renewable','provider_institution','countries_eligible','level_of_study','application_deadline','application_link','description')
		       ) AS field_values
		FROM scholarships s
		JOIN contents c ON c.id = s.guide_id
		WHERE s.guide_id = $1
		  AND c.status = 'published' AND c.deleted_at IS NULL
		  AND c.guide_category = 'scholarship'`

	var (
		r                 ScholarshipResult
		id                pgtype.UUID
		intelRecordID     pgtype.UUID
		gID               pgtype.UUID
		slug              *string
		summary           *string
		country           string
		countrySpaceID    pgtype.UUID
		degreeLevel       *string
		fieldsOfStudy     []string
		deadline          pgtype.Date
		applicationURL    *string
		university        *string
		eligibleCountries []string
		funding           []byte
		description       *string
		fieldValues       []byte
	)

	if err := s.store.pool.Reader.QueryRow(ctx, query, tenantID, guideID).Scan(
		&id, &intelRecordID, &gID, &r.Title, &slug, &summary,
		&country, &countrySpaceID, &degreeLevel, &fieldsOfStudy, &deadline, &applicationURL,
		&university, &eligibleCountries, &funding, &description, &fieldValues,
	); err != nil {
		return ScholarshipResult{}, fmt.Errorf("scholarships: get detail: %w", err)
	}

	r.ID = pgtypeUUIDString(id)
	r.IntelRecordID = pgtypeUUIDString(intelRecordID)
	r.GuideID = pgtypeUUIDString(gID)
	r.CountrySpaceID = pgtypeUUIDString(countrySpaceID)
	r.FieldsOfStudy = fieldsOfStudy

	if slug != nil {
		r.Slug = *slug
	}

	if summary != nil && *summary != "" {
		r.Summary = *summary
	} else if description != nil && *description != "" {
		r.Summary = *description
	}

	populateScholarshipResult(&r, country, degreeLevel, deadline, applicationURL, university, eligibleCountries, funding, description, fieldValues)

	if intelRecordID.Valid {
		if src, err := s.store.GetSourceByRecordID(ctx, intelRecordID); err == nil {
			r.Source = sourceAttributionFromRow(src)
		}
	}

	return r, nil
}

func sourceAttributionFromRow(src gen.ListSourcesByRecordIDsRow) SourceAttribution {
	a := SourceAttribution{
		ID:         pgtypeUUIDString(src.SourceID),
		Name:       src.Name,
		SourceURL:  src.SourceUrl,
		TrustLevel: src.TrustLevel,
	}
	if src.Slug != nil {
		a.Slug = *src.Slug
	}
	if src.LogoUrl != nil {
		a.LogoURL = *src.LogoUrl
	}
	return a
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

	defaultReminderDays := safeInt32(s.settings.GetIntOr(ctx, pgtype.UUID{}, "scholarship_finder.default_reminder_days", 14))

	saved, err := s.store.SaveScholarship(ctx, gen.SaveScholarshipParams{

		UserID:           userID,
		GuideID:          guideID,
		RemindBeforeDays: defaultReminderDays,
	})
	if err != nil {
		// ON CONFLICT (, user_id, guide_id) DO NOTHING returns no
		// row when the tracker entry already exists; surface the existing
		// row so a repeat save is idempotent rather than a 400.
		if errors.Is(err, pgx.ErrNoRows) {
			existing, gerr := s.store.GetSavedScholarship(ctx, gen.GetSavedScholarshipParams{

				UserID:  userID,
				GuideID: guideID,
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
		completionPoints := s.settings.GetIntOr(ctx, pgtype.UUID{}, "scholarship_finder.completion_points", 10)
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

		UserID:  userID,
		GuideID: guideID,
	})
}

// IsSavedScholarship checks whether the user has saved the given guide.
// Returns false (not an error) when no saved-tracker row exists.
func (s *Service) IsSavedScholarship(ctx context.Context, tenantID, userID, guideID pgtype.UUID) (bool, error) {
	_, err := s.store.GetSavedScholarship(ctx, gen.GetSavedScholarshipParams{

		UserID:  userID,
		GuideID: guideID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}

		return false, err
	}

	return true, nil
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

	offset := safeOffset(page, limit)

	rows, err := s.store.ListSavedScholarships(ctx, gen.ListSavedScholarshipsParams{

		UserID:     userID,
		Status:     status,
		PageLimit:  safeInt32(limit),
		PageOffset: offset,
	})
	if err != nil {
		return nil, 0, err
	}

	total, err := s.store.CountSavedScholarships(ctx, gen.CountSavedScholarshipsParams{

		UserID: userID,
		Status: status,
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

		PageLimit:  1000,
		PageOffset: 0,
	})
	if err != nil {
		return AdminStats{}, err
	}

	upcoming := int64(len(dl))

	rl, err := s.store.AdminReminderLog(ctx, gen.AdminScholarshipReminderLogParams{

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

	offset := safeOffset(page, limit)

	rows, err := s.store.AdminUpcomingDeadlines(ctx, gen.AdminScholarshipUpcomingDeadlinesParams{

		PageLimit:  safeInt32(limit),
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

// pgtypeUUID converts a UUID string to a pgtype.UUID. Invalid input becomes
// an invalid (NULL) pgtype.UUID.
func pgtypeUUID(s string) pgtype.UUID {
	if s == "" {
		return pgtype.UUID{}
	}

	id, err := uuid.Parse(s)
	if err != nil {
		return pgtype.UUID{}
	}

	return pgtype.UUID{Bytes: [16]byte(id), Valid: true}
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

	offset := safeOffset(page, limit)

	return s.store.AdminTopSaved(ctx, gen.AdminScholarshipTopSavedParams{

		PageLimit:  safeInt32(limit),
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

	offset := safeOffset(page, limit)

	return s.store.AdminUpcomingDeadlines(ctx, gen.AdminScholarshipUpcomingDeadlinesParams{

		PageLimit:  safeInt32(limit),
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

	offset := safeOffset(page, limit)

	return s.store.AdminReminderLog(ctx, gen.AdminScholarshipReminderLogParams{

		PageLimit:  safeInt32(limit),
		PageOffset: offset,
	})
}

// FindNeedingReminders returns saved scholarships needing deadline reminders.
// Called by the scholarship:check_deadlines worker task.
// // The store query returns every saved scholarship with a non-null deadline
// and no recent reminder; this method additionally enforces the
// remind_before_days window so reminders are only dispatched for deadlines
// that are still ahead and within the user's configured lead time. Without
// this filter the daily sweep would notify users about far-future deadlines
// (and re-notify every day), which is notification spam (ADR-073 §3).
func (s *Service) FindNeedingReminders(ctx context.Context, batchLimit int32) ([]gen.FindScholarshipsNeedingRemindersRow, error) {
	rows, err := s.store.FindNeedingReminders(ctx, batchLimit)
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

// safeInt32 converts an int to int32, clamping to the int32 range. It
// prevents G115 integer-overflow conversions from untrusted input.
func safeInt32(v int) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}

	if v < math.MinInt32 {
		return math.MinInt32
	}

	return int32(v)
}

// safeOffset computes (page-1)*limit and returns it as an int32, clamping
// page to the largest value that keeps the offset inside the int32 range.
// limit is expected to be positive and already bounded (e.g. <= 100);
// if it is not, it falls back to 20 before the calculation.
func safeOffset(page, limit int) int32 {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	maxPage := math.MaxInt32 / limit
	if page > maxPage {
		page = maxPage
	}
	// #nosec G115 -- page and limit are clamped so (page-1)*limit <= math.MaxInt32.
	return int32((page - 1) * limit)
}

// MarkReminderSent updates last_reminded_at for a saved scholarship.
func (s *Service) MarkReminderSent(ctx context.Context, id pgtype.UUID) error {
	return s.store.MarkReminderSent(ctx, id)
}
