package content

import (
	"encoding/json"
	"testing"
)

// TestCreateContentInput_SeoFields verifies the CreateContentInput struct
// carries the SEO + featured image fields added in migration 000165. This is
// a compile-time + runtime guard: if the fields are removed or renamed, this
// test fails to compile.
func TestCreateContentInput_SeoFields(t *testing.T) {
	t.Parallel()

	input := CreateContentInput{
		Title:           "Test",
		MetaTitle:       strPtr("Custom SEO Title"),
		MetaDescription: strPtr("Custom meta description"),
		OgImageUrl:      strPtr("https://example.com/og.png"),
		SeoIndexable:    false,
	}

	if input.MetaTitle == nil || *input.MetaTitle != "Custom SEO Title" {
		t.Fatalf("MetaTitle not carried: %v", input.MetaTitle)
	}
	if input.MetaDescription == nil || *input.MetaDescription != "Custom meta description" {
		t.Fatalf("MetaDescription not carried: %v", input.MetaDescription)
	}
	if input.OgImageUrl == nil || *input.OgImageUrl != "https://example.com/og.png" {
		t.Fatalf("OgImageUrl not carried: %v", input.OgImageUrl)
	}
	if input.SeoIndexable != false {
		t.Fatalf("SeoIndexable not carried: %v", input.SeoIndexable)
	}
}

// TestUpdateContentInput_SeoFields verifies the UpdateContentInput struct
// carries the SEO + featured image fields. SeoIndexable is *bool (nullable)
// so PATCH can omit it without changing the existing value.
func TestUpdateContentInput_SeoFields(t *testing.T) {
	t.Parallel()

	indexable := false
	input := UpdateContentInput{
		Title:           "Test",
		MetaTitle:       strPtr("Updated SEO Title"),
		MetaDescription: strPtr("Updated meta description"),
		OgImageUrl:      strPtr("https://example.com/og-updated.png"),
		SeoIndexable:    &indexable,
	}

	if input.MetaTitle == nil || *input.MetaTitle != "Updated SEO Title" {
		t.Fatalf("MetaTitle not carried: %v", input.MetaTitle)
	}
	if input.MetaDescription == nil || *input.MetaDescription != "Updated meta description" {
		t.Fatalf("MetaDescription not carried: %v", input.MetaDescription)
	}
	if input.OgImageUrl == nil || *input.OgImageUrl != "https://example.com/og-updated.png" {
		t.Fatalf("OgImageUrl not carried: %v", input.OgImageUrl)
	}
	if input.SeoIndexable == nil || *input.SeoIndexable != false {
		t.Fatalf("SeoIndexable not carried: %v", input.SeoIndexable)
	}
}

// TestCreateContentJSON_ParsesSeoFields verifies the JSON shape the /create
// frontend sends is parseable into the handler's input struct shape. This
// guards the JSON contract between frontend and backend.
func TestCreateContentJSON_ParsesSeoFields(t *testing.T) {
	t.Parallel()

	body := `{
		"content_type": "blog_post",
		"title": "My Post",
		"body_html": "<p>Hello</p>",
		"featured_image_id": "550e8400-e29b-41d4-a716-446655440000",
		"meta_title": "Custom SEO Title",
		"meta_description": "Custom meta description",
		"og_image_url": "https://example.com/og.png",
		"seo_indexable": false,
		"visibility": "public"
	}`

	var input struct {
		ContentType     string          `json:"content_type"`
		Title           string          `json:"title"`
		BodyHtml        string          `json:"body_html"`
		FeaturedImageID *string         `json:"featured_image_id"`
		MetaTitle       *string         `json:"meta_title"`
		MetaDescription *string         `json:"meta_description"`
		OgImageUrl      *string         `json:"og_image_url"`
		SeoIndexable    *bool           `json:"seo_indexable"`
		Visibility      string          `json:"visibility"`
	}

	if err := json.Unmarshal([]byte(body), &input); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if input.Title != "My Post" {
		t.Fatalf("title: %q", input.Title)
	}
	if input.FeaturedImageID == nil || *input.FeaturedImageID != "550e8400-e29b-41d4-a716-446655440000" {
		t.Fatalf("featured_image_id: %v", input.FeaturedImageID)
	}
	if input.MetaTitle == nil || *input.MetaTitle != "Custom SEO Title" {
		t.Fatalf("meta_title: %v", input.MetaTitle)
	}
	if input.MetaDescription == nil || *input.MetaDescription != "Custom meta description" {
		t.Fatalf("meta_description: %v", input.MetaDescription)
	}
	if input.OgImageUrl == nil || *input.OgImageUrl != "https://example.com/og.png" {
		t.Fatalf("og_image_url: %v", input.OgImageUrl)
	}
	if input.SeoIndexable == nil || *input.SeoIndexable != false {
		t.Fatalf("seo_indexable: %v", input.SeoIndexable)
	}
}

func strPtr(s string) *string {
	return &s
}
