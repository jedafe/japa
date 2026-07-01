// Package spaces implements the Geo-First Space System for JapaConnect
// (Section 05).
//
// Spaces are the primary organizational unit for content. They form a
// hierarchy: country -> state -> city, with custom spaces for non-geo
// communities. Each space has five permission axes (visibility, posting,
// replying, commenting, joining) that control who can do what. Seven
// default permission presets are seeded and admin-editable.
//
// Space types (Country, City, University, Community) optionally link to
// custom field groups from Section 06 for type-specific fields.
package spaces

import (
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Space hierarchy levels. Country is the root, state and city nest under
// it, and custom covers non-geographic communities (universities, clubs).
const (
	LevelCountry = "country"
	LevelState   = "state"
	LevelCity    = "city"
	LevelCustom  = "custom"
)

// Visibility axis values. Public spaces are listed to everyone, members
// spaces only to logged-in members, and private spaces only to invited
// members.
const (
	VisibilityPublic  = "public"
	VisibilityMembers = "members"
	VisibilityPrivate = "private"
)

// Permission axis values shared by the posting, replying, and commenting
// axes. They describe the minimum role required to perform the action.
const (
	PermOpen       = "open"
	PermMembers    = "members"
	PermVerified   = "verified"
	PermModerators = "moderators"
	PermAdmins     = "admins"
)

// Joining axis values. Open lets anyone join immediately, request requires
// moderator approval, and invite means only invited users can join.
const (
	JoiningOpen    = "open"
	JoiningRequest = "request"
	JoiningInvite  = "invite"
)

// Space lifecycle status values.
const (
	StatusActive   = "active"
	StatusArchived = "archived"
	StatusDraft    = "draft"
)

// SpacePermAxes holds the five permission axes for a space. Each axis is
// a text value enforced by the permission-checking logic. A space moderator
// can override individual axes without changing the preset label.
type SpacePermAxes struct {
	Visibility  string
	Posting     string
	Replying    string
	Commenting  string
	Joining     string
}

// validLevels is the set of recognized space hierarchy levels.
var validLevels = map[string]bool{
	LevelCountry: true, LevelState: true, LevelCity: true, LevelCustom: true,
}

// IsValidLevel returns true if the given string is a recognized space level.
func IsValidLevel(level string) bool {
	return validLevels[level]
}

// validVisibility is the set of recognized visibility axis values.
var validVisibility = map[string]bool{
	VisibilityPublic: true, VisibilityMembers: true, VisibilityPrivate: true,
}

// validPostingPerm is the set of recognized posting, replying, and
// commenting axis values.
var validPostingPerm = map[string]bool{
	PermOpen: true, PermMembers: true, PermVerified: true,
	PermModerators: true, PermAdmins: true,
}

// validJoiningPerm is the set of recognized joining axis values.
var validJoiningPerm = map[string]bool{
	JoiningOpen: true, JoiningRequest: true, JoiningInvite: true,
}

// ValidPermissionAxes returns true if all five axis values are recognized.
// Posting, replying, and commenting share the same value set.
func ValidPermissionAxes(a SpacePermAxes) bool {
	return validVisibility[a.Visibility] &&
		validPostingPerm[a.Posting] &&
		validPostingPerm[a.Replying] &&
		validPostingPerm[a.Commenting] &&
		validJoiningPerm[a.Joining]
}

// DefaultPermAxes returns the default permission axes applied to a new
// space when no preset is specified: public visibility, members-only
// posting/replying/commenting, and open joining.
func DefaultPermAxes() SpacePermAxes {
	return SpacePermAxes{
		Visibility:  VisibilityPublic,
		Posting:     PermMembers,
		Replying:    PermMembers,
		Commenting:  PermMembers,
		Joining:     JoiningOpen,
	}
}

// Service provides space operations with reader/writer pool separation.
// Reads go through the reader pool; writes go through the writer pool.
// This follows the same pattern as the existing db.DB struct.
type Service struct {
	read  *gen.Queries
	write *gen.Queries
	db    *db.DB
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{
		read:  gen.New(database.Reader),
		write: gen.New(database.Writer),
		db:    database,
	}
}
