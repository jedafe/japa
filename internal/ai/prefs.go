package ai

import "encoding/json"

// AIPrefs is the user_preferences.ai_prefs JSONB shape (ADR-130). All fields
// are optional; the zero value means defaults — chat mode, no restrictions.
// Ref: japaconnect-engineering/adr/ADR-130-unified-japa-ai.md.
type AIPrefs struct {
	DefaultMode        string         `json:"default_mode,omitempty"` // "" | "chat" | "agent"
	DisabledCategories []string       `json:"disabled_categories,omitempty"`
	DisabledTools      []string       `json:"disabled_tools,omitempty"`
	EnabledTools       []string       `json:"enabled_tools,omitempty"` // per-tool exceptions vs disabled_categories
	ChatTemplates      []ChatTemplate `json:"chat_templates,omitempty"`
}

// ChatTemplate is a saved generation-spec preset for the /ai composer — a
// named, client-managed blob of structured drafting options (content_type,
// tone, framework, word/heading/link counts, include_* flags) that the web
// surface serializes into outgoing messages. The backend stores and returns
// it verbatim: Values is opaque so template fields can evolve without a
// schema change here.
type ChatTemplate struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Slash-separated grouping path (e.g. "visa/guides") — opaque to the
	// backend like Values; folders are derived client-side, not entities.
	Folder    string          `json:"folder,omitempty"`
	Values    json.RawMessage `json:"values,omitempty"`
	CreatedAt string          `json:"created_at,omitempty"`
	UpdatedAt string          `json:"updated_at,omitempty"`
}

// ParseAIPrefs decodes the ai_prefs column; malformed payloads degrade to
// defaults rather than erroring (a corrupt pref must not break chat).
func ParseAIPrefs(raw []byte) AIPrefs {
	var p AIPrefs
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &p)
	}

	return p
}

// AllowsTool reports whether the user's prefs permit a tool. Evaluation
// order: explicit per-tool disable wins; then a disabled category blocks
// unless the tool is explicitly re-enabled via enabled_tools.
func (p AIPrefs) AllowsTool(functionName, category string) bool {
	for _, t := range p.DisabledTools {
		if t == functionName {
			return false
		}
	}

	for _, c := range p.DisabledCategories {
		if c == category {
			for _, t := range p.EnabledTools {
				if t == functionName {
					return true // per-tool exception inside a disabled category
				}
			}

			return false
		}
	}

	return true
}

// ToolSelection is the per-message `tools` override on POST /ai/chat.
// Enabled is tri-state: nil = inherit prefs, false = force the turn to pure
// chat, true = tools on (still bounded by prefs/RBAC — an override can only
// narrow the allowed set, never widen it).
type ToolSelection struct {
	Enabled    *bool    `json:"enabled,omitempty"`
	Categories []string `json:"categories,omitempty"`
	Tools      []string `json:"tools,omitempty"`
}

// FilterCatalog applies user prefs then the per-message override to the
// RBAC-filtered catalog. Every layer can only narrow what the platform and
// RBAC already allowed.
func FilterCatalog(tools []ToolConfig, categories map[string]string, prefs AIPrefs, sel *ToolSelection) []ToolConfig {
	if sel != nil && sel.Enabled != nil && !*sel.Enabled {
		return nil
	}

	out := tools[:0:0]

	for _, tc := range tools {
		if !prefs.AllowsTool(tc.FunctionName, categories[tc.FunctionName]) {
			continue
		}

		if sel != nil {
			// A non-nil whitelist is authoritative: an explicitly empty
			// array narrows to nothing — "[]" is not the same as absent.
			if sel.Categories != nil && !sliceContains(sel.Categories, categories[tc.FunctionName]) {
				continue
			}

			if sel.Tools != nil && !sliceContains(sel.Tools, tc.FunctionName) {
				continue
			}
		}

		out = append(out, tc)
	}

	return out
}

func sliceContains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}

	return false
}
