package ai

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// stubSettingsStore implements SettingsStore for the settings tool tests.
type stubSettingsStore struct {
	values   map[string]string
	getErr   error
	pending  map[string][]byte
	isPublic map[string]bool
	setErr   error
}

func (s *stubSettingsStore) GetSetting(ctx context.Context, key string) (string, error) {
	if s.getErr != nil {
		return "", s.getErr
	}
	if v, ok := s.values[key]; ok {
		return v, nil
	}
	return "", errors.New("setting not found")
}

func (s *stubSettingsStore) UpdateSettingsBatchWithPublic(ctx context.Context, settings map[string][]byte, isPublic map[string]bool) error {
	if s.setErr != nil {
		return s.setErr
	}
	s.pending = settings
	s.isPublic = isPublic
	return nil
}

// stubSettingsCatalog implements SettingsCatalog keyed on a meta map.
type stubSettingsCatalog struct {
	meta         map[string]AgentSettingInfo
	validateErr  error
	validatedRaw string
}

func (s *stubSettingsCatalog) LookupSetting(key string) AgentSettingInfo {
	if m, ok := s.meta[key]; ok {
		return m
	}
	return AgentSettingInfo{}
}

func (s *stubSettingsCatalog) ValidateSetting(key, raw string) error {
	s.validatedRaw = raw
	return s.validateErr
}

func settingsDeps() *ToolDeps {
	return &ToolDeps{
		SettingsStore: &stubSettingsStore{
			values: map[string]string{"docs.categories": `[{"name":"Getting Started"}]`},
		},
		SettingsCatalog: &stubSettingsCatalog{
			meta: map[string]AgentSettingInfo{
				"docs.categories":           {Known: true, Type: "json", Public: true},
				"security.max_login_attempts": {Known: true, Type: "int"},
				"secret.key":                {Known: true, Type: "string", Sensitive: true},
				"old.flag":                  {Known: true, Type: "bool", Deprecated: true},
			},
		},
	}
}

func TestSettingsGetHandler_ReadsCataloguedKey(t *testing.T) {
	deps := settingsDeps()
	h := SettingsGetHandler(deps)

	res, err := h(t.Context(), mustJSON(t, map[string]any{"key": "docs.categories"}), pgtype.UUID{})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if res.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %s, want success", res.Status)
	}
}

func TestSettingsGetHandler_RefusesSecurityKey(t *testing.T) {
	deps := settingsDeps()
	h := SettingsGetHandler(deps)

	res, err := h(t.Context(), mustJSON(t, map[string]any{"key": "security.max_login_attempts"}), pgtype.UUID{})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if res.Status != ToolCallStatusError {
		t.Errorf("status = %s, want error", res.Status)
	}
}

func TestSettingsGetHandler_RefusesSensitiveKey(t *testing.T) {
	deps := settingsDeps()
	h := SettingsGetHandler(deps)

	res, err := h(t.Context(), mustJSON(t, map[string]any{"key": "secret.key"}), pgtype.UUID{})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if res.Status != ToolCallStatusError {
		t.Errorf("status = %s, want error", res.Status)
	}
}

func TestSettingsGetHandler_RefusesUnknownKey(t *testing.T) {
	deps := settingsDeps()
	h := SettingsGetHandler(deps)

	res, err := h(t.Context(), mustJSON(t, map[string]any{"key": "made.up.key"}), pgtype.UUID{})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if res.Status != ToolCallStatusError {
		t.Errorf("status = %s, want error", res.Status)
	}
}

func TestSettingsSetHandler_WritesWithCatalogPublic(t *testing.T) {
	deps := settingsDeps()
	store := deps.SettingsStore.(*stubSettingsStore)
	cat := deps.SettingsCatalog.(*stubSettingsCatalog)
	h := SettingsSetHandler(deps)

	res, err := h(t.Context(), mustJSON(t, map[string]any{
		"key":   "docs.categories",
		"value": []any{map[string]any{"name": "Getting Started", "subcategories": []any{}}},
	}), pgtype.UUID{})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if res.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %s, want success", res.Status)
	}
	if store.pending == nil {
		t.Fatal("no settings write captured")
	}
	if !store.isPublic["docs.categories"] {
		t.Error("is_public should come from the catalog (true)")
	}
	if cat.validatedRaw == "" {
		t.Error("expected catalog validation to run")
	}
}

func TestSettingsSetHandler_RefusesDeprecated(t *testing.T) {
	deps := settingsDeps()
	store := deps.SettingsStore.(*stubSettingsStore)
	h := SettingsSetHandler(deps)

	res, err := h(t.Context(), mustJSON(t, map[string]any{"key": "old.flag", "value": true}), pgtype.UUID{})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if res.Status != ToolCallStatusError {
		t.Errorf("status = %s, want error", res.Status)
	}
	if store.pending != nil {
		t.Error("deprecated keys must not be written")
	}
}

func TestSettingsSetHandler_RefusesSensitive(t *testing.T) {
	deps := settingsDeps()
	store := deps.SettingsStore.(*stubSettingsStore)
	h := SettingsSetHandler(deps)

	res, err := h(t.Context(), mustJSON(t, map[string]any{"key": "secret.key", "value": "x"}), pgtype.UUID{})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if res.Status != ToolCallStatusError {
		t.Errorf("status = %s, want error", res.Status)
	}
	if store.pending != nil {
		t.Error("sensitive keys must not be written")
	}
}

func TestSettingsSetHandler_ValidationFailure(t *testing.T) {
	deps := settingsDeps()
	deps.SettingsCatalog.(*stubSettingsCatalog).validateErr = errors.New("setting \"docs.categories\" must be a whole number")
	store := deps.SettingsStore.(*stubSettingsStore)
	h := SettingsSetHandler(deps)

	res, err := h(t.Context(), mustJSON(t, map[string]any{"key": "docs.categories", "value": 1.5}), pgtype.UUID{})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if res.Status != ToolCallStatusError {
		t.Errorf("status = %s, want error", res.Status)
	}
	if store.pending != nil {
		t.Error("values failing catalog validation must not be written")
	}
}
