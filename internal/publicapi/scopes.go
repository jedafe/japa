// Package publicapi implements the Public API & Developer Portal (Section 47).
package publicapi

// ScopeOption describes a single API scope offered to developers when creating
// an API key. The catalog is the canonical list of scopes enforced by
// RequireScope — the scope value MUST match a string passed to RequireScope
// (or be a wildcard that ResolveScope honours) so the developer portal never
// offers a scope the middleware will reject.
//
// Ref: japaconnect-engineering/sections/47-public-api-developer-portal.md
type ScopeOption struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description"`
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
		Description: "Create and update content via API key (admin-approved)",
	},
	{
		Value:       "api.write.comments",
		Label:       "Write Comments",
		Description: "Post comments via API key (admin-approved)",
	},
	{
		Value:       "api.write.me",
		Label:       "Write Me",
		Description: "Update the authenticated user's own profile (admin-approved)",
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
	},
}

// AvailableScopes returns the scope catalog for the developer portal UI.
// It is the single source of truth consumed by both /developer/scopes and
// /control/api-scopes so the two endpoints can never drift apart.
func AvailableScopes() []ScopeOption {
	return ScopeCatalog
}
