package content

import (
	"bytes"
	"encoding/json"
	"strconv"
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
		{"items_per_page ok", `{"items_per_page":5,"items":[]}`, ""},
		{"items_per_page one", `{"items_per_page":1,"items":[]}`, ""},
		{"items_per_page zero", `{"items_per_page":0,"items":[]}`, "invalid_items_per_page"},
		{"items_per_page negative", `{"items_per_page":-2,"items":[]}`, "invalid_items_per_page"},
		{"items_per_page float", `{"items_per_page":2.5,"items":[]}`, "invalid_items_per_page"},
		{"items_per_page string", `{"items_per_page":"5","items":[]}`, "invalid_items_per_page"},
		{"items_per_page over max", `{"items_per_page":51,"items":[]}`, "invalid_items_per_page"},
	}
	for _, tc := range cases {
		_, err := validateListFieldValues(json.RawMessage(tc.fv), 50, nil)
		if tc.wantErr == "" && err != nil {
			t.Errorf("%s: unexpected error %v", tc.name, err)
		}

		if tc.wantErr != "" && (err == nil || err.Code != tc.wantErr) {
			t.Errorf("%s: want %s, got %v", tc.name, tc.wantErr, err)
		}
	}
}

func TestValidateListFieldValuesItemsPerPage(t *testing.T) {
	// null disables pagination — the key is normalized out of field_values.
	out, err := validateListFieldValues(json.RawMessage(`{"items_per_page":null,"items":[]}`), 50, nil)
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Contains(out, []byte("items_per_page")) {
		t.Fatalf("null items_per_page should be dropped, got %s", out)
	}

	// A valid bound is normalized to a plain integer for stable round-trips.
	out, err = validateListFieldValues(json.RawMessage(`{"items_per_page":3,"items":[]}`), 50, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Contains(out, []byte(`"items_per_page":3`)) {
		t.Fatalf("want normalized items_per_page:3, got %s", out)
	}
}

func TestValidateListFieldValuesItemsPerPageGrandfather(t *testing.T) {
	// §14 grandfather rule: a PATCH resubmitting the stored items_per_page
	// passes even when list.max_items was lowered below it — the author
	// isn't touching the bound, so unrelated edits must not strand.
	prev := json.RawMessage(`{"items_per_page":40,"items":[{"heading":"a"},{"heading":"b"},{"heading":"c"},{"heading":"d"},{"heading":"e"},{"heading":"f"},{"heading":"g"},{"heading":"h"},{"heading":"i"},{"heading":"j"},{"heading":"k"},{"heading":"l"}]}`)
	if _, err := validateListFieldValues(json.RawMessage(`{"items_per_page":40,"items":[]}`), 10, prev); err != nil {
		t.Fatalf("unchanged stored value should pass under a lowered cap, got %v", err)
	}

	// A NEW over-cap value still fails — the grandfather only covers the
	// exact stored number.
	if _, err := validateListFieldValues(json.RawMessage(`{"items_per_page":39,"items":[]}`), 10, prev); err == nil || err.Code != "invalid_items_per_page" {
		t.Fatalf("changed over-cap value should fail, got %v", err)
	}

	// The same rule covers the items array: 12 stored items under a cap of
	// 10 may resubmit or shrink, but not grow.
	sameCount := `{"items":[{"heading":"a"},{"heading":"b"},{"heading":"c"},{"heading":"d"},{"heading":"e"},{"heading":"f"},{"heading":"g"},{"heading":"h"},{"heading":"i"},{"heading":"j"},{"heading":"k"},{"heading":"l"}]}`
	if _, err := validateListFieldValues(json.RawMessage(sameCount), 10, prev); err != nil {
		t.Fatalf("stored item count should pass under a lowered cap, got %v", err)
	}
	if _, err := validateListFieldValues(json.RawMessage(`{"items":[{"heading":"a"},{"heading":"b"},{"heading":"c"},{"heading":"d"},{"heading":"e"},{"heading":"f"},{"heading":"g"},{"heading":"h"},{"heading":"i"},{"heading":"j"},{"heading":"k"}]}`), 10, prev); err != nil {
		t.Fatalf("shrinking toward the cap should pass, got %v", err)
	}
	grown := `{"items":[{"heading":"a"},{"heading":"b"},{"heading":"c"},{"heading":"d"},{"heading":"e"},{"heading":"f"},{"heading":"g"},{"heading":"h"},{"heading":"i"},{"heading":"j"},{"heading":"k"},{"heading":"l"},{"heading":"m"}]}`
	if _, err := validateListFieldValues(json.RawMessage(grown), 10, prev); err == nil || err.Code != "too_many_items" {
		t.Fatalf("growing past the cap should fail, got %v", err)
	}
}

// §14: field_values PATCH is per-key — the stored object is merged under
// the submitted one so a PATCH carrying only items_per_page cannot wipe
// items/list_mode (the column itself is still replaced wholesale).
func TestValidateListFieldValuesPatchMerge(t *testing.T) {
	prev := json.RawMessage(`{"list_mode":"numbered","items_per_page":4,"custom_key":"keep","items":[{"heading":"a"},{"heading":"b"}]}`)

	// ipp-only patch: items, mode, and foreign keys all survive.
	out, err := validateListFieldValues(json.RawMessage(`{"items_per_page":3}`), 50, prev)
	if err != nil {
		t.Fatalf("ipp-only patch should validate, got %v", err)
	}
	for _, want := range []string{`"items_per_page":3`, `"list_mode":"numbered"`, `"custom_key":"keep"`, `"heading":"a"`, `"heading":"b"`} {
		if !bytes.Contains(out, []byte(want)) {
			t.Fatalf("merged output missing %s: %s", want, out)
		}
	}

	// Explicit null disables pagination but keeps everything else.
	out, err = validateListFieldValues(json.RawMessage(`{"items_per_page":null}`), 50, prev)
	if err != nil {
		t.Fatalf("null ipp patch should validate, got %v", err)
	}
	if bytes.Contains(out, []byte("items_per_page")) {
		t.Fatalf("null items_per_page should be dropped, got %s", out)
	}
	if !bytes.Contains(out, []byte(`"heading":"a"`)) || !bytes.Contains(out, []byte(`"list_mode":"numbered"`)) {
		t.Fatalf("null ipp patch should keep items/mode, got %s", out)
	}

	// An explicit items array replaces wholesale — no element-wise merge.
	out, err = validateListFieldValues(json.RawMessage(`{"items":[{"heading":"z"}]}`), 50, prev)
	if err != nil {
		t.Fatalf("items patch should validate, got %v", err)
	}
	if !bytes.Contains(out, []byte(`"heading":"z"`)) || bytes.Contains(out, []byte(`"heading":"a"`)) {
		t.Fatalf("items should replace wholesale, got %s", out)
	}
	if !bytes.Contains(out, []byte(`"items_per_page":4`)) {
		t.Fatalf("items patch should keep stored items_per_page, got %s", out)
	}

	// Explicit null on items clears to the normalized empty array.
	out, err = validateListFieldValues(json.RawMessage(`{"items":null}`), 50, prev)
	if err != nil {
		t.Fatalf("null items patch should validate, got %v", err)
	}
	if !bytes.Contains(out, []byte(`"items":[]`)) {
		t.Fatalf("null items should normalize to [], got %s", out)
	}

	// Corrupt stored fv falls back to the patch alone.
	out, err = validateListFieldValues(json.RawMessage(`{"items_per_page":2}`), 50, json.RawMessage(`{bad`))
	if err != nil {
		t.Fatalf("corrupt prev should not fail validation, got %v", err)
	}
	if !bytes.Contains(out, []byte(`"items_per_page":2`)) {
		t.Fatalf("want items_per_page:2, got %s", out)
	}
}

func TestValidateListFieldValuesMaxItems(t *testing.T) {
	fv := `{"items":[{"heading":"a"},{"heading":"b"},{"heading":"c"}]}`
	if _, err := validateListFieldValues(json.RawMessage(fv), 2, nil); err == nil || err.Code != "too_many_items" {
		t.Fatalf("want too_many_items, got %v", err)
	}

	if _, err := validateListFieldValues(json.RawMessage(fv), 3, nil); err != nil {
		t.Fatalf("3 items at max 3 should pass, got %v", err)
	}

	// A non-positive cap is a misconfiguration, not a zero-item rule —
	// validation falls back to the default bound.
	if _, err := validateListFieldValues(json.RawMessage(fv), 0, nil); err != nil {
		t.Fatalf("maxItems=0 should fall back to default, got %v", err)
	}
	if _, err := validateListFieldValues(json.RawMessage(`{"items_per_page":5}`), -3, nil); err != nil {
		t.Fatalf("negative maxItems should fall back to default, got %v", err)
	}
}

func TestListItemHTMLSanitized(t *testing.T) {
	fv := `{"items":[{"body_html":"<p onclick='x()'>hi</p><script>bad()</script>"}]}`

	out, err := validateListFieldValues(json.RawMessage(fv), 50, nil)
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
	const itemBase = "https://example.com/list/my-list"

	t.Run("numbered order preserved", func(t *testing.T) {
		fv := json.RawMessage(`{"list_mode":"numbered","items":[{"heading":"First"},{"heading":"Second"},{"heading":"Third"}]}`)
		doc := decodeJSONLD(t, injectListItemListElements(jsonLd, fv, itemBase))

		els, ok := doc["itemListElement"].([]any)
		if !ok || len(els) != 3 {
			t.Fatalf("itemListElement = %v, want 3 elements", doc["itemListElement"])
		}

		for i, want := range []string{"First", "Second", "Third"} {
			el := els[i].(map[string]any)
			wantURL := "https://example.com/list/my-list#item-" + strconv.Itoa(i+1)
			if el["position"] != float64(i+1) || el["name"] != want || el["url"] != wantURL {
				t.Fatalf("element %d = %v, want position %d name %q url %q", i, el, i+1, want, wantURL)
			}
		}

		if doc["numberOfItems"] != float64(3) {
			t.Fatalf("numberOfItems = %v, want 3", doc["numberOfItems"])
		}
	})

	t.Run("countdown reverses positions", func(t *testing.T) {
		fv := json.RawMessage(`{"list_mode":"countdown","items":[{"heading":"Bronze"},{"heading":"Silver"},{"heading":"Gold"}]}`)
		doc := decodeJSONLD(t, injectListItemListElements(jsonLd, fv, itemBase))

		els, ok := doc["itemListElement"].([]any)
		if !ok || len(els) != 3 {
			t.Fatalf("itemListElement = %v, want 3 elements", doc["itemListElement"])
		}
		// Position 1 is the top-ranked item — the LAST element of the array
		// (displayed as "#1" on the page). Position 3 is items[0] ("#3").
		want := []string{"Gold", "Silver", "Bronze"}
		for i, w := range want {
			el := els[i].(map[string]any)
			// position 1 (top rank) anchors the LAST DOM item — #item-3.
			wantURL := "https://example.com/list/my-list#item-" + strconv.Itoa(len(want)-i)
			if el["position"] != float64(i+1) || el["name"] != w || el["url"] != wantURL {
				t.Fatalf("element %d = %v, want position %d name %q url %q", i, el, i+1, w, wantURL)
			}
		}
	})

	t.Run("empty items returns input unchanged", func(t *testing.T) {
		fv := json.RawMessage(`{"list_mode":"numbered","items":[]}`)

		out := injectListItemListElements(jsonLd, fv, itemBase)
		if !bytes.Equal(out, jsonLd) {
			t.Fatalf("expected unchanged input, got %s", out)
		}
	})

	t.Run("garbage field_values returns input unchanged", func(t *testing.T) {
		out := injectListItemListElements(jsonLd, json.RawMessage(`not json`), itemBase)
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
