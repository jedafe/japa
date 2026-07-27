package customfields

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// ---------------------------------------------------------------------------
// Custom Field Groups
// ---------------------------------------------------------------------------

// ListFieldGroups returns every custom field group owned by the given tenant,
// newest first.
func (s *Service) ListFieldGroups(ctx context.Context, tenantID pgtype.UUID) ([]gen.CustomFieldGroup, error) {
	groups, err := s.read.ListCustomFieldGroupsByTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing custom field groups: %w", err)
	}

	return groups, nil
}

// ListFieldGroupsByEntityClass returns custom field groups owned by the given
// tenant that are assigned to the given entity class (e.g. "form", "space_type"),
// newest first.
func (s *Service) ListFieldGroupsByEntityClass(ctx context.Context, tenantID pgtype.UUID, entityClass string) ([]gen.CustomFieldGroup, error) {
	groups, err := s.read.ListCustomFieldGroupsByTenantAndEntityClass(ctx, gen.ListCustomFieldGroupsByTenantAndEntityClassParams{
		TenantID:    tenantID,
		EntityClass: entityClass,
	})
	if err != nil {
		return nil, fmt.Errorf("listing custom field groups by entity class: %w", err)
	}

	return groups, nil
}

// GetFieldGroup returns the custom field group with the given ID, scoped to
// the tenant.
func (s *Service) GetFieldGroup(ctx context.Context, id, tenantID pgtype.UUID) (gen.CustomFieldGroup, error) {
	group, err := s.read.GetCustomFieldGroupByID(ctx, gen.GetCustomFieldGroupByIDParams{
		ID:       id,
		TenantID: tenantID,
	})
	if err != nil {
		return gen.CustomFieldGroup{}, fmt.Errorf("getting custom field group: %w", err)
	}

	return group, nil
}

// GetFieldGroupBySlug returns the custom field group matching the given slug,
// scoped to the tenant.
func (s *Service) GetFieldGroupBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (gen.CustomFieldGroup, error) {
	group, err := s.read.GetCustomFieldGroupBySlug(ctx, gen.GetCustomFieldGroupBySlugParams{
		Slug:     slug,
		TenantID: tenantID,
	})
	if err != nil {
		return gen.CustomFieldGroup{}, fmt.Errorf("getting custom field group by slug: %w", err)
	}

	return group, nil
}

// CreateFieldGroup creates a new custom field group. System groups can only be
// seeded at install time; set isSystem to false for tenant-created groups.
func (s *Service) CreateFieldGroup(ctx context.Context, tenantID pgtype.UUID, name, slug string, description *string, isSystem bool) (gen.CustomFieldGroup, error) {
	group, err := s.write.CreateCustomFieldGroup(ctx, gen.CreateCustomFieldGroupParams{
		TenantID:    tenantID,
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
// group, scoped to the tenant.
func (s *Service) UpdateFieldGroup(ctx context.Context, id, tenantID pgtype.UUID, name string, description *string) (gen.CustomFieldGroup, error) {
	group, err := s.write.UpdateCustomFieldGroup(ctx, gen.UpdateCustomFieldGroupParams{
		Name:        name,
		Description: description,
		ID:          id,
		TenantID:    tenantID,
	})
	if err != nil {
		return gen.CustomFieldGroup{}, fmt.Errorf("updating custom field group: %w", err)
	}

	return group, nil
}

// DeleteFieldGroup deletes a non-system custom field group, scoped to the
// tenant. System groups are protected at the database level.
func (s *Service) DeleteFieldGroup(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.DeleteCustomFieldGroup(ctx, gen.DeleteCustomFieldGroupParams{
		ID:       id,
		TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("deleting custom field group: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Custom Fields
// ---------------------------------------------------------------------------

// ListFieldsByGroup returns every custom field belonging to the given group,
// ordered by position then creation time.
func (s *Service) ListFieldsByGroup(ctx context.Context, groupID, tenantID pgtype.UUID) ([]gen.CustomField, error) {
	fields, err := s.read.ListCustomFieldsByGroup(ctx, gen.ListCustomFieldsByGroupParams{
		GroupID:  groupID,
		TenantID: tenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("listing custom fields by group: %w", err)
	}

	return fields, nil
}

// GetField returns the custom field with the given ID, scoped to the tenant.
func (s *Service) GetField(ctx context.Context, id, tenantID pgtype.UUID) (gen.CustomField, error) {
	field, err := s.read.GetCustomFieldByID(ctx, gen.GetCustomFieldByIDParams{
		ID:       id,
		TenantID: tenantID,
	})
	if err != nil {
		return gen.CustomField{}, fmt.Errorf("getting custom field: %w", err)
	}

	return field, nil
}

// CreateField creates a new custom field. The caller is responsible for
// validating the field type via IsValidFieldType before calling this method.
func (s *Service) CreateField(ctx context.Context, params gen.CreateCustomFieldParams) (gen.CustomField, error) {
	field, err := s.write.CreateCustomField(ctx, params)
	if err != nil {
		return gen.CustomField{}, fmt.Errorf("creating custom field: %w", err)
	}

	return field, nil
}

// UpdateField updates a non-system custom field. System fields are protected
// at the database level.
func (s *Service) UpdateField(ctx context.Context, params gen.UpdateCustomFieldParams) (gen.CustomField, error) {
	field, err := s.write.UpdateCustomField(ctx, params)
	if err != nil {
		return gen.CustomField{}, fmt.Errorf("updating custom field: %w", err)
	}

	return field, nil
}

// DeleteField deletes a non-system custom field, scoped to the tenant. System
// fields are protected at the database level.
func (s *Service) DeleteField(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.DeleteCustomField(ctx, gen.DeleteCustomFieldParams{
		ID:       id,
		TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("deleting custom field: %w", err)
	}

	return nil
}

// UpdateFieldPosition updates a single field's position within its group,
// scoped to the tenant. Used by ReorderFields to persist a bulk reorder as a
// sequence of single-field updates, mirroring the comparison package's
// ReorderComparisonItem.
func (s *Service) UpdateFieldPosition(ctx context.Context, id, groupID, tenantID pgtype.UUID, position int32) error {
	if err := s.write.UpdateCustomFieldPosition(ctx, gen.UpdateCustomFieldPositionParams{
		Position: position,
		ID:       id,
		GroupID:  groupID,
		TenantID: tenantID,
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
func (s *Service) ListFieldValuesByEntityForUser(ctx context.Context, entityType string, entityID pgtype.UUID, visibleFieldIDs []pgtype.UUID) ([]gen.CustomFieldValue, error) {
	if len(visibleFieldIDs) == 0 {
		return []gen.CustomFieldValue{}, nil
	}

	values, err := s.read.ListCustomFieldValuesByEntity(ctx, gen.ListCustomFieldValuesByEntityParams{
		EntityType: entityType,
		EntityID:   entityID,
	})
	if err != nil {
		return nil, fmt.Errorf("listing custom field values by entity: %w", err)
	}

	visibleSet := make(map[pgtype.UUID]struct{}, len(visibleFieldIDs))
	for _, id := range visibleFieldIDs {
		visibleSet[id] = struct{}{}
	}

	filtered := make([]gen.CustomFieldValue, 0, len(values))
	for _, v := range values {
		if _, ok := visibleSet[v.FieldID]; ok {
			filtered = append(filtered, v)
		}
	}

	return filtered, nil
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
// to view per field-level RBAC (Section 06).
func (s *Service) ListVisibleFieldIDsForUser(ctx context.Context, userID pgtype.UUID) ([]pgtype.UUID, error) {
	ids, err := s.read.ListVisibleFieldIDsForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("listing visible field ids: %w", err)
	}
	return ids, nil
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

// ListForms returns every form owned by the given tenant, newest first.
func (s *Service) ListForms(ctx context.Context, tenantID pgtype.UUID) ([]gen.ListFormsByTenantRow, error) {
	forms, err := s.read.ListFormsByTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing forms: %w", err)
	}

	return forms, nil
}

// ListFormsBySpace returns every form scoped to the given space, with
// submission counts, newest first.
func (s *Service) ListFormsBySpace(ctx context.Context, tenantID, spaceID pgtype.UUID) ([]gen.ListFormsBySpaceRow, error) {
	forms, err := s.read.ListFormsBySpace(ctx, gen.ListFormsBySpaceParams{
		TenantID: tenantID,
		SpaceID:  spaceID,
	})
	if err != nil {
		return nil, fmt.Errorf("listing forms by space: %w", err)
	}

	return forms, nil
}

// GetForm returns the form with the given ID, scoped to the tenant.
func (s *Service) GetForm(ctx context.Context, id, tenantID pgtype.UUID) (gen.GetFormByIDRow, error) {
	form, err := s.read.GetFormByID(ctx, gen.GetFormByIDParams{
		ID:       id,
		TenantID: tenantID,
	})
	if err != nil {
		return gen.GetFormByIDRow{}, fmt.Errorf("getting form: %w", err)
	}

	return form, nil
}

// GetFormBySlug returns the form matching the given slug, scoped to the
// tenant.
func (s *Service) GetFormBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (gen.GetFormBySlugRow, error) {
	form, err := s.read.GetFormBySlug(ctx, gen.GetFormBySlugParams{
		Slug:     slug,
		TenantID: tenantID,
	})
	if err != nil {
		return gen.GetFormBySlugRow{}, fmt.Errorf("getting form by slug: %w", err)
	}

	return form, nil
}

// CreateForm creates a new form. System forms can only be seeded at install
// time; set IsSystem to false for tenant-created forms.
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

// DeleteForm deletes a non-system form, scoped to the tenant. System forms
// are protected at the database level.
func (s *Service) DeleteForm(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.DeleteForm(ctx, gen.DeleteFormParams{
		ID:       id,
		TenantID: tenantID,
	}); err != nil {
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
func (s *Service) CreateFormWithFields(ctx context.Context, tenantID, spaceID pgtype.UUID, name, slug string, description *string, fieldDefs []FormFieldDef) (gen.CreateFormRow, error) {
	return db.WithTxResult(ctx, s.pool, func(q *gen.Queries) (gen.CreateFormRow, error) {
		// 1. Create the form
		form, err := q.CreateForm(ctx, gen.CreateFormParams{
			TenantID:    tenantID,
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
			TenantID: tenantID,
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
				GroupID:        group.ID,
				TenantID:       tenantID,
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
func (s *Service) GetFormFields(ctx context.Context, formID, tenantID pgtype.UUID) ([]gen.CustomField, error) {
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

	fields, err := s.read.ListCustomFieldsByGroup(ctx, gen.ListCustomFieldsByGroupParams{
		GroupID:  assignment.GroupID,
		TenantID: tenantID,
	})
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
func (s *Service) EnsureFormGroup(ctx context.Context, tenantID, formID pgtype.UUID, formName, formSlug string) error {
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
		TenantID: tenantID,
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

// CreateFormStep adds a new step to a multi-step form.
func (s *Service) CreateFormStep(ctx context.Context, params gen.CreateFormStepParams) (gen.FormStep, error) {
	step, err := s.write.CreateFormStep(ctx, params)
	if err != nil {
		return gen.FormStep{}, fmt.Errorf("creating form step: %w", err)
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

// ---------------------------------------------------------------------------
// Form Submissions
// ---------------------------------------------------------------------------

// ListFormSubmissions returns a page of submissions for a form, scoped to the
// tenant, using cursor-based pagination (newest first).
func (s *Service) ListFormSubmissions(ctx context.Context, params gen.ListFormSubmissionsByFormParams) ([]gen.FormSubmission, error) {
	submissions, err := s.read.ListFormSubmissionsByForm(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("listing form submissions: %w", err)
	}

	return submissions, nil
}

// CreateFormSubmission records a new submission for a form.
func (s *Service) CreateFormSubmission(ctx context.Context, params gen.CreateFormSubmissionParams) (gen.FormSubmission, error) {
	submission, err := s.write.CreateFormSubmission(ctx, params)
	if err != nil {
		return gen.FormSubmission{}, fmt.Errorf("creating form submission: %w", err)
	}

	return submission, nil
}

// GetFormSubmission returns the submission with the given ID, scoped to the
// tenant.
func (s *Service) GetFormSubmission(ctx context.Context, id, tenantID pgtype.UUID) (gen.FormSubmission, error) {
	submission, err := s.read.GetFormSubmissionByID(ctx, gen.GetFormSubmissionByIDParams{
		ID:       id,
		TenantID: tenantID,
	})
	if err != nil {
		return gen.FormSubmission{}, fmt.Errorf("getting form submission: %w", err)
	}

	return submission, nil
}

// UpdateFormSubmissionStatus updates the processing status of a submission,
// scoped to the tenant.
func (s *Service) UpdateFormSubmissionStatus(ctx context.Context, id, tenantID pgtype.UUID, status string) error {
	if err := s.write.UpdateFormSubmissionStatus(ctx, gen.UpdateFormSubmissionStatusParams{
		Status:   status,
		ID:       id,
		TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("updating form submission status: %w", err)
	}

	return nil
}

// DeleteFormSubmission deletes a submission, scoped to the tenant.
func (s *Service) DeleteFormSubmission(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.DeleteFormSubmission(ctx, gen.DeleteFormSubmissionParams{
		ID:       id,
		TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("deleting form submission: %w", err)
	}

	return nil
}
