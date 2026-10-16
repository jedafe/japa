package rag

// Integration test for the §69 scoped-retrieval amendment: the full
// visibility matrix is exercised against a real Postgres + pgvector so the
// generated SQL (not a stub) proves each clause. Skipped unless
// JC_DATABASE_URL is set — never runs in CI without a database.
//
// Ref: japaconnect-engineering/sections/69-rag-knowledge-layer.md

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
)

// fixture IDs for the scoped-retrieval matrix.
type scopeFixture struct {
	authorID   pgtype.UUID
	memberID   pgtype.UUID // approved member of the private space
	pendingID  pgtype.UUID // pending member — must gain nothing
	outsiderID pgtype.UUID // authenticated, no membership
	privSpace  pgtype.UUID
	pubSpace   pgtype.UUID
	cPublic    pgtype.UUID
	cMembers   pgtype.UUID
	cPrivate   pgtype.UUID // author-private, not in the member space
	cPrivSpace pgtype.UUID // author-private INSIDE the member space
	thread     pgtype.UUID // thread inside the private space
}

func TestScopedRetrievalMatrix(t *testing.T) {
	dsn := os.Getenv("JC_DATABASE_URL")
	if dsn == "" {
		t.Skip("JC_DATABASE_URL not set; skipping integration test")
	}

	database, err := db.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	// t.Cleanup is LIFO — registering the closes BEFORE the fixture cleanup
	// means fixture rows are deleted first, then the pools close.
	t.Cleanup(database.Writer.Close)
	t.Cleanup(database.Reader.Close)

	ctx := t.Context()
	fx := seedScopeFixture(t, ctx, database)
	store := NewStore(database)

	// All fixture chunks share one vector, so similarity ordering is
	// irrelevant — presence/absence in the result set is the assertion.
	vec := make([]float32, 1536)
	for i := range vec {
		vec[i] = 0.5
	}

	search := func(scope RetrievalScope) map[string]bool {
		chunks, err := store.Search(ctx, vec, "en", 500, scope)
		if err != nil {
			t.Fatalf("search: %v", err)
		}

		got := make(map[string]bool, len(chunks))
		for _, c := range chunks {
			got[c.EntityID] = true
		}

		return got
	}

	assertScope := func(name string, scope RetrievalScope, want, notWant []pgtype.UUID) {
		t.Helper()

		got := search(scope)
		for _, id := range want {
			if !got[id.String()] {
				t.Errorf("%s: expected entity %s in results", name, id.String())
			}
		}

		for _, id := range notWant {
			if got[id.String()] {
				t.Errorf("%s: entity %s leaked into results", name, id.String())
			}
		}
	}

	memberScope := func(viewer pgtype.UUID) RetrievalScope {
		ids, err := store.ListMemberSpaceIDs(ctx, viewer)
		if err != nil {
			t.Fatalf("ListMemberSpaceIDs: %v", err)
		}

		return RetrievalScope{ViewerID: viewer, MemberSpaceIDs: ids}
	}

	t.Run("anonymous sees public only", func(t *testing.T) {
		assertScope("anonymous", RetrievalScope{},
			[]pgtype.UUID{fx.cPublic, fx.pubSpace},
			[]pgtype.UUID{fx.cMembers, fx.cPrivate, fx.cPrivSpace, fx.privSpace, fx.thread})
	})

	t.Run("authenticated outsider sees public+members", func(t *testing.T) {
		assertScope("outsider", memberScope(fx.outsiderID),
			[]pgtype.UUID{fx.cPublic, fx.cMembers, fx.pubSpace},
			[]pgtype.UUID{fx.cPrivate, fx.cPrivSpace, fx.privSpace, fx.thread})
	})

	t.Run("author sees own private content and thread", func(t *testing.T) {
		assertScope("author", memberScope(fx.authorID),
			[]pgtype.UUID{fx.cPublic, fx.cMembers, fx.cPrivate, fx.cPrivSpace, fx.pubSpace, fx.thread},
			[]pgtype.UUID{fx.privSpace}) // author is not a member of the space
	})

	t.Run("approved member sees private space+thread, not author-private content", func(t *testing.T) {
		assertScope("member", memberScope(fx.memberID),
			[]pgtype.UUID{fx.cPublic, fx.cMembers, fx.pubSpace, fx.privSpace, fx.thread},
			[]pgtype.UUID{fx.cPrivate, fx.cPrivSpace})
	})

	t.Run("pending member gains nothing", func(t *testing.T) {
		assertScope("pending", memberScope(fx.pendingID),
			[]pgtype.UUID{fx.cPublic, fx.cMembers, fx.pubSpace},
			[]pgtype.UUID{fx.cPrivate, fx.cPrivSpace, fx.privSpace, fx.thread})
	})

	t.Run("privileged sees everything", func(t *testing.T) {
		scope := memberScope(fx.outsiderID)
		scope.CanSeePrivate = true
		assertScope("privileged", scope,
			[]pgtype.UUID{fx.cPublic, fx.cMembers, fx.cPrivate, fx.cPrivSpace, fx.pubSpace, fx.privSpace, fx.thread},
			nil)
	})
}

// TestScopedRetrievalGeoFiltered exercises the content-only geo path:
// private content inside a member's space must NOT surface (content
// visibility is self-contained — no space-membership join for content).
func TestScopedRetrievalGeoFiltered(t *testing.T) {
	dsn := os.Getenv("JC_DATABASE_URL")
	if dsn == "" {
		t.Skip("JC_DATABASE_URL not set; skipping integration test")
	}

	database, err := db.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}

	t.Cleanup(database.Writer.Close)
	t.Cleanup(database.Reader.Close)

	ctx := t.Context()
	fx := seedScopeFixture(t, ctx, database)
	store := NewStore(database)

	vec := make([]float32, 1536)
	for i := range vec {
		vec[i] = 0.5
	}

	// cPublic and cPrivSpace are both geo-tagged to the private space.
	// The member is approved for that space, but geo-filtered content
	// retrieval has no member-space clause — only the content contract.
	ids, err := store.ListMemberSpaceIDs(ctx, fx.memberID)
	if err != nil {
		t.Fatalf("ListMemberSpaceIDs: %v", err)
	}

	chunks, err := store.SearchGeoFiltered(ctx, vec, "en", 500, fx.privSpace,
		RetrievalScope{ViewerID: fx.memberID, MemberSpaceIDs: ids})
	if err != nil {
		t.Fatalf("geo search: %v", err)
	}

	got := make(map[string]bool, len(chunks))
	for _, c := range chunks {
		got[c.EntityID] = true
	}

	if !got[fx.cPublic.String()] {
		t.Error("geo search: expected public content in geo space")
	}

	if got[fx.cPrivSpace.String()] {
		t.Error("geo search: author-private content leaked via member space")
	}

	if got[fx.thread.String()] || got[fx.privSpace.String()] {
		t.Error("geo search: non-content entity leaked into content-only path")
	}
}

// seedScopeFixture inserts the users/spaces/content/threads/embedding rows
// for the matrix and registers cascade cleanup. Embedding vectors are
// uniform (0.5 × 1536) — the test asserts set membership, not ranking.
func seedScopeFixture(t *testing.T, ctx context.Context, database *db.DB) scopeFixture {
	t.Helper()

	var fx scopeFixture

	mustID := func(query string, args ...any) pgtype.UUID {
		var id pgtype.UUID
		if err := database.Writer.QueryRow(ctx, query, args...).Scan(&id); err != nil {
			t.Fatalf("fixture insert: %v", err)
		}

		return id
	}

	newUser := func() pgtype.UUID {
		return mustID(`INSERT INTO users (id,email,username,profile_type,status,created_at,updated_at,follower_count,force_password_change)
			VALUES (gen_random_uuid(),gen_random_uuid()::text||'@test.local','rag_'||replace(gen_random_uuid()::text,'-',''),'regular','active',now(),now(),0,false) RETURNING id`)
	}

	fx.authorID = newUser()
	fx.memberID = newUser()
	fx.pendingID = newUser()
	fx.outsiderID = newUser()

	spaceTypeID := mustID(`SELECT id FROM space_types LIMIT 1`)

	newSpace := func(vis string) pgtype.UUID {
		return mustID(`INSERT INTO spaces (id,space_type_id,name,slug,level,visibility,status,created_at,updated_at)
			VALUES (gen_random_uuid(),$1,'ragscope_'||gen_random_uuid()::text,'ragscope-'||replace(gen_random_uuid()::text,'-',''),'custom',$2,'active',now(),now()) RETURNING id`,
			spaceTypeID, vis)
	}

	fx.privSpace = newSpace("private")
	fx.pubSpace = newSpace("public")

	mustExec := func(query string, args ...any) {
		if _, err := database.Writer.Exec(ctx, query, args...); err != nil {
			t.Fatalf("fixture exec: %v", err)
		}
	}

	mustExec(`INSERT INTO space_members (space_id,user_id,role,status) VALUES ($1,$2,'member','approved')`, fx.privSpace, fx.memberID)
	mustExec(`INSERT INTO space_members (space_id,user_id,role,status) VALUES ($1,$2,'member','pending')`, fx.privSpace, fx.pendingID)

	newContent := func(vis string, author, spaceID, geoID pgtype.UUID) pgtype.UUID {
		return mustID(`INSERT INTO contents (id,author_id,title,slug,status,visibility,space_id,geo_space_id,content_type,body,created_at,updated_at)
			VALUES (gen_random_uuid(),$1,'ragscope','ragscope-'||replace(gen_random_uuid()::text,'-',''),'published',$2,$3,$4,'discussion','x',now(),now()) RETURNING id`,
			author, vis, spaceID, geoID)
	}

	fx.cPublic = newContent("public", fx.authorID, pgtype.UUID{}, fx.privSpace)
	fx.cMembers = newContent("members", fx.authorID, pgtype.UUID{}, pgtype.UUID{})
	fx.cPrivate = newContent("private", fx.authorID, pgtype.UUID{}, pgtype.UUID{})
	fx.cPrivSpace = newContent("private", fx.authorID, fx.privSpace, fx.privSpace)

	fx.thread = mustID(`INSERT INTO forum_threads (id,space_id,author_user_id,title,slug,body,created_at,updated_at)
		VALUES (gen_random_uuid(),$1,$2,'ragscope thread','ragscope-'||replace(gen_random_uuid()::text,'-',''),'[]'::jsonb,now(),now()) RETURNING id`,
		fx.privSpace, fx.authorID)

	// Embedding rows mirror what IndexEntity would write — identical vectors
	// so every fixture is equally similar and the scope clause is the only
	// discriminator.
	embed := func(entityType string, entityID pgtype.UUID, vis string, spaceID, authorID pgtype.UUID) {
		mustExec(`INSERT INTO content_embeddings (entity_type,entity_id,chunk_index,chunk_text,embedding,language,visibility,space_id,author_id)
			VALUES ($1,$2,0,'ragscope chunk',array_fill(0.5::float4,ARRAY[1536])::vector,'en',$3,$4,$5)`,
			entityType, entityID, vis, spaceID, authorID)
	}

	embed("content", fx.cPublic, "public", pgtype.UUID{}, fx.authorID)
	embed("content", fx.cMembers, "members", pgtype.UUID{}, fx.authorID)
	embed("content", fx.cPrivate, "private", pgtype.UUID{}, fx.authorID)
	embed("content", fx.cPrivSpace, "private", fx.privSpace, fx.authorID)
	embed("space", fx.privSpace, "private", fx.privSpace, pgtype.UUID{})
	embed("space", fx.pubSpace, "public", fx.pubSpace, pgtype.UUID{})
	embed("forum_thread", fx.thread, "private", fx.privSpace, fx.authorID)

	t.Cleanup(func() {
		mustExec(`DELETE FROM content_embeddings WHERE entity_id = ANY($1::uuid[])`,
			[]pgtype.UUID{fx.cPublic, fx.cMembers, fx.cPrivate, fx.cPrivSpace, fx.privSpace, fx.pubSpace, fx.thread})
		mustExec(`DELETE FROM space_members WHERE space_id = ANY($1::uuid[])`, []pgtype.UUID{fx.privSpace, fx.pubSpace})
		mustExec(`DELETE FROM forum_threads WHERE id = $1`, fx.thread)
		mustExec(`DELETE FROM contents WHERE id = ANY($1::uuid[])`,
			[]pgtype.UUID{fx.cPublic, fx.cMembers, fx.cPrivate, fx.cPrivSpace})
		mustExec(`DELETE FROM spaces WHERE id = ANY($1::uuid[])`, []pgtype.UUID{fx.privSpace, fx.pubSpace})

		for _, u := range []pgtype.UUID{fx.authorID, fx.memberID, fx.pendingID, fx.outsiderID} {
			mustExec(`DELETE FROM users WHERE id = $1`, u)
		}
	})

	return fx
}
