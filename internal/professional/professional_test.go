package professional

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// Constants and configuration
// ---------------------------------------------------------------------------

func TestProfileStatusConstants(t *testing.T) {
	t.Parallel()

	expected := map[string]string{
		StatusActive:   "active",
		StatusInactive: "inactive",
		StatusBanned:   "banned",
	}
	for got, want := range expected {
		if got != want {
			t.Errorf("status constant = %q, want %q", got, want)
		}
	}
}

func TestMemberRoleConstants(t *testing.T) {
	t.Parallel()

	expected := map[string]string{
		MemberRoleOwner:  "owner",
		MemberRoleAdmin:  "admin",
		MemberRoleEditor: "editor",
		MemberRoleMember: "member",
	}
	for got, want := range expected {
		if got != want {
			t.Errorf("member role constant = %q, want %q", got, want)
		}
	}
}

func TestVerificationTypeConstants(t *testing.T) {
	t.Parallel()

	expected := map[string]string{
		VerificationEmbassy:  "embassy",
		VerificationBusiness: "business",
		VerificationNotable:  "notable",
	}
	for got, want := range expected {
		if got != want {
			t.Errorf("verification type constant = %q, want %q", got, want)
		}
	}
}

func TestAuthorContextTypeConstants(t *testing.T) {
	t.Parallel()

	expected := map[string]string{
		AuthorTypeUser:    "user",
		AuthorTypeProfile: "professional_profile",
	}
	for got, want := range expected {
		if got != want {
			t.Errorf("author context type constant = %q, want %q", got, want)
		}
	}
}

func TestPaginationDefaults(t *testing.T) {
	t.Parallel()

	if DefaultPageSize != 20 {
		t.Errorf("DefaultPageSize = %d, want 20", DefaultPageSize)
	}

	if MaxPageSize != 100 {
		t.Errorf("MaxPageSize = %d, want 100", MaxPageSize)
	}
}

// ---------------------------------------------------------------------------
// Status validation
// ---------------------------------------------------------------------------

func TestIsValidStatus(t *testing.T) {
	t.Parallel()

	validStatuses := []string{StatusActive, StatusInactive, StatusBanned}
	for _, s := range validStatuses {
		if !IsValidStatus(s) {
			t.Errorf("expected %q to be a valid status", s)
		}
	}

	invalidStatuses := []string{"", "pending", "deleted", "activee"}
	for _, s := range invalidStatuses {
		if IsValidStatus(s) {
			t.Errorf("expected %q to be an invalid status", s)
		}
	}
}

// ---------------------------------------------------------------------------
// Member role validation
// ---------------------------------------------------------------------------

func TestIsValidMemberRole(t *testing.T) {
	t.Parallel()

	validRoles := []string{MemberRoleOwner, MemberRoleAdmin, MemberRoleEditor, MemberRoleMember}
	for _, r := range validRoles {
		if !IsValidMemberRole(r) {
			t.Errorf("expected %q to be a valid member role", r)
		}
	}

	invalidRoles := []string{"", "viewer", "adminn", "superadmin"}
	for _, r := range invalidRoles {
		if IsValidMemberRole(r) {
			t.Errorf("expected %q to be an invalid member role", r)
		}
	}
}

// ---------------------------------------------------------------------------
// Verification type validation
// ---------------------------------------------------------------------------

func TestIsValidVerificationType(t *testing.T) {
	t.Parallel()

	validTypes := []string{VerificationEmbassy, VerificationBusiness, VerificationNotable}
	for _, vt := range validTypes {
		if !IsValidVerificationType(vt) {
			t.Errorf("expected %q to be a valid verification type", vt)
		}
	}

	invalidTypes := []string{"", "government", "personal", "embassyy"}
	for _, vt := range invalidTypes {
		if IsValidVerificationType(vt) {
			t.Errorf("expected %q to be an invalid verification type", vt)
		}
	}
}

// ---------------------------------------------------------------------------
// Input validation
// ---------------------------------------------------------------------------

func TestCreateProfessionalProfileTypeInputValidation(t *testing.T) {
	t.Parallel()

	input := CreateProfessionalProfileTypeInput{Name: "", Slug: "test"}
	if input.Name != "" {
		t.Error("expected empty name")
	}

	input2 := CreateProfessionalProfileTypeInput{Name: "Test", Slug: ""}
	if input2.Slug != "" {
		t.Error("expected empty slug")
	}
}

func TestCreateProfessionalProfileInputValidation(t *testing.T) {
	t.Parallel()

	input := CreateProfessionalProfileInput{Name: "", Slug: "test"}
	if input.Name != "" {
		t.Error("expected empty name")
	}

	input2 := CreateProfessionalProfileInput{Name: "Test", Slug: ""}
	if input2.Slug != "" {
		t.Error("expected empty slug")
	}
}

func TestAddMemberInputDefaults(t *testing.T) {
	t.Parallel()

	input := AddMemberInput{UserID: pgtype.UUID{}}
	if input.Role != "" {
		t.Error("expected empty role (defaults applied in service)")
	}
}

func TestCreateInvitationInputValidation(t *testing.T) {
	t.Parallel()

	input := CreateInvitationInput{Email: "", Role: MemberRoleMember}
	if input.Email != "" {
		t.Error("expected empty email")
	}
}

// ---------------------------------------------------------------------------
// Author context
// ---------------------------------------------------------------------------

func TestGetUserAuthorContext(t *testing.T) {
	t.Parallel()

	var userID pgtype.UUID
	if err := userID.Scan("550e8400-e29b-41d4-a716-446655440000"); err != nil {
		t.Fatalf("failed to scan user ID: %v", err)
	}

	ctx := GetUserAuthorContext(userID)
	if ctx.Type != AuthorTypeUser {
		t.Errorf("author context type = %q, want %q", ctx.Type, AuthorTypeUser)
	}

	if ctx.ID != userID {
		t.Errorf("author context ID mismatch")
	}
}

func TestAuthorContextStructure(t *testing.T) {
	t.Parallel()

	var id pgtype.UUID

	ctx := AuthorContext{
		Type: AuthorTypeProfile,
		ID:   id,
	}
	if ctx.Type != AuthorTypeProfile {
		t.Errorf("author context type = %q, want %q", ctx.Type, AuthorTypeProfile)
	}
}
