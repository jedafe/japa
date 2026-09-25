-- Down migration for 000015_websockets_dm.sql
-- Drops DM messages, message requests, conversations, WebSocket tickets, and user DM settings tables.
DROP TABLE IF EXISTS dm_messages;
DROP TABLE IF EXISTS dm_message_requests;
DROP TABLE IF EXISTS dm_conversations;
DROP TABLE IF EXISTS ws_tickets;
DROP TABLE IF EXISTS user_dm_settings;
