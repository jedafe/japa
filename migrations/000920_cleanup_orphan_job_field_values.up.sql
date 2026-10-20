-- Remove custom_field_values rows that the pre-fix jobs upsert wrote to
-- same-keyed fields in OTHER field groups. custom_fields.key is not unique
-- (application_deadline exists in scholarship/grant/admission/wiki groups),
-- and the old code inserted one cfv row per same-keyed field — e.g. a job's
-- deadline landed on guide_scholarship_fields.application_deadline too.
-- Keep only rows whose field belongs to job_listing_fields; the canonical
-- group's row always carried the real value.
--
-- Ref: internal/jobs/service.go upsertJobFieldValue (group-scoped fix)
DELETE FROM custom_field_values cfv
USING custom_fields cf, custom_field_groups g, contents c
WHERE cf.id = cfv.field_id
  AND g.id = cf.group_id
  AND c.id = cfv.entity_id
  AND cfv.entity_type = 'content'
  AND c.content_type = 'job_listing'
  AND g.slug <> 'job_listing_fields';
