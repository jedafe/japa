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
		overrideSource := overrideSourceIngestion
		intelStatus := "review_required"

		if autoPublish || hasReviewer {
			g, err := p.createScholarshipGuide(ctx, tx, record, source, ext, hasReviewer, reviewerID, countrySpaceID, now)
			if err != nil {
				return err
			}

			guideID = g

			verificationStatus = ActionVerified
			overrideSource = overrideSourceIngestion

			if hasReviewer {
				overrideSource = overrideSourceEditorial
			}

			intelStatus = statusPublished
		}

		scholarshipParams := buildCreateParams(record, source, guideID, ext, verificationStatus, overrideSource)
		scholarshipParams.CountrySpaceID = countrySpaceID

		scholarship, err := tx.CreateScholarship(ctx, p.tenantID, scholarshipParams)
		if err != nil {
			return fmt.Errorf("promoter: create scholarship: %w", err)
		}

		if intelStatus == statusPublished {
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

// createScholarshipGuide creates the published contents guide for a
// scholarship inside the promotion transaction and returns the content row
// ID to link through scholarships.guide_id.
func (p *ScholarshipPromoter) createScholarshipGuide(ctx context.Context, tx TxStore, record gen.IntelRecord, source gen.IntelSource, ext extractor.Extraction, hasReviewer bool, reviewerID, countrySpaceID pgtype.UUID, now time.Time) (pgtype.UUID, error) {
	title := stringField(ext, fieldTitle)
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
		return pgtype.UUID{}, fmt.Errorf("promoter: ensure unique slug on create: %w", err)
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
		Status:          statusPublished,
		Visibility:      "public",
		SeoIndexable:    true,
		ReadTimeMinutes: content.ComputeReadTime(bodyText),
		GuideCategory:   ptr("scholarship"),
		PublishAt:       pgtype.Timestamptz{Time: now, Valid: true},
		PublishedAt:     pgtype.Timestamptz{Time: now, Valid: true},
		AiGenerated:     true,
		AiModel:         p.aiModel,
		AiGeneratedAt:   pgtype.Timestamptz{Time: now, Valid: true},
		IntelSourceID:   source.ID,
		IntelRecordID:   record.ID,
	}
	if hasReviewer {
		contentParams.HitlReviewedBy = reviewerID
		contentParams.HitlReviewedAt = pgtype.Timestamptz{Time: now, Valid: true}
	}

	contentRow, err := tx.CreateContent(ctx, p.tenantID, contentParams)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("promoter: create guide content: %w", err)
	}

	return contentRow.ID, nil
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
	params.FreshnessScore = freshScore
	reviewerID, hasReviewer := reviewerctx.ReviewerFromContext(ctx)
	approving := hasReviewer && !existing.GuideID.Valid
	now := p.now()

	if approving {
		params.VerificationStatus = ActionVerified
		params.OverrideSource = overrideSourceForApproval(existing)

		changed = append(changed, "guide_id", "verification_status", "override_source")
	}

	if !approving && len(changed) == 0 {
		return p.bumpFreshness(ctx, existing, params)
	}

	var contentRow gen.GetContentByIDRow
	if existing.GuideID.Valid {
		contentRow, err = p.store.GetContentByID(ctx, p.tenantID, existing.GuideID)
		if err != nil {
			return PromoteResult{}, fmt.Errorf("promoter: load existing guide content: %w", err)
		}
	}

	reviewLogParams, err := p.buildAutoUpdateReviewLog(record, existing, params, changed, approving)
	if err != nil {
		return PromoteResult{}, err
	}

	var result PromoteResult

	err = p.store.WithTx(ctx, func(tx TxStore) error {
		var txErr error
		result, txErr = p.applyUpdateTx(ctx, tx, record, source, existing, ext, overrides, &params, reviewLogParams, contentRow, changed, approving, reviewerID, now)

		return txErr
	})

	return result, err
}

// applyUpdateTx runs the scholarship-update transaction body: guide
// creation on first approval, guide-content sync, the scholarship update,
// intel-record promotion marking, and the auto_update review log. It returns
// the PromoteResult to assign on success.
func (p *ScholarshipPromoter) applyUpdateTx(ctx context.Context, tx TxStore, record gen.IntelRecord, source gen.IntelSource, existing gen.Scholarship, ext extractor.Extraction, overrides map[string]struct{}, params *gen.UpdateScholarshipParams, reviewLogParams *gen.CreateIntelReviewLogParams, contentRow gen.GetContentByIDRow, changed []string, approving bool, reviewerID pgtype.UUID, now time.Time) (PromoteResult, error) {
	if approving {
		if err := p.createGuideOnApprove(ctx, tx, source, existing, ext, overrides, params, reviewerID, now); err != nil {
			return PromoteResult{}, err
		}
	}

	// If a published guide exists and source-derived title/body changed,
	// update the contents row while preserving any editorial overrides.
	if existing.GuideID.Valid {
		if err := p.syncGuideContent(ctx, tx, contentRow, existing, ext, overrides, *params, changed); err != nil {
			return PromoteResult{}, err
		}
	}

	if err := tx.UpdateScholarship(ctx, p.tenantID, *params); err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: update scholarship: %w", err)
	}

	if approving {
		if err := tx.UpdateIntelRecordPromoted(ctx, p.tenantID, gen.UpdateIntelRecordPromotedParams{
			ID:               record.ID,
			PromotedRecordID: existing.ID,
		}); err != nil {
			return PromoteResult{}, fmt.Errorf("promoter: mark intel record promoted on approve: %w", err)
		}
	}

	if reviewLogParams != nil {
		if err := tx.CreateIntelReviewLog(ctx, p.tenantID, *reviewLogParams); err != nil {
			return PromoteResult{}, fmt.Errorf("promoter: write auto_update review log: %w", err)
		}
	}

	action := ActionUpdated
	if !approving && len(changed) == 0 {
		action = ActionVerified
	}

	return PromoteResult{
		DomainID:      pgToUUID(existing.ID),
		ContentID:     pgToUUID(params.GuideID),
		Action:        action,
		ChangedFields: changed,
	}, nil
}

// bumpFreshness re-stamps the scholarship row at full freshness when a
// re-crawl produced no field-level changes — the auto_update review log is
// skipped, only last_verified_at + freshness_score move.
func (p *ScholarshipPromoter) bumpFreshness(ctx context.Context, existing gen.Scholarship, params gen.UpdateScholarshipParams) (PromoteResult, error) {
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

// buildAutoUpdateReviewLog builds the review-log params for a non-approving
// re-crawl that produced field changes; it returns nil when no log is needed.
func (p *ScholarshipPromoter) buildAutoUpdateReviewLog(record gen.IntelRecord, existing gen.Scholarship, params gen.UpdateScholarshipParams, changed []string, approving bool) (*gen.CreateIntelReviewLogParams, error) {
	if approving || len(changed) == 0 {
		return nil, nil
	}

	changesJSON, err := marshalFieldChanges(existing, params, changed)
	if err != nil {
		return nil, fmt.Errorf("promoter: marshal field changes: %w", err)
	}

	note := noteAutoRecrawl

	return &gen.CreateIntelReviewLogParams{
		IntelRecordID: record.ID,
		ActorUserID:   p.authorID,
		Action:        "auto_update",
		FieldChanges:  changesJSON,
		Note:          &note,
	}, nil
}

// createGuideOnApprove creates the published guide contents row on first
// reviewer approval and stamps params.GuideID to the new row.
func (p *ScholarshipPromoter) createGuideOnApprove(ctx context.Context, tx TxStore, source gen.IntelSource, existing gen.Scholarship, ext extractor.Extraction, overrides map[string]struct{}, params *gen.UpdateScholarshipParams, reviewerID pgtype.UUID, now time.Time) error {
	mergedExt := bodyExtractionFrom(existing, ext, overrides)
	bodyText := buildScholarshipBody(mergedExt)

	slug := content.Slugify(params.Title)
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
		Title:           params.Title,
		Slug:            slug,
		Body:            bodyText,
		BodyHtml:        bodyHTML(bodyText),
		FieldValues:     []byte(`{}`),
		ContentHash:     &hash,
		GeoSpaceID:      params.CountrySpaceID,
		CanonicalUrl:    &canonicalURL,
		Status:          statusPublished,
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

	return nil
}

// syncGuideContent updates the published guide contents row when
// source-derived title/body changed, preserving editorial overrides.
func (p *ScholarshipPromoter) syncGuideContent(ctx context.Context, tx TxStore, contentRow gen.GetContentByIDRow, existing gen.Scholarship, ext extractor.Extraction, overrides map[string]struct{}, params gen.UpdateScholarshipParams, changed []string) error {
	newTitle := contentRow.Title
	if sliceContains(changed, fieldTitle) && !isOverridden(overrides, fieldTitle) {
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
	if !needsUpdate {
		return nil
	}

	if _, err := tx.UpdateContent(ctx, p.tenantID, buildContentUpdateParams(contentRow, newTitle, newSlug, newBody, newHash)); err != nil {
		return fmt.Errorf("promoter: update guide content: %w", err)
	}

	return nil
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
		Title:                stringField(ext, fieldTitle),
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

	applyStrField(overrides, ext, fieldTitle, &params.Title, existing.Title, &changed)
	// description (description_summary, description, eligibility, abstract)
	applyStrPtrFieldFn(overrides, "description", func() *string { return descriptionField(ext) }, &params.Description, existing.Description, &changed)
	applyStrFieldFn(overrides, "country", func() string { return firstCountry(ext, "country") }, &params.Country, existing.Country, &changed)
	applyStrPtrField(overrides, ext, "university", &params.University, existing.University, &changed)
	applyStrPtrField(overrides, ext, "degree_level", &params.DegreeLevel, existing.DegreeLevel, &changed)
	applyStrSliceField(overrides, ext, "fields_of_study", &params.FieldsOfStudy, existing.FieldsOfStudy, &changed)
	applyStrSliceField(overrides, ext, "eligible_countries", &params.EligibleCountries, existing.EligibleCountries, &changed)
	applyJSONField(overrides, ext, "funding", &params.Funding, existing.Funding, &changed)
	applyDateField(overrides, ext, "deadline", &params.Deadline, existing.Deadline, &changed)
	applyStrPtrField(overrides, ext, "application_url", &params.ApplicationUrl, existing.ApplicationUrl, &changed)
	applyStrSliceField(overrides, ext, "required_documents", &params.RequiredDocuments, existing.RequiredDocuments, &changed)
	applyNumericField(overrides, ext, "minimum_gpa", &params.MinimumGpa, existing.MinimumGpa, &changed)
	applyJSONField(overrides, ext, "language_requirements", &params.LanguageRequirements, existing.LanguageRequirements, &changed)

	return params, changed
}

// ---------------------------------------------------------------------------
// Update-params field applicators
//
// Each helper applies a single extraction field to a params struct: skipped
// when the field is in editorial_overrides, and only assigned when the new
// value differs from the existing column. Changed fields are recorded in
// `changed` so the caller can build an accurate review-log entry.
// ---------------------------------------------------------------------------

// applyStrField applies a plain-string extraction field.
func applyStrField(overrides map[string]struct{}, ext extractor.Extraction, key string, dst *string, current string, changed *[]string) {
	applyStrFieldFn(overrides, key, func() string { return stringField(ext, key) }, dst, current, changed)
}

// applyStrFieldFn applies a string extraction via a custom getter (e.g.
// firstCountry) used when the field needs more than a direct stringField read.
func applyStrFieldFn(overrides map[string]struct{}, key string, get func() string, dst *string, current string, changed *[]string) {
	if _, ov := overrides[key]; ov {
		return
	}

	if v := get(); v != "" && v != current {
		*dst = v

		*changed = append(*changed, key)
	}
}

// applyStrPtrField applies a *string extraction field.
func applyStrPtrField(overrides map[string]struct{}, ext extractor.Extraction, key string, dst **string, current *string, changed *[]string) {
	applyStrPtrFieldFn(overrides, key, func() *string { return stringPtrField(ext, key) }, dst, current, changed)
}

// applyStrPtrFieldFn applies a *string extraction via a custom getter.
func applyStrPtrFieldFn(overrides map[string]struct{}, key string, get func() *string, dst **string, current *string, changed *[]string) {
	if _, ov := overrides[key]; ov {
		return
	}

	if v := get(); v != nil && !strPtrEqual(v, current) {
		*dst = v

		*changed = append(*changed, key)
	}
}

// applyStrSliceField applies a []string extraction field.
func applyStrSliceField(overrides map[string]struct{}, ext extractor.Extraction, key string, dst *[]string, current []string, changed *[]string) {
	applyStrSliceFieldFn(overrides, key, func() []string { return stringSliceField(ext, key) }, dst, current, changed)
}

// applyStrSliceFieldFn applies a []string extraction via a custom getter.
func applyStrSliceFieldFn(overrides map[string]struct{}, key string, get func() []string, dst *[]string, current []string, changed *[]string) {
	if _, ov := overrides[key]; ov {
		return
	}

	if v := get(); v != nil && !stringSliceEqual(v, current) {
		*dst = orEmptySlice(v)

		*changed = append(*changed, key)
	}
}

// applyJSONField applies a JSONB extraction field.
func applyJSONField(overrides map[string]struct{}, ext extractor.Extraction, key string, dst *[]byte, current []byte, changed *[]string) {
	applyJSONFieldFn(overrides, key, func() []byte { return jsonbField(ext, key) }, dst, current, changed)
}

// applyJSONFieldFn applies a JSONB extraction via a custom getter, used when
// the extraction key differs from the override/column key.
func applyJSONFieldFn(overrides map[string]struct{}, key string, get func() []byte, dst *[]byte, current []byte, changed *[]string) {
	if _, ov := overrides[key]; ov {
		return
	}

	if v := get(); v != nil && !bytesEqual(v, current) {
		*dst = orEmptyJSON(v)

		*changed = append(*changed, key)
	}
}

// applyDateField applies a date extraction field.
func applyDateField(overrides map[string]struct{}, ext extractor.Extraction, key string, dst *pgtype.Date, current pgtype.Date, changed *[]string) {
	applyDateFieldFn(overrides, key, func() pgtype.Date { return dateField(ext, key) }, dst, current, changed)
}

// applyDateFieldFn applies a date extraction via a custom getter.
func applyDateFieldFn(overrides map[string]struct{}, key string, get func() pgtype.Date, dst *pgtype.Date, current pgtype.Date, changed *[]string) {
	if _, ov := overrides[key]; ov {
		return
	}

	if v := get(); v.Valid && !dateEqual(v, current) {
		*dst = v

		*changed = append(*changed, key)
	}
}

// applyNumericField applies a numeric extraction field.
func applyNumericField(overrides map[string]struct{}, ext extractor.Extraction, key string, dst *pgtype.Numeric, current pgtype.Numeric, changed *[]string) {
	applyNumericFieldFn(overrides, key, func() pgtype.Numeric { return numericField(ext, key) }, dst, current, changed)
}

// applyNumericFieldFn applies a numeric extraction via a custom getter.
func applyNumericFieldFn(overrides map[string]struct{}, key string, get func() pgtype.Numeric, dst *pgtype.Numeric, current pgtype.Numeric, changed *[]string) {
	if _, ov := overrides[key]; ov {
		return
	}

	if v := get(); v.Valid && !numericEqual(v, current) {
		*dst = v

		*changed = append(*changed, key)
	}
}

// applyBoolField applies a bool extraction field to a non-pointer column.
func applyBoolField(overrides map[string]struct{}, key string, get func() *bool, dst *bool, current bool, changed *[]string) {
	if _, ov := overrides[key]; ov {
		return
	}

	if v := get(); v != nil && *v != current {
		*dst = *v

		*changed = append(*changed, key)
	}
}

// applyBoolPtrField applies a *bool extraction field to a pointer column.
func applyBoolPtrField(overrides map[string]struct{}, key string, get func() *bool, dst **bool, current *bool, changed *[]string) {
	if _, ov := overrides[key]; ov {
		return
	}

	if v := get(); v != nil && !boolPtrEqual(v, current) {
		*dst = v

		*changed = append(*changed, key)
	}
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
	if t := stringField(ext, fieldTitle); t != "" {
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

	// title: an override only pins the existing value when it's non-empty —
	// an override on an empty existing title still lets the extraction through.
	titleOverrides := overrides
	if existing.Title == "" {
		titleOverrides = map[string]struct{}{}
	}

	mergeExtractedField(out, titleOverrides, fieldTitle, fieldTitle,
		strVal(existing.Title), extStr(ext, fieldTitle))
	// description (extraction key: description_summary)
	mergeExtractedField(out, overrides, "description", "description_summary",
		strPtrVal(existing.Description), extDescription(ext))
	mergeExtractedField(out, overrides, "university", "university",
		strPtrVal(existing.University), extStrPtr(ext, "university"))
	mergeExtractedField(out, overrides, "degree_level", "degree_level",
		strPtrVal(existing.DegreeLevel), extStrPtr(ext, "degree_level"))
	mergeExtractedField(out, overrides, "deadline", "deadline",
		dateVal(existing.Deadline), extDate(ext, "deadline"))

	// funding
	if v := bodyFundingValue(existing, ext, overrides); v != nil {
		out["funding"] = extractor.FieldExtraction{Value: v, Confidence: 1}
	}

	mergeExtractedField(out, overrides, "eligible_countries", "eligible_countries",
		strSliceVal(existing.EligibleCountries), extStrSlice(ext, "eligible_countries"))
	mergeExtractedField(out, overrides, "application_url", "application_url",
		strPtrVal(existing.ApplicationUrl), extStrPtr(ext, "application_url"))

	return out
}

// mergeExtractedField applies the override → extraction → existing precedence
// used by bodyExtractionFrom: an editorial override pins the existing value, a
// fresh extraction wins next, and the existing value is the final fallback.
func mergeExtractedField(out extractor.Extraction, overrides map[string]struct{}, overrideKey, outKey string, existing, extracted func() (any, bool)) {
	if _, ov := overrides[overrideKey]; !ov {
		if v, ok := extracted(); ok {
			out[outKey] = extractor.FieldExtraction{Value: v, Confidence: 1}

			return
		}
	}

	if v, ok := existing(); ok {
		out[outKey] = extractor.FieldExtraction{Value: v, Confidence: 1}
	}
}

// strVal adapts a plain string to the mergeExtractedField value signature.
func strVal(s string) func() (any, bool) {
	return func() (any, bool) {
		return s, s != ""
	}
}

// strPtrVal adapts a *string to the mergeExtractedField value signature.
func strPtrVal(p *string) func() (any, bool) {
	return func() (any, bool) {
		if p != nil && *p != "" {
			return *p, true
		}

		return nil, false
	}
}

// strSliceVal adapts a []string to the mergeExtractedField value signature.
func strSliceVal(s []string) func() (any, bool) {
	return func() (any, bool) {
		return s, len(s) > 0
	}
}

// dateVal adapts a pgtype.Date to the mergeExtractedField value signature,
// rendering valid dates as ISO strings.
func dateVal(d pgtype.Date) func() (any, bool) {
	return func() (any, bool) {
		if d.Valid {
			return d.Time.Format("2006-01-02"), true
		}

		return nil, false
	}
}

// extStr reads a non-empty string extraction for key.
func extStr(ext extractor.Extraction, key string) func() (any, bool) {
	return func() (any, bool) {
		if f, ok := ext[key]; ok && f.Value != nil {
			if s := stringField(ext, key); s != "" {
				return s, true
			}
		}

		return nil, false
	}
}

// extStrPtr reads a *string extraction for key.
func extStrPtr(ext extractor.Extraction, key string) func() (any, bool) {
	return func() (any, bool) {
		if f, ok := ext[key]; ok && f.Value != nil {
			if s := stringPtrField(ext, key); s != nil {
				return *s, true
			}
		}

		return nil, false
	}
}

// extStrSlice reads a non-empty []string extraction for key.
func extStrSlice(ext extractor.Extraction, key string) func() (any, bool) {
	return func() (any, bool) {
		if f, ok := ext[key]; ok && f.Value != nil {
			if s := stringSliceField(ext, key); len(s) > 0 {
				return s, true
			}
		}

		return nil, false
	}
}

// extDate reads a valid date extraction for key, rendered as ISO.
func extDate(ext extractor.Extraction, key string) func() (any, bool) {
	return func() (any, bool) {
		if f, ok := ext[key]; ok && f.Value != nil {
			if d := dateField(ext, key); d.Valid {
				return d.Time.Format("2006-01-02"), true
			}
		}

		return nil, false
	}
}

// extDescription reads the description extraction (descriptionField handles
// the description_summary key variants internally).
func extDescription(ext extractor.Extraction) func() (any, bool) {
	return func() (any, bool) {
		if d := descriptionField(ext); d != nil {
			return *d, true
		}

		return nil, false
	}
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
	if existing.OverrideSource == overrideSourceEditorial || existing.OverrideSource == "mixed" {
		return "mixed"
	}

	return overrideSourceEditorial
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

// scholarshipFieldAccessors maps each changeable scholarship column to a
// function returning its (before, after) values for the auto_update review
// log.
var scholarshipFieldAccessors = map[string]func(e gen.Scholarship, p gen.UpdateScholarshipParams) (any, any){
	fieldTitle: func(e gen.Scholarship, p gen.UpdateScholarshipParams) (any, any) { return e.Title, p.Title },
	"description": func(e gen.Scholarship, p gen.UpdateScholarshipParams) (any, any) {
		return derefStr(e.Description), derefStr(p.Description)
	},
	"country": func(e gen.Scholarship, p gen.UpdateScholarshipParams) (any, any) { return e.Country, p.Country },
	"university": func(e gen.Scholarship, p gen.UpdateScholarshipParams) (any, any) {
		return derefStr(e.University), derefStr(p.University)
	},
	"degree_level": func(e gen.Scholarship, p gen.UpdateScholarshipParams) (any, any) {
		return derefStr(e.DegreeLevel), derefStr(p.DegreeLevel)
	},
	"fields_of_study": func(e gen.Scholarship, p gen.UpdateScholarshipParams) (any, any) {
		return e.FieldsOfStudy, p.FieldsOfStudy
	},
	"eligible_countries": func(e gen.Scholarship, p gen.UpdateScholarshipParams) (any, any) {
		return e.EligibleCountries, p.EligibleCountries
	},
	"funding": func(e gen.Scholarship, p gen.UpdateScholarshipParams) (any, any) {
		return jsonOrNull(e.Funding), jsonOrNull(p.Funding)
	},
	"deadline": func(e gen.Scholarship, p gen.UpdateScholarshipParams) (any, any) {
		return dateStr(e.Deadline), dateStr(p.Deadline)
	},
	"application_url": func(e gen.Scholarship, p gen.UpdateScholarshipParams) (any, any) {
		return derefStr(e.ApplicationUrl), derefStr(p.ApplicationUrl)
	},
	"required_documents": func(e gen.Scholarship, p gen.UpdateScholarshipParams) (any, any) {
		return e.RequiredDocuments, p.RequiredDocuments
	},
	"minimum_gpa": func(e gen.Scholarship, p gen.UpdateScholarshipParams) (any, any) {
		return numericStr(e.MinimumGpa), numericStr(p.MinimumGpa)
	},
	"language_requirements": func(e gen.Scholarship, p gen.UpdateScholarshipParams) (any, any) {
		return jsonOrNull(e.LanguageRequirements), jsonOrNull(p.LanguageRequirements)
	},
}

// snapshotField returns (before, after) values for a changed column name.
func snapshotField(existing gen.Scholarship, params gen.UpdateScholarshipParams, col string) (any, any) {
	if get, ok := scholarshipFieldAccessors[col]; ok {
		return get(existing, params)
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
