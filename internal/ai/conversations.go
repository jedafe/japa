package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// ConversationService is the unified Japa AI transcript store (ADR-130).
// ai_conversations is the user-facing thread; ai_messages its turns.
// ai_agent_sessions remains the per-turn audit record, linked via
// conversation_id/message_id.
//
// Ref: japaconnect-engineering/adr/ADR-130-unified-japa-ai.md

// ConversationStore is the subset of *gen.Queries the service needs.
// *gen.Queries satisfies it; tests stub it.
type ConversationStore interface {
	CreateAIConversation(ctx context.Context, arg gen.CreateAIConversationParams) (gen.AiConversation, error)
	GetAIConversationForUser(ctx context.Context, arg gen.GetAIConversationForUserParams) (gen.AiConversation, error)
	TouchAIConversation(ctx context.Context, id pgtype.UUID) error
	ListAIConversationsForUser(ctx context.Context, arg gen.ListAIConversationsForUserParams) ([]gen.AiConversation, error)
	CountAIConversationsForUser(ctx context.Context, userID pgtype.UUID) (int64, error)
	SoftDeleteAIConversationForUser(ctx context.Context, arg gen.SoftDeleteAIConversationForUserParams) (pgtype.UUID, error)
	InsertAIMessage(ctx context.Context, arg gen.InsertAIMessageParams) (gen.AiMessage, error)
	SetAIMessageAgentSession(ctx context.Context, arg gen.SetAIMessageAgentSessionParams) error
	ListAIConversationHistory(ctx context.Context, arg gen.ListAIConversationHistoryParams) ([]gen.AiMessage, error)
	ListAIConversationMessages(ctx context.Context, arg gen.ListAIConversationMessagesParams) ([]gen.AiMessage, error)
	LinkAgentSessionToConversation(ctx context.Context, arg gen.LinkAgentSessionToConversationParams) error
	UnlinkAgentSessionsForConversation(ctx context.Context, conversationID pgtype.UUID) error
	GetAIPreferences(ctx context.Context, userID pgtype.UUID) ([]byte, error)
	UpsertAIPreferences(ctx context.Context, arg gen.UpsertAIPreferencesParams) error
}

// ErrConversationNotFound is returned by owner-scoped lookups/deletes when
// the conversation doesn't exist or belongs to another user.
var ErrConversationNotFound = errors.New("conversation not found")

type ConversationService struct {
	store ConversationStore
	pool  *pgxpool.Pool // nil in tests — Delete then runs non-transactionally
}

func NewConversationService(store ConversationStore, pool *pgxpool.Pool) *ConversationService {
	return &ConversationService{store: store, pool: pool}
}

// Resolve returns the existing conversation when conversationID is a live,
// owned row; otherwise it creates a fresh one. Unknown/foreign IDs never
// error — legacy clients may send Honcho-era IDs that never existed in the
// table (ADR-130). title is stored only on create.
func (s *ConversationService) Resolve(ctx context.Context, userID pgtype.UUID, conversationID *pgtype.UUID, title string) (gen.AiConversation, error) {
	if conversationID != nil && conversationID.Valid {
		conv, err := s.store.GetAIConversationForUser(ctx, gen.GetAIConversationForUserParams{
			ID:     *conversationID,
			UserID: userID,
		})
		if err == nil {
			return conv, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return gen.AiConversation{}, fmt.Errorf("resolving conversation: %w", err)
		}
		// Unknown or foreign → fall through to create.
	}
	return s.store.CreateAIConversation(ctx, gen.CreateAIConversationParams{
		UserID: userID,
		Title:  title,
	})
}

func (s *ConversationService) AppendUserMessage(ctx context.Context, convID pgtype.UUID, content string) (gen.AiMessage, error) {
	return s.store.InsertAIMessage(ctx, gen.InsertAIMessageParams{
		ConversationID: convID,
		Role:           "user",
		Content:        content,
	})
}

func (s *ConversationService) AppendAssistantMessage(ctx context.Context, convID pgtype.UUID, content, model string, tokens int, cost float64, sessionID *pgtype.UUID) (gen.AiMessage, error) {
	var modelArg *string
	if model != "" {
		modelArg = &model
	}
	var sess pgtype.UUID
	if sessionID != nil {
		sess = *sessionID
	}
	return s.store.InsertAIMessage(ctx, gen.InsertAIMessageParams{
		ConversationID: convID,
		Role:           "assistant",
		Content:        content,
		Model:          modelArg,
		TokensUsed:     int32(tokens),
		CostUsd:        floatToNumeric(cost),
		AgentSessionID: sess,
	})
}

// History returns the newest `limit` messages oldest-first, mapped to
// provider-agnostic ChatMessage for prompt building.
func (s *ConversationService) History(ctx context.Context, convID pgtype.UUID, limit int32) ([]ChatMessage, error) {
	rows, err := s.store.ListAIConversationHistory(ctx, gen.ListAIConversationHistoryParams{
		ConversationID: convID,
		RowLimit:       limit,
	})
	if err != nil {
		return nil, fmt.Errorf("loading conversation history: %w", err)
	}
	out := make([]ChatMessage, 0, len(rows))
	for _, m := range rows {
		out = append(out, ChatMessage{Role: m.Role, Content: m.Content})
	}
	return out, nil
}

func (s *ConversationService) List(ctx context.Context, userID pgtype.UUID, limit, offset int32) ([]gen.AiConversation, int64, error) {
	rows, err := s.store.ListAIConversationsForUser(ctx, gen.ListAIConversationsForUserParams{
		UserID:     userID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("listing conversations: %w", err)
	}
	total, err := s.store.CountAIConversationsForUser(ctx, userID)
	if err != nil {
		return nil, 0, fmt.Errorf("counting conversations: %w", err)
	}
	return rows, total, nil
}

// Detail returns the conversation + its messages, owner-scoped.
func (s *ConversationService) Detail(ctx context.Context, userID, convID pgtype.UUID, limit, offset int32) (gen.AiConversation, []gen.AiMessage, error) {
	conv, err := s.store.GetAIConversationForUser(ctx, gen.GetAIConversationForUserParams{
		ID:     convID,
		UserID: userID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.AiConversation{}, nil, ErrConversationNotFound
		}
		return gen.AiConversation{}, nil, fmt.Errorf("loading conversation: %w", err)
	}
	msgs, err := s.store.ListAIConversationMessages(ctx, gen.ListAIConversationMessagesParams{
		ConversationID: convID,
		PageLimit:      limit,
		PageOffset:     offset,
	})
	if err != nil {
		return gen.AiConversation{}, nil, fmt.Errorf("listing messages: %w", err)
	}
	return conv, msgs, nil
}

// Delete soft-deletes the thread (Rule 11) and unlinks its agent session
// rows so the audit trail survives without exposing the deleted thread.
// When a pool is available both writes run in one transaction; tests with a
// stub store run them sequentially.
func (s *ConversationService) Delete(ctx context.Context, userID, convID pgtype.UUID) error {
	do := func(st ConversationStore) error {
		deleted, err := st.SoftDeleteAIConversationForUser(ctx, gen.SoftDeleteAIConversationForUserParams{
			ID:     convID,
			UserID: userID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrConversationNotFound
			}
			return fmt.Errorf("deleting conversation: %w", err)
		}
		_ = deleted
		return st.UnlinkAgentSessionsForConversation(ctx, convID)
	}
	if s.pool != nil {
		return db.WithTx(ctx, s.pool, func(q *gen.Queries) error { return do(q) })
	}
	return do(s.store)
}

// LinkSession points an agent audit session at its conversation and the
// assistant message it produced.
func (s *ConversationService) LinkSession(ctx context.Context, sessionID, convID, messageID pgtype.UUID) error {
	return s.store.LinkAgentSessionToConversation(ctx, gen.LinkAgentSessionToConversationParams{
		ID:             sessionID,
		ConversationID: convID,
		MessageID:      messageID,
	})
}

// Prefs reads user_preferences.ai_prefs; absent row or malformed JSON
// degrade to defaults.
func (s *ConversationService) Prefs(ctx context.Context, userID pgtype.UUID) (AIPrefs, error) {
	raw, err := s.store.GetAIPreferences(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AIPrefs{}, nil
		}
		return AIPrefs{}, fmt.Errorf("loading ai prefs: %w", err)
	}
	return ParseAIPrefs(raw), nil
}

func (s *ConversationService) SavePrefs(ctx context.Context, userID pgtype.UUID, prefs AIPrefs) error {
	raw, err := json.Marshal(prefs)
	if err != nil {
		return fmt.Errorf("encoding ai prefs: %w", err)
	}
	return s.store.UpsertAIPreferences(ctx, gen.UpsertAIPreferencesParams{
		UserID:  userID,
		AiPrefs: raw,
	})
}
