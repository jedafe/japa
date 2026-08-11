// Package ai — tools_register.go registers the Phase 4 core tool catalog
// (Section 103, ADR-111) with a ToolHandlerRegistry.
//
// The wiring layer (internal/server) calls RegisterPhase4Tools after
// constructing a ToolDeps with the real service implementations. This
// helper keeps the server wiring concise and ensures every Phase 4 tool
// handler + privacy target is registered in one place.
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
