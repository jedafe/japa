-- Field-permission lookups by user group (ListVisibleFieldIDsForUser,
-- ListVisibleFieldIDsForGroupSlug, ListEditableFieldIDsForUser) join
-- custom_field_permissions on user_group_id — the PK is (field_id,
-- user_group_id) so user_group_id alone had no index and every anonymous
-- wiki page render seq-scanned the table resolving the 'guest' group.
CREATE INDEX IF NOT EXISTS idx_custom_field_permissions_group
    ON custom_field_permissions (user_group_id);
