-- Down migration for 000028_bot_crawler_management.sql
-- Drops the known_bots table (seed data is removed with the table).
DROP TABLE IF EXISTS known_bots;
