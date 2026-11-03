package content

import (
	"bytes"
	"strings"
	"testing"
)

// §45 editor FAQ block — the accordion markup the TipTap faq node emits.
// Mirrors the shape asserted in sanitize_test.go.
const faqBodyHTML = `<div data-type="faq" class="faq-block">` +
	`<details name="jc-faq" data-type="faq-item" class="faq-item">` +
	`<summary data-type="faq-question" class="faq-question">How long does a visa take?</summary>` +
	`<div data-type="faq-answer" class="faq-answer"><p>3–10 <strong>business</strong> days.</p></div>` +
	`</details>` +
	`<details name="jc-faq" data-type="faq-item" class="faq-item">` +
	`<summary data-type="faq-question" class="faq-question">Can I <em>work</em> remotely?</summary>` +
	`<div data-type="faq-answer" class="faq-answer"><p>Yes, with a <a href="https://example.com/permit">permit</a>.</p></div>` +
	`</details></div>`

func TestExtractFaqItems(t *testing.T) {
	items := extractFaqItems(faqBodyHTML)
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d", len(items))
	}

	if items[0].question != "How long does a visa take?" {
		t.Fatalf("question = %q", items[0].question)
	}

	if !strings.Contains(items[0].answer, "<strong>business</strong>") {
		t.Fatalf("answer lost inline markup: %q", items[0].answer)
	}

	if items[1].question != "Can I work remotely?" {
		t.Fatalf("question should strip inline marks to text, got %q", items[1].question)
	}

	if !strings.Contains(items[1].answer, `href="https://example.com/permit"`) {
		t.Fatalf("answer lost link markup: %q", items[1].answer)
	}
}

func TestExtractFaqItemsSkipsIncomplete(t *testing.T) {
	body := `<div data-type="faq">` +
		`<details data-type="faq-item"><summary data-type="faq-question">No answer?</summary></details>` +
		`<details data-type="faq-item"><summary data-type="faq-question">Good?</summary><div data-type="faq-answer"><p>Yes.</p></div></details>` +
		`</div>`

	items := extractFaqItems(body)
	if len(items) != 1 || items[0].question != "Good?" {
		t.Fatalf("want only the complete pair, got %+v", items)
	}
}

func TestInjectFaqPageJSONLD(t *testing.T) {
	article := []byte(`{"@context":"https://schema.org","@type":"Article","headline":"Guide"}`)

	t.Run("merges under @graph with resolved template", func(t *testing.T) {
		doc := decodeJSONLD(t, InjectFaqPageJSONLD(article, faqBodyHTML, ""))

		if doc["@context"] != "https://schema.org" {
			t.Fatalf("@context = %v", doc["@context"])
		}

		graph, ok := doc["@graph"].([]any)
		if !ok || len(graph) != 2 {
			t.Fatalf("@graph = %v, want 2 nodes", doc["@graph"])
		}

		if graph[0].(map[string]any)["@type"] != "Article" {
			t.Fatalf("first graph node should be the Article, got %v", graph[0])
		}

		faq := graph[1].(map[string]any)
		if faq["@type"] != "FAQPage" {
			t.Fatalf("second graph node @type = %v", faq["@type"])
		}

		entities, ok := faq["mainEntity"].([]any)
		if !ok || len(entities) != 2 {
			t.Fatalf("mainEntity = %v, want 2 Questions", faq["mainEntity"])
		}

		q := entities[0].(map[string]any)
		if q["@type"] != "Question" || q["name"] != "How long does a visa take?" {
			t.Fatalf("question node = %v", q)
		}

		ans := q["acceptedAnswer"].(map[string]any)
		if ans["@type"] != "Answer" || !strings.Contains(ans["text"].(string), "business") {
			t.Fatalf("answer node = %v", ans)
		}
	})

	t.Run("standalone FAQPage when no template resolved", func(t *testing.T) {
		doc := decodeJSONLD(t, InjectFaqPageJSONLD(nil, faqBodyHTML, ""))

		if doc["@type"] != "FAQPage" || doc["@context"] != "https://schema.org" {
			t.Fatalf("standalone doc = %v", doc)
		}
	})

	t.Run("appends to an existing @graph", func(t *testing.T) {
		in := []byte(`{"@context":"https://schema.org","@graph":[{"@type":"BreadcrumbList"}]}`)
		doc := decodeJSONLD(t, InjectFaqPageJSONLD(in, faqBodyHTML, ""))

		graph := doc["@graph"].([]any)
		if len(graph) != 2 || graph[1].(map[string]any)["@type"] != "FAQPage" {
			t.Fatalf("@graph = %v", doc["@graph"])
		}
	})

	t.Run("no faq markup returns input unchanged", func(t *testing.T) {
		out := InjectFaqPageJSONLD(article, `<p>no faq here</p>`, "")
		if !bytes.Equal(out, article) {
			t.Fatalf("expected unchanged input, got %s", out)
		}
	})

	t.Run("empty body and empty jsonLd returns nil", func(t *testing.T) {
		out := InjectFaqPageJSONLD(nil, "", "")
		if len(out) != 0 {
			t.Fatalf("expected empty output, got %s", out)
		}
	})

	t.Run("markdown body fallback", func(t *testing.T) {
		// The faq node's renderMarkdown emits the same data-type markup
		// inside the markdown body — parse it when body_html is empty.
		mdBody := "Intro text.\n\n" + faqBodyHTML
		doc := decodeJSONLD(t, InjectFaqPageJSONLD(nil, "", mdBody))
		if doc["@type"] != "FAQPage" {
			t.Fatalf("doc = %v", doc)
		}
	})

	t.Run("rendered body_html is authoritative — raw body markers ignored", func(t *testing.T) {
		// Goldmark drops raw HTML from markdown bodies (fenced code
		// samples, hand-authored blocks) — markup in `body` that never
		// reached body_html is invisible to readers and must not produce
		// structured data.
		out := InjectFaqPageJSONLD(article, `<p>rendered prose, no accordion</p>`, faqBodyHTML)
		if !bytes.Equal(out, article) {
			t.Fatalf("expected unchanged input, got %s", out)
		}
	})

	t.Run("resolved null emits standalone FAQPage", func(t *testing.T) {
		doc := decodeJSONLD(t, InjectFaqPageJSONLD([]byte(`null`), faqBodyHTML, ""))
		if doc["@type"] != "FAQPage" {
			t.Fatalf("doc = %v — null resolved doc must not wrap into @graph", doc)
		}
	})

	t.Run("existing FAQPage in template is not duplicated", func(t *testing.T) {
		in := []byte(`{"@context":"https://schema.org","@graph":[{"@type":"Article"},{"@type":"FAQPage"}]}`)

		out := InjectFaqPageJSONLD(in, faqBodyHTML, "")
		if !bytes.Equal(out, in) {
			t.Fatalf("expected unchanged input, got %s", out)
		}
	})

	t.Run("top-level FAQPage template is not duplicated", func(t *testing.T) {
		in := []byte(`{"@context":"https://schema.org","@type":"FAQPage","mainEntity":[]}`)

		out := InjectFaqPageJSONLD(in, faqBodyHTML, "")
		if !bytes.Equal(out, in) {
			t.Fatalf("expected unchanged input, got %s", out)
		}
	})

	t.Run("empty answer skips the item", func(t *testing.T) {
		body := `<details data-type="faq-item"><summary data-type="faq-question">Q?</summary><div data-type="faq-answer"><p></p></div></details>` +
			`<details data-type="faq-item"><summary data-type="faq-question">Real?</summary><div data-type="faq-answer"><p>A.</p></div></details>`

		items := extractFaqItems(body)
		if len(items) != 1 || items[0].question != "Real?" {
			t.Fatalf("want only the non-empty answer pair, got %+v", items)
		}
	})
}
