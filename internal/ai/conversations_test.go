package ai

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// stubConvoStore is a map-backed ConversationStore for unit tests.
type stubConvoStore struct {
	convs    map[string]gen.AiConversation
	msgs     map[string][]gen.AiMessage // conversation_id hex → messages
	prefs    map[string][]byte          // user_id hex → ai_prefs
	sessions map[string]gen.AiAgentSession
	// busyConvs: conversation_ids reported as having a running turn
	// (HasRunningTurn); absent → not busy.
	busyConvs map[string]bool
	seq       int
}

func newStubConvoStore() *stubConvoStore {
	return &stubConvoStore{
		convs:    map[string]gen.AiConversation{},
		msgs:     map[string][]gen.AiMessage{},
		prefs:    map[string][]byte{},
		sessions: map[string]gen.AiAgentSession{},
	}
}

func uuidStr(u pgtype.UUID) string { return fmt.Sprintf("%x", u.Bytes) }

func (s *stubConvoStore) nextID() pgtype.UUID {
	s.seq++
	var b [16]byte
	b[15] = byte(s.seq)
	b[14] = byte(s.seq >> 8)
	return pgtype.UUID{Bytes: b, Valid: true}
}

func (s *stubConvoStore) CreateAIConversation(_ context.Context, arg gen.CreateAIConversationParams) (gen.AiConversation, error) {
	c := gen.AiConversation{ID: s.nextID(), UserID: arg.UserID, Title: arg.Title,
		CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		UpdatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}}
	s.convs[uuidStr(c.ID)] = c
	return c, nil
}

func (s *stubConvoStore) GetAIConversationForUser(_ context.Context, arg gen.GetAIConversationForUserParams) (gen.AiConversation, error) {
	c, ok := s.convs[uuidStr(arg.ID)]
	if !ok || c.DeletedAt.Valid || uuidStr(c.UserID) != uuidStr(arg.UserID) {
		return gen.AiConversation{}, pgx.ErrNoRows
	}
	return c, nil
}

func (s *stubConvoStore) TouchAIConversation(_ context.Context, id pgtype.UUID) error {
	c, ok := s.convs[uuidStr(id)]
	if !ok {
		return pgx.ErrNoRows
	}
	c.UpdatedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	s.convs[uuidStr(id)] = c
	return nil
}

func (s *stubConvoStore) ListAIConversationsForUser(_ context.Context, arg gen.ListAIConversationsForUserParams) ([]gen.ListAIConversationsForUserRow, error) {
	var out []gen.ListAIConversationsForUserRow
	for _, c := range s.convs {
		if uuidStr(c.UserID) == uuidStr(arg.UserID) && !c.DeletedAt.Valid {
			row := gen.ListAIConversationsForUserRow{
				ID: c.ID, UserID: c.UserID, Title: c.Title,
				CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, DeletedAt: c.DeletedAt,
			}
			for _, m := range s.msgs[uuidStr(c.ID)] {
				row.TurnCount++
				row.LastMessage = m.Content
			}
			out = append(out, row)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.Time.After(out[j].UpdatedAt.Time) })
	return out, nil
}

func (s *stubConvoStore) CountAIConversationsForUser(_ context.Context, userID pgtype.UUID) (int64, error) {
	var n int64
	for _, c := range s.convs {
		if uuidStr(c.UserID) == uuidStr(userID) && !c.DeletedAt.Valid {
			n++
		}
	}
	return n, nil
}

func (s *stubConvoStore) HasRunningAgentTurnForConversation(_ context.Context, arg gen.HasRunningAgentTurnForConversationParams) (bool, error) {
	return s.busyConvs[uuidStr(arg.ConversationID)], nil
}

func (s *stubConvoStore) AIConversationExistsIncludingDeleted(_ context.Context, arg gen.AIConversationExistsIncludingDeletedParams) (bool, error) {
	c, ok := s.convs[uuidStr(arg.ID)]
	return ok && uuidStr(c.UserID) == uuidStr(arg.UserID), nil
}

func (s *stubConvoStore) SoftDeleteAIConversationForUser(_ context.Context, arg gen.SoftDeleteAIConversationForUserParams) (pgtype.UUID, error) {
	c, ok := s.convs[uuidStr(arg.ID)]
	if !ok || c.DeletedAt.Valid || uuidStr(c.UserID) != uuidStr(arg.UserID) {
		return pgtype.UUID{}, pgx.ErrNoRows
	}
	c.DeletedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	s.convs[uuidStr(arg.ID)] = c
	return c.ID, nil
}

func (s *stubConvoStore) InsertAIMessage(_ context.Context, arg gen.InsertAIMessageParams) (gen.AiMessage, error) {
	m := gen.AiMessage{ID: s.nextID(), ConversationID: arg.ConversationID, Role: arg.Role,
		Content: arg.Content, Model: arg.Model, VoiceID: arg.VoiceID, TokensUsed: arg.TokensUsed,
		CostUsd: arg.CostUsd, AgentSessionID: arg.AgentSessionID,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().Add(time.Duration(s.seq) * time.Millisecond), Valid: true}}
	k := uuidStr(arg.ConversationID)
	s.msgs[k] = append(s.msgs[k], m)
	return m, nil
}

func (s *stubConvoStore) SetAIMessageAgentSession(_ context.Context, arg gen.SetAIMessageAgentSessionParams) error {
	for k, msgs := range s.msgs {
		for i, m := range msgs {
			if uuidStr(m.ID) == uuidStr(arg.ID) {
				s.msgs[k][i].AgentSessionID = arg.AgentSessionID
				return nil
			}
		}
	}
	return pgx.ErrNoRows
}

func (s *stubConvoStore) ListAIConversationHistory(_ context.Context, arg gen.ListAIConversationHistoryParams) ([]gen.AiMessage, error) {
	msgs := s.msgs[uuidStr(arg.ConversationID)]
	if int32(len(msgs)) > arg.RowLimit {
		msgs = msgs[len(msgs)-int(arg.RowLimit):]
	}
	return msgs, nil
}

func (s *stubConvoStore) ListAIConversationMessages(_ context.Context, arg gen.ListAIConversationMessagesParams) ([]gen.AiMessage, error) {
	return s.msgs[uuidStr(arg.ConversationID)], nil
}

func (s *stubConvoStore) LinkAgentSessionToConversation(_ context.Context, arg gen.LinkAgentSessionToConversationParams) error {
	sess, ok := s.sessions[uuidStr(arg.ID)]
	if !ok {
		return pgx.ErrNoRows
	}
	sess.ConversationID = arg.ConversationID
	sess.MessageID = arg.MessageID
	s.sessions[uuidStr(arg.ID)] = sess
	return nil
}

func (s *stubConvoStore) UnlinkAgentSessionsForConversation(_ context.Context, conversationID pgtype.UUID) error {
	for k, sess := range s.sessions {
		if uuidStr(sess.ConversationID) == uuidStr(conversationID) {
			sess.ConversationID = pgtype.UUID{}
			s.sessions[k] = sess
		}
	}
	return nil
}

func (s *stubConvoStore) GetAIPreferences(_ context.Context, userID pgtype.UUID) ([]byte, error) {
	raw, ok := s.prefs[uuidStr(userID)]
	if !ok {
		return nil, pgx.ErrNoRows
	}
	return raw, nil
}

func (s *stubConvoStore) UpsertAIPreferences(_ context.Context, arg gen.UpsertAIPreferencesParams) error {
	s.prefs[uuidStr(arg.UserID)] = arg.AiPrefs
	return nil
}

func TestConversationService_Resolve(t *testing.T) {
	st := newStubConvoStore()
	svc := NewConversationService(st, nil)
	uid := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	other := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}

	// No id → creates.
	c1, err := svc.Resolve(context.Background(), uid, nil, "hello world")
	if err != nil || c1.Title != "hello world" {
		t.Fatalf("create failed: %v %+v", err, c1)
	}

	// Same id → reuses.
	c2, err := svc.Resolve(context.Background(), uid, &c1.ID, "ignored")
	if err != nil || c2.ID != c1.ID {
		t.Fatalf("reuse failed: %v %+v", err, c2)
	}

	// Unknown id → creates fresh (legacy Honcho-era ids must not 404).
	bogus := pgtype.UUID{Bytes: [16]byte{9, 9}, Valid: true}
	c3, err := svc.Resolve(context.Background(), uid, &bogus, "fresh")
	if err != nil || c3.ID == bogus {
		t.Fatalf("unknown id should create fresh: %v %+v", err, c3)
	}

	// Foreign id → creates fresh too.
	c4, err := svc.Resolve(context.Background(), other, &c1.ID, "foreign")
	if err != nil || c4.ID == c1.ID {
		t.Fatalf("foreign id should create fresh: %v %+v", err, c4)
	}
}

func TestConversationService_HistoryWindow(t *testing.T) {
	st := newStubConvoStore()
	svc := NewConversationService(st, nil)
	uid := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	conv, _ := svc.Resolve(context.Background(), uid, nil, "t")

	for i := 0; i < 25; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		if _, err := svc.AppendUserMessage(context.Background(), conv.ID, fmt.Sprintf("m%d", i)); err != nil {
			_ = role
			t.Fatal(err)
		}
	}

	hist, err := svc.History(context.Background(), conv.ID, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 20 {
		t.Fatalf("expected 20, got %d", len(hist))
	}
	// Oldest five dropped → first row is m5, chronological order preserved.
	if hist[0].Content != "m5" || hist[19].Content != "m24" {
		t.Fatalf("window ordering wrong: %s … %s", hist[0].Content, hist[19].Content)
	}
}

func TestConversationService_DetailOwnerScoped(t *testing.T) {
	st := newStubConvoStore()
	svc := NewConversationService(st, nil)
	owner := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	stranger := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	conv, _ := svc.Resolve(context.Background(), owner, nil, "t")

	if _, _, err := svc.Detail(context.Background(), stranger, conv.ID, 50, 0); !errors.Is(err, ErrConversationNotFound) {
		t.Fatalf("expected ErrConversationNotFound, got %v", err)
	}
	if _, _, err := svc.Detail(context.Background(), owner, conv.ID, 50, 0); err != nil {
		t.Fatalf("owner detail failed: %v", err)
	}
}

func TestConversationService_DeleteUnlinksSessions(t *testing.T) {
	st := newStubConvoStore()
	svc := NewConversationService(st, nil)
	uid := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	conv, _ := svc.Resolve(context.Background(), uid, nil, "t")

	sessID := pgtype.UUID{Bytes: [16]byte{7}, Valid: true}
	st.sessions[uuidStr(sessID)] = gen.AiAgentSession{ID: sessID, ConversationID: conv.ID}

	if err := svc.Delete(context.Background(), uid, conv.ID); err != nil {
		t.Fatal(err)
	}
	if got := st.sessions[uuidStr(sessID)]; got.ConversationID.Valid {
		t.Fatal("session still linked after delete")
	}
	if err := svc.Delete(context.Background(), uid, conv.ID); !errors.Is(err, ErrConversationNotFound) {
		t.Fatalf("second delete should 404, got %v", err)
	}
}

func TestConversationService_PrefsRoundTrip(t *testing.T) {
	st := newStubConvoStore()
	svc := NewConversationService(st, nil)
	uid := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}

	p, err := svc.Prefs(context.Background(), uid)
	if err != nil || p.DefaultMode != "" {
		t.Fatalf("default prefs: %v %+v", err, p)
	}

	want := AIPrefs{DefaultMode: "agent", DisabledCategories: []string{"forum"}, DisabledTools: []string{"content.delete"}}
	if err := svc.SavePrefs(context.Background(), uid, want); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Prefs(context.Background(), uid)
	if err != nil {
		t.Fatal(err)
	}
	if got.DefaultMode != "agent" || len(got.DisabledCategories) != 1 || got.DisabledTools[0] != "content.delete" {
		t.Fatalf("prefs round-trip wrong: %+v", got)
	}
}
