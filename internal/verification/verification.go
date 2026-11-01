// Package verification implements user-initiated verification badge requests
// (JEOS §07 Verification Requests): organizations and individual profiles
// submit evidence via the Form Builder, requests queue for admin review, and
// the badge grant stays a separate admin action (two-step).
//
// Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md
package verification

import (
	"context"
)

// Entity types stored on verification_requests.entity_type.
const (
	EntityTypeOrganization = "organization"
	EntityTypeUser         = "user"
)

// Request status values (same lifecycle vocabulary as ownership_claims).
const (
	StatusPending   = "pending"
	StatusApproved  = "approved"
	StatusRejected  = "rejected"
	StatusWithdrawn = "withdrawn"
)

// Pagination defaults.
const (
	DefaultPageSize int32 = 20
	MaxPageSize     int32 = 100
)

// Scopes on verification_types.scope (migration 001013). entity_type on a
// request maps 1:1 to the scope of the same name; service listings are
// verified out-of-band (no request flow) but share the taxonomy.
const (
	ScopeOrganization   = "organization"
	ScopeUser           = "user"
	ScopeServiceListing = "service_listing"
)

// VerificationTypes is the §07 badge-type vocabulary shared by organizations
// and profiles. It seeds the verification_types table and is the compiled-in
// fallback when the table is unavailable — the managed taxonomy is the
// source of truth at runtime.
var VerificationTypes = []string{
	"business", "embassy", "notable", "immigration_lawyer", "professional_body",
}

// ServiceListingTypes is the §98 listing-verification vocabulary — same
// fallback role for scope 'service_listing'.
var ServiceListingTypes = []string{
	"self", "community", "admin", "professional_body",
}

// FallbackTypeValues returns the compiled-in type vocabulary for a scope,
// used when verification_types cannot be read.
func FallbackTypeValues(scope string) []string {
	if scope == ScopeServiceListing {
		return ServiceListingTypes
	}
	return VerificationTypes
}

// IsValidType reports whether s is a known verification badge type
// (compiled §07 vocabulary — static callers/tests only; runtime validation
// goes through Service.IsValidTypeValue).
func IsValidType(s string) bool {
	for _, t := range VerificationTypes {
		if s == t {
			return true
		}
	}
	return false
}

// SubmitInput is the validated payload for POST /verification-requests.
type SubmitInput struct {
	EntityType       string
	OrganizationID   *string
	RequestedType    *string
	FormSubmissionID string
}

// ReviewInput is the admin decision payload for PATCH
// /admin/verification-requests/{id}.
type ReviewInput struct {
	Status         string
	ResolutionNote string
}

// WebhookEmitter fans verification_request.* / user.verified events out to
// subscribed endpoints (fire-and-forget, Section 12).
type WebhookEmitter interface {
	EmitEvent(ctx context.Context, eventType string, payload any)
}
