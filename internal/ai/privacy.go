// Package ai — privacy.go implements the AI privacy-exemption layer for the
// Agent Executor (Section 103, ADR-111).
//
// The PrivacyChecker answers two questions for every tool call that targets
// a piece of user data:
//
//  1. IsExempt — is the target entity marked ai_exempt? When true, the AI
//     may neither read nor write that entity. The flag is user-settable on
//     a user's own contents, custom_field_values, and profile (users row);
//     admins may set it on any entity. The check runs at three points per
//     §103: context building (pre-search / RAG exclude exempt content),
//     tool execution (tools targeting exempt entities are blocked), and
//     field reading (custom_field_values.read filters exempt values).
//
//  2. IsBlockedOperation — is the requested operation on the hardcoded
//     blocklist that no user may ever perform through the AI agent, regardless
//     of RBAC? This is a defense-in-depth layer on top of the per-tool
//     is_blocked flag in the ai_tools config JSONB. The blocklist covers
//     RBAC/permission management, AI provider configuration, AI exemption
//     management itself, user deletion, reading other users' private data,
//     and security settings modification.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Privacy Exemption Columns, Tool Executor step 3-4)
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 3, tasks 3.01-3.02)
package ai

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// PrivacyEntityType is the set of entity types whose rows can carry the
// ai_exempt flag. These map 1:1 to the three tables amended by migration
// 000372 (contents, custom_field_values, users).
type PrivacyEntityType string

const (
	// PrivacyEntityContent targets the contents table.
	PrivacyEntityContent PrivacyEntityType = "content"
	// PrivacyEntityCustomFieldValue targets the custom_field_values table.
	PrivacyEntityCustomFieldValue PrivacyEntityType = "custom_field_value"
	// PrivacyEntityUser targets the users table (profile-level exemption).
	PrivacyEntityUser PrivacyEntityType = "user"
)

// ErrPrivacyBlocked is returned by the executor when a tool call targets an
// ai_exempt entity. The agent loop surfaces this to the LLM with the message
// "this data is marked as private and not available to AI" per §103.
var ErrPrivacyBlocked = errors.New("ai: target entity is marked ai_exempt and not available to the agent")

// PrivacyChecker resolves the ai_exempt flag for a given entity within a
// tenant. All lookups are tenant-scoped at the SQL layer for defense-in-depth
// (BUG-014 pattern): contents and users filter directly on tenant_id;
// custom_field_values (no tenant_id column) JOINs custom_fields to enforce
// tenancy transitively. A missing row is treated as non-exempt — the agent
// proceeds — because exemption is an opt-in flag with a NOT NULL DEFAULT false.
type PrivacyChecker struct {
	read *gen.Queries
}

// NewPrivacyChecker creates a PrivacyChecker backed by the read pool. The
// agent executor only needs to read the ai_exempt flag; writes to the flag
// happen through the user-facing /me/ai-privacy routes (Phase 6), never
// through the agent.
func NewPrivacyChecker(read *gen.Queries) *PrivacyChecker {
	return &PrivacyChecker{read: read}
}

// IsExempt reports whether the given entity is marked ai_exempt for the
// tenant. entityType selects which lookup to run. An unknown entity type
// returns false (no exemption table to consult) and a nil error.
//
// A pgx.ErrNoRows result is translated to (false, nil): a non-existent
// entity has no exemption flag set, and the caller (tool handler) will
// surface the not-found condition through its own lookup.
func (c *PrivacyChecker) IsExempt(ctx context.Context, entityType PrivacyEntityType, tenantID, entityID pgtype.UUID) (bool, error) {
	if c == nil || c.read == nil {
		return false, errors.New("ai: privacy checker not configured")
	}

	if tenantID == (pgtype.UUID{}) || entityID == (pgtype.UUID{}) {
		return false, errors.New("ai: tenant_id and entity_id are required for privacy check")
	}

	switch entityType {
	case PrivacyEntityContent:
		exempt, err := c.read.GetContentAIExempt(ctx, gen.GetContentAIExemptParams{
			ID:       entityID,
			TenantID: tenantID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return false, nil
			}
			return false, fmt.Errorf("checking content ai_exempt: %w", err)
		}
		return exempt, nil

	case PrivacyEntityCustomFieldValue:
		exempt, err := c.read.GetCustomFieldValueAIExempt(ctx, gen.GetCustomFieldValueAIExemptParams{
			ID:       entityID,
			TenantID: tenantID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return false, nil
			}
			return false, fmt.Errorf("checking custom_field_value ai_exempt: %w", err)
		}
		return exempt, nil

	case PrivacyEntityUser:
		exempt, err := c.read.GetUserAIExempt(ctx, gen.GetUserAIExemptParams{
			ID:       entityID,
			TenantID: tenantID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return false, nil
			}
			return false, fmt.Errorf("checking user ai_exempt: %w", err)
		}
		return exempt, nil

	default:
		// Unknown entity types have no exemption column — treat as non-exempt
		// rather than blocking the call. The tool handler is responsible for
		// its own not-found handling.
		return false, nil
	}
}

// blockedPrefixes is the hardcoded blocklist from §103 "Blocked tools". Any
// tool whose name starts with one of these prefixes is never available to the
// AI agent, regardless of the user's RBAC grants. This is enforced in code
// (not settings) because these operations are structurally unsafe for an AI
// to perform: RBAC/permission management, AI provider configuration, AI
// exemption management itself, user deletion, reading other users' private
// data, and security settings modification.
var blockedPrefixes = []string{
	"rbac.",
	"ai_privacy.",
	"ai_providers.",
	"settings.security.",
}

// blockedExact is the set of fully-qualified tool names that are blocked
// regardless of prefix match. These are the single operations called out in
// §103 that don't fit a prefix pattern.
var blockedExact = map[string]struct{}{
	"user.delete":       {},
	"user.read_private": {},
}

// IsBlockedOperation reports whether the given tool name is on the hardcoded
// blocklist and therefore never available to the AI agent. This is a
// defense-in-depth layer on top of the per-tool is_blocked flag in the
// ai_tools config JSONB: even if a row were misconfigured with is_blocked =
// false, the executor still refuses to dispatch.
//
// The check normalizes the tool name (trim + lowercase) before matching so
// an LLM cannot bypass the blocklist via casing or whitespace (Phase 11.03a).
func IsBlockedOperation(toolName string) bool {
	normalized := strings.ToLower(strings.TrimSpace(toolName))
	if _, ok := blockedExact[normalized]; ok {
		return true
	}
	for _, p := range blockedPrefixes {
		if strings.HasPrefix(normalized, p) {
			return true
		}
	}
	return false
}

// highStakesCategories is the set of §103 guide categories that unconditionally
// require HITL review even when the tool's requires_hitl flag is false. These
// categories have significant real-world consequences (visa applications,
// banking, grants, health insurance, university admission) where incorrect AI
// output could cause harm. The check is case-insensitive and whitespace-
// tolerant (Phase 11.02a).
var highStakesCategories = map[string]struct{}{
	"visa":             {},
	"banking":          {},
	"grants":           {},
	"health_insurance": {},
	"admission":        {},
}

// IsHighStakesCategory reports whether the given guide category is a §103
// high-stakes category that unconditionally requires HITL review. The check
// is case-insensitive and trims surrounding whitespace (Phase 11.02a).
func IsHighStakesCategory(category string) bool {
	normalized := strings.ToLower(strings.TrimSpace(category))
	_, ok := highStakesCategories[normalized]
	return ok
}
