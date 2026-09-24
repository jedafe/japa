// Package scopes is the leaf home of the public API scope catalog
// (Section 47). It is deliberately dependency-free: packages that sit
// upstream of publicapi in the import graph (e.g. rbac, reached through
// content/visibility) consume the catalog from here instead of importing
// publicapi, which would create an import cycle now that publicapi reads
// the moat domain services.
//
// Ref: japaconnect-engineering/sections/47-public-api-developer-portal.md.
package scopes

// ScopeOption describes a single API scope offered to developers when creating
// an API key. The catalog is the canonical list of scopes enforced by
// publicapi.RequireScope — the scope value MUST match a string passed to
// RequireScope (or be a wildcard that HasScope honours) so the developer
// portal never offers a scope the middleware will reject.
type ScopeOption struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description"`
	// AdminOnly scopes can only be granted by an admin (control panel or a
	// caller holding api.keys.manage). They are hidden from the self-serve
	// /developer/scopes catalog and rejected by CreateAPIKey for non-admin
	// callers (403 scope_requires_admin) so write access can never be
	// self-minted. Ref: §47 Phase 3 scope gating.
	AdminOnly bool `json:"admin_only,omitempty"`
}

// ScopeCatalog is the canonical, spec-aligned list of API scopes (Section 47).
// V1 public API is intentionally read-heavy; write scopes are approved by
// admin on a per-key basis. The wildcard entries (api.read.*, *) are included
// so admins can grant broad access without enumerating every leaf scope.
//
// Every leaf scope here is enforced by at least one RequireScope call in
// internal/server/server.go. Adding a new enforced scope requires adding it
// here too, otherwise the developer portal cannot offer it.
var ScopeCatalog = []ScopeOption{
	{
		Value:       "api.read.spaces",
		Label:       "Read Spaces",
		Description: "List and read spaces and communities",
	},
	{
		Value:       "api.read.content",
		Label:       "Read Content",
		Description: "Read published content and articles",
	},
	{
		Value:       "api.read.users.public",
		Label:       "Read Public Users",
		Description: "Read public user profiles",
	},
	{
		Value:       "api.read.tags",
		Label:       "Read Tags",
		Description: "Read tags",
	},
	{
		Value:       "api.read.me",
		Label:       "Read Me",
		Description: "Read the authenticated user's own data",
	},
	{
		Value:       "api.write.content",
		Label:       "Write Content",
		Description: "Create draft content via API key (admin-approved)",
		AdminOnly:   true,
	},
	{
		Value:       "api.write.bookmarks",
		Label:       "Write Bookmarks",
		Description: "Add and remove the key owner's bookmarks via API key (admin-approved)",
		AdminOnly:   true,
	},
	{
		Value:       "api.write.comments",
		Label:       "Write Comments",
		Description: "Post comments via API key (admin-approved)",
		AdminOnly:   true,
	},
	{
		Value:       "api.write.me",
		Label:       "Write Me",
		Description: "Update the authenticated user's own profile (admin-approved)",
		AdminOnly:   true,
	},
	{
		Value:       "api.read.visa",
		Label:       "Read Visa & Immigration",
		Description: "Read visa routes, processing timelines, credential pathways, and language test requirements",
	},
	{
		Value:       "api.read.education",
		Label:       "Read Education",
		Description: "Read scholarships and universities",
	},
	{
		Value:       "api.read.brief",
		Label:       "Read Destination Briefs",
		Description: "Read aggregate destination briefs (premium endpoint)",
	},
	{
		Value:       "api.read.safety",
		Label:       "Read Scam Alerts",
		Description: "Read confirmed scam reports and the scam alert schema",
	},
	{
		Value:       "api.read.search",
		Label:       "Semantic Search",
		Description: "Semantic (embedding) search over indexed content and spaces (premium endpoint)",
	},
	{
		Value:       "api.read.digest",
		Label:       "Read Digest",
		Description: "Read the personalized new-tab digest (extension aggregate endpoint)",
	},
	{
		Value:       "agent.use",
		Label:       "Use Agent Tools",
		Description: "Call MCP agent tools via /api/v1/mcp (read-only tool set for API keys)",
	},
	{
		Value:       "api.read.*",
		Label:       "Read All",
		Description: "Wildcard — read access to all read-scoped resources",
	},
	{
		Value:       "*",
		Label:       "All Access",
		Description: "Wildcard — full access to every scope (admin-approved only)",
		AdminOnly:   true,
	},
}

// AvailableScopes returns the full scope catalog — the single source of
// truth consumed by /control/api-scopes (admins need the complete list to
// grant write scopes).
func AvailableScopes() []ScopeOption {
	return ScopeCatalog
}

// SelfServiceScopes returns the subset of the catalog a non-admin caller may
// request on their own keys — the list served by /developer/scopes.
func SelfServiceScopes() []ScopeOption {
	out := make([]ScopeOption, 0, len(ScopeCatalog))
	for _, s := range ScopeCatalog {
		if !s.AdminOnly {
			out = append(out, s)
		}
	}
	return out
}

// AnyAdminOnly reports whether any of the requested scope strings is an
// admin-only catalog entry. Unknown scope strings are not admin-only — they
// are simply inert (no RequireScope gate will ever match them).
func AnyAdminOnly(requested []string) bool {
	admin := make(map[string]struct{})
	for _, s := range ScopeCatalog {
		if s.AdminOnly {
			admin[s.Value] = struct{}{}
		}
	}
	for _, r := range requested {
		if _, ok := admin[r]; ok {
			return true
		}
	}
	return false
}
