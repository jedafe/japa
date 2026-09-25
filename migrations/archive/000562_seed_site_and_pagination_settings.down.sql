-- Reverse the site/pagination settings seed.
DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN ('site.custom_css', 'site.custom_js', 'content.pagination.style');
