//go:build integration

package scholarships

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/testdb"
)

// TestSyncFromContentGuide_CreatesAndUpdates verifies that a published
// scholarship guide produces a scholarships projection row, and that a second
// sync with updated field_values updates the same row.
func TestSyncFromContentGuide_CreatesAndUpdates(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	author := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "scholarship-guide-author"})

	ctx := context.Background()
	contentSvc := content.NewService(database)
	store := NewStore(database)

	spaceID := testdb.SeedSpaceMembership(t, database, tenantID, author, testdb.SpaceOpts{})

	guideCategory := "scholarship"
	fieldValues, err := json.Marshal(map[string]any{
		"provider_institution": "Test University",
		"level_of_study":       "Bachelor",
		"award_amount_usd":     5000,
		"is_renewable":         true,
		"application_deadline": "2026-12-31",
		"application_link":     "https://example.com/apply",
		"countries_eligible":   "Nigeria, Ghana",
		"field_of_study":       "Computer Science, Engineering",
		"eligibility_criteria": "Open to African students.",
	})
	if err != nil {
		t.Fatalf("marshal field values: %v", err)
	}

	row, err := contentSvc.CreateContent(ctx, tenantID, content.CreateContentInput{
		AuthorID:      author,
		SpaceID:       spaceID,
		ContentType:   "guide",
		Title:         "Test Scholarship Guide",
		Body:          "A scholarship for African students.",
		BodyHtml:      "<p>A scholarship for African students.</p>",
		FieldValues:   fieldValues,
		Status:        "published",
		Visibility:    "public",
		GuideCategory: &guideCategory,
	})
	if err != nil {
		t.Fatalf("create content: %v", err)
	}

	if err := store.SyncFromContentGuide(ctx, tenantID, row.ID); err != nil {
		t.Fatalf("sync from guide: %v", err)
	}

	created, err := store.queries.GetScholarshipByGuide(ctx,

		row.ID,
	)
	if err != nil {
		t.Fatalf("get scholarship by guide: %v", err)
	}

	if created.Title != "Test Scholarship Guide" {
		t.Errorf("title = %q, want %q", created.Title, "Test Scholarship Guide")
	}
	if created.University == nil || *created.University != "Test University" {
		t.Errorf("university = %v, want %q", created.University, "Test University")
	}
	if created.DegreeLevel == nil || *created.DegreeLevel != "Bachelor" {
		t.Errorf("degree_level = %v, want %q", created.DegreeLevel, "Bachelor")
	}
	if created.ApplicationUrl == nil || *created.ApplicationUrl != "https://example.com/apply" {
		t.Errorf("application_url = %v, want %q", created.ApplicationUrl, "https://example.com/apply")
	}
	if !created.Deadline.Valid {
		t.Errorf("deadline not valid")
	}

	// Update the guide field values and run the sync again.
	updatedFieldValues, err := json.Marshal(map[string]any{
		"provider_institution": "Updated University",
		"level_of_study":       "Master",
		"award_amount_usd":     7500,
		"is_renewable":         false,
	})
	if err != nil {
		t.Fatalf("marshal updated field values: %v", err)
	}

	if _, err := contentSvc.UpdateContent(ctx, row.ID, tenantID, author, content.UpdateContentInput{
		FieldValues:   updatedFieldValues,
		GuideCategory: &guideCategory,
	}, false); err != nil {
		t.Fatalf("update content: %v", err)
	}

	if err := store.SyncFromContentGuide(ctx, tenantID, row.ID); err != nil {
		t.Fatalf("sync from guide after update: %v", err)
	}

	updated, err := store.queries.GetScholarshipByGuide(ctx,

		row.ID,
	)
	if err != nil {
		t.Fatalf("get updated scholarship: %v", err)
	}

	if updated.ID != created.ID {
		t.Errorf("scholarship id changed: %s vs %s", updated.ID, created.ID)
	}
	if updated.University == nil || *updated.University != "Updated University" {
		t.Errorf("updated university = %v, want %q", updated.University, "Updated University")
	}
	if updated.DegreeLevel == nil || *updated.DegreeLevel != "Master" {
		t.Errorf("updated degree_level = %v, want %q", updated.DegreeLevel, "Master")
	}

	funding := map[string]any{}
	if err := json.Unmarshal(updated.Funding, &funding); err != nil {
		t.Fatalf("unmarshal funding: %v", err)
	}
	if funding["award_amount_usd"] != 7500.0 {
		t.Errorf("funding award_amount_usd = %v, want %v", funding["award_amount_usd"], 7500.0)
	}
	if funding["is_renewable"] != false {
		t.Errorf("funding is_renewable = %v, want false", funding["is_renewable"])
	}
}

// TestSyncFromContentGuide_DraftDoesNotCreate verifies that an unpublished
// scholarship guide does not produce a finder row.
func TestSyncFromContentGuide_DraftDoesNotCreate(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	author := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "scholarship-draft-author"})

	ctx := context.Background()
	contentSvc := content.NewService(database)
	store := NewStore(database)

	spaceID := testdb.SeedSpaceMembership(t, database, tenantID, author, testdb.SpaceOpts{})

	guideCategory := "scholarship"
	row, err := contentSvc.CreateContent(ctx, tenantID, content.CreateContentInput{
		AuthorID:      author,
		SpaceID:       spaceID,
		ContentType:   "guide",
		Title:         "Draft Scholarship Guide",
		Body:          "Draft body.",
		BodyHtml:      "<p>Draft body.</p>",
		FieldValues:   []byte("{}"),
		Status:        "draft",
		Visibility:    "public",
		GuideCategory: &guideCategory,
	})
	if err != nil {
		t.Fatalf("create content: %v", err)
	}

	if err := store.SyncFromContentGuide(ctx, tenantID, row.ID); err != nil {
		t.Fatalf("sync from draft: %v", err)
	}

	if _, err := store.queries.GetScholarshipByGuide(ctx,

		row.ID,
	); err == nil {
		t.Fatalf("expected no scholarship for draft guide")
	}
}

// TestSyncFromContentGuide_DeletedGuideCleansProjection verifies that a sync
// call for a deleted guide removes the existing scholarships row.
func TestSyncFromContentGuide_DeletedGuideCleansProjection(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	author := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "scholarship-delete-author"})

	ctx := context.Background()
	contentSvc := content.NewService(database)
	store := NewStore(database)

	spaceID := testdb.SeedSpaceMembership(t, database, tenantID, author, testdb.SpaceOpts{})

	guideCategory := "scholarship"
	row, err := contentSvc.CreateContent(ctx, tenantID, content.CreateContentInput{
		AuthorID:      author,
		SpaceID:       spaceID,
		ContentType:   "guide",
		Title:         "Scholarship To Delete",
		Body:          "Body.",
		BodyHtml:      "<p>Body.</p>",
		FieldValues:   []byte(`{"provider_institution":"Delete Me"}`),
		Status:        "published",
		Visibility:    "public",
		GuideCategory: &guideCategory,
	})
	if err != nil {
		t.Fatalf("create content: %v", err)
	}

	if err := store.SyncFromContentGuide(ctx, tenantID, row.ID); err != nil {
		t.Fatalf("sync from guide: %v", err)
	}

	if _, err := store.queries.GetScholarshipByGuide(ctx,

		row.ID,
	); err != nil {
		t.Fatalf("expected scholarship before delete: %v", err)
	}

	if err := contentSvc.DeleteContent(ctx, row.ID, tenantID); err != nil {
		t.Fatalf("delete content: %v", err)
	}

	if err := store.SyncFromContentGuide(ctx, tenantID, row.ID); err != nil {
		t.Fatalf("sync after delete: %v", err)
	}

	if _, err := store.queries.GetScholarshipByGuide(ctx,

		row.ID,
	); err == nil {
		t.Fatalf("expected scholarship to be removed after delete")
	}
}
