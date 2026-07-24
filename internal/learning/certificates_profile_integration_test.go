//go:build integration

package learning

import (
	"testing"

	"github.com/jedafe/japa/internal/testdb"
)

// TestListCertificatesByUser_TenantIsolated proves the cert list is tenant-scoped:
// a cert issued to the same user in another tenant must NOT leak into this tenant's
// results. Guards the multi-tenancy defect (query previously filtered user_id only).
func TestListCertificatesByUser_TenantIsolated(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantA := testdb.NewTenant(t, database)
	tenantB := testdb.NewTenant(t, database)
	userID := testdb.SeedUser(t, database, tenantA, testdb.UserOpts{Username: "cert-user"})

	testdb.SeedCertificate(t, database, tenantA, userID) // 1 cert in tenant A
	testdb.SeedCertificate(t, database, tenantB, userID) // 1 cert in tenant B, same user

	store := NewStore(database)
	got, err := store.ListCertificatesByUser(t.Context(), tenantA, userID)
	if err != nil {
		t.Fatalf("ListCertificatesByUser: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d certs for tenant A, want 1 — tenant isolation broken", len(got))
	}
}
