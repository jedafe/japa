package rbac

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
)

// Store provides CRUD operations for RBAC entities.
// After any mutation that affects permission resolution, the Store calls
// InvalidateUserCache on affected users so the L1/L2 caches stay consistent.
type Store struct {
	db   *db.DB
	rbac *RBAC // nil-safe: if nil, cache invalidation is skipped
}

// NewStore creates a Store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{db: database}
}

// SetRBAC wires the RBAC resolver into the store so mutation methods can
// trigger cache invalidation. Must be called before any mutation methods
// are invoked.
func (s *Store) SetRBAC(resolver *RBAC) {
	s.rbac = resolver
}

// invalidateUserCache is a nil-safe helper that triggers L1+L2 cache
// eviction for a single user. Failures are logged, never propagated.
func (s *Store) invalidateUserCache(ctx context.Context, userID pgtype.UUID) {
	if s.rbac == nil {
		return
	}

	s.rbac.InvalidateUserCache(ctx, userID)
}

// invalidateGroupCache evicts cache for every member of a group.
// Called when a role is assigned/revoked from a group (affects all members).
func (s *Store) invalidateGroupCache(ctx context.Context, groupID pgtype.UUID) {
	if s.rbac == nil {
		return
	}

	rows, err := s.db.Reader.Query(ctx, `
		SELECT user_id FROM user_group_memberships WHERE group_id = $1
	`, groupID)
	if err != nil {
		slog.Warn("rbac: failed to list group members for cache invalidation", "group_id", groupID, "error", err)
		return
	}

	defer rows.Close()

	for rows.Next() {
		var userID pgtype.UUID
		if err := rows.Scan(&userID); err != nil {
			slog.Warn("rbac: failed to scan user_id for group cache invalidation", "group_id", groupID, "error", err)
			continue
		}

		s.invalidateUserCache(ctx, userID)
	}

	if err := rows.Err(); err != nil {
		slog.Warn("rbac: error iterating group members for cache invalidation", "group_id", groupID, "error", err)
	}
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

// AddUserToGroup adds a user to a group and invalidates the user's cache.
func (s *Store) AddUserToGroup(ctx context.Context, tenantID, userID, groupID pgtype.UUID) error {
	_, err := s.db.Writer.Exec(ctx, `
		INSERT INTO user_group_memberships (tenant_id, user_id, group_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (tenant_id, user_id, group_id) DO NOTHING
	`, tenantID, userID, groupID)
	if err != nil {
		return fmt.Errorf("adding user to group: %w", err)
	}

	s.invalidateUserCache(ctx, userID)

	return nil
}

// RemoveUserFromGroup removes a user from a group and invalidates the user's cache.
func (s *Store) RemoveUserFromGroup(ctx context.Context, tenantID, userID, groupID pgtype.UUID) error {
	_, err := s.db.Writer.Exec(ctx, `
		DELETE FROM user_group_memberships
		WHERE tenant_id = $1 AND user_id = $2 AND group_id = $3
	`, tenantID, userID, groupID)
	if err != nil {
		return fmt.Errorf("removing user from group: %w", err)
	}

	s.invalidateUserCache(ctx, userID)

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

// AssignPermissionToRole grants a permission to a role and invalidates cache
// for every user in groups that hold this role.
func (s *Store) AssignPermissionToRole(ctx context.Context, roleID pgtype.UUID, permKey string) error {
	_, err := s.db.Writer.Exec(ctx, `
		INSERT INTO role_permissions (role_id, permission_id)
		SELECT $1, id FROM permissions WHERE key = $2
		ON CONFLICT DO NOTHING
	`, roleID, permKey)
	if err != nil {
		return fmt.Errorf("assigning permission %s to role: %w", permKey, err)
	}

	// Find all groups that hold this role and invalidate their members' caches.
	rows, err := s.db.Reader.Query(ctx, `
		SELECT group_id FROM group_roles WHERE role_id = $1
	`, roleID)
	if err != nil {
		slog.Warn("rbac: failed to list groups for role cache invalidation", "role_id", roleID, "error", err)
		return nil // mutation succeeded — don't fail on cache invalidation
	}

	defer rows.Close()

	for rows.Next() {
		var groupID pgtype.UUID
		if err := rows.Scan(&groupID); err != nil {
			continue
		}

		s.invalidateGroupCache(ctx, groupID)
	}

	return nil
}

// AssignRoleToGroup grants a role to a group and invalidates cache for every
// member of that group (their permissions just changed).
func (s *Store) AssignRoleToGroup(ctx context.Context, groupID, roleID pgtype.UUID) error {
	_, err := s.db.Writer.Exec(ctx, `
		INSERT INTO group_roles (group_id, role_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, groupID, roleID)
	if err != nil {
		return fmt.Errorf("assigning role to group: %w", err)
	}

	s.invalidateGroupCache(ctx, groupID)

	return nil
}

// ---------------------------------------------------------------------------
// List / query methods for admin RBAC management
// ---------------------------------------------------------------------------

// ListGroups returns all user groups for a tenant, ordered by name.
func (s *Store) ListGroups(ctx context.Context, tenantID pgtype.UUID) ([]Group, error) {
	rows, err := s.db.Reader.Query(ctx, `
		SELECT id, tenant_id, name, slug, is_system
		FROM user_groups
		WHERE tenant_id = $1
		ORDER BY name
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing groups: %w", err)
	}
	defer rows.Close()

	var groups []Group

	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.TenantID, &g.Name, &g.Slug, &g.IsSystem); err != nil {
			return nil, fmt.Errorf("scanning group: %w", err)
		}

		groups = append(groups, g)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating groups: %w", err)
	}

	return groups, nil
}

// GroupMember represents a user within a group.
type GroupMember struct {
	ID          pgtype.UUID `json:"id"`
	Username    string      `json:"username"`
	DisplayName string      `json:"display_name"`
}

// ListGroupMembers returns all members of a group.
func (s *Store) ListGroupMembers(ctx context.Context, groupID, tenantID pgtype.UUID) ([]GroupMember, error) {
	rows, err := s.db.Reader.Query(ctx, `
		SELECT u.id, u.username, COALESCE(u.display_name, '')
		FROM user_group_memberships ugm
		JOIN users u ON ugm.user_id = u.id
		WHERE ugm.group_id = $1 AND ugm.tenant_id = $2
		ORDER BY u.username
	`, groupID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing group members: %w", err)
	}
	defer rows.Close()

	var members []GroupMember

	for rows.Next() {
		var m GroupMember
		if err := rows.Scan(&m.ID, &m.Username, &m.DisplayName); err != nil {
			return nil, fmt.Errorf("scanning member: %w", err)
		}

		members = append(members, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating members: %w", err)
	}

	return members, nil
}

// ListPermissions returns all registered permission nodes.
func (s *Store) ListPermissions(ctx context.Context) ([]Permission, error) {
	rows, err := s.db.Reader.Query(ctx, `
		SELECT key, description
		FROM permissions
		ORDER BY key
	`)
	if err != nil {
		return nil, fmt.Errorf("listing permissions: %w", err)
	}
	defer rows.Close()

	var perms []Permission

	for rows.Next() {
		var p Permission
		if err := rows.Scan(&p.Key, &p.Description); err != nil {
			return nil, fmt.Errorf("scanning permission: %w", err)
		}

		perms = append(perms, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating permissions: %w", err)
	}

	return perms, nil
}

// Role represents a user role.
type Role struct {
	ID          pgtype.UUID `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
}

// ListRoles returns all roles.
func (s *Store) ListRoles(ctx context.Context) ([]Role, error) {
	rows, err := s.db.Reader.Query(ctx, `
		SELECT id, name, COALESCE(description, '')
		FROM user_roles
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("listing roles: %w", err)
	}
	defer rows.Close()

	var roles []Role

	for rows.Next() {
		var r Role
		if err := rows.Scan(&r.ID, &r.Name, &r.Description); err != nil {
			return nil, fmt.Errorf("scanning role: %w", err)
		}

		roles = append(roles, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating roles: %w", err)
	}

	return roles, nil
}

// CreateRole creates a new role and returns its ID.
func (s *Store) CreateRole(ctx context.Context, tenantID pgtype.UUID, name, _ string) (pgtype.UUID, error) {
	var id pgtype.UUID

	err := s.db.Writer.QueryRow(ctx, `
		INSERT INTO user_roles (tenant_id, name, slug)
		VALUES ($1, $2, $3)
		RETURNING id
	`, tenantID, name, slugify(name)).Scan(&id)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("creating role: %w", err)
	}

	return id, nil
}

// ListRolePermissions returns all permissions assigned to a role.
func (s *Store) ListRolePermissions(ctx context.Context, roleID pgtype.UUID) ([]Permission, error) {
	rows, err := s.db.Reader.Query(ctx, `
		SELECT p.key, p.description
		FROM role_permissions rp
		JOIN permissions p ON rp.permission_id = p.id
		WHERE rp.role_id = $1
		ORDER BY p.key
	`, roleID)
	if err != nil {
		return nil, fmt.Errorf("listing role permissions: %w", err)
	}
	defer rows.Close()

	var perms []Permission

	for rows.Next() {
		var p Permission
		if err := rows.Scan(&p.Key, &p.Description); err != nil {
			return nil, fmt.Errorf("scanning permission: %w", err)
		}

		perms = append(perms, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating permissions: %w", err)
	}

	return perms, nil
}

// ListGroupRoles returns all roles assigned to a group.
func (s *Store) ListGroupRoles(ctx context.Context, groupID pgtype.UUID) ([]Role, error) {
	rows, err := s.db.Reader.Query(ctx, `
		SELECT ur.id, ur.name, COALESCE(ur.description, '')
		FROM group_roles gr
		JOIN user_roles ur ON gr.role_id = ur.id
		WHERE gr.group_id = $1
		ORDER BY ur.name
	`, groupID)
	if err != nil {
		return nil, fmt.Errorf("listing group roles: %w", err)
	}
	defer rows.Close()

	var roles []Role

	for rows.Next() {
		var r Role
		if err := rows.Scan(&r.ID, &r.Name, &r.Description); err != nil {
			return nil, fmt.Errorf("scanning role: %w", err)
		}

		roles = append(roles, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating roles: %w", err)
	}

	return roles, nil
}

// RemoveRoleFromGroup removes a role from a group and invalidates cache.
func (s *Store) RemoveRoleFromGroup(ctx context.Context, groupID, roleID pgtype.UUID) error {
	_, err := s.db.Writer.Exec(ctx, `
		DELETE FROM group_roles WHERE group_id = $1 AND role_id = $2
	`, groupID, roleID)
	if err != nil {
		return fmt.Errorf("removing role from group: %w", err)
	}

	s.invalidateGroupCache(ctx, groupID)

	return nil
}

// RemovePermissionFromRole revokes a permission from a role and invalidates
// cache for all affected groups.
func (s *Store) RemovePermissionFromRole(ctx context.Context, roleID pgtype.UUID, permKey string) error {
	_, err := s.db.Writer.Exec(ctx, `
		DELETE FROM role_permissions
		WHERE role_id = $1 AND permission_id = (SELECT id FROM permissions WHERE key = $2)
	`, roleID, permKey)
	if err != nil {
		return fmt.Errorf("removing permission from role: %w", err)
	}

	// Invalidate cache for all groups that hold this role.
	rows, err := s.db.Reader.Query(ctx, `
		SELECT group_id FROM group_roles WHERE role_id = $1
	`, roleID)
	if err != nil {
		slog.Warn("rbac: failed to list groups for cache invalidation", "role_id", roleID, "error", err)
		return nil
	}

	defer rows.Close()

	for rows.Next() {
		var groupID pgtype.UUID
		if err := rows.Scan(&groupID); err != nil {
			continue
		}

		s.invalidateGroupCache(ctx, groupID)
	}

	return nil
}

// slugify converts a name to a URL-safe slug (lowercase, hyphens for spaces).
func slugify(name string) string {
	slug := strings.ToLower(name)
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = strings.ReplaceAll(slug, "_", "-")

	return slug
}
