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

// RemittanceStore is the persistence surface the RemittancePromoter needs.
// *stores.Store satisfies this interface; tests substitute a mock.
//
// Under Approach B (ADR-076 Amendment 1), the remittance promoter does NOT
// create a contents guide row — it only writes the remittance_rates
// projection row (ALTERed with intel columns by migration 000446). After
// successful projection, the intel record is marked status='extracted' so
// the admin can promote it to Studio for editorial generation.
//
// WithTx runs a callback inside a transaction. The callback receives a
// RemittanceTxStore — a narrower interface exposing only the write methods
// needed inside the transaction — so createPath and updatePath perform their
// multi-table writes atomically (BUG-01/02 fix, §6.2 transaction boundary).
type RemittanceStore interface {
	GetIntelRecord(ctx context.Context, id pgtype.UUID) (gen.IntelRecord, error)
	GetIntelSource(ctx context.Context, id pgtype.UUID) (gen.IntelSource, error)
	GetRemittanceRateByIntelRecord(ctx context.Context, intelRecordID pgtype.UUID) (gen.RemittanceRate, error)
	CreateRemittanceRateIntel(ctx context.Context, arg gen.CreateRemittanceRateIntelParams) (gen.RemittanceRate, error)
	UpdateRemittanceRateIntel(ctx context.Context, arg gen.UpdateRemittanceRateIntelParams) error
	UpdateIntelRecordStatus(ctx context.Context, arg gen.UpdateIntelRecordStatusParams) error
	CreateIntelReviewLog(ctx context.Context, arg gen.CreateIntelReviewLogParams) error
	WithTx(ctx context.Context, fn func(RemittanceTxStore) error) error
}

// RemittanceTxStore is the subset of RemittanceStore needed inside a
// transaction. It exposes only the write methods that createPath and
// updatePath call, so the transactional store can be a narrower type.
// *stores.Store satisfies this interface (via WithTx's callback), and the
// test mock satisfies it directly.
type RemittanceTxStore interface {
	CreateRemittanceRateIntel(ctx context.Context, arg gen.CreateRemittanceRateIntelParams) (gen.RemittanceRate, error)
	UpdateIntelRecordStatus(ctx context.Context, arg gen.UpdateIntelRecordStatusParams) error
	UpdateRemittanceRateIntel(ctx context.Context, arg gen.UpdateRemittanceRateIntelParams) error
	CreateIntelReviewLog(ctx context.Context, arg gen.CreateIntelReviewLogParams) error
}

// RemittancePromoter promotes remittance intel_records into the
// remittance_rates projection table.
//
// Under Approach B (ADR-076 Amendment 1), remittance does NOT create a
// contents guide row — it only creates/updates the remittance_rates row
// and marks the intel record status='extracted'. Studio generates the
// editorial remittance guide when the admin promotes the record.
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
	authorID pgtype.UUID // system/automation user for review-log actor
	now      func() time.Time
}

// RemittancePromoterOption configures a RemittancePromoter.
type RemittancePromoterOption func(*RemittancePromoter)

// WithRemittanceClock overrides the clock used for timestamps (tests).
func WithRemittanceClock(fn func() time.Time) RemittancePromoterOption {
	return func(p *RemittancePromoter) { p.now = fn }
}

// NewRemittancePromoter creates a promoter backed by the given store. The
// ID identifies the tenant stamped onto the remittance_rates rows
// created by the create path. No authorID is needed because remittance does
// not create a contents guide row.
func NewRemittancePromoter(store RemittanceStore, authorID pgtype.UUID, opts ...RemittancePromoterOption) *RemittancePromoter {
	p := &RemittancePromoter{store: store, authorID: authorID, now: time.Now}
	for _, opt := range opts {
		opt(p)
	}

	return p
}

// Entity returns the entity_type this promoter handles.
func (p *RemittancePromoter) Entity() string { return "remittance" }

// ErrNotRemittance is returned when Promote is called on an intel record whose
// entity_type is not "remittance".
var ErrNotRemittance = errors.New("promoter: intel record is not a remittance")

// Promote maps the intel record identified by recordID to a remittance_rates
// projection row, creating it or updating it in place while respecting
// editorial overrides. Under Approach B, no contents guide row is created
// — the promoter only writes the remittance_rates projection and marks
// the intel record status='extracted'. Studio generates the editorial
// remittance guide when the admin promotes the record.
func (p *RemittancePromoter) Promote(ctx context.Context, recordID uuid.UUID) (PromoteResult, error) {
	return promoteDomain(ctx, p.store, recordID, domainPromoteOps[gen.RemittanceRate]{
		entityType: p.Entity(),
		notErr:     ErrNotRemittance,
		loadLabel:  "remittance rate",
		getExisting: func(ctx context.Context, pgID pgtype.UUID) (gen.RemittanceRate, error) {
			return p.store.GetRemittanceRateByIntelRecord(ctx, pgID)
		},
		createPath: p.remittanceCreatePath,
		updatePath: p.remittanceUpdatePath,
	})
}

// remittanceCreatePath inserts the remittance_rates projection row and
// marks the intel record status='extracted' (Approach B). Both writes run
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
		rate, err := tx.CreateRemittanceRateIntel(ctx, params)
		if err != nil {
			return fmt.Errorf("promoter: create remittance rate: %w", err)
		}

		if err := tx.UpdateIntelRecordStatus(ctx, gen.UpdateIntelRecordStatusParams{

			ID:     record.ID,
			Status: "extracted",
		}); err != nil {
			return fmt.Errorf("promoter: mark intel record extracted: %w", err)
		}

		result = PromoteResult{
			DomainID: pgToUUID(rate.ID),
			Action:   ActionCreated,
			// Approach B: no contents row created by the promoter.
			ContentID: uuid.Nil,
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
	params.FreshnessScore = freshScore

	var changesJSON []byte

	if len(changed) > 0 {
		if changesJSON, err = marshalRemittanceFieldChanges(existing, params, changed); err != nil {
			return PromoteResult{}, fmt.Errorf("promoter: marshal field changes: %w", err)
		}
	}

	return domainUpdateTx(ctx, record, p.authorID, "remittance rate",
		pgToUUID(existing.ID), uuid.Nil, params, changed, changesJSON,
		func(ctx context.Context, fn func(RemittanceTxStore) error) error { return p.store.WithTx(ctx, fn) },
		RemittanceTxStore.UpdateRemittanceRateIntel)
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
	applyNumericField(overrides, ext, "rate", &params.Rate, existing.Rate, &changed)
	// fee_flat
	applyNumericField(overrides, ext, "fee_flat", &params.FeeFlat, existing.FeeFlat, &changed)
	// fee_pct
	applyNumericField(overrides, ext, "fee_pct", &params.FeePct, existing.FeePct, &changed)
	// transfer_time
	applyStrPtrField(overrides, ext, "transfer_time", &params.TransferTime, existing.TransferTime, &changed)

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
