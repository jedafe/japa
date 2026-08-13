package server

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/extractor"
	"github.com/jedafe/japa/internal/intelligence/extractor/schemas"
	"github.com/jedafe/japa/internal/intelligence/promoter"
	"github.com/jedafe/japa/internal/intelligence/sources"
	"github.com/jedafe/japa/internal/worker"
)

// intelSchedulerSourceAdapter wraps *sources.Store so it satisfies the
// scheduler.DueSourceLister interface. The sources store exposes ListDue
// (which wraps the gen.ListDueIntelSources query); the adapter renames it
// to the method the scheduler expects.
type intelSchedulerSourceAdapter struct {
	store *sources.Store
}

func (a *intelSchedulerSourceAdapter) ListDueIntelSources(ctx context.Context, limit int32) ([]gen.IntelSource, error) {
	return a.store.ListDue(ctx, limit)
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
// worker.IntelExtractResult expects.
func (a *intelExtractorPortAdapter) Extract(ctx context.Context, sourceURL, sourceType, sourceText string) (worker.IntelExtractResult, error) {
	result, err := a.ext.Extract(ctx, schemas.Scholarship, sourceURL, sourceType, sourceText)
	if err != nil {
		return worker.IntelExtractResult{}, err
	}
	raw, err := json.Marshal(result.Extraction)
	if err != nil {
		return worker.IntelExtractResult{}, err
	}
	return worker.IntelExtractResult{
		RawExtraction:      raw,
		ConfidenceOverall:  aggregateConfidence(result.Extraction),
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
