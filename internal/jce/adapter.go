package jce

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/ai"
)

// ToolAdapter converts typed internal/jce.Service results into the
// map[string]any shape the AI agent tool handlers expect.
//
// Ref: docs/superpowers/plans/2026-08-30-jce-subspec-4.md (Task 8)
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Comparison Engine).
type ToolAdapter struct {
	svc *Service
}

// NewToolAdapter creates an ai.JCEService backed by a *jce.Service.
func NewToolAdapter(svc *Service) ai.JCEService {
	return &ToolAdapter{svc: svc}
}

// Refresh implements ai.JCEService by delegating to Service.RefreshComparison.
func (a *ToolAdapter) Refresh(ctx context.Context, contentID, userID pgtype.UUID, sourceKind *string) (map[string]any, error) {
	res, err := a.svc.RefreshComparison(ctx, contentID, userID, sourceKind)
	if err != nil {
		return nil, err
	}

	return toMap(res)
}

// toMap converts a typed struct to a map[string]any using JSON as an
// intermediate representation. The JCE domain models carry JSON tags, so
// the resulting map matches the JSON API response shape.
func toMap(v any) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}

	return m, nil
}
