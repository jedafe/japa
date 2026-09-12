package promoter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/extractor"
)

// WikiStore is the persistence surface the WikiPromoter needs.
// *stores.Store satisfies this interface; tests substitute a mock.
//
// WithTx runs a callback inside a transaction. The callback receives a
// WikiTxStore — a narrower interface that only exposes the write methods
// needed inside the transaction. This ensures createPath and updatePath
// perform their multi-table writes atomically (BUG-01/02 fix, §6.2
// transaction boundary).
type WikiStore interface {
	PersonaResolver
	GetIntelRecord(ctx context.Context, tenantID, id pgtype.UUID) (gen.IntelRecord, error)
	GetIntelSource(ctx context.Context, tenantID, id pgtype.UUID) (gen.IntelSource, error)
	GetWikiPageByIntelRecord(ctx context.Context, tenantID, intelRecordID pgtype.UUID) (gen.WikiPage, error)
	CreateWikiPageIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateWikiPageIntelParams) (gen.WikiPage, error)
	UpdateWikiPageIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateWikiPageIntelParams) error
	UpdateIntelRecordStatus(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelRecordStatusParams) error
	CreateIntelReviewLog(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateIntelReviewLogParams) error
	WithTx(ctx context.Context, fn func(WikiTxStore) error) error
}

// WikiTxStore is the subset of WikiStore needed inside a transaction.
// It exposes only the write methods that createPath and updatePath call, so
// the transactional store can be a narrower type. *stores.Store satisfies
// this interface (via WithTx's callback), and the test mock satisfies it
// directly.
type WikiTxStore interface {
	CreateWikiPageIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateWikiPageIntelParams) (gen.WikiPage, error)
	UpdateIntelRecordStatus(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelRecordStatusParams) error
	UpdateWikiPageIntel(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateWikiPageIntelParams) error
	CreateIntelReviewLog(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateIntelReviewLogParams) error
}

// WikiPromoter promotes wiki_page intel_records into the wiki_pages
// projection table. It implements the editorial-override invariant: on the
// update path, any field present in editorial_overrides is left untouched so
// a human edit always wins over a re-crawl. Under Approach B (ADR-076
// Amendment 1) it writes ONLY the projection row with guide_id=NULL — the
// contents guide entry is generated later by the AI Content Studio via
// intel_record_id. The intel record is flipped to 'extracted' status (NOT
// 'promoted') so Studio can pick it up.
type WikiPromoter struct {
	store    WikiStore
	tenantID pgtype.UUID
	authorID pgtype.UUID
	aiModel  *string // AI model used for extraction (ADR-088, INTEL-003)
	now      func() time.Time
}

// WikiPromoterOption configures a WikiPromoter.
type WikiPromoterOption func(*WikiPromoter)

// WithWikiClock overrides the clock used for timestamps (tests).
func WithWikiClock(fn func() time.Time) WikiPromoterOption {
	return func(p *WikiPromoter) { p.now = fn }
}

// WithWikiAIModel sets the AI model name stamped on created content rows (ADR-088).
func WithWikiAIModel(model string) WikiPromoterOption {
	return func(p *WikiPromoter) { p.aiModel = &model }
}

// NewWikiPromoter creates a promoter backed by the given store. The
// tenantID and authorID identify the tenant and system author stamped onto
// the wiki_pages projection rows created by the create path.
func NewWikiPromoter(store WikiStore, tenantID, authorID pgtype.UUID, opts ...WikiPromoterOption) *WikiPromoter {
	p := &WikiPromoter{store: store, tenantID: tenantID, authorID: authorID, aiModel: ptr("gpt-4o-mini"), now: time.Now}
	for _, opt := range opts {
		opt(p)
	}

	return p
}

// Entity returns the entity_type this promoter handles.
func (p *WikiPromoter) Entity() string { return "wiki_page" }

// ErrNotWikiPage is returned when Promote is called on an intel record whose
// entity_type is not "wiki_page".
var ErrNotWikiPage = errors.New("promoter: intel record is not a wiki_page")

// Promote maps the intel record identified by recordID to a wiki_pages
// projection row, creating it or updating it in place while respecting
// editorial overrides.
func (p *WikiPromoter) Promote(ctx context.Context, recordID uuid.UUID) (PromoteResult, error) {
	return promoteDomain(ctx, p.store, p.tenantID, recordID, domainPromoteOps[gen.WikiPage]{
		entityType: p.Entity(),
		notErr:     ErrNotWikiPage,
		loadLabel:  "wiki page",
		getExisting: func(ctx context.Context, tenantID, pgID pgtype.UUID) (gen.WikiPage, error) {
			return p.store.GetWikiPageByIntelRecord(ctx, tenantID, pgID)
		},
		createPath: p.wikiCreatePath,
		updatePath: p.wikiUpdatePath,
	})
}

// wikiCreatePath inserts the wiki_pages projection row and flips the intel
// record to 'extracted' status. The contents guide row is NOT created here —
// Studio generates it from the extraction data via intel_record_id
// (ADR-076 Amendment 1). guide_id is left NULL so Studio can fill it in
// later. Both writes run inside a single transaction so a failure in either
// rolls back the other (BUG-01 fix, §6.2).
func (p *WikiPromoter) wikiCreatePath(ctx context.Context, record gen.IntelRecord, source gen.IntelSource, ext extractor.Extraction) (PromoteResult, error) {
	authorID := resolveAuthorID(ctx, p.store, source, p.tenantID, p.authorID)
	params := buildWikiCreateParams(record, ext, p.tenantID, authorID)

	var result PromoteResult

	err := p.store.WithTx(ctx, func(tx WikiTxStore) error {
		wiki, err := tx.CreateWikiPageIntel(ctx, p.tenantID, params)
		if err != nil {
			return fmt.Errorf("promoter: create wiki page: %w", err)
		}

		if err := tx.UpdateIntelRecordStatus(ctx, p.tenantID, gen.UpdateIntelRecordStatusParams{
			ID:     record.ID,
			Status: "extracted",
		}); err != nil {
			return fmt.Errorf("promoter: mark intel record extracted: %w", err)
		}

		result = PromoteResult{
			DomainID:  pgToUUID(wiki.ID),
			ContentID: uuid.Nil,
			Action:    ActionCreated,
		}

		return nil
	})

	return result, err
}

// wikiUpdatePath applies the new extraction to the existing wiki_pages row,
// skipping any field present in editorial_overrides, and records an
// auto_update review-log entry when fields actually change. The wiki update
// and the review log write run inside a single transaction so the audit
// trail is always consistent with the projection state (BUG-02 fix, §6.2).
func (p *WikiPromoter) wikiUpdatePath(ctx context.Context, record gen.IntelRecord, existing gen.WikiPage, ext extractor.Extraction) (PromoteResult, error) {
	overrides, err := parseOverrideKeys(existing.EditorialOverrides)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("promoter: parse editorial overrides: %w", err)
	}

	params, changed := buildWikiUpdateParams(existing, ext, overrides)
	params.FreshnessScore = freshScore

	var changesJSON []byte

	if len(changed) > 0 {
		if changesJSON, err = marshalWikiFieldChanges(existing, params, changed); err != nil {
			return PromoteResult{}, fmt.Errorf("promoter: marshal field changes: %w", err)
		}
	}

	return domainUpdateTx(ctx, record, p.authorID, p.tenantID, "wiki page",
		pgToUUID(existing.ID), uuid.Nil, params, changed, changesJSON,
		func(ctx context.Context, fn func(WikiTxStore) error) error { return p.store.WithTx(ctx, fn) },
		WikiTxStore.UpdateWikiPageIntel)
}

// ---------------------------------------------------------------------------
// Extraction -> wiki params
// ---------------------------------------------------------------------------

// buildWikiCreateParams maps a validated extraction to a
// CreateWikiPageIntelParams. SpaceID is resolved from the extraction's
// "space_id" field when present (a UUID string); otherwise it is left NULL
// for Studio to assign. guide_id is always NULL on the create path so
// Studio can fill it in later (Approach B).
func buildWikiCreateParams(record gen.IntelRecord, ext extractor.Extraction, tenantID, authorID pgtype.UUID) gen.CreateWikiPageIntelParams {
	title := stringField(ext, "title")
	if title == "" {
		title = stringField(ext, "name")
	}

	slug := stringField(ext, "slug")
	if slug == "" && title != "" {
		slug = slugify(title)
	}

	category := stringField(ext, "category")
	if category == "" {
		category = "general"
	}

	return gen.CreateWikiPageIntelParams{

		SpaceID:            extractSpaceID(ext),
		Slug:               slug,
		Title:              title,
		Category:           category,
		IsActive:           true,
		IsFeatured:         false,
		IsLocked:           false,
		CreatedBy:          authorID,
		IntelRecordID:      record.ID,
		GuideID:            pgtype.UUID{}, // NULL — Studio fills this later
		ConfidenceOverall:  record.ConfidenceOverall,
		VerificationStatus: "verified",
		FreshnessScore:     freshScore,
		OverrideSource:     "ingestion",
		EditorialOverrides: emptyJSONObject(),
	}
}

// buildWikiUpdateParams produces an UpdateWikiPageIntelParams seeded from
// the existing row, then applies each extraction field that is NOT in
// overrides and whose value actually changed. It returns the params and the
// list of changed column names. Fields in editorial_overrides keep the
// existing value (human edit wins).
func buildWikiUpdateParams(existing gen.WikiPage, ext extractor.Extraction, overrides map[string]struct{}) (gen.UpdateWikiPageIntelParams, []string) {
	params := gen.UpdateWikiPageIntelParams{

		ID:                 existing.ID,
		Title:              existing.Title,
		Category:           existing.Category,
		IsActive:           existing.IsActive,
		IsFeatured:         existing.IsFeatured,
		IsLocked:           existing.IsLocked,
		ConfidenceOverall:  existing.ConfidenceOverall,
		VerificationStatus: existing.VerificationStatus,
		FreshnessScore:     existing.FreshnessScore,
		OverrideSource:     existing.OverrideSource,
		EditorialOverrides: existing.EditorialOverrides,
	}

	var changed []string

	// title
	applyStrField(overrides, ext, "title", &params.Title, existing.Title, &changed)
	// category
	applyStrField(overrides, ext, "category", &params.Category, existing.Category, &changed)
	// is_active
	applyBoolField(overrides, "is_active", func() *bool { return boolField(ext, "is_active") }, &params.IsActive, existing.IsActive, &changed)
	// is_featured
	applyBoolField(overrides, "is_featured", func() *bool { return boolField(ext, "is_featured") }, &params.IsFeatured, existing.IsFeatured, &changed)
	// is_locked
	applyBoolField(overrides, "is_locked", func() *bool { return boolField(ext, "is_locked") }, &params.IsLocked, existing.IsLocked, &changed)

	return params, changed
}

// ---------------------------------------------------------------------------
// Review log field-change snapshot
// ---------------------------------------------------------------------------

func marshalWikiFieldChanges(existing gen.WikiPage, params gen.UpdateWikiPageIntelParams, changed []string) ([]byte, error) {
	changes := make([]fieldChange, 0, len(changed))

	for _, c := range changed {
		fc := fieldChange{Field: c}

		switch c {
		case "title":
			fc.From, fc.To = existing.Title, params.Title
		case "category":
			fc.From, fc.To = existing.Category, params.Category
		case "is_active":
			fc.From, fc.To = existing.IsActive, params.IsActive
		case "is_featured":
			fc.From, fc.To = existing.IsFeatured, params.IsFeatured
		case "is_locked":
			fc.From, fc.To = existing.IsLocked, params.IsLocked
		}

		changes = append(changes, fc)
	}

	return json.Marshal(changes)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// extractSpaceID parses the "space_id" field from the extraction (a UUID
// string) into a pgtype.UUID. Returns an invalid (NULL) UUID when the field
// is absent or unparseable.
func extractSpaceID(ext extractor.Extraction) pgtype.UUID {
	s := stringField(ext, "space_id")
	if s == "" {
		return pgtype.UUID{}
	}

	id, err := uuid.Parse(s)
	if err != nil {
		return pgtype.UUID{}
	}

	return pgtype.UUID{Bytes: id, Valid: true}
}

// boolField returns a pointer to the bool form of an extraction field, or
// nil when the field is absent / unparseable.
func boolField(ext extractor.Extraction, key string) *bool {
	f, ok := ext[key]
	if !ok || f.Value == nil {
		return nil
	}

	switch v := f.Value.(type) {
	case bool:
		return &v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "1", "yes":
			b := true
			return &b
		case "false", "0", "no":
			b := false
			return &b
		}
	case float64:
		b := v != 0
		return &b
	}

	return nil
}

// slugify converts a title into a URL-safe slug. It lowercases, replaces
// non-alphanumeric runs with a single hyphen, and trims leading/trailing
// hyphens.
func slugify(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))

	var b strings.Builder

	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}

	out := strings.Trim(b.String(), "-")
	// Collapse runs of hyphens.
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}

	return out
}
