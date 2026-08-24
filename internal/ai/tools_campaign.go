// Package ai — tools_campaign.go implements the content.campaign.create
// agent tool (Section 104, AI Content Campaigns).
//
// The handler validates the brief and count, then delegates to the Campaign
// service dependency. The executor's RBAC gate checks campaign.create before
// dispatch; the service additionally enforces persona.manage when auto-
// provisioning personas.
package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// campaignCreateArgs is the LLM-produced argument shape for
// content.campaign.create.
type campaignCreateArgs struct {
	Prompt                string   `json:"prompt"`
	ContentType           string   `json:"content_type,omitempty"`
	Count                 int      `json:"count"`
	PersonaIDs            []string `json:"persona_ids,omitempty"`
	AutoProvisionPersonas *bool    `json:"auto_provision_personas,omitempty"`
	AutoAssign            *bool    `json:"auto_assign,omitempty"`
	TargetSpaceID         string   `json:"target_space_id,omitempty"`
	TargetTags            []string `json:"target_tags,omitempty"`
	RunImmediately        *bool    `json:"run_immediately,omitempty"`
}

// ContentCampaignCreateHandler returns a ToolHandler that creates and plans
// (and optionally runs) a multi-persona content campaign.
func ContentCampaignCreateHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID, tenantID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.Campaign == nil {
			return errorToolResult("campaign_not_configured", "campaign service is not configured"), nil
		}

		var a campaignCreateArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}
		if a.Prompt == "" {
			return errorToolResult("missing_prompt", "prompt is required"), nil
		}
		if a.Count <= 0 {
			return errorToolResult("invalid_count", "count must be greater than 0"), nil
		}

		autoProvision := true
		if a.AutoProvisionPersonas != nil {
			autoProvision = *a.AutoProvisionPersonas
		}
		autoAssign := true
		if a.AutoAssign != nil {
			autoAssign = *a.AutoAssign
		}
		runImmediately := false
		if a.RunImmediately != nil {
			runImmediately = *a.RunImmediately
		}

		in := CampaignCreateInput{
			Prompt:                a.Prompt,
			ContentType:           a.ContentType,
			Count:                 a.Count,
			PersonaIDs:            a.PersonaIDs,
			AutoProvisionPersonas: &autoProvision,
			AutoAssign:            &autoAssign,
			TargetSpaceID:         a.TargetSpaceID,
			TargetTags:            a.TargetTags,
			RunImmediately:        &runImmediately,
		}

		result, err := deps.Campaign.CreateCampaign(ctx, tenantID, userID, in)
		if err != nil {
			return errorToolResult("campaign_create_failed", err.Error()), nil
		}

		return successToolResult(result), nil
	}
}
