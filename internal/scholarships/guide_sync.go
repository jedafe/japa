package scholarships

// Ref: japaconnect-engineering/sections/73-scholarship-finder.md
//
// Reverse Scholarship → Guide flow: when a user creates or updates a guide with
// content_type='guide' and guide_category='scholarship', the platform derives a
// matching scholarships projection row from the guide's custom field values and
// contents.field_values JSONB. This keeps the finder automatically in sync with
// manually authored scholarship guides without requiring the intelligence spine.

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

	"github.com/jedafe/japa/internal/db/gen"
)

// knownScholarshipFieldKeys is the set of field keys the sync reads from a
// scholarship guide (§14 scholarship category + common custom-field aliases).
var knownScholarshipFieldKeys = map[string]bool{
	"scholarship_name":      true,
	"provider_institution":  true,
	"institution_name":      true,
	"university":            true,
	"eligibility_criteria":  true,
	"eligibility":           true,
	"award_amount_usd":      true,
	"award_amount":          true,
	"grant_amount_usd":      true,
	"application_deadline":  true,
	"application_link":      true,
	"application_url":       true,
	"level_of_study":        true,
	"degree_level":          true,
	"is_renewable":          true,
	"renewable":             true,
	"coverage":              true,
	"countries_eligible":    true,
	"eligible_countries":    true,
	"country":               true,
	"field_of_study":        true,
	"fields_of_study":       true,
	"required_documents":    true,
	"minimum_gpa":           true,
	"language_requirements": true,
	"how_to_apply":          true,
	"contact_email":         true,
	"description":           true,
}

// fieldMap is the merged guide field values (custom_field_values + field_values).
// Keys are canonical; values are plain strings so they can be mapped directly
// into the structured scholarships columns.
type fieldMap map[string]string

// SyncFromContentGuide derives a scholarships projection row from a published
// scholarship guide. It is idempotent by guide_id: it creates a row when one
// does not exist and updates the existing row otherwise.
func (s *Store) SyncFromContentGuide(ctx context.Context, contentID pgtype.UUID) error {
	content, err := s.queries.GetContentByID(ctx,
		contentID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// The guide was deleted before the sync task ran; remove any
			// projection row that may still reference it.
			return s.deleteProjectionIfExists(ctx, contentID)
		}
		return fmt.Errorf("scholarships: sync from guide: get content: %w", err)
	}

	if !isSyncEligible(content) {
		existing, err := s.queries.GetScholarshipByGuide(ctx,

			content.ID,
		)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("scholarships: sync from guide: check existing: %w", err)
		}
		if existing.GuideID.Valid {
			if err := s.queries.DeleteScholarship(ctx,

				existing.ID,
			); err != nil {
				return fmt.Errorf("scholarships: sync from guide: delete stale projection: %w", err)
			}
			slog.Info("scholarships: deleted stale projection", "content_id", content.ID.String())
		}
		return nil
	}

	fields, err := s.loadGuideFields(ctx, content.ID, content.FieldValues)
	if err != nil {
		return fmt.Errorf("scholarships: sync from guide: load fields: %w", err)
	}

	existing, err := s.queries.GetScholarshipByGuide(ctx,

		content.ID,
	)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("scholarships: sync from guide: get existing: %w", err)
	}

	params, err := s.buildScholarshipFromGuide(ctx, content, fields)
	if err != nil {
		return fmt.Errorf("scholarships: sync from guide: build params: %w", err)
	}

	if existing.GuideID.Valid {
		_, err = s.UpdateScholarship(ctx, existing.ID, buildUpdateInput(params, existing))
		if err != nil {
			return fmt.Errorf("scholarships: sync from guide: update: %w", err)
		}
		slog.Info("scholarships: synced guide to existing scholarship", "content_id", content.ID.String(), "scholarship_id", existing.ID.String())
		return nil
	}

	create := buildCreateParams(params)

	_, err = s.AdminCreateScholarship(ctx, create)
	if err != nil {
		return fmt.Errorf("scholarships: sync from guide: create: %w", err)
	}
	slog.Info("scholarships: created scholarship from guide", "content_id", content.ID.String())
	return nil
}

// deleteProjectionIfExists removes the scholarships row for a guide that no
// longer exists. It is safe to call when the content row was deleted before the
// sync task processed.
func (s *Store) deleteProjectionIfExists(ctx context.Context, guideID pgtype.UUID) error {
	existing, err := s.queries.GetScholarshipByGuide(ctx,

		guideID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("scholarships: sync from guide: check deleted guide: %w", err)
	}

	if !existing.GuideID.Valid {
		return nil
	}

	if err := s.queries.DeleteScholarship(ctx,

		existing.ID,
	); err != nil {
		return fmt.Errorf("scholarships: sync from guide: delete projection for deleted guide: %w", err)
	}
	slog.Info("scholarships: deleted projection for deleted guide", "content_id", guideID.String())
	return nil
}

func isScholarshipGuideCategory(category *string) bool {
	if category == nil {
		return false
	}
	return *category == "scholarship"
}

// isSyncEligible returns true when the content row is a published scholarship
// guide that should have a finder projection.
func isSyncEligible(content gen.GetContentByIDRow) bool {
	return content.ContentType == "guide" &&
		isScholarshipGuideCategory(content.GuideCategory) &&
		content.Status == "published"
}

// loadGuideFields merges custom_field_values with contents.field_values JSONB.
// contents.field_values takes precedence (right-hand side of the merge) to match
// the precedence used by FindScholarshipsQuery.
func (s *Store) loadGuideFields(ctx context.Context, contentID pgtype.UUID, fieldValues []byte) (fieldMap, error) {
	fields := make(fieldMap)

	customRows, err := s.queries.ListCustomFieldValuesWithDefsByEntity(ctx, gen.ListCustomFieldValuesWithDefsByEntityParams{
		EntityType: "content",
		EntityID:   contentID,
	})
	if err != nil {
		return nil, err
	}

	for _, row := range customRows {
		if row.FieldKey == "" {
			continue
		}
		if v := customFieldValueToString(row); v != "" {
			fields[row.FieldKey] = v
		}
	}

	mergeContentFieldValues(fields, fieldValues)
	return fields, nil
}

// customFieldValueToString returns the best string representation of a custom
// field value. For multi-select / JSON arrays, elements are joined with ", ".
func customFieldValueToString(row gen.ListCustomFieldValuesWithDefsByEntityRow) string {
	if row.ValueText != nil {
		s := strings.TrimSpace(*row.ValueText)
		if s != "" {
			return s
		}
	}

	if row.ValueNumber.Valid {
		if f, err := row.ValueNumber.Float64Value(); err == nil && f.Valid {
			return strconv.FormatFloat(f.Float64, 'f', -1, 64)
		}
	}

	if row.ValueBool != nil {
		return strconv.FormatBool(*row.ValueBool)
	}

	if row.ValueDate.Valid {
		return row.ValueDate.Time.Format("2006-01-02")
	}

	if len(row.ValueJson) > 0 && string(row.ValueJson) != "null" && string(row.ValueJson) != "{}" {
		var arr []interface{}
		if err := json.Unmarshal(row.ValueJson, &arr); err == nil {
			return joinInterfaceArray(arr, ", ")
		}

		// Single JSON scalar value.
		var raw interface{}
		if err := json.Unmarshal(row.ValueJson, &raw); err == nil {
			return valueToString(raw)
		}
	}

	return ""
}

// joinInterfaceArray joins non-empty values in an interface slice.
func joinInterfaceArray(arr []interface{}, sep string) string {
	parts := make([]string, 0, len(arr))
	for _, v := range arr {
		if s := valueToString(v); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, sep)
}

func valueToString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case bool:
		return strconv.FormatBool(t)
	case map[string]interface{}:
		if s, ok := t["value"].(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// guideScholarshipParams is the intermediate, normalized representation used to
// build either a CreateScholarshipParams or an UpdateScholarshipInput.
type guideScholarshipParams struct {
	GuideID              pgtype.UUID
	Title                string
	Description          *string
	SourceURL            string
	SourceType           string
	TrustLevel           int16
	Country              string
	CountrySpaceID       pgtype.UUID
	University           *string
	DegreeLevel          *string
	FieldsOfStudy        []string
	EligibleCountries    []string
	Funding              []byte
	Deadline             pgtype.Date
	ApplicationURL       *string
	RequiredDocuments    []string
	MinimumGpa           pgtype.Numeric
	LanguageRequirements []byte
	ConfidenceOverall    float32
	VerificationStatus   string
	FreshnessScore       int32
	OverrideSource       string
	EditorialOverrides   []byte
}

// buildScholarshipFromGuide converts a content row and its merged field map
// into a guideScholarshipParams. It never returns an error for missing data;
// missing or invalid fields are best-effort skipped.
func (s *Store) buildScholarshipFromGuide(ctx context.Context, content gen.GetContentByIDRow, fields fieldMap) (guideScholarshipParams, error) {
	var p guideScholarshipParams

	p.GuideID = content.ID
	p.Title = strings.TrimSpace(content.Title)
	if p.Title == "" {
		if name := fields["scholarship_name"]; name != "" {
			p.Title = name
		} else {
			p.Title = strings.ReplaceAll(content.Slug, "-", " ")
		}
	}

	if desc := fields["description"]; desc != "" {
		p.Description = &desc
	} else if body := strings.TrimSpace(content.Body); body != "" {
		trunc := body
		if len(trunc) > 500 {
			trunc = trunc[:500]
		}
		p.Description = &trunc
	}

	if content.CanonicalUrl != nil && *content.CanonicalUrl != "" {
		p.SourceURL = *content.CanonicalUrl
	} else {
		p.SourceURL = "/guide/" + content.Slug
	}
	p.SourceType = "community"
	p.TrustLevel = 4

	p.Country = strings.TrimSpace(fields["country"])
	if p.Country == "" {
		p.Country = firstFromList(fields["countries_eligible"])
	}

	countrySpaceID, err := s.resolveCountrySpaceID(ctx, p.Country)
	if err == nil {
		p.CountrySpaceID = countrySpaceID
	}

	uni := fields["provider_institution"]
	if uni == "" {
		uni = fields["institution_name"]
	}
	if uni == "" {
		uni = fields["university"]
	}
	if uni = strings.TrimSpace(uni); uni != "" {
		p.University = &uni
	}

	lvl := strings.TrimSpace(fields["level_of_study"])
	if lvl == "" {
		lvl = strings.TrimSpace(fields["degree_level"])
	}
	if lvl != "" {
		p.DegreeLevel = &lvl
	}

	p.FieldsOfStudy = splitList(fields["field_of_study"])
	if len(p.FieldsOfStudy) == 0 {
		p.FieldsOfStudy = splitList(fields["fields_of_study"])
	}

	p.EligibleCountries = splitList(fields["countries_eligible"])
	if len(p.EligibleCountries) == 0 {
		p.EligibleCountries = splitList(fields["eligible_countries"])
	}

	p.Funding = buildFundingJSON(fields)

	p.Deadline = parseDate(fields["application_deadline"])

	appURL := strings.TrimSpace(fields["application_link"])
	if appURL == "" {
		appURL = strings.TrimSpace(fields["application_url"])
	}
	if appURL != "" {
		if _, ok := sanitizeScholarshipURL(appURL); !ok {
			slog.Warn("scholarships: guide_sync quarantined invalid application_url", "value", appURL)
		} else {
			p.ApplicationURL = &appURL
		}
	}

	p.RequiredDocuments = splitList(fields["required_documents"])
	p.MinimumGpa = parseNumeric(fields["minimum_gpa"])
	p.LanguageRequirements = buildLanguageRequirementsJSON(fields["language_requirements"])

	p.ConfidenceOverall = 0.0
	p.VerificationStatus = "verified"
	p.FreshnessScore = 70
	p.OverrideSource = "editorial"
	p.EditorialOverrides = buildEditorialOverrides(fields)

	return p, nil
}

// resolveCountrySpaceID resolves a country-level space by geo_code. The user
// may supply either a 3-letter ISO code or a country name; best-effort only.
func (s *Store) resolveCountrySpaceID(ctx context.Context, countryCode string) (pgtype.UUID, error) {
	if countryCode == "" {
		return pgtype.UUID{}, nil
	}

	code := strings.ToUpper(strings.TrimSpace(countryCode))
	if len(code) != 3 {
		return pgtype.UUID{}, nil
	}

	row, err := s.queries.GetSpaceByGeoCode(ctx,
		&code,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, nil
		}
		return pgtype.UUID{}, err
	}

	return row.ID, nil
}

// firstFromList returns the first non-empty element in a comma-separated list.
func firstFromList(s string) string {
	for _, part := range splitList(s) {
		if part != "" {
			return part
		}
	}
	return ""
}

// splitList splits a comma-separated string and trims each element.
func splitList(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// buildFundingJSON assembles the funding JSONB from award, renewable, and
// coverage custom fields. It returns a non-nil byte slice (at least `{}`).
// Non-numeric award values are quarantined: logged and dropped from the
// structured blob (kept in editorial_overrides) instead of persisted.
func buildFundingJSON(fields fieldMap) []byte {
	m := map[string]interface{}{}

	quarantineAward := func(raw string) {
		slog.Warn("scholarships: guide_sync quarantined invalid award_amount", "value", raw)
	}

	if v := fields["award_amount_usd"]; v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			m["award_amount_usd"] = f
		} else {
			quarantineAward(v)
		}
	} else if v := fields["award_amount"]; v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			m["award_amount_usd"] = f
		} else {
			quarantineAward(v)
		}
	} else if v := fields["grant_amount_usd"]; v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			m["award_amount_usd"] = f
		} else {
			quarantineAward(v)
		}
	}

	if v := fields["is_renewable"]; v != "" {
		m["is_renewable"] = parseBool(v)
	} else if v := fields["renewable"]; v != "" {
		m["is_renewable"] = parseBool(v)
	}

	if v := fields["coverage"]; v != "" {
		m["coverage"] = v
	}

	b, err := json.Marshal(m)
	if err != nil || len(b) == 0 {
		return []byte("{}")
	}
	return b
}

// buildLanguageRequirementsJSON stores language requirements as JSON. If the
// value is already valid JSON it is used as-is; otherwise it is wrapped.
func buildLanguageRequirementsJSON(v string) []byte {
	v = strings.TrimSpace(v)
	if v == "" {
		return []byte("{}")
	}

	if (strings.HasPrefix(v, "{") && strings.HasSuffix(v, "}")) || (strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]")) {
		if json.Valid([]byte(v)) {
			return []byte(v)
		}
	}

	b, err := json.Marshal(map[string]string{"requirements": v})
	if err != nil {
		return []byte("{}")
	}
	return b
}

// buildEditorialOverrides marks every field key we actually derived from the
// guide as an editorial override, so future intelligence promoter re-crawls
// will not clobber user-authored values.
func buildEditorialOverrides(fields fieldMap) []byte {
	overrides := map[string]bool{}
	for key, value := range fields {
		if value != "" && knownScholarshipFieldKeys[key] {
			overrides[key] = true
		}
	}

	b, err := json.Marshal(overrides)
	if err != nil || len(b) == 0 {
		return []byte("{}")
	}
	return b
}

func parseDate(v string) pgtype.Date {
	var d pgtype.Date
	v = strings.TrimSpace(v)
	if v == "" {
		return d
	}

	for _, layout := range []string{"2006-01-02", time.RFC3339, "2006-01-02T15:04:05Z"} {
		if t, err := time.Parse(layout, v); err == nil {
			d = pgtype.Date{Time: t, Valid: true}
			return d
		}
	}
	slog.Warn("scholarships: guide_sync quarantined invalid application_deadline", "value", v)
	return d
}

func parseNumeric(v string) pgtype.Numeric {
	v = strings.TrimSpace(v)
	if v == "" {
		return pgtype.Numeric{}
	}

	if f, err := strconv.ParseFloat(v, 64); err == nil {
		return float64ToNumeric(f)
	}
	return pgtype.Numeric{}
}

func parseBool(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	return v == "true" || v == "1" || v == "yes"
}

// buildCreateParams converts the normalized guide params into the sqlc
// CreateScholarshipParams shape.
func buildCreateParams(p guideScholarshipParams) gen.CreateScholarshipParams {
	country := p.Country
	if country == "" {
		country = "International"
	}

	return gen.CreateScholarshipParams{
		IntelRecordID:        pgtype.UUID{},
		GuideID:              p.GuideID,
		SourceUrl:            p.SourceURL,
		SourceType:           p.SourceType,
		TrustLevel:           p.TrustLevel,
		Title:                p.Title,
		Description:          p.Description,
		Country:              country,
		CountrySpaceID:       p.CountrySpaceID,
		University:           p.University,
		DegreeLevel:          p.DegreeLevel,
		FieldsOfStudy:        emptyIfNil(p.FieldsOfStudy),
		EligibleCountries:    emptyIfNil(p.EligibleCountries),
		Funding:              p.Funding,
		Deadline:             p.Deadline,
		ApplicationUrl:       p.ApplicationURL,
		RequiredDocuments:    emptyIfNil(p.RequiredDocuments),
		MinimumGpa:           p.MinimumGpa,
		LanguageRequirements: p.LanguageRequirements,
		ConfidenceOverall:    p.ConfidenceOverall,
		VerificationStatus:   p.VerificationStatus,
		FreshnessScore:       p.FreshnessScore,
		OverrideSource:       p.OverrideSource,
		EditorialOverrides:   p.EditorialOverrides,
	}
}

func emptyIfNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// buildUpdateInput merges the newly derived guide params over an existing row.
// Fields with editorial overrides are preserved when the existing row already
// marked them as user-locked, unless the guide itself is the source of truth.
//
// For simplicity, the guide sync always treats the guide as the editorial
// source, so it overwrites the existing scholarship row. The promoter already
// respects editorial_overrides on the ingestion update path.
func buildUpdateInput(p guideScholarshipParams, existing gen.Scholarship) UpdateScholarshipInput {
	desc := p.Description
	if desc == nil && existing.Description != nil && *existing.Description != "" {
		d := *existing.Description
		desc = &d
	}

	country := p.Country
	if country == "" && existing.Country != "" {
		country = existing.Country
	}

	// Pointer helpers for optional columns.
	strPtr := func(s string) *string {
		if s == "" {
			return nil
		}
		return &s
	}

	var minimumGpa *float64
	if f, err := p.MinimumGpa.Float64Value(); err == nil && f.Valid {
		minimumGpa = &f.Float64
	}

	return UpdateScholarshipInput{
		ID:                   existing.ID,
		IntelRecordID:        existing.IntelRecordID,
		GuideID:              p.GuideID,
		SourceURL:            &p.SourceURL,
		SourceType:           &p.SourceType,
		TrustLevel:           &p.TrustLevel,
		Title:                &p.Title,
		Description:          desc,
		Country:              strPtr(country),
		CountrySpaceID:       p.CountrySpaceID,
		University:           p.University,
		DegreeLevel:          p.DegreeLevel,
		FieldsOfStudy:        p.FieldsOfStudy,
		EligibleCountries:    p.EligibleCountries,
		Funding:              p.Funding,
		Deadline:             p.Deadline,
		ApplicationURL:       p.ApplicationURL,
		RequiredDocuments:    p.RequiredDocuments,
		MinimumGpa:           minimumGpa,
		LanguageRequirements: p.LanguageRequirements,
		ConfidenceOverall:    &p.ConfidenceOverall,
		VerificationStatus:   &p.VerificationStatus,
		FreshnessScore:       &p.FreshnessScore,
		OverrideSource:       &p.OverrideSource,
		EditorialOverrides:   p.EditorialOverrides,
	}
}
