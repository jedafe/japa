-- 000724_user_groups_allow_search_indexing.down.sql

ALTER TABLE user_groups
    DROP COLUMN IF EXISTS allow_search_indexing;
