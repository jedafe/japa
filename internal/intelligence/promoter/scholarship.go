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

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/extractor"
)

// Store is the persistence surface the ScholarshipPromoter needs.
// *stores.Store satisfies this interface; tests substitute a mock.
//
// WithTx runs a callback inside a transaction. The callback receives a
// TxStore — a narrower interface that only exposes the write methods needed
// inside the transaction. This ensures createPath and updatePath perform
// their multi-table writes atomically (BUG-01/02 fix, §6.2 transaction
// boundary).
type Store interface {
	PersonaResolver
	GetIntelRecord(ctx context.Context, tenantID, id pgtype.UUID) (gen.IntelRecord, error)
	GetIntelSource(ctx context.Context, tenantID, id pgtype.UUID) (gen.IntelSource, error)
	GetScholarshipByIntelRecord(ctx context.Context, tenantID, intelRecordID pgtype.UUID) (gen.Scholarship, error)
	CreateScholarship(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateScholarshipParams) (gen.Scholarship, error)
	UpdateScholarship(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateScholarshipParams) error
	UpdateIntelRecordStatus(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelRecordStatusParams) error
	CreateIntelReviewLog(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateIntelReviewLogParams) error
	// GetCountrySpace resolves the country-level geo space (§05) id for an
	// ISO-3 country code, so the promoter can stamp scholarships.country_space_id.
	// Returns (pgtype.UUID{}, nil) when no country space exists yet; the
	// promoter treats that as a nullable country_space_id (spec §455). BUG-005 fix.
	GetCountrySpace(ctx context.Context, tenantID pgtype.UUID, countryCode string) (pgtype.UUID, error)
	WithTx(ctx context.Context, fn func(TxStore) error) error
}

// TxStore is the subset of Store needed inside a transaction. It exposes
// only the write methods that createPath and updatePath call, so the
// transactional store can be a narrower type. *stores.Store satisfies this
// interface (via WithTx's callback), and the test mock satisfies it
// directly.
type TxStore interface {
	CreateScholarship(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateScholarshipParams) (gen.Scholarship, error)
	UpdateIntelRecordStatus(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelRecordStatusParams) error
	UpdateScholarship(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateScholarshipParams) error
	CreateIntelReviewLog(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateIntelReviewLogParams) error
}

// Default freshness score for a freshly promoted / re-verified row. The
// freshness worker (Task 21) decays this over time; the promoter resets it to
// full whenever a re-crawl confirms or updates the projection.
const freshScore int32 = 100

// ScholarshipPromoter promotes scholarship intel_records into the
// scholarships projection table. Under Approach B (ADR-076 Amendment 1) it
// writes ONLY the projection row — the contents guide entry is generated
// later by the AI Content Studio via intel_record_id.
//
// It implements the editorial-override-respect invariant: on the update path,
// any field present in scholarships.editorial_overrides is left untouched so a
// human edit always wins over a re-crawl.
type ScholarshipPromoter struct {
	store    Store
	tenantID pgtype.UUID // tenant the generated guide rows are stamped with
	authorID pgtype.UUID // system/automation user credited as the guide author
	aiModel  *string     // AI model used for extraction (ADR-088, INTEL-003)
	now      func() time.Time
}

// ScholarshipPromoterOption configures a ScholarshipPromoter.
type ScholarshipPromoterOption func(*ScholarshipPromoter)

// WithClock overrides the clock used for publish timestamps (tests).
func WithClock(fn func() time.Time) ScholarshipPromoterOption {
	return func(p *ScholarshipPromoter) { p.now = fn }
}

// WithAIModel sets the AI model name stamped on created content rows (ADR-088).
func WithAIModel(model string) ScholarshipPromoterOption {
	return func(p *ScholarshipPromoter) { p.aiModel = &model }
}

// NewScholarshipPromoter creates a promoter backed by the given store. The
// tenantID and authorID identify the tenant and system author used for
// review-log actor stamps on the update path.
func NewScholarshipPromoter(store Store, tenantID, authorID pgtype.UUID, opts ...ScholarshipPromoterOption) *ScholarshipPromoter {
	p := &ScholarshipPromoter{
		store:    store,
		tenantID: tenantID,
		authorID: authorID,
		aiModel:  ptr("gpt-4o-mini"), // default from system-settings-catalog §INTEL
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
// projection row, creating it or updating it in place while respecting
// editorial overrides. The contents guide entry is NOT created here —
// Studio generates it from the extraction data (ADR-076 Amendment 1).
func (p *ScholarshipPromoter) Promote(ctx context.Context, recordID uuid.UUID) (PromoteResult, error) {
	pgID := uuidToPg(recordID)

	record, err := p.store.GetIntelRecord(ctx, p.tenantID, pgID)
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

	source, err := p.store.GetIntelSource(ctx, p.tenantID, record.SourceID)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: load intel source: %w", err)
	}

	// Does a projection row already exist for this intel record? A not-found
	// error means we take the create path; any other error propagates.
	existing, err := p.store.GetScholarshipByIntelRecord(ctx, p.tenantID, pgID)
	if err != nil {
		if isNotFound(err) {
			return p.createPath(ctx, record, source, ext)
		}
		return PromoteResult{}, fmt.Errorf("promoter: load existing scholarship: %w", err)
	}

	return p.updatePath(ctx, record, existing, ext)
}

// createPath inserts the scholarships projection row and flips the intel
// record to 'extracted' status (awaiting Studio editorial promotion).
// The contents guide row is NO LONGER created here — Studio generates it
// from the extraction data via intel_record_id (ADR-076 Amendment 1).
func (p *ScholarshipPromoter) createPath(ctx context.Context, record gen.IntelRecord, source gen.IntelSource, ext extractor.Extraction) (PromoteResult, error) {
	// Resolve the destination country's geo space (§05) so the space-scoped
	// radar feed (ListScholarshipsBySpace) can match this scholarship. The
	// lookup is best-effort: when no country space exists for the extracted
	// ISO-3 code, country_space_id stays NULL and the radar falls back to the
	// ?country= filter. BUG-005 fix (spec §455).
	countryCode := firstCountry(ext, "country")
	var countrySpaceID pgtype.UUID
	if countryCode != "" {
		if id, err := p.store.GetCountrySpace(ctx, p.tenantID, countryCode); err == nil && id.Valid {
			countrySpaceID = id
		}
	}

	scholarshipParams := buildCreateParams(record, source, pgtype.UUID{}, ext) // guide_id NULL — Studio fills it later
	scholarshipParams.CountrySpaceID = countrySpaceID                          // override the invalid pgtype.UUID{} default

	var result PromoteResult
	err := p.store.WithTx(ctx, func(tx TxStore) error {
		scholarship, err := tx.CreateScholarship(ctx, p.tenantID, scholarshipParams)
		if err != nil {
			return fmt.Errorf("promoter: create scholarship: %w", err)
		}
		if err := tx.UpdateIntelRecordStatus(ctx, p.tenantID, gen.UpdateIntelRecordStatusParams{
			ID:     record.ID,
			Status: "extracted",
		}); err != nil {
			return fmt.Errorf("promoter: mark intel record extracted: %w", err)
		}
		result = PromoteResult{
			DomainID:  pgToUUID(scholarship.ID),
			ContentID: uuid.Nil, // no contents row — Studio generates it later
			Action:    ActionCreated,
		}
		return nil
	})
	return result, err
}

// updatePath applies the new extraction to the existing scholarship row,
// skipping any field present in editorial_overrides, and records an
// auto_update review-log entry when fields actually change. The scholarship
// update and the review log write run inside a single transaction so the
// audit trail is always consistent with the projection state (BUG-02 fix,
// §6.2).
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
		if err := p.store.WithTx(ctx, func(tx TxStore) error {
			return tx.UpdateScholarship(ctx, p.tenantID, params)
		}); err != nil {
			return PromoteResult{}, fmt.Errorf("promoter: bump scholarship freshness: %w", err)
		}
		return PromoteResult{
			DomainID:  pgToUUID(existing.ID),
			ContentID: pgToUUID(existing.GuideID), // uuid.Nil when Studio hasn't generated the guide yet
			Action:    ActionVerified,
		}, nil
	}

	params.FreshnessScore = freshScore
	changesJSON, err := marshalFieldChanges(existing, params, changed)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: marshal field changes: %w", err)
	}
	note := "automatic re-crawl update"
	reviewLogParams := gen.CreateIntelReviewLogParams{
		IntelRecordID: record.ID,
		ActorUserID:   p.authorID,
		Action:        "auto_update",
		FieldChanges:  changesJSON,
		Note:          &note,
	}

	var result PromoteResult
	err = p.store.WithTx(ctx, func(tx TxStore) error {
		if err := tx.UpdateScholarship(ctx, p.tenantID, params); err != nil {
			return fmt.Errorf("promoter: update scholarship: %w", err)
		}
		if err := tx.CreateIntelReviewLog(ctx, p.tenantID, reviewLogParams); err != nil {
			return fmt.Errorf("promoter: write auto_update review log: %w", err)
		}
		result = PromoteResult{
			DomainID:      pgToUUID(existing.ID),
			ContentID:     pgToUUID(existing.GuideID), // uuid.Nil when Studio hasn't generated the guide yet
			Action:        ActionUpdated,
			ChangedFields: changed,
		}
		return nil
	})
	return result, err
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
		FieldsOfStudy:        orEmptySlice(stringSliceField(ext, "fields_of_study")),
		EligibleCountries:    orEmptySlice(stringSliceField(ext, "eligible_countries")),
		Funding:              orEmptyJSON(jsonbField(ext, "funding")),
		Deadline:             dateField(ext, "deadline"),
		ApplicationUrl:       stringPtrField(ext, "application_url"),
		RequiredDocuments:    orEmptySlice(stringSliceField(ext, "required_documents")),
		MinimumGpa:           numericField(ext, "minimum_gpa"),
		LanguageRequirements: orEmptyJSON(jsonbField(ext, "language_requirements")),
		ConfidenceOverall:    record.ConfidenceOverall,
		// spec §530: create path stamps 'verified' + 'ingestion'. The
		// scholarships CHECK constraint (migration 000429) only allows
		// verification_status IN ('verified','needs_review','conflict','stale')
		// and override_source IN ('ingestion','editorial','mixed'); the prior
		// 'auto' values violated the CHECK and rejected every insert.
		VerificationStatus: "verified",
		FreshnessScore:     freshScore,
		OverrideSource:     "ingestion",
		EditorialOverrides: emptyJSONObject(),
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
			params.FieldsOfStudy = orEmptySlice(v)
			changed = append(changed, "fields_of_study")
		}
	}
	// eligible_countries
	if _, ov := overrides["eligible_countries"]; !ov {
		if v := stringSliceField(ext, "eligible_countries"); !stringSliceEqual(v, existing.EligibleCountries) {
			params.EligibleCountries = orEmptySlice(v)
			changed = append(changed, "eligible_countries")
		}
	}
	// funding
	if _, ov := overrides["funding"]; !ov {
		if v := jsonbField(ext, "funding"); !bytesEqual(v, existing.Funding) {
			params.Funding = orEmptyJSON(v)
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
			params.RequiredDocuments = orEmptySlice(v)
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
			params.LanguageRequirements = orEmptyJSON(v)
			changed = append(changed, "language_requirements")
		}
	}

	return params, changed
}

// ---------------------------------------------------------------------------
// Extraction field accessors
// ---------------------------------------------------------------------------

func parseExtraction(raw []byte) (extractor.Extraction, error) {
	return parseExtractionShared(raw)
}

// orEmptySlice returns s if non-nil/non-empty, or []string{} otherwise.
// Prevents NULL insertion into NOT NULL array columns with DEFAULT '{}'.
func orEmptySlice(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// orEmptyJSON returns b if non-nil, or []byte(`{}`) otherwise.
// Prevents NULL insertion into NOT NULL jsonb columns with DEFAULT '{}'.
func orEmptyJSON(b []byte) []byte {
	if b == nil {
		return []byte(`{}`)
	}
	return b
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
		return []string{}
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

// bodyHTML renders a minimal HTML wrapper around the plain-text body. The
// body column is the source of truth; body_html is a best-effort render.
// Shared with other promoters (e.g. visa) that still create contents rows.
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
