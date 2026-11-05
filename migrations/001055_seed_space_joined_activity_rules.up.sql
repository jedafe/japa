-- Round 2 of unruled emitted keys — space.joined fired 10× with no payout;
-- tool.community_locator.used joins the tool.* family.
INSERT INTO activity_point_rules (activity_key, point_value, is_enabled, cap_per_day, description, one_time)
VALUES
    ('space.joined',               2, true, 10, 'Points for joining a community space',           false),
    ('tool.community_locator.used', 1, true, 15, 'Points for exploring the community locator',    false)
ON CONFLICT (activity_key) DO NOTHING;
