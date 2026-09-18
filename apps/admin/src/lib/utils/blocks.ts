import type { Block, ContainerProps } from '@japaconnect/landing-blocks';

/** Block types that can contain nested child blocks in the page builder. */
export const CONTAINER_LIKE_TYPES = new Set([
	'container',
	'accordion',
	'tabs',
	'carousel',
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
]);

/** Whether a block can hold nested children. */
export function isContainerLike(b: Block): boolean {
	return CONTAINER_LIKE_TYPES.has(b.type);
}

/** The default slot key — `props.children`. */
export const DEFAULT_SLOT = 'children';

/**
 * A named array of nested blocks. Most container-like blocks keep children
 * in `props.children`; accordion, tabs, and carousel keep them in per-item
 * slots (`items[].content_blocks`, `tabs[].content_blocks`, `slides`).
 */
export interface ChildSlot {
	/** Stable slot identifier — 'children', 'slides', 'item:{i}', 'tab:{i}'. */
	key: string;
	/** Display label for the slot group (empty for single-slot blocks). */
	label: string;
	blocks: Block[];
}

/** Return the child-block slots of a container-like block. */
export function childSlotsOf(b: Block): ChildSlot[] {
	const props = b.props as Record<string, unknown> | undefined;
	if (b.type === 'accordion') {
		return ((props?.items as Array<{ title?: string; content_blocks?: Block[] }> | undefined) ?? []).map(
			(item, i) => ({
				key: `item:${i}`,
				label: item.title?.trim() ? `Item: ${item.title}` : `Item ${i + 1}`,
				blocks: item.content_blocks ?? [],
			})
		);
	}
	if (b.type === 'tabs') {
		return ((props?.tabs as Array<{ label?: string; content_blocks?: Block[] }> | undefined) ?? []).map(
			(tab, i) => ({
				key: `tab:${i}`,
				label: tab.label?.trim() ? `Tab: ${tab.label}` : `Tab ${i + 1}`,
				blocks: tab.content_blocks ?? [],
			})
		);
	}
	if (b.type === 'carousel') {
		return [{ key: 'slides', label: '', blocks: ((props?.slides as Block[] | undefined) ?? []) }];
	}
	return [{ key: DEFAULT_SLOT, label: '', blocks: ((props?.children as Block[] | undefined) ?? []) }];
}

/** Flattened list of all nested blocks across every slot. */
export function childrenOf(b: Block): Block[] {
	return childSlotsOf(b).flatMap((slot) => slot.blocks);
}

/**
 * Return a copy of `b` with `fn` applied to every nested-child slot array.
 * Only call on container-like blocks.
 */
export function mapChildSlots(b: Block, fn: (children: Block[], slot: ChildSlot) => Block[]): Block {
	const props = (b.props ?? {}) as Record<string, unknown>;
	if (b.type === 'accordion') {
		const items = ((props.items as Array<Record<string, unknown> & { content_blocks?: Block[] }> | undefined) ?? []).map(
			(item, i) => ({ ...item, content_blocks: fn(item.content_blocks ?? [], { key: `item:${i}`, label: '', blocks: [] }) })
		);
		return { ...b, props: { ...props, items } } as Block;
	}
	if (b.type === 'tabs') {
		const tabs = ((props.tabs as Array<Record<string, unknown> & { content_blocks?: Block[] }> | undefined) ?? []).map(
			(tab, i) => ({ ...tab, content_blocks: fn(tab.content_blocks ?? [], { key: `tab:${i}`, label: '', blocks: [] }) })
		);
		return { ...b, props: { ...props, tabs } } as Block;
	}
	if (b.type === 'carousel') {
		return {
			...b,
			props: { ...props, slides: fn(((props.slides as Block[] | undefined) ?? []), { key: 'slides', label: '', blocks: [] }) },
		} as Block;
	}
	return {
		...b,
		props: { ...props, children: fn(((props.children as Block[] | undefined) ?? []), { key: DEFAULT_SLOT, label: '', blocks: [] }) },
	} as Block;
}

/** The slot to append to when none is specified (last item/tab, slides, children). */
export function defaultSlotKey(b: Block): string {
	const slots = childSlotsOf(b);
	return slots.length > 0 ? slots[slots.length - 1].key : DEFAULT_SLOT;
}

/** Effective layout for a container-like block in the page builder sidebar. */
export function getContainerLayout(b: Block): ContainerProps['layout'] {
	const props = b.props as Partial<ContainerProps>;
	if (props.layout) return props.layout;
	// The generic container and hero container use the 12-col grid; every other
	// guest container renders its children as an ordered item list.
	if (b.type === 'container' || b.type === 'guest_hero') return 'grid';
	return 'stack';
}

/** Effective column count for a container-like block in the page builder sidebar. */
export function getContainerColumns(b: Block): number {
	const props = b.props as Partial<ContainerProps>;
	if (typeof props.columns === 'number' && props.columns > 0) return props.columns;
	if (b.type === 'guest_hero') return 2;
	return 1;
}

function mkBlock(type: Block['type'], props: Record<string, unknown>): Block {
	return { id: crypto.randomUUID(), type, visible: true, props } as Block;
}

function mkFooterLink(label: string, url?: string, icon?: string): Block {
	return mkBlock('guest_footer_link', { label, url, icon });
}

function mkFooterSocial(icon: string, url: string, label: string): Block {
	return mkBlock('guest_footer_social', { icon, url, label });
}

function mkFooterColumn(title: string, links: Array<{ label: string; url?: string; icon?: string }>): Block {
	return mkBlock('guest_footer_column', { title, children: links.map((l) => mkFooterLink(l.label, l.url, l.icon)) });
}

// Mirror the built-in defaults in GuestFooterBlock.svelte so a seeded footer
// renders identically to the legacy fallback markup.
const DEFAULT_FOOTER_SOCIALS: Array<[string, string, string]> = [
	['Facebook', 'https://facebook.com/japaconnect', 'Facebook'],
	['Twitter', 'https://twitter.com/japaconnect', 'Twitter'],
	['Instagram', 'https://instagram.com/japaconnect', 'Instagram'],
	['Youtube', 'https://youtube.com/japaconnect', 'YouTube'],
	['Linkedin', 'https://linkedin.com/company/japaconnect', 'LinkedIn'],
];

const DEFAULT_FOOTER_COLUMNS: Array<[string, Array<{ label: string; url?: string; icon?: string }>]> = [
	['Explore', [
		{ label: 'Jobs Abroad', url: '/jobs' },
		{ label: 'Study Abroad', url: '/scholarships' },
		{ label: 'Visa & Relocation', url: '/guides' },
		{ label: 'Business', url: '/jobs' },
		{ label: 'Countries', url: '/compare' },
	]],
	['Resources', [
		{ label: 'Blog', url: '/blog' },
		{ label: 'Guides', url: '/guides' },
		{ label: 'Tools', url: '/tools' },
		{ label: 'Compare Countries', url: '/compare' },
		{ label: 'Success Stories', url: '/stories' },
	]],
	['Company', [
		{ label: 'About Us', url: '/' },
		{ label: 'How It Works', url: '/' },
		{ label: 'Pricing', url: '/pricing' },
		{ label: 'FAQs', url: '/' },
		{ label: 'Contact Us', url: '/' },
	]],
	['Need Help?', [
		{ label: 'support@japaconnect.com', url: 'mailto:support@japaconnect.com', icon: 'Mail' },
		{ label: '+1 (234) 567-8900', url: 'tel:+12345678900', icon: 'Phone' },
		{ label: 'Mon - Fri: 9AM - 6PM (EST)', icon: 'Clock' },
	]],
];

const DEFAULT_FOOTER_BOTTOM_LINKS: Array<[string, string]> = [
	['Privacy Policy', '/'],
	['Terms of Service', '/'],
	['Cookies Policy', '/'],
];

const DEFAULT_HERO_CTAS: Array<[string, string, string]> = [
	['Get Started Free', '/signup', 'primary'],
	['Explore Opportunities', '/explore', 'ghost'],
];

/** Seed `guest_hero_text` children from the block's legacy `ctas` prop (or defaults). */
function seedHeroTextChildren(props: Record<string, unknown>): Block[] {
	const ctas = (props.ctas as Array<{ label?: string; href?: string; variant?: string; icon?: string }> | undefined) ?? [];
	const source: Array<{ label?: string; href?: string; variant?: string; icon?: string }> =
		ctas.length > 0 ? ctas : DEFAULT_HERO_CTAS.map(([label, href, variant]) => ({ label, href, variant }));
	return source.map((cta, i) =>
		mkBlock('guest_hero_cta_item', {
			label: cta.label,
			href: cta.href,
			variant: cta.variant ?? 'primary',
			icon: cta.icon ?? (i === 0 ? 'ArrowRight' : undefined),
		})
	);
}

/** Seed `guest_footer` children from the block's legacy socials/columns/bottom_links (or defaults). */
function seedGuestFooterChildren(props: Record<string, unknown>): Block[] {
	const socials = (props.socials as Array<{ icon: string; url: string; label: string }> | undefined) ?? [];
	const columns = (props.columns as Array<{ title: string; links?: Array<{ label: string; url?: string; target?: string; icon?: string }> }> | undefined) ?? [];
	const bottomLinks = (props.bottom_links as Array<{ label: string; url: string; target?: string }> | undefined) ?? [];

	return [
		...(socials.length > 0
			? socials.map((s) => mkFooterSocial(s.icon, s.url, s.label))
			: DEFAULT_FOOTER_SOCIALS.map(([icon, url, label]) => mkFooterSocial(icon, url, label))),
		...(columns.length > 0
			? columns.map((col) =>
					mkBlock('guest_footer_column', {
						title: col.title,
						children: (col.links ?? []).map((link) =>
							mkBlock('guest_footer_link', { label: link.label, url: link.url, target: link.target, icon: link.icon })
						),
					})
				)
			: DEFAULT_FOOTER_COLUMNS.map(([title, links]) => mkFooterColumn(title, links))),
		...(bottomLinks.length > 0
			? bottomLinks.map((l) => mkBlock('guest_footer_link', { label: l.label, url: l.url, target: l.target }))
			: DEFAULT_FOOTER_BOTTOM_LINKS.map(([label, url]) => mkFooterLink(label, url))),
	];
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
			// Members start with no image_url — the member block renders a
			// Users icon fallback. (The earlier seeds pointed at /brand/
			// avatar-*.jpg files that don't exist on any host and 404'd.)
			return [
				{ id: crypto.randomUUID(), type: 'guest_hero_community_member', visible: true, props: {} },
				{ id: crypto.randomUUID(), type: 'guest_hero_community_member', visible: true, props: {} },
				{ id: crypto.randomUUID(), type: 'guest_hero_community_member', visible: true, props: {} },
				{ id: crypto.randomUUID(), type: 'guest_hero_community_member', visible: true, props: {} },
			] as Block[];
		default:
			return undefined;
	}
}

/**
 * Whether a `guest_home` block carries legacy monolith props (copy, hero_image,
 * logo, newsletter_endpoint). Mirrors GuestHomeContainerBlock's isLegacy check —
 * such blocks must keep rendering via GuestHomeLegacyBlock, so children are
 * NOT seeded for them (any seeded children would switch the render to the
 * split container layout and make the legacy props dead).
 */
function hasGuestHomeLegacyData(props: Record<string, unknown>): boolean {
	const copy = props.copy as Record<string, unknown> | undefined;
	const logo = props.logo as { light?: string; dark?: string } | undefined;
	return !!(
		(copy && Object.keys(copy).length > 0) ||
		props.hero_image ||
		(logo && (logo.light || logo.dark)) ||
		props.newsletter_endpoint
	);
}

/** Recursively populate a container-like block with its default children when none are stored. */
export function seedBlockDefaults(b: Block): Block {
	if (!isContainerLike(b)) return b;
	const props = { ...(b.props as Record<string, unknown>) };
	const currentChildren = props.children as Block[] | undefined;
	let children: Block[] | undefined;
	if (currentChildren && currentChildren.length > 0) {
		children = currentChildren.map(seedBlockDefaults);
	} else {
		// For guest_hero_destinations/guest_hero_community, do not inject manual defaults when the block
		// is explicitly configured to pull from context data.
		if ((b.type === 'guest_hero_destinations' || b.type === 'guest_hero_community') && props.data_source === 'context') {
			children = currentChildren ?? [];
		} else if (b.type === 'guest_home' && hasGuestHomeLegacyData(props)) {
			// Legacy-configured guest_home keeps its monolithic render — leave
			// children absent so the block isn't silently migrated.
			children = currentChildren;
		} else if (b.type === 'guest_hero_text') {
			children = seedHeroTextChildren(props);
		} else if (b.type === 'guest_footer') {
			children = seedGuestFooterChildren(props);
		} else {
			// Blocks without a preset keep `children` absent (undefined) so legacy
			// `items` props still drive the render; an explicit [] is preserved.
			children = defaultChildrenFor(b.type)?.map(seedBlockDefaults) ?? currentChildren;
		}
	}
	// When seeding destination/community children, explicitly set the source to manual so the
	// editor preview matches the sidebar blocks.
	if (
		(b.type === 'guest_hero_destinations' || b.type === 'guest_hero_community') &&
		(!currentChildren || currentChildren.length === 0) &&
		children && children.length > 0
	) {
		props.data_source = 'manual';
	}
	const seeded = { ...b, props: children === undefined ? props : { ...props, children } } as Block;
	// Recurse into non-`children` slots (accordion items, tabs, carousel slides)
	// so nested container-like blocks there are seeded too.
	if (b.type === 'accordion' || b.type === 'tabs' || b.type === 'carousel') {
		return mapChildSlots(seeded, (slotChildren) => slotChildren.map(seedBlockDefaults));
	}
	return seeded;
}

/** Recursively populate all container-like blocks in a tree with default children. */
export function seedAllContainerDefaults(tree: Block[]): Block[] {
	return tree.map(seedBlockDefaults);
}
