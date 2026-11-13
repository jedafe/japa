// Package credentialchecker implements the Professional Credential
// Recognition Checker (§94, ADR-102).
// // The package provides credential pathway lookup, community note
// submission, and admin HITL review workflow for professional credential
// recognition pathways between jurisdictions. Pathways describe how a
// credential earned in one space (issuing jurisdiction) maps to another
// (destination jurisdiction) for a given profession, including the
// regulatory body, pathway type (direct recognition, bridging exam,
// supervised practice, or full requalification), timeline and cost
// estimates, and official application URLs. Community notes let members
// share lived experience alongside the official pathway data.
// // Ref: japaconnect-engineering/sections/94-professional-credential-checker.md
// //	japaconnect-engineering/adr/ADR-102-credential-checker.md
package credentialchecker

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
)

// Profession categories (§94).
const (
	CategoryMedical     = "medical"
	CategoryEngineering = "engineering"
	CategoryLegal       = "legal"
	CategoryFinance     = "finance"
	CategoryEducation   = "education"
	CategoryOther       = "other"
)

// Pathway types (§94).
const (
	PathwayDirect              = "direct"
	PathwayBridgingExam        = "bridging_exam"
	PathwaySupervisedPractice  = "supervised_practice"
	PathwayFullRequalification = "full_requalification"
)

// Pathway statuses.
const (
	StatusDraft     = "draft"
	StatusAIDraft   = "ai_draft"
	StatusPending   = "pending"
	StatusPublished = "published"
	StatusRejected  = "rejected"
)

// Store is the data-access layer for credential_professions,
// credential_pathways, and credential_community_notes. It uses direct
// pgx queries because these are new tables without sqlc-generated
// queries yet. No business logic here.
type Store struct {
	pool *db.DB
}

// NewStore creates a credential checker store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{pool: database}
}

// ProfessionRow is a single credential_professions row.
type ProfessionRow struct {
	ID       pgtype.UUID
	Slug     string
	Name     string
	Category *string
}

// PathwayRow is the joined result of credential_pathways +
// credential_professions + spaces (issuing + destination), returned by
// list and get-by-id queries.
type PathwayRow struct {
	ID pgtype.UUID

	ProfessionID           pgtype.UUID
	ProfessionSlug         string
	ProfessionName         string
	ProfessionCategory     *string
	IssuingSpaceID         pgtype.UUID
	IssuingSpaceName       *string
	IssuingSpaceCode       *string
	DestinationSpaceID     pgtype.UUID
	DestinationSpaceName   *string
	DestinationSpaceCode   *string
	RegulatoryBody         string
	RegulatoryBodyURL      *string
	PathwayType            string
	TimelineEstimate       *string
	CostEstimateUSD        *string
	PathwayDescription     string
	OfficialApplicationURL *string
	SourceURLLastChecked   pgtype.Date
	Status                 string
	IsActive               bool
	AIGenerated            bool
	AIModel                *string
	AIPromptHash           *string
	HITLReviewedBy         pgtype.UUID
	HITLReviewedByName     *string
	HITLReviewedAt         pgtype.Timestamptz
	HITLReviewNotes        *string
	IsJapabotIndexed       bool
	CreatedAt              pgtype.Timestamptz
	UpdatedAt              pgtype.Timestamptz
}

// CommunityNoteRow is a single credential_community_notes row joined
// with the submitting user's display name.
// ProfessionName/DestinationSpaceName are populated only on the admin
// moderation list so reviewers see pathway context without a second query.
type CommunityNoteRow struct {
	ID                   pgtype.UUID
	PathwayID            pgtype.UUID
	UserID               pgtype.UUID
	UserName             *string
	Note                 string
	Upvotes              int32
	Status               string
	CreatedAt            pgtype.Timestamptz
	ProfessionName       *string
	DestinationSpaceName *string
}

// CreatePathwayInput holds the fields for inserting a new pathway.
type CreatePathwayInput struct {
	ProfessionID           pgtype.UUID
	IssuingSpaceID         pgtype.UUID
	DestinationSpaceID     pgtype.UUID
	RegulatoryBody         string
	RegulatoryBodyURL      *string
	PathwayType            string
	TimelineEstimate       *string
	CostEstimateUSD        *string
	PathwayDescription     string
	OfficialApplicationURL *string
	SourceURLLastChecked   pgtype.Date
	Status                 string
	IsActive               bool
	AIGenerated            bool
	AIModel                *string
	AIPromptHash           *string
}

// UpdatePathwayInput holds the mutable fields for a pathway update. Only
// non-nil pointers are applied. is_japabot_indexed is intentionally absent
// — it is a GENERATED ALWAYS AS column computed from status + is_active.
type UpdatePathwayInput struct {
	ProfessionID           *pgtype.UUID
	IssuingSpaceID         *pgtype.UUID
	DestinationSpaceID     *pgtype.UUID
	RegulatoryBody         *string
	RegulatoryBodyURL      *string
	PathwayType            *string
	TimelineEstimate       *string
	CostEstimateUSD        *string
	PathwayDescription     *string
	OfficialApplicationURL *string
	SourceURLLastChecked   *pgtype.Date
	Status                 *string
	IsActive               *bool
}

// ListProfessions returns all credential_professions rows ordered by name.
func (s *Store) ListProfessions(ctx context.Context) ([]ProfessionRow, error) {
	const q = `
SELECT id, slug, name, category
FROM credential_professions
ORDER BY name ASC
`

	rows, err := s.pool.Reader.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("credentialchecker: list professions: %w", err)
	}

	defer rows.Close()

	var out []ProfessionRow

	for rows.Next() {
		var r ProfessionRow
		if err := rows.Scan(&r.ID, &r.Slug, &r.Name, &r.Category); err != nil {
			return nil, fmt.Errorf("credentialchecker: scan profession row: %w", err)
		}

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("credentialchecker: iterate profession rows: %w", err)
	}

	return out, nil
}

// GetProfessionByID returns a single profession by ID. Used by
// GeneratePathwayDraft to build the AI generation prompt.
func (s *Store) GetProfessionByID(ctx context.Context, id pgtype.UUID) (ProfessionRow, error) {
	const q = `
SELECT id, slug, name, category
FROM credential_professions
WHERE id = $1
`

	var r ProfessionRow

	err := s.pool.Reader.QueryRow(ctx, q, id).Scan(&r.ID, &r.Slug, &r.Name, &r.Category)
	if err != nil {
		return ProfessionRow{}, fmt.Errorf("credentialchecker: get profession by id: %w", err)
	}

	return r, nil
}

// GetProfessionBySlug returns a single profession by slug. Used by the
// Studio routing adapter to resolve a profession from the job's
// guide_category or topic_prompt.
func (s *Store) GetProfessionBySlug(ctx context.Context, slug string) (ProfessionRow, error) {
	const q = `
SELECT id, slug, name, category
FROM credential_professions
WHERE slug = $1
`

	var r ProfessionRow

	err := s.pool.Reader.QueryRow(ctx, q, slug).Scan(&r.ID, &r.Slug, &r.Name, &r.Category)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ProfessionRow{}, pgx.ErrNoRows
		}

		return ProfessionRow{}, fmt.Errorf("credentialchecker: get profession by slug: %w", err)
	}

	return r, nil
}

// SpaceNameRow holds the name of a single space, used by GeneratePathwayDraft
// to build the AI generation prompt.
type SpaceNameRow struct {
	ID   pgtype.UUID
	Name string
}

// GetSpaceNames returns the names of the issuing and destination spaces
// for the given IDs. Used by GeneratePathwayDraft to build the AI prompt.
func (s *Store) GetSpaceNames(ctx context.Context, issuingSpaceID, destSpaceID pgtype.UUID) (issuingName, destName string, err error) {
	const q = `
SELECT id, name FROM spaces WHERE id = ANY($1)
`

	ids := []pgtype.UUID{issuingSpaceID, destSpaceID}

	rows, err := s.pool.Reader.Query(ctx, q, ids)
	if err != nil {
		return "", "", fmt.Errorf("credentialchecker: get space names: %w", err)
	}

	defer rows.Close()

	for rows.Next() {
		var r SpaceNameRow
		if err := rows.Scan(&r.ID, &r.Name); err != nil {
			return "", "", fmt.Errorf("credentialchecker: scan space name: %w", err)
		}

		if r.ID == issuingSpaceID {
			issuingName = r.Name
		}

		if r.ID == destSpaceID {
			destName = r.Name
		}
	}

	if err := rows.Err(); err != nil {
		return "", "", fmt.Errorf("credentialchecker: iterate space names: %w", err)
	}

	return issuingName, destName, nil
}

// ListPathwaysFilter holds the filter parameters for listing pathways.
type ListPathwaysFilter struct {
	DestinationSpaceID pgtype.UUID // optional
	ProfessionID       pgtype.UUID // optional (Valid = false → no filter)
	IssuingSpaceID     pgtype.UUID // optional
	Status             string      // default: published
	IsActive           *bool       // default: true
	// IncludeInactive bypasses the is_active default for admin callers —
	// AI drafts are created with is_active=false and must stay visible in
	// the review queue.
	IncludeInactive bool
	// AllStatuses bypasses the status='published' default for admin
	// callers — the review queue must see drafts and pending rows.
	AllStatuses bool
	Q           string // optional ILIKE on profession name / regulatory body
	Sort        string // "key:dir" — admin list; whitelist only
	Limit       int32
	Offset      int32
}

// ListPathways returns a paginated, filtered list of credential pathways.
// The query joins credential_pathways + credential_professions + spaces
// (issuing + destination).
func (s *Store) ListPathways(ctx context.Context, f ListPathwaysFilter) ([]PathwayRow, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	if f.Status != "" {
		conditions = append(conditions, fmt.Sprintf("p.status = $%d", argIdx))
		args = append(args, f.Status)
		argIdx++
	} else if !f.AllStatuses {
		conditions = append(conditions, "p.status = 'published'")
	}

	if f.IsActive != nil {
		conditions = append(conditions, fmt.Sprintf("p.is_active = $%d", argIdx))
		args = append(args, *f.IsActive)
		argIdx++
	} else if !f.IncludeInactive {
		conditions = append(conditions, "p.is_active = true")
	}

	if f.DestinationSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("p.destination_space_id = $%d", argIdx))
		args = append(args, f.DestinationSpaceID)
		argIdx++
	}

	if f.IssuingSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("p.issuing_space_id = $%d", argIdx))
		args = append(args, f.IssuingSpaceID)
		argIdx++
	}

	if f.ProfessionID.Valid {
		conditions = append(conditions, fmt.Sprintf("p.profession_id = $%d", argIdx))
		args = append(args, f.ProfessionID)
		argIdx++
	}

	if f.Q != "" {
		conditions = append(conditions, fmt.Sprintf("(pr.name ILIKE $%d OR p.regulatory_body ILIKE $%d)", argIdx, argIdx))
		args = append(args, "%"+f.Q+"%")
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")
	if whereClause == "" {
		whereClause = "TRUE"
	}

	orderBy := "p.updated_at DESC"
	if col, dir, ok := parsePathwaySort(f.Sort); ok {
		orderBy = col + " " + dir
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 20
	}

	if limit > 100 {
		limit = 100
	}

	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	listQuery := fmt.Sprintf(`
SELECT p.id, p.profession_id,
       pr.slug AS profession_slug, pr.name AS profession_name, pr.category AS profession_category,
       p.issuing_space_id, iss.name AS issuing_space_name, iss.geo_code AS issuing_space_code,
       p.destination_space_id, dst.name AS destination_space_name, dst.geo_code AS destination_space_code,
       p.regulatory_body, p.regulatory_body_url, p.pathway_type,
       p.timeline_estimate, p.cost_estimate_usd, p.pathway_description,
       p.official_application_url, p.source_url_last_checked,
       p.status, p.is_active, p.ai_generated, p.ai_model, p.ai_prompt_hash,
       p.hitl_reviewed_by, p.hitl_reviewed_at, p.hitl_review_notes,
       p.is_japabot_indexed, p.created_at, p.updated_at,
       rev.username AS hitl_reviewed_by_name
FROM credential_pathways p
JOIN credential_professions pr ON pr.id = p.profession_id
LEFT JOIN spaces iss ON iss.id = p.issuing_space_id
LEFT JOIN spaces dst ON dst.id = p.destination_space_id
LEFT JOIN users rev ON rev.id = p.hitl_reviewed_by
WHERE %s
ORDER BY %s
LIMIT $%d OFFSET $%d
`, whereClause, orderBy, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("credentialchecker: list pathways: %w", err)
	}
	defer rows.Close()

	var out []PathwayRow

	for rows.Next() {
		var r PathwayRow
		if err := rows.Scan(
			&r.ID, &r.ProfessionID,
			&r.ProfessionSlug, &r.ProfessionName, &r.ProfessionCategory,
			&r.IssuingSpaceID, &r.IssuingSpaceName, &r.IssuingSpaceCode,
			&r.DestinationSpaceID, &r.DestinationSpaceName, &r.DestinationSpaceCode,
			&r.RegulatoryBody, &r.RegulatoryBodyURL, &r.PathwayType,
			&r.TimelineEstimate, &r.CostEstimateUSD, &r.PathwayDescription,
			&r.OfficialApplicationURL, &r.SourceURLLastChecked,
			&r.Status, &r.IsActive, &r.AIGenerated, &r.AIModel, &r.AIPromptHash,
			&r.HITLReviewedBy, &r.HITLReviewedAt, &r.HITLReviewNotes,
			&r.IsJapabotIndexed, &r.CreatedAt, &r.UpdatedAt,
			&r.HITLReviewedByName,
		); err != nil {
			return nil, 0, fmt.Errorf("credentialchecker: scan pathway row: %w", err)
		}

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("credentialchecker: iterate pathway rows: %w", err)
	}

	// The professions join is only needed by the q filter — skip it in the
	// count when unused so the count stays cheap.
	countJoin := ""
	if f.Q != "" {
		countJoin = " JOIN credential_professions pr ON pr.id = p.profession_id"
	}

	countQuery := fmt.Sprintf(`
SELECT COUNT(*) FROM credential_pathways p%s
WHERE %s
`, countJoin, whereClause)
	countArgs := args[:len(args)-2]

	var total int64

	if err := s.pool.Reader.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("credentialchecker: count pathways: %w", err)
	}

	return out, total, nil
}

// GetPathwayByID returns a single credential pathway by its id, joining
// credential_professions + spaces (issuing + destination).
func (s *Store) GetPathwayByID(ctx context.Context, id pgtype.UUID) (PathwayRow, error) {
	const q = `
SELECT p.id, p.profession_id,
       pr.slug AS profession_slug, pr.name AS profession_name, pr.category AS profession_category,
       p.issuing_space_id, iss.name AS issuing_space_name, iss.geo_code AS issuing_space_code,
       p.destination_space_id, dst.name AS destination_space_name, dst.geo_code AS destination_space_code,
       p.regulatory_body, p.regulatory_body_url, p.pathway_type,
       p.timeline_estimate, p.cost_estimate_usd, p.pathway_description,
       p.official_application_url, p.source_url_last_checked,
       p.status, p.is_active, p.ai_generated, p.ai_model, p.ai_prompt_hash,
       p.hitl_reviewed_by, p.hitl_reviewed_at, p.hitl_review_notes,
       p.is_japabot_indexed, p.created_at, p.updated_at,
       rev.username AS hitl_reviewed_by_name
FROM credential_pathways p
JOIN credential_professions pr ON pr.id = p.profession_id
LEFT JOIN spaces iss ON iss.id = p.issuing_space_id
LEFT JOIN spaces dst ON dst.id = p.destination_space_id
LEFT JOIN users rev ON rev.id = p.hitl_reviewed_by
WHERE p.id = $1
LIMIT 1
`

	var r PathwayRow

	err := s.pool.Reader.QueryRow(ctx, q, id).Scan(
		&r.ID, &r.ProfessionID,
		&r.ProfessionSlug, &r.ProfessionName, &r.ProfessionCategory,
		&r.IssuingSpaceID, &r.IssuingSpaceName, &r.IssuingSpaceCode,
		&r.DestinationSpaceID, &r.DestinationSpaceName, &r.DestinationSpaceCode,
		&r.RegulatoryBody, &r.RegulatoryBodyURL, &r.PathwayType,
		&r.TimelineEstimate, &r.CostEstimateUSD, &r.PathwayDescription,
		&r.OfficialApplicationURL, &r.SourceURLLastChecked,
		&r.Status, &r.IsActive, &r.AIGenerated, &r.AIModel, &r.AIPromptHash,
		&r.HITLReviewedBy, &r.HITLReviewedAt, &r.HITLReviewNotes,
		&r.IsJapabotIndexed, &r.CreatedAt, &r.UpdatedAt,
		&r.HITLReviewedByName,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PathwayRow{}, pgx.ErrNoRows
		}

		return PathwayRow{}, fmt.Errorf("credentialchecker: get pathway by id: %w", err)
	}

	return r, nil
}

// GetPathwayNotes returns published community notes for a pathway, joined
// with the submitting user's username. Ordered by upvotes DESC then
// created_at DESC.
func (s *Store) GetPathwayNotes(ctx context.Context, pathwayID pgtype.UUID) ([]CommunityNoteRow, error) {
	const q = `
SELECT n.id, n.pathway_id, n.user_id, u.username AS user_name,
       n.note, n.upvotes, n.status, n.created_at
FROM credential_community_notes n
JOIN credential_pathways p ON p.id = n.pathway_id
LEFT JOIN users u ON u.id = n.user_id
WHERE n.pathway_id = $1 AND n.status = 'published' AND n.deleted_at IS NULL
ORDER BY n.upvotes DESC, n.created_at DESC
LIMIT 200
`

	rows, err := s.pool.Reader.Query(ctx, q, pathwayID)
	if err != nil {
		return nil, fmt.Errorf("credentialchecker: get pathway notes: %w", err)
	}

	defer rows.Close()

	var out []CommunityNoteRow

	for rows.Next() {
		var r CommunityNoteRow
		if err := rows.Scan(
			&r.ID, &r.PathwayID, &r.UserID, &r.UserName,
			&r.Note, &r.Upvotes, &r.Status, &r.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("credentialchecker: scan note row: %w", err)
		}

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("credentialchecker: iterate note rows: %w", err)
	}

	return out, nil
}

// CreatePathway inserts a new credential_pathways row and returns the
// inserted id.
func (s *Store) CreatePathway(ctx context.Context, input CreatePathwayInput) (pgtype.UUID, error) {
	status := input.Status
	if status == "" {
		status = StatusPublished
	}

	const q = `
INSERT INTO credential_pathways (
    profession_id, issuing_space_id, destination_space_id,
    regulatory_body, regulatory_body_url, pathway_type,
    timeline_estimate, cost_estimate_usd, pathway_description,
    official_application_url, source_url_last_checked,
    status, is_active, ai_generated, ai_model, ai_prompt_hash
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
RETURNING id
`

	var id pgtype.UUID

	err := s.pool.Writer.QueryRow(
		ctx, q,
		input.ProfessionID, input.IssuingSpaceID, input.DestinationSpaceID,
		input.RegulatoryBody, input.RegulatoryBodyURL, input.PathwayType,
		input.TimelineEstimate, input.CostEstimateUSD, input.PathwayDescription,
		input.OfficialApplicationURL, input.SourceURLLastChecked,
		status, input.IsActive, input.AIGenerated, input.AIModel, input.AIPromptHash,
	).Scan(&id)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("credentialchecker: create pathway: %w", err)
	}

	return id, nil
}

// UpdatePathway updates the mutable fields on a credential_pathways row.
// Only non-nil pointers in the input are applied. Returns pgx.ErrNoRows
// if the row does not exist for the given platform.
func (s *Store) UpdatePathway(ctx context.Context, id pgtype.UUID, input UpdatePathwayInput) error {
	var (
		sets   []string
		args   []any
		argIdx = 1
	)

	addSet := func(col string, val any) {
		sets = append(sets, fmt.Sprintf("%s = $%d", col, argIdx))
		args = append(args, val)
		argIdx++
	}

	if input.ProfessionID != nil {
		addSet("profession_id", *input.ProfessionID)
	}

	if input.IssuingSpaceID != nil {
		addSet("issuing_space_id", *input.IssuingSpaceID)
	}

	if input.DestinationSpaceID != nil {
		addSet("destination_space_id", *input.DestinationSpaceID)
	}

	if input.RegulatoryBody != nil {
		addSet("regulatory_body", *input.RegulatoryBody)
	}

	if input.RegulatoryBodyURL != nil {
		addSet("regulatory_body_url", *input.RegulatoryBodyURL)
	}

	if input.PathwayType != nil {
		addSet("pathway_type", *input.PathwayType)
	}

	if input.TimelineEstimate != nil {
		addSet("timeline_estimate", *input.TimelineEstimate)
	}

	if input.CostEstimateUSD != nil {
		addSet("cost_estimate_usd", *input.CostEstimateUSD)
	}

	if input.PathwayDescription != nil {
		addSet("pathway_description", *input.PathwayDescription)
	}

	if input.OfficialApplicationURL != nil {
		addSet("official_application_url", *input.OfficialApplicationURL)
	}

	if input.SourceURLLastChecked != nil {
		addSet("source_url_last_checked", *input.SourceURLLastChecked)
	}

	if input.Status != nil {
		addSet("status", *input.Status)
	}

	if input.IsActive != nil {
		addSet("is_active", *input.IsActive)
	}

	if len(sets) == 0 {
		return nil
	}

	sets = append(sets, "updated_at = now()")

	args = append(args, id)
	q := fmt.Sprintf(`
UPDATE credential_pathways
SET %s
WHERE id = $%d
`, strings.Join(sets, ", "), argIdx)

	cmd, err := s.pool.Writer.Exec(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("credentialchecker: update pathway: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// DeletePathway removes a non-published pathway. Published pathways are
// protected — deactivate them instead (is_active=false), same guard as
// eca_guides: agent and admin callers can't hard-delete live content.
// Returns pgx.ErrNoRows if the row does not exist or is published.
func (s *Store) DeletePathway(ctx context.Context, id pgtype.UUID) error {
	const q = `
DELETE FROM credential_pathways
WHERE id = $1 AND status IN ('draft','ai_draft','pending','rejected')
`

	cmd, err := s.pool.Writer.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("credentialchecker: delete pathway: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// ApprovePathway marks a pathway as HITL-approved: sets hitl_reviewed_by,
// hitl_reviewed_at = now(), status = 'published', and optional review
// notes. Returns pgx.ErrNoRows if the row does not exist for the platform.
func (s *Store) ApprovePathway(ctx context.Context, id, reviewerID pgtype.UUID, notes *string) error {
	const q = `
UPDATE credential_pathways
SET hitl_reviewed_by = $2,
    hitl_reviewed_at = now(),
    hitl_review_notes = $3,
    status = 'published',
    is_active = true,
    updated_at = now()
WHERE id = $1
`

	cmd, err := s.pool.Writer.Exec(ctx, q, id, reviewerID, notes)
	if err != nil {
		return fmt.Errorf("credentialchecker: approve pathway: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// RejectPathway marks a pathway rejected with HITL reviewer attribution.
// Mirrors ecaguide.RejectGuide — the reviewer who rejected is recorded so
// the queue keeps an audit trail (ADR-088).
func (s *Store) RejectPathway(ctx context.Context, id, reviewerID pgtype.UUID, notes *string) error {
	const q = `
UPDATE credential_pathways
SET hitl_reviewed_by = $2,
    hitl_reviewed_at = now(),
    hitl_review_notes = $3,
    status = 'rejected',
    is_active = false,
    updated_at = now()
WHERE id = $1
`

	cmd, err := s.pool.Writer.Exec(ctx, q, id, reviewerID, notes)
	if err != nil {
		return fmt.Errorf("credentialchecker: reject pathway: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// CreateNote inserts a community note with an explicit status — new
// submissions are 'pending' so they only appear publicly after admin
// moderation flips them to 'published'.
func (s *Store) CreateNote(ctx context.Context, pathwayID, userID pgtype.UUID, note, status string) (pgtype.UUID, pgtype.Timestamptz, error) {
	const q = `
INSERT INTO credential_community_notes (pathway_id, user_id, note, status)
VALUES ($1, $2, $3, $4)
RETURNING id, created_at
`

	var id pgtype.UUID

	var createdAt pgtype.Timestamptz

	err := s.pool.Writer.QueryRow(ctx, q, pathwayID, userID, note, status).Scan(&id, &createdAt)
	if err != nil {
		return pgtype.UUID{}, pgtype.Timestamptz{}, fmt.Errorf("credentialchecker: create note: %w", err)
	}

	return id, createdAt, nil
}

// CreateProfessionInput holds the fields for inserting a profession.
type CreateProfessionInput struct {
	Slug     string  `json:"slug"`
	Name     string  `json:"name"`
	Category *string `json:"category,omitempty"`
}

// CreateProfession inserts a new credential_professions row.
func (s *Store) CreateProfession(ctx context.Context, in CreateProfessionInput) (ProfessionRow, error) {
	const q = `
INSERT INTO credential_professions (slug, name, category)
VALUES ($1, $2, $3)
RETURNING id, slug, name, category
`

	var r ProfessionRow

	err := s.pool.Writer.QueryRow(ctx, q, in.Slug, in.Name, in.Category).Scan(
		&r.ID, &r.Slug, &r.Name, &r.Category,
	)
	if err != nil {
		return ProfessionRow{}, fmt.Errorf("credentialchecker: create profession: %w", err)
	}

	return r, nil
}

// UpdateProfessionInput holds the mutable fields for a profession update.
type UpdateProfessionInput struct {
	Slug     *string `json:"slug,omitempty"`
	Name     *string `json:"name,omitempty"`
	Category *string `json:"category,omitempty"`
}

// UpdateProfession updates a credential_professions row by id.
func (s *Store) UpdateProfession(ctx context.Context, id pgtype.UUID, in UpdateProfessionInput) (ProfessionRow, error) {
	var (
		sets   []string
		args   []any
		argIdx = 1
	)

	if in.Slug != nil {
		sets = append(sets, fmt.Sprintf("slug = $%d", argIdx))
		args = append(args, *in.Slug)
		argIdx++
	}

	if in.Name != nil {
		sets = append(sets, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *in.Name)
		argIdx++
	}

	if in.Category != nil {
		sets = append(sets, fmt.Sprintf("category = $%d", argIdx))
		args = append(args, *in.Category)
		argIdx++
	}

	if len(sets) == 0 {
		return s.GetProfessionByID(ctx, id)
	}

	sets = append(sets, "updated_at = now()")
	args = append(args, id)

	q := fmt.Sprintf(
		"UPDATE credential_professions SET %s WHERE id = $%d RETURNING id, slug, name, category",
		strings.Join(sets, ", "),
		argIdx,
	)

	var r ProfessionRow

	err := s.pool.Writer.QueryRow(ctx, q, args...).Scan(
		&r.ID, &r.Slug, &r.Name, &r.Category,
	)
	if err != nil {
		return ProfessionRow{}, fmt.Errorf("credentialchecker: update profession: %w", err)
	}

	return r, nil
}

// DeleteProfession deletes a credential_professions row by id.
func (s *Store) DeleteProfession(ctx context.Context, id pgtype.UUID) error {
	cmd, err := s.pool.Writer.Exec(ctx, `DELETE FROM credential_professions WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("credentialchecker: delete profession: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// pathwaySortColumns maps admin sortable column keys to safe SQL
// expressions — anything not in the map falls back to updated_at DESC so
// user input can never inject into ORDER BY.
var pathwaySortColumns = map[string]string{
	"profession_name":        "pr.name",
	"destination_space_name": "dst.name",
	"issuing_space_name":     "iss.name",
	"status":                 "p.status",
	"created_at":             "p.created_at",
	"updated_at":             "p.updated_at",
}

// parsePathwaySort parses the admin "key:dir" sort param (e.g.
// "profession_name:asc") into a whitelisted column + direction.
func parsePathwaySort(param string) (string, string, bool) {
	key, dir, _ := strings.Cut(param, ":")

	col, ok := pathwaySortColumns[key]
	if !ok {
		return "", "", false
	}

	if dir != "asc" {
		dir = "desc"
	}

	return col, dir, true
}

// AdminListNotes returns community notes for the admin moderation queue,
// joined with the author username and pathway context (profession +
// destination) so reviewers can judge the note without a second query.
func (s *Store) AdminListNotes(ctx context.Context, status string, limit, offset int32) ([]CommunityNoteRow, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	conditions = append(conditions, "n.deleted_at IS NULL")

	if status != "" {
		conditions = append(conditions, fmt.Sprintf("n.status = $%d", argIdx))
		args = append(args, status)
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")

	listQuery := fmt.Sprintf(`
SELECT n.id, n.pathway_id, n.user_id, u.username AS user_name,
       n.note, n.upvotes, n.status, n.created_at,
       pr.name AS profession_name, dst.name AS destination_space_name
FROM credential_community_notes n
JOIN credential_pathways p ON p.id = n.pathway_id
JOIN credential_professions pr ON pr.id = p.profession_id
LEFT JOIN spaces dst ON dst.id = p.destination_space_id
LEFT JOIN users u ON u.id = n.user_id
WHERE %s
ORDER BY n.created_at DESC
LIMIT $%d OFFSET $%d
`, whereClause, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("credentialchecker: admin list notes: %w", err)
	}
	defer rows.Close()

	var out []CommunityNoteRow

	for rows.Next() {
		var r CommunityNoteRow
		if err := rows.Scan(
			&r.ID, &r.PathwayID, &r.UserID, &r.UserName,
			&r.Note, &r.Upvotes, &r.Status, &r.CreatedAt,
			&r.ProfessionName, &r.DestinationSpaceName,
		); err != nil {
			return nil, 0, fmt.Errorf("credentialchecker: scan admin note row: %w", err)
		}

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("credentialchecker: iterate admin note rows: %w", err)
	}

	var total int64

	countQuery := fmt.Sprintf(
		`SELECT COUNT(*) FROM credential_community_notes n WHERE %s`, whereClause)

	if err := s.pool.Reader.QueryRow(ctx, countQuery, args[:len(args)-2]...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("credentialchecker: count admin notes: %w", err)
	}

	return out, total, nil
}

// ModerateNote sets a community note's status ('published' to approve,
// 'rejected' to reject). Returns pgx.ErrNoRows when the note does not
// exist or is already deleted.
func (s *Store) ModerateNote(ctx context.Context, id pgtype.UUID, status string) error {
	cmd, err := s.pool.Writer.Exec(ctx, `
UPDATE credential_community_notes
SET status = $2
WHERE id = $1 AND deleted_at IS NULL
`, id, status)
	if err != nil {
		return fmt.Errorf("credentialchecker: moderate note: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}
