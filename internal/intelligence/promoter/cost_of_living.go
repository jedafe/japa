package promoter

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/extractor"
)

// CostOfLivingStore is the persistence surface the CostOfLivingPromoter
// needs. *stores.Store satisfies this interface; tests substitute a mock.
//
// Unlike the scholarship/job promoters, the cost-of-living promoter writes
// to the comparison_items table (ALTERed with intel columns by migration
// 000447) and does NOT create a contents guide row — it only creates the
// comparison_items projection row.
//
// WithTx runs a callback inside a transaction. The callback receives a
// CostOfLivingTxStore — a narrower interface that only exposes the write
// methods needed inside the transaction. This ensures createPath and
// updatePath perform their multi-table writes atomically (BUG-01/02 fix,
// §6.2 transaction boundary).
type CostOfLivingStore interface {
	GetIntelRecord(ctx context.Context, tenantID, id pgtype.UUID) (gen.IntelRecord, error)
	GetIntelSource(ctx context.Context, tenantID, id pgtype.UUID) (gen.IntelSource, error)
	GetComparisonItemByIntelRecord(ctx context.Context, tenantID, intelRecordID pgtype.UUID) (gen.ComparisonItem, error)
	CreateComparisonItemIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateComparisonItemIntelParams) (gen.ComparisonItem, error)
	UpdateComparisonItemIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateComparisonItemIntelParams) error
	UpdateIntelRecordPromoted(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelRecordPromotedParams) error
	CreateIntelReviewLog(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateIntelReviewLogParams) error
	// GetCountrySpace resolves the country-level geo space (§05) id for an
	// ISO-3 country code, so the promoter can stamp comparison_items.geo_space_id.
	// Returns (pgtype.UUID{}, nil) when no country space exists yet; the
	// promoter treats that as a nullable geo_space_id (spec §455). BUG-005 fix.
	GetCountrySpace(ctx context.Context, tenantID pgtype.UUID, countryCode string) (pgtype.UUID, error)
	WithTx(ctx context.Context, fn func(CostOfLivingTxStore) error) error
}

// CostOfLivingTxStore is the subset of CostOfLivingStore needed inside a
// transaction. It exposes only the write methods that createPath and
// updatePath call, so the transactional store can be a narrower type.
// *stores.Store satisfies this interface (via WithTx's callback), and the
// test mock satisfies it directly.
type CostOfLivingTxStore interface {
	CreateComparisonItemIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateComparisonItemIntelParams) (gen.ComparisonItem, error)
	UpdateIntelRecordPromoted(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelRecordPromotedParams) error
	UpdateComparisonItemIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateComparisonItemIntelParams) error
	CreateIntelReviewLog(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateIntelReviewLogParams) error
}

// CostOfLivingPromoter promotes cost_of_living intel_records into the
// comparison_items projection table. Unlike scholarships/jobs, it does not
// create a contents guide row — it only creates the comparison_items row.
//
// It implements the editorial-override-respect invariant: on the update
// path, any field present in comparison_items.editorial_overrides is left
// untouched so a human edit always wins over a re-crawl. Only
// intel_cost_data and the intel provenance columns
// (confidence_overall, verification_status, freshness_score,
// override_source, editorial_overrides) are updatable via the
// UpdateComparisonItemIntel query.
type CostOfLivingPromoter struct {
	store    CostOfLivingStore
	tenantID pgtype.UUID // tenant the projection rows are stamped with
	now      func() time.Time
}

// CostOfLivingPromoterOption configures a CostOfLivingPromoter.
type CostOfLivingPromoterOption func(*CostOfLivingPromoter)

// WithCostOfLivingClock overrides the clock used for timestamps (tests).
func WithCostOfLivingClock(fn func() time.Time) CostOfLivingPromoterOption {
	return func(p *CostOfLivingPromoter) { p.now = fn }
}

// NewCostOfLivingPromoter creates a promoter backed by the given store. The
// tenantID identifies the tenant stamped onto the comparison_items rows
// created by the create path.
func NewCostOfLivingPromoter(store CostOfLivingStore, tenantID pgtype.UUID, opts ...CostOfLivingPromoterOption) *CostOfLivingPromoter {
	p := &CostOfLivingPromoter{
		store:    store,
		tenantID: tenantID,
		now:      time.Now,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Entity returns the entity_type this promoter handles.
func (p *CostOfLivingPromoter) Entity() string { return "cost_of_living" }

// ErrNotCostOfLiving is returned when Promote is called on an intel record
// whose entity_type is not "cost_of_living".
var ErrNotCostOfLiving = fmt.Errorf("promoter: intel record is not a cost_of_living")

// Promote maps the intel record identified by recordID to a comparison_items
// projection row, creating it or updating it in place while respecting
// editorial overrides. Unlike the scholarship/job promoters, no contents
// guide row is created — the cost-of-living domain only projects into
// comparison_items.
func (p *CostOfLivingPromoter) Promote(ctx context.Context, recordID uuid.UUID) (PromoteResult, error) {
	pgID := uuidToPg(recordID)

	record, err := p.store.GetIntelRecord(ctx, p.tenantID, pgID)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: load intel record: %w", err)
	}
	if record.EntityType != p.Entity() {
		return PromoteResult{}, ErrNotCostOfLiving
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
	existing, err := p.store.GetComparisonItemByIntelRecord(ctx, p.tenantID, pgID)
	if err != nil {
		if isNotFound(err) {
			return p.costOfLivingCreatePath(ctx, record, source, ext)
		}
		return PromoteResult{}, fmt.Errorf("promoter: load existing comparison item: %w", err)
	}

	return p.costOfLivingUpdatePath(ctx, record, existing, ext)
}

// costOfLivingCreatePath inserts the comparison_items projection row and
// flips the intel record to 'published' with the promoted row id. Both
// writes run inside a single transaction so a failure in either rolls back
// the other — no orphaned projection rows (BUG-01 fix, §6.2).
func (p *CostOfLivingPromoter) costOfLivingCreatePath(ctx context.Context, record gen.IntelRecord, source gen.IntelSource, ext extractor.Extraction) (PromoteResult, error) {
	// The title is the city name; fall back to the source name rather than
	// failing the promotion when no city was extracted.
	title := stringField(ext, "city")
	if title == "" {
		title = source.Name
	}

	// Resolve the destination country's geo space (§05) so the
	// space-scoped comparison feed can match this item. The lookup is
	// best-effort: when no country space exists for the extracted ISO-3
	// code, geo_space_id stays NULL. BUG-005 fix (spec §455).
	countryCode := firstCountry(ext, "country")
	var geoSpaceID pgtype.UUID
	if countryCode != "" {
		if id, err := p.store.GetCountrySpace(ctx, p.tenantID, countryCode); err == nil && id.Valid {
			geoSpaceID = id
		}
	}

	params := buildCostOfLivingCreateParams(record, source, ext, p.tenantID)
	params.GeoSpaceID = geoSpaceID

	var result PromoteResult
	err := p.store.WithTx(ctx, func(tx CostOfLivingTxStore) error {
		item, err := tx.CreateComparisonItemIntel(ctx, p.tenantID, params)
		if err != nil {
			return fmt.Errorf("promoter: create comparison item: %w", err)
		}
		if err := tx.UpdateIntelRecordPromoted(ctx, p.tenantID, gen.UpdateIntelRecordPromotedParams{
			ID:               record.ID,
			PromotedRecordID: item.ID,
		}); err != nil {
			return fmt.Errorf("promoter: mark intel record promoted: %w", err)
		}
		result = PromoteResult{
			DomainID: pgToUUID(item.ID),
			Action:   ActionCreated,
		}
		return nil
	})
	return result, err
}

// costOfLivingUpdatePath applies the new extraction to the existing
// comparison_items row, skipping any field present in editorial_overrides,
// and records an auto_update review-log entry when fields actually change.
// The comparison item update and the review log write run inside a single
// transaction so the audit trail is always consistent with the projection
// state (BUG-02 fix, §6.2).
//
// Only intel_cost_data and the intel provenance columns are updatable via
// UpdateComparisonItemIntel; the title/geo_space_id/type_id/position are
// immutable after creation. So the only extraction-driven field that can
// change is intel_cost_data (from the cost_data extraction field).
func (p *CostOfLivingPromoter) costOfLivingUpdatePath(ctx context.Context, record gen.IntelRecord, existing gen.ComparisonItem, ext extractor.Extraction) (PromoteResult, error) {
	overrides, err := parseOverrideKeys(existing.EditorialOverrides)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: parse editorial overrides: %w", err)
	}

	params, changed := buildCostOfLivingUpdateParams(existing, ext, overrides, p.tenantID)

	if len(changed) == 0 {
		// No field-level changes: skip the auto_update log, just bump
		// freshness_score by re-stamping the row with its current values
		// at full freshness.
		params.FreshnessScore = freshScore
		if err := p.store.WithTx(ctx, func(tx CostOfLivingTxStore) error {
			return tx.UpdateComparisonItemIntel(ctx, p.tenantID, params)
		}); err != nil {
			return PromoteResult{}, fmt.Errorf("promoter: bump comparison item freshness: %w", err)
		}
		return PromoteResult{
			DomainID: pgToUUID(existing.ID),
			Action:   ActionVerified,
		}, nil
	}

	params.FreshnessScore = freshScore
	changesJSON, err := marshalCostOfLivingFieldChanges(existing, params, changed)
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
	err = p.store.WithTx(ctx, func(tx CostOfLivingTxStore) error {
		if err := tx.UpdateComparisonItemIntel(ctx, p.tenantID, params); err != nil {
			return fmt.Errorf("promoter: update comparison item: %w", err)
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
// Extraction -> comparison_items params
// ---------------------------------------------------------------------------

// buildCostOfLivingCreateParams maps a validated extraction to a
// CreateComparisonItemIntelParams. The title is the city name; cost_data
// maps to intel_cost_data.
//
// TODO(follow-up): resolve the cost_of_living comparison type id from the
// comparison_type_definitions table. For now type_id is a placeholder
// pgtype.UUID{}; a follow-up will query the table and stamp the resolved
// id so the comparison feed can filter by type.
func buildCostOfLivingCreateParams(record gen.IntelRecord, source gen.IntelSource, ext extractor.Extraction, tenantID pgtype.UUID) gen.CreateComparisonItemIntelParams {
	title := stringField(ext, "city")
	if title == "" {
		title = source.Name
	}
	return gen.CreateComparisonItemIntelParams{
		TenantID:      tenantID,
		Title:         title,
		GeoSpaceID:    pgtype.UUID{}, // resolved by the promoter from the country space
		TypeID:        pgtype.UUID{}, // placeholder — follow-up resolves from comparison_type_definitions
		Position:      0,
		IntelRecordID: record.ID,
		IntelCostData: jsonbField(ext, "cost_data"),
		// spec §530: create path stamps 'verified' + 'ingestion'. The
		// comparison_items CHECK constraint (migration 000447) only allows
		// verification_status IN ('verified','needs_review','conflict','stale')
		// and override_source IN ('ingestion','editorial','mixed'); the prior
		// 'auto' values violated the CHECK and rejected every insert.
		ConfidenceOverall:  record.ConfidenceOverall,
		VerificationStatus: "verified",
		FreshnessScore:     freshScore,
		OverrideSource:     "ingestion",
		EditorialOverrides: emptyJSONObject(),
	}
}

// buildCostOfLivingUpdateParams produces an UpdateComparisonItemIntelParams
// seeded from the existing row, then applies each extraction field that is
// NOT in overrides and whose value actually changed. It returns the params
// and the list of changed column names. Fields in editorial_overrides keep
// the existing value (human edit wins).
//
// Only intel_cost_data is extraction-driven on the update path; the
// provenance columns (confidence_overall, verification_status,
// override_source, editorial_overrides) are re-stamped from the existing
// row so the UPDATE refreshes last_updated_at. freshness_score is set by
// the caller before the write.
func buildCostOfLivingUpdateParams(existing gen.ComparisonItem, ext extractor.Extraction, overrides map[string]struct{}, tenantID pgtype.UUID) (gen.UpdateComparisonItemIntelParams, []string) {
	params := gen.UpdateComparisonItemIntelParams{
		TenantID:           tenantID,
		ID:                 existing.ID,
		IntelCostData:      existing.IntelCostData,
		ConfidenceOverall:  existing.ConfidenceOverall,
		VerificationStatus: existing.VerificationStatus,
		FreshnessScore:     existing.FreshnessScore,
		OverrideSource:     existing.OverrideSource,
		EditorialOverrides: existing.EditorialOverrides,
	}

	var changed []string

	// intel_cost_data (extraction key: cost_data)
	if _, ov := overrides["intel_cost_data"]; !ov {
		if v := jsonbField(ext, "cost_data"); !bytesEqual(v, existing.IntelCostData) {
			params.IntelCostData = v
			changed = append(changed, "intel_cost_data")
		}
	}

	return params, changed
}

// marshalCostOfLivingFieldChanges serializes the before/after values of the
// changed columns into the field_changes JSON array stored in the
// intel_review_log row.
func marshalCostOfLivingFieldChanges(existing gen.ComparisonItem, params gen.UpdateComparisonItemIntelParams, changed []string) ([]byte, error) {
	changes := make([]fieldChange, 0, len(changed))
	for _, c := range changed {
		fc := fieldChange{Field: c}
		switch c {
		case "intel_cost_data":
			fc.From, fc.To = jsonOrNull(existing.IntelCostData), jsonOrNull(params.IntelCostData)
		}
		changes = append(changes, fc)
	}
	return json.Marshal(changes)
}
