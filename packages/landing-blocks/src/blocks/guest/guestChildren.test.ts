import { describe, it, expect } from 'vitest';
import { resolveGuestItems } from './guestChildren';
import type { Block } from '../../types';

function mkItem(id: string): Block {
	return { id, type: 'guest_category_item', visible: true, props: {} } as Block;
}

const contextItems = [
	{ id: 'a', name: 'A' },
	{ id: 'b', name: 'B' },
	{ id: 'c', name: 'C' },
];

const cloneForContext = (template: Block, item: { id: string; name: string }) =>
	({ ...template, id: `${template.id}-${item.id}`, props: { ...template.props, name: item.name } }) as Block;

describe('resolveGuestItems', () => {
	it('treats children as manual when no data_source is set', () => {
		const res = resolveGuestItems({
			children: [mkItem('x'), mkItem('y')],
			contextItems,
			cloneForContext,
		});
		expect(res.dataSource).toBe('manual');
		expect(res.manualChildren).toHaveLength(2);
		expect(res.legacyItems).toBeUndefined();
	});

	it('defaults to context when there are no children and no data_source', () => {
		const res = resolveGuestItems({ contextItems, cloneForContext });
		expect(res.dataSource).toBe('context');
		expect(res.legacyItems).toHaveLength(3);
	});

	it('renders nothing for an explicitly empty manual children array', () => {
		const res = resolveGuestItems({
			dataSource: 'manual',
			children: [],
			items: [{ id: 'i', name: 'legacy' }],
			contextItems,
			cloneForContext,
		});
		expect(res.manualChildren).toEqual([]);
		expect(res.legacyItems).toBeUndefined();
	});

	it('falls back to legacy items in manual mode when children is undefined', () => {
		const res = resolveGuestItems({
			dataSource: 'manual',
			items: [{ id: 'i', name: 'legacy' }],
			contextItems,
			cloneForContext,
		});
		expect(res.legacyItems).toEqual([{ id: 'i', name: 'legacy' }]);
	});

	it('falls back to context items in manual mode when neither children nor items exist', () => {
		const res = resolveGuestItems({
			dataSource: 'manual',
			contextItems,
			cloneForContext,
		});
		expect(res.legacyItems).toHaveLength(3);
	});

	it('clones the first child per context item in context mode', () => {
		const res = resolveGuestItems({
			dataSource: 'context',
			children: [mkItem('tpl'), mkItem('ignored')],
			contextItems,
			cloneForContext,
		});
		expect(res.contextChildren).toHaveLength(3);
		expect(res.contextChildren?.[0].id).toBe('tpl-a');
		expect(res.contextChildren?.[2].id).toBe('tpl-c');
		expect((res.contextChildren?.[1].props as { name: string }).name).toBe('B');
	});

	it('uses legacy context markup in context mode without a template child', () => {
		const res = resolveGuestItems({
			dataSource: 'context',
			children: [],
			contextItems,
			cloneForContext,
		});
		expect(res.contextChildren).toBeUndefined();
		expect(res.legacyItems).toHaveLength(3);
	});

	it('caps legacy and cloned items at maxItems', () => {
		const capped = resolveGuestItems({
			dataSource: 'context',
			children: [mkItem('tpl')],
			contextItems,
			maxItems: 2,
			cloneForContext,
		});
		expect(capped.contextChildren).toHaveLength(2);
		const legacy = resolveGuestItems({ dataSource: 'context', contextItems, maxItems: 1, cloneForContext });
		expect(legacy.legacyItems).toHaveLength(1);
	});

	it('does not cap manual children', () => {
		const res = resolveGuestItems({
			dataSource: 'manual',
			children: [mkItem('a'), mkItem('b'), mkItem('c')],
			contextItems,
			maxItems: 1,
			cloneForContext,
		});
		expect(res.manualChildren).toHaveLength(3);
	});
});
