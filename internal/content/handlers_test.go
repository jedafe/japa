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
	"time"

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
	return errRow(e)
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
	return structRow(d)
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

// ---------------------------------------------------------------------------
// UpdateContent — explicit-null clears vs preserve-on-omit
// ---------------------------------------------------------------------------

// captureUpdateDBTX answers the two writes UpdateContent performs (the UPDATE
// contents call — whose args are recorded for assertion — and the revision
// INSERT), dispatching on the sqlc name comment embedded in each query const.
// Any other query fails loudly so newly added writes surface in tests.
type captureUpdateDBTX struct {
	updateArgs []any
	updateRow  *gen.UpdateContentRow
}

func (d *captureUpdateDBTX) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("unexpected Exec in test")
}

func (d *captureUpdateDBTX) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return nil, errors.New("unexpected Query in test")
}

func (d *captureUpdateDBTX) QueryRow(_ context.Context, query string, args ...interface{}) pgx.Row {
	switch {
	case strings.Contains(query, "-- name: UpdateContent :one"):
		d.updateArgs = args
		return structRow{src: d.updateRow}
	case strings.Contains(query, "-- name: CreateContentRevision"):
		return structRow{src: &gen.ContentRevision{}}
	default:
		return errRow{err: fmt.Errorf("unexpected QueryRow in test: %.80s", query)}
	}
}

func (d *captureUpdateDBTX) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, errors.New("unexpected CopyFrom in test")
}

// Arg positions mirror the UpdateContentParams field order in the generated
// QueryRow call (internal/db/gen/contents.sql.go) — author_id is arg 0.
const (
	argAuthorID        = 0
	argCanonicalUrl    = 8 //nolint:revive // matches sqlc-generated field name
	argHeaderImageID   = 9
	argFeaturedImageID = 10
	argMetaTitle       = 11
	argMetaDescription = 12
	argOgImageUrl      = 13 //nolint:revive // matches sqlc-generated field name
	argPublishAt       = 18
	argOrganizationID  = 22
)

func argStringPtr(t *testing.T, args []any, i int) *string {
	t.Helper()

	v, ok := args[i].(*string)
	if !ok {
		t.Fatalf("arg %d is %T, want *string", i, args[i])
	}

	return v
}

func argUUID(t *testing.T, args []any, i int) pgtype.UUID {
	t.Helper()

	v, ok := args[i].(pgtype.UUID)
	if !ok {
		t.Fatalf("arg %d is %T, want pgtype.UUID", i, args[i])
	}

	return v
}

func argTimestamptz(t *testing.T, args []any, i int) pgtype.Timestamptz {
	t.Helper()

	v, ok := args[i].(pgtype.Timestamptz)
	if !ok {
		t.Fatalf("arg %d is %T, want pgtype.Timestamptz", i, args[i])
	}

	return v
}

// PATCH /content/{id} sends literal JSON nulls to clear nullable columns
// (the composer relies on this for cover removal, "post as yourself", SEO
// resets and unscheduling). These tests run the real handler into a stubbed
// write layer and assert the UPDATE args actually carry NULL — while an
// omitted key must still preserve the stored value.
func TestUpdateContentExplicitNullClears(t *testing.T) {
	userID := newUUID(t)
	contentID := newUUID(t)

	orgID := newUUID(t)
	headerID := newUUID(t)
	featuredID := newUUID(t)
	storedPublishAt := pgtype.Timestamptz{Time: time.Now().Add(48 * time.Hour).UTC(), Valid: true}
	storedCanonical := strPtr("https://example.com/canonical")
	storedMetaTitle := strPtr("stored meta title")
	storedMetaDesc := strPtr("stored meta description")
	storedOgImage := strPtr("https://example.com/og.png")

	prev := gen.GetContentByIDRow{
		ID:              contentID,
		AuthorID:        userID,
		ContentType:     TypeDiscussion,
		Title:           "Stored title",
		Slug:            "stored-slug",
		Body:            "stored body",
		BodyHtml:        "<p>stored body</p>",
		OrganizationID:  orgID,
		HeaderImageID:   headerID,
		FeaturedImageID: featuredID,
		CanonicalUrl:    storedCanonical,
		MetaTitle:       storedMetaTitle,
		MetaDescription: storedMetaDesc,
		OgImageUrl:      storedOgImage,
		Status:          StatusDraft,
		Visibility:      VisibilityPublic,
		PublishAt:       storedPublishAt,
	}

	updated := &gen.UpdateContentRow{
		ID:          contentID,
		AuthorID:    userID,
		ContentType: TypeDiscussion,
		Title:       "x",
		Slug:        "stored-slug",
		Body:        "stored body",
		BodyHtml:    "<p>stored body</p>",
		Status:      StatusDraft,
		Visibility:  VisibilityPublic,
	}

	patch := func(t *testing.T, write *captureUpdateDBTX, body string) *httptest.ResponseRecorder {
		t.Helper()

		h := &Handlers{Service: &Service{
			read:  gen.New(rowDBTX{src: &prev}),
			write: gen.New(write),
		}}
		rr := httptest.NewRecorder()
		req := listRequest(t, http.MethodPatch, "/api/v1/content/"+contentID.String(), body, userID, nil,
			map[string]string{"id": contentID.String()})
		h.UpdateContent(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body=%s)", rr.Code, rr.Body.String())
		}

		if len(write.updateArgs) == 0 {
			t.Fatal("UpdateContent write was not reached")
		}

		return rr
	}

	t.Run("explicit nulls write NULL", func(t *testing.T) {
		write := &captureUpdateDBTX{updateRow: updated}
		patch(t, write, `{"title":"x","featured_image_id":null,"header_image_id":null,"organization_id":null,"canonical_url":null,"meta_title":null,"meta_description":null,"og_image_url":null,"publish_at":null}`)

		if v := argStringPtr(t, write.updateArgs, argCanonicalUrl); v != nil {
			t.Errorf("canonical_url arg = %q, want nil (NULL write)", *v)
		}

		if v := argStringPtr(t, write.updateArgs, argMetaTitle); v != nil {
			t.Errorf("meta_title arg = %q, want nil (NULL write)", *v)
		}

		if v := argStringPtr(t, write.updateArgs, argMetaDescription); v != nil {
			t.Errorf("meta_description arg = %q, want nil (NULL write)", *v)
		}

		if v := argStringPtr(t, write.updateArgs, argOgImageUrl); v != nil {
			t.Errorf("og_image_url arg = %q, want nil (NULL write)", *v)
		}

		if v := argUUID(t, write.updateArgs, argFeaturedImageID); v.Valid {
			t.Errorf("featured_image_id arg = %v, want invalid (NULL write)", v)
		}

		if v := argUUID(t, write.updateArgs, argHeaderImageID); v.Valid {
			t.Errorf("header_image_id arg = %v, want invalid (NULL write)", v)
		}

		if v := argUUID(t, write.updateArgs, argOrganizationID); v.Valid {
			t.Errorf("organization_id arg = %v, want invalid (NULL write)", v)
		}

		if v := argTimestamptz(t, write.updateArgs, argPublishAt); v.Valid {
			t.Errorf("publish_at arg = %v, want invalid (NULL write)", v)
		}
	})

	t.Run("absent keys preserve stored values", func(t *testing.T) {
		write := &captureUpdateDBTX{updateRow: updated}
		patch(t, write, `{"title":"x"}`)

		if v := argStringPtr(t, write.updateArgs, argCanonicalUrl); v != storedCanonical {
			t.Errorf("canonical_url arg = %v, want stored %q", v, *storedCanonical)
		}

		if v := argStringPtr(t, write.updateArgs, argMetaTitle); v != storedMetaTitle {
			t.Errorf("meta_title arg = %v, want stored %q", v, *storedMetaTitle)
		}

		if v := argStringPtr(t, write.updateArgs, argMetaDescription); v != storedMetaDesc {
			t.Errorf("meta_description arg = %v, want stored %q", v, *storedMetaDesc)
		}

		if v := argStringPtr(t, write.updateArgs, argOgImageUrl); v != storedOgImage {
			t.Errorf("og_image_url arg = %v, want stored %q", v, *storedOgImage)
		}

		if v := argUUID(t, write.updateArgs, argFeaturedImageID); v != featuredID {
			t.Errorf("featured_image_id arg = %v, want stored %v", v, featuredID)
		}

		if v := argUUID(t, write.updateArgs, argHeaderImageID); v != headerID {
			t.Errorf("header_image_id arg = %v, want stored %v", v, headerID)
		}

		if v := argUUID(t, write.updateArgs, argOrganizationID); v != orgID {
			t.Errorf("organization_id arg = %v, want stored %v", v, orgID)
		}

		if v := argTimestamptz(t, write.updateArgs, argPublishAt); v != storedPublishAt {
			t.Errorf("publish_at arg = %v, want stored %v", v, storedPublishAt)
		}
	})

	t.Run("empty strings still clear uuid and schedule fields", func(t *testing.T) {
		write := &captureUpdateDBTX{updateRow: updated}
		patch(t, write, `{"title":"x","featured_image_id":"","organization_id":"","canonical_url":"","publish_at":""}`)

		if v := argUUID(t, write.updateArgs, argFeaturedImageID); v.Valid {
			t.Errorf("featured_image_id arg = %v, want invalid for empty-string clear", v)
		}

		if v := argUUID(t, write.updateArgs, argOrganizationID); v.Valid {
			t.Errorf("organization_id arg = %v, want invalid for empty-string clear", v)
		}

		if v := argTimestamptz(t, write.updateArgs, argPublishAt); v.Valid {
			t.Errorf("publish_at arg = %v, want invalid for empty-string clear", v)
		}
		// canonical_url "" stays an empty-string write (pre-existing contract —
		// only a literal null produces NULL).
		if v := argStringPtr(t, write.updateArgs, argCanonicalUrl); v == nil || *v != "" {
			t.Errorf("canonical_url arg = %v, want non-nil empty string", v)
		}
	})
}

// ---------------------------------------------------------------------------
// Author override (content.assign_author) — bots-only re-attribution
// ---------------------------------------------------------------------------

// authorAwareReadDBTX serves the read side of the PATCH author-override path:
// every query gets the canned struct row except the is_bot probe, which
// returns the configured flag (or botErr for a missing user).
type authorAwareReadDBTX struct {
	src    any
	isBot  bool
	botErr error
}

func (d authorAwareReadDBTX) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("unexpected Exec in test")
}

func (d authorAwareReadDBTX) Query(_ context.Context, query string, _ ...interface{}) (pgx.Rows, error) {
	// The publish transition's required-field check (validateRequiredFields)
	// lists custom-field groups + fields — return empty sets so the
	// stubbed path sees "no required fields" instead of a stub error.
	if strings.Contains(query, "custom_field") {
		return emptyRows{}, nil
	}
	return nil, errors.New("unexpected Query in test")
}

func (d authorAwareReadDBTX) QueryRow(_ context.Context, query string, _ ...interface{}) pgx.Row {
	if strings.Contains(query, "is_bot") {
		if d.botErr != nil {
			return errRow{err: d.botErr}
		}
		return boolRow(d.isBot)
	}
	return structRow{src: d.src}
}

func (d authorAwareReadDBTX) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, errors.New("unexpected CopyFrom in test")
}

// emptyRows is a zero-row pgx.Rows for stubbed :many queries.
type emptyRows struct{}

func (emptyRows) Close()                                       {}
func (emptyRows) Err() error                                   { return nil }
func (emptyRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (emptyRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (emptyRows) Next() bool                                   { return false }
func (emptyRows) Scan(...any) error                            { return errors.New("emptyRows: no rows") }
func (emptyRows) Values() ([]any, error)                       { return nil, nil }
func (emptyRows) RawValues() [][]byte                          { return nil }
func (emptyRows) Conn() *pgx.Conn                              { return nil }

type boolRow bool

func (r boolRow) Scan(dest ...any) error {
	b, ok := dest[0].(*bool)
	if !ok {
		return fmt.Errorf("boolRow dest is %T, want *bool", dest[0])
	}
	*b = bool(r)
	return nil
}

// PATCH /content/{id} {author_id} — the override requires
// content.assign_author and a bot persona target (users.is_bot). Absent or
// equal-to-stored values are no-ops; real users are rejected outright.
func TestUpdateContentAuthorOverride(t *testing.T) {
	userID := newUUID(t)
	botID := newUUID(t)
	otherID := newUUID(t)
	contentID := newUUID(t)

	prev := gen.GetContentByIDRow{
		ID:          contentID,
		AuthorID:    userID,
		ContentType: TypeDiscussion,
		Title:       "Stored title",
		Slug:        "stored-slug",
		Status:      StatusDraft,
		Visibility:  VisibilityPublic,
	}
	updated := &gen.UpdateContentRow{
		ID:          contentID,
		AuthorID:    botID,
		ContentType: TypeDiscussion,
		Title:       "Stored title",
		Slug:        "stored-slug",
		Status:      StatusDraft,
		Visibility:  VisibilityPublic,
	}

	patch := func(t *testing.T, read authorAwareReadDBTX, write *captureUpdateDBTX, body string, resolver *rbac.RBAC) *httptest.ResponseRecorder {
		t.Helper()

		h := &Handlers{Service: &Service{
			read:  gen.New(read),
			write: gen.New(write),
		}}
		rr := httptest.NewRecorder()
		req := listRequest(t, http.MethodPatch, "/api/v1/content/"+contentID.String(), body, userID, resolver,
			map[string]string{"id": contentID.String()})
		h.UpdateContent(rr, req)

		return rr
	}

	t.Run("malformed author_id rejected", func(t *testing.T) {
		write := &captureUpdateDBTX{updateRow: updated}
		rr := patch(t, authorAwareReadDBTX{src: &prev, isBot: true}, write,
			`{"author_id":"not-a-uuid"}`, newListTestResolver(t, userID, map[string]bool{"content.assign_author": true}))

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body=%s)", rr.Code, rr.Body.String())
		}
		if code := responseErrorCode(t, rr); code != "invalid_author_id" {
			t.Fatalf("error code = %q, want invalid_author_id", code)
		}
	})

	t.Run("override without assign_author denied", func(t *testing.T) {
		write := &captureUpdateDBTX{updateRow: updated}
		rr := patch(t, authorAwareReadDBTX{src: &prev, isBot: true}, write,
			`{"author_id":"`+botID.String()+`"}`, newListTestResolver(t, userID, nil))

		if rr.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403 (body=%s)", rr.Code, rr.Body.String())
		}
		if code := responseErrorCode(t, rr); code != "author_assign_forbidden" {
			t.Fatalf("error code = %q, want author_assign_forbidden", code)
		}
	})

	t.Run("non-bot target rejected", func(t *testing.T) {
		write := &captureUpdateDBTX{updateRow: updated}
		rr := patch(t, authorAwareReadDBTX{src: &prev, isBot: false}, write,
			`{"author_id":"`+otherID.String()+`"}`, newListTestResolver(t, userID, map[string]bool{"content.assign_author": true}))

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body=%s)", rr.Code, rr.Body.String())
		}
		if code := responseErrorCode(t, rr); code != "author_not_bot" {
			t.Fatalf("error code = %q, want author_not_bot", code)
		}
	})

	t.Run("unknown target rejected", func(t *testing.T) {
		write := &captureUpdateDBTX{updateRow: updated}
		rr := patch(t, authorAwareReadDBTX{src: &prev, botErr: pgx.ErrNoRows}, write,
			`{"author_id":"`+otherID.String()+`"}`, newListTestResolver(t, userID, map[string]bool{"content.assign_author": true}))

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body=%s)", rr.Code, rr.Body.String())
		}
		if code := responseErrorCode(t, rr); code != "invalid_author_id" {
			t.Fatalf("error code = %q, want invalid_author_id", code)
		}
	})

	t.Run("bot author reassigned", func(t *testing.T) {
		write := &captureUpdateDBTX{updateRow: updated}
		rr := patch(t, authorAwareReadDBTX{src: &prev, isBot: true}, write,
			`{"author_id":"`+botID.String()+`"}`, newListTestResolver(t, userID, map[string]bool{"content.assign_author": true}))

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body=%s)", rr.Code, rr.Body.String())
		}
		if len(write.updateArgs) == 0 {
			t.Fatal("UpdateContent write was not reached")
		}
		if v := argUUID(t, write.updateArgs, argAuthorID); v != botID {
			t.Errorf("author_id arg = %v, want %v", v, botID)
		}
	})

	t.Run("stored author is a no-op", func(t *testing.T) {
		write := &captureUpdateDBTX{updateRow: updated}
		// No assign_author grant — an author_id equal to the stored author
		// must not reach the permission gate at all.
		rr := patch(t, authorAwareReadDBTX{src: &prev, isBot: false}, write,
			`{"author_id":"`+userID.String()+`"}`, newListTestResolver(t, userID, nil))

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body=%s)", rr.Code, rr.Body.String())
		}
	})
}

// Bot-authored content bypasses the in_review queue — a story publish by a
// persona stays 'published' while the same write on a human-authored row
// clamps to 'in_review' (§79). Status is arg 15 in UpdateContentParams.
func TestUpdateContentBotAuthorSkipsReviewQueue(t *testing.T) {
	editorID := newUUID(t)
	botID := newUUID(t)
	contentID := newUUID(t)
	const argStatus = 15

	prev := func(author pgtype.UUID) *gen.GetContentByIDRow {
		return &gen.GetContentByIDRow{
			ID:          contentID,
			AuthorID:    author,
			ContentType: TypeStory,
			Title:       "Stored story",
			Slug:        "stored-story",
			Status:      StatusDraft,
			Visibility:  VisibilityPublic,
			// Publish transitions enforce content.min_length (default 10) —
			// an empty stored body fails before the §79 clamp runs.
			Body: "A stored story body long enough to publish.",
			// Publishing requires a primary space — a stored one skips the
			// content_spaces membership probe in the stubbed read layer.
			SpaceID: newUUID(t),
		}
	}
	updated := &gen.UpdateContentRow{ID: contentID, ContentType: TypeStory, Title: "x", Slug: "s"}

	patch := func(t *testing.T, read authorAwareReadDBTX, write *captureUpdateDBTX, callerID pgtype.UUID) *httptest.ResponseRecorder {
		t.Helper()

		h := &Handlers{Service: &Service{read: gen.New(read), write: gen.New(write)}}
		rr := httptest.NewRecorder()
		// The caller IS the author (ownership edit) — no elevated permissions,
		// so isStoryModerator is false and only the bot-author bypass can
		// skip the §79 clamp. content.publish is seeded because the
		// BUG-CE-001 status-transition gate requires it to PATCH
		// status → published.
		req := listRequest(t, http.MethodPatch, "/api/v1/content/"+contentID.String(), `{"status":"published"}`, callerID,
			newListTestResolver(t, callerID, map[string]bool{"content.publish": true}),
			map[string]string{"id": contentID.String()})
		h.UpdateContent(rr, req)

		return rr
	}

	t.Run("bot author publishes directly", func(t *testing.T) {
		write := &captureUpdateDBTX{updateRow: updated}
		rr := patch(t, authorAwareReadDBTX{src: prev(botID), isBot: true}, write, botID)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body=%s)", rr.Code, rr.Body.String())
		}
		if v, ok := write.updateArgs[argStatus].(string); !ok || v != StatusPublished {
			t.Errorf("status arg = %v, want %q (bot-authored story must not clamp)", write.updateArgs[argStatus], StatusPublished)
		}
	})

	t.Run("human author clamps to in_review", func(t *testing.T) {
		write := &captureUpdateDBTX{updateRow: updated}
		rr := patch(t, authorAwareReadDBTX{src: prev(editorID), isBot: false}, write, editorID)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body=%s)", rr.Code, rr.Body.String())
		}
		if v, ok := write.updateArgs[argStatus].(string); !ok || v != StatusInReview {
			t.Errorf("status arg = %v, want %q (human-authored story must clamp)", write.updateArgs[argStatus], StatusInReview)
		}
	})
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
