package pofcalculator

import "testing"

func TestResolveSchedule_ArrayForm(t *testing.T) {
	// Express Entry style: exact per-member amounts.
	schedule := []byte(`[
		{"members":1,"amount":15263},
		{"members":2,"amount":19001},
		{"members":3,"amount":23360},
		{"members":7,"amount":36734,"per_additional":3941}
	]`)

	cases := []struct {
		household int
		want      float64
		wantErr   bool
	}{
		{1, 15263, false},
		{2, 19001, false},
		{3, 23360, false},
		{7, 36734, false},
		{8, 36734 + 3941, false}, // overage via per_additional on highest band
		{10, 36734 + 3*3941, false},
	}
	for _, tc := range cases {
		got, err := resolveSchedule(schedule, tc.household)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("household=%d: expected error, got %v", tc.household, got)
			}
			continue
		}
		if err != nil {
			t.Fatalf("household=%d: unexpected error %v", tc.household, err)
		}
		if got != tc.want {
			t.Fatalf("household=%d: got %v, want %v", tc.household, got, tc.want)
		}
	}
}

func TestResolveSchedule_FormulaForm(t *testing.T) {
	// UK student visa style: base for N members + per-additional.
	schedule := []byte(`{"base":13347,"base_members":1,"per_additional":680}`)

	if got, err := resolveSchedule(schedule, 1); err != nil || got != 13347 {
		t.Fatalf("household=1: got %v err %v, want 13347", got, err)
	}
	if got, err := resolveSchedule(schedule, 3); err != nil || got != 13347+2*680 {
		t.Fatalf("household=3: got %v err %v, want %v", got, err, 13347+2*680)
	}
}

func TestResolveSchedule_Invalid(t *testing.T) {
	if _, err := resolveSchedule([]byte(`{}`), 2); err == nil {
		t.Fatal("expected error on empty object")
	}
	if _, err := resolveSchedule([]byte(`[]`), 2); err == nil {
		t.Fatal("expected error on empty array")
	}
	if _, err := resolveSchedule([]byte(`not json`), 2); err == nil {
		t.Fatal("expected error on invalid json")
	}
}
