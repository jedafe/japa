// Package ai — agent_unified.go implements the unified Japa AI entry point
// (ADR-130). HandleUnifiedChat runs the §103 agent loop for every chat turn:
// the resolved tool catalog is what differs between "agent" and "chat" —
// an empty catalog degenerates the loop to JapaBot-class chat (pre-search
// RAG + Honcho memory + LLM answer), so a single engine serves both.
//
// Admission differs from the legacy /ai/agent path in two ways:
//   - Surface gate is feature.japabot (GateJapaBot), and permission is an
//     OR-gate — ai.context.use OR agent.use. The legacy endpoint required
//     agent.use only; agent-only users must keep working after the merge.
//   - Agent-scoped counters (jc:agent:rate, jc:agent:cost) run only when the
//     resolved catalog is non-empty; pure-chat turns consume jc:ai:* only.
//
// Ref: japaconnect-engineering/adr/ADR-130-unified-japa-ai.md
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Amendment)
package ai

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// UnifiedChatRequest is the ADR-130 chat request — a superset of the legacy
// JapaBot and agent payloads.
type UnifiedChatRequest struct {
	Prompt         string
	ConversationID *pgtype.UUID
	Context        map[string]any // client-supplied context (lowest precedence, ADR-025)
	Model          string
	VoiceID        string
	Humanize       *bool // nil → inherit global ai.humanize.* settings
	HumanizeLevel  string
	Tools          *ToolSelection
}

// Unified-path sentinels. writeAgentError maps them alongside the existing
// agent sentinels.
var (
	errAINoPermission = errors.New("ai: user lacks ai.context.use or agent.use permission")
	errInvalidModel   = errors.New("ai: model is not available for the chat category")
)

const (
	defaultHistoryWindow        = 20
	defaultConversationTitleLen = 80
)

// HandleUnifiedChat is the single chat entry point (ADR-130). Pre-check
// order matters: the catalog is resolved BEFORE the agent-scoped counters
// so pure-chat turns never consume jc:agent:* quota.
func (h *AgentHandler) HandleUnifiedChat(ctx context.Context, userID pgtype.UUID, req UnifiedChatRequest) (<-chan AgentEvent, error) {
	if h == nil {
		return nil, errAgentNotConfigured
	}

	if req.Prompt == "" {
		return nil, errAgentEmptyPrompt
	}

	// --- Shared §20 admission: feature.japabot reachability + ai_processing
	// consent, then the unified daily budget (jc:ai:queries/jc:ai:cost).
	// PreflightError carries Status/Code/Message — returned verbatim. ---
	if h.preflight != nil {
		if perr := h.preflight.CheckAdmission(ctx, userID, GateJapaBot); perr != nil {
			return nil, perr
		}

		if perr := h.preflight.CheckBudget(ctx, userID); perr != nil {
			return nil, perr
		}
	}

	// --- Permission OR-gate (ADR-130): either surface permission admits.
	// agent.use additionally controls tool eligibility below. ---
	hasChat, hasAgent := true, true

	if h.perms != nil {
		hasChat = h.perms.HasPermission(ctx, userID, "ai.context.use")
		hasAgent = h.perms.HasPermission(ctx, userID, "agent.use")

		if !hasChat && !hasAgent {
			return nil, errAINoPermission
		}
	}

	// --- Tool eligibility: agent.use RBAC node AND the global tools
	// kill-switch (feature.ai_agent / legacy agent.enabled fallback).
	// Ineligible users still get JapaBot-class chat. ---
	toolsEligible := hasAgent && h.agentEnabled(ctx)

	// --- Resolve the catalog EARLY (ADR-130 ordering): platform is_enabled
	// + RBAC (cached in buildToolCatalog), then user prefs, then the
	// per-message override. Each layer can only narrow. ---
	var prefs AIPrefs

	if h.convos != nil {
		if p, err := h.convos.Prefs(ctx, userID); err == nil {
			prefs = p
		} else {
			slog.Warn("ai: loading ai prefs, using defaults", "user_id", userID, "error", err)
		}
	}

	var catalog []ToolConfig

	if toolsEligible {
		catalog = FilterCatalog(h.buildToolCatalog(ctx, userID), h.registry.Categories(), prefs, req.Tools)
	}

	// --- Agent-scoped counters only when the resolved catalog is non-empty
	// (ADR-130): a pure-chat turn must not consume jc:agent:rate/cost. ---
	if len(catalog) > 0 {
		if err := h.checkRateLimit(ctx, userID); err != nil {
			return nil, errAgentRateLimited
		}

		if err := h.checkDailyBudget(ctx, userID); err != nil {
			return nil, errAgentBudgetExceeded
		}
	}

	// --- Model override validation (before any writes so a bad model id
	// can't open a session). ---
	effModel := req.Model
	if effModel == "" {
		effModel = h.getStringSetting(ctx, "ai.model.default.chat", "")
	}
	if effModel != "" && h.modelValidator != nil && !h.modelValidator(effModel, h.llmCategory) {
		return nil, errInvalidModel
	}

	// --- Resolve the transcript thread and snapshot history BEFORE
	// appending the current user message, so runLoop can append it itself
	// as the final user turn. ---
	var conv gen.AiConversation
	hasConv := false

	var history []ChatMessage

	if h.convos != nil {
		titleLen := h.getIntSetting(ctx, "ai.conversation_title_length", defaultConversationTitleLen)

		c, err := h.convos.Resolve(ctx, userID, req.ConversationID, truncateRunes(req.Prompt, titleLen))
		if err != nil {
			return nil, fmt.Errorf("ai: resolving conversation: %w", err)
		}

		conv, hasConv = c, true

		window := int32(h.getIntSetting(ctx, "ai.history_window", defaultHistoryWindow)) // #nosec G115 -- bounded by setting
		if h2, err := h.convos.History(ctx, conv.ID, window); err == nil {
			history = h2
		} else {
			slog.Warn("ai: loading conversation history", "conversation_id", conv.ID, "error", err)
		}

		if _, err := h.convos.AppendUserMessage(ctx, conv.ID, req.Prompt); err != nil {
			// Degrade: the reply still streams; the transcript just lacks
			// this user turn.
			slog.Warn("ai: failed to persist user message", "conversation_id", conv.ID, "error", err)
		}
	}

	// --- Geo + pre-search (same as legacy path). ---
	geoSpaceID, geoSummary := h.resolveGeoContext(ctx, userID)
	preSearchResult := h.runPreSearch(ctx, req.Prompt, geoSpaceID, userID)

	// --- Audit session, linked to the conversation when one exists. ---
	var convUUID pgtype.UUID
	if hasConv {
		convUUID = conv.ID
	}

	session, err := h.createSession(ctx, userID, req.Prompt, geoSpaceID, preSearchResult.Level, convUUID)
	if err != nil {
		return nil, fmt.Errorf("ai: creating agent session: %w", err)
	}

	h.setSessionState(ctx, session.ID)

	// --- Phase 8 draft-request context only when tools are actually live —
	// the prompt section instructs the LLM to call tools, which would be
	// wrong (and confusing) on a pure-chat turn. ---
	var draftCtx draftRequestContext
	if len(catalog) > 0 {
		draftCtx = h.resolveDraftRequestContext(ctx, userID, preSearchResult.Level)
	}

	events := make(chan AgentEvent, 32)

	go h.runLoop(ctx, session, userID, loopParams{
		prompt:     req.Prompt,
		geoSpaceID: geoSpaceID,
		geoSummary: geoSummary,
		preSearch:  preSearchResult,
		draftCtx:   draftCtx,
		catalog:    catalog,
		catalogSet: true,
		history:    history,
		model:      effModel,
		unified:    &unifiedTurn{req: req, conv: conv, hasConv: hasConv},
	}, events)

	return events, nil
}

// truncateRunes returns s truncated to n runes (auto-titles).
func truncateRunes(s string, n int) string {
	if n <= 0 {
		return s
	}
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n])
}
