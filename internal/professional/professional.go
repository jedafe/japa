// Package professional implements the Professional Profiles system for
// JapaConnect (Section 33).
//
// Professional Profiles represent structured public-facing entities — agencies,
// consultancies, practices, or brands — that a user owns and can post as.
// Users switch authorship context when creating content, choosing between
// posting as themselves or as one of their professional profiles.
//
// The Service struct wraps two sqlc-generated Queries instances: one reading
// from the reader pool and one writing to the writer pool.
package professional

import (
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Profile status values.
const (
	StatusActive  = "active"
	StatusInactive = "inactive"
	StatusBanned  = "banned"
)

// Member role values.
const (
	MemberRoleOwner  = "owner"
	MemberRoleAdmin  = "admin"
	MemberRoleEditor = "editor"
	MemberRoleMember = "member"
)

// Verification type values.
const (
	VerificationEmbassy  = "embassy"
	VerificationBusiness = "business"
	VerificationNotable  = "notable"
)

// Author context type values.
const (
	AuthorTypeUser    = "user"
	AuthorTypeProfile = "professional_profile"
)

// Relocation status values.
const (
	RelocationStatusPlanned     = "planned"
	RelocationStatusInProgress  = "in_progress"
	RelocationStatusCompleted   = "completed"
)

// Verifier status values.
const (
	VerifierStatusPending  = "pending"
	VerifierStatusVerified = "verified"
	VerifierStatusRejected = "rejected"
)

// Pagination defaults.
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// validStatuses is the set of all supported profile status values.
var validStatuses = map[string]bool{
	StatusActive:  true,
	StatusInactive: true,
	StatusBanned:  true,
}

// IsValidStatus returns true if the given status is a supported profile status.
func IsValidStatus(s string) bool {
	return validStatuses[s]
}

// validMemberRoles is the set of all supported member role values.
var validMemberRoles = map[string]bool{
	MemberRoleOwner:  true,
	MemberRoleAdmin:  true,
	MemberRoleEditor: true,
	MemberRoleMember: true,
}

// IsValidMemberRole returns true if the given role is a supported member role.
func IsValidMemberRole(r string) bool {
	return validMemberRoles[r]
}

// validVerificationTypes is the set of all supported verification type values.
var validVerificationTypes = map[string]bool{
	VerificationEmbassy:  true,
	VerificationBusiness: true,
	VerificationNotable:  true,
}

// IsValidVerificationType returns true if the given type is a supported verification type.
func IsValidVerificationType(t string) bool {
	return validVerificationTypes[t]
}

// validRelocationStatuses is the set of all supported relocation status values.
var validRelocationStatuses = map[string]bool{
	RelocationStatusPlanned:    true,
	RelocationStatusInProgress: true,
	RelocationStatusCompleted:  true,
}

// IsValidRelocationStatus returns true if the given status is a supported relocation status.
func IsValidRelocationStatus(s string) bool {
	return validRelocationStatuses[s]
}

// validVerifierStatuses is the set of all supported verifier status values.
var validVerifierStatuses = map[string]bool{
	VerifierStatusPending:  true,
	VerifierStatusVerified: true,
	VerifierStatusRejected: true,
}

// IsValidVerifierStatus returns true if the given status is a supported verifier status.
func IsValidVerifierStatus(s string) bool {
	return validVerifierStatuses[s]
}

// Service provides professional profile operations with reader/writer pool separation.
type Service struct {
	store *Store
	db    *db.DB
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{
		store: &Store{
			read:  gen.New(database.Reader),
			write: gen.New(database.Writer),
		},
		db: database,
	}
}
