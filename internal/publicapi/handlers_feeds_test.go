package publicapi

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

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/feed"
	"github.com/jedafe/japa/internal/rag"
	"github.com/jedafe/japa/internal/scamalert"
	"github.com/jedafe/japa/internal/visitor"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// withChiParams injects chi URL params into a request — handlers are
// invoked directly in tests, bypassing the router.
func withChiParams(r *http.Request, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func withAPIKeyUser(r *http.Request, userID pgtype.UUID) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), apiKeyContextKey,
		&APIKeyContext{UserID: userID, Scopes: []string{"api.read.*"}}))
}

// ---------------------------------------------------------------------------
// Phase 2 reader stubs
// ---------------------------------------------------------------------------

type stubFeedReader struct {
	getFeedFn    func(ctx context.Context, timeline string, userID, spaceID, authorID *pgtype.UUID, sort string, limit, offset int32, vc, vr string) ([]feed.FeedItem, error)
	forYouFeedFn func(ctx context.Context, userID pgtype.UUID, sortMode string, limit, offset int32, vc *visitor.VisitorContext) ([]feed.FeedItem, error)
}

func (s stubFeedReader) GetFeed(ctx context.Context, timeline string, userID, spaceID, authorID *pgtype.UUID, sort string, limit, offset int32, vc, vr string) ([]feed.FeedItem, error) {
	if s.getFeedFn == nil {
		return nil, nil
	}
	return s.getFeedFn(ctx, timeline, userID, spaceID, authorID, sort, limit, offset, vc, vr)
}

func (s stubFeedReader) GetForYouFeed(ctx context.Context, userID pgtype.UUID, sortMode string, limit, offset int32, vc *visitor.VisitorContext) ([]feed.FeedItem, error) {
	if s.forYouFeedFn == nil {
		return nil, nil
	}
	return s.forYouFeedFn(ctx, userID, sortMode, limit, offset, vc)
}

type stubSafetyReader struct {
	listFn   func(ctx context.Context, destSpaceID pgtype.UUID, scamType string, limit, offset int32) ([]scamalert.ScamReportDTO, int64, error)
	getFn    func(ctx context.Context, id pgtype.UUID) (scamalert.ScamReportDTO, error)
	schemaFn func() scamalert.SchemaOptions
}

func (s stubSafetyReader) Enabled(_ context.Context) bool { return true }

func (s stubSafetyReader) Schema() scamalert.SchemaOptions {
	if s.schemaFn == nil {
		return scamalert.SchemaOptions{}
	}
	return s.schemaFn()
}

func (s stubSafetyReader) Get(ctx context.Context, id pgtype.UUID) (scamalert.ScamReportDTO, error) {
	if s.getFn == nil {
		return scamalert.ScamReportDTO{}, pgx.ErrNoRows
	}
	return s.getFn(ctx, id)
}

func (s stubSafetyReader) ListByOffset(ctx context.Context, destSpaceID pgtype.UUID, scamType string, limit, offset int32) ([]scamalert.ScamReportDTO, int64, error) {
	if s.listFn == nil {
		return nil, 0, nil
	}
	return s.listFn(ctx, destSpaceID, scamType, limit, offset)
}

type stubSemanticSearcher struct {
	retrieveFn func(ctx context.Context, query, language string, geoSpaceID *pgtype.UUID, viewerID pgtype.UUID) ([]rag.RetrievedChunk, error)
	embedder   bool
}

func (s stubSemanticSearcher) EmbedderConfigured() bool { return s.embedder }

func (s stubSemanticSearcher) ScopeHash(ctx context.Context, viewerID pgtype.UUID) string {
	return viewerID.String()
}

func (s stubSemanticSearcher) Retrieve(ctx context.Context, query, language string, geoSpaceID *pgtype.UUID, viewerID pgtype.UUID) ([]rag.RetrievedChunk, error) {
	if s.retrieveFn == nil {
		return nil, nil
	}
	return s.retrieveFn(ctx, query, language, geoSpaceID, viewerID)
}

// stubSettingsReader satisfies rateLimitSettingsReader (GetIntOr) and the
// structural boolSettingGetter (GetBoolOr) used for rag.enabled.
type stubSettingsReader struct {
	ints  map[string]int
	bools map[string]bool
}

func (s stubSettingsReader) GetIntOr(_ context.Context, key string, fallback int) int {
	if v, ok := s.ints[key]; ok {
		return v
	}
	return fallback
}

func (s stubSettingsReader) GetBoolOr(_ context.Context, key string, fallback bool) bool {
	if v, ok := s.bools[key]; ok {
		return v
	}
	return fallback
}

// ---------------------------------------------------------------------------
// gen.DBTX stub — backs gen.New() for tag-feed and markdown tests
// ---------------------------------------------------------------------------

type stubRow struct {
	scanFn func(dest ...any) error
}

func (r stubRow) Scan(dest ...any) error { return r.scanFn(dest...) }

func errRow(err error) stubRow {
	return stubRow{scanFn: func(_ ...any) error { return err }}
}

// assignScan fills dest pointers positionally from vals via reflection —
// sqlc-generated Scan calls pass one pointer per selected column.
func assignScan(vals []any) func(dest ...any) error {
	return func(dest ...any) error {
		for i := range dest {
			if i >= len(vals) {
				break
			}
			if vals[i] == nil {
				continue
			}
			dv := reflect.ValueOf(dest[i])
			if dv.Kind() != reflect.Ptr {
				return fmt.Errorf("assignScan: dest[%d] is %v (kind %v), not a pointer", i, dv.Type(), dv.Kind())
			}
			dv.Elem().Set(reflect.ValueOf(vals[i]))
		}
		return nil
	}
}

func rowOf(vals []any) stubRow {
	return stubRow{scanFn: assignScan(vals)}
}

type stubRows struct {
	vals [][]any
	idx  int
}

func (r *stubRows) Close()                                       {}
func (r *stubRows) Err() error                                   { return nil }
func (r *stubRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *stubRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *stubRows) Conn() *pgx.Conn                              { return nil }

func (r *stubRows) Next() bool {
	r.idx++
	return r.idx <= len(r.vals)
}

func (r *stubRows) Scan(dest ...any) error {
	return assignScan(r.vals[r.idx-1])(dest...)
}

func (r *stubRows) Values() ([]any, error) { return r.vals[r.idx-1], nil }
func (r *stubRows) RawValues() [][]byte    { return nil }

type stubDBTX struct {
	queryRowFn func(sql string, args ...any) pgx.Row
	queryFn    func(sql string, args ...any) (pgx.Rows, error)
}

func (s stubDBTX) Exec(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("stubDBTX: Exec not implemented")
}

func (s stubDBTX) Query(_ context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	if s.queryFn == nil {
		return nil, errors.New("stubDBTX: Query not implemented")
	}
	return s.queryFn(sql, args...)
}

func (s stubDBTX) QueryRow(_ context.Context, sql string, args ...interface{}) pgx.Row {
	if s.queryRowFn == nil {
		return errRow(errors.New("stubDBTX: QueryRow not implemented"))
	}
	return s.queryRowFn(sql, args...)
}

func (s stubDBTX) CopyFrom(_ context.Context, _ pgx.Identifier, _ []string, _ pgx.CopyFromSource) (int64, error) {
	return 0, errors.New("stubDBTX: CopyFrom not implemented")
}

// Row builders — values must match the generated Scan() order.

func tagRowVals(id pgtype.UUID) []any {
	return []any{
		id, "visas", "visas", (*string)(nil), (*string)(nil),
		pgtype.UUID{}, pgtype.UUID{}, (*string)(nil), int32(0), int32(0), false,
		[]byte(nil), pgtype.Timestamptz{}, pgtype.Timestamptz{},
		(*string)(nil), (*string)(nil), (*string)(nil), (*string)(nil),
	}
}

func tagContentRowVals(id pgtype.UUID, created time.Time) []any {
	ts := pgtype.Timestamptz{Time: created, Valid: true}
	return []any{
		id, pgtype.UUID{}, pgtype.UUID{}, "post", "Title", "slug-" + id.String()[:8],
		"body", "<p>body</p>", []byte(nil), (*string)(nil),
		pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{},
		(*string)(nil), (*string)(nil), (*string)(nil), (*string)(nil),
		true, "published", "public", false, false,
		int32(0), int32(5), int32(1), int32(0), int32(0),
		pgtype.Timestamptz{}, ts, ts, pgtype.Timestamptz{},
		(*string)(nil), (*string)(nil),
		"Jane Doe", "", "Canada", "canada", "excerpt", "visas,jobs",
	}
}

func publishedContentVals(id, authorID, spaceID pgtype.UUID, canonical *string) []any {
	return []any{
		id, authorID, pgtype.UUID{}, spaceID, "post", "Hello World", "hello-world",
		"# Body\n\nSome text", "<p>x</p>", []byte(nil), (*string)(nil), pgtype.UUID{},
		pgtype.UUID{}, pgtype.UUID{}, canonical, (*string)(nil), (*string)(nil),
		(*string)(nil), true, "published", "public", false, false,
		int32(3), int32(10), int32(2), int32(0), int32(0),
		pgtype.Timestamptz{}, pgtype.Timestamptz{Time: time.Now(), Valid: true},
		pgtype.Timestamptz{}, pgtype.Timestamptz{}, (*string)(nil),
	}
}

func spaceByIDVals(id pgtype.UUID, slug string) []any {
	return []any{
		id, pgtype.UUID{}, pgtype.UUID{}, "Canada", slug, (*string)(nil), []byte(nil),
		pgtype.UUID{}, pgtype.UUID{}, (*string)(nil), "country", int32(0), int32(0), int32(0),
		"public", "all", "all", "all", "all", "active",
		pgtype.Timestamptz{}, pgtype.Timestamptz{}, pgtype.Timestamptz{}, (*string)(nil),
	}
}

func publicUserVals(id pgtype.UUID, username string) []any {
	return []any{
		id, username, "individual", int32(0), (*string)(nil), (*string)(nil),
		(*string)(nil), (*string)(nil), pgtype.Timestamptz{}, int32(0),
	}
}

// ---------------------------------------------------------------------------
// Explore feed
// ---------------------------------------------------------------------------

func TestGetExploreFeedMapsNameToSort(t *testing.T) {
	var gotTimeline, gotSort string
	svc := NewService(nil)
	svc.SetFeedReader(stubFeedReader{
		getFeedFn: func(_ context.Context, timeline string, _, _, _ *pgtype.UUID, sort string, _, _ int32, _, _ string) ([]feed.FeedItem, error) {
			gotTimeline, gotSort = timeline, sort
			return []feed.FeedItem{{ID: "1", Title: "Post"}}, nil
		},
	})
	h := NewHandlers(svc)

	rec := httptest.NewRecorder()
	r := withChiParams(httptest.NewRequest(http.MethodGet, "/public/feeds/popular", nil), map[string]string{"name": "popular"})
	h.GetExploreFeed(rec, withAPIKey(r))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if gotTimeline != "explore" || gotSort != feed.SortTop {
		t.Fatalf("timeline/sort = %q/%q, want explore/%q", gotTimeline, gotSort, feed.SortTop)
	}
	if !strings.Contains(rec.Body.String(), `"data"`) {
		t.Fatalf("expected data envelope; body: %s", rec.Body.String())
	}
}

func TestGetExploreFeedSortOverride(t *testing.T) {
	var gotSort string
	svc := NewService(nil)
	svc.SetFeedReader(stubFeedReader{
		getFeedFn: func(_ context.Context, _ string, _, _, _ *pgtype.UUID, sort string, _, _ int32, _, _ string) ([]feed.FeedItem, error) {
			gotSort = sort
			return nil, nil
		},
	})
	h := NewHandlers(svc)

	// Valid sort overrides the name mapping.
	rec := httptest.NewRecorder()
	r := withChiParams(httptest.NewRequest(http.MethodGet, "/public/feeds/popular?sort=latest", nil), map[string]string{"name": "popular"})
	h.GetExploreFeed(rec, withAPIKey(r))
	if rec.Code != http.StatusOK || gotSort != feed.SortLatest {
		t.Fatalf("status = %d, sort = %q, want 200/%q", rec.Code, gotSort, feed.SortLatest)
	}

	// Invalid sort falls back to the name mapping.
	rec = httptest.NewRecorder()
	r = withChiParams(httptest.NewRequest(http.MethodGet, "/public/feeds/popular?sort=bogus", nil), map[string]string{"name": "popular"})
	h.GetExploreFeed(rec, withAPIKey(r))
	if rec.Code != http.StatusOK || gotSort != feed.SortTop {
		t.Fatalf("status = %d, sort = %q, want 200/%q", rec.Code, gotSort, feed.SortTop)
	}
}

func TestGetExploreFeedInvalidName(t *testing.T) {
	svc := NewService(nil)
	svc.SetFeedReader(stubFeedReader{})
	h := NewHandlers(svc)

	rec := httptest.NewRecorder()
	r := withChiParams(httptest.NewRequest(http.MethodGet, "/public/feeds/bogus", nil), map[string]string{"name": "bogus"})
	h.GetExploreFeed(rec, withAPIKey(r))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid_feed_name") {
		t.Fatalf("expected invalid_feed_name; body: %s", rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Tag feed
// ---------------------------------------------------------------------------

func TestGetTagFeedNotFound(t *testing.T) {
	svc := NewService(nil)
	svc.SetQueries(gen.New(stubDBTX{
		queryRowFn: func(_ string, _ ...any) pgx.Row {
			return errRow(pgx.ErrNoRows)
		},
	}))
	h := NewHandlers(svc)

	rec := httptest.NewRecorder()
	r := withChiParams(httptest.NewRequest(http.MethodGet, "/public/feeds/tag/nope", nil), map[string]string{"tag": "nope"})
	h.GetTagFeed(rec, withAPIKey(r))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "tag_not_found") {
		t.Fatalf("expected tag_not_found; body: %s", rec.Body.String())
	}
}

func TestGetTagFeedCursorRoundTrip(t *testing.T) {
	tagID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	created := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)

	newSvc := func(rows [][]any) *Service {
		svc := NewService(nil)
		svc.SetQueries(gen.New(stubDBTX{
			queryRowFn: func(sql string, _ ...any) pgx.Row {
				if strings.Contains(sql, "FROM tags t") {
					return rowOf(tagRowVals(tagID))
				}
				return errRow(pgx.ErrNoRows)
			},
			queryFn: func(_ string, _ ...any) (pgx.Rows, error) {
				return &stubRows{vals: rows}, nil
			},
		}))
		return svc
	}

	// Full page → next_cursor emitted.
	svc := newSvc([][]any{
		tagContentRowVals(pgtype.UUID{Bytes: [16]byte{2}, Valid: true}, created),
		tagContentRowVals(pgtype.UUID{Bytes: [16]byte{3}, Valid: true}, created.Add(-time.Hour)),
	})
	h := NewHandlers(svc)

	rec := httptest.NewRecorder()
	r := withChiParams(httptest.NewRequest(http.MethodGet, "/public/feeds/tag/visas?limit=2", nil), map[string]string{"tag": "visas"})
	h.GetTagFeed(rec, withAPIKey(r))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Data       []PublicFeedItem `json:"data"`
		NextCursor string           `json:"next_cursor"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.NextCursor == "" {
		t.Fatal("expected next_cursor on a full page")
	}
	if len(env.Data) != 2 {
		t.Fatalf("data len = %d, want 2", len(env.Data))
	}
	// Cursor round-trips through the decoder.
	ts, cid, err := decodeTagFeedCursor(env.NextCursor)
	if err != nil {
		t.Fatalf("decodeTagFeedCursor: %v", err)
	}
	if !ts.Time.Equal(created.Add(-time.Hour)) || cid.String() != (pgtype.UUID{Bytes: [16]byte{3}, Valid: true}).String() {
		t.Fatalf("cursor decoded to %v/%v", ts.Time, cid)
	}

	// Partial page → no next_cursor.
	svc = newSvc([][]any{
		tagContentRowVals(pgtype.UUID{Bytes: [16]byte{2}, Valid: true}, created),
	})
	h = NewHandlers(svc)

	rec = httptest.NewRecorder()
	r = withChiParams(httptest.NewRequest(http.MethodGet, "/public/feeds/tag/visas?limit=2", nil), map[string]string{"tag": "visas"})
	h.GetTagFeed(rec, withAPIKey(r))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	env = struct {
		Data       []PublicFeedItem `json:"data"`
		NextCursor string           `json:"next_cursor"`
	}{}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.NextCursor != "" {
		t.Fatalf("expected empty next_cursor on partial page; body: %s", rec.Body.String())
	}
}

func TestGetTagFeedBadCursor(t *testing.T) {
	svc := NewService(nil)
	svc.SetQueries(gen.New(stubDBTX{
		queryRowFn: func(sql string, _ ...any) pgx.Row {
			if strings.Contains(sql, "FROM tags t") {
				return rowOf(tagRowVals(pgtype.UUID{Bytes: [16]byte{1}, Valid: true}))
			}
			return errRow(pgx.ErrNoRows)
		},
	}))
	h := NewHandlers(svc)

	rec := httptest.NewRecorder()
	r := withChiParams(httptest.NewRequest(http.MethodGet, "/public/feeds/tag/visas?cursor=!!!not-base64!!!", nil), map[string]string{"tag": "visas"})
	h.GetTagFeed(rec, withAPIKey(r))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Space feed
// ---------------------------------------------------------------------------

func TestGetSpaceFeedNotFound(t *testing.T) {
	svc := NewService(nil)
	svc.SetFeedReader(stubFeedReader{})
	svc.SetMoatReaders(&MoatReaders{
		Spaces: stubSpaceReader{getBySlugFn: func(_ context.Context, _ string) (gen.GetPublicSpaceBySlugRow, error) {
			return gen.GetPublicSpaceBySlugRow{}, pgx.ErrNoRows
		}},
	})
	h := NewHandlers(svc)

	rec := httptest.NewRecorder()
	r := withChiParams(httptest.NewRequest(http.MethodGet, "/public/feeds/space/private-space", nil), map[string]string{"slug": "private-space"})
	h.GetSpaceFeed(rec, withAPIKey(r))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "space_not_found") {
		t.Fatalf("expected space_not_found; body: %s", rec.Body.String())
	}
}

func TestGetSpaceFeedPassesViewerID(t *testing.T) {
	viewerID := pgtype.UUID{Bytes: [16]byte{9, 9, 9}, Valid: true}
	spaceID := pgtype.UUID{Bytes: [16]byte{7}, Valid: true}

	var gotViewer, gotSpace *pgtype.UUID
	svc := NewService(nil)
	svc.SetFeedReader(stubFeedReader{
		getFeedFn: func(_ context.Context, _ string, userID, spaceID, _ *pgtype.UUID, _ string, _, _ int32, _, _ string) ([]feed.FeedItem, error) {
			gotViewer, gotSpace = userID, spaceID
			return []feed.FeedItem{{ID: "1"}}, nil
		},
	})
	svc.SetMoatReaders(&MoatReaders{
		Spaces: stubSpaceReader{getBySlugFn: func(_ context.Context, _ string) (gen.GetPublicSpaceBySlugRow, error) {
			return gen.GetPublicSpaceBySlugRow{ID: spaceID, Slug: "canada"}, nil
		}},
	})
	h := NewHandlers(svc)

	rec := httptest.NewRecorder()
	r := withChiParams(httptest.NewRequest(http.MethodGet, "/public/feeds/space/canada", nil), map[string]string{"slug": "canada"})
	h.GetSpaceFeed(rec, withAPIKeyUser(r, viewerID))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if gotViewer == nil || *gotViewer != viewerID {
		t.Fatalf("viewerID = %v, want key owner %v", gotViewer, viewerID)
	}
	if gotSpace == nil || *gotSpace != spaceID {
		t.Fatalf("spaceID = %v, want %v", gotSpace, spaceID)
	}
}

// ---------------------------------------------------------------------------
// Me feed
// ---------------------------------------------------------------------------

func TestGetMyFeedUsesKeyOwner(t *testing.T) {
	ownerID := pgtype.UUID{Bytes: [16]byte{4, 2, 1}, Valid: true}

	var gotUserID pgtype.UUID
	svc := NewService(nil)
	svc.SetFeedReader(stubFeedReader{
		forYouFeedFn: func(_ context.Context, userID pgtype.UUID, _ string, _, _ int32, _ *visitor.VisitorContext) ([]feed.FeedItem, error) {
			gotUserID = userID
			return []feed.FeedItem{{ID: "1", Title: "For you"}}, nil
		},
	})
	h := NewHandlers(svc)

	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/public/me/feed?sort=latest&limit=5", nil)
	h.GetMyFeed(rec, withAPIKeyUser(r, ownerID))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if gotUserID != ownerID {
		t.Fatalf("userID = %v, want key owner %v", gotUserID, ownerID)
	}
}

func TestGetMyFeedNoKey(t *testing.T) {
	h := NewHandlers(NewService(nil))
	rec := httptest.NewRecorder()
	h.GetMyFeed(rec, httptest.NewRequest(http.MethodGet, "/public/me/feed", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// Markdown representation
// ---------------------------------------------------------------------------

func newMarkdownHandlers(t *testing.T, canonical *string) (*Handlers, pgtype.UUID) {
	t.Helper()
	contentID := pgtype.UUID{Bytes: [16]byte{5, 5, 5}, Valid: true}
	authorID := pgtype.UUID{Bytes: [16]byte{6}, Valid: true}
	spaceID := pgtype.UUID{Bytes: [16]byte{7}, Valid: true}

	h := NewHandlers(NewService(nil))
	h.SetQueries(gen.New(stubDBTX{
		queryRowFn: func(sql string, _ ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "FROM contents"):
				return rowOf(publishedContentVals(contentID, authorID, spaceID, canonical))
			case strings.Contains(sql, "FROM spaces"):
				return rowOf(spaceByIDVals(spaceID, "canada"))
			case strings.Contains(sql, "FROM users u"):
				return rowOf(publicUserVals(authorID, "jane_doe"))
			default:
				return errRow(pgx.ErrNoRows)
			}
		},
	}))
	return h, contentID
}

func TestGetContentMarkdown(t *testing.T) {
	canonical := "https://japaconnect.com/content/hello-world"
	h, contentID := newMarkdownHandlers(t, &canonical)

	rec := httptest.NewRecorder()
	r := withChiParams(httptest.NewRequest(http.MethodGet, "/public/content/"+contentID.String()+"/markdown", nil),
		map[string]string{"id": contentID.String()})
	h.GetContentMarkdown(rec, withAPIKey(r))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/markdown") {
		t.Fatalf("Content-Type = %q, want text/markdown", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`title: "Hello World"`, `author: "jane_doe"`, `space: "canada"`,
		`canonical: "` + canonical + `"`, "# Body", "Some text",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("markdown missing %q; body:\n%s", want, body)
		}
	}
}

func TestGetContentByIDAcceptNegotiation(t *testing.T) {
	h, contentID := newMarkdownHandlers(t, nil)

	// Accept: text/markdown → markdown representation.
	rec := httptest.NewRecorder()
	r := withChiParams(httptest.NewRequest(http.MethodGet, "/public/content/"+contentID.String(), nil),
		map[string]string{"id": contentID.String()})
	r.Header.Set("Accept", "text/markdown")
	h.GetContentByID(rec, withAPIKey(r))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/markdown") {
		t.Fatalf("Content-Type = %q, want text/markdown", ct)
	}
	if !strings.Contains(rec.Body.String(), `canonical: "/content/hello-world"`) {
		t.Fatalf("expected slug-derived canonical; body:\n%s", rec.Body.String())
	}

	// Accept: application/json → JSON envelope (default preserved).
	rec = httptest.NewRecorder()
	r = withChiParams(httptest.NewRequest(http.MethodGet, "/public/content/"+contentID.String(), nil),
		map[string]string{"id": contentID.String()})
	r.Header.Set("Accept", "application/json")
	h.GetContentByID(rec, withAPIKey(r))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	if !strings.Contains(rec.Body.String(), `"data"`) {
		t.Fatalf("expected JSON data envelope; body: %s", rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Scam alerts
// ---------------------------------------------------------------------------

func TestListScamAlerts(t *testing.T) {
	var gotType string
	var gotLimit, gotOffset int32
	svc := NewService(nil)
	svc.SetSafetyReader(stubSafetyReader{
		listFn: func(_ context.Context, _ pgtype.UUID, scamType string, limit, offset int32) ([]scamalert.ScamReportDTO, int64, error) {
			gotType, gotLimit, gotOffset = scamType, limit, offset
			return []scamalert.ScamReportDTO{{
				ID:       "r1",
				ScamType: "fake_job",
				Status:   "confirmed",
			}}, 1, nil
		},
	})
	h := NewHandlers(svc)

	rec := httptest.NewRecorder()
	// Deliberately misaligned offset — the public contract is limit/offset
	// and must pass through verbatim (no page flooring).
	r := httptest.NewRequest(http.MethodGet, "/public/scam-alerts?scam_type=fake_job&limit=10&offset=25", nil)
	h.ListScamAlerts(rec, withAPIKey(r))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if gotType != "fake_job" || gotLimit != 10 || gotOffset != 25 {
		t.Fatalf("delegate args = %q/limit %d/offset %d, want fake_job/10/25", gotType, gotLimit, gotOffset)
	}
	if !strings.Contains(rec.Body.String(), `"total":1`) {
		t.Fatalf("expected total in envelope; body: %s", rec.Body.String())
	}
	// Public DTO must not leak moderation internals.
	if strings.Contains(rec.Body.String(), "moderator_note") || strings.Contains(rec.Body.String(), "reporter_id") {
		t.Fatalf("moderation internals leaked; body: %s", rec.Body.String())
	}
}

func TestListScamAlertsInvalidDestID(t *testing.T) {
	h := NewHandlers(NewService(nil))
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/public/scam-alerts?destination_space_id=not-a-uuid", nil)
	h.ListScamAlerts(rec, withAPIKey(r))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestGetScamAlertSchema(t *testing.T) {
	svc := NewService(nil)
	svc.SetSafetyReader(stubSafetyReader{
		schemaFn: func() scamalert.SchemaOptions {
			return scamalert.SchemaOptions{
				ScamTypes: []scamalert.Option{{Value: "fake_job", Label: "Fake job offer"}},
				Platforms: []scamalert.Option{{Value: "whatsapp", Label: "WhatsApp"}},
			}
		},
	})
	h := NewHandlers(svc)

	rec := httptest.NewRecorder()
	h.GetScamAlertSchema(rec, withAPIKey(httptest.NewRequest(http.MethodGet, "/public/scam-alerts/schema", nil)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "fake_job") || !strings.Contains(rec.Body.String(), "whatsapp") {
		t.Fatalf("schema options missing; body: %s", rec.Body.String())
	}
}

func TestGetScamAlert(t *testing.T) {
	svc := NewService(nil)
	svc.SetSafetyReader(stubSafetyReader{
		getFn: func(_ context.Context, _ pgtype.UUID) (scamalert.ScamReportDTO, error) {
			return scamalert.ScamReportDTO{ID: "r1", ScamType: "housing_fraud", Status: "confirmed"}, nil
		},
	})
	h := NewHandlers(svc)

	id := pgtype.UUID{Bytes: [16]byte{8}, Valid: true}
	rec := httptest.NewRecorder()
	r := withChiParams(httptest.NewRequest(http.MethodGet, "/public/scam-alerts/"+id.String(), nil),
		map[string]string{"id": id.String()})
	h.GetScamAlert(rec, withAPIKey(r))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "housing_fraud") {
		t.Fatalf("expected report body; got: %s", rec.Body.String())
	}
}

func TestGetScamAlertNotFound(t *testing.T) {
	svc := NewService(nil)
	svc.SetSafetyReader(stubSafetyReader{
		getFn: func(_ context.Context, _ pgtype.UUID) (scamalert.ScamReportDTO, error) {
			return scamalert.ScamReportDTO{}, pgx.ErrNoRows
		},
	})
	h := NewHandlers(svc)

	id := pgtype.UUID{Bytes: [16]byte{8}, Valid: true}
	rec := httptest.NewRecorder()
	r := withChiParams(httptest.NewRequest(http.MethodGet, "/public/scam-alerts/"+id.String(), nil),
		map[string]string{"id": id.String()})
	h.GetScamAlert(rec, withAPIKey(r))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "report_not_found") {
		t.Fatalf("expected report_not_found; body: %s", rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Semantic search
// ---------------------------------------------------------------------------

func TestSemanticSearchMapsResults(t *testing.T) {
	var gotLang string
	svc := NewService(nil)
	svc.SetSemanticSearcher(stubSemanticSearcher{
		embedder: true,
		retrieveFn: func(_ context.Context, _, language string, _ *pgtype.UUID, _ pgtype.UUID) ([]rag.RetrievedChunk, error) {
			gotLang = language
			// wiki_page hits are public-corpus — they surface without a
			// per-entity visibility resolver.
			return []rag.RetrievedChunk{
				{EntityType: "wiki_page", EntityID: "e1", ContentType: "wiki", ChunkText: "visa snippet", Similarity: 0.81},
				{EntityType: "learning_track", EntityID: "e2", ChunkText: "track snippet", Similarity: 0.72},
			}, nil
		},
	})
	h := NewHandlers(svc)

	rec := httptest.NewRecorder()
	h.SemanticSearch(rec, withAPIKey(httptest.NewRequest(http.MethodGet, "/public/search/semantic?q=visa&limit=10", nil)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if gotLang != "en" {
		t.Fatalf("lang = %q, want default en", gotLang)
	}
	body := rec.Body.String()
	for _, want := range []string{`"entity_type":"wiki_page"`, "visa snippet", `"cached":false`, `"query":"visa"`, `"language":"en"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q; body: %s", want, body)
		}
	}
}

// TestSemanticSearchDropsUnresolvableHits verifies the scoped-retrieval hit
// contract (§69 amendment): retrieval already filtered by viewer scope, so
// the resolver drops only entities that no longer resolve — deleted/inactive
// spaces, unpublished/deleted content, missing threads. A private-space hit
// that Retrieve returned is legitimate (the viewer is an approved member)
// and must be kept.
func TestSemanticSearchDropsUnresolvableHits(t *testing.T) {
	spaceID := pgtype.UUID{Bytes: [16]byte{7}, Valid: true}
	threadID := pgtype.UUID{Bytes: [16]byte{8}, Valid: true}
	svc := NewService(nil)
	svc.SetSemanticSearcher(stubSemanticSearcher{
		embedder: true,
		retrieveFn: func(_ context.Context, _ string, _ string, _ *pgtype.UUID, _ pgtype.UUID) ([]rag.RetrievedChunk, error) {
			return []rag.RetrievedChunk{
				// Member-visible space hit — kept: scoped retrieval already
				// authorized it for this viewer.
				{EntityType: "space", EntityID: spaceID.String(), ChunkText: "member space snippet", Similarity: 0.9},
				// Content that no longer resolves → dropped.
				{EntityType: "content", EntityID: pgtype.UUID{Bytes: [16]byte{9}, Valid: true}.String(), ChunkText: "unpublished snippet", Similarity: 0.8},
				// Deleted thread → dropped.
				{EntityType: "forum_thread", EntityID: threadID.String(), ChunkText: "dead thread snippet", Similarity: 0.75},
				{EntityType: "wiki_page", EntityID: "e3", ChunkText: "public wiki snippet", Similarity: 0.7},
			}, nil
		},
	})
	svc.SetQueries(gen.New(stubDBTX{
		queryRowFn: func(sql string, _ ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "FROM spaces"):
				// Space resolves and is active — visibility is irrelevant
				// here (retrieval scoped it); hit is kept.
				vals := spaceByIDVals(spaceID, "member-space")
				vals[14] = "members"
				return rowOf(vals)
			default:
				// Content + thread lookups miss → both dropped.
				return errRow(pgx.ErrNoRows)
			}
		},
	}))
	h := NewHandlers(svc)

	rec := httptest.NewRecorder()
	h.SemanticSearch(rec, withAPIKey(httptest.NewRequest(http.MethodGet, "/public/search/semantic?q=visa", nil)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "unpublished snippet") || strings.Contains(body, "dead thread snippet") {
		t.Fatalf("unresolvable entity snippets leaked; body: %s", body)
	}
	for _, want := range []string{"member space snippet", "member-space", "public wiki snippet"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q; body: %s", want, body)
		}
	}
}

func TestSemanticSearchMissingQuery(t *testing.T) {
	svc := NewService(nil)
	svc.SetSemanticSearcher(stubSemanticSearcher{embedder: true})
	h := NewHandlers(svc)

	rec := httptest.NewRecorder()
	h.SemanticSearch(rec, withAPIKey(httptest.NewRequest(http.MethodGet, "/public/search/semantic", nil)))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "missing_query") {
		t.Fatalf("expected missing_query; body: %s", rec.Body.String())
	}
}

func TestSemanticSearchNoEmbedder(t *testing.T) {
	svc := NewService(nil)
	svc.SetSemanticSearcher(stubSemanticSearcher{embedder: false})
	h := NewHandlers(svc)

	rec := httptest.NewRecorder()
	h.SemanticSearch(rec, withAPIKey(httptest.NewRequest(http.MethodGet, "/public/search/semantic?q=visa", nil)))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "search_unavailable") {
		t.Fatalf("expected search_unavailable; body: %s", rec.Body.String())
	}
}

func TestSemanticSearchRagDisabled(t *testing.T) {
	svc := NewService(nil)
	svc.SetSemanticSearcher(stubSemanticSearcher{embedder: true})
	svc.SetSettingsReader(stubSettingsReader{bools: map[string]bool{"rag.enabled": false}})
	h := NewHandlers(svc)

	rec := httptest.NewRecorder()
	h.SemanticSearch(rec, withAPIKey(httptest.NewRequest(http.MethodGet, "/public/search/semantic?q=visa", nil)))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "search_unavailable") {
		t.Fatalf("expected search_unavailable; body: %s", rec.Body.String())
	}
}
