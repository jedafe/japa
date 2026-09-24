// Package ai — tools_moat_test.go covers the agent data tools backed by
// the public API moat facade (visa.*, university.get, destination.brief).
// The handlers exercise the ai-local AgentDataReader interface; the wiring
// layer adapts *publicapi.Service. Tests use a stub reader — no database.
//
// Ref: docs/superpowers/specs/2026-09-23-agent-data-api-design.md
package ai

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// stubAgentData implements AgentDataReader; populate only what a test needs.
type stubAgentData struct {
	listVisaFn    func(ctx context.Context, dest pgtype.UUID, prof, edu string, yrs, page, limit int32) (any, int64, error)
	exploreFeedFn func(ctx context.Context, name, sort string, limit, offset int32) (any, error)
	tagFeedFn     func(ctx context.Context, tagSlug, cursor string, limit int32) (any, error)
	spaceFeedFn   func(ctx context.Context, viewerID pgtype.UUID, slug, sort string, limit, offset int32) (any, error)
	myFeedFn      func(ctx context.Context, userID pgtype.UUID, sort string, limit, offset int32) (any, error)
	listScamFn    func(ctx context.Context, dest pgtype.UUID, scamType string, limit, offset int32) (any, int64, error)
	getScamFn     func(ctx context.Context, id pgtype.UUID) (any, error)
	semanticFn    func(ctx context.Context, query, lang, spaceSlug string, limit int32) (any, error)
}

func (s stubAgentData) ListVisaRoutes(ctx context.Context, dest pgtype.UUID, prof, edu string, yrs, page, limit int32) (any, int64, error) {
	return s.listVisaFn(ctx, dest, prof, edu, yrs, page, limit)
}
func (s stubAgentData) GetVisaRoute(_ context.Context, _ pgtype.UUID) (any, error) {
	return nil, nil
}
func (s stubAgentData) CompareVisaRoutes(_ context.Context, _ []pgtype.UUID) (any, error) {
	return nil, nil
}
func (s stubAgentData) GetUniversity(_ context.Context, _ string) (any, error) {
	return nil, nil
}
func (s stubAgentData) GetDestinationBrief(_ context.Context, _ string, _ []string) (any, error) {
	return nil, nil
}
func (s stubAgentData) ExploreFeed(ctx context.Context, name, sort string, limit, offset int32) (any, error) {
	if s.exploreFeedFn == nil {
		return nil, nil
	}
	return s.exploreFeedFn(ctx, name, sort, limit, offset)
}
func (s stubAgentData) TagFeed(ctx context.Context, tagSlug, cursor string, limit int32) (any, error) {
	if s.tagFeedFn == nil {
		return nil, nil
	}
	return s.tagFeedFn(ctx, tagSlug, cursor, limit)
}
func (s stubAgentData) SpaceFeed(ctx context.Context, viewerID pgtype.UUID, slug, sort string, limit, offset int32) (any, error) {
	if s.spaceFeedFn == nil {
		return nil, nil
	}
	return s.spaceFeedFn(ctx, viewerID, slug, sort, limit, offset)
}
func (s stubAgentData) MyFeed(ctx context.Context, userID pgtype.UUID, sort string, limit, offset int32) (any, error) {
	if s.myFeedFn == nil {
		return nil, nil
	}
	return s.myFeedFn(ctx, userID, sort, limit, offset)
}
func (s stubAgentData) ListScamAlerts(ctx context.Context, dest pgtype.UUID, scamType string, limit, offset int32) (any, int64, error) {
	if s.listScamFn == nil {
		return nil, 0, nil
	}
	return s.listScamFn(ctx, dest, scamType, limit, offset)
}
func (s stubAgentData) GetScamAlert(ctx context.Context, id pgtype.UUID) (any, error) {
	if s.getScamFn == nil {
		return nil, nil
	}
	return s.getScamFn(ctx, id)
}
func (s stubAgentData) SemanticSearch(ctx context.Context, query, lang, spaceSlug string, limit int32) (any, error) {
	if s.semanticFn == nil {
		return nil, nil
	}
	return s.semanticFn(ctx, query, lang, spaceSlug, limit)
}

func TestVisaSearchTool(t *testing.T) {
	h := VisaSearchHandler(&ToolDeps{AgentData: stubAgentData{
		listVisaFn: func(_ context.Context, _ pgtype.UUID, _, _ string, _, _, _ int32) (any, int64, error) {
			return []map[string]any{{"id": "r1"}}, 1, nil
		},
	}})
	res, err := h(context.Background(), json.RawMessage(`{"destination_space_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{})
	if err != nil || res == nil || res.Status != ToolCallStatusSuccess {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestVisaSearchToolUnconfigured(t *testing.T) {
	h := VisaSearchHandler(&ToolDeps{})
	res, err := h(context.Background(), json.RawMessage(`{}`), pgtype.UUID{})
	if err != nil || res == nil || res.Status == ToolCallStatusSuccess {
		t.Fatalf("expected error result for unconfigured reader; res=%v err=%v", res, err)
	}
}

// ---------------------------------------------------------------------------
// Phase 2 — feeds, scam alerts, semantic search
// ---------------------------------------------------------------------------

func TestFeedsExploreRoutesToTag(t *testing.T) {
	var gotTag, gotCursor string
	var gotLimit int32
	var exploreCalled, spaceCalled bool
	h := FeedsExploreHandler(&ToolDeps{AgentData: stubAgentData{
		tagFeedFn: func(_ context.Context, tagSlug, cursor string, limit int32) (any, error) {
			gotTag, gotCursor, gotLimit = tagSlug, cursor, limit
			return map[string]any{"items": []string{"c1"}, "next_cursor": "nc"}, nil
		},
		exploreFeedFn: func(_ context.Context, _, _ string, _, _ int32) (any, error) {
			exploreCalled = true
			return nil, nil
		},
		spaceFeedFn: func(_ context.Context, _ pgtype.UUID, _, _ string, _, _ int32) (any, error) {
			spaceCalled = true
			return nil, nil
		},
	}})
	res, err := h(context.Background(), json.RawMessage(`{"tag":"germany","cursor":"abc"}`), pgtype.UUID{})
	if err != nil || res == nil || res.Status != ToolCallStatusSuccess {
		t.Fatalf("res=%v err=%v", res, err)
	}
	if gotTag != "germany" || gotCursor != "abc" || gotLimit != 20 {
		t.Fatalf("tag routing wrong: tag=%q cursor=%q limit=%d", gotTag, gotCursor, gotLimit)
	}
	if exploreCalled || spaceCalled {
		t.Fatal("tag arg must route only to TagFeed")
	}
}

func TestFeedsExploreRoutesToSpace(t *testing.T) {
	viewerID := mustUUID(t, "00000000-0000-0000-0000-0000000000aa")
	var gotViewer pgtype.UUID
	var gotSlug string
	h := FeedsExploreHandler(&ToolDeps{AgentData: stubAgentData{
		spaceFeedFn: func(_ context.Context, v pgtype.UUID, slug, _ string, _, _ int32) (any, error) {
			gotViewer, gotSlug = v, slug
			return []string{"c1"}, nil
		},
	}})
	res, err := h(context.Background(), json.RawMessage(`{"space_slug":"canada","sort":"latest"}`), viewerID)
	if err != nil || res == nil || res.Status != ToolCallStatusSuccess {
		t.Fatalf("res=%v err=%v", res, err)
	}
	if gotViewer != viewerID || gotSlug != "canada" {
		t.Fatalf("space routing wrong: viewer=%v slug=%q", gotViewer, gotSlug)
	}
}

func TestFeedsExploreDefaultName(t *testing.T) {
	var gotName string
	h := FeedsExploreHandler(&ToolDeps{AgentData: stubAgentData{
		exploreFeedFn: func(_ context.Context, name, _ string, limit, offset int32) (any, error) {
			gotName = name
			return []string{"c1"}, nil
		},
	}})
	res, err := h(context.Background(), json.RawMessage(`{}`), pgtype.UUID{})
	if err != nil || res == nil || res.Status != ToolCallStatusSuccess {
		t.Fatalf("res=%v err=%v", res, err)
	}
	if gotName != "popular" {
		t.Fatalf("expected default name popular, got %q", gotName)
	}
}

func TestFeedsExploreInvalidArgs(t *testing.T) {
	h := FeedsExploreHandler(&ToolDeps{AgentData: stubAgentData{}})
	res, err := h(context.Background(), json.RawMessage(`{"limit":`), pgtype.UUID{})
	if err != nil || res == nil || res.Status == ToolCallStatusSuccess {
		t.Fatalf("expected error result for malformed args; res=%v err=%v", res, err)
	}
}

func TestFeedsForYouUsesCallerIdentity(t *testing.T) {
	callerID := mustUUID(t, "00000000-0000-0000-0000-0000000000bb")
	var gotUser pgtype.UUID
	h := FeedsForYouHandler(&ToolDeps{AgentData: stubAgentData{
		myFeedFn: func(_ context.Context, userID pgtype.UUID, _ string, _, _ int32) (any, error) {
			gotUser = userID
			return []string{"c1"}, nil
		},
	}})
	res, err := h(context.Background(), json.RawMessage(`{"sort":"latest","limit":5}`), callerID)
	if err != nil || res == nil || res.Status != ToolCallStatusSuccess {
		t.Fatalf("res=%v err=%v", res, err)
	}
	if gotUser != callerID {
		t.Fatalf("MyFeed must receive the caller userID, got %v", gotUser)
	}
}

func TestFeedsForYouRequiresCaller(t *testing.T) {
	h := FeedsForYouHandler(&ToolDeps{AgentData: stubAgentData{}})
	res, err := h(context.Background(), json.RawMessage(`{}`), pgtype.UUID{})
	if err != nil || res == nil || res.Status == ToolCallStatusSuccess {
		t.Fatalf("expected error result for missing caller identity; res=%v err=%v", res, err)
	}
}

func TestScamListTool(t *testing.T) {
	var gotDest pgtype.UUID
	var gotType string
	h := ScamListHandler(&ToolDeps{AgentData: stubAgentData{
		listScamFn: func(_ context.Context, dest pgtype.UUID, scamType string, _, _ int32) (any, int64, error) {
			gotDest, gotType = dest, scamType
			return []map[string]any{{"id": "a1"}}, 1, nil
		},
	}})
	res, err := h(context.Background(), json.RawMessage(`{"destination_space_id":"00000000-0000-0000-0000-000000000001","scam_type":"visa_fee"}`), pgtype.UUID{})
	if err != nil || res == nil || res.Status != ToolCallStatusSuccess {
		t.Fatalf("res=%v err=%v", res, err)
	}
	if !gotDest.Valid || gotType != "visa_fee" {
		t.Fatalf("scam.list args not routed: dest=%v type=%q", gotDest, gotType)
	}
}

func TestScamListToolBadDest(t *testing.T) {
	h := ScamListHandler(&ToolDeps{AgentData: stubAgentData{}})
	res, err := h(context.Background(), json.RawMessage(`{"destination_space_id":"not-a-uuid"}`), pgtype.UUID{})
	if err != nil || res == nil || res.Status == ToolCallStatusSuccess {
		t.Fatalf("expected error result for bad destination_space_id; res=%v err=%v", res, err)
	}
}

func TestScamGetTool(t *testing.T) {
	var gotID pgtype.UUID
	h := ScamGetHandler(&ToolDeps{AgentData: stubAgentData{
		getScamFn: func(_ context.Context, id pgtype.UUID) (any, error) {
			gotID = id
			return map[string]any{"id": id.String()}, nil
		},
	}})
	res, err := h(context.Background(), json.RawMessage(`{"id":"00000000-0000-0000-0000-000000000002"}`), pgtype.UUID{})
	if err != nil || res == nil || res.Status != ToolCallStatusSuccess {
		t.Fatalf("res=%v err=%v", res, err)
	}
	if !gotID.Valid {
		t.Fatal("GetScamAlert was not called with a valid ID")
	}
}

func TestScamGetToolMissingID(t *testing.T) {
	h := ScamGetHandler(&ToolDeps{AgentData: stubAgentData{}})
	res, err := h(context.Background(), json.RawMessage(`{}`), pgtype.UUID{})
	if err != nil || res == nil || res.Status == ToolCallStatusSuccess {
		t.Fatalf("expected error result for missing id; res=%v err=%v", res, err)
	}
}

func TestSemanticSearchTool(t *testing.T) {
	var gotQuery, gotLang, gotSpace string
	h := SemanticSearchHandler(&ToolDeps{AgentData: stubAgentData{
		semanticFn: func(_ context.Context, query, lang, spaceSlug string, _ int32) (any, error) {
			gotQuery, gotLang, gotSpace = query, lang, spaceSlug
			return map[string]any{"data": []string{}}, nil
		},
	}})
	res, err := h(context.Background(), json.RawMessage(`{"query":"nursing jobs","lang":"en","space":"canada"}`), pgtype.UUID{})
	if err != nil || res == nil || res.Status != ToolCallStatusSuccess {
		t.Fatalf("res=%v err=%v", res, err)
	}
	if gotQuery != "nursing jobs" || gotLang != "en" || gotSpace != "canada" {
		t.Fatalf("semantic args not routed: q=%q lang=%q space=%q", gotQuery, gotLang, gotSpace)
	}
}

func TestSemanticSearchToolMissingQuery(t *testing.T) {
	h := SemanticSearchHandler(&ToolDeps{AgentData: stubAgentData{}})
	res, err := h(context.Background(), json.RawMessage(`{"query":"  "}`), pgtype.UUID{})
	if err != nil || res == nil || res.Status == ToolCallStatusSuccess {
		t.Fatalf("expected error result for blank query; res=%v err=%v", res, err)
	}
}
