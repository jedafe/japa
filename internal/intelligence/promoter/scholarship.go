package promoter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/extractor"
	"github.com/jedafe/japa/internal/intelligence/reviewerctx"
	"github.com/jedafe/japa/internal/intelligence/services"
)

// Store is the persistence surface the ScholarshipPromoter needs.
// *stores.Store satisfies this interface; tests substitute a mock.
//
// WithTx runs a callback inside a transaction. The callback receives a
// TxStore — a narrower interface that only exposes the write methods needed
// inside the transaction. This ensures createPath and updatePath perform
// their multi-table writes atomically (BUG-01/02 fix, §6.2 transaction
// boundary).
type Store interface {
	PersonaResolver
	GetIntelRecord(ctx context.Context, tenantID, id pgtype.UUID) (gen.IntelRecord, error)
	GetIntelSource(ctx context.Context, tenantID, id pgtype.UUID) (gen.IntelSource, error)
	GetScholarshipByIntelRecord(ctx context.Context, tenantID, intelRecordID pgtype.UUID) (gen.Scholarship, error)
	CreateScholarship(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateScholarshipParams) (gen.Scholarship, error)
	UpdateScholarship(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateScholarshipParams) error
	UpdateIntelRecordStatus(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelRecordStatusParams) error
	UpdateIntelRecordPromoted(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelRecordPromotedParams) error
	CreateIntelReviewLog(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateIntelReviewLogParams) error
	// GetCountrySpace resolves the country-level geo space (§05) id for an
	// ISO-3 country code, so the promoter can stamp scholarships.country_space_id.
	// Returns (pgtype.UUID{}, nil) when no country space exists yet; the
	// promoter treats that as a nullable country_space_id (spec §455). BUG-005 fix.
	GetCountrySpace(ctx context.Context, tenantID pgtype.UUID, countryCode string) (pgtype.UUID, error)
	// Content operations for creating and updating the guide entry.
	CreateContent(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateContentParams) (gen.CreateContentRow, error)
	GetContentByID(ctx context.Context, tenantID, id pgtype.UUID) (gen.GetContentByIDRow, error)
	GetContentBySlug(ctx context.Context, tenantID pgtype.UUID, slug string) (gen.GetContentBySlugRow, error)
	ContentSlugExists(ctx context.Context, tenantID pgtype.UUID, slug string) (bool, error)
	UpdateContent(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateContentParams) (gen.UpdateContentRow, error)
	WithTx(ctx context.Context, fn func(TxStore) error) error
}

// TxStore is the subset of Store needed inside a transaction. It exposes
// only the write methods that createPath and updatePath call, so the
// transactional store can be a narrower type. *stores.Store satisfies this
// interface (via WithTx's callback), and the test mock satisfies it
// directly.
type TxStore interface {
	CreateScholarship(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateScholarshipParams) (gen.Scholarship, error)
	UpdateIntelRecordStatus(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelRecordStatusParams) error
	UpdateIntelRecordPromoted(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelRecordPromotedParams) error
	UpdateScholarship(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateScholarshipParams) error
	CreateContent(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateContentParams) (gen.CreateContentRow, error)
	GetContentBySlug(ctx context.Context, tenantID pgtype.UUID, slug string) (gen.GetContentBySlugRow, error)
	ContentSlugExists(ctx context.Context, tenantID pgtype.UUID, slug string) (bool, error)
	UpdateContent(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateContentParams) (gen.UpdateContentRow, error)
	CreateIntelReviewLog(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateIntelReviewLogParams) error
}

// Default freshness score for a freshly promoted / re-verified row. The
// freshness worker (Task 21) decays this over time; the promoter resets it to
// full whenever a re-crawl confirms or updates the projection.
const freshScore int32 = 100

// ScholarshipPromoter promotes scholarship intel_records into the
// scholarships projection table. High-confidence, trusted records also create
// a published contents guide entry and link it through scholarships.guide_id.
// Lower-confidence records are created with guide_id NULL and routed to review.
//
// It implements the editorial-override-respect invariant: on the update path,
// any field present in scholarships.editorial_overrides is left untouched so a
// human edit always wins over a re-crawl.
type ScholarshipPromoter struct {
	store                Store
	tenantID             pgtype.UUID // tenant the generated guide rows are stamped with
	authorID             pgtype.UUID // system/automation user credited as the guide author
	aiModel              *string     // AI model used for extraction; set by WithAIModel
	autoThreshold        float64     // intel.extraction_confidence_auto, set by WithConfidenceThresholds
	reviewThreshold      float64     // intel.extraction_confidence_review, set by WithConfidenceThresholds
	thresholdsConfigured bool        // true once WithConfidenceThresholds has been applied
	now                  func() time.Time
}

// ScholarshipPromoterOption configures a ScholarshipPromoter.
type ScholarshipPromoterOption func(*ScholarshipPromoter)

// WithClock overrides the clock used for publish timestamps (tests).
func WithClock(fn func() time.Time) ScholarshipPromoterOption {
	return func(p *ScholarshipPromoter) { p.now = fn }
}

// WithAIModel sets the AI model name stamped on created content rows (ADR-088).
func WithAIModel(model string) ScholarshipPromoterOption {
	return func(p *ScholarshipPromoter) { p.aiModel = &model }
}

// WithConfidenceThresholds sets the auto-publish and review thresholds read
// from system_settings. The server wires these from
// intel.extraction_confidence_auto and intel.extraction_confidence_review.
// Callers that do not apply this option cannot promote records.
func WithConfidenceThresholds(auto, review float64) ScholarshipPromoterOption {
	return func(p *ScholarshipPromoter) {
		p.autoThreshold = auto
		p.reviewThreshold = review
		p.thresholdsConfigured = true
	}
}

// NewScholarshipPromoter creates a promoter backed by the given store. The
// tenantID and authorID identify the tenant and system author used for
// review-log actor stamps on the update path.
func NewScholarshipPromoter(store Store, tenantID, authorID pgtype.UUID, opts ...ScholarshipPromoterOption) *ScholarshipPromoter {
	p := &ScholarshipPromoter{
		store:    store,
		tenantID: tenantID,
		authorID: authorID,
		// Thresholds and the AI model are intentionally not defaulted here.
		// AGENTS.md Rule 3 prohibits hard-coding config values; the server
		// must supply them via WithConfidenceThresholds / WithAIModel.
		now: time.Now,
	}
	for _, opt := range opts {
		opt(p)
	}

	return p
}

// Entity returns the entity_type this promoter handles.
func (p *ScholarshipPromoter) Entity() string { return "scholarship" }

// ErrNotScholarship is returned when Promote is called on an intel record whose
// entity_type is not "scholarship".
var ErrNotScholarship = errors.New("promoter: intel record is not a scholarship")

// ErrThresholdsNotConfigured is returned when Promote is called on a promoter
// that was not configured with confidence thresholds.
var ErrThresholdsNotConfigured = errors.New("promoter: confidence thresholds not configured")

// Promote maps the intel record identified by recordID to a scholarships
// projection row, creating it or updating it in place while respecting
// editorial overrides. High-confidence, trusted records create a published
// contents guide row immediately; lower-confidence records are routed to
// review with a NULL guide_id.
func (p *ScholarshipPromoter) Promote(ctx context.Context, recordID uuid.UUID) (PromoteResult, error) {
	pgID := uuidToPg(recordID)

	record, err := p.store.GetIntelRecord(ctx, p.tenantID, pgID)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: load intel record: %w", err)
	}

	if record.EntityType != p.Entity() {
		return PromoteResult{}, ErrNotScholarship
	}

	if !p.thresholdsConfigured {
		return PromoteResult{}, ErrThresholdsNotConfigured
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
	existing, err := p.store.GetScholarshipByIntelRecord(ctx, p.tenantID, pgID)
	if err != nil {
		if isNotFound(err) {
			return p.createPath(ctx, record, source, ext)
		}

		return PromoteResult{}, fmt.Errorf("promoter: load existing scholarship: %w", err)
	}

	return p.updatePath(ctx, record, source, existing, ext)
}

// createPath inserts the scholarships projection row. High-confidence,
// trusted records also create a published contents guide and link it through
// scholarships.guide_id. Lower-confidence records are created in review
// status with guide_id NULL and the intel record is routed to review_required.
func (p *ScholarshipPromoter) createPath(ctx context.Context, record gen.IntelRecord, source gen.IntelSource, ext extractor.Extraction) (PromoteResult, error) {
	// Resolve the destination country's geo space (§05) so the space-scoped
	// radar feed (ListScholarshipsBySpace) can match this scholarship. The
	// lookup is best-effort: when no country space exists for the extracted
	// ISO-3 code, country_space_id stays NULL and the radar falls back to the
	// ?country= filter. BUG-005 fix (spec §455).
	countryCode := firstCountry(ext, "country")

	var countrySpaceID pgtype.UUID

	if countryCode != "" {
		if id, err := p.store.GetCountrySpace(ctx, p.tenantID, countryCode); err == nil && id.Valid {
			countrySpaceID = id
		}
	}

	reviewerID, hasReviewer := reviewerctx.ReviewerFromContext(ctx)
	autoPublish := p.shouldAutoPublish(record, source, hasReviewer)
	now := p.now()

	var result PromoteResult

	err := p.store.WithTx(ctx, func(tx TxStore) error {
		var guideID pgtype.UUID

		verificationStatus := "needs_review"
		overrideSource := "ingestion"
		intelStatus := "review_required"

		if autoPublish || hasReviewer {
			title := stringField(ext, "title")
			if title == "" {
				title = source.Name
			}

			bodyText := buildScholarshipBody(ext)

			slug := content.Slugify(title)
			if slug == "" || slug == "untitled" {
				slug = content.Slugify(source.Name)
			}

			slug, err := p.ensureUniqueSlug(ctx, tx, slug, pgtype.UUID{})
			if err != nil {
				return fmt.Errorf("promoter: ensure unique slug on create: %w", err)
			}

			hash := content.ComputeContentHash(bodyText)

			canonicalURL := source.SourceUrl
			if u := stringPtrField(ext, "source_url"); u != nil && *u != "" {
				canonicalURL = *u
			}

			contentParams := gen.CreateContentParams{
				TenantID:        p.tenantID,
				AuthorID:        resolveAuthorID(ctx, p.store, source, p.tenantID, p.authorID),
				ContentType:     "guide",
				Title:           title,
				Slug:            slug,
				Body:            bodyText,
				BodyHtml:        bodyHTML(bodyText),
				FieldValues:     []byte(`{}`),
				ContentHash:     &hash,
				GeoSpaceID:      countrySpaceID,
				CanonicalUrl:    &canonicalURL,
				Status:          "published",
				Visibility:      "public",
				SeoIndexable:    true,
				ReadTimeMinutes: content.ComputeReadTime(bodyText),
				GuideCategory:   ptr("scholarship"),
				PublishAt:       pgtype.Timestamptz{Time: now, Valid: true},
				PublishedAt:     pgtype.Timestamptz{Time: now, Valid: true},
				AiGenerated:     true,
				AiModel:         p.aiModel,
				AiGeneratedAt:   pgtype.Timestamptz{Time: now, Valid: true},
			}
			if hasReviewer {
				contentParams.HitlReviewedBy = reviewerID
				contentParams.HitlReviewedAt = pgtype.Timestamptz{Time: now, Valid: true}
			}

			contentRow, err := tx.CreateContent(ctx, p.tenantID, contentParams)
			if err != nil {
				return fmt.Errorf("promoter: create guide content: %w", err)
			}

			guideID = contentRow.ID
			verificationStatus = "verified"

			overrideSource = "ingestion"
			if hasReviewer {
				overrideSource = "editorial"
			}

			intelStatus = "published"
		}

		scholarshipParams := buildCreateParams(record, source, guideID, ext, verificationStatus, overrideSource)
		scholarshipParams.CountrySpaceID = countrySpaceID

		scholarship, err := tx.CreateScholarship(ctx, p.tenantID, scholarshipParams)
		if err != nil {
			return fmt.Errorf("promoter: create scholarship: %w", err)
		}

		if intelStatus == "published" {
			if err := tx.UpdateIntelRecordPromoted(ctx, p.tenantID, gen.UpdateIntelRecordPromotedParams{
				ID:               record.ID,
				PromotedRecordID: scholarship.ID,
			}); err != nil {
				return fmt.Errorf("promoter: mark intel record promoted: %w", err)
			}

			result = PromoteResult{
				DomainID:  pgToUUID(scholarship.ID),
				ContentID: pgToUUID(guideID),
				Action:    ActionCreated,
			}
		} else {
			if err := tx.UpdateIntelRecordStatus(ctx, p.tenantID, gen.UpdateIntelRecordStatusParams{
				ID:     record.ID,
				Status: "review_required",
			}); err != nil {
				return fmt.Errorf("promoter: mark intel record review required: %w", err)
			}

			result = PromoteResult{
				DomainID:  pgToUUID(scholarship.ID),
				ContentID: uuid.Nil,
				Action:    ActionCreated,
			}
		}

		return nil
	})

	return result, err
}

// updatePath applies the new extraction to the existing scholarship row,
// skipping any field present in editorial_overrides, and records an
// auto_update review-log entry when fields actually change. It also syncs the
// linked contents row (if any) with the new extraction, preserving editorial
// edits. When a human reviewer approves a review-required record, it creates
// the contents guide and publishes the scholarship in one transaction.
func (p *ScholarshipPromoter) updatePath(ctx context.Context, record gen.IntelRecord, source gen.IntelSource, existing gen.Scholarship, ext extractor.Extraction) (PromoteResult, error) {
	overrides, err := parseOverrideKeys(existing.EditorialOverrides)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: parse editorial overrides: %w", err)
	}

	params, changed := buildUpdateParams(existing, ext, overrides)
	params.ConfidenceOverall = record.ConfidenceOverall
	reviewerID, hasReviewer := reviewerctx.ReviewerFromContext(ctx)
	approving := hasReviewer && !existing.GuideID.Valid
	now := p.now()

	if approving {
		params.FreshnessScore = freshScore
		params.VerificationStatus = "verified"
		params.OverrideSource = overrideSourceForApproval(existing)

		changed = append(changed, "guide_id", "verification_status", "override_source")
	} else {
		params.FreshnessScore = freshScore
	}

	if !approving && len(changed) == 0 {
		// No field-level changes: skip the auto_update log, just bump
		// last_verified_at + freshness_score by re-stamping the row with
		// its current values at full freshness.
		if err := p.store.WithTx(ctx, func(tx TxStore) error {
			return tx.UpdateScholarship(ctx, p.tenantID, params)
		}); err != nil {
			return PromoteResult{}, fmt.Errorf("promoter: bump scholarship freshness: %w", err)
		}

		return PromoteResult{
			DomainID:  pgToUUID(existing.ID),
			ContentID: pgToUUID(existing.GuideID),
			Action:    ActionVerified,
		}, nil
	}

	var contentRow gen.GetContentByIDRow
	if existing.GuideID.Valid {
		contentRow, err = p.store.GetContentByID(ctx, p.tenantID, existing.GuideID)
		if err != nil {
			return PromoteResult{}, fmt.Errorf("promoter: load existing guide content: %w", err)
		}
	}

	var reviewLogParams *gen.CreateIntelReviewLogParams

	if !approving && len(changed) > 0 {
		changesJSON, err := marshalFieldChanges(existing, params, changed)
		if err != nil {
			return PromoteResult{}, fmt.Errorf("promoter: marshal field changes: %w", err)
		}

		note := "automatic re-crawl update"
		reviewLogParams = &gen.CreateIntelReviewLogParams{
			IntelRecordID: record.ID,
			ActorUserID:   p.authorID,
			Action:        "auto_update",
			FieldChanges:  changesJSON,
			Note:          &note,
		}
	}

	var result PromoteResult

	err = p.store.WithTx(ctx, func(tx TxStore) error {
		if approving {
			mergedExt := bodyExtractionFrom(existing, ext, overrides)
			title := params.Title
			bodyText := buildScholarshipBody(mergedExt)

			slug := content.Slugify(title)
			if slug == "" || slug == "untitled" {
				slug = content.Slugify(source.Name)
			}

			slug, err := p.ensureUniqueSlug(ctx, tx, slug, pgtype.UUID{})
			if err != nil {
				return fmt.Errorf("promoter: ensure unique slug on approve: %w", err)
			}

			hash := content.ComputeContentHash(bodyText)

			canonicalURL := source.SourceUrl
			if u := stringPtrField(ext, "source_url"); u != nil && *u != "" {
				canonicalURL = *u
			}

			contentParams := gen.CreateContentParams{
				TenantID:        p.tenantID,
				AuthorID:        resolveAuthorID(ctx, p.store, source, p.tenantID, p.authorID),
				ContentType:     "guide",
				Title:           title,
				Slug:            slug,
				Body:            bodyText,
				BodyHtml:        bodyHTML(bodyText),
				FieldValues:     []byte(`{}`),
				ContentHash:     &hash,
				GeoSpaceID:      params.CountrySpaceID,
				CanonicalUrl:    &canonicalURL,
				Status:          "published",
				Visibility:      "public",
				SeoIndexable:    true,
				ReadTimeMinutes: content.ComputeReadTime(bodyText),
				GuideCategory:   ptr("scholarship"),
				PublishAt:       pgtype.Timestamptz{Time: now, Valid: true},
				PublishedAt:     pgtype.Timestamptz{Time: now, Valid: true},
				AiGenerated:     true,
				AiModel:         p.aiModel,
				AiGeneratedAt:   pgtype.Timestamptz{Time: now, Valid: true},
				HitlReviewedBy:  reviewerID,
				HitlReviewedAt:  pgtype.Timestamptz{Time: now, Valid: true},
			}

			newContent, err := tx.CreateContent(ctx, p.tenantID, contentParams)
			if err != nil {
				return fmt.Errorf("promoter: create guide content on approve: %w", err)
			}

			params.GuideID = newContent.ID
		}

		// If a published guide exists and source-derived title/body changed,
		// update the contents row while preserving any editorial overrides.
		if existing.GuideID.Valid {
			newTitle := contentRow.Title
			if sliceContains(changed, "title") && !isOverridden(overrides, "title") {
				newTitle = params.Title
			}

			mergedExt := bodyExtractionFrom(existing, ext, overrides)
			newBody := buildScholarshipBody(mergedExt)
			newHash := content.ComputeContentHash(newBody)

			newSlug, err := p.ensureUniqueSlug(ctx, tx, content.Slugify(newTitle), existing.GuideID)
			if err != nil {
				return fmt.Errorf("promoter: ensure unique slug on update: %w", err)
			}

			needsUpdate := newBody != contentRow.Body || newTitle != contentRow.Title || newSlug != contentRow.Slug
			if needsUpdate {
				if _, err := tx.UpdateContent(ctx, p.tenantID, buildContentUpdateParams(contentRow, newTitle, newSlug, newBody, newHash)); err != nil {
					return fmt.Errorf("promoter: update guide content: %w", err)
				}
			}
		}

		if err := tx.UpdateScholarship(ctx, p.tenantID, params); err != nil {
			return fmt.Errorf("promoter: update scholarship: %w", err)
		}

		if approving {
			if err := tx.UpdateIntelRecordPromoted(ctx, p.tenantID, gen.UpdateIntelRecordPromotedParams{
				ID:               record.ID,
				PromotedRecordID: existing.ID,
			}); err != nil {
				return fmt.Errorf("promoter: mark intel record promoted on approve: %w", err)
			}
		}

		if reviewLogParams != nil {
			if err := tx.CreateIntelReviewLog(ctx, p.tenantID, *reviewLogParams); err != nil {
				return fmt.Errorf("promoter: write auto_update review log: %w", err)
			}
		}

		action := ActionUpdated
		if !approving && len(changed) == 0 {
			action = ActionVerified
		}

		result = PromoteResult{
			DomainID:      pgToUUID(existing.ID),
			ContentID:     pgToUUID(params.GuideID),
			Action:        action,
			ChangedFields: changed,
		}

		return nil
	})

	return result, err
}

// ---------------------------------------------------------------------------
// Extraction -> scholarship params
// ---------------------------------------------------------------------------

// buildCreateParams maps a validated extraction to a CreateScholarshipParams.
// The verificationStatus and overrideSource are set by the caller based on
// the publish gate (verified/ingestion for auto-publish, needs_review for
// review-required records).
func buildCreateParams(record gen.IntelRecord, source gen.IntelSource, guideID pgtype.UUID, ext extractor.Extraction, verificationStatus, overrideSource string) gen.CreateScholarshipParams {
	return gen.CreateScholarshipParams{
		IntelRecordID:        record.ID,
		GuideID:              guideID,
		SourceUrl:            source.SourceUrl,
		SourceType:           source.SourceType,
		TrustLevel:           source.TrustLevel,
		Title:                stringField(ext, "title"),
		Description:          descriptionField(ext),
		Country:              firstCountry(ext, "country"),
		CountrySpaceID:       pgtype.UUID{}, // caller resolves and overrides
		University:           stringPtrField(ext, "university"),
		DegreeLevel:          stringPtrField(ext, "degree_level"),
		FieldsOfStudy:        orEmptySlice(stringSliceField(ext, "fields_of_study")),
		EligibleCountries:    orEmptySlice(stringSliceField(ext, "eligible_countries")),
		Funding:              orEmptyJSON(jsonbField(ext, "funding")),
		Deadline:             dateField(ext, "deadline"),
		ApplicationUrl:       stringPtrField(ext, "application_url"),
		RequiredDocuments:    orEmptySlice(stringSliceField(ext, "required_documents")),
		MinimumGpa:           numericField(ext, "minimum_gpa"),
		LanguageRequirements: orEmptyJSON(jsonbField(ext, "language_requirements")),
		ConfidenceOverall:    record.ConfidenceOverall,
		// The scholarships CHECK constraint (migration 000429) only allows
		// verification_status IN ('verified','needs_review','conflict','stale')
		// and override_source IN ('ingestion','editorial','mixed'); the prior
		// 'auto' values violated the CHECK and rejected every insert.
		VerificationStatus: verificationStatus,
		FreshnessScore:     freshScore,
		OverrideSource:     overrideSource,
		EditorialOverrides: emptyJSONObject(),
	}
}

// buildUpdateParams produces an UpdateScholarshipParams seeded from the
// existing row, then applies each extraction field that is NOT in overrides
// and whose value actually changed. It returns the params and the list of
// changed column names. Fields in editorial_overrides keep the existing value
// (human edit wins).
func buildUpdateParams(existing gen.Scholarship, ext extractor.Extraction, overrides map[string]struct{}) (gen.UpdateScholarshipParams, []string) {
	params := gen.UpdateScholarshipParams{
		ID:                   existing.ID,
		Title:                existing.Title,
		Description:          existing.Description,
		Country:              existing.Country,
		CountrySpaceID:       existing.CountrySpaceID,
		University:           existing.University,
		DegreeLevel:          existing.DegreeLevel,
		FieldsOfStudy:        existing.FieldsOfStudy,
		EligibleCountries:    existing.EligibleCountries,
		Funding:              existing.Funding,
		Deadline:             existing.Deadline,
		ApplicationUrl:       existing.ApplicationUrl,
		RequiredDocuments:    existing.RequiredDocuments,
		MinimumGpa:           existing.MinimumGpa,
		LanguageRequirements: existing.LanguageRequirements,
		ConfidenceOverall:    existing.ConfidenceOverall,
		VerificationStatus:   existing.VerificationStatus,
		FreshnessScore:       existing.FreshnessScore,
		OverrideSource:       existing.OverrideSource,
		EditorialOverrides:   existing.EditorialOverrides,
		SourceUrl:            existing.SourceUrl,
		SourceType:           existing.SourceType,
		TrustLevel:           existing.TrustLevel,
		GuideID:              existing.GuideID,
	}

	var changed []string

	// title
	if _, ov := overrides["title"]; !ov {
		if v := stringField(ext, "title"); v != "" && v != existing.Title {
			params.Title = v

			changed = append(changed, "title")
		}
	}
	// description (description_summary, description, eligibility, abstract)
	if _, ov := overrides["description"]; !ov {
		if v := descriptionField(ext); v != nil && !strPtrEqual(v, existing.Description) {
			params.Description = v

			changed = append(changed, "description")
		}
	}
	// country
	if _, ov := overrides["country"]; !ov {
		if v := firstCountry(ext, "country"); v != "" && v != existing.Country {
			params.Country = v

			changed = append(changed, "country")
		}
	}
	// university
	if _, ov := overrides["university"]; !ov {
		if v := stringPtrField(ext, "university"); v != nil && !strPtrEqual(v, existing.University) {
			params.University = v

			changed = append(changed, "university")
		}
	}
	// degree_level
	if _, ov := overrides["degree_level"]; !ov {
		if v := stringPtrField(ext, "degree_level"); v != nil && !strPtrEqual(v, existing.DegreeLevel) {
			params.DegreeLevel = v

			changed = append(changed, "degree_level")
		}
	}
	// fields_of_study
	if _, ov := overrides["fields_of_study"]; !ov {
		if v := stringSliceField(ext, "fields_of_study"); v != nil && !stringSliceEqual(v, existing.FieldsOfStudy) {
			params.FieldsOfStudy = orEmptySlice(v)

			changed = append(changed, "fields_of_study")
		}
	}
	// eligible_countries
	if _, ov := overrides["eligible_countries"]; !ov {
		if v := stringSliceField(ext, "eligible_countries"); v != nil && !stringSliceEqual(v, existing.EligibleCountries) {
			params.EligibleCountries = orEmptySlice(v)

			changed = append(changed, "eligible_countries")
		}
	}
	// funding
	if _, ov := overrides["funding"]; !ov {
		if v := jsonbField(ext, "funding"); v != nil && !bytesEqual(v, existing.Funding) {
			params.Funding = orEmptyJSON(v)

			changed = append(changed, "funding")
		}
	}
	// deadline
	if _, ov := overrides["deadline"]; !ov {
		if v := dateField(ext, "deadline"); v.Valid && !dateEqual(v, existing.Deadline) {
			params.Deadline = v

			changed = append(changed, "deadline")
		}
	}
	// application_url
	if _, ov := overrides["application_url"]; !ov {
		if v := stringPtrField(ext, "application_url"); v != nil && !strPtrEqual(v, existing.ApplicationUrl) {
			params.ApplicationUrl = v

			changed = append(changed, "application_url")
		}
	}
	// required_documents
	if _, ov := overrides["required_documents"]; !ov {
		if v := stringSliceField(ext, "required_documents"); v != nil && !stringSliceEqual(v, existing.RequiredDocuments) {
			params.RequiredDocuments = orEmptySlice(v)

			changed = append(changed, "required_documents")
		}
	}
	// minimum_gpa
	if _, ov := overrides["minimum_gpa"]; !ov {
		if v := numericField(ext, "minimum_gpa"); v.Valid && !numericEqual(v, existing.MinimumGpa) {
			params.MinimumGpa = v

			changed = append(changed, "minimum_gpa")
		}
	}
	// language_requirements
	if _, ov := overrides["language_requirements"]; !ov {
		if v := jsonbField(ext, "language_requirements"); v != nil && !bytesEqual(v, existing.LanguageRequirements) {
			params.LanguageRequirements = orEmptyJSON(v)

			changed = append(changed, "language_requirements")
		}
	}

	return params, changed
}

// ---------------------------------------------------------------------------
// Body / content helpers
// ---------------------------------------------------------------------------

// buildScholarshipBody turns a scholarship extraction into a plain-text guide
// body. It includes title, description, university, degree level, deadline,
// funding, eligible countries and application URL. Empty or collection-only
// fields are omitted. The resulting text is HTML-escaped when wrapped by the
// caller (bodyHTML).
func buildScholarshipBody(ext extractor.Extraction) string {
	var b strings.Builder
	if t := stringField(ext, "title"); t != "" {
		b.WriteString(t)
		b.WriteString("\n\n")
	}

	if d := descriptionField(ext); d != nil && *d != "" {
		b.WriteString(*d)
		b.WriteString("\n\n")
	}

	writeField(&b, ext, "country", "Country")
	writeField(&b, ext, "university", "Host institution")
	writeField(&b, ext, "degree_level", "Level")
	writeField(&b, ext, "deadline", "Deadline")
	writeField(&b, ext, "funding", "Funding")
	writeField(&b, ext, "eligible_countries", "Eligible countries")

	if u := stringPtrField(ext, "application_url"); u != nil && *u != "" {
		b.WriteString("Apply at: ")
		b.WriteString(*u)
		b.WriteString("\n")
	}

	return strings.TrimSpace(b.String())
}

// descriptionField returns the first non-empty summary/description field from
// the extraction, trying description_summary, description, eligibility and
// abstract in order.
func descriptionField(ext extractor.Extraction) *string {
	for _, key := range []string{"description_summary", "description", "eligibility", "abstract"} {
		if v := stringPtrField(ext, key); v != nil && *v != "" {
			return v
		}
	}

	return nil
}

// writeField appends a "Label: value\n" line to b when the extraction field
// is present and non-empty. It skips empty collections rendered as "[]" or
// empty maps rendered as "{}".
func writeField(b *strings.Builder, ext extractor.Extraction, name, label string) {
	v, ok := ext[name]
	if !ok || v.Value == nil {
		return
	}

	s := formatFieldValue(v.Value)
	if s == "" || s == "[]" || s == "{}" {
		return
	}

	b.WriteString(label)
	b.WriteString(": ")
	b.WriteString(s)
	b.WriteString("\n")
}

// formatFieldValue renders an extraction field value as a human-readable string.
// Strings are returned as-is, slices and maps are JSON-encoded to avoid Go's
// default map formatting like "map[amount:full]".
func formatFieldValue(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case []any:
		if len(v) == 0 {
			return "[]"
		}
		// If the slice is a list of strings, render as a comma-separated list.
		allStrings := true

		for _, e := range v {
			if _, ok := e.(string); !ok {
				allStrings = false
				break
			}
		}

		if allStrings {
			a := make([]string, 0, len(v))

			for _, e := range v {
				if s, ok := e.(string); ok {
					a = append(a, strings.TrimSpace(s))
				}
			}

			return strings.Join(a, ", ")
		}

		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}

		return string(b)
	case map[string]any:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}

		return string(b)
	default:
		s := fmt.Sprint(v)
		return strings.TrimSpace(s)
	}
}

// bodyExtractionFrom builds an extraction that drives buildScholarshipBody for
// the update path. Fields present in editorial_overrides use the existing
// scholarship value, so a human edit is never overwritten. Fields not
// overridden prefer the latest extraction, but fall back to the existing value
// when the re-crawl no longer returns them.
func bodyExtractionFrom(existing gen.Scholarship, ext extractor.Extraction, overrides map[string]struct{}) extractor.Extraction {
	out := make(extractor.Extraction)

	// title
	if _, ov := overrides["title"]; ov && existing.Title != "" {
		out["title"] = extractor.FieldExtraction{Value: existing.Title, Confidence: 1}
	} else if f, ok := ext["title"]; ok && f.Value != nil {
		if s := stringField(ext, "title"); s != "" {
			out["title"] = extractor.FieldExtraction{Value: s, Confidence: 1}
		}
	} else if existing.Title != "" {
		out["title"] = extractor.FieldExtraction{Value: existing.Title, Confidence: 1}
	}

	// description (extraction key: description_summary)
	if _, ov := overrides["description"]; ov {
		if existing.Description != nil && *existing.Description != "" {
			out["description_summary"] = extractor.FieldExtraction{Value: *existing.Description, Confidence: 1}
		}
	} else if d := descriptionField(ext); d != nil {
		out["description_summary"] = extractor.FieldExtraction{Value: *d, Confidence: 1}
	} else if existing.Description != nil && *existing.Description != "" {
		out["description_summary"] = extractor.FieldExtraction{Value: *existing.Description, Confidence: 1}
	}

	// university
	if _, ov := overrides["university"]; ov {
		if existing.University != nil && *existing.University != "" {
			out["university"] = extractor.FieldExtraction{Value: *existing.University, Confidence: 1}
		}
	} else if f, ok := ext["university"]; ok && f.Value != nil {
		if s := stringPtrField(ext, "university"); s != nil {
			out["university"] = extractor.FieldExtraction{Value: *s, Confidence: 1}
		}
	} else if existing.University != nil && *existing.University != "" {
		out["university"] = extractor.FieldExtraction{Value: *existing.University, Confidence: 1}
	}

	// degree_level
	if _, ov := overrides["degree_level"]; ov {
		if existing.DegreeLevel != nil && *existing.DegreeLevel != "" {
			out["degree_level"] = extractor.FieldExtraction{Value: *existing.DegreeLevel, Confidence: 1}
		}
	} else if f, ok := ext["degree_level"]; ok && f.Value != nil {
		if s := stringPtrField(ext, "degree_level"); s != nil {
			out["degree_level"] = extractor.FieldExtraction{Value: *s, Confidence: 1}
		}
	} else if existing.DegreeLevel != nil && *existing.DegreeLevel != "" {
		out["degree_level"] = extractor.FieldExtraction{Value: *existing.DegreeLevel, Confidence: 1}
	}

	// deadline
	if _, ov := overrides["deadline"]; ov {
		if existing.Deadline.Valid {
			out["deadline"] = extractor.FieldExtraction{Value: existing.Deadline.Time.Format("2006-01-02"), Confidence: 1}
		}
	} else if f, ok := ext["deadline"]; ok && f.Value != nil {
		if d := dateField(ext, "deadline"); d.Valid {
			out["deadline"] = extractor.FieldExtraction{Value: d.Time.Format("2006-01-02"), Confidence: 1}
		}
	} else if existing.Deadline.Valid {
		out["deadline"] = extractor.FieldExtraction{Value: existing.Deadline.Time.Format("2006-01-02"), Confidence: 1}
	}

	// funding
	if v := bodyFundingValue(existing, ext, overrides); v != nil {
		out["funding"] = extractor.FieldExtraction{Value: v, Confidence: 1}
	}

	// eligible_countries
	if _, ov := overrides["eligible_countries"]; ov {
		if len(existing.EligibleCountries) > 0 {
			out["eligible_countries"] = extractor.FieldExtraction{Value: existing.EligibleCountries, Confidence: 1}
		}
	} else if f, ok := ext["eligible_countries"]; ok && f.Value != nil {
		if s := stringSliceField(ext, "eligible_countries"); len(s) > 0 {
			out["eligible_countries"] = extractor.FieldExtraction{Value: s, Confidence: 1}
		}
	} else if len(existing.EligibleCountries) > 0 {
		out["eligible_countries"] = extractor.FieldExtraction{Value: existing.EligibleCountries, Confidence: 1}
	}

	// application_url
	if _, ov := overrides["application_url"]; ov {
		if existing.ApplicationUrl != nil && *existing.ApplicationUrl != "" {
			out["application_url"] = extractor.FieldExtraction{Value: *existing.ApplicationUrl, Confidence: 1}
		}
	} else if f, ok := ext["application_url"]; ok && f.Value != nil {
		if s := stringPtrField(ext, "application_url"); s != nil {
			out["application_url"] = extractor.FieldExtraction{Value: *s, Confidence: 1}
		}
	} else if existing.ApplicationUrl != nil && *existing.ApplicationUrl != "" {
		out["application_url"] = extractor.FieldExtraction{Value: *existing.ApplicationUrl, Confidence: 1}
	}

	return out
}

// bodyFundingValue returns the display value to use for the funding body line,
// respecting editorial overrides. JSON values are unmarshalled to a generic any
// so fmt.Sprint renders them sensibly.
func bodyFundingValue(existing gen.Scholarship, ext extractor.Extraction, overrides map[string]struct{}) any {
	var raw []byte
	if _, ov := overrides["funding"]; ov {
		raw = existing.Funding
	} else if f, ok := ext["funding"]; ok && f.Value != nil {
		if b, ok := f.Value.([]byte); ok {
			raw = b
		} else {
			// Already a map/slice from JSON decode.
			return f.Value
		}
	} else {
		raw = existing.Funding
	}

	if len(raw) == 0 || bytes.Equal(raw, []byte(`{}`)) || bytes.Equal(raw, []byte(`null`)) {
		return nil
	}

	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}

	return v
}

// buildContentUpdateParams returns a contents row update that preserves
// existing metadata and only replaces title, slug, body, body_html and hash.
func buildContentUpdateParams(contentRow gen.GetContentByIDRow, title, slug, body, hash string) gen.UpdateContentParams {
	return gen.UpdateContentParams{
		ID:              contentRow.ID,
		TenantID:        contentRow.TenantID,
		Title:           title,
		Slug:            slug,
		Body:            body,
		BodyHtml:        bodyHTML(body),
		FieldValues:     contentRow.FieldValues,
		ContentHash:     &hash,
		GeoSpaceID:      contentRow.GeoSpaceID,
		CanonicalUrl:    contentRow.CanonicalUrl,
		HeaderImageID:   contentRow.HeaderImageID,
		FeaturedImageID: contentRow.FeaturedImageID,
		MetaTitle:       contentRow.MetaTitle,
		MetaDescription: contentRow.MetaDescription,
		OgImageUrl:      contentRow.OgImageUrl,
		SeoIndexable:    &contentRow.SeoIndexable,
		Status:          contentRow.Status,
		Visibility:      contentRow.Visibility,
		ReadTimeMinutes: content.ComputeReadTime(body),
		PublishAt:       contentRow.PublishAt,
		PublishedAt:     contentRow.PublishedAt,
		IsLocked:        contentRow.IsLocked,
		IsPinned:        contentRow.IsPinned,
		OrganizationID:  contentRow.OrganizationID,
		GuideCategory:   contentRow.GuideCategory,
		AiGeneratedAt:   contentRow.AiGeneratedAt,
		AiModel:         contentRow.AiModel,
		JourneyStage:    contentRow.JourneyStage,
		VisaRoute:       contentRow.VisaRoute,
		SourceCountry:   contentRow.SourceCountry,
		TargetCountry:   contentRow.TargetCountry,
	}
}

// shouldAutoPublish runs the same confidence + trust gate used by the worker.
// A human reviewer (reviewer context) always wins and publishes.
func (p *ScholarshipPromoter) shouldAutoPublish(record gen.IntelRecord, source gen.IntelSource, hasReviewer bool) bool {
	if hasReviewer {
		return true
	}

	decision := services.Classify(services.GateInput{
		EntityType:        record.EntityType,
		ConfidenceOverall: float64(record.ConfidenceOverall),
		TrustLevel:        source.TrustLevel,
		AutoThreshold:     p.autoThreshold,
		ReviewThreshold:   p.reviewThreshold,
	})

	return decision == services.GateAutoPublish
}

// overrideSourceForApproval returns the correct override_source when a human
// reviewer approves a scholarship. Ingestion-only rows become editorial; rows
// already carrying human edits become mixed.
func overrideSourceForApproval(existing gen.Scholarship) string {
	if existing.OverrideSource == "editorial" || existing.OverrideSource == "mixed" {
		return "mixed"
	}

	return "editorial"
}

// sliceContains reports whether s contains v.
func sliceContains(s []string, v string) bool {
	for _, e := range s {
		if e == v {
			return true
		}
	}

	return false
}

// isOverridden reports whether key is in the editorial override set.
func isOverridden(overrides map[string]struct{}, key string) bool {
	_, ok := overrides[key]
	return ok
}

// ---------------------------------------------------------------------------
// Extraction field accessors
// ---------------------------------------------------------------------------

func parseExtraction(raw []byte) (extractor.Extraction, error) {
	return parseExtractionShared(raw)
}

// orEmptySlice returns s if non-nil/non-empty, or []string{} otherwise.
// Prevents NULL insertion into NOT NULL array columns with DEFAULT '{}'.
func orEmptySlice(s []string) []string {
	if s == nil {
		return []string{}
	}

	return s
}

// orEmptyJSON returns b if non-nil, or []byte(`{}`) otherwise.
// Prevents NULL insertion into NOT NULL jsonb columns with DEFAULT '{}'.
func orEmptyJSON(b []byte) []byte {
	if b == nil {
		return []byte(`{}`)
	}

	return b
}

// stringField returns the string form of an extraction field, or "".
func stringField(ext extractor.Extraction, key string) string {
	f, ok := ext[key]
	if !ok || f.Value == nil {
		return ""
	}

	switch v := f.Value.(type) {
	case string:
		return strings.TrimSpace(v)
	case []any:
		// country_array may arrive as a slice; take the first element.
		if len(v) > 0 {
			if s, ok := v[0].(string); ok {
				return strings.TrimSpace(s)
			}
		}
	}

	return ""
}

// firstCountry returns the first element of a country_array field, or "".
func firstCountry(ext extractor.Extraction, key string) string {
	f, ok := ext[key]
	if !ok || f.Value == nil {
		return ""
	}

	if arr, ok := f.Value.([]any); ok && len(arr) > 0 {
		if s, ok := arr[0].(string); ok {
			return strings.TrimSpace(s)
		}
	}

	if s, ok := f.Value.(string); ok {
		return strings.TrimSpace(s)
	}

	return ""
}

func stringPtrField(ext extractor.Extraction, key string) *string {
	s := stringField(ext, key)
	if s == "" {
		return nil
	}

	return &s
}

func stringSliceField(ext extractor.Extraction, key string) []string {
	f, ok := ext[key]
	if !ok || f.Value == nil {
		return nil
	}

	arr, ok := f.Value.([]any)
	if !ok {
		return nil
	}

	out := make([]string, 0, len(arr))

	for _, item := range arr {
		if s, ok := item.(string); ok {
			if t := strings.TrimSpace(s); t != "" {
				out = append(out, t)
			}
		}
	}

	if len(out) == 0 {
		return []string{}
	}

	return out
}

func dateField(ext extractor.Extraction, key string) pgtype.Date {
	f, ok := ext[key]
	if !ok || f.Value == nil {
		return pgtype.Date{}
	}

	s, ok := f.Value.(string)
	if !ok {
		return pgtype.Date{}
	}

	t, err := time.Parse("2006-01-02", strings.TrimSpace(s))
	if err != nil {
		return pgtype.Date{}
	}

	return pgtype.Date{Time: t, Valid: true}
}

func numericField(ext extractor.Extraction, key string) pgtype.Numeric {
	f, ok := ext[key]
	if !ok || f.Value == nil {
		return pgtype.Numeric{}
	}

	s := ""
	switch v := f.Value.(type) {
	case string:
		s = strings.TrimSpace(v)
	case float64:
		s = fmt.Sprintf("%g", v)
	default:
		s = fmt.Sprintf("%v", v)
	}

	if s == "" {
		return pgtype.Numeric{}
	}

	rat, ok := new(big.Rat).SetString(s)
	if !ok {
		return pgtype.Numeric{}
	}
	// Express the value as integer mantissa * 10^exp so pgtype can encode it.
	// For decimal inputs the denominator is a power of ten; for non-decimal
	// rationals we scale until the denominator is 1 (losing precision beyond
	// what the column stores, which is acceptable for a GPA field).
	num := new(big.Int).Set(rat.Num())
	den := new(big.Int).Set(rat.Denom())
	exp := int32(0)

	ten := big.NewInt(10)
	for den.Cmp(big.NewInt(1)) > 0 {
		if new(big.Int).Mod(den, ten).Sign() != 0 {
			// Non-power-of-ten denominator: scale by 10 until integral.
			num.Mul(num, ten)

			exp--

			den.Quo(den, ten)

			continue
		}

		num.Mul(num, ten)

		exp--

		den.Quo(den, ten)
	}

	return pgtype.Numeric{Int: num, Exp: exp, Valid: true}
}

func jsonbField(ext extractor.Extraction, key string) []byte {
	f, ok := ext[key]
	if !ok || f.Value == nil {
		return nil
	}

	b, err := json.Marshal(f.Value)
	if err != nil {
		return nil
	}

	return b
}

// ---------------------------------------------------------------------------
// Comparison helpers
// ---------------------------------------------------------------------------

func strPtrEqual(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}

	return *a == *b
}

func stringSliceEqual(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}

	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func bytesEqual(a, b []byte) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}

	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func dateEqual(a, b pgtype.Date) bool {
	if !a.Valid && !b.Valid {
		return true
	}

	if a.Valid != b.Valid {
		return false
	}

	return a.Time.Equal(b.Time)
}

func numericEqual(a, b pgtype.Numeric) bool {
	if !a.Valid && !b.Valid {
		return true
	}

	if a.Valid != b.Valid {
		return false
	}

	return numericBigFloat(a).Cmp(numericBigFloat(b)) == 0
}

func numericBigFloat(n pgtype.Numeric) *big.Float {
	if !n.Valid || n.Int == nil {
		return new(big.Float)
	}

	f := new(big.Float).SetInt(n.Int)
	// Multiply by 10^exp.
	exp := int(n.Exp)
	base := big.NewFloat(1)
	ten := big.NewFloat(10)

	for range exp {
		base.Mul(base, ten)
	}

	for i := 0; i > exp; i-- {
		base.Quo(base, ten)
	}

	f.Mul(f, base)

	return f
}

// ---------------------------------------------------------------------------
// Editorial overrides + review log
// ---------------------------------------------------------------------------

// parseOverrideKeys decodes the editorial_overrides JSONB into the set of
// field names that are under human editorial control. A nil/empty blob means
// no overrides.
func parseOverrideKeys(raw []byte) (map[string]struct{}, error) {
	out := make(map[string]struct{})
	if len(raw) == 0 {
		return out, nil
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}

	for k := range m {
		out[k] = struct{}{}
	}

	return out, nil
}

// fieldChange is one entry in the intel_review_log.field_changes JSON array.
type fieldChange struct {
	Field string `json:"field"`
	From  any    `json:"from"`
	To    any    `json:"to"`
}

func marshalFieldChanges(existing gen.Scholarship, params gen.UpdateScholarshipParams, changed []string) ([]byte, error) {
	changes := make([]fieldChange, 0, len(changed))

	for _, c := range changed {
		fc := fieldChange{Field: c}
		fc.From, fc.To = snapshotField(existing, params, c)
		changes = append(changes, fc)
	}

	return json.Marshal(changes)
}

// snapshotField returns (before, after) values for a changed column name.
func snapshotField(existing gen.Scholarship, params gen.UpdateScholarshipParams, col string) (any, any) {
	switch col {
	case "title":
		return existing.Title, params.Title
	case "description":
		return derefStr(existing.Description), derefStr(params.Description)
	case "country":
		return existing.Country, params.Country
	case "university":
		return derefStr(existing.University), derefStr(params.University)
	case "degree_level":
		return derefStr(existing.DegreeLevel), derefStr(params.DegreeLevel)
	case "fields_of_study":
		return existing.FieldsOfStudy, params.FieldsOfStudy
	case "eligible_countries":
		return existing.EligibleCountries, params.EligibleCountries
	case "funding":
		return jsonOrNull(existing.Funding), jsonOrNull(params.Funding)
	case "deadline":
		return dateStr(existing.Deadline), dateStr(params.Deadline)
	case "application_url":
		return derefStr(existing.ApplicationUrl), derefStr(params.ApplicationUrl)
	case "required_documents":
		return existing.RequiredDocuments, params.RequiredDocuments
	case "minimum_gpa":
		return numericStr(existing.MinimumGpa), numericStr(params.MinimumGpa)
	case "language_requirements":
		return jsonOrNull(existing.LanguageRequirements), jsonOrNull(params.LanguageRequirements)
	}

	return nil, nil
}

func derefStr(s *string) any {
	if s == nil {
		return nil
	}

	return *s
}

func dateStr(d pgtype.Date) any {
	if !d.Valid {
		return nil
	}

	return d.Time.Format("2006-01-02")
}

func numericStr(n pgtype.Numeric) any {
	if !n.Valid {
		return nil
	}

	return numericBigFloat(n).Text('f', -1)
}

func jsonOrNull(b []byte) any {
	if len(b) == 0 {
		return nil
	}

	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return string(b)
	}

	return v
}

// ---------------------------------------------------------------------------
// Content body helpers
// ---------------------------------------------------------------------------

// bodyHTML renders a minimal HTML wrapper around the plain-text body. The
// body column is the source of truth; body_html is a best-effort render.
// Shared with other promoters (e.g. visa) that still create contents rows.
func bodyHTML(body string) string {
	if body == "" {
		return ""
	}

	return "<p>" + html.EscapeString(body) + "</p>"
}

// maxSlugAttempts bounds the number of suffix candidates tried when a
// generated scholarship title slug collides with an existing contents row.
const maxSlugAttempts = 100

// ensureUniqueSlug appends a numeric suffix to the candidate (-2, -3, ...) until
// no contents row in the tenant uses the same slug. The optional excludeID
// allows an existing content's own slug to be reused during an update. The
// check includes soft-deleted rows because the unique index on (tenant_id,
// slug) is not partial.
func (p *ScholarshipPromoter) ensureUniqueSlug(ctx context.Context, tx TxStore, slug string, excludeID pgtype.UUID) (string, error) {
	if slug == "" {
		return "", errors.New("promoter: cannot ensure unique empty slug")
	}

	base := slug
	for i := 2; i <= maxSlugAttempts+1; i++ {
		candidate := base
		if i > 2 {
			candidate = fmt.Sprintf("%s-%d", base, i-1)
		}

		exists, err := tx.ContentSlugExists(ctx, p.tenantID, candidate)
		if err != nil {
			return "", fmt.Errorf("promoter: checking slug %q: %w", candidate, err)
		}

		if !exists {
			return candidate, nil
		}
		// Allow the excluded content's own non-deleted slug to be reused (e.g.
		// title changed back to the same slug). Soft-deleted rows are not
		// returned by GetContentBySlug, and their slugs cannot be reused.
		if excludeID.Valid {
			row, err := tx.GetContentBySlug(ctx, p.tenantID, candidate)
			if err == nil && row.ID == excludeID {
				return candidate, nil
			}

			if err != nil && !isNotFound(err) {
				return "", fmt.Errorf("promoter: resolving slug %q: %w", candidate, err)
			}
		}
	}

	return "", fmt.Errorf("promoter: could not find unique slug after %d attempts", maxSlugAttempts)
}

// ---------------------------------------------------------------------------
// Misc helpers
// ---------------------------------------------------------------------------

func ptr[T any](v T) *T { return &v }

func emptyJSONObject() []byte { return []byte("{}") }

func uuidToPg(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func pgToUUID(id pgtype.UUID) uuid.UUID {
	if !id.Valid {
		return uuid.Nil
	}

	return uuid.UUID(id.Bytes)
}

// isNotFound reports whether err is a pgx no-rows error. We avoid importing
// pgx directly here by matching the error message, keeping the promoter
// package decoupled from the driver for mock-based tests.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}

	msg := err.Error()

	return strings.Contains(msg, "no rows in result set")
}
