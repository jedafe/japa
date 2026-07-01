// Package mentorship — service.go provides business logic for mentorship program management.
package mentorship

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
)

// Program represents a mentorship program.
type Program struct {
	ID          pgtype.UUID      `json:"id"`
	TenantID    pgtype.UUID      `json:"tenant_id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	IsActive    bool             `json:"is_active"`
	CreatedAt   pgtype.Timestamptz `json:"created_at"`
	UpdatedAt   pgtype.Timestamptz `json:"updated_at"`
}

// Match represents a mentor-mentee match.
type Match struct {
	ID        pgtype.UUID      `json:"id"`
	TenantID  pgtype.UUID      `json:"tenant_id"`
	ProgramID pgtype.UUID      `json:"program_id"`
	MentorID  pgtype.UUID      `json:"mentor_id"`
	MenteeID  pgtype.UUID      `json:"mentee_id"`
	Status    string           `json:"status"`
	CreatedAt pgtype.Timestamptz `json:"created_at"`
	UpdatedAt pgtype.Timestamptz `json:"updated_at"`
}

// Service coordinates mentorship program and match management.
type Service struct {
	db *db.DB
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{db: database}
}

// ListPrograms returns all mentorship programs for the tenant.
func (s *Service) ListPrograms(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]Program, error) {
	rows, err := s.db.Reader.Query(ctx, `
		SELECT id, tenant_id, name, description, is_active, created_at, updated_at
		FROM mentorship_programs
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing mentorship programs: %w", err)
	}
	defer rows.Close()

	var programs []Program
	for rows.Next() {
		var p Program
		if err := rows.Scan(&p.ID, &p.TenantID, &p.Name, &p.Description, &p.IsActive, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning mentorship program: %w", err)
		}
		programs = append(programs, p)
	}
	return programs, rows.Err()
}

// GetProgram returns a single mentorship program by ID.
func (s *Service) GetProgram(ctx context.Context, tenantID, id pgtype.UUID) (Program, error) {
	var p Program
	err := s.db.Reader.QueryRow(ctx, `
		SELECT id, tenant_id, name, description, is_active, created_at, updated_at
		FROM mentorship_programs
		WHERE tenant_id = $1 AND id = $2
	`, tenantID, id).Scan(&p.ID, &p.TenantID, &p.Name, &p.Description, &p.IsActive, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return Program{}, fmt.Errorf("fetching mentorship program %s: %w", id, err)
	}
	return p, nil
}

// CreateProgram creates a new mentorship program.
func (s *Service) CreateProgram(ctx context.Context, tenantID pgtype.UUID, name, description string) (Program, error) {
	var p Program
	err := s.db.Writer.QueryRow(ctx, `
		INSERT INTO mentorship_programs (tenant_id, name, description, is_active)
		VALUES ($1, $2, $3, true)
		RETURNING id, tenant_id, name, description, is_active, created_at, updated_at
	`, tenantID, name, description).Scan(&p.ID, &p.TenantID, &p.Name, &p.Description, &p.IsActive, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return Program{}, fmt.Errorf("creating mentorship program: %w", err)
	}
	return p, nil
}

// UpdateProgram updates an existing mentorship program.
func (s *Service) UpdateProgram(ctx context.Context, tenantID, id pgtype.UUID, name, description *string, isActive *bool) (Program, error) {
	// Fetch current values for merge.
	current, err := s.GetProgram(ctx, tenantID, id)
	if err != nil {
		return Program{}, err
	}

	n := current.Name
	if name != nil {
		n = *name
	}
	desc := current.Description
	if description != nil {
		desc = *description
	}
	ia := current.IsActive
	if isActive != nil {
		ia = *isActive
	}

	var updated Program
	err = s.db.Writer.QueryRow(ctx, `
		UPDATE mentorship_programs
		SET name = $3, description = $4, is_active = $5, updated_at = now()
		WHERE tenant_id = $1 AND id = $2
		RETURNING id, tenant_id, name, description, is_active, created_at, updated_at
	`, tenantID, id, n, desc, ia).Scan(&updated.ID, &updated.TenantID, &updated.Name, &updated.Description, &updated.IsActive, &updated.CreatedAt, &updated.UpdatedAt)
	if err != nil {
		return Program{}, fmt.Errorf("updating mentorship program %s: %w", id, err)
	}
	return updated, nil
}

// ListMatches returns all mentor-mentee matches for the tenant.
func (s *Service) ListMatches(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]Match, error) {
	rows, err := s.db.Reader.Query(ctx, `
		SELECT id, tenant_id, program_id, mentor_id, mentee_id, status, created_at, updated_at
		FROM mentorship_matches
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing mentorship matches: %w", err)
	}
	defer rows.Close()

	var matches []Match
	for rows.Next() {
		var m Match
		if err := rows.Scan(&m.ID, &m.TenantID, &m.ProgramID, &m.MentorID, &m.MenteeID, &m.Status, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning mentorship match: %w", err)
		}
		matches = append(matches, m)
	}
	return matches, rows.Err()
}

// UpdateMatch updates a mentor-mentee match status.
func (s *Service) UpdateMatch(ctx context.Context, tenantID, id pgtype.UUID, status string) (Match, error) {
	var m Match
	err := s.db.Writer.QueryRow(ctx, `
		UPDATE mentorship_matches
		SET status = $3, updated_at = now()
		WHERE tenant_id = $1 AND id = $2
		RETURNING id, tenant_id, program_id, mentor_id, mentee_id, status, created_at, updated_at
	`, tenantID, id, status).Scan(&m.ID, &m.TenantID, &m.ProgramID, &m.MentorID, &m.MenteeID, &m.Status, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return Match{}, fmt.Errorf("updating mentorship match %s: %w", id, err)
	}
	return m, nil
}
