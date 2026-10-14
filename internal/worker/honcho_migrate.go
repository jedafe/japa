package worker

// honcho_migrate.go — ai:migrate_conversations (ADR-130 Phase 4).
//
// Backfills one user's legacy Honcho context entries into the relational
// ai_conversations/ai_messages transcript store. Entries group by
// metadata.conversation_id; entries without one become one-message threads
// keyed by entry id. The thread row carries legacy_ref='honcho:{group id}'
// under a partial unique index, so reruns are no-ops and the conversation
// list's legacy union never double-counts a migrated thread.
//
// Lazily enqueued by GET /ai/conversations when unmigrated legacy entries
// exist; the task is unique-keyed per user.
//
// Ref: japaconnect-engineering/adr/ADR-130-unified-japa-ai.md
// Ref: japaconnect-engineering/sections/11-job-queue-asynq.md

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/honcho"
)

// honchoContextFetcher is the slice of honcho.Client the backfill needs —
// declared as an interface so tests stub it.
type honchoContextFetcher interface {
	GetContext(ctx context.Context, userID string) ([]honcho.ContextEntry, error)
}

var honchoFetcher honchoContextFetcher

// RegisterHonchoFetcher injects the Honcho context fetcher for the
// ai:migrate_conversations handler. Nil → the task completes as a no-op
// (Honcho not configured in this deployment).
func RegisterHonchoFetcher(f honchoContextFetcher) {
	honchoFetcher = f
}

// errLegacyRefTaken signals the ON CONFLICT DO NOTHING insert saw an
// existing thread for the legacy ref — a concurrent run already wrote it.
var errLegacyRefTaken = errors.New("legacy ref already migrated")

type aiMigrateConversationsPayload struct {
	UserID string `json:"user_id"`
}

// HandleAIMigrateConversations backfills one user's Honcho entries into
// ai_conversations + ai_messages. Idempotent: the legacy_ref unique index
// and the per-thread lookup make reruns skip already-migrated groups.
func HandleAIMigrateConversations(ctx context.Context, t *asynq.Task) error {
	var p aiMigrateConversationsPayload
	if err := parsePayloadOrSkip(t, &p); err != nil {
		return err
	}
	userID, err := scanUUID(p.UserID)
	if err != nil {
		return fmt.Errorf("ai:migrate_conversations: %w: %w", err, asynq.SkipRetry)
	}
	if dbPool == nil {
		return errors.New("worker: db pool not registered")
	}
	if honchoFetcher == nil {
		// Honcho not configured — nothing to migrate, mark success.
		return nil
	}

	entries, err := honchoFetcher.GetContext(ctx, p.UserID)
	if err != nil {
		return fmt.Errorf("ai:migrate_conversations: fetch honcho context: %w", err)
	}

	// Group by metadata.conversation_id; entries the unified path already
	// wrote (metadata.unified) must not be migrated. Ungrouped entries form
	// one-message threads keyed by entry id.
	groups := map[string][]honcho.ContextEntry{}
	for _, e := range entries {
		if e.Metadata != nil {
			if u, ok := e.Metadata["unified"].(bool); ok && u {
				continue
			}
		}
		cid := legacyConversationID(e)
		groups[cid] = append(groups[cid], e)
	}
	if len(groups) == 0 {
		return nil
	}

	q := gen.New(dbPool.Writer)
	titleLen := 80
	if appSettings != nil {
		titleLen = appSettings.GetIntOr(ctx, "ai.conversation_title_length", 80)
	}

	var migrated, skipped int
	for _, legacyID := range sortedKeys(groups) {
		ents := groups[legacyID]
		sort.SliceStable(ents, func(i, j int) bool { return ents[i].CreatedAt.Before(ents[j].CreatedAt) })

		legacyRef := "honcho:" + legacyID

		// Fast path: already migrated (including soft-deleted rows — a
		// deleted thread must not be resurrected by a rerun).
		if _, err := q.GetAIConversationByLegacyRef(ctx, gen.GetAIConversationByLegacyRefParams{
			UserID:    userID,
			LegacyRef: legacyRef,
		}); err == nil {
			skipped++
			continue
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("ai:migrate_conversations: lookup %s: %w", legacyRef, err)
		}

		// Reuse the legacy id as the conversation PK when it is UUID-shaped
		// so existing client-side links keep resolving.
		convID, err := scanUUID(legacyID)
		if err != nil || !convID.Valid {
			convID = pgtype.UUID{Bytes: uuid.New(), Valid: true}
		}

		first, last := ents[0].CreatedAt, ents[len(ents)-1].CreatedAt
		title := migrateTitle(ents, titleLen)

		txErr := db.WithTx(ctx, dbPool.Writer, func(txq *gen.Queries) error {
			if _, err := txq.CreateAIConversationWithLegacyRef(ctx, gen.CreateAIConversationWithLegacyRefParams{
				ID:        convID,
				UserID:    userID,
				Title:     title,
				LegacyRef: legacyRef,
				CreatedAt: pgtype.Timestamptz{Time: first, Valid: !first.IsZero()},
				UpdatedAt: pgtype.Timestamptz{Time: last, Valid: !last.IsZero()},
			}); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return errLegacyRefTaken
				}
				return err
			}
			for _, e := range ents {
				if _, err := txq.InsertAIMessageAt(ctx, gen.InsertAIMessageAtParams{
					ConversationID: convID,
					Role:           legacyMessageRole(e),
					Content:        e.Content,
					CreatedAt:      pgtype.Timestamptz{Time: e.CreatedAt, Valid: !e.CreatedAt.IsZero()},
				}); err != nil {
					return err
				}
			}
			return nil
		})
		switch {
		case errors.Is(txErr, errLegacyRefTaken):
			skipped++
		case txErr != nil:
			return fmt.Errorf("ai:migrate_conversations: thread %s: %w", legacyRef, txErr)
		default:
			migrated++
		}
	}

	slog.Info("ai:migrate_conversations done",
		"user_id", p.UserID, "migrated", migrated, "skipped", skipped)
	return nil
}

// legacyConversationID resolves a Honcho entry to its thread key:
// metadata.conversation_id when present, else the entry id (ungrouped
// entries become their own one-message thread).
func legacyConversationID(e honcho.ContextEntry) string {
	if e.Metadata != nil {
		if v, ok := e.Metadata["conversation_id"].(string); ok && v != "" {
			return v
		}
	}
	return e.ID
}

// legacyMessageRole maps the Honcho metadata role onto the ai_messages
// role check constraint (user|assistant). Unknown/empty roles default to
// user — legacy entries were predominantly user prompts.
func legacyMessageRole(e honcho.ContextEntry) string {
	if e.Metadata != nil {
		switch v, _ := e.Metadata["role"].(string); v {
		case "assistant", "ai", "bot":
			return "assistant"
		case "user", "human":
			return "user"
		}
	}
	return "user"
}

// migrateTitle derives the thread title from the first user-role message,
// truncated to the configured ai.conversation_title_length runes.
func migrateTitle(entries []honcho.ContextEntry, titleLen int) string {
	if titleLen <= 0 {
		titleLen = 80
	}
	for _, e := range entries {
		if legacyMessageRole(e) != "user" {
			continue
		}
		runes := []rune(e.Content)
		if len(runes) > titleLen {
			return string(runes[:titleLen])
		}
		return e.Content
	}
	return ""
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
