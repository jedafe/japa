package customfields

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/valkey"
)

// ---------------------------------------------------------------------------
// Custom Field Groups
// ---------------------------------------------------------------------------

// ListFieldGroups returns every custom field group owned by the given platform,
// newest first.
func (s *Service) ListFieldGroups(ctx context.Context) ([]gen.CustomFieldGroup, error) {
	groups, err := s.read.ListCustomFieldGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing custom field groups: %w", err)
	}

	return groups, nil
}

// ListFieldGroupsByEntityClass returns custom field groups owned by the given
// platform that are assigned to the given entity class (e.g. "form", "space_type"),
// newest first.
func (s *Service) ListFieldGroupsByEntityClass(ctx context.Context, entityClass string) ([]gen.CustomFieldGroup, error) {
	groups, err := s.read.ListCustomFieldGroupsAndEntityClass(ctx,

		entityClass,
	)
	if err != nil {
		return nil, fmt.Errorf("listing custom field groups by entity class: %w", err)
	}

	return groups, nil
}

// ListAllAssignmentsByPlatform returns every group assignment for groups owned
// by the given platform. Used to enrich ListFieldGroups with entity_type without
// N+1 per-group assignment lookups.
func (s *Service) ListAllAssignmentsByPlatform(ctx context.Context) ([]gen.CustomFieldGroupAssignment, error) {
	assignments, err := s.read.ListAllGroupAssignments(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing assignments by platform groups: %w", err)
	}

	return assignments, nil
}

// ListAllFieldsByPlatform returns every custom field for groups owned by the
// given platform. Used to enrich ListFieldGroups with fields[] without N+1
// per-group field lookups.
func (s *Service) ListAllFieldsByPlatform(ctx context.Context) ([]gen.ListAllCustomFieldsRow, error) {
	fields, err := s.read.ListAllCustomFields(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing all custom fields by platform: %w", err)
	}

	return fields, nil
}

// GetFieldGroup returns the custom field group with the given ID, scoped to
// the platform.
func (s *Service) GetFieldGroup(ctx context.Context, id pgtype.UUID) (gen.CustomFieldGroup, error) {
	group, err := s.read.GetCustomFieldGroupByID(ctx,
		id,
	)
	if err != nil {
		return gen.CustomFieldGroup{}, fmt.Errorf("getting custom field group: %w", err)
	}

	return group, nil
}

// GetFieldGroupBySlug returns the custom field group matching the given slug,
// scoped to the platform.
func (s *Service) GetFieldGroupBySlug(ctx context.Context, slug string) (gen.CustomFieldGroup, error) {
	group, err := s.read.GetCustomFieldGroupBySlug(ctx,
		slug,
	)
	if err != nil {
		return gen.CustomFieldGroup{}, fmt.Errorf("getting custom field group by slug: %w", err)
	}

	return group, nil
}

// CreateFieldGroup creates a new custom field group. System groups can only be
// seeded at install time; set isSystem to false for platform-created groups.
func (s *Service) CreateFieldGroup(ctx context.Context, name, slug string, description *string, isSystem bool) (gen.CustomFieldGroup, error) {
	group, err := s.write.CreateCustomFieldGroup(ctx, gen.CreateCustomFieldGroupParams{

		Name:        name,
		Slug:        slug,
		Description: description,
		IsSystem:    isSystem,
	})
	if err != nil {
		return gen.CustomFieldGroup{}, fmt.Errorf("creating custom field group: %w", err)
	}

	return group, nil
}

// UpdateFieldGroup updates the name and description of a non-system field
// group, scoped to the platform.
func (s *Service) UpdateFieldGroup(ctx context.Context, id pgtype.UUID, name string, description *string) (gen.CustomFieldGroup, error) {
	group, err := s.write.UpdateCustomFieldGroup(ctx, gen.UpdateCustomFieldGroupParams{
		Name:        name,
		Description: description,
		ID:          id,
	})
	if err != nil {
		return gen.CustomFieldGroup{}, fmt.Errorf("updating custom field group: %w", err)
	}

	return group, nil
}

// DeleteFieldGroup deletes a non-system custom field group, scoped to the
// platform. System groups are protected at the database level.
// BUG-008: without force, rejects deletion when the group has existing
// custom_field_values (i.e. the group is actively storing data for entities).
// This prevents silent data loss when an admin deletes a group that's in use.
// With force (DELETE ?force=true, Section 06), the single DELETE statement
// cascades through custom_fields → custom_field_values and
// custom_field_group_assignments. References declared ON DELETE SET NULL —
// space_types.field_group_id and wiki_type_categories.field_group_id — are
// detached (the space type / wiki category loses its bound group). References
// without a cascade — ticket_types.field_group_id and
// comparison_type_definitions.field_group_id — still block via FK violation
// and surface as ErrInUse.
func (s *Service) DeleteFieldGroup(ctx context.Context, id pgtype.UUID, force bool) error {
	if !force {
		// Check for existing field values referencing this group's fields.
		count, err := s.read.CountCustomFieldValuesByGroup(ctx,
			id,
		)
		if err != nil {
			return fmt.Errorf("checking field group usage: %w", err)
		}

		if count > 0 {
			return fmt.Errorf("%w: cannot delete field group with %d existing value(s)", errpkg.ErrInUse, count)
		}
	}

	if err := s.write.DeleteCustomFieldGroup(ctx,
		id,
	); err != nil {
		// foreign_key_violation — the group is still referenced by a
		// non-cascading table (e.g. ticket_types, comparison_type_definitions).
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return fmt.Errorf("%w: field group is still referenced by %s", errpkg.ErrInUse, pgErr.TableName)
		}
		return fmt.Errorf("deleting custom field group: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Custom Fields
// ---------------------------------------------------------------------------

// ListFieldsByGroup returns every custom field belonging to the given group,
// ordered by position then creation time.
func (s *Service) ListFieldsByGroup(ctx context.Context, groupID pgtype.UUID) ([]gen.ListCustomFieldsByGroupRow, error) {
	fields, err := s.read.ListCustomFieldsByGroup(ctx,
		groupID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing custom fields by group: %w", err)
	}

	return fields, nil
}

// GetField returns the custom field with the given ID, scoped to the platform.
func (s *Service) GetField(ctx context.Context, id pgtype.UUID) (gen.GetCustomFieldByIDRow, error) {
	field, err := s.read.GetCustomFieldByID(ctx,
		id,
	)
	if err != nil {
		return gen.GetCustomFieldByIDRow{}, fmt.Errorf("getting custom field: %w", err)
	}

	return field, nil
}

// CreateField creates a new custom field. The caller is responsible for
// validating the field type via IsValidFieldType before calling this method.
func (s *Service) CreateField(ctx context.Context, params gen.CreateCustomFieldParams) (gen.CreateCustomFieldRow, error) {
	field, err := s.write.CreateCustomField(ctx, params)
	if err != nil {
		return gen.CreateCustomFieldRow{}, fmt.Errorf("creating custom field: %w", err)
	}

	return field, nil
}

// UpdateField updates a non-system custom field. System fields are protected
// at the database level.
func (s *Service) UpdateField(ctx context.Context, params gen.UpdateCustomFieldParams) (gen.UpdateCustomFieldRow, error) {
	field, err := s.write.UpdateCustomField(ctx, params)
	if err != nil {
		return gen.UpdateCustomFieldRow{}, fmt.Errorf("updating custom field: %w", err)
	}

	return field, nil
}

// DeleteField deletes a non-system custom field, scoped to the platform. System
// fields are protected at the database level.
func (s *Service) DeleteField(ctx context.Context, id pgtype.UUID) error {
	if err := s.write.DeleteCustomField(ctx,
		id,
	); err != nil {
		return fmt.Errorf("deleting custom field: %w", err)
	}

	return nil
}

// UpdateFieldPosition updates a single field's position within its group,
// scoped to the platform. Used by ReorderFields to persist a bulk reorder as a
// sequence of single-field updates, mirroring the comparison package's
// ReorderComparisonItem.
func (s *Service) UpdateFieldPosition(ctx context.Context, id, groupID pgtype.UUID, position int32) error {
	if err := s.write.UpdateCustomFieldPosition(ctx, gen.UpdateCustomFieldPositionParams{
		Position: position,
		ID:       id,
		GroupID:  groupID,
	}); err != nil {
		return fmt.Errorf("updating custom field position: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Group Assignments
// ---------------------------------------------------------------------------

// ListAssignmentsByEntityClass returns every group assignment for the given
// entity class (e.g. "content_type", "space_type"), ordered by position.
func (s *Service) ListAssignmentsByEntityClass(ctx context.Context, entityClass string) ([]gen.CustomFieldGroupAssignment, error) {
	assignments, err := s.read.ListAssignmentsByEntityClass(ctx, entityClass)
	if err != nil {
		return nil, fmt.Errorf("listing assignments by entity class: %w", err)
	}

	return assignments, nil
}

// CreateAssignment assigns a custom field group to an entity. If the
// assignment already exists it is a no-op (ON CONFLICT DO NOTHING).
func (s *Service) CreateAssignment(ctx context.Context, params gen.CreateAssignmentParams) (gen.CustomFieldGroupAssignment, error) {
	assignment, err := s.write.CreateAssignment(ctx, params)
	if err != nil {
		return gen.CustomFieldGroupAssignment{}, fmt.Errorf("creating group assignment: %w", err)
	}

	return assignment, nil
}

// DeleteAssignment removes a group assignment matching the given group,
// entity class, and entity ID.
func (s *Service) DeleteAssignment(ctx context.Context, groupID pgtype.UUID, entityClass string, entityID pgtype.UUID) error {
	if err := s.write.DeleteAssignment(ctx, gen.DeleteAssignmentParams{
		GroupID:     groupID,
		EntityClass: entityClass,
		EntityID:    entityID,
	}); err != nil {
		return fmt.Errorf("deleting group assignment: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Field Permissions
// ---------------------------------------------------------------------------

// ListPermissionsByField returns every RBAC permission row for the given
// field, one per user group.
func (s *Service) ListPermissionsByField(ctx context.Context, fieldID pgtype.UUID) ([]gen.CustomFieldPermission, error) {
	perms, err := s.read.ListPermissionsByField(ctx, fieldID)
	if err != nil {
		return nil, fmt.Errorf("listing field permissions: %w", err)
	}

	return perms, nil
}

// UpsertFieldPermission inserts or updates a permission row for a field and
// user group pair.
func (s *Service) UpsertFieldPermission(ctx context.Context, params gen.UpsertFieldPermissionParams) error {
	if err := s.write.UpsertFieldPermission(ctx, params); err != nil {
		return fmt.Errorf("upserting field permission: %w", err)
	}

	return nil
}

// DeleteFieldPermission removes the permission row for the given field and
// user group.
func (s *Service) DeleteFieldPermission(ctx context.Context, fieldID, userGroupID pgtype.UUID) error {
	if err := s.write.DeleteFieldPermission(ctx, gen.DeleteFieldPermissionParams{
		FieldID:     fieldID,
		UserGroupID: userGroupID,
	}); err != nil {
		return fmt.Errorf("deleting field permission: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Field Values
// ---------------------------------------------------------------------------

// GetFieldValue returns the stored value for a single field on a given entity.
func (s *Service) GetFieldValue(ctx context.Context, fieldID pgtype.UUID, entityType string, entityID pgtype.UUID) (gen.CustomFieldValue, error) {
	value, err := s.read.GetCustomFieldValue(ctx, gen.GetCustomFieldValueParams{
		FieldID:    fieldID,
		EntityType: entityType,
		EntityID:   entityID,
	})
	if err != nil {
		return gen.CustomFieldValue{}, fmt.Errorf("getting custom field value: %w", err)
	}

	return value, nil
}

// ListFieldValuesByEntity returns every stored field value for a given entity.
// ID is required — the underlying query enforces platform scoping at the
// SQL layer via a JOIN to custom_fields. (BUG-014). Passing a
// zero-value UUID would yield no rows.
func (s *Service) ListFieldValuesByEntity(ctx context.Context, entityType string, entityID pgtype.UUID) ([]gen.CustomFieldValue, error) {
	values, err := s.read.ListCustomFieldValuesByEntity(ctx, gen.ListCustomFieldValuesByEntityParams{

		EntityType: entityType,
		EntityID:   entityID,
	})
	if err != nil {
		return nil, fmt.Errorf("listing custom field values by entity: %w", err)
	}

	return values, nil
}

// ListFieldValuesByEntityForUser returns the stored field values for a given
// entity, filtered to only fields the user is allowed to view per the
// field-level RBAC (Section 06). When visibleFieldIDs is empty, no values are
// returned (fail-closed). Super-admins should bypass this method by calling
// ListFieldValuesByEntity directly.
// // BUG-017: the visible-field-ID filter is pushed into SQL via
// ListCustomFieldValuesByEntityForUser (field_id = ANY(visible_field_ids))
// instead of fetching all values and filtering in Go.
func (s *Service) ListFieldValuesByEntityForUser(ctx context.Context, entityType string, entityID pgtype.UUID, visibleFieldIDs []pgtype.UUID) ([]gen.CustomFieldValue, error) {
	if len(visibleFieldIDs) == 0 {
		return []gen.CustomFieldValue{}, nil
	}

	values, err := s.read.ListCustomFieldValuesByEntityForUser(ctx, gen.ListCustomFieldValuesByEntityForUserParams{
		EntityType: entityType,
		EntityID:   entityID,

		VisibleFieldIds: visibleFieldIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("listing custom field values by entity for user: %w", err)
	}

	return values, nil
}

// ListFieldValuesWithDefsByEntity returns the stored field values for a given
// entity joined with their custom_fields definitions (label, key, field_type,
// options, position), filtered to show_in_frontend = true. This is the
// display-ready shape consumed by public surfaces like the
// /universities/{slug} About facts grid (Section 39) — it carries both the
// stored value and the metadata needed to render it in one row, so the caller
// never has to do a second lookup to resolve a field_id to a label.
// Platform-scoped via the JOIN (BUG-014).
func (s *Service) ListFieldValuesWithDefsByEntity(ctx context.Context, entityType string, entityID pgtype.UUID) ([]gen.ListCustomFieldValuesWithDefsByEntityRow, error) {
	rows, err := s.read.ListCustomFieldValuesWithDefsByEntity(ctx, gen.ListCustomFieldValuesWithDefsByEntityParams{
		EntityType: entityType,
		EntityID:   entityID,
	})
	if err != nil {
		return nil, fmt.Errorf("listing custom field values with defs by entity: %w", err)
	}

	return rows, nil
}

// CheckFieldPermissionForUser returns the can_view / can_edit flags for the
// given user on the given field (Section 06 field-level RBAC). When no
// permission rows exist for the field, both default to false (fail-closed).
func (s *Service) CheckFieldPermissionForUser(ctx context.Context, fieldID, userID pgtype.UUID) (canView, canEdit bool, err error) {
	row, err := s.read.CheckFieldPermissionForUser(ctx, gen.CheckFieldPermissionForUserParams{
		FieldID: fieldID,
		UserID:  userID,
	})
	if err != nil {
		return false, false, fmt.Errorf("checking field permission: %w", err)
	}
	// sqlc emits interface{} for COALESCE(bool_or(...), false) — assert bool.
	if v, ok := row.CanView.(bool); ok {
		canView = v
	}

	if v, ok := row.CanEdit.(bool); ok {
		canEdit = v
	}

	return canView, canEdit, nil
}

// ListVisibleFieldIDsForUser returns the set of field IDs the user is allowed
// to view per field-level RBAC (Section 06). Results are cached in Valkey
// under jc:field_perm:{user_id} with a 5-minute TTL (BUG-016). When the
// cache is unavailable or not configured, the DB is queried directly.
func (s *Service) ListVisibleFieldIDsForUser(ctx context.Context, userID pgtype.UUID) ([]pgtype.UUID, error) {
	// BUG-016: try Valkey cache first.
	if s.cache != nil {
		key := valkey.MustKey("field_perm:%s", userID.String())
		if cached, err := s.cache.Get(ctx, key).Bytes(); err == nil && len(cached) > 0 {
			var ids []string
			if json.Unmarshal(cached, &ids) == nil && len(ids) > 0 {
				result := make([]pgtype.UUID, 0, len(ids))

				for _, idStr := range ids {
					var id pgtype.UUID
					if err := id.Scan(idStr); err == nil {
						result = append(result, id)
					}
				}

				if len(result) > 0 {
					return result, nil
				}
			}
		}
	}

	ids, err := s.read.ListVisibleFieldIDsForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("listing visible field ids: %w", err)
	}

	// BUG-016: cache the result for 5 minutes.
	if s.cache != nil && len(ids) > 0 {
		key := valkey.MustKey("field_perm:%s", userID.String())

		idStrs := make([]string, 0, len(ids))
		for _, id := range ids {
			idStrs = append(idStrs, id.String())
		}

		if data, err := json.Marshal(idStrs); err == nil {
			s.cache.Set(ctx, key, data, 5*time.Minute)
		}
	}

	return ids, nil
}

// ListEditableFieldIDsForUser returns the set of field IDs the user is
// allowed to edit per field-level RBAC (Section 06) — the batched
// counterpart of CheckFieldPermissionForUser's can_edit arm, used by the
// §80 wiki entity-fields schema and set_value validation to avoid one
// permission query per field. Results are cached in Valkey under
// jc:field_perm_edit:{user_id} with a 5-minute TTL, matching
// ListVisibleFieldIDsForUser. When the cache is unavailable or not
// configured, the DB is queried directly.
func (s *Service) ListEditableFieldIDsForUser(ctx context.Context, userID pgtype.UUID) ([]pgtype.UUID, error) {
	if s.cache != nil {
		key := valkey.MustKey("field_perm_edit:%s", userID.String())
		if cached, err := s.cache.Get(ctx, key).Bytes(); err == nil && len(cached) > 0 {
			var ids []string
			if json.Unmarshal(cached, &ids) == nil && len(ids) > 0 {
				result := make([]pgtype.UUID, 0, len(ids))

				for _, idStr := range ids {
					var id pgtype.UUID
					if err := id.Scan(idStr); err == nil {
						result = append(result, id)
					}
				}

				if len(result) > 0 {
					return result, nil
				}
			}
		}
	}

	ids, err := s.read.ListEditableFieldIDsForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("listing editable field ids: %w", err)
	}

	if s.cache != nil && len(ids) > 0 {
		key := valkey.MustKey("field_perm_edit:%s", userID.String())

		idStrs := make([]string, 0, len(ids))
		for _, id := range ids {
			idStrs = append(idStrs, id.String())
		}

		if data, err := json.Marshal(idStrs); err == nil {
			s.cache.Set(ctx, key, data, 5*time.Minute)
		}
	}

	return ids, nil
}

// InvalidateFieldPermissionCache clears the cached visible and editable
// field IDs for a user. Must be called after SetFieldPermission or
// user_group membership changes (BUG-015/BUG-016).
func (s *Service) InvalidateFieldPermissionCache(ctx context.Context, userID pgtype.UUID) {
	if s.cache == nil {
		return
	}

	key := valkey.MustKey("field_perm:%s", userID.String())
	s.cache.Del(ctx, key)

	editKey := valkey.MustKey("field_perm_edit:%s", userID.String())
	s.cache.Del(ctx, editKey)
}

// InvalidateFieldPermissionCacheForGroup clears the cached visible field IDs
// for every member of the given user group. Must be called after
// SetFieldPermission (which upserts a permission for a user_group, not a
// specific user) so all affected users see the change immediately instead of
// waiting for the 5-minute TTL (Rule 6 / valkey-keys-catalog:
// jc:field_perm:{user_id} invalidated by SetFieldPermission).
func (s *Service) InvalidateFieldPermissionCacheForGroup(ctx context.Context, groupID pgtype.UUID) {
	if s.cache == nil {
		return
	}

	memberIDs, err := s.read.ListGroupMembersByGroupID(ctx, groupID)
	if err != nil {
		slog.Warn("customfields: failed to list group members for field-perm cache invalidation",
			"group_id", groupID.String(), "error", err)
		return
	}

	for _, userID := range memberIDs {
		s.InvalidateFieldPermissionCache(ctx, userID)
	}
}

// UpsertFieldValue inserts or updates the stored value for a field on a given
// entity. The conflict target is (field_id, entity_type, entity_id).
func (s *Service) UpsertFieldValue(ctx context.Context, params gen.UpsertCustomFieldValueParams) (gen.CustomFieldValue, error) {
	value, err := s.write.UpsertCustomFieldValue(ctx, params)
	if err != nil {
		return gen.CustomFieldValue{}, fmt.Errorf("upserting custom field value: %w", err)
	}

	return value, nil
}

// DeleteFieldValue removes the stored value for a field on a given entity.
func (s *Service) DeleteFieldValue(ctx context.Context, fieldID pgtype.UUID, entityType string, entityID pgtype.UUID) error {
	if err := s.write.DeleteCustomFieldValue(ctx, gen.DeleteCustomFieldValueParams{
		FieldID:    fieldID,
		EntityType: entityType,
		EntityID:   entityID,
	}); err != nil {
		return fmt.Errorf("deleting custom field value: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Forms
// ---------------------------------------------------------------------------

// ListForms returns every form owned by the given platform, newest first.
func (s *Service) ListForms(ctx context.Context) ([]gen.ListFormsRow, error) {
	forms, err := s.read.ListForms(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing forms: %w", err)
	}

	return forms, nil
}

// ListFormsBySpace returns every form scoped to the given space, with
// submission counts, newest first.
func (s *Service) ListFormsBySpace(ctx context.Context, spaceID pgtype.UUID) ([]gen.ListFormsBySpaceRow, error) {
	forms, err := s.read.ListFormsBySpace(ctx,

		spaceID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing forms by space: %w", err)
	}

	return forms, nil
}

// GetForm returns the form with the given ID, scoped to the platform.
func (s *Service) GetForm(ctx context.Context, id pgtype.UUID) (gen.GetFormByIDRow, error) {
	form, err := s.read.GetFormByID(ctx,
		id,
	)
	if err != nil {
		return gen.GetFormByIDRow{}, fmt.Errorf("getting form: %w", err)
	}

	return form, nil
}

// GetFormBySlug returns the form matching the given slug, scoped to the
// platform.
func (s *Service) GetFormBySlug(ctx context.Context, slug string) (gen.GetFormBySlugRow, error) {
	form, err := s.read.GetFormBySlug(ctx,
		slug,
	)
	if err != nil {
		return gen.GetFormBySlugRow{}, fmt.Errorf("getting form by slug: %w", err)
	}

	return form, nil
}

// CreateForm creates a new form. System forms can only be seeded at install
// time; set IsSystem to false for platform-created forms.
func (s *Service) CreateForm(ctx context.Context, params gen.CreateFormParams) (gen.CreateFormRow, error) {
	form, err := s.write.CreateForm(ctx, params)
	if err != nil {
		return gen.CreateFormRow{}, fmt.Errorf("creating form: %w", err)
	}

	return form, nil
}

// UpdateForm updates a non-system form's mutable fields. System forms are
// protected at the database level.
func (s *Service) UpdateForm(ctx context.Context, params gen.UpdateFormParams) (gen.UpdateFormRow, error) {
	form, err := s.write.UpdateForm(ctx, params)
	if err != nil {
		return gen.UpdateFormRow{}, fmt.Errorf("updating form: %w", err)
	}

	return form, nil
}

// DeleteForm deletes a non-system form, scoped to the platform. System forms
// are protected at the database level.
func (s *Service) DeleteForm(ctx context.Context, id pgtype.UUID) error {
	if err := s.write.DeleteForm(ctx,
		id,
	); err != nil {
		return fmt.Errorf("deleting form: %w", err)
	}

	return nil
}

// FormFieldDef describes a field to be created as part of a form.
type FormFieldDef struct {
	Label     string
	Key       string
	FieldType string
	Required  bool
	Position  int32
	Options   []byte // JSON array, or nil
}

// CreateFormWithFields creates a form, a backing custom field group, the
// requested fields, and the group-to-form assignment — all in one transaction.
// Returns the created form row (compatible with gen.Form fields).
func (s *Service) CreateFormWithFields(ctx context.Context, spaceID pgtype.UUID, name, slug string, description *string, fieldDefs []FormFieldDef) (gen.CreateFormRow, error) {
	return db.WithTxResult(ctx, s.pool, func(q *gen.Queries) (gen.CreateFormRow, error) {
		// 1. Create the form
		form, err := q.CreateForm(ctx, gen.CreateFormParams{

			SpaceID:     spaceID,
			Name:        name,
			Slug:        slug,
			Description: description,
			Status:      "draft",
			IsSystem:    false,
		})
		if err != nil {
			return gen.CreateFormRow{}, fmt.Errorf("creating form: %w", err)
		}

		// 2. Create a backing field group for this form
		groupSlug := "form-" + slug

		group, err := q.CreateCustomFieldGroup(ctx, gen.CreateCustomFieldGroupParams{

			Name:     "Form: " + name,
			Slug:     groupSlug,
			IsSystem: false,
		})
		if err != nil {
			return gen.CreateFormRow{}, fmt.Errorf("creating form field group: %w", err)
		}

		// 3. Create each field in the group
		for _, fd := range fieldDefs {
			_, err := q.CreateCustomField(ctx, gen.CreateCustomFieldParams{
				GroupID: group.ID,

				Label:          fd.Label,
				Key:            fd.Key,
				FieldType:      fd.FieldType,
				IsRequired:     fd.Required,
				IsSystem:       false,
				Position:       fd.Position,
				Options:        fd.Options,
				ShowInFrontend: true,
			})
			if err != nil {
				return gen.CreateFormRow{}, fmt.Errorf("creating form field %q: %w", fd.Label, err)
			}
		}

		// 4. Assign the group to this form (entity_class = "form")
		if _, err := q.CreateAssignment(ctx, gen.CreateAssignmentParams{
			GroupID:     group.ID,
			EntityClass: "form",
			EntityID:    form.ID,
			Position:    0,
		}); err != nil {
			return gen.CreateFormRow{}, fmt.Errorf("assigning form field group: %w", err)
		}

		return form, nil
	})
}

// GetFormFields returns the custom fields attached to a form via the
// custom_field_group_assignments polymorphic link (entity_class = "form").
func (s *Service) GetFormFields(ctx context.Context, formID pgtype.UUID) ([]gen.ListCustomFieldsByGroupRow, error) {
	assignment, err := s.read.GetAssignmentByEntity(ctx, gen.GetAssignmentByEntityParams{
		EntityClass: "form",
		EntityID:    formID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // form has no fields yet
		}

		return nil, fmt.Errorf("getting form field assignment: %w", err)
	}

	fields, err := s.read.ListCustomFieldsByGroup(ctx,
		assignment.GroupID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing form fields: %w", err)
	}

	return fields, nil
}

// GetFormGroupID returns the field group UUID assigned to the given form,
// or nil if no group is assigned yet.
func (s *Service) GetFormGroupID(ctx context.Context, formID pgtype.UUID) *string {
	assignment, err := s.read.GetAssignmentByEntity(ctx, gen.GetAssignmentByEntityParams{
		EntityClass: "form",
		EntityID:    formID,
	})
	if err != nil {
		return nil
	}

	if !assignment.GroupID.Valid {
		return nil
	}

	gid := assignment.GroupID.String()

	return &gid
}

// EnsureFormGroup creates a backing field group and assignment for a form
// if one doesn't already exist. Called by CreateForm to ensure every form
// has a field group for the admin UI to add fields to.
func (s *Service) EnsureFormGroup(ctx context.Context, formID pgtype.UUID, formName, formSlug string) error {
	// Check if assignment already exists
	_, err := s.read.GetAssignmentByEntity(ctx, gen.GetAssignmentByEntityParams{
		EntityClass: "form",
		EntityID:    formID,
	})
	if err == nil {
		return nil // already has a field group
	}

	// Create a backing field group
	groupSlug := "form-" + formSlug

	group, err := s.write.CreateCustomFieldGroup(ctx, gen.CreateCustomFieldGroupParams{

		Name:     "Form: " + formName,
		Slug:     groupSlug,
		IsSystem: false,
	})
	if err != nil {
		return fmt.Errorf("creating form field group: %w", err)
	}

	// Assign the group to this form
	_, err = s.write.CreateAssignment(ctx, gen.CreateAssignmentParams{
		GroupID:     group.ID,
		EntityClass: "form",
		EntityID:    formID,
		Position:    0,
	})
	if err != nil {
		return fmt.Errorf("assigning form field group: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Form Steps
// ---------------------------------------------------------------------------

// ListFormSteps returns every step belonging to the given form, ordered by
// position.
func (s *Service) ListFormSteps(ctx context.Context, formID pgtype.UUID) ([]gen.FormStep, error) {
	steps, err := s.read.ListFormStepsByForm(ctx, formID)
	if err != nil {
		return nil, fmt.Errorf("listing form steps: %w", err)
	}

	return steps, nil
}

// GetStepFields returns the custom fields attached to a form step via the
// custom_field_group_assignments polymorphic link (entity_class = "form_step").
func (s *Service) GetStepFields(ctx context.Context, stepID pgtype.UUID) ([]gen.ListCustomFieldsByGroupRow, error) {
	assignment, err := s.read.GetAssignmentByEntity(ctx, gen.GetAssignmentByEntityParams{
		EntityClass: "form_step",
		EntityID:    stepID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // step has no fields yet
		}

		return nil, fmt.Errorf("getting step field assignment: %w", err)
	}

	fields, err := s.read.ListCustomFieldsByGroup(ctx,
		assignment.GroupID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing step fields: %w", err)
	}

	return fields, nil
}

// GetStepFormGroupID returns the field group UUID assigned to the given step,
// or nil if no group is assigned yet.
func (s *Service) GetStepFormGroupID(ctx context.Context, stepID pgtype.UUID) *string {
	assignment, err := s.read.GetAssignmentByEntity(ctx, gen.GetAssignmentByEntityParams{
		EntityClass: "form_step",
		EntityID:    stepID,
	})
	if err != nil {
		return nil
	}

	if !assignment.GroupID.Valid {
		return nil
	}

	gid := assignment.GroupID.String()

	return &gid
}

// CreateFormStep adds a new step to a multi-step form and provisions a backing
// custom field group + assignment (entity_class="form_step", entity_id=step_id)
// in one transaction. This lets the admin UI add fields directly to a step via
// the existing field-group field CRUD endpoints.
func (s *Service) CreateFormStep(ctx context.Context, formSlug string, params gen.CreateFormStepParams) (gen.FormStep, error) {
	return db.WithTxResult(ctx, s.pool, func(q *gen.Queries) (gen.FormStep, error) {
		step, err := q.CreateFormStep(ctx, params)
		if err != nil {
			return gen.FormStep{}, fmt.Errorf("creating form step: %w", err)
		}

		// Create a backing field group for this step.
		group, err := q.CreateCustomFieldGroup(ctx, gen.CreateCustomFieldGroupParams{

			Name:     "Form step: " + params.Title,
			Slug:     "form-" + formSlug + "-step-" + strconv.Itoa(int(params.Position)),
			IsSystem: false,
		})
		if err != nil {
			return gen.FormStep{}, fmt.Errorf("creating step field group: %w", err)
		}

		// Assign the group to this step (entity_class="form_step").
		_, err = q.CreateAssignment(ctx, gen.CreateAssignmentParams{
			GroupID:     group.ID,
			EntityClass: "form_step",
			EntityID:    step.ID,
			Position:    params.Position,
		})
		if err != nil {
			return gen.FormStep{}, fmt.Errorf("assigning step field group: %w", err)
		}

		return step, nil
	})
}

// UpdateFormStep updates a step's title and position, scoped to the form.
func (s *Service) UpdateFormStep(ctx context.Context, params gen.UpdateFormStepParams) (gen.FormStep, error) {
	step, err := s.write.UpdateFormStep(ctx, params)
	if err != nil {
		return gen.FormStep{}, fmt.Errorf("updating form step: %w", err)
	}

	return step, nil
}

// DeleteFormStep removes a step from a form, identified by both step ID and
// form ID.
func (s *Service) DeleteFormStep(ctx context.Context, id, formID pgtype.UUID) error {
	if err := s.write.DeleteFormStep(ctx, gen.DeleteFormStepParams{
		ID:     id,
		FormID: formID,
	}); err != nil {
		return fmt.Errorf("deleting form step: %w", err)
	}

	return nil
}

// ReorderFormSteps updates the position of each step by ID. The step_ids array
// is in the desired order; position is assigned by index. The form_steps table
// has a UNIQUE(form_id, position) constraint, so a naive in-place update would
// collide when two steps swap positions. The reorder runs inside a single
// transaction in two phases: first bump every position by a large offset
// (guaranteed not to collide with real indices), then assign the final index.
func (s *Service) ReorderFormSteps(ctx context.Context, formID pgtype.UUID, stepIDs []pgtype.UUID) error {
	offset := int32(len(stepIDs) + 1) // #nosec

	return db.WithTx(ctx, s.pool, func(q *gen.Queries) error {
		// Phase 1: move every step to a temporary high position so the final
		// positions are free to claim without colliding with the unique index.
		for i, id := range stepIDs {
			if err := q.UpdateFormStepPosition(ctx, gen.UpdateFormStepPositionParams{
				ID:       id,
				FormID:   formID,
				Position: int32(i) + offset, // #nosec
			}); err != nil {
				return fmt.Errorf("reordering form steps (temp) at index %d: %w", i, err)
			}
		}
		// Phase 2: assign the final positions.
		for i, id := range stepIDs {
			if err := q.UpdateFormStepPosition(ctx, gen.UpdateFormStepPositionParams{
				ID:       id,
				FormID:   formID,
				Position: int32(i), // #nosec
			}); err != nil {
				return fmt.Errorf("reordering form steps (final) at index %d: %w", i, err)
			}
		}

		return nil
	})
}

// ---------------------------------------------------------------------------
// Form Submissions
// ---------------------------------------------------------------------------

// ListFormSubmissions returns a page of submissions for a form, scoped to the
// platform, using offset pagination (Rule 5: newest first).
func (s *Service) ListFormSubmissions(ctx context.Context, params gen.ListFormSubmissionsByFormParams) ([]gen.ListFormSubmissionsByFormRow, error) {
	submissions, err := s.read.ListFormSubmissionsByForm(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("listing form submissions: %w", err)
	}

	return submissions, nil
}

// CountFormSubmissions returns the total count of submissions for a form,
// optionally filtered by status. Used by the offset-pagination response
// envelope (Rule 5) to populate the total count.
func (s *Service) CountFormSubmissions(ctx context.Context, params gen.CountFormSubmissionsByFormParams) (int64, error) {
	count, err := s.read.CountFormSubmissionsByForm(ctx, params)
	if err != nil {
		return 0, fmt.Errorf("counting form submissions: %w", err)
	}

	return count, nil
}

// CreateFormSubmission records a new submission for a form.
func (s *Service) CreateFormSubmission(ctx context.Context, params gen.CreateFormSubmissionParams) (gen.CreateFormSubmissionRow, error) {
	submission, err := s.write.CreateFormSubmission(ctx, params)
	if err != nil {
		return gen.CreateFormSubmissionRow{}, fmt.Errorf("creating form submission: %w", err)
	}

	return submission, nil
}

// GetFormSubmission returns the submission with the given ID, scoped to the
// platform.
func (s *Service) GetFormSubmission(ctx context.Context, id pgtype.UUID) (gen.GetFormSubmissionByIDRow, error) {
	submission, err := s.read.GetFormSubmissionByID(ctx,
		id,
	)
	if err != nil {
		return gen.GetFormSubmissionByIDRow{}, fmt.Errorf("getting form submission: %w", err)
	}

	return submission, nil
}

// UpdateFormSubmissionStatus updates the processing status of a submission,
// scoped to the platform.
func (s *Service) UpdateFormSubmissionStatus(ctx context.Context, id pgtype.UUID, status string) error {
	if err := s.write.UpdateFormSubmissionStatus(ctx, gen.UpdateFormSubmissionStatusParams{
		Status: status,
		ID:     id,
	}); err != nil {
		return fmt.Errorf("updating form submission status: %w", err)
	}

	return nil
}

// DeleteFormSubmission deletes a submission, scoped to the platform.
func (s *Service) DeleteFormSubmission(ctx context.Context, id pgtype.UUID) error {
	if err := s.write.DeleteFormSubmission(ctx,
		id,
	); err != nil {
		return fmt.Errorf("deleting form submission: %w", err)
	}

	return nil
}

// =====================
// Form Submission Triggers (Section 06)
// =====================

// ListFormTriggers returns the triggers configured on a form, ordered by position.
func (s *Service) ListFormTriggers(ctx context.Context, formID pgtype.UUID) ([]gen.FormSubmissionTrigger, error) {
	triggers, err := s.read.ListFormSubmissionTriggersByForm(ctx, formID)
	if err != nil {
		return nil, fmt.Errorf("listing form submission triggers: %w", err)
	}

	return triggers, nil
}

// CreateFormTrigger creates a new trigger on a form.
func (s *Service) CreateFormTrigger(ctx context.Context, params gen.CreateFormSubmissionTriggerParams) (gen.FormSubmissionTrigger, error) {
	trigger, err := s.write.CreateFormSubmissionTrigger(ctx, params)
	if err != nil {
		return gen.FormSubmissionTrigger{}, fmt.Errorf("creating form submission trigger: %w", err)
	}

	return trigger, nil
}

// UpdateFormTrigger updates a trigger's type, config, and position.
func (s *Service) UpdateFormTrigger(ctx context.Context, params gen.UpdateFormSubmissionTriggerParams) (gen.FormSubmissionTrigger, error) {
	trigger, err := s.write.UpdateFormSubmissionTrigger(ctx, params)
	if err != nil {
		return gen.FormSubmissionTrigger{}, fmt.Errorf("updating form submission trigger: %w", err)
	}

	return trigger, nil
}

// DeleteFormTrigger deletes a trigger from a form.
func (s *Service) DeleteFormTrigger(ctx context.Context, id, formID pgtype.UUID) error {
	if err := s.write.DeleteFormSubmissionTrigger(ctx, gen.DeleteFormSubmissionTriggerParams{
		ID:     id,
		FormID: formID,
	}); err != nil {
		return fmt.Errorf("deleting form submission trigger: %w", err)
	}

	return nil
}

// ReorderFormTriggers updates the position of each trigger by ID. The
// trigger_ids array is in the desired order; position is assigned by index.
func (s *Service) ReorderFormTriggers(ctx context.Context, formID pgtype.UUID, triggerIDs []pgtype.UUID) error {
	for i, id := range triggerIDs {
		if err := s.write.UpdateFormSubmissionTriggerPosition(ctx, gen.UpdateFormSubmissionTriggerPositionParams{
			ID:       id,
			FormID:   formID,
			Position: int32(i), // #nosec
		}); err != nil {
			return fmt.Errorf("reordering form submission triggers at index %d: %w", i, err)
		}
	}

	return nil
}

// =====================
// Custom Field Entity Types (Section 06)
// =====================

// ListEntityTypes returns all entity types for a platform, ordered by sort_order.
func (s *Service) ListEntityTypes(ctx context.Context) ([]gen.CustomFieldEntityType, error) {
	types, err := s.read.ListCustomFieldEntityTypes(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing custom field entity types: %w", err)
	}

	return types, nil
}

// CreateEntityType creates a new custom field entity type for a platform.
func (s *Service) CreateEntityType(ctx context.Context, params gen.CreateCustomFieldEntityTypeParams) (gen.CustomFieldEntityType, error) {
	et, err := s.write.CreateCustomFieldEntityType(ctx, params)
	if err != nil {
		return gen.CustomFieldEntityType{}, fmt.Errorf("creating custom field entity type: %w", err)
	}

	return et, nil
}

// UpdateEntityType updates the label and sort_order of an entity type.
func (s *Service) UpdateEntityType(ctx context.Context, params gen.UpdateCustomFieldEntityTypeParams) (gen.CustomFieldEntityType, error) {
	et, err := s.write.UpdateCustomFieldEntityType(ctx, params)
	if err != nil {
		return gen.CustomFieldEntityType{}, fmt.Errorf("updating custom field entity type: %w", err)
	}

	return et, nil
}

// DeleteEntityType deletes a non-system entity type. System types (is_system=true)
// are protected at the SQL layer and will return 0 rows affected.
func (s *Service) DeleteEntityType(ctx context.Context, id pgtype.UUID) error {
	rows, err := s.write.DeleteCustomFieldEntityType(ctx,
		id,
	)
	if err != nil {
		return fmt.Errorf("deleting custom field entity type: %w", err)
	}

	if rows == 0 {
		return errors.New("entity type not found or is a system type that cannot be deleted")
	}

	return nil
}
