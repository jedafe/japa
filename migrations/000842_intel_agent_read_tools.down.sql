-- 000842_intel_agent_read_tools.down.sql
--
-- Remove the three read-only intelligence agent tools. The reused
-- permission nodes (intel.sources.view, intel.review.view) are left in
-- place — they predate this migration and gate other surfaces.

DELETE FROM public.ai_tools
WHERE name IN ('intel.source.get', 'intel.record.get', 'intel.record.list');
