// Package ai — tools_personas.go implements the personas.create agent tool
// (Section 104, AI Content Campaigns).
//
// The handler validates the incoming persona definitions and delegates to the
// PersonaProvisioner dependency. The executor's RBAC gate already checks
// persona.manage before dispatch.
package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// personasCreateArgs is the LLM-produced argument shape for personas.create.
type personasCreateArgs struct {
	Personas []PersonaDefinition `json:"personas"`
}

// PersonasCreateHandler returns a ToolHandler that provisions one or more
// AI personas transactionally.
func PersonasCreateHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.PersonaProvisioner == nil {
			return errorToolResult("personas_not_configured", "persona provisioning is not configured"), nil
		}

		var a personasCreateArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		if len(a.Personas) == 0 {
			return errorToolResult("missing_personas", "personas array is required"), nil
		}

		for i, p := range a.Personas {
			if p.Name == "" || p.Slug == "" || p.VoicePrompt == "" {
				return errorToolResult("invalid_persona", fmt.Sprintf("persona %d is missing name, slug, or voice_prompt", i)), nil
			}
		}

		results, err := deps.PersonaProvisioner.ProvisionPersonas(ctx, tenantID, userID, a.Personas)
		if err != nil {
			return errorToolResult("personas_provision_failed", err.Error()), nil
		}

		var provisioned int

		for _, r := range results {
			if r.Error == "" && r.ID != "" {
				provisioned++
			}
		}

		return successToolResult(map[string]any{
			"provisioned_count": provisioned,
			"personas":          results,
		}), nil
	}
}
