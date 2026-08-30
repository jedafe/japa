package literals

import (
	"context"
	"testing"
	"time"
)

func TestResolve(t *testing.T) {
	rc := RenderContext{
		Now:                time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC),
		SourceCountry:      "Nigeria",
		SourceCode:         "NG",
		DestinationCountry: "the UK",
		DestinationCode:    "GB",
		TenantName:         "JapaConnect",
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
	rc := RenderContext{Now: time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC), DestinationCountry: "Nigeria"}
	got, _ := Resolve(context.Background(), "Next year: {{year(Now) + 1}}, adjective: {{adjective(Destination_Country)}}", rc, nil)
	want := "Next year: 2027, adjective: Nigerian"
	if got != want {
		t.Fatalf("got %q; want %q", got, want)
	}
}

func TestResolveFallback(t *testing.T) {
	rc := RenderContext{Now: time.Now()}
	got, _ := Resolve(context.Background(), "{{Unknown|fallback value}}", rc, nil)
	if got != "fallback value" {
		t.Fatalf("got %q; want fallback value", got)
	}
}

func TestResolveStatic(t *testing.T) {
	rc := RenderContext{}
	tokens := []TemplateLiteral{{Key: "Brand", Kind: "static", Value: "Japa"}}
	got, _ := Resolve(context.Background(), "Welcome to {{Brand}}", rc, tokens)
	if got != "Welcome to Japa" {
		t.Fatalf("got %q; want Welcome to Japa", got)
	}
}

func TestResolveNoTemplates(t *testing.T) {
	rc := RenderContext{}
	got, _ := Resolve(context.Background(), "plain text", rc, nil)
	if got != "plain text" {
		t.Fatalf("got %q; want plain text", got)
	}
}
