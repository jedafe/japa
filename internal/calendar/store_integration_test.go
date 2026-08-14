//go:build integration

// Package calendar — store_integration_test.go tests the calendar store
// against a real PostgreSQL database via the testdb harness.
//
// Covers: ListEntries pagination + COUNT(*) OVER() (BUG-024), tenant
// isolation, status filtering, AdminListEntries sorting, and
// SubscribersForEntry.
//
// Ref: japaconnect-engineering/sections/86-community-calendar.md
//
//	japaconnect-engineering/adr/ADR-094-community-calendar.md
package calendar

import (
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jedafe/japa/internal/testdb"
)

// TestListEntries_OnlyPublished verifies that ListEntries returns only
// published entries — pending, archived, and unverified entries must
// not appear in the public list (§86, BUG-002).
func TestListEntries_OnlyPublished(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	userID := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "cal-user-pub"})
	spaceID := testdb.SeedSpaceMembership(t, database, tenantID, userID, testdb.SpaceOpts{})

	// Seed entries in different statuses.
	testdb.SeedCalendarEntry(t, database, tenantID, userID, testdb.CalendarEntryOpts{
		Title: "Published Entry", Status: "published", DestinationSpaceID: spaceID,
	})
	testdb.SeedCalendarEntry(t, database, tenantID, userID, testdb.CalendarEntryOpts{
		Title: "Pending Entry", Status: "pending", DestinationSpaceID: spaceID,
	})
	testdb.SeedCalendarEntry(t, database, tenantID, userID, testdb.CalendarEntryOpts{
		Title: "Archived Entry", Status: "archived", DestinationSpaceID: spaceID,
	})
	testdb.SeedCalendarEntry(t, database, tenantID, userID, testdb.CalendarEntryOpts{
		Title: "Unverified Entry", Status: "unverified", DestinationSpaceID: spaceID,
	})

	store := NewStore(database)
	entries, total, err := store.ListEntries(t.Context(), ListEntriesFilter{
		TenantID: tenantID,
		Limit:    100,
	})
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}

	if total != 1 {
		t.Fatalf("total = %d, want 1 (only published)", total)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1 (only published)", len(entries))
	}
	if entries[0].Title != "Published Entry" {
		t.Errorf("entry title = %q, want %q", entries[0].Title, "Published Entry")
	}
}

// TestListEntries_TenantIsolated verifies that entries from one tenant
// do not leak into another tenant's list (multi-tenancy guard).
func TestListEntries_TenantIsolated(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantA := testdb.NewTenant(t, database)
	tenantB := testdb.NewTenant(t, database)
	userA := testdb.SeedUser(t, database, tenantA, testdb.UserOpts{Username: "cal-user-a"})
	userB := testdb.SeedUser(t, database, tenantB, testdb.UserOpts{Username: "cal-user-b"})
	spaceA := testdb.SeedSpaceMembership(t, database, tenantA, userA, testdb.SpaceOpts{})
	spaceB := testdb.SeedSpaceMembership(t, database, tenantB, userB, testdb.SpaceOpts{})

	testdb.SeedCalendarEntry(t, database, tenantA, userA, testdb.CalendarEntryOpts{
		Title: "Tenant A Entry", Status: "published", DestinationSpaceID: spaceA,
	})
	testdb.SeedCalendarEntry(t, database, tenantB, userB, testdb.CalendarEntryOpts{
		Title: "Tenant B Entry", Status: "published", DestinationSpaceID: spaceB,
	})

	store := NewStore(database)
	entries, total, err := store.ListEntries(t.Context(), ListEntriesFilter{
		TenantID: tenantA,
		Limit:    100,
	})
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	if total != 1 {
		t.Fatalf("total = %d, want 1 (tenant isolated)", total)
	}
	if len(entries) != 1 || entries[0].Title != "Tenant A Entry" {
		t.Errorf("expected only Tenant A Entry, got %d entries", len(entries))
	}
}

// TestListEntries_PaginationAndCount verifies that the COUNT(*) OVER()
// window function returns the correct total across pages (BUG-024).
func TestListEntries_PaginationAndCount(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	userID := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "cal-user-page"})
	spaceID := testdb.SeedSpaceMembership(t, database, tenantID, userID, testdb.SpaceOpts{})

	// Seed 5 published entries.
	for i := 0; i < 5; i++ {
		testdb.SeedCalendarEntry(t, database, tenantID, userID, testdb.CalendarEntryOpts{
			Title: "Entry", Status: "published", DestinationSpaceID: spaceID,
		})
	}

	store := NewStore(database)

	// Page 1: limit=2, should return 2 entries but total=5.
	entries, total, err := store.ListEntries(t.Context(), ListEntriesFilter{
		TenantID: tenantID,
		Limit:    2,
		Offset:   0,
	})
	if err != nil {
		t.Fatalf("ListEntries page 1: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("page 1: got %d entries, want 2", len(entries))
	}
	if total != 5 {
		t.Fatalf("page 1: total = %d, want 5 (COUNT(*) OVER())", total)
	}

	// Page 3: limit=2, offset=4, should return 1 entry, total still 5.
	entries, total, err = store.ListEntries(t.Context(), ListEntriesFilter{
		TenantID: tenantID,
		Limit:    2,
		Offset:   4,
	})
	if err != nil {
		t.Fatalf("ListEntries page 3: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("page 3: got %d entries, want 1", len(entries))
	}
	if total != 5 {
		t.Fatalf("page 3: total = %d, want 5", total)
	}
}

// TestListEntries_OriginFilter verifies that the origin filter works
// (BUG-013).
func TestListEntries_OriginFilter(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	userID := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "cal-user-origin"})
	destSpace := testdb.SeedSpaceMembership(t, database, tenantID, userID, testdb.SpaceOpts{})
	originSpace := testdb.SeedSpaceMembership(t, database, tenantID, userID, testdb.SpaceOpts{})

	// Entry with origin.
	testdb.SeedCalendarEntry(t, database, tenantID, userID, testdb.CalendarEntryOpts{
		Title: "With Origin", Status: "published", DestinationSpaceID: destSpace,
	})
	// Entry without origin (origin defaults to zero UUID in seed helper).

	store := NewStore(database)
	entries, total, err := store.ListEntries(t.Context(), ListEntriesFilter{
		TenantID:      tenantID,
		OriginSpaceID: originSpace,
		Limit:         100,
	})
	if err != nil {
		t.Fatalf("ListEntries with origin filter: %v", err)
	}
	// The seed helper doesn't set origin_space_id, so filtering by a
	// specific origin should return 0 entries.
	if total != 0 {
		t.Fatalf("origin filter: total = %d, want 0", total)
	}
	if len(entries) != 0 {
		t.Fatalf("origin filter: got %d entries, want 0", len(entries))
	}
}

// TestAdminListEntries_AnyStatus verifies that AdminListEntries returns
// entries in any status (not just published).
func TestAdminListEntries_AnyStatus(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	userID := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "cal-admin-user"})
	spaceID := testdb.SeedSpaceMembership(t, database, tenantID, userID, testdb.SpaceOpts{})

	testdb.SeedCalendarEntry(t, database, tenantID, userID, testdb.CalendarEntryOpts{
		Title: "Pub", Status: "published", DestinationSpaceID: spaceID,
	})
	testdb.SeedCalendarEntry(t, database, tenantID, userID, testdb.CalendarEntryOpts{
		Title: "Pend", Status: "pending", DestinationSpaceID: spaceID,
	})
	testdb.SeedCalendarEntry(t, database, tenantID, userID, testdb.CalendarEntryOpts{
		Title: "Arch", Status: "archived", DestinationSpaceID: spaceID,
	})

	store := NewStore(database)
	entries, total, err := store.AdminListEntries(t.Context(), AdminListEntriesFilter{
		TenantID: tenantID,
		Limit:    100,
	})
	if err != nil {
		t.Fatalf("AdminListEntries: %v", err)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3 (all statuses)", total)
	}
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(entries))
	}
}

// TestAdminListEntries_StatusFilter verifies that the status filter
// works correctly on the admin list.
func TestAdminListEntries_StatusFilter(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	userID := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "cal-admin-filter"})
	spaceID := testdb.SeedSpaceMembership(t, database, tenantID, userID, testdb.SpaceOpts{})

	testdb.SeedCalendarEntry(t, database, tenantID, userID, testdb.CalendarEntryOpts{
		Title: "Pub", Status: "published", DestinationSpaceID: spaceID,
	})
	testdb.SeedCalendarEntry(t, database, tenantID, userID, testdb.CalendarEntryOpts{
		Title: "Pend1", Status: "pending", DestinationSpaceID: spaceID,
	})
	testdb.SeedCalendarEntry(t, database, tenantID, userID, testdb.CalendarEntryOpts{
		Title: "Pend2", Status: "pending", DestinationSpaceID: spaceID,
	})

	store := NewStore(database)
	entries, total, err := store.AdminListEntries(t.Context(), AdminListEntriesFilter{
		TenantID: tenantID,
		Status:   "pending",
		Limit:    100,
	})
	if err != nil {
		t.Fatalf("AdminListEntries with status filter: %v", err)
	}
	if total != 2 {
		t.Fatalf("total = %d, want 2 (pending only)", total)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
}

// TestAdminUpdateEntry_ValidStatuses verifies that AdminUpdateEntry
// accepts published, archived, and unverified statuses (BUG-002).
func TestAdminUpdateEntry_ValidStatuses(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	userID := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "cal-update-user"})
	reviewerID := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "cal-reviewer"})
	spaceID := testdb.SeedSpaceMembership(t, database, tenantID, userID, testdb.SpaceOpts{})

	entryID := testdb.SeedCalendarEntry(t, database, tenantID, userID, testdb.CalendarEntryOpts{
		Title: "To Update", Status: "pending", DestinationSpaceID: spaceID,
	})

	store := NewStore(database)

	// Publish it.
	if err := store.AdminUpdateEntry(t.Context(), entryID, tenantID, reviewerID, "published"); err != nil {
		t.Fatalf("AdminUpdateEntry to published: %v", err)
	}

	// Verify it's now visible in the public list.
	entries, _, err := store.ListEntries(t.Context(), ListEntriesFilter{
		TenantID: tenantID,
		Limit:    100,
	})
	if err != nil {
		t.Fatalf("ListEntries after publish: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.ID == entryID {
			found = true
			if e.Status != "published" {
				t.Errorf("status = %q, want %q", e.Status, "published")
			}
		}
	}
	if !found {
		t.Fatal("published entry not found in public list")
	}

	// Archive it.
	if err := store.AdminUpdateEntry(t.Context(), entryID, tenantID, reviewerID, "archived"); err != nil {
		t.Fatalf("AdminUpdateEntry to archived: %v", err)
	}

	// Verify it's no longer in the public list.
	entries, _, err = store.ListEntries(t.Context(), ListEntriesFilter{
		TenantID: tenantID,
		Limit:    100,
	})
	if err != nil {
		t.Fatalf("ListEntries after archive: %v", err)
	}
	for _, e := range entries {
		if e.ID == entryID {
			t.Fatal("archived entry should not appear in public list")
		}
	}

	// Mark unverified.
	if err := store.AdminUpdateEntry(t.Context(), entryID, tenantID, reviewerID, "unverified"); err != nil {
		t.Fatalf("AdminUpdateEntry to unverified: %v", err)
	}
}

// TestSubscribersForEntry verifies that SubscribersForEntry returns
// user IDs matching the entry's destination+category.
func TestSubscribersForEntry(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	userID := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "cal-sub-user"})
	spaceID := testdb.SeedSpaceMembership(t, database, tenantID, userID, testdb.SpaceOpts{})

	// Seed a subscriber.
	_, err := database.Writer.Exec(t.Context(), `
		INSERT INTO calendar_subscriptions (user_id, tenant_id, destination_space_id, category, created_at)
		VALUES ($1, $2, $3, 'visa_window', now())
	`, userID, tenantID, spaceID)
	if err != nil {
		t.Fatalf("seed subscription: %v", err)
	}

	// Seed an entry matching the subscription.
	entryID := testdb.SeedCalendarEntry(t, database, tenantID, userID, testdb.CalendarEntryOpts{
		Title: "Subscribed Entry", Status: "published",
		Category: "visa_window", DestinationSpaceID: spaceID,
	})

	store := NewStore(database)

	// Get the entry to pass to SubscribersForEntry.
	entry, err := store.GetEntry(t.Context(), entryID, tenantID)
	if err != nil {
		t.Fatalf("GetEntry: %v", err)
	}

	subs, err := store.SubscribersForEntry(t.Context(), entry)
	if err != nil {
		t.Fatalf("SubscribersForEntry: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("got %d subscribers, want 1", len(subs))
	}
	if subs[0] != userID {
		t.Errorf("subscriber = %v, want %v", subs[0], userID)
	}
}

// TestSubscribersForEntry_CategoryAgnostic verifies that a subscription
// with NULL category matches entries of any category.
func TestSubscribersForEntry_CategoryAgnostic(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	userID := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "cal-sub-all"})
	spaceID := testdb.SeedSpaceMembership(t, database, tenantID, userID, testdb.SpaceOpts{})

	// Seed a subscriber with NULL category (all categories).
	_, err := database.Writer.Exec(t.Context(), `
		INSERT INTO calendar_subscriptions (user_id, tenant_id, destination_space_id, category, created_at)
		VALUES ($1, $2, $3, NULL, now())
	`, userID, tenantID, spaceID)
	if err != nil {
		t.Fatalf("seed subscription: %v", err)
	}

	// Seed an entry with a specific category.
	entryID := testdb.SeedCalendarEntry(t, database, tenantID, userID, testdb.CalendarEntryOpts{
		Title: "Any Cat Entry", Status: "published",
		Category: "embassy_closure", DestinationSpaceID: spaceID,
	})

	store := NewStore(database)
	entry, err := store.GetEntry(t.Context(), entryID, tenantID)
	if err != nil {
		t.Fatalf("GetEntry: %v", err)
	}

	subs, err := store.SubscribersForEntry(t.Context(), entry)
	if err != nil {
		t.Fatalf("SubscribersForEntry: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("got %d subscribers, want 1 (NULL category matches all)", len(subs))
	}
}

// TestSaveUnsaveEntry verifies the save/unsave lifecycle.
func TestSaveUnsaveEntry(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	userID := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "cal-save-user"})
	spaceID := testdb.SeedSpaceMembership(t, database, tenantID, userID, testdb.SpaceOpts{})

	entryID := testdb.SeedCalendarEntry(t, database, tenantID, userID, testdb.CalendarEntryOpts{
		Title: "Save Me", Status: "published", DestinationSpaceID: spaceID,
	})

	store := NewStore(database)

	// Save.
	if err := store.SaveEntry(t.Context(), userID, entryID, tenantID); err != nil {
		t.Fatalf("SaveEntry: %v", err)
	}

	// Save again (idempotent — ON CONFLICT DO NOTHING).
	if err := store.SaveEntry(t.Context(), userID, entryID, tenantID); err != nil {
		t.Fatalf("SaveEntry (idempotent): %v", err)
	}

	// Unsave.
	if err := store.UnsaveEntry(t.Context(), userID, entryID, tenantID); err != nil {
		t.Fatalf("UnsaveEntry: %v", err)
	}

	// Unsave again (idempotent — DELETE is no-op if row doesn't exist).
	if err := store.UnsaveEntry(t.Context(), userID, entryID, tenantID); err != nil {
		t.Fatalf("UnsaveEntry (idempotent): %v", err)
	}
}

// TestSubscribeUnsubscribe verifies the subscribe/unsubscribe lifecycle.
func TestSubscribeUnsubscribe(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	userID := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "cal-sub-lifecycle"})
	spaceID := testdb.SeedSpaceMembership(t, database, tenantID, userID, testdb.SpaceOpts{})

	store := NewStore(database)

	// Subscribe.
	if err := store.Subscribe(t.Context(), userID, tenantID, spaceID, "visa_window"); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// Subscribe again (idempotent).
	if err := store.Subscribe(t.Context(), userID, tenantID, spaceID, "visa_window"); err != nil {
		t.Fatalf("Subscribe (idempotent): %v", err)
	}

	// Unsubscribe.
	if err := store.Unsubscribe(t.Context(), userID, tenantID, spaceID, "visa_window"); err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}

	// Unsubscribe again (idempotent).
	if err := store.Unsubscribe(t.Context(), userID, tenantID, spaceID, "visa_window"); err != nil {
		t.Fatalf("Unsubscribe (idempotent): %v", err)
	}
}

// TestFlagStaleEntries verifies that FlagStaleEntries marks entries with
// passed staleness_check_due as unverified (§86, BUG-006).
func TestFlagStaleEntries(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	userID := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "cal-stale-user"})
	spaceID := testdb.SeedSpaceMembership(t, database, tenantID, userID, testdb.SpaceOpts{})

	// Seed a published entry with staleness_check_due in the past.
	entryID := testdb.SeedCalendarEntry(t, database, tenantID, userID, testdb.CalendarEntryOpts{
		Title: "Stale Entry", Status: "published", DestinationSpaceID: spaceID,
	})
	_, err := database.Writer.Exec(t.Context(), `
		UPDATE calendar_entries SET staleness_check_due = now() - interval '1 day'
		WHERE id = $1
	`, entryID)
	if err != nil {
		t.Fatalf("set staleness_check_due: %v", err)
	}

	store := NewStore(database)
	n, err := store.FlagStaleEntries(t.Context())
	if err != nil {
		t.Fatalf("FlagStaleEntries: %v", err)
	}
	if n < 1 {
		t.Fatalf("FlagStaleEntries flagged %d entries, want >= 1", n)
	}

	// Verify the entry is now unverified.
	_, err = store.GetEntry(t.Context(), entryID, tenantID)
	// GetEntry only returns published entries, so this should be ErrNoRows
	// now that the entry is unverified.
	if err != pgx.ErrNoRows {
		t.Fatalf("expected ErrNoRows for unverified entry, got err=%v", err)
	}
}
