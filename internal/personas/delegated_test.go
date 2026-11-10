package personas

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type fakeBots struct {
	bot bool
	err error
}

func (f fakeBots) IsBotUser(context.Context, pgtype.UUID) (bool, error) {
	return f.bot, f.err
}

type fakePerms struct{ allow bool }

func (f fakePerms) IsSuperAdmin(context.Context, pgtype.UUID) bool { return false }
func (f fakePerms) HasPermission(context.Context, pgtype.UUID, string) bool {
	return f.allow
}

func TestResolveDelegatedActor_InvalidUUID(t *testing.T) {
	_, err := ResolveDelegatedActor(context.Background(), fakeBots{bot: true}, nil, pgtype.UUID{}, "not-a-uuid")
	if !errors.Is(err, ErrDelegatedInvalid) {
		t.Fatalf("err = %v, want ErrDelegatedInvalid", err)
	}
}

func TestResolveDelegatedActor_NoRBACForbidden(t *testing.T) {
	uid := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	_, err := ResolveDelegatedActor(context.Background(), fakeBots{bot: true}, nil, pgtype.UUID{}, uid.String())
	if !errors.Is(err, ErrDelegatedForbidden) {
		t.Fatalf("err = %v, want ErrDelegatedForbidden", err)
	}
}

func TestResolveDelegatedActor_MissingUserIsInvalid(t *testing.T) {
	uid := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	// pgx.ErrNoRows from the is_bot lookup means the target user doesn't
	// exist → invalid, not an internal error.
	_, err := ResolveDelegatedActor(context.Background(), fakeBots{err: pgx.ErrNoRows}, fakePerms{allow: true}, pgtype.UUID{}, uid.String())
	if !errors.Is(err, ErrDelegatedInvalid) {
		t.Fatalf("err = %v, want ErrDelegatedInvalid", err)
	}
}

func TestResolveDelegatedActor_NotBot(t *testing.T) {
	uid := pgtype.UUID{Bytes: [16]byte{3}, Valid: true}
	_, err := ResolveDelegatedActor(context.Background(), fakeBots{bot: false}, fakePerms{allow: true}, pgtype.UUID{}, uid.String())
	if !errors.Is(err, ErrDelegatedNotBot) {
		t.Fatalf("err = %v, want ErrDelegatedNotBot", err)
	}
}

func TestResolveDelegatedActor_BotOK(t *testing.T) {
	uid := pgtype.UUID{Bytes: [16]byte{4}, Valid: true}
	got, err := ResolveDelegatedActor(context.Background(), fakeBots{bot: true}, fakePerms{allow: true}, pgtype.UUID{}, uid.String())
	if err != nil || got != uid {
		t.Fatalf("got %v, %v — want %v, nil", got, err, uid)
	}
}

func TestWriteDelegatedError_Codes(t *testing.T) {
	for param, want := range map[string]string{"author_id": "invalid_author_id", "actor_id": "invalid_actor_id"} {
		rr := httptest.NewRecorder()
		WriteDelegatedError(rr, ErrDelegatedInvalid, param)
		if rr.Code != http.StatusBadRequest || !bytes.Contains(rr.Body.Bytes(), []byte(want)) {
			t.Fatalf("param %s: got %d %s", param, rr.Code, rr.Body.String())
		}
	}
}

func TestWriteDelegatedError_InternalMapsTo500(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteDelegatedError(rr, errors.New("db down"), "actor_id")
	if rr.Code != http.StatusInternalServerError || !bytes.Contains(rr.Body.Bytes(), []byte("internal_error")) {
		t.Fatalf("got %d %s", rr.Code, rr.Body.String())
	}
}
