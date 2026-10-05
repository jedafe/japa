// Package server — agent_studio_hitl_test.go tests the agent.skip_hitl
// bypass in the Studio generation pipeline (§75 HITL bypass): tool-table
// drafts publish directly instead of routing to ai_hitl_reviews.
//
// Ref: japaconnect-engineering/sections/75-ai-content-studio.md (Review Routing)
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (agent.skip_hitl)
package server

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/arrivalchecklist"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/studio"
)

// stubStudioService implements studioProcessorService for tests.
type stubStudioService struct {
	job             studio.Job
	generatedCalls  int
	publishedCalls  int
	publishActorID  pgtype.UUID
	publishTable    string
	publishTargetID pgtype.UUID
	publishErr      error
	routedCalls     int
}

func (s *stubStudioService) GetJob(ctx context.Context, id pgtype.UUID) (studio.Job, error) {
	return s.job, nil
}

func (s *stubStudioService) DefaultPersonaID(ctx context.Context) (pgtype.UUID, error) {
	return pgtype.UUID{}, nil
}

func (s *stubStudioService) MarkJobGenerating(ctx context.Context, jobID pgtype.UUID) error {
	return nil
}

func (s *stubStudioService) MarkJobGenerated(ctx context.Context, jobID, draftContentID pgtype.UUID, aiModel string) error {
	s.generatedCalls++
	return nil
}

func (s *stubStudioService) MarkJobRoutedToReview(ctx context.Context, jobID, reviewID pgtype.UUID) error {
	s.routedCalls++
	return nil
}

func (s *stubStudioService) MarkJobFailed(ctx context.Context, jobID pgtype.UUID, errMsg string) error {
	return nil
}

func (s *stubStudioService) MarkJobPublished(ctx context.Context, jobID, draftContentID pgtype.UUID, aiModel string) error {
	s.publishedCalls++
	return nil
}

func (s *stubStudioService) PublishToolTableDraft(ctx context.Context, actorID pgtype.UUID, targetTable string, targetID pgtype.UUID) error {
	if s.publishErr != nil {
		return s.publishErr
	}

	s.publishActorID = actorID
	s.publishTable = targetTable
	s.publishTargetID = targetID

	return nil
}

func (s *stubStudioService) ShouldAutoPublish(ctx context.Context, contentType, guideCategory string) bool {
	return false
}

func (s *stubStudioService) RunBatch(ctx context.Context, batchID pgtype.UUID) error {
	return nil
}

func (s *stubStudioService) RetryFailed(ctx context.Context, jobID pgtype.UUID) error {
	return nil
}

func (s *stubStudioService) RetryBatchFailed(ctx context.Context, batchID pgtype.UUID) (int, error) {
	return 0, nil
}

// stubPersonaResolverSrv implements personaUserIDResolver for tests.
type stubPersonaResolverSrv struct {
	persona gen.GetAIPersonaByIDRow
}

func (s *stubPersonaResolverSrv) GetAIPersonaByID(ctx context.Context, id pgtype.UUID) (gen.GetAIPersonaByIDRow, error) {
	return s.persona, nil
}

func (s *stubPersonaResolverSrv) GetPersonaVoiceByID(ctx context.Context, id pgtype.UUID) (gen.PersonaVoice, error) {
	return gen.PersonaVoice{}, nil
}

// stubArrivalGenerator implements arrivalDraftGenerator for tests.
type stubArrivalGenerator struct {
	guideID string
}

func (s *stubArrivalGenerator) GenerateGuide(ctx context.Context, req arrivalchecklist.GenerateGuideRequest) (arrivalchecklist.GenerateGuideResponse, error) {
	return arrivalchecklist.GenerateGuideResponse{GuideID: s.guideID}, nil
}

// stubBypassChecker implements hitlBypassChecker for tests.
type stubBypassChecker struct {
	granted map[string]bool
}

func (s *stubBypassChecker) HasPermission(ctx context.Context, userID pgtype.UUID, permKey string) bool {
	return s.granted[permKey]
}

func mustUUIDSrv(t *testing.T, s string) pgtype.UUID {
	t.Helper()

	var id pgtype.UUID
	if err := id.Scan(s); err != nil {
		t.Fatalf("bad test uuid %q: %v", s, err)
	}

	return id
}

func queuedArrivalJob(t *testing.T, createdBy pgtype.UUID) studio.Job {
	t.Helper()

	return studio.Job{
		ID:          mustUUIDSrv(t, "00000000-0000-0000-0000-000000000010"),
		PersonaID:   mustUUIDSrv(t, "00000000-0000-0000-0000-000000000020"),
		ContentType: "arrival_checklist",
		TopicPrompt: "Nigeria student arrival checklist",
		Status:      "queued",
		CreatedBy:   createdBy,
	}
}

func TestGenerateJob_ToolTable_SkipHITL_PublishesDirectly(t *testing.T) {
	t.Parallel()

	adminID := mustUUIDSrv(t, "00000000-0000-0000-0000-000000000099")
	draftID := "00000000-0000-0000-0000-0000000000aa"

	studioStub := &stubStudioService{job: queuedArrivalJob(t, adminID)}
	perms := &stubBypassChecker{granted: map[string]bool{"agent.skip_hitl": true}}

	a := &studioDraftProcessorAdapter{
		studio: studioStub,
		perms:  perms,
		personaStore: &stubPersonaResolverSrv{persona: gen.GetAIPersonaByIDRow{
			UserID: mustUUIDSrv(t, "00000000-0000-0000-0000-0000000000bb"),
		}},
		arrivalSvc: &stubArrivalGenerator{guideID: draftID},
	}

	if err := a.GenerateJob(t.Context(), mustUUIDSrv(t, "00000000-0000-0000-0000-000000000010")); err != nil {
		t.Fatalf("GenerateJob: %v", err)
	}

	// The tool-table row publishes directly — no review detour.
	if studioStub.publishTable != "arrival_checklist_guides" {
		t.Errorf("publish target_table = %q, want arrival_checklist_guides", studioStub.publishTable)
	}

	if !studioStub.publishTargetID.Valid || studioStub.publishTargetID.String() != draftID {
		t.Errorf("publish target_id = %v, want %s", studioStub.publishTargetID, draftID)
	}

	if studioStub.publishActorID != adminID {
		t.Errorf("publish actor = %v, want job creator %v", studioStub.publishActorID, adminID)
	}

	if studioStub.publishedCalls != 1 {
		t.Errorf("MarkJobPublished calls = %d, want 1", studioStub.publishedCalls)
	}

	// No review routing for the bypassed job.
	if studioStub.generatedCalls != 0 || studioStub.routedCalls != 0 {
		t.Errorf("expected no review routing, got generated=%d routed=%d", studioStub.generatedCalls, studioStub.routedCalls)
	}
}

func TestGenerateJob_ToolTable_RegularUser_RoutesToHITL(t *testing.T) {
	t.Parallel()

	userID := mustUUIDSrv(t, "00000000-0000-0000-0000-000000000077")

	studioStub := &stubStudioService{job: queuedArrivalJob(t, userID)}
	perms := &stubBypassChecker{granted: map[string]bool{}}

	a := &studioDraftProcessorAdapter{
		studio: studioStub,
		perms:  perms,
		personaStore: &stubPersonaResolverSrv{persona: gen.GetAIPersonaByIDRow{
			UserID: mustUUIDSrv(t, "00000000-0000-0000-0000-0000000000bb"),
		}},
		arrivalSvc: &stubArrivalGenerator{guideID: "00000000-0000-0000-0000-0000000000aa"},
	}

	if err := a.GenerateJob(t.Context(), mustUUIDSrv(t, "00000000-0000-0000-0000-000000000010")); err != nil {
		t.Fatalf("GenerateJob: %v", err)
	}

	// Regular users route to the review queue — generated is marked, no
	// direct publish happens. (aiStore is nil in this harness so the
	// review row itself is skipped after MarkJobGenerated.)
	if studioStub.generatedCalls != 1 {
		t.Errorf("MarkJobGenerated calls = %d, want 1", studioStub.generatedCalls)
	}

	if studioStub.publishedCalls != 0 || studioStub.publishTable != "" {
		t.Error("expected no direct publish for a user without agent.skip_hitl")
	}
}
