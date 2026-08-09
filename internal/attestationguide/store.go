// Package attestationguide implements the Document Attestation &
// Apostille Guide (§97, ADR-105).
//
// The guide publishes curated attestation pathways for documents that
// must be legalized/apostilled before use in a destination jurisdiction
// (e.g. a Jamaican birth certificate attested for use in the UAE). Each
// pathway records the issuing space, destination space, document type,
// whether an apostille is required, and an ordered list of steps
// (agency, what to bring, fee estimate, turnaround, official URL).
// Community members can submit updates to individual steps, which flow
// through a pending/approved/rejected moderation queue. AI-drafted
// pathways are supported via the Content Agent and gated by a
// human-in-the-loop (HITL) approval step before publication.
//
// Ref: japaconnect-engineering/sections/97-attestation-guide.md
//      japaconnect-engineering/adr/ADR-105-attestation-guide.md
package attestationguide

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
)

// Document types (§97).
const (
	DocumentTypeBirthCertificate    = "birth_certificate"
	DocumentTypeDegree              = "degree"
	DocumentTypeMarriage            = "marriage"
	DocumentTypePoliceClearance     = "police_clearance"
	DocumentTypeProfessionalLicence = "professional_licence"
	DocumentTypeOther               = "other"
)

// Pathway statuses (§97).
const (
	PathwayStatusDraft    = "draft"
	PathwayStatusAIDraft  = "ai_draft"
	PathwayStatusPending  = "pending"
	PathwayStatusPublished = "published"
	PathwayStatusRejected = "rejected"
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

// Pathway holds a single attestation_pathways row.
type Pathway struct {
	ID                     pgtype.UUID
	TenantID               pgtype.UUID
	DocumentType           string
	IssuingSpaceID         pgtype.UUID
	DestinationSpaceID     pgtype.UUID
	ApostilleRequired      bool
	PathwaySteps           []byte // JSON array of step objects
	CommunityLastVerified  pgtype.Date
	Status                 string
	IsActive               bool
	AIGenerated            bool
	AIModel                *string
	AIPromptHash           *string
	HITLReviewedBy         pgtype.UUID
	HITLReviewedAt         pgtype.Timestamptz
	HITLReviewNotes        *string
	IsJapabotIndexed       bool
	CreatedAt              pgtype.Timestamptz
	UpdatedAt              pgtype.Timestamptz
}

// PathwayRow is the joined result of attestation_pathways + spaces
// (issuing + destination), returned by list and get-by-id queries.
type PathwayRow struct {
	Pathway
	IssuingSpaceName     *string
	IssuingSpaceCode     *string
	DestSpaceName        *string
	DestSpaceCode        *string
}

// CommunityUpdate holds a single attestation_community_updates row.
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
}

// ListPathwaysFilter holds the filter parameters for listing pathways.
type ListPathwaysFilter struct {
	TenantID           pgtype.UUID
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

	conditions = append(conditions, fmt.Sprintf("p.tenant_id = $%d", argIdx))
	args = append(args, f.TenantID)
	argIdx++

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
		conditions = append(conditions, fmt.Sprintf("p.document_type = $%d", argIdx))
		args = append(args, f.DocumentType)
		argIdx++
	}
	if f.IssuingSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("p.issuing_space_id = $%d", argIdx))
		args = append(args, f.IssuingSpaceID)
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")

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
SELECT p.id, p.tenant_id, p.document_type, p.issuing_space_id, p.destination_space_id,
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
			&r.ID, &r.TenantID, &r.DocumentType, &r.IssuingSpaceID, &r.DestinationSpaceID,
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
// spaces for the issuing and destination jurisdictions.
func (s *Store) GetPathwayByID(ctx context.Context, id, tenantID pgtype.UUID) (PathwayRow, error) {
	const q = `
SELECT p.id, p.tenant_id, p.document_type, p.issuing_space_id, p.destination_space_id,
       p.apostille_required, p.pathway_steps, p.community_last_verified, p.status,
       p.is_active, p.ai_generated, p.ai_model, p.ai_prompt_hash,
       p.hitl_reviewed_by, p.hitl_reviewed_at, p.hitl_review_notes,
       p.is_japabot_indexed, p.created_at, p.updated_at,
       iss.name AS issuing_space_name, iss.geo_code AS issuing_space_code,
       ds.name AS dest_space_name, ds.geo_code AS dest_space_code
FROM attestation_pathways p
LEFT JOIN spaces iss ON iss.id = p.issuing_space_id
LEFT JOIN spaces ds ON ds.id = p.destination_space_id
WHERE p.id = $1 AND p.tenant_id = $2
LIMIT 1
`
	var r PathwayRow
	err := s.pool.Reader.QueryRow(ctx, q, id, tenantID).Scan(
		&r.ID, &r.TenantID, &r.DocumentType, &r.IssuingSpaceID, &r.DestinationSpaceID,
		&r.ApostilleRequired, &r.PathwaySteps, &r.CommunityLastVerified, &r.Status,
		&r.IsActive, &r.AIGenerated, &r.AIModel, &r.AIPromptHash,
		&r.HITLReviewedBy, &r.HITLReviewedAt, &r.HITLReviewNotes,
		&r.IsJapabotIndexed, &r.CreatedAt, &r.UpdatedAt,
		&r.IssuingSpaceName, &r.IssuingSpaceCode,
		&r.DestSpaceName, &r.DestSpaceCode,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return PathwayRow{}, pgx.ErrNoRows
		}
		return PathwayRow{}, fmt.Errorf("attestationguide: get pathway by id: %w", err)
	}
	return r, nil
}

// CreatePathway inserts a new attestation_pathways row.
func (s *Store) CreatePathway(ctx context.Context, p Pathway) (Pathway, error) {
	const q = `
INSERT INTO attestation_pathways (
    tenant_id, document_type, issuing_space_id, destination_space_id,
    apostille_required, pathway_steps, community_last_verified, status,
    is_active, ai_generated, ai_model, ai_prompt_hash,
    hitl_reviewed_by, hitl_reviewed_at, hitl_review_notes, is_japabot_indexed
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
RETURNING id, tenant_id, document_type, issuing_space_id, destination_space_id,
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
	err := s.pool.Writer.QueryRow(ctx, q,
		p.TenantID, p.DocumentType, p.IssuingSpaceID, p.DestinationSpaceID,
		p.ApostilleRequired, steps, p.CommunityLastVerified, status,
		p.IsActive, p.AIGenerated, p.AIModel, p.AIPromptHash,
		p.HITLReviewedBy, p.HITLReviewedAt, p.HITLReviewNotes, p.IsJapabotIndexed,
	).Scan(
		&out.ID, &out.TenantID, &out.DocumentType, &out.IssuingSpaceID, &out.DestinationSpaceID,
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

// UpdatePathway updates the mutable fields on an attestation_pathways
// row. Only the provided (non-nil) fields are applied.
func (s *Store) UpdatePathway(ctx context.Context, id, tenantID pgtype.UUID, p Pathway) error {
	steps := p.PathwaySteps
	if steps == nil {
		steps = []byte("[]")
	}
	const q = `
UPDATE attestation_pathways
SET document_type        = $3,
    issuing_space_id     = $4,
    destination_space_id = $5,
    apostille_required   = $6,
    pathway_steps        = $7,
    community_last_verified = $8,
    status               = $9,
    is_active            = $10,
    ai_generated         = $11,
    ai_model             = $12,
    ai_prompt_hash       = $13,
    hitl_review_notes    = $14,
    is_japabot_indexed   = $15,
    updated_at           = now()
WHERE id = $1 AND tenant_id = $2
`
	cmd, err := s.pool.Writer.Exec(ctx, q,
		id, tenantID,
		p.DocumentType, p.IssuingSpaceID, p.DestinationSpaceID,
		p.ApostilleRequired, steps, p.CommunityLastVerified, p.Status,
		p.IsActive, p.AIGenerated, p.AIModel, p.AIPromptHash,
		p.HITLReviewNotes, p.IsJapabotIndexed,
	)
	if err != nil {
		return fmt.Errorf("attestationguide: update pathway: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// ApprovePathway performs the HITL approval: sets hitl_reviewed_by,
// hitl_reviewed_at = now(), status = 'published', and optionally records
// review notes.
func (s *Store) ApprovePathway(ctx context.Context, id, tenantID, reviewerID pgtype.UUID, notes *string) error {
	const q = `
UPDATE attestation_pathways
SET hitl_reviewed_by   = $3,
    hitl_reviewed_at   = now(),
    hitl_review_notes  = $4,
    status             = 'published',
    updated_at         = now()
WHERE id = $1 AND tenant_id = $2
`
	cmd, err := s.pool.Writer.Exec(ctx, q, id, tenantID, reviewerID, notes)
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
	err := s.pool.Writer.QueryRow(ctx, q,
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
// approved or rejected and records the reviewer.
func (s *Store) ReviewCommunityUpdate(ctx context.Context, id, reviewerID pgtype.UUID, status string) error {
	const q = `
UPDATE attestation_community_updates
SET status      = $3,
    reviewed_by = $4
WHERE id = $1 AND deleted_at IS NULL
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

// AdminListFilter holds filter parameters for the admin pathway list.
type AdminListFilter struct {
	TenantID           pgtype.UUID
	Status             string // optional
	DocumentType       string // optional
	DestinationSpaceID pgtype.UUID // optional
	IsActive           *bool  // optional
	Limit              int32
	Offset             int32
}

// AdminList returns all attestation pathways (any status) for the
// moderation queue.
func (s *Store) AdminList(ctx context.Context, f AdminListFilter) ([]PathwayRow, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	conditions = append(conditions, fmt.Sprintf("p.tenant_id = $%d", argIdx))
	args = append(args, f.TenantID)
	argIdx++

	if f.Status != "" {
		conditions = append(conditions, fmt.Sprintf("p.status = $%d", argIdx))
		args = append(args, f.Status)
		argIdx++
	}
	if f.DocumentType != "" {
		conditions = append(conditions, fmt.Sprintf("p.document_type = $%d", argIdx))
		args = append(args, f.DocumentType)
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

	whereClause := strings.Join(conditions, " AND ")

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
SELECT p.id, p.tenant_id, p.document_type, p.issuing_space_id, p.destination_space_id,
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
ORDER BY p.created_at DESC
LIMIT $%d OFFSET $%d
`, whereClause, argIdx, argIdx+1)
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
			&r.ID, &r.TenantID, &r.DocumentType, &r.IssuingSpaceID, &r.DestinationSpaceID,
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
