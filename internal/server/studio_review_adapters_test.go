// Package server — studio_review_adapters_test.go tests the merge-onto-
// original approval path for jobless regen reviews (§75): approving a
// regeneration draft rewrites the original contents row instead of
// publishing a second live copy.
//
// Ref: japaconnect-engineering/sections/75-ai-content-studio.md (Review Routing)
package server

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db/gen"
)

// stubRegenContentStore implements regenContentStore for tests.
type stubRegenContentStore struct {
	rows         map[string]gen.GetContentByIDRow
	updateCalled bool
	updatedID    pgtype.UUID
	updatedBy    pgtype.UUID
	updInput     content.UpdateContentInput
	updateErr    error
	deletedID    pgtype.UUID
	deleteErr    error
	getErr       error
}

func (s *stubRegenContentStore) GetContent(ctx context.Context, id pgtype.UUID) (gen.GetContentByIDRow, error) {
	if s.getErr != nil {
		return gen.GetContentByIDRow{}, s.getErr
	}

	row, ok := s.rows[id.String()]
	if !ok {
		return gen.GetContentByIDRow{}, fmt.Errorf("content %s not found", id)
	}

	return row, nil
}

func (s *stubRegenContentStore) UpdateContent(ctx context.Context, id, editedBy pgtype.UUID, input content.UpdateContentInput, bypassEditWindow bool) (gen.UpdateContentRow, error) {
	if s.updateErr != nil {
		return gen.UpdateContentRow{}, s.updateErr
	}

	s.updateCalled = true
	s.updatedID = id
	s.updatedBy = editedBy
	s.updInput = input

	return gen.UpdateContentRow{}, nil
}

func (s *stubRegenContentStore) DeleteContent(ctx context.Context, id pgtype.UUID) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}

	s.deletedID = id

	return nil
}

func regenDraftFV(t *testing.T, prevID, intelID string) []byte {
	t.Helper()

	return []byte(fmt.Sprintf(`{"_ai_provenance": {"ai_generated": true, "source": "regenerate_from_source", "previous_content_id": %q, "intel_record_id": %q, "ai_model": "test-model"}}`, prevID, intelID))
}

func TestMergeRegenDraft_MergesOntoOriginal(t *testing.T) {
	t.Parallel()

	draftID := mustUUIDSrv(t, "00000000-0000-0000-0000-0000000000d1")
	originalID := mustUUIDSrv(t, "00000000-0000-0000-0000-0000000000d2")
	intelID := mustUUIDSrv(t, "00000000-0000-0000-0000-0000000000d3")
	reviewerID := mustUUIDSrv(t, "00000000-0000-0000-0000-0000000000d4")

	store := &stubRegenContentStore{rows: map[string]gen.GetContentByIDRow{
		draftID.String(): {
			ID:          draftID,
			Title:       "Regenerated guide",
			Body:        "new AI body",
			Status:      "draft",
			FieldValues: regenDraftFV(t, originalID.String(), intelID.String()),
		},
		originalID.String(): {
			ID:           originalID,
			Slug:         "original-slug",
			Status:       "published",
			Visibility:   "public",
			SeoIndexable: true,
		},
	}}

	m := &regenDraftMergerAdapter{contentSvc: store}

	gotIntel, merged, err := m.MergeRegenDraft(t.Context(), draftID, reviewerID)
	if err != nil {
		t.Fatalf("MergeRegenDraft: %v", err)
	}

	if !merged {
		t.Fatal("expected merged=true for a regen draft")
	}

	if gotIntel != intelID {
		t.Errorf("intel_record_id = %v, want %v", gotIntel, intelID)
	}

	if store.updatedID != originalID {
		t.Errorf("updated id = %v, want original %v", store.updatedID, originalID)
	}

	if store.updatedBy != reviewerID {
		t.Errorf("edited_by = %v, want reviewer %v", store.updatedBy, reviewerID)
	}

	if store.updInput.Body != "new AI body" {
		t.Errorf("merged body = %q, want draft body", store.updInput.Body)
	}

	if store.updInput.Slug != "original-slug" || store.updInput.Status != "published" {
		t.Errorf("identity fields changed: slug=%q status=%q", store.updInput.Slug, store.updInput.Status)
	}

	if store.updInput.ContentHash == nil || *store.updInput.ContentHash != content.ComputeContentHash("new AI body") {
		t.Error("expected content_hash recomputed from the merged body")
	}

	if store.deletedID != draftID {
		t.Errorf("deleted draft = %v, want %v", store.deletedID, draftID)
	}
}

func TestMergeRegenDraft_NoProvenance_NotMerged(t *testing.T) {
	t.Parallel()

	draftID := mustUUIDSrv(t, "00000000-0000-0000-0000-0000000000d1")

	store := &stubRegenContentStore{rows: map[string]gen.GetContentByIDRow{
		draftID.String(): {
			ID:          draftID,
			Status:      "draft",
			FieldValues: []byte(`{"_ai_provenance": {"source": "studio"}}`),
		},
	}}

	m := &regenDraftMergerAdapter{contentSvc: store}

	_, merged, err := m.MergeRegenDraft(t.Context(), draftID, mustUUIDSrv(t, "00000000-0000-0000-0000-0000000000d4"))
	if err != nil {
		t.Fatalf("MergeRegenDraft: %v", err)
	}

	if merged {
		t.Error("expected merged=false without previous_content_id")
	}

	if store.updateCalled || store.deletedID.Valid {
		t.Error("no update/delete should run for a non-regen draft")
	}
}

func TestMergeRegenDraft_MissingOriginal_FailsClosed(t *testing.T) {
	t.Parallel()

	draftID := mustUUIDSrv(t, "00000000-0000-0000-0000-0000000000d1")
	originalID := mustUUIDSrv(t, "00000000-0000-0000-0000-0000000000d2")

	store := &stubRegenContentStore{rows: map[string]gen.GetContentByIDRow{
		draftID.String(): {
			ID:          draftID,
			Status:      "draft",
			FieldValues: regenDraftFV(t, originalID.String(), ""),
		},
	}}

	m := &regenDraftMergerAdapter{contentSvc: store}

	_, merged, err := m.MergeRegenDraft(t.Context(), draftID, mustUUIDSrv(t, "00000000-0000-0000-0000-0000000000d4"))
	if err == nil {
		t.Fatal("expected error when the original row is gone")
	}

	if merged || store.updateCalled {
		t.Error("merge must fail closed — no update when the original is missing")
	}
}

func TestMergeRegenDraft_DraftLoadError_Propagates(t *testing.T) {
	t.Parallel()

	store := &stubRegenContentStore{getErr: errors.New("db down")}
	m := &regenDraftMergerAdapter{contentSvc: store}

	if _, merged, err := m.MergeRegenDraft(t.Context(), mustUUIDSrv(t, "00000000-0000-0000-0000-0000000000d1"), mustUUIDSrv(t, "00000000-0000-0000-0000-0000000000d4")); err == nil || merged {
		t.Errorf("expected (false, err) on draft load failure, got merged=%v err=%v", merged, err)
	}
}

// ---------------------------------------------------------------------------
// hitlDraftPublisher — §103 "Approve → publish" for non-studio HITL reviews
// ---------------------------------------------------------------------------

type stubHITLContentPub struct {
	calledID pgtype.UUID
	status   string
	err      error
}

func (s *stubHITLContentPub) UpdateContentStatus(ctx context.Context, id pgtype.UUID, status string) (any, error) {
	if s.err != nil {
		return nil, s.err
	}

	s.calledID = id
	s.status = status

	return nil, nil
}

type stubHITLToolTablePub struct {
	calledTable string
	calledID    pgtype.UUID
	reviewer    pgtype.UUID
	err         error
}

func (s *stubHITLToolTablePub) Publish(ctx context.Context, reviewerID pgtype.UUID, targetTable string, targetID pgtype.UUID, notes string) error {
	if s.err != nil {
		return s.err
	}

	s.calledTable = targetTable
	s.calledID = targetID
	s.reviewer = reviewerID

	return nil
}

func TestPublishHITLTarget_Contents(t *testing.T) {
	t.Parallel()

	targetID := mustUUIDSrv(t, "00000000-0000-0000-0000-0000000000e1")
	reviewer := mustUUIDSrv(t, "00000000-0000-0000-0000-0000000000e2")

	contentPub := &stubHITLContentPub{}
	p := &hitlDraftPublisher{contentPub: contentPub}

	if err := p.PublishHITLTarget(t.Context(), reviewer, "contents", targetID); err != nil {
		t.Fatalf("PublishHITLTarget: %v", err)
	}

	if contentPub.calledID != targetID || contentPub.status != "published" {
		t.Errorf("publish = (%v, %q), want (%v, published)", contentPub.calledID, contentPub.status, targetID)
	}
}

func TestPublishHITLTarget_ToolTable(t *testing.T) {
	t.Parallel()

	targetID := mustUUIDSrv(t, "00000000-0000-0000-0000-0000000000e1")
	reviewer := mustUUIDSrv(t, "00000000-0000-0000-0000-0000000000e2")

	toolPub := &stubHITLToolTablePub{}
	p := &hitlDraftPublisher{toolTablePub: toolPub}

	if err := p.PublishHITLTarget(t.Context(), reviewer, "housing_guides", targetID); err != nil {
		t.Fatalf("PublishHITLTarget: %v", err)
	}

	if toolPub.calledTable != "housing_guides" || toolPub.calledID != targetID || toolPub.reviewer != reviewer {
		t.Errorf("publish = (%q, %v, %v), want (housing_guides, %v, %v)", toolPub.calledTable, toolPub.calledID, toolPub.reviewer, targetID, reviewer)
	}
}

func TestPublishHITLTarget_InvalidID(t *testing.T) {
	t.Parallel()

	p := &hitlDraftPublisher{contentPub: &stubHITLContentPub{}}

	if err := p.PublishHITLTarget(t.Context(), mustUUIDSrv(t, "00000000-0000-0000-0000-0000000000e2"), "contents", pgtype.UUID{}); err == nil {
		t.Fatal("expected error for invalid target id")
	}
}
