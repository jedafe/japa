package worker

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jedafe/japa/internal/honcho"
)

func TestHandleAIMigrateConversations_MalformedPayload_Skips(t *testing.T) {
	task := asynq.NewTask(TypeAIMigrateConversations, []byte("{not-json"))

	err := HandleAIMigrateConversations(t.Context(), task)
	if !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("malformed payload should skip retry, got %v", err)
	}
}

func TestHandleAIMigrateConversations_BadUserID_Skips(t *testing.T) {
	payload, _ := json.Marshal(map[string]string{"user_id": "not-a-uuid"})
	task := asynq.NewTask(TypeAIMigrateConversations, payload)

	err := HandleAIMigrateConversations(t.Context(), task)
	if err == nil || !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("bad user_id should skip retry, got %v", err)
	}
}

func TestHandleAIMigrateConversations_NoFetcher_NoOp(t *testing.T) {
	// dbPool is required before the fetcher check, so this exercises the
	// dbPool guard — with no registered pool the task errors for retry.
	payload, _ := json.Marshal(map[string]string{"user_id": "00000000-0000-0000-0000-000000000001"})

	task := asynq.NewTask(TypeAIMigrateConversations, payload)
	if err := HandleAIMigrateConversations(t.Context(), task); err == nil {
		t.Fatal("expected an error when the db pool is unregistered")
	}
}

func TestLegacyConversationID(t *testing.T) {
	withConv := honcho.ContextEntry{
		ID:       "entry-1",
		Metadata: map[string]any{"conversation_id": "conv-9"},
	}
	if got := legacyConversationID(withConv); got != "conv-9" {
		t.Fatalf("expected conv-9, got %q", got)
	}

	noConv := honcho.ContextEntry{ID: "entry-2", Metadata: map[string]any{}}
	if got := legacyConversationID(noConv); got != "entry-2" {
		t.Fatalf("ungrouped entry should key on entry id, got %q", got)
	}

	emptyConv := honcho.ContextEntry{
		ID:       "entry-3",
		Metadata: map[string]any{"conversation_id": ""},
	}
	if got := legacyConversationID(emptyConv); got != "entry-3" {
		t.Fatalf("empty conversation_id should key on entry id, got %q", got)
	}
}

func TestLegacyMessageRole(t *testing.T) {
	cases := []struct {
		role any
		want string
	}{
		{"assistant", "assistant"},
		{"ai", "assistant"},
		{"bot", "assistant"},
		{"user", "user"},
		{"human", "user"},
		{"system", "user"}, // unknown roles default to user
		{nil, "user"},
	}
	for _, c := range cases {
		m := map[string]any{}
		if c.role != nil {
			m["role"] = c.role
		}

		if got := legacyMessageRole(honcho.ContextEntry{Metadata: m}); got != c.want {
			t.Fatalf("role %v: got %q want %q", c.role, got, c.want)
		}
	}
	// No metadata at all → user.
	if got := legacyMessageRole(honcho.ContextEntry{}); got != "user" {
		t.Fatalf("no metadata: got %q", got)
	}
}

func TestMigrateTitle(t *testing.T) {
	now := time.Now()

	entries := []honcho.ContextEntry{
		{Content: "assistant text", Metadata: map[string]any{"role": "assistant"}, CreatedAt: now},
		{Content: "first user question", Metadata: map[string]any{"role": "user"}, CreatedAt: now.Add(time.Second)},
		{Content: "second user question", Metadata: map[string]any{"role": "user"}, CreatedAt: now.Add(2 * time.Second)},
	}
	if got := migrateTitle(entries, 80); got != "first user question" {
		t.Fatalf("title should come from the first user message, got %q", got)
	}

	if got := migrateTitle([]honcho.ContextEntry{
		{Content: "a fairly long title that exceeds the cap", Metadata: map[string]any{"role": "user"}},
	}, 10); got != "a fairly l" {
		t.Fatalf("title should truncate to titleLen runes, got %q", got)
	}

	// No user message → empty title (client shows a placeholder).
	if got := migrateTitle(entries[:1], 80); got != "" {
		t.Fatalf("no user message should give empty title, got %q", got)
	}
}
