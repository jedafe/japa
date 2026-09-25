-- Down migration for 000010_content_type_system.sql
-- Drops content type system tables in reverse creation order.
-- Seeded default reaction types are removed when reaction_types is dropped.
DROP TABLE IF EXISTS space_pinned_posts;
DROP TABLE IF EXISTS content_type_jsonld_templates;
DROP TABLE IF EXISTS content_scores;
DROP TABLE IF EXISTS reactions;
DROP TABLE IF EXISTS reaction_types;
DROP TABLE IF EXISTS slug_history;
DROP TABLE IF EXISTS content_revisions;
DROP TABLE IF EXISTS contents;
