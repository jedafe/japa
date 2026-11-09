package customfields

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestValidateFieldConfig(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"object", `{"visible_if":{"all":[]}}`, true},
		{"empty object", `{}`, true},
		{"array", `[1,2]`, false},
		{"scalar", `5`, false},
		{"string", `"x"`, false},
		{"bad json", `{oops`, false},
		{"valid visible_if", `{"visible_if":{"all":[{"field":"a","op":"eq","value":1}]}}`, true},
		{"valid jump", `{"jump":{"when":{"all":[{"op":"eq","value":"x"}]},"step":"s2"}}`, true},
		{"visible_if not object", `{"visible_if":"nope"}`, false},
		{"clause not object", `{"visible_if":{"all":[5]}}`, false},
		{"clause missing op", `{"visible_if":{"all":[{"field":"a"}]}}`, false},
		{"unknown op", `{"visible_if":{"all":[{"op":"between"}]}}`, false},
		{"in needs array value", `{"visible_if":{"all":[{"op":"in","value":"a"}]}}`, false},
		{"jump missing step", `{"jump":{"when":{"all":[{"op":"eq"}]}}}`, false},
		{"jump bad when", `{"jump":{"when":"x","step":"s2"}}`, false},
		{"scoring keys valid", `{"weight":2,"dimension":"d","option_scores":{"a":1}}`, true},
		{"weight not number", `{"weight":"2"}`, false},
		{"dimension not string", `{"dimension":5}`, false},
		{"option_scores not object", `{"option_scores":[1]}`, false},
		{"option_scores fractional", `{"option_scores":{"a":2.5}}`, false},
		{"option_scores non-numeric", `{"option_scores":{"a":"x"}}`, false},
	}
	for _, c := range cases {
		err := validateFieldConfig([]byte(c.in))
		if got := err == nil; got != c.want {
			t.Errorf("%s: got valid=%v want %v (err=%v)", c.name, got, c.want, err)
		}
	}
}

// A partial or semantically-invalid scoring_config must be rejected before
// the service is touched — the column is fully replaced, so a
// {step_rules}-only write would silently wipe max_score/thresholds/weights.
// The nil service proves validation returns first (any service call would
// panic).
func TestApplyScoringConfigValidation(t *testing.T) {
	t.Parallel()

	h := &Handlers{}
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPatch, "/x", nil)

	cases := []struct {
		name string
		body string
	}{
		{"step_rules only clobbers", `{"step_rules":{"d":{"visible_if":{"all":[]}}}}`},
		{"missing thresholds", `{"max_score":100,"step_rules":{}}`},
		{"non-positive max_score", `{"max_score":0,"thresholds":[{"min":0,"max":10,"label":"x"}]}`},
		{"bad step_rules clause op", `{"max_score":100,"thresholds":[{"min":0,"max":10,"label":"x"}],"step_rules":{"d":{"visible_if":{"all":[{"op":"bogus"}]}}}}`},
		{"non-object", `[1,2]`},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		handled := h.applyScoringConfig(rec, req, pgtype.UUID{}, json.RawMessage(c.body), false)

		if !handled || rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: got handled=%v status=%d, want handled=true 422", c.name, handled, rec.Code)
		}
	}
}
