package promoter

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/extractor"
	"github.com/jedafe/japa/internal/intelligence/reviewerctx"
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
	CreateContent(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateContentParams) (gen.CreateContentRow, error)
	UpdateIntelRecordPromoted(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelRecordPromotedParams) error
	CreateIntelReviewLog(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateIntelReviewLogParams) error
	GetCountrySpace(ctx context.Context, tenantID pgtype.UUID, countryCode string) (pgtype.UUID, error)
	WithTx(ctx context.Context, fn func(JobTxStore) error) error
}

// JobTxStore is the subset of JobStore needed inside a transaction.
type JobTxStore interface {
	CreateContent(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateContentParams) (gen.CreateContentRow, error)
	CreateIntelJob(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateIntelJobParams) (gen.IntelJob, error)
	UpdateIntelRecordPromoted(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelRecordPromotedParams) error
	UpdateIntelJob(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelJobParams) error
	CreateIntelReviewLog(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateIntelReviewLogParams) error
}

// JobPromoter promotes job intel_records into the intel_jobs projection
// table plus a contents guide entry. It implements the editorial-override
// invariant: on the update path, any field in editorial_overrides is left
// untouched so a human edit always wins over a re-crawl.
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
var ErrNotJob = fmt.Errorf("promoter: intel record is not a job")

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

func (p *JobPromoter) jobCreatePath(ctx context.Context, record gen.IntelRecord, source gen.IntelSource, ext extractor.Extraction) (PromoteResult, error) {
	title := stringField(ext, "title")
	if title == "" {
		title = source.Name
	}
	body := bodyFromJobExtraction(ext, title)
	slug := content.Slugify(title)
	hash := content.ComputeContentHash(body)
	canonicalURL := source.SourceUrl

	countryCode := firstCountry(ext, "country")
	var countrySpaceID pgtype.UUID
	if countryCode != "" {
		if id, err := p.store.GetCountrySpace(ctx, p.tenantID, countryCode); err == nil && id.Valid {
			countrySpaceID = id
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
		GuideCategory:   ptr("work_career"),
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
	jobParams := buildJobCreateParams(record, source, pgtype.UUID{}, ext)
	jobParams.CountrySpaceID = countrySpaceID

	var result PromoteResult
	err := p.store.WithTx(ctx, func(tx JobTxStore) error {
		contentRow, err := tx.CreateContent(ctx, p.tenantID, contentParams)
		if err != nil {
			return fmt.Errorf("promoter: create guide content: %w", err)
		}
		jobParams.GuideID = contentRow.ID
		job, err := tx.CreateIntelJob(ctx, p.tenantID, jobParams)
		if err != nil {
			return fmt.Errorf("promoter: create job: %w", err)
		}
		if err := tx.UpdateIntelRecordPromoted(ctx, p.tenantID, gen.UpdateIntelRecordPromotedParams{
			ID:               record.ID,
			PromotedRecordID: job.ID,
		}); err != nil {
			return fmt.Errorf("promoter: mark intel record promoted: %w", err)
		}
		result = PromoteResult{DomainID: pgToUUID(job.ID), ContentID: pgToUUID(contentRow.ID), Action: ActionCreated}
		return nil
	})
	return result, err
}

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

func bodyFromJobExtraction(ext extractor.Extraction, title string) string {
	if desc := stringField(ext, "description_summary"); desc != "" {
		return desc
	}
	return title
}
