package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/extractor"
	"github.com/jedafe/japa/internal/intelligence/extractor/schemas"
	"github.com/jedafe/japa/internal/intelligence/promoter"
	"github.com/jedafe/japa/internal/intelligence/sources"
	intel_stores "github.com/jedafe/japa/internal/intelligence/stores"
	"github.com/jedafe/japa/internal/worker"
)

// intelSchedulerSourceAdapter wraps *sources.Store so it satisfies the
// scheduler.DueSourceLister interface. The sources store exposes ListDue
// (which wraps the gen.ListDueIntelSources query); the adapter passes the
// configured tenantID through. Per ADR-112 Amendment 2, all intelligence
// queries are tenant-scoped.
type intelSchedulerSourceAdapter struct {
	store    *sources.Store
	tenantID pgtype.UUID
}

func (a *intelSchedulerSourceAdapter) ListDue(ctx context.Context, tenantID pgtype.UUID, limit int32) ([]gen.IntelSource, error) {
	return a.store.ListDue(ctx, tenantID, limit)
}

// intelExtractorPortAdapter wraps *extractor.Extractor so it satisfies the
// worker.intelExtractorPort interface without the worker package importing
// internal/intelligence/extractor (which would close the cycle
// extractor → ai → auth → worker). The adapter looks up the entity schema
// by entityType from the schemas registry; the worker derives entityType
// from the source's category via schemas.CategoryToEntityType.
type intelExtractorPortAdapter struct {
	ext *extractor.Extractor
}

// Extract calls the underlying extractor with the schema for the given
// entityType and returns the marshalled extraction + aggregate confidence
// in the shape worker.IntelExtractResult expects. If the extractor
// returns ErrExtractionRetriesExhausted, the error is wrapped with the
// worker's port-level sentinel so HandleIntelExtract can detect it via
// errors.Is and route the record to REVIEW_REQUIRED instead of failing
// the task (BUG-16 fix).
func (a *intelExtractorPortAdapter) Extract(ctx context.Context, entityType, sourceURL, sourceType, sourceText string) (worker.IntelExtractResult, error) {
	schema, ok := schemas.ForEntityType(entityType)
	if !ok {
		return worker.IntelExtractResult{}, fmt.Errorf("intel: no schema registered for entity type %q", entityType)
	}
	result, err := a.ext.Extract(ctx, schema, sourceURL, sourceType, sourceText)
	if err != nil {
		if errors.Is(err, extractor.ErrExtractionRetriesExhausted) {
			return worker.IntelExtractResult{}, fmt.Errorf("%w: %v", worker.ErrIntelExtractionRetriesExhausted, err)
		}
		return worker.IntelExtractResult{}, err
	}
	raw, err := json.Marshal(result.Extraction)
	if err != nil {
		return worker.IntelExtractResult{}, err
	}
	return worker.IntelExtractResult{
		RawExtraction:     raw,
		ConfidenceOverall: aggregateConfidence(result.Extraction),
	}, nil
}

// intelPromoterPortAdapter wraps a promoter.Promoter so it satisfies the
// worker.intelPromoterPort interface without the worker package importing
// internal/intelligence/promoter. One adapter is registered per domain
// (scholarship, job, visa, housing, remittance, cost_of_living, and the
// 7 guide content types), keyed by Entity() in the promoter map.
type intelPromoterPortAdapter struct {
	prom promoter.Promoter
}

func (a *intelPromoterPortAdapter) Promote(ctx context.Context, recordID uuid.UUID) (worker.IntelPromoteResult, error) {
	result, err := a.prom.Promote(ctx, recordID)
	if err != nil {
		return worker.IntelPromoteResult{}, err
	}
	return worker.IntelPromoteResult{
		DomainID:      result.DomainID,
		ContentID:     result.ContentID,
		Action:        result.Action,
		ChangedFields: result.ChangedFields,
	}, nil
}

func (a *intelPromoterPortAdapter) Entity() string {
	return a.prom.Entity()
}

// aggregateConfidence computes the mean confidence across all extracted
// fields. The extractor returns per-field confidence values; the worker
// persists a single aggregate as intel_records.confidence_overall.
func aggregateConfidence(ext extractor.Extraction) float32 {
	if len(ext) == 0 {
		return 0
	}
	var sum float64
	for _, f := range ext {
		sum += f.Confidence
	}
	return float32(sum / float64(len(ext)))
}

// intelPromoterStoreAdapter wraps *stores.Store so it satisfies the
// promoter.Store interface, including the WithTx method whose callback
// type differs between the two packages. The embedded *stores.Store
// provides all CRUD methods via promotion; the adapter only overrides
// WithTx to bridge func(*stores.Store) error → func(promoter.TxStore) error
// (BUG-01/02 fix: §6.2 transaction boundary).
type intelPromoterStoreAdapter struct {
	*intel_stores.Store
}

func (a *intelPromoterStoreAdapter) WithTx(ctx context.Context, fn func(promoter.TxStore) error) error {
	return a.Store.WithTx(ctx, func(s *intel_stores.Store) error {
		return fn(s)
	})
}

// intelJobPromoterStoreAdapter wraps *stores.Store for the JobPromoter's
// JobStore interface. The WithTx bridges func(*stores.Store) error →
// func(promoter.JobTxStore) error.
type intelJobPromoterStoreAdapter struct {
	*intel_stores.Store
}

func (a *intelJobPromoterStoreAdapter) WithTx(ctx context.Context, fn func(promoter.JobTxStore) error) error {
	return a.Store.WithTx(ctx, func(s *intel_stores.Store) error {
		return fn(s)
	})
}

// intelVisaPromoterStoreAdapter wraps *stores.Store for the VisaPromoter's
// VisaStore interface.
type intelVisaPromoterStoreAdapter struct {
	*intel_stores.Store
}

func (a *intelVisaPromoterStoreAdapter) WithTx(ctx context.Context, fn func(promoter.VisaTxStore) error) error {
	return a.Store.WithTx(ctx, func(s *intel_stores.Store) error {
		return fn(s)
	})
}

// intelHousingPromoterStoreAdapter wraps *stores.Store for the
// HousingPromoter's HousingStore interface.
type intelHousingPromoterStoreAdapter struct {
	*intel_stores.Store
}

func (a *intelHousingPromoterStoreAdapter) WithTx(ctx context.Context, fn func(promoter.HousingTxStore) error) error {
	return a.Store.WithTx(ctx, func(s *intel_stores.Store) error {
		return fn(s)
	})
}

// intelRemittancePromoterStoreAdapter wraps *stores.Store for the
// RemittancePromoter's RemittanceStore interface.
type intelRemittancePromoterStoreAdapter struct {
	*intel_stores.Store
}

func (a *intelRemittancePromoterStoreAdapter) WithTx(ctx context.Context, fn func(promoter.RemittanceTxStore) error) error {
	return a.Store.WithTx(ctx, func(s *intel_stores.Store) error {
		return fn(s)
	})
}

// intelCostOfLivingPromoterStoreAdapter wraps *stores.Store for the
// CostOfLivingPromoter's CostOfLivingStore interface.
type intelCostOfLivingPromoterStoreAdapter struct {
	*intel_stores.Store
}

func (a *intelCostOfLivingPromoterStoreAdapter) WithTx(ctx context.Context, fn func(promoter.CostOfLivingTxStore) error) error {
	return a.Store.WithTx(ctx, func(s *intel_stores.Store) error {
		return fn(s)
	})
}

// intelGuidePromoterStoreAdapter wraps *stores.Store for the
// GuideContentPromoter's GuideContentStore interface (used by the 7 pure
// guide content type domains).
type intelGuidePromoterStoreAdapter struct {
	*intel_stores.Store
}

func (a *intelGuidePromoterStoreAdapter) WithTx(ctx context.Context, fn func(promoter.GuideContentTxStore) error) error {
	return a.Store.WithTx(ctx, func(s *intel_stores.Store) error {
		return fn(s)
	})
}
