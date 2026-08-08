// Package mentorship — service.go provides business logic for mentorship program management.
package mentorship

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// ErrProgramNotFound is returned when a mentorship program does not exist.
var ErrProgramNotFound = errors.New("mentorship program not found")

// Program represents a mentorship program.
type Program struct {
	ID          pgtype.UUID        `json:"id"`
	TenantID    pgtype.UUID        `json:"tenant_id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	IsActive    bool               `json:"is_active"`
	CreatedAt   pgtype.Timestamptz `json:"created_at"`
	UpdatedAt   pgtype.Timestamptz `json:"updated_at"`
}

// ProgramWithCounts is a mentorship program with aggregate mentor/mentee/match counts.
type ProgramWithCounts struct {
	ID          pgtype.UUID        `json:"id"`
	TenantID    pgtype.UUID        `json:"tenant_id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	IsActive    bool               `json:"is_active"`
	CreatedAt   pgtype.Timestamptz `json:"created_at"`
	UpdatedAt   pgtype.Timestamptz `json:"updated_at"`
	MentorCount int32              `json:"mentor_count"`
	MenteeCount int32              `json:"mentee_count"`
	MatchCount  int32              `json:"match_count"`
}

// Match represents a mentor-mentee match with resolved names.
type Match struct {
	ID             pgtype.UUID        `json:"id"`
	TenantID       pgtype.UUID        `json:"tenant_id"`
	ProgramID      pgtype.UUID        `json:"program_id"`
	MentorID       pgtype.UUID        `json:"mentor_id"`
	MenteeID       pgtype.UUID        `json:"mentee_id"`
	Status         string             `json:"status"`
	CreatedAt      pgtype.Timestamptz `json:"created_at"`
	UpdatedAt      pgtype.Timestamptz `json:"updated_at"`
	ProgramName    *string            `json:"program_name"`
	MentorUsername *string            `json:"mentor_username"`
	MenteeUsername *string            `json:"mentee_username"`
}

// Service coordinates mentorship program and match management.
type Service struct {
	read  *gen.Queries
	write *gen.Queries
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{
		read:  gen.New(database.Reader),
		write: gen.New(database.Writer),
	}
}

// ListPrograms returns all mentorship programs for the tenant with aggregate counts.
func (s *Service) ListPrograms(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]ProgramWithCounts, error) {
	rows, err := s.read.ListMentorshipProgramsWithCounts(ctx, gen.ListMentorshipProgramsWithCountsParams{
		TenantID:   tenantID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing mentorship programs: %w", err)
	}

	programs := make([]ProgramWithCounts, len(rows))
	for i, r := range rows {
		programs[i] = ProgramWithCounts(r)
	}

	return programs, nil
}

// GetProgram returns a single mentorship program by ID.
func (s *Service) GetProgram(ctx context.Context, tenantID, id pgtype.UUID) (Program, error) {
	row, err := s.read.GetMentorshipProgram(ctx, gen.GetMentorshipProgramParams{
		TenantID: tenantID,
		ID:       id,
	})
	if err != nil {
		return Program{}, fmt.Errorf("fetching mentorship program %s: %w", id, err)
	}

	return Program(row), nil
}

// CreateProgram creates a new mentorship program.
func (s *Service) CreateProgram(ctx context.Context, tenantID pgtype.UUID, name, description string, isActive bool) (Program, error) {
	row, err := s.write.CreateMentorshipProgram(ctx, gen.CreateMentorshipProgramParams{
		TenantID:    tenantID,
		Name:        name,
		Description: description,
		IsActive:    isActive,
	})
	if err != nil {
		return Program{}, fmt.Errorf("creating mentorship program: %w", err)
	}

	return Program(row), nil
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

	row, err := s.write.UpdateMentorshipProgram(ctx, gen.UpdateMentorshipProgramParams{
		TenantID:    tenantID,
		ID:          id,
		Name:        n,
		Description: desc,
		IsActive:    ia,
	})
	if err != nil {
		return Program{}, fmt.Errorf("updating mentorship program %s: %w", id, err)
	}

	return Program(row), nil
}

// ListMatches returns all mentor-mentee matches for the tenant with resolved names.
func (s *Service) ListMatches(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]Match, error) {
	rows, err := s.read.ListMentorshipMatches(ctx, gen.ListMentorshipMatchesParams{
		TenantID:   tenantID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing mentorship matches: %w", err)
	}

	matches := make([]Match, len(rows))
	for i, r := range rows {
		matches[i] = Match(r)
	}

	return matches, nil
}

// UpdateMatch updates a mentor-mentee match status.
func (s *Service) UpdateMatch(ctx context.Context, tenantID, id pgtype.UUID, status string) (Match, error) {
	row, err := s.write.UpdateMentorshipMatch(ctx, gen.UpdateMentorshipMatchParams{
		TenantID: tenantID,
		ID:       id,
		Status:   status,
	})
	if err != nil {
		return Match{}, fmt.Errorf("updating mentorship match %s: %w", id, err)
	}

	return Match{
		ID:        row.ID,
		TenantID:  row.TenantID,
		ProgramID: row.ProgramID,
		MentorID:  row.MentorID,
		MenteeID:  row.MenteeID,
		Status:    row.Status,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}, nil
}

// DeleteProgram deletes a mentorship program. The ON DELETE CASCADE on
// mentorship_matches.program_id will remove all associated matches.
func (s *Service) DeleteProgram(ctx context.Context, tenantID, id pgtype.UUID) error {
	rowsAffected, err := s.write.DeleteMentorshipProgram(ctx, gen.DeleteMentorshipProgramParams{
		TenantID: tenantID,
		ID:       id,
	})
	if err != nil {
		return fmt.Errorf("deleting mentorship program %s: %w", id, err)
	}

	if rowsAffected == 0 {
		return ErrProgramNotFound
	}

	return nil
}
