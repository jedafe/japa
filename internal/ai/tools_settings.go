// Package ai — tools_settings.go implements the system settings tool
// handlers for the AI Agent Executor extended catalog.
//
// Tools implemented here:
//
//   - settings.get — read one system_setting by dot-notation key
//   - settings.set — write one system_setting (HITL-queued)
//
// Safety rules (mirroring the §103 blocked-operations spirit — security
// settings modification is structurally unsafe for an AI):
//
//   - Uncatalogued keys are refused outright (fail-closed — an unknown key
//     could be sensitive, or a dead row from a typo).
//   - Sensitive keys can neither be read nor written through the agent.
//   - Deprecated reachability toggles are refused on set — they are managed
//     via feature_flags (conventions/feature-flag-naming.md).
//   - `security.*` keys are refused for both tools — the hardcoded
//     blocklist already bans a `settings.security.*` tool namespace; the
//     same boundary applies to the keys themselves.
//   - Values are validated against the catalog (type, Min/Max, Options)
//     through the same validateSettingValue the admin PUT uses.
//
// is_public on upsert is resolved from the catalog's Public flag — never
// caller-supplied — so the agent cannot quietly expose a setting on the
// public settings endpoint.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
// Ref: japaconnect-engineering/conventions/system-settings-catalog.md
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

// settingsKeyDenied reports whether a dot-notation key is off-limits to the
// agent settings tools. `security.*` mirrors the blocked "settings.security.*"
// tool-namespace prefix: those keys govern login/abuse controls and must not
// be reachable through the agent even when the caller holds
// system.settings.edit.
func settingsKeyDenied(key string) bool {
	return strings.HasPrefix(strings.ToLower(key), "security.")
}

// lookupSetting resolves catalog metadata for a key through the wired
// SettingsCatalog bridge. nil bridge → fail-closed unknown.
func lookupSetting(deps *ToolDeps, key string) AgentSettingInfo {
	if deps == nil || deps.SettingsCatalog == nil {
		return AgentSettingInfo{}
	}

	return deps.SettingsCatalog.LookupSetting(key)
}

// ---------------------------------------------------------------------------
// settings.get
// ---------------------------------------------------------------------------

// settingsGetArgs is the argument shape for settings.get.
type settingsGetArgs struct {
	Key string `json:"key"`
}

// SettingsGetHandler returns a ToolHandler that reads one system_setting.
// Sensitive and security.* keys are refused; unknown keys error with a
// not-catalogued message. The value is returned parsed (raw JSON) together
// with its catalog metadata. Required permission system.settings.view.
func SettingsGetHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.SettingsStore == nil {
			return errorToolResult("settings_unavailable", "settings store is not configured"), nil
		}

		var a settingsGetArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		key := strings.TrimSpace(a.Key)
		if key == "" {
			return errorToolResult("missing_key", "key is required"), nil
		}

		if settingsKeyDenied(key) {
			return errorToolResult("setting_denied", fmt.Sprintf("setting %q is not accessible through the agent", key)), nil
		}

		info := lookupSetting(deps, key)
		if !info.Known {
			return errorToolResult("unknown_setting", fmt.Sprintf("setting %q is not in the settings catalog", key)), nil
		}

		if info.Sensitive {
			return errorToolResult("setting_sensitive", fmt.Sprintf("setting %q is sensitive and cannot be read through the agent", key)), nil
		}

		raw, err := deps.SettingsStore.GetSetting(ctx, key)
		if err != nil {
			// Absent stored row → the catalog default applies at read time.
			return successToolResult(map[string]any{
				"key":         key,
				"type":        info.Type,
				"description": info.Description,
				"public":      info.Public,
				"deprecated":  info.Deprecated,
				"set":         false,
			}), nil
		}

		var parsed any
		if json.Unmarshal([]byte(raw), &parsed) != nil {
			parsed = raw // non-JSON stored value — return as-is
		}

		return successToolResult(map[string]any{
			"key":         key,
			"type":        info.Type,
			"description": info.Description,
			"public":      info.Public,
			"deprecated":  info.Deprecated,
			"set":         true,
			"value":       parsed,
		}), nil
	}
}

// ---------------------------------------------------------------------------
// settings.set
// ---------------------------------------------------------------------------

// settingsSetArgs is the argument shape for settings.set. Value is any JSON
// — validated against the catalog type before the write.
type settingsSetArgs struct {
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value"`
}

// SettingsSetHandler returns a ToolHandler that upserts one system_setting.
// Refused: unknown/sensitive/deprecated/security.* keys and values failing
// catalog type validation. is_public resolves from the catalog — the agent
// cannot expose a setting on the public endpoint. Required permission
// system.settings.edit; requires_hitl queues the write for review.
func SettingsSetHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.SettingsStore == nil {
			return errorToolResult("settings_unavailable", "settings store is not configured"), nil
		}

		var a settingsSetArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		key := strings.TrimSpace(a.Key)
		if key == "" {
			return errorToolResult("missing_key", "key is required"), nil
		}

		if len(a.Value) == 0 {
			return errorToolResult("missing_value", "value is required"), nil
		}

		if settingsKeyDenied(key) {
			return errorToolResult("setting_denied", fmt.Sprintf("setting %q is not writable through the agent", key)), nil
		}

		info := lookupSetting(deps, key)
		if !info.Known {
			return errorToolResult("unknown_setting", fmt.Sprintf("setting %q is not in the settings catalog", key)), nil
		}

		if info.Sensitive {
			return errorToolResult("setting_sensitive", fmt.Sprintf("setting %q is sensitive and cannot be written through the agent", key)), nil
		}

		if info.Deprecated {
			return errorToolResult("setting_deprecated", fmt.Sprintf("setting %q is deprecated — manage it via feature_flags", key)), nil
		}

		// Catalog validation expects the unquoted scalar form: strings
		// unquote; everything else (numbers, bools, objects) passes through
		// as its compact JSON literal.
		raw := string(a.Value)
		var s string
		if json.Unmarshal(a.Value, &s) == nil {
			raw = s
		}

		if deps.SettingsCatalog != nil {
			if err := deps.SettingsCatalog.ValidateSetting(key, raw); err != nil {
				return errorToolResult("invalid_value", err.Error()), nil
			}
		}

		if err := deps.SettingsStore.UpdateSettingsBatchWithPublic(ctx,
			map[string][]byte{key: a.Value},
			map[string]bool{key: info.Public},
		); err != nil {
			return errorToolResult("settings_set_failed", err.Error()), nil
		}

		return successToolResult(map[string]any{
			"key":   key,
			"set":   true,
			"value": json.RawMessage(a.Value),
		}), nil
	}
}
