// Package scholarships implements the Scholarship Finder surface
// (Section 73, ADR-073). Filterable scholarship finder built on the
// `scholarship` guide category, with a per-user saved scholarships tracker
// and deadline reminder pipeline.
//
// Ref: japaconnect-engineering/sections/73-scholarship-finder.md
//      japaconnect-engineering/adr/ADR-073-scholarship-finder.md
package scholarships

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store wraps the sqlc-generated Queries for the scholarship finder.
type Store struct {
	queries *gen.Queries
	pool    *db.DB
}

// NewStore creates a scholarships store backed by the given database. The
// queries field uses database.Writer (the primary) because the store performs
// both reads and writes — admin CRUD handlers call CreateScholarship,
// UpdateScholarship, and DeleteScholarship directly through s.queries.
func NewStore(database *db.DB) *Store {
	return &Store{
		queries: gen.New(database.Writer),
		pool:    database,
	}
}

func (s *Store) SaveScholarship(ctx context.Context, arg gen.SaveScholarshipParams) (gen.SavedScholarship, error) {
	row, err := s.queries.SaveScholarship(ctx, arg)
	if err != nil {
		return gen.SavedScholarship{}, fmt.Errorf("scholarships: save: %w", err)
	}
	return row, nil
}

func (s *Store) UpdateSavedScholarship(ctx context.Context, arg gen.UpdateSavedScholarshipParams) (gen.SavedScholarship, error) {
	row, err := s.queries.UpdateSavedScholarship(ctx, arg)
	if err != nil {
		return gen.SavedScholarship{}, fmt.Errorf("scholarships: update saved: %w", err)
	}
	return row, nil
}

func (s *Store) DeleteSavedScholarship(ctx context.Context, arg gen.DeleteSavedScholarshipParams) error {
	if err := s.queries.DeleteSavedScholarship(ctx, arg); err != nil {
		return fmt.Errorf("scholarships: delete saved: %w", err)
	}
	return nil
}

func (s *Store) GetSavedScholarship(ctx context.Context, arg gen.GetSavedScholarshipParams) (gen.SavedScholarship, error) {
	row, err := s.queries.GetSavedScholarship(ctx, arg)
	if err != nil {
		return gen.SavedScholarship{}, fmt.Errorf("scholarships: get saved: %w", err)
	}
	return row, nil
}

func (s *Store) ListSavedScholarships(ctx context.Context, arg gen.ListSavedScholarshipsParams) ([]gen.ListSavedScholarshipsRow, error) {
	rows, err := s.queries.ListSavedScholarships(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("scholarships: list saved: %w", err)
	}
	return rows, nil
}

func (s *Store) CountSavedScholarships(ctx context.Context, arg gen.CountSavedScholarshipsParams) (int64, error) {
	count, err := s.queries.CountSavedScholarships(ctx, arg)
	if err != nil {
		return 0, fmt.Errorf("scholarships: count saved: %w", err)
	}
	return count, nil
}

func (s *Store) AdminStatusDistribution(ctx context.Context, tenantID pgtype.UUID) ([]gen.AdminScholarshipStatusDistributionRow, error) {
	rows, err := s.queries.AdminScholarshipStatusDistribution(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("scholarships: admin status distribution: %w", err)
	}
	return rows, nil
}

func (s *Store) AdminTopSaved(ctx context.Context, arg gen.AdminScholarshipTopSavedParams) ([]gen.AdminScholarshipTopSavedRow, error) {
	rows, err := s.queries.AdminScholarshipTopSaved(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("scholarships: admin top saved: %w", err)
	}
	return rows, nil
}

func (s *Store) AdminUpcomingDeadlines(ctx context.Context, arg gen.AdminScholarshipUpcomingDeadlinesParams) ([]gen.AdminScholarshipUpcomingDeadlinesRow, error) {
	rows, err := s.queries.AdminScholarshipUpcomingDeadlines(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("scholarships: admin upcoming deadlines: %w", err)
	}
	return rows, nil
}

func (s *Store) AdminReminderLog(ctx context.Context, arg gen.AdminScholarshipReminderLogParams) ([]gen.AdminScholarshipReminderLogRow, error) {
	rows, err := s.queries.AdminScholarshipReminderLog(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("scholarships: admin reminder log: %w", err)
	}
	return rows, nil
}

func (s *Store) FindNeedingReminders(ctx context.Context, arg gen.FindScholarshipsNeedingRemindersParams) ([]gen.FindScholarshipsNeedingRemindersRow, error) {
	rows, err := s.queries.FindScholarshipsNeedingReminders(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("scholarships: find needing reminders: %w", err)
	}
	return rows, nil
}

func (s *Store) MarkReminderSent(ctx context.Context, id pgtype.UUID) error {
	if err := s.queries.MarkScholarshipReminderSent(ctx, id); err != nil {
		return fmt.Errorf("scholarships: mark reminder sent: %w", err)
	}
	return nil
}

// --- Admin CRUD methods ---

// AdminListScholarships returns a paginated, filtered list of scholarships for
// the admin management page. q is an optional case-insensitive search over
// title/university/country; status is an optional verification_status filter.
// The tenantID is stamped into the query params.
func (s *Store) AdminListScholarships(ctx context.Context, tenantID pgtype.UUID, arg gen.AdminListScholarshipsParams) ([]gen.Scholarship, error) {
	arg.TenantID = tenantID
	rows, err := s.queries.AdminListScholarships(ctx, arg)
	if err != nil {
		return nil, fmt.Errorf("scholarships: admin list: %w", err)
	}
	return rows, nil
}

// AdminCountScholarships returns the total row count for the admin management
// list with the same q + status filter as AdminListScholarships, so the admin
// panel can compute total pages.
func (s *Store) AdminCountScholarships(ctx context.Context, tenantID pgtype.UUID, arg gen.AdminCountScholarshipsParams) (int64, error) {
	arg.TenantID = tenantID
	count, err := s.queries.AdminCountScholarships(ctx, arg)
	if err != nil {
		return 0, fmt.Errorf("scholarships: admin count: %w", err)
	}
	return count, nil
}

// AdminGetScholarship returns a single scholarship by ID for the admin
// management page.
func (s *Store) AdminGetScholarship(ctx context.Context, tenantID, id pgtype.UUID) (gen.Scholarship, error) {
	row, err := s.queries.GetScholarship(ctx, gen.GetScholarshipParams{
		TenantID: tenantID,
		ID:       id,
	})
	if err != nil {
		return gen.Scholarship{}, fmt.Errorf("scholarships: admin get: %w", err)
	}
	return row, nil
}

// AdminCreateScholarship creates a new scholarship record. The tenantID is
// stamped into the query params.
func (s *Store) AdminCreateScholarship(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateScholarshipParams) (gen.Scholarship, error) {
	arg.TenantID = tenantID
	row, err := s.queries.CreateScholarship(ctx, arg)
	if err != nil {
		return gen.Scholarship{}, fmt.Errorf("scholarships: admin create: %w", err)
	}
	return row, nil
}

// AdminUpdateScholarship updates an existing scholarship record. The
// tenantID is stamped into the query params.
func (s *Store) AdminUpdateScholarship(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateScholarshipParams) error {
	arg.TenantID = tenantID
	if err := s.queries.UpdateScholarship(ctx, arg); err != nil {
		return fmt.Errorf("scholarships: admin update: %w", err)
	}
	return nil
}

// AdminDeleteScholarship deletes a scholarship record and soft-deletes the
// guide content row it referenced (Rule 11 — user content is soft-deleted, never
// hard-deleted). The tenantID is stamped into the query params for tenant
// isolation. The guide content row is soft-deleted so it stops appearing in the
// finder and content listings without losing audit history.
func (s *Store) AdminDeleteScholarship(ctx context.Context, tenantID, id pgtype.UUID) error {
	sch, err := s.queries.GetScholarship(ctx, gen.GetScholarshipParams{
		TenantID: tenantID,
		ID:       id,
	})
	if err != nil {
		return fmt.Errorf("scholarships: admin delete (get): %w", err)
	}
	if sch.GuideID.Valid {
		if err := s.queries.DeleteContent(ctx, gen.DeleteContentParams{
			ID:       sch.GuideID,
			TenantID: tenantID,
		}); err != nil {
			return fmt.Errorf("scholarships: admin delete (soft-delete guide): %w", err)
		}
	}
	if err := s.queries.DeleteScholarship(ctx, gen.DeleteScholarshipParams{
		TenantID: tenantID,
		ID:       id,
	}); err != nil {
		return fmt.Errorf("scholarships: admin delete: %w", err)
	}
	return nil
}
