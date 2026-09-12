package tags

import (
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
)

// TestTagModeratorsIntegration exercises the full tag-moderator lifecycle
// (add → list → remove) against a real Postgres. It is skipped unless
// JC_DATABASE_URL is set, so it never runs in CI without a database.
func TestTagModeratorsIntegration(t *testing.T) {
	dsn := os.Getenv("JC_DATABASE_URL")
	if dsn == "" {
		t.Skip("JC_DATABASE_URL not set; skipping integration test")
	}

	database, err := db.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}

	defer database.Writer.Close()
	defer database.Reader.Close()

	ctx := t.Context()
	svc := NewService(database)

	// Minimal fixtures (cascade cleanup at the end).
	var tenantID, userID, tagID pgtype.UUID

	mustExecReturning := func(query string, args ...any) pgtype.UUID {
		var id pgtype.UUID
		if err := database.Writer.QueryRow(ctx, query, args...).Scan(&id); err != nil {
			t.Fatalf("fixture insert: %v", err)
		}

		return id
	}

	tenantID = mustExecReturning(`INSERT INTO tenants (id,name,slug,status,created_at,updated_at)
		VALUES (gen_random_uuid(),'tm','tm_'||replace(gen_random_uuid()::text,'-',''),'active',now(),now()) RETURNING id`)
	userID = mustExecReturning(`INSERT INTO users (id,tenant_id,email,username,profile_type,status,created_at,updated_at,follower_count,force_password_change)
		VALUES (gen_random_uuid(),$1,gen_random_uuid()::text||'@test.local','mod_'||replace(gen_random_uuid()::text,'-',''),'personal','active',now(),now(),0,false) RETURNING id`, tenantID)
	tagID = mustExecReturning(`INSERT INTO tags (id,tenant_id,name,slug,follower_count,created_at,updated_at)
		VALUES (gen_random_uuid(),$1,'modtag','modtag_'||replace(gen_random_uuid()::text,'-',''),0,now(),now()) RETURNING id`, tenantID)

	t.Cleanup(func() {
		database.Writer.Exec(ctx, `DELETE FROM tag_moderators WHERE tenant_id=$1`, tenantID)
		database.Writer.Exec(ctx, `DELETE FROM tags WHERE id=$1`, tagID)
		database.Writer.Exec(ctx, `DELETE FROM users WHERE id=$1`, userID)
		database.Writer.Exec(ctx, `DELETE FROM tenants WHERE id=$1`, tenantID)
	})

	// Add.
	added, err := svc.AddTagModerator(ctx, AddTagModeratorInput{
		TagID:  tagID,
		UserID: userID,
	})
	if err != nil {
		t.Fatalf("AddTagModerator: %v", err)
	}

	if added.TagID != tagID || added.UserID != userID {
		t.Fatalf("unexpected row: %+v", added)
	}

	// List → exactly one.
	list, err := svc.ListTagModerators(ctx, tagID, tenantID)
	if err != nil {
		t.Fatalf("ListTagModerators: %v", err)
	}

	if len(list) != 1 {
		t.Fatalf("expected 1 moderator, got %d", len(list))
	}

	if list[0].UserID != userID || list[0].Username == "" {
		t.Fatalf("unexpected list row: %+v", list[0])
	}

	// Idempotent re-add does not duplicate.
	if _, err := svc.AddTagModerator(ctx, AddTagModeratorInput{
		TagID:  tagID,
		UserID: userID,
	}); err != nil {
		t.Fatalf("re-AddTagModerator: %v", err)
	}

	list, _ = svc.ListTagModerators(ctx, tagID, tenantID)
	if len(list) != 1 {
		t.Fatalf("expected idempotent add to keep 1 moderator, got %d", len(list))
	}

	// Remove.
	if err := svc.RemoveTagModerator(ctx, tagID, userID, tenantID); err != nil {
		t.Fatalf("RemoveTagModerator: %v", err)
	}

	list, _ = svc.ListTagModerators(ctx, tagID, tenantID)
	if len(list) != 0 {
		t.Fatalf("expected 0 moderators after remove, got %d", len(list))
	}
}
