-- 000879_agent_spec_search_tool.up.sql — register the spec.search agent
-- tool (§121, ADR-131). Staff-only privileged retrieval over the .jeos
-- engineering-spec corpus (spec_doc chunks, visibility='private') via
-- rag.Service.RetrievePrivileged. Gated by the agent.spec_search node
-- seeded in migration 000874; the rag.spec_corpus feature flag additionally
-- gates the corpus itself.
--
-- Ref: .jeos/sections/121-engineering-spec-corpus.md
--      .jeos/sections/103-ai-agent-executor.md (Knowledge tools)

BEGIN;

INSERT INTO ai_tools (name, description, category, config, is_enabled, required_permission) VALUES ('spec.search', 'Privileged vector search over the engineering spec corpus (indexed .jeos spec sections, ADRs, conventions, roadmaps). Use to ground documentation authoring and engineering answers in the actual spec. Optional path_prefix narrows results to a subtree (e.g. "sections/", "adr/"). Staff-only.', 'agent_knowledge', '{"function_name":"spec.search","description":"Privileged vector search over the engineering spec corpus (indexed .jeos spec sections, ADRs, conventions, roadmaps). Use to ground documentation authoring and engineering answers in the actual spec. Optional path_prefix narrows results to a subtree.","handler":"spec.search","parameters":{"type":"object","required":["query"],"properties":{"query":{"type":"string","description":"Search query"},"path_prefix":{"type":"string","description":"Optional corpus path prefix filter (e.g. sections/, adr/, conventions/)"},"limit":{"type":"integer","description":"Max results"}},"additionalProperties":true},"writes_user_data":false,"reads_user_data":false,"requires_hitl":false,"is_blocked":false}'::jsonb, true, 'agent.spec_search') ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description, category = EXCLUDED.category, config = EXCLUDED.config, is_enabled = EXCLUDED.is_enabled, required_permission = EXCLUDED.required_permission;

COMMIT;
