package mentions

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func mustUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := id.Scan(s); err != nil {
		t.Fatalf("scan uuid %q: %v", s, err)
	}
	return id
}

func TestExtractMentions(t *testing.T) {
	doc := `{
		"type": "doc",
		"content": [
			{
				"type": "paragraph",
				"content": [
					{"type": "text", "text": "Hello "},
					{"type": "japaMention", "attrs": {"id": "550e8400-e29b-41d4-a716-446655440000", "entity_type": "user", "label": "Alice"}},
					{"type": "text", "text": " and "},
					{"type": "japaMention", "attrs": {"id": "660e8400-e29b-41d4-a716-446655440000", "entity_type": "user", "label": "Bob"}}
				]
			}
		]
	}`

	mentions, err := ExtractMentions([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if len(mentions) != 2 {
		t.Fatalf("expected 2 mentions, got %d", len(mentions))
	}
	if mentions[0].Label != "Alice" {
		t.Errorf("mentions[0].Label = %s, want Alice", mentions[0].Label)
	}
	if mentions[1].Label != "Bob" {
		t.Errorf("mentions[1].Label = %s, want Bob", mentions[1].Label)
	}
}

func TestExtractMentions_SkipsNonUser(t *testing.T) {
	doc := `{
		"type": "doc",
		"content": [
			{
				"type": "paragraph",
				"content": [
					{"type": "japaMention", "attrs": {"id": "550e8400-e29b-41d4-a716-446655440000", "entity_type": "content", "label": "Post"}}
				]
			}
		]
	}`

	mentions, err := ExtractMentions([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if len(mentions) != 0 {
		t.Errorf("expected 0 mentions, got %d", len(mentions))
	}
}

func TestFilterMentions_SkipsAuthor(t *testing.T) {
	authorID := mustUUID(t, "550e8400-e29b-41d4-a716-446655440000")
	spaceID := mustUUID(t, "660e8400-e29b-41d4-a716-446655440000")

	mentions := []Mention{
		{ID: authorID, EntityType: "user", Label: "Author"},
	}

	filtered, err := FilterMentions(
		context.Background(),
		mentions,
		authorID,
		spaceID,
		false,
		nil, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 0 {
		t.Errorf("expected 0 mentions (author skipped), got %d", len(filtered))
	}
}

func TestFilterMentions_SkipsBlocked(t *testing.T) {
	authorID := mustUUID(t, "550e8400-e29b-41d4-a716-446655440000")
	mentionedID := mustUUID(t, "660e8400-e29b-41d4-a716-446655440000")
	spaceID := mustUUID(t, "770e8400-e29b-41d4-a716-446655440000")

	mentions := []Mention{
		{ID: mentionedID, EntityType: "user", Label: "Blocked"},
	}

	// Mock block checker that says author blocked mentioned
	blockChecker := &mockBlockChecker{blocked: true}

	filtered, err := FilterMentions(
		context.Background(),
		mentions,
		authorID,
		spaceID,
		false,
		blockChecker, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 0 {
		t.Errorf("expected 0 mentions (blocked), got %d", len(filtered))
	}
}

func TestFilterMentions_SkipsNonMemberInPrivateSpace(t *testing.T) {
	authorID := mustUUID(t, "550e8400-e29b-41d4-a716-446655440000")
	mentionedID := mustUUID(t, "660e8400-e29b-41d4-a716-446655440000")
	spaceID := mustUUID(t, "770e8400-e29b-41d4-a716-446655440000")

	mentions := []Mention{
		{ID: mentionedID, EntityType: "user", Label: "NonMember"},
	}

	// Mock member checker that says user is not a member
	memberChecker := &mockMemberChecker{isMember: false}

	filtered, err := FilterMentions(
		context.Background(),
		mentions,
		authorID,
		spaceID,
		true, // private space
		nil, memberChecker, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 0 {
		t.Errorf("expected 0 mentions (non-member in private space), got %d", len(filtered))
	}
}

// Mock implementations for testing

type mockBlockChecker struct {
	blocked bool
}

func (m *mockBlockChecker) IsBlocked(ctx context.Context, blockerID, blockedID pgtype.UUID) (bool, error) {
	return m.blocked, nil
}

type mockMemberChecker struct {
	isMember bool
}

func (m *mockMemberChecker) IsMember(ctx context.Context, spaceID, userID pgtype.UUID) (bool, error) {
	return m.isMember, nil
}

type mockUserChecker struct {
	erased bool
}

func (m *mockUserChecker) IsErased(ctx context.Context, userID pgtype.UUID) (bool, error) {
	return m.erased, nil
}
