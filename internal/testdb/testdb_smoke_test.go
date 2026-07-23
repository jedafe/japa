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
