package promoter

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/extractor"
	"github.com/jedafe/japa/internal/intelligence/reviewerctx"
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
	GetIntelRecord(ctx context.Context, tenantID, id pgtype.UUID) (gen.IntelRecord, error)
	GetIntelSource(ctx context.Context, tenantID, id pgtype.UUID) (gen.IntelSource, error)
	GetHousingGuideByIntelRecord(ctx context.Context, tenantID, intelRecordID pgtype.UUID) (gen.HousingGuide, error)
	CreateHousingGuideIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateHousingGuideIntelParams) (gen.HousingGuide, error)
	UpdateHousingGuideIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateHousingGuideIntelParams) error
	CreateContent(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateContentParams) (gen.CreateContentRow, error)
	UpdateIntelRecordPromoted(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelRecordPromotedParams) error
	CreateIntelReviewLog(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateIntelReviewLogParams) error
	// GetCountrySpace resolves the country-level geo space (§05) id for an
	// ISO-3 country code, so the promoter can stamp
	// housing_guides.destination_space_id. Returns (pgtype.UUID{}, nil) when
	// no country space exists yet; the promoter treats that as a nullable
	// destination_space_id. BUG-005 fix.
	GetCountrySpace(ctx context.Context, tenantID pgtype.UUID, countryCode string) (pgtype.UUID, error)
	WithTx(ctx context.Context, fn func(HousingTxStore) error) error
}

// HousingTxStore is the subset of HousingStore needed inside a transaction.
// It exposes only the write methods that createPath and updatePath call, so
// the transactional store can be a narrower type. *stores.Store satisfies
// this interface (via WithTx's callback), and the test mock satisfies it
// directly.
type HousingTxStore interface {
	CreateContent(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateContentParams) (gen.CreateContentRow, error)
	CreateHousingGuideIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateHousingGuideIntelParams) (gen.HousingGuide, error)
	UpdateIntelRecordPromoted(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelRecordPromotedParams) error
	UpdateHousingGuideIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateHousingGuideIntelParams) error
	CreateIntelReviewLog(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateIntelReviewLogParams) error
}

// HousingPromoter promotes housing intel_records into the housing_guides
// projection table plus the contents guide entry that backs it
// (guide_category='accommodation'). It implements the editorial-override
// invariant: on the update path, any field present in editorial_overrides is
// left untouched so a human edit always wins over a re-crawl.
type HousingPromoter struct {
	store    HousingStore
	tenantID pgtype.UUID
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
// tenantID and authorID identify the tenant and system author stamped onto
// the contents guide rows created by the create path.
func NewHousingPromoter(store HousingStore, tenantID, authorID pgtype.UUID, opts ...HousingPromoterOption) *HousingPromoter {
	p := &HousingPromoter{store: store, tenantID: tenantID, authorID: authorID, aiModel: ptr("gpt-4o-mini"), now: time.Now}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Entity returns the entity_type this promoter handles.
func (p *HousingPromoter) Entity() string { return "housing" }

// ErrNotHousing is returned when Promote is called on an intel record whose
// entity_type is not "housing".
var ErrNotHousing = fmt.Errorf("promoter: intel record is not a housing")

// Promote maps the intel record identified by recordID to a housing_guides
// projection row, creating it (plus its backing guide entry) or updating it
// in place while respecting editorial overrides.
func (p *HousingPromoter) Promote(ctx context.Context, recordID uuid.UUID) (PromoteResult, error) {
	pgID := uuidToPg(recordID)

	record, err := p.store.GetIntelRecord(ctx, p.tenantID, pgID)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: load intel record: %w", err)
	}
	if record.EntityType != p.Entity() {
		return PromoteResult{}, ErrNotHousing
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
	existing, err := p.store.GetHousingGuideByIntelRecord(ctx, p.tenantID, pgID)
	if err != nil {
		if isNotFound(err) {
			return p.housingCreatePath(ctx, record, source, ext)
		}
		return PromoteResult{}, fmt.Errorf("promoter: load existing housing guide: %w", err)
	}

	return p.housingUpdatePath(ctx, record, existing, ext)
}

// housingCreatePath inserts the contents guide entry, the housing_guides
// projection row, and flips the intel record to 'published' with the promoted
// row id. All three writes run inside a single transaction so a failure in
// any one rolls back the others — no orphaned content rows (BUG-01 fix,
// §6.2).
func (p *HousingPromoter) housingCreatePath(ctx context.Context, record gen.IntelRecord, source gen.IntelSource, ext extractor.Extraction) (PromoteResult, error) {
	title := housingTitle(ext, source.Name)
	body := bodyFromHousingExtraction(ext, title)
	slug := content.Slugify(title)
	hash := content.ComputeContentHash(body)
	canonicalURL := source.SourceUrl

	// Resolve the destination country's geo space (§05) so the space-scoped
	// feed can match this housing guide. The lookup is best-effort: when no
	// country space exists for the extracted ISO-3 code,
	// destination_space_id stays NULL. BUG-005 fix (spec §455).
	countryCode := firstCountry(ext, "country")
	var destinationSpaceID pgtype.UUID
	if countryCode != "" {
		if id, err := p.store.GetCountrySpace(ctx, p.tenantID, countryCode); err == nil && id.Valid {
			destinationSpaceID = id
		}
	}

	// Resolve the city-level geo space if a city was extracted. There is no
	// dedicated city-space lookup, so we fall back to GetCountrySpace with
	// the city name; when no match is found city_space_id stays NULL.
	var citySpaceID pgtype.UUID
	if city := stringField(ext, "city"); city != "" {
		if id, err := p.store.GetCountrySpace(ctx, p.tenantID, city); err == nil && id.Valid {
			citySpaceID = id
		}
	}

	contentParams := gen.CreateContentParams{
		TenantID:        p.tenantID,
		AuthorID:        resolveAuthorID(ctx, p.store, source, p.tenantID, p.authorID),
		FieldValues:     []byte(`{}`),
		ContentType:     "guide",
		Title:           title,
		Slug:            slug,
		Body:            body,
		BodyHtml:        "<p>" + html.EscapeString(body) + "</p>",
		ContentHash:     &hash,
		CanonicalUrl:    &canonicalURL,
		Status:          "published",
		Visibility:      "public",
		SeoIndexable:    true,
		ReadTimeMinutes: content.ComputeReadTime(body),
		GuideCategory:   ptr("accommodation"),
		PublishAt:       pgtype.Timestamptz{Time: p.now(), Valid: true},
		PublishedAt:     pgtype.Timestamptz{Time: p.now(), Valid: true},
		AiGenerated:     true,      // ADR-088: mark as AI-generated (INTEL-003)
		AiModel:         p.aiModel, // from intel.extraction_model system setting
		AiGeneratedAt:   pgtype.Timestamptz{Time: p.now(), Valid: true},
	}
	// INTEL-026 fix: stamp hitl_reviewed_by when a human approved the record.
	if reviewerID, ok := reviewerctx.ReviewerFromContext(ctx); ok {
		contentParams.HitlReviewedBy = reviewerID
		contentParams.HitlReviewedAt = pgtype.Timestamptz{Time: p.now(), Valid: true}
	}
	housingParams := buildHousingCreateParams(record, ext)
	housingParams.DestinationSpaceID = destinationSpaceID
	housingParams.CitySpaceID = citySpaceID

	var result PromoteResult
	err := p.store.WithTx(ctx, func(tx HousingTxStore) error {
		contentRow, err := tx.CreateContent(ctx, p.tenantID, contentParams)
		if err != nil {
			return fmt.Errorf("promoter: create guide content: %w", err)
		}
		// housing_guides has no guide_id FK; the content row is linked
		// implicitly via intel_record_id and exists for the guide catalog.
		housing, err := tx.CreateHousingGuideIntel(ctx, p.tenantID, housingParams)
		if err != nil {
			return fmt.Errorf("promoter: create housing guide: %w", err)
		}
		if err := tx.UpdateIntelRecordPromoted(ctx, p.tenantID, gen.UpdateIntelRecordPromotedParams{
			ID:               record.ID,
			PromotedRecordID: housing.ID,
		}); err != nil {
			return fmt.Errorf("promoter: mark intel record promoted: %w", err)
		}
		result = PromoteResult{
			DomainID:  pgToUUID(housing.ID),
			ContentID: pgToUUID(contentRow.ID),
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

	if len(changed) == 0 {
		// No field-level changes: skip the auto_update log, just bump
		// last_verified_at + freshness_score by re-stamping the row with
		// its current values at full freshness.
		params.FreshnessScore = freshScore
		if err := p.store.WithTx(ctx, func(tx HousingTxStore) error {
			return tx.UpdateHousingGuideIntel(ctx, p.tenantID, params)
		}); err != nil {
			return PromoteResult{}, fmt.Errorf("promoter: bump housing guide freshness: %w", err)
		}
		return PromoteResult{
			DomainID: pgToUUID(existing.ID),
			Action:   ActionVerified,
		}, nil
	}

	params.FreshnessScore = freshScore
	changesJSON, err := marshalHousingFieldChanges(existing, params, changed)
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
	err = p.store.WithTx(ctx, func(tx HousingTxStore) error {
		if err := tx.UpdateHousingGuideIntel(ctx, p.tenantID, params); err != nil {
			return fmt.Errorf("promoter: update housing guide: %w", err)
		}
		if err := tx.CreateIntelReviewLog(ctx, p.tenantID, reviewLogParams); err != nil {
			return fmt.Errorf("promoter: write auto_update review log: %w", err)
		}
		result = PromoteResult{
			DomainID:      pgToUUID(existing.ID),
			Action:        ActionUpdated,
			ChangedFields: changed,
		}
		return nil
	})
	return result, err
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
	if _, ov := overrides["accommodation_type"]; !ov {
		if v := stringPtrField(ext, "accommodation_type"); !strPtrEqual(v, existing.AccommodationType) {
			params.AccommodationType = v
			changed = append(changed, "accommodation_type")
		}
	}
	// search_platforms
	if _, ov := overrides["search_platforms"]; !ov {
		if v := jsonbField(ext, "search_platforms"); !bytesEqual(v, existing.SearchPlatforms) {
			params.SearchPlatforms = v
			changed = append(changed, "search_platforms")
		}
	}
	// landlord_requirements
	if _, ov := overrides["landlord_requirements"]; !ov {
		if v := stringPtrField(ext, "landlord_requirements"); !strPtrEqual(v, existing.LandlordRequirements) {
			params.LandlordRequirements = v
			changed = append(changed, "landlord_requirements")
		}
	}
	// rent_ranges
	if _, ov := overrides["rent_ranges"]; !ov {
		if v := jsonbField(ext, "rent_ranges"); !bytesEqual(v, existing.RentRanges) {
			params.RentRanges = v
			changed = append(changed, "rent_ranges")
		}
	}
	// scam_categories
	if _, ov := overrides["scam_categories"]; !ov {
		if v := stringSliceField(ext, "scam_categories"); !stringSliceEqual(v, existing.ScamCategories) {
			params.ScamCategories = v
			changed = append(changed, "scam_categories")
		}
	}
	// tips
	if _, ov := overrides["tips"]; !ov {
		if v := stringPtrField(ext, "tips"); !strPtrEqual(v, existing.Tips) {
			params.Tips = v
			changed = append(changed, "tips")
		}
	}

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

// housingTitle builds a human-readable guide title for a housing extraction.
// It prefers an explicit "title" field, then composes one from the extracted
// city/country, and finally falls back to the source name.
func housingTitle(ext extractor.Extraction, sourceName string) string {
	if t := stringField(ext, "title"); t != "" {
		return t
	}
	city := stringField(ext, "city")
	country := firstCountry(ext, "country")
	switch {
	case city != "" && country != "":
		return city + ", " + country + " accommodation guide"
	case city != "":
		return city + " accommodation guide"
	case country != "":
		return country + " accommodation guide"
	}
	return sourceName
}

// bodyFromHousingExtraction builds the guide body text from the extracted
// tips / landlord requirements / description, falling back to the title.
func bodyFromHousingExtraction(ext extractor.Extraction, title string) string {
	var parts []string
	if desc := stringField(ext, "description_summary"); desc != "" {
		parts = append(parts, desc)
	}
	if tips := stringField(ext, "tips"); tips != "" {
		parts = append(parts, tips)
	}
	if lr := stringField(ext, "landlord_requirements"); lr != "" {
		parts = append(parts, lr)
	}
	if len(parts) > 0 {
		return strings.Join(parts, "\n\n")
	}
	if title == "" {
		return ""
	}
	return title
}
