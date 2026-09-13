//go:build integration

package tiers

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/testdb"
)

// newTierGroups creates the members, pro, and business tier groups a tenant
// needs for tier resolution tests.
func newTierGroups(t *testing.T, s *Service, tenantID pgtype.UUID) {
	t.Helper()
	ctx := context.Background()
	_, err := s.db.Writer.Exec(ctx, `
		INSERT INTO user_groups (id, name, slug, is_system, tier, max_spaces_creatable) VALUES
			($1, $4, 'Members', 'members', true, 'regular', 3),
			($2, $4, 'Pro', 'pro', true, 'pro', -1),
			($3, $4, 'Business', 'business', true, 'business', -1)
		ON CONFLICT (slug) DO NOTHING;
	`, mustUUID(t), mustUUID(t), mustUUID(t), tenantID)
	if err != nil {
		t.Fatalf("seed tier groups: %v", err)
	}
	_ = ctx
}

func mustUUID(t *testing.T) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := id.Scan(uuid.New().String()); err != nil {
		t.Fatalf("scan uuid: %v", err)
	}
	return id
}

func TestResolveUserTier(t *testing.T) {
	t.Parallel()

	db := testdb.Connect(t)
	s := NewService(db)
	tenantID := testdb.NewTenant(t, db)
	newTierGroups(t, s, tenantID)

	regularUser := testdb.SeedUser(t, db, tenantID, testdb.UserOpts{})
	proUser := testdb.SeedUser(t, db, tenantID, testdb.UserOpts{})
	bizUser := testdb.SeedUser(t, db, tenantID, testdb.UserOpts{})

	ctx := context.Background()

	if err := s.SetUserTier(ctx, tenantID, regularUser, "regular"); err != nil {
		t.Fatalf("set regular tier: %v", err)
	}
	if err := s.SetUserTier(ctx, tenantID, proUser, "pro"); err != nil {
		t.Fatalf("set pro tier: %v", err)
	}
	if err := s.SetUserTier(ctx, tenantID, bizUser, "business"); err != nil {
		t.Fatalf("set business tier: %v", err)
	}

	cases := []struct {
		userID pgtype.UUID
		want   string
	}{
		{regularUser, "regular"},
		{proUser, "pro"},
		{bizUser, "business"},
	}

	for _, c := range cases {
		got, err := s.ResolveUserTier(ctx, c.userID)
		if err != nil {
			t.Fatalf("resolve tier: %v", err)
		}
		if got != c.want {
			t.Fatalf("tier for %s = %s, want %s", c.userID.String(), got, c.want)
		}
	}
}
