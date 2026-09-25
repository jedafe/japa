-- Down migration for 000054_ai_tools.sql
-- Drops AI tool usage tracking and AI tool registry tables.
DROP TABLE IF EXISTS ai_tool_usage;
DROP TABLE IF EXISTS ai_tools;
