import { describe, it, expect } from 'vitest';
import { defaultChildrenFor, seedBlockDefaults, seedAllContainerDefaults, isContainerLike, childrenOf, materializeGuestContextItems } from './blocks';
import type { Block, GuestHomeData } from '@japaconnect/landing-blocks';

describe('blocks helpers', () => {
	it('defaultChildrenFor returns nested defaults for guest_home', () => {
		const children = defaultChildrenFor('guest_home') ?? [];
		expect(children).toHaveLength(7);
		const hero = children.find((b) => b.type === 'guest_hero') as Block | undefined;
		expect(hero).toBeDefined();
		const heroChildren = childrenOf(hero as Block);
		expect(heroChildren).toHaveLength(3);
		const heroImage = heroChildren.find((b) => b.type === 'guest_hero_image') as Block | undefined;
		expect(heroImage).toBeDefined();
		expect(childrenOf(heroImage as Block)).toHaveLength(2);
	});

	it('seedBlockDefaults populates missing children for guest containers', () => {
		const guestHome: Block = { id: 'gh-1', type: 'guest_home', visible: true, props: {} } as Block;
		const seeded = seedBlockDefaults(guestHome);
		expect(isContainerLike(seeded)).toBe(true);
		const homeChildren = childrenOf(seeded);
		expect(homeChildren).toHaveLength(7);
		const hero = homeChildren.find((b) => b.type === 'guest_hero') as Block;
		expect(childrenOf(hero)).toHaveLength(3);
		const image = childrenOf(hero).find((b) => b.type === 'guest_hero_image') as Block;
		expect(childrenOf(image)).toHaveLength(2);
	});

	it('seedBlockDefaults preserves existing children', () => {
		const guestHero: Block = {
			id: 'gh-1',
			type: 'guest_hero',
			visible: true,
			props: {
				children: [{ id: 'custom', type: 'guest_hero_text', visible: true, props: {} } as Block]
			}
		} as Block;
		const seeded = seedBlockDefaults(guestHero);
		expect(childrenOf(seeded)).toHaveLength(1);
		expect(childrenOf(seeded)[0].id).toBe('custom');
	});

	it('seedAllContainerDefaults normalizes a tree', () => {
		const tree: Block[] = [
			{ id: 'gh-1', type: 'guest_home', visible: true, props: {} } as Block,
			{ id: 'txt', type: 'content', visible: true, props: {} } as Block,
		];
		const seeded = seedAllContainerDefaults(tree);
		expect(seeded).toHaveLength(2);
		expect(childrenOf(seeded[0])).toHaveLength(7);
	});

	it('recognizes all guest parent blocks as container-like', () => {
		const containerTypes = [
			'guest_home',
			'guest_hero',
			'guest_hero_text',
			'guest_hero_image',
			'guest_hero_stats',
			'guest_hero_destinations',
			'guest_hero_community',
			'guest_categories',
			'guest_opportunities',
			'guest_stories',
			'guest_tools',
			'guest_newsletter',
			'guest_footer',
			'guest_footer_column',
		];
		for (const type of containerTypes) {
			expect(isContainerLike({ id: 'x', type, visible: true, props: {} } as Block), type).toBe(true);
		}
		// Leaf item blocks are not containers.
		const leafTypes = [
			'guest_hero_cta_item',
			'guest_hero_stat_item',
			'guest_hero_destination_item',
			'guest_hero_community_member',
			'guest_category_item',
			'guest_opportunity_item',
			'guest_story_item',
			'guest_tool_item',
			'guest_footer_link',
			'guest_footer_social',
		];
		for (const type of leafTypes) {
			expect(isContainerLike({ id: 'x', type, visible: true, props: {} } as Block), type).toBe(false);
		}
	});

	it('seedBlockDefaults seeds CTA children for guest_hero_text', () => {
		const heroText: Block = { id: 'ht-1', type: 'guest_hero_text', visible: true, props: {} } as Block;
		const seeded = seedBlockDefaults(heroText);
		const children = childrenOf(seeded);
		expect(children).toHaveLength(2);
		expect(children.every((c) => c.type === 'guest_hero_cta_item')).toBe(true);
	});

	it('seedBlockDefaults converts legacy ctas into children for guest_hero_text', () => {
		const heroText: Block = {
			id: 'ht-1',
			type: 'guest_hero_text',
			visible: true,
			props: { ctas: [{ label: 'Join Now', href: '/signup', variant: 'primary' }] },
		} as Block;
		const seeded = seedBlockDefaults(heroText);
		const children = childrenOf(seeded);
		expect(children).toHaveLength(1);
		expect(children[0].type).toBe('guest_hero_cta_item');
		expect((children[0].props as { label?: string }).label).toBe('Join Now');
	});

	it('seedBlockDefaults seeds footer children partitioned by area', () => {
		const footer: Block = { id: 'ft-1', type: 'guest_footer', visible: true, props: {} } as Block;
		const seeded = seedBlockDefaults(footer);
		const children = childrenOf(seeded);
		expect(children.filter((c) => c.type === 'guest_footer_social')).toHaveLength(5);
		const columns = children.filter((c) => c.type === 'guest_footer_column');
		expect(columns).toHaveLength(4);
		// Each seeded column contains its own link children.
		expect(childrenOf(columns[0]).length).toBeGreaterThan(0);
		expect(childrenOf(columns[0]).every((l) => l.type === 'guest_footer_link')).toBe(true);
		expect(children.filter((c) => c.type === 'guest_footer_link')).toHaveLength(3);
	});

	it('seedBlockDefaults converts legacy footer props into children', () => {
		const footer: Block = {
			id: 'ft-1',
			type: 'guest_footer',
			visible: true,
			props: {
				socials: [{ icon: 'Twitter', url: 'https://x.com/japa', label: 'X' }],
				columns: [{ title: 'Links', links: [{ label: 'Home', url: '/' }] }],
				bottom_links: [{ label: 'Privacy', url: '/privacy' }],
			},
		} as Block;
		const seeded = seedBlockDefaults(footer);
		const children = childrenOf(seeded);
		expect(children.filter((c) => c.type === 'guest_footer_social')).toHaveLength(1);
		const columns = children.filter((c) => c.type === 'guest_footer_column');
		expect(columns).toHaveLength(1);
		expect((columns[0].props as { title?: string }).title).toBe('Links');
		expect(childrenOf(columns[0])).toHaveLength(1);
		expect(children.filter((c) => c.type === 'guest_footer_link')).toHaveLength(1);
	});

	it('seedBlockDefaults leaves list blocks unseeded so context data renders', () => {
		const categories: Block = { id: 'cat-1', type: 'guest_categories', visible: true, props: {} } as Block;
		const seeded = seedBlockDefaults(categories);
		// children stays absent — the block falls back to context/legacy items.
		expect((seeded.props as { children?: Block[] }).children).toBeUndefined();
	});

	it('seedBlockDefaults seeds nested defaults inside guest_home', () => {
		const guestHome: Block = { id: 'gh-1', type: 'guest_home', visible: true, props: {} } as Block;
		const seeded = seedBlockDefaults(guestHome);
		const hero = childrenOf(seeded).find((b) => b.type === 'guest_hero') as Block;
		const heroText = childrenOf(hero).find((b) => b.type === 'guest_hero_text') as Block;
		expect(childrenOf(heroText)).toHaveLength(2);
		const footer = childrenOf(seeded).find((b) => b.type === 'guest_footer') as Block;
		expect(childrenOf(footer).length).toBeGreaterThan(0);
	});

	it('seedBlockDefaults leaves legacy-configured guest_home unseeded', () => {
		const legacy: Block = {
			id: 'gh-legacy',
			type: 'guest_home',
			visible: true,
			props: { copy: { headline: 'Old headline' }, hero_image: '/old.png' },
		} as Block;
		const seeded = seedBlockDefaults(legacy);
		// children stays absent so the monolithic legacy render keeps working
		// and the copy/hero_image props aren't shadowed by seeded children.
		expect((seeded.props as { children?: Block[] }).children).toBeUndefined();
	});

	const guestCtx: GuestHomeData = {
		topDestinations: [],
		stories: [],
		stats: [
			{ value: '50K+', label: 'Opportunities', icon: 'Sparkles' },
			{ value: '185+', label: 'Countries', icon: 'Globe' },
		],
		categories: [
			{ id: 'jobs', title: 'Find Jobs Abroad', description: 'Jobs.', href: '/jobs', icon: 'Briefcase', color: 'green' },
		],
		opportunities: [
			{ id: 'o1', type: 'job', title: 'Software Engineer', organization: 'Acme', location: 'Canada', tag: 'Full-time', href: '/jobs' },
		],
		tools: [
			{ id: 't1', name: 'Visa Checklist', slug: 'visa-checklist', href: '/tools/visa-checklist', icon: 'FileCheck' },
		],
	};

	it('materializeGuestContextItems seeds item children for context-driven list blocks', () => {
		const guestHome: Block = {
			id: 'gh-1',
			type: 'guest_home',
			visible: true,
			props: {
				children: [
					{ id: 'stats-1', type: 'guest_hero_stats', visible: true, props: {} } as Block,
					{ id: 'cat-1', type: 'guest_categories', visible: true, props: {} } as Block,
					{ id: 'opp-1', type: 'guest_opportunities', visible: true, props: {} } as Block,
					{ id: 'tool-1', type: 'guest_tools', visible: true, props: {} } as Block,
					{ id: 'nl-1', type: 'guest_newsletter', visible: true, props: {} } as Block,
				],
			},
		} as Block;
		const [out] = materializeGuestContextItems([guestHome], guestCtx);
		const kids = childrenOf(out);
		const stats = kids.find((b) => b.type === 'guest_hero_stats') as Block;
		expect(childrenOf(stats)).toHaveLength(2);
		expect(childrenOf(stats).every((c) => c.type === 'guest_hero_stat_item')).toBe(true);
		expect((stats.props as { data_source?: string }).data_source).toBe('manual');
		// Sidebar label carries the stat text.
		expect(childrenOf(stats)[0].label).toBe('50K+ Opportunities');
		const cats = kids.find((b) => b.type === 'guest_categories') as Block;
		expect(childrenOf(cats)).toHaveLength(1);
		expect((childrenOf(cats)[0].props as { title?: string }).title).toBe('Find Jobs Abroad');
		const opps = kids.find((b) => b.type === 'guest_opportunities') as Block;
		expect(childrenOf(opps)).toHaveLength(1);
		const tools = kids.find((b) => b.type === 'guest_tools') as Block;
		expect(childrenOf(tools)).toHaveLength(1);
		// Newsletter takes arbitrary children — nothing is seeded for it.
		const newsletter = kids.find((b) => b.type === 'guest_newsletter') as Block;
		expect(childrenOf(newsletter)).toHaveLength(0);
	});

	it('materializeGuestContextItems skips blocks pinned to context mode and blocks with children', () => {
		const existing: Block = { id: 'x', type: 'guest_category_item', visible: true, props: { title: 'Mine' } } as Block;
		const tree: Block[] = [
			{ id: 'c1', type: 'guest_categories', visible: true, props: { data_source: 'context' } } as Block,
			{ id: 'c2', type: 'guest_categories', visible: true, props: { children: [existing] } } as Block,
		];
		const [ctxBlock, manualBlock] = materializeGuestContextItems(tree, guestCtx);
		expect(childrenOf(ctxBlock)).toHaveLength(0);
		expect(childrenOf(manualBlock)).toHaveLength(1);
		expect(childrenOf(manualBlock)[0].id).toBe('x');
	});

	it('materializeGuestContextItems honors max_items and seeds nested lists', () => {
		const tree: Block[] = [
			{
				id: 'hero',
				type: 'guest_hero',
				visible: true,
				props: {
					children: [
						{ id: 's1', type: 'guest_hero_stats', visible: true, props: { max_items: 1 } } as Block,
					],
				},
			} as Block,
		];
		const [hero] = materializeGuestContextItems(tree, guestCtx);
		const stats = childrenOf(hero)[0];
		expect(childrenOf(stats)).toHaveLength(1);
		// No context data → tree returned unchanged.
		expect(materializeGuestContextItems(tree, undefined)[0]).toEqual(tree[0]);
	});
});
