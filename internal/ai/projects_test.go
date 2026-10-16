package ai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// stubProjectStore is a map-backed ProjectStore for unit tests. Voice ids
// in `voices` are considered live persona voices; `createErr`/`updateErr`
// inject a raw store failure (e.g. a 23505 unique-violation).
type stubProjectStore struct {
	projects  map[string]gen.AiProject
	voices    map[string]bool
	convs     map[string]gen.AiConversation
	sessions  map[string]gen.AiAgentSession
	createErr error
	updateErr error
	seq       int
}

func newStubProjectStore() *stubProjectStore {
	return &stubProjectStore{
		projects: map[string]gen.AiProject{},
		voices:   map[string]bool{},
		convs:    map[string]gen.AiConversation{},
		sessions: map[string]gen.AiAgentSession{},
	}
}

func (s *stubProjectStore) nextID() pgtype.UUID {
	s.seq++

	var b [16]byte
	b[15] = byte(s.seq)
	b[14] = byte(s.seq >> 8)
	b[13] = byte(s.seq >> 16)

	return pgtype.UUID{Bytes: b, Valid: true}
}

func (s *stubProjectStore) CreateAIProject(_ context.Context, arg gen.CreateAIProjectParams) (gen.AiProject, error) {
	if s.createErr != nil {
		return gen.AiProject{}, s.createErr
	}

	p := gen.AiProject{
		ID:             s.nextID(),
		UserID:         arg.UserID,
		Name:           arg.Name,
		Instructions:   arg.Instructions,
		DefaultModel:   arg.DefaultModel,
		DefaultTone:    arg.DefaultTone,
		DefaultVoiceID: arg.DefaultVoiceID,
		DefaultTools:   arg.DefaultTools,
		CreatedAt:      pgtype.Timestamptz{Time: time.Now(), Valid: true},
		UpdatedAt:      pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	s.projects[uuidStr(p.ID)] = p

	return p, nil
}

func (s *stubProjectStore) GetAIProjectForUser(_ context.Context, arg gen.GetAIProjectForUserParams) (gen.AiProject, error) {
	p, ok := s.projects[uuidStr(arg.ID)]
	if !ok || p.DeletedAt.Valid || uuidStr(p.UserID) != uuidStr(arg.UserID) {
		return gen.AiProject{}, pgx.ErrNoRows
	}

	return p, nil
}

func (s *stubProjectStore) ListAIProjectsForUser(_ context.Context, arg gen.ListAIProjectsForUserParams) ([]gen.ListAIProjectsForUserRow, error) {
	var out []gen.ListAIProjectsForUserRow

	for _, p := range s.projects {
		if uuidStr(p.UserID) != uuidStr(arg.UserID) || p.DeletedAt.Valid {
			continue
		}

		row := gen.ListAIProjectsForUserRow{
			ID: p.ID, UserID: p.UserID, Name: p.Name, Instructions: p.Instructions,
			DefaultModel: p.DefaultModel, DefaultTone: p.DefaultTone,
			DefaultVoiceID: p.DefaultVoiceID, DefaultTools: p.DefaultTools,
			CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt, DeletedAt: p.DeletedAt,
		}

		for _, c := range s.convs {
			if uuidStr(c.ProjectID) == uuidStr(p.ID) && !c.DeletedAt.Valid {
				row.ConversationCount++
			}
		}

		for _, ss := range s.sessions {
			if uuidStr(ss.ProjectID) == uuidStr(p.ID) {
				row.SessionCount++
			}
		}

		out = append(out, row)
	}

	return out, nil
}

func (s *stubProjectStore) CountAIProjectsForUser(_ context.Context, userID pgtype.UUID) (int64, error) {
	var n int64

	for _, p := range s.projects {
		if uuidStr(p.UserID) == uuidStr(userID) && !p.DeletedAt.Valid {
			n++
		}
	}

	return n, nil
}

func (s *stubProjectStore) UpdateAIProjectForUser(_ context.Context, arg gen.UpdateAIProjectForUserParams) (gen.AiProject, error) {
	if s.updateErr != nil {
		return gen.AiProject{}, s.updateErr
	}

	p, ok := s.projects[uuidStr(arg.ID)]
	if !ok || p.DeletedAt.Valid || uuidStr(p.UserID) != uuidStr(arg.UserID) {
		return gen.AiProject{}, pgx.ErrNoRows
	}

	p.Name = arg.Name
	p.Instructions = arg.Instructions
	p.DefaultModel = arg.DefaultModel
	p.DefaultTone = arg.DefaultTone
	p.DefaultVoiceID = arg.DefaultVoiceID
	p.DefaultTools = arg.DefaultTools
	p.UpdatedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	s.projects[uuidStr(p.ID)] = p

	return p, nil
}

func (s *stubProjectStore) SoftDeleteAIProjectForUser(_ context.Context, arg gen.SoftDeleteAIProjectForUserParams) (pgtype.UUID, error) {
	p, ok := s.projects[uuidStr(arg.ID)]
	if !ok || p.DeletedAt.Valid || uuidStr(p.UserID) != uuidStr(arg.UserID) {
		return pgtype.UUID{}, pgx.ErrNoRows
	}

	p.DeletedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	s.projects[uuidStr(p.ID)] = p

	return p.ID, nil
}

func (s *stubProjectStore) ClearAIConversationProjectForProject(_ context.Context, projectID pgtype.UUID) error {
	for k, c := range s.convs {
		// Mirrors the SQL's `deleted_at IS NULL` filter — soft-deleted
		// threads keep their historical grouping.
		if !c.DeletedAt.Valid && uuidStr(c.ProjectID) == uuidStr(projectID) {
			c.ProjectID = pgtype.UUID{}
			s.convs[k] = c
		}
	}

	return nil
}

func (s *stubProjectStore) ClearAIAgentSessionProjectForProject(_ context.Context, projectID pgtype.UUID) error {
	for k, ss := range s.sessions {
		if uuidStr(ss.ProjectID) == uuidStr(projectID) {
			ss.ProjectID = pgtype.UUID{}
			s.sessions[k] = ss
		}
	}

	return nil
}

func (s *stubProjectStore) SetAIConversationProject(_ context.Context, arg gen.SetAIConversationProjectParams) (pgtype.UUID, error) {
	c, ok := s.convs[uuidStr(arg.ID)]
	if !ok || c.DeletedAt.Valid || uuidStr(c.UserID) != uuidStr(arg.UserID) {
		return pgtype.UUID{}, pgx.ErrNoRows
	}

	c.ProjectID = arg.ProjectID
	s.convs[uuidStr(arg.ID)] = c

	return c.ID, nil
}

func (s *stubProjectStore) SetAIAgentSessionProject(_ context.Context, arg gen.SetAIAgentSessionProjectParams) (pgtype.UUID, error) {
	ss, ok := s.sessions[uuidStr(arg.ID)]
	if !ok || uuidStr(ss.UserID) != uuidStr(arg.UserID) {
		return pgtype.UUID{}, pgx.ErrNoRows
	}

	ss.ProjectID = arg.ProjectID
	s.sessions[uuidStr(arg.ID)] = ss

	return ss.ID, nil
}

func (s *stubProjectStore) SetAIAgentSessionsProjectByConversation(_ context.Context, arg gen.SetAIAgentSessionsProjectByConversationParams) error {
	for k, ss := range s.sessions {
		if uuidStr(ss.ConversationID) == uuidStr(arg.ConversationID) {
			ss.ProjectID = arg.ProjectID
			s.sessions[k] = ss
		}
	}

	return nil
}

func (s *stubProjectStore) GetAgentSessionForUser(_ context.Context, arg gen.GetAgentSessionForUserParams) (gen.AiAgentSession, error) {
	ss, ok := s.sessions[uuidStr(arg.ID)]
	if !ok || uuidStr(ss.UserID) != uuidStr(arg.UserID) {
		return gen.AiAgentSession{}, pgx.ErrNoRows
	}

	return ss, nil
}

func (s *stubProjectStore) GetPersonaVoiceByID(_ context.Context, id pgtype.UUID) (gen.PersonaVoice, error) {
	if !s.voices[uuidStr(id)] {
		return gen.PersonaVoice{}, pgx.ErrNoRows
	}

	return gen.PersonaVoice{ID: id}, nil
}

func TestProjectService_NameTakenMapping(t *testing.T) {
	st := newStubProjectStore()
	svc := NewProjectService(st, nil)
	uid := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}

	// A 23505 on create must surface as ErrProjectNameTaken, not a wrapped 500.
	st.createErr = &pgconn.PgError{Code: "23505", ConstraintName: "idx_ai_projects_user_name"}

	_, err := svc.Create(t.Context(), uid, ProjectInput{Name: "Dup"})
	if !errors.Is(err, ErrProjectNameTaken) {
		t.Fatalf("expected ErrProjectNameTaken, got %v", err)
	}

	// Same on update.
	st.createErr = nil

	p, err := svc.Create(t.Context(), uid, ProjectInput{Name: "ok"})
	if err != nil {
		t.Fatal(err)
	}

	st.updateErr = &pgconn.PgError{Code: "23505"}

	_, err = svc.Update(t.Context(), uid, p.ID, ProjectInput{Name: "Dup"})
	if !errors.Is(err, ErrProjectNameTaken) {
		t.Fatalf("expected ErrProjectNameTaken on update, got %v", err)
	}
}

func TestProjectService_VoicePinValidation(t *testing.T) {
	st := newStubProjectStore()
	svc := NewProjectService(st, nil)
	uid := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	voice := pgtype.UUID{Bytes: [16]byte{9}, Valid: true}
	st.voices[uuidStr(voice)] = true

	// Malformed id → ErrInvalidVoicePin.
	if _, err := svc.Create(t.Context(), uid, ProjectInput{Name: "a", DefaultVoiceID: "not-a-uuid"}); !errors.Is(err, ErrInvalidVoicePin) {
		t.Fatalf("malformed voice should fail, got %v", err)
	}
	// Well-formed but unknown id → ErrInvalidVoicePin.
	unknown := pgtype.UUID{Bytes: [16]byte{8}, Valid: true}
	if _, err := svc.Create(t.Context(), uid, ProjectInput{Name: "a", DefaultVoiceID: unknown.String()}); !errors.Is(err, ErrInvalidVoicePin) {
		t.Fatalf("unknown voice should fail, got %v", err)
	}
	// Known id → accepted; empty → clears (accepted).
	if _, err := svc.Create(t.Context(), uid, ProjectInput{Name: "a", DefaultVoiceID: voice.String()}); err != nil {
		t.Fatalf("known voice rejected: %v", err)
	}

	if _, err := svc.Create(t.Context(), uid, ProjectInput{Name: "b", DefaultVoiceID: ""}); err != nil {
		t.Fatalf("empty voice pin should clear, got %v", err)
	}
}

func TestProjectService_OwnershipAndSentinels(t *testing.T) {
	st := newStubProjectStore()
	svc := NewProjectService(st, nil)
	owner := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	stranger := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}

	p, err := svc.Create(t.Context(), owner, ProjectInput{Name: "mine", Instructions: "ins"})
	if err != nil {
		t.Fatal(err)
	}

	// Foreign get/update/delete → ErrProjectNotFound (no existence leak).
	if _, err := svc.Get(t.Context(), stranger, p.ID); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("foreign get should 404, got %v", err)
	}

	if _, err := svc.Update(t.Context(), stranger, p.ID, ProjectInput{Name: "x"}); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("foreign update should 404, got %v", err)
	}

	if err := svc.Delete(t.Context(), stranger, p.ID); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("foreign delete should 404, got %v", err)
	}

	// Assign with a foreign project id → ErrProjectNotFound.
	convID := pgtype.UUID{Bytes: [16]byte{7}, Valid: true}
	st.convs[uuidStr(convID)] = gen.AiConversation{ID: convID, UserID: owner}
	sessID := pgtype.UUID{Bytes: [16]byte{5}, Valid: true}
	st.sessions[uuidStr(sessID)] = gen.AiAgentSession{ID: sessID, UserID: owner, ConversationID: convID}

	if err := svc.SetConversationProject(t.Context(), owner, convID, &p.ID); err != nil {
		t.Fatalf("assign to own project failed: %v", err)
	}
	// The conversation's sessions cascade to the same project.
	if uuidStr(st.sessions[uuidStr(sessID)].ProjectID) != uuidStr(p.ID) {
		t.Fatal("session did not cascade to the conversation's project")
	}

	if err := svc.SetConversationProject(t.Context(), stranger, convID, &p.ID); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("foreign project assign should 404 project, got %v", err)
	}
	// Missing/foreign conversation → ErrConversationNotFound.
	missing := pgtype.UUID{Bytes: [16]byte{6}, Valid: true}
	if err := svc.SetConversationProject(t.Context(), owner, missing, &p.ID); !errors.Is(err, ErrConversationNotFound) {
		t.Fatalf("missing convo should be ErrConversationNotFound, got %v", err)
	}

	if err := svc.SetConversationProject(t.Context(), stranger, convID, nil); !errors.Is(err, ErrConversationNotFound) {
		t.Fatalf("foreign convo unassign should be ErrConversationNotFound, got %v", err)
	}
	// Sessions: same contract, ErrSessionNotFound.
	if err := svc.SetSessionProject(t.Context(), owner, missing, &p.ID); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("missing session should be ErrSessionNotFound, got %v", err)
	}
	// sessID is linked to a conversation — it mirrors the thread's project
	// and can't be assigned independently.
	if err := svc.SetSessionProject(t.Context(), owner, sessID, &p.ID); !errors.Is(err, ErrSessionLinked) {
		t.Fatalf("linked session assign should be ErrSessionLinked, got %v", err)
	}
	// A standalone session assigns normally.
	standID := pgtype.UUID{Bytes: [16]byte{9}, Valid: true}
	st.sessions[uuidStr(standID)] = gen.AiAgentSession{ID: standID, UserID: owner}

	if err := svc.SetSessionProject(t.Context(), owner, standID, &p.ID); err != nil {
		t.Fatalf("standalone session assign failed: %v", err)
	}

	if uuidStr(st.sessions[uuidStr(standID)].ProjectID) != uuidStr(p.ID) {
		t.Fatal("standalone session not stamped with project")
	}
}

func TestProjectService_InstructionsFor(t *testing.T) {
	st := newStubProjectStore()
	svc := NewProjectService(st, nil)
	uid := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}

	p, _ := svc.Create(t.Context(), uid, ProjectInput{Name: "p", Instructions: "project rules"})

	// Ungrouped convo → empty.
	if got := svc.InstructionsFor(t.Context(), uid, gen.AiConversation{}); got != "" {
		t.Fatalf("ungrouped convo should yield no instructions, got %q", got)
	}
	// Grouped → instructions.
	conv := gen.AiConversation{ProjectID: p.ID}
	if got := svc.InstructionsFor(t.Context(), uid, conv); got != "project rules" {
		t.Fatalf("expected instructions, got %q", got)
	}
	// Foreign viewer → empty (project owner-scoped).
	stranger := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	if got := svc.InstructionsFor(t.Context(), stranger, conv); got != "" {
		t.Fatalf("foreign instructions leaked: %q", got)
	}
	// Deleted project → empty.
	if err := svc.Delete(t.Context(), uid, p.ID); err != nil {
		t.Fatal(err)
	}

	if got := svc.InstructionsFor(t.Context(), uid, conv); got != "" {
		t.Fatalf("deleted project still injected instructions: %q", got)
	}
}

func TestProjectService_DeleteUngroups(t *testing.T) {
	st := newStubProjectStore()
	svc := NewProjectService(st, nil)
	uid := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}

	p, _ := svc.Create(t.Context(), uid, ProjectInput{Name: "p"})
	convID := pgtype.UUID{Bytes: [16]byte{7}, Valid: true}
	sessID := pgtype.UUID{Bytes: [16]byte{8}, Valid: true}
	deadConvID := pgtype.UUID{Bytes: [16]byte{9}, Valid: true}
	st.convs[uuidStr(convID)] = gen.AiConversation{ID: convID, UserID: uid, ProjectID: p.ID}
	// A soft-deleted conversation keeps its historical grouping.
	st.convs[uuidStr(deadConvID)] = gen.AiConversation{
		ID: deadConvID, UserID: uid, ProjectID: p.ID,
		DeletedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	st.sessions[uuidStr(sessID)] = gen.AiAgentSession{ID: sessID, UserID: uid, ProjectID: p.ID}

	if err := svc.Delete(t.Context(), uid, p.ID); err != nil {
		t.Fatal(err)
	}

	if st.convs[uuidStr(convID)].ProjectID.Valid {
		t.Fatal("conversation still grouped after delete")
	}

	if st.sessions[uuidStr(sessID)].ProjectID.Valid {
		t.Fatal("session still grouped after delete")
	}

	if !st.convs[uuidStr(deadConvID)].ProjectID.Valid {
		t.Fatal("soft-deleted conversation lost its grouping")
	}
}

// A project_id on POST /ai/chat stamps only NEW conversations — an existing
// thread keeps its membership, and a foreign/unknown project id 404s.
func TestUnifiedChat_ProjectID(t *testing.T) {
	provider := &recordingProvider{name: "mock", reply: "ok"}
	st := newStubProjectStore()
	projects := NewProjectService(st, nil)
	h, convSt, _ := newUnifiedHandler(t, provider, &stubPerms{granted: map[string]bool{"ai.context.use": true}},
		WithAgentProjects(projects))
	uid := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	other := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}

	p, err := projects.Create(t.Context(), uid, ProjectInput{Name: "ctx"})
	if err != nil {
		t.Fatal(err)
	}

	// Foreign project id → errProjectNotFound.
	pid := p.ID

	_, err = h.HandleUnifiedChat(t.Context(), other, UnifiedChatRequest{Prompt: "hi", ProjectID: &pid})
	if !errors.Is(err, errProjectNotFound) {
		t.Fatalf("foreign project_id should fail, got %v", err)
	}

	// Owned project id → new conversation stamped.
	events, err := h.HandleUnifiedChat(t.Context(), uid, UnifiedChatRequest{Prompt: "hi", ProjectID: &pid})
	if err != nil {
		t.Fatal(err)
	}

	var convID pgtype.UUID

	for e := range events {
		if e.Type != "conversation" {
			continue
		}

		if e.ProjectID != p.ID.String() {
			t.Fatalf("conversation event missing project_id, got %q", e.ProjectID)
		}

		u, _ := scanUUID(e.ConversationID)
		convID = u
	}

	if !convID.Valid {
		t.Fatal("no conversation event emitted")
	}

	stored, err := convSt.GetAIConversationForUser(t.Context(), gen.GetAIConversationForUserParams{ID: convID, UserID: uid})
	if err != nil {
		t.Fatal(err)
	}

	if uuidStr(stored.ProjectID) != uuidStr(p.ID) {
		t.Fatalf("conversation not stamped with project: %+v", stored.ProjectID)
	}

	// Second send on the SAME conversation with a different project id must
	// not re-group it.
	p2, _ := projects.Create(t.Context(), uid, ProjectInput{Name: "other"})
	p2id := p2.ID

	reEvents, err := h.HandleUnifiedChat(t.Context(), uid, UnifiedChatRequest{Prompt: "again", ConversationID: &convID, ProjectID: &p2id})
	if err != nil {
		t.Fatal(err)
	}

	for range reEvents {
	}

	stored, _ = convSt.GetAIConversationForUser(t.Context(), gen.GetAIConversationForUserParams{ID: convID, UserID: uid})
	if uuidStr(stored.ProjectID) != uuidStr(p.ID) {
		t.Fatalf("existing conversation re-grouped: %+v", stored.ProjectID)
	}

	// A FOREIGN project_id on an existing conversation is ignored entirely —
	// no 404, no re-group. The field only matters when a new row is created.
	pf, _ := projects.Create(t.Context(), other, ProjectInput{Name: "foreign"})
	pfID := pf.ID

	ignEvents, err := h.HandleUnifiedChat(t.Context(), uid, UnifiedChatRequest{Prompt: "third", ConversationID: &convID, ProjectID: &pfID})
	if err != nil {
		t.Fatalf("foreign project_id on existing conversation should be ignored, got %v", err)
	}

	for range ignEvents {
	}

	stored, _ = convSt.GetAIConversationForUser(t.Context(), gen.GetAIConversationForUserParams{ID: convID, UserID: uid})
	if uuidStr(stored.ProjectID) != uuidStr(p.ID) {
		t.Fatalf("existing conversation re-grouped to foreign project: %+v", stored.ProjectID)
	}
}

// POST /ai/projects must reject a pinned voice that doesn't exist.
func TestHandleCreateProject_InvalidVoicePin(t *testing.T) {
	st := newStubProjectStore()
	h := &Handlers{Projects: NewProjectService(st, nil)}

	r := authedRequest(t, http.MethodPost, "/api/v1/ai/projects",
		`{"name":"x","default_voice_id":"not-a-uuid"}`, 1)
	w := httptest.NewRecorder()
	h.HandleCreateProject(w, r)

	if w.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed voice pin should 400, got %d", w.Result().StatusCode)
	}
}

// PATCH must keep omitted fields, clear pins on "", and map dup names to 409.
func TestHandleUpdateProject_MergeAndConflict(t *testing.T) {
	st := newStubProjectStore()
	svc := NewProjectService(st, nil)
	h := &Handlers{Projects: svc}
	uid := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}

	p, err := svc.Create(t.Context(), uid, ProjectInput{Name: "one", Instructions: "keep me", DefaultTone: "witty"})
	if err != nil {
		t.Fatal(err)
	}

	// PATCH tone only — name/instructions must survive.
	r := authedRequest(t, http.MethodPatch, "/api/v1/ai/projects/"+p.ID.String(),
		`{"default_tone":"formal"}`, 1)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", p.ID.String())
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.HandleUpdateProject(w, r)

	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("patch failed: %d %s", w.Result().StatusCode, w.Body.String())
	}

	body := w.Body.String()
	if !strings.Contains(body, `"default_tone":"formal"`) || !strings.Contains(body, `"instructions":"keep me"`) || !strings.Contains(body, `"name":"one"`) {
		t.Fatalf("PATCH dropped merged fields: %s", body)
	}

	// Duplicate name → 409 project_name_taken.
	st.updateErr = &pgconn.PgError{Code: "23505"}
	r = authedRequest(t, http.MethodPatch, "/api/v1/ai/projects/"+p.ID.String(), `{"name":"dup"}`, 1)
	rctx2 := chi.NewRouteContext()
	rctx2.URLParams.Add("id", p.ID.String())
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx2))
	w = httptest.NewRecorder()
	h.HandleUpdateProject(w, r)

	res := w.Result()
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("dup name should 409, got %d", res.StatusCode)
	}

	if code := decodeError(t, w.Body)["code"]; code != "project_name_taken" {
		t.Fatalf("expected project_name_taken, got %v", code)
	}
}
