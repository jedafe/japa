//go:build integration

package users_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jedafe/japa/internal/testdb"
	"github.com/jedafe/japa/internal/users"
)

// TestGetPublicProfileByUsername_OmitsPII verifies that GET /users/{username}'s
// service method returns a lean, PII-safe profile: correct follower/following
// counts (Section 31 / ADR-031) and no email/status/last_login_at/groups
// fields anywhere in the serialized response (Section 19 / ADR-014).
func TestGetPublicProfileByUsername_OmitsPII(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	d := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, d)
	uid := testdb.SeedUser(t, d, tenantID, testdb.UserOpts{Username: "ada"})
	testdb.SeedFollowers(t, d, tenantID, uid, 3) // 3 users follow ada
	testdb.SeedFollowing(t, d, tenantID, uid, 2) // ada follows 2

	got, err := users.NewService(d).GetPublicProfileByUsername(ctx, tenantID, "ada")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if got.Username != "ada" {
		t.Fatalf("username = %q, want ada", got.Username)
	}

	if got.FollowerCount != 3 || got.FollowingCount != 2 {
		t.Fatalf("counts = %d/%d, want 3/2", got.FollowerCount, got.FollowingCount)
	}

	b, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal profile: %v", err)
	}

	body := string(b)
	for _, k := range []string{`"email"`, `"status"`, `"last_login_at"`, `"groups"`, `"user_group_name"`} {
		if strings.Contains(body, k) {
			t.Fatalf("public profile leaked field %q: %s", k, body)
		}
	}
}
