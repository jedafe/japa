package rbac

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
)

// Store provides CRUD operations for RBAC entities.
type Store struct {
	db *db.DB
}

// NewStore creates a Store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{db: database}
}

// CreateGroup creates a new user group. System groups cannot be created
// via this method - they are seeded at install time only.
func (s *Store) CreateGroup(ctx context.Context, tenantID pgtype.UUID, name, slug string) (pgtype.UUID, error) {
	if IsSystemGroup(slug) {
		return pgtype.UUID{}, fmt.Errorf("cannot create system group %s via API", slug)
	}
	var id pgtype.UUID
	err := s.db.Writer.QueryRow(ctx, `
		INSERT INTO user_groups (tenant_id, name, slug)
		VALUES ($1, $2, $3)
		RETURNING id
	`, tenantID, name, slug).Scan(&id)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("creating group: %w", err)
	}
	return id, nil
}

// DeleteGroup deletes a user group. System groups cannot be deleted.
func (s *Store) DeleteGroup(ctx context.Context, tenantID pgtype.UUID, slug string) error {
	if IsSystemGroup(slug) {
		return fmt.Errorf("cannot delete system group %s", slug)
	}
	_, err := s.db.Writer.Exec(ctx, `
		DELETE FROM user_groups WHERE tenant_id = $1 AND slug = $2 AND is_system = false
	`, tenantID, slug)
	if err != nil {
		return fmt.Errorf("deleting group: %w", err)
	}
	return nil
}

// AddUserToGroup adds a user to a group.
func (s *Store) AddUserToGroup(ctx context.Context, tenantID, userID, groupID pgtype.UUID) error {
	_, err := s.db.Writer.Exec(ctx, `
		INSERT INTO user_group_memberships (tenant_id, user_id, group_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (tenant_id, user_id, group_id) DO NOTHING
	`, tenantID, userID, groupID)
	if err != nil {
		return fmt.Errorf("adding user to group: %w", err)
	}
	return nil
}

// RemoveUserFromGroup removes a user from a group.
func (s *Store) RemoveUserFromGroup(ctx context.Context, tenantID, userID, groupID pgtype.UUID) error {
	_, err := s.db.Writer.Exec(ctx, `
		DELETE FROM user_group_memberships
		WHERE tenant_id = $1 AND user_id = $2 AND group_id = $3
	`, tenantID, userID, groupID)
	if err != nil {
		return fmt.Errorf("removing user from group: %w", err)
	}
	return nil
}

// RegisterPermission creates a permission node if it doesn't exist.
// Called at application startup for all known permission keys.
func (s *Store) RegisterPermission(ctx context.Context, key, description string) error {
	_, err := s.db.Writer.Exec(ctx, `
		INSERT INTO permissions (key, description)
		VALUES ($1, $2)
		ON CONFLICT (key) DO NOTHING
	`, key, description)
	if err != nil {
		return fmt.Errorf("registering permission %s: %w", key, err)
	}
	return nil
}

// AssignPermissionToRole grants a permission to a role.
func (s *Store) AssignPermissionToRole(ctx context.Context, roleID pgtype.UUID, permKey string) error {
	_, err := s.db.Writer.Exec(ctx, `
		INSERT INTO role_permissions (role_id, permission_id)
		SELECT $1, id FROM permissions WHERE key = $2
		ON CONFLICT DO NOTHING
	`, roleID, permKey)
	if err != nil {
		return fmt.Errorf("assigning permission %s to role: %w", permKey, err)
	}
	return nil
}

// AssignRoleToGroup grants a role to a group.
func (s *Store) AssignRoleToGroup(ctx context.Context, groupID, roleID pgtype.UUID) error {
	_, err := s.db.Writer.Exec(ctx, `
		INSERT INTO group_roles (group_id, role_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, groupID, roleID)
	if err != nil {
		return fmt.Errorf("assigning role to group: %w", err)
	}
	return nil
}
