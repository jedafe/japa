package spaces

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
)

// TestToSpaceCards_LeanAndCorrect verifies the profile Spaces tab mapper (a)
// copies the card fields correctly and (b) drops the internal columns the raw
// gen.Space row carries so a public endpoint never echoes them.
func TestToSpaceCards_LeanAndCorrect(t *testing.T) {
	desc := "a public space"
	wiki := []byte(`{"secret":"internal wiki body"}`)
	row := gen.Space{
		ID:            mustUUID(t, "11111111-1111-1111-1111-111111111111"),
		TenantID:      mustUUID(t, "22222222-2222-2222-2222-222222222222"),
		ParentID:      mustUUID(t, "33333333-3333-3333-3333-333333333333"),
		SpaceTypeID:   mustUUID(t, "44444444-4444-4444-4444-444444444444"),
		Name:          "Lagos Devs",
		Slug:          "lagos-devs",
		Description:   &desc,
		WikiContent:   wiki,
		MemberCount:   42,
		PostCount:     99,
		Visibility:    "public",
		PostingPerm:   "members",
		ReplyingPerm:  "members",
		JoiningPerm:   "open",
		Status:        "active",
		CreatedAt:     pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}

	cards := toSpaceCards([]gen.Space{row})
	if len(cards) != 1 {
		t.Fatalf("got %d cards, want 1", len(cards))
	}
	c := cards[0]
	if c.Name != "Lagos Devs" || c.Slug != "lagos-devs" || c.MemberCount != 42 || c.Visibility != "public" {
		t.Fatalf("field-copy mismatch: %+v", c)
	}

	blob, err := json.Marshal(cards)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(blob)
	for _, leaked := range []string{"tenant_id", "wiki_content", "posting_perm", "replying_perm", "joining_perm", "parent_id", "space_type_id", "post_count", "status", "level"} {
		if strings.Contains(got, leaked) {
			t.Errorf("SpaceCard JSON leaks internal field %q: %s", leaked, got)
		}
	}
	for _, want := range []string{"\"id\"", "\"name\"", "\"slug\"", "\"member_count\""} {
		if !strings.Contains(got, want) {
			t.Errorf("SpaceCard JSON missing expected field %q: %s", want, got)
		}
	}
}
