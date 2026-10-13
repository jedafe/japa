-- 000841_sidebar_route_family_seeds.up.sql
--
-- Sidebar audit fix W18 (Section 59, ADR-066): seed route-family right
-- sidebars so forum, story/stories, calendar, jobs, wiki, universities and
-- services resolve a specific sidebar instead of the generic
-- main-default-right fallback (kept as the catch-all for everything else).
--
-- Widget sets follow the main-guide-right template: topbar chrome (search,
-- theme_toggle, user_dropdown, notification_centre where authenticated-only
-- makes sense) plus content widgets (ad slots, trending_tags, and one
-- context widget per family: related_discussions on forum/story,
-- upcoming_events on calendar, space_categories on wiki).
--
-- dashboard is seeded INACTIVE on purpose (ADR-060: dashboard is private, no
-- ad slots on /dashboard/*) — the row documents the exemption while resolve
-- keeps falling through to the generic fallback.
--
-- '/stor*' covers /story/* and /stories/* (/studio/* also matches the glob
-- but studio skips the resolve call entirely, so it never renders).

BEGIN;

INSERT INTO public.sidebars (id, app, position, slug, name, page_pattern, visibility, required_group_ids, status)
VALUES
    ('593a51e4-a51f-4933-906f-ed578c3067e9', 'main', 'right', 'main-forum-right', 'Forum Right', '/forum/*', 'all', '[]', 'active'),
    ('fba75a0b-fe6b-4dd5-bfe4-6f4ee5c2e53d', 'main', 'right', 'main-story-right', 'Story Right', '/stor*', 'all', '[]', 'active'),
    ('83c55b58-638f-46c9-8629-e9b0d582c5e0', 'main', 'right', 'main-dashboard-right', 'Dashboard Right', '/dashboard/*', 'all', '[]', 'inactive'),
    ('d0b84244-564e-49e6-8ccf-d58db0731829', 'main', 'right', 'main-calendar-right', 'Calendar Right', '/calendar/*', 'all', '[]', 'active'),
    ('3115e848-e88d-41c9-968c-addc79f0f9d4', 'main', 'right', 'main-jobs-right', 'Jobs Right', '/jobs/*', 'all', '[]', 'active'),
    ('7758383e-52e4-4047-a8f0-4317bb534b22', 'main', 'right', 'main-wiki-right', 'Wiki Right', '/wiki/*', 'all', '[]', 'active'),
    ('73e8b367-eeee-4945-8c7a-16a0fab5a592', 'main', 'right', 'main-universities-right', 'Universities Right', '/universities/*', 'all', '[]', 'active'),
    ('3b0c4d1c-b5fd-4ed8-819a-b69cecce442d', 'main', 'right', 'main-services-right', 'Services Right', '/services/*', 'all', '[]', 'active');

INSERT INTO public.sidebar_widgets (id, sidebar_id, widget_type, label, props, sort_order, visibility, required_group_ids, status)
VALUES
    -- main-forum-right
    ('d38cef82-6ab6-4b89-857c-145d2ca800c9', '593a51e4-a51f-4933-906f-ed578c3067e9', 'search', 'Search', '{}', 0, 'all', '[]', 'active'),
    ('0b558661-8ea8-4f42-822e-8b4bc66a5488', '593a51e4-a51f-4933-906f-ed578c3067e9', 'theme_toggle', 'Theme Toggle', '{}', 1, 'all', '[]', 'active'),
    ('130b0420-a4b2-4561-b447-b977d6b73a44', '593a51e4-a51f-4933-906f-ed578c3067e9', 'user_dropdown', 'User Dropdown', '{}', 2, 'all', '[]', 'active'),
    ('65e4e244-e3cb-4f9b-95a4-5552381ede2f', '593a51e4-a51f-4933-906f-ed578c3067e9', 'notification_centre', 'Notifications', '{}', 3, 'authenticated', '[]', 'active'),
    ('6424c4be-2240-4423-a624-6192a01938d4', '593a51e4-a51f-4933-906f-ed578c3067e9', 'ad_slot', 'Ad Top', '{"slot": "sidebar_top", "format": "square"}', 5, 'all', '[]', 'active'),
    ('bfbefe7a-1178-4632-bf05-97ed434a1bad', '593a51e4-a51f-4933-906f-ed578c3067e9', 'related_discussions', 'Related Discussions', '{"limit": 5, "title": "Discussions"}', 6, 'all', '[]', 'active'),
    ('fc486228-a8fc-4115-a60c-a92e2467ae34', '593a51e4-a51f-4933-906f-ed578c3067e9', 'trending_tags', 'Trending Tags', '{"limit": 8, "title": "Trending Tags"}', 7, 'all', '[]', 'active'),
    ('fce4ecf3-98c0-4643-860a-2fcd48fb28e1', '593a51e4-a51f-4933-906f-ed578c3067e9', 'ad_slot', 'Ad Bottom', '{"slot": "sidebar_bottom", "format": "banner"}', 8, 'all', '[]', 'active'),
    -- main-story-right
    ('a9ba034b-2b81-4e4f-819c-426fd404e428', 'fba75a0b-fe6b-4dd5-bfe4-6f4ee5c2e53d', 'search', 'Search', '{}', 0, 'all', '[]', 'active'),
    ('b64d8bc0-bc96-49c0-b507-30b68d828201', 'fba75a0b-fe6b-4dd5-bfe4-6f4ee5c2e53d', 'theme_toggle', 'Theme Toggle', '{}', 1, 'all', '[]', 'active'),
    ('468559d5-ac0d-4260-9aa5-e3441cd58cb5', 'fba75a0b-fe6b-4dd5-bfe4-6f4ee5c2e53d', 'user_dropdown', 'User Dropdown', '{}', 2, 'all', '[]', 'active'),
    ('5185d301-8dbf-4210-a26f-658304561ec3', 'fba75a0b-fe6b-4dd5-bfe4-6f4ee5c2e53d', 'ad_slot', 'Ad Top', '{"slot": "sidebar_top", "format": "square"}', 5, 'all', '[]', 'active'),
    ('a6926465-71c1-4441-9212-e118dd442da5', 'fba75a0b-fe6b-4dd5-bfe4-6f4ee5c2e53d', 'user_card', 'Author', '{"title": "Author", "user_id": ""}', 6, 'all', '[]', 'active'),
    ('f55faad2-d813-4186-8e9a-ebd3c0ff487e', 'fba75a0b-fe6b-4dd5-bfe4-6f4ee5c2e53d', 'related_discussions', 'Discussions', '{"limit": 5, "title": "Discussions"}', 7, 'all', '[]', 'active'),
    ('a60f4807-1664-46c9-a2c5-404638798ef6', 'fba75a0b-fe6b-4dd5-bfe4-6f4ee5c2e53d', 'trending_tags', 'Trending Tags', '{"limit": 8, "title": "Trending Tags"}', 8, 'all', '[]', 'active'),
    ('3431b917-47b2-4d80-ab44-e398882e11a9', 'fba75a0b-fe6b-4dd5-bfe4-6f4ee5c2e53d', 'ad_slot', 'Ad Bottom', '{"slot": "sidebar_bottom", "format": "banner"}', 9, 'all', '[]', 'active'),
    -- main-dashboard-right (inactive: ADR-060, no ads on dashboard)
    ('e2278bcd-cd5b-41bb-aff9-578f7100e3c1', '83c55b58-638f-46c9-8629-e9b0d582c5e0', 'search', 'Search', '{}', 0, 'all', '[]', 'active'),
    ('d047c3a4-86df-4f65-a781-3c9ebcbd4dfb', '83c55b58-638f-46c9-8629-e9b0d582c5e0', 'theme_toggle', 'Theme Toggle', '{}', 1, 'all', '[]', 'active'),
    ('ed85f519-9e49-427b-95ba-3e08c4e540ba', '83c55b58-638f-46c9-8629-e9b0d582c5e0', 'user_dropdown', 'User Dropdown', '{}', 2, 'all', '[]', 'active'),
    ('cae8bfbf-a04e-4a7b-b6b1-fea118ef6706', '83c55b58-638f-46c9-8629-e9b0d582c5e0', 'bookmark_folders', 'Bookmarks', '{"title": "Bookmarks"}', 5, 'authenticated', '[]', 'active'),
    -- main-calendar-right
    ('35f1f688-fa6f-4f03-9bed-e01f1ad2ca58', 'd0b84244-564e-49e6-8ccf-d58db0731829', 'search', 'Search', '{}', 0, 'all', '[]', 'active'),
    ('218c36cc-bef6-4540-ab75-1d70c9e6d034', 'd0b84244-564e-49e6-8ccf-d58db0731829', 'theme_toggle', 'Theme Toggle', '{}', 1, 'all', '[]', 'active'),
    ('e0c557b2-a4b0-4932-bff3-9f98edea6f65', 'd0b84244-564e-49e6-8ccf-d58db0731829', 'user_dropdown', 'User Dropdown', '{}', 2, 'all', '[]', 'active'),
    ('f80d2ea5-c79d-4d69-afa5-ac6f069dfd53', 'd0b84244-564e-49e6-8ccf-d58db0731829', 'upcoming_events', 'Upcoming Events', '{"limit": 3, "title": "Upcoming Events"}', 5, 'all', '[]', 'active'),
    ('b665e79e-6d7f-4a2b-acde-cc4b4e5703e3', 'd0b84244-564e-49e6-8ccf-d58db0731829', 'ad_slot', 'Ad Top', '{"slot": "sidebar_top", "format": "square"}', 6, 'all', '[]', 'active'),
    ('7600d75c-0106-4ec4-9727-c5c35d0f3646', 'd0b84244-564e-49e6-8ccf-d58db0731829', 'ad_slot', 'Ad Bottom', '{"slot": "sidebar_bottom", "format": "banner"}', 7, 'all', '[]', 'active'),
    -- main-jobs-right
    ('71aa5ace-313b-4a13-91f0-222d2bb6af1a', '3115e848-e88d-41c9-968c-addc79f0f9d4', 'search', 'Search', '{}', 0, 'all', '[]', 'active'),
    ('40528cb2-50ac-430a-b62e-b9bed8c565f3', '3115e848-e88d-41c9-968c-addc79f0f9d4', 'theme_toggle', 'Theme Toggle', '{}', 1, 'all', '[]', 'active'),
    ('fd0d0042-31fb-4c62-a410-bc9e37172898', '3115e848-e88d-41c9-968c-addc79f0f9d4', 'user_dropdown', 'User Dropdown', '{}', 2, 'all', '[]', 'active'),
    ('9854597d-a437-4bbf-b806-ce6c51a0b2bf', '3115e848-e88d-41c9-968c-addc79f0f9d4', 'ad_slot', 'Ad Top', '{"slot": "sidebar_top", "format": "square"}', 5, 'all', '[]', 'active'),
    ('cb3849be-f4f7-40f3-914e-8863b04f2e0f', '3115e848-e88d-41c9-968c-addc79f0f9d4', 'trending_tags', 'Trending Tags', '{"limit": 8, "title": "Trending Tags"}', 6, 'all', '[]', 'active'),
    ('1b0c3098-73e0-40b7-9f00-d84119b22f05', '3115e848-e88d-41c9-968c-addc79f0f9d4', 'ad_slot', 'Ad Bottom', '{"slot": "sidebar_bottom", "format": "banner"}', 7, 'all', '[]', 'active'),
    -- main-wiki-right
    ('b6ded70c-ef1b-45f8-94e2-d313414fc0bc', '7758383e-52e4-4047-a8f0-4317bb534b22', 'search', 'Search', '{}', 0, 'all', '[]', 'active'),
    ('af4e026b-93ff-4bce-a635-a744879fdd2b', '7758383e-52e4-4047-a8f0-4317bb534b22', 'theme_toggle', 'Theme Toggle', '{}', 1, 'all', '[]', 'active'),
    ('67ebaea2-1c3c-4b5c-af1a-80f3bf53d637', '7758383e-52e4-4047-a8f0-4317bb534b22', 'user_dropdown', 'User Dropdown', '{}', 2, 'all', '[]', 'active'),
    ('bcc86d44-2a28-4d8a-a7f8-12ee92457307', '7758383e-52e4-4047-a8f0-4317bb534b22', 'space_categories', 'Categories', '{"title": "Categories"}', 5, 'all', '[]', 'active'),
    ('fce3dc3e-cf79-494f-bf0d-bab8f3f13d5c', '7758383e-52e4-4047-a8f0-4317bb534b22', 'ad_slot', 'Ad Top', '{"slot": "sidebar_top", "format": "square"}', 6, 'all', '[]', 'active'),
    ('db50151d-c304-45b9-aa1f-6fbfc5396012', '7758383e-52e4-4047-a8f0-4317bb534b22', 'ad_slot', 'Ad Bottom', '{"slot": "sidebar_bottom", "format": "banner"}', 7, 'all', '[]', 'active'),
    -- main-universities-right
    ('c213b8a3-9c23-4cca-b2df-016b9fc40d5f', '73e8b367-eeee-4945-8c7a-16a0fab5a592', 'search', 'Search', '{}', 0, 'all', '[]', 'active'),
    ('42a9b9e8-2251-4c4e-a13c-09616b3b57f4', '73e8b367-eeee-4945-8c7a-16a0fab5a592', 'theme_toggle', 'Theme Toggle', '{}', 1, 'all', '[]', 'active'),
    ('de40f9be-6c75-410a-8ef5-a47fa6893d98', '73e8b367-eeee-4945-8c7a-16a0fab5a592', 'user_dropdown', 'User Dropdown', '{}', 2, 'all', '[]', 'active'),
    ('c8ee21c0-d032-47bb-b96c-24853ea27768', '73e8b367-eeee-4945-8c7a-16a0fab5a592', 'space_list', 'Browse Spaces', '{"limit": 5, "level": "", "title": "Browse Spaces"}', 5, 'all', '[]', 'active'),
    ('4a4c4de1-69a4-46fc-a271-a769368257ea', '73e8b367-eeee-4945-8c7a-16a0fab5a592', 'ad_slot', 'Ad Top', '{"slot": "sidebar_top", "format": "square"}', 6, 'all', '[]', 'active'),
    ('be836d31-18e4-482d-8c36-05c7e587839f', '73e8b367-eeee-4945-8c7a-16a0fab5a592', 'trending_tags', 'Trending Tags', '{"limit": 8, "title": "Trending Tags"}', 7, 'all', '[]', 'active'),
    ('03ef27f9-ecad-47a6-9b65-56cb35be7e7f', '73e8b367-eeee-4945-8c7a-16a0fab5a592', 'ad_slot', 'Ad Bottom', '{"slot": "sidebar_bottom", "format": "banner"}', 8, 'all', '[]', 'active'),
    -- main-services-right
    ('56f82998-f143-455f-b5d3-b5e3c0038685', '3b0c4d1c-b5fd-4ed8-819a-b69cecce442d', 'search', 'Search', '{}', 0, 'all', '[]', 'active'),
    ('b71bff01-1aac-4ddd-9691-2a28ca3327fa', '3b0c4d1c-b5fd-4ed8-819a-b69cecce442d', 'theme_toggle', 'Theme Toggle', '{}', 1, 'all', '[]', 'active'),
    ('1bdde237-2eeb-4421-b411-715f04b07a54', '3b0c4d1c-b5fd-4ed8-819a-b69cecce442d', 'user_dropdown', 'User Dropdown', '{}', 2, 'all', '[]', 'active'),
    ('2d4ae6a0-6469-4b69-a753-7d96225da52c', '3b0c4d1c-b5fd-4ed8-819a-b69cecce442d', 'ad_slot', 'Ad Top', '{"slot": "sidebar_top", "format": "square"}', 5, 'all', '[]', 'active'),
    ('eb53f3ab-db0a-44d9-800e-c374097203c5', '3b0c4d1c-b5fd-4ed8-819a-b69cecce442d', 'leaderboard', 'Top Contributors', '{"limit": 5, "title": "Top Contributors"}', 6, 'all', '[]', 'active'),
    ('c571be12-58a6-40bc-b375-77e7299d713f', '3b0c4d1c-b5fd-4ed8-819a-b69cecce442d', 'ad_slot', 'Ad Bottom', '{"slot": "sidebar_bottom", "format": "banner"}', 7, 'all', '[]', 'active');

COMMIT;
