package ai

import (
	"strings"
	"testing"
)

// TestApplyHumanization_OffLevelIsNoop verifies that HumanizeOff never mutates
// the system context, regardless of the input.
func TestApplyHumanization_OffLevelIsNoop(t *testing.T) {
	t.Parallel()

	cases := []string{"", "You are a helpful assistant.", "multi\nline\ncontext"}
	for _, in := range cases {
		got := ApplyHumanization(in, HumanizeOff)
		if got != in {
			t.Errorf("HumanizeOff should be a no-op; input=%q got=%q", in, got)
		}
	}
}

// TestApplyHumanization_AppendsInstruction verifies that each level appends
// its instruction fragment to an existing context, separated by a blank line.
func TestApplyHumanization_AppendsInstruction(t *testing.T) {
	t.Parallel()

	existing := "You are a helpful assistant."
	for _, level := range []HumanizeLevel{HumanizeLow, HumanizeMedium, HumanizeHigh} {
		got := ApplyHumanization(existing, level)
		if !strings.HasPrefix(got, existing) {
			t.Errorf("level %q: result should start with existing context; got=%q", level, got)
		}
		if !strings.Contains(got, humanizeInstructions[level]) {
			t.Errorf("level %q: result should contain the instruction fragment; got=%q", level, got)
		}
		if !strings.Contains(got, "\n\n") {
			t.Errorf("level %q: instruction should be separated by a blank line; got=%q", level, got)
		}
	}
}

// TestApplyHumanization_EmptyExistingBecomesInstruction verifies that when
// there is no existing context, the instruction becomes the entire context
// (no leading blank line).
func TestApplyHumanization_EmptyExistingBecomesInstruction(t *testing.T) {
	t.Parallel()

	got := ApplyHumanization("", HumanizeMedium)
	if got != humanizeInstructions[HumanizeMedium] {
		t.Errorf("empty context + medium should equal the instruction; got=%q", got)
	}
	if strings.HasPrefix(got, "\n") {
		t.Errorf("empty context should not produce a leading newline; got=%q", got)
	}
}

// TestApplyHumanization_Idempotent verifies that applying the same level twice
// does not duplicate the instruction.
func TestApplyHumanization_Idempotent(t *testing.T) {
	t.Parallel()

	once := ApplyHumanization("base context", HumanizeHigh)
	twice := ApplyHumanization(once, HumanizeHigh)
	if once != twice {
		t.Errorf("applying the same level twice should be idempotent;\n once=%q\ntwice=%q", once, twice)
	}
}

// TestPayloadHumanize_AbsentKey reports no request.
func TestPayloadHumanize_AbsentKey(t *testing.T) {
	t.Parallel()

	level, requested := PayloadHumanize(map[string]any{"prompt": "hi"})
	if requested {
		t.Errorf("absent humanize key should report requested=false")
	}
	if level != HumanizeOff {
		t.Errorf("absent humanize key should report level=Off; got=%q", level)
	}
}

// TestPayloadHumanize_NilPayload reports no request.
func TestPayloadHumanize_NilPayload(t *testing.T) {
	t.Parallel()

	level, requested := PayloadHumanize(nil)
	if requested || level != HumanizeOff {
		t.Errorf("nil payload should report requested=false, level=Off; got level=%q requested=%v", level, requested)
	}
}

// TestPayloadHumanize_ExplicitEnableDefaultsMedium verifies that enabling
// humanization without a level defaults to medium.
func TestPayloadHumanize_ExplicitEnableDefaultsMedium(t *testing.T) {
	t.Parallel()

	level, requested := PayloadHumanize(map[string]any{"humanize": true})
	if !requested {
		t.Errorf("humanize=true should report requested=true")
	}
	if level != HumanizeMedium {
		t.Errorf("humanize=true without level should default to medium; got=%q", level)
	}
}

// TestPayloadHumanize_ExplicitDisable verifies that humanize=false is an
// explicit opt-out (requested=true, level=Off).
func TestPayloadHumanize_ExplicitDisable(t *testing.T) {
	t.Parallel()

	level, requested := PayloadHumanize(map[string]any{"humanize": false})
	if !requested {
		t.Errorf("humanize=false should report requested=true (explicit opt-out)")
	}
	if level != HumanizeOff {
		t.Errorf("humanize=false should report level=Off; got=%q", level)
	}
}

// TestPayloadHumanize_LevelHonored verifies each valid level is honored.
func TestPayloadHumanize_LevelHonored(t *testing.T) {
	t.Parallel()

	for _, lvl := range []HumanizeLevel{HumanizeLow, HumanizeMedium, HumanizeHigh} {
		level, requested := PayloadHumanize(map[string]any{
			"humanize":       true,
			"humanize_level": string(lvl),
		})
		if !requested {
			t.Errorf("level %q: requested should be true", lvl)
		}
		if level != lvl {
			t.Errorf("level %q: got=%q", lvl, level)
		}
	}
}

// TestPayloadHumanize_InvalidLevelFallsBackToMedium verifies that an
// unrecognized level string falls back to medium rather than being rejected.
func TestPayloadHumanize_InvalidLevelFallsBackToMedium(t *testing.T) {
	t.Parallel()

	level, requested := PayloadHumanize(map[string]any{
		"humanize":       true,
		"humanize_level": "ultra",
	})
	if !requested {
		t.Errorf("invalid level should still report requested=true")
	}
	if level != HumanizeMedium {
		t.Errorf("invalid level should fall back to medium; got=%q", level)
	}
}

// TestResolveHumanize_GlobalOnly verifies that when no per-request override is
// supplied, the global setting stands.
func TestResolveHumanize_GlobalOnly(t *testing.T) {
	t.Parallel()

	enabled, level := ResolveHumanize(true, HumanizeHigh, nil, "")
	if !enabled || level != HumanizeHigh {
		t.Errorf("global on/high with no override should stay on/high; got enabled=%v level=%q", enabled, level)
	}

	enabled, level = ResolveHumanize(false, HumanizeMedium, nil, "")
	if enabled || level != HumanizeMedium {
		t.Errorf("global off with no override should stay off/medium; got enabled=%v level=%q", enabled, level)
	}
}

// TestResolveHumanize_RequestOverrideWins verifies that an explicit per-request
// override takes precedence over the global setting.
func TestResolveHumanize_RequestOverrideWins(t *testing.T) {
	t.Parallel()

	on := true
	off := false

	// Global off, request on → on.
	enabled, level := ResolveHumanize(false, HumanizeMedium, &on, "low")
	if !enabled || level != HumanizeLow {
		t.Errorf("request override should win; got enabled=%v level=%q", enabled, level)
	}

	// Global on, request off → off.
	enabled, _ = ResolveHumanize(true, HumanizeHigh, &off, "")
	if enabled {
		t.Errorf("request off should win over global on; got enabled=%v", enabled)
	}
}

// TestResolveHumanize_GlobalLevelOffDefaultsToMedium verifies that a global
// level of "" (unset) resolves to medium rather than producing an empty level.
func TestResolveHumanize_GlobalLevelOffDefaultsToMedium(t *testing.T) {
	t.Parallel()

	enabled, level := ResolveHumanize(true, HumanizeOff, nil, "")
	if !enabled {
		t.Errorf("global enabled should stay enabled")
	}
	if level != HumanizeMedium {
		t.Errorf("empty global level should default to medium; got=%q", level)
	}
}

// TestResolveHumanize_RequestLevelOnlyKeepsGlobalEnable verifies that a
// request which only specifies a level (no enable flag) keeps the global
// enable state but adopts the requested level.
func TestResolveHumanize_RequestLevelOnlyKeepsGlobalEnable(t *testing.T) {
	t.Parallel()

	enabled, level := ResolveHumanize(true, HumanizeMedium, nil, "high")
	if !enabled || level != HumanizeHigh {
		t.Errorf("level-only override should keep global enable and adopt level; got enabled=%v level=%q", enabled, level)
	}
}

// TestWriteHumanizePayload_AlwaysSetsBothKeys verifies the payload writer sets
// both keys so PayloadHumanize sees an explicit request.
func TestWriteHumanizePayload_AlwaysSetsBothKeys(t *testing.T) {
	t.Parallel()

	payload := map[string]any{}
	writeHumanizePayload(payload, true, HumanizeHigh)

	level, requested := PayloadHumanize(payload)
	if !requested || level != HumanizeHigh {
		t.Errorf("writeHumanizePayload(true, high) should round-trip; got requested=%v level=%q", requested, level)
	}

	payload2 := map[string]any{}
	writeHumanizePayload(payload2, false, HumanizeMedium)
	level, requested = PayloadHumanize(payload2)
	if !requested {
		t.Errorf("disabled payload should still be an explicit request; got requested=%v", requested)
	}
	if level != HumanizeOff {
		t.Errorf("disabled payload should resolve to Off; got=%q", level)
	}
}

// TestBuildOpenAIMessages_HumanizeInjectsSystem verifies the OpenAI provider
// message builder injects humanization into the system message.
func TestBuildOpenAIMessages_HumanizeInjectsSystem(t *testing.T) {
	t.Parallel()

	msgs := buildOpenAIMessages("hello", map[string]any{
		"context":        "You are a bot.",
		"humanize":       true,
		"humanize_level": "high",
	})
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages (system+user); got %d", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Errorf("first message should be system; got %q", msgs[0].Role)
	}
	if !strings.Contains(msgs[0].Content, "You are a bot.") {
		t.Errorf("system message should retain original context; got=%q", msgs[0].Content)
	}
	if !strings.Contains(msgs[0].Content, humanizeInstructions[HumanizeHigh]) {
		t.Errorf("system message should contain high humanization instruction; got=%q", msgs[0].Content)
	}
	if msgs[1].Role != "user" || msgs[1].Content != "hello" {
		t.Errorf("second message should be the user prompt; got role=%q content=%q", msgs[1].Role, msgs[1].Content)
	}
}

// TestBuildOpenAIMessages_HumanizeNoContextCreatesSystem verifies that
// humanization with no prior context still produces a system message.
func TestBuildOpenAIMessages_HumanizeNoContextCreatesSystem(t *testing.T) {
	t.Parallel()

	msgs := buildOpenAIMessages("hello", map[string]any{
		"humanize":       true,
		"humanize_level": "low",
	})
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages; got %d", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Errorf("first message should be system; got %q", msgs[0].Role)
	}
	if msgs[0].Content != humanizeInstructions[HumanizeLow] {
		t.Errorf("system message should be the low instruction; got=%q", msgs[0].Content)
	}
}

// TestBuildOpenAIMessages_NoHumanizeNoSystem verifies that without humanization
// and without context, no system message is emitted (preserves prior behavior).
func TestBuildOpenAIMessages_NoHumanizeNoSystem(t *testing.T) {
	t.Parallel()

	msgs := buildOpenAIMessages("hello", map[string]any{})
	if len(msgs) != 1 || msgs[0].Role != "user" {
		t.Errorf("without context or humanize, only the user message should exist; got %+v", msgs)
	}
}
