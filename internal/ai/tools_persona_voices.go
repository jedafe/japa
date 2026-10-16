// Package ai — tools_persona_voices.go implements the persona voice +
// persona catalog read tools (Section 68, ADR-114).
//
// These read-only tools let the agent — and external MCP clients via the
// §119 bridge — discover the shared voice/tone catalog (persona_voices) and
// the AI personas that can author drafts (content.create_draft's
// ai_persona_id). All four tools are gated by persona.view at the catalog
// layer (migration 000722).
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Tool Catalog Expansion)
// Ref: japaconnect-engineering/adr/ADR-114-shared-persona-voices-and-content-type-model-defaults.md
package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// personaVoicesListArgs is the argument shape for persona_voices.list.
type personaVoicesListArgs struct {
	Limit          int32 `json:"limit,omitempty"`
	Offset         int32 `json:"offset,omitempty"`
	IncludePrompts bool  `json:"include_prompts,omitempty"`
}

// PersonaVoicesListHandler returns a ToolHandler that lists the shared
// persona voice/tone catalog. voice_prompt is omitted by default to keep
// the list compact — pass include_prompts=true or call persona_voices.read
// to fetch the full prompt text.
func PersonaVoicesListHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.PersonaVoices == nil {
			return errorToolResult("persona_voices_unavailable", "persona voice service is not configured"), nil
		}

		var a personaVoicesListArgs
		if len(args) > 0 {
			if err := json.Unmarshal(args, &a); err != nil {
				return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
			}
		}

		limit := a.Limit
		if limit <= 0 || limit > 200 {
			limit = 50
		}

		offset := a.Offset
		if offset < 0 {
			offset = 0
		}

		voices, total, err := deps.PersonaVoices.ListPersonaVoices(ctx, limit, offset)
		if err != nil {
			return errorToolResult("persona_voices_list_failed", err.Error()), nil
		}

		out := make([]PersonaVoiceItem, 0, len(voices))

		for _, v := range voices {
			if !a.IncludePrompts {
				v.VoicePrompt = ""
			}

			out = append(out, v)
		}

		return successToolResult(map[string]any{
			"voices": out,
			"total":  total,
		}), nil
	}
}

// personaVoicesReadArgs is the argument shape for persona_voices.read.
type personaVoicesReadArgs struct {
	ID   string `json:"id,omitempty"`
	Slug string `json:"slug,omitempty"`
}

// PersonaVoicesReadHandler returns a ToolHandler that reads a single shared
// persona voice by id or slug, including the full voice_prompt.
func PersonaVoicesReadHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.PersonaVoices == nil {
			return errorToolResult("persona_voices_unavailable", "persona voice service is not configured"), nil
		}

		var a personaVoicesReadArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		if a.ID == "" && a.Slug == "" {
			return errorToolResult("missing_identifier", "id or slug is required"), nil
		}

		var (
			voice PersonaVoiceItem
			err   error
		)

		if a.ID != "" {
			id, scanErr := scanUUID(a.ID)
			if scanErr != nil {
				return errorToolResult("invalid_voice_id", "id is not a valid UUID"), nil
			}

			voice, err = deps.PersonaVoices.GetPersonaVoice(ctx, id)
		} else {
			voice, err = deps.PersonaVoices.GetPersonaVoiceBySlug(ctx, a.Slug)
		}

		if err != nil {
			return errorToolResult("persona_voice_not_found", fmt.Sprintf("voice not found: %v", err)), nil
		}

		return successToolResult(map[string]any{"voice": voice}), nil
	}
}

// personasListArgs is the argument shape for personas.list.
type personasListArgs struct {
	Status string `json:"status,omitempty"`
	Q      string `json:"q,omitempty"`
	Sort   string `json:"sort,omitempty"`
	Dir    string `json:"dir,omitempty"`
	Limit  int32  `json:"limit,omitempty"`
	Offset int32  `json:"offset,omitempty"`
}

// PersonasListHandler returns a ToolHandler that lists AI personas with
// their status, linked voice, and activity stats. The returned id is the
// ai_persona_id to pass to content.create_draft.
func PersonasListHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.PersonaCatalog == nil {
			return errorToolResult("personas_unavailable", "persona catalog service is not configured"), nil
		}

		var a personasListArgs
		if len(args) > 0 {
			if err := json.Unmarshal(args, &a); err != nil {
				return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
			}
		}

		sort := a.Sort
		switch sort {
		case "", "name", "created_at", "total_actions":
		default:
			sort = "created_at"
		}

		if sort == "" {
			sort = "created_at"
		}

		dir := a.Dir
		switch dir {
		case "asc", "desc":
		default:
			dir = "desc"
		}

		limit := a.Limit
		if limit <= 0 || limit > 100 {
			limit = 50
		}

		offset := a.Offset
		if offset < 0 {
			offset = 0
		}

		items, total, err := deps.PersonaCatalog.ListPersonas(ctx, a.Status, a.Q, sort, dir, limit, offset)
		if err != nil {
			return errorToolResult("personas_list_failed", err.Error()), nil
		}

		if items == nil {
			items = []AgentPersonaItem{}
		}

		return successToolResult(map[string]any{
			"personas": items,
			"total":    total,
		}), nil
	}
}

// personasReadArgs is the argument shape for personas.read.
type personasReadArgs struct {
	ID string `json:"id"`
}

// PersonasReadHandler returns a ToolHandler that reads a single AI persona
// by id, including the resolved voice_prompt and activity_policy.
func PersonasReadHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.PersonaCatalog == nil {
			return errorToolResult("personas_unavailable", "persona catalog service is not configured"), nil
		}

		var a personasReadArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		id, err := scanUUID(a.ID)
		if err != nil {
			return errorToolResult("invalid_persona_id", "id is not a valid UUID"), nil
		}

		persona, err := deps.PersonaCatalog.GetPersona(ctx, id)
		if err != nil {
			return errorToolResult("persona_not_found", fmt.Sprintf("persona not found: %v", err)), nil
		}

		return successToolResult(map[string]any{"persona": persona}), nil
	}
}
