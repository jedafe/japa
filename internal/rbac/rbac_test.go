package rbac

import (
	"testing"
)

func TestIsSystemGroup(t *testing.T) {
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
			got := IsSystemGroup(tt.slug)
			if got != tt.want {
				t.Errorf("IsSystemGroup(%q) = %v, want %v", tt.slug, got, tt.want)
			}
		})
	}
}

func TestUserContext_PermissionCheck(t *testing.T) {
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
			got := uc.Permissions[tt.hasPerm]
			if got != tt.want {
				t.Errorf("permission check for %s = %v, want %v", tt.hasPerm, got, tt.want)
			}
		})
	}
}

func TestRBAC_ClearCache(t *testing.T) {
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
