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

// HousingStore is the persistence surface the HousingPromoter needs.
// *stores.Store satisfies this interface; tests substitute a mock.
//
// WithTx runs a callback inside a transaction. The callback receives a
// HousingTxStore — a narrower interface that only exposes the write methods
// needed inside the transaction. This ensures createPath and updatePath
// perform their multi-table writes atomically (BUG-01/02 fix, §6.2
// transaction boundary).
type HousingStore interface {
	PersonaResolver
	GetIntelRecord(ctx context.Context, id pgtype.UUID) (gen.IntelRecord, error)
	GetIntelSource(ctx context.Context, id pgtype.UUID) (gen.IntelSource, error)
	GetHousingGuideByIntelRecord(ctx context.Context, intelRecordID pgtype.UUID) (gen.HousingGuide, error)
	CreateHousingGuideIntel(ctx context.Context, arg gen.CreateHousingGuideIntelParams) (gen.HousingGuide, error)
	UpdateHousingGuideIntel(ctx context.Context, arg gen.UpdateHousingGuideIntelParams) error
	UpdateIntelRecordStatus(ctx context.Context, arg gen.UpdateIntelRecordStatusParams) error
	CreateIntelReviewLog(ctx context.Context, arg gen.CreateIntelReviewLogParams) error
	// GetCountrySpace resolves the country-level geo space (§05) id for an
	// ISO-3 country code, so the promoter can stamp
	// housing_guides.destination_space_id. Returns (pgtype.UUID{}, nil) when
	// no country space exists yet; the promoter treats that as a nullable
	// destination_space_id. BUG-005 fix.
	GetCountrySpace(ctx context.Context, countryCode string) (pgtype.UUID, error)
	WithTx(ctx context.Context, fn func(HousingTxStore) error) error
}

// HousingTxStore is the subset of HousingStore needed inside a transaction.
// It exposes only the write methods that createPath and updatePath call, so
// the transactional store can be a narrower type. *stores.Store satisfies
// this interface (via WithTx's callback), and the test mock satisfies it
// directly.
type HousingTxStore interface {
	CreateHousingGuideIntel(ctx context.Context, arg gen.CreateHousingGuideIntelParams) (gen.HousingGuide, error)
	UpdateIntelRecordStatus(ctx context.Context, arg gen.UpdateIntelRecordStatusParams) error
	UpdateHousingGuideIntel(ctx context.Context, arg gen.UpdateHousingGuideIntelParams) error
	CreateIntelReviewLog(ctx context.Context, arg gen.CreateIntelReviewLogParams) error
}

// HousingPromoter promotes housing intel_records into the housing_guides
// projection table. It implements the editorial-override invariant: on the
// update path, any field present in editorial_overrides is left untouched so
// a human edit always wins over a re-crawl. The contents guide row is NO
// LONGER created here — Studio generates it from the extraction data via
// intel_record_id (ADR-076 Amendment 1).
type HousingPromoter struct {
	store    HousingStore
	authorID pgtype.UUID
	aiModel  *string // AI model used for extraction (ADR-088, INTEL-003)
	now      func() time.Time
}

// HousingPromoterOption configures a HousingPromoter.
type HousingPromoterOption func(*HousingPromoter)

// WithHousingClock overrides the clock used for publish timestamps (tests).
func WithHousingClock(fn func() time.Time) HousingPromoterOption {
	return func(p *HousingPromoter) { p.now = fn }
}

// WithHousingAIModel sets the AI model name stamped on created content rows (ADR-088).
func WithHousingAIModel(model string) HousingPromoterOption {
	return func(p *HousingPromoter) { p.aiModel = &model }
}

// NewHousingPromoter creates a promoter backed by the given store. The
// ID and authorID identify the tenant and system author stamped onto
// the housing_guides projection rows created by the create path.
func NewHousingPromoter(store HousingStore, authorID pgtype.UUID, opts ...HousingPromoterOption) *HousingPromoter {
	p := &HousingPromoter{store: store, authorID: authorID, aiModel: ptr("gpt-4o-mini"), now: time.Now}
	for _, opt := range opts {
		opt(p)
	}

	return p
}

// Entity returns the entity_type this promoter handles.
func (p *HousingPromoter) Entity() string { return "housing" }

// ErrNotHousing is returned when Promote is called on an intel record whose
// entity_type is not "housing".
var ErrNotHousing = errors.New("promoter: intel record is not a housing")

// Promote maps the intel record identified by recordID to a housing_guides
// projection row, creating it or updating it in place while respecting
// editorial overrides.
func (p *HousingPromoter) Promote(ctx context.Context, recordID uuid.UUID) (PromoteResult, error) {
	return promoteDomain(ctx, p.store, recordID, domainPromoteOps[gen.HousingGuide]{
		entityType: p.Entity(),
		notErr:     ErrNotHousing,
		loadLabel:  "housing guide",
		getExisting: func(ctx context.Context, pgID pgtype.UUID) (gen.HousingGuide, error) {
			return p.store.GetHousingGuideByIntelRecord(ctx, pgID)
		},
		createPath: p.housingCreatePath,
		updatePath: p.housingUpdatePath,
	})
}

// housingCreatePath inserts the housing_guides projection row and flips the
// intel record to 'extracted' status. The contents guide row is NO LONGER
// created here — Studio generates it from the extraction data via
// intel_record_id (ADR-076 Amendment 1). Both writes run inside a single
// transaction so a failure in either rolls back the other (BUG-01 fix,
// §6.2).
func (p *HousingPromoter) housingCreatePath(ctx context.Context, record gen.IntelRecord, _ gen.IntelSource, ext extractor.Extraction) (PromoteResult, error) {
	// Resolve the destination country's geo space (§05) so the space-scoped
	// feed can match this housing guide. The lookup is best-effort: when no
	// country space exists for the extracted ISO-3 code,
	// destination_space_id stays NULL. BUG-005 fix (spec §455).
	countryCode := firstCountry(ext, "country")

	var destinationSpaceID pgtype.UUID

	if countryCode != "" {
		if id, err := p.store.GetCountrySpace(ctx, countryCode); err == nil && id.Valid {
			destinationSpaceID = id
		}
	}

	// Resolve the city-level geo space if a city was extracted. There is no
	// dedicated city-space lookup, so we fall back to GetCountrySpace with
	// the city name; when no match is found city_space_id stays NULL.
	var citySpaceID pgtype.UUID

	if city := stringField(ext, "city"); city != "" {
		if id, err := p.store.GetCountrySpace(ctx, city); err == nil && id.Valid {
			citySpaceID = id
		}
	}

	housingParams := buildHousingCreateParams(record, ext)
	housingParams.DestinationSpaceID = destinationSpaceID
	housingParams.CitySpaceID = citySpaceID

	var result PromoteResult

	err := p.store.WithTx(ctx, func(tx HousingTxStore) error {
		housing, err := tx.CreateHousingGuideIntel(ctx, housingParams)
		if err != nil {
			return fmt.Errorf("promoter: create housing guide: %w", err)
		}

		if err := tx.UpdateIntelRecordStatus(ctx, gen.UpdateIntelRecordStatusParams{
			ID:     record.ID,
			Status: "extracted",
		}); err != nil {
			return fmt.Errorf("promoter: mark intel record extracted: %w", err)
		}

		result = PromoteResult{
			DomainID:  pgToUUID(housing.ID),
			ContentID: uuid.Nil,
			Action:    ActionCreated,
		}

		return nil
	})

	return result, err
}

// housingUpdatePath applies the new extraction to the existing housing_guides
// row, skipping any field present in editorial_overrides, and records an
// auto_update review-log entry when fields actually change. The housing
// update and the review log write run inside a single transaction so the
// audit trail is always consistent with the projection state (BUG-02 fix,
// §6.2).
func (p *HousingPromoter) housingUpdatePath(ctx context.Context, record gen.IntelRecord, existing gen.HousingGuide, ext extractor.Extraction) (PromoteResult, error) {
	overrides, err := parseOverrideKeys(existing.EditorialOverrides)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: parse editorial overrides: %w", err)
	}

	params, changed := buildHousingUpdateParams(existing, ext, overrides)
	params.FreshnessScore = freshScore

	var changesJSON []byte

	if len(changed) > 0 {
		if changesJSON, err = marshalHousingFieldChanges(existing, params, changed); err != nil {
			return PromoteResult{}, fmt.Errorf("promoter: marshal field changes: %w", err)
		}
	}

	return domainUpdateTx(ctx, record, p.authorID, "housing guide",
		pgToUUID(existing.ID), uuid.Nil, params, changed, changesJSON,
		func(ctx context.Context, fn func(HousingTxStore) error) error { return p.store.WithTx(ctx, fn) },
		HousingTxStore.UpdateHousingGuideIntel)
}

// ---------------------------------------------------------------------------
// Extraction -> housing params
// ---------------------------------------------------------------------------

// buildHousingCreateParams maps a validated extraction to a
// CreateHousingGuideIntelParams. DestinationSpaceID and CitySpaceID are
// resolved by the caller (geo worker / GetCountrySpace) and stamped in
// housingCreatePath.
func buildHousingCreateParams(record gen.IntelRecord, ext extractor.Extraction) gen.CreateHousingGuideIntelParams {
	return gen.CreateHousingGuideIntelParams{
		IntelRecordID:        record.ID,
		AccommodationType:    stringPtrField(ext, "accommodation_type"),
		SearchPlatforms:      jsonbField(ext, "search_platforms"),
		LandlordRequirements: stringPtrField(ext, "landlord_requirements"),
		RentRanges:           jsonbField(ext, "rent_ranges"),
		ScamCategories:       stringSliceField(ext, "scam_categories"),
		Tips:                 stringPtrField(ext, "tips"),
		Status:               "published",
		IsActive:             true,
		ConfidenceOverall:    record.ConfidenceOverall,
		// spec §530: create path stamps 'verified' + 'ingestion'. The
		// housing_guides CHECK constraint only allows verification_status IN
		// ('verified','needs_review','conflict','stale') and override_source
		// IN ('ingestion','editorial','mixed'); the prior 'auto' values
		// violated the CHECK and rejected every insert.
		VerificationStatus: "verified",
		FreshnessScore:     freshScore,
		OverrideSource:     "ingestion",
		EditorialOverrides: emptyJSONObject(),
	}
}

// buildHousingUpdateParams produces an UpdateHousingGuideIntelParams seeded
// from the existing row, then applies each extraction field that is NOT in
// overrides and whose value actually changed. It returns the params and the
// list of changed column names. Fields in editorial_overrides keep the
// existing value (human edit wins).
func buildHousingUpdateParams(existing gen.HousingGuide, ext extractor.Extraction, overrides map[string]struct{}) (gen.UpdateHousingGuideIntelParams, []string) {
	params := gen.UpdateHousingGuideIntelParams{
		ID:                   existing.ID,
		AccommodationType:    existing.AccommodationType,
		SearchPlatforms:      existing.SearchPlatforms,
		LandlordRequirements: existing.LandlordRequirements,
		RentRanges:           existing.RentRanges,
		ScamCategories:       existing.ScamCategories,
		Tips:                 existing.Tips,
		ConfidenceOverall:    existing.ConfidenceOverall,
		VerificationStatus:   existing.VerificationStatus,
		FreshnessScore:       existing.FreshnessScore,
		OverrideSource:       existing.OverrideSource,
		EditorialOverrides:   existing.EditorialOverrides,
	}

	var changed []string

	// accommodation_type
	applyStrPtrField(overrides, ext, "accommodation_type", &params.AccommodationType, existing.AccommodationType, &changed)
	// search_platforms
	applyJSONField(overrides, ext, "search_platforms", &params.SearchPlatforms, existing.SearchPlatforms, &changed)
	// landlord_requirements
	applyStrPtrField(overrides, ext, "landlord_requirements", &params.LandlordRequirements, existing.LandlordRequirements, &changed)
	// rent_ranges
	applyJSONField(overrides, ext, "rent_ranges", &params.RentRanges, existing.RentRanges, &changed)
	// scam_categories
	applyStrSliceField(overrides, ext, "scam_categories", &params.ScamCategories, existing.ScamCategories, &changed)
	// tips
	applyStrPtrField(overrides, ext, "tips", &params.Tips, existing.Tips, &changed)

	return params, changed
}

// ---------------------------------------------------------------------------
// Review log field-change snapshot
// ---------------------------------------------------------------------------

func marshalHousingFieldChanges(existing gen.HousingGuide, params gen.UpdateHousingGuideIntelParams, changed []string) ([]byte, error) {
	changes := make([]fieldChange, 0, len(changed))

	for _, c := range changed {
		fc := fieldChange{Field: c}

		switch c {
		case "accommodation_type":
			fc.From, fc.To = derefStr(existing.AccommodationType), derefStr(params.AccommodationType)
		case "search_platforms":
			fc.From, fc.To = jsonOrNull(existing.SearchPlatforms), jsonOrNull(params.SearchPlatforms)
		case "landlord_requirements":
			fc.From, fc.To = derefStr(existing.LandlordRequirements), derefStr(params.LandlordRequirements)
		case "rent_ranges":
			fc.From, fc.To = jsonOrNull(existing.RentRanges), jsonOrNull(params.RentRanges)
		case "scam_categories":
			fc.From, fc.To = existing.ScamCategories, params.ScamCategories
		case "tips":
			fc.From, fc.To = derefStr(existing.Tips), derefStr(params.Tips)
		}

		changes = append(changes, fc)
	}

	return json.Marshal(changes)
}

// ---------------------------------------------------------------------------
// Content body helpers
// ---------------------------------------------------------------------------
