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

// VerificationTypes is the §07 badge-type vocabulary shared by organizations
// and profiles. The requester may suggest one; the admin assigns the final
// type at grant time.
var VerificationTypes = []string{
	"business", "embassy", "notable", "immigration_lawyer", "professional_body",
}

// IsValidType reports whether s is a known verification badge type.
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
