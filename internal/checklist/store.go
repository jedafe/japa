// Package checklist implements the Visa Document Checklist Generator
// (Section 70, ADR-070). Checklist items are extracted from the
// required_documents custom field on published visa guides — no parallel
// document data store. Per-user progress is persisted in checklist_progress.
// // Ref: japaconnect-engineering/sections/70-visa-checklist.md
// //	japaconnect-engineering/adr/ADR-070-visa-document-checklist-generator.md
package checklist

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store wraps the sqlc-generated Queries for checklist_progress operations
// and provides direct pgx queries against contents + custom_field_values for
// the visa-guide selector and item extraction. It is the data-access layer —
// no business logic here.
type Store struct {
	queries      *gen.Queries
	writeQueries *gen.Queries
	pool         *db.DB
}

// NewStore creates a checklist store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{
		queries:      gen.New(database.Reader),
		writeQueries: gen.New(database.Writer),
		pool:         database,
	}
}

// VisaGuideField holds a single field value extracted from a published visa
// guide's custom_field_values row.
type VisaGuideField struct {
	GuideID   pgtype.UUID
	FieldKey  string
	ValueText string
	ValueJSON []byte
}

// ListPublishedVisaGuideFields queries published visa guides and their
// custom_field_values for the requested field keys (e.g.
// destination_country, visa_type, required_documents). The query joins
// contents with custom_field_values + custom_fields so platform scoping is enforced
// transitively via custom_fields. (BUG-014 pattern).
func (s *Store) ListPublishedVisaGuideFields(ctx context.Context, fieldKeys []string) ([]VisaGuideField, error) {
	if len(fieldKeys) == 0 {
		return nil, nil
	}

	const q = `
SELECT c.id, cf.key, cfv.value_text, cfv.value_json
FROM contents c
JOIN custom_field_values cfv
  ON cfv.entity_type = 'content' AND cfv.entity_id = c.id
JOIN custom_fields cf
  ON cf.id = cfv.field_id
WHERE c.content_type = 'guide'
  AND c.guide_category = 'visa'
  AND c.status = 'published'
  AND c.deleted_at IS NULL
  AND cf.key = ANY($1)
ORDER BY c.id, cf.key
`

	rows, err := s.pool.Reader.Query(ctx, q, fieldKeys)
	if err != nil {
		return nil, fmt.Errorf("checklist: list visa guide fields: %w", err)
	}
	defer rows.Close()

	var out []VisaGuideField

	for rows.Next() {
		var f VisaGuideField

		var valueText *string
		if err := rows.Scan(&f.GuideID, &f.FieldKey, &valueText, &f.ValueJSON); err != nil {
			return nil, fmt.Errorf("checklist: scan visa guide field: %w", err)
		}

		if valueText != nil {
			f.ValueText = *valueText
		}

		out = append(out, f)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("checklist: iterate visa guide fields: %w", err)
	}

	return out, nil
}

// CountrySpace is a country-level Geo Space (§05) used as an origin selector
// option. Only the fields needed by the selector are returned.
type CountrySpace struct {
	ID   pgtype.UUID
	Name string
	Slug string
}

// ListCountrySpaces returns all country-level Geo Spaces for the platform,
// ordered by name. Used to populate the origin selector (§70 step 1).
func (s *Store) ListCountrySpaces(ctx context.Context) ([]CountrySpace, error) {
	const q = `
SELECT id, name, slug
FROM spaces
WHERE level = 'country' AND deleted_at IS NULL
ORDER BY name ASC
`

	rows, err := s.pool.Reader.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("checklist: list country spaces: %w", err)
	}
	defer rows.Close()

	var out []CountrySpace

	for rows.Next() {
		var c CountrySpace
		if err := rows.Scan(&c.ID, &c.Name, &c.Slug); err != nil {
			return nil, fmt.Errorf("checklist: scan country space: %w", err)
		}

		out = append(out, c)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("checklist: iterate country spaces: %w", err)
	}

	return out, nil
}

// GetVisaGuideFields returns the requested field values for a specific
// guide (required_documents + destination_country + visa_type in one round
// trip). Missing field keys simply produce no row — the caller defaults.
func (s *Store) GetVisaGuideFields(ctx context.Context, guideID pgtype.UUID, fieldKeys []string) ([]VisaGuideField, error) {
	if len(fieldKeys) == 0 {
		return nil, nil
	}

	const q = `
SELECT c.id, cf.key, cfv.value_text, cfv.value_json
FROM contents c
JOIN custom_field_values cfv
  ON cfv.entity_type = 'content' AND cfv.entity_id = c.id
JOIN custom_fields cf
  ON cf.id = cfv.field_id
WHERE c.id = $1
  AND c.content_type = 'guide'
  AND c.guide_category = 'visa'
  AND c.status = 'published'
  AND c.deleted_at IS NULL
  AND cf.key = ANY($2)
`

	rows, err := s.pool.Reader.Query(ctx, q, guideID, fieldKeys)
	if err != nil {
		return nil, fmt.Errorf("checklist: get visa guide fields: %w", err)
	}
	defer rows.Close()

	var out []VisaGuideField

	for rows.Next() {
		var f VisaGuideField

		var valueText *string
		if err := rows.Scan(&f.GuideID, &f.FieldKey, &valueText, &f.ValueJSON); err != nil {
			return nil, fmt.Errorf("checklist: scan visa guide field: %w", err)
		}

		if valueText != nil {
			f.ValueText = *valueText
		}

		out = append(out, f)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("checklist: iterate visa guide fields: %w", err)
	}

	return out, nil
}

// SelectorVersion returns a version string covering the visa-guide corpus
// plus the country spaces feeding the origin step — used to hash the
// jc:render selector cache key so corpus changes mint fresh keys.
func (s *Store) SelectorVersion(ctx context.Context) (string, error) {
	const q = `
SELECT
  (SELECT COUNT(*)::bigint FROM contents
    WHERE content_type = 'guide' AND guide_category = 'visa'
      AND status = 'published' AND deleted_at IS NULL),
  COALESCE(EXTRACT(EPOCH FROM (SELECT MAX(updated_at) FROM contents
    WHERE content_type = 'guide' AND guide_category = 'visa'
      AND status = 'published' AND deleted_at IS NULL))::bigint, 0),
  (SELECT COUNT(*)::bigint FROM spaces
    WHERE level = 'country' AND deleted_at IS NULL),
  COALESCE(EXTRACT(EPOCH FROM (SELECT MAX(updated_at) FROM spaces
    WHERE level = 'country' AND deleted_at IS NULL))::bigint, 0)
`

	var guideCount, guideMax, spaceCount, spaceMax int64
	if err := s.pool.Reader.QueryRow(ctx, q).Scan(&guideCount, &guideMax, &spaceCount, &spaceMax); err != nil {
		return "", fmt.Errorf("checklist: selector version: %w", err)
	}

	return fmt.Sprintf("%d:%d:%d:%d", guideCount, guideMax, spaceCount, spaceMax), nil
}

// GetVisaGuideMeta returns the title + updated_at of a published visa guide.
// updated_at feeds the jc:render cache key so guide edits mint fresh keys.
func (s *Store) GetVisaGuideMeta(ctx context.Context, guideID pgtype.UUID) (string, pgtype.Timestamptz, error) {
	const q = `
SELECT c.title, c.updated_at
FROM contents c
WHERE c.id = $1
  AND c.content_type = 'guide'
  AND c.guide_category = 'visa'
  AND c.status = 'published'
  AND c.deleted_at IS NULL
LIMIT 1
`

	var (
		title     string
		updatedAt pgtype.Timestamptz
	)
	if err := s.pool.Reader.QueryRow(ctx, q, guideID).Scan(&title, &updatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", pgtype.Timestamptz{}, pgx.ErrNoRows
		}

		return "", pgtype.Timestamptz{}, fmt.Errorf("checklist: get visa guide meta: %w", err)
	}

	return title, updatedAt, nil
}

// ListChecklistProgress returns all progress rows for a user + guide.
func (s *Store) ListChecklistProgress(ctx context.Context, userID, guideID pgtype.UUID) ([]gen.ChecklistProgress, error) {
	return s.queries.ListChecklistProgressByUserGuide(ctx, gen.ListChecklistProgressByUserGuideParams{
		UserID:  userID,
		GuideID: guideID,
	})
}

// GetSingleProgress returns a single progress row for a user + guide +
// item_key (read pool — safe for display paths).
func (s *Store) GetSingleProgress(ctx context.Context, userID, guideID pgtype.UUID, itemKey string) (gen.ChecklistProgress, error) {
	return s.queries.GetSingleChecklistProgress(ctx, gen.GetSingleChecklistProgressParams{
		UserID:  userID,
		GuideID: guideID,
		ItemKey: itemKey,
	})
}

// GetSingleProgressWriter is the write-pool variant used inside the PATCH
// path so a note-only update reads its own writes under replica lag.
func (s *Store) GetSingleProgressWriter(ctx context.Context, userID, guideID pgtype.UUID, itemKey string) (gen.ChecklistProgress, error) {
	return s.writeQueries.GetSingleChecklistProgress(ctx, gen.GetSingleChecklistProgressParams{
		UserID:  userID,
		GuideID: guideID,
		ItemKey: itemKey,
	})
}

// ListChecklistProgressWriter is the write-pool variant used by the
// completion-award check — the upsert just landed on the Writer, so the
// count must not race a replica.
func (s *Store) ListChecklistProgressWriter(ctx context.Context, userID, guideID pgtype.UUID) ([]gen.ChecklistProgress, error) {
	return s.writeQueries.ListChecklistProgressByUserGuide(ctx, gen.ListChecklistProgressByUserGuideParams{
		UserID:  userID,
		GuideID: guideID,
	})
}

// UpsertChecklistProgress marks a single item complete/incomplete + note.
func (s *Store) UpsertChecklistProgress(ctx context.Context, arg gen.UpsertChecklistProgressParams) (gen.ChecklistProgress, error) {
	return s.writeQueries.UpsertChecklistProgress(ctx, arg)
}

// DeleteChecklistProgress resets all progress for a user + guide.
func (s *Store) DeleteChecklistProgress(ctx context.Context, userID, guideID pgtype.UUID) error {
	return s.writeQueries.DeleteChecklistProgressByUserGuide(ctx, gen.DeleteChecklistProgressByUserGuideParams{
		UserID:  userID,
		GuideID: guideID,
	})
}

// ---------------------------------------------------------------------------
// Admin queries — dynamic WHERE/ORDER BY lives in pgx (sqlc can't emit
// whitelisted dynamic ordering); parameter values stay bound.
// ---------------------------------------------------------------------------

// AdminProgressParams carries the admin list filters. Q matches
// username/email/item_key/guide_title via ILIKE; Sort/Dir are whitelisted
// before reaching SQL.
type AdminProgressParams struct {
	GuideID    pgtype.UUID
	UserID     pgtype.UUID
	Q          string
	Sort       string
	Dir        string
	PageOffset int32
	PageLimit  int32
}

// adminProgressSorts maps the public sort key to a safe SQL column
// expression — user input can never reach ORDER BY directly.
var adminProgressSorts = map[string]string{
	"updated_at":   "cp.updated_at",
	"created_at":   "cp.created_at",
	"completed_at": "cp.completed_at",
	"username":     "u.username",
	"guide":        "COALESCE(c.title, '')",
	"item_key":     "cp.item_key",
	"completed":    "cp.completed",
}

// AdminProgressFilteredRow is one row of the admin progress listing.
type AdminProgressFilteredRow struct {
	ID          pgtype.UUID
	UserID      pgtype.UUID
	GuideID     pgtype.UUID
	GuideTitle  string
	ItemKey     string
	Completed   bool
	CompletedAt pgtype.Timestamptz
	Notes       *string
	UpdatedAt   pgtype.Timestamptz
	Username    string
}

// adminProgressWhere builds the shared WHERE clause + args for the admin
// list/count queries. Returns the clause (with leading ANDs) and the arg
// slice starting after $1=guideID $2=userID $3=q.
func adminProgressWhere(guideID, userID pgtype.UUID, q string) (string, []any) {
	args := []any{guideID, userID, q}
	clause := `
 AND ($1::uuid IS NULL OR cp.guide_id = $1)
 AND ($2::uuid IS NULL OR cp.user_id = $2)
 AND ($3 = '' OR u.username ILIKE '%' || $3 || '%'
      OR u.email ILIKE '%' || $3 || '%'
      OR cp.item_key ILIKE '%' || $3 || '%'
      OR c.title ILIKE '%' || $3 || '%')`

	return clause, args
}

// AdminListProgressFiltered lists progress rows with the full filter/sort
// surface the admin UI sends (guide_id, user_id, q, sort, dir, page).
func (s *Store) AdminListProgressFiltered(ctx context.Context, p AdminProgressParams) ([]AdminProgressFilteredRow, error) {
	sortCol := adminProgressSorts[p.Sort]
	if sortCol == "" {
		sortCol = "cp.updated_at"
	}

	dir := "DESC"
	if p.Dir == "asc" {
		dir = "ASC"
	}

	clause, args := adminProgressWhere(p.GuideID, p.UserID, strings.TrimSpace(p.Q))
	args = append(args, p.PageLimit, p.PageOffset)

	q := fmt.Sprintf(`
SELECT
 cp.id, cp.user_id, cp.guide_id, cp.item_key, cp.completed, cp.completed_at,
 cp.notes, cp.updated_at, u.username, COALESCE(c.title, '') AS guide_title
FROM checklist_progress cp
JOIN users u ON u.id = cp.user_id
LEFT JOIN contents c ON c.id = cp.guide_id
WHERE cp.list_type = 'visa'%s
ORDER BY %s %s, cp.id ASC
LIMIT $%d OFFSET $%d`, clause, sortCol, dir, len(args)-1, len(args))

	rows, err := s.pool.Reader.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("checklist: admin list progress: %w", err)
	}
	defer rows.Close()

	var out []AdminProgressFilteredRow

	for rows.Next() {
		var r AdminProgressFilteredRow
		if err := rows.Scan(&r.ID, &r.UserID, &r.GuideID, &r.ItemKey, &r.Completed,
			&r.CompletedAt, &r.Notes, &r.UpdatedAt, &r.Username, &r.GuideTitle); err != nil {
			return nil, fmt.Errorf("checklist: scan admin progress: %w", err)
		}

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("checklist: iterate admin progress: %w", err)
	}

	return out, nil
}

// AdminCountProgressFiltered counts rows under the same filters.
func (s *Store) AdminCountProgressFiltered(ctx context.Context, guideID, userID pgtype.UUID, q string) (int64, error) {
	clause, args := adminProgressWhere(guideID, userID, strings.TrimSpace(q))

	sql := `
SELECT COUNT(*)
FROM checklist_progress cp
JOIN users u ON u.id = cp.user_id
LEFT JOIN contents c ON c.id = cp.guide_id
WHERE cp.list_type = 'visa'` + clause

	var total int64
	if err := s.pool.Reader.QueryRow(ctx, sql, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("checklist: admin count progress: %w", err)
	}

	return total, nil
}

// AdminProgressGuideIDs returns one page of guide_ids that have progress
// rows (most-active first) plus the total distinct-guide count for
// pagination meta.
func (s *Store) AdminProgressGuideIDs(ctx context.Context, offset, limit int32) ([]pgtype.UUID, int64, error) {
	const pageQ = `
SELECT cp.guide_id
FROM checklist_progress cp
WHERE cp.list_type = 'visa'
GROUP BY cp.guide_id
ORDER BY COUNT(DISTINCT cp.user_id) DESC, cp.guide_id
LIMIT $1 OFFSET $2
`

	rows, err := s.pool.Reader.Query(ctx, pageQ, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("checklist: admin stats guide ids: %w", err)
	}
	defer rows.Close()

	var ids []pgtype.UUID

	for rows.Next() {
		var id pgtype.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, 0, fmt.Errorf("checklist: scan stats guide id: %w", err)
		}

		ids = append(ids, id)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("checklist: iterate stats guide ids: %w", err)
	}

	const countQ = `SELECT COUNT(DISTINCT guide_id) FROM checklist_progress WHERE list_type = 'visa'`

	var total int64
	if err := s.pool.Reader.QueryRow(ctx, countQ).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("checklist: admin stats guide count: %w", err)
	}

	return ids, total, nil
}

// AdminPairRow is one user's completed item keys for a guide — the service
// intersects these with the guide's live item set to compute real
// completion rates.
type AdminPairRow struct {
	GuideID  pgtype.UUID
	UserID   pgtype.UUID
	DoneKeys []string
}

// AdminProgressPairs returns per-(guide,user) completed item keys for the
// given guides.
func (s *Store) AdminProgressPairs(ctx context.Context, guideIDs []pgtype.UUID) ([]AdminPairRow, error) {
	if len(guideIDs) == 0 {
		return nil, nil
	}

	const q = `
SELECT cp.guide_id, cp.user_id,
       COALESCE(ARRAY_AGG(cp.item_key) FILTER (WHERE cp.completed), '{}') AS done_keys
FROM checklist_progress cp
WHERE cp.list_type = 'visa' AND cp.guide_id = ANY($1)
GROUP BY cp.guide_id, cp.user_id
`

	rows, err := s.pool.Reader.Query(ctx, q, guideIDs)
	if err != nil {
		return nil, fmt.Errorf("checklist: admin stats pairs: %w", err)
	}
	defer rows.Close()

	var out []AdminPairRow

	for rows.Next() {
		var r AdminPairRow
		if err := rows.Scan(&r.GuideID, &r.UserID, &r.DoneKeys); err != nil {
			return nil, fmt.Errorf("checklist: scan stats pair: %w", err)
		}

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("checklist: iterate stats pairs: %w", err)
	}

	return out, nil
}

// GuideTitles resolves titles + deletion state for a set of guide ids.
// Deleted or missing guides map to ("", true).
func (s *Store) GuideTitles(ctx context.Context, guideIDs []pgtype.UUID) (map[pgtype.UUID]struct {
	Title   string
	Deleted bool
}, error) {
	out := make(map[pgtype.UUID]struct {
		Title   string
		Deleted bool
	}, len(guideIDs))

	if len(guideIDs) == 0 {
		return out, nil
	}

	const q = `
SELECT c.id, c.title, c.deleted_at IS NOT NULL OR c.status <> 'published' AS deleted
FROM contents c
WHERE c.id = ANY($1)
`

	rows, err := s.pool.Reader.Query(ctx, q, guideIDs)
	if err != nil {
		return nil, fmt.Errorf("checklist: guide titles: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id      pgtype.UUID
			title   string
			deleted bool
		)
		if err := rows.Scan(&id, &title, &deleted); err != nil {
			return nil, fmt.Errorf("checklist: scan guide title: %w", err)
		}

		out[id] = struct {
			Title   string
			Deleted bool
		}{Title: title, Deleted: deleted}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("checklist: iterate guide titles: %w", err)
	}

	return out, nil
}
