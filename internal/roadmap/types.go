// Package roadmap implements the public product roadmap (§113).
//
// Items move through a status lifecycle; community submissions land in
// pending and are triaged by moderators onto the public board. Votes are
// one-per-user; comments ride the §49 comments system via a hidden
// contents row per item.
//
// Ref: japaconnect-engineering/sections/113-public-product-roadmap.md
package roadmap

import (
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// ItemStatus is the lifecycle state of a roadmap item.
type ItemStatus string

// Roadmap item statuses. pending holds community submissions awaiting
// triage; under_review/planned/in_progress are public and votable;
// shipped items stay on the board inside roadmap.shipped_window_days and
// live permanently on the changelog; declined/archived are admin-only.
const (
	StatusPending     ItemStatus = "pending"
	StatusUnderReview ItemStatus = "under_review"
	StatusPlanned     ItemStatus = "planned"
	StatusInProgress  ItemStatus = "in_progress"
	StatusShipped     ItemStatus = "shipped"
	StatusDeclined    ItemStatus = "declined"
	StatusArchived    ItemStatus = "archived"
)

// PublicStatuses are the statuses rendered on the public board.
// Ref: §113 (Public board renders).
var PublicStatuses = []string{
	string(StatusUnderReview),
	string(StatusPlanned),
	string(StatusInProgress),
	string(StatusShipped),
}

// VotableStatuses are the only statuses that accept votes — voting on
// pending, shipped, declined, or archived items is rejected.
// Ref: §113 (Votable statuses).
var VotableStatuses = []string{
	string(StatusUnderReview),
	string(StatusPlanned),
	string(StatusInProgress),
}

// IsVotableStatus reports whether an item in the given status accepts votes.
func IsVotableStatus(s ItemStatus) bool {
	for _, v := range VotableStatuses {
		if string(s) == v {
			return true
		}
	}

	return false
}

// IsPublicStatus reports whether an item in the given status is reachable
// on public routes (board columns + item detail).
func IsPublicStatus(s ItemStatus) bool {
	for _, v := range PublicStatuses {
		if string(s) == v {
			return true
		}
	}

	return false
}

// AllStatuses returns every valid roadmap item status.
func AllStatuses() []ItemStatus {
	return []ItemStatus{
		StatusPending,
		StatusUnderReview,
		StatusPlanned,
		StatusInProgress,
		StatusShipped,
		StatusDeclined,
		StatusArchived,
	}
}

// ValidStatus reports whether s is one of the seven lifecycle statuses.
func ValidStatus(s string) bool {
	for _, v := range AllStatuses() {
		if string(v) == s {
			return true
		}
	}

	return false
}

// validTransitions defines the allowed status state machine transitions.
// Ref: §113 (Status Lifecycle).
var validTransitions = map[ItemStatus][]ItemStatus{
	StatusPending:     {StatusUnderReview, StatusDeclined},
	StatusUnderReview: {StatusPlanned, StatusDeclined, StatusArchived},
	StatusPlanned:     {StatusInProgress, StatusUnderReview, StatusDeclined, StatusArchived},
	StatusInProgress:  {StatusShipped, StatusPlanned, StatusArchived},
	StatusShipped:     {StatusArchived},
	StatusDeclined:    {StatusUnderReview, StatusArchived},
	StatusArchived:    {},
}

// IsValidTransition reports whether a status transition is allowed.
// A no-op transition (from == to) is permitted so idempotent PATCHes succeed.
func IsValidTransition(from, to ItemStatus) bool {
	if from == to {
		return true
	}

	for _, s := range validTransitions[from] {
		if s == to {
			return true
		}
	}

	return false
}

// Service-layer sentinel errors. ErrNotFound, ErrInvalidTransition, and
// ErrSubmissionsDisabled alias the shared errpkg sentinels so
// errpkg.HandleServiceError maps them without a package-local switch:
// not_found → 404, invalid_status_transition → 409, feature_disabled → 404.
var (
	ErrNotFound            = errpkg.ErrNotFound
	ErrAlreadyVoted        = errors.New("already_voted")
	ErrNotVoted            = errors.New("not_voted")
	ErrInvalidTransition   = errpkg.ErrInvalidStatusTransition
	ErrSubmissionsDisabled = errpkg.ErrFeatureDisabled
)

// Pagination defaults shared with the admin list endpoints.
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// boardColumnDef pairs a status with the public column label, in the order
// the columns render on the board. Ref: §113 (Frontend — status columns).
var boardColumnOrder = []struct {
	Status ItemStatus
	Label  string
}{
	{StatusUnderReview, "Under Consideration"},
	{StatusPlanned, "Planned"},
	{StatusInProgress, "In Progress"},
	{StatusShipped, "Recently Shipped"},
}

// BoardItem is one card on the public board.
type BoardItem struct {
	ID             pgtype.UUID  `json:"id"`
	Slug           string       `json:"slug"`
	Title          string       `json:"title"`
	Summary        *string      `json:"summary"`
	Status         string       `json:"status"`
	CategoryName   *string      `json:"category_name"`
	CategorySlug   *string      `json:"category_slug"`
	CategoryColor  *string      `json:"category_color"`
	VoteCount      int32        `json:"vote_count"`
	HasVoted       bool         `json:"has_voted"`
	IsFlagship     bool         `json:"is_flagship"`
	TargetWindow   *string      `json:"target_window"`
	FeatureFlagKey *string      `json:"feature_flag_key"`
	ContentID      *pgtype.UUID `json:"content_id"`
	ShippedAt      *time.Time   `json:"shipped_at"`
	CreatedAt      time.Time    `json:"created_at"`
}

// ItemDetail adds the description body for the detail page.
type ItemDetail struct {
	BoardItem
	Body *string `json:"body"`
}

// BoardColumn groups items under one status on the public board.
type BoardColumn struct {
	Status string      `json:"status"`
	Label  string      `json:"label"`
	Items  []BoardItem `json:"items"`
}

// CategoryDTO is the public category shape.
type CategoryDTO struct {
	ID          pgtype.UUID `json:"id"`
	Slug        string      `json:"slug"`
	Name        string      `json:"name"`
	Description *string     `json:"description"`
	Color       *string     `json:"color"`
	SortOrder   int32       `json:"sort_order"`
}

// Board is the GET /roadmap/board payload.
type Board struct {
	Columns    []BoardColumn `json:"columns"`
	Categories []CategoryDTO `json:"categories"`
}

// AdminListFilter carries the admin DataTable filters for list queries.
type AdminListFilter struct {
	Status         string
	Search         string
	Sort           string // recent | votes | title
	CategoryID     pgtype.UUID
	IncludeDeleted bool
	PageSize       int32
	PageOffset     int32
}

// AdminListResponse is the paginated admin list payload.
type AdminListResponse struct {
	Items      []gen.ListAdminRoadmapItemsRow `json:"items"`
	Total      int64                          `json:"total"`
	Page       int32                          `json:"page"`
	TotalPages int32                          `json:"total_pages"`
}
