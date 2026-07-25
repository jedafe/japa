package organizations

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

func TestToPublicOrg_LeanAndCorrect(t *testing.T) {
	tagline := "we help"
	row := gen.Organization{
		ID:            mustUUID(t, "11111111-1111-1111-1111-111111111111"),
		TenantID:      mustUUID(t, "22222222-2222-2222-2222-222222222222"),
		OwnerUserID:   mustUUID(t, "33333333-3333-3333-3333-333333333333"),
		Name:          "Acme Corp",
		Slug:          "acme",
		Tagline:       &tagline,
		OrgType:       "business",
		Verified:      true,
		FollowerCount: 12,
		MemberCount:   4,
		Status:        "active",
		FieldValues:   []byte(`{"secret":"x"}`),
		CreatedAt:     pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}

	c := toPublicOrg(row)
	if c.Name != "Acme Corp" || c.Slug != "acme" || !c.Verified || c.MemberCount != 4 {
		t.Fatalf("field-copy mismatch: %+v", c)
	}

	blob, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(blob)
	for _, leaked := range []string{"tenant_id", "owner_user_id", "field_values", "\"status\"", "deleted_at", "updated_at"} {
		if strings.Contains(got, leaked) {
			t.Errorf("PublicOrgResponse leaks %q: %s", leaked, got)
		}
	}
	for _, want := range []string{"\"id\"", "\"name\"", "\"slug\"", "member_count", "verified"} {
		if !strings.Contains(got, want) {
			t.Errorf("PublicOrgResponse missing %q: %s", want, got)
		}
	}
}
