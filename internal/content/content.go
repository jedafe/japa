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
	"fmt"
	"log/slog"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/bookmarkfolders"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/literals"
	"github.com/jedafe/japa/internal/wallet"
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

// ErrInvalidStatus is returned when a create or update input carries a
// contents.status value outside the allowed set (§14). The HTTP layer
// validates this up front; non-HTTP callers (agent tools, imports, domain
// services) previously reached the DB CHECK constraint — the store now
// rejects the value first.
var ErrInvalidStatus = errors.New("invalid content status")

// ErrInvalidVisibility is returned when a create or update input carries a
// contents.visibility value outside the allowed set (§14).
var ErrInvalidVisibility = errors.New("invalid content visibility")

// defaultEditWindowMinutes is the fallback for content.edit_window_minutes
// when the setting is unset or no settings reader is wired (BL-008).
const defaultEditWindowMinutes = 1440

// isOrgPostRole reports whether an org membership role may post/edit as the org.
func isOrgPostRole(role string) bool {
	return role == "owner" || role == "admin" || role == "editor"
}

// Content status values stored in the contents.status column (§14).
// The moderation-hold state is "in_review" per the spec's CHECK constraint:
//
//	CHECK (status IN ('draft','in_review','scheduled','published','archived'))
//
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
// V1 types plus V2 types per §14 and V2 community types per §§79/88/98.
const (
	TypeBlogPost   = "blog_post"
	TypeDiscussion = "discussion"
	TypePage       = "page"
	TypeComparison = "comparison"
	TypeGuide      = "guide"
	// V2 types per §14.
	TypeEvent      = "event"
	TypeJobListing = "job_listing"
	// V2 community content types (§§79, 88, 98 — ADR-087/097/106).
	TypeStory          = "story"
	TypeAchievement    = "achievement"
	TypeServiceListing = "service_listing"
	// Video content type (§106 / ADR-117).
	TypeVideo = "video"
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
	TypeEvent:          true,
	TypeJobListing:     true,
	TypeStory:          true,
	TypeAchievement:    true,
	TypeServiceListing: true,
	// Video content type (§106 / ADR-117).
	TypeVideo: true,
}

// IsValidStatus reports whether s is a valid contents.status value.
func IsValidStatus(s string) bool { return validStatuses[s] }

// IsValidVisibility reports whether v is a valid contents.visibility value.
func IsValidVisibility(v string) bool { return validVisibilities[v] }

// IsValidContentType reports whether t is a recognised built-in content type.
func IsValidContentType(t string) bool { return validContentTypes[t] }

// ContentTypes returns the set of built-in content types.
//
//nolint:revive // exported: stutter naming is intentional
func ContentTypes() []string {
	out := make([]string, 0, len(validContentTypes))
	for t := range validContentTypes {
		out = append(out, t)
	}

	sort.Strings(out)

	return out
}

// ContentTypeLabels maps built-in content type slugs to human labels.
//
// Ref: japaconnect-engineering/sections/14-content-type-system.md.
var ContentTypeLabels = map[string]string{
	TypeBlogPost:       "Blog Post",
	TypeDiscussion:     "Discussion",
	TypePage:           "Page",
	TypeComparison:     "Comparison",
	TypeGuide:          "Guide",
	TypeEvent:          "Event",
	TypeJobListing:     "Job Listing",
	TypeStory:          "Story",
	TypeAchievement:    "Achievement",
	TypeServiceListing: "Service Listing",
}

// ContentTypeHasCategory reports whether a content type uses admin-configurable
// categories. Only guide does today.
//
// Ref: japaconnect-engineering/sections/14-content-type-system.md (Guide Content Type System).
//
//nolint:revive // exported: stutter naming is intentional
func ContentTypeHasCategory(contentType string) bool {
	return contentType == TypeGuide
}

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

// folderResolver resolves a default bookmark folder for an entity/content
// type pair. Implemented by bookmarkfolders.DefaultFolderResolver.
type folderResolver interface {
	Resolve(ctx context.Context, tenantID, userID pgtype.UUID, entityType, contentType string) (bookmarkfolders.FolderResponse, error)
}

// Service provides content operations with reader/writer pool
// separation. Reads go through the reader pool; writes go through the
// writer pool. This follows the same pattern as the existing db.DB struct.
type Service struct {
	db             *db.DB
	read           *gen.Queries
	write          *gen.Queries
	settings       settingsReader
	webhooks       webhookEmitter
	searcher       ContentSearcher
	spaceChecker   SpacePostChecker
	folderResolver folderResolver
	catalog        *literals.Catalog
	activity       wallet.ActivityEmitter
}

// SetActivityEmitter injects the wallet activity emitter used to record
// content.created events so §64 badge/challenge triggers (e.g. first_post)
// can fire. Optional — nil-safe.
func (s *Service) SetActivityEmitter(e wallet.ActivityEmitter) { s.activity = e }

// emitActivity records a wallet activity event and enqueues its processing.
// Fire-and-forget (Rule 8): failures are logged, never propagated — the
// content mutation is already settled by the time this runs. A duplicate
// reference hits ON CONFLICT DO NOTHING and returns ErrNoRows, which is
// treated as "already recorded" rather than an error.
func (s *Service) emitActivity(ctx context.Context, tenantID, userID pgtype.UUID, key, reference string, payload map[string]interface{}) {
	if s.activity == nil || !userID.Valid {
		return
	}

	event, err := s.activity.EmitActivityEvent(ctx, tenantID, userID, wallet.EmitActivityEventInput{
		ActivityKey: key,
		Reference:   reference,
		Payload:     payload,
	})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			slog.Warn("content: emit activity event failed", "activity_key", key, "error", err)
		}

		return
	}

	if err := s.activity.EnqueueProcessActivityEvent(ctx, event.ID, tenantID); err != nil {
		slog.Warn("content: enqueue activity event failed", "activity_key", key, "error", err)
	}
}

// ContentSearcher abstracts full-text search for content IDs.
// When wired (via SetSearcher), the content listing uses Meilisearch
// for ?q= queries instead of PostgreSQL ILIKE — providing typo tolerance,
// relevance ranking, and search-engine-grade matching.
// When not wired (nil), the service falls back to ILIKE filtering.
//
//nolint:revive // exported: stutter naming is intentional
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

// SetCatalog wires the shared literal catalog for resolving template literals
// in content titles and bodies.
func (s *Service) SetCatalog(c *literals.Catalog) {
	s.catalog = c
}

// SetFolderResolver injects the default bookmark folder resolver. Optional
// — when not set, bookmarks fall back to the "default" free-text folder.
func (s *Service) SetFolderResolver(r folderResolver) { s.folderResolver = r }

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

// GetSpacePrimaryGeoParents returns the country/state/city spaces linked to the
// given space via space_parent_spaces. Used when auto-assigning content to
// also inherit the geo parents of the space it is posted in.
func (s *Service) GetSpacePrimaryGeoParents(ctx context.Context, spaceID, tenantID pgtype.UUID) ([]pgtype.UUID, error) {
	ids, err := s.read.GetSpacePrimaryGeoParents(ctx, gen.GetSpacePrimaryGeoParentsParams{
		SpaceID:  spaceID,
		TenantID: tenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("getting space primary geo parents: %w", err)
	}

	return ids, nil
}

// HasSearcher is an exported method.
func (s *Service) HasSearcher() bool { return s.searcher != nil }

// emitWebhook fires a content.* event fire-and-forget (Rule 8). Errors are
// logged but never returned to the caller — webhook delivery is async.
func (s *Service) emitWebhook(ctx context.Context, tenantID pgtype.UUID, eventType string, payload any) {
	if s.webhooks == nil {
		return
	}

	s.webhooks.EmitEvent(ctx, tenantID, eventType, payload)
}
