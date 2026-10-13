// Package ai — tools_register.go registers the Phase 4 core tool catalog
// (Section 103, ADR-111) with a ToolHandlerRegistry.
//
// The wiring layer (internal/server) calls RegisterPhase4Tools after
// constructing a ToolDeps with the real service implementations. This
// helper keeps the server wiring concise and ensures every Phase 4 tool
// handler + privacy target is registered in one place.
//
// Phase 9 (extended catalog) is registered via RegisterPhase9Tools, also
// defined in this file. The server wiring calls both.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Tool Catalog Expansion)
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 4, tasks 4.01-4.10)
package ai

// RegisterPhase4Tools registers all Phase 4 core tool catalog handlers and
// their privacy-target extractors with the given registry. The deps
// container provides the service implementations the handlers close over.
//
// The tool function names here must match the `function_name` field in the
// ai_tools.config JSONB seeded by migration 000388. A mismatch means the
// executor cannot find the handler for a tool call the LLM emits.
func RegisterPhase4Tools(registry *ToolHandlerRegistry, deps *ToolDeps) {
	if registry == nil || deps == nil {
		return
	}

	// --- Content tools (tasks 4.01-4.04) ---
	registry.RegisterHandler("content.search", ContentSearchHandler(deps))
	registry.RegisterHandler("content.read", ContentReadHandler(deps))
	registry.RegisterPrivacyTarget("content.read", ContentReadPrivacyTarget)
	registry.RegisterHandler("content.create", ContentCreateHandler(deps))
	registry.RegisterHandler("content.bulk_create", ContentBulkCreateHandler(deps))
	registry.RegisterHandler("content.update", ContentUpdateHandler(deps))
	registry.RegisterPrivacyTarget("content.update", ContentUpdatePrivacyTarget)
	registry.RegisterHandler("content_types.list", ContentTypesListHandler(deps))
	registry.RegisterHandler("guide_categories.list", GuideCategoriesListHandler(deps))

	// --- Assist tools (§20 V1 catalog; migration 000847) ---
	// The six in-editor assist ops, shared with POST /ai/assist/* via
	// deps.Assist. All six ai_tools rows carry
	// required_permission='ai.editor.assist' so unauthorized users never
	// see them in the catalog. content.editor_assist (the v1 umbrella row)
	// stays disabled — superseded by the granular ops.
	registry.RegisterHandler("content.improve", AssistImproveHandler(deps))
	registry.RegisterHandler("content.summarize", AssistSummarizeHandler(deps))
	registry.RegisterHandler("platform.translate_string", AssistTranslateHandler(deps))
	registry.RegisterHandler("content.suggest_tags", AssistSuggestTagsHandler(deps))
	registry.RegisterHandler("content.generate_og", AssistGenerateOGHandler(deps))
	registry.RegisterHandler("content.generate_draft", AssistGenerateDraftHandler(deps))

	// --- Content draft-request tool (Phase 8, tasks 8.01 + 8.02 + 8.05) ---
	// content.create_draft is gated by agent.draft_request (not
	// content.create) at the catalog layer. It forces status="draft",
	// forces author_id to the AI persona's backing user, and creates a
	// HITL review with source='agent_draft_request'.
	registry.RegisterHandler("content.create_draft", ContentCreateDraftHandler(deps))

	// --- Custom field tools (tasks 4.05-4.08) ---
	registry.RegisterHandler("custom_fields.list", CustomFieldsListHandler(deps))
	registry.RegisterHandler("custom_field_values.read", CFVReadHandler(deps))
	registry.RegisterPrivacyTarget("custom_field_values.read", CFVReadPrivacyTarget)
	registry.RegisterHandler("custom_field_values.set", CFVSetHandler(deps))
	registry.RegisterPrivacyTarget("custom_field_values.set", CFVSetPrivacyTarget)
	registry.RegisterHandler("custom_field_values.bulk_set", CFVBulkSetHandler(deps))
	registry.RegisterPrivacyTarget("custom_field_values.bulk_set", CFVBulkSetPrivacyTarget)

	// --- Knowledge tools (tasks 4.09-4.10) ---
	registry.RegisterHandler("rag.search", RAGSearchHandler(deps))
	registry.RegisterHandler("external.search", ExternalSearchHandler(deps))

	// --- Intelligence tools (ADR-076 Amendment 2, on-demand source/record
	// refresh and content regeneration).
	registry.RegisterHandler("source.crawl_now", SourceCrawlNowHandler(deps))
	registry.RegisterHandler("record.update_now", RecordUpdateNowHandler(deps))
	registry.RegisterHandler("content.regenerate_from_source", ContentRegenerateFromSourceHandler(deps))
	registry.RegisterHandler("scholarship.search", ScholarshipSearchHandler(deps))
	registry.RegisterHandler("source.discover", SourceDiscoverHandler(deps))

	// --- Comparison tools (Phase 7, task 7.06) ---
	RegisterPhase7ComparisonTools(registry, deps)
}

// RegisterPhase7ComparisonTools registers the Phase 7 comparison tool
// handlers and their privacy-target extractors with the given registry.
// Called by RegisterPhase4Tools (above) so a single RegisterPhase4Tools
// call wires the full Phase 4 + Phase 7 catalog. The tool function names
// here must match the `function_name` field in the ai_tools.config JSONB
// seeded by migration 000393.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Comparison Engine)
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 7, task 7.06).
func RegisterPhase7ComparisonTools(registry *ToolHandlerRegistry, deps *ToolDeps) {
	if registry == nil || deps == nil {
		return
	}

	// comparison.types.list — read-only, no privacy target.
	registry.RegisterHandler("comparison.types.list", ComparisonTypesListHandler(deps))

	// comparison.create — writes a new content row, no privacy target.
	registry.RegisterHandler("comparison.create", ComparisonCreateHandler(deps))

	// comparison.fill_values — writes values on an existing content row;
	// privacy gate blocks when the backing content is ai_exempt.
	registry.RegisterHandler("comparison.fill_values", ComparisonFillValuesHandler(deps))
	registry.RegisterPrivacyTarget("comparison.fill_values", ComparisonFillValuesPrivacyTarget)

	// comparison.add_dimension — adds dimensions to an existing content row.
	// No privacy target: this creates new dimension rows, not reads/writes
	// of the content body. The executor's RBAC gate checks the tool's
	// required_permission.
	registry.RegisterHandler("comparison.add_dimension", ComparisonAddDimensionHandler(deps))

	// comparison.read — reads a comparison matrix; privacy gate blocks
	// when the backing content is ai_exempt.
	registry.RegisterHandler("comparison.read", ComparisonReadHandler(deps))
	registry.RegisterPrivacyTarget("comparison.read", ComparisonReadPrivacyTarget)

	// --- JCE Sub-spec 4 refresh tool (Task 8) ---
	// comparison.refresh — trigger a refresh of stale comparison values.
	registry.RegisterHandler("comparison.refresh", JCERefreshHandler(deps))
	registry.RegisterPrivacyTarget("comparison.refresh", JCERefreshPrivacyTarget)
}

// RegisterPhase9Tools registers all Phase 9 extended tool catalog handlers
// and their privacy-target extractors with the given registry. The deps
// container provides the service implementations the handlers close over.
//
// The tool function names here must match the `function_name` field in the
// ai_tools.config JSONB seeded by the Phase 9 migration. A mismatch means
// the executor cannot find the handler for a tool call the LLM emits.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Tool Catalog Expansion)
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 9, tasks 9.01-9.08).
func RegisterPhase9Tools(registry *ToolHandlerRegistry, deps *ToolDeps) {
	if registry == nil || deps == nil {
		return
	}

	// --- Space tools (task 9.01) ---
	// No privacy targets — spaces have no ai_exempt column.
	registry.RegisterHandler("spaces.find", SpacesFindHandler(deps))
	registry.RegisterHandler("spaces.read", SpacesReadHandler(deps))
	registry.RegisterHandler("spaces.list", SpacesListHandler(deps))
	registry.RegisterHandler("spaces.create", SpacesCreateHandler(deps))
	registry.RegisterHandler("spaces.update", SpacesUpdateHandler(deps))

	// --- Academy tools (task 9.02) ---
	// No privacy targets — academy data has no ai_exempt column.
	registry.RegisterHandler("academy.tracks.list", AcademyTracksListHandler(deps))
	registry.RegisterHandler("academy.tracks.create", AcademyTracksCreateHandler(deps))
	registry.RegisterHandler("academy.tracks.update", AcademyTracksUpdateHandler(deps))
	registry.RegisterHandler("academy.lessons.list", AcademyLessonsListHandler(deps))
	registry.RegisterHandler("academy.lessons.create", AcademyLessonsCreateHandler(deps))
	registry.RegisterHandler("academy.lessons.update", AcademyLessonsUpdateHandler(deps))
	registry.RegisterHandler("academy.lessons.reorder", AcademyLessonsReorderHandler(deps))

	// --- Docs tools (task 9.03) ---
	// No privacy targets — docs have no ai_exempt column.
	// docs.organize is registered but seeded with is_enabled = false in the
	// migration because the docs service does not expose a parent_id/
	// position update query. The handler returns "not configured" when
	// called. When the backing query is added, flip is_enabled to true.
	registry.RegisterHandler("docs.list", DocsListHandler(deps))
	registry.RegisterHandler("docs.read", DocsReadHandler(deps))
	registry.RegisterHandler("docs.create", DocsCreateHandler(deps))
	registry.RegisterHandler("docs.update", DocsUpdateHandler(deps))
	registry.RegisterHandler("docs.organize", DocsOrganizeHandler(deps))

	// --- Tools-app tools (task 9.04) ---
	// tools.list and tools.read are fully implemented. tools.create,
	// tools.update and tools.assign_geo are disabled stubs; the real write
	// handlers are the dedicated per-use-case tools below.
	registry.RegisterHandler("tools.list", ToolsListHandler(deps))
	registry.RegisterHandler("tools.read", ToolsReadHandler(deps))
	registry.RegisterHandler("tools.create", ToolsCreateHandler(deps))
	registry.RegisterHandler("tools.update", ToolsUpdateHandler(deps))
	registry.RegisterHandler("tools.assign_geo", ToolsAssignGeoHandler(deps))

	// --- Dedicated per-use-case write tools (design: 2026-08-24-dedicated-ai-tool-crud) ---
	// Registration is per resolved op: a tool type whose backing service
	// lacks e.g. a delete method gets no <type>.delete handler, so its
	// seeded row must be disabled to avoid a catalog-advertised dead tool.
	// Read-only catalog types (e.g. fx_rate) are still surfaced via
	// tools.list/read.
	if deps != nil && deps.ToolsRegistry != nil {
		for toolType, ops := range deps.ToolsRegistry.WritableToolOps() {
			if ops["create"] {
				registry.RegisterHandler(toolType+".create", UseCaseCreateHandler(deps, toolType))
			}
			if ops["update"] {
				registry.RegisterHandler(toolType+".update", UseCaseUpdateHandler(deps, toolType))
			}
			if ops["delete"] {
				registry.RegisterHandler(toolType+".delete", UseCaseDeleteHandler(deps, toolType))
			}
			if ops["assign_geo"] {
				registry.RegisterHandler(toolType+".assign_geo", UseCaseAssignGeoHandler(deps, toolType))
			}
		}
	}

	// --- Menu tools (task 9.05) ---
	// No privacy targets — menus have no ai_exempt column.
	registry.RegisterHandler("menu.list", MenuListHandler(deps))
	registry.RegisterHandler("menu.create", MenuCreateHandler(deps))
	registry.RegisterHandler("menu.update", MenuUpdateHandler(deps))
	registry.RegisterHandler("menu.delete", MenuDeleteHandler(deps))

	// --- User tools (task 9.06) ---
	// user.read_profile targets a user entity — register a privacy target.
	// user.update_profile and user.read_own_fields are self-only (the
	// handler enforces user_id == calling user), so no privacy target.
	registry.RegisterHandler("user.read_profile", UserReadProfileHandler(deps))
	registry.RegisterPrivacyTarget("user.read_profile", UserReadProfilePrivacyTarget)
	registry.RegisterHandler("user.update_profile", UserUpdateProfileHandler(deps))
	registry.RegisterHandler("user.read_own_fields", UserReadOwnFieldsHandler(deps))

	// --- Custom field definition tools (task 9.07) ---
	// No privacy targets — field definitions are schema, not user data.
	registry.RegisterHandler("custom_fields.create", CustomFieldsCreateHandler(deps))
	registry.RegisterHandler("custom_fields.update", CustomFieldsUpdateHandler(deps))
	registry.RegisterHandler("custom_field_groups.list", CustomFieldGroupsListHandler(deps))
	registry.RegisterHandler("custom_field_groups.create", CustomFieldGroupsCreateHandler(deps))

	// --- Content lifecycle tools (task 9.08) ---
	// content.delete and content.publish target a content entity — register
	// privacy targets. content.list returns a result set (no privacy target;
	// exempt content is filtered post-query).
	registry.RegisterHandler("content.delete", ContentDeleteHandler(deps))
	registry.RegisterPrivacyTarget("content.delete", ContentDeletePrivacyTarget)
	registry.RegisterHandler("content.list", ContentListHandler(deps))
	registry.RegisterHandler("content.publish", ContentPublishHandler(deps))
	registry.RegisterPrivacyTarget("content.publish", ContentPublishPrivacyTarget)
	registry.RegisterHandler("content.bulk_publish", ContentBulkPublishHandler(deps))
	registry.RegisterPrivacyTarget("content.bulk_publish", ContentBulkPublishPrivacyTarget)

	// --- Section 104 — AI Content Campaigns ---
	registry.RegisterHandler("personas.create", PersonasCreateHandler(deps))
	registry.RegisterHandler("content.campaign.create", ContentCampaignCreateHandler(deps))

	// --- Persona voice + catalog read tools (§68, ADR-114) ---
	// Read-only discovery tools — persona_voices and ai_personas are admin
	// config, not user data, so no privacy targets are registered. The
	// catalog RBAC gate enforces persona.view (migration 000722).
	registry.RegisterHandler("persona_voices.list", PersonaVoicesListHandler(deps))
	registry.RegisterHandler("persona_voices.read", PersonaVoicesReadHandler(deps))
	registry.RegisterHandler("personas.list", PersonasListHandler(deps))
	registry.RegisterHandler("personas.read", PersonasReadHandler(deps))

	// --- Agent data tools (spec 2026-09-23-agent-data-api) ---
	// Read-only moat reads via the publicapi facade — the registered names
	// match the MCP apiKeyToolGates allowlist exactly.
	registry.RegisterHandler("visa.search", VisaSearchHandler(deps))
	registry.RegisterHandler("visa.get", VisaGetHandler(deps))
	registry.RegisterHandler("visa.compare", VisaCompareHandler(deps))
	registry.RegisterHandler("university.get", UniversityGetHandler(deps))
	registry.RegisterHandler("destination.brief", DestinationBriefHandler(deps))

	// --- Agent data tools Phase 2 (spec 2026-09-24-agent-data-api-phase2) ---
	// feeds.explore routes internally to the explore/tag/space feed reads;
	// feeds.for_you is caller-scoped. The registered names match the MCP
	// apiKeyToolGates allowlist exactly.
	registry.RegisterHandler("feeds.explore", FeedsExploreHandler(deps))
	registry.RegisterHandler("feeds.for_you", FeedsForYouHandler(deps))
	registry.RegisterHandler("scam.list", ScamListHandler(deps))
	registry.RegisterHandler("scam.get", ScamGetHandler(deps))
	registry.RegisterHandler("search.semantic", SemanticSearchHandler(deps))

	// --- Community discussion tools (§08 forum, §49 comments) ---
	// Author-vs-moderator semantics are enforced inside the forum/comments
	// services (edit_own vs edit_any); the catalog RBAC gate enforces each
	// tool's required_permission first.
	registry.RegisterHandler("forum.threads.list", ForumThreadsListHandler(deps))
	registry.RegisterHandler("forum.threads.read", ForumThreadsReadHandler(deps))
	registry.RegisterHandler("forum.threads.create", ForumThreadsCreateHandler(deps))
	registry.RegisterHandler("forum.threads.update", ForumThreadsUpdateHandler(deps))
	registry.RegisterHandler("forum.threads.delete", ForumThreadsDeleteHandler(deps))
	registry.RegisterHandler("forum.threads.solve", ForumThreadsSolveHandler(deps))
	registry.RegisterHandler("forum.replies.list", ForumRepliesListHandler(deps))
	registry.RegisterHandler("forum.replies.create", ForumRepliesCreateHandler(deps))
	registry.RegisterHandler("forum.replies.update", ForumRepliesUpdateHandler(deps))
	registry.RegisterHandler("forum.replies.delete", ForumRepliesDeleteHandler(deps))
	registry.RegisterHandler("forum.vote", ForumVoteHandler(deps))
	registry.RegisterHandler("comments.list", CommentsListHandler(deps))
	registry.RegisterHandler("comments.create", CommentsCreateHandler(deps))
	registry.RegisterHandler("comments.update", CommentsUpdateHandler(deps))
	registry.RegisterHandler("comments.delete", CommentsDeleteHandler(deps))
	registry.RegisterHandler("comments.vote", CommentsVoteHandler(deps))

	// --- Deterministic calculator tools (§107, §108, §112) ---
	registry.RegisterHandler("crs.calculate", CRSCalculateHandler(deps))
	registry.RegisterHandler("pof.calculate", POFCalculateHandler(deps))
	registry.RegisterHandler("fx.convert", FXConvertHandler(deps))

	// --- Destination wiki tools (§80 revision queue) ---
	// wiki_page.create is registered again here, overriding the generic
	// writer's shell create: payloads carrying `content` route through
	// CreatePageWithRevision so the page lands with an initial pending
	// revision; content-less payloads keep the provisioning-shell path.
	// submit_revision/review_revision cover the body-edit and moderator
	// approve/reject steps the generic ops cannot express.
	registry.RegisterHandler("wiki_page.create", WikiPageCreateHandler(deps))
	registry.RegisterHandler("wiki_page.submit_revision", WikiPageSubmitRevisionHandler(deps))
	registry.RegisterHandler("wiki_page.review_revision", WikiPageReviewRevisionHandler(deps))
}
