package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/features"
	"github.com/valkey-io/valkey-go"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// startFlagTestValkey spins up an in-memory Valkey and returns a
// go-redis-compatible client — the L2 layer the flag checker's jc:flags:_all
// aggregate lives on. Mirrors the helper in internal/features/eval_test.go.
func startFlagTestValkey(t *testing.T) valkeycompat.Cmdable {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}

	t.Cleanup(mr.Close)

	vk, err := valkey.NewClient(valkey.ClientOption{
		InitAddress:  []string{mr.Addr()},
		DisableCache: true,
	})
	if err != nil {
		t.Fatalf("valkey.NewClient: %v", err)
	}

	t.Cleanup(func() { vk.Close() })

	return valkeycompat.NewAdapter(vk)
}

// errExtensionNotFound is the error code for unknown flag keys.
const errExtensionNotFound = "extension_not_found"

// phaseTesting is the rollout phase exercised by the PATCH/merge tests.
const phaseTesting = "testing"

// seedFlagAggregate writes the jc:flags:_all JSON snapshot AllFlagRows reads,
// letting tests exercise the stored-row overlay without a live Postgres.
func seedFlagAggregate(t *testing.T, vk valkeycompat.Cmdable, rows []features.FlagRow) {
	t.Helper()

	raw, err := json.Marshal(rows)
	if err != nil {
		t.Fatalf("marshal flag rows: %v", err)
	}

	if err := vk.Set(t.Context(), "jc:flags:_all", string(raw), 0).Err(); err != nil {
		t.Fatalf("seed jc:flags:_all: %v", err)
	}
}

func extensionsRequest(t *testing.T, h *Handlers) (int, []extensionEntry) {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/control/extensions", nil)
	rec := httptest.NewRecorder()
	h.ListExtensions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}

	var env struct {
		Data []extensionEntry `json:"data"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	return rec.Code, env.Data
}

func extensionByKey(t *testing.T, entries []extensionEntry, key string) extensionEntry {
	t.Helper()

	for _, e := range entries {
		if e.Key == key {
			return e
		}
	}

	t.Fatalf("extension %q not in list (got %d entries)", key, len(entries))

	return extensionEntry{}
}

// callExtensionWrite invokes a keyed extension handler with a chi URL param
// and returns the response status + decoded body.
func callExtensionWrite(t *testing.T, fn http.HandlerFunc, method, key, body string) (int, map[string]any) {
	t.Helper()

	var reqBody *strings.Reader
	if body == "" {
		reqBody = strings.NewReader("")
	} else {
		reqBody = strings.NewReader(body)
	}

	req := httptest.NewRequest(method, "/control/extensions/"+key, reqBody)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("key", key)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	fn(rec, req)

	var decoded map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("unmarshal response (status %d): %v — %s", rec.Code, err, rec.Body.String())
	}

	return rec.Code, decoded
}

func errorCode(t *testing.T, body map[string]any) string {
	t.Helper()

	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("response has no error object: %v", body)
	}

	code, _ := errObj["code"].(string)

	return code
}

// TestListExtensions_RegistryUnion verifies the list covers every Registry
// key — not just the old 28-entry hardcoded catalog — and that registry
// defaults drive name/category/settings_category/enabled.
func TestListExtensions_RegistryUnion(t *testing.T) {
	t.Parallel()

	checker := features.New(nil, nil)
	t.Cleanup(checker.Close)

	h := &Handlers{FeatureChecker: checker}

	_, entries := extensionsRequest(t, h)

	if len(entries) != len(features.Registry) {
		t.Fatalf("got %d entries, want %d (one per Registry key)", len(entries), len(features.Registry))
	}

	// Keys that never appeared in the old hardcoded catalog must list now —
	// feature.jce.*, roadmap.enabled, and the SEO tools.
	for _, key := range []string{"feature.jce", "roadmap.enabled", "crs_calculator.enabled", "feature.dm"} {
		extensionByKey(t, entries, key)
	}

	blog := extensionByKey(t, entries, "feature.blog")

	if blog.Name != "Blog" || blog.Category != "Content" || blog.SettingsCategory != "content" {
		t.Fatalf("feature.blog entry = %+v, want registry metadata", blog)
	}

	if !blog.Enabled {
		t.Fatal("feature.blog should default to enabled")
	}

	if blog.RolloutPercentage != 100 || blog.Phase != extPhaseShipped {
		t.Fatalf("feature.blog rollout/phase = %d/%q, want 100/shipped", blog.RolloutPercentage, blog.Phase)
	}

	if blog.SettingKey != "feature.blog" {
		t.Fatalf("setting_key = %q, want the feature_flags key", blog.SettingKey)
	}

	if blog.LinkedRoadmapSlug != "" {
		t.Fatalf("linked_roadmap_slug = %q, want empty (no roadmap DB)", blog.LinkedRoadmapSlug)
	}

	// A disabled-by-default flag derives phase "planned".
	japabot := extensionByKey(t, entries, "feature.japabot")

	if japabot.Enabled || japabot.Phase != extPhasePlanned {
		t.Fatalf("feature.japabot enabled/phase = %v/%q, want false/planned", japabot.Enabled, japabot.Phase)
	}
}

// TestListExtensions_NoChecker verifies the list degrades to Registry
// defaults when the feature checker is not wired.
func TestListExtensions_NoChecker(t *testing.T) {
	t.Parallel()

	h := &Handlers{}

	code, entries := extensionsRequest(t, h)
	if code != http.StatusOK || len(entries) != len(features.Registry) {
		t.Fatalf("code=%d entries=%d, want 200/%d", code, len(entries), len(features.Registry))
	}
}

// TestListExtensions_StoredRowOverlay seeds the jc:flags:_all aggregate and
// verifies stored rows override registry defaults and that admin-created
// flags outside the Registry list under category "Custom".
func TestListExtensions_StoredRowOverlay(t *testing.T) {
	t.Parallel()

	vk := startFlagTestValkey(t)
	checker := features.New(nil, vk)
	t.Cleanup(checker.Close)

	blogCfg, err := json.Marshal(features.FlagConfig{
		Name:              "Custom Blog Label",
		Phase:             phaseTesting,
		ShowOnRoadmap:     true,
		RolloutPercentage: 40,
		Surfaces: map[string]features.SurfaceRule{
			"main": {State: "hidden", Audience: []string{"group:beta_testers"}, GeoDeny: []string{"RU"}},
		},
	})
	if err != nil {
		t.Fatalf("marshal cfg: %v", err)
	}

	customCfg, err := json.Marshal(features.FlagConfig{Description: "Ops-only toggle"})
	if err != nil {
		t.Fatalf("marshal cfg: %v", err)
	}

	seedFlagAggregate(t, vk, []features.FlagRow{
		{Key: "feature.blog", Enabled: false, Config: blogCfg},
		{Key: "ops.kill_switch", Enabled: true, Config: customCfg},
	})

	h := &Handlers{FeatureChecker: checker}

	_, entries := extensionsRequest(t, h)

	if len(entries) != len(features.Registry)+1 {
		t.Fatalf("got %d entries, want %d (Registry + 1 custom)", len(entries), len(features.Registry)+1)
	}

	assertStoredBlogEntry(t, extensionByKey(t, entries, "feature.blog"))
	assertCustomEntry(t, extensionByKey(t, entries, "ops.kill_switch"))
}

// assertStoredBlogEntry checks the seeded feature.blog row overlays its
// registry defaults: stored enabled state, config name, phase, roadmap opt-in,
// rollout %, and the full main-surface rule (audience + geo included — the
// control panel is privileged).
func assertStoredBlogEntry(t *testing.T, blog extensionEntry) {
	t.Helper()

	if blog.Enabled {
		t.Fatal("feature.blog stored row is disabled — list must show stored state, not registry default")
	}

	if blog.Name != "Custom Blog Label" {
		t.Fatalf("name = %q, want stored config name", blog.Name)
	}

	if blog.Phase != phaseTesting || !blog.ShowOnRoadmap || blog.RolloutPercentage != 40 {
		t.Fatalf("phase/showOnRoadmap/rollout = %q/%v/%d, want testing/true/40", blog.Phase, blog.ShowOnRoadmap, blog.RolloutPercentage)
	}

	rule, ok := blog.Surfaces["main"]

	if !ok || rule.State != "hidden" || len(rule.Audience) != 1 || rule.Audience[0] != "group:beta_testers" || len(rule.GeoDeny) != 1 {
		t.Fatalf("surfaces.main = %+v, want full rule incl. audience/geo", rule)
	}
}

// assertCustomEntry checks a flag with no Registry entry lists under the
// "Custom" category with its stored enabled state and config metadata.
func assertCustomEntry(t *testing.T, custom extensionEntry) {
	t.Helper()

	if custom.Category != "Custom" {
		t.Fatalf("custom flag category = %q, want Custom", custom.Category)
	}

	if !custom.Enabled || custom.Description != "Ops-only toggle" {
		t.Fatalf("custom flag = %+v, want enabled + stored description", custom)
	}

	if custom.Phase != extPhaseShipped {
		t.Fatalf("custom flag phase = %q, want shipped (enabled, no explicit phase)", custom.Phase)
	}
}

// TestUpdateExtension_Validation covers the new PATCH fields: invalid phase,
// empty surface state, and unknown surface all map to 400 with the
// ConfigError's machine-readable wire code via ValidateConfig.
func TestUpdateExtension_Validation(t *testing.T) {
	t.Parallel()

	checker := features.New(nil, nil)
	t.Cleanup(checker.Close)

	h := &Handlers{FeatureChecker: checker}

	tests := []struct {
		name     string
		body     string
		wantCode string
	}{
		{"bogus phase", `{"phase":"bogus"}`, "invalid_phase"},
		{"empty surface state", `{"surfaces":{"main":{"state":""}}}`, "invalid_surface_state"},
		{"unknown surface", `{"surfaces":{"nope":{"state":"visible"}}}`, "invalid_surface"},
		{"bad audience entry", `{"surfaces":{"main":{"state":"visible","audience":["admins"]}}}`, "invalid_audience"},
		{"lowercase geo code", `{"surfaces":{"main":{"state":"visible","geo_allow":["ng"]}}}`, "invalid_geo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			code, body := callExtensionWrite(t, h.UpdateExtension, http.MethodPatch, "feature.blog", tt.body)

			if code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body: %v)", code, body)
			}

			if c := errorCode(t, body); c != tt.wantCode {
				t.Fatalf("error code = %q, want %q", c, tt.wantCode)
			}
		})
	}
}

// TestUpdateExtension_UnknownKey verifies a flag in neither the Registry nor
// feature_flags is rejected with 404 extension_not_found.
func TestUpdateExtension_UnknownKey(t *testing.T) {
	t.Parallel()

	checker := features.New(nil, nil)
	t.Cleanup(checker.Close)

	h := &Handlers{FeatureChecker: checker}

	code, body := callExtensionWrite(t, h.UpdateExtension, http.MethodPatch, "does.not.exist", `{"enabled":true}`)
	if code != http.StatusNotFound || errorCode(t, body) != errExtensionNotFound {
		t.Fatalf("status=%d code=%q, want 404 extension_not_found", code, errorCode(t, body))
	}
}

// deadFlagPool returns a *db.DB whose pools point at a refused address —
// queries fail fast with a connect error (not pgx.ErrNoRows), simulating a
// transient read failure on the feature_flags table.
func deadFlagPool(t *testing.T) *db.DB {
	t.Helper()

	// pgxpool.New is lazy — the pool is returned without connecting and
	// queries fail fast at Acquire time (127.0.0.1:1 refuses instantly).
	pool, err := pgxpool.New(t.Context(), "postgres://127.0.0.1:1/postgres")
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}

	t.Cleanup(pool.Close)

	return &db.DB{Reader: pool, Writer: pool}
}

// TestUpdateExtension_ReadErrorNotNotFound verifies a transient feature_flags
// read failure is NOT conflated with "no row": a custom key must get 500
// (previously it silently returned 404 extension_not_found), and a registry
// key must fail before the merge rather than overwriting a stored config
// that was never read.
func TestUpdateExtension_ReadErrorNotNotFound(t *testing.T) {
	t.Parallel()

	checker := features.New(nil, nil)
	t.Cleanup(checker.Close)

	h := &Handlers{DB: deadFlagPool(t), FeatureChecker: checker}

	// Custom (non-registry) key: read error must surface as 500, not 404.
	code, body := callExtensionWrite(t, h.UpdateExtension, http.MethodPatch, "ops.kill_switch", `{"enabled":true}`)

	if code != http.StatusInternalServerError {
		t.Fatalf("custom key on read error: status=%d, want 500 (body: %v)", code, body)
	}

	if c := errorCode(t, body); c != "internal_error" {
		t.Fatalf("error code = %q, want internal_error", c)
	}

	// Registry key: same — the merge must not run against an empty config.
	code, _ = callExtensionWrite(t, h.UpdateExtension, http.MethodPatch, "feature.blog", `{"phase":"testing"}`)
	if code != http.StatusInternalServerError {
		t.Fatalf("registry key on read error: status=%d, want 500", code)
	}

	// Enable/disable and rollout share the same read precondition.
	code, _ = callExtensionWrite(t, h.EnableExtension, http.MethodPost, "feature.blog", "")
	if code != http.StatusInternalServerError {
		t.Fatalf("enable on read error: status=%d, want 500", code)
	}

	code, _ = callExtensionWrite(t, h.UpdateExtensionRollout, http.MethodPatch, "feature.blog", `{"rollout_percentage":50}`)
	if code != http.StatusInternalServerError {
		t.Fatalf("rollout on read error: status=%d, want 500", code)
	}
}

// TestExtensionWriteEndpoints_UnknownKey covers the enable/disable/rollout
// endpoints — unknown keys (not in Registry, no stored row) → 404.
func TestExtensionWriteEndpoints_UnknownKey(t *testing.T) {
	t.Parallel()

	checker := features.New(nil, nil)
	t.Cleanup(checker.Close)

	h := &Handlers{FeatureChecker: checker}

	code, body := callExtensionWrite(t, h.EnableExtension, http.MethodPost, "ghost.flag", "")
	if code != http.StatusNotFound || errorCode(t, body) != errExtensionNotFound {
		t.Fatalf("enable: status=%d code=%q, want 404 extension_not_found", code, errorCode(t, body))
	}

	code, body = callExtensionWrite(t, h.DisableExtension, http.MethodPost, "ghost.flag", "")
	if code != http.StatusNotFound || errorCode(t, body) != errExtensionNotFound {
		t.Fatalf("disable: status=%d code=%q, want 404 extension_not_found", code, errorCode(t, body))
	}

	code, body = callExtensionWrite(t, h.UpdateExtensionRollout, http.MethodPatch, "ghost.flag", `{"rollout_percentage":50}`)
	if code != http.StatusNotFound || errorCode(t, body) != errExtensionNotFound {
		t.Fatalf("rollout: status=%d code=%q, want 404 extension_not_found", code, errorCode(t, body))
	}
}

// TestUpdateExtension_InvalidBody verifies malformed JSON → 400 invalid_body.
func TestUpdateExtension_InvalidBody(t *testing.T) {
	t.Parallel()

	checker := features.New(nil, nil)
	t.Cleanup(checker.Close)

	h := &Handlers{FeatureChecker: checker}

	code, body := callExtensionWrite(t, h.UpdateExtension, http.MethodPatch, "feature.blog", `{not json`)
	if code != http.StatusBadRequest || errorCode(t, body) != "invalid_body" {
		t.Fatalf("status=%d code=%q, want 400 invalid_body", code, errorCode(t, body))
	}
}

// TestExtensionPatchFields verifies the PATCH-to-jsonb translation: only
// fields present in the body appear in the patch (absent fields keep the
// stored value inside PatchFlag's `config || patch` merge); surfaces replaces
// the map; rollout clamps to 0-100; wire snake_case maps to stored camelCase.
func TestExtensionPatchFields(t *testing.T) {
	t.Parallel()

	// Only phase present — the patch carries just that stored-field key and
	// the validation cfg carries only the provided value (stored fields are
	// untouched inside the upsert's jsonb merge).
	phase := phaseTesting
	patch, cfg := extensionPatchFields(&extensionPatchInput{Phase: &phase})

	if cfg.Phase != phaseTesting {
		t.Fatalf("phase = %q, want testing", cfg.Phase)
	}

	patchKeys := func(raw []byte) map[string]json.RawMessage {
		t.Helper()

		m := map[string]json.RawMessage{}
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("patch unmarshal: %v", err)
		}

		return m
	}

	if patchMap := patchKeys(patch); len(patchMap) != 1 || string(patchMap["phase"]) != `"testing"` {
		t.Fatalf("patch = %s, want only {phase: testing}", patch)
	}

	// Surfaces present → whole-map replace under the stored key.
	newSurfaces := map[string]features.SurfaceRule{"tools": {State: "disabled"}}
	patch, cfg = extensionPatchFields(&extensionPatchInput{Surfaces: &newSurfaces})

	if cfg.Surfaces["tools"].State != "disabled" {
		t.Fatalf("surfaces cfg = %+v, want disabled", cfg.Surfaces)
	}

	if patchMap := patchKeys(patch); len(patchMap) != 1 || patchMap["surfaces"] == nil {
		t.Fatalf("patch = %s, want only {surfaces: …}", patch)
	}

	// Rollout clamps both directions.
	over := 150
	_, cfg = extensionPatchFields(&extensionPatchInput{RolloutPercentage: &over})

	if cfg.RolloutPercentage != 100 {
		t.Fatalf("rollout = %d, want 100 (clamped)", cfg.RolloutPercentage)
	}

	under := -5
	_, cfg = extensionPatchFields(&extensionPatchInput{RolloutPercentage: &under})

	if cfg.RolloutPercentage != 0 {
		t.Fatalf("rollout = %d, want 0 (clamped)", cfg.RolloutPercentage)
	}

	// Names are trimmed; show_on_roadmap maps to the stored camelCase key.
	name := "  Blog 2.0  "
	show := true
	patch, cfg = extensionPatchFields(&extensionPatchInput{Name: &name, ShowOnRoadmap: &show})

	if cfg.Name != "Blog 2.0" {
		t.Fatalf("name = %q, want trimmed", cfg.Name)
	}

	if patchMap := patchKeys(patch); string(patchMap["name"]) != `"Blog 2.0"` || string(patchMap["showOnRoadmap"]) != `true` {
		t.Fatalf("patch = %s, want {name: Blog 2.0, showOnRoadmap: true}", patch)
	}
}

// TestExtensionPhase verifies the derived phase: explicit config phase wins,
// otherwise enabled → shipped / disabled → planned.
func TestExtensionPhase(t *testing.T) {
	t.Parallel()

	if got := extensionPhase(true, features.FlagConfig{}); got != extPhaseShipped {
		t.Fatalf("enabled+no phase = %q, want shipped", got)
	}

	if got := extensionPhase(false, features.FlagConfig{}); got != extPhasePlanned {
		t.Fatalf("disabled+no phase = %q, want planned", got)
	}

	if got := extensionPhase(false, features.FlagConfig{Phase: phaseTesting}); got != phaseTesting {
		t.Fatalf("explicit phase = %q, want testing", got)
	}
}
