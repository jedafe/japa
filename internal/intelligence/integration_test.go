// Package intelligence_test contains cross-package integration tests that
// verify the intelligence ingestion spine packages work together correctly.
//
// These are unit-level integration tests: they wire together the real
// extractor, validator, gate, freshness, and promoter implementations using
// mock implementations of the interfaces those packages declare (the AI
// gateway and the promoter Store). They do NOT require a running Postgres or
// Redis — every persistence boundary is stubbed.
//
// Ref: docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md
//      Task 29 — integration tests (end-to-end task chain).
package intelligence_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/ai"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/changedetector"
	"github.com/jedafe/japa/internal/intelligence/dedup"
	"github.com/jedafe/japa/internal/intelligence/extractor"
	"github.com/jedafe/japa/internal/intelligence/extractor/schemas"
	"github.com/jedafe/japa/internal/intelligence/fetcher"
	"github.com/jedafe/japa/internal/intelligence/promoter"
	"github.com/jedafe/japa/internal/intelligence/services"
)

// ---------------------------------------------------------------------------
// Mock AI gateway (extractor.AIGateway)
// ---------------------------------------------------------------------------

// mockAIGateway is a test double for extractor.AIGateway. It returns a
// pre-canned response body so the extractor + validator chain can run
// end-to-end without a live LLM provider.
type mockAIGateway struct {
	content string
	err     error
	calls   int
}

func (m *mockAIGateway) ChatWithTools(ctx context.Context, category string, req ai.ChatWithToolsRequest) (ai.ChatWithToolsResponse, error) {
	m.calls++
	if m.err != nil {
		return ai.ChatWithToolsResponse{}, m.err
	}
	return ai.ChatWithToolsResponse{Content: m.content}, nil
}

// ---------------------------------------------------------------------------
// Mock promoter Store (promoter.Store)
// ---------------------------------------------------------------------------

// mockPromoterStore is an in-memory implementation of promoter.Store. It
// records every write so integration tests can assert on the exact sequence.
type mockPromoterStore struct {
	record     gen.IntelRecord
	source     gen.IntelSource
	existing   gen.Scholarship
	hasExisting bool
	countrySpaceID pgtype.UUID // BUG-005: country space id returned by GetCountrySpace

	createContentCalls     []gen.CreateContentParams
	createScholarshipCalls []gen.CreateScholarshipParams
	updateScholarshipCalls []gen.UpdateScholarshipParams
	reviewLogCalls         []gen.CreateIntelReviewLogParams
	promotedCalls          []gen.UpdateIntelRecordPromotedParams
}

func (m *mockPromoterStore) GetIntelRecord(ctx context.Context, tenantID, id pgtype.UUID) (gen.IntelRecord, error) {
	return m.record, nil
}

func (m *mockPromoterStore) GetIntelSource(ctx context.Context, tenantID, id pgtype.UUID) (gen.IntelSource, error) {
	return m.source, nil
}

// GetCountrySpace returns the seeded country space id (BUG-005 fix). The
// default zero value leaves country_space_id NULL, exercising the
// best-effort resolution path.
func (m *mockPromoterStore) GetCountrySpace(ctx context.Context, tenantID pgtype.UUID, countryCode string) (pgtype.UUID, error) {
	return m.countrySpaceID, nil
}

func (m *mockPromoterStore) GetScholarshipByIntelRecord(ctx context.Context, tenantID, intelRecordID pgtype.UUID) (gen.Scholarship, error) {
	if !m.hasExisting {
		return gen.Scholarship{}, errors.New("pgx: no rows in result set")
	}
	return m.existing, nil
}

func (m *mockPromoterStore) CreateContent(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateContentParams) (gen.CreateContentRow, error) {
	m.createContentCalls = append(m.createContentCalls, arg)
	id := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	return gen.CreateContentRow{ID: id, GuideCategory: arg.GuideCategory}, nil
}

func (m *mockPromoterStore) CreateScholarship(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateScholarshipParams) (gen.Scholarship, error) {
	m.createScholarshipCalls = append(m.createScholarshipCalls, arg)
	row := gen.Scholarship{
		ID:                 pgtype.UUID{Bytes: uuid.New(), Valid: true},
		IntelRecordID:      arg.IntelRecordID,
		GuideID:            arg.GuideID,
		Title:              arg.Title,
		Deadline:           arg.Deadline,
		EditorialOverrides: arg.EditorialOverrides,
	}
	return row, nil
}

func (m *mockPromoterStore) UpdateScholarship(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateScholarshipParams) error {
	m.updateScholarshipCalls = append(m.updateScholarshipCalls, arg)
	return nil
}

func (m *mockPromoterStore) CreateIntelReviewLog(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateIntelReviewLogParams) error {
	m.reviewLogCalls = append(m.reviewLogCalls, arg)
	return nil
}

func (m *mockPromoterStore) UpdateIntelRecordPromoted(ctx context.Context, tenantID pgtype.UUID, arg gen.UpdateIntelRecordPromotedParams) error {
	m.promotedCalls = append(m.promotedCalls, arg)
	return nil
}

// WithTx runs fn with the mock itself as the TxStore (passthrough — no real
// database in tests).
func (m *mockPromoterStore) WithTx(ctx context.Context, fn func(promoter.TxStore) error) error {
	return fn(m)
}

// ---------------------------------------------------------------------------
// Mock dedup RecordLister (dedup.RecordLister)
// ---------------------------------------------------------------------------

// mockRecordLister is a test double for dedup.RecordLister. It returns a
// configurable result for the (source_id, external_key) lookup so the dedup
// decision can be exercised without a live DB.
type mockRecordLister struct {
	// found controls whether GetIntelRecordByExternalKey returns a row or
	// pgx.ErrNoRows.
	found bool
	// differentSource simulates a row owned by a different source_id than
	// the one queried (used for conflict scenarios).
	differentSource bool
}

func (m *mockRecordLister) GetIntelRecordByExternalKey(ctx context.Context, tenantID pgtype.UUID, arg gen.GetIntelRecordByExternalKeyParams) (gen.IntelRecord, error) {
	if !m.found {
		return gen.IntelRecord{}, pgx.ErrNoRows
	}
	return gen.IntelRecord{SourceID: arg.SourceID, ExternalKey: arg.ExternalKey}, nil
}

// ListIntelRecordsByExternalKey returns a cross-source lookup result for
// the dedup CheckAcrossSources test path (BUG-20 fix). When differentSource
// is true, it returns a record with a different source_id to simulate a
// cross-source conflict.
func (m *mockRecordLister) ListIntelRecordsByExternalKey(ctx context.Context, tenantID pgtype.UUID, externalKey string) ([]gen.IntelRecord, error) {
	if !m.found || !m.differentSource {
		return nil, nil
	}
	// Return a record from a different source.
	otherSource := pgtype.UUID{Bytes: [16]byte{0xff}, Valid: true}
	return []gen.IntelRecord{{SourceID: otherSource, ExternalKey: externalKey}}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

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

func newUUID() pgtype.UUID { return pgtype.UUID{Bytes: uuid.New(), Valid: true} }

// ---------------------------------------------------------------------------
// Test 1: Hallucination guard integration (extractor + validator chain)
// ---------------------------------------------------------------------------

// TestIntelChain_HallucinationGuardIntegration verifies that the extractor +
// validator chain catches hallucinations end-to-end. The mock AI gateway
// returns an extraction where a non-null field lacks evidence (a
// hallucination) and a required field is missing; the validator must reset the
// hallucinated field to null and record validation errors, and the extractor
// must surface those errors in ExtractResult.
//
// This is the end-to-end hallucination guard: it exercises the real
// extractor.Extract (prompt build -> gateway call -> parse -> validate) path
// with a mock gateway, no DB needed.
func TestIntelChain_HallucinationGuardIntegration(t *testing.T) {
	// LLM output: title has evidence (clean), country has NO evidence
	// (hallucination -> must be reset to null), university is non-null with
	// evidence (clean). degree_level is absent (optional, no error).
	body := `{
		"title":{"value":"DAAD EPOS","confidence":0.95,"evidence":"DAAD EPOS programme"},
		"country":{"value":["DEU"],"confidence":0.9,"evidence":""},
		"university":{"value":"DAAD","confidence":0.8,"evidence":"German Academic Exchange Service"}
	}`

	gw := &mockAIGateway{content: body}
	ex := extractor.NewExtractor(gw, extractor.WithMaxRetries(0))

	res, err := ex.Extract(context.Background(), schemas.Scholarship, "https://example.org/daad", "html", "DAAD EPOS programme source text")
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}

	// title survived validation (had evidence).
	if res.Extraction["title"].Value != "DAAD EPOS" {
		t.Errorf("title = %v, want %q (had evidence, should survive)", res.Extraction["title"].Value, "DAAD EPOS")
	}

	// country was a hallucination (non-null, empty evidence) -> must be reset to null.
	if res.Extraction["country"].Value != nil {
		t.Errorf("country = %v, want nil (hallucination: non-null value with empty evidence must be reset)", res.Extraction["country"].Value)
	}

	// university survived (had evidence).
	if res.Extraction["university"].Value != "DAAD" {
		t.Errorf("university = %v, want %q (had evidence, should survive)", res.Extraction["university"].Value, "DAAD")
	}

	// The validator must have recorded at least one error for the country field.
	foundHallucination := false
	for _, verr := range res.Errors {
		if verr.Field == "country" && strings.Contains(verr.Message, "hallucination") {
			foundHallucination = true
		}
	}
	if !foundHallucination {
		t.Errorf("expected a hallucination validation error for country; got %v", res.Errors)
	}

	// The gateway must have been called exactly once (no retries on success).
	if gw.calls != 1 {
		t.Errorf("gateway calls = %d, want 1", gw.calls)
	}
}

// TestIntelChain_HallucinationGuardRequiredFieldMissing verifies that a
// missing required field (title) is recorded as a validation error by the
// full extractor -> validator chain.
func TestIntelChain_HallucinationGuardRequiredFieldMissing(t *testing.T) {
	// title is required but absent; country is present with evidence.
	body := `{
		"country":{"value":["DEU"],"confidence":0.9,"evidence":"Germany"}
	}`
	gw := &mockAIGateway{content: body}
	ex := extractor.NewExtractor(gw, extractor.WithMaxRetries(0))

	res, err := ex.Extract(context.Background(), schemas.Scholarship, "https://example.org/daad", "html", "source")
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}

	// title is required and missing -> must be nil with a validation error.
	if res.Extraction["title"].Value != nil {
		t.Errorf("title = %v, want nil (required field missing)", res.Extraction["title"].Value)
	}
	foundRequired := false
	for _, verr := range res.Errors {
		if verr.Field == "title" && strings.Contains(verr.Message, "required") {
			foundRequired = true
		}
	}
	if !foundRequired {
		t.Errorf("expected a 'required field missing' error for title; got %v", res.Errors)
	}
}

// ---------------------------------------------------------------------------
// Test 2: Editorial override integration (promoter update path)
// ---------------------------------------------------------------------------

// TestIntelChain_EditorialOverrideIntegration verifies that the promoter
// respects editorial overrides on the update path: a field present in
// editorial_overrides is NOT overwritten by a re-crawl, while a non-overridden
// field IS updated. It also asserts an auto_update review-log entry is written
// for the changed (non-overridden) field only.
//
// This wires the real ScholarshipPromoter against a mock Store; no DB needed.
func TestIntelChain_EditorialOverrideIntegration(t *testing.T) {
	recordID := newUUID()
	sourceID := newUUID()
	scholarshipID := newUUID()
	guideID := newUUID()

	// Human-edited deadline that must survive the re-crawl.
	humanDeadline := mustDate(t, "2027-12-31")

	// Re-crawl extraction: deadline changed (must be discarded) and title
	// changed (must be applied).
	newExt := extractor.Extraction{
		"title":    {Value: "MEXT Scholarship 2027 (Updated)", Evidence: ptrString("MEXT updated title")},
		"country":  {Value: []any{"JP"}, Evidence: ptrString("Japan")},
		"deadline": {Value: "2027-10-15", Evidence: ptrString("new deadline")},
	}

	store := &mockPromoterStore{
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
		},
		hasExisting: true,
	}

	p := promoter.NewScholarshipPromoter(store, newUUID(), newUUID(),
		promoter.WithClock(func() time.Time { return time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC) }),
	)

	res, err := p.Promote(context.Background(), uuid.UUID(recordID.Bytes))
	if err != nil {
		t.Fatalf("Promote returned error: %v", err)
	}

	if res.Action != promoter.ActionUpdated {
		t.Fatalf("Action = %q, want %q", res.Action, promoter.ActionUpdated)
	}

	if len(store.updateScholarshipCalls) != 1 {
		t.Fatalf("expected 1 UpdateScholarship call, got %d", len(store.updateScholarshipCalls))
	}
	upd := store.updateScholarshipCalls[0]

	// THE invariant: deadline is still the human-edited value, not the
	// re-crawled 2027-10-15.
	if !upd.Deadline.Valid || !upd.Deadline.Time.Equal(humanDeadline.Time) {
		t.Errorf("deadline after update = %v, want human-edited %v (editorial override must win)",
			upd.Deadline, humanDeadline)
	}

	// The title WAS updated because it is not under editorial override.
	wantTitle := "MEXT Scholarship 2027 (Updated)"
	if upd.Title != wantTitle {
		t.Errorf("title after update = %q, want %q (non-overridden field should update)",
			upd.Title, wantTitle)
	}

	// ChangedFields reports title but NOT deadline.
	if !sliceContains(res.ChangedFields, "title") {
		t.Errorf("ChangedFields = %v, want to contain %q", res.ChangedFields, "title")
	}
	if sliceContains(res.ChangedFields, "deadline") {
		t.Errorf("ChangedFields = %v, must NOT contain %q (deadline is overridden)",
			res.ChangedFields, "deadline")
	}

	// Exactly one auto_update review log entry; its field_changes must
	// reference title but never deadline.
	if len(store.reviewLogCalls) != 1 {
		t.Fatalf("expected 1 review log call, got %d", len(store.reviewLogCalls))
	}
	rl := store.reviewLogCalls[0]
	if rl.Action != "auto_update" {
		t.Errorf("review log action = %q, want %q", rl.Action, "auto_update")
	}
	var changes []struct {
		Field string `json:"field"`
		From  any    `json:"from"`
		To    any    `json:"to"`
	}
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

	// Update path must not create content or scholarship rows.
	if len(store.createContentCalls) != 0 {
		t.Errorf("update path must not create content rows; got %d", len(store.createContentCalls))
	}
	if len(store.createScholarshipCalls) != 0 {
		t.Errorf("update path must not create scholarships; got %d", len(store.createScholarshipCalls))
	}
}

// TestIntelChain_EditorialOverrideCreatePath verifies the promoter create
// path: when no existing scholarship exists, a content guide row + scholarship
// row are inserted and the intel record is marked promoted.
func TestIntelChain_EditorialOverrideCreatePath(t *testing.T) {
	recordID := newUUID()
	sourceID := newUUID()

	ext := extractor.Extraction{
		"title":               {Value: "New Scholarship", Evidence: ptrString("title")},
		"description_summary": {Value: "A summary.", Evidence: ptrString("summary")},
		"country":             {Value: []any{"US"}, Evidence: ptrString("USA")},
		"deadline":            {Value: "2027-06-15", Evidence: ptrString("deadline")},
	}

	store := &mockPromoterStore{
		record: gen.IntelRecord{
			ID:         recordID,
			SourceID:   sourceID,
			EntityType: "scholarship",
			Extracted:  extractionJSON(t, ext),
		},
		source: gen.IntelSource{ID: sourceID, SourceUrl: "https://example.org/new", SourceType: "official", TrustLevel: 3},
		hasExisting: false,
	}

	p := promoter.NewScholarshipPromoter(store, newUUID(), newUUID())

	res, err := p.Promote(context.Background(), uuid.UUID(recordID.Bytes))
	if err != nil {
		t.Fatalf("Promote returned error: %v", err)
	}
	if res.Action != promoter.ActionCreated {
		t.Fatalf("Action = %q, want %q", res.Action, promoter.ActionCreated)
	}
	if len(store.createContentCalls) != 1 {
		t.Fatalf("expected 1 CreateContent call, got %d", len(store.createContentCalls))
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

// ---------------------------------------------------------------------------
// Test 3: Gate classification + freshness + promoter routing
// ---------------------------------------------------------------------------

// TestIntelChain_GateClassificationIntegration verifies that the gate
// classifier routes records correctly based on confidence + trust level, that
// the freshness score decays with age, and that the promoter action (created
// vs updated vs verified) reflects whether the projection already exists.
//
// It wires the real services.Classify, services.FreshnessScore,
// changedetector.Compare, dedup.Service, and promoter.ScholarshipPromoter
// against mocks; no DB needed.
func TestIntelChain_GateClassificationIntegration(t *testing.T) {
	const (
		autoThreshold   = 0.85
		reviewThreshold = 0.50
	)

	// --- Gate routing: high confidence + tier-1 source -> auto-publish. ---
	autoDecision := services.Classify(services.GateInput{
		ConfidenceOverall: 0.92,
		TrustLevel:        1,
		AutoThreshold:     autoThreshold,
		ReviewThreshold:   reviewThreshold,
	})
	if autoDecision != services.GateAutoPublish {
		t.Errorf("high-conf tier1: gate = %d, want %d (GateAutoPublish)", autoDecision, services.GateAutoPublish)
	}

	// --- Gate routing: mid confidence -> review required. ---
	reviewDecision := services.Classify(services.GateInput{
		ConfidenceOverall: 0.60,
		TrustLevel:        2,
		AutoThreshold:     autoThreshold,
		ReviewThreshold:   reviewThreshold,
	})
	if reviewDecision != services.GateReviewRequired {
		t.Errorf("mid-conf: gate = %d, want %d (GateReviewRequired)", reviewDecision, services.GateReviewRequired)
	}

	// --- Gate routing: low confidence + tier-3 source -> reject. ---
	rejectDecision := services.Classify(services.GateInput{
		ConfidenceOverall: 0.40,
		TrustLevel:        3,
		AutoThreshold:     autoThreshold,
		ReviewThreshold:   reviewThreshold,
	})
	if rejectDecision != services.GateReject {
		t.Errorf("low-conf tier3: gate = %d, want %d (GateReject)", rejectDecision, services.GateReject)
	}

	// --- Freshness: today -> 100, this week -> 90, this month -> 70, older -> 40, stale -> 0. ---
	now := time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC)
	freshnessCases := []struct {
		label string
		age   time.Duration
		want  int
	}{
		{"today", 0, 100},
		{"3 days", 3 * 24 * time.Hour, 90},
		{"20 days", 20 * 24 * time.Hour, 70},
		{"60 days", 60 * 24 * time.Hour, 40},
		{"120 days", 120 * 24 * time.Hour, 0},
	}
	for _, fc := range freshnessCases {
		got := services.FreshnessScore(now.Add(-fc.age), now)
		if got != fc.want {
			t.Errorf("freshness(%s): got %d, want %d", fc.label, got, fc.want)
		}
	}

	// --- Change detector: first crawl, unchanged, changed. ---
	if d := changedetector.Compare("abc", ""); d != changedetector.DecisionFirstCrawl {
		t.Errorf("first crawl: decision = %d, want %d", d, changedetector.DecisionFirstCrawl)
	}
	if d := changedetector.Compare("abc", "abc"); d != changedetector.DecisionUnchanged {
		t.Errorf("unchanged: decision = %d, want %d", d, changedetector.DecisionUnchanged)
	}
	if d := changedetector.Compare("abc", "xyz"); d != changedetector.DecisionChanged {
		t.Errorf("changed: decision = %d, want %d", d, changedetector.DecisionChanged)
	}

	// --- Dedup: new record (no rows) vs update (same source re-extraction). ---
	sourceID := newUUID()
	newLister := &mockRecordLister{found: false}
	dedupSvc := dedup.NewService(newLister, pgtype.UUID{Bytes: [16]byte{0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,1}, Valid: true})
	dec, err := dedupSvc.Check(context.Background(), sourceID, "ext-key-1")
	if err != nil {
		t.Fatalf("dedup.Check (new): %v", err)
	}
	if dec != dedup.CheckResultNew {
		t.Errorf("dedup (no rows): result = %s, want %s", dec, dedup.CheckResultNew)
	}

	updateLister := &mockRecordLister{found: true}
	dedupSvc2 := dedup.NewService(updateLister, pgtype.UUID{Bytes: [16]byte{0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,1}, Valid: true})
	dec, err = dedupSvc2.Check(context.Background(), sourceID, "ext-key-1")
	if err != nil {
		t.Fatalf("dedup.Check (update): %v", err)
	}
	if dec != dedup.CheckResultUpdate {
		t.Errorf("dedup (same source): result = %s, want %s", dec, dedup.CheckResultUpdate)
	}

	// --- Promoter routing: create path when no existing projection. ---
	recordID := newUUID()
	promoterSourceID := newUUID()
	ext := extractor.Extraction{
		"title":   {Value: "Routed Scholarship", Evidence: ptrString("title")},
		"country": {Value: []any{"GB"}, Evidence: ptrString("UK")},
	}

	// Create path: no existing scholarship -> ActionCreated.
	createStore := &mockPromoterStore{
		record: gen.IntelRecord{
			ID:         recordID,
			SourceID:   promoterSourceID,
			EntityType: "scholarship",
			Extracted:  extractionJSON(t, ext),
		},
		source:     gen.IntelSource{ID: promoterSourceID, SourceUrl: "https://example.org/r", SourceType: "official", TrustLevel: 2},
		hasExisting: false,
	}
	p := promoter.NewScholarshipPromoter(createStore, newUUID(), newUUID())
	res, err := p.Promote(context.Background(), uuid.UUID(recordID.Bytes))
	if err != nil {
		t.Fatalf("promoter create path: %v", err)
	}
	if res.Action != promoter.ActionCreated {
		t.Errorf("create path: Action = %q, want %q", res.Action, promoter.ActionCreated)
	}

	// Verified path: existing scholarship, identical extraction -> ActionVerified.
	verifiedStore := &mockPromoterStore{
		record: gen.IntelRecord{
			ID:         recordID,
			SourceID:   promoterSourceID,
			EntityType: "scholarship",
			Extracted:  extractionJSON(t, ext),
		},
		source: gen.IntelSource{ID: promoterSourceID, SourceUrl: "https://example.org/r", SourceType: "official", TrustLevel: 2},
		existing: gen.Scholarship{
			ID:                 newUUID(),
			IntelRecordID:      recordID,
			Title:              "Routed Scholarship",
			Country:            "GB",
			EditorialOverrides: []byte(`{}`),
			FreshnessScore:     30,
		},
		hasExisting: true,
	}
	p2 := promoter.NewScholarshipPromoter(verifiedStore, newUUID(), newUUID())
	res, err = p2.Promote(context.Background(), uuid.UUID(recordID.Bytes))
	if err != nil {
		t.Fatalf("promoter verified path: %v", err)
	}
	if res.Action != promoter.ActionVerified {
		t.Errorf("verified path: Action = %q, want %q", res.Action, promoter.ActionVerified)
	}
	if len(verifiedStore.reviewLogCalls) != 0 {
		t.Errorf("verified path must not write review log; got %d", len(verifiedStore.reviewLogCalls))
	}
}

// ---------------------------------------------------------------------------
// Test 4: OCR fallback (PDF text extraction + OCR fallback)
// ---------------------------------------------------------------------------

// TestIntelChain_OCRFallback verifies that:
//  1. A PDF with an embedded text layer is extracted via the text path (no OCR
//     fallback) and the extracted text exceeds the 50-char threshold.
//  2. A scanned PDF (minimal embedded text) triggers the OCR fallback path.
//     This test skips gracefully if tesseract/pdftoppm are not installed.
func TestIntelChain_OCRFallback(t *testing.T) {
	// 1. Embedded-text PDF: text extraction succeeds, OCR is NOT invoked.
	embeddedBody, err := os.ReadFile("fetcher/testdata/scholarship_gov_pdf.pdf")
	if err != nil {
		t.Fatalf("read embedded-text PDF fixture: %v", err)
	}

	// ocrFallback is non-nil but we assert it is never called by checking that
	// the text path returns > 50 chars (the threshold below which OCR kicks in).
	ocr := fetcher.NewOCRFetcher()
	pdfFetcher := fetcher.NewPDFFetcher(ocr)

	text, err := pdfFetcher.ExtractText(embeddedBody)
	if err != nil {
		t.Fatalf("extract text from embedded PDF: %v", err)
	}
	if len(strings.TrimSpace(text)) < 50 {
		t.Fatalf("embedded-text PDF returned only %d chars, expected >= 50 (OCR fallback should not trigger)",
			len(strings.TrimSpace(text)))
	}
	if !strings.Contains(strings.ToLower(text), "mext") {
		t.Errorf("embedded-text PDF missing expected keyword 'mext': %q", text)
	}

	// 2. Scanned PDF: minimal embedded text -> OCR fallback. Skip if the OCR
	// toolchain is absent.
	if !ocr.Available() {
		t.Skip("tesseract/pdftoppm not installed, skipping OCR fallback test")
	}

	scannedBody, err := os.ReadFile("fetcher/testdata/scholarship_scanned.pdf")
	if err != nil {
		t.Fatalf("read scanned PDF fixture: %v", err)
	}

	ocrText, err := pdfFetcher.ExtractText(scannedBody)
	if err != nil {
		t.Fatalf("OCR fallback extraction failed: %v", err)
	}
	if strings.TrimSpace(ocrText) == "" {
		t.Fatal("OCR fallback returned empty text for scanned PDF")
	}
}

// ---------------------------------------------------------------------------
// Small helpers
// ---------------------------------------------------------------------------

// ptrString returns a pointer to s. Used to build evidence strings inline.
func ptrString(s string) *string { return &s }

// sliceContains reports whether slice contains s.
func sliceContains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
