package roadmap

import (
	"encoding/base64"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// ---------------------------------------------------------------------------
// Service unit tests (pure logic, no DB)
// ---------------------------------------------------------------------------

func TestSentinelAliases(t *testing.T) {
	t.Parallel()

	// Aliases must stay wired to the shared errpkg sentinels so
	// HandleServiceError maps them without a package-local switch.
	if !errors.Is(ErrNotFound, errpkg.ErrNotFound) {
		t.Error("ErrNotFound must alias errpkg.ErrNotFound")
	}

	if !errors.Is(ErrInvalidTransition, errpkg.ErrInvalidStatusTransition) {
		t.Error("ErrInvalidTransition must alias errpkg.ErrInvalidStatusTransition")
	}

	if !errors.Is(ErrSubmissionsDisabled, errpkg.ErrFeatureDisabled) {
		t.Error("ErrSubmissionsDisabled must alias errpkg.ErrFeatureDisabled")
	}
}

func TestSentinelWrapping(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("%w: %s → %s", ErrInvalidTransition, StatusPending, StatusShipped)
	if !errors.Is(err, ErrInvalidTransition) {
		t.Error("wrapped ErrInvalidTransition not detected by errors.Is")
	}

	if !errors.Is(ErrAlreadyVoted, ErrAlreadyVoted) {
		t.Error("ErrAlreadyVoted sentinel broken")
	}

	if !errors.Is(ErrNotVoted, ErrNotVoted) {
		t.Error("ErrNotVoted sentinel broken")
	}
}

func TestShadowBodyText(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"plain text passthrough", "hello world", "hello world"},
		{"markdown passthrough", "# Title\n\nbody", "# Title\n\nbody"},
		{
			"base64 TipTap JSON decoded",
			base64.StdEncoding.EncodeToString([]byte(`{"type":"doc","content":[{"type":"paragraph"}]}`)),
			`{"type":"doc","content":[{"type":"paragraph"}]}`,
		},
		{
			"base64 of non-JSON passes through",
			base64.StdEncoding.EncodeToString([]byte("just text")),
			base64.StdEncoding.EncodeToString([]byte("just text")),
		},
	}

	for _, tc := range cases {
		if got := shadowBodyText(tc.in); got != tc.want {
			t.Errorf("%s: shadowBodyText(%q) = %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}
}

func TestIsUniqueViolation(t *testing.T) {
	t.Parallel()

	if isUniqueViolation(errors.New("some other error")) {
		t.Error("expected false for generic error")
	}

	wrapped := fmt.Errorf("creating roadmap shadow content: %w", errors.New(`ERROR: duplicate key value violates unique constraint "contents_slug_key" (SQLSTATE 23505)`))
	if !isUniqueViolation(wrapped) {
		t.Error("expected true for wrapped 23505 violation")
	}
}

func TestStrPtrOrNil(t *testing.T) {
	t.Parallel()

	if p := strPtrOrNil(""); p != nil {
		t.Error("expected nil for empty string")
	}

	if p := strPtrOrNil("x"); p == nil || *p != "x" {
		t.Error("expected non-nil pointer to \"x\"")
	}
}

func TestDeref(t *testing.T) {
	t.Parallel()

	if got := deref(nil); got != "" {
		t.Errorf("expected empty string for nil, got %q", got)
	}

	s := "body"
	if got := deref(&s); got != "body" {
		t.Errorf("expected \"body\", got %q", got)
	}
}

// ---------------------------------------------------------------------------
// Board assembly
// ---------------------------------------------------------------------------

func TestBuildBoard(t *testing.T) {
	t.Parallel()

	var itemID, catID pgtype.UUID
	if err := itemID.Scan("11111111-1111-1111-1111-111111111111"); err != nil {
		t.Fatal(err)
	}

	if err := catID.Scan("22222222-2222-2222-2222-222222222222"); err != nil {
		t.Fatal(err)
	}

	contentID := itemID
	summary := "A summary"
	catName := "Community"
	now := time.Now()

	rows := []gen.ListPublicRoadmapItemsRow{
		{
			ID:           itemID,
			Title:        "Planned thing",
			Slug:         "planned-thing",
			Summary:      &summary,
			Status:       string(StatusPlanned),
			VoteCount:    7,
			HasVoted:     true,
			CategoryID:   catID,
			CategoryName: &catName,
			ContentID:    contentID,
			CreatedAt:    pgtype.Timestamptz{Time: now, Valid: true},
		},
		{
			ID:        itemID,
			Title:     "Shipped thing",
			Slug:      "shipped-thing",
			Status:    string(StatusShipped),
			ShippedAt: pgtype.Timestamptz{Time: now, Valid: true},
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
	}

	cats := []gen.RoadmapCategory{
		{ID: catID, Slug: "community", Name: "Community", SortOrder: 1},
	}

	board := buildBoard(rows, cats)

	// All four columns must render in order, even the empty ones.
	wantOrder := []struct {
		status string
		label  string
		items  int
	}{
		{"under_review", "Under Consideration", 0},
		{"planned", "Planned", 1},
		{"in_progress", "In Progress", 0},
		{"shipped", "Recently Shipped", 1},
	}

	if len(board.Columns) != len(wantOrder) {
		t.Fatalf("board has %d columns, want %d", len(board.Columns), len(wantOrder))
	}

	for i, want := range wantOrder {
		col := board.Columns[i]
		if col.Status != want.status || col.Label != want.label || len(col.Items) != want.items {
			t.Errorf("column %d = (%q, %q, %d items), want (%q, %q, %d items)",
				i, col.Status, col.Label, len(col.Items), want.status, want.label, want.items)
		}
	}

	planned := board.Columns[1].Items[0]
	if planned.Title != "Planned thing" || planned.VoteCount != 7 || !planned.HasVoted {
		t.Errorf("planned item mapped incorrectly: %+v", planned)
	}

	if planned.ContentID == nil {
		t.Error("expected content_id pointer for valid ContentID")
	}

	shipped := board.Columns[3].Items[0]
	if shipped.ShippedAt == nil {
		t.Error("expected shipped_at pointer for valid ShippedAt")
	}

	if len(board.Categories) != 1 || board.Categories[0].Slug != "community" {
		t.Errorf("categories mapped incorrectly: %+v", board.Categories)
	}
}

func TestBuildBoardEmpty(t *testing.T) {
	t.Parallel()

	board := buildBoard(nil, nil)

	if len(board.Columns) != len(boardColumnOrder) {
		t.Fatalf("empty board has %d columns, want %d", len(board.Columns), len(boardColumnOrder))
	}

	for _, col := range board.Columns {
		if col.Items == nil {
			t.Errorf("column %q has nil Items — must serialize as [] not null", col.Status)
		}
	}

	if board.Categories == nil {
		t.Error("Categories must serialize as [] not null")
	}
}

// ---------------------------------------------------------------------------
// Input validation helpers (no DB — mirrors tickets/service_test.go style)
// ---------------------------------------------------------------------------

func TestSubmissionInputDefaults(t *testing.T) {
	t.Parallel()

	in := SubmissionInput{Title: "Add dark mode"}
	if in.CategoryID.Valid {
		t.Error("expected invalid CategoryID for unset category")
	}

	if in.UserID.Valid {
		t.Error("expected invalid UserID for unset user")
	}
}

func TestAdminItemInputDefaults(t *testing.T) {
	t.Parallel()

	in := AdminItemInput{Title: "Item"}
	if in.Status != "" {
		t.Error("expected empty Status — service defaults to under_review")
	}

	if in.SortOrder != nil || in.IsFlagship != nil || in.CategoryID != nil {
		t.Error("expected nil pointer fields for unset values")
	}
}

func TestAdminListFilterDefaults(t *testing.T) {
	t.Parallel()

	f := AdminListFilter{}
	if f.Status != "" || f.Search != "" || f.Sort != "" || f.IncludeDeleted {
		t.Error("expected zero-value filter fields")
	}
}
