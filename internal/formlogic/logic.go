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
	"slices"
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

// ParseFieldRules decodes the logic slice of a field config. Malformed
// input yields the zero value (fail-open: field visible, no jump).
func ParseFieldRules(config []byte) FieldRules {
	var r FieldRules
	if len(config) > 0 {
		_ = json.Unmarshal(config, &r)
	}

	return r
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

// VisiblePath walks fields in list order, evaluating each field's
// visible_if against answers restricted to fields already determined
// visible. A hidden field's retained/stale answer is off the visible path
// — it does not drive downstream rules, extending the "invisible fields
// can't drive the flow" rule from jump sources to visible_if drivers
// (ADR-137). Returns the visible key set and the restricted answers map —
// pass the latter to step visible_if and jump evaluation so rules never
// read answers the user could not supply.
//
// Order matters: conditions should reference fields positioned earlier
// than the gated field (the authoring convention clients render in). A
// clause referencing a later field sees it as unanswered.
func VisiblePath(fields []RuleField, answers map[string]any) (visible map[string]bool, evalAnswers map[string]any) {
	visible = make(map[string]bool, len(fields))

	// Seed with non-field keys (external context clauses may reference)
	// then re-add each visible field's answer — a hidden field's answer
	// never reaches the eval map.
	evalAnswers = make(map[string]any, len(answers))
	for k, v := range answers {
		evalAnswers[k] = v
	}

	for _, f := range fields {
		delete(evalAnswers, f.Key)
	}

	for _, f := range fields {
		if !FieldVisible(f.Config, evalAnswers, f.Key) {
			continue
		}

		visible[f.Key] = true
		if v, ok := answers[f.Key]; ok {
			evalAnswers[f.Key] = v
		}
	}

	return visible, evalAnswers
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
// stepOrder lists every step identifier in natural order; stepHidden seeds
// the hidden set with steps hidden by other mechanisms (their own
// visible_if) — pass nil when only jumps should be considered. A jump fires
// when its field passes FieldVisible, its containing step is not already
// hidden, and jump.When holds; an absent/empty When fires iff the owning
// field is answered (a jump is answer-driven by definition — an
// unconditional interval skip is an authoring error, not a rule). Sources
// are processed in natural step order so a field inside a step hidden by an
// earlier jump can never fire its own; it hides the open interval
// (source, target). Backward/self/unknown targets are ignored.
// visible carries the VisiblePath result — a field hidden by its own
// visible_if can never fire a jump; nil falls back to per-field
// FieldVisible evaluation for callers that have not run the ordered pass.
func HiddenByJump(fields []RuleField, stepOrder []string, answers map[string]any, stepHidden map[string]bool,
	visible map[string]bool,
) map[string]bool {
	idx := make(map[string]int, len(stepOrder))
	for i, k := range stepOrder {
		idx[k] = i
	}

	hidden := make(map[string]bool, len(stepHidden))
	for k := range stepHidden {
		hidden[k] = true
	}

	// Sort candidate sources by containing-step index so a field inside a
	// step hidden by an earlier-firing jump is skipped — invisible fields
	// cannot drive the flow.
	sorted := make([]RuleField, len(fields))
	copy(sorted, fields)
	slices.SortStableFunc(sorted, func(a, b RuleField) int {
		return idx[a.StepKey] - idx[b.StepKey] // unknown steps sort to the front but fail the idx lookup below
	})

	for _, f := range sorted {
		j := fireableJump(f, hidden, answers, visible)
		if j == nil {
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

// fireableJump returns the field's jump rule when it may fire at the
// current answer state — nil otherwise. The source must be visible to the
// user: its own visible_if passes and its containing step is not already
// hidden (seeded or by an earlier jump). A When with no clauses fires iff
// the owning field is answered.
func fireableJump(f RuleField, hidden map[string]bool, answers map[string]any, visible map[string]bool) *JumpRule {
	if hidden[f.StepKey] {
		return nil
	}

	if visible != nil {
		if !visible[f.Key] {
			return nil
		}
	} else if !FieldVisible(f.Config, answers, f.Key) {
		return nil
	}

	j := ParseFieldRules(f.Config).Jump
	if j == nil || j.Step == "" {
		return nil
	}

	if len(j.When.All) == 0 && len(j.When.Any) == 0 {
		if v, ok := answers[f.Key]; !isAnswered(v, ok) {
			return nil
		}
	} else if !j.When.Eval(answers, f.Key) {
		return nil
	}

	return j
}
