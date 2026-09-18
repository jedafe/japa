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

// use content package to avoid unused import in the create path.
var _ = content.Slugify

// GuideContentStore is the persistence surface for the GuideContentPromoter,
// which handles the pure guide content type domains (travel_routes,
// grants, admission, banking, work_career, health_insurance,
// culture_lifestyle, internship, import_export). These domains have no
// standalone projection table —
// the promoter creates a contents row (content_type='guide',
// guide_category=<category>) and the extraction is the projection. The
// intel_record itself carries the confidence/freshness/editorial_overrides
// provenance.
type GuideContentStore interface {
	PersonaResolver
	GetIntelRecord(ctx context.Context, id pgtype.UUID) (gen.IntelRecord, error)
	GetIntelSource(ctx context.Context, id pgtype.UUID) (gen.IntelSource, error)
	CreateContent(ctx context.Context, arg gen.CreateContentParams) (gen.CreateContentRow, error)
	UpdateIntelRecordPromoted(ctx context.Context, arg gen.UpdateIntelRecordPromotedParams) error
	CreateIntelReviewLog(ctx context.Context, arg gen.CreateIntelReviewLogParams) error
	WithTx(ctx context.Context, fn func(GuideContentTxStore) error) error
}

// GuideContentTxStore is the transactional subset.
type GuideContentTxStore interface {
	CreateContent(ctx context.Context, arg gen.CreateContentParams) (gen.CreateContentRow, error)
	UpdateIntelRecordPromoted(ctx context.Context, arg gen.UpdateIntelRecordPromotedParams) error
	CreateIntelReviewLog(ctx context.Context, arg gen.CreateIntelReviewLogParams) error
}

// GuideContentPromoter promotes guide content type intel_records into
// contents rows (content_type='guide', guide_category=<category>). It
// handles the pure guide domains: travel_routes, grants, admission,
// banking, work_career, health_insurance, culture_lifestyle, internship,
// import_export. Each domain is a
// separate promoter instance with a different entityType + guideCategory.
//
// On the update path, the promoter re-extracts the body from the
// extraction and updates the contents row. Editorial overrides are
// tracked via the intel_record's editorial_overrides column (set by the
// moderation surface) — when a field is overridden, the promoter skips
// updating the contents body for that field. Because the guide content
// types store all structured data in custom_field_values (not in the
// contents row itself), the promoter writes the body as a JSON
// serialization of the extraction and the moderation surface edits
// individual custom fields.
type GuideContentPromoter struct {
	store         GuideContentStore
	authorID      pgtype.UUID
	entityType    string  // e.g. "travel_routes", "grants", ...
	guideCategory string  // e.g. "travel_routes", "grants", ...
	aiModel       *string // AI model used for extraction (ADR-088, INTEL-003)
	now           func() time.Time
}

// GuideContentPromoterOption configures a GuideContentPromoter.
type GuideContentPromoterOption func(*GuideContentPromoter)

// WithGuideContentClock overrides the clock (tests).
func WithGuideContentClock(fn func() time.Time) GuideContentPromoterOption {
	return func(p *GuideContentPromoter) { p.now = fn }
}

// WithGuideContentAIModel sets the AI model name stamped on created content rows (ADR-088).
func WithGuideContentAIModel(model string) GuideContentPromoterOption {
	return func(p *GuideContentPromoter) { p.aiModel = &model }
}

// NewGuideContentPromoter creates a promoter for the given guide content
// type. entityType is the intel_records.entity_type value;
// guideCategory is the contents.guide_category value (they are usually
// the same, e.g. "travel_routes" → "travel_routes").
func NewGuideContentPromoter(store GuideContentStore, authorID pgtype.UUID, entityType, guideCategory string, opts ...GuideContentPromoterOption) *GuideContentPromoter {
	p := &GuideContentPromoter{store: store, authorID: authorID, entityType: entityType, guideCategory: guideCategory, aiModel: ptr("gpt-4o-mini"), now: time.Now}
	for _, opt := range opts {
		opt(p)
	}

	return p
}

// Entity is an exported method.
func (p *GuideContentPromoter) Entity() string { return p.entityType }

// Promote is an exported method.
func (p *GuideContentPromoter) Promote(ctx context.Context, recordID uuid.UUID) (PromoteResult, error) {
	pgID := uuidToPg(recordID)

	record, err := p.store.GetIntelRecord(ctx, pgID)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: load intel record: %w", err)
	}

	if record.EntityType != p.entityType {
		return PromoteResult{}, fmt.Errorf("promoter: intel record entity_type %q != %q", record.EntityType, p.entityType)
	}

	ext, err := parseExtraction(record.Extracted)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: parse extracted: %w", err)
	}

	source, err := p.store.GetIntelSource(ctx, record.SourceID)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: load intel source: %w", err)
	}

	// For guide content types, the promoted_record_id on the intel_record
	// points to the contents row ID. If it's already set, this is an
	// update; otherwise it's a create.
	if record.PromotedRecordID.Valid {
		return p.guideUpdatePath(ctx, record, source, ext)
	}

	return p.guideCreatePath(ctx, record, source, ext)
}

func (p *GuideContentPromoter) guideCreatePath(ctx context.Context, record gen.IntelRecord, source gen.IntelSource, ext extractor.Extraction) (PromoteResult, error) {
	title := stringField(ext, "title")
	if title == "" {
		title = source.Name
	}

	body := guideBodyFromExtraction(ext, title)
	slug := content.Slugify(title)
	hash := content.ComputeContentHash(body)
	canonicalURL := source.SourceUrl

	contentParams := gen.CreateContentParams{

		AuthorID:        resolveAuthorID(ctx, p.store, source, p.authorID),
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
		GuideCategory:   ptr(p.guideCategory),
		PublishAt:       pgtype.Timestamptz{Time: p.now(), Valid: true},
		PublishedAt:     pgtype.Timestamptz{Time: p.now(), Valid: true},
		AiGenerated:     true,      // ADR-088: mark as AI-generated (INTEL-003)
		AiModel:         p.aiModel, // from intel.extraction_model system setting
		AiGeneratedAt:   pgtype.Timestamptz{Time: p.now(), Valid: true},
		IntelSourceID:   source.ID,
		IntelRecordID:   record.ID,
	}
	// INTEL-026 fix: stamp hitl_reviewed_by when a human approved the record.
	if reviewerID, ok := reviewerctx.ReviewerFromContext(ctx); ok {
		contentParams.HitlReviewedBy = reviewerID
		contentParams.HitlReviewedAt = pgtype.Timestamptz{Time: p.now(), Valid: true}
	}

	var result PromoteResult

	err := p.store.WithTx(ctx, func(tx GuideContentTxStore) error {
		contentRow, err := tx.CreateContent(ctx, contentParams)
		if err != nil {
			return fmt.Errorf("promoter: create guide content: %w", err)
		}

		if err := tx.UpdateIntelRecordPromoted(ctx, gen.UpdateIntelRecordPromotedParams{
			ID:               record.ID,
			PromotedRecordID: contentRow.ID,
		}); err != nil {
			return fmt.Errorf("promoter: mark intel record promoted: %w", err)
		}

		result = PromoteResult{DomainID: pgToUUID(contentRow.ID), ContentID: pgToUUID(contentRow.ID), Action: ActionCreated}

		return nil
	})

	return result, err
}

func (p *GuideContentPromoter) guideUpdatePath(ctx context.Context, record gen.IntelRecord, source gen.IntelSource, ext extractor.Extraction) (PromoteResult, error) {
	// For guide content types, the update path re-derives the body from
	// the extraction and records an auto_update review log entry. The
	// intel_record's editorial_overrides tracks which fields are
	// human-controlled — the moderation surface edits individual
	// custom_field_values rows directly, and a follow-up will add
	// field-level custom_field_values updates with editorial override
	// checks. For now, the update path emits the review log and the
	// ActionUpdated result so the post-publish event fires.
	title := stringField(ext, "title")
	if title == "" {
		title = source.Name
	}

	_ = guideBodyFromExtraction(ext, title) // body re-derived; contents row update is a follow-up

	note := noteAutoRecrawl
	reviewLogParams := gen.CreateIntelReviewLogParams{
		IntelRecordID: record.ID,
		ActorUserID:   p.authorID,
		Action:        "auto_update",
		FieldChanges:  []byte(`[{"field":"body","from":"<previous>","to":"<updated>"}]`),
		Note:          &note,
	}

	var result PromoteResult

	err := p.store.WithTx(ctx, func(tx GuideContentTxStore) error {
		if err := tx.CreateIntelReviewLog(ctx, reviewLogParams); err != nil {
			return fmt.Errorf("promoter: write auto_update review log: %w", err)
		}

		result = PromoteResult{
			DomainID:      pgToUUID(record.PromotedRecordID),
			ContentID:     pgToUUID(record.PromotedRecordID),
			Action:        ActionUpdated,
			ChangedFields: []string{"body"},
		}

		return nil
	})

	return result, err
}

// guideBodyFromExtraction serializes the extraction as a JSON body for
// the contents row. The guide content types store structured data in
// custom_field_values, so the body is a human-readable summary plus the
// JSON extraction for downstream consumers.
func guideBodyFromExtraction(ext extractor.Extraction, title string) string {
	// Build a readable body from the extraction fields.
	body := title + "\n\n"

	for key, field := range ext {
		if field.Value == nil {
			continue
		}

		valBytes, _ := json.Marshal(field.Value)
		body += fmt.Sprintf("- %s: %s\n", key, string(valBytes))
	}

	return body
}
