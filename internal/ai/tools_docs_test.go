package ai

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// stubDocsService implements DocsService, capturing the last requests.
type stubDocsService struct {
	createReq  *DocsCreatePageRequest
	updateReq  *DocsUpdatePageRequest
	approvedBy pgtype.UUID
	published  pgtype.UUID
	approveErr error
	publishErr error
}

func (s *stubDocsService) ListAllDocPages(ctx context.Context) ([]DocsPage, error) {
	return []DocsPage{}, nil
}

func (s *stubDocsService) GetDocPageByID(ctx context.Context, id pgtype.UUID) (*DocsPage, error) {
	return &DocsPage{ID: id.String()}, nil
}

func (s *stubDocsService) CreateDocPage(ctx context.Context, req DocsCreatePageRequest, userID pgtype.UUID) (pgtype.UUID, error) {
	s.createReq = &req

	var id pgtype.UUID
	_ = id.Scan("11111111-1111-1111-1111-111111111111")

	return id, nil
}

func (s *stubDocsService) UpdateDocPage(ctx context.Context, id pgtype.UUID, req DocsUpdatePageRequest) error {
	s.updateReq = &req
	return nil
}

func (s *stubDocsService) ApproveDocPage(ctx context.Context, id, reviewerID pgtype.UUID) error {
	s.approvedBy = reviewerID
	return s.approveErr
}

func (s *stubDocsService) PublishDocPage(ctx context.Context, id, reviewerID pgtype.UUID) error {
	s.published = id
	return s.publishErr
}

func TestDocsCreateHandler_TaxonomyFields(t *testing.T) {
	svc := &stubDocsService{}
	h := DocsCreateHandler(&ToolDeps{Docs: svc})

	cat, sub := "getting-started", "account"

	res, err := h(t.Context(), mustJSON(t, map[string]any{
		"title":       "Getting Started",
		"slug":        "getting-started",
		"audience":    "users",
		"category":    cat,
		"subcategory": sub,
	}), mustUUID(t, "33333333-3333-3333-3333-333333333333"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if res.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %s, want success", res.Status)
	}

	if svc.createReq == nil {
		t.Fatal("create request not captured")
	}

	if svc.createReq.Category == nil || *svc.createReq.Category != cat {
		t.Errorf("category = %v, want %q", svc.createReq.Category, cat)
	}

	if svc.createReq.Subcategory == nil || *svc.createReq.Subcategory != sub {
		t.Errorf("subcategory = %v, want %q", svc.createReq.Subcategory, sub)
	}

	if !svc.createReq.AIGenerated {
		t.Error("AIGenerated should be forced true")
	}
}

func TestDocsCreateHandler_RejectsPublishStatus(t *testing.T) {
	svc := &stubDocsService{}
	h := DocsCreateHandler(&ToolDeps{Docs: svc})

	res, err := h(t.Context(), mustJSON(t, map[string]any{
		"title":         "T",
		"slug":          "t",
		"audience":      "users",
		"review_status": "published",
	}), mustUUID(t, "33333333-3333-3333-3333-333333333333"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if svc.createReq != nil {
		t.Error("service must not be called for rejected status")
	}

	if res.Status != ToolCallStatusError {
		t.Errorf("status = %s, want error", res.Status)
	}
}

func TestDocsUpdateHandler_TaxonomyAndHierarchy(t *testing.T) {
	svc := &stubDocsService{}
	h := DocsUpdateHandler(&ToolDeps{Docs: svc})

	cat := "billing"
	pos := int32(3)

	res, err := h(t.Context(), mustJSON(t, map[string]any{
		"doc_id":   "11111111-1111-1111-1111-111111111111",
		"category": cat,
		"position": pos,
	}), mustUUID(t, "33333333-3333-3333-3333-333333333333"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if res.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %s, want success", res.Status)
	}

	if svc.updateReq.Category == nil || *svc.updateReq.Category != cat {
		t.Errorf("category = %v, want %q", svc.updateReq.Category, cat)
	}

	if svc.updateReq.Position == nil || *svc.updateReq.Position != pos {
		t.Errorf("position = %v, want %d", svc.updateReq.Position, pos)
	}
}

func TestDocsApproveHandler_RecordsReviewer(t *testing.T) {
	svc := &stubDocsService{}
	h := DocsApproveHandler(&ToolDeps{Docs: svc})
	user := mustUUID(t, "33333333-3333-3333-3333-333333333333")

	res, err := h(t.Context(), mustJSON(t, map[string]any{
		"doc_id": "11111111-1111-1111-1111-111111111111",
	}), user)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if res.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %s, want success", res.Status)
	}

	if svc.approvedBy != user {
		t.Errorf("reviewer = %v, want calling user %v", svc.approvedBy, user)
	}
}

func TestDocsPublishHandler_ServiceError(t *testing.T) {
	svc := &stubDocsService{publishErr: errors.New("cannot publish status \"draft\", must be 'approved'")}
	h := DocsPublishHandler(&ToolDeps{Docs: svc})

	res, err := h(t.Context(), mustJSON(t, map[string]any{
		"doc_id": "11111111-1111-1111-1111-111111111111",
	}), mustUUID(t, "33333333-3333-3333-3333-333333333333"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if res.Status != ToolCallStatusError {
		t.Errorf("status = %s, want error", res.Status)
	}
}

func TestDocsApproveHandler_InvalidDocID(t *testing.T) {
	svc := &stubDocsService{}
	h := DocsApproveHandler(&ToolDeps{Docs: svc})

	res, err := h(t.Context(), mustJSON(t, map[string]any{"doc_id": "nope"}), mustUUID(t, "33333333-3333-3333-3333-333333333333"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if res.Status != ToolCallStatusError {
		t.Errorf("status = %s, want error", res.Status)
	}

	if svc.approvedBy.Valid {
		t.Error("service must not be called for invalid doc_id")
	}
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()

	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	return b
}
