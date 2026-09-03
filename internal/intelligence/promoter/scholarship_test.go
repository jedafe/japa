package promoter

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/extractor"
	"github.com/jedafe/japa/internal/intelligence/reviewerctx"
)

// mockStore is an in-memory implementation of the promoter Store interface.
// It records every call so tests can assert on the exact write sequence.
type mockStore struct {
	// seed data
	record         gen.IntelRecord
	source         gen.IntelSource
	existing       gen.Scholarship
	hasExisting    bool
	countrySpaceID pgtype.UUID // BUG-005: country space id returned by GetCountrySpace
	contentReturn  *gen.GetContentByIDRow

	// failure injection
	failCreateContent bool
	failCreateScholarship bool

	// existing content by slug for uniqueness checks
	contentBySlug map[string]gen.GetContentBySlugRow

	// recorded calls
	createScholarshipCalls []gen.CreateScholarshipParams
	updateScholarshipCalls []gen.UpdateScholarshipParams
	reviewLogCalls         []gen.CreateIntelReviewLogParams
	statusCalls            []gen.UpdateIntelRecordStatusParams
	promotedCalls          []gen.UpdateIntelRecordPromotedParams
	createContentCalls     []gen.CreateContentParams
	updateContentCalls     []gen.UpdateContentParams
	getContentByIDCalls    []pgtype.UUID

	// return values for the last scholarship read
	lastScholarship gen.Scholarship
}

func (m *mockStore) GetIntelRecord(ctx context.Context, tenantID, id pgtype.UUID) (gen.IntelRecord, error) {
	return m.record, nil
}

func (m *mockStore) GetIntelSource(ctx context.Context, tenantID, id pgtype.UUID) (gen.IntelSource, error) {
	return m.source, nil
}

func (m *mockStore) GetScholarshipByIntelRecord(ctx context.Context, tenantID, intelRecordID pgtype.UUID) (gen.Scholarship, error) {
	if !m.hasExisting {
		return gen.Scholarship{}, errors.New("pgx: no rows in result set")
	}
	return m.existing, nil
}

// GetCountrySpace returns the seeded country space id (BUG-005 fix). Tests
// that want to exercise the space-resolution path set countrySpaceID; the
// default zero value leaves country_space_id NULL.
func (m *mockStore) GetCountrySpace(ctx context.Context, tenantID pgtype.UUID, countryCode string) (pgtype.UUID, error) {
	return m.countrySpaceID, nil
}

// GetPersonaUserID returns an invalid UUID (no persona in the mock) so the
// promoter falls back to the default authorID. Tests that want to exercise
// the persona resolution path can override this.
func (m *mockStore) GetPersonaUserID(ctx context.Context, tenantID, personaID pgtype.UUID) (pgtype.UUID, error) {
	return pgtype.UUID{}, nil
}

func (m *mockStore) CreateContent(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateContentParams) (gen.CreateContentRow, error) {
	m.createContentCalls = append(m.createContentCalls, arg)
	if m.failCreateContent {
		return gen.CreateContentRow{}, errors.New("mock: create content failed")
	}
	return gen.CreateContentRow{ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}, TenantID: tenantID, Title: arg.Title, Slug: arg.Slug}, nil
}

func (m *mockStore) GetContentByID(ctx context.Context, tenantID, id pgtype.UUID) (gen.GetContentByIDRow, error) {
	m.getContentByIDCalls = append(m.getContentByIDCalls, id)
	if m.contentReturn != nil {
		return *m.contentReturn, nil
	}
	return gen.GetContentByIDRow{
		ID:       id,
		TenantID: tenantID,
		Title:    m.existing.Title,
		Body:     "mock body",
		Status:   "published",
		Visibility: "public",
	}, nil
}

func (m *mockStore) GetContentBySlug(ctx context.Context, tenantID pgtype.UUID, slug string) (gen.GetContentBySlugRow, error) {
	if row, ok := m.contentBySlug[slug]; ok {
		return row, nil
	}
	return gen.GetContentBySlugRow{}, errors.New("pgx: no rows in result set")
}

func (m *mockStore) ContentSlugExists(ctx context.Context, tenantID pgtype.UUID, slug string) (bool, error) {
	_, exists := m.contentBySlug[slug]
	return exists, nil
}

func (m *mockStore) UpdateContent(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateContentParams) (gen.UpdateContentRow, error) {
	m.updateContentCalls = append(m.updateContentCalls, arg)
	return gen.UpdateContentRow{ID: arg.ID, TenantID: tenantID, Title: arg.Title, Slug: arg.Slug}, nil
}

func (m *mockStore) CreateScholarship(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateScholarshipParams) (gen.Scholarship, error) {
	m.createScholarshipCalls = append(m.createScholarshipCalls, arg)
	if m.failCreateScholarship {
		return gen.Scholarship{}, errors.New("mock: create scholarship failed")
	}
	row := gen.Scholarship{
		ID:                 pgtype.UUID{Bytes: uuid.New(), Valid: true},
		IntelRecordID:      arg.IntelRecordID,
		GuideID:            arg.GuideID,
		Title:              arg.Title,
		Deadline:           arg.Deadline,
		EditorialOverrides: arg.EditorialOverrides,
	}
	m.lastScholarship = row
	return row, nil
}

func (m *mockStore) UpdateScholarship(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateScholarshipParams) error {
	m.updateScholarshipCalls = append(m.updateScholarshipCalls, arg)
	// Reflect the update into lastScholarship so tests can assert final state.
	m.lastScholarship = gen.Scholarship{
		ID:                 arg.ID,
		Title:              arg.Title,
		Description:        arg.Description,
		Deadline:           arg.Deadline,
		EditorialOverrides: arg.EditorialOverrides,
	}
	return nil
}

func (m *mockStore) CreateIntelReviewLog(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateIntelReviewLogParams) error {
	m.reviewLogCalls = append(m.reviewLogCalls, arg)
	return nil
}

func (m *mockStore) UpdateIntelRecordStatus(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelRecordStatusParams) error {
	m.statusCalls = append(m.statusCalls, arg)
	return nil
}

func (m *mockStore) UpdateIntelRecordPromoted(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelRecordPromotedParams) error {
	m.promotedCalls = append(m.promotedCalls, arg)
	return nil
}

// WithTx runs fn with the mock itself as the TxStore. The mock has no real
// database, so the transaction is a passthrough — fn's CRUD calls go to the
// mock's recording methods directly.
func (m *mockStore) WithTx(ctx context.Context, fn func(TxStore) error) error {
	return fn(m)
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

// TestScholarshipPromoter_CreatePathAutoPublishesTrusted verifies that a
// high-confidence, trusted scholarship creates a published contents guide and
// links it through scholarships.guide_id.
func TestScholarshipPromoter_CreatePathAutoPublishesTrusted(t *testing.T) {
	recordID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	sourceID := pgtype.UUID{Bytes: uuid.New(), Valid: true}

	ext := extractor.Extraction{
		"title":               {Value: "New Scholarship"},
		"description_summary": {Value: "A summary."},
		"country":             {Value: []any{"US"}},
		"deadline":            {Value: "2027-06-15"},
		"university":          {Value: "Top University"},
		"degree_level":        {Value: "Bachelor"},
		"funding":             {Value: map[string]any{"amount": "full"}},
		"eligible_countries":  {Value: []any{"US", "CA"}},
		"application_url":     {Value: "https://example.org/apply"},
	}

	store := &mockStore{
		record: gen.IntelRecord{
			ID:                recordID,
			SourceID:          sourceID,
			EntityType:        "scholarship",
			ConfidenceOverall: 0.92,
			Extracted:         extractionJSON(t, ext),
		},
		source:      gen.IntelSource{ID: sourceID, SourceUrl: "https://example.org/new", SourceType: "official", TrustLevel: 1},
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
	if res.ContentID == uuid.Nil {
		t.Fatalf("ContentID is nil, want published guide id")
	}
	if len(store.createContentCalls) != 1 {
		t.Fatalf("expected 1 CreateContent call, got %d", len(store.createContentCalls))
	}
	c := store.createContentCalls[0]
	if c.ContentType != "guide" {
		t.Errorf("content_type = %q, want %q", c.ContentType, "guide")
	}
	if c.GuideCategory == nil || *c.GuideCategory != "scholarship" {
		t.Errorf("guide_category = %v, want scholarship", c.GuideCategory)
	}
	if c.Status != "published" || c.Visibility != "public" {
		t.Errorf("status/visibility = %s/%s, want published/public", c.Status, c.Visibility)
	}
	if c.AiGenerated != true {
		t.Errorf("ai_generated = %v, want true", c.AiGenerated)
	}
	if c.ContentHash == nil || *c.ContentHash == "" {
		t.Errorf("content_hash not set on created guide")
	}
	wantHash := content.ComputeContentHash(c.Body)
	if c.ContentHash == nil || *c.ContentHash != wantHash {
		t.Errorf("content_hash mismatch: got %v, want %s", c.ContentHash, wantHash)
	}

	if len(store.createScholarshipCalls) != 1 {
		t.Fatalf("expected 1 CreateScholarship call, got %d", len(store.createScholarshipCalls))
	}
	sch := store.createScholarshipCalls[0]
	if !sch.GuideID.Valid || uuid.UUID(sch.GuideID.Bytes) != res.ContentID {
		t.Errorf("scholarship guide_id = %v, want %v", sch.GuideID, res.ContentID)
	}
	if sch.VerificationStatus != "verified" {
		t.Errorf("verification_status = %q, want verified", sch.VerificationStatus)
	}
	if sch.OverrideSource != "ingestion" {
		t.Errorf("override_source = %q, want ingestion", sch.OverrideSource)
	}

	if len(store.promotedCalls) != 1 {
		t.Fatalf("expected 1 UpdateIntelRecordPromoted call, got %d", len(store.promotedCalls))
	}
	if store.promotedCalls[0].ID != recordID {
		t.Errorf("promoted record id mismatch")
	}
	if len(store.statusCalls) != 0 {
		t.Errorf("auto-publish path must not call UpdateIntelRecordStatus, got %d", len(store.statusCalls))
	}
}

// TestScholarshipPromoter_CreatePathRoutesToReview verifies that a lower-
// confidence or untrusted record does not create a contents row and is routed
// to review_required.
func TestScholarshipPromoter_CreatePathRoutesToReview(t *testing.T) {
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
			ID:                recordID,
			SourceID:          sourceID,
			EntityType:        "scholarship",
			ConfidenceOverall: 0.60,
			Extracted:         extractionJSON(t, ext),
		},
		source:      gen.IntelSource{ID: sourceID, SourceUrl: "https://example.org/new", SourceType: "official", TrustLevel: 2},
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
	if res.ContentID != uuid.Nil {
		t.Errorf("ContentID = %v, want uuid.Nil (no contents row)", res.ContentID)
	}
	if len(store.createContentCalls) != 0 {
		t.Errorf("review path must not create contents; got %d", len(store.createContentCalls))
	}
	if len(store.createScholarshipCalls) != 1 {
		t.Fatalf("expected 1 CreateScholarship call, got %d", len(store.createScholarshipCalls))
	}
	sch := store.createScholarshipCalls[0]
	if sch.GuideID.Valid {
		t.Errorf("guide_id must be NULL on review path; got valid %v", sch.GuideID)
	}
	if sch.VerificationStatus != "needs_review" {
		t.Errorf("verification_status = %q, want needs_review", sch.VerificationStatus)
	}
	if len(store.statusCalls) != 1 {
		t.Fatalf("expected 1 UpdateIntelRecordStatus call, got %d", len(store.statusCalls))
	}
	if store.statusCalls[0].Status != "review_required" {
		t.Errorf("status = %q, want %q", store.statusCalls[0].Status, "review_required")
	}
	if len(store.promotedCalls) != 0 {
		t.Errorf("review path must not call UpdateIntelRecordPromoted; got %d", len(store.promotedCalls))
	}
}

// TestScholarshipPromoter_CreatePathRollsBackOnContentFailure verifies that
// the transaction rolls back if the contents row cannot be created: no
// scholarship and no promoted status are recorded.
func TestScholarshipPromoter_CreatePathRollsBackOnContentFailure(t *testing.T) {
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
			ID:                recordID,
			SourceID:          sourceID,
			EntityType:        "scholarship",
			ConfidenceOverall: 0.92,
			Extracted:         extractionJSON(t, ext),
		},
		source:            gen.IntelSource{ID: sourceID, SourceUrl: "https://example.org/new", SourceType: "official", TrustLevel: 1},
		hasExisting:       false,
		failCreateContent: true,
	}

	p := NewScholarshipPromoter(store,
		pgtype.UUID{Bytes: uuid.New(), Valid: true},
		pgtype.UUID{Bytes: uuid.New(), Valid: true},
	)

	if _, err := p.Promote(context.Background(), uuid.UUID(recordID.Bytes)); err == nil {
		t.Fatalf("expected error from CreateContent failure, got nil")
	}
	if len(store.createContentCalls) != 1 {
		t.Fatalf("expected 1 CreateContent call, got %d", len(store.createContentCalls))
	}
	if len(store.createScholarshipCalls) != 0 {
		t.Errorf("scholarship must not be created on content failure; got %d", len(store.createScholarshipCalls))
	}
	if len(store.promotedCalls) != 0 {
		t.Errorf("intel record must not be promoted on content failure; got %d", len(store.promotedCalls))
	}
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
			ID:                 scholarshipID,
			IntelRecordID:      recordID,
			GuideID:            guideID,
			SourceUrl:          "https://example.org/mext-2027",
			Title:              "MEXT Scholarship 2027",
			Country:            "JP",
			Deadline:           humanDeadline,
			EditorialOverrides: []byte(`{"deadline":"2027-12-31"}`),
			FreshnessScore:     40,
			OverrideSource:     "ingestion",
		},
		hasExisting: true,
		contentReturn: &gen.GetContentByIDRow{
			ID:           guideID,
			TenantID:     pgtype.UUID{Bytes: uuid.New(), Valid: true},
			Title:        "MEXT Scholarship 2027",
			Body:         "stale body",
			Status:       "published",
			Visibility:   "public",
			SeoIndexable: true,
		},
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

	// The linked contents row was updated because title/body changed.
	if len(store.updateContentCalls) != 1 {
		t.Fatalf("expected 1 UpdateContent call, got %d", len(store.updateContentCalls))
	}
	cu := store.updateContentCalls[0]
	if cu.Title != wantTitle {
		t.Errorf("updated content title = %q, want %q", cu.Title, wantTitle)
	}
	if cu.ContentHash == nil || *cu.ContentHash == "" {
		t.Errorf("updated content hash not set")
	}
	wantHash := content.ComputeContentHash(cu.Body)
	if cu.ContentHash == nil || *cu.ContentHash != wantHash {
		t.Errorf("updated content hash mismatch: got %v, want %s", cu.ContentHash, wantHash)
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

	// No create-scholarship call on the update path.
	if len(store.createScholarshipCalls) != 0 {
		t.Errorf("update path must not create scholarships; got %d", len(store.createScholarshipCalls))
	}
}

// TestScholarshipPromoter_UpdatePreservesContentBodyOverride verifies that
// when a description is under editorial override the contents body is rebuilt
// from the existing (human-edited) description, not the re-crawl.
func TestScholarshipPromoter_UpdatePreservesContentBodyOverride(t *testing.T) {
	recordID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	sourceID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	scholarshipID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	guideID := pgtype.UUID{Bytes: uuid.New(), Valid: true}

	humanDescription := "Human-edited description."
	newExt := extractor.Extraction{
		"title":               {Value: "MEXT Scholarship 2027 (Updated)"},
		"description_summary": {Value: "Crawled description."},
		"country":             {Value: []any{"JP"}},
		"deadline":            {Value: "2027-10-15"},
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
			TrustLevel: 1,
		},
		existing: gen.Scholarship{
			ID:                 scholarshipID,
			IntelRecordID:      recordID,
			GuideID:            guideID,
			Title:              "MEXT Scholarship 2027",
			Description:        &humanDescription,
			Country:            "JP",
			Deadline:           mustDate(t, "2027-10-15"),
			EditorialOverrides: []byte(`{"description":"Human-edited description."}`),
			FreshnessScore:     40,
			OverrideSource:     "editorial",
		},
		hasExisting: true,
		contentReturn: &gen.GetContentByIDRow{
			ID:           guideID,
			TenantID:     pgtype.UUID{Bytes: uuid.New(), Valid: true},
			Title:        "MEXT Scholarship 2027",
			Body:         "stale body",
			Status:       "published",
			Visibility:   "public",
			SeoIndexable: true,
		},
	}

	p := NewScholarshipPromoter(store,
		pgtype.UUID{Bytes: uuid.New(), Valid: true},
		pgtype.UUID{Bytes: uuid.New(), Valid: true},
	)

	_, err := p.Promote(context.Background(), uuid.UUID(recordID.Bytes))
	if err != nil {
		t.Fatalf("Promote returned error: %v", err)
	}

	if len(store.updateContentCalls) != 1 {
		t.Fatalf("expected 1 UpdateContent call, got %d", len(store.updateContentCalls))
	}
	cu := store.updateContentCalls[0]
	if !bodyContains(cu.Body, humanDescription) {
		t.Errorf("updated content body should contain human-edited description; got %q", cu.Body)
	}
	if bodyContains(cu.Body, "Crawled description") {
		t.Errorf("updated content body must not contain crawled description when overridden; got %q", cu.Body)
	}
}

// TestScholarshipPromoter_ApproveReviewCreatesContent verifies that a human
// reviewer approving a review-required scholarship creates the published guide,
// links it, and marks the record promoted.
func TestScholarshipPromoter_ApproveReviewCreatesContent(t *testing.T) {
	recordID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	sourceID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	scholarshipID := pgtype.UUID{Bytes: uuid.New(), Valid: true}

	ext := extractor.Extraction{
		"title":               {Value: "Reviewed Scholarship"},
		"description_summary": {Value: "A summary."},
		"country":             {Value: []any{"US"}},
		"deadline":            {Value: "2027-08-01"},
	}

	store := &mockStore{
		record: gen.IntelRecord{
			ID:                recordID,
			SourceID:          sourceID,
			EntityType:        "scholarship",
			ConfidenceOverall: 0.40,
			Extracted:         extractionJSON(t, ext),
		},
		source: gen.IntelSource{ID: sourceID, SourceUrl: "https://example.org/review", SourceType: "official", TrustLevel: 2},
		existing: gen.Scholarship{
			ID:                 scholarshipID,
			IntelRecordID:      recordID,
			Title:              "Reviewed Scholarship",
			Country:            "US",
			Deadline:           mustDate(t, "2027-08-01"),
			VerificationStatus: "needs_review",
			OverrideSource:     "ingestion",
			FreshnessScore:     50,
		},
		hasExisting: true,
	}

	p := NewScholarshipPromoter(store,
		pgtype.UUID{Bytes: uuid.New(), Valid: true},
		pgtype.UUID{Bytes: uuid.New(), Valid: true},
	)

	reviewerID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	ctx := reviewerctx.WithReviewerID(context.Background(), reviewerID)
	res, err := p.Promote(ctx, uuid.UUID(recordID.Bytes))
	if err != nil {
		t.Fatalf("Promote returned error: %v", err)
	}
	if res.Action != ActionUpdated {
		t.Fatalf("Action = %q, want %q", res.Action, ActionUpdated)
	}
	if res.ContentID == uuid.Nil {
		t.Fatalf("ContentID is nil, want published guide id")
	}
	if len(store.createContentCalls) != 1 {
		t.Fatalf("expected 1 CreateContent call, got %d", len(store.createContentCalls))
	}
	c := store.createContentCalls[0]
	if c.HitlReviewedBy != reviewerID {
		t.Errorf("hitl_reviewed_by = %v, want %v", c.HitlReviewedBy, reviewerID)
	}
	if len(store.updateScholarshipCalls) != 1 {
		t.Fatalf("expected 1 UpdateScholarship call, got %d", len(store.updateScholarshipCalls))
	}
	upd := store.updateScholarshipCalls[0]
	if !upd.GuideID.Valid || uuid.UUID(upd.GuideID.Bytes) != res.ContentID {
		t.Errorf("update guide_id = %v, want %v", upd.GuideID, res.ContentID)
	}
	if upd.VerificationStatus != "verified" {
		t.Errorf("verification_status = %q, want verified", upd.VerificationStatus)
	}
	if upd.OverrideSource != "editorial" {
		t.Errorf("override_source = %q, want editorial", upd.OverrideSource)
	}
	if len(store.promotedCalls) != 1 {
		t.Fatalf("expected 1 UpdateIntelRecordPromoted call, got %d", len(store.promotedCalls))
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
		"title":    {Value: "Monbukagakusho Scholarship"},
		"country":  {Value: []any{"JP"}},
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
			ID:                 scholarshipID,
			IntelRecordID:      recordID,
			Title:              "Monbukagakusho Scholarship",
			Country:            "JP",
			Deadline:           mustDate(t, "2027-09-01"),
			EditorialOverrides: []byte(`{}`),
			FreshnessScore:     30,
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

// TestScholarshipPromoter_CreatePathGeneratesUniqueSlug verifies that the
// promoter appends a numeric suffix when the generated title slug collides with
// an existing contents row.
func TestScholarshipPromoter_CreatePathGeneratesUniqueSlug(t *testing.T) {
	recordID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	sourceID := pgtype.UUID{Bytes: uuid.New(), Valid: true}

	ext := extractor.Extraction{
		"title":               {Value: "New Scholarship"},
		"description_summary": {Value: "A summary."},
		"country":             {Value: []any{"US"}},
		"deadline":            {Value: "2027-06-15"},
		"university":          {Value: "Top University"},
		"degree_level":        {Value: "Bachelor"},
		"funding":             {Value: map[string]any{"amount": "full"}},
		"eligible_countries":  {Value: []any{"US", "CA"}},
		"application_url":     {Value: "https://example.org/apply"},
	}

	store := &mockStore{
		record: gen.IntelRecord{
			ID:                recordID,
			SourceID:          sourceID,
			EntityType:        "scholarship",
			ConfidenceOverall: 0.92,
			Extracted:         extractionJSON(t, ext),
		},
		source: gen.IntelSource{
			ID:        sourceID,
			SourceUrl: "https://example.org/r",
			SourceType: "official",
			TrustLevel: 2,
		},
		contentBySlug: map[string]gen.GetContentBySlugRow{
			"new-scholarship":   {ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}},
			"new-scholarship-2": {ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}},
		},
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
	if got := store.createContentCalls[0].Slug; got != "new-scholarship-3" {
		t.Fatalf("Slug = %q, want %q", got, "new-scholarship-3")
	}
}

// TestScholarshipPromoter_UpdateDoesNotEraseMissingFields verifies that a
// re-crawl that no longer includes optional fields does not wipe the existing
// scholarship values.
func TestScholarshipPromoter_UpdateDoesNotEraseMissingFields(t *testing.T) {
	recordID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	sourceID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	guideID := pgtype.UUID{Bytes: uuid.New(), Valid: true}

	existingDesc := "Original description"
	existingURL := "https://example.org/apply"
	existing := gen.Scholarship{
		ID:            recordID,
		IntelRecordID: recordID,
		GuideID:       pgtype.UUID{Bytes: guideID.Bytes, Valid: true},
		Title:         "Original Scholarship",
		Description:   &existingDesc,
		University:    strPtr("Old University"),
		Country:       "US",
		Deadline:      mustDate(t, "2027-06-15"),
		ApplicationUrl: &existingURL,
		TrustLevel:    2,
		VerificationStatus: "verified",
		OverrideSource: "ingestion",
	}

	// Re-crawl omits optional fields; only title and deadline changed.
	ext := extractor.Extraction{
		"title":    {Value: "Updated Scholarship"},
		"deadline": {Value: "2027-07-15"},
	}

	store := &mockStore{
		record: gen.IntelRecord{
			ID:                recordID,
			SourceID:          sourceID,
			EntityType:        "scholarship",
			ConfidenceOverall: 0.92,
			Extracted:         extractionJSON(t, ext),
		},
		source: gen.IntelSource{
			ID:         sourceID,
			SourceUrl:  "https://example.org/r",
			SourceType: "official",
			TrustLevel: 2,
		},
		hasExisting: true,
		existing:    existing,
		contentReturn: &gen.GetContentByIDRow{
			ID:     guideID,
			Title:  existing.Title,
			Body:   "Original body",
			Slug:   "original-scholarship",
			Status: "published",
		},
	}

	p := NewScholarshipPromoter(store,
		pgtype.UUID{Bytes: uuid.New(), Valid: true},
		pgtype.UUID{Bytes: uuid.New(), Valid: true},
	)

	res, err := p.Promote(context.Background(), uuid.UUID(recordID.Bytes))
	if err != nil {
		t.Fatalf("Promote returned error: %v", err)
	}
	if res.Action != ActionUpdated {
		t.Fatalf("Action = %q, want %q", res.Action, ActionUpdated)
	}
	if len(store.updateScholarshipCalls) != 1 {
		t.Fatalf("expected 1 UpdateScholarship call, got %d", len(store.updateScholarshipCalls))
	}
	updated := store.updateScholarshipCalls[0]
	if updated.Title != "Updated Scholarship" {
		t.Errorf("Title = %q, want %q", updated.Title, "Updated Scholarship")
	}
	if updated.Description == nil || *updated.Description != existingDesc {
		t.Errorf("Description overwritten: got %v, want %q", updated.Description, existingDesc)
	}
	if updated.University == nil || *updated.University != "Old University" {
		t.Errorf("University overwritten: got %v, want %q", updated.University, "Old University")
	}
	if updated.ApplicationUrl == nil || *updated.ApplicationUrl != existingURL {
		t.Errorf("ApplicationUrl overwritten: got %v, want %q", updated.ApplicationUrl, existingURL)
	}
	if updated.Deadline != mustDate(t, "2027-07-15") {
		t.Errorf("Deadline not updated: got %v", updated.Deadline)
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

func bodyContains(body, sub string) bool {
	return strings.Contains(body, sub)
}
