package content

import (
	"encoding/json"
	"testing"
)

func TestListItemMediaRefs_PositionsAndDedup(t *testing.T) {
	fv := []byte(`{"items":[
		{"heading":"a","media_id":"11111111-1111-1111-1111-111111111111"},
		{"heading":"b"},
		{"heading":"c","media_id":"22222222-2222-2222-2222-222222222222"},
		{"heading":"d","media_id":"not-a-uuid"},
		{"heading":"e","media_id":"11111111-1111-1111-1111-111111111111"}
	]}`)

	refs := listItemMediaRefs(fv)
	if len(refs) != 3 {
		t.Fatalf("refs = %d, want 3 (empty, invalid skipped; same media twice tracked per slot)", len(refs))
	}

	if refs[0].Position != 0 || refs[1].Position != 2 || refs[2].Position != 4 {
		t.Fatalf("positions = %d,%d,%d, want 0,2,4 (item indexes)", refs[0].Position, refs[1].Position, refs[2].Position)
	}

	for i, r := range refs {
		if r.FieldKey != "item_media" {
			t.Fatalf("refs[%d].FieldKey = %q, want item_media", i, r.FieldKey)
		}
	}
}

func TestListItemMediaRefs_Malformed(t *testing.T) {
	if refs := listItemMediaRefs([]byte("not json")); refs != nil {
		t.Fatalf("malformed field_values should yield nil, got %v", refs)
	}
	if refs := listItemMediaRefs([]byte(`{}`)); refs != nil {
		t.Fatalf("no items should yield nil, got %v", refs)
	}
}

func TestMediaServeURL_Extraction(t *testing.T) {
	cases := []struct {
		name, text, bucket, path string
	}{
		{"tiptap src", `{"type":"image","attrs":{"src":"/api/v1/media/serve/blog/2026/photo.webp"}}`, "blog", "2026/photo.webp"},
		{"absolute url", `<img src="https://japa.to/api/v1/media/serve/avatars/u/1.png">`, "avatars", "u/1.png"},
		{"markdown", `![alt](/api/v1/media/serve/media/x/y-z_9.jpg)`, "media", "x/y-z_9.jpg"},
		{"url-encoded", `/api/v1/media/serve/media/a%20b/c.png`, "media", "a%20b/c.png"},
	}

	for _, tc := range cases {
		m := mediaServeURL.FindStringSubmatch(tc.text)
		if m == nil {
			t.Fatalf("%s: no match in %q", tc.name, tc.text)
		}
		if m[1] != tc.bucket || m[2] != tc.path {
			t.Fatalf("%s: got bucket=%q path=%q, want %q/%q", tc.name, m[1], m[2], tc.bucket, tc.path)
		}
	}
}

func TestMediaServeURL_NoMatch(t *testing.T) {
	for _, text := range []string{
		`external https://cdn.other.com/x/y.png`,
		`no media at all`,
		`media/serve/` + "\n" + `broken split`,
	} {
		if m := mediaServeURL.FindStringSubmatch(text); m != nil {
			t.Fatalf("unexpected match %v in %q", m, text)
		}
	}
}

func TestMediaValueIDs_Extraction(t *testing.T) {
	// The documented storage shapes — the structured walk must pull the id
	// out of every one of them.
	for _, raw := range []string{
		`"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"`,
		`{"value":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"}`,
		`{"id":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"}`,
		`{"media_id":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"}`,
		`["aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa","bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"]`,
		`{"media_ids":["aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"]}`,
		`[{"id":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa","alt":"x"}]`,
	} {
		ids := mediaValueIDs(json.RawMessage(raw))
		if len(ids) == 0 {
			t.Fatalf("no id extracted from %s", raw)
		}
		if ids[0].String() != "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" {
			t.Fatalf("first id = %s, want the documented id", ids[0])
		}
	}
}

func TestMediaValueIDs_RejectsNonMediaShapes(t *testing.T) {
	// UUID-looking strings in unrelated keys are not media references.
	for _, raw := range []string{
		`{"label":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"}`,
		`{"description":"see aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"}`,
		`{"foreign_ref":{"entity":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"}}`,
		`12345`,
		`null`,
	} {
		if ids := mediaValueIDs(json.RawMessage(raw)); len(ids) != 0 {
			t.Fatalf("expected no ids from %s, got %v", raw, ids)
		}
	}
}
