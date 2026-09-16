import { describe, it, expect } from 'vitest';
import { defaultChildrenFor, seedBlockDefaults, seedAllContainerDefaults, isContainerLike, childrenOf } from './blocks';
import type { Block } from '@japaconnect/landing-blocks';

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
});
