package search

import (
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	gen "github.com/jedafe/japa/internal/db/gen"
)

// Regression: doc_pages.content is jsonb and can hold a quoted JSON string
// (e.g. `""` or a markdown blob). Before the '"'-case fix, tiptapPlainText
// returned the literal quotes, so search excerpts rendered as `""`.
func TestTiptapPlainTextJSONStringBody(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
		want string
	}{
		{"empty quoted string", `""`, ""},
		{"quoted markdown", `"# Hello World"`, "# Hello World"},
		{"quoted plain text", `"plain body text"`, "plain body text"},
		{"unquoted plain text", `plain body text`, "plain body text"},
		{"tiptap doc", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Hello"}]}]}`, "Hello"},
		{"html body", `<p>Hello <b>world</b></p>`, "Hello world"},
		{"empty body", ``, ""},
		{"bare quotes no close", `"unterminated`, `"unterminated`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tiptapPlainText(tc.body); got != tc.want {
				t.Fatalf("tiptapPlainText(%q) = %q, want %q", tc.body, got, tc.want)
			}
		})
	}
}

// Regression: ReindexAll must pass a VALID zero UUID as the AdminListSpaces
// space_type_id filter. The query's no-filter sentinel is
// "@space_type_id::uuid = '00000000-0000-0000-0000-000000000000'" — an
// invalid pgtype.UUID encodes as NULL, the comparison evaluates to NULL, and
// every space is filtered out (the spaces sweep silently indexed 0 rows).
// Requires a real Postgres; skipped without JC_DATABASE_URL.
func TestAdminListSpacesZeroUUIDSentinel(t *testing.T) {
	dsn := os.Getenv("JC_DATABASE_URL")
	if dsn == "" {
		t.Skip("JC_DATABASE_URL not set; skipping integration test")
	}

	database, err := db.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}

	defer database.Writer.Close()
	defer database.Reader.Close()

	q := gen.New(database.Reader)

	unfiltered, err := q.AdminListSpaces(t.Context(), gen.AdminListSpacesParams{
		SpaceTypeID: pgtype.UUID{Valid: true}, // valid zero UUID = "no filter"
		OffsetVal:   0,
		LimitVal:    1,
	})
	if err != nil {
		t.Fatalf("AdminListSpaces with zero-UUID sentinel: %v", err)
	}

	if len(unfiltered) == 0 {
		t.Fatal("AdminListSpaces returned 0 rows with the zero-UUID sentinel — spaces would never reindex")
	}

	nullFiltered, err := q.AdminListSpaces(t.Context(), gen.AdminListSpacesParams{
		SpaceTypeID: pgtype.UUID{}, // NULL — must filter everything out
		OffsetVal:   0,
		LimitVal:    1,
	})
	if err != nil {
		t.Fatalf("AdminListSpaces with NULL space_type_id: %v", err)
	}

	if len(nullFiltered) != 0 {
		t.Fatal("NULL space_type_id unexpectedly matched rows — the sentinel semantics changed; check ReindexAll")
	}
}
