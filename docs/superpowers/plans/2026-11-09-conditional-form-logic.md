# Conditional Form Logic (visible_if + goto jumps) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let any §06 form field/step appear conditionally on earlier answers, and let choices jump forward over steps — with identical evaluation semantics in Go, TypeScript, and Dart, enforced server-side at validation and scoring.

**Architecture:** One declarative `Condition` grammar (`all`/`any` clause groups over the answers map) stored in existing JSONB config (`custom_fields.config.visible_if` / `.jump`, `form_steps.config.visible_if`, `forms.scoring_config.step_rules`). A new leaf package `internal/formlogic` (Go) is ported 1:1 to `packages/formlogic` (TS) and `japaconnect_core` (Dart). The server re-evaluates rules at submit (generic forms + assessment) and at score time; hidden answers are dropped and never scored. Full spec: `docs/superpowers/specs/2026-11-09-conditional-form-logic-design.md`.

**Tech Stack:** Go 1.x + sqlc/pgx + golang-migrate · SvelteKit 2 + Svelte 5 runes + Tailwind v4 · Bun workspaces (`bun test`) · Flutter/Riverpod (`flutter test`).

## Global Constraints

- **Bun only** for all JS/TS — never npm/npx/pnpm (repo rule 18). `bun install` runs **at repo root only**; never inside `apps/*`/`packages/*`.
- **Never hand-edit `internal/db/gen/`** — edit `internal/db/queries/*.sql`, then `sqlc generate` (Makefile target `make sqlc-gen`).
- **JEOS before code** — Task 1 updates the section files + ADR before any implementation task.
- **Commit to `main`** after every task; commit style `type(scope): msg` (see `git log --oneline`). **No AI attribution** in commits or code comments (repo rule 17 — no "Generated with", no Co-Authored-By trailers).
- Rule semantics (verbatim from spec — every evaluator MUST match):
  - `Condition` = `{all: [Clause], any: [Clause]}`; both present → `AND(∀all, ∃any)`; neither → true; absent/malformed rule → treated as absent (fail-open visible).
  - Ops: `eq|neq|in|contains|answered|not_answered`. `value` required except `answered`/`not_answered`.
  - "Answered" = present, non-nil, non-empty-string (trimmed), non-empty-array; numbers/booleans (incl. 0/false) count.
  - Unanswered reference → every op false except `not_answered`. `neq` = answered AND different.
  - `eq` normalizes bools (`true`/`"true"`/`1`/`yes`/`on`) and numerics; `in` = answer ∈ value array; `contains` = answer array ∋ value (non-array answer → false).
  - Clause `value`s compare against the **canonical option value**: server-side eval canonicalizes select/multiselect answers label→value via `NormalizeChoiceValue` before comparing (embed clients submit labels; assessment clients submit values). Admin-authored options have `value == label` (handler maps `options []string` → `{value:o,label:o}`), so this is a no-op for every field the admin UI creates — it only matters for seeded fields with distinct pairs, and keeps server/client eval identical.
  - `jump.when` clause with omitted `field` → defaults to the owning field's key.
  - Jumps fire only from fields whose own `visible_if` passes; target must be strictly later in natural step order; union of hidden intervals; target's own visibility still applies.
  - Step auto-hides when it has ≥1 defined field but zero visible fields.
  - Submissions store the canonical snapshot: hidden fields' answers are dropped server-side at validation; they never reach scoring.

---

### Task 1: JEOS section updates + ADR-137

**Files:**
- Modify: `.jeos/sections/06-custom-field-builder-form-builder.md`
- Modify: `.jeos/sections/71-readiness-assessment.md`
- Create: `.jeos/adr/ADR-137-conditional-form-logic.md`

- [ ] **Step 1: Extend §06 with the conditional-logic contract**

Append a "Conditional visibility and jumps (ADR-137)" section to `06-custom-field-builder-form-builder.md` documenting:

```markdown
## Conditional visibility and jumps (ADR-137)

- `custom_fields.config.visible_if` — `Condition {all?, any?}` of
  `Clause {field?, op, value?}` over the submitter's answers. Ops:
  `eq|neq|in|contains|answered|not_answered`. Absent/malformed → visible.
  Unanswered referenced field → clause false (except `not_answered`).
- `custom_fields.config.jump` — `{when: Condition, step: "<step id|dimension>"}`.
  Forward-only; hides the open interval of steps between the firing field's
  step and the target; union across all triggered jumps; fires only from
  visible fields.
- `form_steps.config` (jsonb, added by migration `001099_form_steps_config`) —
  carries `visible_if` for standard multi-step steps.
- `forms.scoring_config.step_rules.<dimension>.visible_if` — step rules for
  assessment forms (steps are dimension-derived; no form_steps rows exist).
- A step is auto-hidden when it has ≥1 defined field but zero visible fields.
- Server re-evaluates at submit: hidden fields are not required, not
  option-validated, and their submitted values are dropped before storage
  and scoring.
- Clause values compare against canonical option *values*: submitted
  select/multiselect labels are normalized via `NormalizeChoiceValue`
  before rule evaluation (admin-authored options already have
  `value == label`; the assessment client submits values).
```

- [ ] **Step 2: Extend §71**

Append to `71-readiness-assessment.md` under a "Conditional steps" heading:

```markdown
## Conditional steps (ADR-137)

- `scoring_config.step_rules` maps a dimension name to `{visible_if: Condition}`.
- Skipped (hidden) fields are excluded from scoring entirely — both numerator
  and denominator — matching the existing unanswered-question behavior of
  `computeScore`.
- `jump` rules on field config allow choice-driven forward skips between
  dimension steps.
```

- [ ] **Step 3: Write ADR-137**

Create `.jeos/adr/ADR-137-conditional-form-logic.md` in the house ADR style
(Status/Date/Drivers/References/Context/Decision/Consequences — see
`ADR-071-japa-readiness-assessment.md` for format). Decision summary:
declarative `visible_if` + forward-only `jump` on existing JSONB config over a
flow-graph DSL; shared tri-language evaluator (Go `internal/formlogic`, TS
`packages/formlogic`, Dart `japaconnect_core`); union-of-intervals jump
semantics; canonical stored submissions; new field-config write path
(`fieldRequest.config`, `UpdateCustomField`/`CreateCustomField` config param).

- [ ] **Step 4: Commit**

```bash
git add .jeos/sections/06-custom-field-builder-form-builder.md .jeos/sections/71-readiness-assessment.md .jeos/adr/ADR-137-conditional-form-logic.md
git commit -m "docs(jeos): §06/§71 conditional form logic contract + ADR-137"
```

---

### Task 2: Go evaluator — `internal/formlogic`

**Files:**
- Create: `internal/formlogic/logic.go`
- Test: `internal/formlogic/logic_test.go`

**Interfaces:**
- Produces (consumed by Tasks 3–5):
  ```go
  type Clause    struct{ Field, Op string; Value any }
  type Condition struct{ All, Any []Clause }
  type JumpRule  struct{ When Condition; Step string }
  type FieldRules struct{ VisibleIf *Condition; Jump *JumpRule }
  type RuleField struct{ Key, StepKey string; Config []byte }
  func ParseFieldRules(config []byte) FieldRules
  func (c *Condition) Eval(answers map[string]any, selfKey string) bool
  func FieldVisible(config []byte, answers map[string]any, selfKey string) bool
  func StepVisible(config []byte, answers map[string]any) bool
  func HiddenByJump(fields []RuleField, stepOrder []string, answers map[string]any) map[string]bool
  ```

- [ ] **Step 1: Write the failing test**

`internal/formlogic/logic_test.go` — keep this vector list; the TS and Dart
suites (Tasks 7, 11) mirror the same cases:

```go
package formlogic

import "testing"

func TestEvalClauseOps(t *testing.T) {
	answers := map[string]any{
		"purpose":  "study",
		"pets":     []any{"dog", "cat"},
		"score":    80.0,
		"flag":     true,
		"empty":    "",
		"emptyarr": []any{},
	}
	cases := []struct {
		name string
		cl   Clause
		want bool
	}{
		{"eq hit", Clause{Field: "purpose", Op: "eq", Value: "study"}, true},
		{"eq miss", Clause{Field: "purpose", Op: "eq", Value: "work"}, false},
		{"eq numeric", Clause{Field: "score", Op: "eq", Value: "80"}, true},
		{"eq bool", Clause{Field: "flag", Op: "eq", Value: "true"}, true},
		{"neq answered diff", Clause{Field: "purpose", Op: "neq", Value: "work"}, true},
		{"neq answered same", Clause{Field: "purpose", Op: "neq", Value: "study"}, false},
		{"neq unanswered", Clause{Field: "nope", Op: "neq", Value: "x"}, false},
		{"in hit", Clause{Field: "purpose", Op: "in", Value: []any{"work", "study"}}, true},
		{"in miss", Clause{Field: "purpose", Op: "in", Value: []any{"work"}}, false},
		{"contains hit", Clause{Field: "pets", Op: "contains", Value: "dog"}, true},
		{"contains miss", Clause{Field: "pets", Op: "contains", Value: "fish"}, false},
		{"contains nonarray", Clause{Field: "purpose", Op: "contains", Value: "x"}, false},
		{"answered yes", Clause{Field: "purpose", Op: "answered"}, true},
		{"answered empty", Clause{Field: "empty", Op: "answered"}, false},
		{"answered emptyarr", Clause{Field: "emptyarr", Op: "answered"}, false},
		{"answered missing", Clause{Field: "nope", Op: "answered"}, false},
		{"not_answered missing", Clause{Field: "nope", Op: "not_answered"}, true},
		{"not_answered present", Clause{Field: "purpose", Op: "not_answered"}, false},
		{"self default", Clause{Op: "eq", Value: "study"}, true}, // selfKey "purpose"
	}
	for _, c := range cases {
		selfKey := ""
		if c.name == "self default" {
			selfKey = "purpose"
		}
		if got := evalClause(c.cl, answers, selfKey); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestConditionAllAny(t *testing.T) {
	answers := map[string]any{"a": "x", "b": "y"}
	all := &Condition{All: []Clause{{Field: "a", Op: "eq", Value: "x"}, {Field: "b", Op: "eq", Value: "y"}}}
	if !all.Eval(answers, "") {
		t.Fatal("all should pass")
	}
	bad := &Condition{All: []Clause{{Field: "a", Op: "eq", Value: "x"}}, Any: []Clause{{Field: "b", Op: "eq", Value: "z"}}}
	if bad.Eval(answers, "") {
		t.Fatal("all&&any: any-group failed should be false")
	}
	var nilc *Condition
	if !nilc.Eval(answers, "") {
		t.Fatal("nil condition should be true")
	}
}

func TestFieldVisible(t *testing.T) {
	if !FieldVisible(nil, map[string]any{}, "k") {
		t.Fatal("no config → visible")
	}
	if !FieldVisible([]byte(`{bad json`), map[string]any{}, "k") {
		t.Fatal("malformed → fail-open visible")
	}
	cfg := []byte(`{"visible_if":{"all":[{"field":"a","op":"eq","value":1}]}}`)
	if FieldVisible(cfg, map[string]any{"a": "nope"}, "k") {
		t.Fatal("failing rule should hide")
	}
	if !FieldVisible(cfg, map[string]any{"a": 1.0}, "k") {
		t.Fatal("passing rule should show")
	}
}

func TestHiddenByJump(t *testing.T) {
	mk := func(key, step string, cfg string) RuleField {
		return RuleField{Key: key, StepKey: step, Config: []byte(cfg)}
	}
	order := []string{"s1", "s2", "s3", "s4", "s5"}

	// Basic forward jump: s1 field jumps to s4 → s2,s3 hidden.
	fields := []RuleField{
		mk("f1", "s1", `{"jump":{"when":{"any":[{"op":"eq","value":"go"}]},"step":"s4"}}`),
		mk("f2", "s2", `{}`),
	}
	h := HiddenByJump(fields, order, map[string]any{"f1": "go"})
	if !h["s2"] || !h["s3"] || h["s4"] || len(h) != 2 {
		t.Fatalf("expected {s2,s3}, got %v", h)
	}

	// No trigger → nothing hidden.
	h = HiddenByJump(fields, order, map[string]any{"f1": "stop"})
	if len(h) != 0 {
		t.Fatalf("untriggered jump hid %v", h)
	}

	// Backward/self/unknown targets ignored.
	bad := []RuleField{
		mk("f3", "s3", `{"jump":{"when":{"any":[{"op":"eq","value":1}]},"step":"s1"}}`),
		mk("f4", "s3", `{"jump":{"when":{"any":[{"op":"eq","value":1}]},"step":"s3"}}`),
		mk("f5", "s3", `{"jump":{"when":{"any":[{"op":"eq","value":1}]},"step":"ghost"}}`),
	}
	h = HiddenByJump(bad, order, map[string]any{"f3": 1.0, "f4": 1.0, "f5": 1.0})
	if len(h) != 0 {
		t.Fatalf("invalid targets hid %v", h)
	}

	// Hidden firing field cannot jump.
	hid := []RuleField{
		mk("f6", "s1", `{"visible_if":{"all":[{"field":"gate","op":"eq","value":0}]},"jump":{"when":{"any":[{"op":"eq","value":1}]},"step":"s3"}}`),
	}
	h = HiddenByJump(hid, order, map[string]any{"f6": 1.0})
	if len(h) != 0 {
		t.Fatalf("hidden field fired jump: %v", h)
	}

	// Union of intervals + chain: s1→s4 hides 2-3; s2 field jumps 5 → union {s2,s3,s4}.
	fields = []RuleField{
		mk("j1", "s1", `{"jump":{"when":{"any":[{"op":"eq","value":"a"}]},"step":"s4"}}`),
		mk("j2", "s2", `{"jump":{"when":{"any":[{"op":"eq","value":"b"}]},"step":"s5"}}`),
	}
	h = HiddenByJump(fields, order, map[string]any{"j1": "a", "j2": "b"})
	if len(h) != 3 || !h["s2"] || !h["s3"] || !h["s4"] {
		t.Fatalf("union expected {s2,s3,s4}, got %v", h)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/formlogic/ -v
```
Expected: FAIL — `evalClause`/`FieldVisible`/`HiddenByJump` undefined.

- [ ] **Step 3: Implement `internal/formlogic/logic.go`**

```go
// Package formlogic evaluates §06 conditional form rules: per-field
// visible_if conditions, per-step visible_if conditions, and forward-only
// jump rules. Semantics are mirrored 1:1 in packages/formlogic (TS) and
// packages/japaconnect_core (Dart) — keep all three in sync.
// Ref: japaconnect-engineering/sections/06-custom-field-builder-form-builder.md
// Ref: docs/superpowers/specs/2026-11-09-conditional-form-logic-design.md
package formlogic

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Clause is one predicate over the answers map. Field may be omitted inside
// jump.when — it then defaults to the owning field's key (selfKey).
type Clause struct {
	Field string `json:"field,omitempty"`
	Op    string `json:"op"`
	Value any    `json:"value,omitempty"`
}

// Condition groups clauses: AND(all) && (len(any)==0 || OR(any)).
// A nil/empty Condition is true.
type Condition struct {
	All []Clause `json:"all,omitempty"`
	Any []Clause `json:"any,omitempty"`
}

// JumpRule hides the open step interval between the firing field's step and
// Step when When holds. Forward-only, union across triggered jumps.
type JumpRule struct {
	When Condition `json:"when"`
	Step string    `json:"step"`
}

// FieldRules is the logic slice of custom_fields.config.
type FieldRules struct {
	VisibleIf *Condition `json:"visible_if,omitempty"`
	Jump      *JumpRule  `json:"jump,omitempty"`
}

// StepRules is the logic slice of form_steps.config / step_rules entries.
type StepRules struct {
	VisibleIf *Condition `json:"visible_if,omitempty"`
}

// RuleField is the minimal field shape the evaluator needs — callers map
// their field type (gen.CustomField, handler DTOs) into it.
type RuleField struct {
	Key     string
	StepKey string // containing step's identifier; "" = ungrouped
	Config  []byte // raw custom_fields.config JSONB
}

func ParseFieldRules(config []byte) FieldRules {
	var r FieldRules
	if len(config) > 0 {
		// Malformed → zero value (fail-open: field visible, no jump).
		_ = json.Unmarshal(config, &r)
	}
	return &r
}

// Eval reports whether the condition holds for answers. selfKey supplies the
// default clause field inside jump.when.
func (c *Condition) Eval(answers map[string]any, selfKey string) bool {
	if c == nil {
		return true
	}
	for _, cl := range c.All {
		if !evalClause(cl, answers, selfKey) {
			return false
		}
	}
	if len(c.Any) > 0 {
		for _, cl := range c.Any {
			if evalClause(cl, answers, selfKey) {
				return true
			}
		}
		return false
	}
	return true
}

func isAnswered(v any, ok bool) bool {
	if !ok || v == nil {
		return false
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t) != ""
	case []any:
		return len(t) > 0
	}
	return true
}

func evalClause(cl Clause, answers map[string]any, selfKey string) bool {
	key := cl.Field
	if key == "" {
		key = selfKey
	}
	v, ok := answers[key]
	has := isAnswered(v, ok)

	switch cl.Op {
	case "answered":
		return has
	case "not_answered":
		return !has
	}
	if !has {
		return false
	}

	switch cl.Op {
	case "eq":
		return scalarEqual(v, cl.Value)
	case "neq":
		return !scalarEqual(v, cl.Value)
	case "in":
		arr, _ := cl.Value.([]any)
		for _, item := range arr {
			if scalarEqual(v, item) {
				return true
			}
		}
	case "contains":
		arr, _ := v.([]any)
		for _, el := range arr {
			if scalarEqual(el, cl.Value) {
				return true
			}
		}
	}
	return false // unknown op → clause false
}

func scalarEqual(a, b any) bool {
	if ab, ok := toBool(a); ok {
		if bb, ok2 := toBool(b); ok2 {
			return ab == bb
		}
	}
	if af, ok := toFloat(a); ok {
		if bf, ok2 := toFloat(b); ok2 {
			return af == bf
		}
	}
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

func toBool(v any) (bool, bool) {
	if b, ok := v.(bool); ok {
		return b, true
	}
	if s, ok := v.(string); ok {
		switch strings.ToLower(s) {
		case "true", "1", "yes", "on":
			return true, true
		case "false", "0", "no", "off":
			return false, true
		}
	}
	return false, false
}

func toFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f, err == nil
	}
	return 0, false
}

// FieldVisible reports whether the field's own visible_if passes.
// Malformed/absent config → visible (fail-open).
func FieldVisible(config []byte, answers map[string]any, selfKey string) bool {
	return ParseFieldRules(config).VisibleIf.Eval(answers, selfKey)
}

// StepVisible reports whether a form_steps.config (or a step_rules entry's
// serialized config) passes its visible_if. Nil/empty config → visible.
func StepVisible(config []byte, answers map[string]any) bool {
	var r StepRules
	if len(config) > 0 {
		_ = json.Unmarshal(config, &r)
	}
	return r.VisibleIf.Eval(answers, "")
}

// HiddenByJump returns the step keys hidden by triggered forward-only jumps.
// stepOrder lists every step identifier in natural order. A jump fires when
// its field passes FieldVisible and jump.When evaluates true; it hides the
// open interval (source, target). Backward/self/unknown targets are ignored.
func HiddenByJump(fields []RuleField, stepOrder []string, answers map[string]any) map[string]bool {
	idx := make(map[string]int, len(stepOrder))
	for i, k := range stepOrder {
		idx[k] = i
	}
	hidden := map[string]bool{}
	for _, f := range fields {
		if !FieldVisible(f.Config, answers, f.Key) {
			continue
		}
		j := ParseFieldRules(f.Config).Jump
		if j == nil || j.Step == "" {
			continue
		}
		if !j.When.Eval(answers, f.Key) {
			continue
		}
		s, sOK := idx[f.StepKey]
		t, tOK := idx[j.Step]
		if !sOK || !tOK || t <= s {
			continue
		}
		for i := s + 1; i < t; i++ {
			hidden[stepOrder[i]] = true
		}
	}
	return hidden
}
```

- [ ] **Step 4: Run tests — expect PASS**

```bash
go test ./internal/formlogic/ -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/formlogic/
git commit -m "feat(formlogic): shared visible_if/jump condition evaluator (Go)"
```

---

### Task 3: `form_steps.config` migration + step query/handler plumbing

**Files:**
- Create: `migrations/001099_form_steps_config.up.sql`, `migrations/001099_form_steps_config.down.sql`
- Modify: `internal/db/queries/forms.sql` (step queries, lines ~67–91)
- Regenerate: `internal/db/gen/` via `sqlc generate`
- Modify: `internal/customfields/handlers.go` — `CreateFormStep` (~3605), `UpdateFormStep` (~3670), both step serialization sites (~1691–1759, ~2259–2313)

**Interfaces:**
- Produces: `gen.FormStep.Config []byte`; `stepJSON` gains `config`; step create/update accept `{"config": {...}}`.

- [ ] **Step 1: Write migrations**

```sql
-- 001099_form_steps_config.up.sql
ALTER TABLE public.form_steps
  ADD COLUMN config jsonb NOT NULL DEFAULT '{}'::jsonb;
```

```sql
-- 001099_form_steps_config.down.sql
ALTER TABLE public.form_steps
  DROP COLUMN config;
```

- [ ] **Step 2: Update the step queries in `internal/db/queries/forms.sql`**

```sql
-- name: ListFormStepsByForm :many
SELECT id, form_id, title, position, config
FROM form_steps
WHERE form_id = @form_id
ORDER BY position ASC;

-- name: CreateFormStep :one
INSERT INTO form_steps (form_id, title, position, config) VALUES (@form_id, @title, @position, @config)
RETURNING id, form_id, title, position, config;

-- name: UpdateFormStep :one
UPDATE form_steps
SET title = @title, position = @position,
 config = CASE WHEN @update_config::bool THEN @config ELSE config END
WHERE id = @id AND form_id = @form_id
RETURNING id, form_id, title, position, config;
```

(`UpdateFormStep` uses the same three-state pattern as `UpdateCustomField`'s
`section` — omitted `config` leaves the stored value.)

- [ ] **Step 3: Regenerate sqlc + fix compile errors**

```bash
make sqlc-gen && go build ./...
```
Fix the (few) callers the new `Config`/`UpdateConfig` params break: pass
`json.RawMessage("{}")` for config on create; `UpdateFormStepParams` gains
`Config` + `UpdateConfig bool`.

- [ ] **Step 4: Accept `config` in `CreateFormStep`/`UpdateFormStep` handlers**

In both request structs (handlers.go ~3628 and ~3683) add:

```go
Config json.RawMessage `json:"config"`
```

Validate it is a JSON object whose only recognized key is `visible_if` —
reject non-objects with `400 invalid_step_config`; pass through to params:

```go
// capture three-state BEFORE defaulting — the update path must distinguish
// "config omitted" (keep stored) from "config present" (replace).
configSent := req.Config != nil
if req.Config == nil {
	req.Config = json.RawMessage("{}")
}
// create:
gen.CreateFormStepParams{FormID: formID, Title: req.Title, Position: req.Position, Config: req.Config}
// update:
gen.UpdateFormStepParams{ID: stepID, FormID: formID, Title: req.Title, Position: req.Position, Config: req.Config, UpdateConfig: configSent}
```

- [ ] **Step 5: Expose step config in both serializers**

Both `stepJSON` types (handlers.go ~1700 and ~2259) gain:

```go
Config json.RawMessage `json:"config"`
```

populated from `st.Config` (default `"{}"` when nil). The admin form GET keeps
the raw object for editing; the public/embed serializer at ~2259 should emit
a whitelisted `{visible_if}` only:

```go
var stepCfg struct {
	VisibleIf json.RawMessage `json:"visible_if,omitempty"`
}
_ = json.Unmarshal(st.Config, &stepCfg)
cfg, _ := json.Marshal(map[string]any{"visible_if": stepCfg.VisibleIf})
```
(emit `cfg` as the step's `config` key; nil visible_if → `"visible_if":null` is
fine, or marshal `{}` when empty.)

- [ ] **Step 6: Run migrate + build + existing tests**

```bash
make migrate   # applies 001099
go build ./... && go test ./internal/customfields/ ./internal/server/ 2>&1 | tail -20
```

- [ ] **Step 7: Commit**

```bash
git add migrations/001099_form_steps_config.*.sql internal/db/queries/forms.sql internal/db/gen/ internal/customfields/handlers.go
git commit -m "feat(forms): form_steps.config column for step-level conditional rules"
```

---

### Task 4: `validateFormSubmission` visibility + embed/public field JSON

**Files:**
- Modify: `internal/customfields/handlers.go` — `validateFormSubmission` (~2682), `fieldDef` struct + `fieldJSON`/`toFieldJSON` serializers (~1640, ~2245)
- Test: `internal/customfields/visibility_test.go` (new)

**Interfaces:**
- Consumes: `formlogic.FieldVisible`, `formlogic.ParseFieldRules` (Task 2).
- Produces: hidden fields skipped by required-check; hidden submitted keys dropped; public field JSON gains `config: {visible_if, jump}`.

- [ ] **Step 1: Write failing test**

```go
package customfields

import (
	"context"
	"encoding/json"
	"testing"
)

// TestDropHiddenFieldValues verifies a field hidden by visible_if neither
// blocks submission as required nor survives in the stored values.
func TestDropHiddenFieldValues(t *testing.T) {
	fields := []fieldDef{
		{Label: "Purpose", Key: "purpose", FieldType: "select", IsRequired: true},
		{Label: "Admission", Key: "admission_status", FieldType: "select", IsRequired: true,
			Config: []byte(`{"visible_if":{"all":[{"field":"purpose","op":"eq","value":"study"}]}}`)},
	}
	got := dropHiddenValues(fields, map[string]any{"purpose": "work"})
	if _, ok := got["admission_status"]; ok {
		t.Fatal("hidden field value should be dropped")
	}
	if _, ok := got["purpose"]; !ok {
		t.Fatal("visible field value must stay")
	}
}
```

- [ ] **Step 2: Implement**

Extend `fieldDef` (the local struct built in `validateFormSubmission`, ~2756)
with `Config []byte`. Populate from `f.Config` in the byLabel/byKey build.

Add helpers (same file or a new `visibility.go` in package):

```go
// canonicalAnswers maps submitted values (label- OR key-keyed) onto field
// keys and normalizes select/multiselect answers to canonical option
// values, so rules compare against option values whether the client sent
// labels (embed renderer) or values (assessment). Storage keeps the
// submitted key style — this view is eval-only.
func canonicalAnswers(fields []fieldDef, vals map[string]any) map[string]any {
	out := make(map[string]any, len(vals))
	for k, v := range vals {
		out[k] = v
	}
	for _, f := range fields {
		if v, ok := vals[f.Label]; ok {
			out[f.Key] = v
		}
	}
	for _, f := range fields {
		v, ok := out[f.Key]
		if !ok {
			continue
		}
		switch f.FieldType {
		case FieldTypeSelect:
			if s, isStr := v.(string); isStr && s != "" {
				out[f.Key] = NormalizeChoiceValue(f.Options, s)
			}
		case FieldTypeMultiselect:
			switch arr := v.(type) {
			case string: // embed renderer submits comma-joined values
				parts := strings.Split(arr, ",")
				norm := make([]any, 0, len(parts))
				for _, p := range parts {
					if t := strings.TrimSpace(p); t != "" {
						norm = append(norm, NormalizeChoiceValue(f.Options, t))
					}
				}
				out[f.Key] = norm
			case []any:
				norm := make([]any, len(arr))
				for i, item := range arr {
					if s, isStr := item.(string); isStr {
						norm[i] = NormalizeChoiceValue(f.Options, s)
					} else {
						norm[i] = item
					}
				}
				out[f.Key] = norm
			}
		}
	}
	return out
}

// hiddenFieldKeys returns the keys of fields whose visible_if fails under
// canon — shared by the required-check skip and the hidden-answer drop.
func hiddenFieldKeys(fields []fieldDef, canon map[string]any) map[string]bool {
	hidden := make(map[string]bool, len(fields))
	for _, f := range fields {
		if !formlogic.FieldVisible(f.Config, canon, f.Key) {
			hidden[f.Key] = true
		}
	}
	return hidden
}

// dropHiddenValues removes entries whose field's visible_if fails against
// the canonical answers; matched by label OR key so the stored snapshot is
// the visible-path truth.
func dropHiddenValues(fields []fieldDef, vals map[string]any) map[string]any {
	hidden := hiddenFieldKeys(fields, canonicalAnswers(fields, vals))
	byKeyOrLabel := make(map[string]fieldDef, len(fields)*2)
	for _, f := range fields {
		byKeyOrLabel[f.Key] = f
		byKeyOrLabel[f.Label] = f
	}
	out := make(map[string]any, len(vals))
	for k, v := range vals {
		if f, ok := byKeyOrLabel[k]; ok && hidden[f.Key] {
			continue
		}
		out[k] = v
	}
	return out
}
```

Then in `validateFormSubmission`, right after the `byLabel`/`byKey` build
loop (~2758) and before the required-fields loop:

```go
// Answer-driven visibility (ADR-137): drop submitted values for fields
// whose visible_if fails, and exclude them from the required check. The
// fieldValues map is mutated in place (same convention as the select
// normalization + rich_text sanitization below) so callers store the
// canonical visible-path snapshot.
canon := canonicalAnswers(fields, fieldValues)
hidden := hiddenFieldKeys(fields, canon)
for rawKey := range fieldValues {
	f, ok := byLabel[rawKey]
	if !ok {
		f, ok = byKey[rawKey]
	}
	if ok && hidden[f.Key] {
		delete(fieldValues, rawKey)
	}
}
```

and change the required loop's skip condition to include `|| hidden[f.Key]`.
(Deletion before the required loop also makes the unknown-field and
type-validation loops ignore hidden entries automatically — deleting is
in-place because reassigning the local `fieldValues` variable would NOT
propagate to the caller's map.)

- [ ] **Step 3: Run tests**

```bash
go test ./internal/customfields/ -v -run TestDropHidden
```

- [ ] **Step 4: Expose `visible_if`/`jump` on field JSON — whitelist public, raw admin**

Two serializer families live in this file — keep them straight:

- **Admin form GET**: `fieldJSON` (~1631) and `stepFieldJSON` (~1697) — gains
  `Config json.RawMessage` populated from `f.Config` **raw** (the Task 10
  editor needs the whole object to merge keys before the full-replace PATCH),
  plus `OptionDefs` carrying the raw `{value,label}` option objects (needed
  by the rule-builder's value picker for select source fields — `options`
  ships labels only):

  ```go
  OptionDefs []struct {
      Value string `json:"value"`
      Label string `json:"label"`
  } `json:"option_defs,omitempty"`
  ```

- **Public embed GET**: `fieldJSON` + `toFieldJSON` (~2201–2240; the same
  helper builds both flat `fields[]` and per-step `fields[]` at ~2286) —
  gains whitelisted `Config` only (no option_defs: admin-authored options
  have value==label so the public renderer never needs the map):

```go
Config json.RawMessage `json:"config,omitempty"`
```

```go
// logicConfigJSON projects the public-safe slice of custom_fields.config:
// visible_if + jump only. Scoring keys (weight, option_scores) never ship
// on public payloads.
func logicConfigJSON(raw []byte) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var cfg struct {
		VisibleIf json.RawMessage `json:"visible_if"`
		Jump      json.RawMessage `json:"jump"`
	}
	if json.Unmarshal(raw, &cfg) != nil || (cfg.VisibleIf == nil && cfg.Jump == nil) {
		return nil
	}
	out, _ := json.Marshal(map[string]json.RawMessage{"visible_if": cfg.VisibleIf, "jump": cfg.Jump})
	return out
}
```

- [ ] **Step 5: Run package tests + build**

```bash
go build ./... && go test ./internal/customfields/
```

- [ ] **Step 6: Commit**

```bash
git add internal/customfields/
git commit -m "feat(forms): honor visible_if in submission validation; ship rules on public field JSON"
```

---

### Task 5: Assessment — submit validation, scoring, public DTO

**Files:**
- Modify: `internal/assessment/service.go` — `FieldScoringConfig` (111), `SubmitAssessment` (345), `missingRequiredFields` (397), `invalidOptionAnswers` (440), `computeScore` (1245)
- Modify: `internal/assessment/handlers.go` — `publicFieldDTOs` (188), `HandleGetAssessment` response (~259–277)
- Test: `internal/assessment/visibility_test.go` (new)

**Interfaces:**
- Consumes: `formlogic` (Task 2).
- Produces: `FieldScoringConfig.VisibleIf/Jump`; visible-only submit validation; `computeScore` skips hidden; public payload ships `visible_if`/`jump`/`step_rules`.

- [ ] **Step 1: Write failing tests**

```go
package assessment

import (
	"encoding/json"
	"testing"

	"japaconnect/internal/db/gen"
	"japaconnect/internal/formlogic"
)

func TestMissingRequiredSkipsHidden(t *testing.T) {
	fields := []AssessmentField{
		{Key: "gate", Required: true},
		{Key: "dep", Required: true,
			Config: FieldScoringConfig{VisibleIf: &formlogic.Condition{
				All: []formlogic.Clause{{Field: "gate", Op: "eq", Value: "yes"}},
			}}},
	}
	vals, _ := json.Marshal(map[string]any{"gate": "no"})
	if got := missingRequiredFields(fields, vals); len(got) != 0 {
		t.Fatalf("hidden required field blocked submit: %v", got)
	}
}

func TestComputeScoreSkipsHidden(t *testing.T) {
	fields := []gen.CustomField{
		{Key: "gate", FieldType: "select", Config: []byte(`{"weight":1,"dimension":"d","option_scores":{"yes":100,"no":0}}`)},
		{Key: "dep", FieldType: "select", Config: []byte(`{"weight":1,"dimension":"d","option_scores":{"x":100},
			"visible_if":{"all":[{"field":"gate","op":"eq","value":"yes"}]}}`)},
	}
	// gate=no hides dep; a stale dep=x answer must not score.
	vals, _ := json.Marshal(map[string]any{"gate": "no", "dep": "x"})
	got := computeScore(vals, fields, ScoringConfig{})
	if got.DimensionScores["d"] != 0 {
		t.Fatalf("hidden field scored: %v", got.DimensionScores)
	}
}

func TestPublicFieldDTOsShipsRulesNotScores(t *testing.T) {
	out := publicFieldDTOs([]AssessmentField{{
		ID: "1", Label: "Q", Key: "q", FieldType: "select",
		Config: FieldScoringConfig{
			Weight: 1, Dimension: "d", OptionScores: map[string]int{"a": 5},
			VisibleIf: &formlogic.Condition{All: []formlogic.Clause{{Field: "g", Op: "answered"}}},
		},
	}})
	cfg := out[0]["config"].(map[string]any)
	if _, leak := cfg["option_scores"]; leak {
		t.Fatal("option_scores leaked to public payload")
	}
	if _, ok := cfg["visible_if"]; !ok {
		t.Fatal("visible_if missing from public payload")
	}
}
```

- [ ] **Step 2: Extend `FieldScoringConfig` + helpers**

```go
type FieldScoringConfig struct {
	Weight       float64              `json:"weight"`
	Dimension    string               `json:"dimension"`
	OptionScores map[string]int       `json:"option_scores"`
	VisibleIf    *formlogic.Condition `json:"visible_if,omitempty"`
	Jump         *formlogic.JumpRule  `json:"jump,omitempty"`
}
```

Add to service.go (`AssessmentField.Config` is already a parsed
`FieldScoringConfig`, so call `Eval` on the typed member directly — no
re-marshal):

```go
// visibleFieldSet returns keys of fields whose visible_if passes for the
// submitted answers — the single source of truth shared by submit
// validation and scoring.
func visibleFieldSet(fields []AssessmentField, answers map[string]any) map[string]bool {
	set := make(map[string]bool, len(fields))
	for _, f := range fields {
		if f.Config.VisibleIf.Eval(answers, f.Key) {
			set[f.Key] = true
		}
	}
	return set
}
```

(`(*Condition).Eval` is nil-receiver safe → absent rules = visible.)

`computeScore` works on `[]gen.CustomField` and already parses each config
via `parseFieldScoringConfig` — which returns the extended struct with
`VisibleIf` filled. Inside its answers loop (~1273), after the `fsc` parse:

```go
fsc := parseFieldScoringConfig(field.Config)
if !fsc.VisibleIf.Eval(answers, key) {
	continue // defense-in-depth: stale/crafted answer for a hidden field
}
if fsc.Dimension == "" {
	continue
}
```

- [ ] **Step 3: Apply to submit validation + canonical storage**

In `SubmitAssessment` (~345): unmarshal `fieldValues` to `map[string]any`
once, compute `vis := visibleFieldSet(form.Fields, answers)`, then:

- `missingRequiredFields`/`invalidOptionAnswers` take a `visible map[string]bool`
  param (update signatures + both call sites); skip `!visible[f.Key]` fields.
- Build the canonical payload: delete `answers[k]` for every `!vis[k]` key
  that names a defined form field (leave genuinely unknown keys — the
  journey/option validators ignore them); re-marshal and pass THAT to
  `CreateAssessmentSubmission` so the stored snapshot is the visible-path
  truth.

- [ ] **Step 4: Extend `publicFieldDTOs` + `HandleGetAssessment`**

In `publicFieldDTOs` (~206) extend the config whitelist:

```go
cfg := map[string]any{"dimension": f.Config.Dimension}
if f.Config.VisibleIf != nil {
	cfg["visible_if"] = f.Config.VisibleIf
}
if f.Config.Jump != nil {
	cfg["jump"] = f.Config.Jump
}
... "config": cfg,
```

In `HandleGetAssessment`'s `scoring_config` map (~272) add
`"step_rules": form.ScoringConfig.StepRules` and extend `ScoringConfig`:

```go
StepRules map[string]struct {
	VisibleIf *formlogic.Condition `json:"visible_if,omitempty"`
} `json:"step_rules,omitempty"`
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/assessment/ -v && go build ./...
```

- [ ] **Step 6: Commit**

```bash
git add internal/assessment/
git commit -m "feat(assessment): visible_if-aware validation + scoring; rules on public payload"
```

---

### Task 6: Field `config` write path (admin PATCH/POST)

**Files:**
- Modify: `internal/db/queries/custom_fields.sql` — `CreateCustomField`, `UpdateCustomField` (~90–120)
- Regenerate: `internal/db/gen/` via `sqlc generate`
- Modify: `internal/customfields/handlers.go` — `fieldRequest` (~801), create/update paths (~929, ~1023), admin `fieldJSON` (~1640)
- Test: `internal/customfields/config_write_test.go` (new)

- [ ] **Step 1: Failing test — `validFieldConfig` guard**

```go
package customfields

import "testing"

func TestValidFieldConfig(t *testing.T) {
	cases := []struct{ name, in string; want bool }{
		{"object", `{"visible_if":{"all":[]}}`, true},
		{"empty object", `{}`, true},
		{"array", `[1,2]`, false},
		{"scalar", `5`, false},
		{"string", `"x"`, false},
		{"bad json", `{oops`, false},
	}
	for _, c := range cases {
		if got := validFieldConfig([]byte(c.in)); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}
```

```go
// validFieldConfig accepts only JSON objects (or nil) — the config column
// must stay object-shaped for the rule keys the evaluator reads.
func validFieldConfig(raw []byte) bool {
	if len(raw) == 0 {
		return true
	}
	var v map[string]any
	return json.Unmarshal(raw, &v) == nil
}
```

(The PATCH/GET round-trip is exercised in Task 12's manual pass — the unit
gate here is the request validation.)

- [ ] **Step 2: SQL + regen**

`CreateCustomField` INSERT adds `config` column + `@config` param.
`UpdateCustomField` SET adds:

```sql
config = CASE WHEN @update_config::bool THEN @config ELSE config END,
```

Run `make sqlc-gen && go build ./...` — thread `Config`/`UpdateConfig` params
through `CreateField`/`UpdateField` service calls.

- [ ] **Step 3: `fieldRequest` + handlers**

```go
// Config replaces the full config object when present (PATCH three-state:
// omitted = keep stored). The admin editor merges keys client-side and
// sends the whole object.
Config *json.RawMessage `json:"config"`
```

In both create and update: when `req.Config != nil`, validate it unmarshals
to a JSON object (reject arrays/scalars → `400 invalid_config`), then pass
`[]byte(*req.Config)`. Create: nil `Config` → pass `json.RawMessage("{}")`
(the column may lack a default on existing inserts). Update: `Config: raw,
UpdateConfig: true` — and when `req.Config == nil`, `UpdateConfig: false`
(the CASE then preserves the stored value; `Config` can stay nil).

(The admin `fieldJSON` raw-`Config` passthrough is already done in Task 4
Step 4 — no work needed here; this task only wires the write path.)

- [ ] **Step 4: Tests + build + commit**

```bash
go test ./internal/customfields/ && go build ./...
git add -A && git commit -m "feat(forms): PATCH/POST field config write path (visible_if/jump/scoring keys)"
```

---

### Task 7: `packages/formlogic` — TS evaluator

**Files:**
- Create: `packages/formlogic/package.json`, `packages/formlogic/src/index.ts`, `packages/formlogic/src/index.test.ts`
- Modify: `apps/main/package.json`, `apps/admin/package.json` (dep `"@japaconnect/formlogic": "workspace:*"`)
- Run: `bun install` **at repo root**

**Interfaces:**
- Produces (consumed by Tasks 8–10):
  ```ts
  export interface Clause { field?: string; op: string; value?: unknown }
  export interface Condition { all?: Clause[]; any?: Clause[] }
  export interface JumpRule { when?: Condition; step: string }
  export function evalCondition(cond: Condition | null | undefined, answers: Record<string, unknown>, selfKey?: string): boolean
  export function fieldVisible(config: { visible_if?: Condition | null } | null | undefined, answers: Record<string, unknown>, selfKey?: string): boolean
  export function visibleFields<T extends { key: string; config?: { visible_if?: Condition | null } | null }>(fields: T[], answers): T[]
  export function hiddenStepsByJump(fields: { key: string; stepKey: string; config: { jump?: JumpRule | null; visible_if?: Condition | null } | null }[], stepOrder: string[], answers): Set<string>
  export function stepVisible(config: { visible_if?: Condition | null } | null | undefined, answers): boolean
  ```

- [ ] **Step 1: Package scaffold + failing test**

`package.json` — mirror `packages/api-url/package.json` (name
`@japaconnect/formlogic`, `"type": "module"`, exports `./src/index.ts`).

`index.test.ts` — mirror the Go vector list from Task 2 Step 1 verbatim
(same cases, same expectations) using `bun:test`:

```ts
import { describe, it, expect } from 'bun:test';
import { evalCondition, fieldVisible, hiddenStepsByJump } from './index';
// ... same 30+ cases as Task 2 Step 1, adapted to TS literals
```

- [ ] **Step 2: `bun test packages/formlogic` → FAIL (exports missing)**

- [ ] **Step 3: Implement `src/index.ts`**

Direct port of `logic.go` — same function names (camelCase), same semantics:
`isAnswered`, `scalarEqual` (bool → number → string normalization),
`evalClause` (selfKey defaulting), `evalCondition`, `fieldVisible`,
`stepVisible`, `visibleFields` (filter wrapper), `hiddenStepsByJump` (returns
`Set<string>` of hidden step keys; same fire-only-from-visible + forward-only
+ union rules).

- [ ] **Step 4: `bun test packages/formlogic` → PASS; `bun install` at root;
  `bun run check` (or `bunx svelte-check`) in `apps/main` still green.**

- [ ] **Step 5: Commit**

```bash
git add packages/formlogic apps/main/package.json apps/admin/package.json bun.lock
git commit -m "feat(formlogic): TS condition evaluator package (@japaconnect/formlogic)"
```

---

### Task 8: Readiness page integration (`apps/main`)

**Files:**
- Modify: `apps/main/src/lib/types/index.ts` — `AssessmentFieldScoringConfig` (~2788), `AssessmentScoringConfig` (~2782)
- Modify: `apps/main/src/routes/(app)/tools/readiness/+page.svelte`

**Interfaces:**
- Consumes: `@japaconnect/formlogic` (Task 7); `config.visible_if`/`jump`/`step_rules` on the wire (Task 5).

- [ ] **Step 1: Types**

```ts
export interface LogicClause { field?: string; op: string; value?: unknown }
export interface LogicCondition { all?: LogicClause[]; any?: LogicClause[] }
export interface AssessmentFieldScoringConfig {
	weight: number;
	dimension: string;
	option_scores?: Record<string, number>;
	visible_if?: LogicCondition | null;
	jump?: { when?: LogicCondition; step: string } | null;
}
export interface AssessmentScoringConfig {
	max_score: number;
	thresholds: AssessmentThreshold[];
	dimension_weights: Record<string, number>;
	step_rules?: Record<string, { visible_if?: LogicCondition | null }>;
}
```

- [ ] **Step 2: Wire visibility into the step pipeline**

In `+page.svelte` (script): import `fieldVisible`, `visibleFields`, `stepVisible`/`evalCondition`, `hiddenStepsByJump` from `@japaconnect/formlogic`.

Replace the `steps` derived (~202) so it:

1. `visFields = visibleFields(fields, answers)` — reactive on answers.
2. Builds dimension groups from `visFields` only (existing loop), tracking
   `stepKey → fields`.
3. `stepOrder` = dimension order over the FULL field list (needed for jump
   intervals) → `hiddenByJump = hiddenStepsByJump(fields.map(f => ({key: f.key, stepKey: f.config?.dimension ?? 'general', config: f.config})), stepOrder, answers)`.
4. Drops steps where `hiddenByJump.has(dim)` OR `stepVisible(stepRules?.[dim], answers)` fails OR zero visible fields (`step_rules[dim]` is the `{visible_if}` wrapper object the `stepVisible` signature expects).
5. `requiredFields`, `requiredAnsweredCount`, `stepComplete` all derive from
   the visible set.
6. `buildFieldValues` keeps sending the full `answers` (server canonicalizes).
7. `$effect`: clamp `currentStep` when `steps[currentStep]` vanishes:
   `if (currentStep >= steps.length) currentStep = Math.max(0, steps.length - 1)`.

- [ ] **Step 3: Verify in dev**

`cd apps/main && bunx vite dev --port 3000` — seed a `visible_if` on a field
via the Task 6 PATCH (or direct SQL update on `custom_fields.config`), answer
the gating field both ways, confirm reveal/hide + step skip in the rail.

- [ ] **Step 4: Typecheck + commit**

```bash
cd apps/main && bunx svelte-check --tsconfig ./tsconfig.json 2>&1 | tail -10
git add apps/main && git commit -m "feat(readiness): conditional fields/steps + goto jumps in assessment UI"
```

---

### Task 9: Generic form page (`apps/main/src/routes/(app)/forms/[slug]`)

**Files:**
- Modify: `+page.svelte` (217 lines — small)
- Modify: `apps/main/src/lib/types/index.ts` — `FormField` + the `form` type's `steps` shape

- [ ] **Step 1:** `FormField` gains `config?: { visible_if?: LogicCondition; jump?: {when?: LogicCondition; step: string} } | null`; step type gains `config?: { visible_if?: LogicCondition } | null`.

- [ ] **Step 2:** Filter `sortedFields` per step by `fieldVisible(f.config, values, fieldKey(f))`;
filter `steps` by `stepVisible(s.config, values)` + `hiddenStepsByJump` (field
`stepKey` = containing step `id`; `stepOrder` = step ids by position) +
auto-hide when the step has fields defined but none visible. `validateStep`
checks visible fields only. Multiselect `values` here are comma-joined strings —
convert to array before eval for `contains` (`v.split(',').filter(Boolean)`)
or document `contains` unsuitability for this renderer; keep parity by feeding
the evaluator a normalized view: `evalValues(k => typeof === 'string' && field.type === 'multiselect' ? split : v)`.

- [ ] **Step 3:** Dev-verify on a multi-step test form; commit:
`feat(forms): honor visible_if/jump on generic form renderer`.

---

### Task 10: Admin rule editors (`apps/admin/src/routes/forms/[formId]`)

**Files:**
- Modify: `+page.svelte` — FieldDef type (~43), edit-field modal (~1390+), step rows (~1009), assessment-steps card (~1056), `updateField`/`addField` (~675–760)
- Modify: `apps/admin/src/lib/locales/en.json`, `fr.json` (new keys)

**Interfaces:**
- Consumes: `fieldRequest.config` write path (Task 6), `form_steps.config` (Task 3), `@japaconnect/formlogic` for save-time warnings.

- [ ] **Step 1:** `FieldDef` gains `config?: Record<string, unknown>` (server
  now returns it). `openEditField` seeds `editFieldVisibleIf`/`editFieldJump`
  from `field.config`; `updateField` sends `config` (merge existing config +
  edited keys, send whole object — config is full-replace).
- [ ] **Step 2:** Edit-field modal: "Visibility" section — source-field select
  (all field keys on the form), op select (six ops), value input (option
  dropdown of **canonical values** from the source field's `option_defs`
  when source is select/radio, checkbox for boolean, text else);
  plus/minus row per clause inside `all` (keep v1 UI to `all`-only — `any`
  editable via the existing JSON path). "Jump" section — `when` = same clause
  editor defaulting `field` to self, `step` = select of later steps
  (dimensions for assessment, step ids for standard).
- [ ] **Step 3:** Assessment-steps card: per-dimension `visible_if` editor
  writing `scoring_config.step_rules` via the existing
  `readinessAdminApi.updateForm` path. Standard step rows: `visible_if`
  editor → `stepApi.update(id, {config})`.
- [ ] **Step 4:** Save-time warnings via `hiddenStepsByJump` + ref check:
  unknown `field` refs, backward/self/unknown jump targets, jump to
  always-hidden step → confirm dialog listing warnings before PUT.
- [ ] **Step 5:** i18n keys (en+fr), `bunx svelte-check`, manual verify, commit:
`feat(forms): admin editors for visible_if/jump/step_rules with save warnings`.

---

### Task 11: Mobile — Dart evaluator + readiness screen

**Files:**
- Create: `packages/japaconnect_core/lib/utils/form_logic.dart`, `packages/japaconnect_core/test/utils/form_logic_test.dart`
- Modify: `packages/japaconnect_core/lib/japaconnect_core.dart` (export), `apps/mobile/lib/features/tools/readiness/screens/readiness_screen.dart`

- [ ] **Step 1:** Failing `flutter_test` mirroring the Task 2 vector list on
  `Map<String, dynamic>` inputs.
- [ ] **Step 2:** Port evaluator — functions operate on decoded maps/lists:
  `evalCondition(Map<String,dynamic>? cond, Map<String,dynamic> answers, [String selfKey=''])`,
  `fieldVisible(Map<String,dynamic>? config, answers, selfKey)`,
  `hiddenStepsByJump(List<Map<String,dynamic>> fields, List<String> stepOrder, answers)`
  (fields read `f['key']`, `f['config']['dimension']` as stepKey,
  `f['config']['jump']`).
- [ ] **Step 3:** Wire `_fieldsForStep`/`_stepOrder`/`_isStepComplete`/`_submit`
  to the visible set + hidden steps; `_currentStep` clamped via
  `WidgetsBinding.instance.addPostFrameCallback` when `order[_currentStep]` is
  out of range.
- [ ] **Step 4:** `flutter test` in `packages/japaconnect_core` + `apps/mobile`
  analyze; commit `feat(mobile): conditional fields/steps in readiness screen`.

---

### Task 12 (final): End-to-end verification + housekeeping

- [ ] **Step 1:** `make check` (build vet lint test) at repo root; `bun test` for
  new TS package; `flutter test` japaconnect_core.
- [ ] **Step 2:** Author a demo rule via the admin UI on the readiness form
  (e.g. a `has_dependents` → `dependents_plan` `visible_if`) and walk
  `/tools/readiness` end-to-end: reveal → submit → result shows excluded
  dimension math.
- [ ] **Step 3:** If all green, final commit of any stragglers; report.
