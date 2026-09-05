// Package menus implements navigation menus with hierarchical, RBAC-gated
// menu items for JapaConnect (ADR-054).
//
// Menus have a placement (sidebar or bottom_bar) and contain a tree of
// menu items. Each item can be gated by permission_node (only shown to
// users with that RBAC permission) and visibility (all, authenticated, guest).
// Ad slots can be placed in the menu tree via is_ad_slot items.
package menus

import "errors"

var (
	ErrMenuNotFound      = errors.New("menu not found")
	ErrMenuItemNotFound  = errors.New("menu item not found")
	ErrMenuItemNotInMenu = errors.New("menu item does not belong to this menu")
	ErrInvalidPlacement  = errors.New("invalid placement; must be sidebar, bottom_bar, or dashboard_rail")
	ErrInvalidVisibility = errors.New("invalid visibility; must be all, authenticated, or guest")
	ErrInvalidStatus     = errors.New("invalid status; must be active or inactive")
	ErrInvalidTarget     = errors.New("invalid target; must be _self or _blank")
	ErrDuplicateSlug     = errors.New("menu slug already exists for this tenant")
)

// Placement defines where the menu appears in the UI.
type Placement string

const (
	PlacementSidebar       Placement = "sidebar"
	PlacementBottomBar     Placement = "bottom_bar"
	PlacementDropdown      Placement = "dropdown"
	PlacementFooter        Placement = "footer"
	PlacementHeader        Placement = "header"
	PlacementDashboardRail Placement = "dashboard_rail"
)

// Visibility controls which auth states see a menu item.
type Visibility string

const (
	VisibilityAll           Visibility = "all"
	VisibilityAuthenticated Visibility = "authenticated"
	VisibilityGuest         Visibility = "guest"
)

// Status represents the lifecycle state.
type Status string

const (
	StatusActive   Status = "active"
	StatusInactive Status = "inactive"
)

// Target controls how the link opens.
type Target string

const (
	TargetSelf  Target = "_self"
	TargetBlank Target = "_blank"
)

// Pagination defaults.
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// ValidPlacement checks whether a string is a valid placement.
func ValidPlacement(s string) bool {
	return s == string(PlacementSidebar) || s == string(PlacementBottomBar) ||
		s == string(PlacementDropdown) || s == string(PlacementFooter) ||
		s == string(PlacementHeader) || s == string(PlacementDashboardRail)
}

// ValidVisibility checks whether a string is a valid visibility.
func ValidVisibility(s string) bool {
	return s == string(VisibilityAll) || s == string(VisibilityAuthenticated) || s == string(VisibilityGuest)
}

// ValidStatus checks whether a string is a valid status.
func ValidStatus(s string) bool {
	return s == string(StatusActive) || s == string(StatusInactive)
}

// ValidTarget checks whether a string is a valid link target.
func ValidTarget(s string) bool {
	return s == string(TargetSelf) || s == string(TargetBlank)
}
