package ai

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// Stubs
// ---------------------------------------------------------------------------

type stubPersonaVoicesService struct {
	voices []PersonaVoiceItem
	total  int64
	voice  PersonaVoiceItem
	err    error
}

func (s *stubPersonaVoicesService) ListPersonaVoices(ctx context.Context, limit, offset int32) ([]PersonaVoiceItem, int64, error) {
	if s.err != nil {
		return nil, 0, s.err
	}

	return s.voices, s.total, nil
}

func (s *stubPersonaVoicesService) GetPersonaVoice(ctx context.Context, id pgtype.UUID) (PersonaVoiceItem, error) {
	if s.err != nil {
		return PersonaVoiceItem{}, s.err
	}

	return s.voice, nil
}

func (s *stubPersonaVoicesService) GetPersonaVoiceBySlug(ctx context.Context, slug string) (PersonaVoiceItem, error) {
	if s.err != nil {
		return PersonaVoiceItem{}, s.err
	}

	return s.voice, nil
}

type stubPersonaCatalogService struct {
	items   []AgentPersonaItem
	total   int64
	persona AgentPersonaItem
	err     error
}

func (s *stubPersonaCatalogService) ListPersonas(ctx context.Context, status, q, sort, dir string, limit, offset int32) ([]AgentPersonaItem, int64, error) {
	if s.err != nil {
		return nil, 0, s.err
	}

	return s.items, s.total, nil
}

func (s *stubPersonaCatalogService) GetPersona(ctx context.Context, id pgtype.UUID) (AgentPersonaItem, error) {
	if s.err != nil {
		return AgentPersonaItem{}, s.err
	}

	return s.persona, nil
}

// ---------------------------------------------------------------------------
// persona_voices.list
// ---------------------------------------------------------------------------

func TestPersonaVoicesListHandler_Success(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{
		PersonaVoices: &stubPersonaVoicesService{
			total: 2,
			voices: []PersonaVoiceItem{
				{ID: "00000000-0000-0000-0000-0000000000a1", Name: "Expat Mentor", Slug: "expat-mentor", VoicePrompt: "warm, practical", IsSystem: true},
				{ID: "00000000-0000-0000-0000-0000000000a2", Name: "Visa Analyst", Slug: "visa-analyst", VoicePrompt: "precise, formal"},
			},
		},
	}

	handler := PersonaVoicesListHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(`{}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}

	m := parseToolResult(t, tr)
	if m["total"] != float64(2) {
		t.Errorf("total = %v, want 2", m["total"])
	}

	voices, _ := m["voices"].([]any)
	if len(voices) != 2 {
		t.Fatalf("expected 2 voices, got %d", len(voices))
	}

	first, _ := voices[0].(map[string]any)
	if first["slug"] != "expat-mentor" {
		t.Errorf("first slug = %v, want expat-mentor", first["slug"])
	}

	// voice_prompt must be omitted by default.
	if _, ok := first["voice_prompt"]; ok {
		t.Errorf("voice_prompt should be omitted when include_prompts is false")
	}
}

func TestPersonaVoicesListHandler_IncludePrompts(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{
		PersonaVoices: &stubPersonaVoicesService{
			total: 1,
			voices: []PersonaVoiceItem{
				{ID: "00000000-0000-0000-0000-0000000000a1", Name: "Expat Mentor", Slug: "expat-mentor", VoicePrompt: "warm, practical"},
			},
		},
	}

	handler := PersonaVoicesListHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(`{"include_prompts": true}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := parseToolResult(t, tr)

	voices, _ := m["voices"].([]any)
	first, _ := voices[0].(map[string]any)

	if first["voice_prompt"] != "warm, practical" {
		t.Errorf("voice_prompt = %v, want 'warm, practical'", first["voice_prompt"])
	}
}

func TestPersonaVoicesListHandler_NoDeps(t *testing.T) {
	t.Parallel()

	handler := PersonaVoicesListHandler(&ToolDeps{})

	tr, err := handler(t.Context(), json.RawMessage(`{}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}

	m := parseToolResult(t, tr)
	if m["code"] != "persona_voices_unavailable" {
		t.Errorf("code = %v, want persona_voices_unavailable", m["code"])
	}
}

// ---------------------------------------------------------------------------
// persona_voices.read
// ---------------------------------------------------------------------------

func TestPersonaVoicesReadHandler_ByID(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{
		PersonaVoices: &stubPersonaVoicesService{
			voice: PersonaVoiceItem{ID: "00000000-0000-0000-0000-0000000000a1", Name: "Expat Mentor", Slug: "expat-mentor", VoicePrompt: "warm, practical"},
		},
	}

	handler := PersonaVoicesReadHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(`{"id": "00000000-0000-0000-0000-0000000000a1"}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}

	m := parseToolResult(t, tr)
	voice, _ := m["voice"].(map[string]any)

	if voice["voice_prompt"] != "warm, practical" {
		t.Errorf("voice_prompt = %v, want 'warm, practical'", voice["voice_prompt"])
	}
}

func TestPersonaVoicesReadHandler_BySlug(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{
		PersonaVoices: &stubPersonaVoicesService{
			voice: PersonaVoiceItem{ID: "00000000-0000-0000-0000-0000000000a1", Slug: "expat-mentor"},
		},
	}

	handler := PersonaVoicesReadHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(`{"slug": "expat-mentor"}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}
}

func TestPersonaVoicesReadHandler_MissingIdentifier(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{PersonaVoices: &stubPersonaVoicesService{}}
	handler := PersonaVoicesReadHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(`{}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}

	m := parseToolResult(t, tr)
	if m["code"] != "missing_identifier" {
		t.Errorf("code = %v, want missing_identifier", m["code"])
	}
}

func TestPersonaVoicesReadHandler_NotFound(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{
		PersonaVoices: &stubPersonaVoicesService{err: errors.New("no rows")},
	}

	handler := PersonaVoicesReadHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(`{"slug": "nope"}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}

	m := parseToolResult(t, tr)
	if m["code"] != "persona_voice_not_found" {
		t.Errorf("code = %v, want persona_voice_not_found", m["code"])
	}
}

// ---------------------------------------------------------------------------
// personas.list
// ---------------------------------------------------------------------------

func TestPersonasListHandler_Success(t *testing.T) {
	t.Parallel()

	voiceName := "Expat Mentor"

	deps := &ToolDeps{
		PersonaCatalog: &stubPersonaCatalogService{
			total: 1,
			items: []AgentPersonaItem{
				{ID: "00000000-0000-0000-0000-0000000000b1", ProfileName: "Auntie Japa", BackingUsername: "auntie_japa", VoiceName: &voiceName, Status: "active", TotalActions: 12},
			},
		},
	}

	handler := PersonasListHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(`{"status": "active"}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}

	m := parseToolResult(t, tr)

	personas, _ := m["personas"].([]any)
	if len(personas) != 1 {
		t.Fatalf("expected 1 persona, got %d", len(personas))
	}

	first, _ := personas[0].(map[string]any)
	if first["profile_name"] != "Auntie Japa" {
		t.Errorf("profile_name = %v, want 'Auntie Japa'", first["profile_name"])
	}

	if first["voice_name"] != "Expat Mentor" {
		t.Errorf("voice_name = %v, want 'Expat Mentor'", first["voice_name"])
	}
}

func TestPersonasListHandler_NoDeps(t *testing.T) {
	t.Parallel()

	handler := PersonasListHandler(&ToolDeps{})

	tr, err := handler(t.Context(), json.RawMessage(`{}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}

	m := parseToolResult(t, tr)
	if m["code"] != "personas_unavailable" {
		t.Errorf("code = %v, want personas_unavailable", m["code"])
	}
}

// ---------------------------------------------------------------------------
// personas.read
// ---------------------------------------------------------------------------

func TestPersonasReadHandler_Success(t *testing.T) {
	t.Parallel()

	voicePrompt := "warm, practical"

	deps := &ToolDeps{
		PersonaCatalog: &stubPersonaCatalogService{
			persona: AgentPersonaItem{ID: "00000000-0000-0000-0000-0000000000b1", ProfileName: "Auntie Japa", VoicePrompt: &voicePrompt},
		},
	}

	handler := PersonasReadHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(`{"id": "00000000-0000-0000-0000-0000000000b1"}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}

	m := parseToolResult(t, tr)
	persona, _ := m["persona"].(map[string]any)

	if persona["voice_prompt"] != "warm, practical" {
		t.Errorf("voice_prompt = %v, want 'warm, practical'", persona["voice_prompt"])
	}
}

func TestPersonasReadHandler_InvalidID(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{PersonaCatalog: &stubPersonaCatalogService{}}
	handler := PersonasReadHandler(deps)

	tr, err := handler(t.Context(), json.RawMessage(`{"id": "not-a-uuid"}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}

	m := parseToolResult(t, tr)
	if m["code"] != "invalid_persona_id" {
		t.Errorf("code = %v, want invalid_persona_id", m["code"])
	}
}
