package rbac

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store provides CRUD operations for RBAC entities.
// After any mutation that affects permission resolution, the Store calls
// InvalidateUserCache on affected users so the L1/L2 caches stay consistent.
type Store struct {
	read  *gen.Queries
	write *gen.Queries
	db    *db.DB
	rbac  *RBAC // nil-safe: if nil, cache invalidation is skipped
}

// NewStore creates a Store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{
		read:  gen.New(database.Reader),
		write: gen.New(database.Writer),
		db:    database,
	}
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

	rows, err := s.read.ListGroupMembersByGroupID(ctx, groupID)
	if err != nil {
		slog.Warn("rbac: failed to list group members for cache invalidation", "group_id", groupID, "error", err)
		return
	}

	for _, userID := range rows {
		s.invalidateUserCache(ctx, userID)
	}
}

// CreateGroup creates a new user group. System groups cannot be created
// via this method - they are seeded at install time only.
func (s *Store) CreateGroup(ctx context.Context, tenantID pgtype.UUID, name, slug string) (pgtype.UUID, error) {
	if IsSystemGroup(slug) {
		return pgtype.UUID{}, fmt.Errorf("cannot create system group %s via API", slug)
	}

	id, err := s.write.CreateUserGroup(ctx, gen.CreateUserGroupParams{
		TenantID: tenantID,
		Name:     name,
		Slug:     slug,
	})
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("creating group: %w", err)
	}

	return id, nil
}

// DeleteGroup deletes a user group. System groups cannot be deleted.
// After deletion, it invalidates the permission cache for all former
// members so they don't retain the group's capabilities for up to 5
// minutes (Rule 6).
func (s *Store) DeleteGroup(ctx context.Context, tenantID pgtype.UUID, slug string) error {
	if IsSystemGroup(slug) {
		return fmt.Errorf("cannot delete system group %s via API", slug)
	}

	// Collect member IDs before deletion so we can invalidate their caches
	// after the group is removed.
	memberRows, err := s.read.ListUserGroupsByTenant(ctx, tenantID)
	if err != nil {
		slog.Warn("rbac: failed to list groups for member lookup", "tenant_id", tenantID, "error", err)
	}

	var memberIDs []pgtype.UUID
	for _, g := range memberRows {
		if g.Slug == slug {
			memberIDs, err = s.read.ListGroupMembersByGroupID(ctx, g.ID)
			if err != nil {
				slog.Warn("rbac: failed to list group members for cache invalidation", "group_id", g.ID, "error", err)
			}
			break
		}
	}

	if err := s.write.DeleteUserGroup(ctx, gen.DeleteUserGroupParams{
		TenantID: tenantID,
		Slug:     slug,
	}); err != nil {
		return fmt.Errorf("deleting group: %w", err)
	}

	// Invalidate cache for all former members (Rule 6).
	for _, userID := range memberIDs {
		s.invalidateUserCache(ctx, userID)
	}

	return nil
}

// DeleteGroupByID deletes a user group by its UUID. System groups cannot be
// deleted (the sqlc query enforces is_system = false). After deletion, it
// invalidates the permission cache for all former members (Rule 6).
func (s *Store) DeleteGroupByID(ctx context.Context, groupID pgtype.UUID) error {
	// Collect member IDs before deletion so we can invalidate their caches
	// after the group is removed.
	memberIDs, err := s.read.ListGroupMembersByGroupID(ctx, groupID)
	if err != nil {
		slog.Warn("rbac: failed to list group members for cache invalidation", "group_id", groupID, "error", err)
		memberIDs = nil // proceed — don't block the mutation
	}

	if err := s.write.DeleteUserGroupByID(ctx, groupID); err != nil {
		return fmt.Errorf("deleting group by ID: %w", err)
	}

	// Invalidate cache for all former members (Rule 6).
	for _, userID := range memberIDs {
		s.invalidateUserCache(ctx, userID)
	}

	return nil
}

// AddUserToGroup adds a user to a group and invalidates the user's cache.
func (s *Store) AddUserToGroup(ctx context.Context, tenantID, userID, groupID pgtype.UUID) error {
	if err := s.write.AddUserToGroup(ctx, gen.AddUserToGroupParams{
		TenantID: tenantID,
		UserID:   userID,
		GroupID:  groupID,
	}); err != nil {
		return fmt.Errorf("adding user to group: %w", err)
	}

	s.invalidateUserCache(ctx, userID)

	return nil
}

// RemoveUserFromGroup removes a user from a group and invalidates the user's cache.
func (s *Store) RemoveUserFromGroup(ctx context.Context, tenantID, userID, groupID pgtype.UUID) error {
	if err := s.write.RemoveUserFromGroup(ctx, gen.RemoveUserFromGroupParams{
		TenantID: tenantID,
		UserID:   userID,
		GroupID:  groupID,
	}); err != nil {
		return fmt.Errorf("removing user from group: %w", err)
	}

	s.invalidateUserCache(ctx, userID)

	return nil
}

// RegisterPermission creates a permission node if it doesn't exist.
// Called at application startup for all known permission keys.
func (s *Store) RegisterPermission(ctx context.Context, key, description string) error {
	if err := s.write.RegisterPermission(ctx, gen.RegisterPermissionParams{
		Key:         key,
		Description: &description,
	}); err != nil {
		return fmt.Errorf("registering permission %s: %w", key, err)
	}

	return nil
}

// AssignPermissionToRole grants a permission to a role and invalidates cache
// for every user in groups that hold this role.
func (s *Store) AssignPermissionToRole(ctx context.Context, roleID pgtype.UUID, permKey string) error {
	if err := s.write.AssignPermissionToRole(ctx, gen.AssignPermissionToRoleParams{
		RoleID:  roleID,
		PermKey: permKey,
	}); err != nil {
		return fmt.Errorf("assigning permission %s to role: %w", permKey, err)
	}

	// Find all groups that hold this role and invalidate their members' caches.
	groupIDs, err := s.read.ListGroupsByRoleID(ctx, roleID)
	if err != nil {
		slog.Warn("rbac: failed to list groups for role cache invalidation", "role_id", roleID, "error", err)
		return nil // mutation succeeded — don't fail on cache invalidation
	}

	for _, groupID := range groupIDs {
		s.invalidateGroupCache(ctx, groupID)
	}

	return nil
}

// AssignRoleToGroup grants a role to a group and invalidates cache for every
// member of that group (their permissions just changed).
func (s *Store) AssignRoleToGroup(ctx context.Context, groupID, roleID pgtype.UUID) error {
	if err := s.write.AssignRoleToGroup(ctx, gen.AssignRoleToGroupParams{
		GroupID: groupID,
		RoleID:  roleID,
	}); err != nil {
		return fmt.Errorf("assigning role to group: %w", err)
	}

	s.invalidateGroupCache(ctx, groupID)

	return nil
}

// ---------------------------------------------------------------------------
// List / query methods for admin RBAC management
// ---------------------------------------------------------------------------

// ListGroups returns all user groups for a tenant, ordered by name.
// Uses three batch queries (groups, member counts, permissions) instead of
// per-group N+1 lookups.
func (s *Store) ListGroups(ctx context.Context, tenantID pgtype.UUID) ([]Group, error) {
	rows, err := s.read.ListUserGroupsByTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing groups: %w", err)
	}

	// Batch query 1: member counts for all groups in this tenant.
	memberCountRows, err := s.read.CountGroupMembersByTenant(ctx, tenantID)
	if err != nil {
		slog.Warn("rbac: failed to batch count group members", "tenant_id", tenantID, "error", err)
		memberCountRows = nil
	}
	memberCounts := make(map[pgtype.UUID]int32, len(memberCountRows))
	for _, r := range memberCountRows {
		memberCounts[r.GroupID] = r.MemberCount
	}

	// Batch query 2: all permissions for all groups in this tenant.
	permRows, err := s.read.ListGroupPermissionsByTenant(ctx, tenantID)
	if err != nil {
		slog.Warn("rbac: failed to batch list group permissions", "tenant_id", tenantID, "error", err)
		permRows = nil
	}
	groupPerms := make(map[pgtype.UUID]map[string]struct{})
	for _, r := range permRows {
		if groupPerms[r.GroupID] == nil {
			groupPerms[r.GroupID] = make(map[string]struct{})
		}
		groupPerms[r.GroupID][r.Key] = struct{}{}
	}

	groups := make([]Group, 0, len(rows))
	for _, r := range rows {
		g := Group{
			ID:          r.ID,
			TenantID:    r.TenantID,
			Name:        r.Name,
			Slug:        r.Slug,
			IsSystem:    r.IsSystem,
			MemberCount: memberCounts[r.ID],
		}

		if perms, ok := groupPerms[r.ID]; ok {
			g.Permissions = make([]string, 0, len(perms))
			for k := range perms {
				g.Permissions = append(g.Permissions, k)
			}
		}

		groups = append(groups, g)
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
	rows, err := s.read.ListGroupMembersWithUser(ctx, gen.ListGroupMembersWithUserParams{
		GroupID:  groupID,
		TenantID: tenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("listing group members: %w", err)
	}

	members := make([]GroupMember, 0, len(rows))

	for _, r := range rows {
		members = append(members, GroupMember{
			ID:          r.ID,
			Username:    r.Username,
			DisplayName: r.DisplayName,
		})
	}

	return members, nil
}

// ListPermissions returns all registered permission nodes.
func (s *Store) ListPermissions(ctx context.Context) ([]Permission, error) {
	rows, err := s.read.ListAllPermissions(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing permissions: %w", err)
	}

	perms := make([]Permission, 0, len(rows))

	for _, r := range rows {
		desc := ""
		if r.Description != nil {
			desc = *r.Description
		}

		perms = append(perms, Permission{
			Key:         r.Key,
			Description: desc,
		})
	}

	return perms, nil
}

// Role represents a user role.
type Role struct {
	ID         pgtype.UUID `json:"id"`
	Name       string      `json:"name"`
	Slug       string      `json:"slug"`
	Permissions []string   `json:"permissions"`
	UserCount  int32       `json:"user_count"`
}

// ListRoles returns all roles.
func (s *Store) ListRoles(ctx context.Context) ([]Role, error) {
	rows, err := s.read.ListAllRoles(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing roles: %w", err)
	}

	roles := make([]Role, 0, len(rows))

	for _, r := range rows {
		roles = append(roles, Role{
			ID:   r.ID,
			Name: r.Name,
		})
	}

	return roles, nil
}

// ListRolesPaginated returns a paginated list of roles with optional search.
func (s *Store) ListRolesPaginated(ctx context.Context, search string, limit, offset int32) ([]Role, int32, error) {
	rows, err := s.read.ListRolesPaginated(ctx, gen.ListRolesPaginatedParams{
		Search:     search,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("listing roles paginated: %w", err)
	}

	roles := make([]Role, 0, len(rows))
	for _, r := range rows {
		role := Role{
			ID:   r.ID,
			Name: r.Name,
			Slug: r.Slug,
		}

		// Get permissions for this role.
		permRows, err := s.read.ListRolePermissionsByKey(ctx, r.ID)
		if err == nil {
			perms := make([]string, 0, len(permRows))
			for _, p := range permRows {
				perms = append(perms, p.Key)
			}
			role.Permissions = perms
		}

		// Count groups assigned to this role.
		groupRows, err := s.read.ListGroupsByRoleID(ctx, r.ID)
		if err == nil {
			role.UserCount = int32(len(groupRows))
		}

		roles = append(roles, role)
	}

	total, err := s.read.CountRoles(ctx, search)
	if err != nil {
		return nil, 0, fmt.Errorf("counting roles: %w", err)
	}

	return roles, total, nil
}

// CreateRole creates a new role and returns its ID.
func (s *Store) CreateRole(ctx context.Context, tenantID pgtype.UUID, name string) (pgtype.UUID, error) {
	id, err := s.write.CreateUserRole(ctx, gen.CreateUserRoleParams{
		TenantID: tenantID,
		Name:     name,
		Slug:     slugify(name),
	})
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("creating role: %w", err)
	}

	return id, nil
}

// ListRolePermissions returns all permissions assigned to a role.
func (s *Store) ListRolePermissions(ctx context.Context, roleID pgtype.UUID) ([]Permission, error) {
	rows, err := s.read.ListRolePermissionsByKey(ctx, roleID)
	if err != nil {
		return nil, fmt.Errorf("listing role permissions: %w", err)
	}

	perms := make([]Permission, 0, len(rows))

	for _, r := range rows {
		desc := ""
		if r.Description != nil {
			desc = *r.Description
		}

		perms = append(perms, Permission{
			Key:         r.Key,
			Description: desc,
		})
	}

	return perms, nil
}

// ListGroupRoles returns all roles assigned to a group.
func (s *Store) ListGroupRoles(ctx context.Context, groupID pgtype.UUID) ([]Role, error) {
	rows, err := s.read.ListGroupRolesByGroupID(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("listing group roles: %w", err)
	}

	roles := make([]Role, 0, len(rows))

	for _, r := range rows {
		roles = append(roles, Role{
			ID:   r.ID,
			Name: r.Name,
		})
	}

	return roles, nil
}

// RemoveRoleFromGroup removes a role from a group and invalidates cache.
func (s *Store) RemoveRoleFromGroup(ctx context.Context, groupID, roleID pgtype.UUID) error {
	if err := s.write.RemoveRoleFromGroup(ctx, gen.RemoveRoleFromGroupParams{
		GroupID: groupID,
		RoleID:  roleID,
	}); err != nil {
		return fmt.Errorf("removing role from group: %w", err)
	}

	s.invalidateGroupCache(ctx, groupID)

	return nil
}

// RemovePermissionFromRole revokes a permission from a role and invalidates
// cache for all affected groups.
func (s *Store) RemovePermissionFromRole(ctx context.Context, roleID pgtype.UUID, permKey string) error {
	if err := s.write.RemovePermissionFromRole(ctx, gen.RemovePermissionFromRoleParams{
		RoleID:  roleID,
		PermKey: permKey,
	}); err != nil {
		return fmt.Errorf("removing permission from role: %w", err)
	}

	// Invalidate cache for all groups that hold this role.
	groupIDs, err := s.read.ListGroupsByRoleID(ctx, roleID)
	if err != nil {
		slog.Warn("rbac: failed to list groups for cache invalidation", "role_id", roleID, "error", err)
		return nil
	}

	for _, groupID := range groupIDs {
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

// DeleteRole removes a role and all its permission/group assignments (P5.6).
// After deletion, it invalidates the permission cache for every group that
// held this role so members don't retain the role's capabilities for up to
// 5 minutes (Rule 6).
func (s *Store) DeleteRole(ctx context.Context, roleID pgtype.UUID) error {
	// Collect affected groups before deletion so we can invalidate their
	// members' caches after the role is removed.
	groupIDs, err := s.read.ListGroupsByRoleID(ctx, roleID)
	if err != nil {
		slog.Warn("rbac: failed to list groups for role cache invalidation", "role_id", roleID, "error", err)
		groupIDs = nil // proceed — don't block the mutation
	}

	if err := s.write.DeleteUserRole(ctx, roleID); err != nil {
		return fmt.Errorf("deleting role: %w", err)
	}

	// Invalidate cache for all groups that held this role (Rule 6).
	for _, groupID := range groupIDs {
		s.invalidateGroupCache(ctx, groupID)
	}

	return nil
}

// UpdateRole updates a role's name (P5.6).
func (s *Store) UpdateRole(ctx context.Context, roleID pgtype.UUID, name string) error {
	if err := s.write.UpdateUserRole(ctx, gen.UpdateUserRoleParams{
		ID:   roleID,
		Name: name,
	}); err != nil {
		return fmt.Errorf("updating role: %w", err)
	}
	return nil
}

// SetRolePermissions replaces all permissions for a role (P5.6).
// After the transaction commits, it invalidates the permission cache for
// every group that holds this role so members don't retain stale capabilities
// for up to 5 minutes (Rule 6).
func (s *Store) SetRolePermissions(ctx context.Context, roleID pgtype.UUID, permKeys []string) error {
	// Collect affected groups before the transaction so we can invalidate
	// their members' caches after commit.
	groupIDs, err := s.read.ListGroupsByRoleID(ctx, roleID)
	if err != nil {
		slog.Warn("rbac: failed to list groups for role cache invalidation", "role_id", roleID, "error", err)
		groupIDs = nil // proceed — don't block the mutation
	}

	tx, err := s.db.Writer.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	q := gen.New(tx)

	// Remove existing
	if err := q.DeleteRolePermissionsByRoleID(ctx, roleID); err != nil {
		return err
	}
	// Add new
	for _, key := range permKeys {
		if err := q.InsertRolePermissionByKey(ctx, gen.InsertRolePermissionByKeyParams{
			RoleID:  roleID,
			PermKey: key,
		}); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	// Invalidate cache for all groups that hold this role (Rule 6).
	for _, groupID := range groupIDs {
		s.invalidateGroupCache(ctx, groupID)
	}

	return nil
}

// UpdateGroup updates a group's name and slug (P5.6).
func (s *Store) UpdateGroup(ctx context.Context, groupID pgtype.UUID, name, slug string) error {
	if err := s.write.UpdateUserGroup(ctx, gen.UpdateUserGroupParams{
		ID:   groupID,
		Name: name,
		Slug: slug,
	}); err != nil {
		return fmt.Errorf("updating group: %w", err)
	}
	return nil
}

// IsSystemGroupByID checks whether the group with the given ID is one of the
// three system defaults (super_admin, members, guest). Used by handlers to
// prevent dangerous slug changes on system groups.
func (s *Store) IsSystemGroupByID(ctx context.Context, groupID pgtype.UUID) (bool, error) {
	g, err := s.read.GetUserGroupByID(ctx, groupID)
	if err != nil {
		return false, fmt.Errorf("looking up group: %w", err)
	}
	return g.IsSystem, nil
}

// GetGroupByID returns the user group with the given ID, including its
// current slug and is_system flag. Used by handlers that need to preserve
// the slug of system groups during updates.
func (s *Store) GetGroupByID(ctx context.Context, groupID pgtype.UUID) (Group, error) {
	g, err := s.read.GetUserGroupByID(ctx, groupID)
	if err != nil {
		return Group{}, fmt.Errorf("looking up group: %w", err)
	}
	return Group{
		ID:       g.ID,
		TenantID: g.TenantID,
		Name:     g.Name,
		Slug:     g.Slug,
		IsSystem: g.IsSystem,
	}, nil
}
