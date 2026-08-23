package resolver

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type fakeResolver struct {
	resolveFunc func(ctx context.Context, ref map[string]any, tenantID pgtype.UUID) (ResolvedItem, bool, error)
	searchFunc  func(ctx context.Context, query string, filters map[string]any, tenantID pgtype.UUID, limit int32) ([]ResolvedItem, error)
}

func (f *fakeResolver) Resolve(ctx context.Context, ref map[string]any, tenantID pgtype.UUID) (ResolvedItem, bool, error) {
	return f.resolveFunc(ctx, ref, tenantID)
}

func (f *fakeResolver) Search(ctx context.Context, query string, filters map[string]any, tenantID pgtype.UUID, limit int32) ([]ResolvedItem, error) {
	return f.searchFunc(ctx, query, filters, tenantID, limit)
}

func TestRegistryResolve(t *testing.T) {
	reg := NewRegistry()
	reg.Register("test", &fakeResolver{
		resolveFunc: func(_ context.Context, ref map[string]any, _ pgtype.UUID) (ResolvedItem, bool, error) {
			id, _ := ref["id"].(string)
			return ResolvedItem{ID: id, Title: "Found"}, true, nil
		},
	})

	ctx := context.Background()
	item, err := reg.Resolve(ctx, map[string]any{"type": "test", "id": "abc"}, pgtype.UUID{})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if item.Type != "test" {
		t.Fatalf("expected type overwritten to 'test', got %q", item.Type)
	}
	if item.Title != "Found" || item.ID != "abc" {
		t.Fatalf("unexpected item: %+v", item)
	}
}

func TestRegistryResolveMissingType(t *testing.T) {
	reg := NewRegistry()
	ctx := context.Background()
	_, err := reg.Resolve(ctx, map[string]any{"id": "abc"}, pgtype.UUID{})
	if err == nil {
		t.Fatal("expected error for missing type")
	}
}

func TestRegistryResolveUnsupported(t *testing.T) {
	reg := NewRegistry()
	ctx := context.Background()
	_, err := reg.Resolve(ctx, map[string]any{"type": "missing", "id": "abc"}, pgtype.UUID{})
	if !errors.Is(err, fmt.Errorf("unsupported item_ref type: missing")) && err.Error() != "unsupported item_ref type: missing" {
		t.Fatalf("expected unsupported type error, got %v", err)
	}
}

func TestRegistryResolveNotFound(t *testing.T) {
	reg := NewRegistry()
	reg.Register("test", &fakeResolver{
		resolveFunc: func(_ context.Context, _ map[string]any, _ pgtype.UUID) (ResolvedItem, bool, error) {
			return ResolvedItem{}, false, nil
		},
	})

	ctx := context.Background()
	_, err := reg.Resolve(ctx, map[string]any{"type": "test", "id": "abc"}, pgtype.UUID{})
	if err == nil {
		t.Fatal("expected error for not found")
	}
}

func TestRegistrySearch(t *testing.T) {
	reg := NewRegistry()
	reg.Register("test", &fakeResolver{
		searchFunc: func(_ context.Context, query string, _ map[string]any, _ pgtype.UUID, limit int32) ([]ResolvedItem, error) {
			return []ResolvedItem{{ID: "1", Title: query}}, nil
		},
	})

	ctx := context.Background()
	items, err := reg.Search(ctx, "test", "q", nil, pgtype.UUID{}, 10)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(items) != 1 || items[0].Type != "test" {
		t.Fatalf("unexpected items: %+v", items)
	}
}

func TestParseUUID(t *testing.T) {
	valid := "550e8400-e29b-41d4-a716-446655440000"
	u, err := ParseUUID(valid)
	if err != nil {
		t.Fatalf("valid uuid failed: %v", err)
	}
	if !u.Valid {
		t.Fatal("expected valid uuid")
	}
	if u.String() != valid {
		t.Fatalf("expected %s, got %s", valid, u.String())
	}

	_, err = ParseUUID("not-a-uuid")
	if err == nil {
		t.Fatal("expected error for invalid uuid")
	}
}

func TestExcerpt(t *testing.T) {
	cases := []struct {
		input string
		max   int
		want  string
	}{
		{"hello world", 5, "hello…"},
		{"<p>hello world</p>", 20, "hello world"},
		{"{\"a\":\"hello\",\"b\":\"world\"}", 20, "hello world"},
	}

	for _, tc := range cases {
		got := Excerpt(tc.input, tc.max)
		if got != tc.want {
			t.Fatalf("Excerpt(%q, %d) = %q, want %q", tc.input, tc.max, got, tc.want)
		}
	}
}

func TestCoerceMap(t *testing.T) {
	m, err := CoerceMap([]byte(`{"type":"test","id":"1"}`))
	if err != nil {
		t.Fatalf("CoerceMap failed: %v", err)
	}
	if m["type"] != "test" {
		t.Fatalf("unexpected map: %+v", m)
	}

	m, err = CoerceMap(nil)
	if err != nil || m != nil {
		t.Fatalf("nil input should return nil map: %v, %v", m, err)
	}
}

func TestIsNotFound(t *testing.T) {
	if !IsNotFound(pgx.ErrNoRows) {
		t.Fatal("expected pgx.ErrNoRows to be not found")
	}
	if IsNotFound(errors.New("other")) {
		t.Fatal("expected other errors to not be not found")
	}
}

func TestQuoteIdent(t *testing.T) {
	cases := []struct {
		input string
		ok    bool
	}{
		{"valid_table", true},
		{"also_valid_2", true},
		{"with; injection", false},
		{"", false},
	}

	for _, tc := range cases {
		got, err := quoteIdent(tc.input)
		if tc.ok {
			if err != nil || got != tc.input {
				t.Fatalf("quoteIdent(%q) = %q, %v", tc.input, got, err)
			}
		} else {
			if err == nil {
				t.Fatalf("quoteIdent(%q) should have failed", tc.input)
			}
		}
	}
}
