// Package organizations implements the Organization Profiles system for
// JapaConnect (Section 07).
//
// Organizations represent structured entities — businesses, NGOs, government
// bodies, embassies, universities, media, or communities — that users own
// and manage. Members can be invited with specific roles, and unclaimed
// listings can be claimed via the ownership claim flow.
package organizations

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Organization status values.
const (
	StatusActive              = "active"
	StatusSuspended           = "suspended"
	StatusPendingVerification = "pending_verification"
)

// Organization type values.
const (
	OrgTypeBusiness   = "business"
	OrgTypeNGO        = "ngo"
	OrgTypeGovernment = "government"
	OrgTypeEmbassy    = "embassy"
	OrgTypeUniversity = "university"
	OrgTypeMedia      = "media"
	OrgTypeCommunity  = "community"
)

// Member role values.
const (
	MemberRoleOwner  = "owner"
	MemberRoleAdmin  = "admin"
	MemberRoleEditor = "editor"
	MemberRoleMember = "member"
)

// Invitation role values.
const (
	InvitationRoleAdmin  = "admin"
	InvitationRoleEditor = "editor"
	InvitationRoleMember = "member"
)

// Ownership claim status values.
const (
	ClaimStatusPending   = "pending"
	ClaimStatusApproved  = "approved"
	ClaimStatusRejected  = "rejected"
	ClaimStatusWithdrawn = "withdrawn"
)

// Pagination defaults.
const (
	DefaultPageSize int32 = 20
	MaxPageSize     int32 = 100
)

// SettingsReader is the subset of config.Settings used by the org service.
type SettingsReader interface {
	GetSettingInt(ctx context.Context, tenantID pgtype.UUID, key string) (int, error)
}

// Service provides organization business logic.
type Service struct {
	store    *Store
	db       *db.DB
	settings SettingsReader
}

// NewService creates a Service backed by the given database.
func NewService(database *db.DB) *Service {
	return &Service{
		store: NewStore(gen.New(database.Reader), gen.New(database.Writer), database.Writer),
		db:    database,
	}
}

// NewServiceWithSettings creates a Service with a settings reader for
// system_settings-backed gates (e.g. organizations.max_per_user).
func NewServiceWithSettings(database *db.DB, settings SettingsReader) *Service {
	return &Service{
		store:    NewStore(gen.New(database.Reader), gen.New(database.Writer), database.Writer),
		db:       database,
		settings: settings,
	}
}
