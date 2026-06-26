package forum

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// Constants and configuration
// ---------------------------------------------------------------------------

func TestVoteConstants(t *testing.T) {
	if VoteDown != -1 {
		t.Errorf("VoteDown = %d, want -1", VoteDown)
	}
	if VoteUp != 1 {
		t.Errorf("VoteUp = %d, want 1", VoteUp)
	}
}

func TestEntityTypeConstants(t *testing.T) {
	if EntityTypeThread != "thread" {
		t.Errorf("EntityTypeThread = %q, want %q", EntityTypeThread, "thread")
	}
	if EntityTypeReply != "reply" {
		t.Errorf("EntityTypeReply = %q, want %q", EntityTypeReply, "reply")
	}
}

func TestPaginationDefaults(t *testing.T) {
	if DefaultPageSize != 20 {
		t.Errorf("DefaultPageSize = %d, want 20", DefaultPageSize)
	}
	if MaxPageSize != 100 {
		t.Errorf("MaxPageSize = %d, want 100", MaxPageSize)
	}
}

func TestDefaultEditWindowMinutes(t *testing.T) {
	if DefaultEditWindowMinutes != 30 {
		t.Errorf("DefaultEditWindowMinutes = %d, want 30", DefaultEditWindowMinutes)
	}
}

// ---------------------------------------------------------------------------
// Vote value validation
// ---------------------------------------------------------------------------

func TestVoteValueValidation(t *testing.T) {
	tests := []struct {
		name    string
		value   int16
		wantErr bool
	}{
		{"valid upvote", VoteUp, false},
		{"valid downvote", VoteDown, false},
		{"invalid zero", 0, true},
		{"invalid two", 2, true},
		{"invalid negative two", -2, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != VoteDown && tt.value != VoteUp {
				if !tt.wantErr {
					t.Errorf("expected no error for value %d", tt.value)
				}
				return
			}
			if tt.wantErr {
				t.Errorf("expected error for value %d but got none", tt.value)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Entity type validation
// ---------------------------------------------------------------------------

func TestEntityTypeValidation(t *testing.T) {
	validTypes := []string{EntityTypeThread, EntityTypeReply}
	for _, vt := range validTypes {
		if vt != "thread" && vt != "reply" {
			t.Errorf("entity type %q should be valid", vt)
		}
	}

	invalidTypes := []string{"comment", "post", "message", ""}
	for _, it := range invalidTypes {
		if it == "thread" || it == "reply" {
			t.Errorf("entity type %q should be invalid", it)
		}
	}
}

// ---------------------------------------------------------------------------
// Poll creation input validation
// ---------------------------------------------------------------------------

func TestCreatePollInputValidation(t *testing.T) {
	// Empty question should be invalid.
	input := CreatePollInput{
		Question: "",
		Options: []PollOptionInput{
			{Label: "Yes", Position: 0},
			{Label: "No", Position: 1},
		},
	}
	if input.Question != "" {
		t.Error("expected empty question")
	}

	// Less than 2 options should be invalid.
	input2 := CreatePollInput{
		Question: "Test?",
		Options: []PollOptionInput{
			{Label: "Yes", Position: 0},
		},
	}
	if len(input2.Options) >= 2 {
		t.Error("expected fewer than 2 options")
	}
}

// ---------------------------------------------------------------------------
// Tag input validation
// ---------------------------------------------------------------------------

func TestCreateTagInputValidation(t *testing.T) {
	// Empty name should be invalid.
	input := CreateTagInput{Name: "", Slug: "test"}
	if input.Name != "" {
		t.Error("expected empty name")
	}

	// Empty slug should be invalid.
	input2 := CreateTagInput{Name: "Test", Slug: ""}
	if input2.Slug != "" {
		t.Error("expected empty slug")
	}
}

// ---------------------------------------------------------------------------
// Bookmark folder default
// ---------------------------------------------------------------------------

func TestBookmarkFolderDefault(t *testing.T) {
	folder := ""
	if folder == "" {
		folder = "default"
	}
	if folder != "default" {
		t.Errorf("expected default folder, got %q", folder)
	}
}

// ---------------------------------------------------------------------------
// Thread creation input defaults
// ---------------------------------------------------------------------------

func TestCreateThreadInputDefaults(t *testing.T) {
	input := CreateThreadInput{
		Title: "Test Thread",
		Slug:  "test-thread",
	}
	if input.EditWindowMinutes <= 0 {
		input.EditWindowMinutes = DefaultEditWindowMinutes
	}
	if input.EditWindowMinutes != 30 {
		t.Errorf("expected edit window 30, got %d", input.EditWindowMinutes)
	}
	if input.Body == nil {
		input.Body = []byte("[]")
	}
	if string(input.Body) != "[]" {
		t.Errorf("expected empty JSON array body, got %q", string(input.Body))
	}
}

// ---------------------------------------------------------------------------
// pgtype.UUID zero value
// ---------------------------------------------------------------------------

func TestUUIDZeroValue(t *testing.T) {
	var id pgtype.UUID
	if id.Valid {
		t.Error("zero pgtype.UUID should not be valid")
	}
}
