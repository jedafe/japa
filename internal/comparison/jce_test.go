package comparison

import (
	"encoding/json"
	"testing"
)

func TestJCEDepthConstants(t *testing.T) {
	t.Parallel()

	if DepthQuick != "quick" {
		t.Errorf("DepthQuick = %q, want quick", DepthQuick)
	}

	if DepthDetailed != "detailed" {
		t.Errorf("DepthDetailed = %q, want detailed", DepthDetailed)
	}
}

func TestSelectionTokenKey(t *testing.T) {
	t.Parallel()

	token := "abc123"
	want := "jc:compare:selection:abc123"
	got := selectionTokenKey(token)

	if got != want {
		t.Errorf("selectionTokenKey(%q) = %q, want %q", token, got, want)
	}
}

func TestMarshalItemRef(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ref  any
		want string
	}{
		{name: "nil", ref: nil, want: "{}"},
		{name: "map", ref: map[string]any{"type": "university", "id": "u1"}, want: `{"id":"u1","type":"university"}`},
		{name: "struct", ref: struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}{Type: "course", ID: "c1"}, want: `{"type":"course","id":"c1"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := marshalItemRef(tt.ref)

			var gotMap, wantMap map[string]any
			if err := json.Unmarshal(got, &gotMap); err != nil {
				t.Fatalf("unmarshal got: %v", err)
			}

			if err := json.Unmarshal([]byte(tt.want), &wantMap); err != nil {
				t.Fatalf("unmarshal want: %v", err)
			}

			for k, v := range wantMap {
				if gotMap[k] != v {
					t.Errorf("marshalItemRef(%v) = %s, want %s", tt.ref, got, tt.want)
				}
			}
		})
	}
}

func TestParseOptionalUUID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		wantErr  bool
		wantZero bool
	}{
		{name: "empty", input: "", wantErr: false, wantZero: true},
		{name: "valid", input: "00000000-0000-0000-0000-000000000001", wantErr: false, wantZero: false},
		{name: "invalid", input: "not-a-uuid", wantErr: true, wantZero: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseOptionalUUID(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseOptionalUUID(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}

			isZero := !got.Valid
			if isZero != tt.wantZero {
				t.Errorf("parseOptionalUUID(%q) zero=%v, want %v", tt.input, isZero, tt.wantZero)
			}
		})
	}
}

func TestParseTypeSlugCSV(t *testing.T) {
	t.Parallel()

	cases := []struct {
		raw  string
		want []string
	}{
		{"countries", []string{"countries"}},
		{"countries,states,cities", []string{"countries", "states", "cities"}},
		{" countries , states ", []string{"countries", "states"}},
		{"", nil},
		{"a,,b,", []string{"a", "b"}},
	}
	for _, tc := range cases {
		got := parseTypeSlugCSV(tc.raw)
		if len(got) != len(tc.want) {
			t.Fatalf("parseTypeSlugCSV(%q) = %v, want %v", tc.raw, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("parseTypeSlugCSV(%q)[%d] = %q, want %q", tc.raw, i, got[i], tc.want[i])
			}
		}
	}
}
