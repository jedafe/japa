package federation

import "sort"

// Minimal ActivityStreams JSON handling. Remote payloads are deliberately
// parsed as generic maps — AP documents vary wildly across Mastodon,
// GoToSocial, Misskey, Threads and friends, and a strict struct would
// reject conformant traffic over an unexpected field shape.

// apStr returns v as a string when it is one — object references in AP
// may be a bare IRI string or an embedded object.
func apStr(v any) string {
	s, _ := v.(string)
	return s
}

// apID extracts an object's @id: bare IRIs return themselves, embedded
// objects return their "id" field.
func apID(v any) string {
	switch o := v.(type) {
	case string:
		return o
	case map[string]any:
		return apStr(o["id"])
	}
	return ""
}

// apObj returns v as a map when it's an embedded object.
func apObj(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

// apType extracts an activity or object's "type".
func apType(v any) string {
	switch o := v.(type) {
	case map[string]any:
		return apStr(o["type"])
	}
	return ""
}

// apLocalizedText reads a text-bearing AP field — `field`, falling back
// to the first non-empty `<field>Map` value for servers that ship
// localized maps instead (contentMap, summaryMap). Map iteration is
// nondeterministic, so keys are sorted before selection.
func apLocalizedText(obj map[string]any, field string) string {
	if v := apStr(obj[field]); v != "" {
		return v
	}
	m, _ := obj[field+"Map"].(map[string]any)
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if v := apStr(m[k]); v != "" {
			return v
		}
	}
	return ""
}

// apContentText returns the object's HTML body (content / contentMap).
func apContentText(obj map[string]any) string { return apLocalizedText(obj, "content") }

// apActorID resolves an activity's actor — conventionally a bare IRI but
// some implementations embed the actor object.
func apActorID(act map[string]any) string { return apID(act["actor"]) }

// apObjectID resolves an activity's object reference the same way.
func apObjectID(act map[string]any) string { return apID(act["object"]) }
