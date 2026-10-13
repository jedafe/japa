// Package sidebars implements the Sidebar Management System (Section 59, ADR-066).
//
// Sidebars are dedicated entities (not a special case of pages) that let admins
// manage left and right sidebars across the Main, Docs, and Academy frontend apps.
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
	ErrInvalidApp            = errors.New("invalid app; must be main, docs, or academy")
	ErrInvalidPosition       = errors.New("invalid position; must be left or right")
	ErrInvalidVisibility     = errors.New("invalid visibility; must be all, authenticated, or guest")
	ErrInvalidStatus         = errors.New("invalid status; must be active or inactive")
	ErrInvalidWidgetType     = errors.New("invalid widget_type; not a registered sidebar widget type")
	ErrDuplicateSlug         = errors.New("sidebar slug already exists for this platform/app/position")
	ErrInvalidPagePattern    = errors.New("invalid page_pattern; must not be empty or blank")
	// ErrWidgetTypeNotAllowedForApp is returned when a widget type has no
	// renderer implementation in the sidebar's app (Section 59, ADR-066).
	ErrWidgetTypeNotAllowedForApp = errors.New("widget type not implemented for this app")
	// ErrLeftRailWidgetType is returned when a widget type other than ad_slot
	// is placed on a left-position sidebar. The left rail is a 160px ad-only
	// column (Section 59) — other types would resolve but never display.
	ErrLeftRailWidgetType = errors.New("left-position sidebars only support the ad_slot widget type")
	// ErrInvalidSort is returned when the admin list sort key is not in the
	// allowlist below. Unknown keys must 400, not silently fall back to the
	// default order (audit fix W11).
	ErrInvalidSort = errors.New("invalid sort; must be one of name, app, position, slug, page_pattern, status, updated_at, created_at")
)

// App defines which frontend app a sidebar belongs to.
type App string

// Exported const values.
const (
	// AppMain is an exported constant.
	AppMain    App = "main"
	AppDocs    App = "docs"
	AppAcademy App = "academy"
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
// "learn" is accepted as a legacy alias for "academy" (renamed in 000800)
// and normalized by normalizeApp before use; the DB CHECK stores academy.
func ValidApp(s string) bool {
	return s == string(AppMain) || s == string(AppDocs) || s == string(AppAcademy) || s == "learn"
}

// normalizeApp maps the legacy "learn" alias to "academy". Callers that
// persist or query by app must normalize first so legacy clients keep
// working while storage only ever sees the canonical value.
func normalizeApp(s string) string {
	if s == "learn" {
		return string(AppAcademy)
	}

	return s
}

// ValidSortKey reports whether a sort key (without the :dir suffix) is in
// the admin list allowlist. W11: unknown keys must 400 (see ValidateSort)
// instead of silently falling back to the default order.
func ValidSortKey(key string) bool {
	switch key {
	case "name", "app", "position", "slug", "page_pattern", "status", "updated_at", "created_at":
		return true
	default:
		return false
	}
}

// ValidateSort checks a composed "key" or "key:dir" sort value against the
// allowlist. Empty means default order. Returns ErrInvalidSort on unknown
// keys or directions.
func ValidateSort(sort string) error {
	if sort == "" {
		return nil
	}

	key := sort
	dir := ""
	if i := len(sort); i > 0 {
		for j := 0; j < len(sort); j++ {
			if sort[j] == ':' {
				key = sort[:j]
				dir = sort[j+1:]

				break
			}
		}
	}

	if !ValidSortKey(key) {
		return ErrInvalidSort
	}

	if dir != "" && dir != "asc" && dir != "desc" {
		return ErrInvalidSort
	}

	return nil
}

// ValidPagePattern rejects empty and whitespace-only page patterns.
// ErrInvalidPagePattern is otherwise dead code by design (flagged, never
// deleted per audit rule) — Create/Update default "" to "*" before this runs,
// so only an explicit blank slips through (audit fix W5).
func ValidPagePattern(pattern string) bool {
	if pattern == "" {
		return false
	}

	for _, r := range pattern {
		if r != ' ' && r != '\t' && r != '\n' && r != '\r' && r != '\v' && r != '\f' {
			return true
		}
	}

	return false
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
