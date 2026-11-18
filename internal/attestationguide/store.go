// Package attestationguide implements the Document Attestation &
// Apostille Guide (§97, ADR-105).
// // The guide publishes curated attestation pathways for documents that
// must be legalized/apostilled before use in a destination jurisdiction
// (e.g. a Jamaican birth certificate attested for use in the UAE). Each
// pathway records the issuing space, destination space, document type,
// whether an apostille is required, and an ordered list of steps
// (agency, what to bring, fee estimate, turnaround, official URL).
// Community members can submit updates to individual steps, which flow
// through a pending/approved/rejected moderation queue. AI-drafted
// pathways are supported via the Content Agent and gated by a
// human-in-the-loop (HITL) approval step before publication.
// // Ref: japaconnect-engineering/sections/97-attestation-guide.md
// //	japaconnect-engineering/adr/ADR-105-attestation-guide.md
package attestationguide

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
)

// Document types (§97) — kept in sync with the
// attestation_pathways_document_type_check CHECK constraint.
const (
	DocumentTypeBirthCertificate    = "birth_certificate"
	DocumentTypeDegree              = "degree"
	DocumentTypeAcademicTranscript  = "academic_transcript"
	DocumentTypeMarriage            = "marriage"
	DocumentTypeDeathCertificate    = "death_certificate"
	DocumentTypeDivorceCertificate  = "divorce_certificate"
	DocumentTypeSingleStatus        = "single_status"
	DocumentTypeAdoptionPapers      = "adoption_papers"
	DocumentTypePoliceClearance     = "police_clearance"
	DocumentTypeProfessionalLicence = "professional_licence"
	DocumentTypePowerOfAttorney     = "power_of_attorney"
	DocumentTypeDrivingLicence      = "driving_licence"
	DocumentTypeEmploymentLetter    = "employment_letter"
	DocumentTypeCompanyRegistration = "company_registration"
	DocumentTypeMedicalCertificate  = "medical_certificate"
	DocumentTypeOther               = "other"
)

// Pathway statuses (§97).
const (
	PathwayStatusDraft     = "draft"
	PathwayStatusAIDraft   = "ai_draft"
	PathwayStatusPending   = "pending"
	PathwayStatusPublished = "published"
	PathwayStatusRejected  = "rejected"
)

// Community update statuses (§97).
const (
	UpdateStatusPending  = "pending"
	UpdateStatusApproved = "approved"
	UpdateStatusRejected = "rejected"
)

// Store is the data-access layer for attestation_pathways and
// attestation_community_updates. It uses direct pgx queries because
// these tables do not have sqlc-generated queries yet. No business
// logic here.
type Store struct {
	pool *db.DB
}

// NewStore creates an attestation guide store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{pool: database}
}

// GetSpaceName returns the name of a space scoped by platform. It is used by
// the service to build human-readable, jurisdiction-specific web research
// queries. If the space is missing, not valid, or soft-deleted, it returns an
// empty string with no error.
func (s *Store) GetSpaceName(ctx context.Context, spaceID pgtype.UUID) (string, error) {
	if !spaceID.Valid {
		return "", nil
	}

	const q = `
SELECT name FROM spaces
WHERE id = $1 AND deleted_at IS NULL
LIMIT 1
`

	var name string

	err := s.pool.Reader.QueryRow(ctx, q, spaceID).Scan(&name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}

		return "", fmt.Errorf("attestationguide: get space name: %w", err)
	}

	return name, nil
}

// GetSpaceNames returns id → name for a batch of spaces in one query. Used
// by draft generation to resolve issuing + destination jurisdictions
// without two round-trips.
func (s *Store) GetSpaceNames(ctx context.Context, ids []pgtype.UUID) (map[pgtype.UUID]string, error) {
	out := make(map[pgtype.UUID]string, len(ids))

	uuids := make([]pgtype.UUID, 0, len(ids))
	for _, id := range ids {
		if id.Valid {
			uuids = append(uuids, id)
		}
	}

	if len(uuids) == 0 {
		return out, nil
	}

	const q = `
SELECT id, name FROM spaces
WHERE id = ANY($1) AND deleted_at IS NULL
`

	rows, err := s.pool.Reader.Query(ctx, q, uuids)
	if err != nil {
		return nil, fmt.Errorf("attestationguide: get space names: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id   pgtype.UUID
			name string
		)
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("attestationguide: scan space name: %w", err)
		}

		out[id] = name
	}

	return out, rows.Err()
}

// GuideSlugForPathway returns the slug of the published editorial guide
// (guide_category='attestation') linked to the given pathway — the
// tool→guide half of the cross-link (§97 editorial integration). A guide
// links either explicitly via field_values.pathway_id or implicitly via
// the matching field combination (document_type + issuing_country +
// destination_country geo codes) — the pathway's natural key, so an
// editor-written guide resolves without anyone pasting a UUID. An
// explicit pathway_id claim wins over a combination match on conflicts.
// Returns "" with nil error when no guide claims the pathway.
func (s *Store) GuideSlugForPathway(ctx context.Context, p PathwayRow) (string, error) {
	if !p.ID.Valid {
		return "", nil
	}

	var issGeo, destGeo *string
	if p.IssuingSpaceCode != nil && *p.IssuingSpaceCode != "" {
		issGeo = p.IssuingSpaceCode
	}
	if p.DestSpaceCode != nil && *p.DestSpaceCode != "" {
		destGeo = p.DestSpaceCode
	}

	const q = `
SELECT slug FROM contents
WHERE content_type = 'guide'
  AND guide_category = 'attestation'
  AND status = 'published'
  AND deleted_at IS NULL
  AND (
      field_values->>'pathway_id' = $1
      OR (
          -- Combination matching only claims guides without an explicit
          -- pathway_id — a guide that names a different pathway must not
          -- also surface on this one's detail page.
          COALESCE(BTRIM(field_values->>'pathway_id'), '') = ''
          AND field_values->>'document_type' = $2
          AND UPPER(field_values->>'issuing_country') = UPPER($3::text)
          AND UPPER(field_values->>'destination_country') = UPPER($4::text)
      )
  )
ORDER BY (field_values->>'pathway_id' = $1) DESC, created_at, id
LIMIT 1
`

	var slug string
	err := s.pool.Reader.QueryRow(ctx, q, p.ID.String(), p.DocumentType, issGeo, destGeo).Scan(&slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}

		return "", fmt.Errorf("attestationguide: guide slug for pathway: %w", err)
	}

	return slug, nil
}

// SpaceIDByCountryGeoCode resolves a country-level geo space by its
// geo_code (case-insensitive) — the reverse of the /geo/countries list
// the editorial guide fields are sourced from. Returns pgx.ErrNoRows
// when the code is unknown.
func (s *Store) SpaceIDByCountryGeoCode(ctx context.Context, geoCode string) (pgtype.UUID, error) {
	const q = `
SELECT id FROM spaces
WHERE level = 'country'
  AND UPPER(geo_code) = UPPER($1)
  AND deleted_at IS NULL
ORDER BY created_at, id
LIMIT 1
`

	var id pgtype.UUID
	if err := s.pool.Reader.QueryRow(ctx, q, geoCode).Scan(&id); err != nil {
		return pgtype.UUID{}, err
	}

	return id, nil
}

// PathwayIDForCombination returns the id of the pathway matching the
// (document_type, issuing_space, destination_space) natural key, in any
// status — a draft still claims the corridor so the publish hook never
// duplicates it. Read off Writer: this feeds a write decision (claim
// vs. create), where replica lag would duplicate a just-committed row.
// Returns pgx.ErrNoRows when no pathway matches.
func (s *Store) PathwayIDForCombination(ctx context.Context, documentType string, issuingSpaceID, destinationSpaceID pgtype.UUID) (pgtype.UUID, error) {
	const q = `
SELECT id FROM attestation_pathways
WHERE document_type = $1
  AND issuing_space_id = $2
  AND destination_space_id = $3
ORDER BY created_at, id
LIMIT 1
`

	var id pgtype.UUID
	if err := s.pool.Writer.QueryRow(ctx, q, documentType, issuingSpaceID, destinationSpaceID).Scan(&id); err != nil {
		return pgtype.UUID{}, err
	}

	return id, nil
}

// BackfillGuidePathwayID stamps field_values.pathway_id onto an editorial
// guide so an auto-resolved link becomes explicit (§97). field_values is
// JSONB-merged, not rewritten — concurrent edits to other keys are
// preserved. content_hash is untouched (Rule 12 covers body only).
func (s *Store) BackfillGuidePathwayID(ctx context.Context, contentID, pathwayID pgtype.UUID) error {
	const q = `
UPDATE contents
SET field_values = COALESCE(field_values, '{}'::jsonb) || jsonb_build_object('pathway_id', $2::text),
    updated_at = now()
WHERE id = $1
`

	if _, err := s.pool.Writer.Exec(ctx, q, contentID, pathwayID.String()); err != nil {
		return fmt.Errorf("attestationguide: backfill guide pathway_id: %w", err)
	}

	return nil
}

// Pathway holds a single attestation_pathways row.
type Pathway struct {
	ID pgtype.UUID

	DocumentType          string
	IssuingSpaceID        pgtype.UUID
	DestinationSpaceID    pgtype.UUID
	ApostilleRequired     bool
	PathwaySteps          []byte // JSON array of step objects
	CommunityLastVerified pgtype.Date
	Status                string
	IsActive              bool
	AIGenerated           bool
	AIModel               *string
	AIPromptHash          *string
	HITLReviewedBy        pgtype.UUID
	HITLReviewedAt        pgtype.Timestamptz
	HITLReviewNotes       *string
	IsJapabotIndexed      bool
	CreatedAt             pgtype.Timestamptz
	UpdatedAt             pgtype.Timestamptz
}

// PathwayRow is the joined result of attestation_pathways + spaces
// (issuing + destination), returned by list and get-by-id queries.
type PathwayRow struct {
	Pathway
	IssuingSpaceName *string
	IssuingSpaceCode *string
	DestSpaceName    *string
	DestSpaceCode    *string
}

// CommunityUpdate holds a single attestation_community_updates row.
// The Pathway*/UserDisplayName fields are populated only by
// ListCommunityUpdates (the admin moderation queue needs pathway context to
// review a submission without a second lookup).
type CommunityUpdate struct {
	ID             pgtype.UUID
	PathwayID      pgtype.UUID
	UserID         pgtype.UUID
	StepIndex      *int32
	FeeEstimate    *string
	TurnaroundDays *int32
	Note           *string
	Status         string
	ReviewedBy     pgtype.UUID
	CreatedAt      pgtype.Timestamptz

	PathwayDocumentType  string
	IssuingSpaceName     *string
	DestinationSpaceName *string
	SubmitterName        *string
	ReviewerName         *string
}

// ListPathwaysFilter holds the filter parameters for listing pathways.
type ListPathwaysFilter struct {
	DestinationSpaceID pgtype.UUID // optional
	DocumentType       string      // optional
	IssuingSpaceID     pgtype.UUID // optional
	Status             string      // default: published
	IsActive           *bool       // default: true
	Limit              int32
	Offset             int32
}

// ListPathways returns a paginated, filtered list of attestation
// pathways. The query joins attestation_pathways with spaces for the
// issuing and destination jurisdictions.
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
	} else {
		conditions = append(conditions, "p.status = 'published'")
	}

	if f.IsActive != nil {
		conditions = append(conditions, fmt.Sprintf("p.is_active = $%d", argIdx))
		args = append(args, *f.IsActive)
		argIdx++
	} else {
		conditions = append(conditions, "p.is_active = true")
	}

	if f.DestinationSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("p.destination_space_id = $%d", argIdx))
		args = append(args, f.DestinationSpaceID)
		argIdx++
	}

	if f.DocumentType != "" {
		// Match document_type case-insensitively, treating '_' and ' ' as
		// equivalent — the public UI accepts free text like "birth certificate"
		// while stored values use the canonical "birth_certificate" form (§97).
		conditions = append(conditions, fmt.Sprintf("lower(replace(p.document_type, '_', ' ')) = lower(replace($%d, '_', ' '))", argIdx))
		args = append(args, strings.TrimSpace(f.DocumentType))
		argIdx++
	}

	if f.IssuingSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("p.issuing_space_id = $%d", argIdx))
		args = append(args, f.IssuingSpaceID)
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")
	if whereClause == "" {
		whereClause = "TRUE"
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
SELECT p.id, p.document_type, p.issuing_space_id, p.destination_space_id,
       p.apostille_required, p.pathway_steps, p.community_last_verified, p.status,
       p.is_active, p.ai_generated, p.ai_model, p.ai_prompt_hash,
       p.hitl_reviewed_by, p.hitl_reviewed_at, p.hitl_review_notes,
       p.is_japabot_indexed, p.created_at, p.updated_at,
       iss.name AS issuing_space_name, iss.geo_code AS issuing_space_code,
       ds.name AS dest_space_name, ds.geo_code AS dest_space_code
FROM attestation_pathways p
LEFT JOIN spaces iss ON iss.id = p.issuing_space_id
LEFT JOIN spaces ds ON ds.id = p.destination_space_id
WHERE %s
ORDER BY p.updated_at DESC
LIMIT $%d OFFSET $%d
`, whereClause, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("attestationguide: list pathways: %w", err)
	}
	defer rows.Close()

	var out []PathwayRow

	for rows.Next() {
		var r PathwayRow
		if err := rows.Scan(
			&r.ID, &r.DocumentType, &r.IssuingSpaceID, &r.DestinationSpaceID,
			&r.ApostilleRequired, &r.PathwaySteps, &r.CommunityLastVerified, &r.Status,
			&r.IsActive, &r.AIGenerated, &r.AIModel, &r.AIPromptHash,
			&r.HITLReviewedBy, &r.HITLReviewedAt, &r.HITLReviewNotes,
			&r.IsJapabotIndexed, &r.CreatedAt, &r.UpdatedAt,
			&r.IssuingSpaceName, &r.IssuingSpaceCode,
			&r.DestSpaceName, &r.DestSpaceCode,
		); err != nil {
			return nil, 0, fmt.Errorf("attestationguide: scan pathway row: %w", err)
		}

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("attestationguide: iterate pathway rows: %w", err)
	}

	countQuery := fmt.Sprintf(`
SELECT COUNT(*) FROM attestation_pathways p
WHERE %s
`, whereClause)
	countArgs := args[:len(args)-2]

	var total int64

	if err := s.pool.Reader.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("attestationguide: count pathways: %w", err)
	}

	return out, total, nil
}

// GetPathwayByID returns a single attestation pathway by its id, joining
// spaces for the issuing and destination jurisdictions. Returns any status
// (used by admin handlers). Public handlers must use GetPublishedPathwayByID
// to avoid leaking draft/ai_draft/pending/rejected pathways (BUG-001).
func (s *Store) GetPathwayByID(ctx context.Context, id pgtype.UUID) (PathwayRow, error) {
	const q = `
SELECT p.id, p.document_type, p.issuing_space_id, p.destination_space_id,
       p.apostille_required, p.pathway_steps, p.community_last_verified, p.status,
       p.is_active, p.ai_generated, p.ai_model, p.ai_prompt_hash,
       p.hitl_reviewed_by, p.hitl_reviewed_at, p.hitl_review_notes,
       p.is_japabot_indexed, p.created_at, p.updated_at,
       iss.name AS issuing_space_name, iss.geo_code AS issuing_space_code,
       ds.name AS dest_space_name, ds.geo_code AS dest_space_code
FROM attestation_pathways p
LEFT JOIN spaces iss ON iss.id = p.issuing_space_id
LEFT JOIN spaces ds ON ds.id = p.destination_space_id
WHERE p.id = $1
LIMIT 1
`

	var r PathwayRow

	err := s.pool.Reader.QueryRow(ctx, q, id).Scan(
		&r.ID, &r.DocumentType, &r.IssuingSpaceID, &r.DestinationSpaceID,
		&r.ApostilleRequired, &r.PathwaySteps, &r.CommunityLastVerified, &r.Status,
		&r.IsActive, &r.AIGenerated, &r.AIModel, &r.AIPromptHash,
		&r.HITLReviewedBy, &r.HITLReviewedAt, &r.HITLReviewNotes,
		&r.IsJapabotIndexed, &r.CreatedAt, &r.UpdatedAt,
		&r.IssuingSpaceName, &r.IssuingSpaceCode,
		&r.DestSpaceName, &r.DestSpaceCode,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PathwayRow{}, pgx.ErrNoRows
		}

		return PathwayRow{}, fmt.Errorf("attestationguide: get pathway by id: %w", err)
	}

	return r, nil
}

// GetPublishedPathwayByID returns a single PUBLISHED, ACTIVE attestation
// pathway by its id. Used by public handlers (HandleGet, HandleExportPDF,
// SubmitCommunityUpdate) to prevent leaking draft/ai_draft/pending/rejected
// pathways to anonymous users (BUG-001).
func (s *Store) GetPublishedPathwayByID(ctx context.Context, id pgtype.UUID) (PathwayRow, error) {
	const q = `
SELECT p.id, p.document_type, p.issuing_space_id, p.destination_space_id,
       p.apostille_required, p.pathway_steps, p.community_last_verified, p.status,
       p.is_active, p.ai_generated, p.ai_model, p.ai_prompt_hash,
       p.hitl_reviewed_by, p.hitl_reviewed_at, p.hitl_review_notes,
       p.is_japabot_indexed, p.created_at, p.updated_at,
       iss.name AS issuing_space_name, iss.geo_code AS issuing_space_code,
       ds.name AS dest_space_name, ds.geo_code AS dest_space_code
FROM attestation_pathways p
LEFT JOIN spaces iss ON iss.id = p.issuing_space_id
LEFT JOIN spaces ds ON ds.id = p.destination_space_id
WHERE p.id = $1 AND p.status = 'published' AND p.is_active = true
LIMIT 1
`

	var r PathwayRow

	err := s.pool.Reader.QueryRow(ctx, q, id).Scan(
		&r.ID, &r.DocumentType, &r.IssuingSpaceID, &r.DestinationSpaceID,
		&r.ApostilleRequired, &r.PathwaySteps, &r.CommunityLastVerified, &r.Status,
		&r.IsActive, &r.AIGenerated, &r.AIModel, &r.AIPromptHash,
		&r.HITLReviewedBy, &r.HITLReviewedAt, &r.HITLReviewNotes,
		&r.IsJapabotIndexed, &r.CreatedAt, &r.UpdatedAt,
		&r.IssuingSpaceName, &r.IssuingSpaceCode,
		&r.DestSpaceName, &r.DestSpaceCode,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PathwayRow{}, pgx.ErrNoRows
		}

		return PathwayRow{}, fmt.Errorf("attestationguide: get published pathway by id: %w", err)
	}

	return r, nil
}

// CreatePathway inserts a new attestation_pathways row.
// is_japabot_indexed is a GENERATED column and is NOT inserted.
func (s *Store) CreatePathway(ctx context.Context, p Pathway) (Pathway, error) {
	const q = `
INSERT INTO attestation_pathways (
    document_type, issuing_space_id, destination_space_id,
    apostille_required, pathway_steps, community_last_verified, status,
    is_active, ai_generated, ai_model, ai_prompt_hash,
    hitl_reviewed_by, hitl_reviewed_at, hitl_review_notes
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
RETURNING id, document_type, issuing_space_id, destination_space_id,
          apostille_required, pathway_steps, community_last_verified, status,
          is_active, ai_generated, ai_model, ai_prompt_hash,
          hitl_reviewed_by, hitl_reviewed_at, hitl_review_notes,
          is_japabot_indexed, created_at, updated_at
`

	steps := p.PathwaySteps
	if steps == nil {
		steps = []byte("[]")
	}

	status := p.Status
	if status == "" {
		status = PathwayStatusPublished
	}

	var out Pathway

	err := s.pool.Writer.QueryRow(
		ctx, q,
		p.DocumentType, p.IssuingSpaceID, p.DestinationSpaceID,
		p.ApostilleRequired, steps, p.CommunityLastVerified, status,
		p.IsActive, p.AIGenerated, p.AIModel, p.AIPromptHash,
		p.HITLReviewedBy, p.HITLReviewedAt, p.HITLReviewNotes,
	).Scan(
		&out.ID, &out.DocumentType, &out.IssuingSpaceID, &out.DestinationSpaceID,
		&out.ApostilleRequired, &out.PathwaySteps, &out.CommunityLastVerified, &out.Status,
		&out.IsActive, &out.AIGenerated, &out.AIModel, &out.AIPromptHash,
		&out.HITLReviewedBy, &out.HITLReviewedAt, &out.HITLReviewNotes,
		&out.IsJapabotIndexed, &out.CreatedAt, &out.UpdatedAt,
	)
	if err != nil {
		return Pathway{}, fmt.Errorf("attestationguide: create pathway: %w", err)
	}

	return out, nil
}

// GetPathwayForUpdate locks a pathway row inside the given transaction and
// returns it — the read-modify-write pair for UpdatePathwayInTx. Without the
// lock, two concurrent PATCHes each overwrite the other's changes (the
// update writes the full row).
func (s *Store) GetPathwayForUpdate(ctx context.Context, tx pgx.Tx, id pgtype.UUID) (Pathway, error) {
	const q = `
SELECT id, document_type, issuing_space_id, destination_space_id,
       apostille_required, pathway_steps, community_last_verified, status,
       is_active, ai_generated, ai_model, ai_prompt_hash,
       hitl_reviewed_by, hitl_reviewed_at, hitl_review_notes,
       is_japabot_indexed, created_at, updated_at
FROM attestation_pathways
WHERE id = $1
FOR UPDATE
`

	var p Pathway

	err := tx.QueryRow(ctx, q, id).Scan(
		&p.ID, &p.DocumentType, &p.IssuingSpaceID, &p.DestinationSpaceID,
		&p.ApostilleRequired, &p.PathwaySteps, &p.CommunityLastVerified, &p.Status,
		&p.IsActive, &p.AIGenerated, &p.AIModel, &p.AIPromptHash,
		&p.HITLReviewedBy, &p.HITLReviewedAt, &p.HITLReviewNotes,
		&p.IsJapabotIndexed, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return Pathway{}, err
	}

	return p, nil
}

// pgxExec is satisfied by the writer pool and by pgx.Tx — lets
// updatePathwayExec serve both direct and transactional calls.
type pgxExec interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// UpdatePathwayInTx is the only writer of mutable pathway fields — always
// pair it with GetPathwayForUpdate (see MutatePathway) so the
// read-modify-write happens under one row lock. A non-tx variant is
// deliberately omitted: calling it would silently lose concurrent updates
// because the UPDATE writes the full row.
// is_japabot_indexed is a GENERATED column and is NOT updated.
func (s *Store) UpdatePathwayInTx(ctx context.Context, tx pgx.Tx, id pgtype.UUID, p Pathway) error {
	return updatePathwayExec(ctx, tx, id, p)
}

func updatePathwayExec(ctx context.Context, ex pgxExec, id pgtype.UUID, p Pathway) error {
	steps := p.PathwaySteps
	if steps == nil {
		steps = []byte("[]")
	}

	const q = `
UPDATE attestation_pathways
SET document_type        = $2,
    issuing_space_id     = $3,
    destination_space_id = $4,
    apostille_required   = $5,
    pathway_steps        = $6,
    community_last_verified = $7,
    status               = $8,
    is_active            = $9,
    ai_generated         = $10,
    ai_model             = $11,
    ai_prompt_hash       = $12,
    hitl_review_notes    = $13,
    updated_at           = now()
WHERE id = $1
`

	cmd, err := ex.Exec(
		ctx, q,
		id,
		p.DocumentType, p.IssuingSpaceID, p.DestinationSpaceID,
		p.ApostilleRequired, steps, p.CommunityLastVerified, p.Status,
		p.IsActive, p.AIGenerated, p.AIModel, p.AIPromptHash,
		p.HITLReviewNotes,
	)
	if err != nil {
		return fmt.Errorf("attestationguide: update pathway: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// MutatePathway locks a pathway row FOR UPDATE, applies fn to it, writes
// it back, and commits — one atomic read-modify-write. fn mutates the
// locked row in place; a fn error rolls the transaction back untouched.
// pgx.ErrNoRows propagates for a missing id.
func (s *Store) MutatePathway(ctx context.Context, id pgtype.UUID, fn func(*Pathway) error) error {
	tx, err := s.pool.Writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("attestationguide: mutate pathway: begin tx: %w", err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	p, err := s.GetPathwayForUpdate(ctx, tx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("attestationguide: mutate pathway: lock: %w", err)
	}

	if err := fn(&p); err != nil {
		return err
	}

	if err := s.UpdatePathwayInTx(ctx, tx, id, p); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("attestationguide: mutate pathway: commit: %w", err)
	}

	return nil
}

// ApprovePathway performs the HITL approval: sets hitl_reviewed_by,
// hitl_reviewed_at = now(), status = 'published', recomputes
// apostille_required from the Hague Convention lookup (BUG-002), and sets
// community_last_verified = now()::date if it was NULL (BUG-003).
// Optionally records review notes.
func (s *Store) ApprovePathway(ctx context.Context, id, reviewerID pgtype.UUID, apostilleRequired bool, notes *string) error {
	// status != 'published' in the WHERE closes the TOCTOU window: two
	// concurrent approvals can both pass the service-level check, but only
	// the first write lands — the second affects 0 rows.
	const q = `
UPDATE attestation_pathways
SET hitl_reviewed_by        = $2,
    hitl_reviewed_at        = now(),
    hitl_review_notes      = $3,
    status                 = 'published',
    is_active              = true,
    apostille_required     = $4,
    community_last_verified = COALESCE(community_last_verified, now()::date),
    updated_at             = now()
WHERE id = $1 AND status != 'published'
`

	cmd, err := s.pool.Writer.Exec(ctx, q, id, reviewerID, notes, apostilleRequired)
	if err != nil {
		return fmt.Errorf("attestationguide: approve pathway: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// CreateCommunityUpdate inserts a new attestation_community_updates row
// in 'pending' status.
func (s *Store) CreateCommunityUpdate(ctx context.Context, u CommunityUpdate) (CommunityUpdate, error) {
	const q = `
INSERT INTO attestation_community_updates (
    pathway_id, user_id, step_index, fee_estimate, turnaround_days, note, status
) VALUES ($1, $2, $3, $4, $5, $6, COALESCE($7, 'pending'))
RETURNING id, pathway_id, user_id, step_index, fee_estimate, turnaround_days,
          note, status, reviewed_by, created_at
`

	var out CommunityUpdate

	err := s.pool.Writer.QueryRow(
		ctx, q,
		u.PathwayID, u.UserID, u.StepIndex, u.FeeEstimate, u.TurnaroundDays, u.Note, u.Status,
	).Scan(
		&out.ID, &out.PathwayID, &out.UserID, &out.StepIndex, &out.FeeEstimate,
		&out.TurnaroundDays, &out.Note, &out.Status, &out.ReviewedBy, &out.CreatedAt,
	)
	if err != nil {
		return CommunityUpdate{}, fmt.Errorf("attestationguide: create community update: %w", err)
	}

	return out, nil
}

// ReviewCommunityUpdate sets the status of a community update to
// approved or rejected and records the reviewer. The ID scopes
// the update through its parent pathway to prevent cross-platform access.
func (s *Store) ReviewCommunityUpdate(ctx context.Context, id, reviewerID pgtype.UUID, status string) error {
	// status='pending' in the WHERE closes the TOCTOU window: two
	// concurrent reviews can both pass the service-level pending check,
	// but only the first write lands — the second affects 0 rows.
	const q = `
UPDATE attestation_community_updates u
SET status      = $2,
    reviewed_by = $3
FROM attestation_pathways p
WHERE u.id = $1
  AND u.pathway_id = p.id
  AND u.deleted_at IS NULL
  AND u.status = 'pending'
`

	cmd, err := s.pool.Writer.Exec(ctx, q, id, status, reviewerID)
	if err != nil {
		return fmt.Errorf("attestationguide: review community update: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// ApplyApprovedUpdateToPathway patches the parent pathway's pathway_steps
// JSONB with the approved community update values (fee_estimate /
// turnaround_days at the given step_index) and bumps community_last_verified
// to today (BUG-003 + BUG-004). If stepIndex is nil the steps are not patched
// (general note update) but community_last_verified is still bumped.
func (s *Store) ApplyApprovedUpdateToPathway(ctx context.Context, pathwayID pgtype.UUID, stepIndex *int32, feeEstimate *string, turnaroundDays *int32) error {
	// Read-modify-write inside a writer transaction with FOR UPDATE so a
	// concurrent approval cannot clobber an in-flight steps patch.
	tx, err := s.pool.Writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("attestationguide: apply update: begin tx: %w", err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	// Load current pathway_steps so we can patch in Go (cleaner than nested
	// jsonb_set SQL for conditional fields).
	const loadQ = `SELECT pathway_steps FROM attestation_pathways WHERE id = $1 FOR UPDATE`

	var steps []byte
	if err := tx.QueryRow(ctx, loadQ, pathwayID).Scan(&steps); err != nil {
		return fmt.Errorf("attestationguide: apply update: load steps: %w", err)
	}

	if stepIndex != nil {
		var arr []map[string]any
		if err := json.Unmarshal(steps, &arr); err != nil {
			return fmt.Errorf("attestationguide: apply update: decode steps: %w", err)
		}

		idx := int(*stepIndex)
		if idx < 0 || idx >= len(arr) {
			// Submit-side validation covers this, but a step list can shrink
			// between submission and review — fail visibly rather than bump
			// community_last_verified on a no-op patch.
			return fmt.Errorf("attestationguide: apply update: step_index %d out of range (%d steps)", idx, len(arr))
		}

		// A JSON null element decodes as a nil map — assigning into it would
		// panic. A null step has no place to put fee/turnaround fields, so
		// fail visibly instead.
		if arr[idx] == nil {
			return fmt.Errorf("attestationguide: apply update: step_index %d is not a step object", idx)
		}

		if feeEstimate != nil {
			arr[idx]["fee_estimate"] = *feeEstimate
		}

		if turnaroundDays != nil {
			arr[idx]["turnaround_days"] = *turnaroundDays
		}

		patched, err := json.Marshal(arr)
		if err != nil {
			return fmt.Errorf("attestationguide: apply update: encode steps: %w", err)
		}

		steps = patched
	}

	const q = `
UPDATE attestation_pathways
SET pathway_steps            = $2,
    community_last_verified = now()::date,
    updated_at              = now()
WHERE id = $1`

	cmd, err := tx.Exec(ctx, q, pathwayID, steps)
	if err != nil {
		return fmt.Errorf("attestationguide: apply update: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("attestationguide: apply update: commit: %w", err)
	}

	return nil
}

// GetCommunityUpdateByID returns a single attestation_community_updates
// row by its id, scoped through the parent pathway's platform.
func (s *Store) GetCommunityUpdateByID(ctx context.Context, id pgtype.UUID) (CommunityUpdate, error) {
	const q = `
SELECT u.id, u.pathway_id, u.user_id, u.step_index, u.fee_estimate,
       u.turnaround_days, u.note, u.status, u.reviewed_by, u.created_at,
       p.document_type, iss.name, ds.name,
       COALESCE(up.display_name, usr.username) AS submitter_name,
       COALESCE(rp.display_name, rusr.username) AS reviewer_name
FROM attestation_community_updates u
JOIN attestation_pathways p ON p.id = u.pathway_id
LEFT JOIN spaces iss ON iss.id = p.issuing_space_id
LEFT JOIN spaces ds ON ds.id = p.destination_space_id
LEFT JOIN users usr ON usr.id = u.user_id
LEFT JOIN user_profiles up ON up.user_id = usr.id
LEFT JOIN users rusr ON rusr.id = u.reviewed_by
LEFT JOIN user_profiles rp ON rp.user_id = rusr.id
WHERE u.id = $1 AND u.deleted_at IS NULL
LIMIT 1
`

	var u CommunityUpdate

	err := s.pool.Reader.QueryRow(ctx, q, id).Scan(
		&u.ID, &u.PathwayID, &u.UserID, &u.StepIndex, &u.FeeEstimate,
		&u.TurnaroundDays, &u.Note, &u.Status, &u.ReviewedBy, &u.CreatedAt,
		&u.PathwayDocumentType, &u.IssuingSpaceName, &u.DestinationSpaceName,
		&u.SubmitterName, &u.ReviewerName,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CommunityUpdate{}, pgx.ErrNoRows
		}

		return CommunityUpdate{}, fmt.Errorf("attestationguide: get community update by id: %w", err)
	}

	return u, nil
}

// ListCommunityUpdatesFilter holds filter parameters for listing community
// updates in the admin moderation queue (BUG-005).
type ListCommunityUpdatesFilter struct {
	Status string // optional; default: pending
	Limit  int32
	Offset int32
}

// ListCommunityUpdates returns a paginated list of attestation_community_updates
// scoped to the platform via the parent pathway. Used by the admin moderation
// queue (GET /admin/attestation/updates — BUG-005).
func (s *Store) ListCommunityUpdates(ctx context.Context, f ListCommunityUpdatesFilter) ([]CommunityUpdate, int64, error) {
	var (
		conditions = []string{"u.deleted_at IS NULL"}
		args       = []any{}
		argIdx     = 1
	)

	if f.Status != "" {
		conditions = append(conditions, fmt.Sprintf("u.status = $%d", argIdx))
		args = append(args, f.Status)
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")
	if whereClause == "" {
		whereClause = "TRUE"
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
SELECT u.id, u.pathway_id, u.user_id, u.step_index, u.fee_estimate,
       u.turnaround_days, u.note, u.status, u.reviewed_by, u.created_at,
       p.document_type, iss.name, ds.name,
       COALESCE(up.display_name, usr.username) AS submitter_name,
       COALESCE(rp.display_name, rusr.username) AS reviewer_name
FROM attestation_community_updates u
JOIN attestation_pathways p ON p.id = u.pathway_id
LEFT JOIN spaces iss ON iss.id = p.issuing_space_id
LEFT JOIN spaces ds ON ds.id = p.destination_space_id
LEFT JOIN users usr ON usr.id = u.user_id
LEFT JOIN user_profiles up ON up.user_id = usr.id
LEFT JOIN users rusr ON rusr.id = u.reviewed_by
LEFT JOIN user_profiles rp ON rp.user_id = rusr.id
WHERE %s
ORDER BY u.created_at ASC
LIMIT $%d OFFSET $%d
`, whereClause, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("attestationguide: list community updates: %w", err)
	}
	defer rows.Close()

	var out []CommunityUpdate

	for rows.Next() {
		var u CommunityUpdate
		if err := rows.Scan(
			&u.ID, &u.PathwayID, &u.UserID, &u.StepIndex, &u.FeeEstimate,
			&u.TurnaroundDays, &u.Note, &u.Status, &u.ReviewedBy, &u.CreatedAt,
			&u.PathwayDocumentType, &u.IssuingSpaceName, &u.DestinationSpaceName,
			&u.SubmitterName, &u.ReviewerName,
		); err != nil {
			return nil, 0, fmt.Errorf("attestationguide: scan community update row: %w", err)
		}

		out = append(out, u)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("attestationguide: iterate community update rows: %w", err)
	}

	countQuery := fmt.Sprintf(`
SELECT COUNT(*) FROM attestation_community_updates u
JOIN attestation_pathways p ON p.id = u.pathway_id
WHERE %s
`, whereClause)
	countArgs := args[:len(args)-2]

	var total int64

	if err := s.pool.Reader.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("attestationguide: count community updates: %w", err)
	}

	return out, total, nil
}

// AdminListFilter holds filter parameters for the admin pathway list.
type AdminListFilter struct {
	Status             string      // optional
	DocumentType       string      // optional
	DestinationSpaceID pgtype.UUID // optional
	IsActive           *bool       // optional
	Search             string      // optional — q param (BUG-004)
	SortBy             string      // optional — column key (BUG-004)
	SortDir            string      // optional — "asc" | "desc" (BUG-004)
	Limit              int32
	Offset             int32
}

// adminSortColumn maps a frontend column key to a qualified SQL column.
// Only whitelisted columns are accepted to prevent SQL injection (BUG-004).
var adminSortColumns = map[string]string{
	"document_type":          "p.document_type",
	"issuing_space_name":     "iss.name",
	"destination_space_name": "ds.name",
	"status":                 "p.status",
	"updated_at":             "p.updated_at",
	"created_at":             "p.created_at",
}

// AdminList returns all attestation pathways (any status) for the
// moderation queue.
func (s *Store) AdminList(ctx context.Context, f AdminListFilter) ([]PathwayRow, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	if f.Status != "" {
		conditions = append(conditions, fmt.Sprintf("p.status = $%d", argIdx))
		args = append(args, f.Status)
		argIdx++
	}

	if f.DocumentType != "" {
		conditions = append(conditions, fmt.Sprintf("lower(replace(p.document_type, '_', ' ')) = lower(replace($%d, '_', ' '))", argIdx))
		args = append(args, strings.TrimSpace(f.DocumentType))
		argIdx++
	}

	if f.DestinationSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("p.destination_space_id = $%d", argIdx))
		args = append(args, f.DestinationSpaceID)
		argIdx++
	}

	if f.IsActive != nil {
		conditions = append(conditions, fmt.Sprintf("p.is_active = $%d", argIdx))
		args = append(args, *f.IsActive)
		argIdx++
	}
	// BUG-004: search (q) — case-insensitive ILIKE on document_type.
	// % and _ are ILIKE wildcards — escape them so a literal search for
	// e.g. "police_clearance" does not match unintended rows.
	if f.Search != "" {
		escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(f.Search)
		conditions = append(conditions, fmt.Sprintf(`p.document_type ILIKE $%d ESCAPE '\'`, argIdx))
		args = append(args, "%"+escaped+"%")
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")
	if whereClause == "" {
		whereClause = "TRUE"
	}

	// BUG-004: sort — whitelist column + direction to prevent SQL injection.
	orderCol := "p.created_at"
	if col, ok := adminSortColumns[f.SortBy]; ok {
		orderCol = col
	}

	orderDir := "DESC"
	if strings.ToLower(f.SortDir) == "asc" {
		orderDir = "ASC"
	}

	orderByClause := fmt.Sprintf("ORDER BY %s %s", orderCol, orderDir)

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

	listQuery := fmt.Sprintf(`
SELECT p.id, p.document_type, p.issuing_space_id, p.destination_space_id,
       p.apostille_required, p.pathway_steps, p.community_last_verified, p.status,
       p.is_active, p.ai_generated, p.ai_model, p.ai_prompt_hash,
       p.hitl_reviewed_by, p.hitl_reviewed_at, p.hitl_review_notes,
       p.is_japabot_indexed, p.created_at, p.updated_at,
       iss.name AS issuing_space_name, iss.geo_code AS issuing_space_code,
       ds.name AS dest_space_name, ds.geo_code AS dest_space_code
FROM attestation_pathways p
LEFT JOIN spaces iss ON iss.id = p.issuing_space_id
LEFT JOIN spaces ds ON ds.id = p.destination_space_id
WHERE %s
%s
LIMIT $%d OFFSET $%d
`, whereClause, orderByClause, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("attestationguide: admin list pathways: %w", err)
	}
	defer rows.Close()

	var out []PathwayRow

	for rows.Next() {
		var r PathwayRow
		if err := rows.Scan(
			&r.ID, &r.DocumentType, &r.IssuingSpaceID, &r.DestinationSpaceID,
			&r.ApostilleRequired, &r.PathwaySteps, &r.CommunityLastVerified, &r.Status,
			&r.IsActive, &r.AIGenerated, &r.AIModel, &r.AIPromptHash,
			&r.HITLReviewedBy, &r.HITLReviewedAt, &r.HITLReviewNotes,
			&r.IsJapabotIndexed, &r.CreatedAt, &r.UpdatedAt,
			&r.IssuingSpaceName, &r.IssuingSpaceCode,
			&r.DestSpaceName, &r.DestSpaceCode,
		); err != nil {
			return nil, 0, fmt.Errorf("attestationguide: admin scan pathway row: %w", err)
		}

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("attestationguide: admin iterate pathway rows: %w", err)
	}

	countQuery := fmt.Sprintf(`
SELECT COUNT(*) FROM attestation_pathways p
WHERE %s
`, whereClause)
	countArgs := args[:len(args)-2]

	var total int64

	if err := s.pool.Reader.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("attestationguide: admin count pathways: %w", err)
	}

	return out, total, nil
}
