// Package publicapi implements the Public API & Developer Portal (Section 47).
package publicapi

import "github.com/jedafe/japa/internal/publicapi/scopes"

// The canonical scope catalog lives in the leaf package
// internal/publicapi/scopes so packages upstream of publicapi (rbac, reached
// through content/visibility) can consume it without importing publicapi —
// publicapi reads the moat domain services, which transitively reach rbac,
// and rbac importing publicapi would be an import cycle. The aliases below
// keep the publicapi.ScopeCatalog surface stable for existing consumers.
//
// Ref: japaconnect-engineering/sections/47-public-api-developer-portal.md.
type ScopeOption = scopes.ScopeOption

// ScopeCatalog is the canonical, spec-aligned list of API scopes (Section 47).
// See internal/publicapi/scopes for the definition.
var ScopeCatalog = scopes.ScopeCatalog

// AvailableScopes returns the full scope catalog (admin surface).
func AvailableScopes() []ScopeOption {
	return scopes.AvailableScopes()
}

// SelfServiceScopes returns the non-admin subset offered to self-serve
// key creation (/developer/scopes).
func SelfServiceScopes() []ScopeOption {
	return scopes.SelfServiceScopes()
}

// AnyAdminOnly reports whether any requested scope is an admin-only
// catalog entry.
func AnyAdminOnly(requested []string) bool {
	return scopes.AnyAdminOnly(requested)
}
