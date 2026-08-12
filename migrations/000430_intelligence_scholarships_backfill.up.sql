-- 000430_intelligence_scholarships_backfill.up.sql
-- One-time backfill: existing scholarship guide content → scholarships rows.
-- override_source='editorial' (treat existing data as human-curated).
-- intel_record_id is NULL (these predate the ingestion system).
--
-- custom_field_values uses field_id → custom_fields(id) where the key lives.
-- contents uses guide_category = 'scholarship' (migration 000225) and content_type = 'guide'.
-- contents has no summary column; description is derived from LEFT(body, 500).
-- Uses correlated subqueries to avoid cartesian products on custom_field_values.

BEGIN;

INSERT INTO scholarships (
  intel_record_id, guide_id, source_url, source_type, trust_level,
  title, description, country, country_space_id, university,
  degree_level, fields_of_study, eligible_countries, funding,
  deadline, application_url, required_documents, minimum_gpa,
  language_requirements, confidence_overall, verification_status,
  freshness_score, override_source, editorial_overrides
)
SELECT
  NULL,
  c.id,
  COALESCE(c.canonical_url, '/content/' || c.id::text),
  'community',
  4,
  c.title,
  LEFT(c.body, 500),
  COALESCE(
    (SELECT cfv.value_text FROM custom_field_values cfv
     JOIN custom_fields cf ON cf.id = cfv.field_id
     WHERE cfv.entity_id = c.id AND cfv.entity_type = 'content' AND cf.key = 'country'
     LIMIT 1),
    'UNKNOWN'
  ),
  NULL,
  (SELECT cfv.value_text FROM custom_field_values cfv
   JOIN custom_fields cf ON cf.id = cfv.field_id
   WHERE cfv.entity_id = c.id AND cfv.entity_type = 'content' AND cf.key = 'provider_institution'
   LIMIT 1),
  (SELECT cfv.value_text FROM custom_field_values cfv
   JOIN custom_fields cf ON cf.id = cfv.field_id
   WHERE cfv.entity_id = c.id AND cfv.entity_type = 'content' AND cf.key = 'level_of_study'
   LIMIT 1),
  COALESCE(string_to_array(
    (SELECT cfv.value_text FROM custom_field_values cfv
     JOIN custom_fields cf ON cf.id = cfv.field_id
     WHERE cfv.entity_id = c.id AND cfv.entity_type = 'content' AND cf.key = 'field_of_study'
     LIMIT 1), ','), '{}'),
  COALESCE(string_to_array(
    (SELECT cfv.value_text FROM custom_field_values cfv
     JOIN custom_fields cf ON cf.id = cfv.field_id
     WHERE cfv.entity_id = c.id AND cfv.entity_type = 'content' AND cf.key = 'countries_eligible'
     LIMIT 1), ','), '{}'),
  COALESCE(
    (SELECT cfv.value_json FROM custom_field_values cfv
     JOIN custom_fields cf ON cf.id = cfv.field_id
     WHERE cfv.entity_id = c.id AND cfv.entity_type = 'content' AND cf.key = 'coverage'
     LIMIT 1),
    '{}'::jsonb),
  (SELECT cfv.value_date::date FROM custom_field_values cfv
   JOIN custom_fields cf ON cf.id = cfv.field_id
   WHERE cfv.entity_id = c.id AND cfv.entity_type = 'content' AND cf.key = 'application_deadline'
   LIMIT 1),
  (SELECT cfv.value_text FROM custom_field_values cfv
   JOIN custom_fields cf ON cf.id = cfv.field_id
   WHERE cfv.entity_id = c.id AND cfv.entity_type = 'content' AND cf.key = 'application_link'
   LIMIT 1),
  COALESCE(string_to_array(
    (SELECT cfv.value_text FROM custom_field_values cfv
     JOIN custom_fields cf ON cf.id = cfv.field_id
     WHERE cfv.entity_id = c.id AND cfv.entity_type = 'content' AND cf.key = 'required_documents'
     LIMIT 1), ','), '{}'),
  (SELECT cfv.value_number::numeric(3,2) FROM custom_field_values cfv
   JOIN custom_fields cf ON cf.id = cfv.field_id
   WHERE cfv.entity_id = c.id AND cfv.entity_type = 'content' AND cf.key = 'minimum_gpa'
   LIMIT 1),
  COALESCE(
    (SELECT cfv.value_json FROM custom_field_values cfv
     JOIN custom_fields cf ON cf.id = cfv.field_id
     WHERE cfv.entity_id = c.id AND cfv.entity_type = 'content' AND cf.key = 'language_requirements'
     LIMIT 1),
    '{}'::jsonb),
  0.0,
  'needs_review',
  70,
  'editorial',
  '{}'::jsonb
FROM contents c
WHERE c.content_type = 'guide'
  AND c.guide_category = 'scholarship'
  AND c.deleted_at IS NULL
  AND c.status = 'published'
  AND NOT EXISTS (
    SELECT 1 FROM scholarships s WHERE s.guide_id = c.id
  );

COMMIT;
