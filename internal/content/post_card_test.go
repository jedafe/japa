package content

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
)

// TestToPostCards_LeanAndCorrect verifies the profile Posts tab mapper copies
// the card fields correctly and drops the internal columns the raw gen row
// carries (tenant_id, raw body, field_values, content_hash, etc.).
func TestToPostCards_LeanAndCorrect(t *testing.T) {
	hash := "deadbeef"
	canonical := "https://example.com/x"
	row := gen.ListPublishedContentByAuthorRow{
		ID:              mustUUID(t, "11111111-1111-1111-1111-111111111111"),
		TenantID:        mustUUID(t, "22222222-2222-2222-2222-222222222222"),
		AuthorID:        mustUUID(t, "33333333-3333-3333-3333-333333333333"),
		SpaceID:         mustUUID(t, "44444444-4444-4444-4444-444444444444"),
		ContentType:     "discussion",
		Title:           "Hello World",
		Slug:            "hello-world",
		Body:            `{"type":"doc","content":[]}`,
		BodyHtml:        "<p>Hello World</p>",
		FieldValues:     []byte(`{"custom":"maybe-not-public"}`),
		ContentHash:     &hash,
		CanonicalUrl:    &canonical,
		Status:          "published",
		Visibility:      "public",
		ViewCount:       10,
		CommentCount:    3,
		ShareCount:      2,
		IsPinned:        true,
		PublishedAt:     pgtype.Timestamptz{Time: time.Now(), Valid: true},
		CreatedAt:       pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}

	cards := toPostCards([]gen.ListPublishedContentByAuthorRow{row})
	if len(cards) != 1 {
		t.Fatalf("got %d cards, want 1", len(cards))
	}
	c := cards[0]
	if c.Title != "Hello World" || c.Slug != "hello-world" || c.ContentType != "discussion" ||
		c.BodyHtml != "<p>Hello World</p>" || c.ViewCount != 10 || c.CommentCount != 3 || !c.IsPinned {
		t.Fatalf("field-copy mismatch: %+v", c)
	}

	blob, err := json.Marshal(cards)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(blob)
	for _, leaked := range []string{"tenant_id", "field_values", "content_hash", "canonical_url", "\"body\"", "geo_space_id", "\"status\"", "visibility", "is_locked", "reshare_count"} {
		if strings.Contains(got, leaked) {
			t.Errorf("PostCard JSON leaks internal field %q: %s", leaked, got)
		}
	}
	for _, want := range []string{"\"id\"", "\"title\"", "\"slug\"", "body_html", "comment_count"} {
		if !strings.Contains(got, want) {
			t.Errorf("PostCard JSON missing expected field %q: %s", want, got)
		}
	}
}
