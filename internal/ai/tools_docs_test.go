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
	deleted    pgtype.UUID
	approveErr error
	publishErr error
	deleteErr  error
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

func (s *stubDocsService) DeleteDocPage(ctx context.Context, id pgtype.UUID) error {
	s.deleted = id
	return s.deleteErr
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

func TestDocsUpdateHandler_AbsentContentPreservesBody(t *testing.T) {
	svc := &stubDocsService{}
	h := DocsUpdateHandler(&ToolDeps{Docs: svc})

	// An update that only touches metadata must pass nil content so the
	// service keeps the stored body — normalizing an absent field into an
	// empty doc would wipe it.
	res, err := h(t.Context(), mustJSON(t, map[string]any{
		"doc_id": "11111111-1111-1111-1111-111111111111",
		"title":  "New Title",
	}), mustUUID(t, "33333333-3333-3333-3333-333333333333"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if res.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %s, want success", res.Status)
	}

	if svc.updateReq.Content != nil {
		t.Errorf("content = %s, want nil (preserve stored body)", svc.updateReq.Content)
	}
}

func TestDocsUpdateHandler_StringEncodedContentUnwraps(t *testing.T) {
	svc := &stubDocsService{}
	h := DocsUpdateHandler(&ToolDeps{Docs: svc})

	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"hi"}]}]}`

	res, err := h(t.Context(), mustJSON(t, map[string]any{
		"doc_id":  "11111111-1111-1111-1111-111111111111",
		"content": doc,
	}), mustUUID(t, "33333333-3333-3333-3333-333333333333"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if res.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %s, want success", res.Status)
	}

	var parsed map[string]any
	if err := json.Unmarshal(svc.updateReq.Content, &parsed); err != nil {
		t.Fatalf("stored content not JSON: %v", err)
	}
	if parsed["type"] != "doc" {
		t.Fatalf("content type = %v, want doc object, not quoted string", parsed["type"])
	}
}

func TestDocsUpdateHandler_ScalarStringWrapsAsText(t *testing.T) {
	svc := &stubDocsService{}
	h := DocsUpdateHandler(&ToolDeps{Docs: svc})

	// "123" is valid JSON but a scalar — it must not be stored as a bare
	// number document; it becomes paragraph text instead.
	res, err := h(t.Context(), mustJSON(t, map[string]any{
		"doc_id":  "11111111-1111-1111-1111-111111111111",
		"content": "123",
	}), mustUUID(t, "33333333-3333-3333-3333-333333333333"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if res.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %s, want success", res.Status)
	}

	var parsed map[string]any
	if err := json.Unmarshal(svc.updateReq.Content, &parsed); err != nil {
		t.Fatalf("content not a doc object: %v — scalar strings must wrap as text", err)
	}
	if parsed["type"] != "doc" {
		t.Fatalf("type = %v, want doc", parsed["type"])
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

func TestDocsDeleteHandler_Deletes(t *testing.T) {
	svc := &stubDocsService{}
	h := DocsDeleteHandler(&ToolDeps{Docs: svc})

	res, err := h(t.Context(), mustJSON(t, map[string]any{
		"doc_id": "11111111-1111-1111-1111-111111111111",
	}), mustUUID(t, "33333333-3333-3333-3333-333333333333"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if res.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %s, want success", res.Status)
	}

	if svc.deleted.String() != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("deleted = %v, want the requested doc id", svc.deleted)
	}
}

func TestDocsDeleteHandler_InvalidDocID(t *testing.T) {
	svc := &stubDocsService{}
	h := DocsDeleteHandler(&ToolDeps{Docs: svc})

	res, err := h(t.Context(), mustJSON(t, map[string]any{"doc_id": "nope"}), mustUUID(t, "33333333-3333-3333-3333-333333333333"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if res.Status != ToolCallStatusError {
		t.Errorf("status = %s, want error", res.Status)
	}

	if svc.deleted.Valid {
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
