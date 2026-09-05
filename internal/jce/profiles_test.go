//go:build integration

package jce

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/testdb"
)

func TestProfileStore_CreateListGetUpdateDeleteDefault(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	userID := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "jce-profile-user"})
	spaceID := testdb.SeedSpaceMembership(t, database, tenantID, userID, testdb.SpaceOpts{})
	spaceIDStr := spaceID.String()

	store := newProfileStore(database)
	ctx := t.Context()

	age := 28
	bmin := 1000.0
	bmax := 5000.0
	input := ComparisonProfileInput{
		Title:          "Nigeria to Canada",
		SourceSpaceID:  &spaceIDStr,
		TargetSpaceIDs: []string{spaceIDStr},
		Goals: []ProfileGoal{
			{Key: "pr_fast", Weight: 0.3},
			{Key: "low_cost", Weight: 0.2},
		},
		Preferences: map[string]float64{"cost_sensitivity": 0.8},
		Demographics: ProfileDemographics{
			Age:                 &age,
			Occupation:          "software engineer",
			FamilyStatus:        "family",
			LanguageProficiency: "intermediate",
			BudgetMin:           &bmin,
			BudgetMax:           &bmax,
			BudgetCurrency:      "USD",
		},
	}

	// Create
	created, err := store.create(ctx, tenantID, userID, input)
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if created.ID == "" {
		t.Fatal("created profile has empty ID")
	}
	if created.Title != input.Title {
		t.Fatalf("created title = %q, want %q", created.Title, input.Title)
	}
	if created.UserID == nil || *created.UserID != userID.String() {
		t.Fatalf("created user_id = %v, want %v", created.UserID, userID.String())
	}
	if created.SourceSpaceID == nil || *created.SourceSpaceID != spaceIDStr {
		t.Fatalf("created source_space_id = %v, want %v", created.SourceSpaceID, spaceIDStr)
	}
	if len(created.Goals) != 2 {
		t.Fatalf("created goals length = %d, want 2", len(created.Goals))
	}

	// List
	profiles, err := store.listByUser(ctx, tenantID, userID)
	if err != nil {
		t.Fatalf("list by user: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("list by user length = %d, want 1", len(profiles))
	}

	// Get
	got, err := store.get(ctx, stringToUUID(created.ID), tenantID)
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("get id = %s, want %s", got.ID, created.ID)
	}

	// Update
	input.Title = "Nigeria to UK"
	input.Preferences["cost_sensitivity"] = 0.9
	updated, err := store.update(ctx, stringToUUID(created.ID), tenantID, userID, input)
	if err != nil {
		t.Fatalf("update profile: %v", err)
	}
	if updated.Title != "Nigeria to UK" {
		t.Fatalf("updated title = %q, want %q", updated.Title, "Nigeria to UK")
	}
	if updated.Preferences["cost_sensitivity"] != 0.9 {
		t.Fatalf("updated preference = %v, want 0.9", updated.Preferences["cost_sensitivity"])
	}

	// Set default
	if err := store.setDefault(ctx, stringToUUID(created.ID), tenantID, userID); err != nil {
		t.Fatalf("set default: %v", err)
	}
	def, err := store.get(ctx, stringToUUID(created.ID), tenantID)
	if err != nil {
		t.Fatalf("get after set default: %v", err)
	}
	if !def.IsDefault {
		t.Fatal("expected profile to be default after setDefault")
	}

	// Create second profile and switch default
	input2 := input
	input2.Title = "Nigeria to Australia"
	second, err := store.create(ctx, tenantID, userID, input2)
	if err != nil {
		t.Fatalf("create second profile: %v", err)
	}
	if err := store.setDefault(ctx, stringToUUID(second.ID), tenantID, userID); err != nil {
		t.Fatalf("set second default: %v", err)
	}

	first, err := store.get(ctx, stringToUUID(created.ID), tenantID)
	if err != nil {
		t.Fatalf("get first after second default: %v", err)
	}
	if first.IsDefault {
		t.Fatal("first profile should no longer be default")
	}
	secondRefreshed, err := store.get(ctx, stringToUUID(second.ID), tenantID)
	if err != nil {
		t.Fatalf("get second default: %v", err)
	}
	if !secondRefreshed.IsDefault {
		t.Fatal("second profile should be default")
	}

	// Count by source space
	stats, err := store.countBySourceSpace(ctx, tenantID, userID)
	if err != nil {
		t.Fatalf("count by source space: %v", err)
	}
	found := false
	for _, st := range stats {
		if st.SourceSpaceID == spaceIDStr {
			if st.ProfileCount != 2 {
				t.Fatalf("source %s count = %d, want 2", spaceIDStr, st.ProfileCount)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("expected count for source space %s, got %v", spaceIDStr, stats)
	}

	// Delete first
	firstID := stringToUUID(created.ID)
	if err := store.delete(ctx, firstID, tenantID); err != nil {
		t.Fatalf("delete profile: %v", err)
	}
	_, err = store.get(ctx, firstID, tenantID)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("get deleted profile: got %v, want pgx.ErrNoRows", err)
	}
}

func TestProfileStore_TemplateAccess(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	userID := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "jce-template-user"})
	store := newProfileStore(database)
	ctx := t.Context()

	templateInput := ComparisonProfileInput{
		Title:      "Family Template",
		IsTemplate: true,
		Goals:      []ProfileGoal{{Key: "family_friendly", Weight: 0.5}},
	}

	// Create a template without a user.
	template, err := store.create(ctx, tenantID, pgtype.UUID{}, templateInput)
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	if !template.IsTemplate {
		t.Fatal("created profile is not a template")
	}
	if template.UserID != nil {
		t.Fatalf("template user_id = %v, want nil", template.UserID)
	}

	regularInput := ComparisonProfileInput{
		Title: "Personal Profile",
		Goals: []ProfileGoal{{Key: "low_cost", Weight: 0.4}},
	}
	regular, err := store.create(ctx, tenantID, userID, regularInput)
	if err != nil {
		t.Fatalf("create regular profile: %v", err)
	}

	// listByUser should return only the regular profile.
	byUser, err := store.listByUser(ctx, tenantID, userID)
	if err != nil {
		t.Fatalf("list by user: %v", err)
	}
	if len(byUser) != 1 || byUser[0].ID != regular.ID {
		t.Fatalf("list by user = %v, want one regular profile", byUser)
	}

	// listTemplates should return only the template.
	templates, err := store.listTemplates(ctx, tenantID)
	if err != nil {
		t.Fatalf("list templates: %v", err)
	}
	if len(templates) != 1 || templates[0].ID != template.ID {
		t.Fatalf("list templates = %v, want one template", templates)
	}
}
