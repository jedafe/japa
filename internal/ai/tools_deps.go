// Package ai — tools_deps.go defines the dependency interfaces and the
// shared ToolDeps container used by the Phase 4 core tool catalog handlers
// (Section 103, ADR-111).
// // Each tool handler is a closure over a *ToolDeps, registered with the
// ToolHandlerRegistry via RegisterHandler. The interfaces below are the
// minimal subsets of the real service signatures the handlers call, defined
// as interfaces so the handlers can be unit-tested with stubs instead of a
// live database, Meilisearch, or external HTTP API.
// // Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Tool Catalog Expansion)
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 4, tasks 4.01-4.10)
package ai

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/content/visibility"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/visitor"
)

// ---------------------------------------------------------------------------
// Service interfaces (minimal subsets the tool handlers call)
// ---------------------------------------------------------------------------

// ContentService is the subset of internal/content.Service the content tools
// call. Defined as an interface so handlers can be tested with a stub.
// The signatures match the real content.Service methods 1:1 so the wiring
// layer can assign the concrete service directly (no adapter needed).
type ContentService interface {
	// ListContents returns a tenant-wide text-filtered page of content.
	// cursor is a zero/invalid pgtype.Timestamptz for the first page.
	ListContents(ctx context.Context, q string, cursor pgtype.Timestamptz, limit int32, filter visibility.VisibilityFilter) ([]gen.ListContentsByTenantRow, error)
	// SearchContentsBySpace returns content within a space whose title
	// matches the query.
	SearchContentsBySpace(ctx context.Context, spaceID pgtype.UUID, query string, limit, offset int32, filter visibility.VisibilityFilter) ([]gen.SearchContentsBySpaceRow, error)
	// GetContent returns a single content row by ID.
	GetContent(ctx context.Context, id pgtype.UUID) (gen.GetContentByIDRow, error)
}

// CustomFieldsService is the subset of internal/customfields.Service the
// field tools call.
type CustomFieldsService interface {
	ListFieldsByGroup(ctx context.Context, groupID pgtype.UUID) ([]gen.ListCustomFieldsByGroupRow, error)
	ListFieldValuesByEntity(ctx context.Context, entityType string, entityID pgtype.UUID) ([]gen.CustomFieldValue, error)
	UpsertFieldValue(ctx context.Context, params gen.UpsertCustomFieldValueParams) (gen.CustomFieldValue, error)
}

// RAGService is the subset of internal/rag.Service the knowledge tools call.
type RAGService interface {
	Retrieve(ctx context.Context, tenantID pgtype.UUID, query, language string, geoSpaceID *pgtype.UUID) ([]RAGChunk, error)
}

// ComparisonService is the subset of internal/comparison.Service the
// comparison tools (Phase 7) call. Defined as an interface so handlers can
// be tested with a stub. The signatures match the real comparison.Service
// methods 1:1 so the wiring layer can assign the concrete service directly.
// // Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Comparison Engine)
// Ref: japaconnect-engineering/sections/30-generic-comparison-system.md.
type ComparisonService interface {
	// Type definitions.
	ListComparisonTypes(ctx context.Context, limit, offset int32) ([]gen.ListComparisonTypesByTenantRow, error)
	GetComparisonTypeBySlug(ctx context.Context, slug string) (gen.GetComparisonTypeBySlugRow, error)
	CreateComparisonType(ctx context.Context, input ComparisonTypeCreateInput) (gen.CreateComparisonTypeRow, error)

	// Comparison content (creates a content_type='comparison' row + items + dimensions).
	CreateComparison(ctx context.Context, userID pgtype.UUID, input ComparisonCreateInput) (ComparisonCreateResult, error)
	GetComparisonDetail(ctx context.Context, id pgtype.UUID) (ComparisonDetail, error)
	GetComparisonMatrix(ctx context.Context, contentID pgtype.UUID) ([]ComparisonMatrixItem, error)

	// Dimensions.
	CreateComparisonDimension(ctx context.Context, input ComparisonDimensionCreateInput) (gen.CreateComparisonDimensionRow, error)

	// Values (with provenance metadata for AI-filled values).
	UpsertComparisonValueWithMetadata(ctx context.Context, input ComparisonValueUpsertInput, metadata []byte) (gen.ComparisonValue, error)

	// Cache invalidation for JCE freshness/advisor after value mutations.
	DeleteJCEComparisonCaches(ctx context.Context, contentID, tenantID pgtype.UUID)
}

// JCEService is the subset of internal/jce.Service the JCE Sub-spec 4 AI
// advisor tools (analyze, rank, explain, recommend, refresh) call. The
// adapter in internal/jce implements this interface and converts the typed
// domain results to map[string]any for the agent.
// // Ref: docs/superpowers/plans/2026-08-30-jce-subspec-4.md (Task 8)
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Comparison Engine).
type JCEService interface {
	Analyze(ctx context.Context, contentID, userID, profileID pgtype.UUID, question string) (map[string]any, error)
	Rank(ctx context.Context, contentID, userID, profileID pgtype.UUID) (map[string]any, error)
	Explain(ctx context.Context, contentID, itemID, userID, profileID pgtype.UUID, factorKey, question string) (map[string]any, error)
	Recommend(ctx context.Context, contentID, userID, profileID pgtype.UUID) (map[string]any, error)
	Refresh(ctx context.Context, contentID, tenantID, userID pgtype.UUID, sourceKind *string) (map[string]any, error)
}

// ComparisonTypeCreateInput mirrors comparison.CreateComparisonTypeInput
// without importing internal/comparison (avoids a cycle: comparison imports
// content + customfields, and the ai package must stay leaf-adjacent).
type ComparisonTypeCreateInput struct {
	Name         string
	Slug         string
	Description  string
	Icon         string
	FieldGroupID pgtype.UUID
	ItemLabel    string
	IsSystem     bool
}

// ComparisonCreateInput mirrors comparison.CreateComparisonInput and the
// JCE CreateJCEComparisonInput. It supports both the legacy generic flow and
// the new quick/detailed JCE flow with polymorphic item_ref values.
type ComparisonCreateInput struct {
	Title      string
	Body       string
	Visibility string
	Status     string
	SpaceID    pgtype.UUID
	TypeID     pgtype.UUID
	Depth      string // 'quick' or 'detailed' (JCE). Empty → legacy generic flow.
	Items      []ComparisonItemInput
	Dimensions []string
}

// ComparisonItemInput mirrors comparison.ComparisonItemInput plus a JCE-style
// item_ref map for polymorphic entity resolution.
type ComparisonItemInput struct {
	Name    string
	Slug    string
	ItemRef map[string]any
}

// ComparisonCreateResult mirrors comparison.CreateComparisonResult / JCEComparisonResult.
type ComparisonCreateResult struct {
	ID pgtype.UUID `json:"id"`

	AuthorID   pgtype.UUID `json:"author_id"`
	Title      string      `json:"title"`
	Slug       string      `json:"slug"`
	Visibility string      `json:"visibility"`
	SpaceID    pgtype.UUID `json:"space_id"`
	Depth      string      `json:"depth"`
}

// ComparisonDimensionCreateInput mirrors comparison.CreateComparisonDimensionInput.
type ComparisonDimensionCreateInput struct {
	ContentID     pgtype.UUID
	Label         string
	DimensionType string
	Position      int32
}

// ComparisonValueUpsertInput mirrors comparison.UpsertComparisonValueInput.
type ComparisonValueUpsertInput struct {
	DimensionID  pgtype.UUID
	ItemID       pgtype.UUID
	Value        string
	ValueRating  *float64
	ValueBoolean *bool
}

// ComparisonDetail mirrors comparison.ComparisonDetail (the subset the AI
// tools surface to the LLM).
type ComparisonDetail struct {
	ID pgtype.UUID `json:"id"`

	AuthorID   pgtype.UUID                                `json:"author_id"`
	Title      string                                     `json:"title"`
	Slug       string                                     `json:"slug"`
	Body       string                                     `json:"body"`
	Visibility string                                     `json:"visibility"`
	Status     string                                     `json:"status"`
	SpaceID    pgtype.UUID                                `json:"space_id"`
	Items      []gen.ListComparisonItemsByContentRow      `json:"items"`
	Dimensions []gen.ListComparisonDimensionsByContentRow `json:"dimensions"`
}

// ComparisonMatrixItem mirrors comparison.ComparisonMatrixItem.
type ComparisonMatrixItem struct {
	Item       gen.ListComparisonItemsByContentRow `json:"item"`
	Dimensions []ComparisonMatrixDimension         `json:"dimensions"`
}

// ComparisonMatrixDimension mirrors comparison.ComparisonMatrixDimension.
type ComparisonMatrixDimension struct {
	DimensionID  pgtype.UUID `json:"dimension_id"`
	Label        string      `json:"label"`
	Type         string      `json:"type"`
	Value        *string     `json:"value"`
	ValueRating  *float64    `json:"value_rating"`
	ValueBoolean *bool       `json:"value_boolean"`
}

// RAGChunk is the provider-agnostic shape of rag.RetrievedChunk used by the
// rag.search tool. It mirrors rag.RetrievedChunk but is defined here so the
// ai package does not import internal/rag (avoiding a cycle when rag imports
// ai for the embedder adapter).
type RAGChunk struct {
	EntityID   string  `json:"entity_id"`
	EntityType string  `json:"entity_type"`
	ChunkText  string  `json:"chunk_text"`
	Similarity float64 `json:"similarity"`
	Tier       string  `json:"tier,omitempty"`
}

// SettingsReader is the subset of internal/config.Settings the tool handlers
// need for hot-reloadable system_settings lookups (Rule 3).
type SettingsReader interface {
	GetStringOr(ctx context.Context, key, fallback string) string
	GetIntOr(ctx context.Context, key string, fallback int) int
	GetFloatOr(ctx context.Context, key string, fallback float64) float64
	GetBoolOr(ctx context.Context, key string, fallback bool) bool
}

// CacheClient is the minimal Valkey client interface the external.search tool
// uses for the jc:agent:external:{hash} cache. nil disables caching.
type CacheClient interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string, ttlSeconds int) error
}

// PersonaResolver looks up an AI persona's backing users.id by persona ID.
// The content.create_draft tool (Phase 8) uses this to force author_id to
// the persona's backing user rather than the calling user. *personas.Store
// satisfies this via GetAIPersonaByID. nil disables the draft-request path.
// // Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Content Request Flow)
// Ref: japaconnect-engineering/sections/68-ai-personas.md (ai_personas.user_id).
type PersonaResolver interface {
	// GetPersonaUserID returns the backing users.id for the given persona
	// ID, scoped to the tenant. Returns pgx.ErrNoRows when the persona
	// does not exist in this tenant.
	GetPersonaUserID(ctx context.Context, personaID pgtype.UUID) (pgtype.UUID, error)
}

// HITLStore is the subset of *Store the content.create_draft tool uses to
// create a HITL review with source='agent_draft_request' + the created
// content_id. *Store satisfies this. nil disables HITL review creation
// (the executor's generic createHITL still runs, but with source=
// 'agent_tool' and no content_id).
type HITLStore interface {
	CreateHITLReview(ctx context.Context, review HITLReview) (HITLReview, error)
}

// ---------------------------------------------------------------------------
// Phase 9 service interfaces (Section 103 — extended tool catalog)
// ---------------------------------------------------------------------------

// SpacesService is the subset of internal/spaces.Service the Phase 9 space
// tools call. The signatures match the real spaces.Service methods 1:1 so
// the wiring layer can assign the concrete service directly.
// // Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Space/geo tools).
type SpacesService interface {
	FindCitySpaceByName(ctx context.Context, cityName string) (gen.FindCitySpaceByNameRow, error)
	GetSpaceByGeoCode(ctx context.Context, countryCode string) (gen.GetSpaceByGeoCodeRow, error)
	FindSpacesByKeywords(ctx context.Context, keywords []string) ([]gen.FindSpacesByKeywordsRow, error)
	GetSpace(ctx context.Context, id pgtype.UUID) (gen.GetSpaceByIDRow, error)
	ListSpaces(ctx context.Context, viewerUserID pgtype.UUID) ([]gen.ListSpacesByTenantRow, error)
	CreateSpace(ctx context.Context, params gen.CreateSpaceParams) (gen.CreateSpaceRow, error)
	UpdateSpace(ctx context.Context, params gen.UpdateSpaceParams) (gen.UpdateSpaceRow, error)
}

// LearningStore is the subset of internal/learning.Store the Phase 9 learn
// tools call. learning.Track and learning.Lesson are content-package types
// that would create an import cycle if imported here, so mirror types
// (LearningTrack, LearningLesson) are defined below and the wiring layer
// adapts between them.
// // Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Learn tools)
// Ref: japaconnect-engineering/sections/36-learning-platform.md.
type LearningStore interface {
	ListTracks(ctx context.Context) ([]LearningTrack, error)
	CreateTrack(ctx context.Context, input LearningCreateTrackInput) (*LearningTrack, error)
	UpdateTrack(ctx context.Context, id pgtype.UUID, input LearningUpdateTrackInput) (*LearningTrack, error)
	ListLessons(ctx context.Context) ([]LearningLesson, error)
	CreateLesson(ctx context.Context, moduleID pgtype.UUID, input LearningCreateLessonInput) (*LearningLesson, error)
	UpdateLesson(ctx context.Context, id pgtype.UUID, input LearningUpdateLessonInput) (*LearningLesson, error)
	ReorderLessons(ctx context.Context, entries []LearningReorderEntry) error
}

// LearningTrack mirrors learning.Track (the subset the AI tools surface).
type LearningTrack struct {
	ID pgtype.UUID `json:"id"`

	Title       string             `json:"title"`
	Slug        string             `json:"slug"`
	Description *string            `json:"description,omitempty"`
	Difficulty  string             `json:"difficulty"`
	IsPublished bool               `json:"is_published"`
	SortOrder   int32              `json:"sort_order"`
	CreatedAt   pgtype.Timestamptz `json:"created_at"`
	UpdatedAt   pgtype.Timestamptz `json:"updated_at"`
}

// LearningCreateTrackInput mirrors learning.CreateTrackInput.
type LearningCreateTrackInput struct {
	Title                string      `json:"title"`
	Slug                 string      `json:"slug"`
	Description          *string     `json:"description,omitempty"`
	Difficulty           string      `json:"difficulty"`
	CoverMediaID         pgtype.UUID `json:"cover_media_id"`
	IsPublished          bool        `json:"is_published"`
	SortOrder            int32       `json:"sort_order"`
	Category             *string     `json:"category,omitempty"`
	SubscriptionEligible bool        `json:"subscription_eligible,omitempty"`
}

// LearningUpdateTrackInput mirrors learning.UpdateTrackInput.
type LearningUpdateTrackInput struct {
	Title                string      `json:"title"`
	Description          *string     `json:"description,omitempty"`
	Difficulty           string      `json:"difficulty"`
	CoverMediaID         pgtype.UUID `json:"cover_media_id"`
	IsPublished          bool        `json:"is_published"`
	SortOrder            int32       `json:"sort_order"`
	Category             *string     `json:"category,omitempty"`
	SubscriptionEligible *bool       `json:"subscription_eligible,omitempty"`
}

// LearningLesson mirrors learning.Lesson (the subset the AI tools surface).
type LearningLesson struct {
	ID pgtype.UUID `json:"id"`

	ModuleID        pgtype.UUID        `json:"module_id"`
	Title           string             `json:"title"`
	Slug            string             `json:"slug"`
	LessonType      string             `json:"lesson_type"`
	Content         *string            `json:"content,omitempty"`
	DurationMinutes *int32             `json:"duration_minutes,omitempty"`
	SortOrder       int32              `json:"sort_order"`
	IsPublished     bool               `json:"is_published"`
	IsPreview       bool               `json:"is_preview"`
	CreatedAt       pgtype.Timestamptz `json:"created_at"`
	UpdatedAt       pgtype.Timestamptz `json:"updated_at"`
}

// LearningCreateLessonInput mirrors learning.CreateLessonInput.
type LearningCreateLessonInput struct {
	Title           string      `json:"title"`
	Slug            string      `json:"slug"`
	LessonType      string      `json:"lesson_type"`
	Content         *string     `json:"content,omitempty"`
	MediaID         pgtype.UUID `json:"media_id"`
	DurationMinutes *int32      `json:"duration_minutes,omitempty"`
	SortOrder       int32       `json:"sort_order"`
	IsPublished     bool        `json:"is_published"`
	IsPreview       bool        `json:"is_preview"`
}

// LearningUpdateLessonInput mirrors learning.UpdateLessonInput.
type LearningUpdateLessonInput struct {
	Title           string      `json:"title"`
	LessonType      string      `json:"lesson_type"`
	Content         *string     `json:"content,omitempty"`
	MediaID         pgtype.UUID `json:"media_id"`
	DurationMinutes *int32      `json:"duration_minutes,omitempty"`
	SortOrder       int32       `json:"sort_order"`
	IsPublished     bool        `json:"is_published"`
	IsPreview       *bool       `json:"is_preview,omitempty"`
}

// LearningReorderEntry is a single {id, sort_order} pair for bulk reorder.
type LearningReorderEntry struct {
	ID        pgtype.UUID `json:"id"`
	SortOrder int32       `json:"sort_order"`
}

// DocsService is the subset of internal/docs.Service the Phase 9 docs tools
// call. docs.DocPage and docs.CreateDocPageRequest are docs-package types
// that would create an import cycle if imported here, so mirror types are
// defined below.
// // Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Docs tools)
// Ref: japaconnect-engineering/sections/44-documentation-system.md.
type DocsService interface {
	ListAllDocPages(ctx context.Context) ([]DocsPage, error)
	GetDocPageByID(ctx context.Context, id pgtype.UUID) (*DocsPage, error)
	CreateDocPage(ctx context.Context, tenantID pgtype.UUID, req DocsCreatePageRequest) (pgtype.UUID, error)
	UpdateDocPage(ctx context.Context, tenantID, id pgtype.UUID, req DocsUpdatePageRequest) error
}

// DocsOrganizer is the function-type bridge for docs.organize. The docs
// service does not currently expose a parent_id/position update method, so
// the wiring layer provides this bridge. nil disables docs.organize.
type DocsOrganizer func(ctx context.Context, tenantID pgtype.UUID, entries []DocsOrganizeEntry) error

// DocsPage mirrors docs.DocPage (the subset the AI tools surface).
type DocsPage struct {
	ID           string          `json:"id"`
	Title        string          `json:"title"`
	Slug         string          `json:"slug"`
	Audience     string          `json:"audience"`
	Content      json.RawMessage `json:"content,omitempty"`
	AIGenerated  bool            `json:"ai_generated"`
	ReviewStatus string          `json:"review_status"`
	ParentID     *string         `json:"parent_id,omitempty"`
	Position     int32           `json:"position"`
	CreatedAt    string          `json:"created_at"`
	UpdatedAt    string          `json:"updated_at"`
}

// DocsCreatePageRequest mirrors docs.CreateDocPageRequest.
type DocsCreatePageRequest struct {
	Title        string          `json:"title"`
	Slug         string          `json:"slug"`
	Audience     string          `json:"audience"`
	Content      json.RawMessage `json:"content"`
	AIGenerated  bool            `json:"ai_generated"`
	ReviewStatus string          `json:"review_status"`
	ParentID     *string         `json:"parent_id,omitempty"`
	Position     int32           `json:"position"`
}

// DocsUpdatePageRequest mirrors docs.UpdateDocPageRequest.
type DocsUpdatePageRequest struct {
	Title        string          `json:"title"`
	Content      json.RawMessage `json:"content"`
	ReviewStatus string          `json:"review_status"`
}

// DocsOrganizeEntry is a single {id, parent_id, position} entry for
// docs.organize.
type DocsOrganizeEntry struct {
	ID       pgtype.UUID  `json:"id"`
	ParentID *pgtype.UUID `json:"parent_id,omitempty"`
	Position int32        `json:"position"`
}

// MenusService is the subset of internal/menus.Service the Phase 9 menu
// tools call. menus.CreateMenuItemInput and menus.UpdateMenuItemInput are
// menus-package types, so mirror types are defined below.
// // Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Menu tools)
// Ref: japaconnect-engineering/sections/054-menu-system.md (ADR-054).
type MenusService interface {
	ListMenus(ctx context.Context, q, placement, status, sort, sortDir string, limit, offset int32) (MenusListResponse, error)
	CreateMenuItem(ctx context.Context, input MenusCreateMenuItemInput) (gen.CreateMenuItemRow, error)
	UpdateMenuItem(ctx context.Context, id pgtype.UUID, input MenusUpdateMenuItemInput) (gen.UpdateMenuItemRow, error)
	DeleteMenuItem(ctx context.Context, id pgtype.UUID) error
}

// MenusListResponse mirrors menus.ListMenusResponse.
type MenusListResponse struct {
	Items      []gen.ListMenusRow `json:"items"`
	Total      int32              `json:"total"`
	TotalPages int32              `json:"total_pages"`
	Page       int32              `json:"page"`
}

// MenusCreateMenuItemInput mirrors menus.CreateMenuItemInput.
type MenusCreateMenuItemInput struct {
	MenuID           string          `json:"menu_id"`
	ParentID         *string         `json:"parent_id,omitempty"`
	Label            string          `json:"label"`
	URL              string          `json:"url"`
	Icon             *string         `json:"icon,omitempty"`
	SortOrder        int             `json:"sort_order"`
	PermissionNode   *string         `json:"permission_node,omitempty"`
	RequiredGroupIDs []string        `json:"required_group_ids,omitempty"`
	Visibility       string          `json:"visibility"`
	Target           string          `json:"target"`
	IsAdSlot         bool            `json:"is_ad_slot"`
	AdSlotConfig     json.RawMessage `json:"ad_slot_config,omitempty"`
	Status           string          `json:"status,omitempty"`
}

// MenusUpdateMenuItemInput mirrors menus.UpdateMenuItemInput.
type MenusUpdateMenuItemInput struct {
	Label            string          `json:"label"`
	URL              string          `json:"url"`
	Icon             *string         `json:"icon,omitempty"`
	ParentID         *string         `json:"parent_id,omitempty"`
	SortOrder        int             `json:"sort_order"`
	PermissionNode   *string         `json:"permission_node,omitempty"`
	RequiredGroupIDs []string        `json:"required_group_ids,omitempty"`
	Visibility       string          `json:"visibility"`
	Target           string          `json:"target"`
	IsAdSlot         bool            `json:"is_ad_slot"`
	AdSlotConfig     json.RawMessage `json:"ad_slot_config,omitempty"`
	Status           string          `json:"status"`
}

// UsersService is the subset of internal/users.Service the Phase 9 user
// tools call. users.UserProfileResponse and users.UpdateProfileRequest are
// users-package types, so mirror types are defined below.
// // Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (User tools).
type UsersService interface {
	GetProfile(ctx context.Context, userID pgtype.UUID) (*UserProfile, error)
	UpdateProfile(ctx context.Context, userID pgtype.UUID, req *UserProfileUpdateRequest) error
}

// UserProfile mirrors users.UserProfileResponse (the subset the AI tools
// surface — PII fields like email are omitted for non-self reads).
type UserProfile struct {
	ID            string  `json:"id"`
	Username      string  `json:"username"`
	DisplayName   *string `json:"display_name,omitempty"`
	AvatarURL     *string `json:"avatar_url,omitempty"`
	Bio           *string `json:"bio,omitempty"`
	Timezone      *string `json:"timezone,omitempty"`
	ProfileType   string  `json:"profile_type"`
	FollowerCount int32   `json:"follower_count"`
	CreatedAt     string  `json:"created_at"`
}

// UserProfileUpdateRequest mirrors users.UpdateProfileRequest.
type UserProfileUpdateRequest struct {
	DisplayName        *string `json:"display_name,omitempty"`
	AvatarURL          *string `json:"avatar_url,omitempty"`
	Bio                *string `json:"bio,omitempty"`
	Timezone           *string `json:"timezone,omitempty"`
	OriginCountry      *string `json:"origin_country,omitempty"`
	DestinationCountry *string `json:"destination_country,omitempty"`
}

// CustomFieldDefsService is the subset of internal/customfields.Service the
// Phase 9 custom field definition tools call. The existing
// CustomFieldsService (Phase 4) covers field value reads/writes; this
// interface covers field definition and group CRUD.
// // Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Custom field tools).
type CustomFieldDefsService interface {
	CreateField(ctx context.Context, params gen.CreateCustomFieldParams) (gen.CreateCustomFieldRow, error)
	UpdateField(ctx context.Context, params gen.UpdateCustomFieldParams) (gen.UpdateCustomFieldRow, error)
	ListFieldGroups(ctx context.Context) ([]gen.CustomFieldGroup, error)
	ListFieldGroupsByEntityClass(ctx context.Context, entityClass string) ([]gen.CustomFieldGroup, error)
	CreateFieldGroup(ctx context.Context, name, slug string, description *string, isSystem bool) (gen.CustomFieldGroup, error)
}

// GuideCategory describes an evergreen guide category (relocation, visa,
// banking, etc.) and its associated custom field group. This is a mirror
// type so the ai package does not import internal/customfields directly.
// // Ref: japaconnect-engineering/sections/14-content-type-system.md (Guide Content Type System).
type GuideCategory struct {
	Slug      string `json:"slug"`
	Label     string `json:"label"`
	IsSystem  bool   `json:"is_system"`
	GroupID   string `json:"group_id,omitempty"`
	GroupSlug string `json:"group_slug,omitempty"`
}

// GuideCategoryService lists admin-configured guide categories for the
// agent. It is implemented by an adapter in internal/server wrapping
// customfields.Service.ListGuideCategories.
// // Ref: .jeos/adr/ADR-111-ai-agent-executor.md.
type GuideCategoryService interface {
	ListGuideCategories(ctx context.Context) ([]GuideCategory, error)
}

// ContentType describes a JapaConnect content type and the custom field
// group that holds its structured fields. This is a mirror type so the ai
// package does not import internal/content directly.
// // Ref: japaconnect-engineering/sections/14-content-type-system.md.
type ContentType struct {
	ContentType           string `json:"content_type"`
	Label                 string `json:"label"`
	HasCategory           bool   `json:"has_category"`
	CategoryListTool      string `json:"category_list_tool,omitempty"`
	FieldGroupEntityClass string `json:"field_group_entity_class,omitempty"`
	FieldGroupID          string `json:"field_group_id,omitempty"`
	FieldGroupSlug        string `json:"field_group_slug,omitempty"`
}

// ContentTypesService lists valid content types for the agent. It is
// implemented by an adapter in internal/server that derives the list from
// the content package constants and the custom field group assignments.
// // Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Content Type Discovery).
type ContentTypesService interface {
	ListContentTypes(ctx context.Context) ([]ContentType, error)
}

// ContentLifecycleService is the subset of internal/content.Service the
// Phase 9 content lifecycle tools (delete/list/publish) call. The existing
// ContentService (Phase 4) covers search/read; this interface covers
// deletion, type-filtered listing, and status transitions.
// // Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Content tools).
type ContentLifecycleService interface {
	DeleteContent(ctx context.Context, id pgtype.UUID) error
	ListContentsByType(ctx context.Context, contentType, guideCategory string, cursor pgtype.Timestamptz, cursorID pgtype.UUID, offset, limit int32, sort string, vctx visitor.VisitorContext, filter visibility.VisibilityFilter) ([]gen.ListContentsByTypeRow, error)
	UpdateContentStatus(ctx context.Context, id pgtype.UUID, status string) (gen.UpdateContentStatusRow, error)
}

// ---------------------------------------------------------------------------
// ToolDeps — shared dependency container
// ---------------------------------------------------------------------------

// ToolDeps holds the service dependencies every Phase 4 tool handler closes
// over. A nil field degrades gracefully: the handler returns an error result
// explaining the dependency is not configured, rather than panicking.
// // ContentCreator and ContentUpdater are function-type bridges (not
// interfaces on the content.Service) because content.CreateContentInput and
// content.UpdateContentInput are content-package types — importing
// internal/content here would create a cycle risk. The wiring layer
// (internal/server) adapts the real service into these function types.
type ToolDeps struct {
	Content        ContentService
	ContentCreator ContentCreator
	ContentUpdater ContentUpdater
	CustomFields   CustomFieldsService
	RAG            RAGService
	Comparison     ComparisonService
	JCE            JCEService // nil → JCE advisor/refresh tools return "not configured"
	Privacy        PrivacyExemptionChecker
	Permission     PermissionChecker // nil skips permission checks in tool handlers
	Settings       SettingsReader
	Cache          CacheClient     // nil disables external-search caching
	HTTPClient     *http.Client    // nil → http.DefaultClient is used
	Persona        PersonaResolver // nil disables content.create_draft (Phase 8)
	HITL           HITLStore       // nil → executor's generic createHITL is used

	// Phase 9 extended catalog (Section 103).
	Spaces           SpacesService           // nil → space tools return "not configured"
	Learning         LearningStore           // nil → learn tools return "not configured"
	Docs             DocsService             // nil → docs tools return "not configured"
	DocsOrganize     DocsOrganizer           // nil → docs.organize returns "not configured"
	Menus            MenusService            // nil → menu tools return "not configured"
	Users            UsersService            // nil → user tools return "not configured"
	CustomFieldDefs  CustomFieldDefsService  // nil → field/group definition tools return "not configured"
	ContentLifecycle ContentLifecycleService // nil → content.delete/list/publish return "not configured"
	GuideCategories  GuideCategoryService    // nil → guide_categories.list returns "not configured"
	ContentTypes     ContentTypesService     // nil → content_types.list returns "not configured"

	// ToolsRegistry provides read access to the per-tool tables
	// (scholarships, visa_routes, housing_guides, etc.) for the
	// tools.list / tools.read AI Agent handlers. nil → tools handlers
	// return "not configured".
	ToolsRegistry ToolsRegistryService

	// Intelligence tools (§75 ADR-076 Amendment 2, on-demand source/record
	// update, content regeneration, and source discovery).
	TaskEnqueuer       TaskEnqueuer       // nil disables agent-triggered background tasks
	IntelQuerier       IntelQuerier       // nil disables source/record validation
	IntelSourceManager IntelSourceManager // nil disables source.discover

	// SearchWebFunc overrides SearchWeb for tests and custom search wiring.
	// If nil, SourceDiscoverHandler calls SearchWeb directly.
	SearchWebFunc func(ctx context.Context, client *http.Client, settings SettingsReader, tenantID pgtype.UUID, query string, limit int) (string, []WebSearchResult, error)

	// ScholarshipSearcher queries the scholarships projection table.
	ScholarshipSearcher ScholarshipSearcher // nil → scholarship.search returns "not configured"

	// Section 104 — AI Content Campaigns.
	PersonaProvisioner PersonaProvisioner // nil → personas.create returns "not configured"
	Campaign           CampaignService    // nil → content.campaign.create returns "not configured"
}

// ---------------------------------------------------------------------------
// Section 104 — AI Content Campaigns tool dependencies
// ---------------------------------------------------------------------------

// PersonaDefinition is a single persona the agent wants to create via the
// personas.create tool. The wiring-layer adapter maps this to the real
// personas.PersonaInput type.
type PersonaDefinition struct {
	Name           string         `json:"name"`
	Slug           string         `json:"slug"`
	Tagline        string         `json:"tagline,omitempty"`
	VoicePrompt    string         `json:"voice_prompt"`
	DisclosureMode string         `json:"disclosure_mode,omitempty"`
	AutonomyMode   string         `json:"autonomy_mode,omitempty"`
	ActivityPolicy map[string]any `json:"activity_policy,omitempty"`
	RoleFocus      string         `json:"role_focus,omitempty"`
}

// ProvisionedPersona is the result of creating one persona.
type ProvisionedPersona struct {
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Slug  string `json:"slug,omitempty"`
	Error string `json:"error,omitempty"`
}

// PersonaProvisioner is the subset of the personas service used by the
// personas.create agent tool.
type PersonaProvisioner interface {
	ProvisionPersonas(ctx context.Context, tenantID, adminUserID pgtype.UUID, personas []PersonaDefinition) ([]ProvisionedPersona, error)
}

// CampaignCreateInput is the payload for the content.campaign.create agent
// tool. The wiring-layer adapter maps this to campaign.CreateCampaignInput.
type CampaignCreateInput struct {
	Prompt                string   `json:"prompt"`
	ContentType           string   `json:"content_type,omitempty"`
	Count                 int      `json:"count"`
	PersonaIDs            []string `json:"persona_ids,omitempty"`
	AutoProvisionPersonas *bool    `json:"auto_provision_personas,omitempty"`
	AutoAssign            *bool    `json:"auto_assign,omitempty"`
	TargetSpaceID         string   `json:"target_space_id,omitempty"`
	TargetTags            []string `json:"target_tags,omitempty"`
	RunImmediately        *bool    `json:"run_immediately,omitempty"`
}

// CampaignCreateResult is the structured result returned by the
// content.campaign.create agent tool.
type CampaignCreateResult struct {
	CampaignID          string                `json:"campaign_id"`
	RequestedCount      int                   `json:"requested_count"`
	ProvisionedPersonas int                   `json:"provisioned_personas"`
	Status              string                `json:"status"`
	Items               []CampaignItemSummary `json:"items,omitempty"`
}

// CampaignItemSummary is a lightweight view of a planned campaign item.
type CampaignItemSummary struct {
	PersonaID   string `json:"persona_id"`
	ContentType string `json:"content_type"`
	Title       string `json:"title,omitempty"`
}

// CampaignService is the subset of the campaign service used by the
// content.campaign.create agent tool.
type CampaignService interface {
	CreateCampaign(ctx context.Context, tenantID, userID pgtype.UUID, in CampaignCreateInput) (CampaignCreateResult, error)
}

// ToolsRegistryService is the subset of the per-tool services the
// tools.list / tools.read AI Agent handlers call. The wiring layer
// (internal/server) adapts the real per-tool services into this
// interface so the ai package does not import internal/scholarships,
// internal/visaroutes, etc.
// // Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Tools app tools)
// Ref: japaconnect-engineering/sections/78-tools-web-surface.md.
type ToolsRegistryService interface {
	// ListTools returns the catalog of available Japa tools with counts.
	ListTools(ctx context.Context) ([]ToolCatalogEntry, error)
	// ReadTool reads a single record from a tool table by type + ID.
	ReadTool(ctx context.Context, toolType, id string) (map[string]any, error)
	// ToolTypes returns the tool_type values in the catalog.
	ToolTypes() []string
	// CreateTool creates a new record in the named tool table.
	CreateTool(ctx context.Context, tenantID, userID pgtype.UUID, toolType string, payload map[string]any) (map[string]any, error)
	// UpdateTool updates an existing record in the named tool table.
	UpdateTool(ctx context.Context, tenantID, userID pgtype.UUID, toolType, id string, payload map[string]any) (map[string]any, error)
	// DeleteTool soft-deletes an existing record from the named tool table.
	DeleteTool(ctx context.Context, tenantID, userID pgtype.UUID, toolType, id string) error
	// AssignGeo updates the geo-scoping fields of an existing record in the named tool table.
	AssignGeo(ctx context.Context, tenantID, userID pgtype.UUID, toolType, id string, payload map[string]any) (map[string]any, error)
}

// ToolCatalogEntry describes a single Japa tool in the registry catalog.
type ToolCatalogEntry struct {
	ToolType string `json:"tool_type"`
	Label    string `json:"label"`
	Count    int64  `json:"count"`
	Category string `json:"category"`
}

// TaskEnqueuer enqueues Asynq tasks from AI agent handlers. The worker
// package's Enqueuer satisfies this interface.
// // Ref: japaconnect-engineering/sections/11-job-queue-asynq.md.
type TaskEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...any) error
}

// IntelQuerier loads tenant-scoped intelligence sources and records so
// agent handlers can validate IDs before enqueueing background work.
// // Ref: japaconnect-engineering/adr/ADR-112-intelligence-scholarships-table.md.
type IntelQuerier interface {
	GetIntelSource(ctx context.Context, id pgtype.UUID) (gen.IntelSource, error)
	GetIntelRecord(ctx context.Context, id pgtype.UUID) (gen.IntelRecord, error)
	GetIntelSourceByURL(ctx context.Context, sourceUrl string) (gen.IntelSource, error)
}

// IntelSourceManager creates and looks up sources for the source.discover
// workflow. It is satisfied by sources.Service or by a direct gen.Queries
// adapter.
// // Ref: japaconnect-engineering/adr/ADR-112-intelligence-scholarships-table.md.
type IntelSourceManager interface {
	Create(ctx context.Context, tenantID pgtype.UUID, arg gen.CreateIntelSourceParams) (gen.IntelSource, error)
	GetByURL(ctx context.Context, sourceURL string) (gen.IntelSource, error)
}

// ScholarshipSearchFilters is the argument shape for a scholarship search.
// It mirrors the filters used by the Scholarship Finder surface (Section 73)
// so the AI agent tool and the public UI share semantics.
// // Ref: japaconnect-engineering/sections/73-scholarship-finder.md.
type ScholarshipSearchFilters struct {
	Country           string
	EligibleCountries string
	DeadlineFrom      string
	DeadlineTo        string
	FundingType       string
	Level             string
	Q                 string
	Limit             int
	Page              int
}

// ScholarshipSearchResult is a single scholarship record returned to the AI
// agent. It is intentionally a flat, stable shape independent of the internal
// scholarships package so the ai package does not import it.
// // Ref: docs/superpowers/specs/2026-08-19-on-demand-intel-to-content.md.
type ScholarshipSearchResult struct {
	ID                  string   `json:"id"`
	GuideID             string   `json:"guide_id"`
	IntelRecordID       string   `json:"intel_record_id"`
	Title               string   `json:"title"`
	Slug                string   `json:"slug"`
	Summary             string   `json:"summary"`
	Description         string   `json:"description"`
	Institution         string   `json:"institution"`
	Country             string   `json:"country"`
	LevelOfStudy        string   `json:"level_of_study"`
	AwardAmountUSD      *float64 `json:"award_amount_usd"`
	IsRenewable         *bool    `json:"is_renewable"`
	ApplicationDeadline string   `json:"application_deadline"`
	ApplicationURL      string   `json:"application_url"`
	EligibleCountries   []string `json:"eligible_countries"`
}

// ScholarshipSearcher is the subset of the scholarships finder service the
// scholarship.search AI Agent handler calls.
// // Ref: japaconnect-engineering/sections/73-scholarship-finder.md.
type ScholarshipSearcher interface {
	FindScholarships(ctx context.Context, filters ScholarshipSearchFilters) ([]ScholarshipSearchResult, int64, error)
}

// NewToolDeps creates an empty ToolDeps. Fields are set directly or via the
// With* setters below.
func NewToolDeps() *ToolDeps {
	return &ToolDeps{}
}

// WithContent injects the content service.
func (d *ToolDeps) WithContent(s ContentService) *ToolDeps { d.Content = s; return d }

// WithCustomFields injects the custom fields service.
func (d *ToolDeps) WithCustomFields(s CustomFieldsService) *ToolDeps { d.CustomFields = s; return d }

// WithRAG injects the RAG service.
func (d *ToolDeps) WithRAG(s RAGService) *ToolDeps { d.RAG = s; return d }

// WithComparison injects the comparison service (Phase 7).
func (d *ToolDeps) WithComparison(s ComparisonService) *ToolDeps { d.Comparison = s; return d }

// WithJCE injects the JCE advisor/refresh service (Sub-spec 4).
func (d *ToolDeps) WithJCE(s JCEService) *ToolDeps { d.JCE = s; return d }

// WithPrivacy injects the privacy exemption checker.
func (d *ToolDeps) WithPrivacy(p PrivacyExemptionChecker) *ToolDeps { d.Privacy = p; return d }

// WithPermission injects the RBAC permission checker. Tool handlers use this
// for fine-grained checks that go beyond the tool's top-level
// required_permission (e.g. content.assign_author for author overrides,
// agent.content_type.* for type-specific creation).
func (d *ToolDeps) WithPermission(p PermissionChecker) *ToolDeps { d.Permission = p; return d }

// WithSettings injects the hot-reloadable settings reader.
func (d *ToolDeps) WithSettings(s SettingsReader) *ToolDeps { d.Settings = s; return d }

// WithCache injects the Valkey cache client for external-search caching.
func (d *ToolDeps) WithCache(c CacheClient) *ToolDeps { d.Cache = c; return d }

// WithHTTPClient injects the HTTP client used for external search API calls.
func (d *ToolDeps) WithHTTPClient(c *http.Client) *ToolDeps { d.HTTPClient = c; return d }

// WithPersona injects the AI persona resolver (Phase 8 content.create_draft).
func (d *ToolDeps) WithPersona(p PersonaResolver) *ToolDeps { d.Persona = p; return d }

// WithHITL injects the HITL store (Phase 8 content.create_draft creates a
// review with source='agent_draft_request' + the created content_id).
func (d *ToolDeps) WithHITL(s HITLStore) *ToolDeps { d.HITL = s; return d }

// WithSpaces injects the spaces service (Phase 9 space tools).
func (d *ToolDeps) WithSpaces(s SpacesService) *ToolDeps { d.Spaces = s; return d }

// WithLearning injects the learning store (Phase 9 learn tools).
func (d *ToolDeps) WithLearning(s LearningStore) *ToolDeps { d.Learning = s; return d }

// WithDocs injects the docs service (Phase 9 docs tools).
func (d *ToolDeps) WithDocs(s DocsService) *ToolDeps { d.Docs = s; return d }

// WithDocsOrganizer injects the docs.organize bridge (Phase 9).
func (d *ToolDeps) WithDocsOrganizer(f DocsOrganizer) *ToolDeps { d.DocsOrganize = f; return d }

// WithMenus injects the menus service (Phase 9 menu tools).
func (d *ToolDeps) WithMenus(s MenusService) *ToolDeps { d.Menus = s; return d }

// WithUsers injects the users service (Phase 9 user tools).
func (d *ToolDeps) WithUsers(s UsersService) *ToolDeps { d.Users = s; return d }

// WithCustomFieldDefs injects the custom field definition service (Phase 9).
func (d *ToolDeps) WithCustomFieldDefs(s CustomFieldDefsService) *ToolDeps {
	d.CustomFieldDefs = s
	return d
}

// WithGuideCategories injects the guide category lister so the
// guide_categories.list tool can return admin-configured guide categories.
func (d *ToolDeps) WithGuideCategories(s GuideCategoryService) *ToolDeps {
	d.GuideCategories = s
	return d
}

// WithContentTypes injects the content type lister so the content_types.list
// tool can return valid content types and their custom field group hints.
func (d *ToolDeps) WithContentTypes(s ContentTypesService) *ToolDeps {
	d.ContentTypes = s
	return d
}

// WithContentLifecycle injects the content lifecycle service (Phase 9).
func (d *ToolDeps) WithContentLifecycle(s ContentLifecycleService) *ToolDeps {
	d.ContentLifecycle = s
	return d
}

// WithToolsRegistry injects the tools registry service (Phase 9 tools.list/read).
func (d *ToolDeps) WithToolsRegistry(s ToolsRegistryService) *ToolDeps {
	d.ToolsRegistry = s
	return d
}

// WithTaskEnqueuer injects the Asynq task enqueuer used by agent handlers
// that trigger background workers (source.crawl_now, record.update_now,
// content.regenerate_from_source).
func (d *ToolDeps) WithTaskEnqueuer(e TaskEnqueuer) *ToolDeps {
	d.TaskEnqueuer = e
	return d
}

// WithIntelQuerier injects the intelligence query provider used to validate
// source/record IDs before enqueueing background work.
func (d *ToolDeps) WithIntelQuerier(q IntelQuerier) *ToolDeps {
	d.IntelQuerier = q
	return d
}

// WithIntelSourceManager injects the intelligence source manager used to
// create proposed sources from the source.discover agent tool.
func (d *ToolDeps) WithIntelSourceManager(m IntelSourceManager) *ToolDeps {
	d.IntelSourceManager = m
	return d
}

// WithScholarshipSearcher injects the scholarship finder service for the
// scholarship.search agent tool.
func (d *ToolDeps) WithScholarshipSearcher(s ScholarshipSearcher) *ToolDeps {
	d.ScholarshipSearcher = s
	return d
}

// WithPersonaProvisioner injects the persona provisioning service for the
// personas.create agent tool.
func (d *ToolDeps) WithPersonaProvisioner(p PersonaProvisioner) *ToolDeps {
	d.PersonaProvisioner = p
	return d
}

// WithCampaign injects the content campaign service for the
// content.campaign.create agent tool.
func (d *ToolDeps) WithCampaign(c CampaignService) *ToolDeps {
	d.Campaign = c
	return d
}

// httpClient returns the configured HTTP client or http.DefaultClient.
func (d *ToolDeps) httpClient() *http.Client {
	if d.HTTPClient != nil {
		return d.HTTPClient
	}

	return http.DefaultClient
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// scanUUID parses a UUID string from tool arguments into pgtype.UUID.
// Returns the zero UUID and the error on failure.
func scanUUID(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		return pgtype.UUID{}, err
	}

	return u, nil
}

// optionalUUID parses an optional UUID string. An empty string yields a
// zero (invalid) pgtype.UUID, signalling "not provided".
func optionalUUID(s string) (pgtype.UUID, error) {
	if s == "" {
		return pgtype.UUID{}, nil
	}

	return scanUUID(s)
}

// uuidPtr returns a *pgtype.UUID for a valid UUID, or nil when the pgtype
// value is the zero/invalid value. Used to build the *pgtype.UUID that
// rag.Service.Retrieve expects.
func uuidPtr(u pgtype.UUID) *pgtype.UUID {
	if !u.Valid {
		return nil
	}

	return &u
}
