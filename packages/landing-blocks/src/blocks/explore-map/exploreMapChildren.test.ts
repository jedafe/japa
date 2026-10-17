import { describe, it, expect } from 'vitest';
import { partitionExploreMapChildren, defaultExploreMapChildren } from './exploreMapChildren';
import type { Block } from '../../types';

const kids = defaultExploreMapChildren('t');
const custom: Block = { id: 'x', type: 'cta_button', visible: true, props: {} } as Block;

describe('partitionExploreMapChildren', () => {
	it('left mode: legend/filter/stats aside; toolbar/canvas/col_index main', () => {
		const { aside, main } = partitionExploreMapChildren(kids, false);
		expect(aside.map((b) => b.type)).toEqual(['explore_map_legend', 'explore_map_region_filter', 'explore_map_stats']);
		expect(main.map((b) => b.type)).toEqual(['explore_map_toolbar', 'explore_map_canvas', 'explore_map_col_index']);
	});

	it('bottom mode: col_index joins the aside grid; stats partitions to main (renders nothing — canvas floats it)', () => {
		const { aside, main } = partitionExploreMapChildren(kids, true);
		expect(aside.map((b) => b.type)).toEqual(['explore_map_legend', 'explore_map_region_filter', 'explore_map_col_index']);
		expect(main.map((b) => b.type)).toEqual(['explore_map_toolbar', 'explore_map_stats', 'explore_map_canvas']);
	});

	it('unknown types go to main, preserving order', () => {
		const { main } = partitionExploreMapChildren([...kids, custom], false);
		expect(main.at(-1)).toBe(custom);
	});
});

describe('defaultExploreMapChildren', () => {
	it('seeds the six children with stable ids', () => {
		expect(kids.map((b) => b.id)).toEqual(['t-toolbar', 't-legend', 't-region', 't-stats', 't-canvas', 't-colindex']);
	});
});
