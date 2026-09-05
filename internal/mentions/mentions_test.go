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

func (m *mockBlockChecker) IsBlocked(_ context.Context, _, _, _ pgtype.UUID) (bool, error) {
	return m.blocked, nil
}

type mockMemberChecker struct {
	isMember bool
}

func (m *mockMemberChecker) IsMember(_ context.Context, _, _, _ pgtype.UUID) (bool, error) {
	return m.isMember, nil
}

type mockUserStatusChecker struct {
	erasedIDs map[pgtype.UUID]bool
}

func (m *mockUserStatusChecker) IsErased(_ context.Context, _, userID pgtype.UUID) (bool, error) {
	return m.erasedIDs[userID], nil
}

// BUG-012: Test that FilterMentions skips erased users when a
// UserStatusChecker is provided.
func TestFilterMentions_SkipsErased(t *testing.T) {
	t.Parallel()
	tenantID := mustUUID(t, "110e8400-e29b-41d4-a716-446655440000")
	authorID := mustUUID(t, "550e8400-e29b-41d4-a716-446655440000")
	erasedUserID := mustUUID(t, "660e8400-e29b-41d4-a716-446655440000")
	activeUserID := mustUUID(t, "770e8400-e29b-41d4-a716-446655440000")
	spaceID := mustUUID(t, "880e8400-e29b-41d4-a716-446655440000")

	mentions := []Mention{
		{ID: erasedUserID, EntityType: "user", Label: "Erased"},
		{ID: activeUserID, EntityType: "user", Label: "Active"},
	}

	userChecker := &mockUserStatusChecker{
		erasedIDs: map[pgtype.UUID]bool{erasedUserID: true},
	}

	filtered, err := FilterMentions(
		t.Context(),
		mentions,
		tenantID,
		authorID,
		spaceID,
		false,
		nil, nil, userChecker,
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(filtered) != 1 {
		t.Fatalf("expected 1 mention (erased skipped), got %d", len(filtered))
	}

	if filtered[0].Label != "Active" {
		t.Errorf("expected Active user to remain, got %s", filtered[0].Label)
	}
}

// BUG-013: Test that FilterMentions returns all non-self mentions when
// all checkers are nil (the production fallback when checkers aren't wired).
func TestFilterMentions_NilCheckers(t *testing.T) {
	t.Parallel()
	tenantID := mustUUID(t, "110e8400-e29b-41d4-a716-446655440000")
	authorID := mustUUID(t, "550e8400-e29b-41d4-a716-446655440000")
	user1 := mustUUID(t, "660e8400-e29b-41d4-a716-446655440000")
	user2 := mustUUID(t, "770e8400-e29b-41d4-a716-446655440000")
	spaceID := mustUUID(t, "880e8400-e29b-41d4-a716-446655440000")

	mentions := []Mention{
		{ID: authorID, EntityType: "user", Label: "Author"}, // self — skipped
		{ID: user1, EntityType: "user", Label: "User1"},
		{ID: user2, EntityType: "user", Label: "User2"},
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

	if len(filtered) != 2 {
		t.Fatalf("expected 2 mentions (self skipped, nil checkers), got %d", len(filtered))
	}
}

// BUG-004: Test that FilterMentions deduplicates mentions of the same user.
func TestFilterMentions_Deduplicates(t *testing.T) {
	t.Parallel()
	tenantID := mustUUID(t, "110e8400-e29b-41d4-a716-446655440000")
	authorID := mustUUID(t, "550e8400-e29b-41d4-a716-446655440000")
	user1 := mustUUID(t, "660e8400-e29b-41d4-a716-446655440000")
	spaceID := mustUUID(t, "770e8400-e29b-41d4-a716-446655440000")

	mentions := []Mention{
		{ID: user1, EntityType: "user", Label: "User1"},
		{ID: user1, EntityType: "user", Label: "User1"}, // duplicate
		{ID: user1, EntityType: "user", Label: "User1"}, // duplicate
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

	if len(filtered) != 1 {
		t.Fatalf("expected 1 mention (duplicates removed), got %d", len(filtered))
	}
}

// BUG-001: Test that japaMention nodes without entity_type default to "user".
func TestExtractMentions_JapaMentionDefaultEntityType(t *testing.T) {
	t.Parallel()

	doc := `{
		"type": "doc",
		"content": [
			{
				"type": "paragraph",
				"content": [
					{"type": "japaMention", "attrs": {"id": "550e8400-e29b-41d4-a716-446655440000", "label": "Legacy"}}
				]
			}
		]
	}`

	mentions, err := ExtractMentions([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}

	if len(mentions) != 1 {
		t.Fatalf("expected 1 japaMention, got %d", len(mentions))
	}

	if mentions[0].EntityType != "user" {
		t.Errorf("EntityType = %s, want user (default for japaMention)", mentions[0].EntityType)
	}
}

// TestRedactErasedMentionsInHTML verifies that mention spans for erased users
// are replaced with the @[deleted user] placeholder.
func TestRedactErasedMentionsInHTML(t *testing.T) {
	t.Parallel()

	erasedID := "550e8400-e29b-41d4-a716-446655440000"
	activeID := "660e8400-e29b-41d4-a716-446655440000"

	// Test both the new format (data-id, matching TipTap extension) and
	// the old format (data-user-id, for backward compatibility).
	html := `<p>Hello <span class="mention" data-type="mention" data-id="` + erasedID + `" data-label="Alice">@Alice</span> and <span class="mention" data-user-id="` + activeID + `">Bob</span></p>`

	redacted := RedactErasedMentionsInHTML(html, []string{erasedID})

	if !contains(redacted, "@[deleted user]") {
		t.Errorf("expected @[deleted user] in redacted HTML, got: %s", redacted)
	}

	if !contains(redacted, activeID) {
		t.Errorf("active user mention should be preserved, got: %s", redacted)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || containsString(s, substr))
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}

	return false
}
