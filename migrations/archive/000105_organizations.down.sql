-- Down migration for 000105_organizations.sql
-- Drops ownership_claims, organization_invitations, organization_members, and organizations tables.
DROP TABLE IF EXISTS ownership_claims;
DROP TABLE IF EXISTS organization_invitations;
DROP TABLE IF EXISTS organization_members;
DROP TABLE IF EXISTS organizations;
