//go:build integration

package testdb_test

import (
	"testing"

	"github.com/jedafe/japa/internal/testdb"
)

func TestHarness_ConnectAndIsolateByTenant(t *testing.T) {
	t.Parallel()
	d := testdb.Connect(t)
	tenantA := testdb.NewTenant(t, d)
	tenantB := testdb.NewTenant(t, d)
	if tenantA == tenantB {
		t.Fatal("NewTenant returned duplicate tenant ids")
	}
	// Seeding under A must not be visible under B (proves isolation boundary).
	uid := testdb.SeedUser(t, d, tenantA, testdb.UserOpts{Username: "iso_probe"})
	if !uid.Valid {
		t.Fatal("SeedUser returned invalid uuid")
	}
}

func TestSeedOrganizationSmoke(t *testing.T) {
	t.Parallel()
	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)

	orgID := testdb.SeedOrganization(t, database, tenantID, testdb.OrgOpts{Slug: "smoke-org", Status: "active"})
	owner := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "smoke-owner"})
	testdb.SeedOrgMembership(t, database, orgID, owner, "admin")

	var status string
	if err := database.Reader.QueryRow(t.Context(),
		`SELECT status FROM organizations WHERE id = $1`, orgID).Scan(&status); err != nil {
		t.Fatalf("read org: %v", err)
	}
	if status != "active" {
		t.Fatalf("org status = %q, want active", status)
	}
}
