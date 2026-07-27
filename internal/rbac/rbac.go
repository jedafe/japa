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
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// permCacheTTL is how long permission entries live in Valkey before expiring.
// Matches the roadmap spec: 5 minutes.
const permCacheTTL = 5 * time.Minute

// permCacheKeyPrefix namespaces all RBAC cache keys in Valkey.
const permCacheKeyPrefix = "jc:perm:"

// SystemDefaultGroups are the three groups that cannot be deleted.
const (
	SystemGroupSuperAdmin = "super_admin"
	SystemGroupMembers    = "members"
	SystemGroupGuest      = "guest"
)

// SystemWildcardPermission is the special permission key that grants a user
// unrestricted access to every permission node. It is assigned only to the
// super-admin role(s); HasPermission short-circuits to true when present,
// so super-admin never needs the (redundant) exhaustive list of per-node grants.
const SystemWildcardPermission = "*"

// Permission represents a single permission node (e.g. "blog.publish").
type Permission struct {
	Key         string `json:"key"`
	Description string `json:"description"`
}

// Group represents a user group.
type Group struct {
	ID          pgtype.UUID `json:"id"`
	TenantID    pgtype.UUID `json:"tenant_id"`
	Name        string      `json:"name"`
	Slug        string      `json:"slug"`
	Description string      `json:"description"`
	IsSystem    bool        `json:"is_system"`
	MemberCount int32       `json:"member_count"`
	Permissions []string    `json:"permissions"`
}

// UserContext holds a user's groups and their resolved permissions.
// This is cached per-request to avoid repeated DB lookups.
type UserContext struct {
	Groups      []Group
	Permissions map[string]bool
}

// RBAC is the core RBAC resolver.
// L1 = in-memory map (fastest, per-process).
// L2 = Valkey (shared across server instances, 5min TTL).
// DB  = source of truth (queried only on L1+L2 miss).
type RBAC struct {
	read  *gen.Queries
	db    *db.DB
	cache *permissionCache
	vk    valkeycompat.Cmdable // nil when Valkey is unavailable (graceful degradation)
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

// New creates a new RBAC resolver backed by the given database and Valkey cache.
// When vkClient is nil, only the in-memory L1 cache is used (graceful
// degradation when Valkey is unreachable at startup).
// A background goroutine evicts L1 cache entries older than 60 seconds.
func New(database *db.DB, vkClient valkeycompat.Cmdable) *RBAC {
	r := &RBAC{
		read: gen.New(database.Reader),
		db:   database,
		cache: &permissionCache{
			entries: make(map[string]*cacheEntry),
		},
		vk: vkClient,
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

	// Wildcard: a user granted "*" implicitly holds every permission node,
	// so super-admin does not need an explicit grant per node.
	if uc.Permissions[SystemWildcardPermission] {
		return true
	}

	return uc.Permissions[permKey]
}

// HasSpacePermission checks if a user has a permission in the context of a
// specific space. For "spaces.moderator.assign", the permission is
// dynamically resolved by checking the space_moderators table at runtime
// (Section 03 / RBAC-001) — it is NOT a static grant in the RBAC table.
// For all other permission nodes, this falls back to HasPermission.
func (r *RBAC) HasSpacePermission(ctx context.Context, userID, tenantID, spaceID pgtype.UUID, permKey string) bool {
	// Static permissions resolve through the normal path.
	if permKey != "spaces.moderator.assign" {
		return r.HasPermission(ctx, userID, tenantID, permKey)
	}

	// Wildcard / super-admin short-circuit.
	uc, err := r.GetUserContext(ctx, userID, tenantID)
	if err != nil || uc == nil {
		return false
	}
	if uc.Permissions[SystemWildcardPermission] {
		return true
	}
	// If the user has the broader "spaces.moderate" (all spaces) node,
	// they don't need per-space assignment.
	if uc.Permissions["spaces.moderate"] {
		return true
	}

	// Dynamic resolution: check space_moderators for this user + space.
	// Per Section 03, the assignment record IS the grant — no static RBAC
	// entry is needed.
	if !spaceID.Valid {
		return false
	}
	if _, err := r.read.GetSpaceModerator(ctx, gen.GetSpaceModeratorParams{
		SpaceID: spaceID,
		UserID:  userID,
	}); err != nil {
		return false // not a moderator (or DB error — fail closed)
	}
	return true
}

// IsSuperAdmin checks if a user belongs to the super_admin group.
// Super admins bypass all permission checks.
func (r *RBAC) IsSuperAdmin(ctx context.Context, userID, tenantID pgtype.UUID) bool {
	inGroup, err := r.read.IsUserInGroup(ctx, gen.IsUserInGroupParams{
		TenantID:  tenantID,
		UserID:   userID,
		GroupSlug: SystemGroupSuperAdmin,
	})
	if err != nil {
		slog.Warn("rbac: failed to check super admin group", "error", err)
		return false
	}

	return inGroup
}

// adminCapabilityNodes lists the permission nodes that mark a user as holding
// admin-level capabilities. Per Section 46 ("A user with admin.impersonate
// cannot impersonate another user who holds any admin-level role"), any user
// holding one of these nodes — or the wildcard "*" — is considered
// admin-level and cannot be impersonated. The list mirrors the
// admin-panel section-visibility gates seeded by migration 000147 plus the
// wallet admin node, which together capture every role that can reach the
// admin panel or perform privileged admin actions.
var adminCapabilityNodes = []string{
	"admin.users.view",
	"admin.content.view",
	"admin.moderation.view",
	"admin.analytics.view",
	"admin.settings.view",
	"admin.roles.view",
	"admin.audit.view",
	"admin.advertising.view",
	"admin.payments.view",
	"admin.ai.view",
	"admin.feature_flags.view",
	"wallet.admin",
	"admin.impersonate",
}

// HasAdminCapability reports whether a user holds any admin-level permission
// node (or the wildcard "*"). Used by the impersonation subsystem (Section 46)
// to enforce the no-admin-on-admin-impersonation safety rule, and available
// for any other code path that needs to detect privileged users.
//
// Ref: japaconnect-engineering/sections/46-impersonation.md
func (r *RBAC) HasAdminCapability(ctx context.Context, userID, tenantID pgtype.UUID) bool {
	uc, err := r.GetUserContext(ctx, userID, tenantID)
	if err != nil || uc == nil {
		return false
	}

	// Wildcard grants every node, including admin ones.
	if uc.Permissions[SystemWildcardPermission] {
		return true
	}

	// Super-admin group membership is itself an admin capability.
	for _, g := range uc.Groups {
		if g.Slug == SystemGroupSuperAdmin {
			return true
		}
	}

	for _, key := range adminCapabilityNodes {
		if uc.Permissions[key] {
			return true
		}
	}

	return false
}

// GetUserContext loads a user's groups and resolved permissions.
// Lookup order: L1 (in-memory) → L2 (Valkey) → DB.
// Results are cached at both layers to avoid repeated DB queries.
func (r *RBAC) GetUserContext(ctx context.Context, userID, tenantID pgtype.UUID) (*UserContext, error) {
	cacheKey := userID.String()

	// L1 — in-memory (fastest, per-process)
	if cached := r.cacheGet(cacheKey); cached != nil {
		return cached, nil
	}

	// L2 — Valkey (shared across server instances)
	if uc := r.valkeyGet(ctx, cacheKey); uc != nil {
		// Promote to L1 so subsequent in-process calls skip Valkey.
		r.cacheSet(cacheKey, uc)
		return uc, nil
	}

	// Cache miss — query DB, then populate both layers.
	uc, err := r.loadUserContext(ctx, userID, tenantID)
	if err != nil {
		return nil, err
	}

	r.cacheSet(cacheKey, uc)
	r.valkeySet(ctx, cacheKey, uc)

	return uc, nil
}

// loadUserContext queries the database for the user's groups and permissions.
func (r *RBAC) loadUserContext(ctx context.Context, userID, tenantID pgtype.UUID) (*UserContext, error) {
	permRows, err := r.read.ResolveUserPermissions(ctx, gen.ResolveUserPermissionsParams{
		TenantID: tenantID,
		UserID:   userID,
	})
	if err != nil {
		return nil, fmt.Errorf("querying user permissions: %w", err)
	}

	permissions := make(map[string]bool, len(permRows))

	for _, key := range permRows {
		permissions[key] = true
	}

	groupRows, err := r.read.ResolveUserGroups(ctx, gen.ResolveUserGroupsParams{
		TenantID: tenantID,
		UserID:   userID,
	})
	if err != nil {
		return nil, fmt.Errorf("querying user groups: %w", err)
	}

	groups := make([]Group, 0, len(groupRows))

	for _, g := range groupRows {
		groups = append(groups, Group{
			ID:       g.ID,
			TenantID: g.TenantID,
			Name:     g.Name,
			Slug:     g.Slug,
			IsSystem: g.IsSystem,
		})
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

// valkeyGet retrieves a cached UserContext from Valkey (L2).
// Returns nil on cache miss or when Valkey is unavailable.
func (r *RBAC) valkeyGet(ctx context.Context, key string) *UserContext {
	if r.vk == nil {
		return nil
	}

	data, err := r.vk.Get(ctx, permCacheKeyPrefix+key).Bytes()
	if err != nil {
		// redis.Nil (miss) or connection error — treat as miss.
		return nil
	}

	var uc UserContext
	if err := json.Unmarshal(data, &uc); err != nil {
		slog.Warn("rbac: corrupt valkey cache entry", "key", key, "error", err)
		return nil
	}

	return &uc
}

// valkeySet stores a UserContext in Valkey with a 5-minute TTL.
// Failures are logged but never propagated — the L1 cache + DB remain
// authoritative.
func (r *RBAC) valkeySet(ctx context.Context, key string, uc *UserContext) {
	if r.vk == nil {
		return
	}

	data, err := json.Marshal(uc)
	if err != nil {
		slog.Warn("rbac: failed to marshal user context for valkey", "key", key, "error", err)
		return
	}

	if err := r.vk.Set(ctx, permCacheKeyPrefix+key, data, permCacheTTL).Err(); err != nil {
		slog.Warn("rbac: failed to write valkey cache", "key", key, "error", err)
	}
}

// InvalidateUserCache removes a single user's permission entries from both
// L1 and L2 caches. Called after group/role membership changes.
func (r *RBAC) InvalidateUserCache(ctx context.Context, userID pgtype.UUID) {
	key := userID.String()

	// Evict from L1.
	r.cache.mu.Lock()
	delete(r.cache.entries, key)
	r.cache.mu.Unlock()

	// Evict from L2.
	if r.vk != nil {
		if err := r.vk.Del(ctx, permCacheKeyPrefix+key).Err(); err != nil {
			slog.Warn("rbac: failed to invalidate valkey cache", "key", key, "error", err)
		}
	}
}

// InvalidateAllCache flushes every cached permission entry from both L1 and L2.
// Use sparingly — intended for admin-triggered full cache flushes only.
func (r *RBAC) InvalidateAllCache(ctx context.Context) {
	// Flush L1.
	r.cache.mu.Lock()
	r.cache.entries = make(map[string]*cacheEntry)
	r.cache.mu.Unlock()

	// Flush L2 — scan and delete all jc:perm:* keys.
	if r.vk == nil {
		return
	}

	var cursor uint64

	prefix := permCacheKeyPrefix + "*"

	for {
		keys, nextCursor, err := r.vk.Scan(ctx, cursor, prefix, 100).Result()
		if err != nil {
			slog.Warn("rbac: failed to scan valkey for cache flush", "error", err)
			return
		}

		if len(keys) > 0 {
			if err := r.vk.Del(ctx, keys...).Err(); err != nil {
				slog.Warn("rbac: failed to delete valkey cache keys", "count", len(keys), "error", err)
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
}

// ClearCache clears the permission cache.
func (r *RBAC) ClearCache() {
	r.cache.mu.Lock()
	defer r.cache.mu.Unlock()
	r.cache.entries = make(map[string]*cacheEntry)
}

// InvalidatePermCache removes a single user's permission entries from Valkey.
// Ref: japaconnect-engineering/sections/03-rbac.md (Amendment: Permission Cache Invalidation)
// Called after: group reassignment, role grant/revoke, ban create, moderator assignment/removal.
func InvalidatePermCache(ctx context.Context, v valkeycompat.Cmdable, userID pgtype.UUID) error {
	return v.Del(ctx, permCacheKeyPrefix+userID.String()).Err()
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
