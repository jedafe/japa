package rbac

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

// containsWildcard reports whether a permission set includes the system
// wildcard ("*"). Exported helpers/tests use this to spot escalation
// attempts without hardcoding the literal.
func containsWildcard(permKeys []string) bool {
	for _, k := range permKeys {
		if k == SystemWildcardPermission {
			return true
		}
	}

	return false
}

// wildcardRequiresOwner enforces the §03 owner amendment on wildcard grants:
// "*" is the super_admin capability — delegating it through a role or group
// grants full platform access outside the owner-only super_admin lifecycle
// gate, so every grant path that carries "*" is owner-only. Returns false
// (after writing owner_required) when the mutation must abort.
func (h *Handlers) wildcardRequiresOwner(w http.ResponseWriter, r *http.Request, permKeys []string) bool {
	if !containsWildcard(permKeys) {
		return true
	}

	if h.callerIsOwner(r) {
		return true
	}

	errpkg.WriteError(w, http.StatusForbidden, "owner_required",
		"Only the owner can grant the wildcard (*) permission")

	return false
}

// isUniqueViolation detects Postgres unique-constraint errors (23505) —
// slug/name collisions should surface as a client 409, not a 500.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// roleOnSuperAdminGroup reports whether a role backs the super_admin group.
// Mutating such a role's permission set is owner-only — stripping the
// wildcard from it would lock the owner out of every admin endpoint.
// Callers must abort the mutation when err != nil (fail closed — a lookup
// failure must never silently skip the owner gate).
func (h *Handlers) roleOnSuperAdminGroup(ctx context.Context, roleID pgtype.UUID) (bool, error) {
	groupIDs, err := h.store.read.ListGroupsByRoleID(ctx, roleID)
	if err != nil {
		return false, fmt.Errorf("listing groups for role: %w", err)
	}
	for _, gid := range groupIDs {
		if h.isSuperAdminGroup(ctx, gid) {
			return true, nil
		}
	}
	return false, nil
}

// ownerRequired writes the standard owner_required error and returns false —
// the caller must abort. Returns true when the caller is the owner.
func (h *Handlers) ownerRequired(w http.ResponseWriter, r *http.Request, msg string) bool {
	if h.callerIsOwner(r) {
		return true
	}
	errpkg.WriteError(w, http.StatusForbidden, "owner_required", msg)
	return false
}

// handleStoreError maps sentinel store errors to the standard error
// envelope; unmapped errors fall through to a 500.
func (h *Handlers) handleStoreError(w http.ResponseWriter, err error, fallbackMsg string) {
	var unknown *UnknownPermissionError
	switch {
	case errors.Is(err, pgx.ErrNoRows), errors.Is(err, errpkg.ErrNotFound):
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Resource not found")
	case errors.As(err, &unknown):
		errpkg.WriteError(w, http.StatusUnprocessableEntity, "unknown_permission",
			"Unknown permission keys: "+strings.Join(unknown.Keys, ", "))
	case errors.Is(err, ErrSystemRole):
		errpkg.WriteError(w, http.StatusConflict, "system_role",
			"System roles cannot be deleted")
	case isUniqueViolation(err):
		errpkg.WriteError(w, http.StatusConflict, "already_exists",
			"A record with the same name or slug already exists")
	default:
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", fallbackMsg)
	}
}

// groupPermissionRole returns the role that carries a group's permission
// set — the group's first attached role, or an implicit "<name> Role"
// created and attached on demand. The implicit path fixes a silent no-op
// where PATCHing permissions on a role-less group reported success while
// persisting nothing. Role resolution is idempotent under concurrent calls:
// a role matching the derived slug is reused rather than duplicated, and a
// create-time unique violation retries the slug lookup once.
func (h *Handlers) groupPermissionRole(ctx context.Context, groupID pgtype.UUID, groupName string) (pgtype.UUID, error) {
	roles, err := h.store.ListGroupRoles(ctx, groupID)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("listing group roles: %w", err)
	}

	if len(roles) > 0 {
		return roles[0].ID, nil
	}

	roleName := groupName + " Role"
	roleSlug := slugify(roleName)

	// Reuse an existing role with the derived slug instead of creating a
	// duplicate — covers retries and concurrent first-writes.
	if existing, err := h.store.read.GetUserRoleBySlug(ctx, roleSlug); err == nil {
		if err := h.store.AssignRoleToGroup(ctx, groupID, existing.ID); err != nil {
			return pgtype.UUID{}, fmt.Errorf("attaching group role: %w", err)
		}

		return existing.ID, nil
	}

	roleID, err := h.store.CreateRole(ctx, roleName)
	if err != nil {
		// Race: another request created the role between our list and create —
		// the unique slug constraint rejected ours. Re-resolve by slug.
		if existing, slErr := h.store.read.GetUserRoleBySlug(ctx, roleSlug); slErr == nil {
			if err := h.store.AssignRoleToGroup(ctx, groupID, existing.ID); err != nil {
				return pgtype.UUID{}, fmt.Errorf("attaching group role: %w", err)
			}

			return existing.ID, nil
		}

		return pgtype.UUID{}, fmt.Errorf("creating group role: %w", err)
	}

	if err := h.store.AssignRoleToGroup(ctx, groupID, roleID); err != nil {
		return pgtype.UUID{}, fmt.Errorf("attaching group role: %w", err)
	}

	return roleID, nil
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

	// §03 owner amendment: a group's initial permission set may not carry
	// the wildcard unless the caller is the owner.
	if !h.wildcardRequiresOwner(w, r, input.Permissions) {
		return
	}

	// Transactional: group + backing role + permission set + assignment all
	// commit or roll back together (no orphan groups/roles).
	id, err := h.store.CreateGroupWithRole(r.Context(), input.Name, slug, input.Description, input.Permissions)
	if err != nil {
		h.handleStoreError(w, err, "Failed to create group")
		return
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

// ListGroupMembers handles GET /api/v1/admin/user-groups/{id}/members.
// When `page_size` is present the response is paginated
// (page_size, page_offset, search) — required for the members system group
// where every registered user is a member.
func (h *Handlers) ListGroupMembers(w http.ResponseWriter, r *http.Request) {
	groupID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid group ID")
		return
	}

	// 404 (not an empty list) when the group does not exist.
	if _, err := h.store.GetGroupByID(r.Context(), groupID); err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	if r.URL.Query().Get("page_size") != "" {
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

		members, total, err := h.store.ListGroupMembersPaged(r.Context(), groupID, search, limit, offset)
		if err != nil {
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list members")
			return
		}

		totalPages := total / limit
		if total%limit > 0 {
			totalPages++
		}

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
			"members":     members,
			"total":       total,
			"page":        offset / limit,
			"total_pages": totalPages,
		}})

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

	// 404 (not a FK-violation 500) when the group or user does not exist.
	group, err := h.store.GetGroupByID(r.Context(), groupID)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	if _, err := h.store.read.GetUserEmailByID(r.Context(), userID); err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "user_not_found", "User not found")
		return
	}

	// §03 owner amendment: granting super_admin membership is owner-only.
	if group.Slug == SystemGroupSuperAdmin && !h.callerIsOwner(r) {
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

	// 404 when the group does not exist (the membership DELETE otherwise
	// silently affects 0 rows and returns 204).
	group, err := h.store.GetGroupByID(r.Context(), groupID)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	// 404 when the user does not exist — same silent-204 hazard as above.
	// The email doubles as the super_admin owner check below.
	email, emailErr := h.store.read.GetUserEmailByID(r.Context(), userID)
	if emailErr != nil {
		if errors.Is(emailErr, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "user_not_found", "User not found")
		} else {
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to verify target account")
		}

		return
	}

	// §03 owner amendment: removing super_admin membership is owner-only, and
	// the owner can never be demoted (even by themselves). A target-email
	// lookup failure aborts — the owner check must never be skipped silently.
	if group.Slug == SystemGroupSuperAdmin {
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
	sort.Strings(permKeys)

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

	// §03 owner amendment: a role's initial permission set may not carry
	// the wildcard unless the caller is the owner.
	if !h.wildcardRequiresOwner(w, r, input.Permissions) {
		return
	}

	id, err := h.store.CreateRole(r.Context(), input.Name)
	if err != nil {
		h.handleStoreError(w, err, "Failed to create role")
		return
	}

	// Persist permissions if provided. SetRolePermissions handles cache
	// invalidation for all groups that hold this role (Rule 6).
	if len(input.Permissions) > 0 {
		if err := h.store.SetRolePermissions(r.Context(), id, input.Permissions); err != nil {
			h.handleStoreError(w, err, "Failed to set role permissions")
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

// AssignPermissionInput is the request body for assigning permissions to a
// role. `permission_key` grants one node; `permission_keys` grants a batch
// additively — both merge with the role's existing set (they never replace).
type AssignPermissionInput struct {
	PermissionKey  string   `json:"permission_key"`
	PermissionKeys []string `json:"permission_keys"`
}

// AssignPermissionToRole handles POST /api/v1/admin/roles/{id}/permissions.
// Additive grant: each key is merged into the role's existing set. Use
// PATCH /admin/roles/{id}/permissions for full-set replace semantics.
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

	keys := input.PermissionKeys
	if input.PermissionKey != "" {
		keys = append(keys, input.PermissionKey)
	}

	if len(keys) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", "permission_key is required")
		return
	}

	// §03 owner amendment: wildcard grants are owner-only — and writes to a
	// role backing the super_admin group are owner-only outright. A lookup
	// failure fails closed.
	if !h.wildcardRequiresOwner(w, r, keys) {
		return
	}

	onSA, saErr := h.roleOnSuperAdminGroup(r.Context(), roleID)
	if saErr != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to verify role")
		return
	}

	if onSA &&
		!h.ownerRequired(w, r, "Only the owner can modify the super_admin role's permissions") {
		return
	}

	// Validate the whole batch before granting so a bad key at position N
	// doesn't leave keys 1..N-1 already merged.
	if err := h.store.validatePermissionKeys(r.Context(), keys); err != nil {
		h.handleStoreError(w, err, "Failed to assign permission")
		return
	}

	for _, key := range keys {
		if err := h.store.AssignPermissionToRole(r.Context(), roleID, key); err != nil {
			h.handleStoreError(w, err, "Failed to assign permission")
			return
		}
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{"permission_keys": keys}})
}

// ListGroupRoles handles GET /api/v1/admin/user-groups/{id}/roles.
func (h *Handlers) ListGroupRoles(w http.ResponseWriter, r *http.Request) {
	groupID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid group ID")
		return
	}

	// 404 (not an empty list) when the group does not exist.
	if _, err := h.store.GetGroupByID(r.Context(), groupID); err != nil {
		errpkg.HandleServiceError(w, err)
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

	// 404 (not a FK-violation 500) when either side is missing.
	if _, err := h.store.GetGroupByID(r.Context(), groupID); err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	if _, err := h.store.read.GetUserRoleByID(r.Context(), roleID); err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "role_not_found", "Role not found")
		return
	}

	// §03 owner amendment: attaching a role that carries the wildcard to a
	// group grants its members unrestricted access — owner-only, same as a
	// direct "*" grant.
	rolePerms, err := h.store.ListRolePermissions(r.Context(), roleID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to load role permissions")
		return
	}

	keys := make([]string, 0, len(rolePerms))
	for _, p := range rolePerms {
		keys = append(keys, p.Key)
	}

	if containsWildcard(keys) &&
		!h.ownerRequired(w, r, "Only the owner can attach a wildcard-bearing role to a group") {
		return
	}

	if err := h.store.AssignRoleToGroup(r.Context(), groupID, roleID); err != nil {
		h.handleStoreError(w, err, "Failed to assign role")
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{"role_id": input.RoleID}})
}

// RemoveRoleFromGroup handles DELETE /admin/user-groups/{id}/roles/{roleId}.
// Detaches a role from a group (the role itself is not deleted). Members'
// permission caches are invalidated for every affected user (Rule 6).
func (h *Handlers) RemoveRoleFromGroup(w http.ResponseWriter, r *http.Request) {
	groupID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid group ID")
		return
	}

	roleID, err := parseUUID(chi.URLParam(r, "roleId"))
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid role ID")
		return
	}

	group, err := h.store.GetGroupByID(r.Context(), groupID)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	// 404 (not a silent 204) when the role does not exist.
	if _, err := h.store.read.GetUserRoleByID(r.Context(), roleID); err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "role_not_found", "Role not found")
		return
	}

	// §03 owner amendment: detaching a role from the super_admin group would
	// strip the owner's permission source — owner-only.
	if group.Slug == SystemGroupSuperAdmin && !h.ownerRequired(w, r, "Only the owner can detach roles from the super_admin group") {
		return
	}

	if err := h.store.RemoveRoleFromGroup(r.Context(), groupID, roleID); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to detach role from group")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// RemovePermissionFromRole handles DELETE /admin/roles/{id}/permissions/{key}.
// Revokes one permission node from a role without touching the rest of its
// set — the granular counterpart to the PATCH full-set replace.
func (h *Handlers) RemovePermissionFromRole(w http.ResponseWriter, r *http.Request) {
	roleID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid role ID")
		return
	}

	key := chi.URLParam(r, "key")
	if key == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", "permission key is required")
		return
	}

	// 404 (not a silent 204) when the role does not exist.
	if _, err := h.store.read.GetUserRoleByID(r.Context(), roleID); err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "role_not_found", "Role not found")
		return
	}

	// §03 owner amendment: revoking the wildcard anywhere, or mutating the
	// permission set of a role backing the super_admin group, is owner-only —
	// either path could strip the owner's own access (lockout). A lookup
	// failure fails closed — the owner check must never be silently skipped.
	onSA, saErr := h.roleOnSuperAdminGroup(r.Context(), roleID)
	if saErr != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to verify role")
		return
	}

	if key == SystemWildcardPermission || onSA {
		if !h.ownerRequired(w, r, "Only the owner can revoke permissions from the super_admin role or revoke the wildcard") {
			return
		}
	}

	if err := h.store.RemovePermissionFromRole(r.Context(), roleID, key); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to remove permission")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// AddGroupPermissionsInput is the request body for the additive group grant.
type AddGroupPermissionsInput struct {
	PermissionKeys []string `json:"permission_keys"`
}

// AddGroupPermissions handles POST /admin/user-groups/{id}/permissions.
// Additive grant: the keys are merged into the group's permission role
// WITHOUT replacing existing grants — the merge counterpart to the
// replace-semantics PATCH /admin/user-groups/{id} permissions field.
func (h *Handlers) AddGroupPermissions(w http.ResponseWriter, r *http.Request) {
	groupID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid group ID")
		return
	}

	var input AddGroupPermissionsInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", "Invalid request body")
		return
	}

	if len(input.PermissionKeys) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", "permission_keys is required")
		return
	}

	group, err := h.store.GetGroupByID(r.Context(), groupID)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	// §03 owner amendment: the super_admin group's permission role is the
	// owner's power source — all writes to it are owner-only.
	if group.Slug == SystemGroupSuperAdmin && !h.ownerRequired(w, r, "Only the owner can modify the super_admin group's permissions") {
		return
	}

	// §03 owner amendment: wildcard grants are owner-only.
	if !h.wildcardRequiresOwner(w, r, input.PermissionKeys) {
		return
	}

	// Validate the whole batch before resolving/creating the role so a bad
	// key neither leaves partial grants nor an orphan implicit role.
	if err := h.store.validatePermissionKeys(r.Context(), input.PermissionKeys); err != nil {
		h.handleStoreError(w, err, "Failed to grant permission")
		return
	}

	roleID, err := h.groupPermissionRole(r.Context(), groupID, group.Name)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to resolve group role")
		return
	}

	for _, key := range input.PermissionKeys {
		if err := h.store.AssignPermissionToRole(r.Context(), roleID, key); err != nil {
			h.handleStoreError(w, err, "Failed to grant permission")
			return
		}
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{
		"group_id":        groupID.String(),
		"permission_keys": input.PermissionKeys,
	}})
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
		h.handleStoreError(w, err, "Failed to delete role")
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

	// 404 when the role does not exist (UPDATE otherwise no-ops silently).
	if _, err := h.store.read.GetUserRoleByID(r.Context(), roleID); err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "role_not_found", "Role not found")
		return
	}

	// Only update the name when a non-empty value is provided, so a
	// PATCH that only changes permissions doesn't wipe the role name.
	if input.Name != "" {
		if err := h.store.UpdateRole(r.Context(), roleID, input.Name); err != nil {
			h.handleStoreError(w, err, "Failed to update role")
			return
		}
	}

	// Persist permissions when provided (non-nil). SetRolePermissions
	// handles cache invalidation for all groups that hold this role
	// (Rule 6). A nil slice means "don't touch permissions"; an empty
	// slice means "clear all permissions".
	if input.Permissions != nil {
		if !h.wildcardRequiresOwner(w, r, input.Permissions) {
			return
		}

		// §03 owner amendment: a replace on a role backing the super_admin
		// group could strip the wildcard — owner-only (lockout protection).
		// A lookup failure fails closed.
		onSA, saErr := h.roleOnSuperAdminGroup(r.Context(), roleID)
		if saErr != nil {
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to verify role")
			return
		}

		if onSA &&
			!h.ownerRequired(w, r, "Only the owner can modify the super_admin role's permissions") {
			return
		}

		if err := h.store.SetRolePermissions(r.Context(), roleID, input.Permissions); err != nil {
			h.handleStoreError(w, err, "Failed to set role permissions")
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

	// 404 (not a FK-violation 500) when the role does not exist.
	if _, err := h.store.read.GetUserRoleByID(r.Context(), roleID); err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "role_not_found", "Role not found")
		return
	}

	// §03 owner amendment: wildcard grants are owner-only — and a full-set
	// replace on a role backing the super_admin group could strip the
	// wildcard itself (owner lockout), so that path is owner-only outright.
	if !h.wildcardRequiresOwner(w, r, input.Permissions) {
		return
	}

	onSA, saErr := h.roleOnSuperAdminGroup(r.Context(), roleID)
	if saErr != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to verify role")
		return
	}

	if onSA &&
		!h.ownerRequired(w, r, "Only the owner can modify the super_admin role's permissions") {
		return
	}

	if err := h.store.SetRolePermissions(r.Context(), roleID, input.Permissions); err != nil {
		h.handleStoreError(w, err, "Failed to set role permissions")
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
		Description *string  `json:"description"` // pointer: absent = keep, "" = clear
		Permissions []string `json:"permissions"`
		Slug        string   `json:"slug"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", "Invalid request body")
		return
	}

	// The slug only changes when the caller explicitly provides one —
	// renaming a group must NOT silently rewrite the slug, because slugs are
	// resolution keys for feature-flag audience overlays, custom-field
	// visibility, and IsUserInGroup checks.
	slug := input.Slug
	if slug != "" {
		slug = slugify(slug)
	}

	// Protect system groups from slug changes — the slug is the resolution
	// key for IsSuperAdmin (rbac.go:178) and IsSystemGroup. Changing it
	// would break the system.
	existing, err := h.store.GetGroupByID(r.Context(), groupID)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	if existing.IsSystem && slug != "" && slug != existing.Slug {
		errpkg.WriteError(w, http.StatusForbidden, "forbidden", "Cannot change the slug of a system group")
		return
	}

	if IsSystemGroup(slug) && slug != existing.Slug {
		errpkg.WriteError(w, http.StatusForbidden, "forbidden", "Cannot take the slug of a system group")
		return
	}

	updateSlug := slug
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

	updateDesc := existing.Description
	if input.Description != nil {
		updateDesc = *input.Description
	}

	// §03 owner amendment: wildcard grants are owner-only on every path —
	// and replacing the super_admin group's permission set is owner-only
	// outright (it carries the wildcard that is the owner's power source).
	if !h.wildcardRequiresOwner(w, r, input.Permissions) {
		return
	}

	if existing.Slug == SystemGroupSuperAdmin && input.Permissions != nil {
		if !h.ownerRequired(w, r, "Only the owner can modify the super_admin group's permissions") {
			return
		}
	}

	if err := h.store.UpdateGroup(r.Context(), groupID, updateName, updateSlug, updateDesc); err != nil {
		h.handleStoreError(w, err, "Failed to update group")
		return
	}

	// Update permissions on the group's permission role (first attached
	// role, or an implicit "<name> Role" created on demand when the group
	// has none — previously the write was silently skipped).
	if input.Permissions != nil {
		roleID, err := h.groupPermissionRole(r.Context(), groupID, updateName)
		if err != nil {
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to resolve group role")
			return
		}

		if err := h.store.SetRolePermissions(r.Context(), roleID, input.Permissions); err != nil {
			h.handleStoreError(w, err, "Failed to update role permissions")
			return
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
