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
	registry.RegisterHandler("content.update", ContentUpdateHandler(deps))
	registry.RegisterPrivacyTarget("content.update", ContentUpdatePrivacyTarget)

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
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 7, task 7.06)
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
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 9, tasks 9.01-9.08)
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

	// --- Learning tools (task 9.02) ---
	// No privacy targets — learning data has no ai_exempt column.
	registry.RegisterHandler("learn.tracks.list", LearnTracksListHandler(deps))
	registry.RegisterHandler("learn.tracks.create", LearnTracksCreateHandler(deps))
	registry.RegisterHandler("learn.tracks.update", LearnTracksUpdateHandler(deps))
	registry.RegisterHandler("learn.lessons.list", LearnLessonsListHandler(deps))
	registry.RegisterHandler("learn.lessons.create", LearnLessonsCreateHandler(deps))
	registry.RegisterHandler("learn.lessons.update", LearnLessonsUpdateHandler(deps))
	registry.RegisterHandler("learn.lessons.reorder", LearnLessonsReorderHandler(deps))

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
	// Stubs — the backing tools table does not exist yet (§78 discrepancy,
	// see tools_tools.go). Registered so the catalog is complete; seeded
	// with is_enabled = false in the migration.
	registry.RegisterHandler("tools.list", ToolsListHandler(deps))
	registry.RegisterHandler("tools.read", ToolsReadHandler(deps))
	registry.RegisterHandler("tools.create", ToolsCreateHandler(deps))
	registry.RegisterHandler("tools.update", ToolsUpdateHandler(deps))
	registry.RegisterHandler("tools.assign_geo", ToolsAssignGeoHandler(deps))

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
}
