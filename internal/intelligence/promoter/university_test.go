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

var errNoRows = errors.New("pgx: no rows in result set")

// mockUniversityStore is a map-backed UniversityStore + UniversityTxStore.
type mockUniversityStore struct {
	record       gen.IntelRecord
	source       gen.IntelSource
	spaceTypeID  pgtype.UUID
	countrySpace pgtype.UUID
	groupID      pgtype.UUID
	fieldDefs    map[string]gen.GetCustomFieldByGroupAndKeyRow // key → def
	spaces       map[string]*gen.GetUniversitySpaceBySlugRow   // slug → space
	spaceByID    map[string]*gen.GetSpaceByIDRow
	links        map[string]*gen.UniversityIntelLink        // recordID → link
	cfvs         map[string]map[string]gen.CustomFieldValue // spaceID → fieldID → cfv
	linkBySpace  map[string]*gen.UniversityIntelLink

	createdSpaces  []gen.CreateSpaceParams
	updatedSpaces  []gen.UpdateSpaceParams
	cfvCalls       []gen.UpsertCustomFieldValueParams
	linkCalls      []gen.UpsertUniversityIntelLinkParams
	reviewLogCalls []gen.CreateIntelReviewLogParams
	statusCalls    []gen.UpdateIntelRecordStatusParams
}

func newMockUniversityStore() *mockUniversityStore {
	m := &mockUniversityStore{
		spaceTypeID:  pgtype.UUID{Bytes: uuid.New(), Valid: true},
		countrySpace: pgtype.UUID{Bytes: uuid.New(), Valid: true},
		groupID:      pgtype.UUID{Bytes: uuid.New(), Valid: true},
		fieldDefs:    map[string]gen.GetCustomFieldByGroupAndKeyRow{},
		spaces:       map[string]*gen.GetUniversitySpaceBySlugRow{},
		spaceByID:    map[string]*gen.GetSpaceByIDRow{},
		links:        map[string]*gen.UniversityIntelLink{},
		cfvs:         map[string]map[string]gen.CustomFieldValue{},
		linkBySpace:  map[string]*gen.UniversityIntelLink{},
	}
	for key, ft := range map[string]string{
		"initials": "text", "official_website_url": "url", "contact_email": "email",
		"accreditation_body": "text", "world_ranking": "number", "country_ranking": "number",
		"founded_year": "number", "type": "select", "programs_offered": "multiselect",
		"tuition_range": "select", "student_population": "number", "acceptance_rate": "number",
		"international_support": "boolean", "scholarship_available": "boolean",
		"application_url": "url",
	} {
		m.fieldDefs[key] = gen.GetCustomFieldByGroupAndKeyRow{
			ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}, GroupID: m.groupID,
			Key: key, FieldType: ft,
		}
	}

	return m
}

func (m *mockUniversityStore) seedUniversityRecord(t *testing.T, fields map[string]any) gen.IntelRecord {
	t.Helper()

	ext := make(extractor.Extraction, len(fields))
	for k, v := range fields {
		ext[k] = extractor.FieldExtraction{Value: v, Confidence: 0.9}
	}
	raw, err := json.Marshal(ext)
	if err != nil {
		t.Fatalf("marshal extraction: %v", err)
	}

	m.record = gen.IntelRecord{
		ID:         uuidToPg(uuid.New()),
		EntityType: "university",
		SourceID:   uuidToPg(uuid.New()),
		Extracted:  raw,
	}

	return m.record
}

func (m *mockUniversityStore) GetIntelRecord(_ context.Context, _ pgtype.UUID) (gen.IntelRecord, error) {
	return m.record, nil
}
func (m *mockUniversityStore) GetIntelSource(_ context.Context, _ pgtype.UUID) (gen.IntelSource, error) {
	return m.source, nil
}
func (m *mockUniversityStore) GetPersonaUserID(_ context.Context, _ pgtype.UUID) (pgtype.UUID, error) {
	return pgtype.UUID{}, nil
}

func (m *mockUniversityStore) GetUniversitySpaceBySlug(_ context.Context, slug string) (gen.GetUniversitySpaceBySlugRow, error) {
	if s, ok := m.spaces[slug]; ok {
		return *s, nil
	}

	return gen.GetUniversitySpaceBySlugRow{}, errNoRows
}

func (m *mockUniversityStore) GetUniversityIntelLinkByIntelRecord(_ context.Context, id pgtype.UUID) (gen.UniversityIntelLink, error) {
	if l, ok := m.links[id.String()]; ok {
		return *l, nil
	}

	return gen.UniversityIntelLink{}, errNoRows
}

func (m *mockUniversityStore) GetSpaceTypeBySlug(_ context.Context, slug string) (gen.SpaceType, error) {
	if slug != "education" {
		return gen.SpaceType{}, errNoRows
	}

	return gen.SpaceType{ID: m.spaceTypeID, Slug: slug}, nil
}

func (m *mockUniversityStore) GetSpaceByID(_ context.Context, id pgtype.UUID) (gen.GetSpaceByIDRow, error) {
	if s, ok := m.spaceByID[id.String()]; ok {
		return *s, nil
	}

	return gen.GetSpaceByIDRow{}, errNoRows
}

func (m *mockUniversityStore) GetCountrySpace(_ context.Context, _ string) (pgtype.UUID, error) {
	return m.countrySpace, nil
}

func (m *mockUniversityStore) GetCustomFieldGroupBySlug(_ context.Context, slug string) (gen.CustomFieldGroup, error) {
	return gen.CustomFieldGroup{ID: m.groupID, Slug: slug}, nil
}

func (m *mockUniversityStore) GetCustomFieldByGroupAndKey(_ context.Context, arg gen.GetCustomFieldByGroupAndKeyParams) (gen.GetCustomFieldByGroupAndKeyRow, error) {
	if d, ok := m.fieldDefs[arg.Key]; ok {
		return d, nil
	}

	return gen.GetCustomFieldByGroupAndKeyRow{}, errNoRows
}

func (m *mockUniversityStore) ListCustomFieldValuesByEntity(_ context.Context, arg gen.ListCustomFieldValuesByEntityParams) ([]gen.CustomFieldValue, error) {
	out := []gen.CustomFieldValue{}
	for _, v := range m.cfvs[arg.EntityID.String()] {
		out = append(out, v)
	}

	return out, nil
}

func (m *mockUniversityStore) UpdateIntelRecordStatus(_ context.Context, arg gen.UpdateIntelRecordStatusParams) error {
	m.statusCalls = append(m.statusCalls, arg)
	return nil
}

func (m *mockUniversityStore) WithTx(_ context.Context, fn func(UniversityTxStore) error) error {
	return fn(m)
}

// --- TxStore ---

func (m *mockUniversityStore) CreateSpace(_ context.Context, arg gen.CreateSpaceParams) (gen.CreateSpaceRow, error) {
	m.createdSpaces = append(m.createdSpaces, arg)
	id := uuidToPg(uuid.New())
	slugRow := &gen.GetUniversitySpaceBySlugRow{
		ID: id, Name: arg.Name, Slug: arg.Slug, GeoCode: arg.GeoCode,
		Level: arg.Level, SpaceTypeID: arg.SpaceTypeID,
	}
	m.spaces[arg.Slug] = slugRow
	m.spaceByID[id.String()] = &gen.GetSpaceByIDRow{
		ID: id, Name: arg.Name, Slug: arg.Slug, Description: arg.Description,
		GeoCode: arg.GeoCode, Level: arg.Level, Visibility: arg.Visibility,
		PostingPerm: arg.PostingPerm, ReplyingPerm: arg.ReplyingPerm,
		CommentingPerm: arg.CommentingPerm, JoiningPerm: arg.JoiningPerm,
		Status: arg.Status,
	}

	return gen.CreateSpaceRow{ID: id, Name: arg.Name, Slug: arg.Slug}, nil
}

func (m *mockUniversityStore) UpdateSpace(_ context.Context, arg gen.UpdateSpaceParams) (gen.UpdateSpaceRow, error) {
	m.updatedSpaces = append(m.updatedSpaces, arg)

	return gen.UpdateSpaceRow{ID: arg.ID, Name: arg.Name}, nil
}

func (m *mockUniversityStore) UpsertCustomFieldValue(_ context.Context, arg gen.UpsertCustomFieldValueParams) (gen.CustomFieldValue, error) {
	m.cfvCalls = append(m.cfvCalls, arg)
	if m.cfvs[arg.EntityID.String()] == nil {
		m.cfvs[arg.EntityID.String()] = map[string]gen.CustomFieldValue{}
	}
	m.cfvs[arg.EntityID.String()][arg.FieldID.String()] = gen.CustomFieldValue{
		FieldID: arg.FieldID, EntityType: arg.EntityType, EntityID: arg.EntityID,
		ValueText: arg.ValueText, ValueNumber: arg.ValueNumber,
		ValueBool: arg.ValueBool, ValueJson: arg.ValueJson,
	}

	return gen.CustomFieldValue{FieldID: arg.FieldID, EntityID: arg.EntityID}, nil
}

func (m *mockUniversityStore) UpsertUniversityIntelLink(_ context.Context, arg gen.UpsertUniversityIntelLinkParams) (gen.UniversityIntelLink, error) {
	m.linkCalls = append(m.linkCalls, arg)
	l := &gen.UniversityIntelLink{SpaceID: arg.SpaceID, IntelRecordID: arg.IntelRecordID}
	m.links[arg.IntelRecordID.String()] = l
	m.linkBySpace[arg.SpaceID.String()] = l

	return *l, nil
}

func (m *mockUniversityStore) CreateIntelReviewLog(_ context.Context, arg gen.CreateIntelReviewLogParams) error {
	m.reviewLogCalls = append(m.reviewLogCalls, arg)
	return nil
}

// ---------------------------------------------------------------------------

func TestUniversityPromote_CreateBuildsSpaceCFVsAndLink(t *testing.T) {
	store := newMockUniversityStore()
	p := NewUniversityPromoter(store, uuidToPg(uuid.New()))

	rec := store.seedUniversityRecord(t, map[string]any{
		"name": "Test University", "country": []any{"CAN"},
		"official_website_url": "https://testu.ca", "type": "public",
		"world_ranking": float64(42), "source_url": "https://testu.ca/about",
	})

	res, err := p.Promote(context.Background(), uuid.UUID(rec.ID.Bytes))
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if res.Action != ActionCreated {
		t.Fatalf("Action = %q, want created", res.Action)
	}
	if len(store.createdSpaces) != 1 {
		t.Fatalf("createdSpaces = %d", len(store.createdSpaces))
	}
	sp := store.createdSpaces[0]
	if sp.Slug != "test-university" || sp.Level != "university" || sp.Name != "Test University" {
		t.Fatalf("unexpected space: %+v", sp)
	}
	if sp.GeoCode == nil || *sp.GeoCode != "CA" {
		t.Fatalf("GeoCode = %v, want CA", sp.GeoCode)
	}
	if sp.ParentID != store.countrySpace {
		t.Fatalf("ParentID = %v, want country space", sp.ParentID)
	}

	// CFVs: website (url→text), type (select→json), ranking (number), initials backfill.
	cfvs := store.cfvs[res.DomainID.String()]
	if len(cfvs) != 4 {
		t.Fatalf("cfv count = %d, want 4: %+v", len(cfvs), cfvs)
	}
	var sawSelect bool
	for _, c := range store.cfvCalls {
		if string(c.ValueJson) == `{"value":"public"}` {
			sawSelect = true
		}
	}
	if !sawSelect {
		t.Error("select CFV {value:public} not written")
	}
	if len(store.linkCalls) != 1 || store.linkCalls[0].IntelRecordID != rec.ID {
		t.Fatalf("link calls = %+v", store.linkCalls)
	}
	if len(store.statusCalls) != 1 || store.statusCalls[0].Status != "extracted" {
		t.Fatalf("status calls = %+v", store.statusCalls)
	}
}

func TestUniversityPromote_UpdateRespectsEditorialOverrides(t *testing.T) {
	store := newMockUniversityStore()
	p := NewUniversityPromoter(store, uuidToPg(uuid.New()))

	spaceID := uuidToPg(uuid.New())
	rankDef := store.fieldDefs["world_ranking"]
	urlDef := store.fieldDefs["official_website_url"]
	store.cfvs[spaceID.String()] = map[string]gen.CustomFieldValue{
		rankDef.ID.String(): {FieldID: rankDef.ID, EntityID: spaceID, ValueNumber: mustNum(t, "42")},
		urlDef.ID.String():  {FieldID: urlDef.ID, EntityID: spaceID, ValueText: ptr("https://old.ca")},
	}
	store.spaceByID[spaceID.String()] = &gen.GetSpaceByIDRow{
		ID: spaceID, Name: "Test University", Slug: "test-university",
	}
	rec := store.seedUniversityRecord(t, map[string]any{
		"name": "Test University", "country": []any{"CAN"},
		"official_website_url": "https://new.ca", "world_ranking": float64(99),
		"source_url": "https://testu.ca/about",
	})
	link := &gen.UniversityIntelLink{
		SpaceID: spaceID, IntelRecordID: rec.ID,
		EditorialOverrides: []byte(`{"world_ranking":true}`),
	}
	store.links[rec.ID.String()] = link

	res, err := p.Promote(context.Background(), uuid.UUID(rec.ID.Bytes))
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if res.Action != ActionUpdated {
		t.Fatalf("Action = %q, want updated", res.Action)
	}
	for _, c := range store.cfvCalls {
		if c.FieldID == rankDef.ID {
			t.Error("overridden world_ranking was rewritten")
		}
	}
	var sawURL bool
	for _, c := range store.cfvCalls {
		if c.FieldID == urlDef.ID && c.ValueText != nil && *c.ValueText == "https://new.ca" {
			sawURL = true
		}
	}
	if !sawURL {
		t.Error("non-overridden official_website_url not updated")
	}
	if len(store.reviewLogCalls) != 1 {
		t.Fatalf("review log calls = %d", len(store.reviewLogCalls))
	}
	var suppressed []map[string]any
	if err := json.Unmarshal(store.reviewLogCalls[0].FieldChanges, &suppressed); err != nil {
		t.Fatalf("unmarshal field changes: %v", err)
	}
	var sawSuppressed bool
	for _, ch := range suppressed {
		if ch["field"] == "world_ranking" && ch["suppressed"] == true {
			sawSuppressed = true
		}
	}
	if !sawSuppressed {
		t.Errorf("expected suppressed world_ranking entry in review log: %s", store.reviewLogCalls[0].FieldChanges)
	}
}

func TestUniversityPromote_AdoptsUnlinkedSpaceBySlug(t *testing.T) {
	store := newMockUniversityStore()
	p := NewUniversityPromoter(store, uuidToPg(uuid.New()))

	spaceID := uuidToPg(uuid.New())
	store.spaces["test-university"] = &gen.GetUniversitySpaceBySlugRow{
		ID: spaceID, Name: "Test University", Slug: "test-university",
	}
	store.spaceByID[spaceID.String()] = &gen.GetSpaceByIDRow{
		ID: spaceID, Name: "Test University", Slug: "test-university",
	}
	rec := store.seedUniversityRecord(t, map[string]any{
		"name": "Test University", "country": []any{"CAN"},
		"source_url": "https://testu.ca/about",
	})

	res, err := p.Promote(context.Background(), uuid.UUID(rec.ID.Bytes))
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if len(store.createdSpaces) != 0 {
		t.Fatal("adopt path created a duplicate space")
	}
	if len(store.linkCalls) != 1 || store.linkCalls[0].SpaceID != spaceID {
		t.Fatalf("adopt did not link: %+v", store.linkCalls)
	}
	if res.DomainID != uuid.UUID(spaceID.Bytes) {
		t.Fatalf("DomainID = %v, want adopted space", res.DomainID)
	}
}

func TestUniversityPromote_NoCountryRoutesToReview(t *testing.T) {
	store := newMockUniversityStore()
	p := NewUniversityPromoter(store, uuidToPg(uuid.New()))

	rec := store.seedUniversityRecord(t, map[string]any{
		"name": "Test University", "source_url": "https://testu.ca/about",
	})

	_, err := p.Promote(context.Background(), uuid.UUID(rec.ID.Bytes))
	if !errors.Is(err, ErrRouteToReview) {
		t.Fatalf("err = %v, want ErrRouteToReview", err)
	}
}

func mustNum(t *testing.T, s string) pgtype.Numeric {
	t.Helper()
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		t.Fatalf("scan numeric: %v", err)
	}
	return n
}
