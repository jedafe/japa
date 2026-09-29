// Package wiki implements the Destination Wiki (§80, ADR-088).
// // Collaborative, versioned knowledge-base pages attached to each destination
// country/city Space. Wiki pages are living documents continuously improved
// by reputation-gated community members. Every edit creates a pending
// revision that must be approved by a moderator+ before it becomes the
// current content.
// // Ref: japaconnect-engineering/sections/80-destination-wiki.md
// //	japaconnect-engineering/adr/ADR-088-destination-wiki.md
package wiki

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	contentpkg "github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/rag"
)

// Store is the data-access layer for wiki_pages and wiki_revisions.
// Queries live in internal/db/queries/wiki*.sql and run through the
// generated gen.Queries facade (Rule 2); the write pool is also kept so
// multi-statement methods can open a transaction and share it via WithTx.
type Store struct {
	pool  *db.DB
	read  *gen.Queries
	write *gen.Queries
}

// NewStore creates a wiki store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{
		pool:  database,
		read:  gen.New(database.Reader),
		write: gen.New(database.Writer),
	}
}

// WikiPage represents a single wiki page row.
// //nolint:revive // exported: stutter naming is intentional
type WikiPage struct {
	ID pgtype.UUID

	SpaceID          pgtype.UUID
	Slug             string
	Title            string
	Category         string
	IsActive         bool
	IsFeatured       bool
	IsLocked         bool
	CreatedBy        pgtype.UUID
	CreatedAt        pgtype.Timestamptz
	UpdatedAt        pgtype.Timestamptz
	IsJapabotIndexed bool
	DataEntityType   *string
	// §80 Hierarchical UX: PageTemplateID records the wiki_type_page_templates
	// row this page was created from (NULL for freeform pages); FieldGroupID
	// stores the resolved page-level group binding so it survives template
	// deletion (effective group = field_group_id ?? category.field_group_id);
	// Position orders sibling pages inside a category for Prev/Next.
	PageTemplateID pgtype.UUID
	FieldGroupID   pgtype.UUID
	Position       int32
}

// WikiPageWithSpace includes the space slug/name for the wiki index listing.
// //nolint:revive // exported: stutter naming is intentional
type WikiPageWithSpace struct {
	WikiPage
	SpaceSlug string
	SpaceName string
}

// AdminWikiPageWithSpace extends WikiPageWithSpace with the latest revision
// metadata for the admin review queue (B1: contract drift fix). The latest
// revision is the most recently created row for the page, regardless of status.
type AdminWikiPageWithSpace struct {
	WikiPageWithSpace
	LatestRevisionID     *pgtype.UUID
	LatestRevisionStatus *string
	Excerpt              *string
	// DeletedAt is the soft-delete marker (§80) — non-nil rows can be
	// force-deleted from the admin queue.
	DeletedAt pgtype.Timestamptz
}

// WikiDestination is a wiki-bearing space with its active wiki page count.
// Region is set on country-level spaces and mirrors the spaces.region value
// (Africa, Americas, Asia, Europe, Oceania).
// //nolint:revive // exported: stutter naming is intentional
type WikiDestination struct {
	SpaceID   pgtype.UUID
	Slug      string
	Name      string
	GeoCode   *string
	Region    *string
	Level     string
	PageCount int64
}

// WikiRevision represents a single revision row.
// //nolint:revive // exported: stutter naming is intentional
type WikiRevision struct {
	ID pgtype.UUID

	WikiPageID pgtype.UUID // nullable — page-less ops revisions (§80)
	Content    []byte      // TipTap JSON
	Summary    *string
	// §80 community field ops: kind is content|fields|mixed; ops carries the
	// structured field/group mutation array; target_* identifies the
	// polymorphic entity the ops apply to.
	Kind             string
	Ops              []byte
	TargetEntityType *string
	TargetEntityID   pgtype.UUID

	Status      string // pending|approved|rejected
	SubmittedBy pgtype.UUID
	ReviewedBy  pgtype.UUID
	ReviewedAt  pgtype.Timestamptz
	// ApplyError records the last ops-application failure on a pending
	// fields/mixed revision (§80) — set when approve+applyOps rolls back so
	// reviewers can see why; cleared on successful approval.
	ApplyError      *string
	AiGenerated     bool
	AiModel         *string
	AiPromptHash    *string
	HitlReviewedBy  pgtype.UUID
	HitlReviewedAt  pgtype.Timestamptz
	HitlReviewNotes *string
	CreatedAt       pgtype.Timestamptz
	SubmittedByName *string
	ReviewedByName  *string
}

// strPtrOrNil maps a COALESCE'd username column (” when the LEFT JOIN missed)
// back to the *string the public model exposes.
func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func boolDeref(b *bool) bool { return b != nil && *b }

// mapWikiPage fills a WikiPageWithSpace from a generated page row — all page
// row shapes share the same column set, so callers pass the scalars through.
func mapWikiPage(
	id, spaceID pgtype.UUID, slug, title, category string,
	isActive, isFeatured, isLocked bool, createdBy pgtype.UUID,
	createdAt, updatedAt pgtype.Timestamptz, indexed *bool, det *string,
	ptID, fgID pgtype.UUID, position int32,
	spaceSlug, spaceName string,
) WikiPageWithSpace {
	return WikiPageWithSpace{
		WikiPage: WikiPage{
			ID:               id,
			SpaceID:          spaceID,
			Slug:             slug,
			Title:            title,
			Category:         category,
			IsActive:         isActive,
			IsFeatured:       isFeatured,
			IsLocked:         isLocked,
			CreatedBy:        createdBy,
			CreatedAt:        createdAt,
			UpdatedAt:        updatedAt,
			IsJapabotIndexed: boolDeref(indexed),
			DataEntityType:   det,
			PageTemplateID:   ptID,
			FieldGroupID:     fgID,
			Position:         position,
		},
		SpaceSlug: spaceSlug,
		SpaceName: spaceName,
	}
}

func pageFromSlugRow(r gen.GetWikiPageBySlugRow) WikiPageWithSpace {
	return mapWikiPage(r.ID, r.SpaceID, r.Slug, r.Title, r.Category,
		r.IsActive, r.IsFeatured, r.IsLocked, r.CreatedBy, r.CreatedAt,
		r.UpdatedAt, r.IsJapabotIndexed, r.DataEntityType,
		r.PageTemplateID, r.FieldGroupID, r.Position,
		r.SpaceSlug, r.SpaceName)
}

func pageFromSpaceSlugRow(r gen.ListWikiPagesBySpaceSlugRow) WikiPageWithSpace {
	return mapWikiPage(r.ID, r.SpaceID, r.Slug, r.Title, r.Category,
		r.IsActive, r.IsFeatured, r.IsLocked, r.CreatedBy, r.CreatedAt,
		r.UpdatedAt, r.IsJapabotIndexed, r.DataEntityType,
		r.PageTemplateID, r.FieldGroupID, r.Position,
		r.SpaceSlug, r.SpaceName)
}

func pageFromSpaceIDRow(r gen.ListWikiPagesBySpaceIDRow) WikiPageWithSpace {
	return mapWikiPage(r.ID, r.SpaceID, r.Slug, r.Title, r.Category,
		r.IsActive, r.IsFeatured, r.IsLocked, r.CreatedBy, r.CreatedAt,
		r.UpdatedAt, r.IsJapabotIndexed, r.DataEntityType,
		r.PageTemplateID, r.FieldGroupID, r.Position,
		r.SpaceSlug, r.SpaceName)
}

// mapRevision fills a WikiRevision from a generated revision row — the
// generated queries COALESCE the join usernames to ” so they map back to
// nil via strPtrOrNil.
func mapRevision(
	id, pageID pgtype.UUID, content []byte, summary *string,
	kind string, ops []byte, tet *string, teid pgtype.UUID,
	status string, subBy, revBy pgtype.UUID, revAt pgtype.Timestamptz,
	applyErr *string, aiGen bool, aiModel, aiHash *string,
	hitlBy pgtype.UUID, hitlAt pgtype.Timestamptz, hitlNotes *string,
	createdAt pgtype.Timestamptz, subName, revName string,
) WikiRevision {
	return WikiRevision{
		ID:               id,
		WikiPageID:       pageID,
		Content:          content,
		Summary:          summary,
		Kind:             kind,
		Ops:              ops,
		TargetEntityType: tet,
		TargetEntityID:   teid,
		Status:           status,
		SubmittedBy:      subBy,
		ReviewedBy:       revBy,
		ReviewedAt:       revAt,
		ApplyError:       applyErr,
		AiGenerated:      aiGen,
		AiModel:          aiModel,
		AiPromptHash:     aiHash,
		HitlReviewedBy:   hitlBy,
		HitlReviewedAt:   hitlAt,
		HitlReviewNotes:  hitlNotes,
		CreatedAt:        createdAt,
		SubmittedByName:  strPtrOrNil(subName),
		ReviewedByName:   strPtrOrNil(revName),
	}
}

func revisionFromApprovedRow(r gen.GetApprovedWikiRevisionRow) WikiRevision {
	return mapRevision(r.ID, r.WikiPageID, r.Content, r.Summary, r.Kind, r.Ops,
		r.TargetEntityType, r.TargetEntityID, r.Status, r.SubmittedBy,
		r.ReviewedBy, r.ReviewedAt, r.ApplyError, r.AiGenerated, r.AiModel,
		r.AiPromptHash, r.HitlReviewedBy, r.HitlReviewedAt, r.HitlReviewNotes,
		r.CreatedAt, r.SubmittedByName, "")
}

func revisionFromListRow(r gen.ListWikiRevisionsRow) WikiRevision {
	return mapRevision(r.ID, r.WikiPageID, r.Content, r.Summary, r.Kind, r.Ops,
		r.TargetEntityType, r.TargetEntityID, r.Status, r.SubmittedBy,
		r.ReviewedBy, r.ReviewedAt, r.ApplyError, r.AiGenerated, r.AiModel,
		r.AiPromptHash, r.HitlReviewedBy, r.HitlReviewedAt, r.HitlReviewNotes,
		r.CreatedAt, r.SubmittedByName, r.ReviewedByName)
}

func revisionFromGetRow(r gen.GetWikiRevisionRow) WikiRevision {
	return mapRevision(r.ID, r.WikiPageID, r.Content, r.Summary, r.Kind, r.Ops,
		r.TargetEntityType, r.TargetEntityID, r.Status, r.SubmittedBy,
		r.ReviewedBy, r.ReviewedAt, r.ApplyError, r.AiGenerated, r.AiModel,
		r.AiPromptHash, r.HitlReviewedBy, r.HitlReviewedAt, r.HitlReviewNotes,
		r.CreatedAt, r.SubmittedByName, r.ReviewedByName)
}

func revisionFromByTargetRow(r gen.ListWikiRevisionsByTargetRow) WikiRevision {
	return mapRevision(r.ID, r.WikiPageID, r.Content, r.Summary, r.Kind, r.Ops,
		r.TargetEntityType, r.TargetEntityID, r.Status, r.SubmittedBy,
		r.ReviewedBy, r.ReviewedAt, r.ApplyError, r.AiGenerated, r.AiModel,
		r.AiPromptHash, r.HitlReviewedBy, r.HitlReviewedAt, r.HitlReviewNotes,
		r.CreatedAt, r.SubmittedByName, r.ReviewedByName)
}

func revisionFromAdminRow(r gen.AdminListWikiRevisionsRow) WikiRevision {
	return mapRevision(r.ID, r.WikiPageID, r.Content, r.Summary, r.Kind, r.Ops,
		r.TargetEntityType, r.TargetEntityID, r.Status, r.SubmittedBy,
		r.ReviewedBy, r.ReviewedAt, r.ApplyError, r.AiGenerated, r.AiModel,
		r.AiPromptHash, r.HitlReviewedBy, r.HitlReviewedAt, r.HitlReviewNotes,
		r.CreatedAt, r.SubmittedByName, r.ReviewedByName)
}

func revisionFromApproveRow(r gen.ApproveWikiRevisionByIDRow) WikiRevision {
	return mapRevision(r.ID, r.WikiPageID, r.Content, r.Summary, r.Kind, r.Ops,
		r.TargetEntityType, r.TargetEntityID, r.Status, r.SubmittedBy,
		r.ReviewedBy, r.ReviewedAt, r.ApplyError, r.AiGenerated, r.AiModel,
		r.AiPromptHash, r.HitlReviewedBy, r.HitlReviewedAt, r.HitlReviewNotes,
		r.CreatedAt, "", "")
}

// GetWikiPageBySlug returns a wiki page by space slug + page slug.
func (s *Store) GetWikiPageBySlug(ctx context.Context, spaceSlug, pageSlug string) (WikiPageWithSpace, error) {
	r, err := s.read.GetWikiPageBySlug(ctx, gen.GetWikiPageBySlugParams{
		SpaceSlug: spaceSlug,
		PageSlug:  pageSlug,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return WikiPageWithSpace{}, pgx.ErrNoRows
		}

		return WikiPageWithSpace{}, fmt.Errorf("wiki: get page by slug: %w", err)
	}

	return pageFromSlugRow(r), nil
}

// WikiPageRowExists reports whether a wiki_pages row exists for
// (space_slug, page_slug) regardless of is_active/deleted_at — the lazy
// template path uses it to keep tombstoned/deactivated pages dead instead
// of resurrecting them as lazy renders.
func (s *Store) WikiPageRowExists(ctx context.Context, spaceSlug, pageSlug string) (bool, error) {
	exists, err := s.read.WikiPageRowExistsBySlug(ctx, gen.WikiPageRowExistsBySlugParams{
		SpaceSlug: spaceSlug,
		PageSlug:  pageSlug,
	})
	if err != nil {
		return false, fmt.Errorf("wiki: page row exists: %w", err)
	}

	return exists, nil
}

// ListWikiPagesBySpace returns all active wiki pages for a space, with the
// associated space slug and name (Rule 16 — avoid a second API round-trip).
func (s *Store) ListWikiPagesBySpace(ctx context.Context, spaceSlug string) ([]WikiPageWithSpace, error) {
	rows, err := s.read.ListWikiPagesBySpaceSlug(ctx, spaceSlug)
	if err != nil {
		return nil, fmt.Errorf("wiki: list pages by space: %w", err)
	}

	out := make([]WikiPageWithSpace, 0, len(rows))
	for _, r := range rows {
		out = append(out, pageFromSpaceSlugRow(r))
	}

	return out, nil
}

// ListWikiPagesBySpaceID returns all active wiki pages for a space by its ID.
// Used by the ?dest= geo-aware query parameter (ADR-086 geo-aware pattern).
func (s *Store) ListWikiPagesBySpaceID(ctx context.Context, spaceID pgtype.UUID) ([]WikiPageWithSpace, error) {
	rows, err := s.read.ListWikiPagesBySpaceID(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("wiki: list pages by space ID: %w", err)
	}

	out := make([]WikiPageWithSpace, 0, len(rows))
	for _, r := range rows {
		out = append(out, pageFromSpaceIDRow(r))
	}

	return out, nil
}

// ListWikiDestinations returns active wiki-bearing spaces with their active
// wiki page count. The base set is spaces whose space_type has an enabled
// wiki_type_configs row (§80 Wiki Types amendment — replaces the hardcoded
// geo+university level list). Optional level, parent, region, search and
// within filters narrow the result; level matches sp.level or the
// space_type slug. within is a primary-space slug matched against the
// space_parent_spaces junction and parent_id, including descendants of the
// target (e.g. a state matches its cities' universities). When hasWiki is
// true, only spaces with at least one active wiki page are returned.
func (s *Store) ListWikiDestinations(ctx context.Context, level, parentSlug, region, search, within string, hasWiki bool) ([]WikiDestination, error) {
	rows, err := s.read.ListWikiDestinations(ctx, gen.ListWikiDestinationsParams{
		Level:   level,
		Region:  region,
		Search:  search,
		Parent:  parentSlug,
		Within:  within,
		HasWiki: hasWiki,
	})
	if err != nil {
		return nil, fmt.Errorf("wiki: list destinations: %w", err)
	}

	out := make([]WikiDestination, 0, len(rows))
	for _, r := range rows {
		out = append(out, WikiDestination{
			SpaceID:   r.ID,
			Slug:      r.Slug,
			Name:      r.Name,
			GeoCode:   r.GeoCode,
			Region:    r.Region,
			Level:     r.Level,
			PageCount: r.PageCount,
		})
	}

	return out, nil
}

// GetApprovedRevision returns the latest approved revision for a wiki page.
// kind='fields' revisions are excluded: they carry no prose (content='{}') and
// approving one must never replace the page's body (§80 community field ops).
func (s *Store) GetApprovedRevision(ctx context.Context, pageID pgtype.UUID) (WikiRevision, error) {
	r, err := s.read.GetApprovedWikiRevision(ctx, pageID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return WikiRevision{}, pgx.ErrNoRows
		}

		return WikiRevision{}, fmt.Errorf("wiki: get approved revision: %w", err)
	}

	return revisionFromApprovedRow(r), nil
}

// ListRevisions returns the revision history for a wiki page (all statuses).
func (s *Store) ListRevisions(ctx context.Context, pageID pgtype.UUID, limit, offset int32) ([]WikiRevision, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if limit > 100 {
		limit = 100
	}

	rows, err := s.read.ListWikiRevisions(ctx, gen.ListWikiRevisionsParams{
		PageID:    pageID,
		OffsetVal: offset,
		LimitVal:  limit,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("wiki: list revisions: %w", err)
	}

	out := make([]WikiRevision, 0, len(rows))
	for _, r := range rows {
		out = append(out, revisionFromListRow(r))
	}

	total, err := s.read.CountWikiRevisionsByPage(ctx, pageID)
	if err != nil {
		return nil, 0, fmt.Errorf("wiki: count revisions: %w", err)
	}

	return out, total, nil
}

// ErrDuplicateContent is returned when a submitted revision has the same
// normalized content_hash as an existing revision for the same page (Rule 12).
// Callers should map this to a 409 duplicate_content API error.
var ErrDuplicateContent = errors.New("wiki: duplicate content")

// ErrRevisionNotPending is returned when an approve/reject targets a
// revision that does not exist or is no longer pending. Handlers map it to
// a 409 conflict (the revision was already reviewed) or 404 when the ID is
// unknown — the store cannot distinguish the two, so handlers surface 409.
var ErrRevisionNotPending = errors.New("wiki: revision not found or not pending")

// ErrPageExists is returned when a wiki page with the same slug already
// exists in the same space.
var ErrPageExists = errors.New("wiki page already exists")

// revisionContentHash computes the dedupe hash for a revision (Rule 12):
// plain text extracted from the TipTap body plus a NUL-separated ops payload.
// Appending the ops bytes keeps idx_wiki_revisions_page_hash deduping
// identical submissions while preventing fields-only revisions (empty prose)
// from all colliding on the hash of an empty document.
// Ref: japaconnect-engineering/AGENTS.md Rule 12
func revisionContentHash(body, ops []byte) string {
	// Normalize here (not just in insertRevision) so every caller hashes the
	// same input whether ops arrives as nil or []byte("[]").
	if len(ops) == 0 {
		ops = []byte("[]")
	}

	plainText := rag.ExtractPlainText(string(body))

	return contentpkg.ComputeContentHash(plainText + "\x00" + string(ops))
}

// isDupContentErr maps a unique-violation on idx_wiki_revisions_page_hash.
func isDupContentErr(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" &&
		strings.Contains(pgErr.ConstraintName, "wiki_revisions_page_hash")
}

// SubmitRevision inserts a new pending revision. pageID may be an invalid
// pgtype.UUID for page-less ops revisions (§80 community field ops); kind is
// content|fields|mixed; body may be empty for fields-only revisions (stored
// as '{}'::jsonb); ops is the marshaled FieldOp array ('[]' when nil).
// BUG-002: computes content_hash (Rule 12) and rejects duplicate no-op edits
// with ErrDuplicateContent when the unique index idx_wiki_revisions_page_hash
// conflicts.
func (s *Store) SubmitRevision(ctx context.Context, pageID, userID pgtype.UUID, kind string, body, ops []byte, summary string, targetType *string, targetID pgtype.UUID) (pgtype.UUID, error) {
	return s.insertRevision(ctx, s.write, pageID, userID, kind, body, ops, summary, targetType, targetID, false, "", "")
}

// SubmitAIRevision inserts a pending revision flagged as AI-generated
// (ai_generated, ai_model, ai_prompt_hash) — used by GenerateDraft
// (ADR-088 HITL pipeline). Ops revisions never carry AI provenance, so this
// always records kind='content' with an empty ops array.
func (s *Store) SubmitAIRevision(ctx context.Context, pageID, userID pgtype.UUID, body []byte, summary, aiModel, aiPromptHash string) (pgtype.UUID, error) {
	return s.insertRevision(ctx, s.write, pageID, userID, "content", body, nil, summary, nil, pgtype.UUID{}, true, aiModel, aiPromptHash)
}

// insertRevision is the shared INSERT for SubmitRevision/SubmitAIRevision.
// It takes the Queries facade so callers inside a transaction can pass
// s.write.WithTx(tx).
func (s *Store) insertRevision(ctx context.Context, q *gen.Queries, pageID, userID pgtype.UUID, kind string, body, ops []byte, summary string, targetType *string, targetID pgtype.UUID, aiGenerated bool, aiModel, aiPromptHash string) (pgtype.UUID, error) {
	var summaryPtr *string
	if summary != "" {
		summaryPtr = &summary
	}

	var aiModelPtr, aiPromptHashPtr *string
	if aiModel != "" {
		aiModelPtr = &aiModel
	}

	if aiPromptHash != "" {
		aiPromptHashPtr = &aiPromptHash
	}

	if kind == "" {
		kind = "content"
	}

	// content is NOT NULL — fields-only revisions store an empty doc so
	// ::text excerpt rendering keeps working.
	if len(body) == 0 {
		body = []byte("{}")
	}

	// ops is NOT NULL DEFAULT '[]' — normalize nil to an empty array so the
	// content_hash input is stable whether callers pass nil or []byte("[]").
	if len(ops) == 0 {
		ops = []byte("[]")
	}

	// BUG-002: compute content_hash per Rule 12 (see revisionContentHash).
	contentHash := revisionContentHash(body, ops)

	revID, err := q.InsertWikiRevision(ctx, gen.InsertWikiRevisionParams{
		WikiPageID:       pageID,
		Kind:             kind,
		Content:          body,
		Ops:              ops,
		Summary:          summaryPtr,
		TargetEntityType: targetType,
		TargetEntityID:   targetID,
		SubmittedBy:      userID,
		AiGenerated:      aiGenerated,
		AiModel:          aiModelPtr,
		AiPromptHash:     aiPromptHashPtr,
		ContentHash:      &contentHash,
	})
	if err != nil {
		if isDupContentErr(err) {
			return pgtype.UUID{}, ErrDuplicateContent
		}

		return pgtype.UUID{}, fmt.Errorf("wiki: submit revision: %w", err)
	}

	return revID, nil
}

// ApproveRevision marks a revision as approved and stamps the reviewer.
// Also updates the wiki_page's updated_at. Returns the approved revision's
// content (TipTap JSON) so the caller can enqueue RAG indexing (B8).
func (s *Store) ApproveRevision(ctx context.Context, revID, pageID, reviewerID pgtype.UUID, reviewNotes string) ([]byte, error) {
	tx, err := s.pool.Writer.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("wiki: approve: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	q := s.write.WithTx(tx)

	var notesPtr *string
	if reviewNotes != "" {
		notesPtr = &reviewNotes
	}

	content, err := q.ApproveWikiRevision(ctx, gen.ApproveWikiRevisionParams{
		ReviewedBy:      reviewerID,
		HitlReviewNotes: notesPtr,
		ID:              revID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRevisionNotPending
		}

		return nil, fmt.Errorf("wiki: approve: update revision: %w", err)
	}

	if err := q.UpdateWikiPageTouched(ctx, pageID); err != nil {
		return nil, fmt.Errorf("wiki: approve: update page: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("wiki: approve: commit: %w", err)
	}

	return content, nil
}

// RejectRevision marks a revision as rejected and stamps the reviewer.
// Returns ErrRevisionNotPending when the revision is missing or already
// reviewed — the UPDATE's status='pending' guard matches zero rows.
func (s *Store) RejectRevision(ctx context.Context, revID, reviewerID pgtype.UUID, reviewNotes string) error {
	var notesPtr *string
	if reviewNotes != "" {
		notesPtr = &reviewNotes
	}

	n, err := s.write.RejectWikiRevision(ctx, gen.RejectWikiRevisionParams{
		ReviewedBy:      reviewerID,
		HitlReviewNotes: notesPtr,
		ID:              revID,
	})
	if err != nil {
		return fmt.Errorf("wiki: reject revision: %w", err)
	}

	if n == 0 {
		return ErrRevisionNotPending
	}

	return nil
}

// GetRevision returns a single revision row by ID (any kind/status), with
// submitter/reviewer usernames resolved like ListRevisions.
func (s *Store) GetRevision(ctx context.Context, revID pgtype.UUID) (WikiRevision, error) {
	r, err := s.read.GetWikiRevision(ctx, revID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return WikiRevision{}, pgx.ErrNoRows
		}

		return WikiRevision{}, fmt.Errorf("wiki: get revision: %w", err)
	}

	return revisionFromGetRow(r), nil
}

// BeginTx opens a writer-pool transaction for multi-statement wiki writes —
// used by the service's ops-approval path where the revision status flip and
// the field-ops application must commit or roll back atomically (§80).
func (s *Store) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return s.pool.Writer.Begin(ctx)
}

// ApproveRevisionByID marks a revision as approved by revision ID alone —
// used by the revision-keyed review routes where the caller may not know the
// host page (page-less ops revisions have NULL wiki_page_id, §80). Returns
// the full approved row (kind + ops + content) so the caller can apply ops
// and enqueue propagation. The host page's updated_at is bumped when the
// revision is page-bound.
func (s *Store) ApproveRevisionByID(ctx context.Context, revID, reviewerID pgtype.UUID, reviewNotes string) (WikiRevision, error) {
	tx, err := s.pool.Writer.Begin(ctx)
	if err != nil {
		return WikiRevision{}, fmt.Errorf("wiki: approve: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	r, err := s.ApproveRevisionTx(ctx, tx, revID, reviewerID, reviewNotes)
	if err != nil {
		return WikiRevision{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return WikiRevision{}, fmt.Errorf("wiki: approve: commit: %w", err)
	}

	return r, nil
}

// ApproveRevisionTx is the transaction-scoped core of ApproveRevisionByID.
// Callers supplying their own transaction (e.g. the service's
// approve+applyOps path in service_fields.go) share one atomic commit between
// the status flip and the field mutations. Marks the revision approved (it
// must be pending), stamps the reviewer, bumps the host page's updated_at
// when the revision is page-bound, and returns the updated row.
func (s *Store) ApproveRevisionTx(ctx context.Context, tx pgx.Tx, revID, reviewerID pgtype.UUID, reviewNotes string) (WikiRevision, error) {
	var notesPtr *string
	if reviewNotes != "" {
		notesPtr = &reviewNotes
	}

	q := s.write.WithTx(tx)

	r, err := q.ApproveWikiRevisionByID(ctx, gen.ApproveWikiRevisionByIDParams{
		ReviewedBy:      reviewerID,
		HitlReviewNotes: notesPtr,
		ID:              revID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return WikiRevision{}, ErrRevisionNotPending
		}

		return WikiRevision{}, fmt.Errorf("wiki: approve: update revision: %w", err)
	}

	rev := revisionFromApproveRow(r)

	if rev.WikiPageID.Valid {
		if err := q.UpdateWikiPageTouched(ctx, rev.WikiPageID); err != nil {
			return WikiRevision{}, fmt.Errorf("wiki: approve: update page: %w", err)
		}
	}

	return rev, nil
}

// RejectRevisionByID marks a revision as rejected by revision ID alone —
// the revision-keyed counterpart used by the admin review queue (§80).
// RejectRevision already keys on revision ID, so this delegates to it.
func (s *Store) RejectRevisionByID(ctx context.Context, revID, reviewerID pgtype.UUID, reviewNotes string) error {
	return s.RejectRevision(ctx, revID, reviewerID, reviewNotes)
}

// SetApplyError records an ops-application failure on a revision (§80). It
// runs outside the failed approve transaction — the status flip and ops
// mutations roll back, but the failure reason persists so the admin queue
// can show why the revision cannot be approved. Errors are swallowed by
// callers (best-effort observability, never blocks the real error return).
func (s *Store) SetApplyError(ctx context.Context, revID pgtype.UUID, applyErr string) error {
	if err := s.write.SetWikiRevisionApplyError(ctx, gen.SetWikiRevisionApplyErrorParams{
		ApplyError: &applyErr,
		ID:         revID,
	}); err != nil {
		return fmt.Errorf("wiki: set apply error: %w", err)
	}

	return nil
}

// ListRevisionsByTarget returns revisions (all statuses, newest first)
// targeting a specific entity via target_entity_type + target_entity_id.
func (s *Store) ListRevisionsByTarget(ctx context.Context, entityType string, entityID pgtype.UUID, limit, offset int32) ([]WikiRevision, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if limit > 100 {
		limit = 100
	}

	rows, err := s.read.ListWikiRevisionsByTarget(ctx, gen.ListWikiRevisionsByTargetParams{
		TargetEntityType: &entityType,
		TargetEntityID:   entityID,
		OffsetVal:        offset,
		LimitVal:         limit,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("wiki: list revisions by target: %w", err)
	}

	out := make([]WikiRevision, 0, len(rows))
	for _, r := range rows {
		out = append(out, revisionFromByTargetRow(r))
	}

	total, err := s.read.CountWikiRevisionsByTarget(ctx, gen.CountWikiRevisionsByTargetParams{
		TargetEntityType: &entityType,
		TargetEntityID:   entityID,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("wiki: count revisions by target: %w", err)
	}

	return out, total, nil
}

// AdminListRevisions returns revisions for the admin review queue. kind,
// status, and q are optional filters — an empty string means no filter.
// sort is a whitelisted "key:dir" token ("created_at:asc|desc"); anything
// else falls through to the created_at DESC default (§80 community field
// ops review).
func (s *Store) AdminListRevisions(ctx context.Context, kind, status, q, sort string, limit, offset int32) ([]WikiRevision, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if limit > 200 {
		limit = 200
	}

	if offset < 0 {
		offset = 0
	}

	rows, err := s.read.AdminListWikiRevisions(ctx, gen.AdminListWikiRevisionsParams{
		Kind:      kind,
		Status:    status,
		Q:         q,
		Sort:      sort,
		OffsetVal: offset,
		LimitVal:  limit,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("wiki: admin list revisions: %w", err)
	}

	out := make([]WikiRevision, 0, len(rows))
	for _, r := range rows {
		out = append(out, revisionFromAdminRow(r))
	}

	total, err := s.read.AdminCountWikiRevisions(ctx, gen.AdminCountWikiRevisionsParams{
		Kind:   kind,
		Status: status,
		Q:      q,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("wiki: admin count revisions: %w", err)
	}

	return out, total, nil
}

// CountPendingByTarget returns the number of pending revisions targeting an
// entity — used to surface "N pending" badges on the entity fields panel.
func (s *Store) CountPendingByTarget(ctx context.Context, entityType string, entityID pgtype.UUID) (int64, error) {
	n, err := s.read.CountPendingRevisionsByTarget(ctx, gen.CountPendingRevisionsByTargetParams{
		TargetEntityType: &entityType,
		TargetEntityID:   entityID,
	})
	if err != nil {
		return 0, fmt.Errorf("wiki: count pending by target: %w", err)
	}

	return n, nil
}

// FindHostPageID returns the first active wiki page for a space+category —
// the "host page" an entity-bound ops revision attaches to when the entity
// is space-backed (§80). Returns pgx.ErrNoRows when no page matches.
func (s *Store) FindHostPageID(ctx context.Context, spaceID pgtype.UUID, category string) (pgtype.UUID, error) {
	id, err := s.read.FindWikiHostPage(ctx, gen.FindWikiHostPageParams{
		SpaceID:  spaceID,
		Category: category,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, pgx.ErrNoRows
		}

		return pgtype.UUID{}, fmt.Errorf("wiki: find host page: %w", err)
	}

	return id, nil
}

// GetSpaceTypeRef returns the space_type id and slug for a space — needed to
// resolve entity_class tokens for space-backed field group assignments
// (§06/§80): seeded assignments use space_type:{slug} and the class-wide
// space_type token, while ops-created rows may use space_type:{id}.
func (s *Store) GetSpaceTypeRef(ctx context.Context, spaceID pgtype.UUID) (pgtype.UUID, string, error) {
	r, err := s.read.GetSpaceTypeRef(ctx, spaceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, "", pgx.ErrNoRows
		}

		return pgtype.UUID{}, "", fmt.Errorf("wiki: get space type ref: %w", err)
	}

	return r.SpaceTypeID, r.SpaceTypeSlug, nil
}

// UpdatePage updates wiki page admin-managed fields (lock, feature, active,
// data entity binding). dataEntityType binds the page to a child entity
// class (§80 community field ops); an empty string clears it to NULL.
func (s *Store) UpdatePage(ctx context.Context, pageID pgtype.UUID, isLocked, isFeatured, isActive *bool, dataEntityType *string, position *int32) error {
	var detVal any
	if dataEntityType != nil {
		detVal = *dataEntityType
	}

	affected, err := s.write.UpdateWikiPage(ctx, gen.UpdateWikiPageParams{
		IsLocked:             isLocked,
		IsFeatured:           isFeatured,
		IsActive:             isActive,
		Position:             position,
		UpdateDataEntityType: dataEntityType != nil,
		DataEntityType:       detVal,
		ID:                   pageID,
	})
	if err != nil {
		return fmt.Errorf("wiki: update page: %w", err)
	}

	if affected == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// ErrPageNotDeleted is returned by HardDeleteWikiPage when the page exists
// but is not soft-deleted — permanent removal requires a prior soft delete
// (§80).
var ErrPageNotDeleted = errors.New("wiki page is not soft-deleted")

// DeleteWikiPage soft-deletes a wiki page: deleted_at is stamped and
// is_active flips false so every public reader excludes it. Returns
// pgx.ErrNoRows when the page is missing or already deleted.
func (s *Store) DeleteWikiPage(ctx context.Context, pageID pgtype.UUID) error {
	affected, err := s.write.SoftDeleteWikiPage(ctx, pageID)
	if err != nil {
		return fmt.Errorf("wiki: soft delete page: %w", err)
	}

	if affected == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// HardDeleteWikiPage permanently removes a soft-deleted page; revisions
// cascade via ON DELETE CASCADE. Returns pgx.ErrNoRows when the page does
// not exist and ErrPageNotDeleted when it exists but is still live.
func (s *Store) HardDeleteWikiPage(ctx context.Context, pageID pgtype.UUID) error {
	affected, err := s.write.HardDeleteWikiPage(ctx, pageID)
	if err != nil {
		return fmt.Errorf("wiki: hard delete page: %w", err)
	}

	if affected == 0 {
		exists, err := s.read.WikiPageExists(ctx, pageID)
		if err != nil {
			return fmt.Errorf("wiki: check page exists: %w", err)
		}
		if !exists {
			return pgx.ErrNoRows
		}
		return ErrPageNotDeleted
	}

	return nil
}

// GetSpaceBySlug resolves a wiki-bearing space slug to its ID and level —
// the space's type must have an enabled wiki_type_configs row (§80 Wiki
// Types amendment). Returns pgx.ErrNoRows if the space does not exist or
// its type has no wiki.
func (s *Store) GetSpaceBySlug(ctx context.Context, spaceSlug string) (pgtype.UUID, string, error) {
	r, err := s.read.GetWikiSpaceBySlug(ctx, spaceSlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, "", pgx.ErrNoRows
		}

		return pgtype.UUID{}, "", fmt.Errorf("wiki: get space by slug: %w", err)
	}

	return r.ID, r.Level, nil
}

// ListSeedSpaceIDs returns the IDs of all active spaces whose wiki type
// opts into stub seeding (wiki_type_configs.seed_stubs) — the seed set for
// the wiki:seed_stub_pages fan-out (§80 Wiki Types amendment; replaces the
// country-only list).
func (s *Store) ListSeedSpaceIDs(ctx context.Context) ([]pgtype.UUID, error) {
	ids, err := s.read.ListWikiSeedSpaceIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("wiki: list seed spaces: %w", err)
	}

	return ids, nil
}

// IsUniversitySpaceBySlug reports whether the slug resolves to an active
// university space — typed via space_types.slug = 'university' or, for the
// seeded university spaces, level = 'university'. Unknown slugs return false.
func (s *Store) IsUniversitySpaceBySlug(ctx context.Context, spaceSlug string) (bool, error) {
	isUni, err := s.read.IsUniversitySpaceBySlug(ctx, spaceSlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}

		return false, fmt.Errorf("wiki: is university space: %w", err)
	}

	return boolDeref(isUni), nil
}

// GetSpaceByID resolves a wiki-bearing space ID to its slug and level —
// the space's type must have an enabled wiki_type_configs row. Returns
// pgx.ErrNoRows if the space does not exist or its type has no wiki.
func (s *Store) GetSpaceByID(ctx context.Context, spaceID pgtype.UUID) (slug, level string, err error) {
	r, err := s.read.GetWikiSpaceByID(ctx, spaceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", pgx.ErrNoRows
		}

		return "", "", fmt.Errorf("wiki: get space by id: %w", err)
	}

	return r.Slug, r.Level, nil
}

// CreatePage creates a new wiki page. Used by the admin generate endpoint
// and the space activation seed job.
func (s *Store) CreatePage(ctx context.Context, spaceID, createdBy pgtype.UUID, slug, title, category string) (pgtype.UUID, error) {
	return s.createPageBound(ctx, s.write, spaceID, createdBy, slug, title, category, pgtype.UUID{}, pgtype.UUID{}, 0)
}

// createPageBound is createPage with the page-template binding + sibling
// position (§80 Hierarchical UX). pageTemplateID/fieldGroupID are the
// resolved template binding — an invalid UUID stores NULL (freeform page).
func (s *Store) createPageBound(ctx context.Context, q *gen.Queries, spaceID, createdBy pgtype.UUID, slug, title, category string, pageTemplateID, fieldGroupID pgtype.UUID, position int32) (pgtype.UUID, error) {
	pageID, err := q.CreateWikiPage(ctx, gen.CreateWikiPageParams{
		SpaceID:        spaceID,
		Slug:           slug,
		Title:          title,
		Category:       category,
		CreatedBy:      createdBy,
		PageTemplateID: pageTemplateID,
		FieldGroupID:   fieldGroupID,
		Position:       position,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, ErrPageExists
		}

		return pgtype.UUID{}, fmt.Errorf("wiki: create page: %w", err)
	}

	return pageID, nil
}

// CreatePageWithRevision creates a new wiki page and its initial pending
// revision inside a single transaction.
func (s *Store) CreatePageWithRevision(ctx context.Context, spaceID, userID pgtype.UUID, slug, title, category string, content []byte, summary string, pageTemplateID, fieldGroupID pgtype.UUID, position int32) (pageID, revID pgtype.UUID, err error) {
	tx, err := s.pool.Writer.Begin(ctx)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("wiki: create page tx: %w", err)
	}
	defer tx.Rollback(ctx)

	q := s.write.WithTx(tx)

	// Insert the page, ignoring duplicate (space_id, slug).
	pageIDResult, err := s.createPageBound(ctx, q, spaceID, userID, slug, title, category, pageTemplateID, fieldGroupID, position)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, err
	}

	// Compute content_hash per Rule 12 — same pipeline as insertRevision
	// (plain text + NUL + ops) so a later identical SubmitRevision still
	// dedupes against this initial revision.
	contentHash := revisionContentHash(content, nil)

	var summaryPtr *string
	if summary != "" {
		summaryPtr = &summary
	}

	// Insert the initial pending revision.
	revIDResult, err := q.InsertInitialWikiRevision(ctx, gen.InsertInitialWikiRevisionParams{
		WikiPageID:  pageIDResult,
		Content:     content,
		Summary:     summaryPtr,
		SubmittedBy: userID,
		ContentHash: &contentHash,
	})
	if err != nil {
		if isDupContentErr(err) {
			return pgtype.UUID{}, pgtype.UUID{}, ErrDuplicateContent
		}

		return pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("wiki: create page revision: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("wiki: create page commit: %w", err)
	}

	return pageIDResult, revIDResult, nil
}

// AdminListPagesFilter holds filter parameters for the admin page list.
type AdminListPagesFilter struct {
	SpaceID  pgtype.UUID
	DestSlug string // destination filter by space slug (admin UI sends ?dest=)
	Category string
	IsLocked *bool
	Q        string
	Sort     string // "<key>:<dir>" — whitelisted CASE keys in the query
	// Deleted selects the soft-delete scope (§80): '' excludes deleted rows,
	// 'only' returns just them, 'all' includes both.
	Deleted string
	Limit   int32
	Offset  int32
}

// AdminListPages returns all wiki pages for admin management (paginated).
// Each row includes the latest revision's id, status, and an excerpt so the
// admin review queue can gate Approve/Reject actions (B1).
func (s *Store) AdminListPages(ctx context.Context, f AdminListPagesFilter) ([]AdminWikiPageWithSpace, int64, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 20
	}

	if limit > 200 {
		limit = 200
	}

	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	var spaceID pgtype.UUID
	if f.SpaceID.Valid {
		spaceID = f.SpaceID
	}

	rows, err := s.read.AdminListWikiPages(ctx, gen.AdminListWikiPagesParams{
		SpaceID:   spaceID,
		DestSlug:  f.DestSlug,
		Category:  f.Category,
		IsLocked:  f.IsLocked,
		Q:         f.Q,
		Sort:      f.Sort,
		Deleted:   f.Deleted,
		OffsetVal: offset,
		LimitVal:  limit,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("wiki: admin list pages: %w", err)
	}

	out := make([]AdminWikiPageWithSpace, 0, len(rows))
	for _, r := range rows {
		p := AdminWikiPageWithSpace{
			WikiPageWithSpace: mapWikiPage(
				r.ID, r.SpaceID, r.Slug, r.Title, r.Category,
				r.IsActive, r.IsFeatured, r.IsLocked, r.CreatedBy,
				r.CreatedAt, r.UpdatedAt, r.IsJapabotIndexed,
				r.DataEntityType, r.PageTemplateID, r.FieldGroupID,
				r.Position, r.SpaceSlug, r.SpaceName,
			),
		}
		if r.LatestRevisionID.Valid {
			p.LatestRevisionID = &r.LatestRevisionID
		}
		p.LatestRevisionStatus = strPtrOrNil(r.LatestRevisionStatus)
		p.Excerpt = strPtrOrNil(r.Excerpt)
		p.DeletedAt = r.DeletedAt

		out = append(out, p)
	}

	total, err := s.read.AdminCountWikiPages(ctx, gen.AdminCountWikiPagesParams{
		SpaceID:  spaceID,
		DestSlug: f.DestSlug,
		Category: f.Category,
		IsLocked: f.IsLocked,
		Q:        f.Q,
		Deleted:  f.Deleted,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("wiki: admin count pages: %w", err)
	}

	return out, total, nil
}

// ---------------------------------------------------------------------------
// Hierarchy reads (§80 Hierarchical UX & Itemized Pricing amendment)
// ---------------------------------------------------------------------------

// WikiSibling is one sibling page in a category's Prev/Next ordering.
type WikiSibling struct {
	ID       pgtype.UUID
	Slug     string
	Title    string
	Position int32
}

// ListPageSiblings returns a space's pages inside one category in display
// order — the Prev/Next navigation set.
func (s *Store) ListPageSiblings(ctx context.Context, spaceID pgtype.UUID, category string) ([]WikiSibling, error) {
	rows, err := s.read.ListWikiPageSiblings(ctx, gen.ListWikiPageSiblingsParams{
		SpaceID:  spaceID,
		Category: category,
	})
	if err != nil {
		return nil, fmt.Errorf("wiki: list siblings: %w", err)
	}

	out := make([]WikiSibling, 0, len(rows))
	for _, r := range rows {
		out = append(out, WikiSibling{ID: r.ID, Slug: r.Slug, Title: r.Title, Position: r.Position})
	}

	return out, nil
}

// WikiParentLink is one space one hop up the actual hierarchy — parents via
// space_parent_spaces ∪ spaces.parent_id. IsPrimary marks the parent_id
// link so the service can prefer it when arbitrating breadcrumb chains.
type WikiParentLink struct {
	ID            pgtype.UUID
	Slug          string
	Name          string
	Level         string
	SpaceTypeID   pgtype.UUID
	SpaceTypeSlug string
	// SpaceTypeLabel is the display label of the parent's type — the
	// wiki_type_configs label with the raw space_type name as fallback —
	// so ancestor crumbs can render "Country"/"City" without a second
	// lookup.
	SpaceTypeLabel string
	IsPrimary      bool
}

// ListSpaceParentLinks returns the space's direct parents.
func (s *Store) ListSpaceParentLinks(ctx context.Context, spaceID pgtype.UUID) ([]WikiParentLink, error) {
	rows, err := s.read.ListWikiSpaceParentLinks(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("wiki: list parent links: %w", err)
	}

	out := make([]WikiParentLink, 0, len(rows))
	for _, r := range rows {
		out = append(out, WikiParentLink{
			ID: r.ID, Slug: r.Slug, Name: r.Name, Level: r.Level,
			SpaceTypeID: r.SpaceTypeID, SpaceTypeSlug: r.SpaceTypeSlug,
			SpaceTypeLabel: r.SpaceTypeLabel,
			IsPrimary:      r.IsPrimary,
		})
	}

	return out, nil
}

// WikiDescendant is one descendant space row for rollup reads.
type WikiDescendant struct {
	ID            pgtype.UUID
	Slug          string
	Name          string
	Level         string
	SpaceTypeID   pgtype.UUID
	SpaceTypeSlug string
	Depth         int32
}

// ListDescendantSpaces returns the space's transitive descendants (depth
// ≤3) restricted to the given space types — an empty typeIDs list returns
// every descendant.
func (s *Store) ListDescendantSpaces(ctx context.Context, spaceID pgtype.UUID, typeIDs []pgtype.UUID) ([]WikiDescendant, error) {
	if typeIDs == nil {
		typeIDs = []pgtype.UUID{}
	}

	rows, err := s.read.ListWikiDescendantSpaces(ctx, gen.ListWikiDescendantSpacesParams{
		SpaceID: spaceID,
		TypeIds: typeIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("wiki: list descendants: %w", err)
	}

	out := make([]WikiDescendant, 0, len(rows))
	for _, r := range rows {
		out = append(out, WikiDescendant{
			ID: r.ID, Slug: r.Slug, Name: r.Name, Level: r.Level,
			SpaceTypeID: r.SpaceTypeID, SpaceTypeSlug: r.SpaceTypeSlug,
			Depth: r.Depth,
		})
	}

	return out, nil
}

// ListTypeChildIDs returns the space types that declare the given type as
// a parent (wiki_type_parents reverse edges) — the declared child types
// driving rollup scope and the landing drill-down.
func (s *Store) ListTypeChildIDs(ctx context.Context, spaceTypeID pgtype.UUID) ([]pgtype.UUID, error) {
	ids, err := s.read.ListWikiTypeChildIDs(ctx, spaceTypeID)
	if err != nil {
		return nil, fmt.Errorf("wiki: list type children: %w", err)
	}

	return ids, nil
}

// ListTypeParentIDs returns the type's declared parent types — the
// arbitration input for multi-parent breadcrumb chains.
func (s *Store) ListTypeParentIDs(ctx context.Context, spaceTypeID pgtype.UUID) ([]pgtype.UUID, error) {
	ids, err := s.read.ListWikiTypeParentIDs(ctx, spaceTypeID)
	if err != nil {
		return nil, fmt.Errorf("wiki: list type parents: %w", err)
	}

	return ids, nil
}
