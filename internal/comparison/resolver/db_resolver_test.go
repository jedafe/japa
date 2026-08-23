package resolver

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// fakeRow implements pgx.Row for QueryRow tests.
type fakeRow struct {
	values []any
	called bool
	err    error
}

func (r *fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(r.values) == 0 {
		return pgx.ErrNoRows
	}
	if len(dest) != len(r.values) {
		return fmt.Errorf("destination count %d does not match value count %d", len(dest), len(r.values))
	}
	for i, v := range r.values {
		reflect.ValueOf(dest[i]).Elem().Set(reflect.ValueOf(v))
	}
	r.called = true
	return nil
}

// fakeRows implements pgx.Rows for Query tests.
type fakeRows struct {
	rows   [][]any
	pos    int
	closed bool
}

func (r *fakeRows) Close()                                    { r.closed = true }
func (r *fakeRows) Err() error                                { return nil }
func (r *fakeRows) CommandTag() pgconn.CommandTag             { return pgconn.CommandTag{} }
func (r *fakeRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *fakeRows) Next() bool {
	if r.closed {
		return false
	}
	r.pos++
	return r.pos <= len(r.rows)
}
func (r *fakeRows) Scan(dest ...any) error {
	if r.pos < 1 || r.pos > len(r.rows) {
		return errors.New("no current row")
	}
	row := r.rows[r.pos-1]
	if len(dest) != len(row) {
		return fmt.Errorf("destination count %d does not match value count %d", len(dest), len(row))
	}
	for i, v := range row {
		reflect.ValueOf(dest[i]).Elem().Set(reflect.ValueOf(v))
	}
	return nil
}
func (r *fakeRows) Values() ([]any, error)                        { return r.rows[r.pos-1], nil }
func (r *fakeRows) RawValues() [][]byte                           { return nil }
func (r *fakeRows) Conn() *pgx.Conn                               { return nil }

// fakeDBTX records the queries it receives and returns programmed rows.
type fakeDBTX struct {
	queryRowQueries []string
	queryRowArgs    [][]any
	queryRowResults []*fakeRow

	queryQueries []string
	queryArgs    [][]any
	queryRows    []*fakeRows
}

func (f *fakeDBTX) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (f *fakeDBTX) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	f.queryQueries = append(f.queryQueries, sql)
	f.queryArgs = append(f.queryArgs, args)
	if len(f.queryRows) == 0 {
		return &fakeRows{}, nil
	}
	rows := f.queryRows[0]
	f.queryRows = f.queryRows[1:]
	return rows, nil
}

func (f *fakeDBTX) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	f.queryRowQueries = append(f.queryRowQueries, sql)
	f.queryRowArgs = append(f.queryRowArgs, args)
	if len(f.queryRowResults) == 0 {
		return &fakeRow{err: pgx.ErrNoRows}
	}
	row := f.queryRowResults[0]
	f.queryRowResults = f.queryRowResults[1:]
	return row
}

func (f *fakeDBTX) CopyFrom(_ context.Context, _ pgx.Identifier, _ []string, _ pgx.CopyFromSource) (int64, error) {
	return 0, nil
}

func TestDBResolverResolveByID(t *testing.T) {
	tid := pgtype.UUID{Valid: true}
	db := &fakeDBTX{
		queryRowResults: []*fakeRow{{
			values: []any{
				"id-1", "RemitCo", "remitco", "https://remitco.example",
			},
		}},
	}

	r := NewDBResolver(db, DBConfig{
		Entity:       "remittance",
		Table:        "remittance_providers",
		IDColumn:     "id",
		TitleColumn:  "name",
		SlugColumn:   "slug",
		DescColumn:   "website_url",
		TenantColumn: "tenant_id",
		ActiveWhere:  "t.is_active = true",
	})

	item, found, err := r.Resolve(context.Background(), map[string]any{"id": "550e8400-e29b-41d4-a716-446655440000"}, tid)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if !found {
		t.Fatal("expected item to be found")
	}
	if item.ID != "id-1" || item.Title != "RemitCo" || item.Metadata["slug"] != "remitco" {
		t.Fatalf("unexpected item: %+v", item)
	}

	if len(db.queryRowQueries) != 1 {
		t.Fatalf("expected 1 QueryRow, got %d", len(db.queryRowQueries))
	}
	q := db.queryRowQueries[0]
	if !strContains(q, "remittance_providers") || !strContains(q, "tenant_id = $1") || !strContains(q, "is_active = true") {
		t.Fatalf("unexpected query: %s", q)
	}
}

func TestDBResolverResolveBySlug(t *testing.T) {
	tid := pgtype.UUID{Valid: true}
	db := &fakeDBTX{
		queryRowResults: []*fakeRow{{
			values: []any{
				"id-1", "OrgCo", "orgco", "A nice org",
			},
		}},
	}

	r := NewDBResolver(db, DBConfig{
		Entity:       "organization",
		Table:        "organizations",
		IDColumn:     "id",
		TitleColumn:  "name",
		SlugColumn:   "slug",
		DescColumn:   "description",
		TenantColumn: "tenant_id",
		ActiveWhere:  "t.verified = true",
	})

	item, found, err := r.Resolve(context.Background(), map[string]any{"slug": "orgco"}, tid)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if !found || item.ID != "id-1" {
		t.Fatalf("unexpected result: found=%v, item=%+v", found, item)
	}

	q := db.queryRowQueries[0]
	if !strContains(q, "t.slug = ") {
		t.Fatalf("expected slug predicate in query: %s", q)
	}
}

func TestDBResolverResolveMissingRef(t *testing.T) {
	r := NewDBResolver(&fakeDBTX{}, DBConfig{Entity: "test", Table: "test", IDColumn: "id", TitleColumn: "name"})
	_, found, err := r.Resolve(context.Background(), map[string]any{}, pgtype.UUID{})
	if err == nil || found {
		t.Fatal("expected error for missing ref")
	}
}

func TestDBResolverResolveNotFound(t *testing.T) {
	r := NewDBResolver(&fakeDBTX{
		queryRowResults: []*fakeRow{{err: pgx.ErrNoRows}},
	}, DBConfig{
		Entity:      "remittance",
		Table:       "remittance_providers",
		IDColumn:    "id",
		TitleColumn: "name",
	})

	_, found, err := r.Resolve(context.Background(), map[string]any{"id": "550e8400-e29b-41d4-a716-446655440000"}, pgtype.UUID{})
	if err != nil || found {
		t.Fatalf("expected not found without error, got found=%v err=%v", found, err)
	}
}

func TestDBResolverSearch(t *testing.T) {
	db := &fakeDBTX{
		queryRows: []*fakeRows{{
			rows: [][]any{
				{"id-1", "RemitCo", "remitco", ""},
				{"id-2", "SendIt", "sendit", ""},
			},
		}},
	}

	r := NewDBResolver(db, DBConfig{
		Entity:       "remittance",
		Table:        "remittance_providers",
		IDColumn:     "id",
		TitleColumn:  "name",
		SlugColumn:   "slug",
		TenantColumn: "tenant_id",
	})

	items, err := r.Search(context.Background(), "remit", nil, pgtype.UUID{Valid: true}, 10)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Title != "RemitCo" || items[1].Title != "SendIt" {
		t.Fatalf("unexpected items: %+v", items)
	}

	q := db.queryQueries[0]
	if !strContains(q, "remittance_providers") || !strContains(q, "ILIKE") || !strContains(q, "LIMIT") {
		t.Fatalf("unexpected query: %s", q)
	}
}

func TestDBResolverRejectsBadIdentifier(t *testing.T) {
	r := NewDBResolver(&fakeDBTX{}, DBConfig{
		Entity:      "test",
		Table:       "bad; table",
		IDColumn:    "id",
		TitleColumn: "name",
	})
	_, _, err := r.Resolve(context.Background(), map[string]any{"id": "550e8400-e29b-41d4-a716-446655440000"}, pgtype.UUID{})
	if err == nil {
		t.Fatal("expected error for invalid identifier")
	}
}

func strContains(s, substr string) bool {
	return strings.Contains(s, substr)
}
