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

// RemittanceStore is the persistence surface the RemittancePromoter needs.
// *stores.Store satisfies this interface; tests substitute a mock.
//
// Unlike the scholarship/job promoters, the remittance promoter does NOT
// create a contents guide row — it only writes the remittance_rates
// projection row (ALTERed with intel columns by migration 000446). There is
// therefore no authorID on the promoter and no CreateContent in the store.
//
// WithTx runs a callback inside a transaction. The callback receives a
// RemittanceTxStore — a narrower interface exposing only the write methods
// needed inside the transaction — so createPath and updatePath perform their
// multi-table writes atomically (BUG-01/02 fix, §6.2 transaction boundary).
type RemittanceStore interface {
	GetIntelRecord(ctx context.Context, tenantID, id pgtype.UUID) (gen.IntelRecord, error)
	GetIntelSource(ctx context.Context, tenantID, id pgtype.UUID) (gen.IntelSource, error)
	GetRemittanceRateByIntelRecord(ctx context.Context, tenantID, intelRecordID pgtype.UUID) (gen.RemittanceRate, error)
	CreateRemittanceRateIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateRemittanceRateIntelParams) (gen.RemittanceRate, error)
	UpdateRemittanceRateIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateRemittanceRateIntelParams) error
	UpdateIntelRecordPromoted(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelRecordPromotedParams) error
	CreateIntelReviewLog(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateIntelReviewLogParams) error
	WithTx(ctx context.Context, fn func(RemittanceTxStore) error) error
}

// RemittanceTxStore is the subset of RemittanceStore needed inside a
// transaction. It exposes only the write methods that createPath and
// updatePath call, so the transactional store can be a narrower type.
// *stores.Store satisfies this interface (via WithTx's callback), and the
// test mock satisfies it directly.
type RemittanceTxStore interface {
	CreateRemittanceRateIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateRemittanceRateIntelParams) (gen.RemittanceRate, error)
	UpdateIntelRecordPromoted(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelRecordPromotedParams) error
	UpdateRemittanceRateIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateRemittanceRateIntelParams) error
	CreateIntelReviewLog(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateIntelReviewLogParams) error
}

// RemittancePromoter promotes remittance intel_records into the
// remittance_rates projection table.
//
// Unlike scholarships/jobs, remittance does NOT create a contents guide row —
// it only creates/updates the remittance_rates row. There is therefore no
// authorID on the promoter.
//
// It implements the editorial-override-respect invariant: on the update path,
// any field present in remittance_rates.editorial_overrides is left untouched
// so a human edit always wins over a re-crawl.
//
// The promoter needs to resolve provider_id from the extracted provider_name.
// For now, provider_id is stamped with pgtype.UUID{} (the zero/invalid uuid);
// the moderation surface or a follow-up worker resolves the provider. The
// remittance_rates.provider_id column is NOT NULL, so callers must ensure a
// provider is associated before promotion reaches this promoter in production
// — the placeholder is a transitional measure tracked by the remittance
// provider-resolution follow-up.
type RemittancePromoter struct {
	store    RemittanceStore
	tenantID pgtype.UUID // tenant the projection rows are stamped with
	now      func() time.Time
}

// RemittancePromoterOption configures a RemittancePromoter.
type RemittancePromoterOption func(*RemittancePromoter)

// WithRemittanceClock overrides the clock used for timestamps (tests).
func WithRemittanceClock(fn func() time.Time) RemittancePromoterOption {
	return func(p *RemittancePromoter) { p.now = fn }
}

// NewRemittancePromoter creates a promoter backed by the given store. The
// tenantID identifies the tenant stamped onto the remittance_rates rows
// created by the create path. No authorID is needed because remittance does
// not create a contents guide row.
func NewRemittancePromoter(store RemittanceStore, tenantID pgtype.UUID, opts ...RemittancePromoterOption) *RemittancePromoter {
	p := &RemittancePromoter{store: store, tenantID: tenantID, now: time.Now}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Entity returns the entity_type this promoter handles.
func (p *RemittancePromoter) Entity() string { return "remittance" }

// ErrNotRemittance is returned when Promote is called on an intel record whose
// entity_type is not "remittance".
var ErrNotRemittance = fmt.Errorf("promoter: intel record is not a remittance")

// Promote maps the intel record identified by recordID to a remittance_rates
// projection row, creating it or updating it in place while respecting
// editorial overrides.
func (p *RemittancePromoter) Promote(ctx context.Context, recordID uuid.UUID) (PromoteResult, error) {
	pgID := uuidToPg(recordID)

	record, err := p.store.GetIntelRecord(ctx, p.tenantID, pgID)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: load intel record: %w", err)
	}
	if record.EntityType != p.Entity() {
		return PromoteResult{}, ErrNotRemittance
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
	existing, err := p.store.GetRemittanceRateByIntelRecord(ctx, p.tenantID, pgID)
	if err != nil {
		if isNotFound(err) {
			return p.remittanceCreatePath(ctx, record, source, ext)
		}
		return PromoteResult{}, fmt.Errorf("promoter: load existing remittance rate: %w", err)
	}

	return p.remittanceUpdatePath(ctx, record, existing, ext)
}

// remittanceCreatePath inserts the remittance_rates projection row and flips
// the intel record to 'published' with the promoted row id. Both writes run
// inside a single transaction so a failure in either rolls back the other
// (BUG-01 fix, §6.2).
//
// provider_id is stamped with the zero/invalid uuid for now; the moderation
// surface or a follow-up provider-resolution worker resolves the provider.
// The rate, fee_flat, fee_pct, transfer_time, and source_url come from the
// extraction; send_currency/receive_currency come from the extraction too.
func (p *RemittancePromoter) remittanceCreatePath(ctx context.Context, record gen.IntelRecord, source gen.IntelSource, ext extractor.Extraction) (PromoteResult, error) {
	params := buildRemittanceCreateParams(record, source, ext)

	var result PromoteResult
	err := p.store.WithTx(ctx, func(tx RemittanceTxStore) error {
		rate, err := tx.CreateRemittanceRateIntel(ctx, p.tenantID, params)
		if err != nil {
			return fmt.Errorf("promoter: create remittance rate: %w", err)
		}
		if err := tx.UpdateIntelRecordPromoted(ctx, p.tenantID, gen.UpdateIntelRecordPromotedParams{
			ID:               record.ID,
			PromotedRecordID: rate.ID,
		}); err != nil {
			return fmt.Errorf("promoter: mark intel record promoted: %w", err)
		}
		result = PromoteResult{
			DomainID: pgToUUID(rate.ID),
			Action:   ActionCreated,
		}
		return nil
	})
	return result, err
}

// remittanceUpdatePath applies the new extraction to the existing
// remittance_rates row, skipping any field present in editorial_overrides,
// and records an auto_update review-log entry when fields actually change.
// The remittance update and the review log write run inside a single
// transaction so the audit trail is always consistent with the projection
// state (BUG-02 fix, §6.2).
func (p *RemittancePromoter) remittanceUpdatePath(ctx context.Context, record gen.IntelRecord, existing gen.RemittanceRate, ext extractor.Extraction) (PromoteResult, error) {
	overrides, err := parseOverrideKeys(existing.EditorialOverrides)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: parse editorial overrides: %w", err)
	}

	params, changed := buildRemittanceUpdateParams(existing, ext, overrides)

	if len(changed) == 0 {
		// No field-level changes: skip the auto_update log, just bump
		// fetched_at + freshness_score by re-stamping the row with its
		// current values at full freshness.
		params.FreshnessScore = freshScore
		if err := p.store.WithTx(ctx, func(tx RemittanceTxStore) error {
			return tx.UpdateRemittanceRateIntel(ctx, p.tenantID, params)
		}); err != nil {
			return PromoteResult{}, fmt.Errorf("promoter: bump remittance rate freshness: %w", err)
		}
		return PromoteResult{
			DomainID: pgToUUID(existing.ID),
			Action:   ActionVerified,
		}, nil
	}

	params.FreshnessScore = freshScore
	changesJSON, err := marshalRemittanceFieldChanges(existing, params, changed)
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
	err = p.store.WithTx(ctx, func(tx RemittanceTxStore) error {
		if err := tx.UpdateRemittanceRateIntel(ctx, p.tenantID, params); err != nil {
			return fmt.Errorf("promoter: update remittance rate: %w", err)
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
// Extraction -> remittance params
// ---------------------------------------------------------------------------

// buildRemittanceCreateParams maps a validated extraction to a
// CreateRemittanceRateIntelParams. provider_id is the zero/invalid uuid
// placeholder (see RemittancePromoter doc comment); the moderation surface or
// a follow-up provider-resolution worker resolves the provider.
func buildRemittanceCreateParams(record gen.IntelRecord, source gen.IntelSource, ext extractor.Extraction) gen.CreateRemittanceRateIntelParams {
	return gen.CreateRemittanceRateIntelParams{
		IntelRecordID: record.ID,
		// provider_name is extracted but not stored directly on the row —
		// the provider_id FK is the canonical reference. Until provider
		// resolution is wired in, stamp the zero/invalid uuid.
		ProviderID:         pgtype.UUID{},
		SendCurrency:       stringField(ext, "send_currency"),
		ReceiveCurrency:    stringField(ext, "receive_currency"),
		Rate:               numericField(ext, "rate"),
		FeeFlat:            numericField(ext, "fee_flat"),
		FeePct:             numericField(ext, "fee_pct"),
		TransferTime:       stringPtrField(ext, "transfer_time"),
		SourceUrl:          sourceURLForRemittance(ext, source),
		ConfidenceOverall:  record.ConfidenceOverall,
		VerificationStatus: "verified",
		FreshnessScore:     freshScore,
		OverrideSource:     "ingestion",
		EditorialOverrides: emptyJSONObject(),
	}
}

// buildRemittanceUpdateParams produces an UpdateRemittanceRateIntelParams
// seeded from the existing row, then applies each extraction field that is
// NOT in overrides and whose value actually changed. It returns the params
// and the list of changed column names. Fields in editorial_overrides keep
// the existing value (human edit wins).
//
// Only rate, fee_flat, fee_pct, and transfer_time are updatable via the
// UpdateRemittanceRateIntel query — send_currency, receive_currency,
// provider_id, and source_url are immutable after creation (the query does
// not touch them), so they are not considered here.
func buildRemittanceUpdateParams(existing gen.RemittanceRate, ext extractor.Extraction, overrides map[string]struct{}) (gen.UpdateRemittanceRateIntelParams, []string) {
	params := gen.UpdateRemittanceRateIntelParams{
		ID:                 existing.ID,
		Rate:               existing.Rate,
		FeeFlat:            existing.FeeFlat,
		FeePct:             existing.FeePct,
		TransferTime:       existing.TransferTime,
		ConfidenceOverall:  existing.ConfidenceOverall,
		VerificationStatus: existing.VerificationStatus,
		FreshnessScore:     existing.FreshnessScore,
		OverrideSource:     existing.OverrideSource,
		EditorialOverrides: existing.EditorialOverrides,
	}

	var changed []string

	// rate
	if _, ov := overrides["rate"]; !ov {
		if v := numericField(ext, "rate"); v.Valid && !numericEqual(v, existing.Rate) {
			params.Rate = v
			changed = append(changed, "rate")
		}
	}
	// fee_flat
	if _, ov := overrides["fee_flat"]; !ov {
		if v := numericField(ext, "fee_flat"); v.Valid && !numericEqual(v, existing.FeeFlat) {
			params.FeeFlat = v
			changed = append(changed, "fee_flat")
		}
	}
	// fee_pct
	if _, ov := overrides["fee_pct"]; !ov {
		if v := numericField(ext, "fee_pct"); v.Valid && !numericEqual(v, existing.FeePct) {
			params.FeePct = v
			changed = append(changed, "fee_pct")
		}
	}
	// transfer_time
	if _, ov := overrides["transfer_time"]; !ov {
		if v := stringPtrField(ext, "transfer_time"); !strPtrEqual(v, existing.TransferTime) {
			params.TransferTime = v
			changed = append(changed, "transfer_time")
		}
	}

	return params, changed
}

// marshalRemittanceFieldChanges builds the intel_review_log.field_changes JSON
// array for the changed remittance fields.
func marshalRemittanceFieldChanges(existing gen.RemittanceRate, params gen.UpdateRemittanceRateIntelParams, changed []string) ([]byte, error) {
	changes := make([]fieldChange, 0, len(changed))
	for _, c := range changed {
		fc := fieldChange{Field: c}
		switch c {
		case "rate":
			fc.From, fc.To = numericStr(existing.Rate), numericStr(params.Rate)
		case "fee_flat":
			fc.From, fc.To = numericStr(existing.FeeFlat), numericStr(params.FeeFlat)
		case "fee_pct":
			fc.From, fc.To = numericStr(existing.FeePct), numericStr(params.FeePct)
		case "transfer_time":
			fc.From, fc.To = derefStr(existing.TransferTime), derefStr(params.TransferTime)
		}
		changes = append(changes, fc)
	}
	return json.Marshal(changes)
}

// sourceURLForRemittance prefers the extraction's source_url and falls back to
// the intel source's canonical URL.
func sourceURLForRemittance(ext extractor.Extraction, source gen.IntelSource) *string {
	if v := stringPtrField(ext, "source_url"); v != nil {
		return v
	}
	if source.SourceUrl != "" {
		return ptr(source.SourceUrl)
	}
	return nil
}
