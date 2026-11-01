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

// mockCourseStore is a map-backed UniversityCourseStore + TxStore.
type mockCourseStore struct {
	record     gen.IntelRecord
	source     gen.IntelSource
	unis       map[string]*gen.GetUniversitySpaceBySlugRow // slug → uni
	uniByName  map[string][]gen.GetUniversitySpaceByNameRow
	catalogs   map[string]gen.GetCourseCatalogBySlugRow               // slug → catalog
	offerings  map[string]*gen.GetCourseOfferingByCourseUniversityRow // "courseID|uniID"
	catalogSeq int

	catalogCalls   []gen.CreateCourseCatalogIntelParams
	offeringCalls  []gen.CreateCourseOfferingIntelParams
	updateCalls    []gen.UpdateCourseOfferingIntelParams
	reviewLogCalls []gen.CreateIntelReviewLogParams
	statusCalls    []gen.UpdateIntelRecordStatusParams
}

func newMockCourseStore() *mockCourseStore {
	return &mockCourseStore{
		unis:      map[string]*gen.GetUniversitySpaceBySlugRow{},
		uniByName: map[string][]gen.GetUniversitySpaceByNameRow{},
		catalogs:  map[string]gen.GetCourseCatalogBySlugRow{},
		offerings: map[string]*gen.GetCourseOfferingByCourseUniversityRow{},
	}
}

func (m *mockCourseStore) seedCourseRecord(t *testing.T, fields map[string]any) gen.IntelRecord {
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
		EntityType: "university_course",
		SourceID:   uuidToPg(uuid.New()),
		Extracted:  raw,
	}

	return m.record
}

func (m *mockCourseStore) seedUniversity(t *testing.T, slug, name string, geo *string) pgtype.UUID {
	t.Helper()
	id := uuidToPg(uuid.New())
	m.unis[slug] = &gen.GetUniversitySpaceBySlugRow{ID: id, Slug: slug, Name: name}
	m.uniByName[name] = append(m.uniByName[name], gen.GetUniversitySpaceByNameRow{
		ID: id, Name: name, Slug: slug, GeoCode: geo,
	})
	return id
}

func (m *mockCourseStore) GetIntelRecord(_ context.Context, _ pgtype.UUID) (gen.IntelRecord, error) {
	return m.record, nil
}
func (m *mockCourseStore) GetIntelSource(_ context.Context, _ pgtype.UUID) (gen.IntelSource, error) {
	return m.source, nil
}
func (m *mockCourseStore) GetPersonaUserID(_ context.Context, _ pgtype.UUID) (pgtype.UUID, error) {
	return pgtype.UUID{}, nil
}

func (m *mockCourseStore) GetUniversitySpaceBySlug(_ context.Context, slug string) (gen.GetUniversitySpaceBySlugRow, error) {
	if u, ok := m.unis[slug]; ok {
		return *u, nil
	}
	return gen.GetUniversitySpaceBySlugRow{}, errNoRows
}

func (m *mockCourseStore) GetUniversitySpaceByName(_ context.Context, name string) ([]gen.GetUniversitySpaceByNameRow, error) {
	return m.uniByName[name], nil
}

func (m *mockCourseStore) GetCourseCatalogBySlug(_ context.Context, slug string) (gen.GetCourseCatalogBySlugRow, error) {
	if c, ok := m.catalogs[slug]; ok {
		return c, nil
	}
	return gen.GetCourseCatalogBySlugRow{}, errNoRows
}

func (m *mockCourseStore) GetCourseOfferingByCourseUniversity(_ context.Context, arg gen.GetCourseOfferingByCourseUniversityParams) (gen.GetCourseOfferingByCourseUniversityRow, error) {
	key := arg.CourseID.String() + "|" + arg.UniversityID.String()
	if o, ok := m.offerings[key]; ok {
		return *o, nil
	}
	return gen.GetCourseOfferingByCourseUniversityRow{}, errNoRows
}

func (m *mockCourseStore) UpdateIntelRecordStatus(_ context.Context, arg gen.UpdateIntelRecordStatusParams) error {
	m.statusCalls = append(m.statusCalls, arg)
	return nil
}

func (m *mockCourseStore) WithTx(_ context.Context, fn func(UniversityCourseTxStore) error) error {
	return fn(m)
}

func (m *mockCourseStore) CreateCourseCatalogIntel(_ context.Context, arg gen.CreateCourseCatalogIntelParams) (pgtype.UUID, error) {
	m.catalogCalls = append(m.catalogCalls, arg)
	m.catalogSeq++
	id := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	m.catalogs[arg.Slug] = gen.GetCourseCatalogBySlugRow{ID: id, Slug: arg.Slug, Title: arg.Title}
	return id, nil
}

func (m *mockCourseStore) CreateCourseOfferingIntel(_ context.Context, arg gen.CreateCourseOfferingIntelParams) (pgtype.UUID, error) {
	m.offeringCalls = append(m.offeringCalls, arg)
	id := uuidToPg(uuid.New())
	key := arg.CourseID.String() + "|" + arg.UniversityID.String()
	m.offerings[key] = &gen.GetCourseOfferingByCourseUniversityRow{
		ID: id, CourseID: arg.CourseID, UniversityID: arg.UniversityID, Slug: arg.Slug,
		Tuition: arg.Tuition, Currency: arg.Currency, IntelRecordID: arg.IntelRecordID,
	}
	return id, nil
}

func (m *mockCourseStore) UpdateCourseOfferingIntel(_ context.Context, arg gen.UpdateCourseOfferingIntelParams) error {
	m.updateCalls = append(m.updateCalls, arg)
	return nil
}

func (m *mockCourseStore) CreateIntelReviewLog(_ context.Context, arg gen.CreateIntelReviewLogParams) error {
	m.reviewLogCalls = append(m.reviewLogCalls, arg)
	return nil
}

// ---------------------------------------------------------------------------

func TestCoursePromote_CreatesCatalogAndOffering(t *testing.T) {
	store := newMockCourseStore()
	p := NewUniversityCoursePromoter(store, uuidToPg(uuid.New()))
	ca := "CA"
	uniID := store.seedUniversity(t, "test-university", "Test University", &ca)

	rec := store.seedCourseRecord(t, map[string]any{
		"course_title": "MSc Computer Science", "university_name": "Test University",
		"country": []any{"CAN"}, "degree_type": "masters",
		"tuition": float64(28000), "currency": "cad",
		"study_mode": "full_time", "intake_months": []any{"September"},
		"source_url": "https://testu.ca/msc-cs",
	})

	res, err := p.Promote(context.Background(), uuid.UUID(rec.ID.Bytes))
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if res.Action != ActionCreated {
		t.Fatalf("Action = %q, want created", res.Action)
	}
	if len(store.catalogCalls) != 1 || store.catalogCalls[0].Title != "MSc Computer Science" {
		t.Fatalf("catalog calls = %+v", store.catalogCalls)
	}
	if len(store.offeringCalls) != 1 {
		t.Fatalf("offering calls = %d", len(store.offeringCalls))
	}
	o := store.offeringCalls[0]
	if o.Slug != "msc-computer-science-test-university" {
		t.Fatalf("offering slug = %q", o.Slug)
	}
	if o.UniversityID != uniID || o.IntelRecordID != rec.ID {
		t.Fatalf("offering linkage wrong: %+v", o)
	}
	if o.Currency == nil || *o.Currency != "CAD" {
		t.Fatalf("currency = %v, want CAD", o.Currency)
	}
	if len(o.IntakeMonths) != 1 || o.IntakeMonths[0] != "september" {
		t.Fatalf("intake_months = %v", o.IntakeMonths)
	}
	if len(store.statusCalls) != 1 || store.statusCalls[0].Status != "extracted" {
		t.Fatalf("status calls = %+v", store.statusCalls)
	}
}

func TestCoursePromote_UpdatesExistingOffering(t *testing.T) {
	store := newMockCourseStore()
	p := NewUniversityCoursePromoter(store, uuidToPg(uuid.New()))
	uniID := store.seedUniversity(t, "test-university", "Test University", nil)
	courseID := uuidToPg(uuid.New())
	offID := uuidToPg(uuid.New())

	key := courseID.String() + "|" + uniID.String()
	store.offerings[key] = &gen.GetCourseOfferingByCourseUniversityRow{
		ID: offID, CourseID: courseID, UniversityID: uniID,
		Tuition: mustNum(t, "20000"), IsPublished: true,
	}
	store.catalogs["msc-computer-science"] = gen.GetCourseCatalogBySlugRow{
		ID: courseID, Slug: "msc-computer-science",
	}

	rec := store.seedCourseRecord(t, map[string]any{
		"course_title": "MSc Computer Science", "university_slug": "test-university",
		"tuition": float64(25000), "currency": "USD", "source_url": "https://testu.ca/msc-cs",
	})

	res, err := p.Promote(context.Background(), uuid.UUID(rec.ID.Bytes))
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if res.Action != ActionUpdated {
		t.Fatalf("Action = %q, want updated", res.Action)
	}
	if len(store.updateCalls) != 1 {
		t.Fatalf("update calls = %d", len(store.updateCalls))
	}
	if res.DomainID != uuid.UUID(offID.Bytes) {
		t.Fatalf("DomainID wrong")
	}
	if len(store.reviewLogCalls) != 1 || store.reviewLogCalls[0].Action != "auto_update" {
		t.Fatalf("review log calls = %+v", store.reviewLogCalls)
	}
}

func TestCoursePromote_OverrideSkipsField(t *testing.T) {
	store := newMockCourseStore()
	p := NewUniversityCoursePromoter(store, uuidToPg(uuid.New()))
	uniID := store.seedUniversity(t, "test-university", "Test University", nil)
	courseID := uuidToPg(uuid.New())

	key := courseID.String() + "|" + uniID.String()
	store.offerings[key] = &gen.GetCourseOfferingByCourseUniversityRow{
		ID: uuidToPg(uuid.New()), CourseID: courseID, UniversityID: uniID,
		Tuition: mustNum(t, "20000"), IsPublished: true,
		EditorialOverrides: []byte(`{"tuition":true}`),
	}
	store.catalogs["msc-computer-science"] = gen.GetCourseCatalogBySlugRow{ID: courseID, Slug: "msc-computer-science"}

	rec := store.seedCourseRecord(t, map[string]any{
		"course_title": "MSc Computer Science", "university_slug": "test-university",
		"tuition": float64(25000), "source_url": "https://testu.ca/msc-cs",
	})

	res, err := p.Promote(context.Background(), uuid.UUID(rec.ID.Bytes))
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if res.Action != ActionVerified {
		t.Fatalf("Action = %q, want verified (all changes overridden)", res.Action)
	}
	if len(store.updateCalls) != 1 {
		t.Fatalf("update calls = %d", len(store.updateCalls))
	}
	if store.updateCalls[0].Tuition != store.offerings[key].Tuition {
		t.Error("overridden tuition was rewritten")
	}
}

func TestCoursePromote_UnknownUniversityRoutesToReview(t *testing.T) {
	store := newMockCourseStore()
	p := NewUniversityCoursePromoter(store, uuidToPg(uuid.New()))

	rec := store.seedCourseRecord(t, map[string]any{
		"course_title": "MSc Computer Science", "university_name": "Nowhere University",
		"source_url": "https://x.ca/msc-cs",
	})

	_, err := p.Promote(context.Background(), uuid.UUID(rec.ID.Bytes))
	if !errors.Is(err, ErrRouteToReview) {
		t.Fatalf("err = %v, want ErrRouteToReview", err)
	}
}
