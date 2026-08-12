// Package control — HTTP handlers for the super-admin Control Panel
// (Section 21). Distinct from internal/admin (tenant-scoped admin panel):
// every route here is gated by permission keys that only the super_admin
// role receives (bootstrapPermissions grants every permission to
// super_admin, so a control.* key is never reachable by a regular admin
// unless explicitly granted).
package control

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

const (
	TenantStatusActive    = "active"
	TenantStatusSuspended = "suspended"
	TenantStatusDeleted   = "deleted"

	// DefaultTenantID is the UUID of the seeded default tenant
	// (00000000-0000-0000-0000-000000000001). It must never be
	// soft-deleted or suspended — deleting it would orphan every
	// test-seed reference and break the platform.
	DefaultTenantID = "00000000-0000-0000-0000-000000000001"
)

// Service provides control-panel data access.
type Service struct {
	read  *gen.Queries
	write *gen.Queries
}

// NewService creates a Service backed by the given database.
func NewService(database *db.DB) *Service {
	return &Service{read: gen.New(database.Reader), write: gen.New(database.Writer)}
}

// ListTenants returns a paginated list of tenants, newest first.
// statusFilter is empty for all statuses, or one of active/suspended/deleted.
func (s *Service) ListTenants(ctx context.Context, statusFilter string, limit, offset int32) ([]gen.ListTenantsRow, error) {
	tenants, err := s.read.ListTenants(ctx, gen.ListTenantsParams{
		StatusFilter: statusFilter,
		PageLimit:    limit,
		PageOffset:   offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing tenants: %w", err)
	}

	return tenants, nil
}

// CountTenants returns the total number of tenants matching the optional
// status filter, for pagination metadata.
func (s *Service) CountTenants(ctx context.Context, statusFilter string) (int32, error) {
	count, err := s.read.CountTenants(ctx, statusFilter)
	if err != nil {
		return 0, fmt.Errorf("counting tenants: %w", err)
	}

	return count, nil
}

// GetTenant returns a single tenant by ID.
func (s *Service) GetTenant(ctx context.Context, id pgtype.UUID) (gen.GetTenantByIDRow, error) {
	t, err := s.read.GetTenantByID(ctx, id)
	if err != nil {
		return gen.GetTenantByIDRow{}, fmt.Errorf("getting tenant: %w", err)
	}

	return t, nil
}

// CreateTenantInput holds parameters for creating a tenant.
type CreateTenantInput struct {
	Name   string
	Slug   string
	Domain *string
}

// CreateTenant creates a new tenant.
func (s *Service) CreateTenant(ctx context.Context, input CreateTenantInput) (gen.CreateTenantRow, error) {
	t, err := s.write.CreateTenant(ctx, gen.CreateTenantParams{
		Name:   input.Name,
		Slug:   input.Slug,
		Domain: input.Domain,
	})
	if err != nil {
		return gen.CreateTenantRow{}, fmt.Errorf("creating tenant: %w", err)
	}

	return t, nil
}

// UpdateTenantInput holds parameters for updating a tenant. Slug is immutable.
type UpdateTenantInput struct {
	Name   string
	Domain *string
}

// UpdateTenant updates a tenant's name/domain.
func (s *Service) UpdateTenant(ctx context.Context, id pgtype.UUID, input UpdateTenantInput) (gen.UpdateTenantRow, error) {
	t, err := s.write.UpdateTenant(ctx, gen.UpdateTenantParams{
		ID:     id,
		Name:   input.Name,
		Domain: input.Domain,
	})
	if err != nil {
		return gen.UpdateTenantRow{}, fmt.Errorf("updating tenant: %w", err)
	}

	return t, nil
}

// DeleteTenant soft-deletes a tenant (sets status='deleted', deleted_at=now()).
// The default tenant (00000000-0000-0000-0000-000000000001) cannot be deleted.
// Per Section 21, a 30-day grace period applies before the tenant:cleanup_data
// Asynq task hard-deletes all associated data. The task is enqueued by the
// HTTP handler (Handlers.DeleteTenant) after this service call succeeds,
// using a 30-day ProcessAt delay so the cleanup runs after the grace period.
func (s *Service) DeleteTenant(ctx context.Context, id pgtype.UUID) (gen.DeleteTenantRow, error) {
	if id.String() == DefaultTenantID {
		return gen.DeleteTenantRow{}, fmt.Errorf("deleting tenant: cannot delete the default tenant")
	}
	t, err := s.write.DeleteTenant(ctx, id)
	if err != nil {
		return gen.DeleteTenantRow{}, fmt.Errorf("deleting tenant: %w", err)
	}

	return t, nil
}

// SuspendTenant sets a tenant's status to suspended. The default tenant
// (00000000-0000-0000-0000-000000000001) cannot be suspended — suspending it
// would take down the entire platform.
func (s *Service) SuspendTenant(ctx context.Context, id pgtype.UUID) (gen.SetTenantStatusRow, error) {
	if id.String() == DefaultTenantID {
		return gen.SetTenantStatusRow{}, fmt.Errorf("suspending tenant: cannot suspend the default tenant")
	}
	return s.setStatus(ctx, id, TenantStatusSuspended)
}

// UnsuspendTenant sets a tenant's status back to active.
func (s *Service) UnsuspendTenant(ctx context.Context, id pgtype.UUID) (gen.SetTenantStatusRow, error) {
	return s.setStatus(ctx, id, TenantStatusActive)
}

func (s *Service) setStatus(ctx context.Context, id pgtype.UUID, status string) (gen.SetTenantStatusRow, error) {
	t, err := s.write.SetTenantStatus(ctx, gen.SetTenantStatusParams{ID: id, Status: status})
	if err != nil {
		return gen.SetTenantStatusRow{}, fmt.Errorf("setting tenant status: %w", err)
	}

	return t, nil
}
