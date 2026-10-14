// Package ai — unified_handlers.go implements the ADR-130 unified Japa AI
// HTTP surface: POST /ai/chat (SSE or JSON), the conversation CRUD endpoints,
// and the per-user AI preferences endpoints.
//
// Content negotiation on POST /ai/chat: clients that send
// `Accept: text/event-stream` get the SSE event stream; everyone else gets
// the legacy §20 JSON contract {reply, conversation_id, sources} so existing
// non-streaming clients keep working.
//
// Ref: japaconnect-engineering/adr/ADR-130-unified-japa-ai.md
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/worker"
)

// unifiedChatInput is the POST /ai/chat request body. `message` is the
// canonical field; `prompt` is accepted for legacy /ai/agent clients.
type unifiedChatInput struct {
	Message        string         `json:"message"`
	Prompt         string         `json:"prompt"` // legacy alias
	ConversationID string         `json:"conversation_id,omitempty"`
	Context        map[string]any `json:"context,omitempty"`
	Model          string         `json:"model,omitempty"`
	VoiceID        string         `json:"voice_id,omitempty"`
	Humanize       *bool          `json:"humanize,omitempty"`
	HumanizeLevel  string         `json:"humanize_level,omitempty"`
	Tools          *ToolSelection `json:"tools,omitempty"`
}

// HandleUnifiedChat serves POST /ai/chat (ADR-130). SSE when the client
// accepts text/event-stream; the legacy JSON reply otherwise.
func (h *AgentHTTPHandlers) HandleUnifiedChat(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}

	var input unifiedChatInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	prompt := input.Message
	if prompt == "" {
		prompt = input.Prompt
	}
	if prompt == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "message is required")
		return
	}

	// Sanitize client-supplied context to prevent prompt injection and cache
	// poisoning (ADR-010, BUG-007) — same allowlist as the legacy /ai/chat.
	input.Context = sanitizeClientContext(input.Context)

	var convID *pgtype.UUID
	if input.ConversationID != "" {
		if u, err := scanUUID(input.ConversationID); err == nil && u.Valid {
			convID = &u
		}
		// Non-UUID ids (Honcho-era) are dropped — Resolve creates a fresh
		// thread rather than erroring.
	}

	req := UnifiedChatRequest{
		Prompt:         prompt,
		ConversationID: convID,
		Context:        input.Context,
		Model:          input.Model,
		VoiceID:        input.VoiceID,
		Humanize:       input.Humanize,
		HumanizeLevel:  input.HumanizeLevel,
		Tools:          input.Tools,
	}

	events, err := h.agent.HandleUnifiedChat(r.Context(), user.UserID, req)
	if err != nil {
		writeAgentError(w, err)
		return
	}

	if strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		streamAgentEvents(w, events)
		return
	}

	// JSON contract (legacy §20 shape): drain the channel, emit the final
	// reply + conversation id + citation sources.
	var reply, conversationID, messageID string
	var sources []RAGChunk
	var loopErr *AgentEvent

	for ev := range events {
		switch ev.Type {
		case EventFinalResponse:
			reply = ev.Content
			conversationID = ev.ConversationID
			messageID = ev.MessageID
		case EventConversation, EventSessionStart:
			if ev.ConversationID != "" {
				conversationID = ev.ConversationID
			}
		case EventSources:
			sources = ev.Sources
		case EventError:
			e := ev
			loopErr = &e
		}
	}

	if loopErr != nil {
		errpkg.WriteError(w, http.StatusBadGateway, "ai_error", loopErr.Error)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"reply":           reply,
			"conversation_id": conversationID,
			"message_id":      messageID,
			"sources":         sources,
		},
	})
}

// streamAgentEvents drains the event channel as SSE `data: {json}\n\n`
// lines. Shared by the legacy /ai/agent handler and the unified /ai/chat.
func streamAgentEvents(w http.ResponseWriter, events <-chan AgentEvent) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable Nginx buffering
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)
	if flusher == nil {
		slog.Warn("ai: response writer does not support flushing, SSE will not stream incrementally")
	}

	for ev := range events {
		data, marshalErr := json.Marshal(ev)
		if marshalErr != nil {
			slog.Warn("ai: failed to marshal agent event", "type", ev.Type, "error", marshalErr)
			continue
		}

		if _, writeErr := fmt.Fprintf(w, "data: %s\n\n", data); writeErr != nil {
			return
		}

		if flusher != nil {
			flusher.Flush()
		}
	}
}

// ---------------------------------------------------------------------------
// Conversation endpoints — on *Handlers (owns Honcho for the legacy union)
// ---------------------------------------------------------------------------

// SetConversationService injects the ADR-130 transcript store.
func (h *Handlers) SetConversationService(s *ConversationService) {
	h.Convos = s
}

// SetAgentSessionQueries injects the session/tool-call reader used to embed
// tool calls in conversation detail responses.
func (h *Handlers) SetAgentSessionQueries(q AgentSessionQueries) {
	h.agentSessions = q
}

// UserAIContextFunc exposes buildUserAIContext for wiring into the agent
// handler's unified memory/context injection (ADR-130).
func (h *Handlers) UserAIContextFunc() func(ctx context.Context, userID pgtype.UUID, clientCtx map[string]any) map[string]any {
	return h.buildUserAIContext
}

// PersistChatTurnFunc exposes persistChatTurn for wiring into the agent
// handler's Honcho long-term-memory write (ADR-130).
func (h *Handlers) PersistChatTurnFunc() func(ctx context.Context, userID, message, reply, conversationID string, unified bool) {
	return h.persistChatTurn
}

// ChatModelValidator exposes isValidModelForCategory for wiring into the
// agent handler's per-turn model override validation (ADR-130).
func (h *Handlers) ChatModelValidator() func(model, category string) bool {
	return h.isValidModelForCategory
}

// HandleListConversationsV2 handles GET /ai/conversations for the unified
// surface: ai_conversations rows unioned with legacy Honcho-grouped threads
// (flagged "legacy": true, read-only).
func (h *Handlers) HandleListConversationsV2(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	pageSize := parsePageSize(r)
	pageOffset := parsePageOffset(r)

	type convoSummary struct {
		ConversationID string    `json:"conversation_id"`
		Title          string    `json:"title,omitempty"`
		Preview        string    `json:"preview"`
		TurnCount      int       `json:"turn_count"`
		LastEntryAt    time.Time `json:"last_entry_at"`
		Legacy         bool      `json:"legacy"`
	}

	out := []convoSummary{}
	dbIDs := map[string]bool{}

	// New relational threads.
	if h.Convos != nil {
		rows, _, err := h.Convos.List(r.Context(), user.UserID, 200, 0)
		if err != nil {
			slog.Warn("ai: listing ai_conversations", "error", err)
		}
		for _, c := range rows {
			dbIDs[c.ID.String()] = true
			out = append(out, convoSummary{
				ConversationID: c.ID.String(),
				Title:          c.Title,
				Preview:        c.LastMessage,
				TurnCount:      int(c.TurnCount),
				LastEntryAt:    c.UpdatedAt.Time,
			})
		}
	}

	// Legacy Honcho threads (read-only — flagged legacy:true).
	if h.Honcho != nil {
		entries, err := h.Honcho.GetContext(r.Context(), user.UserID.String())
		if err != nil {
			slog.Warn("ai: listing legacy conversations from honcho", "error", err)
		} else {
			// Phase 4: threads already backfilled into ai_conversations are
			// excluded via legacy_ref — including soft-deleted rows, so a
			// deleted migrated thread never resurfaces as legacy.
			var migratedRefs map[string]bool
			if h.Convos != nil {
				if m, merr := h.Convos.MigratedLegacyRefs(r.Context(), user.UserID); merr == nil {
					migratedRefs = m
				}
			}
			unmigrated := false
			legacy := map[string]*convoSummary{}
			for _, e := range entries {
				// Skip entries that belong to unified threads — the unified
				// path still writes each turn to Honcho long-term memory,
				// marked with metadata.unified (and conversation_id = the
				// ai_conversations id for entries written before the marker
				// existed). Without this the same thread lists twice.
				if e.Metadata != nil {
					if u, ok := e.Metadata["unified"].(bool); ok && u {
						continue
					}
				}
				cid := ""
				if e.Metadata != nil {
					if v, ok := e.Metadata["conversation_id"].(string); ok {
						cid = v
					}
				}
				if cid == "" {
					cid = e.ID
				}
				if dbIDs[cid] {
					continue
				}
				if migratedRefs["honcho:"+cid] {
					continue
				}
				unmigrated = true
				c, exists := legacy[cid]
				if !exists {
					c = &convoSummary{ConversationID: cid, Legacy: true}
					legacy[cid] = c
				}
				c.TurnCount++
				if e.CreatedAt.After(c.LastEntryAt) {
					c.LastEntryAt = e.CreatedAt
					c.Preview = e.Content
				}
			}
			for _, c := range legacy {
				out = append(out, *c)
			}
			// Lazy backfill enqueue (ADR-130 Phase 4) — unique-keyed per
			// user so repeated list hits don't pile up duplicate tasks.
			if unmigrated {
				h.enqueueConversationMigration(user.UserID)
			}
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].LastEntryAt.After(out[j].LastEntryAt) })

	total := int64(len(out))
	if int(pageOffset) >= len(out) {
		out = out[:0]
	} else {
		out = out[pageOffset:]
		if int(pageSize) > 0 && int(pageSize) < len(out) {
			out = out[:pageSize]
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": out,
		"meta": map[string]any{
			"total":       total,
			"page_size":   pageSize,
			"page_offset": pageOffset,
		},
	})
}

// enqueueConversationMigration lazily schedules the ADR-130 Phase 4 backfill
// when the conversation list still contains unmigrated Honcho groups. The
// task is unique-keyed per user so repeated list hits don't pile up
// duplicates; enqueue failures are logged and non-fatal.
func (h *Handlers) enqueueConversationMigration(userID pgtype.UUID) {
	if h.AsynqClient == nil {
		return
	}
	payload, err := json.Marshal(map[string]string{"user_id": userID.String()})
	if err != nil {
		return
	}
	opts := append(worker.TaskOpts(worker.TypeAIMigrateConversations),
		asynq.TaskID(worker.TypeAIMigrateConversations+":"+userID.String()),
	)
	if _, err := h.AsynqClient.Enqueue(asynq.NewTask(worker.TypeAIMigrateConversations, payload), opts...); err != nil {
		slog.Debug("ai: lazy conversation-migration enqueue skipped", "error", err)
	}
}

// HandleGetConversationV2 handles GET /ai/conversations/:id for the unified
// surface. UUID ids look up ai_conversations; anything else falls back to
// the legacy Honcho entry scan. Assistant messages embed their tool calls
// (looked up via the linked agent session).
func (h *Handlers) HandleGetConversationV2(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	idParam := chi.URLParam(r, "id")
	if idParam == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "conversation id is required")
		return
	}

	// Phase 4: a backfilled Honcho thread claims this id via
	// ai_conversations.legacy_ref. Resolve it before any legacy fallback so
	// the relational thread serves (or 404s when soft-deleted) instead of
	// resurfacing the read-only Honcho view.
	if h.Convos != nil {
		if migrated, err := h.Convos.LookupByLegacyRef(r.Context(), user.UserID, "honcho:"+idParam); err == nil {
			if migrated.DeletedAt.Valid {
				errpkg.WriteError(w, http.StatusNotFound, "conversation_not_found", "Conversation not found")
				return
			}
			h.serveUnifiedDetail(w, r, user, migrated.ID)
			return
		}
	}

	convID, parseErr := scanUUID(idParam)
	if parseErr != nil || !convID.Valid || h.Convos == nil {
		h.HandleGetConversation(w, r) // legacy Honcho fallback
		return
	}

	if h.serveUnifiedDetail(w, r, user, convID) {
		return
	}

	// Detail miss — distinguish "Honcho-era UUID" (fall back to the legacy
	// entry scan) from "unified thread, deleted" — the row still exists
	// with deleted_at set, and its Honcho memory entries must not
	// resurrect it.
	exists, exErr := h.Convos.ExistsIncludingDeleted(r.Context(), convID, user.UserID)
	if exErr == nil && exists {
		errpkg.WriteError(w, http.StatusNotFound, "conversation_not_found", "Conversation not found")
		return
	}
	h.HandleGetConversation(w, r) // may be a Honcho-era UUID
}

// serveUnifiedDetail renders the relational conversation + messages and
// returns false when the thread does not exist (caller falls back to the
// legacy path). Errors other than not-found write a 500 and return true —
// they must not be mistaken for "legacy thread".
func (h *Handlers) serveUnifiedDetail(w http.ResponseWriter, r *http.Request, user *auth.AuthenticatedUser, convID pgtype.UUID) bool {
	pageSize := parsePageSize(r)
	pageOffset := parsePageOffset(r)

	conv, msgs, err := h.Convos.Detail(r.Context(), user.UserID, convID, pageSize, pageOffset)
	if err != nil {
		if errors.Is(err, ErrConversationNotFound) {
			return false
		}
		slog.Warn("ai: loading conversation detail", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to load conversation")
		return true
	}

	// Embed tool calls on assistant messages that have an audit session.
	// toolCallToJSON decodes the JSONB arguments/result columns to nested
	// JSON (raw gen rows would serialize []byte as base64).
	type messageDTO struct {
		gen.AiMessage
		ToolCalls []map[string]any `json:"tool_calls,omitempty"`
	}
	out := make([]messageDTO, 0, len(msgs))
	for _, m := range msgs {
		dto := messageDTO{AiMessage: m}
		if m.AgentSessionID.Valid && h.agentSessions != nil {
			if calls, err := h.agentSessions.ListAgentToolCallsBySession(r.Context(), m.AgentSessionID); err == nil && len(calls) > 0 {
				dto.ToolCalls = make([]map[string]any, 0, len(calls))
				for _, tc := range calls {
					dto.ToolCalls = append(dto.ToolCalls, toolCallToJSON(tc))
				}
			}
		}
		out = append(out, dto)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"conversation_id": conv.ID.String(),
		"title":           conv.Title,
		"messages":        out,
		"created_at":      conv.CreatedAt.Time,
		"updated_at":      conv.UpdatedAt.Time,
	}})
	return true
}

// HandleDeleteConversation handles DELETE /ai/conversations/:id. Soft-deletes
// the thread and unlinks its agent session audit rows (kept per Rule 11).
// Legacy Honcho threads are read-only and cannot be deleted — 404.
func (h *Handlers) HandleDeleteConversation(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	idParam := chi.URLParam(r, "id")
	convID, parseErr := scanUUID(idParam)
	if parseErr != nil || !convID.Valid {
		errpkg.WriteError(w, http.StatusNotFound, "conversation_not_found", "Conversation not found")
		return
	}

	if h.Convos == nil {
		errpkg.WriteError(w, http.StatusNotFound, "conversation_not_found", "Conversation not found")
		return
	}

	if err := h.Convos.Delete(r.Context(), user.UserID, convID); err != nil {
		if errors.Is(err, ErrConversationNotFound) {
			errpkg.WriteError(w, http.StatusNotFound, "conversation_not_found", "Conversation not found")
			return
		}
		slog.Warn("ai: deleting conversation", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete conversation")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// AI preferences — GET/PUT /me/ai-preferences
// ---------------------------------------------------------------------------

// aiPrefsInput mirrors AIPrefs for the PUT body; kept separate so request
// validation can reject malformed shapes before touching the store.
type aiPrefsInput struct {
	DefaultMode        string   `json:"default_mode"`
	DisabledCategories []string `json:"disabled_categories"`
	DisabledTools      []string `json:"disabled_tools"`
	EnabledTools       []string `json:"enabled_tools"`
}

// HandleGetAIPrefs handles GET /me/ai-preferences.
func (h *Handlers) HandleGetAIPrefs(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	if h.Convos == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "not_configured", "AI preferences are not configured")
		return
	}

	prefs, err := h.Convos.Prefs(r.Context(), user.UserID)
	if err != nil {
		slog.Warn("ai: loading ai prefs", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to load AI preferences")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": prefs})
}

// HandlePutAIPrefs handles PUT /me/ai-preferences. Only ai_prefs is touched
// — dark_mode/language are managed by the profile preferences endpoints.
func (h *Handlers) HandlePutAIPrefs(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	if h.Convos == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "not_configured", "AI preferences are not configured")
		return
	}

	var input aiPrefsInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	if input.DefaultMode != "" && input.DefaultMode != "chat" && input.DefaultMode != "agent" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "default_mode must be \"chat\" or \"agent\"")
		return
	}

	// Bound the preference lists — an unbounded client payload would just
	// bloat user_preferences.ai_prefs (names never match real tools anyway).
	const maxPrefEntries, maxPrefLen = 512, 128
	lists := map[string][]string{
		"disabled_categories": input.DisabledCategories,
		"disabled_tools":      input.DisabledTools,
		"enabled_tools":       input.EnabledTools,
	}
	for field, xs := range lists {
		if len(xs) > maxPrefEntries {
			errpkg.WriteError(w, http.StatusBadRequest, "validation_error", field+" has too many entries")
			return
		}
		for _, x := range xs {
			if len(x) > maxPrefLen {
				errpkg.WriteError(w, http.StatusBadRequest, "validation_error", field+" contains an overlong entry")
				return
			}
		}
	}

	prefs := AIPrefs{
		DefaultMode:        input.DefaultMode,
		DisabledCategories: input.DisabledCategories,
		DisabledTools:      input.DisabledTools,
		EnabledTools:       input.EnabledTools,
	}

	if err := h.Convos.SavePrefs(r.Context(), user.UserID, prefs); err != nil {
		slog.Warn("ai: saving ai prefs", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to save AI preferences")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": prefs})
}
