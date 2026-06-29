// Package rbac implements the dynamic three-tier RBAC system for JapaConnect.
//
// The model is: User Groups -> User Roles -> Permissions.
// Every access decision resolves through HasPermission at runtime.
// No role name is ever hardcoded in business logic.
//
// Three system defaults exist and cannot be deleted: super_admin, members, guest.
// All other groups are admin-created.
package rbac

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
)

// SystemDefaultGroups are the three groups that cannot be deleted.
const (
	SystemGroupSuperAdmin = "super_admin"
	SystemGroupMembers    = "members"
	SystemGroupGuest      = "guest"
)

// Permission represents a single permission node (e.g. "blog.publish").
type Permission struct {
	Key         string
	Description string
}

// Group represents a user group.
type Group struct {
	ID       pgtype.UUID
	TenantID pgtype.UUID
	Name     string
	Slug     string
	IsSystem bool
}

// UserContext holds a user's groups and their resolved permissions.
// This is cached per-request to avoid repeated DB lookups.
type UserContext struct {
	Groups      []Group
	Permissions map[string]bool
}

// RBAC is the core RBAC resolver.
type RBAC struct {
	db    *db.DB
	cache *permissionCache
}

// cacheEntry wraps a UserContext with its creation time for TTL-based eviction.
type cacheEntry struct {
	user      *UserContext
	createdAt time.Time
}

// permissionCache provides in-memory caching of resolved permissions
// with automatic TTL-based eviction to prevent stale permissions.
type permissionCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
}

// New creates a new RBAC resolver backed by the given database.
// A background goroutine evicts cache entries older than 60 seconds.
func New(database *db.DB) *RBAC {
	r := &RBAC{
		db: database,
		cache: &permissionCache{
			entries: make(map[string]*cacheEntry),
		},
	}
	go r.cache.cleanupLoop(60 * time.Second)
	return r
}

// HasPermission checks if a user has a specific permission.
// This is the canonical access check - never hardcode role names.
//
// Usage:
//
//	if rbac.HasPermission(ctx, userID, tenantID, "blog.publish") {
//	    allowPublish()
//	}
func (r *RBAC) HasPermission(ctx context.Context, userID, tenantID pgtype.UUID, permKey string) bool {
	uc, err := r.GetUserContext(ctx, userID, tenantID)
	if err != nil || uc == nil {
		return false
	}
	return uc.Permissions[permKey]
}

// GetUserContext loads a user's groups and resolved permissions.
// Results are cached per-request to avoid repeated DB queries.
func (r *RBAC) GetUserContext(ctx context.Context, userID, tenantID pgtype.UUID) (*UserContext, error) {
	cacheKey := userID.String()
	if cached := r.cacheGet(cacheKey); cached != nil {
		return cached, nil
	}

	uc, err := r.loadUserContext(ctx, userID, tenantID)
	if err != nil {
		return nil, err
	}

	r.cacheSet(cacheKey, uc)
	return uc, nil
}

// loadUserContext queries the database for the user's groups and permissions.
func (r *RBAC) loadUserContext(ctx context.Context, userID, tenantID pgtype.UUID) (*UserContext, error) {
	query := `
		SELECT DISTINCT p.key
		FROM user_group_memberships ugm
		JOIN user_groups ug ON ugm.group_id = ug.id
		JOIN group_roles gr ON gr.group_id = ug.id
		JOIN user_roles ur ON gr.role_id = ur.id
		JOIN role_permissions rp ON rp.role_id = ur.id
		JOIN permissions p ON rp.permission_id = p.id
		WHERE ugm.tenant_id = $1 AND ugm.user_id = $2
	`
	rows, err := r.db.Reader.Query(ctx, query, tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("querying user permissions: %w", err)
	}
	defer rows.Close()

	permissions := make(map[string]bool)
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("scanning permission key: %w", err)
		}
		permissions[key] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating permission rows: %w", err)
	}

	groupQuery := `
		SELECT ug.id, ug.tenant_id, ug.name, ug.slug, ug.is_system
		FROM user_group_memberships ugm
		JOIN user_groups ug ON ugm.group_id = ug.id
		WHERE ugm.tenant_id = $1 AND ugm.user_id = $2
	`
	groupRows, err := r.db.Reader.Query(ctx, groupQuery, tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("querying user groups: %w", err)
	}
	defer groupRows.Close()

	var groups []Group
	for groupRows.Next() {
		var g Group
		if err := groupRows.Scan(&g.ID, &g.TenantID, &g.Name, &g.Slug, &g.IsSystem); err != nil {
			return nil, fmt.Errorf("scanning group: %w", err)
		}
		groups = append(groups, g)
	}
	if err := groupRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating group rows: %w", err)
	}

	return &UserContext{
		Groups:      groups,
		Permissions: permissions,
	}, nil
}

// cacheGet retrieves a cached UserContext if still valid.
func (r *RBAC) cacheGet(key string) *UserContext {
	r.cache.mu.RLock()
	defer r.cache.mu.RUnlock()
	entry, ok := r.cache.entries[key]
	if !ok {
		return nil
	}
	return entry.user
}

// cacheSet stores a UserContext in the cache with current timestamp.
func (r *RBAC) cacheSet(key string, uc *UserContext) {
	r.cache.mu.Lock()
	defer r.cache.mu.Unlock()
	r.cache.entries[key] = &cacheEntry{
		user:      uc,
		createdAt: time.Now(),
	}
}

// ClearCache clears the permission cache.
func (r *RBAC) ClearCache() {
	r.cache.mu.Lock()
	defer r.cache.mu.Unlock()
	r.cache.entries = make(map[string]*cacheEntry)
}

// cleanupLoop periodically evicts cache entries older than maxAge.
func (c *permissionCache) cleanupLoop(maxAge time.Duration) {
	ticker := time.NewTicker(maxAge)
	defer ticker.Stop()
	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for key, entry := range c.entries {
			if now.Sub(entry.createdAt) > maxAge {
				delete(c.entries, key)
			}
		}
		c.mu.Unlock()
	}
}

// IsSystemGroup checks if a group slug is one of the three system defaults.
func IsSystemGroup(slug string) bool {
	return slug == SystemGroupSuperAdmin || slug == SystemGroupMembers || slug == SystemGroupGuest
}
