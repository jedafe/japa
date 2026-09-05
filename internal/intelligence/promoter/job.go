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

func (p *JobPromoter) Entity() string { return "job" }

// ErrNotJob is returned when Promote is called on a non-job intel record.
var ErrNotJob = errors.New("promoter: intel record is not a job")

func (p *JobPromoter) Promote(ctx context.Context, recordID uuid.UUID) (PromoteResult, error) {
	pgID := uuidToPg(recordID)

	record, err := p.store.GetIntelRecord(ctx, p.tenantID, pgID)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: load intel record: %w", err)
	}

	if record.EntityType != p.Entity() {
		return PromoteResult{}, ErrNotJob
	}

	ext, err := parseExtraction(record.Extracted)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: parse extracted: %w", err)
	}

	source, err := p.store.GetIntelSource(ctx, p.tenantID, record.SourceID)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: load intel source: %w", err)
	}

	existing, err := p.store.GetIntelJobByIntelRecord(ctx, p.tenantID, pgID)
	if err != nil {
		if isNotFound(err) {
			return p.jobCreatePath(ctx, record, source, ext)
		}

		return PromoteResult{}, fmt.Errorf("promoter: load existing job: %w", err)
	}

	return p.jobUpdatePath(ctx, record, existing, ext)
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
	if len(changed) == 0 {
		params.FreshnessScore = freshScore

		if err := p.store.WithTx(ctx, func(tx JobTxStore) error {
			return tx.UpdateIntelJob(ctx, p.tenantID, params)
		}); err != nil {
			return PromoteResult{}, fmt.Errorf("promoter: bump job freshness: %w", err)
		}

		return PromoteResult{DomainID: pgToUUID(existing.ID), ContentID: pgToUUID(existing.GuideID), Action: ActionVerified}, nil
	}

	params.FreshnessScore = freshScore

	changesJSON, err := marshalJobFieldChanges(existing, params, changed)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: marshal field changes: %w", err)
	}

	note := "automatic re-crawl update"
	reviewLogParams := gen.CreateIntelReviewLogParams{IntelRecordID: record.ID, ActorUserID: p.authorID, Action: "auto_update", FieldChanges: changesJSON, Note: &note}

	var result PromoteResult

	err = p.store.WithTx(ctx, func(tx JobTxStore) error {
		if err := tx.UpdateIntelJob(ctx, p.tenantID, params); err != nil {
			return fmt.Errorf("promoter: update job: %w", err)
		}

		if err := tx.CreateIntelReviewLog(ctx, p.tenantID, reviewLogParams); err != nil {
			return fmt.Errorf("promoter: write auto_update review log: %w", err)
		}

		result = PromoteResult{DomainID: pgToUUID(existing.ID), ContentID: pgToUUID(existing.GuideID), Action: ActionUpdated, ChangedFields: changed}

		return nil
	})

	return result, err
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

	if _, ov := overrides["title"]; !ov {
		if v := stringField(ext, "title"); v != "" && v != existing.Title {
			params.Title = v

			changed = append(changed, "title")
		}
	}

	if _, ov := overrides["description"]; !ov {
		if v := stringPtrField(ext, "description_summary"); !strPtrEqual(v, existing.Description) {
			params.Description = v

			changed = append(changed, "description")
		}
	}

	if _, ov := overrides["country"]; !ov {
		if v := firstCountry(ext, "country"); v != "" && v != existing.Country {
			params.Country = v

			changed = append(changed, "country")
		}
	}

	if _, ov := overrides["city"]; !ov {
		if v := stringPtrField(ext, "city"); !strPtrEqual(v, existing.City) {
			params.City = v

			changed = append(changed, "city")
		}
	}

	if _, ov := overrides["employer"]; !ov {
		if v := stringPtrField(ext, "employer"); !strPtrEqual(v, existing.Employer) {
			params.Employer = v

			changed = append(changed, "employer")
		}
	}

	if _, ov := overrides["job_type"]; !ov {
		if v := stringPtrField(ext, "job_type"); !strPtrEqual(v, existing.JobType) {
			params.JobType = v

			changed = append(changed, "job_type")
		}
	}

	if _, ov := overrides["experience_level"]; !ov {
		if v := stringPtrField(ext, "experience_level"); !strPtrEqual(v, existing.ExperienceLevel) {
			params.ExperienceLevel = v

			changed = append(changed, "experience_level")
		}
	}

	if _, ov := overrides["salary_min"]; !ov {
		if v := numericField(ext, "salary_min"); !numericEqual(v, existing.SalaryMin) {
			params.SalaryMin = v

			changed = append(changed, "salary_min")
		}
	}

	if _, ov := overrides["salary_max"]; !ov {
		if v := numericField(ext, "salary_max"); !numericEqual(v, existing.SalaryMax) {
			params.SalaryMax = v

			changed = append(changed, "salary_max")
		}
	}

	if _, ov := overrides["currency"]; !ov {
		if v := stringPtrField(ext, "currency"); !strPtrEqual(v, existing.Currency) {
			params.Currency = v

			changed = append(changed, "currency")
		}
	}

	if _, ov := overrides["skills"]; !ov {
		if v := stringSliceField(ext, "skills"); !stringSliceEqual(v, existing.Skills) {
			params.Skills = v

			changed = append(changed, "skills")
		}
	}

	if _, ov := overrides["application_url"]; !ov {
		if v := stringPtrField(ext, "application_url"); !strPtrEqual(v, existing.ApplicationUrl) {
			params.ApplicationUrl = v

			changed = append(changed, "application_url")
		}
	}

	if _, ov := overrides["deadline"]; !ov {
		if v := dateField(ext, "deadline"); !dateEqual(v, existing.Deadline) {
			params.Deadline = v

			changed = append(changed, "deadline")
		}
	}

	if _, ov := overrides["remote"]; !ov {
		if v := stringPtrField(ext, "remote"); !strPtrEqual(v, existing.Remote) {
			params.Remote = v

			changed = append(changed, "remote")
		}
	}

	return params, changed
}

func marshalJobFieldChanges(existing gen.IntelJob, params gen.UpdateIntelJobParams, changed []string) ([]byte, error) {
	changes := make([]fieldChange, 0, len(changed))

	for _, c := range changed {
		fc := fieldChange{Field: c}

		switch c {
		case "title":
			fc.From, fc.To = existing.Title, params.Title
		case "description":
			fc.From, fc.To = derefStr(existing.Description), derefStr(params.Description)
		case "country":
			fc.From, fc.To = existing.Country, params.Country
		case "city":
			fc.From, fc.To = derefStr(existing.City), derefStr(params.City)
		case "employer":
			fc.From, fc.To = derefStr(existing.Employer), derefStr(params.Employer)
		case "job_type":
			fc.From, fc.To = derefStr(existing.JobType), derefStr(params.JobType)
		case "experience_level":
			fc.From, fc.To = derefStr(existing.ExperienceLevel), derefStr(params.ExperienceLevel)
		case "salary_min":
			fc.From, fc.To = numericStr(existing.SalaryMin), numericStr(params.SalaryMin)
		case "salary_max":
			fc.From, fc.To = numericStr(existing.SalaryMax), numericStr(params.SalaryMax)
		case "currency":
			fc.From, fc.To = derefStr(existing.Currency), derefStr(params.Currency)
		case "skills":
			fc.From, fc.To = existing.Skills, params.Skills
		case "application_url":
			fc.From, fc.To = derefStr(existing.ApplicationUrl), derefStr(params.ApplicationUrl)
		case "deadline":
			fc.From, fc.To = dateStr(existing.Deadline), dateStr(params.Deadline)
		case "remote":
			fc.From, fc.To = derefStr(existing.Remote), derefStr(params.Remote)
		}

		changes = append(changes, fc)
	}

	return json.Marshal(changes)
}
