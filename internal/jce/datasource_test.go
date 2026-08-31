//go:build integration

package jce

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/testdb"
)

func ptr(s string) *string { return &s }

func pgUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		t.Fatalf("parse uuid %q: %v", s, err)
	}
	return u
}

func dataSourceInput(overrides ...func(*DataSourceInput)) DataSourceInput {
	in := DataSourceInput{
		SourceName: fmt.Sprintf("test-source-%s", uuid.New().String()),
		SourceKind: "official",
		TrustLevel: 3,
		Config:     map[string]any{"key": "value"},
	}
	for _, o := range overrides {
		o(&in)
	}
	return in
}

func seedDataSource(t *testing.T, ctx context.Context, store *dataSourceStore, tenantID pgtype.UUID, overrides ...func(*DataSourceInput)) DataSource {
	t.Helper()
	ds, err := store.create(ctx, tenantID, dataSourceInput(overrides...))
	if err != nil {
		t.Fatalf("create data source: %v", err)
	}
	return ds
}

func TestDataSource_StoreCreateListGetUpdateDelete(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantA := testdb.NewTenant(t, database)
	tenantB := testdb.NewTenant(t, database)
	store := newDataSourceStore(database)
	ctx := t.Context()

	eid := uuid.New().String()

	// Create two sources in tenant A.
	ds1 := seedDataSource(t, ctx, store, tenantA, func(in *DataSourceInput) {
		in.SourceName = "tenant-a-source-1"
		in.SourceKind = "official"
		in.TrustLevel = 5
		in.EntityType = ptr("university")
		in.EntityID = &eid
		in.Config = map[string]any{"url": "https://a.example.com"}
	})

	ds2 := seedDataSource(t, ctx, store, tenantA, func(in *DataSourceInput) {
		in.SourceName = "tenant-a-source-2"
		in.SourceKind = "third_party"
		in.TrustLevel = 2
	})

	// Create one source in tenant B for isolation testing.
	dsB := seedDataSource(t, ctx, store, tenantB, func(in *DataSourceInput) {
		in.SourceName = "tenant-b-source"
		in.SourceKind = "ai_estimate"
		in.TrustLevel = 1
	})

	// List all data sources for tenant A.
	all, err := store.list(ctx, tenantA, nil, nil)
	if err != nil {
		t.Fatalf("list data sources: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("list tenant A length = %d, want 2", len(all))
	}

	// Filter by entity_type.
	filtered, err := store.list(ctx, tenantA, ptr("university"), nil)
	if err != nil {
		t.Fatalf("list by entity type: %v", err)
	}
	if len(filtered) != 1 || filtered[0].ID != ds1.ID {
		t.Fatalf("list by entity type = %v, want ds1", filtered)
	}

	// Get ds1.
	got, err := store.get(ctx, stringToUUID(ds1.ID), tenantA)
	if err != nil {
		t.Fatalf("get data source: %v", err)
	}
	if got.ID != ds1.ID || got.SourceName != ds1.SourceName {
		t.Fatalf("got data source = %+v, want ds1", got)
	}
	if got.EntityType == nil || *got.EntityType != "university" {
		t.Fatalf("got entity_type = %v, want university", got.EntityType)
	}
	if got.EntityID == nil || *got.EntityID != eid {
		t.Fatalf("got entity_id = %v, want %s", got.EntityID, eid)
	}
	if got.Config["url"] != "https://a.example.com" {
		t.Fatalf("got config = %v", got.Config)
	}

	// Update ds2.
	updatedName := "tenant-a-source-2-updated"
	updated, err := store.update(ctx, stringToUUID(ds2.ID), tenantA, dataSourceInput(func(in *DataSourceInput) {
		in.SourceName = updatedName
		in.SourceKind = "official"
		in.TrustLevel = 4
		in.EntityType = ptr("visa_route")
		in.Config = map[string]any{"updated": true}
	}))
	if err != nil {
		t.Fatalf("update data source: %v", err)
	}
	if updated.SourceName != updatedName {
		t.Fatalf("updated source_name = %q, want %q", updated.SourceName, updatedName)
	}
	if updated.SourceKind != "official" {
		t.Fatalf("updated source_kind = %q, want official", updated.SourceKind)
	}
	if updated.TrustLevel != 4 {
		t.Fatalf("updated trust_level = %d, want 4", updated.TrustLevel)
	}
	if updated.EntityType == nil || *updated.EntityType != "visa_route" {
		t.Fatalf("updated entity_type = %v, want visa_route", updated.EntityType)
	}
	if updated.FreshnessScore != ds2.FreshnessScore {
		t.Fatalf("freshness_score changed from %d to %d", ds2.FreshnessScore, updated.FreshnessScore)
	}

	// Delete ds1.
	if err := store.delete(ctx, stringToUUID(ds1.ID), tenantA); err != nil {
		t.Fatalf("delete data source: %v", err)
	}
	_, err = store.get(ctx, stringToUUID(ds1.ID), tenantA)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("get deleted data source: got %v, want pgx.ErrNoRows", err)
	}

	// Tenant B should still see its own source.
	bList, err := store.list(ctx, tenantB, nil, nil)
	if err != nil {
		t.Fatalf("list tenant B: %v", err)
	}
	if len(bList) != 1 || bList[0].ID != dsB.ID {
		t.Fatalf("list tenant B = %v, want dsB", bList)
	}

	// Tenant A should not see tenant B's source.
	_, err = store.get(ctx, stringToUUID(dsB.ID), tenantA)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("get tenant B source from tenant A: got %v, want pgx.ErrNoRows", err)
	}
}

func TestDataSource_Validation(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	store := newDataSourceStore(database)

	base := dataSourceInput()

	// Empty source_name.
	invalid := base
	invalid.SourceName = ""
	if _, err := store.create(ctx, tenantID, invalid); err == nil {
		t.Fatal("expected error for empty source_name")
	}

	// Invalid source_kind.
	invalid = base
	invalid.SourceKind = "bad_kind"
	if _, err := store.create(ctx, tenantID, invalid); err == nil {
		t.Fatal("expected error for invalid source_kind")
	}

	// Trust level below 1.
	invalid = base
	invalid.TrustLevel = 0
	if _, err := store.create(ctx, tenantID, invalid); err == nil {
		t.Fatal("expected error for trust_level 0")
	}

	// Trust level above 5.
	invalid = base
	invalid.TrustLevel = 6
	if _, err := store.create(ctx, tenantID, invalid); err == nil {
		t.Fatal("expected error for trust_level 6")
	}

	// Invalid entity_id.
	invalid = base
	invalid.EntityID = ptr("not-a-uuid")
	if _, err := store.create(ctx, tenantID, invalid); err == nil {
		t.Fatal("expected error for invalid entity_id")
	}

	// fromDataSourceInput should validate directly.
	_, err := fromDataSourceInput(DataSourceInput{SourceName: base.SourceName, SourceKind: "bad_kind", TrustLevel: 3})
	if err == nil {
		t.Fatal("fromDataSourceInput: expected error for invalid source_kind")
	}

	_, err = fromDataSourceInput(DataSourceInput{SourceName: "", SourceKind: "official", TrustLevel: 3})
	if err == nil {
		t.Fatal("fromDataSourceInput: expected error for empty source_name")
	}

	_, err = fromDataSourceInput(DataSourceInput{SourceName: base.SourceName, SourceKind: "official", TrustLevel: -1})
	if err == nil {
		t.Fatal("fromDataSourceInput: expected error for negative trust_level")
	}
}

func TestDataSource_MatchSpecificity(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	otherTenant := testdb.NewTenant(t, database)
	store := newDataSourceStore(database)
	ctx := t.Context()

	entityType := ptr("university")
	entityID := uuid.New().String()
	dimension := "tuition"

	// Seed sources at increasing specificity.
	global := seedDataSource(t, ctx, store, tenantID, func(in *DataSourceInput) {
		in.SourceName = "match-global"
		in.SourceKind = "ai_estimate"
		in.TrustLevel = 1
	})

	typeWide := seedDataSource(t, ctx, store, tenantID, func(in *DataSourceInput) {
		in.SourceName = "match-type-wide"
		in.SourceKind = "third_party"
		in.TrustLevel = 2
		in.EntityType = entityType
	})

	entityWide := seedDataSource(t, ctx, store, tenantID, func(in *DataSourceInput) {
		in.SourceName = "match-entity-wide"
		in.SourceKind = "official"
		in.TrustLevel = 3
		in.EntityType = entityType
		in.EntityID = &entityID
	})

	entityDim := seedDataSource(t, ctx, store, tenantID, func(in *DataSourceInput) {
		in.SourceName = "match-entity-dim"
		in.SourceKind = "official"
		in.TrustLevel = 4
		in.EntityType = entityType
		in.EntityID = &entityID
		in.DimensionSlug = &dimension
	})

	eid := pgUUID(t, entityID)

	cases := []struct {
		name        string
		sourceName  string
		entityType  *string
		entityID    *pgtype.UUID
		dimension   string
		wantID      string
		wantName    string
		wantNil     bool
	}{
		{
			name:       "global source matches any scope",
			sourceName: global.SourceName,
			entityType: entityType,
			entityID:   &eid,
			dimension:  dimension,
			wantID:     global.ID,
			wantName:   global.SourceName,
		},
		{
			name:       "type-wide source matches its entity type",
			sourceName: typeWide.SourceName,
			entityType: entityType,
			wantID:     typeWide.ID,
			wantName:   typeWide.SourceName,
		},
		{
			name:       "entity-wide source matches entity without dimension",
			sourceName: entityWide.SourceName,
			entityType: entityType,
			entityID:   &eid,
			wantID:     entityWide.ID,
			wantName:   entityWide.SourceName,
		},
		{
			name:       "entity-wide source falls back from specific dimension",
			sourceName: entityWide.SourceName,
			entityType: entityType,
			entityID:   &eid,
			dimension:  dimension,
			wantID:     entityWide.ID,
			wantName:   entityWide.SourceName,
		},
		{
			name:       "entity-dim source matches exact dimension",
			sourceName: entityDim.SourceName,
			entityType: entityType,
			entityID:   &eid,
			dimension:  dimension,
			wantID:     entityDim.ID,
			wantName:   entityDim.SourceName,
		},
		{
			name:       "unknown source returns nil",
			sourceName: "match-does-not-exist",
			entityType: entityType,
			entityID:   &eid,
			dimension:  dimension,
			wantNil:    true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := store.match(ctx, tenantID, tc.entityType, tc.entityID, tc.dimension, tc.sourceName)
			if err != nil {
				t.Fatalf("match data source: %v", err)
			}
			if tc.wantNil {
				if got != nil {
					t.Fatalf("got %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("got nil, want %s", tc.wantName)
			}
			if got.ID != tc.wantID || got.SourceName != tc.wantName {
				t.Fatalf("got id=%s name=%q, want id=%s name=%q", got.ID, got.SourceName, tc.wantID, tc.wantName)
			}
		})
	}

	// Tenant isolation for matching.
	got, err := store.match(ctx, otherTenant, entityType, &eid, dimension, entityDim.SourceName)
	if err != nil {
		t.Fatalf("match other tenant: %v", err)
	}
	if got != nil {
		t.Fatalf("match other tenant returned %v, want nil", got)
	}
}
