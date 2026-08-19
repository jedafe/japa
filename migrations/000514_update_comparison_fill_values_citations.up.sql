-- 000514 — Update comparison.fill_values tool description to guide citation use.
--
-- Perplexity-backed external.search now returns a citations array. This
-- migration updates the comparison.fill_values tool description and schema
-- so the agent knows to put the most relevant citation URL in source_ref
-- when source is 'external'.
--
-- Ref: internal/ai/tools_knowledge.go, internal/ai/tools_comparison.go

UPDATE ai_tools
SET description = 'Fill comparison values for items x dimensions. Each value records its source (custom_field, rag, external, or llm_knowledge), source_ref, confidence, and retrieved_at timestamp. When source is "external", set source_ref to the most relevant URL from the external.search result citations array.',
    config = jsonb_set(
        jsonb_set(
            config,
            '{description}',
            '"Fill comparison values with provenance metadata. When source is external, use the most relevant URL from the external.search result citations array as source_ref."'
        ),
        '{parameters,properties,values,items,properties,source_ref,description}',
        '"Source reference: URL from external.search citations, content UUID for rag, or custom_field_values UUID"'
    ),
    updated_at = now()
WHERE name = 'comparison.fill_values';
