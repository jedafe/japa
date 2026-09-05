// Package worker — agent_draft_handler_test.go tests the
// agent:create_draft_request task handler validation (Section 103,
// Phase 8, task 8.03).
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Content Request Flow, Asynq path)
// Ref: japaconnect-engineering/sections/11-job-queue-asynq.md (agent:create_draft_request)
package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hibiken/asynq"
)

// stubAgentDraftProcessor captures the payload passed to CreateAgentDraft.
type stubAgentDraftProcessor struct {
	lastPayload AgentDraftPayload
	called      bool
	err         error
}

func (s *stubAgentDraftProcessor) CreateAgentDraft(_ context.Context, payload AgentDraftPayload) error {
	s.called = true
	s.lastPayload = payload

	if s.err != nil {
		return s.err
	}

	return nil
}

func makeDraftTask(t *testing.T, payload AgentDraftPayload) *asynq.Task {
	t.Helper()

	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshaling payload: %v", err)
	}

	return asynq.NewTask(TypeAgentCreateDraftRequest, b)
}

func TestHandleAgentCreateDraftRequest_EmptyTopicPrompt_Skips(t *testing.T) {
	orig := agentDraftProcessor

	t.Cleanup(func() { agentDraftProcessor = orig })

	stub := &stubAgentDraftProcessor{}
	agentDraftProcessor = stub

	err := HandleAgentCreateDraftRequest(t.Context(), makeDraftTask(t, AgentDraftPayload{
		TenantID:    "00000000-0000-0000-0000-000000000001",
		PersonaID:   "00000000-0000-0000-0000-000000000002",
		ContentType: "article",
		// TopicPrompt intentionally empty
	}))
	if err != nil {
		t.Fatalf("expected nil (skip), got %v", err)
	}

	if stub.called {
		t.Error("processor should not be called for empty topic_prompt")
	}
}

func TestHandleAgentCreateDraftRequest_EmptyPersonaID_Skips(t *testing.T) {
	orig := agentDraftProcessor

	t.Cleanup(func() { agentDraftProcessor = orig })

	stub := &stubAgentDraftProcessor{}
	agentDraftProcessor = stub

	err := HandleAgentCreateDraftRequest(t.Context(), makeDraftTask(t, AgentDraftPayload{
		TenantID:    "00000000-0000-0000-0000-000000000001",
		ContentType: "article",
		TopicPrompt: "Tell me about Nigerian visas",
		// PersonaID intentionally empty
	}))
	if err != nil {
		t.Fatalf("expected nil (skip), got %v", err)
	}

	if stub.called {
		t.Error("processor should not be called for empty persona_id")
	}
}

func TestHandleAgentCreateDraftRequest_EmptyTenantID_Skips(t *testing.T) {
	orig := agentDraftProcessor

	t.Cleanup(func() { agentDraftProcessor = orig })

	stub := &stubAgentDraftProcessor{}
	agentDraftProcessor = stub

	err := HandleAgentCreateDraftRequest(t.Context(), makeDraftTask(t, AgentDraftPayload{
		PersonaID:   "00000000-0000-0000-0000-000000000002",
		ContentType: "article",
		TopicPrompt: "Tell me about Nigerian visas",
		// TenantID intentionally empty
	}))
	if err != nil {
		t.Fatalf("expected nil (skip), got %v", err)
	}

	if stub.called {
		t.Error("processor should not be called for empty tenant_id")
	}
}

func TestHandleAgentCreateDraftRequest_EmptyContentType_Skips(t *testing.T) {
	orig := agentDraftProcessor

	t.Cleanup(func() { agentDraftProcessor = orig })

	stub := &stubAgentDraftProcessor{}
	agentDraftProcessor = stub

	err := HandleAgentCreateDraftRequest(t.Context(), makeDraftTask(t, AgentDraftPayload{
		TenantID:    "00000000-0000-0000-0000-000000000001",
		PersonaID:   "00000000-0000-0000-0000-000000000002",
		TopicPrompt: "Tell me about Nigerian visas",
		// ContentType intentionally empty
	}))
	if err != nil {
		t.Fatalf("expected nil (skip), got %v", err)
	}

	if stub.called {
		t.Error("processor should not be called for empty content_type")
	}
}

func TestHandleAgentCreateDraftRequest_NoProcessor_Skips(t *testing.T) {
	orig := agentDraftProcessor

	t.Cleanup(func() { agentDraftProcessor = orig })

	agentDraftProcessor = nil

	err := HandleAgentCreateDraftRequest(t.Context(), makeDraftTask(t, AgentDraftPayload{
		TenantID:    "00000000-0000-0000-0000-000000000001",
		PersonaID:   "00000000-0000-0000-0000-000000000002",
		ContentType: "article",
		TopicPrompt: "Tell me about Nigerian visas",
	}))
	if err != nil {
		t.Fatalf("expected nil (skip), got %v", err)
	}
}

func TestHandleAgentCreateDraftRequest_Success(t *testing.T) {
	orig := agentDraftProcessor

	t.Cleanup(func() { agentDraftProcessor = orig })

	stub := &stubAgentDraftProcessor{}
	agentDraftProcessor = stub

	payload := AgentDraftPayload{
		AgentSessionID:  "00000000-0000-0000-0000-0000000000aa",
		TenantID:        "00000000-0000-0000-0000-000000000001",
		UserID:          "00000000-0000-0000-0000-000000000099",
		PersonaID:       "00000000-0000-0000-0000-000000000002",
		ContentType:     "article",
		TopicPrompt:     "Tell me about Nigerian visas",
		ExternalResults: "Some external results",
	}

	err := HandleAgentCreateDraftRequest(t.Context(), makeDraftTask(t, payload))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !stub.called {
		t.Fatal("processor should be called for valid payload")
	}

	if stub.lastPayload.TopicPrompt != payload.TopicPrompt {
		t.Errorf("topic_prompt = %q, want %q", stub.lastPayload.TopicPrompt, payload.TopicPrompt)
	}

	if stub.lastPayload.PersonaID != payload.PersonaID {
		t.Errorf("persona_id = %q, want %q", stub.lastPayload.PersonaID, payload.PersonaID)
	}
}

func TestHandleAgentCreateDraftRequest_ProcessorError_Propagates(t *testing.T) {
	orig := agentDraftProcessor

	t.Cleanup(func() { agentDraftProcessor = orig })

	stub := &stubAgentDraftProcessor{err: errors.New("db down")}
	agentDraftProcessor = stub

	err := HandleAgentCreateDraftRequest(t.Context(), makeDraftTask(t, AgentDraftPayload{
		TenantID:    "00000000-0000-0000-0000-000000000001",
		PersonaID:   "00000000-0000-0000-0000-000000000002",
		ContentType: "article",
		TopicPrompt: "Tell me about Nigerian visas",
	}))
	if err == nil {
		t.Fatal("expected error from processor failure")
	}
}

func TestHandleAgentCreateDraftRequest_MalformedPayload_SkipsRetry(t *testing.T) {
	orig := agentDraftProcessor

	t.Cleanup(func() { agentDraftProcessor = orig })

	stub := &stubAgentDraftProcessor{}
	agentDraftProcessor = stub

	// Invalid JSON payload
	task := asynq.NewTask(TypeAgentCreateDraftRequest, []byte("{not valid json"))

	err := HandleAgentCreateDraftRequest(t.Context(), task)
	if err == nil {
		t.Fatal("expected error for malformed payload")
	}

	if !errors.Is(err, asynq.SkipRetry) {
		t.Errorf("expected SkipRetry error for malformed payload, got: %v", err)
	}

	if stub.called {
		t.Error("processor should not be called for malformed payload")
	}
}
