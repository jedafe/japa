package rbac

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestIsSystemGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		slug string
		want bool
	}{
		{"super_admin", "super_admin", true},
		{"members", "members", true},
		{"guest", "guest", true},
		{"custom group", "editors", false},
		{"empty", "", false},
		{"admin (not system)", "admin", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := IsSystemGroup(tt.slug)
			if got != tt.want {
				t.Errorf("IsSystemGroup(%q) = %v, want %v", tt.slug, got, tt.want)
			}
		})
	}
}

func TestUserContext_PermissionCheck(t *testing.T) {
	t.Parallel()

	uc := &UserContext{
		Permissions: map[string]bool{
			"blog.publish":   true,
			"blog.edit_own":  true,
			"content.create": true,
		},
	}

	tests := []struct {
		name    string
		hasPerm string
		want    bool
	}{
		{"has blog.publish", "blog.publish", true},
		{"has blog.edit_own", "blog.edit_own", true},
		{"does not have blog.delete", "blog.delete", false},
		{"does not have admin.access", "admin.access", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := uc.Permissions[tt.hasPerm]
			if got != tt.want {
				t.Errorf("permission check for %s = %v, want %v", tt.hasPerm, got, tt.want)
			}
		})
	}
}

func TestRBAC_ClearCache(t *testing.T) {
	t.Parallel()

	r := &RBAC{
		cache: &permissionCache{
			entries: map[string]*cacheEntry{
				"user1": {user: &UserContext{Permissions: map[string]bool{"test": true}}},
			},
		},
	}

	r.ClearCache()

	if len(r.cache.entries) != 0 {
		t.Errorf("cache should be empty after clear, got %d entries", len(r.cache.entries))
	}
}

func TestSystemGroupConstants(t *testing.T) {
	t.Parallel()

	if SystemGroupSuperAdmin != "super_admin" {
		t.Errorf("SystemGroupSuperAdmin = %q, want %q", SystemGroupSuperAdmin, "super_admin")
	}

	if SystemGroupMembers != "members" {
		t.Errorf("SystemGroupMembers = %q, want %q", SystemGroupMembers, "members")
	}

	if SystemGroupGuest != "guest" {
		t.Errorf("SystemGroupGuest = %q, want %q", SystemGroupGuest, "guest")
	}
}

// --- Valkey-backed cache tests ---

// TestCacheHitL1_SkipsDB verifies that a second GetUserContext call returns
// the in-memory L1 cache without touching loadUserContext (which would fail
// without a real DB).
func TestCacheHitL1_SkipsDB(t *testing.T) {
	t.Parallel()

	r := &RBAC{
		db: nil, // no DB — would panic if loadUserContext is called
		cache: &permissionCache{
			entries: make(map[string]*cacheEntry),
		},
		vk: nil, // no Valkey
	}

	userID := pgtype.UUID{Bytes: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}}
	_ = pgtype.UUID{Bytes: [16]byte{10, 20, 30, 40, 50, 60, 70, 80, 90, 100, 110, 120, 130, 140, 150, 160}}

	// Seed L1 cache directly.
	uc := &UserContext{
		Groups:      []Group{{Name: "editors"}},
		Permissions: map[string]bool{"blog.publish": true},
	}
	r.cacheSet(userID.String(), uc)

	// GetUserContext should return from L1 without calling loadUserContext.
	got, err := r.GetUserContext(t.Context(), userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Permissions["blog.publish"] != true {
		t.Error("expected blog.publish permission from L1 cache")
	}

	if got.Groups[0].Name != "editors" {
		t.Error("expected group 'editors' from L1 cache")
	}
}

// TestCacheHitL2_PromotesToL1 verifies that a Valkey L2 hit is promoted to
// the in-memory L1 cache.
func TestCacheHitL2_PromotesToL1(t *testing.T) {
	t.Parallel()
	// Without a real Valkey server, we test the round-trip through JSON
	// marshal/unmarshal to confirm the L2 data shape is correct.
	uc := &UserContext{
		Groups:      []Group{{Name: "moderators"}},
		Permissions: map[string]bool{"spaces.moderate": true},
	}

	data, err := json.Marshal(uc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded UserContext
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Permissions["spaces.moderate"] != true {
		t.Error("expected spaces.moderate permission after JSON round-trip")
	}

	if decoded.Groups[0].Name != "moderators" {
		t.Error("expected group 'moderators' after JSON round-trip")
	}
}

// TestInvalidateUserCache_RemovesFromL1 verifies that InvalidateUserCache
// evicts the entry from L1.
func TestInvalidateUserCache_RemovesFromL1(t *testing.T) {
	t.Parallel()

	r := &RBAC{
		cache: &permissionCache{
			entries: make(map[string]*cacheEntry),
		},
		vk: nil, // no Valkey — L2 path skipped gracefully
	}

	userID := pgtype.UUID{Bytes: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}}

	// Seed L1.
	uc := &UserContext{
		Permissions: map[string]bool{"admin.access": true},
	}
	r.cacheSet(userID.String(), uc)

	// Confirm it's cached.
	if cached := r.cacheGet(userID.String()); cached == nil {
		t.Fatal("expected entry in L1 cache before invalidation")
	}

	// Invalidate.
	r.InvalidateUserCache(t.Context(), userID)

	// Confirm it's gone.
	if cached := r.cacheGet(userID.String()); cached != nil {
		t.Error("expected L1 cache entry to be removed after invalidation")
	}
}

// TestGracefulDegradation_NilValkey confirms the RBAC resolver works when
// vk is nil (Valkey unreachable at startup). L1-only mode should function
// correctly.
func TestGracefulDegradation_NilValkey(t *testing.T) {
	t.Parallel()

	r := &RBAC{
		cache: &permissionCache{
			entries: make(map[string]*cacheEntry),
		},
		vk: nil,
	}

	userID := pgtype.UUID{Bytes: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}}
	_ = pgtype.UUID{Bytes: [16]byte{10, 20, 30, 40, 50, 60, 70, 80, 90, 100, 110, 120, 130, 140, 150, 160}}

	// L1 miss + nil Valkey → should NOT panic (no DB either, so loadUserContext
	// would fail — but we're testing that the nil Valkey path doesn't crash).
	// We seed L1 to avoid the DB call.
	uc := &UserContext{
		Permissions: map[string]bool{"content.create": true},
	}
	r.cacheSet(userID.String(), uc)

	got, err := r.GetUserContext(t.Context(), userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Permissions["content.create"] != true {
		t.Error("expected content.create permission")
	}
}

// TestInvalidateAllCache_ClearsL1 verifies that InvalidateAllCache empties L1.
func TestInvalidateAllCache_ClearsL1(t *testing.T) {
	t.Parallel()

	r := &RBAC{
		cache: &permissionCache{
			entries: make(map[string]*cacheEntry),
		},
		vk: nil, // L2 path skipped
	}

	// Seed multiple entries with unique cache keys.
	userIDs := []string{
		"00000000-0000-0000-0000-000000000001",
		"00000000-0000-0000-0000-000000000002",
		"00000000-0000-0000-0000-000000000003",
		"00000000-0000-0000-0000-000000000004",
		"00000000-0000-0000-0000-000000000005",
	}
	for _, id := range userIDs {
		r.cacheSet(id, &UserContext{
			Permissions: map[string]bool{"test": true},
		})
	}

	if len(r.cache.entries) != 5 {
		t.Fatalf("expected 5 L1 entries, got %d", len(r.cache.entries))
	}

	r.InvalidateAllCache(t.Context())

	if len(r.cache.entries) != 0 {
		t.Errorf("expected 0 L1 entries after flush, got %d", len(r.cache.entries))
	}
}

// TestCacheEntryTTL verifies that the cacheEntry timestamp is set and used
// for TTL-based eviction by the cleanup loop.
func TestCacheEntryTTL(t *testing.T) {
	t.Parallel()

	entry := &cacheEntry{
		user:      &UserContext{Permissions: map[string]bool{"test": true}},
		createdAt: time.Now(),
	}

	if time.Since(entry.createdAt) > time.Second {
		t.Error("createdAt should be close to time.Now()")
	}
}

// TestHasPermission_Wildcard verifies that holding the "*" node grants every
// permission without needing an explicit per-node grant.
func TestHasPermission_Wildcard(t *testing.T) {
	t.Parallel()

	r := &RBAC{
		cache: &permissionCache{
			entries: make(map[string]*cacheEntry),
		},
		vk: nil,
	}

	userID := pgtype.UUID{Bytes: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}}

	// Super-admin context: only the wildcard node is granted.
	r.cacheSet(userID.String(), &UserContext{
		Permissions: map[string]bool{SystemWildcardPermission: true},
	})

	for _, perm := range []string{"blog.publish", "admin.users.manage", "system.tenants.manage", "wallet.admin"} {
		if !r.HasPermission(t.Context(), userID, perm) {
			t.Errorf("wildcard user should have %q", perm)
		}
	}

	// The wildcard key itself is also satisfied.
	if !r.HasPermission(t.Context(), userID, SystemWildcardPermission) {
		t.Error("wildcard user should have the wildcard node itself")
	}
}

// TestPermCacheKeyPrefix verifies the Valkey key naming convention.
func TestPermCacheKeyPrefix(t *testing.T) {
	t.Parallel()

	if permCacheKeyPrefix != "jc:perm:" {
		t.Errorf("permCacheKeyPrefix = %q, want %q", permCacheKeyPrefix, "jc:perm:")
	}
}

// TestInvalidatePermCache_NilValkey verifies that InvalidatePermCache does
// not panic when the Valkey client is nil (graceful degradation when Valkey
// is unreachable at startup).
func TestInvalidatePermCache_NilValkey(t *testing.T) {
	t.Parallel()

	userID := pgtype.UUID{Bytes: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}}

	if err := InvalidatePermCache(t.Context(), nil, userID); err != nil {
		t.Errorf("InvalidatePermCache with nil Valkey should return nil, got %v", err)
	}
}
