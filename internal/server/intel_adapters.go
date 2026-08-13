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
	intel_stores "github.com/jedafe/japa/internal/intelligence/stores"
	"github.com/jedafe/japa/internal/intelligence/sources"
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
// extractor → ai → auth → worker). The adapter selects the scholarship
// schema internally; future entity types will branch on sourceType here.
type intelExtractorPortAdapter struct {
	ext *extractor.Extractor
}

// Extract calls the underlying extractor with the scholarship schema and
// returns the marshalled extraction + aggregate confidence in the shape
// worker.IntelExtractResult expects. If the extractor returns
// ErrExtractionRetriesExhausted, the error is wrapped with the worker's
// port-level sentinel so HandleIntelExtract can detect it via errors.Is
// and route the record to REVIEW_REQUIRED instead of failing the task
// (BUG-16 fix).
func (a *intelExtractorPortAdapter) Extract(ctx context.Context, sourceURL, sourceType, sourceText string) (worker.IntelExtractResult, error) {
	result, err := a.ext.Extract(ctx, schemas.Scholarship, sourceURL, sourceType, sourceText)
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

// intelPromoterPortAdapter wraps *promoter.ScholarshipPromoter so it
// satisfies the worker.intelPromoterPort interface without the worker
// package importing internal/intelligence/promoter.
type intelPromoterPortAdapter struct {
	prom *promoter.ScholarshipPromoter
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
