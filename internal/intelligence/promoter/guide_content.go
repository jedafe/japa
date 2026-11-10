package promoter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/extractor"
	"github.com/jedafe/japa/internal/intelligence/reviewerctx"
)

// use content package to avoid unused import in the create path.
var _ = content.Slugify

// extractedFieldValues flattens a validated extraction to a {name: value}
// map for contents.field_values — the structured-data column guide types
// render from (spec: "all structured data in custom_field_values"). Null
// fields are omitted so only evidence-backed values persist. Prior code
// wrote '{}' — every extracted schema field was lost on promote.
func extractedFieldValues(ext extractor.Extraction) []byte {
	out := make(map[string]any, len(ext))
	for name, fe := range ext {
		if fe.Value != nil {
			out[name] = fe.Value
		}
	}

	b, err := json.Marshal(out)
	if err != nil || len(out) == 0 {
		return []byte(`{}`)
	}

	return b
}

// GuideContentStore is the persistence surface for the GuideContentPromoter,
// which handles the pure guide content type domains (travel_routes,
// grants, admission, banking, work_career, health_insurance,
// culture_lifestyle, internship, import_export). These domains have no
// standalone projection table —
// the promoter creates a contents row (content_type='guide',
// guide_category=<category>) and the extraction is the projection. The
// intel_record itself carries the confidence/freshness provenance.
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
	GetContentByID(ctx context.Context, id pgtype.UUID) (gen.GetContentByIDRow, error)
	UpdateContent(ctx context.Context, arg gen.UpdateContentParams) (gen.UpdateContentRow, error)
	UpdateIntelRecordPromoted(ctx context.Context, arg gen.UpdateIntelRecordPromotedParams) error
	CreateIntelReviewLog(ctx context.Context, arg gen.CreateIntelReviewLogParams) error
}

// GuideContentPromoter promotes content-projection intel_records into
// contents rows (content_type='guide', guide_category=<category>). It
// handles the pure guide domains: travel_routes, grants, admission,
// banking, work_career, health_insurance, culture_lifestyle, internship,
// import_export — and, via WithGuideContentType, the 'news' domain which
// promotes to content_type='news' with no guide_category. Each domain
// is a separate promoter instance with a different entityType +
// guideCategory (+ contentType when not 'guide').
//
// On the update path, the promoter re-derives title/body/field_values/
// canonical_url from the extraction and writes them back to the contents
// row while preserving editorial columns (slug, status, images, meta/seo,
// publish stamps). contents has no per-field override tracking, so the
// guard rails are the is_locked flag (locked rows are skipped entirely)
// and the auto_update review log, which records the real field-level diff.
type GuideContentPromoter struct {
	store         GuideContentStore
	authorID      pgtype.UUID
	entityType    string  // e.g. "travel_routes", "grants", ...
	guideCategory string  // e.g. "travel_routes", "grants", ... (empty = none)
	contentType   string  // contents.content_type to write; default "guide"
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

// WithGuideContentType overrides the contents.content_type written on
// promotion. Used by non-guide domains that still project to a contents row —
// e.g. the 'news' entity_type promotes to content_type='news' so
// extracted articles surface on the source feed and /content/{slug}.
func WithGuideContentType(contentType string) GuideContentPromoterOption {
	return func(p *GuideContentPromoter) { p.contentType = contentType }
}

// NewGuideContentPromoter creates a promoter for the given guide content
// type. entityType is the intel_records.entity_type value;
// guideCategory is the contents.guide_category value (they are usually
// the same, e.g. "travel_routes" → "travel_routes"). Pass an empty
// guideCategory with WithGuideContentType for non-guide domains.
func NewGuideContentPromoter(store GuideContentStore, authorID pgtype.UUID, entityType, guideCategory string, opts ...GuideContentPromoterOption) *GuideContentPromoter {
	p := &GuideContentPromoter{store: store, authorID: authorID, entityType: entityType, guideCategory: guideCategory, contentType: "guide", aiModel: ptr("gpt-4o-mini"), now: time.Now}
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
	title := guideTitle(ext, source)
	contentParams := p.buildCreateContentParams(ctx, record, source, ext, title)

	var result PromoteResult

	err := p.store.WithTx(ctx, func(tx GuideContentTxStore) error {
		contentRow, err := tx.CreateContent(ctx, contentParams)
		if err != nil {
			return fmt.Errorf("promoter: create guide content: %w", err)
		}

		if err := syncCreateMediaRefs(ctx, tx, contentRow); err != nil {
			return fmt.Errorf("promoter: sync media fields: %w", err)
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

// guideTitle resolves the extraction title, falling back to the source name.
func guideTitle(ext extractor.Extraction, source gen.IntelSource) string {
	title := stringField(ext, "title")
	if title == "" {
		title = source.Name
	}

	return title
}

// buildCreateContentParams assembles the contents-row insert for a guide
// promotion — shared by the create path and the update path's
// recreate-when-missing branch.
func (p *GuideContentPromoter) buildCreateContentParams(ctx context.Context, record gen.IntelRecord, source gen.IntelSource, ext extractor.Extraction, title string) gen.CreateContentParams {
	body := guideBodyFromExtraction(ext, title)
	hash := content.ComputeContentHash(body)
	// Prefer the extraction's own article link (news schema carries
	// article_url — the per-item URL is a better canonical than the feed
	// URL); fall back to the source URL when absent.
	canonicalURL := source.SourceUrl
	if u := stringField(ext, "article_url"); u != "" {
		canonicalURL = u
	}

	// guide_category only applies to the 'guide' content type; other
	// content-projection domains (e.g. news) leave it NULL.
	var guideCategory *string
	if p.guideCategory != "" {
		guideCategory = &p.guideCategory
	}

	params := gen.CreateContentParams{
		AuthorID:        resolveAuthorID(ctx, p.store, source, p.authorID),
		FieldValues:     extractedFieldValues(ext),
		ContentType:     p.contentType,
		Title:           title,
		Slug:            content.Slugify(title),
		Body:            body,
		BodyHtml:        "<p>" + html.EscapeString(body) + "</p>",
		ContentHash:     &hash,
		CanonicalUrl:    &canonicalURL,
		Status:          "published",
		Visibility:      "public",
		SeoIndexable:    true,
		ReadTimeMinutes: content.ComputeReadTime(body),
		GuideCategory:   guideCategory,
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
		params.HitlReviewedBy = reviewerID
		params.HitlReviewedAt = pgtype.Timestamptz{Time: p.now(), Valid: true}
	}

	return params
}

func (p *GuideContentPromoter) guideUpdatePath(ctx context.Context, record gen.IntelRecord, source gen.IntelSource, ext extractor.Extraction) (PromoteResult, error) {
	title := guideTitle(ext, source)
	body := guideBodyFromExtraction(ext, title)
	hash := content.ComputeContentHash(body)
	fieldValues := extractedFieldValues(ext)
	canonicalURL := source.SourceUrl
	if u := stringField(ext, "article_url"); u != "" {
		canonicalURL = u
	}

	var result PromoteResult

	err := p.store.WithTx(ctx, func(tx GuideContentTxStore) error {
		existing, gerr := tx.GetContentByID(ctx, record.PromotedRecordID)
		if errors.Is(gerr, pgx.ErrNoRows) {
			// The promoted contents row was deleted — recreate it so the
			// record still resolves to a live contents row.
			created, cerr := tx.CreateContent(ctx, p.buildCreateContentParams(ctx, record, source, ext, title))
			if cerr != nil {
				return fmt.Errorf("promoter: recreate guide content: %w", cerr)
			}

			if err := syncCreateMediaRefs(ctx, tx, created); err != nil {
				return fmt.Errorf("promoter: sync media fields: %w", err)
			}

			if err := tx.UpdateIntelRecordPromoted(ctx, gen.UpdateIntelRecordPromotedParams{
				ID:               record.ID,
				PromotedRecordID: created.ID,
			}); err != nil {
				return fmt.Errorf("promoter: mark intel record promoted: %w", err)
			}

			result = PromoteResult{DomainID: pgToUUID(created.ID), ContentID: pgToUUID(created.ID), Action: ActionCreated}

			return nil
		}

		if gerr != nil {
			return fmt.Errorf("promoter: load guide content: %w", gerr)
		}

		// An editor-locked row is never auto-updated — the lock is the
		// moderation surface's "human owns this content" marker (there is
		// no per-field override tracking on contents).
		if existing.IsLocked {
			note := "auto_update skipped: contents row is editor-locked"
			if lerr := tx.CreateIntelReviewLog(ctx, gen.CreateIntelReviewLogParams{
				IntelRecordID: record.ID,
				ActorUserID:   p.authorID,
				Action:        "auto_update",
				FieldChanges:  []byte(`[]`),
				Note:          &note,
			}); lerr != nil {
				return fmt.Errorf("promoter: write lock-skip review log: %w", lerr)
			}

			result = PromoteResult{DomainID: pgToUUID(existing.ID), ContentID: pgToUUID(existing.ID), Action: ActionVerified}

			return nil
		}

		// A re-crawl that yields no evidence-backed fields must not wipe the
		// existing projection — a sparse page would otherwise blank every
		// previously-extracted field. Preserve the current projection and
		// drop it from the diff.
		updateFieldValues := fieldValues
		if len(fieldValues) <= 2 {
			updateFieldValues = existing.FieldValues
		}

		changes, changed := diffGuideContent(existing, title, body, updateFieldValues, canonicalURL)
		if len(changed) == 0 {
			result = PromoteResult{DomainID: pgToUUID(existing.ID), ContentID: pgToUUID(existing.ID), Action: ActionVerified}

			return nil
		}

		params := buildContentUpdateParams(existing, title, existing.Slug, body, hash)
		params.FieldValues = updateFieldValues
		params.CanonicalUrl = &canonicalURL
		params.AiGeneratedAt = pgtype.Timestamptz{Time: p.now(), Valid: true}
		params.AiModel = p.aiModel

		updated, uerr := tx.UpdateContent(ctx, params)
		if uerr != nil {
			return fmt.Errorf("promoter: update guide content: %w", uerr)
		}

		if err := syncUpdateMediaRefs(ctx, tx, updated); err != nil {
			return fmt.Errorf("promoter: sync media fields: %w", err)
		}

		changesJSON, merr := json.Marshal(changes)
		if merr != nil {
			return fmt.Errorf("promoter: marshal field changes: %w", merr)
		}

		note := noteAutoRecrawl
		if lerr := tx.CreateIntelReviewLog(ctx, gen.CreateIntelReviewLogParams{
			IntelRecordID: record.ID,
			ActorUserID:   p.authorID,
			Action:        "auto_update",
			FieldChanges:  changesJSON,
			Note:          &note,
		}); lerr != nil {
			return fmt.Errorf("promoter: write auto_update review log: %w", lerr)
		}

		result = PromoteResult{
			DomainID:      pgToUUID(existing.ID),
			ContentID:     pgToUUID(existing.ID),
			Action:        ActionUpdated,
			ChangedFields: changed,
		}

		return nil
	})

	return result, err
}

// diffGuideContent computes the real extraction-owned field diff between
// the live contents row and the re-derived extraction — the auto_update
// review log records these, not placeholders.
func diffGuideContent(existing gen.GetContentByIDRow, title, body string, fieldValues []byte, canonicalURL string) ([]fieldChange, []string) {
	changes := []fieldChange{}
	changed := []string{}

	if existing.Title != title {
		changes = append(changes, fieldChange{Field: fieldTitle, From: existing.Title, To: title})
		changed = append(changed, fieldTitle)
	}

	if existing.Body != body {
		changes = append(changes, fieldChange{Field: "body", From: fmt.Sprintf("%d chars", len(existing.Body)), To: fmt.Sprintf("%d chars", len(body))})
		changed = append(changed, "body")
	}

	// field_values is the projection for guide types — diff per key so the
	// review log names the field that moved, not a blob.
	changes, changed = diffFieldValues(existing.FieldValues, fieldValues, changes, changed)

	existingCanonical := ""
	if existing.CanonicalUrl != nil {
		existingCanonical = *existing.CanonicalUrl
	}

	if existingCanonical != canonicalURL {
		changes = append(changes, fieldChange{Field: "canonical_url", From: existingCanonical, To: canonicalURL})
		changed = append(changed, "canonical_url")
	}

	return changes, changed
}

// diffFieldValues appends one fieldChange per differing top-level key in
// contents.field_values ({name: value} map written by extractedFieldValues).
// Capped at 20 entries so a wholesale rewrite doesn't bloat the log row.
func diffFieldValues(oldJSON, newJSON []byte, changes []fieldChange, changed []string) ([]fieldChange, []string) {
	var oldMap, newMap map[string]any

	_ = json.Unmarshal(oldJSON, &oldMap)
	_ = json.Unmarshal(newJSON, &newMap)

	if len(oldMap) == 0 && len(newMap) == 0 {
		return changes, changed
	}

	keys := map[string]struct{}{}
	for k := range oldMap {
		keys[k] = struct{}{}
	}

	for k := range newMap {
		keys[k] = struct{}{}
	}

	sortedKeys := make([]string, 0, len(keys))
	for k := range keys {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	diffed := 0

	for _, k := range sortedKeys {
		oldV, okOld := oldMap[k]
		newV, okNew := newMap[k]

		if okOld && okNew {
			oldB, _ := json.Marshal(oldV)
			newB, _ := json.Marshal(newV)
			if string(oldB) == string(newB) {
				continue
			}
		}

		if diffed >= 20 {
			break
		}

		changes = append(changes, fieldChange{Field: "field_values." + k, From: oldV, To: newV})
		diffed++
	}

	if diffed > 0 {
		changed = append(changed, "field_values")
	}

	return changes, changed
}

// guideBodyFromExtraction serializes the extraction as a JSON body for
// the contents row. The guide content types store structured data in
// custom_field_values, so the body is a human-readable summary plus the
// JSON extraction for downstream consumers.
func guideBodyFromExtraction(ext extractor.Extraction, title string) string {
	// Build a readable body from the extraction fields. Keys are sorted so
	// the same extraction always serializes identically — an unordered map
	// walk would produce a new content_hash on every re-crawl and trigger
	// a spurious auto_update diff on the update path.
	body := title + "\n\n"

	keys := make([]string, 0, len(ext))
	for key := range ext {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		field := ext[key]
		if field.Value == nil {
			continue
		}

		valBytes, _ := json.Marshal(field.Value)
		body += fmt.Sprintf("- %s: %s\n", key, string(valBytes))
	}

	return body
}
