package promoter

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/extractor"
)

// mockGuideStore is an in-memory GuideContentStore + GuideContentTxStore.
// It records every call so tests can assert the exact write sequence.
type mockGuideStore struct {
	record       gen.IntelRecord
	source       gen.IntelSource
	content      gen.GetContentByIDRow
	hasContent   bool
	contentErr   error
	newContentID pgtype.UUID

	createContentCalls []gen.CreateContentParams
	updateContentCalls []gen.UpdateContentParams
	reviewLogCalls     []gen.CreateIntelReviewLogParams
	promotedCalls      []gen.UpdateIntelRecordPromotedParams
}

func (m *mockGuideStore) GetIntelRecord(_ context.Context, _ pgtype.UUID) (gen.IntelRecord, error) {
	return m.record, nil
}

func (m *mockGuideStore) GetIntelSource(_ context.Context, _ pgtype.UUID) (gen.IntelSource, error) {
	return m.source, nil
}

func (m *mockGuideStore) GetPersonaUserID(_ context.Context, _ pgtype.UUID) (pgtype.UUID, error) {
	return pgtype.UUID{}, nil
}

func (m *mockGuideStore) CreateContent(_ context.Context, arg gen.CreateContentParams) (gen.CreateContentRow, error) {
	m.createContentCalls = append(m.createContentCalls, arg)
	if m.newContentID.Valid {
		return gen.CreateContentRow{ID: m.newContentID, Title: arg.Title, Slug: arg.Slug}, nil
	}

	return gen.CreateContentRow{ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}, Title: arg.Title, Slug: arg.Slug}, nil
}

func (m *mockGuideStore) GetContentByID(_ context.Context, _ pgtype.UUID) (gen.GetContentByIDRow, error) {
	if m.contentErr != nil {
		return gen.GetContentByIDRow{}, m.contentErr
	}
	if !m.hasContent {
		return gen.GetContentByIDRow{}, pgx.ErrNoRows
	}

	return m.content, nil
}

func (m *mockGuideStore) UpdateContent(_ context.Context, arg gen.UpdateContentParams) (gen.UpdateContentRow, error) {
	m.updateContentCalls = append(m.updateContentCalls, arg)
	return gen.UpdateContentRow{ID: arg.ID, Title: arg.Title, Slug: arg.Slug}, nil
}

func (m *mockGuideStore) UpdateIntelRecordPromoted(_ context.Context, arg gen.UpdateIntelRecordPromotedParams) error {
	m.promotedCalls = append(m.promotedCalls, arg)
	return nil
}

func (m *mockGuideStore) CreateIntelReviewLog(_ context.Context, arg gen.CreateIntelReviewLogParams) error {
	m.reviewLogCalls = append(m.reviewLogCalls, arg)
	return nil
}

func (m *mockGuideStore) WithTx(_ context.Context, fn func(GuideContentTxStore) error) error {
	return fn(m)
}

func newGuideRecord(t *testing.T, ext extractor.Extraction, promoted pgtype.UUID) gen.IntelRecord {
	t.Helper()

	return gen.IntelRecord{
		ID:               pgtype.UUID{Bytes: uuid.New(), Valid: true},
		SourceID:         pgtype.UUID{Bytes: uuid.New(), Valid: true},
		EntityType:       "grants",
		Extracted:        extractionJSON(t, ext),
		PromotedRecordID: promoted,
	}
}

func newGuideSource() gen.IntelSource {
	return gen.IntelSource{
		ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
		Name:      "Grants Feed",
		SourceUrl: "https://example.org/feed",
	}
}

func TestGuideContentPromoter_UpdateWritesRealFields(t *testing.T) {
	t.Parallel()

	contentID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	record := newGuideRecord(t, extractor.Extraction{
		"title":   {Value: "New Grant Title"},
		"summary": {Value: "updated summary"},
	}, contentID)

	existingBody := "Old Grant Title\n\n- summary: \"old summary\"\n"

	store := &mockGuideStore{
		record: record,
		source: newGuideSource(),
		content: gen.GetContentByIDRow{
			ID:            contentID,
			Title:         "Old Grant Title",
			Slug:          "old-grant-title",
			Body:          existingBody,
			FieldValues:   []byte(`{"summary":"old summary"}`),
			Status:        "published",
			Visibility:    "public",
			GuideCategory: strPtr("grants"),
		},
		hasContent: true,
	}

	p := NewGuideContentPromoter(store, pgtype.UUID{Bytes: uuid.New(), Valid: true}, "grants", "grants")

	res, err := p.Promote(context.Background(), record.ID.Bytes)
	if err != nil {
		t.Fatalf("promote: %v", err)
	}

	if res.Action != ActionUpdated {
		t.Fatalf("action = %q, want %q", res.Action, ActionUpdated)
	}
	if res.ContentID != contentID.Bytes {
		t.Fatalf("content id = %v, want %v", res.ContentID, contentID.Bytes)
	}
	if len(store.updateContentCalls) != 1 {
		t.Fatalf("UpdateContent calls = %d, want 1", len(store.updateContentCalls))
	}

	upd := store.updateContentCalls[0]
	if upd.Title != "New Grant Title" {
		t.Errorf("title = %q, want New Grant Title", upd.Title)
	}
	if upd.Slug != "old-grant-title" {
		t.Errorf("slug = %q, want preserved old-grant-title", upd.Slug)
	}
	if !strings.Contains(upd.Body, "updated summary") {
		t.Errorf("body missing new summary: %q", upd.Body)
	}
	if upd.Status != "published" || upd.Visibility != "public" {
		t.Errorf("editorial status/visibility not preserved: %q/%q", upd.Status, upd.Visibility)
	}
	if !strings.Contains(string(upd.FieldValues), `"summary":"updated summary"`) {
		t.Errorf("field_values = %s, want refreshed extraction", upd.FieldValues)
	}
	if upd.ContentHash == nil || *upd.ContentHash == "" {
		t.Error("content_hash not stamped")
	}

	if len(store.reviewLogCalls) != 1 {
		t.Fatalf("review log calls = %d, want 1", len(store.reviewLogCalls))
	}
	log := store.reviewLogCalls[0]
	if log.Action != "auto_update" {
		t.Errorf("log action = %q, want auto_update", log.Action)
	}

	var changes []fieldChange
	if err := json.Unmarshal(log.FieldChanges, &changes); err != nil {
		t.Fatalf("field_changes not valid JSON: %v", err)
	}
	if len(changes) == 0 {
		t.Error("auto_update log has empty field_changes")
	}
}

func TestGuideContentPromoter_UpdateNoChangesVerifies(t *testing.T) {
	t.Parallel()

	contentID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	// Single-field extraction keeps guideBodyFromExtraction deterministic —
	// it iterates the extraction map in random order.
	ext := extractor.Extraction{
		"title": {Value: "Same Title"},
	}
	record := newGuideRecord(t, ext, contentID)

	// Seed the contents row to exactly what the update path would derive.
	body := guideBodyFromExtraction(ext, "Same Title")

	store := &mockGuideStore{
		record: record,
		source: newGuideSource(),
		content: gen.GetContentByIDRow{
			ID:           contentID,
			Title:        "Same Title",
			Slug:         "same-title",
			Body:         body,
			FieldValues:  []byte(`{"title":"Same Title"}`),
			Status:       "published",
			Visibility:   "public",
			CanonicalUrl: strPtr("https://example.org/feed"),
		},
		hasContent: true,
	}

	p := NewGuideContentPromoter(store, pgtype.UUID{Bytes: uuid.New(), Valid: true}, "grants", "grants")

	res, err := p.Promote(context.Background(), record.ID.Bytes)
	if err != nil {
		t.Fatalf("promote: %v", err)
	}

	if res.Action != ActionVerified {
		t.Fatalf("action = %q, want %q", res.Action, ActionVerified)
	}
	if len(store.updateContentCalls) != 0 {
		t.Fatalf("UpdateContent calls = %d, want 0", len(store.updateContentCalls))
	}
	if len(store.reviewLogCalls) != 0 {
		t.Fatalf("review log calls = %d, want 0 on verify", len(store.reviewLogCalls))
	}
}

func TestGuideContentPromoter_UpdateSkipsLockedRow(t *testing.T) {
	t.Parallel()

	contentID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	record := newGuideRecord(t, extractor.Extraction{
		"title": {Value: "Crawler Wants This"},
	}, contentID)

	store := &mockGuideStore{
		record: record,
		source: newGuideSource(),
		content: gen.GetContentByIDRow{
			ID:       contentID,
			Title:    "Editor Locked Title",
			Slug:     "editor-locked-title",
			Body:     "locked body",
			IsLocked: true,
			Status:   "published",
		},
		hasContent: true,
	}

	p := NewGuideContentPromoter(store, pgtype.UUID{Bytes: uuid.New(), Valid: true}, "grants", "grants")

	res, err := p.Promote(context.Background(), record.ID.Bytes)
	if err != nil {
		t.Fatalf("promote: %v", err)
	}

	if res.Action != ActionVerified {
		t.Fatalf("action = %q, want %q", res.Action, ActionVerified)
	}
	if len(store.updateContentCalls) != 0 {
		t.Fatalf("UpdateContent ran on a locked row")
	}
	if len(store.reviewLogCalls) != 1 {
		t.Fatalf("review log calls = %d, want 1 lock-skip note", len(store.reviewLogCalls))
	}
	if note := store.reviewLogCalls[0].Note; note == nil || !strings.Contains(*note, "locked") {
		t.Errorf("lock-skip note missing: %v", note)
	}
}

func TestGuideContentPromoter_UpdateRecreatesMissingRow(t *testing.T) {
	t.Parallel()

	contentID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	newID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	record := newGuideRecord(t, extractor.Extraction{
		"title": {Value: "Recreated Guide"},
	}, contentID)

	store := &mockGuideStore{
		record:       record,
		source:       newGuideSource(),
		contentErr:   pgx.ErrNoRows,
		newContentID: newID,
	}

	p := NewGuideContentPromoter(store, pgtype.UUID{Bytes: uuid.New(), Valid: true}, "grants", "grants")

	res, err := p.Promote(context.Background(), record.ID.Bytes)
	if err != nil {
		t.Fatalf("promote: %v", err)
	}

	if res.Action != ActionCreated {
		t.Fatalf("action = %q, want %q", res.Action, ActionCreated)
	}
	if len(store.createContentCalls) != 1 {
		t.Fatalf("CreateContent calls = %d, want 1", len(store.createContentCalls))
	}
	if len(store.promotedCalls) != 1 || store.promotedCalls[0].PromotedRecordID != newID {
		t.Fatalf("promoted_record_id not repointed to new content: %+v", store.promotedCalls)
	}
	if res.ContentID != newID.Bytes {
		t.Fatalf("content id = %v, want %v", res.ContentID, newID.Bytes)
	}
}

func TestGuideContentPromoter_UpdateFieldValuesDiffLogged(t *testing.T) {
	t.Parallel()

	contentID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	record := newGuideRecord(t, extractor.Extraction{
		"title":   {Value: "Same"},
		"deadline": {Value: "2027-01-01"},
	}, contentID)

	store := &mockGuideStore{
		record: record,
		source: newGuideSource(),
		content: gen.GetContentByIDRow{
			ID:           contentID,
			Title:        "Same",
			Slug:         "same",
			Body:         "Same\n\n- deadline: \"2026-01-01\"\n- title: \"Same\"\n",
			FieldValues:  []byte(`{"deadline":"2026-01-01"}`),
			Status:       "published",
			CanonicalUrl: strPtr("https://example.org/feed"),
		},
		hasContent: true,
	}

	p := NewGuideContentPromoter(store, pgtype.UUID{Bytes: uuid.New(), Valid: true}, "grants", "grants")

	res, err := p.Promote(context.Background(), record.ID.Bytes)
	if err != nil {
		t.Fatalf("promote: %v", err)
	}

	if res.Action != ActionUpdated {
		t.Fatalf("action = %q, want %q", res.Action, ActionUpdated)
	}
	if len(store.reviewLogCalls) != 1 {
		t.Fatalf("review log calls = %d, want 1", len(store.reviewLogCalls))
	}

	var changes []fieldChange
	if err := json.Unmarshal(store.reviewLogCalls[0].FieldChanges, &changes); err != nil {
		t.Fatalf("field_changes not valid JSON: %v", err)
	}

	var sawFV bool
	for _, c := range changes {
		if strings.HasPrefix(c.Field, "field_values.") {
			sawFV = true
		}
	}
	if !sawFV {
		t.Errorf("expected a field_values.* change entry, got %+v", changes)
	}
}

func TestGuideContentPromoter_GetContentErrorPropagates(t *testing.T) {
	t.Parallel()

	contentID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	record := newGuideRecord(t, extractor.Extraction{
		"title": {Value: "x"},
	}, contentID)

	store := &mockGuideStore{
		record:     record,
		source:     newGuideSource(),
		contentErr: errors.New("db gone"),
	}

	p := NewGuideContentPromoter(store, pgtype.UUID{Bytes: uuid.New(), Valid: true}, "grants", "grants")

	if _, err := p.Promote(context.Background(), record.ID.Bytes); err == nil {
		t.Fatal("expected error to propagate")
	}
}
