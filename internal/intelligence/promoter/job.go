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

// JobStore is the persistence surface the JobPromoter needs.
// *stores.Store satisfies this interface; tests substitute a mock.
type JobStore interface {
	PersonaResolver
	GetIntelRecord(ctx context.Context, tenantID, id pgtype.UUID) (gen.IntelRecord, error)
	GetIntelSource(ctx context.Context, tenantID, id pgtype.UUID) (gen.IntelSource, error)
	GetIntelJobByIntelRecord(ctx context.Context, tenantID, intelRecordID pgtype.UUID) (gen.IntelJob, error)
	CreateIntelJob(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateIntelJobParams) (gen.IntelJob, error)
	UpdateIntelJob(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelJobParams) error
	UpdateIntelRecordStatus(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelRecordStatusParams) error
	CreateIntelReviewLog(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateIntelReviewLogParams) error
	GetCountrySpace(ctx context.Context, tenantID pgtype.UUID, countryCode string) (pgtype.UUID, error)
	WithTx(ctx context.Context, fn func(JobTxStore) error) error
}

// JobTxStore is the subset of JobStore needed inside a transaction.
type JobTxStore interface {
	CreateIntelJob(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateIntelJobParams) (gen.IntelJob, error)
	UpdateIntelRecordStatus(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelRecordStatusParams) error
	UpdateIntelJob(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelJobParams) error
	CreateIntelReviewLog(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateIntelReviewLogParams) error
}

// JobPromoter promotes job intel_records into the intel_jobs projection
// table. It implements the editorial-override invariant: on the update
// path, any field in editorial_overrides is left untouched so a human edit
// always wins over a re-crawl. The contents guide row is NO LONGER created
// here — Studio generates it from the extraction data via intel_record_id
// (ADR-076 Amendment 1).
type JobPromoter struct {
	store    JobStore
	tenantID pgtype.UUID
	authorID pgtype.UUID
	aiModel  *string // AI model used for extraction (ADR-088, INTEL-003)
	now      func() time.Time
}

// JobPromoterOption configures a JobPromoter.
type JobPromoterOption func(*JobPromoter)

// WithJobClock overrides the clock used for publish timestamps (tests).
func WithJobClock(fn func() time.Time) JobPromoterOption {
	return func(p *JobPromoter) { p.now = fn }
}

// WithJobAIModel sets the AI model name stamped on created content rows (ADR-088).
func WithJobAIModel(model string) JobPromoterOption {
	return func(p *JobPromoter) { p.aiModel = &model }
}

// NewJobPromoter creates a promoter backed by the given store.
func NewJobPromoter(store JobStore, tenantID, authorID pgtype.UUID, opts ...JobPromoterOption) *JobPromoter {
	p := &JobPromoter{store: store, tenantID: tenantID, authorID: authorID, aiModel: ptr("gpt-4o-mini"), now: time.Now}
	for _, opt := range opts {
		opt(p)
	}

	return p
}

// Entity is an exported method.
func (p *JobPromoter) Entity() string { return "job" }

// ErrNotJob is returned when Promote is called on a non-job intel record.
var ErrNotJob = errors.New("promoter: intel record is not a job")

// Promote maps the intel record identified by recordID to an intel_jobs
// projection row, creating or updating it while respecting editorial
// overrides.
func (p *JobPromoter) Promote(ctx context.Context, recordID uuid.UUID) (PromoteResult, error) {
	return promoteDomain(ctx, p.store, p.tenantID, recordID, domainPromoteOps[gen.IntelJob]{
		entityType: p.Entity(),
		notErr:     ErrNotJob,
		loadLabel:  "job",
		getExisting: func(ctx context.Context, tenantID, pgID pgtype.UUID) (gen.IntelJob, error) {
			return p.store.GetIntelJobByIntelRecord(ctx, tenantID, pgID)
		},
		createPath: p.jobCreatePath,
		updatePath: p.jobUpdatePath,
	})
}

// jobCreatePath inserts the intel_jobs projection row and flips the intel
// record to 'extracted' status. The contents guide row is NO LONGER created
// here — Studio generates it from the extraction data via intel_record_id
// (ADR-076 Amendment 1).
func (p *JobPromoter) jobCreatePath(ctx context.Context, record gen.IntelRecord, source gen.IntelSource, ext extractor.Extraction) (PromoteResult, error) {
	countryCode := firstCountry(ext, "country")

	var countrySpaceID pgtype.UUID

	if countryCode != "" {
		if id, err := p.store.GetCountrySpace(ctx, p.tenantID, countryCode); err == nil && id.Valid {
			countrySpaceID = id
		}
	}

	jobParams := buildJobCreateParams(record, source, pgtype.UUID{}, ext) // guide_id NULL — Studio fills it later
	jobParams.CountrySpaceID = countrySpaceID

	var result PromoteResult

	err := p.store.WithTx(ctx, func(tx JobTxStore) error {
		job, err := tx.CreateIntelJob(ctx, p.tenantID, jobParams)
		if err != nil {
			return fmt.Errorf("promoter: create job: %w", err)
		}

		if err := tx.UpdateIntelRecordStatus(ctx, p.tenantID, gen.UpdateIntelRecordStatusParams{
			ID:     record.ID,
			Status: "extracted",
		}); err != nil {
			return fmt.Errorf("promoter: mark intel record extracted: %w", err)
		}

		result = PromoteResult{DomainID: pgToUUID(job.ID), ContentID: uuid.Nil, Action: ActionCreated}

		return nil
	})

	return result, err
}

// jobUpdatePath applies the new extraction to the existing intel_jobs row,
// skipping any field present in editorial_overrides, and records an
// auto_update review-log entry when fields actually change. ContentID is
// derived from existing.GuideID, which may be NULL (uuid.Nil) when Studio
// has not yet generated the editorial guide row (ADR-076 Amendment 1).
func (p *JobPromoter) jobUpdatePath(ctx context.Context, record gen.IntelRecord, existing gen.IntelJob, ext extractor.Extraction) (PromoteResult, error) {
	overrides, err := parseOverrideKeys(existing.EditorialOverrides)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: parse editorial overrides: %w", err)
	}

	params, changed := buildJobUpdateParams(existing, ext, overrides)
	params.FreshnessScore = freshScore

	var changesJSON []byte

	if len(changed) > 0 {
		if changesJSON, err = marshalJobFieldChanges(existing, params, changed); err != nil {
			return PromoteResult{}, fmt.Errorf("promoter: marshal field changes: %w", err)
		}
	}

	return domainUpdateTx(ctx, record, p.authorID, p.tenantID, "job",
		pgToUUID(existing.ID), pgToUUID(existing.GuideID), params, changed, changesJSON,
		func(ctx context.Context, fn func(JobTxStore) error) error { return p.store.WithTx(ctx, fn) },
		JobTxStore.UpdateIntelJob)
}

func buildJobCreateParams(record gen.IntelRecord, source gen.IntelSource, guideID pgtype.UUID, ext extractor.Extraction) gen.CreateIntelJobParams {
	return gen.CreateIntelJobParams{
		IntelRecordID:      record.ID,
		GuideID:            guideID,
		SourceUrl:          source.SourceUrl,
		SourceType:         source.SourceType,
		TrustLevel:         source.TrustLevel,
		Title:              stringField(ext, "title"),
		Description:        stringPtrField(ext, "description_summary"),
		Country:            firstCountry(ext, "country"),
		CountrySpaceID:     pgtype.UUID{},
		City:               stringPtrField(ext, "city"),
		Employer:           stringPtrField(ext, "employer"),
		JobType:            stringPtrField(ext, "job_type"),
		ExperienceLevel:    stringPtrField(ext, "experience_level"),
		SalaryMin:          numericField(ext, "salary_min"),
		SalaryMax:          numericField(ext, "salary_max"),
		Currency:           stringPtrField(ext, "currency"),
		Skills:             stringSliceField(ext, "skills"),
		ApplicationUrl:     stringPtrField(ext, "application_url"),
		Deadline:           dateField(ext, "deadline"),
		Remote:             stringPtrField(ext, "remote"),
		ConfidenceOverall:  record.ConfidenceOverall,
		VerificationStatus: "verified",
		FreshnessScore:     freshScore,
		OverrideSource:     "ingestion",
		EditorialOverrides: emptyJSONObject(),
	}
}

func buildJobUpdateParams(existing gen.IntelJob, ext extractor.Extraction, overrides map[string]struct{}) (gen.UpdateIntelJobParams, []string) {
	params := gen.UpdateIntelJobParams{
		ID: existing.ID, Title: existing.Title, Description: existing.Description,
		Country: existing.Country, CountrySpaceID: existing.CountrySpaceID,
		City: existing.City, Employer: existing.Employer, JobType: existing.JobType,
		ExperienceLevel: existing.ExperienceLevel, SalaryMin: existing.SalaryMin,
		SalaryMax: existing.SalaryMax, Currency: existing.Currency, Skills: existing.Skills,
		ApplicationUrl: existing.ApplicationUrl, Deadline: existing.Deadline, Remote: existing.Remote,
		ConfidenceOverall: existing.ConfidenceOverall, VerificationStatus: existing.VerificationStatus,
		FreshnessScore: existing.FreshnessScore, OverrideSource: existing.OverrideSource,
		EditorialOverrides: existing.EditorialOverrides,
	}

	var changed []string

	applyStrField(overrides, ext, "title", &params.Title, existing.Title, &changed)

	applyStrPtrFieldFn(overrides, "description", func() *string { return stringPtrField(ext, "description_summary") }, &params.Description, existing.Description, &changed)

	applyStrFieldFn(overrides, "country", func() string { return firstCountry(ext, "country") }, &params.Country, existing.Country, &changed)

	applyStrPtrField(overrides, ext, "city", &params.City, existing.City, &changed)

	applyStrPtrField(overrides, ext, "employer", &params.Employer, existing.Employer, &changed)

	applyStrPtrField(overrides, ext, "job_type", &params.JobType, existing.JobType, &changed)

	applyStrPtrField(overrides, ext, "experience_level", &params.ExperienceLevel, existing.ExperienceLevel, &changed)

	applyNumericField(overrides, ext, "salary_min", &params.SalaryMin, existing.SalaryMin, &changed)

	applyNumericField(overrides, ext, "salary_max", &params.SalaryMax, existing.SalaryMax, &changed)

	applyStrPtrField(overrides, ext, "currency", &params.Currency, existing.Currency, &changed)

	applyStrSliceField(overrides, ext, "skills", &params.Skills, existing.Skills, &changed)

	applyStrPtrField(overrides, ext, "application_url", &params.ApplicationUrl, existing.ApplicationUrl, &changed)

	applyDateField(overrides, ext, "deadline", &params.Deadline, existing.Deadline, &changed)

	applyStrPtrField(overrides, ext, "remote", &params.Remote, existing.Remote, &changed)

	return params, changed
}

// jobFieldAccessors maps each changeable intel_jobs field to a function that
// returns its (from, to) values for the auto_update review log.
var jobFieldAccessors = map[string]func(e gen.IntelJob, p gen.UpdateIntelJobParams) (any, any){
	"title": func(e gen.IntelJob, p gen.UpdateIntelJobParams) (any, any) { return e.Title, p.Title },
	"description": func(e gen.IntelJob, p gen.UpdateIntelJobParams) (any, any) {
		return derefStr(e.Description), derefStr(p.Description)
	},
	"country": func(e gen.IntelJob, p gen.UpdateIntelJobParams) (any, any) { return e.Country, p.Country },
	"city":    func(e gen.IntelJob, p gen.UpdateIntelJobParams) (any, any) { return derefStr(e.City), derefStr(p.City) },
	"employer": func(e gen.IntelJob, p gen.UpdateIntelJobParams) (any, any) {
		return derefStr(e.Employer), derefStr(p.Employer)
	},
	"job_type": func(e gen.IntelJob, p gen.UpdateIntelJobParams) (any, any) {
		return derefStr(e.JobType), derefStr(p.JobType)
	},
	"experience_level": func(e gen.IntelJob, p gen.UpdateIntelJobParams) (any, any) {
		return derefStr(e.ExperienceLevel), derefStr(p.ExperienceLevel)
	},
	"salary_min": func(e gen.IntelJob, p gen.UpdateIntelJobParams) (any, any) {
		return numericStr(e.SalaryMin), numericStr(p.SalaryMin)
	},
	"salary_max": func(e gen.IntelJob, p gen.UpdateIntelJobParams) (any, any) {
		return numericStr(e.SalaryMax), numericStr(p.SalaryMax)
	},
	"currency": func(e gen.IntelJob, p gen.UpdateIntelJobParams) (any, any) {
		return derefStr(e.Currency), derefStr(p.Currency)
	},
	"skills": func(e gen.IntelJob, p gen.UpdateIntelJobParams) (any, any) { return e.Skills, p.Skills },
	"application_url": func(e gen.IntelJob, p gen.UpdateIntelJobParams) (any, any) {
		return derefStr(e.ApplicationUrl), derefStr(p.ApplicationUrl)
	},
	"deadline": func(e gen.IntelJob, p gen.UpdateIntelJobParams) (any, any) {
		return dateStr(e.Deadline), dateStr(p.Deadline)
	},
	"remote": func(e gen.IntelJob, p gen.UpdateIntelJobParams) (any, any) {
		return derefStr(e.Remote), derefStr(p.Remote)
	},
}

func marshalJobFieldChanges(existing gen.IntelJob, params gen.UpdateIntelJobParams, changed []string) ([]byte, error) {
	changes := make([]fieldChange, 0, len(changed))

	for _, c := range changed {
		fc := fieldChange{Field: c}

		if get, ok := jobFieldAccessors[c]; ok {
			fc.From, fc.To = get(existing, params)
		}

		changes = append(changes, fc)
	}

	return json.Marshal(changes)
}
