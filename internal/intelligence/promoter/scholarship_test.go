package promoter

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/extractor"
)

// mockStore is an in-memory implementation of the promoter Store interface.
// It records every call so tests can assert on the exact write sequence.
type mockStore struct {
	// seed data
	record    gen.IntelRecord
	source    gen.IntelSource
	existing  gen.Scholarship
	hasExisting bool

	// recorded calls
	createContentCalls   []gen.CreateContentParams
	createScholarshipCalls []gen.CreateScholarshipParams
	updateScholarshipCalls []gen.UpdateScholarshipParams
	reviewLogCalls       []gen.CreateIntelReviewLogParams
	promotedCalls        []gen.UpdateIntelRecordPromotedParams

	// return values for the last scholarship read
	lastScholarship gen.Scholarship
}

func (m *mockStore) GetIntelRecord(ctx context.Context, id pgtype.UUID) (gen.IntelRecord, error) {
	return m.record, nil
}

func (m *mockStore) GetIntelSource(ctx context.Context, id pgtype.UUID) (gen.IntelSource, error) {
	return m.source, nil
}

func (m *mockStore) GetScholarshipByIntelRecord(ctx context.Context, intelRecordID pgtype.UUID) (gen.Scholarship, error) {
	if !m.hasExisting {
		return gen.Scholarship{}, errors.New("pgx: no rows in result set")
	}
	return m.existing, nil
}

func (m *mockStore) CreateContent(ctx context.Context, arg gen.CreateContentParams) (gen.CreateContentRow, error) {
	m.createContentCalls = append(m.createContentCalls, arg)
	id := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	return gen.CreateContentRow{ID: id, GuideCategory: arg.GuideCategory}, nil
}

func (m *mockStore) CreateScholarship(ctx context.Context, arg gen.CreateScholarshipParams) (gen.Scholarship, error) {
	m.createScholarshipCalls = append(m.createScholarshipCalls, arg)
	row := gen.Scholarship{
		ID:             pgtype.UUID{Bytes: uuid.New(), Valid: true},
		IntelRecordID:  arg.IntelRecordID,
		GuideID:        arg.GuideID,
		Title:          arg.Title,
		Deadline:       arg.Deadline,
		EditorialOverrides: arg.EditorialOverrides,
	}
	m.lastScholarship = row
	return row, nil
}

func (m *mockStore) UpdateScholarship(ctx context.Context, arg gen.UpdateScholarshipParams) error {
	m.updateScholarshipCalls = append(m.updateScholarshipCalls, arg)
	// Reflect the update into lastScholarship so tests can assert final state.
	m.lastScholarship = gen.Scholarship{
		ID:             arg.ID,
		Title:          arg.Title,
		Description:    arg.Description,
		Deadline:       arg.Deadline,
		EditorialOverrides: arg.EditorialOverrides,
	}
	return nil
}

func (m *mockStore) CreateIntelReviewLog(ctx context.Context, arg gen.CreateIntelReviewLogParams) error {
	m.reviewLogCalls = append(m.reviewLogCalls, arg)
	return nil
}

func (m *mockStore) UpdateIntelRecordPromoted(ctx context.Context, arg gen.UpdateIntelRecordPromotedParams) error {
	m.promotedCalls = append(m.promotedCalls, arg)
	return nil
}

func mustDate(t *testing.T, s string) pgtype.Date {
	t.Helper()
	tt, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("parse date %q: %v", s, err)
	}
	return pgtype.Date{Time: tt, Valid: true}
}

func strPtr(s string) *string { return &s }

func extractionJSON(t *testing.T, ext extractor.Extraction) []byte {
	t.Helper()
	b, err := json.Marshal(ext)
	if err != nil {
		t.Fatalf("marshal extraction: %v", err)
	}
	return b
}

// TestScholarshipPromoter_UpdateRespectsEditorialOverrides is THE critical
// regression guard for the editorial-override-respect invariant.
//
// Setup: an existing scholarship has editorial_overrides = {"deadline": ...}
// and a human-edited deadline of 2027-12-31. A re-crawl produces a new
// extraction with deadline = 2027-10-15 and a changed title.
//
// Assert: the deadline is STILL 2027-12-31 (human edit wins), the title WAS
// updated, and the auto_update review log records the title change but NOT the
// deadline.
func TestScholarshipPromoter_UpdateRespectsEditorialOverrides(t *testing.T) {
	recordID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	sourceID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	scholarshipID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	guideID := pgtype.UUID{Bytes: uuid.New(), Valid: true}

	// Human-edited deadline that must survive the re-crawl.
	humanDeadline := mustDate(t, "2027-12-31")
	// New deadline from the re-crawl that must be discarded.
	newExtractionDeadline := "2027-10-15"

	// New extraction: deadline changed AND title changed.
	newExt := extractor.Extraction{
		"title":               {Value: "MEXT Scholarship 2027 (Updated)"},
		"description_summary": {Value: "Updated summary."},
		"country":             {Value: []any{"JP"}},
		"deadline":            {Value: newExtractionDeadline},
	}

	store := &mockStore{
		record: gen.IntelRecord{
			ID:                recordID,
			SourceID:          sourceID,
			EntityType:        "scholarship",
			ConfidenceOverall: 0.92,
			Extracted:         extractionJSON(t, newExt),
		},
		source: gen.IntelSource{
			ID:         sourceID,
			SourceUrl:  "https://example.org/mext-2027",
			SourceType: "official",
			TrustLevel: 5,
		},
		existing: gen.Scholarship{
			ID:                scholarshipID,
			IntelRecordID:     recordID,
			GuideID:           guideID,
			SourceUrl:         "https://example.org/mext-2027",
			Title:             "MEXT Scholarship 2027",
			Country:           "JP",
			Deadline:          humanDeadline,
			EditorialOverrides: []byte(`{"deadline":"2027-12-31"}`),
			FreshnessScore:    40,
		},
		hasExisting: true,
	}

	p := NewScholarshipPromoter(store,
		pgtype.UUID{Bytes: uuid.New(), Valid: true},
		pgtype.UUID{Bytes: uuid.New(), Valid: true},
		WithClock(func() time.Time { return time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC) }),
	)

	res, err := p.Promote(context.Background(), uuid.UUID(recordID.Bytes))
	if err != nil {
		t.Fatalf("Promote returned error: %v", err)
	}

	if res.Action != ActionUpdated {
		t.Fatalf("Action = %q, want %q", res.Action, ActionUpdated)
	}
	if res.DomainID != uuid.UUID(scholarshipID.Bytes) {
		t.Errorf("DomainID = %v, want existing scholarship id", res.DomainID)
	}

	if len(store.updateScholarshipCalls) != 1 {
		t.Fatalf("expected 1 UpdateScholarship call, got %d", len(store.updateScholarshipCalls))
	}
	upd := store.updateScholarshipCalls[0]

	// THE invariant: deadline is still the human-edited value, not the
	// re-crawled 2027-10-15.
	if !upd.Deadline.Valid || !upd.Deadline.Time.Equal(humanDeadline.Time) {
		t.Errorf("deadline after update = %v, want human-edited %v (editorial override must win)",
			dateStr(upd.Deadline), dateStr(humanDeadline))
	}

	// The title WAS updated because it is not under editorial override.
	wantTitle := "MEXT Scholarship 2027 (Updated)"
	if upd.Title != wantTitle {
		t.Errorf("title after update = %q, want %q (non-overridden field should update)",
			upd.Title, wantTitle)
	}

	// ChangedFields reports title but NOT deadline.
	if !contains(res.ChangedFields, "title") {
		t.Errorf("ChangedFields = %v, want to contain %q", res.ChangedFields, "title")
	}
	if contains(res.ChangedFields, "deadline") {
		t.Errorf("ChangedFields = %v, must NOT contain %q (deadline is overridden)",
			res.ChangedFields, "deadline")
	}

	// Exactly one auto_update review log entry, and its field_changes must
	// reference title but never deadline.
	if len(store.reviewLogCalls) != 1 {
		t.Fatalf("expected 1 review log call, got %d", len(store.reviewLogCalls))
	}
	rl := store.reviewLogCalls[0]
	if rl.Action != "auto_update" {
		t.Errorf("review log action = %q, want %q", rl.Action, "auto_update")
	}
	var changes []fieldChange
	if err := json.Unmarshal(rl.FieldChanges, &changes); err != nil {
		t.Fatalf("unmarshal field_changes: %v", err)
	}
	for _, c := range changes {
		if c.Field == "deadline" {
			t.Errorf("field_changes must not record deadline (it is overridden); got %+v", c)
		}
	}
	foundTitle := false
	for _, c := range changes {
		if c.Field == "title" {
			foundTitle = true
		}
	}
	if !foundTitle {
		t.Errorf("field_changes must record title change; got %+v", changes)
	}

	// No content row should be created on the update path.
	if len(store.createContentCalls) != 0 {
		t.Errorf("update path must not create content rows; got %d", len(store.createContentCalls))
	}
	// No create-scholarship call on the update path.
	if len(store.createScholarshipCalls) != 0 {
		t.Errorf("update path must not create scholarships; got %d", len(store.createScholarshipCalls))
	}
}

// TestScholarshipPromoter_NoChangeBumpsFreshness verifies that when a re-crawl
// produces no field-level changes, the promoter skips the auto_update log and
// just bumps freshness (ActionVerified).
func TestScholarshipPromoter_NoChangeBumpsFreshness(t *testing.T) {
	recordID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	sourceID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	scholarshipID := pgtype.UUID{Bytes: uuid.New(), Valid: true}

	ext := extractor.Extraction{
		"title":  {Value: "Monbukagakusho Scholarship"},
		"country": {Value: []any{"JP"}},
		"deadline": {Value: "2027-09-01"},
	}

	store := &mockStore{
		record: gen.IntelRecord{
			ID:         recordID,
			SourceID:   sourceID,
			EntityType: "scholarship",
			Extracted:  extractionJSON(t, ext),
		},
		source: gen.IntelSource{ID: sourceID, SourceUrl: "https://example.org/m", SourceType: "official", TrustLevel: 4},
		existing: gen.Scholarship{
			ID:                scholarshipID,
			IntelRecordID:     recordID,
			Title:             "Monbukagakusho Scholarship",
			Country:           "JP",
			Deadline:          mustDate(t, "2027-09-01"),
			EditorialOverrides: []byte(`{}`),
			FreshnessScore:    30,
		},
		hasExisting: true,
	}

	p := NewScholarshipPromoter(store,
		pgtype.UUID{Bytes: uuid.New(), Valid: true},
		pgtype.UUID{Bytes: uuid.New(), Valid: true},
	)

	res, err := p.Promote(context.Background(), uuid.UUID(recordID.Bytes))
	if err != nil {
		t.Fatalf("Promote returned error: %v", err)
	}
	if res.Action != ActionVerified {
		t.Fatalf("Action = %q, want %q", res.Action, ActionVerified)
	}
	if len(res.ChangedFields) != 0 {
		t.Errorf("ChangedFields = %v, want empty", res.ChangedFields)
	}
	if len(store.reviewLogCalls) != 0 {
		t.Errorf("no-change path must not write review log; got %d", len(store.reviewLogCalls))
	}
	if len(store.updateScholarshipCalls) != 1 {
		t.Fatalf("expected 1 freshness-bump update, got %d", len(store.updateScholarshipCalls))
	}
	if store.updateScholarshipCalls[0].FreshnessScore != freshScore {
		t.Errorf("freshness = %d, want %d", store.updateScholarshipCalls[0].FreshnessScore, freshScore)
	}
}

// TestScholarshipPromoter_CreatePath inserts content + scholarship + marks the
// intel record promoted.
func TestScholarshipPromoter_CreatePath(t *testing.T) {
	recordID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	sourceID := pgtype.UUID{Bytes: uuid.New(), Valid: true}

	ext := extractor.Extraction{
		"title":               {Value: "New Scholarship"},
		"description_summary": {Value: "A summary."},
		"country":             {Value: []any{"US"}},
		"deadline":            {Value: "2027-06-15"},
	}

	store := &mockStore{
		record: gen.IntelRecord{
			ID:         recordID,
			SourceID:   sourceID,
			EntityType: "scholarship",
			Extracted:  extractionJSON(t, ext),
		},
		source: gen.IntelSource{ID: sourceID, SourceUrl: "https://example.org/new", SourceType: "official", TrustLevel: 3},
		hasExisting: false,
	}

	p := NewScholarshipPromoter(store,
		pgtype.UUID{Bytes: uuid.New(), Valid: true},
		pgtype.UUID{Bytes: uuid.New(), Valid: true},
	)

	res, err := p.Promote(context.Background(), uuid.UUID(recordID.Bytes))
	if err != nil {
		t.Fatalf("Promote returned error: %v", err)
	}
	if res.Action != ActionCreated {
		t.Fatalf("Action = %q, want %q", res.Action, ActionCreated)
	}
	if len(store.createContentCalls) != 1 {
		t.Fatalf("expected 1 CreateContent call, got %d", len(store.createContentCalls))
	}
	cc := store.createContentCalls[0]
	if cc.ContentType != "guide" {
		t.Errorf("content_type = %q, want %q", cc.ContentType, "guide")
	}
	if cc.GuideCategory == nil || *cc.GuideCategory != "scholarship" {
		t.Errorf("guide_category = %v, want %q", cc.GuideCategory, "scholarship")
	}
	if cc.Status != "published" {
		t.Errorf("status = %q, want %q", cc.Status, "published")
	}
	if cc.CanonicalUrl == nil || *cc.CanonicalUrl != "https://example.org/new" {
		t.Errorf("canonical_url = %v, want source url", cc.CanonicalUrl)
	}
	if cc.ContentHash == nil || *cc.ContentHash == "" {
		t.Errorf("content_hash must be computed per Rule 12")
	}
	if len(store.createScholarshipCalls) != 1 {
		t.Fatalf("expected 1 CreateScholarship call, got %d", len(store.createScholarshipCalls))
	}
	if len(store.promotedCalls) != 1 {
		t.Fatalf("expected 1 UpdateIntelRecordPromoted call, got %d", len(store.promotedCalls))
	}
	if store.promotedCalls[0].ID != recordID {
		t.Errorf("promoted record id mismatch")
	}
}

// TestScholarshipPromoter_RejectsNonScholarshipRecord ensures the promoter
// only handles its own entity type.
func TestScholarshipPromoter_RejectsNonScholarshipRecord(t *testing.T) {
	recordID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	store := &mockStore{
		record: gen.IntelRecord{ID: recordID, EntityType: "job"},
	}
	p := NewScholarshipPromoter(store, pgtype.UUID{}, pgtype.UUID{})
	if _, err := p.Promote(context.Background(), uuid.UUID(recordID.Bytes)); !errors.Is(err, ErrNotScholarship) {
		t.Fatalf("expected ErrNotScholarship, got %v", err)
	}
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
