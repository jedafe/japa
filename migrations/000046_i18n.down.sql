-- Down migration for 000046_i18n.sql
-- Drops the translations table used for the i18n translation system.
DROP TABLE IF EXISTS translations;
