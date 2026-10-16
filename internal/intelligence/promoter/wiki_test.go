package promoter

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/extractor"
)

// wikiMockStore is an in-memory WikiStore + WikiTxStore. Lookup tables are
// keyed so tests can seed exactly the spaces/categories the resolution
// order should hit; writes are recorded for assertion.
type wikiMockStore struct {
	record      gen.IntelRecord
	source      gen.IntelSource
	existing    gen.WikiPage
	hasExisting bool

	spacesByID    map[pgtype.UUID]gen.GetSpaceByIDRow
	spacesBySlug  map[string]gen.GetSpaceBySlugRow
	typeRefs      map[pgtype.UUID]gen.GetSpaceTypeRefRow
	countrySpaces map[string]pgtype.UUID
	categoryRows  map[pgtype.UUID]map[string]gen.WikiTypeCategory // spaceTypeID -> token -> row

	created   []gen.CreateWikiPageIntelParams
	updated   []gen.UpdateWikiPageIntelParams
	statuses  []gen.UpdateIntelRecordStatusParams
	reviewLog []gen.CreateIntelReviewLogParams

	// existingSlugs simulates the (space_id, slug) unique index.
	existingSlugs map[pgtype.UUID]map[string]bool
}

var errWikiNoRows = errors.New("pgx: no rows in result set")

func (m *wikiMockStore) GetIntelRecord(_ context.Context, _ pgtype.UUID) (gen.IntelRecord, error) {
	return m.record, nil
}

func (m *wikiMockStore) GetIntelSource(_ context.Context, _ pgtype.UUID) (gen.IntelSource, error) {
	return m.source, nil
}

func (m *wikiMockStore) GetWikiPageByIntelRecord(_ context.Context, _ pgtype.UUID) (gen.WikiPage, error) {
	if !m.hasExisting {
		return gen.WikiPage{}, errWikiNoRows
	}

	return m.existing, nil
}

func (m *wikiMockStore) GetSpaceByID(_ context.Context, id pgtype.UUID) (gen.GetSpaceByIDRow, error) {
	if row, ok := m.spacesByID[id]; ok {
		return row, nil
	}

	return gen.GetSpaceByIDRow{}, errWikiNoRows
}

func (m *wikiMockStore) GetSpaceBySlug(_ context.Context, slug string) (gen.GetSpaceBySlugRow, error) {
	if row, ok := m.spacesBySlug[slug]; ok {
		return row, nil
	}

	return gen.GetSpaceBySlugRow{}, errWikiNoRows
}

func (m *wikiMockStore) GetSpaceTypeRef(_ context.Context, spaceID pgtype.UUID) (gen.GetSpaceTypeRefRow, error) {
	if row, ok := m.typeRefs[spaceID]; ok {
		return row, nil
	}

	return gen.GetSpaceTypeRefRow{}, errWikiNoRows
}

func (m *wikiMockStore) GetCountrySpace(_ context.Context, code string) (pgtype.UUID, error) {
	if id, ok := m.countrySpaces[code]; ok {
		return id, nil
	}

	return pgtype.UUID{}, errWikiNoRows
}

func (m *wikiMockStore) ResolveWikiTypeCategoryToken(_ context.Context, arg gen.ResolveWikiTypeCategoryTokenParams) (gen.WikiTypeCategory, error) {
	if rows, ok := m.categoryRows[arg.SpaceTypeID]; ok {
		if row, ok := rows[arg.Token]; ok {
			return row, nil
		}
	}

	return gen.WikiTypeCategory{}, errWikiNoRows
}

func (m *wikiMockStore) GetPersonaUserID(_ context.Context, _ pgtype.UUID) (pgtype.UUID, error) {
	return pgtype.UUID{}, nil
}

func (m *wikiMockStore) WikiPageSlugExists(_ context.Context, arg gen.WikiPageSlugExistsParams) (bool, error) {
	return m.existingSlugs[arg.SpaceID][arg.Slug], nil
}

func (m *wikiMockStore) CreateWikiPageIntel(_ context.Context, arg gen.CreateWikiPageIntelParams) (gen.WikiPage, error) {
	m.created = append(m.created, arg)

	return gen.WikiPage{ID: uuidToPg(uuid.New()), SpaceID: arg.SpaceID, Category: arg.Category}, nil
}

func (m *wikiMockStore) UpdateWikiPageIntel(_ context.Context, arg gen.UpdateWikiPageIntelParams) error {
	m.updated = append(m.updated, arg)

	return nil
}

func (m *wikiMockStore) UpdateIntelRecordStatus(_ context.Context, arg gen.UpdateIntelRecordStatusParams) error {
	m.statuses = append(m.statuses, arg)

	return nil
}

func (m *wikiMockStore) CreateIntelReviewLog(_ context.Context, arg gen.CreateIntelReviewLogParams) error {
	m.reviewLog = append(m.reviewLog, arg)

	return nil
}

func (m *wikiMockStore) WithTx(_ context.Context, fn func(WikiTxStore) error) error {
	return fn(m)
}

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

func wikiPgUUID() pgtype.UUID { return uuidToPg(uuid.New()) }

func wikiIntelRecord(t *testing.T, fields map[string]any) gen.IntelRecord {
	t.Helper()

	ext := make(extractor.Extraction, len(fields))
	for k, v := range fields {
		ext[k] = extractor.FieldExtraction{Value: v, Confidence: 0.9}
	}

	raw, err := json.Marshal(ext)
	if err != nil {
		t.Fatalf("marshal extraction: %v", err)
	}

	return gen.IntelRecord{
		ID:                wikiPgUUID(),
		EntityType:        "wiki_page",
		SourceID:          wikiPgUUID(),
		Extracted:         raw,
		ConfidenceOverall: 0.9,
	}
}

func seedSpace(m *wikiMockStore, slug string) (spaceID, typeID pgtype.UUID) {
	spaceID, typeID = wikiPgUUID(), wikiPgUUID()

	if m.spacesBySlug == nil {
		m.spacesBySlug = map[string]gen.GetSpaceBySlugRow{}
	}

	if m.spacesByID == nil {
		m.spacesByID = map[pgtype.UUID]gen.GetSpaceByIDRow{}
	}

	if m.typeRefs == nil {
		m.typeRefs = map[pgtype.UUID]gen.GetSpaceTypeRefRow{}
	}

	m.spacesBySlug[slug] = gen.GetSpaceBySlugRow{ID: spaceID, SpaceTypeID: typeID}
	m.spacesByID[spaceID] = gen.GetSpaceByIDRow{ID: spaceID, SpaceTypeID: typeID}
	m.typeRefs[spaceID] = gen.GetSpaceTypeRefRow{SpaceTypeID: typeID}

	return spaceID, typeID
}

func seedCategory(m *wikiMockStore, typeID pgtype.UUID, token, slug string) {
	if m.categoryRows == nil {
		m.categoryRows = map[pgtype.UUID]map[string]gen.WikiTypeCategory{}
	}

	if m.categoryRows[typeID] == nil {
		m.categoryRows[typeID] = map[string]gen.WikiTypeCategory{}
	}

	m.categoryRows[typeID][token] = gen.WikiTypeCategory{Category: token, Slug: slug}
}

// ---------------------------------------------------------------------------
// Create path — space resolution
// ---------------------------------------------------------------------------

func TestWikiPromoteCreateResolvesSpaceIDAndCategory(t *testing.T) {
	m := &wikiMockStore{}
	spaceID, typeID := seedSpace(m, "lagos")
	seedCategory(m, typeID, "cost_of_living", "cost-of-living")

	m.record = wikiIntelRecord(t, map[string]any{
		"title":      "Cost of Living",
		"category":   "Cost of Living",
		"space_id":   pgToUUID(spaceID).String(),
		"source_url": "https://example.com/col",
	})

	res, err := NewWikiPromoter(m, wikiPgUUID()).Promote(t.Context(), pgToUUID(m.record.ID))
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}

	if res.Action != ActionCreated {
		t.Fatalf("action = %q, want created", res.Action)
	}

	if len(m.created) != 1 {
		t.Fatalf("created calls = %d, want 1", len(m.created))
	}

	got := m.created[0]
	if got.SpaceID != spaceID {
		t.Errorf("SpaceID = %v, want %v", got.SpaceID, spaceID)
	}

	if got.Category != "cost_of_living" {
		t.Errorf("Category = %q, want cost_of_living", got.Category)
	}

	if len(m.statuses) != 1 || m.statuses[0].Status != "extracted" {
		t.Errorf("statuses = %+v, want one 'extracted' write", m.statuses)
	}
}

func TestWikiPromoteCreateSpaceHintFallbacks(t *testing.T) {
	cases := []struct {
		name   string
		fields func(spaceID pgtype.UUID) map[string]any
		seed   func(m *wikiMockStore) pgtype.UUID
	}{
		{
			name: "space_slug",
			fields: func(_ pgtype.UUID) map[string]any {
				return map[string]any{"title": "Overview", "space_slug": "lagos"}
			},
			seed: func(m *wikiMockStore) pgtype.UUID {
				id, _ := seedSpace(m, "lagos")
				return id
			},
		},
		{
			name: "city name",
			fields: func(_ pgtype.UUID) map[string]any {
				return map[string]any{"title": "Housing", "city": "Kumasi"}
			},
			seed: func(m *wikiMockStore) pgtype.UUID {
				id, _ := seedSpace(m, "kumasi")
				return id
			},
		},
		{
			name: "country geo code",
			fields: func(_ pgtype.UUID) map[string]any {
				return map[string]any{"title": "Banking", "country": "GHA"}
			},
			seed: func(m *wikiMockStore) pgtype.UUID {
				id, _ := seedSpace(m, "ghana")
				m.countrySpaces = map[string]pgtype.UUID{"GHA": id}
				return id
			},
		},
		{
			name: "single geo_space_ids",
			fields: func(id pgtype.UUID) map[string]any {
				return map[string]any{"title": "Community"}
			},
			seed: func(m *wikiMockStore) pgtype.UUID {
				id, _ := seedSpace(m, "accra")
				return id
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &wikiMockStore{}
			wantSpace := tc.seed(m)
			m.record = wikiIntelRecord(t, tc.fields(wantSpace))

			if tc.name == "single geo_space_ids" {
				m.record.GeoSpaceIds = []pgtype.UUID{wantSpace}
			}

			res, err := NewWikiPromoter(m, wikiPgUUID()).Promote(t.Context(), pgToUUID(m.record.ID))
			if err != nil {
				t.Fatalf("Promote: %v", err)
			}

			if res.Action != ActionCreated || len(m.created) != 1 {
				t.Fatalf("action=%q created=%d, want created/1", res.Action, len(m.created))
			}

			if m.created[0].SpaceID != wantSpace {
				t.Errorf("SpaceID = %v, want %v", m.created[0].SpaceID, wantSpace)
			}
		})
	}
}

func TestWikiPromoteCreateSuffixesSlugCollision(t *testing.T) {
	m := &wikiMockStore{}
	spaceID, typeID := seedSpace(m, "lagos")
	seedCategory(m, typeID, "overview", "overview")
	m.existingSlugs = map[pgtype.UUID]map[string]bool{
		spaceID: {"cost-of-living": true},
	}
	m.record = wikiIntelRecord(t, map[string]any{
		"title":      "Cost of Living",
		"space_slug": "lagos",
	})

	res, err := NewWikiPromoter(m, wikiPgUUID()).Promote(t.Context(), pgToUUID(m.record.ID))
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}

	if res.Action != ActionCreated || len(m.created) != 1 {
		t.Fatalf("action=%q created=%d", res.Action, len(m.created))
	}

	if got := m.created[0].Slug; got != "cost-of-living-2" {
		t.Errorf("Slug = %q, want cost-of-living-2", got)
	}
}

func TestWikiPromoteCreateUnresolvableSpaceRoutesToReview(t *testing.T) {
	m := &wikiMockStore{
		record: wikiIntelRecord(t, map[string]any{
			"title": "Orphaned Page",
			// no space hints; empty geo_space_ids
		}),
	}

	_, err := NewWikiPromoter(m, wikiPgUUID()).Promote(t.Context(), pgToUUID(m.record.ID))
	if err == nil {
		t.Fatal("expected error for unresolvable space, got nil")
	}

	if !errors.Is(err, ErrRouteToReview) {
		t.Errorf("err = %v, want wrapped ErrRouteToReview so the worker routes the record to the review queue", err)
	}

	if len(m.created) != 0 {
		t.Errorf("no wiki_pages row should be written, got %d creates", len(m.created))
	}

	if len(m.statuses) != 0 {
		t.Errorf("record status must stay untouched, got %+v", m.statuses)
	}
}

// ---------------------------------------------------------------------------
// Create path — category canonicalization
// ---------------------------------------------------------------------------

func TestWikiPromoteCreateCategoryFallbacks(t *testing.T) {
	cases := []struct {
		name       string
		extractCat string
		seed       []string // tokens resolvable for the type
		want       string
	}{
		{"canonical token", "banking", []string{"banking", "overview"}, "banking"},
		{"normalized phrase", "Cost of Living", []string{"cost_of_living", "overview"}, "cost_of_living"},
		{"hyphenated slug form", "cost-of-living", []string{"cost_of_living", "overview"}, "cost_of_living"},
		{"unmanaged token falls back to overview", "cuisine", []string{"overview"}, "overview"},
		{"absent falls back to overview", "", []string{"overview"}, "overview"},
		{"nothing seeded still lands on overview", "", nil, "overview"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &wikiMockStore{}
			_, typeID := seedSpace(m, "lagos")

			for _, tok := range tc.seed {
				seedCategory(m, typeID, tok, tok)
			}

			fields := map[string]any{"title": "X", "space_slug": "lagos"}
			if tc.extractCat != "" {
				fields["category"] = tc.extractCat
			}

			m.record = wikiIntelRecord(t, fields)

			_, err := NewWikiPromoter(m, wikiPgUUID()).Promote(t.Context(), pgToUUID(m.record.ID))
			if err != nil {
				t.Fatalf("Promote: %v", err)
			}

			if got := m.created[0].Category; got != tc.want {
				t.Errorf("Category = %q, want %q", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Update path — category canonicalization + editorial overrides
// ---------------------------------------------------------------------------

func TestWikiPromoteUpdateCanonicalizesCategory(t *testing.T) {
	m := &wikiMockStore{}
	spaceID, typeID := seedSpace(m, "lagos")
	seedCategory(m, typeID, "banking", "banking")

	m.hasExisting = true
	m.existing = gen.WikiPage{
		ID:                 wikiPgUUID(),
		SpaceID:            spaceID,
		Category:           "overview",
		Title:              "Old Title",
		EditorialOverrides: []byte("{}"),
	}
	m.record = wikiIntelRecord(t, map[string]any{
		"title":    "Old Title",
		"category": "Banking",
	})

	res, err := NewWikiPromoter(m, wikiPgUUID()).Promote(t.Context(), pgToUUID(m.record.ID))
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}

	if res.Action != ActionUpdated {
		t.Fatalf("action = %q, want updated", res.Action)
	}

	if len(m.updated) != 1 {
		t.Fatalf("updated calls = %d, want 1", len(m.updated))
	}

	if got := m.updated[0].Category; got != "banking" {
		t.Errorf("Category = %q, want canonical banking", got)
	}

	if len(m.reviewLog) != 1 {
		t.Fatalf("review log entries = %d, want 1", len(m.reviewLog))
	}
}

func TestWikiPromoteUpdateSuppressesUnresolvableCategory(t *testing.T) {
	m := &wikiMockStore{}
	spaceID, typeID := seedSpace(m, "lagos")
	// Only 'overview' is seeded; extraction's 'cuisine' doesn't resolve.
	seedCategory(m, typeID, "overview", "overview")

	m.hasExisting = true
	m.existing = gen.WikiPage{
		ID:                 wikiPgUUID(),
		SpaceID:            spaceID,
		Category:           "banking",
		Title:              "Same Title",
		EditorialOverrides: []byte("{}"),
	}
	m.record = wikiIntelRecord(t, map[string]any{
		"title":    "Same Title",
		"category": "cuisine",
	})

	res, err := NewWikiPromoter(m, wikiPgUUID()).Promote(t.Context(), pgToUUID(m.record.ID))
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	// Nothing changed → verified re-stamp, no review log.
	if res.Action != ActionVerified {
		t.Fatalf("action = %q, want verified", res.Action)
	}

	if len(m.updated) != 1 || m.updated[0].Category != "banking" {
		t.Errorf("category must keep existing 'banking', got %+v", m.updated)
	}

	if len(m.reviewLog) != 0 {
		t.Errorf("no review log expected, got %d", len(m.reviewLog))
	}
}

func TestWikiPromoteUpdateEditorialOverrideWins(t *testing.T) {
	m := &wikiMockStore{}
	spaceID, typeID := seedSpace(m, "lagos")
	seedCategory(m, typeID, "banking", "banking")

	m.hasExisting = true
	m.existing = gen.WikiPage{
		ID:                 wikiPgUUID(),
		SpaceID:            spaceID,
		Category:           "overview",
		Title:              "Editor Title",
		EditorialOverrides: []byte(`{"category":{"actor":"admin"},"title":{"actor":"admin"}}`),
	}
	m.record = wikiIntelRecord(t, map[string]any{
		"title":    "Recrawl Title",
		"category": "Banking",
	})

	res, err := NewWikiPromoter(m, wikiPgUUID()).Promote(t.Context(), pgToUUID(m.record.ID))
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}

	if res.Action != ActionVerified {
		t.Fatalf("action = %q, want verified (overrides suppress all changes)", res.Action)
	}

	if m.updated[0].Category != "overview" || m.updated[0].Title != "Editor Title" {
		t.Errorf("overridden fields must keep existing values, got %+v", m.updated[0])
	}
}

func TestWikiPromoteRejectsNonWikiRecord(t *testing.T) {
	m := &wikiMockStore{record: wikiIntelRecord(t, map[string]any{"title": "x"})}
	m.record.EntityType = "scholarship"

	_, err := NewWikiPromoter(m, wikiPgUUID()).Promote(t.Context(), pgToUUID(m.record.ID))
	if !errors.Is(err, ErrNotWikiPage) {
		t.Fatalf("err = %v, want ErrNotWikiPage", err)
	}
}
