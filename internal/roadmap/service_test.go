package roadmap

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/features"
	"github.com/jedafe/japa/internal/worker"
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

	if !errors.Is(ErrStatusDerived, errpkg.ErrStatusDerived) {
		t.Error("ErrStatusDerived must alias errpkg.ErrStatusDerived")
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

func TestIsSlugConflict(t *testing.T) {
	t.Parallel()

	if isSlugConflict(errors.New("some other error")) {
		t.Error("expected false for generic error")
	}

	// A content_hash violation names a different constraint — retrying with a
	// new slug cannot fix it, so the shadow-create loop must not retry.
	hashConflict := fmt.Errorf("creating roadmap shadow content: %w", errors.New(`ERROR: duplicate key value violates unique constraint "idx_contents_content_hash" (SQLSTATE 23505)`))
	if isSlugConflict(hashConflict) {
		t.Error("expected false for content_hash violation")
	}

	slugConflict := fmt.Errorf("creating roadmap shadow content: %w", errors.New(`ERROR: duplicate key value violates unique constraint "contents_slug_key" (SQLSTATE 23505)`))
	if !isSlugConflict(slugConflict) {
		t.Error("expected true for slug violation")
	}
}

// Regression test for the approval 500: the shadow contents row's hash must
// be scoped to the item so an empty body (or any body shared with another
// contents row) never collides under the global idx_contents unique index.
func TestShadowContentHashDistinguished(t *testing.T) {
	t.Parallel()

	var a, b gen.RoadmapItem

	a.ID.Scan("00000000-0000-0000-0000-000000000001")
	b.ID.Scan("00000000-0000-0000-0000-000000000002")

	if shadowContentHash(a, "") == content.ComputeContentHash("") {
		t.Error("empty-body hash must be distinguished from the global empty hash")
	}

	if shadowContentHash(a, "same body") == shadowContentHash(b, "same body") {
		t.Error("identical bodies on different items must not share a hash")
	}

	if shadowContentHash(a, "x") != shadowContentHash(a, "x") {
		t.Error("hash must be deterministic for the same item + body")
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

// boardFixture returns two rows (one planned, one shipped) and one category
// for buildBoard tests.
func boardFixture(t *testing.T) ([]gen.ListPublicRoadmapItemsRow, []gen.RoadmapCategory) {
	t.Helper()

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

	return rows, cats
}

func TestBuildBoardColumns(t *testing.T) {
	t.Parallel()

	rows, cats := boardFixture(t)
	board := buildBoard(rows, cats)

	// All five columns must render in order, even the empty ones.
	wantOrder := []struct {
		status string
		label  string
		items  int
	}{
		{"under_review", "Under Consideration", 0},
		{"planned", "Planned", 1},
		{"in_progress", "In Progress", 0},
		{"testing", "Testing", 0},
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
}

func TestBuildBoardItemMapping(t *testing.T) {
	t.Parallel()

	rows, cats := boardFixture(t)
	board := buildBoard(rows, cats)

	planned := board.Columns[1].Items[0]
	if planned.Title != "Planned thing" || planned.VoteCount != 7 || !planned.HasVoted {
		t.Errorf("planned item mapped incorrectly: %+v", planned)
	}

	if planned.ContentID == nil {
		t.Error("expected content_id pointer for valid ContentID")
	}

	shipped := board.Columns[4].Items[0]
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

// ---------------------------------------------------------------------------
// Flag phase → item status derivation (spec §4.2)
// ---------------------------------------------------------------------------

func TestFlagPhaseStatus(t *testing.T) {
	t.Parallel()

	cases := []struct {
		phase string
		want  ItemStatus
	}{
		{"planned", StatusPlanned},
		{"in_progress", StatusInProgress},
		{"testing", StatusTesting},
		{"shipped", StatusShipped},
		{"archived", StatusArchived},
		{"", StatusPlanned},      // undeclared → planned
		{"bogus", StatusPlanned}, // unknown → planned
	}

	for _, tc := range cases {
		if got := flagPhaseStatus(tc.phase); got != tc.want {
			t.Errorf("flagPhaseStatus(%q) = %q, want %q", tc.phase, got, tc.want)
		}
	}
}

func TestEffectiveFlagPhase(t *testing.T) {
	t.Parallel()

	// §4.1 defaults: enabled flags without a phase are live (shipped);
	// disabled flags without a phase are planned. Declared phases always win.
	if got := effectiveFlagPhase("", true); got != string(StatusShipped) {
		t.Errorf("effectiveFlagPhase(\"\", true) = %q, want shipped", got)
	}

	if got := effectiveFlagPhase("", false); got != string(StatusPlanned) {
		t.Errorf("effectiveFlagPhase(\"\", false) = %q, want planned", got)
	}

	if got := effectiveFlagPhase("testing", true); got != string(StatusTesting) {
		t.Errorf("effectiveFlagPhase(\"testing\", true) = %q, want testing", got)
	}

	if got := effectiveFlagPhase("bogus", false); got != string(StatusPlanned) {
		t.Errorf("effectiveFlagPhase(\"bogus\", false) = %q, want planned", got)
	}
}

// ---------------------------------------------------------------------------
// SyncFlagPhase integration (skipped without JC_DATABASE_URL)
// ---------------------------------------------------------------------------

// recordingEnqueuer captures enqueued task types for assertions.
type recordingEnqueuer struct {
	mu    sync.Mutex
	tasks []string
}

func (r *recordingEnqueuer) Enqueue(taskType string, _ []byte, _ ...interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tasks = append(r.tasks, taskType)

	return nil
}

func (r *recordingEnqueuer) count(taskType string) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	n := 0

	for _, tt := range r.tasks {
		if tt == taskType {
			n++
		}
	}

	return n
}

// uniqueTestKey returns a collision-free feature flag key for tests.
func uniqueTestKey(t *testing.T) string {
	t.Helper()

	return fmt.Sprintf("test.sync.%d", time.Now().UnixNano())
}

// flagSyncFixture bundles the live-DB state a SyncFlagPhase lifecycle test
// needs: the service under test, a throwaway flag key, and the shadow
// content slugs created along the way (for cleanup).
type flagSyncFixture struct {
	svc   *Service
	db    *db.DB
	enq   *recordingEnqueuer
	key   string
	slugs []string
	ids   []string
}

func newFlagSyncFixture(t *testing.T, database *db.DB) *flagSyncFixture {
	t.Helper()

	enq := &recordingEnqueuer{}
	f := &flagSyncFixture{
		svc: NewServiceWithEnqueuer(database, enq),
		db:  database,
		enq: enq,
		key: uniqueTestKey(t),
	}

	t.Cleanup(func() { f.cleanup(t) })

	return f
}

// cleanup removes every row the fixture touched: shadow contents rows
// first (roadmap_items.content_id FKs into contents), then items (by flag
// link and by tracked ID) and flag rows. Deletes ignore errors — the rows
// may already be gone.
func (f *flagSyncFixture) cleanup(t *testing.T) {
	t.Helper()

	_, _ = f.db.Writer.Exec(t.Context(), `DELETE FROM contents WHERE content_type='roadmap' AND slug = ANY($1::text[])`, f.slugs)
	_, _ = f.db.Writer.Exec(t.Context(), `DELETE FROM roadmap_items WHERE feature_flag_key = $1 OR id = ANY($2::uuid[])`, f.key, f.ids)
	_, _ = f.db.Writer.Exec(t.Context(), `DELETE FROM feature_flags WHERE key = $1`, f.key)
}

// writeFlag upserts the fixture's feature_flags row so the row the sync
// reads back matches the cfg the hook payload carries.
func (f *flagSyncFixture) writeFlag(t *testing.T, cfg features.FlagConfig, enabled bool) {
	t.Helper()

	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal cfg: %v", err)
	}

	if _, err := f.db.Writer.Exec(t.Context(), `
		INSERT INTO feature_flags (key, enabled, config) VALUES ($1, $2, $3)
		ON CONFLICT (key) DO UPDATE SET enabled = $2, config = $3, updated_at = now()
	`, f.key, enabled, raw); err != nil {
		t.Fatalf("write flag row: %v", err)
	}
}

func (f *flagSyncFixture) deleteFlag(t *testing.T) {
	t.Helper()

	if _, err := f.db.Writer.Exec(t.Context(), `DELETE FROM feature_flags WHERE key = $1`, f.key); err != nil {
		t.Fatalf("delete flag row: %v", err)
	}
}

func (f *flagSyncFixture) mustSync(t *testing.T, cfg features.FlagConfig) {
	t.Helper()

	if err := f.svc.SyncFlagPhase(t.Context(), f.key, cfg); err != nil {
		t.Fatalf("SyncFlagPhase: %v", err)
	}
}

// mustLinked returns the item linked to the fixture flag, asserting the
// link exists and the status matches want.
func (f *flagSyncFixture) mustLinked(t *testing.T, want ItemStatus) gen.RoadmapItem {
	t.Helper()

	item, err := f.svc.store.getItemByFlagKey(t.Context(), f.key)
	if err != nil {
		t.Fatalf("get linked item: %v", err)
	}

	if item.FeatureFlagKey == nil || *item.FeatureFlagKey != f.key {
		t.Fatalf("feature_flag_key = %v, want %q", item.FeatureFlagKey, f.key)
	}

	if item.Status != string(want) {
		t.Fatalf("status = %q, want %q", item.Status, want)
	}

	f.slugs = append(f.slugs, "roadmap-"+item.Slug)

	return item
}

// mustItem fetches an item by ID — needed once the flag link is gone and
// getItemByFlagKey can no longer find it.
func (f *flagSyncFixture) mustItem(t *testing.T, id pgtype.UUID) gen.RoadmapItem {
	t.Helper()

	item, err := f.svc.store.getItemByIDOnWrite(t.Context(), id)
	if err != nil {
		t.Fatalf("get item: %v", err)
	}

	return item
}

// mustCreate runs AdminCreateItem and tracks the new item (ID + shadow
// slug) for cleanup.
func (f *flagSyncFixture) mustCreate(t *testing.T, in AdminItemInput) gen.RoadmapItem {
	t.Helper()

	item, err := f.svc.AdminCreateItem(t.Context(), in)
	if err != nil {
		t.Fatalf("AdminCreateItem %q: %v", in.Title, err)
	}

	f.ids = append(f.ids, item.ID.String())
	f.slugs = append(f.slugs, "roadmap-"+item.Slug)

	return item
}

func (f *flagSyncFixture) mustNotifyCount(t *testing.T, want int) {
	t.Helper()

	if n := f.enq.count(worker.TypeRoadmapShipNotify); n != want {
		t.Fatalf("ship_notify enqueued %d times, want %d", n, want)
	}
}

// syncTestDB connects to the dedicated test database or skips. It reads
// JC_TEST_DATABASE_URL only — never JC_DATABASE_URL — so flag-sync fixtures
// can't leave test.sync.* rows in a dev or production feature_flags table
// (orphaned rows previously surfaced as phantom "Custom" extensions and in
// the public /api/v1/feature-flags map).
func syncTestDB(t *testing.T) *db.DB {
	t.Helper()

	dsn := os.Getenv("JC_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("JC_TEST_DATABASE_URL not set; skipping integration test")
	}

	database, err := db.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}

	t.Cleanup(func() {
		database.Writer.Close()
		database.Reader.Close()
	})

	return database
}

// TestSyncFlagPhaseIntegration exercises the full §4.3/§4.4 lifecycle
// against a real Postgres: create-on-enable, phase moves, archive on
// show_off, unlink on flag delete, the linked-status lock, and link
// validation. Skipped unless JC_TEST_DATABASE_URL is set.
func TestSyncFlagPhaseIntegration(t *testing.T) { //nolint:paralleltest // sequential DB lifecycle — later steps depend on earlier state
	database := syncTestDB(t)
	f := newFlagSyncFixture(t, database)

	cfg := features.FlagConfig{
		Name:          "Sync Test Feature",
		Description:   "Flag-synced roadmap entry",
		Phase:         string(StatusInProgress),
		ShowOnRoadmap: true,
	}
	f.writeFlag(t, cfg, true)

	// show_on_roadmap=true with no linked item → create with the
	// phase-derived status (§4.3).
	f.mustSync(t, cfg)

	item := f.mustLinked(t, StatusInProgress)

	if item.Title != "Sync Test Feature" {
		t.Errorf("title = %q, want flag name", item.Title)
	}

	// A phase write moves the linked item's status (§4.2).
	cfg.Phase = string(StatusTesting)
	f.writeFlag(t, cfg, true)
	f.mustSync(t, cfg)
	f.mustLinked(t, StatusTesting)

	// →shipped stamps shipped_at (SQL-side CASE) and fans out
	// roadmap:ship_notify exactly once (§4.4).
	cfg.Phase = string(StatusShipped)
	f.writeFlag(t, cfg, true)
	f.mustSync(t, cfg)

	item = f.mustLinked(t, StatusShipped)

	if !item.ShippedAt.Valid {
		t.Error("shipped_at not stamped on phase→shipped")
	}

	f.mustNotifyCount(t, 1)

	// Idempotent re-sync must not re-notify.
	f.mustSync(t, cfg)
	f.mustNotifyCount(t, 1)

	// Manual status changes on a linked item are locked — the flag's phase
	// owns status (§4.2 → 409 status_derived).
	if _, err := f.svc.AdminSetStatus(t.Context(), item.ID, StatusArchived, pgtype.UUID{}); !errors.Is(err, ErrStatusDerived) {
		t.Errorf("AdminSetStatus on linked item = %v, want ErrStatusDerived", err)
	}

	// show_on_roadmap=false archives the item but retains the link for
	// re-enable (§4.4).
	cfg.ShowOnRoadmap = false
	f.writeFlag(t, cfg, true)
	f.mustSync(t, cfg)
	f.mustLinked(t, StatusArchived)

	// Re-enable un-archives and applies the new phase — bypassing the
	// manual transition map (archived has no outgoing manual transitions).
	cfg.ShowOnRoadmap = true
	cfg.Phase = string(StatusPlanned)
	f.writeFlag(t, cfg, true)
	f.mustSync(t, cfg)
	f.mustLinked(t, StatusPlanned)

	// Deleting the flag row clears feature_flag_key; the item keeps its
	// last status and returns to manual control (§4.3).
	f.deleteFlag(t)
	f.mustSync(t, features.FlagConfig{})

	got := f.mustItem(t, item.ID)

	if got.FeatureFlagKey != nil {
		t.Errorf("feature_flag_key = %v after flag delete, want NULL", *got.FeatureFlagKey)
	}

	if got.Status != string(StatusPlanned) {
		t.Errorf("status after unlink = %q, want last status kept", got.Status)
	}

	// Linking an unknown flag key is rejected.
	bad := "test.nope.doesnotexist"

	if _, err := f.svc.AdminCreateItem(t.Context(), AdminItemInput{
		Title:          "Bad Link",
		FeatureFlagKey: &bad,
	}); !errors.Is(err, ErrUnknownFlagKey) {
		t.Errorf("AdminCreateItem bad flag key = %v, want ErrUnknownFlagKey", err)
	}
}

// TestFlagLinkLifecycleIntegration covers the admin-side link paths
// (§4.2): PATCH feature_flag_key validates + applies the flag's current
// phase immediately, "" unlinks, and manual transitions resume after
// unlink. Skipped unless JC_TEST_DATABASE_URL is set.
func TestFlagLinkLifecycleIntegration(t *testing.T) { //nolint:paralleltest // sequential DB lifecycle — later steps depend on earlier state
	f := newFlagSyncFixture(t, syncTestDB(t))
	f.writeFlag(t, features.FlagConfig{Phase: string(StatusTesting), ShowOnRoadmap: true}, true)

	manual := f.mustCreate(t, AdminItemInput{
		Title: "Link Me",
		Slug:  "link-me",
	})

	// Linking applies the flag's current phase immediately.
	if _, err := f.svc.AdminUpdateItem(t.Context(), manual.ID, AdminItemInput{FeatureFlagKey: &f.key}); err != nil {
		t.Fatalf("AdminUpdateItem link: %v", err)
	}

	f.mustLinked(t, StatusTesting)

	// Explicit unlink keeps the last status and restores manual control.
	empty := ""

	if _, err := f.svc.AdminUpdateItem(t.Context(), manual.ID, AdminItemInput{FeatureFlagKey: &empty}); err != nil {
		t.Fatalf("AdminUpdateItem unlink: %v", err)
	}

	got := f.mustItem(t, manual.ID)

	if got.FeatureFlagKey != nil {
		t.Errorf("feature_flag_key = %v after unlink, want NULL", *got.FeatureFlagKey)
	}

	if got.Status != string(StatusTesting) {
		t.Errorf("status after unlink = %q, want testing kept", got.Status)
	}

	if _, err := f.svc.AdminSetStatus(t.Context(), manual.ID, StatusArchived, pgtype.UUID{}); err != nil {
		t.Errorf("AdminSetStatus after unlink = %v, want nil", err)
	}
}

// TestFlagLinkGuardsIntegration covers the one-flag-one-item rule: a flag
// key already linked to a live item cannot be linked again — neither at
// create nor via PATCH — while re-setting the same key on the same item
// stays a no-op. Skipped unless JC_TEST_DATABASE_URL is set.
func TestFlagLinkGuardsIntegration(t *testing.T) { //nolint:paralleltest // sequential DB lifecycle — later steps depend on earlier state
	f := newFlagSyncFixture(t, syncTestDB(t))
	f.writeFlag(t, features.FlagConfig{Phase: string(StatusInProgress), ShowOnRoadmap: true}, true)

	first := f.mustCreate(t, AdminItemInput{Title: "Guard First", FeatureFlagKey: &f.key})

	if first.Status != string(StatusInProgress) {
		t.Errorf("linked create status = %q, want in_progress", first.Status)
	}

	// Creating a second item against the same flag is rejected.
	if _, err := f.svc.AdminCreateItem(t.Context(), AdminItemInput{
		Title:          "Guard Second",
		FeatureFlagKey: &f.key,
	}); !errors.Is(err, ErrFlagAlreadyLinked) {
		t.Errorf("AdminCreateItem with taken flag key = %v, want ErrFlagAlreadyLinked", err)
	}

	// So is linking an existing item to it.
	orphan := f.mustCreate(t, AdminItemInput{Title: "Guard Orphan"})

	if _, err := f.svc.AdminUpdateItem(t.Context(), orphan.ID, AdminItemInput{FeatureFlagKey: &f.key}); !errors.Is(err, ErrFlagAlreadyLinked) {
		t.Errorf("AdminUpdateItem to taken flag key = %v, want ErrFlagAlreadyLinked", err)
	}

	// Re-setting the same key on the already-linked item is a no-op, not a
	// conflict — the item is excluded from the taken check.
	if _, err := f.svc.AdminUpdateItem(t.Context(), first.ID, AdminItemInput{FeatureFlagKey: &f.key}); err != nil {
		t.Errorf("re-link same key on same item = %v, want nil", err)
	}
}

// TestFlagLinkHiddenFlagIntegration covers linking to a flag opted off the
// roadmap: the link is kept but the item stays archived (hidden) rather
// than taking the phase's public status — matching SyncFlagPhase's
// show_off path. Skipped unless JC_TEST_DATABASE_URL is set.
func TestFlagLinkHiddenFlagIntegration(t *testing.T) { //nolint:paralleltest // sequential DB lifecycle — later steps depend on earlier state
	f := newFlagSyncFixture(t, syncTestDB(t))
	f.writeFlag(t, features.FlagConfig{Phase: string(StatusInProgress), ShowOnRoadmap: false}, true)

	item := f.mustCreate(t, AdminItemInput{Title: "Hidden Link", FeatureFlagKey: &f.key})

	if item.Status != string(StatusArchived) {
		t.Errorf("status = %q on link to opted-out flag, want archived", item.Status)
	}

	if item.FeatureFlagKey == nil || *item.FeatureFlagKey != f.key {
		t.Errorf("feature_flag_key = %v, want %q retained", item.FeatureFlagKey, f.key)
	}

	// Opting the flag in lifts the linked item out of archived on the next
	// flag write — the retained link reconciles.
	cfg := features.FlagConfig{Phase: string(StatusInProgress), ShowOnRoadmap: true}
	f.writeFlag(t, cfg, true)
	f.mustSync(t, cfg)
	f.mustLinked(t, StatusInProgress)
}
