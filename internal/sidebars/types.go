// Package sidebars implements the Sidebar Management System (Section 59, ADR-066).
//
// Sidebars are dedicated entities (not a special case of pages) that let admins
// manage left and right sidebars across the Main, Docs, and Learn frontend apps.
// Each sidebar is scoped by app + position + page-pattern glob, and can be gated
// by visibility (all/authenticated/guest) and required user groups. Sidebar widgets
// inherit the sidebar's gating and add their own widget-level gating. Widgets use
// a dedicated widget type registry (search, theme_toggle, trending_tags, etc.),
// NOT pagebuilder block types.
package sidebars

import "errors"

// Exported var values.
var (
	// ErrSidebarNotFound is a sentinel error: sidebar not found.
	ErrSidebarNotFound       = errors.New("sidebar not found")
	ErrSidebarWidgetNotFound = errors.New("sidebar widget not found")
	ErrInvalidApp            = errors.New("invalid app; must be main, docs, or learn")
	ErrInvalidPosition       = errors.New("invalid position; must be left or right")
	ErrInvalidVisibility     = errors.New("invalid visibility; must be all, authenticated, or guest")
	ErrInvalidStatus         = errors.New("invalid status; must be active or inactive")
	ErrInvalidWidgetType     = errors.New("invalid widget_type; not a registered sidebar widget type")
	ErrDuplicateSlug         = errors.New("sidebar slug already exists for this tenant/app/position")
	ErrInvalidPagePattern    = errors.New("invalid page_pattern; must not be empty")
	// ErrWidgetTypeNotAllowedForApp is returned when a widget type has no
	// renderer implementation in the sidebar's app (Section 59, ADR-066).
	ErrWidgetTypeNotAllowedForApp = errors.New("widget type not implemented for this app")
)

// App defines which frontend app a sidebar belongs to.
type App string

// Exported const values.
const (
	// AppMain is an exported constant.
	AppMain  App = "main"
	AppDocs  App = "docs"
	AppLearn App = "learn"
)

// Position defines whether the sidebar is on the left or right.
type Position string

// Exported const values.
const (
	// PositionLeft is an exported constant.
	PositionLeft  Position = "left"
	PositionRight Position = "right"
)

// Visibility controls which auth states see a sidebar or block.
type Visibility string

// Exported const values.
const (
	// VisibilityAll is an exported constant.
	VisibilityAll           Visibility = "all"
	VisibilityAuthenticated Visibility = "authenticated"
	VisibilityGuest         Visibility = "guest"
)

// Status represents the lifecycle state.
type Status string

// Exported const values.
const (
	// StatusActive is an exported constant.
	StatusActive   Status = "active"
	StatusInactive Status = "inactive"
)

// Pagination defaults.
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// ValidApp checks whether a string is a valid app identifier.
func ValidApp(s string) bool {
	return s == string(AppMain) || s == string(AppDocs) || s == string(AppLearn)
}

// ValidPosition checks whether a string is a valid position.
func ValidPosition(s string) bool {
	return s == string(PositionLeft) || s == string(PositionRight)
}

// ValidVisibility checks whether a string is a valid visibility.
func ValidVisibility(s string) bool {
	return s == string(VisibilityAll) || s == string(VisibilityAuthenticated) || s == string(VisibilityGuest)
}

// ValidStatus checks whether a string is a valid status.
func ValidStatus(s string) bool {
	return s == string(StatusActive) || s == string(StatusInactive)
}
