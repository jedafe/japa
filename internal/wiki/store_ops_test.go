package wiki

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
)

// opsTestStore returns a Store backed by a real Postgres. Skipped unless
// JC_DATABASE_URL is set, so it never runs in CI without a database.
func opsTestStore(t *testing.T) (*Store, *db.DB) {
	t.Helper()

	dsn := os.Getenv("JC_DATABASE_URL")
	if dsn == "" {
		t.Skip("JC_DATABASE_URL not set; skipping integration test")
	}

	database, err := db.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}

	t.Cleanup(func() {
		database.Writer.Close()
		database.Reader.Close()
	})

	return NewStore(database), database
}

// opsFixture holds the rows shared by the ops-revision tests.
type opsFixture struct {
	userID  pgtype.UUID
	spaceID pgtype.UUID
	pageID  pgtype.UUID
	target  pgtype.UUID
}

// seedOpsFixture creates a user, a space, and a wiki page, and registers
// cascade cleanup. The target entity ID is a bare UUID — target_entity_id is
// polymorphic with no FK, so the entity itself need not exist.
func seedOpsFixture(t *testing.T, ctx context.Context, database *db.DB) opsFixture {
	t.Helper()

	var fx opsFixture

	mustID := func(query string, args ...any) pgtype.UUID {
		var id pgtype.UUID
		if err := database.Writer.QueryRow(ctx, query, args...).Scan(&id); err != nil {
			t.Fatalf("fixture insert: %v", err)
		}

		return id
	}

	fx.userID = mustID(`INSERT INTO users (id,email,username,profile_type,status,created_at,updated_at,follower_count,force_password_change)
		VALUES (gen_random_uuid(),gen_random_uuid()::text||'@test.local','wop_'||replace(gen_random_uuid()::text,'-',''),'regular','active',now(),now(),0,false) RETURNING id`)

	spaceTypeID := mustID(`SELECT id FROM space_types LIMIT 1`)

	fx.spaceID = mustID(`INSERT INTO spaces (id,space_type_id,name,slug,level,visibility,status,created_at,updated_at)
		VALUES (gen_random_uuid(),$1,'wop_'||gen_random_uuid()::text,'wop-'||replace(gen_random_uuid()::text,'-',''),'custom','public','active',now(),now()) RETURNING id`,
		spaceTypeID)

	fx.pageID = mustID(`INSERT INTO wiki_pages (id,space_id,slug,title,category,created_by,created_at,updated_at)
		VALUES (gen_random_uuid(),$1,'wop-'||replace(gen_random_uuid()::text,'-',''),'Ops Test Page','overview',$2,now(),now()) RETURNING id`,
		fx.spaceID, fx.userID)

	fx.target = mustUUID(t, genUUID(t, ctx, database))

	t.Cleanup(func() {
		database.Writer.Exec(ctx, `DELETE FROM wiki_revisions WHERE submitted_by = $1 OR target_entity_id = $2`, fx.userID, fx.target)
		database.Writer.Exec(ctx, `DELETE FROM wiki_pages WHERE id = $1`, fx.pageID)
		database.Writer.Exec(ctx, `DELETE FROM spaces WHERE id = $1`, fx.spaceID)
		database.Writer.Exec(ctx, `DELETE FROM users WHERE id = $1`, fx.userID)
	})

	return fx
}

func genUUID(t *testing.T, ctx context.Context, database *db.DB) string {
	t.Helper()

	var id string
	if err := database.Writer.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&id); err != nil {
		t.Fatalf("gen uuid: %v", err)
	}

	return id
}

func mustUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()

	var id pgtype.UUID
	if err := id.Scan(s); err != nil {
		t.Fatalf("parse uuid %q: %v", s, err)
	}

	return id
}

// TestSubmitRevision_PageLessFieldsRevision covers the §80 ops-revision
// shape: kind='fields', a JSON ops payload, a polymorphic target, and a NULL
// wiki_page_id — then round-trips through GetRevision.
func TestSubmitRevision_PageLessFieldsRevision(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	fx := seedOpsFixture(t, ctx, database)

	ops := []byte(`[{"op":"set_value","entity_type":"university","entity_id":"` + fx.target.String() + `","field_key":"tuition_fee","value":12000}]`)
	targetType := "university"

	revID, err := store.SubmitRevision(ctx, pgtype.UUID{}, fx.userID, "fields", nil, ops, "set tuition", &targetType, fx.target)
	if err != nil {
		t.Fatalf("SubmitRevision: %v", err)
	}

	if !revID.Valid {
		t.Fatal("expected valid revision id")
	}

	rev, err := store.GetRevision(ctx, revID)
	if err != nil {
		t.Fatalf("GetRevision: %v", err)
	}

	if rev.Kind != "fields" {
		t.Fatalf("kind: got %q want %q", rev.Kind, "fields")
	}

	if rev.WikiPageID.Valid {
		t.Fatalf("expected NULL wiki_page_id, got %s", rev.WikiPageID.String())
	}

	if rev.TargetEntityType == nil || *rev.TargetEntityType != "university" {
		t.Fatalf("target_entity_type: got %v", rev.TargetEntityType)
	}

	if !rev.TargetEntityID.Valid || rev.TargetEntityID.String() != fx.target.String() {
		t.Fatalf("target_entity_id: got %s want %s", rev.TargetEntityID.String(), fx.target.String())
	}

	if rev.Status != "pending" {
		t.Fatalf("status: got %q want pending", rev.Status)
	}

	// jsonb normalizes key order/whitespace — compare parsed values.
	var wantOps, gotOps any
	if err := json.Unmarshal(ops, &wantOps); err != nil {
		t.Fatalf("unmarshal want ops: %v", err)
	}

	if err := json.Unmarshal(rev.Ops, &gotOps); err != nil {
		t.Fatalf("unmarshal got ops: %v", err)
	}

	if !reflect.DeepEqual(wantOps, gotOps) {
		t.Fatalf("ops round-trip: got %s want %s", rev.Ops, ops)
	}

	// Fields-only revision stores an empty doc so NOT NULL content holds.
	if string(rev.Content) != "{}" {
		t.Fatalf("content: got %s want {}", rev.Content)
	}

	// A second identical submission on the same (NULL) page must not be
	// blocked by the page-hash unique index — NULL wiki_page_id rows are
	// distinct under the index. It inserts a second pending row instead.
	if _, err := store.SubmitRevision(ctx, pgtype.UUID{}, fx.userID, "fields", nil, ops, "set tuition", &targetType, fx.target); err != nil {
		t.Fatalf("second SubmitRevision (page-less): %v", err)
	}
}

// TestAdminListRevisions_KindFilter verifies the admin review queue filters
// revisions by kind and returns the new ops columns.
func TestAdminListRevisions_KindFilter(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	fx := seedOpsFixture(t, ctx, database)

	// One content revision bound to the page, one fields revision page-less.
	contentRevID, err := store.SubmitRevision(ctx, fx.pageID, fx.userID, "content",
		[]byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"hello wiki"}]}]}`),
		nil, "content edit", nil, pgtype.UUID{})
	if err != nil {
		t.Fatalf("SubmitRevision content: %v", err)
	}

	targetType := "university"
	fieldsRevID, err := store.SubmitRevision(ctx, pgtype.UUID{}, fx.userID, "fields",
		nil, []byte(`[{"op":"set_value","entity_type":"university","entity_id":"`+fx.target.String()+`","field_key":"tuition_fee","value":13000}]`),
		"fields edit", &targetType, fx.target)
	if err != nil {
		t.Fatalf("SubmitRevision fields: %v", err)
	}

	// kind=fields → only the fields revision.
	fieldsOnly, total, err := store.AdminListRevisions(ctx, "fields", "", 50, 0)
	if err != nil {
		t.Fatalf("AdminListRevisions kind=fields: %v", err)
	}

	var foundFields bool

	for _, r := range fieldsOnly {
		if r.ID == fieldsRevID {
			foundFields = true
		}

		if r.Kind != "fields" {
			t.Fatalf("kind=fields filter leaked %q revision %s", r.Kind, r.ID.String())
		}
	}

	if !foundFields || total < 1 {
		t.Fatalf("fields revision %s not in filtered list (total=%d)", fieldsRevID, total)
	}

	// No filter → both revisions present.
	all, _, err := store.AdminListRevisions(ctx, "", "", 50, 0)
	if err != nil {
		t.Fatalf("AdminListRevisions unfiltered: %v", err)
	}

	var sawContent, sawFields bool

	for _, r := range all {
		if r.ID == contentRevID {
			sawContent = true
		}

		if r.ID == fieldsRevID {
			sawFields = true
		}
	}

	if !sawContent || !sawFields {
		t.Fatalf("unfiltered list missing revisions (content=%v fields=%v)", sawContent, sawFields)
	}

	// status filter composes with kind.
	pendingFields, _, err := store.AdminListRevisions(ctx, "fields", "pending", 50, 0)
	if err != nil {
		t.Fatalf("AdminListRevisions kind+status: %v", err)
	}

	for _, r := range pendingFields {
		if r.Status != "pending" || r.Kind != "fields" {
			t.Fatalf("combined filter leaked %s/%s revision", r.Kind, r.Status)
		}
	}
}

// TestCountPendingByTarget verifies pending counting per polymorphic target,
// including the drop after ApproveRevisionByID approves a row.
func TestCountPendingByTarget(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	fx := seedOpsFixture(t, ctx, database)

	targetType := "university"
	mkOps := func(v int) []byte {
		return []byte(`[{"op":"set_value","entity_type":"university","entity_id":"` + fx.target.String() + `","field_key":"tuition_fee","value":` + strconv.Itoa(v) + `}]`)
	}

	before, err := store.CountPendingByTarget(ctx, targetType, fx.target)
	if err != nil {
		t.Fatalf("CountPendingByTarget: %v", err)
	}

	revA, err := store.SubmitRevision(ctx, pgtype.UUID{}, fx.userID, "fields", nil, mkOps(1), "a", &targetType, fx.target)
	if err != nil {
		t.Fatalf("SubmitRevision A: %v", err)
	}

	if _, err := store.SubmitRevision(ctx, pgtype.UUID{}, fx.userID, "fields", nil, mkOps(2), "b", &targetType, fx.target); err != nil {
		t.Fatalf("SubmitRevision B: %v", err)
	}

	after2, err := store.CountPendingByTarget(ctx, targetType, fx.target)
	if err != nil {
		t.Fatalf("CountPendingByTarget after submit: %v", err)
	}

	if after2 != before+2 {
		t.Fatalf("pending count: got %d want %d", after2, before+2)
	}

	// Approve one — count drops by one; ApproveRevisionByID returns the row
	// with kind + ops populated for the ops-apply path.
	approved, err := store.ApproveRevisionByID(ctx, revA, fx.userID, "looks good")
	if err != nil {
		t.Fatalf("ApproveRevisionByID: %v", err)
	}

	if approved.Status != "approved" || approved.Kind != "fields" || len(approved.Ops) == 0 {
		t.Fatalf("approved row malformed: %+v", approved)
	}

	afterApprove, err := store.CountPendingByTarget(ctx, targetType, fx.target)
	if err != nil {
		t.Fatalf("CountPendingByTarget after approve: %v", err)
	}

	if afterApprove != before+1 {
		t.Fatalf("pending count after approve: got %d want %d", afterApprove, before+1)
	}

	// A second approve of the same revision must fail (not pending).
	if _, err := store.ApproveRevisionByID(ctx, revA, fx.userID, ""); err == nil {
		t.Fatal("expected error re-approving a non-pending revision")
	}
}

// TestGetRevision_NotFound and TestFindHostPageID cover the lookup helpers.
func TestGetRevision_NotFound(t *testing.T) {
	store, _ := opsTestStore(t)

	_, err := store.GetRevision(t.Context(), mustUUID(t, "00000000-0000-0000-0000-000000000001"))
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected pgx.ErrNoRows, got %v", err)
	}
}

func TestFindHostPageID(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	fx := seedOpsFixture(t, ctx, database)

	hostID, err := store.FindHostPageID(ctx, fx.spaceID, "overview")
	if err != nil {
		t.Fatalf("FindHostPageID: %v", err)
	}

	if hostID != fx.pageID {
		t.Fatalf("host page: got %s want %s", hostID, fx.pageID)
	}

	if _, err := store.FindHostPageID(ctx, fx.spaceID, "no-such-category"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected pgx.ErrNoRows for missing category, got %v", err)
	}

	typeID, _, err := store.GetSpaceTypeRef(ctx, fx.spaceID)
	if err != nil {
		t.Fatalf("GetSpaceTypeRef: %v", err)
	}

	if !typeID.Valid {
		t.Fatal("expected valid space_type_id")
	}
}
