-- 000514 — Reverses 000514_update_comparison_fill_values_citations.up.sql.

UPDATE ai_tools
SET description = 'Fill comparison values for items x dimensions. Each value records its source (custom_field, rag, external, or llm_knowledge), source_ref, confidence, and retrieved_at timestamp in the comparison_values metadata JSONB for provenance tracking.',
    config = jsonb_set(
        jsonb_set(
            config,
            '{description}',
            '"Fill comparison values with provenance metadata."'
        ),
        '{parameters,properties,values,items,properties,source_ref,description}',
        '"Source reference (URL, content UUID, or field value UUID)"'
    ),
    updated_at = now()
WHERE name = 'comparison.fill_values';
