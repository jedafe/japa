-- Down migration for 000026_ticket_system.sql
-- Drops ticket events, ticket attachments, ticket comments, tickets, and ticket types tables.
DROP TABLE IF EXISTS ticket_events;
DROP TABLE IF EXISTS ticket_attachments;
DROP TABLE IF EXISTS ticket_comments;
DROP TABLE IF EXISTS tickets;
DROP TABLE IF EXISTS ticket_types;
