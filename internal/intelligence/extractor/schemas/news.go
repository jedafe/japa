package schemas

// News is the entity schema for the immigration/migration news domain
// (spec item 10: "Immigration news"). News sources are RSS feeds from
// outlets like BBC Africa, Al Jazeera, Punch Nigeria, etc. The extractor
// parses each feed item into a structured news article for the moderation
// queue. News records are always human-reviewed before publishing (no
// auto-promote projection table exists for news); the extracted fields
// give the reviewer structured metadata to act on.
var News = EntitySchema{
	EntityType: EntityTypeNews,
	Fields: []FieldSpec{
		{Name: "title", Type: "string", Required: true},
		{Name: "summary", Type: "string", Required: false},
		{Name: "published_date", Type: "date", Required: false},
		{Name: "countries", Type: "country_array", Required: false},
		{Name: "topics", Type: "text_array", Required: false},
		{Name: "key_points", Type: "text_array", Required: false},
		{Name: "article_url", Type: "string", Required: false},
		{Name: "author", Type: "string", Required: false},
	},
}
