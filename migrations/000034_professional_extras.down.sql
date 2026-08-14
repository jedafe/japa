-- Down migration for 000034_professional_extras.sql
-- Drops professional verifiers, professional documents, and professional relocations tables.
DROP TABLE IF EXISTS professional_verifiers;
DROP TABLE IF EXISTS professional_documents;
DROP TABLE IF EXISTS professional_relocations;
