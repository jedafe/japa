package sidebars

import (
	"context"
	"crypto/sha1" // #nosec
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// sidebarResolveCacheTTL is the TTL for cached sidebar resolution results.
// Ref: Section 59 §5 (Valkey Caching).
const sidebarResolveCacheTTL = 5 * time.Minute

// sidebarResolveCachePrefix is the Valkey key prefix for cached sidebar
// resolution results. Per Rule 4 + valkey-keys-catalog, all keys use the
// jc: prefix and are tenant-scoped.
const sidebarResolveCachePrefix = "jc:sidebar:resolve:"

// PermissionResolver resolves a user's full permission set in one call.
// rbac.RBAC satisfies this via UserPermissions (L1 in-memory → L2 Valkey → DB),
// so the public sidebar resolver pays one cached lookup per request instead of
// one HasPermission call per widget. Ref: Section 59 §4 step 5 (widget
// permission_node filtering); mirrors internal/menus.PermissionResolver.
type PermissionResolver interface {
	UserPermissions(ctx context.Context, userID, tenantID pgtype.UUID) (map[string]bool, error)
}

// Service provides business logic for managing sidebars and resolving them
// for the frontend apps.
type Service struct {
	store *Store
	db    *db.DB
	vk    valkeycompat.Cmdable
	perms PermissionResolver
}

func NewService(database *db.DB, vkClient valkeycompat.Cmdable) *Service {
	return &Service{
		store: &Store{read: gen.New(database.Reader), write: gen.New(database.Writer)},
		db:    database,
		vk:    vkClient,
	}
}

// NewServiceWithPerms wires the RBAC permission resolver so the public
// GET /sidebars/resolve endpoint can prune widgets whose permission_node is
// not held by the requesting user (Section 59 §4 step 5). Without this,
// widget-level permission_node gating is disabled (fail-open).
func NewServiceWithPerms(database *db.DB, vkClient valkeycompat.Cmdable, perms PermissionResolver) *Service {
	s := NewService(database, vkClient)
	s.perms = perms

	return s
}

// ── Sidebar CRUD ──────────────────────────────────────────────────────

// ListSidebarsResponse is the paginated list response for admin sidebars.
type ListSidebarsResponse struct {
	Items      []SidebarJSON `json:"items"`
	Page       int32         `json:"page"`
	TotalPages int32         `json:"total_pages"`
	Total      int32         `json:"total"`
}

func (s *Service) ListSidebars(ctx context.Context, tenantID pgtype.UUID, app, position, status, q, sort string, limit, offset int32) (*ListSidebarsResponse, error) {
	rows, err := s.store.listSidebars(ctx, tenantID, app, position, status, q, sort, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing sidebars: %w", err)
	}

	total, err := s.store.countSidebars(ctx, tenantID, app, position, status, q)
	if err != nil {
		return nil, fmt.Errorf("counting sidebars: %w", err)
	}

	totalPages := total / limit
	if total%limit > 0 {
		totalPages++
	}

	items := make([]SidebarJSON, len(rows))
	for i, row := range rows {
		items[i] = genListSidebarsRowToJSON(row)
	}

	return &ListSidebarsResponse{Items: items, Page: offset / limit, TotalPages: totalPages, Total: total}, nil
}

func (s *Service) GetSidebar(ctx context.Context, id, tenantID pgtype.UUID) (*SidebarJSON, error) {
	sidebar, err := s.store.getSidebarByID(ctx, id, tenantID)
	if err != nil {
		return nil, fmt.Errorf("getting sidebar: %w", err)
	}

	widgets, err := s.store.listSidebarWidgets(ctx, tenantID, sidebar.ID)
	if err != nil {
		return nil, fmt.Errorf("listing sidebar widgets: %w", err)
	}

	result := genSidebarToJSON(sidebar)
	result.Widgets = genSidebarWidgetsToJSON(widgets)

	return &result, nil
}

// CreateSidebarInput is the request body for creating a sidebar.
type CreateSidebarInput struct {
	App              string   `json:"app"`
	Position         string   `json:"position"`
	Slug             string   `json:"slug"`
	Name             string   `json:"name"`
	PagePattern      string   `json:"page_pattern"`
	Visibility       string   `json:"visibility"`
	RequiredGroupIDs []string `json:"required_group_ids"`
	Status           string   `json:"status"`
}

func (s *Service) CreateSidebar(ctx context.Context, tenantID pgtype.UUID, input CreateSidebarInput) (gen.Sidebar, error) {
	if !ValidApp(input.App) {
		return gen.Sidebar{}, ErrInvalidApp
	}

	if !ValidPosition(input.Position) {
		return gen.Sidebar{}, ErrInvalidPosition
	}

	if !ValidVisibility(input.Visibility) {
		return gen.Sidebar{}, ErrInvalidVisibility
	}

	if !ValidStatus(input.Status) {
		return gen.Sidebar{}, ErrInvalidStatus
	}

	if input.PagePattern == "" {
		input.PagePattern = "*"
	}

	groupIDsJSON, err := json.Marshal(input.RequiredGroupIDs)
	if err != nil {
		return gen.Sidebar{}, fmt.Errorf("encoding required_group_ids: %w", err)
	}

	if len(groupIDsJSON) == 0 {
		groupIDsJSON = []byte("[]")
	}

	sb, err := s.store.createSidebar(ctx, gen.CreateSidebarParams{
		TenantID:         tenantID,
		App:              input.App,
		Position:         input.Position,
		Slug:             input.Slug,
		Name:             input.Name,
		PagePattern:      input.PagePattern,
		Visibility:       input.Visibility,
		RequiredGroupIds: groupIDsJSON,
		Status:           input.Status,
	})
	if err != nil {
		return gen.Sidebar{}, err
	}

	s.invalidateResolveCache(ctx, tenantID)

	return sb, nil
}

// UpdateSidebarInput is the request body for updating a sidebar.
type UpdateSidebarInput struct {
	Name             string   `json:"name"`
	PagePattern      string   `json:"page_pattern"`
	Visibility       string   `json:"visibility"`
	RequiredGroupIDs []string `json:"required_group_ids"`
	Status           string   `json:"status"`
}

func (s *Service) UpdateSidebar(ctx context.Context, id, tenantID pgtype.UUID, input UpdateSidebarInput) (gen.Sidebar, error) {
	if !ValidVisibility(input.Visibility) {
		return gen.Sidebar{}, ErrInvalidVisibility
	}

	if !ValidStatus(input.Status) {
		return gen.Sidebar{}, ErrInvalidStatus
	}

	if input.PagePattern == "" {
		input.PagePattern = "*"
	}

	groupIDsJSON, err := json.Marshal(input.RequiredGroupIDs)
	if err != nil {
		return gen.Sidebar{}, fmt.Errorf("encoding required_group_ids: %w", err)
	}

	if len(groupIDsJSON) == 0 {
		groupIDsJSON = []byte("[]")
	}

	sb, err := s.store.updateSidebar(ctx, gen.UpdateSidebarParams{
		Name:             input.Name,
		PagePattern:      input.PagePattern,
		Visibility:       input.Visibility,
		RequiredGroupIds: groupIDsJSON,
		Status:           input.Status,
		ID:               id,
		TenantID:         tenantID,
	})
	if err != nil {
		return gen.Sidebar{}, err
	}

	s.invalidateResolveCache(ctx, tenantID)

	return sb, nil
}

func (s *Service) DeleteSidebar(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.store.deleteSidebar(ctx, id, tenantID); err != nil {
		return err
	}

	s.invalidateResolveCache(ctx, tenantID)

	return nil
}

// ── Sidebar Widget CRUD ───────────────────────────────────────────────

// CreateSidebarWidgetInput is the request body for creating a sidebar widget.
type CreateSidebarWidgetInput struct {
	WidgetType       string          `json:"widget_type"`
	Label            string          `json:"label"`
	Props            json.RawMessage `json:"props"`
	SortOrder        int             `json:"sort_order"`
	Visibility       string          `json:"visibility"`
	RequiredGroupIDs []string        `json:"required_group_ids"`
	PermissionNode   *string         `json:"permission_node"`
	Status           string          `json:"status"`
}

// UpdateSidebarWidgetInput is the request body for updating a sidebar widget.
type UpdateSidebarWidgetInput struct {
	WidgetType       string          `json:"widget_type"`
	Label            string          `json:"label"`
	Props            json.RawMessage `json:"props"`
	SortOrder        int             `json:"sort_order"`
	Visibility       string          `json:"visibility"`
	RequiredGroupIDs []string        `json:"required_group_ids"`
	PermissionNode   *string         `json:"permission_node"`
	Status           string          `json:"status"`
}

func (s *Service) CreateSidebarWidget(ctx context.Context, tenantID, sidebarID pgtype.UUID, input CreateSidebarWidgetInput) (gen.SidebarWidget, error) {
	if !IsValidWidgetType(input.WidgetType) {
		return gen.SidebarWidget{}, ErrInvalidWidgetType
	}

	if !ValidVisibility(input.Visibility) {
		return gen.SidebarWidget{}, ErrInvalidVisibility
	}

	if !ValidStatus(input.Status) {
		return gen.SidebarWidget{}, ErrInvalidStatus
	}

	var props []byte
	if len(input.Props) > 0 {
		props = sanitizeWidgetProps(input.WidgetType, input.Props)
	} else {
		props = []byte("{}")
	}

	groupIDsJSON, err := json.Marshal(input.RequiredGroupIDs)
	if err != nil {
		return gen.SidebarWidget{}, fmt.Errorf("encoding required_group_ids: %w", err)
	}

	if len(groupIDsJSON) == 0 {
		groupIDsJSON = []byte("[]")
	}

	widget, err := s.store.createSidebarWidget(ctx, gen.CreateSidebarWidgetParams{
		TenantID:   tenantID,
		SidebarID:  sidebarID,
		WidgetType: input.WidgetType,
		Label:      input.Label,
		Props:      props,
		// #nosec G115 // value bounded by caller
		SortOrder:        int32(input.SortOrder),
		Visibility:       input.Visibility,
		RequiredGroupIds: groupIDsJSON,
		PermissionNode:   input.PermissionNode,
		Status:           input.Status,
	})
	if err != nil {
		return gen.SidebarWidget{}, err
	}

	s.invalidateResolveCache(ctx, tenantID)

	return widget, nil
}

func (s *Service) UpdateSidebarWidget(ctx context.Context, id, sidebarID, tenantID pgtype.UUID, input UpdateSidebarWidgetInput) (gen.SidebarWidget, error) {
	if !IsValidWidgetType(input.WidgetType) {
		return gen.SidebarWidget{}, ErrInvalidWidgetType
	}

	if !ValidVisibility(input.Visibility) {
		return gen.SidebarWidget{}, ErrInvalidVisibility
	}

	if !ValidStatus(input.Status) {
		return gen.SidebarWidget{}, ErrInvalidStatus
	}
	// Verify the widget exists and belongs to the sidebar in the URL path,
	// preventing a within-tenant IDOR where a widget from sidebar B could be
	// mutated via /admin/sidebars/{sidebarA_id}/widgets/{widgetB_id}.
	// Ref: Bug Hunter's Guide §5.3 (RBAC & Multi-tenancy Escapes).
	existing, err := s.store.getSidebarWidget(ctx, id, tenantID)
	if err != nil {
		return gen.SidebarWidget{}, err
	}

	if existing.SidebarID != sidebarID {
		return gen.SidebarWidget{}, ErrSidebarWidgetNotFound
	}

	var props []byte
	if len(input.Props) > 0 {
		props = sanitizeWidgetProps(input.WidgetType, input.Props)
	} else {
		props = []byte("{}")
	}

	groupIDsJSON, err := json.Marshal(input.RequiredGroupIDs)
	if err != nil {
		return gen.SidebarWidget{}, fmt.Errorf("encoding required_group_ids: %w", err)
	}

	if len(groupIDsJSON) == 0 {
		groupIDsJSON = []byte("[]")
	}

	widget, err := s.store.updateSidebarWidget(ctx, gen.UpdateSidebarWidgetParams{
		Label:      input.Label,
		WidgetType: input.WidgetType,
		Props:      props,
		// #nosec G115 // value bounded by caller
		SortOrder:        int32(input.SortOrder),
		Visibility:       input.Visibility,
		RequiredGroupIds: groupIDsJSON,
		PermissionNode:   input.PermissionNode,
		Status:           input.Status,
		ID:               id,
		TenantID:         tenantID,
	})
	if err != nil {
		return gen.SidebarWidget{}, err
	}

	s.invalidateResolveCache(ctx, tenantID)

	return widget, nil
}

func (s *Service) DeleteSidebarWidget(ctx context.Context, id, sidebarID, tenantID pgtype.UUID) error {
	// Verify the widget exists and belongs to the sidebar in the URL path.
	// Ref: Bug Hunter's Guide §5.3 (RBAC & Multi-tenancy Escapes).
	existing, err := s.store.getSidebarWidget(ctx, id, tenantID)
	if err != nil {
		return err
	}

	if existing.SidebarID != sidebarID {
		return ErrSidebarWidgetNotFound
	}

	if err := s.store.deleteSidebarWidget(ctx, id, tenantID); err != nil {
		return err
	}

	s.invalidateResolveCache(ctx, tenantID)

	return nil
}

// ReorderInput is the request body for bulk reordering sidebar widgets.
type ReorderInput struct {
	Items []ReorderItem `json:"items"`
}

// ReorderItem is a single item in a reorder request.
type ReorderItem struct {
	ID        string `json:"id"`
	SortOrder int    `json:"sort_order"`
}

func (s *Service) ReorderSidebarWidgets(ctx context.Context, sidebarID, tenantID pgtype.UUID, items []ReorderItem) error {
	if len(items) == 0 {
		return nil
	}

	ids := make([]pgtype.UUID, len(items))
	sortOrders := make([]int32, len(items))

	for i, item := range items {
		if err := ids[i].Scan(item.ID); err != nil {
			return fmt.Errorf("invalid item id %s: %w", item.ID, err)
		}

		// #nosec G115 // value bounded by caller
		sortOrders[i] = int32(item.SortOrder)
	}
	// Verify every widget belongs to the sidebar in the URL path, preventing
	// a within-tenant IDOR where widgets from other sidebars could be
	// reordered via /admin/sidebars/{sidebarA_id}/widgets/reorder.
	// Ref: Bug Hunter's Guide §5.3 (RBAC & Multi-tenancy Escapes).
	for i := range ids {
		w, err := s.store.getSidebarWidget(ctx, ids[i], tenantID)
		if err != nil {
			return err
		}

		if w.SidebarID != sidebarID {
			return ErrSidebarWidgetNotFound
		}
	}

	if err := s.store.bulkUpdateSidebarWidgetSortOrder(ctx, tenantID, ids, sortOrders); err != nil {
		slog.Error("reorder sidebar widgets failed", "tenant_id", tenantID.String(), "error", err)
		return err
	}

	s.invalidateResolveCache(ctx, tenantID)

	return nil
}

// ── Public Resolution ──────────────────────────────────────────────────

// ResolveResponse is the response for the public resolve endpoint.
type ResolveResponse struct {
	Sidebar *SidebarJSON        `json:"sidebar"`
	Widgets []SidebarWidgetJSON `json:"widgets"`
}

// Resolve finds the best-matching sidebar for (app, position, path) and returns
// its widgets filtered by the user's auth state, group memberships, and permissions.
// userID is the authenticated user's ID (zero UUID for guests); it is used to
// resolve the user's permission set for widget-level permission_node pruning
// (Section 59 §4 step 5). Guests skip permission pruning.
func (s *Service) Resolve(ctx context.Context, tenantID pgtype.UUID, app, position, requestPath string, userGroupIDs map[string]bool, isAuthenticated bool, userID pgtype.UUID) (*ResolveResponse, error) {
	if !ValidApp(app) {
		return nil, ErrInvalidApp
	}

	if !ValidPosition(position) {
		return nil, ErrInvalidPosition
	}

	// Resolve the authenticated user's permission set once per request so
	// filterWidgets can prune widgets whose permission_node is not held.
	// ADR-066 / Section 59 §4 step 5. On resolution failure we fail open (no
	// permission pruning) rather than 500'ing the whole sidebar render — a
	// missing widget is preferable to a blank sidebar. The RBAC resolver is
	// cached, so this path is cheap. Guests skip permission pruning.
	//
	// cacheable tracks whether the resolved result is safe to cache. If perm
	// resolution failed for an authenticated user, the cache key collapses to
	// a shared "auth::" entry and the degraded (permission-gated widgets
	// missing) result could be served to other authenticated users who DO hold
	// those permissions — a cache-poisoning vector. We skip writing to the
	// cache in that case. Ref: Bug Hunter's Guide §4.2 (Caching).
	cacheable := true

	var userPerms map[string]bool

	if isAuthenticated && userID.Valid && s.perms != nil {
		perms, err := s.perms.UserPermissions(ctx, userID, tenantID)
		if err == nil {
			userPerms = perms
		} else {
			// Perm resolution failed — fail open for this request but do NOT
			// cache the degraded result.
			cacheable = false
		}
	}

	// Check Valkey cache first. Ref: Section 59 §5. The cache key includes a
	// perm_hash so two users with the same groups but different roles do not
	// share a cache entry (permission-gated widgets must not leak across users).
	cacheKey := sidebarResolveCacheKey(tenantID, app, position, requestPath, isAuthenticated, userGroupIDs, userPerms)
	if s.vk != nil && cacheKey != "" {
		if data, err := s.vk.Get(ctx, cacheKey).Bytes(); err == nil {
			var cached ResolveResponse
			if json.Unmarshal(data, &cached) == nil {
				return &cached, nil
			}
		}
	}

	candidates, err := s.store.listActiveSidebarsForResolve(ctx, tenantID, app, position)
	if err != nil {
		return nil, fmt.Errorf("listing active sidebars: %w", err)
	}

	// Filter by visibility + required_group_ids, then pick the most-specific page_pattern match.
	var bestSidebar *gen.Sidebar

	bestSpecificity := -1

	for i := range candidates {
		sb := &candidates[i]
		// Visibility filter.
		switch sb.Visibility {
		case "guest":
			if isAuthenticated {
				continue
			}
		case "authenticated":
			if !isAuthenticated {
				continue
			}
		}
		// Required group filter.
		if len(sb.RequiredGroupIds) > 0 && string(sb.RequiredGroupIds) != "null" && string(sb.RequiredGroupIds) != "[]" {
			var groupIDs []string
			if err := json.Unmarshal(sb.RequiredGroupIds, &groupIDs); err == nil && len(groupIDs) > 0 {
				matched := false

				for _, gid := range groupIDs {
					if userGroupIDs[gid] {
						matched = true
						break
					}
				}

				if !matched {
					continue
				}
			}
		}
		// Page pattern match.
		if !matchGlob(sb.PagePattern, requestPath) {
			continue
		}

		spec := patternSpecificity(sb.PagePattern)
		if spec > bestSpecificity {
			bestSpecificity = spec
			bestSidebar = sb
		}
	}

	if bestSidebar == nil {
		emptyResult := &ResolveResponse{Sidebar: nil, Widgets: []SidebarWidgetJSON{}}
		if cacheable && s.vk != nil && cacheKey != "" {
			if data, err := json.Marshal(emptyResult); err == nil {
				_ = s.vk.Set(ctx, cacheKey, data, sidebarResolveCacheTTL).Err()
			}
		}

		return emptyResult, nil
	}

	// Load widgets for the winning sidebar.
	widgets, err := s.store.listActiveSidebarWidgets(ctx, tenantID, bestSidebar.ID)
	if err != nil {
		return nil, fmt.Errorf("listing sidebar widgets: %w", err)
	}

	// Filter widgets by visibility, required_group_ids, and permission_node.
	filtered := filterWidgets(widgets, userGroupIDs, isAuthenticated, userPerms)

	sidebarJSON := genSidebarToJSON(*bestSidebar)
	result := &ResolveResponse{
		Sidebar: &sidebarJSON,
		Widgets: filtered,
	}

	// Cache the resolved result. Ref: Section 59 §5. Skip caching when perm
	// resolution failed for an authenticated user (cacheable=false) to avoid
	// poisoning the shared cache with a degraded, permission-pruned result.
	if cacheable && s.vk != nil && cacheKey != "" {
		if data, err := json.Marshal(result); err == nil {
			_ = s.vk.Set(ctx, cacheKey, data, sidebarResolveCacheTTL).Err()
		}
	}

	return result, nil
}

// GetUserGroupIDs returns the set of user group IDs the user belongs to.
func (s *Service) GetUserGroupIDs(ctx context.Context, tenantID pgtype.UUID, userID string) ([]string, error) {
	var uid pgtype.UUID
	if err := uid.Scan(userID); err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}

	return s.store.getUserGroupIDs(ctx, tenantID, uid)
}

// sidebarResolveCacheKey builds the Valkey cache key for a resolve request.
// Format: jc:sidebar:resolve:{tenant_id}:{app}:{position}:{path_hash}:{auth_state}:{group_hash}:{perm_hash}
// Ref: Section 59 §5, valkey-keys-catalog. The perm_hash ensures two users
// with the same groups but different roles do not share a cache entry —
// permission-gated widgets must not leak across users with different perms.
func sidebarResolveCacheKey(tenantID pgtype.UUID, app, position, requestPath string, isAuthenticated bool, userGroupIDs, userPerms map[string]bool) string {
	pathHash := fmt.Sprintf("%x", sha1.Sum([]byte(requestPath)))[:12] // #nosec

	authState := "guest"
	if isAuthenticated {
		authState = "auth"
	}

	groups := make([]string, 0, len(userGroupIDs))
	for gid := range userGroupIDs {
		groups = append(groups, gid)
	}

	sort.Strings(groups)
	groupHash := fmt.Sprintf("%x", sha1.Sum([]byte(strings.Join(groups, ","))))[:12] // #nosec

	permKeys := make([]string, 0, len(userPerms))
	for p := range userPerms {
		permKeys = append(permKeys, p)
	}

	sort.Strings(permKeys)
	permHash := fmt.Sprintf("%x", sha1.Sum([]byte(strings.Join(permKeys, ","))))[:12] // #nosec

	return fmt.Sprintf("%s%s:%s:%s:%s:%s:%s:%s", sidebarResolveCachePrefix, tenantID.String(), app, position, pathHash, authState, groupHash, permHash)
}

// invalidateResolveCache flushes all cached sidebar resolution results for a
// tenant. Called after every sidebar/widget CRUD mutation. Ref: Section 59 §5.
func (s *Service) invalidateResolveCache(ctx context.Context, tenantID pgtype.UUID) {
	if s.vk == nil {
		return
	}

	prefix := sidebarResolveCachePrefix + tenantID.String() + ":"

	var cursor uint64

	for {
		keys, nextCursor, err := s.vk.Scan(ctx, cursor, prefix+"*", 100).Result()
		if err != nil {
			slog.Warn("sidebars: failed to scan valkey for resolve cache invalidation", "error", err)
			return
		}

		if len(keys) > 0 {
			_ = s.vk.Del(ctx, keys...).Err()
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
}

// ── Helpers ───────────────────────────────────────────────────────────

// matchGlob matches a path against a glob pattern where * matches any
// sequence of characters (including /). A literal pattern matches exactly.
func matchGlob(pattern, p string) bool {
	if pattern == "*" {
		return true
	}

	return globMatch(pattern, p)
}

// globMatch implements * matching that crosses / boundaries.
func globMatch(pattern, p string) bool {
	pi, si := 0, 0
	starPi, starSi := -1, -1

	for si < len(p) {
		if pi < len(pattern) && (pattern[pi] == p[si]) {
			pi++
			si++
		} else if pi < len(pattern) && pattern[pi] == '*' {
			starPi = pi
			starSi = si
			pi++
		} else if starPi != -1 {
			pi = starPi + 1
			starSi++
			si = starSi
		} else {
			return false
		}
	}

	for pi < len(pattern) && pattern[pi] == '*' {
		pi++
	}

	return pi == len(pattern)
}

// patternSpecificity returns a score for how specific a glob pattern is.
// More non-wildcard characters = more specific. Ties broken by length.
func patternSpecificity(pattern string) int {
	nonWild := len(strings.ReplaceAll(pattern, "*", ""))
	return nonWild*10 + len(pattern)
}

// filterWidgets removes widgets the user should not see based on auth state,
// required group membership, and permission node. Only active widgets are returned.
// Authenticated users with a non-nil permission_node are pruned unless they hold
// that node in userPerms; guests skip permission_node pruning (ADR-066).
func filterWidgets(widgets []gen.SidebarWidget, userGroupIDs map[string]bool, isAuthenticated bool, userPerms map[string]bool) []SidebarWidgetJSON {
	var filtered []SidebarWidgetJSON

	for _, widget := range widgets {
		if widget.Status != "active" {
			continue
		}

		switch widget.Visibility {
		case "guest":
			if isAuthenticated {
				continue
			}
		case "authenticated":
			if !isAuthenticated {
				continue
			}
		}
		// Group-based gating.
		if len(widget.RequiredGroupIds) > 0 && string(widget.RequiredGroupIds) != "null" && string(widget.RequiredGroupIds) != "[]" {
			var groupIDs []string
			if err := json.Unmarshal(widget.RequiredGroupIds, &groupIDs); err == nil && len(groupIDs) > 0 {
				matched := false

				for _, gid := range groupIDs {
					if userGroupIDs[gid] {
						matched = true
						break
					}
				}

				if !matched {
					continue
				}
			}
		}
		// Permission-node gating (Section 59 §4 step 5). An authenticated user
		// sees a widget with a non-empty permission_node only if they hold that
		// node. Guests skip this (no permission check). A nil/empty
		// permission_node means no permission restriction.
		if isAuthenticated && widget.PermissionNode != nil && *widget.PermissionNode != "" {
			if userPerms == nil || !userPerms[*widget.PermissionNode] {
				continue
			}
		}

		filtered = append(filtered, genSidebarWidgetToJSON(widget))
	}

	if filtered == nil {
		filtered = []SidebarWidgetJSON{}
	}

	return filtered
}

// sanitizeWidgetProps decodes the raw widget props JSON, sanitizes any HTML
// string fields the widget type renders with {@html} on the frontend
// (rich_text.content, custom_html.html), and re-encodes. This is the
// server-side sanitization required by Bug Hunter's Guide §5.1/§14 — the
// frontend renders these fields with {@html}, so stored XSS must be blocked
// at the storage boundary. Uses the shared content.SanitizeHTML allowlist
// (the same sanitizer used for content posts in internal/renderer).
//
// Unknown widget types and non-HTML props pass through unchanged.
func sanitizeWidgetProps(widgetType string, raw json.RawMessage) []byte {
	if len(raw) == 0 {
		return []byte("{}")
	}
	// Only the rich_text and custom_html widget types carry user-authored HTML.
	if widgetType != "rich_text" && widgetType != "custom_html" {
		return raw
	}

	var props map[string]any
	if err := json.Unmarshal(raw, &props); err != nil {
		// Malformed props — fall back to empty object rather than persisting
		// something we can't reason about.
		return []byte("{}")
	}

	switch widgetType {
	case "rich_text":
		if v, ok := props["content"].(string); ok {
			props["content"] = content.SanitizeHTML(v)
		}
	case "custom_html":
		if v, ok := props["html"].(string); ok {
			props["html"] = content.SanitizeHTML(v)
		}
	}

	out, err := json.Marshal(props)
	if err != nil {
		return []byte("{}")
	}

	return out
}
