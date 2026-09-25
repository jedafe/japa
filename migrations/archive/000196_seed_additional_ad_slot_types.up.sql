-- Seed additional ad slot types to cover all JapaConnect surfaces.
-- The original 5 slot types (feed_inline, sidebar, search_sponsored,
-- comparison_sponsored, forum_inline) only covered the main feed, sidebar,
-- search, comparison, and forum. This adds slots for the learn platform,
-- org/profile pages, spaces, blog, discover, homepage, coupons, tags,
-- content detail, notifications, and messages.
--
-- Ref: japaconnect-engineering/sections/25-ad-placement.md
-- Ref: japaconnect-engineering/sections/36-learning-platform.md (no ads on lesson player)

INSERT INTO ad_slot_types (tenant_id, slot_name, description, max_display_per_page) VALUES
    -- Learning platform
    ('00000000-0000-0000-0000-000000000001', 'learn_catalog_inline', 'In-line ad between course cards in the learning track catalog', 2),
    ('00000000-0000-0000-0000-000000000001', 'learn_detail_sidebar', 'Sidebar ad on track detail pages', 1),
    ('00000000-0000-0000-0000-000000000001', 'learn_dashboard_banner', 'Hero banner ad on the learning dashboard', 1),
    -- Profile & organization surfaces
    ('00000000-0000-0000-0000-000000000001', 'org_profile_sidebar', 'Sidebar ad on organization profile pages', 1),
    ('00000000-0000-0000-0000-000000000001', 'org_listing_inline', 'In-line ad between organization cards on the organizations listing', 2),
    ('00000000-0000-0000-0000-000000000001', 'professional_profile_sidebar', 'Sidebar ad on professional profile pages', 1),
    ('00000000-0000-0000-0000-000000000001', 'user_profile_sidebar', 'Sidebar ad on user profile pages', 1),
    ('00000000-0000-0000-0000-000000000001', 'user_timeline_inline', 'In-line ad between posts on user timeline feed', 2),
    -- Space & community surfaces
    ('00000000-0000-0000-0000-000000000001', 'space_detail_sidebar', 'Sidebar ad on space detail pages', 1),
    ('00000000-0000-0000-0000-000000000001', 'space_feed_inline', 'In-line ad between posts in space-specific feed', 2),
    -- Content detail surfaces
    ('00000000-0000-0000-0000-000000000001', 'content_detail_sidebar', 'Sidebar ad on content detail pages', 1),
    ('00000000-0000-0000-0000-000000000001', 'blog_detail_below_content', 'Ad displayed below the article body on blog single pages', 1),
    ('00000000-0000-0000-0000-000000000001', 'blog_listing_inline', 'In-line ad between blog post cards on the blog listing page', 2),
    -- Discovery & homepage surfaces
    ('00000000-0000-0000-0000-000000000001', 'homepage_hero', 'Hero banner ad on the authenticated homepage', 1),
    ('00000000-0000-0000-0000-000000000001', 'discover_inline', 'In-line ad between cards on the Discover page', 2),
    ('00000000-0000-0000-0000-000000000001', 'explore_inline', 'In-line ad between content cards on the Explore timeline', 2),
    ('00000000-0000-0000-0000-000000000001', 'trending_inline', 'In-line ad between trending items on the Trending page', 2),
    -- University surfaces
    ('00000000-0000-0000-0000-000000000001', 'university_listing_inline', 'In-line ad between university cards on the universities listing', 2),
    ('00000000-0000-0000-0000-000000000001', 'university_detail_sidebar', 'Sidebar ad on university detail pages', 1),
    -- Coupons & affiliate surfaces
    ('00000000-0000-0000-0000-000000000001', 'coupons_listing_inline', 'In-line ad between coupon cards on the coupons listing page', 2),
    -- Tag surfaces
    ('00000000-0000-0000-0000-000000000001', 'tag_detail_sidebar', 'Sidebar ad on tag detail pages', 1),
    -- Notification & messaging surfaces
    ('00000000-0000-0000-0000-000000000001', 'notification_feed_inline', 'In-line ad between notification items in the notification center', 1),
    ('00000000-0000-0000-0000-000000000001', 'messages_inbox_sidebar', 'Sidebar ad on the messages inbox page', 1)
ON CONFLICT (tenant_id, slot_name) DO NOTHING;
