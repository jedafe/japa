-- 000724_user_groups_allow_search_indexing.up.sql
-- Section 26 (SEO/Performance): search indexing is a per-UserGroup setting.
-- The users sitemap (ServeUsersSitemap / latestUsersLastMod) already gates on
-- COALESCE(ug.allow_search_indexing, true); this column makes that gate real.
-- Default true preserves current behaviour for existing groups.

ALTER TABLE user_groups
    ADD COLUMN IF NOT EXISTS allow_search_indexing boolean NOT NULL DEFAULT true;
