-- Down migration for 000020_comparison_system.sql
-- Drops comparison values, dimensions, items, type access, and type definitions tables.
DROP TABLE IF EXISTS comparison_values;
DROP TABLE IF EXISTS comparison_dimensions;
DROP TABLE IF EXISTS comparison_items;
DROP TABLE IF EXISTS comparison_type_access;
DROP TABLE IF EXISTS comparison_type_definitions;
