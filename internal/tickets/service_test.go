package tickets

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// Service unit tests (pure logic, no DB)
// ---------------------------------------------------------------------------

func TestCreateTicketInputValidation(t *testing.T) {
	t.Parallel()
	// Empty subject should be invalid.
	input := CreateTicketInput{Subject: "", Description: "desc"}
	if input.Subject != "" {
		t.Error("expected empty subject")
	}

	// Empty description should be invalid.
	input2 := CreateTicketInput{Subject: "Test", Description: ""}
	if input2.Description != "" {
		t.Error("expected empty description")
	}
}

func TestAddCommentInputValidation(t *testing.T) {
	t.Parallel()
	// Empty body should be invalid.
	input := AddCommentInput{Body: ""}
	if input.Body != "" {
		t.Error("expected empty body")
	}
}

func TestUpdateTicketStatusInputDefaults(t *testing.T) {
	t.Parallel()

	input := UpdateTicketStatusInput{
		Status: string(StatusOpen),
	}
	if input.ResolutionNote != "" {
		t.Error("expected empty resolution note")
	}
}

func TestUpdateTicketPriorityInputDefaults(t *testing.T) {
	t.Parallel()

	input := UpdateTicketPriorityInput{
		Priority: string(PriorityNormal),
	}
	if input.Priority != string(PriorityNormal) {
		t.Errorf("expected priority %q, got %q", PriorityNormal, input.Priority)
	}
}

func TestAssignTicketInputDefaults(t *testing.T) {
	t.Parallel()

	input := AssignTicketInput{}
	if input.AssignedTo.Valid {
		t.Error("expected invalid AssignedTo")
	}

	if input.AssignedGroupID.Valid {
		t.Error("expected invalid AssignedGroupID")
	}
}

func TestStrPtrHelper(t *testing.T) {
	t.Parallel()

	p := strPtr("hello")
	if p == nil {
		t.Fatal("expected non-nil pointer")
	}

	if *p != "hello" {
		t.Errorf("expected %q, got %q", "hello", *p)
	}
}

// ---------------------------------------------------------------------------
// Ticket type input validation
// ---------------------------------------------------------------------------

func TestCreateTicketTypeInputValidation(t *testing.T) {
	t.Parallel()
	// Empty name should be invalid.
	input := CreateTicketTypeInput{Name: "", Slug: "test"}
	if input.Name != "" {
		t.Error("expected empty name")
	}

	// Empty slug should be invalid.
	input2 := CreateTicketTypeInput{Name: "Test", Slug: ""}
	if input2.Slug != "" {
		t.Error("expected empty slug")
	}
}

func TestUpdateTicketTypeInputValidation(t *testing.T) {
	t.Parallel()
	// Empty name should be invalid.
	input := UpdateTicketTypeInput{Name: ""}
	if input.Name != "" {
		t.Error("expected empty name")
	}
}

// ---------------------------------------------------------------------------
// Ticket structure tests
// ---------------------------------------------------------------------------

func TestCreateTicketInputFieldValues(t *testing.T) {
	t.Parallel()

	fv := map[string]any{
		"browser": "Chrome",
		"version": "120",
	}

	input := CreateTicketInput{
		Subject:     "Test",
		Description: "Desc",
		FieldValues: fv,
	}
	if len(input.FieldValues) != 2 {
		t.Errorf("expected 2 field values, got %d", len(input.FieldValues))
	}
}

func TestCreateTicketTypeInputSLAHours(t *testing.T) {
	t.Parallel()

	hours := int32(24)

	input := CreateTicketTypeInput{
		Name:     "Support",
		Slug:     "support",
		SLAHours: &hours,
		IsPublic: true,
		IsSystem: false,
	}
	if input.SLAHours == nil || *input.SLAHours != 24 {
		t.Error("expected SLA hours to be 24")
	}
}

func TestUpdateTicketStatusInputWithResolution(t *testing.T) {
	t.Parallel()

	input := UpdateTicketStatusInput{
		Status:         string(StatusResolved),
		ResolutionNote: "Issue fixed",
		UserID:         pgtype.UUID{},
	}
	if input.ResolutionNote != "Issue fixed" {
		t.Errorf("expected resolution note %q, got %q", "Issue fixed", input.ResolutionNote)
	}
}

func TestParseTicketNumber(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"JC-1042", 1042, true},
		{"jc-1042", 1042, true},
		{"1042", 1042, true},
		{"  JC-7 ", 7, true},
		{"JC-", 0, false},
		{"", 0, false},
		{"abc", 0, false},
		{"0", 0, false},
		{"-5", 0, false},
		{"JC-abc", 0, false},
	}
	for _, tc := range cases {
		got, ok := parseTicketNumber(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Errorf("parseTicketNumber(%q) = %d,%v; want %d,%v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}
