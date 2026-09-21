package curator

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jedafe/japa/internal/intelligence/extractor"
	"github.com/jedafe/japa/internal/intelligence/extractor/schemas"
	"github.com/jedafe/japa/internal/intelligence/sources"
)

// ICal curates ical crawl-method channels: the raw doc body is the
// iCalendar (RFC 5545) bytes and every VEVENT becomes one event record.
// The parser is deliberately minimal — unfold continuation lines, split
// VEVENT blocks, read the scalar properties — because the repo carries no
// iCal dependency. RRULE is captured verbatim on the `recurrence` field and
// simple FREQ rules (DAILY/WEEKLY/MONTHLY/YEARLY with INTERVAL/COUNT/UNTIL)
// are expanded into an `occurrences` window so reviewers see the upcoming
// instances without a recurrence engine.
type ICal struct{}

// Handles returns the crawl method this curator serves.
func (ICal) Handles() string { return string(sources.CrawlMethodICal) }

// icalProp is one content line: NAME(;PARAM=val)*:VALUE.
type icalProp struct {
	params map[string]string
	value  string
}

// icalEvent is the parsed property set of one VEVENT block.
type icalEvent map[string][]icalProp

// recurrenceHorizon caps RRULE expansion at one year and 52 instances —
// enough for a reviewer to see the pattern without materializing thousands
// of occurrences on an open-ended rule.
const (
	recurrenceHorizon = 370 * 24 * time.Hour
	recurrenceMax     = 52
)

// Curate parses the iCalendar body and returns one record per VEVENT.
func (ICal) Curate(_ context.Context, body []byte, _ string, ch sources.Channel) ([]Record, error) {
	events, err := parseICalEvents(body)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, errors.New("curator: ical: no VEVENT blocks found")
	}

	entityType := resolveEntityType(schemas.EntityTypeEvent, ch)
	out := make([]Record, 0, len(events))

	for _, ev := range events {
		ext := icalEventToExtraction(ev)
		if _, ok := ext["title"]; !ok {
			continue // an untitled event has no usable record
		}

		out = append(out, Record{
			Extraction:  validate(ext, entityType),
			ExternalKey: icalEventKey(ev),
			EntityType:  entityType,
		})
	}

	if len(out) == 0 {
		return nil, errors.New("curator: ical: VEVENTs carried no usable fields")
	}

	return out, nil
}

// parseICalEvents unfolds the content lines and returns the VEVENT blocks.
func parseICalEvents(body []byte) ([]icalEvent, error) {
	lines := unfoldICalLines(string(body))
	if len(lines) == 0 {
		return nil, errors.New("curator: ical: empty body")
	}

	var events []icalEvent
	var cur icalEvent
	inEvent := false

	for _, line := range lines {
		upper := strings.ToUpper(line)
		switch {
		case upper == "BEGIN:VEVENT":
			cur = icalEvent{}
			inEvent = true
		case upper == "END:VEVENT":
			if inEvent {
				events = append(events, cur)
			}
			inEvent = false
		case inEvent:
			name, prop := parseICalProp(line)
			if name == "" {
				continue
			}
			cur[name] = append(cur[name], prop)
		}
	}

	return events, nil
}

// unfoldICalLines normalizes CRLF and joins continuation lines (a line
// starting with space or tab continues the previous line per RFC 5545 §3.1).
func unfoldICalLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	var lines []string
	for _, raw := range strings.Split(s, "\n") {
		if raw == "" {
			continue
		}
		if (raw[0] == ' ' || raw[0] == '\t') && len(lines) > 0 {
			lines[len(lines)-1] += raw[1:]
			continue
		}
		lines = append(lines, raw)
	}

	return lines
}

// parseICalProp splits "NAME;PARAM=val;PARAM2=v2:VALUE" into its name and
// property. Params and values are upper-cased on the name only — values keep
// their case.
func parseICalProp(line string) (string, icalProp) {
	// The value starts at the first unquoted/unescaped colon.
	colon := strings.Index(line, ":")
	if colon < 0 {
		return "", icalProp{}
	}

	head, value := line[:colon], line[colon+1:]
	prop := icalProp{value: unescapeICalValue(value)}

	parts := strings.Split(head, ";")
	name := strings.ToUpper(strings.TrimSpace(parts[0]))
	if name == "" {
		return "", icalProp{}
	}

	for _, p := range parts[1:] {
		k, v, ok := strings.Cut(p, "=")
		if !ok {
			continue
		}
		if prop.params == nil {
			prop.params = map[string]string{}
		}
		prop.params[strings.ToUpper(strings.TrimSpace(k))] = strings.Trim(strings.TrimSpace(v), `"`)
	}

	return name, prop
}

// unescapeICalValue decodes RFC 5545 text escapes (\n, \, \; \,).
func unescapeICalValue(s string) string {
	r := strings.NewReplacer(`\n`, "\n", `\N`, "\n", `\,`, ",", `\;`, ";", `\\`, "\\")
	return r.Replace(s)
}

// icalValue returns the first property value for name, "" when absent.
func icalValue(ev icalEvent, name string) string {
	if props, ok := ev[name]; ok && len(props) > 0 {
		return strings.TrimSpace(props[0].value)
	}

	return ""
}

// icalPropFor returns the first property (value + params) for name.
func icalPropFor(ev icalEvent, name string) (icalProp, bool) {
	if props, ok := ev[name]; ok && len(props) > 0 {
		return props[0], true
	}

	return icalProp{}, false
}

// icalEventToExtraction maps a VEVENT onto the event schema.
func icalEventToExtraction(ev icalEvent) extractor.Extraction {
	ext := make(extractor.Extraction)

	if s := icalValue(ev, "SUMMARY"); s != "" {
		field(ext, "title", s, s)
	}
	if s := icalValue(ev, "DESCRIPTION"); s != "" {
		field(ext, "description", s, s)
	}

	if prop, ok := icalPropFor(ev, "DTSTART"); ok {
		if raw, iso := prop.value, icalDateTime(prop); iso != "" {
			field(ext, "starts_at", iso, raw)
		}
	}
	if prop, ok := icalPropFor(ev, "DTEND"); ok {
		if raw, iso := prop.value, icalDateTime(prop); iso != "" {
			field(ext, "ends_at", iso, raw)
		}
	}
	if s := icalValue(ev, "LOCATION"); s != "" {
		field(ext, "location", s, s)
	}
	if s := icalValue(ev, "URL"); s != "" {
		field(ext, "url", s, s)
	}
	if s := icalOrganizer(ev); s != "" {
		field(ext, "organizer", s, s)
	}

	if rrule := icalValue(ev, "RRULE"); rrule != "" {
		field(ext, "recurrence", rrule, rrule)

		if prop, ok := icalPropFor(ev, "DTSTART"); ok {
			if start, ok := parseICalTime(prop); ok {
				if occ := expandRRule(rrule, start); len(occ) > 0 {
					field(ext, "occurrences", occ, rrule)
				}
			}
		}
	}

	return ext
}

// icalOrganizer prefers the ORGANIZER CN param; otherwise it strips the
// mailto: scheme from the value.
func icalOrganizer(ev icalEvent) string {
	prop, ok := icalPropFor(ev, "ORGANIZER")
	if !ok {
		return ""
	}
	if cn := prop.params["CN"]; cn != "" {
		return cn
	}

	return strings.TrimPrefix(prop.value, "mailto:")
}

// icalEventKey derives the record's external key: UID + RECURRENCE-ID when
// present (a recurring series shares one UID across instances), else a
// content hash of summary + start.
func icalEventKey(ev icalEvent) string {
	uid := icalValue(ev, "UID")
	rid := icalValue(ev, "RECURRENCE-ID")

	switch {
	case uid != "" && rid != "":
		return uid + "#" + rid
	case uid != "":
		return uid
	}

	return "hash:" + hashKey(icalValue(ev, "SUMMARY"), icalValue(ev, "DTSTART"))
}

// parseICalTime parses a DTSTART/DTEND property into a time. DATE values
// (20060102) return midnight UTC; DATE-TIME values honor a trailing Z or a
// TZID param; floating times parse as UTC.
func parseICalTime(prop icalProp) (time.Time, bool) {
	v := strings.TrimSpace(prop.value)

	// DATE value: YYYYMMDD.
	if len(v) == 8 && !strings.Contains(v, "T") {
		t, err := time.Parse("20060102", v)
		return t, err == nil
	}

	layouts := []string{"20060102T150405Z", "20060102T150405", "20060102T1504"}

	loc := time.UTC
	if tzid := prop.params["TZID"]; tzid != "" && !strings.HasSuffix(v, "Z") {
		if l, err := time.LoadLocation(tzid); err == nil {
			loc = l
		}
	}

	for _, layout := range layouts {
		if strings.HasSuffix(v, "Z") && !strings.HasSuffix(layout, "Z") {
			continue
		}
		if t, err := time.ParseInLocation(layout, v, loc); err == nil {
			return t, true
		}
	}

	return time.Time{}, false
}

// icalDateTime formats a DTSTART/DTEND property as ISO 8601 — date-only for
// all-day values, RFC 3339 for date-times.
func icalDateTime(prop icalProp) string {
	t, ok := parseICalTime(prop)
	if !ok {
		return ""
	}
	if len(strings.TrimSpace(prop.value)) == 8 && !strings.Contains(prop.value, "T") {
		return t.Format("2006-01-02")
	}

	return t.UTC().Format(time.RFC3339)
}

// expandRRule expands a simple RRULE (FREQ with optional INTERVAL, COUNT,
// UNTIL) into occurrence dates starting at dtstart. BYDAY/BYMONTH modifiers
// are not evaluated — unsupported rules return just dtstart so the field
// stays honest rather than wrong.
func expandRRule(rrule string, dtstart time.Time) []string {
	parts := map[string]string{}

	for _, kv := range strings.Split(rrule, ";") {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		parts[strings.ToUpper(strings.TrimSpace(k))] = strings.ToUpper(strings.TrimSpace(v))
	}

	// Rules with positional modifiers are too complex for the minimal
	// expander — return the start date alone rather than guessing.
	for k := range parts {
		switch k {
		case "FREQ", "INTERVAL", "COUNT", "UNTIL":
		default:
			return []string{dtstart.Format(time.RFC3339)}
		}
	}

	interval := 1
	if s := parts["INTERVAL"]; s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			interval = n
		}
	}

	count := recurrenceMax
	if s := parts["COUNT"]; s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			count = n
		}
	}

	var until time.Time
	if s := parts["UNTIL"]; s != "" {
		if t, ok := parseICalTime(icalProp{value: s}); ok {
			until = t
		}
	}

	horizon := dtstart.Add(recurrenceHorizon)

	var step func(time.Time, int) time.Time
	switch parts["FREQ"] {
	case "DAILY":
		step = func(t time.Time, i int) time.Time { return t.AddDate(0, 0, interval*i) }
	case "WEEKLY":
		step = func(t time.Time, i int) time.Time { return t.AddDate(0, 0, 7*interval*i) }
	case "MONTHLY":
		step = func(t time.Time, i int) time.Time { return t.AddDate(0, interval*i, 0) }
	case "YEARLY":
		step = func(t time.Time, i int) time.Time { return t.AddDate(interval*i, 0, 0) }
	default:
		return []string{dtstart.Format(time.RFC3339)}
	}

	out := make([]string, 0, count)

	for i := 0; i < count && i < recurrenceMax; i++ {
		occ := step(dtstart, i)
		if occ.After(horizon) {
			break
		}
		if !until.IsZero() && occ.After(until) {
			break
		}
		out = append(out, occ.UTC().Format(time.RFC3339))
	}

	return out
}
