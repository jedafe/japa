package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/content/visibility"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/visitor"
)

// stubContentLifecycleService is a minimal ContentLifecycleService for bulk
// publish tests.
type stubContentLifecycleService struct {
	updateStatusFunc func(ctx context.Context, id pgtype.UUID, status string) (gen.UpdateContentStatusRow, error)
	listErr          error
	deleteErr        error
}

func (s *stubContentLifecycleService) DeleteContent(ctx context.Context, id pgtype.UUID) error {
	return s.deleteErr
}

func (s *stubContentLifecycleService) ListContentsByType(ctx context.Context, contentType, guideCategory string, cursor pgtype.Timestamptz, cursorID pgtype.UUID, offset, limit int32, sort string, vctx visitor.VisitorContext, filter visibility.VisibilityFilter) ([]gen.ListContentsByTypeRow, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}

	return nil, nil
}

func (s *stubContentLifecycleService) UpdateContentStatus(ctx context.Context, id pgtype.UUID, status string) (gen.UpdateContentStatusRow, error) {
	if s.updateStatusFunc != nil {
		return s.updateStatusFunc(ctx, id, status)
	}

	return gen.UpdateContentStatusRow{}, nil
}

// ---------------------------------------------------------------------------
// content.bulk_create
// ---------------------------------------------------------------------------

func TestContentBulkCreateHandler_Success(t *testing.T) {
	t.Parallel()

	userID := mustUUID(t, "00000000-0000-0000-0000-000000000099")
	contentID1 := mustUUID(t, "00000000-0000-0000-0000-000000000001")
	contentID2 := mustUUID(t, "00000000-0000-0000-0000-000000000002")

	created := 0
	contentSvc := &stubContentTypesService{
		types: []ContentType{
			{ContentType: "blog_post", Label: "Blog Post"},
		},
	}
	permSvc := &stubPerms{granted: map[string]bool{
		"content.create":               true,
		"agent.content_type.blog_post": true,
	}}

	deps := &ToolDeps{
		ContentTypes: contentSvc,
		Permission:   permSvc,
		ContentCreator: func(ctx context.Context, tenantID pgtype.UUID, input ContentCreateInput) (ContentCreateRow, error) {
			created++
			if created == 1 {
				return ContentCreateRow{ID: contentID1, Title: input.Title, Slug: "first", Status: "draft", AuthorID: input.AuthorID}, nil
			}
			return ContentCreateRow{ID: contentID2, Title: input.Title, Slug: "second", Status: "draft", AuthorID: input.AuthorID}, nil
		},
	}

	handler := ContentBulkCreateHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(`{
		"items":[
			{"title":"First","content_type":"blog_post"},
			{"title":"Second","content_type":"blog_post"}
		]
	}`), userID, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}

	m := parseToolResult(t, tr)
	if m["succeeded"].(float64) != 2 {
		t.Errorf("expected 2 succeeded, got %v", m["succeeded"])
	}

	if m["failed"].(float64) != 0 {
		t.Errorf("expected 0 failed, got %v", m["failed"])
	}

	if created != 2 {
		t.Errorf("expected 2 creates, got %d", created)
	}
}

func TestContentBulkCreateHandler_PartialFailure(t *testing.T) {
	t.Parallel()

	userID := mustUUID(t, "00000000-0000-0000-0000-000000000099")
	contentID := mustUUID(t, "00000000-0000-0000-0000-000000000001")

	contentSvc := &stubContentTypesService{
		types: []ContentType{
			{ContentType: "blog_post", Label: "Blog Post"},
		},
	}
	permSvc := &stubPerms{granted: map[string]bool{
		"content.create":               true,
		"agent.content_type.blog_post": true,
	}}

	deps := &ToolDeps{
		ContentTypes: contentSvc,
		Permission:   permSvc,
		ContentCreator: func(ctx context.Context, tenantID pgtype.UUID, input ContentCreateInput) (ContentCreateRow, error) {
			return ContentCreateRow{ID: contentID, Title: input.Title, Slug: "ok", Status: "draft", AuthorID: input.AuthorID}, nil
		},
	}

	handler := ContentBulkCreateHandler(deps)

	tr, _ := handler(t.Context(), json.RawMessage(`{
		"items":[
			{"title":"OK","content_type":"blog_post"},
			{"content_type":"blog_post"}
		]
	}`), userID, pgtype.UUID{})
	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}

	m := parseToolResult(t, tr)
	if m["succeeded"].(float64) != 1 {
		t.Errorf("expected 1 succeeded, got %v", m["succeeded"])
	}

	if m["failed"].(float64) != 1 {
		t.Errorf("expected 1 failed, got %v", m["failed"])
	}
}

func TestContentBulkCreateHandler_UnauthorizedAuthor(t *testing.T) {
	t.Parallel()

	userID := mustUUID(t, "00000000-0000-0000-0000-000000000099")
	otherID := mustUUID(t, "00000000-0000-0000-0000-000000000098")

	contentSvc := &stubContentTypesService{
		types: []ContentType{
			{ContentType: "blog_post", Label: "Blog Post"},
		},
	}
	permSvc := &stubPerms{granted: map[string]bool{
		"content.create":               true,
		"agent.content_type.blog_post": true,
	}}

	deps := &ToolDeps{
		ContentTypes: contentSvc,
		Permission:   permSvc,
		ContentCreator: func(ctx context.Context, tenantID pgtype.UUID, input ContentCreateInput) (ContentCreateRow, error) {
			t.Fatal("creator should not be called without content.assign_author")
			return ContentCreateRow{}, nil
		},
	}

	handler := ContentBulkCreateHandler(deps)
	args := fmt.Sprintf(`{"items":[{"title":"T","content_type":"blog_post","author_id":"%s"}]}`, otherID.String())

	tr, _ := handler(t.Context(), json.RawMessage(args), userID, pgtype.UUID{})
	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success (partial failure)", tr.Status)
	}

	m := parseToolResult(t, tr)
	if m["succeeded"].(float64) != 0 {
		t.Errorf("expected 0 succeeded, got %v", m["succeeded"])
	}

	if m["failed"].(float64) != 1 {
		t.Errorf("expected 1 failed, got %v", m["failed"])
	}

	results, ok := m["results"].([]any)
	if !ok || len(results) != 1 {
		t.Fatalf("expected 1 result, got %v", results)
	}

	res := results[0].(map[string]any)
	if res["error_code"] != "unauthorized_author" {
		t.Errorf("expected unauthorized_author, got %v", res["error_code"])
	}
}

func TestContentBulkCreateHandler_UnauthorizedContentType(t *testing.T) {
	t.Parallel()

	userID := mustUUID(t, "00000000-0000-0000-0000-000000000099")

	contentSvc := &stubContentTypesService{
		types: []ContentType{
			{ContentType: "blog_post", Label: "Blog Post"},
		},
	}
	permSvc := &stubPerms{granted: map[string]bool{
		"content.create":               true,
		"agent.content_type.blog_post": true,
	}}

	deps := &ToolDeps{
		ContentTypes: contentSvc,
		Permission:   permSvc,
		ContentCreator: func(ctx context.Context, tenantID pgtype.UUID, input ContentCreateInput) (ContentCreateRow, error) {
			t.Fatal("creator should not be called for unauthorized content type")
			return ContentCreateRow{}, nil
		},
	}

	handler := ContentBulkCreateHandler(deps)

	tr, _ := handler(t.Context(), json.RawMessage(`{"items":[{"title":"T","content_type":"guide"}]}`), userID, pgtype.UUID{})
	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success (partial failure)", tr.Status)
	}

	m := parseToolResult(t, tr)
	if m["succeeded"].(float64) != 0 {
		t.Errorf("expected 0 succeeded, got %v", m["succeeded"])
	}

	if m["failed"].(float64) != 1 {
		t.Errorf("expected 1 failed, got %v", m["failed"])
	}

	results, ok := m["results"].([]any)
	if !ok || len(results) != 1 {
		t.Fatalf("expected 1 result, got %v", results)
	}

	res := results[0].(map[string]any)
	if res["error_code"] != "unauthorized_content_type" {
		t.Errorf("expected unauthorized_content_type, got %v", res["error_code"])
	}
}

// ---------------------------------------------------------------------------
// content.bulk_publish
// ---------------------------------------------------------------------------

func TestContentBulkPublishHandler_Success(t *testing.T) {
	t.Parallel()

	contentID1 := mustUUID(t, "00000000-0000-0000-0000-000000000001")
	contentID2 := mustUUID(t, "00000000-0000-0000-0000-000000000002")

	updated := 0
	deps := &ToolDeps{
		ContentLifecycle: &stubContentLifecycleService{
			updateStatusFunc: func(ctx context.Context, id pgtype.UUID, status string) (gen.UpdateContentStatusRow, error) {
				updated++
				return gen.UpdateContentStatusRow{ID: id, Title: fmt.Sprintf("Title %d", updated), Slug: fmt.Sprintf("slug-%d", updated), Status: status, UpdatedAt: pgtype.Timestamptz{}}, nil
			},
		},
	}

	handler := ContentBulkPublishHandler(deps)
	args := fmt.Sprintf(`{"content_ids":["%s","%s"]}`, contentID1.String(), contentID2.String())

	tr, err := handler(t.Context(), json.RawMessage(args), pgtype.UUID{}, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}

	m := parseToolResult(t, tr)
	if m["succeeded"].(float64) != 2 {
		t.Errorf("expected 2 succeeded, got %v", m["succeeded"])
	}

	if m["failed"].(float64) != 0 {
		t.Errorf("expected 0 failed, got %v", m["failed"])
	}

	if updated != 2 {
		t.Errorf("expected 2 updates, got %d", updated)
	}
}

func TestContentBulkPublishHandler_InvalidUUID(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{
		ContentLifecycle: &stubContentLifecycleService{
			updateStatusFunc: func(ctx context.Context, id pgtype.UUID, status string) (gen.UpdateContentStatusRow, error) {
				t.Fatal("update should not be called for invalid content_id")
				return gen.UpdateContentStatusRow{}, nil
			},
		},
	}

	handler := ContentBulkPublishHandler(deps)

	tr, _ := handler(t.Context(), json.RawMessage(`{"content_ids":["bad"],"status":"published"}`), pgtype.UUID{}, pgtype.UUID{})
	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success (partial failure)", tr.Status)
	}

	m := parseToolResult(t, tr)
	if m["succeeded"].(float64) != 0 || m["failed"].(float64) != 1 {
		t.Errorf("expected 0 succeeded/1 failed, got %v/%v", m["succeeded"], m["failed"])
	}
}
