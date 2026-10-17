// Ref: japaconnect-engineering/sections/16-page-builder.md (explore_map)
// Pure child-layout helpers for the explore_map parent/children split —
// kept in a plain .ts module (no svelte/lucide imports) so they stay
// unit-testable outside a component render context.
import type { Block, ExploreMapProps } from '../../types';

const ASIDE_LEFT = new Set(['explore_map_legend', 'explore_map_region_filter', 'explore_map_stats']);
const ASIDE_BOTTOM = new Set(['explore_map_legend', 'explore_map_region_filter', 'explore_map_col_index']);

/** Whether a child of `type` currently renders anything, given the parent's
 *  merged props. Children self-gate on this; the parent also uses it to keep
 *  gated-off children from mounting an empty BlockShell. */
export function exploreMapChildVisible(P: ExploreMapProps, type: string): boolean {
	switch (type) {
		case 'explore_map_toolbar': return !!P.show_toolbar;
		case 'explore_map_legend': return !!P.show_legend && (P.categories?.length ?? 0) > 0;
		case 'explore_map_region_filter': return !!P.show_region_filter;
		// Bottom mode floats stats inside the canvas card instead.
		case 'explore_map_stats': return !!P.show_stats && (P.stats?.length ?? 0) > 0 && P.aside_position !== 'bottom';
		case 'explore_map_col_index': return !!P.show_col_index && (P.col_bands?.length ?? 0) > 0;
		case 'explore_map_canvas': return true;
		default: return true; // unknown nested types always render
	}
}

/** Split children into the aside vs main column by type (like guest_hero's rightTypes). */
export function partitionExploreMapChildren(children: Block[], bottom: boolean): { aside: Block[]; main: Block[] } {
	const set = bottom ? ASIDE_BOTTOM : ASIDE_LEFT;
	return {
		aside: children.filter((b) => set.has(b.type)),
		main: children.filter((b) => !set.has(b.type)),
	};
}

/** Seeded children for a block with no stored children — reproduces the standard layout. */
export function defaultExploreMapChildren(id: string): Block[] {
	const mk = (type: string, suffix: string) =>
		({ id: `${id}-${suffix}`, type, visible: true, props: {} }) as Block;
	return [
		mk('explore_map_toolbar', 'toolbar'),
		mk('explore_map_legend', 'legend'),
		mk('explore_map_region_filter', 'region'),
		mk('explore_map_stats', 'stats'),
		mk('explore_map_canvas', 'canvas'),
		mk('explore_map_col_index', 'colindex'),
	];
}
