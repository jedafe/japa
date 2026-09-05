package promoter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/extractor"
)

// LanguageTestStore is the persistence surface the LanguageTestPromoter
// needs. *stores.Store satisfies this interface; tests substitute a mock.
//
// WithTx runs a callback inside a transaction. The callback receives a
// LanguageTestTxStore — a narrower interface that only exposes the write
// methods needed inside the transaction. This ensures createPath and
// updatePath perform their multi-table writes atomically (BUG-01/02 fix,
// §6.2 transaction boundary).
type LanguageTestStore interface {
	PersonaResolver
	GetIntelRecord(ctx context.Context, tenantID, id pgtype.UUID) (gen.IntelRecord, error)
	GetIntelSource(ctx context.Context, tenantID, id pgtype.UUID) (gen.IntelSource, error)
	GetLanguageTestRequirementByIntelRecord(ctx context.Context, tenantID, intelRecordID pgtype.UUID) (gen.LanguageTestRequirement, error)
	CreateLanguageTestRequirementIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateLanguageTestRequirementIntelParams) (gen.LanguageTestRequirement, error)
	UpdateLanguageTestRequirementIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateLanguageTestRequirementIntelParams) error
	UpdateIntelRecordStatus(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelRecordStatusParams) error
	CreateIntelReviewLog(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateIntelReviewLogParams) error
	// GetCountrySpace resolves the country-level geo space (§05) id for an
	// ISO-3 country code, so the promoter can stamp
	// language_test_requirements.destination_space_id. Returns
	// (pgtype.UUID{}, nil) when no country space exists yet; the promoter
	// treats that as a nullable destination_space_id. BUG-005 fix.
	GetCountrySpace(ctx context.Context, tenantID pgtype.UUID, countryCode string) (pgtype.UUID, error)
	// GetLanguageTestByName resolves the language_tests(id) FK for a test
	// name extracted as free text. The extraction may carry a test_name
	// rather than a test_id UUID; the promoter uses this lookup to resolve
	// the FK before inserting the projection row. Returns
	// (gen.LanguageTest{}, error) when no match is found.
	GetLanguageTestByName(ctx context.Context, tenantID pgtype.UUID, name string) (gen.LanguageTest, error)
	WithTx(ctx context.Context, fn func(LanguageTestTxStore) error) error
}

// LanguageTestTxStore is the subset of LanguageTestStore needed inside a
// transaction. It exposes only the write methods that createPath and
// updatePath call, so the transactional store can be a narrower type.
// *stores.Store satisfies this interface (via WithTx's callback), and the
// test mock satisfies it directly.
type LanguageTestTxStore interface {
	CreateLanguageTestRequirementIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateLanguageTestRequirementIntelParams) (gen.LanguageTestRequirement, error)
	UpdateIntelRecordStatus(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelRecordStatusParams) error
	UpdateLanguageTestRequirementIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateLanguageTestRequirementIntelParams) error
	CreateIntelReviewLog(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateIntelReviewLogParams) error
}

// LanguageTestPromoter promotes language_test_requirement intel_records into
// the language_test_requirements projection table. It implements the
// editorial-override invariant: on the update path, any field present in
// editorial_overrides is left untouched so a human edit always wins over a
// re-crawl. The contents guide row is NO LONGER created here — Studio
// generates it from the extraction data via intel_record_id (ADR-076
// Amendment 1).
type LanguageTestPromoter struct {
	store    LanguageTestStore
	tenantID pgtype.UUID
	authorID pgtype.UUID
	aiModel  *string // AI model used for extraction (ADR-088, INTEL-003)
	now      func() time.Time
}

// LanguageTestPromoterOption configures a LanguageTestPromoter.
type LanguageTestPromoterOption func(*LanguageTestPromoter)

// WithLanguageTestClock overrides the clock used for publish timestamps (tests).
func WithLanguageTestClock(fn func() time.Time) LanguageTestPromoterOption {
	return func(p *LanguageTestPromoter) { p.now = fn }
}

// WithLanguageTestAIModel sets the AI model name stamped on created content rows (ADR-088).
func WithLanguageTestAIModel(model string) LanguageTestPromoterOption {
	return func(p *LanguageTestPromoter) { p.aiModel = &model }
}

// NewLanguageTestPromoter creates a promoter backed by the given store. The
// tenantID and authorID identify the tenant and system author stamped onto
// the language_test_requirements projection rows created by the create path.
func NewLanguageTestPromoter(store LanguageTestStore, tenantID, authorID pgtype.UUID, opts ...LanguageTestPromoterOption) *LanguageTestPromoter {
	p := &LanguageTestPromoter{store: store, tenantID: tenantID, authorID: authorID, aiModel: ptr("gpt-4o-mini"), now: time.Now}
	for _, opt := range opts {
		opt(p)
	}

	return p
}

// Entity returns the entity_type this promoter handles.
func (p *LanguageTestPromoter) Entity() string { return "language_test_requirement" }

// ErrNotLanguageTest is returned when Promote is called on an intel record
// whose entity_type is not "language_test_requirement".
var ErrNotLanguageTest = errors.New("promoter: intel record is not a language_test_requirement")

// ErrLanguageTestIDMissing is returned when the extraction cannot resolve a
// test_id FK (neither a test_id UUID nor a resolvable test_name was
// provided). The caller should route the intel record to human review.
var ErrLanguageTestIDMissing = errors.New("promoter: language test requirement requires a test_id — routing to review")

// Promote maps the intel record identified by recordID to a
// language_test_requirements projection row, creating it or updating it in
// place while respecting editorial overrides.
func (p *LanguageTestPromoter) Promote(ctx context.Context, recordID uuid.UUID) (PromoteResult, error) {
	return promoteDomain(ctx, p.store, p.tenantID, recordID, domainPromoteOps[gen.LanguageTestRequirement]{
		entityType: p.Entity(),
		notErr:     ErrNotLanguageTest,
		loadLabel:  "language test requirement",
		getExisting: func(ctx context.Context, tenantID, pgID pgtype.UUID) (gen.LanguageTestRequirement, error) {
			return p.store.GetLanguageTestRequirementByIntelRecord(ctx, tenantID, pgID)
		},
		createPath: p.languageTestCreatePath,
		updatePath: p.languageTestUpdatePath,
	})
}

// languageTestCreatePath inserts the language_test_requirements projection
// row and flips the intel record to 'extracted' status. The contents guide
// row is NO LONGER created here — Studio generates it from the extraction
// data via intel_record_id (ADR-076 Amendment 1). Both writes run inside a
// single transaction so a failure in either rolls back the other (BUG-01
// fix, §6.2).
func (p *LanguageTestPromoter) languageTestCreatePath(ctx context.Context, record gen.IntelRecord, _ gen.IntelSource, ext extractor.Extraction) (PromoteResult, error) {
	// Resolve the destination country's geo space (§05) so the space-scoped
	// feed can match this language test requirement. The lookup is
	// best-effort: when no country space exists for the extracted ISO-3
	// code, destination_space_id stays NULL. BUG-005 fix (spec §455).
	countryCode := firstCountry(ext, "country")

	var destinationSpaceID pgtype.UUID

	if countryCode != "" {
		if id, err := p.store.GetCountrySpace(ctx, p.tenantID, countryCode); err == nil && id.Valid {
			destinationSpaceID = id
		}
	}

	// Resolve the language_tests(id) FK. The extraction may carry a test_id
	// UUID directly, or a test_name that must be looked up. When neither
	// resolves, the record is routed to human review rather than inserting a
	// row with a dangling/NULL FK.
	testID, err := p.resolveTestID(ctx, ext)
	if err != nil {
		return PromoteResult{}, err
	}

	params := buildLanguageTestCreateParams(record, ext)
	params.DestinationSpaceID = destinationSpaceID
	params.TestID = testID

	var result PromoteResult

	err = p.store.WithTx(ctx, func(tx LanguageTestTxStore) error {
		req, err := tx.CreateLanguageTestRequirementIntel(ctx, p.tenantID, params)
		if err != nil {
			return fmt.Errorf("promoter: create language test requirement: %w", err)
		}

		if err := tx.UpdateIntelRecordStatus(ctx, p.tenantID, gen.UpdateIntelRecordStatusParams{
			ID:     record.ID,
			Status: "extracted",
		}); err != nil {
			return fmt.Errorf("promoter: mark intel record extracted: %w", err)
		}

		result = PromoteResult{
			DomainID:  pgToUUID(req.ID),
			ContentID: uuid.Nil,
			Action:    ActionCreated,
		}

		return nil
	})

	return result, err
}

// languageTestUpdatePath applies the new extraction to the existing
// language_test_requirements row, skipping any field present in
// editorial_overrides, and records an auto_update review-log entry when
// fields actually change. The update and the review log write run inside a
// single transaction so the audit trail is always consistent with the
// projection state (BUG-02 fix, §6.2).
func (p *LanguageTestPromoter) languageTestUpdatePath(ctx context.Context, record gen.IntelRecord, existing gen.LanguageTestRequirement, ext extractor.Extraction) (PromoteResult, error) {
	overrides, err := parseOverrideKeys(existing.EditorialOverrides)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: parse editorial overrides: %w", err)
	}

	params, changed := buildLanguageTestUpdateParams(existing, ext, overrides)
	params.FreshnessScore = freshScore

	var changesJSON []byte

	if len(changed) > 0 {
		if changesJSON, err = marshalLanguageTestFieldChanges(existing, params, changed); err != nil {
			return PromoteResult{}, fmt.Errorf("promoter: marshal field changes: %w", err)
		}
	}

	return domainUpdateTx(ctx, record, p.authorID, p.tenantID, "language test requirement",
		pgToUUID(existing.ID), uuid.Nil, params, changed, changesJSON,
		func(ctx context.Context, fn func(LanguageTestTxStore) error) error { return p.store.WithTx(ctx, fn) },
		LanguageTestTxStore.UpdateLanguageTestRequirementIntel)
}

// resolveTestID resolves the language_tests(id) FK from the extraction. It
// first tries a test_id field parsed as a UUID; if that is absent or
// invalid, it falls back to a test_name lookup via GetLanguageTestByName.
// When neither resolves, it returns ErrLanguageTestIDMissing so the caller
// routes the record to human review.
func (p *LanguageTestPromoter) resolveTestID(ctx context.Context, ext extractor.Extraction) (pgtype.UUID, error) {
	if raw := stringField(ext, "test_id"); raw != "" {
		if id, err := uuid.Parse(raw); err == nil {
			return uuidToPg(id), nil
		}
	}

	if name := stringField(ext, "test_name"); name != "" {
		test, err := p.store.GetLanguageTestByName(ctx, p.tenantID, name)
		if err == nil && test.ID.Valid {
			return test.ID, nil
		}
	}

	return pgtype.UUID{}, ErrLanguageTestIDMissing
}

// ---------------------------------------------------------------------------
// Extraction -> language test requirement params
// ---------------------------------------------------------------------------

// buildLanguageTestCreateParams maps a validated extraction to a
// CreateLanguageTestRequirementIntelParams. DestinationSpaceID and TestID
// are resolved by the caller (geo worker / GetCountrySpace / test lookup)
// and stamped in languageTestCreatePath. GuideID is NULL (Approach B:
// Studio generates the guide from the extraction via intel_record_id).
func buildLanguageTestCreateParams(record gen.IntelRecord, ext extractor.Extraction) gen.CreateLanguageTestRequirementIntelParams {
	return gen.CreateLanguageTestRequirementIntelParams{
		IntelRecordID:  record.ID,
		VisaCategory:   stringPtrField(ext, "visa_category"),
		MinimumScore:   stringField(ext, "minimum_score"),
		ScoreBreakdown: jsonbField(ext, "score_breakdown"),
		Notes:          stringPtrField(ext, "notes"),
		SourceUrl:      stringPtrField(ext, "source_url"),
		Status:         "published",
		IsActive:       true,
		// spec §530: create path stamps 'verified' + 'ingestion'. The
		// language_test_requirements CHECK constraint only allows
		// verification_status IN ('verified','needs_review','conflict',
		// 'stale') and override_source IN ('ingestion','editorial','mixed');
		// the prior 'auto' values violated the CHECK and rejected every
		// insert.
		VerificationStatus: "verified",
		FreshnessScore:     freshScore,
		OverrideSource:     "ingestion",
		EditorialOverrides: emptyJSONObject(),
	}
}

// buildLanguageTestUpdateParams produces an UpdateLanguageTestRequirementIntelParams
// seeded from the existing row, then applies each extraction field that is
// NOT in overrides and whose value actually changed. It returns the params
// and the list of changed column names. Fields in editorial_overrides keep
// the existing value (human edit wins). TestID is never overwritten on the
// update path — it is an immutable FK set at create time.
func buildLanguageTestUpdateParams(existing gen.LanguageTestRequirement, ext extractor.Extraction, overrides map[string]struct{}) (gen.UpdateLanguageTestRequirementIntelParams, []string) {
	params := gen.UpdateLanguageTestRequirementIntelParams{
		ID:                 existing.ID,
		VisaCategory:       existing.VisaCategory,
		MinimumScore:       existing.MinimumScore,
		ScoreBreakdown:     existing.ScoreBreakdown,
		Notes:              existing.Notes,
		SourceUrl:          existing.SourceUrl,
		Status:             existing.Status,
		IsActive:           existing.IsActive,
		ConfidenceOverall:  existing.ConfidenceOverall,
		VerificationStatus: existing.VerificationStatus,
		FreshnessScore:     existing.FreshnessScore,
		OverrideSource:     existing.OverrideSource,
		EditorialOverrides: existing.EditorialOverrides,
	}

	var changed []string

	// visa_category
	applyStrPtrField(overrides, ext, "visa_category", &params.VisaCategory, existing.VisaCategory, &changed)
	// minimum_score
	applyStrField(overrides, ext, "minimum_score", &params.MinimumScore, existing.MinimumScore, &changed)
	// score_breakdown
	applyJSONField(overrides, ext, "score_breakdown", &params.ScoreBreakdown, existing.ScoreBreakdown, &changed)
	// notes
	applyStrPtrField(overrides, ext, "notes", &params.Notes, existing.Notes, &changed)
	// source_url
	applyStrPtrField(overrides, ext, "source_url", &params.SourceUrl, existing.SourceUrl, &changed)

	return params, changed
}

// ---------------------------------------------------------------------------
// Review log field-change snapshot
// ---------------------------------------------------------------------------

func marshalLanguageTestFieldChanges(existing gen.LanguageTestRequirement, params gen.UpdateLanguageTestRequirementIntelParams, changed []string) ([]byte, error) {
	changes := make([]fieldChange, 0, len(changed))

	for _, c := range changed {
		fc := fieldChange{Field: c}

		switch c {
		case "visa_category":
			fc.From, fc.To = derefStr(existing.VisaCategory), derefStr(params.VisaCategory)
		case "minimum_score":
			fc.From, fc.To = existing.MinimumScore, params.MinimumScore
		case "score_breakdown":
			fc.From, fc.To = jsonOrNull(existing.ScoreBreakdown), jsonOrNull(params.ScoreBreakdown)
		case "notes":
			fc.From, fc.To = derefStr(existing.Notes), derefStr(params.Notes)
		case "source_url":
			fc.From, fc.To = derefStr(existing.SourceUrl), derefStr(params.SourceUrl)
		}

		changes = append(changes, fc)
	}

	return json.Marshal(changes)
}
