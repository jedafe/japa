//go:build integration

package jce

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/testdb"
)

// newTestUUID returns a random UUID as a pgtype.UUID.
func newTestUUID(t *testing.T) pgtype.UUID {
	t.Helper()
	var u pgtype.UUID
	if err := u.Scan(uuid.New().String()); err != nil {
		t.Fatalf("newTestUUID: %v", err)
	}
	return u
}

// seedComparisonType creates a comparison type for a tenant and returns its id.
func seedComparisonType(t *testing.T, d *db.DB, tenantID pgtype.UUID) pgtype.UUID {
	t.Helper()

	fieldGroupID := newTestUUID(t)
	_, err := d.Writer.Exec(t.Context(),
		`INSERT INTO custom_field_groups (id, tenant_id, name, slug, is_system)
		 VALUES ($1, $2, 'Test Fields', 'test-fields', false)`,
		fieldGroupID, tenantID)
	if err != nil {
		t.Fatalf("seed comparison type: insert custom_field_groups: %v", err)
	}

	typeID := newTestUUID(t)
	slug := "test-type-" + uuid.New().String()
	_, err = d.Writer.Exec(t.Context(),
		`INSERT INTO comparison_type_definitions (id, tenant_id, name, slug, field_group_id, item_label, is_system, config)
		 VALUES ($1, $2, 'Test Type', $3, $4, 'Item', false, '{}'::jsonb)`,
		typeID, tenantID, slug, fieldGroupID)
	if err != nil {
		t.Fatalf("seed comparison type: insert comparison_type_definitions: %v", err)
	}

	return typeID
}

func TestScoreFactorStore_CreateListUpdateDelete(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	typeID := seedComparisonType(t, database, tenantID)
	typeIDStr := typeID.String()

	store := newScoreFactorStore(database)
	ctx := t.Context()

	input := ScoreFactorInput{
		ComparisonTypeID: &typeIDStr,
		FactorKey:        "cost",
		Label:            "Cost of Living",
		Description:      "Lower is better",
		DataSource:       "resolver_metadata",
		DefaultWeight:    0.25,
		Config: ScoreFactorConfig{
			Kind:         "number",
			Direction:    "lower_is_better",
			ValueKey:     "cost",
			MissingScore: 0,
		},
	}

	created, err := store.create(ctx, tenantID, input)
	if err != nil {
		t.Fatalf("create score factor: %v", err)
	}
	if created.ID == "" {
		t.Fatal("created score factor has empty ID")
	}
	if created.FactorKey != input.FactorKey {
		t.Fatalf("factor key = %q, want %q", created.FactorKey, input.FactorKey)
	}
	if created.ComparisonTypeID == nil || *created.ComparisonTypeID != typeIDStr {
		t.Fatalf("comparison type id = %v, want %v", created.ComparisonTypeID, typeIDStr)
	}

	factors, err := store.listByType(ctx, tenantID, typeID, false)
	if err != nil {
		t.Fatalf("list by type: %v", err)
	}
	if len(factors) != 1 {
		t.Fatalf("list by type length = %d, want 1", len(factors))
	}

	got, err := store.get(ctx, stringToUUID(created.ID), tenantID)
	if err != nil {
		t.Fatalf("get score factor: %v", err)
	}
	if got.Label != created.Label {
		t.Fatalf("get label = %q, want %q", got.Label, created.Label)
	}
	if got.Config.ValueKey != "cost" {
		t.Fatalf("get config.value_key = %q, want cost", got.Config.ValueKey)
	}

	input.Label = "Cost of Living (updated)"
	input.DefaultWeight = 0.35
	input.Config.MissingScore = 10
	updated, err := store.update(ctx, stringToUUID(created.ID), tenantID, input)
	if err != nil {
		t.Fatalf("update score factor: %v", err)
	}
	if updated.Label != input.Label {
		t.Fatalf("updated label = %q, want %q", updated.Label, input.Label)
	}
	if updated.DefaultWeight != input.DefaultWeight {
		t.Fatalf("updated weight = %v, want %v", updated.DefaultWeight, input.DefaultWeight)
	}
	if updated.Config.MissingScore != 10 {
		t.Fatalf("updated missing_score = %v, want 10", updated.Config.MissingScore)
	}

	if err := store.delete(ctx, stringToUUID(created.ID), tenantID); err != nil {
		t.Fatalf("delete score factor: %v", err)
	}
	_, err = store.get(ctx, stringToUUID(created.ID), tenantID)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("get deleted factor: got %v, want pgx.ErrNoRows", err)
	}
}

func TestScoreFactorStore_GlobalOverride(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	typeID := seedComparisonType(t, database, tenantID)
	typeIDStr := typeID.String()

	store := newScoreFactorStore(database)
	ctx := t.Context()

	globalInput := ScoreFactorInput{
		ComparisonTypeID: nil,
		FactorKey:        "cost",
		Label:            "Global Cost",
		DataSource:       "resolver_metadata",
		DefaultWeight:    0.20,
		IsSystem:         true,
		Config: ScoreFactorConfig{
			Kind:         "number",
			Direction:    "lower_is_better",
			ValueKey:     "cost",
			MissingScore: 0,
		},
	}

	global, err := store.create(ctx, tenantID, globalInput)
	if err != nil {
		t.Fatalf("create global factor: %v", err)
	}
	if global.ComparisonTypeID != nil {
		t.Fatalf("global factor comparison_type_id = %v, want nil", global.ComparisonTypeID)
	}

	overrideInput := globalInput
	overrideInput.ComparisonTypeID = &typeIDStr
	overrideInput.Label = "Type-Specific Cost"
	overrideInput.DefaultWeight = 0.40
	overrideInput.IsSystem = false

	override, err := store.create(ctx, tenantID, overrideInput)
	if err != nil {
		t.Fatalf("create override factor: %v", err)
	}
	if override.ComparisonTypeID == nil || *override.ComparisonTypeID != typeIDStr {
		t.Fatalf("override comparison_type_id = %v, want %v", override.ComparisonTypeID, typeIDStr)
	}

	// getByKey for the type should return the override.
	byType, err := store.getByKey(ctx, tenantID, typeID, "cost")
	if err != nil {
		t.Fatalf("get by key with type: %v", err)
	}
	if byType.Label != overrideInput.Label {
		t.Fatalf("by type label = %q, want %q", byType.Label, overrideInput.Label)
	}
	if byType.DefaultWeight != overrideInput.DefaultWeight {
		t.Fatalf("by type weight = %v, want %v", byType.DefaultWeight, overrideInput.DefaultWeight)
	}

	// getByKey with no type should return the global factor.
	globalAgain, err := store.getByKey(ctx, tenantID, pgtype.UUID{}, "cost")
	if err != nil {
		t.Fatalf("get by key global: %v", err)
	}
	if globalAgain.Label != globalInput.Label {
		t.Fatalf("global label = %q, want %q", globalAgain.Label, globalInput.Label)
	}

	// listByType with includeGlobal should contain both; without it only the override.
	withGlobal, err := store.listByType(ctx, tenantID, typeID, true)
	if err != nil {
		t.Fatalf("list by type with global: %v", err)
	}
	if len(withGlobal) != 2 {
		t.Fatalf("list with global length = %d, want 2", len(withGlobal))
	}

	withoutGlobal, err := store.listByType(ctx, tenantID, typeID, false)
	if err != nil {
		t.Fatalf("list by type without global: %v", err)
	}
	if len(withoutGlobal) != 1 {
		t.Fatalf("list without global length = %d, want 1", len(withoutGlobal))
	}
}
