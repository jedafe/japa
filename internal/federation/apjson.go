package federation

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

// apActorID resolves an activity's actor — conventionally a bare IRI but
// some implementations embed the actor object.
func apActorID(act map[string]any) string { return apID(act["actor"]) }

// apObjectID resolves an activity's object reference the same way.
func apObjectID(act map[string]any) string { return apID(act["object"]) }
