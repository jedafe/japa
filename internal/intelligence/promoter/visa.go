package promoter

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/extractor"
	"github.com/jedafe/japa/internal/intelligence/reviewerctx"
)

// VisaStore is the persistence surface the VisaPromoter needs.
// *stores.Store satisfies this interface; tests substitute a mock.
//
// The visa domain is unified: it writes BOTH a visa_routes standalone
// projection row (via CreateVisaRouteIntel/UpdateVisaRouteIntel) AND a
// contents guide entry (guide_category='visa'). This mirrors how the
// scholarship promoter creates both a scholarships row and a contents guide
// row. WithTx runs the multi-table writes atomically (BUG-01/02 fix, §6.2
// transaction boundary).
type VisaStore interface {
	PersonaResolver
	GetIntelRecord(ctx context.Context, tenantID, id pgtype.UUID) (gen.IntelRecord, error)
	GetIntelSource(ctx context.Context, tenantID, id pgtype.UUID) (gen.IntelSource, error)
	GetVisaRouteByIntelRecord(ctx context.Context, tenantID, intelRecordID pgtype.UUID) (gen.VisaRoute, error)
	CreateVisaRouteIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateVisaRouteIntelParams) (gen.VisaRoute, error)
	UpdateVisaRouteIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateVisaRouteIntelParams) error
	CreateContent(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateContentParams) (gen.CreateContentRow, error)
	UpdateIntelRecordPromoted(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelRecordPromotedParams) error
	CreateIntelReviewLog(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateIntelReviewLogParams) error
	// GetCountrySpace resolves the destination country-level geo space (§05)
	// id for an ISO-3 country code, so the promoter can stamp
	// visa_routes.destination_space_id. Returns (pgtype.UUID{}, nil) when no
	// country space exists yet; the promoter treats that as a nullable
	// destination_space_id. BUG-005 fix (spec §455).
	GetCountrySpace(ctx context.Context, tenantID pgtype.UUID, countryCode string) (pgtype.UUID, error)
	WithTx(ctx context.Context, fn func(VisaTxStore) error) error
}

// VisaTxStore is the subset of VisaStore needed inside a transaction. It
// exposes only the write methods that createPath and updatePath call, so the
// transactional store can be a narrower type. *stores.Store satisfies this
// interface (via WithTx's callback), and the test mock satisfies it directly.
type VisaTxStore interface {
	CreateContent(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateContentParams) (gen.CreateContentRow, error)
	CreateVisaRouteIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateVisaRouteIntelParams) (gen.VisaRoute, error)
	UpdateIntelRecordPromoted(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelRecordPromotedParams) error
	UpdateVisaRouteIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateVisaRouteIntelParams) error
	CreateIntelReviewLog(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateIntelReviewLogParams) error
}

// VisaPromoter promotes visa intel_records into the visa_routes projection
// table plus a contents guide entry (guide_category='visa'). It implements
// the editorial-override-respect invariant: on the update path, any field
// present in visa_routes.editorial_overrides is left untouched so a human
// edit always wins over a re-crawl.
type VisaPromoter struct {
	store    VisaStore
	tenantID pgtype.UUID // tenant the generated guide rows are stamped with
	authorID pgtype.UUID // system/automation user credited as the guide author
	aiModel  *string     // AI model used for extraction (ADR-088, INTEL-003)
	now      func() time.Time
}

// VisaPromoterOption configures a VisaPromoter.
type VisaPromoterOption func(*VisaPromoter)

// WithVisaClock overrides the clock used for publish timestamps (tests).
func WithVisaClock(fn func() time.Time) VisaPromoterOption {
	return func(p *VisaPromoter) { p.now = fn }
}

// WithVisaAIModel sets the AI model name stamped on created content rows (ADR-088).
func WithVisaAIModel(model string) VisaPromoterOption {
	return func(p *VisaPromoter) { p.aiModel = &model }
}

// NewVisaPromoter creates a promoter backed by the given store. The tenantID
// and authorID identify the tenant and system author stamped onto the
// contents guide rows created by the create path.
func NewVisaPromoter(store VisaStore, tenantID, authorID pgtype.UUID, opts ...VisaPromoterOption) *VisaPromoter {
	p := &VisaPromoter{store: store, tenantID: tenantID, authorID: authorID, aiModel: ptr("gpt-4o-mini"), now: time.Now}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Entity returns the entity_type this promoter handles.
func (p *VisaPromoter) Entity() string { return "visa" }

// ErrNotVisa is returned when Promote is called on an intel record whose
// entity_type is not "visa".
var ErrNotVisa = fmt.Errorf("promoter: intel record is not a visa")

// Promote maps the intel record identified by recordID to a visa_routes
// projection row (plus its backing guide entry), creating it or updating it
// in place while respecting editorial overrides.
func (p *VisaPromoter) Promote(ctx context.Context, recordID uuid.UUID) (PromoteResult, error) {
	pgID := uuidToPg(recordID)

	record, err := p.store.GetIntelRecord(ctx, p.tenantID, pgID)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: load intel record: %w", err)
	}
	if record.EntityType != p.Entity() {
		return PromoteResult{}, ErrNotVisa
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
	existing, err := p.store.GetVisaRouteByIntelRecord(ctx, p.tenantID, pgID)
	if err != nil {
		if isNotFound(err) {
			return p.visaCreatePath(ctx, record, source, ext)
		}
		return PromoteResult{}, fmt.Errorf("promoter: load existing visa route: %w", err)
	}

	return p.visaUpdatePath(ctx, record, existing, ext)
}

// visaCreatePath inserts the contents guide entry, the visa_routes projection
// row, and flips the intel record to 'published' with the promoted row id.
// All three writes run inside a single transaction so a failure in any one
// rolls back the others — no orphaned content rows (BUG-01 fix, §6.2).
func (p *VisaPromoter) visaCreatePath(ctx context.Context, record gen.IntelRecord, source gen.IntelSource, ext extractor.Extraction) (PromoteResult, error) {
	name := stringField(ext, "name")
	if name == "" {
		name = source.Name // fall back to the source name rather than failing
	}
	body := bodyFromVisaExtraction(ext, name)
	slug := content.Slugify(name)
	hash := content.ComputeContentHash(body)
	canonicalURL := source.SourceUrl

	// Resolve the destination country's geo space (§05) so the space-scoped
	// visa feed can match this route. visa_routes.destination_space_id is
	// NOT NULL, so when no country space can be resolved we route the record
	// back to review rather than failing the promotion. BUG-005 fix (spec §455).
	countryCode := firstCountry(ext, "country")
	var destinationSpaceID pgtype.UUID
	if countryCode != "" {
		if id, err := p.store.GetCountrySpace(ctx, p.tenantID, countryCode); err == nil && id.Valid {
			destinationSpaceID = id
		}
	}
	if !destinationSpaceID.Valid {
		return PromoteResult{}, fmt.Errorf(
			"promoter: visa route requires a destination country space but none resolved for %q — routing to review",
			countryCode,
		)
	}

	contentParams := gen.CreateContentParams{
		TenantID:        p.tenantID,
		AuthorID:        resolveAuthorID(ctx, p.store, source, p.tenantID, p.authorID),
		FieldValues:     []byte(`{}`),
		ContentType:     "guide",
		Title:           name,
		Slug:            slug,
		Body:            body,
		BodyHtml:        bodyHTML(body),
		ContentHash:     &hash,
		CanonicalUrl:    &canonicalURL,
		Status:          "published",
		Visibility:      "public",
		SeoIndexable:    true,
		ReadTimeMinutes: content.ComputeReadTime(body),
		GuideCategory:   ptr("visa"),
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
	visaParams := buildVisaCreateParams(record, source, ext)
	visaParams.DestinationSpaceID = destinationSpaceID

	var result PromoteResult
	err := p.store.WithTx(ctx, func(tx VisaTxStore) error {
		contentRow, err := tx.CreateContent(ctx, p.tenantID, contentParams)
		if err != nil {
			return fmt.Errorf("promoter: create guide content: %w", err)
		}
		// visa_routes has no guide_id FK; the content row is the public guide
		// surface for the route and is tracked via PromoteResult.ContentID.
		visa, err := tx.CreateVisaRouteIntel(ctx, p.tenantID, visaParams)
		if err != nil {
			return fmt.Errorf("promoter: create visa route: %w", err)
		}
		if err := tx.UpdateIntelRecordPromoted(ctx, p.tenantID, gen.UpdateIntelRecordPromotedParams{
			ID:               record.ID,
			PromotedRecordID: visa.ID,
		}); err != nil {
			return fmt.Errorf("promoter: mark intel record promoted: %w", err)
		}
		result = PromoteResult{
			DomainID:  pgToUUID(visa.ID),
			ContentID: pgToUUID(contentRow.ID),
			Action:    ActionCreated,
		}
		return nil
	})
	return result, err
}

// visaUpdatePath applies the new extraction to the existing visa_routes row,
// skipping any field present in editorial_overrides, and records an
// auto_update review-log entry when fields actually change. The visa update
// and the review log write run inside a single transaction so the audit
// trail is always consistent with the projection state (BUG-02 fix, §6.2).
func (p *VisaPromoter) visaUpdatePath(ctx context.Context, record gen.IntelRecord, existing gen.VisaRoute, ext extractor.Extraction) (PromoteResult, error) {
	overrides, err := parseOverrideKeys(existing.EditorialOverrides)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: parse editorial overrides: %w", err)
	}

	params, changed := buildVisaUpdateParams(existing, ext, overrides)

	if len(changed) == 0 {
		// No field-level changes: skip the auto_update log, just bump
		// last_verified_at + freshness_score by re-stamping the row with
		// its current values at full freshness.
		params.FreshnessScore = freshScore
		if err := p.store.WithTx(ctx, func(tx VisaTxStore) error {
			return tx.UpdateVisaRouteIntel(ctx, p.tenantID, params)
		}); err != nil {
			return PromoteResult{}, fmt.Errorf("promoter: bump visa route freshness: %w", err)
		}
		return PromoteResult{
			DomainID:  pgToUUID(existing.ID),
			ContentID: uuid.Nil, // no new content on the update path
			Action:    ActionVerified,
		}, nil
	}

	params.FreshnessScore = freshScore
	changesJSON, err := marshalVisaFieldChanges(existing, params, changed)
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
	err = p.store.WithTx(ctx, func(tx VisaTxStore) error {
		if err := tx.UpdateVisaRouteIntel(ctx, p.tenantID, params); err != nil {
			return fmt.Errorf("promoter: update visa route: %w", err)
		}
		if err := tx.CreateIntelReviewLog(ctx, p.tenantID, reviewLogParams); err != nil {
			return fmt.Errorf("promoter: write auto_update review log: %w", err)
		}
		result = PromoteResult{
			DomainID:      pgToUUID(existing.ID),
			ContentID:     uuid.Nil,
			Action:        ActionUpdated,
			ChangedFields: changed,
		}
		return nil
	})
	return result, err
}

// ---------------------------------------------------------------------------
// Extraction -> visa route params
// ---------------------------------------------------------------------------

// buildVisaCreateParams maps a validated extraction to a
// CreateVisaRouteIntelParams. DestinationSpaceID is resolved by the caller
// (visaCreatePath) via GetCountrySpace and overridden after construction.
func buildVisaCreateParams(record gen.IntelRecord, source gen.IntelSource, ext extractor.Extraction) gen.CreateVisaRouteIntelParams {
	name := stringField(ext, "name")
	if name == "" {
		name = source.Name
	}
	visaCategory := stringField(ext, "visa_category")
	if visaCategory == "" {
		visaCategory = "other" // satisfy NOT NULL when the extractor omitted it
	}
	govSourceURL := stringField(ext, "gov_source_url")
	if govSourceURL == "" {
		govSourceURL = source.SourceUrl // satisfy NOT NULL via the crawled source URL
	}
	return gen.CreateVisaRouteIntelParams{
		TenantID:           record.TenantID,
		DestinationSpaceID: pgtype.UUID{}, // resolved by visaCreatePath
		VisaCategory:       visaCategory,
		Name:               name,
		Slug:               content.Slugify(name),
		EligibilitySummary: stringPtrField(ext, "eligibility_summary"),
		RequirementsJson:   jsonbField(ext, "requirements"),
		TypicalFeeUsd:      numericField(ext, "typical_fee_usd"),
		PathToPr:           stringPtrField(ext, "path_to_pr"),
		RenewalTerms:       stringPtrField(ext, "renewal_terms"),
		Restrictions:       stringPtrField(ext, "restrictions"),
		GovSourceUrl:       govSourceURL,
		Status:             "published",
		IsActive:           true,
		IntelRecordID:      record.ID,
		ConfidenceOverall:  record.ConfidenceOverall,
		// spec §530: create path stamps 'verified' + 'ingestion'. The
		// visa_routes CHECK constraint (migration 000444) only allows
		// verification_status IN ('verified','needs_review','conflict','stale')
		// and override_source IN ('ingestion','editorial','mixed').
		VerificationStatus: "verified",
		FreshnessScore:     freshScore,
		OverrideSource:     "ingestion",
		EditorialOverrides: emptyJSONObject(),
	}
}

// buildVisaUpdateParams produces an UpdateVisaRouteIntelParams seeded from
// the existing row, then applies each extraction field that is NOT in
// overrides and whose value actually changed. It returns the params and the
// list of changed column names. Fields in editorial_overrides keep the
// existing value (human edit wins).
//
// destination_space_id and slug are immutable on the update path (the
// UpdateVisaRouteIntel query does not touch them), so the country extraction
// is only honored on the create path.
func buildVisaUpdateParams(existing gen.VisaRoute, ext extractor.Extraction, overrides map[string]struct{}) (gen.UpdateVisaRouteIntelParams, []string) {
	params := gen.UpdateVisaRouteIntelParams{
		TenantID:             existing.TenantID,
		ID:                   existing.ID,
		Name:                 existing.Name,
		VisaCategory:         existing.VisaCategory,
		EligibilitySummary:   existing.EligibilitySummary,
		RequirementsJson:     existing.RequirementsJson,
		TypicalFeeUsd:        existing.TypicalFeeUsd,
		PathToPr:             existing.PathToPr,
		RenewalTerms:         existing.RenewalTerms,
		Restrictions:         existing.Restrictions,
		GovSourceUrl:         existing.GovSourceUrl,
		SourceUrlLastChecked: existing.SourceUrlLastChecked,
		ConfidenceOverall:    existing.ConfidenceOverall,
		VerificationStatus:   existing.VerificationStatus,
		FreshnessScore:       existing.FreshnessScore,
		OverrideSource:       existing.OverrideSource,
		EditorialOverrides:   existing.EditorialOverrides,
	}

	var changed []string

	// name
	if _, ov := overrides["name"]; !ov {
		if v := stringField(ext, "name"); v != "" && v != existing.Name {
			params.Name = v
			changed = append(changed, "name")
		}
	}
	// visa_category
	if _, ov := overrides["visa_category"]; !ov {
		if v := stringField(ext, "visa_category"); v != "" && v != existing.VisaCategory {
			params.VisaCategory = v
			changed = append(changed, "visa_category")
		}
	}
	// eligibility_summary
	if _, ov := overrides["eligibility_summary"]; !ov {
		if v := stringPtrField(ext, "eligibility_summary"); !strPtrEqual(v, existing.EligibilitySummary) {
			params.EligibilitySummary = v
			changed = append(changed, "eligibility_summary")
		}
	}
	// requirements (extraction key: requirements -> column requirements_json)
	if _, ov := overrides["requirements"]; !ov {
		if v := jsonbField(ext, "requirements"); !bytesEqual(v, existing.RequirementsJson) {
			params.RequirementsJson = v
			changed = append(changed, "requirements")
		}
	}
	// typical_fee_usd
	if _, ov := overrides["typical_fee_usd"]; !ov {
		if v := numericField(ext, "typical_fee_usd"); !numericEqual(v, existing.TypicalFeeUsd) {
			params.TypicalFeeUsd = v
			changed = append(changed, "typical_fee_usd")
		}
	}
	// path_to_pr
	if _, ov := overrides["path_to_pr"]; !ov {
		if v := stringPtrField(ext, "path_to_pr"); !strPtrEqual(v, existing.PathToPr) {
			params.PathToPr = v
			changed = append(changed, "path_to_pr")
		}
	}
	// renewal_terms
	if _, ov := overrides["renewal_terms"]; !ov {
		if v := stringPtrField(ext, "renewal_terms"); !strPtrEqual(v, existing.RenewalTerms) {
			params.RenewalTerms = v
			changed = append(changed, "renewal_terms")
		}
	}
	// restrictions
	if _, ov := overrides["restrictions"]; !ov {
		if v := stringPtrField(ext, "restrictions"); !strPtrEqual(v, existing.Restrictions) {
			params.Restrictions = v
			changed = append(changed, "restrictions")
		}
	}
	// gov_source_url
	if _, ov := overrides["gov_source_url"]; !ov {
		if v := stringField(ext, "gov_source_url"); v != "" && v != existing.GovSourceUrl {
			params.GovSourceUrl = v
			changed = append(changed, "gov_source_url")
		}
	}

	return params, changed
}

// marshalVisaFieldChanges builds the intel_review_log.field_changes JSON
// array capturing before/after values for each changed column.
func marshalVisaFieldChanges(existing gen.VisaRoute, params gen.UpdateVisaRouteIntelParams, changed []string) ([]byte, error) {
	changes := make([]fieldChange, 0, len(changed))
	for _, c := range changed {
		fc := fieldChange{Field: c}
		switch c {
		case "name":
			fc.From, fc.To = existing.Name, params.Name
		case "visa_category":
			fc.From, fc.To = existing.VisaCategory, params.VisaCategory
		case "eligibility_summary":
			fc.From, fc.To = derefStr(existing.EligibilitySummary), derefStr(params.EligibilitySummary)
		case "requirements":
			fc.From, fc.To = jsonOrNull(existing.RequirementsJson), jsonOrNull(params.RequirementsJson)
		case "typical_fee_usd":
			fc.From, fc.To = numericStr(existing.TypicalFeeUsd), numericStr(params.TypicalFeeUsd)
		case "path_to_pr":
			fc.From, fc.To = derefStr(existing.PathToPr), derefStr(params.PathToPr)
		case "renewal_terms":
			fc.From, fc.To = derefStr(existing.RenewalTerms), derefStr(params.RenewalTerms)
		case "restrictions":
			fc.From, fc.To = derefStr(existing.Restrictions), derefStr(params.Restrictions)
		case "gov_source_url":
			fc.From, fc.To = existing.GovSourceUrl, params.GovSourceUrl
		}
		changes = append(changes, fc)
	}
	return json.Marshal(changes)
}

// bodyFromVisaExtraction builds the guide body text from the AI-summarized
// eligibility summary, falling back to the route name when no summary was
// extracted.
func bodyFromVisaExtraction(ext extractor.Extraction, name string) string {
	if desc := stringField(ext, "eligibility_summary"); desc != "" {
		return desc
	}
	if name == "" {
		return ""
	}
	return name
}
