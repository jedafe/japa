-- Down migration for 000039_learning.sql
-- Drops learning platform tables: certificates, progress, lessons, modules, tracks.
DROP TABLE IF EXISTS learning_certificates;
DROP TABLE IF EXISTS lesson_progress;
DROP TABLE IF EXISTS learning_lessons;
DROP TABLE IF EXISTS learning_modules;
DROP TABLE IF EXISTS learning_tracks;
