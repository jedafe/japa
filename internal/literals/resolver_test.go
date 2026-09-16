package literals

import (
	"context"
	"testing"
	"time"
)

func TestResolve(t *testing.T) {
	t.Parallel()

	rc := RenderContext{
		Now:                time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC),
		SourceCountry:      "Nigeria",
		SourceCode:         "NG",
		DestinationCountry: "the UK",
		DestinationCode:    "GB",
		PlatformName:         "JapaConnect",
	}

	tokens := []TemplateLiteral{
		{Key: "Current_Year", Kind: "expression", Value: "year(Now)"},
		{Key: "Destination_Country", Kind: "expression", Value: "Destination_Country"},
		{Key: "Source_Country", Kind: "expression", Value: "Source_Country"},
		{Key: "Custom_Greeting", Kind: "static", Value: "Hello"},
	}

	got, err := Resolve(context.Background(), "{{Custom_Greeting}} from {{Source_Country}} to {{Destination_Country}} in {{Current_Year}}", rc, tokens)
	if err != nil {
		t.Fatal(err)
	}

	want := "Hello from Nigeria to the UK in 2026"
	if got != want {
		t.Fatalf("got %q; want %q", got, want)
	}
}

func TestResolveExpression(t *testing.T) {
	t.Parallel()

	rc := RenderContext{Now: time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC), DestinationCountry: "Nigeria"}

	got, _ := Resolve(context.Background(), "Next year: {{year(Now) + 1}}, adjective: {{adjective(Destination_Country)}}", rc, nil)
	if got != "Next year: 2027, adjective: Nigerian" {
		t.Fatalf("got %q; want %q", got, "Next year: 2027, adjective: Nigerian")
	}
}

func TestResolveFallback(t *testing.T) {
	t.Parallel()

	rc := RenderContext{Now: time.Now()}

	got, _ := Resolve(context.Background(), "{{Unknown|fallback value}}", rc, nil)
	if got != "fallback value" {
		t.Fatalf("got %q; want fallback value", got)
	}
}

func TestResolveStatic(t *testing.T) {
	t.Parallel()

	rc := RenderContext{}
	tokens := []TemplateLiteral{{Key: "Brand", Kind: "static", Value: "Japa"}}

	got, _ := Resolve(context.Background(), "Welcome to {{Brand}}", rc, tokens)
	if got != "Welcome to Japa" {
		t.Fatalf("got %q; want Welcome to Japa", got)
	}
}

func TestResolveNoTemplates(t *testing.T) {
	t.Parallel()

	rc := RenderContext{}

	got, _ := Resolve(t.Context(), "plain text", rc, nil)
	if got != "plain text" {
		t.Fatalf("got %q; want plain text", got)
	}
}

func TestResolveBuiltInVariableFallback(t *testing.T) {
	t.Parallel()

	rc := RenderContext{Now: time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)}

	got, _ := Resolve(t.Context(), "Year: {{Current_Year}}", rc, nil)
	if got != "Year: 2026" {
		t.Fatalf("got %q; want %q", got, "Year: 2026")
	}
}

func TestResolveStaticReferencesToken(t *testing.T) {
	t.Parallel()

	rc := RenderContext{Now: time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)}
	tokens := []TemplateLiteral{
		{Key: "Current_Year", Kind: "expression", Value: "year(Now)"},
		{Key: "Greeting", Kind: "static", Value: "Hello in {{Current_Year}}"},
	}

	got, _ := Resolve(t.Context(), "{{Greeting}}!", rc, tokens)
	if got != "Hello in 2026!" {
		t.Fatalf("got %q; want %q", got, "Hello in 2026!")
	}
}

func TestResolveStaticCycle(t *testing.T) {
	t.Parallel()

	rc := RenderContext{}
	tokens := []TemplateLiteral{
		{Key: "A", Kind: "static", Value: "{{B}}"},
		{Key: "B", Kind: "static", Value: "{{A}}"},
	}

	got, _ := Resolve(t.Context(), "{{A}}", rc, tokens)
	// Cycles should not panic and should leave the cyclic token unchanged.
	if got != "{{A}}" {
		t.Fatalf("got %q; want %q", got, "{{A}}")
	}
}

func TestIsReservedKey(t *testing.T) {
	t.Parallel()

	cases := []struct {
		key  string
		want bool
	}{
		{key: "Current_Year", want: true},
		{key: "adjective", want: true},
		{key: "Custom_Token", want: false},
	}

	for _, tc := range cases {
		if got := IsReservedKey(tc.key); got != tc.want {
			t.Fatalf("IsReservedKey(%q) = %v; want %v", tc.key, got, tc.want)
		}
	}
}
