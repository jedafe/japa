package ai

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// Stubs
// ---------------------------------------------------------------------------

type stubWikiService struct {
	pageID       string
	revID        string
	createErr    error
	submitErr    error
	reviewErr    error
	communityOK  bool
	structuredOK bool
	autoApproved bool
	minRep       int
	spaceSlug    string
	spaceID      pgtype.UUID
	spaceErr     error

	gotCreate    *WikiCreatePageInput
	gotSubmit    *wikiSubmitCapture
	gotFieldOps  *wikiSubmitCapture
	gotApproveID pgtype.UUID
	gotRejectID  pgtype.UUID
}

type wikiSubmitCapture struct {
	spaceSlug string
	pageSlug  string
	content   json.RawMessage
	ops       json.RawMessage
	summary   string
	canManage bool
}

func (s *stubWikiService) CreatePageWithRevision(ctx context.Context, userID pgtype.UUID, in WikiCreatePageInput) (string, string, string, error) {
	if s.createErr != nil {
		return "", "", "", s.createErr
	}
	s.gotCreate = &in
	return s.pageID, s.revID, in.Slug, nil
}

func (s *stubWikiService) CreatePageShell(ctx context.Context, spaceID, createdBy pgtype.UUID, slug, title, category string) (string, error) {
	if s.createErr != nil {
		return "", s.createErr
	}
	return s.pageID, nil
}

func (s *stubWikiService) SubmitRevision(ctx context.Context, userID pgtype.UUID, spaceSlug, pageSlug string, content json.RawMessage, summary string) (string, error) {
	if s.submitErr != nil {
		return "", s.submitErr
	}
	s.gotSubmit = &wikiSubmitCapture{spaceSlug: spaceSlug, pageSlug: pageSlug, content: content, summary: summary}
	return s.revID, nil
}

func (s *stubWikiService) SubmitFieldOps(ctx context.Context, userID pgtype.UUID, spaceSlug, pageSlug string, content json.RawMessage, ops json.RawMessage, summary string, canManage bool) (string, bool, error) {
	if s.submitErr != nil {
		return "", false, s.submitErr
	}
	s.gotFieldOps = &wikiSubmitCapture{spaceSlug: spaceSlug, pageSlug: pageSlug, content: content, ops: ops, summary: summary, canManage: canManage}
	return s.revID, s.autoApproved, nil
}

func (s *stubWikiService) StructuredEditsEnabled(ctx context.Context) bool { return s.structuredOK }

func (s *stubWikiService) ApproveRevision(ctx context.Context, revID, reviewerID pgtype.UUID, notes string) error {
	s.gotApproveID = revID
	return s.reviewErr
}

func (s *stubWikiService) RejectRevision(ctx context.Context, revID, reviewerID pgtype.UUID, notes string) error {
	s.gotRejectID = revID
	return s.reviewErr
}

func (s *stubWikiService) CommunityEditsAllowed(ctx context.Context) bool { return s.communityOK }
func (s *stubWikiService) MinReputationToEdit(ctx context.Context) int    { return s.minRep }

func (s *stubWikiService) SpaceSlugByID(ctx context.Context, spaceID pgtype.UUID) (string, error) {
	return s.spaceSlug, s.spaceErr
}

func (s *stubWikiService) SpaceIDBySlug(ctx context.Context, spaceSlug string) (pgtype.UUID, error) {
	return s.spaceID, s.spaceErr
}

type stubPointsReader struct {
	balance int64
	err     error
}

func (s *stubPointsReader) GetPointsBalance(ctx context.Context, userID pgtype.UUID) (int64, error) {
	return s.balance, s.err
}

type stubPermChecker map[string]bool

func (p stubPermChecker) HasPermission(ctx context.Context, userID pgtype.UUID, key string) bool {
	return p[key]
}

func resultErrorCode(t *testing.T, res *ToolResult) string {
	t.Helper()
	if res == nil {
		t.Fatal("nil result")
	}
	var body struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	if err := json.Unmarshal([]byte(res.Content), &body); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if body.Code != "" {
		return body.Code
	}
	return body.Error
}

// ---------------------------------------------------------------------------
// wiki_page.create
// ---------------------------------------------------------------------------

func TestWikiPageCreateHandler_ShellWithoutContent(t *testing.T) {
	wiki := &stubWikiService{pageID: "11111111-1111-1111-1111-111111111111"}
	deps := &ToolDeps{Wiki: wiki}
	userID := mustUUID(t, "22222222-2222-2222-2222-222222222222")

	res, err := WikiPageCreateHandler(deps)(context.Background(), json.RawMessage(`{
		"payload": {"space_id": "33333333-3333-3333-3333-333333333333", "slug": "visa-routes", "title": "Visa Routes"}
	}`), userID)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(res.Content), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out["status"] != "created" || out["page_id"] != wiki.pageID {
		t.Fatalf("unexpected result: %v", out)
	}
	if wiki.gotCreate != nil {
		t.Fatal("content-less payload must not reach the revision path")
	}
}

func TestWikiPageCreateHandler_WithContentCreatesPendingRevision(t *testing.T) {
	wiki := &stubWikiService{
		pageID:    "11111111-1111-1111-1111-111111111111",
		revID:     "44444444-4444-4444-4444-444444444444",
		spaceSlug: "canada",
	}
	deps := &ToolDeps{Wiki: wiki}
	userID := mustUUID(t, "22222222-2222-2222-2222-222222222222")

	res, err := WikiPageCreateHandler(deps)(context.Background(), json.RawMessage(`{
		"payload": {
			"space_id": "33333333-3333-3333-3333-333333333333",
			"slug": "visa-routes",
			"title": "Visa Routes",
			"content": "<p>Express Entry overview</p>",
			"summary": "initial draft"
		}
	}`), userID)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(res.Content), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out["status"] != "pending" || out["revision_id"] != wiki.revID {
		t.Fatalf("unexpected result: %v", out)
	}
	if wiki.gotCreate == nil {
		t.Fatal("revision path not reached")
	}
	if wiki.gotCreate.SpaceSlug != "canada" {
		t.Fatalf("space_id should resolve to slug, got %q", wiki.gotCreate.SpaceSlug)
	}
	var decoded string
	if err := json.Unmarshal(wiki.gotCreate.Content, &decoded); err != nil || decoded != "<p>Express Entry overview</p>" {
		t.Fatalf("content should be a JSON string: %s", wiki.gotCreate.Content)
	}
}

func TestWikiPageCreateHandler_ShellBySpaceSlug(t *testing.T) {
	wiki := &stubWikiService{
		pageID:  "11111111-1111-1111-1111-111111111111",
		spaceID: mustUUID(t, "33333333-3333-3333-3333-333333333333"),
	}
	deps := &ToolDeps{Wiki: wiki}
	userID := mustUUID(t, "22222222-2222-2222-2222-222222222222")

	res, err := WikiPageCreateHandler(deps)(context.Background(), json.RawMessage(`{
		"payload": {"space_slug": "canada", "slug": "visa-routes", "title": "Visa Routes"}
	}`), userID)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(res.Content), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out["status"] != "created" {
		t.Fatalf("unexpected result: %v", out)
	}

	// Missing both space keys → invalid_space.
	res, _ = WikiPageCreateHandler(deps)(context.Background(), json.RawMessage(`{
		"payload": {"slug": "x", "title": "y"}
	}`), userID)
	if got := resultErrorCode(t, res); got != "invalid_space" {
		t.Fatalf("want invalid_space, got %q", got)
	}
}

func TestWikiPageCreateHandler_RequiresSlugAndTitle(t *testing.T) {
	deps := &ToolDeps{Wiki: &stubWikiService{}}
	userID := mustUUID(t, "22222222-2222-2222-2222-222222222222")

	res, _ := WikiPageCreateHandler(deps)(context.Background(), json.RawMessage(`{"payload":{"title":"x"}}`), userID)
	if got := resultErrorCode(t, res); got != "missing_slug" {
		t.Fatalf("want missing_slug, got %q", got)
	}
	res, _ = WikiPageCreateHandler(deps)(context.Background(), json.RawMessage(`{"payload":{"slug":"x"}}`), userID)
	if got := resultErrorCode(t, res); got != "missing_title" {
		t.Fatalf("want missing_title, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// wiki_page.submit_revision
// ---------------------------------------------------------------------------

func TestWikiPageSubmitRevisionHandler_SubmitsPending(t *testing.T) {
	wiki := &stubWikiService{revID: "44444444-4444-4444-4444-444444444444", communityOK: true, minRep: 0}
	deps := &ToolDeps{Wiki: wiki}
	userID := mustUUID(t, "22222222-2222-2222-2222-222222222222")

	res, err := WikiPageSubmitRevisionHandler(deps)(context.Background(), json.RawMessage(`{
		"space_slug": "canada", "page_slug": "visa-routes",
		"content": {"type": "doc", "content": []}, "summary": "fix visa section"
	}`), userID)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(res.Content), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out["status"] != "pending" || out["revision_id"] != wiki.revID {
		t.Fatalf("unexpected result: %v", out)
	}
	if wiki.gotSubmit == nil || wiki.gotSubmit.spaceSlug != "canada" || wiki.gotSubmit.summary != "fix visa section" {
		t.Fatalf("submit not forwarded correctly: %+v", wiki.gotSubmit)
	}
}

func TestWikiPageSubmitRevisionHandler_CommunityEditsDisabled(t *testing.T) {
	wiki := &stubWikiService{communityOK: false}
	userID := mustUUID(t, "22222222-2222-2222-2222-222222222222")

	// Non-moderator is blocked.
	deps := &ToolDeps{Wiki: wiki, Permission: stubPermChecker{}}
	res, _ := WikiPageSubmitRevisionHandler(deps)(context.Background(), json.RawMessage(`{
		"space_slug": "canada", "page_slug": "visa-routes", "content": "x"
	}`), userID)
	if got := resultErrorCode(t, res); got != "community_edits_disabled" {
		t.Fatalf("want community_edits_disabled, got %q", got)
	}

	// wiki.review holder may still submit (§80 mod-only mode).
	deps.Permission = stubPermChecker{"wiki.review": true}
	res, err := WikiPageSubmitRevisionHandler(deps)(context.Background(), json.RawMessage(`{
		"space_slug": "canada", "page_slug": "visa-routes", "content": "x"
	}`), userID)
	if err != nil || resultErrorCode(t, res) != "" {
		t.Fatalf("mod should pass community_edits gate: %v %v", err, res.Content)
	}
}

func TestWikiPageSubmitRevisionHandler_ReputationGate(t *testing.T) {
	userID := mustUUID(t, "22222222-2222-2222-2222-222222222222")
	args := json.RawMessage(`{"space_slug": "canada", "page_slug": "x", "content": "c"}`)

	// Below threshold → blocked.
	wiki := &stubWikiService{communityOK: true, minRep: 100}
	deps := &ToolDeps{Wiki: wiki, Points: &stubPointsReader{balance: 10}}
	res, _ := WikiPageSubmitRevisionHandler(deps)(context.Background(), args, userID)
	if got := resultErrorCode(t, res); got != "insufficient_reputation" {
		t.Fatalf("want insufficient_reputation, got %q", got)
	}

	// Nil points reader with a positive threshold → fail closed.
	deps.Points = nil
	res, _ = WikiPageSubmitRevisionHandler(deps)(context.Background(), args, userID)
	if got := resultErrorCode(t, res); got != "reputation_check_unavailable" {
		t.Fatalf("want reputation_check_unavailable, got %q", got)
	}

	// At/above threshold → submits.
	deps.Points = &stubPointsReader{balance: 150}
	res, err := WikiPageSubmitRevisionHandler(deps)(context.Background(), args, userID)
	if err != nil || resultErrorCode(t, res) != "" {
		t.Fatalf("expected success at threshold: %v %v", err, res.Content)
	}
}

func TestWikiPageSubmitRevisionHandler_FieldOps(t *testing.T) {
	userID := mustUUID(t, "22222222-2222-2222-2222-222222222222")
	ops := `[{"op":"set_value","entity_type":"visa_route","entity_id":"33333333-3333-3333-3333-333333333333","field_key":"processing_time","value":12}]`

	// Structured edits disabled → feature_disabled.
	wiki := &stubWikiService{communityOK: true, structuredOK: false}
	deps := &ToolDeps{Wiki: wiki}
	res, _ := WikiPageSubmitRevisionHandler(deps)(context.Background(), json.RawMessage(`{
		"space_slug": "canada", "page_slug": "visa-routes", "ops": `+ops+`
	}`), userID)
	if got := resultErrorCode(t, res); got != "feature_disabled" {
		t.Fatalf("want feature_disabled, got %q", got)
	}

	// Enabled → SubmitFieldOps; wiki.fields.manage marks auto-approve.
	wiki = &stubWikiService{communityOK: true, structuredOK: true, revID: "44444444-4444-4444-4444-444444444444", autoApproved: true}
	deps = &ToolDeps{Wiki: wiki, Permission: stubPermChecker{"wiki.fields.manage": true}}
	res, err := WikiPageSubmitRevisionHandler(deps)(context.Background(), json.RawMessage(`{
		"space_slug": "canada", "page_slug": "visa-routes",
		"content": "updated prose", "ops": `+ops+`, "summary": "fix times"
	}`), userID)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(res.Content), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out["status"] != "approved" || out["auto_approved"] != true {
		t.Fatalf("expected auto-approved mixed revision: %v", out)
	}
	if wiki.gotFieldOps == nil || !wiki.gotFieldOps.canManage || wiki.gotFieldOps.summary != "fix times" {
		t.Fatalf("field ops not forwarded correctly: %+v", wiki.gotFieldOps)
	}
}

func TestWikiPageCreateHandler_FallbackWithoutWikiService(t *testing.T) {
	userID := mustUUID(t, "22222222-2222-2222-2222-222222222222")

	// Content-bearing payload with no wiki service → refuse rather than
	// silently drop the body.
	deps := &ToolDeps{}
	res, _ := WikiPageCreateHandler(deps)(context.Background(), json.RawMessage(`{
		"payload": {"space_id": "33333333-3333-3333-3333-333333333333", "slug": "x", "title": "y", "content": "body"}
	}`), userID)
	if got := resultErrorCode(t, res); got != "wiki_unavailable" {
		t.Fatalf("want wiki_unavailable, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// wiki_page.review_revision
// ---------------------------------------------------------------------------

func TestWikiPageReviewRevisionHandler_ApproveReject(t *testing.T) {
	wiki := &stubWikiService{}
	deps := &ToolDeps{Wiki: wiki}
	userID := mustUUID(t, "22222222-2222-2222-2222-222222222222")
	rev := "44444444-4444-4444-4444-444444444444"

	res, err := WikiPageReviewRevisionHandler(deps)(context.Background(), json.RawMessage(`{"revision_id":"`+rev+`","action":"approve"}`), userID)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if wiki.gotApproveID.String() != rev {
		t.Fatalf("approve not dispatched for %s", rev)
	}

	res, err = WikiPageReviewRevisionHandler(deps)(context.Background(), json.RawMessage(`{"revision_id":"`+rev+`","action":"reject","notes":"needs sources"}`), userID)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if wiki.gotRejectID.String() != rev {
		t.Fatalf("reject not dispatched for %s", rev)
	}

	// Invalid action.
	res, _ = WikiPageReviewRevisionHandler(deps)(context.Background(), json.RawMessage(`{"revision_id":"`+rev+`","action":"merge"}`), userID)
	if got := resultErrorCode(t, res); got != "invalid_action" {
		t.Fatalf("want invalid_action, got %q", got)
	}

	// Service error propagates as a tool error result.
	wiki.reviewErr = errors.New("revision not pending")
	res, _ = WikiPageReviewRevisionHandler(deps)(context.Background(), json.RawMessage(`{"revision_id":"`+rev+`","action":"approve"}`), userID)
	if got := resultErrorCode(t, res); got != "wiki_approve_failed" {
		t.Fatalf("want wiki_approve_failed, got %q", got)
	}
}
