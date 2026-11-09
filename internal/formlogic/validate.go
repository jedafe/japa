package formlogic

// Shape validation for the §06 logic keys at write time. Evaluation treats
// malformed input permissively (fail-open/fail-closed per clause), so these
// checks exist to catch authoring errors at the API boundary instead of
// letting dead rules silently persist.
// Ref: japaconnect-engineering/sections/06-custom-field-builder-form-builder.md (ADR-137)

import (
	"errors"
	"fmt"
)

// knownOps is the operator set evalClause understands. Writing an unknown op
// produces a clause that can never hold — reject it up front.
var knownOps = map[string]bool{
	"eq":           true,
	"neq":          true,
	"in":           true,
	"contains":     true,
	"answered":     true,
	"not_answered": true,
}

// ValidateFieldLogic checks the logic keys a field config may carry:
// visible_if must be a well-formed condition; jump, when present, must be
// {when?: condition, step: string}. Unknown keys are ignored — config also
// carries scoring keys this function does not own.
func ValidateFieldLogic(config map[string]any) error {
	if v, ok := config["visible_if"]; ok {
		if err := validateCondition(v, "visible_if"); err != nil {
			return err
		}
	}

	if v, ok := config["jump"]; ok {
		if err := validateJump(v); err != nil {
			return err
		}
	}

	return nil
}

// ValidateStepLogic checks a step-level config — visible_if only.
func ValidateStepLogic(config map[string]any) error {
	if v, ok := config["visible_if"]; ok {
		if err := validateCondition(v, "visible_if"); err != nil {
			return err
		}
	}

	return nil
}

// ValidateStepRules checks a scoring_config.step_rules map: every entry must
// be an object whose visible_if (when present) is a well-formed condition.
func ValidateStepRules(v any) error {
	m, ok := v.(map[string]any)
	if !ok {
		return errors.New("step_rules must be an object")
	}

	for step, entry := range m {
		em, ok := entry.(map[string]any)
		if !ok {
			return fmt.Errorf("step_rules.%s must be an object", step)
		}

		if vis, present := em["visible_if"]; present {
			if err := validateCondition(vis, fmt.Sprintf("step_rules.%s.visible_if", step)); err != nil {
				return err
			}
		}
	}

	return nil
}

// validateCondition checks a decoded condition: an object whose all/any
// members (when present) are arrays of clause objects.
func validateCondition(v any, path string) error {
	m, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("%s must be an object", path)
	}

	for _, key := range []string{"all", "any"} {
		raw, present := m[key]
		if !present {
			continue
		}

		arr, ok := raw.([]any)
		if !ok {
			return fmt.Errorf("%s.%s must be an array of clauses", path, key)
		}

		for i, cl := range arr {
			if err := validateClause(cl, fmt.Sprintf("%s.%s[%d]", path, key, i)); err != nil {
				return err
			}
		}
	}

	return nil
}

// validateClause checks one clause: an object with a known op, a string
// field when given, and — for `in` — an array value when a value is given.
func validateClause(v any, path string) error {
	m, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("%s must be an object", path)
	}

	op, present := m["op"]
	if !present {
		return fmt.Errorf("%s.op is required", path)
	}

	opStr, ok := op.(string)
	if !ok || !knownOps[opStr] {
		return fmt.Errorf("%s.op %v is not a known operator", path, op)
	}

	if f, present := m["field"]; present {
		if _, ok := f.(string); !ok {
			return fmt.Errorf("%s.field must be a string", path)
		}
	}

	if opStr == "in" {
		if val, has := m["value"]; has {
			if _, isArr := val.([]any); !isArr {
				return fmt.Errorf("%s.value must be an array for the 'in' operator", path)
			}
		}
	}

	return nil
}

// validateJump checks a decoded jump rule: an object whose optional when is
// a well-formed condition and whose step is a non-empty string.
func validateJump(v any) error {
	m, ok := v.(map[string]any)
	if !ok {
		return errors.New("jump must be an object")
	}

	if w, present := m["when"]; present {
		if err := validateCondition(w, "jump.when"); err != nil {
			return err
		}
	}

	step, present := m["step"]
	if !present {
		return errors.New("jump.step is required")
	}

	s, ok := step.(string)
	if !ok || s == "" {
		return errors.New("jump.step must be a non-empty step identifier")
	}

	return nil
}
