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
	t.Parallel()

	doc := `{
		"type": "doc",
		"content": [
			{
				"type": "paragraph",
				"content": [
					{"type": "text", "text": "Hello "},
					{"type": "mention", "attrs": {"id": "550e8400-e29b-41d4-a716-446655440000", "entity_type": "user", "label": "Alice"}},
					{"type": "text", "text": " and "},
					{"type": "mention", "attrs": {"id": "660e8400-e29b-41d4-a716-446655440000", "entity_type": "user", "label": "Bob"}}
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

func TestExtractMentions_DefaultsEntityTypeForMentionNode(t *testing.T) {
	t.Parallel()

	// A "mention" node without entity_type should default to "user"
	// (the frontend suggestion renderer may omit entity_type).
	doc := `{
		"type": "doc",
		"content": [
			{
				"type": "paragraph",
				"content": [
					{"type": "mention", "attrs": {"id": "550e8400-e29b-41d4-a716-446655440000", "label": "Alice"}}
				]
			}
		]
	}`

	mentions, err := ExtractMentions([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}

	if len(mentions) != 1 {
		t.Fatalf("expected 1 mention (default entity_type), got %d", len(mentions))
	}

	if mentions[0].EntityType != "user" {
		t.Errorf("EntityType = %s, want user (default)", mentions[0].EntityType)
	}
}

func TestExtractMentions_NestedInTable(t *testing.T) {
	t.Parallel()

	// A mention nested inside a table → tableRow → tableCell → paragraph
	// must be found by the recursive walker.
	doc := `{
		"type": "doc",
		"content": [
			{
				"type": "table",
				"content": [
					{
						"type": "tableRow",
						"content": [
							{
								"type": "tableCell",
								"content": [
									{
										"type": "paragraph",
										"content": [
											{"type": "mention", "attrs": {"id": "550e8400-e29b-41d4-a716-446655440000", "entity_type": "user", "label": "Nested"}}
										]
									}
								]
							}
						]
					}
				]
			}
		]
	}`

	mentions, err := ExtractMentions([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}

	if len(mentions) != 1 {
		t.Fatalf("expected 1 nested mention, got %d", len(mentions))
	}

	if mentions[0].Label != "Nested" {
		t.Errorf("Label = %s, want Nested", mentions[0].Label)
	}
}

func TestExtractMentions_BareArray(t *testing.T) {
	t.Parallel()

	// A bare array of top-level nodes (no doc wrapper).
	doc := `[
		{
			"type": "paragraph",
			"content": [
				{"type": "mention", "attrs": {"id": "550e8400-e29b-41d4-a716-446655440000", "entity_type": "user", "label": "Bare"}}
			]
		}
	]`

	mentions, err := ExtractMentions([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}

	if len(mentions) != 1 {
		t.Fatalf("expected 1 mention from bare array, got %d", len(mentions))
	}

	if mentions[0].Label != "Bare" {
		t.Errorf("Label = %s, want Bare", mentions[0].Label)
	}
}

func TestExtractMentions_SkipsNonUser(t *testing.T) {
	t.Parallel()

	doc := `{
		"type": "doc",
		"content": [
			{
				"type": "paragraph",
				"content": [
					{"type": "mention", "attrs": {"id": "550e8400-e29b-41d4-a716-446655440000", "entity_type": "content", "label": "Post"}}
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
	t.Parallel()
	tenantID := mustUUID(t, "110e8400-e29b-41d4-a716-446655440000")
	authorID := mustUUID(t, "550e8400-e29b-41d4-a716-446655440000")
	spaceID := mustUUID(t, "660e8400-e29b-41d4-a716-446655440000")

	mentions := []Mention{
		{ID: authorID, EntityType: "user", Label: "Author"},
	}

	filtered, err := FilterMentions(
		t.Context(),
		mentions,
		tenantID,
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
	t.Parallel()
	tenantID := mustUUID(t, "110e8400-e29b-41d4-a716-446655440000")
	authorID := mustUUID(t, "550e8400-e29b-41d4-a716-446655440000")
	mentionedID := mustUUID(t, "660e8400-e29b-41d4-a716-446655440000")
	spaceID := mustUUID(t, "770e8400-e29b-41d4-a716-446655440000")

	mentions := []Mention{
		{ID: mentionedID, EntityType: "user", Label: "Blocked"},
	}

	// Mock block checker that says author blocked mentioned
	blockChecker := &mockBlockChecker{blocked: true}

	filtered, err := FilterMentions(
		t.Context(),
		mentions,
		tenantID,
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
	t.Parallel()
	tenantID := mustUUID(t, "110e8400-e29b-41d4-a716-446655440000")
	authorID := mustUUID(t, "550e8400-e29b-41d4-a716-446655440000")
	mentionedID := mustUUID(t, "660e8400-e29b-41d4-a716-446655440000")
	spaceID := mustUUID(t, "770e8400-e29b-41d4-a716-446655440000")

	mentions := []Mention{
		{ID: mentionedID, EntityType: "user", Label: "NonMember"},
	}

	// Mock member checker that says user is not a member
	memberChecker := &mockMemberChecker{isMember: false}

	filtered, err := FilterMentions(
		t.Context(),
		mentions,
		tenantID,
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

func (m *mockBlockChecker) IsBlocked(_ context.Context, _, _ pgtype.UUID) (bool, error) {
	return m.blocked, nil
}

type mockMemberChecker struct {
	isMember bool
}

func (m *mockMemberChecker) IsMember(_ context.Context, _, _, _ pgtype.UUID) (bool, error) {
	return m.isMember, nil
}
