package rbac

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
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

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes an error JSON response.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
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
		writeError(w, http.StatusUnauthorized, "unauthenticated", "Authentication required")
		return
	}

	groups, err := h.store.ListGroups(r.Context(), tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list groups")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"groups": groups})
}

// CreateGroupInput is the request body for creating a group.
type CreateGroupInput struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// CreateGroup handles POST /api/v1/admin/rbac/groups.
func (h *Handlers) CreateGroup(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "Authentication required")
		return
	}

	var input CreateGroupInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input", "Invalid request body")
		return
	}

	if input.Name == "" || input.Slug == "" {
		writeError(w, http.StatusBadRequest, "invalid_input", "name and slug are required")
		return
	}

	if IsSystemGroup(input.Slug) {
		writeError(w, http.StatusForbidden, "forbidden", "Cannot create system groups via API")
		return
	}

	id, err := h.store.CreateGroup(r.Context(), tenantID, input.Name, input.Slug)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create group")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "name": input.Name, "slug": input.Slug})
}

// DeleteGroup handles DELETE /api/v1/admin/rbac/groups/{id}.
func (h *Handlers) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "Authentication required")
		return
	}

	groupSlug := chi.URLParam(r, "id")
	if IsSystemGroup(groupSlug) {
		writeError(w, http.StatusForbidden, "forbidden", "Cannot delete system groups")
		return
	}

	if err := h.store.DeleteGroup(r.Context(), tenantID, groupSlug); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete group")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListGroupMembers handles GET /api/v1/admin/rbac/groups/{id}/members.
func (h *Handlers) ListGroupMembers(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "Authentication required")
		return
	}

	groupID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid group ID")
		return
	}

	members, err := h.store.ListGroupMembers(r.Context(), groupID, tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list members")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"members": members})
}

// AddGroupMemberInput is the request body for adding a user to a group.
type AddGroupMemberInput struct {
	UserID string `json:"user_id"`
}

// AddGroupMember handles POST /api/v1/admin/rbac/groups/{id}/members.
func (h *Handlers) AddGroupMember(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "Authentication required")
		return
	}

	groupID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid group ID")
		return
	}

	var input AddGroupMemberInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input", "Invalid request body")
		return
	}

	if input.UserID == "" {
		writeError(w, http.StatusBadRequest, "invalid_input", "user_id is required")
		return
	}

	userID, err := parseUUID(input.UserID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	if err := h.store.AddUserToGroup(r.Context(), tenantID, userID, groupID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to add user to group")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"user_id": input.UserID, "group_id": chi.URLParam(r, "id")})
}

// RemoveGroupMember handles DELETE /api/v1/admin/rbac/groups/{id}/members/{userId}.
func (h *Handlers) RemoveGroupMember(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "Authentication required")
		return
	}

	groupID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid group ID")
		return
	}

	userID, err := parseUUID(chi.URLParam(r, "userId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	if err := h.store.RemoveUserFromGroup(r.Context(), tenantID, userID, groupID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to remove user from group")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListPermissions handles GET /api/v1/admin/rbac/permissions.
func (h *Handlers) ListPermissions(w http.ResponseWriter, r *http.Request) {
	perms, err := h.store.ListPermissions(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list permissions")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"permissions": perms})
}

// GetUserPermissions handles GET /api/v1/admin/rbac/user-permissions.
// Returns the resolved permissions for the authenticated user.
func (h *Handlers) GetUserPermissions(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "Authentication required")
		return
	}

	uc, err := h.rbac.GetUserContext(r.Context(), user.UserID, user.TenantID)
	if err != nil || uc == nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"permissions": []string{}}})
		return
	}

	// Convert map to sorted slice for consistent response.
	permKeys := make([]string, 0, len(uc.Permissions))
	for k, v := range uc.Permissions {
		if v {
			permKeys = append(permKeys, k)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"permissions": permKeys}})
}

// ListRoles handles GET /api/v1/admin/rbac/roles.
func (h *Handlers) ListRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := h.store.ListRoles(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list roles")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"roles": roles})
}

// CreateRoleInput is the request body for creating a role.
type CreateRoleInput struct {
	Name string `json:"name"`
}

// CreateRole handles POST /api/v1/admin/rbac/roles.
func (h *Handlers) CreateRole(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "Authentication required")
		return
	}

	var input CreateRoleInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input", "Invalid request body")
		return
	}

	if input.Name == "" {
		writeError(w, http.StatusBadRequest, "invalid_input", "name is required")
		return
	}

	id, err := h.store.CreateRole(r.Context(), tenantID, input.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create role")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "name": input.Name})
}

// ListRolePermissions handles GET /api/v1/admin/rbac/roles/{id}/permissions.
func (h *Handlers) ListRolePermissions(w http.ResponseWriter, r *http.Request) {
	roleID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid role ID")
		return
	}

	perms, err := h.store.ListRolePermissions(r.Context(), roleID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list role permissions")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"permissions": perms})
}

// AssignPermissionInput is the request body for assigning a permission to a role.
type AssignPermissionInput struct {
	PermissionKey string `json:"permission_key"`
}

// AssignPermissionToRole handles POST /api/v1/admin/rbac/roles/{id}/permissions.
func (h *Handlers) AssignPermissionToRole(w http.ResponseWriter, r *http.Request) {
	roleID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid role ID")
		return
	}

	var input AssignPermissionInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input", "Invalid request body")
		return
	}

	if input.PermissionKey == "" {
		writeError(w, http.StatusBadRequest, "invalid_input", "permission_key is required")
		return
	}

	if err := h.store.AssignPermissionToRole(r.Context(), roleID, input.PermissionKey); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to assign permission")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"permission_key": input.PermissionKey})
}

// ListGroupRoles handles GET /api/v1/admin/rbac/groups/{id}/roles.
func (h *Handlers) ListGroupRoles(w http.ResponseWriter, r *http.Request) {
	groupID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid group ID")
		return
	}

	roles, err := h.store.ListGroupRoles(r.Context(), groupID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list group roles")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"roles": roles})
}

// AssignRoleInput is the request body for assigning a role to a group.
type AssignRoleInput struct {
	RoleID string `json:"role_id"`
}

// AssignRoleToGroup handles POST /api/v1/admin/rbac/groups/{id}/roles.
func (h *Handlers) AssignRoleToGroup(w http.ResponseWriter, r *http.Request) {
	groupID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid group ID")
		return
	}

	var input AssignRoleInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input", "Invalid request body")
		return
	}

	if input.RoleID == "" {
		writeError(w, http.StatusBadRequest, "invalid_input", "role_id is required")
		return
	}

	roleID, err := parseUUID(input.RoleID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid role ID")
		return
	}

	if err := h.store.AssignRoleToGroup(r.Context(), groupID, roleID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to assign role")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"role_id": input.RoleID})
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
	roleID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid role ID")
		return
	}
	if err := h.store.DeleteRole(r.Context(), roleID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to delete role")
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// UpdateRole handles PATCH /admin/roles/:id (P5.6).
func (h *Handlers) UpdateRole(w http.ResponseWriter, r *http.Request) {
	roleID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid role ID")
		return
	}
	var input struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input", "Invalid request body")
		return
	}
	if err := h.store.UpdateRole(r.Context(), roleID, input.Name); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update role")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": roleID.String()})
}

// SetRolePermissions handles PATCH /admin/roles/:id/permissions (P5.6).
func (h *Handlers) SetRolePermissions(w http.ResponseWriter, r *http.Request) {
	roleID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid role ID")
		return
	}
	var input struct {
		Permissions []string `json:"permissions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input", "Invalid request body")
		return
	}
	if err := h.store.SetRolePermissions(r.Context(), roleID, input.Permissions); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to set role permissions")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"role_id": roleID.String()})
}

// UpdateGroup handles PATCH /admin/user-groups/:id (P5.6).
func (h *Handlers) UpdateGroup(w http.ResponseWriter, r *http.Request) {
	groupID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid group ID")
		return
	}
	var input struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input", "Invalid request body")
		return
	}
	if err := h.store.UpdateGroup(r.Context(), groupID, input.Name, input.Slug); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update group")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": groupID.String()})
}

// ListScopes handles GET /developer/scopes — returns available API scopes.
func (h *Handlers) ListScopes(w http.ResponseWriter, r *http.Request) {
	scopes := []map[string]string{
		{ "value": "api.read.*", "label": "Read All", "description": "Read access to all resources" },
		{ "value": "content.read", "label": "Read Content", "description": "Read content and articles" },
		{ "value": "content.create", "label": "Create Content", "description": "Create new content" },
		{ "value": "content.edit", "label": "Edit Content", "description": "Edit existing content" },
		{ "value": "content.delete", "label": "Delete Content", "description": "Delete content" },
		{ "value": "spaces.read", "label": "Read Spaces", "description": "Read spaces and communities" },
		{ "value": "spaces.create", "label": "Create Spaces", "description": "Create new spaces" },
		{ "value": "forum.read", "label": "Read Forum", "description": "Read forum threads and replies" },
		{ "value": "forum.create", "label": "Create Forum", "description": "Create forum threads and replies" },
		{ "value": "users.read", "label": "Read Users", "description": "Read user profiles" },
		{ "value": "media.upload", "label": "Upload Media", "description": "Upload media files" },
		{ "value": "notifications.read", "label": "Read Notifications", "description": "Read notifications" },
		{ "value": "search", "label": "Search", "description": "Use search functionality" },
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": scopes})
}
