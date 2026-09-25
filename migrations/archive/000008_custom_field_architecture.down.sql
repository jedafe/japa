-- Down migration for 000008_custom_field_architecture.sql
-- Drops custom field and form tables in reverse creation order.
-- Seeded system field groups, forms, fields, and assignments are removed with the tables.
DROP TABLE IF EXISTS form_submission_triggers;
DROP TABLE IF EXISTS form_submissions;
DROP TABLE IF EXISTS form_steps;
DROP TABLE IF EXISTS forms;
DROP TABLE IF EXISTS custom_field_values;
DROP TABLE IF EXISTS custom_field_permissions;
DROP TABLE IF EXISTS custom_field_group_assignments;
DROP TABLE IF EXISTS custom_fields;
DROP TABLE IF EXISTS custom_field_groups;
