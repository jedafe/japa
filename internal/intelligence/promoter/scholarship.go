package promoter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/extractor"
)

// Store is the persistence surface the ScholarshipPromoter needs.
// *stores.Store satisfies this interface; tests substitute a mock.
type Store interface {
	GetIntelRecord(ctx context.Context, id pgtype.UUID) (gen.IntelRecord, error)
	GetIntelSource(ctx context.Context, id pgtype.UUID) (gen.IntelSource, error)
	GetScholarshipByIntelRecord(ctx context.Context, intelRecordID pgtype.UUID) (gen.Scholarship, error)
	CreateScholarship(ctx context.Context, arg gen.CreateScholarshipParams) (gen.Scholarship, error)
	UpdateScholarship(ctx context.Context, arg gen.UpdateScholarshipParams) error
	CreateContent(ctx context.Context, arg gen.CreateContentParams) (gen.CreateContentRow, error)
	UpdateIntelRecordPromoted(ctx context.Context, arg gen.UpdateIntelRecordPromotedParams) error
	CreateIntelReviewLog(ctx context.Context, arg gen.CreateIntelReviewLogParams) error
}

// Default freshness score for a freshly promoted / re-verified row. The
// freshness worker (Task 21) decays this over time; the promoter resets it to
// full whenever a re-crawl confirms or updates the projection.
const freshScore int32 = 100

// ScholarshipPromoter promotes scholarship intel_records into the
// scholarships projection table plus the contents guide entry that backs it.
//
// It implements the editorial-override-respect invariant: on the update path,
// any field present in scholarships.editorial_overrides is left untouched so a
// human edit always wins over a re-crawl.
type ScholarshipPromoter struct {
	store    Store
	tenantID pgtype.UUID // tenant the generated guide rows are stamped with
	authorID pgtype.UUID // system/automation user credited as the guide author
	now      func() time.Time
}

// ScholarshipPromoterOption configures a ScholarshipPromoter.
type ScholarshipPromoterOption func(*ScholarshipPromoter)

// WithClock overrides the clock used for publish timestamps (tests).
func WithClock(fn func() time.Time) ScholarshipPromoterOption {
	return func(p *ScholarshipPromoter) { p.now = fn }
}

// NewScholarshipPromoter creates a promoter backed by the given store. The
// tenantID and authorID identify the tenant and system author stamped onto the
// contents guide rows created by the create path.
func NewScholarshipPromoter(store Store, tenantID, authorID pgtype.UUID, opts ...ScholarshipPromoterOption) *ScholarshipPromoter {
	p := &ScholarshipPromoter{
		store:    store,
		tenantID: tenantID,
		authorID: authorID,
		now:      time.Now,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Entity returns the entity_type this promoter handles.
func (p *ScholarshipPromoter) Entity() string { return "scholarship" }

// ErrNotScholarship is returned when Promote is called on an intel record whose
// entity_type is not "scholarship".
var ErrNotScholarship = errors.New("promoter: intel record is not a scholarship")

// Promote maps the intel record identified by recordID to a scholarships
// projection row, creating it (plus its backing guide entry) or updating it
// in place while respecting editorial overrides.
func (p *ScholarshipPromoter) Promote(ctx context.Context, recordID uuid.UUID) (PromoteResult, error) {
	pgID := uuidToPg(recordID)

	record, err := p.store.GetIntelRecord(ctx, pgID)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: load intel record: %w", err)
	}
	if record.EntityType != p.Entity() {
		return PromoteResult{}, ErrNotScholarship
	}

	ext, err := parseExtraction(record.Extracted)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: parse extracted: %w", err)
	}

	source, err := p.store.GetIntelSource(ctx, record.SourceID)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: load intel source: %w", err)
	}

	// Does a projection row already exist for this intel record? A not-found
	// error means we take the create path; any other error propagates.
	existing, err := p.store.GetScholarshipByIntelRecord(ctx, pgID)
	if err != nil {
		if isNotFound(err) {
			return p.createPath(ctx, record, source, ext)
		}
		return PromoteResult{}, fmt.Errorf("promoter: load existing scholarship: %w", err)
	}

	return p.updatePath(ctx, record, existing, ext)
}

// createPath inserts the contents guide entry, the scholarships projection row,
// and flips the intel record to 'published' with the promoted row id.
func (p *ScholarshipPromoter) createPath(ctx context.Context, record gen.IntelRecord, source gen.IntelSource, ext extractor.Extraction) (PromoteResult, error) {
	title := stringField(ext, "title")
	if title == "" {
		title = source.Name // fall back to the source name rather than failing
	}
	body := bodyFromExtraction(ext, title)
	slug := content.Slugify(title)
	hash := content.ComputeContentHash(body)
	canonicalURL := source.SourceUrl

	contentRow, err := p.store.CreateContent(ctx, gen.CreateContentParams{
		TenantID:        p.tenantID,
		AuthorID:        p.authorID,
		ContentType:     "guide",
		Title:           title,
		Slug:            slug,
		Body:            body,
		BodyHtml:        bodyHTML(body),
		ContentHash:     &hash,
		CanonicalUrl:    &canonicalURL,
		Status:          "published",
		Visibility:      "public",
		SeoIndexable:    true,
		ReadTimeMinutes: content.ComputeReadTime(body),
		GuideCategory:   ptr("scholarship"),
		PublishAt:       pgtype.Timestamptz{Time: p.now(), Valid: true},
		PublishedAt:     pgtype.Timestamptz{Time: p.now(), Valid: true},
	})
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: create guide content: %w", err)
	}

	scholarship, err := p.store.CreateScholarship(ctx, buildCreateParams(record, source, contentRow.ID, ext))
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: create scholarship: %w", err)
	}

	if err := p.store.UpdateIntelRecordPromoted(ctx, gen.UpdateIntelRecordPromotedParams{
		ID:               record.ID,
		PromotedRecordID: scholarship.ID,
	}); err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: mark intel record promoted: %w", err)
	}

	return PromoteResult{
		DomainID:  pgToUUID(scholarship.ID),
		ContentID: pgToUUID(contentRow.ID),
		Action:    ActionCreated,
	}, nil
}

// updatePath applies the new extraction to the existing scholarship row,
// skipping any field present in editorial_overrides, and records an
// auto_update review-log entry when fields actually change.
func (p *ScholarshipPromoter) updatePath(ctx context.Context, record gen.IntelRecord, existing gen.Scholarship, ext extractor.Extraction) (PromoteResult, error) {
	overrides, err := parseOverrideKeys(existing.EditorialOverrides)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: parse editorial overrides: %w", err)
	}

	params, changed := buildUpdateParams(existing, ext, overrides)

	if len(changed) == 0 {
		// No field-level changes: skip the auto_update log, just bump
		// last_verified_at + freshness_score by re-stamping the row with
		// its current values at full freshness.
		params.FreshnessScore = freshScore
		if err := p.store.UpdateScholarship(ctx, params); err != nil {
			return PromoteResult{}, fmt.Errorf("promoter: bump scholarship freshness: %w", err)
		}
		return PromoteResult{
			DomainID:  pgToUUID(existing.ID),
			ContentID: pgToUUID(existing.GuideID),
			Action:    ActionVerified,
		}, nil
	}

	params.FreshnessScore = freshScore
	if err := p.store.UpdateScholarship(ctx, params); err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: update scholarship: %w", err)
	}

	// Record the auto_update in the review log so the human-in-the-loop UI can
	// show what the automation changed. Field changes are JSON-encoded as
	// [{"field": "...", "from": ..., "to": ...}, ...].
	changesJSON, err := marshalFieldChanges(existing, params, changed)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: marshal field changes: %w", err)
	}
	note := "automatic re-crawl update"
	if err := p.store.CreateIntelReviewLog(ctx, gen.CreateIntelReviewLogParams{
		IntelRecordID: record.ID,
		Action:        "auto_update",
		FieldChanges:  changesJSON,
		Note:          &note,
	}); err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: write auto_update review log: %w", err)
	}

	return PromoteResult{
		DomainID:     pgToUUID(existing.ID),
		ContentID:    pgToUUID(existing.GuideID),
		Action:       ActionUpdated,
		ChangedFields: changed,
	}, nil
}

// ---------------------------------------------------------------------------
// Extraction -> scholarship params
// ---------------------------------------------------------------------------

// buildCreateParams maps a validated extraction to a CreateScholarshipParams.
func buildCreateParams(record gen.IntelRecord, source gen.IntelSource, guideID pgtype.UUID, ext extractor.Extraction) gen.CreateScholarshipParams {
	return gen.CreateScholarshipParams{
		IntelRecordID:        record.ID,
		GuideID:              guideID,
		SourceUrl:            source.SourceUrl,
		SourceType:           source.SourceType,
		TrustLevel:           source.TrustLevel,
		Title:                stringField(ext, "title"),
		Description:          stringPtrField(ext, "description_summary"),
		Country:              firstCountry(ext, "country"),
		CountrySpaceID:       pgtype.UUID{}, // resolved by the geo worker, not the promoter
		University:           stringPtrField(ext, "university"),
		DegreeLevel:          stringPtrField(ext, "degree_level"),
		FieldsOfStudy:        stringSliceField(ext, "fields_of_study"),
		EligibleCountries:    stringSliceField(ext, "eligible_countries"),
		Funding:              jsonbField(ext, "funding"),
		Deadline:             dateField(ext, "deadline"),
		ApplicationUrl:       stringPtrField(ext, "application_url"),
		RequiredDocuments:    stringSliceField(ext, "required_documents"),
		MinimumGpa:           numericField(ext, "minimum_gpa"),
		LanguageRequirements: jsonbField(ext, "language_requirements"),
		ConfidenceOverall:    record.ConfidenceOverall,
		VerificationStatus:   "auto",
		FreshnessScore:       freshScore,
		OverrideSource:       "auto",
		EditorialOverrides:   emptyJSONObject(),
	}
}

// buildUpdateParams produces an UpdateScholarshipParams seeded from the
// existing row, then applies each extraction field that is NOT in overrides
// and whose value actually changed. It returns the params and the list of
// changed column names. Fields in editorial_overrides keep the existing value
// (human edit wins).
func buildUpdateParams(existing gen.Scholarship, ext extractor.Extraction, overrides map[string]struct{}) (gen.UpdateScholarshipParams, []string) {
	params := gen.UpdateScholarshipParams{
		ID:                   existing.ID,
		Title:                existing.Title,
		Description:          existing.Description,
		Country:              existing.Country,
		CountrySpaceID:       existing.CountrySpaceID,
		University:           existing.University,
		DegreeLevel:          existing.DegreeLevel,
		FieldsOfStudy:        existing.FieldsOfStudy,
		EligibleCountries:    existing.EligibleCountries,
		Funding:              existing.Funding,
		Deadline:             existing.Deadline,
		ApplicationUrl:       existing.ApplicationUrl,
		RequiredDocuments:    existing.RequiredDocuments,
		MinimumGpa:           existing.MinimumGpa,
		LanguageRequirements: existing.LanguageRequirements,
		ConfidenceOverall:    existing.ConfidenceOverall,
		VerificationStatus:   existing.VerificationStatus,
		FreshnessScore:       existing.FreshnessScore,
		OverrideSource:       existing.OverrideSource,
		EditorialOverrides:   existing.EditorialOverrides,
	}

	var changed []string

	// title
	if _, ov := overrides["title"]; !ov {
		if v := stringField(ext, "title"); v != "" && v != existing.Title {
			params.Title = v
			changed = append(changed, "title")
		}
	}
	// description (extraction key: description_summary)
	if _, ov := overrides["description"]; !ov {
		if v := stringPtrField(ext, "description_summary"); !strPtrEqual(v, existing.Description) {
			params.Description = v
			changed = append(changed, "description")
		}
	}
	// country
	if _, ov := overrides["country"]; !ov {
		if v := firstCountry(ext, "country"); v != "" && v != existing.Country {
			params.Country = v
			changed = append(changed, "country")
		}
	}
	// university
	if _, ov := overrides["university"]; !ov {
		if v := stringPtrField(ext, "university"); !strPtrEqual(v, existing.University) {
			params.University = v
			changed = append(changed, "university")
		}
	}
	// degree_level
	if _, ov := overrides["degree_level"]; !ov {
		if v := stringPtrField(ext, "degree_level"); !strPtrEqual(v, existing.DegreeLevel) {
			params.DegreeLevel = v
			changed = append(changed, "degree_level")
		}
	}
	// fields_of_study
	if _, ov := overrides["fields_of_study"]; !ov {
		if v := stringSliceField(ext, "fields_of_study"); !stringSliceEqual(v, existing.FieldsOfStudy) {
			params.FieldsOfStudy = v
			changed = append(changed, "fields_of_study")
		}
	}
	// eligible_countries
	if _, ov := overrides["eligible_countries"]; !ov {
		if v := stringSliceField(ext, "eligible_countries"); !stringSliceEqual(v, existing.EligibleCountries) {
			params.EligibleCountries = v
			changed = append(changed, "eligible_countries")
		}
	}
	// funding
	if _, ov := overrides["funding"]; !ov {
		if v := jsonbField(ext, "funding"); !bytesEqual(v, existing.Funding) {
			params.Funding = v
			changed = append(changed, "funding")
		}
	}
	// deadline
	if _, ov := overrides["deadline"]; !ov {
		if v := dateField(ext, "deadline"); !dateEqual(v, existing.Deadline) {
			params.Deadline = v
			changed = append(changed, "deadline")
		}
	}
	// application_url
	if _, ov := overrides["application_url"]; !ov {
		if v := stringPtrField(ext, "application_url"); !strPtrEqual(v, existing.ApplicationUrl) {
			params.ApplicationUrl = v
			changed = append(changed, "application_url")
		}
	}
	// required_documents
	if _, ov := overrides["required_documents"]; !ov {
		if v := stringSliceField(ext, "required_documents"); !stringSliceEqual(v, existing.RequiredDocuments) {
			params.RequiredDocuments = v
			changed = append(changed, "required_documents")
		}
	}
	// minimum_gpa
	if _, ov := overrides["minimum_gpa"]; !ov {
		if v := numericField(ext, "minimum_gpa"); !numericEqual(v, existing.MinimumGpa) {
			params.MinimumGpa = v
			changed = append(changed, "minimum_gpa")
		}
	}
	// language_requirements
	if _, ov := overrides["language_requirements"]; !ov {
		if v := jsonbField(ext, "language_requirements"); !bytesEqual(v, existing.LanguageRequirements) {
			params.LanguageRequirements = v
			changed = append(changed, "language_requirements")
		}
	}

	return params, changed
}

// ---------------------------------------------------------------------------
// Extraction field accessors
// ---------------------------------------------------------------------------

func parseExtraction(raw []byte) (extractor.Extraction, error) {
	if len(raw) == 0 {
		return extractor.Extraction{}, nil
	}
	var ext extractor.Extraction
	if err := json.Unmarshal(raw, &ext); err != nil {
		return nil, err
	}
	return ext, nil
}

// stringField returns the string form of an extraction field, or "".
func stringField(ext extractor.Extraction, key string) string {
	f, ok := ext[key]
	if !ok || f.Value == nil {
		return ""
	}
	switch v := f.Value.(type) {
	case string:
		return strings.TrimSpace(v)
	case []any:
		// country_array may arrive as a slice; take the first element.
		if len(v) > 0 {
			if s, ok := v[0].(string); ok {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

// firstCountry returns the first element of a country_array field, or "".
func firstCountry(ext extractor.Extraction, key string) string {
	f, ok := ext[key]
	if !ok || f.Value == nil {
		return ""
	}
	if arr, ok := f.Value.([]any); ok && len(arr) > 0 {
		if s, ok := arr[0].(string); ok {
			return strings.TrimSpace(s)
		}
	}
	if s, ok := f.Value.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func stringPtrField(ext extractor.Extraction, key string) *string {
	s := stringField(ext, key)
	if s == "" {
		return nil
	}
	return &s
}

func stringSliceField(ext extractor.Extraction, key string) []string {
	f, ok := ext[key]
	if !ok || f.Value == nil {
		return nil
	}
	arr, ok := f.Value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			if t := strings.TrimSpace(s); t != "" {
				out = append(out, t)
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func dateField(ext extractor.Extraction, key string) pgtype.Date {
	f, ok := ext[key]
	if !ok || f.Value == nil {
		return pgtype.Date{}
	}
	s, ok := f.Value.(string)
	if !ok {
		return pgtype.Date{}
	}
	t, err := time.Parse("2006-01-02", strings.TrimSpace(s))
	if err != nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: t, Valid: true}
}

func numericField(ext extractor.Extraction, key string) pgtype.Numeric {
	f, ok := ext[key]
	if !ok || f.Value == nil {
		return pgtype.Numeric{}
	}
	s := ""
	switch v := f.Value.(type) {
	case string:
		s = strings.TrimSpace(v)
	case float64:
		s = fmt.Sprintf("%g", v)
	default:
		s = fmt.Sprintf("%v", v)
	}
	if s == "" {
		return pgtype.Numeric{}
	}
	rat, ok := new(big.Rat).SetString(s)
	if !ok {
		return pgtype.Numeric{}
	}
	// Express the value as integer mantissa * 10^exp so pgtype can encode it.
	// For decimal inputs the denominator is a power of ten; for non-decimal
	// rationals we scale until the denominator is 1 (losing precision beyond
	// what the column stores, which is acceptable for a GPA field).
	num := new(big.Int).Set(rat.Num())
	den := new(big.Int).Set(rat.Denom())
	exp := int32(0)
	ten := big.NewInt(10)
	for den.Cmp(big.NewInt(1)) > 0 {
		if new(big.Int).Mod(den, ten).Sign() != 0 {
			// Non-power-of-ten denominator: scale by 10 until integral.
			num.Mul(num, ten)
			exp--
			den.Quo(den, ten)
			continue
		}
		num.Mul(num, ten)
		exp--
		den.Quo(den, ten)
	}
	return pgtype.Numeric{Int: num, Exp: exp, Valid: true}
}

func jsonbField(ext extractor.Extraction, key string) []byte {
	f, ok := ext[key]
	if !ok || f.Value == nil {
		return nil
	}
	b, err := json.Marshal(f.Value)
	if err != nil {
		return nil
	}
	return b
}

// ---------------------------------------------------------------------------
// Comparison helpers
// ---------------------------------------------------------------------------

func strPtrEqual(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func stringSliceEqual(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func bytesEqual(a, b []byte) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func dateEqual(a, b pgtype.Date) bool {
	if !a.Valid && !b.Valid {
		return true
	}
	if a.Valid != b.Valid {
		return false
	}
	return a.Time.Equal(b.Time)
}

func numericEqual(a, b pgtype.Numeric) bool {
	if !a.Valid && !b.Valid {
		return true
	}
	if a.Valid != b.Valid {
		return false
	}
	return numericBigFloat(a).Cmp(numericBigFloat(b)) == 0
}

func numericBigFloat(n pgtype.Numeric) *big.Float {
	if !n.Valid || n.Int == nil {
		return new(big.Float)
	}
	f := new(big.Float).SetInt(n.Int)
	// Multiply by 10^exp.
	exp := int(n.Exp)
	base := big.NewFloat(1)
	ten := big.NewFloat(10)
	for i := 0; i < exp; i++ {
		base.Mul(base, ten)
	}
	for i := 0; i > exp; i-- {
		base.Quo(base, ten)
	}
	f.Mul(f, base)
	return f
}

// ---------------------------------------------------------------------------
// Editorial overrides + review log
// ---------------------------------------------------------------------------

// parseOverrideKeys decodes the editorial_overrides JSONB into the set of
// field names that are under human editorial control. A nil/empty blob means
// no overrides.
func parseOverrideKeys(raw []byte) (map[string]struct{}, error) {
	out := make(map[string]struct{})
	if len(raw) == 0 {
		return out, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	for k := range m {
		out[k] = struct{}{}
	}
	return out, nil
}

// fieldChange is one entry in the intel_review_log.field_changes JSON array.
type fieldChange struct {
	Field string `json:"field"`
	From  any    `json:"from"`
	To    any    `json:"to"`
}

func marshalFieldChanges(existing gen.Scholarship, params gen.UpdateScholarshipParams, changed []string) ([]byte, error) {
	changes := make([]fieldChange, 0, len(changed))
	for _, c := range changed {
		fc := fieldChange{Field: c}
		fc.From, fc.To = snapshotField(existing, params, c)
		changes = append(changes, fc)
	}
	return json.Marshal(changes)
}

// snapshotField returns (before, after) values for a changed column name.
func snapshotField(existing gen.Scholarship, params gen.UpdateScholarshipParams, col string) (any, any) {
	switch col {
	case "title":
		return existing.Title, params.Title
	case "description":
		return derefStr(existing.Description), derefStr(params.Description)
	case "country":
		return existing.Country, params.Country
	case "university":
		return derefStr(existing.University), derefStr(params.University)
	case "degree_level":
		return derefStr(existing.DegreeLevel), derefStr(params.DegreeLevel)
	case "fields_of_study":
		return existing.FieldsOfStudy, params.FieldsOfStudy
	case "eligible_countries":
		return existing.EligibleCountries, params.EligibleCountries
	case "funding":
		return jsonOrNull(existing.Funding), jsonOrNull(params.Funding)
	case "deadline":
		return dateStr(existing.Deadline), dateStr(params.Deadline)
	case "application_url":
		return derefStr(existing.ApplicationUrl), derefStr(params.ApplicationUrl)
	case "required_documents":
		return existing.RequiredDocuments, params.RequiredDocuments
	case "minimum_gpa":
		return numericStr(existing.MinimumGpa), numericStr(params.MinimumGpa)
	case "language_requirements":
		return jsonOrNull(existing.LanguageRequirements), jsonOrNull(params.LanguageRequirements)
	}
	return nil, nil
}

func derefStr(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func dateStr(d pgtype.Date) any {
	if !d.Valid {
		return nil
	}
	return d.Time.Format("2006-01-02")
}

func numericStr(n pgtype.Numeric) any {
	if !n.Valid {
		return nil
	}
	return numericBigFloat(n).Text('f', -1)
}

func jsonOrNull(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return string(b)
	}
	return v
}

// ---------------------------------------------------------------------------
// Content body helpers
// ---------------------------------------------------------------------------

// bodyFromExtraction builds the guide body text from the AI-summarized
// description, falling back to the title when no summary was extracted.
func bodyFromExtraction(ext extractor.Extraction, title string) string {
	if desc := stringField(ext, "description_summary"); desc != "" {
		return desc
	}
	if title == "" {
		return ""
	}
	return title
}

// bodyHTML renders a minimal HTML wrapper around the plain-text body. The
// body column is the source of truth; body_html is a best-effort render.
func bodyHTML(body string) string {
	if body == "" {
		return ""
	}
	return "<p>" + html.EscapeString(body) + "</p>"
}

// ---------------------------------------------------------------------------
// Misc helpers
// ---------------------------------------------------------------------------

func ptr[T any](v T) *T { return &v }

func emptyJSONObject() []byte { return []byte("{}") }

func uuidToPg(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func pgToUUID(id pgtype.UUID) uuid.UUID {
	if !id.Valid {
		return uuid.Nil
	}
	return uuid.UUID(id.Bytes)
}

// isNotFound reports whether err is a pgx no-rows error. We avoid importing
// pgx directly here by matching the error message, keeping the promoter
// package decoupled from the driver for mock-based tests.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "no rows in result set")
}
