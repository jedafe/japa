package content

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/rbac"
	"github.com/valkey-io/valkey-go"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// Handler-level tests for the §14 list amendment gates.
//
// NOTE: the list_feature_disabled path is NOT unit-testable here —
// config.Settings is a concrete struct whose zero value panics on
// GetBoolOr/GetIntOr, so handler tests construct Handlers with nil
// settings (the handler nil-guards it). The disabled-killswitch branch
// requires a live system_settings row and is covered by the manual smoke
// checklist instead.

// ---------------------------------------------------------------------------
// Stub gen.DBTX implementations
// ---------------------------------------------------------------------------

// errDBTX fails every query — used where the handler must reach the service
// layer (proving the RBAC gate did not fire) without a real database.
type errDBTX struct{ err error }

func (e errDBTX) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, e.err
}

func (e errDBTX) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return nil, e.err
}

func (e errDBTX) QueryRow(context.Context, string, ...interface{}) pgx.Row {
	return errRow{err: e.err}
}

func (e errDBTX) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, e.err
}

type errRow struct{ err error }

func (r errRow) Scan(...any) error { return r.err }

// rowDBTX returns a canned row for every QueryRow call. structRow copies the
// source struct's fields positionally into the generated Scan destinations —
// field order in the generated row struct matches the query's column order.
type rowDBTX struct{ src any }

func (d rowDBTX) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("unexpected Exec in test")
}

func (d rowDBTX) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return nil, errors.New("unexpected Query in test")
}

func (d rowDBTX) QueryRow(context.Context, string, ...interface{}) pgx.Row {
	return structRow{src: d.src}
}

func (d rowDBTX) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, errors.New("unexpected CopyFrom in test")
}

type structRow struct{ src any }

func (r structRow) Scan(dest ...any) error {
	sv := reflect.ValueOf(r.src)
	if sv.Kind() == reflect.Pointer {
		sv = sv.Elem()
	}
	if sv.NumField() < len(dest) {
		return fmt.Errorf("canned row has %d fields, scan wants %d", sv.NumField(), len(dest))
	}
	for i := range dest {
		reflect.ValueOf(dest[i]).Elem().Set(sv.Field(i))
	}
	return nil
}

// ---------------------------------------------------------------------------
// Request + resolver harness
// ---------------------------------------------------------------------------

// newListTestResolver builds an *rbac.RBAC with no working database: a lazy
// pgx pool pointed at a dead address fails every query closed (IsSuperAdmin
// → false; permission lookups → error → false). The only way a node resolves
// true is via the L2 permission cache, seeded from miniredis — pass perms=nil
// for a resolver that denies everything.
func newListTestResolver(t *testing.T, userID pgtype.UUID, perms map[string]bool) *rbac.RBAC {
	t.Helper()

	// pgxpool.New is lazy — the pool is returned without connecting and
	// queries fail fast at Acquire time (127.0.0.1:1 refuses instantly).
	pool, err := pgxpool.New(t.Context(), "postgres://127.0.0.1:1/postgres")
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)

	vk, err := valkey.NewClient(valkey.ClientOption{
		InitAddress:  []string{mr.Addr()},
		DisableCache: true,
	})
	if err != nil {
		t.Fatalf("valkey.NewClient: %v", err)
	}
	t.Cleanup(func() { vk.Close() })

	compat := valkeycompat.NewAdapter(vk)

	if perms != nil {
		data, err := json.Marshal(rbac.UserContext{Permissions: perms})
		if err != nil {
			t.Fatalf("marshal user context: %v", err)
		}
		if err := compat.Set(t.Context(), "jc:perm:"+userID.String(), data, 0).Err(); err != nil {
			t.Fatalf("seed permission cache: %v", err)
		}
	}

	return rbac.New(&db.DB{Reader: pool}, compat)
}

// listRequest builds an authenticated request carrying an optional RBAC
// resolver and optional chi URL params.
func listRequest(t *testing.T, method, target, body string, userID pgtype.UUID, resolver *rbac.RBAC, urlParams map[string]string) *http.Request {
	t.Helper()

	req := httptest.NewRequest(method, target, strings.NewReader(body))
	ctx := context.WithValue(req.Context(), auth.UserContextKey, &auth.AuthenticatedUser{UserID: userID})
	if resolver != nil {
		ctx = context.WithValue(ctx, rbac.ResolverKey, resolver)
	}
	if len(urlParams) > 0 {
		rctx := chi.NewRouteContext()
		for k, v := range urlParams {
			rctx.URLParams.Add(k, v)
		}
		ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
	}
	return req.WithContext(ctx)
}

func responseErrorCode(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("response is not an error envelope: %v (body=%s)", err, rr.Body.String())
	}
	return body.Error.Code
}

func newUUID(t *testing.T) pgtype.UUID {
	t.Helper()
	return pgtype.UUID{Bytes: uuid.New(), Valid: true}
}

// ---------------------------------------------------------------------------
// CreateContent — list.create / list.enabled gates
// ---------------------------------------------------------------------------

// A list create without the list.create node must be rejected with 403
// list_permission_required — both when no resolver is in context and when a
// resolver resolves no grant for the caller.
func TestCreateContentListPermissionDenied(t *testing.T) {
	userID := newUUID(t)
	body := `{"title":"My list","content_type":"list","status":"draft","field_values":{"list_mode":"numbered","items":[{"heading":"A"}]}}`

	t.Run("no resolver in context", func(t *testing.T) {
		h := &Handlers{}
		rr := httptest.NewRecorder()
		h.CreateContent(rr, listRequest(t, http.MethodPost, "/api/v1/content", body, userID, nil, nil))

		if rr.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403 (body=%s)", rr.Code, rr.Body.String())
		}
		if code := responseErrorCode(t, rr); code != "list_permission_required" {
			t.Fatalf("error code = %q, want list_permission_required", code)
		}
	})

	t.Run("resolver without list.create grant", func(t *testing.T) {
		resolver := newListTestResolver(t, userID, nil)
		h := &Handlers{}
		rr := httptest.NewRecorder()
		h.CreateContent(rr, listRequest(t, http.MethodPost, "/api/v1/content", body, userID, resolver, nil))

		if rr.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403 (body=%s)", rr.Code, rr.Body.String())
		}
		if code := responseErrorCode(t, rr); code != "list_permission_required" {
			t.Fatalf("error code = %q, want list_permission_required", code)
		}
	})
}

// With list.create granted the request must pass the RBAC gate and reach the
// service layer — which errors against the stub DBTX, yielding a 500 rather
// than any permission denial.
func TestCreateContentListPermissionGranted(t *testing.T) {
	userID := newUUID(t)
	resolver := newListTestResolver(t, userID, map[string]bool{"list.create": true})

	h := &Handlers{Service: &Service{
		read:  gen.New(errDBTX{err: errors.New("no database")}),
		write: gen.New(errDBTX{err: errors.New("no database")}),
	}}

	body := `{"title":"My list","content_type":"list","status":"draft","field_values":{"list_mode":"numbered","items":[{"heading":"A"}]}}`
	rr := httptest.NewRecorder()
	h.CreateContent(rr, listRequest(t, http.MethodPost, "/api/v1/content", body, userID, resolver, nil))

	if rr.Code == http.StatusForbidden {
		t.Fatalf("granted caller was denied: status = 403 (body=%s)", rr.Body.String())
	}
	if code := responseErrorCode(t, rr); code == "list_permission_required" || code == "list_feature_disabled" {
		t.Fatalf("gate error code %q for a granted caller", code)
	}
}

// The same gate must also run field_values validation once permission is
// granted: a bad list_mode produces 400 invalid_list_mode.
func TestCreateContentListInvalidMode(t *testing.T) {
	userID := newUUID(t)
	resolver := newListTestResolver(t, userID, map[string]bool{"list.create": true})

	h := &Handlers{}
	body := `{"title":"My list","content_type":"list","status":"draft","field_values":{"list_mode":"banana","items":[]}}`
	rr := httptest.NewRecorder()
	h.CreateContent(rr, listRequest(t, http.MethodPost, "/api/v1/content", body, userID, resolver, nil))

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", rr.Code, rr.Body.String())
	}
	if code := responseErrorCode(t, rr); code != "invalid_list_mode" {
		t.Fatalf("error code = %q, want invalid_list_mode", code)
	}
}

// ---------------------------------------------------------------------------
// UpdateContent — field_values validation on list edits
// ---------------------------------------------------------------------------

// A PATCH resubmitting an oversized items array on a list must be rejected
// with 400 too_many_items. The canned GetContentByID row marks the target as
// content_type=list; h.settings is nil so the default cap (50) applies.
func TestUpdateContentListTooManyItems(t *testing.T) {
	userID := newUUID(t)
	contentID := newUUID(t)

	prev := gen.GetContentByIDRow{
		ID:          contentID,
		AuthorID:    userID,
		ContentType: TypeList,
		Title:       "My list",
		Slug:        "my-list",
		Status:      StatusDraft,
		Visibility:  VisibilityPublic,
	}
	h := &Handlers{Service: &Service{read: gen.New(rowDBTX{src: &prev})}}

	items := make([]map[string]string, listDefaultMaxItems+1)
	for i := range items {
		items[i] = map[string]string{"heading": fmt.Sprintf("item %d", i+1)}
	}
	payload, err := json.Marshal(map[string]any{
		"field_values": map[string]any{"list_mode": "numbered", "items": items},
	})
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	req := listRequest(t, http.MethodPatch, "/api/v1/content/"+contentID.String(), string(payload), userID, nil,
		map[string]string{"id": contentID.String()})
	h.UpdateContent(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", rr.Code, rr.Body.String())
	}
	if code := responseErrorCode(t, rr); code != "too_many_items" {
		t.Fatalf("error code = %q, want too_many_items", code)
	}
}

// A PATCH with an oversized items array on a NON-list row must not trip the
// list validator — the guard is scoped to content_type=list.
func TestUpdateContentNonListSkipsListValidation(t *testing.T) {
	userID := newUUID(t)
	contentID := newUUID(t)

	prev := gen.GetContentByIDRow{
		ID:          contentID,
		AuthorID:    userID,
		ContentType: TypeDiscussion,
		Title:       "Thread",
		Slug:        "thread",
		Status:      StatusDraft,
		Visibility:  VisibilityPublic,
	}
	h := &Handlers{Service: &Service{
		read:  gen.New(rowDBTX{src: &prev}),
		write: gen.New(errDBTX{err: errors.New("no database")}),
	}}

	items := make([]map[string]string, listDefaultMaxItems+1)
	for i := range items {
		items[i] = map[string]string{"heading": fmt.Sprintf("item %d", i+1)}
	}
	payload, err := json.Marshal(map[string]any{
		"field_values": map[string]any{"items": items},
	})
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	req := listRequest(t, http.MethodPatch, "/api/v1/content/"+contentID.String(), string(payload), userID, nil,
		map[string]string{"id": contentID.String()})
	h.UpdateContent(rr, req)

	if rr.Code == http.StatusBadRequest {
		if code := responseErrorCode(t, rr); code == "too_many_items" || code == "invalid_list_items" || code == "invalid_list_mode" {
			t.Fatalf("non-list content hit the list validator: %s", code)
		}
	}
}
