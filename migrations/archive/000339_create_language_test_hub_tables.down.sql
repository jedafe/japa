-- 000339_create_language_test_hub_tables.down.sql
BEGIN;
DROP TABLE IF EXISTS language_score_stories;
DROP TABLE IF EXISTS language_test_resources;
DROP TABLE IF EXISTS language_test_requirements;
DROP TABLE IF EXISTS language_tests;
COMMIT;
