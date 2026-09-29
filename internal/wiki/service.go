// Package wiki — service.go orchestrates wiki page browsing, revision
// submission, HITL review, and admin management. Feature-flagged via
// wiki.enabled system_setting (§80).
// // Ref: japaconnect-engineering/sections/80-destination-wiki.md
// //	japaconnect-engineering/adr/ADR-088-destination-wiki.md
package wiki

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/ai"
	"github.com/jedafe/japa/internal/config"
	contentpkg "github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/valkey"
)

// WikiPageDTO is the API response shape for a wiki page (index listing).
// //nolint:revive // exported: stutter naming is intentional
type WikiPageDTO struct {
	ID             string  `json:"id"`
	SpaceID        string  `json:"space_id"`
	Slug           string  `json:"slug"`
	Title          string  `json:"title"`
	Category       string  `json:"category"`
	IsFeatured     bool    `json:"is_featured"`
	IsLocked       bool    `json:"is_locked"`
	IsActive       bool    `json:"is_active"`
	CreatedAt      string  `json:"created_at,omitempty"`
	UpdatedAt      string  `json:"updated_at"`
	SpaceSlug      string  `json:"space_slug,omitempty"`
	SpaceName      string  `json:"space_name,omitempty"`
	DataEntityType *string `json:"data_entity_type,omitempty"`
	// §80 Hierarchical UX: page_template_id echoes the template the page was
	// bound to at create (omitted for freeform pages); position orders
	// siblings inside the category for Prev/Next navigation.
	PageTemplateID string `json:"page_template_id,omitempty"`
	Position       int32  `json:"position"`
}

// AdminWikiPageDTO extends WikiPageDTO with the latest revision metadata
// needed by the admin review queue (B1). latest_revision_id gates the
// Approve/Reject actions in the frontend; status + excerpt populate the
// table columns.
type AdminWikiPageDTO struct {
	WikiPageDTO
	LatestRevisionID     string `json:"latest_revision_id,omitempty"`
	LatestRevisionStatus string `json:"status,omitempty"`
	Excerpt              string `json:"excerpt,omitempty"`
	// DeletedAt marks soft-deleted pages (§80) — the admin queue surfaces
	// them via ?deleted=only|all for force delete.
	DeletedAt string `json:"deleted_at,omitempty"`
}

// WikiPageDetailDTO is the full wiki page with current approved revision.
// Fields carries the page category's bound field group — schema + this
// space's stored values — when the wiki type binds a group to the category
// (§80 Wiki Types & Field-Group Pages).
// //nolint:revive // exported: stutter naming is intentional
type WikiPageDetailDTO struct {
	WikiPageDTO
	SpaceSlug       string          `json:"space_slug"`
	SpaceName       string          `json:"space_name"`
	CurrentContent  json.RawMessage `json:"current_content"`
	RevisionSummary string          `json:"revision_summary,omitempty"`
	SubmittedBy     string          `json:"submitted_by,omitempty"`
	// Fields is the bound field group's schema+values block (§80); every
	// category binds a group, so nil only when the group lookup itself
	// fails. CategoryLabel echoes the template's admin-set label so the
	// page heading isn't hardcoded. FieldGroupID carries the binding even
	// when the group has no fields — the frontend scopes the entity-fields
	// panel by it, and without it an empty bound group would leave the
	// panel unscoped (showing every class-assigned group on a prose page).
	Fields       *EntityFieldGroupDTO `json:"fields,omitempty"`
	FieldGroupID string               `json:"field_group_id,omitempty"`
	// FieldGroupSlug mirrors FieldGroupID for ops submissions — the
	// propose_field op targets groups by slug, and the bound group's slug
	// must reach the client even when the group has no fields (then
	// Fields is nil).
	FieldGroupSlug string `json:"field_group_slug,omitempty"`
	CategoryLabel  string `json:"category_label,omitempty"`
	// §80 Hierarchical UX additions: Ancestors is the resolved breadcrumb
	// chain (root-first) walked over space_parent_spaces ∪ parent_id and
	// arbitrated by the declared type edges; Siblings is the category's
	// ordered page set for Prev/Next; Rollup is the read-time descendant
	// aggregate when the category opts in (nil otherwise).
	Ancestors []WikiSpaceRefDTO `json:"ancestors,omitempty"`
	// AncestorsAlt holds the multi-parent links not picked into the
	// breadcrumb chain — undeclared junction parents and declared
	// runners-up — rendered as "also under X" (§80 amendment §6).
	AncestorsAlt []WikiSpaceRefDTO `json:"ancestors_alt,omitempty"`
	Siblings     []WikiSiblingDTO  `json:"siblings,omitempty"`
	Rollup       *WikiRollupDTO    `json:"rollup,omitempty"`
	// PageTemplates lists the page's category's declared sibling template
	// pages (§80 lazy pages) — labels carry the space's name substituted
	// for the {name} token. The frontend links them for discovery; lazy
	// ones resolve without a wiki_pages row.
	PageTemplates []WikiPageTemplateItemDTO `json:"page_templates,omitempty"`
	// Lazy marks a page rendered straight from its page template — no
	// wiki_pages row exists yet (§80: template pages are lazy like category
	// pages, materializing on the first prose/ops submission). The frontend
	// uses it to show the "not yet created" affordance instead of revision
	// history.
	Lazy bool `json:"lazy,omitempty"`
}

// WikiPageTemplateItemDTO is the public shape for one page template link —
// slug (the /wiki/{space}/{slug} target), label ({name} substituted), and
// template ordering position.
// //nolint:revive // exported: stutter naming is intentional
type WikiPageTemplateItemDTO struct {
	Slug     string `json:"slug"`
	Label    string `json:"label"`
	Position int32  `json:"position"`
}

// pageTemplateItems maps a category's stored templates to the public
// link DTOs, substituting the {name} token with the space's name.
func pageTemplateItems(templates []WikiPageTemplate, spaceName string) []WikiPageTemplateItemDTO {
	if len(templates) == 0 {
		return nil
	}

	out := make([]WikiPageTemplateItemDTO, 0, len(templates))
	for _, tm := range templates {
		out = append(out, WikiPageTemplateItemDTO{
			Slug:     tm.Slug,
			Label:    substituteTemplateLabel(tm.Label, spaceName),
			Position: tm.Position,
		})
	}

	return out
}

// WikiDestinationDTO is the API response shape for a destination with wikis.
// //nolint:revive // exported: stutter naming is intentional
type WikiDestinationDTO struct {
	SpaceID   string `json:"space_id"`
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	GeoCode   string `json:"geo_code,omitempty"`
	Region    string `json:"region,omitempty"`
	Level     string `json:"level"`
	PageCount int64  `json:"page_count"`
}

// WikiRevisionDTO is the API response shape for a revision. The §80 community
// field ops columns (kind/ops/target_*) are included so the admin review
// queue can render a field-level diff and the entity revision history can
// show structured edits.
// //nolint:revive // exported: stutter naming is intentional
type WikiRevisionDTO struct {
	ID          string          `json:"id"`
	Status      string          `json:"status"`
	Kind        string          `json:"kind"`
	Content     json.RawMessage `json:"content,omitempty"`
	Ops         json.RawMessage `json:"ops,omitempty"`
	Summary     string          `json:"summary,omitempty"`
	AiGenerated bool            `json:"ai_generated"`

	TargetEntityType string `json:"target_entity_type,omitempty"`
	TargetEntityID   string `json:"target_entity_id,omitempty"`

	SubmittedBy string `json:"submitted_by,omitempty"`
	ReviewedBy  string `json:"reviewed_by,omitempty"`
	ReviewedAt  string `json:"reviewed_at,omitempty"`
	// ApplyError is the last ops-application failure recorded on a pending
	// fields/mixed revision (§80) — surfaced in the admin queue so reviewers
	// can see why an approve attempt rolled back.
	ApplyError string `json:"apply_error,omitempty"`
	CreatedAt  string `json:"created_at"`
}

// SubmitRevisionInput is the request body for POST /wiki/:space_slug/:page_slug/revisions.
// Ops carries §80 community field ops (structured edits); when present the
// handler stores a single fields/mixed revision via SubmitFieldOps — prose
// and ops approve atomically rather than splitting into two queue entries.
type SubmitRevisionInput struct {
	Content json.RawMessage `json:"content"`
	Summary string          `json:"summary"`
	Ops     []FieldOp       `json:"ops"`
}

// UpdatePageInput is the request body for PATCH /admin/wiki/:page_id.
// DataEntityType binds the page to a child entity class (e.g. 'visa_route')
// so field ops target that entity instead of the host space (§80).
type UpdatePageInput struct {
	IsLocked       *bool   `json:"is_locked"`
	IsFeatured     *bool   `json:"is_featured"`
	IsActive       *bool   `json:"is_active"`
	DataEntityType *string `json:"data_entity_type"`
	// Position orders the page among its category siblings (§80 Prev/Next).
	Position *int32 `json:"position"`
}

// CreateWikiPageInput is the request body for POST /wiki/:space_slug.
type CreateWikiPageInput struct {
	SpaceSlug string          `json:"-"`
	Slug      string          `json:"slug"`
	Title     string          `json:"title"`
	Category  string          `json:"category"`
	Content   json.RawMessage `json:"content"`
	Summary   string          `json:"summary"`
	Position  *int32          `json:"position"`
}

// GeneratePageInput is the request body for POST /admin/wiki/generate.
type GeneratePageInput struct {
	SpaceSlug string `json:"space_slug"`
	PageSlug  string `json:"page_slug"`
	Title     string `json:"title"`
	Category  string `json:"category"`
}

// GenerateDraftRequest is the input for AI-generated wiki page drafts
// (ADR-088, routed from the Content Studio).
type GenerateDraftRequest struct {
	SpaceID  string `json:"space_id"`
	PageSlug string `json:"page_slug"`
	Title    string `json:"title"`
	Category string `json:"category"`
	Prompt   string `json:"prompt"`
}

// GenerateDraftResponse is the response from Content Agent draft generation.
type GenerateDraftResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	PageID  string `json:"page_id"`
}

// FeatureChecker checks whether a feature flag is enabled for a platform.
// Satisfied by *features.Checker via structural typing — defined locally to
// avoid importing the features package (keeps the dependency graph clean).
type FeatureChecker interface {
	IsEnabled(ctx context.Context, key string) bool
}

// AIDispatcher is the subset of the AI Gateway the wiki service needs to
// generate draft pages (content_generation category).
type AIDispatcher interface {
	Dispatch(ctx context.Context, category string, req ai.Request) (ai.Response, error)
}

// Service holds the business logic for the Destination Wiki.
type Service struct {
	store      *Store
	settings   *config.Settings
	vk         valkeycompat.Cmdable
	checker    FeatureChecker
	dispatcher AIDispatcher

	// §80 community field ops collaborators (see service_fields.go):
	fields           fieldReadStore        // submit-time custom-field reads
	fieldProvisioner fieldGroupProvisioner // category group auto-provisioning
	entityResolver   entityResolver        // JCE item_ref resolver registry
	jceBuster        jceCacheBuster        // JCE comparison cache invalidation
	enqueuer         TaskEnqueuer          // Asynq enqueue for wiki:fields_propagate
	perms            permissionChecker     // per-op RBAC node checks
}

// NewService creates a wiki service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{store: store, settings: settings}
}

// SetFeatureChecker injects the feature flag checker so Enabled can read the
// feature_flags table (the single source of truth for feature reachability)
// instead of system_settings. Falls back to system_settings when nil.
func (s *Service) SetFeatureChecker(c FeatureChecker) { s.checker = c }

// SetValkey injects the Valkey client used for the jc:wiki_edit_lock lock
// (B6: soft edit lock preventing concurrent wiki edits, §80 / valkey-keys-catalog).
func (s *Service) SetValkey(vk valkeycompat.Cmdable) { s.vk = vk }

// SetDispatcher injects the AI Gateway dispatcher for draft generation.
func (s *Service) SetDispatcher(d AIDispatcher) { s.dispatcher = d }

// SetCustomFields injects the custom-fields read store used for submit-time
// ops validation (field existence, can_edit) and the entity fields schema
// read (§80 Community Field Ops). Implemented by *customfields.Service.
// When the collaborator also satisfies fieldGroupProvisioner (the real
// service does) it is retained for category group auto-provisioning.
func (s *Service) SetCustomFields(cfs fieldReadStore) {
	s.fields = cfs
	s.fieldProvisioner, _ = cfs.(fieldGroupProvisioner)
}

// SetEntityResolver injects the JCE item_ref resolver registry used by
// SuggestFields to verify the target entity exists and to find its host wiki
// page via the resolved item's geo_space_id metadata. Satisfied by
// *resolver.Registry (wired in server.go).
func (s *Service) SetEntityResolver(r entityResolver) { s.entityResolver = r }

// SetJCECacheBuster injects the comparison cache buster invoked after an ops
// revision is approved — either directly (when no Asynq enqueuer is wired) or
// from the wiki:fields_propagate task via PropagateFields. Implemented by
// *comparison.Service.
func (s *Service) SetJCECacheBuster(b jceCacheBuster) { s.jceBuster = b }

// SetEnqueuer injects the Asynq task enqueuer used to fire-and-forget
// wiki:fields_propagate after an ops revision is approved (Rule 8).
func (s *Service) SetEnqueuer(e TaskEnqueuer) { s.enqueuer = e }

// SetPermissionChecker injects the RBAC resolver used for the per-op grant
// map in SubmitFieldOps (wiki.fields.edit/propose/manage, §10). Shares the
// permissionChecker shape the wiki Handlers already use.
func (s *Service) SetPermissionChecker(pc permissionChecker) { s.perms = pc }

// Enabled checks whether the wiki feature is enabled for the given platform.
// Delegates to the features.Checker (feature_flags table) when available,
// falling back to system_settings for backward compatibility.
func (s *Service) Enabled(ctx context.Context) bool {
	if s.checker != nil {
		return s.checker.IsEnabled(ctx, "wiki.enabled")
	}

	if s.settings == nil {
		return false
	}

	return s.settings.GetBoolOr(ctx, "wiki.enabled", false)
}

// CommunityEditsAllowed checks if community edits are allowed (vs mod-only).
// BUG-003: fail closed when settings is nil — community edits disabled by
// default to avoid a fail-open bypass if settings injection fails.
func (s *Service) CommunityEditsAllowed(ctx context.Context) bool {
	if s.settings == nil {
		slog.Warn("wiki: settings service is nil, community_edits defaulting to false (fail-closed)")
		return false
	}

	return s.settings.GetBoolOr(ctx, "wiki.community_edits", true)
}

// MinReputationToEdit returns the reputation threshold for wiki edits.
func (s *Service) MinReputationToEdit(ctx context.Context) int {
	if s.settings == nil {
		return 100
	}

	return s.settings.GetIntOr(ctx, "wiki.min_reputation_to_edit", 100)
}

// GetWikiDestinations returns active destination spaces (country/state/city)
// with their active wiki page count. A non-empty level filters to that level; a
// non-empty parentSlug filters to direct children of that space; a non-empty
// region filters country-level spaces by spaces.region; hasWiki, when true,
// only returns spaces with at least one active wiki page.
func (s *Service) GetWikiDestinations(ctx context.Context, level, parentSlug, region, search, within string, hasWiki bool) ([]WikiDestinationDTO, error) {
	dests, err := s.store.ListWikiDestinations(ctx, level, parentSlug, region, search, within, hasWiki)
	if err != nil {
		return nil, fmt.Errorf("wiki: get destinations: %w", err)
	}

	dtos := make([]WikiDestinationDTO, 0, len(dests))

	for _, d := range dests {
		dto := WikiDestinationDTO{
			SpaceID:   d.SpaceID.String(),
			Slug:      d.Slug,
			Name:      d.Name,
			Level:     d.Level,
			PageCount: d.PageCount,
		}
		if d.GeoCode != nil {
			dto.GeoCode = *d.GeoCode
		}

		if d.Region != nil {
			dto.Region = *d.Region
		}

		dtos = append(dtos, dto)
	}

	return dtos, nil
}

// IsUniversitySpace reports whether the slug resolves to a university space
// (spaces.level = 'university' or space_types.slug = 'university'). Unknown
// slugs return false.
func (s *Service) IsUniversitySpace(ctx context.Context, spaceSlug string) (bool, error) {
	return s.store.IsUniversitySpaceBySlug(ctx, spaceSlug)
}

// GetWikiIndex returns all active wiki pages for a destination space (by space slug).
// WikiIndexDTO is the space-level index payload — the page list plus the
// space's ancestor chain (§80 Hierarchical UX: the index page renders the
// same breadcrumb as its pages).
type WikiIndexDTO struct {
	Pages        []WikiPageDTO     `json:"pages"`
	Ancestors    []WikiSpaceRefDTO `json:"ancestors,omitempty"`
	AncestorsAlt []WikiSpaceRefDTO `json:"ancestors_alt,omitempty"`
}

func (s *Service) GetWikiIndex(ctx context.Context, spaceSlug string) (*WikiIndexDTO, error) {
	spaceID, _, err := s.store.GetSpaceBySlug(ctx, spaceSlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSpaceNotFound
		}

		return nil, fmt.Errorf("wiki: index: resolve space: %w", err)
	}

	pages, err := s.store.ListWikiPagesBySpace(ctx, spaceSlug)
	if err != nil {
		return nil, fmt.Errorf("wiki: index: %w", err)
	}

	// §80 Wiki Types: a space with an empty index and a seed_stubs wiki type
	// hasn't been seeded yet (e.g. created after wiki.enabled activation) —
	// enqueue the idempotent seed task rather than leaving the index empty.
	if len(pages) == 0 {
		if seed, serr := s.store.SpaceSeedStubs(ctx, spaceID); serr == nil && seed {
			s.enqueueStubSeed(ctx, spaceID)
		}
	}

	dtos := make([]WikiPageDTO, 0, len(pages))
	for _, p := range pages {
		dtos = append(dtos, pageToDTO(p))
	}

	out := &WikiIndexDTO{Pages: dtos}
	if spaceTypeID, _, terr := s.store.GetSpaceTypeRef(ctx, spaceID); terr == nil {
		out.Ancestors, out.AncestorsAlt = s.getAncestors(ctx, spaceID, spaceTypeID)
	}

	return out, nil
}

// enqueueStubSeed fire-and-forget enqueues wiki:seed_stub_pages for one
// space (Rule 8). The task is idempotent, and asynq.Unique dedupes by
// type+payload for an hour so repeated anonymous views of an unseeded
// country index can't flood the queue.
func (s *Service) enqueueStubSeed(ctx context.Context, spaceID pgtype.UUID) {
	if s.enqueuer == nil || !spaceID.Valid {
		return
	}
	payload, err := json.Marshal(map[string]string{"space_id": spaceID.String()})
	if err != nil {
		return
	}
	if err := s.enqueuer.Enqueue(TaskSeedStubPages, payload, asynq.Unique(time.Hour)); err != nil &&
		!errors.Is(err, asynq.ErrDuplicateTask) {
		slog.Warn("wiki: lazy seed_stub_pages enqueue failed", "space_id", spaceID.String(), "error", err)
	}
}

// GetWikiIndexByDestSpaceID returns all active wiki pages for a destination space by its ID.
// Used by the ?dest= geo-aware query parameter.
func (s *Service) GetWikiIndexByDestSpaceID(ctx context.Context, spaceID pgtype.UUID) (*WikiIndexDTO, error) {
	if _, _, err := s.store.GetSpaceByID(ctx, spaceID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSpaceNotFound
		}

		return nil, fmt.Errorf("wiki: index by dest: resolve space: %w", err)
	}

	pages, err := s.store.ListWikiPagesBySpaceID(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("wiki: index by dest: %w", err)
	}

	dtos := make([]WikiPageDTO, 0, len(pages))
	for _, p := range pages {
		dtos = append(dtos, pageToDTO(p))
	}

	out := &WikiIndexDTO{Pages: dtos}
	if spaceTypeID, _, terr := s.store.GetSpaceTypeRef(ctx, spaceID); terr == nil {
		out.Ancestors, out.AncestorsAlt = s.getAncestors(ctx, spaceID, spaceTypeID)
	}

	return out, nil
}

// GetWikiPage returns a single wiki page with its current approved
// revision. When the space's wiki type binds a field group to the page's
// category (§80 Wiki Types & Field-Group Pages), the response carries the
// group's schema + this space's stored values in fields — the shared
// structured block every entity of the type renders at the same slug.
// userID drives field-level can_view/can_edit (invalid UUID = anonymous,
// fail-closed per §06).
func (s *Service) GetWikiPage(ctx context.Context, userID pgtype.UUID, spaceSlug, pageSlug string) (WikiPageDetailDTO, error) {
	spaceID, spaceName, err := s.store.GetSpaceBySlug(ctx, spaceSlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return WikiPageDetailDTO{}, ErrSpaceNotFound
		}

		return WikiPageDetailDTO{}, fmt.Errorf("wiki: get page: resolve space: %w", err)
	}

	page, err := s.store.GetWikiPageBySlug(ctx, spaceSlug, pageSlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// §80 lazy template pages: a slug matching the space type's
			// (category-scoped) page templates renders without a
			// wiki_pages row — the page materializes on the first
			// prose/ops submission.
			return s.getLazyTemplatePage(ctx, userID, spaceID, spaceSlug, spaceName, pageSlug)
		}

		return WikiPageDetailDTO{}, fmt.Errorf("wiki: get page: %w", err)
	}

	dto := WikiPageDetailDTO{
		WikiPageDTO: pageToDTO(page),
		SpaceSlug:   page.SpaceSlug,
		SpaceName:   page.SpaceName,
	}

	// Fetch the current approved revision.
	rev, err := s.store.GetApprovedRevision(ctx, page.ID)
	if err == nil {
		dto.CurrentContent = json.RawMessage(rev.Content)
		if rev.Summary != nil {
			dto.RevisionSummary = *rev.Summary
		}

		if rev.SubmittedByName != nil {
			dto.SubmittedBy = *rev.SubmittedByName
		}
	}
	// If no approved revision exists, current_content is nil — the page
	// is a stub waiting for its first approved content.

	// §80 Wiki Types: resolve the page's category template and attach the
	// bound field group's schema+values. Failures degrade to a prose-only
	// page — a missing template or group never blocks the page itself. The
	// category resolves by the page's stored category token (page templates
	// give sibling pages their own slugs under one category), falling back
	// to the slug for pages whose token predates the template columns.
	cat, cerr := s.store.GetCategoryForSpace(ctx, spaceID, page.Category)
	if cerr != nil && page.Category != pageSlug {
		cat, cerr = s.store.GetCategoryForSpace(ctx, spaceID, pageSlug)
	}
	if cerr == nil {
		dto.CategoryLabel = cat.Label

		// §80 lazy template pages: the category's declared template pages —
		// the discovery links for pages that render without a wiki_pages row.
		if tmpls, terr := s.store.ListPageTemplates(ctx, cat.ID); terr == nil {
			dto.PageTemplates = pageTemplateItems(tmpls, page.SpaceName)
		}

		// Effective group: the page-level binding wins over the category's
		// (§80 — template-bound pages carry their own group).
		groupID := cat.FieldGroupID
		if page.FieldGroupID.Valid {
			groupID = page.FieldGroupID
		}

		if groupID.Valid {
			dto.FieldGroupID = groupID.String()
			if s.fields != nil {
				if g, gerr := s.fields.GetFieldGroup(ctx, groupID); gerr == nil {
					dto.FieldGroupSlug = g.Slug
				}
			}
			if grp, ferr := s.GetPageFields(ctx, userID, spaceID, groupID); ferr == nil && grp != nil {
				dto.Fields = grp
			} else if ferr != nil {
				slog.Warn("wiki: page fields load failed",
					"space_id", spaceID.String(), "group_id", groupID.String(), "error", ferr)
			}
		}

		// Rollup: only on the category's own page (sibling template pages
		// render their bound group's items, not the aggregate).
		if rcfg := parseRollupConfig(cat.Rollup); rcfg != nil && page.Slug == cat.Slug {
			if spaceTypeID, _, terr := s.store.GetSpaceTypeRef(ctx, spaceID); terr == nil {
				dto.Rollup = s.computeRollup(ctx, userID, spaceID, spaceTypeID, rcfg, cat.Category)
			}
		}
	}

	// §80 Hierarchical UX: ancestors (breadcrumb chain) + siblings
	// (Prev/Next) accompany every page read — both degrade to empty on
	// lookup failure rather than failing the page.
	if spaceTypeID, _, terr := s.store.GetSpaceTypeRef(ctx, spaceID); terr == nil {
		dto.Ancestors, dto.AncestorsAlt = s.getAncestors(ctx, spaceID, spaceTypeID)
	}
	dto.Siblings = s.getSiblings(ctx, spaceID, page.Category, page.Slug)

	return dto, nil
}

// substituteTemplateLabel expands the `{name}` token in a page-template
// label ("Prices in {name}") with the space's display name. Labels
// without the token pass through unchanged.
func substituteTemplateLabel(label, spaceName string) string {
	return strings.ReplaceAll(label, "{name}", spaceName)
}

// getLazyTemplatePage renders a wiki page straight from the space type's
// page template when no wiki_pages row exists (§80: template pages are
// lazy — they materialize on the first prose/ops submission). The
// synthesized page carries the template's category token, bound field
// group, and ancestors/siblings like a materialized page; prose is empty
// and Lazy=true marks the unmaterialized state. Returns pgx.ErrNoRows
// when no template declares the slug (→ 404).
func (s *Service) getLazyTemplatePage(ctx context.Context, userID, spaceID pgtype.UUID, spaceSlug, spaceName, pageSlug string) (WikiPageDetailDTO, error) {
	tmpl, err := s.store.GetPageTemplateBySlug(ctx, spaceID, pageSlug)
	if err != nil {
		if errors.Is(err, ErrWikiPageTemplateNotFound) {
			return WikiPageDetailDTO{}, pgx.ErrNoRows // no template → real 404
		}

		return WikiPageDetailDTO{}, err
	}

	dto := WikiPageDetailDTO{
		WikiPageDTO: WikiPageDTO{
			SpaceID:   spaceID.String(),
			Slug:      pageSlug,
			Title:     substituteTemplateLabel(tmpl.Label, spaceName),
			Category:  tmpl.CategoryToken,
			IsActive:  true,
			SpaceSlug: spaceSlug,
			SpaceName: spaceName,
			Position:  tmpl.Position,
		},
		SpaceSlug: spaceSlug,
		SpaceName: spaceName,
		Lazy:      true,
	}
	dto.PageTemplateID = tmpl.ID.String()
	if tmpl.FieldGroupID.Valid {
		dto.FieldGroupID = tmpl.FieldGroupID.String()
	}
	if tmpl.FieldGroupSlug != nil {
		dto.FieldGroupSlug = *tmpl.FieldGroupSlug
	}

	// CategoryLabel mirrors the materialized path — the owning category's
	// admin-set label, best-effort.
	if cat, cerr := s.store.GetWikiTypeCategory(ctx, tmpl.CategoryID); cerr == nil {
		dto.CategoryLabel = cat.Label
	}

	// The category's sibling template pages — discovery links, same as a
	// materialized page's.
	if tmpls, terr := s.store.ListPageTemplates(ctx, tmpl.CategoryID); terr == nil {
		dto.PageTemplates = pageTemplateItems(tmpls, spaceName)
	}

	// The bound template group's schema + this space's stored values —
	// the same block a materialized template page renders.
	if tmpl.FieldGroupID.Valid && s.fields != nil {
		if grp, ferr := s.GetPageFields(ctx, userID, spaceID, tmpl.FieldGroupID); ferr == nil && grp != nil {
			dto.Fields = grp
		} else if ferr != nil {
			slog.Warn("wiki: lazy page fields load failed",
				"space_id", spaceID.String(), "group_id", tmpl.FieldGroupID.String(), "error", ferr)
		}
	}

	if spaceTypeID, _, terr := s.store.GetSpaceTypeRef(ctx, spaceID); terr == nil {
		dto.Ancestors, dto.AncestorsAlt = s.getAncestors(ctx, spaceID, spaceTypeID)
	}
	dto.Siblings = s.getSiblings(ctx, spaceID, tmpl.CategoryToken, pageSlug)

	return dto, nil
}

// GetRevisionHistory returns the revision history for a wiki page. Pending
// revisions are still listed for transparency, but their bodies (content,
// ops, summary — all unmoderated user text) are redacted unless the viewer
// is the submitter or holds wiki.review/wiki.manage (canReview).
func (s *Service) GetRevisionHistory(ctx context.Context, viewerID pgtype.UUID, canReview bool, spaceSlug, pageSlug string, page, limit int32) ([]WikiRevisionDTO, int64, error) {
	if _, _, err := s.store.GetSpaceBySlug(ctx, spaceSlug); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, 0, ErrSpaceNotFound
		}

		return nil, 0, fmt.Errorf("wiki: history: resolve space: %w", err)
	}

	wikiPage, err := s.store.GetWikiPageBySlug(ctx, spaceSlug, pageSlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, 0, pgx.ErrNoRows
		}

		return nil, 0, fmt.Errorf("wiki: history: get page: %w", err)
	}

	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	revs, total, err := s.store.ListRevisions(ctx, wikiPage.ID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("wiki: history: list: %w", err)
	}

	dtos := make([]WikiRevisionDTO, 0, len(revs))
	for _, r := range revs {
		dto := revisionToDTO(r)
		// Redact unmoderated bodies on pending revisions for viewers who
		// are neither the submitter nor a reviewer (BUG: public history
		// previously exposed pending content to anonymous readers).
		if r.Status == "pending" && !canReview && !(viewerID.Valid && r.SubmittedBy == viewerID) {
			dto.Content = nil
			dto.Ops = nil
			dto.Summary = ""
		}
		dtos = append(dtos, dto)
	}

	return dtos, total, nil
}

// SubmitRevision creates a new pending revision for a wiki page. The
// caller must have the wiki.edit permission and meet the reputation
// threshold (checked by the handler before calling this method).
// // B6: acquires the jc:wiki_edit_lock:{page_id}:{user_id} Valkey lock before
// inserting to prevent the same user from double-submitting concurrently.
// The lock is released on success or failure; a 5-minute TTL auto-expires
// if the process crashes mid-request.
func (s *Service) SubmitRevision(ctx context.Context, userID pgtype.UUID, spaceSlug, pageSlug string, input SubmitRevisionInput) (string, error) {
	spaceID, spaceName, err := s.store.GetSpaceBySlug(ctx, spaceSlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrSpaceNotFound
		}

		return "", fmt.Errorf("wiki: submit: resolve space: %w", err)
	}

	if len(input.Content) == 0 {
		return "", errors.New("content is required")
	}

	// BUG-020: sanitize/normalize content before storage. If the content is a
	// JSON string (raw HTML from a plain-text editor), sanitize the HTML and
	// wrap it in a TipTap doc structure so the frontend never renders
	// unsanitized HTML via {@html}. If it's already a TipTap JSON object,
	// validate it parses as JSON. Ref: §5.1 XSS, §14 Tiptap sanitize.
	normalizedContent, err := normalizeWikiContent(input.Content)
	if err != nil {
		return "", fmt.Errorf("wiki: submit: invalid content: %w", err)
	}

	input.Content = normalizedContent

	page, err := s.store.GetWikiPageBySlug(ctx, spaceSlug, pageSlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// §80 lazy template pages: the first prose submission
			// materializes the row (bound to the resolved page template),
			// then the revision attaches normally.
			page, err = s.materializeTemplatePage(ctx, spaceID, spaceSlug, spaceName, userID, pageSlug)
		}
		if err != nil {
			return "", err
		}
	}

	if page.IsLocked {
		return "", ErrPageLocked
	}

	// B6: acquire the per-user-per-page edit lock. SET NX EX 5m.
	// Platform-scoped per Rule 4 (VALKEY-004): jc:wiki_edit_lock:{}:{page_id}:{user_id}
	lockKey := valkey.MustKey("wiki_edit_lock::%s:%s", page.ID.String(), userID.String())
	if s.vk != nil {
		acquired, lerr := s.vk.SetNX(ctx, lockKey, "1", 5*time.Minute).Result()
		if lerr != nil {
			slog.Warn("wiki: edit lock acquire failed, proceeding", "error", lerr)
			// Fail open — degrade gracefully if Valkey is unreachable.
		} else if !acquired {
			return "", ErrEditInProgress
		}

		defer func() {
			if err := s.vk.Del(ctx, lockKey).Err(); err != nil {
				slog.Warn("wiki: edit lock release failed", "error", err)
			}
		}()
	}

	// Content-only path — structured field ops submitted alongside prose are
	// routed through SubmitFieldOps (§80 community field ops).
	revID, err := s.store.SubmitRevision(ctx, page.ID, userID, "content", input.Content, nil, input.Summary, nil, pgtype.UUID{})
	if err != nil {
		if errors.Is(err, ErrDuplicateContent) {
			return "", ErrDuplicateContent
		}

		return "", fmt.Errorf("wiki: submit: %w", err)
	}

	return revID.String(), nil
}

// materializeTemplatePage creates the wiki_pages row for a lazy template
// page (§80): the slug must resolve to a page template on the space's
// type — untemplated slugs keep returning pgx.ErrNoRows. Returns the
// synthesized page for callers that need the row id (revision attach,
// edit lock).
func (s *Service) materializeTemplatePage(ctx context.Context, spaceID pgtype.UUID, spaceSlug, spaceName string, createdBy pgtype.UUID, pageSlug string) (WikiPageWithSpace, error) {
	tmpl, err := s.store.GetPageTemplateBySlug(ctx, spaceID, pageSlug)
	if err != nil {
		if errors.Is(err, ErrWikiPageTemplateNotFound) {
			return WikiPageWithSpace{}, pgx.ErrNoRows
		}

		return WikiPageWithSpace{}, fmt.Errorf("wiki: materialize page: resolve template: %w", err)
	}

	title := substituteTemplateLabel(tmpl.Label, spaceName)
	pageID, err := s.store.createPageBound(ctx, s.store.write, spaceID, createdBy, pageSlug, title, tmpl.CategoryToken, tmpl.ID, tmpl.FieldGroupID, tmpl.Position)
	if err != nil {
		if errors.Is(err, ErrPageExists) {
			// Concurrent first submission lost the create race — the page
			// row exists now; re-resolve and continue.
			page, rerr := s.store.GetWikiPageBySlug(ctx, spaceSlug, pageSlug)
			if rerr != nil {
				return WikiPageWithSpace{}, pgx.ErrNoRows
			}

			return page, nil
		}

		return WikiPageWithSpace{}, fmt.Errorf("wiki: materialize page: %w", err)
	}

	return WikiPageWithSpace{
		WikiPage: WikiPage{
			ID:             pageID,
			SpaceID:        spaceID,
			Slug:           pageSlug,
			Title:          title,
			Category:       tmpl.CategoryToken,
			IsActive:       true,
			PageTemplateID: tmpl.ID,
			FieldGroupID:   tmpl.FieldGroupID,
			Position:       tmpl.Position,
		},
		SpaceSlug: spaceSlug,
		SpaceName: spaceName,
	}, nil
}

// CreatePageWithRevision creates a new wiki page for the given space slug and
// immediately submits an initial pending revision. The caller (handler) is
// responsible for permission and reputation checks.
func (s *Service) CreatePageWithRevision(ctx context.Context, userID pgtype.UUID, input CreateWikiPageInput) (string, string, error) {
	if input.Title == "" || input.Slug == "" || input.Category == "" {
		return "", "", errors.New("title, slug, and category are required")
	}

	if len(input.Content) == 0 {
		return "", "", errors.New("content is required")
	}

	normalizedContent, err := normalizeWikiContent(input.Content)
	if err != nil {
		return "", "", fmt.Errorf("wiki: create page: invalid content: %w", err)
	}

	spaceID, _, err := s.store.GetSpaceBySlug(ctx, input.SpaceSlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", ErrSpaceNotFound
		}

		return "", "", fmt.Errorf("wiki: create page: resolve space: %w", err)
	}

	// §80 Hierarchical UX: when a page template declares this (category,
	// slug) pair on the space's type, the page row stores the resolved
	// binding — the group survives a later template deletion (effective
	// group falls back to the category's when absent).
	var tmplID, tmplGroupID pgtype.UUID
	if t, terr := s.store.GetPageTemplateForSpace(ctx, spaceID, input.Category, input.Slug); terr == nil {
		tmplID = t.ID
		tmplGroupID = t.FieldGroupID
	}

	var position int32
	if input.Position != nil {
		position = *input.Position
	}

	pageID, revID, err := s.store.CreatePageWithRevision(ctx, spaceID, userID, input.Slug, input.Title, input.Category, normalizedContent, input.Summary, tmplID, tmplGroupID, position)
	if err != nil {
		if errors.Is(err, ErrPageExists) {
			return "", "", ErrPageExists
		}

		return "", "", fmt.Errorf("wiki: create page: %w", err)
	}

	return pageID.String(), revID.String(), nil
}

// ApproveRevision approves a pending revision (moderator+). Returns the
// approved revision's content (TipTap JSON bytes) so the handler can
// fire-and-forget RAG indexing (B8, §69/ADR-069); fields-only revisions
// return nil content — they carry no prose worth indexing.
//
// §80 community field ops: every kind routes through ApproveRevisionByID so
// fields/mixed revisions apply their ops array transactionally (approve +
// apply share one commit) and trigger wiki:fields_propagate. Content
// revisions take the plain status-flip path inside ApproveRevisionByID.
func (s *Service) ApproveRevision(ctx context.Context, revID, pageID, reviewerID pgtype.UUID, notes string) ([]byte, error) {
	rev, err := s.store.GetRevision(ctx, revID)
	if err != nil {
		return nil, fmt.Errorf("wiki: approve: %w", err)
	}

	// The page-scoped route only approves revisions bound to that page;
	// page-less ops revisions go through the revision-keyed route (§80).
	if !rev.WikiPageID.Valid || rev.WikiPageID != pageID {
		return nil, pgx.ErrNoRows
	}

	if err := s.ApproveRevisionByID(ctx, revID, reviewerID, notes); err != nil {
		return nil, fmt.Errorf("wiki: approve: %w", err)
	}

	if rev.Kind == "fields" {
		return nil, nil
	}

	return rev.Content, nil
}

// RejectRevision rejects a pending revision (moderator+) via the
// page-scoped route. Mirrors ApproveRevision: the revision must be bound to
// pageID — a mismatch returns pgx.ErrNoRows ("not found for this page").
// Page-less ops revisions go through the revision-keyed route (§80).
func (s *Service) RejectRevision(ctx context.Context, revID, pageID, reviewerID pgtype.UUID, notes string) error {
	rev, err := s.store.GetRevision(ctx, revID)
	if err != nil {
		return fmt.Errorf("wiki: reject: %w", err)
	}

	if !rev.WikiPageID.Valid || rev.WikiPageID != pageID {
		return pgx.ErrNoRows
	}

	if err := s.RejectRevisionByID(ctx, revID, reviewerID, notes); err != nil {
		return fmt.Errorf("wiki: reject: %w", err)
	}

	return nil
}

// UpdatePage updates admin-managed wiki page fields (lock, feature, active,
// data entity binding).
func (s *Service) UpdatePage(ctx context.Context, pageID pgtype.UUID, input UpdatePageInput) error {
	if err := s.store.UpdatePage(ctx, pageID, input.IsLocked, input.IsFeatured, input.IsActive, input.DataEntityType, input.Position); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("wiki: update page: %w", err)
	}

	return nil
}

// AdminListPages returns all wiki pages for admin management (paginated).
// Each row includes the latest revision metadata for the review queue (B1).
func (s *Service) AdminListPages(ctx context.Context, spaceID pgtype.UUID, destSlug, category, q, sort string, isLocked *bool, deleted string, page, limit int32) ([]AdminWikiPageDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	pages, total, err := s.store.AdminListPages(ctx, AdminListPagesFilter{

		SpaceID:  spaceID,
		DestSlug: destSlug,
		Category: category,
		IsLocked: isLocked,
		Q:        q,
		Sort:     sort,
		Deleted:  deleted,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("wiki: admin list: %w", err)
	}

	dtos := make([]AdminWikiPageDTO, 0, len(pages))

	for _, p := range pages {
		dto := AdminWikiPageDTO{
			WikiPageDTO: pageToDTO(p.WikiPageWithSpace),
		}
		if p.LatestRevisionID != nil {
			dto.LatestRevisionID = p.LatestRevisionID.String()
		}

		if p.LatestRevisionStatus != nil {
			dto.LatestRevisionStatus = *p.LatestRevisionStatus
		}

		if p.Excerpt != nil {
			dto.Excerpt = *p.Excerpt
		}

		if p.DeletedAt.Valid {
			dto.DeletedAt = formatTs(p.DeletedAt)
		}

		dtos = append(dtos, dto)
	}

	return dtos, total, nil
}

// DeletePage soft-deletes a wiki page, or permanently removes it when
// force=true — force requires a prior soft delete (ErrPageNotDeleted → 409).
// Soft delete also flips is_active=false so every public reader drops the
// page (§80).
func (s *Service) DeletePage(ctx context.Context, pageID pgtype.UUID, force bool) error {
	var err error
	if force {
		err = s.store.HardDeleteWikiPage(ctx, pageID)
	} else {
		err = s.store.DeleteWikiPage(ctx, pageID)
	}
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, ErrPageNotDeleted) {
			return err
		}
		return fmt.Errorf("wiki: delete page: %w", err)
	}
	return nil
}

// ErrPageLocked is returned when a wiki page is locked and cannot accept
// community edits.
var ErrPageLocked = errors.New("wiki page is locked to mod-only edits")

// ErrEditInProgress is returned when the same user already has a concurrent
// wiki edit submission in-flight for the same page (B6: jc:wiki_edit_lock).
var ErrEditInProgress = errors.New("a wiki edit is already in progress for this page by this user")

// ErrSpaceNotFound is returned when the destination space slug does not exist.
var ErrSpaceNotFound = errors.New("destination space not found")

// GeneratePage creates a stub wiki page for a space/category combination,
// resolving the space_slug to a space_id. If the page already exists (same
// slug within the space), it returns the existing page id (idempotent). The
// actual AI draft content is added later as a pending ai_generated revision
// via the Content Agent — this method only ensures the page shell exists
// (B3: replaces the former stub handler).
func (s *Service) GeneratePage(ctx context.Context, userID pgtype.UUID, input GeneratePageInput) (string, error) {
	spaceID, _, err := s.store.GetSpaceBySlug(ctx, input.SpaceSlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrSpaceNotFound
		}

		return "", fmt.Errorf("wiki: generate: resolve space: %w", err)
	}

	category := input.Category
	if category == "" {
		category = "other"
	}

	// §80: template-bound slugs are reserved per type — a generated page at
	// a declared (category, slug) lands on the template's binding like a
	// hand-created one.
	var tmplID, tmplGroupID pgtype.UUID
	if t, terr := s.store.GetPageTemplateForSpace(ctx, spaceID, category, input.PageSlug); terr == nil {
		tmplID = t.ID
		tmplGroupID = t.FieldGroupID
	}

	pageID, err := s.store.createPageBound(ctx, s.store.write, spaceID, userID, input.PageSlug, input.Title, category, tmplID, tmplGroupID, 0)
	if err != nil {
		if errors.Is(err, ErrPageExists) {
			// Page already exists — return a sentinel so the handler can 200
			// instead of 201. The caller can still enqueue an AI draft for it.
			return "", ErrPageExists
		}

		return "", fmt.Errorf("wiki: generate: create page: %w", err)
	}

	return pageID.String(), nil
}

// pageToDTO converts a WikiPage (with its space context) to a WikiPageDTO.
func pageToDTO(p WikiPageWithSpace) WikiPageDTO {
	dto := WikiPageDTO{
		ID:             p.ID.String(),
		SpaceID:        p.SpaceID.String(),
		Slug:           p.Slug,
		Title:          p.Title,
		Category:       p.Category,
		IsFeatured:     p.IsFeatured,
		IsLocked:       p.IsLocked,
		IsActive:       p.IsActive,
		CreatedAt:      formatTs(p.CreatedAt),
		UpdatedAt:      formatTs(p.UpdatedAt),
		SpaceSlug:      p.SpaceSlug,
		SpaceName:      p.SpaceName,
		DataEntityType: p.DataEntityType,
		Position:       p.Position,
	}
	if p.PageTemplateID.Valid {
		dto.PageTemplateID = p.PageTemplateID.String()
	}

	return dto
}

// revisionToDTO converts a WikiRevision to a WikiRevisionDTO.
func revisionToDTO(r WikiRevision) WikiRevisionDTO {
	dto := WikiRevisionDTO{
		ID:          r.ID.String(),
		Status:      r.Status,
		Kind:        r.Kind,
		AiGenerated: r.AiGenerated,
		CreatedAt:   formatTs(r.CreatedAt),
	}
	if len(r.Content) > 0 {
		dto.Content = json.RawMessage(r.Content)
	}

	if len(r.Ops) > 0 {
		dto.Ops = json.RawMessage(r.Ops)
	}

	if r.TargetEntityType != nil {
		dto.TargetEntityType = *r.TargetEntityType
	}

	if r.TargetEntityID.Valid {
		dto.TargetEntityID = r.TargetEntityID.String()
	}

	if r.Summary != nil {
		dto.Summary = *r.Summary
	}

	if r.SubmittedByName != nil {
		dto.SubmittedBy = *r.SubmittedByName
	}

	if r.ReviewedByName != nil && *r.ReviewedByName != "" {
		dto.ReviewedBy = *r.ReviewedByName
	} else if r.ReviewedBy.Valid {
		dto.ReviewedBy = r.ReviewedBy.String()
	}

	dto.ReviewedAt = formatTs(r.ReviewedAt)

	if r.ApplyError != nil {
		dto.ApplyError = *r.ApplyError
	}

	return dto
}

// formatTs renders a pgtype.Timestamptz as RFC 3339 UTC, or "" if NULL.
func formatTs(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}

	return t.Time.UTC().Format(time.RFC3339)
}

// slugify produces a lowercase, hyphen-joined slug from a free-form title.
func slugify(s string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), " ", "-"))
}

// GenerateDraft creates a wiki page draft via the Content Agent (ADR-088).
// The draft page + pending revision are created with ai_generated=true,
// ai_model='content_agent_v1', and ai_prompt_hash=SHA256(prompt). It
// requires HITL approval (ApproveRevision) before publication.
// // When an AI dispatcher is wired (SetDispatcher), the method calls the AI
// Gateway via the "content_generation" category. If the dispatcher is nil,
// the draft is still created with the prompt as content so a reviewer can
// fill it manually (graceful degradation).
func (s *Service) GenerateDraft(ctx context.Context, input GenerateDraftRequest) (GenerateDraftResponse, error) {
	if input.SpaceID == "" {
		return GenerateDraftResponse{}, errors.New("space_id is required")
	}

	if input.Title == "" {
		return GenerateDraftResponse{}, errors.New("title is required")
	}

	var spaceID pgtype.UUID
	if err := spaceID.Scan(input.SpaceID); err != nil {
		return GenerateDraftResponse{}, fmt.Errorf("wiki: generate draft: invalid space_id: %w", err)
	}

	slug := input.PageSlug
	if slug == "" {
		slug = slugify(input.Title)
	}

	// Build the prompt sent to the Content Agent.
	prompt := input.Prompt
	if prompt == "" {
		prompt = fmt.Sprintf("Generate a wiki page about: %s (category: %s)", input.Title, input.Category)
	}

	// ai_prompt_hash = SHA256(prompt) for audit traceability.
	sum := sha256.Sum256([]byte(prompt))
	promptHash := hex.EncodeToString(sum[:])

	// Call the AI Gateway when a dispatcher is wired.
	generatedText := prompt

	if s.dispatcher != nil {
		resp, aiErr := s.dispatcher.Dispatch(ctx, "content_generation", ai.Request{
			Category: "content_generation",
			Payload: map[string]any{
				"prompt": prompt,

				"guide_type": "wiki_page",
			},
		})
		if aiErr != nil {
			return GenerateDraftResponse{}, fmt.Errorf("wiki: AI generation failed: %w", aiErr)
		}

		if text, ok := resp.Raw["text"].(string); ok {
			generatedText = text
		} else if text, ok := resp.Raw["content"].(string); ok {
			generatedText = text
		} else if text, ok := resp.Raw["output"].(string); ok {
			generatedText = text
		}

		slog.Info("wiki: AI Gateway generated draft", "space_id", input.SpaceID, "title", input.Title, "prompt_hash", promptHash)
	}

	// Create the wiki page.
	var createdBy pgtype.UUID // zero UUID — AI-generated, no human author

	// §80: an AI draft at a template-bound slug stores the resolved
	// (category, slug) binding, same as member/admin-created pages.
	var tmplID, tmplGroupID pgtype.UUID
	if t, terr := s.store.GetPageTemplateForSpace(ctx, spaceID, input.Category, slug); terr == nil {
		tmplID = t.ID
		tmplGroupID = t.FieldGroupID
	}

	pageID, err := s.store.createPageBound(ctx, s.store.write, spaceID, createdBy, slug, input.Title, input.Category, tmplID, tmplGroupID, 0)
	if err != nil {
		return GenerateDraftResponse{}, fmt.Errorf("wiki: generate draft: create page: %w", err)
	}

	// Create a pending revision with the AI-generated content as TipTap JSON.
	tiptapJSON := []byte(fmt.Sprintf(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":%q}]}]}`, generatedText))
	aiModel := "content_agent_v1"
	summary := fmt.Sprintf("AI-generated draft (model: %s)", aiModel)

	if _, err := s.store.SubmitAIRevision(ctx, pageID, createdBy, tiptapJSON, summary, aiModel, promptHash); err != nil {
		return GenerateDraftResponse{}, fmt.Errorf("wiki: generate draft: submit revision: %w", err)
	}

	return GenerateDraftResponse{
		Status:  "accepted",
		Message: "Content Agent draft created with pending revision. Review and approve to publish.",
		PageID:  pageID.String(),
	}, nil
}

// SeedStubPages materializes the wiki type's category template as wiki
// pages for a space (§80 Wiki Types amendment). The template — slug, title,
// category token — comes from wiki_type_categories for the space's type,
// replacing the former hardcoded 7-page country set. Idempotent: pages that
// already exist (matched by space_id + slug) are skipped via the store's
// ON CONFLICT DO NOTHING. Used by the wiki:seed_stub_pages Asynq task
// handler.
// // Ref: japaconnect-engineering/sections/80-destination-wiki.md
// //	japaconnect-engineering/adr/ADR-088-destination-wiki.md
func (s *Service) SeedStubPages(ctx context.Context, spaceID pgtype.UUID) error {
	if !spaceID.Valid {
		return errors.New("wiki: seed stub pages: space_id is required")
	}

	spaceTypeID, _, err := s.store.GetSpaceTypeRef(ctx, spaceID)
	if err != nil {
		return fmt.Errorf("wiki: seed stub pages: resolve space type: %w", err)
	}

	specs, err := s.store.ListSeedCategories(ctx, spaceTypeID)
	if err != nil {
		return err
	}

	var createdBy pgtype.UUID // system-created stub pages have no human author
	for _, spec := range specs {
		if _, err := s.store.CreatePage(ctx, spaceID, createdBy, spec.Slug, spec.Label, spec.Category); err != nil {
			if errors.Is(err, ErrPageExists) {
				continue // idempotent — page already seeded
			}

			return fmt.Errorf("wiki: seed stub page %q: %w", spec.Slug, err)
		}
	}

	return nil
}

// EnqueueStubSeedFanOut enqueues one wiki:seed_stub_pages task per active
// space whose wiki type opts into stub seeding — the §80 activation-time
// background job wired to the wiki.enabled flag transition
// (features.SetOnFlagEnabled in server.go). Each task is idempotent, so
// re-activation is harmless. Failures on individual spaces are logged and
// skipped — one bad space must not abort the rest of the fan-out.
func (s *Service) EnqueueStubSeedFanOut(ctx context.Context) {
	if s.enqueuer == nil {
		slog.Warn("wiki: seed stub fan-out skipped — no Asynq enqueuer wired")
		return
	}

	ids, err := s.store.ListSeedSpaceIDs(ctx)
	if err != nil {
		slog.Error("wiki: seed stub fan-out: list country spaces failed", "error", err)
		return
	}

	for _, id := range ids {
		payload, err := json.Marshal(map[string]string{"space_id": id.String()})
		if err != nil {
			continue
		}
		// Unique dedupes a rapid off→on→off→on flag toggle — same payload
		// per space collapses to one pending task.
		if err := s.enqueuer.Enqueue(TaskSeedStubPages, payload, asynq.Unique(time.Hour)); err != nil &&
			!errors.Is(err, asynq.ErrDuplicateTask) {
			slog.Warn("wiki: seed_stub_pages enqueue failed", "space_id", id.String(), "error", err)
		}
	}

	slog.Info("wiki: seed_stub_pages fan-out enqueued", "spaces", len(ids))
}

// normalizeWikiContent ensures wiki revision content is always stored as a
// valid TipTap JSON document, never as raw HTML. This prevents stored XSS
// when the frontend renders content with {@html}.
// // If the content is a JSON string (e.g. "some text"), it is treated as plain
// text, sanitized, and wrapped in a TipTap doc. If it is a JSON object, it
// is validated as a TipTap doc and the text nodes are sanitized.
// // Ref: japaconnect-engineering/sections/14-content-type-system.md
// //	japaconnect-engineering/sections/05-authentication-sessions.md (XSS)
func normalizeWikiContent(raw json.RawMessage) (json.RawMessage, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	switch val := v.(type) {
	case string:
		// Raw string content — sanitize as HTML and wrap in a TipTap doc.
		sanitized := contentpkg.SanitizeHTML(val)
		doc := map[string]any{
			"type": "doc",
			"content": []any{
				map[string]any{
					"type": "paragraph",
					"content": []any{
						map[string]any{"type": "text", "text": sanitized},
					},
				},
			},
		}

		return json.Marshal(doc)
	case map[string]any:
		// Already a TipTap JSON object — validate it has a type field.
		if _, ok := val["type"]; !ok {
			return nil, errors.New("invalid TipTap document: missing type field")
		}
		// Recursively sanitize text nodes to strip any embedded HTML.
		sanitizeTipTapNodes(val)

		return json.Marshal(val)
	default:
		return nil, errors.New("content must be a string or TipTap JSON object")
	}
}

// sanitizeTipTapNodes walks a TipTap node tree and sanitizes text node
// values to strip any embedded HTML tags.
func sanitizeTipTapNodes(node map[string]any) {
	if text, ok := node["text"].(string); ok {
		node["text"] = contentpkg.SanitizeText(text)
	}

	if content, ok := node["content"].([]any); ok {
		for _, child := range content {
			if childMap, ok := child.(map[string]any); ok {
				sanitizeTipTapNodes(childMap)
			}
		}
	}
}
