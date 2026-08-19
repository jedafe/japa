// Package content implements the Content Type System for JapaConnect
// (Section 14).
//
// The contents table uses a content_type discriminator with JSONB
// field_values for flexible per-type schemas. Every save creates a
// revision row; the live row always reflects the latest revision. Slug
// changes are tracked in slug_history for 301 redirects, preserving SEO
// equity. Reactions are polymorphic (entity_type/entity_id) and use an
// admin-configurable emoji set. Content scores (pgvector-driven) feed
// the Activity Feed sorting pipeline (Section 32).
//
// The Service struct wraps two sqlc-generated Queries instances: one
// reading from the reader pool and one writing to the writer pool.
package content

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// ErrOrgPostForbidden is returned when a caller tries to attribute content to
// an organization they are not an owner/admin/editor of (ADR-062).
var ErrOrgPostForbidden = errors.New("not authorized to post as this organization")

// ErrDuplicateContent is returned when an insert or update would produce a
// row with a content_hash that already exists for the same tenant (Rule 12 /
// BL-006). The unique index idx_contents_tenant_content_hash is the ultimate
// guard, but this explicit pre-check yields a clean 409 instead of a raw
// 23505 violation.
var ErrDuplicateContent = errors.New("duplicate content: a post with this content already exists")

// ErrEditWindowExpired is returned when the original author tries to edit
// content after the `content.edit_window_minutes` window has elapsed
// (BL-008 / Section 49). Moderators/admins editing someone else's post
// bypass this check because the editor is not the author.
var ErrEditWindowExpired = errors.New("edit window has expired")

// ErrMissingPrimarySpace is returned when a content item is published or
// scheduled without exactly one primary space assignment (Section 14 / BUG-007).
var ErrMissingPrimarySpace = errors.New("content requires a primary space")

// ErrInvalidSpace is returned when a content space assignment references a
// space that does not exist in the content's tenant (BUG-018).
var ErrInvalidSpace = errors.New("invalid space for content")

// ErrNotSpaceMember is returned when a user tries to assign a space they
// have not joined as an additional/primary space (Section 45 / BUG-018).
var ErrNotSpaceMember = errors.New("user is not a member of the requested space")

// defaultEditWindowMinutes is the fallback for content.edit_window_minutes
// when the setting is unset or no settings reader is wired (BL-008).
const defaultEditWindowMinutes = 1440

// isOrgPostRole reports whether an org membership role may post/edit as the org.
func isOrgPostRole(role string) bool {
	return role == "owner" || role == "admin" || role == "editor"
}

// Content status values stored in the contents.status column (§14).
// The moderation-hold state is "in_review" per the spec's CHECK constraint:
//   CHECK (status IN ('draft','in_review','scheduled','published','archived'))
// Migration 000494 renamed legacy "pending" rows to "in_review" and added
// the CHECK constraint that was missing from migration 000010.
const (
	StatusDraft     = "draft"
	StatusInReview  = "in_review"
	StatusPublished = "published"
	StatusScheduled = "scheduled"
	StatusArchived  = "archived"
)

// Built-in content types stored in the contents.content_type column.
// V2 types (event, job_listing, service_listing) are added later.
const (
	TypeBlogPost   = "blog_post"
	TypeDiscussion = "discussion"
	TypePage       = "page"
	TypeComparison = "comparison"
	TypeGuide      = "guide"
	// V2 community content types (§§79, 88, 98 — ADR-087/097/106).
	TypeStory          = "story"
	TypeAchievement    = "achievement"
	TypeServiceListing = "service_listing"
)

// Visibility values stored in the contents.visibility column.
const (
	VisibilityPublic  = "public"
	VisibilityMembers = "members"
	VisibilityPrivate = "private"
)

// validStatuses is the set of allowed values for contents.status (§14).
var validStatuses = map[string]bool{
	StatusDraft:     true,
	StatusInReview:  true,
	StatusPublished: true,
	StatusScheduled: true,
	StatusArchived:  true,
}

// validVisibilities is the set of allowed values for contents.visibility (§14).
var validVisibilities = map[string]bool{
	VisibilityPublic:  true,
	VisibilityMembers: true,
	VisibilityPrivate: true,
}

// validContentTypes is the set of built-in content types (§14 + v2 community
// types §§79/88/98). Admin-defined custom types are not listed here; the
// CreateContent handler accepts any type that the universal content system
// supports, but rejects clearly invalid values.
var validContentTypes = map[string]bool{
	TypeBlogPost:       true,
	TypeDiscussion:     true,
	TypePage:           true,
	TypeComparison:     true,
	TypeGuide:          true,
	TypeStory:          true,
	TypeAchievement:    true,
	TypeServiceListing: true,
}

// IsValidStatus reports whether s is a valid contents.status value.
func IsValidStatus(s string) bool { return validStatuses[s] }

// IsValidVisibility reports whether v is a valid contents.visibility value.
func IsValidVisibility(v string) bool { return validVisibilities[v] }

// IsValidContentType reports whether t is a recognised built-in content type.
func IsValidContentType(t string) bool { return validContentTypes[t] }

// EntityType constants for the polymorphic reactions and slug_history tables.
const (
	EntityContent = "content"
	EntitySpace   = "space"
	EntityTag     = "tag"
	EntityUser    = "user"
)

// DefaultPageSize is the default number of items returned by list queries
// when no explicit limit is provided.
const DefaultPageSize = 20

// MaxPageSize is the maximum number of items a client can request in a
// single list query. Larger values are clamped to prevent abuse.
const MaxPageSize = 100

// webhookEmitter fans a content.* event out to all subscribed webhook
// endpoints for the tenant (fire-and-forget). Ref: Section 12 (webhooks).
type webhookEmitter interface {
	EmitEvent(ctx context.Context, tenantID pgtype.UUID, eventType string, payload any)
}

// Service provides content operations with reader/writer pool
// separation. Reads go through the reader pool; writes go through the
// writer pool. This follows the same pattern as the existing db.DB struct.
type Service struct {
	db          *db.DB
	read        *gen.Queries
	write       *gen.Queries
	settings    settingsReader
	webhooks    webhookEmitter
	searcher    ContentSearcher
	spaceChecker SpacePostChecker
}

// ContentSearcher abstracts full-text search for content IDs.
// When wired (via SetSearcher), the content listing uses Meilisearch
// for ?q= queries instead of PostgreSQL ILIKE — providing typo tolerance,
// relevance ranking, and search-engine-grade matching.
// When not wired (nil), the service falls back to ILIKE filtering.
type ContentSearcher interface {
	SearchContentIDs(ctx context.Context, tenantID pgtype.UUID, query string, limit int) ([]pgtype.UUID, error)
}

// settingsReader is the subset of *config.Settings used by the content
// service for hot-reloadable operational limits (max tags, max pinned
// posts, etc.). Defined as an interface so tests can substitute a stub.
type settingsReader interface {
	GetIntOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback int) int
	GetFloatOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback float64) float64
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{
		db:    database,
		read:  gen.New(database.Reader),
		write: gen.New(database.Writer),
	}
}

// SetSettings wires the hot-reloadable settings reader. Optional — when
// not called, the service falls back to hardcoded defaults for any
// setting-governed limit.
func (s *Service) SetSettings(sr settingsReader) {
	s.settings = sr
}

// SetWebhookEmitter injects the content.* event fan-out (Section 12).
func (s *Service) SetWebhookEmitter(w webhookEmitter) { s.webhooks = w }

// SetSpaceChecker injects the space post-permission checker used to
// validate primary and additional space assignments (Section 45 / BUG-018).
// When nil, membership/permission checks are skipped.
func (s *Service) SetSpaceChecker(sc SpacePostChecker) { s.spaceChecker = sc }

// SetSearcher injects a Meilisearch-backed content searcher. When set,
// ListContents uses Meilisearch for ?q= queries instead of ILIKE.
// Pass nil to disable and fall back to ILIKE.
func (s *Service) SetSearcher(searcher ContentSearcher) { s.searcher = searcher }

// HasSearcher returns true when a Meilisearch content searcher is wired.
// Used by handlers to decide whether to use SearchContents or ListContents.
func (s *Service) HasSearcher() bool { return s.searcher != nil }

// emitWebhook fires a content.* event fire-and-forget (Rule 8). Errors are
// logged but never returned to the caller — webhook delivery is async.
func (s *Service) emitWebhook(ctx context.Context, tenantID pgtype.UUID, eventType string, payload any) {
	if s.webhooks == nil {
		return
	}
	s.webhooks.EmitEvent(ctx, tenantID, eventType, payload)
}
