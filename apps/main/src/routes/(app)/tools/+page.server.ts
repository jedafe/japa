import type { PageServerLoad } from './$types';

/**
 * Tools hub landing page at /tools — the data-driven card grid for every
 * identity and functional tool (§67, §69–74, §§90–97, §§99–101).
 *
 * The tool list and per-tool enabled/disabled filtering live in
 * `$lib/tools/registry.ts` and read `data.siteSettings` (provided by the root
 * layout) — disabled tools are hidden from the hub. Each tool's own page
 * additionally surfaces a disabled state when its backend handler returns 404.
 *
 * Ref: §78 (Tools Web Surface), conventions/system-settings-catalog.md.
 */
export const load: PageServerLoad = async () => {
	return {};
};
