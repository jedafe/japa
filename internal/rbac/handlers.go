package rbac

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/publicapi/scopes"
)

// Handlers provides HTTP handlers for admin RBAC management.
type Handlers struct {
	store *Store
	rbac  *RBAC
	// ownerEmail is the JC_OWNER_EMAIL platform-owner designation (§03 owner
	// amendment). super_admin group membership mutations are owner-only.
	ownerEmail string
	// fieldPermInvalidator clears a user's cached custom-field permission
	// sets (jc:field_perm:{user_id} + jc:field_perm_edit:{user_id}) after
	// membership add/remove (CF8). Nil disables the hook (no customfields
	// wiring in that deployment); failures never fail the request.
	fieldPermInvalidator FieldPermissionCacheInvalidator
}

// FieldPermissionCacheInvalidator clears cached custom-field permission
// sets for one user. Implemented by *customfields.Service via
// InvalidateFieldPermissionCache (wired in server.go to avoid an
// rbac→customfields import cycle).
type FieldPermissionCacheInvalidator interface {
	InvalidateFieldPermissionCache(ctx context.Context, userID pgtype.UUID)
}

// NewHandlers creates RBAC management handlers.
func NewHandlers(store *Store, rbac *RBAC) *Handlers {
	return &Handlers{store: store, rbac: rbac}
}

// SetFieldPermissionCacheInvalidator wires the customfields cache hook
// (CF8). Production wires *customfields.Service; tests may omit it.
func (h *Handlers) SetFieldPermissionCacheInvalidator(f FieldPermissionCacheInvalidator) {
	h.fieldPermInvalidator = f
}

// SetOwnerEmail wires the JC_OWNER_EMAIL designation (§03 owner amendment).
func (h *Handlers) SetOwnerEmail(email string) {
	h.ownerEmail = email
}

// callerIsOwner reports whether the authenticated caller is the configured
// platform owner. Fails closed on lookup error.
func (h *Handlers) callerIsOwner(r *http.Request) bool {
	u := auth.FromContext(r.Context())
	if u == nil || !u.UserID.Valid {
		return false
	}

	email, err := h.store.read.GetUserEmailByID(r.Context(), u.UserID)
	if err != nil {
		return false
	}

	return auth.IsOwnerEmail(email, h.ownerEmail)
}

// isSuperAdminGroup resolves the group and reports whether it is the
// super_admin system group.
func (h *Handlers) isSuperAdminGroup(ctx context.Context, groupID pgtype.UUID) bool {
	g, err := h.store.read.GetUserGroupByID(ctx, groupID)
	return err == nil && g.Slug == SystemGroupSuperAdmin
}

// ListGroups handles GET /api/v1/admin/user-groups.
// Two modes: when `page_size` is present the request is paginated
// (page_size, page_offset, search, sort, sort_dir — used by the admin
// groups table); without it the full, unpaginated list is returned (used
// by dropdown option loaders on other admin pages).
func (h *Handlers) ListGroups(w http.ResponseWriter, r *http.Request) {
	// Paged mode — the admin groups table drives these params.
	if r.URL.Query().Get("page_size") != "" {
		limit := int32(25)
		offset := int32(0)
		search := r.URL.Query().Get("search")
		sortKey := r.URL.Query().Get("sort")
		sortDir := r.URL.Query().Get("sort_dir")

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

		groups, total, err := h.store.ListGroupsPaged(r.Context(), search, sortKey, sortDir, limit, offset)
		if err != nil {
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list groups")
			return
		}

		totalPages := total / limit
		if total%limit > 0 {
			totalPages++
		}

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
			"groups":      groups,
			"total":       total,
			"page":        offset / limit,
			"total_pages": totalPages,
		}})

		return
	}

	groups, err := h.store.ListGroups(r.Context())
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

	id, err := h.store.CreateGroup(r.Context(), input.Name, slug, input.Description)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create group")
		return
	}

	// Create a role for this group and assign permissions
	if len(input.Permissions) > 0 {
		roleName := input.Name + " Role"

		roleID, err := h.store.CreateRole(r.Context(), roleName)
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
// DELETE query) and by an explicit check here. The filter prevents
// cross-platform deletion.
func (h *Handlers) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	groupID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid group ID")
		return
	}

	// The delete query silently no-ops on system groups (is_system = false
	// filter) and missing rows alike — distinguish them so the API doesn't
	// report success for a group that still exists.
	group, err := h.store.GetGroupByID(r.Context(), groupID)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	if group.IsSystem {
		errpkg.WriteError(w, http.StatusConflict, "system_group", "System groups cannot be deleted")
		return
	}

	if err := h.store.DeleteGroupByID(r.Context(), groupID); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete group")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListGroupMembers handles GET /api/v1/admin/rbac/groups/{id}/members.
func (h *Handlers) ListGroupMembers(w http.ResponseWriter, r *http.Request) {
	groupID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid group ID")
		return
	}

	members, err := h.store.ListGroupMembers(r.Context(), groupID)
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

	// §03 owner amendment: granting super_admin membership is owner-only.
	if h.isSuperAdminGroup(r.Context(), groupID) && !h.callerIsOwner(r) {
		errpkg.WriteError(w, http.StatusForbidden, "owner_required", "Only the owner can grant super_admin membership")
		return
	}

	if err := h.store.AddUserToGroup(r.Context(), userID, groupID); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to add user to group")
		return
	}

	// CF8: membership changes the user's field-level grants — flush their
	// cached field permission sets so the change is visible immediately.
	if h.fieldPermInvalidator != nil {
		h.fieldPermInvalidator.InvalidateFieldPermissionCache(r.Context(), userID)
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{"user_id": input.UserID, "group_id": chi.URLParam(r, "id")}})
}

// RemoveGroupMember handles DELETE /api/v1/admin/rbac/groups/{id}/members/{userId}.
func (h *Handlers) RemoveGroupMember(w http.ResponseWriter, r *http.Request) {
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

	// §03 owner amendment: removing super_admin membership is owner-only, and
	// the owner can never be demoted (even by themselves). A target-email
	// lookup failure aborts — the owner check must never be skipped silently.
	if h.isSuperAdminGroup(r.Context(), groupID) {
		email, emailErr := h.store.read.GetUserEmailByID(r.Context(), userID)
		if emailErr != nil {
			if errors.Is(emailErr, pgx.ErrNoRows) {
				errpkg.WriteError(w, http.StatusNotFound, "user_not_found", "User not found")
			} else {
				errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to verify target account")
			}

			return
		}

		if auth.IsOwnerEmail(email, h.ownerEmail) {
			errpkg.WriteError(w, http.StatusForbidden, "owner_protected", "The owner cannot be removed from the super_admin group")
			return
		}

		if !h.callerIsOwner(r) {
			errpkg.WriteError(w, http.StatusForbidden, "owner_required", "Only the owner can remove super_admin members")
			return
		}
	}

	if err := h.store.RemoveUserFromGroup(r.Context(), userID, groupID); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to remove user from group")
		return
	}

	// CF8: membership removal revokes field-level grants — flush the cache.
	if h.fieldPermInvalidator != nil {
		h.fieldPermInvalidator.InvalidateFieldPermissionCache(r.Context(), userID)
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

	uc, err := h.rbac.GetUserContext(r.Context(), user.UserID)
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
	limit := int32(25)
	offset := int32(0)
	search := r.URL.Query().Get("search")
	sortKey := r.URL.Query().Get("sort")
	sortDir := r.URL.Query().Get("sort_dir")

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

	roles, total, err := h.store.ListRolesPaginated(r.Context(), search, sortKey, sortDir, limit, offset)
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
	var input CreateRoleInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", "Invalid request body")
		return
	}

	if input.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", "name is required")
		return
	}

	id, err := h.store.CreateRole(r.Context(), input.Name)
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
	roleID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid role ID")
		return
	}

	if err := h.store.DeleteRole(r.Context(), roleID); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete role")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// UpdateRole handles PATCH /admin/roles/:id (P5.6).
func (h *Handlers) UpdateRole(w http.ResponseWriter, r *http.Request) {
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
		if err := h.store.UpdateRole(r.Context(), roleID, input.Name); err != nil {
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
// and permission updates on system groups are allowed. The filter
// prevents cross-platform updates.
func (h *Handlers) UpdateGroup(w http.ResponseWriter, r *http.Request) {
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
	existing, err := h.store.GetGroupByID(r.Context(), groupID)
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

	if err := h.store.UpdateGroup(r.Context(), groupID, updateName, updateSlug, updateDesc); err != nil {
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

// ListScopes handles GET /developer/scopes — returns the API scopes a
// self-serve caller may request on their own keys. Admin-only scopes
// (api.write.*, *) are withheld — minting them is rejected with
// scope_requires_admin, so offering them in the portal would be dead UI.
// The catalog is sourced from the canonical scopes.ScopeCatalog (the leaf
// package internal/publicapi/scopes — rbac cannot import publicapi itself
// without creating an import cycle through the moat domain services) so the
// developer portal only offers scopes that RequireScope actually enforces
// (Rule 16 — no hardcoded data that can drift from the enforcement layer).
// // Ref: japaconnect-engineering/sections/47-public-api-developer-portal.md.
func (h *Handlers) ListScopes(w http.ResponseWriter, r *http.Request) {
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": scopes.SelfServiceScopes()})
}

// ListAllScopes handles GET /control/api-scopes — the complete catalog,
// including admin-only scopes, for the control panel's key-scope editor.
func (h *Handlers) ListAllScopes(w http.ResponseWriter, r *http.Request) {
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": scopes.AvailableScopes()})
}
