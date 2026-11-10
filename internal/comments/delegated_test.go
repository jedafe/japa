package comments

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
)

// Delegated-authorship spec §1.2: author_id + is_anonymous are
// mutually_exclusive — a persona-attributed comment can't be anonymous.
// The check runs before any service/spam call, so a bare Handlers{} suffices.
func TestCreateComment_AuthorIDAndAnonymous_MutuallyExclusive(t *testing.T) {
	h := &Handlers{}
	contentID := pgtype.UUID{Bytes: [16]byte{9}, Valid: true}
	botID := pgtype.UUID{Bytes: [16]byte{8}, Valid: true}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/content/"+contentID.String()+"/comments",
		strings.NewReader(`{"author_id":"`+botID.String()+`","is_anonymous":true,"body":[123]}`))
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey,
		&auth.AuthenticatedUser{UserID: pgtype.UUID{Bytes: [16]byte{7}, Valid: true}}))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", contentID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.CreateComment(rr, req)
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "mutually_exclusive") {
		t.Fatalf("got %d %s", rr.Code, rr.Body.String())
	}
}
