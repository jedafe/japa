package affiliate

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestPaginateAffiliate(t *testing.T) {
	tests := []struct {
		name     string
		page     int32
		pageSize int32
		wantLim  int32
		wantOff  int32
	}{
		{"defaults", 0, 0, DefaultPageSize, 0},
		{"page 1", 1, 10, 10, 0},
		{"page 2", 2, 10, 10, 10},
		{"page 3 size 20", 3, 20, 20, 40},
		{"max page size", 1, MaxPageSize + 50, DefaultPageSize, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLim, gotOff := paginate(tt.page, tt.pageSize)
			if gotLim != tt.wantLim || gotOff != tt.wantOff {
				t.Errorf("paginate(%d, %d) = (%d, %d), want (%d, %d)",
					tt.page, tt.pageSize, gotLim, gotOff, tt.wantLim, tt.wantOff)
			}
		})
	}
}

func TestGenerateUniqueCode(t *testing.T) {
	code1, err := generateUniqueCode()
	if err != nil {
		t.Fatalf("generateUniqueCode failed: %v", err)
	}
	code2, err := generateUniqueCode()
	if err != nil {
		t.Fatalf("generateUniqueCode failed: %v", err)
	}
	if code1 == code2 {
		t.Error("generateUniqueCode should generate different codes")
	}
	if len(code1) != 12 || code1[:4] != "jph-" {
		t.Errorf("code should be 'jph-' + 8 hex chars, got %q (len=%d)", code1, len(code1))
	}
}

func TestValidProductTypes(t *testing.T) {
	types := ValidProductTypes()
	if len(types) != 3 {
		t.Errorf("expected 3 product types, got %d", len(types))
	}
	expected := map[string]bool{
		"pro_subscription": true,
		"course":          true,
		"general":         true,
	}
	for _, pt := range types {
		if !expected[pt] {
			t.Errorf("unexpected product type: %s", pt)
		}
	}
}

func TestMemberStatusConstants(t *testing.T) {
	if MemberStatusActive != "active" {
		t.Error("wrong MemberStatusActive")
	}
	if MemberStatusSuspended != "suspended" {
		t.Error("wrong MemberStatusSuspended")
	}
	if MemberStatusBanned != "banned" {
		t.Error("wrong MemberStatusBanned")
	}
}

func TestProductTypeConstants(t *testing.T) {
	if ProductTypeProSubscription != "pro_subscription" {
		t.Error("wrong ProductTypeProSubscription")
	}
	if ProductTypeCourse != "course" {
		t.Error("wrong ProductTypeCourse")
	}
	if ProductTypeGeneral != "general" {
		t.Error("wrong ProductTypeGeneral")
	}
}

func TestEventTypeConstants(t *testing.T) {
	if EventTypeClick != "click" {
		t.Error("wrong EventTypeClick")
	}
	if EventTypeConversion != "conversion" {
		t.Error("wrong EventTypeConversion")
	}
}

func TestErrors(t *testing.T) {
	if ErrMemberNotFound.Error() != "affiliate member not found" {
		t.Error("wrong error message")
	}
	if ErrAlreadyEnrolled.Error() != "user is already enrolled in the affiliate program" {
		t.Error("wrong error message")
	}
	if ErrNotEnrolled.Error() != "user is not enrolled in the affiliate program" {
		t.Error("wrong error message")
	}
	if ErrLinkNotFound.Error() != "affiliate link not found" {
		t.Error("wrong error message")
	}
	if ErrLinkNotActive.Error() != "affiliate link is not active" {
		t.Error("wrong error message")
	}
	if ErrTooManyLinks.Error() != "affiliate link limit reached" {
		t.Error("wrong error message")
	}
	if ErrInvalidProductType.Error() != "invalid product type" {
		t.Error("wrong error message")
	}
	if ErrMemberSuspended.Error() != "affiliate member is suspended or banned" {
		t.Error("wrong error message")
	}
}

func TestGetProgramInfo(t *testing.T) {
	svc := &Service{}
	info := svc.GetProgramInfo(nil)
	if info.CommissionRates.ProMonthly != 15 {
		t.Error("wrong ProMonthly rate")
	}
	if info.CommissionRates.ProAnnual != 20 {
		t.Error("wrong ProAnnual rate")
	}
	if info.CommissionRates.Course != 10 {
		t.Error("wrong Course rate")
	}
	if info.MinAccountDays != 30 {
		t.Error("wrong MinAccountDays")
	}
	if info.MaxLinksPerUser != 10 {
		t.Error("wrong MaxLinksPerUser")
	}
}

func TestToNullableUUID(t *testing.T) {
	u := toNullableUUID(nil)
	if u.Valid {
		t.Error("toNullableUUID(nil) should return invalid UUID")
	}
	id := pgtype.UUID{Bytes: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, Valid: true}
	u2 := toNullableUUID(&id)
	if !u2.Valid || u2 != id {
		t.Error("toNullableUUID should copy UUID value")
	}
}

func TestToTextFromRef(t *testing.T) {
	txt := toTextFromRef(nil)
	if txt.Valid {
		t.Error("toTextFromRef(nil) should return invalid text")
	}
	s := "https://example.com"
	txt2 := toTextFromRef(&s)
	if !txt2.Valid || txt2.String != s {
		t.Errorf("toTextFromRef should return text with value %q", s)
	}
}
