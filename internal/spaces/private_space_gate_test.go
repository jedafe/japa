package spaces

// Private-space gate on GetSpace: the detail view must 404 for guests and
// outsiders while staying visible to owners, approved members, moderators,
// and static-perm holders. DB-backed via linkTestService.

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/content/visibility"
	gen "github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/literals"
)

type gateFakePermChecker struct{ allow bool }

func (f *gateFakePermChecker) HasPermission(context.Context, pgtype.UUID, string) bool {
	return f.allow
}

// GetSpace resolves literals through the content service unconditionally,
// so handler tests need a stub even though literals are irrelevant here.
type gateNoopContent struct{}

func (gateNoopContent) ListContentsBySpace(context.Context, pgtype.UUID, pgtype.Timestamptz, int32, visibility.VisibilityFilter) ([]gen.ListContentsBySpaceRow, error) {
	return nil, nil
}

func (gateNoopContent) SearchContentsBySpace(context.Context, pgtype.UUID, string, int32, int32, visibility.VisibilityFilter) ([]gen.SearchContentsBySpaceRow, error) {
	return nil, nil
}

func (gateNoopContent) PinContent(context.Context, pgtype.UUID, pgtype.UUID, pgtype.UUID, int32) (gen.SpacePinnedPost, error) {
	return gen.SpacePinnedPost{}, nil
}

func (gateNoopContent) UnpinContent(context.Context, pgtype.UUID, pgtype.UUID) error { return nil }

func (gateNoopContent) ListPinnedContent(context.Context, pgtype.UUID) ([]gen.SpacePinnedPost, error) {
	return nil, nil
}

func (gateNoopContent) ListPinnedContentBySpace(context.Context, pgtype.UUID, visibility.VisibilityFilter) ([]gen.ListPinnedContentBySpaceRow, error) {
	return nil, nil
}

func (gateNoopContent) ResolveContentFields(context.Context, any, literals.RenderContext) error {
	return nil
}

func TestPrivateSpaceGate_GetSpace(t *testing.T) {
	svc, database := linkTestService(t)
	ctx := t.Context()
	// t.Context() is cancelled before Cleanup funcs run — teardown
	// deletes need a live context or they silently no-op.
	bg := t.Context()
	q := gen.New(database.Writer)

	community, err := q.GetSpaceTypeBySlug(ctx, "community")
	if err != nil {
		t.Skipf("community space type missing; seed data required: %v", err)
	}

	uniq := time.Now().UnixNano()
	seq := 0
	mkUser := func(tag string) pgtype.UUID {
		t.Helper()

		seq++
		hash := "x"

		id, err := q.CreateUser(ctx, gen.CreateUserParams{
			Email:        fmt.Sprintf("priv-gate-%s-%d-%d@example.invalid", tag, uniq, seq),
			Username:     fmt.Sprintf("priv-gate-%s-%d-%d", tag, uniq, seq),
			PasswordHash: &hash,
		})
		if err != nil {
			t.Fatalf("create user %s: %v", tag, err)
		}

		t.Cleanup(func() {
			_, _ = database.Writer.Exec(bg, `DELETE FROM users WHERE id = $1`, id)
		})

		return id
	}

	mkSpace := func(vis, slug string, owner pgtype.UUID) gen.CreateSpaceRow {
		t.Helper()

		sp, err := svc.CreateSpaceWithOwner(ctx, gen.CreateSpaceParams{
			SpaceTypeID:    community.ID,
			Name:           "Gate " + slug,
			Slug:           slug,
			Level:          LevelCustom,
			Visibility:     vis,
			PostingPerm:    PermMembers,
			ReplyingPerm:   PermMembers,
			CommentingPerm: PermMembers,
			JoiningPerm:    JoiningOpen,
			Status:         StatusActive,
		}, owner, nil)
		if err != nil {
			t.Fatalf("create space %s: %v", slug, err)
		}

		t.Cleanup(func() {
			_, _ = database.Writer.Exec(bg, `DELETE FROM spaces WHERE id = $1`, sp.ID)
		})

		return sp
	}

	ownerID := mkUser("owner")
	memberID := mkUser("member")
	moderatorID := mkUser("moderator")
	outsiderID := mkUser("outsider")
	permID := mkUser("perm")

	private := mkSpace(VisibilityPrivate, fmt.Sprintf("priv-gate-%d", uniq), ownerID)

	if ok, err := svc.IsOwner(ctx, private.ID, ownerID); err != nil || !ok {
		t.Fatalf("creator must be owner (role owner, status approved): ok=%v err=%v", ok, err)
	}

	// AddSpaceMember leaves status at its 'approved' default.
	if err := q.AddSpaceMember(ctx, gen.AddSpaceMemberParams{SpaceID: private.ID, UserID: memberID}); err != nil {
		t.Fatalf("add member: %v", err)
	}

	// Moderator without membership isolates the moderator branch of the gate.
	if err := svc.AssignModerator(ctx, gen.AssignSpaceModeratorParams{
		SpaceID:          private.ID,
		UserID:           moderatorID,
		AssignedBy:       "space_owner",
		AssignedByUserID: ownerID,
	}); err != nil {
		t.Fatalf("assign moderator: %v", err)
	}

	public := mkSpace(VisibilityPublic, fmt.Sprintf("priv-gate-pub-%d", uniq), ownerID)

	deny := NewHandlers(svc, nil, gateNoopContent{})
	deny.SetPermissionChecker(&gateFakePermChecker{allow: false})
	allow := NewHandlers(svc, nil, gateNoopContent{})
	allow.SetPermissionChecker(&gateFakePermChecker{allow: true})

	callGetSpace := func(h *Handlers, slug string, viewer *pgtype.UUID) int {
		t.Helper()

		req := httptest.NewRequest(http.MethodGet, "/api/v1/spaces/"+slug, nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("slug", slug)

		rctxCtx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
		if viewer != nil {
			rctxCtx = context.WithValue(rctxCtx, auth.UserContextKey, &auth.AuthenticatedUser{UserID: *viewer})
		}

		rec := httptest.NewRecorder()
		h.GetSpace(rec, req.WithContext(rctxCtx))

		return rec.Code
	}

	cases := []struct {
		name   string
		h      *Handlers
		slug   string
		viewer *pgtype.UUID
		want   int
	}{
		{"guest on private", deny, private.Slug, nil, http.StatusNotFound},
		{"outsider on private", deny, private.Slug, &outsiderID, http.StatusNotFound},
		{"static perm denied on private", deny, private.Slug, &permID, http.StatusNotFound},
		{"member on private", deny, private.Slug, &memberID, http.StatusOK},
		{"owner on private", deny, private.Slug, &ownerID, http.StatusOK},
		{"moderator on private", deny, private.Slug, &moderatorID, http.StatusOK},
		{"static perm allowed on private", allow, private.Slug, &permID, http.StatusOK},
		{"guest on public control", deny, public.Slug, nil, http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := callGetSpace(tc.h, tc.slug, tc.viewer); got != tc.want {
				t.Fatalf("GetSpace %s: got %d want %d", tc.name, got, tc.want)
			}
		})
	}
}
