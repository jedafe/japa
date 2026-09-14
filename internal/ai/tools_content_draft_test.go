// Package ai — tools_content_draft_test.go tests the Phase 8
// content.create_draft tool handler (Section 103, ADR-111).
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Content Request Flow, HITL Source Tracking)
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 8, tasks 8.01, 8.02, 8.05)
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// stubPersonaResolver implements ai.PersonaResolver for tests.
type stubPersonaResolver struct {
	userID  pgtype.UUID
	err     error
	called  bool
	lastPID pgtype.UUID
}

func (s *stubPersonaResolver) GetPersonaUserID(ctx context.Context, personaID pgtype.UUID) (pgtype.UUID, error) {
	s.called = true
	s.lastPID = personaID

	if s.err != nil {
		return pgtype.UUID{}, s.err
	}

	return s.userID, nil
}

// stubHITLStore implements ai.HITLStore for tests.
type stubHITLStore struct {
	lastReview HITLReview
	err        error
	called     bool
}

func (s *stubHITLStore) CreateHITLReview(ctx context.Context, review HITLReview) (HITLReview, error) {
	s.called = true
	s.lastReview = review

	if s.err != nil {
		return HITLReview{}, s.err
	}
	// Return a review with a fake ID so the handler can surface it.
	var id pgtype.UUID
	_ = id.Scan("00000000-0000-0000-0000-0000000000aa")

	return HITLReview{
		ID: id,

		ContentID:   review.ContentID,
		ContentType: review.ContentType,
		AIOutput:    review.AIOutput,
		Source:      review.Source,
	}, nil
}

// ---------------------------------------------------------------------------
// content.create_draft (Phase 8, tasks 8.01 + 8.02 + 8.05)
// ---------------------------------------------------------------------------

func TestContentCreateDraftHandler_Success_PersonaAuthor(t *testing.T) {
	t.Parallel()

	contentID := mustUUID(t, "00000000-0000-0000-0000-000000000001")
	personaUserID := mustUUID(t, "00000000-0000-0000-0000-0000000000bb")
	personaID := mustUUID(t, "00000000-0000-0000-0000-0000000000cc")
	callingUserID := mustUUID(t, "00000000-0000-0000-0000-000000000099")
	tenantID := mustUUID(t, "00000000-0000-0000-0000-000000000001")

	var capturedInput ContentCreateInput

	deps := &ToolDeps{
		ContentCreator: func(ctx context.Context, tid pgtype.UUID, input ContentCreateInput) (ContentCreateRow, error) {
			capturedInput = input
			return ContentCreateRow{
				ID:          contentID,
				Title:       input.Title,
				Slug:        "test-draft",
				Status:      "draft",
				ContentType: input.ContentType,
				AuthorID:    input.AuthorID,
				CreatedAt:   pgtype.Timestamptz{},
			}, nil
		},
		Persona: &stubPersonaResolver{userID: personaUserID},
		HITL:    &stubHITLStore{},
	}

	args := map[string]any{
		"title":           "Draft Article",
		"body":            "Draft body synthesized from external results.",
		"content_type":    "article",
		"author_id":       "00000000-0000-0000-0000-0000000000ff", // arbitrary — should be overridden by persona resolver
		"ai_persona_id":   personaID.String(),
		"original_prompt": "Tell me about Nigerian visa types",
	}
	argsJSON, _ := json.Marshal(args)

	handler := ContentCreateDraftHandler(deps)

	tr, err := handler(t.Context(), argsJSON, callingUserID, tenantID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success; content=%s", tr.Status, tr.Content)
	}

	// Author must be the persona's backing user_id, not the LLM-sent value.
	if !capturedInput.AuthorID.Valid || capturedInput.AuthorID != personaUserID {
		t.Errorf("author_id = %v, want persona user_id %v", capturedInput.AuthorID, personaUserID)
	}

	// Status must be forced to "draft".
	if capturedInput.Status != "draft" {
		t.Errorf("status = %q, want draft", capturedInput.Status)
	}

	// _ai_provenance must be injected with ai_persona_id + original_prompt.
	var fv map[string]any
	if err := json.Unmarshal(capturedInput.FieldValues, &fv); err != nil {
		t.Fatalf("field values not valid JSON: %v", err)
	}

	prov, ok := fv["_ai_provenance"].(map[string]any)
	if !ok {
		t.Fatal("expected _ai_provenance object in field_values")
	}

	if prov["ai_generated"] != true {
		t.Error("expected ai_generated = true")
	}

	if prov["ai_persona_id"] != personaID.String() {
		t.Errorf("ai_persona_id = %v, want %s", prov["ai_persona_id"], personaID.String())
	}

	if prov["original_prompt"] != "Tell me about Nigerian visa types" {
		t.Errorf("original_prompt = %v, want the user prompt", prov["original_prompt"])
	}

	if prov["prompted_by_user_id"] != callingUserID.String() {
		t.Errorf("prompted_by_user_id = %v, want %s", prov["prompted_by_user_id"], callingUserID.String())
	}

	// HITL review must be created with source='agent_draft_request' + content_id.
	stubHITL := deps.HITL.(*stubHITLStore)
	if !stubHITL.called {
		t.Fatal("expected HITL review to be created")
	}

	if stubHITL.lastReview.Source != HITLSourceAgentDraftRequest {
		t.Errorf("HITL source = %q, want %q", stubHITL.lastReview.Source, HITLSourceAgentDraftRequest)
	}

	if stubHITL.lastReview.Status != HITLStatusPending {
		t.Errorf("HITL status = %q, want %q", stubHITL.lastReview.Status, HITLStatusPending)
	}

	if !stubHITL.lastReview.ContentID.Valid || stubHITL.lastReview.ContentID != contentID {
		t.Errorf("HITL content_id = %v, want %v", stubHITL.lastReview.ContentID, contentID)
	}

	if stubHITL.lastReview.ContentType != "article" {
		t.Errorf("HITL content_type = %q, want article", stubHITL.lastReview.ContentType)
	}

	// The review ID must be surfaced via Metadata so the executor records it
	// on ai_agent_tool_calls and skips its own generic createHITL.
	reviewID, ok := tr.Metadata["hitl_review_id"].(pgtype.UUID)
	if !ok || !reviewID.Valid {
		t.Fatalf("expected hitl_review_id in Metadata, got %v", tr.Metadata)
	}

	// The tool result content must report the draft was submitted for review.
	m := parseToolResult(t, tr)
	if m["review_status"] != "draft submitted for admin review" {
		t.Errorf("review_status = %v, want 'draft submitted for admin review'", m["review_status"])
	}
}

func TestContentCreateDraftHandler_FallsBackToCallingUser_WhenPersonaResolverNil(t *testing.T) {
	t.Parallel()

	callingUserID := mustUUID(t, "00000000-0000-0000-0000-000000000099")

	var capturedInput ContentCreateInput

	deps := &ToolDeps{
		ContentCreator: func(ctx context.Context, tid pgtype.UUID, input ContentCreateInput) (ContentCreateRow, error) {
			capturedInput = input
			return ContentCreateRow{ID: mustUUID(t, "00000000-0000-0000-0000-000000000001"), Status: "draft", AuthorID: input.AuthorID}, nil
		},
		// Persona + HITL are nil — graceful degradation.
	}

	handler := ContentCreateDraftHandler(deps)

	_, err := handler(t.Context(), json.RawMessage(`{"title":"T","content_type":"article","body":"B"}`), callingUserID, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Without a persona resolver, the author falls back to the calling user.
	if !capturedInput.AuthorID.Valid || capturedInput.AuthorID != callingUserID {
		t.Errorf("author_id = %v, want calling user %v (fallback)", capturedInput.AuthorID, callingUserID)
	}
}

func TestContentCreateDraftHandler_PersonaResolverError_FallsBackToAuthorArg(t *testing.T) {
	t.Parallel()

	callingUserID := mustUUID(t, "00000000-0000-0000-0000-000000000099")
	llmAuthorID := mustUUID(t, "00000000-0000-0000-0000-0000000000dd")

	var capturedInput ContentCreateInput

	deps := &ToolDeps{
		ContentCreator: func(ctx context.Context, tid pgtype.UUID, input ContentCreateInput) (ContentCreateRow, error) {
			capturedInput = input
			return ContentCreateRow{ID: mustUUID(t, "00000000-0000-0000-0000-000000000001"), Status: "draft", AuthorID: input.AuthorID}, nil
		},
		Persona: &stubPersonaResolver{err: errors.New("persona not found")},
		HITL:    &stubHITLStore{},
	}

	args := map[string]any{
		"title":         "T",
		"body":          "B",
		"content_type":  "article",
		"author_id":     llmAuthorID.String(),
		"ai_persona_id": "00000000-0000-0000-0000-0000000000cc",
	}
	argsJSON, _ := json.Marshal(args)

	handler := ContentCreateDraftHandler(deps)

	_, err := handler(t.Context(), argsJSON, callingUserID, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// When the persona resolver errors, fall back to the LLM-sent author_id.
	if !capturedInput.AuthorID.Valid || capturedInput.AuthorID != llmAuthorID {
		t.Errorf("author_id = %v, want LLM-sent fallback %v", capturedInput.AuthorID, llmAuthorID)
	}
}

func TestContentCreateDraftHandler_MissingBody(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{
		ContentCreator: func(ctx context.Context, tid pgtype.UUID, input ContentCreateInput) (ContentCreateRow, error) {
			t.Fatal("creator should not be called for invalid input")
			return ContentCreateRow{}, nil
		},
	}
	handler := ContentCreateDraftHandler(deps)

	tr, _ := handler(t.Context(), json.RawMessage(`{"title":"T","content_type":"article"}`), pgtype.UUID{}, pgtype.UUID{})
	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error (missing body)", tr.Status)
	}
}

func TestContentCreateDraftHandler_NoCreatorWired(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{} // no ContentCreator
	handler := ContentCreateDraftHandler(deps)

	tr, _ := handler(t.Context(), json.RawMessage(`{"title":"T","content_type":"article","body":"B"}`), pgtype.UUID{}, pgtype.UUID{})
	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error (no creator wired)", tr.Status)
	}
}

func TestContentCreateDraftHandler_HITLFailure_NonFatal(t *testing.T) {
	t.Parallel()

	contentID := mustUUID(t, "00000000-0000-0000-0000-000000000001")
	deps := &ToolDeps{
		ContentCreator: func(ctx context.Context, tid pgtype.UUID, input ContentCreateInput) (ContentCreateRow, error) {
			return ContentCreateRow{ID: contentID, Status: "draft", AuthorID: input.AuthorID}, nil
		},
		HITL: &stubHITLStore{err: errors.New("db down")},
	}

	handler := ContentCreateDraftHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(`{"title":"T","content_type":"article","body":"B"}`), pgtype.UUID{}, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The draft creation succeeds even when HITL review creation fails.
	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success (HITL failure is non-fatal)", tr.Status)
	}
	// No hitl_review_id in Metadata when HITL creation failed.
	if _, ok := tr.Metadata["hitl_review_id"]; ok {
		t.Error("expected no hitl_review_id in Metadata when HITL creation failed")
	}
}

// ---------------------------------------------------------------------------
// Executor — hitlReviewIDFromMeta + pre-created HITL review honoring
// ---------------------------------------------------------------------------

func TestHitlReviewIDFromMeta(t *testing.T) {
	t.Parallel()

	id := mustUUID(t, "00000000-0000-0000-0000-0000000000aa")

	if _, ok := hitlReviewIDFromMeta(nil); ok {
		t.Error("expected (id, false) for nil result")
	}

	if _, ok := hitlReviewIDFromMeta(&ToolResult{}); ok {
		t.Error("expected (id, false) for nil metadata")
	}

	if _, ok := hitlReviewIDFromMeta(&ToolResult{Metadata: map[string]any{"hitl_review_id": "not-a-uuid"}}); ok {
		t.Error("expected (id, false) for non-UUID metadata value")
	}

	got, ok := hitlReviewIDFromMeta(&ToolResult{Metadata: map[string]any{"hitl_review_id": id}})
	if !ok {
		t.Fatal("expected (id, true) for valid UUID metadata")
	}

	if got != id {
		t.Errorf("got %v, want %v", got, id)
	}
}

// ---------------------------------------------------------------------------
// Agent loop — buildDraftRequestPromptSection + resolveDraftRequestContext
// ---------------------------------------------------------------------------

func TestBuildDraftRequestPromptSection_ContainsPersonaUserID(t *testing.T) {
	t.Parallel()

	d := draftRequestContext{
		Active:        true,
		PersonaUserID: "00000000-0000-0000-0000-0000000000bb",
		PersonaID:     "00000000-0000-0000-0000-0000000000cc",
	}
	s := buildDraftRequestPromptSection(d)

	if !strings.Contains(s, d.PersonaUserID) {
		t.Errorf("expected prompt to contain persona user_id %q", d.PersonaUserID)
	}

	if !strings.Contains(s, d.PersonaID) {
		t.Errorf("expected prompt to contain persona id %q", d.PersonaID)
	}

	if !strings.Contains(s, "content.create_draft") {
		t.Error("expected prompt to mention content.create_draft")
	}

	if !strings.Contains(s, "Do NOT call content.create") {
		t.Error("expected prompt to forbid content.create")
	}
}
