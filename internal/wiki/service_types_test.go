package wiki

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/customfields"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// ---------------------------------------------------------------------------
// Unit tests — validation paths that reject before touching the store
// (s.store is nil for these; a store call would panic, proving the guard
// fired first).
// ---------------------------------------------------------------------------

func TestResolveSpaceTypeID_RejectsBadInput(t *testing.T) {
	svc := &Service{}

	if _, err := svc.resolveSpaceTypeID(t.Context(), "", ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("empty id+slug: want ErrInvalidInput, got %v", err)
	}
	if _, err := svc.resolveSpaceTypeID(t.Context(), "not-a-uuid", ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("bad uuid: want ErrInvalidInput, got %v", err)
	}
}

func TestResolveParentIDs_RejectsBadInput(t *testing.T) {
	svc := &Service{}
	self := mustUUID(t, genUUIDText(t))

	if _, err := svc.resolveParentIDs(t.Context(), self, []string{"bogus"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("bad parent uuid: want ErrInvalidInput, got %v", err)
	}
	if _, err := svc.resolveParentIDs(t.Context(), self, []string{self.String()}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("self-parent: want ErrInvalidInput, got %v", err)
	}
}

func TestUpsertWikiType_RejectsMissingTypeAndSelfParent(t *testing.T) {
	svc := &Service{}

	if _, err := svc.UpsertWikiType(t.Context(), UpsertWikiTypeRequest{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("no id/slug: want ErrInvalidInput, got %v", err)
	}

	self := genUUIDText(t)
	if _, err := svc.UpsertWikiType(t.Context(), UpsertWikiTypeRequest{
		SpaceTypeID:   self,
		ParentTypeIDs: []string{self},
	}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("self-parent: want ErrInvalidInput, got %v", err)
	}
}

func TestUpdateDeleteWikiType_RejectBadID(t *testing.T) {
	svc := &Service{}

	if err := svc.UpdateWikiType(t.Context(), "bogus", UpdateWikiTypeRequest{}); !errors.Is(err, ErrWikiTypeNotFound) {
		t.Fatalf("update: want ErrWikiTypeNotFound, got %v", err)
	}
	if err := svc.DeleteWikiType(t.Context(), "bogus"); !errors.Is(err, ErrWikiTypeNotFound) {
		t.Fatalf("delete: want ErrWikiTypeNotFound, got %v", err)
	}
}

func TestUpdateDeleteWikiCategory_RejectBadID(t *testing.T) {
	svc := &Service{}

	if err := svc.UpdateWikiTypeCategory(t.Context(), "bogus", UpdateWikiCategoryRequest{}); !errors.Is(err, ErrWikiCategoryNotFound) {
		t.Fatalf("update: want ErrWikiCategoryNotFound, got %v", err)
	}
	if err := svc.DeleteWikiTypeCategory(t.Context(), "bogus"); !errors.Is(err, ErrWikiCategoryNotFound) {
		t.Fatalf("delete: want ErrWikiCategoryNotFound, got %v", err)
	}
}

func TestUpdateWikiTypeCategory_RejectEmptyLabel(t *testing.T) {
	svc := &Service{}

	blank := "   "
	if err := svc.UpdateWikiTypeCategory(t.Context(), genUUIDText(t), UpdateWikiCategoryRequest{Label: &blank}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("blank label: want ErrInvalidInput, got %v", err)
	}
}

func TestCreateWikiTypeCategory_RejectsBadTypeID(t *testing.T) {
	svc := &Service{}

	if _, err := svc.CreateWikiTypeCategory(t.Context(), "bogus", CreateWikiCategoryRequest{Slug: "x", Label: "X"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("bad type id: want ErrInvalidInput, got %v", err)
	}
}

func TestValidateFieldGroup(t *testing.T) {
	groupID := mustUUID(t, genUUIDText(t))

	fields := &fakeFieldReader{
		groups: map[string]gen.CustomFieldGroup{
			groupID.String(): {ID: groupID, Slug: "cost_of_living", Name: "Cost of Living"},
		},
	}
	svc := &Service{fields: fields}

	// Empty is rejected — the binding is mandatory; category create routes an
	// omitted id through provisionCategoryGroup instead.
	if _, err := svc.validateFieldGroup(t.Context(), ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("empty: want ErrInvalidInput, got %v", err)
	}

	if _, err := svc.validateFieldGroup(t.Context(), "not-a-uuid"); !errors.Is(err, ErrFieldGroupNotFound) {
		t.Fatalf("bad uuid: want ErrFieldGroupNotFound, got %v", err)
	}
	if _, err := svc.validateFieldGroup(t.Context(), genUUIDText(t)); !errors.Is(err, ErrFieldGroupNotFound) {
		t.Fatalf("unknown group: want ErrFieldGroupNotFound, got %v", err)
	}

	id, err := svc.validateFieldGroup(t.Context(), groupID.String())
	if err != nil || id != groupID {
		t.Fatalf("known group: want %v, got %v / %v", groupID, id, err)
	}

	// Without a fields service the id is accepted as-is (FK still guards).
	svcNoFields := &Service{}
	if _, err := svcNoFields.validateFieldGroup(t.Context(), groupID.String()); err != nil {
		t.Fatalf("nil fields service: want nil error, got %v", err)
	}
}

func TestUpdateWikiTypeCategory_RejectsBadFieldGroup(t *testing.T) {
	svc := &Service{fields: &fakeFieldReader{groups: map[string]gen.CustomFieldGroup{}}}
	bad := genUUIDText(t)

	err := svc.UpdateWikiTypeCategory(t.Context(), genUUIDText(t), UpdateWikiCategoryRequest{FieldGroupID: &bad})
	if !errors.Is(err, ErrFieldGroupNotFound) {
		t.Fatalf("want ErrFieldGroupNotFound, got %v", err)
	}
}

func TestWikiSlugAndCategoryPatterns(t *testing.T) {
	for _, ok := range []string{"overview", "cost-of-living", "visa-routes", "a1"} {
		if !wikiSlugPattern.MatchString(ok) {
			t.Errorf("slug %q should be valid", ok)
		}
	}
	for _, bad := range []string{"", "Cost-Of-Living", "cost of living", "cost_of_living", "-lead", "trail-", "a--b", "_x"} {
		if wikiSlugPattern.MatchString(bad) {
			t.Errorf("slug %q should be invalid", bad)
		}
	}

	for _, ok := range []string{"overview", "cost_of_living", "visa", "a1"} {
		if !wikiCategoryPattern.MatchString(ok) {
			t.Errorf("category %q should be valid", ok)
		}
	}
	for _, bad := range []string{"", "Cost_Of_Living", "cost of living", "cost-of-living", "_lead", "trail_", "a__b", "-x"} {
		if wikiCategoryPattern.MatchString(bad) {
			t.Errorf("category %q should be invalid", bad)
		}
	}
}

func TestWikiCategoryToDTO_MapsNullableFields(t *testing.T) {
	id := mustUUID(t, genUUIDText(t))
	groupID := mustUUID(t, genUUIDText(t))
	gslug, gname := "cost_of_living", "Cost of Living"

	d := wikiCategoryToDTO(WikiCategory{
		ID:             id,
		Slug:           "cost-of-living",
		Category:       "cost_of_living",
		Label:          "Cost of Living",
		FieldGroupID:   groupID,
		FieldGroupSlug: &gslug,
		FieldGroupName: &gname,
		Position:       3,
		IsSystem:       true,
	})

	if d.ID != id.String() || d.Slug != "cost-of-living" || d.Category != "cost_of_living" {
		t.Fatalf("identity mapping wrong: %+v", d)
	}
	if d.FieldGroupID != groupID.String() || d.FieldGroupSlug != gslug || d.FieldGroupName != gname {
		t.Fatalf("group mapping wrong: %+v", d)
	}

	// Unbound category — group fields must be empty, not "00000000-…".
	bare := wikiCategoryToDTO(WikiCategory{Slug: "overview", Category: "overview", Label: "Overview"})
	if bare.ID != "" || bare.FieldGroupID != "" || bare.FieldGroupSlug != "" {
		t.Fatalf("prose-only category leaked zero values: %+v", bare)
	}
}

// genUUIDText returns a random UUIDv4 string without needing a DB —
// value-only tests should not require a database round-trip.
func genUUIDText(t *testing.T) string {
	t.Helper()

	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("rand: %v", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return hex.EncodeToString(b[0:4]) + "-" + hex.EncodeToString(b[4:6]) + "-" +
		hex.EncodeToString(b[6:8]) + "-" + hex.EncodeToString(b[8:10]) + "-" +
		hex.EncodeToString(b[10:16])
}

// ---------------------------------------------------------------------------
// Integration tests — DB-backed via opsTestStore (skipped without
// JC_DATABASE_URL), covering the guards that must see real rows.
// ---------------------------------------------------------------------------

// seedWikiTypeFixture attaches a wiki_type_configs row to any existing space
// type and returns it. Categories/pages the tests create are cleaned up by
// the tests themselves (prefix-tagged slugs).
func seedWikiTypeFixture(t *testing.T, ctx context.Context, database *db.DB) pgtype.UUID {
	t.Helper()

	var typeID pgtype.UUID
	if err := database.Writer.QueryRow(ctx, `SELECT id FROM space_types LIMIT 1`).Scan(&typeID); err != nil {
		t.Fatalf("pick space_type: %v", err)
	}
	if _, err := database.Writer.Exec(ctx,
		`INSERT INTO wiki_type_configs (space_type_id, enabled, seed_stubs) VALUES ($1, true, false)
		 ON CONFLICT (space_type_id) DO NOTHING`, typeID); err != nil {
		t.Fatalf("ensure wiki_type_configs: %v", err)
	}

	return typeID
}

// seedTestGroup inserts a real custom_field_groups row — category rows
// require a valid field_group_id (NOT NULL). The group is removed on
// cleanup after the referencing category (cleanups run LIFO, so seed the
// group before creating categories that bind it).
func seedTestGroup(t *testing.T, ctx context.Context, database *db.DB, slug string) pgtype.UUID {
	t.Helper()

	var id pgtype.UUID
	if err := database.Writer.QueryRow(ctx,
		`INSERT INTO custom_field_groups (name, slug, is_system)
		 VALUES ($1, $2, false)
		 ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name
		 RETURNING id`, "Test "+slug, slug).Scan(&id); err != nil {
		t.Fatalf("seed group: %v", err)
	}
	t.Cleanup(func() {
		database.Writer.Exec(context.Background(), `DELETE FROM custom_field_group_assignments WHERE group_id = $1`, id)
		database.Writer.Exec(context.Background(), `DELETE FROM custom_field_groups WHERE id = $1`, id)
	})

	return id
}

func TestDeleteWikiTypeCategory_SystemGuard(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	typeID := seedWikiTypeFixture(t, ctx, database)
	groupID := seedTestGroup(t, ctx, database, "t-grp-"+genUUIDText(t)[:8])
	svc := &Service{store: store}

	sysSlug := "t-sys-" + genUUIDText(t)[:8]
	cat, err := store.CreateWikiTypeCategory(ctx, typeID, sysSlug, strings.ReplaceAll(sysSlug, "-", "_"), "System Cat", groupID, 90, true)
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	t.Cleanup(func() {
		database.Writer.Exec(context.Background(), `DELETE FROM wiki_type_categories WHERE id = $1`, cat.ID)
	})

	if err := svc.DeleteWikiTypeCategory(ctx, cat.ID.String()); !errors.Is(err, ErrWikiCategoryProtected) {
		t.Fatalf("want ErrWikiCategoryProtected, got %v", err)
	}
}

func TestDeleteWikiTypeCategory_ProseGuard(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	typeID := seedWikiTypeFixture(t, ctx, database)
	fx := seedOpsFixture(t, ctx, database)
	svc := &Service{store: store}

	// Reuse the fixture's space but repoint it at our wiki type so the
	// category↔page join on space_type_id holds.
	if _, err := database.Writer.Exec(ctx, `UPDATE spaces SET space_type_id = $1 WHERE id = $2`, typeID, fx.spaceID); err != nil {
		t.Fatalf("retype space: %v", err)
	}

	groupID := seedTestGroup(t, ctx, database, "t-grp-"+genUUIDText(t)[:8])
	proseSlug := "t-prose-" + genUUIDText(t)[:8]
	proseCat := strings.ReplaceAll(proseSlug, "-", "_")
	cat, err := store.CreateWikiTypeCategory(ctx, typeID, proseSlug, proseCat, "Guarded", groupID, 91, false)
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	t.Cleanup(func() {
		database.Writer.Exec(context.Background(), `DELETE FROM wiki_type_categories WHERE id = $1`, cat.ID)
	})

	// A page in the category with an approved content revision = prose.
	pageID, err := store.CreatePage(ctx, fx.spaceID, fx.userID, "t-pg-"+genUUIDText(t)[:8], "Guarded Page", proseCat)
	if err != nil {
		t.Fatalf("create page: %v", err)
	}
	t.Cleanup(func() {
		database.Writer.Exec(context.Background(), `DELETE FROM wiki_pages WHERE id = $1`, pageID)
	})

	revID, err := store.SubmitRevision(ctx, pageID, fx.userID, "content", []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"body"}]}]}`), nil, "prose", nil, pgtype.UUID{})
	if err != nil {
		t.Fatalf("submit revision: %v", err)
	}
	if _, err := store.ApproveRevisionByID(ctx, revID, fx.userID, ""); err != nil {
		t.Fatalf("approve revision: %v", err)
	}

	if err := svc.DeleteWikiTypeCategory(ctx, cat.ID.String()); !errors.Is(err, ErrWikiCategoryInUse) {
		t.Fatalf("with prose: want ErrWikiCategoryInUse, got %v", err)
	}

	// Drop the page — the guard lifts and the category deletes.
	if _, err := database.Writer.Exec(ctx, `DELETE FROM wiki_pages WHERE id = $1`, pageID); err != nil {
		t.Fatalf("delete page: %v", err)
	}
	if err := svc.DeleteWikiTypeCategory(ctx, cat.ID.String()); err != nil {
		t.Fatalf("after prose removed: want nil, got %v", err)
	}
}

func TestDeleteWikiType_InUse(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	typeID := seedWikiTypeFixture(t, ctx, database)
	fx := seedOpsFixture(t, ctx, database)
	svc := &Service{store: store}

	if _, err := database.Writer.Exec(ctx, `UPDATE spaces SET space_type_id = $1 WHERE id = $2`, typeID, fx.spaceID); err != nil {
		t.Fatalf("retype space: %v", err)
	}
	// fx.pageID is an active page on the space — the type cannot be deleted.
	if err := svc.DeleteWikiType(ctx, typeID.String()); !errors.Is(err, ErrWikiTypeInUse) {
		t.Fatalf("want ErrWikiTypeInUse, got %v", err)
	}
}

func TestCreateWikiTypeCategory_DBValidation(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	typeID := seedWikiTypeFixture(t, ctx, database)
	// Real customfields service — auto-provisioning must write the group +
	// assignment so the category row's mandatory FK resolves.
	svc := &Service{store: store}
	svc.SetCustomFields(customfields.NewService(database))

	if _, err := svc.CreateWikiTypeCategory(ctx, typeID.String(), CreateWikiCategoryRequest{Slug: "Bad Slug", Label: "X"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid slug: want ErrInvalidInput, got %v", err)
	}
	if _, err := svc.CreateWikiTypeCategory(ctx, typeID.String(), CreateWikiCategoryRequest{Slug: "ok-slug", Label: " "}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("empty label: want ErrInvalidInput, got %v", err)
	}
	if _, err := svc.CreateWikiTypeCategory(ctx, typeID.String(), CreateWikiCategoryRequest{Slug: "ok-slug", Category: "bad-token", Label: "X"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid category token: want ErrInvalidInput, got %v", err)
	}
	if _, err := svc.CreateWikiTypeCategory(ctx, typeID.String(), CreateWikiCategoryRequest{Slug: "ok-slug", Label: "X", FieldGroupID: genUUIDText(t)}); !errors.Is(err, ErrFieldGroupNotFound) {
		t.Fatalf("unknown group: want ErrFieldGroupNotFound, got %v", err)
	}

	slug := "t-cr-" + genUUIDText(t)[:8]
	cat, err := svc.CreateWikiTypeCategory(ctx, typeID.String(), CreateWikiCategoryRequest{Slug: slug, Label: "Created"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() {
		database.Writer.Exec(context.Background(), `DELETE FROM wiki_type_categories WHERE slug = $1`, slug)
		if cat.FieldGroupID != "" {
			database.Writer.Exec(context.Background(), `DELETE FROM custom_field_group_assignments WHERE group_id = $1`, cat.FieldGroupID)
			database.Writer.Exec(context.Background(), `DELETE FROM custom_field_groups WHERE id = $1`, cat.FieldGroupID)
		}
	})

	// Category token derives from the slug (hyphens → underscores) when omitted.
	if cat.Category != strings.ReplaceAll(slug, "-", "_") {
		t.Fatalf("derived category: want %q, got %q", strings.ReplaceAll(slug, "-", "_"), cat.Category)
	}
	if cat.Slug != slug || cat.Label != "Created" {
		t.Fatalf("created row mismatch: %+v", cat)
	}
	// A dedicated group was auto-provisioned: wiki_{type}_{category}.
	var typeSlug string
	if err := database.Writer.QueryRow(ctx, `SELECT slug FROM space_types WHERE id = $1`, typeID).Scan(&typeSlug); err != nil {
		t.Fatalf("type slug: %v", err)
	}
	wantGroupSlug := "wiki_" + typeSlug + "_" + cat.Category
	if cat.FieldGroupSlug != wantGroupSlug {
		t.Fatalf("auto-provisioned group: want slug %q, got %+v", wantGroupSlug, cat)
	}
	// And it is scoped to exactly this type's entity class.
	var assignCount int
	if err := database.Writer.QueryRow(ctx,
		`SELECT count(*) FROM custom_field_group_assignments
		 WHERE group_id = $1 AND entity_class = $2 AND entity_id IS NULL`,
		cat.FieldGroupID, "space_type:"+typeSlug).Scan(&assignCount); err != nil {
		t.Fatalf("assignment lookup: %v", err)
	}
	if assignCount != 1 {
		t.Fatalf("want 1 space_type:%s assignment, got %d", typeSlug, assignCount)
	}
}

func TestUpdateWikiTypeCategory_PatchesMutableFields(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	typeID := seedWikiTypeFixture(t, ctx, database)
	groupID := seedTestGroup(t, ctx, database, "t-grp-"+genUUIDText(t)[:8])
	svc := &Service{store: store}
	svc.SetCustomFields(customfields.NewService(database))

	updSlug := "t-upd-" + genUUIDText(t)[:8]
	cat, err := store.CreateWikiTypeCategory(ctx, typeID, updSlug, strings.ReplaceAll(updSlug, "-", "_"), "Before", groupID, 92, false)
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	t.Cleanup(func() {
		database.Writer.Exec(context.Background(), `DELETE FROM wiki_type_categories WHERE id = $1`, cat.ID)
	})

	// The binding is mandatory — an explicit empty field_group_id is
	// rejected rather than clearing the group.
	empty := ""
	if err := svc.UpdateWikiTypeCategory(ctx, cat.ID.String(), UpdateWikiCategoryRequest{FieldGroupID: &empty}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("clear binding: want ErrInvalidInput, got %v", err)
	}

	label, pos := "After", int32(7)
	if err := svc.UpdateWikiTypeCategory(ctx, cat.ID.String(), UpdateWikiCategoryRequest{Label: &label, Position: &pos}); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := store.GetWikiTypeCategory(ctx, cat.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Label != "After" || got.Position != 7 || got.FieldGroupID != groupID {
		t.Fatalf("patch mismatch: %+v", got)
	}
	// Slug and category are immutable — not even addressable by the update
	// request; confirm the row is unchanged.
	if got.Slug != cat.Slug || got.Category != cat.Category {
		t.Fatalf("immutable fields changed: %+v vs %+v", got, cat)
	}
}

func TestProvisionCategoryGroup(t *testing.T) {
	// Nil provisioner → mandatory binding cannot be satisfied.
	svcNil := &Service{}
	if _, err := svcNil.provisionCategoryGroup(t.Context(), "city", "cost_of_living", "Cost of Living"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("nil provisioner: want ErrInvalidInput, got %v", err)
	}

	// Fresh category → dedicated group + entity-class assignment created.
	fields := &fakeFieldReader{}
	svc := &Service{fields: fields, fieldProvisioner: fields}

	gid, err := svc.provisionCategoryGroup(t.Context(), "city", "cost_of_living", "Cost of Living")
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if len(fields.createdGroups) != 1 {
		t.Fatalf("want 1 created group, got %d", len(fields.createdGroups))
	}
	g := fields.createdGroups[0]
	if g.Slug != "wiki_city_cost_of_living" || g.Name != "Cost of Living (city)" || g.IsSystem {
		t.Fatalf("created group mismatch: %+v", g)
	}
	if gid != g.ID {
		t.Fatalf("returned id %v != created group %v", gid, g.ID)
	}
	if len(fields.createdAssigns) != 1 {
		t.Fatalf("want 1 assignment, got %d", len(fields.createdAssigns))
	}
	a := fields.createdAssigns[0]
	if a.GroupID != g.ID || a.EntityClass != "space_type:city" || a.EntityID.Valid {
		t.Fatalf("assignment mismatch: %+v", a)
	}

	// Per-level isolation — the same category on another level yields a
	// distinct group slug.
	if _, err := svc.provisionCategoryGroup(t.Context(), "country", "cost_of_living", "Cost of Living"); err != nil {
		t.Fatalf("provision country: %v", err)
	}
	if len(fields.createdGroups) != 2 || fields.createdGroups[1].Slug != "wiki_country_cost_of_living" {
		t.Fatalf("country group: %+v", fields.createdGroups)
	}

	// Reuse — an existing group with its assignment intact is bound as-is.
	fields2 := &fakeFieldReader{
		groups: map[string]gen.CustomFieldGroup{
			g.ID.String(): g,
		},
		assignments: []gen.CustomFieldGroupAssignment{
			{GroupID: g.ID, EntityClass: "space_type:city"},
		},
	}
	svc2 := &Service{fields: fields2, fieldProvisioner: fields2}
	gotID, err := svc2.provisionCategoryGroup(t.Context(), "city", "cost_of_living", "Cost of Living")
	if err != nil {
		t.Fatalf("reuse provision: %v", err)
	}
	if gotID != g.ID || len(fields2.createdGroups) != 0 || len(fields2.createdAssigns) != 0 {
		t.Fatalf("reuse should create nothing: id=%v groups=%d assigns=%d", gotID, len(fields2.createdGroups), len(fields2.createdAssigns))
	}

	// Assignment create fails AND the retry read finds nothing — the error
	// must surface; a silently-missing assignment leaves the group
	// invisible to entity-field resolution.
	fields3 := &fakeFieldReader{assignErr: errors.New("conn reset")}
	svc3 := &Service{fields: fields3, fieldProvisioner: fields3}
	if _, err := svc3.provisionCategoryGroup(t.Context(), "city", "cost_of_living", "Cost of Living"); err == nil {
		t.Fatal("assignment failure must surface, got nil error")
	}

	// Assignment create fails but the retry read finds the row (concurrent
	// commit / replica lag shadowed it in the first list) — provisioning
	// succeeds by reuse rather than erroring.
	existingID := mustUUID(t, genUUIDText(t))
	fields4 := &fakeFieldReader{
		assignErr: errors.New("conn reset"),
		groups: map[string]gen.CustomFieldGroup{
			existingID.String(): {ID: existingID, Slug: "wiki_city_cost_of_living", Name: "Cost of Living (city)"},
		},
		assignmentsOnRetry: []gen.CustomFieldGroupAssignment{
			{GroupID: existingID, EntityClass: "space_type:city"},
		},
	}
	svc4 := &Service{fields: fields4, fieldProvisioner: fields4}
	gotID4, err := svc4.provisionCategoryGroup(t.Context(), "city", "cost_of_living", "Cost of Living")
	if err != nil {
		t.Fatalf("retry-materialized assignment should succeed: %v", err)
	}
	if gotID4 != existingID {
		t.Fatalf("want reused group %v, got %v", existingID, gotID4)
	}
}

func TestDeleteFieldGroup_BlockedWhileCategoryBound(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	typeID := seedWikiTypeFixture(t, ctx, database)
	groupID := seedTestGroup(t, ctx, database, "t-grp-"+genUUIDText(t)[:8])
	cfSvc := customfields.NewService(database)

	catSlug := "t-fg-" + genUUIDText(t)[:8]
	cat, err := store.CreateWikiTypeCategory(ctx, typeID, catSlug, strings.ReplaceAll(catSlug, "-", "_"), "Bound", groupID, 93, false)
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	t.Cleanup(func() {
		database.Writer.Exec(context.Background(), `DELETE FROM wiki_type_categories WHERE id = $1`, cat.ID)
	})

	// ON DELETE RESTRICT — a bound group cannot be deleted even with force.
	if err := cfSvc.DeleteFieldGroup(ctx, groupID, true); !errors.Is(err, errpkg.ErrInUse) {
		t.Fatalf("delete bound group: want ErrInUse, got %v", err)
	}

	// Deleting the category leaves the group + its data intact; the group
	// is then deletable on its own.
	if _, err := database.Writer.Exec(ctx, `DELETE FROM wiki_type_categories WHERE id = $1`, cat.ID); err != nil {
		t.Fatalf("delete category: %v", err)
	}
	if err := cfSvc.DeleteFieldGroup(ctx, groupID, true); err != nil {
		t.Fatalf("delete unbound group: %v", err)
	}
}

func TestUpsertWikiType_ParentsMustBeWikiTypes(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	typeID := seedWikiTypeFixture(t, ctx, database)
	svc := &Service{store: store}

	// A random UUID parses but is not a wiki type → invalid input.
	if _, err := svc.UpsertWikiType(ctx, UpsertWikiTypeRequest{
		SpaceTypeID:   typeID.String(),
		ParentTypeIDs: []string{genUUIDText(t)},
	}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("non-wiki-type parent: want ErrInvalidInput, got %v", err)
	}

	// A real wiki type as parent is accepted.
	var parentID pgtype.UUID
	if err := database.Writer.QueryRow(ctx, `SELECT id FROM space_types OFFSET 1 LIMIT 1`).Scan(&parentID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			t.Skip("need ≥2 space_types for a parent edge")
		}
		t.Fatalf("pick parent type: %v", err)
	}
	if _, err := database.Writer.Exec(ctx,
		`INSERT INTO wiki_type_configs (space_type_id, enabled) VALUES ($1, true) ON CONFLICT DO NOTHING`, parentID); err != nil {
		t.Fatalf("ensure parent config: %v", err)
	}
	t.Cleanup(func() {
		database.Writer.Exec(context.Background(), `DELETE FROM wiki_type_parents WHERE space_type_id = $1`, typeID)
	})

	dto, err := svc.UpsertWikiType(ctx, UpsertWikiTypeRequest{
		SpaceTypeID:   typeID.String(),
		ParentTypeIDs: []string{parentID.String()},
	})
	if err != nil {
		t.Fatalf("upsert with valid parent: %v", err)
	}
	if len(dto.ParentTypeIDs) == 0 {
		t.Fatal("expected parent edge in returned DTO")
	}
}
