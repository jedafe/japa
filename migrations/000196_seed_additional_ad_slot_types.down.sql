-- Remove the additional ad slot types added in 000196.
-- Only removes the slots seeded by 000196; the original 5 are preserved.
DELETE FROM ad_slot_types
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slot_name IN (
    'learn_catalog_inline',
    'learn_detail_sidebar',
    'learn_dashboard_banner',
    'org_profile_sidebar',
    'org_listing_inline',
    'professional_profile_sidebar',
    'user_profile_sidebar',
    'user_timeline_inline',
    'space_detail_sidebar',
    'space_feed_inline',
    'content_detail_sidebar',
    'blog_detail_below_content',
    'blog_listing_inline',
    'homepage_hero',
    'discover_inline',
    'explore_inline',
    'trending_inline',
    'university_listing_inline',
    'university_detail_sidebar',
    'coupons_listing_inline',
    'tag_detail_sidebar',
    'notification_feed_inline',
    'messages_inbox_sidebar'
  );
