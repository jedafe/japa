package schemas

// Event is the entity schema for community/embassy/webinar event records
// produced by the deterministic curators (iCal feeds, JSON-LD Event nodes).
// It is curate-only: no LLM prompt template exists for it, so intel:extract
// must never dispatch on this entity type — the worker guards that path via
// IsCurateOnly and routes review instead.
//
// starts_at/ends_at are plain strings (not the "date" coercer, which only
// accepts YYYY-MM-DD) because event times carry a full ISO 8601 datetime.
// recurrence holds the verbatim iCal RRULE and occurrences the expanded
// instance dates the ical curator computed inside its horizon window.
var Event = EntitySchema{
	EntityType: EntityTypeEvent,
	CurateOnly: true,
	Fields: []FieldSpec{
		{Name: "title", Type: "string", Required: true},
		{Name: "description", Type: "string", Required: false},
		{Name: "starts_at", Type: "string", Required: true},
		{Name: "ends_at", Type: "string", Required: false},
		{Name: "location", Type: "string", Required: false},
		{Name: "url", Type: "string", Required: false},
		{Name: "organizer", Type: "string", Required: false},
		{Name: "recurrence", Type: "string", Required: false},
		{Name: "occurrences", Type: "text_array", Required: false},
		{Name: "countries", Type: "country_array", Required: false},
	},
}
