package content

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/google/uuid"
)

// Ref: docs/superpowers/specs/2026-09-28-list-content-type-design.md
//      .jeos/sections/14-content-type-system.md (List amendment)

const listDefaultMaxItems = 50

type listValidationError struct {
	Code    string
	Message string
}

func (e *listValidationError) Error() string { return e.Message }

type listItemEmbed struct {
	Kind          string          `json:"kind"`
	ContentID     string          `json:"content_id,omitempty"`
	ContentSlug   string          `json:"content_slug,omitempty"`
	Snapshot      json.RawMessage `json:"snapshot,omitempty"`
	URL           string          `json:"url,omitempty"`
	Resolved      json.RawMessage `json:"resolved,omitempty"`
	VideoSourceID string          `json:"video_source_id,omitempty"`
}

type listItem struct {
	Heading   string         `json:"heading,omitempty"`
	BodyHTML  string         `json:"body_html,omitempty"`
	MediaID   string         `json:"media_id,omitempty"`
	Embed     *listItemEmbed `json:"embed,omitempty"`
	SourceURL string         `json:"source_url,omitempty"`
	Credit    string         `json:"credit,omitempty"`
}

// validateListFieldValues validates and normalizes the `list` field_values
// payload: list_mode enum, item cap (system_settings list.max_items, resolved
// by the caller), per-item body_html sanitization, UUID/embed-kind checks,
// http(s)-only URLs.
func validateListFieldValues(raw json.RawMessage, maxItems int) (json.RawMessage, *listValidationError) {
	var fv map[string]json.RawMessage
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &fv); err != nil {
			return nil, &listValidationError{"invalid_list_items", "field_values must be an object"}
		}
	} else {
		fv = map[string]json.RawMessage{}
	}

	if modeRaw, ok := fv["list_mode"]; ok {
		var mode string
		if err := json.Unmarshal(modeRaw, &mode); err != nil ||
			(mode != "numbered" && mode != "countdown" && mode != "unnumbered") {
			return nil, &listValidationError{"invalid_list_mode", "list_mode must be numbered, countdown, or unnumbered"}
		}
	}

	var items []listItem
	if itemsRaw, ok := fv["items"]; ok {
		if err := json.Unmarshal(itemsRaw, &items); err != nil {
			return nil, &listValidationError{"invalid_list_items", "items must be an array"}
		}
	}
	// U7: an absent items array defaults to [] (never null) so stored
	// field_values always carry an array. Empty is still valid at
	// draft/create time — the composer creates the row before items exist
	// (list_test.go empty cases) — but the normalized output is [] not null.
	if items == nil {
		items = []listItem{}
	}

	if len(items) > maxItems {
		return nil, &listValidationError{"too_many_items", fmt.Sprintf("list supports at most %d items", maxItems)}
	}

	for i := range items {
		if err := validateListItem(&items[i]); err != nil {
			return nil, err
		}
	}
	// items were unmarshalled into a detached slice — write the (sanitized)
	// slice back into the map before re-marshalling the whole payload.
	if enc, err := json.Marshal(items); err == nil {
		fv["items"] = enc
	}

	out, err := json.Marshal(fv)
	if err != nil {
		return nil, &listValidationError{"invalid_list_items", err.Error()}
	}

	return out, nil
}

// validateListItem validates one list item: field length caps, body_html
// re-sanitization (written back), the media_id/embed mutual exclusion, UUID
// formats, embed-kind requirements, and http(s)-only URLs.
func validateListItem(it *listItem) *listValidationError {
	invalid := func(msg string) *listValidationError {
		return &listValidationError{"invalid_list_items", msg}
	}

	if len(it.Heading) > 300 {
		return invalid("item heading exceeds 300 characters")
	}

	if len(it.Credit) > 300 {
		return invalid("item credit exceeds 300 characters")
	}

	// Re-sanitize per-item rich text — item bodies go through the same
	// whitelist as content body_html so stored field_values never carry
	// script/event-handler markup.
	if it.BodyHTML != "" {
		it.BodyHTML = SanitizeHTML(it.BodyHTML)
	}

	// One visual slot per item: media_id XOR embed.
	if it.MediaID != "" && it.Embed != nil {
		return invalid("item cannot set both media_id and embed")
	}

	if it.MediaID != "" {
		if _, err := uuid.Parse(it.MediaID); err != nil {
			return invalid("media_id must be a valid UUID")
		}
	}

	if it.SourceURL != "" && !isHTTPURL(it.SourceURL) {
		return invalid("source_url must be an http(s) URL")
	}

	if it.Embed != nil {
		if err := validateListItemEmbed(it.Embed); err != nil {
			return err
		}
	}

	return nil
}

// validateListItemEmbed enforces the embed-kind contract: `content` needs a
// content_id + content_slug, `url` needs an http(s) url, `video` needs a
// video_source_id. Resolved snapshots (link-preview/oEmbed) are checked for
// http(s)-only embed_url/url when present.
func validateListItemEmbed(e *listItemEmbed) *listValidationError {
	invalid := func(msg string) *listValidationError {
		return &listValidationError{"invalid_list_items", msg}
	}

	switch e.Kind {
	case "content":
		if _, err := uuid.Parse(e.ContentID); err != nil {
			return invalid("embed.content_id must be a valid UUID")
		}

		if e.ContentSlug == "" {
			return invalid("embed.content_slug is required for content embeds")
		}
	case "url":
		if e.URL == "" {
			return invalid("embed.url is required for url embeds")
		}

		if !isHTTPURL(e.URL) {
			return invalid("embed.url must be an http(s) URL")
		}

		if len(e.Resolved) > 0 {
			var resolved struct {
				EmbedURL string `json:"embed_url"`
				URL      string `json:"url"`
			}

			if err := json.Unmarshal(e.Resolved, &resolved); err != nil {
				return invalid("embed.resolved must be an object")
			}

			if resolved.EmbedURL != "" && !isHTTPURL(resolved.EmbedURL) {
				return invalid("embed.resolved.embed_url must be an http(s) URL")
			}

			if resolved.URL != "" && !isHTTPURL(resolved.URL) {
				return invalid("embed.resolved.url must be an http(s) URL")
			}
		}
	case "video":
		if _, err := uuid.Parse(e.VideoSourceID); err != nil {
			return invalid("embed.video_source_id must be a valid UUID")
		}
	default:
		return invalid("embed.kind must be content, url, or video")
	}

	return nil
}

// isHTTPURL reports whether s parses as an http or https URL.
func isHTTPURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil {
		return false
	}

	return u.Scheme == "http" || u.Scheme == "https"
}

// injectListItemListElements merges itemListElement/numberOfItems derived from
// field_values.items into a resolved ItemList JSON-LD document. For
// 'countdown' lists the array order is reversed so position 1 is the top-ranked
// item (the list's last element). Best-effort: any parse failure returns the
// input unchanged.
func injectListItemListElements(jsonLd []byte, fv json.RawMessage) []byte {
	var doc map[string]any
	if err := json.Unmarshal(jsonLd, &doc); err != nil {
		return jsonLd
	}

	var parsed struct {
		ListMode string     `json:"list_mode"`
		Items    []listItem `json:"items"`
	}

	if err := json.Unmarshal(fv, &parsed); err != nil || len(parsed.Items) == 0 {
		return jsonLd
	}

	type element struct {
		Type     string `json:"@type"`
		Position int    `json:"position"`
		Name     string `json:"name,omitempty"`
		URL      string `json:"url,omitempty"`
	}

	els := make([]element, 0, len(parsed.Items))

	for i := range parsed.Items {
		it := parsed.Items[i]
		if parsed.ListMode == "countdown" {
			// Countdown display order is reversed: the last array element
			// renders as "#1", so schema.org position p maps to
			// items[len-p] — the position matches the displayed number.
			it = parsed.Items[len(parsed.Items)-1-i]
		}

		els = append(els, element{Type: "ListItem", Position: i + 1, Name: it.Heading})
	}

	doc["itemListElement"] = els
	doc["numberOfItems"] = len(parsed.Items)

	out, err := json.Marshal(doc)
	if err != nil {
		return jsonLd
	}

	return out
}
