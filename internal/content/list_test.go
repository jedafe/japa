package content

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestValidateListFieldValues(t *testing.T) {
	cases := []struct {
		name    string
		fv      string
		wantErr string // expected code; "" = valid
	}{
		{"empty", ``, ""},
		{"empty object", `{}`, ""},
		{"numbered ok", `{"list_mode":"numbered","items":[{"heading":"A"}]}`, ""},
		{"bad mode", `{"list_mode":"banana","items":[]}`, "invalid_list_mode"},
		{"items not array", `{"items":{}}`, "invalid_list_items"},
		{"bad media uuid", `{"items":[{"media_id":"nope"}]}`, "invalid_list_items"},
		{"media and embed", `{"items":[{"media_id":"` + uuid.New().String() + `","embed":{"kind":"url","url":"https://x.co"}}]}`, "invalid_list_items"},
		{"bad embed kind", `{"items":[{"embed":{"kind":"portal"}}]}`, "invalid_list_items"},
		{"content embed missing slug", `{"items":[{"embed":{"kind":"content","content_id":"` + uuid.New().String() + `"}}]}`, "invalid_list_items"},
		{"url embed bad scheme", `{"items":[{"embed":{"kind":"url","url":"javascript:alert(1)"}}]}`, "invalid_list_items"},
		{"bad source_url", `{"items":[{"source_url":"ftp://x"}]}`, "invalid_list_items"},
	}
	for _, tc := range cases {
		_, err := validateListFieldValues(json.RawMessage(tc.fv), 50)
		if tc.wantErr == "" && err != nil {
			t.Errorf("%s: unexpected error %v", tc.name, err)
		}
		if tc.wantErr != "" && (err == nil || err.Code != tc.wantErr) {
			t.Errorf("%s: want %s, got %v", tc.name, tc.wantErr, err)
		}
	}
}

func TestValidateListFieldValuesMaxItems(t *testing.T) {
	fv := `{"items":[{"heading":"a"},{"heading":"b"},{"heading":"c"}]}`
	if _, err := validateListFieldValues(json.RawMessage(fv), 2); err == nil || err.Code != "too_many_items" {
		t.Fatalf("want too_many_items, got %v", err)
	}
	if _, err := validateListFieldValues(json.RawMessage(fv), 3); err != nil {
		t.Fatalf("3 items at max 3 should pass, got %v", err)
	}
}

func TestListItemHTMLSanitized(t *testing.T) {
	fv := `{"items":[{"body_html":"<p onclick='x()'>hi</p><script>bad()</script>"}]}`
	out, err := validateListFieldValues(json.RawMessage(fv), 50)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out, []byte("onclick")) || bytes.Contains(out, []byte("script")) {
		t.Fatal("item body_html not sanitized")
	}
}

// ItemList JSON-LD: position numbers must match the DISPLAYED item number.
// Numbered/unnumbered lists show items in array order (items[0] = "#1");
// countdown lists display the last array element as "#1", so positions map
// to items in reverse (position p → items[len-p]).
func TestInjectListItemListElements(t *testing.T) {
	jsonLd := []byte(`{"@context":"https://schema.org","@type":"ItemList","name":"My list"}`)

	t.Run("numbered order preserved", func(t *testing.T) {
		fv := json.RawMessage(`{"list_mode":"numbered","items":[{"heading":"First"},{"heading":"Second"},{"heading":"Third"}]}`)
		doc := decodeJSONLD(t, injectListItemListElements(jsonLd, fv))

		els, ok := doc["itemListElement"].([]any)
		if !ok || len(els) != 3 {
			t.Fatalf("itemListElement = %v, want 3 elements", doc["itemListElement"])
		}
		for i, want := range []string{"First", "Second", "Third"} {
			el := els[i].(map[string]any)
			if el["position"] != float64(i+1) || el["name"] != want {
				t.Fatalf("element %d = %v, want position %d name %q", i, el, i+1, want)
			}
		}
		if doc["numberOfItems"] != float64(3) {
			t.Fatalf("numberOfItems = %v, want 3", doc["numberOfItems"])
		}
	})

	t.Run("countdown reverses positions", func(t *testing.T) {
		fv := json.RawMessage(`{"list_mode":"countdown","items":[{"heading":"Bronze"},{"heading":"Silver"},{"heading":"Gold"}]}`)
		doc := decodeJSONLD(t, injectListItemListElements(jsonLd, fv))

		els, ok := doc["itemListElement"].([]any)
		if !ok || len(els) != 3 {
			t.Fatalf("itemListElement = %v, want 3 elements", doc["itemListElement"])
		}
		// Position 1 is the top-ranked item — the LAST element of the array
		// (displayed as "#1" on the page). Position 3 is items[0] ("#3").
		want := []string{"Gold", "Silver", "Bronze"}
		for i, w := range want {
			el := els[i].(map[string]any)
			if el["position"] != float64(i+1) || el["name"] != w {
				t.Fatalf("element %d = %v, want position %d name %q", i, el, i+1, w)
			}
		}
	})

	t.Run("empty items returns input unchanged", func(t *testing.T) {
		fv := json.RawMessage(`{"list_mode":"numbered","items":[]}`)
		out := injectListItemListElements(jsonLd, fv)
		if !bytes.Equal(out, jsonLd) {
			t.Fatalf("expected unchanged input, got %s", out)
		}
	})

	t.Run("garbage field_values returns input unchanged", func(t *testing.T) {
		out := injectListItemListElements(jsonLd, json.RawMessage(`not json`))
		if !bytes.Equal(out, jsonLd) {
			t.Fatalf("expected unchanged input, got %s", out)
		}
	})
}

func decodeJSONLD(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("injected JSON-LD is not valid JSON: %v (%s)", err, raw)
	}
	return doc
}
