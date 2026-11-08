import { describe, it, expect } from 'vitest';
import { resolveView, viewsByGroup, DEFAULT_VIEW } from './dashboardViews';
import type { Menu, MenuItem } from '#lib/types/index.js';

const item = (over: Partial<MenuItem> = {}): MenuItem => ({
	id: 'x',
	label: 'Item',
	url: '/dashboard/overview',
	icon: null,
	sort_order: 0,
	visibility: 'authenticated',
	target: '_self',
	is_ad_slot: false,
	status: 'active',
	...over
});

const menu = (items: MenuItem[]): Menu => ({
	id: 'm',
	slug: 'dashboard_rail',
	name: 'Dashboard Rail',
	placement: 'dashboard_rail',
	items
});

describe('dashboard registry', () => {
	it('defaults to overview and resolves known views', () => {
		expect(DEFAULT_VIEW).toBe('overview');
		expect(resolveView('relationships')?.slug).toBe('relationships');
		expect(resolveView('nope')).toBeNull();
	});
	it('flags analytics as backend-ready', () => {
		expect(resolveView('analytics')?.backendReady).toBe(true);
	});
});

describe('viewsByGroup', () => {
	it('groups flat items under the menu name', () => {
		const groups = viewsByGroup(
			menu([item({ url: '/dashboard/overview' }), item({ url: '/dashboard/tickets' })])
		);
		expect(groups).toHaveLength(1);
		expect(groups[0][0]).toBe('Dashboard Rail');
		expect(groups[0][1].map((v) => v.slug)).toEqual(['overview', 'tickets']);
	});

	it('turns a parent with children into its own group and keeps its URL reachable', () => {
		const groups = viewsByGroup(
			menu([
				item({
					label: 'Learning',
					url: '/dashboard/academy',
					children: [
						item({ url: '/dashboard/certificates' }),
						item({ url: '/dashboard/knowledge-base' })
					]
				})
			])
		);
		expect(groups.map(([g]) => g)).toEqual(['Dashboard Rail', 'Learning']);
		expect(groups[0][1].map((v) => v.slug)).toEqual(['academy']);
		expect(groups[1][1].map((v) => v.slug)).toEqual(['certificates', 'knowledge-base']);
	});

	it('treats a bare /dashboard parent as a non-leaf group header', () => {
		const groups = viewsByGroup(
			menu([
				item({
					label: 'Group',
					url: '/dashboard',
					children: [item({ url: '/dashboard/media' })]
				})
			])
		);
		expect(groups.map(([g]) => g)).toEqual(['Group']);
		expect(groups[0][1].map((v) => v.slug)).toEqual(['media']);
	});

	it('drops inactive, ad-slot, and url-less items at both levels', () => {
		const groups = viewsByGroup(
			menu([
				item({ status: 'inactive' }),
				item({ is_ad_slot: true }),
				item({ url: '' }),
				item({
					label: 'Group',
					url: '/dashboard',
					children: [item({ url: '/dashboard/media' }), item({ url: '/dashboard/x', status: 'inactive' })]
				})
			])
		);
		expect(groups.map(([g]) => g)).toEqual(['Group']);
		expect(groups[0][1].map((v) => v.slug)).toEqual(['media']);
	});
});
