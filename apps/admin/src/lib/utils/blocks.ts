import type { Block, ContainerProps } from '@japaconnect/landing-blocks';

/** Block types that can contain a nested `children` array in the page builder. */
export const CONTAINER_LIKE_TYPES = new Set([
	'container',
	'guest_home',
	'guest_hero',
	'guest_hero_image',
	'guest_hero_destinations',
	'guest_hero_community',
]);

/** Whether a block can hold nested children. */
export function isContainerLike(b: Block): boolean {
	return CONTAINER_LIKE_TYPES.has(b.type);
}

/** Return the child blocks of a container-like block. */
export function childrenOf(b: Block): Block[] {
	return ((b.props as { children?: Block[] }).children) ?? [];
}

/** Effective layout for a container-like block in the page builder sidebar. */
export function getContainerLayout(b: Block): ContainerProps['layout'] {
	const props = b.props as Partial<ContainerProps>;
	if (props.layout) return props.layout;
	if (b.type === 'guest_home' || b.type === 'guest_hero_image') return 'stack';
	return 'grid';
}

/** Effective column count for a container-like block in the page builder sidebar. */
export function getContainerColumns(b: Block): number {
	const props = b.props as Partial<ContainerProps>;
	if (typeof props.columns === 'number' && props.columns > 0) return props.columns;
	if (b.type === 'guest_hero') return 2;
	return 1;
}

/** Return the default child blocks for guest container presets. */
export function defaultChildrenFor(type: Block['type']): Block[] | undefined {
	switch (type) {
		case 'guest_home':
			return [
				{ id: crypto.randomUUID(), type: 'guest_hero', visible: true, props: { children: defaultChildrenFor('guest_hero') ?? [] } },
				{ id: crypto.randomUUID(), type: 'guest_categories', visible: true, props: {} },
				{ id: crypto.randomUUID(), type: 'guest_opportunities', visible: true, props: {} },
				{ id: crypto.randomUUID(), type: 'guest_stories', visible: true, props: {} },
				{ id: crypto.randomUUID(), type: 'guest_tools', visible: true, props: {} },
				{ id: crypto.randomUUID(), type: 'guest_newsletter', visible: true, props: {} },
				{ id: crypto.randomUUID(), type: 'guest_footer', visible: true, props: {} },
			] as Block[];
		case 'guest_hero':
			return [
				{ id: crypto.randomUUID(), type: 'guest_hero_text', visible: true, props: {} },
				{ id: crypto.randomUUID(), type: 'guest_hero_image', visible: true, props: { children: defaultChildrenFor('guest_hero_image') ?? [] } },
				{ id: crypto.randomUUID(), type: 'guest_hero_stats', visible: true, props: {} },
			] as Block[];
		case 'guest_hero_image':
			return [
				{ id: crypto.randomUUID(), type: 'guest_hero_destinations', visible: true, props: {} },
				{ id: crypto.randomUUID(), type: 'guest_hero_community', visible: true, props: {} },
			] as Block[];
		case 'guest_hero_destinations':
			return [
				{ id: crypto.randomUUID(), type: 'guest_hero_destination_item', visible: true, props: { name: 'USA' } },
				{ id: crypto.randomUUID(), type: 'guest_hero_destination_item', visible: true, props: { name: 'Canada' } },
				{ id: crypto.randomUUID(), type: 'guest_hero_destination_item', visible: true, props: { name: 'France' } },
			] as Block[];
		case 'guest_hero_community':
			return [
				{ id: crypto.randomUUID(), type: 'guest_hero_community_member', visible: true, props: { image_url: '/brand/avatar-1.jpg' } },
				{ id: crypto.randomUUID(), type: 'guest_hero_community_member', visible: true, props: { image_url: '/brand/avatar-2.jpg' } },
				{ id: crypto.randomUUID(), type: 'guest_hero_community_member', visible: true, props: { image_url: '/brand/avatar-3.jpg' } },
				{ id: crypto.randomUUID(), type: 'guest_hero_community_member', visible: true, props: { image_url: '/brand/avatar-4.jpg' } },
			] as Block[];
		default:
			return undefined;
	}
}

/** Recursively populate a container-like block with its default children when none are stored. */
export function seedBlockDefaults(b: Block): Block {
	if (!isContainerLike(b)) return b;
	const props = { ...(b.props as Record<string, unknown>) };
	const currentChildren = (props.children as Block[] | undefined) ?? [];
	let children: Block[];
	if (currentChildren.length === 0) {
		// For guest_hero_destinations/guest_hero_community, do not inject manual defaults when the block
		// is explicitly configured to pull from context data.
		if ((b.type === 'guest_hero_destinations' || b.type === 'guest_hero_community') && props.data_source === 'context') {
			children = [];
		} else {
			children = defaultChildrenFor(b.type)?.map(seedBlockDefaults) ?? [];
		}
	} else {
		children = currentChildren.map(seedBlockDefaults);
	}
	// When seeding destination/community children, explicitly set the source to manual so the
	// editor preview matches the sidebar blocks.
	if ((b.type === 'guest_hero_destinations' || b.type === 'guest_hero_community') && currentChildren.length === 0 && children.length > 0) {
		props.data_source = 'manual';
	}
	return { ...b, props: { ...props, children } } as Block;
}

/** Recursively populate all container-like blocks in a tree with default children. */
export function seedAllContainerDefaults(tree: Block[]): Block[] {
	return tree.map(seedBlockDefaults);
}
