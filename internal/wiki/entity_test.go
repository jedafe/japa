package wiki

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
)

// seedOfferingFixtures seeds two spaces, one catalog course, and one
// published offering per space — the shared fixture for program/
// related-university tests (spec 2026-10-30-wiki-university-course-
// program-links).
func seedOfferingFixtures(t *testing.T, ctx context.Context, database *db.DB, typeID pgtype.UUID, uniq string) (uniA, uniB, courseID pgtype.UUID) {
	t.Helper()

	mkSpace := func(slug string) pgtype.UUID {
		return mustIDF(t, ctx, database,
			`INSERT INTO spaces (id,space_type_id,name,slug,level,visibility,status)
			 VALUES (gen_random_uuid(),$1,$2,$3,'custom','public','active') RETURNING id`,
			typeID, slug, slug)
	}
	uniA, uniB = mkSpace("wop-uni-a-"+uniq), mkSpace("wop-uni-b-"+uniq)

	courseID = mustIDF(t, ctx, database,
		`INSERT INTO university_course_catalog (id,title,slug,is_published)
		 VALUES (gen_random_uuid(),$1,$2,true) RETURNING id`,
		"Wop Program "+uniq, "wop-prog-"+uniq)

	for _, tc := range []struct {
		uni  pgtype.UUID
		slug string
	}{
		{uniA, "wop-off-a-" + uniq},
		{uniB, "wop-off-b-" + uniq},
	} {
		mustIDF(t, ctx, database,
			`INSERT INTO university_course_offerings (id,course_id,university_id,slug,is_published)
			 VALUES (gen_random_uuid(),$1,$2,$3,true) RETURNING id`,
			courseID, tc.uni, tc.slug)
	}

	t.Cleanup(func() {
		cctx := t.Context()
		database.Writer.Exec(cctx, `DELETE FROM university_course_offerings WHERE course_id = $1`, courseID)
		database.Writer.Exec(cctx, `DELETE FROM university_course_catalog WHERE id = $1`, courseID)
		database.Writer.Exec(cctx, `DELETE FROM spaces WHERE id IN ($1, $2)`, uniA, uniB)
	})

	return uniA, uniB, courseID
}

// TestResolveEntityRef_Program — the catalog entity kind resolves a
// published catalog row by slug, with no host space.
func TestResolveEntityRef_Program(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	uniq := strings.ReplaceAll(genUUID(t, ctx, database), "-", "")[:12]

	mustIDF(t, ctx, database,
		`INSERT INTO university_course_catalog (id,title,slug,is_published)
		 VALUES (gen_random_uuid(),$1,$2,true) RETURNING id`,
		"Wop Program "+uniq, "wop-program-"+uniq)
	t.Cleanup(func() {
		database.Writer.Exec(t.Context(),
			`DELETE FROM university_course_catalog WHERE slug = $1`, "wop-program-"+uniq)
	})

	ref, err := store.ResolveEntityRef(ctx, "program", "wop-program-"+uniq)
	if err != nil {
		t.Fatalf("ResolveEntityRef: %v", err)
	}
	if ref.Name != "Wop Program "+uniq || ref.ParentSpaceID.Valid {
		t.Fatalf("unexpected ref: %+v", ref)
	}
}

// TestComputeEntityRelated_CourseSiblings — an offering's related set is
// the OTHER universities offering the same catalog course; a program's
// related set is every offering's university.
func TestComputeEntityRelated_CourseSiblings(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	hf := seedHierarchyFixture(t, ctx, database)
	uniq := strings.ReplaceAll(genUUID(t, ctx, database), "-", "")[:12]

	uniA, uniB, _ := seedOfferingFixtures(t, ctx, database, hf.typeID, uniq)
	_ = uniA

	svc := NewService(store, nil)

	rel := svc.computeEntityRelated(ctx, "course", EntityRef{Kind: "course", Slug: "wop-off-a-" + uniq})
	if rel == nil || len(rel.Items) != 1 || rel.Items[0].Slug != "wop-uni-b-"+uniq {
		t.Fatalf("course related: %+v", rel)
	}
	if rel.Items[0].OfferingSlug != "wop-off-b-"+uniq {
		t.Fatalf("offering slug missing: %+v", rel.Items[0])
	}

	rel = svc.computeEntityRelated(ctx, "program", EntityRef{Kind: "program", Slug: "wop-prog-" + uniq})
	if rel == nil || rel.Total != 2 {
		t.Fatalf("program related: %+v", rel)
	}
	seen := map[string]bool{}
	for _, it := range rel.Items {
		seen[it.Slug] = true
	}
	if !seen["wop-uni-a-"+uniq] || !seen["wop-uni-b-"+uniq] {
		t.Fatalf("program related unis: %+v", rel.Items)
	}
	_ = uniB
}
