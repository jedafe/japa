// Package cvformatter implements the International CV / Resume Formatter
// (§95, ADR-103).
//
// The package manages two tables created by migration 000333:
// cv_standards (per-destination-space formatting guidance) and
// cv_format_sessions (individual CV formatting requests submitted by
// users). Standards are public; sessions are owner-scoped. The actual
// AI formatting is performed by an Asynq task — this package creates
// the session row, returns 202 Accepted, and exposes a poll/download
// endpoint.
//
// Ref: japaconnect-engineering/sections/95-cv-formatter.md
//
//	japaconnect-engineering/adr/ADR-103-cv-formatter.md
package cvformatter

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
)

// Photo policies (§95).
const (
	PhotoPolicyRequired       = "required"
	PhotoPolicyOptional       = "optional"
	PhotoPolicyNotRecommended = "not_recommended"
	PhotoPolicyProhibited     = "prohibited"
)

// Session statuses (§95).
const (
	SessionStatusPending  = "pending"
	SessionStatusComplete = "complete"
	SessionStatusFailed   = "failed"
)

// Output formats (§95).
const (
	OutputFormatDOCX = "docx"
	OutputFormatPDF  = "pdf"
)

// Store is the data-access layer for cv_standards and cv_format_sessions.
// It uses direct pgx queries because these tables do not have
// sqlc-generated queries yet. No business logic here.
type Store struct {
	pool *db.DB
}

// NewStore creates a CV formatter store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{pool: database}
}

// StandardRow holds a single cv_standards row joined with the destination
// space name and geo_code.
type StandardRow struct {
	ID                     pgtype.UUID
	DestinationSpaceID     pgtype.UUID
	DestinationSpaceName   *string
	DestinationSpaceCode   *string
	RecommendedLengthPages *string
	PhotoPolicy            *string
	DateFormat             *string
	RecommendedSections    []string
	ToneGuidance           *string
	CommonPitfalls         *string
	CreatedAt              pgtype.Timestamptz
	UpdatedAt              pgtype.Timestamptz
}

// SessionRow holds a single cv_format_sessions row.
type SessionRow struct {
	ID                 pgtype.UUID
	TenantID           pgtype.UUID
	UserID             pgtype.UUID
	OriginSpaceID      pgtype.UUID
	DestinationSpaceID pgtype.UUID
	InputText          string
	OutputText         *string
	OutputFormat       string
	Status             string
	AIModel            *string
	TokensUsed         *int32
	CreatedAt          pgtype.Timestamptz
	ExpiresAt          pgtype.Timestamptz
	// ADR-088 provenance columns (migration 000473).
	AIGenerated       bool
	AIPromptHash      *string
	HITLReviewedBy    pgtype.UUID
	HITLReviewedAt    pgtype.Timestamptz
	HITLReviewNotes   *string
	// File upload metadata (migration 000487).
	OriginalFilename *string
	FileStoragePath  *string
	FileMimeType     *string
	FileSizeBytes    *int64
}

// ListStandards returns all cv_standards rows joined with destination
// space info, ordered by destination space name.
func (s *Store) ListStandards(ctx context.Context) ([]StandardRow, error) {
	const q = `
SELECT cs.id, cs.destination_space_id, sp.name AS dest_space_name,
       sp.geo_code AS dest_space_code, cs.recommended_length_pages,
       cs.photo_policy, cs.date_format, cs.recommended_sections,
       cs.tone_guidance, cs.common_pitfalls, cs.created_at, cs.updated_at
FROM cv_standards cs
JOIN spaces sp ON sp.id = cs.destination_space_id
ORDER BY sp.name
`
	rows, err := s.pool.Reader.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("cvformatter: list standards: %w", err)
	}
	defer rows.Close()

	var out []StandardRow
	for rows.Next() {
		var r StandardRow
		if err := rows.Scan(
			&r.ID, &r.DestinationSpaceID, &r.DestinationSpaceName,
			&r.DestinationSpaceCode, &r.RecommendedLengthPages,
			&r.PhotoPolicy, &r.DateFormat, &r.RecommendedSections,
			&r.ToneGuidance, &r.CommonPitfalls, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("cvformatter: scan standard row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cvformatter: iterate standard rows: %w", err)
	}
	return out, nil
}

// GetStandardByDest returns the cv_standards row for the given
// destination_space_id, joined with destination space info.
func (s *Store) GetStandardByDest(ctx context.Context, destSpaceID pgtype.UUID) (StandardRow, error) {
	const q = `
SELECT cs.id, cs.destination_space_id, sp.name AS dest_space_name,
       sp.geo_code AS dest_space_code, cs.recommended_length_pages,
       cs.photo_policy, cs.date_format, cs.recommended_sections,
       cs.tone_guidance, cs.common_pitfalls, cs.created_at, cs.updated_at
FROM cv_standards cs
JOIN spaces sp ON sp.id = cs.destination_space_id
WHERE cs.destination_space_id = $1
`
	var r StandardRow
	err := s.pool.Reader.QueryRow(ctx, q, destSpaceID).Scan(
		&r.ID, &r.DestinationSpaceID, &r.DestinationSpaceName,
		&r.DestinationSpaceCode, &r.RecommendedLengthPages,
		&r.PhotoPolicy, &r.DateFormat, &r.RecommendedSections,
		&r.ToneGuidance, &r.CommonPitfalls, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return StandardRow{}, pgx.ErrNoRows
		}
		return StandardRow{}, fmt.Errorf("cvformatter: get standard by dest: %w", err)
	}
	return r, nil
}

// StandardUpsert holds the mutable fields for creating or updating a
// cv_standards row.
type StandardUpsert struct {
	DestinationSpaceID     pgtype.UUID
	RecommendedLengthPages *string
	PhotoPolicy            *string
	DateFormat             *string
	RecommendedSections    []string
	ToneGuidance           *string
	CommonPitfalls         *string
}

// CreateStandard inserts a new cv_standards row. Returns
// pgx.ErrNoRows-equivalent unique-violation if the destination already
// has a standard — callers should use UpdateStandard in that case.
func (s *Store) CreateStandard(ctx context.Context, input StandardUpsert) (StandardRow, error) {
	const q = `
INSERT INTO cv_standards (
    destination_space_id, recommended_length_pages, photo_policy,
    date_format, recommended_sections, tone_guidance, common_pitfalls
) VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, destination_space_id, recommended_length_pages, photo_policy,
          date_format, recommended_sections, tone_guidance, common_pitfalls,
          created_at, updated_at
`
	var r StandardRow
	err := s.pool.Writer.QueryRow(ctx, q,
		input.DestinationSpaceID, input.RecommendedLengthPages,
		input.PhotoPolicy, input.DateFormat, input.RecommendedSections,
		input.ToneGuidance, input.CommonPitfalls,
	).Scan(
		&r.ID, &r.DestinationSpaceID, &r.RecommendedLengthPages,
		&r.PhotoPolicy, &r.DateFormat, &r.RecommendedSections,
		&r.ToneGuidance, &r.CommonPitfalls, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return StandardRow{}, fmt.Errorf("cvformatter: create standard: %w", err)
	}
	return r, nil
}

// UpdateStandard updates the mutable fields on a cv_standards row
// identified by destination_space_id.
func (s *Store) UpdateStandard(ctx context.Context, input StandardUpsert) error {
	const q = `
UPDATE cv_standards
SET recommended_length_pages = $2,
    photo_policy = $3,
    date_format = $4,
    recommended_sections = $5,
    tone_guidance = $6,
    common_pitfalls = $7,
    updated_at = now()
WHERE destination_space_id = $1
`
	cmd, err := s.pool.Writer.Exec(ctx, q,
		input.DestinationSpaceID, input.RecommendedLengthPages,
		input.PhotoPolicy, input.DateFormat, input.RecommendedSections,
		input.ToneGuidance, input.CommonPitfalls,
	)
	if err != nil {
		return fmt.Errorf("cvformatter: update standard: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// SessionInput holds the fields for creating a new cv_format_sessions row.
type SessionInput struct {
	TenantID           pgtype.UUID
	UserID             pgtype.UUID
	OriginSpaceID      pgtype.UUID
	DestinationSpaceID pgtype.UUID
	InputText          string
	OutputFormat       string
	AIModel            *string
	AIPromptHash       *string // SHA-256 of input_text (ADR-088)
	// File upload metadata (migration 000487). Populated when the user
	// uploads a PDF/DOCX file instead of pasting text. The extracted text
	// is stored in InputText; these columns preserve the original file.
	OriginalFilename *string
	FileStoragePath  *string
	FileMimeType     *string
	FileSizeBytes    *int64
}

// CreateSession inserts a new cv_format_sessions row with status='pending'
// and returns the full row. The expires_at default (now()+24h) is applied
// by the database.
func (s *Store) CreateSession(ctx context.Context, input SessionInput) (SessionRow, error) {
	outputFormat := input.OutputFormat
	if outputFormat == "" {
		outputFormat = OutputFormatDOCX
	}
	const q = `
INSERT INTO cv_format_sessions (
    tenant_id, user_id, origin_space_id, destination_space_id,
    input_text, output_format, status, ai_model,
    ai_generated, ai_prompt_hash,
    original_filename, file_storage_path, file_mime_type, file_size_bytes
) VALUES ($1, $2, $3, $4, $5, $6, 'pending', $7, true, $8, $9, $10, $11, $12)
RETURNING id, tenant_id, user_id, origin_space_id, destination_space_id,
          input_text, output_text, output_format, status, ai_model,
          tokens_used, created_at, expires_at,
          ai_generated, ai_prompt_hash, hitl_reviewed_by,
          hitl_reviewed_at, hitl_review_notes,
          original_filename, file_storage_path, file_mime_type, file_size_bytes`
	var r SessionRow
	err := s.pool.Writer.QueryRow(ctx, q,
		input.TenantID, input.UserID, input.OriginSpaceID,
		input.DestinationSpaceID, input.InputText, outputFormat,
		input.AIModel, input.AIPromptHash,
		input.OriginalFilename, input.FileStoragePath,
		input.FileMimeType, input.FileSizeBytes,
	).Scan(
		&r.ID, &r.TenantID, &r.UserID, &r.OriginSpaceID,
		&r.DestinationSpaceID, &r.InputText, &r.OutputText,
		&r.OutputFormat, &r.Status, &r.AIModel, &r.TokensUsed,
		&r.CreatedAt, &r.ExpiresAt,
		&r.AIGenerated, &r.AIPromptHash, &r.HITLReviewedBy,
		&r.HITLReviewedAt, &r.HITLReviewNotes,
		&r.OriginalFilename, &r.FileStoragePath,
		&r.FileMimeType, &r.FileSizeBytes,
	)
	if err != nil {
		return SessionRow{}, fmt.Errorf("cvformatter: create session: %w", err)
	}
	return r, nil
}

// GetSessionByID returns a single cv_format_sessions row by its id.
func (s *Store) GetSessionByID(ctx context.Context, id pgtype.UUID) (SessionRow, error) {
	const q = `
SELECT id, tenant_id, user_id, origin_space_id, destination_space_id,
       input_text, output_text, output_format, status, ai_model,
       tokens_used, created_at, expires_at,
       ai_generated, ai_prompt_hash, hitl_reviewed_by,
       hitl_reviewed_at, hitl_review_notes,
       original_filename, file_storage_path, file_mime_type, file_size_bytes
FROM cv_format_sessions
WHERE id = $1
`
	var r SessionRow
	err := s.pool.Reader.QueryRow(ctx, q, id).Scan(
		&r.ID, &r.TenantID, &r.UserID, &r.OriginSpaceID,
		&r.DestinationSpaceID, &r.InputText, &r.OutputText,
		&r.OutputFormat, &r.Status, &r.AIModel, &r.TokensUsed,
		&r.CreatedAt, &r.ExpiresAt,
		&r.AIGenerated, &r.AIPromptHash, &r.HITLReviewedBy,
		&r.HITLReviewedAt, &r.HITLReviewNotes,
		&r.OriginalFilename, &r.FileStoragePath,
		&r.FileMimeType, &r.FileSizeBytes,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return SessionRow{}, pgx.ErrNoRows
		}
		return SessionRow{}, fmt.Errorf("cvformatter: get session by id: %w", err)
	}
	return r, nil
}

// UpdateSessionStatus updates a session's status, output_text, tokens_used,
// and ai_model. Called by ProcessSession after the AI Gateway returns.
// outputText, tokensUsed, and model may be nil.
func (s *Store) UpdateSessionStatus(ctx context.Context, id pgtype.UUID, status string, outputText *string, tokensUsed *int32, model *string) error {
	const q = `
UPDATE cv_format_sessions
SET status = $2,
    output_text = $3,
    tokens_used = $4,
    ai_model = $5
WHERE id = $1
`
	cmd, err := s.pool.Writer.Exec(ctx, q, id, status, outputText, tokensUsed, model)
	if err != nil {
		return fmt.Errorf("cvformatter: update session status: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// DeleteExpiredSessions deletes cv_format_sessions rows past their
// expires_at. Called by the cv_format_sessions:purge nightly Asynq task
// (§95, ADR-103). Returns the number of deleted rows.
func (s *Store) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	const q = `DELETE FROM cv_format_sessions WHERE expires_at < now()`
	cmd, err := s.pool.Writer.Exec(ctx, q)
	if err != nil {
		return 0, fmt.Errorf("cvformatter: delete expired sessions: %w", err)
	}
	return cmd.RowsAffected(), nil
}

// CountUserSessionsThisMonth returns the number of cv_format_sessions
// rows created by the given user in the current calendar month. Used by
// the service for the standard-member quota check (§95).
func (s *Store) CountUserSessionsThisMonth(ctx context.Context, userID pgtype.UUID) (int64, error) {
	const q = `
SELECT COUNT(*)
FROM cv_format_sessions
WHERE user_id = $1
  AND date_trunc('month', created_at) = date_trunc('month', now())
`
	var count int64
	err := s.pool.Reader.QueryRow(ctx, q, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("cvformatter: count user sessions this month: %w", err)
	}
	return count, nil
}
