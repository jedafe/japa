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
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// UnifiedChatRequest is the ADR-130 chat request — a superset of the legacy
// JapaBot and agent payloads.
type UnifiedChatRequest struct {
	Prompt         string
	ConversationID *pgtype.UUID
	// §103 AI Projects: stamps a newly created conversation into the
	// project. Validated owner-side before Resolve; ignored for existing
	// threads (explicit moves go through PUT …/project).
	ProjectID     *pgtype.UUID
	Context       map[string]any // client-supplied context (lowest precedence, ADR-025)
	Model         string
	VoiceID       string
	Humanize      *bool // nil → inherit global ai.humanize.* settings
	HumanizeLevel string
	Tools         *ToolSelection
}

// Unified-path sentinels. writeAgentError maps them alongside the existing
// agent sentinels.
var (
	errAINoPermission   = errors.New("ai: user lacks ai.context.use or agent.use permission")
	errInvalidModel     = errors.New("ai: model is not available for the chat category")
	errConversationBusy = errors.New("ai: conversation already has a running turn")
	errProjectNotFound  = errors.New("ai: project not found")
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
	// consent. PreflightError carries Status/Code/Message — returned
	// verbatim. ---
	if h.preflight != nil {
		if perr := h.preflight.CheckAdmission(ctx, userID, GateJapaBot); perr != nil {
			return nil, perr
		}
	}

	// --- Permission OR-gate (ADR-130): either surface permission admits.
	// agent.use additionally controls tool eligibility below. Runs before
	// CheckBudget so denied requests don't consume jc:ai:queries — the
	// legacy route checks permission in middleware for the same reason. ---
	hasChat, hasAgent := true, true

	if h.perms != nil {
		hasChat = h.perms.HasPermission(ctx, userID, "ai.context.use")
		hasAgent = h.perms.HasPermission(ctx, userID, "agent.use")

		if !hasChat && !hasAgent {
			return nil, errAINoPermission
		}
	}

	// --- One running turn per conversation: reject a second send while a
	// turn on the same thread is still in flight (concurrent turns would
	// interleave the transcript). Runs before CheckBudget so a busy
	// rejection doesn't consume jc:ai:queries. Owner-scoped — a foreign id
	// can never report busy (it's not the caller's thread anyway, so
	// Resolve below would just create a fresh one). ---
	if h.convos != nil && req.ConversationID != nil && req.ConversationID.Valid {
		timeout := h.getIntSetting(ctx, "agent.timeout_seconds", defaultTimeoutSeconds)
		busy, err := h.convos.HasRunningTurn(ctx, *req.ConversationID, userID, time.Now().Add(-time.Duration(timeout)*time.Second))
		if err != nil {
			slog.Warn("ai: conversation busy check failed, proceeding", "error", err)
		} else if busy {
			return nil, errConversationBusy
		}
	}

	// --- Unified daily budget (jc:ai:queries + jc:ai:cost) — after identity
	// checks, before any provider-visible work. ---
	if h.preflight != nil {
		if perr := h.preflight.CheckBudget(ctx, userID); perr != nil {
			return nil, perr
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

	if toolsEligible && h.registry != nil {
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

	// --- §103 AI Projects: a project_id on the request stamps a NEW
	// conversation into the project. On an existing owned conversation the
	// field is ignored entirely — no validation, no 404 — so stale clients
	// sending it keep working. Validation only runs when the id could
	// actually be applied (fresh chat, or a conversation_id that doesn't
	// resolve → Resolve creates a new row). ---
	if req.ProjectID != nil && req.ProjectID.Valid {
		applies := true
		if req.ConversationID != nil && req.ConversationID.Valid && h.convos != nil {
			if _, err := h.convos.Get(ctx, userID, *req.ConversationID); err == nil {
				applies = false
			}
		}
		if applies {
			// No projects service → can't validate ownership; fail closed
			// rather than stamping an unchecked foreign id.
			if h.projects == nil {
				return nil, errProjectNotFound
			}
			if _, perr := h.projects.Get(ctx, userID, *req.ProjectID); perr != nil {
				return nil, errProjectNotFound
			}
		}
	}

	// --- Resolve the transcript thread and snapshot history BEFORE
	// appending the current user message, so runLoop can append it itself
	// as the final user turn. ---
	var conv gen.AiConversation
	hasConv := false

	var history []ChatMessage
	msgCount := 0
	userMsgPersisted := false

	if h.convos != nil {
		titleLen := h.getIntSetting(ctx, "ai.conversation_title_length", defaultConversationTitleLen)

		c, err := h.convos.Resolve(ctx, userID, req.ConversationID, req.ProjectID, truncateRunes(req.Prompt, titleLen))
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

		// turn_count bookkeeping needs the true message count — the windowed
		// history length under-counts long threads. Degrade to the history
		// length if the count query fails.
		if n, err := h.convos.MessageCount(ctx, conv.ID); err == nil {
			msgCount = int(n) // #nosec G115 -- message counts fit int
		} else {
			slog.Warn("ai: counting conversation messages", "conversation_id", conv.ID, "error", err)
			msgCount = len(history)
		}
	}

	// --- §103 AI Projects: merge the project's instructions into the
	// request context server-side (lands in the USER CONTEXT system-prompt
	// block). `project_instructions` is not in the client context allowlist
	// so it can't be forged; setting it here also overwrites anything a
	// client tried to inject. ---
	if hasConv && h.projects != nil {
		if instructions := h.projects.InstructionsFor(ctx, userID, conv); instructions != "" {
			if req.Context == nil {
				req.Context = map[string]any{}
			}
			req.Context["project_instructions"] = instructions
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

	session, err := h.createSession(ctx, userID, req.Prompt, geoSpaceID, preSearchResult.Level, convUUID, conv.ProjectID)
	if err != nil {
		// The ai_agent_sessions_one_running_per_conv partial unique index
		// makes "one running turn per conversation" a DB invariant — a
		// concurrent send that slipped past the busy pre-check lands here.
		// A crashed/killed turn also lands here: its 'running' row outlives
		// the busy check's staleness window and would lock the thread
		// forever. Sweep rows past the timeout and retry once; a still-
		// failing insert means a genuinely live turn is in flight.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && hasConv {
			timeout := h.getIntSetting(ctx, "agent.timeout_seconds", defaultTimeoutSeconds)
			if swept, serr := h.sessions.FailStaleRunningAgentSessionsForConversation(ctx, gen.FailStaleRunningAgentSessionsForConversationParams{
				ConversationID: conv.ID,
				StaleBefore:    pgtype.Timestamptz{Time: time.Now().Add(-time.Duration(timeout) * time.Second), Valid: true},
			}); serr == nil && swept > 0 {
				slog.Info("ai: swept stale running session(s) for conversation", "conversation_id", conv.ID, "swept", swept)
				session, err = h.createSession(ctx, userID, req.Prompt, geoSpaceID, preSearchResult.Level, convUUID, conv.ProjectID)
			}
		}
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return nil, errConversationBusy
			}
			return nil, fmt.Errorf("ai: creating agent session: %w", err)
		}
	}

	// Persist the user turn only after the session exists — a rejected send
	// (busy race, DB failure) must not leave an unanswered user message in
	// the transcript polluting the next turn's history.
	if hasConv {
		if _, err := h.convos.AppendUserMessage(ctx, conv.ID, req.Prompt); err != nil {
			// Degrade: the reply still streams; the transcript just lacks
			// this user turn.
			slog.Warn("ai: failed to persist user message", "conversation_id", conv.ID, "error", err)
		} else {
			userMsgPersisted = true
		}
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
		prompt:        req.Prompt,
		geoSpaceID:    geoSpaceID,
		geoSummary:    geoSummary,
		preSearch:     preSearchResult,
		draftCtx:      draftCtx,
		catalog:       catalog,
		catalogSet:    true,
		agentCounters: len(catalog) > 0, // ADR-130: pure chat never touches jc:agent:* counters
		detached:      len(catalog) > 0, // tools on → run to completion in background
		history:       history,
		model:         effModel,
		unified:       &unifiedTurn{req: req, conv: conv, hasConv: hasConv, msgCount: msgCount, userMsgPersisted: userMsgPersisted},
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
