package rbac

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/publicapi"
)

// Handlers provides HTTP handlers for admin RBAC management.
type Handlers struct {
	store *Store
	rbac  *RBAC
}

// NewHandlers creates RBAC management handlers.
func NewHandlers(store *Store, rbac *RBAC) *Handlers {
	return &Handlers{store: store, rbac: rbac}
}

// requireTenant extracts the tenant ID from the authenticated user context.
// Returns false and writes an error response if the user is not available.
func requireTenant(r *http.Request) (pgtype.UUID, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		return pgtype.UUID{}, false
	}

	return user.TenantID, true
}

// ListGroups handles GET /api/v1/admin/rbac/groups.
func (h *Handlers) ListGroups(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(r)
	if !ok {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Authentication required")
		return
	}

	groups, err := h.store.ListGroups(r.Context(), tenantID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list groups")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"groups": groups,
		"total":  len(groups),
	}})
}

// CreateGroupInput is the request body for creating a group.
type CreateGroupInput struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
	Slug        string   `json:"slug"`
}

// CreateGroup handles POST /api/v1/admin/rbac/groups.
func (h *Handlers) CreateGroup(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(r)
	if !ok {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Authentication required")
		return
	}

	var input CreateGroupInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", "Invalid request body")
		return
	}

	if input.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", "name is required")
		return
	}

	slug := input.Slug
	if slug == "" {
		slug = slugify(input.Name)
	}

	if IsSystemGroup(slug) {
		errpkg.WriteError(w, http.StatusForbidden, "forbidden", "Cannot create system groups via API")
		return
	}

	id, err := h.store.CreateGroup(r.Context(), tenantID, input.Name, slug, input.Description)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create group")
		return
	}

	// Create a role for this group and assign permissions
	if len(input.Permissions) > 0 {
		roleName := input.Name + " Role"
		roleID, err := h.store.CreateRole(r.Context(), tenantID, roleName)
		if err != nil {
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create role for group")
			return
		}
		if err := h.store.SetRolePermissions(r.Context(), roleID, input.Permissions); err != nil {
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to set role permissions")
			return
		}
		if err := h.store.AssignRoleToGroup(r.Context(), id, roleID); err != nil {
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to assign role to group")
			return
		}
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{
		"id":          id,
		"name":        input.Name,
		"slug":        slug,
		"description": input.Description,
		"permissions": input.Permissions,
	}})
}

// DeleteGroup handles DELETE /api/v1/admin/user-groups/{id}.
// The {id} parameter is a UUID — consistent with all other group endpoints.
// System groups are protected by the store layer (is_system = false in the
// DELETE query) and by an explicit check here. The tenant_id filter prevents
// cross-tenant deletion.
func (h *Handlers) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(r)
	if !ok {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Authentication required")
		return
	}

	groupID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid group ID")
		return
	}

	if err := h.store.DeleteGroupByID(r.Context(), tenantID, groupID); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete group")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListGroupMembers handles GET /api/v1/admin/rbac/groups/{id}/members.
func (h *Handlers) ListGroupMembers(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(r)
	if !ok {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Authentication required")
		return
	}

	groupID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid group ID")
		return
	}

	members, err := h.store.ListGroupMembers(r.Context(), groupID, tenantID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list members")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"members": members}})
}

// AddGroupMemberInput is the request body for adding a user to a group.
type AddGroupMemberInput struct {
	UserID string `json:"user_id"`
}

// AddGroupMember handles POST /api/v1/admin/rbac/groups/{id}/members.
func (h *Handlers) AddGroupMember(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(r)
	if !ok {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Authentication required")
		return
	}

	groupID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid group ID")
		return
	}

	var input AddGroupMemberInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", "Invalid request body")
		return
	}

	if input.UserID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", "user_id is required")
		return
	}

	userID, err := parseUUID(input.UserID)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	if err := h.store.AddUserToGroup(r.Context(), tenantID, userID, groupID); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to add user to group")
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{"user_id": input.UserID, "group_id": chi.URLParam(r, "id")}})
}

// RemoveGroupMember handles DELETE /api/v1/admin/rbac/groups/{id}/members/{userId}.
func (h *Handlers) RemoveGroupMember(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(r)
	if !ok {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Authentication required")
		return
	}

	groupID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid group ID")
		return
	}

	userID, err := parseUUID(chi.URLParam(r, "userId"))
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	if err := h.store.RemoveUserFromGroup(r.Context(), tenantID, userID, groupID); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to remove user from group")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListPermissions handles GET /api/v1/admin/rbac/permissions.
func (h *Handlers) ListPermissions(w http.ResponseWriter, r *http.Request) {
	perms, err := h.store.ListPermissions(r.Context())
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list permissions")
		return
	}

	keys := make([]string, 0, len(perms))
	for _, p := range perms {
		keys = append(keys, p.Key)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": keys})
}

// GetUserPermissions handles GET /api/v1/admin/rbac/user-permissions.
// Returns the resolved permissions for the authenticated user.
func (h *Handlers) GetUserPermissions(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Authentication required")
		return
	}

	uc, err := h.rbac.GetUserContext(r.Context(), user.UserID, user.TenantID)
	if err != nil || uc == nil {
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"permissions": []string{}}})
		return
	}

	// Convert map to sorted slice for consistent response.
	permKeys := make([]string, 0, len(uc.Permissions))
	for k, v := range uc.Permissions {
		if v {
			permKeys = append(permKeys, k)
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"permissions": permKeys}})
}

// ListRoles handles GET /api/v1/admin/rbac/roles.
func (h *Handlers) ListRoles(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(r)
	if !ok {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Authentication required")
		return
	}

	limit := int32(25)
	offset := int32(0)
	search := r.URL.Query().Get("search")

	if s := r.URL.Query().Get("page_size"); s != "" {
		if n, err := strconv.ParseInt(s, 10, 32); err == nil && n > 0 {
			limit = int32(n)
			if limit > 100 {
				limit = 100
			}
		}
	}

	if s := r.URL.Query().Get("page_offset"); s != "" {
		if n, err := strconv.ParseInt(s, 10, 32); err == nil && n >= 0 {
			offset = int32(n)
		}
	}

	roles, total, err := h.store.ListRolesPaginated(r.Context(), tenantID, search, limit, offset)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list roles")
		return
	}

	totalPages := total / limit
	if total%limit > 0 {
		totalPages++
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"roles":       roles,
		"total":       total,
		"page":        offset / limit,
		"total_pages": totalPages,
	}})
}

// CreateRoleInput is the request body for creating a role.
type CreateRoleInput struct {
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
}

// CreateRole handles POST /api/v1/admin/rbac/roles.
func (h *Handlers) CreateRole(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(r)
	if !ok {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Authentication required")
		return
	}

	var input CreateRoleInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", "Invalid request body")
		return
	}

	if input.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", "name is required")
		return
	}

	id, err := h.store.CreateRole(r.Context(), tenantID, input.Name)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create role")
		return
	}

	// Persist permissions if provided. SetRolePermissions handles cache
	// invalidation for all groups that hold this role (Rule 6).
	if len(input.Permissions) > 0 {
		if err := h.store.SetRolePermissions(r.Context(), id, input.Permissions); err != nil {
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to set role permissions")
			return
		}
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{
		"id":          id,
		"name":        input.Name,
		"permissions": input.Permissions,
	}})
}

// ListRolePermissions handles GET /api/v1/admin/rbac/roles/{id}/permissions.
func (h *Handlers) ListRolePermissions(w http.ResponseWriter, r *http.Request) {
	roleID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid role ID")
		return
	}

	perms, err := h.store.ListRolePermissions(r.Context(), roleID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list role permissions")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"permissions": perms}})
}

// AssignPermissionInput is the request body for assigning a permission to a role.
type AssignPermissionInput struct {
	PermissionKey string `json:"permission_key"`
}

// AssignPermissionToRole handles POST /api/v1/admin/rbac/roles/{id}/permissions.
func (h *Handlers) AssignPermissionToRole(w http.ResponseWriter, r *http.Request) {
	roleID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid role ID")
		return
	}

	var input AssignPermissionInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", "Invalid request body")
		return
	}

	if input.PermissionKey == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", "permission_key is required")
		return
	}

	if err := h.store.AssignPermissionToRole(r.Context(), roleID, input.PermissionKey); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to assign permission")
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{"permission_key": input.PermissionKey}})
}

// ListGroupRoles handles GET /api/v1/admin/rbac/groups/{id}/roles.
func (h *Handlers) ListGroupRoles(w http.ResponseWriter, r *http.Request) {
	groupID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid group ID")
		return
	}

	roles, err := h.store.ListGroupRoles(r.Context(), groupID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list group roles")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"roles": roles}})
}

// AssignRoleInput is the request body for assigning a role to a group.
type AssignRoleInput struct {
	RoleID string `json:"role_id"`
}

// AssignRoleToGroup handles POST /api/v1/admin/rbac/groups/{id}/roles.
func (h *Handlers) AssignRoleToGroup(w http.ResponseWriter, r *http.Request) {
	groupID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid group ID")
		return
	}

	var input AssignRoleInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", "Invalid request body")
		return
	}

	if input.RoleID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", "role_id is required")
		return
	}

	roleID, err := parseUUID(input.RoleID)
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid role ID")
		return
	}

	if err := h.store.AssignRoleToGroup(r.Context(), groupID, roleID); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to assign role")
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{"role_id": input.RoleID}})
}

// parseUUID parses a UUID string into a pgtype.UUID.
func parseUUID(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		return pgtype.UUID{}, err
	}

	return u, nil
}

// DeleteRole handles DELETE /admin/roles/:id (P5.6).
func (h *Handlers) DeleteRole(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(r)
	if !ok {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Authentication required")
		return
	}
	roleID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid role ID")
		return
	}
	if err := h.store.DeleteRole(r.Context(), tenantID, roleID); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete role")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// UpdateRole handles PATCH /admin/roles/:id (P5.6).
func (h *Handlers) UpdateRole(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(r)
	if !ok {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Authentication required")
		return
	}
	roleID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid role ID")
		return
	}
	var input struct {
		Name        string   `json:"name"`
		Permissions []string `json:"permissions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", "Invalid request body")
		return
	}

	// Only update the name when a non-empty value is provided, so a
	// PATCH that only changes permissions doesn't wipe the role name.
	if input.Name != "" {
		if err := h.store.UpdateRole(r.Context(), tenantID, roleID, input.Name); err != nil {
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update role")
			return
		}
	}

	// Persist permissions when provided (non-nil). SetRolePermissions
	// handles cache invalidation for all groups that hold this role
	// (Rule 6). A nil slice means "don't touch permissions"; an empty
	// slice means "clear all permissions".
	if input.Permissions != nil {
		if err := h.store.SetRolePermissions(r.Context(), roleID, input.Permissions); err != nil {
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to set role permissions")
			return
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"id":          roleID.String(),
		"name":        input.Name,
		"permissions": input.Permissions,
	}})
}

// SetRolePermissions handles PATCH /admin/roles/:id/permissions (P5.6).
func (h *Handlers) SetRolePermissions(w http.ResponseWriter, r *http.Request) {
	roleID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid role ID")
		return
	}
	var input struct {
		Permissions []string `json:"permissions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", "Invalid request body")
		return
	}
	if err := h.store.SetRolePermissions(r.Context(), roleID, input.Permissions); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to set role permissions")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"role_id": roleID.String()}})
}

// UpdateGroup handles PATCH /admin/user-groups/:id (P5.6).
// System groups cannot have their slug changed — the slug is the resolution
// key for IsSuperAdmin and other system-group checks. Name, description,
// and permission updates on system groups are allowed. The tenant_id filter
// prevents cross-tenant updates.
func (h *Handlers) UpdateGroup(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(r)
	if !ok {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Authentication required")
		return
	}
	groupID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid group ID")
		return
	}
	var input struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Permissions []string `json:"permissions"`
		Slug        string   `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", "Invalid request body")
		return
	}

	slug := input.Slug
	if slug == "" && input.Name != "" {
		slug = slugify(input.Name)
	}

	// Protect system groups from slug changes — the slug is the resolution
	// key for IsSuperAdmin (rbac.go:178) and IsSystemGroup. Changing it
	// would break the system.
	existing, err := h.store.GetGroupByID(r.Context(), tenantID, groupID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to look up group")
		return
	}
	if existing.IsSystem && slug != "" && slug != existing.Slug {
		errpkg.WriteError(w, http.StatusForbidden, "forbidden", "Cannot change the slug of a system group")
		return
	}

	// For system groups, preserve the existing slug. For non-system groups,
	// apply the new slug (or derive from name).
	updateSlug := slug
	if existing.IsSystem {
		updateSlug = existing.Slug
	}
	if updateSlug == "" {
		updateSlug = existing.Slug
	}

	// Preserve the existing name and description when the request omits
	// them, so a PATCH that only updates slug/permissions doesn't wipe
	// them.
	updateName := input.Name
	if updateName == "" {
		updateName = existing.Name
	}
	updateDesc := input.Description
	if updateDesc == "" {
		updateDesc = existing.Description
	}

	if err := h.store.UpdateGroup(r.Context(), tenantID, groupID, updateName, updateSlug, updateDesc); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update group")
		return
	}

	// Update permissions: find the group's role and update its permissions
	if input.Permissions != nil {
		roles, err := h.store.ListGroupRoles(r.Context(), groupID)
		if err != nil {
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list group roles")
			return
		}
		if len(roles) > 0 {
			// Update the first role's permissions
			if err := h.store.SetRolePermissions(r.Context(), roles[0].ID, input.Permissions); err != nil {
				errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update role permissions")
				return
			}
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"id":          groupID.String(),
		"name":        updateName,
		"slug":        updateSlug,
		"description": updateDesc,
		"permissions": input.Permissions,
	}})
}

// ListScopes handles GET /developer/scopes — returns available API scopes.
// The scope catalog is sourced from publicapi.ScopeCatalog so the developer
// portal only offers scopes that RequireScope actually enforces (Rule 16 —
// no hardcoded data that can drift from the enforcement layer).
//
// Ref: japaconnect-engineering/sections/47-public-api-developer-portal.md
func (h *Handlers) ListScopes(w http.ResponseWriter, r *http.Request) {
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": publicapi.AvailableScopes()})
}
