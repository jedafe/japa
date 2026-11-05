-- Seed activity rules for keys emitted but never given a payout — these
-- events settled with zero points (follow.created was observed firing in
-- production; tool.comparison_tool.used matches the tool.* emission family
-- and had no rule unlike its siblings). Values mirror peer rules; admins
-- can retune in Admin → Wallet.
INSERT INTO activity_point_rules (activity_key, point_value, is_enabled, cap_per_day, description, one_time)
VALUES
    ('follow.created',             2, true, 20, 'Points for connecting with a member',            false),
    ('tool.comparison_tool.used',  1, true, 15, 'Points for running a comparison',                 false)
ON CONFLICT (activity_key) DO NOTHING;
