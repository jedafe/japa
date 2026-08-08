// Package ai — humanize.go implements the AI text humanization feature
// (Section 20 extension).
//
// Humanization rewrites AI output to sound more natural and human-like by
// injecting style instructions into the LLM system context. It is:
//
//   - Gated globally by the `ai.humanize.enabled` system setting (admin
//     toggle, RBAC-gated via the `ai.humanize.manage` permission node).
//   - Overridable per request via the `humanize` / `humanize_level` payload
//     keys, so a user can opt in or out per generation even when the global
//     toggle is on.
//   - Applied at the provider layer so every generation surface (generate,
//     chat, assist/improve, assist/summarize, assist/translate, persona
//     activity, studio drafts) inherits it without per-handler changes.
//
// The injection is provider-agnostic: each provider calls ApplyHumanization
// to merge humanization instructions into its system/context string before
// building the vendor-specific request body.
package ai

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

// HumanizeLevel controls the intensity of humanization instructions.
type HumanizeLevel string

const (
	HumanizeOff    HumanizeLevel = ""
	HumanizeLow    HumanizeLevel = "low"
	HumanizeMedium HumanizeLevel = "medium"
	HumanizeHigh   HumanizeLevel = "high"
)

// humanizeInstructions maps each level to the system-prompt fragment injected
// to steer the model toward more natural output. Kept intentionally short to
// avoid inflating token usage on every request.
var humanizeInstructions = map[HumanizeLevel]string{
	HumanizeLow: "Write in a natural, conversational tone. Avoid overly formal or robotic phrasing. Prefer active voice.",
	HumanizeMedium: "Write as a real person would. Use contractions, varied sentence length, and a conversational tone. Avoid list-heavy, formulaic, or robotic patterns. Prefer active voice and natural transitions.",
	HumanizeHigh: "Write as if you are a real person speaking naturally. Use casual language, contractions, idioms, occasional first-person voice, and varied rhythm. Avoid any robotic, formulaic, or overly structured patterns. Sound human, not generated.",
}

// PayloadHumanize extracts the humanization request from a provider payload.
// Returns the level and whether humanization was explicitly requested.
// Payload keys:
//   - "humanize" (bool):   explicitly enable/disable for this request.
//   - "humanize_level" (string): "low" | "medium" | "high" (default "medium").
//
// When "humanize" is absent, the caller's global setting decides; this helper
// only reports what the payload says so the caller can AND it with the global.
func PayloadHumanize(payload map[string]any) (level HumanizeLevel, requested bool) {
	if payload == nil {
		return HumanizeOff, false
	}

	v, ok := payload["humanize"]
	if !ok {
		return HumanizeOff, false
	}

	enabled, _ := v.(bool)
	if !enabled {
		// Explicit opt-out: signal requested=true so the caller knows the
		// payload addressed humanization (and should NOT apply the global).
		return HumanizeOff, true
	}

	lvl := HumanizeMedium
	if s, ok := payload["humanize_level"].(string); ok {
		switch HumanizeLevel(s) {
		case HumanizeLow, HumanizeMedium, HumanizeHigh:
			lvl = HumanizeLevel(s)
		}
	}

	return lvl, true
}

// ResolveHumanize merges a global enable/level (read from system settings by
// the caller) with a per-request override. The handler writes the returned
// values into the provider payload as payload["humanize"] and
// payload["humanize_level"], where the provider layer picks them up.
//
//   - reqHumanize is nil when the request did not address humanization; the
//     global setting stands.
//   - reqLevel is "" when the request did not specify a level; the global
//     level (or "medium" default) stands.
//
// Keeping this pure (no config import) lets humanize.go stay unit-testable
// without a database or settings service.
func ResolveHumanize(globalEnabled bool, globalLevel HumanizeLevel, reqHumanize *bool, reqLevel string) (bool, HumanizeLevel) {
	enabled := globalEnabled
	level := globalLevel
	if level == HumanizeOff {
		level = HumanizeMedium
	}

	if reqHumanize != nil {
		enabled = *reqHumanize
	}

	if reqLevel != "" {
		switch HumanizeLevel(reqLevel) {
		case HumanizeLow, HumanizeMedium, HumanizeHigh:
			level = HumanizeLevel(reqLevel)
		}
	}

	return enabled, level
}

// resolveHandlerHumanize reads the global ai.humanize.* system settings and
// merges them with a per-request override, returning the values to write into
// a provider payload. When Settings is nil (feature not wired), humanization
// is off unless the request explicitly enables it.
//
// reqHumanize is nil when the request did not specify humanization.
func (h *Handlers) resolveHandlerHumanize(ctx context.Context, tenantID pgtype.UUID, reqHumanize *bool, reqLevel string) (bool, HumanizeLevel) {
	globalEnabled := false
	globalLevel := HumanizeMedium

	if h.Settings != nil {
		globalEnabled = h.Settings.GetBoolOr(ctx, tenantID, "ai.humanize.enabled", false)
		if lvl := h.Settings.GetStringOr(ctx, tenantID, "ai.humanize.level", "medium"); lvl != "" {
			globalLevel = HumanizeLevel(lvl)
		}
	}

	return ResolveHumanize(globalEnabled, globalLevel, reqHumanize, reqLevel)
}

// writeHumanizePayload sets the humanize/humanize_level keys on a provider
// payload so the provider layer applies the instructions. It always writes
// both keys (even when disabled) so PayloadHumanize sees an explicit request
// and does not fall back to any caller default.
func writeHumanizePayload(payload map[string]any, enabled bool, level HumanizeLevel) {
	payload["humanize"] = enabled
	payload["humanize_level"] = string(level)
}

// ApplyHumanization merges humanization instructions into a system/context
// string. It is a no-op when level is HumanizeOff.
//
// If existing is empty, the instruction becomes the entire system context.
// If existing already contains humanization guidance (idempotency guard), the
// instruction is not appended twice.
func ApplyHumanization(existing string, level HumanizeLevel) string {
	if level == HumanizeOff {
		return existing
	}

	instr, ok := humanizeInstructions[level]
	if !ok {
		return existing
	}

	// Idempotency: skip if the exact instruction is already present.
	if strings.Contains(existing, instr) {
		return existing
	}

	existing = strings.TrimSpace(existing)
	if existing == "" {
		return instr
	}

	return existing + "\n\n" + instr
}
