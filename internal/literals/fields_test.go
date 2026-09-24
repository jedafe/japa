package literals

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

var testTokens = []TemplateLiteral{
	{Key: "Platform_Name", Kind: "static", Value: "JapaConnect"},
	{Key: "Destination_Country", Kind: "expression", Value: "Destination_Country"},
}

func testRC() RenderContext {
	return RenderContext{
		Now:                time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC),
		DestinationCountry: "Canada",
		DestinationCode:    "CA",
		PlatformName:       "JapaConnect",
	}
}

func TestResolveCaseInsensitiveToken(t *testing.T) {
	t.Parallel()

	got, _ := Resolve(context.Background(), "{{current_year}} {{CURRENT_YEAR}} {{Current_Year}}", testRC(), nil)
	if got != "2026 2026 2026" {
		t.Fatalf("got %q; want %q", got, "2026 2026 2026")
	}
}

func TestResolveCaseInsensitiveCustomToken(t *testing.T) {
	t.Parallel()

	got, _ := Resolve(context.Background(), "{{platform_name}}", testRC(), testTokens)
	if got != "JapaConnect" {
		t.Fatalf("got %q; want %q", got, "JapaConnect")
	}
}

func TestResolveCaseInsensitiveExpressionVar(t *testing.T) {
	t.Parallel()

	got, _ := Resolve(context.Background(), "{{destination_country}} {{adjective(destination_country)}}", testRC(), nil)
	if got != "Canada Canadian" {
		t.Fatalf("got %q; want %q", got, "Canada Canadian")
	}
}

func TestResolveArithmeticAndConditional(t *testing.T) {
	t.Parallel()

	got, _ := Resolve(context.Background(), "{{Current_Year + 1}} {{if(Destination_Country != \"\", Destination_Country, \"nowhere\")}}", testRC(), nil)
	if got != "2027 Canada" {
		t.Fatalf("got %q; want %q", got, "2027 Canada")
	}
}

type testRow struct {
	Title      string
	Body       *string
	Excerpt    string
	Slug       string // not resolvable
	Ignored    int
	unexported string //nolint:unused
}

func TestResolveFieldsStruct(t *testing.T) {
	t.Parallel()

	// Structs carry entity-scoped geo: request geo is cleared by
	// applyContentGeo, so {{Destination_Country}} resolves empty and the
	// inline fallback wins. {{Current_Year}} is unaffected.
	body := "Move to {{Destination_Country|your destination}} in {{Current_Year}}"
	row := testRow{
		Title:   "Guide to {{Destination_Country|your destination}}",
		Body:    &body,
		Excerpt: "About {{Destination_Country|your destination}}",
		Slug:    "guide-{{literal}}",
	}

	if err := ResolveFields(context.Background(), &row, testRC(), testTokens); err != nil {
		t.Fatal(err)
	}
	if row.Title != "Guide to your destination" {
		t.Fatalf("Title = %q", row.Title)
	}
	if *row.Body != "Move to your destination in 2026" {
		t.Fatalf("Body = %q", *row.Body)
	}
	if row.Excerpt != "About your destination" {
		t.Fatalf("Excerpt = %q", row.Excerpt)
	}
	if row.Slug != "guide-{{literal}}" {
		t.Fatalf("Slug must stay raw; got %q", row.Slug)
	}
}

type testGeoRow struct {
	Title         string
	SourceCountry string
	TargetCountry string
}

func TestResolveFieldsContentGeoWins(t *testing.T) {
	t.Parallel()

	// A row declaring its own geo overrides (and never leaks) request geo.
	row := testGeoRow{Title: "From {{Source_Country}} to {{Destination_Country}}", SourceCountry: "Nigeria", TargetCountry: "GB"}
	if err := ResolveFields(context.Background(), &row, testRC(), testTokens); err != nil {
		t.Fatal(err)
	}
	if row.Title != "From Nigeria to United Kingdom" {
		t.Fatalf("Title = %q", row.Title)
	}
}

type testNested struct {
	Title string
	Item  testRow
	Items []testRow
}

func TestResolveFieldsNested(t *testing.T) {
	t.Parallel()

	n := testNested{
		Title: "Root {{Current_Year}}",
		Item:  testRow{Title: "Child {{Destination_Country|your destination}}"},
		Items: []testRow{{Title: "A {{Current_Year}}"}, {Title: "B {{Current_Year}}"}},
	}

	if err := ResolveFields(context.Background(), &n, testRC(), testTokens); err != nil {
		t.Fatal(err)
	}
	if n.Title != "Root 2026" || n.Item.Title != "Child your destination" || n.Items[0].Title != "A 2026" || n.Items[1].Title != "B 2026" {
		t.Fatalf("nested resolution failed: %+v", n)
	}
}

type testByteRow struct {
	Title   string
	Content []byte
}

func TestResolveFieldsByteSlice(t *testing.T) {
	t.Parallel()

	row := testByteRow{Title: "T {{Current_Year}}", Content: []byte(`{"type":"doc","text":"{{Destination_Country|your destination}}"}`)}
	if err := ResolveFields(context.Background(), &row, testRC(), testTokens); err != nil {
		t.Fatal(err)
	}
	if string(row.Content) != `{"type":"doc","text":"your destination"}` {
		t.Fatalf("Content = %q", row.Content)
	}
}

func TestResolveFieldsNilPointerMeta(t *testing.T) {
	t.Parallel()

	type meta struct {
		MetaTitle *string
	}
	m := meta{}
	if err := ResolveFields(context.Background(), &m, testRC(), testTokens); err != nil {
		t.Fatal(err)
	}
	if m.MetaTitle != nil {
		t.Fatalf("nil MetaTitle must stay nil; got %q", *m.MetaTitle)
	}
}

func TestResolveDataMap(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"title":   "Welcome to {{Platform_Name}}",
		"count":   3,
		"items":   []any{"in {{Current_Year}}", map[string]any{"name": "{{Destination_Country}}"}},
		"content": []byte(`body {{Current_Year}}`),
	}

	ResolveData(context.Background(), payload, testRC(), testTokens)

	if payload["title"] != "Welcome to JapaConnect" {
		t.Fatalf("title = %v", payload["title"])
	}
	items := payload["items"].([]any)
	if items[0] != "in 2026" {
		t.Fatalf("items[0] = %v", items[0])
	}
	if items[1].(map[string]any)["name"] != "Canada" {
		t.Fatalf("nested name = %v", items[1])
	}
	if string(payload["content"].([]byte)) != "body 2026" {
		t.Fatalf("content = %v", payload["content"])
	}
	if payload["count"] != 3 {
		t.Fatalf("count mutated: %v", payload["count"])
	}
}

func TestResolveDataUnmarshalledJSON(t *testing.T) {
	t.Parallel()

	var payload map[string]any
	if err := json.Unmarshal([]byte(`{"title":"{{Platform_Name}} in {{Current_Year}}","items":[{"label":"{{Destination_Country}}"}]}`), &payload); err != nil {
		t.Fatal(err)
	}

	ResolveData(context.Background(), payload, testRC(), testTokens)

	if payload["title"] != "JapaConnect in 2026" {
		t.Fatalf("title = %v", payload["title"])
	}
	if payload["items"].([]any)[0].(map[string]any)["label"] != "Canada" {
		t.Fatalf("label = %v", payload["items"])
	}
}

func TestResolveFieldsSuffixNames(t *testing.T) {
	t.Parallel()

	type Card struct {
		Heading         string `json:"heading"`
		Subject         string `json:"subject"`
		ItemTitle       string `json:"item_title"`
		Question        string `json:"question"`
		Tagline         string `json:"tagline"`
		RejectionReason string `json:"rejection_reason"`
		TitleID         string `json:"title_id"`
		Username        string `json:"username"`
		FileName        string `json:"file_name"`
		Prompt          string `json:"prompt"`
	}

	c := &Card{
		Heading:         "Heading {{Current_Year}}",
		Subject:         "About {{Platform_Name}}",
		ItemTitle:       "Item {{Current_Year}}",
		Question:        "Why {{Platform_Name}}?",
		Tagline:         "Best of {{Current_Year}}",
		RejectionReason: "Rejected {{Current_Year}}",
		TitleID:         "{{keep}}",
		Username:        "{{keep}}",
		FileName:        "{{keep}}.pdf",
		Prompt:          "{{keep}}",
	}

	ResolveFields(context.Background(), c, testRC(), testTokens)

	if c.Heading != "Heading 2026" {
		t.Fatalf("Heading = %q", c.Heading)
	}
	if c.Subject != "About JapaConnect" {
		t.Fatalf("Subject = %q", c.Subject)
	}
	if c.ItemTitle != "Item 2026" {
		t.Fatalf("ItemTitle = %q", c.ItemTitle)
	}
	if c.Question != "Why JapaConnect?" {
		t.Fatalf("Question = %q", c.Question)
	}
	if c.Tagline != "Best of 2026" {
		t.Fatalf("Tagline = %q", c.Tagline)
	}
	if c.RejectionReason != "Rejected 2026" {
		t.Fatalf("RejectionReason = %q", c.RejectionReason)
	}

	// Identifier/config fields must stay untouched.
	for name, got := range map[string]string{
		"TitleID":  c.TitleID,
		"Username": c.Username,
		"FileName": c.FileName,
		"Prompt":   c.Prompt,
	} {
		if got != map[string]string{
			"TitleID":  "{{keep}}",
			"Username": "{{keep}}",
			"FileName": "{{keep}}.pdf",
			"Prompt":   "{{keep}}",
		}[name] {
			t.Fatalf("%s = %q; want raw", name, got)
		}
	}
}

func TestResolveFieldsNamedStringSlice(t *testing.T) {
	t.Parallel()

	type FAQ struct {
		Answers []string `json:"answers"`
		Tags    []string `json:"tags"`
	}

	f := &FAQ{
		Answers: []string{"In {{Current_Year}}", "Yes"},
		Tags:    []string{"{{keep}}"},
	}

	ResolveFields(context.Background(), f, testRC(), testTokens)

	if f.Answers[0] != "In 2026" || f.Answers[1] != "Yes" {
		t.Fatalf("Answers = %v", f.Answers)
	}
	if f.Tags[0] != "{{keep}}" {
		t.Fatalf("Tags = %v", f.Tags)
	}
}

func TestResolveFieldsEmbeddedSuffix(t *testing.T) {
	t.Parallel()

	type Inner struct {
		ItemHeading string `json:"item_heading"`
	}
	type Outer struct {
		Inner
		Slug string `json:"slug"`
	}

	o := &Outer{Inner: Inner{ItemHeading: "Head {{Current_Year}}"}, Slug: "{{keep}}"}
	ResolveFields(context.Background(), o, testRC(), testTokens)

	if o.ItemHeading != "Head 2026" {
		t.Fatalf("ItemHeading = %q", o.ItemHeading)
	}
	if o.Slug != "{{keep}}" {
		t.Fatalf("Slug = %q", o.Slug)
	}
}
