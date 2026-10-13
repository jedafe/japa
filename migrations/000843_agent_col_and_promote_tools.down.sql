-- 000843_agent_col_and_promote_tools.down.sql
--
-- Remove the col.city.get and comparison.promote agent tool rows.

DELETE FROM public.ai_tools
WHERE name IN ('col.city.get', 'comparison.promote');
