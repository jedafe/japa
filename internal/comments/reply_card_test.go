package comments

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
)

func mustUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()

	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		t.Fatalf("scan uuid %q: %v", s, err)
	}

	return u
}

// TestToReplyCards_LeanAndCorrect verifies the profile Replies tab mapper copies
// the card fields correctly and drops the internal tenant_id the raw gen row
// carries.
func TestToReplyCards_LeanAndCorrect(t *testing.T) {
	row := gen.ListCommentsByAuthorRow{
		ID:           mustUUID(t, "11111111-1111-1111-1111-111111111111"),
		TenantID:     mustUUID(t, "22222222-2222-2222-2222-222222222222"),
		ContentID:    mustUUID(t, "33333333-3333-3333-3333-333333333333"),
		ParentID:     mustUUID(t, "44444444-4444-4444-4444-444444444444"),
		Body:         []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"great post"}]}]}`),
		VoteScore:    7,
		CreatedAt:    pgtype.Timestamptz{Time: time.Now(), Valid: true},
		ContentTitle: "Hello World",
		ContentSlug:  "hello-world",
	}

	cards := toReplyCards([]gen.ListCommentsByAuthorRow{row})
	if len(cards) != 1 {
		t.Fatalf("got %d cards, want 1", len(cards))
	}

	c := cards[0]
	if string(c.Body) != `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"great post"}]}]}` || c.VoteScore != 7 || c.ContentTitle != "Hello World" || c.ContentSlug != "hello-world" {
		t.Fatalf("field-copy mismatch: %+v", c)
	}

	blob, err := json.Marshal(cards)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	got := string(blob)
	if strings.Contains(got, "tenant_id") {
		t.Errorf("ReplyCard JSON leaks tenant_id: %s", got)
	}

	for _, want := range []string{"\"id\"", "\"body\"", "content_title", "content_slug", "vote_score"} {
		if !strings.Contains(got, want) {
			t.Errorf("ReplyCard JSON missing expected field %q: %s", want, got)
		}
	}
}
