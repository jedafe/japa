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
	listVisaFn func(ctx context.Context, dest pgtype.UUID, prof, edu string, yrs, page, limit int32) (any, int64, error)
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
