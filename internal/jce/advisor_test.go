//go:build integration

package jce_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/comparison"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	_ "github.com/jedafe/japa/internal/jce/scoring"
	"github.com/jedafe/japa/internal/testdb"
)

// mockDispatcher is a test double for the unexported jce.aiDispatcher
// interface. It records every call and returns a preconfigured response.
type mockDispatcher struct {
	response string
	err      error
	calls    []mockDispatchCall
}

type mockDispatchCall struct {
	category      string
	prompt        string
	systemContext string
	model         string
	maxTokens     int
}

func (m *mockDispatcher) Generate(ctx context.Context, category, prompt, systemContext, model string, maxTokens int) (string, error) {
	m.calls = append(m.calls, mockDispatchCall{category: category, prompt: prompt, systemContext: systemContext, model: model, maxTokens: maxTokens})
	return m.response, m.err
}

func enableAdvisorFeatures(t *testing.T, d *db.DB, tenantID pgtype.UUID) {
	t.Helper()
	ctx := t.Context()
	_, err := d.Writer.Exec(ctx, `
		INSERT INTO feature_flags (key, enabled, config) VALUES ($1, $2, true, '{}')
		ON CONFLICT (key, tenant_id) DO UPDATE SET enabled = true, updated_at = now()
	`, "feature.jce.ai_advisor", tenantID)
	if err != nil {
		t.Fatalf("enableAdvisorFeatures: %v", err)
	}
}

func seedAdvisorPermissions(t *testing.T, d *db.DB, tenantID, userID pgtype.UUID) {
	t.Helper()
	ctx := t.Context()

	groupID := mustUUID(t)
	groupSlug := fmt.Sprintf("jce-advisor-test-%s", uuid.New().String())
	_, err := d.Writer.Exec(ctx, `
		INSERT INTO user_groups (id, name, slug, is_system) VALUES ($1, $2, 'JCE Advisor Test', $3, false)
	`, groupID, tenantID, groupSlug)
	if err != nil {
		t.Fatalf("seedAdvisorPermissions insert group: %v", err)
	}

	roleID := mustUUID(t)
	roleSlug := fmt.Sprintf("jce-advisor-role-%s", uuid.New().String())
	_, err = d.Writer.Exec(ctx, `
		INSERT INTO user_roles (id, name, slug) VALUES ($1, $2, 'JCE Advisor Test', $3)
	`, roleID, tenantID, roleSlug)
	if err != nil {
		t.Fatalf("seedAdvisorPermissions insert role: %v", err)
	}

	_, err = d.Writer.Exec(ctx, `INSERT INTO group_roles (group_id, role_id) VALUES ($1, $2)`, groupID, roleID)
	if err != nil {
		t.Fatalf("seedAdvisorPermissions insert group_roles: %v", err)
	}

	_, err = d.Writer.Exec(ctx, `
		INSERT INTO user_group_memberships (id, user_id, group_id) VALUES ($1, $2, $3, $4)
	`, mustUUID(t), tenantID, userID, groupID)
	if err != nil {
		t.Fatalf("seedAdvisorPermissions insert membership: %v", err)
	}

	var permID pgtype.UUID
	err = d.Reader.QueryRow(ctx, `SELECT id FROM permissions WHERE key = $1`, "jce.ai_advisor.use").Scan(&permID)
	if err != nil {
		t.Fatalf("seedAdvisorPermissions lookup jce.ai_advisor.use: %v", err)
	}

	_, err = d.Writer.Exec(ctx, `INSERT INTO role_permissions (role_id, permission_id) VALUES ($1, $2)`, roleID, permID)
	if err != nil {
		t.Fatalf("seedAdvisorPermissions grant jce.ai_advisor.use: %v", err)
	}
}

func createComparisonWithItems(t *testing.T, ctx context.Context, comp *comparison.Service, tenantID, userID, spaceID, typeID pgtype.UUID, ids []string) pgtype.UUID {
	t.Helper()

	items := make([]comparison.JCEItemInput, len(ids))
	for i, id := range ids {
		ref, _ := json.Marshal(map[string]any{"type": "test", "id": id})
		items[i] = comparison.JCEItemInput{
			Title:    fmt.Sprintf("Item %s", id),
			TypeID:   typeID,
			Position: int32(i + 1),
			ItemRef:  ref,
		}
	}

	res, err := comp.CreateJCEComparison(ctx, tenantID, userID, comparison.CreateJCEComparisonInput{
		Title:      "Advisor Test Comparison",
		Body:       "A test body",
		Visibility: content.VisibilityPublic,
		Status:     content.StatusPublished,
		SpaceID:    spaceID,
		TypeID:     typeID,
		Depth:      comparison.DepthDetailed,
		Items:      items,
	})
	if err != nil {
		t.Fatalf("createComparisonWithItems: %v", err)
	}
	return res.ID
}

func findItemByTitle(t *testing.T, items []gen.ListComparisonItemsByContentRow, title string) gen.ListComparisonItemsByContentRow {
	t.Helper()
	for _, it := range items {
		if it.Title == title {
			return it
		}
	}
	t.Fatalf("findItemByTitle: item %q not found", title)
	return gen.ListComparisonItemsByContentRow{}
}

func TestAdvisorAnalyzeUsesEngineRankingNotLLM(t *testing.T) {
	d := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, d)
	userID := testdb.SeedUser(t, d, tenantID, testdb.UserOpts{})
	spaceID := testdb.SeedSpaceMembership(t, d, tenantID, userID, testdb.SpaceOpts{})
	enableJCEFeatures(t, d, tenantID)
	enableAdvisorFeatures(t, d, tenantID)
	seedJCEPermissions(t, d, tenantID, userID)
	seedAdvisorPermissions(t, d, tenantID, userID)
	seedScoreFactors(t, d, tenantID)
	typeID := seedComparisonTypeForScoring(t, d, tenantID)

	env := newTestService(t, d)
	ctx := newTestContext(t, d, userID, tenantID)

	cmpID := createComparisonWithItems(t, ctx, env.comp, tenantID, userID, spaceID, typeID, []string{"a", "b"})

	// Ensure a score is cached so the advisor read path is exercised.
	if _, err := env.svc.ScoreComparison(ctx, cmpID, tenantID, userID, nil, "", false); err != nil {
		t.Fatalf("ScoreComparison: %v", err)
	}

	cmp, err := env.comp.GetJCEComparison(ctx, cmpID.String(), tenantID)
	if err != nil {
		t.Fatalf("GetJCEComparison: %v", err)
	}
	if len(cmp.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(cmp.Items))
	}

	itemA := findItemByTitle(t, cmp.Items, "Item a")
	itemB := findItemByTitle(t, cmp.Items, "Item b")

	// The LLM deliberately returns a reversed ranking and fake next actions.
	// The advisor must ignore the LLM's ranking and use the engine's.
	llmResponse := fmt.Sprintf(`{
		"summary": "B is best",
		"ranking": [
			{"item_id": "%s", "rank": 1, "japa_score": 99.0, "reasoning": "fake"},
			{"item_id": "%s", "rank": 2, "japa_score": 1.0, "reasoning": "fake"}
		],
		"next_actions": [{"text": "fake action"}]
	}`, itemB.ID.String(), itemA.ID.String())

	mock := &mockDispatcher{response: llmResponse}
	env.svc.SetAIDispatcher(mock)

	res, err := env.svc.Analyze(ctx, cmpID, tenantID, userID, pgtype.UUID{}, "Which is best?")
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if res.Summary != "B is best" {
		t.Errorf("Summary = %q, want %q", res.Summary, "B is best")
	}
	if len(res.Ranking) != 2 {
		t.Fatalf("Ranking length = %d, want 2", len(res.Ranking))
	}
	if res.Ranking[0].ItemTitle != "Item a" {
		t.Errorf("Ranking[0].ItemTitle = %q, want %q", res.Ranking[0].ItemTitle, "Item a")
	}
	if res.Ranking[1].ItemTitle != "Item b" {
		t.Errorf("Ranking[1].ItemTitle = %q, want %q", res.Ranking[1].ItemTitle, "Item b")
	}
	if res.Ranking[0].JapaScore <= 0 {
		t.Errorf("Ranking[0].JapaScore = %f, want > 0", res.Ranking[0].JapaScore)
	}

	// Next actions must come from the engine, not the LLM's fake action.
	if len(res.NextActions) == 0 {
		t.Fatalf("NextActions is empty")
	}
	for _, na := range res.NextActions {
		if na.Text == "fake action" {
			t.Fatalf("NextActions contained an LLM-invented action")
		}
	}

	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 dispatcher call, got %d", len(mock.calls))
	}
	fullPrompt := mock.calls[0].systemContext + "\n\n" + mock.calls[0].prompt
	if !strings.Contains(fullPrompt, "Item a") {
		t.Errorf("prompt does not contain item a")
	}
	if !strings.Contains(fullPrompt, "JapaScore") {
		t.Errorf("prompt does not contain JapaScore")
	}
	if !strings.Contains(fullPrompt, "ranking") {
		t.Errorf("prompt does not contain ranking")
	}
}

func TestAdvisorStreamSSE(t *testing.T) {
	d := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, d)
	userID := testdb.SeedUser(t, d, tenantID, testdb.UserOpts{})
	spaceID := testdb.SeedSpaceMembership(t, d, tenantID, userID, testdb.SpaceOpts{})
	enableJCEFeatures(t, d, tenantID)
	enableAdvisorFeatures(t, d, tenantID)
	seedJCEPermissions(t, d, tenantID, userID)
	seedAdvisorPermissions(t, d, tenantID, userID)
	seedScoreFactors(t, d, tenantID)
	typeID := seedComparisonTypeForScoring(t, d, tenantID)

	env := newTestService(t, d)
	ctx := newTestContext(t, d, userID, tenantID)

	cmpID := createComparisonWithItems(t, ctx, env.comp, tenantID, userID, spaceID, typeID, []string{"a", "b"})
	if _, err := env.svc.ScoreComparison(ctx, cmpID, tenantID, userID, nil, "", false); err != nil {
		t.Fatalf("ScoreComparison: %v", err)
	}

	mock := &mockDispatcher{response: `{"summary":"Streamed answer"}`}
	env.svc.SetAIDispatcher(mock)

	rec := httptest.NewRecorder()
	if err := env.svc.StreamAdvisor(ctx, cmpID, tenantID, userID, pgtype.UUID{}, "Which is best?", rec); err != nil {
		t.Fatalf("StreamAdvisor: %v", err)
	}

	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", got)
	}

	body := rec.Body.String()
	if !strings.HasPrefix(body, "data: ") {
		t.Errorf("SSE body does not start with 'data: ': %q", body)
	}
	if !strings.Contains(body, `"type":"message"`) {
		t.Errorf("SSE body does not contain message event: %q", body)
	}
	if !strings.Contains(body, `"content":"Streamed answer"`) {
		t.Errorf("SSE body does not contain streamed content: %q", body)
	}
}

func TestAdvisorAnalyze_FallbackWhenNoProvider(t *testing.T) {
	d := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, d)
	userID := testdb.SeedUser(t, d, tenantID, testdb.UserOpts{})
	spaceID := testdb.SeedSpaceMembership(t, d, tenantID, userID, testdb.SpaceOpts{})
	enableJCEFeatures(t, d, tenantID)
	enableAdvisorFeatures(t, d, tenantID)
	seedJCEPermissions(t, d, tenantID, userID)
	seedAdvisorPermissions(t, d, tenantID, userID)
	seedScoreFactors(t, d, tenantID)
	typeID := seedComparisonTypeForScoring(t, d, tenantID)

	env := newTestService(t, d)
	ctx := newTestContext(t, d, userID, tenantID)

	cmpID := createComparisonWithItems(t, ctx, env.comp, tenantID, userID, spaceID, typeID, []string{"a", "b"})

	if _, err := env.svc.ScoreComparison(ctx, cmpID, tenantID, userID, nil, "", false); err != nil {
		t.Fatalf("ScoreComparison: %v", err)
	}

	// Simulate the dispatcher having no provider registered for the "chat"
	// category — the same failure that previously surfaced as a 501.
	mock := &mockDispatcher{err: errors.New(`no providers registered for category "chat"`)}
	env.svc.SetAIDispatcher(mock)

	res, err := env.svc.Analyze(ctx, cmpID, tenantID, userID, pgtype.UUID{}, "Which is best?")
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if res.Summary == "" {
		t.Fatal("expected a fallback summary when no AI provider is available")
	}
	if len(res.Ranking) == 0 {
		t.Fatal("expected the engine-backed ranking to be present in the fallback")
	}
	if len(res.NextActions) == 0 {
		t.Fatal("expected engine-backed next actions to be present in the fallback")
	}
}
