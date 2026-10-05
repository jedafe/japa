package spaces

// §80 Wiki-Only Space Types — pure unit coverage for the forced permission
// axes and the cross-package sentinel marker, plus DB-backed coverage for
// IsWikiOnlyType, axis coercion, and post denial.

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

func TestWikiOnlyPermAxes(t *testing.T) {
	a := WikiOnlyPermAxes()
	if a.Visibility != VisibilityPublic ||
		a.Posting != PermAdmins || a.Replying != PermAdmins || a.Commenting != PermAdmins ||
		a.Joining != JoiningInvite {
		t.Fatalf("unexpected wiki-only axes: %+v", a)
	}
}

func TestErrWikiOnlySpace_Marker(t *testing.T) {
	var m interface{ WikiOnlySpace() bool }
	if !errors.As(ErrWikiOnlySpace, &m) {
		t.Fatal("ErrWikiOnlySpace must satisfy the WikiOnlySpace marker interface")
	}
}

// TestWikiOnlyType_Gating is an integration test over a real database: it
// creates a space type, flips wiki_only on it, and asserts that
//   - IsWikiOnlyType reports true (and false for a normal/invalid type),
//   - CreateSpaceWithOwner coerces permissive request axes to the
//     wiki-only axes,
//   - UpdateSpace coerces permissive axes on the stored space,
//   - CanPostInSpace denies even though posting was requested "open".
//
// The space/type/user rows are cleaned up on test teardown.
func TestWikiOnlyType_Gating(t *testing.T) {
	svc, database := linkTestService(t)
	ctx := t.Context()
	q := gen.New(database.Writer)

	st, err := q.CreateSpaceType(ctx, gen.CreateSpaceTypeParams{
		Name:     "WikiOnly Test Type",
		Slug:     "wiki-only-test",
		IsSystem: false,
	})
	if err != nil {
		t.Fatalf("create space type: %v", err)
	}
	t.Cleanup(func() {
		_, _ = database.Writer.Exec(ctx, `DELETE FROM space_types WHERE id = $1`, st.ID)
	})

	wikiOnly := true
	st, err = q.UpdateSpaceType(ctx, gen.UpdateSpaceTypeParams{
		Name:     st.Name,
		WikiOnly: &wikiOnly,
		ID:       st.ID,
	})
	if err != nil {
		t.Fatalf("set wiki_only: %v", err)
	}
	if !st.WikiOnly {
		t.Fatal("space type should be wiki_only after update")
	}

	if !svc.IsWikiOnlyType(ctx, st.ID) {
		t.Fatal("IsWikiOnlyType must return true for a wiki-only type")
	}
	if svc.IsWikiOnlyType(ctx, pgtype.UUID{}) {
		t.Fatal("IsWikiOnlyType must return false for an invalid type id")
	}

	// A creator user is required by CreateSpaceWithOwner.
	hash := "x"
	creatorID, err := q.CreateUser(ctx, gen.CreateUserParams{
		Email:        "wiki-only-test@example.invalid",
		Username:     "wiki-only-test",
		PasswordHash: &hash,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = database.Writer.Exec(ctx, `DELETE FROM users WHERE id = $1`, creatorID)
	})

	space, err := svc.CreateSpaceWithOwner(ctx, gen.CreateSpaceParams{
		SpaceTypeID:    st.ID,
		Name:           "Wiki Only Space",
		Slug:           "wiki-only-space-test",
		Level:          "custom",
		Visibility:     VisibilityPublic,
		PostingPerm:    PermOpen, // deliberately permissive — must be coerced
		ReplyingPerm:   PermOpen,
		CommentingPerm: PermOpen,
		JoiningPerm:    JoiningOpen,
		Status:         "active",
	}, creatorID, nil)
	if err != nil {
		t.Fatalf("create space: %v", err)
	}
	t.Cleanup(func() {
		_, _ = database.Writer.Exec(ctx, `DELETE FROM spaces WHERE id = $1`, space.ID)
	})

	if space.PostingPerm != PermAdmins || space.ReplyingPerm != PermAdmins ||
		space.CommentingPerm != PermAdmins || space.JoiningPerm != JoiningInvite {
		t.Fatalf("axes not coerced: posting=%s replying=%s commenting=%s joining=%s",
			space.PostingPerm, space.ReplyingPerm, space.CommentingPerm, space.JoiningPerm)
	}

	full, err := svc.GetSpace(ctx, space.ID)
	if err != nil {
		t.Fatalf("get space: %v", err)
	}
	if svc.CanPostInSpace(ctx, full, creatorID, nil, nil) {
		t.Fatal("CanPostInSpace must deny posts to a wiki-only space")
	}
	if svc.CanCommentInSpace(ctx, full, creatorID, nil, nil) {
		t.Fatal("CanCommentInSpace must deny comments on a wiki-only space")
	}
	if svc.CanReplyInSpace(ctx, full, creatorID, nil, nil) {
		t.Fatal("CanReplyInSpace must deny replies on a wiki-only space")
	}

	// UpdateSpace must also coerce permissive axes on a wiki-only space.
	updated, err := svc.UpdateSpace(ctx, gen.UpdateSpaceParams{
		ID:             space.ID,
		Name:           space.Name,
		Visibility:     VisibilityPublic,
		PostingPerm:    PermOpen,
		ReplyingPerm:   PermOpen,
		CommentingPerm: PermOpen,
		JoiningPerm:    JoiningOpen,
		Status:         "active",
	})
	if err != nil {
		t.Fatalf("update space: %v", err)
	}
	if updated.PostingPerm != PermAdmins || updated.JoiningPerm != JoiningInvite {
		t.Fatalf("update did not coerce axes: posting=%s joining=%s",
			updated.PostingPerm, updated.JoiningPerm)
	}
}
