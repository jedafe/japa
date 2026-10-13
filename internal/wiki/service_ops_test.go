package wiki

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/comparison/resolver"
	"github.com/jedafe/japa/internal/customfields"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// ---------------------------------------------------------------------------
// Fakes for the service's injected collaborators
// ---------------------------------------------------------------------------

// fakeFieldReader is a fieldReadStore + fieldGroupProvisioner stub. Field
// lookups hit fieldsByKey; per-field permissions come from perms keyed by
// field ID string — [canView, canEdit] — surfaced through the batched list
// methods the service actually calls. The provisioner methods are an
// in-memory get-or-create store keyed by group slug.
type fakeFieldReader struct {
	fieldsByKey        map[string]gen.GetCustomFieldByKeyRow
	perms              map[string][2]bool // fieldID.String() → [canView, canEdit]
	permErr            error              // injected failure for the batch lookups
	assignments        []gen.CustomFieldGroupAssignment
	assignmentsOnRetry []gen.CustomFieldGroupAssignment // returned by the second+ list call
	assignErr          error                            // injected CreateAssignment failure (provisioning retry path)
	listAssignCalls    int
	groups             map[string]gen.CustomFieldGroup
	groupFields        map[string][]gen.ListCustomFieldsByGroupRow
	values             []gen.ListCustomFieldValuesWithDefsByEntityRow
	byKeyCalls         []string
	createdGroups      []gen.CustomFieldGroup
	createdAssigns     []gen.CreateAssignmentParams
	nextProvisionUUID  int
}

func (f *fakeFieldReader) permSet(wantEdit bool) []pgtype.UUID {
	out := make([]pgtype.UUID, 0, len(f.perms))

	for idStr, p := range f.perms {
		allowed := p[0]
		if wantEdit {
			allowed = p[1]
		}

		if !allowed {
			continue
		}

		var id pgtype.UUID
		if err := id.Scan(idStr); err == nil {
			out = append(out, id)
		}
	}

	return out
}

func (f *fakeFieldReader) ListVisibleFieldIDsForUser(_ context.Context, _ pgtype.UUID) ([]pgtype.UUID, error) {
	if f.permErr != nil {
		return nil, f.permErr
	}

	return f.permSet(false), nil
}

func (f *fakeFieldReader) ListVisibleFieldIDsForGroupSlug(_ context.Context, _ string) ([]pgtype.UUID, error) {
	if f.permErr != nil {
		return nil, f.permErr
	}

	return f.permSet(false), nil
}

func (f *fakeFieldReader) ListEditableFieldIDsForUser(_ context.Context, _ pgtype.UUID) ([]pgtype.UUID, error) {
	if f.permErr != nil {
		return nil, f.permErr
	}

	return f.permSet(true), nil
}

func (f *fakeFieldReader) GetCustomFieldByKey(_ context.Context, key string) (gen.GetCustomFieldByKeyRow, error) {
	f.byKeyCalls = append(f.byKeyCalls, key)

	if row, ok := f.fieldsByKey[key]; ok {
		return row, nil
	}

	return gen.GetCustomFieldByKeyRow{}, pgx.ErrNoRows
}

func (f *fakeFieldReader) ListAssignmentsByEntityClass(_ context.Context, _ string) ([]gen.CustomFieldGroupAssignment, error) {
	f.listAssignCalls++
	// Provisioning retry simulation: return a different set after the first
	// call so a "concurrent commit" can materialize between the reads.
	if f.listAssignCalls > 1 && f.assignmentsOnRetry != nil {
		return f.assignmentsOnRetry, nil
	}

	return f.assignments, nil
}

func (f *fakeFieldReader) ListFieldValuesWithDefsForEntities(_ context.Context, _ string, _ []pgtype.UUID) ([]gen.ListCustomFieldValuesWithDefsForEntitiesRow, error) {
	return nil, nil
}

func (f *fakeFieldReader) ListFieldValuesWithDefsByEntity(_ context.Context, _ string, _ pgtype.UUID) ([]gen.ListCustomFieldValuesWithDefsByEntityRow, error) {
	return f.values, nil
}

func (f *fakeFieldReader) GetFieldGroup(_ context.Context, id pgtype.UUID) (gen.CustomFieldGroup, error) {
	if g, ok := f.groups[id.String()]; ok {
		return g, nil
	}

	return gen.CustomFieldGroup{}, pgx.ErrNoRows
}

func (f *fakeFieldReader) ListFieldsByGroup(_ context.Context, groupID pgtype.UUID) ([]gen.ListCustomFieldsByGroupRow, error) {
	return f.groupFields[groupID.String()], nil
}

// fieldGroupProvisioner side — slug-keyed get-or-create.

func (f *fakeFieldReader) GetFieldGroupBySlug(_ context.Context, slug string) (gen.CustomFieldGroup, error) {
	for _, g := range f.groups {
		if g.Slug == slug {
			return g, nil
		}
	}

	return gen.CustomFieldGroup{}, pgx.ErrNoRows
}

func (f *fakeFieldReader) CreateFieldGroup(_ context.Context, name, slug string, description *string, isSystem bool) (gen.CustomFieldGroup, error) {
	f.nextProvisionUUID++

	var id pgtype.UUID
	if err := id.Scan(fmt.Sprintf("00000000-0000-0000-0000-%012x", f.nextProvisionUUID)); err != nil {
		return gen.CustomFieldGroup{}, err
	}

	g := gen.CustomFieldGroup{ID: id, Name: name, Slug: slug, Description: description, IsSystem: isSystem}
	if f.groups == nil {
		f.groups = map[string]gen.CustomFieldGroup{}
	}
	f.groups[id.String()] = g
	f.createdGroups = append(f.createdGroups, g)

	return g, nil
}

func (f *fakeFieldReader) CreateAssignment(_ context.Context, p gen.CreateAssignmentParams) (gen.CustomFieldGroupAssignment, error) {
	if f.assignErr != nil {
		return gen.CustomFieldGroupAssignment{}, f.assignErr
	}

	a := gen.CustomFieldGroupAssignment{
		GroupID:     p.GroupID,
		EntityClass: p.EntityClass,
		EntityID:    p.EntityID,
		Position:    p.Position,
	}
	f.assignments = append(f.assignments, a)
	f.createdAssigns = append(f.createdAssigns, p)

	return a, nil
}

// fakePermChecker stubs the RBAC resolver; granted maps node → allowed.
type fakePermChecker struct{ granted map[string]bool }

func (f fakePermChecker) HasPermission(_ context.Context, _ pgtype.UUID, key string) bool {
	return f.granted[key]
}

// fakeEntityResolver stubs the JCE item_ref resolver registry.
type fakeEntityResolver struct {
	item   resolver.ResolvedItem
	err    error
	gotRef map[string]any
}

func (f *fakeEntityResolver) Resolve(_ context.Context, ref map[string]any) (resolver.ResolvedItem, error) {
	f.gotRef = ref

	return f.item, f.err
}

// fakeEnqueuer records Asynq enqueues.
type fakeEnqueuer struct {
	calls []enqueuedTask
	err   error
}

type enqueuedTask struct {
	taskType string
	payload  []byte
}

func (f *fakeEnqueuer) Enqueue(taskType string, payload []byte, _ ...interface{}) error {
	f.calls = append(f.calls, enqueuedTask{taskType: taskType, payload: payload})

	return f.err
}

// fakeBuster records BustCachesForItemRef calls.
type fakeBuster struct {
	calls [][2]string
	err   error
}

func (f *fakeBuster) BustCachesForItemRef(_ context.Context, entityType, entityID string) error {
	f.calls = append(f.calls, [2]string{entityType, entityID})

	return f.err
}

// ---------------------------------------------------------------------------
// Unit tests — rejection paths that never reach the store (s.store is nil)
// ---------------------------------------------------------------------------

// TestSubmitFieldOps_RejectsWhenCanEditFalse: a set_value on an existing
// field whose custom_field_permissions deny can_edit to the submitter is
// rejected before a revision row is created (§6 submit validation).
func TestSubmitFieldOps_RejectsWhenCanEditFalse(t *testing.T) {
	t.Parallel()

	groupID := mustUUID(t, "00000000-0000-0000-0000-0000000000a1")
	fieldID := mustUUID(t, "00000000-0000-0000-0000-0000000000a2")
	entityID := mustUUID(t, "00000000-0000-0000-0000-0000000000bb")

	svc := NewService(nil, nil)
	svc.SetPermissionChecker(fakePermChecker{granted: map[string]bool{PermFieldsEdit: true}})
	svc.SetCustomFields(&fakeFieldReader{
		// The field is reachable only through the entity's assigned-group
		// schema — a bare fieldsByKey entry no longer satisfies set_value.
		assignments: []gen.CustomFieldGroupAssignment{{GroupID: groupID, EntityClass: "space_type"}},
		groupFields: map[string][]gen.ListCustomFieldsByGroupRow{
			groupID.String(): {{ID: fieldID, Key: "tuition_ug", FieldType: customfields.FieldTypeNumber, ShowInFrontend: true}},
		},
		perms: map[string][2]bool{fieldID.String(): {true, false}}, // can_view yes, can_edit no
	})

	ops := []FieldOp{{
		Op:         OpSetValue,
		EntityType: "university",
		EntityID:   entityID.String(),
		FieldKey:   "tuition_ug",
		Value:      json.RawMessage(`12000`),
	}}

	_, _, err := svc.SubmitFieldOps(t.Context(), mustUUID(t, "00000000-0000-0000-0000-0000000000c1"),
		pgtype.UUID{}, "university", entityID, nil, ops, "", false)
	if !errors.Is(err, ErrOpNotPermitted) {
		t.Fatalf("expected ErrOpNotPermitted, got %v", err)
	}
}

// TestSubmitFieldOps_PerOpPermissionMapping: each op kind is gated by its
// wiki.fields.* node — a submission containing an op the submitter lacks is
// rejected even when other ops are permitted (§10).
func TestSubmitFieldOps_PerOpPermissionMapping(t *testing.T) {
	t.Parallel()

	userID := mustUUID(t, "00000000-0000-0000-0000-0000000000c1")
	entityID := mustUUID(t, "00000000-0000-0000-0000-0000000000bb")

	cases := []struct {
		name      string
		op        FieldOp
		grants    map[string]bool
		canManage bool
		wantErr   bool
	}{
		{
			name: "set_value needs fields.edit",
			op: FieldOp{
				Op: OpSetValue, EntityType: "university", EntityID: entityID.String(),
				FieldKey: "k", Value: json.RawMessage(`"v"`),
			},
			grants:  map[string]bool{PermFieldsPropose: true},
			wantErr: true,
		},
		{
			name: "propose_field needs fields.propose",
			op: FieldOp{
				Op: OpProposeField, GroupSlug: "g",
				Field: &FieldDefInput{Key: "k", Label: "K", FieldType: customfields.FieldTypeText},
			},
			grants:  map[string]bool{PermFieldsEdit: true},
			wantErr: true,
		},
		{
			name: "upsert_field needs fields.manage",
			op: FieldOp{
				Op: OpUpsertField, GroupSlug: "g",
				Field: &FieldDefInput{Key: "k", Label: "K", FieldType: customfields.FieldTypeText},
			},
			grants:  map[string]bool{PermFieldsEdit: true, PermFieldsPropose: true},
			wantErr: true,
		},
		{
			name:    "assign_group needs fields.manage",
			op:      FieldOp{Op: OpAssignGroup, GroupSlug: "g", EntityClass: "organization"},
			grants:  map[string]bool{PermFieldsEdit: true, PermFieldsPropose: true},
			wantErr: true,
		},
		{
			name:      "manage-tier submission skips per-op checks",
			op:        FieldOp{Op: OpAssignGroup, GroupSlug: "g", EntityClass: "organization"},
			grants:    nil, // no nodes granted — canManage covers everything
			canManage: true,
			wantErr:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			svc := NewService(nil, nil)
			svc.SetPermissionChecker(fakePermChecker{granted: tc.grants})

			if !tc.wantErr {
				// The store is nil, so the full submit path can't run without
				// a DB — assert the gate itself passes for the manage tier.
				// The end-to-end manage path runs against the real store in
				// TestSubmitFieldOps_ManageTierAutoApproves below.
				if err := svc.checkOpsPermissions(t.Context(), userID, []FieldOp{tc.op}, tc.canManage); err != nil {
					t.Fatalf("manage tier should skip per-op checks: %v", err)
				}

				return
			}

			_, _, err := svc.SubmitFieldOps(t.Context(), userID, pgtype.UUID{}, "university", entityID, nil, []FieldOp{tc.op}, "", false)
			if !errors.Is(err, ErrOpNotPermitted) {
				t.Fatalf("expected ErrOpNotPermitted, got %v", err)
			}
		})
	}
}

// TestSubmitFieldOps_SetValueOnProposedKeySkipsLookup: a set_value whose
// field_key is created by an earlier propose op in the same array skips the
// field-existence and can_edit checks (§6 — new fields have no permission
// rows yet).
func TestSubmitFieldOps_SetValueOnProposedKeySkipsLookup(t *testing.T) {
	t.Parallel()

	entityID := mustUUID(t, "00000000-0000-0000-0000-0000000000bb")
	fields := &fakeFieldReader{} // GetCustomFieldByKey always ErrNoRows

	svc := NewService(nil, nil)
	svc.SetPermissionChecker(fakePermChecker{granted: map[string]bool{
		PermFieldsEdit: true, PermFieldsPropose: true,
	}})
	svc.SetCustomFields(fields)

	ops := []FieldOp{
		{
			Op: OpProposeField, GroupSlug: "g", EntityType: "university",
			Field: &FieldDefInput{Key: "visa_success_rate", Label: "Visa Success Rate", FieldType: customfields.FieldTypePercentage},
		},
		{
			Op: OpSetValue, EntityType: "university", EntityID: entityID.String(),
			FieldKey: "visa_success_rate", Value: json.RawMessage(`0.82`),
		},
	}

	// checkSetValueFields must pass without ever consulting the field store
	// for the proposed key.
	if err := svc.checkSetValueFields(t.Context(), mustUUID(t, "00000000-0000-0000-0000-0000000000c1"), ops, false, nil); err != nil {
		t.Fatalf("set_value on proposed key should validate: %v", err)
	}

	for _, k := range fields.byKeyCalls {
		if k == "visa_success_rate" {
			t.Fatal("proposed field key must not hit GetCustomFieldByKey")
		}
	}
}

// TestSubmitFieldOps_MaxOpsBound: submissions over
// wiki.suggest.max_ops_per_revision are rejected structurally.
func TestSubmitFieldOps_MaxOpsBound(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil) // nil settings → default 50

	ops := make([]FieldOp, 51)
	for i := range ops {
		ops[i] = FieldOp{Op: OpAssignGroup, GroupSlug: "g", EntityClass: "organization"}
	}

	_, _, err := svc.SubmitFieldOps(t.Context(), mustUUID(t, "00000000-0000-0000-0000-0000000000c1"),
		pgtype.UUID{}, "", pgtype.UUID{}, nil, ops, "", true)
	if err == nil || !strings.Contains(err.Error(), "exceeds maximum") {
		t.Fatalf("expected ops bound error, got %v", err)
	}
}

// TestSuggestFields_ResolverFailure: an unresolvable entity rejects the
// submission before any revision work.
func TestSuggestFields_ResolverFailure(t *testing.T) {
	t.Parallel()

	svc := NewService(nil, nil)
	svc.SetEntityResolver(&fakeEntityResolver{err: errors.New("university item not found")})

	entityID := mustUUID(t, "00000000-0000-0000-0000-0000000000bb")

	_, _, err := svc.SuggestFields(t.Context(), mustUUID(t, "00000000-0000-0000-0000-0000000000c1"),
		"university", entityID, []FieldOp{{Op: OpAssignGroup, GroupSlug: "g", EntityClass: "organization"}}, "", true)
	if !errors.Is(err, ErrEntityNotResolvable) {
		t.Fatalf("expected ErrEntityNotResolvable, got %v", err)
	}
}

// TestEntityClassesFor covers the §8.1 entity_type → entity_class mapping.
// Space-backed entities resolve the full seeded convention — per-type slug,
// per-type id, and the class-wide token — while other types map to a single
// class.
func TestEntityClassesFor(t *testing.T) {
	t.Parallel()

	spaceTypeID := mustUUID(t, "00000000-0000-0000-0000-0000000000d1")

	cases := []struct {
		entityType    string
		spaceTypeID   pgtype.UUID
		spaceTypeSlug string
		want          []string
	}{
		{"university", spaceTypeID, "university", []string{"space_type:university", "space_type:" + spaceTypeID.String(), "space_type"}},
		{"space", spaceTypeID, "country", []string{"space_type:country", "space_type:" + spaceTypeID.String(), "space_type"}},
		{"country", pgtype.UUID{}, "", []string{"space_type"}},
		{"organization", pgtype.UUID{}, "", []string{"organization"}},
		{"bank", pgtype.UUID{}, "", []string{"organization"}},
		{"content", pgtype.UUID{}, "", []string{"content_type"}},
		{"content:guide", pgtype.UUID{}, "", []string{"content_type:guide"}},
		{"guide_category:visa", pgtype.UUID{}, "", []string{"content_type:guide:visa"}},
		{"visa_route", pgtype.UUID{}, "", []string{"jce_entity:visa_route"}},
		{"scholarship", pgtype.UUID{}, "", []string{"jce_entity:scholarship"}},
	}

	for _, tc := range cases {
		got := entityClassesFor(tc.entityType, tc.spaceTypeID, tc.spaceTypeSlug)
		if len(got) != len(tc.want) {
			t.Fatalf("entityClassesFor(%q) = %v, want %v", tc.entityType, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("entityClassesFor(%q) = %v, want %v", tc.entityType, got, tc.want)
			}
		}
	}
}

// TestValueWithinOptions covers select/multiselect membership validation.
func TestValueWithinOptions(t *testing.T) {
	t.Parallel()

	options := json.RawMessage(`[{"value":"fall","label":"Fall"},{"value":"spring","label":"Spring"}]`)
	sel := schemaField{key: "intake", fieldType: customfields.FieldTypeSelect, options: options}
	multi := schemaField{key: "intakes", fieldType: customfields.FieldTypeMultiselect, options: options}
	text := schemaField{key: "name", fieldType: customfields.FieldTypeText}

	if err := valueWithinOptions(sel, json.RawMessage(`"fall"`)); err != nil {
		t.Fatalf("valid select option rejected: %v", err)
	}

	if err := valueWithinOptions(sel, json.RawMessage(`"winter"`)); err == nil {
		t.Fatal("invalid select option accepted")
	}

	if err := valueWithinOptions(multi, json.RawMessage(`["fall","spring"]`)); err != nil {
		t.Fatalf("valid multiselect rejected: %v", err)
	}

	if err := valueWithinOptions(multi, json.RawMessage(`["fall","winter"]`)); err == nil {
		t.Fatal("multiselect with invalid member accepted")
	}

	if err := valueWithinOptions(text, json.RawMessage(`"anything"`)); err != nil {
		t.Fatalf("non-select field should skip options check: %v", err)
	}

	// Fields with no options configured impose no membership constraint.
	noOpts := schemaField{key: "s", fieldType: customfields.FieldTypeSelect}
	if err := valueWithinOptions(noOpts, json.RawMessage(`"whatever"`)); err != nil {
		t.Fatalf("options-less select should pass: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Integration tests — need JC_DATABASE_URL (same pattern as store_ops_test.go)
// ---------------------------------------------------------------------------

// fieldFixture seeds a custom field group (and optionally one field) for the
// ops integration tests, with cascade cleanup via the group row.
type fieldFixture struct {
	groupID pgtype.UUID
	fieldID pgtype.UUID
	slug    string
	key     string
}

// assignFixtureGroup links a fixture group to an entity class so the
// entity's field schema (entitySchemaFields) resolves its fields — field
// keys are group-scoped, so set_value ops cannot reach an unassigned group.
func assignFixtureGroup(t *testing.T, ctx context.Context, database *db.DB, groupID pgtype.UUID, entityClass string) {
	t.Helper()

	if _, err := database.Writer.Exec(ctx,
		`INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
		 VALUES ($1, $2, NULL, 0)`, groupID, entityClass); err != nil {
		t.Fatalf("seed assignment: %v", err)
	}
}

func seedFieldFixture(t *testing.T, ctx context.Context, database *db.DB, withField bool, fieldType string) fieldFixture {
	t.Helper()

	uniq := strings.ReplaceAll(genUUID(t, ctx, database), "-", "")[:12]

	var fx fieldFixture
	fx.slug = "wop_" + uniq
	fx.key = "wop_" + uniq

	var gid string
	if err := database.Writer.QueryRow(ctx,
		`INSERT INTO custom_field_groups (name, slug, is_system) VALUES ($1, $2, false) RETURNING id`,
		"Wiki Ops Test "+uniq, fx.slug).Scan(&gid); err != nil {
		t.Fatalf("seed field group: %v", err)
	}

	fx.groupID = mustUUID(t, gid)

	if withField {
		var fid string
		if err := database.Writer.QueryRow(ctx,
			`INSERT INTO custom_fields (group_id, label, key, field_type, is_system, show_in_frontend)
			 VALUES ($1, $2, $3, $4, false, true) RETURNING id`,
			fx.groupID, "Test Field "+uniq, fx.key, fieldType).Scan(&fid); err != nil {
			t.Fatalf("seed field: %v", err)
		}

		fx.fieldID = mustUUID(t, fid)
	}

	t.Cleanup(func() {
		database.Writer.Exec(ctx, `DELETE FROM custom_field_groups WHERE id = $1`, fx.groupID)
	})

	return fx
}

// TestSubmitFieldOps_ManageTierAutoApproves covers the §6 step-2 manage-tier
// path: revision row kept, status flipped to approved, ops applied inside the
// same transaction (cfv row written under normalized entity_type 'space'),
// and wiki:fields_propagate enqueued after commit.
func TestSubmitFieldOps_ManageTierAutoApproves(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	fx := seedOpsFixture(t, ctx, database)
	ff := seedFieldFixture(t, ctx, database, true, customfields.FieldTypeNumber)

	spaceTypeID, _, err := store.GetSpaceTypeRef(ctx, fx.spaceID)
	if err != nil {
		t.Fatalf("GetSpaceTypeRef: %v", err)
	}

	assignFixtureGroup(t, ctx, database, ff.groupID, "space_type:"+spaceTypeID.String())

	enq := &fakeEnqueuer{}
	buster := &fakeBuster{}

	svc := NewService(store, nil)
	svc.SetCustomFields(customfields.NewService(database))
	svc.SetEnqueuer(enq)
	svc.SetJCECacheBuster(buster)

	ops := []FieldOp{{
		Op:         OpSetValue,
		EntityType: "university",
		EntityID:   fx.spaceID.String(),
		FieldKey:   ff.key,
		Value:      json.RawMessage(`12000`),
	}}

	revID, autoApproved, err := svc.SubmitFieldOps(ctx, fx.userID, pgtype.UUID{}, "university", fx.spaceID, nil, ops, "set tuition", true)
	if err != nil {
		t.Fatalf("SubmitFieldOps: %v", err)
	}

	if !autoApproved {
		t.Fatal("expected autoApproved=true for manage tier")
	}

	rev, err := store.GetRevision(ctx, mustUUID(t, revID))
	if err != nil {
		t.Fatalf("GetRevision: %v", err)
	}

	if rev.Status != "approved" || rev.Kind != "fields" {
		t.Fatalf("expected approved fields revision, got status=%q kind=%q", rev.Status, rev.Kind)
	}

	// cfv row: entity_type normalized 'university' → 'space' (§5).
	var value float64
	if err := database.Reader.QueryRow(ctx,
		`SELECT value_number::float8 FROM custom_field_values WHERE field_id = $1 AND entity_type = 'space' AND entity_id = $2`,
		ff.fieldID, fx.spaceID).Scan(&value); err != nil {
		t.Fatalf("expected cfv row after auto-approve: %v", err)
	}

	if value != 12000 {
		t.Fatalf("cfv value: got %v want 12000", value)
	}

	// Propagation went through the enqueuer, not the direct buster.
	if len(enq.calls) != 1 || enq.calls[0].taskType != TaskFieldsPropagate {
		t.Fatalf("expected one %s enqueue, got %#v", TaskFieldsPropagate, enq.calls)
	}

	var payload map[string]string
	if err := json.Unmarshal(enq.calls[0].payload, &payload); err != nil {
		t.Fatalf("propagate payload not JSON: %v", err)
	}

	if payload["entity_type"] != "university" || payload["entity_id"] != fx.spaceID.String() {
		t.Fatalf("propagate payload: %#v", payload)
	}

	if len(buster.calls) != 0 {
		t.Fatalf("buster must not run when enqueue succeeds: %#v", buster.calls)
	}
}

// TestSuggestFields_Pageless covers POST /wiki/suggest for an entity with no
// wiki page: the revision is page-less (NULL wiki_page_id) but still carries
// the entity target.
func TestSuggestFields_Pageless(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	fx := seedOpsFixture(t, ctx, database)

	groupID := mustUUID(t, "00000000-0000-0000-0000-0000000000a1")
	fieldID := mustUUID(t, "00000000-0000-0000-0000-0000000000a2")

	svc := NewService(store, nil)
	svc.SetPermissionChecker(fakePermChecker{granted: map[string]bool{PermFieldsEdit: true}})
	svc.SetCustomFields(&fakeFieldReader{
		// visa_route resolves to entity_class jce_entity:visa_route; the
		// schema map needs the assignment so set_value can find the field.
		assignments: []gen.CustomFieldGroupAssignment{{GroupID: groupID, EntityClass: "jce_entity:visa_route"}},
		groupFields: map[string][]gen.ListCustomFieldsByGroupRow{
			groupID.String(): {{ID: fieldID, Key: "tuition_ug", FieldType: customfields.FieldTypeNumber, ShowInFrontend: true}},
		},
		perms: map[string][2]bool{fieldID.String(): {true, true}},
	})
	// Resolver returns an item with no geo_space_id → no host page.
	svc.SetEntityResolver(&fakeEntityResolver{item: resolver.ResolvedItem{
		Type:  "visa_route",
		ID:    fx.target.String(),
		Title: "Test route",
	}})

	ops := []FieldOp{{
		Op:         OpSetValue,
		EntityType: "visa_route",
		EntityID:   fx.target.String(),
		FieldKey:   "tuition_ug",
		Value:      json.RawMessage(`55`),
	}}

	revID, autoApproved, err := svc.SuggestFields(ctx, fx.userID, "visa_route", fx.target, ops, "update fee", false)
	if err != nil {
		t.Fatalf("SuggestFields: %v", err)
	}

	if autoApproved {
		t.Fatal("non-manage submission must stay pending")
	}

	rev, err := store.GetRevision(ctx, mustUUID(t, revID))
	if err != nil {
		t.Fatalf("GetRevision: %v", err)
	}

	if rev.WikiPageID.Valid {
		t.Fatalf("expected page-less revision, got wiki_page_id %s", rev.WikiPageID.String())
	}

	if rev.TargetEntityType == nil || *rev.TargetEntityType != "visa_route" {
		t.Fatalf("target_entity_type: %v", rev.TargetEntityType)
	}

	if rev.TargetEntityID.String() != fx.target.String() {
		t.Fatalf("target_entity_id: %s want %s", rev.TargetEntityID.String(), fx.target.String())
	}

	if rev.Status != "pending" || rev.Kind != "fields" {
		t.Fatalf("expected pending fields revision, got %s/%s", rev.Status, rev.Kind)
	}
}

// TestSuggestFields_AttachesHostPage: when the resolved item carries
// geo_space_id, the revision attaches to that space's overview page.
func TestSuggestFields_AttachesHostPage(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	fx := seedOpsFixture(t, ctx, database) // seeds an 'overview' page on fx.spaceID

	groupID := mustUUID(t, "00000000-0000-0000-0000-0000000000a1")

	svc := NewService(store, nil)
	svc.SetPermissionChecker(fakePermChecker{granted: map[string]bool{PermFieldsPropose: true}})
	svc.SetCustomFields(&fakeFieldReader{
		// propose_field's group must exist at submit — a revision that can
		// never apply is rejected instead of queuing.
		groups: map[string]gen.CustomFieldGroup{groupID.String(): {ID: groupID, Slug: "g"}},
	})
	svc.SetEntityResolver(&fakeEntityResolver{item: resolver.ResolvedItem{
		Type:     "university",
		ID:       fx.spaceID.String(),
		Metadata: map[string]any{"geo_space_id": fx.spaceID.String()},
	}})

	ops := []FieldOp{{
		Op: OpProposeField, GroupSlug: "g", EntityType: "university",
		Field: &FieldDefInput{Key: "k", Label: "K", FieldType: customfields.FieldTypeText},
	}}

	revID, _, err := svc.SuggestFields(ctx, fx.userID, "university", fx.spaceID, ops, "propose field", false)
	if err != nil {
		t.Fatalf("SuggestFields: %v", err)
	}

	rev, err := store.GetRevision(ctx, mustUUID(t, revID))
	if err != nil {
		t.Fatalf("GetRevision: %v", err)
	}

	if !rev.WikiPageID.Valid || rev.WikiPageID != fx.pageID {
		t.Fatalf("expected host page %s, got %s", fx.pageID, rev.WikiPageID)
	}
}

// TestApproveRevisionByID_AppliesOpsAndEnqueues covers the reviewer path for
// a pending fields revision: ops apply inside the approve transaction and
// propagation fires after commit.
func TestApproveRevisionByID_AppliesOpsAndEnqueues(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	fx := seedOpsFixture(t, ctx, database)
	ff := seedFieldFixture(t, ctx, database, true, customfields.FieldTypeText)

	spaceTypeID, _, err := store.GetSpaceTypeRef(ctx, fx.spaceID)
	if err != nil {
		t.Fatalf("GetSpaceTypeRef: %v", err)
	}

	assignFixtureGroup(t, ctx, database, ff.groupID, "space_type:"+spaceTypeID.String())

	targetType := "university"
	opsJSON := []byte(`[{"op":"set_value","entity_type":"university","entity_id":"` + fx.spaceID.String() +
		`","field_key":"` + ff.key + `","value":"fall intake"}]`)

	revID, err := store.SubmitRevision(ctx, pgtype.UUID{}, fx.userID, "fields", nil, opsJSON, "set intake", &targetType, fx.spaceID)
	if err != nil {
		t.Fatalf("SubmitRevision: %v", err)
	}

	enq := &fakeEnqueuer{}
	buster := &fakeBuster{}

	svc := NewService(store, nil)
	svc.SetEnqueuer(enq)
	svc.SetJCECacheBuster(buster)

	if err := svc.ApproveRevisionByID(ctx, revID, fx.userID, "looks good"); err != nil {
		t.Fatalf("ApproveRevisionByID: %v", err)
	}

	// Ops applied: cfv row written under normalized 'space' entity type.
	var text string
	if err := database.Reader.QueryRow(ctx,
		`SELECT value_text FROM custom_field_values WHERE field_id = $1 AND entity_type = 'space' AND entity_id = $2`,
		ff.fieldID, fx.spaceID).Scan(&text); err != nil {
		t.Fatalf("expected cfv row after approve: %v", err)
	}

	if text != "fall intake" {
		t.Fatalf("cfv value_text: got %q", text)
	}

	if len(enq.calls) != 1 || enq.calls[0].taskType != TaskFieldsPropagate {
		t.Fatalf("expected one %s enqueue, got %#v", TaskFieldsPropagate, enq.calls)
	}

	if len(buster.calls) != 0 {
		t.Fatalf("direct bust must not run when enqueued: %#v", buster.calls)
	}

	// Approving again fails — revision is no longer pending.
	if err := svc.ApproveRevisionByID(ctx, revID, fx.userID, ""); err == nil {
		t.Fatal("expected error re-approving a non-pending revision")
	}
}

// TestApproveRevisionByID_DirectBustFallback: when no Asynq enqueuer is
// wired, propagation falls back to a synchronous cache bust so correctness
// never depends on worker wiring order.
func TestApproveRevisionByID_DirectBustFallback(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	fx := seedOpsFixture(t, ctx, database)
	ff := seedFieldFixture(t, ctx, database, true, customfields.FieldTypeText)

	spaceTypeID, _, err := store.GetSpaceTypeRef(ctx, fx.spaceID)
	if err != nil {
		t.Fatalf("GetSpaceTypeRef: %v", err)
	}

	assignFixtureGroup(t, ctx, database, ff.groupID, "space_type:"+spaceTypeID.String())

	targetType := "university"
	opsJSON := []byte(`[{"op":"set_value","entity_type":"university","entity_id":"` + fx.spaceID.String() +
		`","field_key":"` + ff.key + `","value":"direct"}]`)

	revID, err := store.SubmitRevision(ctx, pgtype.UUID{}, fx.userID, "fields", nil, opsJSON, "set", &targetType, fx.spaceID)
	if err != nil {
		t.Fatalf("SubmitRevision: %v", err)
	}

	buster := &fakeBuster{}

	svc := NewService(store, nil) // no enqueuer
	svc.SetJCECacheBuster(buster)

	if err := svc.ApproveRevisionByID(ctx, revID, fx.userID, ""); err != nil {
		t.Fatalf("ApproveRevisionByID: %v", err)
	}

	if len(buster.calls) != 1 || buster.calls[0] != [2]string{"university", fx.spaceID.String()} {
		t.Fatalf("expected direct bust for university %s, got %#v", fx.spaceID.String(), buster.calls)
	}
}

// TestApproveRevisionByID_SchemaOpsPropagate: propose/assign revisions apply
// AND enqueue wiki:fields_propagate — new fields/dims change the matrix
// shape, so JCE freshness must bust alongside value-affecting ops.
func TestApproveRevisionByID_SchemaOpsPropagate(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	fx := seedOpsFixture(t, ctx, database)

	targetType := "visa_route"
	groupSlug := "wop_" + strings.ReplaceAll(genUUID(t, ctx, database), "-", "")[:12]
	opsJSON := []byte(`[{"op":"propose_group","entity_class":"jce_entity:visa_route","group":{"slug":"` +
		groupSlug + `","name":"Visa Outcomes","fields":[{"key":"wop_rate","label":"Rate","field_type":"percentage"}]}}]`)

	revID, err := store.SubmitRevision(ctx, pgtype.UUID{}, fx.userID, "fields", nil, opsJSON, "new group", &targetType, fx.target)
	if err != nil {
		t.Fatalf("SubmitRevision: %v", err)
	}

	t.Cleanup(func() {
		database.Writer.Exec(ctx, `DELETE FROM custom_field_groups WHERE slug = $1`, groupSlug)
	})

	enq := &fakeEnqueuer{}
	buster := &fakeBuster{}

	svc := NewService(store, nil)
	svc.SetEnqueuer(enq)
	svc.SetJCECacheBuster(buster)

	if err := svc.ApproveRevisionByID(ctx, revID, fx.userID, ""); err != nil {
		t.Fatalf("ApproveRevisionByID: %v", err)
	}

	// The group + field + assignment landed.
	var gid string
	if err := database.Reader.QueryRow(ctx,
		`SELECT id FROM custom_field_groups WHERE slug = $1`, groupSlug).Scan(&gid); err != nil {
		t.Fatalf("proposed group not created: %v", err)
	}

	var assigned bool
	if err := database.Reader.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM custom_field_group_assignments WHERE group_id = $1 AND entity_class = 'jce_entity:visa_route')`,
		mustUUID(t, gid)).Scan(&assigned); err != nil || !assigned {
		t.Fatalf("group assignment missing: assigned=%v err=%v", assigned, err)
	}

	// The enqueuer is wired, so propagation enqueues (the sync buster is
	// the no-enqueuer fallback and stays untouched).
	if len(enq.calls) != 1 || enq.calls[0].taskType != TaskFieldsPropagate || len(buster.calls) != 0 {
		t.Fatalf("schema ops must propagate freshness: enq=%#v bust=%#v", enq.calls, buster.calls)
	}
}

// TestApproveRevisionByID_ContentRevision: plain content revisions approve
// through the same revision-keyed path without touching the ops machinery.
func TestApproveRevisionByID_ContentRevision(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	fx := seedOpsFixture(t, ctx, database)

	revID, err := store.SubmitRevision(ctx, fx.pageID, fx.userID, "content",
		[]byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"hello"}]}]}`),
		nil, "content edit", nil, pgtype.UUID{})
	if err != nil {
		t.Fatalf("SubmitRevision: %v", err)
	}

	enq := &fakeEnqueuer{}
	svc := NewService(store, nil)
	svc.SetEnqueuer(enq)

	if err := svc.ApproveRevisionByID(ctx, revID, fx.userID, ""); err != nil {
		t.Fatalf("ApproveRevisionByID content: %v", err)
	}

	rev, err := store.GetRevision(ctx, revID)
	if err != nil {
		t.Fatalf("GetRevision: %v", err)
	}

	if rev.Status != "approved" {
		t.Fatalf("expected approved, got %s", rev.Status)
	}

	if len(enq.calls) != 0 {
		t.Fatalf("content approval must not enqueue propagation: %#v", enq.calls)
	}
}

// TestGetEntityFields covers the §7 schema read: group assignments resolve
// via entity_class, stored values hydrate the fields, and manage-tier users
// see fields as editable.
func TestGetEntityFields(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	fx := seedOpsFixture(t, ctx, database)
	ff := seedFieldFixture(t, ctx, database, true, customfields.FieldTypeText)

	spaceTypeID, _, err := store.GetSpaceTypeRef(ctx, fx.spaceID)
	if err != nil {
		t.Fatalf("GetSpaceTypeRef: %v", err)
	}

	// Assign the group to this space's type class and store a value.
	if _, err := database.Writer.Exec(ctx,
		`INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
		 VALUES ($1, $2, NULL, 0)`, ff.groupID, "space_type:"+spaceTypeID.String()); err != nil {
		t.Fatalf("seed assignment: %v", err)
	}

	if _, err := database.Writer.Exec(ctx,
		`INSERT INTO custom_field_values (field_id, entity_type, entity_id, value_text)
		 VALUES ($1, 'space', $2, 'fall intake')`, ff.fieldID, fx.spaceID); err != nil {
		t.Fatalf("seed cfv: %v", err)
	}

	svc := NewService(store, nil)
	svc.SetCustomFields(customfields.NewService(database))
	// Manage-tier user → fields reported editable despite no permission rows.
	svc.SetPermissionChecker(fakePermChecker{granted: map[string]bool{PermFieldsManage: true}})

	dto, err := svc.GetEntityFields(ctx, fx.userID, "university", fx.spaceID)
	if err != nil {
		t.Fatalf("GetEntityFields: %v", err)
	}

	// The seeded DB may resolve additional class-wide groups (space_fields)
	// or per-type groups — locate the fixture group rather than asserting a
	// total count.
	var g EntityFieldGroupDTO
	found := false

	for _, grp := range dto.Groups {
		if grp.ID == ff.groupID.String() {
			g = grp
			found = true
			break
		}
	}

	if !found || g.Slug != ff.slug {
		t.Fatalf("fixture group missing from schema: %#v", dto.Groups)
	}

	if len(g.Fields) != 1 || g.Fields[0].Key != ff.key {
		t.Fatalf("fields wrong: %#v", g.Fields)
	}

	f := g.Fields[0]
	if string(f.Value) != `"fall intake"` {
		t.Fatalf("field value: got %s want %q", f.Value, `"fall intake"`)
	}

	if !f.CanEdit || !f.Visible {
		t.Fatalf("manage-tier user should see field editable+visible: %#v", f)
	}
}

// TestGetEntityFields_BatchedFieldPerms covers the §06 field-level RBAC path
// for a non-manage user: visibility and editability come from the batched
// ListVisible/ListEditable lookups (not per-field queries), a view-only
// field reports can_edit=false, and an invisible field's stored value is
// withheld while the schema entry remains.
func TestGetEntityFields_BatchedFieldPerms(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	fx := seedOpsFixture(t, ctx, database)

	spaceTypeID, _, err := store.GetSpaceTypeRef(ctx, fx.spaceID)
	if err != nil {
		t.Fatalf("GetSpaceTypeRef: %v", err)
	}

	groupID := mustUUID(t, genUUID(t, ctx, database))
	entityClass := "space_type:" + spaceTypeID.String()

	fieldA := gen.ListCustomFieldsByGroupRow{ID: mustUUID(t, genUUID(t, ctx, database)), Key: "a", Label: "A", FieldType: customfields.FieldTypeText, ShowInFrontend: true}
	fieldB := gen.ListCustomFieldsByGroupRow{ID: mustUUID(t, genUUID(t, ctx, database)), Key: "b", Label: "B", FieldType: customfields.FieldTypeText, ShowInFrontend: true}
	fieldC := gen.ListCustomFieldsByGroupRow{ID: mustUUID(t, genUUID(t, ctx, database)), Key: "c", Label: "C", FieldType: customfields.FieldTypeText, ShowInFrontend: true}

	hidden := "secret"
	fake := &fakeFieldReader{
		assignments: []gen.CustomFieldGroupAssignment{{GroupID: groupID, EntityClass: entityClass}},
		groups:      map[string]gen.CustomFieldGroup{groupID.String(): {ID: groupID, Slug: "g", Name: "G"}},
		groupFields: map[string][]gen.ListCustomFieldsByGroupRow{groupID.String(): {fieldA, fieldB, fieldC}},
		values: []gen.ListCustomFieldValuesWithDefsByEntityRow{
			{FieldID: fieldC.ID, FieldType: customfields.FieldTypeText, ValueText: &hidden},
		},
		perms: map[string][2]bool{
			fieldA.ID.String(): {true, true},
			fieldB.ID.String(): {true, false},
			// fieldC has no permission rows → invisible and uneditable.
		},
	}

	svc := NewService(store, nil)
	svc.SetCustomFields(fake)
	svc.SetPermissionChecker(fakePermChecker{granted: map[string]bool{}}) // no manage tier

	dto, err := svc.GetEntityFields(ctx, fx.userID, "university", fx.spaceID)
	if err != nil {
		t.Fatalf("GetEntityFields: %v", err)
	}

	if len(dto.Groups) != 1 || len(dto.Groups[0].Fields) != 3 {
		t.Fatalf("expected 1 group with 3 fields, got %#v", dto.Groups)
	}

	byKey := make(map[string]EntityFieldDTO, 3)
	for _, f := range dto.Groups[0].Fields {
		byKey[f.Key] = f
	}

	if !byKey["a"].Visible || !byKey["a"].CanEdit {
		t.Fatalf("field a should be visible+editable: %#v", byKey["a"])
	}

	if !byKey["b"].Visible || byKey["b"].CanEdit {
		t.Fatalf("field b should be view-only: %#v", byKey["b"])
	}

	if byKey["c"].Visible || byKey["c"].CanEdit {
		t.Fatalf("field c should be invisible+uneditable: %#v", byKey["c"])
	}

	if byKey["c"].Value != nil {
		t.Fatalf("invisible field must not leak its value, got %s", byKey["c"].Value)
	}
}

// TestGetEntityFields_PermLookupFailsClosed covers the degraded path: when
// the batched permission lookups error, every field reports
// visible=false/can_edit=false and stored values are withheld rather than
// leaked on the public route.
func TestGetEntityFields_PermLookupFailsClosed(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	fx := seedOpsFixture(t, ctx, database)

	spaceTypeID, _, err := store.GetSpaceTypeRef(ctx, fx.spaceID)
	if err != nil {
		t.Fatalf("GetSpaceTypeRef: %v", err)
	}

	groupID := mustUUID(t, genUUID(t, ctx, database))
	fieldID := mustUUID(t, genUUID(t, ctx, database))

	val := "v"
	fake := &fakeFieldReader{
		permErr:     errors.New("perm store down"),
		assignments: []gen.CustomFieldGroupAssignment{{GroupID: groupID, EntityClass: "space_type:" + spaceTypeID.String()}},
		groups:      map[string]gen.CustomFieldGroup{groupID.String(): {ID: groupID, Slug: "g", Name: "G"}},
		groupFields: map[string][]gen.ListCustomFieldsByGroupRow{groupID.String(): {
			{ID: fieldID, Key: "k", Label: "K", FieldType: customfields.FieldTypeText, ShowInFrontend: true},
		}},
		values: []gen.ListCustomFieldValuesWithDefsByEntityRow{
			{FieldID: fieldID, FieldType: customfields.FieldTypeText, ValueText: &val},
		},
	}

	svc := NewService(store, nil)
	svc.SetCustomFields(fake)
	svc.SetPermissionChecker(fakePermChecker{granted: map[string]bool{}})

	dto, err := svc.GetEntityFields(ctx, fx.userID, "university", fx.spaceID)
	if err != nil {
		t.Fatalf("GetEntityFields: %v", err)
	}

	f := dto.Groups[0].Fields[0]
	if f.Visible || f.CanEdit || f.Value != nil {
		t.Fatalf("perm lookup failure must fail closed: %#v", f)
	}
}

// TestSubmitFieldOps_MixedRevision covers the §80 mixed kind: a submission
// carrying prose AND ops stores one revision with kind='mixed', and approval
// applies the ops transactionally while the prose becomes the page's current
// content — no separate queue entries.
func TestSubmitFieldOps_MixedRevision(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	fx := seedOpsFixture(t, ctx, database)
	ff := seedFieldFixture(t, ctx, database, true, customfields.FieldTypeText)

	// Submit-time schema resolves through s.fields (the fake); approve-time
	// schema re-resolves inside the transaction against the real DB — both
	// need to see the field under the entity's assigned groups.
	spaceTypeID, _, err := store.GetSpaceTypeRef(ctx, fx.spaceID)
	if err != nil {
		t.Fatalf("GetSpaceTypeRef: %v", err)
	}

	assignFixtureGroup(t, ctx, database, ff.groupID, "space_type:"+spaceTypeID.String())

	body := []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"new prose"}]}]}`)

	svc := NewService(store, nil)
	svc.SetPermissionChecker(fakePermChecker{granted: map[string]bool{PermFieldsEdit: true}})
	svc.SetCustomFields(&fakeFieldReader{
		assignments: []gen.CustomFieldGroupAssignment{{GroupID: ff.groupID, EntityClass: "space_type"}},
		groupFields: map[string][]gen.ListCustomFieldsByGroupRow{
			ff.groupID.String(): {{ID: ff.fieldID, Key: ff.key, FieldType: customfields.FieldTypeText, ShowInFrontend: true}},
		},
		perms: map[string][2]bool{ff.fieldID.String(): {true, true}},
	})

	ops := []FieldOp{{
		Op:         OpSetValue,
		EntityType: "university",
		EntityID:   fx.spaceID.String(),
		FieldKey:   ff.key,
		Value:      json.RawMessage(`"mixed value"`),
	}}

	revID, autoApproved, err := svc.SubmitFieldOps(ctx, fx.userID, fx.pageID, "", pgtype.UUID{}, body, ops, "prose + fields", false)
	if err != nil {
		t.Fatalf("SubmitFieldOps: %v", err)
	}

	if autoApproved {
		t.Fatal("non-manage mixed submission must stay pending")
	}

	rev, err := store.GetRevision(ctx, mustUUID(t, revID))
	if err != nil {
		t.Fatalf("GetRevision: %v", err)
	}

	if rev.Kind != "mixed" {
		t.Fatalf("expected kind=mixed, got %q", rev.Kind)
	}

	if !strings.Contains(string(rev.Content), "new prose") {
		t.Fatalf("mixed revision lost its prose: %s", rev.Content)
	}

	// Approve: ops apply and the prose becomes the page's current content.
	if err := svc.ApproveRevisionByID(ctx, mustUUID(t, revID), fx.userID, "ok"); err != nil {
		t.Fatalf("ApproveRevisionByID: %v", err)
	}

	approved, err := store.GetApprovedRevision(ctx, fx.pageID)
	if err != nil {
		t.Fatalf("GetApprovedRevision: %v", err)
	}

	if !strings.Contains(string(approved.Content), "new prose") {
		t.Fatalf("approved mixed revision prose missing: %s", approved.Content)
	}

	var text string
	if err := database.Reader.QueryRow(ctx,
		`SELECT value_text FROM custom_field_values WHERE field_id = $1 AND entity_type = 'space' AND entity_id = $2`,
		ff.fieldID, fx.spaceID).Scan(&text); err != nil {
		t.Fatalf("expected cfv row after mixed approve: %v", err)
	}

	if text != "mixed value" {
		t.Fatalf("cfv value_text: got %q", text)
	}
}

// TestApproveRevisionByID_ApplyFailureRecordsApplyError covers the §80
// failure surface: when an op cannot be applied at approve time (its field
// was deleted between submit and approve), the transaction rolls back, the
// revision stays pending, and apply_error records the reason for reviewers.
// A later successful approve clears the recorded error.
func TestApproveRevisionByID_ApplyFailureRecordsApplyError(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	fx := seedOpsFixture(t, ctx, database)
	ff := seedFieldFixture(t, ctx, database, true, customfields.FieldTypeText)

	spaceTypeID, _, err := store.GetSpaceTypeRef(ctx, fx.spaceID)
	if err != nil {
		t.Fatalf("GetSpaceTypeRef: %v", err)
	}

	assignFixtureGroup(t, ctx, database, ff.groupID, "space_type:"+spaceTypeID.String())

	targetType := "university"
	opsJSON := []byte(`[{"op":"set_value","entity_type":"university","entity_id":"` + fx.spaceID.String() +
		`","field_key":"` + ff.key + `","value":"x"}]`)

	revID, err := store.SubmitRevision(ctx, pgtype.UUID{}, fx.userID, "fields", nil, opsJSON, "set", &targetType, fx.spaceID)
	if err != nil {
		t.Fatalf("SubmitRevision: %v", err)
	}

	// Delete the field after submit so applyOps fails at approve time.
	if _, err := database.Writer.Exec(ctx, `DELETE FROM custom_fields WHERE id = $1`, ff.fieldID); err != nil {
		t.Fatalf("delete field: %v", err)
	}

	svc := NewService(store, nil)

	if err := svc.ApproveRevisionByID(ctx, revID, fx.userID, ""); err == nil {
		t.Fatal("expected approve to fail when the target field is gone")
	}

	rev, err := store.GetRevision(ctx, revID)
	if err != nil {
		t.Fatalf("GetRevision: %v", err)
	}

	if rev.Status != "pending" {
		t.Fatalf("revision must stay pending after apply failure, got %q", rev.Status)
	}

	if rev.ApplyError == nil || *rev.ApplyError == "" {
		t.Fatal("apply_error must record the failure reason")
	}

	// Restore the field and re-approve — the recorded error clears on success.
	if _, err := database.Writer.Exec(ctx,
		`INSERT INTO custom_fields (id, group_id, label, key, field_type, is_system, show_in_frontend)
		 VALUES ($1, $2, $3, $4, 'text', false, true)`,
		ff.fieldID, ff.groupID, "Restored", ff.key); err != nil {
		t.Fatalf("restore field: %v", err)
	}

	if err := svc.ApproveRevisionByID(ctx, revID, fx.userID, "retry"); err != nil {
		t.Fatalf("retry approve after field restore: %v", err)
	}

	rev, err = store.GetRevision(ctx, revID)
	if err != nil {
		t.Fatalf("GetRevision: %v", err)
	}

	if rev.ApplyError != nil {
		t.Fatalf("apply_error must clear on successful approve, got %q", *rev.ApplyError)
	}
}
