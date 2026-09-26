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

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
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

	// Empty = prose-only — returns an invalid (NULL) UUID without a lookup.
	id, err := svc.validateFieldGroup(t.Context(), "")
	if err != nil || id.Valid {
		t.Fatalf("empty: want valid NULL id, got %v / %v", id, err)
	}

	if _, err := svc.validateFieldGroup(t.Context(), "not-a-uuid"); !errors.Is(err, ErrFieldGroupNotFound) {
		t.Fatalf("bad uuid: want ErrFieldGroupNotFound, got %v", err)
	}
	if _, err := svc.validateFieldGroup(t.Context(), genUUIDText(t)); !errors.Is(err, ErrFieldGroupNotFound) {
		t.Fatalf("unknown group: want ErrFieldGroupNotFound, got %v", err)
	}

	id, err = svc.validateFieldGroup(t.Context(), groupID.String())
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

func TestDeleteWikiTypeCategory_SystemGuard(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	typeID := seedWikiTypeFixture(t, ctx, database)
	svc := &Service{store: store}

	sysSlug := "t-sys-" + genUUIDText(t)[:8]
	cat, err := store.CreateWikiTypeCategory(ctx, typeID, sysSlug, strings.ReplaceAll(sysSlug, "-", "_"), "System Cat", pgtype.UUID{}, 90, true)
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

	proseSlug := "t-prose-" + genUUIDText(t)[:8]
	proseCat := strings.ReplaceAll(proseSlug, "-", "_")
	cat, err := store.CreateWikiTypeCategory(ctx, typeID, proseSlug, proseCat, "Guarded", pgtype.UUID{}, 91, false)
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
	// fakeFieldReader validates field_group_id against an empty group set —
	// any binding resolves to ErrFieldGroupNotFound before the store.
	svc := &Service{store: store, fields: &fakeFieldReader{groups: map[string]gen.CustomFieldGroup{}}}

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
	})

	// Category token derives from the slug (hyphens → underscores) when omitted.
	if cat.Category != strings.ReplaceAll(slug, "-", "_") {
		t.Fatalf("derived category: want %q, got %q", strings.ReplaceAll(slug, "-", "_"), cat.Category)
	}
	if cat.Slug != slug || cat.Label != "Created" {
		t.Fatalf("created row mismatch: %+v", cat)
	}
}

func TestUpdateWikiTypeCategory_PatchesMutableFields(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	typeID := seedWikiTypeFixture(t, ctx, database)
	svc := &Service{store: store}

	updSlug := "t-upd-" + genUUIDText(t)[:8]
	cat, err := store.CreateWikiTypeCategory(ctx, typeID, updSlug, strings.ReplaceAll(updSlug, "-", "_"), "Before", pgtype.UUID{}, 92, false)
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	t.Cleanup(func() {
		database.Writer.Exec(context.Background(), `DELETE FROM wiki_type_categories WHERE id = $1`, cat.ID)
	})

	label, pos, fg := "After", int32(7), ""
	if err := svc.UpdateWikiTypeCategory(ctx, cat.ID.String(), UpdateWikiCategoryRequest{Label: &label, Position: &pos, FieldGroupID: &fg}); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := store.GetWikiTypeCategory(ctx, cat.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Label != "After" || got.Position != 7 || got.FieldGroupID.Valid {
		t.Fatalf("patch mismatch: %+v", got)
	}
	// Slug and category are immutable — not even addressable by the update
	// request; confirm the row is unchanged.
	if got.Slug != cat.Slug || got.Category != cat.Category {
		t.Fatalf("immutable fields changed: %+v vs %+v", got, cat)
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
