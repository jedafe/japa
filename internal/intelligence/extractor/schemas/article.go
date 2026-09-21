package schemas

// Article is the entity schema for blog/editorial article records produced
// by the deterministic curators (RSS blog feeds, JSON-LD Article/
// BlogPosting nodes). It is curate-only: no LLM prompt template exists for
// it, so intel:extract must never dispatch on this entity type — the worker
// guards that path via IsCurateOnly and routes review instead.
var Article = EntitySchema{
	EntityType: EntityTypeArticle,
	CurateOnly: true,
	Fields: []FieldSpec{
		{Name: "title", Type: "string", Required: true},
		{Name: "summary", Type: "string", Required: false},
		{Name: "body", Type: "string", Required: false},
		{Name: "published_date", Type: "date", Required: false},
		{Name: "author", Type: "string", Required: false},
		{Name: "article_url", Type: "string", Required: false},
		{Name: "topics", Type: "text_array", Required: false},
		{Name: "image_url", Type: "string", Required: false},
	},
}
